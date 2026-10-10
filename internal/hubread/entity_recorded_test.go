package hubread

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// entitiesRecording is one file of what cub v0.8.12 printed on a real v0.8.12
// server, the SDK version this is built with (NOTICE.md beside the files).
// These are compared byte for byte; the older recordings in entity_test.go
// cannot be.
func entitiesRecording(t *testing.T, name string) []byte {
	t.Helper()
	recorded, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-entities-v0812-recorded", name))
	require.NoError(t, err)
	return recorded
}

// #758, batches B and C: a Unit's data, its revisions and their data, a View
// and a change order, each held to the bytes cub printed for it on a real
// server of this SDK's version.
func TestEntityReadsAreWhatCubPrintedOnTheRealServer(t *testing.T) {
	ctx := context.Background()
	head := entitiesRecording(t, "revision-3-get.json")
	var recorded struct {
		Revision struct{ RevisionID, UnitID, SpaceID string }
		Unit     json.RawMessage
	}
	require.NoError(t, json.Unmarshal(head, &recorded))
	var unit struct{ Slug, SpaceSlug, DataHash string }
	require.NoError(t, json.Unmarshal(recorded.Unit, &unit))
	spaceID, unitID := recorded.Revision.SpaceID, recorded.Revision.UnitID

	hub := newFakeHub(t)
	hub.spaces = []map[string]json.RawMessage{{"Space": json.RawMessage(`{"Slug":"` + unit.SpaceSlug + `","SpaceID":"` + spaceID + `"}`)}}
	hub.units = []map[string]json.RawMessage{{"Unit": recorded.Unit}}
	underUnit := "/api/space/" + spaceID + "/unit/" + unitID
	first := entitiesRecording(t, "revision-1-get.json")
	var firstID struct{ Revision struct{ RevisionID string } }
	require.NoError(t, json.Unmarshal(first, &firstID))
	hub.raw[underUnit+"/data"], hub.contentTypes[underUnit+"/data"] = string(entitiesRecording(t, "unit-data.txt")), "application/octet-stream"
	headData := underUnit + "/revision/" + recorded.Revision.RevisionID + "/data"
	hub.raw[headData], hub.contentTypes[headData] = string(entitiesRecording(t, "revision-3-data.txt")), "application/octet-stream"
	// The first revision has no data: the server answers with nothing.
	hub.contentTypes[underUnit+"/revision/"+firstID.Revision.RevisionID+"/data"] = "application/octet-stream"
	hub.raw["/api/change_order"] = "[" + string(entitiesRecording(t, "changeorder-get.json")) + "]"
	hub.raw["/api/view"] = "[" + string(entitiesRecording(t, "view-get.json")) + "]"
	reader := hub.reader(Options{})

	for _, space := range []string{unit.SpaceSlug, spaceID} {
		data, err := reader.UnitData(ctx, space, unit.Slug)
		require.NoError(t, err)
		require.Equal(t, string(entitiesRecording(t, "unit-data.txt")), string(data))

		hub.raw[underUnit+"/revision"] = "[" + string(head) + "]"
		revision, err := reader.RevisionJSON(ctx, space, unit.Slug, 3)
		require.NoError(t, err)
		require.Equal(t, string(head), string(revision))
		data, err = reader.RevisionData(ctx, space, unit.Slug, 3)
		require.NoError(t, err)
		require.Equal(t, string(entitiesRecording(t, "revision-3-data.txt")), string(data))

		// The revision the Unit was created with: it reads, and it has no
		// data, which is an error by either route and not an empty document.
		hub.raw[underUnit+"/revision"] = "[" + string(first) + "]"
		revision, err = reader.RevisionJSON(ctx, space, unit.Slug, 1)
		require.NoError(t, err)
		require.Equal(t, string(first), string(revision))
		data, err = reader.RevisionData(ctx, space, unit.Slug, 1)
		require.Equal(t, KindNotFound, KindOf(err))
		require.Nil(t, data)

		order, err := reader.ChangeOrderJSON(ctx, space, "entity-order")
		require.NoError(t, err)
		require.Equal(t, string(entitiesRecording(t, "changeorder-get.json")), string(order))
	}

	var view struct{ View struct{ ViewID string } }
	require.NoError(t, json.Unmarshal(entitiesRecording(t, "view-get.json"), &view))
	for _, space := range []string{"*", unit.SpaceSlug, spaceID} {
		got, err := reader.ViewJSON(ctx, space, view.View.ViewID)
		require.NoError(t, err, space)
		require.Equal(t, string(entitiesRecording(t, "view-get.json")), string(got), space)
	}
}
