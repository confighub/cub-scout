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

// governanceRecording is one file of what cub printed on a real ConfigHub
// v0.8.3 server for one Unit, its revision, their data and a change order
// (test/fixtures/confighub-governance-v083-recorded/NOTICE.md).
func governanceRecording(t *testing.T, name string) []byte {
	t.Helper()
	recorded, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-governance-v083-recorded", name))
	require.NoError(t, err)
	return recorded
}

// governanceHub serves the recorded Unit and its space, and nothing else
// until a test gives a path a body.
func governanceHub(t *testing.T) *fakeHub {
	t.Helper()
	var unit map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(governanceRecording(t, "unit-get.json"), &unit))
	hub := newFakeHub(t)
	hub.units = []map[string]json.RawMessage{unit}
	hub.spaces = []map[string]json.RawMessage{{"Space": unit["Space"]}}
	return hub
}

// sameButForNewerFields compares what the reader returned with a recording
// made by an older cub. The recordings here are from cub v0.8.3; this is built
// with a newer SDK, whose types have fields that cub did not have, and a UUID
// field is printed even when it is zero. So every value cub printed must be
// there unchanged, and anything added must be one of the named fields, zero.
// Bytes are compared on a server of the SDK's own version, in the Connected
// lane.
func sameButForNewerFields(t *testing.T, recorded, got []byte, added ...string) {
	t.Helper()
	var want, have interface{}
	require.NoError(t, json.Unmarshal(recorded, &want))
	require.NoError(t, json.Unmarshal(got, &have))
	extra := 0
	var strip func(value interface{}) interface{}
	strip = func(value interface{}) interface{} {
		switch typed := value.(type) {
		case map[string]interface{}:
			for _, field := range added {
				if typed[field] == "00000000-0000-0000-0000-000000000000" {
					delete(typed, field)
					extra++
				}
			}
			for key, inner := range typed {
				typed[key] = strip(inner)
			}
		case []interface{}:
			for i, inner := range typed {
				typed[i] = strip(inner)
			}
		}
		return value
	}
	require.Equal(t, want, strip(have))
	require.Positive(t, extra, "nothing was added; compare the bytes instead")
	require.True(t, strings.HasPrefix(string(got), "{\n  \"") && strings.HasSuffix(string(got), "\n}\n"), "encoded as cub encodes it")
}

const (
	unitDataPath     = "/api/space/" + recordedSpaceID + "/unit/" + recordedUnitID + "/data"
	revisionListPath = "/api/space/" + recordedSpaceID + "/unit/" + recordedUnitID + "/revision"
	recordedRevision = "5af31398-fd3d-4ac0-b267-4549eaabef37"
	revisionDataPath = revisionListPath + "/" + recordedRevision + "/data"
	elsewhereID      = "11111111-2222-4333-8444-555555555555"
)

// #758, batch C: a Unit's data is the bytes cub wrote for it on a real
// server, downloaded from the Unit's own endpoint after the Unit is found.
func TestUnitDataIsTheBytesCubWroteOnTheRealServer(t *testing.T) {
	hub := governanceHub(t)
	recorded := governanceRecording(t, "unit-data.txt")
	hub.raw[unitDataPath], hub.contentTypes[unitDataPath] = string(recorded), "application/octet-stream"
	reader := hub.reader(Options{})

	got, err := reader.UnitData(context.Background(), recordedSpace, recordedUnit)
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
	seen := hub.requests()
	require.Len(t, seen, 3, "the space, the Unit, its data")
	require.Equal(t, []string{"/api/space", "/api/unit", unitDataPath}, []string{seen[0].Path, seen[1].Path, seen[2].Path})
	for _, request := range seen {
		require.Equal(t, http.MethodGet, request.Method)
	}

	// By the space's ID: no lookup of the space, the same bytes.
	got, err = reader.UnitData(context.Background(), recordedSpaceID, recordedUnit)
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
	require.Len(t, hub.requests(), 5)
}

// The Unit reports the SHA-256 of its data. Bytes that do not have it are not
// the Unit's data, whatever they are, and are not returned.
func TestUnitDataIsHeldToTheHashTheUnitReports(t *testing.T) {
	recorded := string(governanceRecording(t, "unit-data.txt"))
	for name, served := range map[string]string{
		"a sign-in page":    "<html>sign in</html>",
		"a truncated body":  recorded[:len(recorded)/2],
		"an empty body":     "",
		"one byte more":     recorded + "\n",
		"another unit's":    strings.Replace(recorded, "contract", "contrast", 1),
		"a JSON error body": `{"Message":"not the data"}`,
	} {
		t.Run(name, func(t *testing.T) {
			hub := governanceHub(t)
			hub.raw[unitDataPath], hub.contentTypes[unitDataPath] = served, "application/octet-stream"
			if served == "" {
				// An empty raw body means "not set" to the fake.
				hub.contentTypes[unitDataPath] = "text/plain"
			}
			got, err := hub.reader(Options{}).UnitData(context.Background(), recordedSpace, recordedUnit)
			require.Equal(t, KindMalformed, KindOf(err), "%v", err)
			require.ErrorContains(t, err, "does not have the hash ConfigHub reports")
			var typed *Error
			require.ErrorAs(t, err, &typed)
			require.True(t, typed.Unverified, "marked, so that no caller is sent to a route that does not check")
			require.Nil(t, got)
			require.NotContains(t, err.Error(), "sign in", "what was downloaded is not quoted")
		})
	}

	// The servers recorded from report a hash for every Unit. One that
	// reports only a size is held to the size, and one that reports neither
	// cannot be checked: its data is returned as cub returns it.
	without := func(fields ...string) *fakeHub {
		hub := governanceHub(t)
		var unit map[string]interface{}
		require.NoError(t, json.Unmarshal(hub.units[0]["Unit"], &unit))
		for _, field := range fields {
			delete(unit, field)
		}
		stripped, err := json.Marshal(unit)
		require.NoError(t, err)
		hub.units[0]["Unit"] = stripped
		return hub
	}
	sized := without("DataHash")
	sized.raw[unitDataPath], sized.contentTypes[unitDataPath] = "<html>sign in</html>", "application/octet-stream"
	got, err := sized.reader(Options{}).UnitData(context.Background(), recordedSpace, recordedUnit)
	require.Equal(t, KindMalformed, KindOf(err))
	require.ErrorContains(t, err, "ConfigHub reports 113")
	require.Nil(t, got)
	var typed *Error
	require.ErrorAs(t, err, &typed)
	require.True(t, typed.Unverified)
	sized.raw[unitDataPath] = recorded
	got, err = sized.reader(Options{}).UnitData(context.Background(), recordedSpace, recordedUnit)
	require.NoError(t, err)
	require.Equal(t, recorded, string(got))

	unchecked := without("DataHash", "DataSize")
	unchecked.raw[unitDataPath], unchecked.contentTypes[unitDataPath] = "anything", "application/octet-stream"
	got, err = unchecked.reader(Options{}).UnitData(context.Background(), recordedSpace, recordedUnit)
	require.NoError(t, err)
	require.Equal(t, "anything", string(got))
}

func TestUnitDataFailuresKeepTheirReasons(t *testing.T) {
	ctx := context.Background()
	hub := governanceHub(t)
	reader := hub.reader(Options{})

	for _, scope := range []string{"", "*"} {
		_, err := reader.UnitData(ctx, scope, recordedUnit)
		require.Equal(t, KindInvalidScope, KindOf(err))
		_, err = reader.UnitData(ctx, recordedSpace, scope)
		require.Equal(t, KindInvalidScope, KindOf(err))
	}
	require.Empty(t, hub.requests(), "a refused scope sends nothing")

	_, err := reader.UnitData(ctx, recordedSpace, "no-such-unit")
	require.Equal(t, KindNotFound, KindOf(err))
	require.Len(t, hub.requests(), 2, "the data of a Unit that was not found is not asked for")
	_, err = reader.UnitData(ctx, "no-such-space", recordedUnit)
	require.Equal(t, KindNotFound, KindOf(err))

	// The Unit is there and the server has nowhere to download it from.
	hub.status[unitDataPath] = http.StatusNotFound
	_, err = reader.UnitData(ctx, recordedSpace, recordedUnit)
	require.Equal(t, KindFailed, KindOf(err))
	require.ErrorContains(t, err, "no data endpoint for the unit")

	for status, want := range map[int]Kind{http.StatusUnauthorized: KindUnauthorized, http.StatusForbidden: KindForbidden, http.StatusInternalServerError: KindFailed} {
		hub.status[unitDataPath] = status
		got, err := reader.UnitData(ctx, recordedSpace, recordedUnit)
		require.Equal(t, want, KindOf(err), "HTTP %d", status)
		require.Nil(t, got)
	}
}

// #758, batch B: one revision, as cub printed it on a real server.
func TestRevisionJSONIsWhatAnOlderCubPrinted(t *testing.T) {
	hub := governanceHub(t)
	recorded := governanceRecording(t, "revision-get.json")
	hub.raw[revisionListPath] = "[" + string(recorded) + "]"
	reader := hub.reader(Options{})

	got, err := reader.RevisionJSON(context.Background(), recordedSpace, recordedUnit, 2)
	require.NoError(t, err)
	sameButForNewerFields(t, recorded, got, "ReleaseTagID")
	seen := hub.requests()
	require.Len(t, seen, 3, "the space, the Unit, its revisions")
	require.Equal(t, revisionListPath, seen[2].Path)
	require.Equal(t, "RevisionNum = 2", seen[2].Where)
	// cub's revision commands ask for these expansions, and for every field.
	require.Equal(t, "UserID,UnitID,ChangeSetID,Tags,ChangeOrders,Releases", hub.includes[2])
	require.Equal(t, "include=UserID%2CUnitID%2CChangeSetID%2CTags%2CChangeOrders%2CReleases&where=RevisionNum+%3D+2", hub.queries[2])
}

func TestRevisionReadsFindExactlyOneRevisionOfTheUnit(t *testing.T) {
	ctx := context.Background()
	recorded := string(governanceRecording(t, "revision-get.json"))
	entry := func(mutate func(revision map[string]interface{})) string {
		var whole map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(recorded), &whole))
		mutate(whole["Revision"].(map[string]interface{}))
		encoded, err := json.Marshal(whole)
		require.NoError(t, err)
		return string(encoded)
	}
	for name, tc := range map[string]struct {
		body string
		want Kind
		says string
	}{
		"no such revision":            {"[]", KindNotFound, "no revision 2 of unit"},
		"only another number":         {"[" + entry(func(r map[string]interface{}) { r["RevisionNum"] = 3 }) + "]", KindNotFound, "no revision 2"},
		"the number twice":            {"[" + recorded + "," + recorded + "]", KindAmbiguous, "more than one revision 2"},
		"a revision of another unit":  {"[" + entry(func(r map[string]interface{}) { r["UnitID"] = elsewhereID }) + "]", KindMalformed, "a revision of another unit"},
		"a revision of another space": {"[" + entry(func(r map[string]interface{}) { r["SpaceID"] = elsewhereID }) + "]", KindMalformed, "a revision of another unit"},
		"an entry with no revision":   {`[{"Unit":{}}]`, KindMalformed, "an entry with no revision"},
		"a null entry":                {`[null]`, KindMalformed, "an entry with no revision"},
		"a page that is not the API":  {"<html>sign in</html>", KindMalformed, "no revision list"},
		"an object, not a list":       {"{}", KindMalformed, ""},
	} {
		t.Run(name, func(t *testing.T) {
			hub := governanceHub(t)
			hub.raw[revisionListPath] = tc.body
			reader := hub.reader(Options{})
			got, err := reader.RevisionJSON(ctx, recordedSpace, recordedUnit, 2)
			require.Equal(t, tc.want, KindOf(err), "%v", err)
			require.ErrorContains(t, err, tc.says)
			require.Nil(t, got)
			before := len(hub.requests())
			data, err := reader.RevisionData(ctx, recordedSpace, recordedUnit, 2)
			require.Equal(t, tc.want, KindOf(err))
			require.Nil(t, data)
			require.Len(t, hub.requests()[before:], 3, "the data of a revision that was not found is not asked for")
		})
	}

	hub := governanceHub(t)
	reader := hub.reader(Options{})
	for _, number := range []int64{0, -1} {
		_, err := reader.RevisionJSON(ctx, recordedSpace, recordedUnit, number)
		require.Equal(t, KindInvalidScope, KindOf(err))
		_, err = reader.RevisionData(ctx, recordedSpace, recordedUnit, number)
		require.Equal(t, KindInvalidScope, KindOf(err))
	}
	require.Empty(t, hub.requests())
	_, err := reader.RevisionJSON(ctx, recordedSpace, "no-such-unit", 2)
	require.Equal(t, KindNotFound, KindOf(err))
	require.Len(t, hub.requests(), 2, "the revisions of a Unit that was not found are not asked for")

	// Cut short, and rejected.
	hub.raw[revisionListPath], hub.more[revisionListPath] = "["+recorded+"]", true
	_, err = reader.RevisionJSON(ctx, recordedSpace, recordedUnit, 2)
	require.Equal(t, KindIncomplete, KindOf(err))
	delete(hub.raw, revisionListPath)
	hub.more[revisionListPath] = false
	hub.status[revisionListPath], hub.statusBody = http.StatusBadRequest, `{"Message":"unrecognized attribute name `+"`RevisionNum`"+`"}`
	_, err = reader.RevisionJSON(ctx, recordedSpace, recordedUnit, 2)
	require.ErrorContains(t, err, "the server rejected the request (HTTP 400): unrecognized attribute name")
	hub.status[revisionListPath], hub.statusBody = http.StatusForbidden, ""
	_, err = reader.RevisionJSON(ctx, recordedSpace, recordedUnit, 2)
	require.Equal(t, KindForbidden, KindOf(err))
}

// A revision's data is the bytes cub wrote for it, held to the hash the
// revision reports.
func TestRevisionDataIsTheBytesCubWroteOnTheRealServer(t *testing.T) {
	ctx := context.Background()
	hub := governanceHub(t)
	recorded := governanceRecording(t, "revision-data.txt")
	hub.raw[revisionListPath] = "[" + string(governanceRecording(t, "revision-get.json")) + "]"
	hub.raw[revisionDataPath], hub.contentTypes[revisionDataPath] = string(recorded), "application/octet-stream"
	reader := hub.reader(Options{})

	got, err := reader.RevisionData(ctx, recordedSpace, recordedUnit, 2)
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
	seen := hub.requests()
	require.Len(t, seen, 4, "the space, the Unit, the revision, its data")
	require.Equal(t, revisionDataPath, seen[3].Path)

	hub.raw[revisionDataPath] = string(recorded) + "x"
	got, err = reader.RevisionData(ctx, recordedSpace, recordedUnit, 2)
	require.Equal(t, KindMalformed, KindOf(err))
	require.ErrorContains(t, err, "does not have the hash ConfigHub reports")
	require.Nil(t, got)

	// No data at all is an error for cub too, not an empty document.
	delete(hub.raw, revisionDataPath)
	hub.contentTypes[revisionDataPath] = "text/plain"
	got, err = reader.RevisionData(ctx, recordedSpace, recordedUnit, 2)
	require.Equal(t, KindNotFound, KindOf(err))
	require.ErrorContains(t, err, "no config data for revision 2")
	require.Nil(t, got)

	delete(hub.contentTypes, revisionDataPath)
	hub.status[revisionDataPath] = http.StatusNotFound
	_, err = reader.RevisionData(ctx, recordedSpace, recordedUnit, 2)
	require.Equal(t, KindFailed, KindOf(err))
	require.ErrorContains(t, err, "no data endpoint for the revision")
}

// One change order by slug in one space, as cub printed it on a real server.
func TestChangeOrderJSONIsWhatAnOlderCubPrinted(t *testing.T) {
	ctx := context.Background()
	const path, slug = "/api/change_order", "contract-change"
	hub := governanceHub(t)
	recorded := string(governanceRecording(t, "changeorder-get.json"))
	hub.raw[path] = "[" + recorded + "]"
	reader := hub.reader(Options{})

	got, err := reader.ChangeOrderJSON(ctx, recordedSpace, slug)
	require.NoError(t, err)
	sameButForNewerFields(t, []byte(recorded), got, "ReleaseTagID")
	seen := hub.requests()
	require.Len(t, seen, 2)
	require.Equal(t, path, seen[1].Path)
	require.Equal(t, "Slug = '"+slug+"' AND SpaceID = '"+recordedSpaceID+"'", seen[1].Where)
	require.Equal(t, "SpaceID,StartTagID,EndTagID,RestoreTagID,InvocationID,UnitFilterID", hub.includes[1])

	other := strings.Replace(recorded, `"Slug": "contract-change"`, `"Slug": "contract-change-2"`, 1)
	require.NotEqual(t, recorded, other)
	elsewhere := strings.Replace(recorded, `"SpaceID": "`+recordedSpaceID+`"`, `"SpaceID": "`+elsewhereID+`"`, 1)
	require.NotEqual(t, recorded, elsewhere)
	for name, tc := range map[string]struct {
		body string
		want Kind
	}{
		"none":                       {"[]", KindNotFound},
		"an entry with no order":     {`[{"Space":{}}]`, KindMalformed},
		"only another slug":          {"[" + other + "]", KindNotFound},
		"only one in another space":  {"[" + elsewhere + "]", KindNotFound},
		"two":                        {"[" + recorded + "," + recorded + "]", KindAmbiguous},
		"a page that is not the API": {"<html>sign in</html>", KindMalformed},
	} {
		hub.raw[path] = tc.body
		got, err := reader.ChangeOrderJSON(ctx, recordedSpace, slug)
		require.Equal(t, tc.want, KindOf(err), name)
		require.Nil(t, got, name)
	}
	// The one asked for, among others the filter let through.
	hub.raw[path] = "[" + other + "," + recorded + "," + elsewhere + "]"
	again, err := reader.ChangeOrderJSON(ctx, recordedSpace, slug)
	require.NoError(t, err)
	require.Equal(t, string(got), string(again))

	before := len(hub.requests())
	for _, bad := range [][2]string{{"", slug}, {"*", slug}, {recordedSpace, ""}, {recordedSpace, "*"}, {recordedSpace, "it's"}} {
		_, err := reader.ChangeOrderJSON(ctx, bad[0], bad[1])
		require.Equal(t, KindInvalidScope, KindOf(err), "%q", bad)
	}
	require.Len(t, hub.requests(), before, "a refused scope sends nothing")
	hub.raw[path], hub.more[path] = "["+recorded+"]", true
	_, err = reader.ChangeOrderJSON(ctx, recordedSpace, slug)
	require.Equal(t, KindIncomplete, KindOf(err))
}

// One View by its ID, as cub printed it on a real server. An ID names one
// View wherever it is; a space that was named must still exist.
func TestViewJSONIsWhatCubPrintedOnTheRealServer(t *testing.T) {
	ctx := context.Background()
	const path, viewID = "/api/view", "cbfe0f5b-1398-4dab-8f5f-d8fb57993197"
	recorded, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-views-v083-recorded", "view-get.json"))
	require.NoError(t, err)
	hub := governanceHub(t)
	hub.raw[path] = "[" + string(recorded) + "]"
	reader := hub.reader(Options{})

	got, err := reader.ViewJSON(ctx, "*", viewID)
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
	seen := hub.requests()
	require.Len(t, seen, 1, "an ID and no space: one request")
	require.Equal(t, path, seen[0].Path)
	require.Equal(t, "ViewID = '"+viewID+"'", seen[0].Where, "by ID alone, as cub resolves an entity named by ID")
	require.Equal(t, "SpaceID,FilterID", hub.includes[0])

	// A named space is checked to exist, by slug or by ID, and the View is
	// still found by its ID alone.
	got, err = reader.ViewJSON(ctx, recordedSpace, viewID)
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
	got, err = reader.ViewJSON(ctx, recordedSpaceID, viewID)
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
	_, err = reader.ViewJSON(ctx, "no-such-space", viewID)
	require.Equal(t, KindNotFound, KindOf(err))
	require.ErrorContains(t, err, `no space "no-such-space"`)
	hub.spaces = nil
	_, err = reader.ViewJSON(ctx, recordedSpaceID, viewID)
	require.Equal(t, KindNotFound, KindOf(err))
	require.ErrorContains(t, err, "no space with ID")

	before := len(hub.requests())
	for _, bad := range []string{"", "shape-view", strings.ToUpper(viewID), strings.ReplaceAll(viewID, "-", ""), "{" + viewID + "}", "00000000-0000-0000-0000-000000000000"} {
		_, err := reader.ViewJSON(ctx, "*", bad)
		require.Equal(t, KindInvalidScope, KindOf(err), "%q", bad)
	}
	_, err = reader.ViewJSON(ctx, "", viewID)
	require.Equal(t, KindInvalidScope, KindOf(err))
	require.Len(t, hub.requests(), before, "a refused scope sends nothing")

	another := strings.Replace(string(recorded), viewID, elsewhereID, 1)
	for name, tc := range map[string]struct {
		body string
		want Kind
	}{
		"none":                        {"[]", KindNotFound},
		"an entry with no View":       {`[{"Space":{}}]`, KindMalformed},
		"only a View with another ID": {"[" + another + "]", KindNotFound},
		"two":                         {"[" + string(recorded) + "," + string(recorded) + "]", KindAmbiguous},
		"a page that is not the API":  {"<html>sign in</html>", KindMalformed},
	} {
		hub.raw[path] = tc.body
		got, err := reader.ViewJSON(ctx, "*", viewID)
		require.Equal(t, tc.want, KindOf(err), name)
		require.Nil(t, got, name)
	}
}

// A caller can bound what one read takes from the network. A response past
// the bound fails when the bound is passed, not after the rest has arrived.
func TestAResponseLimitStopsTheReadWhenItIsPassed(t *testing.T) {
	hub := governanceHub(t)
	big := strings.Repeat("x", 512*1024)
	hub.raw[unitDataPath], hub.contentTypes[unitDataPath] = big, "application/octet-stream"
	reader := hub.reader(Options{})

	_, err := reader.UnitData(WithResponseLimit(context.Background(), 64*1024), recordedSpace, recordedUnit)
	require.Equal(t, KindTooLarge, KindOf(err), "%v", err)
	require.Less(t, reader.Stats().ResponseBytes, int64(len(big)/2), "the rest of the body was not read")

	// Under the bound, and with no bound, the read is as before: here it
	// fails only because these bytes are not the Unit's data.
	for _, ctx := range []context.Context{WithResponseLimit(context.Background(), 1024*1024), context.Background()} {
		_, err = reader.UnitData(ctx, recordedSpace, recordedUnit)
		require.Equal(t, KindMalformed, KindOf(err))
	}
	// The bound is on each response, the lookups included.
	_, err = reader.UnitData(WithResponseLimit(context.Background(), 16), recordedSpace, recordedUnit)
	require.Equal(t, KindTooLarge, KindOf(err))
}
