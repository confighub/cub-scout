// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConfigHubOriginEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want *ConfigHubOriginEvidence
	}{
		{"complete", `{"spaceId":"space-a","spaceSlug":"team-a","unitId":"unit-a","unitSlug":"api","revisionNum":7}`, &ConfigHubOriginEvidence{Source: "annotation:confighub.com/origin", SpaceID: "space-a", SpaceSlug: "team-a", UnitID: "unit-a", UnitSlug: "api", RevisionNum: originRevision(7)}},
		{"minimal", `{"spaceId":"space-a","unitSlug":"api"}`, &ConfigHubOriginEvidence{Source: "annotation:confighub.com/origin", SpaceID: "space-a", UnitSlug: "api"}},
		{"zero-revision", `{"spaceId":"space-a","unitSlug":"api","revisionNum":0}`, &ConfigHubOriginEvidence{Source: "annotation:confighub.com/origin", SpaceID: "space-a", UnitSlug: "api", RevisionNum: originRevision(0)}},
		{"large-exact-integer", `{"spaceId":"space-a","unitSlug":"api","revisionNum":9007199254740993}`, &ConfigHubOriginEvidence{Source: "annotation:confighub.com/origin", SpaceID: "space-a", UnitSlug: "api", RevisionNum: originRevision(9007199254740993)}},
		{"optional-empty", `{"spaceId":"space-a","spaceSlug":"","unitId":"","unitSlug":"api"}`, &ConfigHubOriginEvidence{Source: "annotation:confighub.com/origin", SpaceID: "space-a", UnitSlug: "api"}},
		{"future-fields", `{"spaceId":"space-a","unitSlug":"api","token":"sensitive-do-not-emit","targetId":"unverified","component":"unverified"}`, &ConfigHubOriginEvidence{Source: "annotation:confighub.com/origin", SpaceID: "space-a", UnitSlug: "api"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetAnnotations(map[string]string{ConfigHubOriginAnnotation: tc.raw})
			before := obj.DeepCopy()
			got, omission := BuildConfigHubOriginEvidence(obj)
			require.Nil(t, omission)
			require.Equal(t, tc.want, got)
			require.Equal(t, before, obj)
			encoded, err := json.Marshal(got)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "sensitive")
			require.NotContains(t, string(encoded), "unverified")
			require.Contains(t, got.Summary(), "spaceId=space-a")
			require.Contains(t, got.Summary(), "unitSlug=api")
		})
	}
}

func originRevision(value int64) *int64 { return &value }

func TestConfigHubOriginRejectsAmbiguousOrMalformedEvidence(t *testing.T) {
	for _, raw := range []string{
		``, `null`, `[]`, `"secret-value"`, `{`, `{}`, `{"spaceId":"space-a"}`,
		`{"SpaceId":"space-a","unitSlug":"api"}`,
		`{"spaceId":null,"unitSlug":"api"}`,
		`{"spaceId":7,"unitSlug":"api"}`,
		`{"spaceId":"space-a","unitSlug":""}`,
		`{"spaceId":"space-a","unitSlug":" api"}`,
		`{"spaceId":"space-a","unitSlug":"api\nsecret-value"}`,
		`{"spaceId":"space-a","unitSlug":"api\u001b[2J"}`,
		`{"spaceId":"space-a","unitSlug":"[api](https://example.com)"}`,
		`{"spaceId":"space-a","spaceId":"space-b","unitSlug":"api"}`,
		`{"spaceId":"space-a","space\u0049d":"space-b","unitSlug":"api"}`,
		`{"spaceId":"space-a","unitSlug":"api","unitId":null}`,
		`{"spaceId":"space-a","unitSlug":"api","spaceSlug":{}}`,
		`{"spaceId":"space-a","unitSlug":"api","revisionNum":null}`,
		`{"spaceId":"space-a","unitSlug":"api","revisionNum":"7"}`,
		`{"spaceId":"space-a","unitSlug":"api","revisionNum":-1}`,
		`{"spaceId":"space-a","unitSlug":"api","revisionNum":1.5}`,
		`{"spaceId":"space-a","unitSlug":"api","revisionNum":1e2}`,
		`{"spaceId":"space-a","unitSlug":"api","revisionNum":9223372036854775808}`,
		`{"spaceId":"space-a","unitSlug":"api"} {}`,
		`{"spaceId":"space-a","unitSlug":"` + strings.Repeat("a", 257) + `"}`,
		`{"spaceId":"space-a","unitSlug":"api","future":"` + strings.Repeat("a", 8192) + `"}`,
	} {
		t.Run(raw[:min(len(raw), 100)], func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetAnnotations(map[string]string{ConfigHubOriginAnnotation: raw, "confighub.com/SpaceID": "space-a", "confighub.com/UnitSlug": "fallback"})
			got, omission := BuildConfigHubOriginEvidence(obj)
			require.Nil(t, got, "never fall back to or merge legacy identity")
			require.NotNil(t, omission)
			require.Equal(t, "confighub-origin", omission.Missing)
			require.Equal(t, "warning", omission.Severity)
			require.NotContains(t, omission.Reason, "secret-value")
		})
	}
}

func TestConfigHubOriginMissingIsNotUnmanaged(t *testing.T) {
	for _, obj := range []*unstructured.Unstructured{nil, {}, {Object: map[string]interface{}{"metadata": map[string]interface{}{"annotations": map[string]interface{}{"confighub.com/UnitSlug": "legacy-only"}}}}} {
		got, omission := BuildConfigHubOriginEvidence(obj)
		require.Nil(t, got)
		require.NotNil(t, omission)
		require.Equal(t, "info", omission.Severity)
		require.NotContains(t, omission.Reason, "unmanaged")
	}
}

func TestConfigHubOriginLegacyConflicts(t *testing.T) {
	for _, field := range []string{"SpaceID", "UnitID", "UnitSlug", "RevisionNum"} {
		for _, metadata := range []string{"label", "annotation"} {
			t.Run(metadata+field, func(t *testing.T) {
				obj := &unstructured.Unstructured{}
				annotations := map[string]string{ConfigHubOriginAnnotation: `{"spaceId":"space-a","unitId":"unit-a","unitSlug":"api","revisionNum":7}`}
				key := "confighub.com/" + field
				if metadata == "label" {
					obj.SetLabels(map[string]string{key: "different"})
				} else {
					annotations[key] = "different"
				}
				obj.SetAnnotations(annotations)
				got, omission := BuildConfigHubOriginEvidence(obj)
				require.Nil(t, got)
				require.NotNil(t, omission)
				require.Equal(t, "warning", omission.Severity)
				require.Contains(t, omission.Reason, "conflict")
			})
		}
	}
	obj := &unstructured.Unstructured{}
	obj.SetAnnotations(map[string]string{ConfigHubOriginAnnotation: `{"spaceId":"space-a","unitSlug":"api"}`, "confighub.com/SpaceID": "space-a", "confighub.com/UnitSlug": "api", "confighub.com/UnitID": "legacy-id", "confighub.com/RevisionNum": "7"})
	got, omission := BuildConfigHubOriginEvidence(obj)
	require.Nil(t, omission)
	require.Empty(t, got.UnitID, "do not complete origin from legacy fields")
	require.Nil(t, got.RevisionNum)
}
