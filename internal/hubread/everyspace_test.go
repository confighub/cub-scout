package hubread

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/confighub/sdk/core/cubapi"
	"github.com/stretchr/testify/require"
)

// #758, batch D: a list across the whole organization is asked for by name,
// `--space '*'`, never by leaving the space out. For the kinds cub lists at
// the organization's endpoint it is the same request with no space AND-ed on:
// what the SDK's list helper sends for a filter that names no space.
func TestListsAcrossEverySpaceAreAskedForByName(t *testing.T) {
	hub, _, id := hubOfTheConnectedLane(t)
	hub.otherLists = true
	reader := hub.reader(Options{})
	ctx := context.Background()
	const elsewhere = "11111111-2222-4333-8444-555555555555"

	kinds := spaceListKinds(reader, ctx)
	kinds["unit"] = struct {
		read   func(space string, filter Filter) ([]byte, error)
		viaSDK func(where cubapi.Where, contains string) error
		entity string
	}{
		func(space string, f Filter) ([]byte, error) { return reader.UnitListJSON(ctx, space, f) },
		func(w cubapi.Where, contains string) error {
			_, err := cubapi.ListUnits(ctx, reader.client, w, cubapi.ListOpts{Include: unitListInclude, Contains: contains})
			return err
		}, "Unit"}
	for name, kind := range kinds {
		t.Run(name, func(t *testing.T) {
			var path string
			for _, filter := range []Filter{{}, {Where: "Slug != 'x'"}, {Contains: "a b"}} {
				before := len(hub.requests())
				require.NoError(t, kind.viaSDK(cubapi.NewWhere(filter.Where), filter.Contains))
				_, err := kind.read(EverySpace, filter)
				require.NoError(t, err)
				seen := hub.requests()[before:]
				require.Len(t, seen, 2, "the helper's request and the reader's; no space is looked up")
				require.Equal(t, seen[0].Path, seen[1].Path)
				require.Equal(t, hub.queries[before], hub.queries[before+1], "%+v", filter)
				require.NotContains(t, seen[1].Where, "SpaceID", "no space is AND-ed on")
				require.Equal(t, filter.Where, seen[1].Where)
				path = seen[1].Path
			}
			defer delete(hub.raw, path)
			defer delete(hub.more, path)
			defer delete(hub.status, path)

			// Entries of two spaces are the answer, not an error.
			hub.raw[path] = `[{"` + kind.entity + `":{"SpaceID":"` + id + `"}},{"` + kind.entity + `":{"SpaceID":"` + elsewhere + `"}}]`
			out, err := kind.read(EverySpace, Filter{})
			require.NoError(t, err)
			require.Equal(t, 2, strings.Count(string(out), `"SpaceID"`))
			// The same answer for one named space is still refused.
			_, err = kind.read(id, Filter{})
			require.Equal(t, KindMalformed, KindOf(err))

			// What is broken is still not read as a list.
			for what, tc := range map[string]struct {
				body string
				want Kind
			}{
				"an entry with no entity":    {`[{"Space":{}}]`, KindMalformed},
				"a page that is not the API": {`<html>sign in</html>`, KindMalformed},
				"an object, not a list":      {`{}`, KindMalformed},
			} {
				hub.raw[path] = tc.body
				out, err := kind.read(EverySpace, Filter{})
				require.Equal(t, tc.want, KindOf(err), what)
				require.Nil(t, out, what)
			}
			hub.raw[path], hub.more[path] = `[{"`+kind.entity+`":{"SpaceID":"`+id+`"}}]`, true
			_, err = kind.read(EverySpace, Filter{})
			require.Equal(t, KindIncomplete, KindOf(err))
			hub.more[path] = false
			delete(hub.raw, path)
			hub.status[path] = http.StatusForbidden
			_, err = kind.read(EverySpace, Filter{})
			require.Equal(t, KindForbidden, KindOf(err))
		})
	}

	// Unit events across the organization: the filter as written, newest first.
	before := len(hub.requests())
	_, err := reader.UnitEventListJSON(ctx, EverySpace, Filter{Where: "CreatedAt > '2026-10-01T00:00:00Z'"})
	require.NoError(t, err)
	require.Equal(t, "CreatedAt > '2026-10-01T00:00:00Z'", hub.requests()[before].Where)

	// A space's releases are at the space's own endpoint; across spaces cub
	// asks a different one and orders differently, which this does not do.
	_, err = reader.ReleaseListJSON(ctx, EverySpace, Filter{})
	require.Equal(t, KindInvalidScope, KindOf(err))
	// One Unit, its data, its events and its revisions are never "every space".
	for name, read := range map[string]func() error{
		"unit get":  func() error { _, err := reader.UnitJSON(ctx, EverySpace, recordedUnit); return err },
		"unit data": func() error { _, err := reader.UnitData(ctx, EverySpace, recordedUnit); return err },
		"unit events": func() error {
			_, err := reader.UnitEventsOfUnitJSON(ctx, EverySpace, recordedUnit, Filter{})
			return err
		},
		"revision":       func() error { _, err := reader.RevisionJSON(ctx, EverySpace, recordedUnit, 1); return err },
		"a change order": func() error { _, err := reader.ChangeOrderJSON(ctx, EverySpace, "order"); return err },
	} {
		require.Equal(t, KindInvalidScope, KindOf(read()), name)
	}
}
