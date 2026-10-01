// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

type sourceTruthResultMsg struct {
	evidence    agent.SourceTruthEvidence
	err         error
	generation  uint64
	item        TraceItem
	strategy    agent.SourceTruthStrategy
	contextName string
}

var sourceTruthCollectFn = collectSourceTruthObservation

func (m LocalClusterModel) runSourceTruth(item TraceItem, strategy agent.SourceTruthStrategy, generation uint64, ctx context.Context) tea.Cmd {
	contextName := ""
	if m.clusterBinding != nil {
		contextName = m.clusterBinding.context
	}
	return func() tea.Msg {
		if ctx == nil {
			ctx = context.Background()
		}
		if err := ctx.Err(); err != nil {
			return sourceTruthResultMsg{err: err, generation: generation, item: item, strategy: strategy, contextName: contextName}
		}
		if err := requireConfigHubFor("TUI source-truth"); err != nil {
			return sourceTruthResultMsg{err: err, generation: generation, item: item, strategy: strategy, contextName: contextName}
		}
		session, err := newTraceSessionFromBinding(m.clusterBinding)
		if err != nil {
			return sourceTruthResultMsg{err: fmt.Errorf("resolve selected Kubernetes context: %w", err), generation: generation, item: item, strategy: strategy, contextName: contextName}
		}
		observation := sourceTruthCollectFn(ctx, session, item.Kind, item.Name, item.Namespace, strategy)
		return sourceTruthResultMsg{evidence: observation.Evidence, generation: generation, item: item, strategy: strategy, contextName: contextName}
	}
}

func (m LocalClusterModel) renderSourceTruth() string {
	var b strings.Builder
	b.WriteString(lcHeaderStyle.Render("SOURCE-TRUTH EVIDENCE"))
	b.WriteString("\n\n")
	if m.sourceTruthLoading {
		b.WriteString("  " + m.spinner.View() + " Collecting source-truth evidence from the selected Kubernetes context and ConfigHub.\n")
		return b.String()
	}
	if m.sourceTruthError != nil {
		fmt.Fprintf(&b, "Error: %s\n\n", m.sourceTruthError)
		b.WriteString(lcDimStyle.Render("Press any key to return"))
		return b.String()
	}
	if m.sourceTruthEvidence != nil {
		b.WriteString(renderSourceTruthASCII(*m.sourceTruthEvidence))
		b.WriteString("\n" + lcDimStyle.Render("Press any key to return"))
		return b.String()
	}
	contextLabel := "default context"
	if m.clusterBinding != nil && strings.TrimSpace(m.clusterBinding.context) != "" {
		contextLabel = m.clusterBinding.context
	}
	fmt.Fprintf(&b, "Workload: %s/%s in %s\n", m.sourceTruthItem.Kind, m.sourceTruthItem.Name, m.sourceTruthItem.Namespace)
	fmt.Fprintf(&b, "Selected Kubernetes context: %s (label, not stable cluster identity)\n\n", contextLabel)
	b.WriteString("Select the declared delivery strategy. cub-scout will not infer it.\n\n")
	strategies := agent.AllStrategies()
	for i, strategy := range strategies {
		cursor := "  "
		if i == m.sourceTruthCursor {
			cursor = lcOkStyle.Render("▸ ")
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, strategy.Human())
	}
	b.WriteString("\n" + lcDimStyle.Render("↑/k ↓/j select, Enter collect, Esc cancel"))
	return b.String()
}
