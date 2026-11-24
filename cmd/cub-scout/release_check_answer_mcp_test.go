package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v3/pkg/agent"
	"github.com/stretchr/testify/require"
)

// The MCP release_check tool carries the CLI's answer unchanged, in its text
// and in structuredContent.
func TestReleaseCheckAnswerIsTheSameThroughMCP(t *testing.T) {
	start := time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC)
	report := agent.NewReleaseCheckReport(agent.BoundedResourceRef{Kind: "Kustomization", Name: "apps", Namespace: "delivery"}, "cluster-a", "cluster-a", 10, start)
	report.Stages[1].Verdict = "WATCH"
	report.Finish(start.Add(time.Second))
	data, err := json.Marshal(releaseCheckOutput{ReleaseCheckReport: report, Answer: buildReleaseCheckAnswer(report)})
	require.NoError(t, err)
	cli := string(data)
	var cliDoc struct {
		Answer json.RawMessage `json:"answer"`
	}
	require.NoError(t, json.Unmarshal(data, &cliDoc))
	require.NotEmpty(t, cliDoc.Answer)

	runner := func(context.Context, []string) (string, error) { return strings.TrimSpace(cli), nil }
	gateway := newMCPGatewayWithMode(runner, runner, false)
	tool, ok := gateway.tools["release_check"]
	require.True(t, ok)
	reply := gateway.runNamedTool(context.Background(), tool.Descriptor.Name, tool, map[string]interface{}{
		"bundle": "oci://registry.example.test/app@sha256:" + strings.Repeat("a", 64), "controller": "Kustomization/apps",
		"api_version": "kustomize.toolkit.fluxcd.io/v1", "controller_namespace": "delivery", "context": "cluster-a",
	})
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
