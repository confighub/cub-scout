// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

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
	scanThreshold         string
	scanFile              string
)

var scanCmd = &cobra.Command{
	Use:   "scan [flags]",
	Short: "Scan for CCVEs and stuck states",
	Long: `Scan the cluster for CCVEs including Kyverno violations and stuck reconciliation states.

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

  # Scan for dangling/orphan resources (HPA, Service, Ingress, NetworkPolicy)
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
  - CCVE identifiers where matched
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
	scanCmd.Flags().BoolVar(&scanDangling, "dangling", false, "Scan for dangling/orphan resources (HPA, Service, Ingress, NetworkPolicy)")
	scanCmd.Flags().StringVar(&scanThreshold, "threshold", "5m", "Duration threshold for stuck detection (e.g., 30s, 2m, 5m)")
	scanCmd.Flags().StringVar(&scanFile, "file", "", "YAML file to scan (static analysis, no cluster required)")
}

// CombinedScanResult holds results from all scanners
type CombinedScanResult struct {
	Kyverno     *agent.ScanResult        `json:"kyverno,omitempty"`
	State       *agent.StateScanResult   `json:"state,omitempty"`
	TimingBombs *agent.TimingBombResult  `json:"timingBombs,omitempty"`
	Unresolved  *agent.UnresolvedResult  `json:"unresolved,omitempty"`
	Dangling    *agent.DanglingResult    `json:"dangling,omitempty"`
	Static      *agent.StaticScanResult  `json:"static,omitempty"`
}

func runScan(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Check ConfigHub connection for full scan capabilities
	// Note: --list and --file modes work with embedded patterns
	// Full cluster scanning requires ConfigHub pattern database
	client := hub.NewClient()
	if !scanList && scanFile == "" {
		if err := client.RequireConnected(); err != nil {
			// TODO: When pattern database is fully migrated to ConfigHub API,
			// uncomment this to enforce auth. For now, use embedded patterns.
			// return err
			_ = err // Placeholder: will enforce auth when pattern DB is API-based
		}
	}

	// Find policy database directory
	policyDBDir := findPolicyDBDir()

	// List mode - show all KPOL policies
	if scanList {
		return listKPOLPolicies(policyDBDir)
	}

	// File mode - static analysis of YAML file (no cluster required)
	if scanFile != "" {
		return runFileScan(ctx, scanFile, policyDBDir)
	}

	// Build k8s config
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	// Determine what to scan (default: both)
	runKyverno := !scanStateOnly || scanKyvernoOnly
	runState := !scanKyvernoOnly || scanStateOnly
	// If both flags are set, run both
	if scanStateOnly && scanKyvernoOnly {
		runKyverno = true
		runState = true
	}

	var kyvernoResult *agent.ScanResult
	var stateResult *agent.StateScanResult
	var timingBombResult *agent.TimingBombResult
	var unresolvedResult *agent.UnresolvedResult
	var danglingResult *agent.DanglingResult

	// Run Kyverno scan
	if runKyverno {
		scanner, err := agent.NewKyvernoScanner(cfg, policyDBDir)
		if err != nil {
			return fmt.Errorf("failed to create kyverno scanner: %w", err)
		}

		if scanner.Available(ctx) {
			if scanNamespace != "" {
				kyvernoResult, err = scanner.ScanNamespace(ctx, scanNamespace)
			} else {
				kyvernoResult, err = scanner.Scan(ctx)
			}
			if err != nil {
				return fmt.Errorf("kyverno scan failed: %w", err)
			}
		} else if scanKyvernoOnly {
			// Only warn if Kyverno was explicitly requested
			if scanJSON {
				return outputCombinedJSON(&CombinedScanResult{
					Kyverno: &agent.ScanResult{Error: "Kyverno not installed or PolicyReport CRD not found"},
				})
			}
			fmt.Printf("\n%s⚠ Kyverno not installed%s\n", colorYellow, colorReset)
			fmt.Printf("  PolicyReport CRD not found in cluster.\n")
			fmt.Printf("  Install Kyverno: https://kyverno.io/docs/installation/\n\n")
			return nil
		}
	}

	// Run State scan
	if runState {
		stateScanner, err := agent.NewStateScanner(cfg)
		if err != nil {
			return fmt.Errorf("failed to create state scanner: %w", err)
		}

		// Parse threshold duration
		threshold, err := time.ParseDuration(scanThreshold)
		if err != nil {
			return fmt.Errorf("invalid threshold duration %q: %w", scanThreshold, err)
		}

		if scanNamespace != "" {
			stateResult, err = stateScanner.ScanNamespaceWithThreshold(ctx, scanNamespace, threshold)
		} else {
			stateResult, err = stateScanner.ScanWithThreshold(ctx, threshold)
		}
		if err != nil {
			return fmt.Errorf("state scan failed: %w", err)
		}
	}

	// Run Timing Bombs scan
	if scanTimingBombs {
		stateScanner, err := agent.NewStateScanner(cfg)
		if err != nil {
			return fmt.Errorf("failed to create state scanner for timing bombs: %w", err)
		}

		timingBombResult, err = stateScanner.ScanTimingBombs(ctx)
		if err != nil {
			return fmt.Errorf("timing bomb scan failed: %w", err)
		}
	}

	// Run Unresolved Findings scan
	if scanIncludeUnresolved {
		stateScanner, err := agent.NewStateScanner(cfg)
		if err != nil {
			return fmt.Errorf("failed to create state scanner for unresolved: %w", err)
		}

		unresolvedResult, err = stateScanner.ScanUnresolvedFindings(ctx)
		if err != nil {
			return fmt.Errorf("unresolved findings scan failed: %w", err)
		}
	}

	// Run Dangling Resources scan
	if scanDangling {
		stateScanner, err := agent.NewStateScanner(cfg)
		if err != nil {
			return fmt.Errorf("failed to create state scanner for dangling: %w", err)
		}

		danglingResult, err = stateScanner.ScanDanglingResources(ctx)
		if err != nil {
			return fmt.Errorf("dangling resources scan failed: %w", err)
		}
	}

	// Output results
	if scanJSON {
		return outputCombinedJSON(&CombinedScanResult{
			Kyverno:     kyvernoResult,
			State:       stateResult,
			TimingBombs: timingBombResult,
			Unresolved:  unresolvedResult,
			Dangling:    danglingResult,
		})
	}
	return outputCombinedHuman(kyvernoResult, stateResult, timingBombResult, unresolvedResult, danglingResult)
}

// findPolicyDBDir locates the Kyverno policy database
func findPolicyDBDir() string {
	// Try relative to executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "..", "cve", "ccve", "kyverno")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	// Try relative to current directory
	cwd, err := os.Getwd()
	if err == nil {
		dir := filepath.Join(cwd, "cve", "ccve", "kyverno")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	// Try common locations
	locations := []string{
		"/usr/local/share/confighub/cve/ccve/kyverno",
		"/usr/share/confighub/cve/ccve/kyverno",
	}
	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return ""
}

// listKPOLPolicies lists all policies in the database
func listKPOLPolicies(policyDBDir string) error {
	if policyDBDir == "" {
		return fmt.Errorf("policy database not found")
	}

	scanner := agent.NewKyvernoScannerWithClient(nil, policyDBDir)
	policies, err := scanner.GetPolicyCatalog()
	if err != nil {
		return fmt.Errorf("failed to load policy catalog: %w", err)
	}

	// Sort by ID
	sort.Slice(policies, func(i, j int) bool {
		return policies[i].ID < policies[j].ID
	})

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

	hasOutput := false

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

	// ConfigHub hook hint
	fmt.Printf("%s🔗 Track violations in ConfigHub: cub-scout scan --confighub%s\n\n", colorDim, colorReset)

	return nil
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

// runFileScan performs static analysis on a YAML file without requiring cluster access
func runFileScan(ctx context.Context, filename string, ccveDBDir string) error {
	// Determine CCVE database directory (parent of kyverno dir)
	ccveDir := ccveDBDir
	if strings.HasSuffix(ccveDir, "/kyverno") {
		ccveDir = filepath.Dir(ccveDir)
	}

	scanner, err := agent.NewStaticScanner(ccveDir)
	if err != nil {
		return fmt.Errorf("failed to create static scanner: %w", err)
	}

	result, err := scanner.ScanFile(ctx, filename)
	if err != nil {
		return fmt.Errorf("static scan failed: %w", err)
	}

	if scanJSON {
		return outputCombinedJSON(&CombinedScanResult{Static: result})
	}
	return outputStaticScanHuman(result)
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
