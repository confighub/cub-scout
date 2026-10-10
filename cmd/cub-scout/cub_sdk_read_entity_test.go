package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/internal/hubread"
	"github.com/stretchr/testify/require"
)

const governedRevisionID = "5af31398-fd3d-4ac0-b267-4549eaabef37"

// The reads of one thing: only the exact shapes cub-scout builds are taken.
func TestSDKEntityArgsTakeOnlyTheCommandsTheyReproduce(t *testing.T) {
	taken := map[string][]string{
		"unit data":                 {"unit", "data", "u", "--space", "s"},
		"unit data, space first":    {"unit", "data", "--space", "s", "u"},
		"unit data, space by ID":    {"unit", "data", "u", "--space", recordedSpaceID},
		"revision get":              {"revision", "get", "u", "2", "--space", "s", "-o", "json"},
		"revision get, quiet":       {"revision", "get", "u", "12", "-o", "json", "--quiet", "--space", "s"},
		"revision data":             {"revision", "data", "u", "2", "--space", "s"},
		"view get, every space":     {"view", "get", recordedViewID, "--space", allConfigHubSpaces, "-o", "json"},
		"view get, a named space":   {"view", "get", recordedViewID, "--space", "s", "-o", "json"},
		"changeorder get":           {"changeorder", "get", "bump-image", "-o", "json", "--space", "s"},
		"changeorder get, by space": {"changeorder", "get", "bump-image", "-o", "json", "--space", recordedSpaceID},
	}
	for name, args := range taken {
		require.NotNil(t, sdkRead(args), "%s: cub %s", name, strings.Join(args, " "))
	}
	left := map[string][]string{
		"unit data of many":             {"unit", "data", "--space", "s", "--where", "Slug LIKE 'a-%'"},
		"unit data to a file":           {"unit", "data", "u", "--space", "s", "-O", "f"},
		"unit data as JSON":             {"unit", "data", "u", "--space", "s", "-o", "json"},
		"unit data, no space":           {"unit", "data", "u"},
		"unit data, every space":        {"unit", "data", "u", "--space", allConfigHubSpaces},
		"unit data, a unit by ID":       {"unit", "data", recordedUnitID, "--space", "s"},
		"unit data, a qualified unit":   {"unit", "data", "s/u", "--space", "s"},
		"unit data, two units":          {"unit", "data", "u", "v", "--space", "s"},
		"unit data, two spaces":         {"unit", "data", "u", "--space", "s", "--space", "t"},
		"unit data, a flag as a space":  {"unit", "data", "u", "--space", "--quiet"},
		"revision get, no number":       {"revision", "get", "u", "--space", "s", "-o", "json"},
		"revision get, relative":        {"revision", "get", "u", "-1", "--space", "s", "-o", "json"},
		"revision get, zero":            {"revision", "get", "u", "0", "--space", "s", "-o", "json"},
		"revision get, a plus sign":     {"revision", "get", "u", "+2", "--space", "s", "-o", "json"},
		"revision get, leading zero":    {"revision", "get", "u", "02", "--space", "s", "-o", "json"},
		"revision get, a name":          {"revision", "get", "u", "HeadRevisionNum", "--space", "s", "-o", "json"},
		"revision get, as a table":      {"revision", "get", "u", "2", "--space", "s"},
		"revision get, a selection":     {"revision", "get", "u", "2", "--space", "s", "-o", "json", "--select", "RevisionNum"},
		"revision get, every space":     {"revision", "get", "u", "2", "--space", allConfigHubSpaces, "-o", "json"},
		"revision get, a unit by ID":    {"revision", "get", recordedUnitID, "2", "--space", "s", "-o", "json"},
		"revision data as JSON":         {"revision", "data", "u", "2", "--space", "s", "-o", "json"},
		"revision data to a file":       {"revision", "data", "u", "2", "--space", "s", "--filename", "f"},
		"revision list":                 {"revision", "list", "u", "--space", "s", "-o", "json"},
		"view get, by slug":             {"view", "get", "my-view", "--space", "s", "-o", "json"},
		"view get, capitals":            {"view", "get", strings.ToUpper(recordedViewID), "--space", "s", "-o", "json"},
		"view get, the nil ID":          {"view", "get", "00000000-0000-0000-0000-000000000000", "--space", "s", "-o", "json"},
		"view get, no space":            {"view", "get", recordedViewID, "-o", "json"},
		"view get, as a table":          {"view", "get", recordedViewID, "--space", "s"},
		"view get, a qualified space":   {"view", "get", recordedViewID, "--space", "a/b", "-o", "json"},
		"changeorder get, by ID":        {"changeorder", "get", recordedUnitID, "-o", "json", "--space", "s"},
		"changeorder get, every space":  {"changeorder", "get", "bump-image", "-o", "json", "--space", allConfigHubSpaces},
		"changeorder get, with images":  {"changeorder", "get", "bump-image", "-o", "json", "--space", "s", "--container-images"},
		"changeorder get, as a table":   {"changeorder", "get", "bump-image", "--space", "s"},
		"changeorder get, a qualified":  {"changeorder", "get", "s/bump-image", "-o", "json", "--space", "s"},
		"changeorder list":              {"changeorder", "list", "-o", "json", "--space", "s"},
		"an entity this does not read":  {"trigger", "get", "t", "-o", "json", "--space", "s"},
		"a write that looks like a get": {"unit", "update", "u", "--space", "s"},
	}
	for name, args := range left {
		require.Nil(t, sdkRead(args), "%s: cub %s", name, strings.Join(args, " "))
	}
}

// Every place production code builds one of these reads builds a shape the
// route takes, or is named here with the reason it stays with cub.
func TestEveryEntityReadCallSiteIsTakenOrExplained(t *testing.T) {
	sites := cubCallSites(t, "unit", "data")
	require.GreaterOrEqual(t, len(sites), 4)
	for _, site := range sites {
		require.NotNil(t, sdkRead(site.args), "%s: cub %s", site.where, strings.Join(site.args, " "))
	}

	require.NotNil(t, sdkRead(changeOrderGetArgs(changeOrderReadQuery{Space: "space", Order: "bump-image"})))
	require.Nil(t, sdkRead(changeOrderGetArgs(changeOrderReadQuery{Space: "space", Order: recordedUnitID})), "a change order named by ID stays with cub")

	// The view and revision reads are built with the caller's values in
	// place, so each is rebuilt here as its call site builds it.
	require.NotNil(t, sdkRead([]string{"view", "get", recordedViewID, "--space", allConfigHubSpaces, "-o", "json"}))
	require.NotNil(t, sdkRead([]string{"revision", "get", "unit", "7", "--space", recordedSpaceID, "-o", "json"}))
	require.NotNil(t, sdkRead([]string{"revision", "data", "unit", "7", "--space", recordedSpaceID}))
	for command, want := range map[[2]string]int{{"view", "get"}: 1, {"revision", "get"}: 1, {"revision", "data"}: 1, {"changeorder", "get"}: 1} {
		require.Equal(t, want, countCubSpellings(t, command[0], command[1]), "a cub %s %s is built somewhere this test does not model", command[0], command[1])
	}
}

// countCubSpellings counts the places non-test code spells a cub command's
// two words as adjacent string literals.
func countCubSpellings(t *testing.T, entity, verb string) int {
	t.Helper()
	count := 0
	for _, root := range []string{".", filepath.Join("..", "..", "pkg"), filepath.Join("..", "..", "internal")} {
		require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			count += strings.Count(string(source), `"`+entity+`", "`+verb+`"`)
			return nil
		}))
	}
	return count
}

// governedHub serves the Unit recorded on a real ConfigHub v0.8.3 server, its
// revision, their data, a change order and a View, at the API's paths. Data is
// served as bytes, not JSON, as the server serves it.
func governedHub(t *testing.T) (url string, seen *[]string, read func(name string) []byte) {
	t.Helper()
	read = func(name string) []byte {
		dir := "confighub-governance-v083-recorded"
		if name == "view-get.json" {
			dir = "confighub-views-v083-recorded"
		}
		recorded, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", dir, name))
		require.NoError(t, err)
		return recorded
	}
	var unit map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(read("unit-get.json"), &unit))
	list := func(entries ...[]byte) []byte {
		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			parts = append(parts, string(entry))
		}
		return []byte("[" + strings.Join(parts, ",") + "]")
	}
	space, _ := json.Marshal(map[string]json.RawMessage{"Space": unit["Space"]})
	underUnit := "/api/space/" + recordedSpaceID + "/unit/" + recordedUnitID
	jsonBodies := map[string][]byte{
		"/api/space":            list(space),
		"/api/unit":             list(read("unit-get.json")),
		underUnit + "/revision": list(read("revision-get.json")),
		"/api/change_order":     list(read("changeorder-get.json")),
		"/api/view":             list(read("view-get.json")),
	}
	dataBodies := map[string][]byte{
		underUnit + "/data": read("unit-data.txt"),
		underUnit + "/revision/" + governedRevisionID + "/data": read("revision-data.txt"),
	}
	seen = &[]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Method+" "+r.URL.Path)
		if body, ok := dataBodies[r.URL.Path]; ok {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if body, ok := jsonBodies[r.URL.Path]; ok {
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"Message":"not found"}`))
	}))
	t.Cleanup(server.Close)
	return server.URL, seen, read
}

// #758, batches B and C: through cubStdout, by each route. The fake cub
// prints the recording for each command; the test server sends what a real
// server sent for it. Data is equal byte for byte. The JSON reads are equal
// in every value cub printed (the recordings are from an older cub than the
// SDK this is built with; see the reader's tests).
func TestCubStdoutAnswersTheEntityReadsThroughTheSDK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	url, seen, read := governedHub(t)
	fixture := func(name string) string {
		path, err := filepath.Abs(filepath.Join("..", "..", "test", "fixtures", "confighub-governance-v083-recorded", name))
		require.NoError(t, err)
		return path
	}
	dir := t.TempDir()
	cubLog := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" + "echo \"$*\" >> \"" + cubLog + "\"\n" + "case \"$1 $2\" in\n" +
		"  \"unit data\") /bin/cat \"" + fixture("unit-data.txt") + "\" ;;\n" +
		"  \"revision data\") /bin/cat \"" + fixture("revision-data.txt") + "\" ;;\n" +
		"  \"revision get\") /bin/cat \"" + fixture("revision-get.json") + "\" ;;\n" +
		"  \"changeorder get\") /bin/cat \"" + fixture("changeorder-get.json") + "\" ;;\n" +
		"  *) exit 9 ;;\nesac\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755))
	t.Setenv("PATH", dir)
	old := sdkReader
	t.Cleanup(func() { sdkReader = old })
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return hubread.New(url, "test-token", hubread.Options{})
	}

	for _, tc := range []struct {
		name     string
		args     []string
		requests int
		exact    bool
	}{
		{"unit data", withConfigHubSpace([]string{"unit", "data", recordedUnit}, recordedSpace), 3, true},
		{"revision data", []string{"revision", "data", recordedUnit, "2", "--space", recordedSpaceID}, 3, true},
		{"revision get", []string{"revision", "get", recordedUnit, "2", "--space", recordedSpaceID, "-o", "json"}, 2, false},
		{"changeorder get", changeOrderGetArgs(changeOrderReadQuery{Space: recordedSpace, Order: "contract-change"}), 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cubBefore, seenBefore := len(fakeCubCalls(t, cubLog)), len(*seen)
			t.Setenv(configHubReaderEnv, "cub")
			viaCub, err := cubStdout(context.Background(), tc.args...)
			require.NoError(t, err)
			require.Len(t, fakeCubCalls(t, cubLog), cubBefore+1)
			require.Len(t, *seen, seenBefore, "by the cub route nothing is read through the SDK")

			t.Setenv(configHubReaderEnv, "sdk")
			viaSDK, err := cubStdout(context.Background(), tc.args...)
			require.NoError(t, err)
			require.Len(t, fakeCubCalls(t, cubLog), cubBefore+1, "by the SDK route no cub process is started")
			require.Len(t, (*seen)[seenBefore:], tc.requests)
			for _, request := range (*seen)[seenBefore:] {
				require.True(t, strings.HasPrefix(request, "GET "), request)
			}
			if tc.exact {
				require.Equal(t, string(viaCub), string(viaSDK))
				return
			}
			// Everything cub printed, unchanged. The newer SDK adds one
			// field the older cub did not have, printed as the zero ID;
			// that field alone is taken out before the two are compared.
			var want, have interface{}
			require.NoError(t, json.Unmarshal(viaCub, &want))
			require.NoError(t, json.Unmarshal(viaSDK, &have))
			require.Equal(t, want, withoutZeroReleaseTag(have))
		})
	}

	// The View, which cub-scout asks for by ID in every space.
	t.Setenv(configHubReaderEnv, "sdk")
	before := len(*seen)
	view, err := fetchView(context.Background(), recordedViewID, allConfigHubSpaces)
	require.NoError(t, err)
	require.Len(t, (*seen)[before:], 1)
	require.Equal(t, string(read("view-get.json")), mustIndent(t, view), "the View's JSON, decoded and encoded again, is what cub printed")
	columns, err := extractColumnsSpec(view)
	require.NoError(t, err)
	require.NotEmpty(t, columns)
}

// withoutZeroReleaseTag removes a ReleaseTagID that is the zero ID, at any
// depth, and nothing else.
func withoutZeroReleaseTag(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		if typed["ReleaseTagID"] == "00000000-0000-0000-0000-000000000000" {
			delete(typed, "ReleaseTagID")
		}
		for key, inner := range typed {
			typed[key] = withoutZeroReleaseTag(inner)
		}
	case []interface{}:
		for i, inner := range typed {
			typed[i] = withoutZeroReleaseTag(inner)
		}
	}
	return value
}

// mustIndent encodes a decoded JSON object the way cub prints one.
func mustIndent(t *testing.T, value map[string]interface{}) string {
	t.Helper()
	encoded, err := json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	return string(encoded) + "\n"
}

// The reads behind attestation evidence use the SDK route under the same
// byte limit, and say no more about a failure than the cub route does.
func TestBoundedAttestationReadsUseTheSDKRouteUnderTheSameLimit(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	url, seen, read := governedHub(t)
	old := sdkReader
	t.Cleanup(func() { sdkReader = old })
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return hubread.New(url, "test-token", hubread.Options{})
	}
	t.Setenv(configHubReaderEnv, "sdk")
	ctx := context.Background()

	data, err := boundedAttestationCubRead(ctx, "revision", "data", recordedUnit, "2", "--space", recordedSpaceID)
	require.NoError(t, err)
	require.Equal(t, string(read("revision-data.txt")), string(data))
	revision, err := boundedAttestationCubRead(ctx, "revision", "get", recordedUnit, "2", "--space", recordedSpaceID, "-o", "json")
	require.NoError(t, err)
	require.Contains(t, string(revision), governedRevisionID)
	require.Empty(t, fakeCubCalls(t, cubLog), "no cub process for either read")
	require.NotEmpty(t, *seen)

	// Anything but the three permitted reads is refused before either route.
	before := len(*seen)
	_, err = boundedAttestationCubRead(ctx, "unit", "data", recordedUnit, "--space", recordedSpaceID)
	require.ErrorContains(t, err, "permits only")
	require.Len(t, *seen, before)

	// A failure says what the cub route's says, and nothing about the server.
	_, err = boundedAttestationCubRead(ctx, "revision", "data", "no-such-unit", "2", "--space", recordedSpaceID)
	require.EqualError(t, err, "bounded ConfigHub read failed")
	require.Empty(t, fakeCubCalls(t, cubLog), "a failed SDK read does not fall back to cub")

	// More than the limit is refused, as it is from cub, and the read stops
	// when the limit is passed: the rest is not taken from the network.
	sent := 0
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/space" {
			_, _ = w.Write([]byte(`[{"Space":{"Slug":"s","SpaceID":"` + recordedSpaceID + `"}}]`))
			return
		}
		entry := `{"Attestation":{"SpaceID":"` + recordedSpaceID + `","Note":"` + strings.Repeat("n", 2048) + `"}},`
		_, _ = w.Write([]byte("["))
		for i := 0; i < 20000; i++ { // about 42 MB if nothing stops it
			if _, err := w.Write([]byte(entry)); err != nil {
				return
			}
			sent += len(entry)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		_, _ = w.Write([]byte(`{"Attestation":{"SpaceID":"` + recordedSpaceID + `"}}]`))
	}))
	t.Cleanup(big.Close)
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return hubread.New(big.URL, "test-token", hubread.Options{})
	}
	_, err = boundedAttestationCubRead(ctx, "attestation", "list", "--space", recordedSpaceID, "-o", "json")
	require.EqualError(t, err, "ConfigHub read exceeds 1 MiB")
	big.Close() // wait for the handler, so that sent is final
	require.Less(t, sent, 8*1024*1024, "the server was cut off well before the 42 MB it had to send")
}

// A failed read on the SDK route is not worded as a cub command that failed:
// no cub ran.
func TestEntityReadFailuresAreNotWordedAsCubFailuresOnTheSDKRoute(t *testing.T) {
	onlyFakeCub(t, "3")
	hub := newRecordedHub(t, http.StatusForbidden)
	sdkReadsFrom(t, hub)

	t.Setenv(configHubReaderEnv, "sdk")
	_, err := fetchView(context.Background(), recordedViewID, allConfigHubSpaces)
	require.ErrorContains(t, err, "ConfigHub view read failed")
	require.NotContains(t, err.Error(), "cub view get failed")
	require.Equal(t, hubread.KindForbidden, hubread.KindOf(err))

	t.Setenv(configHubReaderEnv, "cub")
	_, err = fetchView(context.Background(), recordedViewID, allConfigHubSpaces)
	require.ErrorContains(t, err, "cub view get failed")
}
