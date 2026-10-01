// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Exercise the gateway's actual Scout Cobra handler and HTTP reads, with an
// independent cub runner that must not receive the Scout command. Real stdio
// and child-process isolation have a separate integration capture.
func TestMCPSourceTruthUsesScoutCommandAndSelectedAPI(t *testing.T) {
	alpha := newSourceTruthHTTPFixture(t, "alpha")
	beta := newSourceTruthHTTPFixture(t, "beta")
	path := filepath.Join(t.TempDir(), "config")
	writeTraceKubeconfig(t, path, "beta-context", alpha.URL, beta.URL)
	t.Setenv("KUBECONFIG", path)
	stubConnectedGate(t, nil)
	oldUnitGet := sourceTruthUnitGet
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		return []byte(`{"HeadRevisionNum":3,"SpaceID":"space-id","UnitID":"unit-id"}`), nil
	}
	oldNamespace, oldStrategy, oldFormat, oldContext := sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat, sourceTruthContext
	flag := sourceTruthCmd.Flags().Lookup("kube-context")
	oldChanged := flag.Changed
	oldRootContext := rootCmd.Context()
	t.Cleanup(func() {
		sourceTruthUnitGet = oldUnitGet
		sourceTruthNamespace, sourceTruthStrategy, sourceTruthFormat, sourceTruthContext = oldNamespace, oldStrategy, oldFormat, oldContext
		flag.Changed = oldChanged
		rootCmd.SetArgs(nil)
		rootCmd.SetContext(oldRootContext)
	})
	var scoutArgs []string
	cubCalls := 0
	gateway := newMCPGatewayWithMode(mcpTraceParserRunner(t, &scoutArgs), func(context.Context, []string) (string, error) {
		cubCalls++
		return "", fmt.Errorf("Scout command must not be sent to cub")
	}, true)
	params, err := json.Marshal(map[string]interface{}{"name": "compare_source_truth", "arguments": map[string]interface{}{"target": "Deployment/api", "namespace": "team-a", "strategy": "git-argo", "context": "alpha-context"}})
	require.NoError(t, err)
	response := gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params})
	require.Nil(t, response.Error)
	require.Zero(t, cubCalls, "source-truth belongs to the Scout executable even though it is connected-only")
	require.NotEmpty(t, scoutArgs)
	encoded, err := json.Marshal(response.Result)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `alpha-context`)
	require.Contains(t, string(encoded), `"status":"PASS"`)
	require.NotEmpty(t, alpha.allRequests())
	require.Empty(t, beta.allRequests())
}
