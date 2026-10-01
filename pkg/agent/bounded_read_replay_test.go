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

type cacheReplayDocument struct {
	Schema string `json:"schema"`
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

	for name, response := range replay.Responses {
		digest := sha256.Sum256([]byte(response.Body))
		require.Equal(t, hex.EncodeToString(digest[:]), response.SHA256, "response %s hash", name)
	}

	var mu sync.Mutex
	currentResponse := "objectA"
	var served []string
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requestCount++
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		responseName := ""
		if r.URL.Path == replay.Responses["discovery"].Path {
			responseName = "discovery"
		} else if r.URL.Path == replay.Responses["objectA"].Path {
			responseName = currentResponse
		} else {
			http.NotFound(w, r)
			return
		}
		response := replay.Responses[responseName]
		served = append(served, responseName)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.Status)
		_, _ = w.Write([]byte(response.Body))
	}))
	defer server.Close()

	reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, replay.Scope.Context)
	require.NoError(t, err)
	clock, err := time.Parse(time.RFC3339, replay.Scope.ClockStart)
	require.NoError(t, err)
	reader.now = func() time.Time { return clock }
	ref := replay.Scope.Resource
	actual := make([]cacheReplayStep, 0, len(replay.ExpectedSteps))

	for i, want := range replay.ExpectedSteps {
		clock, err = time.Parse(time.RFC3339, want.At)
		require.NoError(t, err, "step %s clock", want.Name)
		switch want.Name {
		case "initial-object-a", "unexpired-cache-hit":
			currentResponse = "objectA"
		case "changed-server-hidden-by-cache", "explicit-refresh-replacement-uid":
			currentResponse = "objectB"
		case "explicit-refresh-same-uid-new-digest":
			currentResponse = "objectC"
		case "expired-cache-object-d":
			currentResponse = "objectD"
		case "missing-uid-and-digest":
			currentResponse = "objectMissingIdentity"
		case "failed-refresh", "ordinary-read-after-failed-refresh":
			currentResponse = "objectUnavailable"
		default:
			t.Fatalf("unexpected replay step %q", want.Name)
		}

		obj, evidence, readErr := reader.Read(context.Background(), ref, want.Refresh)
		step := cacheReplayStep{Name: want.Name, At: want.At, Refresh: want.Refresh,
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
		mu.Unlock()

		require.Equal(t, want, step, "step %d (%s)", i, want.Name)
		if want.Name == "missing-uid-and-digest" {
			require.Empty(t, step.UID, "UID must not be inferred from the stable name")
			require.NotContains(t, step.Image, "@sha256:", "digest must not be inferred from a mutable tag")
		}
		actual = append(actual, step)
	}

	mu.Lock()
	actualServed := append([]string(nil), served...)
	mu.Unlock()
	require.Equal(t, replay.ServedResponseSequence, actualServed)
	require.True(t, reflect.DeepEqual(replay.ExpectedSteps, actual))
}
