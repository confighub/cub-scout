// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type gitOpsStatusTUIModel struct {
	viewport viewport.Model
	content  string
}

func newGitOpsStatusTUIModel(summary GitOpsSummary) *gitOpsStatusTUIModel {
	return newGitOpsMarkdownTUIModel(renderGitOpsStatusMarkdown(summary))
}

// newGitOpsMarkdownTUIModel shows one already-collected Markdown snapshot.
func newGitOpsMarkdownTUIModel(markdown string) *gitOpsStatusTUIModel {
	content := safeGitOpsTUIContent(markdown)
	vp := viewport.New(78, 20)
	vp.SetContent(ansi.Hardwrap(content, vp.Width, true))
	return &gitOpsStatusTUIModel{viewport: vp, content: content}
}

func safeGitOpsTUIContent(content string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, content)
}

func (m *gitOpsStatusTUIModel) Init() tea.Cmd { return nil }

func (m *gitOpsStatusTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = max(1, msg.Width-2)
		m.viewport.Height = max(1, msg.Height-3)
		m.viewport.SetContent(ansi.Hardwrap(m.content, m.viewport.Width, true))
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *gitOpsStatusTUIModel) View() string {
	return m.viewport.View() + "\n\n" + ansi.Truncate("q / Esc close | arrows or PgUp/PgDn scroll", m.viewport.Width, "")
}

func runGitOpsStatusTUI(ctx context.Context, summary GitOpsSummary) error {
	_, err := tea.NewProgram(newGitOpsStatusTUIModel(summary), tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}

func runGitOpsMarkdownTUI(ctx context.Context, markdown string) error {
	_, err := tea.NewProgram(newGitOpsMarkdownTUIModel(markdown), tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}
