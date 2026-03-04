//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"testing"
)

// TestTreeGitLineage_ApplicationSetLinkStatus validates that unresolved
// ApplicationSet lineage is surfaced as orphan (explicit) or unknown (inferred)
// in tree git --format json output.
func TestTreeGitLineage_ApplicationSetLinkStatus(t *testing.T) {
	skipIfNoCluster(t)

	out := runCubAgentAllowFailures(t, "tree", "git", "--format", "json")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Skipf("tree git --format json unavailable or non-JSON output: %v", err)
	}

	appsAny, ok := result["argoApplications"].([]interface{})
	if !ok || len(appsAny) == 0 {
		t.Skip("No Argo applications found in tree git output")
	}

	appSetAny, _ := result["applicationSets"].([]interface{})
	appSetsByNS := map[string]map[string]struct{}{}
	appSetsByName := map[string]struct{}{}
	for _, raw := range appSetAny {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		namespace, _ := item["namespace"].(string)
		if name == "" {
			continue
		}
		if _, exists := appSetsByNS[namespace]; !exists {
			appSetsByNS[namespace] = map[string]struct{}{}
		}
		appSetsByNS[namespace][name] = struct{}{}
		appSetsByName[name] = struct{}{}
	}

	foundUnresolved := false
	for _, raw := range appsAny {
		app, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		genBy, _ := app["generatedByApplicationSet"].(string)
		if genBy == "" {
			continue
		}
		namespace, _ := app["namespace"].(string)

		_, foundInNS := appSetsByNS[namespace][genBy]
		_, foundByName := appSetsByName[genBy]
		if foundInNS || foundByName {
			continue
		}

		foundUnresolved = true
		status, _ := app["applicationSetLinkStatus"].(string)
		confidence, _ := app["lineageConfidence"].(string)

		if confidence == "explicit" {
			if status != "orphan" {
				t.Fatalf("explicit unresolved appset link must be orphan, got status=%q app=%v", status, app["name"])
			}
			continue
		}

		if status != "unknown" {
			t.Fatalf("inferred unresolved appset link must be unknown, got status=%q app=%v", status, app["name"])
		}
	}

	if !foundUnresolved {
		t.Skip("No unresolved ApplicationSet lineage found in cluster")
	}
}
