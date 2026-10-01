// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestMapListKindFilterLimitsLiveRequestsAndRetainsUnknownGVR(t *testing.T) {
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		counts[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		items := []map[string]interface{}{}
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/team-a/deployments":
			items = append(items, map[string]interface{}{
				"apiVersion": "apps/v1", "kind": "Deployment",
				"metadata": map[string]interface{}{"name": "owned", "namespace": "team-a", "labels": map[string]interface{}{"kustomize.toolkit.fluxcd.io/name": "checkout"}},
			})
		case "/apis/custom.example/v1/namespaces/team-a/widgets":
			// Config has GVR but no canonical Kind. Retaining it prevents a
			// same-Kind CRD in another API group from disappearing silently.
			items = append(items, map[string]interface{}{
				"apiVersion": "custom.example/v1", "kind": "Deployment",
				"metadata": map[string]interface{}{"name": "duplicate-kind", "namespace": "team-a"},
			})
		}
		apiVersion, kind := "v1", "List"
		if r.URL.Path == "/apis/argoproj.io/v1alpha1/applicationsets" {
			apiVersion = "argoproj.io/v1alpha1"
			kind = "ApplicationSetList"
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": apiVersion, "kind": kind, "metadata": map[string]interface{}{}, "items": items})
	}))
	defer server.Close()

	oldKind, oldNamespace := mapKind, mapNamespace
	oldFormat, oldJSON := mapListFormat, mapJSON
	oldLoader := mapWatchLoadCustomResourceConfigs
	t.Cleanup(func() {
		mapKind, mapNamespace = oldKind, oldNamespace
		mapListFormat, mapJSON = oldFormat, oldJSON
		mapWatchLoadCustomResourceConfigs = oldLoader
	})
	mapKind, mapNamespace = "Deployment", "team-a"
	mapListFormat, mapJSON = "json", false
	mapWatchLoadCustomResourceConfigs = func() []customResourceConfig {
		return []customResourceConfig{{GVR: schema.GroupVersionResource{Group: "custom.example", Version: "v1", Resource: "widgets"}}}
	}

	config := clientcmdapi.NewConfig()
	config.CurrentContext = "offline"
	config.Clusters["offline"] = &clientcmdapi.Cluster{Server: server.URL}
	config.Contexts["offline"] = &clientcmdapi.Context{Cluster: "offline"}
	data, err := clientcmd.Write(*config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.QPS, cfg.Burst = 1000, 1000 // fixture measures request selection, not client throttling

	output := captureStdout(t, func() {
		if err := runMapListFromClusterWithConfigAndDiagnostics(context.Background(), cfg, nil, false); err != nil {
			t.Errorf("run map list: %v", err)
		}
	})
	wantPaths := []string{
		"/apis/argoproj.io/v1alpha1/namespaces/team-a/applicationsets", // retained lineage dependency
		"/apis/apps/v1/namespaces/team-a/deployments",
		"/apis/custom.example/v1/namespaces/team-a/widgets", // unknown Kind retained
	}
	gotPaths := make([]string, 0, len(counts))
	for path, count := range counts {
		if count != 1 {
			t.Errorf("request %s count = %d, want 1", path, count)
		}
		gotPaths = append(gotPaths, path)
	}
	sort.Strings(gotPaths)
	sort.Strings(wantPaths)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Errorf("live request paths = %v, want exactly %v", gotPaths, wantPaths)
	}
	if !strings.Contains(output, `"name": "owned"`) || !strings.Contains(output, `"name": "duplicate-kind"`) || !strings.Contains(output, `"owner": "Flux"`) {
		t.Errorf("filtered output lost matching resource/ownership evidence: %s", output)
	}
	if strings.Contains(output, `"name": "not-a-match"`) {
		t.Errorf("unexpected nonmatching resource in output: %s", output)
	}
}

func TestMapListKindSelectionConservativeControls(t *testing.T) {
	all := collectMapResourceList()
	custom := schema.GroupVersionResource{Group: "custom.example", Version: "v1", Resource: "widgets"}
	all = append(all, custom)
	deployment := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	fluxInstance := schema.GroupVersionResource{Group: "fluxcd.controlplane.io", Version: "v1", Resource: "fluxinstances"}
	for _, tc := range []struct {
		name string
		kind string
		want []schema.GroupVersionResource
	}{
		{name: "exact known kind", kind: "Deployment", want: []schema.GroupVersionResource{deployment, custom}},
		{name: "first class controller kind", kind: "FluxInstance", want: []schema.GroupVersionResource{fluxInstance, custom}},
		{name: "empty filter retains all", kind: "", want: all},
		{name: "unsupported or noncanonical filter retains all", kind: "deployment", want: all},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mapResourcesForKind(all, tc.kind)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("mapResourcesForKind(%q) = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

func TestMapListUnfilteredRequestsPreserveEmptyAndDeniedDiagnostics(t *testing.T) {
	counts := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counts[r.URL.Path]++
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/statefulsets") {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403}`)
			return
		}
		items := []map[string]interface{}{}
		if strings.HasSuffix(r.URL.Path, "/deployments") {
			items = append(items, map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "visible", "namespace": "team-a"}})
		}
		_, _ = fmt.Fprintf(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":%s}`, mapKindTestJSON(t, items))
	}))
	defer server.Close()

	oldKind, oldNamespace := mapKind, mapNamespace
	oldFormat, oldJSON, oldLoader := mapListFormat, mapJSON, mapWatchLoadCustomResourceConfigs
	t.Cleanup(func() {
		mapKind, mapNamespace = oldKind, oldNamespace
		mapListFormat, mapJSON = oldFormat, oldJSON
		mapWatchLoadCustomResourceConfigs = oldLoader
	})
	mapKind, mapNamespace = "", "team-a"
	mapListFormat, mapJSON = "json", false
	mapWatchLoadCustomResourceConfigs = func() []customResourceConfig { return nil }
	config := clientcmdapi.NewConfig()
	config.CurrentContext = "offline"
	config.Clusters["offline"] = &clientcmdapi.Cluster{Server: server.URL}
	config.Contexts["offline"] = &clientcmdapi.Context{Cluster: "offline"}
	data, err := clientcmd.Write(*config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.QPS, cfg.Burst = 1000, 1000 // fixture measures request selection, not client throttling
	output := captureStdout(t, func() {
		if err := runMapListFromClusterWithConfigAndDiagnostics(context.Background(), cfg, nil, true); err != nil {
			t.Errorf("run map list: %v", err)
		}
	})
	wantCount := len(collectMapResourceList()) + 1 // separate ApplicationSet lookup
	if len(counts) != wantCount {
		t.Errorf("unfiltered request count = %d, want %d", len(counts), wantCount)
	}
	for requestPath, count := range counts {
		if count != 1 || !strings.Contains(requestPath, "/namespaces/team-a/") {
			t.Errorf("unfiltered request %s count=%d; want one request scoped to team-a", requestPath, count)
		}
	}
	for _, want := range []string{`"name": "visible"`, `"reason": "forbidden"`, `"resource": "statefulsets"`, `"namespace": "team-a"`} {
		if !strings.Contains(output, want) {
			t.Errorf("unfiltered populated/empty/denied result missing %q: %s", want, output)
		}
	}

	// An unrecognized filter conservatively issues the same collection set; it
	// can still render no rows because the output filter remains exact.
	for path := range counts {
		delete(counts, path)
	}
	mapKind = "NotAKnownKind"
	unsupportedOutput := captureStdout(t, func() {
		if err := runMapListFromClusterWithConfigAndDiagnostics(context.Background(), cfg, nil, true); err != nil {
			t.Errorf("run unsupported-kind map list: %v", err)
		}
	})
	if len(counts) != wantCount {
		t.Errorf("unsupported-kind request count = %d, want conservative full set %d", len(counts), wantCount)
	}
	for requestPath, count := range counts {
		if count != 1 || !strings.Contains(requestPath, "/namespaces/team-a/") {
			t.Errorf("unsupported-kind request %s count=%d; want one request scoped to team-a", requestPath, count)
		}
	}
	if !strings.Contains(unsupportedOutput, `"reason": "forbidden"`) {
		t.Errorf("unsupported-kind fallback hid denied-collection diagnostic: %s", unsupportedOutput)
	}
}

func TestMapListKnownKindFilteredEmptyAndForbiddenResponses(t *testing.T) {
	for _, tc := range []struct {
		name          string
		forbidden     bool
		wantStatus    string
		wantOmissions int
	}{
		{name: "successful empty list", wantStatus: "complete"},
		{name: "forbidden list is partial", forbidden: true, wantStatus: "partial", wantOmissions: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counts := map[string]int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				counts[r.URL.Path]++
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/deployments") && tc.forbidden {
					w.WriteHeader(http.StatusForbidden)
					_, _ = fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403}`)
					return
				}
				apiVersion, kind := "apps/v1", "DeploymentList"
				if strings.HasSuffix(r.URL.Path, "/applicationsets") {
					apiVersion, kind = "argoproj.io/v1alpha1", "ApplicationSetList"
				}
				_, _ = fmt.Fprintf(w, `{"apiVersion":%q,"kind":%q,"metadata":{},"items":[]}`, apiVersion, kind)
			}))

			oldKind, oldNamespace := mapKind, mapNamespace
			oldFormat, oldJSON, oldLoader := mapListFormat, mapJSON, mapWatchLoadCustomResourceConfigs
			t.Cleanup(func() {
				server.Close()
				mapKind, mapNamespace = oldKind, oldNamespace
				mapListFormat, mapJSON = oldFormat, oldJSON
				mapWatchLoadCustomResourceConfigs = oldLoader
			})
			mapKind, mapNamespace = "Deployment", "team-a"
			mapListFormat, mapJSON = "json", false
			mapWatchLoadCustomResourceConfigs = func() []customResourceConfig { return nil }

			cfg := mapKindTestConfig(t, server.URL)
			outputText := captureStdout(t, func() {
				if err := runMapListFromClusterWithConfigAndDiagnostics(context.Background(), cfg, nil, true); err != nil {
					t.Errorf("run filtered map list: %v", err)
				}
			})
			wantPaths := []string{
				"/apis/apps/v1/namespaces/team-a/deployments",
				"/apis/argoproj.io/v1alpha1/namespaces/team-a/applicationsets",
			}
			gotPaths := make([]string, 0, len(counts))
			for path, count := range counts {
				if count != 1 {
					t.Errorf("request %s count = %d, want 1", path, count)
				}
				gotPaths = append(gotPaths, path)
			}
			sort.Strings(gotPaths)
			sort.Strings(wantPaths)
			if !reflect.DeepEqual(gotPaths, wantPaths) {
				t.Fatalf("filtered request paths = %v, want exactly %v", gotPaths, wantPaths)
			}

			var output mapsvc.OwnershipEvidenceOutput
			if err := json.Unmarshal([]byte(outputText), &output); err != nil {
				t.Fatalf("decode filtered evidence output: %v\n%s", err, outputText)
			}
			if output.Collection.Status != tc.wantStatus {
				t.Errorf("collection status = %q, want %q: %+v", output.Collection.Status, tc.wantStatus, output.Collection)
			}
			if len(output.Resources) != 0 {
				t.Errorf("filtered empty/denied response reported resources: %+v", output.Resources)
			}
			if len(output.Collection.Omissions) != tc.wantOmissions {
				t.Errorf("omissions = %+v, want %d", output.Collection.Omissions, tc.wantOmissions)
			}
			if tc.forbidden && len(output.Collection.Omissions) == 1 {
				omission := output.Collection.Omissions[0]
				if omission.APIVersion != "apps/v1" || omission.Resource != "deployments" || omission.Namespace != "team-a" || omission.Reason != "forbidden" {
					t.Errorf("filtered denial omission = %+v", omission)
				}
			}
			for _, overclaim := range []string{"orphan", "cluster is empty", "no resources exist"} {
				if strings.Contains(strings.ToLower(outputText), overclaim) {
					t.Errorf("filtered output overclaims %q: %s", overclaim, outputText)
				}
			}
		})
	}
}

func mapKindTestConfig(t *testing.T, serverURL string) *rest.Config {
	t.Helper()
	config := clientcmdapi.NewConfig()
	config.CurrentContext = "offline"
	config.Clusters["offline"] = &clientcmdapi.Cluster{Server: serverURL}
	config.Contexts["offline"] = &clientcmdapi.Context{Cluster: "offline"}
	data, err := clientcmd.Write(*config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.QPS, cfg.Burst = 1000, 1000
	return cfg
}

func mapKindTestJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
