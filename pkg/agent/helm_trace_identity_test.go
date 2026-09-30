// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const helmManifestIdentityFixtureDir = "testdata/helm-manifest-identity"

func readHelmManifestIdentityFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(helmManifestIdentityFixtureDir, name))
	if err != nil {
		t.Fatalf("read Helm manifest fixture %q: %v", name, err)
	}
	return string(b)
}

func helmManifestIdentitySecret(t *testing.T, releaseName, manifest string) *corev1.Secret {
	t.Helper()
	release := &helmRelease{
		Name:      releaseName,
		Namespace: "team-a",
		Version:   1,
		Info:      helmReleaseInfo{Status: "deployed"},
		Manifest:  manifest,
	}
	secret := helmSecret(encodeRelease(t, release), releaseName, release.Version)
	secret.Namespace = release.Namespace
	return secret
}

func helmManifestIdentityTracer(secrets ...*corev1.Secret) *HelmTracer {
	objects := make([]runtime.Object, len(secrets))
	for i := range secrets {
		objects[i] = secrets[i]
	}
	return NewHelmTracer(fake.NewSimpleClientset(objects...))
}

func traceHelmManifestIdentity(t *testing.T, tracer *HelmTracer, kind, name, namespace string) (*TraceResult, error) {
	t.Helper()
	return tracer.Trace(context.Background(), kind, name, namespace)
}

func requireHelmTraceManaged(t *testing.T, result *TraceResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Trace() error = %v; want a unique exact match", err)
	}
	if result == nil || !result.FullyManaged || result.Error != "" {
		t.Fatalf("Trace() = %+v; want confirmed managed trace", result)
	}
}

func requireHelmTraceAbsent(t *testing.T, result *TraceResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("Trace() error = %v; want a valid, nonmatching manifest", err)
	}
	if result == nil || result.FullyManaged || result.Error == "" {
		t.Fatalf("Trace() = %+v; want explicit no-match and unmanaged result", result)
	}
}

func requireHelmTraceUnresolved(t *testing.T, result *TraceResult, err error) {
	t.Helper()
	if err == nil || result != nil {
		t.Fatalf("Trace() = (%+v, %v); want explicit unresolved error and no result", result, err)
	}
	if strings.Contains(err.Error(), "api:\n") || strings.Contains(err.Error(), "team-a") {
		t.Fatalf("Trace() error contains manifest payload: %v", err)
	}
}

func TestHelmTraceUsesExactStructuredManifestIdentity(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		kind     string
		resource string
		ns       string
		managed  bool
	}{
		{"exact with empty and comment docs", readHelmManifestIdentityFixture(t, "valid.yaml"), "Deployment", "api", "team-a", true},
		{"prefix lookalike", readHelmManifestIdentityFixture(t, "prefix-lookalike.yaml"), "Deployment", "api", "team-a", false},
		{"comment lookalike", readHelmManifestIdentityFixture(t, "comment-lookalike.yaml"), "Deployment", "api", "team-a", false},
		{"nested name lookalike", readHelmManifestIdentityFixture(t, "nested-lookalike.yaml"), "Deployment", "api", "team-a", false},
		{"wrong namespace", readHelmManifestIdentityFixture(t, "wrong-namespace.yaml"), "Deployment", "api", "team-a", false},
		{"name is case sensitive", readHelmManifestIdentityFixture(t, "valid.yaml"), "Deployment", "API", "team-a", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracer := helmManifestIdentityTracer(helmManifestIdentitySecret(t, "release", tc.manifest))
			result, err := traceHelmManifestIdentity(t, tracer, tc.kind, tc.resource, tc.ns)
			if tc.managed {
				requireHelmTraceManaged(t, result, err)
			} else {
				requireHelmTraceAbsent(t, result, err)
			}
		})
	}
}

func TestHelmTraceDoesNotInferMissingNamespace(t *testing.T) {
	tracer := helmManifestIdentityTracer(helmManifestIdentitySecret(t, "release", readHelmManifestIdentityFixture(t, "omitted-namespace.yaml")))
	result, err := traceHelmManifestIdentity(t, tracer, "Deployment", "api", "team-a")
	requireHelmTraceUnresolved(t, result, err)

	// With no caller namespace, Trace cannot establish whether the resource is
	// namespaced or cluster-scoped. It must not treat empty namespace as a wildcard.
	tracer = helmManifestIdentityTracer(helmManifestIdentitySecret(t, "release", readHelmManifestIdentityFixture(t, "valid.yaml")))
	result, err = traceHelmManifestIdentity(t, tracer, "Deployment", "api", "")
	requireHelmTraceUnresolved(t, result, err)
}

func TestHelmTraceRejectsUnreadableOrUnsupportedManifestDocuments(t *testing.T) {
	for _, fixture := range []string{
		"malformed.yaml", "valid-plus-malformed.yaml", "non-object.yaml", "unsupported.yaml", "missing-api-version.yaml",
		"api-version-number.yaml", "kind-boolean.yaml", "name-number.yaml", "namespace-number.yaml", "kubernetes-list.yaml",
	} {
		t.Run(fixture, func(t *testing.T) {
			tracer := helmManifestIdentityTracer(helmManifestIdentitySecret(t, "release", readHelmManifestIdentityFixture(t, fixture)))
			result, err := traceHelmManifestIdentity(t, tracer, "Deployment", "api", "team-a")
			requireHelmTraceUnresolved(t, result, err)
		})
	}
}

func orderedHelmManifestTracer(secrets []*corev1.Secret) *HelmTracer {
	client := fake.NewSimpleClientset()
	client.PrependReactor("list", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		items := make([]corev1.Secret, len(secrets))
		for i := range secrets {
			items[i] = *secrets[i]
		}
		return true, &corev1.SecretList{Items: items}, nil
	})
	return NewHelmTracer(client)
}

func TestHelmTraceRejectsDuplicateManifestIdentitiesRegardlessOfOrder(t *testing.T) {
	for _, fixture := range []string{"duplicate-api-versions.yaml", "duplicate-same-api-version.yaml"} {
		valid := readHelmManifestIdentityFixture(t, fixture)
		for order, manifest := range []string{valid, reverseManifestDocs(valid)} {
			t.Run(fmt.Sprintf("%s-order-%d", fixture, order), func(t *testing.T) {
				tracer := helmManifestIdentityTracer(helmManifestIdentitySecret(t, "release", manifest))
				result, err := traceHelmManifestIdentity(t, tracer, "Deployment", "api", "team-a")
				requireHelmTraceUnresolved(t, result, err)
			})
		}
	}

	first := helmManifestIdentitySecret(t, "first-release", readHelmManifestIdentityFixture(t, "valid.yaml"))
	second := helmManifestIdentitySecret(t, "second-release", readHelmManifestIdentityFixture(t, "valid.yaml"))
	for order, secrets := range [][]*corev1.Secret{{first, second}, {second, first}} {
		t.Run(fmt.Sprintf("release-order-%d", order), func(t *testing.T) {
			tracer := orderedHelmManifestTracer(secrets)
			result, err := traceHelmManifestIdentity(t, tracer, "Deployment", "api", "team-a")
			requireHelmTraceUnresolved(t, result, err)
		})
	}
}

func reverseManifestDocs(manifest string) string {
	parts := strings.Split(manifest, "---")
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return strings.Join(parts, "---")
}
