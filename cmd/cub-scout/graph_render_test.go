package main

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	internalgraph "github.com/confighub/cub-scout/internal/graph"
)

func TestRenderGraphOutput_JSONMatchesExport(t *testing.T) {
	g := sampleRenderGraph()

	got, err := renderGraphOutput(g, "json", 0)
	if err != nil {
		t.Fatalf("renderGraphOutput(json) error = %v", err)
	}

	want, err := g.Export()
	if err != nil {
		t.Fatalf("g.Export() error = %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("json output mismatch\ngot:\n%s\n\nwant:\n%s", string(got), string(want))
	}
}

func TestRenderGraphOutput_DOTContainsOwnershipAndEdges(t *testing.T) {
	g := sampleRenderGraph()

	got, err := renderGraphOutput(g, "dot", 0)
	if err != nil {
		t.Fatalf("renderGraphOutput(dot) error = %v", err)
	}
	text := string(got)

	mustContain(t, text, "digraph")
	mustContain(t, text, "namespace: payments")
	mustContain(t, text, "label=\"owns\"")
	mustContain(t, text, "label=\"selects\"")
	// Helm color.
	mustContain(t, text, "#f59e0b")
	// Argo color.
	mustContain(t, text, "#22c55e")
}

func TestRenderGraphOutput_SVGValidXML(t *testing.T) {
	g := sampleRenderGraph()

	got, err := renderGraphOutput(g, "svg", 0)
	if err != nil {
		t.Fatalf("renderGraphOutput(svg) error = %v", err)
	}

	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(got, &root); err != nil {
		t.Fatalf("svg is not valid XML: %v\n%s", err, string(got))
	}
	if root.XMLName.Local != "svg" {
		t.Fatalf("root element = %q, want svg", root.XMLName.Local)
	}

	text := string(got)
	mustContain(t, text, "Legend")
	mustContain(t, text, "owns")
}

func TestRenderGraphOutput_HTMLSelfContainedAndInteractive(t *testing.T) {
	g := sampleRenderGraph()

	got, err := renderGraphOutput(g, "html", 2)
	if err != nil {
		t.Fatalf("renderGraphOutput(html) error = %v", err)
	}
	text := string(got)

	mustContain(t, text, "<!doctype html>")
	mustContain(t, text, "<svg")
	mustContain(t, text, "addEventListener")
	mustContain(t, text, "node-details")
	mustContain(t, text, "Showing 2 of 3 nodes")

	if strings.Contains(strings.ToLower(text), "<script src=") {
		t.Fatalf("html output should not reference external scripts:\n%s", text)
	}
	if strings.Contains(strings.ToLower(text), "<link rel=\"stylesheet\"") {
		t.Fatalf("html output should not reference external stylesheets:\n%s", text)
	}
}

func TestRenderGraphOutput_UnknownFormat(t *testing.T) {
	g := sampleRenderGraph()

	if _, err := renderGraphOutput(g, "bmp", 0); err == nil {
		t.Fatal("expected error for unknown format, got nil")
	}
}

func sampleRenderGraph() *internalgraph.Graph {
	g := internalgraph.NewGraph("kind-dev")
	g.GeneratedAt = time.Date(2026, 3, 6, 10, 30, 0, 0, time.UTC)

	deploymentID := internalgraph.NodeID("kind-dev", "payments", "Deployment", "api")
	podID := internalgraph.NodeID("kind-dev", "payments", "Pod", "api-7f8f")
	appID := internalgraph.NodeID("kind-dev", "argocd", "Application", "payments")

	g.AddNode(internalgraph.Node{
		ID:         deploymentID,
		Cluster:    "kind-dev",
		Namespace:  "payments",
		Kind:       "Deployment",
		Name:       "api",
		APIVersion: "apps/v1",
		Labels: map[string]string{
			"app.kubernetes.io/managed-by": "Helm",
		},
	})
	g.AddNode(internalgraph.Node{
		ID:         podID,
		Cluster:    "kind-dev",
		Namespace:  "payments",
		Kind:       "Pod",
		Name:       "api-7f8f",
		APIVersion: "v1",
	})
	g.AddNode(internalgraph.Node{
		ID:         appID,
		Cluster:    "kind-dev",
		Namespace:  "argocd",
		Kind:       "Application",
		Name:       "payments",
		APIVersion: "argoproj.io/v1alpha1",
	})

	g.AddEdge(internalgraph.Edge{
		From: deploymentID,
		To:   podID,
		Type: internalgraph.EdgeTypeOwns,
	})
	g.AddEdge(internalgraph.Edge{
		From: appID,
		To:   deploymentID,
		Type: internalgraph.EdgeTypeSelects,
	})
	return g
}

func mustContain(t *testing.T, text, part string) {
	t.Helper()
	if !strings.Contains(text, part) {
		t.Fatalf("expected output to contain %q\noutput:\n%s", part, text)
	}
}
