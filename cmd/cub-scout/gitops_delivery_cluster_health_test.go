package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktesting "k8s.io/client-go/testing"
)

// An Application on the Release's digest that lists the given workloads.
func healthApplication(digest string, workloads ...*unstructured.Unstructured) *unstructured.Unstructured {
	app := checkApplication("payments", checkRepo, digest)
	resources := []interface{}{map[string]interface{}{"kind": "Service", "namespace": "payments", "name": "api", "version": "v1"}}
	for _, w := range workloads {
		resources = append(resources, map[string]interface{}{"group": "apps", "kind": w.GetKind(), "namespace": w.GetNamespace(), "name": w.GetName(), "version": "v1"})
	}
	_ = unstructured.SetNestedSlice(app.Object, resources, "status", "resources")
	return app
}

func healthDeployment(name string, ready int64, unavailableSince time.Time) *unstructured.Unstructured {
	status := map[string]interface{}{"observedGeneration": int64(3), "replicas": int64(2), "updatedReplicas": int64(2), "readyReplicas": ready}
	if ready < 2 {
		status["unavailableReplicas"] = 2 - ready
	}
	if !unavailableSince.IsZero() {
		status["conditions"] = []interface{}{map[string]interface{}{"type": "Available", "status": "False", "lastTransitionTime": unavailableSince.Format(time.RFC3339)}}
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": name, "namespace": "payments", "generation": int64(3)},
		"spec":     map[string]interface{}{"replicas": int64(2)}, "status": status,
	}}
}

func healthStatefulSet(ready int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "StatefulSet",
		"metadata": map[string]interface{}{"name": "db", "namespace": "payments", "generation": int64(1)},
		"spec":     map[string]interface{}{"replicas": int64(1)},
		"status":   map[string]interface{}{"observedGeneration": int64(1), "replicas": int64(1), "readyReplicas": ready, "unavailableReplicas": 1 - ready},
	}}
}

func healthStatuses(sync, health string) []ConfigHubLiveStatusEvidence {
	statuses := checkStatus(sync)
	statuses[0].HealthStatus = health
	return statuses
}

func TestClusterCheckComparesTheRecordedHealthWithTheListedWorkloads(t *testing.T) {
	noRegistryLookup(t)
	recorded := checkNow.Add(-30 * time.Second)
	ready := healthDeployment("api", 2, time.Time{})
	downBefore := healthDeployment("api", 0, recorded.Add(-time.Minute))
	downAfter := healthDeployment("api", 0, recorded.Add(10*time.Second))
	partial := healthDeployment("api", 1, time.Time{})
	rolling := healthDeployment("api", 2, time.Time{})
	_ = unstructured.SetNestedField(rolling.Object, int64(1), "status", "updatedReplicas")
	behind := healthDeployment("api", 2, time.Time{})
	_ = unstructured.SetNestedField(behind.Object, int64(2), "status", "observedGeneration")
	for _, tc := range []struct {
		name     string
		health   string
		listed   []*unstructured.Unstructured
		present  []runtime.Object
		want     string
		reason   string
		notReady int
	}{
		{"Healthy and every listed workload is ready", "Healthy", []*unstructured.Unstructured{ready, healthStatefulSet(1)}, []runtime.Object{ready, healthStatefulSet(1)}, liveStatusClusterAgrees, "all 2 workloads", 0},
		{"Healthy, and a Deployment was unavailable before that was recorded", "Healthy", []*unstructured.Unstructured{downBefore}, []runtime.Object{downBefore}, liveStatusClusterDisagrees, "since before it was recorded", 1},
		{"Healthy, and a Deployment became unavailable afterwards", "Healthy", []*unstructured.Unstructured{downAfter}, []runtime.Object{downAfter}, liveStatusClusterNotObservable, "both can have been true in turn", 1},
		{"Healthy, and a Deployment is short of replicas with no time given", "Healthy", []*unstructured.Unstructured{partial}, []runtime.Object{partial}, liveStatusClusterNotObservable, "give no time", 1},
		{"Healthy, and a StatefulSet is not ready, which carries no time", "Healthy", []*unstructured.Unstructured{healthStatefulSet(0)}, []runtime.Object{healthStatefulSet(0)}, liveStatusClusterNotObservable, "give no time", 1},
		{"Healthy, and a rollout is under way", "Healthy", []*unstructured.Unstructured{rolling}, []runtime.Object{rolling}, liveStatusClusterNotObservable, "mid-rollout", 1},
		{"Healthy, and the Deployment's own controller is behind its spec", "Healthy", []*unstructured.Unstructured{behind}, []runtime.Object{behind}, liveStatusClusterNotObservable, "mid-rollout", 1},
		{"Healthy, and a listed workload is not in the cluster", "Healthy", []*unstructured.Unstructured{ready}, nil, liveStatusClusterNotObservable, "missing", 1},
		{"Healthy, and the controller lists no workload", "Healthy", nil, nil, liveStatusClusterNotObservable, "list no Deployment, StatefulSet or DaemonSet", 0},
		{"Degraded and a workload is not ready", "Degraded", []*unstructured.Unstructured{downAfter}, []runtime.Object{downAfter}, liveStatusClusterAgrees, "1 of 1 workloads", 1},
		{"Degraded while every workload read is ready is not a disagreement", "Degraded", []*unstructured.Unstructured{ready}, []runtime.Object{ready}, liveStatusClusterNotObservable, "a kind that is not read here", 0},
		{"Progressing is not compared", "Progressing", []*unstructured.Unstructured{ready}, []runtime.Object{ready}, liveStatusClusterNotObservable, "neither Healthy nor Degraded", 0},
		{"no health was recorded", "", []*unstructured.Unstructured{ready}, []runtime.Object{ready}, liveStatusClusterNotObservable, "neither Healthy nor Degraded", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statuses := healthStatuses("Synced", tc.health)
			objects := append([]runtime.Object{healthApplication(checkDigest, tc.listed...)}, tc.present...)
			checkLiveStatusesAgainstCluster(context.Background(), checkClient(objects...), statuses, checkNow)
			check := statuses[0].ClusterCheck
			require.Equal(t, liveStatusClusterAgrees, check.State, check.Reason)
			require.NotNil(t, check.Health)
			require.Equal(t, tc.want, check.Health.State, check.Health.Reason)
			require.Contains(t, check.Health.Reason, tc.reason)
			require.Len(t, check.Health.NotReady, tc.notReady)
			if tc.reason != "neither Healthy nor Degraded" {
				// A health that is not compared reads no inventory.
				require.Equal(t, len(tc.listed), check.Health.WorkloadsListed)
			}
		})
	}
}

// Workloads listed by a controller that is on another digest, or under a
// status that is not Synced, are not this Release's: health is left alone and
// no workload is read.
func TestClusterCheckComparesNoHealthUnlessTheControllersAreOnTheRelease(t *testing.T) {
	noRegistryLookup(t)
	down := healthDeployment("api", 0, checkNow.Add(-time.Hour))
	for _, tc := range []struct {
		name, sync, digest, state string
	}{
		{"controller on another digest", "Synced", otherDigest, liveStatusClusterDisagrees},
		{"recorded OutOfSync and the cluster agrees", "OutOfSync", otherDigest, liveStatusClusterAgrees},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statuses := healthStatuses(tc.sync, "Healthy")
			client := checkClient(healthApplication(tc.digest, down), down)
			checkLiveStatusesAgainstCluster(context.Background(), client, statuses, checkNow)
			check := statuses[0].ClusterCheck
			require.Equal(t, tc.state, check.State, check.Reason)
			require.Equal(t, liveStatusClusterNotObservable, check.Health.State)
			require.Contains(t, check.Health.Reason, "may belong to another Release")
			for _, action := range client.Actions() {
				require.NotEqual(t, "deployments", action.GetResource().Resource)
			}
		})
	}
}

func TestClusterCheckHealthNamesAWorkloadKindItCouldNotList(t *testing.T) {
	noRegistryLookup(t)
	ready := healthDeployment("api", 2, time.Time{})
	client := checkClient(healthApplication(checkDigest, ready), ready)
	client.PrependReactor("list", "deployments", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "", nil)
	})
	statuses := healthStatuses("Synced", "Healthy")
	checkLiveStatusesAgainstCluster(context.Background(), client, statuses, checkNow)
	health := statuses[0].ClusterCheck.Health
	require.Equal(t, liveStatusClusterNotObservable, health.State)
	require.Contains(t, health.Reason, "Deployments could not be listed")
	require.Equal(t, liveStatusWorkloadUnread, health.NotReady[0].State)
}

// A Flux Kustomization's inventory is read by its documented id form, and
// each workload kind is listed once however many statuses are checked.
func TestClusterCheckHealthReadsAFluxInventoryAndListsEachKindOnce(t *testing.T) {
	noRegistryLookup(t)
	repository, kustomization := checkFlux(checkDigest)
	_ = unstructured.SetNestedSlice(kustomization.Object, []interface{}{
		map[string]interface{}{"id": "payments_api_apps_Deployment", "v": "v1"},
		map[string]interface{}{"id": "payments_api__Service", "v": "v1"},
		map[string]interface{}{"id": "_payments__Namespace", "v": "v1"},
	}, "status", "inventory", "entries")
	ready := healthDeployment("api", 2, time.Time{})
	client := checkClient(repository, kustomization, ready)
	statuses := append(healthStatuses("Synced", "Healthy"), healthStatuses("Synced", "Healthy")...)
	checkLiveStatusesAgainstCluster(context.Background(), client, statuses, checkNow)
	for _, status := range statuses {
		require.Equal(t, liveStatusClusterAgrees, status.ClusterCheck.Health.State, status.ClusterCheck.Health.Reason)
		require.Equal(t, 1, status.ClusterCheck.Health.WorkloadsListed)
	}
	lists := 0
	for _, action := range client.Actions() {
		if action.GetResource().Resource == "deployments" {
			lists++
		}
	}
	require.Equal(t, 1, lists)
	require.Contains(t, liveStatusClusterCheckText(statuses[0].ClusterCheck), "; health agrees: ")
}
