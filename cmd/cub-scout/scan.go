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
	"time"

	"github.com/spf13/cobra"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
)

var (
	scanNamespace         string
	scanJSON              bool
	scanList              bool
	scanVerbose           bool
	scanStateOnly         bool
	scanKyvernoOnly       bool
	scanTimingBombs       bool
	scanIncludeUnresolved bool
	scanDangling          bool
	scanLifecycleHazards  bool
	scanThreshold         string
	scanFile              string
	scanExplain           bool
	scanNormalizedJSON    bool
)

var scanCmd = &cobra.Command{
	Use:   "scan [flags]",
	Short: "Scan for risk issues and stuck states",
	Long: `Scan the cluster for risk issues including Kyverno violations and stuck reconciliation states.

Pattern database: 46 active scanner patterns + 4,500+ reference database
See: https://github.com/confighubai/confighub-scan

This command performs two types of scanning:
1. Kyverno PolicyReports - reads violations and maps to KPOL database
2. State scanning - detects stuck HelmReleases, Kustomizations, and Applications

Examples:
  # Full scan (Kyverno + state)
  cub-scout scan

  # Scan specific namespace
  cub-scout scan -n production

  # State scan only (stuck reconciliations)
  cub-scout scan --state

  # Kyverno scan only
  cub-scout scan --kyverno

  # Scan for timing bombs (expiring certs, quota limits)
  cub-scout scan --timing-bombs

  # Include unresolved findings from Trivy/Kyverno
  cub-scout scan --include-unresolved

  # Scan for dangling/orphan resources (HPA, Service, Ingress, NetworkPolicy, Argo ApplicationSet links)
  cub-scout scan --dangling

  # Output as JSON
  cub-scout scan --json

  # Scan a YAML file (static analysis, no cluster required)
  cub-scout scan --file manifest.yaml

  # List all KPOL policies in database
  cub-scout scan --list

The output shows:
  - Stuck HelmReleases/Kustomizations/Applications with remediation commands
  - Kyverno policy violations from PolicyReports
  - Severity (critical, warning, info) based on duration/impact
  - Risk issue identifiers where matched
`,
	RunE: runScan,
}

func init() {
	rootCmd.AddCommand(scanCmd)

	scanCmd.Flags().StringVarP(&scanNamespace, "namespace", "n", "", "Namespace to scan (default: all namespaces)")
	scanCmd.Flags().BoolVar(&scanJSON, "json", false, "Output as JSON")
	scanCmd.Flags().BoolVar(&scanList, "list", false, "List all KPOL policies in database")
	scanCmd.Flags().BoolVar(&scanVerbose, "verbose", false, "Show detailed output")
	scanCmd.Flags().BoolVar(&scanStateOnly, "state", false, "State scan only (stuck reconciliations)")
	scanCmd.Flags().BoolVar(&scanKyvernoOnly, "kyverno", false, "Kyverno scan only (PolicyReports)")
	scanCmd.Flags().BoolVar(&scanTimingBombs, "timing-bombs", false, "Scan for timing bombs (expiring certs, quota limits)")
	scanCmd.Flags().BoolVar(&scanIncludeUnresolved, "include-unresolved", false, "Include unresolved findings from Trivy/Kyverno")
	scanCmd.Flags().BoolVar(&scanDangling, "dangling", false, "Scan for dangling/orphan resources (HPA, Service, Ingress, NetworkPolicy, Argo ApplicationSet links)")
	scanCmd.Flags().BoolVar(&scanLifecycleHazards, "lifecycle-hazards", false, "Scan for GitOps lifecycle hazards (Helm hooks under ArgoCD)")
	scanCmd.Flags().StringVar(&scanThreshold, "threshold", "5m", "Duration threshold for stuck detection (e.g., 30s, 2m, 5m)")
	scanCmd.Flags().StringVar(&scanFile, "file", "", "YAML file to scan (static analysis, no cluster required)")
	scanCmd.Flags().BoolVar(&scanExplain, "explain", false, "Show explanatory content to help learn GitOps risk concepts")
	scanCmd.Flags().BoolVar(&scanNormalizedJSON, "normalized-json", false, "Output normalized findings JSON (scan.normalized.v1 schema)")
}

// CombinedScanResult is the legacy type name, preserved for compatibility
// with the CUB_SCOUT_TEST_SCAN_JSON test hook and rendering functions.
type CombinedScanResult = scan.CombinedResult

func runScan(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Check ConfigHub connection for full scan capabilities.
	// --list and --file modes work with embedded patterns; cluster scanning
	// benefits from the ConfigHub pattern database when connected.
	// Currently: embedded patterns are sufficient for all scan modes.
	// Future: when pattern DB is fully API-hosted, enforce auth for cluster scans.
	client := hub.NewClient()
	if !scanList && scanFile == "" {
		if err := client.RequireConnected(); err != nil && scanVerbose {
			fmt.Fprintf(os.Stderr, "Note: not connected to ConfigHub (%v); using embedded patterns\n", err)
		}
	}

	// Create scan provider (auto-selects cub-scan when available)
	provider := scan.SelectProvider(scan.ProviderConfig{})

	// List mode - show all KPOL policies
	if scanList {
		return listKPOLPolicies(provider)
	}

	// File mode - static analysis of YAML file (no cluster required)
	if scanFile != "" {
		return runFileScan(ctx, provider, scanFile)
	}

	// TEST HOOK: Load scan results from JSON file to bypass cluster access in tests.
	// In production this env var is never set, so real cluster scanning is always used.
	if testJSON := os.Getenv("CUB_SCOUT_TEST_SCAN_JSON"); testJSON != "" {
		return loadAndRenderScanFromJSON(testJSON)
	}

	// Build k8s config
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	// Parse threshold duration
	threshold, err := time.ParseDuration(scanThreshold)
	if err != nil {
		return fmt.Errorf("invalid threshold duration %q: %w", scanThreshold, err)
	}

	// Determine what to scan (default: both)
	runKyverno := !scanStateOnly || scanKyvernoOnly
	runState := !scanKyvernoOnly || scanStateOnly
	if scanStateOnly && scanKyvernoOnly {
		runKyverno = true
		runState = true
	}

	// Execute scan through provider
	result, err := provider.ScanCluster(ctx, scan.ClusterScanOpts{
		Config:              cfg,
		Namespace:           scanNamespace,
		RunKyverno:          runKyverno,
		RunState:            runState,
		RunTimingBombs:      scanTimingBombs,
		RunUnresolved:       scanIncludeUnresolved,
		RunDangling:         scanDangling,
		RunLifecycleHazards: scanLifecycleHazards,
		Threshold:           threshold,
	})
	if err != nil {
		return err
	}

	// Connected-mode durability: persist normalized summary artifacts for later query/trend views.
	persistConnectedScanSummary(result, scanNamespace)

	// Handle Kyverno-only mode where Kyverno is not installed
	if scanKyvernoOnly && result.Kyverno == nil && !runState {
		if scanJSON || scanNormalizedJSON {
			return outputCombinedJSON(&CombinedScanResult{
				Kyverno: &agent.ScanResult{Error: "Kyverno not installed or PolicyReport CRD not found"},
			})
		}
		fmt.Printf("\n%s⚠ Kyverno not installed%s\n", colorYellow, colorReset)
		fmt.Printf("  PolicyReport CRD not found in cluster.\n")
		fmt.Printf("  Install Kyverno: https://kyverno.io/docs/installation/\n\n")
		return nil
	}

	// Output results
	if scanNormalizedJSON {
		return outputNormalizedJSON(result)
	}
	if scanJSON {
		return outputCombinedJSON(result)
	}
	if err := outputCombinedHuman(result.Kyverno, result.State, result.TimingBombs, result.Unresolved, result.Dangling); err != nil {
		return err
	}
	if result.LifecycleHazards != nil {
		fmt.Printf("\n")
		fmt.Print(agent.RenderLifecycleHazardsASCII(result.LifecycleHazards))
	}
	return nil
}

// listKPOLPolicies lists all policies via the scan provider.
func listKPOLPolicies(provider scan.Provider) error {
	policies, err := provider.ListPolicies()
	if err != nil {
		return err
	}

	if scanJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(policies)
	}

	// Human output
	fmt.Printf("\n%s%sKYVERNO POLICY CATALOG%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%s%d policies available%s\n\n", colorDim, len(policies), colorReset)

	fmt.Printf("%-12s %-8s %-45s %s\n", "ID", "SEV", "NAME", "CATEGORY")
	fmt.Printf("%-12s %-8s %-45s %s\n", "----", "---", "----", "--------")

	for _, p := range policies {
		sevColor := severityColor(p.Severity)
		name := p.Name
		if len(name) > 43 {
			name = name[:40] + "..."
		}
		fmt.Printf("%-12s %s%-8s%s %-45s %s\n",
			p.ID, sevColor, p.Severity, colorReset, name, p.Category)
	}

	fmt.Printf("\n%sRun 'cub-scout scan' to check for violations%s\n\n", colorDim, colorReset)
	return nil
}

// outputCombinedJSON outputs the combined scan result as JSON
func outputCombinedJSON(result *CombinedScanResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// outputFinding outputs a single finding
func outputFinding(f agent.ScanFinding) {
	// ID with KPOL reference if matched
	id := f.PolicyName
	if f.PolicyID != "" {
		id = fmt.Sprintf("%s[%s]", f.PolicyName, f.PolicyID)
	}

	sevColor := severityColor(f.Severity)

	// Main line
	fmt.Printf("%s[%s]%s %s\n", sevColor, strings.ToUpper(f.Severity[:1]), colorReset, id)

	// Resource
	resource := f.Resource
	if f.Namespace != "" {
		resource = fmt.Sprintf("%s/%s", f.Namespace, f.Resource)
	}
	fmt.Printf("  %sResource:%s %s\n", colorDim, colorReset, resource)

	// Message
	if f.Message != "" {
		msg := f.Message
		if len(msg) > 70 {
			msg = msg[:67] + "..."
		}
		fmt.Printf("  %sMessage:%s %s\n", colorDim, colorReset, msg)
	}

	// Rule
	if f.Rule != "" && scanVerbose {
		fmt.Printf("  %sRule:%s %s\n", colorDim, colorReset, f.Rule)
	}
}

// severityColor returns the ANSI color for a severity level
func severityColor(severity string) string {
	switch strings.ToLower(severity) {
	case "critical", "high":
		return colorRed
	case "warning", "medium":
		return colorYellow
	default:
		return colorDim
	}
}

// outputCombinedHuman outputs Kyverno, state, timing bomb, unresolved, and dangling results in human-readable format
func outputCombinedHuman(kyvernoResult *agent.ScanResult, stateResult *agent.StateScanResult, timingBombResult *agent.TimingBombResult, unresolvedResult *agent.UnresolvedResult, danglingResult *agent.DanglingResult) error {
	fmt.Printf("\n")

	// Explanatory content when --explain is used
	if scanExplain {
		fmt.Printf("%s%sRISK SCANNING EXPLAINED%s\n", colorBold, colorWhite, colorReset)
		fmt.Printf("%s════════════════════════════════════════════════════════════════════%s\n", colorDim, colorReset)
		fmt.Printf("cub-scout scans for configuration risks that cause production incidents:\n\n")
		fmt.Printf("  %sStuck Reconciliations%s — GitOps deployers failing to sync\n", colorRed, colorReset)
		fmt.Printf("       HelmRelease, Kustomization, or Application not Ready\n")
		fmt.Printf("       Risk: Changes not deploying, drift accumulating\n\n")
		fmt.Printf("  %sKyverno Violations%s — Policy violations in cluster\n", colorYellow, colorReset)
		fmt.Printf("       Security, best practices, or compliance policies\n")
		fmt.Printf("       Risk: Insecure or non-compliant configurations\n\n")
		fmt.Printf("  %sTiming Bombs%s — Things that will break in the future\n", colorCyan, colorReset)
		fmt.Printf("       Expiring certs, quota limits approaching\n")
		fmt.Printf("       Risk: Surprise outages when time runs out\n\n")
		fmt.Printf("  %sDangling Resources%s — Resources pointing to nothing\n", colorPurple, colorReset)
		fmt.Printf("       HPA, Service, Ingress targeting deleted workloads\n")
		fmt.Printf("       Risk: Broken routing, wasted capacity\n\n")
		fmt.Printf("%sEach finding has a risk issue ID (e.g., CCVE-2025-0027) from our Risk Scorecard database.%s\n", colorDim, colorReset)
		fmt.Printf("%sSee: https://github.com/confighubai/confighub-scan%s\n", colorDim, colorReset)
		fmt.Printf("\n")
	}

	hasOutput := false

	// Output runtime failures first (direct pod-level breakages).
	if stateResult != nil && len(stateResult.RuntimeFindings) > 0 {
		hasOutput = true
		fmt.Printf("%s%sRUNTIME FAILURES%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("%sScanned at: %s%s\n\n", colorDim, stateResult.ScannedAt.Format("2006-01-02 15:04:05"), colorReset)

		for _, group := range groupRuntimeFailures(stateResult.RuntimeFindings) {
			podLabel := "pods"
			if len(group.Pods) == 1 {
				podLabel = "pod"
			}
			fmt.Printf("%s✗ %s (%d %s)%s\n", colorRed, group.FailureType, len(group.Pods), podLabel, colorReset)
			fmt.Printf("  %sAffected:%s %s\n", colorDim, colorReset, strings.Join(group.Pods, ", "))
			if group.Image != "" {
				fmt.Printf("  %sImage:%s %s\n", colorDim, colorReset, group.Image)
			}
			if group.Message != "" {
				fmt.Printf("  %sReason:%s %s\n", colorDim, colorReset, group.Message)
			}
			fmt.Printf("  %sSuggestion:%s %s\n\n", colorDim, colorReset, group.Suggestion)
		}
	}

	// Output state findings first (stuck resources are more urgent)
	if stateResult != nil && len(stateResult.Findings) > 0 {
		hasOutput = true
		fmt.Printf("%s%sSTUCK RECONCILIATION SCAN%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("%sScanned at: %s%s\n\n", colorDim, stateResult.ScannedAt.Format("2006-01-02 15:04:05"), colorReset)

		// Group by severity
		critical := []agent.StuckFinding{}
		warning := []agent.StuckFinding{}
		info := []agent.StuckFinding{}

		for _, f := range stateResult.Findings {
			switch f.Severity {
			case "critical":
				critical = append(critical, f)
			case "warning":
				warning = append(warning, f)
			default:
				info = append(info, f)
			}
		}

		// Output by severity
		if len(critical) > 0 {
			fmt.Printf("%s%sCRITICAL (%d)%s\n", colorBold, colorRed, len(critical), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range critical {
				outputStuckFinding(f)
			}
			fmt.Printf("\n")
		}

		if len(warning) > 0 {
			fmt.Printf("%s%sWARNING (%d)%s\n", colorBold, colorYellow, len(warning), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range warning {
				outputStuckFinding(f)
			}
			fmt.Printf("\n")
		}

		if len(info) > 0 && scanVerbose {
			fmt.Printf("%sINFO (%d)%s\n", colorDim, len(info), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range info {
				outputStuckFinding(f)
			}
			fmt.Printf("\n")
		}

		// State summary
		fmt.Printf("════════════════════════════════════════════════════════════════════\n")
		fmt.Printf("State Summary: %s%d HelmRelease%s, %s%d Kustomization%s, %s%d Application%s stuck\n\n",
			colorRed, stateResult.Summary.HelmReleaseStuck, colorReset,
			colorYellow, stateResult.Summary.KustomizationStuck, colorReset,
			colorCyan, stateResult.Summary.ApplicationStuck, colorReset)
	} else if stateResult != nil && len(stateResult.RuntimeFindings) > 0 {
		// Explicitly distinguish runtime failures from GitOps state issues.
		fmt.Printf("STATE ISSUES\n")
		fmt.Printf("%s%s✓ No state issues found%s\n\n", colorBold, colorGreen, colorReset)
	}

	// Output Kyverno findings
	if kyvernoResult != nil {
		if kyvernoResult.Error != "" {
			fmt.Printf("%s⚠ Kyverno: %s%s\n\n", colorYellow, kyvernoResult.Error, colorReset)
		} else if len(kyvernoResult.Findings) > 0 {
			hasOutput = true
			if stateResult != nil && len(stateResult.Findings) > 0 {
				fmt.Printf("\n") // Extra spacing between sections
			}
			fmt.Printf("%s%sKYVERNO POLICY SCAN%s\n", colorBold, colorCyan, colorReset)
			fmt.Printf("%sScanned at: %s%s\n\n", colorDim, kyvernoResult.ScannedAt.Format("2006-01-02 15:04:05"), colorReset)

			// Group by severity
			critical := []agent.ScanFinding{}
			warning := []agent.ScanFinding{}
			info := []agent.ScanFinding{}

			for _, f := range kyvernoResult.Findings {
				switch f.Severity {
				case "critical":
					critical = append(critical, f)
				case "warning":
					warning = append(warning, f)
				default:
					info = append(info, f)
				}
			}

			// Output by severity
			if len(critical) > 0 {
				fmt.Printf("%s%sCRITICAL (%d)%s\n", colorBold, colorRed, len(critical), colorReset)
				fmt.Printf("────────────────────────────────────────────────────────────────────\n")
				for _, f := range critical {
					outputFinding(f)
				}
				fmt.Printf("\n")
			}

			if len(warning) > 0 {
				fmt.Printf("%s%sWARNING (%d)%s\n", colorBold, colorYellow, len(warning), colorReset)
				fmt.Printf("────────────────────────────────────────────────────────────────────\n")
				for _, f := range warning {
					outputFinding(f)
				}
				fmt.Printf("\n")
			}

			if len(info) > 0 && scanVerbose {
				fmt.Printf("%sINFO (%d)%s\n", colorDim, len(info), colorReset)
				fmt.Printf("────────────────────────────────────────────────────────────────────\n")
				for _, f := range info {
					outputFinding(f)
				}
				fmt.Printf("\n")
			}

			// Kyverno summary
			fmt.Printf("════════════════════════════════════════════════════════════════════\n")
			fmt.Printf("Kyverno Summary: %s%d critical%s, %s%d warning%s, %d info\n\n",
				colorRed, kyvernoResult.Summary.Critical, colorReset,
				colorYellow, kyvernoResult.Summary.Warning, colorReset,
				kyvernoResult.Summary.Info)
		}
	}

	// Output timing bomb findings
	if timingBombResult != nil && len(timingBombResult.Findings) > 0 {
		hasOutput = true
		if stateResult != nil && len(stateResult.Findings) > 0 || (kyvernoResult != nil && len(kyvernoResult.Findings) > 0) {
			fmt.Printf("\n") // Extra spacing between sections
		}
		fmt.Printf("%s%sTIMING BOMB SCAN%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("%sScanned at: %s%s\n\n", colorDim, timingBombResult.ScannedAt.Format("2006-01-02 15:04:05"), colorReset)

		// Group by severity
		critical := []agent.TimingBombFinding{}
		warning := []agent.TimingBombFinding{}
		info := []agent.TimingBombFinding{}

		for _, f := range timingBombResult.Findings {
			switch f.Severity {
			case "critical":
				critical = append(critical, f)
			case "warning":
				warning = append(warning, f)
			default:
				info = append(info, f)
			}
		}

		// Output by severity
		if len(critical) > 0 {
			fmt.Printf("%s%sCRITICAL (%d)%s — Expires within 3 days\n", colorBold, colorRed, len(critical), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range critical {
				outputTimingBombFinding(f)
			}
			fmt.Printf("\n")
		}

		if len(warning) > 0 {
			fmt.Printf("%s%sWARNING (%d)%s — Expires within 14 days\n", colorBold, colorYellow, len(warning), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range warning {
				outputTimingBombFinding(f)
			}
			fmt.Printf("\n")
		}

		if len(info) > 0 && scanVerbose {
			fmt.Printf("%sINFO (%d)%s — Expires within 30 days\n", colorDim, len(info), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range info {
				outputTimingBombFinding(f)
			}
			fmt.Printf("\n")
		}

		// Timing bomb summary
		fmt.Printf("════════════════════════════════════════════════════════════════════\n")
		fmt.Printf("Timing Bombs: %s%d critical%s, %s%d warning%s, %d info\n\n",
			colorRed, timingBombResult.Summary.Critical, colorReset,
			colorYellow, timingBombResult.Summary.Warning, colorReset,
			timingBombResult.Summary.Info)
	} else if scanTimingBombs {
		// Timing bombs was requested but nothing found
		fmt.Printf("%s%sTIMING BOMB SCAN%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("%s%s✓ No expiring certificates or quotas found%s\n\n", colorBold, colorGreen, colorReset)
	}

	// Output unresolved findings
	if unresolvedResult != nil && len(unresolvedResult.Findings) > 0 {
		hasOutput = true
		fmt.Printf("\n")
		fmt.Printf("%s%sUNRESOLVED FINDINGS%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("%sScanned at: %s%s\n\n", colorDim, unresolvedResult.ScannedAt.Format("2006-01-02 15:04:05"), colorReset)

		// Group by source
		trivyFindings := []agent.UnresolvedFinding{}
		kyvernoFindings := []agent.UnresolvedFinding{}

		for _, f := range unresolvedResult.Findings {
			switch f.Source {
			case "trivy":
				trivyFindings = append(trivyFindings, f)
			case "kyverno":
				kyvernoFindings = append(kyvernoFindings, f)
			}
		}

		// Output Trivy findings
		if len(trivyFindings) > 0 {
			fmt.Printf("%sFROM TRIVY OPERATOR%s\n", colorBold, colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range trivyFindings {
				outputUnresolvedFinding(f)
			}
			fmt.Printf("\n")
		}

		// Output Kyverno findings
		if len(kyvernoFindings) > 0 {
			fmt.Printf("%sFROM KYVERNO%s\n", colorBold, colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range kyvernoFindings {
				outputUnresolvedFinding(f)
			}
			fmt.Printf("\n")
		}

		// Unresolved summary
		fmt.Printf("════════════════════════════════════════════════════════════════════\n")
		fmt.Printf("Unresolved: %s%d critical%s, %s%d high%s (Trivy: %d, Kyverno: %d)\n\n",
			colorRed, unresolvedResult.Summary.Critical, colorReset,
			colorYellow, unresolvedResult.Summary.High, colorReset,
			unresolvedResult.Summary.Trivy, unresolvedResult.Summary.Kyverno)
	} else if scanIncludeUnresolved {
		// Unresolved was requested but nothing found
		fmt.Printf("\n%s%sUNRESOLVED FINDINGS%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("%s%s✓ No unresolved Trivy/Kyverno findings%s\n\n", colorBold, colorGreen, colorReset)
	}

	// Output dangling resource findings
	if danglingResult != nil && len(danglingResult.Findings) > 0 {
		hasOutput = true
		fmt.Printf("\n")
		fmt.Printf("%s%sDANGLING RESOURCE SCAN%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("%sOrphan detection: HPA, Service, Ingress, NetworkPolicy%s\n\n", colorDim, colorReset)

		// Group by type
		hpaFindings := []agent.DanglingFinding{}
		svcFindings := []agent.DanglingFinding{}
		ingressFindings := []agent.DanglingFinding{}
		npFindings := []agent.DanglingFinding{}

		for _, f := range danglingResult.Findings {
			switch f.Kind {
			case "HorizontalPodAutoscaler":
				hpaFindings = append(hpaFindings, f)
			case "Service":
				svcFindings = append(svcFindings, f)
			case "Ingress":
				ingressFindings = append(ingressFindings, f)
			case "NetworkPolicy":
				npFindings = append(npFindings, f)
			}
		}

		// Output HPA findings
		if len(hpaFindings) > 0 {
			fmt.Printf("%s%sDANGLING HPA (%d)%s\n", colorBold, colorYellow, len(hpaFindings), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range hpaFindings {
				outputDanglingFinding(f)
			}
			fmt.Printf("\n")
		}

		// Output Service findings
		if len(svcFindings) > 0 {
			fmt.Printf("%s%sDANGLING SERVICE (%d)%s\n", colorBold, colorYellow, len(svcFindings), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range svcFindings {
				outputDanglingFinding(f)
			}
			fmt.Printf("\n")
		}

		// Output Ingress findings
		if len(ingressFindings) > 0 {
			fmt.Printf("%s%sDANGLING INGRESS (%d)%s\n", colorBold, colorYellow, len(ingressFindings), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range ingressFindings {
				outputDanglingFinding(f)
			}
			fmt.Printf("\n")
		}

		// Output NetworkPolicy findings
		if len(npFindings) > 0 {
			fmt.Printf("%s%sDANGLING NETWORKPOLICY (%d)%s\n", colorBold, colorYellow, len(npFindings), colorReset)
			fmt.Printf("────────────────────────────────────────────────────────────────────\n")
			for _, f := range npFindings {
				outputDanglingFinding(f)
			}
			fmt.Printf("\n")
		}

		// Dangling summary
		fmt.Printf("════════════════════════════════════════════════════════════════════\n")
		fmt.Printf("Dangling: %d HPA, %d Service, %d Ingress, %d NetworkPolicy\n\n",
			danglingResult.Summary.HPAs, danglingResult.Summary.Services,
			danglingResult.Summary.Ingresses, danglingResult.Summary.NetworkPolicies)
	} else if scanDangling {
		// Dangling was requested but nothing found
		fmt.Printf("\n%s%sDANGLING RESOURCE SCAN%s\n", colorBold, colorCyan, colorReset)
		fmt.Printf("%s%s✓ No dangling resources found%s\n\n", colorBold, colorGreen, colorReset)
	}

	if !hasOutput {
		fmt.Printf("%s%s✓ No issues found%s\n\n", colorBold, colorGreen, colorReset)
	}

	// Next steps when --explain is used
	if scanExplain {
		fmt.Printf("%sNEXT STEPS:%s\n", colorBold, colorReset)
		fmt.Printf("→ See all patterns:        cub-scout scan --list\n")
		fmt.Printf("→ Scan a YAML file:        cub-scout scan --file manifest.yaml\n")
		fmt.Printf("→ Trace failing resource:  cub-scout trace <kind>/<name> -n <namespace>\n")
		fmt.Printf("→ Visual guide:            docs/diagrams/risk-categories.svg\n")
		fmt.Printf("\n")
	}

	// ConfigHub hook hint
	fmt.Printf("%s🔗 Track violations in ConfigHub: cub-scout scan --confighub%s\n\n", colorDim, colorReset)

	return nil
}

type runtimeFailureGroup struct {
	FailureType string
	Pods        []string
	Image       string
	Message     string
	Suggestion  string
}

func groupRuntimeFailures(findings []agent.RuntimeFailureFinding) []runtimeFailureGroup {
	byType := map[string]*runtimeFailureGroup{}

	for _, finding := range findings {
		group, ok := byType[finding.FailureType]
		if !ok {
			group = &runtimeFailureGroup{
				FailureType: finding.FailureType,
				Suggestion:  finding.Remediation,
			}
			byType[finding.FailureType] = group
		}

		group.Pods = append(group.Pods, fmt.Sprintf("%s/%s", finding.Namespace, finding.Name))
		if group.Image == "" && finding.Image != "" {
			group.Image = finding.Image
		}
		if group.Message == "" {
			group.Message = strings.TrimSpace(finding.Message)
			if group.Message == "" {
				group.Message = strings.TrimSpace(finding.Reason)
			}
		}
		if group.Suggestion == "" && finding.Remediation != "" {
			group.Suggestion = finding.Remediation
		}
	}

	ordered := make([]runtimeFailureGroup, 0, len(byType))
	for _, group := range byType {
		sort.Strings(group.Pods)
		ordered = append(ordered, *group)
	}

	sort.Slice(ordered, func(i, j int) bool {
		return runtimeFailureOrder(ordered[i].FailureType) < runtimeFailureOrder(ordered[j].FailureType)
	})

	return ordered
}

func runtimeFailureOrder(failureType string) int {
	order := map[string]int{
		"ImagePullBackOff":     0,
		"ErrImagePull":         1,
		"CrashLoopBackOff":     2,
		"CreateContainerError": 3,
		"Pending":              4,
		"Evicted":              5,
	}
	if idx, ok := order[failureType]; ok {
		return idx
	}
	return 99
}

// outputStuckFinding outputs a single stuck finding with remediation
func outputStuckFinding(f agent.StuckFinding) {
	sevColor := severityColor(f.Severity)

	// Main line with CCVE ID
	fmt.Printf("%s[%s]%s %s/%s %s[%s]%s\n",
		sevColor, strings.ToUpper(f.Severity[:1]), colorReset,
		f.Kind, f.Name,
		colorDim, f.CCVEID, colorReset)

	// Resource location
	fmt.Printf("  %sNamespace:%s %s\n", colorDim, colorReset, f.Namespace)

	// Condition and duration
	fmt.Printf("  %sCondition:%s %s (%s)\n", colorDim, colorReset, f.Condition, f.Duration)

	// Reason
	if f.Reason != "" {
		fmt.Printf("  %sReason:%s %s\n", colorDim, colorReset, f.Reason)
	}

	// Message (truncated)
	if f.Message != "" {
		fmt.Printf("  %sMessage:%s %s\n", colorDim, colorReset, f.Message)
	}

	// Remediation (the key P1.2 feature!)
	if f.Remediation != "" {
		fmt.Printf("  %s→ Remediation:%s %s\n", colorYellow, colorReset, f.Remediation)
	}

	// Copy-paste command (the "10x faster resolution" feature)
	if f.Command != "" {
		fmt.Printf("  %sFIX:%s %s%s%s\n", colorGreen, colorReset, colorBold, f.Command, colorReset)
	}
	fmt.Printf("\n")
}

// outputTimingBombFinding outputs a single timing bomb finding with expiry info
func outputTimingBombFinding(f agent.TimingBombFinding) {
	sevColor := severityColor(f.Severity)

	// Main line with CCVE ID
	fmt.Printf("%s[%s]%s %s/%s %s[%s]%s\n",
		sevColor, strings.ToUpper(f.Severity[:1]), colorReset,
		f.Kind, f.Name,
		colorDim, f.CCVEID, colorReset)

	// Resource location
	fmt.Printf("  %sNamespace:%s %s\n", colorDim, colorReset, f.Namespace)

	// Expiry info (the key timing bomb feature!)
	fmt.Printf("  %sExpires:%s %s (%s)\n", colorDim, colorReset, f.ExpiresAt.Format("2006-01-02 15:04:05"), f.ExpiresIn)

	// Reason
	if f.Reason != "" {
		fmt.Printf("  %sReason:%s %s\n", colorDim, colorReset, f.Reason)
	}

	// Message (truncated)
	if f.Message != "" {
		msg := f.Message
		if len(msg) > 70 {
			msg = msg[:67] + "..."
		}
		fmt.Printf("  %sMessage:%s %s\n", colorDim, colorReset, msg)
	}

	// Remediation
	if f.Remediation != "" {
		fmt.Printf("  %s→ Remediation:%s %s\n", colorYellow, colorReset, f.Remediation)
	}

	// Copy-paste command
	if f.Command != "" {
		fmt.Printf("  %sFIX:%s %s%s%s\n", colorGreen, colorReset, colorBold, f.Command, colorReset)
	}
	fmt.Printf("\n")
}

// outputUnresolvedFinding outputs a single unresolved finding from Trivy/Kyverno
func outputUnresolvedFinding(f agent.UnresolvedFinding) {
	sevColor := severityColor(f.Severity)

	// Main line with CCVE ID
	fmt.Printf("%s[%s]%s %s/%s %s[%s]%s\n",
		sevColor, strings.ToUpper(f.Severity[:1]), colorReset,
		f.Kind, f.Name,
		colorDim, f.CCVEID, colorReset)

	// Resource location
	fmt.Printf("  %sNamespace:%s %s\n", colorDim, colorReset, f.Namespace)

	// Finding type and count
	fmt.Printf("  %sType:%s %s (%d findings)\n", colorDim, colorReset, f.FindingType, f.Count)

	// Message
	if f.Message != "" {
		fmt.Printf("  %sMessage:%s %s\n", colorDim, colorReset, f.Message)
	}

	// Command
	if f.Command != "" {
		fmt.Printf("  %sView:%s %s%s%s\n", colorGreen, colorReset, colorBold, f.Command, colorReset)
	}
	fmt.Printf("\n")
}

// outputDanglingFinding outputs a single dangling/orphan resource finding
func outputDanglingFinding(f agent.DanglingFinding) {
	sevColor := severityColor(f.Severity)

	// Main line with CCVE ID
	fmt.Printf("%s[%s]%s %s/%s %s[%s]%s\n",
		sevColor, strings.ToUpper(f.Severity[:1]), colorReset,
		f.Kind, f.Name,
		colorDim, f.CCVEID, colorReset)

	// Resource location
	fmt.Printf("  %sNamespace:%s %s\n", colorDim, colorReset, f.Namespace)

	// Target reference
	if f.TargetKind != "" && f.TargetName != "" {
		fmt.Printf("  %sTarget:%s %s/%s (not found)\n", colorDim, colorReset, f.TargetKind, f.TargetName)
	}

	// Message
	if f.Message != "" {
		fmt.Printf("  %sMessage:%s %s\n", colorDim, colorReset, f.Message)
	}

	// Remediation
	if f.Remediation != "" {
		fmt.Printf("  %s→ Remediation:%s %s\n", colorYellow, colorReset, f.Remediation)
	}

	// Command
	if f.Command != "" {
		fmt.Printf("  %sFIX:%s %s%s%s\n", colorGreen, colorReset, colorBold, f.Command, colorReset)
	}
	fmt.Printf("\n")
}

// runFileScan performs static analysis on a YAML file via the scan provider.
func runFileScan(ctx context.Context, provider scan.Provider, filename string) error {
	combined, err := provider.ScanFile(ctx, scan.FileScanOpts{
		Filename:            filename,
		RunLifecycleHazards: scanLifecycleHazards,
	})
	if err != nil {
		return err
	}

	// Output
	if scanNormalizedJSON {
		if err := outputNormalizedJSON(combined); err != nil {
			return err
		}
	} else if scanJSON {
		if err := outputCombinedJSON(combined); err != nil {
			return err
		}
	} else {
		if err := outputStaticScanHuman(combined.Static); err != nil {
			return err
		}
		if combined.LifecycleHazards != nil && combined.LifecycleHazards.Summary.Total > 0 {
			fmt.Print(agent.RenderLifecycleHazardsASCII(combined.LifecycleHazards))
		}
	}

	// Per cli-contract.md: exit code 1 for "Issues found or error"
	hasFindings := combined.Static != nil && (len(combined.Static.Findings) > 0 || combined.Static.Error != "")
	if combined.LifecycleHazards != nil {
		hasFindings = hasFindings || combined.LifecycleHazards.Summary.Total > 0
	}
	if hasFindings {
		os.Exit(1)
	}
	return nil
}

// outputStaticScanHuman outputs static scan results in human-readable format
func outputStaticScanHuman(result *agent.StaticScanResult) error {
	fmt.Printf("\n")

	if result.Error != "" {
		fmt.Printf("%s⚠ %s%s\n\n", colorYellow, result.Error, colorReset)
		return nil
	}

	// Header
	fmt.Printf("%s%sSTATIC FILE SCAN%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%sFile: %s%s\n", colorDim, result.File, colorReset)
	fmt.Printf("%sResources: %d%s\n\n", colorDim, result.ResourceCount, colorReset)

	if len(result.Findings) == 0 {
		fmt.Printf("%s%s✓ No misconfigurations found%s\n\n", colorBold, colorGreen, colorReset)
		return nil
	}

	// Group by severity
	critical := []agent.StaticFinding{}
	warning := []agent.StaticFinding{}
	info := []agent.StaticFinding{}

	for _, f := range result.Findings {
		switch f.Severity {
		case "critical":
			critical = append(critical, f)
		case "warning":
			warning = append(warning, f)
		default:
			info = append(info, f)
		}
	}

	// Output by severity
	if len(critical) > 0 {
		fmt.Printf("%s%sCRITICAL (%d)%s\n", colorBold, colorRed, len(critical), colorReset)
		fmt.Printf("────────────────────────────────────────────────────────────────────\n")
		for _, f := range critical {
			outputStaticFinding(f)
		}
		fmt.Printf("\n")
	}

	if len(warning) > 0 {
		fmt.Printf("%s%sWARNING (%d)%s\n", colorBold, colorYellow, len(warning), colorReset)
		fmt.Printf("────────────────────────────────────────────────────────────────────\n")
		for _, f := range warning {
			outputStaticFinding(f)
		}
		fmt.Printf("\n")
	}

	if len(info) > 0 && scanVerbose {
		fmt.Printf("%sINFO (%d)%s\n", colorDim, len(info), colorReset)
		fmt.Printf("────────────────────────────────────────────────────────────────────\n")
		for _, f := range info {
			outputStaticFinding(f)
		}
		fmt.Printf("\n")
	}

	// Summary
	fmt.Printf("════════════════════════════════════════════════════════════════════\n")
	fmt.Printf("Summary: %s%d critical%s, %s%d warning%s, %d info\n\n",
		colorRed, len(critical), colorReset,
		colorYellow, len(warning), colorReset,
		len(info))

	return nil
}

// outputStaticFinding outputs a single static analysis finding
func outputStaticFinding(f agent.StaticFinding) {
	sevColor := severityColor(f.Severity)

	// Main line with CCVE ID
	fmt.Printf("%s[%s]%s %s %s[%s]%s\n",
		sevColor, strings.ToUpper(f.Severity[:1]), colorReset,
		f.Name,
		colorDim, f.CCVEID, colorReset)

	// Resource
	resource := fmt.Sprintf("%s/%s", f.Kind, f.ResourceName)
	if f.Namespace != "" {
		resource = fmt.Sprintf("%s/%s/%s", f.Namespace, f.Kind, f.ResourceName)
	}
	fmt.Printf("  %sResource:%s %s\n", colorDim, colorReset, resource)

	// Message
	if f.Message != "" {
		msg := f.Message
		if len(msg) > 70 {
			msg = msg[:67] + "..."
		}
		fmt.Printf("  %sMessage:%s %s\n", colorDim, colorReset, msg)
	}

	// Remediation
	if f.Remediation != "" {
		fmt.Printf("  %s→ Remediation:%s %s\n", colorYellow, colorReset, f.Remediation)
	}
	fmt.Printf("\n")
}

// outputNormalizedJSON outputs the combined result in the scan.normalized.v1 schema.
func outputNormalizedJSON(result *CombinedScanResult) error {
	normalized := scan.Normalize(result)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(normalized)
}

// loadAndRenderScanFromJSON loads scan results from a JSON file and renders them.
// This is used for golden testing to bypass cluster access.
func loadAndRenderScanFromJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read scan JSON: %w", err)
	}

	var result CombinedScanResult
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("failed to parse scan JSON: %w", err)
	}

	if scanNormalizedJSON {
		return outputNormalizedJSON(&result)
	}
	if scanJSON {
		return outputCombinedJSON(&result)
	}
	return outputCombinedHuman(result.Kyverno, result.State, result.TimingBombs, result.Unresolved, result.Dangling)
}
