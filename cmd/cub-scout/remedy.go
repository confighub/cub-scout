// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/remedy"
)

var (
	remedyDryRun    bool
	remedyNamespace string
	remedyForce     bool
	remedyAll       bool
	remedyJSON      bool
	remedyFile      string
	remedyList      bool
	remedyTimeout   string
	remedyAudit     bool
	remedyAuditFile string
)

var remedyCmd = &cobra.Command{
	Use:   "remedy [CCVE-ID]",
	Short: "Execute remediation for CCVE findings",
	Long: `Execute automated remediation for detected CCVE issues.

The remedy command can:
1. Fix a specific CCVE finding by ID
2. Fix all auto-fixable findings in a namespace
3. Show what would be fixed (dry-run mode)

Examples:
  # Show what would be fixed (always run first!)
  cub-scout remedy CCVE-2025-0687 --dry-run -n production

  # Fix a specific CCVE issue
  cub-scout remedy CCVE-2025-0687 -n production

  # Fix all auto-fixable issues in namespace (dry-run)
  cub-scout remedy --all --dry-run -n production

  # Force fix without confirmation
  cub-scout remedy CCVE-2025-0687 -n production --force

  # Scan file and fix issues
  cub-scout remedy --file manifest.yaml --dry-run

  # List auto-fixable CCVEs
  cub-scout remedy --list

Supported remedy types (auto-fixable):
  - config_fix:      786 CCVEs - kubectl apply/patch
  - trigger_action:  169 CCVEs - rollout restart, scale
  - delete_resource: 348 CCVEs - delete orphaned resources (needs --force)
  - restart:          70 CCVEs - restart pods

Total auto-fixable: 1,373 CCVEs (40%)
`,
	RunE: runRemedy,
}

func init() {
	rootCmd.AddCommand(remedyCmd)

	remedyCmd.Flags().BoolVar(&remedyDryRun, "dry-run", true, "Show what would be changed (default: true)")
	remedyCmd.Flags().StringVarP(&remedyNamespace, "namespace", "n", "", "Namespace to operate in")
	remedyCmd.Flags().BoolVar(&remedyForce, "force", false, "Skip confirmation for high-risk actions")
	remedyCmd.Flags().BoolVar(&remedyAll, "all", false, "Fix all auto-fixable issues")
	remedyCmd.Flags().BoolVar(&remedyJSON, "json", false, "Output as JSON")
	remedyCmd.Flags().StringVar(&remedyFile, "file", "", "YAML file to scan and fix")
	remedyCmd.Flags().BoolVar(&remedyList, "list", false, "List auto-fixable CCVEs")
	remedyCmd.Flags().StringVar(&remedyTimeout, "timeout", "30s", "Timeout for each action")
	remedyCmd.Flags().BoolVar(&remedyAudit, "audit", true, "Log actions to audit file")
	remedyCmd.Flags().StringVar(&remedyAuditFile, "audit-file", "remedy-audit.log", "Audit log file path")
}

// RemedyOutput is the JSON output structure
type RemedyOutput struct {
	DryRun   bool               `json:"dryRun"`
	Findings []RemedyFindingOut `json:"findings"`
	Summary  RemedySummary      `json:"summary"`
}

type RemedyFindingOut struct {
	CCVE       string             `json:"ccve"`
	Resource   string             `json:"resource"`
	Namespace  string             `json:"namespace,omitempty"`
	RemedyType string             `json:"remedyType"`
	RiskLevel  string             `json:"riskLevel"`
	Reversible bool               `json:"reversible"`
	Actions    []RemedyActionOut  `json:"actions"`
	Result     *RemedyResultOut   `json:"result,omitempty"`
}

type RemedyActionOut struct {
	Description string `json:"description"`
	Command     string `json:"command"`
}

type RemedyResultOut struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	RollbackCmd string `json:"rollbackCmd,omitempty"`
}

type RemedySummary struct {
	Total     int `json:"total"`
	Fixed     int `json:"fixed"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}

func runRemedy(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Parse timeout
	timeout, err := time.ParseDuration(remedyTimeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}

	// List mode
	if remedyList {
		return listAutoFixableCCVEs()
	}

	// Build registry
	reg := remedy.DefaultRegistry()

	// File mode - static analysis
	if remedyFile != "" {
		return runRemedyFile(ctx, reg, remedyFile, timeout)
	}

	// Require namespace for cluster operations
	if remedyNamespace == "" && !remedyAll {
		return fmt.Errorf("namespace required (-n) or use --all for all namespaces")
	}

	// All mode - scan and fix all
	if remedyAll {
		return runRemedyAll(ctx, reg, timeout)
	}

	// Single CCVE mode
	if len(args) < 1 {
		return fmt.Errorf("CCVE ID required (e.g., CCVE-2025-0687) or use --all")
	}

	ccveID := args[0]
	return runRemedySingle(ctx, reg, ccveID, timeout)
}

func runRemedySingle(ctx context.Context, reg *remedy.Registry, ccveID string, timeout time.Duration) error {
	// Load CCVE definition
	ccve, err := loadCCVE(ccveID)
	if err != nil {
		return fmt.Errorf("load CCVE %s: %w", ccveID, err)
	}

	// Check if auto-fixable
	if !remedy.IsAutoFixable(remedy.RemedyType(ccve.RemedyType)) {
		return fmt.Errorf("CCVE %s has remedy type %q which is not auto-fixable", ccveID, ccve.RemedyType)
	}

	// Create finding from CCVE
	finding := &remedy.Finding{
		CCVE:       ccveID,
		Namespace:  remedyNamespace,
		RemedyType: remedy.RemedyType(ccve.RemedyType),
		Commands:   ccve.Commands,
		Steps:      ccve.Steps,
		Resource: remedy.ResourceRef{
			Kind: ccve.Kind,
		},
	}

	// Validate finding before proceeding
	if err := validateFinding(ctx, finding); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Get executor
	executor, err := reg.ExecutorFor(finding)
	if err != nil {
		return fmt.Errorf("no executor: %w", err)
	}

	// Run dry-run
	plan, err := executor.DryRun(ctx, finding)
	if err != nil {
		return fmt.Errorf("plan failed: %w", err)
	}

	// Output plan
	if remedyJSON {
		return outputRemedyPlanJSON(plan, nil)
	}

	printRemedyPlan(plan)

	// Stop if dry-run
	if remedyDryRun {
		logRemedyAction(finding, nil, true)
		fmt.Printf("\n%s[dry-run] No changes made. Remove --dry-run to apply.%s\n\n", colorYellow, colorReset)
		return nil
	}

	// Confirm high-risk actions
	if plan.RiskLevel == remedy.RiskHigh && !remedyForce {
		fmt.Printf("\n%s⚠ HIGH RISK: This action is irreversible!%s\n", colorRed, colorReset)
		if !confirmRemedy("Continue?") {
			fmt.Println("Cancelled")
			return nil
		}
	}

	// Execute
	opts := &remedy.ExecuteOptions{
		DryRun:  false,
		Force:   remedyForce,
		Timeout: timeout,
	}

	result, err := executor.Execute(ctx, finding, opts)
	if err != nil {
		logRemedyAction(finding, nil, false)
		return fmt.Errorf("execute failed: %w", err)
	}

	// Log the action
	logRemedyAction(finding, result, false)

	// Output result
	if remedyJSON {
		return outputRemedyPlanJSON(plan, result)
	}

	printRemedyResult(result)
	return nil
}

func runRemedyAll(ctx context.Context, reg *remedy.Registry, timeout time.Duration) error {
	// Build k8s config
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	// Run static scan on cluster
	stateScanner, err := agent.NewStateScanner(cfg)
	if err != nil {
		return fmt.Errorf("create scanner: %w", err)
	}

	// Scan for dangling resources
	danglingResult, err := stateScanner.ScanDanglingResources(ctx)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Convert to findings
	var findings []*remedy.Finding
	for _, d := range danglingResult.Findings {
		// Only include delete_resource type for dangling findings
		findings = append(findings, &remedy.Finding{
			CCVE:       d.CCVEID,
			Namespace:  d.Namespace,
			RemedyType: remedy.DeleteResource,
			Commands:   []string{d.Command},
			Resource: remedy.ResourceRef{
				Kind:      d.Kind,
				Name:      d.Name,
				Namespace: d.Namespace,
			},
		})
	}

	if len(findings) == 0 {
		fmt.Printf("\n%s✓ No auto-fixable issues found%s\n\n", colorGreen, colorReset)
		return nil
	}

	// Process findings
	output := &RemedyOutput{
		DryRun: remedyDryRun,
	}

	for _, finding := range findings {
		if remedyNamespace != "" && finding.Namespace != remedyNamespace {
			continue
		}

		executor, err := reg.ExecutorFor(finding)
		if err != nil {
			output.Summary.Skipped++
			continue
		}

		plan, err := executor.DryRun(ctx, finding)
		if err != nil {
			output.Summary.Skipped++
			continue
		}

		findingOut := RemedyFindingOut{
			CCVE:       finding.CCVE,
			Resource:   finding.Resource.String(),
			Namespace:  finding.Namespace,
			RemedyType: string(finding.RemedyType),
			RiskLevel:  string(plan.RiskLevel),
			Reversible: plan.Reversible,
		}

		for _, action := range plan.Actions {
			findingOut.Actions = append(findingOut.Actions, RemedyActionOut{
				Description: action.Description,
				Command:     action.Command,
			})
		}

		if !remedyDryRun {
			// Skip high-risk without force
			if plan.RiskLevel == remedy.RiskHigh && !remedyForce {
				output.Summary.Skipped++
				continue
			}

			opts := &remedy.ExecuteOptions{
				DryRun:  false,
				Force:   remedyForce,
				Timeout: timeout,
			}

			result, err := executor.Execute(ctx, finding, opts)
			if err != nil || !result.Success {
				output.Summary.Failed++
				findingOut.Result = &RemedyResultOut{
					Success: false,
					Message: err.Error(),
				}
			} else {
				output.Summary.Fixed++
				findingOut.Result = &RemedyResultOut{
					Success:     true,
					Message:     result.Message,
					RollbackCmd: result.RollbackCmd,
				}
			}
		}

		output.Findings = append(output.Findings, findingOut)
		output.Summary.Total++
	}

	if remedyJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	// Human output
	printRemedyAllSummary(output)
	return nil
}

func runRemedyFile(ctx context.Context, reg *remedy.Registry, file string, timeout time.Duration) error {
	// Run static scan
	ccveDir := findCCVEDir()
	scanner, err := agent.NewStaticScanner(ccveDir)
	if err != nil {
		return fmt.Errorf("create scanner: %w", err)
	}
	result, err := scanner.ScanFile(ctx, file)
	if err != nil {
		return fmt.Errorf("scan file: %w", err)
	}

	if len(result.Findings) == 0 {
		fmt.Printf("\n%s✓ No issues found in %s%s\n\n", colorGreen, file, colorReset)
		return nil
	}

	fmt.Printf("\n%s%sFOUND %d ISSUES%s\n", colorBold, colorYellow, len(result.Findings), colorReset)
	fmt.Printf("%sFile: %s%s\n\n", colorDim, file, colorReset)

	autoFixable := 0
	for _, f := range result.Findings {
		// Load CCVE to get remedy type
		ccve, err := loadCCVE(f.CCVEID)
		if err != nil {
			continue
		}
		if remedy.IsAutoFixable(remedy.RemedyType(ccve.RemedyType)) {
			autoFixable++
		}
	}

	fmt.Printf("  Total findings:  %d\n", len(result.Findings))
	fmt.Printf("  Auto-fixable:    %d\n", autoFixable)
	fmt.Printf("\n%sRun 'cub-scout remedy CCVE-ID --file %s' to fix specific issues%s\n\n",
		colorDim, file, colorReset)

	return nil
}

func listAutoFixableCCVEs() error {
	ccveDir := findCCVEDir()
	if ccveDir == "" {
		return fmt.Errorf("CCVE database not found")
	}

	// Count by remedy type
	counts := map[string]int{
		"config_fix":      0,
		"trigger_action":  0,
		"delete_resource": 0,
		"restart":         0,
	}

	files, _ := filepath.Glob(filepath.Join(ccveDir, "CCVE-*.yaml"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}

		// Parse with raw map to get nested remedy.type
		var raw map[string]interface{}
		if err := yaml.Unmarshal(data, &raw); err != nil {
			continue
		}

		remedyType := ""
		if remedyBlock, ok := raw["remedy"].(map[string]interface{}); ok {
			if t, ok := remedyBlock["type"].(string); ok {
				remedyType = t
			}
		}

		if _, ok := counts[remedyType]; ok {
			counts[remedyType]++
		}
	}

	fmt.Printf("\n%s%sAUTO-FIXABLE CCVEs%s\n\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%-18s %6s %s\n", "REMEDY TYPE", "COUNT", "DESCRIPTION")
	fmt.Printf("%-18s %6s %s\n", "-----------", "-----", "-----------")
	fmt.Printf("%-18s %6d %s\n", "config_fix", counts["config_fix"], "kubectl apply/patch")
	fmt.Printf("%-18s %6d %s\n", "delete_resource", counts["delete_resource"], "kubectl delete (needs --force)")
	fmt.Printf("%-18s %6d %s\n", "trigger_action", counts["trigger_action"], "rollout restart/scale")
	fmt.Printf("%-18s %6d %s\n", "restart", counts["restart"], "pod/deployment restart")
	fmt.Printf("%-18s %6s %s\n", "-----------", "-----", "-----------")

	total := counts["config_fix"] + counts["trigger_action"] + counts["delete_resource"] + counts["restart"]
	fmt.Printf("%-18s %6d\n", "TOTAL", total)
	fmt.Printf("\n%sRun 'cub-scout remedy --all --dry-run -n <namespace>' to find fixable issues%s\n\n",
		colorDim, colorReset)

	return nil
}

// CCVEDefinition represents a CCVE YAML file
type CCVEDefinition struct {
	ID          string   `yaml:"id"`
	Category    string   `yaml:"category"`
	Name        string   `yaml:"name"`
	Severity    string   `yaml:"severity"`
	Kind        string   `yaml:"-"` // Extracted from detection.resources
	RemedyType  string   `yaml:"-"` // From remedy.type
	Commands    []string `yaml:"-"` // From remediation.commands
	Steps       []string `yaml:"-"` // From remediation.steps
}

func loadCCVE(id string) (*CCVEDefinition, error) {
	ccveDir := findCCVEDir()
	if ccveDir == "" {
		return nil, fmt.Errorf("CCVE database not found")
	}

	filename := filepath.Join(ccveDir, id+".yaml")
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", id, err)
	}

	// Parse YAML with nested structure
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", id, err)
	}

	ccve := &CCVEDefinition{
		ID:       getString(raw, "id"),
		Category: getString(raw, "category"),
		Name:     getString(raw, "name"),
		Severity: getString(raw, "severity"),
	}

	// Extract detection.resources[0] as Kind
	if detection, ok := raw["detection"].(map[string]interface{}); ok {
		if resources, ok := detection["resources"].([]interface{}); ok && len(resources) > 0 {
			ccve.Kind = fmt.Sprintf("%v", resources[0])
		}
	}

	// Extract remedy.type
	if remedy, ok := raw["remedy"].(map[string]interface{}); ok {
		ccve.RemedyType = getString(remedy, "type")
	}

	// Extract remediation.commands and steps
	if remediation, ok := raw["remediation"].(map[string]interface{}); ok {
		if cmds, ok := remediation["commands"].([]interface{}); ok {
			for _, cmd := range cmds {
				ccve.Commands = append(ccve.Commands, fmt.Sprintf("%v", cmd))
			}
		}
		if steps, ok := remediation["steps"].([]interface{}); ok {
			for _, step := range steps {
				ccve.Steps = append(ccve.Steps, fmt.Sprintf("%v", step))
			}
		}
	}

	return ccve, nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func findCCVEDir() string {
	// Try relative to executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "..", "cve", "ccve")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	// Try relative to current directory
	cwd, err := os.Getwd()
	if err == nil {
		dir := filepath.Join(cwd, "cve", "ccve")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	return ""
}

func printRemedyPlan(plan *remedy.RemedyPlan) {
	fmt.Printf("\n%s%s=== REMEDY PLAN ===%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("CCVE:       %s\n", plan.Finding.CCVE)
	fmt.Printf("Resource:   %s\n", plan.Finding.Resource.String())

	riskColor := colorGreen
	if plan.RiskLevel == remedy.RiskMedium {
		riskColor = colorYellow
	} else if plan.RiskLevel == remedy.RiskHigh {
		riskColor = colorRed
	}
	fmt.Printf("Risk Level: %s%s%s\n", riskColor, plan.RiskLevel, colorReset)
	fmt.Printf("Reversible: %v\n", plan.Reversible)

	fmt.Printf("\n%sActions:%s\n", colorBold, colorReset)
	for i, action := range plan.Actions {
		fmt.Printf("  %d. %s\n", i+1, action.Description)
		fmt.Printf("     %s$ %s%s\n", colorDim, action.Command, colorReset)
	}
}

func printRemedyResult(result *remedy.RemedyResult) {
	fmt.Printf("\n%s%s=== RESULT ===%s\n", colorBold, colorCyan, colorReset)

	if result.Success {
		fmt.Printf("%s✓ %s%s\n", colorGreen, result.Message, colorReset)
	} else {
		fmt.Printf("%s✗ %s%s\n", colorRed, result.Message, colorReset)
	}

	for _, action := range result.Actions {
		if action.Success {
			fmt.Printf("  %s✓%s %s\n", colorGreen, colorReset, action.Action.Description)
		} else {
			fmt.Printf("  %s✗%s %s: %s\n", colorRed, colorReset, action.Action.Description, action.Error)
		}
		if action.Output != "" && strings.TrimSpace(action.Output) != "" {
			fmt.Printf("    %s%s%s\n", colorDim, strings.TrimSpace(action.Output), colorReset)
		}
	}

	if result.RollbackCmd != "" {
		fmt.Printf("\n%sRollback command:%s\n", colorDim, colorReset)
		fmt.Printf("  %s\n", result.RollbackCmd)
	}
	fmt.Println()
}

func printRemedyAllSummary(output *RemedyOutput) {
	fmt.Printf("\n%s%s=== REMEDY SUMMARY ===%s\n\n", colorBold, colorCyan, colorReset)

	if output.DryRun {
		fmt.Printf("%s[dry-run mode]%s\n\n", colorYellow, colorReset)
	}

	for _, f := range output.Findings {
		riskColor := colorGreen
		if f.RiskLevel == "medium" {
			riskColor = colorYellow
		} else if f.RiskLevel == "high" {
			riskColor = colorRed
		}

		fmt.Printf("%s%s%s %s/%s\n", riskColor, f.CCVE, colorReset, f.Namespace, f.Resource)
		for _, action := range f.Actions {
			fmt.Printf("  %s→%s %s\n", colorDim, colorReset, action.Description)
		}

		if f.Result != nil {
			if f.Result.Success {
				fmt.Printf("  %s✓ Fixed%s\n", colorGreen, colorReset)
			} else {
				fmt.Printf("  %s✗ Failed: %s%s\n", colorRed, f.Result.Message, colorReset)
			}
		}
		fmt.Println()
	}

	fmt.Printf("Total:   %d\n", output.Summary.Total)
	if !output.DryRun {
		fmt.Printf("Fixed:   %s%d%s\n", colorGreen, output.Summary.Fixed, colorReset)
		fmt.Printf("Skipped: %d\n", output.Summary.Skipped)
		fmt.Printf("Failed:  %s%d%s\n", colorRed, output.Summary.Failed, colorReset)
	}
	fmt.Println()
}

func outputRemedyPlanJSON(plan *remedy.RemedyPlan, result *remedy.RemedyResult) error {
	output := RemedyFindingOut{
		CCVE:       plan.Finding.CCVE,
		Resource:   plan.Finding.Resource.String(),
		Namespace:  plan.Finding.Namespace,
		RemedyType: string(plan.Finding.RemedyType),
		RiskLevel:  string(plan.RiskLevel),
		Reversible: plan.Reversible,
	}

	for _, action := range plan.Actions {
		output.Actions = append(output.Actions, RemedyActionOut{
			Description: action.Description,
			Command:     action.Command,
		})
	}

	if result != nil {
		output.Result = &RemedyResultOut{
			Success:     result.Success,
			Message:     result.Message,
			RollbackCmd: result.RollbackCmd,
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func confirmRemedy(prompt string) bool {
	fmt.Printf("%s [y/N]: ", prompt)
	var response string
	fmt.Scanln(&response)
	return strings.ToLower(response) == "y"
}

// validateFinding performs pre-execution safety checks
func validateFinding(ctx context.Context, f *remedy.Finding) error {
	// Check CCVE exists
	if _, err := loadCCVE(f.CCVE); err != nil {
		return fmt.Errorf("unknown CCVE: %s", f.CCVE)
	}

	// Check namespace exists (if specified)
	if f.Namespace != "" {
		if err := checkNamespaceExists(ctx, f.Namespace); err != nil {
			return err
		}
	}

	// Check resource exists (if specified)
	if f.Resource.Name != "" {
		if err := checkResourceExists(ctx, f.Resource, f.Namespace); err != nil {
			return fmt.Errorf("resource not found: %v", err)
		}
	}

	return nil
}

// checkNamespaceExists verifies the namespace exists in the cluster
func checkNamespaceExists(ctx context.Context, namespace string) error {
	cmd := fmt.Sprintf("kubectl get namespace %s -o name 2>/dev/null", namespace)
	out, err := execCommand(ctx, cmd)
	if err != nil || strings.TrimSpace(out) == "" {
		return fmt.Errorf("namespace %q not found", namespace)
	}
	return nil
}

// checkResourceExists verifies the resource exists in the cluster
func checkResourceExists(ctx context.Context, ref remedy.ResourceRef, namespace string) error {
	if ref.Kind == "" || ref.Name == "" {
		return nil // Skip check if resource not fully specified
	}

	cmd := fmt.Sprintf("kubectl get %s %s", strings.ToLower(ref.Kind), ref.Name)
	if namespace != "" {
		cmd += fmt.Sprintf(" -n %s", namespace)
	}
	cmd += " -o name 2>/dev/null"

	out, err := execCommand(ctx, cmd)
	if err != nil || strings.TrimSpace(out) == "" {
		return fmt.Errorf("%s/%s not found", ref.Kind, ref.Name)
	}
	return nil
}

// execCommand runs a shell command and returns output
func execCommand(ctx context.Context, cmd string) (string, error) {
	out, err := exec.CommandContext(ctx, "sh", "-c", cmd).CombinedOutput()
	return string(out), err
}

// logRemedyAction logs remedy actions to the audit file
func logRemedyAction(finding *remedy.Finding, result *remedy.RemedyResult, dryRun bool) {
	if !remedyAudit {
		return
	}

	status := "SUCCESS"
	if result != nil && !result.Success {
		status = "FAILED"
	}
	if dryRun {
		status = "DRY-RUN"
	}

	logLine := fmt.Sprintf("[%s] %s CCVE=%s Namespace=%s Resource=%s/%s Message=%q\n",
		time.Now().Format(time.RFC3339),
		status,
		finding.CCVE,
		finding.Namespace,
		finding.Resource.Kind,
		finding.Resource.Name,
		getMessage(result),
	)

	f, err := os.OpenFile(remedyAuditFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write to audit log: %v\n", err)
		return
	}
	defer f.Close()
	f.WriteString(logLine)
}

func getMessage(result *remedy.RemedyResult) string {
	if result == nil {
		return "planned"
	}
	return result.Message
}
