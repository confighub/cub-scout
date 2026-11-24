// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFilterThreeWayUnrecordedKeepsOnlyUnrecordedAndPreservesSummaryAndOmissions(t *testing.T) {
	report := threeWayReport{
		Scope:   "namespace/prod",
		Context: "prod-east",
		Omissions: []threeWayOmission{
			{Resource: "secrets", Reason: "forbidden"},
		},
		Summary: threeWaySummary{
			TotalResources:      4,
			ConnectedResources:  3,
			MismatchedResources: 2,
		},
		Resources: []threeWayResourceEntry{
			{Result: compareResourceResult{Resource: "Deployment/in-sync", Namespace: "prod"}, Pattern: PatternAgreed},
			{Result: compareResourceResult{Resource: "Deployment/pending", Namespace: "prod"}, Pattern: PatternRolloutPending},
			{Result: compareResourceResult{Resource: "Deployment/hotfixed", Namespace: "prod"}, Pattern: PatternChangeInProgress},
			{Result: compareResourceResult{Resource: "Deployment/manual-orphan", Namespace: "prod"}, Pattern: PatternUnlinked},
		},
	}

	filtered := filterThreeWayUnrecorded(report)

	if !filtered.UnrecordedOnly {
		t.Fatalf("expected UnrecordedOnly=true")
	}
	if len(filtered.Resources) != 2 {
		t.Fatalf("filtered resources = %d, want 2: %+v", len(filtered.Resources), filtered.Resources)
	}
	if filtered.Resources[0].Result.Resource != "Deployment/hotfixed" || filtered.Resources[1].Result.Resource != "Deployment/manual-orphan" {
		t.Fatalf("unexpected filtered resources: %+v", filtered.Resources)
	}
	if filtered.Summary.TotalResources != 4 {
		t.Fatalf("summary totalResources = %d, want 4", filtered.Summary.TotalResources)
	}
	if len(filtered.Omissions) != 1 || filtered.Omissions[0].Resource != "secrets" {
		t.Fatalf("omissions must be preserved: %+v", filtered.Omissions)
	}

	raw, err := json.Marshal(filtered)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"unrecordedOnly":true`) {
		t.Fatalf("expected unrecordedOnly in JSON: %s", raw)
	}
}

func TestCompareThreeWayMCPUnrecordedOnlyArg(t *testing.T) {
	gw := newMCPGatewayWithMode(nil, nil, true)
	tool, ok := gw.tools["compare_three_way"]
	if !ok {
		t.Fatal("compare_three_way MCP tool not registered")
	}
	args, err := tool.BuildArgs(map[string]interface{}{"scope": "namespace/prod", "unrecorded_only": true})
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"compare three-way --format json", "--scope namespace/prod", "--unrecorded-only"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}
