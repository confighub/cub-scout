// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	pkghub "github.com/confighub/cub-scout/v2/pkg/hub"
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

// spaceNamedIn is the space a test's argument vector names: the value after
// --space, or else the part of a qualified reference before the slash.
func spaceNamedIn(args []string) string {
	for i, arg := range args {
		if arg == "--space" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
	}
	for _, arg := range args[2:] {
		if space, _, qualified := strings.Cut(arg, "/"); qualified {
			return strings.TrimSpace(space)
		}
	}
	return ""
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
			require.Equal(t, spaceNamedIn(args), space)
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
		"a flag where the unit goes": {"unit", "get", "--verbose", "-o", "json", "--space", "s"},
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
		// cub reads every spelling uuid.Parse accepts as an ID.
		"unit by ID, bare hex":      {"unit", "get", "6221926e8cc44847a25ca4104042f640", "-o", "json", "--space", "s"},
		"unit by ID, braces":        {"unit", "get", "{" + recordedUnitID + "}", "-o", "json", "--space", "s"},
		"unit by ID, urn":           {"unit", "get", "urn:uuid:" + recordedUnitID, "-o", "json", "--space", "s"},
		"unit by ID, upper case":    {"unit", "get", strings.ToUpper(recordedUnitID), "-o", "json", "--space", "s"},
		"space by ID, bare hex":     {"unit", "get", "u", "-o", "json", "--space", "68843338f9bd485c9a665d5aee820247"},
		"space by ID in capitals":   {"unit", "get", "u", "-o", "json", "--space", strings.ToUpper(recordedSpaceID)},
		"space by ID, braces":       {"unit", "get", "u", "-o", "json", "--space", "{" + recordedSpaceID + "}"},
		"space by ID, urn":          {"unit", "get", "u", "-o", "json", "--space", "urn:uuid:" + recordedSpaceID},
		"qualified, space bare hex": {"unit", "get", "-o", "json", "68843338f9bd485c9a665d5aee820247/u"},
		"every unit":                {"unit", "get", "*", "-o", "json", "--space", "s"},
		"qualified, every unit":     {"unit", "get", "-o", "json", "s/*"},
		"qualified, every space":    {"unit", "get", "-o", "json", "*/u"},
		"space with a slash":        {"unit", "get", "u", "-o", "json", "--space", "a/b"},
		"space that is a flag":      {"unit", "get", "u", "-o", "json", "--space", "--quiet"},
		"output format in capitals": {"unit", "get", "u", "-o", "JSON", "--space", "s"},
		"output format then a flag": {"unit", "get", "u", "--space", "s", "-o", "--quiet"},
		"argument terminator":       {"unit", "get", "-o", "json", "--space", "s", "--", "u"},
		"short flag bundle":         {"unit", "get", "u", "-ojson", "--space", "s"},
	}
	for name, args := range left {
		t.Run("left to cub/"+name, func(t *testing.T) {
			_, _, ok := sdkUnitGetArgs(args)
			require.False(t, ok)
		})
	}
}

// cubCallSite is one place production code builds a cub command from literals.
type cubCallSite struct {
	where string   // file:line
	line  string   // the source line
	args  []string // the arguments, with "name" for each variable and the space helpers applied
}

// cubCallSites finds every place this package builds `cub <entity> <verb>`.
//
// It reads one line at a time, so it also counts every spelling of the pair
// across cmd/cub-scout, pkg and internal and fails when the counts differ: a
// call written across lines, spaced differently, or outside this package
// would otherwise pass unseen. It models the shape of the arguments, not the
// values a user supplies.
func cubCallSites(t *testing.T, entity, verb string) []cubCallSite {
	t.Helper()
	pair := `"` + entity + `", "` + verb + `"`
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	literalCall := regexp.MustCompile(regexp.QuoteMeta(pair) + `((?:, *(?:"[^"]*"|[A-Za-z_.]+))*) *[})]`)
	var sites []cubCallSite
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		source, err := os.ReadFile(file)
		require.NoError(t, err)
		for number, line := range strings.Split(string(source), "\n") {
			if !strings.Contains(line, pair) {
				continue
			}
			where := fmt.Sprintf("%s:%d", file, number+1)
			require.Equal(t, 1, strings.Count(line, pair), "%s builds two on one line", where)
			match := literalCall.FindStringSubmatch(line)
			require.NotNil(t, match, "%s builds a cub %s %s this test cannot read: %s", where, entity, verb, strings.TrimSpace(line))
			args := []string{entity, verb}
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
			sites = append(sites, cubCallSite{where: where, line: line, args: args})
		}
	}

	anySpelling := regexp.MustCompile(`"` + entity + `"\s*,\s*"` + verb + `"`)
	everywhere := 0
	for _, root := range []string{".", filepath.Join("..", "..", "pkg"), filepath.Join("..", "..", "internal")} {
		require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			source, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			everywhere += len(anySpelling.FindAll(source, -1))
			return nil
		}))
	}
	require.Equal(t, len(sites), everywhere, "a cub %s %s is built somewhere the line scan does not read", entity, verb)
	return sites
}

// Every place production code builds a `unit get` must build one the adapter
// takes. A call site added later in a shape the adapter does not know fails
// this test instead of quietly staying on cub.
func TestEveryUnitGetCallSiteIsOneTheSDKRouteTakes(t *testing.T) {
	sites := cubCallSites(t, "unit", "get")
	require.GreaterOrEqual(t, len(sites), 11, "the scan no longer finds the call sites")
	for _, site := range sites {
		space, unit, ok := sdkUnitGetArgs(site.args)
		require.True(t, ok, "%s: cub %s", site.where, strings.Join(site.args, " "))
		require.Equal(t, "name", unit)
		require.Contains(t, []string{"space", "name"}, space) // hierarchy.go passes its space as a variable
	}

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

// The same for the lists, except that some call sites are meant to stay with
// cub. Each of those is named here with its reason, so a list call that is
// neither taken nor explained fails.
func TestEveryListCallSiteIsTakenOrExplained(t *testing.T) {
	leftToCub := map[string]string{
		`"--select", "Slug,SpaceID,Annotations,Labels"`: "a selection changes what the server returns",
		`"--view", viewUUID`:                            "ConfigHub evaluates the columns of a View; that request is not reproduced here",
		`[]string{"unit", "list"}, space)`:              "prints cub's table to the user, through cubCommand",
		`cubArgs := []string{"space", "list"}`:          "streams cub's own output to the user, through cubCommand",
		`{"space", "list"}, // the spaces themselves`:   "not a call: the table of commands that name no space",
	}
	used := map[string]bool{}
	taken := map[string]int{}
	for _, command := range [][2]string{{"unit", "list"}, {"space", "list"}, {"worker", "list"}, {"target", "list"}, {"changeset", "list"}, {"link", "list"}} {
		for _, site := range cubCallSites(t, command[0], command[1]) {
			explained := false
			for fragment := range leftToCub {
				if strings.Contains(site.line, fragment) {
					explained, used[fragment] = true, true
				}
			}
			isTaken := sdkRead(site.args) != nil
			require.NotEqual(t, explained, isTaken, "%s: cub %s (taken %v, explained %v)", site.where, strings.Join(site.args, " "), isTaken, explained)
			if isTaken {
				taken[command[0]+" "+command[1]]++
			}
		}
	}
	for fragment := range leftToCub {
		require.True(t, used[fragment], "no call site matches %q any more; remove it", fragment)
	}
	require.GreaterOrEqual(t, taken["unit list"], 9)
	require.GreaterOrEqual(t, taken["space list"], 5)
	// Every worker, target, change set and link list is taken: none is
	// built with a flag the route does not know.
	require.GreaterOrEqual(t, taken["worker list"], 6)
	require.GreaterOrEqual(t, taken["target list"], 5)
	require.GreaterOrEqual(t, taken["changeset list"], 3)
	require.GreaterOrEqual(t, taken["link list"], 2)

	// The MCP tool appends a filter when it is given one.
	tool, ok := newMCPGatewayWithMode(nil, nil, true).tools["confighub_units"]
	require.True(t, ok)
	args, err := tool.BuildArgs(map[string]interface{}{"space": "space"})
	require.NoError(t, err)
	space, filter, ok := sdkUnitListArgs(args)
	require.True(t, ok, "cub %s", strings.Join(args, " "))
	require.Equal(t, "space", space)
	require.Equal(t, hubread.UnitFilter{}, filter)
	args, err = tool.BuildArgs(map[string]interface{}{"space": "space", "where": "Slug LIKE 'a-%'", "contains": "backend"})
	require.NoError(t, err)
	space, filter, ok = sdkUnitListArgs(args)
	require.True(t, ok, "cub %s", strings.Join(args, " "))
	require.Equal(t, "space", space)
	require.Equal(t, hubread.UnitFilter{Where: "Slug LIKE 'a-%'", Contains: "backend"}, filter)
}

func TestSDKListArgsTakeOnlyTheCommandsTheyReproduce(t *testing.T) {
	for name, args := range map[string][]string{
		"flags then space": {"unit", "list", "-o", "json", "--space", "s"},
		"space then flags": {"unit", "list", "--space", "s", "-o", "json"},
		"quiet":            {"unit", "list", "-o", "json", "--quiet", "--space", "s"},
		"long output flag": {"unit", "list", "--output", "json", "--space", "s"},
		"space by ID":      {"unit", "list", "-o", "json", "--space", recordedSpaceID},
		"placeholder space (cub's own refusal applies later)": {"unit", "list", "-o", "json", "--space", unresolvedConfigHubSpace},
	} {
		t.Run("unit list taken/"+name, func(t *testing.T) {
			space, filter, ok := sdkUnitListArgs(args)
			require.True(t, ok)
			require.Equal(t, spaceNamedIn(args), space)
			require.Equal(t, hubread.UnitFilter{}, filter)
			require.False(t, sdkSpaceListArgs(args))
			_, _, isGet := sdkUnitGetArgs(args)
			require.False(t, isGet)
		})
	}
	for name, args := range map[string][]string{
		"no space":                {"unit", "list", "-o", "json"},
		"every space":             {"unit", "list", "-o", "json", "--space", allConfigHubSpaces},
		"a selection":             {"unit", "list", "-o", "json", "--space", "s", "--select", "Slug"},
		"a stored filter":         {"unit", "list", "-o", "json", "--space", "s", "--filter", "f"},
		"a filter on data":        {"unit", "list", "-o", "json", "--space", "s", "--where-data", "x = 1"},
		"an empty filter":         {"unit", "list", "-o", "json", "--space", "s", "--where", " "},
		"a filter with no value":  {"unit", "list", "-o", "json", "--space", "s", "--where"},
		"two filters":             {"unit", "list", "-o", "json", "--space", "s", "--where", "A = '1'", "--where", "B = '2'"},
		"a filter spelt with =":   {"unit", "list", "-o", "json", "--space", "s", "--where=Slug = 'x'"},
		"a search spelt with =":   {"unit", "list", "-o", "json", "--space", "s", "--contains=x"},
		"an empty search":         {"unit", "list", "-o", "json", "--space", "s", "--contains", ""},
		"two searches":            {"unit", "list", "-o", "json", "--space", "s", "--contains", "a", "--contains", "b"},
		"a filter and no space":   {"unit", "list", "-o", "json", "--where", "Slug = 'x'"},
		"a filter, every space":   {"unit", "list", "-o", "json", "--space", allConfigHubSpaces, "--where", "Slug = 'x'"},
		"a limit":                 {"unit", "list", "-o", "json", "--space", "s", "--limit", "5"},
		"an ordering":             {"unit", "list", "-o", "json", "--space", "s", "--order-by", "Slug"},
		"hidden units too":        {"unit", "list", "-o", "json", "--space", "s", "--include-hidden"},
		"a view":                  {"unit", "list", "-o", "json", "--space", "s", "--view", "v"},
		"a positional":            {"unit", "list", "u", "-o", "json", "--space", "s"},
		"no output format":        {"unit", "list", "--space", "s"},
		"another output format":   {"unit", "list", "-o", "yaml", "--space", "s"},
		"names only":              {"unit", "list", "-o", "name", "--space", "s"},
		"space bare hex":          {"unit", "list", "-o", "json", "--space", "68843338f9bd485c9a665d5aee820247"},
		"space by ID in capitals": {"unit", "list", "-o", "json", "--space", strings.ToUpper(recordedSpaceID)},
		"space with a slash":      {"unit", "list", "-o", "json", "--space", "a/b"},
		"space named twice":       {"unit", "list", "-o", "json", "--space", "s", "--space", "t"},
		"space that is a flag":    {"unit", "list", "-o", "json", "--space", "--quiet"},
		"another command":         {"unit", "get", "u", "-o", "json", "--space", "s"},
		"another entity":          {"target", "list", "-o", "json", "--space", "s"},
		"too short":               {"unit"},
	} {
		t.Run("unit list left to cub/"+name, func(t *testing.T) {
			_, _, ok := sdkUnitListArgs(args)
			require.False(t, ok)
		})
	}

	for name, tc := range map[string]struct {
		args []string
		want hubread.UnitFilter
	}{
		"a filter":             {[]string{"unit", "list", "-o", "json", "--space", "s", "--where", "Slug = 'x'"}, hubread.UnitFilter{Where: "Slug = 'x'"}},
		"a filter first":       {[]string{"unit", "list", "--where", "Slug LIKE 'a-%'", "--space", "s", "-o", "json"}, hubread.UnitFilter{Where: "Slug LIKE 'a-%'"}},
		"a text search":        {[]string{"unit", "list", "-o", "json", "--space", "s", "--contains", "backend"}, hubread.UnitFilter{Contains: "backend"}},
		"both":                 {[]string{"unit", "list", "-o", "json", "--quiet", "--space", "s", "--where", "Slug != 'x'", "--contains", "b"}, hubread.UnitFilter{Where: "Slug != 'x'", Contains: "b"}},
		"a search like a flag": {[]string{"unit", "list", "-o", "json", "--space", "s", "--contains", "--quiet"}, hubread.UnitFilter{Contains: "--quiet"}},
		"a filter like a flag": {[]string{"unit", "list", "-o", "json", "--space", "s", "--where", "-o"}, hubread.UnitFilter{Where: "-o"}},
	} {
		t.Run("filtered unit list taken/"+name, func(t *testing.T) {
			space, filter, ok := sdkUnitListArgs(tc.args)
			require.True(t, ok)
			require.Equal(t, "s", space)
			require.Equal(t, tc.want, filter)
		})
	}
	// A filter belongs to a list: with one, the other two reads stay with cub.
	require.Nil(t, sdkRead([]string{"unit", "get", "u", "-o", "json", "--space", "s", "--where", "Slug = 'u'"}))
	require.Nil(t, sdkRead([]string{"space", "list", "-o", "json", "--contains", "x"}))

	for name, args := range map[string][]string{
		"plain":            {"space", "list", "-o", "json"},
		"quiet":            {"space", "list", "-o", "json", "--quiet"},
		"long output flag": {"space", "list", "--quiet", "--output", "json"},
	} {
		t.Run("space list taken/"+name, func(t *testing.T) {
			require.True(t, sdkSpaceListArgs(args))
			require.NotNil(t, sdkRead(args))
		})
	}
	for name, args := range map[string][]string{
		"no output format":      {"space", "list"},
		"another output format": {"space", "list", "-o", "yaml"},
		"a selection":           {"space", "list", "-o", "json", "--select", "Slug,SpaceID"},
		"a filter":              {"space", "list", "-o", "json", "--where", "Slug = 'x'"},
		"a component":           {"space", "list", "-o", "json", "--component", "c"},
		"a limit":               {"space", "list", "-o", "json", "--limit", "5"},
		"a space flag":          {"space", "list", "-o", "json", "--space", "s"},
		"a positional":          {"space", "list", "s", "-o", "json"},
		"another command":       {"space", "get", "s", "-o", "json"},
		"too short":             {"space"},
	} {
		t.Run("space list left to cub/"+name, func(t *testing.T) {
			require.False(t, sdkSpaceListArgs(args))
			require.Nil(t, sdkRead(args))
		})
	}
}

// #758: under the SDK route a `unit get` returns what cub printed for the same
// Unit, and no cub process is started.
func TestCubStdoutAnswersUnitGetThroughTheSDKWhenAsked(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	sdkReadsFrom(t, hub)
	args := []string{"unit", "get", recordedUnit, "-o", "json", "--space", recordedSpace}

	t.Setenv(configHubReaderEnv, "cub")
	viaCub, err := cubStdout(context.Background(), args...)
	require.NoError(t, err)
	require.Len(t, fakeCubCalls(t, cubLog), 1)
	require.Empty(t, hub.seen, "asked for cub, nothing is read through the SDK")

	t.Setenv(configHubReaderEnv, "sdk")
	viaSDK, err := cubStdout(context.Background(), args...)
	require.NoError(t, err)
	require.Len(t, fakeCubCalls(t, cubLog), 1, "with the setting no cub process is started")
	require.Equal(t, []string{"GET /api/space", "GET /api/unit"}, hub.seen)
	// The fake cub prints the recorded file, so this is byte for byte what
	// cub printed for this Unit.
	require.Equal(t, string(viaCub), string(viaSDK))

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
		{"unit", "data", recordedUnit, "--space", recordedSpace},
		{"unit", "get", recordedUnitID, "-o", "json", "--space", recordedSpace},
		{"unit", "get", recordedUnit, "-o", "yaml", "--space", recordedSpace},
		{"unit", "list", "-o", "json", "--space", recordedSpace, "--select", "Slug"},
		{"space", "list", "-o", "json", "--select", "Slug,SpaceID"},
	} {
		_, err := cubStdout(context.Background(), args...)
		require.NoError(t, err)
	}
	require.Len(t, fakeCubCalls(t, cubLog), 5)
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
	// No cub ran and no read was tried: the reason is the setting, and it is
	// not worded as a cub failure.
	require.Equal(t, `ConfigHub unit read failed: CUB_SCOUT_CONFIGHUB_READER="sdkk" is not a reader (valid: cub, sdk)`, collection.Reason)

	require.Empty(t, fakeCubCalls(t, cubLog))
	require.Empty(t, hub.seen)

	// A command the SDK route does not take has one route, so the setting is
	// not consulted for it.
	_, err = cubStdout(context.Background(), "unit", "data", recordedUnit, "--space", recordedSpace)
	require.NoError(t, err)
	require.Len(t, fakeCubCalls(t, cubLog), 1)

	// The lists have two routes as well.
	for _, args := range [][]string{{"unit", "list", "-o", "json", "--space", recordedSpace}, {"space", "list", "-o", "json"}} {
		_, err = cubStdout(context.Background(), args...)
		require.EqualError(t, err, `CUB_SCOUT_CONFIGHUB_READER="sdkk" is not a reader (valid: cub, sdk)`)
	}
	require.Len(t, fakeCubCalls(t, cubLog), 1)
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

	// The same refusal, but source-truth must not say cub failed on the SDK
	// route. (On the cub route that wording is older than this route.)
	workload := recordedWorkload()
	workload.Annotations["confighub.com/SpaceName"] = unresolvedConfigHubSpace
	_, err := collectConfigHubSurface(context.Background(), workload)
	var collection *agent.CollectionError
	require.ErrorAs(t, err, &collection)
	require.True(t, strings.HasPrefix(collection.Reason, "ConfigHub unit read failed: "), collection.Reason)

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

	t.Setenv(configHubReaderEnv, "cub")
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
	// The production resolver, which TestMain replaces for every other test.
	// HOME and CUB_CONFIG are private to this test, so it can find nothing
	// but what is set here.
	old := sdkReader
	t.Cleanup(func() { sdkReader = old })
	sdkReader = resolveSDKReader
	t.Setenv("CUB_PLUGIN", "1")
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

// connectedLaneFixture is one file cub v0.8.3 printed on the disposable server
// of the Connected lane (see the fixture's NOTICE).
func connectedLaneFixture(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "test", "fixtures", "confighub-sdk-parity-v083-recorded", name))
	require.NoError(t, err)
	return path
}

// connectedLaneRoutes sets up both routes on the Connected lane's recordings:
// a fake cub, the only cub that can run, that prints the recorded file for
// each command, and a server that answers each list with the same recording.
// It returns the fake cub's call log, the requests the server saw, and the
// recorded space's slug.
func connectedLaneRoutes(t *testing.T) (cubLog string, seen *[]string, space string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	files := map[string]string{}
	for _, name := range []string{"unit-get.json", "unit-list.json", "space-list.json"} {
		files[name] = connectedLaneFixture(t, name)
	}
	dir := t.TempDir()
	cubLog = filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"" + cubLog + "\"\n" +
		"case \"$1 $2\" in\n" +
		"  \"unit get\") /bin/cat \"" + files["unit-get.json"] + "\" ;;\n" +
		"  \"unit list\") /bin/cat \"" + files["unit-list.json"] + "\" ;;\n" +
		"  \"space list\") /bin/cat \"" + files["space-list.json"] + "\" ;;\n" +
		"  *) exit 9 ;;\n" +
		"esac\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755))
	t.Setenv("PATH", dir)

	units, err := os.ReadFile(files["unit-list.json"])
	require.NoError(t, err)
	spaces, err := os.ReadFile(files["space-list.json"])
	require.NoError(t, err)
	var recorded []struct{ Space struct{ Slug string } }
	require.NoError(t, json.Unmarshal(units, &recorded))
	require.NotEmpty(t, recorded)

	seen = &[]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = append(*seen, r.Method+" "+r.URL.Path+"?"+r.URL.Query().Encode())
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/space":
			_, _ = w.Write(spaces)
		case "/api/unit":
			_, _ = w.Write(units)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	old := sdkReader
	t.Cleanup(func() { sdkReader = old })
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return hubread.New(server.URL, "test-token", hubread.Options{})
	}
	return cubLog, seen, recorded[0].Space.Slug
}

// #758: under the SDK route the lists return, byte for byte, what cub printed
// for them on a real server, and no cub process is started.
func TestCubStdoutAnswersTheListsThroughTheSDKWhenAsked(t *testing.T) {
	cubLog, seen, space := connectedLaneRoutes(t)
	for _, tc := range []struct {
		name     string
		args     []string
		requests int
	}{
		{"unit list", []string{"unit", "list", "-o", "json", "--space", space}, 2},
		{"unit list, quiet", []string{"unit", "list", "-o", "json", "--quiet", "--space", space}, 2},
		{"unit list, filtered", []string{"unit", "list", "-o", "json", "--space", space, "--where", "Slug LIKE 'parity-%'", "--contains", "parity"}, 2},
		{"space list", []string{"space", "list", "-o", "json"}, 1},
		{"space list, quiet", []string{"space", "list", "-o", "json", "--quiet"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cubBefore, seenBefore := len(fakeCubCalls(t, cubLog)), len(*seen)

			t.Setenv(configHubReaderEnv, "cub")
			viaCub, err := cubStdout(context.Background(), tc.args...)
			require.NoError(t, err)
			require.Len(t, fakeCubCalls(t, cubLog), cubBefore+1)
			require.Len(t, *seen, seenBefore, "without the setting nothing is read through the SDK")

			t.Setenv(configHubReaderEnv, "sdk")
			viaSDK, err := cubStdout(context.Background(), tc.args...)
			require.NoError(t, err)
			require.Len(t, fakeCubCalls(t, cubLog), cubBefore+1, "with the setting no cub process is started")
			require.Len(t, *seen, seenBefore+tc.requests)
			require.Equal(t, string(viaCub), string(viaSDK))
			require.True(t, strings.HasPrefix(string(viaSDK), "[\n  {"))
		})
	}

	// What the list asked the server: the one space, cub's expansions, no
	// limit; and for the spaces, their summary.
	require.Contains(t, (*seen)[1], "GET /api/unit?include=UnitEventID%2CTargetID%2CUpstreamUnitID%2CSpaceID%2CFromLinkID%2CChangeSetID&where=SpaceID+%3D+")
	require.NotContains(t, (*seen)[1], "limit=")
	require.Equal(t, "GET /api/space?include=ComponentID&summary=true", (*seen)[len(*seen)-1])
	// The filter is the caller's, AND-ed with the space, with the search
	// beside it.
	require.Contains(t, (*seen)[5], "GET /api/unit?contains=parity&include=UnitEventID")
	require.Contains(t, (*seen)[5], "&where=Slug+LIKE+%27parity-%25%27+AND+SpaceID+%3D+%27")
}

// The callers of the lists get the same result by either route.
func TestListCallersGetTheSameByEitherRoute(t *testing.T) {
	_, seen, space := connectedLaneRoutes(t)
	read := func() (string, []FleetUnit) {
		text, err := runMCPConnectedToolCommand(context.Background(), withConfigHubSpace([]string{"unit", "list", "-o", "json"}, space))
		require.NoError(t, err)
		units, err := fetchFleetUnits(space, "")
		require.NoError(t, err)
		return text, units
	}
	t.Setenv("CUB_SCOUT_TEST_MAP_FLEET_JSON", "")
	t.Setenv(configHubReaderEnv, "cub")
	cubText, cubUnits := read()
	require.Empty(t, *seen)
	t.Setenv(configHubReaderEnv, "sdk")
	sdkText, sdkUnits := read()
	// Two lists at two requests each, then the fleet reads each of the two
	// Units again, at two requests each: the space is looked up every time.
	require.Len(t, *seen, 8)

	require.Equal(t, cubText, sdkText)
	require.Equal(t, cubUnits, sdkUnits)
	var listed []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(sdkText), &listed))
	require.Len(t, listed, 2)
}

// A failed list is the answer, worded as a ConfigHub read, with no fallback;
// and a list is never empty because the read failed.
func TestSDKRouteListFailureIsReportedAndDoesNotFallBack(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, http.StatusUnauthorized)
	sdkReadsFrom(t, hub)
	t.Setenv(configHubReaderEnv, "sdk")

	for _, args := range [][]string{{"unit", "list", "-o", "json", "--space", recordedSpace}, {"space", "list", "-o", "json"}} {
		out, err := cubStdout(context.Background(), args...)
		require.Nil(t, out)
		require.Equal(t, hubread.KindUnauthorized, hubread.KindOf(err))
		require.True(t, failedOnSDKRoute(err))

		_, err = cubText(context.Background(), args...)
		require.True(t, strings.HasPrefix(err.Error(), "ConfigHub read in place of cub "+args[0]+" list "), err.Error())
	}
	// The refusal for a list that forgot its space is the cub route's.
	_, err := cubStdout(context.Background(), withConfigHubSpace([]string{"unit", "list", "-o", "json"}, "")...)
	require.Error(t, err)
	require.True(t, failedOnSDKRoute(err))
	require.Len(t, hub.seen, 4, "one request for each of the four failed reads, none for the refused one")
	require.Empty(t, fakeCubCalls(t, cubLog), "cub was run after the SDK route failed")
}

// The fleet view's advice follows the route: checking that cub is installed
// is no help when no cub ran.
func TestFleetFailureAdviceFollowsTheRoute(t *testing.T) {
	onlyFakeCub(t, "3")
	hub := newRecordedHub(t, http.StatusInternalServerError)
	sdkReadsFrom(t, hub)
	t.Setenv("CUB_SCOUT_TEST_MAP_FLEET_JSON", "")

	t.Setenv(configHubReaderEnv, "sdk")
	_, err := fetchFleetUnits(recordedSpace, "")
	require.ErrorContains(t, err, "failed to fetch units from ConfigHub: ")
	require.Equal(t, hubread.KindFailed, hubread.KindOf(err))
	require.NotContains(t, err.Error(), "'cub' CLI is installed")

	t.Setenv(configHubReaderEnv, "cub")
	_, err = fetchFleetUnits(recordedSpace, "")
	require.ErrorContains(t, err, "Check that 'cub' CLI is installed")
}

// The views lookups word a failed list for the route that ran it.
func TestViewsListFailureWordingFollowsTheRoute(t *testing.T) {
	onlyFakeCub(t, "3")
	hub := newRecordedHub(t, http.StatusForbidden)
	sdkReadsFrom(t, hub)

	t.Setenv(configHubReaderEnv, "sdk")
	_, err := listUnitsForFilter(context.Background(), "Slug LIKE 'a-%'", recordedSpace)
	require.True(t, strings.HasPrefix(err.Error(), "ConfigHub unit list: "), err.Error())
	require.Equal(t, hubread.KindForbidden, hubread.KindOf(err))
	_, err = listUnitSlugsForFilter(context.Background(), "Slug LIKE 'a-%'", recordedSpace)
	require.True(t, strings.HasPrefix(err.Error(), "ConfigHub unit list: "), err.Error())

	// Across every space the read stays with cub, and so does the wording.
	_, err = listUnitsForFilter(context.Background(), "Slug LIKE 'a-%'", allConfigHubSpaces)
	require.True(t, strings.HasPrefix(err.Error(), "cub unit list: "), err.Error())
	t.Setenv(configHubReaderEnv, "cub")
	_, err = listUnitSlugsForFilter(context.Background(), "Slug LIKE 'a-%'", recordedSpace)
	require.True(t, strings.HasPrefix(err.Error(), "cub unit list: "), err.Error())
}

// The default is the SDK route: with the setting absent, a read this package
// reproduces starts no cub. "cub" is the way back.
func TestTheDefaultReaderIsTheSDK(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	sdkReadsFrom(t, hub)
	args := []string{"unit", "get", recordedUnit, "-o", "json", "--space", recordedSpace}

	for _, unset := range []string{"", "  ", "sdk", "SDK"} {
		t.Setenv(configHubReaderEnv, unset)
		route, err := configHubReaderRoute()
		require.NoError(t, err)
		require.Equal(t, "sdk", route, "%q", unset)
	}
	t.Setenv(configHubReaderEnv, "")
	_, err := cubStdout(context.Background(), args...)
	require.NoError(t, err)
	require.Equal(t, []string{"GET /api/space", "GET /api/unit"}, hub.seen)
	require.Empty(t, fakeCubCalls(t, cubLog), "the default starts no cub for this read")

	t.Setenv(configHubReaderEnv, "cub")
	route, err := configHubReaderRoute()
	require.NoError(t, err)
	require.Equal(t, "cub", route)
	_, err = cubStdout(context.Background(), args...)
	require.NoError(t, err)
	require.Len(t, fakeCubCalls(t, cubLog), 1)
	require.Len(t, hub.seen, 2)
}

// The test binary itself is held to the cub route and has no way to resolve
// a real credential: a test that forgets to say which server to read fails,
// it does not read whatever this machine is logged in to.
func TestTheTestBinaryCannotReachRealCredentials(t *testing.T) {
	require.Equal(t, "cub", os.Getenv(configHubReaderEnv), "TestMain pins the route for the package and for the processes it starts")
	// Whatever the shell exported: the pin does not defer to it.
	for _, exported := range []string{"sdk", "", "anything"} {
		t.Setenv(configHubReaderEnv, exported)
		pinTestsToTheCubRoute()
		require.Equal(t, "cub", os.Getenv(configHubReaderEnv), "exported as %q", exported)
	}
	_, err := sdkReader(context.Background())
	require.ErrorIs(t, err, errSDKRouteInATest)

	onlyFakeCub(t, "0")
	t.Setenv(configHubReaderEnv, "sdk")
	_, err = cubStdout(context.Background(), "unit", "get", recordedUnit, "-o", "json", "--space", recordedSpace)
	require.ErrorIs(t, err, errSDKRouteInATest)
	require.True(t, failedOnSDKRoute(err))
}

// When a read fails on the SDK route in a way cub might not, the error says
// how to read through cub. A denial or a missing Unit is the same either way.
func TestASDKRouteFailureNamesTheWayBackOnlyWhenItCouldHelp(t *testing.T) {
	onlyFakeCub(t, "0")
	t.Setenv(configHubReaderEnv, "sdk")
	args := []string{"unit", "get", recordedUnit, "-o", "json", "--space", recordedSpace}
	const wayBack = "CUB_SCOUT_CONFIGHUB_READER=cub reads this through the cub CLI instead"

	// Both routes use one server and one credential, so a denial, a server
	// error or a missing endpoint is the same through cub.
	for status, hinted := range map[int]bool{
		http.StatusInternalServerError: false,
		http.StatusNotFound:            false,
		http.StatusForbidden:           false,
		http.StatusUnauthorized:        false,
	} {
		sdkReadsFrom(t, newRecordedHub(t, status))
		_, err := cubText(context.Background(), args...)
		require.Error(t, err)
		require.Equal(t, hinted, strings.Contains(err.Error(), wayBack), "HTTP %d: %v", status, err)
		// Every caller gets it, not only the one that works in text.
		_, err = collectConfigHubSurface(context.Background(), recordedWorkload())
		require.Equal(t, hinted, strings.Contains(err.Error(), wayBack), "source-truth, HTTP %d: %v", status, err)
		require.Equal(t, 1, strings.Count(err.Error(), wayBack)+boolToInt(!hinted), "said once")
	}
	// What only this reader does: refuse an answer it cannot decode, one the
	// server cut short, a request it will not send, and one that outlasts its
	// own time limit. cub might have answered each.
	for _, kind := range []hubread.Kind{hubread.KindMalformed, hubread.KindIncomplete, hubread.KindRefused, hubread.KindTimeout} {
		routed := &sdkRouteError{&hubread.Error{Kind: kind, Op: "unit read", Message: "x"}}
		require.Equal(t, 1, strings.Count(routed.Error(), wayBack), string(kind))
	}
	for _, kind := range []hubread.Kind{hubread.KindFailed, hubread.KindForbidden, hubread.KindUnauthorized, hubread.KindNotFound,
		hubread.KindNotConfigured, hubread.KindInvalidScope, hubread.KindAmbiguous, hubread.KindCanceled} {
		routed := &sdkRouteError{&hubread.Error{Kind: kind, Op: "unit read", Message: "x"}}
		require.NotContains(t, routed.Error(), wayBack, string(kind))
	}
	// End to end: a body that is not the list the status promised.
	truncated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"Space":`))
	}))
	defer truncated.Close()
	old := sdkReader
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return hubread.New(truncated.URL, "test-token", hubread.Options{})
	}
	_, malformedErr := cubText(context.Background(), args...)
	sdkReader = old
	require.Equal(t, hubread.KindMalformed, hubread.KindOf(malformedErr))
	require.Equal(t, 1, strings.Count(malformedErr.Error(), wayBack), malformedErr.Error())

	// The Unit is not there: the same answer by either route.
	hub := newRecordedHub(t, 0)
	sdkReadsFrom(t, hub)
	_, err := cubText(context.Background(), "unit", "get", "no-such-unit", "-o", "json", "--space", recordedSpace)
	require.Equal(t, hubread.KindNotFound, hubread.KindOf(err))
	require.NotContains(t, err.Error(), wayBack)
	// A cub failure never suggests cub.
	t.Setenv(configHubReaderEnv, "cub")
	onlyFakeCub(t, "3")
	_, err = cubText(context.Background(), args...)
	require.Error(t, err)
	require.NotContains(t, err.Error(), wayBack)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CUB_SCOUT_OFFLINE and the telemetry switch turn ConfigHub reads off. Some
// commands read without first asking whether they may; through cub, a machine
// with no cub stayed offline regardless. The SDK route needs no cub, so it
// honours the switch itself, before it looks for a credential.
func TestTheSDKRouteHonoursTheOfflineSwitch(t *testing.T) {
	cubLog := onlyFakeCub(t, "0")
	hub := newRecordedHub(t, 0)
	resolved := 0
	oldReader := sdkReader
	t.Cleanup(func() { sdkReader = oldReader })
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		resolved++
		return hubread.New(hub.url, "test-token", hubread.Options{})
	}
	oldSwitch := configHubReadsDisabledFn
	t.Cleanup(func() { configHubReadsDisabledFn = oldSwitch })
	configHubReadsDisabledFn = pkghub.ConfigHubReadsDisabled
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CUB_SCOUT_TELEMETRY", "")
	t.Setenv(configHubReaderEnv, "sdk")

	for _, args := range [][]string{
		{"unit", "get", recordedUnit, "-o", "json", "--space", recordedSpace},
		{"unit", "list", "-o", "json", "--space", recordedSpace},
		{"space", "list", "-o", "json"},
	} {
		t.Setenv("CUB_SCOUT_OFFLINE", "true")
		out, err := cubStdout(context.Background(), args...)
		require.Nil(t, out)
		require.ErrorIs(t, err, pkghub.ErrConfigHubReadsDisabled, "%v", args)
		require.ErrorContains(t, err, "CUB_SCOUT_OFFLINE=true is set")
		require.True(t, failedOnSDKRoute(err))
	}
	require.Empty(t, hub.seen, "nothing is sent")
	require.Zero(t, resolved, "no credential is looked for")
	require.Empty(t, fakeCubCalls(t, cubLog), "and no cub is started in its place")

	// The fleet view is one of the commands with no gate of its own.
	t.Setenv("CUB_SCOUT_TEST_MAP_FLEET_JSON", "")
	_, err := fetchFleetUnits(recordedSpace, "")
	require.ErrorIs(t, err, pkghub.ErrConfigHubReadsDisabled)
	require.Empty(t, hub.seen)

	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "false")
	_, err = cubStdout(context.Background(), "space", "list", "-o", "json")
	require.ErrorIs(t, err, pkghub.ErrConfigHubReadsDisabled)
	require.ErrorContains(t, err, "telemetry is disabled")
	require.Empty(t, hub.seen)

	t.Setenv("CUB_SCOUT_TELEMETRY", "")
	_, err = cubStdout(context.Background(), "space", "list", "-o", "json")
	require.NoError(t, err)
	require.Equal(t, 1, resolved)
}

// The import wizard reads a Unit to see whether it has a target, and sets one
// if it has none. A read that failed says nothing about the target: going on
// would write to ConfigHub on no evidence.
func TestImportWizardDoesNotSetATargetWhenItCouldNotReadTheUnit(t *testing.T) {
	oldDebug := testDebugDir
	t.Cleanup(func() { testDebugDir = oldDebug })
	testDebugDir = t.TempDir()
	model := ImportWizardModel{proposal: &FullProposal{App: recordedSpace}, testUnitSlug: recordedUnit}

	for _, route := range []string{"cub", "sdk"} {
		t.Run(route, func(t *testing.T) {
			cubLog := onlyFakeCub(t, "3")
			hub := newRecordedHub(t, http.StatusInternalServerError)
			sdkReadsFrom(t, hub)
			t.Setenv(configHubReaderEnv, route)

			result, ok := model.runTestApply().(wizardTestPhaseMsg)
			require.True(t, ok)
			require.False(t, result.success)
			require.ErrorContains(t, result.err, "could not read unit "+recordedUnit+" before applying")
			for _, call := range fakeCubCalls(t, cubLog) {
				require.NotContains(t, call, "set-target", "a target was set after a failed read")
				require.NotContains(t, call, "target list")
				require.NotContains(t, call, "apply")
			}
			if route == "sdk" {
				require.Empty(t, fakeCubCalls(t, cubLog))
			}
		})
	}
}

// The lists of one space besides Units: the same shape for each entity, and
// nothing else.
func TestSDKListArgsForTheOtherEntities(t *testing.T) {
	for _, entity := range []string{"worker", "target", "changeset", "link", "resource", "release", "unit-event"} {
		t.Run(entity, func(t *testing.T) {
			for name, tc := range map[string]struct {
				args []string
				want hubread.Filter
			}{
				"plain":         {[]string{entity, "list", "--space", "s", "-o", "json"}, hubread.Filter{}},
				"flags first":   {[]string{entity, "list", "-o", "json", "--quiet", "--space", "s"}, hubread.Filter{}},
				"a filter":      {[]string{entity, "list", "--space", "s", "--where", "Slug = 'x'", "-o", "json", "--quiet"}, hubread.Filter{Where: "Slug = 'x'"}},
				"a text search": {[]string{entity, "list", "-o", "json", "--contains", "break-glass", "--space", "s"}, hubread.Filter{Contains: "break-glass"}},
			} {
				got, space, filter, ok := sdkListArgs(tc.args)
				require.True(t, ok, name)
				require.Equal(t, entity, got)
				require.Equal(t, "s", space)
				require.Equal(t, tc.want, filter, name)
				require.NotNil(t, sdkRead(tc.args))
				// It is this entity's list, not the unit list.
				_, _, isUnits := sdkUnitListArgs(tc.args)
				require.False(t, isUnits)
			}
			for name, args := range map[string][]string{
				"no space":         {entity, "list", "-o", "json"},
				"every space":      {entity, "list", "-o", "json", "--space", allConfigHubSpaces},
				"no output format": {entity, "list", "--space", "s"},
				"a selection":      {entity, "list", "-o", "json", "--space", "s", "--select", "Slug"},
				"a stored filter":  {entity, "list", "-o", "json", "--space", "s", "--filter", "f"},
				"a positional":     {entity, "list", "x", "-o", "json", "--space", "s"},
				"another command":  {entity, "get", "x", "-o", "json", "--space", "s"},
			} {
				_, _, _, ok := sdkListArgs(args)
				require.False(t, ok, name)
				if entity == "unit-event" && name == "a positional" {
					continue // the events of one Unit, which is its own read
				}
				require.Nil(t, sdkRead(args), name)
			}
		})
	}
	// An entity this file does not list stays with cub.
	for _, entity := range []string{"attestation", "trigger", "organization", "revision", "view"} {
		require.Nil(t, sdkRead([]string{entity, "list", "-o", "json", "--space", "s"}), entity)
	}
}

// #758, batch A: the other lists of a space, through cubStdout by each route,
// on what cub printed for them on a real v0.8.12 server. The fake cub prints
// the recording; the test server sends the same JSON, with the worker's
// Secret where a real server sends it.
func TestCubStdoutAnswersTheOtherSpaceListsThroughTheSDK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	fixture := func(name string) string {
		path, err := filepath.Abs(filepath.Join("..", "..", "test", "fixtures", "confighub-space-lists-v0812-recorded", name))
		require.NoError(t, err)
		return path
	}
	read := func(name string) []byte {
		recorded, err := os.ReadFile(fixture(name))
		require.NoError(t, err)
		return recorded
	}
	var units []struct {
		Space struct{ Slug, SpaceID string }
	}
	require.NoError(t, json.Unmarshal(read("unit-list.json"), &units))
	space, spaceID := units[0].Space.Slug, units[0].Space.SpaceID

	dir := t.TempDir()
	cubLog := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" + "echo \"$*\" >> \"" + cubLog + "\"\n" + "case \"$1 $2\" in\n"
	for _, entity := range []string{"worker", "target", "changeset", "link"} {
		script += "  \"" + entity + " list\") /bin/cat \"" + fixture(entity+"-list.json") + "\" ;;\n"
	}
	script += "  *) exit 9 ;;\nesac\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755))
	t.Setenv("PATH", dir)

	const secret = "ch_not-a-real-worker-secret"
	workers := strings.Replace(string(read("worker-list.json")), `      "Slug": "parity-worker",`, `      "Secret": "`+secret+`",`+"\n"+`      "Slug": "parity-worker",`, 1)
	require.Contains(t, workers, secret)
	served := map[string][]byte{
		"/api/space": read("space-list.json"), "/api/bridge_worker": []byte(workers), "/api/target": read("target-list.json"),
		"/api/change_set": read("changeset-list.json"), "/api/link": read("link-list.json"),
	}
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path+"?where="+r.URL.Query().Get("where"))
		w.Header().Set("Content-Type", "application/json")
		if body, ok := served[r.URL.Path]; ok {
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	old := sdkReader
	t.Cleanup(func() { sdkReader = old })
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return hubread.New(server.URL, "test-token", hubread.Options{})
	}

	for entity, path := range map[string]string{"worker": "/api/bridge_worker", "target": "/api/target", "changeset": "/api/change_set", "link": "/api/link"} {
		t.Run(entity, func(t *testing.T) {
			// As the call sites build it: the space appended by
			// withConfigHubSpace, by slug.
			args := withConfigHubSpace([]string{entity, "list", "-o", "json"}, space)
			cubBefore, seenBefore := len(fakeCubCalls(t, cubLog)), len(seen)

			t.Setenv(configHubReaderEnv, "cub")
			viaCub, err := cubStdout(context.Background(), args...)
			require.NoError(t, err)
			require.Len(t, fakeCubCalls(t, cubLog), cubBefore+1)
			require.Len(t, seen, seenBefore, "by the cub route nothing is read through the SDK")

			t.Setenv(configHubReaderEnv, "sdk")
			viaSDK, err := cubStdout(context.Background(), args...)
			require.NoError(t, err)
			require.Len(t, fakeCubCalls(t, cubLog), cubBefore+1, "by the SDK route no cub process is started")
			require.Equal(t, []string{"/api/space?where=Slug = '" + space + "'", path + "?where=SpaceID = '" + spaceID + "'"}, seen[seenBefore:])
			require.Equal(t, string(viaCub), string(viaSDK))
			require.NotContains(t, string(viaSDK), secret, "a worker's secret is not passed on")
			require.Greater(t, len(viaSDK), 100)
		})
	}

	// The attestation list is read by the reader but not routed here: its
	// one caller runs cub itself under a byte limit of its own.
	t.Setenv(configHubReaderEnv, "sdk")
	require.Nil(t, sdkRead(withConfigHubSpace([]string{"attestation", "list", "-o", "json"}, space)))
}

// The events of one Unit: `cub unit-event list <unit>`, with the Unit named as
// `unit get` requires it.
func TestSDKUnitEventsOfUnitArgs(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want hubread.Filter
	}{
		"as the MCP tool builds it": {[]string{"unit-event", "list", "payments-api", "--space", "s", "-o", "json"}, hubread.Filter{}},
		"with a filter":             {[]string{"unit-event", "list", "payments-api", "--space", "s", "-o", "json", "--where", "CreatedAt > '2026-09-10T00:00:00Z'"}, hubread.Filter{Where: "CreatedAt > '2026-09-10T00:00:00Z'"}},
		"the unit last":             {[]string{"unit-event", "list", "--space", "s", "-o", "json", "--quiet", "payments-api"}, hubread.Filter{}},
	} {
		space, unit, filter, ok := sdkUnitEventsOfUnitArgs(tc.args)
		require.True(t, ok, name)
		require.Equal(t, []string{"s", "payments-api"}, []string{space, unit}, name)
		require.Equal(t, tc.want, filter, name)
		require.NotNil(t, sdkRead(tc.args), name)
		_, _, _, wholeSpace := sdkListArgs(tc.args)
		require.False(t, wholeSpace, name)
	}
	for name, args := range map[string][]string{
		"no space":            {"unit-event", "list", "payments-api", "-o", "json"},
		"every space":         {"unit-event", "list", "payments-api", "-o", "json", "--space", allConfigHubSpaces},
		"a unit named by ID":  {"unit-event", "list", recordedUnitID, "-o", "json", "--space", "s"},
		"a qualified unit":    {"unit-event", "list", "s/payments-api", "-o", "json", "--space", "s"},
		"every unit":          {"unit-event", "list", allConfigHubSpaces, "-o", "json", "--space", "s"},
		"two units":           {"unit-event", "list", "a", "b", "-o", "json", "--space", "s"},
		"no output format":    {"unit-event", "list", "payments-api", "--space", "s"},
		"a stored filter":     {"unit-event", "list", "payments-api", "-o", "json", "--space", "s", "--filter", "f"},
		"another entity":      {"release", "list", "payments-api", "-o", "json", "--space", "s"},
		"another verb":        {"unit-event", "get", "payments-api", "-o", "json", "--space", "s"},
		"a non-canonical ID":  {"unit-event", "list", "payments-api", "-o", "json", "--space", strings.ToUpper(recordedSpaceID)},
		"a blank unit":        {"unit-event", "list", " ", "-o", "json", "--space", "s"},
		"a limit on the list": {"unit-event", "list", "payments-api", "-o", "json", "--space", "s", "--limit", "5"},
	} {
		_, _, _, ok := sdkUnitEventsOfUnitArgs(args)
		require.False(t, ok, name)
		require.Nil(t, sdkRead(args), name)
	}
}

// The release, unit-event and resource lists are built in ways the line scan
// of cubCallSites does not read (across lines, or inside two appends), so
// each builder is called here, and the number of places the pair is spelled
// is pinned: a new call site fails this test until it is added below.
func TestTheReleaseEventAndResourceListsAreTaken(t *testing.T) {
	gateway := newMCPGatewayWithMode(nil, nil, true)
	build := func(tool string, arguments map[string]interface{}) []string {
		args, err := gateway.tools[tool].BuildArgs(arguments)
		require.NoError(t, err, tool)
		return args
	}
	const cutoff = "2026-09-09T12:00:00Z"
	taken := map[string][][]string{
		"release list": {
			gitOpsConfigHubReleaseListArgs("space", cutoff),
			build("confighub_releases", map[string]interface{}{"space": "space"}),
			build("confighub_releases", map[string]interface{}{"space": "space", "where": "ReleaseNum > 3"}),
		},
		"unit-event list": {
			gitOpsConfigHubUnitEventListArgs("space", cutoff),
			build("confighub_unit_events", map[string]interface{}{"space": "space"}),
			build("confighub_unit_events", map[string]interface{}{"space": "space", "unit": "payments-api", "where": "CreatedAt > '" + cutoff + "'"}),
		},
		"resource list": {
			build("confighub_resources", map[string]interface{}{"space": "space"}),
			build("confighub_resources", map[string]interface{}{"space": "space", "where": "ResourceType = 'apps/v1/Deployment'", "contains": "nginx"}),
		},
	}
	builders := map[string]int{"release list": 2, "unit-event list": 2, "resource list": 1}
	for command, calls := range taken {
		entity, verb, _ := strings.Cut(command, " ")
		for _, args := range calls {
			require.Equal(t, []string{entity, verb}, args[:2])
			require.NotNil(t, sdkRead(args), "cub %s", strings.Join(args, " "))
		}
		spelled := 0
		anySpelling := regexp.MustCompile(`"` + entity + `"\s*,\s*"` + verb + `"`)
		for _, root := range []string{".", filepath.Join("..", "..", "pkg"), filepath.Join("..", "..", "internal")} {
			require.NoError(t, filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
				if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
					return err
				}
				source, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				spelled += len(anySpelling.FindAll(source, -1))
				return nil
			}))
		}
		require.Equal(t, builders[command], spelled, "a cub %s is built somewhere this test does not call", command)
	}

	// What stays with cub: every space at once, and the options of the
	// resource list that change what the server is asked.
	for name, args := range map[string][]string{
		"releases of every space":  build("confighub_releases", map[string]interface{}{"space": allConfigHubSpaces}),
		"resources of every space": build("confighub_resources", map[string]interface{}{"space": allConfigHubSpaces}),
		"a selection of resources": build("confighub_resources", map[string]interface{}{"space": "space", "select": "ResourceName"}),
		"a stored resource filter": build("confighub_resources", map[string]interface{}{"space": "space", "filter": "f"}),
	} {
		require.Nil(t, sdkRead(args), "%s: cub %s", name, strings.Join(args, " "))
	}
}
