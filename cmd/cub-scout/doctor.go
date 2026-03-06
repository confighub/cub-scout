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
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	doctorFormat    string
	doctorNamespace string
	doctorTopIssues int
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
}

// DoctorSummary is the canonical model behind both ASCII and JSON output.
type DoctorSummary struct {
	Cluster   string                 `json:"cluster"`
	Namespace string                 `json:"namespace"`
	Resources DoctorResourceSummary  `json:"resources"`
	Ownership DoctorOwnershipSummary `json:"ownership"`
	Health    DoctorHealthSummary    `json:"health"`
	Risks     DoctorRiskSummary      `json:"risks"`
	Drift     DoctorDriftSummary     `json:"drift"`
	TopIssues []DoctorIssue          `json:"topIssues,omitempty"`
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

	if doctorTopIssues < 0 {
		return fmt.Errorf("--top must be >= 0")
	}

	namespaceLabel := "all"
	if strings.TrimSpace(doctorNamespace) != "" {
		namespaceLabel = doctorNamespace
	}

	var (
		summary DoctorSummary
		err     error
	)

	if fixturePath := os.Getenv("CUB_SCOUT_TEST_DOCTOR_INPUT_JSON"); fixturePath != "" {
		summary, err = runDoctorFromFixture(fixturePath, namespaceLabel)
		if err != nil {
			return err
		}
	} else {
		summary, err = runDoctorFromCluster(cmd.Context(), namespaceLabel)
		if err != nil {
			return err
		}
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	default:
		fmt.Print(renderDoctorASCII(summary))
		return nil
	}
}

func runDoctorFromFixture(path, namespaceLabel string) (DoctorSummary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return DoctorSummary{}, fmt.Errorf("read doctor fixture: %w", err)
	}
	var in doctorFixtureInput
	if err := json.Unmarshal(b, &in); err != nil {
		return DoctorSummary{}, fmt.Errorf("parse doctor fixture: %w", err)
	}
	cluster := strings.TrimSpace(in.Cluster)
	if cluster == "" {
		cluster = getClusterName()
	}
	return buildDoctorSummary(in.Entries, in.Findings, cluster, namespaceLabel, doctorTopIssues), nil
}

func runDoctorFromCluster(ctx context.Context, namespaceLabel string) (DoctorSummary, error) {
	entries, cluster, err := collectDoctorEntries(ctx, doctorNamespace)
	if err != nil {
		return DoctorSummary{}, withKubeRecoveryHint(err, "cub-scout doctor")
	}

	findings, err := collectDoctorFindings(ctx, doctorNamespace)
	if err != nil {
		// Degrade gracefully if scanning is unavailable.
		fmt.Fprintf(os.Stderr, "Note: doctor risk scan unavailable: %v\n", err)
		findings = nil
	}

	return buildDoctorSummary(entries, findings, cluster, namespaceLabel, doctorTopIssues), nil
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

func renderDoctorASCII(summary DoctorSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Cluster: %s (namespace: %s)\n", summary.Cluster, summary.Namespace)
	fmt.Fprintf(&b, "Resources: %d total\n\n", summary.Resources.Total)

	total := summary.Resources.Total
	fmt.Fprintf(&b, "Ownership:\n")
	fmt.Fprintf(&b, "  Flux: %d (%d%%)\n", summary.Ownership.Flux, doctorPercent(summary.Ownership.Flux, total))
	fmt.Fprintf(&b, "  ArgoCD: %d (%d%%)\n", summary.Ownership.ArgoCD, doctorPercent(summary.Ownership.ArgoCD, total))
	fmt.Fprintf(&b, "  Helm: %d (%d%%)\n", summary.Ownership.Helm, doctorPercent(summary.Ownership.Helm, total))
	fmt.Fprintf(&b, "  Native: %d (%d%%)  <- %d unmanaged\n", summary.Ownership.Native, doctorPercent(summary.Ownership.Native, total), summary.Ownership.Unmanaged)
	if summary.Ownership.Other > 0 {
		fmt.Fprintf(&b, "  Other: %d (%d%%)\n", summary.Ownership.Other, doctorPercent(summary.Ownership.Other, total))
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "Health:\n")
	fmt.Fprintf(&b, "  Healthy: %d\n", summary.Health.Healthy)
	fmt.Fprintf(&b, "  Warning: %d\n", summary.Health.Warning)
	fmt.Fprintf(&b, "  Error: %d\n\n", summary.Health.Error)

	fmt.Fprintf(&b, "Risks: %d findings (%d CRITICAL, %d WARNING, %d INFO)\n",
		summary.Risks.Total, summary.Risks.Critical, summary.Risks.Warning, summary.Risks.Info)
	fmt.Fprintf(&b, "Drift: %d resources drifted from declared state\n\n", summary.Drift.Resources)

	fmt.Fprintf(&b, "Top Issues:\n")
	if len(summary.TopIssues) == 0 {
		fmt.Fprintf(&b, "  (none)\n")
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
			fmt.Fprintf(&b, "  %d. %s (ns: %s) - %s [%s]\n", i+1, issue.Resource, ns, msg, issue.Severity)
		}
	}

	hints := doctorTryNextHints(summary)
	if len(hints) > 0 {
		b.WriteString(renderTryNextSection(hints))
	}

	return b.String()
}

func doctorPercent(part, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(part) * 100.0 / float64(total))
}
