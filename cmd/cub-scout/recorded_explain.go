// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
)

func readRecordedObjectSnapshot(path string) (recordedObjectSnapshot, error) {
	if strings.TrimSpace(path) == "" {
		return recordedObjectSnapshot{}, fmt.Errorf("recording file is required")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return recordedObjectSnapshot{}, fmt.Errorf("recording must be a readable regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return recordedObjectSnapshot{}, fmt.Errorf("recording must be a readable regular file")
	}
	defer f.Close()
	openedInfo, err := f.Stat()
	if err != nil || !openedInfo.Mode().IsRegular() {
		return recordedObjectSnapshot{}, fmt.Errorf("recording must be a readable regular file")
	}
	return loadRecordedObjectSnapshot(io.LimitReader(f, maxRecordedObjectBytes+1))
}

func parseRecordedExplainArgs(args []string) (kind, name string, err error) {
	if len(args) == 1 {
		parts := strings.Split(args[0], "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("recorded explain requires exact Kind/name")
		}
		return parts[0], parts[1], nil
	}
	if len(args) == 2 && args[0] != "" && args[1] != "" {
		return args[0], args[1], nil
	}
	return "", "", fmt.Errorf("recorded explain requires exact Kind/name")
}

func recordedExplainSummary(snapshot recordedObjectSnapshot, identity recordedObjectIdentity, fieldPath string) (ExplainSummary, error) {
	if fieldPath != "" {
		if err := agent.ValidateCanonicalFieldPath(fieldPath); err != nil {
			return ExplainSummary{}, err
		}
	}
	recorded, err := snapshot.selectObject(identity)
	if err != nil {
		return ExplainSummary{}, err
	}
	return buildRecordedExplainSummary(recorded, identity, fieldPath), nil
}

func runRecordedExplainCLI(cmd *cobra.Command, args []string, format string) error {
	for _, name := range []string{"bounded", "kube-context", "refresh", "expected-revision", "with-confighub", "confighub-space", "confighub-since", "confighub-stale-after"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--recording cannot be combined with --%s", name)
		}
	}
	if explainBounded || explainContext != "" || explainRefresh || explainExpectedRevision != "" || explainWithConfigHub ||
		explainConfigHubSpace != "" || explainConfigHubSince != "24h" || explainConfigHubStaleAfter != "15m" {
		return fmt.Errorf("--recording cannot be combined with live or ConfigHub options")
	}
	if explainWithConfigHub || explainConfigHubSpace != "" || cmd.Flags().Changed("confighub-space") || cmd.Flags().Changed("confighub-since") || cmd.Flags().Changed("confighub-stale-after") {
		return fmt.Errorf("--recording cannot be combined with ConfigHub enrichment")
	}
	if !cmd.Flags().Changed("namespace") {
		return fmt.Errorf("--recording requires an explicit --namespace (use --namespace=\"\" for an empty namespace)")
	}
	if !cmd.Flags().Changed("api-version") || strings.TrimSpace(explainAPIVersion) == "" {
		return fmt.Errorf("--recording requires --api-version")
	}
	kind, name, err := parseRecordedExplainArgs(args)
	if err != nil {
		return err
	}
	identity := recordedObjectIdentity{APIVersion: explainAPIVersion, Kind: kind, Namespace: explainNamespace, Name: name}
	snapshot, err := readRecordedObjectSnapshot(explainRecording)
	if err != nil {
		return err
	}
	summary, err := recordedExplainSummary(snapshot, identity, explainFieldPath)
	if err != nil {
		return err
	}
	if explainTUI {
		return runRecordedExplainTUI(summary)
	}
	invCtx, err := NewInvocationContext(explainPresentation, TransportCLI)
	if err != nil {
		return err
	}
	hintMode, err := ParseHintMode(explainHintMode)
	if err != nil {
		return err
	}
	return outputExplainSummary(summary, format, invCtx, HintContext{Mode: hintMode})
}

func recordedExplainTool(snapshot recordedObjectSnapshot) mcpTool {
	return mcpTool{
		Descriptor: mcpToolDescriptor{
			Name:        "explain",
			Description: "Explain one exact object from the immutable recording configured when this MCP server started. This is static recorded evidence, not current cluster state. It reads no live cluster or ConfigHub data.",
			Annotations: &mcpToolAnnotations{ReadOnlyHint: true},
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"api_version": map[string]interface{}{"type": "string", "description": "Exact recorded apiVersion, such as apps/v1."},
					"kind":        map[string]interface{}{"type": "string", "description": "Exact Kubernetes kind."},
					"namespace":   map[string]interface{}{"type": "string", "description": "Exact namespace; use the empty string only for an explicitly empty recorded namespace."},
					"name":        map[string]interface{}{"type": "string", "description": "Exact resource name."},
					"field_path":  map[string]interface{}{"type": "string", "description": "Optional exact canonical managedFields path."},
				},
				"required":             []string{"api_version", "kind", "namespace", "name"},
				"additionalProperties": false,
			},
		},
		BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
			allowed := map[string]bool{"api_version": true, "kind": true, "namespace": true, "name": true, "field_path": true}
			for key := range arguments {
				if !allowed[key] {
					return nil, fmt.Errorf("unsupported recorded explain argument %q", key)
				}
			}
			for _, key := range []string{"api_version", "kind", "namespace", "name"} {
				if _, ok := arguments[key].(string); !ok {
					return nil, fmt.Errorf("%s must be a string", key)
				}
			}
			fieldPath := ""
			if value, present := arguments["field_path"]; present {
				var ok bool
				fieldPath, ok = value.(string)
				if !ok {
					return nil, fmt.Errorf("field_path must be a string")
				}
			}
			return []string{arguments["api_version"].(string), arguments["kind"].(string), arguments["namespace"].(string), arguments["name"].(string), fieldPath}, nil
		},
		Runner: func(ctx context.Context, args []string) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if len(args) != 5 {
				return "", fmt.Errorf("invalid recorded explain selection")
			}
			identity := recordedObjectIdentity{APIVersion: args[0], Kind: args[1], Namespace: args[2], Name: args[3]}
			summary, err := recordedExplainSummary(snapshot, identity, args[4])
			if err != nil {
				return "", err
			}
			out, err := json.Marshal(withExplainJSONHints(summary, HintContext{Mode: HintModeDefault}))
			return string(out), err
		},
	}
}

func newRecordedMCPGateway(snapshot recordedObjectSnapshot) *mcpGateway {
	tool := recordedExplainTool(snapshot)
	return &mcpGateway{
		tools:    map[string]mcpTool{"explain": tool},
		toolList: []mcpToolDescriptor{tool.Descriptor},
		// No ordinary or connected runner is installed in recorded-only mode.
	}
}

type recordedExplainViewer struct {
	content  string
	viewport viewport.Model
}

func newRecordedExplainViewer(summary ExplainSummary) recordedExplainViewer {
	content := "RECORDED OBJECT — static evidence; no live cluster was read\n" + renderExplainMarkdown(summary, PresentationMode(""), false, HintContext{})
	vp := viewport.New(80, 20)
	vp.SetContent(wrapRecordedExplainText(content, 76))
	return recordedExplainViewer{content: content, viewport: vp}
}

func wrapRecordedExplainText(content string, width int) string {
	if width < 1 {
		return content
	}
	var out []string
	for _, line := range strings.Split(content, "\n") {
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		remaining := strings.TrimSpace(line)
		if remaining == "" {
			out = append(out, "")
			continue
		}
		for len([]rune(indent+remaining)) > width {
			available := width - len([]rune(indent))
			if available < 1 {
				available = 1
			}
			runes := []rune(remaining)
			cut := available
			if cut < len(runes) {
				for cut > 0 && runes[cut] != ' ' {
					cut--
				}
			}
			if cut == 0 {
				cut = available
			}
			out = append(out, indent+strings.TrimRight(string(runes[:cut]), " "))
			remaining = strings.TrimSpace(string(runes[cut:]))
		}
		out = append(out, indent+remaining)
	}
	return strings.Join(out, "\n")
}

func (m recordedExplainViewer) Init() tea.Cmd { return nil }

func (m recordedExplainViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch value := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = max(1, value.Width-4)
		m.viewport.Height = max(1, value.Height-5)
		m.viewport.SetContent(wrapRecordedExplainText(m.content, m.viewport.Width))
	case tea.KeyMsg:
		if value.String() == "q" || value.String() == "esc" || value.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m recordedExplainViewer) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render("Recorded explain · q to quit · arrows/page keys to scroll")
	return header + "\n" + m.viewport.View()
}

func runRecordedExplainTUI(summary ExplainSummary) error {
	program := tea.NewProgram(newRecordedExplainViewer(summary))
	_, err := program.Run()
	return err
}
