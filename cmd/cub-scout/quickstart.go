// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/spf13/cobra"
)

var (
	quickstartYes        bool
	quickstartNamespace  string
	quickstartTopFinding int
)

var quickstartCmd = &cobra.Command{
	Use:   "quickstart",
	Short: "Run a guided first-run tour on your cluster",
	Long: `quickstart walks through core cub-scout capabilities using your current cluster.

Flow:
  1) detect cluster access and context
  2) show doctor summary
  3) explain one representative workload
  4) show ownership snapshot
  5) highlight top finding
  6) show connected-mode preview when relevant
  7) print practical next steps
`,
	RunE: runQuickstart,
}

func init() {
	rootCmd.AddCommand(quickstartCmd)
	quickstartCmd.Flags().BoolVarP(&quickstartYes, "yes", "y", false, "Run non-interactively (skip prompts)")
	quickstartCmd.Flags().StringVarP(&quickstartNamespace, "namespace", "n", "", "Namespace scope (default: all namespaces)")
	quickstartCmd.Flags().IntVar(&quickstartTopFinding, "top-finding", 1, "Numbered top finding to highlight (1-based)")
}

type quickstartFixtureInput struct {
	ClusterConnected bool                     `json:"clusterConnected"`
	Context          string                   `json:"context"`
	ConnectedPreview bool                     `json:"connectedPreview"`
	Entries          []MapEntry               `json:"entries"`
	Findings         []scan.NormalizedFinding `json:"findings"`
}

type quickstartState struct {
	ClusterConnected bool
	Context          string
	ConnectedPreview bool
	Summary          DoctorSummary
	Entries          []MapEntry
	Findings         []scan.NormalizedFinding
	Representative   *MapEntry
	Explain          *ExplainSummary
}

func runQuickstart(cmd *cobra.Command, args []string) error {
	if quickstartTopFinding < 1 {
		return fmt.Errorf("--top-finding must be >= 1")
	}

	state, err := loadQuickstartState(cmd.Context())
	if err != nil {
		return err
	}

	renderQuickstart(os.Stdout, state, shouldQuickstartPrompt())
	return nil
}

func loadQuickstartState(ctx context.Context) (quickstartState, error) {
	if fixturePath := os.Getenv("CUB_SCOUT_TEST_QUICKSTART_JSON"); strings.TrimSpace(fixturePath) != "" {
		return loadQuickstartFromFixture(fixturePath)
	}

	connected, contextName := detectQuickstartCluster()
	if !connected {
		return quickstartState{ClusterConnected: false, Context: contextName}, nil
	}

	namespaceLabel := strings.TrimSpace(quickstartNamespace)
	if namespaceLabel == "" {
		namespaceLabel = "all"
	}

	entries, clusterName, err := collectDoctorEntries(ctx, quickstartNamespace)
	if err != nil {
		return quickstartState{}, err
	}
	findings, err := collectDoctorFindings(ctx, quickstartNamespace)
	if err != nil {
		findings = nil
	}

	summary := buildDoctorSummary(entries, findings, clusterName, namespaceLabel, 3)
	state := quickstartState{
		ClusterConnected: true,
		Context:          contextName,
		ConnectedPreview: detectQuickstartConnectedPreview(),
		Summary:          summary,
		Entries:          entries,
		Findings:         findings,
	}

	if rep, ok := pickQuickstartRepresentative(entries); ok {
		state.Representative = &rep
		explain := quickstartExplainForEntry(ctx, rep)
		state.Explain = &explain
	}

	return state, nil
}

func loadQuickstartFromFixture(path string) (quickstartState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return quickstartState{}, fmt.Errorf("read quickstart fixture: %w", err)
	}
	var in quickstartFixtureInput
	if err := json.Unmarshal(b, &in); err != nil {
		return quickstartState{}, fmt.Errorf("parse quickstart fixture: %w", err)
	}

	contextName := strings.TrimSpace(in.Context)
	if contextName == "" {
		contextName = getCurrentContext()
	}
	if !in.ClusterConnected {
		return quickstartState{ClusterConnected: false, Context: contextName}, nil
	}

	namespaceLabel := strings.TrimSpace(quickstartNamespace)
	if namespaceLabel == "" {
		namespaceLabel = "all"
	}

	summary := buildDoctorSummary(in.Entries, in.Findings, contextName, namespaceLabel, 3)
	state := quickstartState{
		ClusterConnected: true,
		Context:          contextName,
		ConnectedPreview: in.ConnectedPreview,
		Summary:          summary,
		Entries:          in.Entries,
		Findings:         in.Findings,
	}

	if rep, ok := pickQuickstartRepresentative(in.Entries); ok {
		state.Representative = &rep
		explain := buildQuickstartExplainFromEntry(rep)
		state.Explain = &explain
	}

	return state, nil
}

func detectQuickstartCluster() (bool, string) {
	contextName := getCurrentContext()
	if _, err := exec.LookPath("kubectl"); err != nil {
		return false, contextName
	}
	if err := exec.Command("kubectl", "cluster-info").Run(); err != nil {
		return false, contextName
	}
	return true, contextName
}

func detectQuickstartConnectedPreview() bool {
	if _, err := exec.LookPath("cub"); err != nil {
		return false
	}
	return exec.Command("cub", "context", "get").Run() == nil
}

func pickQuickstartRepresentative(entries []MapEntry) (MapEntry, bool) {
	if len(entries) == 0 {
		return MapEntry{}, false
	}

	priority := map[string]int{
		"Deployment":  1,
		"StatefulSet": 2,
		"DaemonSet":   3,
		"Pod":         4,
		"Service":     5,
	}

	bestIdx := -1
	bestRank := 999
	for i, e := range entries {
		rank, ok := priority[e.Kind]
		if !ok {
			rank = 500
		}
		if rank < bestRank {
			bestRank = rank
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return MapEntry{}, false
	}
	return entries[bestIdx], true
}

func quickstartExplainForEntry(ctx context.Context, entry MapEntry) ExplainSummary {
	if result, err := traceForExplain(ctx, entry.Kind, entry.Name, entry.Namespace); err == nil && result != nil {
		summary := buildExplainSummary(result)
		if summary.Resource == "" {
			summary.Resource = fmt.Sprintf("%s/%s", entry.Kind, entry.Name)
		}
		if summary.Namespace == "" {
			summary.Namespace = entry.Namespace
		}
		return summary
	}
	return buildQuickstartExplainFromEntry(entry)
}

func buildQuickstartExplainFromEntry(entry MapEntry) ExplainSummary {
	owner := strings.TrimSpace(entry.Owner)
	if owner == "" {
		owner = "Unknown"
	}
	health := strings.TrimSpace(entry.Status)
	if health == "" {
		health = "Unknown"
	}
	return ExplainSummary{
		Resource:    fmt.Sprintf("%s/%s", entry.Kind, entry.Name),
		Namespace:   entry.Namespace,
		Owner:       owner,
		Source:      "unknown",
		DeployedVia: fmt.Sprintf("%s/%s", entry.Kind, entry.Name),
		Health:      health,
		Risks:       "Not assessed",
		Drift:       "Unknown",
		Notes:       []string{"quickstart fallback: trace unavailable in this environment"},
	}
}

func renderQuickstart(w io.Writer, state quickstartState, interactive bool) {
	fmt.Fprintln(w, "Quickstart Wizard")
	fmt.Fprintln(w, "================")
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Step 1/7 - Cluster check\n")
	if !state.ClusterConnected {
		fmt.Fprintf(w, "No cluster connection detected (context: %s).\n", state.Context)
		fmt.Fprintln(w, "Run quickstart again after kubectl is connected, or use example bundle mode:")
		fmt.Fprintln(w, "  cub-scout map --bundle ./testdata/fixtures/bundle")
		fmt.Fprintln(w, "  cub-scout doctor")
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "Connected to context: %s\n\n", state.Context)
	quickstartPause(w, interactive)

	fmt.Fprintln(w, "Step 2/7 - Doctor Summary")
	fmt.Fprintf(w, "Doctor Summary: resources=%d healthy=%d warning=%d error=%d\n",
		state.Summary.Resources.Total,
		state.Summary.Health.Healthy,
		state.Summary.Health.Warning,
		state.Summary.Health.Error,
	)
	fmt.Fprintln(w)
	quickstartPause(w, interactive)

	fmt.Fprintln(w, "Step 3/7 - Explain representative workload")
	if state.Explain != nil {
		fmt.Fprintf(w, "Explain: %s\n", state.Explain.Resource)
		fmt.Fprintf(w, "  Owner: %s\n", state.Explain.Owner)
		fmt.Fprintf(w, "  Health: %s\n", state.Explain.Health)
	} else {
		fmt.Fprintln(w, "Explain: no representative workload found")
	}
	fmt.Fprintln(w)
	quickstartPause(w, interactive)

	fmt.Fprintln(w, "Step 4/7 - Ownership snapshot")
	printQuickstartOwnershipSnapshot(w, state.Entries)
	fmt.Fprintln(w)
	quickstartPause(w, interactive)

	fmt.Fprintln(w, "Step 5/7 - Top finding")
	printQuickstartTopFinding(w, state.Findings)
	fmt.Fprintln(w)
	quickstartPause(w, interactive)

	fmt.Fprintln(w, "Step 6/7 - Connected mode preview")
	if state.ConnectedPreview {
		fmt.Fprintln(w, "Connected mode preview available: import/fleet history features detected.")
		fmt.Fprintln(w, "Try: cub-scout import --dry-run -n <namespace>")
	} else {
		fmt.Fprintln(w, "Connected mode preview unavailable (cub CLI context not detected).")
	}
	fmt.Fprintln(w)
	quickstartPause(w, interactive)

	fmt.Fprintln(w, "Step 7/7 - Next steps")
	fmt.Fprintln(w, "Next steps:")
	fmt.Fprintln(w, "  1) cub-scout doctor")
	fmt.Fprintln(w, "  2) cub-scout explain <kind/name> -n <namespace>")
	fmt.Fprintln(w, "  3) cub-scout map list --json")
	fmt.Fprintln(w, "  4) cub-scout scan")
	if state.ConnectedPreview {
		fmt.Fprintln(w, "  5) cub-scout import --dry-run -n <namespace>")
	}
}

func printQuickstartOwnershipSnapshot(w io.Writer, entries []MapEntry) {
	counts := map[string]int{}
	for _, e := range entries {
		owner := strings.TrimSpace(e.Owner)
		if owner == "" {
			owner = "Unknown"
		}
		counts[owner]++
	}
	if len(counts) == 0 {
		fmt.Fprintln(w, "Ownership snapshot: no resources discovered")
		return
	}

	owners := make([]string, 0, len(counts))
	for owner := range counts {
		owners = append(owners, owner)
	}
	sort.Strings(owners)

	fmt.Fprintln(w, "Ownership snapshot:")
	for _, owner := range owners {
		fmt.Fprintf(w, "  - %s: %d\n", owner, counts[owner])
	}
}

func printQuickstartTopFinding(w io.Writer, findings []scan.NormalizedFinding) {
	if len(findings) == 0 {
		fmt.Fprintln(w, "Top finding: none (no findings returned)")
		return
	}

	sorted := append([]scan.NormalizedFinding(nil), findings...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri := quickstartSeverityRank(sorted[i].Severity)
		rj := quickstartSeverityRank(sorted[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if sorted[i].Namespace != sorted[j].Namespace {
			return sorted[i].Namespace < sorted[j].Namespace
		}
		return sorted[i].Resource < sorted[j].Resource
	})

	idx := quickstartTopFinding - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	f := sorted[idx]
	fmt.Fprintf(w, "Top finding: [%s] %s/%s - %s\n",
		strings.ToUpper(f.Severity),
		f.Namespace,
		f.Resource,
		f.Message,
	)
}

func quickstartSeverityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

func shouldQuickstartPrompt() bool {
	if quickstartYes {
		return false
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func quickstartPause(w io.Writer, interactive bool) {
	if !interactive {
		return
	}
	fmt.Fprint(w, "Press Enter to continue... ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Fprintln(w)
}
