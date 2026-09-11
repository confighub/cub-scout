// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/confighub/cub-scout/pkg/agent"
)

type releaseCheckMessage struct {
	request uint64
	report  agent.ReleaseCheckReport
	err     error
}

type releaseCheckModel struct {
	ctx      context.Context
	options  releaseCheckOptions
	observe  func(context.Context, releaseCheckOptions) (agent.ReleaseCheckReport, error)
	cancel   context.CancelFunc
	request  uint64
	closed   bool
	viewport viewport.Model
	content  string
}

func newReleaseCheckModel(ctx context.Context, o releaseCheckOptions) *releaseCheckModel {
	return &releaseCheckModel{ctx: ctx, options: o, observe: observeReleaseCheck, viewport: viewport.New(78, 20)}
}

func (m *releaseCheckModel) Init() tea.Cmd { return m.refresh() }
func (m *releaseCheckModel) refresh() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	m.request++
	request := m.request
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.content = "Checking exact configuration release..."
	m.viewport.SetContent(ansi.Hardwrap(m.content, m.viewport.Width, true))
	m.viewport.GotoTop()
	observe, options := m.observe, m.options
	return func() tea.Msg {
		r, err := observe(ctx, options)
		return releaseCheckMessage{request: request, report: r, err: err}
	}
}

func (m *releaseCheckModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = max(1, msg.Width-2)
		m.viewport.Height = max(1, msg.Height-3)
		m.viewport.SetContent(ansi.Hardwrap(m.content, m.viewport.Width, true))
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.closed = true
			m.request++
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "r":
			return m, m.refresh()
		}
	case releaseCheckMessage:
		if m.closed || msg.request != m.request {
			return m, nil
		}
		if msg.err != nil {
			m.content = fmt.Sprintf("Release check unavailable: %q", msg.err.Error())
		} else {
			m.content = renderReleaseCheck(msg.report, "ascii")
		}
		m.viewport.SetContent(ansi.Hardwrap(m.content, m.viewport.Width, true))
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *releaseCheckModel) View() string {
	return m.viewport.View() + "\n\n" + ansi.Truncate("r refresh | Esc close | arrows scroll", m.viewport.Width, "")
}

func runReleaseCheckTUI(ctx context.Context, o releaseCheckOptions) error {
	m := newReleaseCheckModel(ctx, o)
	defer func() {
		if m.cancel != nil {
			m.cancel()
		}
	}()
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}
