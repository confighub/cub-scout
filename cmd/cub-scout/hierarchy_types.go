// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	breadcrumbStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			MarginBottom(1)

	activeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	groupStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("141")).
			Bold(true)

	treeStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1, 2)

	statusOK = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82"))

	statusWarn = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	statusErr = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			MarginTop(1)

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	searchMatchStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("226")).
				Foreground(lipgloss.Color("0"))

	searchInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	// Split pane styles
	leftPaneStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	rightPaneStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	rightPaneActiveStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("212")).
				Padding(0, 1)

	detailsHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				MarginBottom(1)

	jsonKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81"))

	jsonStringStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("114"))

	jsonNumberStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	jsonBoolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("213"))

	// Help bar styles
	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("228")).
			Bold(true)

	helpActionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	helpDotStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

// titleCase converts a string to title case (first letter of each word capitalized)
var titleCaser = cases.Title(language.English)

func titleCase(s string) string {
	return titleCaser.String(s)
}

// renderStatusIcon returns a styled status icon with trailing space for the given status.
// Returns empty string for unknown status, allowing callers to handle their own defaults.
func renderStatusIcon(status string) string {
	switch status {
	case "ok":
		return statusOK.Render(iconCheckOK) + " "
	case "warn":
		return statusWarn.Render(iconCheckWarn) + " "
	case "error":
		return statusErr.Render(iconCheckErr) + " "
	default:
		return ""
	}
}

// Tree icons
const (
	iconExpanded  = "▼"
	iconCollapsed = "▶"
	iconBranch    = "├─"
	iconLast      = "└─"
	iconPipe      = "│ "
	iconSpace     = "  "
	iconActive    = "●"
	iconInactive  = "○"
	iconCheckOK   = "✓"
	iconCheckWarn = "⚠"
	iconCheckErr  = "✗"
	iconFolder    = "📁"
	iconUnit      = "📦"
	iconTarget    = "🎯"
	iconWorker    = "⚙"
)

// Data structures from cub CLI
type CubContext struct {
	Name       string `json:"name"`
	Coordinate struct {
		ServerURL      string `json:"serverURL"`
		OrganizationID string `json:"organizationID"`
	} `json:"coordinate"`
	Settings struct {
		DefaultSpace string `json:"defaultSpace"`
	} `json:"settings"`
}

type CubOrganization struct {
	OrganizationID string `json:"OrganizationID"`
	ExternalID     string `json:"ExternalID"`
	DisplayName    string `json:"DisplayName"`
	Slug           string `json:"Slug"`
}

type CubSpaceData struct {
	Space struct {
		Slug           string `json:"Slug"`
		SpaceID        string `json:"SpaceID"`
		OrganizationID string `json:"OrganizationID"`
	} `json:"Space"`
	TotalUnitCount         int            `json:"TotalUnitCount"`
	TotalBridgeWorkerCount int            `json:"TotalBridgeWorkerCount"`
	TargetCountByType      map[string]int `json:"TargetCountByToolchainType"`
}

type CubUnitData struct {
	Unit struct {
		Slug            string `json:"Slug"`
		HeadRevisionNum int    `json:"HeadRevisionNum"`
		LiveRevisionNum int    `json:"LiveRevisionNum"`
		ToolchainType   string `json:"ToolchainType"`
	} `json:"Unit"`
	Target struct {
		Slug          string `json:"Slug"`
		ProviderType  string `json:"ProviderType"`
		ToolchainType string `json:"ToolchainType"`
	} `json:"Target"`
	UnitStatus struct {
		Status       string `json:"Status"`
		SyncStatus   string `json:"SyncStatus"`
		Drift        string `json:"Drift"`
		Action       string `json:"Action"`
		ActionResult string `json:"ActionResult"`
	} `json:"UnitStatus"`
	BridgeWorker struct {
		Slug       string `json:"Slug"`
		Condition  string `json:"Condition"`
		IPAddress  string `json:"IPAddress"`
		LastSeenAt string `json:"LastSeenAt"`
	} `json:"BridgeWorker"`
	Space struct {
		Slug string `json:"Slug"`
	} `json:"Space"`
}

type CubTargetData struct {
	Target struct {
		Slug         string `json:"Slug"`
		ProviderType string `json:"ProviderType"`
	} `json:"Target"`
}

type CubWorkerData struct {
	BridgeWorker struct {
		Slug      string `json:"Slug"`
		Condition string `json:"Condition"`
	} `json:"BridgeWorker"`
}

// DeriveStatus returns the status string for a unit based on its state
func (u CubUnitData) DeriveStatus() string {
	if u.UnitStatus.Status == "Error" {
		return "error"
	}
	if u.UnitStatus.SyncStatus == "OutOfSync" || u.UnitStatus.Drift == "Drifted" {
		return "warn"
	}
	return "ok"
}

// DeriveStatus returns the status string for a worker based on its condition
func (w CubWorkerData) DeriveStatus() string {
	if w.BridgeWorker.Condition != "Ready" {
		return "warn"
	}
	return "ok"
}

// TreeNode represents a node in the hierarchy
type TreeNode struct {
	ID       string
	Name     string
	Type     string // "org", "space", "group", "unit", "target", "worker", "detail"
	Status   string // "ok", "warn", "error", "pending", ""
	Info     string // Description text
	Children []*TreeNode
	Parent   *TreeNode
	Expanded bool
	Data     interface{} // Original data
	OrgID    string      // For tracking which org this belongs to (for auth switching)
}

// PendingAction tracks optimistic UI state for CRUD operations
type PendingAction struct {
	ActionType string    // "creating" or "deleting"
	NodeType   string    // "space", "unit", "target"
	Name       string    // Name of the resource
	ParentID   string    // Parent node ID (space slug for units/targets, org for spaces)
	StartTime  time.Time // For timeout handling
}

// Import wizard steps
const (
	importStepSource    = iota // Choose import source: Kubernetes namespace or ArgoCD
	importStepNamespace        // Select Kubernetes namespace
	importStepArgoApps         // Select ArgoCD Application
	importStepSetup            // Choose to create space/target or use existing
	importStepCreateSpace
	importStepCreateWorker  // Create worker (target auto-created when worker runs)
	importStepWaitTarget    // Wait for target to be auto-created by running worker
	importStepDiscovering
	importStepSelection
	importStepUnitStructure // Choose combined vs individual units (ArgoCD only)
	importStepExtractConfig // Extract GitOps config from Argo/Flux/Helm
	importStepImporting
	importStepComplete
	importStepArgoCleanup // Offer to disable/delete ArgoCD Application (ArgoCD imports only)
	importStepTest        // Offer to test ConfigHub pipeline
)

// ArgoCD cleanup options
const (
	argoCleanupDisableSync = iota // Disable auto-sync, keep Application
	argoCleanupDeleteApp          // Delete Application entirely
	argoCleanupKeepAsIs           // Keep Application as-is
)

// Test pipeline options
const (
	testOptionTest = iota // Test with annotation update + rollout
	testOptionSkip        // Skip testing
)

// Unit structure options (ArgoCD imports)
const (
	unitStructureCombined   = iota // Single unit with all resources
	unitStructureIndividual        // One unit per resource
)

// Import source types
const (
	importSourceKubernetes = iota
	importSourceArgoCD
)

// Create wizard steps
const (
	createStepSelectType = iota // Select space/unit/target
	createStepEnterName         // Enter name
	createStepUnitMethod        // Clone or empty (for units)
	createStepSelectSource      // Select unit to clone (for units)
	createStepSelectTarget      // Select target (for units)
	createStepSelectWorker      // Select worker (for targets)
	createStepSelectProvider    // Select provider (for targets)
	createStepConfirm           // Confirm creation
	createStepCreating          // Creating in progress
	createStepComplete          // Done
)

// Delete wizard steps
const (
	deleteStepConfirm = iota // Confirm deletion
	deleteStepDeleting       // Deletion in progress
	deleteStepComplete       // Done
)

// Model represents the TUI state
type Model struct {
	nodes         []*TreeNode // Root nodes (organizations)
	flatList      []*TreeNode // Flattened visible list for navigation
	cursor        int         // Current selection
	width         int
	height        int
	ready         bool
	loading       bool
	err           error
	currentOrg    string // Current org ID (external ID)
	currentOrgInt string // Current org internal ID
	searchMode    bool
	searchQuery   string
	searchMatches []int              // Indices of matching nodes in flatList (direct matches only)
	searchIndex   int                // Current position in searchMatches
	filterActive  bool               // Whether filter mode is active (hides non-matching nodes)
	matchCache    map[*TreeNode]bool // Cache of nodes that match or have matching descendants
	keymap        keyMap
	authPrompt    bool   // Show auth prompt
	authOrgName   string // Org name to switch to
	authOrgID     string // Org ID to switch to
	statusMsg     string // Status message to display

	// Import wizard state
	importMode       bool
	importStep       int
	importNamespaces []namespaceInfo
	importShowAllNS  bool // Show all namespaces including system/empty
	importNamespace  string
	importSpace      string
	importWorkloads  []WorkloadInfo
	importSelected   []bool
	importCursor     int
	importLoading    bool
	importError      error
	importApplyError error // Non-nil if apply failed during import (unit created but no livedata)
	importProgress   int
	importTotal      int

	// Config extraction state
	importExtractDone    bool // Whether extraction has completed
	importExtractSuccess int  // Number of successful extractions
	importViewingConfig  bool // Whether user is viewing full config
	importViewConfigIdx  int  // Index of workload whose config is being viewed

	// Smart suggestions state
	importSuggestion  *ImportSuggestion // Smart structure suggestion from labels/namespace
	importGroupedView bool              // Toggle between grouped/flat view in selection

	// Setup wizard state (for creating space/target)
	importCreateNewSpace  bool         // true = create new space, false = use selected space
	importNewSpaceName    string       // name for new space
	importCreateTarget    bool         // whether to create a target
	importNewWorkerName   string       // name for new worker
	importNewTargetName   string       // name for new target (defaults to namespace name)
	importTargetParams    string       // JSON parameters for target
	importSetupChoice     int          // 0 = use existing, 1 = create new space
	importExistingSpaces  []string     // list of existing spaces
	importExistingWorkers []workerInfo // list of existing workers in selected space
	importSelectedWorker  string       // selected existing worker (for target creation)

	// ArgoCD import state
	importSource        int               // importSourceKubernetes or importSourceArgoCD
	importArgoApps      []argoAppInfo     // list of ArgoCD Applications
	importSelectedArgo  *argoAppInfo      // selected ArgoCD Application
	importArgoResources []ManagedResource // resources managed by selected ArgoCD app
	importArgoCleanup   int               // cleanup choice: argoCleanupDisableSync, argoCleanupDeleteApp, argoCleanupKeepAsIs
	importUnitStructure int               // unit structure: unitStructureCombined, unitStructureIndividual

	// Test pipeline state
	importTestChoice int               // test choice: testOptionTest, testOptionSkip
	importTestResult *TestUpdateResult // result of test
	importTestRan    bool              // whether the test was actually executed

	// Create wizard state
	createMode      bool             // Create wizard active
	createStep      int              // Current step in create wizard
	createType      string           // "space", "unit", or "target"
	createName      string           // Name being entered
	createSpace     string           // Space for unit/target creation
	createCloneFrom string           // Unit to clone from (for unit creation)
	createTarget    string           // Target to assign (for unit creation)
	createWorker    string           // Worker for target creation
	createProvider  string           // Provider type for target (default Kubernetes)
	createToolchain string           // Toolchain type for unit (for filtering targets)
	createCursor    int              // Cursor for selection lists
	createLoading   bool             // Loading state
	createError     error            // Error state
	createUnits     []createUnitInfo // Available units for cloning (with toolchain)
	createTargets   []string         // Available targets for assignment
	createWorkers   []string         // Available workers for target

	// Delete wizard state
	deleteMode    bool   // Delete wizard active
	deleteStep    int    // Current step in delete wizard
	deleteType    string // "space", "unit", or "target"
	deleteName    string // Name of resource to delete
	deleteSpace   string // Space containing the resource (for unit/target)
	deleteCursor  int    // Cursor for confirmation (0=yes, 1=no)
	deleteLoading bool   // Loading state
	deleteError   error  // Error state

	// Command palette state (: to open)
	cmdMode       bool     // Command mode active
	cmdInput      string   // Current command being typed
	cmdHistory    []string // Recent commands (max 20)
	cmdHistoryIdx int      // Position in history (-1 = current input)
	cmdOutput     string   // Output from last command
	cmdRunning    bool     // Command is running in background
	cmdShowOutput bool     // Show output panel

	// Worker status (shown in header)
	workers       []workerStatus // Current worker status
	workersLoaded bool           // Whether workers have been fetched

	// Org selector mode (O to open)
	orgSelectMode   bool // Org selection popup active
	orgSelectCursor int  // Cursor position in org list

	// Help overlay mode (? to open)
	helpMode bool // Help overlay active

	// Activity view mode (a to open)
	activityMode bool // Activity view active

	// Maps view mode (M to open)
	mapsMode bool // Three Maps view active

	// Panel view mode (P to open) - WET↔LIVE side-by-side
	panelMode        bool                  // Panel view active
	panelWorkloads   []MapEntry            // Cluster workloads (LIVE)
	panelLoading     bool                  // Loading cluster data
	panelError       error                 // Error fetching cluster data
	panelCorrelation map[string][]MapEntry // Unit slug → workloads (by confighub.com/UnitSlug label)
	panelOrphans     []MapEntry            // Workloads not in ConfigHub

	// Suggest view mode (g to open) - recommend Units from live resources
	suggestMode     bool                      // Suggest view active
	suggestLoading  bool                      // Loading cluster data
	suggestError    error                     // Error fetching data
	suggestProposal *HubAppSpaceSuggestion    // Generated suggestion
	suggestCursor   int                       // Cursor for navigation

	// Launch local cluster TUI flag (L to switch)
	launchLocalCluster bool // Switch to local cluster TUI on exit

	// Hub/AppSpace view mode (B to toggle)
	hubViewMode bool // Group spaces into Hub (platform) vs AppSpaces (apps)

	// Cluster filter mode (a to toggle)
	showAllUnits   bool   // false = filter to current cluster, true = show all units
	currentCluster string // Cluster name extracted from context
	contextName    string // Raw kubectl context name

	// Pending snapshot for restoring expanded paths after data loads
	pendingSnapshot *HubSnapshot

	// Optimistic UI state - tracks pending CRUD operations for immediate feedback
	pendingActions []PendingAction

	// Details pane state (right panel)
	detailsPane    viewport.Model // Scrollable viewport for right pane
	detailsContent string         // Current formatted content
	detailsNode    *TreeNode      // Node being displayed (nil = org summary)
	detailsLoading bool           // Async loading state
	detailsError   error          // Error from last fetch attempt
	detailsFocused bool           // Keyboard focus on details pane

	// UI components
	spinner spinner.Model // Loading spinner

	// Transition to new import wizard
	launchImportWizard bool // Set to true to exit and launch import wizard
}

// isCurrentOrg checks if the given organization is the currently selected org
func (m *Model) isCurrentOrg(org CubOrganization) bool {
	return org.ExternalID == m.currentOrg ||
		org.Slug == m.currentOrg ||
		org.OrganizationID == m.currentOrgInt
}

type keyMap struct {
	Up           key.Binding
	Down         key.Binding
	Left         key.Binding
	Right        key.Binding
	Enter        key.Binding
	Quit         key.Binding
	Search       key.Binding
	NextMatch    key.Binding
	PrevMatch    key.Binding
	ToggleFilter key.Binding
	Refresh      key.Binding
	Help         key.Binding
	Import       key.Binding
	Create       key.Binding
	Delete       key.Binding
	Tab          key.Binding
	OpenWeb      key.Binding
	Command      key.Binding
	SwitchOrg    key.Binding
	LocalCluster key.Binding
	Activity     key.Binding
	Maps         key.Binding
	Panel        key.Binding
	Suggest      key.Binding
	HubView      key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "collapse"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "expand"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "details"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		NextMatch: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		PrevMatch: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),
		ToggleFilter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "toggle filter"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Import: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "import"),
		),
		Create: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "create"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d", "x"),
			key.WithHelp("d/x", "delete"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch pane"),
		),
		OpenWeb: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "open in browser"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command"),
		),
		SwitchOrg: key.NewBinding(
			key.WithKeys("O"),
			key.WithHelp("O", "switch org"),
		),
		LocalCluster: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "local cluster"),
		),
		Activity: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "toggle filter"),
		),
		Maps: key.NewBinding(
			key.WithKeys("M"),
			key.WithHelp("M", "three maps"),
		),
		Panel: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "panel view"),
		),
		Suggest: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "suggest units"),
		),
		HubView: key.NewBinding(
			key.WithKeys("B"),
			key.WithHelp("B", "hub/appspace view"),
		),
	}
}

// Message types
type dataLoadedMsg struct {
	nodes         []*TreeNode
	currentOrg    string
	currentOrgInt string
	currentSpace  string // Auto-focus on this space at startup
	spacesToLoad  []string
}

type errMsg struct {
	err error
}

type authCompleteMsg struct {
	success bool
	orgID   string
}

type spaceDataLoadedMsg struct {
	spaceSlug string
	units     []CubUnitData
	targets   []CubTargetData
	workers   []CubWorkerData
	err       error
}

type panelDataLoadedMsg struct {
	workloads   []MapEntry            // All cluster workloads (LIVE)
	correlation map[string][]MapEntry // Unit slug → workloads
	orphans     []MapEntry            // Workloads not in ConfigHub
	err         error
}

type suggestDataLoadedMsg struct {
	proposal *HubAppSpaceSuggestion // Generated suggestion
	err      error
}

type detailsLoadedMsg struct {
	node    *TreeNode
	content string
	err     error
}

type namespaceInfo struct {
	Name        string
	Deployments int
	StatefulSet int
	DaemonSets  int
	// Owner counts
	FluxCount      int
	ArgoCount      int
	HelmCount      int
	ConfigHubCount int
	NativeCount    int
}

func (ns *namespaceInfo) hasWorkloads() bool {
	return ns.Deployments > 0 || ns.StatefulSet > 0 || ns.DaemonSets > 0
}

// padRight pads a styled string to a fixed display width
func padRight(s string, width int) string {
	displayWidth := lipgloss.Width(s)
	if displayWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-displayWidth)
}

func getFilteredNamespaces(namespaces []namespaceInfo, showAll bool) []namespaceInfo {
	if showAll {
		return namespaces
	}
	var filtered []namespaceInfo
	for _, ns := range namespaces {
		if isSystemNamespace(ns.Name) || !ns.hasWorkloads() {
			continue
		}
		filtered = append(filtered, ns)
	}
	return filtered
}

func countHiddenNamespaces(namespaces []namespaceInfo) int {
	hidden := 0
	for _, ns := range namespaces {
		if isSystemNamespace(ns.Name) || !ns.hasWorkloads() {
			hidden++
		}
	}
	return hidden
}

type argoAppInfo struct {
	Name         string
	Namespace    string
	Project      string
	RepoURL      string
	Path         string
	DestServer   string
	DestNS       string
	SyncStatus   string
	HealthStatus string
	IsAppOfApps  bool
	ChildApps    []string
}

// workerStatus represents a ConfigHub worker's connection status
type workerStatus struct {
	Name      string
	Slug      string
	Condition string // "Ready", "Disconnected", etc
	Space     string
}

// cmdCompleteMsg is sent when a command palette command finishes
type cmdCompleteMsg struct {
	output string
	err    error
}

// workersStatusMsg is sent when worker status is fetched
type workersStatusMsg struct {
	workers []workerStatus
	err     error
}

type namespacesLoadedMsg struct {
	namespaces []namespaceInfo
	err        error
}

type argoAppsLoadedMsg struct {
	apps []argoAppInfo
	err  error
}

type argoResourcesLoadedMsg struct {
	app       *argoAppInfo
	resources []ManagedResource
	appYAML   string
	err       error
}

type argoSyncDisabledMsg struct {
	appName string
	err     error
}

type argoAppDeletedMsg struct {
	appName string
	err     error
}

type unitAppliedMsg struct {
	unitSlug string
	err      error
}

type testUpdateCompleteMsg struct {
	result *TestUpdateResult
	err    error
}

type workloadsDiscoveredMsg struct {
	workloads []WorkloadInfo
	err       error
}

type importCompleteMsg struct {
	success    int
	failed     int
	applyError error
}

type configExtractedMsg struct {
	workloads []WorkloadInfo
	success   int
}

type spacesLoadedMsg struct {
	spaces []string
	err    error
}

type spaceCreatedMsg struct {
	space string
	err   error
}

type workerCreatedMsg struct {
	worker string
	err    error
}

type workerStartedMsg struct {
	worker string
	err    error
}

type workerReadyMsg struct {
	worker string
	err    error
}

type targetFoundMsg struct {
	target string
	err    error
}

type workersLoadedMsg struct {
	workers []workerInfo
	err     error
}

type createUnitsLoadedMsg struct {
	units []createUnitInfo
	err   error
}

type createTargetsLoadedMsg struct {
	targets []string
	err     error
}

type createWorkersLoadedMsg struct {
	workers []string
	err     error
}

type createResourceMsg struct {
	resourceType string
	name         string
	space        string
	err          error
}

type deleteResourceMsg struct {
	resourceType string
	name         string
	space        string
	err          error
}

type createUnitInfo struct {
	Slug      string
	Toolchain string
}

type workerInfo struct {
	Slug      string
	Condition string
}

// FilterValue implements list.Item for TreeNode
func (n TreeNode) FilterValue() string {
	return n.Name
}

// Title implements list.Item for TreeNode
func (n TreeNode) Title() string {
	return n.Name
}

// Description implements list.Item for TreeNode
func (n TreeNode) Description() string {
	return n.Info
}

// HubSnapshot represents saved Hub TUI state for resumption
type HubSnapshot struct {
	Version       string    `json:"version"`
	UpdatedAt     time.Time `json:"updated_at"`
	Cursor        int       `json:"cursor"`
	CurrentOrg    string    `json:"current_org,omitempty"`
	MapsMode      bool      `json:"maps_mode"`
	PanelMode     bool      `json:"panel_mode"`
	ExpandedPaths []string  `json:"expanded_paths,omitempty"` // Paths of expanded nodes
}

const hubSnapshotVersion = "1.0"

func getHubSnapshotPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".confighub", "sessions", "hub-snapshot.json")
}

func loadHubSnapshot() *HubSnapshot {
	path := getHubSnapshotPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var snap HubSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil
	}
	// Only restore if snapshot is less than 24 hours old
	if time.Since(snap.UpdatedAt) > 24*time.Hour {
		return nil
	}
	return &snap
}

func saveHubSnapshot(m *Model) {
	// Collect expanded node paths
	var expandedPaths []string
	var collectExpanded func(nodes []*TreeNode, path string)
	collectExpanded = func(nodes []*TreeNode, path string) {
		for _, n := range nodes {
			nodePath := path + "/" + n.Name
			if n.Expanded && len(n.Children) > 0 {
				expandedPaths = append(expandedPaths, nodePath)
				collectExpanded(n.Children, nodePath)
			}
		}
	}
	collectExpanded(m.nodes, "")

	snap := HubSnapshot{
		Version:       hubSnapshotVersion,
		UpdatedAt:     time.Now(),
		Cursor:        m.cursor,
		CurrentOrg:    m.currentOrg,
		MapsMode:      m.mapsMode,
		PanelMode:     m.panelMode,
		ExpandedPaths: expandedPaths,
	}

	path := getHubSnapshotPath()
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

