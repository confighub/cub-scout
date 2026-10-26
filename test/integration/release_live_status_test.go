//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/hubread"
)

// TestRecordReleaseLiveStatusOnARealServer writes a LiveStatus on a Release of
// a disposable ConfigHub, as the tool deploying it would, and checks that the
// built binary's MCP confighub_live_status reports it. It keeps the release
// list as the server printed it, so the unit tests' shape can be held to a
// recording (#882).
func TestRecordReleaseLiveStatusOnARealServer(t *testing.T) {
	if os.Getenv(disposableConfigHubEnv) != "1" {
		t.Skipf("writes a space, a Unit, a Target, a release and its live status; set %s=1 only for a disposable ConfigHub server", disposableConfigHubEnv)
	}
	skipIfNotConnected(t)
	contextOut, err := exec.Command("cub", "context", "get").CombinedOutput()
	if err != nil {
		t.Fatalf("cub context get: %v: %s", err, contextOut)
	}
	if strings.Contains(strings.ToLower(string(contextOut)), "confighub.com") {
		t.Fatalf("refusing to run: the cub context names a hosted ConfigHub server, not a disposable one")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	reader, err := hubread.Resolve(ctx, hubread.Options{UserAgent: "cub-scout-integration"})
	if err != nil {
		t.Fatalf("resolve the credential cub uses: %v", err)
	}
	if host := serverHost(reader.Server()); host != "localhost" && host != "127.0.0.1" && host != "::1" {
		t.Fatalf("refusing to run: the resolved server %q is not on this machine", host)
	}
	out := os.Getenv(sdkParityOutEnv)
	if out == "" {
		out = t.TempDir()
	}
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(out, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	space := fmt.Sprintf("scout-live-status-%d", time.Now().UnixNano())
	if _, err := cubStdoutOnly("space", "create", space); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = cubStdoutOnly("space", "update", "--patch", space, "--release-target", "-")
		deleteTestSpace(t, space)
	})
	manifest := filepath.Join(t.TempDir(), "live.yaml")
	if err := os.WriteFile(manifest, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: live\n  namespace: default\ndata:\n  key: one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"unit", "create", "--space", space, "live", manifest},
		{"target", "create", "--space", space, "live-target"},
		{"space", "update", "--patch", space, "--release-target", "live-target"},
		{"unit", "set-target", "live", "live-target", "--space", space},
		{"release", "publish", space},
	} {
		if printed, err := cubStdoutOnly(args...); err != nil {
			t.Fatalf("cub %s: %v: %s", strings.Join(args, " "), err, printed)
		}
	}
	listed, err := cubStdoutOnly("release", "list", "--space", space, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var releases []struct{ Release struct{ ReleaseID string } }
	if err := json.Unmarshal(listed, &releases); err != nil || len(releases) != 1 || releases[0].Release.ReleaseID == "" {
		t.Fatalf("expected one release: %v: %s", err, listed)
	}

	// What argobot sends: the whole status, as a merge patch of LiveStatus.
	observedAt := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	status := map[string]interface{}{"LiveStatus": map[string]interface{}{
		"Reporter": "cub-scout-integration", "DataSource": "test",
		"Sync": "Synced", "Health": "Degraded", "Operation": "Succeeded",
		"ReporterSync": "Synced", "ReporterHealth": "Degraded", "ReporterOperation": "Succeeded",
		"Message": "written by the integration test", "ObservedAt": observedAt,
	}}
	body, _ := json.Marshal(status)
	written := ""
	for _, args := range [][]string{
		{"release", "update", "--patch", releases[0].Release.ReleaseID, "--from-stdin"},
		{"release", "update", releases[0].Release.ReleaseID, "--from-stdin"},
	} {
		cmd := exec.Command("cub", args...)
		cmd.Stdin = bytes.NewReader(body)
		if printed, err := cmd.CombinedOutput(); err != nil {
			t.Logf("cub %s: %v: %s", strings.Join(args, " "), err, printed)
			continue
		}
		written = strings.Join(args[:3], " ")
		break
	}
	if written == "" {
		t.Fatalf("no cub release update wrote a LiveStatus on this server")
	}
	withStatus, err := cubStdoutOnly("release", "list", "--space", space, "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	keep("release-list-with-live-status.cub.json", withStatus)
	if !bytes.Contains(withStatus, []byte(`"LiveStatus"`)) || !bytes.Contains(withStatus, []byte(`"Degraded"`)) {
		t.Fatalf("the release list does not show the LiveStatus written by %q: %s", written, withStatus)
	}

	binary := getCubAgentPath()
	request, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]interface{}{"name": "confighub_live_status", "arguments": map[string]string{"space": space}},
	})
	cmd := exec.Command(binary, "mcp", "serve")
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" + string(request) + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("mcp serve: %v: %s", err, stderr.String())
	}
	var answer json.RawMessage
	for _, line := range strings.Split(string(stdout), "\n") {
		var response struct {
			ID     json.RawMessage
			Result json.RawMessage
		}
		if json.Unmarshal([]byte(line), &response) == nil && string(response.ID) == "2" {
			answer = response.Result
		}
	}
	keep("mcp-live-status.json", answer)
	var result struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			LiveStatuses []map[string]interface{} `json:"liveStatuses"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(answer, &result); err != nil || result.IsError {
		t.Fatalf("confighub_live_status failed: %v: %s", err, answer)
	}
	if len(result.StructuredContent.LiveStatuses) != 1 {
		t.Fatalf("expected the Release's live status: %s", answer)
	}
	got := result.StructuredContent.LiveStatuses[0]
	for key, want := range map[string]string{"recordedOn": "release", "space": space, "healthStatus": "Degraded", "syncStatus": "Synced", "source": "cub-scout-integration", "applicationHealthVerdict": "BLOCK"} {
		if got[key] != want {
			t.Errorf("%s = %v, want %q: %s", key, got[key], want, answer)
		}
	}
	summary, _ := json.MarshalIndent(map[string]string{"written_by": written, "observed_at": observedAt}, "", "  ")
	keep("release-live-status.json", append(summary, '\n'))
}
