// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestControllerRevision(t *testing.T) {
	commit := strings.Repeat("a", 40)
	digest := "sha256:" + strings.Repeat("b", 64)
	now := time.Date(2026, 9, 11, 12, 1, 0, 0, time.UTC)
	set := func(value interface{}, path ...string) func(*unstructured.Unstructured) {
		return func(obj *unstructured.Unstructured) {
			require.NoError(t, unstructured.SetNestedField(obj.Object, value, path...))
		}
	}
	remove := func(path ...string) func(*unstructured.Unstructured) {
		return func(obj *unstructured.Unstructured) { unstructured.RemoveNestedField(obj.Object, path...) }
	}
	for _, tc := range []struct {
		name, fixture, expected, comparison string
		change                              func(*unstructured.Unstructured)
	}{
		{"application match despite unhealthy app", "application", commit, "match", nil},
		{"application mismatch", "application", strings.Repeat("b", 40), "mismatch", nil},
		{"application missing report", "application", commit, "unknown", remove("status", "sync", "revision")},
		{"application out of sync", "application", commit, "unknown", set("OutOfSync", "status", "sync", "status")},
		{"application compared source changed", "application", commit, "unknown", set("new", "spec", "source", "path")},
		{"application compared destination changed", "application", commit, "unknown", set("other", "spec", "destination", "namespace")},
		{"application missing comparison", "application", commit, "unknown", remove("status", "sync", "comparedTo")},
		{"application ignore policy changed", "application", commit, "unknown", set([]interface{}{map[string]interface{}{"kind": "Secret"}}, "spec", "ignoreDifferences")},
		{"application multi source", "application", commit, "unknown", set([]interface{}{map[string]interface{}{}}, "spec", "sources")},
		{"application multi report", "application", commit, "unknown", set([]interface{}{commit}, "status", "sync", "revisions")},
		{"application malformed sources", "application", commit, "unknown", set("invalid", "spec", "sources")},
		{"application hydrated", "application", commit, "unknown", set(map[string]interface{}{}, "spec", "sourceHydrator")},
		{"application pending", "application", commit, "unknown", set(map[string]interface{}{}, "operation")},
		{"application running", "application", commit, "unknown", set("Running", "status", "operationState", "phase")},
		{"application failed", "application", commit, "unknown", set("Failed", "status", "operationState", "phase")},
		{"application malformed phase", "application", commit, "unknown", set(int64(1), "status", "operationState", "phase")},
		{"application condition", "application", commit, "unknown", set([]interface{}{map[string]interface{}{"type": "ComparisonError"}}, "status", "conditions")},
		{"application missing time", "application", commit, "unknown", remove("status", "reconciledAt")},
		{"application future time", "application", commit, "unknown", set(now.Add(time.Nanosecond).Format(time.RFC3339Nano), "status", "reconciledAt")},
		{"application exact time", "application", commit, "match", set(now.Format(time.RFC3339Nano), "status", "reconciledAt")},
		{"application exact age", "application", commit, "match", set(now.Add(-15*time.Minute).Format(time.RFC3339Nano), "status", "reconciledAt")},
		{"application stale", "application", commit, "unknown", set(now.Add(-15*time.Minute-time.Nanosecond).Format(time.RFC3339Nano), "status", "reconciledAt")},
		{"application invalid time", "application", commit, "unknown", set("yesterday", "status", "reconciledAt")},
		{"application zero time", "application", commit, "unknown", set("0001-01-01T00:00:00Z", "status", "reconciledAt")},
		{"application short hash", "application", commit, "unknown", set("aaaaaaa", "status", "sync", "revision")},
		{"application mutable revision", "application", commit, "unknown", set("main", "status", "sync", "revision")},
		{"application wrong identifier type", "application", digest, "unknown", nil},
		{"application missing uid", "application", commit, "unknown", remove("metadata", "uid")},
		{"application missing generation", "application", commit, "unknown", remove("metadata", "generation")},
		{"application deleted", "application", commit, "unknown", set(now.Format(time.RFC3339), "metadata", "deletionTimestamp")},
		{"kustomization match", "kustomization", commit, "match", nil},
		{"kustomization mismatch", "kustomization", strings.Repeat("b", 40), "mismatch", nil},
		{"kustomization unobserved", "kustomization", commit, "unknown", set(int64(1), "status", "observedGeneration")},
		{"kustomization future generation", "kustomization", commit, "unknown", set(int64(3), "status", "observedGeneration")},
		{"kustomization generation missing", "kustomization", commit, "unknown", remove("status", "observedGeneration")},
		{"kustomization suspended", "kustomization", commit, "unknown", set(true, "spec", "suspend")},
		{"kustomization missing conditions", "kustomization", commit, "unknown", remove("status", "conditions")},
		{"kustomization stale ready", "kustomization", commit, "unknown", set([]interface{}{map[string]interface{}{"type": "Ready", "status": "True", "observedGeneration": int64(1)}}, "status", "conditions")},
		{"kustomization ready false", "kustomization", commit, "unknown", set([]interface{}{map[string]interface{}{"type": "Ready", "status": "False", "observedGeneration": int64(2)}}, "status", "conditions")},
		{"kustomization attempted differs", "kustomization", commit, "unknown", set("main@sha1:"+strings.Repeat("b", 40), "status", "lastAttemptedRevision")},
		{"kustomization attempted malformed", "kustomization", commit, "unknown", set(int64(1), "status", "lastAttemptedRevision")},
		{"kustomization source missing", "kustomization", commit, "unknown", remove("spec", "sourceRef", "name")},
		{"kustomization unsupported source", "kustomization", commit, "unknown", set("Bucket", "spec", "sourceRef", "kind")},
		{"kustomization deleted", "kustomization", commit, "unknown", set(now.Format(time.RFC3339), "metadata", "deletionTimestamp")},
		{"custom kind collision", "kustomization", commit, "unknown", set("other.io/v1", "apiVersion")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile("../../examples/controller-revision/" + tc.fixture + ".json")
			require.NoError(t, err)
			obj := &unstructured.Unstructured{}
			require.NoError(t, obj.UnmarshalJSON(data))
			if tc.change != nil {
				tc.change(obj)
			}
			before := obj.DeepCopy()
			e := BuildControllerRevisionEvidence(obj, tc.expected, now)
			require.Equal(t, tc.comparison, e.Comparison, "%+v", e)
			require.Equal(t, tc.expected, e.ExpectedRevision)
			require.NotEmpty(t, e.Reason)
			require.Equal(t, before, obj, "comparison must not mutate its input")
		})
	}
}

func TestControllerRevisionOCIAndAmbiguity(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	now := time.Date(2026, 9, 11, 12, 1, 0, 0, time.UTC)
	for _, kind := range []string{"application", "kustomization"} {
		data, err := os.ReadFile("../../examples/controller-revision/" + kind + ".json")
		require.NoError(t, err)
		obj := &unstructured.Unstructured{}
		require.NoError(t, obj.UnmarshalJSON(data))
		if kind == "application" {
			for _, path := range [][]string{{"spec", "source", "repoURL"}, {"status", "sync", "comparedTo", "source", "repoURL"}} {
				require.NoError(t, unstructured.SetNestedField(obj.Object, "oci://example.invalid/config", path...))
			}
			require.NoError(t, unstructured.SetNestedField(obj.Object, digest, "status", "sync", "revision"))
		} else {
			require.NoError(t, unstructured.SetNestedField(obj.Object, "OCIRepository", "spec", "sourceRef", "kind"))
			for _, key := range []string{"lastAppliedRevision", "lastAttemptedRevision"} {
				require.NoError(t, unstructured.SetNestedField(obj.Object, "v1@"+digest, "status", key))
			}
			require.NoError(t, unstructured.SetNestedField(obj.Object, strings.Repeat("a", 40), "status", "lastAppliedOriginRevision"))
		}
		e := BuildControllerRevisionEvidence(obj, digest, now)
		require.Equal(t, "match", e.Comparison, "%+v", e)
		require.Equal(t, digest, e.Revision)
		if kind == "kustomization" {
			require.Empty(t, e.ReportedAt, "Ready transition time is not a reconciliation heartbeat")
			conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
			for _, extra := range []interface{}{conditions[0], map[string]interface{}{"type": "Reconciling", "status": "True"}, map[string]interface{}{"type": "Stalled", "status": "True"}, "malformed"} {
				broken := obj.DeepCopy()
				require.NoError(t, unstructured.SetNestedSlice(broken.Object, append(conditions, extra), "status", "conditions"))
				require.Equal(t, "unknown", BuildControllerRevisionEvidence(broken, digest, now).Comparison)
			}
		}
	}
}

func TestValidateExpectedRevision(t *testing.T) {
	for _, value := range []string{"", "main", "v1.0", "aaaaaaa", strings.Repeat("A", 40), strings.Repeat("a", 64), "sha256:abc", "\n" + strings.Repeat("a", 40), "$(id)"} {
		require.Error(t, ValidateExpectedRevision(value), "%q", value)
	}
	require.NoError(t, ValidateExpectedRevision(strings.Repeat("a", 40)))
	require.NoError(t, ValidateExpectedRevision("sha256:"+strings.Repeat("a", 64)))
	require.Equal(t, "unknown", BuildControllerRevisionEvidence(nil, strings.Repeat("a", 40), time.Now()).Comparison)
}
