// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

// #617: explain and trace reported the tracer that ran as the owner. For an
// unlabelled Deployment explain said Helm (the last tracer to answer "not
// managed by me") and trace JSON said Flux; a ConfigHub-labelled one came out
// Unknown. The results below are the ones those tracers produced for the
// Deployments in evals/fixtures/scenario.yaml.
func TestReportedOwnerComesFromDetection(t *testing.T) {
	deployment := agent.ResourceRef{Kind: "Deployment", Name: "hotfix-worker", Namespace: "default"}
	completeFluxChain := []agent.ChainLink{
		{Kind: "GitRepository", Name: "shop", Namespace: "flux-system", Ready: true},
		{Kind: "Kustomization", Name: "shop-apps", Namespace: "flux-system", Ready: true},
		{Kind: "Deployment", Name: "checkout", Namespace: "shop", Ready: true},
	}
	for _, tc := range []struct {
		name   string
		result agent.TraceResult
		want   string
	}{
		{
			name:   "unlabelled, Helm tracer answered not managed",
			result: agent.TraceResult{Object: deployment, Tool: "helm", Error: "no Helm release found for Deployment/hotfix-worker", DetectedOwner: agent.OwnerUnknown},
			want:   "Native",
		},
		{
			name:   "unlabelled, Flux tracer answered not managed",
			result: agent.TraceResult{Object: deployment, Tool: "flux", Error: "resource not managed by Flux", DetectedOwner: agent.OwnerUnknown},
			want:   "Native",
		},
		{
			name:   "owned only through ownerReferences",
			result: agent.TraceResult{Object: deployment, Tool: "flux", Error: "resource not managed by Flux", DetectedOwner: agent.OwnerKubernetes},
			want:   "Native",
		},
		{
			name:   "ConfigHub label, no tracer for it",
			result: agent.TraceResult{Object: deployment, Tool: "confighub", Error: "ownership detected via label", DetectedOwner: agent.OwnerConfigHub},
			want:   "ConfigHub",
		},
		{
			name:   "Flux labels, Flux trace incomplete",
			result: agent.TraceResult{Object: deployment, Tool: "flux", Error: "flux trace failed: exit status 1", DetectedOwner: agent.OwnerFlux},
			want:   "Flux",
		},
		{
			// A complete chain is positive evidence and outranks the labels,
			// as in Helm-rendered resources delivered by an Argo Application.
			name:   "complete chain from a different tool than the labels",
			result: agent.TraceResult{Object: deployment, Tool: "flux", Chain: completeFluxChain, DetectedOwner: agent.OwnerHelm},
			want:   "Flux",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.result
			if got := buildExplainSummary(&result).Owner; got != tc.want {
				t.Errorf("explain owner = %q, want %q", got, tc.want)
			}
			if got := traceSummaryOwner(&result); got != tc.want {
				t.Errorf("trace ownerType = %q, want %q", got, tc.want)
			}
		})
	}
}

// Without a detection result (detection failed), the previous behaviour is
// kept: the owner comes from the tracer, or explain says it could not tell.
func TestReportedOwnerWithoutDetectionKeepsTracerOwner(t *testing.T) {
	result := agent.TraceResult{Object: agent.ResourceRef{Kind: "Deployment", Name: "api"}, Error: "resource not found"}
	if got := buildExplainSummary(&result).Owner; got != "Unknown - no recognized ownership labels found" {
		t.Errorf("explain owner = %q", got)
	}
	result.Tool = "flux"
	if got := traceSummaryOwner(&result); got != "Flux" {
		t.Errorf("trace ownerType = %q, want Flux", got)
	}
}

// When no tracer applies to a resource with no GitOps owner, explain gets a
// Native ownership-only result, not an error or a guessed tool.
func TestOwnershipOnlyResultForNativeResource(t *testing.T) {
	result := buildOwnershipOnlyTraceResult("Deployment", "debug-nginx", "temp-testing",
		&agent.Ownership{Type: agent.OwnerUnknown}, errors.New("resource not managed by Flux"))
	if result.Tool != "" {
		t.Errorf("Tool = %q, want empty: no tracer owns a Native resource", result.Tool)
	}
	summary := buildExplainSummary(result)
	if summary.Owner != "Native" {
		t.Errorf("Owner = %q, want Native", summary.Owner)
	}
	if summary.DeployedVia != "partial trace only" || summary.Source != "unknown" {
		t.Errorf("DeployedVia/Source = %q/%q, want the partial-trace defaults", summary.DeployedVia, summary.Source)
	}
}

func TestNotManagedTraceError(t *testing.T) {
	for msg, want := range map[string]bool{
		"resource not managed by Flux":                       true,
		"no Helm release found for Deployment/hotfix-worker": true,
		"flux trace failed: exit status 1":                   false,
		"":                                                   false,
	} {
		if got := isNotManagedTraceError(msg); got != want {
			t.Errorf("isNotManagedTraceError(%q) = %v, want %v", msg, got, want)
		}
	}
}
