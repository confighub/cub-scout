// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	digestNew = "sha256:" + strings.Repeat("a", 64)
	digestOld = "sha256:" + strings.Repeat("b", 64)
)

func desiredWorkload(kind string, containers ...[2]string) *unstructured.Unstructured {
	items := make([]interface{}, 0, len(containers))
	for _, c := range containers {
		items = append(items, map[string]interface{}{"name": c[0], "image": c[1]})
	}
	spec := map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{"containers": items}}}
	if kind == "Pod" {
		spec = map[string]interface{}{"containers": items}
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": map[string]string{"Pod": "v1"}[kind],
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": "api", "namespace": "delivery"},
		"spec":       spec,
	}}
}

func runningPod(statuses ...[2]string) *unstructured.Unstructured {
	cs := make([]interface{}, 0, len(statuses))
	for _, s := range statuses {
		cs = append(cs, map[string]interface{}{"name": s[0], "imageID": s[1]})
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{"name": "api-xyz", "namespace": "delivery"},
		"status":   map[string]interface{}{"containerStatuses": cs},
	}}
}

func deployment(containers ...[2]string) *unstructured.Unstructured {
	o := desiredWorkload("Deployment", containers...)
	o.SetAPIVersion("apps/v1")
	return o
}

func TestParseImageReference(t *testing.T) {
	for _, tc := range []struct {
		in                   string
		repo, tag, digest    string
	}{
		{"example.invalid/api@" + digestNew, "example.invalid/api", "", digestNew},
		{"example.invalid/api:v1", "example.invalid/api", "v1", ""},
		{"example.invalid/api:v1@" + digestNew, "example.invalid/api", "v1", digestNew},
		{"example.invalid/api", "example.invalid/api", "", ""},
		{"registry:5000/team/api:v2", "registry:5000/team/api", "v2", ""},
		{"api@sha256:notavaliddigest!", "api", "", ""},
	} {
		repo, tag, digest := ParseImageReference(tc.in)
		require.Equal(t, tc.repo, repo, tc.in)
		require.Equal(t, tc.tag, tag, tc.in)
		require.Equal(t, tc.digest, digest, tc.in)
	}
}

func TestDigestFromImageID(t *testing.T) {
	for _, tc := range []struct{ in, repo, digest string }{
		{"docker-pullable://example.invalid/api@" + digestNew, "example.invalid/api", digestNew},
		{"example.invalid/api@" + digestNew, "example.invalid/api", digestNew},
		{digestNew, "", digestNew},
		{"example.invalid/api:v1", "example.invalid/api:v1", ""},
		{"", "", ""},
	} {
		repo, digest := DigestFromImageID(tc.in)
		require.Equal(t, tc.repo, repo, tc.in)
		require.Equal(t, tc.digest, digest, tc.in)
	}
}

func TestBuildRunningImageWorkloadMatch(t *testing.T) {
	w := BuildRunningImageWorkload(
		deployment([2]string{"api", "example.invalid/api@" + digestNew}),
		[]*unstructured.Unstructured{runningPod([2]string{"api", "docker-pullable://example.invalid/api@" + digestNew})},
		false, "")
	require.Equal(t, "match", w.Verdict)
	require.Equal(t, 1, w.PodsRead)
	require.Equal(t, "match", w.Containers[0].Verdict)
	require.Empty(t, w.Containers[0].Caveat)
}

func TestBuildRunningImageWorkloadMismatch(t *testing.T) {
	w := BuildRunningImageWorkload(
		deployment([2]string{"api", "example.invalid/api@" + digestNew}),
		[]*unstructured.Unstructured{runningPod([2]string{"api", "example.invalid/api@" + digestOld})},
		false, "")
	require.Equal(t, "mismatch", w.Verdict)
	require.Equal(t, "mismatch", w.Containers[0].Verdict)
	require.Contains(t, w.Containers[0].Caveat, "Multi-architecture")
	require.Equal(t, []string{digestOld}, w.Containers[0].RunningDigests)
}

func TestBuildRunningImageWorkloadMutableTag(t *testing.T) {
	w := BuildRunningImageWorkload(deployment([2]string{"api", "example.invalid/api:v1"}), nil, false, "")
	require.Equal(t, "unknown", w.Verdict)
	require.Equal(t, "mutable-tag", w.Containers[0].Reason)
}

func TestBuildRunningImageWorkloadMultiContainer(t *testing.T) {
	w := BuildRunningImageWorkload(
		deployment([2]string{"api", "example.invalid/api@" + digestNew}, [2]string{"sidecar", "example.invalid/side@" + digestNew}),
		[]*unstructured.Unstructured{runningPod(
			[2]string{"api", "example.invalid/api@" + digestOld},
			[2]string{"sidecar", "example.invalid/side@" + digestNew})},
		false, "")
	require.Equal(t, "mismatch", w.Verdict, "weakest container wins")
	require.Equal(t, "mismatch", w.Containers[0].Verdict)
	require.Equal(t, "match", w.Containers[1].Verdict)
}

func TestBuildRunningImageWorkloadMultiPodRollout(t *testing.T) {
	w := BuildRunningImageWorkload(
		deployment([2]string{"api", "example.invalid/api@" + digestNew}),
		[]*unstructured.Unstructured{
			runningPod([2]string{"api", "example.invalid/api@" + digestNew}),
			runningPod([2]string{"api", "example.invalid/api@" + digestOld}),
		},
		false, "")
	require.Equal(t, "mismatch", w.Verdict, "a single old pod prevents a clean match")
}

func TestBuildRunningImageWorkloadDegradation(t *testing.T) {
	pinned := deployment([2]string{"api", "example.invalid/api@" + digestNew})
	for _, tc := range []struct {
		name    string
		pods    []*unstructured.Unstructured
		capped  bool
		readErr string
		verdict string
		reason  string
	}{
		{"read-denied", nil, false, "read-denied", "unknown", "read-denied"},
		{"no-running-pods", nil, false, "", "unknown", "no-running-pods"},
		{"container-not-found", []*unstructured.Unstructured{runningPod([2]string{"other", "example.invalid/other@" + digestNew})}, false, "", "unknown", "container-not-found"},
		{"different-repository", []*unstructured.Unstructured{runningPod([2]string{"api", "other.invalid/api@" + digestOld})}, false, "", "unknown", "different-repository"},
		{"unreadable", []*unstructured.Unstructured{runningPod([2]string{"api", ""})}, false, "", "unknown", "unreadable"},
		{"coverage-capped", []*unstructured.Unstructured{runningPod([2]string{"api", "example.invalid/api@" + digestNew})}, true, "", "unknown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := BuildRunningImageWorkload(pinned, tc.pods, tc.capped, tc.readErr)
			require.Equal(t, tc.verdict, w.Verdict)
			if tc.reason != "" {
				require.Equal(t, tc.reason, w.Containers[0].Reason)
			}
		})
	}
}

func TestBuildRunningImageWorkloadBareDigestMismatch(t *testing.T) {
	w := BuildRunningImageWorkload(
		deployment([2]string{"api", "example.invalid/api@" + digestNew}),
		[]*unstructured.Unstructured{runningPod([2]string{"api", digestOld})},
		false, "")
	require.Equal(t, "mismatch", w.Verdict, "a bare-digest imageID differing from intended is a mismatch")
}

func TestAggregateRunningImage(t *testing.T) {
	match := RunningImageWorkload{Verdict: "match"}
	unknown := RunningImageWorkload{Verdict: "unknown", Reason: "mutable-tag"}
	mismatch := RunningImageWorkload{Verdict: "mismatch", Reason: "running digest differs from intended"}

	v, _ := AggregateRunningImage([]RunningImageWorkload{match, match})
	require.Equal(t, "match", v)
	v, r := AggregateRunningImage([]RunningImageWorkload{match, unknown})
	require.Equal(t, "unknown", v)
	require.Equal(t, "mutable-tag", r)
	v, r = AggregateRunningImage([]RunningImageWorkload{match, unknown, mismatch})
	require.Equal(t, "mismatch", v)
	require.Equal(t, "running digest differs from intended", r)
	v, _ = AggregateRunningImage(nil)
	require.Equal(t, "unknown", v)
}

func TestIntendedImageDigestPinned(t *testing.T) {
	require.True(t, IntendedImageDigestPinned(deployment([2]string{"api", "example.invalid/api@" + digestNew})))
	require.False(t, IntendedImageDigestPinned(deployment([2]string{"api", "example.invalid/api:v1"})))
}

func TestWorkloadSelectorLabels(t *testing.T) {
	live := deployment([2]string{"api", "example.invalid/api@" + digestNew})
	require.NoError(t, unstructured.SetNestedStringMap(live.Object, map[string]string{"app": "api"}, "spec", "selector", "matchLabels"))
	labels, ok := WorkloadSelectorLabels(live)
	require.True(t, ok)
	require.Equal(t, map[string]string{"app": "api"}, labels)

	_, ok = WorkloadSelectorLabels(deployment([2]string{"api", "example.invalid/api@" + digestNew}))
	require.False(t, ok, "no matchLabels is selector-unsupported")
}
