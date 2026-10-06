// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const clusterIdentityBodyLimit int64 = 64 << 10

// KubernetesReadCost counts consumed response-body bytes and transport attempts,
// not wire/header bytes, credential traffic or unrelated clients. Duration is
// the identity operation's wall duration, including cancellable queue waiting.
type KubernetesReadCost struct {
	RequestsMade      int64  `json:"requestsMade"`
	ResponseBodyBytes int64  `json:"responseBodyBytes"`
	TransportErrors   int64  `json:"transportErrors"`
	BodyReadErrors    int64  `json:"bodyReadErrors"`
	DurationMillis    int64  `json:"durationMillis"`
	Reused            bool   `json:"reused"`
	Coverage          string `json:"coverage"`
}

type ClusterIdentityEvidence struct {
	Context    string             `json:"context"`
	APIServer  string             `json:"apiServer"`
	ID         string             `json:"id,omitempty"`
	Identity   string             `json:"identity"`
	IDSource   string             `json:"idSource"`
	ObservedAt *time.Time         `json:"observedAt,omitempty"`
	Omission   string             `json:"omission,omitempty"`
	Cost       KubernetesReadCost `json:"cost"`
}

type kubernetesReadMeter struct {
	mu     sync.Mutex
	totals KubernetesReadCost
}
type meteredTransport struct {
	base  http.RoundTripper
	meter *kubernetesReadMeter
}
type meteredBody struct {
	io.ReadCloser
	meter *kubernetesReadMeter
}

// Stream consumes error responses inside client-go. Apply this identity-specific
// cap before status handling, preserving the underlying body's Close method.
type clusterIdentityResponseTransport struct{ base http.RoundTripper }

func (t clusterIdentityResponseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(req)
	if err == nil && response != nil && response.Body != nil {
		response.Body = struct {
			io.Reader
			io.Closer
		}{io.LimitReader(response.Body, clusterIdentityBodyLimit+1), response.Body}
	}
	return response, err
}

func (m *kubernetesReadMeter) snapshot() KubernetesReadCost {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.totals
}
func (t meteredTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.meter.mu.Lock()
	t.meter.totals.RequestsMade++
	t.meter.mu.Unlock()
	response, err := t.base.RoundTrip(req)
	if err != nil {
		t.meter.mu.Lock()
		t.meter.totals.TransportErrors++
		t.meter.mu.Unlock()
	}
	if response != nil && response.Body != nil {
		response.Body = meteredBody{response.Body, t.meter}
	}
	return response, err
}
func (b meteredBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.meter.mu.Lock()
	defer b.meter.mu.Unlock()
	b.meter.totals.ResponseBodyBytes += int64(n)
	if err != nil && err != io.EOF {
		b.meter.totals.BodyReadErrors++
	}
	return n, err
}

// ClusterIdentityReader is a shared P4 foundation. Existing command output and
// clients do not use it yet. It binds one copied client and never reads a second
// context, discovery, workloads or connected Targets. There is no identity cache.
type ClusterIdentityReader struct {
	client      rest.Interface
	contextName string
	server      string
	meter       *kubernetesReadMeter
	coverage    string
	gate        chan struct{}
	now         func() time.Time
}

func NewClusterIdentityReader(config *rest.Config, contextName string) (*ClusterIdentityReader, error) {
	if config == nil || !utf8.ValidString(contextName) || len(contextName) > 512 {
		return nil, fmt.Errorf("cluster identity requires a configured client and bounded context label")
	}
	endpoint, err := url.Parse(config.Host)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, fmt.Errorf("cluster identity requires a valid API endpoint")
	}
	endpoint.User = nil
	endpoint.RawQuery = ""
	endpoint.ForceQuery = false
	endpoint.Fragment = ""
	endpoint.RawFragment = ""
	cfg := rest.CopyConfig(config)
	if cfg.Timeout <= 0 || cfg.Timeout > 5*time.Second {
		cfg.Timeout = 5 * time.Second
	}
	meter := &kubernetesReadMeter{}
	prior := cfg.WrapTransport
	coverage := "selected-kubernetes-transport"
	if prior != nil {
		coverage = "partial-opaque-transport"
	}
	cfg.WrapTransport = func(base http.RoundTripper) http.RoundTripper {
		measured := http.RoundTripper(meteredTransport{base, meter})
		if prior != nil {
			return prior(measured)
		}
		return measured
	}
	httpClient, err := newBoundedHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	bounded := *httpClient
	base := bounded.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	bounded.Transport = clusterIdentityResponseTransport{base: base}
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client, err := rest.UnversionedRESTClientForConfigAndClient(dynamic.ConfigFor(cfg), &bounded)
	if err != nil {
		return nil, err
	}
	return &ClusterIdentityReader{client: client, contextName: contextName, server: endpoint.String(), meter: meter, coverage: coverage, gate: make(chan struct{}, 1), now: time.Now}, nil
}

// Read performs one fixed Namespace GET with REST retries and redirects disabled.
// An unavailable UID is unverified identity, never an ownership/health verdict.
func (r *ClusterIdentityReader) Read(ctx context.Context) ClusterIdentityEvidence {
	start := r.now()
	evidence := ClusterIdentityEvidence{Context: r.contextName, APIServer: r.server, Identity: "unverified", IDSource: "v1/Namespace/kube-system", Cost: KubernetesReadCost{Coverage: r.coverage}}
	finish := func() { evidence.Cost.DurationMillis = max(0, r.now().Sub(start).Milliseconds()) }
	if err := ctx.Err(); err != nil {
		evidence.Omission = clusterIdentityErrorReason(err)
		finish()
		return evidence
	}
	select {
	case r.gate <- struct{}{}:
	case <-ctx.Done():
		evidence.Omission = clusterIdentityErrorReason(ctx.Err())
		finish()
		return evidence
	}
	defer func() { <-r.gate }()
	before := r.meter.snapshot()
	stream, err := r.client.Get().AbsPath("/api/v1/namespaces/kube-system").MaxRetries(0).Stream(ctx)
	var raw []byte
	if err == nil {
		raw, err = io.ReadAll(io.LimitReader(stream, clusterIdentityBodyLimit+1))
		closeErr := stream.Close()
		if err == nil {
			err = closeErr
		}
	}
	after := r.meter.snapshot()
	evidence.Cost.RequestsMade = after.RequestsMade - before.RequestsMade
	evidence.Cost.ResponseBodyBytes = after.ResponseBodyBytes - before.ResponseBodyBytes
	evidence.Cost.TransportErrors = after.TransportErrors - before.TransportErrors
	evidence.Cost.BodyReadErrors = after.BodyReadErrors - before.BodyReadErrors
	if err != nil {
		evidence.Omission = clusterIdentityErrorReason(err)
		finish()
		return evidence
	}
	if int64(len(raw)) > clusterIdentityBodyLimit || !utf8.Valid(raw) || uniqueIdentityJSON(raw) != nil {
		evidence.Omission = "invalid_identity_response"
		finish()
		return evidence
	}
	var obj map[string]json.RawMessage
	var metadata map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || json.Unmarshal(obj["metadata"], &metadata) != nil {
		evidence.Omission = "invalid_identity_response"
		finish()
		return evidence
	}
	// Match exact Kubernetes JSON field names. Struct decoding would accept
	// uppercase aliases (such as UID) and could overwrite the actual uid.
	field := func(values map[string]json.RawMessage, key string) string {
		var value string
		if json.Unmarshal(values[key], &value) != nil {
			return ""
		}
		return value
	}
	uid := field(metadata, "uid")
	namespaceValid := true
	if rawNamespace, present := metadata["namespace"]; present {
		var namespace *string
		namespaceValid = json.Unmarshal(rawNamespace, &namespace) == nil && namespace != nil && *namespace == ""
	}
	if field(obj, "apiVersion") != "v1" || field(obj, "kind") != "Namespace" || field(metadata, "name") != "kube-system" || !namespaceValid || strings.TrimSpace(uid) == "" || len(uid) > 512 || !utf8.ValidString(uid) || strings.ContainsRune(uid, utf8.RuneError) {
		evidence.Omission = "invalid_namespace_identity"
		finish()
		return evidence
	}
	observed := r.now().UTC()
	evidence.Identity = "verified"
	evidence.ID = uid
	evidence.ObservedAt = &observed
	evidence.Cost.DurationMillis = max(0, observed.Sub(start).Milliseconds())
	return evidence
}

func clusterIdentityErrorReason(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded), apierrors.IsTimeout(err):
		return "timeout"
	case apierrors.IsForbidden(err):
		return "forbidden"
	case apierrors.IsUnauthorized(err):
		return "unauthorized"
	case apierrors.IsNotFound(err):
		return "not_found"
	default:
		return "read_failed"
	}
}

// Unknown fields are permitted across Kubernetes versions, but ambiguous
// duplicate fields, trailing documents and excessive nesting are refused.
func uniqueIdentityJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var read func(int) error
	read = func(depth int) error {
		if depth > 32 {
			return fmt.Errorf("identity nesting exceeds bound")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				token, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := token.(string)
				if !ok || seen[key] {
					return fmt.Errorf("ambiguous identity fields")
				}
				seen[key] = true
				if err := read(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := read(depth + 1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("invalid identity JSON")
		}
		_, err = decoder.Token()
		return err
	}
	if err := read(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing identity JSON")
	}
	return nil
}
