// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v3/internal/scan"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

// Per-answer size ceilings (Agent plan, package C; four bytes is roughly one
// token). Just as mcp_catalog_size_test.go holds the MCP tool catalog to
// declared byte ceilings, this test holds the `answer` block of every
// answer-carrying command to a declared ceiling on a representative
// deterministic fixture, rounded up to the next hundred bytes. A change that
// grows an answer block past its ceiling fails here: raise the ceiling in the
// same change, on purpose, and say why. Shrinking needs no permission; lower
// the ceiling when you do.
// Measured 2026-11-24 on deterministic fixtures, rounded up to the next
// hundred bytes.
var mcpAnswerBlockCeilings = map[string]int{
	"compare_source_truth": 400,  // 336 bytes
	"doctor":               500,  // 418 bytes
	"explain":              900,  // 821 bytes
	"gitops_settings":      1000, // 962 bytes
	"gitops_status":        900,  // 842 bytes
	"map":                  400,  // 387 bytes
	"release_check":        1600, // 1548 bytes
	"scan":                 700,  // 600 bytes
	"trace":                900,  // 828 bytes
}

func TestAnswerBlocksStayWithinDeclaredSizeCeilings(t *testing.T) {
	result := recordedModelplaneTrace(t)
	traceOut := convertTraceToV014(result, "ModelDeployment", "smollm", "ml-team", nil)

	sourceTruthEv := agent.Derive(agent.StrategyConfigHubOCIFlux, agent.SourceTruthSurfaces{
		Controller: &agent.ControllerSurface{Kind: "Flux", Source: "oci://registry/space/demo", RevisionOrDigest: "latest@sha256:0828", Health: "Ready"},
		Runtime:    &agent.RuntimeSurface{Resource: "Deployment/api in demo", Field: "spec.template.spec.containers[0].image", Value: "example/api:1", Health: "Current"},
	})
	sourceTruthAns := buildSourceTruthAnswer(sourceTruthEv, agent.StrategyConfigHubOCIFlux, "Deployment", "api", "demo", observedAtForTests)

	gitopsSummary := GitOpsSummary{
		Context:   "prod-east",
		Deployers: []DeployerStatus{{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: false, Reason: "BuildFailed", Stage: "build"}},
	}
	gitopsAns := buildGitOpsStatusAnswer(gitopsSummary, observedAtForTests, nil)

	explainAns := buildExplainAnswer(buildExplainSummary(result), observedAtForTests)

	doctorSummary := DoctorSummary{
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 4},
		Risks:     DoctorRiskSummary{Warning: 1},
	}
	doctorAns := buildDoctorAnswer(doctorSummary, observedAtForTests)

	mapAns := buildMapListAnswer(
		[]MapEntry{{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux"}},
		nil, "", nil, observedAtForTests,
	)

	scanAns := buildScanAnswer(&scan.CombinedResult{
		KubernetesContext: "prod-east",
		State: &agent.StateScanResult{
			Findings: []agent.StuckFinding{{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Severity: "warning", Reason: "BuildFailed"}},
		},
	}, "prod", nil, observedAtForTests)

	settingsAns := buildDeliverySettingsAnswer(
		settingsReport(t, settingsFixtureClient(), deliverySettingsParams{}),
		nil, observedAtForTests,
	)

	relReport := agent.NewReleaseCheckReport(agent.BoundedResourceRef{Kind: "Kustomization", Name: "apps", Namespace: "flux-system"}, "prod-east", "prod-east", 10, observedAtForTests)
	for i := range relReport.Stages {
		switch relReport.Stages[i].Name {
		case "bundle", "controller", "configuration":
			relReport.Stages[i].Verdict = "PASS"
			relReport.Stages[i].Reason = "fixture PASS"
		}
	}
	relReport.RequestCounts = agent.BoundedReadCounts{Discovery: 2, Object: 7}
	relReport.Finish(observedAtForTests.Add(1500 * time.Millisecond))
	releaseAns := buildReleaseCheckAnswer(relReport)

	answers := map[string]interface{}{
		"trace":                traceOut.Answer,
		"compare_source_truth": sourceTruthAns,
		"gitops_status":        gitopsAns,
		"explain":              explainAns,
		"doctor":               doctorAns,
		"map":                  mapAns,
		"scan":                 scanAns,
		"gitops_settings":      settingsAns,
		"release_check":        releaseAns,
	}

	names := make([]string, 0, len(answers))
	for name := range answers {
		names = append(names, name)
	}
	sort.Strings(names)

	totalBytes := 0
	for _, name := range names {
		raw, err := json.Marshal(answers[name])
		if err != nil {
			t.Fatalf("%s: marshal answer: %v", name, err)
		}
		size := len(raw)
		totalBytes += size
		tokens := (size + 3) / 4
		ceiling, ok := mcpAnswerBlockCeilings[name]
		t.Logf("%-22s %5d bytes (~%3d tokens), ceiling %d", name, size, tokens, ceiling)
		switch {
		case !ok:
			t.Errorf("%s (%d bytes) declares no ceiling in mcpAnswerBlockCeilings", name, size)
		case size == 0 || string(raw) == "null":
			t.Errorf("%s produced an empty or null answer block", name)
		case size > ceiling:
			t.Errorf("%s answer block is %d bytes, over its ceiling of %d; shorten it or raise the ceiling on purpose", name, size, ceiling)
		}
	}
	t.Logf("total across %d answers: %d bytes (~%d tokens)", len(names), totalBytes, (totalBytes+3)/4)
}
