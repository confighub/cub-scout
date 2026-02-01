// Package ownership provides golden tests for ownership detection precedence.
//
// These tests verify that ownership detection follows the rules documented in:
// docs/reference/ownership-precedence.md
//
// Test scenarios cover:
//   - Explicit labels beat inferred (ownerRef) ownership
//   - Inferred ownership beats default (unknown)
//   - Multiple signals resolve deterministically per precedence order
//   - No signals result in "Native" (CLI display for unmanaged)
//   - Conflicting explicit signals resolve per first-match-wins
//
// Each test loads a fixture YAML and compares the ownership detection result
// against a golden file. The output format matches the CLI JSON contract
// defined in docs/reference/cli-contract.md.
package ownership

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// OwnershipOutput represents the CLI-level ownership output format.
// This matches the JSON contract in docs/reference/cli-contract.md.
type OwnershipOutput struct {
	Resource   ResourceRef `json:"resource"`
	Owner      OwnerRef    `json:"owner"`
	Confidence string      `json:"confidence,omitempty"`
	Source     string      `json:"source,omitempty"`
}

// ResourceRef identifies the resource being checked.
type ResourceRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// OwnerRef describes the detected owner.
type OwnerRef struct {
	Type      string `json:"type"`
	SubType   string `json:"subType,omitempty"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// updateGolden controls whether to update golden files.
// Set via: go test ./test/golden/ownership -update
var updateGolden = os.Getenv("UPDATE_GOLDEN") == "1"

func init() {
	// Also check for -update flag via testing package
	for _, arg := range os.Args {
		if arg == "-update" || arg == "--update" {
			updateGolden = true
			break
		}
	}
}

// TestOwnershipPrecedence_ExplicitBeatsInferred verifies that explicit labels
// (e.g., Flux) take precedence over inferred ownership (K8s ownerRef).
func TestOwnershipPrecedence_ExplicitBeatsInferred(t *testing.T) {
	runOwnershipGoldenTest(t, "01-explicit-beats-inferred")
}

// TestOwnershipPrecedence_InferredBeatsDefault verifies that inferred ownership
// (K8s ownerRef) takes precedence over no signals (unknown).
func TestOwnershipPrecedence_InferredBeatsDefault(t *testing.T) {
	runOwnershipGoldenTest(t, "02-inferred-beats-default")
}

// TestOwnershipPrecedence_MultipleInferredDeterministic verifies that when
// multiple ownership signals are present, the winner is deterministic per
// the precedence order documented in ownership-precedence.md.
func TestOwnershipPrecedence_MultipleInferredDeterministic(t *testing.T) {
	runOwnershipGoldenTest(t, "03-multiple-inferred-deterministic")
}

// TestOwnershipPrecedence_NoOwnerSignals verifies that resources without
// ownership signals are classified as "Native" (unmanaged) in CLI output.
func TestOwnershipPrecedence_NoOwnerSignals(t *testing.T) {
	runOwnershipGoldenTest(t, "04-no-owner-signals")
}

// TestOwnershipPrecedence_ConflictingExplicitSignals verifies that when
// multiple explicit signals conflict, resolution follows first-match-wins
// per the precedence order.
func TestOwnershipPrecedence_ConflictingExplicitSignals(t *testing.T) {
	runOwnershipGoldenTest(t, "05-conflicting-explicit-signals")
}

// runOwnershipGoldenTest loads a fixture, runs ownership detection, and
// compares the output against the golden file.
func runOwnershipGoldenTest(t *testing.T, name string) {
	t.Helper()

	// Load fixture
	fixturePath := filepath.Join("testdata", name+".yaml")
	fixtureData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", fixturePath, err)
	}

	// Parse YAML to unstructured
	var obj map[string]interface{}
	if err := yaml.Unmarshal(fixtureData, &obj); err != nil {
		t.Fatalf("failed to parse fixture YAML: %v", err)
	}

	resource := &unstructured.Unstructured{Object: obj}

	// Detect ownership
	ownership := agent.DetectOwnership(resource)

	// Format output to match CLI JSON contract
	output := OwnershipOutput{
		Resource: ResourceRef{
			Kind:      resource.GetKind(),
			Name:      resource.GetName(),
			Namespace: resource.GetNamespace(),
		},
		Owner: OwnerRef{
			Type:      formatOwnerType(ownership.Type),
			SubType:   ownership.SubType,
			Name:      ownership.Name,
			Namespace: ownership.Namespace,
		},
		Confidence: ownership.Confidence,
		Source:     ownership.Source,
	}

	// Marshal to JSON with stable formatting
	actual, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal output: %v", err)
	}
	actualStr := string(actual) + "\n"

	// Golden file path
	goldenPath := filepath.Join("testdata", name+".golden.json")

	if updateGolden {
		if err := os.WriteFile(goldenPath, []byte(actualStr), 0644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	// Read and compare golden
	expected, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		t.Fatalf("golden file not found: %s\n\nActual output:\n%s\n\nRun with UPDATE_GOLDEN=1 to create it:\n  UPDATE_GOLDEN=1 go test ./test/golden/ownership/...", goldenPath, actualStr)
	}
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if actualStr != string(expected) {
		t.Errorf("output does not match golden file %s\n\n--- EXPECTED ---\n%s\n--- ACTUAL ---\n%s",
			goldenPath, string(expected), actualStr)
	}
}

// formatOwnerType converts internal owner type constants to CLI display format.
// Reference: docs/reference/cli-contract.md - "Owner Values" table.
func formatOwnerType(t string) string {
	switch t {
	case agent.OwnerFlux:
		return "Flux"
	case agent.OwnerArgo:
		return "ArgoCD"
	case agent.OwnerHelm:
		return "Helm"
	case agent.OwnerTerraform:
		return "Terraform"
	case agent.OwnerConfigHub:
		return "ConfigHub"
	case agent.OwnerCrossplane:
		return "Crossplane"
	case agent.OwnerKubernetes, agent.OwnerUnknown:
		// Both k8s (ownerRef-only) and unknown (no signals) map to "Native" in CLI
		// Reference: internal/mapsvc/types.go DisplayOwner()
		return "Native"
	default:
		return strings.Title(t)
	}
}
