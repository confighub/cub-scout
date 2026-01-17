// Package unit provides unit tests for ConfigHub Agent.
package unit

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCubCLIOutputStructure validates that cub CLI returns properly structured JSON.
// The cub CLI returns nested objects with entity-specific wrappers.
//
// Structure:
//   - worker list: [{BridgeWorker: {Slug, Condition, ...}, Space: {...}}]
//   - target list: [{Target: {Slug, DisplayName, ...}, BridgeWorker: {...}, Space: {...}}]
//   - unit list: [{Unit: {Slug, HeadRevisionNum, ...}, Space: {...}, FromLink: [...]}]
//
// See: https://github.com/confighubai/confighub-agent/issues/1
func TestCubCLIOutputStructure(t *testing.T) {
	// Skip if cub not available
	RequireCubAuth(t)

	t.Run("unit list returns nested Unit objects", func(t *testing.T) {
		output := RunCub(t, "unit", "list", "--json")

		var units []map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &units))

		if len(units) == 0 {
			t.Skip("No units in space")
		}

		unit := units[0]

		// Units have nested Unit structure (like workers/targets)
		unitObj, hasUnit := unit["Unit"].(map[string]interface{})
		assert.True(t, hasUnit, "Unit should have 'Unit' wrapper object")

		if hasUnit {
			_, hasSlug := unitObj["Slug"]
			assert.True(t, hasSlug, "Unit.Slug should exist")

			_, hasHeadRevisionNum := unitObj["HeadRevisionNum"]
			assert.True(t, hasHeadRevisionNum, "Unit.HeadRevisionNum should exist")
		}

		// Should also have Space wrapper
		_, hasSpace := unit["Space"].(map[string]interface{})
		assert.True(t, hasSpace, "Unit should have 'Space' wrapper object")
	})

	t.Run("worker list returns nested BridgeWorker objects", func(t *testing.T) {
		output := RunCub(t, "worker", "list", "--json")

		var workers []map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &workers))

		if len(workers) == 0 {
			t.Skip("No workers in space")
		}

		worker := workers[0]

		// Workers have nested BridgeWorker structure
		bridgeWorker, hasBridgeWorker := worker["BridgeWorker"].(map[string]interface{})
		assert.True(t, hasBridgeWorker, "Worker should have 'BridgeWorker' wrapper object")

		if hasBridgeWorker {
			slug, hasSlug := bridgeWorker["Slug"]
			assert.True(t, hasSlug, "BridgeWorker should have 'Slug' field")
			assert.NotEqual(t, "null", slug, "BridgeWorker Slug should not be string 'null'")

			condition, hasCondition := bridgeWorker["Condition"]
			assert.True(t, hasCondition, "BridgeWorker should have 'Condition' field")
			assert.NotEqual(t, "unknown", condition, "BridgeWorker Condition should not be 'unknown'")
		}

		// Should also have Space wrapper
		_, hasSpace := worker["Space"].(map[string]interface{})
		assert.True(t, hasSpace, "Worker should have 'Space' wrapper object")
	})

	t.Run("target list returns nested Target objects", func(t *testing.T) {
		output := RunCub(t, "target", "list", "--json")

		var targets []map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(output), &targets))

		if len(targets) == 0 {
			t.Skip("No targets in space")
		}

		target := targets[0]

		// Targets have nested Target structure
		targetObj, hasTarget := target["Target"].(map[string]interface{})
		if hasTarget {
			slug, hasSlug := targetObj["Slug"]
			assert.True(t, hasSlug, "Target should have 'Slug' field")
			assert.NotEqual(t, "null", slug, "Target Slug should not be string 'null'")
		} else {
			// Some target list responses may use BridgeWorker wrapper instead
			bridgeWorker, hasBridgeWorker := target["BridgeWorker"].(map[string]interface{})
			assert.True(t, hasBridgeWorker, "Target should have either 'Target' or 'BridgeWorker' wrapper")
			if hasBridgeWorker {
				slug, hasSlug := bridgeWorker["Slug"]
				assert.True(t, hasSlug, "BridgeWorker should have 'Slug' field")
				assert.NotEqual(t, "null", slug, "BridgeWorker Slug should not be string 'null'")
			}
		}
	})
}

// TestCubCLINoNullSlugs verifies that cub CLI never returns null slugs.
// Null slugs indicate a parsing error in the CLI output.
func TestCubCLINoNullSlugs(t *testing.T) {
	RequireCubAuth(t)

	t.Run("units have non-null slugs", func(t *testing.T) {
		var units []map[string]interface{}
		RunCubJSON(t, &units, "unit", "list")

		for i, unit := range units {
			// Extract from nested Unit wrapper
			unitObj, ok := unit["Unit"].(map[string]interface{})
			if !ok {
				t.Errorf("Unit %d missing Unit wrapper", i)
				continue
			}
			slug, _ := unitObj["Slug"].(string)
			assert.NotEmpty(t, slug, "Unit %d has empty Slug", i)
			assert.NotEqual(t, "null", slug, "Unit %d has 'null' Slug", i)
		}
	})

	t.Run("workers have non-null slugs", func(t *testing.T) {
		var workers []map[string]interface{}
		RunCubJSON(t, &workers, "worker", "list")

		for i, worker := range workers {
			// Extract from nested BridgeWorker
			bridgeWorker, ok := worker["BridgeWorker"].(map[string]interface{})
			if !ok {
				t.Errorf("Worker %d missing BridgeWorker wrapper", i)
				continue
			}
			slug, _ := bridgeWorker["Slug"].(string)
			assert.NotEmpty(t, slug, "Worker %d has empty Slug", i)
			assert.NotEqual(t, "null", slug, "Worker %d has 'null' Slug", i)
		}
	})

	t.Run("targets have non-null slugs", func(t *testing.T) {
		var targets []map[string]interface{}
		RunCubJSON(t, &targets, "target", "list")

		for i, target := range targets {
			// Try Target wrapper first, fall back to BridgeWorker
			var slug string
			if targetObj, ok := target["Target"].(map[string]interface{}); ok {
				slug, _ = targetObj["Slug"].(string)
			} else if bridgeWorker, ok := target["BridgeWorker"].(map[string]interface{}); ok {
				slug, _ = bridgeWorker["Slug"].(string)
			}
			assert.NotEmpty(t, slug, "Target %d has empty Slug", i)
			assert.NotEqual(t, "null", slug, "Target %d has 'null' Slug", i)
		}
	})
}

// JQPathPattern documents the correct jq paths for cub CLI output.
// Use this as a reference when parsing cub CLI JSON.
//
// NOTE: The cub CLI uses nested structures for all entity types:
//   - Workers: .BridgeWorker.Slug, .BridgeWorker.Condition
//   - Targets: .Target.Slug or .BridgeWorker.Slug (depends on context)
//   - Units: .Unit.Slug, .Unit.HeadRevisionNum (nested structure)
var JQPathPatterns = map[string]struct {
	Correct string
	Example string
}{
	"unit_slug": {
		Correct: ".Unit.Slug",
		Example: `cub unit list --json | jq '.[0].Unit.Slug'`,
	},
	"unit_revision": {
		Correct: ".Unit.HeadRevisionNum",
		Example: `cub unit list --json | jq '.[0].Unit.HeadRevisionNum'`,
	},
	"unit_target": {
		Correct: ".Unit.TargetSlug",
		Example: `cub unit list --json | jq '.[0].Unit.TargetSlug'`,
	},
	"worker_slug": {
		Correct: ".BridgeWorker.Slug",
		Example: `cub worker list --json | jq '.[0].BridgeWorker.Slug'`,
	},
	"worker_condition": {
		Correct: ".BridgeWorker.Condition",
		Example: `cub worker list --json | jq '.[0].BridgeWorker.Condition'`,
	},
	"target_slug": {
		Correct: ".Target.Slug or .BridgeWorker.Slug",
		Example: `cub target list --json | jq '.[0].Target.Slug // .[0].BridgeWorker.Slug'`,
	},
}

// TestJQPathDocumentation is a documentation test that shows correct jq usage.
func TestJQPathDocumentation(t *testing.T) {
	for name, pattern := range JQPathPatterns {
		t.Run(name, func(t *testing.T) {
			t.Logf("Correct: %s", pattern.Correct)
			t.Logf("Example: %s", pattern.Example)
		})
	}
}
