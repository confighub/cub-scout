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

	want := []string{"explain", "map", "scan", "trace"}
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
	if len(result.Tools) != 4 {
		t.Fatalf("tool count = %d, want 4", len(result.Tools))
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

func marshalInto(v interface{}, target interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
