// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func namespaceIdentityFixture(uid string) string {
	return fmt.Sprintf(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":%q},"futureField":{"allowed":true}}`, uid)
}
func newIdentityFixture(t *testing.T, body string, code int) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var count atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/kube-system" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server, &count
}
func identityReader(t *testing.T, cfg *rest.Config, label string) *ClusterIdentityReader {
	t.Helper()
	r, err := NewClusterIdentityReader(cfg, label)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestClusterIdentityObservedUIDAndCostArePerBoundClient(t *testing.T) {
	for _, uid := range []string{"first-cluster-instance", "second-cluster-instance"} {
		t.Run(uid, func(t *testing.T) {
			body := namespaceIdentityFixture(uid)
			server, count := newIdentityFixture(t, body, 200)
			reader := identityReader(t, &rest.Config{Host: server.URL}, "same-context-label")
			if count.Load() != 0 {
				t.Fatal("constructor made a request")
			}
			for i := 0; i < 2; i++ {
				evidence := reader.Read(context.Background())
				if evidence.ID != uid || evidence.Identity != "verified" || evidence.Context != "same-context-label" || evidence.ObservedAt == nil || evidence.Omission != "" {
					t.Fatalf("identity: %+v", evidence)
				}
				if evidence.Cost.RequestsMade != 1 || evidence.Cost.ResponseBodyBytes != int64(len(body)) || evidence.Cost.Reused || evidence.Cost.TransportErrors != 0 || evidence.Cost.BodyReadErrors != 0 {
					t.Fatalf("cost: %+v", evidence.Cost)
				}
			}
			if count.Load() != 2 {
				t.Fatalf("request count %d", count.Load())
			}
		})
	}
}

func TestClusterIdentityPinsConfigAndRedactsEndpointCredentials(t *testing.T) {
	first, c1 := newIdentityFixture(t, namespaceIdentityFixture("first"), 200)
	second, c2 := newIdentityFixture(t, namespaceIdentityFixture("second"), 200)
	cfg := &rest.Config{Host: first.URL}
	reader := identityReader(t, cfg, "Test-Exact")
	cfg.Host = second.URL
	result := reader.Read(context.Background())
	if result.ID != "first" || c1.Load() != 1 || c2.Load() != 0 || result.APIServer != first.URL {
		t.Fatalf("binding: %+v", result)
	}
	redacted := identityReader(t, &rest.Config{Host: strings.Replace(first.URL, "http://", "http://user:password@", 1) + "?secret=value#fragment"}, "Test-Exact")
	encoded, _ := json.Marshal(redacted.Read(context.Background()))
	for _, secret := range []string{"password", "secret=value", "fragment", "user:"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("endpoint credentials in %s", encoded)
		}
	}
}

func TestClusterIdentityErrorsKeepUIDUnknownAndCountConsumedBodies(t *testing.T) {
	cases := []struct {
		code         int
		body, reason string
	}{
		{403, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403}`, "forbidden"},
		{401, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Unauthorized","code":401}`, "unauthorized"},
		{404, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`, "not_found"},
		{500, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"InternalError","code":500}`, "read_failed"},
		{403, strings.Repeat("x", int(clusterIdentityBodyLimit)*3), "forbidden"},
		{500, strings.Repeat("x", int(clusterIdentityBodyLimit)*3), "read_failed"},
		{200, `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system"}}`, "invalid_namespace_identity"},
		{200, strings.Replace(namespaceIdentityFixture("x"), "kube-system", "wrong", 1), "invalid_namespace_identity"},
		{200, `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":"x","namespace":false}}`, "invalid_namespace_identity"},
		{200, `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":"x","namespace":null}}`, "invalid_namespace_identity"},
		{200, strings.Replace(namespaceIdentityFixture("x"), `"allowed":true`, `"nested":`+strings.Repeat("[", 34)+"0"+strings.Repeat("]", 34), 1), "invalid_identity_response"},
		{200, `{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":"one","uid":"two"}}`, "invalid_identity_response"},
		{200, namespaceIdentityFixture("x") + ` {}`, "invalid_identity_response"},
		{200, strings.Repeat("x", int(clusterIdentityBodyLimit)+2), "invalid_identity_response"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d-%s-%d", tc.code, tc.reason, len(tc.body)), func(t *testing.T) {
			server, count := newIdentityFixture(t, tc.body, tc.code)
			result := identityReader(t, &rest.Config{Host: server.URL}, "denied").Read(context.Background())
			if result.Identity != "unverified" || result.ID != "" || result.ObservedAt != nil || result.Omission != tc.reason {
				t.Fatalf("identity: %+v", result)
			}
			wantBytes := int64(len(tc.body))
			if wantBytes > clusterIdentityBodyLimit+1 {
				wantBytes = clusterIdentityBodyLimit + 1
			}
			if count.Load() != 1 || result.Cost.RequestsMade != 1 || result.Cost.ResponseBodyBytes != wantBytes {
				t.Fatalf("count=%d cost=%+v wantBytes=%d", count.Load(), result.Cost, wantBytes)
			}
		})
	}
}

func TestClusterIdentityTimeoutAndTransportFailureStayUnverified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	reader := identityReader(t, &rest.Config{Host: server.URL, Timeout: 20 * time.Millisecond}, "timeout")
	result := reader.Read(context.Background())
	server.Close()
	if result.Omission != "timeout" || result.ID != "" || result.ObservedAt != nil || result.Cost.RequestsMade != 1 || result.Cost.TransportErrors != 1 {
		t.Fatalf("timeout: %+v", result)
	}
	closed := identityReader(t, &rest.Config{Host: server.URL}, "closed")
	result = closed.Read(context.Background())
	if result.Omission != "read_failed" || result.Identity != "unverified" || result.Cost.RequestsMade != 1 || result.Cost.TransportErrors != 1 {
		t.Fatalf("unreachable: %+v", result)
	}
}

func TestClusterIdentityInvalidConfigAndClock(t *testing.T) {
	for _, cfg := range []*rest.Config{nil, {Host: "file:///tmp/api"}, {Host: "https://"}} {
		if _, err := NewClusterIdentityReader(cfg, "test"); err == nil {
			t.Fatalf("accepted invalid config %+v", cfg)
		}
	}
	server, _ := newIdentityFixture(t, namespaceIdentityFixture("timed"), 200)
	for _, label := range []string{strings.Repeat("x", 513), string([]byte{0xff})} {
		if _, err := NewClusterIdentityReader(&rest.Config{Host: server.URL}, label); err == nil {
			t.Fatal("accepted invalid context label")
		}
	}
	reader := identityReader(t, &rest.Config{Host: server.URL}, "clock")
	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.FixedZone("offset", 3600))
	calls := 0
	reader.now = func() time.Time { calls++; return start.Add(time.Duration(calls-1) * 125 * time.Millisecond) }
	got := reader.Read(context.Background())
	if got.Cost.DurationMillis != 125 || got.ObservedAt == nil || got.ObservedAt.Location() != time.UTC || !got.ObservedAt.Equal(start.Add(125*time.Millisecond)) {
		t.Fatalf("clock %+v", got)
	}
}

func TestClusterIdentityNoRedirectRetryOrCanceledRead(t *testing.T) {
	var calls atomic.Int64
	target, count := newIdentityFixture(t, namespaceIdentityFixture("fallback"), 200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Location", target.URL+"/api/v1/namespaces/kube-system")
		w.WriteHeader(302)
	}))
	defer server.Close()
	reader := identityReader(t, &rest.Config{Host: server.URL}, "redirect")
	got := reader.Read(context.Background())
	if got.Identity != "unverified" || calls.Load() != 1 || count.Load() != 0 || got.Cost.RequestsMade != 1 {
		t.Fatalf("redirect: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got = reader.Read(ctx)
	if got.Omission != "canceled" || got.Cost.RequestsMade != 0 || calls.Load() != 1 {
		t.Fatalf("canceled: %+v", got)
	}
	var retryCount atomic.Int64
	retry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		retryCount.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
	}))
	defer retry.Close()
	got = identityReader(t, &rest.Config{Host: retry.URL}, "429").Read(context.Background())
	if retryCount.Load() != 1 || got.Cost.RequestsMade != 1 || got.Identity != "unverified" {
		t.Fatalf("retry: %+v", got)
	}
}

func TestClusterIdentityWaitIsCancellableAndReadsRefreshIdentity(t *testing.T) {
	start := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if calls.Load() == 1 {
			close(start)
			<-release
		}
		fmt.Fprint(w, namespaceIdentityFixture(fmt.Sprint(calls.Load())))
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })
	reader := identityReader(t, &rest.Config{Host: server.URL}, "queue")
	done := make(chan ClusterIdentityEvidence, 1)
	go func() { done <- reader.Read(context.Background()) }()
	<-start
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	second := reader.Read(ctx)
	if second.Omission != "timeout" || second.Cost.RequestsMade != 0 || calls.Load() != 1 {
		t.Fatalf("wait: %+v", second)
	}
	releaseOnce.Do(func() { close(release) })
	first := <-done
	next := reader.Read(context.Background())
	if first.ID != "1" || next.ID != "2" || next.Cost.Reused || next.Cost.RequestsMade != 1 {
		t.Fatalf("refresh: %+v %+v", first, next)
	}
}

func TestClusterIdentityExactJSONFieldNamesCannotBeShadowed(t *testing.T) {
	for _, tc := range []struct{ body, uid string }{
		{`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":"exact","UID":"shadow"}}`, "exact"},
		{`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","UID":"shadow"}}`, ""},
		{`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":"\ud800"}}`, ""},
	} {
		server, _ := newIdentityFixture(t, tc.body, 200)
		result := identityReader(t, &rest.Config{Host: server.URL}, "case").Read(context.Background())
		if result.ID != tc.uid {
			t.Fatalf("identity %+v", result)
		}
		if tc.uid == "" && (result.Identity != "unverified" || result.ObservedAt != nil) {
			t.Fatalf("false identity %+v", result)
		}
	}
}

func TestClusterIdentityOpaqueTransportCostIsPartial(t *testing.T) {
	server, _ := newIdentityFixture(t, namespaceIdentityFixture("opaque"), 200)
	cfg := &rest.Config{Host: server.URL, WrapTransport: func(base http.RoundTripper) http.RoundTripper { return base }}
	result := identityReader(t, cfg, "opaque").Read(context.Background())
	if result.ID != "opaque" || result.Cost.Coverage != "partial-opaque-transport" || result.Cost.RequestsMade != 1 {
		t.Fatalf("opaque: %+v", result)
	}
}

type readErrorBody struct{}

func (readErrorBody) Read(p []byte) (int, error) { copy(p, "x"); return 1, fmt.Errorf("read failed") }
func (readErrorBody) Close() error               { return nil }
func TestKubernetesReadMeterConsumedBytesAndConcurrentReads(t *testing.T) {
	meter := &kubernetesReadMeter{}
	body := meteredBody{readErrorBody{}, meter}
	_, err := body.Read(make([]byte, 5))
	if err == nil {
		t.Fatal("expected error")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b := meteredBody{io.NopCloser(strings.NewReader("abc")), meter}
			_, _ = io.ReadAll(b)
			_ = b.Close()
		}()
	}
	wg.Wait()
	cost := meter.snapshot()
	if cost.ResponseBodyBytes != 61 || cost.BodyReadErrors != 1 {
		t.Fatalf("meter: %+v", cost)
	}
}
