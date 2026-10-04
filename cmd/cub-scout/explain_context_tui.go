// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// This viewer renders one completed observation; no async reads can overwrite
// a different selection or repaint a closed pane.
type enrichedExplainViewer struct{ recordedExplainViewer }

func newEnrichedExplainViewer(summary ExplainSummary) enrichedExplainViewer {
	content := safeGitOpsTUIContent(renderExplainMarkdown(summary, PresentationMode(""), false, HintContext{}))
	vp := viewport.New(80, 20)
	vp.SetContent(wrapRecordedExplainText(content, 76))
	return enrichedExplainViewer{recordedExplainViewer{content: content, viewport: vp}}
}

func (m enrichedExplainViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.recordedExplainViewer.Update(msg)
	m.recordedExplainViewer = model.(recordedExplainViewer)
	return m, cmd
}

func (m enrichedExplainViewer) View() string {
	return "Explain snapshot · q to quit · arrows/page keys to scroll\n" + m.viewport.View()
}

var runEnrichedExplainTUI = func(summary ExplainSummary) error {
	_, err := tea.NewProgram(newEnrichedExplainViewer(summary)).Run()
	return err
}
