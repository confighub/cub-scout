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

// #758: the release, resource and unit-event lists through cubStdout, by each
// route, on what cub printed for them on a real v0.8.12 server. Each command
// must reach its own endpoint: the recordings differ, so a command routed to
// another kind's reader returns other bytes or fails.
func TestCubStdoutAnswersTheReleaseResourceAndEventListsThroughTheSDK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	fixture := func(name string) string {
		path, err := filepath.Abs(filepath.Join("..", "..", "test", "fixtures", "confighub-release-lists-v0812-recorded", name))
		require.NoError(t, err)
		return path
	}
	read := func(name string) []byte {
		recorded, err := os.ReadFile(fixture(name))
		require.NoError(t, err)
		return recorded
	}
	var releases []struct {
		Release struct{ SpaceID, SpaceSlug string }
	}
	require.NoError(t, json.Unmarshal(read("release-list.json"), &releases))
	space, spaceID := releases[0].Release.SpaceSlug, releases[0].Release.SpaceID
	var resources []struct {
		Resource struct{ UnitID, UnitSlug string }
	}
	require.NoError(t, json.Unmarshal(read("resource-list.json"), &resources))
	unit, unitID := resources[0].Resource.UnitSlug, resources[0].Resource.UnitID
	require.NotEmpty(t, unit)
	require.NotEmpty(t, unitID)

	dir := t.TempDir()
	cubLog := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" + "echo \"$*\" >> \"" + cubLog + "\"\n" + "case \"$1 $2\" in\n" +
		"  \"release list\") /bin/cat \"" + fixture("release-list.json") + "\" ;;\n" +
		"  \"resource list\") /bin/cat \"" + fixture("resource-list.json") + "\" ;;\n" +
		"  \"unit-event list\") /bin/cat \"" + fixture("unit-event-list.json") + "\" ;;\n" +
		"  *) exit 9 ;;\nesac\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755))
	t.Setenv("PATH", dir)

	unitEvents := "/api/space/" + spaceID + "/unit/" + unitID + "/unit_event"
	served := map[string]string{
		"/api/space":                         `[{"Space":{"Slug":"` + space + `","SpaceID":"` + spaceID + `"}}]`,
		"/api/unit":                          `[{"Unit":{"Slug":"` + unit + `","UnitID":"` + unitID + `","SpaceID":"` + spaceID + `"}}]`,
		"/api/space/" + spaceID + "/release": string(read("release-list.json")),
		"/api/resource":                      string(read("resource-list.json")),
		"/api/unit_event":                    string(read("unit-event-list.json")),
		unitEvents:                           string(read("unit-event-list.json")),
	}
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if body, ok := served[r.URL.Path]; ok {
			_, _ = w.Write([]byte(body))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"Message":"not found"}`))
	}))
	t.Cleanup(server.Close)
	old := sdkReader
	t.Cleanup(func() { sdkReader = old })
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return hubread.New(server.URL, "test-token", hubread.Options{})
	}

	gateway := newMCPGatewayWithMode(nil, nil, true)
	build := func(tool string, arguments map[string]interface{}) []string {
		args, err := gateway.tools[tool].BuildArgs(arguments)
		require.NoError(t, err)
		return args
	}
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"releases, as gitops status asks", gitOpsConfigHubReleaseListArgs(space, "2020-01-01T00:00:00Z"), []string{"/api/space", "/api/space/" + spaceID + "/release"}},
		{"releases, as the MCP tool asks", build("confighub_releases", map[string]interface{}{"space": space}), []string{"/api/space", "/api/space/" + spaceID + "/release"}},
		{"resources", build("confighub_resources", map[string]interface{}{"space": space}), []string{"/api/space", "/api/resource"}},
		{"unit events, as gitops status asks", gitOpsConfigHubUnitEventListArgs(space, "2020-01-01T00:00:00Z"), []string{"/api/space", "/api/unit_event"}},
		{"unit events of one unit", build("confighub_unit_events", map[string]interface{}{"space": space, "unit": unit}), []string{"/api/space", "/api/unit", unitEvents}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cubBefore, seenBefore := len(fakeCubCalls(t, cubLog)), len(seen)
			t.Setenv(configHubReaderEnv, "cub")
			viaCub, err := cubStdout(context.Background(), tc.args...)
			require.NoError(t, err)
			require.Len(t, fakeCubCalls(t, cubLog), cubBefore+1)
			require.Len(t, seen, seenBefore)

			t.Setenv(configHubReaderEnv, "sdk")
			viaSDK, err := cubStdout(context.Background(), tc.args...)
			require.NoError(t, err, "cub %s", strings.Join(tc.args, " "))
			require.Len(t, fakeCubCalls(t, cubLog), cubBefore+1, "by the SDK route no cub process is started")
			require.Equal(t, tc.want, seen[seenBefore:], "each command reads its own endpoint")
			require.Equal(t, string(viaCub), string(viaSDK))
		})
	}
}
