// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/internal/mapsvc"
	"github.com/confighub/cub-scout/pkg/agent"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestMapOwnershipEvidenceRenderersAndLegacyJSON(t *testing.T) {
	entry := MapEntry{
		ClusterName: "local", APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api", Owner: "Flux",
		OwnerDetails: map[string]string{"name": "app"}, Labels: map[string]string{"large": strings.Repeat("x", 1024)}, Status: "Ready",
		OwnershipDetection: mapsvc.NewOwnershipDetectionEvidence(agent.Ownership{Type: agent.OwnerFlux, Source: "label:kustomize.toolkit.fluxcd.io/name"}),
	}
	oldFormat, err := json.MarshalIndent([]MapEntry{entry}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	saveFormat, saveJSON, saveVerbose, saveOwner := mapListFormat, mapJSON, mapVerbose, mapOwner
	defer func() { mapListFormat, mapJSON, mapVerbose, mapOwner = saveFormat, saveJSON, saveVerbose, saveOwner }()
	mapOwner = ""
	mapJSON = false
	mapVerbose = false
	mapListFormat = "json"
	legacy := captureStdout(t, func() {
		if err := renderMapListFromEntries([]MapEntry{entry}); err != nil {
			t.Errorf("legacy render: %v", err)
		}
	})
	if legacy != string(oldFormat)+"\n" {
		t.Fatalf("default JSON changed with internal diagnostics field\nwant %q\ngot  %q", string(oldFormat)+"\n", legacy)
	}

	var evidenceJSON string
	evidenceJSON = captureStdout(t, func() {
		if err := renderMapListFromEntriesWithDiagnostics([]MapEntry{entry}, []mapsvc.CollectionOmission{{APIVersion: "apps/v1", Resource: "statefulsets", Reason: "forbidden"}}, true); err != nil {
			t.Errorf("evidence render: %v", err)
		}
	})
	for _, want := range []string{`"schema": "map-list-ownership-evidence.v1"`, `"source": "label:kustomize.toolkit.fluxcd.io/name"`, `"reason": "forbidden"`} {
		if !strings.Contains(evidenceJSON, want) {
			t.Errorf("JSON evidence missing %q: %s", want, evidenceJSON)
		}
	}
	for _, absent := range []string{`"labels"`, `"createdAt"`, `"observation"`, strings.Repeat("x", 100)} {
		if strings.Contains(evidenceJSON, absent) {
			t.Errorf("compact evidence includes %q", absent)
		}
	}
	if len(evidenceJSON) >= len(legacy) {
		t.Fatalf("ownership projection should be smaller for this synthetic metadata-heavy example: full=%d diagnostics=%d", len(legacy), len(evidenceJSON))
	}
	t.Logf("synthetic one-entry JSON bytes: full=%d ownership-diagnostics=%d", len(legacy), len(evidenceJSON))
	for _, format := range []string{"ascii", "md"} {
		mapListFormat = format
		text := captureStdout(t, func() {
			if err := renderMapListFromEntriesWithDiagnostics([]MapEntry{entry}, []mapsvc.CollectionOmission{{APIVersion: "apps/v1", Resource: "statefulsets", Reason: "forbidden"}}, true); err != nil {
				t.Errorf("%s render: %v", format, err)
			}
		})
		if !strings.Contains(text, "detected via label:kustomize.toolkit.fluxcd.io/name") || !strings.Contains(text, "statefulsets not returned (forbidden)") {
			t.Errorf("%s evidence/omission missing: %s", format, text)
		}
	}
}

func TestMapExplainDefaultProseRemainsStable(t *testing.T) {
	saveFormat, saveJSON, saveExplain, saveOwner := mapListFormat, mapJSON, mapExplain, mapOwner
	defer func() { mapListFormat, mapJSON, mapExplain, mapOwner = saveFormat, saveJSON, saveExplain, saveOwner }()
	mapListFormat, mapJSON, mapExplain, mapOwner = "ascii", false, true, ""
	output := captureStdout(t, func() {
		if err := renderMapListFromEntries([]MapEntry{{Kind: "Deployment", Name: "legacy", Namespace: "default", Owner: "Native"}}); err != nil {
			t.Errorf("render default --explain: %v", err)
		}
	})
	for _, legacy := range []string{
		"NATIVE means no GitOps tool claims ownership (kubectl-applied).",
		"• 1 resources are Native → No detected GitOps or platform controller ownership",
	} {
		if !strings.Contains(output, legacy) {
			t.Errorf("legacy --explain prose changed, missing %q", legacy)
		}
	}
}

func TestMapOwnershipEvidenceViewUsesLoadedEntriesOnly(t *testing.T) {
	m := initialLocalModel()
	m.entries = []MapEntry{{Kind: "Deployment", Namespace: "team-a", Name: "api", Owner: "Native", OwnershipDetection: mapsvc.NewOwnershipDetectionEvidence(agent.Ownership{Type: agent.OwnerUnknown})}}
	m.ownershipOmissions = []mapsvc.CollectionOmission{{APIVersion: "apps/v1", Resource: "deployments", Namespace: "team-a", Reason: "forbidden"}}
	before := len(m.entries)
	got := m.getPanelOwnershipEvidence()
	for _, want := range []string{"team-a/api Deployment", "no known marker", "does not establish that a resource is orphaned", "partial (1 workload list request(s) omitted)", "forbidden"} {
		if !strings.Contains(got, want) {
			t.Errorf("evidence view missing %q: %s", want, got)
		}
	}
	if len(m.entries) != before {
		t.Fatal("rendering evidence changed loaded inventory")
	}
}

func TestMapListCollectionOmissionIsSanitized(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"forbidden", apierrors.NewForbidden(gvr.GroupResource(), "secret", errors.New("sensitive server detail")), "forbidden"},
		{"missing", apierrors.NewNotFound(gvr.GroupResource(), "secret"), "not_found"},
		{"other", errors.New("sensitive server detail"), "request_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mapListCollectionOmission(gvr, "team-a", tc.err)
			if got.Reason != tc.want || got.Namespace != "team-a" || strings.Contains(got.Reason, "sensitive") {
				t.Fatalf("unsafe omission: %+v", got)
			}
		})
	}
}

func TestMapProcessOwnershipEvidenceUsesAlreadyReadObject(t *testing.T) {
	// Evidence is derived from the exact object already passed through the
	// canonical detector. This regression locks that no second data source or
	// follow-up read is introduced into resource processing.
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "api", "namespace": "team-a", "labels": map[string]interface{}{"kustomize.toolkit.fluxcd.io/name": "app"}}}}
	entries := processResourceWithLookup(obj, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, "local", nil, map[string]int{}, mapApplicationSetLookup{byNamespacedName: map[string]int{}, byName: map[string]int{}})
	if len(entries) != 1 || entries[0].OwnershipDetection.Status != "detected" || !strings.HasPrefix(entries[0].OwnershipDetection.Source, "label:") {
		t.Fatalf("canonical detection was not attached: %+v", entries)
	}
}

func TestMapListOwnershipEvidenceAddsNoRequests(t *testing.T) {
	requestCounts := map[bool]map[string]int{}
	for _, includeEvidence := range []bool{false, true} {
		counts := map[string]int{}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			counts[r.Method+" "+r.URL.Path]++
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/apis/argoproj.io/v1alpha1/applicationsets" {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"applicationsets forbidden","reason":"Forbidden","code":403}`)
				return
			}
			fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","metadata":{},"items":[]}`)
		}))
		config := clientcmdapi.NewConfig()
		config.CurrentContext = "offline"
		config.Clusters["offline"] = &clientcmdapi.Cluster{Server: server.URL}
		config.Contexts["offline"] = &clientcmdapi.Context{Cluster: "offline"}
		data, err := clientcmd.Write(*config)
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "config")
		if err := os.WriteFile(path, data, 0600); err != nil {
			server.Close()
			t.Fatal(err)
		}
		t.Setenv("KUBECONFIG", path)
		t.Setenv("CLUSTER_NAME", "offline")
		save := struct {
			namespace, kind, owner, query, since, format   string
			json, verbose, summary, count, names, evidence bool
		}{
			mapNamespace, mapKind, mapOwner, mapQuery, mapSince, mapListFormat, mapJSON, mapVerbose, mapSummary, mapCount, mapNamesOnly, mapOwnershipEvidence,
		}
		mapNamespace, mapKind, mapOwner, mapQuery, mapSince = "", "", "", "", ""
		mapListFormat, mapJSON, mapVerbose, mapSummary, mapCount, mapNamesOnly = "json", false, false, false, false, false
		mapOwnershipEvidence = includeEvidence
		output := captureStdout(t, func() {
			cfg, configErr := buildConfig()
			if err := runMapListFromClusterWithConfigAndDiagnostics(context.Background(), cfg, configErr, includeEvidence); err != nil {
				t.Errorf("run list evidence=%v: %v", includeEvidence, err)
			}
		})
		if includeEvidence && (!strings.Contains(output, `"resource": "applicationsets"`) || !strings.Contains(output, `"reason": "forbidden"`)) {
			t.Errorf("denied ApplicationSet lookup was not surfaced in diagnostics: %s", output)
		}
		mapNamespace, mapKind, mapOwner, mapQuery, mapSince = save.namespace, save.kind, save.owner, save.query, save.since
		mapListFormat, mapJSON, mapVerbose, mapSummary, mapCount, mapNamesOnly, mapOwnershipEvidence = save.format, save.json, save.verbose, save.summary, save.count, save.names, save.evidence
		server.Close()
		requestCounts[includeEvidence] = counts
	}
	if !reflect.DeepEqual(requestCounts[false], requestCounts[true]) {
		t.Fatalf("ownership diagnostics changed Kubernetes reads\nwithout: %v\nwith: %v", requestCounts[false], requestCounts[true])
	}
}
