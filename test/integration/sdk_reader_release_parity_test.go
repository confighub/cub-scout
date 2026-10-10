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

// #758: the release, resource and unit-event lists of a space, by cub and by
// the SDK reader, on a real ConfigHub server.
//
// Releases are published here through cub: a Target, the space's release
// Target, both Units assigned to it, and two releases with a change between
// them. Each of those steps is allowed to fail, since what a server lets a
// bare space do changes between versions; the artifact says which did, and a
// list that stays empty is still compared. Nothing on this server applies a
// Unit, so it has no unit events unless a later server makes some: that list
// is expected to be compared empty, and the test says so in what it keeps.
func TestSDKReaderMatchesCubOnReleasesResourcesAndEvents(t *testing.T) {
	if os.Getenv(disposableConfigHubEnv) != "1" {
		t.Skipf("writes a space, Units, a Target and releases; set %s=1 only for a disposable ConfigHub server", disposableConfigHubEnv)
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
	// Checked before anything is written: a server anywhere but this machine
	// is not one this test may write to, whatever it is called.
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

	space := fmt.Sprintf("scout-sdk-releases-%d", time.Now().UnixNano())
	if _, err := cubStdoutOnly("space", "create", space); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// The space's release Target is in the space; clear the reference
		// so the space can be deleted with it.
		_, _ = cubStdoutOnly("space", "update", "--patch", space, "--release-target", "-")
		deleteTestSpace(t, space)
	})
	manifest := func(name, value string) string {
		path := filepath.Join(t.TempDir(), name+".yaml")
		body := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: " + name + "\n  namespace: default\ndata:\n  key: " + value + "\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	const unit, other, target = "release-unit", "release-other", "release-target"
	for _, name := range []string{unit, other} {
		if _, err := cubStdoutOnly("unit", "create", "--space", space, name, manifest(name, "one")); err != nil {
			t.Fatal(err)
		}
	}
	spaceListed, err := cubStdoutOnly("space", "list", "-o", "json", "--where", "Slug = '"+space+"'")
	if err != nil {
		t.Fatal(err)
	}
	var spaces []struct{ Space struct{ SpaceID string } }
	if err := json.Unmarshal(spaceListed, &spaces); err != nil || len(spaces) != 1 || spaces[0].Space.SpaceID == "" {
		t.Fatalf("the new space's ID could not be read: %v: %s", err, spaceListed)
	}
	spaceID := spaces[0].Space.SpaceID

	steps := map[string]string{}
	step := func(name string, args ...string) {
		if printed, err := cubStdoutOnly(args...); err != nil {
			steps[name] = "failed: " + err.Error()
			t.Logf("%s did not work on this server: %v", name, err)
		} else {
			steps[name] = "ok: " + strings.TrimSpace(string(printed))
		}
	}
	step("target create", "target", "create", "--space", space, target)
	step("space release target", "space", "update", "--patch", space, "--release-target", target)
	step("set target of "+unit, "unit", "set-target", unit, target, "--space", space)
	step("set target of "+other, "unit", "set-target", other, target, "--space", space)
	step("first release", "release", "publish", space)
	step("change "+unit, "unit", "update", "--space", space, unit, manifest(unit, "two"))
	step("second release", "release", "publish", space)

	equal, sizes := map[string]bool{}, map[string]int{}
	compare := func(name string, cubArgs []string, read func(space string) ([]byte, error)) []byte {
		t.Helper()
		listedByCub, err := cubStdoutOnly(cubArgs...)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return nil
		}
		keep(name+".cub.json", listedByCub)
		var entries []json.RawMessage
		if err := json.Unmarshal(listedByCub, &entries); err != nil {
			t.Errorf("%s: cub did not print a JSON list: %v", name, err)
			return nil
		}
		sizes[name] = len(entries)
		for how, named := range map[string]string{"slug": space, "id": spaceID} {
			listedBySDK, err := read(named)
			if err != nil {
				t.Errorf("%s through the SDK, space by %s: %v (kind %s)", name, how, err, hubread.KindOf(err))
				continue
			}
			if how == "slug" {
				keep(name+".sdk.json", listedBySDK)
			}
			equal[name+"-by-"+how] = bytes.Equal(listedByCub, listedBySDK)
			if !bytes.Equal(listedByCub, listedBySDK) {
				keep(name+".sdk-by-"+how+".json", listedBySDK)
				t.Errorf("%s, space by %s: the SDK reader's JSON is not what cub printed: %d bytes against %d; both are kept in %s", name, how, len(listedBySDK), len(listedByCub), out)
			}
		}
		return listedByCub
	}
	// The filter `gitops status` sends with both of its ConfigHub lists.
	const since = "CreatedAt > '2020-01-01T00:00:00Z'"

	compare("resource-list", []string{"resource", "list", "--space", space, "-o", "json"},
		func(s string) ([]byte, error) { return reader.ResourceListJSON(ctx, s, hubread.Filter{}) })
	compare("resource-list-contains", []string{"resource", "list", "--space", space, "-o", "json", "--contains", unit},
		func(s string) ([]byte, error) { return reader.ResourceListJSON(ctx, s, hubread.Filter{Contains: unit}) })
	if sizes["resource-list"] == 0 {
		t.Errorf("the space has two Units, each one ConfigMap, and cub lists no resources in it")
	}

	releases := compare("release-list", []string{"release", "list", "--space", space, "-o", "json"},
		func(s string) ([]byte, error) { return reader.ReleaseListJSON(ctx, s, hubread.Filter{}) })
	compare("release-list-since", []string{"release", "list", "--space", space, "-o", "json", "--where", since},
		func(s string) ([]byte, error) { return reader.ReleaseListJSON(ctx, s, hubread.Filter{Where: since}) })
	compare("release-list-first", []string{"release", "list", "--space", space, "-o", "json", "--where", "ReleaseNum = 1"},
		func(s string) ([]byte, error) {
			return reader.ReleaseListJSON(ctx, s, hubread.Filter{Where: "ReleaseNum = 1"})
		})
	// Equal bytes would also hold for two lists that ignored the filter, or
	// were in the same wrong order.
	var listed []struct{ Release struct{ ReleaseNum int } }
	if err := json.Unmarshal(releases, &listed); err != nil {
		t.Errorf("release list: %v", err)
	}
	for i := 1; i < len(listed); i++ {
		if listed[i-1].Release.ReleaseNum <= listed[i].Release.ReleaseNum {
			t.Errorf("cub's release list is not newest first: release %d before %d", listed[i-1].Release.ReleaseNum, listed[i].Release.ReleaseNum)
		}
	}
	if sizes["release-list"] > 1 && sizes["release-list-first"] != 1 {
		t.Errorf("the filter ReleaseNum = 1 listed %d of %d releases, want 1", sizes["release-list-first"], sizes["release-list"])
	}

	compare("unit-event-list", []string{"unit-event", "list", "--space", space, "-o", "json"},
		func(s string) ([]byte, error) { return reader.UnitEventListJSON(ctx, s, hubread.Filter{}) })
	compare("unit-event-list-since", []string{"unit-event", "list", "--space", space, "-o", "json", "--where", since},
		func(s string) ([]byte, error) { return reader.UnitEventListJSON(ctx, s, hubread.Filter{Where: since}) })
	compare("unit-event-list-of-unit", []string{"unit-event", "list", unit, "--space", space, "-o", "json"},
		func(s string) ([]byte, error) { return reader.UnitEventsOfUnitJSON(ctx, s, unit, hubread.Filter{}) })

	// A space that does not exist, by slug and by an ID that names nothing:
	// not_found from the reader, a failure from cub, and no list from either.
	for _, missing := range []string{"no-such-space-" + space, "11111111-2222-4333-8444-555555555555"} {
		for kind, read := range map[string]func() ([]byte, error){
			"resource":   func() ([]byte, error) { return reader.ResourceListJSON(ctx, missing, hubread.Filter{}) },
			"release":    func() ([]byte, error) { return reader.ReleaseListJSON(ctx, missing, hubread.Filter{}) },
			"unit-event": func() ([]byte, error) { return reader.UnitEventListJSON(ctx, missing, hubread.Filter{}) },
		} {
			if listed, err := read(); hubread.KindOf(err) != hubread.KindNotFound || listed != nil {
				t.Errorf("%s list in space %q, which does not exist: kind %s, want %s (%v)", kind, missing, hubread.KindOf(err), hubread.KindNotFound, err)
			}
			if listed, err := cubStdoutOnly(kind, "list", "--space", missing, "-o", "json"); err == nil {
				t.Errorf("cub listed the %ss of space %q, which does not exist: %s", kind, missing, listed)
			}
		}
	}
	// The events of a Unit that is not there are not an empty list.
	if listed, err := reader.UnitEventsOfUnitJSON(ctx, space, "no-such-unit", hubread.Filter{}); hubread.KindOf(err) != hubread.KindNotFound || listed != nil {
		t.Errorf("the events of a Unit that does not exist: kind %s, want %s (%v)", hubread.KindOf(err), hubread.KindNotFound, err)
	}
	if listed, err := cubStdoutOnly("unit-event", "list", "no-such-unit", "--space", space, "-o", "json"); err == nil {
		t.Errorf("cub listed the events of a Unit that does not exist: %s", listed)
	}
	// A filter the server rejects fails by both routes.
	const badFilter = "NoSuchField = 'x'"
	if listed, err := cubStdoutOnly("release", "list", "--space", space, "-o", "json", "--where", badFilter); err == nil {
		t.Errorf("cub accepted the release filter %q: %s", badFilter, listed)
	}
	if listed, err := reader.ReleaseListJSON(ctx, space, hubread.Filter{Where: badFilter}); err == nil {
		t.Errorf("the reader accepted the release filter %q: %s", badFilter, listed)
	} else {
		keep("release-list-bad-filter.sdk.txt", []byte(err.Error()+"\n"))
	}

	// End to end through the built binary: each MCP tool, by each route.
	binary := getCubAgentPath()
	callTool := func(tool string, arguments map[string]string, route string) []byte {
		t.Helper()
		request, _ := json.Marshal(map[string]interface{}{
			"jsonrpc": "2.0", "id": 2, "method": "tools/call",
			"params": map[string]interface{}{"name": tool, "arguments": arguments},
		})
		cmd := exec.Command(binary, "mcp", "serve")
		cmd.Env = append(os.Environ(), "CUB_SCOUT_CONFIGHUB_READER="+route)
		cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" + string(request) + "\n")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		stdout, err := cmd.Output()
		if err != nil {
			t.Fatalf("mcp serve by the %s route: %v: %s", route, err, stderr.String())
		}
		for _, line := range strings.Split(string(stdout), "\n") {
			var response struct {
				ID     json.RawMessage
				Result json.RawMessage
			}
			if json.Unmarshal([]byte(line), &response) == nil && string(response.ID) == "2" {
				return response.Result
			}
		}
		t.Fatalf("mcp serve by the %s route gave no answer to the call: %s", route, stdout)
		return nil
	}
	for name, call := range map[string]struct {
		tool      string
		arguments map[string]string
	}{
		"releases":            {"confighub_releases", map[string]string{"space": space}},
		"releases-filtered":   {"confighub_releases", map[string]string{"space": space, "where": since}},
		"resources":           {"confighub_resources", map[string]string{"space": space}},
		"unit-events":         {"confighub_unit_events", map[string]string{"space": space}},
		"unit-events-of-unit": {"confighub_unit_events", map[string]string{"space": space, "unit": unit, "where": since}},
	} {
		viaCub, viaSDK := callTool(call.tool, call.arguments, "cub"), callTool(call.tool, call.arguments, "sdk")
		equal["mcp-"+name] = bytes.Equal(viaCub, viaSDK)
		if !bytes.Equal(viaCub, viaSDK) {
			keep("mcp-"+name+".cub.json", viaCub)
			keep("mcp-"+name+".sdk.json", viaSDK)
			t.Errorf("the MCP tool %s (%s) answers differently by route; both are kept in %s", call.tool, name, out)
		}
		if bytes.Contains(viaSDK, []byte(`"isError":true`)) {
			t.Errorf("the MCP tool %s (%s) failed: %s", call.tool, name, viaSDK)
		}
	}

	summary, _ := json.MarshalIndent(map[string]interface{}{"steps": steps, "entries": sizes, "equal": equal}, "", "  ")
	keep("release-event-lists.json", append(summary, '\n'))
	t.Logf("release, resource and unit-event lists: %s", summary)
}
