package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The MCP gitops_settings tool carries the CLI's answer unchanged, in its
// text and in structuredContent.
func TestGitOpsSettingsAnswerIsTheSameThroughMCP(t *testing.T) {
	report := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{})
	report.Answer = buildDeliverySettingsAnswer(report, nil, time.Date(2026, 11, 19, 13, 0, 0, 0, time.UTC))
	var out bytes.Buffer
	require.NoError(t, writeDeliverySettingsView(&out, report, deliverySettingsViewSummary))
	cli := out.String()
	var cliDoc struct {
		Answer json.RawMessage `json:"answer"`
	}
	require.NoError(t, json.Unmarshal([]byte(cli), &cliDoc))
	require.NotEmpty(t, cliDoc.Answer, "the summary view keeps the answer")

	runner := func(context.Context, []string) (string, error) { return strings.TrimSpace(cli), nil }
	gateway := newMCPGatewayWithMode(runner, runner, false)
	tool, ok := gateway.tools["gitops_settings"]
	require.True(t, ok)
	reply := gateway.runNamedTool(context.Background(), tool.Descriptor.Name, tool, map[string]interface{}{})
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
