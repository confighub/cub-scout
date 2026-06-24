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
	Resource                 string   `json:"resource"`
	Namespace                string   `json:"namespace"`
	Owner                    string   `json:"owner"`
	Source                   string   `json:"source"`
	DeployedVia              string   `json:"deployedVia"`
	Health                   string   `json:"health"`
	Risks                    string   `json:"risks"`
	Drift                    string   `json:"drift"`
	Notes                    []string `json:"notes,omitempty"`
	ConfigHubURL             string   `json:"confighubUrl,omitempty"`             // Canonical unit detail URL in ConfigHub GUI (only when connected)
	ConfigHubRevisionsURL    string   `json:"confighubRevisionsUrl,omitempty"`    // Canonical unit revisions URL in ConfigHub GUI (only when connected)
	ConfigHubRevisionNum     string   `json:"configHubRevisionNum,omitempty"`     // Deployed ConfigHub revision when known
	ConfigHubLiveRevisionNum string   `json:"configHubLiveRevisionNum,omitempty"` // Latest/live ConfigHub revision when known

	// Events contains recent Kubernetes events for the resource.
	// Prioritizes Warning/error events, bounded to top 5.
	Events *agent.ResourceEventSummary `json:"events,omitempty"`

	// ThreeWay contains the three-way disagreement status (ConfigHub vs controller vs cluster).
	// Only populated in connected mode when disagreement is detected.
	ThreeWay *ThreeWayDisagreement `json:"threeWay,omitempty"`

	// NextSteps contains structured action-typed hints for AI/MCP clients.
	NextSteps []StructuredHint `json:"nextSteps,omitempty"`

	// MutationCause classifies the source of recent field mutations on the
	// live resource (controller-drift / manual-edit / unknown), derived from
	// metadata.managedFields with the detected owner as co-signal. See
	// pkg/agent.AttributeFieldMutation.
	MutationCause agent.FieldMutationCause `json:"mutationCause,omitempty"`

	// MutationManager is a representative manager string for transparency
	// (see pkg/agent.FieldMutationAttribution.ManagerHint).
	MutationManager string `json:"mutationManager,omitempty"`
}

type explainTraceApplicationFunc func(ctx context.Context, appName string) (*agent.TraceResult, error)

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
		// Populate structured hints for JSON output (reuses existing hint logic)
		hints := explainHintsWithContext(summary, hintCtx)
		// Include ConfigHub hint if present (backs the visible "OPEN IN CONFIGHUB" section)
		if chHint := explainConfigHubHint(summary); chHint != nil {
			hints = append(hints, *chHint)
		}
		sortHints(hints)
		if len(hints) > 3 {
			hints = hints[:3]
		}
		summary.NextSteps = HintsToStructured(hints)

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

	if result, err, attempted := traceOwnedArgoResourceForExplain(
		ctx,
		kind,
		name,
		namespace,
		ownership,
		func(ctx context.Context, appName string) (*agent.TraceResult, error) {
			tracer := agent.NewArgoTracer()
			if !tracer.Available() {
				return nil, fmt.Errorf("argocd CLI not found - install from https://argo-cd.readthedocs.io/en/stable/cli_installation/")
			}
			return tracer.TraceApplication(ctx, appName)
		},
	); attempted {
		if err == nil && result != nil {
			return result, nil
		}
		lastErr = err
	}

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

		// Successful trace with chain - return immediately
		if len(result.Chain) > 0 {
			return result, nil
		}

		// Successful trace without error - return immediately
		if strings.TrimSpace(result.Error) == "" {
			return result, nil
		}

		// Result has an error but no chain. Check if this is a "negative mismatch":
		// a tracer saying "not managed by me" when we already know the owner is different.
		// Don't let these override a known ownership signal.
		if isNegativeMismatchCandidate(result, ownership) {
			// Skip this candidate - it's a tracer saying "not mine" when we
			// know the resource belongs to a different tool
			continue
		}

		// Accept this partial result as candidate
		candidate = result
	}

	// If we have a same-tool partial (or unknown ownership), use it
	if candidate != nil {
		return candidate, nil
	}

	// If ownership was detected but tracers failed or only returned negative mismatches,
	// return a partial result with the ownership information preserved. This allows
	// explain to show "ArgoCD" instead of "Unknown" for ApplicationSet-managed
	// resources where the ArgoTracer can't complete a full trace chain.
	if ownership != nil && ownership.Type != agent.OwnerUnknown {
		return buildOwnershipOnlyTraceResult(kind, name, namespace, ownership, lastErr), nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("unable to trace resource via available tracers")
}

func traceOwnedArgoResourceForExplain(
	ctx context.Context,
	kind, name, namespace string,
	ownership *agent.Ownership,
	traceApp explainTraceApplicationFunc,
) (*agent.TraceResult, error, bool) {
	if ownership == nil || ownership.Type != agent.OwnerArgo || strings.TrimSpace(ownership.Name) == "" || traceApp == nil {
		return nil, nil, false
	}

	result, err := traceApp(ctx, ownership.Name)
	if err != nil {
		return nil, err, true
	}
	if result == nil {
		return nil, nil, true
	}

	return projectArgoApplicationTraceToResource(result, kind, name, namespace), nil, true
}

func projectArgoApplicationTraceToResource(result *agent.TraceResult, kind, name, namespace string) *agent.TraceResult {
	if result == nil {
		return nil
	}

	projected := *result
	projected.Object = agent.ResourceRef{
		Kind:      kind,
		Name:      name,
		Namespace: namespace,
	}

	sourceIdx := -1
	appIdx := -1
	targetIdx := -1
	for i, link := range result.Chain {
		if sourceIdx == -1 && isExplainSourceLink(link.Kind) {
			sourceIdx = i
		}
		if appIdx == -1 && strings.EqualFold(strings.TrimSpace(link.Kind), "Application") {
			appIdx = i
		}
		if strings.EqualFold(strings.TrimSpace(link.Kind), kind) &&
			strings.EqualFold(strings.TrimSpace(link.Name), name) &&
			strings.EqualFold(strings.TrimSpace(link.Namespace), namespace) {
			targetIdx = i
		}
	}

	if sourceIdx != -1 || appIdx != -1 || targetIdx != -1 {
		projectedChain := make([]agent.ChainLink, 0, 3)
		seen := make(map[int]struct{})
		appendIfPresent := func(idx int) {
			if idx < 0 {
				return
			}
			if _, ok := seen[idx]; ok {
				return
			}
			projectedChain = append(projectedChain, result.Chain[idx])
			seen[idx] = struct{}{}
		}
		appendIfPresent(sourceIdx)
		appendIfPresent(appIdx)
		appendIfPresent(targetIdx)
		projected.Chain = projectedChain
	}

	return &projected
}

func isExplainSourceLink(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "Source", "HelmChart", "ConfigHub OCI", "OCIRepository", "GitRepository":
		return true
	default:
		return false
	}
}

// isNegativeMismatchCandidate returns true if the trace result is a "negative mismatch":
// a tracer reporting "resource not managed by me" when we already detected that
// the resource is owned by a *different* tool.
//
// For example, if ownership detection found ArgoCD (via tracking-id), and FluxTracer
// returns "resource not managed by Flux", that's a negative mismatch we should skip
// rather than accept as the final answer.
func isNegativeMismatchCandidate(result *agent.TraceResult, ownership *agent.Ownership) bool {
	// If ownership is unknown, we can't determine mismatch
	if ownership == nil || ownership.Type == agent.OwnerUnknown {
		return false
	}

	// If result has no error, it's not a negative result
	errMsg := strings.TrimSpace(result.Error)
	if errMsg == "" {
		return false
	}

	// Check if the result's tool matches the detected ownership
	resultTool := strings.ToLower(strings.TrimSpace(result.Tool))
	ownerTool := ownerTypeToToolName(ownership.Type)

	// If tools match, this is a same-tool partial - not a mismatch
	if resultTool == ownerTool {
		return false
	}

	// Check for "not managed" patterns that indicate a negative result
	errLower := strings.ToLower(errMsg)
	negativePatterns := []string{
		"not managed",
		"no flux object found",
		"object not managed",
		"no helm release found",
		"not found managing",
	}

	for _, pattern := range negativePatterns {
		if strings.Contains(errLower, pattern) {
			return true
		}
	}

	return false
}

// ownerTypeToToolName converts an ownership type constant to its tool name.
func ownerTypeToToolName(ownerType string) string {
	switch ownerType {
	case agent.OwnerArgo:
		return "argocd"
	case agent.OwnerFlux:
		return "flux"
	case agent.OwnerHelm:
		return "helm"
	default:
		return ownerType
	}
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
		if result.ConfigHub.UnitURL != "" {
			summary.ConfigHubURL = result.ConfigHub.UnitURL
		} else if result.ConfigHub.RemediationURL != "" {
			summary.ConfigHubURL = result.ConfigHub.RemediationURL
		}
		summary.ConfigHubRevisionsURL = result.ConfigHub.RevisionsURL
		summary.ConfigHubRevisionNum = strings.TrimSpace(result.ConfigHub.RevisionNum)
		summary.ConfigHubLiveRevisionNum = strings.TrimSpace(result.ConfigHub.LiveRevisionNum)
	}

	if strings.TrimSpace(result.GeneratedByApplicationSet) != "" {
		summary.Notes = append(summary.Notes, fmt.Sprintf("generated by ApplicationSet/%s", strings.TrimSpace(result.GeneratedByApplicationSet)))
	}
	if strings.TrimSpace(result.ParentApplication) != "" {
		summary.Notes = append(summary.Notes, fmt.Sprintf("child of Application/%s", strings.TrimSpace(result.ParentApplication)))
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
		note := "partial trace: no GitOps owner chain was discovered"
		if len(result.Chain) > 0 {
			note = strings.TrimSpace(result.Error)
			if note == "" {
				note = "partial trace: no GitOps owner chain was discovered"
			} else if !strings.HasPrefix(strings.ToLower(note), "partial trace:") {
				note = "partial trace: " + note
			}
		}
		summary.Notes = append(summary.Notes, note)
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
	if summary.MutationCause != "" {
		mutationLine := string(summary.MutationCause)
		if summary.MutationManager != "" {
			mutationLine += fmt.Sprintf(" (manager: %s)", summary.MutationManager)
		}
		fmt.Fprintf(&b, "  %s %s\n", label("Mutation cause"), colorExplainMutationCause(summary.MutationCause, mutationLine))
	}
	if len(summary.Notes) > 0 {
		fmt.Fprintf(&b, "  %s\n", label("Notes"))
		for _, note := range summary.Notes {
			fmt.Fprintf(&b, "    - %s\n", Yellow(note))
		}
	}

	// Three-way disagreement section (connected mode only)
	if summary.ThreeWay != nil && summary.ThreeWay.IsDisagreement() {
		b.WriteString(formatThreeWaySection(summary.ThreeWay))
	}

	// Recent events section
	if summary.Events != nil && len(summary.Events.Events) > 0 {
		b.WriteString(renderEventsSection(summary.Events, mode, explicitMode))
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
	case strings.Contains(lower, "sveltos"):
		return OwnerColor("Sveltos")
	case strings.Contains(lower, "modelplane"):
		return OwnerColor("Modelplane")
	case strings.Contains(lower, "crossplane"):
		return OwnerColor("Crossplane")
	case strings.Contains(lower, "kro"):
		return OwnerColor("kro")
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

// colorExplainMutationCause colors the mutation cause line. Manual edits are
// the actionable case, so they get amber emphasis; controller drift is
// expected steady-state coloring.
func colorExplainMutationCause(cause agent.FieldMutationCause, text string) string {
	switch cause {
	case agent.CauseManualEdit:
		return Yellow(text)
	case agent.CauseControllerDrift:
		return text
	default:
		return text
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
	if summary.MutationCause != "" {
		mutationLine := fmt.Sprintf("`%s`", summary.MutationCause)
		if summary.MutationManager != "" {
			mutationLine += fmt.Sprintf(" (manager: `%s`)", summary.MutationManager)
		}
		fmt.Fprintf(&b, "- **Mutation cause:** %s\n", mutationLine)
	}
	if len(summary.Notes) > 0 {
		fmt.Fprintf(&b, "- **Notes:**\n")
		for _, note := range summary.Notes {
			fmt.Fprintf(&b, "  - %s\n", note)
		}
	}

	// Three-way disagreement section (connected mode only)
	if summary.ThreeWay != nil && summary.ThreeWay.IsDisagreement() {
		b.WriteString(formatThreeWayMarkdown(summary.ThreeWay))
	}

	// Recent events section
	if summary.Events != nil && len(summary.Events.Events) > 0 {
		b.WriteString(renderEventsMarkdown(summary.Events, mode, explicitMode))
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
		if summary.ConfigHubRevisionsURL != "" {
			b.WriteString(fmt.Sprintf("- [Review revision history in ConfigHub](%s)\n", summary.ConfigHubRevisionsURL))
		}
	}

	// Outro for explicit AI mode - at the true end after all content
	if explicitMode && mode == PresentationAI {
		b.WriteString("\n[end resource context]\n")
	}
	return b.String()
}

// renderEventsSection renders the events section for text output.
func renderEventsSection(events *agent.ResourceEventSummary, mode PresentationMode, explicitMode bool) string {
	var b strings.Builder

	// Section heading
	if explicitMode && mode == PresentationAI {
		fmt.Fprintf(&b, "\nRECENT EVENTS:\n")
	} else {
		fmt.Fprintf(&b, "\n%sRecent Events:%s\n", Bold(""), "")
	}

	// Summary line
	if events.WarningCount > 0 || events.ErrorCount > 0 {
		fmt.Fprintf(&b, "  %s%d warning(s), %d error(s) of %d total%s\n",
			Yellow(""), events.WarningCount, events.ErrorCount, events.TotalCount, "")
	}

	// Event list
	for _, ev := range events.Events {
		var icon, color string
		reset := ""
		switch ev.Severity {
		case "error":
			icon = SymError
			color = Red("")
		case "warning":
			icon = "⚠"
			color = Yellow("")
		default:
			icon = "○"
			color = Dim("")
		}

		// Format: icon Age Reason: Message (xCount)
		countStr := ""
		if ev.Count > 1 {
			countStr = fmt.Sprintf(" (x%d)", ev.Count)
		}

		fmt.Fprintf(&b, "  %s%s%s %s %s: %s%s%s\n",
			color, icon, reset,
			ev.Age,
			ev.Reason,
			ev.Message,
			countStr,
			reset,
		)
	}

	return b.String()
}

// renderEventsMarkdown renders the events section for markdown output.
func renderEventsMarkdown(events *agent.ResourceEventSummary, mode PresentationMode, explicitMode bool) string {
	var b strings.Builder

	// Section heading
	if explicitMode && mode == PresentationAI {
		fmt.Fprintf(&b, "\n### RECENT EVENTS\n\n")
	} else {
		fmt.Fprintf(&b, "\n### Recent Events\n\n")
	}

	// Summary line
	if events.WarningCount > 0 || events.ErrorCount > 0 {
		fmt.Fprintf(&b, "**%d warning(s), %d error(s)** of %d total\n\n",
			events.WarningCount, events.ErrorCount, events.TotalCount)
	}

	// Event table
	fmt.Fprintf(&b, "| Age | Type | Reason | Message |\n")
	fmt.Fprintf(&b, "|-----|------|--------|--------|\n")
	for _, ev := range events.Events {
		msg := ev.Message
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}
		countStr := ""
		if ev.Count > 1 {
			countStr = fmt.Sprintf(" (x%d)", ev.Count)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s%s |\n",
			ev.Age, ev.Type, ev.Reason, msg, countStr)
	}

	return b.String()
}
