package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// The states of one workload a controller lists for a Release.
const (
	liveStatusWorkloadReady     = "ready"
	liveStatusWorkloadNotReady  = "not-ready"
	liveStatusWorkloadUnsettled = "unsettled"
	liveStatusWorkloadMissing   = "missing"
	liveStatusWorkloadUnread    = "unread"
)

// ConfigHubLiveStatusHealthCheck sets the health a deploying tool recorded on
// a Release beside the workloads the matched controllers list for it (#887).
// Like the digest comparison it compares two reports and is not a verdict.
type ConfigHubLiveStatusHealthCheck struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
	// WorkloadsListed counts the Deployments, StatefulSets and DaemonSets the
	// matched controllers list; WorkloadsReady those found ready.
	WorkloadsListed int `json:"workloadsListed"`
	WorkloadsReady  int `json:"workloadsReady"`
	// NotReady names every listed workload that was not found ready, and why.
	NotReady []ConfigHubLiveStatusWorkload `json:"notReady,omitempty"`
}

// ConfigHubLiveStatusWorkload is one listed workload that was not found ready.
type ConfigHubLiveStatusWorkload struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	State     string `json:"state"`
	// NotReadySince is when a Deployment's Available condition turned false.
	// No other time is read: without it, when the workload stopped being
	// ready is unknown.
	NotReadySince string `json:"notReadySince,omitempty"`
}

var liveStatusWorkloadGVRs = map[string]schema.GroupVersionResource{
	"Deployment":  {Group: "apps", Version: "v1", Resource: "deployments"},
	"StatefulSet": {Group: "apps", Version: "v1", Resource: "statefulsets"},
	"DaemonSet":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
}

type liveStatusWorkloadRef struct{ kind, namespace, name string }

// liveStatusWorkloads reads each workload kind at most once for a whole run.
type liveStatusWorkloads struct {
	client dynamic.Interface
	byKind map[string]map[string]*unstructured.Unstructured
	unread map[string]string
}

func (w *liveStatusWorkloads) get(ctx context.Context, ref liveStatusWorkloadRef) (*unstructured.Unstructured, string) {
	if w.byKind == nil {
		w.byKind, w.unread = map[string]map[string]*unstructured.Unstructured{}, map[string]string{}
	}
	if _, read := w.byKind[ref.kind]; !read {
		w.byKind[ref.kind] = map[string]*unstructured.Unstructured{}
		items, err := w.client.Resource(liveStatusWorkloadGVRs[ref.kind]).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		switch {
		case apierrors.IsNotFound(err) || meta.IsNoMatchError(err):
		case err != nil:
			w.unread[ref.kind] = fmt.Sprintf("%ss could not be listed: %v", ref.kind, err)
		default:
			for i := range items.Items {
				w.byKind[ref.kind][items.Items[i].GetNamespace()+"/"+items.Items[i].GetName()] = &items.Items[i]
			}
		}
	}
	return w.byKind[ref.kind][ref.namespace+"/"+ref.name], w.unread[ref.kind]
}

// liveStatusListedWorkloads is the Deployments, StatefulSets and DaemonSets a
// controller object lists as its own: a Flux Kustomization's inventory or an
// Argo CD Application's resources. Nothing is inferred from labels.
func liveStatusListedWorkloads(object *unstructured.Unstructured) []liveStatusWorkloadRef {
	var refs []liveStatusWorkloadRef
	add := func(group, kind, namespace, name string) {
		if _, known := liveStatusWorkloadGVRs[kind]; known && group == "apps" && namespace != "" && name != "" {
			refs = append(refs, liveStatusWorkloadRef{kind, namespace, name})
		}
	}
	switch object.GetKind() {
	case "Kustomization":
		entries, _, _ := unstructured.NestedSlice(object.Object, "status", "inventory", "entries")
		for _, entry := range entries {
			fields, _ := entry.(map[string]interface{})
			id, _ := fields["id"].(string)
			// <namespace>_<name>_<group>_<Kind>; none of the four can hold an underscore.
			if parts := strings.Split(id, "_"); len(parts) == 4 {
				add(parts[2], parts[3], parts[0], parts[1])
			}
		}
	case "Application":
		resources, _, _ := unstructured.NestedSlice(object.Object, "status", "resources")
		for _, resource := range resources {
			fields, _ := resource.(map[string]interface{})
			group, _ := fields["group"].(string)
			kind, _ := fields["kind"].(string)
			namespace, _ := fields["namespace"].(string)
			name, _ := fields["name"].(string)
			add(group, kind, namespace, name)
		}
	}
	return refs
}

// liveStatusWorkloadState reads one workload with the rule map and scan use
// for readiness, and refuses to call it ready while its own controller has
// not caught up with its spec or a rollout is under way.
func liveStatusWorkloadState(object *unstructured.Unstructured) (state, since string) {
	observed, found, _ := unstructured.NestedInt64(object.Object, "status", "observedGeneration")
	if !found || observed < object.GetGeneration() {
		return liveStatusWorkloadUnsettled, ""
	}
	switch detectStatus(object) {
	case "Ready":
		if object.GetKind() == "Deployment" {
			desired, set, _ := unstructured.NestedInt64(object.Object, "spec", "replicas")
			if !set {
				desired = 1
			}
			if updated, _, _ := unstructured.NestedInt64(object.Object, "status", "updatedReplicas"); updated < desired {
				return liveStatusWorkloadUnsettled, ""
			}
		}
		return liveStatusWorkloadReady, ""
	case "NotReady":
		conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
		for _, condition := range conditions {
			fields, _ := condition.(map[string]interface{})
			if fields["type"] == "Available" && fields["status"] == "False" {
				since, _ = fields["lastTransitionTime"].(string)
			}
		}
		return liveStatusWorkloadNotReady, since
	}
	return liveStatusWorkloadUnsettled, ""
}

// checkLiveStatusHealth compares the recorded health with the workloads the
// matched controllers list. It is called only once those controllers were all
// found on the Release's digest, so the workloads they list are this Release's.
func checkLiveStatusHealth(ctx context.Context, workloads *liveStatusWorkloads, controllers []*unstructured.Unstructured, recorded string, recordedAt time.Time) *ConfigHubLiveStatusHealthCheck {
	check := &ConfigHubLiveStatusHealthCheck{State: liveStatusClusterNotObservable}
	health := strings.ToLower(strings.TrimSpace(recorded))
	if health != "healthy" && health != "degraded" {
		check.Reason = fmt.Sprintf("the recorded health %q is neither Healthy nor Degraded, so it says nothing workloads can be compared with", recorded)
		return check
	}
	seen := map[liveStatusWorkloadRef]bool{}
	var refs []liveStatusWorkloadRef
	for _, controller := range controllers {
		for _, ref := range liveStatusListedWorkloads(controller) {
			if !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	sort.Slice(refs, func(a, b int) bool {
		return refs[a].kind+"/"+refs[a].namespace+"/"+refs[a].name < refs[b].kind+"/"+refs[b].namespace+"/"+refs[b].name
	})
	check.WorkloadsListed = len(refs)
	if len(refs) == 0 {
		check.Reason = "the controllers compared list no Deployment, StatefulSet or DaemonSet for this Release, and no other kind is read"
		return check
	}
	notReadyBefore, notReadyLater, notReadyUndated, unknown := 0, 0, 0, 0
	unread := map[string]bool{}
	for _, ref := range refs {
		object, failure := workloads.get(ctx, ref)
		state, since := liveStatusWorkloadMissing, ""
		switch {
		case failure != "":
			state = liveStatusWorkloadUnread
			unread[failure] = true
		case object != nil:
			state, since = liveStatusWorkloadState(object)
		}
		switch state {
		case liveStatusWorkloadReady:
			check.WorkloadsReady++
			continue
		case liveStatusWorkloadNotReady:
			began, err := time.Parse(time.RFC3339, since)
			switch {
			case err != nil:
				notReadyUndated++
			case began.After(recordedAt):
				notReadyLater++
			default:
				notReadyBefore++
			}
		default:
			unknown++
		}
		check.NotReady = append(check.NotReady, ConfigHubLiveStatusWorkload{Kind: ref.kind, Namespace: ref.namespace, Name: ref.name, State: state, NotReadySince: since})
	}
	notReady := notReadyBefore + notReadyLater + notReadyUndated
	switch {
	case health == "degraded" && notReady > 0:
		check.State = liveStatusClusterAgrees
		check.Reason = fmt.Sprintf("the recorded health is %q, and %d of %d workloads the controllers list are not ready", recorded, notReady, len(refs))
	case health == "degraded" && unknown > 0:
		check.Reason = fmt.Sprintf("%d of %d workloads the controllers list could not be read as ready or not", unknown, len(refs))
	case health == "degraded":
		check.Reason = fmt.Sprintf("the recorded health is %q and all %d workloads the controllers list are ready now; the report may concern a kind that is not read here, or an earlier moment", recorded, len(refs))
	case notReadyBefore > 0:
		check.State = liveStatusClusterDisagrees
		check.Reason = fmt.Sprintf("the recorded health is %q, and %d of %d workloads the controllers list have been unavailable since before it was recorded", recorded, notReadyBefore, len(refs))
	case notReadyLater > 0:
		check.Reason = fmt.Sprintf("%d of %d workloads the controllers list became unavailable after the health was recorded; both can have been true in turn", notReadyLater, len(refs))
	case notReadyUndated > 0:
		check.Reason = fmt.Sprintf("%d of %d workloads the controllers list are not ready now and give no time for when that began, so it may have followed the recorded health", notReadyUndated, len(refs))
	case unknown > 0:
		check.Reason = fmt.Sprintf("%d of %d workloads the controllers list are missing, mid-rollout or could not be read", unknown, len(refs))
	default:
		check.State = liveStatusClusterAgrees
		check.Reason = fmt.Sprintf("the recorded health is %q, and all %d workloads the controllers list are ready", recorded, len(refs))
	}
	failures := make([]string, 0, len(unread))
	for failure := range unread {
		failures = append(failures, failure)
	}
	sort.Strings(failures)
	if len(failures) > 0 {
		check.Reason += "; " + strings.Join(failures, "; ")
	}
	return check
}
