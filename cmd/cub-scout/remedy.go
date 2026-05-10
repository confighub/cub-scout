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

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/remedy"
)

// suggest-remedy is the read-only successor to the previous `remedy` command.
//
// cub-scout never applies a fix. This command describes a structured
// suggested patch that *would* resolve a risk finding — caller (Pilot,
// ConfigHub, an operator) decides whether and how to apply it.
//
// History: the original `remedy` command both produced the suggestion AND
// applied it via `kubectl apply / patch / delete`. That violated the
// triad-locked contract that cub-scout is read-only evidence. See #410
// (decision) and #428 (this implementation).

var (
	suggestNamespace string
	suggestAll       bool
	suggestJSON      bool
	suggestFile      string
	suggestList      bool
)

var suggestRemedyCmd = &cobra.Command{
	Use:     "suggest-remedy [RISK-ID]",
	Aliases: []string{"remedy"},
	Short:   "Describe a suggested remediation for a risk finding (read-only)",
	Long: `Describe how a risk finding would be remediated, without applying anything.

cub-scout never modifies cluster state. suggest-remedy emits a structured
description of the patch that would resolve a finding — the kubectl command
that would carry it out, the current state, and the expected change. It is
up to a downstream tool (ConfigHub Pilot, an operator, or your CI pipeline)
to decide whether and how to apply that suggestion.

Examples:
  # Describe the suggested fix for a specific finding
  cub-scout suggest-remedy CCVE-2025-0687 -n production

  # Describe suggestions for all auto-fixable findings in a namespace
  cub-scout suggest-remedy --all -n production

  # Scan a manifest file and report suggestions
  cub-scout suggest-remedy --file manifest.yaml

  # JSON output (the load-bearing contract for downstream tools)
  cub-scout suggest-remedy --all -n production --json

  # List the auto-fixable risk-issue catalogue
  cub-scout suggest-remedy --list

Auto-fixable remedy types (cub-scout can produce a structured suggestion):
  - config_fix       (kubectl apply / patch)
  - trigger_action   (rollout restart, scale)
  - delete_resource  (kubectl delete)
  - restart          (pod / deployment restart)

The legacy ` + "`remedy`" + ` verb is still accepted as an alias. The
` + "`--dry-run`, `--force`, `--audit`, and `--audit-file`" + ` flags have
been removed because cub-scout no longer has a non-dry-run path.
`,
	RunE: runSuggestRemedy,
}

func init() {
	rootCmd.AddCommand(suggestRemedyCmd)

	suggestRemedyCmd.Flags().StringVarP(&suggestNamespace, "namespace", "n", "", "Namespace to operate in")
	suggestRemedyCmd.Flags().BoolVar(&suggestAll, "all", false, "Describe suggestions for all auto-fixable findings")
	suggestRemedyCmd.Flags().BoolVar(&suggestJSON, "json", false, "Output as JSON")
	suggestRemedyCmd.Flags().StringVar(&suggestFile, "file", "", "YAML file to scan for findings")
	suggestRemedyCmd.Flags().BoolVar(&suggestList, "list", false, "List auto-fixable risk-issue categories")
}

// SuggestRemedyOutput is the top-level JSON contract.
type SuggestRemedyOutput struct {
	Suggestions []SuggestedFindingOut `json:"suggestions"`
	Summary     SuggestSummary        `json:"summary"`
}

// SuggestedFindingOut is one suggested-remedy entry in the JSON contract.
type SuggestedFindingOut struct {
	CCVE       string               `json:"ccve"`
	Resource   string               `json:"resource"`
	Namespace  string               `json:"namespace,omitempty"`
	RemedyType string               `json:"remedyType"`
	RiskLevel  string               `json:"riskLevel"`
	Reversible bool                 `json:"reversible"`
	Actions    []SuggestedActionOut `json:"actions"`
}

// SuggestedActionOut describes one step of a suggested remedy.
type SuggestedActionOut struct {
	Description string `json:"description"`
	Command     string `json:"command"`
}

// SuggestSummary aggregates counts. cub-scout never applies, so there are
// no fixed/failed counts — only describable vs not.
type SuggestSummary struct {
	Total       int `json:"total"`
	Describable int `json:"describable"`
	Skipped     int `json:"skipped"`
}

func runSuggestRemedy(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	if suggestList {
		return listAutoFixableCCVEs()
	}

	reg := remedy.DefaultRegistry()

	if suggestFile != "" {
		return runSuggestFile(ctx, reg, suggestFile)
	}

	if suggestNamespace == "" && !suggestAll {
		return fmt.Errorf("namespace required (-n) or use --all for all namespaces")
	}

	if suggestAll {
		return runSuggestAll(ctx, reg)
	}

	if len(args) < 1 {
		return fmt.Errorf("risk issue ID required (e.g., CCVE-2025-0687) or use --all")
	}

	return runSuggestSingle(ctx, reg, args[0])
}

func runSuggestSingle(ctx context.Context, reg *remedy.Registry, ccveID string) error {
	ccve, err := loadCCVE(ccveID)
	if err != nil {
		return fmt.Errorf("load risk issue %s: %w", ccveID, err)
	}

	if !remedy.IsAutoFixable(remedy.RemedyType(ccve.RemedyType)) {
		return fmt.Errorf("risk issue %s has remedy type %q which is not auto-suggestable", ccveID, ccve.RemedyType)
	}

	finding := &remedy.Finding{
		CCVE:       ccveID,
		Namespace:  suggestNamespace,
		RemedyType: remedy.RemedyType(ccve.RemedyType),
		Commands:   ccve.Commands,
		Steps:      ccve.Steps,
		Resource: remedy.ResourceRef{
			Kind: ccve.Kind,
		},
	}

	if err := validateFinding(ctx, finding); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	suggester, err := reg.SuggesterFor(finding)
	if err != nil {
		return fmt.Errorf("no suggester: %w", err)
	}

	suggestion, err := suggester.Suggest(ctx, finding)
	if err != nil {
		return fmt.Errorf("describe suggestion: %w", err)
	}

	if suggestJSON {
		return outputSuggestionJSON(suggestion)
	}

	printSuggestion(suggestion)
	fmt.Printf("\n%scub-scout produces evidence only. Apply via ConfigHub or kubectl, governed.%s\n\n",
		colorDim, colorReset)
	return nil
}

func runSuggestAll(ctx context.Context, reg *remedy.Registry) error {
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	stateScanner, err := agent.NewStateScanner(cfg)
	if err != nil {
		return fmt.Errorf("create scanner: %w", err)
	}

	danglingResult, err := stateScanner.ScanDanglingResources(ctx)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	var findings []*remedy.Finding
	for _, d := range danglingResult.Findings {
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
		fmt.Printf("\n%s✓ No auto-suggestable findings%s\n\n", colorGreen, colorReset)
		return nil
	}

	output := &SuggestRemedyOutput{}

	for _, finding := range findings {
		if suggestNamespace != "" && finding.Namespace != suggestNamespace {
			continue
		}

		suggester, err := reg.SuggesterFor(finding)
		if err != nil {
			output.Summary.Skipped++
			continue
		}

		suggestion, err := suggester.Suggest(ctx, finding)
		if err != nil {
			output.Summary.Skipped++
			continue
		}

		entry := SuggestedFindingOut{
			CCVE:       finding.CCVE,
			Resource:   finding.Resource.String(),
			Namespace:  finding.Namespace,
			RemedyType: string(finding.RemedyType),
			RiskLevel:  string(suggestion.RiskLevel),
			Reversible: suggestion.Reversible,
		}

		for _, action := range suggestion.Actions {
			entry.Actions = append(entry.Actions, SuggestedActionOut{
				Description: action.Description,
				Command:     action.Command,
			})
		}

		output.Suggestions = append(output.Suggestions, entry)
		output.Summary.Total++
		output.Summary.Describable++
	}

	if suggestJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	printSuggestSummary(output)
	return nil
}

func runSuggestFile(ctx context.Context, _ *remedy.Registry, file string) error {
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

	autoSuggestable := 0
	for _, f := range result.Findings {
		ccve, err := loadCCVE(f.CCVEID)
		if err != nil {
			continue
		}
		if remedy.IsAutoFixable(remedy.RemedyType(ccve.RemedyType)) {
			autoSuggestable++
		}
	}

	fmt.Printf("  Total findings:    %d\n", len(result.Findings))
	fmt.Printf("  Auto-suggestable:  %d\n", autoSuggestable)
	fmt.Printf("\n%sRun 'cub-scout suggest-remedy RISK-ID --file %s' for a specific suggestion%s\n\n",
		colorDim, file, colorReset)

	return nil
}

func listAutoFixableCCVEs() error {
	ccveDir := findCCVEDir()
	if ccveDir == "" {
		return fmt.Errorf("risk issue database not found")
	}

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

	fmt.Printf("\n%s%sAUTO-SUGGESTABLE RISK ISSUES%s\n\n", colorBold, colorCyan, colorReset)
	fmt.Printf("%-18s %6s %s\n", "REMEDY TYPE", "COUNT", "DESCRIPTION")
	fmt.Printf("%-18s %6s %s\n", "-----------", "-----", "-----------")
	fmt.Printf("%-18s %6d %s\n", "config_fix", counts["config_fix"], "kubectl apply/patch")
	fmt.Printf("%-18s %6d %s\n", "delete_resource", counts["delete_resource"], "kubectl delete")
	fmt.Printf("%-18s %6d %s\n", "trigger_action", counts["trigger_action"], "rollout restart/scale")
	fmt.Printf("%-18s %6d %s\n", "restart", counts["restart"], "pod/deployment restart")
	fmt.Printf("%-18s %6s %s\n", "-----------", "-----", "-----------")

	total := counts["config_fix"] + counts["trigger_action"] + counts["delete_resource"] + counts["restart"]
	fmt.Printf("%-18s %6d\n", "TOTAL", total)
	fmt.Printf("\n%scub-scout describes these patches; ConfigHub Pilot or kubectl applies, governed.%s\n", colorDim, colorReset)
	fmt.Printf("%sRun 'cub-scout suggest-remedy --all -n <namespace>' for findings in a namespace.%s\n\n",
		colorDim, colorReset)

	return nil
}

// CCVEDefinition represents a risk issue YAML file
type CCVEDefinition struct {
	ID         string   `yaml:"id"`
	Category   string   `yaml:"category"`
	Name       string   `yaml:"name"`
	Severity   string   `yaml:"severity"`
	Kind       string   `yaml:"-"` // Extracted from detection.resources
	RemedyType string   `yaml:"-"` // From remedy.type
	Commands   []string `yaml:"-"` // From remediation.commands
	Steps      []string `yaml:"-"` // From remediation.steps
}

func loadCCVE(id string) (*CCVEDefinition, error) {
	ccveDir := findCCVEDir()
	if ccveDir == "" {
		return nil, fmt.Errorf("risk issue database not found")
	}

	filename := filepath.Join(ccveDir, id+".yaml")
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", id, err)
	}

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

	if detection, ok := raw["detection"].(map[string]interface{}); ok {
		if resources, ok := detection["resources"].([]interface{}); ok && len(resources) > 0 {
			ccve.Kind = fmt.Sprintf("%v", resources[0])
		}
	}

	if remedyBlock, ok := raw["remedy"].(map[string]interface{}); ok {
		ccve.RemedyType = getString(remedyBlock, "type")
	}

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
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "..", "cve", "ccve")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	cwd, err := os.Getwd()
	if err == nil {
		dir := filepath.Join(cwd, "cve", "ccve")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}

	return ""
}

func printSuggestion(s *remedy.SuggestedRemedy) {
	fmt.Printf("\n%s%s=== SUGGESTED REMEDY ===%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("CCVE:       %s\n", s.Finding.CCVE)
	fmt.Printf("Resource:   %s\n", s.Finding.Resource.String())

	riskColor := colorGreen
	if s.RiskLevel == remedy.RiskMedium {
		riskColor = colorYellow
	} else if s.RiskLevel == remedy.RiskHigh {
		riskColor = colorRed
	}
	fmt.Printf("Risk Level: %s%s%s\n", riskColor, s.RiskLevel, colorReset)
	fmt.Printf("Reversible: %v\n", s.Reversible)

	fmt.Printf("\n%sSuggested actions (cub-scout will not run these):%s\n", colorBold, colorReset)
	for i, action := range s.Actions {
		fmt.Printf("  %d. %s\n", i+1, action.Description)
		fmt.Printf("     %s$ %s%s\n", colorDim, action.Command, colorReset)
	}
}

func printSuggestSummary(output *SuggestRemedyOutput) {
	fmt.Printf("\n%s%s=== SUGGESTED REMEDIES ===%s\n\n", colorBold, colorCyan, colorReset)

	for _, f := range output.Suggestions {
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
		fmt.Println()
	}

	fmt.Printf("Total:        %d\n", output.Summary.Total)
	fmt.Printf("Describable:  %d\n", output.Summary.Describable)
	fmt.Printf("Skipped:      %d\n", output.Summary.Skipped)
	fmt.Printf("\n%scub-scout produces evidence only. Apply via ConfigHub or kubectl, governed.%s\n\n",
		colorDim, colorReset)
}

func outputSuggestionJSON(s *remedy.SuggestedRemedy) error {
	output := SuggestedFindingOut{
		CCVE:       s.Finding.CCVE,
		Resource:   s.Finding.Resource.String(),
		Namespace:  s.Finding.Namespace,
		RemedyType: string(s.Finding.RemedyType),
		RiskLevel:  string(s.RiskLevel),
		Reversible: s.Reversible,
	}

	for _, action := range s.Actions {
		output.Actions = append(output.Actions, SuggestedActionOut{
			Description: action.Description,
			Command:     action.Command,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// validateFinding performs read-only safety checks before describing a
// suggestion: confirm the CCVE exists, the namespace exists, and the
// resource exists. Each check shells out to `kubectl get`, which is
// read-only.
func validateFinding(ctx context.Context, f *remedy.Finding) error {
	if _, err := loadCCVE(f.CCVE); err != nil {
		return fmt.Errorf("unknown CCVE: %s", f.CCVE)
	}

	if f.Namespace != "" {
		if err := checkNamespaceExists(ctx, f.Namespace); err != nil {
			return err
		}
	}

	if f.Resource.Name != "" {
		if err := checkResourceExists(ctx, f.Resource, f.Namespace); err != nil {
			return fmt.Errorf("resource not found: %v", err)
		}
	}

	return nil
}

func checkNamespaceExists(ctx context.Context, namespace string) error {
	cmd := fmt.Sprintf("kubectl get namespace %s -o name 2>/dev/null", namespace)
	out, err := execCommand(ctx, cmd)
	if err != nil || strings.TrimSpace(out) == "" {
		return fmt.Errorf("namespace %q not found", namespace)
	}
	return nil
}

func checkResourceExists(ctx context.Context, ref remedy.ResourceRef, namespace string) error {
	if ref.Kind == "" || ref.Name == "" {
		return nil
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

func execCommand(ctx context.Context, cmd string) (string, error) {
	out, err := exec.CommandContext(ctx, "sh", "-c", cmd).CombinedOutput()
	return string(out), err
}
