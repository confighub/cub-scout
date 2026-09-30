package main

import (
	"encoding/json"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// This JSON-level check deliberately avoids referencing HealthMeasurement so
// it can run against pre-feature source and demonstrate the old contract fails.
func TestExplainJSONReportsHealthCoverageSeparatelyFromRollout(t *testing.T) {
	summary := buildExplainSummaryFromFailure("Deployment", "api", "prod", nil)
	summary.CurrentChange = &agent.RolloutDecision{Verdict: agent.VerdictPASS}

	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal explain summary: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode explain summary: %v", err)
	}
	if decoded["health"] != "Unavailable" {
		t.Fatalf("legacy health = %v, want Unavailable", decoded["health"])
	}
	measurement, ok := decoded["healthMeasurement"].(map[string]interface{})
	if !ok || measurement["status"] != "unmeasured" {
		t.Fatalf("healthMeasurement = %v, want status unmeasured", decoded["healthMeasurement"])
	}
	change, ok := decoded["currentChange"].(map[string]interface{})
	if !ok || change["verdict"] != string(agent.VerdictPASS) {
		t.Fatalf("currentChange = %v, want independent PASS", decoded["currentChange"])
	}
}
