// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const threeWayTUITimeout = 30 * time.Second

type threeWayTUIResultMsg struct {
	requestID    uint64
	scope        threeWayScope
	contextLabel string
	report       threeWayReport
	err          error
}

// The pane owns scope and invocation options; it never changes command globals.
// Both CLI --tui and the selected-workload picker use it.
type threeWayTUIModel struct {
	viewport   viewport.Model
	session    *traceSession
	parent     context.Context
	scope      threeWayScope
	options    threeWayOptions
	requestID  uint64
	cancel     context.CancelFunc
	loading    bool
	output     string
	err        error
	editing    bool
	input      string
	closed     bool
	standalone bool
}

func newThreeWayTUIModel(ctx context.Context, session *traceSession, scope threeWayScope, options threeWayOptions) threeWayTUIModel {
	if ctx == nil {
		ctx = context.Background()
	}
	return threeWayTUIModel{parent: ctx, session: session, scope: scope, options: options, viewport: viewport.New(78, 20)}
}

func (m threeWayTUIModel) Init() tea.Cmd { return nil }

func (m *threeWayTUIModel) start() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithTimeout(m.parent, threeWayTUITimeout)
	m.cancel = cancel
	m.requestID++
	m.loading, m.err = true, nil
	m.clearEvidence()
	id, scope, session, options := m.requestID, m.scope, m.session, m.options
	return func() tea.Msg {
		defer cancel()
		if err := ctx.Err(); err != nil {
			return threeWayTUIResultMsg{requestID: id, scope: scope, contextLabel: session.contextLabel(), err: err}
		}
		// A local rendered operand requires no ConfigHub auth check. Reject its
		// incompatible View scope before any external read, including auth.
		if scope.ScopeType == threeWayScopeView && options.hasLocalDry() {
			return threeWayTUIResultMsg{requestID: id, scope: scope, contextLabel: session.contextLabel(), err: fmt.Errorf("--dry-from and --view are mutually exclusive (--view requires connected mode)")}
		}
		// Refresh once at each visible observation boundary. Failed auth leaves
		// resource/namespace/cluster observation available as standalone evidence.
		if !options.hasLocalDry() {
			_ = refreshConfigHubReads()
		}
		report, err := collectThreeWayWithSession(ctx, session, scope, options)
		return threeWayTUIResultMsg{requestID: id, scope: scope, contextLabel: session.contextLabel(), report: report, err: err}
	}
}

func (m *threeWayTUIModel) clearEvidence() {
	m.output = ""
	m.viewport.SetContent("")
	m.viewport.GotoTop()
}

func (m threeWayTUIModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = max(1, msg.Width-2)
		m.viewport.Height = max(1, msg.Height-7)
		m.viewport.SetContent(ansi.Hardwrap(safeGitOpsTUIContent(m.output), m.viewport.Width, true))
	case threeWayTUIResultMsg:
		if m.closed || !m.loading || msg.requestID != m.requestID || msg.scope != m.scope || msg.contextLabel != m.session.contextLabel() {
			return m, nil
		}
		m.loading = false
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
		}
		m.err = msg.err
		if msg.err == nil {
			m.output = renderThreeWayASCII(msg.report)
			m.viewport.SetContent(ansi.Hardwrap(safeGitOpsTUIContent(m.output), m.viewport.Width, true))
			m.viewport.GotoTop()
		} else {
			m.clearEvidence()
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			if m.cancel != nil {
				m.cancel()
				m.cancel = nil
			}
			m.requestID++
			if m.editing {
				m.editing = false
				return m, nil
			}
			m.loading = false
			m.closed = true
			if m.standalone {
				return m, tea.Quit
			}
		case "s":
			if !m.editing {
				if m.cancel != nil {
					m.cancel()
					m.cancel = nil
				}
				m.requestID++
				m.loading = false
				m.editing = true
				m.input = m.scope.String()
				return m, nil
			}
			m.input += "s"
		case "backspace":
			if m.editing {
				r := []rune(m.input)
				if len(r) > 0 {
					m.input = string(r[:len(r)-1])
				}
			}
		case "enter":
			if m.editing {
				raw := strings.TrimSpace(m.input)
				var scope threeWayScope
				var err error
				if strings.HasPrefix(raw, "view/") {
					scope = threeWayScope{ScopeType: threeWayScopeView, ScopeValue: strings.TrimPrefix(raw, "view/")}
				} else {
					scope, err = parseThreeWayScope(raw)
				}
				if err != nil {
					m.err = err
					return m, nil
				}
				m.scope = scope
				m.editing = false
			}
			return m, m.start()
		case "r":
			if !m.editing {
				return m, m.start()
			}
			m.input += "r"
		default:
			if m.editing && msg.Type == tea.KeyRunes {
				m.input += string(msg.Runes)
			} else if !m.editing {
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
		}
	}
	return m, nil
}

func (m threeWayTUIModel) View() string {
	var b strings.Builder
	b.WriteString("Three-way comparison · " + safeGitOpsTUIContent(m.scope.String()) + "\n")
	b.WriteString("Kubernetes context label: " + safeGitOpsTUIContent(m.session.contextLabel()) + "\n\n")
	if m.editing {
		b.WriteString("Scope (kind/name, namespace/name, cluster, view/UUID):\n" + safeGitOpsTUIContent(m.input) + "▏\n")
	} else if m.loading {
		b.WriteString("Collecting scoped comparison...\n")
	} else {
		b.WriteString(m.viewport.View())
	}
	if m.err != nil {
		b.WriteString("Error: " + safeGitOpsTUIContent(m.err.Error()) + "\n")
	}
	b.WriteString("\nEnter/r refresh · s edit scope · arrows/PgUp/PgDn scroll · Esc cancel/return to selected workload\n")
	return b.String()
}

func runThreeWayTUI(ctx context.Context, session *traceSession, scope threeWayScope, options threeWayOptions) error {
	model := newThreeWayTUIModel(ctx, session, scope, options)
	model.standalone = true
	command := model.start()
	// Start is scheduled on the initialized model, so request guards precede work.
	program := tea.NewProgram(threeWayTUIInitialModel{threeWayTUIModel: model, initial: command}, tea.WithContext(ctx), tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return fmt.Errorf("comparison TUI: %w", err)
	}
	if result, ok := final.(threeWayTUIModel); ok && result.cancel != nil {
		result.cancel()
	}
	return nil
}

type threeWayTUIInitialModel struct {
	threeWayTUIModel
	initial tea.Cmd
}

func (m threeWayTUIInitialModel) Init() tea.Cmd { return m.initial }
