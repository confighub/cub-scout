package main

import (
	"context"
	"os"
	"path/filepath"
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
		liveStatusApplicationGVR:              "ApplicationList",
		liveStatusOCIRepositoryGVR:            "OCIRepositoryList",
		liveStatusKustomizationGVR:            "KustomizationList",
		liveStatusClusterProfileGVR:           "ClusterProfileList",
		liveStatusWorkloadGVRs["Deployment"]:  "DeploymentList",
		liveStatusWorkloadGVRs["StatefulSet"]: "StatefulSetList",
		liveStatusWorkloadGVRs["DaemonSet"]:   "DaemonSetList",
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
	return []ConfigHubLiveStatusEvidence{{RecordedOn: "release", Space: "payments-prod", Release: "payments-prod#2", SyncStatus: sync, manifestDigest: checkDigest,
		ObservedAt: checkNow.Add(-30 * time.Second).Format(time.RFC3339), Freshness: "fresh"}}
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
			require.Contains(t, check.Limitation, "no other kind, no pod and nothing a controller does not list is read")
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

// The findings of the independent review of #896, each as a case that must
// not produce agreement or disagreement.
func TestClusterCheckDoesNotCompareWhatCannotBeCompared(t *testing.T) {
	noRegistryLookup(t)
	repository, applied := checkFlux(checkDigest)
	multi := checkApplication("multi", "", checkDigest)
	unstructured.RemoveNestedField(multi.Object, "spec", "source")
	require.NoError(t, unstructured.SetNestedSlice(multi.Object, []interface{}{map[string]interface{}{"repoURL": checkRepo}}, "spec", "sources"))
	forbidden := func(client *dynamicfake.FakeDynamicClient) {
		client.PrependReactor("list", "applications", func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "", nil)
		})
	}
	for _, tc := range []struct {
		name    string
		change  func(*ConfigHubLiveStatusEvidence)
		objects []runtime.Object
		react   func(*dynamicfake.FakeDynamicClient)
		reason  string
	}{
		{"a stale status", func(s *ConfigHubLiveStatusEvidence) { s.Freshness = "stale" }, []runtime.Object{checkApplication("a", checkRepo, otherDigest)}, nil, "the recorded status is stale"},
		{"an undated status", func(s *ConfigHubLiveStatusEvidence) { s.Freshness, s.ObservedAt = "unknown", "" }, []runtime.Object{checkApplication("a", checkRepo, checkDigest)}, nil, "the recorded status is unknown"},
		{"a sync state that is neither Synced nor OutOfSync", func(s *ConfigHubLiveStatusEvidence) { s.SyncStatus = "Progressing" }, []runtime.Object{checkApplication("a", checkRepo, checkDigest)}, nil, `"Progressing" is neither Synced nor OutOfSync`},
		{"a newer Release with no status", func(s *ConfigHubLiveStatusEvidence) { s.SupersededByRelease = "payments-prod#3" }, []runtime.Object{checkApplication("a", checkRepo, otherDigest)}, nil, "a newer Release, payments-prod#3, has no recorded status"},
		{"a controller that reconciled after the status was recorded", func(s *ConfigHubLiveStatusEvidence) {
			s.ObservedAt = checkNow.Add(-5 * time.Minute).Format(time.RFC3339)
		}, []runtime.Object{checkApplication("a", checkRepo, otherDigest)}, nil, "reconciled after the status was recorded"},
		{"a kind that could not be read beside one that agrees", nil, []runtime.Object{repository, applied}, forbidden, "not every controller kind could be read; Argo CD Applications could not be listed"},
		{"a multi-source Application", nil, []runtime.Object{multi}, nil, "1 of 1 controllers pulling this space gave no settled revision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statuses := checkStatus("Synced")
			if tc.change != nil {
				tc.change(&statuses[0])
			}
			client := checkClient(tc.objects...)
			if tc.react != nil {
				tc.react(client)
			}
			checkLiveStatusesAgainstCluster(context.Background(), client, statuses, checkNow)
			check := statuses[0].ClusterCheck
			require.Equal(t, liveStatusClusterNotObservable, check.State, check.Reason)
			require.Contains(t, check.Reason, tc.reason)
		})
	}
}

// A kind that is not installed is not a kind that could not be read.
func TestClusterCheckTreatsAMissingKindAsAbsent(t *testing.T) {
	noRegistryLookup(t)
	client := checkClient(checkApplication("a", checkRepo, checkDigest))
	client.PrependReactor("list", "ocirepositories", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "source.toolkit.fluxcd.io", Resource: "ocirepositories"}, "")
	})
	statuses := checkStatus("Synced")
	checkLiveStatusesAgainstCluster(context.Background(), client, statuses, checkNow)
	require.Equal(t, liveStatusClusterAgrees, statuses[0].ClusterCheck.State, statuses[0].ClusterCheck.Reason)
	require.NotContains(t, statuses[0].ClusterCheck.Reason, "could not be listed")
}

// The recorded cluster of the Live Status Proof lane
// (test/fixtures/confighub-live-status-flux-v01011-recorded): `cub cluster up
// --controller flux` against a local ConfigHub. The cluster pulls from the
// Docker bridge address while the server advertises localhost, so the registry
// does not identify the source. The status names the Kustomization it read,
// and that object pulls a repository named for the space: that is enough to
// compare the two reports, and they agree.
func TestClusterCheckOnTheRecordedFluxClusterUsesTheNamedDataSource(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "fixtures", "confighub-live-status-flux-v01011-recorded")
	releases, err := os.ReadFile(filepath.Join(dir, "release-list.json"))
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "flux-objects.json"))
	require.NoError(t, err)
	var list unstructured.UnstructuredList
	require.NoError(t, list.UnmarshalJSON(raw))
	objects := make([]runtime.Object, 0, len(list.Items))
	for i := range list.Items {
		objects = append(objects, &list.Items[i])
	}
	require.Len(t, objects, 4)
	// The moment cub-scout read the cluster in that run.
	now := time.Date(2026, 11, 10, 18, 8, 51, 0, time.UTC)

	for _, tc := range []struct {
		name       string
		advertised string
		matchedBy  string
	}{
		// As recorded: the server advertises localhost:32181, the cluster
		// pulls from 172.18.0.1:32181.
		{"the advertised registry is not the one the cluster pulls from", "localhost:32181", liveStatusMatchedByDataSource},
		{"no registry is advertised at all", "", liveStatusMatchedByDataSource},
		// Where the address is the advertised one, the registry settles it.
		{"the cluster pulls from the advertised registry", "172.18.0.1:32181", liveStatusMatchedByRegistry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := agent.ConfigHubOCIRegistryFn
			agent.ConfigHubOCIRegistryFn = func() string { return tc.advertised }
			t.Cleanup(func() { agent.ConfigHubOCIRegistryFn = old })

			statuses, omissions := configHubLiveStatusFromReleases(string(releases), now, 15*time.Minute)
			require.Empty(t, omissions)
			require.Len(t, statuses, 2)
			checkLiveStatusesAgainstCluster(context.Background(), checkClient(objects...), statuses, now)
			for _, status := range statuses {
				check := status.ClusterCheck
				require.NotNil(t, check, status.Release)
				require.Equal(t, liveStatusClusterAgrees, check.State, check.Reason)
				require.Equal(t, tc.matchedBy, check.MatchedBy)
				require.Len(t, check.Controllers, 1)
				require.Equal(t, "Kustomization/flux-system/"+check.Controllers[0].Name, status.DataSource)
				require.Equal(t, "match", check.Controllers[0].Revision.Comparison)
				if tc.matchedBy == liveStatusMatchedByDataSource {
					require.Contains(t, check.Reason, "names as its data source")
				}
			}
		})
	}
}

// A repository named for the space on an unrecognised registry is compared only
// when the recorded status names that very object. A lookalike, or a status
// that names something else, stays not observable, and says what was seen.
func TestClusterCheckDoesNotFollowADataSourceItCannotCorroborate(t *testing.T) {
	noRegistryLookup(t)
	lookalike := func() (*unstructured.Unstructured, *unstructured.Unstructured) {
		repository, kustomization := checkFlux(checkDigest)
		require.NoError(t, unstructured.SetNestedField(repository.Object, "oci://172.18.0.1:32181/space/payments-prod", "spec", "url"))
		return repository, kustomization
	}
	for _, tc := range []struct {
		name       string
		dataSource string
		space      string
		want       string
		reason     string
	}{
		{"the status names the object", "Kustomization/flux-system/confighub-root", "payments-prod", liveStatusClusterAgrees, "names as its data source"},
		{"the status names no data source", "", "payments-prod", liveStatusClusterNotObservable, "names none of them as its data source"},
		{"the status names another object", "Kustomization/flux-system/other", "payments-prod", liveStatusClusterNotObservable, "172.18.0.1:32181"},
		{"the status names another kind", "Application/flux-system/confighub-root", "payments-prod", liveStatusClusterNotObservable, "names none of them"},
		// A bare name is an Argo CD Application's, and this is a Kustomization.
		{"the status gives a bare name and the object is not an Application", "confighub-root", "payments-prod", liveStatusClusterNotObservable, "names none of them"},
		{"the status is malformed", "Kustomization//confighub-root", "payments-prod", liveStatusClusterNotObservable, "names none of them"},
		{"the named object pulls another space's repository", "Kustomization/flux-system/confighub-root", "another-space", liveStatusClusterNotObservable, `pulling ConfigHub space "another-space"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository, kustomization := lookalike()
			statuses := checkStatus("Synced")
			statuses[0].DataSource, statuses[0].Space = tc.dataSource, tc.space
			checkLiveStatusesAgainstCluster(context.Background(), checkClient(repository, kustomization), statuses, checkNow)
			check := statuses[0].ClusterCheck
			require.Equal(t, tc.want, check.State, check.Reason)
			require.Contains(t, check.Reason, tc.reason)
			if tc.want != liveStatusClusterAgrees {
				require.Empty(t, check.MatchedBy)
				require.Empty(t, check.Controllers)
			}
		})
	}
}

// The same lane with `cub cluster up --controller argo`
// (test/fixtures/confighub-live-status-argo-v01011-recorded). In Argo CD mode
// argobot names its data source by the Application's name alone.
func TestClusterCheckOnTheRecordedArgoClusterUsesTheNamedApplication(t *testing.T) {
	old := agent.ConfigHubOCIRegistryFn
	agent.ConfigHubOCIRegistryFn = func() string { return "localhost:32181" }
	t.Cleanup(func() { agent.ConfigHubOCIRegistryFn = old })
	dir := filepath.Join("..", "..", "test", "fixtures", "confighub-live-status-argo-v01011-recorded")
	releases, err := os.ReadFile(filepath.Join(dir, "release-list.json"))
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "argo-objects.json"))
	require.NoError(t, err)
	var list unstructured.UnstructuredList
	require.NoError(t, list.UnmarshalJSON(raw))
	objects := make([]runtime.Object, 0, len(list.Items))
	for i := range list.Items {
		objects = append(objects, &list.Items[i])
	}
	require.Len(t, objects, 2)
	// Seconds after argobot recorded the statuses in that run.
	now := time.Date(2026, 11, 10, 18, 49, 40, 0, time.UTC)

	statuses, omissions := configHubLiveStatusFromReleases(string(releases), now, 15*time.Minute)
	require.Empty(t, omissions)
	require.Len(t, statuses, 2)
	checkLiveStatusesAgainstCluster(context.Background(), checkClient(objects...), statuses, now)
	for _, status := range statuses {
		check := status.ClusterCheck
		require.NotNil(t, check, status.Release)
		require.Equal(t, liveStatusClusterAgrees, check.State, check.Reason)
		require.Equal(t, liveStatusMatchedByDataSource, check.MatchedBy)
		require.Len(t, check.Controllers, 1)
		require.Equal(t, "Application", check.Controllers[0].Kind)
		require.Equal(t, status.DataSource, check.Controllers[0].Name)
		require.Equal(t, "match", check.Controllers[0].Revision.Comparison, check.Controllers[0].Revision.Reason)
	}
}

// A bare name that two Applications share does not say which was read.
func TestClusterCheckDoesNotGuessBetweenTwoApplicationsOfOneName(t *testing.T) {
	noRegistryLookup(t)
	const repo = "oci://172.18.0.1:32181/space/payments-prod"
	first, second := checkApplication("payments", repo, checkDigest), checkApplication("payments", repo, checkDigest)
	second.SetNamespace("argocd-two")
	statuses := checkStatus("Synced")
	statuses[0].DataSource = "payments"
	checkLiveStatusesAgainstCluster(context.Background(), checkClient(first, second), statuses, checkNow)
	require.Equal(t, liveStatusClusterNotObservable, statuses[0].ClusterCheck.State)
	require.Contains(t, statuses[0].ClusterCheck.Reason, "2 pull a repository named space/payments-prod")

	one := checkStatus("Synced")
	one[0].DataSource = "payments"
	checkLiveStatusesAgainstCluster(context.Background(), checkClient(first), one, checkNow)
	require.Equal(t, liveStatusClusterAgrees, one[0].ClusterCheck.State, one[0].ClusterCheck.Reason)
	require.Equal(t, liveStatusMatchedByDataSource, one[0].ClusterCheck.MatchedBy)
}

func checkClusterProfile(name, url string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "config.projectsveltos.io/v1beta1", "kind": "ClusterProfile",
		"metadata": map[string]interface{}{"name": name, "uid": "uid-" + name, "generation": int64(1)},
		"spec": map[string]interface{}{"policyRefs": []interface{}{
			map[string]interface{}{"kind": "ConfigMap", "name": "unrelated", "namespace": "default"},
			map[string]interface{}{"deploymentType": "Remote", "remoteURL": map[string]interface{}{"url": url, "interval": "10m0s"}},
		}},
	}}
}

// Sveltos gives no digest for what it applied, so a ClusterProfile pulling the
// space is named and never compared, and agreement is not claimed over it.
func TestClusterCheckNamesASveltosProfileAndDoesNotCompareIt(t *testing.T) {
	noRegistryLookup(t)
	profile := checkClusterProfile("payments-prod", checkRepo+":latest")
	for _, tc := range []struct {
		name    string
		sync    string
		objects []runtime.Object
		want    string
		reason  string
		rows    int
	}{
		{"Sveltos alone", "Synced", []runtime.Object{profile}, liveStatusClusterNotObservable, "1 of 1 controllers pulling this space are Sveltos ClusterProfiles (payments-prod): Sveltos reports a hash of its own", 1},
		{"Sveltos alone, recorded OutOfSync", "OutOfSync", []runtime.Object{profile}, liveStatusClusterNotObservable, "Sveltos reports a hash of its own", 1},
		{"Argo CD agrees, and Sveltos pulls the space too", "Synced", []runtime.Object{profile, checkApplication("payments", checkRepo, checkDigest)}, liveStatusClusterNotObservable, "1 of 2 controllers pulling this space are Sveltos ClusterProfiles", 2},
		{"Argo CD disagrees, and Sveltos pulls the space too", "Synced", []runtime.Object{profile, checkApplication("payments", checkRepo, otherDigest)}, liveStatusClusterDisagrees, "1 of 2 controllers", 2},
		{"a profile for another space is left out", "Synced", []runtime.Object{checkClusterProfile("other", "oci://oci.api.confighub.com/space/another:latest"), checkApplication("payments", checkRepo, checkDigest)}, liveStatusClusterAgrees, "all 1 controllers", 1},
		{"a profile on a registry not known to be ConfigHub's", "Synced", []runtime.Object{checkClusterProfile("payments-prod", "oci://172.18.0.1:32181/space/payments-prod:latest")}, liveStatusClusterNotObservable, "1 Sveltos ClusterProfiles (payments-prod) pull a repository named space/payments-prod from a registry not known to be ConfigHub's", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statuses := checkStatus(tc.sync)
			statuses[0].HealthStatus = "Healthy"
			checkLiveStatusesAgainstCluster(context.Background(), checkClient(tc.objects...), statuses, checkNow)
			check := statuses[0].ClusterCheck
			require.Equal(t, tc.want, check.State, check.Reason)
			require.Contains(t, check.Reason, tc.reason)
			require.Len(t, check.Controllers, tc.rows)
			require.Contains(t, check.Limitation, "Sveltos reports no digest")
			if tc.want != liveStatusClusterAgrees {
				require.Equal(t, liveStatusClusterNotObservable, check.Health.State)
			}
		})
	}
}

// The cluster `cub cluster up --controller sveltos` made, as recorded in CI
// run 43414463943: argobot says Synced, and nothing Sveltos wrote carries the
// digest it pulled. The answer names the ClusterProfile and compares nothing,
// whether or not the registry address is one the server advertises.
func TestClusterCheckOnTheRecordedSveltosClusterComparesNothing(t *testing.T) {
	dir := filepath.Join("..", "..", "test", "fixtures", "confighub-live-status-sveltos-v01011-recorded")
	releases, err := os.ReadFile(filepath.Join(dir, "release-list.json"))
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(dir, "sveltos-objects.json"))
	require.NoError(t, err)
	var list unstructured.UnstructuredList
	require.NoError(t, list.UnmarshalJSON(raw))
	var objects []runtime.Object
	for i := range list.Items {
		if list.Items[i].GetKind() == "ClusterProfile" {
			objects = append(objects, &list.Items[i])
		}
		// No object of any kind Sveltos wrote holds the Release's digest.
		text, err := list.Items[i].MarshalJSON()
		require.NoError(t, err)
		require.NotContains(t, string(text), "47b52b0aa8be0c55dca5952979832091e7ead210ec940fb9e4365ecb60df39d9")
	}
	require.Len(t, objects, 2)
	// Seconds after argobot recorded the status in that run.
	now := time.Date(2026, 11, 13, 11, 23, 5, 0, time.UTC)
	for _, tc := range []struct{ name, registry, reason, matchedBy string }{
		{"the address the cluster uses is not the one advertised", "localhost:32181", "1 Sveltos ClusterProfiles (argobot-scout-live) pull a repository named space/argobot-scout-live from a registry not known to be ConfigHub's", ""},
		{"the address is the one advertised", "172.18.0.1:32181", "1 of 1 controllers pulling this space are Sveltos ClusterProfiles (argobot-scout-live)", liveStatusMatchedByRegistry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := agent.ConfigHubOCIRegistryFn
			agent.ConfigHubOCIRegistryFn = func() string { return tc.registry }
			t.Cleanup(func() { agent.ConfigHubOCIRegistryFn = old })
			statuses, omissions := configHubLiveStatusFromReleases(string(releases), now, 15*time.Minute)
			require.Empty(t, omissions)
			// Only the argobot Release carries a status; the apps space's has none.
			require.Len(t, statuses, 1)
			require.Equal(t, "Synced", statuses[0].SyncStatus)
			checkLiveStatusesAgainstCluster(context.Background(), checkClient(objects...), statuses, now)
			check := statuses[0].ClusterCheck
			require.Equal(t, liveStatusClusterNotObservable, check.State, check.Reason)
			require.Contains(t, check.Reason, tc.reason)
			require.Contains(t, check.Reason, "Sveltos reports a hash of its own")
			require.Equal(t, tc.matchedBy, check.MatchedBy)
			require.Equal(t, liveStatusClusterNotObservable, check.Health.State)
		})
	}
}
