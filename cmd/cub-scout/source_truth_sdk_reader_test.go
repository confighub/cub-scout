// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/confighub/cub-scout/v2/internal/hubread"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// recordedUnitGet is what a real ConfigHub v0.8.3 server returned for one Unit.
func recordedUnitGet(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-governance-v083-recorded", "unit-get.json"))
	require.NoError(t, err)
	return data
}

// sdkRouteAgainstRecordedUnit points the SDK route at a server that answers
// the two list calls with the recorded object, and counts the requests.
func sdkRouteAgainstRecordedUnit(t *testing.T, status int) *[]string {
	t.Helper()
	var element map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(recordedUnitGet(t), &element))
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"Message":"simulated"}`))
			return
		}
		switch r.URL.Path {
		case "/api/space":
			_ = json.NewEncoder(w).Encode([]map[string]json.RawMessage{{"Space": element["Space"]}})
		case "/api/unit":
			_ = json.NewEncoder(w).Encode([]map[string]json.RawMessage{element})
		}
	}))
	t.Cleanup(server.Close)
	old := sourceTruthSDKUnitHead
	t.Cleanup(func() { sourceTruthSDKUnitHead = old })
	sourceTruthSDKUnitHead = func(ctx context.Context, unit, space string) (hubread.UnitHead, error) {
		reader, err := hubread.New(server.URL, "test-token", hubread.Options{})
		if err != nil {
			return hubread.UnitHead{}, err
		}
		return reader.UnitHead(ctx, space, unit)
	}
	return &seen
}

func recordedWorkload() *runtimeWorkload {
	return &runtimeWorkload{
		Labels:      map[string]string{"confighub.com/UnitSlug": "contract-config"},
		Annotations: map[string]string{"confighub.com/SpaceName": "scout-v213-contract"},
	}
}

// #758: the SDK route must report exactly what the cub route reports for the
// same Unit. Both are given the same recorded server object: the cub route as
// `cub unit get -o json` printed it, the SDK route over HTTP.
func TestSourceTruthSDKRouteReportsWhatTheCubRouteReports(t *testing.T) {
	oldGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldGet })
	cubCalls := 0
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		cubCalls++
		return recordedUnitGet(t), nil
	}
	seen := sdkRouteAgainstRecordedUnit(t, 0)

	t.Setenv(configHubReaderEnv, "")
	viaCub, err := collectConfigHubSurface(context.Background(), recordedWorkload())
	require.NoError(t, err)
	require.Equal(t, 1, cubCalls)
	require.Empty(t, *seen, "without the setting the SDK route is not used")

	t.Setenv(configHubReaderEnv, "sdk")
	viaSDK, err := collectConfigHubSurface(context.Background(), recordedWorkload())
	require.NoError(t, err)
	require.Equal(t, 1, cubCalls, "with the setting no cub process is started")
	require.Equal(t, []string{"GET /api/space", "GET /api/unit"}, *seen)

	require.Equal(t, viaCub, viaSDK)
	require.Equal(t, &agent.ConfigHubSurface{
		Space: "scout-v213-contract", Unit: "contract-config", Revision: "2",
		URL: configHubUnitDetailURL("68843338-f9bd-485c-9a66-5d5aee820247", "6221926e-8cc4-4847-a25c-a4104042f640"),
	}, viaSDK)

	t.Setenv(configHubReaderEnv, "CUB")
	_, err = collectConfigHubSurface(context.Background(), recordedWorkload())
	require.NoError(t, err)
	require.Equal(t, 2, cubCalls, "the route name is case-insensitive")
}

// A failed SDK read is an omission with its own reason. It never falls back to
// the cub route, which would hide that the route asked for did not work.
func TestSourceTruthSDKRouteFailureIsReportedAndDoesNotFallBack(t *testing.T) {
	oldGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldGet })
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		t.Fatal("the cub route was used after the SDK route failed")
		return nil, nil
	}
	sdkRouteAgainstRecordedUnit(t, http.StatusForbidden)
	t.Setenv(configHubReaderEnv, "sdk")

	surface, err := collectConfigHubSurface(context.Background(), recordedWorkload())
	require.Nil(t, surface)
	var collection *agent.CollectionError
	require.ErrorAs(t, err, &collection)
	require.Equal(t, "confighub", collection.Surface)
	require.Contains(t, collection.Reason, "ConfigHub unit read failed")
	require.Contains(t, collection.Reason, "forbidden")
	require.NotContains(t, collection.Reason, "test-token")
}

// A misspelt route is an error, not the default route under another name.
func TestSourceTruthRejectsAnUnknownConfigHubReader(t *testing.T) {
	oldGet := sourceTruthUnitGet
	t.Cleanup(func() { sourceTruthUnitGet = oldGet })
	sourceTruthUnitGet = func(context.Context, string, string) ([]byte, error) {
		t.Fatal("an unknown reader fell back to cub")
		return nil, nil
	}
	seen := sdkRouteAgainstRecordedUnit(t, 0)
	t.Setenv(configHubReaderEnv, "sdkk")

	_, err := collectConfigHubSurface(context.Background(), recordedWorkload())
	var collection *agent.CollectionError
	require.ErrorAs(t, err, &collection)
	require.Contains(t, collection.Reason, `CUB_SCOUT_CONFIGHUB_READER="sdkk" is not a reader (valid: cub, sdk)`)
	require.Empty(t, *seen)

	route, err := configHubReaderRoute()
	require.Error(t, err)
	require.Empty(t, route)
}
