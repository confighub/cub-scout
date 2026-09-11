package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestMCPMapNamespaceUsesSupportedCLIFlag(t *testing.T) {
	tool := newMCPGateway(nil).tools["map"]
	args, err := tool.BuildArgs(map[string]interface{}{"namespace": "team-a"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"map", "list", "--json", "--namespace", "team-a"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("namespace-scoped inventory args = %v, want %v", args, want)
	}
	if mapCmd.PersistentFlags().Lookup("namespace") == nil && mapListCmd.Flags().Lookup("namespace") == nil {
		t.Fatal("namespace flag no longer exists on map list")
	}
}

func TestMCPResourceReadBudget(t *testing.T) {
	for _, tool := range []string{"confighub_k8s_resources", "confighub_k8s_types", "confighub_resources"} {
		t.Run(tool, func(t *testing.T) {
			calls := 0
			gateway := newMCPGatewayWithMode(func(context.Context, []string) (string, error) {
				t.Fatal("stored resource query must not call the live cluster runner")
				return "", nil
			}, func(_ context.Context, args []string) (string, error) {
				calls++
				if !strings.Contains(strings.Join(args, " "), "--space team-a") {
					t.Fatalf("lost bounded scope: %v", args)
				}
				return `[{"Resource":{"ResourceName":"api"}}]`, nil
			}, true)
			for n := 1; n <= 3; n++ {
				params, _ := json.Marshal(map[string]interface{}{
					"name": tool, "arguments": map[string]string{"space": "team-a", "type": "deploy"},
				})
				resp := gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params})
				if resp == nil || resp.Error != nil {
					t.Fatalf("response = %+v", resp)
				}
				if calls != n {
					t.Fatalf("calls = %d, want %d (one per request; no implicit cache)", calls, n)
				}
			}
			params, _ := json.Marshal(map[string]interface{}{"name": tool, "arguments": map[string]string{"type": "deploy"}})
			resp := gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`2`), Method: "tools/call", Params: params})
			var result struct {
				IsError bool `json:"isError"`
			}
			if err := marshalInto(resp.Result, &result); err != nil {
				t.Fatal(err)
			}
			if !result.IsError || calls != 3 {
				t.Fatalf("invalid scope: error=%v calls=%d", result.IsError, calls)
			}
		})
	}
}

func TestMCPConnectedRunnerContextIsolation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix subprocess fixture; Windows standalone covered separately")
	}
	binDir := t.TempDir()
	// Only synthetic context values are printed. No auth tokens are read by this fixture.
	script := "#!/bin/sh\nprintf '{\"context\":\"%s\",\"space\":\"%s\",\"kubeconfig\":\"%s\"}' \"$CUB_CONTEXT\" \"$CUB_SPACE\" \"$KUBECONFIG\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "cub"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	t.Setenv("KUBECONFIG", "/fixture/kube-context")
	for _, name := range []string{"context-a", "context-b"} {
		t.Setenv("CUB_CONTEXT", name)
		t.Setenv("CUB_SPACE", name+"-space")
		raw, err := runMCPConnectedToolCommand(context.Background(), []string{"resource", "list", "--space", name + "-space", "-o", "json"})
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]string
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatal(err)
		}
		if got["context"] != name || got["space"] != name+"-space" || got["kubeconfig"] != "/fixture/kube-context" {
			t.Fatalf("context leaked or changed: %v", got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runMCPConnectedToolCommand(ctx, []string{"resource", "list"}); err == nil {
		t.Fatal("cancelled request should fail")
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := runMCPConnectedToolCommand(context.Background(), []string{"resource", "list"}); err == nil {
		t.Fatal("missing cub must remain an error")
	}
}
