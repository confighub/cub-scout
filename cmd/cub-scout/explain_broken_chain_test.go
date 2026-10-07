// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// #827: explain took health from the resource's own link. A Deployment still
// running its last good revision under a Kustomization whose build was
// failing came out as "Managed by Flux", with nothing about the failure.
// The values are those recorded on a real cluster for #826.
func TestExplainReportsABrokenDeliveryChain(t *testing.T) {
	source := agent.ChainLink{Kind: "GitRepository", Name: "podinfo", Namespace: "flux-system", Ready: true, Status: "Ready", URL: "https://github.com/stefanprodan/podinfo"}
	workload := agent.ChainLink{Kind: "Deployment", Name: "podinfo", Namespace: "team-a", Ready: true, Status: "Managed by Flux"}
	object := agent.ResourceRef{Kind: "Deployment", Name: "podinfo", Namespace: "team-a"}
	const failure = "kustomization path not found: stat /tmp/kustomization-889571984/does-not-exist: no such file or directory"

	for _, tc := range []struct {
		name       string
		owner      agent.ChainLink
		wantHealth string
		wantNote   string
	}{
		{
			name:       "healthy chain is unchanged",
			owner:      agent.ChainLink{Kind: "Kustomization", Name: "podinfo", Namespace: "flux-system", Ready: true, Status: "Ready"},
			wantHealth: "Managed by Flux",
		},
		{
			name:       "failing owner",
			owner:      agent.ChainLink{Kind: "Kustomization", Name: "podinfo", Namespace: "flux-system", Status: "Not ready", StatusReason: failure},
			wantHealth: "Managed by Flux; delivery not ready at Kustomization/podinfo",
			wantNote:   "delivery not ready at Kustomization/podinfo: " + failure,
		},
		{
			name:       "suspended owner",
			owner:      agent.ChainLink{Kind: "Kustomization", Name: "podinfo", Namespace: "flux-system", Status: "Suspended"},
			wantHealth: "Managed by Flux; delivery not ready at Kustomization/podinfo",
			wantNote:   "delivery not ready at Kustomization/podinfo: Suspended",
		},
		{
			name:       "readiness that could not be read",
			owner:      agent.ChainLink{Kind: "Kustomization", Name: "podinfo", Namespace: "flux-system", Status: "Unknown", StatusReason: "readiness could not be read from the cluster: forbidden"},
			wantHealth: "Managed by Flux; delivery not ready at Kustomization/podinfo",
			wantNote:   "delivery not ready at Kustomization/podinfo: readiness could not be read from the cluster: forbidden",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &agent.TraceResult{Object: object, Tool: "flux", DetectedOwner: agent.OwnerFlux, Chain: []agent.ChainLink{source, tc.owner, workload}}
			summary := buildExplainSummary(result)
			if summary.Health != tc.wantHealth {
				t.Errorf("health = %q, want %q", summary.Health, tc.wantHealth)
			}
			notes := strings.Join(summary.Notes, "\n")
			if tc.wantNote == "" {
				if strings.Contains(notes, "delivery not ready") {
					t.Errorf("a healthy chain got a delivery note: %q", notes)
				}
			} else if !strings.Contains(notes, tc.wantNote) {
				t.Errorf("notes = %q, want one containing %q", notes, tc.wantNote)
			}
		})
	}

	// A source link that reports no status at all, as an Argo source does, is
	// not a failure: there is no signal to report.
	silent := buildExplainSummary(&agent.TraceResult{Object: object, Tool: "argocd", Chain: []agent.ChainLink{
		{Kind: "OCIRepository", Name: "demo", URL: "oci://example.invalid/demo"},
		{Kind: "Application", Name: "demo", Namespace: "argocd", Status: "Synced / Healthy", Ready: true},
		{Kind: "Deployment", Name: "podinfo", Namespace: "team-a", Status: "Synced", Ready: true},
	}})
	if strings.Contains(silent.Health, "delivery not ready") || strings.Contains(strings.Join(silent.Notes, "\n"), "delivery not ready") {
		t.Errorf("a link with no readiness signal was reported as not ready: health=%q notes=%q", silent.Health, silent.Notes)
	}
	// An Application that says it is degraded is a signal.
	degraded := buildExplainSummary(&agent.TraceResult{Object: object, Tool: "argocd", Chain: []agent.ChainLink{
		{Kind: "Application", Name: "demo", Namespace: "argocd", Status: "OutOfSync / Degraded"},
		{Kind: "Deployment", Name: "podinfo", Namespace: "team-a", Status: "Synced", Ready: true},
	}})
	if degraded.Health != "Synced; delivery not ready at Application/demo" {
		t.Errorf("health = %q for a degraded Application", degraded.Health)
	}

	// The resource's own readiness is the leaf's business; a chain that is
	// only the resource has no delivery to report on.
	alone := buildExplainSummary(&agent.TraceResult{Object: object, Chain: []agent.ChainLink{{Kind: "Deployment", Name: "podinfo", Status: "Unhealthy"}}})
	if strings.Contains(alone.Health, "delivery not ready") {
		t.Errorf("health = %q for a chain with no delivery links", alone.Health)
	}
}
