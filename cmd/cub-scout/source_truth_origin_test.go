// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// The annotations are the ones recorded on Deployment argobot/argobot after
// `cub cluster up --controller flux` (CI run 43447867725): the origin
// annotation and no UnitSlug or SpaceName metadata.
func recordedClusterUpWorkload() *runtimeWorkload {
	return &runtimeWorkload{
		Kind: "Deployment", Namespace: "argobot", Name: "argobot",
		Labels: map[string]string{"app": "argobot", "kustomize.toolkit.fluxcd.io/name": "argobot-scout-live", "kustomize.toolkit.fluxcd.io/namespace": "flux-system"},
		Annotations: map[string]string{
			"confighub.com/origin": `{"spaceId":"19db4ee7-faae-463c-ba71-7779ea79e2ca","spaceSlug":"argobot-scout-live","unitId":"0f19817b-5b19-45f8-b93e-ab67b80ef5bc","unitSlug":"argobot","revisionNum":3}`,
		},
	}
}

func TestSourceTruthReadsTheUnitNamedByTheOriginAnnotation(t *testing.T) {
	var gotUnit, gotSpace string
	old := sourceTruthUnitGet
	sourceTruthUnitGet = func(_ context.Context, unit, space string) ([]byte, error) {
		gotUnit, gotSpace = unit, space
		return []byte(`{"HeadRevisionNum": 3}`), nil
	}
	t.Cleanup(func() { sourceTruthUnitGet = old })

	surface, err := collectConfigHubSurface(context.Background(), recordedClusterUpWorkload())
	require.NoError(t, err)
	require.Equal(t, "argobot", gotUnit)
	require.Equal(t, "argobot-scout-live", gotSpace)
	require.Equal(t, &agent.ConfigHubSurface{Space: "argobot-scout-live", Unit: "argobot", Revision: "3", ObjectRevision: "3"}, surface)
}

// The object says it came from revision 3; the unit's head is 5. Every surface
// is present, but nothing shows the cluster runs what was read, so the answer
// is not an agreement.
func TestSourceTruthDoesNotAgreeWhenTheObjectNamesAnOlderRevision(t *testing.T) {
	surfaces := func(head string) agent.SourceTruthSurfaces {
		return agent.SourceTruthSurfaces{
			ConfigHub:  &agent.ConfigHubSurface{Space: "argobot-scout-live", Unit: "argobot", Revision: head, ObjectRevision: "3"},
			Controller: &agent.ControllerSurface{Kind: "Flux", Source: "oci://172.18.0.1:32181/space/argobot-scout-live", RevisionOrDigest: "latest@sha256:08289f07c2da7e29d1e155507137b1f924e71d95a202db1b7eafefcc7e58bae8", Health: "Ready"},
			Runtime:    &agent.RuntimeSurface{Resource: "Deployment/argobot in argobot", Field: "spec.template.spec.containers[0].image", Value: "ghcr.io/confighub/argobot:v0.2.9", Health: "Current"},
		}
	}
	same := agent.Derive(agent.StrategyConfigHubOCIFlux, surfaces("3"))
	require.Equal(t, agent.StatusPASS, same.Status)

	behind := agent.Derive(agent.StrategyConfigHubOCIFlux, surfaces("5"))
	require.Equal(t, agent.StatusWATCH, behind.Status)
	require.Equal(t, agent.VerdictINCOMPLETE, behind.SourceTruth)
	require.Contains(t, behind.ProofGaps, "confighub.object_revision_differs_from_head")
}

func TestSourceTruthConfigHubIdentityOfWorkload(t *testing.T) {
	const origin = "confighub.com/origin"
	cases := []struct {
		name        string
		labels      map[string]string
		annotations map[string]string
		unit, space string
		reason      string
	}{
		{name: "older metadata alone", labels: map[string]string{"confighub.com/UnitSlug": "api"}, annotations: map[string]string{"confighub.com/SpaceName": "prod"}, unit: "api", space: "prod"},
		{name: "nothing: not a claim that ConfigHub does not manage it", reason: "cannot be read from the object"},
		{name: "older unit without a space", labels: map[string]string{"confighub.com/UnitSlug": "api"}, reason: "no confighub.com/SpaceName"},
		{name: "origin without a space slug uses the older space name", annotations: map[string]string{origin: `{"spaceId":"s1","unitSlug":"api"}`, "confighub.com/SpaceName": "prod"}, unit: "api", space: "prod"},
		{name: "origin without any space slug", annotations: map[string]string{origin: `{"spaceId":"s1","unitSlug":"api"}`}, reason: "no space slug"},
		{name: "malformed origin is not replaced by older metadata", labels: map[string]string{"confighub.com/UnitSlug": "api"}, annotations: map[string]string{origin: `{"spaceId":`, "confighub.com/SpaceName": "prod"}, reason: "is not usable"},
		{name: "origin that conflicts with the older unit", labels: map[string]string{"confighub.com/UnitSlug": "other"}, annotations: map[string]string{origin: `{"spaceId":"s1","spaceSlug":"prod","unitSlug":"api"}`}, reason: "conflicts with legacy"},
		{name: "origin that conflicts with the older space name", annotations: map[string]string{origin: `{"spaceId":"s1","spaceSlug":"prod","unitSlug":"api"}`, "confighub.com/SpaceName": "staging"}, reason: "name different spaces"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			labels, annotations := tc.labels, tc.annotations
			if labels == nil {
				labels = map[string]string{}
			}
			if annotations == nil {
				annotations = map[string]string{}
			}
			unit, space, _, err := configHubIdentityOfWorkload(&runtimeWorkload{Kind: "Deployment", Namespace: "n", Name: "w", Labels: labels, Annotations: annotations})
			if tc.reason != "" {
				require.NotNil(t, err)
				require.Equal(t, "confighub", err.Surface)
				require.Contains(t, err.Reason, tc.reason)
				require.NotContains(t, err.Reason, "not ConfigHub-managed")
				return
			}
			require.Nil(t, err)
			require.Equal(t, tc.unit, unit)
			require.Equal(t, tc.space, space)
		})
	}
}
