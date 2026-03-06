package main

import (
	"bytes"
	"fmt"
	"html"
	"sort"
	"strings"

	internalgraph "github.com/confighub/cub-scout/internal/graph"
)

const (
	ownerArgo      = "Argo"
	ownerFlux      = "Flux"
	ownerHelm      = "Helm"
	ownerConfigHub = "ConfigHub"
	ownerNative    = "Native"
	ownerUnknown   = "Unknown"
)

var ownerColors = map[string]string{
	ownerArgo:      "#22c55e",
	ownerFlux:      "#3b82f6",
	ownerHelm:      "#f59e0b",
	ownerConfigHub: "#06b6d4",
	ownerNative:    "#9ca3af",
	ownerUnknown:   "#6b7280",
}

var ownerLegendOrder = []string{
	ownerArgo,
	ownerFlux,
	ownerHelm,
	ownerConfigHub,
	ownerNative,
	ownerUnknown,
}

type visualNode struct {
	Node      internalgraph.Node
	ID        string
	Namespace string
	Owner     string
	Color     string
	X         float64
	Y         float64
}

type visualEdge struct {
	FromID string
	ToID   string
	Type   string
	X1     float64
	Y1     float64
	X2     float64
	Y2     float64
}

type namespaceGroup struct {
	Name      string
	CenterX   float64
	TopY      float64
	Width     float64
	Height    float64
	NodeCount int
}

type visualLayout struct {
	Cluster     string
	GeneratedAt string
	Nodes       []visualNode
	Edges       []visualEdge
	Groups      []namespaceGroup
	OwnerCounts map[string]int
	Width       float64
	Height      float64
	ShownNodes  int
	TotalNodes  int
	Truncated   bool
}

func renderGraphOutput(g *internalgraph.Graph, format string, maxNodes int) ([]byte, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "json"
	}

	switch format {
	case "json":
		return g.Export()
	case "dot":
		layout := buildVisualLayout(g, maxNodes)
		return renderGraphDOT(layout), nil
	case "svg":
		layout := buildVisualLayout(g, maxNodes)
		return renderGraphSVG(layout, true), nil
	case "html":
		layout := buildVisualLayout(g, maxNodes)
		return renderGraphHTML(layout)
	default:
		return nil, fmt.Errorf("unsupported graph export format %q (supported: json, dot, svg, html)", format)
	}
}

func buildVisualLayout(g *internalgraph.Graph, maxNodes int) visualLayout {
	nodes, edges, truncated := limitGraphForVisual(g, maxNodes)
	totalNodes := len(g.Nodes)

	grouped := make(map[string][]internalgraph.Node)
	for _, n := range nodes {
		ns := n.Namespace
		if ns == "" {
			ns = "(cluster)"
		}
		grouped[ns] = append(grouped[ns], n)
	}

	namespaces := make([]string, 0, len(grouped))
	for ns := range grouped {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)
	if len(namespaces) == 0 {
		namespaces = []string{"(cluster)"}
	}

	groupWidth := 280.0
	groupPadding := 40.0
	groupTop := 120.0
	nodeStartY := 180.0
	nodeStepY := 110.0

	layout := visualLayout{
		Cluster:     g.Cluster,
		GeneratedAt: g.GeneratedAt.UTC().Format("2006-01-02 15:04:05Z"),
		OwnerCounts: make(map[string]int),
		ShownNodes:  len(nodes),
		TotalNodes:  totalNodes,
		Truncated:   truncated,
	}

	nodeByID := make(map[string]visualNode, len(nodes))
	maxNodesInGroup := 0
	for idx, ns := range namespaces {
		nsNodes := grouped[ns]
		sort.Slice(nsNodes, func(i, j int) bool {
			return nsNodes[i].ID < nsNodes[j].ID
		})

		centerX := 180.0 + float64(idx)*(groupWidth+groupPadding)
		groupHeight := 260.0
		if len(nsNodes) > 0 {
			groupHeight = 180.0 + float64(len(nsNodes))*nodeStepY
		}
		if len(nsNodes) > maxNodesInGroup {
			maxNodesInGroup = len(nsNodes)
		}

		layout.Groups = append(layout.Groups, namespaceGroup{
			Name:      ns,
			CenterX:   centerX,
			TopY:      groupTop,
			Width:     groupWidth,
			Height:    groupHeight,
			NodeCount: len(nsNodes),
		})

		for i, n := range nsNodes {
			owner := detectNodeOwner(n)
			color := ownerColors[owner]
			if color == "" {
				color = ownerColors[ownerUnknown]
			}
			vn := visualNode{
				Node:      n,
				ID:        n.ID,
				Namespace: ns,
				Owner:     owner,
				Color:     color,
				X:         centerX,
				Y:         nodeStartY + float64(i)*nodeStepY,
			}
			layout.Nodes = append(layout.Nodes, vn)
			nodeByID[n.ID] = vn
			layout.OwnerCounts[owner]++
		}
	}

	for _, e := range edges {
		fromNode, okFrom := nodeByID[e.From]
		toNode, okTo := nodeByID[e.To]
		if !okFrom || !okTo {
			continue
		}
		layout.Edges = append(layout.Edges, visualEdge{
			FromID: e.From,
			ToID:   e.To,
			Type:   string(e.Type),
			X1:     fromNode.X,
			Y1:     fromNode.Y,
			X2:     toNode.X,
			Y2:     toNode.Y,
		})
	}

	layout.Width = 420.0 + float64(len(layout.Groups))*(groupWidth+groupPadding) + 260.0
	if layout.Width < 1200.0 {
		layout.Width = 1200.0
	}
	layout.Height = 400.0 + float64(maxNodesInGroup)*nodeStepY
	if layout.Height < 760.0 {
		layout.Height = 760.0
	}

	return layout
}

func limitGraphForVisual(g *internalgraph.Graph, maxNodes int) ([]internalgraph.Node, []internalgraph.Edge, bool) {
	nodes := append([]internalgraph.Node(nil), g.Nodes...)
	edges := append([]internalgraph.Edge(nil), g.Edges...)

	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Type < edges[j].Type
	})

	truncated := false
	if maxNodes > 0 && len(nodes) > maxNodes {
		nodes = nodes[:maxNodes]
		truncated = true
	}

	visible := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		visible[n.ID] = true
	}

	filteredEdges := make([]internalgraph.Edge, 0, len(edges))
	for _, e := range edges {
		if visible[e.From] && visible[e.To] {
			filteredEdges = append(filteredEdges, e)
		}
	}

	return nodes, filteredEdges, truncated
}

func detectNodeOwner(n internalgraph.Node) string {
	labels := n.Labels
	kind := n.Kind
	apiVersion := n.APIVersion

	if labels["confighub.com/UnitSlug"] != "" {
		return ownerConfigHub
	}
	if labels["argocd.argoproj.io/instance"] != "" || kind == "Application" || kind == "ApplicationSet" {
		return ownerArgo
	}
	if strings.EqualFold(labels["app.kubernetes.io/managed-by"], "helm") || strings.EqualFold(labels["app.kubernetes.io/managed-by"], "Helm") {
		return ownerHelm
	}
	if strings.HasPrefix(apiVersion, "kustomize.toolkit.fluxcd.io/") ||
		strings.HasPrefix(apiVersion, "helm.toolkit.fluxcd.io/") ||
		strings.HasPrefix(apiVersion, "source.toolkit.fluxcd.io/") ||
		kind == "Kustomization" || kind == "HelmRelease" || kind == "GitRepository" ||
		labels["kustomize.toolkit.fluxcd.io/name"] != "" || labels["helm.toolkit.fluxcd.io/name"] != "" {
		return ownerFlux
	}

	if kind == "" || apiVersion == "" {
		return ownerUnknown
	}
	return ownerNative
}

func renderGraphDOT(layout visualLayout) []byte {
	var b bytes.Buffer
	b.WriteString("digraph cub_scout {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  graph [fontname=\"Helvetica\", bgcolor=\"white\"];\n")
	b.WriteString("  node [shape=box, style=filled, fontname=\"Helvetica\", color=\"#334155\"];\n")
	b.WriteString("  edge [fontname=\"Helvetica\", color=\"#64748b\"];\n")

	legend := ownerCountsSummary(layout.OwnerCounts)
	label := fmt.Sprintf("cluster: %s\\n%s", layout.Cluster, legend)
	if layout.Truncated {
		label += fmt.Sprintf("\\nshowing %d of %d nodes", layout.ShownNodes, layout.TotalNodes)
	}
	b.WriteString(fmt.Sprintf("  labelloc=\"t\";\n  label=\"%s\";\n", escapeDOT(label)))

	nodesByNamespace := make(map[string][]visualNode)
	for _, n := range layout.Nodes {
		nodesByNamespace[n.Namespace] = append(nodesByNamespace[n.Namespace], n)
	}
	namespaces := make([]string, 0, len(nodesByNamespace))
	for ns := range nodesByNamespace {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	for i, ns := range namespaces {
		b.WriteString(fmt.Sprintf("  subgraph \"cluster_%d\" {\n", i))
		b.WriteString(fmt.Sprintf("    label=\"namespace: %s\";\n", escapeDOT(ns)))
		b.WriteString("    color=\"#cbd5e1\";\n")
		nsNodes := nodesByNamespace[ns]
		sort.Slice(nsNodes, func(i, j int) bool {
			return nsNodes[i].ID < nsNodes[j].ID
		})
		for _, n := range nsNodes {
			nodeLabel := fmt.Sprintf("%s\\n%s", n.Node.Kind, n.Node.Name)
			b.WriteString(fmt.Sprintf("    \"%s\" [label=\"%s\", fillcolor=\"%s\"];\n",
				escapeDOT(n.ID), escapeDOT(nodeLabel), n.Color))
		}
		b.WriteString("  }\n")
	}

	for _, e := range layout.Edges {
		b.WriteString(fmt.Sprintf("  \"%s\" -> \"%s\" [label=\"%s\"];\n",
			escapeDOT(e.FromID), escapeDOT(e.ToID), escapeDOT(e.Type)))
	}

	b.WriteString("}\n")
	return b.Bytes()
}

func renderGraphSVG(layout visualLayout, includeXMLHeader bool) []byte {
	var b bytes.Buffer
	if includeXMLHeader {
		b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	}
	b.WriteString(fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%.0f\" height=\"%.0f\" viewBox=\"0 0 %.0f %.0f\" role=\"img\" aria-label=\"cub-scout resource graph\">\n", layout.Width, layout.Height, layout.Width, layout.Height))
	b.WriteString("<defs>\n")
	b.WriteString("  <marker id=\"arrow\" viewBox=\"0 0 10 10\" refX=\"9\" refY=\"5\" markerWidth=\"7\" markerHeight=\"7\" orient=\"auto-start-reverse\">\n")
	b.WriteString("    <path d=\"M 0 0 L 10 5 L 0 10 z\" fill=\"#64748b\"/>\n")
	b.WriteString("  </marker>\n")
	b.WriteString("</defs>\n")
	b.WriteString("<style>\n")
	b.WriteString("  text { font-family: Arial, sans-serif; fill: #0f172a; }\n")
	b.WriteString("  .header { font-size: 20px; font-weight: 700; }\n")
	b.WriteString("  .subhead { font-size: 13px; fill: #334155; }\n")
	b.WriteString("  .namespace-box { fill: #f8fafc; stroke: #cbd5e1; stroke-width: 1.2; rx: 12; }\n")
	b.WriteString("  .namespace-label { font-size: 13px; font-weight: 600; }\n")
	b.WriteString("  .edge { stroke: #64748b; stroke-width: 1.6; marker-end: url(#arrow); }\n")
	b.WriteString("  .edge-label { font-size: 11px; fill: #475569; }\n")
	b.WriteString("  .node circle { stroke: #1f2937; stroke-width: 1.2; }\n")
	b.WriteString("  .node text { font-size: 11px; text-anchor: middle; dominant-baseline: middle; pointer-events: none; }\n")
	b.WriteString("  .legend-title { font-size: 14px; font-weight: 700; }\n")
	b.WriteString("  .legend-item { font-size: 12px; }\n")
	b.WriteString("</style>\n")

	b.WriteString(fmt.Sprintf("<text x=\"40\" y=\"44\" class=\"header\">Resource graph: %s</text>\n", xmlEscape(layout.Cluster)))
	b.WriteString(fmt.Sprintf("<text x=\"40\" y=\"68\" class=\"subhead\">Generated at %s</text>\n", xmlEscape(layout.GeneratedAt)))
	if layout.Truncated {
		b.WriteString(fmt.Sprintf("<text x=\"40\" y=\"92\" class=\"subhead\">Showing %d of %d nodes (use --max-nodes to adjust)</text>\n", layout.ShownNodes, layout.TotalNodes))
	}

	for _, group := range layout.Groups {
		x := group.CenterX - group.Width/2
		b.WriteString(fmt.Sprintf("<rect x=\"%.1f\" y=\"%.1f\" width=\"%.1f\" height=\"%.1f\" class=\"namespace-box\"/>\n", x, group.TopY, group.Width, group.Height))
		b.WriteString(fmt.Sprintf("<text x=\"%.1f\" y=\"%.1f\" class=\"namespace-label\">namespace: %s</text>\n", group.CenterX, group.TopY+22, xmlEscape(group.Name)))
	}

	for _, e := range layout.Edges {
		b.WriteString(fmt.Sprintf("<line x1=\"%.1f\" y1=\"%.1f\" x2=\"%.1f\" y2=\"%.1f\" class=\"edge\"/>\n", e.X1, e.Y1, e.X2, e.Y2))
		midX := (e.X1 + e.X2) / 2
		midY := (e.Y1+e.Y2)/2 - 6
		b.WriteString(fmt.Sprintf("<text x=\"%.1f\" y=\"%.1f\" class=\"edge-label\">%s</text>\n", midX, midY, xmlEscape(e.Type)))
	}

	for idx, n := range layout.Nodes {
		kind := n.Node.Kind
		if kind == "" {
			kind = "Unknown"
		}
		name := n.Node.Name
		if name == "" {
			name = "(unnamed)"
		}
		b.WriteString(fmt.Sprintf("<g class=\"node\" id=\"node-%d\" data-node-id=\"%s\" data-owner=\"%s\">\n", idx, xmlEscape(n.ID), xmlEscape(strings.ToLower(n.Owner))))
		b.WriteString(fmt.Sprintf("  <title>%s (%s owner)</title>\n", xmlEscape(n.ID), xmlEscape(n.Owner)))
		b.WriteString(fmt.Sprintf("  <circle cx=\"%.1f\" cy=\"%.1f\" r=\"28\" fill=\"%s\"/>\n", n.X, n.Y, n.Color))
		b.WriteString(fmt.Sprintf("  <text x=\"%.1f\" y=\"%.1f\">%s</text>\n", n.X, n.Y-6, xmlEscape(kind)))
		b.WriteString(fmt.Sprintf("  <text x=\"%.1f\" y=\"%.1f\">%s</text>\n", n.X, n.Y+8, xmlEscape(name)))
		b.WriteString("</g>\n")
	}

	legendX := layout.Width - 230
	legendY := 140.0
	b.WriteString(fmt.Sprintf("<text x=\"%.1f\" y=\"%.1f\" class=\"legend-title\">Legend</text>\n", legendX, legendY))
	for i, owner := range ownerLegendOrder {
		count := layout.OwnerCounts[owner]
		y := legendY + 26 + float64(i)*26
		b.WriteString(fmt.Sprintf("<circle cx=\"%.1f\" cy=\"%.1f\" r=\"8\" fill=\"%s\" stroke=\"#1f2937\"/>\n", legendX+8, y-4, ownerColors[owner]))
		b.WriteString(fmt.Sprintf("<text x=\"%.1f\" y=\"%.1f\" class=\"legend-item\">%s: %d</text>\n", legendX+24, y, xmlEscape(owner), count))
	}

	b.WriteString("</svg>\n")
	return b.Bytes()
}

func renderGraphHTML(layout visualLayout) ([]byte, error) {
	svg := renderGraphSVG(layout, false)

	var b bytes.Buffer
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("  <meta charset=\"utf-8\" />\n")
	b.WriteString("  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\" />\n")
	b.WriteString("  <title>cub-scout graph export</title>\n")
	b.WriteString("  <style>\n")
	b.WriteString("    body { margin: 0; font-family: Arial, sans-serif; background: #f8fafc; color: #0f172a; }\n")
	b.WriteString("    .shell { display: grid; grid-template-columns: 1fr 320px; min-height: 100vh; gap: 0; }\n")
	b.WriteString("    .canvas { padding: 12px; overflow: auto; background: white; }\n")
	b.WriteString("    .panel { border-left: 1px solid #e2e8f0; padding: 16px; background: #f8fafc; }\n")
	b.WriteString("    .panel h2 { margin: 0 0 10px; font-size: 16px; }\n")
	b.WriteString("    .hint { color: #475569; font-size: 13px; margin-bottom: 12px; }\n")
	b.WriteString("    #node-details { white-space: pre-wrap; font-family: Menlo, monospace; font-size: 12px; color: #0f172a; }\n")
	b.WriteString("    g.node { cursor: pointer; }\n")
	b.WriteString("    g.node.selected circle { stroke: #0f172a; stroke-width: 4; }\n")
	b.WriteString("  </style>\n")
	b.WriteString("</head>\n<body>\n")
	b.WriteString("  <div class=\"shell\">\n")
	b.WriteString("    <div class=\"canvas\">\n")
	if layout.Truncated {
		b.WriteString(fmt.Sprintf("      <p class=\"hint\">Showing %d of %d nodes. Use <code>--max-nodes</code> to adjust.</p>\n", layout.ShownNodes, layout.TotalNodes))
	}
	b.WriteString(string(svg))
	b.WriteString("    </div>\n")
	b.WriteString("    <aside class=\"panel\">\n")
	b.WriteString("      <h2>Node Details</h2>\n")
	b.WriteString("      <p class=\"hint\">Click a node to inspect owner and resource id.</p>\n")
	b.WriteString("      <div id=\"node-details\">No node selected.</div>\n")
	b.WriteString("    </aside>\n")
	b.WriteString("  </div>\n")
	b.WriteString("  <script>\n")
	b.WriteString("  (function() {\n")
	b.WriteString("    const nodes = document.querySelectorAll('g.node');\n")
	b.WriteString("    const detail = document.getElementById('node-details');\n")
	b.WriteString("    function clearSelection() {\n")
	b.WriteString("      nodes.forEach((n) => n.classList.remove('selected'));\n")
	b.WriteString("    }\n")
	b.WriteString("    nodes.forEach((node) => {\n")
	b.WriteString("      node.addEventListener('click', () => {\n")
	b.WriteString("        clearSelection();\n")
	b.WriteString("        node.classList.add('selected');\n")
	b.WriteString("        const nodeID = node.getAttribute('data-node-id') || '(unknown)';\n")
	b.WriteString("        const owner = node.getAttribute('data-owner') || 'unknown';\n")
	b.WriteString("        detail.textContent = 'resource: ' + nodeID + '\\nowner: ' + owner;\n")
	b.WriteString("      });\n")
	b.WriteString("    });\n")
	b.WriteString("  })();\n")
	b.WriteString("  </script>\n")
	b.WriteString("</body>\n</html>\n")

	return b.Bytes(), nil
}

func ownerCountsSummary(counts map[string]int) string {
	parts := make([]string, 0, len(ownerLegendOrder))
	for _, owner := range ownerLegendOrder {
		parts = append(parts, fmt.Sprintf("%s=%d", owner, counts[owner]))
	}
	return "owners: " + strings.Join(parts, ", ")
}

func xmlEscape(s string) string {
	return html.EscapeString(s)
}

func escapeDOT(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}
