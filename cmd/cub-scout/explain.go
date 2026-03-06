// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
)

var (
	explainNamespace string
	explainFormat    string
)

var explainCmd = &cobra.Command{
	Use:   "explain <kind/name> or <kind> <name>",
	Short: "Explain resource ownership and lineage in plain English",
	Long: `Explain provides a plain-English ownership and lineage summary for a resource.

Examples:
  cub-scout explain deploy/my-app -n prod
  cub-scout explain Deployment my-app -n prod --format json
  cub-scout explain deployment/my-app -n prod --format md
`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runExplain,
}

func init() {
	rootCmd.AddCommand(explainCmd)
	explainCmd.Flags().StringVarP(&explainNamespace, "namespace", "n", "", "Namespace of the resource")
	explainCmd.Flags().StringVar(&explainFormat, "format", "text", "Output format: text, json, md")
}

// ExplainSummary is the canonical model for explain output.
type ExplainSummary struct {
	Resource    string   `json:"resource"`
	Namespace   string   `json:"namespace"`
	Owner       string   `json:"owner"`
	Source      string   `json:"source"`
	DeployedVia string   `json:"deployedVia"`
	Health      string   `json:"health"`
	Risks       string   `json:"risks"`
	Drift       string   `json:"drift"`
	Notes       []string `json:"notes,omitempty"`
}

func runExplain(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(explainFormat))
	if format == "ascii" {
		format = "text"
	}
	if format != "text" && format != "json" && format != "md" {
		return fmt.Errorf("invalid --format %q (valid: text, json, md)", explainFormat)
	}

	kind, name, err := parseExplainArgs(args)
	if err != nil {
		return err
	}

	ns := strings.TrimSpace(explainNamespace)
	if ns == "" {
		ns = "default"
	}

	traceResult, err := traceForExplain(cmd.Context(), kind, name, ns)
	if err != nil {
		summary := buildExplainSummaryFromFailure(kind, name, ns, err)
		return outputExplainSummary(summary, format)
	}

	summary := buildExplainSummary(traceResult)
	if summary.Resource == "" {
		summary.Resource = fmt.Sprintf("%s/%s", kind, name)
	}
	if summary.Namespace == "" {
		summary.Namespace = ns
	}

	return outputExplainSummary(summary, format)
}

func outputExplainSummary(summary ExplainSummary, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	case "md":
		fmt.Print(renderExplainMarkdown(summary))
		return nil
	default:
		fmt.Print(renderExplainText(summary))
		return nil
	}
}

func parseExplainArgs(args []string) (kind, name string, err error) {
	if len(args) == 1 {
		parts := strings.SplitN(args[0], "/", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("invalid resource format: use kind/name (e.g., deployment/nginx)")
		}
		return normalizeKind(parts[0]), parts[1], nil
	}
	return normalizeKind(args[0]), args[1], nil
}

func traceForExplain(ctx context.Context, kind, name, namespace string) (*agent.TraceResult, error) {
	ownership, err := detectResourceOwnership(ctx, kind, name, namespace)
	if err != nil {
		ownership = &agent.Ownership{Type: agent.OwnerUnknown}
	}

	tracers := buildExplainTracerCandidates(ownership.Type)
	if len(tracers) == 0 {
		return nil, fmt.Errorf("no available tracer candidates for owner type %q", ownership.Type)
	}

	var (
		lastErr   error
		candidate *agent.TraceResult
	)

	for _, tracer := range tracers {
		if tracer == nil || !tracer.Available() {
			continue
		}

		result, err := tracer.Trace(ctx, kind, name, namespace)
		if err != nil {
			lastErr = err
			continue
		}
		if result == nil {
			continue
		}
		if result.Object.Kind == "" {
			result.Object.Kind = kind
			result.Object.Name = name
			result.Object.Namespace = namespace
		}
		candidate = result
		if len(result.Chain) > 0 {
			return result, nil
		}
		if strings.TrimSpace(result.Error) == "" {
			return result, nil
		}
	}

	if candidate != nil {
		return candidate, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("unable to trace resource via available tracers")
}

func buildExplainTracerCandidates(ownerType string) []agent.Tracer {
	var tracers []agent.Tracer

	addFlux := func() { tracers = append(tracers, agent.NewFluxTracer()) }
	addArgo := func() { tracers = append(tracers, agent.NewArgoTracer()) }
	addHelm := func() {
		cfg, cfgErr := buildConfig()
		if cfgErr != nil {
			return
		}
		clientset, clientErr := kubernetes.NewForConfig(cfg)
		if clientErr != nil {
			return
		}
		tracers = append(tracers, agent.NewHelmTracer(clientset))
	}

	switch ownerType {
	case agent.OwnerArgo:
		addArgo()
		addFlux()
		addHelm()
	case agent.OwnerHelm:
		addHelm()
		addFlux()
		addArgo()
	case agent.OwnerFlux:
		addFlux()
		addArgo()
		addHelm()
	default:
		addFlux()
		addArgo()
		addHelm()
	}

	return tracers
}

func buildExplainSummary(result *agent.TraceResult) ExplainSummary {
	summary := ExplainSummary{
		Resource:    fmt.Sprintf("%s/%s", result.Object.Kind, result.Object.Name),
		Namespace:   result.Object.Namespace,
		Owner:       explainOwner(result.Tool),
		Source:      "unknown",
		DeployedVia: "unknown",
		Health:      "Unknown",
		Risks:       "Not assessed",
		Drift:       "Unknown",
	}

	if len(result.Chain) > 0 {
		summary.DeployedVia = explainDeploymentChain(result.Chain)
		summary.Source = explainSource(result.Chain)
		leaf := result.Chain[len(result.Chain)-1]
		if strings.TrimSpace(leaf.Status) != "" {
			summary.Health = leaf.Status
		} else if leaf.Ready {
			summary.Health = "Healthy"
		} else {
			summary.Health = "Unhealthy"
		}
	}

	if result.ConfigHub != nil {
		if result.ConfigHub.DriftDetected {
			summary.Drift = "Detected by ConfigHub"
		} else {
			summary.Drift = "None detected"
		}
	}

	if result.Error != "" {
		summary.Owner = "Unknown - no recognized ownership labels found"
		if summary.Health == "Unknown" {
			summary.Health = "Unavailable"
		}
		if summary.DeployedVia == "unknown" {
			summary.DeployedVia = "partial trace only"
		}
		summary.Notes = append(summary.Notes, "partial trace: no GitOps owner chain was discovered")
	}

	return summary
}

func buildExplainSummaryFromFailure(kind, name, namespace string, err error) ExplainSummary {
	note := "partial trace: no GitOps owner chain was discovered"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		note = fmt.Sprintf("partial trace: %s", strings.TrimSpace(err.Error()))
	}

	return ExplainSummary{
		Resource:    fmt.Sprintf("%s/%s", kind, name),
		Namespace:   namespace,
		Owner:       "Unknown - no recognized ownership labels found",
		Source:      "unknown",
		DeployedVia: "partial trace only",
		Health:      "Unavailable",
		Risks:       "Not assessed",
		Drift:       "Unknown",
		Notes:       []string{note},
	}
}

func explainOwner(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "flux":
		return "Flux"
	case "argocd", "argo":
		return "ArgoCD"
	case "helm":
		return "Helm"
	default:
		return "Unknown"
	}
}

func explainSource(chain []agent.ChainLink) string {
	url := ""
	path := ""
	rev := ""

	for _, link := range chain {
		if url == "" && strings.TrimSpace(link.URL) != "" {
			url = strings.TrimSpace(link.URL)
		}
		if path == "" && strings.TrimSpace(link.Path) != "" {
			path = strings.TrimSpace(link.Path)
		}
		if rev == "" && strings.TrimSpace(link.Revision) != "" {
			rev = strings.TrimSpace(link.Revision)
		}
	}

	if url == "" {
		url = "unknown"
	}

	details := make([]string, 0, 2)
	if path != "" {
		details = append(details, fmt.Sprintf("path: %s", path))
	}
	if rev != "" {
		details = append(details, fmt.Sprintf("revision: %s", rev))
	}
	if len(details) == 0 {
		return url
	}
	return fmt.Sprintf("%s (%s)", url, strings.Join(details, ", "))
}

func explainDeploymentChain(chain []agent.ChainLink) string {
	parts := make([]string, 0, len(chain))
	for _, link := range chain {
		parts = append(parts, fmt.Sprintf("%s/%s", link.Kind, link.Name))
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, " -> ")
}

func renderExplainText(summary ExplainSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s in namespace %s:\n", summary.Resource, summary.Namespace)
	fmt.Fprintf(&b, "  Owner: %s\n", summary.Owner)
	fmt.Fprintf(&b, "  Source: %s\n", summary.Source)
	fmt.Fprintf(&b, "  Deployed via: %s\n", summary.DeployedVia)
	fmt.Fprintf(&b, "  Health: %s\n", summary.Health)
	fmt.Fprintf(&b, "  Risks: %s\n", summary.Risks)
	fmt.Fprintf(&b, "  Drift: %s\n", summary.Drift)
	if len(summary.Notes) > 0 {
		fmt.Fprintf(&b, "  Notes:\n")
		for _, note := range summary.Notes {
			fmt.Fprintf(&b, "    - %s\n", note)
		}
	}
	hints := explainTryNextHints(summary)
	if len(hints) > 0 {
		b.WriteString(renderTryNextSection(hints))
	}
	return b.String()
}

func renderExplainMarkdown(summary ExplainSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Explain\n\n")
	fmt.Fprintf(&b, "- **Resource:** `%s`\n", summary.Resource)
	fmt.Fprintf(&b, "- **Namespace:** `%s`\n", summary.Namespace)
	fmt.Fprintf(&b, "- **Owner:** %s\n", summary.Owner)
	fmt.Fprintf(&b, "- **Source:** %s\n", summary.Source)
	fmt.Fprintf(&b, "- **Deployed via:** %s\n", summary.DeployedVia)
	fmt.Fprintf(&b, "- **Health:** %s\n", summary.Health)
	fmt.Fprintf(&b, "- **Risks:** %s\n", summary.Risks)
	fmt.Fprintf(&b, "- **Drift:** %s\n", summary.Drift)
	if len(summary.Notes) > 0 {
		fmt.Fprintf(&b, "- **Notes:**\n")
		for _, note := range summary.Notes {
			fmt.Fprintf(&b, "  - %s\n", note)
		}
	}
	b.WriteString(renderTryNextMarkdown(explainTryNextHints(summary)))
	return b.String()
}
