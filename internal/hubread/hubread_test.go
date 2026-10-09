// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hubread

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	goclientnew "github.com/confighub/sdk/core/openapi/goclient-new"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	testToken = "test-token-do-not-print"
	// The Unit and Space in the recorded fixture.
	recordedSpace   = "scout-v213-contract"
	recordedSpaceID = "68843338-f9bd-485c-9a66-5d5aee820247"
	recordedUnit    = "contract-config"
	recordedUnitID  = "6221926e-8cc4-4847-a25c-a4104042f640"
)

// recordedUnitFixture is what a real ConfigHub v0.8.3 server returned for one
// Unit (test/fixtures/confighub-governance-v083-recorded/unit-get.json). The
// element is recorded; the list envelope and the server's handling of the
// where filter are simulated by fakeHub, and say nothing about a real server.
func recordedUnitFixture(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-governance-v083-recorded", "unit-get.json"))
	require.NoError(t, err)
	var fixture map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &fixture))
	require.Contains(t, fixture, "Unit")
	require.Contains(t, fixture, "Space")
	return fixture
}

// with returns the recorded element with fields of its Unit (or Space)
// replaced.
func with(t *testing.T, element map[string]json.RawMessage, object string, fields map[string]interface{}) map[string]json.RawMessage {
	t.Helper()
	var inner map[string]interface{}
	require.NoError(t, json.Unmarshal(element[object], &inner))
	for key, value := range fields {
		inner[key] = value
	}
	encoded, err := json.Marshal(inner)
	require.NoError(t, err)
	out := map[string]json.RawMessage{}
	for key, value := range element {
		out[key] = value
	}
	out[object] = encoded
	return out
}

type seenRequest struct {
	Method, Path, Where, Limit, Authorization, UserAgent string
}

type fakeHub struct {
	t      *testing.T
	server *httptest.Server
	mu     sync.Mutex
	seen   []seenRequest
	// spaces and units are what each list returns, whatever the filter says.
	spaces, units []map[string]json.RawMessage
	status        map[string]int    // path -> status to answer with
	raw           map[string]string // path -> body to answer with instead
	delay         time.Duration
}

func newFakeHub(t *testing.T) *fakeHub {
	t.Helper()
	fixture := recordedUnitFixture(t)
	hub := &fakeHub{t: t, status: map[string]int{}, raw: map[string]string{},
		spaces: []map[string]json.RawMessage{{"Space": fixture["Space"]}},
		units:  []map[string]json.RawMessage{fixture},
	}
	hub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.mu.Lock()
		hub.seen = append(hub.seen, seenRequest{Method: r.Method, Path: r.URL.Path, Where: r.URL.Query().Get("where"),
			Limit: r.URL.Query().Get("limit"), Authorization: r.Header.Get("Authorization"), UserAgent: r.Header.Get("User-Agent")})
		status, raw, delay := hub.status[r.URL.Path], hub.raw[r.URL.Path], hub.delay
		hub.mu.Unlock()
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"Message":"simulated failure"}`))
			return
		}
		if raw != "" {
			_, _ = w.Write([]byte(raw))
			return
		}
		switch r.URL.Path {
		case "/api/space":
			_ = json.NewEncoder(w).Encode(hub.spaces)
		case "/api/unit":
			_ = json.NewEncoder(w).Encode(hub.units)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(hub.server.Close)
	return hub
}

func (h *fakeHub) reader(opts Options) *Reader {
	reader, err := New(h.server.URL, testToken, opts)
	require.NoError(h.t, err)
	return reader
}

func (h *fakeHub) requests() []seenRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]seenRequest{}, h.seen...)
}

// #758: one exact-space Unit read, as two GETs, returning what the recorded
// server object says.
func TestUnitHeadReadsTheRecordedUnitWithTwoGets(t *testing.T) {
	hub := newFakeHub(t)
	reader := hub.reader(Options{UserAgent: "cub-scout-test"})

	head, err := reader.UnitHead(context.Background(), recordedSpace, recordedUnit)
	require.NoError(t, err)
	require.Equal(t, UnitHead{Space: recordedSpace, SpaceID: recordedSpaceID, Unit: recordedUnit, UnitID: recordedUnitID, HeadRevisionNum: 2}, head)

	require.Equal(t, []seenRequest{
		{Method: "GET", Path: "/api/space", Where: "Slug = '" + recordedSpace + "'", Limit: "2",
			Authorization: "Bearer " + testToken, UserAgent: "cub-scout-test"},
		{Method: "GET", Path: "/api/unit", Where: "Slug = '" + recordedUnit + "' AND SpaceID = '" + recordedSpaceID + "'", Limit: "2",
			Authorization: "Bearer " + testToken, UserAgent: "cub-scout-test"},
	}, hub.requests())
	stats := reader.Stats()
	require.EqualValues(t, 2, stats.Requests)
	require.Positive(t, stats.ResponseBytes)
	require.Equal(t, hub.server.URL, reader.Server())
}

// uuid.Parse accepts 32 bare hex digits, braces and a urn: prefix. A space
// could be named any of those; only the canonical form is taken as an ID, so
// the others still go through the lookup that checks the name.
func TestUnitHeadTreatsOnlyACanonicalUUIDAsASpaceID(t *testing.T) {
	bare := strings.ReplaceAll(recordedSpaceID, "-", "")
	for _, name := range []string{bare, "{" + recordedSpaceID + "}", "urn:uuid:" + recordedSpaceID} {
		hub := newFakeHub(t)
		_, err := hub.reader(Options{}).UnitHead(context.Background(), name, recordedUnit)
		require.Equal(t, KindNotFound, KindOf(err), "%q: %v", name, err)
		require.Len(t, hub.requests(), 1, "%q", name)
		require.Equal(t, "/api/space", hub.requests()[0].Path, "%q must be looked up as a slug", name)
		require.Equal(t, "Slug = '"+name+"'", hub.requests()[0].Where)
	}
	upper := newFakeHub(t)
	head, err := upper.reader(Options{}).UnitHead(context.Background(), strings.ToUpper(recordedSpaceID), recordedUnit)
	require.NoError(t, err, "the canonical form in upper case is still the ID")
	require.Equal(t, recordedSpaceID, head.SpaceID)
}

func TestUnitHeadWithASpaceUUIDSkipsTheSpaceLookup(t *testing.T) {
	hub := newFakeHub(t)
	head, err := hub.reader(Options{}).UnitHead(context.Background(), recordedSpaceID, recordedUnit)
	require.NoError(t, err)
	require.Equal(t, UnitHead{SpaceID: recordedSpaceID, Unit: recordedUnit, UnitID: recordedUnitID, HeadRevisionNum: 2}, head,
		"the slug of a space named by ID was not read, so it is not reported")
	require.Len(t, hub.requests(), 1)
	require.Equal(t, "/api/unit", hub.requests()[0].Path)
}

// A list with no space spans the organization. An empty or wildcard scope is
// refused before anything is sent.
func TestUnitHeadRefusesAnEmptyOrWildcardScopeBeforeAnyRequest(t *testing.T) {
	hub := newFakeHub(t)
	reader := hub.reader(Options{})
	for _, scope := range [][2]string{{"", recordedUnit}, {"  ", recordedUnit}, {"*", recordedUnit}, {recordedSpace, ""}, {recordedSpace, "*"},
		{"o'brien", recordedUnit}, {recordedSpace, `a\b`}} {
		_, err := reader.UnitHead(context.Background(), scope[0], scope[1])
		require.Error(t, err, "%q", scope)
		require.Equal(t, KindInvalidScope, KindOf(err), "%q", scope)
	}
	// A name the filter grammar cannot express is refused where the filter is
	// built, for the space and for the unit alike. Nothing was sent.
	require.Empty(t, hub.requests())
}

// The server's filter narrows the answer; it does not decide it. A Unit is
// only the one asked for if its slug and space match exactly.
func TestUnitHeadMatchesSlugAndSpaceExactly(t *testing.T) {
	fixture := recordedUnitFixture(t)

	caseOnly := newFakeHub(t)
	_, err := caseOnly.reader(Options{}).UnitHead(context.Background(), recordedSpace, "Contract-Config")
	require.Equal(t, KindNotFound, KindOf(err), "a slug that differs only in case is a different unit: %v", err)

	spaceCase := newFakeHub(t)
	_, err = spaceCase.reader(Options{}).UnitHead(context.Background(), "Scout-V213-Contract", recordedUnit)
	require.Equal(t, KindNotFound, KindOf(err), "%v", err)
	require.Len(t, spaceCase.requests(), 1, "no unit is read for a space that was not found")

	otherSpace := newFakeHub(t)
	otherSpace.units = []map[string]json.RawMessage{with(t, fixture, "Unit", map[string]interface{}{"SpaceID": uuid.NewString()})}
	_, err = otherSpace.reader(Options{}).UnitHead(context.Background(), recordedSpace, recordedUnit)
	require.Equal(t, KindNotFound, KindOf(err), "a same-named unit in another space is not this unit: %v", err)

	none := newFakeHub(t)
	none.units = nil
	_, err = none.reader(Options{}).UnitHead(context.Background(), recordedSpace, recordedUnit)
	require.Equal(t, KindNotFound, KindOf(err))
	require.ErrorContains(t, err, `no unit "contract-config" in space "scout-v213-contract"`)
}

func TestUnitHeadRefusesToChooseBetweenTwoMatches(t *testing.T) {
	fixture := recordedUnitFixture(t)

	units := newFakeHub(t)
	units.units = []map[string]json.RawMessage{fixture, with(t, fixture, "Unit", map[string]interface{}{"UnitID": uuid.NewString(), "HeadRevisionNum": 9})}
	_, err := units.reader(Options{}).UnitHead(context.Background(), recordedSpace, recordedUnit)
	require.Equal(t, KindAmbiguous, KindOf(err), "%v", err)

	spaces := newFakeHub(t)
	spaces.spaces = append(spaces.spaces, with(t, map[string]json.RawMessage{"Space": fixture["Space"]}, "Space", map[string]interface{}{"SpaceID": uuid.NewString()}))
	_, err = spaces.reader(Options{}).UnitHead(context.Background(), recordedSpace, recordedUnit)
	require.Equal(t, KindAmbiguous, KindOf(err), "%v", err)
	require.Len(t, spaces.requests(), 1)
}

// Each failure keeps its own reason. A forbidden read is not a login problem,
// and no message carries the credential.
func TestUnitHeadClassifiesFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		break_ func(*fakeHub)
		want   Kind
	}{
		{"unauthorized", func(h *fakeHub) { h.status["/api/space"] = 401 }, KindUnauthorized},
		{"forbidden on the unit", func(h *fakeHub) { h.status["/api/unit"] = 403 }, KindForbidden},
		{"not found", func(h *fakeHub) { h.status["/api/unit"] = 404 }, KindNotFound},
		{"server error", func(h *fakeHub) { h.status["/api/space"] = 500 }, KindFailed},
		{"truncated body", func(h *fakeHub) { h.raw["/api/unit"] = `[{"Unit":` }, KindMalformed},
		// A null list is an empty list: nothing matched.
		{"null body", func(h *fakeHub) { h.raw["/api/space"] = `null` }, KindNotFound},
		{"object where a list was promised", func(h *fakeHub) { h.raw["/api/unit"] = `{"Unit":{}}` }, KindMalformed},
		{"space without an ID", func(h *fakeHub) {
			h.spaces = []map[string]json.RawMessage{{"Space": json.RawMessage(`{"Slug":"` + recordedSpace + `"}`)}}
		}, KindMalformed},
		{"unit without an ID", func(h *fakeHub) {
			h.units = []map[string]json.RawMessage{{"Unit": json.RawMessage(`{"Slug":"` + recordedUnit + `","SpaceID":"` + recordedSpaceID + `","HeadRevisionNum":2}`)}}
		}, KindMalformed},
		{"list holding a null element", func(h *fakeHub) { h.raw["/api/unit"] = `[null]` }, KindNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hub := newFakeHub(t)
			tc.break_(hub)
			head, err := hub.reader(Options{}).UnitHead(context.Background(), recordedSpace, recordedUnit)
			require.Error(t, err)
			require.Equal(t, tc.want, KindOf(err), "%v", err)
			require.Equal(t, UnitHead{}, head, "a failed read returns nothing to mistake for an answer")
			require.NotContains(t, err.Error(), testToken)
		})
	}
}

func TestUnitHeadTimesOutAndHonoursCancellation(t *testing.T) {
	slow := newFakeHub(t)
	slow.delay = 300 * time.Millisecond
	_, err := slow.reader(Options{Timeout: 30 * time.Millisecond}).UnitHead(context.Background(), recordedSpace, recordedUnit)
	require.Equal(t, KindTimeout, KindOf(err), "%v", err)

	// A cancelled read is not a slow server.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := newFakeHub(t)
	_, err = cancelled.reader(Options{}).UnitHead(ctx, recordedSpace, recordedUnit)
	require.Equal(t, KindCanceled, KindOf(err), "%v", err)
	require.Empty(t, cancelled.requests())
}

// The reader holds the SDK's whole generated client, which can write. The
// transport is what makes that unreachable: nothing but GET and HEAD leaves.
func TestReaderTransportRefusesEveryWriteBeforeItIsSent(t *testing.T) {
	hub := newFakeHub(t)
	reader := hub.reader(Options{})
	ctx := context.Background()
	id := uuid.MustParse(recordedSpaceID)

	_, err := reader.client.API.DeleteSpaceWithResponse(ctx, id, &goclientnew.DeleteSpaceParams{})
	require.ErrorIs(t, err, errNotReadOnly)
	_, err = reader.client.API.DeleteUnitWithResponse(ctx, id, uuid.MustParse(recordedUnitID), &goclientnew.DeleteUnitParams{})
	require.ErrorIs(t, err, errNotReadOnly)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, "PROPFIND"} {
		req, reqErr := http.NewRequestWithContext(ctx, method, hub.server.URL+"/api/space", strings.NewReader("{}"))
		require.NoError(t, reqErr)
		_, err = reader.transport.RoundTrip(req)
		require.ErrorIs(t, err, errNotReadOnly, method)
	}
	require.Empty(t, hub.requests(), "a refused request never reached the server")
	require.Zero(t, reader.Stats().Requests)
}

// A redirect is not followed: it could carry the request somewhere the caller
// never named.
func TestReaderDoesNotFollowRedirects(t *testing.T) {
	var elsewhere []string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere = append(elsewhere, r.Method+" "+r.URL.Path+" "+r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`[]`))
	}))
	defer target.Close()
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusFound)
	}))
	defer redirecting.Close()

	reader, err := New(redirecting.URL, testToken, Options{})
	require.NoError(t, err)
	_, err = reader.UnitHead(context.Background(), recordedSpace, recordedUnit)
	require.Equal(t, KindRefused, KindOf(err), "%v", err)
	require.Empty(t, elsewhere, "the redirect target was contacted")
}

func TestNewAndResolveNeedACredentialAndWriteNothing(t *testing.T) {
	for _, pair := range [][2]string{{"", testToken}, {"http://127.0.0.1:1", ""}, {" ", " "}} {
		_, err := New(pair[0], pair[1], Options{})
		require.Equal(t, KindNotConfigured, KindOf(err), "%q", pair)
	}

	// As a cub plugin: CUB_SERVER and CUB_TOKEN from the environment.
	hub := newFakeHub(t)
	config, home := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CUB_CONFIG", config)
	t.Setenv("CUB_CONTEXT", "")
	t.Setenv("CUB_SPACE", "")
	t.Setenv("CUB_SERVER", hub.server.URL)
	t.Setenv("CUB_TOKEN", testToken)
	reader, err := Resolve(context.Background(), Options{})
	require.NoError(t, err)
	head, err := reader.UnitHead(context.Background(), recordedSpace, recordedUnit)
	require.NoError(t, err)
	require.Equal(t, recordedUnitID, head.UnitID)
	require.Equal(t, "Bearer "+testToken, hub.requests()[0].Authorization)

	// With no credential anywhere, Resolve says so. It does not create the
	// config directory's contents, log in, or fall back to anything.
	t.Setenv("CUB_SERVER", "")
	t.Setenv("CUB_TOKEN", "")
	_, err = Resolve(context.Background(), Options{})
	require.Equal(t, KindNotConfigured, KindOf(err), "%v", err)
	require.Empty(t, snapshotDir(t, config), "resolving credentials wrote into the cub config directory")

	// The same with no CUB_CONFIG, so the default under HOME is used: still
	// not configured, and HOME is left as it was.
	t.Setenv("CUB_CONFIG", "")
	_, err = Resolve(context.Background(), Options{})
	require.Equal(t, KindNotConfigured, KindOf(err), "%v", err)
	require.Empty(t, snapshotDir(t, home), "resolving credentials wrote under HOME")
}

// From a local cub configuration: the server and token of the selected
// context, read and not changed.
func TestResolveReadsTheSelectedCubContextWithoutChangingIt(t *testing.T) {
	hub := newFakeHub(t)
	config := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(config, "tokens"), 0o700))
	configYAML := `apiVersion: v1
kind: Config
currentContext: other
contexts:
  - name: other
    coordinate:
      serverURL: http://127.0.0.1:1
      organizationID: ` + uuid.NewString() + `
      user: someone@example.test
    settings: {}
    metadata:
      tokenFile: other.json
  - name: selected
    coordinate:
      serverURL: ` + hub.server.URL + `
      organizationID: ` + uuid.NewString() + `
      user: someone@example.test
    settings: {}
    metadata:
      tokenFile: selected.json
`
	require.NoError(t, os.WriteFile(filepath.Join(config, "config.yaml"), []byte(configYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(config, "tokens", "selected.json"), []byte(`{"accessToken":"`+testToken+`"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(config, "tokens", "other.json"), []byte(`{"accessToken":""}`), 0o600))
	before := snapshotDir(t, config)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CUB_CONFIG", config)
	t.Setenv("CUB_CONTEXT", "selected")
	t.Setenv("CUB_SERVER", "")
	t.Setenv("CUB_TOKEN", "")
	t.Setenv("CUB_SPACE", "")
	reader, err := Resolve(context.Background(), Options{})
	require.NoError(t, err)
	require.Equal(t, hub.server.URL, reader.Server(), "CUB_CONTEXT selects the context, not the file's current one")
	_, err = reader.UnitHead(context.Background(), recordedSpace, recordedUnit)
	require.NoError(t, err)
	require.Equal(t, "Bearer "+testToken, hub.requests()[0].Authorization)
	require.Equal(t, before, snapshotDir(t, config), "the cub configuration was modified")

	// A context whose token file holds no token is not configured. Nothing
	// is sent without a credential, and an unknown context is not replaced
	// by the file's current one.
	seen := len(hub.requests())
	for _, name := range []string{"other", "missing"} {
		t.Setenv("CUB_CONTEXT", name)
		_, err = Resolve(context.Background(), Options{})
		require.Equal(t, KindNotConfigured, KindOf(err), "context %q: %v", name, err)
		require.NotContains(t, err.Error(), testToken)
	}
	require.Len(t, hub.requests(), seen)

	// A token file that is not the expected JSON: the parser's complaint
	// quotes what it read, so the cause is not passed on.
	require.NoError(t, os.WriteFile(filepath.Join(config, "tokens", "other.json"), []byte(testToken), 0o600))
	t.Setenv("CUB_CONTEXT", "other")
	_, err = Resolve(context.Background(), Options{})
	require.Equal(t, KindNotConfigured, KindOf(err))
	require.Equal(t, "confighub resolve credentials: the token for context \"other\" could not be loaded; run `cub auth login` (not_configured)", err.Error())
	require.NoError(t, os.WriteFile(filepath.Join(config, "tokens", "other.json"), []byte(`{"accessToken":""}`), 0o600))

	require.Len(t, hub.requests(), seen)
	require.Equal(t, before, snapshotDir(t, config))
	require.Empty(t, snapshotDir(t, home), "resolving credentials wrote under HOME")
}

func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		out[strings.TrimPrefix(path, dir)] = string(data)
		return readErr
	}))
	return out
}
