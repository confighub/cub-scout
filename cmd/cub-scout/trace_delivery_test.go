// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCorrelateTraceDeliveryEvidence_ExactConfigHubMatches(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	result := &agent.TraceResult{
		Tool: "argocd",
		Object: agent.ResourceRef{
			Kind:      "Deployment",
			Name:      "payments-api",
			Namespace: "prod",
		},
		ConfigHub: &agent.TraceConfigHub{
			UnitSlug:  "payments-api",
			UnitID:    "u-123",
			SpaceName: "payments-prod",
			SpaceID:   "sp-123",
			TargetID:  "t-123",
		},
		Chain: []agent.ChainLink{
			{
				Kind: "ConfigHub OCI",
				Name: "payments-prod/prod",
				OCISource: &agent.OCISourceInfo{
					IsConfigHub: true,
					Space:       "payments-prod",
					Target:      "prod",
				},
			},
			{Kind: "Application", Name: "payments-app", Namespace: "argocd"},
			{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Ready: true},
		},
	}
	raw := &GitOpsDeliveryEvidence{
		ObservedAt: now,
		Scope: GitOpsDeliveryEvidenceScope{
			Namespace:  "prod",
			Space:      "payments-prod",
			Since:      "24h",
			StaleAfter: "15m0s",
			MaxItems:   10,
		},
		ConfigHub: &ConfigHubDeliveryEvidence{
			LiveStatuses: []ConfigHubLiveStatusEvidence{
				{
					Space:                    "payments-prod",
					SpaceID:                  "sp-123",
					Source:                   "argobot",
					App:                      "payments-app",
					SyncStatus:               "Synced",
					HealthStatus:             "Healthy",
					OperationPhase:           "Succeeded",
					Revision:                 "sha256:abc",
					ObservedAt:               "2026-09-10T11:55:00Z",
					Freshness:                "fresh",
					FreshnessSeconds:         300,
					DeliveryVerdict:          agent.VerdictPASS,
					ApplicationHealthVerdict: agent.VerdictPASS,
				},
			},
			Releases: []ConfigHubReleaseEvidence{
				{
					Slug:           "rel-42",
					ReleaseID:      "r-42",
					Space:          "payments-prod",
					SpaceID:        "sp-123",
					Target:         "prod",
					TargetID:       "t-123",
					Digest:         "sha256:abcdef",
					BundleBaseName: "payments",
					RevisionNum:    42,
					CreatedAt:      "2026-09-10T11:50:00Z",
				},
			},
			UnitEvents: []ConfigHubUnitEventEvidence{
				{
					EventID:   "ue-1",
					Action:    "ReleasePublished",
					Result:    "Succeeded",
					Unit:      "payments-api",
					UnitID:    "u-123",
					Space:     "payments-prod",
					SpaceID:   "sp-123",
					Target:    "prod",
					TargetID:  "t-123",
					CreatedAt: "2026-09-10T11:51:00Z",
				},
			},
		},
		EventConsumers: []GitOpsEventConsumerEvidence{
			{Kind: "Deployment", Name: "argobot", Namespace: "confighub-ops", Ready: true, Replicas: 1, ReadyReplicas: 1, EvidenceLabel: "app=argobot"},
		},
	}

	correlation := buildTraceDeliveryCorrelation(result)
	got := correlateTraceDeliveryEvidence(result, raw, correlation, nil)
	if got == nil {
		t.Fatal("delivery evidence is nil")
	}
	if got.LiveStatus == nil || got.LiveStatus.DeliveryVerdict != agent.VerdictPASS {
		t.Fatalf("live status = %+v, want PASS match", got.LiveStatus)
	}
	if !containsTraceMatch(got.LiveStatus.MatchedBy, "liveStatus.app==chain.application") {
		t.Fatalf("live status matchedBy = %+v, want application match", got.LiveStatus.MatchedBy)
	}
	if len(got.Releases) != 1 || got.Releases[0].Digest != "sha256:abcdef" {
		t.Fatalf("releases = %+v, want exact release match", got.Releases)
	}
	if !containsTraceMatch(got.Releases[0].MatchedBy, "targetId") {
		t.Fatalf("release matchedBy = %+v, want targetId match", got.Releases[0].MatchedBy)
	}
	if len(got.UnitEvents) != 1 || got.UnitEvents[0].EventID != "ue-1" {
		t.Fatalf("unitEvents = %+v, want exact unit-event match", got.UnitEvents)
	}
	if !containsTraceMatch(got.UnitEvents[0].MatchedBy, "unitEvent.unitId==confighub.unitId") {
		t.Fatalf("unitEvent matchedBy = %+v, want unit ID match", got.UnitEvents[0].MatchedBy)
	}
	if len(got.EventConsumers) != 1 || !got.EventConsumers[0].Ready {
		t.Fatalf("eventConsumers = %+v, want ready argobot evidence", got.EventConsumers)
	}
	line := formatTraceDeliveryEvidenceLine(got)
	for _, want := range []string{"delivery=PASS", "sync=Synced", "appHealth=PASS", "releases=1", "unitEvents=1"} {
		if !strings.Contains(line, want) {
			t.Fatalf("line %q missing %q", line, want)
		}
	}
}

func TestTraceGitOpsDeliveryOptions_RequiresObjectOrFlagSpace(t *testing.T) {
	opts, omissions := traceGitOpsDeliveryOptions(traceConfigHubDeliveryFlags{
		Enabled:   true,
		Namespace: "prod",
		Since:     "24h",
	}, agent.TraceDeliveryCorrelation{})
	if opts.Space != "" {
		t.Fatalf("opts.Space = %q, want empty", opts.Space)
	}
	if len(omissions) != 1 || omissions[0].Layer != "confighub.scope" {
		t.Fatalf("omissions = %+v, want confighub.scope", omissions)
	}
}

func TestMatchTraceReleases_DoesNotMatchBySpaceOnly(t *testing.T) {
	correlation := agent.TraceDeliveryCorrelation{
		Space: "payments-prod",
	}
	releases := []ConfigHubReleaseEvidence{
		{Slug: "rel-42", Space: "payments-prod", Target: "prod", Digest: "sha256:abcdef"},
	}
	got, omission := matchTraceReleases(correlation, releases)
	if len(got) != 0 {
		t.Fatalf("releases = %+v, want no space-only match", got)
	}
	if omission.Layer != "confighub.releases" {
		t.Fatalf("omission = %+v, want confighub.releases", omission)
	}
}

func TestEnrichTraceConfigHubFromObject(t *testing.T) {
	result := &agent.TraceResult{}
	obj := &unstructured.Unstructured{}
	obj.SetLabels(map[string]string{
		"confighub.com/UnitSlug": "payments-api",
	})
	obj.SetAnnotations(map[string]string{
		"confighub.com/UnitID":      "u-123",
		"confighub.com/SpaceName":   "payments-prod",
		"confighub.com/SpaceID":     "sp-123",
		"confighub.com/RevisionNum": "42",
	})

	enrichTraceConfigHubFromObject(result, obj)
	if result.ConfigHub == nil {
		t.Fatal("ConfigHub metadata was not enriched")
	}
	if result.ConfigHub.UnitSlug != "payments-api" || result.ConfigHub.SpaceName != "payments-prod" {
		t.Fatalf("ConfigHub metadata = %+v, want unit and space", result.ConfigHub)
	}
	if result.ConfigHub.UnitURL == "" || result.ConfigHub.RevisionsURL == "" {
		t.Fatalf("ConfigHub URLs missing: %+v", result.ConfigHub)
	}
}

func containsTraceMatch(matches []string, want string) bool {
	for _, match := range matches {
		if match == want {
			return true
		}
	}
	return false
}
