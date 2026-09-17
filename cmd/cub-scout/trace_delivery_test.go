// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
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

func TestCorrelateTraceDeliveryEvidence_ExplicitScopeSpaceMatchesApplication(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Chain: []agent.ChainLink{
			{Kind: "Application", Name: "payments-api", Namespace: "argocd"},
			{Kind: "Deployment", Name: "api", Namespace: "prod"},
		},
	}
	raw := &GitOpsDeliveryEvidence{
		ObservedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
		Scope: GitOpsDeliveryEvidenceScope{
			Namespace: "prod",
			Space:     "payments-prod",
			Since:     "24h",
			MaxItems:  10,
		},
		ConfigHub: &ConfigHubDeliveryEvidence{
			LiveStatuses: []ConfigHubLiveStatusEvidence{{
				Space:                    "payments-prod",
				Source:                   "argobot",
				App:                      "payments-api",
				SyncStatus:               "Synced",
				HealthStatus:             "Healthy",
				OperationPhase:           "Succeeded",
				Freshness:                "fresh",
				DeliveryVerdict:          agent.VerdictPASS,
				ApplicationHealthVerdict: agent.VerdictPASS,
			}},
		},
	}

	got := correlateTraceDeliveryEvidence(result, raw, buildTraceDeliveryCorrelation(result), nil)
	if got.LiveStatus == nil {
		t.Fatalf("live status did not match from explicit scope space: %+v", got)
	}
	if !containsTraceMatch(got.Correlation.MatchedBy, "scope.space") {
		t.Fatalf("correlation matchedBy = %+v, want scope.space", got.Correlation.MatchedBy)
	}
	// The resource named no space; --confighub-space supplied it. The row must
	// say so rather than read as a space the resource itself declared.
	if !containsTraceMatch(got.LiveStatus.MatchedBy, "scope.space") || containsTraceMatch(got.LiveStatus.MatchedBy, "space") {
		t.Fatalf("live status matchedBy = %+v, want scope.space and not space", got.LiveStatus.MatchedBy)
	}
}

func TestCorrelateTraceDeliveryEvidence_WildcardSpaceDoesNotBecomeExactMatch(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Chain:  []agent.ChainLink{{Kind: "Application", Name: "payments-api"}},
	}
	raw := &GitOpsDeliveryEvidence{
		ObservedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
		Scope:      GitOpsDeliveryEvidenceScope{Space: "*", Since: "24h", MaxItems: 10},
		ConfigHub: &ConfigHubDeliveryEvidence{
			LiveStatuses: []ConfigHubLiveStatusEvidence{{
				Space:                    "payments-prod",
				App:                      "payments-api",
				SyncStatus:               "Synced",
				HealthStatus:             "Healthy",
				Freshness:                "fresh",
				DeliveryVerdict:          agent.VerdictPASS,
				ApplicationHealthVerdict: agent.VerdictPASS,
			}},
		},
	}

	got := correlateTraceDeliveryEvidence(result, raw, buildTraceDeliveryCorrelation(result), nil)
	if got.LiveStatus != nil {
		t.Fatalf("wildcard scope space must not be treated as exact match: %+v", got.LiveStatus)
	}
}

func TestCorrelateTraceDeliveryEvidence_ScopeSpaceAloneKeepsIdentityOmission(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Chain:  []agent.ChainLink{{Kind: "Deployment", Name: "api", Namespace: "prod"}},
	}
	raw := &GitOpsDeliveryEvidence{
		ObservedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
		Scope:      GitOpsDeliveryEvidenceScope{Space: "payments-prod", Since: "24h", MaxItems: 10},
		ConfigHub:  &ConfigHubDeliveryEvidence{},
	}

	got := correlateTraceDeliveryEvidence(result, raw, buildTraceDeliveryCorrelation(result), nil)
	if got == nil {
		t.Fatal("delivery evidence is nil")
	}
	if !containsTraceMatch(got.Correlation.MatchedBy, "scope.space") {
		t.Fatalf("correlation matchedBy = %+v, want scope.space", got.Correlation.MatchedBy)
	}
	if !containsTraceOmission(got.Omissions, "confighub.identity") {
		t.Fatalf("omissions = %+v, want confighub.identity", got.Omissions)
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

// Rows are matched to the traced object before trimming. Trimming the whole
// space to maxItems first would cut this target's only release, the oldest of
// twelve, and report a false "no release row matched".
func TestCorrelateTraceDeliveryEvidence_MatchesBeforeTrimming(t *testing.T) {
	opts, _ := traceGitOpsDeliveryOptions(traceConfigHubDeliveryFlags{Space: "payments-prod"}, agent.TraceDeliveryCorrelation{})
	if !opts.MatchBeforeTrim {
		t.Fatalf("trace delivery options must match before trimming: %+v", opts)
	}

	var raw strings.Builder
	raw.WriteString("[")
	for i := 0; i < 11; i++ {
		fmt.Fprintf(&raw, `{"Release":{"ReleaseID":"other-%02d","SpaceID":"sp-1","SpaceSlug":"payments-prod","TargetID":"t-other","CreatedAt":"2026-09-10T11:%02d:00Z"}},`, i, 30+i)
	}
	raw.WriteString(`{"Release":{"ReleaseID":"mine","SpaceID":"sp-1","SpaceSlug":"payments-prod","TargetID":"t-mine","CreatedAt":"2026-09-10T09:00:00Z"}}]`)

	untrimmed, omissions := buildConfigHubReleaseEvidence(raw.String(), 0)
	if len(untrimmed) != 12 || len(omissions) != 0 {
		t.Fatalf("untrimmed releases = %d omissions = %+v, want all 12 rows", len(untrimmed), omissions)
	}
	if trimmed, _ := buildConfigHubReleaseEvidence(raw.String(), defaultGitOpsDeliveryMaxItems); len(trimmed) != defaultGitOpsDeliveryMaxItems {
		t.Fatalf("trimmed releases = %d, want %d (the precondition for the false negative)", len(trimmed), defaultGitOpsDeliveryMaxItems)
	}

	got := correlateTraceDeliveryEvidence(nil, &GitOpsDeliveryEvidence{
		Scope:     GitOpsDeliveryEvidenceScope{Space: "payments-prod", MaxItems: defaultGitOpsDeliveryMaxItems},
		ConfigHub: &ConfigHubDeliveryEvidence{Releases: untrimmed},
	}, agent.TraceDeliveryCorrelation{Space: "payments-prod", SpaceID: "sp-1", TargetID: "t-mine"}, nil)

	if len(got.Releases) != 1 || got.Releases[0].ReleaseID != "mine" {
		t.Fatalf("releases = %+v, want the traced target's release even though it is the oldest of 12", got.Releases)
	}
}

// Matches beyond maxItems are trimmed after matching, and the trim is reported.
func TestCorrelateTraceDeliveryEvidence_TrimsMatchesAndSaysSo(t *testing.T) {
	var releases []ConfigHubReleaseEvidence
	for i := 0; i < 12; i++ {
		releases = append(releases, ConfigHubReleaseEvidence{ReleaseID: fmt.Sprintf("r-%02d", i), SpaceID: "sp-1", TargetID: "t-mine"})
	}
	got := correlateTraceDeliveryEvidence(nil, &GitOpsDeliveryEvidence{
		Scope:     GitOpsDeliveryEvidenceScope{Space: "payments-prod", MaxItems: 10},
		ConfigHub: &ConfigHubDeliveryEvidence{Releases: releases},
	}, agent.TraceDeliveryCorrelation{SpaceID: "sp-1", TargetID: "t-mine"}, nil)

	if len(got.Releases) != 10 {
		t.Fatalf("releases = %d, want 10 after trimming matches", len(got.Releases))
	}
	found := false
	for _, omission := range got.Omissions {
		if omission.Layer == "confighub.releases" && strings.Contains(omission.Reason, "trimmed 12 matching release rows to maxItems=10") {
			found = true
		}
	}
	if !found {
		t.Fatalf("omissions = %+v, want the trim of matching rows reported", got.Omissions)
	}
}

// A release names its target by ID; an OCI-source or renderedFrom chain knows
// only the slug. With no key in common the join was not evaluated, and the
// omission must say that rather than claim no release matched.
func TestMatchTraceReleases_DisjointTargetKeysAreUnknownNotNoMatch(t *testing.T) {
	releases := []ConfigHubReleaseEvidence{{ReleaseID: "r-1", Space: "payments-prod", SpaceID: "sp-1", TargetID: "t-1"}}

	got, omission := matchTraceReleases(agent.TraceDeliveryCorrelation{Space: "payments-prod", Target: "us-west"}, releases)
	if len(got) != 0 {
		t.Fatalf("releases = %+v, want none: a slug cannot be compared to a target ID", got)
	}
	if !strings.Contains(omission.Reason, "different keys") || !strings.Contains(omission.Impact, "unknown") {
		t.Fatalf("omission = %+v, want it to say the join could not be evaluated", omission)
	}

	// Same key type, different target: that is a genuine no-match.
	_, omission = matchTraceReleases(agent.TraceDeliveryCorrelation{Space: "payments-prod", TargetID: "t-2"}, releases)
	if !strings.Contains(omission.Reason, "no release row matched") {
		t.Fatalf("omission = %+v, want a plain no-match when the keys are comparable", omission)
	}

	// No target key on the resource at all: also a plain no-match, never "different keys".
	_, omission = matchTraceReleases(agent.TraceDeliveryCorrelation{Space: "payments-prod"}, releases)
	if !strings.Contains(omission.Reason, "no release row matched") {
		t.Fatalf("omission = %+v, want a plain no-match when the resource has no target key", omission)
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

func containsTraceOmission(omissions []agent.TraceDeliveryOmission, layer string) bool {
	for _, omission := range omissions {
		if omission.Layer == layer {
			return true
		}
	}
	return false
}

// doctor reports how many releases and unit events the window held, not how
// many survived trimming to maxItems.
func TestCollectGitOpsDeliveryEvidence_CountsRowsBeforeTrimming(t *testing.T) {
	oldRequire, oldRun := requireGitOpsConfigHubFn, runGitOpsCubCommand
	t.Cleanup(func() { requireGitOpsConfigHubFn, runGitOpsCubCommand = oldRequire, oldRun })
	requireGitOpsConfigHubFn = func() error { return nil }

	var releases, events strings.Builder
	releases.WriteString("[")
	events.WriteString("[")
	for i := 0; i < 12; i++ {
		if i > 0 {
			releases.WriteString(",")
			events.WriteString(",")
		}
		fmt.Fprintf(&releases, `{"Release":{"ReleaseID":"r-%02d","SpaceSlug":"payments-prod","CreatedAt":"2026-09-10T11:%02d:00Z"}}`, i, 10+i)
		fmt.Fprintf(&events, `{"UnitEventID":"e-%02d","SpaceSlug":"payments-prod","CreatedAt":"2026-09-10T11:%02d:00Z"}`, i, 10+i)
	}
	releases.WriteString("]")
	events.WriteString("]")
	runGitOpsCubCommand = func(ctx context.Context, args []string) (string, error) {
		switch args[0] {
		case "release":
			return releases.String(), nil
		case "unit-event":
			return events.String(), nil
		}
		return "[]", nil
	}

	evidence := collectGitOpsDeliveryEvidence(context.Background(), newGitOpsDeliveryFakeClient(), gitOpsDeliveryEvidenceOptions{
		Space: "payments-prod", Since: "24h", Window: 24 * time.Hour, StaleAfter: 15 * time.Minute,
		Now: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC), MaxItems: 10,
	})

	if len(evidence.ConfigHub.Releases) != 10 || evidence.ConfigHub.ReleasesTotal != 12 {
		t.Fatalf("releases kept = %d total = %d, want 10 kept of 12", len(evidence.ConfigHub.Releases), evidence.ConfigHub.ReleasesTotal)
	}
	if len(evidence.ConfigHub.UnitEvents) != 10 || evidence.ConfigHub.UnitEventsTotal != 12 {
		t.Fatalf("unit events kept = %d total = %d, want 10 kept of 12", len(evidence.ConfigHub.UnitEvents), evidence.ConfigHub.UnitEventsTotal)
	}
	summary := buildDoctorDeliverySummary(evidence)
	if summary.RecentReleases != 12 || summary.RecentUnitEvents != 12 {
		t.Fatalf("doctor counts = %d releases / %d unit events, want 12 / 12, not the trimmed 10", summary.RecentReleases, summary.RecentUnitEvents)
	}
}

// Both sides carrying an ID is the strongest key available. A slug that happens
// to match must not override two IDs that disagree.
func TestTraceJoins_ConflictingIDsBeatMatchingSlugs(t *testing.T) {
	correlation := agent.TraceDeliveryCorrelation{
		UnitSlug: "backend", UnitID: "unit-a",
		Space: "prod", SpaceID: "space-a",
		Target: "cluster", TargetID: "target-a",
	}

	if ok, _ := traceSpaceMatches(correlation, "prod", "space-b"); ok {
		t.Error("space joined on a matching slug although the space IDs differ")
	}
	if ok, by := traceSpaceMatches(correlation, "renamed", "space-a"); !ok || by != "spaceId" {
		t.Errorf("space with the same ID = (%v, %q), want joined by spaceId", ok, by)
	}
	if ok, by := traceSpaceMatches(correlation, "prod", ""); !ok || by != "space" {
		t.Errorf("row without a space ID = (%v, %q), want joined by space", ok, by)
	}
	if ok, _ := traceTargetMatches(correlation, "cluster", "target-b"); ok {
		t.Error("target joined on a matching slug although the target IDs differ")
	}

	events := []ConfigHubUnitEventEvidence{
		{EventID: "other-unit", Unit: "backend", UnitID: "unit-b", Space: "prod", SpaceID: "space-a"},
		{EventID: "same-unit", Unit: "backend", UnitID: "unit-a", Space: "prod", SpaceID: "space-a"},
		{EventID: "slug-only", Unit: "backend", Space: "prod", SpaceID: "space-a"},
	}
	got, _ := matchTraceUnitEvents(correlation, events)
	ids := []string{}
	for _, event := range got {
		ids = append(ids, event.EventID)
	}
	if strings.Join(ids, ",") != "same-unit,slug-only" {
		t.Fatalf("joined events = %v, want same-unit and slug-only; other-unit shares the slug but not the unit ID", ids)
	}
}

// A resource that carries a unit label but names no space joins only through
// --confighub-space. That join is allowed, and every row it produces says the
// space came from the operator's scope, not from the resource.
func TestUnitEventJoinedThroughScopeSpaceSaysSo(t *testing.T) {
	result := &agent.TraceResult{
		Object:    agent.ResourceRef{Kind: "Deployment", Name: "backend", Namespace: "prod"},
		ConfigHub: &agent.TraceConfigHub{UnitSlug: "backend"},
	}
	raw := &GitOpsDeliveryEvidence{
		ObservedAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
		Scope:      GitOpsDeliveryEvidenceScope{Namespace: "prod", Space: "other", MaxItems: 10},
		ConfigHub: &ConfigHubDeliveryEvidence{
			UnitEvents: []ConfigHubUnitEventEvidence{{EventID: "ue-1", Unit: "backend", UnitID: "unit-x", Space: "other"}},
		},
	}

	got := correlateTraceDeliveryEvidence(result, raw, buildTraceDeliveryCorrelation(result), nil)
	if len(got.UnitEvents) != 1 {
		t.Fatalf("unit events = %+v, want the one event in the scoped space", got.UnitEvents)
	}
	matchedBy := got.UnitEvents[0].MatchedBy
	if !containsTraceMatch(matchedBy, "scope.space") || containsTraceMatch(matchedBy, "space") {
		t.Fatalf("matchedBy = %v, want scope.space and not space", matchedBy)
	}
}

// The release join is by space and target. The note that says so has to reach
// every rendering, and must not claim the release is being served.
func TestReleaseJoinNoteIsTargetLevelAndIsPrinted(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	result := &agent.TraceResult{
		Object:    agent.ResourceRef{Kind: "Deployment", Name: "backend", Namespace: "prod"},
		ConfigHub: &agent.TraceConfigHub{UnitSlug: "backend", SpaceID: "space-a", TargetID: "target-a"},
	}
	withdrawn := false
	raw := &GitOpsDeliveryEvidence{
		ObservedAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
		Scope:      GitOpsDeliveryEvidenceScope{Namespace: "prod", Space: "prod", MaxItems: 10},
		ConfigHub: &ConfigHubDeliveryEvidence{
			Releases: []ConfigHubReleaseEvidence{
				{ReleaseID: "r2", BundleBaseName: "prod-bundle", ReleaseNum: 2, SpaceID: "space-a", TargetID: "target-a", Published: boolPtr(true)},
				{ReleaseID: "r1", BundleBaseName: "prod-bundle", ReleaseNum: 1, SpaceID: "space-a", TargetID: "target-a", Published: &withdrawn},
			},
		},
	}

	got := correlateTraceDeliveryEvidence(result, raw, buildTraceDeliveryCorrelation(result), nil)
	if len(got.Releases) != 2 {
		t.Fatalf("releases = %+v, want both rows for the target", got.Releases)
	}
	note := strings.Join(got.Notes, "\n")
	if !strings.Contains(note, "does not show that the release contains this resource's unit") {
		t.Fatalf("notes = %q, want the target-level join stated", note)
	}
	if strings.Contains(note, "was published") {
		t.Fatalf("notes = %q: a withdrawn row is listed, so the note must not say the rows were published", note)
	}

	if line := formatTraceDeliveryEvidenceLine(got); !strings.Contains(line, "releases=2 published=1/2") {
		t.Fatalf("summary line = %q, want the served count beside the row count", line)
	}
	for name, render := range map[string]func(*agent.TraceDeliveryEvidence){
		"human":    renderTraceDeliveryEvidenceHuman,
		"markdown": renderTraceDeliveryEvidenceMarkdown,
	} {
		out := captureStdout(t, func() { render(got) })
		if !strings.Contains(out, "Notes:") || !strings.Contains(out, "does not show that the release contains") {
			t.Errorf("%s output does not print the join note:\n%s", name, out)
		}
	}

	// No matched release, no note: the note describes rows that are listed.
	raw.ConfigHub.Releases = nil
	if got := correlateTraceDeliveryEvidence(result, raw, buildTraceDeliveryCorrelation(result), nil); len(got.Notes) != 0 {
		t.Fatalf("notes = %v, want none when no release row is listed", got.Notes)
	}
}

func TestTracePublishedSummary(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name     string
		releases []agent.TraceDeliveryRelease
		want     string
	}{
		{"server does not report it", []agent.TraceDeliveryRelease{{}, {}}, "published=unknown"},
		{"one served of two", []agent.TraceDeliveryRelease{{Published: &yes}, {Published: &no}}, "published=1/2"},
		{"all withdrawn", []agent.TraceDeliveryRelease{{Published: &no}}, "published=0/1"},
		{"mixed with unknown", []agent.TraceDeliveryRelease{{Published: &yes}, {}}, "published=1/2 unknown=1"},
	} {
		if got := tracePublishedSummary(tc.releases); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
