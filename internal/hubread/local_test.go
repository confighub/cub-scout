package hubread

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// #758, batch D: the caller's organizations, one request with no scope.
func TestOrganizationListJSON(t *testing.T) {
	hub := newFakeHub(t)
	reader := hub.reader(Options{})
	ctx := context.Background()
	const path = "/api/organization"

	hub.raw[path] = `[{"Organization":{"OrganizationID":"11111111-2222-4333-8444-555555555555","Slug":"acme"}}]`
	out, err := reader.OrganizationListJSON(ctx)
	require.NoError(t, err)
	var listed []struct{ Organization struct{ Slug string } }
	require.NoError(t, json.Unmarshal(out, &listed))
	require.Len(t, listed, 1)
	require.Equal(t, "acme", listed[0].Organization.Slug)
	require.True(t, strings.HasPrefix(string(out), "[\n  {\n    \"Organization\": {"), "encoded as cub encodes a list")
	seen := hub.requests()
	require.Len(t, seen, 1)
	require.Equal(t, path, seen[0].Path)
	require.Empty(t, hub.queries[0], "cub sends no parameter for a plain organization list")

	for what, tc := range map[string]struct {
		body string
		want Kind
	}{
		"an entry with no organization": {`[{"Space":{}}]`, KindMalformed},
		"a page that is not the API":    {`<html>sign in</html>`, KindMalformed},
		"an object, not a list":         {`{}`, KindMalformed},
	} {
		hub.raw[path] = tc.body
		out, err := reader.OrganizationListJSON(ctx)
		require.Equal(t, tc.want, KindOf(err), what)
		require.Nil(t, out, what)
	}
	hub.raw[path], hub.more[path] = `[{"Organization":{"Slug":"acme"}}]`, true
	_, err = reader.OrganizationListJSON(ctx)
	require.Equal(t, KindIncomplete, KindOf(err))
	delete(hub.raw, path)
	hub.status[path] = http.StatusUnauthorized
	_, err = reader.OrganizationListJSON(ctx)
	require.Equal(t, KindUnauthorized, KindOf(err))
}

// The local cub context is read from cub's own configuration, with no server
// and no token. The configuration here is written the way cub writes one.
func TestLocalContextJSONReadsCubsConfigurationAndNoServer(t *testing.T) {
	dir := t.TempDir()
	config := `apiVersion: v1
kind: Config
currentContext: work
contexts:
  - name: work
    coordinate:
      serverURL: https://confighub.example.test
      organizationID: 11111111-2222-4333-8444-555555555555
      user: someone@example.test
    settings: {}
    metadata:
      tokenFile: work.json
      organizationName: Example
  - name: other
    coordinate:
      serverURL: https://other.example.test
      organizationID: 99999999-8888-4777-8666-555555555555
      user: someone@example.test
    settings: {}
    metadata:
      tokenFile: other.json
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(config), 0o600))
	before := snapshotDir(t, dir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CUB_CONFIG", dir)
	t.Setenv("CUB_CONTEXT", "")
	t.Setenv("CUB_PLUGIN", "")
	t.Setenv("CUB_SERVER", "")
	t.Setenv("CUB_TOKEN", "")

	out, err := LocalContextJSON(context.Background())
	require.NoError(t, err, "no token is needed to say which context is active")
	var active struct {
		Name       string
		Coordinate struct{ ServerURL, OrganizationID, User string }
	}
	require.NoError(t, json.Unmarshal(out, &active))
	require.Equal(t, "work", active.Name)
	require.Equal(t, "https://confighub.example.test", active.Coordinate.ServerURL)
	require.Contains(t, string(out), `"coordinate": {`, "cub's own field names")
	require.Equal(t, byte('\n'), out[len(out)-1])

	// CUB_CONTEXT selects another, as it does for cub.
	t.Setenv("CUB_CONTEXT", "other")
	out, err = LocalContextJSON(context.Background())
	require.NoError(t, err)
	require.Contains(t, string(out), "https://other.example.test")

	t.Setenv("CUB_CONTEXT", "no-such-context")
	out, err = LocalContextJSON(context.Background())
	require.Equal(t, KindNotConfigured, KindOf(err))
	require.Nil(t, out)

	require.Equal(t, before, snapshotDir(t, dir), "the configuration is read and not written")

	// No configuration at all.
	t.Setenv("CUB_CONTEXT", "")
	t.Setenv("CUB_CONFIG", t.TempDir())
	out, err = LocalContextJSON(context.Background())
	require.Equal(t, KindNotConfigured, KindOf(err), "%s", out)
}
