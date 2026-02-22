// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package patterns

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/internal/graph"
)

// --- Git → Flux bridge tests ---

func TestBridgeGitFlux_Detected(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// GitRepository source
	g.AddNode(graph.Node{
		ID: "test-cluster/flux-system/GitRepository/app-repo", Kind: "GitRepository",
		Namespace: "flux-system", Name: "app-repo",
	})

	// Kustomization deployer
	g.AddNode(graph.Node{
		ID: "test-cluster/flux-system/Kustomization/web-apps", Kind: "Kustomization",
		Namespace: "flux-system", Name: "web-apps",
	})

	// Workloads with Flux labels
	g.AddNode(graph.Node{
		ID: "test-cluster/web/Deployment/frontend", Kind: "Deployment",
		Namespace: "web", Name: "frontend",
		Labels: map[string]string{"kustomize.toolkit.fluxcd.io/name": "web-apps"},
	})
	g.AddNode(graph.Node{
		ID: "test-cluster/web/Service/frontend", Kind: "Service",
		Namespace: "web", Name: "frontend",
		Labels: map[string]string{"kustomize.toolkit.fluxcd.io/name": "web-apps"},
	})

	pr := DetectOne(g, "delivery.bridge.git_flux")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusPass {
		t.Errorf("expected pass, got %s", pr.Status)
	}
	if len(pr.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(pr.Findings))
	}

	f := pr.Findings[0]
	if !strings.Contains(f.Message, "1 sources") {
		t.Errorf("expected 1 source in message, got: %s", f.Message)
	}
	if !strings.Contains(f.Message, "1 deployers") {
		t.Errorf("expected 1 deployer in message, got: %s", f.Message)
	}
	if !strings.Contains(f.Message, "2 managed workloads") {
		t.Errorf("expected 2 managed workloads in message, got: %s", f.Message)
	}
}

func TestBridgeGitFlux_Skipped_NoGitRepo(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Only a Kustomization, no GitRepository
	g.AddNode(graph.Node{
		ID: "test-cluster/flux-system/Kustomization/web-apps", Kind: "Kustomization",
		Namespace: "flux-system", Name: "web-apps",
	})

	pr := DetectOne(g, "delivery.bridge.git_flux")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusSkip {
		t.Errorf("expected skip (no GitRepository prereq), got %s", pr.Status)
	}
}

func TestBridgeGitFlux_Skipped_NoDeployer(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Only a GitRepository, no Kustomization/HelmRelease
	g.AddNode(graph.Node{
		ID: "test-cluster/flux-system/GitRepository/app-repo", Kind: "GitRepository",
		Namespace: "flux-system", Name: "app-repo",
	})

	pr := DetectOne(g, "delivery.bridge.git_flux")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusSkip {
		t.Errorf("expected skip (no deployer prereq), got %s", pr.Status)
	}
}

// --- Git → ArgoCD bridge tests ---

func TestBridgeGitArgoCD_Detected(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Application
	g.AddNode(graph.Node{
		ID: "test-cluster/argocd/Application/frontend", Kind: "Application",
		Namespace: "argocd", Name: "frontend",
	})
	g.AddNode(graph.Node{
		ID: "test-cluster/argocd/Application/backend", Kind: "Application",
		Namespace: "argocd", Name: "backend",
	})

	// Workloads with ArgoCD labels
	g.AddNode(graph.Node{
		ID: "test-cluster/web/Deployment/frontend", Kind: "Deployment",
		Namespace: "web", Name: "frontend",
		Labels: map[string]string{"argocd.argoproj.io/instance": "frontend"},
	})
	g.AddNode(graph.Node{
		ID: "test-cluster/api/Deployment/backend", Kind: "Deployment",
		Namespace: "api", Name: "backend",
		Labels: map[string]string{"argocd.argoproj.io/instance": "backend"},
	})

	pr := DetectOne(g, "delivery.bridge.git_argocd")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusPass {
		t.Errorf("expected pass, got %s", pr.Status)
	}
	if len(pr.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(pr.Findings))
	}

	f := pr.Findings[0]
	if !strings.Contains(f.Message, "2 Applications") {
		t.Errorf("expected 2 Applications in message, got: %s", f.Message)
	}
	if !strings.Contains(f.Message, "2 managed workloads") {
		t.Errorf("expected 2 managed workloads in message, got: %s", f.Message)
	}
}

func TestBridgeGitArgoCD_NoWorkloads(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Application only, no workloads
	g.AddNode(graph.Node{
		ID: "test-cluster/argocd/Application/frontend", Kind: "Application",
		Namespace: "argocd", Name: "frontend",
	})

	pr := DetectOne(g, "delivery.bridge.git_argocd")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusPass {
		t.Errorf("expected pass (bridge still detected), got %s", pr.Status)
	}
	if len(pr.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(pr.Findings))
	}
	if !strings.Contains(pr.Findings[0].Message, "0 managed workloads") {
		t.Errorf("expected 0 managed workloads, got: %s", pr.Findings[0].Message)
	}
}

func TestBridgeGitArgoCD_Skipped(t *testing.T) {
	g := graph.NewGraph("test-cluster")
	// No Application nodes

	pr := DetectOne(g, "delivery.bridge.git_argocd")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusSkip {
		t.Errorf("expected skip, got %s", pr.Status)
	}
}

// --- ConfigHub → OCI bridge tests ---

func TestBridgeConfigHubOCI_Detected(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// OCIRepository with ConfigHub labels
	g.AddNode(graph.Node{
		ID: "test-cluster/flux-system/OCIRepository/confighub-payments", Kind: "OCIRepository",
		Namespace: "flux-system", Name: "confighub-payments",
		Labels: map[string]string{"confighub.com/SpaceName": "payments-prod"},
	})

	// Kustomization deployer
	g.AddNode(graph.Node{
		ID: "test-cluster/flux-system/Kustomization/payments", Kind: "Kustomization",
		Namespace: "flux-system", Name: "payments",
	})

	// Workload with both Flux + ConfigHub labels
	g.AddNode(graph.Node{
		ID: "test-cluster/payments/Deployment/api", Kind: "Deployment",
		Namespace: "payments", Name: "api",
		Labels: map[string]string{
			"kustomize.toolkit.fluxcd.io/name": "payments",
			"confighub.com/UnitSlug":           "payments-api",
		},
	})

	pr := DetectOne(g, "delivery.bridge.confighub_oci")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusPass {
		t.Errorf("expected pass, got %s", pr.Status)
	}
	if len(pr.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(pr.Findings))
	}

	f := pr.Findings[0]
	if !strings.Contains(f.Message, "1 OCI sources with ConfigHub origin") {
		t.Errorf("expected 1 OCI source, got: %s", f.Message)
	}
	if !strings.Contains(f.Message, "1 dual-managed workloads") {
		t.Errorf("expected 1 dual-managed workload, got: %s", f.Message)
	}
}

func TestBridgeConfigHubOCI_NotConfigHub(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// OCIRepository without ConfigHub labels
	g.AddNode(graph.Node{
		ID: "test-cluster/flux-system/OCIRepository/plain-oci", Kind: "OCIRepository",
		Namespace: "flux-system", Name: "plain-oci",
		Labels: map[string]string{},
	})

	pr := DetectOne(g, "delivery.bridge.confighub_oci")
	if pr == nil {
		t.Fatal("expected result")
	}
	// Prerequisites met (has OCIRepository) but detection finds no ConfigHub origin
	if pr.Status != StatusPass {
		t.Errorf("expected pass (no findings), got %s", pr.Status)
	}
	if len(pr.Findings) != 0 {
		t.Errorf("expected 0 findings for non-ConfigHub OCI, got %d", len(pr.Findings))
	}
}

func TestBridgeConfigHubOCI_Skipped(t *testing.T) {
	g := graph.NewGraph("test-cluster")
	// No OCIRepository nodes

	pr := DetectOne(g, "delivery.bridge.confighub_oci")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusSkip {
		t.Errorf("expected skip, got %s", pr.Status)
	}
}

// --- Live import bridge tests ---

func TestBridgeLiveImport_Detected(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Workloads with ConfigHub labels only (no GitOps deployer labels)
	g.AddNode(graph.Node{
		ID: "test-cluster/legacy/Deployment/old-api", Kind: "Deployment",
		Namespace: "legacy", Name: "old-api",
		Labels: map[string]string{"confighub.com/UnitSlug": "old-api"},
	})
	g.AddNode(graph.Node{
		ID: "test-cluster/legacy/Service/old-api", Kind: "Service",
		Namespace: "legacy", Name: "old-api",
		Labels: map[string]string{"confighub.com/UnitSlug": "old-api"},
	})

	pr := DetectOne(g, "delivery.bridge.live_import")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusPass {
		t.Errorf("expected pass, got %s", pr.Status)
	}
	if len(pr.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(pr.Findings))
	}

	f := pr.Findings[0]
	if !strings.Contains(f.Message, "2 resources with ConfigHub ownership but no GitOps deployer") {
		t.Errorf("expected 2 live imports in message, got: %s", f.Message)
	}
}

func TestBridgeLiveImport_NotDetected(t *testing.T) {
	g := graph.NewGraph("test-cluster")

	// Workload with ConfigHub + Flux labels (not live import — has GitOps deployer)
	g.AddNode(graph.Node{
		ID: "test-cluster/web/Deployment/frontend", Kind: "Deployment",
		Namespace: "web", Name: "frontend",
		Labels: map[string]string{
			"confighub.com/UnitSlug":           "frontend",
			"kustomize.toolkit.fluxcd.io/name": "web-apps",
		},
	})

	pr := DetectOne(g, "delivery.bridge.live_import")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusPass {
		t.Errorf("expected pass (no findings), got %s", pr.Status)
	}
	if len(pr.Findings) != 0 {
		t.Errorf("expected 0 findings for GitOps-managed ConfigHub resources, got %d", len(pr.Findings))
	}
}

func TestBridgeLiveImport_Skipped(t *testing.T) {
	g := graph.NewGraph("test-cluster")
	// No workload nodes at all

	pr := DetectOne(g, "delivery.bridge.live_import")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != StatusSkip {
		t.Errorf("expected skip, got %s", pr.Status)
	}
}

// --- Verify all bridge patterns are registered ---

func TestBridgePatterns_Registered(t *testing.T) {
	expectedIDs := []string{
		"delivery.bridge.git_flux",
		"delivery.bridge.git_argocd",
		"delivery.bridge.confighub_oci",
		"delivery.bridge.live_import",
	}

	for _, id := range expectedIDs {
		p := Get(id)
		if p == nil {
			t.Errorf("expected pattern %q to be registered", id)
			continue
		}
		if p.Category != "delivery" {
			t.Errorf("expected category=delivery for %q, got %q", id, p.Category)
		}
	}
}
