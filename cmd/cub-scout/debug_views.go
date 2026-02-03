// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles for debug wizard
var (
	debugTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			MarginBottom(1)

	debugSubtitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("241"))

	debugSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82")).
				Bold(true)

	debugNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	debugDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	debugSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("82"))

	debugWarningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214"))

	debugErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	debugBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("241")).
			Padding(1, 2)

	debugTipStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Italic(true)
)

// renderSelectModeView renders the entry mode selection (step 1)
func (m DebugModel) renderSelectModeView() string {
	var b strings.Builder

	// Title
	b.WriteString(debugTitleStyle.Render("GUIDED DEBUG"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n\n")

	b.WriteString("What kind of problem are you debugging?\n\n")

	// Entry mode options
	for i, mode := range m.entryModes {
		cursor := "  "
		style := debugNormalStyle
		if i == m.entryModeCursor {
			cursor = "> "
			style = debugSelectedStyle
		}

		b.WriteString(cursor)
		b.WriteString(style.Render(fmt.Sprintf("[%d] %s", i+1, mode.String())))
		b.WriteString("\n")
		b.WriteString("      ")
		b.WriteString(debugDimStyle.Render(mode.Description()))
		b.WriteString("\n\n")
	}

	// Footer
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")
	b.WriteString(debugDimStyle.Render("↑/↓ Navigate  Enter Select  q Quit  ? Help"))

	return b.String()
}

// renderPickResourceView renders the resource picker (step 2)
func (m DebugModel) renderPickResourceView() string {
	var b strings.Builder

	// Title
	title := "SELECT RESOURCE"
	switch m.session.EntryMode {
	case EntryModeBrokenWorkload:
		title = "SELECT UNHEALTHY WORKLOAD"
	case EntryModeFailingPipeline:
		title = "SELECT FAILING PIPELINE"
	case EntryModeSyncIssue:
		title = "SELECT SOURCE WITH SYNC ISSUE"
	}

	b.WriteString(debugTitleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n\n")

	if len(m.resources) == 0 {
		b.WriteString(debugDimStyle.Render("No resources found matching the criteria."))
		b.WriteString("\n\n")
		b.WriteString(debugTipStyle.Render("Tip: Try a different debug mode or check your cluster connection."))
	} else {
		// Resource list
		maxVisible := 10
		start := 0
		if m.resourceCursor >= maxVisible {
			start = m.resourceCursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(m.resources) {
			end = len(m.resources)
		}

		for i := start; i < end; i++ {
			res := m.resources[i]
			cursor := "  "
			style := debugNormalStyle
			if i == m.resourceCursor {
				cursor = "> "
				style = debugSelectedStyle
			}

			// Format: kind/name [status] ns: namespace
			line := fmt.Sprintf("%s/%s", res.Kind, res.Name)
			if res.Status != "" {
				line += fmt.Sprintf(" [%s]", res.Status)
			}
			if res.Namespace != "" {
				line += fmt.Sprintf(" ns: %s", res.Namespace)
			}

			b.WriteString(cursor)
			b.WriteString(style.Render(line))
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(m.resources) > maxVisible {
			b.WriteString("\n")
			b.WriteString(debugDimStyle.Render(fmt.Sprintf("Showing %d-%d of %d", start+1, end, len(m.resources))))
		}
	}

	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")
	b.WriteString(debugDimStyle.Render("↑/↓ Navigate  Enter Select  Esc Back  q Quit"))

	return b.String()
}

// renderLoadingView renders the loading spinner
func (m DebugModel) renderLoadingView() string {
	var b strings.Builder

	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(m.spinner.View())
	b.WriteString(" ")
	b.WriteString(m.loadingMsg)
	b.WriteString("\n\n")

	return b.String()
}

// renderHelpView renders the help overlay
func (m DebugModel) renderHelpView() string {
	var b strings.Builder

	b.WriteString(debugTitleStyle.Render("KEYBOARD SHORTCUTS"))
	b.WriteString("\n\n")

	shortcuts := []struct {
		key  string
		desc string
	}{
		{"↑/↓ or j/k", "Navigate up/down"},
		{"Enter", "Select / Continue"},
		{"Esc", "Go back"},
		{"?", "Toggle this help"},
		{"q", "Quit debug wizard"},
		{"c", "Copy summary to clipboard (in summary view)"},
		{"e", "Export analysis (in summary view)"},
	}

	for _, s := range shortcuts {
		b.WriteString(fmt.Sprintf("  %s\n", debugSelectedStyle.Render(s.key)))
		b.WriteString(fmt.Sprintf("      %s\n\n", debugDimStyle.Render(s.desc)))
	}

	b.WriteString("\n")
	b.WriteString(debugDimStyle.Render("Press any key to close"))

	return debugBoxStyle.Render(b.String())
}

// renderAnalysisView renders intermediate analysis steps
func (m DebugModel) renderAnalysisView() string {
	var b strings.Builder

	stepNames := []string{
		"Select Mode",
		"Pick Resource",
		"Workload Status",
		"Container Logs",
		"Ownership",
		"Pipeline Health",
		"Source Health",
		"Root Cause",
		"Summary",
	}

	// Progress indicator
	b.WriteString(debugDimStyle.Render(fmt.Sprintf("Step %d of %d: %s", m.step+1, len(stepNames), stepNames[m.step])))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n\n")

	// Step content based on session data
	switch m.step {
	case DebugStepWorkloadStatus:
		b.WriteString(m.renderWorkloadStatusStep())
	case DebugStepContainerLogs:
		b.WriteString(m.renderContainerLogsStep())
	case DebugStepEventTimeline:
		b.WriteString(m.renderEventTimelineStep())
	case DebugStepOwnership:
		b.WriteString(m.renderOwnershipStep())
	case DebugStepPipelineHealth:
		b.WriteString(m.renderPipelineStep())
	case DebugStepSourceHealth:
		b.WriteString(m.renderSourceStep())
	case DebugStepRootCause:
		b.WriteString(m.renderRootCauseStep())
	}

	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")

	// Step-specific footer
	if m.step == DebugStepWorkloadStatus && m.session.WorkloadStatus != nil && len(m.session.WorkloadStatus.PodIssues) > 0 {
		b.WriteString(debugDimStyle.Render("l View Logs  e View Events  Enter Continue  ? Help  q Quit"))
	} else if m.step == DebugStepContainerLogs {
		b.WriteString(debugDimStyle.Render("↑/↓ Scroll  p Toggle Previous  Esc Back  q Quit"))
	} else if m.step == DebugStepEventTimeline {
		filterMode := "all"
		if !m.eventShowAll {
			filterMode = "warnings only"
		}
		b.WriteString(debugDimStyle.Render(fmt.Sprintf("↑/↓ Scroll  a Toggle Filter (%s)  Esc Back  q Quit", filterMode)))
	} else {
		b.WriteString(debugDimStyle.Render("Enter Continue  ? Toggle Education  q Quit"))
	}

	return b.String()
}

// renderWorkloadStatusStep renders the workload status view
func (m DebugModel) renderWorkloadStatusStep() string {
	var b strings.Builder

	if m.session.WorkloadStatus == nil {
		return debugDimStyle.Render("No workload status available")
	}

	ws := m.session.WorkloadStatus

	// Header
	b.WriteString(debugTitleStyle.Render(fmt.Sprintf("WORKLOAD STATUS: %s/%s in %s",
		ws.Kind, ws.Name, ws.Namespace)))
	b.WriteString("\n\n")

	// Replicas
	replicaStyle := debugSuccessStyle
	if ws.ReadyReplicas < ws.Replicas {
		replicaStyle = debugWarningStyle
	}
	b.WriteString(fmt.Sprintf("Replicas: %s\n\n",
		replicaStyle.Render(fmt.Sprintf("%d/%d ready", ws.ReadyReplicas, ws.Replicas))))

	// Pod issues
	if len(ws.PodIssues) > 0 {
		b.WriteString("Pod Issues:\n")
		for _, issue := range ws.PodIssues {
			icon := debugErrorStyle.Render("✗")
			b.WriteString(fmt.Sprintf("  %s %s   %s (restarted %d times)\n",
				icon, issue.PodName, debugWarningStyle.Render(issue.ContainerStatus), issue.RestartCount))
			if issue.Message != "" {
				b.WriteString(fmt.Sprintf("    %s\n", debugDimStyle.Render(issue.Message)))
			}
		}
		b.WriteString("\n")
	}

	// Education tip
	if len(ws.PodIssues) > 0 {
		tip := getEducationTip(ws.PodIssues[0].ContainerStatus)
		if tip != "" {
			b.WriteString(debugBoxStyle.Render("💡 " + tip))
		}
	}

	return b.String()
}

// renderContainerLogsStep renders the container logs view
func (m DebugModel) renderContainerLogsStep() string {
	var b strings.Builder

	if m.session.ContainerLogs == nil || len(m.session.ContainerLogs) == 0 {
		return debugDimStyle.Render("No container logs available")
	}

	// Get current log result
	logIdx := m.logPodCursor
	if logIdx >= len(m.session.ContainerLogs) {
		logIdx = 0
	}
	logResult := m.session.ContainerLogs[logIdx]

	// Header
	title := fmt.Sprintf("CONTAINER LOGS: %s", logResult.PodName)
	if logResult.ContainerName != "" {
		title += "/" + logResult.ContainerName
	}
	if logResult.Previous {
		title += " (previous)"
	}
	b.WriteString(debugTitleStyle.Render(title))
	b.WriteString("\n\n")

	// Error state
	if logResult.Error != "" {
		b.WriteString(debugErrorStyle.Render("Error: " + logResult.Error))
		b.WriteString("\n\n")
		return b.String()
	}

	// Detected patterns summary
	if len(logResult.Patterns) > 0 {
		b.WriteString(debugWarningStyle.Render("Detected Issues:"))
		b.WriteString("\n")
		for _, pattern := range logResult.Patterns {
			b.WriteString(fmt.Sprintf("  • Line %d: %s\n", pattern.Line, pattern.Match))
			b.WriteString(fmt.Sprintf("    %s\n", debugDimStyle.Render(pattern.Explanation)))
		}
		b.WriteString("\n")
	}

	// Log lines
	b.WriteString("Last 20 lines:\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")

	// Calculate visible range based on scroll position
	maxVisible := 15
	start := m.logScrollPos
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > len(logResult.Lines) {
		end = len(logResult.Lines)
	}
	if start > len(logResult.Lines)-maxVisible {
		start = len(logResult.Lines) - maxVisible
		if start < 0 {
			start = 0
		}
	}

	// Render log lines
	for i := start; i < end; i++ {
		line := logResult.Lines[i]
		lineNum := i + 1

		// Highlight lines with detected patterns
		highlighted := false
		for _, pattern := range logResult.Patterns {
			if pattern.Line == lineNum {
				highlighted = true
				break
			}
		}

		// Truncate long lines
		if len(line) > 80 {
			line = line[:77] + "..."
		}

		if highlighted {
			b.WriteString(fmt.Sprintf("%4d │ %s\n", lineNum, debugWarningStyle.Render(line)))
		} else {
			b.WriteString(fmt.Sprintf("%4d │ %s\n", lineNum, debugDimStyle.Render(line)))
		}
	}

	// Scroll indicator
	if len(logResult.Lines) > maxVisible {
		b.WriteString(strings.Repeat("─", 60))
		b.WriteString("\n")
		b.WriteString(debugDimStyle.Render(fmt.Sprintf("Showing lines %d-%d of %d", start+1, end, len(logResult.Lines))))
	}

	// Pod selector if multiple pods
	if len(m.session.ContainerLogs) > 1 {
		b.WriteString("\n\n")
		b.WriteString("Pods: ")
		for i, log := range m.session.ContainerLogs {
			if i == logIdx {
				b.WriteString(debugSelectedStyle.Render(fmt.Sprintf("[%s] ", log.PodName)))
			} else {
				b.WriteString(debugDimStyle.Render(fmt.Sprintf("%s ", log.PodName)))
			}
		}
		b.WriteString("\n")
		b.WriteString(debugDimStyle.Render("←/→ Switch Pod"))
	}

	return b.String()
}

// renderOwnershipStep renders the ownership view
func (m DebugModel) renderOwnershipStep() string {
	var b strings.Builder

	if m.session.OwnershipChain == nil {
		return debugDimStyle.Render("No ownership information available")
	}

	oc := m.session.OwnershipChain

	b.WriteString(debugTitleStyle.Render("OWNERSHIP"))
	b.WriteString("\n\n")

	// Owner summary
	ownerStyle := debugSuccessStyle
	if oc.Owner == "native" {
		ownerStyle = debugWarningStyle
	}
	b.WriteString(fmt.Sprintf("Owner: %s\n\n", ownerStyle.Render(oc.Owner)))

	// K8s chain
	if len(oc.K8sChain) > 0 {
		b.WriteString("Kubernetes Chain:\n")
		for i, link := range oc.K8sChain {
			prefix := "  ├─"
			if i == len(oc.K8sChain)-1 {
				prefix = "  └─"
			}
			icon := debugSuccessStyle.Render("✓")
			if !link.Ready {
				icon = debugErrorStyle.Render("✗")
			}
			b.WriteString(fmt.Sprintf("%s %s %s/%s\n", prefix, icon, link.Kind, link.Name))
		}
		b.WriteString("\n")
	}

	// GitOps chain
	if len(oc.GitOpsChain) > 0 {
		b.WriteString("GitOps Chain:\n")
		for i, link := range oc.GitOpsChain {
			prefix := "  ├─"
			if i == len(oc.GitOpsChain)-1 {
				prefix = "  └─"
			}
			icon := debugSuccessStyle.Render("✓")
			if !link.Ready {
				icon = debugErrorStyle.Render("✗")
			}
			b.WriteString(fmt.Sprintf("%s %s %s/%s\n", prefix, icon, link.Kind, link.Name))
		}
	}

	return b.String()
}

// renderPipelineStep renders the pipeline health view
func (m DebugModel) renderPipelineStep() string {
	var b strings.Builder

	if m.session.DeployerStatus == nil {
		return debugDimStyle.Render("No pipeline information available")
	}

	ds := m.session.DeployerStatus

	b.WriteString(debugTitleStyle.Render("PIPELINE HEALTH"))
	b.WriteString("\n\n")

	icon := debugSuccessStyle.Render("✓")
	if !ds.Ready {
		icon = debugErrorStyle.Render("✗")
	}

	b.WriteString(fmt.Sprintf("%s %s/%s in %s\n", icon, ds.Kind, ds.Name, ds.Namespace))
	b.WriteString(fmt.Sprintf("  Ready: %v\n", ds.Ready))

	if ds.Stage != "" && ds.Stage != "healthy" {
		b.WriteString(fmt.Sprintf("  Stage: %s\n", debugWarningStyle.Render(ds.Stage)))
	}
	if ds.Reason != "" {
		b.WriteString(fmt.Sprintf("  Reason: %s\n", ds.Reason))
	}
	if ds.Message != "" {
		b.WriteString(fmt.Sprintf("  Message: %s\n", debugDimStyle.Render(ds.Message)))
	}

	// Education tip for stage
	if ds.Stage != "" && ds.Stage != "healthy" {
		tip := getEducationTip(ds.Stage)
		if tip != "" {
			b.WriteString("\n")
			b.WriteString(debugBoxStyle.Render("💡 " + tip))
		}
	}

	return b.String()
}

// renderSourceStep renders the source health view
func (m DebugModel) renderSourceStep() string {
	var b strings.Builder

	if m.session.SourceStatus == nil {
		return debugDimStyle.Render("No source information available")
	}

	ss := m.session.SourceStatus

	b.WriteString(debugTitleStyle.Render("SOURCE HEALTH"))
	b.WriteString("\n\n")

	icon := debugSuccessStyle.Render("✓")
	if !ss.Ready {
		icon = debugErrorStyle.Render("✗")
	}

	b.WriteString(fmt.Sprintf("%s %s/%s in %s\n", icon, ss.Kind, ss.Name, ss.Namespace))
	if ss.URL != "" {
		b.WriteString(fmt.Sprintf("  URL: %s\n", ss.URL))
	}
	if !ss.Ready {
		if ss.Reason != "" {
			b.WriteString(fmt.Sprintf("  Reason: %s\n", debugWarningStyle.Render(ss.Reason)))
		}
		if ss.Message != "" {
			b.WriteString(fmt.Sprintf("  Message: %s\n", debugDimStyle.Render(ss.Message)))
		}
	}

	return b.String()
}

// renderRootCauseStep renders the root cause analysis
func (m DebugModel) renderRootCauseStep() string {
	var b strings.Builder

	if m.session.RootCause == nil {
		return debugDimStyle.Render("Analysis in progress...")
	}

	rc := m.session.RootCause

	b.WriteString(debugTitleStyle.Render("ROOT CAUSE ANALYSIS"))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Category: %s\n", debugWarningStyle.Render(rc.Category)))
	b.WriteString(fmt.Sprintf("Stage: %s\n", rc.Stage))
	b.WriteString(fmt.Sprintf("Confidence: %s\n\n", rc.Confidence))

	b.WriteString(fmt.Sprintf("Summary: %s\n\n", rc.Summary))

	if len(rc.ProbableCauses) > 0 {
		b.WriteString("Probable Causes:\n")
		for _, cause := range rc.ProbableCauses {
			b.WriteString(fmt.Sprintf("  • %s\n", cause))
		}
		b.WriteString("\n")
	}

	if len(rc.SuggestedFixes) > 0 {
		b.WriteString("Suggested Fixes:\n")
		for _, fix := range rc.SuggestedFixes {
			b.WriteString(fmt.Sprintf("  %s\n", fix))
		}
	}

	return b.String()
}

// renderSummaryView renders the final summary
func (m DebugModel) renderSummaryView() string {
	var b strings.Builder

	b.WriteString(debugTitleStyle.Render("DEBUG SUMMARY"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("═", 60))
	b.WriteString("\n\n")

	// Target
	b.WriteString(fmt.Sprintf("Target: %s\n\n", m.session.Target.String()))

	// Quick status
	if m.session.WorkloadStatus != nil {
		ws := m.session.WorkloadStatus
		status := debugSuccessStyle.Render("Healthy")
		if !ws.IsHealthy() {
			status = debugErrorStyle.Render(fmt.Sprintf("%d/%d ready", ws.ReadyReplicas, ws.Replicas))
		}
		b.WriteString(fmt.Sprintf("Workload: %s\n", status))
	}

	if m.session.OwnershipChain != nil {
		b.WriteString(fmt.Sprintf("Owner: %s\n", m.session.OwnershipChain.Owner))
	}

	if m.session.DeployerStatus != nil {
		ds := m.session.DeployerStatus
		status := debugSuccessStyle.Render("Healthy")
		if !ds.Ready {
			status = debugErrorStyle.Render(ds.Stage)
		}
		b.WriteString(fmt.Sprintf("Pipeline: %s\n", status))
	}

	if m.session.SourceStatus != nil {
		ss := m.session.SourceStatus
		status := debugSuccessStyle.Render("Healthy")
		if !ss.Ready {
			status = debugErrorStyle.Render(ss.Reason)
		}
		b.WriteString(fmt.Sprintf("Source: %s\n", status))
	}

	b.WriteString("\n")

	// Root cause
	if m.session.RootCause != nil {
		b.WriteString(strings.Repeat("─", 60))
		b.WriteString("\n\n")
		b.WriteString(debugWarningStyle.Render("Root Cause: "))
		b.WriteString(m.session.RootCause.Summary)
		b.WriteString("\n\n")

		if len(m.session.RootCause.SuggestedFixes) > 0 {
			b.WriteString("Next Steps:\n")
			for i, fix := range m.session.RootCause.SuggestedFixes {
				if i >= 3 {
					b.WriteString(fmt.Sprintf("  ... and %d more\n", len(m.session.RootCause.SuggestedFixes)-3))
					break
				}
				b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, fix))
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")
	b.WriteString(debugDimStyle.Render("c Copy  e Export  Enter/q Done"))

	return b.String()
}

// getEducationTip returns a short education tip for a given key
func getEducationTip(key string) string {
	tips := map[string]string{
		"CrashLoopBackOff": "Container keeps crashing. Check logs: kubectl logs <pod> --previous",
		"ImagePullBackOff": "Cannot pull container image. Check image name and registry credentials.",
		"Pending":          "Pod waiting for resources or scheduling. Check events: kubectl describe pod",
		"OOMKilled":        "Container ran out of memory. Increase memory limits or optimize the app.",
		"source":           "Source fetch failed. Check URL, credentials, and network connectivity.",
		"build":            "Build/render failed. Check kustomize/helm syntax and dependencies.",
		"apply":            "Apply to cluster failed. Check RBAC permissions and resource conflicts.",
		"sync":             "Sync failed. Check ArgoCD application status and destination cluster.",
	}

	if tip, ok := tips[key]; ok {
		return tip
	}
	return ""
}
