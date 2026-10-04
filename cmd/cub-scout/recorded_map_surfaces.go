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
	cmd.Flags().Int("max-report-json-bytes", 0, "Maximum canonical recorded report JSON bytes (1..4194304; excludes display/transport overhead)")
	cmd.Flags().Int("page-size", 0, "Maximum records per immutable recorded page (1..500; not a byte/token cap)")
	cmd.Flags().String("cursor", "", "Continue a recorded page using its source/scope-bound cursor (requires --page-size)")
}

func recordedMapRequested(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("recording")
}

func runRecordedMapCLI(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("recorded map list accepts no positional arguments")
	}
	for _, name := range []string{"kube-context", "query", "since", "count", "names-only", "explain", "verbose", "cluster-identity"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--recording cannot be combined with --%s", name)
		}
	}
	path, _ := cmd.Flags().GetString("recording")
	scope := RecordedMapScope{}
	scope.APIVersion, _ = cmd.Flags().GetString("api-version")
	scope.Kind, _ = cmd.Flags().GetString("kind")
	scope.NamespacePrefix, _ = cmd.Flags().GetString("namespace-prefix")
	scope.Owner, _ = cmd.Flags().GetString("owner")
	if cmd.Flags().Changed("owner") && scope.Owner == "" {
		return fmt.Errorf("--owner requires a canonical built-in owner")
	}
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
	summaryView, _ := cmd.Flags().GetBool("summary")
	ownershipEvidence, _ := cmd.Flags().GetBool("ownership-evidence")
	if summaryView && ownershipEvidence {
		return fmt.Errorf("--ownership-evidence cannot be combined with --summary; summary omits per-object detector evidence")
	}
	if tui && (cmd.Flags().Changed("format") || legacyJSON) {
		return fmt.Errorf("--tui cannot be combined with output format options")
	}
	jsonBudget, _ := cmd.Flags().GetInt("max-report-json-bytes")
	if cmd.Flags().Changed("max-report-json-bytes") {
		if err := validateRecordedMapJSONBudget(jsonBudget); err != nil {
			return err
		}
	}
	pageSize, _ := cmd.Flags().GetInt("page-size")
	cursor, _ := cmd.Flags().GetString("cursor")
	paged := cmd.Flags().Changed("page-size") || cmd.Flags().Changed("cursor")
	if paged {
		if cmd.Flags().Changed("cursor") && cursor == "" {
			return fmt.Errorf("--cursor requires a nonempty recorded continuation")
		}
		if err := validateRecordedMapPageOptions(pageSize, cursor, summaryView); err != nil {
			return err
		}
	}
	snapshot, err := readRecordedObjectSnapshot(path)
	if err != nil {
		return err
	}
	if summaryView {
		summary, err := buildRecordedMapSummary(snapshot, scope)
		if err != nil {
			return err
		}
		if err := checkRecordedMapJSONBudget(summary, jsonBudget); err != nil {
			return err
		}
		if tui {
			_, err = tea.NewProgram(newRecordedMapSummaryViewer(summary)).Run()
			return err
		}
		return writeRecordedMapSummary(cmd.OutOrStdout(), summary, format)
	}
	report, err := buildRecordedMapReport(snapshot, scope)
	if err != nil {
		return err
	}
	if paged {
		if tui {
			viewer, err := newRecordedMapBudgetedPagedViewer(report, pageSize, cursor, jsonBudget)
			if err != nil {
				return err
			}
			_, err = tea.NewProgram(viewer).Run()
			return err
		}
		report, err = buildRecordedMapPage(report, pageSize, cursor)
		if err != nil {
			return err
		}
	}
	if err := checkRecordedMapJSONBudget(report, jsonBudget); err != nil {
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

func writeRecordedMapSummary(out io.Writer, summary RecordedMapSummary, format string) error {
	if format == "json" {
		return json.NewEncoder(out).Encode(summary)
	}
	if format != "ascii" && format != "md" {
		return fmt.Errorf("recorded map format must be ascii, json, or md")
	}
	_, err := io.WriteString(out, renderRecordedMapSummary(summary, format))
	return err
}

func renderRecordedMapReport(report RecordedMapReport, format string) string {
	var b strings.Builder
	b.WriteString("RECORDED INVENTORY — static evidence; no live cluster was read\n")
	fmt.Fprintf(&b, "Input SHA-256: %s\nBytes: %d; documents: %d; objects: %d\n", report.Provenance.SHA256, report.Provenance.Bytes, report.Provenance.Documents, report.Provenance.ObjectCount)
	fmt.Fprintf(&b, "Capture time: %s; capture completeness: %s\n", report.Provenance.CaptureTime, report.Provenance.CaptureCompleteness)
	if report.Provenance.TypedListDerivedObjects > 0 {
		fmt.Fprintf(&b, "Input objects with type supplied by typed list envelope: %d\n", report.Provenance.TypedListDerivedObjects)
	}
	scope, _ := json.Marshal(report.Scope)
	fmt.Fprintf(&b, "Scope: %s\nSelected: %d; excluded by scope: %d\n", scope, report.SelectedCount, report.ExcludedFromScope)
	if report.Pagination != nil {
		page := report.Pagination
		fmt.Fprintf(&b, "Page: offset %d; returned %d of %d selected; page size %d (record limit, not byte/token cap)\n", page.Offset, page.ReturnedCount, report.SelectedCount, page.PageSize)
		b.WriteString("Owner totals describe the full matched recording.\n")
		if page.NextCursor != "" {
			fmt.Fprintf(&b, "Next cursor: %s\n", page.NextCursor)
		}
	}
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

func renderRecordedMapSummary(summary RecordedMapSummary, format string) string {
	var b strings.Builder
	b.WriteString("RECORDED INVENTORY SUMMARY — static evidence; no live cluster was read\n")
	fmt.Fprintf(&b, "Input SHA-256: %s\nBytes: %d; documents: %d; objects: %d\n", summary.Provenance.SHA256, summary.Provenance.Bytes, summary.Provenance.Documents, summary.Provenance.ObjectCount)
	fmt.Fprintf(&b, "Capture time: %s; capture completeness: %s\n", summary.Provenance.CaptureTime, summary.Provenance.CaptureCompleteness)
	if summary.Provenance.TypedListDerivedObjects > 0 {
		fmt.Fprintf(&b, "Input objects with type supplied by typed list envelope: %d\n", summary.Provenance.TypedListDerivedObjects)
	}
	scope, _ := json.Marshal(summary.Scope)
	fmt.Fprintf(&b, "View: summary\nScope: %s\nSelected: %d; excluded by scope: %d\n", scope, summary.SelectedCount, summary.ExcludedFromScope)
	b.WriteString("Objects absent from this recording are unknown. Native means no built-in owner marker observed; it does not prove an orphan.\n")
	owners := make([]string, 0, len(summary.OwnerCounts))
	for owner := range summary.OwnerCounts {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	for _, owner := range owners {
		fmt.Fprintf(&b, "%s: %d\n", owner, summary.OwnerCounts[owner])
	}
	fmt.Fprintf(&b, "\n%s\n", recordedMapDisplayCell(summary.PerObjectEvidenceGuide, format))
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
		Descriptor: mcpToolDescriptor{Name: "map", Description: "Count and list built-in ownership markers from the immutable recording. Optional owner selects one canonical built-in owner; summary=true returns counts without per-object rows. No live cluster is read; capture completeness and time are unknown. Native means no built-in marker, not an orphan.", Annotations: &mcpToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{
				"api_version":           map[string]interface{}{"type": "string", "description": "Required exact case-sensitive apiVersion."},
				"kind":                  map[string]interface{}{"type": "string", "description": "Required exact case-sensitive kind."},
				"namespace":             map[string]interface{}{"type": "string", "description": "Optional exact namespace, including empty; cannot combine with namespace_prefix."},
				"namespace_prefix":      map[string]interface{}{"type": "string", "minLength": 1, "description": "Optional non-empty literal case-sensitive namespace prefix."},
				"owner":                 map[string]interface{}{"type": "string", "enum": recordedMapOwnerNames, "description": "Optional exact canonical built-in owner, including Kubernetes and Native."},
				"summary":               map[string]interface{}{"type": "boolean", "description": "Return selected/excluded and owner counts without per-object rows."},
				"max_report_json_bytes": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": recordedMapMaxReportJSONBytes, "description": "Maximum canonical report-data JSON bytes; excludes display, duplicated MCP content and protocol overhead. Oversized reports are refused, never clipped."},
				"page_size":             map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 500, "description": "Opt into record-count pages; not a byte/token cap. Incompatible with summary."},
				"cursor":                map[string]interface{}{"type": "string", "minLength": 1, "maxLength": 2048, "description": "Canonical continuation bound to this immutable input, scope and page size; requires page_size."},
			}, "required": []string{"api_version", "kind"}, "additionalProperties": false,
		}},
		BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
			scope := RecordedMapScope{}
			summary := false
			pageSize := 0
			jsonBudget := 0
			cursor := ""
			for key, value := range arguments {
				switch key {
				case "api_version":
					s, ok := value.(string)
					if !ok {
						return nil, fmt.Errorf("%s must be a string", key)
					}
					scope.APIVersion = s
				case "kind":
					s, ok := value.(string)
					if !ok {
						return nil, fmt.Errorf("%s must be a string", key)
					}
					scope.Kind = s
				case "namespace":
					s, ok := value.(string)
					if !ok {
						return nil, fmt.Errorf("%s must be a string", key)
					}
					scope.Namespace = &s
				case "owner":
					s, ok := value.(string)
					if !ok {
						return nil, fmt.Errorf("%s must be a string", key)
					}
					if s == "" {
						return nil, fmt.Errorf("owner must be a canonical built-in owner")
					}
					scope.Owner = s
				case "namespace_prefix":
					s, ok := value.(string)
					if !ok {
						return nil, fmt.Errorf("%s must be a string", key)
					}
					if s == "" {
						return nil, fmt.Errorf("namespace_prefix must be non-empty")
					}
					scope.NamespacePrefix = s
				case "max_report_json_bytes":
					parsed, err := recordedMapJSONBudgetArgument(value)
					if err != nil {
						return nil, err
					}
					jsonBudget = parsed
				case "page_size":
					parsed, err := recordedMapPageSizeArgument(value)
					if err != nil {
						return nil, err
					}
					pageSize = parsed
				case "cursor":
					text, ok := value.(string)
					if !ok || text == "" {
						return nil, fmt.Errorf("cursor must be a nonempty string")
					}
					cursor = text
				case "summary":
					var ok bool
					summary, ok = value.(bool)
					if !ok {
						return nil, fmt.Errorf("summary must be a boolean")
					}
				default:
					return nil, fmt.Errorf("unsupported recorded map argument %q", key)
				}
			}
			if err := validateRecordedMapScope(scope); err != nil {
				return nil, err
			}
			if pageSize > 0 || cursor != "" {
				if err := validateRecordedMapPageOptions(pageSize, cursor, summary); err != nil {
					return nil, err
				}
			}
			request := struct {
				Scope      RecordedMapScope `json:"scope"`
				Summary    bool             `json:"summary,omitempty"`
				PageSize   int              `json:"pageSize,omitempty"`
				JSONBudget int              `json:"maxReportJSONBytes,omitempty"`
				Cursor     string           `json:"cursor,omitempty"`
			}{Scope: scope, Summary: summary, PageSize: pageSize, Cursor: cursor, JSONBudget: jsonBudget}
			raw, err := json.Marshal(request)
			return []string{string(raw)}, err
		},
		Runner: func(ctx context.Context, args []string) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if len(args) != 1 {
				return "", fmt.Errorf("invalid recorded map scope")
			}
			var request struct {
				Scope      RecordedMapScope `json:"scope"`
				Summary    bool             `json:"summary"`
				PageSize   int              `json:"pageSize"`
				JSONBudget int              `json:"maxReportJSONBytes"`
				Cursor     string           `json:"cursor"`
			}
			if err := json.Unmarshal([]byte(args[0]), &request); err != nil {
				return "", fmt.Errorf("invalid recorded map scope")
			}
			if request.PageSize != 0 || request.Cursor != "" {
				if err := validateRecordedMapPageOptions(request.PageSize, request.Cursor, request.Summary); err != nil {
					return "", err
				}
			}
			if request.JSONBudget != 0 {
				if err := validateRecordedMapJSONBudget(request.JSONBudget); err != nil {
					return "", err
				}
			}
			var response interface{}
			var err error
			if request.Summary {
				response, err = buildRecordedMapSummary(snapshot, request.Scope)
			} else {
				report, buildErr := buildRecordedMapReport(snapshot, request.Scope)
				err = buildErr
				if err == nil && request.PageSize > 0 {
					report, err = buildRecordedMapPage(report, request.PageSize, request.Cursor)
				}
				response = report
			}
			if err != nil {
				return "", err
			}
			raw, err := json.Marshal(response)
			if err != nil {
				return "", err
			}
			if err := checkRecordedMapJSONBudgetBytes(raw, request.JSONBudget); err != nil {
				return "", err
			}
			return string(raw), nil
		},
	}
}

type recordedMapViewer struct {
	content   string
	viewport  viewport.Model
	pager     *recordedMapPager
	pageError string
}

func newRecordedMapViewer(report RecordedMapReport) recordedMapViewer {
	return newRecordedMapViewerContent(renderRecordedMapReport(report, "ascii"))
}

func newRecordedMapSummaryViewer(summary RecordedMapSummary) recordedMapViewer {
	return newRecordedMapViewerContent(renderRecordedMapSummary(summary, "ascii"))
}

func newRecordedMapViewerContent(content string) recordedMapViewer {
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
		if m.pager != nil && (value.String() == "n" || value.String() == "p") {
			return m.updateRecordedPage(value.String()), nil
		}
		if value.String() == "q" || value.String() == "esc" || value.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}
func (m recordedMapViewer) View() string {
	hint := "Recorded inventory · q to quit · arrows/page keys to scroll\n"
	if m.pager != nil {
		hint = "Recorded inventory · n/p for next/previous page · q to quit · arrows to scroll\n"
	}
	if m.pageError != "" {
		hint += "Page refused: " + m.pageError + "\n"
	}
	return hint + m.viewport.View()
}
