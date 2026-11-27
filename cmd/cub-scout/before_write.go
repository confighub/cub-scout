// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

var (
	beforeWriteNamespace          string
	beforeWriteFieldPath          string
	beforeWriteRecording          string
	beforeWriteKubeContext        string
	beforeWriteFormat             string
	beforeWriteIncludeBlastRadius bool
)

const (
	beforeWriteCostCoverage            = "before-write-session-client"
	beforeWriteBlastRadiusMaxWorkloads = 5
)

var beforeWriteCmd = &cobra.Command{
	Use:   "before-write <kind/name> or <kind> <name>",
	Short: "Read-only pre-write safety check: ownership, revert risk, sync windows, contention, and PDBs",
	Long: `Inspect a target resource before making a write or patch: who owns it,
whether its GitOps deployer will revert an edit (Flux suspend, Argo CD
auto-sync/self-heal/syncWindows), field-level writer contention, and matching
PodDisruptionBudgets in the same namespace.

Read-only: never modifies cluster state. Supports --recording <file> for
offline evaluation of recorded cluster snapshots.

Examples:
  cub scout before-write deploy/api -n prod --format json
  cub scout before-write deploy/api -n prod --field-path .spec.replicas --format json
  cub scout before-write deploy/api -n prod --recording snapshot.yaml --format json`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runBeforeWrite,
}

func init() {
	rootCmd.AddCommand(beforeWriteCmd)
	beforeWriteCmd.Flags().StringVarP(&beforeWriteNamespace, "namespace", "n", "default", "Namespace of the target resource")
	beforeWriteCmd.Flags().StringVar(&beforeWriteFieldPath, "field-path", "", "Exact canonical managedFields path to inspect (for example .spec.replicas)")
	beforeWriteCmd.Flags().StringVar(&beforeWriteRecording, "recording", "", "Offline YAML/JSON cluster snapshot file to evaluate without cluster access")
	beforeWriteCmd.Flags().StringVar(&beforeWriteKubeContext, "kube-context", "", "Exact kube context for all Kubernetes reads; does not change the current context")
	beforeWriteCmd.Flags().StringVar(&beforeWriteFormat, "format", "ascii", "Output format: ascii, json, md")
	beforeWriteCmd.Flags().BoolVar(&beforeWriteIncludeBlastRadius, "include-blast-radius", false, "Include bounded dependency blast-radius summary (affected workloads capped at 5)")
}

// beforeWriteOutput is the compact JSON document emitted by
// `cub scout before-write --format json` and the `before_write` MCP tool.
type beforeWriteOutput struct {
	Context          string                       `json:"context,omitempty"`
	Target           beforeWriteTarget            `json:"target"`
	Owner            beforeWriteOwner             `json:"owner"`
	Deployer         *beforeWriteDeployerSettings `json:"deployer,omitempty"`
	FieldAttribution *FieldAttributionSummary     `json:"fieldAttribution,omitempty"`
	PDBs             []beforeWritePDB             `json:"pdbs,omitempty"`
	BlastRadius      *beforeWriteBlastRadius      `json:"blastRadius,omitempty"`
	MutationCause    agent.FieldMutationCause     `json:"mutationCause,omitempty"`
	MutationManager  string                       `json:"mutationManager,omitempty"`
	Answer           *mapsvc.Answer               `json:"answer"`
}

// beforeWriteBlastRadius is a compact, bounded blast-radius summary attached to
// beforeWriteOutput when --include-blast-radius is enabled.
type beforeWriteBlastRadius struct {
	AffectedWorkloadsCount int      `json:"affectedWorkloadsCount"`
	AffectedWorkloads      []string `json:"affectedWorkloads"`
	Truncated              bool     `json:"truncated"`
}

type beforeWriteTarget struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type beforeWriteOwner struct {
	Type       string `json:"type"`
	Display    string `json:"display"`
	SubType    string `json:"subType,omitempty"`
	Name       string `json:"name,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Source     string `json:"source,omitempty"`
	Confidence string `json:"confidence,omitempty"`
}

type beforeWriteSyncWindow struct {
	Kind         string   `json:"kind,omitempty"`
	Schedule     string   `json:"schedule,omitempty"`
	Duration     string   `json:"duration,omitempty"`
	TimeZone     string   `json:"timeZone,omitempty"`
	ManualSync   bool     `json:"manualSync,omitempty"`
	Applications []string `json:"applications,omitempty"`
	Namespaces   []string `json:"namespaces,omitempty"`
	Clusters     []string `json:"clusters,omitempty"`
}

type beforeWriteDeployerSettings struct {
	Ref             string                  `json:"ref"`
	Kind            string                  `json:"kind"`
	Namespace       string                  `json:"namespace,omitempty"`
	Name            string                  `json:"name"`
	Suspended       *bool                   `json:"suspended,omitempty"`
	AutoSync        *bool                   `json:"autoSync,omitempty"`
	SelfHeal        *bool                   `json:"selfHeal,omitempty"`
	Prune           *bool                   `json:"prune,omitempty"`
	SyncOptions     []string                `json:"syncOptions,omitempty"`
	SyncWindows     []beforeWriteSyncWindow `json:"syncWindows,omitempty"`
	IgnoreDiffCount int                     `json:"ignoreDiffCount,omitempty"`
}

type beforeWritePDB struct {
	Name               string `json:"name"`
	MinAvailable       string `json:"minAvailable,omitempty"`
	MaxUnavailable     string `json:"maxUnavailable,omitempty"`
	DisruptionsAllowed *int64 `json:"disruptionsAllowed,omitempty"`
	CurrentHealthy     *int64 `json:"currentHealthy,omitempty"`
	DesiredHealthy     *int64 `json:"desiredHealthy,omitempty"`
}

type beforeWriteStore interface {
	GetResource(ctx context.Context, kind, name, namespace string) (*unstructured.Unstructured, error)
	GetByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error)
	ListPDBs(ctx context.Context, namespace string) ([]unstructured.Unstructured, error)
}

type beforeWriteBlastRadiusStore interface {
	ComputeBlastRadius(ctx context.Context, kind, name, namespace string, opts beforeWriteOptions) (blastRadiusOutput, error)
}

var pdbPolicyV1GVR = schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}
var argoAppProjectGVR = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "appprojects"}

type dynamicBeforeWriteStore struct {
	dyn dynamic.Interface
}

func (s dynamicBeforeWriteStore) GetResource(ctx context.Context, kind, name, namespace string) (*unstructured.Unstructured, error) {
	return fetchTraceResource(ctx, s.dyn, kind, name, namespace)
}

func (s dynamicBeforeWriteStore) GetByGVR(ctx context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	if namespace != "" {
		return s.dyn.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	}
	return s.dyn.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
}

func (s dynamicBeforeWriteStore) ListPDBs(ctx context.Context, namespace string) ([]unstructured.Unstructured, error) {
	list, err := s.dyn.Resource(pdbPolicyV1GVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s dynamicBeforeWriteStore) ComputeBlastRadius(ctx context.Context, kind, name, namespace string, opts beforeWriteOptions) (blastRadiusOutput, error) {
	objects, listTruncated, err := collectLiveBlastRadiusObjects(ctx, s.dyn, namespace, defaultSnapshotMaxObjects)
	if err != nil {
		return blastRadiusOutput{}, err
	}
	observedAt := opts.ObservedAt
	if observedAt.IsZero() {
		observedAt = blastRadiusNow().UTC()
	}
	snapshot := recordedObjectSnapshot{
		Objects:       objects,
		ObservedAt:    observedAt.UTC().Format(time.RFC3339),
		SourceContext: opts.Context,
	}
	return computeBlastRadiusWithMode(snapshot, kind, name, namespace, true, listTruncated), nil
}

type recordedBeforeWriteStore struct {
	objects []*unstructured.Unstructured
}

func (s recordedBeforeWriteStore) ComputeBlastRadius(_ context.Context, kind, name, namespace string, opts beforeWriteOptions) (blastRadiusOutput, error) {
	var snap recordedObjectSnapshot
	if opts.Recorded != nil {
		snap = *opts.Recorded
	} else {
		observedAt := opts.ObservedAt
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		snap = recordedObjectSnapshot{
			Objects:       s.objects,
			SourceContext: opts.Context,
			ObservedAt:    observedAt.UTC().Format(time.RFC3339),
		}
	}
	return computeBlastRadius(snap, kind, name, namespace), nil
}

func (s recordedBeforeWriteStore) GetResource(_ context.Context, kind, name, namespace string) (*unstructured.Unstructured, error) {
	wantKind := normalizeKind(kind)
	for _, obj := range s.objects {
		if obj == nil {
			continue
		}
		if !strings.EqualFold(normalizeKind(obj.GetKind()), wantKind) {
			continue
		}
		if obj.GetName() == name && (namespace == "" || obj.GetNamespace() == namespace) {
			return obj.DeepCopy(), nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Resource: strings.ToLower(wantKind)}, name)
}

func (s recordedBeforeWriteStore) GetByGVR(_ context.Context, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	for _, obj := range s.objects {
		if obj == nil {
			continue
		}
		if !beforeWriteGVRMatchesObject(gvr, obj) {
			continue
		}
		if obj.GetName() == name && (namespace == "" || obj.GetNamespace() == namespace) {
			return obj.DeepCopy(), nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource}, name)
}

func (s recordedBeforeWriteStore) ListPDBs(_ context.Context, namespace string) ([]unstructured.Unstructured, error) {
	var out []unstructured.Unstructured
	for _, obj := range s.objects {
		if obj == nil {
			continue
		}
		if !beforeWriteGVRMatchesObject(pdbPolicyV1GVR, obj) {
			continue
		}
		if namespace != "" && obj.GetNamespace() != namespace {
			continue
		}
		out = append(out, *obj.DeepCopy())
	}
	return out, nil
}

func beforeWriteGVRMatchesObject(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) bool {
	if gvrMatchesRecordedObject(gvr, obj) {
		return true
	}
	gv, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil || gv.Group != gvr.Group {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(obj.GetKind()))
	switch gvr.Resource {
	case "poddisruptionbudgets":
		return kind == "poddisruptionbudget"
	case "appprojects":
		return kind == "appproject"
	default:
		return pluralizeKind(kind) == gvr.Resource
	}
}

type beforeWriteOptions struct {
	Context            string
	Kind               string
	Name               string
	Namespace          string
	FieldPath          string
	IncludeBlastRadius bool
	Recorded           *recordedObjectSnapshot
	ObservedAt         time.Time
	Cost               *agent.KubernetesReadCost
}

var newBeforeWriteSessionForSelection = newTraceSessionForSelection

func runBeforeWrite(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(beforeWriteFormat))
	if format == "text" {
		format = "ascii"
	}
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("unsupported format %q (supported: ascii, json, md)", beforeWriteFormat)
	}
	if strings.TrimSpace(beforeWriteFieldPath) != "" {
		if err := agent.ValidateCanonicalFieldPath(beforeWriteFieldPath); err != nil {
			return err
		}
	}
	kind, name, err := parseExplainArgs(args)
	if err != nil {
		return err
	}
	ns := strings.TrimSpace(beforeWriteNamespace)
	if ns == "" {
		ns = "default"
	}

	selection, err := clusterContextSelectionFromFlag(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	if strings.TrimSpace(beforeWriteRecording) != "" {
		if selection.explicit {
			return fmt.Errorf("--recording cannot be combined with --kube-context")
		}
		snapshot, err := readRecordedObjectSnapshot(beforeWriteRecording)
		if err != nil {
			return err
		}
		store := recordedBeforeWriteStore{objects: snapshot.Objects}
		out := evaluateBeforeWrite(ctx, store, beforeWriteOptions{
			Context:            snapshot.SourceContext,
			Kind:               kind,
			Name:               name,
			Namespace:          ns,
			FieldPath:          strings.TrimSpace(beforeWriteFieldPath),
			IncludeBlastRadius: beforeWriteIncludeBlastRadius,
			Recorded:           &snapshot,
			ObservedAt:         offlineRecordingObservedAt(snapshot),
		})
		return renderBeforeWriteOutput(cmd.OutOrStdout(), out, format)
	}

	started := time.Now()
	session, sessErr := newBeforeWriteSessionForSelection(selection)
	if sessErr != nil {
		out := buildBeforeWriteReadErrorOutput(beforeWriteOptions{
			Context:            selection.name,
			Kind:               kind,
			Name:               name,
			Namespace:          ns,
			FieldPath:          strings.TrimSpace(beforeWriteFieldPath),
			IncludeBlastRadius: beforeWriteIncludeBlastRadius,
			ObservedAt:         time.Now().UTC(),
		}, sessErr)
		return renderBeforeWriteOutput(cmd.OutOrStdout(), out, format)
	}

	dynClient, err := session.dynamicClient()
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	out := evaluateBeforeWrite(ctx, dynamicBeforeWriteStore{dyn: dynClient}, beforeWriteOptions{
		Context:            session.contextLabel(),
		Kind:               kind,
		Name:               name,
		Namespace:          ns,
		FieldPath:          strings.TrimSpace(beforeWriteFieldPath),
		IncludeBlastRadius: beforeWriteIncludeBlastRadius,
		ObservedAt:         time.Now().UTC(),
	})
	if session.meter != nil {
		cost := session.meter.Snapshot()
		cost.DurationMillis = time.Since(started).Milliseconds()
		cost.Coverage = beforeWriteCostCoverage
		if out.Answer != nil {
			out.Answer.Cost = mapsvc.AnswerCostFrom(cost)
		}
	}
	return renderBeforeWriteOutput(cmd.OutOrStdout(), out, format)
}

func buildBeforeWriteReadErrorOutput(opts beforeWriteOptions, readErr error) beforeWriteOutput {
	observedAt := opts.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	out := beforeWriteOutput{
		Context: opts.Context,
		Target:  beforeWriteTarget{Kind: opts.Kind, Name: opts.Name, Namespace: opts.Namespace},
		Owner:   beforeWriteOwner{Type: agent.OwnerUnknown, Display: mapsvc.DisplayOwner(agent.OwnerUnknown)},
	}
	if opts.FieldPath != "" {
		out.FieldAttribution = unavailableFieldAttribution(opts.FieldPath, "Selected live object could not be read.")
	}
	out.Answer = buildBeforeWriteAnswer(out, beforeWriteAnswerFacts{
		objectRead: false,
		readError:  readErrString(readErr),
		recorded:   opts.Recorded,
		observedAt: observedAt,
		cost:       opts.Cost,
	})
	return out
}

func evaluateBeforeWrite(ctx context.Context, store beforeWriteStore, opts beforeWriteOptions) beforeWriteOutput {
	observedAt := opts.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	obj, err := store.GetResource(ctx, opts.Kind, opts.Name, opts.Namespace)
	if err != nil || obj == nil {
		if err == nil {
			err = fmt.Errorf("resource %s/%s not found in namespace %s", opts.Kind, opts.Name, opts.Namespace)
		}
		return buildBeforeWriteReadErrorOutput(opts, err)
	}

	owner := agent.DetectOwnership(obj)
	if owner.Type == agent.OwnerUnknown && isCrossplaneProviderConfig(obj) {
		owner = agent.Ownership{
			Type:       agent.OwnerCrossplane,
			SubType:    "providerconfig",
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			Source:     "apiGroup:" + obj.GroupVersionKind().Group,
			Confidence: "high",
		}
	}

	mutAttr := agent.AttributeFieldMutation(obj, owner)
	out := beforeWriteOutput{
		Context: opts.Context,
		Target: beforeWriteTarget{
			Kind:      obj.GetKind(),
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		},
		Owner: beforeWriteOwner{
			Type:       owner.Type,
			Display:    mapsvc.DisplayOwner(owner.Type),
			SubType:    owner.SubType,
			Name:       owner.Name,
			Namespace:  owner.Namespace,
			Source:     owner.Source,
			Confidence: owner.Confidence,
		},
		MutationCause:   mutAttr.Cause,
		MutationManager: mutAttr.ManagerHint,
	}
	if out.Target.Kind == "" {
		out.Target.Kind = opts.Kind
	}
	if out.Target.Namespace == "" {
		out.Target.Namespace = opts.Namespace
	}

	var deployerObj *unstructured.Unstructured
	var deployerErr error
	deployerAttempted := false
	if gvr, deployerNS, deployerName, ok := beforeWriteDeployerRef(owner); ok {
		deployerAttempted = true
		deployerObj, deployerErr = store.GetByGVR(ctx, gvr, deployerNS, deployerName)
		if deployerErr == nil && deployerObj != nil {
			settings := extractBeforeWriteDeployerSettings(owner, deployerObj)
			// For Argo CD Applications without inline syncWindows, also check
			// the referenced AppProject in the same namespace if readable.
			if owner.Type == agent.OwnerArgo && len(settings.SyncWindows) == 0 {
				if project, _, _ := unstructured.NestedString(deployerObj.Object, "spec", "project"); strings.TrimSpace(project) != "" {
					if projObj, projErr := store.GetByGVR(ctx, argoAppProjectGVR, deployerNS, strings.TrimSpace(project)); projErr == nil && projObj != nil {
						settings.SyncWindows = extractArgoSyncWindows(projObj)
					}
				}
			}
			out.Deployer = &settings
		}
	}

	if opts.FieldPath != "" {
		attr := fieldAttributionSummary(obj, owner, opts.FieldPath)
		if deployerObj != nil {
			pred := agent.PredictFieldRevert(owner, deployerObj, obj.GetKind(), opts.FieldPath)
			attr.RevertPrediction = &pred
		}
		out.FieldAttribution = attr
	}

	var pdbErr error
	if podLabels := extractPodTemplateLabels(obj); len(podLabels) > 0 && out.Target.Namespace != "" {
		var pdbs []unstructured.Unstructured
		pdbs, pdbErr = store.ListPDBs(ctx, out.Target.Namespace)
		if pdbErr == nil {
			out.PDBs = matchPDBsForLabels(pdbs, podLabels)
		}
	}

	var blastErr error
	if opts.IncludeBlastRadius {
		if brStore, ok := store.(beforeWriteBlastRadiusStore); ok {
			brOut, err := brStore.ComputeBlastRadius(ctx, out.Target.Kind, out.Target.Name, out.Target.Namespace, opts)
			if err != nil {
				blastErr = err
			} else {
				out.BlastRadius = summarizeBeforeWriteBlastRadius(brOut)
			}
		}
	}

	out.Answer = buildBeforeWriteAnswer(out, beforeWriteAnswerFacts{
		objectRead:        true,
		owner:             owner,
		deployerAttempted: deployerAttempted,
		deployerRead:      deployerObj != nil,
		deployerError:     readErrString(deployerErr),
		pdbError:          readErrString(pdbErr),
		blastError:        readErrString(blastErr),
		recorded:          opts.Recorded,
		observedAt:        observedAt,
		cost:              opts.Cost,
	})
	return out
}

func summarizeBeforeWriteBlastRadius(br blastRadiusOutput) *beforeWriteBlastRadius {
	total := len(br.AffectedWorkloads)
	truncated := br.Truncated
	workloads := br.AffectedWorkloads
	if len(workloads) > beforeWriteBlastRadiusMaxWorkloads {
		workloads = workloads[:beforeWriteBlastRadiusMaxWorkloads]
		truncated = true
	}
	out := make([]string, len(workloads))
	copy(out, workloads)
	return &beforeWriteBlastRadius{
		AffectedWorkloadsCount: total,
		AffectedWorkloads:      out,
		Truncated:              truncated,
	}
}

func beforeWriteDeployerRef(owner agent.Ownership) (schema.GroupVersionResource, string, string, bool) {
	name := strings.TrimSpace(owner.Name)
	if name == "" {
		return schema.GroupVersionResource{}, "", "", false
	}
	ns := strings.TrimSpace(owner.Namespace)
	switch owner.Type {
	case agent.OwnerFlux:
		if ns == "" {
			ns = "flux-system"
		}
		if strings.EqualFold(owner.SubType, "helmrelease") {
			return schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}, ns, name, true
		}
		return schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}, ns, name, true
	case agent.OwnerArgo:
		if ns == "" {
			ns = "argocd"
		}
		return schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}, ns, name, true
	default:
		return schema.GroupVersionResource{}, "", "", false
	}
}

func extractBeforeWriteDeployerSettings(owner agent.Ownership, deployer *unstructured.Unstructured) beforeWriteDeployerSettings {
	ref := deployer.GetKind() + "/" + deployer.GetName()
	if deployer.GetNamespace() != "" {
		ref = deployer.GetKind() + "/" + deployer.GetNamespace() + "/" + deployer.GetName()
	}
	settings := beforeWriteDeployerSettings{
		Ref:       ref,
		Kind:      deployer.GetKind(),
		Namespace: deployer.GetNamespace(),
		Name:      deployer.GetName(),
	}
	switch owner.Type {
	case agent.OwnerFlux:
		suspended, _, _ := unstructured.NestedBool(deployer.Object, "spec", "suspend")
		settings.Suspended = &suspended
	case agent.OwnerArgo:
		automated, hasAuto, _ := unstructured.NestedMap(deployer.Object, "spec", "syncPolicy", "automated")
		autoEnabled := hasAuto && automated != nil
		settings.AutoSync = &autoEnabled
		if autoEnabled {
			selfHeal, _, _ := unstructured.NestedBool(deployer.Object, "spec", "syncPolicy", "automated", "selfHeal")
			prune, _, _ := unstructured.NestedBool(deployer.Object, "spec", "syncPolicy", "automated", "prune")
			settings.SelfHeal = &selfHeal
			settings.Prune = &prune
		}
		if opts, found, _ := unstructured.NestedStringSlice(deployer.Object, "spec", "syncPolicy", "syncOptions"); found && len(opts) > 0 {
			sorted := append([]string(nil), opts...)
			sort.Strings(sorted)
			settings.SyncOptions = sorted
		}
		settings.SyncWindows = extractArgoSyncWindows(deployer)
		if diffs, found, _ := unstructured.NestedSlice(deployer.Object, "spec", "ignoreDifferences"); found && len(diffs) > 0 {
			settings.IgnoreDiffCount = len(diffs)
		}
	}
	return settings
}

func extractArgoSyncWindows(obj *unstructured.Unstructured) []beforeWriteSyncWindow {
	if obj == nil {
		return nil
	}
	rawWindows, found, err := unstructured.NestedSlice(obj.Object, "spec", "syncWindows")
	if err != nil || !found || len(rawWindows) == 0 {
		return nil
	}
	out := make([]beforeWriteSyncWindow, 0, len(rawWindows))
	for _, raw := range rawWindows {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		w := beforeWriteSyncWindow{
			Kind:       strings.TrimSpace(stringField(m, "kind")),
			Schedule:   strings.TrimSpace(stringField(m, "schedule")),
			Duration:   strings.TrimSpace(stringField(m, "duration")),
			TimeZone:   strings.TrimSpace(stringField(m, "timeZone")),
			ManualSync: boolField(m, "manualSync"),
		}
		if apps := stringSliceField(m, "applications"); len(apps) > 0 {
			w.Applications = apps
		}
		if nss := stringSliceField(m, "namespaces"); len(nss) > 0 {
			w.Namespaces = nss
		}
		if clusters := stringSliceField(m, "clusters"); len(clusters) > 0 {
			w.Clusters = clusters
		}
		if w.Kind != "" || w.Schedule != "" || w.Duration != "" {
			out = append(out, w)
		}
	}
	return out
}

func stringField(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

func boolField(m map[string]interface{}, key string) bool {
	v, _ := m[key].(bool)
	return v
}

func stringSliceField(m map[string]interface{}, key string) []string {
	raw, ok := m[key].([]interface{})
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func extractPodTemplateLabels(obj *unstructured.Unstructured) map[string]string {
	if obj == nil {
		return nil
	}
	switch obj.GetKind() {
	case "Pod":
		return obj.GetLabels()
	case "CronJob":
		l, found, err := unstructured.NestedStringMap(obj.Object, "spec", "jobTemplate", "spec", "template", "metadata", "labels")
		if err == nil && found && len(l) > 0 {
			return l
		}
	case "Rollout":
		if l, found, err := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels"); err == nil && found && len(l) > 0 {
			return l
		}
		if l, found, err := unstructured.NestedStringMap(obj.Object, "spec", "selector", "matchLabels"); err == nil && found && len(l) > 0 {
			return l
		}
	default:
		l, found, err := unstructured.NestedStringMap(obj.Object, "spec", "template", "metadata", "labels")
		if err == nil && found && len(l) > 0 {
			return l
		}
	}
	return nil
}

func matchPDBsForLabels(pdbs []unstructured.Unstructured, podLabels map[string]string) []beforeWritePDB {
	if len(pdbs) == 0 || len(podLabels) == 0 {
		return nil
	}
	labelSet := labels.Set(podLabels)
	var matched []beforeWritePDB
	for _, pdb := range pdbs {
		rawSel, found, err := unstructured.NestedMap(pdb.Object, "spec", "selector")
		if err != nil || !found || rawSel == nil {
			continue
		}
		var sel metav1.LabelSelector
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(rawSel, &sel); err != nil {
			continue
		}
		selector, err := metav1.LabelSelectorAsSelector(&sel)
		if err != nil || !selector.Matches(labelSet) {
			continue
		}
		entry := beforeWritePDB{
			Name:           pdb.GetName(),
			MinAvailable:   formatIntOrStringField(pdb.Object, "spec", "minAvailable"),
			MaxUnavailable: formatIntOrStringField(pdb.Object, "spec", "maxUnavailable"),
		}
		if v, ok, _ := unstructured.NestedInt64(pdb.Object, "status", "disruptionsAllowed"); ok {
			entry.DisruptionsAllowed = &v
		}
		if v, ok, _ := unstructured.NestedInt64(pdb.Object, "status", "currentHealthy"); ok {
			entry.CurrentHealthy = &v
		}
		if v, ok, _ := unstructured.NestedInt64(pdb.Object, "status", "desiredHealthy"); ok {
			entry.DesiredHealthy = &v
		}
		matched = append(matched, entry)
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].Name < matched[j].Name
	})
	return matched
}

func formatIntOrStringField(obj map[string]interface{}, fields ...string) string {
	val, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if err != nil || !found || val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	default:
		return fmt.Sprint(v)
	}
}

type beforeWriteAnswerFacts struct {
	objectRead        bool
	readError         string
	owner             agent.Ownership
	deployerAttempted bool
	deployerRead      bool
	deployerError     string
	pdbError          string
	blastError        string
	recorded          *recordedObjectSnapshot
	observedAt        time.Time
	cost              *agent.KubernetesReadCost
}

// buildBeforeWriteAnswer constructs the common answer envelope using only the
// approved explain state and reason vocabularies (zero new answer.state words),
// staying strictly under 1 KB.
func buildBeforeWriteAnswer(out beforeWriteOutput, f beforeWriteAnswerFacts) *mapsvc.Answer {
	observedAt := f.observedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	coverage := "not-measured"
	if f.recorded != nil {
		coverage = "offline-recording"
	}
	answer := &mapsvc.Answer{
		Cluster:    mapsvc.AnswerCluster{Context: out.Context, IDSource: "not-read"},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
		Cost:       mapsvc.AnswerCost{Coverage: coverage},
	}
	if f.cost != nil {
		answer.Cost = mapsvc.AnswerCostFrom(*f.cost)
	}

	label, named := out.Target.Kind+"/"+out.Target.Name, traceSafeKind.MatchString(out.Target.Kind) && traceSafeName.MatchString(out.Target.Name)
	if !named {
		label = "the resource"
	}
	scope := ""
	if out.Target.Namespace != "" && traceSafeName.MatchString(out.Target.Namespace) {
		scope = " -n " + out.Target.Namespace
	}

	if !f.objectRead {
		answer.State = explainStateNotObservable
		answer.Reason = explainReasonObjectNotRead
		answer.Summary = fmt.Sprintf("%s could not be read; pre-write checks are unavailable.", label)
		unknown := mapsvc.AnswerUnknown{
			What:   "target resource",
			Kind:   unknownKindForError(f.readError),
			Reason: "the read failed; error is quoted",
			Quoted: truncateBeforeWriteQuoted(f.readError, 140),
		}
		answer.Unknowns = append(answer.Unknowns, unknown)
		if f.recorded != nil {
			answer.Unknowns = append(answer.Unknowns, offlineRecordingUnknown(*f.recorded))
		}
		return answer
	}

	if !explainOwnerRecognised(f.owner.Type) {
		answer.State = explainStateNoOwnerMarker
		answer.Reason = agent.TraceReasonNoOwnerMarker
		answer.Summary = fmt.Sprintf("No GitOps owner marker recognised on %s; pre-write facts are object-local.", label)
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim:   label + " carries no recognised owner marker",
			RestsOn: mapsvc.RestsOnObjectMetadata,
		})
	} else {
		owner := mapsvc.DisplayOwner(f.owner.Type)
		ownerBasis := mapsvc.AnswerBasis{
			Claim:   fmt.Sprintf("%s carries %s owner marker", label, ownerWord(owner)),
			RestsOn: mapsvc.RestsOnObjectMetadata,
		}
		answer.Basis = append(answer.Basis, ownerBasis)

		switch {
		case f.deployerRead && out.Deployer != nil:
			answer.State = explainStateExplained
			answer.Reason = explainReasonChainRead
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
				Claim:   summarizeBeforeWriteDeployerClaim(out.Deployer),
				RestsOn: mapsvc.RestsOnDeployerReport,
			})
			answer.Summary = summarizeBeforeWriteExplained(label, ownerWord(owner), out)
		case f.deployerAttempted:
			answer.State = explainStateOwnerOnly
			answer.Reason = explainReasonChainNotObserved
			answer.Summary = fmt.Sprintf("%s is owned by %s, but its deployer spec could not be read.", label, ownerWord(owner))
			answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
				What:   "owning deployer settings",
				Kind:   unknownKindForError(f.deployerError),
				Reason: "deployer read failed",
				Quoted: truncateBeforeWriteQuoted(f.deployerError, 100),
			})
		default:
			answer.State = explainStateOwnerOnly
			answer.Reason = explainReasonChainNotRequested
			answer.Summary = fmt.Sprintf("%s is owned by %s; controller revert settings are not modelled for this owner.", label, ownerWord(owner))
		}
	}

	if len(out.PDBs) > 0 {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim:   fmt.Sprintf("%d matching PodDisruptionBudget(s) in namespace", len(out.PDBs)),
			RestsOn: mapsvc.RestsOnObjectMetadata,
		})
	}
	if f.pdbError != "" {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   "PodDisruptionBudgets",
			Kind:   unknownKindForError(f.pdbError),
			Reason: "PDB list failed",
			Quoted: truncateBeforeWriteQuoted(f.pdbError, 100),
		})
	}
	if f.blastError != "" {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   "blast radius",
			Kind:   unknownKindForError(f.blastError),
			Reason: "blast-radius read failed",
			Quoted: truncateBeforeWriteQuoted(f.blastError, 100),
		})
	}
	if f.recorded != nil {
		answer.Unknowns = append(answer.Unknowns, offlineRecordingUnknown(*f.recorded))
	} else if named {
		answer.Next = append(answer.Next, mapsvc.AnswerNext{
			Command: preferInvocationForm("cub-scout explain " + label + scope + " --format json"),
			Why:     "full lineage, events, and rollout state",
		})
	}
	return answer
}

func unknownKindForError(msg string) string {
	if strings.Contains(strings.ToLower(msg), "forbidden") {
		return mapsvc.UnknownDenied
	}
	return mapsvc.UnknownNotObservable
}

func truncateBeforeWriteQuoted(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func summarizeBeforeWriteDeployerClaim(d *beforeWriteDeployerSettings) string {
	if d == nil {
		return "deployer spec read"
	}
	var parts []string
	if d.Suspended != nil {
		if *d.Suspended {
			parts = append(parts, "suspend=true")
		} else {
			parts = append(parts, "suspend=false")
		}
	}
	if d.AutoSync != nil {
		if *d.AutoSync {
			parts = append(parts, "autoSync=true")
		} else {
			parts = append(parts, "autoSync=false")
		}
	}
	if d.SelfHeal != nil {
		if *d.SelfHeal {
			parts = append(parts, "selfHeal=true")
		} else {
			parts = append(parts, "selfHeal=false")
		}
	}
	if len(d.SyncWindows) > 0 {
		parts = append(parts, fmt.Sprintf("syncWindows=%d", len(d.SyncWindows)))
	}
	if len(parts) == 0 {
		return d.Kind + " spec read"
	}
	return d.Kind + " spec: " + strings.Join(parts, ", ")
}

func summarizeBeforeWriteExplained(label, owner string, out beforeWriteOutput) string {
	parts := []string{fmt.Sprintf("%s owned by %s", label, owner)}
	if out.FieldAttribution != nil && out.FieldAttribution.RevertPrediction != nil {
		parts = append(parts, "revert="+out.FieldAttribution.RevertPrediction.Verdict)
	}
	if out.FieldAttribution != nil && out.FieldAttribution.Contention != nil && out.FieldAttribution.Contention.Contested {
		parts = append(parts, "field-contested")
	}
	if out.Deployer != nil && len(out.Deployer.SyncWindows) > 0 {
		parts = append(parts, fmt.Sprintf("%d syncWindow(s)", len(out.Deployer.SyncWindows)))
	}
	if len(out.PDBs) > 0 {
		parts = append(parts, fmt.Sprintf("%d PDB(s)", len(out.PDBs)))
	}
	return strings.Join(parts, "; ") + "."
}

func renderBeforeWriteOutput(w io.Writer, out beforeWriteOutput, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	case "md":
		_, err := io.WriteString(w, renderBeforeWriteMarkdown(out))
		return err
	default:
		_, err := io.WriteString(w, renderBeforeWriteASCII(out))
		return err
	}
}

func renderBeforeWriteASCII(out beforeWriteOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Before-Write: %s/%s (namespace: %s)\n", out.Target.Kind, out.Target.Name, out.Target.Namespace)
	fmt.Fprintf(&b, "Owner: %s", out.Owner.Display)
	if out.Owner.Name != "" {
		fmt.Fprintf(&b, " (%s)", out.Owner.Name)
	}
	b.WriteByte('\n')
	if out.Deployer != nil {
		fmt.Fprintf(&b, "Deployer: %s (%s)\n", out.Deployer.Ref, summarizeBeforeWriteDeployerClaim(out.Deployer))
		for _, w := range out.Deployer.SyncWindows {
			fmt.Fprintf(&b, "  SyncWindow: kind=%s schedule=%q duration=%s\n", w.Kind, w.Schedule, w.Duration)
		}
	}
	if out.FieldAttribution != nil {
		fa := out.FieldAttribution
		fmt.Fprintf(&b, "Field: %s (cause=%s", fa.Path, fa.Cause)
		if fa.ValueFound {
			fmt.Fprintf(&b, ", value=%v", fa.Value)
		}
		b.WriteString(")\n")
		if len(fa.Managers) > 0 {
			fmt.Fprintf(&b, "  Managers: %s\n", strings.Join(fa.Managers, ", "))
		}
		if fa.RevertPrediction != nil {
			fmt.Fprintf(&b, "  Revert: %s (%s)\n", fa.RevertPrediction.Verdict, fa.RevertPrediction.Reason)
		}
		if fa.Contention != nil && fa.Contention.Contested {
			fmt.Fprintf(&b, "  Contention: contested (%s)\n", fa.Contention.Reason)
		}
	}
	if len(out.PDBs) > 0 {
		b.WriteString("Matching PDBs:\n")
		for _, p := range out.PDBs {
			fmt.Fprintf(&b, "  - %s", p.Name)
			if p.MinAvailable != "" {
				fmt.Fprintf(&b, " minAvailable=%s", p.MinAvailable)
			}
			if p.MaxUnavailable != "" {
				fmt.Fprintf(&b, " maxUnavailable=%s", p.MaxUnavailable)
			}
			if p.DisruptionsAllowed != nil {
				fmt.Fprintf(&b, " disruptionsAllowed=%d", *p.DisruptionsAllowed)
			}
			b.WriteByte('\n')
		}
	}
	if out.BlastRadius != nil {
		fmt.Fprintf(&b, "Blast Radius: %d affected workload(s)", out.BlastRadius.AffectedWorkloadsCount)
		if out.BlastRadius.Truncated {
			b.WriteString(" (truncated)")
		}
		b.WriteByte('\n')
		for _, wl := range out.BlastRadius.AffectedWorkloads {
			fmt.Fprintf(&b, "  - %s\n", wl)
		}
	}
	if out.Answer != nil {
		fmt.Fprintf(&b, "State: %s (%s)\n", out.Answer.State, out.Answer.Reason)
		for _, u := range out.Answer.Unknowns {
			fmt.Fprintf(&b, "Unknown [%s]: %s (%s)\n", u.Kind, u.What, u.Reason)
		}
	}
	return b.String()
}

func renderBeforeWriteMarkdown(out beforeWriteOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Before-Write: `%s/%s` (`%s`)\n\n", out.Target.Kind, out.Target.Name, out.Target.Namespace)
	fmt.Fprintf(&b, "- **Owner:** %s", out.Owner.Display)
	if out.Owner.Name != "" {
		fmt.Fprintf(&b, " (`%s`)", out.Owner.Name)
	}
	b.WriteByte('\n')
	if out.Deployer != nil {
		fmt.Fprintf(&b, "- **Deployer:** `%s` — %s\n", out.Deployer.Ref, summarizeBeforeWriteDeployerClaim(out.Deployer))
	}
	if out.FieldAttribution != nil {
		fa := out.FieldAttribution
		fmt.Fprintf(&b, "- **Field:** `%s` (cause: `%s`)\n", fa.Path, fa.Cause)
		if fa.RevertPrediction != nil {
			fmt.Fprintf(&b, "- **Revert Prediction:** `%s` (`%s`)\n", fa.RevertPrediction.Verdict, fa.RevertPrediction.Reason)
		}
		if fa.Contention != nil && fa.Contention.Contested {
			fmt.Fprintf(&b, "- **Writer Contention:** contested (%s)\n", fa.Contention.Reason)
		}
	}
	if len(out.PDBs) > 0 {
		names := make([]string, 0, len(out.PDBs))
		for _, p := range out.PDBs {
			names = append(names, "`"+p.Name+"`")
		}
		fmt.Fprintf(&b, "- **Matching PDBs:** %s\n", strings.Join(names, ", "))
	}
	if out.BlastRadius != nil {
		trunc := ""
		if out.BlastRadius.Truncated {
			trunc = " (truncated)"
		}
		fmt.Fprintf(&b, "- **Blast Radius:** %d affected workload(s)%s\n", out.BlastRadius.AffectedWorkloadsCount, trunc)
		for _, wl := range out.BlastRadius.AffectedWorkloads {
			fmt.Fprintf(&b, "  - `%s`\n", wl)
		}
	}
	if out.Answer != nil {
		fmt.Fprintf(&b, "- **State:** `%s` (`%s`)\n", out.Answer.State, out.Answer.Reason)
	}
	return b.String()
}
