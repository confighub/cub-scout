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
)

type threeWayScopeType string

const (
	threeWayScopeResource  threeWayScopeType = "resource"
	threeWayScopeNamespace threeWayScopeType = "namespace"
	threeWayScopeCluster   threeWayScopeType = "cluster"
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
	Result   compareResourceResult `json:"result"`
	Severity string                `json:"severity"`
	Causes   []string              `json:"causes,omitempty"`
	Pattern  ThreeWayPattern       `json:"pattern"`
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
	Scope     string                  `json:"scope"`
	Summary   threeWaySummary         `json:"summary"`
	Resources []threeWayResourceEntry `json:"resources"`
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
	compareThreeWayFormat   string
	compareThreeWayJSON     bool
	compareThreeWayFailOn   string

	buildThreeWayResourceResultFn = buildCompareResourceResult
	discoverThreeWayNamespacesFn  = discoverNamespacesWithWorkloads
	discoverThreeWayWorkloadsFn   = discoverWorkloads
)

var compareThreeWayCmd = &cobra.Command{
	Use:   "three-way",
	Short: "Compare intent vs rendered vs observed state",
	Long: `Connected three-way comparison (DRY/WET/LIVE) for a selected scope.

Supported scopes:
  deploy/my-app            (resource)
  resource:deploy/my-app   (resource)
  namespace/prod           (namespace)
  cluster                  (all namespaces)

Examples:
  cub-scout compare three-way --scope deploy/payment-api -n prod
  cub-scout compare three-way --scope namespace/prod --format json
  cub-scout compare three-way --scope cluster --format md

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
	compareThreeWayCmd.Flags().StringVarP(&combinedNamespace, "namespace", "n", "", "Namespace override for resource scope")
	compareThreeWayCmd.Flags().StringVar(&compareThreeWayFormat, "format", "ascii", "Output format: ascii, json, md")
	compareThreeWayCmd.Flags().BoolVar(&compareThreeWayJSON, "json", false, "Output as JSON (shorthand for --format json)")
	compareThreeWayCmd.Flags().StringVar(&compareThreeWayFailOn, "fail-on", "", "Exit non-zero if max severity >= level (info, warning)")
}

func runCompareThreeWay(cmd *cobra.Command, args []string) error {
	scope, err := parseThreeWayScope(compareThreeWayScopeRaw)
	if err != nil {
		return err
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
		return threeWayScope{}, fmt.Errorf("--scope is required")
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
	targets, err := collectThreeWayTargets(scope)
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

	return threeWayReport{
		Scope:     scope.String(),
		Summary:   summary,
		Resources: entries,
	}, nil
}

// classifyResourcePattern determines the three-way pattern for a resource.
// This reuses the logic from classifyThreeWayPattern but without sync/health status.
func classifyResourcePattern(result compareResourceResult) ThreeWayPattern {
	// Not connected = disconnected
	if !result.Connected {
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

func collectThreeWayTargets(scope threeWayScope) ([]threeWayTarget, error) {
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
	default:
		return nil, fmt.Errorf("unsupported scope type %q", scope.ScopeType)
	}
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
		if len(entry.Causes) > 0 {
			b.WriteString("  causes: " + strings.Join(entry.Causes, ", ") + "\n")
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

	b.WriteString("| Resource | Namespace | Mode | Severity | Mismatches | Causes |\n")
	b.WriteString("|---|---|---|---|---:|---|\n")
	for _, entry := range report.Resources {
		causes := strings.Join(entry.Causes, ",")
		if causes == "" {
			causes = "-"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s |\n",
			historyEscapeMarkdown(entry.Result.Resource),
			historyEscapeMarkdown(entry.Result.Namespace),
			historyEscapeMarkdown(entry.Result.Mode),
			historyEscapeMarkdown(entry.Severity),
			len(entry.Result.Mismatches),
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
