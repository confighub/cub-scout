package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

const coOrderID = "11111111-1111-4111-8111-111111111111"
const coSpaceID = "22222222-2222-4222-8222-222222222222"
const coWorkflowID = "33333333-3333-4333-8333-333333333333"
const coDeclaration = `{"Stages":[{"Name":"Review","Prerequisites":["reviewer","tests"],"ReleasePrerequisites":["publish"],"WhereSpace":"Space.Slug == 'prod'"},{"Name":"Release","Prerequisites":[]}],"Final":{"Prerequisites":["audit"]},"CustomPrerequisites":[{"Name":"tests","Expression":"true","Description":"Declared expression only"}],"AttestationPrerequisites":[{"Name":"reviewer","Type":"review","Count":0,"AllowAuthors":false,"IgnoreFail":false,"DistinctGroups":false,"FromUserIDs":[],"FromGroupIDs":[],"MaxAge":"1h"}]}`

func coFixture() string {
	return `{"ChangeOrder":{"ChangeOrderID":"` + coOrderID + `","Slug":"rollout","SpaceID":"` + coSpaceID + `","SpaceSlug":"prod","Stage":"Completed","State":"Ready","ChangeWorkflowID":"` + coWorkflowID + `","ChangeWorkflow":` + coDeclaration + `},"Space":{"SpaceID":"` + coSpaceID + `","Slug":"prod"}}`
}
func coQuery() changeOrderReadQuery { return changeOrderReadQuery{Order: "rollout", Space: "prod"} }

func TestChangeOrderExactReadAndUnknownEvaluation(t *testing.T) {
	calls := 0
	result, err := readChangeOrderProjection(context.Background(), coQuery(), func(_ context.Context, args []string) (string, error) {
		calls++
		if !reflect.DeepEqual(args, []string{"changeorder", "get", "rollout", "-o", "json", "--space", "prod"}) {
			t.Fatalf("args: %v", args)
		}
		return coFixture(), nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("read: %v calls=%d", err, calls)
	}
	if result.Evaluation != "unknown" || result.DeclarationCoverage != "reported" || *result.ReportedStage != "Completed" || *result.ReportedState != "Ready" {
		t.Fatalf("projection: %+v", result)
	}
	if string(result.WorkflowDeclaration) != coDeclaration {
		t.Fatalf("declaration presence/order changed: %s", result.WorkflowDeclaration)
	}
	for _, format := range []string{"ascii", "md"} {
		out := renderChangeOrderRead(result, format)
		for _, want := range []string{coOrderID, coSpaceID, coWorkflowID, "rollout", "prod", "Completed", "Ready", "Evaluation: unknown", "not evaluated", "runtime health"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s lacks %q", format, want)
			}
		}
	}
	for _, q := range []changeOrderReadQuery{{Order: coOrderID, Space: coSpaceID}, {Order: "prod/rollout", Space: "prod"}} {
		if _, err := projectChangeOrderRead(coFixture(), q); err != nil {
			t.Fatal(err)
		}
	}
	// Flat SDK entity JSON is also accepted, without weakening scope verification.
	var wrapped map[string]json.RawMessage
	_ = json.Unmarshal([]byte(coFixture()), &wrapped)
	if _, err := projectChangeOrderRead(string(wrapped["ChangeOrder"]), coQuery()); err != nil {
		t.Fatal(err)
	}
}
func TestChangeOrderRejectSelectorsBeforeRead(t *testing.T) {
	t.Setenv("CUB_SPACE", "prod")
	for _, q := range []changeOrderReadQuery{{Order: "rollout"}, {Order: "*", Space: "prod"}, {Order: "--all", Space: "prod"}, {Order: "rollout", Space: "*"}, {Order: "rollout", Space: "--space"}, {Order: "other/rollout", Space: "prod"}, {Order: "prod/a/b", Space: "prod"}, {Order: "rollout", Space: " prod"}, {Order: "rollout", Space: "prod/other"}} {
		_, err := readChangeOrderProjection(context.Background(), q, func(context.Context, []string) (string, error) { t.Fatal("invalid query executed"); return "", nil })
		if err == nil {
			t.Fatalf("accepted %+v", q)
		}
	}
}
func TestChangeOrderRefusesUncertainIdentityAndMalformedEvidence(t *testing.T) {
	cases := map[string]string{
		"other space same slug":      strings.ReplaceAll(coFixture(), coSpaceID, "44444444-4444-4444-8444-444444444444"),
		"other space slug":           strings.ReplaceAll(coFixture(), `"prod"`, `"other"`),
		"order mismatch":             strings.Replace(coFixture(), `"rollout"`, `"other"`, 1),
		"missing ID":                 strings.Replace(coFixture(), `"ChangeOrderID":"`+coOrderID+`",`, "", 1),
		"bad ID":                     strings.Replace(coFixture(), coOrderID, "invalid", 1),
		"related space mismatch":     strings.Replace(coFixture(), `"Space":{"SpaceID":"`+coSpaceID, `"Space":{"SpaceID":"44444444-4444-4444-8444-444444444444`, 1),
		"stage type":                 strings.Replace(coFixture(), `"Stage":"Completed"`, `"Stage":true`, 1),
		"state type":                 strings.Replace(coFixture(), `"State":"Ready"`, `"State":[]`, 1),
		"workflow type":              strings.Replace(coFixture(), coDeclaration, `[]`, 1),
		"prerequisite type":          strings.Replace(coFixture(), `"Prerequisites":["reviewer","tests"]`, `"Prerequisites":[true]`, 1),
		"unknown workflow schema":    strings.Replace(coFixture(), `"Stages":`, `"Evaluated":true,"Stages":`, 1),
		"case alias":                 strings.Replace(coFixture(), `"Stage":`, `"stage":`, 1),
		"workflow case alias":        strings.Replace(coFixture(), `"Stages":`, `"stages":`, 1),
		"duplicate identity":         strings.Replace(coFixture(), `"Slug":"rollout"`, `"Slug":"rollout","Slug":"rollout"`, 1),
		"duplicate nested":           strings.Replace(coFixture(), `"Count":0`, `"Count":0,"Count":1`, 1),
		"duplicate ignored metadata": strings.Replace(coFixture(), `"Slug":"rollout"`, `"Slug":"rollout","Meta":{"x":1,"x":2}`, 1),
		"response error":             strings.TrimSuffix(coFixture(), "}") + `,"Error":{"Message":"denied"}}`,
		"multiple JSON":              coFixture() + ` {}`,
		"invalid UTF8":               strings.Replace(coFixture(), "Ready", string([]byte{0xff}), 1),
		"invalid JSON":               `{"ChangeOrder":`,
		"non object":                 `[]`,
		"null":                       `null`,
		"oversize":                   strings.Repeat(" ", maxChangeOrderReadBytes+1),
		"deep":                       strings.Repeat("[", 66) + "0" + strings.Repeat("]", 66),
	}
	// Same slug in another space is valid only when that exact space is requested.
	other := strings.ReplaceAll(strings.ReplaceAll(coFixture(), coSpaceID, "44444444-4444-4444-8444-444444444444"), `"prod"`, `"other"`)
	if _, err := projectChangeOrderRead(other, changeOrderReadQuery{Order: "rollout", Space: "other"}); err != nil {
		t.Fatal(err)
	}
	delete(cases, "other space same slug") // A different valid ID with the same proven slug is not a conflict.
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := projectChangeOrderRead(raw, coQuery()); err == nil {
				t.Fatal("accepted uncertain/malformed evidence")
			}
		})
	}
	if _, err := projectChangeOrderRead(other, changeOrderReadQuery{Order: coOrderID, Space: coSpaceID}); err == nil {
		t.Fatal("accepted wrong exact space ID")
	}
}
func TestChangeOrderMissingEvidenceRemainsUnknown(t *testing.T) {
	base := `{"ChangeOrderID":"` + coOrderID + `","Slug":"rollout","SpaceID":"` + coSpaceID + `","SpaceSlug":"prod"`
	for _, test := range []struct{ name, extra, coverage string }{
		{"missing", "", "unavailable"}, {"null", `,"Stage":null,"State":null,"ChangeWorkflow":null`, "unavailable"},
		{"partial", `,"Stage":"Completed","ChangeWorkflow":{"Stages":[{"Name":""}],"CustomPrerequisites":[{"Name":"check"}],"AttestationPrerequisites":[{}]}`, "partial"},
		{"empty", `,"ChangeWorkflowID":"` + coWorkflowID + `","ChangeWorkflow":{"Stages":[]}`, "partial"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := projectChangeOrderRead(base+test.extra+"}", coQuery())
			if err != nil {
				t.Fatal(err)
			}
			if result.Evaluation != "unknown" || result.DeclarationCoverage != test.coverage || len(result.Omissions) == 0 {
				t.Fatalf("result: %+v", result)
			}
			if !strings.Contains(renderChangeOrderRead(result, "ascii"), "no") && test.coverage == "partial" {
				t.Fatal("missing limitation")
			}
		})
	}
}
func TestChangeOrderCancellationAndReadErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readChangeOrderProjection(ctx, coQuery(), func(context.Context, []string) (string, error) { t.Fatal("cancelled read executed"); return "", nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	_, err = readChangeOrderProjection(ctx, coQuery(), func(context.Context, []string) (string, error) { cancel(); return coFixture(), nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	denied := errors.New("permission denied")
	result, err := readChangeOrderProjection(context.Background(), coQuery(), func(context.Context, []string) (string, error) { return coFixture(), denied })
	if !errors.Is(err, denied) || result.Schema != "" {
		t.Fatalf("failed read used output: %+v %v", result, err)
	}
}
func coCommand() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("space", "prod", "")
	cmd.Flags().String("format", "ascii", "")
	cmd.Flags().Bool("tui", false, "")
	return cmd
}
func TestChangeOrderCLIAndMCPShareProjection(t *testing.T) {
	oldRun, oldGate, oldTUI := runChangeOrderCubCommand, requireChangeOrderConnected, runChangeOrderTUI
	t.Cleanup(func() {
		runChangeOrderCubCommand, requireChangeOrderConnected, runChangeOrderTUI = oldRun, oldGate, oldTUI
	})
	calls := 0
	runChangeOrderCubCommand = func(_ context.Context, args []string) (string, error) {
		calls++
		if !reflect.DeepEqual(args, changeOrderGetArgs(coQuery())) {
			t.Fatalf("args: %v", args)
		}
		return coFixture(), nil
	}
	requireChangeOrderConnected = func() error { return nil }
	expected, err := projectChangeOrderRead(coFixture(), coQuery())
	if err != nil {
		t.Fatal(err)
	}
	cmd := coCommand()
	_ = cmd.Flags().Set("format", "json")
	output := captureStdout(t, func() {
		if err := runHistoryChangeOrder(cmd, []string{"rollout"}); err != nil {
			t.Fatal(err)
		}
	})
	var actual changeOrderReadProjection
	if err := json.Unmarshal([]byte(output), &actual); err != nil {
		t.Fatal(err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, actual.WorkflowDeclaration); err != nil {
		t.Fatal(err)
	}
	actual.WorkflowDeclaration = compact.Bytes()
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("CLI drift: %s", output)
	}
	for _, format := range []string{"ascii", "md"} {
		_ = cmd.Flags().Set("format", format)
		got := captureStdout(t, func() {
			if err := runHistoryChangeOrder(cmd, []string{"rollout"}); err != nil {
				t.Fatal(err)
			}
		})
		if got != renderChangeOrderRead(expected, format) {
			t.Fatal("render drift")
		}
	}
	runChangeOrderTUI = func(got changeOrderReadProjection) error {
		if !reflect.DeepEqual(got, expected) {
			t.Fatal("TUI drift")
		}
		return nil
	}
	_ = cmd.Flags().Set("tui", "true")
	if err := runHistoryChangeOrder(cmd, []string{"rollout"}); err != nil {
		t.Fatal(err)
	}
	gateway := newMCPGatewayWithMode(func(context.Context, []string) (string, error) { t.Fatal("Scout runner called"); return "", nil }, runChangeOrderCubCommand, true)
	tool, ok := gateway.tools["confighub_changeorder_get"]
	if !ok {
		t.Fatal("missing connected tool")
	}
	args, err := tool.BuildArgs(map[string]interface{}{"changeorder": "rollout", "space": "prod"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := tool.Runner(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.MarshalIndent(expected, "", "  ")
	if got != string(want) {
		t.Fatalf("MCP drift: %s", got)
	}
	if calls != 5 {
		t.Fatalf("expected one read per projection, calls=%d", calls)
	}
	if _, ok := newMCPGatewayWithMode(nil, nil, false).tools["confighub_changeorder_get"]; ok {
		t.Fatal("standalone exposes connected read")
	}
	for _, input := range []map[string]interface{}{{"changeorder": "rollout"}, {"changeorder": true, "space": "prod"}, {"changeorder": "rollout", "space": "*"}, {"changeorder": "other/rollout", "space": "prod"}, {"changeorder": "rollout", "space": "prod", "extra": true}} {
		if _, err := tool.BuildArgs(input); err == nil {
			t.Fatalf("accepted %v", input)
		}
	}
	if _, err := tool.Runner(context.Background(), []string{"changeorder", "approve"}); err == nil {
		t.Fatal("accepted mutation")
	}
}
func TestChangeOrderCLIRefusesWithoutConnectedAndPreservesLegacy(t *testing.T) {
	oldGate, oldRun := requireChangeOrderConnected, runChangeOrderCubCommand
	t.Cleanup(func() { requireChangeOrderConnected, runChangeOrderCubCommand = oldGate, oldRun })
	runChangeOrderCubCommand = func(context.Context, []string) (string, error) { t.Fatal("unexpected read"); return "", nil }
	gateCalls := 0
	requireChangeOrderConnected = func() error { gateCalls++; return errors.New("connected mode required") }
	cmd := coCommand()
	_ = cmd.Flags().Set("space", "")
	if err := runHistoryChangeOrder(cmd, []string{"rollout"}); err == nil || gateCalls != 0 {
		t.Fatal("omitted space used gate/default")
	}
	_ = cmd.Flags().Set("space", "prod")
	if err := runHistoryChangeOrder(cmd, []string{"rollout"}); err == nil || gateCalls != 1 {
		t.Fatal("standalone was not refused")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	if err := runHistoryChangeOrder(cmd, []string{"rollout"}); !errors.Is(err, context.Canceled) || gateCalls != 1 {
		t.Fatal("cancelled CLI used gate")
	}
	var child *cobra.Command
	for _, c := range historyCmd.Commands() {
		if c.Name() == "changeorder" {
			child = c
		}
	}
	if child == nil || historyCmd.RunE == nil || historyCmd.Flags().Lookup("since") == nil || historyCmd.Flags().Lookup("include-synthetic") == nil {
		t.Fatal("legacy history registration changed")
	}
	if child.Flags().Lookup("space").DefValue != "" || child.Flags().Lookup("format").DefValue != "ascii" {
		t.Fatal("unsafe child defaults")
	}
}
func TestChangeOrderTUIIsStaticSanitizedSnapshot(t *testing.T) {
	result, err := projectChangeOrderRead(strings.Replace(coFixture(), `"Completed"`, `"Completed\u001b]52;c;BAD\u0007"`, 1), coQuery())
	if err != nil {
		t.Fatal(err)
	}
	viewer := newChangeOrderReadViewer(result)
	if viewer.Init() != nil {
		t.Fatal("snapshot triggered read")
	}
	if strings.ContainsAny(viewer.content, "\x1b\a") {
		t.Fatal("terminal controls survived")
	}
	model, cmd := viewer.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	if cmd != nil {
		t.Fatal("resize triggered work")
	}
	resized := model.(changeOrderReadViewer)
	if resized.content != viewer.content {
		t.Fatal("resize changed snapshot")
	}
	_, cmd = resized.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("quit unavailable")
	}
}

func TestChangeOrderMCPDispatchErrorsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       string
		readErr   error
		cancelled bool
		wantError bool
	}{
		{name: "success", raw: coFixture()},
		{name: "denied with misleading stdout", raw: coFixture(), readErr: errors.New("permission denied"), wantError: true},
		{name: "command error with retained JSON", raw: coFixture(), readErr: &mcpCommandError{args: changeOrderGetArgs(coQuery()), stdout: coFixture(), stderr: "permission denied", err: errors.New("exit 1")}, wantError: true},
		{name: "wrong space", raw: strings.ReplaceAll(coFixture(), `"prod"`, `"other"`), wantError: true},
		{name: "cancelled", cancelled: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			gateway := newMCPGatewayWithMode(func(context.Context, []string) (string, error) { t.Fatal("Scout runner used"); return "", nil }, func(context.Context, []string) (string, error) { calls++; return tc.raw, tc.readErr }, true)
			ctx := context.Background()
			if tc.cancelled {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			response := gateway.handleRequest(ctx, mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: json.RawMessage(`{"name":"confighub_changeorder_get","arguments":{"changeorder":"rollout","space":"prod"}}`)})
			if response == nil || response.Error != nil {
				t.Fatalf("response: %+v", response)
			}
			raw, err := json.Marshal(response.Result)
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			}
			if err = json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if result.IsError != tc.wantError || len(result.Content) == 0 {
				t.Fatalf("result: %+v", result)
			}
			if tc.wantError {
				if strings.Contains(result.Content[0].Text, `"schema"`) || strings.Contains(result.Content[0].Text, `"ChangeOrder"`) {
					t.Fatal("failed read exposed success projection")
				}
			} else {
				var projection changeOrderReadProjection
				if err = json.Unmarshal([]byte(result.Content[0].Text), &projection); err != nil {
					t.Fatal(err)
				}
				if projection.Evaluation != "unknown" || *projection.ReportedStage != "Completed" {
					t.Fatalf("projection: %+v", projection)
				}
			}
			wantCalls := 1
			if tc.cancelled {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("calls=%d want=%d", calls, wantCalls)
			}
		})
	}
}
