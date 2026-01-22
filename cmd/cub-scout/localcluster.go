package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// TUISnapshot represents saved TUI state for resumption
type TUISnapshot struct {
	Version      string    `json:"version"`
	UpdatedAt    time.Time `json:"updated_at"`
	ClusterName  string    `json:"cluster_name"`
	PanelMode    bool      `json:"panel_mode"`
	PanelView    int       `json:"panel_view"`
	Cursor       int       `json:"cursor"`
	NamespaceIdx int       `json:"namespace_idx"`
	ActiveQuery  string    `json:"active_query,omitempty"`
}

const snapshotVersion = "1.0"

func getSnapshotPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".confighub", "sessions", "localcluster-snapshot.json")
}

func loadSnapshot() *TUISnapshot {
	path := getSnapshotPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var snap TUISnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil
	}
	// Only restore if snapshot is less than 24 hours old
	if time.Since(snap.UpdatedAt) > 24*time.Hour {
		return nil
	}
	return &snap
}

func saveSnapshot(m *LocalClusterModel) {
	snap := TUISnapshot{
		Version:      snapshotVersion,
		UpdatedAt:    time.Now(),
		ClusterName:  m.clusterName,
		PanelMode:    m.panelMode,
		PanelView:    int(m.panelView),
		Cursor:       m.cursor,
		NamespaceIdx: m.namespaceIdx,
	}
	if m.activeQuery != nil {
		snap.ActiveQuery = m.activeQuery.Name
	}

	path := getSnapshotPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}

// SavedQuery represents a predefined query
type SavedQuery struct {
	Name        string
	Description string
	Query       string
}

// Built-in saved queries
var savedQueries = []SavedQuery{
	{Name: "all", Description: "All resources (no filter)", Query: ""},
	{Name: "orphans", Description: "Unmanaged resources (Native)", Query: "owner=Native"},
	{Name: "gitops", Description: "GitOps-managed only", Query: "owner!=Native"},
	{Name: "flux", Description: "Flux-managed resources", Query: "owner=Flux"},
	{Name: "argo", Description: "ArgoCD-managed resources", Query: "owner=ArgoCD"},
	{Name: "helm", Description: "Helm-managed resources", Query: "owner=Helm"},
	{Name: "confighub", Description: "ConfigHub-managed resources", Query: "owner=ConfigHub"},
	{Name: "prod", Description: "Production namespaces", Query: "namespace=*-prod,prod-*,production"},
	{Name: "dev", Description: "Development namespaces", Query: "namespace=*-dev,dev-*,development"},
}

// LocalClusterModel represents the local cluster TUI state
type LocalClusterModel struct {
	entries     []MapEntry
	gitops      []GitOpsResource
	gitSources  []GitSourceInfo // Git sources (GitRepository, OCIRepository, HelmRepository)
	width       int
	height      int
	ready       bool
	loading     bool
	err         error
	cursor      int
	view        localView
	spinner     spinner.Model
	keymap      localKeyMap
	statusMsg   string
	clusterName string
	contextName string // kubectl context name

	// Search
	searchMode  bool
	searchQuery string

	// Query mode (saved queries)
	queryMode     bool        // Show query selector
	queryCursor   int         // Cursor in query list
	activeQuery   *SavedQuery // Currently active query filter
	customQuery   string      // User-typed custom query

	// Help overlay
	helpMode bool

	// Panel mode (split view like hierarchy)
	panelMode    bool            // Show split pane view
	panelView    localView       // Which view to show in panel
	panelPane    viewport.Model  // Scrollable viewport for panel
	panelFocused bool            // Is the panel focused (for scrolling)

	// Selected resource (for trace/actions)
	selectedEntry *MapEntry // Currently selected workload
	selectedGitOps *GitOpsResource // Currently selected GitOps resource

	// Trace mode
	traceMode       bool          // In trace picker mode
	traceCursor     int           // Cursor in trace picker
	traceItems      []TraceItem   // Items available to trace
	traceOutput     string        // Output from trace command
	traceLoading    bool          // Is trace running
	traceError      error         // Trace error if any

	// Scan mode
	scanMode       bool              // In scan result mode
	scanOutput     string            // Output from scan command
	scanLoading    bool              // Is scan running
	scanError      error             // Scan error if any
	scanFindings   []scanFinding     // Parsed findings
	scanCategories map[string]int    // Category counts

	// Cross-reference navigation
	xrefMode       bool        // In cross-reference view mode
	xrefItems      []xrefItem  // Cross-reference items
	xrefCursor     int         // Cursor in xref list
	xrefSourceType string      // Type of source (workload, gitops, etc.)
	xrefSourceName string      // Name of source item

	// Switch to ConfigHub mode
	switchToHub    bool
	authNeeded     bool
	hubContext     string   // Context to pass to hub (e.g., app name to filter)
	detectedApps   []string // Apps detected from cluster namespaces

	// Switch to Import wizard
	switchToImport bool

	// Namespace navigation
	namespaces   []string // Sorted list of namespaces
	namespaceIdx int      // 0 = all, 1+ = specific namespace index

	// Command mode (: key)
	cmdMode       bool     // Command input active
	cmdInput      string   // Current command being typed
	cmdHistory    []string // Recent commands (max 20)
	cmdHistoryIdx int      // Position in history (-1 = current input)
	cmdRunning    bool     // Command is executing
	cmdShowOutput bool     // Show command output
	cmdOutput     string   // Output from last command
}

// GitOpsResource represents a Flux/ArgoCD resource
type GitOpsResource struct {
	Kind           string
	Name           string
	Namespace      string
	Status         string
	Source         string
	Path           string
	InventoryCount int
	LastApplied    time.Time
	DependsOn      []string // Dependencies (namespace/name format)
}

// GitSourceInfo represents a Git source (GitRepository, OCIRepository, etc.)
type GitSourceInfo struct {
	Kind       string    // GitRepository, OCIRepository, HelmRepository
	Name       string    // Resource name
	Namespace  string    // Resource namespace
	URL        string    // Git URL, OCI URL, or Helm repo URL
	Branch     string    // Branch (for GitRepository)
	Tag        string    // Tag (for GitRepository)
	Revision   string    // Current resolved revision (commit SHA, etc.)
	Status     string    // Ready, NotReady, etc.
	LastFetch  time.Time // Last successful fetch time
	Interval   string    // Reconciliation interval
	Deployers  []string  // Kustomizations/HelmReleases that reference this source
}

// TraceItem represents an item that can be traced
type TraceItem struct {
	Kind      string // Deployment, StatefulSet, Kustomization, Application, etc.
	Name      string
	Namespace string
	Owner     string // Flux, ArgoCD, Helm, Native
}

type localView int

const (
	viewDashboard localView = iota
	viewWorkloads
	viewPipelines
	viewDrift
	viewOrphans
	viewCrashes
	viewIssues
	viewBypass
	viewSprawl
	viewMaps
	viewTracePicker   // Trace resource picker
	viewTraceResult   // Trace result display
	viewScanResult    // Scan result display
	viewSuspended     // Suspended/paused resources
	viewApps          // Apps view (grouped by app label)
	viewDependencies  // Dependencies view (upstream/downstream)
	viewGitSources    // GitOps Sources view (Git repos → deployers → resources)
	viewClusterData   // Cluster Data view (all data sources TUI reads)
	viewAppHierarchy  // App Hierarchy view (inferred ConfigHub model)
)

type localKeyMap struct {
	Up      key.Binding
	Down    key.Binding
	Quit    key.Binding
	Help    key.Binding
	Refresh key.Binding
	Search  key.Binding
	Hub     key.Binding
	Tab     key.Binding
	Enter   key.Binding
	// View shortcuts
	Dashboard key.Binding
	Workloads key.Binding
	Pipelines key.Binding
	Drift     key.Binding
	Orphans   key.Binding
	Crashes   key.Binding
	Issues    key.Binding
	Bypass    key.Binding
	Sprawl    key.Binding
	Maps      key.Binding
	// Actions on selected resource
	Trace key.Binding
	Scan  key.Binding
	// Query mode
	Query key.Binding
	// Import wizard
	Import key.Binding
	// Suspended view
	Suspended key.Binding
	// Apps view
	Apps key.Binding
	// Dependencies view
	Dependencies key.Binding
	// GitOps Sources view
	GitSources key.Binding
	// Cluster Data view (new)
	ClusterData key.Binding
	// App Hierarchy view (new)
	AppHierarchy key.Binding
	// Namespace navigation
	NextNamespace key.Binding
	PrevNamespace key.Binding
	// Command palette
	Command key.Binding
}

func defaultLocalKeyMap() localKeyMap {
	return localKeyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Quit:      key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Refresh:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Search:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Hub:       key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "ConfigHub")),
		Tab:       key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next view")),
		Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Dashboard: key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "status")),
		Workloads: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "workloads")),
		Pipelines: key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pipelines")),
		Drift:     key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "drift")),
		Orphans:   key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "orphans")),
		Crashes:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "crashes")),
		Issues:    key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "issues")),
		Bypass:    key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "bypass")),
		Sprawl:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "sprawl")),
		Maps:      key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "maps")),
		Trace:     key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "trace")),
		Scan:      key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "scan")),
		Query:     key.NewBinding(key.WithKeys("Q"), key.WithHelp("Q", "query")),
		Import:    key.NewBinding(key.WithKeys("I"), key.WithHelp("I", "import")),
		Suspended:     key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "suspended")),
		Apps:          key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "apps")),
		Dependencies:  key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "dependencies")),
		GitSources:    key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "git sources")),
		ClusterData:   key.NewBinding(key.WithKeys("4"), key.WithHelp("4", "cluster data")),
		AppHierarchy:  key.NewBinding(key.WithKeys("5", "A"), key.WithHelp("5/A", "app hierarchy")),
		NextNamespace: key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next ns")),
		PrevNamespace: key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev ns")),
		Command:       key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "command")),
	}
}

// Messages
type localDataLoadedMsg struct {
	entries      []MapEntry
	gitops       []GitOpsResource
	gitSources   []GitSourceInfo // Git sources (GitRepository, etc.)
	detectedApps []string        // App names detected from namespaces
	err          error
}

type localAuthCheckMsg struct {
	authenticated bool
}

type traceResultMsg struct {
	output string
	err    error
}

type scanResultMsg struct {
	output     string
	err        error
	findings   []scanFinding // Parsed findings for category display
	categories map[string]int // Category counts
}

type localCmdCompleteMsg struct {
	output string
	err    error
}

// scanFinding represents a single CCVE finding for TUI display
type scanFinding struct {
	CCVE        string
	Severity    string
	Category    string
	Resource    string
	Namespace   string
	Message     string
}

// xrefItem represents a cross-reference item for navigation
type xrefItem struct {
	Type        string // "workload", "gitops", "namespace", etc.
	Kind        string // Deployment, Kustomization, etc.
	Name        string
	Namespace   string
	Relation    string // "manages", "managed by", "in namespace", etc.
	TargetView  localView // View to switch to when selected
}

func initialLocalModel() LocalClusterModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Initialize panel viewport
	vp := viewport.New(40, 20)
	vp.MouseWheelEnabled = true

	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "default"
	}

	contextName := getCurrentContext()

	m := LocalClusterModel{
		loading:     true,
		spinner:     s,
		keymap:      defaultLocalKeyMap(),
		view:        viewDashboard,
		clusterName: clusterName,
		contextName: contextName,
		panelPane:   vp,
	}

	// Restore from snapshot if available and matches current cluster
	if snap := loadSnapshot(); snap != nil && snap.ClusterName == clusterName {
		m.panelMode = snap.PanelMode
		m.panelView = localView(snap.PanelView)
		m.cursor = snap.Cursor
		m.namespaceIdx = snap.NamespaceIdx
		// Restore active query by name
		if snap.ActiveQuery != "" {
			for i := range savedQueries {
				if savedQueries[i].Name == snap.ActiveQuery {
					m.activeQuery = &savedQueries[i]
					break
				}
			}
		}
	}

	return m
}

func (m LocalClusterModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		loadLocalClusterData,
	)
}

func loadLocalClusterData() tea.Msg {
	ctx := context.Background()

	cfg, err := buildConfig()
	if err != nil {
		return localDataLoadedMsg{err: fmt.Errorf("build kubernetes config: %w", err)}
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return localDataLoadedMsg{err: fmt.Errorf("create dynamic client: %w", err)}
	}

	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "default"
	}

	var entries []MapEntry
	var gitops []GitOpsResource
	byOwner := map[string]int{}

	// Workload resources
	workloadResources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
	}

	for _, gvr := range workloadResources {
		l, err := dynClient.Resource(gvr).List(ctx, v1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range l.Items {
			entries = processResource(&item, gvr, clusterName, entries, byOwner)
		}
	}

	// Flux Kustomizations
	fluxKustGVR := schema.GroupVersionResource{
		Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations",
	}
	if l, err := dynClient.Resource(fluxKustGVR).List(ctx, v1.ListOptions{}); err == nil {
		for _, item := range l.Items {
			gr := parseFluxKustomization(&item)
			gitops = append(gitops, gr)
		}
	}

	// Flux HelmReleases
	fluxHelmGVR := schema.GroupVersionResource{
		Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases",
	}
	if l, err := dynClient.Resource(fluxHelmGVR).List(ctx, v1.ListOptions{}); err == nil {
		for _, item := range l.Items {
			gr := parseFluxHelmRelease(&item)
			gitops = append(gitops, gr)
		}
	}

	// ArgoCD Applications
	argoGVR := schema.GroupVersionResource{
		Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
	}
	if l, err := dynClient.Resource(argoGVR).List(ctx, v1.ListOptions{}); err == nil {
		for _, item := range l.Items {
			gr := parseArgoApplication(&item)
			gitops = append(gitops, gr)
		}
	}

	// Git Sources: GitRepository, OCIRepository, HelmRepository
	var gitSources []GitSourceInfo

	// Flux GitRepositories
	gitRepoGVR := schema.GroupVersionResource{
		Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories",
	}
	if l, err := dynClient.Resource(gitRepoGVR).List(ctx, v1.ListOptions{}); err == nil {
		for _, item := range l.Items {
			src := parseFluxGitRepository(&item)
			gitSources = append(gitSources, src)
		}
	}

	// Flux OCIRepositories
	ociRepoGVR := schema.GroupVersionResource{
		Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "ocirepositories",
	}
	if l, err := dynClient.Resource(ociRepoGVR).List(ctx, v1.ListOptions{}); err == nil {
		for _, item := range l.Items {
			src := parseFluxOCIRepository(&item)
			gitSources = append(gitSources, src)
		}
	}

	// Flux HelmRepositories
	helmRepoGVR := schema.GroupVersionResource{
		Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories",
	}
	if l, err := dynClient.Resource(helmRepoGVR).List(ctx, v1.ListOptions{}); err == nil {
		for _, item := range l.Items {
			src := parseFluxHelmRepository(&item)
			gitSources = append(gitSources, src)
		}
	}

	// Build source → deployer mapping
	sourceToDeployers := make(map[string][]string) // "namespace/name" → list of deployer names
	for _, g := range gitops {
		if g.Source != "" {
			// For Kustomizations/HelmReleases, g.Source is the sourceRef name
			// Assume same namespace unless specified otherwise
			key := g.Namespace + "/" + g.Source
			sourceToDeployers[key] = append(sourceToDeployers[key], g.Kind+"/"+g.Name)
		}
	}
	// Update gitSources with their deployers
	for i := range gitSources {
		key := gitSources[i].Namespace + "/" + gitSources[i].Name
		gitSources[i].Deployers = sourceToDeployers[key]
	}

	// Detect app names from namespaces (for Hub context)
	// Look for patterns like "appname-prod", "appname-dev", etc.
	appSet := make(map[string]bool)
	for _, e := range entries {
		ns := e.Namespace
		// Skip system namespaces
		if ns == "" || ns == "kube-system" || ns == "kube-public" || ns == "default" ||
			ns == "flux-system" || ns == "argocd" || ns == "cert-manager" {
			continue
		}
		// Extract base app name (remove -prod, -dev, -staging suffixes)
		appName := ns
		for _, suffix := range []string{"-prod", "-dev", "-staging", "-test", "-qa"} {
			if strings.HasSuffix(ns, suffix) {
				appName = strings.TrimSuffix(ns, suffix)
				break
			}
		}
		appSet[appName] = true
	}
	detectedApps := make([]string, 0, len(appSet))
	for app := range appSet {
		detectedApps = append(detectedApps, app)
	}
	sort.Strings(detectedApps)

	return localDataLoadedMsg{entries: entries, gitops: gitops, gitSources: gitSources, detectedApps: detectedApps}
}

func parseFluxKustomization(item *unstructured.Unstructured) GitOpsResource {
	name := item.GetName()
	ns := item.GetNamespace()

	status := "Unknown"
	if conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
		for _, c := range conditions {
			if cm, ok := c.(map[string]interface{}); ok {
				if cm["type"] == "Ready" {
					if cm["status"] == "True" {
						status = "Ready"
					} else {
						status = "NotReady"
					}
					break
				}
			}
		}
	}

	sourceName, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "name")
	path, _, _ := unstructured.NestedString(item.Object, "spec", "path")

	// Count inventory
	invCount := 0
	if inv, ok, _ := unstructured.NestedSlice(item.Object, "status", "inventory", "entries"); ok {
		invCount = len(inv)
	}

	// Parse dependsOn
	var dependsOn []string
	if deps, ok, _ := unstructured.NestedSlice(item.Object, "spec", "dependsOn"); ok {
		for _, dep := range deps {
			if dm, ok := dep.(map[string]interface{}); ok {
				depName, _ := dm["name"].(string)
				depNs, _ := dm["namespace"].(string)
				if depNs == "" {
					depNs = ns // Same namespace if not specified
				}
				if depName != "" {
					dependsOn = append(dependsOn, depNs+"/"+depName)
				}
			}
		}
	}

	return GitOpsResource{
		Kind:           "Kustomization",
		Name:           name,
		Namespace:      ns,
		Status:         status,
		Source:         sourceName,
		Path:           path,
		InventoryCount: invCount,
		DependsOn:      dependsOn,
	}
}

func parseFluxHelmRelease(item *unstructured.Unstructured) GitOpsResource {
	name := item.GetName()
	ns := item.GetNamespace()

	status := "Unknown"
	if conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
		for _, c := range conditions {
			if cm, ok := c.(map[string]interface{}); ok {
				if cm["type"] == "Ready" {
					if cm["status"] == "True" {
						status = "Ready"
					} else {
						status = "NotReady"
					}
					break
				}
			}
		}
	}

	chartName, _, _ := unstructured.NestedString(item.Object, "spec", "chart", "spec", "chart")

	// Parse dependsOn
	var dependsOn []string
	if deps, ok, _ := unstructured.NestedSlice(item.Object, "spec", "dependsOn"); ok {
		for _, dep := range deps {
			if dm, ok := dep.(map[string]interface{}); ok {
				depName, _ := dm["name"].(string)
				depNs, _ := dm["namespace"].(string)
				if depNs == "" {
					depNs = ns
				}
				if depName != "" {
					dependsOn = append(dependsOn, depNs+"/"+depName)
				}
			}
		}
	}

	return GitOpsResource{
		Kind:      "HelmRelease",
		Name:      name,
		Namespace: ns,
		Status:    status,
		Source:    chartName,
		DependsOn: dependsOn,
	}
}

func parseArgoApplication(item *unstructured.Unstructured) GitOpsResource {
	name := item.GetName()
	ns := item.GetNamespace()

	status := "Unknown"
	if health, ok, _ := unstructured.NestedString(item.Object, "status", "health", "status"); ok {
		status = health
	}

	repoURL, _, _ := unstructured.NestedString(item.Object, "spec", "source", "repoURL")
	path, _, _ := unstructured.NestedString(item.Object, "spec", "source", "path")

	return GitOpsResource{
		Kind:      "Application",
		Name:      name,
		Namespace: ns,
		Status:    status,
		Source:    repoURL,
		Path:      path,
	}
}

func parseFluxGitRepository(item *unstructured.Unstructured) GitSourceInfo {
	name := item.GetName()
	ns := item.GetNamespace()

	url, _, _ := unstructured.NestedString(item.Object, "spec", "url")
	branch, _, _ := unstructured.NestedString(item.Object, "spec", "ref", "branch")
	tag, _, _ := unstructured.NestedString(item.Object, "spec", "ref", "tag")
	interval, _, _ := unstructured.NestedString(item.Object, "spec", "interval")

	status := "Unknown"
	revision := ""
	var lastFetch time.Time
	if conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
		for _, c := range conditions {
			if cm, ok := c.(map[string]interface{}); ok {
				if cm["type"] == "Ready" {
					if cm["status"] == "True" {
						status = "Ready"
					} else {
						status = "NotReady"
					}
					break
				}
			}
		}
	}
	if artifact, ok, _ := unstructured.NestedMap(item.Object, "status", "artifact"); ok {
		revision, _, _ = unstructured.NestedString(artifact, "revision")
		if lastUpdate, ok, _ := unstructured.NestedString(artifact, "lastUpdateTime"); ok {
			lastFetch, _ = time.Parse(time.RFC3339, lastUpdate)
		}
	}

	return GitSourceInfo{
		Kind:      "GitRepository",
		Name:      name,
		Namespace: ns,
		URL:       url,
		Branch:    branch,
		Tag:       tag,
		Revision:  revision,
		Status:    status,
		LastFetch: lastFetch,
		Interval:  interval,
	}
}

func parseFluxOCIRepository(item *unstructured.Unstructured) GitSourceInfo {
	name := item.GetName()
	ns := item.GetNamespace()

	url, _, _ := unstructured.NestedString(item.Object, "spec", "url")
	tag, _, _ := unstructured.NestedString(item.Object, "spec", "ref", "tag")
	interval, _, _ := unstructured.NestedString(item.Object, "spec", "interval")

	status := "Unknown"
	revision := ""
	var lastFetch time.Time
	if conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
		for _, c := range conditions {
			if cm, ok := c.(map[string]interface{}); ok {
				if cm["type"] == "Ready" {
					if cm["status"] == "True" {
						status = "Ready"
					} else {
						status = "NotReady"
					}
					break
				}
			}
		}
	}
	if artifact, ok, _ := unstructured.NestedMap(item.Object, "status", "artifact"); ok {
		revision, _, _ = unstructured.NestedString(artifact, "revision")
		if lastUpdate, ok, _ := unstructured.NestedString(artifact, "lastUpdateTime"); ok {
			lastFetch, _ = time.Parse(time.RFC3339, lastUpdate)
		}
	}

	return GitSourceInfo{
		Kind:      "OCIRepository",
		Name:      name,
		Namespace: ns,
		URL:       url,
		Tag:       tag,
		Revision:  revision,
		Status:    status,
		LastFetch: lastFetch,
		Interval:  interval,
	}
}

func parseFluxHelmRepository(item *unstructured.Unstructured) GitSourceInfo {
	name := item.GetName()
	ns := item.GetNamespace()

	url, _, _ := unstructured.NestedString(item.Object, "spec", "url")
	repoType, _, _ := unstructured.NestedString(item.Object, "spec", "type")
	interval, _, _ := unstructured.NestedString(item.Object, "spec", "interval")

	status := "Unknown"
	var lastFetch time.Time
	if conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
		for _, c := range conditions {
			if cm, ok := c.(map[string]interface{}); ok {
				if cm["type"] == "Ready" {
					if cm["status"] == "True" {
						status = "Ready"
					} else {
						status = "NotReady"
					}
					break
				}
			}
		}
	}
	if artifact, ok, _ := unstructured.NestedMap(item.Object, "status", "artifact"); ok {
		if lastUpdate, ok, _ := unstructured.NestedString(artifact, "lastUpdateTime"); ok {
			lastFetch, _ = time.Parse(time.RFC3339, lastUpdate)
		}
	}

	return GitSourceInfo{
		Kind:      "HelmRepository",
		Name:      name,
		Namespace: ns,
		URL:       url,
		Tag:       repoType, // Use tag field to store repo type (default/oci)
		Status:    status,
		LastFetch: lastFetch,
		Interval:  interval,
	}
}

func checkCubAuthForSwitch() tea.Msg {
	_, err := runCubCommand("context", "get")
	return localAuthCheckMsg{authenticated: err == nil}
}

func (m LocalClusterModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case localDataLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.entries = msg.entries
			m.gitops = msg.gitops
			m.gitSources = msg.gitSources
			m.detectedApps = msg.detectedApps
			// Set default hub context to first detected app
			if len(msg.detectedApps) > 0 {
				m.hubContext = msg.detectedApps[0]
			}
			// Build sorted namespace list for navigation
			nsSet := make(map[string]bool)
			for _, e := range msg.entries {
				if e.Namespace != "" {
					nsSet[e.Namespace] = true
				}
			}
			m.namespaces = make([]string, 0, len(nsSet))
			for ns := range nsSet {
				m.namespaces = append(m.namespaces, ns)
			}
			sort.Strings(m.namespaces)
		}
		return m, nil

	case localAuthCheckMsg:
		if msg.authenticated {
			m.switchToHub = true
			saveSnapshot(&m)
			return m, tea.Quit
		} else {
			m.authNeeded = true
		}
		return m, nil

	case traceResultMsg:
		m.traceLoading = false
		m.traceOutput = msg.output
		m.traceError = msg.err
		return m, nil

	case scanResultMsg:
		m.scanLoading = false
		m.scanOutput = msg.output
		m.scanError = msg.err
		m.scanFindings = msg.findings
		m.scanCategories = msg.categories
		return m, nil

	case localCmdCompleteMsg:
		m.cmdRunning = false
		if msg.err != nil {
			m.cmdOutput = fmt.Sprintf("Error: %v\n%s", msg.err, msg.output)
		} else {
			m.cmdOutput = msg.output
		}
		return m, nil

	case tea.KeyMsg:
		// Handle auth needed state
		if m.authNeeded {
			switch msg.String() {
			case "y", "Y":
				// Run cub auth login
				m.authNeeded = false
				m.statusMsg = "Opening browser for authentication..."
				return m, runAuthLogin
			case "n", "N", "esc":
				m.authNeeded = false
				return m, nil
			}
			return m, nil
		}

		// Handle help mode
		if m.helpMode {
			m.helpMode = false
			return m, nil
		}

		// Handle cross-reference mode
		if m.xrefMode {
			switch msg.String() {
			case "esc", "q":
				m.xrefMode = false
				return m, nil
			case "up", "k":
				if m.xrefCursor > 0 {
					m.xrefCursor--
				}
				return m, nil
			case "down", "j":
				if m.xrefCursor < len(m.xrefItems)-1 {
					m.xrefCursor++
				}
				return m, nil
			case "enter":
				// Navigate to selected cross-reference
				if m.xrefCursor >= 0 && m.xrefCursor < len(m.xrefItems) {
					item := m.xrefItems[m.xrefCursor]
					m.xrefMode = false
					m.panelView = item.TargetView
					m.updatePanelContent()
				}
				return m, nil
			}
			return m, nil
		}

		// Handle command mode (: key)
		if m.cmdMode {
			switch msg.String() {
			case "esc":
				m.cmdMode = false
				m.cmdInput = ""
				return m, nil
			case "enter":
				if m.cmdInput != "" {
					// Save to history
					m.cmdHistory = append([]string{m.cmdInput}, m.cmdHistory...)
					if len(m.cmdHistory) > 20 {
						m.cmdHistory = m.cmdHistory[:20]
					}
					// Execute command
					cmd := m.cmdInput
					m.cmdMode = false
					m.cmdRunning = true
					m.cmdShowOutput = true
					m.statusMsg = "Running: " + cmd
					return m, runLocalCommand(cmd)
				}
				m.cmdMode = false
				return m, nil
			case "backspace":
				if len(m.cmdInput) > 0 {
					m.cmdInput = m.cmdInput[:len(m.cmdInput)-1]
				}
				return m, nil
			case "up":
				// Navigate history
				if len(m.cmdHistory) > 0 && m.cmdHistoryIdx < len(m.cmdHistory)-1 {
					m.cmdHistoryIdx++
					m.cmdInput = m.cmdHistory[m.cmdHistoryIdx]
				}
				return m, nil
			case "down":
				if m.cmdHistoryIdx > 0 {
					m.cmdHistoryIdx--
					m.cmdInput = m.cmdHistory[m.cmdHistoryIdx]
				} else if m.cmdHistoryIdx == 0 {
					m.cmdHistoryIdx = -1
					m.cmdInput = ""
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.cmdInput += msg.String()
				}
				return m, nil
			}
		}

		// Handle Esc to dismiss command output
		if msg.String() == "esc" && m.cmdShowOutput {
			m.cmdShowOutput = false
			m.cmdOutput = ""
			return m, nil
		}

		// Handle search mode
		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.searchQuery = ""
			case "enter":
				m.searchMode = false
			case "backspace":
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
				}
			default:
				if len(msg.String()) == 1 {
					m.searchQuery += msg.String()
				}
			}
			return m, nil
		}

		// Handle query mode (saved query selector)
		if m.queryMode {
			switch msg.String() {
			case "esc":
				m.queryMode = false
				return m, nil
			case "enter":
				// Apply selected query
				if m.queryCursor >= 0 && m.queryCursor < len(savedQueries) {
					q := savedQueries[m.queryCursor]
					if q.Query == "" {
						// "all" query - clear filter
						m.activeQuery = nil
					} else {
						m.activeQuery = &q
					}
				}
				m.queryMode = false
				m.statusMsg = m.getQueryStatusMsg()
				return m, nil
			case "up", "k":
				if m.queryCursor > 0 {
					m.queryCursor--
				}
				return m, nil
			case "down", "j":
				if m.queryCursor < len(savedQueries)-1 {
					m.queryCursor++
				}
				return m, nil
			case "c":
				// Clear query filter
				m.activeQuery = nil
				m.queryMode = false
				m.statusMsg = ""
				return m, nil
			}
			return m, nil
		}

		// Handle trace mode (trace picker or result view)
		if m.traceMode {
			if m.traceLoading {
				// Waiting for trace to complete, only allow quit
				if msg.String() == "q" || msg.String() == "esc" {
					m.traceMode = false
					m.traceLoading = false
				}
				return m, nil
			}

			// Showing trace result or picker
			if m.traceOutput != "" || m.traceError != nil {
				// Viewing trace result - any key returns to picker or closes
				m.traceOutput = ""
				m.traceError = nil
				// Go back to picker if there are items, else close
				if len(m.traceItems) > 0 {
					return m, nil
				}
				m.traceMode = false
				return m, nil
			}

			// Trace picker mode
			switch msg.String() {
			case "esc", "q":
				m.traceMode = false
				return m, nil
			case "enter":
				// Run trace on selected item
				if m.traceCursor >= 0 && m.traceCursor < len(m.traceItems) {
					item := m.traceItems[m.traceCursor]
					m.traceLoading = true
					return m, m.runTrace(item)
				}
				return m, nil
			case "up", "k":
				if m.traceCursor > 0 {
					m.traceCursor--
				}
				return m, nil
			case "down", "j":
				if m.traceCursor < len(m.traceItems)-1 {
					m.traceCursor++
				}
				return m, nil
			}
			return m, nil
		}

		// Handle scan mode (viewing scan results)
		if m.scanMode {
			if m.scanLoading {
				// Waiting for scan to complete, only allow quit
				if msg.String() == "q" || msg.String() == "esc" {
					m.scanMode = false
					m.scanLoading = false
				}
				return m, nil
			}

			switch msg.String() {
			case "f":
				// Auto-fix: run remedy --dry-run first
				if len(m.scanFindings) > 0 {
					m.scanLoading = true
					return m, m.runRemedyDryRun()
				}
				return m, nil
			case "F":
				// Force fix: run remedy without dry-run (needs confirmation)
				if len(m.scanFindings) > 0 {
					m.scanLoading = true
					return m, m.runRemedyApply()
				}
				return m, nil
			default:
				// Any other key closes scan view
				m.scanMode = false
				m.scanOutput = ""
				m.scanError = nil
				m.scanFindings = nil
				m.scanCategories = nil
				return m, nil
			}
		}

		// Handle panel mode key navigation
		if m.panelMode {
			switch {
			case key.Matches(msg, m.keymap.Quit):
				saveSnapshot(&m)
				return m, tea.Quit

			case msg.String() == "esc":
				// Close panel
				m.panelMode = false
				m.panelFocused = false
				return m, nil

			case key.Matches(msg, m.keymap.Tab):
				// Toggle focus between dashboard and panel
				m.panelFocused = !m.panelFocused
				return m, nil

			case m.panelFocused:
				// Panel scrolling when focused
				switch {
				case key.Matches(msg, m.keymap.Up):
					m.panelPane.LineUp(1)
				case key.Matches(msg, m.keymap.Down):
					m.panelPane.LineDown(1)
				case msg.String() == "g":
					m.panelPane.GotoTop()
				case msg.String() == "G":
					m.panelPane.GotoBottom()
				case msg.String() == "ctrl+d":
					m.panelPane.HalfPageDown()
				case msg.String() == "ctrl+u":
					m.panelPane.HalfPageUp()
				}
				return m, nil

			default:
				// Dashboard navigation when panel not focused
				// Allow switching panels with view keys
				switch {
				case key.Matches(msg, m.keymap.Dashboard):
					// Exit panel mode, return to dashboard
					m.panelMode = false
					m.panelFocused = false
					return m, nil
				case key.Matches(msg, m.keymap.Workloads):
					m.panelView = viewWorkloads
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Pipelines):
					m.panelView = viewPipelines
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Drift):
					m.panelView = viewDrift
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Orphans):
					m.panelView = viewOrphans
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Crashes):
					m.panelView = viewCrashes
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Issues):
					m.panelView = viewIssues
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Bypass):
					m.panelView = viewBypass
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Sprawl):
					m.panelView = viewSprawl
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Suspended):
					m.panelView = viewSuspended
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Apps):
					m.panelView = viewApps
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Dependencies):
					m.panelView = viewDependencies
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.GitSources):
					m.panelView = viewGitSources
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.ClusterData):
					m.panelView = viewClusterData
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.AppHierarchy):
					m.panelView = viewAppHierarchy
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Maps):
					m.panelView = viewMaps
					m.updatePanelContent()
				case key.Matches(msg, m.keymap.Up):
					if m.cursor > 0 {
						m.cursor--
					}
				case key.Matches(msg, m.keymap.Down):
					m.cursor++
				case key.Matches(msg, m.keymap.Enter):
					// Show cross-references for selected item
					m.showCrossReferences()
					return m, nil
				case key.Matches(msg, m.keymap.Hub):
					return m, checkCubAuthForSwitch
				case key.Matches(msg, m.keymap.Help):
					m.helpMode = true
				case key.Matches(msg, m.keymap.Refresh):
					m.loading = true
					m.statusMsg = "Refreshing..."
					return m, loadLocalClusterData
				}
				return m, nil
			}
		}

		// Normal key handling (no panel open)
		switch {
		case key.Matches(msg, m.keymap.Quit):
			saveSnapshot(&m)
			return m, tea.Quit

		case key.Matches(msg, m.keymap.Help):
			m.helpMode = true
			return m, nil

		case key.Matches(msg, m.keymap.Search):
			m.searchMode = true
			m.searchQuery = ""
			return m, nil

		case key.Matches(msg, m.keymap.Refresh):
			m.loading = true
			m.statusMsg = "Refreshing..."
			return m, loadLocalClusterData

		case key.Matches(msg, m.keymap.Hub):
			// Switch to ConfigHub mode - check auth first
			return m, checkCubAuthForSwitch

		case key.Matches(msg, m.keymap.Dashboard):
			m.panelMode = false
			m.panelFocused = false
			return m, nil

		case key.Matches(msg, m.keymap.Workloads):
			m.panelMode = true
			m.panelView = viewWorkloads
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Pipelines):
			m.panelMode = true
			m.panelView = viewPipelines
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Drift):
			m.panelMode = true
			m.panelView = viewDrift
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Orphans):
			m.panelMode = true
			m.panelView = viewOrphans
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Crashes):
			m.panelMode = true
			m.panelView = viewCrashes
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Issues):
			m.panelMode = true
			m.panelView = viewIssues
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Bypass):
			m.panelMode = true
			m.panelView = viewBypass
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Sprawl):
			m.panelMode = true
			m.panelView = viewSprawl
			m.updatePanelContent()

		case key.Matches(msg, m.keymap.Suspended):
			m.panelMode = true
			m.panelView = viewSuspended
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Apps):
			m.panelMode = true
			m.panelView = viewApps
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Dependencies):
			m.panelMode = true
			m.panelView = viewDependencies
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.GitSources):
			m.panelMode = true
			m.panelView = viewGitSources
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.ClusterData):
			m.panelMode = true
			m.panelView = viewClusterData
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.AppHierarchy):
			m.panelMode = true
			m.panelView = viewAppHierarchy
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Maps):
			m.panelMode = true
			m.panelView = viewMaps
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Tab):
			// If no panel, open workloads panel
			m.panelMode = true
			m.panelView = viewWorkloads
			m.updatePanelContent()
			return m, nil

		case key.Matches(msg, m.keymap.Up):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case key.Matches(msg, m.keymap.Down):
			m.cursor++
			return m, nil

		case key.Matches(msg, m.keymap.Query):
			m.queryMode = true
			return m, nil

		case key.Matches(msg, m.keymap.Trace):
			// Open trace picker with available resources
			m.traceMode = true
			m.traceCursor = 0
			m.traceItems = m.buildTraceItems()
			return m, nil

		case key.Matches(msg, m.keymap.Scan):
			// Run scan and show results
			m.scanMode = true
			m.scanLoading = true
			m.scanOutput = ""
			m.scanError = nil
			return m, m.runScan()

		case key.Matches(msg, m.keymap.Import):
			m.switchToImport = true
			saveSnapshot(&m)
			return m, tea.Quit

		case key.Matches(msg, m.keymap.NextNamespace):
			// Cycle to next namespace (wraps around)
			if len(m.namespaces) > 0 {
				m.namespaceIdx = (m.namespaceIdx + 1) % (len(m.namespaces) + 1)
			}
			return m, nil

		case key.Matches(msg, m.keymap.PrevNamespace):
			// Cycle to previous namespace (wraps around)
			if len(m.namespaces) > 0 {
				m.namespaceIdx--
				if m.namespaceIdx < 0 {
					m.namespaceIdx = len(m.namespaces)
				}
			}
			return m, nil

		case key.Matches(msg, m.keymap.Command):
			m.cmdMode = true
			m.cmdInput = ""
			m.cmdHistoryIdx = -1
			return m, nil
		}
	}

	return m, nil
}

func runAuthLogin() tea.Msg {
	cmd := exec.Command("cub", "auth", "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
	return checkCubAuthForSwitch()
}

// showCrossReferences populates xref items based on current view and selected item
func (m *LocalClusterModel) showCrossReferences() {
	m.xrefItems = nil
	m.xrefCursor = 0

	entries := m.getFilteredEntries()

	switch m.panelView {
	case viewWorkloads:
		// From workload → show managing GitOps resource
		if m.cursor >= 0 && m.cursor < len(entries) {
			e := entries[m.cursor]
			m.xrefSourceType = "workload"
			m.xrefSourceName = e.Name

			// Find GitOps resource that manages this workload
			for _, g := range m.gitops {
				// Check if this GitOps resource might manage the workload
				// Flux inventory check would be ideal, but we use naming convention
				if g.Namespace == e.Namespace || g.Name == e.Namespace || strings.Contains(strings.ToLower(g.Name), strings.ToLower(e.Namespace)) {
					m.xrefItems = append(m.xrefItems, xrefItem{
						Type:       "gitops",
						Kind:       g.Kind,
						Name:       g.Name,
						Namespace:  g.Namespace,
						Relation:   "possibly manages",
						TargetView: viewPipelines,
					})
				}
			}

			// Show namespace as xref
			m.xrefItems = append(m.xrefItems, xrefItem{
				Type:       "namespace",
				Kind:       "Namespace",
				Name:       e.Namespace,
				Namespace:  "",
				Relation:   "contains",
				TargetView: viewWorkloads,
			})

			// Show owner info
			if e.Owner != "Native" {
				m.xrefItems = append(m.xrefItems, xrefItem{
					Type:       "owner",
					Kind:       e.Owner,
					Name:       fmt.Sprintf("Owned by %s", e.Owner),
					Namespace:  e.Namespace,
					Relation:   "managed by",
					TargetView: viewPipelines,
				})
			}
		}

	case viewPipelines:
		// From GitOps resource → show managed workloads
		if m.cursor >= 0 && m.cursor < len(m.gitops) {
			g := m.gitops[m.cursor]
			m.xrefSourceType = "gitops"
			m.xrefSourceName = g.Name

			// Find workloads in same namespace
			for _, e := range entries {
				if e.Namespace == g.Namespace || strings.Contains(strings.ToLower(e.Namespace), strings.ToLower(g.Name)) {
					m.xrefItems = append(m.xrefItems, xrefItem{
						Type:       "workload",
						Kind:       e.Kind,
						Name:       e.Name,
						Namespace:  e.Namespace,
						Relation:   "possibly managed",
						TargetView: viewWorkloads,
					})
				}
			}

			// Show dependencies
			for _, dep := range g.DependsOn {
				m.xrefItems = append(m.xrefItems, xrefItem{
					Type:       "dependency",
					Kind:       "DependsOn",
					Name:       dep,
					Namespace:  "",
					Relation:   "depends on",
					TargetView: viewDependencies,
				})
			}
		}

	case viewOrphans:
		// From orphan → suggest adoption
		orphans := []MapEntry{}
		for _, e := range entries {
			if e.Owner == "Native" {
				orphans = append(orphans, e)
			}
		}
		if m.cursor >= 0 && m.cursor < len(orphans) {
			e := orphans[m.cursor]
			m.xrefSourceType = "orphan"
			m.xrefSourceName = e.Name

			// Suggest GitOps resources in same namespace for adoption
			for _, g := range m.gitops {
				if g.Namespace == e.Namespace {
					m.xrefItems = append(m.xrefItems, xrefItem{
						Type:       "adoption",
						Kind:       g.Kind,
						Name:       g.Name,
						Namespace:  g.Namespace,
						Relation:   "could adopt",
						TargetView: viewPipelines,
					})
				}
			}
		}
	}

	if len(m.xrefItems) > 0 {
		m.xrefMode = true
	}
}

// updatePanelContent sets the panel viewport content based on panelView
func (m *LocalClusterModel) updatePanelContent() {
	var content string
	switch m.panelView {
	case viewWorkloads:
		content = m.getPanelWorkloads()
	case viewPipelines:
		content = m.getPanelPipelines()
	case viewDrift:
		content = m.getPanelDrift()
	case viewOrphans:
		content = m.getPanelOrphans()
	case viewCrashes:
		content = m.getPanelCrashes()
	case viewIssues:
		content = m.getPanelIssues()
	case viewBypass:
		content = m.getPanelBypass()
	case viewSprawl:
		content = m.getPanelSprawl()
	case viewSuspended:
		content = m.getPanelSuspended()
	case viewApps:
		content = m.getPanelApps()
	case viewDependencies:
		content = m.getPanelDependencies()
	case viewGitSources:
		content = m.getPanelGitSources()
	case viewClusterData:
		content = m.getPanelClusterData()
	case viewAppHierarchy:
		content = m.getPanelAppHierarchy()
	case viewMaps:
		content = m.getPanelMaps()
	}
	m.panelPane.SetContent(content)
	m.panelPane.GotoTop()
}

// getPanelTitle returns the title for the current panel view
func (m LocalClusterModel) getPanelTitle() string {
	switch m.panelView {
	case viewWorkloads:
		return "WORKLOADS"
	case viewPipelines:
		return "PIPELINES"
	case viewDrift:
		return "DRIFT"
	case viewOrphans:
		return "ORPHANS"
	case viewCrashes:
		return "CRASHES"
	case viewIssues:
		return "ISSUES"
	case viewBypass:
		return "BYPASS"
	case viewSprawl:
		return "SPRAWL"
	case viewSuspended:
		return "SUSPENDED"
	case viewApps:
		return "APPS"
	case viewDependencies:
		return "DEPENDENCIES"
	case viewGitSources:
		return "GIT SOURCES"
	case viewClusterData:
		return "CLUSTER DATA"
	case viewAppHierarchy:
		return "APP HIERARCHY"
	case viewMaps:
		return "MAPS"
	default:
		return "DETAILS"
	}
}

func (m LocalClusterModel) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	// Auth prompt
	if m.authNeeded {
		return m.renderAuthPrompt()
	}

	// Help overlay
	if m.helpMode {
		return m.renderHelp()
	}

	// Query selector mode
	if m.queryMode {
		return m.renderQuerySelector()
	}

	// Trace mode (picker or result)
	if m.traceMode {
		return m.renderTrace()
	}

	// Scan mode (loading or result)
	if m.scanMode {
		return m.renderScan()
	}

	// Cross-reference mode
	if m.xrefMode {
		return m.renderXref()
	}

	// Panel mode: split view with dashboard on left, details on right
	if m.panelMode {
		return m.renderSplitView()
	}

	// Default: dashboard only
	return m.renderDashboard()
}

// Styles
var (
	lcHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	lcSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	lcNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	lcDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
	lcOkStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	lcWarnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	lcErrStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	lcCyanStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	lcPurpleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))

	// Variant color styles (for environment distinction)
	lcVariantProdStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))  // Green for prod
	lcVariantStagingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208")) // Amber for staging
	lcVariantDevStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))  // Blue for dev
	lcVariantCanaryStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("141")) // Purple for canary
	lcVariantOtherStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("246")) // Gray for other

	// Pane styles for split view
	lcLeftPaneStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	lcLeftPaneActiveStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("212")).
				Padding(0, 1)
	lcRightPaneStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
	lcRightPaneActiveStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("212")).
				Padding(0, 1)

	// Mode header styles
	lcModeStandaloneStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("246")).
				Background(lipgloss.Color("236")).
				Padding(0, 1)
	lcModeConnectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82")).
				Background(lipgloss.Color("236")).
				Padding(0, 1)
	lcModeHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("236"))
)

// renderModeHeader returns the mode indicator header shown at the top of all views
// Format: Standalone │ Cluster: prod-east │ Context: eks-prod-east
func (m LocalClusterModel) renderModeHeader() string {
	mode := lcModeStandaloneStyle.Render("Standalone")
	cluster := lcModeHeaderStyle.Render(fmt.Sprintf(" │ Cluster: %s", m.clusterName))
	context := lcModeHeaderStyle.Render(fmt.Sprintf(" │ Context: %s", m.contextName))

	return mode + cluster + context + "\n"
}

// getNamespaceFilter returns the current namespace filter name
// Returns "All" if no filter is active (idx=0), or the namespace name
func (m *LocalClusterModel) getNamespaceFilter() string {
	if m.namespaceIdx == 0 || len(m.namespaces) == 0 {
		return "All"
	}
	if m.namespaceIdx-1 < len(m.namespaces) {
		return m.namespaces[m.namespaceIdx-1]
	}
	return "All"
}

// getFilteredEntries returns entries filtered by namespace and active query
func (m *LocalClusterModel) getFilteredEntries() []MapEntry {
	entries := m.entries

	// Apply namespace filter first
	if m.namespaceIdx > 0 && len(m.namespaces) > 0 {
		ns := m.getNamespaceFilter()
		filtered := make([]MapEntry, 0)
		for _, e := range entries {
			if e.Namespace == ns {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Apply query filter
	if m.activeQuery != nil && m.activeQuery.Query != "" {
		filtered := make([]MapEntry, 0)
		for _, e := range entries {
			if m.matchesQuery(e, m.activeQuery.Query) {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	return entries
}

// renderNamespaceIndicator returns the namespace filter indicator for status bar
func (m *LocalClusterModel) renderNamespaceIndicator() string {
	if len(m.namespaces) == 0 {
		return ""
	}
	ns := m.getNamespaceFilter()
	if ns == "All" {
		return lcDimStyle.Render(fmt.Sprintf("ns: All (%d)", len(m.namespaces)))
	}
	return lcCyanStyle.Render(fmt.Sprintf("ns: %s (%d/%d)", ns, m.namespaceIdx, len(m.namespaces)))
}

// extractVariant extracts the environment variant from a namespace name
// Returns the variant name and its color-styled version
func extractVariant(namespace string) (variant string, styled string) {
	ns := strings.ToLower(namespace)

	// Check for variant patterns
	variantPatterns := map[string]struct {
		suffixes []string
		prefixes []string
		style    lipgloss.Style
	}{
		"prod": {
			suffixes: []string{"-prod", "-production", "-prd"},
			prefixes: []string{"prod-", "production-", "prd-"},
			style:    lcVariantProdStyle,
		},
		"staging": {
			suffixes: []string{"-staging", "-stg", "-stage", "-preprod", "-pre-prod"},
			prefixes: []string{"staging-", "stg-", "stage-", "preprod-", "pre-prod-"},
			style:    lcVariantStagingStyle,
		},
		"dev": {
			suffixes: []string{"-dev", "-development", "-develop"},
			prefixes: []string{"dev-", "development-", "develop-"},
			style:    lcVariantDevStyle,
		},
		"canary": {
			suffixes: []string{"-canary", "-preview", "-experiment"},
			prefixes: []string{"canary-", "preview-", "experiment-"},
			style:    lcVariantCanaryStyle,
		},
		"test": {
			suffixes: []string{"-test", "-testing", "-qa"},
			prefixes: []string{"test-", "testing-", "qa-"},
			style:    lcVariantStagingStyle, // Yellow like staging
		},
	}

	for variantName, patterns := range variantPatterns {
		for _, suffix := range patterns.suffixes {
			if strings.HasSuffix(ns, suffix) {
				return variantName, patterns.style.Render(variantName)
			}
		}
		for _, prefix := range patterns.prefixes {
			if strings.HasPrefix(ns, prefix) {
				return variantName, patterns.style.Render(variantName)
			}
		}
	}

	// Check for exact matches
	exactMatches := map[string]lipgloss.Style{
		"production": lcVariantProdStyle,
		"prod":       lcVariantProdStyle,
		"staging":    lcVariantStagingStyle,
		"stage":      lcVariantStagingStyle,
		"dev":        lcVariantDevStyle,
		"develop":    lcVariantDevStyle,
		"canary":     lcVariantCanaryStyle,
		"test":       lcVariantStagingStyle,
		"qa":         lcVariantStagingStyle,
	}

	if style, ok := exactMatches[ns]; ok {
		return ns, style.Render(ns)
	}

	// Default: unknown variant
	return "", ""
}

// renderUpgradeCTA returns a contextual upgrade call-to-action for standalone mode
func renderUpgradeCTA(viewName string) string {
	ctaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Italic(true)

	var message string
	switch viewName {
	case "map", "workloads":
		message = "Want fleet-wide visibility? Press H to connect to ConfigHub"
	case "scan":
		message = "Want to track findings over time? Press H to connect to ConfigHub"
	case "trace":
		message = "Want ownership across all clusters? Press H to connect to ConfigHub"
	case "query":
		message = "Want to query across your fleet? Press H to connect to ConfigHub"
	case "dashboard":
		message = "Want more? Press H for ConfigHub hierarchy, I to import workloads"
	default:
		message = "Press H to connect to ConfigHub for fleet-wide features"
	}

	return ctaStyle.Render("💡 " + message)
}

func (m LocalClusterModel) renderAuthPrompt() string {
	var b strings.Builder

	b.WriteString(lcHeaderStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("│") + "  " + lcHeaderStyle.Render("🔐 CONFIGHUB AUTHENTICATION REQUIRED") + "                      " + lcHeaderStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	b.WriteString("  To switch to ConfigHub hierarchy mode, you need to authenticate.\n\n")
	b.WriteString("  This will open your browser to log in to ConfigHub.\n\n")
	b.WriteString("  " + lcNameStyle.Render("Press Y to authenticate, N to cancel") + "\n")

	return b.String()
}

func (m LocalClusterModel) renderHelp() string {
	var b strings.Builder

	b.WriteString(lcHeaderStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("│") + "  " + lcHeaderStyle.Render("LOCAL CLUSTER TUI HELP") + strings.Repeat(" ", 39) + lcHeaderStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	b.WriteString(lcSectionStyle.Render("VIEWS"))
	b.WriteString("\n")
	b.WriteString("  " + lcNameStyle.Render("s") + "  Status/Dashboard\n")
	b.WriteString("  " + lcNameStyle.Render("w") + "  Workloads by owner\n")
	b.WriteString("  " + lcNameStyle.Render("a") + "  Apps (grouped by app label + variant)\n")
	b.WriteString("  " + lcNameStyle.Render("p") + "  Pipelines (GitOps deployers)\n")
	b.WriteString("  " + lcNameStyle.Render("d") + "  Drift detection\n")
	b.WriteString("  " + lcNameStyle.Render("o") + "  Orphans (Native resources)\n")
	b.WriteString("  " + lcNameStyle.Render("c") + "  Crashes (failing pods)\n")
	b.WriteString("  " + lcNameStyle.Render("i") + "  Issues (unhealthy resources)\n")
	b.WriteString("  " + lcNameStyle.Render("u") + "  sUspended (paused/forgotten resources)\n")
	b.WriteString("  " + lcNameStyle.Render("b") + "  Bypass (factory bypass detection)\n")
	b.WriteString("  " + lcNameStyle.Render("x") + "  Sprawl (config sprawl analysis)\n")
	b.WriteString("  " + lcNameStyle.Render("D") + "  Dependencies (upstream/downstream)\n")
	b.WriteString("  " + lcNameStyle.Render("G") + "  Git sources (forward trace)\n")
	b.WriteString("  " + lcNameStyle.Render("4") + "  Cluster Data (all data sources TUI reads)\n")
	b.WriteString("  " + lcNameStyle.Render("5/A") + "  App Hierarchy (inferred ConfigHub model)\n")
	b.WriteString("  " + lcNameStyle.Render("M") + "  Three Maps view\n")
	b.WriteString("  " + lcNameStyle.Render("Tab") + "  Cycle views\n")
	b.WriteString("\n")

	b.WriteString(lcSectionStyle.Render("NAVIGATION"))
	b.WriteString("\n")
	b.WriteString("  " + lcNameStyle.Render("↑/k ↓/j") + "  Move up/down\n")
	b.WriteString("  " + lcNameStyle.Render("] / [") + "   Next/prev namespace\n")
	b.WriteString("  " + lcNameStyle.Render("Enter") + "    Cross-references (in panel view)\n")
	b.WriteString("  " + lcNameStyle.Render("/") + "        Search\n")
	b.WriteString("  " + lcNameStyle.Render("r") + "        Refresh data\n")
	b.WriteString("\n")

	b.WriteString(lcSectionStyle.Render("ACTIONS"))
	b.WriteString("\n")
	b.WriteString("  " + lcNameStyle.Render("Q") + "  Saved queries (filter resources)\n")
	b.WriteString("  " + lcNameStyle.Render("T") + "  Trace ownership chain\n")
	b.WriteString("  " + lcNameStyle.Render("S") + "  Scan for CCVEs\n")
	b.WriteString("  " + lcNameStyle.Render("I") + "  Import wizard (bring workloads to ConfigHub)\n")
	b.WriteString("\n")

	b.WriteString(lcSectionStyle.Render("COMMAND PALETTE"))
	b.WriteString("\n")
	b.WriteString("  " + lcNameStyle.Render(":") + "  Run shell command (↑↓ for history)\n")
	b.WriteString("     " + lcDimStyle.Render("e.g., kubectl get pods, cub-scout scan") + "\n")
	b.WriteString("\n")

	b.WriteString(lcSectionStyle.Render("MODE SWITCHING"))
	b.WriteString("\n")
	b.WriteString("  " + lcNameStyle.Render("H") + "  Switch to ConfigHub hierarchy\n")
	b.WriteString("     " + lcDimStyle.Render("(requires cub auth login)") + "\n")
	b.WriteString("\n")

	b.WriteString(lcSectionStyle.Render("QUIT"))
	b.WriteString("\n")
	b.WriteString("  " + lcNameStyle.Render("q") + "  Quit\n")
	b.WriteString("  " + lcNameStyle.Render("?") + "  Show this help\n")
	b.WriteString("\n")

	b.WriteString(lcDimStyle.Render("Press any key to close"))

	return b.String()
}

// renderSplitView renders the split pane view with dashboard on left and details on right
func (m LocalClusterModel) renderSplitView() string {
	var b strings.Builder

	// Mode header (always shown)
	b.WriteString(m.renderModeHeader())

	// Calculate pane dimensions for 50/50 split
	leftWidth := (m.width / 2) - 2
	rightWidth := m.width - leftWidth - 4
	contentHeight := m.height - 8 // Reserve space for header/footer

	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}
	if contentHeight < 10 {
		contentHeight = 10
	}

	// Update panel viewport size
	m.panelPane.Width = rightWidth - 4
	m.panelPane.Height = contentHeight - 2

	// Left pane: Summary dashboard
	leftContent := m.renderDashboardCompact()
	leftPaneStyled := lcLeftPaneStyle
	if !m.panelFocused {
		leftPaneStyled = lcLeftPaneActiveStyle
	}
	leftPane := leftPaneStyled.
		Width(leftWidth).
		Height(contentHeight).
		Render(leftContent)

	// Right pane: Details based on panelView
	panelTitle := m.getPanelTitle()
	rightHeader := lcSectionStyle.Render(panelTitle)
	if m.panelFocused {
		rightHeader += " " + lcDimStyle.Render("(scroll: j/k)")
	}
	rightContent := rightHeader + "\n\n" + m.panelPane.View()

	rightPaneStyled := lcRightPaneStyle
	if m.panelFocused {
		rightPaneStyled = lcRightPaneActiveStyle
	}
	rightPane := rightPaneStyled.
		Width(rightWidth).
		Height(contentHeight).
		Render(rightContent)

	// Join panes horizontally
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane))
	b.WriteString("\n")

	// Footer with namespace indicator
	nsIndicator := m.renderNamespaceIndicator()
	if nsIndicator != "" {
		b.WriteString(nsIndicator + " " + lcDimStyle.Render("|") + " ")
	}
	b.WriteString(lcDimStyle.Render("[]/[]ns [w]ork [p]ipe [d]rift [o]rph | Tab:focus Esc:close [H]ub [?] [q]"))
	b.WriteString("\n")

	return b.String()
}

// renderDashboardCompact renders a compact version of the dashboard for the left pane
func (m LocalClusterModel) renderDashboardCompact() string {
	var b strings.Builder

	if m.loading {
		b.WriteString(m.spinner.View() + " Loading...\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(lcErrStyle.Render("Error: " + m.err.Error()) + "\n")
		return b.String()
	}

	// Count by owner
	byOwner := map[string]int{}
	for _, e := range m.entries {
		byOwner[e.Owner]++
	}

	total := len(m.entries)
	b.WriteString(lcSectionStyle.Render("SUMMARY") + "\n")
	b.WriteString(fmt.Sprintf("Total: %d workloads\n\n", total))

	if byOwner["Flux"] > 0 {
		b.WriteString(fmt.Sprintf("%s Flux: %d\n", lcCyanStyle.Render("●"), byOwner["Flux"]))
	}
	if byOwner["ArgoCD"] > 0 {
		b.WriteString(fmt.Sprintf("%s ArgoCD: %d\n", lcPurpleStyle.Render("●"), byOwner["ArgoCD"]))
	}
	if byOwner["Helm"] > 0 {
		b.WriteString(fmt.Sprintf("%s Helm: %d\n", lcWarnStyle.Render("●"), byOwner["Helm"]))
	}
	if byOwner["Native"] > 0 {
		b.WriteString(fmt.Sprintf("%s Native: %d\n", lcDimStyle.Render("●"), byOwner["Native"]))
	}
	b.WriteString("\n")

	// GitOps deployers summary
	b.WriteString(lcSectionStyle.Render("DEPLOYERS") + "\n")
	if len(m.gitops) == 0 {
		b.WriteString(lcDimStyle.Render("None found") + "\n")
	} else {
		for i, g := range m.gitops {
			if i >= 5 {
				b.WriteString(fmt.Sprintf("... +%d more\n", len(m.gitops)-5))
				break
			}
			statusIcon := lcOkStyle.Render("✓")
			if g.Status != "Ready" && g.Status != "Healthy" {
				statusIcon = lcWarnStyle.Render("⚠")
			}
			b.WriteString(fmt.Sprintf("%s %s\n", statusIcon, g.Name))
		}
	}

	return b.String()
}

// Panel content methods - these return content for the right panel without footers
func (m LocalClusterModel) getPanelWorkloads() string {
	var b strings.Builder

	// Use filtered entries if query is active
	entries := m.getFilteredEntries()

	// Show filter status if active
	if m.activeQuery != nil && m.activeQuery.Query != "" {
		b.WriteString(lcCyanStyle.Render(fmt.Sprintf("Filter: %s (%d/%d)", m.activeQuery.Name, len(entries), len(m.entries))))
		b.WriteString("\n\n")
	}

	// Group by owner
	byOwner := map[string][]MapEntry{}
	for _, e := range entries {
		byOwner[e.Owner] = append(byOwner[e.Owner], e)
	}

	owners := []string{"Flux", "ArgoCD", "Helm", "ConfigHub", "Native"}
	for _, owner := range owners {
		entries := byOwner[owner]
		if len(entries) == 0 {
			continue
		}

		ownerStyle := lcDimStyle
		switch owner {
		case "Flux":
			ownerStyle = lcCyanStyle
		case "ArgoCD":
			ownerStyle = lcPurpleStyle
		case "Helm":
			ownerStyle = lcWarnStyle
		}

		b.WriteString(ownerStyle.Render(fmt.Sprintf("%s (%d)", owner, len(entries))))
		b.WriteString("\n")

		// Sort by namespace/name
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Namespace != entries[j].Namespace {
				return entries[i].Namespace < entries[j].Namespace
			}
			return entries[i].Name < entries[j].Name
		})

		for _, e := range entries {
			// Build owner ref string (e.g., "kustomization/apps")
			ownerRef := ""
			if e.OwnerDetails != nil && e.OwnerDetails["name"] != "" {
				ownerRef = lcDimStyle.Render(" → " + e.OwnerDetails["name"])
			}

			b.WriteString(fmt.Sprintf("  └── %s/%s %s%s\n",
				e.Namespace,
				lcNameStyle.Render(e.Name),
				lcDimStyle.Render(e.Kind),
				ownerRef))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m LocalClusterModel) getPanelPipelines() string {
	var b strings.Builder

	if len(m.gitops) == 0 {
		b.WriteString(lcDimStyle.Render("No GitOps pipelines found") + "\n\n")
		b.WriteString(lcDimStyle.Render("Install Flux or ArgoCD to see pipelines") + "\n")
		return b.String()
	}

	// Group by type
	kustomizations := []GitOpsResource{}
	helmReleases := []GitOpsResource{}
	argoApps := []GitOpsResource{}

	for _, g := range m.gitops {
		switch g.Kind {
		case "Kustomization":
			kustomizations = append(kustomizations, g)
		case "HelmRelease":
			helmReleases = append(helmReleases, g)
		case "Application":
			argoApps = append(argoApps, g)
		}
	}

	// Summary
	healthy := 0
	for _, g := range m.gitops {
		if g.Status == "Ready" || g.Status == "Healthy" {
			healthy++
		}
	}
	b.WriteString(fmt.Sprintf("%d/%d pipelines healthy\n\n", healthy, len(m.gitops)))

	// Flux Kustomizations
	if len(kustomizations) > 0 {
		b.WriteString(lcCyanStyle.Render("FLUX KUSTOMIZATIONS") + "\n")
		for _, g := range kustomizations {
			renderPipelineFlow(&b, g)
		}
		b.WriteString("\n")
	}

	// Flux HelmReleases
	if len(helmReleases) > 0 {
		b.WriteString(lcWarnStyle.Render("FLUX HELM RELEASES") + "\n")
		for _, g := range helmReleases {
			renderPipelineFlow(&b, g)
		}
		b.WriteString("\n")
	}

	// ArgoCD Applications
	if len(argoApps) > 0 {
		b.WriteString(lcPurpleStyle.Render("ARGOCD APPLICATIONS") + "\n")
		for _, g := range argoApps {
			renderPipelineFlow(&b, g)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderPipelineFlow renders a visual pipeline flow for a GitOps resource
func renderPipelineFlow(b *strings.Builder, g GitOpsResource) {
	statusIcon := lcOkStyle.Render("✓")
	statusStyle := lcOkStyle
	if g.Status != "Ready" && g.Status != "Healthy" {
		statusIcon = lcWarnStyle.Render("⚠")
		statusStyle = lcWarnStyle
	}

	// Visual flow: Source ──▶ Deployer ──▶ Resources
	source := g.Source
	if source == "" {
		source = "unknown"
	}

	// Truncate source if too long
	if len(source) > 20 {
		source = source[:17] + "..."
	}

	resourceCount := ""
	if g.InventoryCount > 0 {
		resourceCount = fmt.Sprintf(" ──▶ %d resources", g.InventoryCount)
	}

	b.WriteString(fmt.Sprintf("%s %s ──▶ %s%s\n",
		statusIcon,
		lcDimStyle.Render(source),
		lcNameStyle.Render(g.Name),
		resourceCount))

	// Show path if available
	if g.Path != "" {
		b.WriteString(fmt.Sprintf("  %s\n", lcDimStyle.Render("path: "+g.Path)))
	}

	// Show status if not healthy
	if g.Status != "Ready" && g.Status != "Healthy" && g.Status != "" {
		b.WriteString(fmt.Sprintf("  %s\n", statusStyle.Render("status: "+g.Status)))
	}
}

func (m LocalClusterModel) getPanelDrift() string {
	var b strings.Builder

	b.WriteString(lcDimStyle.Render("Compares desired (Git) vs actual (cluster)") + "\n\n")

	// Separate by type for actionable grouping
	var fluxDrifted []GitOpsResource
	var argoDrifted []GitOpsResource
	var otherDrifted []GitOpsResource

	for _, g := range m.gitops {
		if g.Status != "Ready" && g.Status != "Healthy" && g.Status != "True" {
			switch g.Kind {
			case "Kustomization", "HelmRelease", "GitRepository":
				fluxDrifted = append(fluxDrifted, g)
			case "Application":
				argoDrifted = append(argoDrifted, g)
			default:
				otherDrifted = append(otherDrifted, g)
			}
		}
	}

	totalDrifted := len(fluxDrifted) + len(argoDrifted) + len(otherDrifted)

	if totalDrifted == 0 {
		b.WriteString(lcOkStyle.Render("✓ No drift detected") + "\n")
		b.WriteString(lcDimStyle.Render("All GitOps resources in sync") + "\n")
		return b.String()
	}

	// Flux drifted
	if len(fluxDrifted) > 0 {
		b.WriteString(lcSectionStyle.Render("FLUX") + lcDimStyle.Render(fmt.Sprintf(" (%d)", len(fluxDrifted))) + "\n")
		for _, g := range fluxDrifted {
			statusStyle := lcWarnStyle
			if g.Status == "Failed" || g.Status == "False" {
				statusStyle = lcErrStyle
			}
			b.WriteString(fmt.Sprintf("  %s %s/%s\n",
				statusStyle.Render("⚠"),
				g.Namespace,
				lcNameStyle.Render(g.Name)))
			b.WriteString(fmt.Sprintf("     Status: %s\n", statusStyle.Render(g.Status)))
			if g.Source != "" {
				b.WriteString(fmt.Sprintf("     Source: %s\n", lcDimStyle.Render(g.Source)))
			}
		}
		b.WriteString(lcDimStyle.Render("  → flux reconcile kustomization <name> -n <ns>") + "\n\n")
	}

	// Argo drifted
	if len(argoDrifted) > 0 {
		b.WriteString(lcSectionStyle.Render("ARGOCD") + lcDimStyle.Render(fmt.Sprintf(" (%d)", len(argoDrifted))) + "\n")
		for _, g := range argoDrifted {
			statusStyle := lcWarnStyle
			if g.Status == "Degraded" || g.Status == "Unknown" {
				statusStyle = lcErrStyle
			}
			b.WriteString(fmt.Sprintf("  %s %s/%s\n",
				statusStyle.Render("⚠"),
				g.Namespace,
				lcNameStyle.Render(g.Name)))
			b.WriteString(fmt.Sprintf("     Status: %s\n", statusStyle.Render(g.Status)))
			if g.Source != "" {
				b.WriteString(fmt.Sprintf("     Source: %s\n", lcDimStyle.Render(g.Source)))
			}
		}
		b.WriteString(lcDimStyle.Render("  → argocd app sync <name>") + "\n\n")
	}

	// Other
	if len(otherDrifted) > 0 {
		b.WriteString(lcSectionStyle.Render("OTHER") + lcDimStyle.Render(fmt.Sprintf(" (%d)", len(otherDrifted))) + "\n")
		for _, g := range otherDrifted {
			b.WriteString(fmt.Sprintf("  %s %s/%s: %s\n",
				lcWarnStyle.Render("⚠"),
				g.Namespace,
				lcNameStyle.Render(g.Name),
				g.Status))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m LocalClusterModel) getPanelOrphans() string {
	var b strings.Builder

	b.WriteString(lcDimStyle.Render("Resources not managed by GitOps") + "\n\n")

	orphans := []MapEntry{}
	for _, e := range m.entries {
		if e.Owner == "Native" {
			orphans = append(orphans, e)
		}
	}

	if len(orphans) == 0 {
		b.WriteString(lcOkStyle.Render("✓ No orphaned resources") + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Found %d orphans:\n\n", len(orphans)))

	// Group by namespace
	byNS := map[string][]MapEntry{}
	for _, e := range orphans {
		byNS[e.Namespace] = append(byNS[e.Namespace], e)
	}

	for ns, entries := range byNS {
		b.WriteString(fmt.Sprintf("%s/\n", ns))
		for _, e := range entries {
			b.WriteString(fmt.Sprintf("  └── %s %s\n",
				lcDimStyle.Render(e.Kind),
				lcNameStyle.Render(e.Name)))
		}
	}

	return b.String()
}

func (m LocalClusterModel) getPanelCrashes() string {
	var b strings.Builder

	b.WriteString(lcDimStyle.Render("CrashLoopBackOff or Failed state") + "\n\n")

	crashes := []MapEntry{}
	for _, e := range m.entries {
		if e.Status == "Failed" || e.Status == "CrashLoopBackOff" || e.Status == "Error" {
			crashes = append(crashes, e)
		}
	}

	if len(crashes) == 0 {
		b.WriteString(lcOkStyle.Render("✓ No crashing resources") + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Found %d crashing:\n\n", len(crashes)))
	for _, e := range crashes {
		b.WriteString(fmt.Sprintf("%s %s/%s %s\n",
			lcErrStyle.Render("✗"),
			e.Namespace,
			lcNameStyle.Render(e.Name),
			lcDimStyle.Render(e.Status)))
	}

	return b.String()
}

func (m LocalClusterModel) getPanelIssues() string {
	var b strings.Builder

	b.WriteString(lcDimStyle.Render("Resources not Ready or with warnings") + "\n\n")

	issues := []MapEntry{}
	for _, e := range m.entries {
		if e.Status != "Ready" && e.Status != "Running" && e.Status != "" {
			issues = append(issues, e)
		}
	}

	for _, g := range m.gitops {
		if g.Status != "Ready" && g.Status != "Healthy" {
			issues = append(issues, MapEntry{
				Kind:      g.Kind,
				Name:      g.Name,
				Namespace: g.Namespace,
				Status:    g.Status,
				Owner:     "GitOps",
			})
		}
	}

	if len(issues) == 0 {
		b.WriteString(lcOkStyle.Render("✓ No issues found") + "\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Found %d issues:\n\n", len(issues)))
	for _, e := range issues {
		icon := lcWarnStyle.Render("⚠")
		if e.Status == "Failed" || e.Status == "Error" {
			icon = lcErrStyle.Render("✗")
		}
		b.WriteString(fmt.Sprintf("%s %s %s/%s: %s\n",
			icon,
			lcDimStyle.Render(e.Kind),
			e.Namespace,
			lcNameStyle.Render(e.Name),
			e.Status))
	}

	return b.String()
}

func (m LocalClusterModel) getPanelBypass() string {
	var b strings.Builder

	b.WriteString(lcDimStyle.Render("Resources modified outside GitOps") + "\n\n")

	// Count GitOps-managed resources
	byOwner := map[string]int{}
	for _, e := range m.entries {
		if e.Owner == "Flux" || e.Owner == "ArgoCD" || e.Owner == "Helm" {
			byOwner[e.Owner]++
		}
	}

	b.WriteString(lcSectionStyle.Render("Monitored for bypass:") + "\n\n")

	if len(byOwner) == 0 {
		b.WriteString(lcDimStyle.Render("No GitOps-managed resources") + "\n")
		return b.String()
	}

	for owner, count := range byOwner {
		ownerStyle := lcCyanStyle
		if owner == "ArgoCD" {
			ownerStyle = lcPurpleStyle
		} else if owner == "Helm" {
			ownerStyle = lcWarnStyle
		}
		b.WriteString(fmt.Sprintf("%s %s: %d resources\n",
			lcOkStyle.Render("✓"),
			ownerStyle.Render(owner),
			count))
	}

	b.WriteString("\n")
	b.WriteString(lcDimStyle.Render("Use CLI for detailed drift analysis") + "\n")

	return b.String()
}

func (m LocalClusterModel) getPanelSprawl() string {
	var b strings.Builder

	b.WriteString(lcDimStyle.Render("Configuration sources and complexity") + "\n\n")

	// Count by owner
	byOwner := map[string]int{}
	for _, e := range m.entries {
		byOwner[e.Owner]++
	}

	// Count namespaces
	namespaces := map[string]bool{}
	for _, e := range m.entries {
		namespaces[e.Namespace] = true
	}

	// Count sources
	sources := map[string]bool{}
	for _, g := range m.gitops {
		if g.Source != "" {
			sources[g.Source] = true
		}
	}

	total := len(m.entries)
	gitopsManaged := byOwner["Flux"] + byOwner["ArgoCD"] + byOwner["Helm"]
	native := byOwner["Native"]

	b.WriteString(lcSectionStyle.Render("METRICS") + "\n")
	b.WriteString(fmt.Sprintf("Workloads: %d\n", total))
	b.WriteString(fmt.Sprintf("Namespaces: %d\n", len(namespaces)))
	b.WriteString(fmt.Sprintf("Git sources: %d\n", len(sources)))
	b.WriteString(fmt.Sprintf("Deployers: %d\n\n", len(m.gitops)))

	b.WriteString(lcSectionStyle.Render("OWNERSHIP") + "\n")
	if total > 0 {
		gitopsPct := float64(gitopsManaged) / float64(total) * 100
		nativePct := float64(native) / float64(total) * 100

		b.WriteString(fmt.Sprintf("GitOps: %d (%.0f%%)\n", gitopsManaged, gitopsPct))
		b.WriteString(fmt.Sprintf("Native: %d (%.0f%%)\n", native, nativePct))

		if gitopsPct >= 80 {
			b.WriteString("\n" + lcOkStyle.Render("✓ Good coverage") + "\n")
		} else if gitopsPct >= 50 {
			b.WriteString("\n" + lcWarnStyle.Render("⚠ Fair coverage") + "\n")
		} else {
			b.WriteString("\n" + lcErrStyle.Render("✗ High sprawl") + "\n")
		}
	}

	return b.String()
}

func (m LocalClusterModel) getPanelSuspended() string {
	var b strings.Builder

	b.WriteString(lcDimStyle.Render("Paused/suspended GitOps resources") + "\n\n")

	// Look for suspended GitOps resources
	// Flux: check for suspend: true in kustomizations/helmreleases
	// ArgoCD: check for operation.sync.suspended
	suspended := []GitOpsResource{}
	for _, g := range m.gitops {
		// Check if status indicates suspension
		if strings.Contains(strings.ToLower(g.Status), "suspend") {
			suspended = append(suspended, g)
		}
	}

	// Also check for resources that haven't been applied recently (stale)
	stale := []GitOpsResource{}
	now := time.Now()
	for _, g := range m.gitops {
		if !g.LastApplied.IsZero() && now.Sub(g.LastApplied) > 7*24*time.Hour {
			// Resource hasn't synced in over a week - might be forgotten
			stale = append(stale, g)
		}
	}

	b.WriteString(lcSectionStyle.Render("SUSPENDED") + "\n")
	if len(suspended) == 0 {
		b.WriteString(lcOkStyle.Render("✓ No suspended resources") + "\n\n")
	} else {
		b.WriteString(fmt.Sprintf("Found %d suspended resources:\n\n", len(suspended)))
		for _, s := range suspended {
			b.WriteString(fmt.Sprintf("  %s %s/%s\n",
				lcWarnStyle.Render("⏸"),
				s.Namespace,
				lcNameStyle.Render(s.Name)))
			b.WriteString(fmt.Sprintf("    Kind: %s\n", s.Kind))
			if s.Status != "" {
				b.WriteString(fmt.Sprintf("    Status: %s\n", s.Status))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(lcSectionStyle.Render("STALE (>7 days)") + "\n")
	if len(stale) == 0 {
		b.WriteString(lcOkStyle.Render("✓ All resources recently synced") + "\n\n")
	} else {
		b.WriteString(fmt.Sprintf("Found %d stale resources:\n\n", len(stale)))
		for _, s := range stale {
			age := now.Sub(s.LastApplied).Round(time.Hour * 24)
			b.WriteString(fmt.Sprintf("  %s %s/%s\n",
				lcWarnStyle.Render("⚠"),
				s.Namespace,
				lcNameStyle.Render(s.Name)))
			b.WriteString(fmt.Sprintf("    Last sync: %s ago\n", age))
		}
		b.WriteString("\n")
	}

	b.WriteString(lcDimStyle.Render("TIP: Use 'flux resume' or 'argocd app sync' to resume"))

	return b.String()
}

func (m LocalClusterModel) getPanelApps() string {
	var b strings.Builder

	b.WriteString(lcDimStyle.Render("Applications grouped by app label") + "\n\n")

	// Group entries by app label
	// Look for common app label patterns: app, app.kubernetes.io/name, app.kubernetes.io/instance
	appGroups := make(map[string][]MapEntry)
	unknownApps := []MapEntry{}

	for _, e := range m.entries {
		appName := ""

		// Try to find app name from labels
		if e.Labels != nil {
			// Priority order for app label detection
			labelKeys := []string{
				"app",
				"app.kubernetes.io/name",
				"app.kubernetes.io/instance",
				"app.kubernetes.io/part-of",
			}
			for _, key := range labelKeys {
				if val, ok := e.Labels[key]; ok && val != "" {
					appName = val
					break
				}
			}
		}

		// Fallback: extract from namespace pattern (e.g., "myapp-prod" -> "myapp")
		if appName == "" {
			appName = extractAppFromNamespace(e.Namespace)
		}

		if appName != "" {
			appGroups[appName] = append(appGroups[appName], e)
		} else {
			unknownApps = append(unknownApps, e)
		}
	}

	// Sort app names
	appNames := make([]string, 0, len(appGroups))
	for name := range appGroups {
		appNames = append(appNames, name)
	}
	sort.Strings(appNames)

	if len(appNames) == 0 {
		b.WriteString(lcDimStyle.Render("No apps detected. Resources may not have app labels.") + "\n\n")
		b.WriteString(lcDimStyle.Render("TIP: Add 'app' label to workloads for grouping.") + "\n")
		return b.String()
	}

	b.WriteString(lcSectionStyle.Render(fmt.Sprintf("APPS (%d)", len(appNames))) + "\n\n")

	for _, appName := range appNames {
		entries := appGroups[appName]

		// Group by variant (extracted from namespace)
		variants := make(map[string][]MapEntry)
		for _, e := range entries {
			variant, _ := extractVariant(e.Namespace)
			if variant == "" {
				variant = "default"
			}
			variants[variant] = append(variants[variant], e)
		}

		// Render app with variants
		b.WriteString(lcNameStyle.Render(appName) + "\n")

		// Sort variants for consistent display: prod first, then staging, dev, others
		variantOrder := []string{"prod", "production", "staging", "stage", "dev", "development", "test", "qa", "canary", "default"}
		seenVariants := make(map[string]bool)

		for _, v := range variantOrder {
			if entries, ok := variants[v]; ok {
				seenVariants[v] = true
				renderAppVariant(&b, v, entries)
			}
		}

		// Render any remaining variants not in the priority list
		for variant, entries := range variants {
			if !seenVariants[variant] {
				renderAppVariant(&b, variant, entries)
			}
		}

		b.WriteString("\n")
	}

	// Show unknown apps count
	if len(unknownApps) > 0 {
		b.WriteString(lcDimStyle.Render(fmt.Sprintf("+ %d resources without app label", len(unknownApps))) + "\n")
	}

	return b.String()
}

// renderAppVariant renders a variant line for an app
func renderAppVariant(b *strings.Builder, variant string, entries []MapEntry) {
	_, styledVariant := extractVariant(variant)
	if styledVariant == "" {
		styledVariant = lcVariantOtherStyle.Render(variant)
	}

	// Count healthy/total
	healthy := 0
	for _, e := range entries {
		if e.Status == "Ready" || e.Status == "Running" || e.Status == "" {
			healthy++
		}
	}

	status := lcOkStyle.Render("healthy")
	if healthy < len(entries) {
		if healthy == 0 {
			status = lcErrStyle.Render("failing")
		} else {
			status = lcWarnStyle.Render(fmt.Sprintf("%d/%d", healthy, len(entries)))
		}
	}

	// Show owner if consistent across all entries
	owner := ""
	if len(entries) > 0 {
		firstOwner := entries[0].Owner
		consistent := true
		for _, e := range entries[1:] {
			if e.Owner != firstOwner {
				consistent = false
				break
			}
		}
		if consistent && firstOwner != "" && firstOwner != "Native" {
			owner = " " + lcDimStyle.Render("["+firstOwner+"]")
		}
	}

	b.WriteString(fmt.Sprintf("  ├── %s → %s%s\n", styledVariant, status, owner))
}

// extractAppFromNamespace tries to extract an app name from a namespace
// e.g., "myapp-prod" -> "myapp", "payment-service-staging" -> "payment-service"
func extractAppFromNamespace(namespace string) string {
	ns := strings.ToLower(namespace)

	// Skip system namespaces
	if ns == "default" || ns == "kube-system" || ns == "kube-public" ||
		ns == "flux-system" || ns == "argocd" || ns == "cert-manager" {
		return ""
	}

	// Remove common suffixes
	suffixes := []string{
		"-prod", "-production", "-prd",
		"-staging", "-stg", "-stage",
		"-dev", "-development",
		"-test", "-testing", "-qa",
		"-canary", "-preview",
	}

	for _, suffix := range suffixes {
		if strings.HasSuffix(ns, suffix) {
			return strings.TrimSuffix(ns, suffix)
		}
	}

	// If no suffix found, return the namespace as-is (might be the app name)
	return namespace
}

func (m LocalClusterModel) getPanelDependencies() string {
	var b strings.Builder

	b.WriteString(lcDimStyle.Render("Upstream/downstream dependencies between GitOps resources") + "\n\n")

	if len(m.gitops) == 0 {
		b.WriteString(lcDimStyle.Render("No GitOps resources detected.") + "\n\n")
		b.WriteString(lcDimStyle.Render("Deploy Flux or ArgoCD resources to see dependencies.") + "\n")
		return b.String()
	}

	// Build reverse dependency map (what depends on each resource)
	// Key: "namespace/name", Value: list of resources that depend on it
	dependedOnBy := make(map[string][]string)
	for _, g := range m.gitops {
		key := g.Namespace + "/" + g.Name
		for _, dep := range g.DependsOn {
			dependedOnBy[dep] = append(dependedOnBy[dep], key)
		}
	}

	// Count resources with dependencies
	withDeps := 0
	withDownstream := 0
	for _, g := range m.gitops {
		if len(g.DependsOn) > 0 {
			withDeps++
		}
		key := g.Namespace + "/" + g.Name
		if len(dependedOnBy[key]) > 0 {
			withDownstream++
		}
	}

	// Summary
	b.WriteString(lcSectionStyle.Render("SUMMARY") + "\n")
	b.WriteString(fmt.Sprintf("  Total GitOps resources: %d\n", len(m.gitops)))
	b.WriteString(fmt.Sprintf("  With upstream deps:     %d\n", withDeps))
	b.WriteString(fmt.Sprintf("  With downstream deps:   %d\n", withDownstream))
	b.WriteString("\n")

	// Show resources with dependencies
	b.WriteString(lcSectionStyle.Render("DEPENDENCY GRAPH") + "\n\n")

	// Sort gitops by namespace/name for consistent display
	type gitopsItem struct {
		g   GitOpsResource
		key string
	}
	items := make([]gitopsItem, 0, len(m.gitops))
	for _, g := range m.gitops {
		items = append(items, gitopsItem{g: g, key: g.Namespace + "/" + g.Name})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].key < items[j].key
	})

	for _, item := range items {
		g := item.g
		key := item.key
		hasUpstream := len(g.DependsOn) > 0
		hasDownstream := len(dependedOnBy[key]) > 0

		if !hasUpstream && !hasDownstream {
			continue // Skip resources with no dependencies
		}

		// Resource header
		kindStyle := lcCyanStyle // Flux
		if g.Kind == "Application" {
			kindStyle = lcPurpleStyle // ArgoCD
		} else if g.Kind == "HelmRelease" {
			kindStyle = lcWarnStyle // Helm
		}
		b.WriteString(kindStyle.Render(g.Kind) + " " + lcNameStyle.Render(g.Name))
		b.WriteString(lcDimStyle.Render(" (" + g.Namespace + ")") + "\n")

		// Show upstream dependencies (what this depends on)
		if hasUpstream {
			b.WriteString("  " + lcDimStyle.Render("↑ depends on:") + "\n")
			for _, dep := range g.DependsOn {
				// Check if dependency exists
				exists := false
				for _, other := range m.gitops {
					if other.Namespace+"/"+other.Name == dep {
						exists = true
						break
					}
				}
				if exists {
					b.WriteString("    " + lcOkStyle.Render("→") + " " + dep + "\n")
				} else {
					b.WriteString("    " + lcWarnStyle.Render("→") + " " + dep + lcDimStyle.Render(" (missing)") + "\n")
				}
			}
		}

		// Show downstream dependencies (what depends on this)
		if hasDownstream {
			b.WriteString("  " + lcDimStyle.Render("↓ depended on by:") + "\n")
			for _, downstream := range dependedOnBy[key] {
				b.WriteString("    " + lcOkStyle.Render("←") + " " + downstream + "\n")
			}
		}

		b.WriteString("\n")
	}

	// Show resources without any dependencies
	orphanCount := 0
	for _, g := range m.gitops {
		key := g.Namespace + "/" + g.Name
		if len(g.DependsOn) == 0 && len(dependedOnBy[key]) == 0 {
			orphanCount++
		}
	}
	if orphanCount > 0 {
		b.WriteString(lcDimStyle.Render(fmt.Sprintf("+ %d resources without explicit dependencies", orphanCount)) + "\n")
	}

	// Tips
	b.WriteString("\n" + lcDimStyle.Render("TIP: Use Flux/ArgoCD 'dependsOn' field to declare dependencies") + "\n")

	return b.String()
}

func (m LocalClusterModel) getPanelGitSources() string {
	var b strings.Builder

	b.WriteString(lcSectionStyle.Render("GIT SOURCES → DEPLOYERS → RESOURCES") + "\n")
	b.WriteString(lcDimStyle.Render("Forward trace: What does your Git define?") + "\n\n")

	if len(m.gitSources) == 0 {
		b.WriteString(lcDimStyle.Render("No Flux sources detected.") + "\n\n")
		b.WriteString(lcDimStyle.Render("Deploy Flux GitRepository/OCIRepository/HelmRepository CRDs") + "\n")
		b.WriteString(lcDimStyle.Render("to see the DRY → LIVE forward trace.") + "\n\n")
		b.WriteString(lcDimStyle.Render("────────────────────────────────────────────────────────") + "\n\n")
		b.WriteString(lcDimStyle.Render("💡 See full Git structure, DRY→WET causality,") + "\n")
		b.WriteString(lcDimStyle.Render("   and fleet-wide visibility with ConfigHub") + "\n")
		b.WriteString(lcDimStyle.Render("   Press H to connect to ConfigHub") + "\n")
		return b.String()
	}

	// Group by kind
	var gitRepos, ociRepos, helmRepos []GitSourceInfo
	for _, src := range m.gitSources {
		switch src.Kind {
		case "GitRepository":
			gitRepos = append(gitRepos, src)
		case "OCIRepository":
			ociRepos = append(ociRepos, src)
		case "HelmRepository":
			helmRepos = append(helmRepos, src)
		}
	}

	// Build deployer → inventory map
	deployerInventory := make(map[string]int)
	for _, g := range m.gitops {
		if g.InventoryCount > 0 {
			key := g.Kind + "/" + g.Name
			deployerInventory[key] = g.InventoryCount
		}
	}

	// Summary
	b.WriteString(lcDimStyle.Render("────────────────────────────────────────────────────────") + "\n")
	totalSources := len(m.gitSources)
	totalDeployers := len(m.gitops)
	b.WriteString(fmt.Sprintf("Sources: %d │ Deployers: %d │ Workloads: %d\n\n",
		totalSources, totalDeployers, len(m.entries)))

	// GitRepositories (most important)
	if len(gitRepos) > 0 {
		b.WriteString(lcCyanStyle.Render("GIT REPOSITORIES") + "\n")
		for _, src := range gitRepos {
			statusIcon := lcOkStyle.Render("✓")
			if src.Status != "Ready" {
				statusIcon = lcWarnStyle.Render("⚠")
			}

			// URL (truncate if long)
			url := src.URL
			if len(url) > 45 {
				url = url[:42] + "..."
			}

			// Branch/tag info
			ref := src.Branch
			if ref == "" {
				ref = src.Tag
			}
			if ref == "" {
				ref = "default"
			}

			// Revision (short)
			rev := src.Revision
			if len(rev) > 12 {
				rev = rev[:12]
			}

			b.WriteString(fmt.Sprintf("%s %s\n", statusIcon, lcNameStyle.Render(src.Name)))
			b.WriteString(fmt.Sprintf("  %s @ %s", lcDimStyle.Render(url), lcDimStyle.Render(ref)))
			if rev != "" {
				b.WriteString(fmt.Sprintf(" (%s)", lcDimStyle.Render(rev)))
			}
			b.WriteString("\n")

			// Show deployers that reference this source
			if len(src.Deployers) > 0 {
				for i, deployer := range src.Deployers {
					prefix := "├─▶"
					if i == len(src.Deployers)-1 {
						prefix = "└─▶"
					}
					invCount := deployerInventory[deployer]
					invInfo := ""
					if invCount > 0 {
						invInfo = fmt.Sprintf(" → %d resources", invCount)
					}
					b.WriteString(fmt.Sprintf("  %s %s%s\n", lcDimStyle.Render(prefix), deployer, lcDimStyle.Render(invInfo)))
				}
			} else {
				b.WriteString(lcDimStyle.Render("  └─ (no deployers reference this source)") + "\n")
			}
			b.WriteString("\n")
		}
	}

	// OCIRepositories
	if len(ociRepos) > 0 {
		b.WriteString(lcPurpleStyle.Render("OCI REPOSITORIES") + " " + lcDimStyle.Render("(Gitless GitOps)") + "\n")
		for _, src := range ociRepos {
			statusIcon := lcOkStyle.Render("✓")
			if src.Status != "Ready" {
				statusIcon = lcWarnStyle.Render("⚠")
			}

			url := src.URL
			if len(url) > 45 {
				url = url[:42] + "..."
			}

			b.WriteString(fmt.Sprintf("%s %s\n", statusIcon, lcNameStyle.Render(src.Name)))
			b.WriteString(fmt.Sprintf("  %s", lcDimStyle.Render(url)))
			if src.Tag != "" {
				b.WriteString(fmt.Sprintf(" @ %s", lcDimStyle.Render(src.Tag)))
			}
			b.WriteString("\n")

			if len(src.Deployers) > 0 {
				for i, deployer := range src.Deployers {
					prefix := "├─▶"
					if i == len(src.Deployers)-1 {
						prefix = "└─▶"
					}
					invCount := deployerInventory[deployer]
					invInfo := ""
					if invCount > 0 {
						invInfo = fmt.Sprintf(" → %d resources", invCount)
					}
					b.WriteString(fmt.Sprintf("  %s %s%s\n", lcDimStyle.Render(prefix), deployer, lcDimStyle.Render(invInfo)))
				}
			}
			b.WriteString("\n")
		}
	}

	// HelmRepositories
	if len(helmRepos) > 0 {
		b.WriteString(lcWarnStyle.Render("HELM REPOSITORIES") + "\n")
		for _, src := range helmRepos {
			statusIcon := lcOkStyle.Render("✓")
			if src.Status != "Ready" {
				statusIcon = lcWarnStyle.Render("⚠")
			}

			url := src.URL
			if len(url) > 45 {
				url = url[:42] + "..."
			}

			repoType := src.Tag // We stored type in Tag field
			if repoType == "" {
				repoType = "default"
			}

			b.WriteString(fmt.Sprintf("%s %s", statusIcon, lcNameStyle.Render(src.Name)))
			b.WriteString(fmt.Sprintf(" (%s)\n", lcDimStyle.Render(repoType)))
			b.WriteString(fmt.Sprintf("  %s\n", lcDimStyle.Render(url)))

			if len(src.Deployers) > 0 {
				for i, deployer := range src.Deployers {
					prefix := "├─▶"
					if i == len(src.Deployers)-1 {
						prefix = "└─▶"
					}
					b.WriteString(fmt.Sprintf("  %s %s\n", lcDimStyle.Render(prefix), deployer))
				}
			}
			b.WriteString("\n")
		}
	}

	// ConfigHub upsell
	b.WriteString(lcDimStyle.Render("────────────────────────────────────────────────────────") + "\n\n")
	b.WriteString(lcDimStyle.Render("💡 This view shows what the ") + lcCyanStyle.Render("cluster") + lcDimStyle.Render(" tells us about Git.") + "\n")
	b.WriteString(lcDimStyle.Render("   For the full picture:") + "\n")
	b.WriteString(lcDimStyle.Render("   • Actual Git repo structure (folders, branches)") + "\n")
	b.WriteString(lcDimStyle.Render("   • DRY→WET→LIVE causality chain") + "\n")
	b.WriteString(lcDimStyle.Render("   • Fleet-wide visibility across clusters") + "\n")
	b.WriteString(lcDimStyle.Render("   • Change history and rollback tracking") + "\n\n")
	b.WriteString(lcDimStyle.Render("   Press ") + lcCyanStyle.Render("H") + lcDimStyle.Render(" to connect to ConfigHub") + "\n")

	return b.String()
}

func (m LocalClusterModel) getPanelMaps() string {
	var b strings.Builder

	// MAP 1: GitOps Resource Trees
	b.WriteString(lcSectionStyle.Render("MAP 1: GITOPS TREES") + "\n")

	if len(m.gitops) == 0 {
		b.WriteString(lcDimStyle.Render("No GitOps deployers") + "\n")
	} else {
		for _, g := range m.gitops {
			kindStyle := lcCyanStyle
			if g.Kind == "Application" {
				kindStyle = lcPurpleStyle
			} else if g.Kind == "HelmRelease" {
				kindStyle = lcWarnStyle
			}

			statusIcon := lcOkStyle.Render("✓")
			if g.Status != "Ready" && g.Status != "Healthy" {
				statusIcon = lcWarnStyle.Render("⚠")
			}

			b.WriteString(fmt.Sprintf("%s %s %s",
				statusIcon,
				kindStyle.Render(g.Kind),
				lcNameStyle.Render(g.Name)))
			if g.InventoryCount > 0 {
				b.WriteString(fmt.Sprintf(" → %d", g.InventoryCount))
			}
			b.WriteString("\n")
		}
	}

	// Native resources
	nativeCount := 0
	for _, e := range m.entries {
		if e.Owner == "Native" {
			nativeCount++
		}
	}
	if nativeCount > 0 {
		b.WriteString(fmt.Sprintf("\n%s Native: %d\n", lcDimStyle.Render("●"), nativeCount))
	}
	b.WriteString("\n")

	// MAP 2: ConfigHub
	b.WriteString(lcSectionStyle.Render("MAP 2: CONFIGHUB") + "\n")
	b.WriteString(lcDimStyle.Render("Press H for hierarchy") + "\n\n")

	// MAP 3: Repo Structure
	b.WriteString(lcSectionStyle.Render("MAP 3: REPO → DEPLOY") + "\n")

	sources := map[string][]string{}
	for _, g := range m.gitops {
		if g.Source != "" {
			key := g.Source
			if g.Path != "" {
				key = g.Source + " @ " + g.Path
			}
			sources[key] = append(sources[key], g.Name)
		}
	}

	if len(sources) == 0 {
		b.WriteString(lcDimStyle.Render("No Git sources") + "\n")
	} else {
		for source, names := range sources {
			b.WriteString(fmt.Sprintf("└── %s\n", lcCyanStyle.Render(source)))
			for _, name := range names {
				b.WriteString(fmt.Sprintf("    └── %s\n", lcNameStyle.Render(name)))
			}
		}
	}

	return b.String()
}

// getPanelClusterData returns the Cluster Data view content
// Shows all data sources the TUI is reading from the cluster with FULL detail
func (m LocalClusterModel) getPanelClusterData() string {
	var b strings.Builder

	// Count resources by type
	fluxKustomizations := 0
	fluxHelmReleases := 0
	argoApps := 0
	argoAppSets := 0
	helmWorkloads := 0
	nativeResources := 0
	configHubResources := 0

	for _, g := range m.gitops {
		switch g.Kind {
		case "Kustomization":
			fluxKustomizations++
		case "HelmRelease":
			fluxHelmReleases++
		case "Application":
			argoApps++
		case "ApplicationSet":
			argoAppSets++
		}
	}

	for _, e := range m.entries {
		switch e.Owner {
		case "Helm":
			helmWorkloads++
		case "Native":
			nativeResources++
		case "ConfigHub":
			configHubResources++
		}
	}

	// ═══════════════════════════════════════════════════════════════
	// FLUX section with full details
	// ═══════════════════════════════════════════════════════════════
	fluxTotal := fluxKustomizations + fluxHelmReleases + len(m.gitSources)
	b.WriteString(lcSectionStyle.Render(fmt.Sprintf("⚡ FLUX (%d resources)", fluxTotal)) + "\n")
	if fluxTotal == 0 {
		b.WriteString(lcDimStyle.Render("  No Flux resources detected") + "\n")
	} else {
		// Kustomizations with full details
		if fluxKustomizations > 0 {
			b.WriteString(fmt.Sprintf("├── Kustomizations (%d)\n", fluxKustomizations))
			for _, g := range m.gitops {
				if g.Kind == "Kustomization" {
					statusIcon := lcOkStyle.Render("✓")
					if g.Status != "Ready" {
						statusIcon = lcWarnStyle.Render("⚠")
					}
					b.WriteString(fmt.Sprintf("│   %s %s\n", statusIcon, lcNameStyle.Render(g.Namespace+"/"+g.Name)))
					b.WriteString(fmt.Sprintf("│   │  Status: %s\n", lcDimStyle.Render(g.Status)))
					if g.Source != "" {
						b.WriteString(fmt.Sprintf("│   │  Source: %s\n", lcDimStyle.Render(g.Source)))
					}
					if g.Path != "" {
						b.WriteString(fmt.Sprintf("│   │  Path: %s\n", lcDimStyle.Render(g.Path)))
					}
					if g.InventoryCount > 0 {
						b.WriteString(fmt.Sprintf("│   │  Inventory: %d resources\n", g.InventoryCount))
					}
					if len(g.DependsOn) > 0 {
						b.WriteString(fmt.Sprintf("│   │  DependsOn: %s\n", lcDimStyle.Render(strings.Join(g.DependsOn, ", "))))
					}
					// Show workloads managed by this Kustomization
					workloadCount := 0
					for _, e := range m.entries {
						if e.Owner == "Flux" && e.OwnerDetails != nil {
							if e.OwnerDetails["name"] == g.Name || e.OwnerDetails["kustomization"] == g.Name {
								workloadCount++
							}
						}
					}
					if workloadCount > 0 {
						b.WriteString(fmt.Sprintf("│   └─ Workloads: %d\n", workloadCount))
					}
				}
			}
		}
		// HelmReleases
		if fluxHelmReleases > 0 {
			b.WriteString(fmt.Sprintf("├── HelmReleases (%d)\n", fluxHelmReleases))
			for _, g := range m.gitops {
				if g.Kind == "HelmRelease" {
					statusIcon := lcOkStyle.Render("✓")
					if g.Status != "Ready" {
						statusIcon = lcWarnStyle.Render("⚠")
					}
					b.WriteString(fmt.Sprintf("│   %s %s\n", statusIcon, lcNameStyle.Render(g.Namespace+"/"+g.Name)))
					b.WriteString(fmt.Sprintf("│   │  Status: %s\n", lcDimStyle.Render(g.Status)))
					if g.Source != "" {
						b.WriteString(fmt.Sprintf("│   └─ Chart: %s\n", lcDimStyle.Render(g.Source)))
					}
				}
			}
		}
		// Git sources with full details
		for _, s := range m.gitSources {
			if s.Kind == "GitRepository" {
				statusIcon := lcOkStyle.Render("✓")
				if s.Status != "Ready" {
					statusIcon = lcWarnStyle.Render("⚠")
				}
				b.WriteString(fmt.Sprintf("├── GitRepository: %s %s\n", statusIcon, lcNameStyle.Render(s.Namespace+"/"+s.Name)))
				b.WriteString(fmt.Sprintf("│   │  URL: %s\n", lcDimStyle.Render(s.URL)))
				if s.Branch != "" {
					b.WriteString(fmt.Sprintf("│   │  Branch: %s\n", lcDimStyle.Render(s.Branch)))
				}
				if s.Revision != "" {
					rev := s.Revision
					if len(rev) > 12 {
						rev = rev[:12]
					}
					b.WriteString(fmt.Sprintf("│   │  Revision: %s\n", lcDimStyle.Render(rev)))
				}
				if len(s.Deployers) > 0 {
					b.WriteString(fmt.Sprintf("│   └─ Used by: %s\n", lcDimStyle.Render(strings.Join(s.Deployers, ", "))))
				}
			}
		}
		// Helm repositories
		for _, s := range m.gitSources {
			if s.Kind == "HelmRepository" {
				statusIcon := lcOkStyle.Render("✓")
				if s.Status != "Ready" {
					statusIcon = lcWarnStyle.Render("⚠")
				}
				b.WriteString(fmt.Sprintf("├── HelmRepository: %s %s\n", statusIcon, lcNameStyle.Render(s.Name)))
				b.WriteString(fmt.Sprintf("│   └─ URL: %s\n", lcDimStyle.Render(s.URL)))
			}
		}
		// OCI repositories
		for _, s := range m.gitSources {
			if s.Kind == "OCIRepository" {
				statusIcon := lcOkStyle.Render("✓")
				if s.Status != "Ready" {
					statusIcon = lcWarnStyle.Render("⚠")
				}
				b.WriteString(fmt.Sprintf("└── OCIRepository: %s %s\n", statusIcon, lcNameStyle.Render(s.Name)))
				b.WriteString(fmt.Sprintf("    └─ URL: %s\n", lcDimStyle.Render(s.URL)))
			}
		}
	}
	b.WriteString("\n")

	// ═══════════════════════════════════════════════════════════════
	// ARGOCD section with full details
	// ═══════════════════════════════════════════════════════════════
	argoTotal := argoApps + argoAppSets
	b.WriteString(lcSectionStyle.Render(fmt.Sprintf("🅰 ARGOCD (%d resources)", argoTotal)) + "\n")
	if argoTotal == 0 {
		b.WriteString(lcDimStyle.Render("  No ArgoCD resources detected") + "\n")
	} else {
		if argoApps > 0 {
			b.WriteString(fmt.Sprintf("├── Applications (%d)\n", argoApps))
			for _, g := range m.gitops {
				if g.Kind == "Application" {
					statusIcon := lcOkStyle.Render("✓")
					status := g.Status
					if status != "Healthy" && status != "Synced" && status != "Healthy/Synced" {
						statusIcon = lcWarnStyle.Render("⚠")
					}
					b.WriteString(fmt.Sprintf("│   %s %s\n", statusIcon, lcNameStyle.Render(g.Namespace+"/"+g.Name)))
					b.WriteString(fmt.Sprintf("│   │  Status: %s\n", lcDimStyle.Render(status)))
					if g.Source != "" {
						b.WriteString(fmt.Sprintf("│   │  Source: %s\n", lcDimStyle.Render(g.Source)))
					}
					if g.Path != "" {
						b.WriteString(fmt.Sprintf("│   │  Path: %s\n", lcDimStyle.Render(g.Path)))
					}
					// Show workloads managed by this Application
					workloadCount := 0
					for _, e := range m.entries {
						if e.Owner == "ArgoCD" && e.OwnerDetails != nil {
							if e.OwnerDetails["name"] == g.Name || e.OwnerDetails["application"] == g.Name {
								workloadCount++
							}
						}
					}
					if workloadCount > 0 {
						b.WriteString(fmt.Sprintf("│   └─ Workloads: %d\n", workloadCount))
					}
				}
			}
		}
		if argoAppSets > 0 {
			b.WriteString(fmt.Sprintf("└── ApplicationSets (%d)\n", argoAppSets))
			for _, g := range m.gitops {
				if g.Kind == "ApplicationSet" {
					b.WriteString(fmt.Sprintf("    ├── %s\n", lcNameStyle.Render(g.Name)))
				}
			}
		}
	}
	b.WriteString("\n")

	// ═══════════════════════════════════════════════════════════════
	// HELM section (standalone releases)
	// ═══════════════════════════════════════════════════════════════
	b.WriteString(lcSectionStyle.Render(fmt.Sprintf("⎈ HELM (%d workloads)", helmWorkloads)) + "\n")
	if helmWorkloads == 0 {
		b.WriteString(lcDimStyle.Render("  No standalone Helm releases") + "\n")
	} else {
		// Group by release name
		releases := map[string][]MapEntry{}
		for _, e := range m.entries {
			if e.Owner == "Helm" {
				releaseName := e.Name
				if e.OwnerDetails != nil {
					if r, ok := e.OwnerDetails["release"]; ok && r != "" {
						releaseName = r
					}
				}
				releases[releaseName] = append(releases[releaseName], e)
			}
		}
		count := 0
		for release, entries := range releases {
			count++
			prefix := "├──"
			if count == len(releases) {
				prefix = "└──"
			}
			statusIcon := lcOkStyle.Render("✓")
			for _, e := range entries {
				if e.Status != "Ready" && e.Status != "Running" {
					statusIcon = lcWarnStyle.Render("⚠")
					break
				}
			}
			b.WriteString(fmt.Sprintf("%s %s %s (%d workloads)\n", prefix, statusIcon, lcNameStyle.Render(release), len(entries)))
			// Show workload details
			for i, e := range entries {
				wlPrefix := "│   ├──"
				if i == len(entries)-1 {
					wlPrefix = "│   └──"
				}
				if count == len(releases) {
					wlPrefix = strings.Replace(wlPrefix, "│", " ", 1)
				}
				b.WriteString(fmt.Sprintf("%s %s/%s in %s\n", wlPrefix, e.Kind, e.Name, lcDimStyle.Render(e.Namespace)))
			}
		}
	}
	b.WriteString("\n")

	// ═══════════════════════════════════════════════════════════════
	// CONFIGHUB section
	// ═══════════════════════════════════════════════════════════════
	if configHubResources > 0 {
		b.WriteString(lcSectionStyle.Render(fmt.Sprintf("📦 CONFIGHUB (%d resources)", configHubResources)) + "\n")
		count := 0
		for _, e := range m.entries {
			if e.Owner == "ConfigHub" {
				count++
				prefix := "├──"
				if count == configHubResources {
					prefix = "└──"
				}
				unitSlug := ""
				if e.Labels != nil {
					unitSlug = e.Labels["confighub.com/UnitSlug"]
				}
				if unitSlug == "" && e.OwnerDetails != nil {
					unitSlug = e.OwnerDetails["unit"]
				}
				statusIcon := lcOkStyle.Render("✓")
				if e.Status != "Ready" && e.Status != "Running" {
					statusIcon = lcWarnStyle.Render("⚠")
				}
				b.WriteString(fmt.Sprintf("%s %s %s/%s\n", prefix, statusIcon, e.Kind, lcNameStyle.Render(e.Name)))
				if unitSlug != "" {
					childPrefix := "│   └──"
					if count == configHubResources {
						childPrefix = "    └──"
					}
					b.WriteString(fmt.Sprintf("%s Unit: %s\n", childPrefix, lcDimStyle.Render(unitSlug)))
				}
			}
		}
		b.WriteString("\n")
	}

	// ═══════════════════════════════════════════════════════════════
	// NATIVE section (orphans)
	// ═══════════════════════════════════════════════════════════════
	b.WriteString(lcSectionStyle.Render(fmt.Sprintf("☸ NATIVE (%d orphans)", nativeResources)) + "\n")
	if nativeResources == 0 {
		b.WriteString(lcOkStyle.Render("  ✓ No orphan resources - all managed by GitOps") + "\n")
	} else {
		b.WriteString(lcWarnStyle.Render("  ⚠ These resources are not tracked by GitOps") + "\n")
		// Group by namespace
		byNs := map[string][]MapEntry{}
		for _, e := range m.entries {
			if e.Owner == "Native" {
				byNs[e.Namespace] = append(byNs[e.Namespace], e)
			}
		}
		nsCount := 0
		for ns, entries := range byNs {
			nsCount++
			nsPrefix := "├──"
			if nsCount == len(byNs) {
				nsPrefix = "└──"
			}
			b.WriteString(fmt.Sprintf("%s %s (%d)\n", nsPrefix, lcNameStyle.Render(ns), len(entries)))
			for i, e := range entries {
				if i >= 3 {
					childPrefix := "│   └──"
					if nsCount == len(byNs) {
						childPrefix = "    └──"
					}
					b.WriteString(fmt.Sprintf("%s ... and %d more\n", childPrefix, len(entries)-3))
					break
				}
				childPrefix := "│   ├──"
				if i == len(entries)-1 || i == 2 {
					childPrefix = "│   └──"
				}
				if nsCount == len(byNs) {
					childPrefix = strings.Replace(childPrefix, "│", " ", 1)
				}
				age := ""
				if !e.CreatedAt.IsZero() {
					age = fmt.Sprintf(" (%s old)", formatAge(time.Since(e.CreatedAt)))
				}
				b.WriteString(fmt.Sprintf("%s %s/%s%s\n", childPrefix, lcDimStyle.Render(e.Kind), e.Name, lcDimStyle.Render(age)))
			}
		}
	}
	b.WriteString("\n")

	// ═══════════════════════════════════════════════════════════════
	// SUMMARY
	// ═══════════════════════════════════════════════════════════════
	b.WriteString(lcSectionStyle.Render("SUMMARY") + "\n")
	total := len(m.entries)
	gitopsManaged := total - nativeResources
	b.WriteString(fmt.Sprintf("├── Total workloads: %d\n", total))
	b.WriteString(fmt.Sprintf("├── GitOps managed: %d (%.0f%%)\n", gitopsManaged, float64(gitopsManaged)/float64(max(total, 1))*100))
	b.WriteString(fmt.Sprintf("├── Orphans: %d\n", nativeResources))
	b.WriteString(fmt.Sprintf("└── Deployers: %d Flux, %d ArgoCD\n", fluxKustomizations+fluxHelmReleases, argoApps+argoAppSets))
	b.WriteString("\n")

	// Upgrade CTA
	b.WriteString(lcDimStyle.Render("───────────────────────────────────────────────────────") + "\n")
	b.WriteString(lcDimStyle.Render("💡 What ConfigHub adds:") + "\n")
	b.WriteString(lcDimStyle.Render("   • Fleet-wide visibility across clusters") + "\n")
	b.WriteString(lcDimStyle.Render("   • Dependency tracking between services") + "\n")
	b.WriteString(lcDimStyle.Render("   • Change history and audit trail") + "\n")
	b.WriteString(lcDimStyle.Render("   Run: cub auth login") + "\n")

	return b.String()
}

// formatAge formats a duration as a human-readable age string
func formatAge(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	if days < 7 {
		return fmt.Sprintf("%d days", days)
	}
	weeks := days / 7
	if weeks == 1 {
		return "1 week"
	}
	return fmt.Sprintf("%d weeks", weeks)
}

// getPanelAppHierarchy returns the App Hierarchy view content
// Shows TUI's best-effort interpretation of cluster in ConfigHub model with full LiveTree
func (m LocalClusterModel) getPanelAppHierarchy() string {
	var b strings.Builder

	// Header and Legend
	b.WriteString(lcSectionStyle.Render("APP HIERARCHY") + lcDimStyle.Render(" (Inferred ConfigHub Model)") + "\n\n")
	b.WriteString(lcDimStyle.Render("Legend: ✓ Ready  ✗ Not Ready  ⚡ Flux  🅰 Argo  ⎈ Helm  📦 ConfigHub  ☸ Native") + "\n\n")

	// Disclaimer
	b.WriteString(lcWarnStyle.Render("⚠ This is TUI's interpretation.") + "\n")
	b.WriteString(lcDimStyle.Render("  Connect to ConfigHub for official hierarchy.") + "\n\n")

	// Helper for owner icons
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

	// Helper for status icons
	statusIcon := func(ready bool) string {
		if ready {
			return "✓"
		}
		return "✗"
	}

	// ═══════════════════════════════════════════════════════════════════════════
	// UNITS TREE - Group workloads by their managing deployer
	// ═══════════════════════════════════════════════════════════════════════════
	b.WriteString(lcSectionStyle.Render("UNITS TREE") + lcDimStyle.Render(" (GitOps deployers + workloads)") + "\n")
	b.WriteString("───────────────────────────────────────────────────────────\n")

	// Build map of deployer -> workloads
	// Key format: "DeployerName" (from OwnerDetails)
	deployerWorkloads := map[string][]MapEntry{}
	nativeWorkloads := []MapEntry{}

	for _, e := range m.entries {
		if e.Owner == "Native" {
			nativeWorkloads = append(nativeWorkloads, e)
		} else if e.OwnerDetails != nil {
			// Get the deployer name from OwnerDetails
			deployerName := ""
			if name, ok := e.OwnerDetails["name"]; ok && name != "" {
				deployerName = name
			} else if ks, ok := e.OwnerDetails["kustomization"]; ok && ks != "" {
				deployerName = ks
			} else if app, ok := e.OwnerDetails["instance"]; ok && app != "" {
				deployerName = app
			} else if release, ok := e.OwnerDetails["release"]; ok && release != "" {
				deployerName = release
			}
			if deployerName != "" {
				deployerWorkloads[deployerName] = append(deployerWorkloads[deployerName], e)
			}
		}
	}

	// Show GitOps deployers with their workloads
	for _, g := range m.gitops {
		// Determine status
		ready := g.Status == "True" || g.Status == "Ready" || g.Status == "Synced" || g.Status == "Healthy"
		icon := ownerIcon(g.Kind)
		if strings.Contains(g.Kind, "Kustomization") {
			icon = "⚡"
		} else if strings.Contains(g.Kind, "Application") {
			icon = "🅰"
		} else if strings.Contains(g.Kind, "HelmRelease") {
			icon = "⎈"
		}

		b.WriteString(fmt.Sprintf("\n%s %s %s/%s\n", icon, statusIcon(ready), g.Kind, lcNameStyle.Render(g.Name)))
		b.WriteString("│\n")

		// Show source info
		if g.Source != "" {
			b.WriteString(fmt.Sprintf("├─ Source: %s\n", lcDimStyle.Render(g.Source)))
		}
		if g.Path != "" {
			b.WriteString(fmt.Sprintf("├─ Path:   %s\n", lcDimStyle.Render(g.Path)))
		}
		if g.Namespace != "" {
			b.WriteString(fmt.Sprintf("├─ Target: %s\n", lcDimStyle.Render(g.Namespace)))
		}

		// Show status
		b.WriteString(fmt.Sprintf("├─ Status: %s\n", lcDimStyle.Render(g.Status)))

		// Show dependencies if any
		if len(g.DependsOn) > 0 {
			b.WriteString(fmt.Sprintf("├─ DependsOn: %s\n", lcDimStyle.Render(strings.Join(g.DependsOn, ", "))))
		}

		// Find workloads managed by this deployer
		wls := deployerWorkloads[g.Name]
		if len(wls) > 0 {
			b.WriteString("│\n")
			b.WriteString(fmt.Sprintf("├─ Workloads (%d):\n", len(wls)))

			for i, wl := range wls {
				isLast := i == len(wls)-1
				wlPrefix := "│  ├─"
				if isLast {
					wlPrefix = "│  └─"
				}

				wlReady := wl.Status == "Ready" || wl.Status == "Running" || wl.Status == "Available"
				wlIcon := statusIcon(wlReady)

				b.WriteString(fmt.Sprintf("%s %s %s/%s (%s)\n", wlPrefix, wlIcon, wl.Kind, wl.Name, wl.Status))
			}
		} else if g.InventoryCount > 0 {
			b.WriteString(fmt.Sprintf("├─ Resources: %d managed\n", g.InventoryCount))
		} else {
			b.WriteString("│  (no workloads found)\n")
		}
		b.WriteString("│\n")
		b.WriteString("└─ (no dependencies detected)\n")
	}

	// ═══════════════════════════════════════════════════════════════════════════
	// NATIVE WORKLOADS - Not managed by GitOps
	// ═══════════════════════════════════════════════════════════════════════════
	if len(nativeWorkloads) > 0 {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("☸ %s (%d) - not tracked by GitOps\n",
			lcWarnStyle.Render("Native/Unmanaged Workloads"), len(nativeWorkloads)))
		b.WriteString("│\n")

		// Group by namespace
		byNs := map[string][]MapEntry{}
		for _, wl := range nativeWorkloads {
			byNs[wl.Namespace] = append(byNs[wl.Namespace], wl)
		}

		nsCount := 0
		for ns, wls := range byNs {
			nsCount++
			isLastNs := nsCount == len(byNs)
			nsPrefix := "├─"
			nsChildPrefix := "│  "
			if isLastNs {
				nsPrefix = "└─"
				nsChildPrefix = "   "
			}

			b.WriteString(fmt.Sprintf("%s %s/ (%d workloads)\n", nsPrefix, lcNameStyle.Render(ns), len(wls)))

			for i, wl := range wls {
				isLast := i == len(wls)-1
				wlPrefix := nsChildPrefix + "├─"
				if isLast {
					wlPrefix = nsChildPrefix + "└─"
				}

				wlReady := wl.Status == "Ready" || wl.Status == "Running" || wl.Status == "Available"
				wlIcon := statusIcon(wlReady)

				b.WriteString(fmt.Sprintf("%s %s %s/%s (%s)\n", wlPrefix, wlIcon, wl.Kind, wl.Name, wl.Status))
			}
		}
	}

	// ═══════════════════════════════════════════════════════════════════════════
	// NAMESPACE ANALYSIS - Inferred AppSpaces
	// ═══════════════════════════════════════════════════════════════════════════
	b.WriteString("\n")
	b.WriteString(lcSectionStyle.Render("NAMESPACE ANALYSIS") + lcDimStyle.Render(" → Inferred AppSpaces") + "\n")
	b.WriteString("───────────────────────────────────────────────────────────\n")

	// Group entries by namespace pattern (environment)
	byEnv := map[string]map[string][]MapEntry{
		"production":  {},
		"staging":     {},
		"development": {},
		"other":       {},
	}

	for _, e := range m.entries {
		ns := strings.ToLower(e.Namespace)
		var env string
		if strings.Contains(ns, "prod") || ns == "production" {
			env = "production"
		} else if strings.Contains(ns, "staging") || strings.Contains(ns, "stage") {
			env = "staging"
		} else if strings.Contains(ns, "dev") || strings.Contains(ns, "development") {
			env = "development"
		} else if ns != "kube-system" && ns != "flux-system" && ns != "argocd" && ns != "default" {
			env = "other"
		} else {
			continue
		}
		if byEnv[env][e.Namespace] == nil {
			byEnv[env][e.Namespace] = []MapEntry{}
		}
		byEnv[env][e.Namespace] = append(byEnv[env][e.Namespace], e)
	}

	for _, env := range []string{"production", "staging", "development", "other"} {
		nss := byEnv[env]
		if len(nss) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("\n[%s] %d namespace(s)\n", lcNameStyle.Render(strings.ToUpper(env)), len(nss)))
		for ns, entries := range nss {
			// Count by owner
			ownerCounts := map[string]int{}
			for _, e := range entries {
				ownerCounts[e.Owner]++
			}
			b.WriteString(fmt.Sprintf("  %s (%d workloads)\n", lcNameStyle.Render(ns), len(entries)))
			for owner, count := range ownerCounts {
				b.WriteString(fmt.Sprintf("    %s %s: %d\n", ownerIcon(owner), owner, count))
			}
		}
	}

	// ═══════════════════════════════════════════════════════════════════════════
	// INFERRED LABELS
	// ═══════════════════════════════════════════════════════════════════════════
	b.WriteString("\n")
	b.WriteString(lcSectionStyle.Render("INFERRED LABELS") + lcDimStyle.Render(" (for ConfigHub categorization)") + "\n")
	b.WriteString("───────────────────────────────────────────────────────────\n")

	// Collect unique label values
	groups := map[string]bool{}
	teams := map[string]bool{}
	apps := map[string]bool{}

	for _, e := range m.entries {
		if e.Labels != nil {
			if g, ok := e.Labels["app.kubernetes.io/component"]; ok && g != "" {
				groups[g] = true
			}
			if t, ok := e.Labels["team"]; ok && t != "" {
				teams[t] = true
			}
			if a, ok := e.Labels["app.kubernetes.io/name"]; ok && a != "" {
				apps[a] = true
			}
			if a, ok := e.Labels["app"]; ok && a != "" {
				apps[a] = true
			}
		}
	}

	if len(groups) > 0 {
		groupList := make([]string, 0, len(groups))
		for g := range groups {
			groupList = append(groupList, g)
		}
		b.WriteString(fmt.Sprintf("├─ component: %s\n", lcDimStyle.Render(strings.Join(groupList, ", "))))
	}
	if len(teams) > 0 {
		teamList := make([]string, 0, len(teams))
		for t := range teams {
			teamList = append(teamList, t)
		}
		b.WriteString(fmt.Sprintf("├─ team:      %s\n", lcDimStyle.Render(strings.Join(teamList, ", "))))
	}
	if len(apps) > 0 {
		appList := make([]string, 0, len(apps))
		for a := range apps {
			appList = append(appList, a)
		}
		if len(appList) > 10 {
			appList = appList[:10]
			appList = append(appList, "...")
		}
		b.WriteString(fmt.Sprintf("├─ app:       %s\n", lcDimStyle.Render(strings.Join(appList, ", "))))
	}
	b.WriteString(fmt.Sprintf("└─ tier:      %s (inferred from namespace)\n", lcDimStyle.Render("prod, staging, dev")))

	// ═══════════════════════════════════════════════════════════════════════════
	// SUMMARY
	// ═══════════════════════════════════════════════════════════════════════════
	b.WriteString("\n")
	b.WriteString(lcSectionStyle.Render("SUMMARY") + "\n")
	b.WriteString("───────────────────────────────────────────────────────────\n")

	// Count by owner
	ownerCounts := map[string]int{}
	for _, e := range m.entries {
		ownerCounts[e.Owner]++
	}

	b.WriteString(fmt.Sprintf("Total Deployers: %d\n", len(m.gitops)))
	b.WriteString(fmt.Sprintf("Total Workloads: %d\n", len(m.entries)))
	for _, owner := range []string{"Flux", "ArgoCD", "Helm", "ConfigHub", "Native"} {
		if count, ok := ownerCounts[owner]; ok && count > 0 {
			b.WriteString(fmt.Sprintf("  %s %s: %d\n", ownerIcon(owner), owner, count))
		}
	}

	// ═══════════════════════════════════════════════════════════════════════════
	// WHAT CONFIGHUB PROVIDES
	// ═══════════════════════════════════════════════════════════════════════════
	b.WriteString("\n")
	b.WriteString(lcDimStyle.Render("╭─────────────────────────────────────────────────────────╮") + "\n")
	b.WriteString(lcDimStyle.Render("│") + " 💡 What ConfigHub provides:                            " + lcDimStyle.Render("│") + "\n")
	b.WriteString(lcDimStyle.Render("│") + "   • Official hierarchy (not inferred)                 " + lcDimStyle.Render("│") + "\n")
	b.WriteString(lcDimStyle.Render("│") + "   • Dependency tracking (explicit, not guessed)       " + lcDimStyle.Render("│") + "\n")
	b.WriteString(lcDimStyle.Render("│") + "   • Cross-cluster visibility (fleet-wide)             " + lcDimStyle.Render("│") + "\n")
	b.WriteString(lcDimStyle.Render("│") + "   • Change history and audit trail                    " + lcDimStyle.Render("│") + "\n")
	b.WriteString(lcDimStyle.Render("│") + "   • Impact analysis before changes                    " + lcDimStyle.Render("│") + "\n")
	b.WriteString(lcDimStyle.Render("│") + "                                                       " + lcDimStyle.Render("│") + "\n")
	b.WriteString(lcDimStyle.Render("│") + " Run: " + lcNameStyle.Render("cub-scout map --hub") + " to connect              " + lcDimStyle.Render("│") + "\n")
	b.WriteString(lcDimStyle.Render("╰─────────────────────────────────────────────────────────╯") + "\n")

	return b.String()
}

func (m LocalClusterModel) renderDashboard() string {
	var b strings.Builder

	// Mode header (always shown)
	b.WriteString(m.renderModeHeader())

	if m.loading {
		b.WriteString("\n  " + m.spinner.View() + " Loading cluster data...\n")
		return b.String()
	}

	if m.err != nil {
		b.WriteString(lcErrStyle.Render("  Error: " + m.err.Error()) + "\n")
		return b.String()
	}

	// Count problems and health
	problems := m.countProblems()
	byOwner := m.countByOwner()
	total := len(m.entries)
	healthyDeployers := m.countHealthyDeployers()
	totalDeployers := len(m.gitops)

	// Health banner
	if problems == 0 {
		b.WriteString(lcOkStyle.Render(" ✓ ALL HEALTHY"))
	} else {
		b.WriteString(lcErrStyle.Render(fmt.Sprintf(" %d FAILURE(S)", problems)))
	}
	b.WriteString("\n\n")

	// Summary bars
	b.WriteString(fmt.Sprintf("  Deployers  %d/%d\n", healthyDeployers, totalDeployers))
	healthyWorkloads := m.countHealthyWorkloads()
	b.WriteString(fmt.Sprintf("  Workloads  %d/%d\n", healthyWorkloads, total))
	b.WriteString("\n")

	// PROBLEMS section (if any)
	if problems > 0 {
		b.WriteString(lcSectionStyle.Render("  PROBLEMS"))
		b.WriteString("\n")
		b.WriteString("  " + lcDimStyle.Render("────────────────────────────────────────────────") + "\n")

		// Show deployer problems
		for _, g := range m.gitops {
			if g.Status != "Ready" && g.Status != "Healthy" {
				b.WriteString(fmt.Sprintf("  %s/%s  %s\n",
					g.Kind, lcNameStyle.Render(g.Name),
					lcWarnStyle.Render(g.Status)))
			}
		}
		// Show workload problems
		for _, e := range m.entries {
			if e.Status == "Failed" || e.Status == "CrashLoopBackOff" || e.Status == "Error" {
				b.WriteString(fmt.Sprintf("  %s/%s  %s\n",
					e.Namespace, lcNameStyle.Render(e.Name),
					lcErrStyle.Render(e.Status)))
			}
		}
		b.WriteString("\n")
	}

	// PIPELINES section
	b.WriteString(lcSectionStyle.Render("  PIPELINES"))
	b.WriteString("\n")
	b.WriteString("  " + lcDimStyle.Render("────────────────────────────────────────────────") + "\n")
	if len(m.gitops) == 0 {
		b.WriteString("  " + lcDimStyle.Render("No GitOps pipelines found") + "\n")
	} else {
		for i, g := range m.gitops {
			if i >= 5 {
				b.WriteString(fmt.Sprintf("  ... and %d more\n", len(m.gitops)-5))
				break
			}
			// Format: source@rev  ->  name  ->  N resources
			source := g.Source
			if source == "" {
				source = "unknown"
			}
			resourceCount := ""
			if g.InventoryCount > 0 {
				resourceCount = fmt.Sprintf("  ->  %d resources", g.InventoryCount)
			}
			b.WriteString(fmt.Sprintf("  %s  ->  %s%s\n",
				lcCyanStyle.Render(source),
				lcNameStyle.Render(g.Name),
				resourceCount))
		}
	}
	b.WriteString("\n")

	// OWNERSHIP section (the key insight!)
	b.WriteString(lcSectionStyle.Render("  OWNERSHIP"))
	b.WriteString("\n")
	b.WriteString("  " + lcDimStyle.Render("────────────────────────────────────────────────") + "\n")

	// Build ownership string: Flux(12) Argo(8) Helm(5) Native(23)
	var ownerParts []string
	if byOwner["Flux"] > 0 {
		ownerParts = append(ownerParts, lcCyanStyle.Render(fmt.Sprintf("Flux(%d)", byOwner["Flux"])))
	}
	if byOwner["ArgoCD"] > 0 {
		ownerParts = append(ownerParts, lcPurpleStyle.Render(fmt.Sprintf("Argo(%d)", byOwner["ArgoCD"])))
	}
	if byOwner["ConfigHub"] > 0 {
		ownerParts = append(ownerParts, lcOkStyle.Render(fmt.Sprintf("ConfigHub(%d)", byOwner["ConfigHub"])))
	}
	if byOwner["Helm"] > 0 {
		ownerParts = append(ownerParts, lcWarnStyle.Render(fmt.Sprintf("Helm(%d)", byOwner["Helm"])))
	}
	if byOwner["Native"] > 0 {
		// Native highlighted as risk
		ownerParts = append(ownerParts, lcDimStyle.Render(fmt.Sprintf("Native(%d)", byOwner["Native"])))
	}

	if len(ownerParts) > 0 {
		b.WriteString("  " + strings.Join(ownerParts, " ") + "\n")
	} else {
		b.WriteString("  " + lcDimStyle.Render("No workloads found") + "\n")
	}

	// Ownership bar graph
	if total > 0 {
		barWidth := 24
		gitopsCount := byOwner["Flux"] + byOwner["ArgoCD"] + byOwner["ConfigHub"] + byOwner["Helm"]
		nativeCount := byOwner["Native"]
		gitopsBars := (gitopsCount * barWidth) / total
		nativeBars := barWidth - gitopsBars

		b.WriteString("  ")
		b.WriteString(lcOkStyle.Render(strings.Repeat("█", gitopsBars)))
		b.WriteString(lcDimStyle.Render(strings.Repeat("░", nativeBars)))
		b.WriteString("\n")

		// Native warning if significant
		if nativeCount > 0 {
			nativePct := (nativeCount * 100) / total
			if nativePct > 30 {
				b.WriteString("\n  " + lcWarnStyle.Render(fmt.Sprintf("⚠ %d%% unmanaged (Native) — check security", nativePct)) + "\n")
			}
		}
	}
	b.WriteString("\n")

	// Footer with key hints and namespace indicator
	b.WriteString(lcDimStyle.Render("────────────────────────────────────────────────────────────────"))
	b.WriteString("\n")
	nsIndicator := m.renderNamespaceIndicator()
	if nsIndicator != "" {
		b.WriteString(nsIndicator + " " + lcDimStyle.Render("|") + " ")
	}
	b.WriteString(lcDimStyle.Render("[:]cmd []/[]ns [w]ork [p]ipe [d]rift [o]rph [T]race [S]can | [H]ub [?] [q]"))
	b.WriteString("\n")

	// Show active query or status message
	if m.activeQuery != nil && m.activeQuery.Query != "" {
		filtered := m.getFilteredEntries()
		b.WriteString("\n" + lcCyanStyle.Render(fmt.Sprintf("Filter: %s (%d/%d)", m.activeQuery.Name, len(filtered), len(m.entries))) + "  " + lcDimStyle.Render("Q to change") + "\n")
	} else if m.statusMsg != "" {
		b.WriteString("\n" + lcDimStyle.Render(m.statusMsg) + "\n")
	}

	// Command mode
	if m.cmdMode {
		b.WriteString("\n" + lcCyanStyle.Render(":"))
		b.WriteString(m.cmdInput)
		b.WriteString("█")
		if len(m.cmdHistory) > 0 {
			b.WriteString("  ")
			b.WriteString(lcDimStyle.Render("↑↓ history"))
		}
		b.WriteString("\n")
	} else if m.cmdShowOutput && m.cmdOutput != "" {
		// Show command output
		b.WriteString("\n")
		outputLines := strings.Split(m.cmdOutput, "\n")
		maxLines := 8
		if len(outputLines) > maxLines {
			outputLines = outputLines[:maxLines]
			outputLines = append(outputLines, lcDimStyle.Render("..."))
		}
		for _, line := range outputLines {
			b.WriteString(lcDimStyle.Render("│ "))
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString(lcDimStyle.Render("Esc to dismiss"))
		b.WriteString("\n")
	}

	// Upgrade CTA for standalone mode (not connected to ConfigHub)
	b.WriteString("\n" + renderUpgradeCTA("dashboard") + "\n")

	return b.String()
}

// Helper methods for dashboard
func (m LocalClusterModel) countProblems() int {
	count := 0
	for _, g := range m.gitops {
		if g.Status != "Ready" && g.Status != "Healthy" {
			count++
		}
	}
	for _, e := range m.entries {
		if e.Status == "Failed" || e.Status == "CrashLoopBackOff" || e.Status == "Error" {
			count++
		}
	}
	return count
}

func (m LocalClusterModel) countByOwner() map[string]int {
	byOwner := map[string]int{}
	for _, e := range m.entries {
		byOwner[e.Owner]++
	}
	return byOwner
}

func (m LocalClusterModel) countHealthyDeployers() int {
	count := 0
	for _, g := range m.gitops {
		if g.Status == "Ready" || g.Status == "Healthy" {
			count++
		}
	}
	return count
}

func (m LocalClusterModel) countHealthyWorkloads() int {
	count := 0
	for _, e := range m.entries {
		if e.Status != "Failed" && e.Status != "CrashLoopBackOff" && e.Status != "Error" {
			count++
		}
	}
	return count
}

// Query helper methods

// matchesQuery checks if an entry matches the query
func (m LocalClusterModel) matchesQuery(e MapEntry, query string) bool {
	// Parse simple query patterns: field=value, field!=value, field=val*
	// Support multiple patterns with comma separation for IN lists
	query = strings.TrimSpace(query)

	// Handle OR conditions
	if strings.Contains(query, " OR ") {
		parts := strings.Split(query, " OR ")
		for _, part := range parts {
			if m.matchesQuery(e, strings.TrimSpace(part)) {
				return true
			}
		}
		return false
	}

	// Handle AND conditions
	if strings.Contains(query, " AND ") {
		parts := strings.Split(query, " AND ")
		for _, part := range parts {
			if !m.matchesQuery(e, strings.TrimSpace(part)) {
				return false
			}
		}
		return true
	}

	// Parse single condition
	var field, op, value string
	if strings.Contains(query, "!=") {
		parts := strings.SplitN(query, "!=", 2)
		field = strings.TrimSpace(parts[0])
		op = "!="
		value = strings.TrimSpace(parts[1])
	} else if strings.Contains(query, "=") {
		parts := strings.SplitN(query, "=", 2)
		field = strings.TrimSpace(parts[0])
		op = "="
		value = strings.TrimSpace(parts[1])
	} else {
		return true // No valid operator
	}

	// Get field value from entry
	var fieldValue string
	switch strings.ToLower(field) {
	case "owner":
		fieldValue = e.Owner
	case "namespace":
		fieldValue = e.Namespace
	case "name":
		fieldValue = e.Name
	case "kind":
		fieldValue = e.Kind
	default:
		return true // Unknown field, match all
	}

	// Handle IN list (comma-separated values)
	values := strings.Split(value, ",")

	// Check each value
	for _, v := range values {
		v = strings.TrimSpace(v)
		match := false

		// Handle wildcards
		if strings.HasPrefix(v, "*") && strings.HasSuffix(v, "*") {
			// Contains match
			core := strings.TrimPrefix(strings.TrimSuffix(v, "*"), "*")
			match = strings.Contains(strings.ToLower(fieldValue), strings.ToLower(core))
		} else if strings.HasPrefix(v, "*") {
			// Ends with
			match = strings.HasSuffix(strings.ToLower(fieldValue), strings.ToLower(strings.TrimPrefix(v, "*")))
		} else if strings.HasSuffix(v, "*") {
			// Starts with
			match = strings.HasPrefix(strings.ToLower(fieldValue), strings.ToLower(strings.TrimSuffix(v, "*")))
		} else {
			// Exact match (case-insensitive)
			match = strings.EqualFold(fieldValue, v)
		}

		if op == "=" && match {
			return true
		}
		if op == "!=" && match {
			return false
		}
	}

	// For != with no match found, return true
	// For = with no match found, return false
	return op == "!="
}

// getQueryStatusMsg returns a status message describing the active query
func (m LocalClusterModel) getQueryStatusMsg() string {
	if m.activeQuery == nil || m.activeQuery.Query == "" {
		return ""
	}
	filtered := m.getFilteredEntries()
	return fmt.Sprintf("Query: %s (%d matches)", m.activeQuery.Name, len(filtered))
}

// countQueryMatches counts how many entries match a query
func (m LocalClusterModel) countQueryMatches(query string) int {
	if query == "" {
		return len(m.entries)
	}
	count := 0
	for _, e := range m.entries {
		if m.matchesQuery(e, query) {
			count++
		}
	}
	return count
}

// Trace helper methods

// buildTraceItems creates the list of items available to trace
func (m LocalClusterModel) buildTraceItems() []TraceItem {
	var items []TraceItem

	// Add GitOps deployers first (most useful to trace)
	for _, g := range m.gitops {
		items = append(items, TraceItem{
			Kind:      g.Kind,
			Name:      g.Name,
			Namespace: g.Namespace,
			Owner:     m.getGitOpsOwner(g.Kind),
		})
	}

	// Add workloads that are GitOps-managed
	for _, e := range m.entries {
		if e.Owner == "Flux" || e.Owner == "ArgoCD" {
			items = append(items, TraceItem{
				Kind:      e.Kind,
				Name:      e.Name,
				Namespace: e.Namespace,
				Owner:     e.Owner,
			})
		}
	}

	return items
}

// getGitOpsOwner returns the owner type for a GitOps resource kind
func (m LocalClusterModel) getGitOpsOwner(kind string) string {
	switch kind {
	case "Kustomization", "HelmRelease":
		return "Flux"
	case "Application":
		return "ArgoCD"
	default:
		return "Unknown"
	}
}

// runTrace runs the trace command for a given item
func (m LocalClusterModel) runTrace(item TraceItem) tea.Cmd {
	return func() tea.Msg {
		var output string
		var err error

		switch item.Owner {
		case "Flux":
			// Use flux trace command
			cmd := exec.Command("flux", "trace", strings.ToLower(item.Kind), item.Name, "-n", item.Namespace)
			out, cmdErr := cmd.CombinedOutput()
			output = string(out)
			if cmdErr != nil {
				// Try alternative: if tracing a workload, trace the kustomization
				if item.Kind == "Deployment" || item.Kind == "StatefulSet" || item.Kind == "DaemonSet" {
					cmd = exec.Command("flux", "trace", strings.ToLower(item.Kind)+"/"+item.Name, "-n", item.Namespace)
					out, cmdErr = cmd.CombinedOutput()
					output = string(out)
				}
				if cmdErr != nil && output == "" {
					err = cmdErr
				}
			}

		case "ArgoCD":
			// Use argocd app get command
			if item.Kind == "Application" {
				cmd := exec.Command("argocd", "app", "get", item.Name, "-o", "wide")
				out, cmdErr := cmd.CombinedOutput()
				output = string(out)
				if cmdErr != nil {
					err = cmdErr
				}
			} else {
				// For workloads, try to find the parent Application
				output = fmt.Sprintf("ArgoCD trace for %s/%s\n\nTo trace this resource, find its parent Application in the argocd namespace.", item.Namespace, item.Name)
			}

		default:
			output = fmt.Sprintf("No GitOps owner found for %s/%s\n\nThis resource is not managed by Flux or ArgoCD.", item.Namespace, item.Name)
		}

		return traceResultMsg{output: output, err: err}
	}
}

// runScan runs the scan command
func (m LocalClusterModel) runScan() tea.Cmd {
	return func() tea.Msg {
		// Run cub-scout scan command
		cmd := exec.Command("./cub-scout", "scan")
		out, err := cmd.CombinedOutput()
		if err != nil {
			// Try without ./
			cmd = exec.Command("cub-scout", "scan")
			out, err = cmd.CombinedOutput()
		}

		// Parse output to extract findings and categories
		output := string(out)
		findings, categories := parseScanOutput(output)

		return scanResultMsg{
			output:     output,
			err:        err,
			findings:   findings,
			categories: categories,
		}
	}
}

// runRemedyDryRun runs the remedy command in dry-run mode
func (m LocalClusterModel) runRemedyDryRun() tea.Cmd {
	return func() tea.Msg {
		// Collect CCVE IDs from findings
		var ccveIDs []string
		seen := make(map[string]bool)
		for _, f := range m.scanFindings {
			if !seen[f.CCVE] {
				ccveIDs = append(ccveIDs, f.CCVE)
				seen[f.CCVE] = true
			}
		}

		if len(ccveIDs) == 0 {
			return scanResultMsg{
				output: "No findings to fix",
				err:    nil,
			}
		}

		// Run remedy --all --dry-run to show what would be fixed
		cmd := exec.Command("./cub-scout", "remedy", "--all", "--dry-run")
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmd = exec.Command("cub-scout", "remedy", "--all", "--dry-run")
			out, err = cmd.CombinedOutput()
		}

		output := string(out)
		if err == nil {
			output = "DRY RUN - Preview of changes:\n" + lcDimStyle.Render("─────────────────────────────────────────────────────────────────") + "\n" + output
			output += "\n" + lcWarnStyle.Render("Press [F] to apply these fixes, or any other key to cancel")
		}

		return scanResultMsg{
			output:     output,
			err:        err,
			findings:   m.scanFindings,
			categories: m.scanCategories,
		}
	}
}

// runRemedyApply runs the remedy command to actually apply fixes
func (m LocalClusterModel) runRemedyApply() tea.Cmd {
	return func() tea.Msg {
		// Run remedy --all (without dry-run) to apply fixes
		cmd := exec.Command("./cub-scout", "remedy", "--all", "--dry-run=false", "--force")
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmd = exec.Command("cub-scout", "remedy", "--all", "--dry-run=false", "--force")
			out, err = cmd.CombinedOutput()
		}

		output := string(out)
		if err == nil {
			output = lcOkStyle.Render("REMEDIATION APPLIED") + "\n" + lcDimStyle.Render("─────────────────────────────────────────────────────────────────") + "\n" + output
		}

		return scanResultMsg{
			output:     output,
			err:        err,
			findings:   nil, // Clear findings after apply
			categories: nil,
		}
	}
}

// runLocalCommand executes a shell command and returns the output
func runLocalCommand(command string) tea.Cmd {
	return func() tea.Msg {
		// Parse command into parts
		parts := strings.Fields(command)
		if len(parts) == 0 {
			return localCmdCompleteMsg{err: fmt.Errorf("empty command")}
		}

		cmdName := parts[0]
		var args []string
		if len(parts) > 1 {
			args = parts[1:]
		}

		// Execute command
		cmd := exec.Command(cmdName, args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return localCmdCompleteMsg{
				output: string(output),
				err:    fmt.Errorf("%s: %w", cmdName, err),
			}
		}
		return localCmdCompleteMsg{output: string(output)}
	}
}

// parseScanOutput parses scan text output to extract findings by category
func parseScanOutput(output string) ([]scanFinding, map[string]int) {
	findings := []scanFinding{}
	categories := map[string]int{}

	// CCVE categories
	knownCategories := []string{"STATE", "ORPHAN", "DRIFT", "CONFIG", "SOURCE", "RENDER", "APPLY", "DEPEND"}

	// Pattern to match CCVE findings: [S] CCVE-2025-XXXX: message
	// Severity markers: [C] critical, [W] warning, [I] info, [S] state
	ccvePattern := regexp.MustCompile(`\[([CWIS])\]\s+(CCVE-\d{4}-\d{4})[:\s]+(.*)`)

	lines := strings.Split(output, "\n")
	currentCategory := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check if line is a category header
		for _, cat := range knownCategories {
			if strings.HasPrefix(line, cat+" (") || strings.HasPrefix(line, cat+":") || line == cat {
				currentCategory = cat
				break
			}
		}

		// Try to match CCVE pattern
		matches := ccvePattern.FindStringSubmatch(line)
		if len(matches) >= 4 {
			severity := "info"
			switch matches[1] {
			case "C":
				severity = "critical"
			case "W":
				severity = "warning"
			case "I":
				severity = "info"
			case "S":
				severity = "state"
			}

			finding := scanFinding{
				CCVE:     matches[2],
				Severity: severity,
				Category: currentCategory,
				Message:  matches[3],
			}
			findings = append(findings, finding)

			// Count by category
			if currentCategory != "" {
				categories[currentCategory]++
			} else {
				categories["OTHER"]++
			}
		}
	}

	return findings, categories
}

// renderTrace renders the trace picker or result view
func (m LocalClusterModel) renderTrace() string {
	var b strings.Builder

	b.WriteString(lcHeaderStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("│") + "  " + lcHeaderStyle.Render("🔗 TRACE OWNERSHIP CHAIN") + strings.Repeat(" ", 37) + lcHeaderStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	// Loading state
	if m.traceLoading {
		b.WriteString("  " + m.spinner.View() + " Running trace...\n")
		b.WriteString("\n" + lcDimStyle.Render("Press Esc to cancel") + "\n")
		return b.String()
	}

	// Show trace result if available
	if m.traceOutput != "" {
		b.WriteString(m.traceOutput)
		b.WriteString("\n\n" + lcDimStyle.Render("Press any key to continue") + "\n")
		return b.String()
	}

	// Show error if any
	if m.traceError != nil {
		b.WriteString(lcErrStyle.Render("Error: " + m.traceError.Error()) + "\n")
		b.WriteString("\n" + lcDimStyle.Render("Press any key to continue") + "\n")
		return b.String()
	}

	// Show trace picker
	if len(m.traceItems) == 0 {
		b.WriteString(lcDimStyle.Render("No GitOps resources found to trace") + "\n")
		b.WriteString(lcDimStyle.Render("Install Flux or ArgoCD to use trace") + "\n")
		b.WriteString("\n" + lcDimStyle.Render("Press Esc to return") + "\n")
		return b.String()
	}

	b.WriteString(lcSectionStyle.Render("SELECT RESOURCE TO TRACE"))
	b.WriteString("\n")
	b.WriteString(lcDimStyle.Render("─────────────────────────────────────────────────────────────────"))
	b.WriteString("\n\n")

	for i, item := range m.traceItems {
		cursor := "  "
		nameStyle := lcNameStyle
		if i == m.traceCursor {
			cursor = lcOkStyle.Render("▸ ")
			nameStyle = lcHeaderStyle
		}

		// Color by owner
		ownerStyle := lcDimStyle
		switch item.Owner {
		case "Flux":
			ownerStyle = lcCyanStyle
		case "ArgoCD":
			ownerStyle = lcPurpleStyle
		}

		b.WriteString(fmt.Sprintf("%s%s %s/%s %s\n",
			cursor,
			ownerStyle.Render(fmt.Sprintf("%-12s", item.Kind)),
			item.Namespace,
			nameStyle.Render(item.Name),
			lcDimStyle.Render("("+item.Owner+")")))
	}

	b.WriteString("\n")
	b.WriteString(lcDimStyle.Render("─────────────────────────────────────────────────────────────────"))
	b.WriteString("\n")
	b.WriteString(lcDimStyle.Render("↑/↓ select  Enter trace  Esc cancel"))
	b.WriteString("\n")

	return b.String()
}

// renderScan renders the scan results view
func (m LocalClusterModel) renderScan() string {
	var b strings.Builder

	b.WriteString(lcHeaderStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("│") + "  " + lcHeaderStyle.Render("🔍 CCVE SCAN RESULTS") + strings.Repeat(" ", 41) + lcHeaderStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	// Loading state
	if m.scanLoading {
		b.WriteString("  " + m.spinner.View() + " Scanning for configuration issues...\n")
		b.WriteString("\n" + lcDimStyle.Render("Press Esc to cancel") + "\n")
		return b.String()
	}

	// Show error if any
	if m.scanError != nil {
		b.WriteString(lcErrStyle.Render("Error running scan: " + m.scanError.Error()) + "\n\n")
		b.WriteString(lcDimStyle.Render("Make sure cub-scout is in your PATH or run from project root.") + "\n")
		b.WriteString("\n" + lcDimStyle.Render("Press any key to return") + "\n")
		return b.String()
	}

	// Show scan output with category grouping
	if len(m.scanFindings) > 0 {
		// Summary line
		totalFindings := len(m.scanFindings)
		criticalCount := 0
		warningCount := 0
		for _, f := range m.scanFindings {
			if f.Severity == "critical" {
				criticalCount++
			} else if f.Severity == "warning" {
				warningCount++
			}
		}

		summaryParts := []string{fmt.Sprintf("%d findings", totalFindings)}
		if criticalCount > 0 {
			summaryParts = append(summaryParts, lcErrStyle.Render(fmt.Sprintf("%d critical", criticalCount)))
		}
		if warningCount > 0 {
			summaryParts = append(summaryParts, lcWarnStyle.Render(fmt.Sprintf("%d warning", warningCount)))
		}
		b.WriteString(strings.Join(summaryParts, " · ") + "\n\n")

		// Group findings by category
		categoryOrder := []string{"STATE", "ORPHAN", "DRIFT", "CONFIG", "SOURCE", "RENDER", "APPLY", "DEPEND"}
		findingsByCategory := make(map[string][]scanFinding)
		for _, f := range m.scanFindings {
			cat := f.Category
			if cat == "" {
				cat = "UNCATEGORIZED"
			}
			findingsByCategory[cat] = append(findingsByCategory[cat], f)
		}

		// Render each category
		for _, cat := range categoryOrder {
			findings, ok := findingsByCategory[cat]
			if !ok || len(findings) == 0 {
				continue
			}

			// Category header with count
			b.WriteString(lcSectionStyle.Render(fmt.Sprintf("%s (%d)", cat, len(findings))))
			b.WriteString("\n")
			b.WriteString(lcDimStyle.Render("─────────────────────────────────────────────────────────────────"))
			b.WriteString("\n")

			// Each finding in category
			for _, f := range findings {
				// Severity icon
				var severityIcon string
				switch f.Severity {
				case "critical":
					severityIcon = lcErrStyle.Render("[C]")
				case "warning":
					severityIcon = lcWarnStyle.Render("[W]")
				case "info":
					severityIcon = lcDimStyle.Render("[I]")
				case "state":
					severityIcon = lcCyanStyle.Render("[S]")
				default:
					severityIcon = lcDimStyle.Render("[?]")
				}

				b.WriteString(fmt.Sprintf("  %s %s  %s\n",
					severityIcon,
					lcCyanStyle.Render(f.CCVE),
					f.Message))
			}
			b.WriteString("\n")
		}

		// Handle uncategorized
		if uncategorized, ok := findingsByCategory["UNCATEGORIZED"]; ok && len(uncategorized) > 0 {
			b.WriteString(lcSectionStyle.Render(fmt.Sprintf("OTHER (%d)", len(uncategorized))))
			b.WriteString("\n")
			b.WriteString(lcDimStyle.Render("─────────────────────────────────────────────────────────────────"))
			b.WriteString("\n")
			for _, f := range uncategorized {
				var severityIcon string
				switch f.Severity {
				case "critical":
					severityIcon = lcErrStyle.Render("[C]")
				case "warning":
					severityIcon = lcWarnStyle.Render("[W]")
				default:
					severityIcon = lcDimStyle.Render("[?]")
				}
				b.WriteString(fmt.Sprintf("  %s %s  %s\n", severityIcon, lcCyanStyle.Render(f.CCVE), f.Message))
			}
			b.WriteString("\n")
		}
	} else if m.scanOutput != "" {
		// Fallback to raw output if parsing didn't find structured data
		b.WriteString(m.scanOutput)
	} else {
		b.WriteString(lcOkStyle.Render("✓ No configuration issues found") + "\n")
	}

	b.WriteString("\n" + lcDimStyle.Render("[f] preview fix (dry-run) · [F] apply fix · any other key to return") + "\n")

	return b.String()
}

// renderXref renders the cross-reference navigation overlay
func (m LocalClusterModel) renderXref() string {
	var b strings.Builder

	b.WriteString(lcHeaderStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	title := fmt.Sprintf("CROSS-REFERENCES: %s", m.xrefSourceName)
	padding := 64 - len(title) - 4
	if padding < 0 {
		padding = 0
	}
	b.WriteString(lcHeaderStyle.Render("│") + "  " + lcNameStyle.Render(title) + strings.Repeat(" ", padding) + lcHeaderStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	b.WriteString(lcDimStyle.Render(fmt.Sprintf("Related items for %s '%s':", m.xrefSourceType, m.xrefSourceName)) + "\n\n")

	if len(m.xrefItems) == 0 {
		b.WriteString(lcDimStyle.Render("No cross-references found.") + "\n")
	} else {
		for i, item := range m.xrefItems {
			cursor := "  "
			if i == m.xrefCursor {
				cursor = lcOkStyle.Render("▸ ")
			}

			// Format based on type
			kindStyle := lcDimStyle
			switch item.Type {
			case "gitops":
				kindStyle = lcCyanStyle
			case "workload":
				kindStyle = lcOkStyle
			case "dependency":
				kindStyle = lcWarnStyle
			case "adoption":
				kindStyle = lcPurpleStyle
			}

			line := fmt.Sprintf("%s%s %s",
				cursor,
				kindStyle.Render(item.Kind),
				lcNameStyle.Render(item.Name))
			if item.Namespace != "" {
				line += lcDimStyle.Render(fmt.Sprintf(" (%s)", item.Namespace))
			}
			line += lcDimStyle.Render(fmt.Sprintf(" — %s", item.Relation))
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n" + lcDimStyle.Render("↑/k ↓/j navigate · Enter select · Esc close") + "\n")

	return b.String()
}

// renderQuerySelector renders the saved query selection UI
func (m LocalClusterModel) renderQuerySelector() string {
	var b strings.Builder

	b.WriteString(lcHeaderStyle.Render("╭────────────────────────────────────────────────────────────────╮"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("│") + "  " + lcHeaderStyle.Render("🔍 SAVED QUERIES") + strings.Repeat(" ", 44) + lcHeaderStyle.Render("│"))
	b.WriteString("\n")
	b.WriteString(lcHeaderStyle.Render("╰────────────────────────────────────────────────────────────────╯"))
	b.WriteString("\n\n")

	b.WriteString(lcSectionStyle.Render("BUILT-IN QUERIES"))
	b.WriteString("\n")
	b.WriteString(lcDimStyle.Render("─────────────────────────────────────────────────────────────────"))
	b.WriteString("\n")

	// Column headers
	b.WriteString(fmt.Sprintf("  %-12s %-35s %s\n",
		lcDimStyle.Render("NAME"),
		lcDimStyle.Render("DESCRIPTION"),
		lcDimStyle.Render("MATCHES")))
	b.WriteString("\n")

	for i, q := range savedQueries {
		// Highlight cursor position
		cursor := "  "
		nameStyle := lcNameStyle
		if i == m.queryCursor {
			cursor = lcOkStyle.Render("▸ ")
			nameStyle = lcHeaderStyle
		}

		// Get match count
		matches := m.countQueryMatches(q.Query)

		// Color based on query type
		matchStyle := lcDimStyle
		switch q.Name {
		case "orphans":
			matchStyle = lcWarnStyle
		case "gitops", "confighub":
			matchStyle = lcOkStyle
		case "flux":
			matchStyle = lcCyanStyle
		case "argo":
			matchStyle = lcPurpleStyle
		case "helm":
			matchStyle = lcWarnStyle
		}

		b.WriteString(fmt.Sprintf("%s%-12s %-35s %s\n",
			cursor,
			nameStyle.Render(q.Name),
			lcDimStyle.Render(q.Description),
			matchStyle.Render(fmt.Sprintf("%d", matches))))
	}

	b.WriteString("\n")

	// Show current filter if active
	if m.activeQuery != nil {
		b.WriteString(lcSectionStyle.Render("CURRENT FILTER"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s: %s\n",
			lcNameStyle.Render(m.activeQuery.Name),
			lcCyanStyle.Render(m.activeQuery.Query)))
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(lcDimStyle.Render("─────────────────────────────────────────────────────────────────"))
	b.WriteString("\n")
	b.WriteString(lcDimStyle.Render("↑/↓ select  Enter apply  C clear  Esc cancel"))
	b.WriteString("\n")

	return b.String()
}

// LocalClusterResult holds the result of running the local cluster TUI
type LocalClusterResult struct {
	SwitchToHub    bool
	HubContext     string
	SwitchToImport bool
}

// runLocalClusterTUI launches the Go-native local cluster TUI
// Returns: (switchToHub, hubContext, switchToImport, error)
func runLocalClusterTUI() (bool, string, bool, error) {
	m := initialLocalModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return false, "", false, err
	}

	// Check if user wants to switch modes
	if fm, ok := finalModel.(LocalClusterModel); ok {
		return fm.switchToHub, fm.hubContext, fm.switchToImport, nil
	}

	return false, "", false, nil
}
