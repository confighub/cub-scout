// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hub

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func prepareOCIRegistryTest(t *testing.T) {
	t.Helper()
	resetOCIRegistryForTest()
	t.Cleanup(resetOCIRegistryForTest)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "")
	t.Setenv(envCubPlugin, "1")
	t.Setenv(envCubToken, "test-token")
	previous := cubAuthStatus
	cubAuthStatus = func() (string, error) { return "", nil }
	t.Cleanup(func() { cubAuthStatus = previous })
}

func TestOCIRegistryReadsAPIInfoAndJoinsHostPort(t *testing.T) {
	prepareOCIRegistryTest(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/info" {
			t.Errorf("request = %s %s, want GET /api/info", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"OCIHost":"localhost","OCIPort":"32181"}`))
	}))
	defer server.Close()
	t.Setenv(envCubServer, server.URL)

	if got := OCIRegistry(); got != "localhost:32181" {
		t.Fatalf("OCIRegistry() = %q, want localhost:32181", got)
	}
	if got := OCIRegistry(); got != "localhost:32181" {
		t.Fatalf("cached OCIRegistry() = %q, want localhost:32181", got)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("/api/info requests = %d, want one successful lookup", got)
	}
}

func TestOCIRegistrySwitchesWithConfiguredPluginServer(t *testing.T) {
	prepareOCIRegistryTest(t)
	newServer := func(host string) (*httptest.Server, *atomic.Int32) {
		var requests atomic.Int32
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			_, _ = w.Write([]byte(`{"OCIHost":"` + host + `","OCIPort":""}`))
		})), &requests
	}
	first, firstRequests := newServer("first.example")
	defer first.Close()
	second, secondRequests := newServer("second.example")
	defer second.Close()

	t.Setenv(envCubServer, first.URL)
	if got := OCIRegistry(); got != "first.example" {
		t.Fatalf("first OCIRegistry() = %q, want first.example", got)
	}
	t.Setenv(envCubServer, second.URL)
	if got := OCIRegistry(); got != "second.example" {
		t.Fatalf("switched OCIRegistry() = %q, want second.example", got)
	}
	if firstRequests.Load() != 1 || secondRequests.Load() != 1 {
		t.Fatalf("requests = first:%d second:%d, want one each", firstRequests.Load(), secondRequests.Load())
	}
}

func TestCurrentOCIRegistryNeverUsesRecognitionCache(t *testing.T) {
	prepareOCIRegistryTest(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"OCIHost":"first.example"}`))
		} else {
			_, _ = w.Write([]byte(`{"OCIHost":"second.example"}`))
		}
	}))
	defer server.Close()
	t.Setenv(envCubServer, server.URL)
	if got := OCIRegistry(); got != "first.example" {
		t.Fatalf("initial registry %q", got)
	}
	if got := CurrentOCIRegistry(); got != "second.example" {
		t.Fatalf("exact join used stale registry %q", got)
	}
	previous := cubAuthStatus
	cubAuthStatus = func() (string, error) { return "expired", errors.New("expired") }
	t.Cleanup(func() { cubAuthStatus = previous })
	if got := CurrentOCIRegistry(); got != "" {
		t.Fatalf("exact join bypassed connected gate: %q", got)
	}
}

func TestOCIRegistryCacheExpiryAndBound(t *testing.T) {
	prepareOCIRegistryTest(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"OCIHost":"registry.example"}`))
	}))
	defer server.Close()
	t.Setenv(envCubServer, server.URL)
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		t.Setenv(envCubContext, name)
		if OCIRegistry() == "" {
			t.Fatal("missing registry")
		}
	}
	ociRegistryCache.Lock()
	count := len(ociRegistryCache.values)
	entry := ociRegistryCache.values[ociRegistryCacheKey()]
	entry.expiresAt = time.Now().Add(-time.Second)
	ociRegistryCache.values[ociRegistryCacheKey()] = entry
	ociRegistryCache.Unlock()
	if count > ociRegistryCacheMax {
		t.Fatalf("cache grew to %d", count)
	}
	if OCIRegistry() == "" || requests.Load() != 10 {
		t.Fatalf("expired entry not refreshed; requests=%d", requests.Load())
	}
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	if OCIRegistry() != "" {
		t.Fatal("offline mode reused a recognition entry")
	}
}

func TestOCIRegistryRetriesFailuresAndMalformedPayloads(t *testing.T) {
	prepareOCIRegistryTest(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1:
			w.WriteHeader(http.StatusBadGateway)
		case 2:
			_, _ = w.Write([]byte("not json"))
		default:
			_, _ = w.Write([]byte(`{"OCIHost":"retry.example","OCIPort":"443"}`))
		}
	}))
	defer server.Close()
	t.Setenv(envCubServer, server.URL)

	if got := OCIRegistry(); got != "" {
		t.Fatalf("error response OCIRegistry() = %q, want empty", got)
	}
	if got := OCIRegistry(); got != "" {
		t.Fatalf("malformed response OCIRegistry() = %q, want empty", got)
	}
	if got := OCIRegistry(); got != "retry.example:443" {
		t.Fatalf("recovered OCIRegistry() = %q, want retry.example:443", got)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("/api/info requests = %d, want retry after each failure", got)
	}
}

func TestOCIRegistryHonorsOfflineTelemetryAndConnectedGates(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*testing.T)
	}{
		{name: "offline", set: func(t *testing.T) { t.Setenv("CUB_SCOUT_OFFLINE", "true") }},
		{name: "telemetry disabled", set: func(t *testing.T) { t.Setenv("CUB_SCOUT_TELEMETRY", "false") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepareOCIRegistryTest(t)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
			}))
			defer server.Close()
			t.Setenv(envCubServer, server.URL)
			tc.set(t)

			if got := OCIRegistry(); got != "" {
				t.Fatalf("OCIRegistry() = %q, want empty", got)
			}
			if requests.Load() != 0 {
				t.Fatal("gate-disabled lookup made an HTTP request")
			}
		})
	}

	t.Run("not connected", func(t *testing.T) {
		prepareOCIRegistryTest(t)
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
		}))
		defer server.Close()
		t.Setenv(envCubServer, server.URL)
		previous := cubAuthStatus
		cubAuthStatus = func() (string, error) { return "expired", errors.New("expired") }
		defer func() { cubAuthStatus = previous }()

		if got := OCIRegistry(); got != "" {
			t.Fatalf("OCIRegistry() = %q, want empty", got)
		}
		if requests.Load() != 0 {
			t.Fatal("disconnected lookup made an HTTP request")
		}
	})
}

func TestOCIRegistryOfflineRepeatedCallsDoNotRunAuthOrNetwork(t *testing.T) {
	resetOCIRegistryForTest()
	t.Cleanup(resetOCIRegistryForTest)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	t.Setenv("CUB_SCOUT_TELEMETRY", "")
	t.Setenv(envCubPlugin, "1")
	t.Setenv(envCubToken, "test-token")
	var authCalls, requests atomic.Int32
	previous := cubAuthStatus
	cubAuthStatus = func() (string, error) {
		authCalls.Add(1)
		return "", nil
	}
	t.Cleanup(func() { cubAuthStatus = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	t.Setenv(envCubServer, server.URL)

	for i := 0; i < 32; i++ {
		if got := OCIRegistry(); got != "" {
			t.Fatalf("offline OCIRegistry() call %d = %q, want empty", i, got)
		}
	}
	if authCalls.Load() != 0 {
		t.Fatalf("offline auth calls = %d, want 0", authCalls.Load())
	}
	if requests.Load() != 0 {
		t.Fatalf("offline HTTP requests = %d, want 0", requests.Load())
	}
}

func TestLookupOCIRegistryHonorsContextTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(100 * time.Millisecond):
			_, _ = w.Write([]byte(`{"OCIHost":"late.example"}`))
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if got := lookupOCIRegistry(ctx, server.URL); got != "" {
		t.Fatalf("lookupOCIRegistry() = %q after timeout, want empty", got)
	}
}

func TestLookupOCIRegistryRejectsUnsafeServerURLs(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	unsafe := []string{
		"ftp://example.com",
		strings.Replace(server.URL, "http://", "http://user:pass@", 1),
		server.URL + "#fragment",
		server.URL + "/%2Fencoded",
	}
	for _, raw := range unsafe {
		t.Run(raw, func(t *testing.T) {
			if got := lookupOCIRegistry(context.Background(), raw); got != "" {
				t.Fatalf("lookupOCIRegistry(%q) = %q, want empty", raw, got)
			}
		})
	}
}

func TestLookupOCIRegistryDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		_, _ = w.Write([]byte(`{"OCIHost":"redirected.example"}`))
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/info", http.StatusFound)
	}))
	defer server.Close()

	if got := lookupOCIRegistry(context.Background(), server.URL); got != "" {
		t.Fatalf("redirecting lookupOCIRegistry() = %q, want empty", got)
	}
	if redirected.Load() != 0 {
		t.Fatal("registry lookup followed an HTTP redirect")
	}
}

func TestLookupOCIRegistryRejectsOversizedTrailingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"OCIHost":"too-large.example"}` + strings.Repeat("x", ociRegistryBodyMax)))
	}))
	defer server.Close()

	if got := lookupOCIRegistry(context.Background(), server.URL); got != "" {
		t.Fatalf("oversized lookupOCIRegistry() = %q, want empty", got)
	}
}
