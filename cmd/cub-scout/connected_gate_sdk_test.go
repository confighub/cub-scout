package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/confighub/cub-scout/v2/internal/hubread"
	"github.com/confighub/cub-scout/v2/pkg/hub"
	"github.com/confighub/sdk/core/constants"
	"github.com/stretchr/testify/require"
)

// membershipServer answers the caller's own membership, the one request the
// SDK route's session check makes.
type membershipServer struct {
	url     string
	status  int
	version string
	seen    []string
}

func newMembershipServer(t *testing.T) *membershipServer {
	t.Helper()
	member := &membershipServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		member.seen = append(member.seen, r.Method+" "+r.URL.Path)
		if member.version != "" {
			w.Header().Set(constants.ServerVersionHeader, member.version)
		}
		w.Header().Set("Content-Type", "application/json")
		if member.status != 0 {
			w.WriteHeader(member.status)
			_, _ = w.Write([]byte(`{"Message":"simulated"}`))
			return
		}
		_, _ = w.Write([]byte(`{"OrganizationID":"11111111-2222-4333-8444-555555555555","UserID":"99999999-8888-4777-8666-555555555555"}`))
	}))
	t.Cleanup(server.Close)
	member.url = server.URL
	return member
}

// sessionGateOnTheSDKRoute selects the SDK route, makes the reader talk to
// member, and leaves no cub that could run.
func sessionGateOnTheSDKRoute(t *testing.T, member *membershipServer) (cubLog string) {
	t.Helper()
	cubLog = onlyFakeCub(t, "1")
	t.Setenv(configHubReaderEnv, "sdk")
	old := sdkReader
	t.Cleanup(func() { sdkReader = old })
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return hubread.New(member.url, "test-token", hubread.Options{})
	}
	return cubLog
}

// #758, batch E: on the SDK route the gate asks the server itself. No cub
// process is started, so a machine with no cub passes it.
func TestTheGateChecksTheSessionWithoutCubOnTheSDKRoute(t *testing.T) {
	member := newMembershipServer(t)
	cubLog := sessionGateOnTheSDKRoute(t, member)

	require.NoError(t, requireConfigHubSession())
	require.Equal(t, []string{"GET /api/me"}, member.seen)
	require.Empty(t, fakeCubCalls(t, cubLog), "no cub process for the session check")

	// With nothing called cub anywhere on PATH.
	t.Setenv("PATH", t.TempDir())
	require.NoError(t, requireConfigHubSession())
	require.Len(t, member.seen, 2)
}

func TestTheGatesRefusalsOnTheSDKRouteSayWhatTheyAre(t *testing.T) {
	member := newMembershipServer(t)
	cubLog := sessionGateOnTheSDKRoute(t, member)

	// The server does not accept the credential: about the session, and not
	// worded as a cub check that never ran.
	member.status = http.StatusUnauthorized
	err := requireConfigHubSession()
	require.ErrorIs(t, err, hub.ErrCubNotAuthenticated)
	require.NotErrorIs(t, err, hub.ErrCubVersionSkew)
	require.ErrorContains(t, err, "cub auth login")
	require.NotContains(t, err.Error(), "cub auth status")
	require.NotContains(t, err.Error(), "test-token")

	// No credential at all.
	member.status = 0
	stored := sdkReader
	sdkReader = func(context.Context) (*hubread.Reader, error) {
		return nil, &hubread.Error{Kind: hubread.KindNotConfigured, Op: "resolve credentials", Message: `context "x" has no access token; run ` + "`cub auth login`"}
	}
	before := len(member.seen)
	err = requireConfigHubSession()
	require.ErrorIs(t, err, hub.ErrCubNotAuthenticated)
	require.ErrorContains(t, err, "has no access token")
	require.Len(t, member.seen, before, "nothing is sent without a credential")
	sdkReader = stored

	// A server a minor version ahead of the SDK this was built with: not
	// about the session, and the way back is named.
	member.version = "v99.0.0"
	err = requireConfigHubSession()
	require.ErrorIs(t, err, hub.ErrCubVersionSkew)
	require.NotErrorIs(t, err, hub.ErrCubNotAuthenticated)
	require.ErrorContains(t, err, "too old for server v99.0.0")
	require.ErrorContains(t, err, configHubReaderEnv+"=cub")
	require.True(t, statusSessionValid(err), "a version refusal proves the server accepted the session")

	// A server that cannot say: neither a session problem nor a version one.
	member.version, member.status = "", http.StatusInternalServerError
	err = requireConfigHubSession()
	require.Error(t, err)
	require.NotErrorIs(t, err, hub.ErrCubNotAuthenticated)
	require.NotErrorIs(t, err, hub.ErrCubVersionSkew)
	require.ErrorContains(t, err, "could not be checked")

	// The user's off switch comes first: nothing is resolved or sent.
	member.status = 0
	oldDisabled := configHubReadsDisabledFn
	t.Cleanup(func() { configHubReadsDisabledFn = oldDisabled })
	configHubReadsDisabledFn = func() error { return hub.ErrConfigHubReadsDisabled }
	before = len(member.seen)
	err = requireConfigHubSession()
	require.ErrorIs(t, err, hub.ErrConfigHubReadsDisabled)
	require.Len(t, member.seen, before)
	// With reads off, status can still say a credential is stored, and asks
	// nothing to say it.
	require.True(t, validateAuthToken())
	require.Len(t, member.seen, before)
	configHubReadsDisabledFn = oldDisabled

	require.Empty(t, fakeCubCalls(t, cubLog), "no refusal on the SDK route starts cub")
}

// By the cub route the gate is `cub auth status`, as before, and a route that
// names nothing is refused rather than read as either.
func TestTheGateByTheCubRouteAndByNoRoute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	member := newMembershipServer(t)
	sessionGateOnTheSDKRoute(t, member)
	dir := t.TempDir()
	cubLog := filepath.Join(dir, "calls.log")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cub"), []byte("#!/bin/sh\necho \"$*\" >> \""+cubLog+"\"\nexit 0\n"), 0o755))
	t.Setenv("PATH", dir)
	oldDisabled := configHubReadsDisabledFn
	t.Cleanup(func() { configHubReadsDisabledFn = oldDisabled })

	t.Setenv(configHubReaderEnv, "cub")
	t.Setenv("CUB_SCOUT_OFFLINE", "")
	if err := hub.ConfigHubReadsDisabled(); err != nil {
		t.Skipf("this machine has ConfigHub reads turned off: %v", err)
	}
	require.NoError(t, requireConfigHubSession())
	require.Equal(t, []string{"auth status"}, fakeCubCalls(t, cubLog))
	require.Empty(t, member.seen, "by the cub route the server is asked by cub, not by the reader")

	t.Setenv(configHubReaderEnv, "sdkk")
	err := requireConfigHubSession()
	require.ErrorContains(t, err, "is not a reader")
	require.False(t, errors.Is(err, hub.ErrCubNotAuthenticated))
	require.Len(t, fakeCubCalls(t, cubLog), 1)
	require.Empty(t, member.seen)
}
