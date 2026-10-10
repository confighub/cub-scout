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

	"github.com/confighub/sdk/core/cubapi"
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
	// includes is the include parameter of each request, in order.
	includes []string
	// queries is every query parameter of each request, sorted by name.
	queries []string
	// statusBody is the body sent with a failing status, when set.
	statusBody string
	// otherLists makes every path but the unit and space lists answer with
	// an empty list rather than 404.
	otherLists bool
	// more is the paths whose answers carry a continue token: the server
	// saying it returned only part of the list.
	more map[string]bool
	// spaces and units are what each list returns, whatever the filter says.
	spaces, units []map[string]json.RawMessage
	status        map[string]int    // path -> status to answer with
	raw           map[string]string // path -> body to answer with instead
	delay         time.Duration
}

func newFakeHub(t *testing.T) *fakeHub {
	t.Helper()
	fixture := recordedUnitFixture(t)
	hub := &fakeHub{t: t, status: map[string]int{}, raw: map[string]string{}, more: map[string]bool{},
		spaces: []map[string]json.RawMessage{{"Space": fixture["Space"]}},
		units:  []map[string]json.RawMessage{fixture},
	}
	hub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.mu.Lock()
		hub.seen = append(hub.seen, seenRequest{Method: r.Method, Path: r.URL.Path, Where: r.URL.Query().Get("where"),
			Limit: r.URL.Query().Get("limit"), Authorization: r.Header.Get("Authorization"), UserAgent: r.Header.Get("User-Agent")})
		hub.queries = append(hub.queries, r.URL.Query().Encode())
		hub.includes = append(hub.includes, r.URL.Query().Get("include"))
		status, raw, delay, statusBody := hub.status[r.URL.Path], hub.raw[r.URL.Path], hub.delay, hub.statusBody
		if hub.more[r.URL.Path] {
			w.Header().Set("ConfigHub-Continue", "next-page")
		}
		hub.mu.Unlock()
		time.Sleep(delay)
		if strings.HasPrefix(raw, "<") {
			// Not the API: a proxy or a login page answering 200.
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(raw))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			body := `{"Message":"simulated failure"}`
			if statusBody != "" {
				body = statusBody
			}
			_, _ = w.Write([]byte(body))
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
			// Any other list of the API is empty here, unless a test gave
			// the path a body or a status above.
			if hub.otherLists && strings.HasPrefix(r.URL.Path, "/api/") {
				_, _ = w.Write([]byte("[]"))
				return
			}
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
		// A list that matches nothing is an empty list; a 404 is a missing
		// endpoint, and must not read as "no such Unit".
		{"endpoint missing", func(h *fakeHub) { h.status["/api/unit"] = 404 }, KindFailed},
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

	// The pair alone is not a credential: the cub CLI does not read it, so
	// using it would send this reader to a server cub is not talking to. It
	// counts only when cub itself set it, which CUB_PLUGIN=1 says.
	t.Setenv("CUB_PLUGIN", "")
	_, err := Resolve(context.Background(), Options{})
	require.Equal(t, KindNotConfigured, KindOf(err), "%v", err)
	require.Empty(t, hub.requests())

	t.Setenv("CUB_PLUGIN", "1")
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

// UnitJSON must be what `cub unit get -o json` prints. The fixture is that
// output, recorded from a real server; served back as the list element, it has
// to come out of the SDK's types and the same marshalling unchanged. A field
// the typed client does not know, or names differently, would show here.
func TestUnitJSONIsWhatCubPrintedForTheRecordedUnit(t *testing.T) {
	recorded, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-governance-v083-recorded", "unit-get.json"))
	require.NoError(t, err)
	hub := newFakeHub(t)
	reader := hub.reader(Options{})

	got, err := reader.UnitJSON(context.Background(), recordedSpace, recordedUnit)
	require.NoError(t, err)
	// Byte for byte: the same fields in the same order, indented two spaces,
	// with one trailing newline.
	require.Equal(t, string(recorded), string(got))

	// The same scope rules and failures as every other read.
	_, err = reader.UnitJSON(context.Background(), "*", recordedUnit)
	require.Equal(t, KindInvalidScope, KindOf(err))
	hub.status["/api/unit"] = http.StatusForbidden
	out, err := reader.UnitJSON(context.Background(), recordedSpace, recordedUnit)
	require.Equal(t, KindForbidden, KindOf(err))
	require.Nil(t, out, "a failed read returns no bytes to parse")
}

// A second real object: what cub v0.8.3 printed for a Unit on the disposable
// server of the Connected lane, where the reader's own answer from that server
// was the same bytes (see the fixture's NOTICE). Served back from a test
// server it must encode to those bytes again, so an SDK bump that changes the
// encoding of a real Unit is noticed without a server.
func TestUnitJSONReproducesTheConnectedLaneRecording(t *testing.T) {
	recorded, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-sdk-parity-v083-recorded", "unit-get.json"))
	require.NoError(t, err)
	var element map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorded, &element))
	var names struct{ Unit, Space struct{ Slug string } }
	require.NoError(t, json.Unmarshal(recorded, &names))
	require.NotEmpty(t, names.Unit.Slug)
	require.NotEmpty(t, names.Space.Slug)

	hub := newFakeHub(t)
	hub.spaces = []map[string]json.RawMessage{{"Space": element["Space"]}}
	hub.units = []map[string]json.RawMessage{element}
	got, err := hub.reader(Options{}).UnitJSON(context.Background(), names.Space.Slug, names.Unit.Slug)
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
}

// The reader asks the server for the Unit the way cub does: cub calls the SDK's
// ResolveUnit, so the filter and the expansions ResolveUnit sends are the
// reference. If an SDK bump changes either, this fails and the constant here
// is updated with it.
func TestUnitLookupSendsWhatTheSDKResolverSends(t *testing.T) {
	hub := newFakeHub(t)
	reader := hub.reader(Options{})
	ctx := context.Background()

	_, err := cubapi.ResolveUnit(ctx, reader.client, cubapi.NewRef(recordedSpaceID, recordedUnit), cubapi.ResolveOpts{})
	require.NoError(t, err)
	require.Len(t, hub.requests(), 1)
	viaSDK, sdkInclude := hub.requests()[0], hub.includes[0]

	_, err = reader.UnitJSON(ctx, recordedSpaceID, recordedUnit)
	require.NoError(t, err)
	require.Len(t, hub.requests(), 2)
	viaReader, readerInclude := hub.requests()[1], hub.includes[1]

	require.Equal(t, viaSDK.Path, viaReader.Path)
	require.Equal(t, viaSDK.Where, viaReader.Where, "the same exact-space filter")
	require.NotEmpty(t, sdkInclude)
	require.Equal(t, sdkInclude, readerInclude, "the same expansions, so the envelope has the same related entities")
	require.Equal(t, unitGetInclude, readerInclude)
	// One deliberate difference: the reader bounds the answer.
	require.Empty(t, viaSDK.Limit)
	require.Equal(t, "2", viaReader.Limit)

	// The space named by slug, as most callers name it: both look the space
	// up first, then ask for the Unit in it with the same filter.
	_, err = cubapi.ResolveUnit(ctx, reader.client, cubapi.NewRef(recordedSpace, recordedUnit), cubapi.ResolveOpts{})
	require.NoError(t, err)
	sdkBySlug := hub.requests()[2:]
	_, err = reader.UnitJSON(ctx, recordedSpace, recordedUnit)
	require.NoError(t, err)
	readerBySlug := hub.requests()[2+len(sdkBySlug):]
	require.Len(t, sdkBySlug, 2)
	require.Len(t, readerBySlug, 2)
	for i := range sdkBySlug {
		require.Equal(t, sdkBySlug[i].Path, readerBySlug[i].Path)
		require.Equal(t, sdkBySlug[i].Where, readerBySlug[i].Where)
	}
	require.Equal(t, []string{"/api/space", "/api/unit"}, []string{readerBySlug[0].Path, readerBySlug[1].Path})
}

// connectedLaneRecording is what cub v0.8.3 printed on the disposable server of
// the Connected lane (see the fixture's NOTICE).
func connectedLaneRecording(t *testing.T, name string) ([]byte, []map[string]json.RawMessage) {
	t.Helper()
	recorded, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-sdk-parity-v083-recorded", name))
	require.NoError(t, err)
	var elements []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recorded, &elements))
	require.NotEmpty(t, elements)
	return recorded, elements
}

// hubOfTheConnectedLane serves the recorded lists, and returns the recorded
// space's slug and ID.
func hubOfTheConnectedLane(t *testing.T) (hub *fakeHub, slug, id string) {
	t.Helper()
	_, units := connectedLaneRecording(t, "unit-list.json")
	_, spaces := connectedLaneRecording(t, "space-list.json")
	var space struct{ Slug, SpaceID string }
	require.NoError(t, json.Unmarshal(units[0]["Space"], &space))
	hub = newFakeHub(t)
	hub.units, hub.spaces = units, spaces
	return hub, space.Slug, space.SpaceID
}

// #758: the lists, held to what cub printed for them on a real server.
func TestUnitListJSONIsWhatCubPrintedOnTheRealServer(t *testing.T) {
	recorded, _ := connectedLaneRecording(t, "unit-list.json")
	hub, slug, id := hubOfTheConnectedLane(t)
	reader := hub.reader(Options{})

	got, err := reader.UnitListJSON(context.Background(), slug, UnitFilter{})
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
	require.Equal(t, []string{"/api/space", "/api/unit"}, []string{hub.requests()[0].Path, hub.requests()[1].Path})

	// By the space's ID: one request, the same bytes.
	got, err = reader.UnitListJSON(context.Background(), id, UnitFilter{})
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
	require.Len(t, hub.requests(), 3)
}

func TestSpaceListJSONIsWhatCubPrintedOnTheRealServer(t *testing.T) {
	recorded, _ := connectedLaneRecording(t, "space-list.json")
	hub, _, _ := hubOfTheConnectedLane(t)

	got, err := hub.reader(Options{}).SpaceListJSON(context.Background())
	require.NoError(t, err)
	require.Equal(t, string(recorded), string(got))
	require.Len(t, hub.requests(), 1)
}

// The lists ask the server what cub asks. cub calls the SDK's list helpers
// with these options, so the whole query each sends is compared.
func TestListsSendWhatTheSDKListHelpersSend(t *testing.T) {
	hub, _, id := hubOfTheConnectedLane(t)
	reader := hub.reader(Options{})
	ctx := context.Background()
	spaceID := uuid.MustParse(id)

	_, err := cubapi.ListUnits(ctx, reader.client, cubapi.Where{}.SpaceID(spaceID), cubapi.ListOpts{Include: unitListInclude})
	require.NoError(t, err)
	_, err = reader.UnitListJSON(ctx, id, UnitFilter{})
	require.NoError(t, err)
	seen := hub.requests()
	require.Len(t, seen, 2)
	require.Equal(t, seen[0].Path, seen[1].Path)
	require.Equal(t, hub.queries[0], hub.queries[1])
	require.Contains(t, hub.queries[1], "include=UnitEventID")
	require.Empty(t, seen[1].Limit, "no limit: the whole list in one request, as cub asks for it")

	_, err = cubapi.ListSpaces(ctx, reader.client, cubapi.Where{}, cubapi.ListOpts{Include: spaceListInclude},
		func(p *goclientnew.ListSpacesParams) { summary := true; p.Summary = &summary })
	require.NoError(t, err)
	_, err = reader.SpaceListJSON(ctx)
	require.NoError(t, err)
	seen = hub.requests()[2:]
	require.Len(t, seen, 2)
	require.Equal(t, seen[0].Path, seen[1].Path)
	require.Equal(t, hub.queries[2], hub.queries[3])
	require.Equal(t, "include=ComponentID&summary=true", hub.queries[3])
}

// A unit list is for exactly one space, and nothing from another space is
// ever returned as part of it.
func TestUnitListJSONKeepsToOneSpace(t *testing.T) {
	hub, slug, id := hubOfTheConnectedLane(t)
	reader := hub.reader(Options{})
	ctx := context.Background()

	for _, scope := range []string{"", " ", "*"} {
		_, err := reader.UnitListJSON(ctx, scope, UnitFilter{})
		require.Equal(t, KindInvalidScope, KindOf(err), "scope %q", scope)
	}
	require.Empty(t, hub.requests(), "a refused scope sends nothing")

	// The server ignores the filter and returns the recorded Units for a
	// different space: an error, not that space's list.
	other := "11111111-2222-4333-8444-555555555555"
	out, err := reader.UnitListJSON(ctx, other, UnitFilter{})
	require.Equal(t, KindMalformed, KindOf(err))
	require.ErrorContains(t, err, "another space")
	require.Nil(t, out)

	_, err = reader.UnitListJSON(ctx, "no-such-space", UnitFilter{})
	require.Equal(t, KindNotFound, KindOf(err))

	for name, tc := range map[string]struct {
		body string
		want Kind
		out  string
	}{
		"no units":                   {`[]`, "", "[]\n"},
		"a null list":                {`null`, "", "[]\n"},
		"an entry with no unit":      {`[{"Space":{}}]`, KindMalformed, ""},
		"a null entry":               {`[null]`, KindMalformed, ""},
		"an object, not a list":      {`{"Unit":{}}`, KindMalformed, ""},
		"a truncated list":           {`[{"Unit":`, KindMalformed, ""},
		"a page that is not the API": {`<html>sign in</html>`, KindMalformed, ""},
	} {
		t.Run(name, func(t *testing.T) {
			hub.raw["/api/unit"] = tc.body
			out, err := reader.UnitListJSON(ctx, id, UnitFilter{})
			if tc.want == "" {
				require.NoError(t, err)
				require.Equal(t, tc.out, string(out))
				return
			}
			require.Equal(t, tc.want, KindOf(err))
			require.Nil(t, out, "a failed read returns no bytes to parse")
		})
	}
	delete(hub.raw, "/api/unit")

	hub.status["/api/unit"] = http.StatusForbidden
	_, err = reader.UnitListJSON(ctx, slug, UnitFilter{})
	require.Equal(t, KindForbidden, KindOf(err))
}

// A space named by its ID is not looked up first, so an empty list could mean
// a space with no Units or no such space. cub says which; so must the reader.
func TestUnitListJSONOfASpaceIDThatNamesNoSpaceIsNotAnEmptyList(t *testing.T) {
	hub, _, id := hubOfTheConnectedLane(t)
	hub.units = []map[string]json.RawMessage{}
	reader := hub.reader(Options{})
	ctx := context.Background()

	out, err := reader.UnitListJSON(ctx, "11111111-2222-4333-8444-555555555555", UnitFilter{})
	require.Equal(t, KindNotFound, KindOf(err))
	require.ErrorContains(t, err, "no space with ID")
	require.Nil(t, out)
	require.Equal(t, []string{"/api/unit", "/api/space"}, []string{hub.requests()[0].Path, hub.requests()[1].Path})
	require.Equal(t, "limit=2&where=SpaceID+%3D+%2711111111-2222-4333-8444-555555555555%27", hub.queries[1])

	// The recorded space exists and has no Units here: an empty list.
	out, err = reader.UnitListJSON(ctx, id, UnitFilter{})
	require.NoError(t, err)
	require.Equal(t, "[]\n", string(out))
	require.Len(t, hub.requests(), 4)

	// The check itself can fail, and then the list is not "empty".
	hub.status["/api/space"] = http.StatusForbidden
	out, err = reader.UnitListJSON(ctx, id, UnitFilter{})
	require.Equal(t, KindForbidden, KindOf(err))
	require.Nil(t, out)
}

// Asked for no limit, the server returns every entity. If it says it returned
// only part, the part is not the list.
func TestAListTheServerCutShortIsAnError(t *testing.T) {
	hub, slug, _ := hubOfTheConnectedLane(t)
	reader := hub.reader(Options{})
	ctx := context.Background()

	hub.more["/api/unit"] = true
	out, err := reader.UnitListJSON(ctx, slug, UnitFilter{})
	require.Equal(t, KindIncomplete, KindOf(err))
	require.Nil(t, out)

	hub.more["/api/unit"], hub.more["/api/space"] = false, true
	out, err = reader.SpaceListJSON(ctx)
	require.Equal(t, KindIncomplete, KindOf(err))
	require.Nil(t, out)

	// A single-entity lookup is bounded on purpose and is not a list read.
	hub.more["/api/unit"] = true
	_, err = reader.UnitJSON(ctx, slug, "parity-unit")
	require.NoError(t, err)
}

// An empty list and a JSON null are both a list with nothing in it, as cub
// reads them; anything broken is an error, never an empty list.
func TestSpaceListJSONReadsAnEmptyListAndRefusesABrokenOne(t *testing.T) {
	hub, _, _ := hubOfTheConnectedLane(t)
	reader := hub.reader(Options{})
	ctx := context.Background()

	for name, tc := range map[string]struct {
		body string
		want Kind
	}{
		"no spaces":                  {`[]`, ""},
		"a null list":                {`null`, ""},
		"an entry with no space":     {`[{"TotalUnitCount":2}]`, KindMalformed},
		"a page that is not the API": {`<html>sign in</html>`, KindMalformed},
	} {
		t.Run(name, func(t *testing.T) {
			hub.raw["/api/space"] = tc.body
			out, err := reader.SpaceListJSON(ctx)
			if tc.want == "" {
				require.NoError(t, err)
				require.Equal(t, "[]\n", string(out))
				return
			}
			require.Equal(t, tc.want, KindOf(err))
			require.Nil(t, out)
		})
	}
	delete(hub.raw, "/api/space")
	hub.status["/api/space"] = http.StatusUnauthorized
	_, err := reader.SpaceListJSON(ctx)
	require.Equal(t, KindUnauthorized, KindOf(err))
}

// A filtered list asks the server what cub asks: the caller's expression
// AND-ed with the space, and the search beside it.
func TestFilteredUnitListSendsWhatTheSDKListHelperSends(t *testing.T) {
	hub, slug, id := hubOfTheConnectedLane(t)
	reader := hub.reader(Options{})
	ctx := context.Background()
	spaceID := uuid.MustParse(id)

	for _, filter := range []UnitFilter{
		{Where: "Slug LIKE 'parity-%'"},
		{Contains: "parity"},
		{Where: "  Slug != 'x' AND DisplayName ILIKE '%a%'  ", Contains: "a b"},
	} {
		before := len(hub.requests())
		_, err := cubapi.ListUnits(ctx, reader.client, cubapi.NewWhere(filter.Where).SpaceID(spaceID),
			cubapi.ListOpts{Include: unitListInclude, Contains: filter.Contains})
		require.NoError(t, err)
		_, err = reader.UnitListJSON(ctx, id, filter)
		require.NoError(t, err)
		require.Len(t, hub.requests(), before+2)
		require.Equal(t, hub.queries[before], hub.queries[before+1], "%+v", filter)
		require.Contains(t, hub.requests()[before+1].Where, "SpaceID = '"+id+"'")
	}

	// The filter never widens the scope. Whatever it says, a Unit from
	// another space is an error; here the server ignores the filter and
	// answers a list for another space with the recorded Units.
	out, err := reader.UnitListJSON(ctx, "11111111-2222-4333-8444-555555555555", UnitFilter{Where: "Slug LIKE '%'"})
	require.Equal(t, KindMalformed, KindOf(err))
	require.Nil(t, out)

	// With a filter, the error says the filter may be why, and shows nothing.
	require.ErrorContains(t, err, "the filter may reach beyond the space")
	_, err = reader.UnitListJSON(ctx, "11111111-2222-4333-8444-555555555555", UnitFilter{Contains: "parity"})
	require.Equal(t, KindMalformed, KindOf(err))
	require.NotContains(t, err.Error(), "the filter may reach")

	// A filter that matches nothing in a space that exists is an empty list.
	hub.units = []map[string]json.RawMessage{}
	out, err = reader.UnitListJSON(ctx, slug, UnitFilter{Where: "Slug = 'none'"})
	require.NoError(t, err)
	require.Equal(t, "[]\n", string(out))
}

// The server says what is wrong with a filter it rejects. The caller wrote the
// filter, so the reason reaches them, cleaned and bounded.
func TestARejectedFilterKeepsTheServersReason(t *testing.T) {
	hub, slug, _ := hubOfTheConnectedLane(t)
	reader := hub.reader(Options{})
	ctx := context.Background()

	hub.status["/api/unit"] = http.StatusBadRequest
	hub.statusBody = `{"Code":"400","Message":"unknown field \"Nope\"\n\tin filter\u0007"}`
	out, err := reader.UnitListJSON(ctx, slug, UnitFilter{Where: "Nope = 1"})
	require.Equal(t, KindFailed, KindOf(err))
	require.Nil(t, out)
	require.ErrorContains(t, err, `the server rejected the request (HTTP 400): unknown field "Nope"  in filter`)
	require.NotContains(t, err.Error(), "\n")
	require.NotContains(t, err.Error(), "\a")
	require.NotContains(t, err.Error(), testToken)

	hub.statusBody = `{"Message":"` + strings.Repeat("é", 500) + `"}`
	_, err = reader.UnitListJSON(ctx, slug, UnitFilter{Where: "Nope = 1"})
	require.ErrorContains(t, err, strings.Repeat("é", 300)+"…")
	require.NotContains(t, err.Error(), strings.Repeat("é", 301))

	// Nothing in the message can act on a terminal: no C1 control (a
	// one-character escape introducer), no bidirectional override, no line
	// or paragraph separator.
	hub.statusBody = `{"Message":"a\u009b31mred\u202eesrever\u2028next\u2029para\u200bzero\ufeff"}`
	_, err = reader.UnitListJSON(ctx, slug, UnitFilter{Where: "Nope = 1"})
	require.ErrorContains(t, err, "(HTTP 400): a 31mred esrever next para zero")
	for _, hidden := range []string{"\u009b", "\u202e", "\u2028", "\u2029", "\u200b", "\ufeff"} {
		require.NotContains(t, err.Error(), hidden)
	}

	// With no message to pass on, the status is the reason.
	hub.statusBody = `{}`
	_, err = reader.UnitListJSON(ctx, slug, UnitFilter{Where: "Nope = 1"})
	require.ErrorContains(t, err, "HTTP 400")

	// Only a 400 is the caller's to read: another failure keeps its own words.
	hub.status["/api/unit"] = http.StatusForbidden
	hub.statusBody = `{"Message":"details that are not shown"}`
	_, err = reader.UnitListJSON(ctx, slug, UnitFilter{Where: "Nope = 1"})
	require.Equal(t, KindForbidden, KindOf(err))
	require.NotContains(t, err.Error(), "details that are not shown")
}

// A caller that asks errors.Is for the context package's errors finds a
// cancelled or timed-out read, and nothing else.
func TestErrorIsTheContextErrorItStandsFor(t *testing.T) {
	require.ErrorIs(t, &Error{Kind: KindCanceled}, context.Canceled)
	require.ErrorIs(t, &Error{Kind: KindTimeout}, context.DeadlineExceeded)
	require.NotErrorIs(t, &Error{Kind: KindTimeout}, context.Canceled)
	require.NotErrorIs(t, &Error{Kind: KindCanceled}, context.DeadlineExceeded)
	require.NotErrorIs(t, &Error{Kind: KindFailed}, context.Canceled)
	require.NotErrorIs(t, &Error{Kind: KindForbidden}, context.DeadlineExceeded)

	// From a real read: a context cancelled before the request.
	hub := newFakeHub(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := hub.reader(Options{}).UnitJSON(ctx, recordedSpace, recordedUnit)
	require.ErrorIs(t, err, context.Canceled)
}

// spaceListKinds are the lists of one space the reader makes besides Units:
// how the reader reads each, how the SDK's own helper does with the options
// cub passes, and the key under which an entry carries its entity.
func spaceListKinds(reader *Reader, ctx context.Context) map[string]struct {
	read   func(space string, filter Filter) ([]byte, error)
	viaSDK func(where cubapi.Where, contains string) error
	entity string
} {
	type kind = struct {
		read   func(space string, filter Filter) ([]byte, error)
		viaSDK func(where cubapi.Where, contains string) error
		entity string
	}
	return map[string]kind{
		"worker": {
			func(space string, f Filter) ([]byte, error) { return reader.WorkerListJSON(ctx, space, f) },
			func(w cubapi.Where, contains string) error {
				_, err := cubapi.ListBridgeWorkers(ctx, reader.client, w, cubapi.ListOpts{Include: workerListInclude, Contains: contains})
				return err
			}, "BridgeWorker"},
		"target": {
			func(space string, f Filter) ([]byte, error) { return reader.TargetListJSON(ctx, space, f) },
			func(w cubapi.Where, contains string) error {
				_, err := cubapi.ListTargets(ctx, reader.client, w, cubapi.ListOpts{Include: targetListInclude, Contains: contains})
				return err
			}, "Target"},
		"changeset": {
			func(space string, f Filter) ([]byte, error) { return reader.ChangeSetListJSON(ctx, space, f) },
			func(w cubapi.Where, contains string) error {
				_, err := cubapi.ListChangeSets(ctx, reader.client, w, cubapi.ListOpts{Include: changeSetListInclude, Contains: contains})
				return err
			}, "ChangeSet"},
		"link": {
			func(space string, f Filter) ([]byte, error) { return reader.LinkListJSON(ctx, space, f) },
			func(w cubapi.Where, contains string) error {
				_, err := cubapi.ListLinks(ctx, reader.client, w, cubapi.ListOpts{Include: linkListInclude, Contains: contains})
				return err
			}, "Link"},
		"attestation": {
			func(space string, f Filter) ([]byte, error) { return reader.AttestationListJSON(ctx, space, f) },
			func(w cubapi.Where, contains string) error {
				_, err := cubapi.ListAttestations(ctx, reader.client, w, cubapi.ListOpts{Contains: contains})
				return err
			}, "Attestation"},
	}
}

// #758, batch A: each list asks the server what cub asks. cub calls the SDK's
// list helper for the entity with the space AND-ed onto the caller's filter,
// the entity's expansions and the search, so the whole query is compared.
func TestSpaceListsSendWhatTheSDKListHelpersSend(t *testing.T) {
	hub, slug, id := hubOfTheConnectedLane(t)
	hub.otherLists = true
	reader := hub.reader(Options{})
	ctx := context.Background()
	spaceID := uuid.MustParse(id)

	for name, kind := range spaceListKinds(reader, ctx) {
		t.Run(name, func(t *testing.T) {
			for _, filter := range []Filter{{}, {Where: "Slug != 'x'"}, {Contains: "a b"}, {Where: " Slug LIKE 'p-%' ", Contains: "p"}} {
				before := len(hub.requests())
				require.NoError(t, kind.viaSDK(cubapi.NewWhere(filter.Where).SpaceID(spaceID), filter.Contains))
				out, err := kind.read(id, filter)
				require.NoError(t, err)
				require.Equal(t, "[]\n", string(out))
				seen := hub.requests()[before:]
				// The helper's request, the reader's, and then the reader's
				// check that a space named by ID exists, since the list
				// came back empty.
				require.Len(t, seen, 3)
				require.Equal(t, seen[0].Path, seen[1].Path)
				require.Equal(t, hub.queries[before], hub.queries[before+1], "%+v", filter)
				require.Contains(t, seen[1].Where, "SpaceID = '"+id+"'")
				require.Empty(t, seen[1].Limit, "no limit: the whole list in one request, as cub asks for it")
				require.Equal(t, "/api/space", seen[2].Path)
			}
			// Named by slug, the space is looked up first and not checked again.
			before := len(hub.requests())
			_, err := kind.read(slug, Filter{})
			require.NoError(t, err)
			require.Len(t, hub.requests()[before:], 2)
			require.Equal(t, "/api/space", hub.requests()[before].Path)
		})
	}
}

// The rules of a list hold for every kind: one named space, nothing from
// another space, and nothing broken read as empty.
func TestSpaceListsKeepToOneSpaceAndRefuseABrokenAnswer(t *testing.T) {
	hub, slug, id := hubOfTheConnectedLane(t)
	hub.otherLists = true
	reader := hub.reader(Options{})
	ctx := context.Background()
	const elsewhere = "11111111-2222-4333-8444-555555555555"

	for name, kind := range spaceListKinds(reader, ctx) {
		t.Run(name, func(t *testing.T) {
			for _, scope := range []string{"", " ", "*"} {
				before := len(hub.requests())
				_, err := kind.read(scope, Filter{})
				require.Equal(t, KindInvalidScope, KindOf(err), "scope %q", scope)
				require.Len(t, hub.requests(), before, "a refused scope sends nothing")
			}
			_, err := kind.read("no-such-space", Filter{})
			require.Equal(t, KindNotFound, KindOf(err))

			// Find the path this kind is listed at.
			before := len(hub.requests())
			_, err = kind.read(slug, Filter{})
			require.NoError(t, err)
			path := hub.requests()[before+1].Path
			require.NotEqual(t, "/api/space", path)
			defer delete(hub.raw, path)
			defer delete(hub.status, path)
			defer delete(hub.more, path)

			inSpace := `[{"` + kind.entity + `":{"SpaceID":"` + id + `"}}]`
			hub.raw[path] = inSpace
			out, err := kind.read(slug, Filter{})
			require.NoError(t, err)
			require.Contains(t, string(out), `"SpaceID": "`+id+`"`)

			for what, tc := range map[string]struct {
				body string
				want Kind
				says string
			}{
				"an entry from another space": {`[{"` + kind.entity + `":{"SpaceID":"` + elsewhere + `"}}]`, KindMalformed, "from another space"},
				"an entry with no entity":     {`[{"Space":{}}]`, KindMalformed, "an entry with no " + name},
				"a null entry":                {`[null]`, KindMalformed, "an entry with no " + name},
				"an object, not a list":       {`{}`, KindMalformed, ""},
				"a truncated list":            {`[{"` + kind.entity + `":`, KindMalformed, ""},
				"a page that is not the API":  {`<html>sign in</html>`, KindMalformed, "no " + name + " list"},
			} {
				hub.raw[path] = tc.body
				out, err := kind.read(slug, Filter{})
				require.Equal(t, tc.want, KindOf(err), what)
				require.ErrorContains(t, err, tc.says, what)
				require.Nil(t, out, "%s: a failed read returns no bytes to parse", what)
			}
			// With a filter, the error says the filter may be why.
			hub.raw[path] = `[{"` + kind.entity + `":{"SpaceID":"` + elsewhere + `"}}]`
			_, err = kind.read(slug, Filter{Where: "Slug LIKE '%'"})
			require.ErrorContains(t, err, "the filter may reach beyond the space")

			// Cut short by the server: the part is not the list.
			hub.raw[path], hub.more[path] = inSpace, true
			_, err = kind.read(slug, Filter{})
			require.Equal(t, KindIncomplete, KindOf(err))
			hub.more[path] = false

			// A filter the server rejects keeps the server's reason.
			delete(hub.raw, path)
			hub.status[path], hub.statusBody = http.StatusBadRequest, `{"Message":"unrecognized attribute name `+"`Nope`"+`"}`
			_, err = kind.read(slug, Filter{Where: "Nope = 1"})
			require.Equal(t, KindFailed, KindOf(err))
			require.ErrorContains(t, err, "the server rejected the request (HTTP 400): unrecognized attribute name")
			hub.status[path], hub.statusBody = http.StatusForbidden, ""
			_, err = kind.read(slug, Filter{})
			require.Equal(t, KindForbidden, KindOf(err))

			// A space ID that names no space is not an empty list.
			delete(hub.status, path)
			_, err = kind.read(elsewhere, Filter{})
			require.Equal(t, KindNotFound, KindOf(err))
			require.ErrorContains(t, err, "no space with ID")
		})
	}
}

// spaceListsRecording is one file of what cub printed for the other lists of
// a space on a real v0.8.12 server (NOTICE.md beside the files).
func spaceListsRecording(t *testing.T, name string) []byte {
	t.Helper()
	recorded, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-space-lists-v0812-recorded", name))
	require.NoError(t, err)
	return recorded
}

// #758, batch A: each list, held to the bytes cub printed for it on a real
// server. The server's JSON goes through the SDK's types and comes out as
// cub's, with the space named either way.
func TestSpaceListsAreWhatCubPrintedOnTheRealServer(t *testing.T) {
	hub := newFakeHub(t)
	require.NoError(t, json.Unmarshal(spaceListsRecording(t, "space-list.json"), &hub.spaces))
	var units []struct {
		Space struct{ Slug, SpaceID string }
	}
	require.NoError(t, json.Unmarshal(spaceListsRecording(t, "unit-list.json"), &units))
	slug, id := units[0].Space.Slug, units[0].Space.SpaceID
	reader := hub.reader(Options{})
	ctx := context.Background()

	// include is what cub asks the server to expand for each list, written
	// out from cub's source (v0.8.12: workerListInclude, targetListInclude,
	// changesetListInclude, linkListInclude; none for attestations). The
	// recordings are what the server returned for exactly these.
	for name, kind := range map[string]struct {
		path, include string
		read          func(space string) ([]byte, error)
	}{
		"worker":      {"/api/bridge_worker", "SpaceID", func(s string) ([]byte, error) { return reader.WorkerListJSON(ctx, s, Filter{}) }},
		"target":      {"/api/target", "SpaceID,TriggerFilterID,TriggerIDs", func(s string) ([]byte, error) { return reader.TargetListJSON(ctx, s, Filter{}) }},
		"changeset":   {"/api/change_set", "SpaceID,StartTagID,EndTagID", func(s string) ([]byte, error) { return reader.ChangeSetListJSON(ctx, s, Filter{}) }},
		"link":        {"/api/link", "SpaceID,FromUnitID,ToUnitID,ToSpaceID", func(s string) ([]byte, error) { return reader.LinkListJSON(ctx, s, Filter{}) }},
		"attestation": {"/api/attestation", "", func(s string) ([]byte, error) { return reader.AttestationListJSON(ctx, s, Filter{}) }},
	} {
		t.Run(name, func(t *testing.T) {
			recorded := spaceListsRecording(t, name+"-list.json")
			require.Greater(t, len(recorded), 100, "the recording has an entry in it")
			hub.raw[kind.path] = string(recorded)
			for _, space := range []string{slug, id} {
				before := len(hub.requests())
				got, err := kind.read(space)
				require.NoError(t, err)
				require.Equal(t, string(recorded), string(got))
				seen := hub.requests()[before:]
				require.Equal(t, kind.path, seen[len(seen)-1].Path, "the recording was served at the path the reader asks")
				require.Equal(t, kind.include, hub.includes[len(hub.includes)-1])
			}
		})
	}
}

// The server returns each worker's Secret, the token the worker authenticates
// with, and cub prints it. The reader does not pass it on: its worker list is
// cub's with that one field gone, and nothing else changed.
func TestWorkerListDropsTheWorkersSecret(t *testing.T) {
	hub := newFakeHub(t)
	require.NoError(t, json.Unmarshal(spaceListsRecording(t, "space-list.json"), &hub.spaces))
	var units []struct{ Space struct{ Slug string } }
	require.NoError(t, json.Unmarshal(spaceListsRecording(t, "unit-list.json"), &units))
	recorded := string(spaceListsRecording(t, "worker-list.json"))
	require.NotContains(t, recorded, "Secret", "the recording was stored without the secret")

	// Put a secret back where the server sends it, as cub printed it.
	const secret = "ch_not-a-real-worker-secret"
	const before = `      "Slug": "parity-worker",`
	require.Equal(t, 1, strings.Count(recorded, before))
	served := strings.Replace(recorded, before, `      "Secret": "`+secret+`",`+"\n"+before, 1)
	var check []struct{ BridgeWorker struct{ Secret string } }
	require.NoError(t, json.Unmarshal([]byte(served), &check))
	require.Equal(t, secret, check[0].BridgeWorker.Secret, "the test's server sends a secret")
	hub.raw["/api/bridge_worker"] = served

	got, err := hub.reader(Options{}).WorkerListJSON(context.Background(), units[0].Space.Slug, Filter{})
	require.NoError(t, err)
	require.NotContains(t, string(got), secret)
	require.NotContains(t, string(got), "Secret")
	require.Equal(t, recorded, string(got), "everything but the secret is what cub printed")
}
