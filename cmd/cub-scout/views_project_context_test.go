package main

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// #599: views project --with-reality reads live workloads. With
// --kube-context the index is read through the selected context only.
func TestViewsRealityIndexReadsOnlyTheGivenConfig(t *testing.T) {
	selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
	t.Setenv("KUBECONFIG", path)

	binding := resolveLocalClusterBindingForSelection(clusterContextSelection{name: "selected", explicit: true})
	require.NoError(t, binding.err)
	_, err := buildWorkloadIndexFromCluster(binding.config)
	require.NoError(t, err)
	require.NotZero(t, selected.requests.Load(), "the selected context was not read")
	require.Zero(t, ambient.requests.Load(), "the ambient context was read: %v", ambient.requestPaths())

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "the kubeconfig was modified")
}

// The config buildProjectedView is given is the one the overlay reads.
func TestViewsProjectPassesTheRealityConfigThrough(t *testing.T) {
	const viewUUID = "11111111-2222-3333-4444-555555555555"
	fakeProjectionRunner(t, map[string]string{
		"view get " + viewUUID: fakeViewWithColumns(viewUUID, "metadata.labels.team = 'payments'", []map[string]interface{}{
			{"Name": "Slug", "ColumnSource": map[string]interface{}{"MetadataAttribute": "slug"}},
		}),
		"unit list": fakeUnitListJSONFromMaps([]map[string]interface{}{{"slug": "payment-api"}}),
	})
	var got *rest.Config
	orig := buildWorkloadIndexFn
	buildWorkloadIndexFn = func(config *rest.Config) (map[string]WorkloadInfo, error) {
		got = config
		return map[string]WorkloadInfo{}, nil
	}
	t.Cleanup(func() { buildWorkloadIndexFn = orig })

	want := &rest.Config{Host: "https://selected.invalid"}
	_, err := buildProjectedView(context.Background(), mockViewRef(viewUUID), "*", true, want)
	require.NoError(t, err)
	require.Same(t, want, got, "the overlay read another config than the one selected")
}

// --kube-context is refused before any read when it is empty, unknown, or
// given without --with-reality, which is the only part that reads a cluster.
func TestViewsProjectRefusesABadContextBeforeAnyRead(t *testing.T) {
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": "https://127.0.0.1:1", "ambient": "https://127.0.0.1:2"})
	t.Setenv("KUBECONFIG", path)
	called := false
	orig := buildWorkloadIndexFn
	buildWorkloadIndexFn = func(*rest.Config) (map[string]WorkloadInfo, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() { buildWorkloadIndexFn = orig })

	cmd := viewsProjectCmd
	flag := cmd.Flags().Lookup("kube-context")
	require.NotNil(t, flag)
	oldReality := viewsProjectWithReality
	t.Cleanup(func() {
		viewsProjectWithReality = oldReality
		_ = flag.Value.Set("")
		flag.Changed = false
	})
	cmd.SetContext(context.Background())
	const ref = "11111111-2222-3333-4444-555555555555"

	cases := []struct {
		context     string
		withReality bool
		want        string
	}{
		{"selected", false, "--kube-context applies to --with-reality"},
		{"missing", true, "missing"},
		{"", true, "non-empty"},
	}
	for _, tc := range cases {
		viewsProjectWithReality = tc.withReality
		require.NoError(t, cmd.Flags().Set("kube-context", tc.context))
		err := runViewsProject(cmd, []string{ref})
		require.Error(t, err, "%+v", tc)
		require.Contains(t, err.Error(), tc.want)
	}
	require.False(t, called, "the cluster was read before a refusal")
}

// End to end through the command: views project --with-reality
// --kube-context selected hands the overlay the selected context's config.
func TestViewsProjectWithRealityUsesTheSelectedContext(t *testing.T) {
	selected := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": "https://127.0.0.1:2"})
	t.Setenv("KUBECONFIG", path)
	stubConnectedGate(t, nil)

	const viewUUID = "11111111-2222-3333-4444-555555555555"
	fakeProjectionRunner(t, map[string]string{
		"view get " + viewUUID: fakeViewWithColumns(viewUUID, "metadata.labels.team = 'payments'", []map[string]interface{}{
			{"Name": "Slug", "ColumnSource": map[string]interface{}{"MetadataAttribute": "slug"}},
		}),
		"unit list": fakeUnitListJSONFromMaps([]map[string]interface{}{{"slug": "payment-api"}}),
	})
	var got *rest.Config
	orig := buildWorkloadIndexFn
	buildWorkloadIndexFn = func(config *rest.Config) (map[string]WorkloadInfo, error) {
		got = config
		return map[string]WorkloadInfo{}, nil
	}
	t.Cleanup(func() { buildWorkloadIndexFn = orig })

	cmd := viewsProjectCmd
	flag := cmd.Flags().Lookup("kube-context")
	oldReality, oldFormat := viewsProjectWithReality, viewsProjectFormat
	t.Cleanup(func() {
		viewsProjectWithReality, viewsProjectFormat = oldReality, oldFormat
		_ = flag.Value.Set("")
		flag.Changed = false
	})
	viewsProjectWithReality, viewsProjectFormat = true, "json"
	require.NoError(t, cmd.Flags().Set("kube-context", "selected"))
	cmd.SetContext(context.Background())

	var err error
	captureStdout(t, func() { err = runViewsProject(cmd, []string{viewUUID}) })
	require.NoError(t, err)
	require.NotNil(t, got, "the overlay was given no config, so it would read the current context")
	require.Equal(t, selected.server.URL, got.Host)
}
