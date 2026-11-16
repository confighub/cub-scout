package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	return helmReleaseSecretWithManifest(t, releaseName, namespace, "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: "+deploymentName+"\n")
}

func helmReleaseSecretWithManifest(t *testing.T, releaseName, namespace, manifest string) *corev1.Secret {
	t.Helper()
	record := map[string]any{
		"name":      releaseName,
		"namespace": namespace,
		"version":   1,
		"info":      map[string]any{"status": "deployed"},
		"chart":     map[string]any{"metadata": map[string]any{"name": "helm-lane", "version": "0.1.0"}},
		"manifest":  manifest,
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
	// Applications can be listed: an unresolved Helm trace tries the Argo
	// fallback, which must find none here.
	session.dynamic = newHelmViaArgoTestDynamicClient(object)
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
	if result == nil || !result.FullyManaged || len(result.Chain) != 3 || result.Object.Kind != "Deployment" || result.Object.Name != "v4-bare" {
		t.Fatalf("result = %#v; want the Deployment, traced through its chart and release", result)
	}
	release := result.Chain[1]
	if release.Kind != "Release" || release.Name != "v4-bare" || release.Namespace != "helm-lane" {
		t.Fatalf("release link = %#v; want Release helm-lane/v4-bare", release)
	}
}

func TestTraceReadsAHelmReleaseInTheNamespaceHelmRecorded(t *testing.T) {
	// A chart may install an object outside its release's namespace by
	// naming the namespace; the annotation says where the release lives.
	object := helmBareDeployment("api", "team-a", "platform", "platform-system")
	manifest := "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: team-a\n"
	session := helmAnnotationTraceSession(t, object, helmReleaseSecretWithManifest(t, "platform", "platform-system", manifest))

	observation, err := observeTrace(context.Background(), session, "Deployment", "api", "team-a", traceObservationOptions{})
	if err != nil {
		t.Fatalf("observeTrace() error = %v", err)
	}
	if got := observation.Result.Chain[1]; got.Name != "platform" || got.Namespace != "platform-system" {
		t.Fatalf("release link = %#v; want Release platform-system/platform", got)
	}
}

func TestTraceDoesNotCreditACopiedObjectToTheReleaseItsAnnotationsName(t *testing.T) {
	// A manifest copied with `kubectl get -o yaml` keeps Helm's annotations.
	// The release they name does not list the copy, so it is not the owner.
	object := helmBareDeployment("copied", "helm-lane", "v4-bare", "helm-lane")
	session := helmAnnotationTraceSession(t, object, helmReleaseSecretForTrace(t, "v4-bare", "helm-lane", "v4-bare"))

	observation, err := observeTrace(context.Background(), session, "Deployment", "copied", "helm-lane", traceObservationOptions{})
	if err != nil {
		t.Fatalf("observeTrace() error = %v; want an unresolved answer, not a failure", err)
	}
	result := observation.Result
	if result.FullyManaged {
		t.Fatalf("result claims the copy is managed: %#v", result)
	}
	for _, link := range result.Chain {
		if link.Kind == "Release" {
			t.Fatalf("chain names a release for an object it does not list: %#v", result.Chain)
		}
	}
	if !strings.Contains(result.Error, "does not list it") {
		t.Fatalf("result.Error = %q; want it to say the release does not list the object", result.Error)
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
		{"a chart name from helm.sh/chart is replaced by the release Helm recorded",
			agent.Ownership{Type: agent.OwnerHelm, Name: "nginx-1.2.3"},
			both,
			agent.Ownership{Type: agent.OwnerHelm, Name: "web", Namespace: "apps"}},
		{"both annotations name the release",
			agent.Ownership{Type: agent.OwnerHelm},
			both,
			agent.Ownership{Type: agent.OwnerHelm, Name: "web", Namespace: "apps"}},
		{"Helm's annotations replace a name from the instance label",
			agent.Ownership{Type: agent.OwnerHelm, Name: "from-label"},
			both,
			agent.Ownership{Type: agent.OwnerHelm, Name: "web", Namespace: "apps"}},
		{"without annotations a label name is left for trace to check",
			agent.Ownership{Type: agent.OwnerHelm, Name: "from-label"},
			nil,
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

func helmLabelledDeployment(name, namespace, instance string) *unstructured.Unstructured {
	object := helmBareDeployment(name, namespace, "", "")
	labels := object.Object["metadata"].(map[string]any)["labels"].(map[string]any)
	labels["app.kubernetes.io/instance"] = instance
	return object
}

func TestTraceChecksAReleaseNamedByTheInstanceLabel(t *testing.T) {
	// The label is the chart's, and Argo CD writes the same label to track
	// its own objects (#931): a release of that name is a candidate.
	listed := helmLabelledDeployment("web", "apps", "web")
	session := helmAnnotationTraceSession(t, listed, helmReleaseSecretForTrace(t, "web", "apps", "web"))
	observation, err := observeTrace(context.Background(), session, "Deployment", "web", "apps", traceObservationOptions{})
	if err != nil {
		t.Fatalf("observeTrace() error = %v", err)
	}
	if result := observation.Result; !result.FullyManaged || len(result.Chain) < 2 || result.Chain[1].Name != "web" {
		t.Fatalf("result = %#v; want the release that lists the object", result)
	}

	unlisted := helmLabelledDeployment("other", "apps", "web")
	session = helmAnnotationTraceSession(t, unlisted, helmReleaseSecretForTrace(t, "web", "apps", "web"))
	observation, err = observeTrace(context.Background(), session, "Deployment", "other", "apps", traceObservationOptions{})
	if err != nil {
		t.Fatalf("observeTrace() error = %v; want an unresolved answer", err)
	}
	result := observation.Result
	if result.FullyManaged || len(result.Chain) != 0 || !strings.Contains(result.Error, "does not list it") {
		t.Fatalf("result = %#v; a release that does not list the object must not be claimed", result)
	}
}
