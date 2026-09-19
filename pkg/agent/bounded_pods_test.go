// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/rest"
)

func TestBoundedPodsSelectorAndCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, body      string
		capped, wantErr bool
	}{
		{"complete", `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":"api-1","namespace":"delivery"}}]}`, false, false},
		{"continued", `{"apiVersion":"v1","kind":"PodList","metadata":{"continue":"next"},"items":[]}`, true, false},
		{"remaining", `{"apiVersion":"v1","kind":"PodList","metadata":{"remainingItemCount":2},"items":[]}`, true, false},
		{"over limit", `{"apiVersion":"v1","kind":"PodList","items":[{},{}]}`, true, true},
		{"wrong namespace", `{"apiVersion":"v1","kind":"PodList","items":[{"metadata":{"name":"api-1","namespace":"other"}}]}`, false, true},
		{"wrong kind", `{"apiVersion":"v1","kind":"PodList","items":[{"kind":"Secret","metadata":{"name":"api-1","namespace":"delivery"}}]}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "/api/v1/namespaces/delivery/pods", r.URL.Path)
				require.Equal(t, "app=api,tier in (web)", r.URL.Query().Get("labelSelector"))
				require.Equal(t, "1", r.URL.Query().Get("limit"))
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			reader, err := NewBoundedResourceReader(&rest.Config{Host: server.URL}, "fixture")
			require.NoError(t, err)
			selector, err := labels.Parse("app=api,tier in (web)")
			require.NoError(t, err)
			_, capped, e, err := reader.ListPodsMatching(context.Background(), "delivery", selector, 1)
			require.Equal(t, tc.wantErr, err != nil, "%v", err)
			require.Equal(t, tc.capped, capped)
			require.Equal(t, 1, requests)
			require.Equal(t, 1, e.Reads.Object)
			require.Zero(t, e.Reads.Discovery)
		})
	}
}

func TestBoundedPodsRejectsUnscopedReads(t *testing.T) {
	reader, err := NewBoundedResourceReader(&rest.Config{Host: "http://127.0.0.1:1"}, "fixture")
	require.NoError(t, err)
	for _, ns := range []string{"delivery", ""} {
		_, _, e, err := reader.ListPodsMatching(context.Background(), ns, labels.Everything(), 10)
		require.Error(t, err)
		require.Zero(t, e.Reads.Object)
	}
}
