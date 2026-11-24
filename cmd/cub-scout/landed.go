// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

var (
	landedNamespace       string
	landedKubeContext     string
	landedRecording       string
	landedRevision        string
	landedDigest          string
	landedUnitRevision    string
	landedVerifyPodImages bool
	landedImage           string
	landedWait            time.Duration
	landedPollInterval    time.Duration
	landedFormat          string
	landedIfUnchanged     string
)

var landedCmd = &cobra.Command{
	Use:   "landed <kind>/<name> | <kind> <name>",
	Short: "Check whether an anchored revision or digest landed on a workload",
	Long: `Check whether a specific Git revision, OCI digest, or ConfigHub unit revision
reached a workload (Deployment, StatefulSet, or DaemonSet) under Flux or Argo CD,
whether delivery is still converging, or which link broke.

Read-only: never modifies cluster state. With --wait, performs a bounded
read-only poll loop (capped at 5m on CLI, 60s via MCP) that exits early as
soon as the state settles.

Examples:
  # Single-shot check against a Git SHA
  cub scout landed deploy/api -n prod --revision a1b2c3d --format json

  # Bounded wait for an OCI digest to converge
  cub scout landed deploy/api -n prod --digest sha256:0828... --wait 60s --format json

  # Standalone binary form
  cub-scout landed deploy/api -n prod --revision a1b2c3d --kube-context prod-east`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runLanded,
}

func init() {
	rootCmd.AddCommand(landedCmd)

	landedCmd.Flags().StringVarP(&landedNamespace, "namespace", "n", "default", "Namespace of the target workload")
	landedCmd.Flags().StringVar(&landedKubeContext, "kube-context", "", "Kubernetes context to inspect (default: current context)")
	landedCmd.Flags().StringVar(&landedRecording, "recording", "", "Offline YAML/JSON cluster snapshot file to evaluate without cluster access")
	landedCmd.Flags().StringVar(&landedRevision, "revision", "", "Expected Git SHA or controller revision")
	landedCmd.Flags().StringVar(&landedDigest, "digest", "", "Expected OCI digest (e.g. sha256:...)")
	landedCmd.Flags().StringVar(&landedUnitRevision, "unit-revision", "", "Expected ConfigHub unit revision")
	landedCmd.Flags().BoolVar(&landedVerifyPodImages, "verify-pod-images", false, "Independently verify running Pod container imageIDs match the expected image or digest")
	landedCmd.Flags().StringVar(&landedImage, "image", "", "Expected running Pod image reference or sha256 digest (requires --verify-pod-images)")
	landedCmd.Flags().DurationVar(&landedWait, "wait", 0, "Bounded read-only wait duration until settled (default: 0s, max: 5m)")
	landedCmd.Flags().DurationVar(&landedPollInterval, "poll-interval", agent.DefaultLandingPollInterval, "Interval between bounded-wait polls (default: 5s, min: 2s)")
	landedCmd.Flags().StringVar(&landedFormat, "format", "ascii", "Output format: ascii, json, md")
	landedCmd.Flags().StringVar(&landedIfUnchanged, "if-unchanged", "", "JSON only: return a compact unchanged envelope when the answer fingerprint matches this hash")
}

// landedOutput is the JSON document emitted by `cub scout landed --format json`
// and the `landed` MCP tool.
type landedOutput struct {
	Context     string                    `json:"context,omitempty"`
	Target      agent.LandingObjectRef    `json:"target"`
	Owner       string                    `json:"owner,omitempty"`
	Anchor      agent.LandingAnchor       `json:"anchor"`
	BrokenLink  *agent.LandingBrokenLink  `json:"brokenLink,omitempty"`
	Convergence *agent.LandingConvergence `json:"convergence,omitempty"`
	Wait        *agent.LandingWait        `json:"wait,omitempty"`
	Fingerprint string                    `json:"fingerprint,omitempty"`
	Answer      *mapsvc.Answer            `json:"answer"`
}

var landedStates = []string{
	agent.LandingStateLanded,
	agent.LandingStateConverging,
	agent.LandingStateNotLanded,
	agent.LandingStateNotObservable,
}

var landedReasons = []string{
	agent.LandingReasonRevisionAndWorkloadConverged,
	agent.LandingReasonDigestAndWorkloadConverged,
	agent.LandingReasonSourceFetching,
	agent.LandingReasonControllerReconciling,
	agent.LandingReasonControllerOnPriorRevision,
	agent.LandingReasonWorkloadRollingOut,
	agent.LandingReasonSourceError,
	agent.LandingReasonControllerReconcileFailed,
	agent.LandingReasonControllerSettledOtherRevision,
	agent.LandingReasonWorkloadFailed,
	agent.LandingReasonNoAnchorSupplied,
	agent.LandingReasonTargetNotFound,
	agent.LandingReasonNoOwnerMarkerRecognised,
	agent.LandingReasonUnsupportedController,
	agent.LandingReasonReadDenied,
	agent.LandingReasonClusterUnreachable,
}

func resolveLandingAnchor(revision, digest, unitRevision string) (agent.LandingAnchor, error) {
	rev := strings.TrimSpace(revision)
	dig := strings.TrimSpace(digest)
	unit := strings.TrimSpace(unitRevision)
	count := 0
	if rev != "" {
		count++
	}
	if dig != "" {
		count++
	}
	if unit != "" {
		count++
	}
	if count > 1 {
		return agent.LandingAnchor{}, fmt.Errorf("specify only one of --revision, --digest, or --unit-revision")
	}
	switch {
	case rev != "":
		return agent.LandingAnchor{Kind: agent.AnchorKindGitSHA, Value: rev}, nil
	case dig != "":
		return agent.LandingAnchor{Kind: agent.AnchorKindOCIDigest, Value: dig}, nil
	case unit != "":
		return agent.LandingAnchor{Kind: agent.AnchorKindUnitRevision, Value: unit}, nil
	default:
		return agent.LandingAnchor{}, nil
	}
}

func parseLandedTarget(args []string, ns string) (agent.LandingObjectRef, error) {
	var rawKind, name string
	if len(args) == 1 {
		parts := strings.SplitN(args[0], "/", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return agent.LandingObjectRef{}, fmt.Errorf("target must be <kind>/<name> (e.g. deploy/api), got %q", args[0])
		}
		rawKind, name = parts[0], parts[1]
	} else {
		rawKind, name = args[0], args[1]
	}
	kind := canonicalWorkloadKind(rawKind)
	if kind == "" {
		return agent.LandingObjectRef{}, fmt.Errorf("unsupported workload kind %q (supported in 3.0: Deployment, StatefulSet, DaemonSet)", rawKind)
	}
	if strings.TrimSpace(ns) == "" {
		ns = "default"
	}
	return agent.LandingObjectRef{
		APIVersion: "apps/v1",
		Kind:       kind,
		Namespace:  ns,
		Name:       strings.TrimSpace(name),
	}, nil
}

func canonicalWorkloadKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "deploy", "deployment", "deployments":
		return "Deployment"
	case "sts", "statefulset", "statefulsets":
		return "StatefulSet"
	case "ds", "daemonset", "daemonsets":
		return "DaemonSet"
	default:
		return ""
	}
}

func workloadGVR(kind string) schema.GroupVersionResource {
	switch kind {
	case "StatefulSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	case "DaemonSet":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	default:
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	}
}

var newLandedSessionForSelection = newTraceSessionForSelection

func runLanded(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(landedFormat))
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("unsupported format %q (supported: ascii, json, md)", landedFormat)
	}
	if strings.TrimSpace(landedIfUnchanged) != "" && format != "json" {
		return fmt.Errorf("--if-unchanged applies to JSON output; pass --format json")
	}

	target, err := parseLandedTarget(args, landedNamespace)
	if err != nil {
		return err
	}
	anchor, err := resolveLandingAnchor(landedRevision, landedDigest, landedUnitRevision)
	if err != nil {
		return err
	}
	if strings.TrimSpace(landedImage) != "" && !landedVerifyPodImages {
		return fmt.Errorf("--image requires --verify-pod-images")
	}
	podVerifyOpts := landingPodVerifyOptions{
		Verify:        landedVerifyPodImages,
		ExpectedImage: resolveExpectedPodImage(landedImage, anchor),
	}

	selection, err := clusterContextSelectionFromFlag(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Offline recorded snapshot mode (Theme 4: zero cluster/network access).
	if strings.TrimSpace(landedRecording) != "" {
		if selection.explicit {
			return fmt.Errorf("--recording cannot be combined with --kube-context")
		}
		if landedWait > 0 {
			return fmt.Errorf("--recording cannot be combined with --wait (recorded snapshots are static)")
		}
		snapshot, err := readRecordedObjectSnapshot(landedRecording)
		if err != nil {
			return err
		}
		if anchor.Value == "" {
			snap := agent.LandingSnapshot{Target: target, ObservedAt: time.Now().UTC()}
			eval := agent.EvaluateLanding(anchor, snap)
			out := buildLandedOutput(eval, nil)
			return renderLandedOutputWithDelta(os.Stdout, out, format, landedIfUnchanged)
		}
		store := recordedLandingStore{objects: snapshot.Objects}
		snap := fetchLandingSnapshotFromStoreWithOptions(ctx, store, "", target, podVerifyOpts)
		eval := agent.EvaluateLanding(anchor, snap)
		out := buildLandedOutput(eval, nil)
		return renderLandedOutputWithDelta(os.Stdout, out, format, landedIfUnchanged)
	}

	// If no anchor was supplied, return not-observable without touching the cluster.
	if anchor.Value == "" {
		snap := agent.LandingSnapshot{Context: selection.name, Target: target, ObservedAt: time.Now().UTC()}
		eval := agent.EvaluateLanding(anchor, snap)
		out := buildLandedOutput(eval, nil)
		return renderLandedOutputWithDelta(os.Stdout, out, format, landedIfUnchanged)
	}

	session, sessErr := newLandedSessionForSelection(selection)
	if sessErr != nil {
		snap := agent.LandingSnapshot{
			Context:     selection.name,
			Target:      target,
			Unreachable: true,
			ReadError:   sessErr.Error(),
			ObservedAt:  time.Now().UTC(),
		}
		eval := agent.EvaluateLanding(anchor, snap)
		out := buildLandedOutput(eval, nil)
		return renderLandedOutputWithDelta(os.Stdout, out, format, landedIfUnchanged)
	}

	dynClient, err := session.dynamicClient()
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	eval, err := agent.WaitForLanding(ctx, anchor, agent.LandingWaitOptions{
		Wait:         landedWait,
		PollInterval: landedPollInterval,
		MaxWait:      agent.MaxLandingWaitCLI,
	}, func(pollCtx context.Context) (agent.LandingSnapshot, error) {
		return fetchLandingSnapshotWithOptions(pollCtx, dynClient, session.contextLabel(), target, podVerifyOpts), nil
	})
	if err != nil {
		return err
	}

	var cost *agent.KubernetesReadCost
	if session.meter != nil {
		measured := session.meter.Snapshot()
		measured.Coverage = "landed-session-client"
		cost = &measured
	}
	out := buildLandedOutput(eval, cost)
	return renderLandedOutputWithDelta(os.Stdout, out, format, landedIfUnchanged)
}

type landingPodVerifyOptions struct {
	Verify        bool
	ExpectedImage string
}

func resolveExpectedPodImage(explicitImage string, anchor agent.LandingAnchor) string {
	if img := strings.TrimSpace(explicitImage); img != "" {
		return img
	}
	if anchor.Kind == agent.AnchorKindOCIDigest {
		return strings.TrimSpace(anchor.Value)
	}
	return ""
}

type landingObjectStore interface {
	Get(ctx context.Context, gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error)
	List(ctx context.Context, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, error)
	ListNamespace(ctx context.Context, gvr schema.GroupVersionResource, ns, labelSelector string) ([]unstructured.Unstructured, error)
}

type dynamicLandingStore struct {
	dyn dynamic.Interface
}

func (s dynamicLandingStore) Get(ctx context.Context, gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
	return s.dyn.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
}

func (s dynamicLandingStore) List(ctx context.Context, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, error) {
	list, err := s.dyn.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func (s dynamicLandingStore) ListNamespace(ctx context.Context, gvr schema.GroupVersionResource, ns, labelSelector string) ([]unstructured.Unstructured, error) {
	list, err := s.dyn.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

type recordedLandingStore struct {
	objects []*unstructured.Unstructured
}

func gvrMatchesRecordedObject(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	gv, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil {
		return false
	}
	if gv.Group != gvr.Group {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(obj.GetKind()))
	switch gvr.Resource {
	case "deployments":
		return kind == "deployment"
	case "statefulsets":
		return kind == "statefulset"
	case "daemonsets":
		return kind == "daemonset"
	case "kustomizations":
		return kind == "kustomization"
	case "helmreleases":
		return kind == "helmrelease"
	case "gitrepositories":
		return kind == "gitrepository"
	case "ocirepositories":
		return kind == "ocirepository"
	case "applications":
		return kind == "application"
	case "secrets":
		return kind == "secret"
	case "pods":
		return kind == "pod"
	default:
		return false
	}
}

func (s recordedLandingStore) Get(_ context.Context, gvr schema.GroupVersionResource, ns, name string) (*unstructured.Unstructured, error) {
	for _, obj := range s.objects {
		if !gvrMatchesRecordedObject(gvr, obj) {
			continue
		}
		if obj.GetName() == name && (ns == "" || obj.GetNamespace() == ns) {
			return obj, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: gvr.Group, Resource: gvr.Resource}, name)
}

func (s recordedLandingStore) List(_ context.Context, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, error) {
	var out []unstructured.Unstructured
	for _, obj := range s.objects {
		if gvrMatchesRecordedObject(gvr, obj) {
			out = append(out, *obj)
		}
	}
	return out, nil
}

func (s recordedLandingStore) ListNamespace(_ context.Context, gvr schema.GroupVersionResource, ns, labelSelector string) ([]unstructured.Unstructured, error) {
	var sel labels.Selector
	if strings.TrimSpace(labelSelector) != "" {
		parsed, err := labels.Parse(labelSelector)
		if err != nil {
			return nil, err
		}
		sel = parsed
	}
	var out []unstructured.Unstructured
	for _, obj := range s.objects {
		if !gvrMatchesRecordedObject(gvr, obj) {
			continue
		}
		if ns != "" && obj.GetNamespace() != ns {
			continue
		}
		if sel != nil && !sel.Matches(labels.Set(obj.GetLabels())) {
			continue
		}
		out = append(out, *obj)
	}
	return out, nil
}

func fetchLandingSnapshot(ctx context.Context, dyn dynamic.Interface, kubeCtx string, target agent.LandingObjectRef) agent.LandingSnapshot {
	return fetchLandingSnapshotWithOptions(ctx, dyn, kubeCtx, target, landingPodVerifyOptions{})
}

func fetchLandingSnapshotWithOptions(ctx context.Context, dyn dynamic.Interface, kubeCtx string, target agent.LandingObjectRef, opts landingPodVerifyOptions) agent.LandingSnapshot {
	return fetchLandingSnapshotFromStoreWithOptions(ctx, dynamicLandingStore{dyn: dyn}, kubeCtx, target, opts)
}

func fetchLandingSnapshotFromStore(ctx context.Context, store landingObjectStore, kubeCtx string, target agent.LandingObjectRef) agent.LandingSnapshot {
	return fetchLandingSnapshotFromStoreWithOptions(ctx, store, kubeCtx, target, landingPodVerifyOptions{})
}

func fetchLandingSnapshotFromStoreWithOptions(ctx context.Context, store landingObjectStore, kubeCtx string, target agent.LandingObjectRef, opts landingPodVerifyOptions) agent.LandingSnapshot {
	snap := agent.LandingSnapshot{
		Context:    kubeCtx,
		Target:     target,
		ObservedAt: time.Now().UTC(),
	}
	obj, err := store.Get(ctx, workloadGVR(target.Kind), target.Namespace, target.Name)
	if err != nil {
		switch {
		case apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err):
			snap.ReadDenied = true
			snap.ReadError = err.Error()
		case apierrors.IsNotFound(err):
			snap.TargetFound = false
		default:
			snap.Unreachable = true
			snap.ReadError = err.Error()
		}
		return snap
	}

	snap.TargetFound = true
	snap.Workload = observeWorkloadLink(obj, target)

	own := agent.DetectOwnership(obj)
	snap.Owner = own.Type
	objLabels := obj.GetLabels()
	annotations := obj.GetAnnotations()

	switch strings.ToLower(own.Type) {
	case "flux":
		snap.Deployer, snap.Source, snap.ReadDenied, snap.ReadError = fetchFluxLandingLinks(ctx, store, objLabels)
	case "argocd", "argo":
		snap.Deployer, snap.ReadDenied, snap.ReadError = fetchArgoLandingLink(ctx, store, objLabels, annotations)
	case "helm":
		snap.Deployer, snap.ReadDenied, snap.ReadError = fetchHelmLandingLink(ctx, store, target.Namespace, annotations)
	}

	if opts.Verify && !snap.ReadDenied {
		podLink, denied, readErr := observePodImagesWitness(ctx, store, obj, target, opts.ExpectedImage)
		if denied {
			snap.ReadDenied = true
			snap.ReadError = readErr
		} else {
			snap.PodImages = podLink
		}
	}

	return snap
}

func workloadLabelSelector(obj *unstructured.Unstructured) (string, error) {
	selMap, found, err := unstructured.NestedMap(obj.Object, "spec", "selector")
	if err != nil || !found || len(selMap) == 0 {
		return "", err
	}
	ls := &metav1.LabelSelector{}
	if ml, ok, _ := unstructured.NestedStringMap(selMap, "matchLabels"); ok && len(ml) > 0 {
		ls.MatchLabels = ml
	}
	if rawExprs, ok, _ := unstructured.NestedSlice(selMap, "matchExpressions"); ok && len(rawExprs) > 0 {
		for _, raw := range rawExprs {
			m, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			key, _ := m["key"].(string)
			op, _ := m["operator"].(string)
			var vals []string
			if rawVals, ok := m["values"].([]interface{}); ok {
				for _, rv := range rawVals {
					if s, ok := rv.(string); ok {
						vals = append(vals, s)
					}
				}
			}
			ls.MatchExpressions = append(ls.MatchExpressions, metav1.LabelSelectorRequirement{
				Key:      key,
				Operator: metav1.LabelSelectorOperator(op),
				Values:   vals,
			})
		}
	}
	if len(ls.MatchLabels) == 0 && len(ls.MatchExpressions) == 0 {
		return "", nil
	}
	sel, err := metav1.LabelSelectorAsSelector(ls)
	if err != nil {
		return "", err
	}
	return sel.String(), nil
}

func workloadTemplateContainerImages(obj *unstructured.Unstructured) map[string]string {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return nil
	}
	out := make(map[string]string, len(containers))
	for _, raw := range containers {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		img, _ := m["image"].(string)
		if strings.TrimSpace(name) != "" && strings.TrimSpace(img) != "" {
			out[name] = strings.TrimSpace(img)
		}
	}
	return out
}

func observePodImagesWitness(
	ctx context.Context,
	store landingObjectStore,
	workloadObj *unstructured.Unstructured,
	target agent.LandingObjectRef,
	expectedImage string,
) (link *agent.LandingLinkObservation, denied bool, readErr string) {
	selStr, err := workloadLabelSelector(workloadObj)
	if err != nil || selStr == "" {
		msg := "workload spec.selector is absent or invalid"
		if err != nil {
			msg = err.Error()
		}
		return &agent.LandingLinkObservation{
			Present:  true,
			Ref:      &target,
			Status:   "unknown",
			Message:  msg,
			Reporter: mapsvc.RestsOnCubScoutComparison,
		}, false, ""
	}

	podGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	pods, err := store.ListNamespace(ctx, podGVR, target.Namespace, selStr)
	if err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			return nil, true, err.Error()
		}
		return &agent.LandingLinkObservation{
			Present:  true,
			Ref:      &target,
			Status:   "unknown",
			Message:  err.Error(),
			Reporter: mapsvc.RestsOnCubScoutComparison,
		}, false, err.Error()
	}

	var running []unstructured.Unstructured
	for i := range pods {
		p := pods[i]
		if p.GetDeletionTimestamp() != nil {
			continue
		}
		phase, _, _ := unstructured.NestedString(p.Object, "status", "phase")
		if phase == "Running" {
			running = append(running, p)
		}
	}
	sort.Slice(running, func(i, j int) bool {
		return running[i].GetName() < running[j].GetName()
	})

	if len(running) == 0 {
		return &agent.LandingLinkObservation{
			Present:  true,
			Ref:      &target,
			Status:   "failed",
			Message:  fmt.Sprintf("no running Pods matched selector %q in namespace %s", selStr, target.Namespace),
			Reporter: mapsvc.RestsOnCubScoutComparison,
		}, false, ""
	}

	expected := strings.TrimSpace(expectedImage)
	if expected != "" {
		for i := range running {
			pod := &running[i]
			statuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
			matched := false
			var observed []string
			for _, raw := range statuses {
				m, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				imgID, _ := m["imageID"].(string)
				img, _ := m["image"].(string)
				if agent.PodImageMatches(expected, imgID, img) {
					matched = true
					break
				}
				if imgID != "" {
					observed = append(observed, imgID)
				} else if img != "" {
					observed = append(observed, img)
				}
			}
			if !matched {
				podRef := &agent.LandingObjectRef{
					APIVersion: "v1",
					Kind:       "Pod",
					Namespace:  pod.GetNamespace(),
					Name:       pod.GetName(),
				}
				obs := "none"
				if len(observed) > 0 {
					obs = strings.Join(observed, ",")
				}
				return &agent.LandingLinkObservation{
					Present:  true,
					Ref:      podRef,
					Status:   "failed",
					Message:  fmt.Sprintf("running pod %s imageID/image (%s) did not match expected %s", pod.GetName(), obs, expected),
					Reporter: mapsvc.RestsOnCubScoutComparison,
				}, false, ""
			}
		}
		return &agent.LandingLinkObservation{
			Present:  true,
			Ref:      &target,
			Status:   "ready",
			Message:  fmt.Sprintf("%d/%d running pods matched expected image", len(running), len(running)),
			Reporter: mapsvc.RestsOnCubScoutComparison,
		}, false, ""
	}

	// No explicit --image or --digest: compare running Pods against workload spec template container images.
	specImages := workloadTemplateContainerImages(workloadObj)
	if len(specImages) == 0 {
		return &agent.LandingLinkObservation{
			Present:  true,
			Ref:      &target,
			Status:   "unknown",
			Message:  "no expected image supplied and workload spec.template.spec.containers is empty",
			Reporter: mapsvc.RestsOnCubScoutComparison,
		}, false, ""
	}
	specNames := make([]string, 0, len(specImages))
	for name := range specImages {
		specNames = append(specNames, name)
	}
	sort.Strings(specNames)

	for i := range running {
		pod := &running[i]
		statuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
		byName := make(map[string]map[string]interface{}, len(statuses))
		for _, raw := range statuses {
			if m, ok := raw.(map[string]interface{}); ok {
				if cname, _ := m["name"].(string); cname != "" {
					byName[cname] = m
				}
			}
		}
		for _, cname := range specNames {
			wantImg := specImages[cname]
			cs, ok := byName[cname]
			var imgID, img string
			if ok {
				imgID, _ = cs["imageID"].(string)
				img, _ = cs["image"].(string)
			}
			if !ok || !agent.PodImageMatches(wantImg, imgID, img) {
				podRef := &agent.LandingObjectRef{
					APIVersion: "v1",
					Kind:       "Pod",
					Namespace:  pod.GetNamespace(),
					Name:       pod.GetName(),
				}
				return &agent.LandingLinkObservation{
					Present:  true,
					Ref:      podRef,
					Status:   "failed",
					Message:  fmt.Sprintf("running pod %s container %s (imageID=%s image=%s) did not match workload spec image %s", pod.GetName(), cname, imgID, img, wantImg),
					Reporter: mapsvc.RestsOnCubScoutComparison,
				}, false, ""
			}
		}
	}

	return &agent.LandingLinkObservation{
		Present:  true,
		Ref:      &target,
		Status:   "ready",
		Message:  fmt.Sprintf("%d/%d running pods matched workload spec images", len(running), len(running)),
		Reporter: mapsvc.RestsOnCubScoutComparison,
	}, false, ""
}

func observeWorkloadLink(obj *unstructured.Unstructured, ref agent.LandingObjectRef) *agent.LandingLinkObservation {
	res, err := status.Compute(obj)
	if err != nil {
		return &agent.LandingLinkObservation{Present: true, Ref: &ref, Status: "unknown", Message: err.Error(), Reporter: mapsvc.RestsOnObjectMetadata}
	}
	link := &agent.LandingLinkObservation{
		Present:  true,
		Ref:      &ref,
		Message:  res.Message,
		Reporter: mapsvc.RestsOnObjectMetadata,
	}
	switch res.Status {
	case status.CurrentStatus:
		link.Status = "ready"
	case status.InProgressStatus:
		link.Status = "progressing"
	case status.FailedStatus:
		link.Status = "failed"
	default:
		link.Status = "unknown"
	}
	return link
}

func fetchFluxLandingLinks(ctx context.Context, store landingObjectStore, labels map[string]string) (deployer, source *agent.LandingLinkObservation, denied bool, readErr string) {
	if ksName := labels["kustomize.toolkit.fluxcd.io/name"]; ksName != "" {
		ksNS := labels["kustomize.toolkit.fluxcd.io/namespace"]
		if ksNS == "" {
			ksNS = "flux-system"
		}
		ksGVR := schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
		ks, err := store.Get(ctx, ksGVR, ksNS, ksName)
		if err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return nil, nil, true, err.Error()
			}
			return nil, nil, false, err.Error()
		}
		ksRef := &agent.LandingObjectRef{APIVersion: ks.GetAPIVersion(), Kind: "Kustomization", Namespace: ksNS, Name: ksName}
		deployer = observeFluxConditionLink(ks, ksRef)
		if rev, _, _ := unstructured.NestedString(ks.Object, "status", "lastAppliedRevision"); rev != "" {
			deployer.Revision = rev
		}

		// Follow spec.sourceRef (GitRepository or OCIRepository).
		srcKind, _, _ := unstructured.NestedString(ks.Object, "spec", "sourceRef", "kind")
		srcName, _, _ := unstructured.NestedString(ks.Object, "spec", "sourceRef", "name")
		srcNS, _, _ := unstructured.NestedString(ks.Object, "spec", "sourceRef", "namespace")
		if srcNS == "" {
			srcNS = ksNS
		}
		if srcName != "" {
			source = fetchFluxSourceObject(ctx, store, srcKind, srcNS, srcName)
		}
		return deployer, source, false, ""
	}
	if hrName := labels["helm.toolkit.fluxcd.io/name"]; hrName != "" {
		hrNS := labels["helm.toolkit.fluxcd.io/namespace"]
		if hrNS == "" {
			hrNS = "flux-system"
		}
		hrGVR := schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
		hr, err := store.Get(ctx, hrGVR, hrNS, hrName)
		if err != nil {
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return nil, nil, true, err.Error()
			}
			return nil, nil, false, err.Error()
		}
		hrRef := &agent.LandingObjectRef{APIVersion: hr.GetAPIVersion(), Kind: "HelmRelease", Namespace: hrNS, Name: hrName}
		deployer = observeFluxConditionLink(hr, hrRef)
		if rev, _, _ := unstructured.NestedString(hr.Object, "status", "lastAppliedRevision"); rev != "" {
			deployer.Revision = rev
		}
		return deployer, nil, false, ""
	}
	return nil, nil, false, ""
}

func fetchFluxSourceObject(ctx context.Context, store landingObjectStore, kind, ns, name string) *agent.LandingLinkObservation {
	var resource string
	switch kind {
	case "GitRepository":
		resource = "gitrepositories"
	case "OCIRepository":
		resource = "ocirepositories"
	default:
		return nil
	}
	gvr := schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: resource}
	obj, err := store.Get(ctx, gvr, ns, name)
	if err != nil {
		return nil
	}
	ref := &agent.LandingObjectRef{APIVersion: obj.GetAPIVersion(), Kind: kind, Namespace: ns, Name: name}
	link := observeFluxConditionLink(obj, ref)
	if rev, _, _ := unstructured.NestedString(obj.Object, "status", "artifact", "revision"); rev != "" {
		link.Revision = rev
	}
	return link
}

func observeFluxConditionLink(obj *unstructured.Unstructured, ref *agent.LandingObjectRef) *agent.LandingLinkObservation {
	link := &agent.LandingLinkObservation{
		Present:  true,
		Ref:      ref,
		Status:   "unknown",
		Reporter: mapsvc.RestsOnControllerReport,
	}
	conds, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	for _, raw := range conds {
		m, ok := raw.(map[string]interface{})
		if !ok || m["type"] != "Ready" {
			continue
		}
		st, _ := m["status"].(string)
		msg, _ := m["message"].(string)
		link.Message = msg
		switch st {
		case "True":
			link.Status = "ready"
		case "False":
			link.Status = "failed"
		case "Unknown":
			link.Status = "progressing"
		}
		return link
	}
	return link
}

func fetchArgoLandingLink(ctx context.Context, store landingObjectStore, labels, annotations map[string]string) (deployer *agent.LandingLinkObservation, denied bool, readErr string) {
	appName := labels["argocd.argoproj.io/instance"]
	if appName == "" {
		if tid := annotations["argocd.argoproj.io/tracking-id"]; tid != "" {
			if idx := strings.IndexByte(tid, ':'); idx > 0 {
				appName = tid[:idx]
			}
		}
	}
	if appName == "" {
		return nil, false, ""
	}
	appGVR := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
	items, err := store.List(ctx, appGVR)
	if err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			return nil, true, err.Error()
		}
		return nil, false, err.Error()
	}
	for i := range items {
		app := &items[i]
		if app.GetName() != appName {
			continue
		}
		ref := &agent.LandingObjectRef{APIVersion: app.GetAPIVersion(), Kind: "Application", Namespace: app.GetNamespace(), Name: app.GetName()}
		link := &agent.LandingLinkObservation{Present: true, Ref: ref, Status: "unknown", Reporter: mapsvc.RestsOnControllerReport}
		syncStatus, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")
		healthStatus, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
		rev, _, _ := unstructured.NestedString(app.Object, "status", "sync", "revision")
		opPhase, _, _ := unstructured.NestedString(app.Object, "status", "operationState", "phase")
		opMsg, _, _ := unstructured.NestedString(app.Object, "status", "operationState", "message")
		link.Revision = rev
		link.Message = opMsg

		switch {
		case opPhase == "Failed" || opPhase == "Error" || healthStatus == "Degraded":
			link.Status = "failed"
		case opPhase == "Running" || healthStatus == "Progressing":
			link.Status = "progressing"
		case syncStatus == "Synced":
			link.Status = "ready"
		case syncStatus == "OutOfSync":
			link.Status = "progressing"
		}
		return link, false, ""
	}
	return nil, false, ""
}

// fetchHelmLandingLink inspects the latest Helm release Secret
// (`sh.helm.release.v1.<release>.v<N>`) in the release namespace named by
// `meta.helm.sh/release-name` and `meta.helm.sh/release-namespace`. It never
// guesses from chart labels alone when `meta.helm.sh/release-name` is absent.
func fetchHelmLandingLink(ctx context.Context, store landingObjectStore, targetNS string, annotations map[string]string) (deployer *agent.LandingLinkObservation, denied bool, readErr string) {
	releaseName := strings.TrimSpace(annotations["meta.helm.sh/release-name"])
	if releaseName == "" {
		return nil, false, ""
	}
	releaseNS := strings.TrimSpace(annotations["meta.helm.sh/release-namespace"])
	if releaseNS == "" {
		releaseNS = targetNS
	}
	secretGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	items, err := store.List(ctx, secretGVR)
	if err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			return nil, true, err.Error()
		}
		return nil, false, err.Error()
	}

	var latest *unstructured.Unstructured
	latestVer := -1
	for i := range items {
		sec := &items[i]
		if releaseNS != "" && sec.GetNamespace() != releaseNS {
			continue
		}
		secLabels := sec.GetLabels()
		if secLabels["owner"] != "helm" || secLabels["name"] != releaseName {
			continue
		}
		ver, err := strconv.Atoi(strings.TrimSpace(secLabels["version"]))
		if err != nil {
			continue
		}
		if ver > latestVer {
			latestVer = ver
			latest = sec
		}
	}
	if latest == nil {
		return nil, false, ""
	}

	secLabels := latest.GetLabels()
	ref := &agent.LandingObjectRef{APIVersion: "v1", Kind: "Secret", Namespace: latest.GetNamespace(), Name: latest.GetName()}
	link := &agent.LandingLinkObservation{
		Present:  true,
		Ref:      ref,
		Status:   "unknown",
		Revision: strings.TrimSpace(secLabels["version"]),
		Reporter: mapsvc.RestsOnControllerReport,
	}
	switch strings.ToLower(strings.TrimSpace(secLabels["status"])) {
	case "deployed":
		link.Status = "ready"
	case "pending-install", "pending-upgrade", "pending-rollback", "uninstalling":
		link.Status = "progressing"
	case "failed":
		link.Status = "failed"
	}
	return link, false, ""
}

func buildLandedOutput(eval agent.LandingEvaluation, cost *agent.KubernetesReadCost) landedOutput {
	ans := buildLandingAnswer(eval, cost)
	return landedOutput{
		Context:     eval.Snapshot.Context,
		Target:      eval.Snapshot.Target,
		Owner:       eval.Snapshot.Owner,
		Anchor:      eval.Anchor,
		BrokenLink:  eval.BrokenLink,
		Convergence: eval.Convergence,
		Wait:        eval.Wait,
		Fingerprint: computeAnswerFingerprint(ans),
		Answer:      ans,
	}
}

func buildLandingAnswer(eval agent.LandingEvaluation, cost *agent.KubernetesReadCost) *mapsvc.Answer {
	observedAt := eval.Snapshot.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	ans := &mapsvc.Answer{
		State:      eval.State,
		Reason:     eval.Reason,
		Summary:    eval.Summary,
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Cluster:    mapsvc.AnswerCluster{Context: eval.Snapshot.Context, IDSource: "not-read"},
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
		Cost:       mapsvc.AnswerCost{Coverage: "landed-session-client"},
	}
	if cost != nil {
		ans.Cost = mapsvc.AnswerCostFrom(*cost)
	}

	addLinkBasis := func(stage string, link *agent.LandingLinkObservation) {
		if link == nil || !link.Present {
			return
		}
		b := mapsvc.AnswerBasis{
			Claim:   fmt.Sprintf("%s reported %s", stage, link.Status),
			RestsOn: link.Reporter,
		}
		if b.RestsOn == "" {
			b.RestsOn = mapsvc.RestsOnControllerReport
		}
		if link.Ref != nil {
			b.Ref = &mapsvc.AnswerRef{
				Kind:      link.Ref.Kind,
				Namespace: link.Ref.Namespace,
				Name:      link.Ref.Name,
			}
		}
		quotedParts := []string{}
		if link.Revision != "" {
			quotedParts = append(quotedParts, "revision="+link.Revision)
		}
		if link.Message != "" {
			quotedParts = append(quotedParts, link.Message)
		}
		if len(quotedParts) > 0 {
			b.Quoted = strings.Join(quotedParts, "; ")
		}
		ans.Basis = append(ans.Basis, b)
	}

	addLinkBasis("source", eval.Snapshot.Source)
	addLinkBasis("deployer", eval.Snapshot.Deployer)
	addLinkBasis("workload", eval.Snapshot.Workload)
	addLinkBasis("pod-images", eval.Snapshot.PodImages)

	switch eval.Reason {
	case agent.LandingReasonNoAnchorSupplied:
		ans.Unknowns = append(ans.Unknowns, mapsvc.AnswerUnknown{
			What:   "expected revision or digest",
			Kind:   mapsvc.UnknownNotRequested,
			Reason: "pass --revision, --digest, or --unit-revision",
		})
	case agent.LandingReasonReadDenied:
		ans.Unknowns = append(ans.Unknowns, mapsvc.AnswerUnknown{
			What:   "target or delivery controller read",
			Kind:   mapsvc.UnknownDenied,
			Reason: "Kubernetes RBAC denied reading the object",
			Quoted: eval.Snapshot.ReadError,
		})
	case agent.LandingReasonClusterUnreachable:
		ans.Unknowns = append(ans.Unknowns, mapsvc.AnswerUnknown{
			What:   "cluster state",
			Kind:   mapsvc.UnknownNotObservable,
			Reason: "cluster could not be reached",
			Quoted: eval.Snapshot.ReadError,
		})
	case agent.LandingReasonTargetNotFound, agent.LandingReasonNoOwnerMarkerRecognised, agent.LandingReasonUnsupportedController:
		ans.Unknowns = append(ans.Unknowns, mapsvc.AnswerUnknown{
			What:   "delivery chain verification",
			Kind:   mapsvc.UnknownNotObservable,
			Reason: eval.Reason,
		})
	}

	target := eval.Snapshot.Target
	if traceSafeKind.MatchString(target.Kind) && traceSafeName.MatchString(target.Name) && traceSafeName.MatchString(target.Namespace) {
		switch eval.State {
		case agent.LandingStateNotLanded:
			ans.Next = append(ans.Next, mapsvc.AnswerNext{
				Command: preferInvocationForm(fmt.Sprintf("cub-scout trace %s/%s -n %s --format json", target.Kind, target.Name, target.Namespace)),
				Why:     "inspect the full delivery chain and controller conditions",
			})
		case agent.LandingStateConverging:
			ans.Next = append(ans.Next, mapsvc.AnswerNext{
				Command: preferInvocationForm(fmt.Sprintf("cub-scout explain %s/%s -n %s --format json", target.Kind, target.Name, target.Namespace)),
				Why:     "inspect current rollout progress and events while converging",
			})
		case agent.LandingStateNotObservable:
			ans.Next = append(ans.Next, mapsvc.AnswerNext{
				Command: preferInvocationForm(fmt.Sprintf("cub-scout explain %s/%s -n %s --format json", target.Kind, target.Name, target.Namespace)),
				Why:     "check ownership metadata and read limitations on the workload",
			})
		}
	}

	return ans
}

func renderLandedOutput(w io.Writer, out landedOutput, format string) error {
	return renderLandedOutputWithDelta(w, out, format, "")
}

func renderLandedOutputWithDelta(w io.Writer, out landedOutput, format, ifUnchanged string) error {
	switch format {
	case "json":
		if out.Fingerprint == "" && out.Answer != nil {
			out.Fingerprint = computeAnswerFingerprint(out.Answer)
		}
		if matchesIfUnchanged(ifUnchanged, out.Fingerprint) {
			observedAt := ""
			if out.Answer != nil {
				observedAt = out.Answer.ObservedAt
			}
			return writeUnchangedEnvelope(w, out.Fingerprint, observedAt)
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(out)
	case "md":
		_, err := fmt.Fprintf(w, "## Landed: `%s` (`%s`)\n\n- Target: `%s/%s` in `%s`\n- Anchor: `%s` = `%s`\n- Summary: %s\n",
			out.Answer.State, out.Answer.Reason,
			out.Target.Kind, out.Target.Name, out.Target.Namespace,
			out.Anchor.Kind, out.Anchor.Value, out.Answer.Summary)
		return err
	default:
		_, err := fmt.Fprintf(w, "LANDED: %s (%s)\n  Target:  %s/%s in %s\n  Anchor:  %s=%s\n  Summary: %s\n",
			out.Answer.State, out.Answer.Reason,
			out.Target.Kind, out.Target.Name, out.Target.Namespace,
			out.Anchor.Kind, out.Anchor.Value, out.Answer.Summary)
		return err
	}
}
