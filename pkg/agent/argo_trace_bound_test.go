// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

var boundArgoApplicationGVR = schema.GroupVersionResource{
	Group: "argoproj.io", Version: "v1alpha1", Resource: "applications",
}

func boundArgoApplication(name, namespace, repo, revision string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata": map[string]interface{}{
			"name": name, "namespace": namespace, "uid": "uid-" + namespace,
		},
		"spec": map[string]interface{}{
			"source": map[string]interface{}{
				"repoURL": repo, "path": "deploy", "targetRevision": "main",
			},
			"destination": map[string]interface{}{"namespace": "workloads"},
		},
		"status": map[string]interface{}{
			"sync":      map[string]interface{}{"status": "Synced", "revision": revision},
			"health":    map[string]interface{}{"status": "Healthy"},
			"resources": []interface{}{},
		},
	}}
}

func newBoundArgoFake(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		boundArgoApplicationGVR: "ApplicationList",
	}, objects...)
}

func TestBoundArgoTracerGetsExactNamespaceAndReusesParser(t *testing.T) {
	client := newBoundArgoFake(
		boundArgoApplication("checkout", "argocd", "https://example.invalid/argocd.git", "rev-a"),
		boundArgoApplication("checkout", "team-b", "https://example.invalid/team-b.git", "rev-b"),
	)
	tracer := NewArgoTracerWithKubernetesClient(client)
	result, err := tracer.Trace(context.Background(), "Application", "checkout", "team-b")
	if err != nil {
		t.Fatalf("TraceApplication() error = %v", err)
	}
	if got := result.Object.Namespace; got != "team-b" {
		t.Fatalf("result namespace = %q, want team-b", got)
	}
	if got := result.Chain[0].URL; got != "https://example.invalid/team-b.git" {
		t.Fatalf("source URL = %q, want team-b Application source", got)
	}
	if got := result.Chain[1].Revision; got != "rev-b" {
		t.Fatalf("application revision = %q, want rev-b", got)
	}
	actions := client.Actions()
	if len(actions) != 1 || actions[0].GetVerb() != "get" || actions[0].GetNamespace() != "team-b" || actions[0].GetResource() != boundArgoApplicationGVR {
		t.Fatalf("actions = %#v, want one namespaced Application get", actions)
	}

	app, err := client.Resource(boundArgoApplicationGVR).Namespace("team-b").Get(context.Background(), "checkout", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want, err := tracer.parseAppOutput(mustMarshalBoundApp(t, app), "checkout", "team-b")
	if err != nil {
		t.Fatal(err)
	}
	if result.Chain[0].URL != want.Chain[0].URL || result.Chain[1].Status != want.Chain[1].Status || result.FullyManaged != want.FullyManaged {
		t.Fatalf("bound parse differs from existing parser: got %#v, want %#v", result, want)
	}
}

func TestBoundArgoTracerListsOnlyToRequireUniqueName(t *testing.T) {
	t.Run("one exact name succeeds", func(t *testing.T) {
		client := newBoundArgoFake(boundArgoApplication("checkout", "team-b", "https://example.invalid/b.git", "b"))
		result, err := NewArgoTracerWithKubernetesClient(client).TraceApplication(context.Background(), "checkout")
		if err != nil {
			t.Fatalf("TraceApplication() error = %v", err)
		}
		if result.Object.Namespace != "team-b" || result.Chain[0].URL != "https://example.invalid/b.git" {
			t.Fatalf("selected wrong Application: %#v", result)
		}
		actions := client.Actions()
		if len(actions) != 1 || actions[0].GetVerb() != "list" || actions[0].GetNamespace() != "" {
			t.Fatalf("actions = %#v, want one all-namespace list", actions)
		}
	})

	t.Run("colliding names are ambiguous", func(t *testing.T) {
		client := newBoundArgoFake(
			boundArgoApplication("checkout", "argocd", "https://example.invalid/a.git", "a"),
			boundArgoApplication("checkout", "team-b", "https://example.invalid/b.git", "b"),
		)
		_, err := NewArgoTracerWithKubernetesClient(client).TraceApplication(context.Background(), "checkout")
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("TraceApplication() error = %v, want ambiguity", err)
		}
	})
}

func TestBoundArgoTracerReportsMissingDeniedAndMalformedReads(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		client := newBoundArgoFake()
		_, err := NewArgoTracerWithKubernetesClient(client).TraceApplicationInNamespace(context.Background(), "missing", "team-a")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("error = %v, want not found", err)
		}
	})

	t.Run("denied", func(t *testing.T) {
		client := newBoundArgoFake()
		client.PrependReactor("get", "applications", func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(boundArgoApplicationGVR.GroupResource(), "checkout", errors.New("denied"))
		})
		_, err := NewArgoTracerWithKubernetesClient(client).TraceApplicationInNamespace(context.Background(), "checkout", "team-a")
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "forbidden") {
			t.Fatalf("error = %v, want RBAC failure", err)
		}
	})

	t.Run("malformed identity", func(t *testing.T) {
		app := boundArgoApplication("wrong-name", "team-a", "https://example.invalid/repo.git", "rev")
		client := newBoundArgoFake()
		client.PrependReactor("get", "applications", func(ktesting.Action) (bool, runtime.Object, error) {
			return true, app, nil
		})
		_, err := NewArgoTracerWithKubernetesClient(client).TraceApplicationInNamespace(context.Background(), "checkout", "team-a")
		if err == nil || !strings.Contains(err.Error(), "identity") {
			t.Fatalf("error = %v, want identity validation failure", err)
		}
	})

	t.Run("malformed JSON shape", func(t *testing.T) {
		app := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata":   map[string]interface{}{"name": "checkout", "namespace": "team-a"},
			"spec":       "invalid",
		}}
		client := newBoundArgoFake(app)
		_, err := NewArgoTracerWithKubernetesClient(client).TraceApplicationInNamespace(context.Background(), "checkout", "team-a")
		if err == nil {
			t.Fatal("expected malformed Application error")
		}
	})
}

func TestExplicitBoundArgoTracerFailsClosedAndNeverExecutesCLIs(t *testing.T) {
	tracer := NewArgoTracerWithKubernetesClient(nil)
	if tracer.Available() {
		t.Fatal("nil explicitly bound client must be unavailable")
	}
	if _, err := tracer.TraceApplicationInNamespace(context.Background(), "checkout", "team-a"); err == nil || !strings.Contains(err.Error(), "bound Kubernetes client") {
		t.Fatalf("TraceApplication() error = %v, want fail-closed error", err)
	}

	dir := t.TempDir()
	marker := filepath.Join(dir, "invoked")
	for _, name := range []string{"argocd", "kubectl"} {
		path := filepath.Join(dir, name)
		script := "#!/bin/sh\nprintf invoked >> '" + marker + "'\nexit 99\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	tracer = NewArgoTracerWithKubernetesClient(newBoundArgoFake(boundArgoApplication("checkout", "team-a", "https://example.invalid/repo.git", "rev")))
	if !tracer.Available() {
		t.Fatal("valid explicitly bound client should be available")
	}
	if _, err := tracer.TraceApplicationInNamespace(context.Background(), "checkout", "team-a"); err != nil {
		t.Fatalf("TraceApplication() error = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("external CLI sentinel exists or stat failed: %v", err)
	}
}

func mustMarshalBoundApp(t *testing.T, app *unstructured.Unstructured) []byte {
	t.Helper()
	data, err := json.Marshal(app.Object)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
