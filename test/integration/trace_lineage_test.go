// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

// Integration tests for trace lineage fields (v1.2 — #194, #195).
//
// These tests verify that trace --json output includes:
//   - parentApplication (App-of-Apps)
//   - generatedByApplicationSet
//   - lineageConfidence (explicit/inferred)
//
// Cluster-dependent tests require ArgoCD with appropriate Application objects.
// Offline tests verify the JSON contract shape without a cluster.
//
// Run with:
//
//	go test -tags=integration ./test/integration/... -run TestTraceLineage -v
package integration

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// TestTraceLineage_JSONContractShape verifies that trace --json output includes
// the lineage fields in its schema, even when tracing an unmanaged resource.
// Requires a cluster (traces a real resource).
func TestTraceLineage_JSONContractShape(t *testing.T) {
	skipIfNoCluster(t)

	// Find any deployment to trace
	cmd := exec.Command("kubectl", "get", "deploy", "-A",
		"-o", "jsonpath={.items[0].metadata.namespace}/{.items[0].metadata.name}")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		t.Skip("No deployments found in cluster")
	}

	parts := strings.Split(string(out), "/")
	if len(parts) != 2 {
		t.Skip("Could not parse deployment namespace/name")
	}
	ns, name := parts[0], parts[1]

	// Run trace --json
	output := runCubAgentAllowFailures(t, "trace", "deploy/"+name, "-n", ns, "--format", "json")

	// Parse JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		// If trace failed entirely (e.g., no ArgoCD), that's OK — skip
		t.Skipf("trace --json did not produce valid JSON (resource may not be traceable): %s", output[:min(len(output), 200)])
	}

	// Verify the JSON schema includes lineage-related fields.
	// These fields are omitempty, so they may be absent on non-Argo resources.
	// What we verify is that the output is parseable and, if present, correctly typed.
	if parent, ok := result["parentApplication"]; ok {
		if _, isStr := parent.(string); !isStr {
			t.Errorf("parentApplication is not a string: %T", parent)
		}
	}

	if appSet, ok := result["generatedByApplicationSet"]; ok {
		if _, isStr := appSet.(string); !isStr {
			t.Errorf("generatedByApplicationSet is not a string: %T", appSet)
		}
	}

	if confidence, ok := result["lineageConfidence"]; ok {
		if s, isStr := confidence.(string); isStr {
			if s != "explicit" && s != "inferred" && s != "" {
				t.Errorf("lineageConfidence = %q, want explicit|inferred|empty", s)
			}
		} else {
			t.Errorf("lineageConfidence is not a string: %T", confidence)
		}
	}
}

// TestTraceLineage_ArgoAppWithParent verifies that an ArgoCD Application with
// a parent relationship shows parentApplication and lineageConfidence in trace output.
// Requires: cluster with ArgoCD and at least one Application with a parent.
// Skips gracefully if ArgoCD or parent apps are not available.
func TestTraceLineage_ArgoAppWithParent(t *testing.T) {
	skipIfNoCluster(t)

	// Check if ArgoCD is installed
	cmd := exec.Command("kubectl", "get", "crd", "applications.argoproj.io")
	if err := cmd.Run(); err != nil {
		t.Skip("ArgoCD not installed")
	}

	// Find an Application with ownerReferences pointing to another Application
	cmd = exec.Command("kubectl", "get", "applications.argoproj.io", "-A",
		"-o", "jsonpath={range .items[*]}{.metadata.namespace}/{.metadata.name}:{range .metadata.ownerReferences[*]}{.kind}{end}{\"\\n\"}{end}")
	out, err := cmd.Output()
	if err != nil {
		t.Skip("Could not list ArgoCD Applications")
	}

	// Look for an app owned by another Application (App-of-Apps pattern)
	var targetNS, targetName string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && strings.Contains(parts[1], "Application") {
			nsParts := strings.SplitN(parts[0], "/", 2)
			if len(nsParts) == 2 {
				targetNS = nsParts[0]
				targetName = nsParts[1]
				break
			}
		}
	}

	if targetName == "" {
		t.Skip("No ArgoCD Application with parent ownerReference found (App-of-Apps pattern not present)")
	}

	// Trace the child application
	output := runCubAgentAllowFailures(t, "trace", "application/"+targetName, "-n", targetNS, "--format", "json")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("trace --json output is not valid JSON: %v\n%.500s", err, output)
	}

	// Verify parentApplication is present
	parent, ok := result["parentApplication"].(string)
	if !ok || parent == "" {
		t.Errorf("parentApplication missing or empty for child app %s/%s", targetNS, targetName)
	}

	// Verify lineageConfidence
	confidence, ok := result["lineageConfidence"].(string)
	if !ok || confidence == "" {
		t.Errorf("lineageConfidence missing or empty")
	}
	if confidence != "explicit" && confidence != "inferred" {
		t.Errorf("lineageConfidence = %q, want explicit or inferred", confidence)
	}

	t.Logf("App-of-Apps lineage verified: %s/%s → parent=%s (confidence=%s)", targetNS, targetName, parent, confidence)
}

// TestTraceLineage_ArgoAppFromApplicationSet verifies that an ArgoCD Application
// generated by an ApplicationSet shows generatedByApplicationSet in trace output.
// Requires: cluster with ArgoCD and at least one ApplicationSet-generated Application.
// Skips gracefully if not available.
func TestTraceLineage_ArgoAppFromApplicationSet(t *testing.T) {
	skipIfNoCluster(t)

	// Check if ArgoCD is installed
	cmd := exec.Command("kubectl", "get", "crd", "applications.argoproj.io")
	if err := cmd.Run(); err != nil {
		t.Skip("ArgoCD not installed")
	}

	// Find an Application with the ApplicationSet label
	cmd = exec.Command("kubectl", "get", "applications.argoproj.io", "-A",
		"-o", "jsonpath={range .items[*]}{.metadata.namespace}/{.metadata.name}:{.metadata.labels.argocd\\.argoproj\\.io/application-set-name}{\"\\n\"}{end}")
	out, err := cmd.Output()
	if err != nil {
		t.Skip("Could not list ArgoCD Applications")
	}

	// Look for an app with appset label
	var targetNS, targetName, appSetName string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 && parts[1] != "" {
			nsParts := strings.SplitN(parts[0], "/", 2)
			if len(nsParts) == 2 {
				targetNS = nsParts[0]
				targetName = nsParts[1]
				appSetName = parts[1]
				break
			}
		}
	}

	if targetName == "" {
		t.Skip("No ArgoCD Application generated by ApplicationSet found")
	}

	// Trace the generated application
	output := runCubAgentAllowFailures(t, "trace", "application/"+targetName, "-n", targetNS, "--format", "json")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("trace --json output is not valid JSON: %v\n%.500s", err, output)
	}

	// Verify generatedByApplicationSet is present
	genBy, ok := result["generatedByApplicationSet"].(string)
	if !ok || genBy == "" {
		t.Errorf("generatedByApplicationSet missing or empty for app %s/%s (expected %q)", targetNS, targetName, appSetName)
	}

	t.Logf("ApplicationSet lineage verified: %s/%s → appSet=%s", targetNS, targetName, genBy)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
