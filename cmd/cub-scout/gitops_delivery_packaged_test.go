// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Opt-in release smoke: the provider is an external binary, not this test's
// implementation. Only the public startup reachability HEAD is external;
// credentials, connected data and CLI queries are isolated fixtures.
func TestLiveStatusFreshnessPackaged(t *testing.T) {
	binary := os.Getenv("CUB_SCOUT_TEST_BINARY")
	if binary == "" {
		t.Skip("set CUB_SCOUT_TEST_BINARY to a native release binary")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Unix plugin smoke; Windows archives do not contain a plugin")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	plugin := os.Getenv("CUB_SCOUT_TEST_PLUGIN_BINARY")
	if plugin == "" {
		plugin = filepath.Join(t.TempDir(), "main")
		if err := copyExecutable(binary, plugin); err != nil {
			t.Fatal(err)
		}
	}
	plugin, err = filepath.Abs(plugin)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	cases := []liveStatusFreshnessCase{
		{Name: "fresh", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339), Freshness: "fresh", Verdict: "PASS"},
		{Name: "fresh-failure", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339), Failed: true, Freshness: "fresh", Verdict: "BLOCK"},
		{Name: "missing", Freshness: "unknown", Verdict: "INCONCLUSIVE"},
		{Name: "invalid", ObservedAt: "not-a-time", Freshness: "unknown", Verdict: "INCONCLUSIVE"},
		{Name: "zero", ObservedAt: "0001-01-01T00:00:00Z", Freshness: "unknown", Verdict: "INCONCLUSIVE"},
		{Name: "future", ObservedAt: now.Add(24 * time.Hour).Format(time.RFC3339), Freshness: "unknown", Verdict: "INCONCLUSIVE"},
		{Name: "stale", ObservedAt: now.Add(-time.Hour).Format(time.RFC3339), Freshness: "stale", Verdict: "WATCH"},
		{Name: "stale-failure", ObservedAt: now.Add(-time.Hour).Format(time.RFC3339), Failed: true, Freshness: "stale", Verdict: "WATCH"},
	}
	var rows []map[string]interface{}
	for _, tc := range cases {
		var row []map[string]interface{}
		if err := json.Unmarshal([]byte(liveStatusFreshnessInput(t, tc)), &row); err != nil {
			t.Fatal(err)
		}
		row[0]["Slug"], row[0]["SpaceID"] = tc.Name, "space-"+tc.Name
		rows = append(rows, row[0])
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []struct {
		name, binary string
		plugin       bool
	}{{"standalone", binary, false}, {"plugin", plugin, true}} {
		t.Run(mode.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.Mkdir(filepath.Join(home, ".cub-scout"), 0o700); err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string][]byte{
				".cub-scout/auth.json": []byte(`{"token":"isolated-fixture-token"}`),
				"statuses.json":        raw,
				"cub": []byte("#!/bin/sh\nset -eu\n" +
					"test \"$#\" -eq 6\n" +
					"test \"$*\" = 'space list -o json --select Slug,SpaceID,Annotations,Labels'\n" +
					"printf 'read\\n' >> \"$HOME/reads\"\n" +
					"exec /bin/cat \"$HOME/statuses.json\"\n"),
			} {
				if err := os.WriteFile(filepath.Join(home, name), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(filepath.Join(home, "cub"), 0o700); err != nil {
				t.Fatal(err)
			}
			var input bytes.Buffer
			for id, request := range []map[string]interface{}{
				{"method": "initialize", "params": map[string]interface{}{"protocolVersion": "2025-03-26", "capabilities": map[string]interface{}{}, "clientInfo": map[string]string{"name": "packaged-freshness-proof", "version": "1"}}},
				{"method": "tools/call", "params": map[string]interface{}{"name": "confighub_live_status", "arguments": map[string]string{"space": "*"}}},
			} {
				request["jsonrpc"], request["id"] = "2.0", id+1
				payload, err := json.Marshal(request)
				if err != nil {
					t.Fatal(err)
				}
				if err := writeMCPFrame(&input, payload); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, mode.binary, "mcp", "serve")
			cmd.Env = []string{"HOME=" + home, "PATH=" + home + ":/usr/bin:/bin", "NO_COLOR=1", "TERM=dumb", "KUBECONFIG=" + filepath.Join(home, "absent-kubeconfig")}
			if mode.plugin {
				cmd.Env = append(cmd.Env, "CUB_PLUGIN=1", "CUB_TOKEN=isolated-fixture-token")
			}
			var stderr bytes.Buffer
			cmd.Stdin, cmd.Stderr = &input, &stderr
			output, err := cmd.Output()
			if err != nil {
				t.Fatalf("provider failed: %v: %s", err, stderr.String())
			}
			reader := bufio.NewReader(bytes.NewReader(output))
			for id := 1; id <= 2; id++ {
				frame, err := readMCPFrame(reader)
				if err != nil {
					t.Fatalf("response %d: %v: %s", id, err, output)
				}
				var response struct {
					ID     int
					Error  json.RawMessage
					Result struct {
						IsError           bool
						StructuredContent struct {
							LiveStatuses []ConfigHubLiveStatusEvidence
							Omissions    []GitOpsDeliveryEvidenceOmission
						}
					}
				}
				if err := json.Unmarshal(frame, &response); err != nil {
					t.Fatal(err)
				}
				if response.ID != id || len(response.Error) > 0 || response.Result.IsError {
					t.Fatalf("response failed (startup requires public health reachability): %s", frame)
				}
				if id == 1 {
					continue
				}
				got := response.Result.StructuredContent
				if len(got.LiveStatuses) != len(cases) || len(got.Omissions) != 6 {
					t.Errorf("expected %d reports and 6 omissions, got %d and %d", len(cases), len(got.LiveStatuses), len(got.Omissions))
				}
				bySpace := map[string]ConfigHubLiveStatusEvidence{}
				for _, status := range got.LiveStatuses {
					bySpace[status.Space] = status
				}
				for _, tc := range cases {
					status := bySpace[tc.Name]
					if status.Freshness != tc.Freshness || status.DeliveryVerdict != tc.Verdict || status.ApplicationHealthVerdict != tc.Verdict || status.ObservedAt != tc.ObservedAt || status.SpaceID != "space-"+tc.Name || status.App != "api" || status.Revision != "sha256:abc" {
						t.Errorf("%s: wrong report: %+v", tc.Name, status)
					}
					sync, health, phase := "Synced", "Healthy", "Succeeded"
					if tc.Failed {
						sync, health, phase = "OutOfSync", "Degraded", "Failed"
					}
					if status.SyncStatus != sync || status.HealthStatus != health || status.OperationPhase != phase || status.Source != "argobot" {
						t.Errorf("%s: original status fields changed: %+v", tc.Name, status)
					}
				}
				for _, omission := range got.Omissions {
					if omission.Layer != "confighub.liveStatus.freshness" || omission.Reason == "" || omission.Impact == "" {
						t.Errorf("unexplained omission: %+v", omission)
					}
				}
			}
			reads, err := os.ReadFile(filepath.Join(home, "reads"))
			if err != nil || string(reads) != "read\n" {
				t.Fatalf("expected exactly one fixture CLI read: %q, %v", reads, err)
			}
			t.Logf("%s: 8 timestamp cases, original fields, 6 omissions, one fixture read", mode.name)
		})
	}
}
