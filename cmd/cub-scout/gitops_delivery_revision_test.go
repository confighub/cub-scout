// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type revisionCorrelationFixture struct {
	Now        string `json:"now"`
	StaleAfter string `json:"staleAfter"`
	Cases      []struct {
		Name       string `json:"name"`
		SpaceID    string `json:"spaceId"`
		Revision   string `json:"revision"`
		ObservedAt string `json:"observedAt"`
		Releases   []struct {
			SpaceID        string `json:"spaceId"`
			ManifestDigest string `json:"manifestDigest"`
			ReleaseID      string `json:"releaseId"`
		} `json:"releases"`
		State          string `json:"state"`
		Matches        int    `json:"matches"`
		Freshness      string `json:"freshness"`
		ReasonContains string `json:"reasonContains"`
	} `json:"cases"`
}

func TestConfigHubReportedRevisionCorrelationFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "live-delivery-observability", "revision-correlation-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture revisionCorrelationFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	now, err := time.Parse(time.RFC3339, fixture.Now)
	if err != nil {
		t.Fatal(err)
	}
	staleAfter, err := time.ParseDuration(fixture.StaleAfter)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			annotation, err := json.Marshal(map[string]string{"revision": tc.Revision, "observedAt": tc.ObservedAt, "syncStatus": "Synced", "healthStatus": "Healthy", "operationPhase": "Succeeded"})
			if err != nil {
				t.Fatal(err)
			}
			space, err := json.Marshal([]interface{}{map[string]interface{}{"SpaceID": tc.SpaceID, "Annotations": map[string]string{configHubLiveStatusAnnotation: string(annotation)}}})
			if err != nil {
				t.Fatal(err)
			}
			statuses, omissions := buildConfigHubLiveStatusEvidence(string(space), now, staleAfter)
			if len(statuses) != 1 {
				t.Fatalf("statuses=%+v omissions=%+v", statuses, omissions)
			}
			releaseItems := make([]map[string]interface{}, 0, len(tc.Releases))
			for _, release := range tc.Releases {
				releaseItems = append(releaseItems, map[string]interface{}{"Release": map[string]string{"SpaceID": release.SpaceID, "ManifestDigest": release.ManifestDigest, "ReleaseID": release.ReleaseID}})
			}
			releaseJSON, err := json.Marshal(releaseItems)
			if err != nil {
				t.Fatal(err)
			}
			releases, releaseOmissions := buildConfigHubReleaseEvidence(string(releaseJSON), 0)
			coverage := "returned_rows"
			if len(releaseOmissions) > 0 {
				coverage = "partial_returned_rows"
			}
			correlateConfigHubReportedRevisions(statuses, releases, coverage)
			got := statuses[0]
			if got.RevisionCorrelation.State != tc.State || got.RevisionCorrelation.ObservedMatchCount != tc.Matches || got.Freshness != tc.Freshness {
				t.Fatalf("correlation/freshness=%+v/%s, want %s/%d/%s", got.RevisionCorrelation, got.Freshness, tc.State, tc.Matches, tc.Freshness)
			}
			if !strings.Contains(got.RevisionCorrelation.Limitation, "does not prove") {
				t.Fatalf("missing evidence limit: %+v", got.RevisionCorrelation)
			}
			if !strings.Contains(got.RevisionCorrelation.CoverageScope, "completeness of all matching server history is not asserted") {
				t.Fatalf("coverage overstates the bounded query: %+v", got.RevisionCorrelation)
			}
			if tc.ReasonContains != "" && !strings.Contains(got.RevisionCorrelation.Reason, tc.ReasonContains) {
				t.Errorf("correlation reason=%q, want substring %q", got.RevisionCorrelation.Reason, tc.ReasonContains)
			}
			if tc.Freshness == "stale" && string(got.DeliveryVerdict) != "WATCH" {
				t.Errorf("revision match changed stale verdict: %s", got.DeliveryVerdict)
			}
			if strings.TrimSpace(tc.Revision) != "" && got.Revision != strings.TrimSpace(tc.Revision) {
				t.Errorf("display revision=%q, want trimmed %q", got.Revision, strings.TrimSpace(tc.Revision))
			}
			if tc.Revision != got.Revision && got.RevisionCorrelation.ReportedRevisionRaw != tc.Revision {
				t.Errorf("raw rejected revision=%q, want %q", got.RevisionCorrelation.ReportedRevisionRaw, tc.Revision)
			}
			if tc.SpaceID != strings.TrimSpace(tc.SpaceID) && got.RevisionCorrelation.ReportedSpaceIDRaw != tc.SpaceID {
				t.Errorf("raw rejected SpaceID=%q, want %q", got.RevisionCorrelation.ReportedSpaceIDRaw, tc.SpaceID)
			}
			if tc.SpaceID != strings.TrimSpace(tc.SpaceID) && !strings.Contains(configHubRevisionCorrelationText(got.RevisionCorrelation), "raw SpaceID="+strconv.Quote(tc.SpaceID)) {
				t.Errorf("human-readable output hides rejected raw SpaceID: %s", configHubRevisionCorrelationText(got.RevisionCorrelation))
			}
			if tc.Revision != got.Revision && !strings.Contains(configHubRevisionCorrelationText(got.RevisionCorrelation), "raw revision="+strconv.Quote(tc.Revision)) {
				t.Errorf("human-readable output hides rejected raw revision: %s", configHubRevisionCorrelationText(got.RevisionCorrelation))
			}
		})
	}
}

func TestConfigHubRevisionCorrelationMalformedReleaseRowCannotProveAbsence(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	raw := `[{"Release":{"SpaceID":"space-a","ManifestDigest":"` + digest + `","ReleaseID":"observed"}},"not-an-object"]`
	releases, omissions := buildConfigHubReleaseEvidence(raw, 0)
	if len(omissions) == 0 {
		t.Fatal("malformed release row was silently dropped")
	}
	statuses := []ConfigHubLiveStatusEvidence{{spaceIDRaw: "space-a", revisionRaw: digest}}
	correlateConfigHubReportedRevisions(statuses, releases, "partial_returned_rows")
	got := statuses[0].RevisionCorrelation
	if got.State != "unknown" || got.ObservedMatchCount != 1 || !strings.Contains(got.Reason, "incomplete") {
		t.Fatalf("partial history overstated result: %+v", got)
	}
}

func TestConfigHubRevisionCorrelationPreservesMalformedReleaseDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	rawDigest := " sha256:abc "
	statuses := []ConfigHubLiveStatusEvidence{{spaceIDRaw: "space-a", revisionRaw: digest, SpaceID: "space-a"}}
	releases := []ConfigHubReleaseEvidence{{spaceIDRaw: "space-a", manifestDigestRaw: rawDigest, ManifestDigest: strings.TrimSpace(rawDigest), ReleaseID: "release-bad"}}
	correlateConfigHubReportedRevisions(statuses, releases, "returned_rows")
	got := statuses[0].RevisionCorrelation
	if got.State != "unknown" || len(got.InvalidRows) != 1 || got.InvalidRows[0].ManifestDigest != rawDigest {
		t.Fatalf("raw malformed manifest digest was lost: %+v", got)
	}
	if !strings.Contains(configHubRevisionCorrelationText(got), strconv.Quote(rawDigest)) {
		t.Fatalf("human-readable output hides malformed raw digest: %s", configHubRevisionCorrelationText(got))
	}
}

func TestConfigHubRevisionCorrelationBoundsCandidateDisplay(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	statuses := []ConfigHubLiveStatusEvidence{{spaceIDRaw: "space-a", revisionRaw: digest}}
	releases := make([]ConfigHubReleaseEvidence, 7)
	for i := range releases {
		releases[i] = ConfigHubReleaseEvidence{SpaceID: "space-a", spaceIDRaw: "space-a", manifestDigestRaw: digest, ReleaseID: string(rune('a' + i))}
	}
	correlateConfigHubReportedRevisions(statuses, releases, "returned_rows")
	got := statuses[0].RevisionCorrelation
	if got.State != "ambiguous" || got.ObservedMatchCount != 7 || len(got.Candidates) != maxRevisionCorrelationCandidates || got.CandidatesOmitted != 4 {
		t.Fatalf("candidate bounds not reported: %+v", got)
	}
}

func TestConfigHubRevisionCorrelationUnavailableHistoryIsUnknown(t *testing.T) {
	statuses := []ConfigHubLiveStatusEvidence{{spaceIDRaw: "space-a", revisionRaw: "sha256:" + strings.Repeat("a", 64)}}
	correlateConfigHubReportedRevisions(statuses, nil, "unavailable")
	if got := statuses[0].RevisionCorrelation; got.State != "unknown" || !strings.Contains(got.Reason, "unavailable") {
		t.Fatalf("unavailable history was overstated: %+v", got)
	}
}

func TestGitOpsTUIRejectsFormatConflict(t *testing.T) {
	if err := validateGitOpsTUIFormat(true, true, false); err == nil {
		t.Fatal("--tui with explicit --format must be rejected")
	}
	if err := validateGitOpsTUIFormat(true, false, true); err == nil {
		t.Fatal("--tui with legacy --json must be rejected")
	}
	if err := validateGitOpsTUIFormat(true, false, false); err != nil {
		t.Fatalf("plain --tui rejected: %v", err)
	}
}

func TestGitOpsStatusTUIUsesSafeSharedMarkdownSnapshot(t *testing.T) {
	summary := GitOpsSummary{Backend: "flux", Transport: "oci", DeliveryEvidence: &GitOpsDeliveryEvidence{ConfigHub: &ConfigHubDeliveryEvidence{LiveStatuses: []ConfigHubLiveStatusEvidence{{
		Space: "prod\x1b[2J", RevisionCorrelation: &ConfigHubRevisionCorrelation{State: "digest_match", Reason: "reported digest equals release digest", Limitation: "does not prove a release was fetched, applied, or executed"},
	}}}}}
	model := newGitOpsStatusTUIModel(summary)
	_, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 100})
	view := model.View()
	flattened := strings.Join(strings.Fields(view), " ")
	if !strings.Contains(flattened, "reported digest equals release digest") {
		t.Fatalf("TUI omitted correlation: %q", view)
	}
	if !strings.Contains(flattened, "does not prove a release was fetched, applied, or executed") {
		t.Fatalf("TUI omitted the correlation limitation: %q", view)
	}
	if strings.Contains(view, "\x1b") {
		t.Fatalf("TUI retained control/escape sequence: %q", view)
	}
	oldWidth := model.viewport.Width
	_, _ = model.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	if model.viewport.Width >= oldWidth || !strings.Contains(model.content, "reported digest equals release digest") {
		t.Fatal("resize did not preserve the same complete read-once content")
	}
}
