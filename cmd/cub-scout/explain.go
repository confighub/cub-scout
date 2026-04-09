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
	explainNamespace    string
	explainFormat       string
	explainPresentation string
	explainHintMode     string
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
	explainCmd.Flags().StringVar(&explainPresentation, "presentation", "", PresentationModeHelp())
	explainCmd.Flags().StringVar(&explainHintMode, "hint-mode", "", HintModeHelp())
}

// ExplainSummary is the canonical model for explain output.
type ExplainSummary struct {
	Resource     string   `json:"resource"`
	Namespace    string   `json:"namespace"`
	Owner        string   `json:"owner"`
	Source       string   `json:"source"`
	DeployedVia  string   `json:"deployedVia"`
	Health       string   `json:"health"`
	Risks        string   `json:"risks"`
	Drift        string   `json:"drift"`
	Notes        []string `json:"notes,omitempty"`
	ConfigHubURL string   `json:"confighubUrl,omitempty"` // URL to view/manage in ConfigHub GUI (only when connected)
}

func runExplain(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(explainFormat))
	if format == "ascii" {
		format = "text"
	}
	if format != "text" && format != "json" && format != "md" {
		return fmt.Errorf("invalid --format %q (valid: text, json, md)", explainFormat)
	}

	// Build invocation context with presentation mode resolution
	invCtx, err := NewInvocationContext(explainPresentation, TransportCLI)
	if err != nil {
		return err
	}

	// Parse hint mode (separate from presentation mode)
	hintMode, err := ParseHintMode(explainHintMode)
	if err != nil {
		return err
	}
	hintCtx := HintContext{Mode: hintMode}

	kind, name, err := parseExplainArgs(args)
	if err != nil {
		return err
	}

	// Call the shared capability seam
	summary, err := ObserveResourceContext(cmd.Context(), ObserveResourceContextRequest{
		Kind:      kind,
		Name:      name,
		Namespace: explainNamespace,
	})
	if err != nil {
		return err
	}

	return outputExplainSummary(summary, format, invCtx, hintCtx)
}

func outputExplainSummary(summary ExplainSummary, format string, invCtx InvocationContext, hintCtx HintContext) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	case "md":
		fmt.Print(renderExplainMarkdown(summary, invCtx.Mode(), invCtx.IsExplicit(), hintCtx))
		return nil
	default:
		fmt.Print(renderExplainText(summary, invCtx.Mode(), invCtx.IsExplicit(), hintCtx))
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

	// Custom owners don't have GitOps trace chains — return immediately
	// with the custom owner information so buildExplainSummary can extract it.
	if ownership.Type == agent.OwnerCustom {
		return buildCustomOwnerUnsupportedTraceResult(kind, name, namespace, ownership), nil
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

	// If ownership was detected but tracers failed, return a partial result
	// with the ownership information preserved. This allows explain to show
	// "ArgoCD" instead of "Unknown" for ApplicationSet-managed resources
	// where the ArgoTracer can't complete a full trace chain.
	if ownership != nil && ownership.Type != agent.OwnerUnknown {
		return buildOwnershipOnlyTraceResult(kind, name, namespace, ownership, lastErr), nil
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
		// Include ConfigHub URL if available (for GUI handoff)
		if result.ConfigHub.RemediationURL != "" {
			summary.ConfigHubURL = result.ConfigHub.RemediationURL
		}
	}

	if result.Error != "" {
		if customOwner := customOwnerFromTraceError(result.Error); customOwner != "" {
			summary.Owner = customOwner
		} else if summary.Owner == "Unknown" {
			// Only set "Unknown - no recognized..." if no ownership was detected.
			// If ownership was detected (e.g., ArgoCD via tracking-id), preserve it
			// even when the full trace chain couldn't be completed.
			summary.Owner = "Unknown - no recognized ownership labels found"
		}
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

// buildOwnershipOnlyTraceResult creates a partial TraceResult when ownership
// was detected (e.g., via ArgoCD tracking-id) but the tracer couldn't complete
// a full chain trace. This preserves the ownership information so explain
// can show "ArgoCD Application X" instead of "Unknown".
func buildOwnershipOnlyTraceResult(kind, name, namespace string, ownership *agent.Ownership, traceErr error) *agent.TraceResult {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		FullyManaged: false,
	}

	// Map ownership type to tool name
	switch ownership.Type {
	case agent.OwnerArgo:
		result.Tool = "argocd"
	case agent.OwnerFlux:
		result.Tool = "flux"
	case agent.OwnerHelm:
		result.Tool = "helm"
	default:
		result.Tool = ownership.Type
	}

	// Build an informative error message that includes the ownership context
	errParts := []string{"ownership detected via " + ownership.Source}
	if ownership.Name != "" {
		errParts = append(errParts, fmt.Sprintf("owner: %s", ownership.Name))
	}
	if traceErr != nil {
		errParts = append(errParts, fmt.Sprintf("trace incomplete: %s", traceErr.Error()))
	} else {
		errParts = append(errParts, "full trace chain unavailable")
	}
	result.Error = strings.Join(errParts, "; ")

	return result
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

func customOwnerFromTraceError(traceErr string) string {
	const prefix = "custom owner detected:"
	msg := strings.TrimSpace(traceErr)
	if !strings.HasPrefix(strings.ToLower(msg), prefix) {
		return ""
	}
	owner := strings.TrimSpace(msg[len(prefix):])
	if idx := strings.Index(owner, "("); idx > 0 {
		owner = strings.TrimSpace(owner[:idx])
	}
	return owner
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

func renderExplainText(summary ExplainSummary, mode PresentationMode, explicitMode bool, hintCtx HintContext) string {
	var b strings.Builder

	// Helper for legacy vs explicit mode label formatting
	label := func(text string) string {
		if explicitMode {
			return SectionLabel(mode, text)
		}
		return Dim(text + ":")
	}

	// Heading - only use presentation-specific format when explicitly requested
	if explicitMode {
		heading := ExplainHeading(mode, summary.Resource, summary.Namespace)
		fmt.Fprintf(&b, "%s\n", heading)
	} else {
		// Legacy format
		fmt.Fprintf(&b, "%s in namespace %s:\n", Bold(summary.Resource), summary.Namespace)
	}

	fmt.Fprintf(&b, "  %s %s\n", label("Owner"), colorExplainOwner(summary.Owner))
	fmt.Fprintf(&b, "  %s %s\n", label("Source"), summary.Source)
	fmt.Fprintf(&b, "  %s %s\n", label("Deployed via"), summary.DeployedVia)
	fmt.Fprintf(&b, "  %s %s\n", label("Health"), StatusColor(summary.Health))
	fmt.Fprintf(&b, "  %s %s\n", label("Risks"), summary.Risks)
	fmt.Fprintf(&b, "  %s %s\n", label("Drift"), colorExplainDrift(summary.Drift))
	if len(summary.Notes) > 0 {
		fmt.Fprintf(&b, "  %s\n", label("Notes"))
		for _, note := range summary.Notes {
			fmt.Fprintf(&b, "    - %s\n", Yellow(note))
		}
	}

	// Outro - only for explicit AI mode
	if explicitMode {
		outro := ExplainOutro(mode)
		if outro != "" {
			fmt.Fprintf(&b, "\n%s\n", outro)
		}
	}

	hints := explainTryNextHintsWithContext(summary, hintCtx)
	if len(hints) > 0 {
		if explicitMode {
			b.WriteString(renderTryNextSectionWithMode(hints, mode))
		} else {
			b.WriteString(renderTryNextSection(hints))
		}
	}
	// Add ConfigHub GUI suggestion if available
	if chHint := explainConfigHubHint(summary); chHint != nil {
		b.WriteString(renderConfigHubSection(chHint))
	}
	return b.String()
}

// colorExplainOwner colors the owner field based on its content.
func colorExplainOwner(owner string) string {
	lower := strings.ToLower(strings.TrimSpace(owner))
	switch {
	case strings.HasPrefix(lower, "unknown"):
		return Yellow(owner)
	case strings.Contains(lower, "flux"):
		return OwnerColor("Flux")
	case strings.Contains(lower, "argo"):
		return OwnerColor("ArgoCD")
	case strings.Contains(lower, "helm"):
		return OwnerColor("Helm")
	case strings.Contains(lower, "confighub"):
		return OwnerColor("ConfigHub")
	default:
		return owner
	}
}

// colorExplainDrift colors drift status.
func colorExplainDrift(drift string) string {
	lower := strings.ToLower(strings.TrimSpace(drift))
	switch {
	case strings.Contains(lower, "detected"):
		return Yellow(drift)
	case strings.Contains(lower, "none"):
		return Green(drift)
	default:
		return drift
	}
}

func renderExplainMarkdown(summary ExplainSummary, mode PresentationMode, explicitMode bool, hintCtx HintContext) string {
	var b strings.Builder

	// Heading - only use presentation-specific format when explicitly requested
	if explicitMode && mode == PresentationAI {
		fmt.Fprintf(&b, "## RESOURCE CONTEXT\n\n")
		fmt.Fprintf(&b, "[resource: %s namespace: %s]\n\n", summary.Resource, summary.Namespace)
	} else {
		// Legacy/human/paired format
		fmt.Fprintf(&b, "## Explain\n\n")
		fmt.Fprintf(&b, "- **Resource:** `%s`\n", summary.Resource)
		fmt.Fprintf(&b, "- **Namespace:** `%s`\n", summary.Namespace)
	}

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

	if explicitMode {
		b.WriteString(renderTryNextMarkdownWithMode(explainTryNextHintsWithContext(summary, hintCtx), mode))
	} else {
		b.WriteString(renderTryNextMarkdown(explainTryNextHintsWithContext(summary, hintCtx)))
	}
	// Add ConfigHub link if available
	if summary.ConfigHubURL != "" {
		b.WriteString("\n### Open in ConfigHub\n\n")
		b.WriteString(fmt.Sprintf("- [Review this unit in ConfigHub](%s)\n", summary.ConfigHubURL))
	}

	// Outro for explicit AI mode - at the true end after all content
	if explicitMode && mode == PresentationAI {
		b.WriteString("\n[end resource context]\n")
	}
	return b.String()
}
