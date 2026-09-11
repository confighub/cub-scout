// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/confighub/cub-scout/pkg/agent"
)

type boundedExplainPanel struct {
	items    []agent.BoundedResourceRef
	cursor   int
	context  string
	viewing  bool
	request  uint64
	cancel   context.CancelFunc
	viewport viewport.Model
	content  string
	height   int
	observe  func(context.Context, agent.BoundedResourceRef, string, bool) (ExplainSummary, error)
}

type boundedExplainMsg struct {
	panel   *boundedExplainPanel
	request uint64
	summary ExplainSummary
	err     error
}

func (m *LocalClusterModel) openBoundedExplain() {
	if m.boundedSession == nil {
		m.boundedSession = &boundedExplainSession{}
	}
	panel := &boundedExplainPanel{context: m.contextName, observe: m.boundedSession.observe}
	seen := make(map[agent.BoundedResourceRef]bool)
	for _, entry := range m.getFilteredEntries() {
		if m.viewOpts.Namespace != "" && entry.Namespace != m.viewOpts.Namespace {
			continue
		}
		if m.viewOpts.Kind != "" && !strings.EqualFold(entry.Kind, m.viewOpts.Kind) {
			continue
		}
		if m.viewOpts.Owner != "" && !strings.EqualFold(entry.Owner, m.viewOpts.Owner) {
			continue
		}
		ref := agent.BoundedResourceRef{APIVersion: entry.APIVersion, Kind: entry.Kind, Namespace: entry.Namespace, Name: entry.Name}
		if ref.Validate() == nil && !seen[ref] {
			panel.items = append(panel.items, ref)
			seen[ref] = true
		}
	}
	sort.Slice(panel.items, func(i, j int) bool { return boundedRefLabel(panel.items[i]) < boundedRefLabel(panel.items[j]) })
	panel.resize(m.width, m.height)
	m.boundedPanel = panel
}

func boundedRefLabel(ref agent.BoundedResourceRef) string {
	return fmt.Sprintf("%s %s %s/%s", ref.APIVersion, ref.Kind, ref.Namespace, ref.Name)
}

func (p *boundedExplainPanel) resize(width, height int) {
	p.height = max(1, height)
	p.viewport.Width = max(1, width-2)
	p.viewport.Height = max(1, height-4)
	p.viewport.SetContent(ansi.Hardwrap(p.content, p.viewport.Width, true))
}

func (p *boundedExplainPanel) setContent(content string) {
	p.content = content
	p.viewport.SetContent(ansi.Hardwrap(content, p.viewport.Width, true))
	p.viewport.GotoTop()
}

func (p *boundedExplainPanel) stop() {
	p.request++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
}

func (p *boundedExplainPanel) read(refresh bool) tea.Cmd {
	p.stop()
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return nil
	}
	p.viewing = true
	p.setContent("Reading selected resource...")
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	request, ref, kubeContext, observe := p.request, p.items[p.cursor], p.context, p.observe
	return func() tea.Msg {
		summary, err := observe(ctx, ref, kubeContext, refresh)
		cancel()
		return boundedExplainMsg{panel: p, request: request, summary: summary, err: err}
	}
}

func (m LocalClusterModel) boundedExplainKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := m.boundedPanel
	switch msg.String() {
	case "ctrl+c":
		p.stop()
		return m, tea.Quit
	case "esc", "q":
		p.stop()
		if p.viewing {
			p.viewing = false
		} else {
			m.boundedPanel = nil
		}
		return m, nil
	case "enter":
		if !p.viewing {
			return m, p.read(false)
		}
	case "r":
		if p.viewing {
			return m, p.read(true)
		}
	case "up", "k":
		if !p.viewing {
			p.cursor = max(0, p.cursor-1)
			return m, nil
		}
	case "down", "j":
		if !p.viewing {
			p.cursor = min(len(p.items)-1, p.cursor+1)
			return m, nil
		}
	}
	if p.viewing {
		var cmd tea.Cmd
		p.viewport, cmd = p.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (p *boundedExplainPanel) view() string {
	var body strings.Builder
	body.WriteString(fmt.Sprintf("Bounded resource evidence | context=%q\n\n", p.context))
	if p.viewing {
		body.WriteString(p.viewport.View())
	} else if len(p.items) == 0 {
		body.WriteString("No resources with complete, supported API identity in the current inventory.")
	} else {
		start := max(0, p.cursor-p.viewport.Height+1)
		end := min(len(p.items), start+p.viewport.Height)
		for i := start; i < end; i++ {
			mark := "  "
			if i == p.cursor {
				mark = "> "
			}
			body.WriteString(mark + boundedRefLabel(p.items[i]) + "\n")
		}
	}
	return lipgloss.NewStyle().Width(p.viewport.Width).MaxWidth(p.viewport.Width).MaxHeight(p.height).Render(body.String())
}

func (m *LocalClusterModel) acceptBoundedExplain(msg boundedExplainMsg) {
	p := m.boundedPanel
	if p == nil || p != msg.panel || p.request != msg.request || !p.viewing {
		return
	}
	p.cancel = nil
	if msg.err != nil {
		p.setContent("Evidence unavailable: " + msg.err.Error())
		return
	}
	ref := p.items[p.cursor]
	command := "./" + boundedExplainCommand(ref, p.context, " \\\n  ")
	body := command + "\n\n" + renderExplainText(msg.summary, PresentationHuman, true, HintContext{Mode: HintModeDefault})
	p.setContent(body)
}
