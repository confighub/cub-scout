package hubread

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/confighub/sdk/core/constants"
	"github.com/stretchr/testify/require"
)

// sessionHub answers the caller's own membership, stamped with a server
// version when it has one.
type sessionHub struct {
	url     string
	version string
	status  int
	body    string
	seen    []string
}

func newSessionHub(t *testing.T) *sessionHub {
	t.Helper()
	hub := &sessionHub{body: `{"UserID":"11111111-2222-4333-8444-555555555555"}`}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.seen = append(hub.seen, r.Method+" "+r.URL.Path)
		if hub.version != "" {
			w.Header().Set(constants.ServerVersionHeader, hub.version)
		}
		w.Header().Set("Content-Type", "application/json")
		if hub.status != 0 {
			w.WriteHeader(hub.status)
			_, _ = w.Write([]byte(`{"Message":"simulated"}`))
			return
		}
		_, _ = w.Write([]byte(hub.body))
	}))
	t.Cleanup(server.Close)
	hub.url = server.URL
	return hub
}

// jwtExpiring is a token shaped like a JWT whose only claim is when it
// expires. It is signed by nothing; the reader reads the claim as cub does and
// leaves verifying to the server.
func jwtExpiring(at time.Time) string {
	claims := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, at.Unix())))
	return "e30." + claims + ".c2lnbmF0dXJl"
}

func withSDKVersion(t *testing.T, version string) {
	t.Helper()
	old := sdkCoreVersion
	t.Cleanup(func() { sdkCoreVersion = old })
	sdkCoreVersion = func() string { return version }
}

// #758, batch E: the check `cub auth status` makes, without cub: one GET of
// the caller's own membership, with the credential.
func TestCheckSessionAsksTheServerOnce(t *testing.T) {
	withSDKVersion(t, "v0.8.12")
	hub := newSessionHub(t)
	hub.version = "v0.8.12"
	reader, err := New(hub.url, testToken, Options{})
	require.NoError(t, err)

	session, err := reader.CheckSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, Session{Server: hub.url, ServerVersion: "v0.8.12", SDKVersion: "v0.8.12"}, session)
	require.Equal(t, []string{"GET /api/me"}, hub.seen)
}

func TestCheckSessionRefusalsKeepTheirReasons(t *testing.T) {
	withSDKVersion(t, "v0.8.12")
	ctx := context.Background()

	for status, want := range map[int]Kind{
		http.StatusUnauthorized: KindUnauthorized, http.StatusForbidden: KindForbidden,
		http.StatusInternalServerError: KindFailed, http.StatusNotFound: KindFailed,
	} {
		hub := newSessionHub(t)
		hub.status = status
		reader, err := New(hub.url, testToken, Options{})
		require.NoError(t, err)
		_, err = reader.CheckSession(ctx)
		require.Equal(t, want, KindOf(err), "HTTP %d", status)
		require.NotContains(t, err.Error(), testToken)
		if status == http.StatusUnauthorized {
			require.ErrorContains(t, err, "cub auth login")
		}
	}

	// A 200 that is not the API is not a session.
	for _, body := range []string{"null", "{}", `{"DisplayName":"someone"}`, "[]", "<html>sign in</html>"} {
		hub := newSessionHub(t)
		hub.body = body
		reader, err := New(hub.url, testToken, Options{})
		require.NoError(t, err)
		_, err = reader.CheckSession(ctx)
		require.Error(t, err, body)
		require.Equal(t, KindMalformed, KindOf(err), body)
	}
	// An organization alone is a membership: a key that is no user's has one.
	keyed := newSessionHub(t)
	keyed.body = `{"OrganizationID":"11111111-2222-4333-8444-555555555555"}`
	reader, err := New(keyed.url, testToken, Options{})
	require.NoError(t, err)
	_, err = reader.CheckSession(ctx)
	require.NoError(t, err)

	// A token that says it has expired is refused before anything is sent.
	expired := newSessionHub(t)
	reader, err = New(expired.url, jwtExpiring(time.Now().Add(-time.Hour)), Options{})
	require.NoError(t, err)
	_, err = reader.CheckSession(ctx)
	require.Equal(t, KindUnauthorized, KindOf(err))
	require.ErrorContains(t, err, "the access token expired at")
	require.Empty(t, expired.seen)

	// One that has not expired, and one that does not say, are both asked
	// about; the server decides.
	for _, token := range []string{jwtExpiring(time.Now().Add(time.Hour)), testToken, "a.b.c", "e30.bm90LWpzb24.c2ln"} {
		fresh := newSessionHub(t)
		reader, err = New(fresh.url, token, Options{})
		require.NoError(t, err)
		_, err = reader.CheckSession(ctx)
		require.NoError(t, err, token)
		require.Len(t, fresh.seen, 1)
	}
}

// cub refuses a client older than its server, because below 1.0 a change in
// the second version number is not backward compatible. Here the client is the
// SDK this binary was built with.
func TestCheckSessionRefusesAnSDKOlderThanTheServer(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		sdk, server string
		want        Kind
		ahead       bool
	}{
		{"v0.8.12", "v0.8.12", "", false},
		{"v0.8.12", "v0.8.3", "", false},
		{"v0.8.3", "v0.8.99", "", false},
		{"v0.8.12", "v0.9.0", KindVersionSkew, false},
		{"v0.8.12", "v1.0.0", KindVersionSkew, false},
		{"0.8.12", "0.9.1-rc1", KindVersionSkew, false},
		{"v0.9.0", "v0.8.12", "", true},
		{"v1.0.0", "v0.99.0", "", true},
		{"v0.9.0-dev", "v0.9.4", "", false},
		// What cannot be compared is not refused, as cub does not refuse it.
		{"v0.8.12", "", "", false},
		{"", "v0.9.0", "", false},
		{"(devel)", "v0.9.0", "", false},
		{"v0.8.12", "latest", "", false},
	} {
		t.Run(tc.sdk+" against "+tc.server, func(t *testing.T) {
			withSDKVersion(t, tc.sdk)
			hub := newSessionHub(t)
			hub.version = tc.server
			reader, err := New(hub.url, testToken, Options{})
			require.NoError(t, err)
			session, err := reader.CheckSession(ctx)
			require.Equal(t, tc.ahead, session.ClientAhead)
			if tc.want == "" {
				require.NoError(t, err)
				require.Len(t, hub.seen, 1)
				return
			}
			require.Equal(t, tc.want, KindOf(err), "%v", err)
			if tc.want == KindVersionSkew {
				require.ErrorContains(t, err, "too old for server "+tc.server)
				require.ErrorContains(t, err, "SDK "+tc.sdk)
			}
		})
	}
}

// The version this binary reports for the SDK is the one in go.mod.
func TestTheSDKVersionIsReadFromTheBuild(t *testing.T) {
	version := sdkCoreVersion()
	if version == "" {
		t.Skip("this test binary records no module versions")
	}
	major, minor, ok := majorMinor(version)
	require.True(t, ok, version)
	require.GreaterOrEqual(t, major*1000+minor, 8)
}
