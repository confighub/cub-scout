// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestReverseApplicationResolvesUniqueNamespace(t *testing.T) {
	for _, tc := range []struct {
		name      string
		count     int
		denied    bool
		wantError string
	}{
		{"unique", 1, false, ""}, {"ambiguous", 2, false, "ambiguous"}, {"missing", 0, false, "not found"}, {"denied", 0, true, "forbidden"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/apis/argoproj.io/v1alpha1/applications" {
					require.Equal(t, "metadata.name=api", r.URL.Query().Get("fieldSelector"))
					if tc.denied {
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","message":"applications forbidden","code":403}`)
						return
					}
					var items []string
					for i := 0; i < tc.count; i++ {
						items = append(items, fmt.Sprintf(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Application","metadata":{"name":"api","namespace":"gitops-%d"},"spec":{"source":{"repoURL":"https://example.invalid/repo","targetRevision":"main"}}}`, i))
					}
					fmt.Fprintf(w, `{"apiVersion":"argoproj.io/v1alpha1","kind":"ApplicationList","items":[%s]}`, strings.Join(items, ","))
					return
				}
				if r.URL.Path == "/apis/argoproj.io/v1alpha1/namespaces/gitops-0/applications/api" {
					fmt.Fprint(w, `{"apiVersion":"argoproj.io/v1alpha1","kind":"Application","metadata":{"name":"api","namespace":"gitops-0","uid":"observed-application"}}`)
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()
			session, err := newTraceSession(&rest.Config{Host: server.URL}, "selected")
			require.NoError(t, err)
			oldFormat, oldJSON := traceFormat, traceJSON
			traceFormat, traceJSON = "json", false
			defer func() { traceFormat, traceJSON = oldFormat, oldJSON }()
			var runErr error
			output := captureStdout(t, func() { runErr = runReverseTraceWithSession(context.Background(), session, "Application", "api", "") })
			if tc.wantError != "" {
				require.ErrorContains(t, runErr, tc.wantError)
				require.Len(t, paths, 1)
				return
			}
			require.NoError(t, runErr)
			require.Contains(t, output, `"namespace": "gitops-0"`)
			require.Contains(t, output, `"context": "selected"`)
			require.Equal(t, []string{"/apis/argoproj.io/v1alpha1/applications", "/apis/argoproj.io/v1alpha1/namespaces/gitops-0/applications/api"}, paths)
		})
	}
}
