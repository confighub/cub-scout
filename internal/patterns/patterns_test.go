package patterns

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/internal/gitctx"
	"github.com/confighub/cub-scout/internal/graph"
)

// buildTestGraph creates a complete ownership chain for testing.
func buildTestGraph() *graph.Graph {
	g := graph.NewGraph("test-cluster")

	// Deployment
	g.AddNode(graph.Node{
		ID:         "test-cluster/default/Deployment/nginx",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "Deployment",
		Name:       "nginx",
		APIVersion: "apps/v1",
		Labels:     map[string]string{"app": "nginx"},
	})

	// ReplicaSet
	g.AddNode(graph.Node{
		ID:         "test-cluster/default/ReplicaSet/nginx-abc123",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "ReplicaSet",
		Name:       "nginx-abc123",
		APIVersion: "apps/v1",
		Labels:     map[string]string{"app": "nginx", "pod-template-hash": "abc123"},
	})

	// Pod
	g.AddNode(graph.Node{
		ID:         "test-cluster/default/Pod/nginx-abc123-xyz789",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "Pod",
		Name:       "nginx-abc123-xyz789",
		APIVersion: "v1",
		Labels:     map[string]string{"app": "nginx"},
	})

	// Deployment → ReplicaSet edge
	g.AddEdge(graph.Edge{
		From: "test-cluster/default/Deployment/nginx",
		To:   "test-cluster/default/ReplicaSet/nginx-abc123",
		Type: graph.EdgeTypeOwns,
		Evidence: []graph.Evidence{{
			Field:  "metadata.ownerReferences",
			Reason: "ReplicaSet nginx-abc123 has ownerReference to Deployment nginx",
		}},
	})

	// ReplicaSet → Pod edge
	g.AddEdge(graph.Edge{
		From: "test-cluster/default/ReplicaSet/nginx-abc123",
		To:   "test-cluster/default/Pod/nginx-abc123-xyz789",
		Type: graph.EdgeTypeOwns,
		Evidence: []graph.Evidence{{
			Field:  "metadata.ownerReferences",
			Reason: "Pod nginx-abc123-xyz789 has ownerReference to ReplicaSet nginx-abc123",
		}},
	})

	return g
}

// buildTestGraphWithGitOps adds GitOps CRDs to the test graph.
func buildTestGraphWithGitOps() *graph.Graph {
	g := buildTestGraph()

	// Add Argo CD Application
	g.AddNode(graph.Node{
		ID:         "test-cluster/argocd/Application/my-app",
		Cluster:    "test-cluster",
		Namespace:  "argocd",
		Kind:       "Application",
		Name:       "my-app",
		APIVersion: "argoproj.io/v1alpha1",
		Labels:     map[string]string{"app.kubernetes.io/name": "my-app"},
	})

	// Add Flux Kustomization
	g.AddNode(graph.Node{
		ID:         "test-cluster/flux-system/Kustomization/flux-system",
		Cluster:    "test-cluster",
		Namespace:  "flux-system",
		Kind:       "Kustomization",
		Name:       "flux-system",
		APIVersion: "kustomize.toolkit.fluxcd.io/v1",
	})

	return g
}

func TestRegistry_List(t *testing.T) {
	patterns := List()

	// Should have at least 2 patterns registered (ownership chain + gitops presence)
	if len(patterns) < 2 {
		t.Errorf("expected at least 2 patterns, got %d", len(patterns))
	}

	// Should be sorted by ID
	for i := 1; i < len(patterns); i++ {
		if patterns[i].ID < patterns[i-1].ID {
			t.Errorf("patterns not sorted: %s comes after %s", patterns[i].ID, patterns[i-1].ID)
		}
	}
}

func TestRegistry_Get(t *testing.T) {
	// Known patterns should be found
	p := Get("k8s.ownership_chain_complete")
	if p == nil {
		t.Fatal("expected to find k8s.ownership_chain_complete pattern")
	}
	if p.ID != "k8s.ownership_chain_complete" {
		t.Errorf("expected ID k8s.ownership_chain_complete, got %s", p.ID)
	}

	// Unknown patterns should return nil
	p = Get("nonexistent.pattern")
	if p != nil {
		t.Errorf("expected nil for nonexistent pattern, got %v", p)
	}
}

func TestDetectAll_Deterministic(t *testing.T) {
	g := buildTestGraph()

	result1 := DetectAll(g)
	result2 := DetectAll(g)

	// Results should be identical
	if len(result1.Patterns) != len(result2.Patterns) {
		t.Errorf("results have different number of patterns: %d vs %d",
			len(result1.Patterns), len(result2.Patterns))
	}

	for i := range result1.Patterns {
		if result1.Patterns[i].ID != result2.Patterns[i].ID {
			t.Errorf("pattern order differs at index %d: %s vs %s",
				i, result1.Patterns[i].ID, result2.Patterns[i].ID)
		}
	}
}

func TestDetectOne(t *testing.T) {
	g := buildTestGraph()

	// Known pattern
	pr := DetectOne(g, "k8s.ownership_chain_complete")
	if pr == nil {
		t.Fatal("expected result for known pattern")
	}
	if pr.ID != "k8s.ownership_chain_complete" {
		t.Errorf("expected ID k8s.ownership_chain_complete, got %s", pr.ID)
	}

	// Unknown pattern
	pr = DetectOne(g, "nonexistent.pattern")
	if pr != nil {
		t.Errorf("expected nil for unknown pattern, got %v", pr)
	}
}

func TestOwnershipChainComplete_Pass(t *testing.T) {
	g := buildTestGraph()

	pr := DetectOne(g, "k8s.ownership_chain_complete")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusPass {
		t.Errorf("expected pass status for complete chain, got %s", pr.Status)
	}
}

func TestOwnershipChainComplete_NoDeployments(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	pr := DetectOne(g, "k8s.ownership_chain_complete")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusSkip {
		t.Errorf("expected skip status for empty graph, got %s", pr.Status)
	}
}

func TestOwnershipChainComplete_OrphanedReplicaSet(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Deployment
	g.AddNode(graph.Node{
		ID:         "test-cluster/default/Deployment/nginx",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "Deployment",
		Name:       "nginx",
		APIVersion: "apps/v1",
	})

	// Orphaned ReplicaSet (no owning Deployment)
	g.AddNode(graph.Node{
		ID:         "test-cluster/default/ReplicaSet/orphan-rs",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "ReplicaSet",
		Name:       "orphan-rs",
		APIVersion: "apps/v1",
	})

	pr := DetectOne(g, "k8s.ownership_chain_complete")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusFail {
		t.Errorf("expected fail status for orphaned RS, got %s", pr.Status)
	}

	// Should have finding about orphaned RS
	found := false
	for _, f := range pr.Findings {
		if strings.Contains(f.Message, "orphan-rs") && strings.Contains(f.Message, "no owning Deployment") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finding about orphaned ReplicaSet")
	}
}

func TestGitOpsPresence_BothPresent(t *testing.T) {
	g := buildTestGraphWithGitOps()

	pr := DetectOne(g, "gitops.controller_presence")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusPass {
		t.Errorf("expected pass status for graph with GitOps CRDs, got %s", pr.Status)
	}

	// Should have findings for both Argo and Flux
	hasArgo := false
	hasFlux := false
	for _, f := range pr.Findings {
		if strings.Contains(f.Message, "Argo CD") {
			hasArgo = true
		}
		if strings.Contains(f.Message, "Flux") {
			hasFlux = true
		}
	}

	if !hasArgo {
		t.Error("expected Argo CD finding")
	}
	if !hasFlux {
		t.Error("expected Flux finding")
	}
}

func TestGitOpsPresence_NonePresent(t *testing.T) {
	g := buildTestGraph() // No GitOps CRDs

	pr := DetectOne(g, "gitops.controller_presence")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusFail {
		t.Errorf("expected fail status for graph without GitOps CRDs, got %s", pr.Status)
	}

	// Should have warning about no GitOps
	found := false
	for _, f := range pr.Findings {
		if strings.Contains(f.Message, "No GitOps controllers") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finding about no GitOps controllers")
	}
}

func TestRenderList(t *testing.T) {
	output := RenderList()

	// Should contain header
	if !strings.Contains(output, "PATTERNS LIST") {
		t.Error("expected PATTERNS LIST header")
	}

	// Should contain schema version
	if !strings.Contains(output, SchemaVersion) {
		t.Errorf("expected schema version %s", SchemaVersion)
	}

	// Should list known patterns
	if !strings.Contains(output, "k8s.ownership_chain_complete") {
		t.Error("expected ownership chain pattern in list")
	}
	if !strings.Contains(output, "gitops.controller_presence") {
		t.Error("expected gitops presence pattern in list")
	}
}

func TestRenderText_Deterministic(t *testing.T) {
	g := buildTestGraph()

	result := DetectAll(g)
	output1 := RenderText(result)
	output2 := RenderText(result)

	if output1 != output2 {
		t.Error("RenderText is not deterministic")
	}
}

func TestRenderJSON_Deterministic(t *testing.T) {
	g := buildTestGraph()

	result := DetectAll(g)
	output1, err1 := RenderJSON(result)
	output2, err2 := RenderJSON(result)

	if err1 != nil || err2 != nil {
		t.Fatalf("RenderJSON errors: %v, %v", err1, err2)
	}

	if output1 != output2 {
		t.Error("RenderJSON is not deterministic")
	}
}

func TestAnyFail(t *testing.T) {
	// Result with all pass
	passResult := Result{
		Patterns: []PatternResult{
			{Status: StatusPass},
			{Status: StatusPass},
		},
	}
	if AnyFail(passResult) {
		t.Error("expected false for all-pass result")
	}

	// Result with one fail
	failResult := Result{
		Patterns: []PatternResult{
			{Status: StatusPass},
			{Status: StatusFail},
		},
	}
	if !AnyFail(failResult) {
		t.Error("expected true for result with failures")
	}

	// Result with skip (not fail)
	skipResult := Result{
		Patterns: []PatternResult{
			{Status: StatusSkip},
			{Status: StatusPass},
		},
	}
	if AnyFail(skipResult) {
		t.Error("expected false for result with only skip/pass")
	}
}

func TestSortFindings(t *testing.T) {
	findings := []Finding{
		{Severity: SeverityInfo, Resource: "z-resource", Message: "message"},
		{Severity: SeverityError, Resource: "a-resource", Message: "message"},
		{Severity: SeverityWarning, Resource: "b-resource", Message: "message"},
	}

	sortFindings(findings)

	// Error should come first
	if findings[0].Severity != SeverityError {
		t.Errorf("expected error first, got %s", findings[0].Severity)
	}

	// Warning should come second
	if findings[1].Severity != SeverityWarning {
		t.Errorf("expected warning second, got %s", findings[1].Severity)
	}

	// Info should come last
	if findings[2].Severity != SeverityInfo {
		t.Errorf("expected info last, got %s", findings[2].Severity)
	}
}

// Track I (v0.8+) tests for enrichment fields

func TestSortFindings_SortsRefs(t *testing.T) {
	findings := []Finding{
		{
			Severity: SeverityWarning,
			Message:  "test",
			Refs:     []string{"z-ref", "a-ref", "m-ref"},
		},
	}

	sortFindings(findings)

	// Refs should be sorted alphabetically
	if findings[0].Refs[0] != "a-ref" {
		t.Errorf("expected refs sorted, got %v", findings[0].Refs)
	}
	if findings[0].Refs[1] != "m-ref" {
		t.Errorf("expected refs sorted, got %v", findings[0].Refs)
	}
	if findings[0].Refs[2] != "z-ref" {
		t.Errorf("expected refs sorted, got %v", findings[0].Refs)
	}
}

func TestSortFindings_SortsRemediationLinks(t *testing.T) {
	findings := []Finding{
		{
			Severity: SeverityWarning,
			Message:  "test",
			Remediation: &Remediation{
				Summary: "Fix it",
				Links:   []string{"https://z.com", "https://a.com"},
			},
		},
	}

	sortFindings(findings)

	// Links should be sorted alphabetically
	if findings[0].Remediation.Links[0] != "https://a.com" {
		t.Errorf("expected links sorted, got %v", findings[0].Remediation.Links)
	}
}

func TestSortFindings_PreservesStepsOrder(t *testing.T) {
	findings := []Finding{
		{
			Severity: SeverityWarning,
			Message:  "test",
			Remediation: &Remediation{
				Summary: "Fix it",
				Steps:   []string{"step 2", "step 1", "step 3"},
			},
		},
	}

	sortFindings(findings)

	// Steps should NOT be re-sorted (pattern-defined order)
	if findings[0].Remediation.Steps[0] != "step 2" {
		t.Errorf("expected steps order preserved, got %v", findings[0].Remediation.Steps)
	}
}

func TestRenderText_WithRemediation(t *testing.T) {
	confidence := 0.9
	result := Result{
		SchemaVersion: SchemaVersion,
		Patterns: []PatternResult{
			{
				ID:     "test.pattern",
				Name:   "Test Pattern",
				Status: StatusFail,
				Findings: []Finding{
					{
						Pattern:    "test.pattern",
						Severity:   SeverityWarning,
						Message:    "Something is wrong",
						Confidence: &confidence,
						Refs:       []string{"k8s:Pod/default/test"},
						Remediation: &Remediation{
							Summary: "Fix the thing",
							Steps:   []string{"Do step 1", "Do step 2"},
							Links:   []string{"https://example.com/docs"},
						},
					},
				},
			},
		},
	}

	output := RenderText(result)

	// Should contain remediation block
	if !strings.Contains(output, "Remediation: Fix the thing") {
		t.Error("expected remediation summary in output")
	}
	if !strings.Contains(output, "- Do step 1") {
		t.Error("expected remediation steps in output")
	}
	if !strings.Contains(output, "Links:") {
		t.Error("expected Links section in output")
	}
	if !strings.Contains(output, "- https://example.com/docs") {
		t.Error("expected remediation link in output")
	}
}

func TestRenderText_WithoutRemediation(t *testing.T) {
	result := Result{
		SchemaVersion: SchemaVersion,
		Patterns: []PatternResult{
			{
				ID:     "test.pattern",
				Name:   "Test Pattern",
				Status: StatusFail,
				Findings: []Finding{
					{
						Pattern:  "test.pattern",
						Severity: SeverityWarning,
						Message:  "Something is wrong",
					},
				},
			},
		},
	}

	output := RenderText(result)

	// Should NOT contain remediation block
	if strings.Contains(output, "Remediation:") {
		t.Error("should not print remediation when absent")
	}
	if strings.Contains(output, "Links:") {
		t.Error("should not print Links when remediation absent")
	}
}

func TestRenderJSON_WithEnrichmentFields(t *testing.T) {
	confidence := 0.85
	result := Result{
		SchemaVersion: SchemaVersion,
		Patterns: []PatternResult{
			{
				ID:     "test.pattern",
				Name:   "Test Pattern",
				Status: StatusFail,
				Findings: []Finding{
					{
						Pattern:    "test.pattern",
						Severity:   SeverityWarning,
						Message:    "Test message",
						Confidence: &confidence,
						Refs:       []string{"k8s:Pod/default/test"},
						Remediation: &Remediation{
							Summary: "Fix it",
							Steps:   []string{"Step 1"},
							Links:   []string{"https://example.com"},
						},
					},
				},
			},
		},
	}

	output, err := RenderJSON(result)
	if err != nil {
		t.Fatalf("RenderJSON error: %v", err)
	}

	// Should contain new fields
	if !strings.Contains(output, `"confidence": 0.85`) {
		t.Error("expected confidence in JSON output")
	}
	if !strings.Contains(output, `"refs"`) {
		t.Error("expected refs in JSON output")
	}
	if !strings.Contains(output, `"remediation"`) {
		t.Error("expected remediation in JSON output")
	}
	if !strings.Contains(output, `"summary": "Fix it"`) {
		t.Error("expected remediation summary in JSON output")
	}
}

func TestRenderJSON_OmitsAbsentFields(t *testing.T) {
	result := Result{
		SchemaVersion: SchemaVersion,
		Patterns: []PatternResult{
			{
				ID:     "test.pattern",
				Name:   "Test Pattern",
				Status: StatusPass,
				Findings: []Finding{
					{
						Pattern:  "test.pattern",
						Severity: SeverityInfo,
						Message:  "All good",
					},
				},
			},
		},
	}

	output, err := RenderJSON(result)
	if err != nil {
		t.Fatalf("RenderJSON error: %v", err)
	}

	// Should NOT contain new optional fields when absent
	if strings.Contains(output, `"confidence"`) {
		t.Error("should not include confidence when nil")
	}
	if strings.Contains(output, `"refs"`) {
		t.Error("should not include refs when nil/empty")
	}
	if strings.Contains(output, `"remediation"`) {
		t.Error("should not include remediation when nil")
	}
}

// Tests for Track J prerequisites (v0.9+)

func TestCheckPrerequisites_NoPrereqs(t *testing.T) {
	g := graph.NewGraph("test")
	p := Pattern{
		ID:            "test.pattern",
		Prerequisites: nil, // No prerequisites
	}

	reason := checkPrerequisites(p, g)
	if reason != "" {
		t.Errorf("expected empty skip reason for pattern without prereqs, got %s", reason)
	}
}

func TestCheckPrerequisites_RequiresNodeKind_Met(t *testing.T) {
	g := graph.NewGraph("test")
	g.AddNode(graph.Node{Kind: "Deployment", ID: "test/default/Deployment/foo"})

	p := Pattern{
		ID: "test.pattern",
		Prerequisites: []Prerequisite{
			{Type: "requires_node_kind", Kinds: []string{"Deployment"}},
		},
	}

	reason := checkPrerequisites(p, g)
	if reason != "" {
		t.Errorf("expected empty skip reason when prereq met, got %s", reason)
	}
}

func TestCheckPrerequisites_RequiresNodeKind_Unmet(t *testing.T) {
	g := graph.NewGraph("test")
	// Empty graph - no Deployment

	p := Pattern{
		ID: "test.pattern",
		Prerequisites: []Prerequisite{
			{Type: "requires_node_kind", Kinds: []string{"Deployment"}},
		},
	}

	reason := checkPrerequisites(p, g)
	if reason == "" {
		t.Error("expected skip reason when prereq not met")
	}
	if !strings.Contains(reason, "Deployment") {
		t.Errorf("expected reason to mention Deployment, got %s", reason)
	}
}

func TestCheckPrerequisites_RequiresAnyOfKinds_Met(t *testing.T) {
	g := graph.NewGraph("test")
	g.AddNode(graph.Node{Kind: "Pod", ID: "test/default/Pod/foo"})

	p := Pattern{
		ID: "test.pattern",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"Deployment", "ReplicaSet", "Pod"}},
		},
	}

	reason := checkPrerequisites(p, g)
	if reason != "" {
		t.Errorf("expected empty skip reason when any-of prereq met, got %s", reason)
	}
}

func TestCheckPrerequisites_RequiresAnyOfKinds_Unmet(t *testing.T) {
	g := graph.NewGraph("test")
	g.AddNode(graph.Node{Kind: "ConfigMap", ID: "test/default/ConfigMap/foo"})

	p := Pattern{
		ID: "test.pattern",
		Prerequisites: []Prerequisite{
			{Type: "requires_any_of_kinds", Kinds: []string{"Deployment", "ReplicaSet", "Pod"}},
		},
	}

	reason := checkPrerequisites(p, g)
	if reason == "" {
		t.Error("expected skip reason when any-of prereq not met")
	}
	if !strings.Contains(reason, "Deployment") || !strings.Contains(reason, "ReplicaSet") || !strings.Contains(reason, "Pod") {
		t.Errorf("expected reason to mention all required kinds, got %s", reason)
	}
}

func TestCheckPrerequisites_MultiplePrereqs_FirstUnmet(t *testing.T) {
	g := graph.NewGraph("test")
	g.AddNode(graph.Node{Kind: "Pod", ID: "test/default/Pod/foo"})

	p := Pattern{
		ID: "test.pattern",
		Prerequisites: []Prerequisite{
			{Type: "requires_node_kind", Kinds: []string{"Deployment"}}, // Will fail
			{Type: "requires_node_kind", Kinds: []string{"Pod"}},        // Would pass
		},
	}

	reason := checkPrerequisites(p, g)
	if reason == "" {
		t.Error("expected skip reason when first prereq not met")
	}
	if !strings.Contains(reason, "Deployment") {
		t.Errorf("expected reason to mention Deployment (first unmet), got %s", reason)
	}
}

func TestDetectAll_WithPrereqs_Skipped(t *testing.T) {
	// Temporarily register a test pattern with prerequisites
	testPattern := Pattern{
		ID:          "test.prereq_pattern",
		Name:        "Test Prereq Pattern",
		Description: "Test pattern with prerequisites",
		Category:    "test",
		Prerequisites: []Prerequisite{
			{Type: "requires_node_kind", Kinds: []string{"NonexistentKind"}},
		},
		Detect: func(g *graph.Graph) ([]Finding, Status) {
			// This should never be called
			t.Error("Detect should not be called when prereqs unmet")
			return nil, StatusPass
		},
	}

	// Register and defer cleanup
	Register(testPattern)
	defer func() {
		// Clean up by removing from registry
		delete(registry, testPattern.ID)
	}()

	g := graph.NewGraph("test")
	result := DetectAll(g)

	// Find our test pattern result
	var testResult *PatternResult
	for i := range result.Patterns {
		if result.Patterns[i].ID == "test.prereq_pattern" {
			testResult = &result.Patterns[i]
			break
		}
	}

	if testResult == nil {
		t.Fatal("expected test pattern in results")
	}

	if testResult.Status != StatusSkip {
		t.Errorf("expected skip status, got %s", testResult.Status)
	}

	if testResult.SkipReason == "" {
		t.Error("expected skip_reason to be set")
	}

	if len(testResult.Findings) != 0 {
		t.Errorf("expected empty findings when skipped, got %d", len(testResult.Findings))
	}
}

func TestRenderText_WithSkipReason(t *testing.T) {
	result := Result{
		SchemaVersion: SchemaVersion,
		Patterns: []PatternResult{
			{
				ID:         "test.pattern",
				Name:       "Test Pattern",
				Status:     StatusSkip,
				SkipReason: "no Deployment nodes in graph",
				Findings:   []Finding{},
			},
		},
	}

	output := RenderText(result)

	if !strings.Contains(output, "[SKIP]") {
		t.Error("expected [SKIP] in output")
	}

	if !strings.Contains(output, "skip_reason: no Deployment nodes in graph") {
		t.Error("expected skip_reason in output")
	}
}

func TestRenderJSON_WithSkipReason(t *testing.T) {
	result := Result{
		SchemaVersion: SchemaVersion,
		Patterns: []PatternResult{
			{
				ID:         "test.pattern",
				Name:       "Test Pattern",
				Status:     StatusSkip,
				SkipReason: "no Deployment nodes in graph",
				Findings:   []Finding{},
			},
		},
	}

	output, err := RenderJSON(result)
	if err != nil {
		t.Fatalf("RenderJSON error: %v", err)
	}

	if !strings.Contains(output, `"skip_reason": "no Deployment nodes in graph"`) {
		t.Error("expected skip_reason in JSON output")
	}
}

func TestRenderJSON_OmitsEmptySkipReason(t *testing.T) {
	result := Result{
		SchemaVersion: SchemaVersion,
		Patterns: []PatternResult{
			{
				ID:       "test.pattern",
				Name:     "Test Pattern",
				Status:   StatusPass,
				Findings: []Finding{},
			},
		},
	}

	output, err := RenderJSON(result)
	if err != nil {
		t.Fatalf("RenderJSON error: %v", err)
	}

	if strings.Contains(output, "skip_reason") {
		t.Error("expected skip_reason to be omitted when empty")
	}
}

// Tests for v0.9.2 GitOps interpretive patterns

func TestArgoCDResourcesPresent_WithResources(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Add multiple Argo CD Applications
	g.AddNode(graph.Node{
		ID:         "test-cluster/argocd/Application/app1",
		Cluster:    "test-cluster",
		Namespace:  "argocd",
		Kind:       "Application",
		Name:       "app1",
		APIVersion: "argoproj.io/v1alpha1",
	})
	g.AddNode(graph.Node{
		ID:         "test-cluster/argocd/Application/app2",
		Cluster:    "test-cluster",
		Namespace:  "argocd",
		Kind:       "Application",
		Name:       "app2",
		APIVersion: "argoproj.io/v1alpha1",
	})

	// Add one ApplicationSet
	g.AddNode(graph.Node{
		ID:         "test-cluster/argocd/ApplicationSet/appset1",
		Cluster:    "test-cluster",
		Namespace:  "argocd",
		Kind:       "ApplicationSet",
		Name:       "appset1",
		APIVersion: "argoproj.io/v1alpha1",
	})

	pr := DetectOne(g, "gitops.argocd.resources_present")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusPass {
		t.Errorf("expected pass status, got %s", pr.Status)
	}

	// Should have 2 findings (one per kind)
	if len(pr.Findings) != 2 {
		t.Errorf("expected 2 findings (one per kind), got %d", len(pr.Findings))
	}

	// Check for correct counts in messages
	hasApps := false
	hasAppSets := false
	for _, f := range pr.Findings {
		if strings.Contains(f.Message, "Applications detected: 2") {
			hasApps = true
		}
		if strings.Contains(f.Message, "ApplicationSets detected: 1") {
			hasAppSets = true
		}
	}

	if !hasApps {
		t.Error("expected finding with Applications count")
	}
	if !hasAppSets {
		t.Error("expected finding with ApplicationSets count")
	}
}

func TestArgoCDResourcesPresent_Skipped(t *testing.T) {
	g := graph.NewGraph("test-cluster")
	// Empty graph - no Argo CD resources

	pr := DetectOne(g, "gitops.argocd.resources_present")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusSkip {
		t.Errorf("expected skip status when no Argo CD resources, got %s", pr.Status)
	}

	if pr.SkipReason == "" {
		t.Error("expected skip_reason to be set")
	}
}

func TestFluxResourcesPresent_WithResources(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Add Flux Kustomizations
	g.AddNode(graph.Node{
		ID:         "test-cluster/flux-system/Kustomization/flux-system",
		Cluster:    "test-cluster",
		Namespace:  "flux-system",
		Kind:       "Kustomization",
		Name:       "flux-system",
		APIVersion: "kustomize.toolkit.fluxcd.io/v1",
	})
	g.AddNode(graph.Node{
		ID:         "test-cluster/flux-system/Kustomization/apps",
		Cluster:    "test-cluster",
		Namespace:  "flux-system",
		Kind:       "Kustomization",
		Name:       "apps",
		APIVersion: "kustomize.toolkit.fluxcd.io/v1",
	})

	// Add HelmRelease
	g.AddNode(graph.Node{
		ID:         "test-cluster/default/HelmRelease/nginx",
		Cluster:    "test-cluster",
		Namespace:  "default",
		Kind:       "HelmRelease",
		Name:       "nginx",
		APIVersion: "helm.toolkit.fluxcd.io/v2",
	})

	// Add GitRepository
	g.AddNode(graph.Node{
		ID:         "test-cluster/flux-system/GitRepository/flux-system",
		Cluster:    "test-cluster",
		Namespace:  "flux-system",
		Kind:       "GitRepository",
		Name:       "flux-system",
		APIVersion: "source.toolkit.fluxcd.io/v1",
	})

	pr := DetectOne(g, "gitops.flux.resources_present")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusPass {
		t.Errorf("expected pass status, got %s", pr.Status)
	}

	// Should have 3 findings (one per kind)
	if len(pr.Findings) != 3 {
		t.Errorf("expected 3 findings (one per kind), got %d", len(pr.Findings))
	}

	// Check for correct counts
	hasKustomizations := false
	hasHelmReleases := false
	hasGitRepos := false
	for _, f := range pr.Findings {
		if strings.Contains(f.Message, "Kustomizations detected: 2") {
			hasKustomizations = true
		}
		if strings.Contains(f.Message, "HelmReleases detected: 1") {
			hasHelmReleases = true
		}
		if strings.Contains(f.Message, "GitRepositories detected: 1") {
			hasGitRepos = true
		}
	}

	if !hasKustomizations {
		t.Error("expected finding with Kustomizations count")
	}
	if !hasHelmReleases {
		t.Error("expected finding with HelmReleases count")
	}
	if !hasGitRepos {
		t.Error("expected finding with GitRepositories count")
	}
}

func TestFluxResourcesPresent_Skipped(t *testing.T) {
	g := graph.NewGraph("test-cluster")
	// Empty graph - no Flux resources

	pr := DetectOne(g, "gitops.flux.resources_present")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusSkip {
		t.Errorf("expected skip status when no Flux resources, got %s", pr.Status)
	}

	if pr.SkipReason == "" {
		t.Error("expected skip_reason to be set")
	}
}

// Tests for v0.10 Hybrid git-aware patterns

func TestApplicationSetGenerators_ReducedEvidence(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Add ApplicationSets
	g.AddNode(graph.Node{
		ID:         "test-cluster/argocd/ApplicationSet/appset1",
		Cluster:    "test-cluster",
		Namespace:  "argocd",
		Kind:       "ApplicationSet",
		Name:       "appset1",
		APIVersion: "argoproj.io/v1alpha1",
	})
	g.AddNode(graph.Node{
		ID:         "test-cluster/argocd/ApplicationSet/appset2",
		Cluster:    "test-cluster",
		Namespace:  "argocd",
		Kind:       "ApplicationSet",
		Name:       "appset2",
		APIVersion: "argoproj.io/v1alpha1",
	})

	// Without git context - should run in reduced evidence mode
	pr := DetectOneWithGit(g, nil, "gitops.argocd.applicationset_generators")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusPass {
		t.Errorf("expected pass status for hybrid pattern without git context, got %s (skip_reason: %s)", pr.Status, pr.SkipReason)
	}

	// Should have finding about count with hint about --git-root
	found := false
	for _, f := range pr.Findings {
		if strings.Contains(f.Message, "ApplicationSets in graph: 2") && strings.Contains(f.Message, "git-root") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finding with ApplicationSet count and git-root hint")
	}
}

func TestApplicationSetGenerators_Skipped_NoPrereqs(t *testing.T) {
	g := graph.NewGraph("test-cluster")
	// Empty graph - no ApplicationSets

	pr := DetectOneWithGit(g, nil, "gitops.argocd.applicationset_generators")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusSkip {
		t.Errorf("expected skip status when no ApplicationSets, got %s", pr.Status)
	}

	if !strings.Contains(pr.SkipReason, "ApplicationSet") {
		t.Errorf("expected skip reason to mention ApplicationSet, got %s", pr.SkipReason)
	}
}

func TestFluxKustomizationPaths_ReducedEvidence(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Add Kustomizations
	g.AddNode(graph.Node{
		ID:         "test-cluster/flux-system/Kustomization/flux-system",
		Cluster:    "test-cluster",
		Namespace:  "flux-system",
		Kind:       "Kustomization",
		Name:       "flux-system",
		APIVersion: "kustomize.toolkit.fluxcd.io/v1",
	})
	g.AddNode(graph.Node{
		ID:         "test-cluster/flux-system/Kustomization/apps",
		Cluster:    "test-cluster",
		Namespace:  "flux-system",
		Kind:       "Kustomization",
		Name:       "apps",
		APIVersion: "kustomize.toolkit.fluxcd.io/v1",
	})

	// Without git context - should run in reduced evidence mode
	pr := DetectOneWithGit(g, nil, "gitops.flux_kustomization_paths")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusPass {
		t.Errorf("expected pass status for hybrid pattern without git context, got %s (skip_reason: %s)", pr.Status, pr.SkipReason)
	}

	// Should have finding about count with hint about --git-root
	found := false
	for _, f := range pr.Findings {
		if strings.Contains(f.Message, "Flux Kustomizations in graph: 2") && strings.Contains(f.Message, "git-root") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected finding with Kustomization count and git-root hint")
	}
}

func TestFluxKustomizationPaths_UnderscoreID(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	g.AddNode(graph.Node{
		ID:         "test-cluster/flux-system/Kustomization/flux-system",
		Cluster:    "test-cluster",
		Namespace:  "flux-system",
		Kind:       "Kustomization",
		Name:       "flux-system",
		APIVersion: "kustomize.toolkit.fluxcd.io/v1",
	})

	pr := DetectOneWithGit(g, nil, "gitops.flux_kustomization_paths")
	if pr == nil {
		t.Fatal("expected result for underscore pattern ID")
	}
	if pr.ID != "gitops.flux_kustomization_paths" {
		t.Fatalf("expected canonical underscore ID, got %q", pr.ID)
	}
}

func TestFluxKustomizationPaths_LegacyIDAlias(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	g.AddNode(graph.Node{
		ID:         "test-cluster/flux-system/Kustomization/flux-system",
		Cluster:    "test-cluster",
		Namespace:  "flux-system",
		Kind:       "Kustomization",
		Name:       "flux-system",
		APIVersion: "kustomize.toolkit.fluxcd.io/v1",
	})

	pr := DetectOneWithGit(g, nil, "gitops.flux.kustomization_paths")
	if pr == nil {
		t.Fatal("expected result for legacy dotted pattern ID alias")
	}
	if pr.ID != "gitops.flux_kustomization_paths" {
		t.Fatalf("expected canonical underscore ID from alias lookup, got %q", pr.ID)
	}
}

func TestFluxKustomizationPaths_Skipped_NoPrereqs(t *testing.T) {
	g := graph.NewGraph("test-cluster")
	// Empty graph - no Kustomizations

	pr := DetectOneWithGit(g, nil, "gitops.flux_kustomization_paths")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusSkip {
		t.Errorf("expected skip status when no Kustomizations, got %s", pr.Status)
	}

	if !strings.Contains(pr.SkipReason, "Kustomization") {
		t.Errorf("expected skip reason to mention Kustomization, got %s", pr.SkipReason)
	}
}

// getRepoRoot returns the absolute path to the repository root.
func getRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	// internal/patterns/patterns_test.go -> repo root
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

func TestApplicationSetGenerators_EnrichedWithGitContext(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Add ApplicationSets
	g.AddNode(graph.Node{
		ID:         "test-cluster/argocd/ApplicationSet/appset1",
		Cluster:    "test-cluster",
		Namespace:  "argocd",
		Kind:       "ApplicationSet",
		Name:       "appset1",
		APIVersion: "argoproj.io/v1alpha1",
	})

	// Open git context for testdata/repo-argocd
	repoRoot := getRepoRoot(t)
	gitRoot := filepath.Join(repoRoot, "testdata", "repo-argocd")
	gitCtx := gitctx.OpenGitRoot(gitRoot)
	if gitCtx == nil || !gitCtx.Valid {
		t.Skipf("git context not valid: %v", gitCtx)
	}

	pr := DetectOneWithGit(g, gitCtx, "gitops.argocd.applicationset_generators")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusPass {
		t.Errorf("expected pass status with git context, got %s (skip_reason: %s)", pr.Status, pr.SkipReason)
	}

	// Should have enriched findings with generator types
	hasGenerators := false
	hasRefs := false
	for _, f := range pr.Findings {
		if strings.Contains(f.Message, "generators:") {
			hasGenerators = true
		}
		for _, ref := range f.Refs {
			if strings.HasPrefix(ref, "git:path:") {
				hasRefs = true
				// Verify refs are repo-relative (no absolute paths)
				if strings.HasPrefix(ref, "git:path:/") {
					t.Errorf("ref should be repo-relative, not absolute: %s", ref)
				}
			}
		}
	}

	if !hasGenerators {
		t.Error("expected finding with generator summary in enriched mode")
	}
	if !hasRefs {
		t.Error("expected refs with git:path: prefix in enriched mode")
	}
}

func TestFluxKustomizationPaths_EnrichedWithGitContext(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Add Kustomizations
	g.AddNode(graph.Node{
		ID:         "test-cluster/flux-system/Kustomization/flux-system",
		Cluster:    "test-cluster",
		Namespace:  "flux-system",
		Kind:       "Kustomization",
		Name:       "flux-system",
		APIVersion: "kustomize.toolkit.fluxcd.io/v1",
	})

	// Open git context for testdata/repo-flux
	repoRoot := getRepoRoot(t)
	gitRoot := filepath.Join(repoRoot, "testdata", "repo-flux")
	gitCtx := gitctx.OpenGitRoot(gitRoot)
	if gitCtx == nil || !gitCtx.Valid {
		t.Skipf("git context not valid: %v", gitCtx)
	}

	pr := DetectOneWithGit(g, gitCtx, "gitops.flux_kustomization_paths")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusPass {
		t.Errorf("expected pass status with git context, got %s (skip_reason: %s)", pr.Status, pr.SkipReason)
	}

	// Should have enriched findings with paths
	hasPaths := false
	hasRefs := false
	for _, f := range pr.Findings {
		if strings.Contains(f.Message, "paths:") {
			hasPaths = true
		}
		for _, ref := range f.Refs {
			if strings.HasPrefix(ref, "git:path:") {
				hasRefs = true
				// Verify refs are repo-relative (no absolute paths)
				if strings.HasPrefix(ref, "git:path:/") {
					t.Errorf("ref should be repo-relative, not absolute: %s", ref)
				}
			}
		}
	}

	if !hasPaths {
		t.Error("expected finding with path summary in enriched mode")
	}
	if !hasRefs {
		t.Error("expected refs with git:path: prefix in enriched mode")
	}
}

func TestHybridPattern_SkipsWhenGitContextInvalid(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Add ApplicationSets
	g.AddNode(graph.Node{
		ID:         "test-cluster/argocd/ApplicationSet/appset1",
		Cluster:    "test-cluster",
		Namespace:  "argocd",
		Kind:       "ApplicationSet",
		Name:       "appset1",
		APIVersion: "argoproj.io/v1alpha1",
	})

	// Create an invalid git context (provided but unusable)
	gitCtx := &gitctx.GitContext{
		Valid:      false,
		SkipReason: "git_root path does not exist",
	}

	pr := DetectOneWithGit(g, gitCtx, "gitops.argocd.applicationset_generators")
	if pr == nil {
		t.Fatal("expected result")
	}

	if pr.Status != StatusSkip {
		t.Errorf("expected skip status when git context is invalid, got %s", pr.Status)
	}

	if pr.SkipReason != "git_root path does not exist" {
		t.Errorf("expected specific skip reason, got %s", pr.SkipReason)
	}
}
