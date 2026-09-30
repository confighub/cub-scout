// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/spf13/cobra"
)

func addRecordedMapFlags(cmd *cobra.Command) {
	cmd.Flags().String("recording", "", "Read ownership inventory from an immutable local recording, without cluster access")
	cmd.Flags().String("api-version", "", "Required exact apiVersion for --recording")
	cmd.Flags().String("namespace-prefix", "", "Literal recorded namespace prefix (requires --recording)")
	cmd.Flags().Bool("tui", false, "View recorded inventory interactively (requires --recording)")
}

func recordedMapRequested(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("recording")
}

func runRecordedMapCLI(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("recorded map list accepts no positional arguments")
	}
	for _, name := range []string{"kube-context", "owner", "query", "since", "count", "names-only", "summary", "explain", "verbose"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--recording cannot be combined with --%s", name)
		}
	}
	path, _ := cmd.Flags().GetString("recording")
	scope := RecordedMapScope{}
	scope.APIVersion, _ = cmd.Flags().GetString("api-version")
	scope.Kind, _ = cmd.Flags().GetString("kind")
	scope.NamespacePrefix, _ = cmd.Flags().GetString("namespace-prefix")
	if cmd.Flags().Changed("namespace") {
		ns, _ := cmd.Flags().GetString("namespace")
		scope.Namespace = &ns
	}
	if cmd.Flags().Changed("namespace-prefix") && scope.NamespacePrefix == "" {
		return fmt.Errorf("--namespace-prefix requires a non-empty literal prefix")
	}
	if err := validateRecordedMapScope(scope); err != nil {
		return err
	}
	format, _ := cmd.Flags().GetString("format")
	legacyJSON, _ := cmd.Flags().GetBool("json")
	if legacyJSON && format == "ascii" {
		format = "json"
	}
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("recorded map format must be ascii, json, or md")
	}
	tui, _ := cmd.Flags().GetBool("tui")
	if tui && (cmd.Flags().Changed("format") || legacyJSON) {
		return fmt.Errorf("--tui cannot be combined with output format options")
	}
	snapshot, err := readRecordedObjectSnapshot(path)
	if err != nil {
		return err
	}
	report, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		return err
	}
	if tui {
		_, err = tea.NewProgram(newRecordedMapViewer(report)).Run()
		return err
	}
	return writeRecordedMapReport(cmd.OutOrStdout(), report, format)
}

func writeRecordedMapReport(out io.Writer, report RecordedMapReport, format string) error {
	if format == "json" {
		return json.NewEncoder(out).Encode(report)
	}
	if format != "ascii" && format != "md" {
		return fmt.Errorf("recorded map format must be ascii, json, or md")
	}
	_, err := io.WriteString(out, renderRecordedMapReport(report, format))
	return err
}

func renderRecordedMapReport(report RecordedMapReport, format string) string {
	var b strings.Builder
	b.WriteString("RECORDED INVENTORY — static evidence; no live cluster was read\n")
	fmt.Fprintf(&b, "Input SHA-256: %s\nBytes: %d; documents: %d; objects: %d\n", report.Provenance.SHA256, report.Provenance.Bytes, report.Provenance.Documents, report.Provenance.ObjectCount)
	fmt.Fprintf(&b, "Capture time: %s; capture completeness: %s\n", report.Provenance.CaptureTime, report.Provenance.CaptureCompleteness)
	scope, _ := json.Marshal(report.Scope)
	fmt.Fprintf(&b, "Scope: %s\nSelected: %d; excluded by scope: %d\n", scope, report.SelectedCount, report.ExcludedFromScope)
	b.WriteString("Objects absent from this recording are unknown. Native means no built-in owner marker observed; it does not prove an orphan.\n")
	owners := make([]string, 0, len(report.OwnerCounts))
	for owner := range report.OwnerCounts {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	for _, owner := range owners {
		fmt.Fprintf(&b, "%s: %d\n", owner, report.OwnerCounts[owner])
	}
	b.WriteString("\n")
	if format == "md" {
		b.WriteString("| API version | Kind | Namespace | Name | Owner | Detector evidence |\n|---|---|---|---|---|---|\n")
	}
	for _, r := range report.Resources {
		cells := []string{r.APIVersion, r.Kind, r.Namespace, r.Name, r.Owner, mapsvc.OwnershipDetectionSummary(r.OwnershipDetection)}
		for i, cell := range cells {
			cells[i] = recordedMapDisplayCell(cell, format)
		}
		if format == "md" {
			fmt.Fprintf(&b, "| %s |\n", strings.Join(cells, " | "))
		} else {
			b.WriteString(strings.Join(cells, "\t") + "\n")
		}
	}
	return b.String()
}

func recordedMapDisplayCell(value, format string) string {
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, value)
	if format == "md" {
		value = strings.ReplaceAll(value, "|", "\\|")
		value = strings.ReplaceAll(value, "<", "&lt;")
		value = strings.ReplaceAll(value, ">", "&gt;")
	}
	return value
}

func recordedMapTool(snapshot recordedObjectSnapshot) mcpTool {
	return mcpTool{
		Descriptor: mcpToolDescriptor{Name: "map", Description: "Count and list built-in ownership markers from the immutable recording. Returns input hash, exact selection scope, owner counts and per-object detector evidence. No live cluster is read; capture completeness and time are unknown. Native means no built-in marker, not an orphan.", Annotations: &mcpToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{
				"api_version":      map[string]interface{}{"type": "string", "description": "Required exact case-sensitive apiVersion."},
				"kind":             map[string]interface{}{"type": "string", "description": "Required exact case-sensitive kind."},
				"namespace":        map[string]interface{}{"type": "string", "description": "Optional exact namespace, including empty; cannot combine with namespace_prefix."},
				"namespace_prefix": map[string]interface{}{"type": "string", "minLength": 1, "description": "Optional non-empty literal case-sensitive namespace prefix."},
			}, "required": []string{"api_version", "kind"}, "additionalProperties": false,
		}},
		BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
			scope := RecordedMapScope{}
			for key, value := range arguments {
				s, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("%s must be a string", key)
				}
				switch key {
				case "api_version":
					scope.APIVersion = s
				case "kind":
					scope.Kind = s
				case "namespace":
					scope.Namespace = &s
				case "namespace_prefix":
					if s == "" {
						return nil, fmt.Errorf("namespace_prefix must be non-empty")
					}
					scope.NamespacePrefix = s
				default:
					return nil, fmt.Errorf("unsupported recorded map argument %q", key)
				}
			}
			if err := validateRecordedMapScope(scope); err != nil {
				return nil, err
			}
			raw, err := json.Marshal(scope)
			return []string{string(raw)}, err
		},
		Runner: func(ctx context.Context, args []string) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if len(args) != 1 {
				return "", fmt.Errorf("invalid recorded map scope")
			}
			var scope RecordedMapScope
			if err := json.Unmarshal([]byte(args[0]), &scope); err != nil {
				return "", fmt.Errorf("invalid recorded map scope")
			}
			report, err := buildRecordedMapReport(snapshot, scope)
			if err != nil {
				return "", err
			}
			raw, err := json.Marshal(report)
			return string(raw), err
		},
	}
}

type recordedMapViewer struct {
	content  string
	viewport viewport.Model
}

func newRecordedMapViewer(report RecordedMapReport) recordedMapViewer {
	content := renderRecordedMapReport(report, "ascii")
	vp := viewport.New(80, 20)
	vp.SetContent(wrapRecordedExplainText(content, 76))
	return recordedMapViewer{content: content, viewport: vp}
}
func (m recordedMapViewer) Init() tea.Cmd { return nil }
func (m recordedMapViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
func (m recordedMapViewer) View() string {
	return "Recorded inventory · q to quit · arrows/page keys to scroll\n" + m.viewport.View()
}
