package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/scan"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
)

// The MCP scan tool carries the CLI's answer unchanged, in its text and in
// structuredContent.
func TestScanAnswerIsTheSameThroughMCP(t *testing.T) {
	result := &scan.CombinedResult{KubernetesContext: "prod-east", State: &agent.StateScanResult{Findings: []agent.StuckFinding{{Kind: "HelmRelease", Name: "redis", Namespace: "data", Severity: "warning"}}}}
	cli := captureStdout(t, func() {
		require.NoError(t, outputCombinedJSONWithAnswer(result, buildScanAnswer(result, "data", nil, time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC))))
	})
	var cliDoc struct {
		Answer json.RawMessage `json:"answer"`
	}
	require.NoError(t, json.Unmarshal([]byte(cli), &cliDoc))
	require.NotEmpty(t, cliDoc.Answer)

	runner := func(context.Context, []string) (string, error) { return strings.TrimSpace(cli), nil }
	gateway := newMCPGatewayWithMode(runner, runner, false)
	tool, ok := gateway.tools["scan"]
	require.True(t, ok)
	reply := gateway.runNamedTool(context.Background(), tool.Descriptor.Name, tool, map[string]interface{}{"namespace": "data"})
	require.Equal(t, false, reply["isError"], "reply: %v", reply)
	content := reply["content"].([]map[string]string)
	var mcpDoc struct {
		Answer json.RawMessage `json:"answer"`
	}
	require.NoError(t, json.Unmarshal([]byte(content[0]["text"]), &mcpDoc))
	require.JSONEq(t, string(cliDoc.Answer), string(mcpDoc.Answer), "MCP text must carry the CLI's answer unchanged")

	if structured, ok := reply["structuredContent"]; ok {
		raw, err := json.Marshal(structured)
		require.NoError(t, err)
		var wrapped struct {
			Data struct {
				Answer json.RawMessage `json:"answer"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(raw, &wrapped))
		require.JSONEq(t, string(cliDoc.Answer), string(wrapped.Data.Answer), "structuredContent must carry the CLI's answer unchanged")
	}
}
