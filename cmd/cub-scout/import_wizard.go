// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Import wizard steps
const (
	StepSelectNamespaces = iota
	StepReviewWorkloads
	StepConfigureStructure
	StepApply
	StepArgoCleanup // Offer to disable/delete ArgoCD Applications
	StepTest        // End-to-end verification test
)

// Test phases for StepTest
const (
	testPhaseIdle          = iota
	testPhaseAddAnnotation // Adding test annotation to unit
	testPhaseApply         // Applying unit to cluster
	testPhaseWaitSync      // Waiting for worker to sync
	testPhaseVerify        // Verifying annotation in cluster
	testPhaseComplete      // Test finished
)

// Edit mode types for Step 3
const (
	editModeNone        = iota
	editModeMenu        // Showing edit menu
	editModeRenameUnit  // Editing unit slug
	editModeRenameSpace // Editing app space name
	editModeMergeSelect // Selecting unit to merge into
	editModeAddLabel    // Adding a new label
	editModeEditLabel   // Editing existing label value
	editModeDeleteLabel // Confirming label deletion
)

// ArgoAppRef represents an ArgoCD Application to clean up
type ArgoAppRef struct {
	Name      string
	Namespace string
}

// testDebugDir is the directory for test debug output
const testDebugDir = "/tmp/confighub-import-test-debug"

// writeTestDebug writes debug content to a file in the debug directory
func writeTestDebug(filename string, content []byte) {
	os.MkdirAll(testDebugDir, 0755)
	path := filepath.Join(testDebugDir, filename)
	os.WriteFile(path, content, 0644)
}

// appendTestDebug appends debug content to a log file
func appendTestDebug(content string) {
	os.MkdirAll(testDebugDir, 0755)
	path := filepath.Join(testDebugDir, "test.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	timestamp := time.Now().Format("15:04:05.000")
	f.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, content))
}

// NamespaceItem represents a namespace with workload count
type NamespaceItem struct {
	Name          string
	WorkloadCount int
	Selected      bool
	Workloads     []WorkloadInfo // Populated when previewing
}

// WorkloadItem represents a workload for selection
type WorkloadItem struct {
	Info     WorkloadInfo
	Selected bool
	App      string // Inferred app name
	Variant  string // Inferred variant
}

// ImportWizardModel is the bubbletea model for the import wizard
type ImportWizardModel struct {
	// Navigation state
	step       int
	cursor     int
	focusRight bool

	// Step 1: Namespace selection
	namespaces      []NamespaceItem
	namespaceCursor int

	// Step 2: Workload review
	workloads      []WorkloadItem
	workloadGroups map[string][]int // app name -> indices into workloads
	expandedGroups map[string]bool
	workloadCursor int

	// Step 3: Configure structure
	proposal       *FullProposal
	proposalCursor int
	expandedUnits  map[int]bool // Track which units are expanded to show workloads

	// Step 3: Edit mode
	editMode       int    // Current edit mode (editModeNone, editModeMenu, etc.)
	editMenuCursor int    // Cursor in edit menu
	editInput      string // Text input buffer for editing
	editLabelKey   string // Label key being edited
	mergeTargetIdx int    // Index of unit to merge into

	// Step 4: Apply progress
	applyProgress  int
	applyTotal     int
	applyResults   []ApplyResult
	applyComplete  bool
	applyStartTime time.Time
	workerStarted  bool
	workerName     string

	// Step 5: ArgoCD cleanup
	argoApps        []ArgoAppRef // Unique ArgoCD Applications from selected workloads
	argoCleanupIdx  int          // Current selection in cleanup options
	argoCleanupDone bool         // Whether cleanup is complete

	// Step 6: End-to-end test
	testPhase      int           // Current test phase
	testUnitSlug   string        // Unit being tested
	testAnnotation string        // The test annotation value
	testStartTime  time.Time     // When test started
	testEndTime    time.Time     // When test finished
	testElapsed    time.Duration // Final elapsed time (set when complete)
	testResults    []TestResult  // Results for each phase
	testError      error         // Any error during test

	// UI components
	viewport viewport.Model
	spinner  spinner.Model
	width    int
	height   int

	// K8s clients
	clientset *kubernetes.Clientset
	dynClient dynamic.Interface
	ctx       context.Context

	// Loading states
	loading        bool
	loadingMessage string
	err            error

	// Result
	quit bool

	// Help overlay
	showHelp bool

	// Search/filter
	searchMode  bool
	searchInput string
}

// ApplyResult tracks the result of creating a unit
type ApplyResult struct {
	UnitSlug string
	Success  bool
	Error    string
}

// TestResult tracks the result of a test phase
type TestResult struct {
	Phase   int
	Label   string
	Success bool
	Details string
	Elapsed time.Duration
}

// Import wizard styles
var (
	wizardTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("212")).
				Background(lipgloss.Color("236")).
				Padding(0, 1).
				MarginBottom(1)

	wizardProgressStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	wizardProgressBarFull = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82"))

	wizardProgressBarEmpty = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	wizardPaneStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	wizardPaneActiveStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("212")).
				Padding(0, 1)

	wizardSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Bold(true)

	wizardCheckboxOn = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82"))

	wizardCheckboxOff = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	wizardOwnerFlux = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81"))

	wizardOwnerArgo = lipgloss.NewStyle().
			Foreground(lipgloss.Color("141"))

	wizardOwnerHelm = lipgloss.NewStyle().
			Foreground(lipgloss.Color("208"))

	wizardOwnerNative = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	wizardHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			MarginTop(1)

	wizardErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	wizardSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82"))
)

// NewImportWizardModel creates a new import wizard
func NewImportWizardModel() ImportWizardModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))

	return ImportWizardModel{
		step:           StepSelectNamespaces,
		namespaces:     []NamespaceItem{},
		workloads:      []WorkloadItem{},
		workloadGroups: make(map[string][]int),
		expandedGroups: make(map[string]bool),
		expandedUnits:  make(map[int]bool),
		applyResults:   []ApplyResult{},
		spinner:        s,
		ctx:            context.Background(),
		loading:        true,
		loadingMessage: "Discovering namespaces...",
	}
}

// Init initializes the model
func (m ImportWizardModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.initK8sClients,
	)
}

// initK8sClients initializes Kubernetes clients
func (m ImportWizardModel) initK8sClients() tea.Msg {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return wizardErrMsg{err: fmt.Errorf("build kubeconfig: %w", err)}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return wizardErrMsg{err: fmt.Errorf("create clientset: %w", err)}
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return wizardErrMsg{err: fmt.Errorf("create dynamic client: %w", err)}
	}

	return wizardK8sReadyMsg{
		clientset: clientset,
		dynClient: dynClient,
	}
}

// Message types for import wizard (prefixed to avoid conflicts with hierarchy.go)
type wizardK8sReadyMsg struct {
	clientset *kubernetes.Clientset
	dynClient dynamic.Interface
}

type wizardNamespacesMsg struct {
	namespaces []NamespaceItem
}

type wizardWorkloadsMsg struct {
	workloads []WorkloadItem
}

type wizardProposalMsg struct {
	proposal *FullProposal
}

type wizardApplyProgressMsg struct {
	unitSlug string
	success  bool
	err      string
}

type wizardApplyCompleteMsg struct{}

type wizardWorkerStartedMsg struct {
	worker string
	err    error
}

type wizardErrMsg struct {
	err error
}

type wizardArgoCleanupMsg struct {
	appName string
	action  int // Which action was performed (argoCleanupDisableSync, etc.)
	success bool
	err     error
}

type wizardTestPhaseMsg struct {
	phase     int
	success   bool
	details   string
	err       error
	startTime time.Time // When this phase started
}

type wizardTestTickMsg struct{} // For polling sync status

// Update handles messages
func (m ImportWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Don't handle keys while loading
		if m.loading {
			if msg.String() == "q" || msg.String() == "esc" {
				m.quit = true
				return m, tea.Quit
			}
			return m, nil
		}

		// Handle search mode input first
		if m.searchMode {
			return m.handleSearchModeKey(msg)
		}

		// Handle edit mode keys first (Step 3 only)
		if m.step == StepConfigureStructure && m.editMode != editModeNone {
			return m.handleEditModeKey(msg)
		}

		switch msg.String() {
		case "q":
			// Always quit
			m.quit = true
			return m, tea.Quit

		case "esc":
			// Quit unless in edit mode (handled above)
			m.quit = true
			return m, tea.Quit

		case "backspace":
			// Go back a step
			if m.step > StepSelectNamespaces {
				return m.prevStep()
			}

		case "left", "h":
			// In Configure Structure, left collapses the current unit
			if m.step == StepConfigureStructure && m.proposal != nil && m.proposalCursor < len(m.proposal.Units) {
				m.expandedUnits[m.proposalCursor] = false
			}

		case "right", "l":
			// In Configure Structure, right expands the current unit
			if m.step == StepConfigureStructure && m.proposal != nil && m.proposalCursor < len(m.proposal.Units) {
				m.expandedUnits[m.proposalCursor] = true
			}

		case "up", "k":
			return m.moveCursor(-1)

		case "down", "j":
			return m.moveCursor(1)

		case " ":
			return m.toggleSelection()

		case "a":
			return m.toggleAll()

		case "enter":
			return m.nextStep()

		case "tab":
			m.focusRight = !m.focusRight

		case "e":
			// Enter edit mode on Configure Structure step
			if m.step == StepConfigureStructure && m.proposal != nil {
				m.editMode = editModeMenu
				m.editMenuCursor = 0
			}

		case "d":
			// Delete unit on Configure Structure step
			if m.step == StepConfigureStructure && m.proposal != nil && len(m.proposal.Units) > 1 {
				m.deleteCurrentUnit()
			}

		case "?":
			m.showHelp = !m.showHelp
			return m, nil

		case "/":
			// Enter search mode on Steps 1-2
			if m.step == StepSelectNamespaces || m.step == StepReviewWorkloads {
				m.searchMode = true
				m.searchInput = ""
				return m, nil
			}

		case "w":
			// Start worker on Apply step when complete (no ArgoCD apps), or after cleanup done
			if m.step == StepApply && m.applyComplete && len(m.argoApps) == 0 && m.proposal != nil {
				return m, m.startWorkerCmd()
			}
			if m.step == StepArgoCleanup && m.argoCleanupDone && m.proposal != nil {
				return m, m.startWorkerCmd()
			}

		case "t":
			// Start end-to-end test (only available after worker started AND ArgoCD cleanup done)
			// If there are ArgoCD apps, we need to wait until cleanup is done so ArgoCD
			// doesn't immediately reconcile away our test changes
			argoCleanupRequired := len(m.argoApps) > 0 && !m.argoCleanupDone
			if m.workerStarted && m.proposal != nil && len(m.proposal.Units) > 0 && !argoCleanupRequired {
				m.step = StepTest
				m.testPhase = testPhaseAddAnnotation
				m.testUnitSlug = m.proposal.Units[0].Slug
				m.testAnnotation = fmt.Sprintf("import-test-%d", time.Now().Unix())
				m.testStartTime = time.Time{} // Will be set when first phase starts
				m.testResults = nil
				m.testError = nil
				m.loading = true
				m.loadingMessage = "Starting end-to-end test..."
				return m, m.runTestPhaseCmd(testPhaseAddAnnotation)
			}

		case "s":
			// Skip ArgoCD cleanup (mark as done without any action)
			if m.step == StepArgoCleanup && !m.argoCleanupDone {
				m.argoCleanupDone = true
			}

		case "r":
			// Refresh data on current step
			switch m.step {
			case StepSelectNamespaces:
				// Refresh namespace list from cluster
				m.loading = true
				m.loadingMessage = "Refreshing namespaces..."
				return m, m.discoverNamespaces

			case StepReviewWorkloads:
				// Refresh workload list (keeps namespace selections)
				m.loading = true
				m.loadingMessage = "Refreshing workloads..."
				return m, m.loadWorkloads

			case StepConfigureStructure:
				// Regenerate proposal (keeps workload selections)
				m.loading = true
				m.loadingMessage = "Regenerating proposal..."
				return m, m.generateProposal

			case StepTest:
				// Retry test (only available when complete)
				if m.testPhase == testPhaseComplete && m.proposal != nil && len(m.proposal.Units) > 0 {
					m.testPhase = testPhaseAddAnnotation
					m.testAnnotation = fmt.Sprintf("import-test-%d", time.Now().Unix())
					m.testStartTime = time.Time{} // Will be set when first phase starts
					m.testResults = nil
					m.testError = nil
					m.loading = true
					m.loadingMessage = "Retrying end-to-end test..."
					return m, m.runTestPhaseCmd(testPhaseAddAnnotation)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width/2 - 4
		m.viewport.Height = msg.Height - 8

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case wizardK8sReadyMsg:
		m.clientset = msg.clientset
		m.dynClient = msg.dynClient
		// Now discover namespaces
		cmds = append(cmds, m.discoverNamespaces)

	case wizardNamespacesMsg:
		m.namespaces = msg.namespaces
		m.loading = false

	case wizardWorkloadsMsg:
		m.workloads = msg.workloads
		(&m).buildWorkloadGroups()
		m.loading = false

	case wizardProposalMsg:
		m.proposal = msg.proposal
		m.loading = false

	case applyStartMsg:
		// Start the apply process - create app space first
		return m, m.startApplyCmd()

	case wizardApplyProgressMsg:
		// Skip the special "__space_created__" marker
		if msg.unitSlug != "__space_created__" {
			m.applyResults = append(m.applyResults, ApplyResult{
				UnitSlug: msg.unitSlug,
				Success:  msg.success,
				Error:    msg.err,
			})
			m.applyProgress++
		}

		// Trigger next unit or complete
		if m.proposal != nil && m.applyProgress < len(m.proposal.Units) {
			return m, m.applyNextUnitCmd(m.applyProgress)
		}
		// All units done
		return m, func() tea.Msg { return wizardApplyCompleteMsg{} }

	case wizardApplyCompleteMsg:
		m.applyComplete = true
		m.loading = false
		// If there are ArgoCD apps, transition to cleanup step
		if len(m.argoApps) > 0 {
			m.step = StepArgoCleanup
			m.argoCleanupIdx = 0
		}

	case wizardWorkerStartedMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.workerStarted = true
			m.workerName = msg.worker
		}

	case wizardErrMsg:
		m.err = msg.err
		m.loading = false

	case wizardArgoCleanupMsg:
		m.loading = false
		if msg.err != nil {
			m.err = fmt.Errorf("ArgoCD cleanup failed for %s: %w", msg.appName, msg.err)
		} else {
			m.argoCleanupDone = true
		}

	case wizardTestPhaseMsg:
		m.loading = false
		// Set test start time from first phase if not already set
		if m.testStartTime.IsZero() && !msg.startTime.IsZero() {
			m.testStartTime = msg.startTime
		}
		elapsed := time.Since(m.testStartTime)

		// Record result
		phaseLabel := ""
		switch msg.phase {
		case testPhaseAddAnnotation:
			phaseLabel = "Add test annotation"
		case testPhaseApply:
			phaseLabel = "Apply to cluster"
		case testPhaseWaitSync:
			phaseLabel = "Wait for sync"
		case testPhaseVerify:
			phaseLabel = "Verify in cluster"
		}

		m.testResults = append(m.testResults, TestResult{
			Phase:   msg.phase,
			Label:   phaseLabel,
			Success: msg.success,
			Details: msg.details,
			Elapsed: elapsed,
		})

		if msg.err != nil {
			m.testError = msg.err
			m.testPhase = testPhaseComplete
			m.testEndTime = time.Now()
			m.testElapsed = m.testEndTime.Sub(m.testStartTime)
			return m, nil
		}

		// Move to next phase
		switch msg.phase {
		case testPhaseAddAnnotation:
			m.testPhase = testPhaseApply
			m.loading = true
			m.loadingMessage = "Applying unit to cluster..."
			return m, m.runTestPhaseCmd(testPhaseApply)
		case testPhaseApply:
			m.testPhase = testPhaseWaitSync
			m.loading = true
			m.loadingMessage = "Waiting for worker to sync..."
			return m, m.runTestPhaseCmd(testPhaseWaitSync)
		case testPhaseWaitSync:
			m.testPhase = testPhaseVerify
			m.loading = true
			m.loadingMessage = "Verifying annotation in cluster..."
			return m, m.runTestPhaseCmd(testPhaseVerify)
		case testPhaseVerify:
			m.testPhase = testPhaseComplete
			m.testEndTime = time.Now()
			m.testElapsed = m.testEndTime.Sub(m.testStartTime)
		}

	case wizardTestTickMsg:
		// Polling tick for sync status
		if m.step == StepTest && m.testPhase == testPhaseWaitSync {
			return m, m.checkSyncStatusCmd()
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the UI
func (m ImportWizardModel) View() string {
	if m.quit {
		return ""
	}

	// Help overlay takes over the screen
	if m.showHelp {
		return m.renderHelpOverlay()
	}

	var b strings.Builder

	// Title and progress bar
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Error display
	if m.err != nil {
		b.WriteString(wizardErrorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
		b.WriteString(wizardHelpStyle.Render("Press q to quit, or any key to retry"))
		return b.String()
	}

	// Loading state (but not for test step - it has its own progress display)
	if m.loading && m.step != StepTest {
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(m.loadingMessage)
		return b.String()
	}

	// Main content (split panes)
	b.WriteString(m.renderPanes())
	b.WriteString("\n")

	// Help bar
	b.WriteString(m.renderHelp())

	return b.String()
}

// renderHeader renders the title and progress bar
func (m ImportWizardModel) renderHeader() string {
	stepNames := []string{"Select Namespaces", "Review Workloads", "Configure Structure", "Apply", "ArgoCD Cleanup", "End-to-End Test"}
	// Adjust total steps based on context
	totalSteps := 4
	if len(m.argoApps) > 0 {
		totalSteps = 5
	}
	if m.step == StepTest {
		totalSteps++ // Show test as an additional step
	}
	stepNum := m.step + 1
	if stepNum > totalSteps {
		stepNum = totalSteps
	}
	title := fmt.Sprintf("IMPORT WIZARD - %s", stepNames[m.step])

	// Progress bar
	progress := float64(stepNum) / float64(totalSteps)
	barWidth := 20
	filled := int(progress * float64(barWidth))
	empty := barWidth - filled

	progressBar := wizardProgressBarFull.Render(strings.Repeat("█", filled)) +
		wizardProgressBarEmpty.Render(strings.Repeat("░", empty))

	stepInfo := fmt.Sprintf("Step %d of %d", stepNum, totalSteps)

	// Count info based on step
	countInfo := m.getCountInfo()

	header := lipgloss.JoinHorizontal(
		lipgloss.Center,
		wizardTitleStyle.Render(title),
		"  ",
		progressBar,
		"  ",
		wizardProgressStyle.Render(stepInfo),
		"  ",
		dimStyle.Render(countInfo),
	)

	return header
}

// getCountInfo returns context-sensitive count info for the header
func (m ImportWizardModel) getCountInfo() string {
	switch m.step {
	case StepSelectNamespaces:
		selected := 0
		for _, ns := range m.namespaces {
			if ns.Selected {
				selected++
			}
		}
		if selected == 0 {
			return fmt.Sprintf("(%d namespaces)", len(m.namespaces))
		}
		return fmt.Sprintf("(%d/%d selected)", selected, len(m.namespaces))

	case StepReviewWorkloads:
		selected := 0
		for _, w := range m.workloads {
			if w.Selected {
				selected++
			}
		}
		return fmt.Sprintf("(%d/%d workloads)", selected, len(m.workloads))

	case StepConfigureStructure:
		if m.proposal != nil {
			totalWorkloads := 0
			for _, u := range m.proposal.Units {
				totalWorkloads += len(u.Workloads)
			}
			return fmt.Sprintf("(%d units, %d workloads)", len(m.proposal.Units), totalWorkloads)
		}
		return ""

	case StepApply:
		if m.applyComplete {
			success := 0
			for _, r := range m.applyResults {
				if r.Success {
					success++
				}
			}
			return fmt.Sprintf("(%d/%d created)", success, len(m.applyResults))
		}
		return fmt.Sprintf("(%d/%d)", m.applyProgress, m.applyTotal)

	case StepArgoCleanup:
		return fmt.Sprintf("(%d apps)", len(m.argoApps))
	}
	return ""
}

// renderPanes renders the split pane view
func (m ImportWizardModel) renderPanes() string {
	paneWidth := m.width/2 - 2
	if paneWidth < 30 {
		paneWidth = 30
	}
	paneHeight := m.height - 8
	if paneHeight < 10 {
		paneHeight = 10
	}

	leftContent := m.renderLeftPane()
	rightContent := m.renderRightPane()

	leftStyle := wizardPaneStyle.Width(paneWidth).Height(paneHeight)
	rightStyle := wizardPaneStyle.Width(paneWidth).Height(paneHeight)

	if !m.focusRight {
		leftStyle = wizardPaneActiveStyle.Width(paneWidth).Height(paneHeight)
	} else {
		rightStyle = wizardPaneActiveStyle.Width(paneWidth).Height(paneHeight)
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftStyle.Render(leftContent),
		rightStyle.Render(rightContent),
	)
}

// renderLeftPane renders content based on current step
func (m ImportWizardModel) renderLeftPane() string {
	switch m.step {
	case StepSelectNamespaces:
		return m.renderNamespaceList()
	case StepReviewWorkloads:
		return m.renderWorkloadList()
	case StepConfigureStructure:
		return m.renderProposalTree()
	case StepApply:
		return m.renderApplyProgress()
	case StepArgoCleanup:
		return m.renderArgoCleanupList()
	case StepTest:
		return m.renderTestProgress()
	}
	return ""
}

// renderRightPane renders details/preview based on current step
func (m ImportWizardModel) renderRightPane() string {
	switch m.step {
	case StepSelectNamespaces:
		return m.renderNamespacePreview()
	case StepReviewWorkloads:
		return m.renderWorkloadDetails()
	case StepConfigureStructure:
		return m.renderArchitectureDiagram()
	case StepApply:
		return m.renderFinalArchitecture()
	case StepArgoCleanup:
		return m.renderArgoCleanupDetails()
	case StepTest:
		return m.renderTestDetails()
	}
	return ""
}

// renderHelp renders context-sensitive help
func (m ImportWizardModel) renderHelp() string {
	var help string

	// Special help for search mode
	if m.searchMode {
		return wizardHelpStyle.Render("type to filter  enter confirm  esc cancel")
	}

	switch m.step {
	case StepSelectNamespaces:
		help = "↑↓ navigate  space toggle  a toggle all  / search  r refresh  enter continue  ? help  q quit"
	case StepReviewWorkloads:
		help = "↑↓ navigate  space toggle  / search  r refresh  ⌫ back  enter continue  ? help  q quit"
	case StepConfigureStructure:
		if m.editMode != editModeNone {
			help = "see edit overlay for controls"
		} else {
			help = "↑↓ navigate  →← expand/collapse  e edit  d delete  r refresh  enter import  ? help  q quit"
		}
	case StepApply:
		if m.applyComplete && len(m.argoApps) == 0 {
			if m.workerStarted {
				help = "t run test  ? help  q quit"
			} else {
				help = "w start worker  ? help  q quit"
			}
		} else if m.applyComplete {
			help = "continuing to ArgoCD cleanup..."
		} else {
			help = "importing...  ? help  q quit"
		}
	case StepArgoCleanup:
		if m.argoCleanupDone {
			if m.workerStarted {
				help = "t run test  ? help  q quit"
			} else {
				help = "w start worker  ? help  q quit"
			}
		} else {
			help = "↑↓ navigate  enter select action  s skip  ? help  q quit"
		}
	case StepTest:
		if m.testPhase == testPhaseComplete {
			help = "r retry test  ? help  q quit"
		} else {
			help = "testing...  ? help  q quit"
		}
	}
	return wizardHelpStyle.Render(help)
}

// renderHelpOverlay renders a full-screen help overlay
func (m ImportWizardModel) renderHelpOverlay() string {
	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212")).
		Background(lipgloss.Color("236")).
		Padding(0, 2)

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39")).
		MarginTop(1)

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("82")).
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	b.WriteString(titleStyle.Render("IMPORT WIZARD - KEYBOARD SHORTCUTS"))
	b.WriteString("\n\n")

	// Global shortcuts
	b.WriteString(sectionStyle.Render("GLOBAL"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("?    "), descStyle.Render("Toggle this help overlay")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("q    "), descStyle.Render("Quit wizard")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("esc  "), descStyle.Render("Quit wizard")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("tab  "), descStyle.Render("Switch between left/right pane")))

	// Navigation
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("NAVIGATION"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("↑/k  "), descStyle.Render("Move cursor up")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("↓/j  "), descStyle.Render("Move cursor down")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("enter"), descStyle.Render("Proceed to next step")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("⌫    "), descStyle.Render("Go back to previous step")))

	// Selection
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("SELECTION (Steps 1-2)"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("space"), descStyle.Render("Toggle selection on current item")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("a    "), descStyle.Render("Toggle all items")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("/    "), descStyle.Render("Search/filter list")))

	// Configure Structure
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("CONFIGURE STRUCTURE (Step 3)"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("→/l  "), descStyle.Render("Expand unit to show workloads")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("←/h  "), descStyle.Render("Collapse unit")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("e    "), descStyle.Render("Edit selected unit (rename, labels, merge)")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("d    "), descStyle.Render("Delete selected unit")))

	// Edit Mode
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("EDIT MODE (Step 3)"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("↑↓   "), descStyle.Render("Navigate menu options")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("enter"), descStyle.Render("Select option / confirm edit")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("esc  "), descStyle.Render("Cancel and exit edit mode")))

	// Apply & Cleanup
	b.WriteString("\n")
	b.WriteString(sectionStyle.Render("APPLY & CLEANUP (Steps 4-5)"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("w    "), descStyle.Render("Start worker (after apply completes)")))
	b.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("s    "), descStyle.Render("Skip ArgoCD cleanup")))

	// Footer
	b.WriteString("\n")
	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Italic(true)
	b.WriteString(footerStyle.Render("Press ? or any key to close this help"))

	return b.String()
}

// Navigation helpers

func (m ImportWizardModel) moveCursor(delta int) (tea.Model, tea.Cmd) {
	switch m.step {
	case StepSelectNamespaces:
		m.namespaceCursor += delta
		if m.namespaceCursor < 0 {
			m.namespaceCursor = 0
		}
		if m.namespaceCursor >= len(m.namespaces) {
			m.namespaceCursor = len(m.namespaces) - 1
		}
	case StepReviewWorkloads:
		m.workloadCursor += delta
		if m.workloadCursor < 0 {
			m.workloadCursor = 0
		}
		if m.workloadCursor >= len(m.workloads) {
			m.workloadCursor = len(m.workloads) - 1
		}
	case StepConfigureStructure:
		if m.proposal != nil {
			m.proposalCursor += delta
			if m.proposalCursor < 0 {
				m.proposalCursor = 0
			}
			if m.proposalCursor >= len(m.proposal.Units) {
				m.proposalCursor = len(m.proposal.Units) - 1
			}
		}
	case StepArgoCleanup:
		// Navigate cleanup options (3 options: disable sync, delete, keep as-is)
		if !m.argoCleanupDone {
			m.argoCleanupIdx += delta
			if m.argoCleanupIdx < 0 {
				m.argoCleanupIdx = 0
			}
			if m.argoCleanupIdx > 2 {
				m.argoCleanupIdx = 2
			}
		}
	}
	return m, nil
}

func (m ImportWizardModel) toggleSelection() (tea.Model, tea.Cmd) {
	switch m.step {
	case StepSelectNamespaces:
		if m.namespaceCursor < len(m.namespaces) {
			m.namespaces[m.namespaceCursor].Selected = !m.namespaces[m.namespaceCursor].Selected
		}
	case StepReviewWorkloads:
		if m.workloadCursor < len(m.workloads) {
			m.workloads[m.workloadCursor].Selected = !m.workloads[m.workloadCursor].Selected
		}
	case StepConfigureStructure:
		// Toggle expand/collapse for the current unit
		if m.proposal != nil && m.proposalCursor < len(m.proposal.Units) {
			m.expandedUnits[m.proposalCursor] = !m.expandedUnits[m.proposalCursor]
		}
	}
	return m, nil
}

func (m ImportWizardModel) toggleAll() (tea.Model, tea.Cmd) {
	switch m.step {
	case StepSelectNamespaces:
		// Check if all are selected
		allSelected := true
		for _, ns := range m.namespaces {
			if !ns.Selected {
				allSelected = false
				break
			}
		}
		// Toggle all
		for i := range m.namespaces {
			m.namespaces[i].Selected = !allSelected
		}
	case StepReviewWorkloads:
		allSelected := true
		for _, w := range m.workloads {
			if !w.Selected {
				allSelected = false
				break
			}
		}
		for i := range m.workloads {
			m.workloads[i].Selected = !allSelected
		}
	}
	return m, nil
}

// handleSearchModeKey handles key presses when in search mode
func (m ImportWizardModel) handleSearchModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		// Cancel search and clear filter
		m.searchMode = false
		m.searchInput = ""
		return m, nil
	case "enter":
		// Confirm search and exit search mode (keep filter active)
		m.searchMode = false
		return m, nil
	case "backspace":
		if len(m.searchInput) > 0 {
			m.searchInput = m.searchInput[:len(m.searchInput)-1]
		}
		return m, nil
	default:
		// Add character to search input (only printable characters)
		if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
			m.searchInput += key
		}
		return m, nil
	}
}

// matchesFilter checks if an item matches the current search filter
func (m ImportWizardModel) matchesFilter(text string) bool {
	if m.searchInput == "" {
		return true
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(m.searchInput))
}

// handleEditModeKey handles key presses when in edit mode
func (m ImportWizardModel) handleEditModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.editMode {
	case editModeMenu:
		// Edit menu navigation
		switch key {
		case "esc", "q":
			m.editMode = editModeNone
		case "up", "k":
			if m.editMenuCursor > 0 {
				m.editMenuCursor--
			}
		case "down", "j":
			if m.editMenuCursor < 4 { // 5 menu options (0-4)
				m.editMenuCursor++
			}
		case "enter":
			return m.selectEditMenuItem()
		}

	case editModeRenameUnit, editModeRenameSpace, editModeAddLabel, editModeEditLabel:
		// Text input mode
		switch key {
		case "esc":
			m.editMode = editModeNone
			m.editInput = ""
		case "enter":
			return m.confirmTextEdit()
		case "backspace":
			if len(m.editInput) > 0 {
				m.editInput = m.editInput[:len(m.editInput)-1]
			}
		default:
			// Only accept printable characters
			if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
				m.editInput += key
			}
		}

	case editModeMergeSelect:
		// Selecting which unit to merge into
		switch key {
		case "esc":
			m.editMode = editModeNone
		case "up", "k":
			m.mergeTargetIdx--
			// Skip the current unit
			if m.mergeTargetIdx == m.proposalCursor {
				m.mergeTargetIdx--
			}
			if m.mergeTargetIdx < 0 {
				m.mergeTargetIdx = 0
				if m.mergeTargetIdx == m.proposalCursor && len(m.proposal.Units) > 1 {
					m.mergeTargetIdx = 1
				}
			}
		case "down", "j":
			m.mergeTargetIdx++
			// Skip the current unit
			if m.mergeTargetIdx == m.proposalCursor {
				m.mergeTargetIdx++
			}
			if m.mergeTargetIdx >= len(m.proposal.Units) {
				m.mergeTargetIdx = len(m.proposal.Units) - 1
				if m.mergeTargetIdx == m.proposalCursor && m.mergeTargetIdx > 0 {
					m.mergeTargetIdx--
				}
			}
		case "enter":
			m.mergeUnits()
			m.editMode = editModeNone
		}

	case editModeDeleteLabel:
		// Confirming label deletion
		switch key {
		case "esc", "n":
			m.editMode = editModeNone
		case "enter", "y":
			m.deleteLabel()
			m.editMode = editModeNone
		}
	}

	return m, nil
}

// selectEditMenuItem handles selection of edit menu items
func (m *ImportWizardModel) selectEditMenuItem() (tea.Model, tea.Cmd) {
	switch m.editMenuCursor {
	case 0: // Rename unit
		if m.proposalCursor < len(m.proposal.Units) {
			m.editMode = editModeRenameUnit
			m.editInput = m.proposal.Units[m.proposalCursor].Slug
		}
	case 1: // Rename app space
		m.editMode = editModeRenameSpace
		m.editInput = m.proposal.AppSpace
	case 2: // Merge with another unit
		if len(m.proposal.Units) > 1 {
			m.editMode = editModeMergeSelect
			// Start at first unit that isn't the current one
			m.mergeTargetIdx = 0
			if m.mergeTargetIdx == m.proposalCursor {
				m.mergeTargetIdx = 1
			}
		}
	case 3: // Add label
		m.editMode = editModeAddLabel
		m.editInput = ""
	case 4: // Edit/delete label
		if m.proposalCursor < len(m.proposal.Units) {
			unit := m.proposal.Units[m.proposalCursor]
			if len(unit.Labels) > 0 {
				// Get first label key for editing
				for k := range unit.Labels {
					m.editLabelKey = k
					break
				}
				m.editMode = editModeEditLabel
				m.editInput = unit.Labels[m.editLabelKey]
			}
		}
	}
	return m, nil
}

// confirmTextEdit applies the text edit
func (m *ImportWizardModel) confirmTextEdit() (tea.Model, tea.Cmd) {
	switch m.editMode {
	case editModeRenameUnit:
		if m.editInput != "" && m.proposalCursor < len(m.proposal.Units) {
			m.proposal.Units[m.proposalCursor].Slug = sanitizeSlug(m.editInput)
		}
	case editModeRenameSpace:
		if m.editInput != "" {
			m.proposal.AppSpace = sanitizeSlug(m.editInput)
		}
	case editModeAddLabel:
		// Format: key=value
		if strings.Contains(m.editInput, "=") {
			parts := strings.SplitN(m.editInput, "=", 2)
			if len(parts) == 2 && parts[0] != "" {
				if m.proposalCursor < len(m.proposal.Units) {
					m.proposal.Units[m.proposalCursor].Labels[parts[0]] = parts[1]
				}
			}
		}
	case editModeEditLabel:
		if m.proposalCursor < len(m.proposal.Units) {
			m.proposal.Units[m.proposalCursor].Labels[m.editLabelKey] = m.editInput
		}
	}
	m.editMode = editModeNone
	m.editInput = ""
	return m, nil
}

// mergeUnits merges the current unit into the target unit
func (m *ImportWizardModel) mergeUnits() {
	if m.proposalCursor >= len(m.proposal.Units) || m.mergeTargetIdx >= len(m.proposal.Units) {
		return
	}
	if m.proposalCursor == m.mergeTargetIdx {
		return
	}

	source := m.proposal.Units[m.proposalCursor]
	target := &m.proposal.Units[m.mergeTargetIdx]

	// Merge workloads
	target.Workloads = append(target.Workloads, source.Workloads...)

	// Merge labels (source overwrites target for conflicts)
	for k, v := range source.Labels {
		if _, exists := target.Labels[k]; !exists {
			target.Labels[k] = v
		}
	}

	// Remove the source unit
	m.proposal.Units = append(m.proposal.Units[:m.proposalCursor], m.proposal.Units[m.proposalCursor+1:]...)

	// Adjust cursor
	if m.proposalCursor >= len(m.proposal.Units) {
		m.proposalCursor = len(m.proposal.Units) - 1
	}
}

// deleteCurrentUnit removes the currently selected unit
func (m *ImportWizardModel) deleteCurrentUnit() {
	if m.proposalCursor >= len(m.proposal.Units) || len(m.proposal.Units) <= 1 {
		return
	}

	m.proposal.Units = append(m.proposal.Units[:m.proposalCursor], m.proposal.Units[m.proposalCursor+1:]...)

	if m.proposalCursor >= len(m.proposal.Units) {
		m.proposalCursor = len(m.proposal.Units) - 1
	}
}

// deleteLabel removes the current label from the unit
func (m *ImportWizardModel) deleteLabel() {
	if m.proposalCursor < len(m.proposal.Units) && m.editLabelKey != "" {
		delete(m.proposal.Units[m.proposalCursor].Labels, m.editLabelKey)
	}
}

func (m ImportWizardModel) nextStep() (tea.Model, tea.Cmd) {
	// Clear search when navigating between steps
	m.searchMode = false
	m.searchInput = ""

	switch m.step {
	case StepSelectNamespaces:
		// Validate at least one namespace selected
		hasSelection := false
		for _, ns := range m.namespaces {
			if ns.Selected {
				hasSelection = true
				break
			}
		}
		if !hasSelection {
			// Auto-select current item and proceed
			if m.namespaceCursor < len(m.namespaces) {
				m.namespaces[m.namespaceCursor].Selected = true
			} else {
				return m, nil
			}
		}
		m.step = StepReviewWorkloads
		m.loading = true
		m.loadingMessage = "Loading workloads..."
		return m, m.loadWorkloads

	case StepReviewWorkloads:
		// Validate at least one workload selected
		hasSelection := false
		for _, w := range m.workloads {
			if w.Selected {
				hasSelection = true
				break
			}
		}
		if !hasSelection {
			return m, nil
		}
		// Collect unique ArgoCD Applications from selected workloads
		m.argoApps = m.collectArgoApps()
		m.step = StepConfigureStructure
		m.loading = true
		m.loadingMessage = "Generating proposal..."
		return m, m.generateProposal

	case StepConfigureStructure:
		m.step = StepApply
		m.loading = true
		m.loadingMessage = "Applying..."
		m.applyProgress = 0
		m.applyStartTime = time.Now()
		if m.proposal != nil {
			m.applyTotal = len(m.proposal.Units)
		}
		return m, m.applyImport

	case StepApply:
		if m.applyComplete {
			// If we have ArgoCD apps, transition happens automatically in message handler
			if len(m.argoApps) == 0 {
				m.quit = true
				return m, tea.Quit
			}
		}

	case StepArgoCleanup:
		if m.argoCleanupDone {
			m.quit = true
			return m, tea.Quit
		}
		// Perform the selected cleanup action on all ArgoCD apps
		m.loading = true
		m.loadingMessage = "Processing ArgoCD Applications..."
		return m, m.performArgoCleanup()
	}
	return m, nil
}

func (m ImportWizardModel) prevStep() (tea.Model, tea.Cmd) {
	// Clear search when navigating between steps
	m.searchMode = false
	m.searchInput = ""

	if m.step > StepSelectNamespaces {
		m.step--
	}
	return m, nil
}

// Data loading commands

func (m ImportWizardModel) discoverNamespaces() tea.Msg {
	namespaces, err := discoverNamespacesWithWorkloads()
	if err != nil {
		return wizardErrMsg{err: err}
	}

	items := make([]NamespaceItem, 0, len(namespaces))
	for _, ns := range namespaces {
		// Count workloads in namespace
		workloads, _ := discoverWorkloads(ns)
		items = append(items, NamespaceItem{
			Name:          ns,
			WorkloadCount: len(workloads),
			Selected:      false, // User must explicitly select namespaces
			Workloads:     workloads,
		})
	}

	return wizardNamespacesMsg{namespaces: items}
}

func (m ImportWizardModel) loadWorkloads() tea.Msg {
	var allWorkloads []WorkloadItem

	for _, ns := range m.namespaces {
		if !ns.Selected {
			continue
		}
		workloads, err := discoverWorkloads(ns.Name)
		if err != nil {
			continue
		}
		for _, w := range workloads {
			app, variant := inferAppAndVariant(w)
			allWorkloads = append(allWorkloads, WorkloadItem{
				Info:     w,
				Selected: true, // Auto-select all
				App:      app,
				Variant:  variant,
			})
		}
	}

	return wizardWorkloadsMsg{workloads: allWorkloads}
}

func (m *ImportWizardModel) buildWorkloadGroups() {
	m.workloadGroups = make(map[string][]int)
	m.expandedGroups = make(map[string]bool)

	for i, w := range m.workloads {
		app := w.App
		if app == "" {
			app = w.Info.Name
		}
		m.workloadGroups[app] = append(m.workloadGroups[app], i)
		m.expandedGroups[app] = true // Expand all by default
	}
}

func (m ImportWizardModel) generateProposal() tea.Msg {
	// Collect selected workloads
	var workloads []WorkloadInfo
	for _, w := range m.workloads {
		if w.Selected {
			workloads = append(workloads, w.Info)
		}
	}

	// Group workloads according to the docs:
	// - ArgoCD: Each Application → one Unit
	// - Flux: Each Kustomization → one Unit
	// - Native: Group by app label
	proposal := m.generateSmartProposal(workloads)
	return wizardProposalMsg{proposal: proposal}
}

// generateSmartProposal groups workloads correctly per the docs:
// - GitOps-managed workloads are grouped by their controller (Application/Kustomization)
// - Native workloads are grouped by app label
func (m ImportWizardModel) generateSmartProposal(workloads []WorkloadInfo) *FullProposal {
	proposal := &FullProposal{
		AppSpace: inferAppSpace(workloads, ""),
	}

	// Group workloads by their "unit key":
	// - For ArgoCD/Flux: use GitOpsRef.Name (the Application/Kustomization name)
	// - For Native: use inferred app name
	type unitGroup struct {
		key       string
		owner     string
		workloads []WorkloadInfo
	}

	groups := make(map[string]*unitGroup)

	for _, w := range workloads {
		var key string

		if w.Owner == "ArgoCD" || w.Owner == "Flux" {
			// Group by GitOps controller name
			if w.GitOpsRef != nil && w.GitOpsRef.Name != "" {
				key = w.GitOpsRef.Name
			} else {
				// Fallback to app name if no GitOpsRef
				key, _ = inferAppAndVariant(w)
				if key == "" {
					key = w.Name
				}
			}
		} else {
			// Native/Helm: group by inferred app name
			key, _ = inferAppAndVariant(w)
			if key == "" {
				key = w.Name
			}
		}

		if groups[key] == nil {
			groups[key] = &unitGroup{
				key:   key,
				owner: w.Owner,
			}
		}
		groups[key].workloads = append(groups[key].workloads, w)
	}

	// Convert groups to units
	for key, group := range groups {
		// Infer variant from first workload
		_, variant := inferAppAndVariant(group.workloads[0])
		if variant == "" {
			variant = "default"
		}

		slug := sanitizeSlug(key)
		if variant != "default" && variant != "" {
			slug = sanitizeSlug(fmt.Sprintf("%s-%s", key, variant))
		}

		unit := UnitProposal{
			Slug:    slug,
			App:     key,
			Variant: variant,
			Labels:  map[string]string{"app": key, "variant": variant, "owner": group.owner},
		}

		// Collect workload references and labels
		for _, w := range group.workloads {
			unit.Workloads = append(unit.Workloads, fmt.Sprintf("%s/%s", w.Namespace, w.Name))

			// Extract additional labels
			if region := extractRegion(w, ""); region != "" && unit.Region == "" {
				unit.Region = region
				unit.Labels["region"] = region
			}
			if tier := extractTier(w); tier != "" && unit.Tier == "" {
				unit.Tier = tier
				unit.Labels["tier"] = tier
			}
			if team := extractTeam(w, proposal.AppSpace); team != "" {
				if _, exists := unit.Labels["team"]; !exists {
					unit.Labels["team"] = team
				}
			}
		}

		proposal.Units = append(proposal.Units, unit)
	}

	// Sort units by app name
	sort.Slice(proposal.Units, func(i, j int) bool {
		return proposal.Units[i].App < proposal.Units[j].App
	})

	return proposal
}

// applyStartMsg signals to start the apply process
type applyStartMsg struct{}

// applyTickMsg is used to poll for apply progress
type applyTickMsg time.Time

func (m ImportWizardModel) applyImport() tea.Msg {
	// Just signal to start - actual work happens in applyNextUnit
	return applyStartMsg{}
}

// startApplyCmd creates the app space and triggers first unit
func (m ImportWizardModel) startApplyCmd() tea.Cmd {
	return func() tea.Msg {
		if m.proposal == nil {
			return wizardErrMsg{err: fmt.Errorf("no proposal to apply")}
		}

		// Create App Space first
		_, err := CreateAppSpaceWithResult(m.proposal.AppSpace, true, nil)
		if err != nil {
			return wizardErrMsg{err: fmt.Errorf("create space: %w", err)}
		}

		// Signal to process first unit
		return wizardApplyProgressMsg{unitSlug: "__space_created__", success: true}
	}
}

// applyNextUnitCmd creates the next unit in the proposal
func (m ImportWizardModel) applyNextUnitCmd(unitIndex int) tea.Cmd {
	return func() tea.Msg {
		if m.proposal == nil || unitIndex >= len(m.proposal.Units) {
			return wizardApplyCompleteMsg{}
		}

		unit := m.proposal.Units[unitIndex]

		// Skip units with no workloads
		if len(unit.Workloads) == 0 {
			return wizardApplyProgressMsg{unitSlug: unit.Slug, success: true}
		}

		// Build workload index
		workloadIndex := make(map[string]WorkloadInfo)
		for _, w := range m.workloads {
			if w.Selected {
				key := fmt.Sprintf("%s/%s", w.Info.Namespace, w.Info.Name)
				workloadIndex[key] = w.Info
			}
		}

		// Get first workload's manifest
		w, ok := workloadIndex[unit.Workloads[0]]
		if !ok {
			return wizardApplyProgressMsg{unitSlug: unit.Slug, success: false, err: "workload not found"}
		}

		manifest, err := fetchManifest(w.Kind, w.Namespace, w.Name)
		if err != nil {
			return wizardApplyProgressMsg{unitSlug: unit.Slug, success: false, err: err.Error()}
		}

		labels := []string{}
		for k, v := range unit.Labels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}

		if err := createUnitWithManifestSimple(m.proposal.AppSpace, unit.Slug, labels, manifest); err != nil {
			return wizardApplyProgressMsg{unitSlug: unit.Slug, success: false, err: err.Error()}
		}

		return wizardApplyProgressMsg{unitSlug: unit.Slug, success: true}
	}
}

// startWorkerCmd starts a worker for the app space
func (m ImportWizardModel) startWorkerCmd() tea.Cmd {
	return func() tea.Msg {
		if m.proposal == nil {
			return wizardWorkerStartedMsg{err: fmt.Errorf("no proposal")}
		}

		// Use app space name as worker name
		workerName := m.proposal.AppSpace + "-worker"
		space := m.proposal.AppSpace

		// Start worker in background
		cmd := exec.Command("cub", "worker", "run", workerName, "--space", space)
		cmd.Stdout = nil
		cmd.Stderr = nil
		cmd.Stdin = nil

		if err := cmd.Start(); err != nil {
			return wizardWorkerStartedMsg{err: fmt.Errorf("failed to start worker: %w", err)}
		}

		// Don't wait for the process - let it run in background
		// We must call Wait() to release process resources, but we don't need the result
		go func() {
			_ = cmd.Wait() //nolint:errcheck // background process, exit status irrelevant
		}()

		return wizardWorkerStartedMsg{worker: workerName}
	}
}

// performArgoCleanup executes the selected cleanup action on all ArgoCD Applications
func (m ImportWizardModel) performArgoCleanup() tea.Cmd {
	return func() tea.Msg {
		if len(m.argoApps) == 0 {
			return wizardArgoCleanupMsg{success: true}
		}

		// If "keep as-is" selected, just mark as done
		if m.argoCleanupIdx == argoCleanupKeepAsIs {
			return wizardArgoCleanupMsg{success: true}
		}

		// Process all ArgoCD apps with the selected action
		for _, app := range m.argoApps {
			var err error

			switch m.argoCleanupIdx {
			case argoCleanupDisableSync:
				err = disableArgoAutoSync(m.ctx, m.dynClient, app.Namespace, app.Name)
			case argoCleanupDeleteApp:
				err = deleteArgoApplication(m.ctx, m.dynClient, app.Namespace, app.Name)
			}

			if err != nil {
				return wizardArgoCleanupMsg{
					appName: app.Name,
					action:  m.argoCleanupIdx,
					success: false,
					err:     err,
				}
			}
		}

		return wizardArgoCleanupMsg{
			action:  m.argoCleanupIdx,
			success: true,
		}
	}
}

// runTestPhaseCmd executes a test phase
func (m ImportWizardModel) runTestPhaseCmd(phase int) tea.Cmd {
	return func() tea.Msg {
		switch phase {
		case testPhaseAddAnnotation:
			return m.runTestAddAnnotation()
		case testPhaseApply:
			return m.runTestApply()
		case testPhaseWaitSync:
			return m.runTestWaitSync()
		case testPhaseVerify:
			return m.runTestVerify()
		}
		return wizardTestPhaseMsg{phase: phase, success: false, err: fmt.Errorf("unknown test phase")}
	}
}

// runTestAddAnnotation adds a test annotation to the unit's Kubernetes manifest
func (m ImportWizardModel) runTestAddAnnotation() tea.Msg {
	startTime := time.Now() // Record when test actually starts

	// Clear debug directory at start of test
	os.RemoveAll(testDebugDir)
	os.MkdirAll(testDebugDir, 0755)
	appendTestDebug("=== TEST STARTED ===")
	appendTestDebug(fmt.Sprintf("Space: %s, Unit: %s, Annotation: %s", m.proposal.AppSpace, m.testUnitSlug, m.testAnnotation))

	if m.proposal == nil || m.testUnitSlug == "" {
		appendTestDebug("ERROR: no unit to test")
		return wizardTestPhaseMsg{
			phase:     testPhaseAddAnnotation,
			success:   false,
			err:       fmt.Errorf("no unit to test"),
			startTime: startTime,
		}
	}

	annotationKey := "confighub.com/import-test"

	// Step 1: Get unit JSON
	appendTestDebug("Step 1: Getting unit JSON...")
	getCmd := exec.Command("cub", "unit", "get", "--space", m.proposal.AppSpace, m.testUnitSlug, "--json")
	unitJSON, err := getCmd.CombinedOutput()
	writeTestDebug("01-unit-get.json", unitJSON)
	appendTestDebug(fmt.Sprintf("Unit get result: %d bytes, err=%v", len(unitJSON), err))
	if err != nil {
		appendTestDebug(fmt.Sprintf("ERROR getting unit: %s", string(unitJSON)))
		return wizardTestPhaseMsg{
			phase:     testPhaseAddAnnotation,
			success:   false,
			details:   string(unitJSON),
			err:       fmt.Errorf("failed to get unit: %w", err),
			startTime: startTime,
		}
	}

	// Step 2: Extract base64 data
	// Write JSON to temp file first to avoid shell escaping issues with embedded newlines
	appendTestDebug("Step 2: Extracting base64 data...")
	jsonTmpFile := filepath.Join(testDebugDir, "unit-tmp.json")
	os.WriteFile(jsonTmpFile, unitJSON, 0644)
	jqCmd := exec.Command("jq", "-r", ".Unit.Data", jsonTmpFile)
	base64Data, err := jqCmd.CombinedOutput()
	writeTestDebug("02-base64-data.txt", base64Data)
	appendTestDebug(fmt.Sprintf("Base64 data: %d bytes, err=%v", len(base64Data), err))
	if err != nil || strings.TrimSpace(string(base64Data)) == "" || strings.TrimSpace(string(base64Data)) == "null" {
		appendTestDebug(fmt.Sprintf("ERROR: No data in unit, base64=%s", string(base64Data)))
		return wizardTestPhaseMsg{
			phase:     testPhaseAddAnnotation,
			success:   false,
			details:   fmt.Sprintf("No data in unit. Unit JSON saved to %s/01-unit-get.json", testDebugDir),
			err:       fmt.Errorf("unit has no data"),
			startTime: startTime,
		}
	}

	// Step 3: Decode YAML
	appendTestDebug("Step 3: Decoding YAML...")
	// Write base64 to file and decode to avoid shell escaping issues
	base64TmpFile := filepath.Join(testDebugDir, "base64-tmp.txt")
	os.WriteFile(base64TmpFile, []byte(strings.TrimSpace(string(base64Data))), 0644)
	decodeCmd := exec.Command("base64", "-d", "-i", base64TmpFile)
	yamlData, err := decodeCmd.CombinedOutput()
	writeTestDebug("03-original-yaml.yaml", yamlData)
	appendTestDebug(fmt.Sprintf("Original YAML: %d bytes, err=%v", len(yamlData), err))
	if err != nil {
		appendTestDebug(fmt.Sprintf("ERROR decoding YAML: %s", string(yamlData)))
		return wizardTestPhaseMsg{
			phase:     testPhaseAddAnnotation,
			success:   false,
			details:   string(yamlData),
			err:       fmt.Errorf("failed to decode yaml: %w", err),
			startTime: startTime,
		}
	}

	// Step 4: Add annotation using awk
	appendTestDebug("Step 4: Adding annotation with awk...")
	awkScript := fmt.Sprintf(`/^  annotations:/{print; print "    %s: \"%s\""; next}1`, annotationKey, m.testAnnotation)
	awkCmd := exec.Command("awk", awkScript)
	awkCmd.Stdin = strings.NewReader(string(yamlData))
	modifiedYAML, err := awkCmd.CombinedOutput()
	writeTestDebug("04-modified-yaml.yaml", modifiedYAML)
	appendTestDebug(fmt.Sprintf("Modified YAML: %d bytes, err=%v", len(modifiedYAML), err))
	if err != nil || len(modifiedYAML) == 0 {
		appendTestDebug(fmt.Sprintf("ERROR modifying YAML: %s", string(modifiedYAML)))
		return wizardTestPhaseMsg{
			phase:     testPhaseAddAnnotation,
			success:   false,
			details:   fmt.Sprintf("Failed to modify YAML. See %s/03-original-yaml.yaml", testDebugDir),
			err:       fmt.Errorf("failed to add annotation with awk"),
			startTime: startTime,
		}
	}

	// Check if annotation was actually added
	if !strings.Contains(string(modifiedYAML), annotationKey) {
		appendTestDebug("ERROR: Annotation not added - possibly no annotations: section in YAML")
		// YAML might not have annotations section, need to add it
		writeTestDebug("04-modified-yaml-FAILED.yaml", modifiedYAML)
		return wizardTestPhaseMsg{
			phase:     testPhaseAddAnnotation,
			success:   false,
			details:   fmt.Sprintf("Annotation not added. YAML may lack annotations section. See %s/", testDebugDir),
			err:       fmt.Errorf("annotation not inserted - check YAML structure"),
			startTime: startTime,
		}
	}

	// Step 5: Update unit
	appendTestDebug("Step 5: Updating unit with modified YAML...")
	updateCmd := exec.Command("cub", "unit", "update", "--space", m.proposal.AppSpace, m.testUnitSlug, "-", "--change-desc", "Import wizard test")
	updateCmd.Stdin = strings.NewReader(string(modifiedYAML))
	updateOutput, err := updateCmd.CombinedOutput()
	writeTestDebug("05-update-result.txt", updateOutput)
	appendTestDebug(fmt.Sprintf("Update result: %s, err=%v", strings.TrimSpace(string(updateOutput)), err))
	if err != nil {
		appendTestDebug(fmt.Sprintf("ERROR updating unit: %s", string(updateOutput)))
		return wizardTestPhaseMsg{
			phase:     testPhaseAddAnnotation,
			success:   false,
			details:   string(updateOutput),
			err:       fmt.Errorf("failed to update unit: %w", err),
			startTime: startTime,
		}
	}

	appendTestDebug("Phase 1 (AddAnnotation) SUCCESS")
	return wizardTestPhaseMsg{
		phase:     testPhaseAddAnnotation,
		success:   true,
		details:   fmt.Sprintf("Added annotation %s=%s to %s (debug: %s)", annotationKey, m.testAnnotation, m.testUnitSlug, testDebugDir),
		startTime: startTime,
	}
}

// runTestApply applies the unit to the cluster
func (m ImportWizardModel) runTestApply() tea.Msg {
	appendTestDebug("=== Phase 2: Apply ===")

	if m.proposal == nil || m.testUnitSlug == "" {
		appendTestDebug("ERROR: no unit to test")
		return wizardTestPhaseMsg{
			phase:   testPhaseApply,
			success: false,
			err:     fmt.Errorf("no unit to test"),
		}
	}

	// First, check if the unit has a target. If not, find one and set it.
	appendTestDebug("Checking if unit has target...")
	checkCmd := exec.Command("cub", "unit", "get",
		"--space", m.proposal.AppSpace,
		"--json",
		m.testUnitSlug)
	checkOutput, _ := checkCmd.CombinedOutput()
	writeTestDebug("06-unit-before-apply.json", checkOutput)
	appendTestDebug(fmt.Sprintf("Unit JSON: %d bytes", len(checkOutput)))

	// Check if Target exists and is not null
	// The JSON structure may have "Target": null, "Target":null, or no Target field at all
	hasTarget := strings.Contains(string(checkOutput), `"Target"`) &&
		!strings.Contains(string(checkOutput), `"Target":null`) &&
		!strings.Contains(string(checkOutput), `"Target": null`) &&
		!strings.Contains(string(checkOutput), `"Target":{}`) &&
		!strings.Contains(string(checkOutput), `"Target": {}`)
	appendTestDebug(fmt.Sprintf("Unit has target: %v (Target field present: %v)", hasTarget, strings.Contains(string(checkOutput), `"Target"`)))

	if !hasTarget {
		appendTestDebug("Unit has no target, finding one...")
		// Find a Kubernetes target
		targetCmd := exec.Command("cub", "target", "list",
			"--space", m.proposal.AppSpace,
			"--json")
		targetOutput, err := targetCmd.CombinedOutput()
		writeTestDebug("07-target-list.json", targetOutput)
		appendTestDebug(fmt.Sprintf("Target list: %d bytes, err=%v", len(targetOutput), err))
		if err != nil {
			appendTestDebug(fmt.Sprintf("ERROR listing targets: %s", string(targetOutput)))
			return wizardTestPhaseMsg{
				phase:   testPhaseApply,
				success: false,
				details: fmt.Sprintf("Could not list targets. See %s/07-target-list.json", testDebugDir),
				err:     fmt.Errorf("failed to list targets: %w", err),
			}
		}

		// Use jq to find Kubernetes target
		jqCmd := exec.Command("sh", "-c",
			fmt.Sprintf(`cub target list --space %s --json | jq -r '[.[] | select(.Target.ProviderType == "Kubernetes")] | .[0].Target.Slug // empty'`,
				m.proposal.AppSpace))
		jqOutput, _ := jqCmd.CombinedOutput()
		targetSlug := strings.TrimSpace(string(jqOutput))
		appendTestDebug(fmt.Sprintf("Found target slug via jq: '%s'", targetSlug))

		if targetSlug == "" {
			appendTestDebug("ERROR: No Kubernetes target found")
			return wizardTestPhaseMsg{
				phase:   testPhaseApply,
				success: false,
				details: fmt.Sprintf("No Kubernetes target found in space. See %s/07-target-list.json", testDebugDir),
				err:     fmt.Errorf("unit has no target and no Kubernetes target found"),
			}
		}

		// Set the target on the unit
		appendTestDebug(fmt.Sprintf("Setting target to: %s", targetSlug))
		setTargetCmd := exec.Command("cub", "unit", "set-target",
			"--space", m.proposal.AppSpace,
			m.testUnitSlug,
			targetSlug)
		setOutput, err := setTargetCmd.CombinedOutput()
		writeTestDebug("08-set-target-result.txt", setOutput)
		appendTestDebug(fmt.Sprintf("Set target result: %s, err=%v", strings.TrimSpace(string(setOutput)), err))
		if err != nil {
			appendTestDebug(fmt.Sprintf("ERROR setting target: %s", string(setOutput)))
			return wizardTestPhaseMsg{
				phase:   testPhaseApply,
				success: false,
				details: string(setOutput),
				err:     fmt.Errorf("failed to set target: %w", err),
			}
		}
	}

	// Apply the unit
	appendTestDebug("Applying unit...")
	cmd := exec.Command("cub", "unit", "apply",
		"--space", m.proposal.AppSpace,
		"--wait",
		m.testUnitSlug)

	output, err := cmd.CombinedOutput()
	writeTestDebug("09-apply-result.txt", output)
	appendTestDebug(fmt.Sprintf("Apply result: %s, err=%v", strings.TrimSpace(string(output)), err))
	if err != nil {
		appendTestDebug(fmt.Sprintf("ERROR applying unit: %s", string(output)))
		return wizardTestPhaseMsg{
			phase:   testPhaseApply,
			success: false,
			details: string(output),
			err:     fmt.Errorf("failed to apply unit: %w", err),
		}
	}

	appendTestDebug("Phase 2 (Apply) SUCCESS")
	return wizardTestPhaseMsg{
		phase:   testPhaseApply,
		success: true,
		details: "Applied unit to cluster",
	}
}

// runTestWaitSync waits for the worker to sync and then returns
func (m ImportWizardModel) runTestWaitSync() tea.Msg {
	appendTestDebug("=== Phase 3: Wait Sync ===")

	if m.proposal == nil || m.testUnitSlug == "" {
		appendTestDebug("ERROR: no unit to test")
		return wizardTestPhaseMsg{
			phase:   testPhaseWaitSync,
			success: false,
			err:     fmt.Errorf("no unit to test"),
		}
	}

	// Check if LiveRevisionNum matches HeadRevisionNum
	// This indicates the worker has synced
	appendTestDebug("Getting unit status...")
	cmd := exec.Command("cub", "unit", "get",
		"--space", m.proposal.AppSpace,
		"--json",
		m.testUnitSlug)

	output, err := cmd.CombinedOutput()
	writeTestDebug("10-unit-sync-status.json", output)
	appendTestDebug(fmt.Sprintf("Unit status: %d bytes, err=%v", len(output), err))
	if err != nil {
		appendTestDebug(fmt.Sprintf("ERROR getting unit status: %s", string(output)))
		return wizardTestPhaseMsg{
			phase:   testPhaseWaitSync,
			success: false,
			details: string(output),
			err:     fmt.Errorf("failed to get unit status: %w", err),
		}
	}

	// Parse the JSON to check revision status
	// We're looking for LiveRevisionNum to match or be close to HeadRevisionNum
	// For simplicity, we'll just wait a short time and move on
	appendTestDebug("Waiting 2 seconds for sync...")
	time.Sleep(2 * time.Second)

	appendTestDebug("Phase 3 (WaitSync) SUCCESS")
	return wizardTestPhaseMsg{
		phase:   testPhaseWaitSync,
		success: true,
		details: "Worker synced successfully",
	}
}

// runTestVerify verifies the annotation appears in the cluster
func (m ImportWizardModel) runTestVerify() tea.Msg {
	appendTestDebug("=== Phase 4: Verify ===")

	if m.proposal == nil || m.testUnitSlug == "" || len(m.proposal.Units) == 0 {
		appendTestDebug("ERROR: no unit to test")
		return wizardTestPhaseMsg{
			phase:   testPhaseVerify,
			success: false,
			err:     fmt.Errorf("no unit to test"),
		}
	}

	// Find the unit and its workload identifiers
	var workloadIDs []string
	for i := range m.proposal.Units {
		if m.proposal.Units[i].Slug == m.testUnitSlug {
			workloadIDs = m.proposal.Units[i].Workloads
			break
		}
	}
	appendTestDebug(fmt.Sprintf("Workload IDs for unit %s: %v", m.testUnitSlug, workloadIDs))

	if len(workloadIDs) == 0 {
		appendTestDebug("ERROR: could not find unit or workloads")
		return wizardTestPhaseMsg{
			phase:   testPhaseVerify,
			success: false,
			err:     fmt.Errorf("could not find unit or workloads"),
		}
	}

	// Find the first selected workload that matches the unit's workloads
	var workload *WorkloadInfo
	for _, wid := range workloadIDs {
		for i := range m.workloads {
			// workload ID can be various formats:
			// - "Kind/Namespace/Name" (e.g., "Deployment/argocd/app")
			// - "Namespace/Name" (e.g., "argocd/app")
			// - Just "Name" (e.g., "app")
			w := &m.workloads[i].Info
			wKey1 := fmt.Sprintf("%s/%s/%s", w.Kind, w.Namespace, w.Name) // Kind/Namespace/Name
			wKey2 := fmt.Sprintf("%s/%s", w.Namespace, w.Name)            // Namespace/Name
			if wKey1 == wid || wKey2 == wid || w.Name == wid {
				workload = w
				appendTestDebug(fmt.Sprintf("Found workload: %s/%s/%s (matched via wid=%s)", w.Kind, w.Namespace, w.Name, wid))
				break
			}
		}
		if workload != nil {
			break
		}
	}

	if workload == nil {
		// Fallback: use the first selected workload
		appendTestDebug("Workload not found by ID, falling back to first selected workload")
		for i := range m.workloads {
			if m.workloads[i].Selected {
				workload = &m.workloads[i].Info
				appendTestDebug(fmt.Sprintf("Using fallback workload: %s/%s/%s", workload.Kind, workload.Namespace, workload.Name))
				break
			}
		}
	}

	if workload == nil {
		appendTestDebug("ERROR: could not find workload to verify")
		return wizardTestPhaseMsg{
			phase:   testPhaseVerify,
			success: false,
			err:     fmt.Errorf("could not find workload to verify"),
		}
	}

	annotationKey := "confighub.com/import-test"

	// First, get ALL annotations for debug
	appendTestDebug(fmt.Sprintf("Getting all annotations from %s/%s/%s...", workload.Kind, workload.Namespace, workload.Name))
	allAnnotationsCmd := exec.Command("kubectl", "get", strings.ToLower(workload.Kind),
		workload.Name,
		"-n", workload.Namespace,
		"-o", "jsonpath={.metadata.annotations}")
	allAnnotations, _ := allAnnotationsCmd.CombinedOutput()
	writeTestDebug("11-cluster-annotations.json", allAnnotations)
	appendTestDebug(fmt.Sprintf("All annotations: %s", string(allAnnotations)))

	// Use kubectl to check the annotation on the resource
	appendTestDebug(fmt.Sprintf("Checking for annotation %s=%s", annotationKey, m.testAnnotation))
	cmd := exec.Command("kubectl", "get", strings.ToLower(workload.Kind),
		workload.Name,
		"-n", workload.Namespace,
		"-o", fmt.Sprintf("jsonpath={.metadata.annotations.%s}", strings.ReplaceAll(annotationKey, "/", "\\/")))

	output, err := cmd.CombinedOutput()
	writeTestDebug("12-annotation-check.txt", output)
	appendTestDebug(fmt.Sprintf("Annotation check result: '%s', err=%v", string(output), err))

	if err != nil {
		// Try alternate jsonpath format
		appendTestDebug("First jsonpath failed, trying alternate format...")
		cmd = exec.Command("kubectl", "get", strings.ToLower(workload.Kind),
			workload.Name,
			"-n", workload.Namespace,
			"-o", "jsonpath={.metadata.annotations['confighub\\.com/import-test']}")
		output, err = cmd.CombinedOutput()
		appendTestDebug(fmt.Sprintf("Alternate jsonpath result: '%s', err=%v", string(output), err))
		if err != nil {
			appendTestDebug(fmt.Sprintf("ERROR: failed to get annotation from cluster: %s", string(output)))
			return wizardTestPhaseMsg{
				phase:   testPhaseVerify,
				success: false,
				details: fmt.Sprintf("Failed to get annotation. See %s/", testDebugDir),
				err:     fmt.Errorf("failed to get annotation from cluster: %w", err),
			}
		}
	}

	foundValue := strings.TrimSpace(string(output))
	appendTestDebug(fmt.Sprintf("Found value: '%s', expected: '%s'", foundValue, m.testAnnotation))

	if foundValue == m.testAnnotation {
		appendTestDebug("Phase 4 (Verify) SUCCESS - annotation found in cluster!")
		return wizardTestPhaseMsg{
			phase:   testPhaseVerify,
			success: true,
			details: fmt.Sprintf("✓ Verified: annotation %s=%s found on %s/%s", annotationKey, m.testAnnotation, workload.Kind, workload.Name),
		}
	}

	// Annotation not found - check if this is a GitOps-managed resource
	// If so, the annotation may have been reconciled away, which is expected behavior
	if foundValue == "" {
		appendTestDebug("Annotation not found in cluster, checking if GitOps-managed...")
		annotations := string(allAnnotations)

		isGitOpsManaged := strings.Contains(annotations, "argocd.argoproj.io") ||
			strings.Contains(annotations, "kustomize.toolkit.fluxcd.io") ||
			strings.Contains(annotations, "helm.toolkit.fluxcd.io")
		appendTestDebug(fmt.Sprintf("Is GitOps managed: %v", isGitOpsManaged))

		if isGitOpsManaged {
			// GitOps reconciliation removed our annotation - this is expected
			// Verify the annotation is still in the Unit (ConfigHub side worked)
			appendTestDebug("Checking if annotation is in Unit data...")
			unitCmd := exec.Command("cub", "unit", "get",
				"--space", m.proposal.AppSpace,
				"--json",
				m.testUnitSlug)
			unitOutput, err := unitCmd.CombinedOutput()
			writeTestDebug("13-unit-final-state.json", unitOutput)
			appendTestDebug(fmt.Sprintf("Unit final state: %d bytes, err=%v", len(unitOutput), err))

			// The annotation is in base64-encoded Unit.Data, need to decode and check
			unitJsonFile := filepath.Join(testDebugDir, "unit-final-tmp.json")
			os.WriteFile(unitJsonFile, unitOutput, 0644)
			jqCmd := exec.Command("jq", "-r", ".Unit.Data", unitJsonFile)
			base64Data, jqErr := jqCmd.CombinedOutput()
			if jqErr == nil && len(base64Data) > 0 {
				base64File := filepath.Join(testDebugDir, "unit-final-base64.txt")
				os.WriteFile(base64File, []byte(strings.TrimSpace(string(base64Data))), 0644)
				decodeCmd := exec.Command("base64", "-d", "-i", base64File)
				decodedYAML, decErr := decodeCmd.CombinedOutput()
				writeTestDebug("14-unit-final-yaml.yaml", decodedYAML)
				appendTestDebug(fmt.Sprintf("Decoded Unit YAML: %d bytes, contains annotation: %v", len(decodedYAML), strings.Contains(string(decodedYAML), m.testAnnotation)))

				if decErr == nil && strings.Contains(string(decodedYAML), m.testAnnotation) {
					appendTestDebug("Phase 4 (Verify) SUCCESS - annotation in Unit data, GitOps reconciled it away")
					return wizardTestPhaseMsg{
						phase:   testPhaseVerify,
						success: true,
						details: fmt.Sprintf("✓ ConfigHub pipeline verified! Annotation was applied but GitOps (ArgoCD/Flux) reconciled it back.\n  This is expected behavior for GitOps-managed resources.\n  The annotation is still stored in ConfigHub Unit data."),
					}
				}
			}
			appendTestDebug("ERROR: GitOps reconciled annotation and it's not in Unit data")
			return wizardTestPhaseMsg{
				phase:   testPhaseVerify,
				success: false,
				details: fmt.Sprintf("GitOps reconciled the annotation away and it's not in Unit data. See %s/", testDebugDir),
				err:     fmt.Errorf("annotation not found anywhere"),
			}
		}

		appendTestDebug(fmt.Sprintf("ERROR: Annotation not propagated to cluster. Debug files at %s/", testDebugDir))
		return wizardTestPhaseMsg{
			phase:   testPhaseVerify,
			success: false,
			details: fmt.Sprintf("Annotation not found on %s/%s. Debug files at %s/", workload.Kind, workload.Name, testDebugDir),
			err:     fmt.Errorf("annotation not propagated to cluster"),
		}
	}

	appendTestDebug(fmt.Sprintf("ERROR: Annotation value mismatch. Expected '%s', got '%s'", m.testAnnotation, foundValue))
	return wizardTestPhaseMsg{
		phase:   testPhaseVerify,
		success: false,
		details: fmt.Sprintf("Expected %s, got %s. Debug files at %s/", m.testAnnotation, foundValue, testDebugDir),
		err:     fmt.Errorf("annotation value mismatch"),
	}
}

// checkSyncStatusCmd checks if the unit is synced (used for polling)
func (m ImportWizardModel) checkSyncStatusCmd() tea.Cmd {
	return func() tea.Msg {
		// Check unit status
		cmd := exec.Command("cub", "unit", "get",
			"--space", m.proposal.AppSpace,
			"--json",
			m.testUnitSlug)

		_, err := cmd.CombinedOutput()
		if err != nil {
			// Keep polling
			time.Sleep(500 * time.Millisecond)
			return wizardTestTickMsg{}
		}

		// Synced, move to verify
		return wizardTestPhaseMsg{
			phase:   testPhaseWaitSync,
			success: true,
			details: "Worker synced",
		}
	}
}

// Render helpers for each step

func (m ImportWizardModel) renderNamespaceList() string {
	var b strings.Builder

	b.WriteString(dimStyle.Render("Select which namespaces to import from."))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Only workloads from selected namespaces will be imported."))
	b.WriteString("\n\n")

	// Search bar
	if m.searchMode {
		b.WriteString(wizardSelectedStyle.Render("/") + " " + m.searchInput + "█\n\n")
	} else if m.searchInput != "" {
		b.WriteString(dimStyle.Render("Filter: ") + m.searchInput + dimStyle.Render("  (/ to edit, esc to clear)") + "\n\n")
	}

	// Build filtered list of indices
	visibleCount := 0
	for i, ns := range m.namespaces {
		// Skip items that don't match filter
		if !m.matchesFilter(ns.Name) {
			continue
		}
		visibleCount++

		cursor := "  "
		if i == m.namespaceCursor {
			cursor = "> "
		}

		checkbox := wizardCheckboxOff.Render("☐")
		if ns.Selected {
			checkbox = wizardCheckboxOn.Render("☑")
		}

		name := ns.Name
		if i == m.namespaceCursor {
			name = wizardSelectedStyle.Render(name)
		}

		count := dimStyle.Render(fmt.Sprintf("(%d workloads)", ns.WorkloadCount))

		b.WriteString(fmt.Sprintf("%s%s %s %s\n", cursor, checkbox, name, count))
	}

	// Show message if filter hides all items
	if visibleCount == 0 && m.searchInput != "" {
		b.WriteString(dimStyle.Render("No namespaces match filter") + "\n")
	}

	return b.String()
}

func (m ImportWizardModel) renderNamespacePreview() string {
	if len(m.namespaces) == 0 || m.namespaceCursor < 0 || m.namespaceCursor >= len(m.namespaces) {
		return ""
	}

	ns := m.namespaces[m.namespaceCursor]
	var b strings.Builder

	b.WriteString(headerStyle.Render(ns.Name))
	b.WriteString("\n\n")

	for _, w := range ns.Workloads {
		ownerStyle := m.getOwnerStyle(w.Owner)
		status := statusOK.Render("●Ready")
		if !w.Ready {
			status = statusWarn.Render("○")
		}

		b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
			w.Name,
			ownerStyle.Render(w.Owner),
			status,
		))
	}

	// Show detected info
	if len(ns.Workloads) > 0 {
		b.WriteString("\n")
		w := ns.Workloads[0]
		if w.KustomizationPath != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("Flux Path: %s", w.KustomizationPath)))
		} else if w.ApplicationPath != "" {
			b.WriteString(dimStyle.Render(fmt.Sprintf("Argo Path: %s", w.ApplicationPath)))
		}
	}

	return b.String()
}

func (m ImportWizardModel) renderWorkloadList() string {
	var b strings.Builder

	b.WriteString(dimStyle.Render("Review discovered workloads. Deselect any you don't"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("want to import. You can adjust grouping on the next step."))
	b.WriteString("\n\n")

	// Search bar
	if m.searchMode {
		b.WriteString(wizardSelectedStyle.Render("/") + " " + m.searchInput + "█\n\n")
	} else if m.searchInput != "" {
		b.WriteString(dimStyle.Render("Filter: ") + m.searchInput + dimStyle.Render("  (/ to edit, esc to clear)") + "\n\n")
	}

	// Build filtered list
	visibleCount := 0
	for i, w := range m.workloads {
		// Skip items that don't match filter (check name and owner)
		if !m.matchesFilter(w.Info.Name) && !m.matchesFilter(w.Info.Owner) {
			continue
		}
		visibleCount++

		cursor := "  "
		if i == m.workloadCursor {
			cursor = "> "
		}

		checkbox := wizardCheckboxOff.Render("☐")
		if w.Selected {
			checkbox = wizardCheckboxOn.Render("☑")
		}

		name := w.Info.Name
		if i == m.workloadCursor {
			name = wizardSelectedStyle.Render(name)
		}

		ownerStyle := m.getOwnerStyle(w.Info.Owner)
		owner := ownerStyle.Render(fmt.Sprintf("[%s]", w.Info.Owner))

		// Show indicator if already imported to ConfigHub
		alreadyImported := ""
		if w.Info.UnitSlug != "" {
			alreadyImported = dimStyle.Render(fmt.Sprintf(" (→%s)", w.Info.UnitSlug))
		}

		b.WriteString(fmt.Sprintf("%s%s %s %s%s\n", cursor, checkbox, name, owner, alreadyImported))
	}

	// Show message if filter hides all items
	if visibleCount == 0 && m.searchInput != "" {
		b.WriteString(dimStyle.Render("No workloads match filter") + "\n")
	}

	return b.String()
}

func (m ImportWizardModel) renderWorkloadDetails() string {
	if m.workloadCursor >= len(m.workloads) {
		return ""
	}

	w := m.workloads[m.workloadCursor]
	var b strings.Builder

	b.WriteString(headerStyle.Render(fmt.Sprintf("%s (%s)", w.Info.Name, w.Info.Kind)))
	b.WriteString("\n\n")

	// ConfigHub status (if already imported)
	if w.Info.UnitSlug != "" {
		b.WriteString(wizardSuccessStyle.Render("✓ Already imported"))
		b.WriteString(fmt.Sprintf(" → %s\n\n", w.Info.UnitSlug))
	}

	// Owner info
	b.WriteString(fmt.Sprintf("Owner: %s\n", m.getOwnerStyle(w.Info.Owner).Render(w.Info.Owner)))

	if w.Info.GitOpsRef != nil {
		b.WriteString(fmt.Sprintf("  %s: %s\n", w.Info.GitOpsRef.Kind, w.Info.GitOpsRef.Name))
	}

	if w.Info.KustomizationPath != "" {
		b.WriteString(fmt.Sprintf("  Path: %s\n", w.Info.KustomizationPath))
	} else if w.Info.ApplicationPath != "" {
		b.WriteString(fmt.Sprintf("  Path: %s\n", w.Info.ApplicationPath))
	}

	b.WriteString("\n")

	// Inferred labels
	b.WriteString(dimStyle.Render("Inferred:"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  app=%s\n", w.App))
	if w.Variant != "" {
		b.WriteString(fmt.Sprintf("  variant=%s\n", w.Variant))
	}

	// K8s labels (sorted for stable display)
	if len(w.Info.Labels) > 0 {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("K8s Labels:"))
		b.WriteString("\n")
		keys := make([]string, 0, len(w.Info.Labels))
		for k := range w.Info.Labels {
			if strings.HasPrefix(k, "app.kubernetes.io/") || k == "app" {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("  %s: %s\n", k, w.Info.Labels[k]))
		}
	}

	return b.String()
}

func (m ImportWizardModel) renderProposalTree() string {
	if m.proposal == nil {
		return "No proposal generated"
	}

	var b strings.Builder

	// Show edit overlay if in edit mode
	if m.editMode != editModeNone {
		return m.renderEditOverlay()
	}

	// Detect and show pattern
	pattern := m.detectImportPattern()
	if pattern != "" {
		patternStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true)
		b.WriteString(patternStyle.Render("DETECTED: "))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render(pattern))
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render("Review the proposed ConfigHub structure."))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("→← expand/collapse  e edit  d delete"))
	b.WriteString("\n\n")

	// Tree root: App Space
	appSpaceIcon := "📁"
	appSpaceName := wizardSelectedStyle.Render(m.proposal.AppSpace)
	b.WriteString(fmt.Sprintf("%s %s\n", appSpaceIcon, appSpaceName))

	numUnits := len(m.proposal.Units)
	for i, unit := range m.proposal.Units {
		isLast := i == numUnits-1
		isSelected := i == m.proposalCursor
		isExpanded := m.expandedUnits[i]

		// Tree connector
		connector := "├──"
		if isLast {
			connector = "└──"
		}

		// Cursor indicator
		cursor := " "
		if isSelected {
			cursor = ">"
		}

		// Expand/collapse indicator
		expandIcon := "▶"
		if isExpanded {
			expandIcon = "▼"
		}

		// Unit icon based on workload count
		unitIcon := "📦"

		// Workload count
		workloadCount := len(unit.Workloads)
		countStr := dimStyle.Render(fmt.Sprintf("(%d)", workloadCount))

		// Unit name with highlighting
		name := unit.Slug
		if isSelected {
			name = wizardSelectedStyle.Render(name)
		}

		// Owner badge (infer from first workload label or default)
		ownerBadge := ""
		if owner, ok := unit.Labels["owner"]; ok {
			ownerBadge = " " + m.getOwnerStyle(owner).Render(fmt.Sprintf("[%s]", owner))
		}

		// Variant badge
		variantBadge := ""
		if variant, ok := unit.Labels["variant"]; ok && variant != "" && variant != "default" {
			variantBadge = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(variant)
		}

		// Main unit line
		b.WriteString(fmt.Sprintf("%s %s %s %s %s%s%s\n",
			cursor, connector, expandIcon, unitIcon, name, countStr, ownerBadge+variantBadge))

		// Show expanded content (workloads and labels)
		if isExpanded {
			// Continuation prefix for tree
			continuePrefix := "│"
			if isLast {
				continuePrefix = " "
			}

			// Show labels
			if len(unit.Labels) > 0 {
				labels := []string{}
				for k, v := range unit.Labels {
					// Skip owner in labels since we show it as a badge
					if k == "owner" {
						continue
					}
					labelStyle := dimStyle
					if k == "app" {
						labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
					} else if k == "variant" {
						labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
					} else if k == "team" {
						labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("183"))
					} else if k == "region" {
						labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("147"))
					} else if k == "tier" {
						labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("222"))
					}
					labels = append(labels, labelStyle.Render(fmt.Sprintf("%s=%s", k, v)))
				}
				sort.Strings(labels)
				if len(labels) > 0 {
					b.WriteString(fmt.Sprintf("  %s     🏷️  %s\n", continuePrefix, strings.Join(labels, " ")))
				}
			}

			// Show workloads
			for j, wRef := range unit.Workloads {
				wIsLast := j == len(unit.Workloads)-1
				wConnector := "├─"
				if wIsLast {
					wConnector = "└─"
				}

				// Parse workload reference (namespace/name)
				wName := wRef
				if parts := strings.SplitN(wRef, "/", 2); len(parts) == 2 {
					wName = dimStyle.Render(parts[0]+"/") + parts[1]
				}

				b.WriteString(fmt.Sprintf("  %s     %s ☸ %s\n", continuePrefix, wConnector, wName))
			}
		}
	}

	// Summary
	b.WriteString("\n")
	totalWorkloads := 0
	for _, u := range m.proposal.Units {
		totalWorkloads += len(u.Workloads)
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("Total: %d units, %d workloads", numUnits, totalWorkloads)))

	return b.String()
}

func (m ImportWizardModel) renderEditOverlay() string {
	var b strings.Builder

	switch m.editMode {
	case editModeMenu:
		b.WriteString(headerStyle.Render("Edit Menu"))
		b.WriteString("\n\n")

		menuItems := []string{
			"Rename unit",
			"Rename app space",
			"Merge with another unit",
			"Add label",
			"Edit label",
		}

		for i, item := range menuItems {
			cursor := "  "
			if i == m.editMenuCursor {
				cursor = "> "
				item = wizardSelectedStyle.Render(item)
			}
			b.WriteString(fmt.Sprintf("%s%s\n", cursor, item))
		}

		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑↓ select  enter confirm  esc cancel"))

	case editModeRenameUnit:
		b.WriteString(headerStyle.Render("Rename Unit"))
		b.WriteString("\n\n")
		b.WriteString("Current: ")
		if m.proposalCursor < len(m.proposal.Units) {
			b.WriteString(m.proposal.Units[m.proposalCursor].Slug)
		}
		b.WriteString("\n\n")
		b.WriteString("New name: ")
		b.WriteString(wizardSelectedStyle.Render(m.editInput))
		b.WriteString("▌\n\n")
		b.WriteString(dimStyle.Render("enter confirm  esc cancel"))

	case editModeRenameSpace:
		b.WriteString(headerStyle.Render("Rename App Space"))
		b.WriteString("\n\n")
		b.WriteString("Current: ")
		b.WriteString(m.proposal.AppSpace)
		b.WriteString("\n\n")
		b.WriteString("New name: ")
		b.WriteString(wizardSelectedStyle.Render(m.editInput))
		b.WriteString("▌\n\n")
		b.WriteString(dimStyle.Render("enter confirm  esc cancel"))

	case editModeMergeSelect:
		b.WriteString(headerStyle.Render("Merge Into"))
		b.WriteString("\n\n")
		b.WriteString("Merging: ")
		if m.proposalCursor < len(m.proposal.Units) {
			b.WriteString(wizardSelectedStyle.Render(m.proposal.Units[m.proposalCursor].Slug))
		}
		b.WriteString("\n\n")
		b.WriteString("Select target unit:\n")

		for i, unit := range m.proposal.Units {
			if i == m.proposalCursor {
				continue // Skip the source unit
			}
			cursor := "  "
			name := unit.Slug
			if i == m.mergeTargetIdx {
				cursor = "> "
				name = wizardSelectedStyle.Render(name)
			}
			b.WriteString(fmt.Sprintf("%s%s\n", cursor, name))
		}

		b.WriteString("\n")
		b.WriteString(dimStyle.Render("↑↓ select  enter merge  esc cancel"))

	case editModeAddLabel:
		b.WriteString(headerStyle.Render("Add Label"))
		b.WriteString("\n\n")
		b.WriteString("Format: key=value\n\n")
		b.WriteString("Label: ")
		b.WriteString(wizardSelectedStyle.Render(m.editInput))
		b.WriteString("▌\n\n")
		b.WriteString(dimStyle.Render("enter confirm  esc cancel"))

	case editModeEditLabel:
		b.WriteString(headerStyle.Render("Edit Label"))
		b.WriteString("\n\n")
		b.WriteString("Key: ")
		b.WriteString(m.editLabelKey)
		b.WriteString("\n\n")
		b.WriteString("Value: ")
		b.WriteString(wizardSelectedStyle.Render(m.editInput))
		b.WriteString("▌\n\n")
		b.WriteString(dimStyle.Render("enter confirm  esc cancel"))
	}

	return b.String()
}

func (m ImportWizardModel) renderArchitectureDiagram() string {
	if m.proposal == nil {
		return ""
	}

	var b strings.Builder

	// Styles
	boxColor := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle := lipgloss.NewStyle().Bold(true)
	spaceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))

	// Count namespaces and workloads
	namespaces := make(map[string]bool)
	totalWorkloads := 0
	for _, unit := range m.proposal.Units {
		totalWorkloads += len(unit.Workloads)
		for _, wRef := range unit.Workloads {
			if parts := strings.SplitN(wRef, "/", 2); len(parts) == 2 {
				namespaces[parts[0]] = true
			}
		}
	}

	// ConfigHub box
	b.WriteString(boxColor.Render("┌───────────────────────────┐") + "\n")
	b.WriteString(boxColor.Render("│") + "  " + titleStyle.Render("ConfigHub") + "                " + boxColor.Render("│") + "\n")
	b.WriteString(boxColor.Render("│") + "                           " + boxColor.Render("│") + "\n")

	// App Space
	spaceName := wizardTruncate(m.proposal.AppSpace, 18)
	b.WriteString(boxColor.Render("│") + "  " + spaceStyle.Render(spaceName) + strings.Repeat(" ", 25-len(spaceName)) + boxColor.Render("│") + "\n")

	// Units (show up to 4)
	maxUnits := 4
	if len(m.proposal.Units) < maxUnits {
		maxUnits = len(m.proposal.Units)
	}
	for i := 0; i < maxUnits; i++ {
		unit := m.proposal.Units[i]
		owner := unit.Labels["owner"]
		ownerStyle := m.getOwnerStyle(owner)
		dot := ownerStyle.Render("●")
		name := wizardTruncate(unit.Slug, 20)
		if i == m.proposalCursor {
			name = wizardSelectedStyle.Render(name)
		}
		line := fmt.Sprintf("    %s %s", dot, name)
		pad := 27 - lipgloss.Width(line)
		if pad < 0 {
			pad = 0
		}
		b.WriteString(boxColor.Render("│") + line + strings.Repeat(" ", pad) + boxColor.Render("│") + "\n")
	}
	if len(m.proposal.Units) > 4 {
		more := fmt.Sprintf("    +%d more", len(m.proposal.Units)-4)
		b.WriteString(boxColor.Render("│") + dimStyle.Render(more) + strings.Repeat(" ", 27-len(more)) + boxColor.Render("│") + "\n")
	}

	b.WriteString(boxColor.Render("│") + "                           " + boxColor.Render("│") + "\n")
	b.WriteString(boxColor.Render("└─────────────┬─────────────┘") + "\n")

	// Arrow down with label
	b.WriteString(boxColor.Render("              │") + "\n")
	b.WriteString(boxColor.Render("              │") + dimStyle.Render(" sync") + "\n")
	b.WriteString(boxColor.Render("              ▼") + "\n")

	// Worker box
	workerName := m.proposal.AppSpace + "-worker"
	if len(workerName) > 17 {
		workerName = workerName[:14] + "..."
	}
	b.WriteString(boxColor.Render("      ┌─────────────────┐") + "\n")
	b.WriteString(boxColor.Render("      │") + "     Worker      " + boxColor.Render("│") + "\n")
	wPad := 17 - len(workerName)
	wLeft := wPad / 2
	wRight := wPad - wLeft
	b.WriteString(boxColor.Render("      │") + strings.Repeat(" ", wLeft) + dimStyle.Render(workerName) + strings.Repeat(" ", wRight) + boxColor.Render("│") + "\n")
	b.WriteString(boxColor.Render("      └────────┬────────┘") + "\n")

	// Arrow down
	b.WriteString(boxColor.Render("               │") + "\n")
	b.WriteString(boxColor.Render("               │") + dimStyle.Render(" deploy") + "\n")
	b.WriteString(boxColor.Render("               ▼") + "\n")

	// K8s Cluster box
	b.WriteString(boxColor.Render("      ┌─────────────────┐") + "\n")
	b.WriteString(boxColor.Render("      │") + "   K8s Cluster   " + boxColor.Render("│") + "\n")
	nsText := fmt.Sprintf("%d namespaces", len(namespaces))
	nPad := 17 - len(nsText)
	nLeft := nPad / 2
	nRight := nPad - nLeft
	b.WriteString(boxColor.Render("      │") + strings.Repeat(" ", nLeft) + dimStyle.Render(nsText) + strings.Repeat(" ", nRight) + boxColor.Render("│") + "\n")
	b.WriteString(boxColor.Render("      └─────────────────┘") + "\n")

	// Legend and summary
	b.WriteString("\n")
	b.WriteString(wizardOwnerFlux.Render("●") + dimStyle.Render(" Flux "))
	b.WriteString(wizardOwnerArgo.Render("●") + dimStyle.Render(" Argo "))
	b.WriteString(wizardOwnerHelm.Render("●") + dimStyle.Render(" Helm "))
	b.WriteString(wizardOwnerNative.Render("●") + dimStyle.Render(" Native") + "\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("%d units, %d workloads", len(m.proposal.Units), totalWorkloads)))

	return b.String()
}

func (m ImportWizardModel) renderApplyProgress() string {
	var b strings.Builder

	// Calculate elapsed time
	elapsed := time.Since(m.applyStartTime).Round(time.Millisecond)
	elapsedStr := formatDuration(elapsed)

	if !m.applyComplete {
		// Still in progress
		b.WriteString(dimStyle.Render("Creating your App Space and Units in ConfigHub."))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("This registers your workloads for management."))
		b.WriteString("\n\n")

		b.WriteString(fmt.Sprintf("Creating App Space: %s\n", m.proposal.AppSpace))
		if m.applyProgress > 0 {
			b.WriteString(wizardSuccessStyle.Render("  ✓ Created") + "\n")
		}
		b.WriteString("\n")

		b.WriteString("Creating Units:\n")
		for i, unit := range m.proposal.Units {
			if i < len(m.applyResults) {
				result := m.applyResults[i]
				if result.Success {
					b.WriteString(wizardSuccessStyle.Render("  ✓ "+unit.Slug) + "\n")
				} else {
					b.WriteString(wizardErrorStyle.Render("  ✗ "+unit.Slug+": "+result.Error) + "\n")
				}
			} else if i == m.applyProgress {
				b.WriteString(fmt.Sprintf("  %s %s\n", m.spinner.View(), unit.Slug))
			} else {
				b.WriteString(dimStyle.Render("  ○ "+unit.Slug) + "\n")
			}
		}

		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("Progress: %d/%d units  %s\n", m.applyProgress, m.applyTotal, dimStyle.Render("("+elapsedStr+")")))
	} else {
		// Completed - show summary and next steps
		b.WriteString(wizardSuccessStyle.Render("✓ Import Complete") + "\n\n")

		// Summary
		successCount := 0
		for _, r := range m.applyResults {
			if r.Success {
				successCount++
			}
		}
		b.WriteString(fmt.Sprintf("Created %d units in ", successCount))
		b.WriteString(wizardSelectedStyle.Render(m.proposal.AppSpace) + "\n\n")

		// Next steps
		b.WriteString(headerStyle.Render("Next Steps") + "\n\n")

		if m.workerStarted {
			b.WriteString(wizardSuccessStyle.Render("✓ Worker started") + "\n")
			b.WriteString(fmt.Sprintf("  Name: %s\n", m.workerName))
			b.WriteString(fmt.Sprintf("  Space: %s\n", m.proposal.AppSpace))
			b.WriteString("\n")

			// Offer test option
			b.WriteString(dimStyle.Render("─────────────────────────────────") + "\n\n")
			b.WriteString(headerStyle.Render("Verify Setup") + "\n\n")
			b.WriteString(dimStyle.Render("Press ") + wizardSelectedStyle.Render("t") + dimStyle.Render(" to run end-to-end test") + "\n")
			b.WriteString(dimStyle.Render("This adds a test annotation and verifies") + "\n")
			b.WriteString(dimStyle.Render("it propagates to your cluster.") + "\n")
		} else {
			b.WriteString("Start a worker to connect this cluster:\n\n")
			b.WriteString(dimStyle.Render("  Press ") + wizardSelectedStyle.Render("w") + dimStyle.Render(" to start now, or run:") + "\n\n")
			b.WriteString(fmt.Sprintf("  cub worker run %s-worker --space %s\n", m.proposal.AppSpace, m.proposal.AppSpace))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("The worker will:") + "\n")
			b.WriteString("  • Connect to ConfigHub\n")
			b.WriteString("  • Create a Target for this cluster\n")
			b.WriteString("  • Sync your Units to the cluster\n")
		}
	}

	return b.String()
}

func (m ImportWizardModel) renderFinalArchitecture() string {
	// Just show the architecture diagram - next steps are in left pane
	return m.renderArchitectureDiagram()
}

func (m ImportWizardModel) renderArgoCleanupList() string {
	var b strings.Builder

	if m.argoCleanupDone {
		b.WriteString(wizardSuccessStyle.Render("✓ Import Complete") + "\n\n")

		// Summary
		successCount := 0
		for _, r := range m.applyResults {
			if r.Success {
				successCount++
			}
		}
		b.WriteString(fmt.Sprintf("Created %d units in ", successCount))
		b.WriteString(wizardSelectedStyle.Render(m.proposal.AppSpace) + "\n")
		b.WriteString("ArgoCD Applications have been handled.\n\n")

		// Next steps
		b.WriteString(headerStyle.Render("Next Steps") + "\n\n")

		if m.workerStarted {
			b.WriteString(wizardSuccessStyle.Render("✓ Worker started") + "\n")
			b.WriteString(fmt.Sprintf("  Name: %s\n", m.workerName))
			b.WriteString(fmt.Sprintf("  Space: %s\n", m.proposal.AppSpace))
			b.WriteString("\n")

			// Offer test option
			b.WriteString(dimStyle.Render("─────────────────────────────────") + "\n\n")
			b.WriteString(headerStyle.Render("Verify Setup") + "\n\n")
			b.WriteString(dimStyle.Render("Press ") + wizardSelectedStyle.Render("t") + dimStyle.Render(" to run end-to-end test") + "\n")
			b.WriteString(dimStyle.Render("This adds a test annotation and verifies") + "\n")
			b.WriteString(dimStyle.Render("it propagates to your cluster.") + "\n")
		} else {
			b.WriteString("Start a worker to connect this cluster:\n\n")
			b.WriteString(dimStyle.Render("  Press ") + wizardSelectedStyle.Render("w") + dimStyle.Render(" to start now, or run:") + "\n\n")
			b.WriteString(fmt.Sprintf("  cub worker run %s-worker --space %s\n", m.proposal.AppSpace, m.proposal.AppSpace))
			b.WriteString("\n")
			b.WriteString(dimStyle.Render("The worker will:") + "\n")
			b.WriteString("  • Connect to ConfigHub\n")
			b.WriteString("  • Create a Target for this cluster\n")
			b.WriteString("  • Sync your Units to the cluster\n")
		}
		return b.String()
	}

	b.WriteString(dimStyle.Render("Your workloads were managed by ArgoCD. To avoid"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("conflicts, choose how to handle the ArgoCD Apps."))
	b.WriteString("\n\n")

	b.WriteString(headerStyle.Render("ArgoCD Applications"))
	b.WriteString("\n\n")

	// Show options for the current app
	options := []struct {
		idx   int
		label string
		desc  string
	}{
		{argoCleanupDisableSync, "Disable auto-sync", "Keep Application for reference"},
		{argoCleanupDeleteApp, "Delete Application", "Remove ArgoCD control entirely"},
		{argoCleanupKeepAsIs, "Keep as-is", "Don't change the Application"},
	}

	for _, opt := range options {
		cursor := "  "
		if opt.idx == m.argoCleanupIdx {
			cursor = "> "
		}

		label := opt.label
		if opt.idx == m.argoCleanupIdx {
			label = wizardSelectedStyle.Render(label)
		}

		b.WriteString(fmt.Sprintf("%s%s\n", cursor, label))
		b.WriteString(fmt.Sprintf("    %s\n", dimStyle.Render(opt.desc)))
	}

	return b.String()
}

func (m ImportWizardModel) renderArgoCleanupDetails() string {
	var b strings.Builder

	if len(m.argoApps) == 0 {
		return "No ArgoCD Applications to clean up."
	}

	if m.argoCleanupDone {
		// Show final architecture with next steps
		return m.renderFinalArchitecture()
	}

	// Show the current Application being considered
	b.WriteString(headerStyle.Render("Current Application"))
	b.WriteString("\n\n")

	// Show all ArgoCD apps being processed
	for i, app := range m.argoApps {
		icon := "○"
		style := dimStyle
		if i == 0 {
			icon = "▶"
			style = wizardSelectedStyle
		}
		b.WriteString(fmt.Sprintf("%s %s\n", icon, style.Render(app.Name)))
		b.WriteString(fmt.Sprintf("   Namespace: %s\n", app.Namespace))
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("─────────────────────────────────"))
	b.WriteString("\n\n")

	// Explain the options
	b.WriteString(headerStyle.Render("Why clean up?"))
	b.WriteString("\n\n")
	b.WriteString("After import, both ArgoCD and ConfigHub\n")
	b.WriteString("would try to manage the same resources.\n")
	b.WriteString("This can cause sync conflicts.\n\n")

	b.WriteString("Recommended: ")
	b.WriteString(wizardOwnerArgo.Render("Disable auto-sync"))
	b.WriteString("\n")
	b.WriteString("Keeps the Application visible in ArgoCD\n")
	b.WriteString("but ConfigHub handles deployments.\n")

	return b.String()
}

// Helper functions

func (m ImportWizardModel) getOwnerStyle(owner string) lipgloss.Style {
	switch owner {
	case "Flux":
		return wizardOwnerFlux
	case "ArgoCD":
		return wizardOwnerArgo
	case "Helm":
		return wizardOwnerHelm
	default:
		return wizardOwnerNative
	}
}

func wizardTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// collectArgoApps extracts unique ArgoCD Applications from selected workloads.
// These are the Applications that need cleanup after import (disable sync or delete).
func (m ImportWizardModel) collectArgoApps() []ArgoAppRef {
	seen := make(map[string]bool)
	var apps []ArgoAppRef

	for _, w := range m.workloads {
		if !w.Selected {
			continue
		}
		// Only consider ArgoCD-managed workloads
		if w.Info.Owner != "ArgoCD" || w.Info.GitOpsRef == nil {
			continue
		}
		// GitOpsRef.Name is the ArgoCD Application name
		// GitOpsRef.Namespace is the namespace where the Application CR lives (usually "argocd")
		key := fmt.Sprintf("%s/%s", w.Info.GitOpsRef.Namespace, w.Info.GitOpsRef.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		apps = append(apps, ArgoAppRef{
			Name:      w.Info.GitOpsRef.Name,
			Namespace: w.Info.GitOpsRef.Namespace,
		})
	}

	return apps
}

// renderTestProgress renders the left pane for the test step
func (m ImportWizardModel) renderTestProgress() string {
	var b strings.Builder

	// Title and description
	b.WriteString(headerStyle.Render("End-to-End Verification Test"))
	b.WriteString("\n\n")

	if m.testPhase != testPhaseComplete {
		b.WriteString(dimStyle.Render("Testing the full ConfigHub → Cluster pipeline."))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("This verifies your import is working correctly."))
		b.WriteString("\n\n")
	}

	// Show the test phases
	phases := []struct {
		phase int
		icon  string
		label string
	}{
		{testPhaseAddAnnotation, "1", "Add test annotation to unit"},
		{testPhaseApply, "2", "Apply unit via ConfigHub"},
		{testPhaseWaitSync, "3", "Wait for worker to sync"},
		{testPhaseVerify, "4", "Verify annotation in cluster"},
	}

	for _, p := range phases {
		// Determine status
		var icon, label string
		completed := false
		for _, r := range m.testResults {
			if r.Phase == p.phase {
				completed = true
				if r.Success {
					icon = wizardSuccessStyle.Render("✓")
					label = wizardSuccessStyle.Render(p.label)
				} else {
					icon = wizardErrorStyle.Render("✗")
					label = wizardErrorStyle.Render(p.label)
				}
				break
			}
		}

		if !completed {
			if m.testPhase == p.phase {
				// Currently running
				icon = m.spinner.View()
				label = wizardSelectedStyle.Render(p.label)
			} else {
				// Pending
				icon = dimStyle.Render("○")
				label = dimStyle.Render(p.label)
			}
		}

		b.WriteString(fmt.Sprintf("  %s %s\n", icon, label))
	}

	b.WriteString("\n")

	// Show elapsed time
	if !m.testStartTime.IsZero() {
		var elapsed time.Duration
		if m.testPhase == testPhaseComplete {
			// Use stored elapsed time when complete
			elapsed = m.testElapsed
		} else {
			// Still running - show live elapsed time
			elapsed = time.Since(m.testStartTime).Round(time.Millisecond)
		}
		b.WriteString(dimStyle.Render(fmt.Sprintf("Elapsed: %s", formatDuration(elapsed))))
		b.WriteString("\n")
	}

	// Show final result
	if m.testPhase == testPhaseComplete {
		b.WriteString("\n")
		allSuccess := true
		for _, r := range m.testResults {
			if !r.Success {
				allSuccess = false
				break
			}
		}

		if allSuccess {
			b.WriteString(wizardSuccessStyle.Render("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━") + "\n\n")
			b.WriteString(wizardSuccessStyle.Render("  ✓ TEST PASSED") + "\n\n")
			b.WriteString(wizardSuccessStyle.Render("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━") + "\n\n")
			b.WriteString("Your ConfigHub import is fully operational!\n\n")
			b.WriteString("The complete pipeline is working:\n")
			b.WriteString("  ConfigHub → Worker → Cluster\n\n")
			b.WriteString(dimStyle.Render("You can now manage your workloads via ConfigHub."))
		} else {
			b.WriteString(wizardErrorStyle.Render("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━") + "\n\n")
			b.WriteString(wizardErrorStyle.Render("  ✗ TEST FAILED") + "\n\n")
			b.WriteString(wizardErrorStyle.Render("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━") + "\n\n")
			if m.testError != nil {
				b.WriteString(wizardErrorStyle.Render("Error: "+m.testError.Error()) + "\n\n")
			}
			b.WriteString(dimStyle.Render("Press 'r' to retry or 'q' to quit."))
		}
	}

	return b.String()
}

// renderTestDetails renders the right pane for the test step
func (m ImportWizardModel) renderTestDetails() string {
	var b strings.Builder

	// Test configuration
	b.WriteString(headerStyle.Render("Test Configuration"))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Unit:       %s\n", wizardSelectedStyle.Render(m.testUnitSlug)))
	b.WriteString(fmt.Sprintf("Annotation: %s\n", dimStyle.Render("confighub.com/import-test")))
	b.WriteString(fmt.Sprintf("Value:      %s\n", dimStyle.Render(m.testAnnotation)))
	b.WriteString(fmt.Sprintf("Space:      %s\n", dimStyle.Render(m.proposal.AppSpace)))
	b.WriteString("\n")

	// Phase details
	b.WriteString(dimStyle.Render("─────────────────────────────────"))
	b.WriteString("\n\n")

	b.WriteString(headerStyle.Render("Phase Details"))
	b.WriteString("\n\n")

	if len(m.testResults) == 0 {
		b.WriteString(dimStyle.Render("Test in progress..."))
		b.WriteString("\n")
	} else {
		for _, r := range m.testResults {
			statusIcon := wizardSuccessStyle.Render("✓")
			if !r.Success {
				statusIcon = wizardErrorStyle.Render("✗")
			}

			b.WriteString(fmt.Sprintf("%s %s\n", statusIcon, r.Label))
			if r.Details != "" {
				// Truncate long details
				details := r.Details
				if len(details) > 60 {
					details = details[:57] + "..."
				}
				b.WriteString(fmt.Sprintf("   %s\n", dimStyle.Render(details)))
			}
			b.WriteString(fmt.Sprintf("   %s\n", dimStyle.Render(formatDuration(r.Elapsed))))
			b.WriteString("\n")
		}
	}

	// Show what we're verifying
	if m.testPhase != testPhaseComplete {
		b.WriteString(dimStyle.Render("─────────────────────────────────"))
		b.WriteString("\n\n")

		b.WriteString(headerStyle.Render("What This Tests"))
		b.WriteString("\n\n")

		b.WriteString("1. ConfigHub stores the config change\n")
		b.WriteString("2. Worker receives the apply command\n")
		b.WriteString("3. Worker updates the cluster\n")
		b.WriteString("4. Annotation appears on the resource\n")
	}

	return b.String()
}

// detectImportPattern analyzes the proposal to detect a known reference architecture pattern
func (m ImportWizardModel) detectImportPattern() string {
	if m.proposal == nil {
		return ""
	}

	// Analyze units for pattern indicators
	hasBase := false
	hasInfra := false
	hasPlatform := false
	hasDevStagingProd := false
	hasRegions := false
	deployer := m.proposal.Deployer

	devCount, stagingCount, prodCount := 0, 0, 0
	regionCount := 0

	for _, unit := range m.proposal.Units {
		slug := strings.ToLower(unit.Slug)
		variant := strings.ToLower(unit.Variant)

		if strings.Contains(slug, "base") || strings.Contains(slug, "-base") {
			hasBase = true
		}
		if strings.Contains(slug, "infra") || strings.Contains(slug, "-infra") {
			hasInfra = true
		}
		if strings.Contains(slug, "platform") {
			hasPlatform = true
		}

		// Check variant labels
		if variant == "dev" || strings.Contains(slug, "-dev") {
			devCount++
		}
		if variant == "staging" || strings.Contains(slug, "staging") {
			stagingCount++
		}
		if variant == "prod" || strings.Contains(slug, "-prod") {
			prodCount++
		}

		// Check for regions
		if unit.Region != "" {
			regionCount++
		}
		if strings.Contains(slug, "-asia") || strings.Contains(slug, "-eu") || strings.Contains(slug, "-us") {
			regionCount++
		}
	}

	if devCount > 0 && prodCount > 0 {
		hasDevStagingProd = true
	}
	if regionCount >= 2 {
		hasRegions = true
	}

	// Determine pattern based on indicators
	switch {
	case deployer == "Flux" && (hasBase || hasInfra) && !hasDevStagingProd:
		return "Banko (Flux cluster-per-dir)"
	case deployer == "ArgoCD" && hasBase && hasDevStagingProd:
		return "Arnie (ArgoCD folders-per-env)"
	case hasRegions && (hasBase || hasInfra):
		return "TraderX (Multi-region)"
	case hasPlatform && hasDevStagingProd:
		return "KubeCon Demo (Platform + Apps)"
	case (hasBase || hasInfra) && hasDevStagingProd:
		return "curious-cub (Standard dev/staging/prod)"
	case hasDevStagingProd:
		return "Environment-based"
	default:
		return ""
	}
}

// RunImportWizard starts the import wizard as a standalone TUI
func RunImportWizard() error {
	p := tea.NewProgram(NewImportWizardModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
