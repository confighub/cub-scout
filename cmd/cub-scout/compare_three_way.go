// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
)

type threeWayScopeType string

const (
	threeWayScopeResource  threeWayScopeType = "resource"
	threeWayScopeNamespace threeWayScopeType = "namespace"
	threeWayScopeCluster   threeWayScopeType = "cluster"
	threeWayScopeView      threeWayScopeType = "view"
)

type threeWayScope struct {
	ScopeType  threeWayScopeType
	ScopeValue string
}

type threeWayTarget struct {
	ResourceArg string
	Namespace   string
}

type threeWayResourceEntry struct {
	Result        compareResourceResult  `json:"result"`
	Severity      string                 `json:"severity"`
	Causes        []string               `json:"causes,omitempty"`
	Pattern       ThreeWayPattern        `json:"pattern"`
	CurrentChange *agent.RolloutDecision `json:"currentChange,omitempty"`
}

// ConformanceResult contains the pass/fail conformance summary.
// The Pass field is computed based on the --fail-on threshold if set,
// ensuring JSON, text output, and exit code all tell the same story.
type ConformanceResult struct {
	Pass        bool   `json:"pass"`
	Threshold   string `json:"threshold,omitempty"` // --fail-on value if set, empty for pure reporting
	MaxSeverity string `json:"maxSeverity"`         // highest severity found (ok, info, warning)
	Violations  int    `json:"violations"`          // count of resources with non-ok severity
}

type threeWaySummary struct {
	TotalResources      int               `json:"totalResources"`
	ConnectedResources  int               `json:"connectedResources"`
	DryWetLiveResources int               `json:"dryWetLiveResources"`
	MismatchedResources int               `json:"mismatchedResources"`
	SeverityCounts      map[string]int    `json:"severityCounts"`
	CauseBuckets        map[string]int    `json:"causeBuckets"`
	Conformance         ConformanceResult `json:"conformance"`
	Agreement           AgreementSummary  `json:"agreement"`
}

type threeWayReport struct {
	Scope                 string                  `json:"scope"`
	ConfigHubURL          string                  `json:"confighubUrl,omitempty"`
	ConfigHubRevisionsURL string                  `json:"confighubRevisionsUrl,omitempty"`
	Summary               threeWaySummary         `json:"summary"`
	Resources             []threeWayResourceEntry `json:"resources"`
	NextSteps             []StructuredHint        `json:"nextSteps,omitempty"`
}

// Exit codes for compare three-way command (CI contract)
const (
	// ConformanceExitOK indicates no failure triggered
	ConformanceExitOK = 0

	// ConformanceExitError indicates operational error (bad args, cluster access, etc.)
	ConformanceExitError = 1

	// ConformanceExitFailure indicates conformance violations met the --fail-on threshold
	ConformanceExitFailure = 2
)

var (
	compareThreeWayScopeRaw string
	compareThreeWayView     string
	compareThreeWayFormat   string
	compareThreeWayJSON     bool
	compareThreeWayFailOn   string
	compareThreeWayDryFrom  string

	buildThreeWayResourceResultFn = buildCompareResourceResult
	discoverThreeWayNamespacesFn  = discoverNamespacesWithWorkloads
	discoverThreeWayWorkloadsFn   = discoverWorkloads
)

var compareThreeWayCmd = &cobra.Command{
	Use:   "three-way",
	Short: "Compare intent vs rendered vs observed state",
	Long: `Connected three-way comparison (DRY/WET/LIVE) for a selected scope.

Supported scopes (--scope):
  deploy/my-app            (resource)
  resource:deploy/my-app   (resource)
  namespace/prod           (namespace)
  cluster                  (all namespaces)

View-scoped comparison (--view):
  Constrain the comparison to live cluster resources whose ConfigHub units
  are selected by a View's filter. Accepts a bare UUID or a View Explorer
  URL (paste from the browser address bar). --scope and --view are mutually
  exclusive. Requires connected mode.

Examples:
  cub-scout compare three-way --scope deploy/payment-api -n prod
  cub-scout compare three-way --scope namespace/prod --format json
  cub-scout compare three-way --scope cluster --format md

  # View-scoped: compare only resources selected by a saved View
  cub-scout compare three-way --view 806aac53-236c-446d-8ad6-91d6daf6810e
  cub-scout compare three-way --view 'https://hub.confighub.com/x/view-explorer?view=806aac53-...'

  # CI/automation: fail if mismatches found
  cub-scout compare three-way --scope namespace/prod --fail-on warning

Exit codes:
  0  No failure triggered (or no --fail-on specified)
  1  Operational error (bad arguments, cluster access failure, etc.)
  2  Conformance violations met the --fail-on severity threshold

CI/Automation note:
  Exit status is determined solely by JSON facts (severity field) and the
  --fail-on flag. ASCII output does not affect CI behavior.`,
	RunE: runCompareThreeWay,
}

func init() {
	combinedCmd.AddCommand(compareThreeWayCmd)

	compareThreeWayCmd.Flags().StringVar(&compareThreeWayScopeRaw, "scope", "", "Scope: <kind/name>, resource:<kind/name>, namespace/<ns>, or cluster")
	compareThreeWayCmd.Flags().StringVar(&compareThreeWayView, "view", "", "View UUID or View Explorer URL; mutually exclusive with --scope (requires connected mode)")
	compareThreeWayCmd.Flags().StringVarP(&combinedNamespace, "namespace", "n", "", "Namespace override for resource scope")
	compareThreeWayCmd.Flags().StringVar(&compareThreeWayFormat, "format", "ascii", "Output format: ascii, json, md")
	compareThreeWayCmd.Flags().BoolVar(&compareThreeWayJSON, "json", false, "Output as JSON (shorthand for --format json)")
	compareThreeWayCmd.Flags().StringVar(&compareThreeWayFailOn, "fail-on", "", "Exit non-zero if max severity >= level (info, warning)")
	compareThreeWayCmd.Flags().StringVar(&compareSourcePath, "source-path", "", "Local git checkout to back-resolve gitSource.file:line for each field mismatch (stage B; raw YAML only)")
	compareThreeWayCmd.Flags().StringVar(&compareThreeWayDryFrom, "dry-from", "", "Standalone DRY source: a rendered YAML file or directory (e.g. a git checkout) used as the intended (DRY) state instead of ConfigHub. Compares DRY vs LIVE with no connected mode (#479).")
}

func runCompareThreeWay(cmd *cobra.Command, args []string) error {
	if compareThreeWayScopeRaw != "" && compareThreeWayView != "" {
		return fmt.Errorf("--scope and --view are mutually exclusive")
	}

	// Standalone git/file-as-DRY (#479): load the DRY side from a local
	// rendered file/dir, so the three-way runs without ConfigHub. Loaded once
	// here (not per resource) and consumed by buildCompareResourceResult.
	if dryFrom := strings.TrimSpace(compareThreeWayDryFrom); dryFrom != "" {
		if compareThreeWayView != "" {
			return fmt.Errorf("--dry-from and --view are mutually exclusive (--view requires connected mode)")
		}
		summaries, err := loadCompareDryFromPathFn(dryFrom)
		if err != nil {
			return fmt.Errorf("load --dry-from %s: %w", dryFrom, err)
		}
		compareThreeWayDrySummaries = summaries
		defer func() { compareThreeWayDrySummaries = nil }()
	}

	var scope threeWayScope
	if compareThreeWayView != "" {
		if hub.QuickMode() != hub.Connected {
			return fmt.Errorf("--view requires ConfigHub authentication (run `cub auth login` or set CONFIGHUB_API_KEY)")
		}
		scope = threeWayScope{ScopeType: threeWayScopeView, ScopeValue: compareThreeWayView}
	} else {
		var err error
		scope, err = parseThreeWayScope(compareThreeWayScopeRaw)
		if err != nil {
			return err
		}
	}

	// Validate --fail-on if provided
	if compareThreeWayFailOn != "" {
		if !isValidConformanceSeverity(compareThreeWayFailOn) {
			fmt.Fprintf(os.Stderr, "Error: invalid --fail-on value %q (valid: info, warning)\n", compareThreeWayFailOn)
			os.Exit(ConformanceExitError)
		}
	}

	format := strings.ToLower(strings.TrimSpace(compareThreeWayFormat))
	if format == "" {
		format = "ascii"
	}
	if compareThreeWayJSON {
		format = "json"
	}
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json, md)", compareThreeWayFormat)
	}

	report, err := buildThreeWayReport(cmd.Context(), scope, compareThreeWayFailOn)
	if err != nil {
		return err
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	case "md":
		fmt.Print(renderThreeWayMarkdown(report))
	default:
		fmt.Print(renderThreeWayASCII(report))
	}

	// Check exit condition based on --fail-on (JSON facts only)
	exitCode := computeConformanceExitCode(report, compareThreeWayFailOn)
	if exitCode != ConformanceExitOK {
		os.Exit(exitCode)
	}

	return nil
}

func parseThreeWayScope(raw string) (threeWayScope, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return threeWayScope{}, fmt.Errorf("one of --scope or --view is required")
	}

	lower := strings.ToLower(value)
	switch {
	case lower == "cluster":
		return threeWayScope{ScopeType: threeWayScopeCluster, ScopeValue: "cluster"}, nil
	case strings.HasPrefix(lower, "namespace/"):
		ns := strings.TrimSpace(value[len("namespace/"):])
		if ns == "" {
			return threeWayScope{}, fmt.Errorf("invalid namespace scope %q", raw)
		}
		return threeWayScope{ScopeType: threeWayScopeNamespace, ScopeValue: ns}, nil
	case strings.HasPrefix(lower, "resource:"):
		resource := strings.TrimSpace(value[len("resource:"):])
		if _, _, err := parseResourceArg(resource); err != nil {
			return threeWayScope{}, err
		}
		return threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: resource}, nil
	case strings.HasPrefix(lower, "resource/"):
		resource := strings.TrimSpace(value[len("resource/"):])
		if _, _, err := parseResourceArg(resource); err != nil {
			return threeWayScope{}, err
		}
		return threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: resource}, nil
	case strings.HasPrefix(lower, "unit/") || strings.HasPrefix(lower, "app/"):
		return threeWayScope{}, fmt.Errorf("scope %q is not supported yet (supported: resource, namespace, cluster)", raw)
	default:
		if _, _, err := parseResourceArg(value); err != nil {
			return threeWayScope{}, fmt.Errorf("invalid --scope %q (examples: deploy/api, namespace/prod, cluster)", raw)
		}
		return threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: value}, nil
	}
}

func buildThreeWayReport(ctx context.Context, scope threeWayScope, failOnThreshold string) (threeWayReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	targets, err := collectThreeWayTargets(ctx, scope)
	if err != nil {
		return threeWayReport{}, err
	}

	entries := make([]threeWayResourceEntry, 0, len(targets))
	summary := threeWaySummary{
		SeverityCounts: map[string]int{},
		CauseBuckets:   map[string]int{},
	}

	// Track pattern counts for agreement summary
	patternCounts := make(map[ThreeWayPattern]int)
	sources := SourceCoverage{Total: len(targets)}

	for _, target := range targets {
		result, err := buildThreeWayResourceResultFn(ctx, target.ResourceArg, target.Namespace)
		if err != nil {
			return threeWayReport{}, err
		}
		if strings.TrimSpace(result.Resource) == "" {
			result.Resource = target.ResourceArg
		}
		if strings.TrimSpace(result.Namespace) == "" {
			result.Namespace = target.Namespace
		}

		severity, causes := classifyThreeWayResult(result)

		// Classify pattern for agreement summary
		pattern := classifyResourcePattern(result)
		patternCounts[pattern]++

		// Track source coverage
		if result.Dry != nil || result.Wet != nil {
			sources.ConfigHub++
		}
		if result.Connected {
			sources.Deployer++ // Connected implies deployer evidence available
		}
		sources.Cluster++ // We always have LIVE if we're checking the resource

		entry := threeWayResourceEntry{
			Result:   result,
			Severity: severity,
			Causes:   causes,
			Pattern:  pattern,
		}
		if decision, ok := fetchThreeWayRolloutDecision(ctx, target); ok {
			entry.CurrentChange = decision
		}
		entries = append(entries, entry)

		summary.TotalResources++
		if result.Connected {
			summary.ConnectedResources++
		}
		if result.Mode == "dry-wet-live" {
			summary.DryWetLiveResources++
		}
		if len(result.Mismatches) > 0 {
			summary.MismatchedResources++
		}
		summary.SeverityCounts[severity]++
		for _, cause := range causes {
			summary.CauseBuckets[cause]++
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		left := strings.ToLower(entries[i].Result.Namespace + "/" + entries[i].Result.Resource)
		right := strings.ToLower(entries[j].Result.Namespace + "/" + entries[j].Result.Resource)
		return left < right
	})

	// Compute conformance result
	// Violations count resources with non-ok severity (warning or info issues)
	violations := summary.SeverityCounts["warning"] + summary.SeverityCounts["info"]
	maxSeverity := getMaxConformanceSeverity(summary.SeverityCounts)

	// Pass is determined by the threshold policy:
	// - No threshold: pure reporting mode, always pass=true
	// - With threshold: pass=true only if max severity doesn't meet threshold
	pass := true
	if failOnThreshold != "" && conformanceSeverityMeetsThreshold(maxSeverity, failOnThreshold) {
		pass = false
	}

	summary.Conformance = ConformanceResult{
		Pass:        pass,
		Threshold:   failOnThreshold,
		MaxSeverity: maxSeverity,
		Violations:  violations,
	}

	// Derive agreement summary from patterns
	summary.Agreement = deriveAgreementSummary(patternCounts, sources)

	report := threeWayReport{
		Scope:     scope.String(),
		Summary:   summary,
		Resources: entries,
	}
	report.ConfigHubURL, report.ConfigHubRevisionsURL, report.NextSteps = buildThreeWayNavigation(report)
	return report, nil
}

func fetchThreeWayRolloutDecision(ctx context.Context, target threeWayTarget) (*agent.RolloutDecision, bool) {
	kind, name, err := parseResourceArg(target.ResourceArg)
	if err != nil {
		return nil, false
	}
	namespace := strings.TrimSpace(target.Namespace)
	if namespace == "" {
		namespace = combinedNamespace
	}
	if namespace == "" {
		namespace = "default"
	}
	return fetchRolloutDecision(ctx, namespace, kind, name)
}

func buildThreeWayNavigation(report threeWayReport) (string, string, []StructuredHint) {
	entry := selectThreeWayTrustEntry(report.Resources)
	reportURL := compareResourceConfigHubURL(entry.Result)
	revisionsURL := compareResourceRevisionsURL(entry.Result)
	revisionHint := compareThreeWayRevisionHint(entry.Result, revisionsURL, report.Summary.Agreement.State)

	hints := make([]Hint, 0, 3)
	repeatCommand := compareThreeWayRepeatCommand(report.Scope, entry)
	explainCommand := compareThreeWayExplainCommand(entry)
	unitCommand := compareThreeWayUnitGetCommand(entry.Result)

	switch report.Summary.Agreement.State {
	case StateAgreed:
		if revisionHint != nil {
			hints = append(hints, *revisionHint)
		} else if revisionsURL != "" {
			hints = append(hints, Hint{
				Rationale:    "Open revision history to review the governed audit trail and compare revisions before sign-off.",
				ConfigHubURL: revisionsURL,
				Priority:     hintPriorityHigh,
				ActionType:   ActionHumanDecision,
			})
		}
		if reportURL != "" {
			hints = append(hints, Hint{
				Rationale:    "Agreement is proven for this scope; open the governed unit to review the audit trail before sign-off.",
				ConfigHubURL: reportURL,
				Priority:     hintPriorityHigh,
				ActionType:   ActionHumanDecision,
			})
		}
		if unitCommand != "" {
			hints = append(hints, Hint{
				Command:    unitCommand,
				Rationale:  "Inspect the linked ConfigHub unit for exact revision and live-state facts behind this agreement.",
				Priority:   hintPriorityNormal,
				ActionType: ActionReadOnly,
			})
		}
	case StateConverging:
		if repeatCommand != "" {
			hints = append(hints, Hint{
				Command:    repeatCommand,
				Rationale:  "Re-run the three-way comparison after controller convergence to confirm this scope has settled.",
				Priority:   hintPriorityHigh,
				ActionType: ActionWaiting,
			})
		}
		if revisionHint != nil {
			hints = append(hints, *revisionHint)
		} else if revisionsURL != "" {
			hints = append(hints, Hint{
				Rationale:    "Open revision history while the controller is still converging to review the pending governed path.",
				ConfigHubURL: revisionsURL,
				Priority:     hintPriorityNormal,
				ActionType:   ActionReadOnly,
			})
		}
		if unitCommand != "" || reportURL != "" {
			hints = append(hints, Hint{
				Command:      unitCommand,
				Rationale:    "Inspect the linked ConfigHub unit while the controller is still converging.",
				ConfigHubURL: reportURL,
				Priority:     hintPriorityNormal,
				ActionType:   ActionReadOnly,
			})
		}
	case StateDiverged:
		if revisionHint != nil {
			hints = append(hints, *revisionHint)
		}
		if unitCommand != "" || reportURL != "" {
			hints = append(hints, Hint{
				Command:      unitCommand,
				Rationale:    "Inspect the linked ConfigHub unit to see which governed object sits behind the divergence.",
				ConfigHubURL: reportURL,
				Priority:     hintPriorityHigh,
				ActionType:   ActionReadOnly,
			})
		}
		if explainCommand != "" {
			hints = append(hints, Hint{
				Command:    explainCommand,
				Rationale:  "Explain the representative resource to separate controller drift from governed-intent disagreement.",
				Priority:   hintPriorityNormal,
				ActionType: ActionReadOnly,
			})
		}
	case StatePartial:
		if revisionHint != nil {
			hints = append(hints, *revisionHint)
		}
		if explainCommand != "" {
			hints = append(hints, Hint{
				Command:    explainCommand,
				Rationale:  "Explain the representative resource to understand what evidence is still missing for a full agreement claim.",
				Priority:   hintPriorityHigh,
				ActionType: ActionReadOnly,
			})
		}
		if unitCommand != "" || reportURL != "" {
			hints = append(hints, Hint{
				Command:      unitCommand,
				Rationale:    "Open the first useful governed unit to anchor the partial view in exact ConfigHub context.",
				ConfigHubURL: reportURL,
				Priority:     hintPriorityNormal,
				ActionType:   ActionReadOnly,
			})
		}
	default:
		if revisionHint != nil {
			hints = append(hints, *revisionHint)
		}
		if repeatCommand != "" {
			hints = append(hints, Hint{
				Command:    repeatCommand,
				Rationale:  "Re-run the three-way comparison to refresh convergence facts for this scope.",
				Priority:   hintPriorityNormal,
				ActionType: ActionReadOnly,
			})
		}
	}

	sortHints(hints)
	if len(hints) > 3 {
		hints = hints[:3]
	}
	return reportURL, revisionsURL, HintsToStructured(hints)
}

func compareResourceRevisionState(result compareResourceResult) mcpUnitRevisionState {
	state := mcpUnitRevisionState{}
	for _, side := range []*compareSideSummary{&result.Live, result.Wet, result.Dry} {
		if side == nil {
			continue
		}
		if !state.HasHeadRevision && side.HeadRevisionNum > 0 {
			state.HeadRevision = side.HeadRevisionNum
			state.HasHeadRevision = true
		}
		if !state.HasLiveRevision && side.LiveRevisionNum > 0 {
			state.LiveRevision = side.LiveRevisionNum
			state.HasLiveRevision = true
		}
		if !state.HasLastApplied && side.LastAppliedRevisionNum > 0 {
			state.LastAppliedRevision = side.LastAppliedRevisionNum
			state.HasLastApplied = true
		}
		if state.HasHeadRevision && state.HasLiveRevision && state.HasLastApplied {
			break
		}
	}
	return state
}

func compareThreeWayRevisionHint(result compareResourceResult, revisionsURL string, state AgreementState) *Hint {
	revisionState := compareResourceRevisionState(result)
	hint := mcpUnitRevisionHint(revisionState, revisionsURL, false)
	if hint == nil {
		return nil
	}
	cloned := *hint
	if state == StateAgreed && revisionState.HasHeadRevision && revisionState.HasLiveRevision && revisionState.HeadRevision == revisionState.LiveRevision {
		cloned.Priority = hintPriorityHigh
		cloned.ActionType = ActionHumanDecision
	}
	return &cloned
}

func selectThreeWayTrustEntry(entries []threeWayResourceEntry) threeWayResourceEntry {
	bestIndex := -1
	bestScore := -1
	for i, entry := range entries {
		score := 0
		if compareResourceConfigHubURL(entry.Result) != "" {
			score += 100
		}
		if compareThreeWayUnitGetCommand(entry.Result) != "" {
			score += 60
		}
		if len(entry.Result.Mismatches) > 0 {
			score += 30
		}
		if entry.Result.Connected {
			score += 20
		}
		switch entry.Severity {
		case "warning":
			score += 10
		case "info":
			score += 5
		}
		if bestIndex == -1 || score > bestScore {
			bestIndex = i
			bestScore = score
		}
	}
	if bestIndex == -1 {
		return threeWayResourceEntry{}
	}
	return entries[bestIndex]
}

func compareThreeWayRepeatCommand(scope string, entry threeWayResourceEntry) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return ""
	}
	args := []string{"cub-scout", "compare", "three-way", "--scope", scope, "--format", "json"}
	if scope != "cluster" && !strings.HasPrefix(strings.ToLower(scope), "namespace/") {
		ns := strings.TrimSpace(entry.Result.Namespace)
		if ns != "" {
			args = append(args, "-n", ns)
		}
	}
	return strings.Join(args, " ")
}

func compareThreeWayExplainCommand(entry threeWayResourceEntry) string {
	resource := strings.TrimSpace(entry.Result.Resource)
	if resource == "" {
		return ""
	}
	args := []string{"cub-scout", "explain", resource}
	if ns := strings.TrimSpace(entry.Result.Namespace); ns != "" {
		args = append(args, "-n", ns)
	}
	args = append(args, "--format", "json")
	return strings.Join(args, " ")
}

func compareThreeWayUnitGetCommand(result compareResourceResult) string {
	unitSlug, _, spaceName, _ := compareResourceUnitIdentity(result)
	if unitSlug == "" {
		return ""
	}
	args := []string{"cub", "unit", "get", unitSlug, "--json"}
	if spaceName != "" {
		args = append(args, "--space", spaceName)
	}
	return strings.Join(args, " ")
}

func compareResourceConfigHubURL(result compareResourceResult) string {
	_, unitID, _, spaceID := compareResourceUnitIdentity(result)
	return configHubUnitDetailURL(spaceID, unitID)
}

func compareResourceRevisionsURL(result compareResourceResult) string {
	_, unitID, _, spaceID := compareResourceUnitIdentity(result)
	return configHubUnitRevisionsURL(spaceID, unitID)
}

func compareResourceUnitIdentity(result compareResourceResult) (unitSlug, unitID, spaceName, spaceID string) {
	for _, side := range []*compareSideSummary{&result.Live, result.Wet, result.Dry} {
		if side == nil {
			continue
		}
		if unitSlug == "" {
			unitSlug = strings.TrimSpace(side.UnitSlug)
		}
		if unitID == "" {
			unitID = strings.TrimSpace(side.UnitID)
		}
		if spaceName == "" {
			spaceName = strings.TrimSpace(side.SpaceName)
		}
		if spaceID == "" {
			spaceID = strings.TrimSpace(side.SpaceID)
		}
		if unitSlug != "" && unitID != "" && spaceName != "" && spaceID != "" {
			break
		}
	}
	return unitSlug, unitID, spaceName, spaceID
}

// classifyResourcePattern determines the three-way pattern for a resource.
// This reuses the logic from classifyThreeWayPattern but without sync/health status.
func classifyResourcePattern(result compareResourceResult) ThreeWayPattern {
	// Not connected AND no local DRY source = disconnected. A standalone
	// --dry-from run (#479) is not connected but carries DRY evidence, so it
	// must not be labeled disconnected.
	if !result.Connected && result.Dry == nil {
		return PatternDisconnected
	}

	// No DRY/WET = unlinked
	if result.Dry == nil && result.Wet == nil {
		return PatternUnlinked
	}

	// No mismatches = agreed
	if len(result.Mismatches) == 0 {
		return PatternAgreed
	}

	// Check mismatch patterns
	hasWetLive := hasWetLiveMismatch(result.Mismatches)
	hasDryWet := hasDryWetMismatch(result.Mismatches)

	if hasWetLive {
		// ConfigHub differs from cluster - could be change-in-progress or sync-stale
		// Without sync status, we conservatively classify as change-in-progress
		// since that's the common case
		return PatternChangeInProgress
	}

	if hasDryWet {
		// DRY differs from WET - rollout pending
		return PatternRolloutPending
	}

	// Multiple mismatches across layers
	return PatternMultiChange
}

func collectThreeWayTargets(ctx context.Context, scope threeWayScope) ([]threeWayTarget, error) {
	switch scope.ScopeType {
	case threeWayScopeResource:
		kindRaw, name, err := parseResourceArg(scope.ScopeValue)
		if err != nil {
			return nil, err
		}
		ns := strings.TrimSpace(combinedNamespace)
		return []threeWayTarget{{
			ResourceArg: normalizeKind(kindRaw) + "/" + name,
			Namespace:   ns,
		}}, nil
	case threeWayScopeNamespace:
		ns := strings.TrimSpace(scope.ScopeValue)
		workloads, err := discoverThreeWayWorkloadsFn(ns)
		if err != nil {
			return nil, err
		}
		return workloadsToThreeWayTargets(workloads, ns), nil
	case threeWayScopeCluster:
		namespaces, err := discoverThreeWayNamespacesFn()
		if err != nil {
			return nil, err
		}
		sort.Strings(namespaces)
		targets := make([]threeWayTarget, 0)
		for _, ns := range namespaces {
			workloads, err := discoverThreeWayWorkloadsFn(ns)
			if err != nil {
				return nil, err
			}
			targets = append(targets, workloadsToThreeWayTargets(workloads, ns)...)
		}
		return targets, nil
	case threeWayScopeView:
		return collectThreeWayTargetsForView(ctx, scope.ScopeValue)
	default:
		return nil, fmt.Errorf("unsupported scope type %q", scope.ScopeType)
	}
}

// collectThreeWayTargetsForView resolves a View reference to a set of
// threeWayTargets. The resolution chain is:
//
//  1. Parse the view ref (UUID or View Explorer URL).
//  2. Fetch the View via `cub view get` to extract Filter.Where.
//  3. Run `cub unit list --where '<clause>'` to get matching unit slugs.
//  4. Discover all cluster namespaces, then filter workloads whose
//     UnitSlug matches one of the slugs from step 3.
//
// All subprocess calls go through viewCubRunner so tests can inject
// fake JSON without a live ConfigHub or cluster.
func collectThreeWayTargetsForView(ctx context.Context, viewRef string) ([]threeWayTarget, error) {
	ref, err := agent.ParseViewRef(viewRef)
	if err != nil {
		return nil, fmt.Errorf("parse view reference: %w", err)
	}

	viewData, err := fetchView(ctx, ref.UUID, "*")
	if err != nil {
		return nil, fmt.Errorf("fetch view %s: %w", ref.UUID, err)
	}

	whereClause, err := extractWhereClause(viewData)
	if err != nil {
		return nil, fmt.Errorf("extract filter from view: %w", err)
	}
	if whereClause == "" {
		return nil, fmt.Errorf("view %s has no Where filter — cannot scope comparison", ref.UUID)
	}

	unitSlugs, err := listUnitSlugsForFilter(ctx, whereClause, "*")
	if err != nil {
		return nil, fmt.Errorf("list units for view filter: %w", err)
	}
	if len(unitSlugs) == 0 {
		return nil, fmt.Errorf("view filter matched no units (check the filter clause or space scope)")
	}

	slugSet := make(map[string]struct{}, len(unitSlugs))
	for _, s := range unitSlugs {
		slugSet[s] = struct{}{}
	}

	namespaces, err := discoverThreeWayNamespacesFn()
	if err != nil {
		return nil, fmt.Errorf("discover namespaces: %w", err)
	}
	sort.Strings(namespaces)

	var targets []threeWayTarget
	for _, ns := range namespaces {
		workloads, err := discoverThreeWayWorkloadsFn(ns)
		if err != nil {
			return nil, err
		}
		for _, w := range workloads {
			if _, ok := slugSet[w.UnitSlug]; !ok {
				continue
			}
			kind := normalizeKind(w.Kind)
			name := strings.TrimSpace(w.Name)
			if kind == "" || name == "" {
				continue
			}
			wsNs := strings.TrimSpace(w.Namespace)
			if wsNs == "" {
				wsNs = ns
			}
			targets = append(targets, threeWayTarget{
				ResourceArg: kind + "/" + name,
				Namespace:   wsNs,
			})
		}
	}
	return targets, nil
}

func workloadsToThreeWayTargets(workloads []WorkloadInfo, namespace string) []threeWayTarget {
	targets := make([]threeWayTarget, 0, len(workloads))
	for _, workload := range workloads {
		kind := normalizeKind(workload.Kind)
		name := strings.TrimSpace(workload.Name)
		if kind == "" || name == "" {
			continue
		}
		ns := strings.TrimSpace(workload.Namespace)
		if ns == "" {
			ns = strings.TrimSpace(namespace)
		}
		targets = append(targets, threeWayTarget{
			ResourceArg: kind + "/" + name,
			Namespace:   ns,
		})
	}

	sort.Slice(targets, func(i, j int) bool {
		left := strings.ToLower(targets[i].Namespace + "/" + targets[i].ResourceArg)
		right := strings.ToLower(targets[j].Namespace + "/" + targets[j].ResourceArg)
		return left < right
	})
	return targets
}

func classifyThreeWayResult(result compareResourceResult) (string, []string) {
	causes := make(map[string]struct{})
	if len(result.Mismatches) > 0 {
		causes["state-mismatch"] = struct{}{}
	}

	for _, note := range result.Notes {
		lower := strings.ToLower(note)
		switch {
		case strings.Contains(lower, "connect to confighub"):
			causes["disconnected"] = struct{}{}
		case strings.Contains(lower, "not linked"):
			causes["unlinked"] = struct{}{}
		case strings.Contains(lower, "snapshot unavailable"):
			causes["missing-snapshots"] = struct{}{}
		case strings.Contains(lower, "lookup failed"):
			causes["lookup-failed"] = struct{}{}
		}
	}

	severity := "ok"
	if len(result.Mismatches) > 0 {
		severity = "warning"
	} else if !result.Connected || len(result.Notes) > 0 {
		severity = "info"
	}

	out := make([]string, 0, len(causes))
	for cause := range causes {
		out = append(out, cause)
	}
	sort.Strings(out)
	return severity, out
}

func renderThreeWayASCII(report threeWayReport) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Three-Way Compare: %s\n", report.Scope))
	b.WriteString(fmt.Sprintf("Resources: %d  Connected: %d  DRY/WET/LIVE: %d  Mismatched: %d\n",
		report.Summary.TotalResources,
		report.Summary.ConnectedResources,
		report.Summary.DryWetLiveResources,
		report.Summary.MismatchedResources,
	))

	// Show agreement summary - the primary convergence indicator
	agreement := report.Summary.Agreement
	agreementIcon := stateToIcon(agreement.State)
	agreementLabel := stateToLabel(agreement.State)
	switch agreement.State {
	case StateAgreed:
		b.WriteString(fmt.Sprintf("Agreement: %s %s - %s\n", Green(agreementIcon), Green(agreementLabel), agreement.Summary))
	case StateConverging:
		b.WriteString(fmt.Sprintf("Agreement: %s %s - %s\n", Yellow(agreementIcon), Yellow(agreementLabel), agreement.Summary))
	case StateDiverged:
		b.WriteString(fmt.Sprintf("Agreement: %s %s - %s\n", Red(agreementIcon), Red(agreementLabel), agreement.Summary))
	case StatePartial:
		b.WriteString(fmt.Sprintf("Agreement: %s %s - %s\n", Yellow(agreementIcon), Yellow(agreementLabel), agreement.Summary))
	default:
		b.WriteString(fmt.Sprintf("Agreement: %s %s - %s\n", agreementIcon, agreementLabel, agreement.Summary))
	}
	for _, reason := range agreement.Reasons {
		b.WriteString(fmt.Sprintf("  -> %s\n", reason))
	}

	// Show conformance status
	// Only show PASS/FAIL verdict when a threshold is set; otherwise show issue count
	if report.Summary.Conformance.Threshold != "" {
		conformanceStatus := Green("PASS")
		if !report.Summary.Conformance.Pass {
			conformanceStatus = Red("FAIL")
		}
		b.WriteString(fmt.Sprintf("Conformance: %s (threshold: %s, max: %s, %d issues)\n",
			conformanceStatus,
			report.Summary.Conformance.Threshold,
			report.Summary.Conformance.MaxSeverity,
			report.Summary.Conformance.Violations,
		))
	} else {
		// Pure reporting mode - no verdict, just facts
		b.WriteString(fmt.Sprintf("Issues: %d (max severity: %s)\n",
			report.Summary.Conformance.Violations,
			report.Summary.Conformance.MaxSeverity,
		))
	}

	if len(report.Resources) == 0 {
		b.WriteString("No resources matched this scope\n")
		return b.String()
	}

	for _, entry := range report.Resources {
		b.WriteString(fmt.Sprintf("\n- %s (%s) severity=%s mismatches=%d\n",
			entry.Result.Resource,
			entry.Result.Namespace,
			entry.Severity,
			len(entry.Result.Mismatches),
		))
		if attr := entry.Result.Live.Attribution; attr != nil && attr.Cause != "" && len(entry.Result.Mismatches) > 0 {
			driftLine := fmt.Sprintf("  drift-cause: %s", attr.Cause)
			if attr.ManagerHint != "" {
				driftLine += fmt.Sprintf(" (manager: %s)", attr.ManagerHint)
			}
			b.WriteString(driftLine + "\n")
		}
		if len(entry.Causes) > 0 {
			b.WriteString("  causes: " + strings.Join(entry.Causes, ", ") + "\n")
		}
		if entry.CurrentChange != nil {
			b.WriteString("  current-change: " + formatRolloutDecisionLine(entry.CurrentChange) + "\n")
		}
		for _, note := range entry.Result.Notes {
			b.WriteString("  note: " + note + "\n")
		}
	}
	return b.String()
}

func renderThreeWayMarkdown(report threeWayReport) string {
	var b strings.Builder
	b.WriteString("# Three-Way Compare\n\n")
	b.WriteString(fmt.Sprintf("- Scope: `%s`\n", report.Scope))
	b.WriteString(fmt.Sprintf("- Resources: `%d`\n", report.Summary.TotalResources))
	b.WriteString(fmt.Sprintf("- Connected: `%d`\n", report.Summary.ConnectedResources))
	b.WriteString(fmt.Sprintf("- DRY/WET/LIVE: `%d`\n", report.Summary.DryWetLiveResources))
	b.WriteString(fmt.Sprintf("- Mismatched: `%d`\n", report.Summary.MismatchedResources))

	// Show agreement summary - the primary convergence indicator
	agreement := report.Summary.Agreement
	agreementLabel := stateToLabel(agreement.State)
	b.WriteString(fmt.Sprintf("- Agreement: **%s** - %s\n", agreementLabel, agreement.Summary))
	if len(agreement.Reasons) > 0 {
		for _, reason := range agreement.Reasons {
			b.WriteString(fmt.Sprintf("  - %s\n", reason))
		}
	}

	// Show conformance status
	// Only show PASS/FAIL verdict when a threshold is set; otherwise show issue count
	if report.Summary.Conformance.Threshold != "" {
		conformanceStatus := "PASS"
		if !report.Summary.Conformance.Pass {
			conformanceStatus = "FAIL"
		}
		b.WriteString(fmt.Sprintf("- Conformance: **%s** (threshold: `%s`, max: `%s`, %d issues)\n\n",
			conformanceStatus,
			report.Summary.Conformance.Threshold,
			report.Summary.Conformance.MaxSeverity,
			report.Summary.Conformance.Violations,
		))
	} else {
		// Pure reporting mode - no verdict, just facts
		b.WriteString(fmt.Sprintf("- Issues: `%d` (max severity: `%s`)\n\n",
			report.Summary.Conformance.Violations,
			report.Summary.Conformance.MaxSeverity,
		))
	}

	if len(report.Resources) == 0 {
		b.WriteString("No resources matched this scope.\n")
		return b.String()
	}

	b.WriteString("| Resource | Namespace | Mode | Severity | Mismatches | Current Change | Causes |\n")
	b.WriteString("|---|---|---|---|---:|---|---|\n")
	for _, entry := range report.Resources {
		causes := strings.Join(entry.Causes, ",")
		if causes == "" {
			causes = "-"
		}
		currentChange := "-"
		if entry.CurrentChange != nil {
			currentChange = formatRolloutDecisionLine(entry.CurrentChange)
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s | %s |\n",
			historyEscapeMarkdown(entry.Result.Resource),
			historyEscapeMarkdown(entry.Result.Namespace),
			historyEscapeMarkdown(entry.Result.Mode),
			historyEscapeMarkdown(entry.Severity),
			len(entry.Result.Mismatches),
			historyEscapeMarkdown(currentChange),
			historyEscapeMarkdown(causes),
		))
	}
	return b.String()
}

func (scope threeWayScope) String() string {
	switch scope.ScopeType {
	case threeWayScopeResource:
		return scope.ScopeValue
	case threeWayScopeNamespace:
		return "namespace/" + scope.ScopeValue
	case threeWayScopeView:
		// Prefer "view/<uuid>" regardless of whether the original input was a
		// UUID or a URL, so output consumers get a stable, compact identifier.
		ref, err := agent.ParseViewRef(scope.ScopeValue)
		if err != nil {
			return "view/" + scope.ScopeValue
		}
		return "view/" + ref.UUID
	default:
		return scope.ScopeValue
	}
}

// isValidConformanceSeverity checks if a severity level string is valid for conformance.
func isValidConformanceSeverity(level string) bool {
	switch level {
	case "info", "warning":
		return true
	}
	return false
}

// computeConformanceExitCode determines the exit code based on severity counts and --fail-on.
// This uses JSON facts (severity field) only - ASCII output does not affect this.
// Contract: Leak Test compliance - exit behavior depends only on structural facts.
func computeConformanceExitCode(report threeWayReport, failOn string) int {
	// No --fail-on specified = pure reporting mode, always exit 0
	if failOn == "" {
		return ConformanceExitOK
	}

	// Get max severity from severity counts
	maxSeverity := getMaxConformanceSeverity(report.Summary.SeverityCounts)

	// Compare max severity against threshold
	if conformanceSeverityMeetsThreshold(maxSeverity, failOn) {
		return ConformanceExitFailure
	}

	return ConformanceExitOK
}

// getMaxConformanceSeverity returns the highest severity from severity counts.
// Severity ranking: warning > info > ok
func getMaxConformanceSeverity(counts map[string]int) string {
	if counts["warning"] > 0 {
		return "warning"
	}
	if counts["info"] > 0 {
		return "info"
	}
	return "ok"
}

// conformanceSeverityMeetsThreshold returns true if maxSeverity >= threshold.
func conformanceSeverityMeetsThreshold(maxSeverity, threshold string) bool {
	rankMap := map[string]int{
		"warning": 2,
		"info":    1,
		"ok":      0,
	}
	thresholdRank := rankMap[threshold]
	maxRank := rankMap[maxSeverity]
	return maxRank >= thresholdRank
}
