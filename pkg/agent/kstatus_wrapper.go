// Package agent — kstatus wrapper.
//
// Narrow slice of #394: provide deterministic readiness derivation for the
// workload kinds touched by the source-truth contract (#393). Today this
// covers Deployment, StatefulSet, DaemonSet — the three kinds the import
// path observes on the live cluster surface.
//
// The wrapper backs kstatus from sigs.k8s.io/cli-utils, which is the same
// library Argo CD and Flux use to compute resource health. Aligning on
// kstatus means cub-scout's "ready" verdict matches what operators already
// see in those tools, which is required for #393's source-truth verdicts to
// be trustworthy.
//
// Behaviour delta vs. the prior ad-hoc `ReadyReplicas == Spec.Replicas`
// pattern:
//   - A workload with Stalled=True now reports `ready=false` instead of
//     "ready or unknown". Stricter, and aligned with operator expectations.
//   - Nil spec.replicas / missing fields no longer panic — kstatus handles
//     them and returns InProgress or Unknown rather than dereferencing nil.
//   - Generation/observedGeneration skew is honoured (a Deployment whose
//     status reflects an older spec is correctly classified as InProgress,
//     not "ready").

package agent

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
)

// IsResourceReadyOrUnknown is the kstatus-backed replacement for the
// custom `isReadyOrUnknown` helper that lived in pkg/agent/state_scanner.go.
// Second slice of the #394 migration — covers the four call sites
// (findSilentFailures × 2, findTimingBombs × 2) that filter Flux
// HelmRelease / Kustomization resources before silent-failure / timing-
// bomb detection.
//
// Returns true when kstatus considers item to be either Current
// (definitely ready) or Unknown (no Ready signal — common on freshly-
// created CRs before the controller stamps a condition). Returns false
// for InProgress, Failed, Terminating, and NotFound.
//
// Behaviour delta vs. the prior helper, which only inspected the Ready
// condition:
//
//   - Stalled=True now returns false (was true). Intended improvement:
//     if Flux already reports Stalled, the silent-failure and timing-
//     bomb detectors should not also probe — we already know the
//     resource is broken, so additional probing is redundant.
//   - No conditions returns true (kstatus reports UnknownStatus under
//     the conditions-reader fallback). Matches the old helper.
//   - Ready=True / Ready=Unknown / Ready=False classify the same way
//     the old helper classified them.
//
// Accepts a value (not a pointer) to match the existing call-site
// signature in state_scanner.go.
func IsResourceReadyOrUnknown(item unstructured.Unstructured) bool {
	res, err := status.Compute(&item)
	if err != nil || res == nil {
		// Could not classify — be lenient, matching the prior helper's
		// "no conditions = treat as unknown" semantics.
		return true
	}
	switch res.Status {
	case status.CurrentStatus, status.UnknownStatus:
		return true
	default:
		return false
	}
}

// IsDeploymentReady returns true when kstatus considers d to be at its
// desired state (CurrentStatus). Any other state returns false.
func IsDeploymentReady(d *appsv1.Deployment) bool {
	return computeStatus(d, "apps/v1", "Deployment") == status.CurrentStatus
}

// IsStatefulSetReady returns true when kstatus considers s to be at its
// desired state (CurrentStatus). Any other state returns false.
func IsStatefulSetReady(s *appsv1.StatefulSet) bool {
	return computeStatus(s, "apps/v1", "StatefulSet") == status.CurrentStatus
}

// IsDaemonSetReady returns true when kstatus considers d to be at its
// desired state (CurrentStatus). Any other state returns false.
func IsDaemonSetReady(d *appsv1.DaemonSet) bool {
	return computeStatus(d, "apps/v1", "DaemonSet") == status.CurrentStatus
}

// WorkloadStatus returns the kstatus verdict and message for a typed
// apps/v1 workload. Useful when callers need to distinguish InProgress
// (rollout pending) from Failed (Stalled) — for example when surfacing
// evidence inside the #393 source-truth contract.
//
// kind must be one of "Deployment", "StatefulSet", "DaemonSet". For other
// kinds the caller should compute status themselves from an unstructured
// object.
func WorkloadStatus(obj runtime.Object, kind string) (status.Status, string) {
	u, err := toUnstructured(obj, "apps/v1", kind)
	if err != nil {
		return status.UnknownStatus, err.Error()
	}
	res, err := status.Compute(u)
	if err != nil || res == nil {
		return status.UnknownStatus, ""
	}
	return res.Status, res.Message
}

// WorkloadConvergence returns the kstatus verdict and message for any
// unstructured object. Unlike WorkloadStatus it does not re-seed the GVK
// (the object already carries it) and accepts any kind kstatus classifies —
// Deployment, StatefulSet, DaemonSet, Job, PersistentVolumeClaim, Pod.
// Returns UnknownStatus when the object is nil or cannot be classified.
func WorkloadConvergence(u *unstructured.Unstructured) (status.Status, string) {
	if u == nil {
		return status.UnknownStatus, ""
	}
	res, err := status.Compute(u)
	if err != nil || res == nil {
		return status.UnknownStatus, ""
	}
	return res.Status, res.Message
}

// computeStatus is the shared helper. Returns UnknownStatus on any error.
func computeStatus(obj runtime.Object, apiVersion, kind string) status.Status {
	u, err := toUnstructured(obj, apiVersion, kind)
	if err != nil {
		return status.UnknownStatus
	}
	res, err := status.Compute(u)
	if err != nil || res == nil {
		return status.UnknownStatus
	}
	return res.Status
}

// toUnstructured converts a typed object into the *unstructured.Unstructured
// kstatus.Compute expects. The conversion drops the GVK if the typed object
// did not have TypeMeta set (which the apps/v1 list APIs do not by default),
// so we re-seed it from the caller-supplied apiVersion/kind. Without this,
// kstatus's polymorphic dispatch falls through to the generic conditions
// reader instead of using the kind-specific logic.
func toUnstructured(obj runtime.Object, apiVersion, kind string) (*unstructured.Unstructured, error) {
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, err
	}
	u := &unstructured.Unstructured{Object: raw}
	u.SetAPIVersion(apiVersion)
	u.SetKind(kind)
	return u, nil
}
