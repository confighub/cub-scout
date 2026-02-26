// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/confighub/cub-scout/pkg/agent"
)

// Debug wizard steps
const (
	DebugStepSelectMode = iota
	DebugStepPickResource
	DebugStepWorkloadStatus
	DebugStepContainerLogs // Step for viewing container logs
	DebugStepEventTimeline // Step for viewing event timeline
	DebugStepOwnership
	DebugStepPipelineHealth
	DebugStepSourceHealth
	DebugStepRootCause
	DebugStepSummary
)

// DebugEntryMode defines how user entered debug mode
type DebugEntryMode int

const (
	EntryModeBrokenWorkload DebugEntryMode = iota
	EntryModeFailingPipeline
	EntryModeSyncIssue
	EntryModeFreeform
)

// String returns the display name for the entry mode
func (m DebugEntryMode) String() string {
	switch m {
	case EntryModeBrokenWorkload:
		return "Broken workload"
	case EntryModeFailingPipeline:
		return "Failing pipeline"
	case EntryModeSyncIssue:
		return "Sync issue"
	case EntryModeFreeform:
		return "Freeform"
	default:
		return "Unknown"
	}
}

// Description returns the description for the entry mode
func (m DebugEntryMode) Description() string {
	switch m {
	case EntryModeBrokenWorkload:
		return "A Deployment or StatefulSet isn't working correctly"
	case EntryModeFailingPipeline:
		return "A Kustomization, HelmRelease, or Argo Application is failing"
	case EntryModeSyncIssue:
		return "Git changes aren't reaching the cluster"
	case EntryModeFreeform:
		return "Pick any resource and trace its GitOps chain"
	default:
		return ""
	}
}

// DebugSession tracks the complete state of a debug session
type DebugSession struct {
	// Entry mode
	EntryMode DebugEntryMode `json:"entryMode"`

	// Selected target
	Target ResourceRef `json:"target"`

	// Workload health (Step 3)
	WorkloadStatus *WorkloadHealthStatus `json:"workloadStatus,omitempty"`

	// Container logs (optional, fetched on demand)
	ContainerLogs []*ContainerLogResult `json:"containerLogs,omitempty"`

	// Event timeline (optional, fetched on demand)
	EventTimeline *EventTimelineResult `json:"eventTimeline,omitempty"`

	// Ownership chain (Step 4)
	OwnershipChain *OwnershipChainResult `json:"ownershipChain,omitempty"`

	// Pipeline health (Step 5)
	DeployerStatus *DeployerStatus `json:"deployerStatus,omitempty"`

	// Source health (Step 6)
	SourceStatus *SourceStatus `json:"sourceStatus,omitempty"`

	// Root cause analysis (Step 7)
	RootCause *RootCauseAnalysis `json:"rootCause,omitempty"`

	// Timestamps
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
}

// ContainerLogResult holds container log data for the debug session
type ContainerLogResult struct {
	PodName       string       `json:"podName"`
	ContainerName string       `json:"containerName"`
	Namespace     string       `json:"namespace"`
	Lines         []string     `json:"lines"`
	TotalLines    int          `json:"totalLines"`
	Truncated     bool         `json:"truncated"`
	Previous      bool         `json:"previous"`
	Patterns      []LogPattern `json:"patterns,omitempty"`
	Error         string       `json:"error,omitempty"`
}

// LogPattern represents a detected pattern in logs
type LogPattern struct {
	Type        string `json:"type"`
	Line        int    `json:"line"`
	Match       string `json:"match"`
	Explanation string `json:"explanation"`
	Suggestion  string `json:"suggestion"`
}

// EventTimelineResult holds the event timeline for a workload
type EventTimelineResult struct {
	WorkloadEvents []TimelineEvent `json:"workloadEvents,omitempty"`
	PodEvents      []TimelineEvent `json:"podEvents,omitempty"`
	AllEvents      []TimelineEvent `json:"allEvents"`
}

// TimelineEvent represents an event in the timeline
type TimelineEvent struct {
	Type           string     `json:"type"` // Normal, Warning
	Reason         string     `json:"reason"`
	Message        string     `json:"message"`
	Count          int32      `json:"count"`
	FirstTimestamp *time.Time `json:"firstTimestamp,omitempty"`
	LastTimestamp  *time.Time `json:"lastTimestamp,omitempty"`
	ResourceKind   string     `json:"resourceKind"`
	ResourceName   string     `json:"resourceName"`
	Explanation    string     `json:"explanation,omitempty"`
	Suggestion     string     `json:"suggestion,omitempty"`
	Severity       string     `json:"severity"` // info, warning, error
}

// ResourceRef identifies a Kubernetes resource
type ResourceRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// String returns kind/name format
func (r ResourceRef) String() string {
	if r.Namespace != "" {
		return r.Kind + "/" + r.Name + " in " + r.Namespace
	}
	return r.Kind + "/" + r.Name
}

// WorkloadHealthStatus contains health info for a workload
type WorkloadHealthStatus struct {
	Kind                string `json:"kind"`
	Name                string `json:"name"`
	Namespace           string `json:"namespace"`
	Replicas            int32  `json:"replicas"`
	ReadyReplicas       int32  `json:"readyReplicas"`
	UnavailableReplicas int32  `json:"unavailableReplicas"`

	// Pod-level issues
	PodIssues []PodIssue `json:"podIssues,omitempty"`

	// Recent events
	RecentEvents []K8sEvent `json:"recentEvents,omitempty"`
}

// IsHealthy returns true if all replicas are ready
func (w *WorkloadHealthStatus) IsHealthy() bool {
	return w.ReadyReplicas >= w.Replicas && len(w.PodIssues) == 0
}

// PodIssue represents a problem with a specific pod
type PodIssue struct {
	PodName         string `json:"podName"`
	Phase           string `json:"phase"`
	ContainerStatus string `json:"containerStatus"`
	Reason          string `json:"reason,omitempty"`
	Message         string `json:"message,omitempty"`
	RestartCount    int32  `json:"restartCount"`
}

// K8sEvent represents a Kubernetes event
type K8sEvent struct {
	Type      string    `json:"type"` // Normal, Warning
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Count     int32     `json:"count"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

// OwnershipChainResult wraps the agent's reverse trace result
type OwnershipChainResult struct {
	Owner        string           `json:"owner"` // flux, argo, helm, native
	OwnerDetails *agent.Ownership `json:"ownerDetails,omitempty"`
	K8sChain     []ChainLink      `json:"k8sChain,omitempty"`
	GitOpsChain  []ChainLink      `json:"gitopsChain,omitempty"`
}

// ChainLink represents a link in the ownership chain
type ChainLink struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Ready     bool   `json:"ready"`
	Status    string `json:"status,omitempty"`
}

// RootCauseAnalysis is the final diagnosis
type RootCauseAnalysis struct {
	// Classification
	Category   string `json:"category"`   // source_auth, source_fetch, build_error, apply_error, workload_crash
	Stage      string `json:"stage"`      // source, build, apply, sync, workload
	Confidence string `json:"confidence"` // high, medium, low

	// Details
	Summary        string   `json:"summary"`
	Explanation    string   `json:"explanation,omitempty"`
	ProbableCauses []string `json:"probableCauses,omitempty"`
	SuggestedFixes []string `json:"suggestedFixes,omitempty"`
	Commands       []string `json:"commands,omitempty"`

	// Education
	LearnMoreURL    string   `json:"learnMoreUrl,omitempty"`
	RelatedConcepts []string `json:"relatedConcepts,omitempty"`
}

// DebugModel is the Bubbletea model for the debug wizard
type DebugModel struct {
	// Navigation
	step int

	// Step 1: Entry mode selection
	entryModes      []DebugEntryMode
	entryModeCursor int

	// Step 2: Resource picker
	resources      []ResourceItem
	resourceCursor int

	// Step: Container logs
	logPodCursor    int  // Which pod's logs to show
	logScrollPos    int  // Scroll position in logs
	logShowPrevious bool // Show previous container logs

	// Step: Event timeline
	eventScrollPos int  // Scroll position in event list
	eventShowAll   bool // Show all events vs just warnings/errors

	// Session state
	session *DebugSession

	// UI components
	spinner    spinner.Model
	width      int
	height     int
	loading    bool
	loadingMsg string

	// K8s clients
	dynClient dynamic.Interface
	clientset *kubernetes.Clientset
	ctx       context.Context

	// Result
	quit bool
	err  error

	// Help/Education overlay
	showHelp bool

	// Status message (shown briefly after clipboard/export actions)
	statusMsg string
}

// ResourceItem represents a resource in the picker
type ResourceItem struct {
	Kind      string
	Name      string
	Namespace string
	Status    string // e.g., "1/3 ready", "CrashLoopBackOff"
	Owner     string // flux, argo, helm, native
}

// NewDebugModel creates a new debug model
func NewDebugModel(ctx context.Context, cfg *rest.Config) (DebugModel, error) {
	// Create dynamic client
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return DebugModel{}, err
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return DebugModel{}, err
	}

	// Initialize spinner
	s := spinner.New()
	s.Spinner = spinner.Dot

	return DebugModel{
		step: DebugStepSelectMode,
		entryModes: []DebugEntryMode{
			EntryModeBrokenWorkload,
			EntryModeFailingPipeline,
			EntryModeSyncIssue,
			EntryModeFreeform,
		},
		session: &DebugSession{
			StartedAt: time.Now(),
		},
		spinner:   s,
		dynClient: dynClient,
		clientset: clientset,
		ctx:       ctx,
	}, nil
}

// Init implements tea.Model
func (m DebugModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model
func (m DebugModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Global keys
		switch msg.String() {
		case "ctrl+c", "q":
			m.quit = true
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
			return m, nil
		}

		// If showing help, any key closes it
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}

		// Step-specific key handling
		return m.handleStepKeys(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case loadResourcesMsg:
		m.loading = false
		m.resources = msg.resources
		return m, nil

	case analysisCompleteMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		m.session = msg.session
		m.step = DebugStepSummary
		return m, nil

	case containerLogsMsg:
		m.loading = false
		if msg.err != nil {
			// Don't fail, just show error in UI
			m.session.ContainerLogs = nil
		} else {
			m.session.ContainerLogs = msg.logs
		}
		m.logPodCursor = 0
		m.logScrollPos = 0
		m.step = DebugStepContainerLogs
		return m, nil

	case eventTimelineMsg:
		m.loading = false
		if msg.err != nil {
			// Don't fail, just show error in UI
			m.session.EventTimeline = nil
		} else {
			m.session.EventTimeline = msg.timeline
		}
		m.eventScrollPos = 0
		m.step = DebugStepEventTimeline
		return m, nil
	}

	return m, nil
}

// handleStepKeys handles key presses for the current step
func (m DebugModel) handleStepKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case DebugStepSelectMode:
		return m.handleSelectModeKeys(msg)
	case DebugStepPickResource:
		return m.handlePickResourceKeys(msg)
	case DebugStepWorkloadStatus:
		return m.handleWorkloadStatusKeys(msg)
	case DebugStepContainerLogs:
		return m.handleContainerLogsKeys(msg)
	case DebugStepEventTimeline:
		return m.handleEventTimelineKeys(msg)
	case DebugStepSummary:
		return m.handleSummaryKeys(msg)
	default:
		// For intermediate steps, Enter advances
		if msg.String() == "enter" {
			m.step++
			return m, nil
		}
	}
	return m, nil
}

// handleWorkloadStatusKeys handles keys for workload status step
func (m DebugModel) handleWorkloadStatusKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "l":
		// View logs - fetch logs and go to logs step
		if m.session.WorkloadStatus != nil && len(m.session.WorkloadStatus.PodIssues) > 0 {
			m.loading = true
			m.loadingMsg = "Fetching container logs..."
			return m, m.fetchContainerLogs()
		}
	case "e":
		// View events - fetch event timeline
		if m.session.WorkloadStatus != nil {
			m.loading = true
			m.loadingMsg = "Fetching event timeline..."
			return m, m.fetchEventTimeline()
		}
	case "enter":
		// Skip logs/events steps and go to ownership
		m.step = DebugStepOwnership
	}
	return m, nil
}

// handleContainerLogsKeys handles keys for container logs step
func (m DebugModel) handleContainerLogsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.logScrollPos > 0 {
			m.logScrollPos--
		}
	case "down", "j":
		if m.session.ContainerLogs != nil && len(m.session.ContainerLogs) > 0 {
			logResult := m.session.ContainerLogs[m.logPodCursor]
			if m.logScrollPos < len(logResult.Lines)-15 {
				m.logScrollPos++
			}
		}
	case "left", "h":
		if m.logPodCursor > 0 {
			m.logPodCursor--
			m.logScrollPos = 0
		}
	case "right", "l":
		if m.session.ContainerLogs != nil && m.logPodCursor < len(m.session.ContainerLogs)-1 {
			m.logPodCursor++
			m.logScrollPos = 0
		}
	case "p":
		// Toggle previous logs
		m.logShowPrevious = !m.logShowPrevious
		m.loading = true
		m.loadingMsg = "Fetching logs..."
		return m, m.fetchContainerLogs()
	case "esc":
		m.step = DebugStepWorkloadStatus
	case "enter":
		m.step = DebugStepOwnership
	}
	return m, nil
}

// handleEventTimelineKeys handles keys for event timeline step
func (m DebugModel) handleEventTimelineKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.eventScrollPos > 0 {
			m.eventScrollPos--
		}
	case "down", "j":
		if m.session.EventTimeline != nil {
			maxScroll := len(m.session.EventTimeline.AllEvents) - 10
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.eventScrollPos < maxScroll {
				m.eventScrollPos++
			}
		}
	case "a":
		// Toggle show all events vs warnings/errors only
		m.eventShowAll = !m.eventShowAll
		m.eventScrollPos = 0
	case "esc":
		m.step = DebugStepWorkloadStatus
	case "enter":
		m.step = DebugStepOwnership
	}
	return m, nil
}

// handleSelectModeKeys handles keys for step 1
func (m DebugModel) handleSelectModeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.entryModeCursor > 0 {
			m.entryModeCursor--
		}
	case "down", "j":
		if m.entryModeCursor < len(m.entryModes)-1 {
			m.entryModeCursor++
		}
	case "enter":
		m.session.EntryMode = m.entryModes[m.entryModeCursor]
		m.step = DebugStepPickResource
		m.loading = true
		m.loadingMsg = "Loading resources..."
		return m, m.loadResources()
	}
	return m, nil
}

// handlePickResourceKeys handles keys for step 2
func (m DebugModel) handlePickResourceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.resourceCursor > 0 {
			m.resourceCursor--
		}
	case "down", "j":
		if m.resourceCursor < len(m.resources)-1 {
			m.resourceCursor++
		}
	case "enter":
		if len(m.resources) > 0 {
			selected := m.resources[m.resourceCursor]
			m.session.Target = ResourceRef{
				Kind:      selected.Kind,
				Name:      selected.Name,
				Namespace: selected.Namespace,
			}
			m.loading = true
			m.loadingMsg = "Analyzing..."
			return m, m.runAnalysis()
		}
	case "esc":
		m.step = DebugStepSelectMode
	}
	return m, nil
}

// handleSummaryKeys handles keys for the summary step
func (m DebugModel) handleSummaryKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "c":
		text := m.plainTextSummary()
		if err := copyToClipboard(text); err != nil {
			m.statusMsg = fmt.Sprintf("Clipboard error: %v", err)
		} else {
			m.statusMsg = "Copied to clipboard"
		}
		return m, nil
	case "e":
		text := m.plainTextSummary()
		path, err := exportDebugSummary(text)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Export error: %v", err)
		} else {
			m.statusMsg = fmt.Sprintf("Exported to %s", path)
		}
		return m, nil
	case "enter", "q":
		return m, tea.Quit
	}
	return m, nil
}

// plainTextSummary generates an unstyled plain-text version of the debug summary.
func (m DebugModel) plainTextSummary() string {
	var b strings.Builder

	b.WriteString("cub-scout debug summary\n")
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	b.WriteString(fmt.Sprintf("Target: %s\n", m.session.Target.String()))
	b.WriteString(fmt.Sprintf("Time:   %s\n\n", m.session.StartedAt.Format(time.RFC3339)))

	if m.session.WorkloadStatus != nil {
		ws := m.session.WorkloadStatus
		if ws.IsHealthy() {
			b.WriteString("Workload: Healthy\n")
		} else {
			b.WriteString(fmt.Sprintf("Workload: %d/%d ready\n", ws.ReadyReplicas, ws.Replicas))
		}
	}

	if m.session.OwnershipChain != nil {
		b.WriteString(fmt.Sprintf("Owner:    %s\n", m.session.OwnershipChain.Owner))
	}

	if m.session.DeployerStatus != nil {
		ds := m.session.DeployerStatus
		if ds.Ready {
			b.WriteString("Pipeline: Healthy\n")
		} else {
			b.WriteString(fmt.Sprintf("Pipeline: %s\n", ds.Stage))
		}
	}

	if m.session.SourceStatus != nil {
		ss := m.session.SourceStatus
		if ss.Ready {
			b.WriteString("Source:   Healthy\n")
		} else {
			b.WriteString(fmt.Sprintf("Source:   %s\n", ss.Reason))
		}
	}

	if m.session.RootCause != nil {
		b.WriteString("\n" + strings.Repeat("-", 60) + "\n\n")
		b.WriteString(fmt.Sprintf("Root Cause: %s\n\n", m.session.RootCause.Summary))

		if len(m.session.RootCause.SuggestedFixes) > 0 {
			b.WriteString("Next Steps:\n")
			for i, fix := range m.session.RootCause.SuggestedFixes {
				b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, fix))
			}
		}
	}

	return b.String()
}

// copyToClipboard copies text to the system clipboard.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		// Try xclip first, fall back to xsel
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard tool found (install xclip or xsel)")
		}
	default:
		return fmt.Errorf("clipboard not supported on %s", runtime.GOOS)
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// exportDebugSummary writes the debug summary to a timestamped file and returns the path.
func exportDebugSummary(text string) (string, error) {
	filename := fmt.Sprintf("cub-scout-debug-%s.txt", time.Now().Format("20060102-150405"))
	path := filepath.Join(".", filename)
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// View implements tea.Model
func (m DebugModel) View() string {
	if m.quit {
		return ""
	}

	// Show help overlay
	if m.showHelp {
		return m.renderHelpView()
	}

	// Show loading
	if m.loading {
		return m.renderLoadingView()
	}

	// Render current step
	switch m.step {
	case DebugStepSelectMode:
		return m.renderSelectModeView()
	case DebugStepPickResource:
		return m.renderPickResourceView()
	case DebugStepSummary:
		return m.renderSummaryView()
	default:
		return m.renderAnalysisView()
	}
}

// Message types for async operations
type loadResourcesMsg struct {
	resources []ResourceItem
	err       error
}

type analysisCompleteMsg struct {
	session *DebugSession
	err     error
}

type containerLogsMsg struct {
	logs []*ContainerLogResult
	err  error
}

type eventTimelineMsg struct {
	timeline *EventTimelineResult
	err      error
}

// loadResources fetches resources based on entry mode
func (m DebugModel) loadResources() tea.Cmd {
	return func() tea.Msg {
		resources, err := fetchResourcesForMode(m.ctx, m.dynClient, m.clientset, m.session.EntryMode, debugNamespace)
		return loadResourcesMsg{resources: resources, err: err}
	}
}

// runAnalysis runs the full debug analysis
func (m DebugModel) runAnalysis() tea.Cmd {
	return func() tea.Msg {
		session, err := runDebugAnalysisFromModel(m.ctx, m.dynClient, m.clientset, m.session)
		return analysisCompleteMsg{session: session, err: err}
	}
}

// fetchContainerLogs fetches logs for pods with issues
func (m DebugModel) fetchContainerLogs() tea.Cmd {
	return func() tea.Msg {
		if m.session.WorkloadStatus == nil || len(m.session.WorkloadStatus.PodIssues) == 0 {
			return containerLogsMsg{logs: nil, err: nil}
		}

		fetcher := agent.NewContainerLogFetcher(m.clientset)
		var logs []*ContainerLogResult

		// Fetch logs for each pod with issues
		for _, issue := range m.session.WorkloadStatus.PodIssues {
			result, err := fetcher.FetchLogs(m.ctx,
				m.session.WorkloadStatus.Namespace,
				issue.PodName,
				"", // All containers
				20, // Last 20 lines
				m.logShowPrevious,
			)
			if err != nil {
				continue
			}

			// Convert agent type to local type
			localResult := &ContainerLogResult{
				PodName:       result.PodName,
				ContainerName: result.ContainerName,
				Namespace:     result.Namespace,
				Lines:         result.Lines,
				TotalLines:    result.TotalLines,
				Truncated:     result.Truncated,
				Previous:      result.Previous,
				Error:         result.Error,
			}

			// Convert patterns
			for _, p := range result.Patterns {
				localResult.Patterns = append(localResult.Patterns, LogPattern{
					Type:        p.Type,
					Line:        p.Line,
					Match:       p.Match,
					Explanation: p.Explanation,
					Suggestion:  p.Suggestion,
				})
			}

			logs = append(logs, localResult)
		}

		return containerLogsMsg{logs: logs, err: nil}
	}
}

// fetchEventTimeline fetches events for the workload and related pods
func (m DebugModel) fetchEventTimeline() tea.Cmd {
	return func() tea.Msg {
		if m.session.WorkloadStatus == nil {
			return eventTimelineMsg{timeline: nil, err: nil}
		}

		fetcher := agent.NewEventTimelineFetcher(m.clientset)

		// Get pod names from pod issues
		var podNames []string
		for _, issue := range m.session.WorkloadStatus.PodIssues {
			podNames = append(podNames, issue.PodName)
		}

		agentTimeline, err := fetcher.FetchForWorkload(
			m.ctx,
			m.session.WorkloadStatus.Namespace,
			m.session.WorkloadStatus.Kind,
			m.session.WorkloadStatus.Name,
			podNames,
		)
		if err != nil {
			return eventTimelineMsg{timeline: nil, err: err}
		}

		// Convert agent timeline to local type
		result := &EventTimelineResult{
			AllEvents: make([]TimelineEvent, len(agentTimeline.AllEvents)),
		}

		for i, ae := range agentTimeline.AllEvents {
			result.AllEvents[i] = TimelineEvent{
				Type:           ae.Type,
				Reason:         ae.Reason,
				Message:        ae.Message,
				Count:          ae.Count,
				FirstTimestamp: ae.FirstTimestamp,
				LastTimestamp:  ae.LastTimestamp,
				ResourceKind:   ae.ResourceKind,
				ResourceName:   ae.ResourceName,
				Explanation:    ae.Explanation,
				Suggestion:     ae.Suggestion,
				Severity:       ae.Severity,
			}
		}

		return eventTimelineMsg{timeline: result, err: nil}
	}
}

// fetchResourcesForMode fetches resources based on the debug entry mode
func fetchResourcesForMode(ctx context.Context, dynClient dynamic.Interface, clientset *kubernetes.Clientset, mode DebugEntryMode, namespace string) ([]ResourceItem, error) {
	switch mode {
	case EntryModeBrokenWorkload:
		return fetchUnhealthyWorkloads(ctx, dynClient, clientset, namespace)
	case EntryModeFailingPipeline:
		return fetchFailingPipelines(ctx, dynClient, namespace)
	case EntryModeSyncIssue:
		return fetchSyncIssues(ctx, dynClient, namespace)
	case EntryModeFreeform:
		return fetchAllWorkloads(ctx, dynClient, clientset, namespace)
	default:
		return nil, nil
	}
}

// fetchUnhealthyWorkloads uses the WorkloadHealthChecker to find unhealthy workloads
func fetchUnhealthyWorkloads(ctx context.Context, dynClient dynamic.Interface, clientset *kubernetes.Clientset, namespace string) ([]ResourceItem, error) {
	checker := agent.NewWorkloadHealthChecker(dynClient, clientset)
	workloads, err := checker.FindUnhealthyWorkloads(ctx, namespace)
	if err != nil {
		return nil, err
	}

	var items []ResourceItem
	for _, w := range workloads {
		items = append(items, ResourceItem{
			Kind:      w.Kind,
			Name:      w.Name,
			Namespace: w.Namespace,
			Status:    w.StatusSummary(),
		})
	}
	return items, nil
}

func fetchFailingPipelines(ctx context.Context, dynClient dynamic.Interface, namespace string) ([]ResourceItem, error) {
	// Use ApplyBackendDetector to get deployers and check their status
	detector := agent.NewApplyBackendDetector(dynClient)
	info, err := detector.DetectBackend(ctx, namespace)
	if err != nil {
		return nil, err
	}

	var items []ResourceItem
	for _, d := range info.Deployers {
		// Get the deployer resource to check its status
		gvr, err := agent.KindToGVR(d.Kind)
		if err != nil {
			continue
		}

		obj, err := dynClient.Resource(gvr).Namespace(d.Namespace).Get(ctx, d.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}

		var failure agent.FailureDetails
		switch d.Kind {
		case "Kustomization", "HelmRelease":
			failure = agent.ExtractFluxDeployerFailure(obj)
		case "Application":
			failure = agent.ExtractArgoFailure(obj)
		}

		if !failure.IsHealthy() {
			status := string(failure.Stage)
			if failure.Reason != "" {
				status = failure.Reason
			}
			items = append(items, ResourceItem{
				Kind:      d.Kind,
				Name:      d.Name,
				Namespace: d.Namespace,
				Status:    status,
			})
		}
	}

	return items, nil
}

func fetchSyncIssues(ctx context.Context, dynClient dynamic.Interface, namespace string) ([]ResourceItem, error) {
	// Use ApplyBackendDetector to get sources and check their status
	detector := agent.NewApplyBackendDetector(dynClient)
	info, err := detector.DetectBackend(ctx, namespace)
	if err != nil {
		return nil, err
	}

	var items []ResourceItem
	for _, s := range info.Sources {
		// Get the source resource to check its status
		gvr, err := agent.KindToGVR(s.Kind)
		if err != nil {
			continue
		}

		obj, err := dynClient.Resource(gvr).Namespace(s.Namespace).Get(ctx, s.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}

		failure := agent.ExtractFluxSourceFailure(obj)
		if !failure.IsHealthy() {
			status := string(failure.Stage)
			if failure.Reason != "" {
				status = failure.Reason
			}
			items = append(items, ResourceItem{
				Kind:      s.Kind,
				Name:      s.Name,
				Namespace: s.Namespace,
				Status:    status,
			})
		}
	}

	return items, nil
}

func fetchAllWorkloads(ctx context.Context, dynClient dynamic.Interface, clientset *kubernetes.Clientset, namespace string) ([]ResourceItem, error) {
	checker := agent.NewWorkloadHealthChecker(dynClient, clientset)
	workloads, err := checker.FindAllWorkloads(ctx, namespace)
	if err != nil {
		return nil, err
	}

	var items []ResourceItem
	for _, w := range workloads {
		items = append(items, ResourceItem{
			Kind:      w.Kind,
			Name:      w.Name,
			Namespace: w.Namespace,
			Status:    w.StatusSummary(),
		})
	}
	return items, nil
}

func runDebugAnalysisFromModel(ctx context.Context, dynClient dynamic.Interface, clientset *kubernetes.Clientset, session *DebugSession) (*DebugSession, error) {
	target := session.Target

	// Step 1: Get workload health (for workload kinds)
	if isWorkloadKind(target.Kind) {
		checker := agent.NewWorkloadHealthChecker(dynClient, clientset)
		health, err := checker.GetWorkloadHealth(ctx, target.Kind, target.Name, target.Namespace)
		if err == nil && health != nil {
			session.WorkloadStatus = convertWorkloadHealth(health)
		}
	}

	// Step 2: Trace ownership chain
	tracer := agent.NewReverseTracer(dynClient)
	traceResult, err := tracer.Trace(ctx, target.Kind, target.Name, target.Namespace)
	if err == nil && traceResult != nil {
		session.OwnershipChain = convertOwnershipChain(traceResult)
	}

	// Step 3: Get deployer/pipeline health
	if session.OwnershipChain != nil && len(session.OwnershipChain.GitOpsChain) > 0 {
		// Find the deployer in the chain (Kustomization, HelmRelease, Application)
		for _, link := range session.OwnershipChain.GitOpsChain {
			if isDeployerKind(link.Kind) {
				gvr, err := agent.KindToGVR(link.Kind)
				if err != nil {
					continue
				}
				obj, err := dynClient.Resource(gvr).Namespace(link.Namespace).Get(ctx, link.Name, metav1.GetOptions{})
				if err != nil {
					continue
				}

				var failure agent.FailureDetails
				switch link.Kind {
				case "Kustomization", "HelmRelease":
					failure = agent.ExtractFluxDeployerFailure(obj)
				case "Application":
					failure = agent.ExtractArgoFailure(obj)
				}

				session.DeployerStatus = &DeployerStatus{
					Kind:                  link.Kind,
					Name:                  link.Name,
					Namespace:             link.Namespace,
					Ready:                 failure.Ready,
					Suspended:             failure.Suspended,
					Stage:                 string(failure.Stage),
					Reason:                failure.Reason,
					Message:               failure.Message,
					LastAppliedRevision:   failure.LastAppliedRevision,
					LastAttemptedRevision: failure.LastAttemptedRevision,
				}
				break
			}
		}

		// Find the source in the chain (GitRepository, OCIRepository, etc.)
		for _, link := range session.OwnershipChain.GitOpsChain {
			if isSourceKind(link.Kind) {
				gvr, err := agent.KindToGVR(link.Kind)
				if err != nil {
					continue
				}
				obj, err := dynClient.Resource(gvr).Namespace(link.Namespace).Get(ctx, link.Name, metav1.GetOptions{})
				if err != nil {
					continue
				}

				failure := agent.ExtractFluxSourceFailure(obj)
				url, _, _ := unstructured.NestedString(obj.Object, "spec", "url")

				session.SourceStatus = &SourceStatus{
					Kind:      link.Kind,
					Name:      link.Name,
					Namespace: link.Namespace,
					Ready:     failure.Ready,
					Reason:    failure.Reason,
					Message:   failure.Message,
					URL:       url,
				}
				break
			}
		}
	}

	// Step 4: Generate root cause analysis
	session.RootCause = analyzeRootCause(session)
	session.CompletedAt = time.Now()

	return session, nil
}

// isWorkloadKind returns true if the kind is a workload type
func isWorkloadKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "ReplicaSet", "Pod":
		return true
	default:
		return false
	}
}

// isDeployerKind returns true if the kind is a GitOps deployer
func isDeployerKind(kind string) bool {
	switch kind {
	case "Kustomization", "HelmRelease", "Application":
		return true
	default:
		return false
	}
}

// isSourceKind returns true if the kind is a GitOps source
func isSourceKind(kind string) bool {
	switch kind {
	case "GitRepository", "OCIRepository", "HelmRepository", "Bucket":
		return true
	default:
		return false
	}
}

// convertWorkloadHealth converts agent.WorkloadHealthStatus to debug model type
func convertWorkloadHealth(health *agent.WorkloadHealthStatus) *WorkloadHealthStatus {
	result := &WorkloadHealthStatus{
		Kind:                health.Kind,
		Name:                health.Name,
		Namespace:           health.Namespace,
		Replicas:            health.Replicas,
		ReadyReplicas:       health.ReadyReplicas,
		UnavailableReplicas: health.UnavailableReplicas,
	}

	for _, issue := range health.PodIssues {
		result.PodIssues = append(result.PodIssues, PodIssue{
			PodName:         issue.PodName,
			Phase:           issue.Phase,
			ContainerStatus: issue.ContainerStatus,
			Reason:          issue.Reason,
			Message:         issue.Message,
			RestartCount:    issue.RestartCount,
		})
	}

	return result
}

// convertOwnershipChain converts agent.ReverseTraceResult to OwnershipChainResult
func convertOwnershipChain(trace *agent.ReverseTraceResult) *OwnershipChainResult {
	result := &OwnershipChainResult{
		Owner:        trace.Owner,
		OwnerDetails: trace.OwnerDetails,
	}

	for _, link := range trace.K8sChain {
		result.K8sChain = append(result.K8sChain, ChainLink{
			Kind:      link.Kind,
			Name:      link.Name,
			Namespace: link.Namespace,
			Ready:     link.Ready,
			Status:    link.Status,
		})
	}

	for _, link := range trace.GitOpsChain {
		result.GitOpsChain = append(result.GitOpsChain, ChainLink{
			Kind:      link.Kind,
			Name:      link.Name,
			Namespace: link.Namespace,
			Ready:     link.Ready,
			Status:    link.Status,
		})
	}

	return result
}

// analyzeRootCause synthesizes findings into a root cause analysis
func analyzeRootCause(session *DebugSession) *RootCauseAnalysis {
	analysis := &RootCauseAnalysis{
		Confidence: "medium",
	}

	// Check source issues first (upstream problems)
	if session.SourceStatus != nil && !session.SourceStatus.Ready {
		analysis.Stage = "source"
		analysis.Category = categorizeSourceFailure(session.SourceStatus.Reason)
		analysis.Summary = fmt.Sprintf("Source %s/%s is failing: %s",
			session.SourceStatus.Kind, session.SourceStatus.Name, session.SourceStatus.Reason)
		analysis.ProbableCauses = getSourceProbableCauses(session.SourceStatus.Reason)
		analysis.SuggestedFixes = getSourceSuggestedFixes(session.SourceStatus)
		analysis.Confidence = "high"
		return analysis
	}

	// Check deployer/pipeline issues
	if session.DeployerStatus != nil && !session.DeployerStatus.Ready && !session.DeployerStatus.Suspended {
		analysis.Stage = session.DeployerStatus.Stage
		analysis.Category = categorizeDeployerFailure(session.DeployerStatus.Stage, session.DeployerStatus.Reason)
		analysis.Summary = fmt.Sprintf("Pipeline %s/%s is failing at %s stage: %s",
			session.DeployerStatus.Kind, session.DeployerStatus.Name,
			session.DeployerStatus.Stage, session.DeployerStatus.Reason)
		analysis.ProbableCauses = getDeployerProbableCauses(session.DeployerStatus.Stage, session.DeployerStatus.Reason)
		analysis.SuggestedFixes = getDeployerSuggestedFixes(session.DeployerStatus)
		analysis.Confidence = "high"
		return analysis
	}

	// Check workload issues
	if session.WorkloadStatus != nil && !session.WorkloadStatus.IsHealthy() {
		analysis.Stage = "workload"
		if len(session.WorkloadStatus.PodIssues) > 0 {
			issue := session.WorkloadStatus.PodIssues[0]
			analysis.Category = categorizeWorkloadFailure(issue.ContainerStatus)
			analysis.Summary = fmt.Sprintf("Workload %s/%s has pod issues: %s",
				session.WorkloadStatus.Kind, session.WorkloadStatus.Name, issue.ContainerStatus)
			analysis.ProbableCauses = getWorkloadProbableCauses(issue.ContainerStatus)
			analysis.SuggestedFixes = getWorkloadSuggestedFixes(session.WorkloadStatus, issue)
			analysis.Confidence = "high"
		} else {
			analysis.Category = "replica_shortage"
			analysis.Summary = fmt.Sprintf("Workload %s/%s has %d/%d replicas ready",
				session.WorkloadStatus.Kind, session.WorkloadStatus.Name,
				session.WorkloadStatus.ReadyReplicas, session.WorkloadStatus.Replicas)
			analysis.ProbableCauses = []string{
				"Pods may be pending scheduling",
				"Node resources may be insufficient",
				"PodDisruptionBudget may be blocking",
			}
		}
		return analysis
	}

	// Everything looks healthy
	analysis.Stage = "healthy"
	analysis.Category = "healthy"
	analysis.Summary = "No issues detected"
	analysis.Confidence = "low"
	return analysis
}

func categorizeSourceFailure(reason string) string {
	switch reason {
	case "AuthenticationFailed":
		return "source_auth"
	case "NotFound", "URLInvalid":
		return "source_not_found"
	default:
		return "source_fetch"
	}
}

func categorizeDeployerFailure(stage, reason string) string {
	switch stage {
	case "build":
		return "build_error"
	case "apply":
		return "apply_error"
	case "sync":
		return "sync_error"
	default:
		return "pipeline_error"
	}
}

func categorizeWorkloadFailure(status string) string {
	switch status {
	case "CrashLoopBackOff":
		return "crash_loop"
	case "ImagePullBackOff", "ErrImagePull":
		return "image_pull"
	case "OOMKilled":
		return "oom_killed"
	case "Pending":
		return "pending"
	default:
		return "workload_error"
	}
}

func getSourceProbableCauses(reason string) []string {
	switch reason {
	case "AuthenticationFailed":
		return []string{
			"Secret does not exist or has invalid credentials",
			"Token may have expired",
			"Token may lack required permissions",
		}
	case "NotFound":
		return []string{
			"Repository or artifact URL may be incorrect",
			"Resource may have been deleted",
		}
	default:
		return []string{
			"Network connectivity issues",
			"Source server may be unavailable",
		}
	}
}

func getSourceSuggestedFixes(source *SourceStatus) []string {
	var fixes []string
	switch source.Reason {
	case "AuthenticationFailed":
		fixes = append(fixes, fmt.Sprintf("kubectl get secret -n %s -l 'source.toolkit.fluxcd.io/name=%s'",
			source.Namespace, source.Name))
		fixes = append(fixes, "Verify credentials are correct and not expired")
	default:
		fixes = append(fixes, fmt.Sprintf("kubectl describe %s %s -n %s",
			strings.ToLower(source.Kind), source.Name, source.Namespace))
	}
	return fixes
}

func getDeployerProbableCauses(stage, reason string) []string {
	switch stage {
	case "build":
		return []string{
			"Kustomize/Helm syntax error",
			"Missing referenced resources",
			"Invalid YAML in source",
		}
	case "apply":
		return []string{
			"RBAC permissions insufficient",
			"Resource validation failed",
			"Namespace does not exist",
		}
	default:
		return []string{
			"Check deployer status for details",
		}
	}
}

func getDeployerSuggestedFixes(deployer *DeployerStatus) []string {
	var fixes []string
	fixes = append(fixes, fmt.Sprintf("kubectl describe %s %s -n %s",
		strings.ToLower(deployer.Kind), deployer.Name, deployer.Namespace))
	if deployer.Kind == "Kustomization" {
		fixes = append(fixes, fmt.Sprintf("flux reconcile kustomization %s -n %s", deployer.Name, deployer.Namespace))
	} else if deployer.Kind == "HelmRelease" {
		fixes = append(fixes, fmt.Sprintf("flux reconcile helmrelease %s -n %s", deployer.Name, deployer.Namespace))
	}
	return fixes
}

func getWorkloadProbableCauses(status string) []string {
	switch status {
	case "CrashLoopBackOff":
		return []string{
			"Application error (check logs)",
			"Missing configuration (ConfigMap, Secret)",
			"Database/dependency unreachable",
		}
	case "ImagePullBackOff", "ErrImagePull":
		return []string{
			"Image does not exist",
			"Registry credentials missing or invalid",
			"Network cannot reach registry",
		}
	case "OOMKilled":
		return []string{
			"Container memory limit too low",
			"Application has memory leak",
		}
	default:
		return []string{
			"Check pod events for details",
		}
	}
}

func getWorkloadSuggestedFixes(workload *WorkloadHealthStatus, issue PodIssue) []string {
	var fixes []string
	fixes = append(fixes, fmt.Sprintf("kubectl logs %s -n %s --previous",
		issue.PodName, workload.Namespace))
	fixes = append(fixes, fmt.Sprintf("kubectl describe pod %s -n %s",
		issue.PodName, workload.Namespace))
	return fixes
}

// runDebugAnalysis runs the full debug analysis (used by non-interactive mode)
func runDebugAnalysis(ctx context.Context, cfg *rest.Config, kind, name, namespace string) (*DebugSession, error) {
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	session := &DebugSession{
		StartedAt: time.Now(),
		Target: ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
	}

	return runDebugAnalysisFromModel(ctx, dynClient, clientset, session)
}
