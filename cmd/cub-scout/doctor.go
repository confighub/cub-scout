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

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/confighub/cub-scout/pkg/hub"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	doctorFormat       string
	doctorNamespace    string
	doctorTopIssues    int
	doctorPresentation string
	doctorHintMode     string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Show a single-command cluster health summary",
	Long: `doctor provides a compact, actionable summary of cluster state.

It combines ownership, health, risk, and drift signals into one view.

Examples:
  cub-scout doctor
  cub-scout doctor --namespace prod
  cub-scout doctor --format json
`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().StringVarP(&doctorNamespace, "namespace", "n", "", "Namespace scope (default: all namespaces)")
	doctorCmd.Flags().StringVar(&doctorFormat, "format", "ascii", "Output format: ascii, json")
	doctorCmd.Flags().IntVar(&doctorTopIssues, "top", 3, "Number of top issues to include")
	doctorCmd.Flags().StringVar(&doctorPresentation, "presentation", "", PresentationModeHelp())
	doctorCmd.Flags().StringVar(&doctorHintMode, "hint-mode", "", HintModeHelp())
}

// DoctorSummary is the canonical model behind both ASCII and JSON output.
type DoctorSummary struct {
	Cluster   string                  `json:"cluster"`
	Namespace string                  `json:"namespace"`
	Resources DoctorResourceSummary   `json:"resources"`
	Ownership DoctorOwnershipSummary  `json:"ownership"`
	Health    DoctorHealthSummary     `json:"health"`
	Risks     DoctorRiskSummary       `json:"risks"`
	Drift     DoctorDriftSummary      `json:"drift"`
	ThreeWay  *DoctorThreeWaySummary  `json:"threeWay,omitempty"`
	TopIssues []DoctorIssue           `json:"topIssues,omitempty"`
	NextSteps []StructuredHint        `json:"nextSteps,omitempty"` // Structured action-typed hints for AI/MCP
}

// DoctorThreeWaySummary indicates three-way comparison status.
// In connected mode, this surfaces whether ConfigHub/Argo/cluster agree.
type DoctorThreeWaySummary struct {
	Available bool   `json:"available"`          // True if connected mode is available
	Hint      string `json:"hint,omitempty"`     // Suggested command for full comparison
}

// DoctorResourceSummary contains resource inventory totals.
type DoctorResourceSummary struct {
	Total int `json:"total"`
}

// DoctorOwnershipSummary contains ownership counts.
type DoctorOwnershipSummary struct {
	Flux      int `json:"flux"`
	ArgoCD    int `json:"argocd"`
	Helm      int `json:"helm"`
	Native    int `json:"native"`
	Other     int `json:"other"`
	Unmanaged int `json:"unmanaged"`
}

// DoctorHealthSummary contains health band counts.
type DoctorHealthSummary struct {
	Healthy int `json:"healthy"`
	Warning int `json:"warning"`
	Error   int `json:"error"`
}

// DoctorRiskSummary contains risk finding counts by severity.
type DoctorRiskSummary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

// DoctorDriftSummary contains drift signal counts.
type DoctorDriftSummary struct {
	Resources int `json:"resources"`
}

// DoctorIssue is a concise issue entry for doctor output.
type DoctorIssue struct {
	Severity  string `json:"severity"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Message   string `json:"message"`
}

type doctorFixtureInput struct {
	Cluster   string                   `json:"cluster"`
	Namespace string                   `json:"namespace"`
	Entries   []MapEntry               `json:"entries"`
	Findings  []scan.NormalizedFinding `json:"findings"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(doctorFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", doctorFormat)
	}

	// Build invocation context with presentation mode resolution
	invCtx, err := NewInvocationContext(doctorPresentation, TransportCLI)
	if err != nil {
		return err
	}

	// Parse hint mode (separate from presentation mode)
	hintMode, err := ParseHintMode(doctorHintMode)
	if err != nil {
		return err
	}
	hintCtx := HintContext{Mode: hintMode}

	if doctorTopIssues < 0 {
		return fmt.Errorf("--top must be >= 0")
	}

	// Call the shared capability seam
	// Fixture path is passed explicitly rather than read inside the seam
	fixturePath := os.Getenv("CUB_SCOUT_TEST_DOCTOR_INPUT_JSON")
	result, err := ObserveScopeSummary(cmd.Context(), ObserveScopeSummaryRequest{
		Namespace:   doctorNamespace,
		TopIssues:   doctorTopIssues,
		FixturePath: fixturePath,
	})
	if err != nil {
		// Only apply kube recovery hints for cluster-path errors, not fixture errors
		if fixturePath == "" {
			return withKubeRecoveryHint(err, "cub-scout doctor")
		}
		return err
	}

	// Print any warnings from the seam (CLI-specific concern)
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Note: %s\n", w)
	}

	summary := result.Summary

	switch format {
	case "json":
		// Populate structured hints for JSON output (reuses existing hint logic)
		hints := doctorHintsWithContext(summary, hintCtx)
		sortHints(hints)
		if len(hints) > 3 {
			hints = hints[:3]
		}
		summary.NextSteps = HintsToStructured(hints)

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	default:
		// Use invocation context for presentation mode, hint context for recommendations
		fmt.Print(renderDoctorASCII(summary, invCtx.Mode(), invCtx.IsExplicit(), hintCtx))
		return nil
	}
}

func collectDoctorEntries(ctx context.Context, namespace string) ([]MapEntry, string, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, "", fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("create dynamic client: %w", err)
	}

	clusterName := getClusterName()
	entries := []MapEntry{}
	byOwner := map[string]int{}

	resources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
	}

	for _, gvr := range resources {
		if namespace != "" {
			l, err := dynClient.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
			if err != nil {
				continue
			}
			for _, item := range l.Items {
				itemCopy := item
				entries = processResource(&itemCopy, gvr, clusterName, entries, byOwner)
			}
		} else {
			l, err := dynClient.Resource(gvr).List(ctx, v1.ListOptions{})
			if err != nil {
				continue
			}
			for _, item := range l.Items {
				itemCopy := item
				entries = processResource(&itemCopy, gvr, clusterName, entries, byOwner)
			}
		}
	}

	return entries, clusterName, nil
}

func collectDoctorFindings(ctx context.Context, namespace string) ([]scan.NormalizedFinding, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubernetes config: %w", err)
	}

	provider := scan.SelectProvider(scan.ProviderConfig{})
	threshold, _ := time.ParseDuration("5m")

	result, err := provider.ScanCluster(ctx, scan.ClusterScanOpts{
		Config:     cfg,
		Namespace:  namespace,
		RunKyverno: true,
		RunState:   true,
		Threshold:  threshold,
	})
	if err != nil {
		return nil, err
	}

	normalized := scan.Normalize(result)
	if normalized == nil {
		return nil, nil
	}
	return normalized.Findings, nil
}

func buildDoctorSummary(entries []MapEntry, findings []scan.NormalizedFinding, cluster, namespace string, topN int) DoctorSummary {
	summary := DoctorSummary{
		Cluster:   cluster,
		Namespace: namespace,
		Resources: DoctorResourceSummary{Total: len(entries)},
	}

	for _, e := range entries {
		switch e.Owner {
		case "Flux":
			summary.Ownership.Flux++
		case "ArgoCD":
			summary.Ownership.ArgoCD++
		case "Helm":
			summary.Ownership.Helm++
		case "Native":
			summary.Ownership.Native++
		default:
			summary.Ownership.Other++
		}

		switch e.Status {
		case "Ready":
			summary.Health.Healthy++
		case "Failed":
			summary.Health.Error++
		default:
			summary.Health.Warning++
		}

		if strings.EqualFold(e.Status, "Drifted") || (e.Kind == "Application" && e.Status != "Ready") {
			summary.Drift.Resources++
		}
	}

	summary.Ownership.Unmanaged = summary.Ownership.Native

	issues := make([]DoctorIssue, 0, len(findings))
	for _, f := range findings {
		sev := strings.ToLower(strings.TrimSpace(f.Severity))
		summary.Risks.Total++
		switch sev {
		case "critical":
			summary.Risks.Critical++
		case "warning":
			summary.Risks.Warning++
		default:
			summary.Risks.Info++
			if sev == "" {
				sev = "info"
			}
		}

		issues = append(issues, DoctorIssue{
			Severity:  strings.ToUpper(sev),
			Resource:  strings.TrimSpace(f.Resource),
			Namespace: strings.TrimSpace(f.Namespace),
			Message:   strings.TrimSpace(f.Message),
		})
	}

	sort.Slice(issues, func(i, j int) bool {
		ri := doctorSeverityRank(issues[i].Severity)
		rj := doctorSeverityRank(issues[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if issues[i].Namespace != issues[j].Namespace {
			return issues[i].Namespace < issues[j].Namespace
		}
		if issues[i].Resource != issues[j].Resource {
			return issues[i].Resource < issues[j].Resource
		}
		return issues[i].Message < issues[j].Message
	})

	if topN < 0 {
		topN = 0
	}
	if topN > len(issues) {
		topN = len(issues)
	}
	summary.TopIssues = issues[:topN]

	// Check for connected mode to surface three-way comparison capability
	client := hub.NewClient()
	if err := client.RequireConnected(); err == nil {
		nsFlag := ""
		if namespace != "" && namespace != "all" {
			nsFlag = fmt.Sprintf(" --scope namespace/%s", namespace)
		} else {
			nsFlag = " --scope cluster"
		}
		summary.ThreeWay = &DoctorThreeWaySummary{
			Available: true,
			Hint:      fmt.Sprintf("cub-scout compare three-way%s", nsFlag),
		}
	}

	return summary
}

func doctorSeverityRank(sev string) int {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "CRITICAL":
		return 0
	case "WARNING":
		return 1
	case "INFO":
		return 2
	default:
		return 3
	}
}

func renderDoctorASCII(summary DoctorSummary, mode PresentationMode, explicitMode bool, hintCtx HintContext) string {
	var b strings.Builder

	// Helper to render section label based on whether presentation mode was explicit
	sectionLabel := func(label string) string {
		if explicitMode {
			return SectionLabel(mode, label)
		}
		return SectionHeader(label) + ":"
	}

	// Only apply presentation framing when explicitly requested
	if explicitMode {
		// Heading - varies by presentation mode
		heading := DoctorHeading(mode)
		if mode == PresentationAI {
			fmt.Fprintf(&b, "%s\n", heading)
		} else {
			fmt.Fprintf(&b, "%s\n", Bold(heading))
		}

		// Intro - varies by presentation mode
		intro := DoctorIntro(mode, summary.Cluster, summary.Namespace)
		fmt.Fprintf(&b, "%s\n", intro)
		fmt.Fprintf(&b, "%s %d total\n\n", SectionLabel(mode, "Resources"), summary.Resources.Total)
	} else {
		// Legacy format - no heading, just cluster line
		fmt.Fprintf(&b, "%s: %s (namespace: %s)\n", Bold("Cluster"), summary.Cluster, summary.Namespace)
		fmt.Fprintf(&b, "%s: %d total\n\n", Bold("Resources"), summary.Resources.Total)
	}

	total := summary.Resources.Total
	fmt.Fprintf(&b, "%s\n", sectionLabel("Ownership"))
	fmt.Fprintf(&b, "  %s: %d (%d%%)\n", OwnerColor("Flux"), summary.Ownership.Flux, doctorPercent(summary.Ownership.Flux, total))
	fmt.Fprintf(&b, "  %s: %d (%d%%)\n", OwnerColor("ArgoCD"), summary.Ownership.ArgoCD, doctorPercent(summary.Ownership.ArgoCD, total))
	fmt.Fprintf(&b, "  %s: %d (%d%%)\n", OwnerColor("Helm"), summary.Ownership.Helm, doctorPercent(summary.Ownership.Helm, total))
	unmanagedText := fmt.Sprintf("%d unmanaged", summary.Ownership.Unmanaged)
	if summary.Ownership.Unmanaged > 0 {
		unmanagedText = Yellow(unmanagedText)
	}
	fmt.Fprintf(&b, "  %s: %d (%d%%)  <- %s\n", OwnerColor("Native"), summary.Ownership.Native, doctorPercent(summary.Ownership.Native, total), unmanagedText)
	if summary.Ownership.Other > 0 {
		fmt.Fprintf(&b, "  Other: %d (%d%%)\n", summary.Ownership.Other, doctorPercent(summary.Ownership.Other, total))
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "%s\n", sectionLabel("Health"))
	fmt.Fprintf(&b, "  %s: %d\n", Green("Healthy"), summary.Health.Healthy)
	fmt.Fprintf(&b, "  %s: %d\n", Yellow("Warning"), summary.Health.Warning)
	fmt.Fprintf(&b, "  %s: %d\n\n", Red("Error"), summary.Health.Error)

	// Color severity counts in the risks line
	criticalText := fmt.Sprintf("%d CRITICAL", summary.Risks.Critical)
	warningText := fmt.Sprintf("%d WARNING", summary.Risks.Warning)
	infoText := fmt.Sprintf("%d INFO", summary.Risks.Info)
	if summary.Risks.Critical > 0 {
		criticalText = BoldRed(criticalText)
	}
	if summary.Risks.Warning > 0 {
		warningText = Yellow(warningText)
	}
	fmt.Fprintf(&b, "%s %d findings (%s, %s, %s)\n",
		sectionLabel("Risks"), summary.Risks.Total, criticalText, warningText, infoText)

	driftText := fmt.Sprintf("%d resources drifted from declared state", summary.Drift.Resources)
	if summary.Drift.Resources > 0 {
		driftText = Yellow(driftText)
	}
	fmt.Fprintf(&b, "%s %s\n\n", sectionLabel("Drift"), driftText)

	// Three-way status (connected mode only)
	if summary.ThreeWay != nil && summary.ThreeWay.Available {
		fmt.Fprintf(&b, "%s ConfigHub connected - run %s for full comparison\n\n",
			sectionLabel("Three-Way"), Cyan(summary.ThreeWay.Hint))
	}

	fmt.Fprintf(&b, "%s\n", sectionLabel("Top Issues"))
	if len(summary.TopIssues) == 0 {
		fmt.Fprintf(&b, "  %s\n", Dim("(none)"))
	} else {
		for i, issue := range summary.TopIssues {
			ns := issue.Namespace
			if ns == "" {
				ns = "-"
			}
			msg := issue.Message
			if msg == "" {
				msg = "no details"
			}
			severityColored := SeverityColor(issue.Severity)
			fmt.Fprintf(&b, "  %d. %s (ns: %s) - %s [%s]\n", i+1, issue.Resource, ns, msg, severityColored)
		}
	}

	// Outro - only for explicit AI mode
	if explicitMode {
		outro := DoctorOutro(mode)
		if outro != "" {
			fmt.Fprintf(&b, "\n%s\n", outro)
		}
	}

	hints := doctorTryNextHintsWithContext(summary, hintCtx)
	if len(hints) > 0 {
		if explicitMode {
			b.WriteString(renderTryNextSectionWithMode(hints, mode))
		} else {
			b.WriteString(renderTryNextSection(hints))
		}
	}

	return b.String()
}

func doctorPercent(part, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(part) * 100.0 / float64(total))
}
