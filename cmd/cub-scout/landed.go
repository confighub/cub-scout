// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

var (
	landedNamespace    string
	landedKubeContext  string
	landedRevision     string
	landedDigest       string
	landedUnitRevision string
	landedWait         time.Duration
	landedPollInterval time.Duration
	landedFormat       string
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
	landedCmd.Flags().StringVar(&landedRevision, "revision", "", "Expected Git SHA or controller revision")
	landedCmd.Flags().StringVar(&landedDigest, "digest", "", "Expected OCI digest (e.g. sha256:...)")
	landedCmd.Flags().StringVar(&landedUnitRevision, "unit-revision", "", "Expected ConfigHub unit revision")
	landedCmd.Flags().DurationVar(&landedWait, "wait", 0, "Bounded read-only wait duration until settled (default: 0s, max: 5m)")
	landedCmd.Flags().DurationVar(&landedPollInterval, "poll-interval", agent.DefaultLandingPollInterval, "Interval between bounded-wait polls (default: 5s, min: 2s)")
	landedCmd.Flags().StringVar(&landedFormat, "format", "ascii", "Output format: ascii, json, md")
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

	target, err := parseLandedTarget(args, landedNamespace)
	if err != nil {
		return err
	}
	anchor, err := resolveLandingAnchor(landedRevision, landedDigest, landedUnitRevision)
	if err != nil {
		return err
	}

	selection, err := clusterContextSelectionFromFlag(cmd)
	if err != nil {
		return err
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// If no anchor was supplied, return not-observable without touching the cluster.
	if anchor.Value == "" {
		snap := agent.LandingSnapshot{Context: selection.name, Target: target, ObservedAt: time.Now().UTC()}
		eval := agent.EvaluateLanding(anchor, snap)
		out := buildLandedOutput(eval, nil)
		return renderLandedOutput(os.Stdout, out, format)
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
		return renderLandedOutput(os.Stdout, out, format)
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
		return fetchLandingSnapshot(pollCtx, dynClient, session.contextLabel(), target), nil
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
	return renderLandedOutput(os.Stdout, out, format)
}

func fetchLandingSnapshot(ctx context.Context, dyn dynamic.Interface, kubeCtx string, target agent.LandingObjectRef) agent.LandingSnapshot {
	snap := agent.LandingSnapshot{
		Context:    kubeCtx,
		Target:     target,
		ObservedAt: time.Now().UTC(),
	}
	obj, err := dyn.Resource(workloadGVR(target.Kind)).Namespace(target.Namespace).Get(ctx, target.Name, metav1.GetOptions{})
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
	labels := obj.GetLabels()
	annotations := obj.GetAnnotations()

	switch strings.ToLower(own.Type) {
	case "flux":
		snap.Deployer, snap.Source, snap.ReadDenied, snap.ReadError = fetchFluxLandingLinks(ctx, dyn, labels)
	case "argocd", "argo":
		snap.Deployer, snap.ReadDenied, snap.ReadError = fetchArgoLandingLink(ctx, dyn, labels, annotations)
	}

	return snap
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

func fetchFluxLandingLinks(ctx context.Context, dyn dynamic.Interface, labels map[string]string) (deployer, source *agent.LandingLinkObservation, denied bool, readErr string) {
	if ksName := labels["kustomize.toolkit.fluxcd.io/name"]; ksName != "" {
		ksNS := labels["kustomize.toolkit.fluxcd.io/namespace"]
		if ksNS == "" {
			ksNS = "flux-system"
		}
		ksGVR := schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
		ks, err := dyn.Resource(ksGVR).Namespace(ksNS).Get(ctx, ksName, metav1.GetOptions{})
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
			source = fetchFluxSourceObject(ctx, dyn, srcKind, srcNS, srcName)
		}
		return deployer, source, false, ""
	}
	if hrName := labels["helm.toolkit.fluxcd.io/name"]; hrName != "" {
		hrNS := labels["helm.toolkit.fluxcd.io/namespace"]
		if hrNS == "" {
			hrNS = "flux-system"
		}
		hrGVR := schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
		hr, err := dyn.Resource(hrGVR).Namespace(hrNS).Get(ctx, hrName, metav1.GetOptions{})
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

func fetchFluxSourceObject(ctx context.Context, dyn dynamic.Interface, kind, ns, name string) *agent.LandingLinkObservation {
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
	obj, err := dyn.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
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

func fetchArgoLandingLink(ctx context.Context, dyn dynamic.Interface, labels, annotations map[string]string) (deployer *agent.LandingLinkObservation, denied bool, readErr string) {
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
	list, err := dyn.Resource(appGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
			return nil, true, err.Error()
		}
		return nil, false, err.Error()
	}
	for i := range list.Items {
		app := &list.Items[i]
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

func buildLandedOutput(eval agent.LandingEvaluation, cost *agent.KubernetesReadCost) landedOutput {
	return landedOutput{
		Context:     eval.Snapshot.Context,
		Target:      eval.Snapshot.Target,
		Owner:       eval.Snapshot.Owner,
		Anchor:      eval.Anchor,
		BrokenLink:  eval.BrokenLink,
		Convergence: eval.Convergence,
		Wait:        eval.Wait,
		Answer:      buildLandingAnswer(eval, cost),
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
	switch format {
	case "json":
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
