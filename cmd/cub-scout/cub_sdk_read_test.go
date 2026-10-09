// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/confighub/cub-scout/v2/internal/hubread"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

const (
	recordedSpace   = "scout-v213-contract"
	recordedUnit    = "contract-config"
	recordedSpaceID = "68843338-f9bd-485c-9a66-5d5aee820247"
	recordedUnitID  = "6221926e-8cc4-4847-a25c-a4104042f640"
)

// recordedUnitGetPath is what a real ConfigHub v0.8.3 server returned for one
// Unit, as `cub unit get -o json` printed it.
func recordedUnitGetPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "test", "fixtures", "confighub-governance-v083-recorded", "unit-get.json"))
	require.NoError(t, err)
	return path
}

func recordedUnitGet(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(recordedUnitGetPath(t))
	require.NoError(t, err)
	return data
}

// recordedHub is a server that answers the two list calls with the recorded
// object, or every call with status when status is not 0.
type recordedHub struct {
	url                  string
	seen, agents, tokens []string
}

func newRecordedHub(t *testing.T, status int) *recordedHub {
	t.Helper()
	var element map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recordedUnitGet(t), &element))
	hub := &recordedHub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.seen = append(hub.seen, r.Method+" "+r.URL.Path)
		hub.agents = append(hub.agents, r.Header.Get("User-Agent"))
		hub.tokens = append(hub.tokens, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"Message":"simulated"}`))
			return
		}
		switch r.URL.Path {
		case "/api/space":
			_ = json.NewEncoder(w).Encode([]map[string]json.RawMessage{{"Space": element["Space"]}})
		case "/api/unit":
			_ = json.NewEncoder(w).Encode([]map[string]json.RawMessage{element})
		}
	}))
	t.Cleanup(server.Close)
	hub.url = server.URL
	return hub
}

// sdkReadsFrom makes the SDK route read from hub, with a credential the test
// supplies. The production credential path has its own test below.
func sdkReadsFrom(t *testing.T, hub *recordedHub) {
	t.Helper()
	old := sdkReader
	t.Cleanup(func() { sdkReader = old })
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return hubread.New(hub.url, "test-token", hubread.Options{})
	}
}

// onlyFakeCub makes a fake cub the only cub that can run: it records its
// arguments and prints the recorded Unit, exiting with exit. With it on PATH a
// test can say whether a cub process was started, not only whether a seam was
// called.
func onlyFakeCub(t *testing.T, exit string) (logPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"" + logPath + "\"\n" +
		"/bin/cat \"" + recordedUnitGetPath(t) + "\"\n" +
		"exit " + exit + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755))
	t.Setenv("PATH", dir)
	return logPath
}

func recordedWorkload() *runtimeWorkload {
	return &runtimeWorkload{
		Labels:      map[string]string{"confighub.com/UnitSlug": recordedUnit},
		Annotations: map[string]string{"confighub.com/SpaceName": recordedSpace},
	}
}

func TestSDKUnitGetArgsTakesOnlyTheCommandItReproduces(t *testing.T) {
	taken := map[string][]string{
		"unit then flags":        {"unit", "get", "u", "-o", "json", "--space", "s"},
		"flags then unit":        {"unit", "get", "-o", "json", "u", "--space", "s"},
		"space first":            {"unit", "get", "--space", "s", "-o", "json", "u"},
		"quiet":                  {"unit", "get", "u", "-o", "json", "--quiet", "--space", "s"},
		"long output flag":       {"unit", "get", "u", "--output", "json", "--space", "s"},
		"qualified reference":    {"unit", "get", "-o", "json", "s/u"},
		"space given by its ID":  {"unit", "get", "u", "-o", "json", "--space", recordedSpaceID},
		"names are trimmed":      {"unit", "get", " u ", "-o", "json", "--space", " s "},
		"qualified, space by ID": {"unit", "get", "-o", "json", recordedSpaceID + "/u"},
		"placeholder space (cub's own refusal applies later)": {"unit", "get", "u", "-o", "json", "--space", unresolvedConfigHubSpace},
	}
	for name, args := range taken {
		t.Run("taken/"+name, func(t *testing.T) {
			space, unit, ok := sdkUnitGetArgs(args)
			require.True(t, ok)
			require.Equal(t, "u", unit)
			require.NotEmpty(t, space)
			require.Equal(t, strings.TrimSpace(space), space)
		})
	}

	left := map[string][]string{
		"another command":            {"unit", "list", "-o", "json", "--space", "s"},
		"another entity":             {"space", "get", "s", "-o", "json"},
		"no output format":           {"unit", "get", "u", "--space", "s"},
		"another output format":      {"unit", "get", "u", "-o", "yaml", "--space", "s"},
		"output flag with no value":  {"unit", "get", "u", "--space", "s", "-o"},
		"a flag this does not know":  {"unit", "get", "u", "-o", "json", "--space", "s", "--where", "Slug = 'x'"},
		"joined flag spelling":       {"unit", "get", "u", "-o=json", "--space", "s"},
		"joined space spelling":      {"unit", "get", "u", "-o", "json", "--space=s"},
		"two positionals":            {"unit", "get", "u", "v", "-o", "json", "--space", "s"},
		"no positional":              {"unit", "get", "-o", "json", "--space", "s"},
		"no space":                   {"unit", "get", "u", "-o", "json"},
		"empty space":                {"unit", "get", "u", "-o", "json", "--space", " "},
		"space flag with no value":   {"unit", "get", "u", "-o", "json", "--space"},
		"space named twice":          {"unit", "get", "u", "-o", "json", "--space", "s", "--space", "t"},
		"every space":                {"unit", "get", "u", "-o", "json", "--space", allConfigHubSpaces},
		"unit by ID":                 {"unit", "get", recordedUnitID, "-o", "json", "--space", "s"},
		"unit by ID, no space":       {"unit", "get", "-o", "json", recordedUnitID},
		"qualified unit by ID":       {"unit", "get", "-o", "json", "s/" + recordedUnitID},
		"qualified and a space flag": {"unit", "get", "-o", "json", "s/u", "--space", "t"},
		"reference with three parts": {"unit", "get", "-o", "json", "s/u/v"},
		"reference with no unit":     {"unit", "get", "-o", "json", "s/"},
		"reference with no space":    {"unit", "get", "-o", "json", "/u"},
		"too short":                  {"unit", "get"},
	}
	for name, args := range left {
		t.Run("left to cub/"+name, func(t *testing.T) {
			_, _, ok := sdkUnitGetArgs(args)
			require.False(t, ok)
		})
	}
}

// Every place production code builds a `unit get` must build one the adapter
// takes, or say here why it does not. A call site added later in a shape the
// adapter does not know fails this test instead of quietly staying on cub.
func TestEveryUnitGetCallSiteIsOneTheSDKRouteTakes(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	literalCall := regexp.MustCompile(`"unit", "get"((?:, *(?:"[^"]*"|[A-Za-z_.]+))*) *[})]`)
	sites := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		require.NoError(t, err)
		for number, line := range strings.Split(string(source), "\n") {
			if !strings.Contains(line, `"unit", "get"`) {
				continue
			}
			sites++
			match := literalCall.FindStringSubmatch(line)
			require.NotNil(t, match, "%s:%d builds a unit get this test cannot read: %s", file, number+1, strings.TrimSpace(line))
			args := []string{"unit", "get"}
			for _, token := range strings.Split(match[1], ",") {
				switch token = strings.TrimSpace(token); {
				case token == "":
				case strings.HasPrefix(token, `"`):
					args = append(args, strings.Trim(token, `"`))
				default:
					args = append(args, "name") // a variable: some slug
				}
			}
			switch {
			case strings.Contains(line, "withConfigHubSpaceFromRef("):
				args = withConfigHubSpaceFromRef(args, "space/name")
			case strings.Contains(line, "withConfigHubSpace("):
				args = withConfigHubSpace(args, "space")
			}
			space, unit, ok := sdkUnitGetArgs(args)
			require.True(t, ok, "%s:%d: cub %s", file, number+1, strings.Join(args, " "))
			require.Equal(t, "name", unit)
			require.Contains(t, []string{"space", "name"}, space) // hierarchy.go passes its space as a variable
		}
	}
	require.GreaterOrEqual(t, sites, 11, "the scan no longer finds the call sites")

	// The MCP tool chooses between three spellings.
	tool, ok := newMCPGatewayWithMode(nil, nil, true).tools["confighub_unit_get"]
	require.True(t, ok)
	for _, arguments := range []map[string]interface{}{
		{"unit": "name", "space": "space"},
		{"unit": "space/name"},
	} {
		args, err := tool.BuildArgs(arguments)
		require.NoError(t, err)
		space, unit, ok := sdkUnitGetArgs(args)
		require.True(t, ok, "cub %s", strings.Join(args, " "))
		require.Equal(t, "space", space)
		require.Equal(t, "name", unit)
	}
	// A Unit named by ID is found in any space; that question stays with cub.
	args, err := tool.BuildArgs(map[string]interface{}{"unit": recordedUnitID})
	require.NoError(t, err)
	_, _, ok = sdkUnitGetArgs(args)
	require.False(t, ok)
}

// #758: under the SDK route a `unit get` returns what cub printed for the same
// Unit, and no cub process is started.
func TestCubStdoutAnswersUnitGetThroughTheSDKWhenAsked(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	sdkReadsFrom(t, hub)
	args := []string{"unit", "get", recordedUnit, "-o", "json", "--space", recordedSpace}

	t.Setenv(configHubReaderEnv, "")
	viaCub, err := cubStdout(context.Background(), args...)
	require.NoError(t, err)
	require.Len(t, fakeCubCalls(t, cubLog), 1)
	require.Empty(t, hub.seen, "without the setting nothing is read through the SDK")

	t.Setenv(configHubReaderEnv, "sdk")
	viaSDK, err := cubStdout(context.Background(), args...)
	require.NoError(t, err)
	require.Len(t, fakeCubCalls(t, cubLog), 1, "with the setting no cub process is started")
	require.Equal(t, []string{"GET /api/space", "GET /api/unit"}, hub.seen)
	require.JSONEq(t, string(viaCub), string(viaSDK))
	require.True(t, strings.HasSuffix(string(viaSDK), "}\n"))

	// The route name is case-insensitive, and "cub" is the default spelt out.
	t.Setenv(configHubReaderEnv, " CUB ")
	_, err = cubStdout(context.Background(), args...)
	require.NoError(t, err)
	require.Len(t, fakeCubCalls(t, cubLog), 2)

	// A nil context is what several callers pass; it must not panic.
	t.Setenv(configHubReaderEnv, "SDK")
	//nolint:staticcheck // the nil context is the case under test
	_, err = cubStdout(nil, args...)
	require.NoError(t, err)
	require.Len(t, hub.seen, 4)
}

// What the SDK route does not reproduce still runs cub, whatever the setting.
func TestCubStdoutLeavesOtherCommandsToCubUnderTheSDKRoute(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	sdkReadsFrom(t, hub)
	t.Setenv(configHubReaderEnv, "sdk")

	for _, args := range [][]string{
		{"unit", "list", "-o", "json", "--space", recordedSpace},
		{"unit", "get", recordedUnitID, "-o", "json", "--space", recordedSpace},
		{"unit", "get", recordedUnit, "-o", "yaml", "--space", recordedSpace},
	} {
		_, err := cubStdout(context.Background(), args...)
		require.NoError(t, err)
	}
	require.Len(t, fakeCubCalls(t, cubLog), 3)
	require.Empty(t, hub.seen)
}

// A failed SDK read is the answer. It never falls back to cub, which would
// hide that the route asked for did not work.
func TestSDKRouteFailureIsReportedAndDoesNotFallBack(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, http.StatusForbidden)
	sdkReadsFrom(t, hub)
	t.Setenv(configHubReaderEnv, "sdk")
	args := []string{"unit", "get", recordedUnit, "-o", "json", "--space", recordedSpace}

	out, err := cubStdout(context.Background(), args...)
	require.Nil(t, out)
	require.Equal(t, hubread.KindForbidden, hubread.KindOf(err))
	require.True(t, failedOnSDKRoute(err))

	// Each caller names what failed, and none of them says cub did.
	_, err = cubText(context.Background(), args...)
	require.ErrorContains(t, err, "ConfigHub read in place of cub unit get "+recordedUnit)
	require.True(t, strings.HasPrefix(err.Error(), "ConfigHub read in place of cub "), err.Error())
	require.NotContains(t, err.Error(), "test-token")

	_, err = runMCPConnectedToolCommand(context.Background(), args)
	require.ErrorContains(t, err, "connected tool command failed: ConfigHub read in place of cub")
	require.Equal(t, hubread.KindForbidden, hubread.KindOf(err))

	_, err = loadCompareDryWetSnapshots(context.Background(), recordedUnit, recordedSpace, compareResourceRef{})
	require.ErrorContains(t, err, "ConfigHub unit read: ")
	require.NotContains(t, err.Error(), "cub unit get: ")

	surface, err := collectConfigHubSurface(context.Background(), recordedWorkload())
	require.Nil(t, surface)
	var collection *agent.CollectionError
	require.ErrorAs(t, err, &collection)
	require.Equal(t, "confighub", collection.Surface)
	require.True(t, strings.HasPrefix(collection.Reason, "ConfigHub unit read failed: "), collection.Reason)
	require.Contains(t, collection.Reason, "forbidden")
	require.NotContains(t, collection.Reason, "test-token")

	require.Empty(t, fakeCubCalls(t, cubLog), "cub was run after the SDK route failed")

	// The cub route keeps its own wording.
	t.Setenv(configHubReaderEnv, "cub")
	onlyFakeCub(t, "3")
	_, err = collectConfigHubSurface(context.Background(), recordedWorkload())
	require.ErrorAs(t, err, &collection)
	require.True(t, strings.HasPrefix(collection.Reason, "cub unit get failed: "), collection.Reason)
	_, err = cubText(context.Background(), args...)
	require.ErrorContains(t, err, "cub unit get "+recordedUnit+" -o json --space "+recordedSpace+" failed: ")
	require.False(t, failedOnSDKRoute(err))
}

// A misspelt route is an error for a read that has two routes, not the default
// route under another name.
func TestAnUnknownConfigHubReaderIsRefused(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	sdkReadsFrom(t, hub)
	t.Setenv(configHubReaderEnv, "sdkk")

	route, err := configHubReaderRoute()
	require.Error(t, err)
	require.Empty(t, route)

	_, err = cubStdout(context.Background(), "unit", "get", recordedUnit, "-o", "json", "--space", recordedSpace)
	require.EqualError(t, err, `CUB_SCOUT_CONFIGHUB_READER="sdkk" is not a reader (valid: cub, sdk)`)

	_, err = collectConfigHubSurface(context.Background(), recordedWorkload())
	var collection *agent.CollectionError
	require.ErrorAs(t, err, &collection)
	require.Contains(t, collection.Reason, `CUB_SCOUT_CONFIGHUB_READER="sdkk" is not a reader (valid: cub, sdk)`)

	require.Empty(t, fakeCubCalls(t, cubLog))
	require.Empty(t, hub.seen)
}

// The refusals the cub route makes before spawning are made on the SDK route
// too, before anything is resolved or sent.
func TestSDKRouteRefusesWhatTheCubRouteRefuses(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	sdkReadsFrom(t, hub)

	args := withConfigHubSpace([]string{"unit", "get", recordedUnit, "-o", "json"}, "")
	t.Setenv(configHubReaderEnv, "cub")
	_, cubErr := cubStdout(context.Background(), args...)
	require.Error(t, cubErr)
	t.Setenv(configHubReaderEnv, "sdk")
	_, sdkErr := cubStdout(context.Background(), args...)
	require.EqualError(t, sdkErr, cubErr.Error())

	require.Empty(t, fakeCubCalls(t, cubLog))
	require.Empty(t, hub.seen)
}

// source-truth reports the same evidence by either route. Both are given the
// same recorded server object: the cub route as cub printed it, the SDK route
// over HTTP.
func TestSourceTruthReportsTheSameByEitherRoute(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	sdkReadsFrom(t, hub)

	t.Setenv(configHubReaderEnv, "")
	viaCub, err := collectConfigHubSurface(context.Background(), recordedWorkload())
	require.NoError(t, err)
	require.Equal(t, []string{"unit get " + recordedUnit + " -o json --space " + recordedSpace}, fakeCubCalls(t, cubLog))

	t.Setenv(configHubReaderEnv, "sdk")
	viaSDK, err := collectConfigHubSurface(context.Background(), recordedWorkload())
	require.NoError(t, err)
	require.Len(t, fakeCubCalls(t, cubLog), 1)
	require.Equal(t, []string{"GET /api/space", "GET /api/unit"}, hub.seen)

	require.Equal(t, viaCub, viaSDK)
	require.Equal(t, &agent.ConfigHubSurface{
		Space: recordedSpace, Unit: recordedUnit, Revision: "2",
		URL: configHubUnitDetailURL(recordedSpaceID, recordedUnitID),
	}, viaSDK)
}

// The MCP tool and the compare read get the same Unit by either route.
func TestConnectedCallersGetTheSameUnitByEitherRoute(t *testing.T) {
	onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	sdkReadsFrom(t, hub)

	read := func() (string, compareUnitMetadata) {
		text, err := runMCPConnectedToolCommand(context.Background(), []string{"unit", "get", "-o", "json", recordedSpace + "/" + recordedUnit})
		require.NoError(t, err)
		raw, err := runCompareCubCommand(context.Background(), compareUnitGetArgs(recordedUnit, recordedSpace))
		require.NoError(t, err)
		meta, err := decodeCompareUnitMetadataFromGetJSON(raw)
		require.NoError(t, err)
		return text, meta
	}
	t.Setenv(configHubReaderEnv, "cub")
	cubText, cubMeta := read()
	require.Empty(t, hub.seen)
	t.Setenv(configHubReaderEnv, "sdk")
	sdkText, sdkMeta := read()
	require.Len(t, hub.seen, 4)

	require.JSONEq(t, cubText, sdkText)
	require.Equal(t, cubMeta, sdkMeta)
	require.NotEqual(t, compareUnitMetadata{}, sdkMeta)
}

// The route as it runs in production: credentials resolved from the cub plugin
// environment, not a reader built by the test.
func TestSDKRouteResolvesItsOwnCredentials(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CUB_CONFIG", t.TempDir())
	t.Setenv("CUB_CONTEXT", "")
	t.Setenv("CUB_SPACE", "")
	t.Setenv("CUB_SERVER", hub.url)
	t.Setenv("CUB_TOKEN", "plugin-token")
	t.Setenv(configHubReaderEnv, "sdk")

	surface, err := collectConfigHubSurface(context.Background(), recordedWorkload())
	require.NoError(t, err)
	require.Equal(t, "2", surface.Revision)
	require.Equal(t, []string{"cub-scout", "cub-scout"}, hub.agents)
	require.Equal(t, []string{"Bearer plugin-token", "Bearer plugin-token"}, hub.tokens)

	// With no credential anywhere, the read is an omission that says so.
	t.Setenv("CUB_SERVER", "")
	t.Setenv("CUB_TOKEN", "")
	_, err = collectConfigHubSurface(context.Background(), recordedWorkload())
	var collection *agent.CollectionError
	require.ErrorAs(t, err, &collection)
	require.True(t, strings.HasPrefix(collection.Reason, "ConfigHub unit read failed: "), collection.Reason)
	require.Contains(t, collection.Reason, "not_configured")
	require.Len(t, hub.seen, 2, "nothing is sent without a credential")
	require.Empty(t, fakeCubCalls(t, cubLog))
}
