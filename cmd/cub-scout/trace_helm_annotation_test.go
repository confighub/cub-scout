package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// What Helm 3 and 4 leave on an object whose chart sets no labels of its own,
// as recorded on a real install (CI run 43869810212): the managed-by label
// and the two release annotations, and no app.kubernetes.io/instance.
func helmBareDeployment(name, namespace, releaseName, releaseNamespace string) *unstructured.Unstructured {
	annotations := map[string]any{}
	if releaseName != "" {
		annotations["meta.helm.sh/release-name"] = releaseName
	}
	if releaseNamespace != "" {
		annotations["meta.helm.sh/release-namespace"] = releaseNamespace
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":        name,
			"namespace":   namespace,
			"labels":      map[string]any{"app": name, "app.kubernetes.io/managed-by": "Helm"},
			"annotations": annotations,
		},
	}}
}

// A release record as Helm stores it: gzip, then base64, in a Secret. The
// manifest has no metadata.namespace, as most charts render.
func helmReleaseSecretForTrace(t *testing.T, releaseName, namespace, deploymentName string) *corev1.Secret {
	t.Helper()
	record := map[string]any{
		"name":      releaseName,
		"namespace": namespace,
		"version":   1,
		"info":      map[string]any{"status": "deployed"},
		"chart":     map[string]any{"metadata": map[string]any{"name": "helm-lane", "version": "0.1.0"}},
		"manifest":  "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: " + deploymentName + "\n",
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1." + releaseName + ".v1",
			Namespace: namespace,
			Labels:    map[string]string{"owner": "helm", "name": releaseName, "version": "1", "status": "deployed"},
		},
		Type: corev1.SecretType("helm.sh/release.v1"),
		Data: map[string][]byte{"release": []byte(base64.StdEncoding.EncodeToString(buf.Bytes()))},
	}
}

func helmAnnotationTraceSession(t *testing.T, object *unstructured.Unstructured, secrets ...runtime.Object) *traceSession {
	t.Helper()
	session := &traceSession{config: &rest.Config{Host: "https://example.invalid"}}
	session.dynamic = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), object)
	session.dynamicOnce.Do(func() {})
	session.kube = kubefake.NewSimpleClientset(secrets...)
	session.kubeOnce.Do(func() {})
	return session
}

func TestTraceReadsTheHelmReleaseNamedByHelmsOwnAnnotations(t *testing.T) {
	object := helmBareDeployment("v4-bare", "helm-lane", "v4-bare", "helm-lane")
	session := helmAnnotationTraceSession(t, object, helmReleaseSecretForTrace(t, "v4-bare", "helm-lane", "v4-bare"))

	observation, err := observeTrace(context.Background(), session, "Deployment", "v4-bare", "helm-lane", traceObservationOptions{})
	if err != nil {
		t.Fatalf("observeTrace() error = %v; before the fix this failed on the manifest's absent namespace", err)
	}
	result := observation.Result
	if result == nil || !result.FullyManaged || len(result.Chain) != 2 {
		t.Fatalf("result = %#v; want the chart and release links", result)
	}
	release := result.Chain[1]
	if release.Kind != "Release" || release.Name != "v4-bare" || release.Namespace != "helm-lane" {
		t.Fatalf("release link = %#v; want Release helm-lane/v4-bare", release)
	}
}

func TestTraceReadsAHelmReleaseInTheNamespaceHelmRecorded(t *testing.T) {
	// Helm may install an object outside its release's namespace; the
	// annotation says where the release record lives.
	object := helmBareDeployment("api", "team-a", "platform", "platform-system")
	session := helmAnnotationTraceSession(t, object, helmReleaseSecretForTrace(t, "platform", "platform-system", "api"))

	observation, err := observeTrace(context.Background(), session, "Deployment", "api", "team-a", traceObservationOptions{})
	if err != nil {
		t.Fatalf("observeTrace() error = %v", err)
	}
	if got := observation.Result.Chain[1]; got.Name != "platform" || got.Namespace != "platform-system" {
		t.Fatalf("release link = %#v; want Release platform-system/platform", got)
	}
}

func TestWithHelmReleaseAnnotationsNeverInfers(t *testing.T) {
	both := map[string]string{"meta.helm.sh/release-name": "web", "meta.helm.sh/release-namespace": "apps"}
	cases := []struct {
		name        string
		ownership   agent.Ownership
		annotations map[string]string
		want        agent.Ownership
	}{
		{"both annotations name the release",
			agent.Ownership{Type: agent.OwnerHelm},
			both,
			agent.Ownership{Type: agent.OwnerHelm, Name: "web", Namespace: "apps"}},
		{"a label-named release is left as it is",
			agent.Ownership{Type: agent.OwnerHelm, Name: "from-label"},
			both,
			agent.Ownership{Type: agent.OwnerHelm, Name: "from-label"}},
		{"name without namespace is not enough",
			agent.Ownership{Type: agent.OwnerHelm},
			map[string]string{"meta.helm.sh/release-name": "web"},
			agent.Ownership{Type: agent.OwnerHelm}},
		{"namespace without name is not enough",
			agent.Ownership{Type: agent.OwnerHelm},
			map[string]string{"meta.helm.sh/release-namespace": "apps"},
			agent.Ownership{Type: agent.OwnerHelm}},
		{"blank values are absent",
			agent.Ownership{Type: agent.OwnerHelm},
			map[string]string{"meta.helm.sh/release-name": " ", "meta.helm.sh/release-namespace": "apps"},
			agent.Ownership{Type: agent.OwnerHelm}},
		{"another owner is never turned into Helm",
			agent.Ownership{Type: agent.OwnerFlux, Name: "apps"},
			both,
			agent.Ownership{Type: agent.OwnerFlux, Name: "apps"}},
		{"no owner stays no owner",
			agent.Ownership{Type: agent.OwnerUnknown},
			both,
			agent.Ownership{Type: agent.OwnerUnknown}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withHelmReleaseAnnotations(tc.ownership, tc.annotations); got != tc.want {
				t.Fatalf("withHelmReleaseAnnotations() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
