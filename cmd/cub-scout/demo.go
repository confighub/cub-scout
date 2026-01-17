// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	demoNoPods  bool
	demoCleanup bool
)

// Styles for demo output
var (
	demoTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	demoInfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	demoPassStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	demoWarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	demoErrStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	demoDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	demoBoldStyle    = lipgloss.NewStyle().Bold(true)
)

// Demo represents a runnable demo
type Demo struct {
	Name        string
	Description string
	Duration    string
	Standalone  bool
	Run         func() error
	Cleanup     func() error
}

// Available demos
var demos = map[string]Demo{
	"quick": {
		Name:        "quick",
		Description: "Fastest path to WOW - see Map in action",
		Duration:    "~30 sec",
		Standalone:  true,
		Run:         runDemoQuick,
		Cleanup:     cleanupDemoQuick,
	},
	"ccve": {
		Name:        "ccve",
		Description: "CCVE-2025-0027 detection - the BIGBANK story",
		Duration:    "~2 min",
		Standalone:  true,
		Run:         runDemoCCVE,
		Cleanup:     cleanupDemoCCVE,
	},
	"query": {
		Name:        "query",
		Description: "Query language demo - filter by owner, namespace",
		Duration:    "~1 min",
		Standalone:  true,
		Run:         runDemoQuery,
		Cleanup:     cleanupDemoQuery,
	},
}

// Scenarios (narrative demos)
var scenarios = map[string]Demo{
	"bigbank-incident": {
		Name:        "bigbank-incident",
		Description: "Walk through the BIGBANK 4-hour outage",
		Duration:    "~3 min",
		Standalone:  true,
		Run:         runScenarioBigbank,
		Cleanup:     cleanupDemoCCVE, // Same fixtures
	},
	"break-glass": {
		Name:        "break-glass",
		Description: "Emergency kubectl -> Accept/Reject workflow",
		Duration:    "~2 min",
		Standalone:  true,
		Run:         runScenarioBreakGlass,
		Cleanup:     cleanupScenarioBreakGlass,
	},
}

var demoCmd = &cobra.Command{
	Use:   "demo [name]",
	Short: "Run interactive demos",
	Long: `Run interactive demos to showcase cub-scout features.

Examples:
  cub-scout demo --list             # List available demos
  cub-scout demo quick              # Quick demo (~30 sec)
  cub-scout demo ccve               # CCVE-2025-0027 demo (~2 min)
  cub-scout demo query              # Query language demo
  cub-scout demo scenario bigbank   # Narrative scenario

  cub-scout demo quick --cleanup    # Remove demo resources`,
	Args: cobra.MaximumNArgs(2),
	RunE: runDemo,
}

var demoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available demos",
	RunE: func(cmd *cobra.Command, args []string) error {
		listDemos()
		return nil
	},
}

var demoScenarioCmd = &cobra.Command{
	Use:   "scenario [name]",
	Short: "Run a narrative scenario demo",
	Long: `Run a narrative scenario demo with guided walkthrough.

Available scenarios:
  bigbank-incident    Walk through the BIGBANK 4-hour outage
  break-glass         Emergency kubectl -> Accept/Reject workflow`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		scenario, ok := scenarios[name]
		if !ok {
			return fmt.Errorf("unknown scenario: %s\nRun 'cub-scout demo --list' to see available scenarios", name)
		}
		if demoCleanup {
			return scenario.Cleanup()
		}
		return scenario.Run()
	},
}

func init() {
	rootCmd.AddCommand(demoCmd)
	demoCmd.AddCommand(demoListCmd)
	demoCmd.AddCommand(demoScenarioCmd)

	demoCmd.Flags().BoolVar(&demoNoPods, "no-pods", false, "Apply without running pods (faster)")
	demoCmd.Flags().BoolVar(&demoCleanup, "cleanup", false, "Remove demo resources")
	demoScenarioCmd.Flags().BoolVar(&demoCleanup, "cleanup", false, "Remove scenario resources")
}

func runDemo(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		listDemos()
		return nil
	}

	name := args[0]

	// Handle "scenario" as first arg
	if name == "scenario" {
		if len(args) < 2 {
			return fmt.Errorf("scenario name required\nRun 'cub-scout demo --list' to see available scenarios")
		}
		scenarioName := args[1]
		scenario, ok := scenarios[scenarioName]
		if !ok {
			return fmt.Errorf("unknown scenario: %s", scenarioName)
		}
		if demoCleanup {
			return scenario.Cleanup()
		}
		return scenario.Run()
	}

	demo, ok := demos[name]
	if !ok {
		return fmt.Errorf("unknown demo: %s\nRun 'cub-scout demo --list' to see available demos", name)
	}

	if demoCleanup {
		return demo.Cleanup()
	}

	return demo.Run()
}

func listDemos() {
	fmt.Println()
	fmt.Println(demoBoldStyle.Render("Available Demos"))
	fmt.Println()
	fmt.Printf("  %-20s %-12s %s\n", demoBoldStyle.Render("NAME"), demoBoldStyle.Render("TIME"), demoBoldStyle.Render("DESCRIPTION"))
	fmt.Println("  " + strings.Repeat("─", 64))

	for _, name := range []string{"quick", "ccve", "query"} {
		d := demos[name]
		fmt.Printf("  %-20s %-12s %s\n", d.Name, d.Duration, d.Description)
	}

	fmt.Println()
	fmt.Println(demoBoldStyle.Render("Scenarios (Narrative Demos)"))
	fmt.Println()

	for _, name := range []string{"bigbank-incident", "break-glass"} {
		s := scenarios[name]
		fmt.Printf("  scenario %-14s %-12s %s\n", s.Name, s.Duration, s.Description)
	}

	fmt.Println()
	fmt.Println(demoDimStyle.Render("Usage: cub-scout demo <name>"))
	fmt.Println(demoDimStyle.Render("       cub-scout demo scenario <name>"))
	fmt.Println()
}

// Helper to get repo root
func getRepoRoot() string {
	// Try to find repo root by looking for go.mod
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Fallback to current dir
	return "."
}

// Apply a YAML file with kubectl
func kubectlApply(yamlPath string) error {
	cmd := exec.Command("kubectl", "apply", "-f", yamlPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Delete a YAML file with kubectl
func kubectlDelete(yamlPath string) error {
	cmd := exec.Command("kubectl", "delete", "-f", yamlPath, "--ignore-not-found")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Run a cub-scout subcommand
func runCubAgent(args ...string) error {
	// Find the cub-scout binary
	binary, err := os.Executable()
	if err != nil {
		binary = "cub-scout"
	}

	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ============================================================================
// DEMO: quick
// ============================================================================

func runDemoQuick() error {
	repoRoot := getRepoRoot()
	fixturesDir := filepath.Join(repoRoot, "test", "atk", "fixtures")

	fmt.Println(demoBoldStyle.Render("Quick Demo: See your cluster in 30 seconds"))
	fmt.Println()

	fmt.Println(demoInfoStyle.Render("Applying minimal demo fixtures..."))
	_ = kubectlApply(filepath.Join(fixturesDir, "flux-basic.yaml"))
	_ = kubectlApply(filepath.Join(fixturesDir, "argo-basic.yaml"))

	fmt.Println()
	fmt.Println(demoBoldStyle.Render("Running: cub-scout map status"))
	_ = runCubAgent("map", "status")
	fmt.Println()

	fmt.Println(demoBoldStyle.Render("Running: cub-scout map list"))
	_ = runCubAgent("map", "list")
	fmt.Println()

	fmt.Println(demoBoldStyle.Render("Running: cub-scout map issues"))
	_ = runCubAgent("map", "issues")
	fmt.Println()

	fmt.Println(demoPassStyle.Render("That's the Map - your cluster at a glance."))
	fmt.Println()
	fmt.Println(demoDimStyle.Render("Next: cub-scout scan to find config issues"))
	fmt.Println(demoDimStyle.Render("      cub-scout demo ccve to see CCVE-2025-0027 (the BIGBANK incident)"))

	return nil
}

func cleanupDemoQuick() error {
	repoRoot := getRepoRoot()
	fixturesDir := filepath.Join(repoRoot, "test", "atk", "fixtures")

	fmt.Println(demoInfoStyle.Render("Cleaning up quick demo resources..."))
	_ = kubectlDelete(filepath.Join(fixturesDir, "flux-basic.yaml"))
	_ = kubectlDelete(filepath.Join(fixturesDir, "argo-basic.yaml"))
	fmt.Println(demoPassStyle.Render("Cleanup complete."))

	return nil
}

// ============================================================================
// DEMO: ccve
// ============================================================================

func runDemoCCVE() error {
	repoRoot := getRepoRoot()
	badConfigPath := filepath.Join(repoRoot, "examples", "impressive-demo", "bad-configs", "monitoring-bad.yaml")

	fmt.Println(demoBoldStyle.Render("CCVE-2025-0027 Demo: The BIGBANK 4-Hour Outage"))
	fmt.Println()
	fmt.Println(demoDimStyle.Render("This exact bug caused a 4-hour outage at BIGBANK Capital Markets (FluxCon 2025)"))
	fmt.Println()

	fmt.Println(demoInfoStyle.Render("Step 1: Deploying Grafana with the bug..."))
	_ = kubectlApply(badConfigPath)

	fmt.Println()
	fmt.Println(demoInfoStyle.Render("Step 2: Running CCVE scan..."))
	time.Sleep(2 * time.Second)
	fmt.Println()
	_ = runCubAgent("scan")

	fmt.Println()
	fmt.Println(demoBoldStyle.Render("CCVE-2025-0027 detected in seconds"))
	fmt.Println(demoDimStyle.Render("Without ConfigHub: 4 hours of debugging sidecar logs"))
	fmt.Println(demoDimStyle.Render("With ConfigHub: Instant detection + fix command"))
	fmt.Println()

	fmt.Println("To fix:")
	fmt.Println("  kubectl set env deployment/grafana -n monitoring \\")
	fmt.Println("    NAMESPACE=\"monitoring,grafana,observability\"")
	fmt.Println()
	fmt.Println(demoDimStyle.Render("Cleanup: cub-scout demo ccve --cleanup"))

	return nil
}

func cleanupDemoCCVE() error {
	repoRoot := getRepoRoot()
	badConfigPath := filepath.Join(repoRoot, "examples", "impressive-demo", "bad-configs", "monitoring-bad.yaml")

	fmt.Println(demoInfoStyle.Render("Cleaning up CCVE demo resources..."))
	_ = kubectlDelete(badConfigPath)
	fmt.Println(demoPassStyle.Render("Cleanup complete."))

	return nil
}

// ============================================================================
// DEMO: query
// ============================================================================

func runDemoQuery() error {
	repoRoot := getRepoRoot()
	multiClusterPath := filepath.Join(repoRoot, "examples", "demos", "multi-cluster.yaml")

	fmt.Println(demoBoldStyle.Render("Query Language Demo: Filter your fleet"))
	fmt.Println()
	fmt.Println(demoDimStyle.Render("Query language for precise resource filtering"))
	fmt.Println()

	fmt.Println(demoInfoStyle.Render("Step 1: Applying multi-cluster demo fixtures..."))
	_ = kubectlApply(multiClusterPath)
	time.Sleep(2 * time.Second)

	fmt.Println()
	fmt.Println(demoBoldStyle.Render("Query 1: GitOps-managed deployments only (owner!=Native)"))
	fmt.Println(demoDimStyle.Render("  $ cub-scout map list -q \"kind=Deployment AND owner!=Native\""))
	fmt.Println()
	_ = runCubAgent("map", "list", "-q", "kind=Deployment AND owner!=Native")

	fmt.Println()
	fmt.Println(demoBoldStyle.Render("Query 2: Production namespaces (namespace=prod*)"))
	fmt.Println(demoDimStyle.Render("  $ cub-scout map list -q \"namespace=prod*\""))
	fmt.Println()
	_ = runCubAgent("map", "list", "-q", "namespace=prod*")

	fmt.Println()
	fmt.Println(demoBoldStyle.Render("Query 3: Flux OR ArgoCD managed"))
	fmt.Println(demoDimStyle.Render("  $ cub-scout map list -q \"owner=Flux OR owner=ArgoCD\""))
	fmt.Println()
	_ = runCubAgent("map", "list", "-q", "owner=Flux OR owner=ArgoCD")

	fmt.Println()
	fmt.Println(demoBoldStyle.Render("Query 4: Find orphans (Native ownership)"))
	fmt.Println(demoDimStyle.Render("  $ cub-scout map list -q \"owner=Native\""))
	fmt.Println()
	_ = runCubAgent("map", "list", "-q", "owner=Native")

	fmt.Println()
	fmt.Println(demoPassStyle.Render("Query language enables precise fleet filtering"))
	fmt.Println()
	fmt.Println(demoBoldStyle.Render("Query Syntax:"))
	fmt.Println("  field=value           Exact match (case-insensitive)")
	fmt.Println("  field!=value          Not equal")
	fmt.Println("  field~=pattern        Regex match")
	fmt.Println("  field=val1,val2       IN list")
	fmt.Println("  field=prefix*         Wildcard")
	fmt.Println("  AND / OR              Logical operators")
	fmt.Println()
	fmt.Println("Available fields: kind, namespace, name, owner, cluster, labels[key]")
	fmt.Println()
	fmt.Println(demoDimStyle.Render("Cleanup: cub-scout demo query --cleanup"))

	return nil
}

func cleanupDemoQuery() error {
	repoRoot := getRepoRoot()
	multiClusterPath := filepath.Join(repoRoot, "examples", "demos", "multi-cluster.yaml")

	fmt.Println(demoInfoStyle.Render("Cleaning up query demo resources..."))
	_ = kubectlDelete(multiClusterPath)
	fmt.Println(demoPassStyle.Render("Cleanup complete."))

	return nil
}

// ============================================================================
// SCENARIO: bigbank-incident
// ============================================================================

func runScenarioBigbank() error {
	repoRoot := getRepoRoot()
	badConfigPath := filepath.Join(repoRoot, "examples", "impressive-demo", "bad-configs", "monitoring-bad.yaml")

	fmt.Println(demoBoldStyle.Render("Scenario: The BIGBANK 4-Hour Outage"))
	fmt.Println()
	fmt.Println(demoDimStyle.Render("Based on the real incident shared at FluxCon 2025"))
	fmt.Println()

	fmt.Println(demoBoldStyle.Render("Day 1, 2:00 AM:") + " On-call gets paged")
	fmt.Println("  'Grafana dashboards aren't loading'")
	fmt.Println()
	time.Sleep(2 * time.Second)

	fmt.Println(demoBoldStyle.Render("Day 1, 2:30 AM:") + " First investigation")
	fmt.Println("  - Grafana pod is running")
	fmt.Println("  - No errors in main logs")
	fmt.Println("  - Network looks fine")
	fmt.Println()
	time.Sleep(2 * time.Second)

	fmt.Println(demoBoldStyle.Render("Day 1, 4:00 AM:") + " Deeper debugging")
	fmt.Println("  - Check sidecar container...")
	fmt.Println("  - Finally find the k8s-sidecar issue")
	fmt.Println()
	time.Sleep(2 * time.Second)

	fmt.Println(demoBoldStyle.Render("Day 1, 6:00 AM:") + " Root cause identified")
	fmt.Println("  NAMESPACE env var has whitespace: 'monitoring ' (trailing space)")
	fmt.Println("  k8s-sidecar silently fails to load dashboards")
	fmt.Println()
	time.Sleep(2 * time.Second)

	fmt.Println(demoWarnStyle.Render("Total time to resolution: 4 hours"))
	fmt.Println()

	fmt.Println(demoInfoStyle.Render("Now let's see how ConfigHub catches this instantly..."))
	fmt.Println()

	_ = kubectlApply(badConfigPath)
	time.Sleep(1 * time.Second)

	fmt.Println(demoBoldStyle.Render("Running: cub-scout scan"))
	fmt.Println()
	_ = runCubAgent("scan")

	fmt.Println()
	fmt.Println(demoPassStyle.Render("CCVE-2025-0027 detected in seconds"))
	fmt.Println(demoDimStyle.Render("30 seconds vs 4 hours"))
	fmt.Println()
	fmt.Println(demoDimStyle.Render("Cleanup: cub-scout demo scenario bigbank-incident --cleanup"))

	return nil
}

// ============================================================================
// SCENARIO: break-glass
// ============================================================================

func runScenarioBreakGlass() error {
	repoRoot := getRepoRoot()
	fixturePath := filepath.Join(repoRoot, "examples", "demos", "break-glass.yaml")

	fmt.Println(demoBoldStyle.Render("Scenario: Break-Glass Emergency Access"))
	fmt.Println()
	fmt.Println(demoDimStyle.Render("Production incident requires emergency kubectl apply"))
	fmt.Println()

	// Timeline
	fmt.Println(demoBoldStyle.Render("14:00") + " - payment-api deployed via Flux (Rev 42)")
	fmt.Println(demoBoldStyle.Render("14:15") + " - " + demoErrStyle.Render("ALERT:") + " Payment errors > 5%")
	fmt.Println(demoBoldStyle.Render("14:18") + " - INC-4521 opened")
	fmt.Println(demoBoldStyle.Render("14:23") + " - " + demoWarnStyle.Render("BREAK-GLASS:") + " hotfix-cache deployed via kubectl")
	fmt.Println(demoBoldStyle.Render("14:25") + " - Errors stabilizing")
	fmt.Println(demoBoldStyle.Render("14:35") + " - Incident mitigated")
	fmt.Println()
	time.Sleep(2 * time.Second)

	fmt.Println(demoInfoStyle.Render("Applying break-glass scenario fixtures..."))
	_ = kubectlApply(fixturePath)
	time.Sleep(2 * time.Second)

	fmt.Println()
	fmt.Println(demoBoldStyle.Render("What Map sees:"))
	fmt.Println()
	_ = runCubAgent("map", "list", "-n", "break-glass-demo")

	fmt.Println()
	fmt.Println(demoBoldStyle.Render("Finding the orphan:"))
	fmt.Println()
	_ = runCubAgent("map", "orphans")

	fmt.Println()
	fmt.Println(demoWarnStyle.Render("Decision needed:"))
	fmt.Println("  " + demoPassStyle.Render("ACCEPT") + " - Import hotfix-cache to ConfigHub (versioned, repeatable)")
	fmt.Println("  " + demoErrStyle.Render("REJECT") + " - Delete hotfix-cache (restore pure GitOps state)")
	fmt.Println()
	fmt.Println(demoDimStyle.Render("With ConfigHub: cub-scout import to bring break-glass resources under management"))
	fmt.Println()
	fmt.Println(demoDimStyle.Render("Cleanup: cub-scout demo scenario break-glass --cleanup"))

	return nil
}

func cleanupScenarioBreakGlass() error {
	repoRoot := getRepoRoot()
	fixturePath := filepath.Join(repoRoot, "examples", "demos", "break-glass.yaml")

	fmt.Println(demoInfoStyle.Render("Cleaning up break-glass scenario resources..."))
	_ = kubectlDelete(fixturePath)

	// Also delete namespace
	cmd := exec.Command("kubectl", "delete", "namespace", "break-glass-demo", "--ignore-not-found")
	_ = cmd.Run()

	fmt.Println(demoPassStyle.Render("Cleanup complete."))

	return nil
}
