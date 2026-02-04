// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
	"time"
)

func TestRenderAttributionReportASCII_Header(t *testing.T) {
	report := &AttributionReport{
		SchemaVersion: AttributionReportSchemaVersion,
		GeneratedAt:   time.Now(),
		Items:         []AttributionReportItem{},
		Summary:       AttributionReportSummary{TotalItems: 0},
	}

	output := RenderAttributionReportASCII(report)

	if !strings.Contains(output, "Attribution Report") {
		t.Error("Output should contain 'Attribution Report' header")
	}
	if !strings.Contains(output, "Summary") {
		t.Error("Output should contain 'Summary' section")
	}
}

func TestRenderAttributionReportASCII_Summary(t *testing.T) {
	report := &AttributionReport{
		SchemaVersion: AttributionReportSchemaVersion,
		GeneratedAt:   time.Now(),
		Items:         []AttributionReportItem{},
		Summary: AttributionReportSummary{
			TotalItems:        5,
			OwnedCount:        3,
			UnattributedCount: 1,
			AmbiguousCount:    1,
		},
	}

	output := RenderAttributionReportASCII(report)

	if !strings.Contains(output, "Total objects:  5") {
		t.Error("Output should show total objects count")
	}
	if !strings.Contains(output, "Owned:          3") {
		t.Error("Output should show owned count")
	}
	if !strings.Contains(output, "Unattributed:   1") {
		t.Error("Output should show unattributed count")
	}
	if !strings.Contains(output, "Ambiguous:      1") {
		t.Error("Output should show ambiguous count")
	}
}

func TestRenderAttributionReportASCII_OwnedItem(t *testing.T) {
	report := &AttributionReport{
		SchemaVersion: AttributionReportSchemaVersion,
		GeneratedAt:   time.Now(),
		Items: []AttributionReportItem{
			{
				Ref:    AttributionRef{Kind: "Instance", Name: "staging-db", Namespace: "crossplane-system"},
				Reason: ReasonOwnedViaOwnerRef,
				BestOwner: &OwnerCandidate{
					Ref:      AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
					NodeType: NodeXR,
					Score:    ScoreOwnerReference,
					Evidence: EvidenceOwnerReference,
				},
				OwnerCandidates: []OwnerCandidate{
					{
						Ref:      AttributionRef{Kind: "XPostgreSQLInstance", Name: "xpostgresql-staging"},
						NodeType: NodeXR,
						Score:    ScoreOwnerReference,
						Evidence: EvidenceOwnerReference,
					},
				},
			},
		},
		Summary: AttributionReportSummary{TotalItems: 1, OwnedCount: 1},
	}

	output := RenderAttributionReportASCII(report)

	if !strings.Contains(output, "Instance:crossplane-system/staging-db") {
		t.Error("Output should show object ref")
	}
	if !strings.Contains(output, "Owned (via owner reference)") {
		t.Error("Output should show owned status")
	}
	if !strings.Contains(output, "XPostgreSQLInstance/xpostgresql-staging") {
		t.Error("Output should show owner ref")
	}
}

func TestRenderAttributionReportASCII_UnattributedItem(t *testing.T) {
	report := &AttributionReport{
		SchemaVersion: AttributionReportSchemaVersion,
		GeneratedAt:   time.Now(),
		Items: []AttributionReportItem{
			{
				Ref:             AttributionRef{Kind: "Instance", Name: "orphan-db", Namespace: "crossplane-system"},
				Reason:          ReasonUnattributed,
				BestOwner:       nil,
				OwnerCandidates: []OwnerCandidate{},
			},
		},
		Summary: AttributionReportSummary{TotalItems: 1, UnattributedCount: 1},
	}

	output := RenderAttributionReportASCII(report)

	if !strings.Contains(output, "Status: Unattributed") {
		t.Error("Output should show unattributed status")
	}
}

func TestRenderAttributionReportASCII_AmbiguousItem(t *testing.T) {
	report := &AttributionReport{
		SchemaVersion: AttributionReportSchemaVersion,
		GeneratedAt:   time.Now(),
		Items: []AttributionReportItem{
			{
				Ref:    AttributionRef{Kind: "Instance", Name: "contested-db", Namespace: "crossplane-system"},
				Reason: ReasonAmbiguous,
				BestOwner: &OwnerCandidate{
					Ref:      AttributionRef{Kind: "XPostgreSQLInstance", Name: "alpha-xr"},
					NodeType: NodeXR,
					Score:    ScoreCompositeLabel,
					Evidence: EvidenceCompositeLabel,
				},
				OwnerCandidates: []OwnerCandidate{
					{
						Ref:      AttributionRef{Kind: "XPostgreSQLInstance", Name: "alpha-xr"},
						NodeType: NodeXR,
						Score:    ScoreCompositeLabel,
						Evidence: EvidenceCompositeLabel,
					},
					{
						Ref:      AttributionRef{Kind: "XPostgreSQLInstance", Name: "beta-xr"},
						NodeType: NodeXR,
						Score:    ScoreCompositeLabel,
						Evidence: EvidenceCompositeLabel,
					},
				},
			},
		},
		Summary: AttributionReportSummary{TotalItems: 1, AmbiguousCount: 1},
	}

	output := RenderAttributionReportASCII(report)

	if !strings.Contains(output, "Ambiguous") {
		t.Error("Output should indicate ambiguous status")
	}
	if !strings.Contains(output, "Other candidates:") {
		t.Error("Output should show other candidates")
	}
	if !strings.Contains(output, "beta-xr") {
		t.Error("Output should show second candidate")
	}
}

func TestRenderAttributionReportASCII_Nil(t *testing.T) {
	output := RenderAttributionReportASCII(nil)

	if !strings.Contains(output, "No attribution report") {
		t.Error("Output should indicate no report available")
	}
}

func TestRenderAttributionReportASCII_Deterministic(t *testing.T) {
	report := &AttributionReport{
		SchemaVersion: AttributionReportSchemaVersion,
		GeneratedAt:   time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		Items: []AttributionReportItem{
			{
				Ref:    AttributionRef{Kind: "Instance", Name: "db-alpha", Namespace: "ns-1"},
				Reason: ReasonOwnedViaOwnerRef,
				BestOwner: &OwnerCandidate{
					Ref:      AttributionRef{Kind: "XPostgreSQLInstance", Name: "xr-alpha"},
					NodeType: NodeXR,
					Score:    ScoreOwnerReference,
					Evidence: EvidenceOwnerReference,
				},
				OwnerCandidates: []OwnerCandidate{
					{
						Ref:      AttributionRef{Kind: "XPostgreSQLInstance", Name: "xr-alpha"},
						NodeType: NodeXR,
						Score:    ScoreOwnerReference,
						Evidence: EvidenceOwnerReference,
					},
				},
			},
			{
				Ref:             AttributionRef{Kind: "Instance", Name: "db-beta", Namespace: "ns-1"},
				Reason:          ReasonUnattributed,
				BestOwner:       nil,
				OwnerCandidates: []OwnerCandidate{},
			},
		},
		Summary: AttributionReportSummary{TotalItems: 2, OwnedCount: 1, UnattributedCount: 1},
	}

	// Run multiple times and compare output
	var firstOutput string
	for i := 0; i < 5; i++ {
		output := RenderAttributionReportASCII(report)
		if i == 0 {
			firstOutput = output
		} else if output != firstOutput {
			t.Errorf("Iteration %d: output differs from first run", i)
		}
	}
}
