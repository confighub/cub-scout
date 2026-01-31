// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestRenderCrossplaneLineageHuman(t *testing.T) {
	t.Run("nil lineage returns empty string", func(t *testing.T) {
		result := renderCrossplaneLineageHuman(nil)
		if result != "" {
			t.Errorf("expected empty string for nil lineage, got %q", result)
		}
	})

	t.Run("XR-only (no claim)", func(t *testing.T) {
		lineage := &agent.CrossplaneLineage{
			Managed: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "RDSInstance", Name: "mydb", Namespace: "prod"},
				Present: true,
			},
			Composite: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "XPostgreSQLInstance", Name: "mydb-xr"},
				Present: true,
			},
			Claim:    nil,
			Evidence: []string{"label:crossplane.io/composite"},
		}

		result := renderCrossplaneLineageHuman(lineage)

		// Should contain header
		if !strings.Contains(result, "Crossplane lineage:") {
			t.Error("expected output to contain 'Crossplane lineage:'")
		}

		// Should contain managed
		if !strings.Contains(result, "managed:") {
			t.Error("expected output to contain 'managed:'")
		}

		// Should contain xr
		if !strings.Contains(result, "xr:") {
			t.Error("expected output to contain 'xr:'")
		}

		// Should NOT contain claim
		if strings.Contains(result, "claim:") {
			t.Error("expected output NOT to contain 'claim:' when Claim is nil")
		}

		// Should contain evidence
		if !strings.Contains(result, "evidence:") {
			t.Error("expected output to contain 'evidence:'")
		}
	})

	t.Run("XR + Claim present", func(t *testing.T) {
		lineage := &agent.CrossplaneLineage{
			Managed: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "RDSInstance", Name: "mydb", Namespace: "prod"},
				Present: true,
			},
			Composite: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "XPostgreSQLInstance", Name: "mydb-xr"},
				Present: true,
			},
			Claim: &agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "PostgreSQLInstance", Name: "mydb", Namespace: "prod"},
				Present: true,
			},
			Evidence: []string{"label:crossplane.io/composite", "label:crossplane.io/claim-name"},
		}

		result := renderCrossplaneLineageHuman(lineage)

		// Should contain claim line
		if !strings.Contains(result, "claim:") {
			t.Error("expected output to contain 'claim:' when Claim is present")
		}

		// Should NOT contain partial lineage (all present)
		if strings.Contains(result, "(partial lineage)") {
			t.Error("expected output NOT to contain '(partial lineage)' when all nodes are present")
		}
	})

	t.Run("partial chain - XR and Claim not present", func(t *testing.T) {
		lineage := &agent.CrossplaneLineage{
			Managed: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "RDSInstance", Name: "mydb", Namespace: "prod"},
				Present: true,
			},
			Composite: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "CompositeResource", Name: "mydb-xr"},
				Present: false, // XR object not found
			},
			Claim: &agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "Claim", Name: "mydb", Namespace: "prod"},
				Present: false, // Claim object not found
			},
			Evidence: []string{"label:crossplane.io/composite"},
		}

		result := renderCrossplaneLineageHuman(lineage)

		// Should contain partial lineage at least once (ideally twice)
		if !strings.Contains(result, "(partial lineage)") {
			t.Error("expected output to contain '(partial lineage)' when nodes are not present")
		}

		// Count occurrences of "(partial lineage)"
		count := strings.Count(result, "(partial lineage)")
		if count < 1 {
			t.Errorf("expected at least 1 '(partial lineage)', got %d", count)
		}
	})

	t.Run("evidence formatting with multiple items", func(t *testing.T) {
		lineage := &agent.CrossplaneLineage{
			Managed: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "RDSInstance", Name: "mydb"},
				Present: true,
			},
			Composite: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "XPostgreSQLInstance", Name: "mydb-xr"},
				Present: true,
			},
			Evidence: []string{"label:crossplane.io/composite", "label:crossplane.io/claim-name"},
		}

		result := renderCrossplaneLineageHuman(lineage)

		// Should contain both evidence items
		if !strings.Contains(result, "label:crossplane.io/composite") {
			t.Error("expected output to contain 'label:crossplane.io/composite'")
		}
		if !strings.Contains(result, "label:crossplane.io/claim-name") {
			t.Error("expected output to contain 'label:crossplane.io/claim-name'")
		}

		// Should contain comma separator
		if !strings.Contains(result, ", ") {
			t.Error("expected evidence items to be comma-separated")
		}
	})

	t.Run("empty XR name does not print xr line", func(t *testing.T) {
		lineage := &agent.CrossplaneLineage{
			Managed: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "RDSInstance", Name: "mydb"},
				Present: true,
			},
			Composite: agent.CrossplaneLineageNode{
				Ref:     agent.ResourceRef{Kind: "CompositeResource", Name: ""}, // Empty name
				Present: false,
			},
			Evidence: []string{"unresolved"},
		}

		result := renderCrossplaneLineageHuman(lineage)

		// Should NOT contain "xr:" followed by space (the label line)
		// Note: evidence may contain "xr:" as a value, so we check for the specific label format
		if strings.Contains(result, "xr:       ") {
			t.Error("expected output NOT to contain xr label line when XR name is empty")
		}
	})
}
