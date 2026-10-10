package hubread

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// releaseListsRecording is one file of what cub printed for the releases and
// resources of a space on a real v0.8.12 server (NOTICE.md beside the files).
func releaseListsRecording(t *testing.T, name string) []byte {
	t.Helper()
	recorded, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-release-lists-v0812-recorded", name))
	require.NoError(t, err)
	return recorded
}

// #758: the release and resource lists, held to the bytes cub printed for
// them on a real server. cub sorts the releases it is given; the test server
// sends them oldest first, as a server is free to, and the bytes still match.
func TestReleaseAndResourceListsAreWhatCubPrintedOnTheRealServer(t *testing.T) {
	releases := releaseListsRecording(t, "release-list.json")
	var recorded []struct {
		Release struct {
			SpaceID, SpaceSlug string
			ReleaseNum         int
		}
	}
	require.NoError(t, json.Unmarshal(releases, &recorded))
	require.Len(t, recorded, 2)
	require.Equal(t, []int{2, 1}, []int{recorded[0].Release.ReleaseNum, recorded[1].Release.ReleaseNum}, "cub printed the newest first")
	slug, id := recorded[0].Release.SpaceSlug, recorded[0].Release.SpaceID

	hub := newFakeHub(t)
	hub.spaces = []map[string]json.RawMessage{{"Space": json.RawMessage(`{"Slug":"` + slug + `","SpaceID":"` + id + `"}`)}}
	reader := hub.reader(Options{})
	ctx := context.Background()

	// The two entries in the other order, each exactly as recorded.
	var entries []json.RawMessage
	require.NoError(t, json.Unmarshal(releases, &entries))
	oldestFirst := "[" + string(entries[1]) + "," + string(entries[0]) + "]"
	releasePath := "/api/space/" + id + "/release"
	for _, served := range []string{string(releases), oldestFirst} {
		hub.raw[releasePath] = served
		for _, space := range []string{slug, id} {
			got, err := reader.ReleaseListJSON(ctx, space, Filter{})
			require.NoError(t, err)
			require.Equal(t, string(releases), string(got))
		}
	}
	require.Equal(t, releasePath, hub.requests()[len(hub.requests())-1].Path)
	require.Equal(t, "TagID", hub.includes[len(hub.includes)-1], "cub's releaseListInclude")

	first := releaseListsRecording(t, "release-list-first.json")
	hub.raw[releasePath] = string(first)
	got, err := reader.ReleaseListJSON(ctx, slug, Filter{Where: "ReleaseNum = 1"})
	require.NoError(t, err)
	require.Equal(t, string(first), string(got))
	require.Equal(t, "ReleaseNum = 1", hub.requests()[len(hub.requests())-1].Where)

	resources := releaseListsRecording(t, "resource-list.json")
	require.Greater(t, strings.Count(string(resources), `"Resource": {`), 1)
	hub.raw["/api/resource"] = string(resources)
	for _, space := range []string{slug, id} {
		got, err := reader.ResourceListJSON(ctx, space, Filter{})
		require.NoError(t, err)
		require.Equal(t, string(resources), string(got))
	}
	last := len(hub.requests()) - 1
	require.Equal(t, "/api/resource", hub.requests()[last].Path)
	require.Equal(t, "SpaceID = '"+id+"'", hub.requests()[last].Where)
	require.Equal(t, "TargetID", hub.includes[last], "cub's resourceListInclude")

	// The server had no unit events; what cub printed for none is what the
	// reader returns for none.
	events := releaseListsRecording(t, "unit-event-list.json")
	hub.otherLists = true
	got, err = reader.UnitEventListJSON(ctx, slug, Filter{})
	require.NoError(t, err)
	require.Equal(t, string(events), string(got))
}
