// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
)

const changeOrderReadSchema = "confighub.changeorderRead.v1"

// This identifies the inspected parser contract, not the runtime server version.
const changeOrderReadContract = "confighub/sdk v0.6.8 4c8d2fc3885fed0d7af6835f2aac0a24387b6221"
const changeOrderEvaluationLimit = "The inspected GET exposes declarations, not evaluated prerequisite, approval, gate, publication or advancement outcomes."
const maxChangeOrderReadBytes = 1 << 20

var changeOrderSelector = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type changeOrderReadQuery struct {
	Order string `json:"order"`
	Space string `json:"space"`
}
type changeOrderReadProjection struct {
	Schema              string               `json:"schema"`
	ReadContract        string               `json:"readContract"`
	Requested           changeOrderReadQuery `json:"requested"`
	ChangeOrderID       string               `json:"changeOrderId"`
	Slug                string               `json:"slug"`
	SpaceID             string               `json:"spaceId"`
	SpaceSlug           string               `json:"spaceSlug,omitempty"`
	ReportedStage       *string              `json:"reportedStage,omitempty"`
	ReportedState       *string              `json:"reportedState,omitempty"`
	WorkflowID          *string              `json:"workflowId,omitempty"`
	WorkflowDeclaration json.RawMessage      `json:"workflowDeclaration,omitempty"`
	DeclarationCoverage string               `json:"declarationCoverage"`
	Evaluation          string               `json:"evaluation"`
	Omissions           []agent.Omission     `json:"omissions,omitempty"`
	Limitations         []string             `json:"limitations"`
}

// PascalCase declaration tags preserve the pinned SDK's stored specification.
// These fields are not interpreted as evaluated gates or effective defaults.
type changeOrderWorkflowSpec struct {
	Stages                   []changeOrderWorkflowStage           `json:"Stages"`
	Final                    *changeOrderFinalStage               `json:"Final,omitempty"`
	CustomPrerequisites      []changeOrderCustomPrerequisite      `json:"CustomPrerequisites,omitempty"`
	AttestationPrerequisites []changeOrderAttestationPrerequisite `json:"AttestationPrerequisites,omitempty"`
}
type changeOrderWorkflowStage struct {
	Name                 string   `json:"Name"`
	Prerequisites        []string `json:"Prerequisites,omitempty"`
	ReleasePrerequisites []string `json:"ReleasePrerequisites,omitempty"`
	WhereSpace           string   `json:"WhereSpace,omitempty"`
}
type changeOrderFinalStage struct {
	Prerequisites []string `json:"Prerequisites,omitempty"`
}
type changeOrderCustomPrerequisite struct {
	Name        string `json:"Name"`
	Expression  string `json:"Expression"`
	Description string `json:"Description,omitempty"`
}
type changeOrderAttestationPrerequisite struct {
	Name           string   `json:"Name"`
	Type           *string  `json:"Type,omitempty"`
	Count          *int     `json:"Count,omitempty"`
	AllowAuthors   *bool    `json:"AllowAuthors,omitempty"`
	IgnoreFail     *bool    `json:"IgnoreFail,omitempty"`
	MaxAge         *string  `json:"MaxAge,omitempty"`
	FromUserIDs    []string `json:"FromUserIDs,omitempty"`
	FromGroupIDs   []string `json:"FromGroupIDs,omitempty"`
	DistinctGroups *bool    `json:"DistinctGroups,omitempty"`
	Description    string   `json:"Description,omitempty"`
}

func normalizeChangeOrderReadQuery(q changeOrderReadQuery) (changeOrderReadQuery, error) {
	if !changeOrderSelector.MatchString(q.Space) {
		return q, fmt.Errorf("an explicit exact --space slug or ID is required; wildcard, options and paths are refused")
	}
	if strings.Contains(q.Order, "/") {
		parts := strings.Split(q.Order, "/")
		if len(parts) != 2 || parts[0] != q.Space {
			return q, fmt.Errorf("qualified ChangeOrder must name the explicit exact space")
		}
		q.Order = parts[1]
	}
	if !changeOrderSelector.MatchString(q.Order) {
		return q, fmt.Errorf("ChangeOrder must be one exact slug or ID, not a wildcard or option")
	}
	return q, nil
}

func changeOrderGetArgs(q changeOrderReadQuery) []string {
	return []string{"changeorder", "get", q.Order, "-o", "json", "--space", q.Space}
}

// Token validation rejects duplicate keys before encoding/json can discard
// them, including nested declarations and ignored entity metadata.
func validateChangeOrderJSON(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("ChangeOrder JSON exceeds nesting limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return fmt.Errorf("invalid JSON delimiter")
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("JSON object key is not a string")
			}
			if seen[name] {
				return fmt.Errorf("ChangeOrder JSON has duplicate key %q", name)
			}
			seen[name] = true
		}
		if err := validateChangeOrderJSON(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func changeOrderObject(raw json.RawMessage, name string) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("%s object is missing", name)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("%s must be an object", name)
	}
	return obj, nil
}

func changeOrderField(obj map[string]json.RawMessage, key string, out interface{}) error {
	for name := range obj {
		if name != key && strings.EqualFold(name, key) {
			return fmt.Errorf("ChangeOrder field %q has unsupported casing", name)
		}
	}
	if raw, ok := obj[key]; ok {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("ChangeOrder %s has an invalid type: %w", key, err)
		}
	}
	return nil
}

func validateChangeOrderWorkflowCasing(value interface{}) error {
	keys := []string{"Stages", "Final", "CustomPrerequisites", "AttestationPrerequisites", "Name", "Prerequisites", "ReleasePrerequisites", "WhereSpace", "Expression", "Description", "Type", "Count", "AllowAuthors", "IgnoreFail", "MaxAge", "FromUserIDs", "FromGroupIDs", "DistinctGroups"}
	switch node := value.(type) {
	case map[string]interface{}:
		for name, child := range node {
			for _, key := range keys {
				if name != key && strings.EqualFold(name, key) {
					return fmt.Errorf("workflow field %q has unsupported casing", name)
				}
			}
			if err := validateChangeOrderWorkflowCasing(child); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, child := range node {
			if err := validateChangeOrderWorkflowCasing(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func projectChangeOrderRead(raw string, query changeOrderReadQuery) (changeOrderReadProjection, error) {
	q, err := normalizeChangeOrderReadQuery(query)
	if err != nil {
		return changeOrderReadProjection{}, err
	}
	if !utf8.ValidString(raw) {
		return changeOrderReadProjection{}, fmt.Errorf("ChangeOrder response is not valid UTF-8")
	}
	if len(raw) > maxChangeOrderReadBytes {
		return changeOrderReadProjection{}, fmt.Errorf("ChangeOrder response exceeds 1 MiB limit")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := validateChangeOrderJSON(decoder, 0); err != nil {
		return changeOrderReadProjection{}, fmt.Errorf("invalid ChangeOrder JSON: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return changeOrderReadProjection{}, fmt.Errorf("ChangeOrder response must contain one JSON object")
	}
	outer, err := changeOrderObject(json.RawMessage(raw), "response")
	if err != nil {
		return changeOrderReadProjection{}, err
	}
	for _, key := range []string{"ChangeOrder", "Space", "Error"} {
		for name := range outer {
			if name != key && strings.EqualFold(name, key) {
				return changeOrderReadProjection{}, fmt.Errorf("response field %q has unsupported casing", name)
			}
		}
	}
	if errorRaw, ok := outer["Error"]; ok && !bytes.Equal(bytes.TrimSpace(errorRaw), []byte("null")) {
		return changeOrderReadProjection{}, fmt.Errorf("ChangeOrder read returned an Error object")
	}
	row := outer
	if nested, ok := outer["ChangeOrder"]; ok {
		if _, conflict := outer["ChangeOrderID"]; conflict {
			return changeOrderReadProjection{}, fmt.Errorf("ambiguous flat and wrapped ChangeOrder identity")
		}
		row, err = changeOrderObject(nested, "ChangeOrder")
		if err != nil {
			return changeOrderReadProjection{}, err
		}
	}
	result := changeOrderReadProjection{Schema: changeOrderReadSchema, ReadContract: changeOrderReadContract, Requested: q, Evaluation: "unknown", DeclarationCoverage: "unavailable", Limitations: []string{changeOrderEvaluationLimit, "Reported Stage/State, including Completed, do not establish runtime health, live convergence or governance acceptance."}}
	for _, field := range []struct {
		key string
		out interface{}
	}{{"ChangeOrderID", &result.ChangeOrderID}, {"Slug", &result.Slug}, {"SpaceID", &result.SpaceID}, {"SpaceSlug", &result.SpaceSlug}, {"Stage", &result.ReportedStage}, {"State", &result.ReportedState}, {"ChangeWorkflowID", &result.WorkflowID}} {
		if err := changeOrderField(row, field.key, field.out); err != nil {
			return result, err
		}
	}
	if !agent.IsUUID(result.ChangeOrderID) || result.ChangeOrderID == "00000000-0000-0000-0000-000000000000" || !changeOrderSelector.MatchString(result.Slug) || !agent.IsUUID(result.SpaceID) || result.SpaceID == "00000000-0000-0000-0000-000000000000" {
		return result, fmt.Errorf("ChangeOrder response requires exact ChangeOrderID, Slug and SpaceID")
	}
	if result.WorkflowID != nil && (!agent.IsUUID(*result.WorkflowID) || *result.WorkflowID == "00000000-0000-0000-0000-000000000000") {
		return result, fmt.Errorf("ChangeWorkflowID is not a UUID")
	}
	if result.SpaceSlug != "" && !changeOrderSelector.MatchString(result.SpaceSlug) {
		return result, fmt.Errorf("reported SpaceSlug is invalid")
	}
	if spaceRaw, ok := outer["Space"]; ok && !bytes.Equal(bytes.TrimSpace(spaceRaw), []byte("null")) {
		space, err := changeOrderObject(spaceRaw, "Space")
		if err != nil {
			return result, err
		}
		var id, slug string
		if err := changeOrderField(space, "SpaceID", &id); err != nil {
			return result, err
		}
		if err := changeOrderField(space, "Slug", &slug); err != nil {
			return result, err
		}
		if id != "" && id != result.SpaceID {
			return result, fmt.Errorf("related SpaceID conflicts with ChangeOrder SpaceID")
		}
		if result.SpaceSlug != "" && slug != "" && result.SpaceSlug != slug {
			return result, fmt.Errorf("related Space slug conflicts with ChangeOrder SpaceSlug")
		}
		if result.SpaceSlug == "" {
			result.SpaceSlug = slug
		}
	}
	if result.SpaceSlug != "" && !changeOrderSelector.MatchString(result.SpaceSlug) {
		return result, fmt.Errorf("related Space slug is invalid")
	}
	if (agent.IsUUID(q.Order) && q.Order != result.ChangeOrderID) || (!agent.IsUUID(q.Order) && q.Order != result.Slug) {
		return result, fmt.Errorf("returned ChangeOrder identity does not match the requested selector")
	}
	if (agent.IsUUID(q.Space) && q.Space != result.SpaceID) || (!agent.IsUUID(q.Space) && q.Space != result.SpaceSlug) {
		return result, fmt.Errorf("returned ChangeOrder space does not match the explicit space")
	}
	omit := func(missing, reason string) {
		result.Omissions = append(result.Omissions, agent.Omission{Missing: missing, Reason: reason, Severity: "inconclusive"})
	}
	if result.ReportedStage == nil || *result.ReportedStage == "" {
		omit("stage", "No Stage was reported; no stage or completion is inferred.")
	}
	if result.ReportedState == nil || *result.ReportedState == "" {
		omit("state", "No State was reported; no governance or runtime outcome is inferred.")
	}
	for name := range row {
		if name != "ChangeWorkflow" && strings.EqualFold(name, "ChangeWorkflow") {
			return result, fmt.Errorf("workflow field %q has unsupported casing", name)
		}
	}
	if workflowRaw, ok := row["ChangeWorkflow"]; ok && !bytes.Equal(bytes.TrimSpace(workflowRaw), []byte("null")) {
		var workflowValue interface{}
		if err := json.Unmarshal(workflowRaw, &workflowValue); err != nil {
			return result, err
		}
		if err := validateChangeOrderWorkflowCasing(workflowValue); err != nil {
			return result, err
		}
		workflowDecoder := json.NewDecoder(bytes.NewReader(workflowRaw))
		workflowDecoder.DisallowUnknownFields()
		var workflow *changeOrderWorkflowSpec
		if err := workflowDecoder.Decode(&workflow); err != nil {
			return result, fmt.Errorf("invalid workflow declaration: %w", err)
		}
		if workflow == nil {
			return result, fmt.Errorf("workflow declaration is not an object")
		}
		result.DeclarationCoverage = "reported"
		result.WorkflowDeclaration = append(json.RawMessage(nil), workflowRaw...)
		if result.WorkflowID == nil {
			result.DeclarationCoverage = "partial"
			omit("workflow.identity", "Workflow ID was not reported; declarations do not establish governing identity.")
		}
		if len(workflow.Stages) == 0 {
			result.DeclarationCoverage = "partial"
			omit("workflow.stages", "Workflow stages were not reported; no ungoverned state is inferred.")
		}
		for _, stage := range workflow.Stages {
			if stage.Name == "" {
				result.DeclarationCoverage = "partial"
				omit("workflow.stage.name", "A reported stage has no name.")
			}
		}
		for _, prerequisite := range workflow.CustomPrerequisites {
			if prerequisite.Name == "" || prerequisite.Expression == "" {
				result.DeclarationCoverage = "partial"
				omit("workflow.customPrerequisite", "A custom prerequisite lacks a name or expression; no evaluation is inferred.")
			}
		}
		for _, prerequisite := range workflow.AttestationPrerequisites {
			if prerequisite.Name == "" {
				result.DeclarationCoverage = "partial"
				omit("workflow.attestationPrerequisite", "An attestation prerequisite lacks a name; no evaluation is inferred.")
			}
		}
	} else {
		omit("workflow", "Stored workflow declaration is unavailable; absence does not establish ungoverned or approved state.")
	}
	return result, nil
}

func readChangeOrderProjection(ctx context.Context, q changeOrderReadQuery, runner mcpToolRunner) (changeOrderReadProjection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return changeOrderReadProjection{}, err
	}
	q, err := normalizeChangeOrderReadQuery(q)
	if err != nil {
		return changeOrderReadProjection{}, err
	}
	if runner == nil {
		return changeOrderReadProjection{}, fmt.Errorf("ChangeOrder read runner is unavailable")
	}
	raw, err := runner(ctx, changeOrderGetArgs(q))
	if err != nil {
		return changeOrderReadProjection{}, fmt.Errorf("ChangeOrder read unavailable: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return changeOrderReadProjection{}, err
	}
	return projectChangeOrderRead(raw, q)
}

var runChangeOrderCubCommand mcpToolRunner = runMCPConnectedToolCommand
var requireChangeOrderConnected = func() error { return requireConfigHubFor("history changeorder") }

func init() {
	command := &cobra.Command{Use: "changeorder <slug-or-id>", Short: "Read exact-space ChangeOrder Stage/State and workflow declarations", Args: cobra.ExactArgs(1), RunE: runHistoryChangeOrder}
	command.Flags().String("space", "", "Required exact ConfigHub space slug or ID; no environment fallback or wildcard")
	command.Flags().String("format", "ascii", "Output format: ascii, json, md")
	command.Flags().Bool("tui", false, "Open the same read projection as a scrollable snapshot")
	historyCmd.AddCommand(command)
}

func runHistoryChangeOrder(cmd *cobra.Command, args []string) error {
	space, _ := cmd.Flags().GetString("space")
	format, _ := cmd.Flags().GetString("format")
	format = strings.ToLower(strings.TrimSpace(format))
	tui, _ := cmd.Flags().GetBool("tui")
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json, md)", format)
	}
	q, err := normalizeChangeOrderReadQuery(changeOrderReadQuery{Order: args[0], Space: space})
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := requireChangeOrderConnected(); err != nil {
		return err
	}
	result, err := readChangeOrderProjection(ctx, q, runChangeOrderCubCommand)
	if err != nil {
		return err
	}
	if tui {
		return runChangeOrderTUI(result)
	}
	if format == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	fmt.Print(renderChangeOrderRead(result, format))
	return nil
}

func changeOrderReportedValue(value *string) string {
	if value == nil || *value == "" {
		return "unavailable (not inferred)"
	}
	return strconv.Quote(*value)
}
func renderChangeOrderRead(result changeOrderReadProjection, format string) string {
	var b strings.Builder
	if format == "md" {
		fmt.Fprintf(&b, "## ChangeOrder read\n\n- Order: %s (%s)\n- Space: %s (%s)\n- Reported Stage: %s\n- Reported State: %s\n", gitOpsMarkdownCodeSpan(result.Slug), gitOpsMarkdownCodeSpan(result.ChangeOrderID), gitOpsMarkdownCodeSpan(result.SpaceSlug), gitOpsMarkdownCodeSpan(result.SpaceID), gitOpsMarkdownCodeSpan(changeOrderReportedValue(result.ReportedStage)), gitOpsMarkdownCodeSpan(changeOrderReportedValue(result.ReportedState)))
	} else {
		fmt.Fprintf(&b, "ChangeOrder %s (%s)\nSpace: %s (%s)\nReported Stage: %s\nReported State: %s\n", result.Slug, result.ChangeOrderID, result.SpaceSlug, result.SpaceID, changeOrderReportedValue(result.ReportedStage), changeOrderReportedValue(result.ReportedState))
	}
	fmt.Fprintf(&b, "\nDeclaration coverage: %s\nEvaluation: %s\n", result.DeclarationCoverage, result.Evaluation)
	if result.WorkflowID != nil {
		fmt.Fprintf(&b, "Workflow ID: %s\n", *result.WorkflowID)
	}
	if result.WorkflowDeclaration != nil {
		raw, _ := json.MarshalIndent(result.WorkflowDeclaration, "", "  ")
		fmt.Fprintf(&b, "\nStored workflow declaration (not evaluated):\n%s\n", raw)
	}
	for _, omission := range result.Omissions {
		fmt.Fprintf(&b, "Omission %s: %s\n", omission.Missing, omission.Reason)
	}
	for _, limit := range result.Limitations {
		fmt.Fprintf(&b, "Limitation: %s\n", limit)
	}
	return b.String()
}

type changeOrderReadViewer struct{ recordedExplainViewer }

func newChangeOrderReadViewer(result changeOrderReadProjection) changeOrderReadViewer {
	content := safeGitOpsTUIContent(renderChangeOrderRead(result, "ascii"))
	vp := viewport.New(80, 20)
	vp.SetContent(wrapRecordedExplainText(content, 76))
	return changeOrderReadViewer{recordedExplainViewer{content: content, viewport: vp}}
}
func (m changeOrderReadViewer) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.recordedExplainViewer.Update(msg)
	m.recordedExplainViewer = model.(recordedExplainViewer)
	return m, cmd
}
func (m changeOrderReadViewer) View() string {
	return "ChangeOrder read snapshot · q to quit · arrows/page keys to scroll\n" + m.viewport.View()
}

var runChangeOrderTUI = func(result changeOrderReadProjection) error {
	_, err := tea.NewProgram(newChangeOrderReadViewer(result)).Run()
	return err
}

func changeOrderMCPTool(connectedRunner mcpToolRunner) mcpTool {
	return mcpTool{Descriptor: mcpToolDescriptor{Name: "confighub_changeorder_get", Description: "Connected-only exact-space ChangeOrder read (cub changeorder get -o json). Use after the order and exact space are known to inspect reported Stage/State and stored workflow/prerequisite declarations. Evaluation remains unknown, including Completed; declarations or missing fields do not establish approval, ungoverned state, health, publish eligibility or automatic advancement. DO NOT use as a prerequisite evaluator, controller observer or promotion command.", Annotations: &mcpToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{"changeorder": map[string]interface{}{"type": "string", "description": "Required exact order slug or ID; qualified slugs must match space."}, "space": map[string]interface{}{"type": "string", "description": "Required exact ConfigHub space slug or ID; wildcard and environment fallback are refused."}}, "required": []string{"changeorder", "space"}, "additionalProperties": false}}, BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
		for key := range arguments {
			if key != "changeorder" && key != "space" {
				return nil, fmt.Errorf("unsupported ChangeOrder argument %q", key)
			}
		}
		order, ok := arguments["changeorder"].(string)
		if !ok {
			return nil, fmt.Errorf("changeorder must be a string")
		}
		space, ok := arguments["space"].(string)
		if !ok {
			return nil, fmt.Errorf("space must be a string")
		}
		q, err := normalizeChangeOrderReadQuery(changeOrderReadQuery{Order: order, Space: space})
		if err != nil {
			return nil, err
		}
		return changeOrderGetArgs(q), nil
	}, Runner: func(ctx context.Context, args []string) (string, error) {
		if len(args) != 7 || args[0] != "changeorder" || args[1] != "get" || args[3] != "-o" || args[4] != "json" || args[5] != "--space" {
			return "", fmt.Errorf("invalid exact ChangeOrder read command")
		}
		result, err := readChangeOrderProjection(ctx, changeOrderReadQuery{Order: args[2], Space: args[6]}, connectedRunner)
		if err != nil {
			// Failed cub stdout has not passed identity/schema validation.
			// Do not trigger the gateway's retained Scout-JSON error path.
			return "", fmt.Errorf("%v", err)
		}
		raw, err := json.MarshalIndent(result, "", "  ")
		return string(raw), err
	}}
}
