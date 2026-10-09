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
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/hubread"
)

// disposableConfigHubEnv must be "1" for the test below to run. It creates a
// space and two Units, so it runs only against a server that exists for the
// test: the one scripts/ci/setup-connected-server.sh installs.
const disposableConfigHubEnv = "SCOUT_DISPOSABLE_CONFIGHUB"

// sdkParityOutEnv names a directory that receives what each route returned,
// the timings and the list recordings, for the CI artifact.
const sdkParityOutEnv = "SCOUT_SDK_PARITY_OUT"

// cubStdoutOnly runs cub and returns stdout alone; cub prints notices on stderr.
func cubStdoutOnly(args ...string) ([]byte, error) {
	cmd := exec.Command("cub", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("cub %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func milliseconds(samples []time.Duration) map[string]float64 {
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	return map[string]float64{"min": ms(sorted[0]), "median": ms(sorted[len(sorted)/2]), "max": ms(sorted[len(sorted)-1])}
}

// #758: the SDK reader against a real ConfigHub server. The unit tests feed
// both routes one recorded object from a test server; this asks a real server
// the question both ways and compares the answers byte for byte.
func TestSDKReaderMatchesCubOnARealServer(t *testing.T) {
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

	space := fmt.Sprintf("scout-sdk-parity-%d", time.Now().UnixNano())
	if _, err := cubStdoutOnly("space", "create", space); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTestSpace(t, space) })

	manifest := func(name string) string {
		path := filepath.Join(t.TempDir(), name+".yaml")
		body := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: " + name + "\n  namespace: default\ndata:\n  key: value\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	const unit, other = "parity-unit", "parity-other"
	for _, name := range []string{unit, other} {
		if _, err := cubStdoutOnly("unit", "create", "--space", space, name, manifest(name)); err != nil {
			t.Fatal(err)
		}
	}

	// What cub prints, twice: if the two differ the Unit is still changing
	// and a byte comparison with anything would mean nothing.
	getArgs := []string{"unit", "get", unit, "--space", space, "-o", "json"}
	viaCub, err := cubStdoutOnly(getArgs...)
	if err != nil {
		t.Fatal(err)
	}
	keep("unit-get.cub.json", viaCub)
	again, err := cubStdoutOnly(getArgs...)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(viaCub, again) {
		keep("unit-get.cub-again.json", again)
		t.Fatalf("cub printed two different answers for the same Unit; it is not at rest")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	reader, err := hubread.Resolve(ctx, hubread.Options{UserAgent: "cub-scout-integration"})
	if err != nil {
		t.Fatalf("resolve the credential cub uses: %v", err)
	}
	if strings.Contains(strings.ToLower(reader.Server()), "confighub.com") {
		t.Fatalf("refusing to run: the resolved server is a hosted ConfigHub, not a disposable one")
	}
	viaSDK, err := reader.UnitJSON(ctx, space, unit)
	if err != nil {
		t.Fatalf("SDK read: %v (kind %s)", err, hubread.KindOf(err))
	}
	keep("unit-get.sdk.json", viaSDK)
	if !bytes.Equal(viaCub, viaSDK) {
		t.Errorf("the SDK reader's JSON is not what cub printed: %d bytes against %d; both are kept in %s", len(viaSDK), len(viaCub), out)
	}
	bySlug := reader.Stats()
	if bySlug.Requests != 2 {
		t.Errorf("a read with the space named by slug made %d requests, want 2", bySlug.Requests)
	}

	// The same Unit with the space named by its ID: one request, same bytes.
	var envelope struct {
		Unit struct{ SpaceID, UnitID, Slug string }
	}
	if err := json.Unmarshal(viaCub, &envelope); err != nil {
		t.Fatalf("parse cub's output: %v", err)
	}
	if envelope.Unit.Slug != unit || envelope.Unit.SpaceID == "" || envelope.Unit.UnitID == "" {
		t.Fatalf("cub's output is not the Unit asked for: %+v", envelope.Unit)
	}
	byID, err := reader.UnitJSON(ctx, envelope.Unit.SpaceID, unit)
	if err != nil {
		t.Fatalf("SDK read with the space by ID: %v", err)
	}
	if !bytes.Equal(viaCub, byID) {
		keep("unit-get.sdk-by-space-id.json", byID)
		t.Errorf("with the space named by ID the SDK reader's JSON is not what cub printed")
	}
	if got := reader.Stats().Requests - bySlug.Requests; got != 1 {
		t.Errorf("a read with the space named by ID made %d requests, want 1", got)
	}

	// The server's own filter is what narrows to one Unit: the other Unit in
	// the space must not come back, and a name that matches nothing is
	// not_found on the SDK route and a failure on the cub route.
	head, err := reader.UnitHead(ctx, space, other)
	if err != nil || head.Unit != other || head.UnitID == envelope.Unit.UnitID {
		t.Errorf("reading the second Unit: %+v, %v", head, err)
	}
	if _, err := reader.UnitJSON(ctx, space, "no-such-unit"); hubread.KindOf(err) != hubread.KindNotFound {
		t.Errorf("a Unit that does not exist: kind %s, want %s (%v)", hubread.KindOf(err), hubread.KindNotFound, err)
	}
	if _, err := cubStdoutOnly("unit", "get", "no-such-unit", "--space", space, "-o", "json"); err == nil {
		t.Errorf("cub found a Unit that does not exist")
	}
	if _, err := reader.UnitJSON(ctx, "no-such-space-"+space, unit); hubread.KindOf(err) != hubread.KindNotFound {
		t.Errorf("a space that does not exist: kind %s, want %s (%v)", hubread.KindOf(err), hubread.KindNotFound, err)
	}

	// End to end through the built binary: the MCP tool, by each route.
	binary := getCubAgentPath()
	callTool := func(tool string, arguments map[string]string, route string) map[string]json.RawMessage {
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
				Result map[string]json.RawMessage
			}
			if json.Unmarshal([]byte(line), &response) == nil && string(response.ID) == "2" {
				keep("mcp-"+tool+"."+route+".json", []byte(line+"\n"))
				return response.Result
			}
		}
		t.Fatalf("mcp serve by the %s route gave no answer to the call: %s", route, stdout)
		return nil
	}
	unitArguments := map[string]string{"unit": unit, "space": space}
	mcpCub, mcpSDK := callTool("confighub_unit_get", unitArguments, "cub"), callTool("confighub_unit_get", unitArguments, "sdk")
	if string(mcpCub["isError"]) == "true" || !strings.Contains(string(mcpCub["content"]), envelope.Unit.UnitID) {
		t.Errorf("the MCP tool by the cub route did not return the Unit: %s", mcpCub["content"])
	}
	cubResult, _ := json.Marshal(mcpCub)
	sdkResult, _ := json.Marshal(mcpSDK)
	if !bytes.Equal(cubResult, sdkResult) {
		t.Errorf("the MCP tool answers differently by route; both are kept in %s", out)
	}

	// The lists, the same way: what cub prints against what the reader
	// returns, and the MCP tool by each route.
	listsEqual := map[string]bool{}
	for name, list := range map[string]struct {
		cub  []string
		read func() ([]byte, error)
	}{
		"unit-list":             {[]string{"unit", "list", "--space", space, "-o", "json"}, func() ([]byte, error) { return reader.UnitListJSON(ctx, space, hubread.UnitFilter{}) }},
		"unit-list-by-space-id": {[]string{"unit", "list", "--space", space, "-o", "json"}, func() ([]byte, error) { return reader.UnitListJSON(ctx, envelope.Unit.SpaceID, hubread.UnitFilter{}) }},
		"space-list":            {[]string{"space", "list", "-o", "json"}, func() ([]byte, error) { return reader.SpaceListJSON(ctx) }},
		"unit-list-where": {[]string{"unit", "list", "--space", space, "-o", "json", "--where", "Slug = '" + unit + "'"}, func() ([]byte, error) {
			return reader.UnitListJSON(ctx, space, hubread.UnitFilter{Where: "Slug = '" + unit + "'"})
		}},
		"unit-list-where-like": {[]string{"unit", "list", "--space", space, "-o", "json", "--where", "Slug LIKE 'parity-o%'"}, func() ([]byte, error) {
			return reader.UnitListJSON(ctx, space, hubread.UnitFilter{Where: "Slug LIKE 'parity-o%'"})
		}},
		"unit-list-contains": {[]string{"unit", "list", "--space", space, "-o", "json", "--contains", other}, func() ([]byte, error) {
			return reader.UnitListJSON(ctx, space, hubread.UnitFilter{Contains: other})
		}},
		"unit-list-where-none": {[]string{"unit", "list", "--space", space, "-o", "json", "--where", "Slug = 'no-such-unit'"}, func() ([]byte, error) {
			return reader.UnitListJSON(ctx, space, hubread.UnitFilter{Where: "Slug = 'no-such-unit'"})
		}},
	} {
		listsEqual[name] = false
		listedByCub, err := cubStdoutOnly(list.cub...)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		keep(name+".cub.json", listedByCub)
		listedBySDK, err := list.read()
		if err != nil {
			t.Errorf("%s through the SDK: %v (kind %s)", name, err, hubread.KindOf(err))
			continue
		}
		keep(name+".sdk.json", listedBySDK)
		listsEqual[name] = bytes.Equal(listedByCub, listedBySDK)
		if !listsEqual[name] {
			t.Errorf("%s: the SDK reader's JSON is not what cub printed: %d bytes against %d; both are kept in %s", name, len(listedBySDK), len(listedByCub), out)
		}
	}
	// A space that does not exist, named by slug and by an ID that names
	// nothing: not_found through the reader, a failure from cub, and never
	// an empty list from either.
	for _, missing := range []string{"no-such-space-" + space, "11111111-2222-4333-8444-555555555555"} {
		if listed, err := reader.UnitListJSON(ctx, missing, hubread.UnitFilter{}); hubread.KindOf(err) != hubread.KindNotFound || listed != nil {
			t.Errorf("a list in space %q, which does not exist: kind %s, want %s (%v)", missing, hubread.KindOf(err), hubread.KindNotFound, err)
		}
		if listed, err := cubStdoutOnly("unit", "list", "--space", missing, "-o", "json"); err == nil {
			t.Errorf("cub listed space %q, which does not exist: %s", missing, listed)
		}
	}
	// The filters narrow as they say: one Unit by its slug, the other by a
	// pattern, and none for a slug that is not there. Equal bytes alone would
	// also hold if both routes ignored the filter.
	for name, want := range map[string][]string{"unit-list-where": {unit}, "unit-list-where-like": {other}, "unit-list-where-none": {}} {
		recorded, err := os.ReadFile(filepath.Join(out, name+".sdk.json"))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		var listed []struct{ Unit struct{ Slug string } }
		if err := json.Unmarshal(recorded, &listed); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		got := []string{}
		for _, entry := range listed {
			got = append(got, entry.Unit.Slug)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s listed %v, want %v", name, got, want)
		}
	}
	// A filter the server rejects fails by both routes, and the reader passes
	// on the server's reason.
	const badFilter = "NoSuchField = 'x'"
	if listed, err := cubStdoutOnly("unit", "list", "--space", space, "-o", "json", "--where", badFilter); err == nil {
		t.Errorf("cub accepted the filter %q: %s", badFilter, listed)
	} else {
		keep("unit-list-bad-filter.cub.txt", []byte(err.Error()+"\n"))
	}
	if listed, err := reader.UnitListJSON(ctx, space, hubread.UnitFilter{Where: badFilter}); err == nil {
		t.Errorf("the reader accepted the filter %q: %s", badFilter, listed)
	} else {
		keep("unit-list-bad-filter.sdk.txt", []byte(err.Error()+"\n"))
		if !strings.Contains(err.Error(), "HTTP 400") {
			t.Errorf("a rejected filter: %v, want the server's HTTP 400", err)
		}
	}
	filteredArguments := map[string]string{"space": space, "where": "Slug = '" + unit + "'"}
	filteredCub, _ := json.Marshal(callTool("confighub_units", filteredArguments, "cub"))
	filteredSDK, _ := json.Marshal(callTool("confighub_units", filteredArguments, "sdk"))
	if !bytes.Equal(filteredCub, filteredSDK) || bytes.Contains(filteredSDK, []byte(`\"Slug\": \"`+other+`\"`)) {
		t.Errorf("the MCP units tool with a filter differs by route, or did not filter")
	}

	unitsArguments := map[string]string{"space": space}
	unitsCub, _ := json.Marshal(callTool("confighub_units", unitsArguments, "cub"))
	unitsSDK, _ := json.Marshal(callTool("confighub_units", unitsArguments, "sdk"))
	if !bytes.Contains(unitsCub, []byte(envelope.Unit.UnitID)) {
		t.Errorf("the MCP units tool by the cub route did not list the Unit")
	}
	if !bytes.Equal(unitsCub, unitsSDK) {
		t.Errorf("the MCP units tool answers differently by route; both are kept in %s", out)
	}

	// How long one read takes by each route, here, against this server. The
	// SDK figure includes resolving the credential, as each read does.
	const runs = 15
	var cubTimes, sdkTimes, sdkByIDTimes []time.Duration
	for i := 0; i < runs; i++ {
		start := time.Now()
		if _, err := cubStdoutOnly(getArgs...); err != nil {
			t.Fatal(err)
		}
		cubTimes = append(cubTimes, time.Since(start))

		start = time.Now()
		fresh, err := hubread.Resolve(ctx, hubread.Options{UserAgent: "cub-scout-integration"})
		if err == nil {
			_, err = fresh.UnitJSON(ctx, space, unit)
		}
		if err != nil {
			t.Fatal(err)
		}
		sdkTimes = append(sdkTimes, time.Since(start))

		start = time.Now()
		fresh, err = hubread.Resolve(ctx, hubread.Options{UserAgent: "cub-scout-integration"})
		if err == nil {
			_, err = fresh.UnitJSON(ctx, envelope.Unit.SpaceID, unit)
		}
		if err != nil {
			t.Fatal(err)
		}
		sdkByIDTimes = append(sdkByIDTimes, time.Since(start))
	}
	timings := map[string]interface{}{
		"runs":                         runs,
		"unit":                         "milliseconds per read, wall clock, on this machine against this server",
		"cub_unit_get_process":         milliseconds(cubTimes),
		"sdk_reader_space_by_slug":     milliseconds(sdkTimes),
		"sdk_reader_space_by_id":       milliseconds(sdkByIDTimes),
		"sdk_requests_space_by_slug":   2,
		"sdk_requests_space_by_id":     1,
		"bytes_equal_to_cub":           bytes.Equal(viaCub, viaSDK),
		"mcp_result_equal_by_route":    bytes.Equal(cubResult, sdkResult),
		"lists_bytes_equal_to_cub":     listsEqual,
		"mcp_units_equal_by_route":     bytes.Equal(unitsCub, unitsSDK),
		"unit_get_bytes":               len(viaCub),
		"sdk_reader_includes_resolve":  true,
		"cub_process_includes_startup": true,
	}
	encoded, _ := json.MarshalIndent(timings, "", "  ")
	keep("timings.json", append(encoded, '\n'))
	t.Logf("timings: %s", encoded)

	if version, err := cubStdoutOnly("version"); err == nil {
		keep("cub-version.txt", version)
	}
}
