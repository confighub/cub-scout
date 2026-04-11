package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNewMCPGateway_ToolsIncludeStandaloneSet(t *testing.T) {
	gateway := newMCPGateway(nil)
	tools := gateway.toolsForList()

	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name)
	}

	want := []string{"doctor", "explain", "map", "scan", "trace"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
}

func TestNewMCPGatewayWithMode_ConnectedAddsConfigHubTools(t *testing.T) {
	gateway := newMCPGatewayWithMode(nil, nil, true)
	tools := gateway.toolsForList()

	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name)
	}

	want := []string{
		"confighub_changesets",
		"confighub_unit_get",
		"confighub_units",
		"doctor",
		"explain",
		"map",
		"scan",
		"trace",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
}

func TestMCPGatewayHandleRequest_ToolsList(t *testing.T) {
	gateway := newMCPGateway(nil)
	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	var result struct {
		Tools []mcpToolDescriptor `json:"tools"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.Tools) != 5 {
		t.Fatalf("tool count = %d, want 5", len(result.Tools))
	}
}

func TestNewMCPGateway_ToolDescriptionsExpressChainBoundaries(t *testing.T) {
	gateway := newMCPGatewayWithMode(nil, nil, true)
	tools := gateway.tools

	cases := []struct {
		name     string
		contains []string
	}{
		{name: "doctor", contains: []string{"FIRST standalone tool", "Use before explain, trace, or scan"}},
		{name: "map", contains: []string{"what's running in this cluster", "use doctor first"}},
		{name: "scan", contains: []string{"Use AFTER doctor", "DO NOT load first"}},
		{name: "explain", contains: []string{"Use AFTER doctor or map", "DO NOT load for broad cluster inventory or health"}},
		{name: "trace", contains: []string{"Use AFTER explain", "DO NOT load for broad cluster status"}},
		{name: "confighub_changesets", contains: []string{"Connected-only", "DO NOT load for current cluster health or ownership"}},
		{name: "confighub_units", contains: []string{"Connected-only", "use map or doctor first"}},
		{name: "confighub_unit_get", contains: []string{"Load ONLY after", "use confighub_units first"}},
	}

	for _, tc := range cases {
		tool, ok := tools[tc.name]
		if !ok {
			t.Fatalf("tool %q not found", tc.name)
		}
		desc := tool.Descriptor.Description
		for _, want := range tc.contains {
			if !strings.Contains(desc, want) {
				t.Errorf("%s description missing %q\nfull description: %s", tc.name, want, desc)
			}
		}
	}
}

func TestMCPGatewayHandleRequest_ToolsCallTrace(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"ok":true}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`7`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"trace",
			"arguments":{"resource":"deployment/api","namespace":"payments"}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"trace", "deployment/api", "-n", "payments", "--format", "json"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.IsError {
		t.Fatal("result.isError = true, want false")
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("content = %+v, want single text item", result.Content)
	}
	if result.Content[0].Text != `{"ok":true}` {
		t.Fatalf("content text = %q, want %q", result.Content[0].Text, `{"ok":true}`)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallValidationError(t *testing.T) {
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		t.Fatal("runner should not be called for validation error")
		return "", nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`9`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"trace",
			"arguments":{"namespace":"payments"}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "resource") {
		t.Fatalf("unexpected error text: %+v", result.Content)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConnectedChangesets(t *testing.T) {
	var gotStandaloneArgs []string
	var gotConnectedArgs []string

	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			gotStandaloneArgs = append([]string(nil), args...)
			return `{"standalone":true}`, nil
		},
		func(ctx context.Context, args []string) (string, error) {
			gotConnectedArgs = append([]string(nil), args...)
			return `{"connected":true}`, nil
		},
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`11`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"confighub_changesets",
			"arguments":{"space":"platform","where":"Slug LIKE 'release-%'"}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	if len(gotStandaloneArgs) != 0 {
		t.Fatalf("standalone runner should not be used, got args %v", gotStandaloneArgs)
	}
	wantConnected := []string{"changeset", "list", "--json", "--space", "platform", "--where", "Slug LIKE 'release-%'"}
	if !reflect.DeepEqual(gotConnectedArgs, wantConnected) {
		t.Fatalf("connected args = %v, want %v", gotConnectedArgs, wantConnected)
	}
}

func TestMCPFrameRoundTrip(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	var stream bytes.Buffer
	if err := writeMCPFrame(&stream, payload); err != nil {
		t.Fatalf("writeMCPFrame() error = %v", err)
	}

	got, err := readMCPFrame(bufio.NewReader(&stream))
	if err != nil {
		t.Fatalf("readMCPFrame() error = %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch\ngot=%s\nwant=%s", string(got), string(payload))
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctor(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"all"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`12`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"doctor", "--format", "json"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithNamespace(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"prod"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`13`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"namespace":"prod"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"doctor", "--format", "json", "-n", "prod"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithTop(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"all"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`14`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":5}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"doctor", "--format", "json", "--top", "5"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithAllParams(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"staging"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`15`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"namespace":"staging","top":10}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"doctor", "--format", "json", "-n", "staging", "--top", "10"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithTopZero(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"all"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`16`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":0}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	// top=0 should be passed through as --top 0 (valid CLI case)
	wantArgs := []string{"doctor", "--format", "json", "--top", "0"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithNegativeTop(t *testing.T) {
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		t.Fatal("runner should not be called for validation error")
		return "", nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`17`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":-1}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "non-negative") {
		t.Fatalf("error should mention non-negative, got: %+v", result.Content)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithStringTop(t *testing.T) {
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		t.Fatal("runner should not be called for validation error")
		return "", nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`18`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":"5"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "integer") {
		t.Fatalf("error should mention integer, got: %+v", result.Content)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithFractionalTop(t *testing.T) {
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		t.Fatal("runner should not be called for validation error")
		return "", nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`19`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":1.5}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "whole number") {
		t.Fatalf("error should mention whole number, got: %+v", result.Content)
	}
}

func TestArgInt(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
		key  string
		want int
	}{
		{"float64", map[string]interface{}{"top": float64(5)}, "top", 5},
		{"int", map[string]interface{}{"top": 3}, "top", 3},
		{"missing", map[string]interface{}{}, "top", 0},
		{"nil", map[string]interface{}{"top": nil}, "top", 0},
		{"string", map[string]interface{}{"top": "5"}, "top", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := argInt(tc.args, tc.key)
			if got != tc.want {
				t.Errorf("argInt() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestArgIntOpt(t *testing.T) {
	tests := []struct {
		name          string
		args          map[string]interface{}
		key           string
		allowNegative bool
		wantVal       int
		wantPresent   bool
		wantErr       bool
		errContains   string
	}{
		{"float64", map[string]interface{}{"top": float64(5)}, "top", false, 5, true, false, ""},
		{"int", map[string]interface{}{"top": 3}, "top", false, 3, true, false, ""},
		{"zero", map[string]interface{}{"top": float64(0)}, "top", false, 0, true, false, ""},
		{"missing", map[string]interface{}{}, "top", false, 0, false, false, ""},
		{"nil", map[string]interface{}{"top": nil}, "top", false, 0, false, false, ""},
		{"string", map[string]interface{}{"top": "5"}, "top", false, 0, true, true, "integer"},
		{"fractional", map[string]interface{}{"top": float64(1.5)}, "top", false, 0, true, true, "whole number"},
		{"negative_rejected", map[string]interface{}{"top": float64(-1)}, "top", false, 0, true, true, "non-negative"},
		{"negative_allowed", map[string]interface{}{"top": float64(-1)}, "top", true, -1, true, false, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotVal, gotPresent, gotErr := argIntOpt(tc.args, tc.key, tc.allowNegative)
			if gotPresent != tc.wantPresent {
				t.Errorf("argIntOpt() present = %v, want %v", gotPresent, tc.wantPresent)
			}
			if tc.wantErr {
				if gotErr == nil {
					t.Errorf("argIntOpt() error = nil, want error containing %q", tc.errContains)
				} else if !strings.Contains(gotErr.Error(), tc.errContains) {
					t.Errorf("argIntOpt() error = %q, want containing %q", gotErr.Error(), tc.errContains)
				}
			} else {
				if gotErr != nil {
					t.Errorf("argIntOpt() error = %v, want nil", gotErr)
				}
				if gotVal != tc.wantVal {
					t.Errorf("argIntOpt() = %d, want %d", gotVal, tc.wantVal)
				}
			}
		})
	}
}

func marshalInto(v interface{}, target interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
