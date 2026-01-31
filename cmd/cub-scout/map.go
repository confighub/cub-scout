// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/confighub/cub-scout/internal/mapsvc"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/queries"
	"github.com/confighub/cub-scout/pkg/query"
)

var (
	mapNamespace      string
	mapKind           string
	mapOwner          string
	mapQuery          string
	mapJSON           bool
	mapVerbose        bool
	mapHub            bool   // --hub flag for ConfigHub hierarchy
	mapSince          string // --since flag for time filtering
	mapCount          bool   // --count flag for count-only output
	mapNamesOnly      bool   // --names-only flag for names-only output
	mapExplain        bool   // --explain flag for learning mode
	deepDiveConnected bool   // --connected flag for ConfigHub integration in deep-dive
)

// MapEntry is an alias for mapsvc.Entry representing a resource in the fleet map.
// This alias maintains backward compatibility with existing code.
type MapEntry = mapsvc.Entry

// displayOwner delegates to mapsvc.DisplayOwner for canonical display names.
func displayOwner(owner string) string {
	return mapsvc.DisplayOwner(owner)
}

var mapCmd = &cobra.Command{
	Use:   "map",
	Short: "Interactive map of resources and ownership",
	Long: `Query and explore Kubernetes resources, their ownership, and relationships.

When run without a subcommand, launches the interactive TUI dashboard.
Use 'map list' for plain text output suitable for scripting.

INTERACTIVE MODE (default):
  cub-scout map              # Local cluster TUI (no auth needed)
  cub-scout map --hub        # ConfigHub hierarchy TUI (requires cub auth)
  cub-scout map hub          # Same as --hub

PLAIN TEXT MODE:
  cub-scout map list         # Scriptable output
  cub-scout map list -q "owner=Native"   # Query filter

Local cluster mode reads from your current kubectl context.
Hub mode requires ConfigHub authentication (cub auth login).
`,
	RunE: runMapTUI,
}

// runMapTUI launches the interactive TUI dashboard
func runMapTUI(cmd *cobra.Command, args []string) error {
	// If --hub flag is set, start with ConfigHub hierarchy TUI
	if mapHub {
		return runHierarchyWithSwitch(cmd, args)
	}

	// Start with local cluster TUI (Go-native)
	return runLocalClusterWithSwitch()
}

// runLocalClusterWithSwitch runs the local cluster TUI and handles mode switching
func runLocalClusterWithSwitch() error {
	for {
		switchToHub, hubContext, switchToImport, err := runLocalClusterTUI()
		if err != nil {
			return err
		}

		// Handle import wizard switch
		if switchToImport {
			if err := RunImportWizard(); err != nil {
				return err
			}
			// After import wizard, return to local cluster TUI
			continue
		}

		if !switchToHub {
			// User quit without switching modes
			return nil
		}

		// User wants to switch to ConfigHub mode - pass context
		switchToLocal, err := runHierarchyLoopWithContext(hubContext)
		if err != nil {
			return err
		}

		if !switchToLocal {
			// User quit from hierarchy mode
			return nil
		}

		// User wants to switch back to local cluster mode - loop continues
	}
}

// runHierarchyWithSwitch runs hierarchy TUI and handles mode switching
func runHierarchyWithSwitch(cmd *cobra.Command, args []string) error {
	for {
		// Start without context (user explicitly chose --hub)
		switchToLocal, err := runHierarchyLoopWithContext("")
		if err != nil {
			return err
		}

		if !switchToLocal {
			// User quit without switching modes
			return nil
		}

		// User wants to switch to local cluster mode
		switchToHub, hubContext, switchToImport, err := runLocalClusterTUI()
		if err != nil {
			return err
		}

		// Handle import wizard switch
		if switchToImport {
			if err := RunImportWizard(); err != nil {
				return err
			}
			// After import wizard, stay in local cluster mode
			continue
		}

		if !switchToHub {
			// User quit from local cluster mode
			return nil
		}

		// User wants to switch back to hub mode with context - but we came from hub
		// so use context if available
		_ = hubContext // Will use in next iteration if needed
	}
}

// runHierarchyLoopWithContext runs the hierarchy TUI with optional app context
// If appContext is provided, starts in Maps view filtered to that app
func runHierarchyLoopWithContext(appContext string) (bool, error) {
	if _, err := exec.LookPath("cub"); err != nil {
		return false, fmt.Errorf("cub CLI not found. Install from: https://docs.confighub.com/cli")
	}

	if _, err := runCubCommand("context", "get"); err != nil {
		return false, fmt.Errorf("ConfigHub authentication required for --hub mode.\n\n  To authenticate: cub auth login\n  To use standalone: cub-scout map (without --hub)")
	}

	// Create model with context
	m := initialModelWithContext(appContext)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}

	// Check if user wants to switch to local cluster mode
	if fm, ok := finalModel.(Model); ok {
		return fm.launchLocalCluster, nil
	}

	return false, nil
}

var mapListCmd = &cobra.Command{
	Use:   "list",
	Short: "List resources and their ownership",
	Long: `List resources and their ownership from the current Kubernetes cluster.

QUICK SHORTCUTS (for common queries):
  cub-scout map crashes     # Find crashing/failing resources
  cub-scout map orphans     # Find unmanaged (Native) resources
  cub-scout map issues      # All resources with issues

Query Syntax:
  field=value           Exact match (case-insensitive)
  field!=value          Not equal
  field~=pattern        Regex match
  field=val1,val2       IN list (comma-separated)
  field=prefix*         Wildcard match

  AND                   Both conditions must match
  OR                    Either condition must match

Available Fields:
  kind, namespace, name, owner, status, cluster, labels[key]

Status Values:
  Ready                 Resource is healthy and operational
  NotReady              Resource exists but not yet ready
  Failed                Resource has failed (crash, error, etc)
  Pending               Resource is waiting to be scheduled
  Unknown               Status cannot be determined

Time Filtering:
  --since=1h            Resources changed in last hour
  --since=24h           Resources changed in last day
  --since=7d            Resources changed in last week

Examples:
  # List all resources from current cluster
  cub-scout map list

  # Filter by namespace and kind
  cub-scout map list --namespace default --kind Deployment

  # Filter by owner (Flux, ArgoCD, Helm, Terraform, Crossplane, ConfigHub, Native)
  cub-scout map list --owner ConfigHub

  # Find unhealthy/failing resources
  cub-scout map list -q "status!=Ready"
  cub-scout map crashes              # shortcut for crashes

  # Find orphaned/unmanaged resources (shadow IT)
  cub-scout map list -q "owner=Native"
  cub-scout map orphans              # shortcut for above

  # Query: GitOps-managed deployments
  cub-scout map list -q "kind=Deployment AND owner!=Native"

  # Query: Resources in production namespaces
  cub-scout map list -q "namespace=prod*"

  # Query: Flux or Argo managed
  cub-scout map list -q "owner=Flux OR owner=ArgoCD"

  # Query: By label
  cub-scout map list -q "labels[app]=nginx"

  # Recent changes (incident investigation)
  cub-scout map list --since=1h      # last hour
  cub-scout map list --since=24h     # last day

  # JSON output
  cub-scout map list --json
`,
	RunE: runMapList,
}

var mapStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "One-line health check of the cluster",
	Long: `Quick health check showing deployer and workload status.

Example output:
  ✓ healthy: 3/3 deployers, 12/12 workloads
  ✗ problems: 1/3 deployers, 10/12 workloads`,
	RunE: runMapStatus,
}

var mapProblemsCmd = &cobra.Command{
	Use:     "issues",
	Aliases: []string{"problems"},
	Short:   "List resources with issues",
	Long: `List resources that have issues - failed deployments, stuck reconciliations, etc.

Shows:
  - Deployments with unavailable replicas
  - Flux Kustomizations/HelmReleases not ready
  - Argo CD Applications not synced/healthy`,
	RunE: runMapProblems,
}

var mapDeployersCmd = &cobra.Command{
	Use:   "deployers",
	Short: "List GitOps deployers (Flux, ArgoCD)",
	Long: `List all GitOps deployers from Flux and ArgoCD.

Shows:
  - Flux Kustomizations and HelmReleases
  - ArgoCD Applications
  - Sync status and health`,
	RunE: runMapDeployers,
}

var mapWorkloadsCmd = &cobra.Command{
	Use:   "workloads",
	Short: "List workloads grouped by owner",
	Long: `List all workloads (Deployments, StatefulSets, DaemonSets) grouped by owner.

Owners: Flux, ArgoCD, Helm, Terraform, Crossplane, ConfigHub, Native`,
	RunE: runMapWorkloads,
}

var mapDriftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Show drift detection - resources diverged from desired state",
	Long: `Drift detection shows resources that have diverged from their desired state.

This includes:
- GitOps resources out of sync (Flux Kustomizations, ArgoCD Applications)
- ConfigHub units with revision drift`,
	RunE: runMapDrift,
}

var mapSprawlCmd = &cobra.Command{
	Use:   "sprawl",
	Short: "Show configuration sprawl analysis",
	Long: `Configuration sprawl analysis shows how configuration is distributed.

Shows:
- GitOps coverage percentage
- Breakdown by owner (Flux, ArgoCD, Helm, ConfigHub, Native)
- Native workloads that should be added to GitOps`,
	RunE: runMapSprawl,
}

var mapDashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Unified cluster dashboard - health + ownership at a glance",
	Long: `Unified dashboard combining health status and ownership breakdown.

Shows:
- Deployer health (Flux Kustomizations, HelmReleases, ArgoCD Applications)
- Workload health (Deployments, StatefulSets, DaemonSets)
- GitOps coverage percentage
- Breakdown by owner with visual bars
- Shadow IT warnings for native workloads

Example:
  cub-scout map dashboard`,
	RunE: runMapDashboard,
}

var mapBypassCmd = &cobra.Command{
	Use:   "bypass",
	Short: "Show factory bypass detection - changes outside GitOps",
	Long: `Factory bypass detection shows resources deployed outside of GitOps pipelines.

This includes:
- Native workloads (kubectl apply'd directly)
- Resources without GitOps ownership labels`,
	RunE: runMapBypass,
}

var mapCrashesCmd = &cobra.Command{
	Use:   "crashes",
	Short: "List crashing pods and failing deployments",
	Long: `Quick way to find resources that are crashing or failing.

Shows:
- Pods in CrashLoopBackOff
- Deployments with unavailable replicas
- Failed Flux Kustomizations/HelmReleases
- OutOfSync ArgoCD Applications

This is equivalent to 'cub-scout map issues' but focused on crashes.

Examples:
  cub-scout map crashes             # Show all crashes
  cub-scout map crashes --json      # JSON output for scripting`,
	RunE: runMapCrashes,
}

var mapOrphansCmd = &cobra.Command{
	Use:     "orphans",
	Aliases: []string{"native", "unmanaged"},
	Short:   "List orphaned resources (not managed by GitOps)",
	Long: `Find resources deployed outside GitOps - shadow IT detection.

Orphaned resources are those without detected GitOps or platform ownership:
- kubectl apply'd directly
- Created by operators/controllers not tracked by GitOps
- Legacy resources from before GitOps adoption

Note: Resources managed by Crossplane or Terraform controllers are not considered orphans.

This is equivalent to: cub-scout map list -q "owner=Native"

Examples:
  cub-scout map orphans             # List all orphaned resources
  cub-scout map orphans --json      # JSON output
  cub-scout map orphans --namespace prod  # Filter by namespace`,
	RunE: runMapOrphans,
}

// mapHubCmd launches the ConfigHub hierarchy TUI
var mapHubCmd = &cobra.Command{
	Use:   "hub",
	Short: "ConfigHub hierarchy explorer (requires cub auth)",
	Long: `Launch the ConfigHub hierarchy TUI to explore Organizations, Spaces, Units, Targets, and Workers.

This is equivalent to 'cub-scout map --hub' or the deprecated 'cub-scout hierarchy' command.

Requires ConfigHub authentication. Run 'cub auth login' first.
`,
	RunE: runHierarchy,
}

var mapClusterDataCmd = &cobra.Command{
	Use:     "deep-dive",
	Aliases: []string{"cluster-data", "data", "sources"},
	Short:   "Deep dive into all GitOps resources with live tree views",
	Long: `Deep dive into all GitOps resources with maximum detail and live state.

This is the CLI equivalent of pressing '4' in the interactive TUI.

Shows for each resource type:
- Flux: GitRepositories, Kustomizations, HelmReleases with conditions, inventory, LiveTree
- ArgoCD: AppProjects, Applications with sync results, history, LiveTree
- Helm: Releases with chart details, values, NOTES.txt, hooks, history, LiveTree
- Workloads: By owner with pod labels, annotations, Prometheus config
- LiveTree: Deployment -> ReplicaSet -> Pod with IPs, nodes, restarts`,
	RunE: runMapClusterData,
}

var mapAppHierarchyCmd = &cobra.Command{
	Use:     "app-hierarchy",
	Aliases: []string{"hierarchy", "infer"},
	Short:   "Show inferred ConfigHub app hierarchy",
	Long: `Show TUI's best-effort interpretation of cluster as ConfigHub model.

This is the CLI equivalent of pressing '5' or 'A' in the interactive TUI.

Shows:
- Inferred Hub (from infrastructure/platform patterns)
- Inferred AppSpaces (from namespace patterns like prod, staging, dev)
- Inferred labels (groups, teams)

Note: This is a heuristic interpretation. Connect to ConfigHub for actual hierarchy.`,
	RunE: runMapAppHierarchy,
}

func init() {
	rootCmd.AddCommand(mapCmd)
	mapCmd.AddCommand(mapListCmd)
	mapCmd.AddCommand(mapFleetCmd)
	mapCmd.AddCommand(mapStatusCmd)
	mapCmd.AddCommand(mapProblemsCmd)
	mapCmd.AddCommand(mapDeployersCmd)
	mapCmd.AddCommand(mapWorkloadsCmd)
	mapCmd.AddCommand(mapDriftCmd)
	mapCmd.AddCommand(mapSprawlCmd)
	mapCmd.AddCommand(mapDashboardCmd)
	mapCmd.AddCommand(mapBypassCmd)
	mapCmd.AddCommand(mapCrashesCmd)
	mapCmd.AddCommand(mapOrphansCmd)
	mapCmd.AddCommand(mapHubCmd)
	mapCmd.AddCommand(mapClusterDataCmd)
	mapCmd.AddCommand(mapAppHierarchyCmd)

	// Hub flag (same as 'map hub' subcommand)
	mapCmd.Flags().BoolVar(&mapHub, "hub", false, "Launch ConfigHub hierarchy TUI (requires cub auth)")

	// Fleet-specific flags
	mapFleetCmd.Flags().StringVar(&fleetApp, "app", "", "Filter by app label")
	mapFleetCmd.Flags().StringVar(&fleetSpace, "space", "", "Filter by space (App Space)")

	// Global map flags
	mapCmd.PersistentFlags().BoolVar(&mapJSON, "json", false, "Output in JSON format")
	mapCmd.PersistentFlags().BoolVar(&mapVerbose, "verbose", false, "Show additional details")

	// List-specific flags
	mapListCmd.Flags().StringVar(&mapNamespace, "namespace", "", "Filter by namespace")
	mapListCmd.Flags().StringVar(&mapKind, "kind", "", "Filter by resource kind")
	mapListCmd.Flags().StringVar(&mapOwner, "owner", "", "Filter by owner (Flux, ArgoCD, Helm, Terraform, Crossplane, ConfigHub, Native)")
	mapListCmd.Flags().StringVarP(&mapQuery, "query", "q", "", "Query expression (e.g., 'kind=Deployment AND owner!=Native')")
	mapListCmd.Flags().StringVar(&mapSince, "since", "", "Show resources changed since duration (e.g., 1h, 24h, 7d)")
	mapListCmd.Flags().BoolVar(&mapCount, "count", false, "Output count only (no list)")
	mapListCmd.Flags().BoolVar(&mapNamesOnly, "names-only", false, "Output names only (for scripting)")
	mapListCmd.Flags().BoolVar(&mapExplain, "explain", false, "Show explanatory content to help learn GitOps concepts")

	// Orphans-specific flags (same as list)
	mapOrphansCmd.Flags().StringVar(&mapNamespace, "namespace", "", "Filter by namespace")

	// Deep-dive flags
	mapClusterDataCmd.Flags().BoolVar(&deepDiveConnected, "connected", false, "Show ConfigHub context for managed resources (requires cub auth)")

	// Register shell completion functions for flags
	_ = mapListCmd.RegisterFlagCompletionFunc("namespace", completeNamespaces)
	_ = mapListCmd.RegisterFlagCompletionFunc("kind", completeKinds)
	_ = mapListCmd.RegisterFlagCompletionFunc("owner", completeOwners)
	_ = mapListCmd.RegisterFlagCompletionFunc("since", completeSince)
}

func runMapList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Build Kubernetes config
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	// Create dynamic client
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	// Get cluster name
	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "default"
	}

	// Collect resources
	entries := []MapEntry{}
	byOwner := map[string]int{}

	// Resource types to scan
	resources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		// Flux resources
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		// Argo resources
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
	}

	for _, gvr := range resources {
		if mapNamespace != "" {
			l, err := dynClient.Resource(gvr).Namespace(mapNamespace).List(ctx, v1.ListOptions{})
			if err != nil {
				continue // Skip resources that don't exist
			}
			for _, item := range l.Items {
				entries = processResource(&item, gvr, clusterName, entries, byOwner)
			}
		} else {
			l, err := dynClient.Resource(gvr).List(ctx, v1.ListOptions{})
			if err != nil {
				continue
			}
			for _, item := range l.Items {
				entries = processResource(&item, gvr, clusterName, entries, byOwner)
			}
		}
	}

	// Apply filters
	filtered := []MapEntry{}

	// Parse query if provided (resolve saved query names first)
	var q *query.Query
	if mapQuery != "" {
		resolvedQuery := resolveSavedQueries(mapQuery)
		var err error
		q, err = query.Parse(resolvedQuery)
		if err != nil {
			return fmt.Errorf("invalid query: %w", err)
		}
	}

	for _, e := range entries {
		// Legacy flag filters
		if mapKind != "" && e.Kind != mapKind {
			continue
		}
		if mapOwner != "" && !strings.EqualFold(e.Owner, mapOwner) {
			continue
		}
		// Query filter
		if q != nil && !q.Matches(e) {
			continue
		}
		filtered = append(filtered, e)
	}
	entries = filtered

	// Sort by namespace, then name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Namespace != entries[j].Namespace {
			return entries[i].Namespace < entries[j].Namespace
		}
		return entries[i].Name < entries[j].Name
	})

	// Handle --count flag (output count only)
	if mapCount {
		fmt.Println(len(entries))
		return nil
	}

	// Handle --names-only flag (output names only, for scripting)
	if mapNamesOnly {
		for _, e := range entries {
			if e.Namespace != "" {
				fmt.Printf("%s/%s\n", e.Namespace, e.Name)
			} else {
				fmt.Println(e.Name)
			}
		}
		return nil
	}

	if mapJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	// Explain mode: show header explaining ownership detection
	if mapExplain {
		fmt.Println()
		fmt.Println("GITOPS OWNERSHIP EXPLAINED")
		fmt.Println("════════════════════════════════════════════════════════════════════")
		fmt.Println("cub-scout detects who manages each resource by reading labels.")
		fmt.Println()
		fmt.Println("FLUX resources have labels like:")
		fmt.Println("  kustomize.toolkit.fluxcd.io/name: my-app")
		fmt.Println("  kustomize.toolkit.fluxcd.io/namespace: flux-system")
		fmt.Println()
		fmt.Println("ARGOCD resources have labels like:")
		fmt.Println("  app.kubernetes.io/instance: my-app")
		fmt.Println("  argocd.argoproj.io/instance: my-app")
		fmt.Println()
		fmt.Println("HELM resources have:")
		fmt.Println("  app.kubernetes.io/managed-by: Helm")
		fmt.Println()
		fmt.Println("NATIVE means no GitOps tool claims ownership (kubectl-applied).")
		fmt.Println("════════════════════════════════════════════════════════════════════")
		fmt.Println()
	}

	// Table output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if mapVerbose {
		fmt.Fprintln(w, "NAMESPACE\tKIND\tNAME\tOWNER\tOWNER_DETAIL")
		for _, e := range entries {
			detail := ""
			if e.OwnerDetails != nil {
				if space := e.OwnerDetails["space"]; space != "" {
					detail = fmt.Sprintf("%s/%s", space, e.OwnerDetails["unit"])
				} else if name := e.OwnerDetails["name"]; name != "" {
					detail = name
				}
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				e.Namespace,
				e.Kind,
				e.Name,
				e.Owner,
				detail,
			)
		}
	} else {
		fmt.Fprintln(w, "NAMESPACE\tKIND\tNAME\tOWNER")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				e.Namespace,
				e.Kind,
				e.Name,
				e.Owner,
			)
		}
	}
	w.Flush()

	// Summary
	fmt.Printf("\nTotal: %d resources\n", len(entries))
	fmt.Print("By Owner: ")
	owners := make([]string, 0, len(byOwner))
	for owner := range byOwner {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	ownerParts := make([]string, 0, len(owners))
	for _, owner := range owners {
		ownerParts = append(ownerParts, fmt.Sprintf("%s(%d)", owner, byOwner[owner]))
	}
	fmt.Println(strings.Join(ownerParts, " "))

	// Explain mode: show what this means and next steps
	if mapExplain {
		fmt.Println()
		fmt.Println("WHAT THIS MEANS:")
		// Show owner-specific explanations based on what was found
		if byOwner["Flux"] > 0 {
			fmt.Printf("• %d resources are managed by Flux → Changes flow from Git automatically\n", byOwner["Flux"])
		}
		if byOwner["ArgoCD"] > 0 {
			fmt.Printf("• %d resources are managed by ArgoCD → Synced from Git via ArgoCD\n", byOwner["ArgoCD"])
		}
		if byOwner["Helm"] > 0 {
			fmt.Printf("• %d resources are managed by Helm → Installed via helm install/upgrade\n", byOwner["Helm"])
		}
		if byOwner["Terraform"] > 0 {
			fmt.Printf("• %d resources are managed by Terraform → Provisioned by Terraform controllers\n", byOwner["Terraform"])
		}
		if byOwner["Crossplane"] > 0 {
			fmt.Printf("• %d resources are managed by Crossplane → Created/controlled by Crossplane compositions\n", byOwner["Crossplane"])
		}
		if byOwner["ConfigHub"] > 0 {
			fmt.Printf("• %d resources are managed by ConfigHub → Deployed via ConfigHub\n", byOwner["ConfigHub"])
		}
		if byOwner["Native"] > 0 {
			fmt.Printf("• %d resources are Native → No detected GitOps or platform controller ownership\n", byOwner["Native"])
		}
		fmt.Println()
		fmt.Println("NEXT STEPS:")
		fmt.Println("→ See the Git→Deployment chain: cub-scout trace <kind>/<name> -n <namespace>")
		fmt.Println("→ See the full GitOps pipeline:  cub-scout map deployers")
		fmt.Println("→ Visual guide:                  docs/diagrams/ownership-detection.svg")
	}

	return nil
}

func processResource(item interface{}, gvr schema.GroupVersionResource, clusterName string, entries []MapEntry, byOwner map[string]int) []MapEntry {
	// Type assert to unstructured.Unstructured
	unstr, ok := item.(*unstructured.Unstructured)
	if !ok {
		return entries
	}

	labels := unstr.GetLabels()
	annotations := unstr.GetAnnotations()

	// Detect ownership using the canonical agent function
	ownership := agent.DetectOwnership(unstr)

	entry := MapEntry{
		ID:          fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, unstr.GetNamespace(), gvr.Group, unstr.GetKind(), unstr.GetName()),
		ClusterName: clusterName,
		Namespace:   unstr.GetNamespace(),
		Kind:        unstr.GetKind(),
		Name:        unstr.GetName(),
		APIVersion:  unstr.GetAPIVersion(),
		Owner:       displayOwner(ownership.Type),
		Labels:      labels,
		Status:      detectStatus(unstr),
		CreatedAt:   unstr.GetCreationTimestamp().Time,
		UpdatedAt:   unstr.GetCreationTimestamp().Time,
	}

	if ownership.Type != "" && ownership.Type != agent.OwnerUnknown {
		entry.OwnerDetails = map[string]string{}
		if ownership.Name != "" {
			entry.OwnerDetails["name"] = ownership.Name
		}
		if ownership.Namespace != "" {
			entry.OwnerDetails["namespace"] = ownership.Namespace
		}
		if ownership.SubType != "" {
			entry.OwnerDetails["subType"] = ownership.SubType
		}
		// Add ConfigHub specific details
		if ownership.Type == agent.OwnerConfigHub {
			if space := annotations["confighub.com/SpaceName"]; space != "" {
				entry.OwnerDetails["space"] = space
			}
			if unit := labels["confighub.com/UnitSlug"]; unit != "" {
				entry.OwnerDetails["unit"] = unit
			}
			if rev := annotations["confighub.com/RevisionNum"]; rev != "" {
				entry.OwnerDetails["revision"] = rev
			}
		}
	}

	// "Native" means no GitOps owner detected (not managed by Flux, Argo, Helm, or ConfigHub).
	// These are resources deployed directly via kubectl, or system components like the GitOps
	// controllers themselves. This is expected and correct - the insight is knowing WHAT is
	// unmanaged, not that unmanaged resources exist.
	// Note: displayOwner() already maps empty/unknown types to "Native"

	byOwner[entry.Owner]++
	return append(entries, entry)
}

// Fleet View: Hub/App Space model display
var mapFleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Show fleet view grouped by app and variant (Hub/App Space model)",
	Long: `Display units across spaces grouped by app and variant labels.

This view requires units imported with --model hub-appspace, which adds:
  - Labels.app: The application name
  - Labels.variant: The environment variant (dev, staging, prod)

The fleet view shows:
  Application (app label)
  ├── variant: dev
  │   └── cluster @ revision
  ├── variant: prod
  │   └── cluster @ revision

Example:
  # View all apps across spaces
  cub-scout map fleet

  # Filter to specific app
  cub-scout map fleet --app payment-api

  # Filter to specific space (App Space)
  cub-scout map fleet --space payments-team
`,
	RunE: runMapFleet,
}

var (
	fleetApp   string
	fleetSpace string
)

// FleetUnit represents a unit with its labels and status
type FleetUnit struct {
	Slug         string
	Space        string
	App          string
	Variant      string
	Revision     int
	LiveRevision int
	Status       string
	Target       string
}

func runMapFleet(cmd *cobra.Command, args []string) error {
	// Get units from ConfigHub
	units, err := fetchFleetUnits(fleetSpace, fleetApp)
	if err != nil {
		return err
	}

	if len(units) == 0 {
		fmt.Println("No units found with app/variant labels.")
		fmt.Println("\nTo use fleet view, import with Hub/App Space model:")
		fmt.Println("  cub-scout import --namespace myapp-prod --model hub-appspace")
		return nil
	}

	if mapJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(units)
	}

	// Group by app, then variant
	apps := make(map[string]map[string][]FleetUnit)
	for _, u := range units {
		if apps[u.App] == nil {
			apps[u.App] = make(map[string][]FleetUnit)
		}
		apps[u.App][u.Variant] = append(apps[u.App][u.Variant], u)
	}

	// Sort app names
	appNames := make([]string, 0, len(apps))
	for app := range apps {
		appNames = append(appNames, app)
	}
	sort.Strings(appNames)

	fmt.Println("ConfigHub Fleet View (Hub/App Space Model)")
	fmt.Println("Hierarchy: Application → Variant → Target")
	fmt.Println()

	for _, appName := range appNames {
		variants := apps[appName]
		fmt.Printf("  %s\n", appName)

		// Sort variant names
		variantNames := make([]string, 0, len(variants))
		for v := range variants {
			variantNames = append(variantNames, v)
		}
		sort.Strings(variantNames)

		for i, variantName := range variantNames {
			units := variants[variantName]
			isLastVariant := i == len(variantNames)-1
			variantPrefix := "├──"
			if isLastVariant {
				variantPrefix = "└──"
			}
			fmt.Printf("  %s variant: %s\n", variantPrefix, variantName)

			for j, u := range units {
				isLastUnit := j == len(units)-1
				unitPrefix := "│   └──"
				if !isLastVariant {
					unitPrefix = "│   └──"
				}
				if isLastVariant {
					unitPrefix = "    └──"
				}
				if !isLastUnit {
					if isLastVariant {
						unitPrefix = "    ├──"
					} else {
						unitPrefix = "│   ├──"
					}
				}

				// Status icon
				icon := "✓"
				if u.Status == "Drifted" {
					icon = "⚠"
				} else if u.Status == "Failed" || u.Status == "Error" {
					icon = "✗"
				}

				// Revision status
				revStatus := fmt.Sprintf("@ rev %d", u.Revision)
				if u.LiveRevision > 0 && u.LiveRevision < u.Revision {
					revStatus = fmt.Sprintf("@ rev %d ← behind!", u.LiveRevision)
					icon = "⚠"
				}

				target := u.Target
				if target == "" {
					target = u.Space
				}

				fmt.Printf("  %s %s %s %s\n", unitPrefix, icon, target, revStatus)
			}
		}
		fmt.Println()
	}

	return nil
}

func fetchFleetUnits(space, appFilter string) ([]FleetUnit, error) {
	// Build cub command to list units
	args := []string{"unit", "list", "--json"}
	if space != "" {
		args = append(args, "--space", space)
	}

	cmd := exec.Command("cub", args...)
	output, err := cmd.Output()
	if err != nil {
		// Check if it's an auth error
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "authentication") ||
				strings.Contains(stderr, "token") ||
				strings.Contains(stderr, "unauthorized") ||
				strings.Contains(stderr, "401") {
				return nil, fmt.Errorf("ConfigHub authentication required.\n\n  To authenticate: cub auth login\n  To use standalone: cub-scout map (without --hub)")
			}
			// Include stderr in error for debugging
			if stderr != "" {
				return nil, fmt.Errorf("failed to fetch units from ConfigHub: %s", strings.TrimSpace(stderr))
			}
		}
		return nil, fmt.Errorf("failed to fetch units from ConfigHub: %w\n\n  Check that 'cub' CLI is installed and you're authenticated: cub auth login", err)
	}

	// The cub CLI returns nested structure: [{Space: {}, Unit: {}, UnitStatus: {}}, ...]
	var response []map[string]interface{}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var units []FleetUnit
	for _, item := range response {
		// Extract from nested Unit object
		unitObj, ok := item["Unit"].(map[string]interface{})
		if !ok {
			continue
		}
		spaceObj, _ := item["Space"].(map[string]interface{})
		statusObj, _ := item["UnitStatus"].(map[string]interface{})

		slug := ""
		spaceSlug := ""
		headRev := 0
		liveRev := 0
		status := ""
		target := ""

		if s, ok := unitObj["Slug"].(string); ok {
			slug = s
		}
		if spaceObj != nil {
			if s, ok := spaceObj["Slug"].(string); ok {
				spaceSlug = s
			}
		}
		if r, ok := unitObj["HeadRevisionNum"].(float64); ok {
			headRev = int(r)
		}
		if r, ok := unitObj["LiveRevisionNum"].(float64); ok {
			liveRev = int(r)
		}
		if statusObj != nil {
			if s, ok := statusObj["Status"].(string); ok {
				status = s
			}
		}
		if t, ok := unitObj["Target"].(string); ok {
			target = t
		}

		// Labels aren't in list output, need to fetch each unit
		labels, err := fetchUnitLabels(spaceSlug, slug)
		if err != nil {
			continue // Skip units we can't fetch
		}

		// Only include units with app label
		app := labels["app"]
		if app == "" {
			continue
		}

		// Filter by app if specified
		if appFilter != "" && app != appFilter {
			continue
		}

		variant := labels["variant"]
		if variant == "" {
			variant = "default"
		}

		units = append(units, FleetUnit{
			Slug:         slug,
			Space:        spaceSlug,
			App:          app,
			Variant:      variant,
			Revision:     headRev,
			LiveRevision: liveRev,
			Status:       status,
			Target:       target,
		})
	}

	return units, nil
}

// fetchUnitLabels gets labels for a specific unit
func fetchUnitLabels(space, slug string) (map[string]string, error) {
	cmd := exec.Command("cub", "unit", "get", slug, "--space", space, "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var response map[string]interface{}
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, err
	}

	labels := make(map[string]string)
	if unitObj, ok := response["Unit"].(map[string]interface{}); ok {
		if l, ok := unitObj["Labels"].(map[string]interface{}); ok {
			for k, v := range l {
				if vs, ok := v.(string); ok {
					labels[k] = vs
				}
			}
		}
	}

	return labels, nil
}

// resolveSavedQueries expands saved query names in a query expression
// Example: "unmanaged AND namespace=prod*" -> "owner=Native AND namespace=prod*"
func resolveSavedQueries(queryExpr string) string {
	store, err := queries.NewQueryStore()
	if err != nil {
		return queryExpr
	}

	// Simple token-based replacement
	// This handles cases like:
	// - "unmanaged" -> "owner=Native"
	// - "unmanaged AND namespace=prod*" -> "owner=Native AND namespace=prod*"
	// - "gitops OR helm-only" -> "(owner=Flux OR owner=Argo) OR owner=Helm"

	tokens := strings.Fields(queryExpr)
	result := make([]string, 0, len(tokens))

	for _, token := range tokens {
		upper := strings.ToUpper(token)
		// Skip operators
		if upper == "AND" || upper == "OR" {
			result = append(result, token)
			continue
		}

		// Check if it's a saved query name (no = or != or ~=)
		if !strings.Contains(token, "=") {
			if saved, found := store.Get(token); found {
				// Wrap in parens if it contains OR to preserve precedence
				if strings.Contains(strings.ToUpper(saved.Query), " OR ") {
					result = append(result, "("+saved.Query+")")
				} else {
					result = append(result, saved.Query)
				}
				continue
			}
		}

		// Not a saved query, keep as-is
		result = append(result, token)
	}

	return strings.Join(result, " ")
}

// runMapStatus shows a one-line health summary
func runMapStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	// Count deployers
	deployersReady, deployersTotal := 0, 0

	// Check Flux Kustomizations
	if ksList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, ks := range ksList.Items {
			deployersTotal++
			if isResourceReady(&ks) {
				deployersReady++
			}
		}
	}

	// Check Flux HelmReleases
	if hrList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, hr := range hrList.Items {
			deployersTotal++
			if isResourceReady(&hr) {
				deployersReady++
			}
		}
	}

	// Check ArgoCD Applications
	if appList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, app := range appList.Items {
			deployersTotal++
			if isArgoAppHealthy(&app) {
				deployersReady++
			}
		}
	}

	// Count workloads
	workloadsReady, workloadsTotal := 0, 0

	// Check Deployments
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if strings.HasPrefix(ns, "kube-") || ns == "local-path-storage" {
				continue
			}
			workloadsTotal++
			if isDeploymentReady(&dep) {
				workloadsReady++
			}
		}
	}

	// Output
	problems := (deployersTotal - deployersReady) + (workloadsTotal - workloadsReady)
	if problems == 0 {
		fmt.Printf("✓ healthy: %d/%d deployers, %d/%d workloads\n",
			deployersReady, deployersTotal, workloadsReady, workloadsTotal)
	} else {
		fmt.Printf("✗ %d problem(s): %d/%d deployers, %d/%d workloads\n",
			problems, deployersReady, deployersTotal, workloadsReady, workloadsTotal)
	}

	return nil
}

// runMapProblems lists resources with issues
func runMapProblems(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	// Track deployer vs workload issues separately
	deployerIssues := []string{}
	workloadIssues := []string{}

	// Check Flux Kustomizations
	if ksList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, ks := range ksList.Items {
			if !isResourceReady(&ks) {
				reason := getConditionReason(&ks)
				deployerIssues = append(deployerIssues, fmt.Sprintf("✗ Kustomization/%s in %s: %s",
					ks.GetName(), ks.GetNamespace(), reason))
			}
		}
	}

	// Check Flux HelmReleases
	if hrList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, hr := range hrList.Items {
			if !isResourceReady(&hr) {
				reason := getConditionReason(&hr)
				deployerIssues = append(deployerIssues, fmt.Sprintf("✗ HelmRelease/%s in %s: %s",
					hr.GetName(), hr.GetNamespace(), reason))
			}
		}
	}

	// Check ArgoCD Applications
	if appList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, app := range appList.Items {
			if !isArgoAppHealthy(&app) {
				status := getArgoStatus(&app)
				deployerIssues = append(deployerIssues, fmt.Sprintf("✗ Application/%s in %s: %s",
					app.GetName(), app.GetNamespace(), status))
			}
		}
	}

	// Check Deployments
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if strings.HasPrefix(ns, "kube-") || ns == "local-path-storage" {
				continue
			}
			if !isDeploymentReady(&dep) {
				desired, available := getDeploymentReplicas(&dep)
				workloadIssues = append(workloadIssues, fmt.Sprintf("✗ Deployment/%s in %s: %d/%d ready",
					dep.GetName(), ns, available, desired))
			}
		}
	}

	totalIssues := len(deployerIssues) + len(workloadIssues)

	if totalIssues == 0 {
		fmt.Println("✓ No issues found")
		return nil
	}

	// Print header
	fmt.Println()
	fmt.Println("RESOURCES WITH ISSUES")
	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("Deployers and workloads with conditions != Ready.")
	fmt.Println()

	// Print deployer issues
	if len(deployerIssues) > 0 {
		fmt.Printf("DEPLOYERS (%d issues)\n", len(deployerIssues))
		for _, p := range deployerIssues {
			fmt.Println(p)
		}
		fmt.Println()
	}

	// Print workload issues
	if len(workloadIssues) > 0 {
		fmt.Printf("WORKLOADS (%d issues)\n", len(workloadIssues))
		for _, p := range workloadIssues {
			fmt.Println(p)
		}
		fmt.Println()
	}

	// Summary
	fmt.Printf("%d total issues (%d deployers, %d workloads)\n", totalIssues, len(deployerIssues), len(workloadIssues))

	// Next steps
	fmt.Println()
	fmt.Println("NEXT STEPS:")
	fmt.Println("→ For remediation commands: cub-scout scan")
	fmt.Println("→ To trace a failing resource: cub-scout trace <kind>/<name> -n <namespace>")
	fmt.Println("→ To see full details: cub-scout map deep-dive")
	fmt.Println("→ Visual guide: docs/diagrams/ownership-trace.svg")

	return nil
}

// runMapDeployers lists GitOps deployers
func runMapDeployers(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	// Count by type
	var ksCount, hrCount, appCount int

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STATUS\tKIND\tNAME\tNAMESPACE\tREVISION\tRESOURCES")
	fmt.Fprintln(w, "──────\t────\t────\t─────────\t────────\t─────────")

	// Flux Kustomizations
	if ksList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, ks := range ksList.Items {
			ksCount++
			status := "✓"
			if !isResourceReady(&ks) {
				status = "✗"
			}
			rev := getLastAppliedRevision(&ks)
			resources := getInventoryCount(&ks)
			fmt.Fprintf(w, "%s\tKustomization\t%s\t%s\t%s\t%d\n",
				status, ks.GetName(), ks.GetNamespace(), rev, resources)
		}
	}

	// Flux HelmReleases
	if hrList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, hr := range hrList.Items {
			hrCount++
			status := "✓"
			if !isResourceReady(&hr) {
				status = "✗"
			}
			rev := getLastAppliedRevision(&hr)
			fmt.Fprintf(w, "%s\tHelmRelease\t%s\t%s\t%s\t-\n",
				status, hr.GetName(), hr.GetNamespace(), rev)
		}
	}

	// ArgoCD Applications
	if appList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, app := range appList.Items {
			appCount++
			status := "✓"
			if !isArgoAppHealthy(&app) {
				status = "✗"
			}
			rev := getArgoRevision(&app)
			resources := getArgoResourceCount(&app)
			fmt.Fprintf(w, "%s\tApplication\t%s\t%s\t%s\t%d\n",
				status, app.GetName(), app.GetNamespace(), rev, resources)
		}
	}

	w.Flush()

	// Summary
	total := ksCount + hrCount + appCount
	fmt.Printf("\n%d deployers: %d Kustomizations, %d HelmReleases, %d Applications\n",
		total, ksCount, hrCount, appCount)
	fmt.Println("→ Visual guide: docs/diagrams/flux-architecture.svg")

	return nil
}

// runMapWorkloads lists workloads by owner
func runMapWorkloads(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	// Count by owner
	ownerCounts := map[string]int{}
	var total int

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STATUS\tNAMESPACE\tNAME\tOWNER\tMANAGED-BY\tIMAGE")
	fmt.Fprintln(w, "──────\t─────────\t────\t─────\t──────────\t─────")

	// Get Deployments
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}

			total++
			status := "✓"
			if !isDeploymentReady(&dep) {
				status = "✗"
			}

			owner, managedBy := detectOwnership(&dep)
			ownerCounts[owner]++
			image := getContainerImage(&dep)

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				status, ns, dep.GetName(), owner, managedBy, image)
		}
	}

	w.Flush()

	// Summary
	if total > 0 {
		// Build owner breakdown in consistent order
		owners := []string{"Flux", "ArgoCD", "Helm", "ConfigHub", "Native"}
		var parts []string
		for _, owner := range owners {
			if count, ok := ownerCounts[owner]; ok && count > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", count, owner))
			}
		}
		fmt.Printf("\n%d workloads: %s\n", total, strings.Join(parts, ", "))
	}

	return nil
}

func runMapDrift(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	fmt.Println("🔄 DRIFT DETECTION")
	fmt.Println()

	var driftedCount int

	// Check Flux Kustomizations
	if kslist, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, ks := range kslist.Items {
			if !isResourceReady(&ks) {
				driftedCount++
				reason := getConditionReason(&ks)
				fmt.Printf("⚠ Kustomization/%s in %s: %s\n",
					ks.GetName(), ks.GetNamespace(), reason)
			}
		}
	}

	// Check ArgoCD Applications
	if appList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, app := range appList.Items {
			syncStatus, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")
			if syncStatus != "" && syncStatus != "Synced" {
				driftedCount++
				healthStatus, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
				fmt.Printf("⚠ Application/%s in %s: %s/%s\n",
					app.GetName(), app.GetNamespace(), syncStatus, healthStatus)
			}
		}
	}

	// Check HelmReleases
	if hrList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, hr := range hrList.Items {
			if !isResourceReady(&hr) {
				driftedCount++
				reason := getConditionReason(&hr)
				fmt.Printf("⚠ HelmRelease/%s in %s: %s\n",
					hr.GetName(), hr.GetNamespace(), reason)
			}
		}
	}

	if driftedCount == 0 {
		fmt.Println("✓ No drift detected - all resources are in sync")
	} else {
		fmt.Printf("\n⚠ %d resource(s) have drifted from desired state\n", driftedCount)
	}

	return nil
}

func runMapSprawl(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	fmt.Println("📊 CONFIGURATION SPRAWL ANALYSIS")
	fmt.Println()

	var fluxCount, argoCount, helmCount, configHubCount, nativeCount int

	// Get all Deployments and count by owner
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}
			owner, _ := detectOwnership(&dep)
			switch owner {
			case "Flux":
				fluxCount++
			case "ArgoCD":
				argoCount++
			case "Helm":
				helmCount++
			case "ConfigHub":
				configHubCount++
			case "Native":
				nativeCount++
			}
		}
	}

	total := fluxCount + argoCount + helmCount + configHubCount + nativeCount
	managed := total - nativeCount

	// Calculate coverage
	coverage := 0
	if total > 0 {
		coverage = (managed * 100) / total
	}

	// Print results
	fmt.Printf("COVERAGE: %d%% GitOps managed\n\n", coverage)

	fmt.Println("BY OWNER:")
	if fluxCount > 0 {
		fmt.Printf("  Flux      %3d %s\n", fluxCount, makeBar(fluxCount, total, 20))
	}
	if argoCount > 0 {
		fmt.Printf("  ArgoCD    %3d %s\n", argoCount, makeBar(argoCount, total, 20))
	}
	if helmCount > 0 {
		fmt.Printf("  Helm      %3d %s\n", helmCount, makeBar(helmCount, total, 20))
	}
	if configHubCount > 0 {
		fmt.Printf("  ConfigHub %3d %s\n", configHubCount, makeBar(configHubCount, total, 20))
	}
	if nativeCount > 0 {
		fmt.Printf("  Native    %3d %s  ← add to GitOps\n", nativeCount, makeBar(nativeCount, total, 20))
	}

	if nativeCount > 0 {
		fmt.Printf("\n⚠ %d native workload(s) should be added to GitOps\n", nativeCount)
		fmt.Println("  Run: cub-scout map bypass  # to see details")
	}

	return nil
}

// runMapDashboard shows unified health + ownership dashboard
func runMapDashboard(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	fmt.Println("📊 CLUSTER DASHBOARD")
	fmt.Println()

	// === HEALTH SECTION (from runMapStatus logic) ===
	deployersReady, deployersTotal := 0, 0

	// Check Flux Kustomizations
	if ksList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, ks := range ksList.Items {
			deployersTotal++
			if isResourceReady(&ks) {
				deployersReady++
			}
		}
	}

	// Check Flux HelmReleases
	if hrList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, hr := range hrList.Items {
			deployersTotal++
			if isResourceReady(&hr) {
				deployersReady++
			}
		}
	}

	// Check ArgoCD Applications
	if appList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, app := range appList.Items {
			deployersTotal++
			if isArgoAppHealthy(&app) {
				deployersReady++
			}
		}
	}

	// Count workloads
	workloadsReady, workloadsTotal := 0, 0
	var fluxCount, argoCount, helmCount, configHubCount, nativeCount int

	// Check Deployments
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}
			workloadsTotal++
			if isDeploymentReady(&dep) {
				workloadsReady++
			}
			// Count by owner
			owner, _ := detectOwnership(&dep)
			switch owner {
			case "Flux":
				fluxCount++
			case "ArgoCD":
				argoCount++
			case "Helm":
				helmCount++
			case "ConfigHub":
				configHubCount++
			case "Native":
				nativeCount++
			}
		}
	}

	// Health status
	problems := (deployersTotal - deployersReady) + (workloadsTotal - workloadsReady)
	if problems == 0 {
		fmt.Printf("HEALTH: ✓ healthy  %d/%d deployers, %d/%d workloads\n",
			deployersReady, deployersTotal, workloadsReady, workloadsTotal)
	} else {
		fmt.Printf("HEALTH: ✗ %d problem(s)  %d/%d deployers, %d/%d workloads\n",
			problems, deployersReady, deployersTotal, workloadsReady, workloadsTotal)
	}

	// === OWNERSHIP SECTION (from runMapSprawl logic) ===
	total := fluxCount + argoCount + helmCount + configHubCount + nativeCount
	managed := total - nativeCount

	coverage := 0
	if total > 0 {
		coverage = (managed * 100) / total
	}

	fmt.Printf("COVERAGE: %d%% GitOps managed\n", coverage)
	fmt.Println()

	fmt.Println("BY OWNER:")
	if fluxCount > 0 {
		fmt.Printf("  Flux      %3d %s\n", fluxCount, makeBar(fluxCount, total, 20))
	}
	if argoCount > 0 {
		fmt.Printf("  ArgoCD    %3d %s\n", argoCount, makeBar(argoCount, total, 20))
	}
	if helmCount > 0 {
		fmt.Printf("  Helm      %3d %s\n", helmCount, makeBar(helmCount, total, 20))
	}
	if configHubCount > 0 {
		fmt.Printf("  ConfigHub %3d %s\n", configHubCount, makeBar(configHubCount, total, 20))
	}
	if nativeCount > 0 {
		fmt.Printf("  Native    %3d %s  ⚠ SHADOW IT\n", nativeCount, makeBar(nativeCount, total, 20))
	}

	if nativeCount > 0 {
		fmt.Printf("\n⚠ %d native workload(s) - run: cub-scout map bypass\n", nativeCount)
	}

	return nil
}

func runMapBypass(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	fmt.Println("🚧 FACTORY BYPASS DETECTION")
	fmt.Println()
	fmt.Println("NATIVE WORKLOADS (not in GitOps):")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tNAME\tIMAGE")
	fmt.Fprintln(w, "─────────\t────\t─────")

	var nativeCount int

	// Get all Deployments
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}

			owner, _ := detectOwnership(&dep)
			if owner == "Native" {
				nativeCount++
				image := getContainerImage(&dep)
				fmt.Fprintf(w, "%s\t%s\t%s\n", ns, dep.GetName(), image)
			}
		}
	}

	w.Flush()

	if nativeCount == 0 {
		fmt.Println("✓ No native workloads found - all workloads are GitOps managed")
	} else {
		fmt.Printf("\n⚠ %d native workload(s) deployed outside GitOps\n", nativeCount)
		fmt.Println("\nRecommendations:")
		fmt.Println("  1. Add GitOps manifests for these workloads")
		fmt.Println("  2. Or import them: cub-scout map (press 'i' to import)")
	}

	return nil
}

// makeBar creates a simple text bar for sprawl output
func makeBar(count, total, width int) string {
	if total == 0 {
		return strings.Repeat("░", width)
	}
	filled := (count * width) / total
	if filled < 1 && count > 0 {
		filled = 1
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// Helper functions for the new commands

func isResourceReady(obj *unstructured.Unstructured) bool {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return false
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Ready" && cond["status"] == "True" {
			return true
		}
	}
	return false
}

func isArgoAppHealthy(obj *unstructured.Unstructured) bool {
	syncStatus, _, _ := unstructured.NestedString(obj.Object, "status", "sync", "status")
	healthStatus, _, _ := unstructured.NestedString(obj.Object, "status", "health", "status")
	return syncStatus == "Synced" && healthStatus == "Healthy"
}

func isDeploymentReady(obj *unstructured.Unstructured) bool {
	desired, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	available, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	if desired == 0 {
		desired = 1
	}
	return available >= desired
}

func getDeploymentReplicas(obj *unstructured.Unstructured) (int64, int64) {
	desired, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
	available, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
	if desired == 0 {
		desired = 1
	}
	return desired, available
}

func getConditionReason(obj *unstructured.Unstructured) string {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return "Unknown"
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Ready" {
			if reason, ok := cond["reason"].(string); ok {
				return reason
			}
		}
	}
	return "Unknown"
}

// detectStatus determines the status string for a resource
// Returns: "Ready", "NotReady", "Failed", "Pending", "Unknown"
func detectStatus(obj *unstructured.Unstructured) string {
	kind := obj.GetKind()

	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		// Check replica readiness
		desired, _, _ := unstructured.NestedInt64(obj.Object, "spec", "replicas")
		ready, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyReplicas")
		if desired == 0 {
			desired = 1
		}
		if ready >= desired {
			return "Ready"
		}
		// Check if there are any unavailable replicas indicating a problem
		unavailable, _, _ := unstructured.NestedInt64(obj.Object, "status", "unavailableReplicas")
		if unavailable > 0 {
			return "NotReady"
		}
		return "Pending"

	case "Pod":
		phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
		switch phase {
		case "Running", "Succeeded":
			// Check container statuses for crashes
			containerStatuses, found, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses")
			if found {
				for _, cs := range containerStatuses {
					csMap, ok := cs.(map[string]interface{})
					if !ok {
						continue
					}
					// Check for CrashLoopBackOff or other waiting states
					waiting, found, _ := unstructured.NestedMap(csMap, "state", "waiting")
					if found {
						reason, _ := waiting["reason"].(string)
						if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" {
							return "Failed"
						}
						return "Pending"
					}
				}
			}
			return "Ready"
		case "Pending":
			return "Pending"
		case "Failed":
			return "Failed"
		default:
			return "Unknown"
		}

	case "Job":
		// Check if job completed successfully
		succeeded, _, _ := unstructured.NestedInt64(obj.Object, "status", "succeeded")
		failed, _, _ := unstructured.NestedInt64(obj.Object, "status", "failed")
		if succeeded > 0 {
			return "Ready"
		}
		if failed > 0 {
			return "Failed"
		}
		return "Pending"

	case "Service", "ConfigMap", "Secret", "ServiceAccount":
		// These are always "Ready" if they exist
		return "Ready"

	case "Application": // Argo CD Application
		syncStatus, _, _ := unstructured.NestedString(obj.Object, "status", "sync", "status")
		healthStatus, _, _ := unstructured.NestedString(obj.Object, "status", "health", "status")
		if syncStatus == "Synced" && healthStatus == "Healthy" {
			return "Ready"
		}
		if healthStatus == "Degraded" || healthStatus == "Missing" {
			return "Failed"
		}
		return "NotReady"

	case "Kustomization", "HelmRelease", "GitRepository", "HelmRepository":
		// Flux resources use Ready condition
		if isResourceReady(obj) {
			return "Ready"
		}
		// Check for specific failure reasons
		reason := getConditionReason(obj)
		if strings.Contains(reason, "Failed") || strings.Contains(reason, "Error") {
			return "Failed"
		}
		return "NotReady"

	default:
		// Generic check using Ready condition
		if isResourceReady(obj) {
			return "Ready"
		}
		// Check if status exists but not ready
		_, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
		if found {
			return "NotReady"
		}
		// No status conditions - assume Ready for simple resources
		return "Ready"
	}
}

func getArgoStatus(obj *unstructured.Unstructured) string {
	syncStatus, _, _ := unstructured.NestedString(obj.Object, "status", "sync", "status")
	healthStatus, _, _ := unstructured.NestedString(obj.Object, "status", "health", "status")
	return fmt.Sprintf("%s/%s", syncStatus, healthStatus)
}

func getLastAppliedRevision(obj *unstructured.Unstructured) string {
	rev, found, _ := unstructured.NestedString(obj.Object, "status", "lastAppliedRevision")
	if !found || rev == "" {
		return "-"
	}
	// Shorten git revisions
	if len(rev) > 12 {
		parts := strings.Split(rev, "@")
		if len(parts) > 1 {
			return parts[len(parts)-1][:7]
		}
		return rev[:7]
	}
	return rev
}

func getInventoryCount(obj *unstructured.Unstructured) int {
	entries, found, _ := unstructured.NestedSlice(obj.Object, "status", "inventory", "entries")
	if !found {
		return 0
	}
	return len(entries)
}

func getArgoRevision(obj *unstructured.Unstructured) string {
	rev, found, _ := unstructured.NestedString(obj.Object, "status", "sync", "revision")
	if !found || rev == "" {
		return "-"
	}
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}

func getArgoResourceCount(obj *unstructured.Unstructured) int {
	resources, found, _ := unstructured.NestedSlice(obj.Object, "status", "resources")
	if !found {
		return 0
	}
	return len(resources)
}

func detectOwnership(obj *unstructured.Unstructured) (string, string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	// ConfigHub
	if slug, ok := labels["confighub.com/UnitSlug"]; ok {
		return "ConfigHub", slug
	}
	// Flux Kustomize
	if name, ok := labels["kustomize.toolkit.fluxcd.io/name"]; ok {
		return "Flux", name
	}
	// Flux Helm
	if name, ok := labels["helm.toolkit.fluxcd.io/name"]; ok {
		return "Flux", name
	}
	// ArgoCD - check both label and tracking-id annotation
	if instance, ok := labels["argocd.argoproj.io/instance"]; ok {
		return "ArgoCD", instance
	}
	// ArgoCD tracking-id annotation (format: <app-name>:<group>/<kind>:<namespace>/<name>)
	if tracking, ok := annotations["argocd.argoproj.io/tracking-id"]; ok {
		parts := strings.SplitN(tracking, ":", 2)
		if len(parts) > 0 && parts[0] != "" {
			return "ArgoCD", parts[0]
		}
	}
	// Helm
	if labels["app.kubernetes.io/managed-by"] == "Helm" {
		name := annotations["meta.helm.sh/release-name"]
		return "Helm", name
	}
	return "Native", "-"
}

func getContainerImage(obj *unstructured.Unstructured) string {
	containers, found, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if !found || len(containers) == 0 {
		return "-"
	}
	container, ok := containers[0].(map[string]interface{})
	if !ok {
		return "-"
	}
	image, ok := container["image"].(string)
	if !ok {
		return "-"
	}
	// Shorten image name
	parts := strings.Split(image, "/")
	short := parts[len(parts)-1]
	if len(short) > 30 {
		return short[:30]
	}
	return short
}

// runMapCrashes shows crashing pods (focused on pod-level health issues)
func runMapCrashes(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	type crashInfo struct {
		namespace string
		podName   string
		status    string
		restarts  int64
		age       string
	}

	crashes := []crashInfo{}

	// List all pods
	podGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	podList, err := dynClient.Resource(podGVR).List(ctx, v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}

	now := time.Now()

	for _, pod := range podList.Items {
		ns := pod.GetNamespace()
		// Skip system namespaces
		if strings.HasPrefix(ns, "kube-") || ns == "local-path-storage" {
			continue
		}

		// Apply namespace filter if specified
		if mapNamespace != "" && ns != mapNamespace {
			continue
		}

		phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
		containerStatuses, found, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")

		var crashStatus string
		var totalRestarts int64

		// Check for pod-level failures
		if phase == "Failed" {
			reason, _, _ := unstructured.NestedString(pod.Object, "status", "reason")
			if reason != "" {
				crashStatus = reason
			} else {
				crashStatus = "Failed"
			}
		}

		// Check container statuses for crashes
		if found {
			for _, cs := range containerStatuses {
				csMap, ok := cs.(map[string]interface{})
				if !ok {
					continue
				}

				// Count restarts
				restarts, _, _ := unstructured.NestedInt64(csMap, "restartCount")
				totalRestarts += restarts

				// Check waiting state for crash reasons
				waiting, waitFound, _ := unstructured.NestedMap(csMap, "state", "waiting")
				if waitFound {
					reason, _ := waiting["reason"].(string)
					if reason == "CrashLoopBackOff" || reason == "ImagePullBackOff" || reason == "ErrImagePull" {
						crashStatus = reason
					}
				}

				// Check terminated state for OOMKilled
				terminated, termFound, _ := unstructured.NestedMap(csMap, "state", "terminated")
				if termFound {
					reason, _ := terminated["reason"].(string)
					if reason == "OOMKilled" || reason == "Error" {
						crashStatus = reason
					}
				}

				// Check lastState for recent crashes
				lastWaiting, lastWaitFound, _ := unstructured.NestedMap(csMap, "lastState", "waiting")
				if lastWaitFound && crashStatus == "" {
					reason, _ := lastWaiting["reason"].(string)
					if reason == "CrashLoopBackOff" {
						crashStatus = reason
					}
				}

				lastTerminated, lastTermFound, _ := unstructured.NestedMap(csMap, "lastState", "terminated")
				if lastTermFound && crashStatus == "" {
					reason, _ := lastTerminated["reason"].(string)
					if reason == "OOMKilled" || reason == "Error" {
						crashStatus = fmt.Sprintf("recently %s", reason)
					}
				}
			}
		}

		// Only include if there's a crash status or high restart count
		if crashStatus != "" || totalRestarts >= 5 {
			if crashStatus == "" {
				crashStatus = fmt.Sprintf("%d restarts", totalRestarts)
			}

			// Calculate age
			creationTime := pod.GetCreationTimestamp().Time
			age := now.Sub(creationTime)
			var ageStr string
			if age.Hours() >= 24 {
				ageStr = fmt.Sprintf("%dd", int(age.Hours()/24))
			} else if age.Hours() >= 1 {
				ageStr = fmt.Sprintf("%dh", int(age.Hours()))
			} else {
				ageStr = fmt.Sprintf("%dm", int(age.Minutes()))
			}

			crashes = append(crashes, crashInfo{
				namespace: ns,
				podName:   pod.GetName(),
				status:    crashStatus,
				restarts:  totalRestarts,
				age:       ageStr,
			})
		}
	}

	if len(crashes) == 0 {
		fmt.Println("✓ No crashing pods found")
		return nil
	}

	// Print header
	fmt.Println()
	fmt.Println("CRASHING PODS")
	fmt.Println("════════════════════════════════════════════════════════════════════")
	fmt.Println("Pods in CrashLoopBackOff, ImagePullBackOff, OOMKilled, Error, or with high restart counts.")
	fmt.Println()

	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tPOD\tSTATUS\tRESTARTS\tAGE")
	fmt.Fprintln(w, "─────────\t───\t──────\t────────\t───")
	for _, c := range crashes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", c.namespace, c.podName, c.status, c.restarts, c.age)
	}
	w.Flush()

	// Summary
	fmt.Printf("\n%d crashing pods\n", len(crashes))

	// Next steps
	fmt.Println()
	fmt.Println("NEXT STEPS:")
	fmt.Println("→ View logs:     kubectl logs -n <namespace> <pod> --previous")
	fmt.Println("→ Describe pod:  kubectl describe pod -n <namespace> <pod>")
	fmt.Println("→ Trace owner:   cub-scout trace pod/<name> -n <namespace>")

	return nil
}

// runMapOrphans shows Native (unmanaged) resources
func runMapOrphans(cmd *cobra.Command, args []string) error {
	// Print header if in table mode (not --json/--count/--names-only)
	showHeader := !mapJSON && !mapCount && !mapNamesOnly
	if showHeader {
		fmt.Println()
		fmt.Println("ORPHAN RESOURCES")
		fmt.Println("════════════════════════════════════════════════════════════════════")
		fmt.Println("Resources not managed by GitOps (Flux, ArgoCD, Helm, ConfigHub).")
		fmt.Println("These may be: legacy systems, manual hotfixes, debug pods, or shadow IT.")
		fmt.Println()
	}

	// Set the owner filter to Native and run list
	mapOwner = "Native"
	err := runMapList(cmd, args)

	// Print next steps if in table mode and no error
	if showHeader && err == nil {
		fmt.Println()
		fmt.Println("NEXT STEPS:")
		fmt.Println("→ To import into ConfigHub: cub-scout import --wizard")
		fmt.Println("→ To trace ownership:       cub-scout trace <kind>/<name> -n <namespace>")
		fmt.Println("→ Visual guide:             docs/diagrams/ownership-detection.svg")
	}

	return err
}

// completeSince provides tab completion for --since flag
func completeSince(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"1h\tlast hour",
		"2h\tlast 2 hours",
		"6h\tlast 6 hours",
		"12h\tlast 12 hours",
		"24h\tlast day",
		"48h\tlast 2 days",
		"7d\tlast week",
		"30d\tlast month",
	}, cobra.ShellCompDirectiveNoFileComp
}

// ConfigHub connected mode types and helpers

// cubUnitInfo holds ConfigHub unit data for display
type cubUnitInfo struct {
	Slug            string
	HeadRevisionNum int
	TargetSlug      string
	Space           string
	LastApplied     string
	DependsOn       []string // Links this unit depends on
	DependedBy      []string // Links that depend on this unit
	OtherSpaces     []string // Other spaces where this unit slug exists
}

// cubUnitCache holds all units from ConfigHub, indexed by UnitSlug
type cubUnitCache struct {
	units      map[string]*cubUnitInfo
	space      string
	allSpaces  []string            // All available spaces
	crossSpace map[string][]string // unit slug -> list of spaces where it exists
}

// fetchConfigHubUnits fetches all units from ConfigHub
func fetchConfigHubUnits() (*cubUnitCache, error) {
	// Get current context (this will fail if not authenticated)
	ctxCmd := exec.Command("cub", "context", "get", "--json")
	ctxOut, err := ctxCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ConfigHub authentication required.\n\n  To authenticate: cub auth login\n  To use standalone: cub-scout map (without --hub)")
	}
	var ctx struct {
		Settings struct {
			DefaultSpace string `json:"defaultSpace"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(ctxOut, &ctx); err != nil {
		return nil, err
	}
	space := ctx.Settings.DefaultSpace
	if space == "" {
		return nil, fmt.Errorf("no space selected (run 'cub context set --space <name>')")
	}

	// Fetch units
	listCmd := exec.Command("cub", "unit", "list", "--json", "--quiet")
	listOut, err := listCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list units: %w", err)
	}

	// Parse units (cub CLI returns [{Unit: {...}, Space: {...}}])
	var unitList []struct {
		Unit struct {
			Slug            string `json:"Slug"`
			HeadRevisionNum int    `json:"HeadRevisionNum"`
			TargetSlug      string `json:"TargetSlug"`
		} `json:"Unit"`
		Space struct {
			Slug string `json:"Slug"`
		} `json:"Space"`
	}
	if err := json.Unmarshal(listOut, &unitList); err != nil {
		return nil, fmt.Errorf("failed to parse units: %w", err)
	}

	cache := &cubUnitCache{
		units:      make(map[string]*cubUnitInfo),
		space:      space,
		crossSpace: make(map[string][]string),
	}

	for _, u := range unitList {
		cache.units[u.Unit.Slug] = &cubUnitInfo{
			Slug:            u.Unit.Slug,
			HeadRevisionNum: u.Unit.HeadRevisionNum,
			TargetSlug:      u.Unit.TargetSlug,
			Space:           u.Space.Slug,
		}
	}

	// Fetch links for dependency info
	linksCmd := exec.Command("cub", "link", "list", "--json", "--quiet")
	linksOut, err := linksCmd.Output()
	if err == nil {
		var linkList []struct {
			FromUnit struct {
				Slug string `json:"Slug"`
			} `json:"FromUnit"`
			ToUnit struct {
				Slug string `json:"Slug"`
			} `json:"ToUnit"`
		}
		if json.Unmarshal(linksOut, &linkList) == nil {
			for _, l := range linkList {
				if from := cache.units[l.FromUnit.Slug]; from != nil {
					from.DependsOn = append(from.DependsOn, l.ToUnit.Slug)
				}
				if to := cache.units[l.ToUnit.Slug]; to != nil {
					to.DependedBy = append(to.DependedBy, l.FromUnit.Slug)
				}
			}
		}
	}

	// Fetch all spaces for cross-space correlation
	spacesCmd := exec.Command("cub", "space", "list", "--json", "--quiet")
	spacesOut, err := spacesCmd.Output()
	if err == nil {
		var spaceList []struct {
			Space struct {
				Slug string `json:"Slug"`
			} `json:"Space"`
		}
		if json.Unmarshal(spacesOut, &spaceList) == nil {
			for _, s := range spaceList {
				if s.Space.Slug != "" && s.Space.Slug != space {
					cache.allSpaces = append(cache.allSpaces, s.Space.Slug)
				}
			}
		}
	}

	// For each unit in current space, check if it exists in other spaces
	// Only check spaces that look related (same prefix pattern)
	unitSlugs := make([]string, 0, len(cache.units))
	for slug := range cache.units {
		unitSlugs = append(unitSlugs, slug)
	}

	// Check related spaces (same base name with different suffix like -dev, -prod)
	baseSpace := strings.TrimSuffix(strings.TrimSuffix(space, "-dev"), "-prod")
	baseSpace = strings.TrimSuffix(strings.TrimSuffix(baseSpace, "-staging"), "-test")

	for _, otherSpace := range cache.allSpaces {
		otherBase := strings.TrimSuffix(strings.TrimSuffix(otherSpace, "-dev"), "-prod")
		otherBase = strings.TrimSuffix(strings.TrimSuffix(otherBase, "-staging"), "-test")

		// Only check spaces with same base name (e.g., apptique-dev and apptique-prod)
		if otherBase != baseSpace {
			continue
		}

		// Query units in this related space
		otherUnitsCmd := exec.Command("cub", "unit", "list", "--json", "--quiet", "--space", otherSpace)
		otherUnitsOut, err := otherUnitsCmd.Output()
		if err != nil {
			continue
		}

		var otherUnitList []struct {
			Unit struct {
				Slug string `json:"Slug"`
			} `json:"Unit"`
		}
		if json.Unmarshal(otherUnitsOut, &otherUnitList) != nil {
			continue
		}

		// Build set of unit slugs in other space
		otherSlugs := make(map[string]bool)
		for _, u := range otherUnitList {
			otherSlugs[u.Unit.Slug] = true
		}

		// Check which of our units exist in this space
		for _, slug := range unitSlugs {
			if otherSlugs[slug] {
				cache.crossSpace[slug] = append(cache.crossSpace[slug], otherSpace)
			}
		}
	}

	// Add cross-space info to each unit
	for slug, unit := range cache.units {
		if spaces, ok := cache.crossSpace[slug]; ok {
			unit.OtherSpaces = spaces
		}
	}

	return cache, nil
}

// getUnitBySlug returns unit info if found
func (c *cubUnitCache) getUnitBySlug(slug string) *cubUnitInfo {
	if c == nil {
		return nil
	}
	return c.units[slug]
}

// printConfigHubContext prints ConfigHub context for a resource
func printConfigHubContext(unit *cubUnitInfo) {
	if unit == nil {
		return
	}
	fmt.Printf("  ╭─ ConfigHub ─────────────────────────────────────────────────────────────────╮\n")
	fmt.Printf("  │ Unit:     %-64s │\n", unit.Slug)
	fmt.Printf("  │ Revision: r%-63d │\n", unit.HeadRevisionNum)
	if unit.TargetSlug != "" {
		fmt.Printf("  │ Target:   %-64s │\n", unit.TargetSlug)
	}
	if len(unit.DependsOn) > 0 {
		deps := strings.Join(unit.DependsOn, ", ")
		if len(deps) > 60 {
			deps = deps[:57] + "..."
		}
		fmt.Printf("  │ Depends:  → %-62s │\n", deps)
	}
	if len(unit.DependedBy) > 0 {
		deps := strings.Join(unit.DependedBy, ", ")
		if len(deps) > 60 {
			deps = deps[:57] + "..."
		}
		fmt.Printf("  │ UsedBy:   ← %-62s │\n", deps)
	}
	if len(unit.OtherSpaces) > 0 {
		spaces := strings.Join(unit.OtherSpaces, ", ")
		if len(spaces) > 60 {
			spaces = spaces[:57] + "..."
		}
		fmt.Printf("  │ Also in:  %-64s │\n", spaces)
	}
	fmt.Printf("  ╰────────────────────────────────────────────────────────────────────────────╯\n")
}

// runMapClusterData shows all data sources read from cluster with MAXIMUM detail
// CLI equivalent of TUI's '4' key (Cluster Data view)
func runMapClusterData(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	// Connected mode: fetch ConfigHub data
	var unitCache *cubUnitCache
	if deepDiveConnected {
		var connErr error
		unitCache, connErr = fetchConfigHubUnits()
		if connErr != nil {
			fmt.Printf("⚠ Connected mode failed: %v\n", connErr)
			fmt.Println("  Falling back to standalone mode. Run 'cub auth login' to enable.")
			fmt.Println()
			deepDiveConnected = false
		}
	}

	// Header changes based on mode
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	if deepDiveConnected && unitCache != nil {
		fmt.Printf("                    DEEP DIVE (CONNECTED TO CONFIGHUB: %s)\n", unitCache.space)
	} else {
		fmt.Println("                         CLUSTER DATA SOURCES (STANDALONE MODE)")
	}
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	if deepDiveConnected && unitCache != nil {
		fmt.Printf("Connected to ConfigHub space '%s' - showing Unit context for managed resources.\n", unitCache.space)
		fmt.Printf("Units in space: %d | Links available for dependency tracking\n", len(unitCache.units))
	} else {
		fmt.Println("This shows ALL information readable from the cluster without ConfigHub connection.")
		fmt.Println("Use --connected flag to show ConfigHub Unit context for managed resources.")
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// FLUX GITREPOSITORIES
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("FLUX GITREPOSITORIES (Source of Truth)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	fluxInstalled := false
	if gitList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories",
	}).List(ctx, v1.ListOptions{}); err == nil {
		fluxInstalled = true
		if len(gitList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, gr := range gitList.Items {
			name := gr.GetName()
			ns := gr.GetNamespace()
			url, _, _ := unstructured.NestedString(gr.Object, "spec", "url")
			branch, _, _ := unstructured.NestedString(gr.Object, "spec", "ref", "branch")
			tag, _, _ := unstructured.NestedString(gr.Object, "spec", "ref", "tag")
			semver, _, _ := unstructured.NestedString(gr.Object, "spec", "ref", "semver")
			commit, _, _ := unstructured.NestedString(gr.Object, "spec", "ref", "commit")
			interval, _, _ := unstructured.NestedString(gr.Object, "spec", "interval")
			timeout, _, _ := unstructured.NestedString(gr.Object, "spec", "timeout")
			secretRef, _, _ := unstructured.NestedString(gr.Object, "spec", "secretRef", "name")
			ignore, _, _ := unstructured.NestedString(gr.Object, "spec", "ignore")

			// Status - artifact
			revision, _, _ := unstructured.NestedString(gr.Object, "status", "artifact", "revision")
			checksum, _, _ := unstructured.NestedString(gr.Object, "status", "artifact", "digest")
			lastFetch, _, _ := unstructured.NestedString(gr.Object, "status", "artifact", "lastUpdateTime")
			artifactPath, _, _ := unstructured.NestedString(gr.Object, "status", "artifact", "path")
			artifactSize, _, _ := unstructured.NestedInt64(gr.Object, "status", "artifact", "size")
			observedGen, _, _ := unstructured.NestedInt64(gr.Object, "status", "observedGeneration")

			// Conditions
			conditions, _, _ := unstructured.NestedSlice(gr.Object, "status", "conditions")

			icon := "✓"
			statusMsg := "Ready"
			if !isResourceReady(&gr) {
				icon = "✗"
				statusMsg = getConditionMessage(&gr)
			}

			fmt.Printf("\n%s %s/%s\n", icon, ns, name)
			fmt.Printf("  URL:        %s\n", url)
			if branch != "" {
				fmt.Printf("  Branch:     %s\n", branch)
			}
			if tag != "" {
				fmt.Printf("  Tag:        %s\n", tag)
			}
			if semver != "" {
				fmt.Printf("  Semver:     %s\n", semver)
			}
			if commit != "" {
				fmt.Printf("  Commit:     %s\n", commit)
			}
			fmt.Printf("  Interval:   %s\n", interval)
			if timeout != "" {
				fmt.Printf("  Timeout:    %s\n", timeout)
			}
			if secretRef != "" {
				fmt.Printf("  Auth:       Secret/%s\n", secretRef)
			} else {
				fmt.Printf("  Auth:       (public)\n")
			}
			if ignore != "" {
				fmt.Printf("  Ignore:     %s\n", ignore)
			}
			fmt.Printf("  Status:     %s\n", statusMsg)
			if revision != "" {
				fmt.Printf("  Revision:   %s\n", revision)
			}
			if checksum != "" {
				fmt.Printf("  Checksum:   %s\n", checksum)
			}
			if artifactSize > 0 {
				fmt.Printf("  Size:       %d bytes\n", artifactSize)
			}
			if artifactPath != "" {
				fmt.Printf("  Path:       %s\n", artifactPath)
			}
			if lastFetch != "" {
				fmt.Printf("  LastFetch:  %s\n", lastFetch)
			}
			if observedGen > 0 {
				fmt.Printf("  Generation: %d\n", observedGen)
			}
			// Show all conditions
			if len(conditions) > 0 {
				fmt.Printf("  Conditions:\n")
				for _, c := range conditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					condType, _ := cond["type"].(string)
					condStatus, _ := cond["status"].(string)
					condReason, _ := cond["reason"].(string)
					condTime, _ := cond["lastTransitionTime"].(string)
					fmt.Printf("    %s: %s (%s) @ %s\n", condType, condStatus, condReason, condTime)
				}
			}
		}
	} else {
		fmt.Println("  (Flux source CRDs not installed)")
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// FLUX HELMREPOSITORIES
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("FLUX HELMREPOSITORIES")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	if helmRepoList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories",
	}).List(ctx, v1.ListOptions{}); err == nil {
		fluxInstalled = true
		if len(helmRepoList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, hr := range helmRepoList.Items {
			name := hr.GetName()
			ns := hr.GetNamespace()
			url, _, _ := unstructured.NestedString(hr.Object, "spec", "url")
			repoType, _, _ := unstructured.NestedString(hr.Object, "spec", "type")
			interval, _, _ := unstructured.NestedString(hr.Object, "spec", "interval")
			secretRef, _, _ := unstructured.NestedString(hr.Object, "spec", "secretRef", "name")
			passCredentials, _, _ := unstructured.NestedBool(hr.Object, "spec", "passCredentials")

			// Status
			artifactRevision, _, _ := unstructured.NestedString(hr.Object, "status", "artifact", "revision")
			artifactChecksum, _, _ := unstructured.NestedString(hr.Object, "status", "artifact", "digest")
			conditions, _, _ := unstructured.NestedSlice(hr.Object, "status", "conditions")

			icon := "✓"
			statusMsg := "Ready"
			if !isResourceReady(&hr) {
				icon = "✗"
				statusMsg = getConditionMessage(&hr)
			}

			fmt.Printf("\n%s %s/%s\n", icon, ns, name)
			fmt.Printf("  URL:        %s\n", url)
			if repoType != "" {
				fmt.Printf("  Type:       %s\n", repoType)
			}
			fmt.Printf("  Interval:   %s\n", interval)
			if secretRef != "" {
				fmt.Printf("  Auth:       Secret/%s\n", secretRef)
			}
			if passCredentials {
				fmt.Printf("  PassCreds:  true\n")
			}
			fmt.Printf("  Status:     %s\n", statusMsg)
			if artifactRevision != "" {
				fmt.Printf("  Revision:   %s\n", artifactRevision)
			}
			if artifactChecksum != "" {
				fmt.Printf("  Checksum:   %s\n", artifactChecksum)
			}
			if len(conditions) > 0 {
				fmt.Printf("  Conditions:\n")
				for _, c := range conditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					condType, _ := cond["type"].(string)
					condStatus, _ := cond["status"].(string)
					condReason, _ := cond["reason"].(string)
					condTime, _ := cond["lastTransitionTime"].(string)
					fmt.Printf("    %s: %s (%s) @ %s\n", condType, condStatus, condReason, condTime)
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// FLUX OCIREPOSITORIES
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("FLUX OCIREPOSITORIES")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	if ociRepoList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories",
	}).List(ctx, v1.ListOptions{}); err == nil {
		fluxInstalled = true
		if len(ociRepoList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, oci := range ociRepoList.Items {
			name := oci.GetName()
			ns := oci.GetNamespace()
			url, _, _ := unstructured.NestedString(oci.Object, "spec", "url")
			ref, _, _ := unstructured.NestedString(oci.Object, "spec", "ref", "tag")
			if ref == "" {
				ref, _, _ = unstructured.NestedString(oci.Object, "spec", "ref", "semver")
			}
			if ref == "" {
				ref, _, _ = unstructured.NestedString(oci.Object, "spec", "ref", "digest")
			}
			interval, _, _ := unstructured.NestedString(oci.Object, "spec", "interval")
			secretRef, _, _ := unstructured.NestedString(oci.Object, "spec", "secretRef", "name")
			provider, _, _ := unstructured.NestedString(oci.Object, "spec", "provider")

			// Status
			artifactRevision, _, _ := unstructured.NestedString(oci.Object, "status", "artifact", "revision")
			artifactChecksum, _, _ := unstructured.NestedString(oci.Object, "status", "artifact", "digest")
			conditions, _, _ := unstructured.NestedSlice(oci.Object, "status", "conditions")

			icon := "✓"
			statusMsg := "Ready"
			if !isResourceReady(&oci) {
				icon = "✗"
				statusMsg = getConditionMessage(&oci)
			}

			fmt.Printf("\n%s %s/%s\n", icon, ns, name)
			fmt.Printf("  URL:        %s\n", url)
			if ref != "" {
				fmt.Printf("  Ref:        %s\n", ref)
			}
			fmt.Printf("  Interval:   %s\n", interval)
			if provider != "" && provider != "generic" {
				fmt.Printf("  Provider:   %s\n", provider)
			}
			if secretRef != "" {
				fmt.Printf("  Auth:       Secret/%s\n", secretRef)
			}
			fmt.Printf("  Status:     %s\n", statusMsg)
			if artifactRevision != "" {
				fmt.Printf("  Revision:   %s\n", artifactRevision)
			}
			if artifactChecksum != "" {
				fmt.Printf("  Checksum:   %s\n", artifactChecksum)
			}
			if len(conditions) > 0 {
				fmt.Printf("  Conditions:\n")
				for _, c := range conditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					condType, _ := cond["type"].(string)
					condStatus, _ := cond["status"].(string)
					condReason, _ := cond["reason"].(string)
					condTime, _ := cond["lastTransitionTime"].(string)
					fmt.Printf("    %s: %s (%s) @ %s\n", condType, condStatus, condReason, condTime)
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// FLUX BUCKETS
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("FLUX BUCKETS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	if bucketList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "buckets",
	}).List(ctx, v1.ListOptions{}); err == nil {
		fluxInstalled = true
		if len(bucketList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, bucket := range bucketList.Items {
			name := bucket.GetName()
			ns := bucket.GetNamespace()
			bucketName, _, _ := unstructured.NestedString(bucket.Object, "spec", "bucketName")
			endpoint, _, _ := unstructured.NestedString(bucket.Object, "spec", "endpoint")
			provider, _, _ := unstructured.NestedString(bucket.Object, "spec", "provider")
			region, _, _ := unstructured.NestedString(bucket.Object, "spec", "region")
			interval, _, _ := unstructured.NestedString(bucket.Object, "spec", "interval")
			secretRef, _, _ := unstructured.NestedString(bucket.Object, "spec", "secretRef", "name")
			insecure, _, _ := unstructured.NestedBool(bucket.Object, "spec", "insecure")

			// Status
			artifactRevision, _, _ := unstructured.NestedString(bucket.Object, "status", "artifact", "revision")
			artifactChecksum, _, _ := unstructured.NestedString(bucket.Object, "status", "artifact", "digest")
			conditions, _, _ := unstructured.NestedSlice(bucket.Object, "status", "conditions")

			icon := "✓"
			statusMsg := "Ready"
			if !isResourceReady(&bucket) {
				icon = "✗"
				statusMsg = getConditionMessage(&bucket)
			}

			fmt.Printf("\n%s %s/%s\n", icon, ns, name)
			fmt.Printf("  Bucket:     %s\n", bucketName)
			fmt.Printf("  Endpoint:   %s\n", endpoint)
			if provider != "" {
				fmt.Printf("  Provider:   %s\n", provider)
			}
			if region != "" {
				fmt.Printf("  Region:     %s\n", region)
			}
			fmt.Printf("  Interval:   %s\n", interval)
			if secretRef != "" {
				fmt.Printf("  Auth:       Secret/%s\n", secretRef)
			}
			if insecure {
				fmt.Printf("  Insecure:   true\n")
			}
			fmt.Printf("  Status:     %s\n", statusMsg)
			if artifactRevision != "" {
				fmt.Printf("  Revision:   %s\n", artifactRevision)
			}
			if artifactChecksum != "" {
				fmt.Printf("  Checksum:   %s\n", artifactChecksum)
			}
			if len(conditions) > 0 {
				fmt.Printf("  Conditions:\n")
				for _, c := range conditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					condType, _ := cond["type"].(string)
					condStatus, _ := cond["status"].(string)
					condReason, _ := cond["reason"].(string)
					condTime, _ := cond["lastTransitionTime"].(string)
					fmt.Printf("    %s: %s (%s) @ %s\n", condType, condStatus, condReason, condTime)
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// FLUX HELMCHARTS
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("FLUX HELMCHARTS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	if helmChartList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmcharts",
	}).List(ctx, v1.ListOptions{}); err == nil {
		fluxInstalled = true
		if len(helmChartList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, hc := range helmChartList.Items {
			name := hc.GetName()
			ns := hc.GetNamespace()
			chart, _, _ := unstructured.NestedString(hc.Object, "spec", "chart")
			version, _, _ := unstructured.NestedString(hc.Object, "spec", "version")
			sourceKind, _, _ := unstructured.NestedString(hc.Object, "spec", "sourceRef", "kind")
			sourceName, _, _ := unstructured.NestedString(hc.Object, "spec", "sourceRef", "name")
			sourceNs, _, _ := unstructured.NestedString(hc.Object, "spec", "sourceRef", "namespace")
			if sourceNs == "" {
				sourceNs = ns
			}
			interval, _, _ := unstructured.NestedString(hc.Object, "spec", "interval")
			reconcileStrategy, _, _ := unstructured.NestedString(hc.Object, "spec", "reconcileStrategy")

			// Status
			artifactRevision, _, _ := unstructured.NestedString(hc.Object, "status", "artifact", "revision")
			artifactChecksum, _, _ := unstructured.NestedString(hc.Object, "status", "artifact", "digest")
			observedChartName, _, _ := unstructured.NestedString(hc.Object, "status", "observedChartName")
			conditions, _, _ := unstructured.NestedSlice(hc.Object, "status", "conditions")

			icon := "✓"
			statusMsg := "Ready"
			if !isResourceReady(&hc) {
				icon = "✗"
				statusMsg = getConditionMessage(&hc)
			}

			fmt.Printf("\n%s %s/%s\n", icon, ns, name)
			fmt.Printf("  Chart:      %s\n", chart)
			if version != "" {
				fmt.Printf("  Version:    %s\n", version)
			}
			fmt.Printf("  Source:     %s/%s/%s\n", sourceNs, sourceKind, sourceName)
			fmt.Printf("  Interval:   %s\n", interval)
			if reconcileStrategy != "" {
				fmt.Printf("  Strategy:   %s\n", reconcileStrategy)
			}
			fmt.Printf("  Status:     %s\n", statusMsg)
			if observedChartName != "" {
				fmt.Printf("  Observed:   %s\n", observedChartName)
			}
			if artifactRevision != "" {
				fmt.Printf("  Revision:   %s\n", artifactRevision)
			}
			if artifactChecksum != "" {
				fmt.Printf("  Checksum:   %s\n", artifactChecksum)
			}
			if len(conditions) > 0 {
				fmt.Printf("  Conditions:\n")
				for _, c := range conditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					condType, _ := cond["type"].(string)
					condStatus, _ := cond["status"].(string)
					condReason, _ := cond["reason"].(string)
					condTime, _ := cond["lastTransitionTime"].(string)
					fmt.Printf("    %s: %s (%s) @ %s\n", condType, condStatus, condReason, condTime)
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// FLUX KUSTOMIZATIONS
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("FLUX KUSTOMIZATIONS (Deployers)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	if ksList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
	}).List(ctx, v1.ListOptions{}); err == nil {
		fluxInstalled = true
		if len(ksList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, ks := range ksList.Items {
			name := ks.GetName()
			ns := ks.GetNamespace()
			path, _, _ := unstructured.NestedString(ks.Object, "spec", "path")
			sourceKind, _, _ := unstructured.NestedString(ks.Object, "spec", "sourceRef", "kind")
			sourceName, _, _ := unstructured.NestedString(ks.Object, "spec", "sourceRef", "name")
			sourceNs, _, _ := unstructured.NestedString(ks.Object, "spec", "sourceRef", "namespace")
			if sourceNs == "" {
				sourceNs = ns
			}
			targetNs, _, _ := unstructured.NestedString(ks.Object, "spec", "targetNamespace")
			interval, _, _ := unstructured.NestedString(ks.Object, "spec", "interval")
			timeout, _, _ := unstructured.NestedString(ks.Object, "spec", "timeout")
			retryInterval, _, _ := unstructured.NestedString(ks.Object, "spec", "retryInterval")
			prune, _, _ := unstructured.NestedBool(ks.Object, "spec", "prune")
			force, _, _ := unstructured.NestedBool(ks.Object, "spec", "force")
			wait, _, _ := unstructured.NestedBool(ks.Object, "spec", "wait")
			suspended, _, _ := unstructured.NestedBool(ks.Object, "spec", "suspend")

			// Patches
			patches, _, _ := unstructured.NestedSlice(ks.Object, "spec", "patches")
			patchesStrategicMerge, _, _ := unstructured.NestedSlice(ks.Object, "spec", "patchesStrategicMerge")
			patchesJSON6902, _, _ := unstructured.NestedSlice(ks.Object, "spec", "patchesJson6902")

			// Health checks
			healthChecks, _, _ := unstructured.NestedSlice(ks.Object, "spec", "healthChecks")

			// Status
			lastApplied, _, _ := unstructured.NestedString(ks.Object, "status", "lastAppliedRevision")
			lastAttempted, _, _ := unstructured.NestedString(ks.Object, "status", "lastAttemptedRevision")
			lastReconcile, _, _ := unstructured.NestedString(ks.Object, "status", "lastHandledReconcileAt")
			observedGen, _, _ := unstructured.NestedInt64(ks.Object, "status", "observedGeneration")

			// Inventory
			inventory, _, _ := unstructured.NestedSlice(ks.Object, "status", "inventory", "entries")

			// Conditions
			conditions, _, _ := unstructured.NestedSlice(ks.Object, "status", "conditions")

			icon := "✓"
			statusMsg := "Ready"
			if !isResourceReady(&ks) {
				icon = "✗"
				statusMsg = getConditionMessage(&ks)
			}

			fmt.Printf("\n%s %s/%s\n", icon, ns, name)
			fmt.Printf("  Source:     %s/%s/%s\n", sourceNs, sourceKind, sourceName)
			fmt.Printf("  Path:       %s\n", path)
			if targetNs != "" {
				fmt.Printf("  TargetNS:   %s\n", targetNs)
			}
			fmt.Printf("  Interval:   %s\n", interval)
			if timeout != "" {
				fmt.Printf("  Timeout:    %s\n", timeout)
			}
			if retryInterval != "" {
				fmt.Printf("  Retry:      %s\n", retryInterval)
			}
			fmt.Printf("  Prune:      %v | Force: %v | Wait: %v\n", prune, force, wait)
			if suspended {
				fmt.Printf("  Suspended:  TRUE (reconciliation paused)\n")
			}

			// Show patches
			totalPatches := len(patches) + len(patchesStrategicMerge) + len(patchesJSON6902)
			if totalPatches > 0 {
				fmt.Printf("  Patches:    %d defined\n", totalPatches)
				for _, p := range patches {
					if patchMap, ok := p.(map[string]interface{}); ok {
						target, _ := patchMap["target"].(map[string]interface{})
						targetKind, _ := target["kind"].(string)
						targetName, _ := target["name"].(string)
						fmt.Printf("              - target: %s/%s\n", targetKind, targetName)
					}
				}
			}

			// Show health checks
			if len(healthChecks) > 0 {
				fmt.Printf("  HealthChks: %d defined\n", len(healthChecks))
			}

			fmt.Printf("  Status:     %s\n", statusMsg)
			if lastApplied != "" {
				fmt.Printf("  Applied:    %s\n", lastApplied)
			}
			if lastAttempted != "" && lastAttempted != lastApplied {
				fmt.Printf("  Attempted:  %s (DRIFT!)\n", lastAttempted)
			}
			if lastReconcile != "" {
				fmt.Printf("  Reconciled: %s\n", lastReconcile)
			}
			if observedGen > 0 {
				fmt.Printf("  Generation: %d\n", observedGen)
			}
			fmt.Printf("  Inventory:  %d resources\n", len(inventory))
			// Show inventory with API versions
			for i, item := range inventory {
				if i >= 5 {
					fmt.Printf("              ... and %d more\n", len(inventory)-5)
					break
				}
				if itemMap, ok := item.(map[string]interface{}); ok {
					id, _ := itemMap["id"].(string)
					version, _ := itemMap["v"].(string)
					if version != "" {
						fmt.Printf("              - %s (%s)\n", id, version)
					} else {
						fmt.Printf("              - %s\n", id)
					}
				}
			}
			// Show all conditions
			if len(conditions) > 0 {
				fmt.Printf("  Conditions:\n")
				for _, c := range conditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					condType, _ := cond["type"].(string)
					condStatus, _ := cond["status"].(string)
					condReason, _ := cond["reason"].(string)
					condTime, _ := cond["lastTransitionTime"].(string)
					fmt.Printf("    %s: %s (%s) @ %s\n", condType, condStatus, condReason, condTime)
				}
			}

			// LiveTree: Build Deployment → ReplicaSet → Pod tree from inventory
			// Parse inventory to find Deployments and show their live state
			for _, item := range inventory {
				itemMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				id, _ := itemMap["id"].(string)
				// ID format: namespace_name_group_kind (e.g., "flux-demo_podinfo_apps_Deployment")
				parts := strings.Split(id, "_")
				if len(parts) < 4 {
					continue
				}
				// Check if it's a Deployment
				kind := parts[len(parts)-1]
				if kind != "Deployment" {
					continue
				}
				deployNs := parts[0]
				deployName := parts[1]

				// Get the deployment to find its ReplicaSets
				deploy, err := dynClient.Resource(schema.GroupVersionResource{
					Group: "apps", Version: "v1", Resource: "deployments",
				}).Namespace(deployNs).Get(ctx, deployName, v1.GetOptions{})
				if err != nil {
					continue
				}

				// Get deployment status for current ReplicaSet info
				deployConditions, _, _ := unstructured.NestedSlice(deploy.Object, "status", "conditions")
				var currentRS string
				for _, c := range deployConditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					if cond["type"] == "Progressing" {
						msg, _ := cond["message"].(string)
						if strings.Contains(msg, "ReplicaSet") {
							msgParts := strings.Split(msg, "\"")
							if len(msgParts) >= 2 {
								currentRS = msgParts[1]
							}
						}
					}
				}

				if currentRS == "" {
					continue
				}

				// Get the ReplicaSet
				rs, err := dynClient.Resource(schema.GroupVersionResource{
					Group: "apps", Version: "v1", Resource: "replicasets",
				}).Namespace(deployNs).Get(ctx, currentRS, v1.GetOptions{})
				if err != nil {
					continue
				}

				rsReplicas, _, _ := unstructured.NestedInt64(rs.Object, "status", "replicas")
				rsReady, _, _ := unstructured.NestedInt64(rs.Object, "status", "readyReplicas")

				fmt.Printf("  LiveTree:   %s/%s\n", deployNs, deployName)
				fmt.Printf("              └─ ReplicaSet/%s (%d/%d ready)\n", currentRS, rsReady, rsReplicas)

				// Get Pods owned by this ReplicaSet (filter by ownerReference)
				rsUID := rs.GetUID()
				podList, err := dynClient.Resource(schema.GroupVersionResource{
					Group: "", Version: "v1", Resource: "pods",
				}).Namespace(deployNs).List(ctx, v1.ListOptions{})
				if err != nil {
					continue
				}

				// Filter pods to only those owned by this specific ReplicaSet
				var matchingPods []unstructured.Unstructured
				for _, pod := range podList.Items {
					owners, _, _ := unstructured.NestedSlice(pod.Object, "metadata", "ownerReferences")
					for _, owner := range owners {
						if ownerMap, ok := owner.(map[string]interface{}); ok {
							if uid, _ := ownerMap["uid"].(string); uid == string(rsUID) {
								matchingPods = append(matchingPods, pod)
								break
							}
						}
					}
				}

				for i, pod := range matchingPods {
					podName := pod.GetName()
					podPhase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
					podIP, _, _ := unstructured.NestedString(pod.Object, "status", "podIP")
					nodeName, _, _ := unstructured.NestedString(pod.Object, "spec", "nodeName")

					// Get container statuses for restart count
					containerStatuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
					restarts := int64(0)
					for _, cs := range containerStatuses {
						if csMap, ok := cs.(map[string]interface{}); ok {
							if r, ok := csMap["restartCount"].(int64); ok {
								restarts += r
							} else if r, ok := csMap["restartCount"].(float64); ok {
								restarts += int64(r)
							}
						}
					}

					connector := "├─"
					if i == len(matchingPods)-1 {
						connector = "└─"
					}

					podIcon := "✓"
					if podPhase != "Running" {
						podIcon = "✗"
					}

					restartStr := ""
					if restarts > 0 {
						restartStr = fmt.Sprintf(", %d restarts", restarts)
					}

					fmt.Printf("                 %s %s Pod/%s (%s, %s%s)\n",
						connector, podIcon, podName, podPhase, podIP, restartStr)
					if nodeName != "" {
						fmt.Printf("                    node: %s\n", nodeName)
					}
				}
			}
		}
	} else {
		fmt.Println("  (Flux kustomize CRDs not installed)")
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// FLUX HELMRELEASES
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("FLUX HELMRELEASES")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	if hrList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases",
	}).List(ctx, v1.ListOptions{}); err == nil {
		fluxInstalled = true
		if len(hrList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, hr := range hrList.Items {
			name := hr.GetName()
			ns := hr.GetNamespace()
			chartName, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "chart")
			chartVersion, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "version")
			sourceKind, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "sourceRef", "kind")
			sourceName, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "sourceRef", "name")
			targetNs, _, _ := unstructured.NestedString(hr.Object, "spec", "targetNamespace")
			interval, _, _ := unstructured.NestedString(hr.Object, "spec", "interval")

			// Values
			values, _, _ := unstructured.NestedMap(hr.Object, "spec", "values")
			valuesFrom, _, _ := unstructured.NestedSlice(hr.Object, "spec", "valuesFrom")

			// Status
			lastApplied, _, _ := unstructured.NestedString(hr.Object, "status", "lastAppliedRevision")
			lastReleaseRev, _, _ := unstructured.NestedInt64(hr.Object, "status", "lastReleaseRevision")

			icon := "✓"
			statusMsg := "Ready"
			if !isResourceReady(&hr) {
				icon = "✗"
				statusMsg = getConditionMessage(&hr)
			}

			fmt.Printf("\n%s %s/%s\n", icon, ns, name)
			fmt.Printf("  Chart:      %s (version: %s)\n", chartName, chartVersion)
			fmt.Printf("  Source:     %s/%s\n", sourceKind, sourceName)
			if targetNs != "" {
				fmt.Printf("  TargetNS:   %s\n", targetNs)
			}
			fmt.Printf("  Interval:   %s\n", interval)
			fmt.Printf("  Status:     %s\n", statusMsg)
			if lastApplied != "" {
				fmt.Printf("  Revision:   %s (release #%d)\n", lastApplied, lastReleaseRev)
			}
			fmt.Printf("  Values:     %d inline keys, %d external sources\n", len(values), len(valuesFrom))
		}
	} else {
		fmt.Println("  (Flux helm CRDs not installed)")
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// ARGOCD APPPROJECTS
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("ARGOCD APPPROJECTS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	argoInstalled := false
	if projList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "appprojects",
	}).List(ctx, v1.ListOptions{}); err == nil {
		argoInstalled = true
		if len(projList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, proj := range projList.Items {
			name := proj.GetName()
			ns := proj.GetNamespace()

			// Spec
			sourceRepos, _, _ := unstructured.NestedStringSlice(proj.Object, "spec", "sourceRepos")
			destinations, _, _ := unstructured.NestedSlice(proj.Object, "spec", "destinations")
			clusterWhitelist, _, _ := unstructured.NestedSlice(proj.Object, "spec", "clusterResourceWhitelist")
			namespaceWhitelist, _, _ := unstructured.NestedSlice(proj.Object, "spec", "namespaceResourceWhitelist")
			roles, _, _ := unstructured.NestedSlice(proj.Object, "spec", "roles")
			description, _, _ := unstructured.NestedString(proj.Object, "spec", "description")

			fmt.Printf("\n✓ %s/%s\n", ns, name)
			if description != "" {
				fmt.Printf("  Desc:       %s\n", description)
			}
			fmt.Printf("  SourceRepos: %d allowed\n", len(sourceRepos))
			for _, repo := range sourceRepos {
				fmt.Printf("              - %s\n", repo)
			}
			fmt.Printf("  Destinations: %d allowed\n", len(destinations))
			for _, dest := range destinations {
				if destMap, ok := dest.(map[string]interface{}); ok {
					server, _ := destMap["server"].(string)
					namespace, _ := destMap["namespace"].(string)
					fmt.Printf("              - %s → %s\n", server, namespace)
				}
			}
			if len(clusterWhitelist) > 0 {
				fmt.Printf("  ClusterRes: %d whitelisted\n", len(clusterWhitelist))
			}
			if len(namespaceWhitelist) > 0 {
				fmt.Printf("  NsRes:      %d whitelisted\n", len(namespaceWhitelist))
			}
			if len(roles) > 0 {
				fmt.Printf("  Roles:      %d defined\n", len(roles))
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// ARGOCD APPLICATIONS
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("ARGOCD APPLICATIONS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	if appList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
	}).List(ctx, v1.ListOptions{}); err == nil {
		argoInstalled = true
		if len(appList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, app := range appList.Items {
			name := app.GetName()
			ns := app.GetNamespace()
			generation := app.GetGeneration()

			// Project
			project, _, _ := unstructured.NestedString(app.Object, "spec", "project")

			// Source
			repoURL, _, _ := unstructured.NestedString(app.Object, "spec", "source", "repoURL")
			path, _, _ := unstructured.NestedString(app.Object, "spec", "source", "path")
			targetRev, _, _ := unstructured.NestedString(app.Object, "spec", "source", "targetRevision")
			chart, _, _ := unstructured.NestedString(app.Object, "spec", "source", "chart")

			// Destination
			destServer, _, _ := unstructured.NestedString(app.Object, "spec", "destination", "server")
			destNs, _, _ := unstructured.NestedString(app.Object, "spec", "destination", "namespace")

			// Sync policy
			automated, hasAutomated, _ := unstructured.NestedMap(app.Object, "spec", "syncPolicy", "automated")
			prune := false
			selfHeal := false
			if hasAutomated {
				prune, _, _ = unstructured.NestedBool(app.Object, "spec", "syncPolicy", "automated", "prune")
				selfHeal, _, _ = unstructured.NestedBool(app.Object, "spec", "syncPolicy", "automated", "selfHeal")
			}
			syncOptions, _, _ := unstructured.NestedStringSlice(app.Object, "spec", "syncPolicy", "syncOptions")

			// Status
			syncStatus, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")
			syncRevision, _, _ := unstructured.NestedString(app.Object, "status", "sync", "revision")
			healthStatus, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
			healthMessage, _, _ := unstructured.NestedString(app.Object, "status", "health", "message")
			healthLastTransition, _, _ := unstructured.NestedString(app.Object, "status", "health", "lastTransitionTime")
			reconciledAt, _, _ := unstructured.NestedString(app.Object, "status", "reconciledAt")
			sourceType, _, _ := unstructured.NestedString(app.Object, "status", "sourceType")

			// Summary
			summaryImages, _, _ := unstructured.NestedStringSlice(app.Object, "status", "summary", "images")

			// Resources
			resources, _, _ := unstructured.NestedSlice(app.Object, "status", "resources")

			// Operation state
			opPhase, _, _ := unstructured.NestedString(app.Object, "status", "operationState", "phase")
			opMessage, _, _ := unstructured.NestedString(app.Object, "status", "operationState", "message")
			opStartedAt, _, _ := unstructured.NestedString(app.Object, "status", "operationState", "startedAt")
			opFinishedAt, _, _ := unstructured.NestedString(app.Object, "status", "operationState", "finishedAt")
			opInitiatedBy, _, _ := unstructured.NestedBool(app.Object, "status", "operationState", "operation", "initiatedBy", "automated")

			// History
			history, _, _ := unstructured.NestedSlice(app.Object, "status", "history")

			icon := "✓"
			if syncStatus != "Synced" || healthStatus != "Healthy" {
				icon = "✗"
			}

			fmt.Printf("\n%s %s/%s\n", icon, ns, name)
			if project != "" && project != "default" {
				fmt.Printf("  Project:    %s\n", project)
			}
			fmt.Printf("  Repo:       %s\n", repoURL)
			if path != "" {
				fmt.Printf("  Path:       %s\n", path)
			}
			if chart != "" {
				fmt.Printf("  Chart:      %s\n", chart)
			}
			fmt.Printf("  TargetRev:  %s\n", targetRev)
			fmt.Printf("  Dest:       %s → %s\n", destServer, destNs)

			syncPolicy := "Manual"
			if len(automated) > 0 {
				syncPolicy = fmt.Sprintf("Auto (prune: %v, selfHeal: %v)", prune, selfHeal)
			}
			fmt.Printf("  SyncPolicy: %s\n", syncPolicy)
			if len(syncOptions) > 0 {
				fmt.Printf("  SyncOpts:   %v\n", syncOptions)
			}

			fmt.Printf("  Sync:       %s @ %s\n", syncStatus, syncRevision)
			fmt.Printf("  Health:     %s\n", healthStatus)
			if healthMessage != "" {
				fmt.Printf("              %s\n", healthMessage)
			}
			if healthLastTransition != "" {
				fmt.Printf("  HealthAt:   %s\n", healthLastTransition)
			}
			if sourceType != "" {
				fmt.Printf("  SourceType: %s\n", sourceType)
			}
			if reconciledAt != "" {
				fmt.Printf("  Reconciled: %s\n", reconciledAt)
			}
			fmt.Printf("  Generation: %d\n", generation)

			if opPhase != "" {
				initiator := "manual"
				if opInitiatedBy {
					initiator = "automated"
				}
				fmt.Printf("  Operation:  %s (%s)\n", opPhase, initiator)
				fmt.Printf("              %s\n", opMessage)
				if opStartedAt != "" {
					fmt.Printf("              Started: %s\n", opStartedAt)
				}
				if opFinishedAt != "" {
					fmt.Printf("              Finished: %s\n", opFinishedAt)
				}
			}

			if len(summaryImages) > 0 {
				fmt.Printf("  Images:     %d\n", len(summaryImages))
				for _, img := range summaryImages {
					fmt.Printf("              - %s\n", img)
				}
			}

			fmt.Printf("  Resources:  %d managed\n", len(resources))
			// Show individual resources with status
			for i, r := range resources {
				if i >= 10 {
					fmt.Printf("              ... and %d more\n", len(resources)-10)
					break
				}
				if rMap, ok := r.(map[string]interface{}); ok {
					kind, _ := rMap["kind"].(string)
					rName, _ := rMap["name"].(string)
					rNs, _ := rMap["namespace"].(string)
					rStatus, _ := rMap["status"].(string)
					rHealth, _ := rMap["health"].(map[string]interface{})
					healthStr := ""
					if rHealth != nil {
						if h, ok := rHealth["status"].(string); ok {
							healthStr = h
						}
					}
					statusIcon := "✓"
					if rStatus != "Synced" || (healthStr != "" && healthStr != "Healthy") {
						statusIcon = "✗"
					}
					if healthStr != "" {
						fmt.Printf("              %s %s/%s (%s, %s)\n", statusIcon, rNs, rName, kind, healthStr)
					} else {
						fmt.Printf("              %s %s/%s (%s, %s)\n", statusIcon, rNs, rName, kind, rStatus)
					}
				}
			}

			if len(history) > 0 {
				fmt.Printf("  History:    %d deployments\n", len(history))
				// Show last 3 deployments
				for i := len(history) - 1; i >= 0 && i >= len(history)-3; i-- {
					if hMap, ok := history[i].(map[string]interface{}); ok {
						hID, _ := hMap["id"].(float64)
						hRev, _ := hMap["revision"].(string)
						hDeployedAt, _ := hMap["deployedAt"].(string)
						if len(hRev) > 12 {
							hRev = hRev[:12]
						}
						fmt.Printf("              #%.0f: %s @ %s\n", hID, hRev, hDeployedAt)
					}
				}
			}

			// Sync result resources (shows per-resource sync messages from last operation)
			syncResultResources, _, _ := unstructured.NestedSlice(app.Object, "status", "operationState", "syncResult", "resources")
			if len(syncResultResources) > 0 {
				fmt.Printf("  SyncResult: %d resources\n", len(syncResultResources))
				for i, sr := range syncResultResources {
					if i >= 5 {
						fmt.Printf("              ... and %d more\n", len(syncResultResources)-5)
						break
					}
					if srMap, ok := sr.(map[string]interface{}); ok {
						srKind, _ := srMap["kind"].(string)
						srName, _ := srMap["name"].(string)
						srStatus, _ := srMap["status"].(string)
						srMessage, _ := srMap["message"].(string)
						srHookPhase, _ := srMap["hookPhase"].(string)
						srSyncPhase, _ := srMap["syncPhase"].(string)

						srIcon := "✓"
						if srStatus != "Synced" {
							srIcon = "✗"
						}

						// Show sync phase and hook phase if present
						phases := ""
						if srSyncPhase != "" && srSyncPhase != "Sync" {
							phases = fmt.Sprintf(" [%s]", srSyncPhase)
						}
						if srHookPhase != "" && srHookPhase != "Running" && srHookPhase != "Succeeded" {
							phases += fmt.Sprintf(" [hook:%s]", srHookPhase)
						}

						fmt.Printf("              %s %s/%s%s\n", srIcon, srKind, srName, phases)
						if srMessage != "" {
							if len(srMessage) > 60 {
								srMessage = srMessage[:57] + "..."
							}
							fmt.Printf("                → %s\n", srMessage)
						}
					}
				}
			}

			// Resource Tree: Build Deployment → ReplicaSet → Pod tree from live cluster state
			// This shows the actual running state, not just what ArgoCD tracks
			for _, r := range resources {
				rMap, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				kind, _ := rMap["kind"].(string)
				rName, _ := rMap["name"].(string)
				rNs, _ := rMap["namespace"].(string)

				// Only build tree for Deployments
				if kind != "Deployment" {
					continue
				}

				// Get the deployment to find its ReplicaSets
				deploy, err := dynClient.Resource(schema.GroupVersionResource{
					Group: "apps", Version: "v1", Resource: "deployments",
				}).Namespace(rNs).Get(ctx, rName, v1.GetOptions{})
				if err != nil {
					continue
				}

				// Get deployment status for current ReplicaSet info
				deployConditions, _, _ := unstructured.NestedSlice(deploy.Object, "status", "conditions")
				var currentRS string
				for _, c := range deployConditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					if cond["type"] == "Progressing" {
						msg, _ := cond["message"].(string)
						// Extract ReplicaSet name from message like:
						// "ReplicaSet \"guestbook-ui-84774bdc6f\" has successfully progressed."
						if strings.Contains(msg, "ReplicaSet") {
							parts := strings.Split(msg, "\"")
							if len(parts) >= 2 {
								currentRS = parts[1]
							}
						}
					}
				}

				if currentRS == "" {
					continue
				}

				// Get the ReplicaSet
				rs, err := dynClient.Resource(schema.GroupVersionResource{
					Group: "apps", Version: "v1", Resource: "replicasets",
				}).Namespace(rNs).Get(ctx, currentRS, v1.GetOptions{})
				if err != nil {
					continue
				}

				rsReplicas, _, _ := unstructured.NestedInt64(rs.Object, "status", "replicas")
				rsReady, _, _ := unstructured.NestedInt64(rs.Object, "status", "readyReplicas")
				rsLabels := rs.GetLabels()
				podTemplateHash := rsLabels["pod-template-hash"]

				fmt.Printf("  LiveTree:   %s/%s\n", rNs, rName)
				fmt.Printf("              └─ ReplicaSet/%s (%d/%d ready)\n", currentRS, rsReady, rsReplicas)

				// Get Pods owned by this ReplicaSet
				podList, err := dynClient.Resource(schema.GroupVersionResource{
					Group: "", Version: "v1", Resource: "pods",
				}).Namespace(rNs).List(ctx, v1.ListOptions{
					LabelSelector: "pod-template-hash=" + podTemplateHash,
				})
				if err != nil {
					continue
				}

				for i, pod := range podList.Items {
					podName := pod.GetName()
					podPhase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
					podIP, _, _ := unstructured.NestedString(pod.Object, "status", "podIP")
					nodeName, _, _ := unstructured.NestedString(pod.Object, "spec", "nodeName")

					// Get container statuses for restart count
					containerStatuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
					restarts := int64(0)
					for _, cs := range containerStatuses {
						if csMap, ok := cs.(map[string]interface{}); ok {
							if r, ok := csMap["restartCount"].(int64); ok {
								restarts += r
							} else if r, ok := csMap["restartCount"].(float64); ok {
								restarts += int64(r)
							}
						}
					}

					// Tree connector
					connector := "├─"
					if i == len(podList.Items)-1 {
						connector = "└─"
					}

					podIcon := "✓"
					if podPhase != "Running" {
						podIcon = "✗"
					}

					restartStr := ""
					if restarts > 0 {
						restartStr = fmt.Sprintf(", %d restarts", restarts)
					}

					fmt.Printf("                 %s %s Pod/%s (%s, %s%s)\n",
						connector, podIcon, podName, podPhase, podIP, restartStr)
					if nodeName != "" {
						fmt.Printf("                    node: %s\n", nodeName)
					}
				}
			}
		}
	} else {
		fmt.Println("  (ArgoCD CRDs not installed)")
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// ARGOCD APPLICATIONSETS
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("ARGOCD APPLICATIONSETS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	if appSetList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applicationsets",
	}).List(ctx, v1.ListOptions{}); err == nil {
		argoInstalled = true
		if len(appSetList.Items) == 0 {
			fmt.Println("  (none)")
		}
		for _, appSet := range appSetList.Items {
			name := appSet.GetName()
			ns := appSet.GetNamespace()
			generation := appSet.GetGeneration()

			// Generators
			generators, _, _ := unstructured.NestedSlice(appSet.Object, "spec", "generators")

			// Template
			templateName, _, _ := unstructured.NestedString(appSet.Object, "spec", "template", "metadata", "name")
			templateRepoURL, _, _ := unstructured.NestedString(appSet.Object, "spec", "template", "spec", "source", "repoURL")
			templatePath, _, _ := unstructured.NestedString(appSet.Object, "spec", "template", "spec", "source", "path")
			templateDestServer, _, _ := unstructured.NestedString(appSet.Object, "spec", "template", "spec", "destination", "server")
			templateDestNs, _, _ := unstructured.NestedString(appSet.Object, "spec", "template", "spec", "destination", "namespace")

			// Sync policy
			preserveOnDelete, _, _ := unstructured.NestedBool(appSet.Object, "spec", "syncPolicy", "preserveResourcesOnDeletion")

			// Status
			conditions, _, _ := unstructured.NestedSlice(appSet.Object, "status", "conditions")

			icon := "✓"
			for _, c := range conditions {
				if cMap, ok := c.(map[string]interface{}); ok {
					if cMap["type"] == "ErrorOccurred" && cMap["status"] == "True" {
						icon = "✗"
						break
					}
				}
			}

			fmt.Printf("\n%s %s/%s\n", icon, ns, name)
			fmt.Printf("  Template:   %s\n", templateName)
			fmt.Printf("  Repo:       %s\n", templateRepoURL)
			if templatePath != "" {
				fmt.Printf("  Path:       %s\n", templatePath)
			}
			fmt.Printf("  Dest:       %s → %s\n", templateDestServer, templateDestNs)
			fmt.Printf("  Generators: %d defined\n", len(generators))
			for _, gen := range generators {
				if genMap, ok := gen.(map[string]interface{}); ok {
					// Detect generator type
					for genType := range genMap {
						fmt.Printf("              - %s\n", genType)
					}
				}
			}
			if preserveOnDelete {
				fmt.Printf("  Preserve:   true (on delete)\n")
			}
			fmt.Printf("  Generation: %d\n", generation)
			if len(conditions) > 0 {
				fmt.Printf("  Conditions:\n")
				for _, c := range conditions {
					cond, ok := c.(map[string]interface{})
					if !ok {
						continue
					}
					condType, _ := cond["type"].(string)
					condStatus, _ := cond["status"].(string)
					condReason, _ := cond["reason"].(string)
					condTime, _ := cond["lastTransitionTime"].(string)
					fmt.Printf("    %s: %s (%s) @ %s\n", condType, condStatus, condReason, condTime)
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// WORKLOADS BY OWNER
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("WORKLOADS (Deployments/StatefulSets/DaemonSets)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	type workloadInfo struct {
		namespace      string
		name           string
		kind           string
		owner          string
		managedBy      string
		replicas       string
		images         []string
		age            string
		labels         map[string]string
		annotations    map[string]string
		podLabels      map[string]string
		podAnnotations map[string]string
	}
	var workloads []workloadInfo

	// Deployments
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}
			name := dep.GetName()
			owner, managedBy := detectOwnership(&dep)

			desired, _, _ := unstructured.NestedInt64(dep.Object, "spec", "replicas")
			ready, _, _ := unstructured.NestedInt64(dep.Object, "status", "readyReplicas")
			available, _, _ := unstructured.NestedInt64(dep.Object, "status", "availableReplicas")

			// Get images
			containers, _, _ := unstructured.NestedSlice(dep.Object, "spec", "template", "spec", "containers")
			var images []string
			for _, c := range containers {
				if cMap, ok := c.(map[string]interface{}); ok {
					if img, ok := cMap["image"].(string); ok {
						images = append(images, img)
					}
				}
			}

			// Get pod template labels and annotations
			podLabelsRaw, _, _ := unstructured.NestedStringMap(dep.Object, "spec", "template", "metadata", "labels")
			podAnnotationsRaw, _, _ := unstructured.NestedStringMap(dep.Object, "spec", "template", "metadata", "annotations")

			age := time.Since(dep.GetCreationTimestamp().Time).Round(time.Hour).String()

			workloads = append(workloads, workloadInfo{
				namespace:      ns,
				name:           name,
				kind:           "Deployment",
				owner:          owner,
				managedBy:      managedBy,
				replicas:       fmt.Sprintf("%d/%d ready, %d available", ready, desired, available),
				images:         images,
				age:            age,
				labels:         dep.GetLabels(),
				annotations:    dep.GetAnnotations(),
				podLabels:      podLabelsRaw,
				podAnnotations: podAnnotationsRaw,
			})
		}
	}

	// StatefulSets
	if stsList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "statefulsets",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, sts := range stsList.Items {
			ns := sts.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}
			name := sts.GetName()
			owner, managedBy := detectOwnership(&sts)

			desired, _, _ := unstructured.NestedInt64(sts.Object, "spec", "replicas")
			ready, _, _ := unstructured.NestedInt64(sts.Object, "status", "readyReplicas")

			containers, _, _ := unstructured.NestedSlice(sts.Object, "spec", "template", "spec", "containers")
			var images []string
			for _, c := range containers {
				if cMap, ok := c.(map[string]interface{}); ok {
					if img, ok := cMap["image"].(string); ok {
						images = append(images, img)
					}
				}
			}

			// Get pod template labels and annotations
			podLabelsRaw, _, _ := unstructured.NestedStringMap(sts.Object, "spec", "template", "metadata", "labels")
			podAnnotationsRaw, _, _ := unstructured.NestedStringMap(sts.Object, "spec", "template", "metadata", "annotations")

			age := time.Since(sts.GetCreationTimestamp().Time).Round(time.Hour).String()

			workloads = append(workloads, workloadInfo{
				namespace:      ns,
				name:           name,
				kind:           "StatefulSet",
				owner:          owner,
				managedBy:      managedBy,
				replicas:       fmt.Sprintf("%d/%d ready", ready, desired),
				images:         images,
				age:            age,
				labels:         sts.GetLabels(),
				annotations:    sts.GetAnnotations(),
				podLabels:      podLabelsRaw,
				podAnnotations: podAnnotationsRaw,
			})
		}
	}

	// Group by owner
	byOwner := map[string][]workloadInfo{}
	for _, w := range workloads {
		byOwner[w.owner] = append(byOwner[w.owner], w)
	}

	for _, owner := range []string{"Flux", "ArgoCD", "Helm", "ConfigHub", "Native"} {
		wls := byOwner[owner]
		if len(wls) == 0 {
			continue
		}
		fmt.Printf("\n[%s] %d workloads\n", owner, len(wls))
		for _, w := range wls {
			fmt.Printf("\n  %s/%s (%s)\n", w.namespace, w.name, w.kind)
			if w.managedBy != "" && w.managedBy != "-" {
				fmt.Printf("    ManagedBy:  %s\n", w.managedBy)
			}
			fmt.Printf("    Replicas:   %s\n", w.replicas)
			fmt.Printf("    Age:        %s\n", w.age)
			for _, img := range w.images {
				fmt.Printf("    Image:      %s\n", img)
			}
			// Show interesting labels (standard kubernetes labels) - check both deployment and pod template
			if app, ok := w.labels["app.kubernetes.io/name"]; ok {
				fmt.Printf("    App:        %s\n", app)
			} else if app, ok := w.labels["app"]; ok {
				fmt.Printf("    App:        %s\n", app)
			} else if app, ok := w.podLabels["app.kubernetes.io/name"]; ok {
				fmt.Printf("    App:        %s (pod)\n", app)
			} else if app, ok := w.podLabels["app"]; ok {
				fmt.Printf("    App:        %s (pod)\n", app)
			}
			if version, ok := w.labels["app.kubernetes.io/version"]; ok {
				fmt.Printf("    Version:    %s\n", version)
			} else if version, ok := w.podLabels["app.kubernetes.io/version"]; ok {
				fmt.Printf("    Version:    %s (pod)\n", version)
			}
			if component, ok := w.labels["app.kubernetes.io/component"]; ok {
				fmt.Printf("    Component:  %s\n", component)
			}
			if partOf, ok := w.labels["app.kubernetes.io/part-of"]; ok {
				fmt.Printf("    PartOf:     %s\n", partOf)
			}
			if instance, ok := w.labels["app.kubernetes.io/instance"]; ok {
				fmt.Printf("    Instance:   %s\n", instance)
			}
			// Show Flux labels
			if ksName, ok := w.labels["kustomize.toolkit.fluxcd.io/name"]; ok {
				ksNs := w.labels["kustomize.toolkit.fluxcd.io/namespace"]
				fmt.Printf("    FluxKS:     %s/%s\n", ksNs, ksName)
			}
			if hrName, ok := w.labels["helm.toolkit.fluxcd.io/name"]; ok {
				hrNs := w.labels["helm.toolkit.fluxcd.io/namespace"]
				fmt.Printf("    FluxHR:     %s/%s\n", hrNs, hrName)
			}
			// Show ArgoCD labels
			if argoInstance, ok := w.labels["argocd.argoproj.io/instance"]; ok {
				fmt.Printf("    ArgoApp:    %s\n", argoInstance)
			}
			// Show Helm labels
			if helmChart, ok := w.labels["helm.sh/chart"]; ok {
				fmt.Printf("    HelmChart:  %s\n", helmChart)
			}
			if helmRelease, ok := w.annotations["meta.helm.sh/release-name"]; ok {
				helmNs := w.annotations["meta.helm.sh/release-namespace"]
				fmt.Printf("    HelmRel:    %s/%s\n", helmNs, helmRelease)
			}
			// Show Prometheus annotations (check both deployment and pod template)
			promScrape, hasProm := w.annotations["prometheus.io/scrape"]
			promPort := w.annotations["prometheus.io/port"]
			promPath := w.annotations["prometheus.io/path"]
			if !hasProm || promScrape != "true" {
				promScrape, hasProm = w.podAnnotations["prometheus.io/scrape"]
				promPort = w.podAnnotations["prometheus.io/port"]
				promPath = w.podAnnotations["prometheus.io/path"]
			}
			if hasProm && promScrape == "true" {
				if promPath == "" {
					promPath = "/metrics"
				}
				fmt.Printf("    Prometheus: :%s%s\n", promPort, promPath)
			}
			// Show ConfigHub context (enhanced in connected mode)
			if unitSlug, ok := w.labels["confighub.com/UnitSlug"]; ok {
				if deepDiveConnected && unitCache != nil {
					if unit := unitCache.getUnitBySlug(unitSlug); unit != nil {
						printConfigHubContext(unit)
					} else {
						fmt.Printf("    CHUnit:     %s (not found in space '%s')\n", unitSlug, unitCache.space)
					}
				} else {
					fmt.Printf("    CHUnit:     %s\n", unitSlug)
					if spaceSlug, ok := w.annotations["confighub.com/SpaceSlug"]; ok {
						fmt.Printf("    CHSpace:    %s\n", spaceSlug)
					}
					fmt.Println("                (use --connected for full Unit context)")
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// HELM RELEASES (from secrets)
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("HELM RELEASES (Standalone)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	// Track releases and all their versions for history
	type helmRevision struct {
		version int
		status  string
		secret  *unstructured.Unstructured
	}
	type helmReleaseInfo struct {
		namespace string
		name      string
		revisions []helmRevision // all revisions, sorted by version descending
	}
	helmReleases := make(map[string]*helmReleaseInfo) // key: namespace/name

	if secretList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "secrets",
	}).List(ctx, v1.ListOptions{
		LabelSelector: "owner=helm",
	}); err == nil {
		for _, secret := range secretList.Items {
			secretName := secret.GetName()
			namespace := secret.GetNamespace()

			// Helm secrets are named: sh.helm.release.v1.<release>.v<version>
			if !strings.HasPrefix(secretName, "sh.helm.release.v1.") {
				continue
			}

			// Parse release name and version from secret name
			parts := strings.Split(secretName, ".")
			if len(parts) < 6 {
				continue
			}
			releaseName := parts[4]
			versionStr := strings.TrimPrefix(parts[5], "v")
			version, _ := strconv.Atoi(versionStr)

			key := namespace + "/" + releaseName
			labels := secret.GetLabels()
			status := labels["status"]

			existing := helmReleases[key]
			if existing == nil {
				existing = &helmReleaseInfo{
					namespace: namespace,
					name:      releaseName,
				}
				helmReleases[key] = existing
			}

			secretCopy := secret.DeepCopy()
			existing.revisions = append(existing.revisions, helmRevision{
				version: version,
				status:  status,
				secret:  secretCopy,
			})
		}

		// Sort revisions by version descending (newest first)
		for _, release := range helmReleases {
			sort.Slice(release.revisions, func(i, j int) bool {
				return release.revisions[i].version > release.revisions[j].version
			})
		}
	}

	if len(helmReleases) == 0 {
		fmt.Println("  (none)")
	} else {
		// Sort releases by namespace/name
		keys := make([]string, 0, len(helmReleases))
		for k := range helmReleases {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			release := helmReleases[key]
			if len(release.revisions) == 0 {
				continue
			}

			// Latest revision is first (sorted descending)
			latest := release.revisions[0]

			// Determine status icon
			statusIcon := "?"
			switch latest.status {
			case "deployed":
				statusIcon = "\u2713" // check mark
			case "failed":
				statusIcon = "\u2717" // x mark
			case "pending-install", "pending-upgrade", "pending-rollback":
				statusIcon = "\u25cf" // circle
			case "superseded":
				statusIcon = "\u2191" // up arrow
			case "uninstalling":
				statusIcon = "\u21bb" // rotation
			}

			fmt.Printf("\n%s %s/%s\n", statusIcon, release.namespace, release.name)
			fmt.Printf("  Status:     %s\n", latest.status)
			fmt.Printf("  Revision:   %d (of %d total)\n", latest.version, len(release.revisions))

			// Try to decode the release data for more details
			if latest.secret != nil {
				if releaseData, found, _ := unstructured.NestedString(latest.secret.Object, "data", "release"); found {
					if decoded := decodeHelmRelease(releaseData); decoded != nil {
						// Chart info
						if chartMeta, ok := decoded["chart"].(map[string]interface{}); ok {
							if metadata, ok := chartMeta["metadata"].(map[string]interface{}); ok {
								chartName, _ := metadata["name"].(string)
								chartVersion, _ := metadata["version"].(string)
								appVersion, _ := metadata["appVersion"].(string)
								description, _ := metadata["description"].(string)
								home, _ := metadata["home"].(string)
								icon, _ := metadata["icon"].(string)
								if chartName != "" {
									fmt.Printf("  Chart:      %s-%s\n", chartName, chartVersion)
								}
								if appVersion != "" {
									fmt.Printf("  AppVersion: %s\n", appVersion)
								}
								if description != "" {
									if len(description) > 70 {
										description = description[:67] + "..."
									}
									fmt.Printf("  Desc:       %s\n", description)
								}
								if home != "" {
									fmt.Printf("  Home:       %s\n", home)
								}
								if icon != "" {
									fmt.Printf("  Icon:       %s\n", icon)
								}

								// Dependencies
								if deps, ok := metadata["dependencies"].([]interface{}); ok && len(deps) > 0 {
									fmt.Printf("  Dependencies: %d\n", len(deps))
									for i, dep := range deps {
										if i >= 3 { // Show max 3
											fmt.Printf("              ... and %d more\n", len(deps)-3)
											break
										}
										if d, ok := dep.(map[string]interface{}); ok {
											depName, _ := d["name"].(string)
											depVersion, _ := d["version"].(string)
											depRepo, _ := d["repository"].(string)
											fmt.Printf("              - %s@%s", depName, depVersion)
											if depRepo != "" {
												// Shorten long repo URLs
												if len(depRepo) > 40 {
													depRepo = depRepo[:37] + "..."
												}
												fmt.Printf(" (%s)", depRepo)
											}
											fmt.Println()
										}
									}
								}

								// Maintainers
								if maintainers, ok := metadata["maintainers"].([]interface{}); ok && len(maintainers) > 0 {
									fmt.Printf("  Maintainers: %d\n", len(maintainers))
									for i, m := range maintainers {
										if i >= 2 { // Show max 2
											fmt.Printf("              ... and %d more\n", len(maintainers)-2)
											break
										}
										if maint, ok := m.(map[string]interface{}); ok {
											name, _ := maint["name"].(string)
											email, _ := maint["email"].(string)
											if email != "" {
												fmt.Printf("              - %s <%s>\n", name, email)
											} else {
												fmt.Printf("              - %s\n", name)
											}
										}
									}
								}

								// Sources
								if sources, ok := metadata["sources"].([]interface{}); ok && len(sources) > 0 {
									fmt.Printf("  Sources:    %d\n", len(sources))
									for i, s := range sources {
										if i >= 2 {
											fmt.Printf("              ... and %d more\n", len(sources)-2)
											break
										}
										if src, ok := s.(string); ok {
											fmt.Printf("              - %s\n", src)
										}
									}
								}

								// Keywords
								if keywords, ok := metadata["keywords"].([]interface{}); ok && len(keywords) > 0 {
									kwStrings := make([]string, 0, len(keywords))
									for _, kw := range keywords {
										if k, ok := kw.(string); ok {
											kwStrings = append(kwStrings, k)
										}
									}
									kwStr := strings.Join(kwStrings, ", ")
									if len(kwStr) > 60 {
										kwStr = kwStr[:57] + "..."
									}
									fmt.Printf("  Keywords:   %s\n", kwStr)
								}
							}

							// Templates (count only)
							if templates, ok := chartMeta["templates"].([]interface{}); ok {
								fmt.Printf("  Templates:  %d files\n", len(templates))
							}
						}

						// Release info (timestamps, etc)
						if info, ok := decoded["info"].(map[string]interface{}); ok {
							if firstDeployed, ok := info["first_deployed"].(string); ok && firstDeployed != "" {
								if t, err := time.Parse(time.RFC3339, firstDeployed); err == nil {
									fmt.Printf("  FirstDeploy: %s\n", t.Format("2006-01-02 15:04:05"))
								}
							}
							if lastDeployed, ok := info["last_deployed"].(string); ok && lastDeployed != "" {
								if t, err := time.Parse(time.RFC3339, lastDeployed); err == nil {
									fmt.Printf("  LastDeploy: %s\n", t.Format("2006-01-02 15:04:05"))
								}
							}
							if deleted, ok := info["deleted"].(string); ok && deleted != "" && deleted != "0001-01-01T00:00:00Z" {
								if t, err := time.Parse(time.RFC3339, deleted); err == nil {
									fmt.Printf("  Deleted:    %s\n", t.Format("2006-01-02 15:04:05"))
								}
							}
							// Status description (error messages, etc)
							if desc, ok := info["description"].(string); ok && desc != "" {
								if len(desc) > 60 {
									desc = desc[:57] + "..."
								}
								fmt.Printf("  Result:     %s\n", desc)
							}
							// Full NOTES.txt content from chart (shows post-install instructions)
							if notes, ok := info["notes"].(string); ok && notes != "" {
								// Count lines and show summary
								lines := strings.Split(notes, "\n")
								nonEmptyLines := 0
								for _, line := range lines {
									if strings.TrimSpace(line) != "" {
										nonEmptyLines++
									}
								}
								fmt.Printf("  PostNotes:  %d lines (NOTES.txt)\n", nonEmptyLines)
								// Show first 5 non-empty lines as preview
								shown := 0
								for _, line := range lines {
									if strings.TrimSpace(line) == "" {
										continue
									}
									if shown >= 5 {
										fmt.Printf("              ... (%d more lines)\n", nonEmptyLines-5)
										break
									}
									if len(line) > 60 {
										line = line[:57] + "..."
									}
									fmt.Printf("              %s\n", line)
									shown++
								}
							}
						}

						// Hooks (pre/post install/upgrade/rollback/delete)
						if hooks, ok := decoded["hooks"].([]interface{}); ok && len(hooks) > 0 {
							fmt.Printf("  Hooks:      %d\n", len(hooks))
							for i, h := range hooks {
								if i >= 3 {
									fmt.Printf("              ... and %d more\n", len(hooks)-3)
									break
								}
								if hook, ok := h.(map[string]interface{}); ok {
									hookName, _ := hook["name"].(string)
									hookKind, _ := hook["kind"].(string)
									hookEvents, _ := hook["events"].([]interface{})
									hookPhase, _ := hook["last_run"].(map[string]interface{})
									var phase string
									if hookPhase != nil {
										phase, _ = hookPhase["phase"].(string)
									}
									eventStrs := make([]string, 0, len(hookEvents))
									for _, e := range hookEvents {
										if es, ok := e.(string); ok {
											eventStrs = append(eventStrs, es)
										}
									}
									fmt.Printf("              - %s (%s) [%s]", hookName, hookKind, strings.Join(eventStrs, ", "))
									if phase != "" {
										fmt.Printf(" → %s", phase)
									}
									fmt.Println()
								}
							}
						}

						// Config (custom values)
						if config, ok := decoded["config"].(map[string]interface{}); ok && len(config) > 0 {
							fmt.Printf("  Values:     %d custom value(s)\n", len(config))
							configKeys := make([]string, 0, len(config))
							for k := range config {
								configKeys = append(configKeys, k)
							}
							sort.Strings(configKeys)
							for i, k := range configKeys {
								if i >= 5 { // Show max 5
									fmt.Printf("              ... and %d more\n", len(configKeys)-5)
									break
								}
								v := config[k]
								vStr := fmt.Sprintf("%v", v)
								if len(vStr) > 40 {
									vStr = vStr[:37] + "..."
								}
								fmt.Printf("              %s: %s\n", k, vStr)
							}
						}

						// Namespace
						if ns, ok := decoded["namespace"].(string); ok && ns != "" && ns != release.namespace {
							fmt.Printf("  TargetNS:   %s\n", ns)
						}

						// Manifest count
						if manifest, ok := decoded["manifest"].(string); ok && manifest != "" {
							// Count YAML documents in manifest
							docCount := strings.Count(manifest, "\n---") + 1
							fmt.Printf("  Manifests:  %d resources\n", docCount)
						}
					}
				}
			}

			// Show release history (all revisions)
			if len(release.revisions) > 1 {
				fmt.Printf("  History:    %d revisions\n", len(release.revisions))
				for i, rev := range release.revisions {
					if i >= 5 {
						fmt.Printf("              ... and %d older\n", len(release.revisions)-5)
						break
					}
					revIcon := "·"
					switch rev.status {
					case "deployed":
						revIcon = "✓"
					case "failed":
						revIcon = "✗"
					case "superseded":
						revIcon = "↑"
					}

					// Try to get timestamp from this revision
					timestamp := ""
					if rev.secret != nil {
						if revData, found, _ := unstructured.NestedString(rev.secret.Object, "data", "release"); found {
							if revDecoded := decodeHelmRelease(revData); revDecoded != nil {
								if revInfo, ok := revDecoded["info"].(map[string]interface{}); ok {
									if ts, ok := revInfo["last_deployed"].(string); ok {
										if t, err := time.Parse(time.RFC3339, ts); err == nil {
											timestamp = t.Format("2006-01-02 15:04")
										}
									}
								}
							}
						}
					}

					if timestamp != "" {
						fmt.Printf("              %s v%d: %s @ %s\n", revIcon, rev.version, rev.status, timestamp)
					} else {
						fmt.Printf("              %s v%d: %s\n", revIcon, rev.version, rev.status)
					}
				}
			}

			// LiveTree: Find Helm-managed Deployments and show their live state
			// Helm labels vary by chart - try multiple selectors
			helmDeployList, err := dynClient.Resource(schema.GroupVersionResource{
				Group: "apps", Version: "v1", Resource: "deployments",
			}).Namespace(release.namespace).List(ctx, v1.ListOptions{
				LabelSelector: "app.kubernetes.io/managed-by=Helm",
			})
			if err == nil && len(helmDeployList.Items) > 0 {
				// Filter to deployments matching this release by checking labels
				var matchingDeploys []unstructured.Unstructured
				for _, d := range helmDeployList.Items {
					labels := d.GetLabels()
					// Check for standard Helm labels that might identify this release
					instance := labels["app.kubernetes.io/instance"]
					name := labels["app.kubernetes.io/name"]
					chartLabel := labels["helm.sh/chart"]
					// Match by instance=release.name or name=release.name or chart starts with release.name
					if instance == release.name || name == release.name ||
						strings.HasPrefix(chartLabel, release.name+"-") {
						matchingDeploys = append(matchingDeploys, d)
					}
				}
				for _, deploy := range matchingDeploys {
					deployName := deploy.GetName()
					deployNs := deploy.GetNamespace()

					// Get deployment status for current ReplicaSet info
					deployConditions, _, _ := unstructured.NestedSlice(deploy.Object, "status", "conditions")
					var currentRS string
					for _, c := range deployConditions {
						cond, ok := c.(map[string]interface{})
						if !ok {
							continue
						}
						if cond["type"] == "Progressing" {
							msg, _ := cond["message"].(string)
							if strings.Contains(msg, "ReplicaSet") {
								msgParts := strings.Split(msg, "\"")
								if len(msgParts) >= 2 {
									currentRS = msgParts[1]
								}
							}
						}
					}

					if currentRS == "" {
						continue
					}

					// Get the ReplicaSet
					rs, err := dynClient.Resource(schema.GroupVersionResource{
						Group: "apps", Version: "v1", Resource: "replicasets",
					}).Namespace(deployNs).Get(ctx, currentRS, v1.GetOptions{})
					if err != nil {
						continue
					}

					rsReplicas, _, _ := unstructured.NestedInt64(rs.Object, "status", "replicas")
					rsReady, _, _ := unstructured.NestedInt64(rs.Object, "status", "readyReplicas")

					fmt.Printf("  LiveTree:   %s/%s\n", deployNs, deployName)
					fmt.Printf("              └─ ReplicaSet/%s (%d/%d ready)\n", currentRS, rsReady, rsReplicas)

					// Get Pods owned by this ReplicaSet
					rsUID := rs.GetUID()
					podList, err := dynClient.Resource(schema.GroupVersionResource{
						Group: "", Version: "v1", Resource: "pods",
					}).Namespace(deployNs).List(ctx, v1.ListOptions{})
					if err != nil {
						continue
					}

					var matchingPods []unstructured.Unstructured
					for _, pod := range podList.Items {
						owners, _, _ := unstructured.NestedSlice(pod.Object, "metadata", "ownerReferences")
						for _, owner := range owners {
							if ownerMap, ok := owner.(map[string]interface{}); ok {
								if uid, _ := ownerMap["uid"].(string); uid == string(rsUID) {
									matchingPods = append(matchingPods, pod)
									break
								}
							}
						}
					}

					for i, pod := range matchingPods {
						podName := pod.GetName()
						podPhase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
						podIP, _, _ := unstructured.NestedString(pod.Object, "status", "podIP")
						nodeName, _, _ := unstructured.NestedString(pod.Object, "spec", "nodeName")

						containerStatuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
						restarts := int64(0)
						for _, cs := range containerStatuses {
							if csMap, ok := cs.(map[string]interface{}); ok {
								if r, ok := csMap["restartCount"].(int64); ok {
									restarts += r
								} else if r, ok := csMap["restartCount"].(float64); ok {
									restarts += int64(r)
								}
							}
						}

						connector := "├─"
						if i == len(matchingPods)-1 {
							connector = "└─"
						}

						podIcon := "✓"
						if podPhase != "Running" {
							podIcon = "✗"
						}

						restartStr := ""
						if restarts > 0 {
							restartStr = fmt.Sprintf(", %d restarts", restarts)
						}

						fmt.Printf("                 %s %s Pod/%s (%s, %s%s)\n",
							connector, podIcon, podName, podPhase, podIP, restartStr)
						if nodeName != "" {
							fmt.Printf("                    node: %s\n", nodeName)
						}
					}
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// SUMMARY
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("SUMMARY")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	fmt.Printf("Flux CRDs:    %v\n", fluxInstalled)
	fmt.Printf("Argo CRDs:    %v\n", argoInstalled)
	fmt.Printf("Workloads:    %d total\n", len(workloads))
	for owner, wls := range byOwner {
		pct := 0
		if len(workloads) > 0 {
			pct = (len(wls) * 100) / len(workloads)
		}
		fmt.Printf("  %s: %d (%d%%)\n", owner, len(wls), pct)
	}
	fmt.Println()

	if deepDiveConnected && unitCache != nil {
		fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
		fmt.Println("CONFIGHUB CONNECTED MODE STATUS")
		fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
		fmt.Printf("  Space:       %s\n", unitCache.space)
		fmt.Printf("  Units:       %d in space\n", len(unitCache.units))

		// Count workloads with ConfigHub labels
		managedCount := 0
		for _, wls := range byOwner {
			for _, w := range wls {
				if _, ok := w.labels["confighub.com/UnitSlug"]; ok {
					managedCount++
				}
			}
		}
		fmt.Printf("  Managed:     %d/%d workloads have confighub.com/UnitSlug label\n", managedCount, len(workloads))

		fmt.Println()
		fmt.Println("  For more ConfigHub features:")
		fmt.Println("    • cub-scout map --hub     - Full ConfigHub hierarchy TUI")
		fmt.Println("    • cub-scout map fleet     - Fleet view across spaces")
		fmt.Println("    • cub unit list           - List all units in space")
		fmt.Println("    • cub link list           - Show all dependencies")
		fmt.Println()
	} else {
		fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
		fmt.Println("WHAT CONFIGHUB ADDS (requires 'cub auth login')")
		fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
		fmt.Println("  • Fleet-wide visibility across ALL clusters")
		fmt.Println("  • Unit/Space/Target hierarchy (who owns what)")
		fmt.Println("  • Revision history and change tracking")
		fmt.Println("  • Cross-cluster dependencies and links")
		fmt.Println("  • Drift detection against desired state")
		fmt.Println("  • CCVE scanning results and remediation")
		fmt.Println("  • Team ownership and RBAC")
		fmt.Println()
		fmt.Println("  Try: cub-scout map deep-dive --connected")
		fmt.Println()
	}

	return nil
}

// getConditionMessage returns the full message from the Ready condition
func getConditionMessage(obj *unstructured.Unstructured) string {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return "Unknown"
	}
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Ready" {
			if msg, ok := cond["message"].(string); ok && msg != "" {
				// Truncate long messages
				if len(msg) > 80 {
					return msg[:77] + "..."
				}
				return msg
			}
			if reason, ok := cond["reason"].(string); ok {
				return reason
			}
		}
	}
	return "Unknown"
}

// decodeHelmRelease decodes a Helm release from secrets
// The data is: base64(k8s secret) -> base64(helm) -> gzip -> json
// When using dynamic client, k8s base64 is already decoded, so we need:
// base64(helm) -> gzip -> json
func decodeHelmRelease(data string) map[string]interface{} {
	// First base64 decode (Helm's encoding)
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil
	}

	// Second base64 decode (Helm double-encodes)
	decoded2, err := base64.StdEncoding.DecodeString(string(decoded))
	if err != nil {
		// Try without second decode in case format varies
		decoded2 = decoded
	}

	// Gzip decompress
	reader, err := gzip.NewReader(bytes.NewReader(decoded2))
	if err != nil {
		return nil
	}
	defer reader.Close()

	uncompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil
	}

	var release map[string]interface{}
	if err := json.Unmarshal(uncompressed, &release); err != nil {
		return nil
	}

	return release
}

// runMapAppHierarchy shows inferred ConfigHub app hierarchy with MAXIMUM detail
// CLI equivalent of TUI's '5' or 'A' key (App Hierarchy view)
func runMapAppHierarchy(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("                    RICH APPLICATION HIERARCHY (STANDALONE)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Full tree view of cluster resources mapped to ConfigHub model.")
	fmt.Println("Legend: ✓ Ready  ✗ Not Ready  ⚡ Flux  🅰 Argo  ⎈ Helm  📦 ConfigHub  ☸ Native")
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// Collect all workloads with full details for dependency inference
	// ═══════════════════════════════════════════════════════════════════════════════
	type podInfo struct {
		name     string
		phase    string
		ip       string
		node     string
		restarts int64
	}
	type workloadDetail struct {
		namespace   string
		name        string
		kind        string
		owner       string
		managedBy   string
		ready       bool
		replicas    string
		images      []string
		envVars     map[string]string // env var name -> value (for dependency inference)
		serviceRefs []string          // services this workload might call
		configMaps  []string          // configmaps used
		secrets     []string          // secrets used
		labels      map[string]string
		replicaSets []string  // RS names
		pods        []podInfo // pod details
	}

	allWorkloads := map[string]*workloadDetail{} // namespace/name -> detail
	allServices := map[string][]string{}         // namespace -> service names

	// Collect services first (for dependency inference)
	if svcList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "services",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, svc := range svcList.Items {
			ns := svc.GetNamespace()
			if !isSystemNamespace(ns) {
				allServices[ns] = append(allServices[ns], svc.GetName())
			}
		}
	}

	// Collect deployments with full details
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}
			name := dep.GetName()
			owner, managedBy := detectOwnership(&dep)

			desired, _, _ := unstructured.NestedInt64(dep.Object, "spec", "replicas")
			ready, _, _ := unstructured.NestedInt64(dep.Object, "status", "readyReplicas")

			// Get containers, images, env vars
			containers, _, _ := unstructured.NestedSlice(dep.Object, "spec", "template", "spec", "containers")
			var images []string
			envVars := map[string]string{}
			var configMaps, secrets []string

			for _, c := range containers {
				cMap, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				if img, ok := cMap["image"].(string); ok {
					images = append(images, img)
				}
				// Extract env vars for dependency inference
				if envList, ok := cMap["env"].([]interface{}); ok {
					for _, e := range envList {
						if eMap, ok := e.(map[string]interface{}); ok {
							eName, _ := eMap["name"].(string)
							eValue, _ := eMap["value"].(string)
							if eName != "" {
								envVars[eName] = eValue
							}
							// Check for configmap/secret refs
							if valueFrom, ok := eMap["valueFrom"].(map[string]interface{}); ok {
								if cmRef, ok := valueFrom["configMapKeyRef"].(map[string]interface{}); ok {
									if cmName, ok := cmRef["name"].(string); ok {
										configMaps = append(configMaps, cmName)
									}
								}
								if secRef, ok := valueFrom["secretKeyRef"].(map[string]interface{}); ok {
									if secName, ok := secRef["name"].(string); ok {
										secrets = append(secrets, secName)
									}
								}
							}
						}
					}
				}
				// Check envFrom
				if envFromList, ok := cMap["envFrom"].([]interface{}); ok {
					for _, ef := range envFromList {
						if efMap, ok := ef.(map[string]interface{}); ok {
							if cmRef, ok := efMap["configMapRef"].(map[string]interface{}); ok {
								if cmName, ok := cmRef["name"].(string); ok {
									configMaps = append(configMaps, cmName)
								}
							}
							if secRef, ok := efMap["secretRef"].(map[string]interface{}); ok {
								if secName, ok := secRef["name"].(string); ok {
									secrets = append(secrets, secName)
								}
							}
						}
					}
				}
			}

			// Infer service references from env vars
			var serviceRefs []string
			for _, svc := range allServices[ns] {
				svcUpper := strings.ToUpper(strings.ReplaceAll(svc, "-", "_"))
				for envName, envVal := range envVars {
					if strings.Contains(envName, svcUpper) || strings.Contains(envVal, svc) {
						serviceRefs = append(serviceRefs, svc)
						break
					}
				}
			}

			key := ns + "/" + name
			allWorkloads[key] = &workloadDetail{
				namespace:   ns,
				name:        name,
				kind:        "Deployment",
				owner:       owner,
				managedBy:   managedBy,
				ready:       ready >= desired && desired > 0,
				replicas:    fmt.Sprintf("%d/%d", ready, desired),
				images:      images,
				envVars:     envVars,
				serviceRefs: serviceRefs,
				configMaps:  configMaps,
				secrets:     secrets,
				labels:      dep.GetLabels(),
			}
		}
	}

	// Collect ReplicaSets and link to parent Deployments
	rsToDeployment := map[string]string{} // rs namespace/name -> deployment namespace/name
	rsReady := map[string]string{}        // rs namespace/name -> "ready/desired"
	if rsList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "replicasets",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, rs := range rsList.Items {
			ns := rs.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}
			rsName := rs.GetName()
			rsKey := ns + "/" + rsName

			// Find owning Deployment
			ownerRefs, _, _ := unstructured.NestedSlice(rs.Object, "metadata", "ownerReferences")
			for _, ref := range ownerRefs {
				if refMap, ok := ref.(map[string]interface{}); ok {
					if refMap["kind"] == "Deployment" {
						if ownerName, ok := refMap["name"].(string); ok {
							depKey := ns + "/" + ownerName
							if wl, exists := allWorkloads[depKey]; exists {
								wl.replicaSets = append(wl.replicaSets, rsName)
								rsToDeployment[rsKey] = depKey
							}
						}
					}
				}
			}

			// Track RS ready count
			replicas, _, _ := unstructured.NestedInt64(rs.Object, "status", "replicas")
			readyReplicas, _, _ := unstructured.NestedInt64(rs.Object, "status", "readyReplicas")
			rsReady[rsKey] = fmt.Sprintf("%d/%d", readyReplicas, replicas)
		}
	}

	// Collect Pods and link to ReplicaSets
	if podList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "pods",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, pod := range podList.Items {
			ns := pod.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}
			podName := pod.GetName()

			// Find owning ReplicaSet
			ownerRefs, _, _ := unstructured.NestedSlice(pod.Object, "metadata", "ownerReferences")
			for _, ref := range ownerRefs {
				if refMap, ok := ref.(map[string]interface{}); ok {
					if refMap["kind"] == "ReplicaSet" {
						if rsOwnerName, ok := refMap["name"].(string); ok {
							rsKey := ns + "/" + rsOwnerName
							if depKey, exists := rsToDeployment[rsKey]; exists {
								if wl, exists := allWorkloads[depKey]; exists {
									phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
									podIP, _, _ := unstructured.NestedString(pod.Object, "status", "podIP")
									nodeName, _, _ := unstructured.NestedString(pod.Object, "spec", "nodeName")

									// Get restart count
									var restarts int64
									containers, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
									for _, c := range containers {
										if cMap, ok := c.(map[string]interface{}); ok {
											if rc, ok := cMap["restartCount"].(int64); ok {
												restarts += rc
											}
										}
									}

									wl.pods = append(wl.pods, podInfo{
										name:     podName,
										phase:    phase,
										ip:       podIP,
										node:     nodeName,
										restarts: restarts,
									})
								}
							}
						}
					}
				}
			}
		}
	}

	// ═══════════════════════════════════════════════════════════════════════════════
	// Build rich unit structure
	// ═══════════════════════════════════════════════════════════════════════════════
	type inferredUnit struct {
		name        string
		kind        string
		owner       string // Flux, ArgoCD, Helm, ConfigHub
		namespace   string
		sourceRepo  string
		sourcePath  string
		targetNs    string
		resources   int
		isInfra     bool
		appLabel    string
		environment string
		ready       bool
		statusMsg   string
		workloads   []*workloadDetail // workloads managed by this unit
		dependsOn   []string          // inferred dependencies
		dependedBy  []string          // inferred reverse dependencies
	}
	var units []*inferredUnit
	unitByName := map[string]*inferredUnit{}

	// Flux Kustomizations
	if ksList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, ks := range ksList.Items {
			name := ks.GetName()
			ns := ks.GetNamespace()
			path, _, _ := unstructured.NestedString(ks.Object, "spec", "path")
			sourceName, _, _ := unstructured.NestedString(ks.Object, "spec", "sourceRef", "name")
			targetNs, _, _ := unstructured.NestedString(ks.Object, "spec", "targetNamespace")
			inventory, _, _ := unstructured.NestedSlice(ks.Object, "status", "inventory", "entries")

			// Get source URL
			sourceURL := ""
			if gr, err := dynClient.Resource(schema.GroupVersionResource{
				Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories",
			}).Namespace(ns).Get(ctx, sourceName, v1.GetOptions{}); err == nil {
				sourceURL, _, _ = unstructured.NestedString(gr.Object, "spec", "url")
			}

			// Check ready status
			ready := isResourceReady(&ks)
			statusMsg := "Ready"
			if !ready {
				statusMsg = getConditionMessage(&ks)
			}

			// Detect if infrastructure
			nameLower := strings.ToLower(name)
			isInfra := strings.Contains(nameLower, "infra") ||
				strings.Contains(nameLower, "platform") ||
				strings.Contains(nameLower, "system")

			// Infer environment from namespace or path
			env := inferEnvironment(targetNs, path)

			unit := &inferredUnit{
				name:        name,
				kind:        "Kustomization",
				owner:       "Flux",
				namespace:   ns,
				sourceRepo:  sourceURL,
				sourcePath:  path,
				targetNs:    targetNs,
				resources:   len(inventory),
				isInfra:     isInfra,
				environment: env,
				ready:       ready,
				statusMsg:   statusMsg,
			}

			// Link workloads managed by this Kustomization
			for key, wl := range allWorkloads {
				if wl.managedBy == name && wl.owner == "Flux" {
					unit.workloads = append(unit.workloads, wl)
				}
				// Also check by target namespace match
				if wl.namespace == targetNs && wl.owner == "Flux" && wl.managedBy == name {
					// Already added above
				} else if wl.namespace == targetNs && strings.Contains(key, name) {
					unit.workloads = append(unit.workloads, wl)
				}
			}

			units = append(units, unit)
			unitByName[name] = unit
		}
	}

	// ArgoCD Applications
	if appList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, app := range appList.Items {
			name := app.GetName()
			ns := app.GetNamespace()
			repoURL, _, _ := unstructured.NestedString(app.Object, "spec", "source", "repoURL")
			path, _, _ := unstructured.NestedString(app.Object, "spec", "source", "path")
			destNs, _, _ := unstructured.NestedString(app.Object, "spec", "destination", "namespace")
			resources, _, _ := unstructured.NestedSlice(app.Object, "status", "resources")

			// Check health/sync status
			syncStatus, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")
			healthStatus, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
			ready := syncStatus == "Synced" && healthStatus == "Healthy"
			statusMsg := fmt.Sprintf("%s/%s", syncStatus, healthStatus)

			// Detect app label
			appLabel := ""
			labels := app.GetLabels()
			if labels != nil {
				if al, ok := labels["app.kubernetes.io/name"]; ok {
					appLabel = al
				} else if al, ok := labels["app"]; ok {
					appLabel = al
				}
			}

			nameLower := strings.ToLower(name)
			isInfra := strings.Contains(nameLower, "infra") ||
				strings.Contains(nameLower, "platform") ||
				strings.Contains(nameLower, "system")

			env := inferEnvironment(destNs, path)

			unit := &inferredUnit{
				name:        name,
				kind:        "Application",
				owner:       "ArgoCD",
				namespace:   ns,
				sourceRepo:  repoURL,
				sourcePath:  path,
				targetNs:    destNs,
				resources:   len(resources),
				isInfra:     isInfra,
				appLabel:    appLabel,
				environment: env,
				ready:       ready,
				statusMsg:   statusMsg,
			}

			// Link workloads managed by this Application
			for _, wl := range allWorkloads {
				if wl.owner == "ArgoCD" && (wl.managedBy == name || wl.namespace == destNs) {
					unit.workloads = append(unit.workloads, wl)
				}
			}

			units = append(units, unit)
			unitByName[name] = unit
		}
	}

	// Helm Releases (from secrets)
	helmReleases := map[string]string{} // namespace/name -> chart
	if secretList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "secrets",
	}).List(ctx, v1.ListOptions{
		LabelSelector: "owner=helm",
	}); err == nil {
		for _, secret := range secretList.Items {
			secretName := secret.GetName()
			namespace := secret.GetNamespace()

			if !strings.HasPrefix(secretName, "sh.helm.release.v1.") {
				continue
			}

			// Parse release name
			parts := strings.Split(secretName, ".")
			if len(parts) < 5 {
				continue
			}
			releaseName := parts[4]
			labels := secret.GetLabels()
			status := labels["status"]
			if status == "" {
				status = "unknown"
			}

			key := namespace + "/" + releaseName
			if _, exists := helmReleases[key]; exists {
				continue
			}

			// Decode to get chart info
			data, _, _ := unstructured.NestedStringMap(secret.Object, "data")
			chartName := releaseName
			if releaseData, ok := data["release"]; ok {
				if decoded := decodeHelmRelease(releaseData); decoded != nil {
					if chart, ok := decoded["chart"].(map[string]interface{}); ok {
						if metadata, ok := chart["metadata"].(map[string]interface{}); ok {
							if name, ok := metadata["name"].(string); ok {
								chartName = name
							}
						}
					}
				}
			}

			helmReleases[key] = chartName

			// Determine ready state based on status
			ready := status == "deployed"

			unit := &inferredUnit{
				name:        releaseName,
				kind:        "HelmRelease",
				owner:       "Helm",
				namespace:   namespace,
				targetNs:    namespace,
				appLabel:    chartName,
				environment: inferEnvironment(namespace, ""),
				ready:       ready,
				statusMsg:   status,
			}

			// Link workloads managed by this Helm release
			for _, wl := range allWorkloads {
				if wl.owner == "Helm" && wl.namespace == namespace {
					// Check if workload belongs to this release using multiple matching strategies
					if wl.labels != nil {
						matched := false
						// Strategy 1: app.kubernetes.io/instance == releaseName (standard Helm label)
						if instance := wl.labels["app.kubernetes.io/instance"]; instance == releaseName {
							matched = true
						}
						// Strategy 2: helm.sh/chart starts with chartName (e.g., "nginx-22.4.3" starts with "nginx")
						if !matched && wl.labels["helm.sh/chart"] != "" && strings.HasPrefix(wl.labels["helm.sh/chart"], chartName) {
							matched = true
						}
						// Strategy 3: app.kubernetes.io/name == chartName (common pattern)
						if !matched && wl.labels["app.kubernetes.io/name"] == chartName {
							matched = true
						}
						// Strategy 4: workload name starts with releaseName (naming convention)
						if !matched && strings.HasPrefix(wl.name, releaseName) {
							matched = true
						}
						// Strategy 5: meta.helm.sh/release-name annotation matches
						if !matched {
							if annotations := wl.labels; annotations != nil {
								// managedBy is set from meta.helm.sh/release-name annotation
								if wl.managedBy == releaseName {
									matched = true
								}
							}
						}
						if matched {
							unit.workloads = append(unit.workloads, wl)
						}
					}
				}
			}

			units = append(units, unit)
			unitByName[releaseName] = unit
		}
	}

	// ConfigHub-labeled workloads (already imported)
	for _, wl := range allWorkloads {
		if wl.owner == "ConfigHub" {
			unitSlug := wl.managedBy
			if unitSlug == "" {
				if slug, ok := wl.labels["confighub.com/UnitSlug"]; ok {
					unitSlug = slug
				}
			}
			if unitSlug != "" {
				unit := &inferredUnit{
					name:        unitSlug,
					kind:        "Unit",
					owner:       "ConfigHub",
					namespace:   wl.namespace,
					targetNs:    wl.namespace,
					environment: inferEnvironment(wl.namespace, ""),
					ready:       wl.ready,
					statusMsg:   "imported",
					workloads:   []*workloadDetail{wl},
				}
				units = append(units, unit)
				unitByName[unitSlug] = unit
			}
		}
	}

	// Native workloads (not managed by GitOps)
	var nativeWorkloads []*workloadDetail
	for _, wl := range allWorkloads {
		if wl.owner == "Native" {
			nativeWorkloads = append(nativeWorkloads, wl)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════════════
	// RICH TREE VIEW - Show each unit with full details
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("UNITS TREE (GitOps deployers + workloads + inferred dependencies)")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	// Helper to get owner icon
	ownerIcon := func(owner string) string {
		switch owner {
		case "Flux":
			return "⚡"
		case "ArgoCD":
			return "🅰"
		case "Helm":
			return "⎈"
		case "ConfigHub":
			return "📦"
		default:
			return "☸"
		}
	}

	// Helper to get status icon
	statusIcon := func(ready bool) string {
		if ready {
			return "✓"
		}
		return "✗"
	}

	// Sort units by owner then name
	sort.Slice(units, func(i, j int) bool {
		if units[i].owner != units[j].owner {
			return units[i].owner < units[j].owner
		}
		return units[i].name < units[j].name
	})

	// Display each unit with rich tree
	for _, u := range units {
		fmt.Println()
		fmt.Printf("%s %s %s/%s\n", ownerIcon(u.owner), statusIcon(u.ready), u.owner, u.name)
		fmt.Printf("│\n")

		// Source info
		if u.sourceRepo != "" {
			fmt.Printf("├─ Source: %s\n", u.sourceRepo)
			if u.sourcePath != "" {
				fmt.Printf("│          path: %s\n", u.sourcePath)
			}
		}
		if u.appLabel != "" && u.appLabel != u.name {
			fmt.Printf("├─ Chart:  %s\n", u.appLabel)
		}

		// Status
		fmt.Printf("├─ Status: %s\n", u.statusMsg)
		if u.environment != "" && u.environment != "unknown" {
			fmt.Printf("├─ Env:    %s (inferred)\n", u.environment)
		}
		fmt.Printf("├─ Target: %s\n", u.targetNs)

		// Workloads tree with LiveTree (Deployment → ReplicaSet → Pod)
		if len(u.workloads) > 0 {
			fmt.Printf("│\n")
			fmt.Printf("├─ Workloads (%d):\n", len(u.workloads))
			for i, wl := range u.workloads {
				isLastWl := i == len(u.workloads)-1
				wlPrefix := "│  ├─"
				wlChildPrefix := "│  │  "
				if isLastWl {
					wlPrefix = "│  └─"
					wlChildPrefix = "│     "
				}

				wlIcon := "✓"
				if !wl.ready {
					wlIcon = "✗"
				}

				fmt.Printf("%s %s %s/%s (%s)\n", wlPrefix, wlIcon, wl.kind, wl.name, wl.replicas)

				// Show images
				hasMoreContent := len(wl.replicaSets) > 0 || len(wl.serviceRefs) > 0
				for j, img := range wl.images {
					imgPrefix := wlChildPrefix + "├─"
					if j == len(wl.images)-1 && !hasMoreContent {
						imgPrefix = wlChildPrefix + "└─"
					}
					// Truncate long image names
					if len(img) > 50 {
						img = "..." + img[len(img)-47:]
					}
					fmt.Printf("%s image: %s\n", imgPrefix, img)
				}

				// Show LiveTree: ReplicaSets and Pods
				if len(wl.replicaSets) > 0 {
					for k, rsName := range wl.replicaSets {
						isLastRS := k == len(wl.replicaSets)-1 && len(wl.serviceRefs) == 0
						rsPrefix := wlChildPrefix + "├─"
						rsChildPrefix := wlChildPrefix + "│  "
						if isLastRS {
							rsPrefix = wlChildPrefix + "└─"
							rsChildPrefix = wlChildPrefix + "   "
						}

						// Get RS ready count
						rsKey := wl.namespace + "/" + rsName
						rsReadyStr := rsReady[rsKey]
						if rsReadyStr == "" {
							rsReadyStr = "?"
						}

						fmt.Printf("%s ReplicaSet/%s (%s)\n", rsPrefix, rsName, rsReadyStr)

						// Show pods under this RS (filter by RS name prefix)
						matchingPods := make([]podInfo, 0)
						for _, pod := range wl.pods {
							if strings.HasPrefix(pod.name, rsName) {
								matchingPods = append(matchingPods, pod)
							}
						}
						for m, pod := range matchingPods {
							isLastPod := m == len(matchingPods)-1
							podPrefix := rsChildPrefix + "├─"
							if isLastPod {
								podPrefix = rsChildPrefix + "└─"
							}

							podIcon := "✓"
							if pod.phase != "Running" {
								podIcon = "✗"
							}

							// Format: Pod/name (Running, 10.0.0.1, 0 restarts)
							restartInfo := ""
							if pod.restarts > 0 {
								restartInfo = fmt.Sprintf(", %d restart", pod.restarts)
								if pod.restarts > 1 {
									restartInfo += "s"
								}
							}
							fmt.Printf("%s %s Pod/%s (%s, %s%s)\n", podPrefix, podIcon, truncatePodName(pod.name), pod.phase, pod.ip, restartInfo)
						}
					}
				}

				// Show service references (inferred dependencies)
				if len(wl.serviceRefs) > 0 {
					fmt.Printf("%s└─ calls: %s\n", wlChildPrefix, strings.Join(wl.serviceRefs, ", "))
				}
			}
		} else if u.resources > 0 {
			fmt.Printf("├─ Resources: %d managed (no deployments)\n", u.resources)
		}

		// Inferred dependencies (from service refs)
		var allDeps []string
		for _, wl := range u.workloads {
			for _, ref := range wl.serviceRefs {
				// Check if this service belongs to another unit
				for _, otherUnit := range units {
					if otherUnit.name == ref || otherUnit.targetNs+"/"+ref == u.targetNs+"/"+ref {
						continue // Skip self
					}
					for _, otherWl := range otherUnit.workloads {
						if otherWl.name == ref {
							allDeps = append(allDeps, otherUnit.name)
						}
					}
				}
			}
		}
		if len(allDeps) > 0 {
			// Dedupe
			seen := map[string]bool{}
			var uniqueDeps []string
			for _, d := range allDeps {
				if !seen[d] {
					seen[d] = true
					uniqueDeps = append(uniqueDeps, d)
				}
			}
			fmt.Printf("│\n")
			fmt.Printf("└─ Depends on: → %s (inferred from env vars)\n", strings.Join(uniqueDeps, ", "))
		} else {
			fmt.Printf("│\n")
			fmt.Printf("└─ (no dependencies detected)\n")
		}
	}

	// Show native workloads (not managed by GitOps) with LiveTree
	if len(nativeWorkloads) > 0 {
		fmt.Println()
		fmt.Printf("☸ Native/Unmanaged Workloads (%d) - not tracked by GitOps\n", len(nativeWorkloads))
		fmt.Printf("│\n")
		for i, wl := range nativeWorkloads {
			isLastWl := i == len(nativeWorkloads)-1
			wlPrefix := "├─"
			wlChildPrefix := "│  "
			if isLastWl {
				wlPrefix = "└─"
				wlChildPrefix = "   "
			}
			wlIcon := "✓"
			if !wl.ready {
				wlIcon = "✗"
			}
			fmt.Printf("%s %s %s/%s (%s)\n", wlPrefix, wlIcon, wl.namespace, wl.name, wl.replicas)

			// Show image
			hasMoreContent := len(wl.replicaSets) > 0
			for j, img := range wl.images {
				imgPrefix := wlChildPrefix + "├─"
				if j == len(wl.images)-1 && !hasMoreContent {
					imgPrefix = wlChildPrefix + "└─"
				}
				if len(img) > 50 {
					img = "..." + img[len(img)-47:]
				}
				fmt.Printf("%s image: %s\n", imgPrefix, img)
			}

			// Show LiveTree: ReplicaSets and Pods
			if len(wl.replicaSets) > 0 {
				for k, rsName := range wl.replicaSets {
					isLastRS := k == len(wl.replicaSets)-1
					rsPrefix := wlChildPrefix + "├─"
					rsChildPrefix := wlChildPrefix + "│  "
					if isLastRS {
						rsPrefix = wlChildPrefix + "└─"
						rsChildPrefix = wlChildPrefix + "   "
					}

					rsKey := wl.namespace + "/" + rsName
					rsReadyStr := rsReady[rsKey]
					if rsReadyStr == "" {
						rsReadyStr = "?"
					}

					fmt.Printf("%s ReplicaSet/%s (%s)\n", rsPrefix, rsName, rsReadyStr)

					// Show pods under this RS
					matchingPods := make([]podInfo, 0)
					for _, pod := range wl.pods {
						if strings.HasPrefix(pod.name, rsName) {
							matchingPods = append(matchingPods, pod)
						}
					}
					for m, pod := range matchingPods {
						isLastPod := m == len(matchingPods)-1
						podPrefix := rsChildPrefix + "├─"
						if isLastPod {
							podPrefix = rsChildPrefix + "└─"
						}
						podIcon := "✓"
						if pod.phase != "Running" {
							podIcon = "✗"
						}
						restartInfo := ""
						if pod.restarts > 0 {
							restartInfo = fmt.Sprintf(", %d restart", pod.restarts)
							if pod.restarts > 1 {
								restartInfo += "s"
							}
						}
						fmt.Printf("%s %s Pod/%s (%s, %s%s)\n", podPrefix, podIcon, truncatePodName(pod.name), pod.phase, pod.ip, restartInfo)
					}
				}
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// NAMESPACE ANALYSIS
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("NAMESPACE ANALYSIS → INFERRED APPSPACES")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Println("Namespaces map to ConfigHub Spaces (environments/teams).")
	fmt.Println()

	// Get all namespaces with their workload counts
	type nsInfo struct {
		name        string
		workloads   int
		byOwner     map[string]int
		environment string
		labels      map[string]string
		annotations map[string]string
	}
	namespaces := map[string]*nsInfo{}

	// Get namespace details
	if nsList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "", Version: "v1", Resource: "namespaces",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, ns := range nsList.Items {
			name := ns.GetName()
			if isSystemNamespace(name) {
				continue
			}
			namespaces[name] = &nsInfo{
				name:        name,
				byOwner:     map[string]int{},
				environment: inferEnvironment(name, ""),
				labels:      ns.GetLabels(),
				annotations: ns.GetAnnotations(),
			}
		}
	}

	// Count workloads per namespace
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if info, ok := namespaces[ns]; ok {
				info.workloads++
				owner, _ := detectOwnership(&dep)
				info.byOwner[owner]++
			}
		}
	}

	// Group namespaces by environment
	byEnv := map[string][]*nsInfo{}
	for _, info := range namespaces {
		if info.workloads > 0 {
			byEnv[info.environment] = append(byEnv[info.environment], info)
		}
	}

	for _, env := range []string{"production", "staging", "development", "unknown"} {
		nss := byEnv[env]
		if len(nss) == 0 {
			continue
		}
		fmt.Printf("[%s] %d namespace(s)\n", strings.ToUpper(env), len(nss))
		for _, info := range nss {
			fmt.Printf("\n  %s\n", info.name)
			fmt.Printf("    Workloads: %d total\n", info.workloads)
			for owner, count := range info.byOwner {
				fmt.Printf("      - %s: %d\n", owner, count)
			}
			// Show namespace labels
			if team, ok := info.labels["team"]; ok {
				fmt.Printf("    Team:      %s\n", team)
			}
			if owner, ok := info.labels["owner"]; ok {
				fmt.Printf("    Owner:     %s\n", owner)
			}
			if app, ok := info.labels["app"]; ok {
				fmt.Printf("    App:       %s\n", app)
			}
			// Check for Kubernetes standard labels
			if env, ok := info.labels["environment"]; ok {
				fmt.Printf("    Env Label: %s\n", env)
			}
		}
		fmt.Println()
	}

	// ═══════════════════════════════════════════════════════════════════════════════
	// OWNERSHIP GRAPH
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("OWNERSHIP GRAPH → WHO MANAGES WHAT")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println()

	// Build ownership tree - map workloads to their managing deployer
	workloadToDeployer := map[string]string{} // "ns/name" -> "deployer"

	// From Flux labels
	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			name := dep.GetName()
			labels := dep.GetLabels()
			if labels == nil {
				continue
			}

			key := fmt.Sprintf("%s/%s", ns, name)
			if ksName, ok := labels["kustomize.toolkit.fluxcd.io/name"]; ok {
				workloadToDeployer[key] = fmt.Sprintf("Kustomization/%s", ksName)
			} else if hrName, ok := labels["helm.toolkit.fluxcd.io/name"]; ok {
				workloadToDeployer[key] = fmt.Sprintf("HelmRelease/%s", hrName)
			} else if argoApp, ok := labels["argocd.argoproj.io/instance"]; ok {
				workloadToDeployer[key] = fmt.Sprintf("Application/%s", argoApp)
			}
		}
	}

	// Group workloads by deployer
	deployerWorkloads := map[string][]string{}
	for workload, deployer := range workloadToDeployer {
		deployerWorkloads[deployer] = append(deployerWorkloads[deployer], workload)
	}

	for deployer, workloads := range deployerWorkloads {
		sort.Strings(workloads)
		fmt.Printf("%s\n", deployer)
		for i, w := range workloads {
			prefix := "├──"
			if i == len(workloads)-1 {
				prefix = "└──"
			}
			fmt.Printf("  %s %s\n", prefix, w)
		}
		fmt.Println()
	}

	// ═══════════════════════════════════════════════════════════════════════════════
	// LABEL ANALYSIS
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println("LABEL ANALYSIS → POTENTIAL CONFIGHUB LABELS")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")
	fmt.Println()

	// Collect all unique label keys and their values
	labelUsage := map[string]map[string]int{} // label -> value -> count

	if depList, err := dynClient.Resource(schema.GroupVersionResource{
		Group: "apps", Version: "v1", Resource: "deployments",
	}).List(ctx, v1.ListOptions{}); err == nil {
		for _, dep := range depList.Items {
			ns := dep.GetNamespace()
			if isSystemNamespace(ns) {
				continue
			}
			labels := dep.GetLabels()
			if labels == nil {
				continue
			}
			for k, v := range labels {
				if labelUsage[k] == nil {
					labelUsage[k] = map[string]int{}
				}
				labelUsage[k][v]++
			}
		}
	}

	// Show interesting labels (non-system, multiple values)
	interestingLabels := []string{
		"app", "app.kubernetes.io/name", "app.kubernetes.io/instance",
		"app.kubernetes.io/component", "app.kubernetes.io/part-of",
		"app.kubernetes.io/version", "app.kubernetes.io/managed-by",
		"team", "owner", "environment", "tier", "release",
	}

	fmt.Println("Standard Kubernetes labels found:")
	for _, label := range interestingLabels {
		if values, ok := labelUsage[label]; ok && len(values) > 0 {
			valueList := make([]string, 0, len(values))
			for v, count := range values {
				valueList = append(valueList, fmt.Sprintf("%s(%d)", v, count))
			}
			sort.Strings(valueList)
			fmt.Printf("  %s:\n", label)
			for _, v := range valueList {
				fmt.Printf("    - %s\n", v)
			}
		}
	}
	fmt.Println()

	// ═══════════════════════════════════════════════════════════════════════════════
	// CONFIGHUB MAPPING SUGGESTIONS
	// ═══════════════════════════════════════════════════════════════════════════════
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("SUGGESTED CONFIGHUB MAPPING")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Based on cluster analysis, here's how to map to ConfigHub model:")
	fmt.Println()

	// Count stats
	fluxCount := 0
	argoCount := 0
	for _, u := range units {
		if strings.HasPrefix(u.kind, "Flux") {
			fluxCount++
		} else {
			argoCount++
		}
	}

	fmt.Println("RECOMMENDED IMPORT STRATEGY:")
	if fluxCount > 0 && argoCount > 0 {
		fmt.Println("  Mixed GitOps (Flux + Argo) - import each deployer as a Unit")
	} else if fluxCount > 0 {
		fmt.Println("  Pure Flux - import each Kustomization/HelmRelease as a Unit")
	} else if argoCount > 0 {
		fmt.Println("  Pure ArgoCD - import each Application as a Unit")
	} else {
		fmt.Println("  No GitOps deployers found - consider adding Flux or ArgoCD first")
	}
	fmt.Println()

	fmt.Println("POTENTIAL SPACES:")
	for env, nss := range byEnv {
		if len(nss) > 0 {
			nsNames := make([]string, 0, len(nss))
			for _, ns := range nss {
				nsNames = append(nsNames, ns.name)
			}
			fmt.Printf("  %s-space: %s\n", env, strings.Join(nsNames, ", "))
		}
	}
	fmt.Println()

	fmt.Println("COMMANDS TO IMPORT:")
	fmt.Println("  cub-scout map              # Launch TUI, press 'i' for import wizard")
	fmt.Println("  cub-scout import --help    # See import options")
	fmt.Println()

	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("WHAT CONFIGHUB PROVIDES (beyond inference)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  • Explicit Unit definitions with metadata")
	fmt.Println("  • Space hierarchy (Organization → Space → Unit)")
	fmt.Println("  • Cross-cluster Unit linking and dependencies")
	fmt.Println("  • Revision history for every change")
	fmt.Println("  • Diff between revisions")
	fmt.Println("  • Approval workflows")
	fmt.Println("  • Team-based access control")
	fmt.Println("  • Audit log of all operations")
	fmt.Println()

	return nil
}

// inferEnvironment guesses the environment from namespace or path
func inferEnvironment(namespace, path string) string {
	combined := strings.ToLower(namespace + " " + path)
	if strings.Contains(combined, "prod") {
		return "production"
	}
	if strings.Contains(combined, "staging") || strings.Contains(combined, "stage") {
		return "staging"
	}
	if strings.Contains(combined, "dev") {
		return "development"
	}
	if strings.Contains(combined, "test") {
		return "testing"
	}
	return "unknown"
}

// truncatePodName shortens pod names for display (removes random suffix hash)
func truncatePodName(name string) string {
	// Pod names typically look like: deployment-name-replicaset-hash-pod-hash
	// We want to show just the last parts to keep it readable
	parts := strings.Split(name, "-")
	if len(parts) > 4 {
		// Show last 4 parts (rs-hash-pod-hash)
		return "..." + strings.Join(parts[len(parts)-4:], "-")
	}
	return name
}
