package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

const (
	checkDigest = "sha256:81477eda3e60e2945122e64a08fad0f0d0aa94618cc5d150df1315a12f6c03f7"
	otherDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	// A known ConfigHub endpoint, so it is recognised without a lookup.
	checkRepo = "oci://oci.api.confighub.com/space/payments-prod"
)

var checkNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// No test here may ask a ConfigHub server which registry it advertises.
func noRegistryLookup(t *testing.T) {
	t.Helper()
	old := agent.ConfigHubOCIRegistryFn
	agent.ConfigHubOCIRegistryFn = func() string { return "" }
	t.Cleanup(func() { agent.ConfigHubOCIRegistryFn = old })
}

func checkClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		liveStatusApplicationGVR:   "ApplicationList",
		liveStatusOCIRepositoryGVR: "OCIRepositoryList",
		liveStatusKustomizationGVR: "KustomizationList",
	}, objects...)
}

// A settled Argo CD Application pulling the space, reporting digest as applied.
func checkApplication(name, repo, digest string) *unstructured.Unstructured {
	source := map[string]interface{}{"repoURL": repo, "targetRevision": "latest", "path": "."}
	destination := map[string]interface{}{"server": "https://kubernetes.default.svc", "namespace": "payments"}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
		"metadata": map[string]interface{}{"name": name, "namespace": "argocd", "uid": "uid-" + name, "generation": int64(1)},
		"spec":     map[string]interface{}{"source": source, "destination": destination},
		"status": map[string]interface{}{
			"reconciledAt":   checkNow.Add(-time.Minute).Format(time.RFC3339),
			"operationState": map[string]interface{}{"phase": "Succeeded"},
			"sync": map[string]interface{}{"status": "Synced", "revision": digest,
				"comparedTo": map[string]interface{}{"source": source, "destination": destination}},
		},
	}}
}

func checkFlux(digest string) (*unstructured.Unstructured, *unstructured.Unstructured) {
	repository := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "source.toolkit.fluxcd.io/v1", "kind": "OCIRepository",
		"metadata": map[string]interface{}{"name": "confighub-root", "namespace": "flux-system"},
		"spec":     map[string]interface{}{"url": checkRepo},
	}}
	kustomization := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1", "kind": "Kustomization",
		"metadata": map[string]interface{}{"name": "confighub-root", "namespace": "flux-system", "uid": "uid-ks", "generation": int64(2)},
		"spec":     map[string]interface{}{"sourceRef": map[string]interface{}{"kind": "OCIRepository", "name": "confighub-root"}},
		"status": map[string]interface{}{
			"observedGeneration": int64(2), "lastAppliedRevision": "latest@" + digest, "lastAttemptedRevision": "latest@" + digest,
			"conditions": []interface{}{map[string]interface{}{"type": "Ready", "status": "True", "observedGeneration": int64(2)}},
		},
	}}
	return repository, kustomization
}

func checkStatus(sync string) []ConfigHubLiveStatusEvidence {
	return []ConfigHubLiveStatusEvidence{{RecordedOn: "release", Space: "payments-prod", Release: "payments-prod#2", SyncStatus: sync, manifestDigest: checkDigest}}
}

func TestClusterCheckComparesTheRecordedSyncStateWithTheControllers(t *testing.T) {
	noRegistryLookup(t)
	repository, applied := checkFlux(checkDigest)
	_, behind := checkFlux(otherDigest)
	for _, tc := range []struct {
		name    string
		sync    string
		objects []runtime.Object
		want    string
		reason  string
	}{
		{"Argo CD applied it and the report says Synced", "Synced", []runtime.Object{checkApplication("payments", checkRepo, checkDigest)}, liveStatusClusterAgrees, "all 1 controllers"},
		{"Argo CD applied another digest and the report says Synced", "Synced", []runtime.Object{checkApplication("payments", checkRepo, otherDigest)}, liveStatusClusterDisagrees, "1 of 1 controllers"},
		{"Argo CD applied it and the report says OutOfSync", "OutOfSync", []runtime.Object{checkApplication("payments", checkRepo, checkDigest)}, liveStatusClusterDisagrees, `"OutOfSync"`},
		{"Argo CD applied another digest and the report says OutOfSync", "OutOfSync", []runtime.Object{checkApplication("payments", checkRepo, otherDigest)}, liveStatusClusterAgrees, "all 1"},
		{"Flux applied it", "Synced", []runtime.Object{repository, applied}, liveStatusClusterAgrees, "all 1"},
		{"Flux is on another digest", "Synced", []runtime.Object{repository, behind}, liveStatusClusterDisagrees, "1 of 1"},
		{"one controller disagrees among two", "Synced", []runtime.Object{checkApplication("a", checkRepo, checkDigest), checkApplication("b", checkRepo, otherDigest)}, liveStatusClusterDisagrees, "1 of 2"},
		{"nothing here pulls the space", "Synced", []runtime.Object{checkApplication("other", "oci://oci.api.confighub.com/space/another", checkDigest)}, liveStatusClusterNotObservable, `pulling ConfigHub space "payments-prod"`},
		{"a registry that is not ConfigHub's is not matched on its path", "Synced", []runtime.Object{checkApplication("lookalike", "oci://registry.example.com/space/payments-prod", checkDigest)}, liveStatusClusterNotObservable, "no Argo CD Application or Flux Kustomization"},
		{"no sync state was recorded", "Unknown", []runtime.Object{checkApplication("payments", checkRepo, checkDigest)}, liveStatusClusterNotObservable, "no sync state to compare"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statuses := checkStatus(tc.sync)
			checkLiveStatusesAgainstCluster(context.Background(), checkClient(tc.objects...), statuses, checkNow)
			check := statuses[0].ClusterCheck
			require.NotNil(t, check)
			require.Equal(t, tc.want, check.State, check.Reason)
			require.Contains(t, check.Reason, tc.reason)
			require.Contains(t, check.Limitation, "Health is not compared")
		})
	}
}

// An unsettled controller is not agreement and not disagreement.
func TestClusterCheckIsNotObservableWhenTheControllerHasNotSettled(t *testing.T) {
	noRegistryLookup(t)
	application := checkApplication("payments", checkRepo, checkDigest)
	require.NoError(t, unstructured.SetNestedField(application.Object, "Running", "status", "operationState", "phase"))
	statuses := checkStatus("Synced")
	checkLiveStatusesAgainstCluster(context.Background(), checkClient(application), statuses, checkNow)
	check := statuses[0].ClusterCheck
	require.Equal(t, liveStatusClusterNotObservable, check.State)
	require.Len(t, check.Controllers, 1)
	require.Equal(t, "unknown", check.Controllers[0].Revision.Comparison)
	require.Contains(t, check.Controllers[0].Revision.Reason, "Operation status is not Succeeded")
}

// A kind that cannot be listed is said so; it is never read as "no controller".
func TestClusterCheckNamesAKindItCouldNotList(t *testing.T) {
	noRegistryLookup(t)
	client := checkClient()
	client.PrependReactor("list", "applications", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "", nil)
	})
	statuses := checkStatus("Synced")
	checkLiveStatusesAgainstCluster(context.Background(), client, statuses, checkNow)
	check := statuses[0].ClusterCheck
	require.Equal(t, liveStatusClusterNotObservable, check.State)
	require.Contains(t, check.Reason, "Argo CD Applications could not be listed")
	require.Contains(t, check.Reason, "forbidden")
}

// Statuses from the Space annotation name no Release, so they are not checked,
// and a cluster is not read when there is nothing to check.
func TestClusterCheckLeavesAnnotationStatusesAloneAndReadsNothingForThem(t *testing.T) {
	noRegistryLookup(t)
	client := checkClient()
	statuses := []ConfigHubLiveStatusEvidence{{RecordedOn: "space-annotation", Space: "payments-prod", SyncStatus: "Synced"}}
	checkLiveStatusesAgainstCluster(context.Background(), client, statuses, checkNow)
	require.Nil(t, statuses[0].ClusterCheck)
	require.Empty(t, client.Actions())

	statuses = checkStatus("Synced")
	checkLiveStatusesAgainstCluster(context.Background(), nil, statuses, checkNow)
	require.Equal(t, "no cluster was read", statuses[0].ClusterCheck.Reason)

	statuses = checkStatus("Synced")
	statuses[0].manifestDigest = "sha256:short"
	checkLiveStatusesAgainstCluster(context.Background(), client, statuses, checkNow)
	require.True(t, strings.Contains(statuses[0].ClusterCheck.Reason, "no complete manifest digest"))
	require.Empty(t, client.Actions())
}
