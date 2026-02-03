// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package ascii_test provides JSON golden tests for trace command output.
//
// NOTE: These goldens lock the v0.14 JSON schema output for trace.
// Changes here should be reviewed as API changes.
//
// To update goldens: go test ./test/ascii/... -update
package ascii_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/confighub/cub-scout/internal/mapsvc"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/test/ascii/golden"
	"github.com/confighub/cub-scout/test/ascii/runner"
)

// TestTraceFlux_JSON tests the v0.14 JSON schema output for Flux trace.
func TestTraceFlux_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)

	// Load trace result from fixture
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "flux.json")
	data, err := os.ReadFile(fixtureAbs)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var traceResult agent.TraceResult
	if err := json.Unmarshal(data, &traceResult); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}

	// Convert to v0.14 format
	output := convertTraceResultToV014(&traceResult)

	// Marshal with indent for readable golden
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal output: %v", err)
	}

	got := string(jsonBytes) + "\n"

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "trace", "flux.json.golden")
	golden.AssertGolden(t, got, goldenPath)
}

// TestTraceArgo_JSON tests the v0.14 JSON schema output for ArgoCD trace.
func TestTraceArgo_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)

	// Load trace result from fixture
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "argo.json")
	data, err := os.ReadFile(fixtureAbs)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var traceResult agent.TraceResult
	if err := json.Unmarshal(data, &traceResult); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}

	// Convert to v0.14 format
	output := convertTraceResultToV014(&traceResult)

	// Marshal with indent for readable golden
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal output: %v", err)
	}

	got := string(jsonBytes) + "\n"

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "trace", "argo.json.golden")
	golden.AssertGolden(t, got, goldenPath)
}

// TestTraceNative_JSON tests the v0.14 JSON schema output for unmanaged resources.
func TestTraceNative_JSON(t *testing.T) {
	repoRoot := runner.RepoRoot(t)

	// Load trace result from fixture
	fixtureAbs := filepath.Join(repoRoot, "test", "ascii", "trace", "testdata", "native.json")
	data, err := os.ReadFile(fixtureAbs)
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	var traceResult agent.TraceResult
	if err := json.Unmarshal(data, &traceResult); err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}

	// Convert to v0.14 format
	output := convertTraceResultToV014(&traceResult)

	// Marshal with indent for readable golden
	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal output: %v", err)
	}

	got := string(jsonBytes) + "\n"

	goldenPath := filepath.Join(repoRoot, "test", "ascii", "trace", "native.json.golden")
	golden.AssertGolden(t, got, goldenPath)
}

// convertTraceResultToV014 converts agent.TraceResult to mapsvc.TraceOutput.
// This mirrors the conversion logic in cmd/cub-scout/trace.go.
func convertTraceResultToV014(result *agent.TraceResult) mapsvc.TraceOutput {
	// Build target from the traced resource
	target := mapsvc.ResourceID{
		Kind:      result.Object.Kind,
		Namespace: result.Object.Namespace,
		Name:      result.Object.Name,
	}

	// Convert chain links to v0.14 chain nodes
	var chain []mapsvc.ChainNode
	for _, link := range result.Chain {
		role := mapsvc.InferRole(link.Kind)
		node := mapsvc.ChainNode{
			ID: mapsvc.ResourceID{
				Kind:      link.Kind,
				Namespace: link.Namespace,
				Name:      link.Name,
			},
			Role: role,
			// Relationship is based on the node's role, not the edge
			Relationship: mapsvc.RelationshipForRole(role),
		}

		// Build evidence from available metadata
		node.Evidence = buildTestEvidence(link, result.Tool)

		chain = append(chain, node)
	}

	// Build summary
	summary := buildTestTraceSummary(result, chain)

	return mapsvc.TraceOutput{
		Command: "trace",
		Target:  target,
		Chain:   chain,
		Summary: summary,
	}
}

// buildTestEvidence constructs evidence from a chain link (test version).
func buildTestEvidence(link agent.ChainLink, tool string) []mapsvc.Evidence {
	var evidence []mapsvc.Evidence

	role := mapsvc.InferRole(link.Kind)

	switch role {
	case mapsvc.RoleSource:
		if link.URL != "" {
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceField,
				Key:   "spec.url",
				Value: link.URL,
				Path:  "spec.url",
			})
		}

	case mapsvc.RoleDeployer:
		if link.Path != "" {
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceField,
				Key:   "spec.path",
				Value: link.Path,
				Path:  "spec.path",
			})
		}

	case mapsvc.RoleWorkload, mapsvc.RoleIntermediate:
		// Workload: evidence from labels based on tool
		toolLower := normalizeToolLower(tool)
		switch toolLower {
		case "flux":
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceLabel,
				Key:   "kustomize.toolkit.fluxcd.io/name",
				Value: "",
				Path:  "metadata.labels",
			})
		case "argocd":
			evidence = append(evidence, mapsvc.Evidence{
				Type:  mapsvc.EvidenceLabel,
				Key:   "argocd.argoproj.io/instance",
				Value: "",
				Path:  "metadata.labels",
			})
		}
	}

	return evidence
}

// buildTestTraceSummary constructs the summary from trace result (test version).
func buildTestTraceSummary(result *agent.TraceResult, chain []mapsvc.ChainNode) mapsvc.TraceSummary {
	summary := mapsvc.TraceSummary{
		OwnerType: normalizeToolToOwnerType(result.Tool),
	}

	// Find source and deployer in chain
	for _, node := range chain {
		switch node.Role {
		case mapsvc.RoleSource:
			// Find URL from original chain
			var url string
			for _, link := range result.Chain {
				if link.Kind == node.ID.Kind && link.Name == node.ID.Name {
					url = link.URL
					break
				}
			}
			summary.Source = &mapsvc.TraceSourceRef{
				Kind:      node.ID.Kind,
				Namespace: node.ID.Namespace,
				Name:      node.ID.Name,
				URL:       url,
			}
		case mapsvc.RoleDeployer:
			summary.Deployer = &mapsvc.ResourceID{
				Kind:      node.ID.Kind,
				Namespace: node.ID.Namespace,
				Name:      node.ID.Name,
			}
		}
	}

	return summary
}

// normalizeToolLower converts tool name to lowercase.
func normalizeToolLower(tool string) string {
	switch tool {
	case "Flux", "flux":
		return "flux"
	case "ArgoCD", "argocd", "Argo":
		return "argocd"
	case "Helm", "helm":
		return "helm"
	default:
		return ""
	}
}

// normalizeToolToOwnerType converts tool name to owner type.
func normalizeToolToOwnerType(tool string) string {
	switch tool {
	case "Flux", "flux":
		return "Flux"
	case "ArgoCD", "argocd", "Argo":
		return "ArgoCD"
	case "Helm", "helm":
		return "Helm"
	default:
		return "Native"
	}
}
