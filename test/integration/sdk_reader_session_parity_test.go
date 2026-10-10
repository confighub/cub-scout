//go:build integration
// +build integration

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

// #758: the session check, the local context, the organization list and the
// lists across every space, by cub and by the SDK reader, on a real ConfigHub
// server; and a connected read through the built binary on a PATH that has no
// cub on it.
func TestSDKReaderMatchesCubOnTheSessionAndAcrossSpaces(t *testing.T) {
	if os.Getenv(disposableConfigHubEnv) != "1" {
		t.Skipf("writes a space and Units; set %s=1 only for a disposable ConfigHub server", disposableConfigHubEnv)
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
	out = filepath.Join(out, "session")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	keep := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(out, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	notes, equal := map[string]string{}, map[string]bool{}

	// The session: cub accepts it, so the reader must, and it must report
	// the server's version as cub does.
	if printed, err := exec.Command("cub", "auth", "status").CombinedOutput(); err != nil {
		t.Fatalf("cub auth status refused the session this test runs with: %v: %s", err, printed)
	}
	session, err := reader.CheckSession(ctx)
	if err != nil {
		t.Fatalf("the reader refused a session cub accepts: %v (kind %s)", err, hubread.KindOf(err))
	}
	notes["server version, from the reader"] = session.ServerVersion
	notes["SDK version"] = session.SDKVersion
	notes["client ahead of the server"] = fmt.Sprint(session.ClientAhead)
	versionOut, _ := exec.Command("cub", "version").CombinedOutput()
	if session.ServerVersion == "" || !strings.Contains(string(versionOut), session.ServerVersion) {
		t.Errorf("the reader read server version %q, which `cub version` does not report: %s", session.ServerVersion, versionOut)
	}
	if session.SDKVersion == "" {
		t.Errorf("this test binary records no SDK version, so the version rule was not exercised")
	}

	// The local context: what cub prints for it, from the same files. It
	// names the instance's user, so only the comparison is kept.
	contextByCub, err := cubStdoutOnly("context", "get", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	contextBySDK, err := hubread.LocalContextJSON(ctx)
	if err != nil {
		t.Fatalf("local context: %v", err)
	}
	equal["context get"] = bytes.Equal(contextByCub, contextBySDK)
	if !bytes.Equal(contextByCub, contextBySDK) {
		t.Errorf("the local context is not what cub printed: %d bytes against %d", len(contextBySDK), len(contextByCub))
	}
	if bytes.Contains(contextBySDK, []byte("accessToken")) {
		t.Fatalf("the local context carries a token")
	}

	orgsByCub, err := cubStdoutOnly("organization", "list", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	orgsBySDK, err := reader.OrganizationListJSON(ctx)
	if err != nil {
		t.Fatalf("organization list: %v (kind %s)", err, hubread.KindOf(err))
	}
	equal["organization list"] = bytes.Equal(orgsByCub, orgsBySDK)
	if !bytes.Equal(orgsByCub, orgsBySDK) {
		keep("organization-list.cub.json", orgsByCub)
		keep("organization-list.sdk.json", orgsBySDK)
		t.Errorf("the organization list is not what cub printed")
	}

	// Two spaces with a Unit each, so that "every space" has more than one
	// space in it whatever else the server holds.
	stamp := time.Now().UnixNano()
	spaces := []string{fmt.Sprintf("scout-sdk-wide-a-%d", stamp), fmt.Sprintf("scout-sdk-wide-b-%d", stamp)}
	for _, space := range spaces {
		if _, err := cubStdoutOnly("space", "create", space); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { deleteTestSpace(t, space) })
		path := filepath.Join(t.TempDir(), "wide.yaml")
		body := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: wide\n  namespace: default\ndata:\n  space: " + space + "\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := cubStdoutOnly("unit", "create", "--space", space, "wide-unit", path); err != nil {
			t.Fatal(err)
		}
	}
	const filter = "Slug = 'wide-unit'"
	lists := map[string]func(hubread.Filter) ([]byte, error){
		"unit":        func(f hubread.Filter) ([]byte, error) { return reader.UnitListJSON(ctx, hubread.EverySpace, f) },
		"worker":      func(f hubread.Filter) ([]byte, error) { return reader.WorkerListJSON(ctx, hubread.EverySpace, f) },
		"target":      func(f hubread.Filter) ([]byte, error) { return reader.TargetListJSON(ctx, hubread.EverySpace, f) },
		"changeset":   func(f hubread.Filter) ([]byte, error) { return reader.ChangeSetListJSON(ctx, hubread.EverySpace, f) },
		"link":        func(f hubread.Filter) ([]byte, error) { return reader.LinkListJSON(ctx, hubread.EverySpace, f) },
		"resource":    func(f hubread.Filter) ([]byte, error) { return reader.ResourceListJSON(ctx, hubread.EverySpace, f) },
		"unit-event":  func(f hubread.Filter) ([]byte, error) { return reader.UnitEventListJSON(ctx, hubread.EverySpace, f) },
		"attestation": func(f hubread.Filter) ([]byte, error) { return reader.AttestationListJSON(ctx, hubread.EverySpace, f) },
	}
	sizes := map[string]int{}
	for kind, read := range lists {
		byCub, err := cubStdoutOnly(kind, "list", "--space", "*", "-o", "json")
		if err != nil {
			t.Errorf("cub %s list across every space: %v", kind, err)
			continue
		}
		if kind == "worker" {
			byCub = workerSecretLine.ReplaceAll(byCub, nil)
			if bytes.Contains(byCub, []byte(`"Secret"`)) || bytes.Contains(byCub, []byte(`"ClaimToken"`)) {
				t.Fatalf("a worker's credential is still in cub's list; nothing is compared")
			}
		}
		bySDK, err := read(hubread.Filter{})
		if err != nil {
			t.Errorf("%s list across every space through the SDK: %v (kind %s)", kind, err, hubread.KindOf(err))
			continue
		}
		var entries []json.RawMessage
		_ = json.Unmarshal(byCub, &entries)
		sizes[kind] = len(entries)
		equal[kind+" list, every space"] = bytes.Equal(byCub, bySDK)
		if !bytes.Equal(byCub, bySDK) {
			// Lengths only: these lists span whatever else the server holds.
			t.Errorf("%s list across every space: %d bytes from the reader against %d from cub", kind, len(bySDK), len(byCub))
		}
	}
	// The filter narrows across spaces to the two Units made here, one in
	// each space; equal bytes alone would also hold for two unfiltered lists.
	filteredByCub, err := cubStdoutOnly("unit", "list", "--space", "*", "-o", "json", "--where", filter)
	if err != nil {
		t.Fatal(err)
	}
	filteredBySDK, err := reader.UnitListJSON(ctx, hubread.EverySpace, hubread.Filter{Where: filter})
	if err != nil {
		t.Fatalf("filtered unit list across every space: %v", err)
	}
	keep("unit-list-every-space-filtered.cub.json", filteredByCub)
	equal["unit list, every space, filtered"] = bytes.Equal(filteredByCub, filteredBySDK)
	var found []struct {
		Unit struct{ Slug, SpaceSlug string }
	}
	if err := json.Unmarshal(filteredBySDK, &found); err != nil {
		t.Fatal(err)
	}
	inSpaces := map[string]bool{}
	for _, entry := range found {
		inSpaces[entry.Unit.SpaceSlug] = true
	}
	if !bytes.Equal(filteredByCub, filteredBySDK) || len(found) != 2 || !inSpaces[spaces[0]] || !inSpaces[spaces[1]] {
		keep("unit-list-every-space-filtered.sdk.json", filteredBySDK)
		t.Errorf("the filtered unit list across every space: equal %v, %d Units, in spaces %v; want the two made here", bytes.Equal(filteredByCub, filteredBySDK), len(found), inSpaces)
	}

	// A connected read through the built binary with no cub on PATH. The cub
	// configuration is still where CUB_CONFIG says; only the program is gone.
	binary := getCubAgentPath()
	emptyPath := t.TempDir()
	request, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]interface{}{"name": "confighub_units", "arguments": map[string]string{"space": spaces[0]}},
	})
	withoutCub := func(route string) (answer []byte, stderr string) {
		cmd := exec.Command(binary, "mcp", "serve")
		cmd.Env = append(os.Environ(), "PATH="+emptyPath, "CUB_SCOUT_CONFIGHUB_READER="+route)
		cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" + string(request) + "\n")
		var errOut bytes.Buffer
		cmd.Stderr = &errOut
		stdout, err := cmd.Output()
		if err != nil {
			t.Fatalf("mcp serve with no cub on PATH, %s route: %v: %s", route, err, errOut.String())
		}
		for _, line := range strings.Split(string(stdout), "\n") {
			var response struct {
				ID     json.RawMessage
				Result json.RawMessage
				Error  json.RawMessage
			}
			if json.Unmarshal([]byte(line), &response) == nil && string(response.ID) == "2" {
				if response.Result != nil {
					return response.Result, errOut.String()
				}
				return response.Error, errOut.String()
			}
		}
		return nil, errOut.String()
	}
	if _, err := exec.LookPath("cub"); err != nil {
		t.Fatalf("this test needs cub on its own PATH to set up: %v", err)
	}
	bySDKRoute, _ := withoutCub("sdk")
	notes["connected MCP read with no cub on PATH, SDK route"] = fmt.Sprint(bytes.Contains(bySDKRoute, []byte("wide-unit")) && !bytes.Contains(bySDKRoute, []byte(`"isError":true`)))
	if !bytes.Contains(bySDKRoute, []byte("wide-unit")) || bytes.Contains(bySDKRoute, []byte(`"isError":true`)) {
		t.Errorf("with no cub on PATH the SDK route did not list the space's Unit: %s", bySDKRoute)
	}
	// By the cub route the same call cannot work without cub, and must say so
	// rather than answer.
	byCubRoute, _ := withoutCub("cub")
	notes["the same with the cub route"] = fmt.Sprint(!bytes.Contains(byCubRoute, []byte("wide-unit")))
	if bytes.Contains(byCubRoute, []byte("wide-unit")) {
		t.Errorf("the cub route answered with no cub on PATH: %s", byCubRoute)
	}

	summary, _ := json.MarshalIndent(map[string]interface{}{"notes": notes, "equal": equal, "entries across every space": sizes}, "", "  ")
	keep("session.json", append(summary, '\n'))
	t.Logf("session and wide reads: %s", summary)
}
