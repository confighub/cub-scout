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
