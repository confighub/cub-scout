// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestRenderKroLineageHuman(t *testing.T) {
	t.Run("nil lineage returns empty string", func(t *testing.T) {
		if got := renderKroLineageHuman(nil); got != "" {
			t.Fatalf("renderKroLineageHuman(nil) = %q, want empty", got)
		}
	})

	t.Run("instance and definition", func(t *testing.T) {
		lineage := &agent.KroLineage{
			Managed: agent.KroLineageNode{
				Ref:     agent.ResourceRef{Kind: "Deployment", Name: "checkout-api", Namespace: "prod"},
				Present: true,
			},
			Instance: agent.KroLineageNode{
				Ref:     agent.ResourceRef{Kind: "WebApp", Name: "checkout", Namespace: "prod"},
				Present: true,
			},
			Definition: &agent.KroLineageNode{
				Ref:     agent.ResourceRef{Kind: "ResourceGraphDefinition", Name: "webapp-stack"},
				Present: true,
			},
			Evidence: []string{"ownerRef:apps.kro.run/v1alpha1/WebApp"},
		}

		out := renderKroLineageHuman(lineage)
		if !strings.Contains(out, "kro lineage:") {
			t.Fatalf("expected kro header, got %q", out)
		}
		if !strings.Contains(out, "instance:") {
			t.Fatalf("expected instance line, got %q", out)
		}
		if !strings.Contains(out, "definition:") {
			t.Fatalf("expected definition line, got %q", out)
		}
		if !strings.Contains(out, "evidence:") {
			t.Fatalf("expected evidence line, got %q", out)
		}
	})

	t.Run("partial lineage marker", func(t *testing.T) {
		lineage := &agent.KroLineage{
			Managed: agent.KroLineageNode{
				Ref:     agent.ResourceRef{Kind: "Deployment", Name: "checkout-api"},
				Present: true,
			},
			Instance: agent.KroLineageNode{
				Ref:     agent.ResourceRef{Kind: "WebApp", Name: "checkout"},
				Present: false,
			},
		}

		out := renderKroLineageHuman(lineage)
		if !strings.Contains(out, "(partial lineage)") {
			t.Fatalf("expected partial lineage marker, got %q", out)
		}
	})
}
