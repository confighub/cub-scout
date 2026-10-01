// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

type cacheReplayResponse struct {
	Status int    `json:"status"`
	Path   string `json:"path"`
	Body   string `json:"body"`
	SHA256 string `json:"sha256"`
}

type cacheReplayStep struct {
	Name            string `json:"name"`
	At              string `json:"at"`
	Refresh         bool   `json:"refresh"`
	UID             string `json:"uid"`
	ResourceVersion string `json:"resourceVersion"`
	Image           string `json:"image"`
	Cache           string `json:"cache"`
	Available       bool   `json:"available"`
	ObservedAt      string `json:"observedAt"`
	ExpiresAt       string `json:"expiresAt"`
	DiscoveryReads  int    `json:"discoveryReads"`
	ObjectReads     int    `json:"objectReads"`
	RequestCount    int    `json:"requestCount"`
	Error           string `json:"error"`
}

type cacheReplayHTTPRecord struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
	Body   string `json:"body"`
	SHA256 string `json:"sha256"`
}

type cacheReplayInputRecord struct {
	StepID                   string              `json:"stepId"`
	Clock                    string              `json:"clock"`
	Refresh                  bool                `json:"refresh"`
	ConfiguredObjectResponse cacheReplayResponse `json:"configuredObjectResponse"`
}

type cacheReplayActualStep struct {
	Input                  cacheReplayInputRecord  `json:"input"`
	ReturnedObject         interface{}             `json:"returnedObject"`
	Evidence               BoundedReadEvidence     `json:"evidence"`
	ErrorClassification    string                  `json:"errorClassification"`
	Requests               []cacheReplayHTTPRecord `json:"requests"`
	CumulativeRequestCount int                     `json:"cumulativeRequestCount"`
}

type cacheReplayActualRecord struct {
	Schema    string                  `json:"schema"`
	InputKind string                  `json:"inputKind"`
	Resource  BoundedResourceRef      `json:"resource"`
	Steps     []cacheReplayActualStep `json:"steps"`
}

type cacheReplayAuthoredInput struct {
	StepID      string `json:"stepId"`
	Clock       string `json:"clock"`
	Refresh     bool   `json:"refresh"`
	ResponseKey string `json:"responseKey"`
}

type cacheReplayDocument struct {
	Inputs []cacheReplayAuthoredInput `json:"inputs"`
	Schema string                     `json:"schema"`
	Scope  struct {
		Context    string             `json:"context"`
		Resource   BoundedResourceRef `json:"resource"`
		ClockStart string             `json:"clockStart"`
		TTLSeconds int                `json:"ttlSeconds"`
	} `json:"scope"`
	Responses              map[string]cacheReplayResponse `json:"responses"`
	ExpectedSteps          []cacheReplayStep              `json:"expectedSteps"`
	ServedResponseSequence []string                       `json:"servedResponseSequence"`
}

// TestBoundedReadRUL02IdentityCacheReplay exercises the real bounded reader
// against fixed local API responses. It records the exact served bytes and the
// cache/evidence result at each fixed-clock step; it makes no live-cluster claim.
func TestBoundedReadRUL02IdentityCacheReplay(t *testing.T) {
	recordPath := filepath.Join("testdata", "rul02-cache-replay.json")
	recordBytes, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	var replay cacheReplayDocument
	require.NoError(t, json.Unmarshal(recordBytes, &replay))
	require.Equal(t, "bounded-resource-cache-replay.v1", replay.Schema)
	require.Equal(t, "recorded-test-cluster", replay.Scope.Context)
	require.Equal(t, BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}, replay.Scope.Resource)
	require.Equal(t, 15, replay.Scope.TTLSeconds)
	require.Len(t, replay.ExpectedSteps, 9)
	require.Len(t, replay.Inputs, 9)
	expectedByID := make(map[string]cacheReplayStep, 9)
	for _, expected := range replay.ExpectedSteps {
		_, duplicate := expectedByID[expected.Name]
		require.False(t, duplicate)
		expectedByID[expected.Name] = expected
	}

	for name, response := range replay.Responses {
		digest := sha256.Sum256([]byte(response.Body))
		require.Equal(t, hex.EncodeToString(digest[:]), response.SHA256, "response %s hash", name)
	}

	var mu sync.Mutex
	currentResponse := "objectA"
	var served []string
	var httpRecords []cacheReplayHTTPRecord
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requestCount++
		status, body := http.StatusNotFound, "404 page not found\n"
		responseName := ""
		if r.Method != http.MethodGet {
			status, body = http.StatusMethodNotAllowed, "method not allowed\n"
		} else if r.URL.Path == replay.Responses["discovery"].Path {
			responseName = "discovery"
		} else if r.URL.Path == replay.Responses["objectA"].Path {
			responseName = currentResponse
		}
		if responseName != "" {
			response := replay.Responses[responseName]
			status, body = response.Status, response.Body
			served = append(served, responseName)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
		digest := sha256.Sum256([]byte(body))
		httpRecords = append(httpRecords, cacheReplayHTTPRecord{Method: r.Method, Path: r.URL.Path,
			Status: status, Body: body, SHA256: hex.EncodeToString(digest[:])})
	}))
	defer server.Close()

	reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, replay.Scope.Context)
	require.NoError(t, err)
	clock, err := time.Parse(time.RFC3339, replay.Scope.ClockStart)
	require.NoError(t, err)
	reader.now = func() time.Time { return clock }
	ref := replay.Scope.Resource
	actual := make([]cacheReplayStep, 0, len(replay.ExpectedSteps))
	actualRecord := cacheReplayActualRecord{Schema: "bounded-resource-cache-replay-result.v1", InputKind: "authored_httptest_responses_and_clock", Resource: ref}

	for i, input := range replay.Inputs {
		require.Equal(t, fmt.Sprintf("step-%02d", i+1), input.StepID)
		want, exists := expectedByID[input.StepID]
		require.True(t, exists)
		clock, err = time.Parse(time.RFC3339, input.Clock)
		require.NoError(t, err, "step %s clock", input.StepID)
		_, responseExists := replay.Responses[input.ResponseKey]
		require.True(t, responseExists)
		mu.Lock()
		currentResponse = input.ResponseKey
		requestStart := len(httpRecords)
		configuredResponse := replay.Responses[currentResponse]
		mu.Unlock()

		obj, evidence, readErr := reader.Read(context.Background(), ref, input.Refresh)
		step := cacheReplayStep{Name: input.StepID, At: input.Clock, Refresh: input.Refresh,
			Cache: evidence.Cache, Available: evidence.Available,
			DiscoveryReads: evidence.Reads.Discovery, ObjectReads: evidence.Reads.Object}
		if !evidence.ObservedAt.IsZero() {
			step.ObservedAt = evidence.ObservedAt.UTC().Format(time.RFC3339)
		}
		if !evidence.ExpiresAt.IsZero() {
			step.ExpiresAt = evidence.ExpiresAt.UTC().Format(time.RFC3339)
		}
		if obj != nil {
			step.UID = string(obj.GetUID())
			step.ResourceVersion = obj.GetResourceVersion()
			if containers, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers"); ok && len(containers) > 0 {
				if container, ok := containers[0].(map[string]interface{}); ok {
					step.Image, _, _ = unstructured.NestedString(container, "image")
				}
			}
		}
		if readErr != nil {
			if strings.Contains(readErr.Error(), "bounded object unavailable") {
				step.Error = "object unavailable"
			} else {
				step.Error = fmt.Sprintf("unexpected: %T", readErr)
			}
		}
		mu.Lock()
		step.RequestCount = requestCount
		stepRequests := append([]cacheReplayHTTPRecord(nil), httpRecords[requestStart:]...)
		cumulativeRequestCount := requestCount
		mu.Unlock()

		require.Equal(t, want, step, "step %d (%s)", i, want.Name)
		if input.StepID == "step-07" {
			require.Empty(t, step.UID, "UID must not be inferred from the stable name")
			require.NotContains(t, step.Image, "@sha256:", "digest must not be inferred from a mutable tag")
		}
		actual = append(actual, step)
		stepID := input.StepID
		classification := "none"
		if readErr != nil {
			if strings.Contains(readErr.Error(), "bounded object unavailable") {
				classification = "bounded_object_unavailable"
			} else {
				classification = fmt.Sprintf("unexpected_%T", readErr)
			}
		}
		var returned interface{}
		if obj != nil {
			returned = obj.Object
		}
		actualRecord.Steps = append(actualRecord.Steps, cacheReplayActualStep{
			Input:          cacheReplayInputRecord{StepID: stepID, Clock: input.Clock, Refresh: input.Refresh, ConfiguredObjectResponse: configuredResponse},
			ReturnedObject: returned, Evidence: evidence, ErrorClassification: classification,
			Requests: stepRequests, CumulativeRequestCount: cumulativeRequestCount,
		})
	}

	mu.Lock()
	actualServed := append([]string(nil), served...)
	mu.Unlock()
	require.Equal(t, replay.ServedResponseSequence, actualServed)
	require.True(t, reflect.DeepEqual(replay.ExpectedSteps, actual))
	recorded, err := json.Marshal(actualRecord)
	require.NoError(t, err)
	t.Logf("RUL02_REPLAY_JSON=%s", recorded)
}
