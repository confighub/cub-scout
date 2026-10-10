//go:build integration
// +build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

// serverHost is the host name of a server URL, or the text itself when it is
// not a URL.
func serverHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return raw
	}
	return strings.ToLower(parsed.Hostname())
}

// workerSecretLine is the line of `cub worker list -o json` that holds a
// worker's Secret. It is never the last field of its object, so removing the
// whole line leaves valid JSON.
var workerSecretLine = regexp.MustCompile(`(?m)^[ \t]*"Secret": "[^"\n]*",\n`)

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
	// The disposable server is on the runner itself. A server anywhere else
	// is not one this test may write to, whatever it is called.
	if host := serverHost(reader.Server()); host != "localhost" && host != "127.0.0.1" && host != "::1" {
		t.Fatalf("refusing to run: the resolved server %q is not on this machine", host)
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
		if err != nil && name == "unit-list-contains" {
			// ConfigHub v0.8.3 answered a text search on Units with HTTP
			// 500, to cub itself; v0.8.12 answers it. Where a server
			// cannot answer, the routes agree by both failing, and both
			// failures are kept.
			keep(name+".cub.txt", []byte("error: "+err.Error()+"\n"))
			listedBySDK, sdkErr := list.read()
			if sdkErr == nil {
				t.Errorf("%s: cub failed (%v) and the SDK reader returned a list: %s", name, err, listedBySDK)
				continue
			}
			keep(name+".sdk.txt", []byte("error: "+sdkErr.Error()+"\n"))
			delete(listsEqual, name)
			t.Logf("%s: the server answered neither route; cub: %v; reader: %v", name, err, sdkErr)
			continue
		}
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
	// The other lists of a space (#758, batch A). Each entity is created
	// through cub where this server lets it be, so that the list compared has
	// something in it; a list that stays empty is still compared, and the
	// artifact says which were which.
	created := map[string]string{}
	for kind, args := range map[string][]string{
		"worker":      {"worker", "create", "--space", space, "parity-worker"},
		"target":      {"target", "create", "--space", space, "parity-target"},
		"changeset":   {"changeset", "create", "--space", space, "parity-changeset", "--description", "recorded for the SDK reader"},
		"link":        {"link", "create", "--space", space, "parity-link", unit, other},
		"attestation": {"attestation", "create", "--space", space, "--type", "SecurityReview", "--note", "recorded for the SDK reader"},
	} {
		if _, err := cubStdoutOnly(args...); err != nil {
			created[kind] = "not created: " + err.Error()
			t.Logf("%s could not be created on this server, so its list is compared empty: %v", kind, err)
			continue
		}
		created[kind] = "created"
	}
	spaceLists := map[string]func(string, hubread.Filter) ([]byte, error){
		"worker":      func(s string, f hubread.Filter) ([]byte, error) { return reader.WorkerListJSON(ctx, s, f) },
		"target":      func(s string, f hubread.Filter) ([]byte, error) { return reader.TargetListJSON(ctx, s, f) },
		"changeset":   func(s string, f hubread.Filter) ([]byte, error) { return reader.ChangeSetListJSON(ctx, s, f) },
		"link":        func(s string, f hubread.Filter) ([]byte, error) { return reader.LinkListJSON(ctx, s, f) },
		"attestation": func(s string, f hubread.Filter) ([]byte, error) { return reader.AttestationListJSON(ctx, s, f) },
	}
	listSizes := map[string]int{}
	for kind, read := range spaceLists {
		listedByCub, err := cubStdoutOnly(kind, "list", "--space", space, "-o", "json")
		if err != nil {
			t.Errorf("cub %s list: %v", kind, err)
			continue
		}
		if kind == "worker" {
			// cub prints each worker's Secret, the token it authenticates
			// with. The reader drops it, and it is not kept in an artifact
			// anyone can download: the comparison is with cub's output
			// less that one line.
			withSecret := len(listedByCub)
			listedByCub = workerSecretLine.ReplaceAll(listedByCub, nil)
			if created["worker"] == "created" && len(listedByCub) == withSecret {
				t.Logf("cub's worker list carried no Secret line on this server")
			}
			if bytes.Contains(listedByCub, []byte(`"Secret"`)) {
				t.Fatalf("a worker's Secret is still in cub's list after the line was removed; nothing is kept")
			}
		}
		keep(kind+"-list.cub.json", listedByCub)
		var entries []json.RawMessage
		if err := json.Unmarshal(listedByCub, &entries); err != nil {
			t.Errorf("cub %s list is not a JSON list: %v", kind, err)
			continue
		}
		listSizes[kind] = len(entries)
		for how, named := range map[string]string{"slug": space, "id": envelope.Unit.SpaceID} {
			listedBySDK, err := read(named, hubread.Filter{})
			if err != nil {
				t.Errorf("%s list through the SDK, space by %s: %v (kind %s)", kind, how, err, hubread.KindOf(err))
				continue
			}
			if bytes.Contains(listedBySDK, []byte(`"Secret"`)) {
				t.Fatalf("the reader's %s list carries a Secret; it is not kept", kind)
			}
			if how == "slug" {
				keep(kind+"-list.sdk.json", listedBySDK)
			}
			listsEqual[kind+"-list-by-"+how] = bytes.Equal(listedByCub, listedBySDK)
			if !bytes.Equal(listedByCub, listedBySDK) {
				keep(kind+"-list.sdk-by-"+how+".json", listedBySDK)
				t.Errorf("%s list, space by %s: the SDK reader's JSON is not what cub printed: %d bytes against %d; both are kept in %s", kind, how, len(listedBySDK), len(listedByCub), out)
			}
		}
		// A filter that matches nothing is an empty list by both routes.
		const nothing = "Slug = 'no-such-entity'"
		if kind != "attestation" {
			filteredByCub, cubErr := cubStdoutOnly(kind, "list", "--space", space, "-o", "json", "--where", nothing)
			filteredBySDK, sdkErr := read(space, hubread.Filter{Where: nothing})
			if (cubErr == nil) != (sdkErr == nil) || (cubErr == nil && !bytes.Equal(filteredByCub, filteredBySDK)) {
				t.Errorf("%s list with a filter: cub %q (%v), the reader %q (%v)", kind, filteredByCub, cubErr, filteredBySDK, sdkErr)
			}
		}
		// In a space that does not exist, neither route lists anything.
		if listed, err := read("no-such-space-"+space, hubread.Filter{}); hubread.KindOf(err) != hubread.KindNotFound || listed != nil {
			t.Errorf("%s list in a space that does not exist: kind %s, want %s (%v)", kind, hubread.KindOf(err), hubread.KindNotFound, err)
		}
	}
	summary, _ := json.MarshalIndent(map[string]interface{}{"created": created, "entries": listSizes}, "", "  ")
	keep("space-lists.json", append(summary, '\n'))
	t.Logf("space lists: %s", summary)

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
	if !bytes.Equal(filteredCub, filteredSDK) {
		t.Errorf("the MCP units tool with a filter differs by route")
	}
	// The filtered answer has the Unit asked for and not the other one.
	// Equal answers alone would also hold for two empty lists or two errors.
	if !bytes.Contains(filteredSDK, []byte(envelope.Unit.UnitID)) || bytes.Contains(filteredSDK, []byte(`\"Slug\": \"`+other+`\"`)) {
		t.Errorf("the MCP units tool with a filter did not return exactly the Unit it names: %s", filteredSDK)
	}

	// Recorded, not asserted: what each route does with an OR. cub's help
	// documents AND only; whether the server accepts OR, and how it reads it
	// beside the space, is not known here (#758).
	const alternative = "Slug = '" + unit + "' OR Slug = '" + other + "'"
	if listed, err := cubStdoutOnly("unit", "list", "--space", space, "-o", "json", "--where", alternative); err != nil {
		keep("unit-list-or-filter.cub.txt", []byte("error: "+err.Error()+"\n"))
	} else {
		keep("unit-list-or-filter.cub.txt", listed)
	}
	if listed, err := reader.UnitListJSON(ctx, space, hubread.UnitFilter{Where: alternative}); err != nil {
		keep("unit-list-or-filter.sdk.txt", []byte("error: "+err.Error()+"\n"))
	} else {
		keep("unit-list-or-filter.sdk.txt", listed)
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

// findString returns the first string value stored under key anywhere in a
// decoded JSON value.
func findString(value interface{}, key string) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		if found, ok := typed[key].(string); ok && found != "" {
			return found
		}
		for _, inner := range typed {
			if found := findString(inner, key); found != "" {
				return found
			}
		}
	case []interface{}:
		for _, inner := range typed {
			if found := findString(inner, key); found != "" {
				return found
			}
		}
	}
	return ""
}

// #852: what a real View looks like, and what `views project` makes of it.
//
// The view commands were written and tested against JSON nobody recorded.
// This creates a Filter and a View on the disposable server, keeps what cub
// prints for them and what cub-scout prints for the View, and holds `views
// project` to the server: the View's own columns, and in each cell the value
// ConfigHub evaluated for it.
func TestRecordViewShapesOnARealServer(t *testing.T) {
	if os.Getenv(disposableConfigHubEnv) != "1" {
		t.Skipf("writes a space, Units, a Filter and a View; set %s=1 only for a disposable ConfigHub server", disposableConfigHubEnv)
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
	out = filepath.Join(out, "views")
	if err := os.MkdirAll(out, 0o700); err != nil {
		t.Fatal(err)
	}
	// record runs a command and keeps its standard output, or what went
	// wrong, under name. It returns the output and whether the command
	// succeeded.
	record := func(name, binary string, args ...string) ([]byte, bool) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		stdout, err := cmd.Output()
		kept := stdout
		if err != nil {
			kept = []byte(fmt.Sprintf("command: %s %s\nerror: %v\nstderr:\n%s\nstdout:\n%s", filepath.Base(binary), strings.Join(args, " "), err, stderr.String(), stdout))
			name += ".failed.txt"
		}
		if writeErr := os.WriteFile(filepath.Join(out, name), kept, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		t.Logf("%s: %d bytes, ok=%v", name, len(kept), err == nil)
		return stdout, err == nil
	}

	space := fmt.Sprintf("scout-view-shapes-%d", time.Now().UnixNano())
	if _, err := cubStdoutOnly("space", "create", space); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTestSpace(t, space) })
	// The third Unit does not match the View's filter.
	for _, name := range []string{"shape-one", "shape-two", "other-three"} {
		path := filepath.Join(t.TempDir(), name+".yaml")
		body := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: " + name + "\n  namespace: default\ndata:\n  key: value\n"
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := cubStdoutOnly("unit", "create", "--space", space, "--label", "tier=recorded", name, path); err != nil {
			t.Logf("a labelled Unit could not be created (%v); creating it without the label", err)
			if _, err := cubStdoutOnly("unit", "create", "--space", space, name, path); err != nil {
				t.Fatal(err)
			}
		}
	}

	record("filter-create.json", "cub", "filter", "create", "--space", space, "-o", "json", "shape-filter", "Unit", "--where-field", "Slug LIKE 'shape-%'")
	record("filter-get.json", "cub", "filter", "get", "shape-filter", "--space", space, "-o", "json")
	created, createdOK := record("view-create.json", "cub", "view", "create", "--space", space, "-o", "json", "shape-view", "shape-filter",
		"--column", "Unit.Slug", "--column", "Unit.DisplayName", "--column", "Unit.HeadRevisionNum", "--column", "Space.Slug", "--column", "Labels.tier")
	got, gotOK := record("view-get.json", "cub", "view", "get", "shape-view", "--space", space, "-o", "json")
	if !createdOK || !gotOK {
		t.Fatalf("the View could not be created and read back (created %v, read %v); nothing after this would be checked", createdOK, gotOK)
	}

	// Recorded, not asserted: how ConfigHub returns a column that has no
	// value for a Unit. No Unit here has the label this column names.
	if _, ok := record("view-absent-create.json", "cub", "view", "create", "--space", space, "-o", "json", "shape-view-absent", "shape-filter",
		"--column", "Unit.Slug", "--column", "Labels.absent"); ok {
		record("unit-list-with-view-absent-label.json", "cub", "unit", "list", "--space", space, "-o", "json", "--view", "shape-view-absent")
	}
	record("view-list.json", "cub", "view", "list", "--space", space, "-o", "json")
	record("unit-list-labelled.json", "cub", "unit", "list", "--space", space, "-o", "json")
	record("unit-list-with-view.json", "cub", "unit", "list", "--space", space, "-o", "json", "--view", "shape-view")
	record("unit-list-with-filter.json", "cub", "unit", "list", "--space", space, "-o", "json", "--filter", "shape-filter")
	record("unit-list-with-view-and-where.json", "cub", "unit", "list", "--space", space, "-o", "json", "--view", "shape-view", "--where", "Slug LIKE 'shape-%'")

	var decoded interface{}
	viewID := ""
	for _, candidate := range [][]byte{got, created} {
		if json.Unmarshal(candidate, &decoded) == nil {
			if viewID = findString(decoded, "ViewID"); viewID != "" {
				break
			}
		}
	}
	if viewID == "" {
		t.Fatalf("no ViewID in what cub printed for the View; cub-scout's view commands cannot be checked")
	}
	record("unit-list-with-view-every-space.json", "cub", "unit", "list", "--space", "*", "-o", "json", "--view", viewID, "--where", "Slug LIKE 'shape-%'")

	// What ConfigHub evaluated for each Unit the filter matches: the answer
	// `views project` is held to.
	listed, ok := record("unit-list-with-view-by-id.json", "cub", "unit", "list", "--space", space, "-o", "json", "--view", viewID, "--where", "Slug LIKE 'shape-%'")
	if !ok {
		t.Fatalf("cub could not list the Units with the View and its filter together; that is the request views project makes")
	}
	var entries []struct {
		Unit        struct{ Slug string }
		ViewColumns []struct{ Name, Value string }
	}
	if err := json.Unmarshal(listed, &entries); err != nil {
		t.Fatalf("parse cub's list with the View: %v", err)
	}
	wantColumns := []string{"Unit.Slug", "Unit.DisplayName", "Unit.HeadRevisionNum", "Space.Slug", "Labels.tier"}
	wantRows := map[string]map[string]string{}
	for _, entry := range entries {
		cells := map[string]string{}
		for _, column := range entry.ViewColumns {
			cells[column.Name] = column.Value
		}
		wantRows[entry.Unit.Slug] = cells
		if len(cells) != len(wantColumns) || cells["Unit.Slug"] != entry.Unit.Slug || cells["Labels.tier"] != "recorded" {
			t.Errorf("ConfigHub's own column values for %s are not what the View asks for: %v", entry.Unit.Slug, cells)
		}
	}
	if len(wantRows) != 2 || wantRows["shape-one"] == nil || wantRows["shape-two"] == nil {
		t.Fatalf("cub listed %d Units for the View's filter, want shape-one and shape-two", len(wantRows))
	}

	binary := getCubAgentPath()
	for name, args := range map[string][]string{
		"scout-views-project":                     {"views", "project", viewID, "--space", space, "--format", "json"},
		"scout-views-project-every-space-checked": {"views", "project", viewID, "--format", "json"},
	} {
		printed, ok := record(name+".checked.json", binary, args...)
		if !ok {
			t.Errorf("%s: cub-scout %s failed", name, strings.Join(args, " "))
			continue
		}
		var projected struct {
			Columns   []struct{ Name string }
			Rows      []map[string]string
			Omissions []map[string]interface{}
		}
		if err := json.Unmarshal(printed, &projected); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		gotColumns := []string{}
		for _, column := range projected.Columns {
			gotColumns = append(gotColumns, column.Name)
		}
		if strings.Join(gotColumns, ",") != strings.Join(wantColumns, ",") {
			t.Errorf("%s: columns %v, want the View's own %v", name, gotColumns, wantColumns)
		}
		if len(projected.Omissions) != 0 {
			t.Errorf("%s: omissions %v, want none: ConfigHub evaluated every column", name, projected.Omissions)
		}
		if len(projected.Rows) != len(wantRows) {
			t.Errorf("%s: %d rows, want %d (the Unit the filter does not match is not one of them)", name, len(projected.Rows), len(wantRows))
		}
		for _, row := range projected.Rows {
			want := wantRows[row["Unit.Slug"]]
			if want == nil {
				t.Errorf("%s: a row for %q, which ConfigHub did not list", name, row["Unit.Slug"])
				continue
			}
			for _, column := range wantColumns {
				if row[column] != want[column] {
					t.Errorf("%s: %s of %s is %q, ConfigHub evaluated %q", name, column, row["Unit.Slug"], row[column], want[column])
				}
			}
		}
	}

	for name, args := range map[string][]string{
		"scout-views-resolve.json":              {"views", "resolve", viewID, "--space", space, "--format", "json"},
		"scout-views-resolve-every-space.json":  {"views", "resolve", viewID, "--format", "json"},
		"scout-views-project.json":              {"views", "project", viewID, "--space", space, "--format", "json"},
		"scout-views-project.txt":               {"views", "project", viewID, "--space", space},
		"scout-views-project-every-space.json":  {"views", "project", viewID, "--format", "json"},
		"scout-views-project-with-reality.json": {"views", "project", viewID, "--space", space, "--format", "json", "--with-reality"},
	} {
		record(name, binary, args...)
	}
}
