package agent

import (
	"context"
	"strings"
	"testing"
)

func helmReleaseForObjectTracer(t *testing.T, releaseName, releaseNamespace, manifest string) *HelmTracer {
	t.Helper()
	release := &helmRelease{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Version:   1,
		Info:      helmReleaseInfo{Status: "deployed"},
		Manifest:  manifest,
	}
	secret := helmSecret(encodeRelease(t, release), releaseName, release.Version)
	secret.Namespace = releaseNamespace
	return helmManifestIdentityTracer(secret)
}

const deploymentWithoutNamespace = "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n"

func TestTraceReleaseForObjectClaimsOnlyAnObjectTheManifestLists(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name               string
		releaseNamespace   string
		manifest           string
		kind, object, ns   string
		wantManaged        bool
		wantErrorSubstring string
	}{
		{"no namespace in the manifest: the release's namespace", "apps", deploymentWithoutNamespace, "Deployment", "web", "apps", true, ""},
		{"no namespace in the manifest, object elsewhere", "apps", deploymentWithoutNamespace, "Deployment", "web", "team-a", false, "does not list it"},
		{"explicit namespace in the manifest", "platform", "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: team-a\n", "Deployment", "web", "team-a", true, ""},
		{"another name", "apps", deploymentWithoutNamespace, "Deployment", "copied", "apps", false, "does not list it"},
		{"another kind", "apps", deploymentWithoutNamespace, "StatefulSet", "web", "apps", false, "does not list it"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tracer := helmReleaseForObjectTracer(t, "web-release", tc.releaseNamespace, tc.manifest)
			result, err := tracer.TraceReleaseForObject(ctx, "web-release", tc.releaseNamespace, tc.kind, tc.object, tc.ns)
			if err != nil {
				t.Fatalf("TraceReleaseForObject() error = %v", err)
			}
			if result.FullyManaged != tc.wantManaged {
				t.Fatalf("FullyManaged = %v, want %v (%#v)", result.FullyManaged, tc.wantManaged, result)
			}
			if tc.wantManaged {
				if result.Object.Kind != tc.kind || result.Object.Name != tc.object || len(result.Chain) < 2 || result.Chain[1].Name != "web-release" {
					t.Fatalf("result = %#v; want the object and its release", result)
				}
				return
			}
			if len(result.Chain) != 0 || !strings.Contains(result.Error, tc.wantErrorSubstring) {
				t.Fatalf("result = %#v; want no chain and an error naming why", result)
			}
			// Helm-via-Argo fallback keys on this phrase.
			if !strings.Contains(strings.ToLower(result.Error), "no helm release found managing this resource") {
				t.Fatalf("error %q no longer matches the Argo fallback", result.Error)
			}
		})
	}
}

func TestTraceReleaseForObjectMissingReleaseAndAmbiguity(t *testing.T) {
	ctx := context.Background()
	tracer := helmReleaseForObjectTracer(t, "web-release", "apps", deploymentWithoutNamespace)
	result, err := tracer.TraceReleaseForObject(ctx, "gone", "apps", "Deployment", "web", "apps")
	if err != nil || result.FullyManaged || !strings.Contains(result.Error, "not found") {
		t.Fatalf("missing release: result = %#v, err = %v; want an unresolved not-found answer", result, err)
	}

	twice := deploymentWithoutNamespace + "---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: web\n  namespace: apps\n"
	tracer = helmReleaseForObjectTracer(t, "web-release", "apps", twice)
	if result, err := tracer.TraceReleaseForObject(ctx, "web-release", "apps", "Deployment", "web", "apps"); err == nil {
		t.Fatalf("two documents for one object: result = %#v; want an ambiguity error", result)
	}

	for _, args := range [][5]string{{"", "apps", "Deployment", "web", "apps"}, {"web-release", "", "Deployment", "web", "apps"}, {"web-release", "apps", "Deployment", "web", ""}} {
		if _, err := tracer.TraceReleaseForObject(ctx, args[0], args[1], args[2], args[3], args[4]); err == nil {
			t.Fatalf("TraceReleaseForObject(%q) accepted an incomplete identity", args)
		}
	}
}

func TestTraceReleaseForObjectFindsAnObjectInAHookManifest(t *testing.T) {
	// Helm keeps hooks out of the release manifest, each with its own.
	release := &helmRelease{
		Name:      "web-release",
		Namespace: "apps",
		Version:   1,
		Info:      helmReleaseInfo{Status: "deployed"},
		Manifest:  deploymentWithoutNamespace,
		Hooks: []helmReleaseHook{
			{Manifest: "apiVersion: batch/v1\nkind: Job\nmetadata:\n  name: web-migrate\n  annotations:\n    helm.sh/hook: pre-install\n"},
		},
	}
	secret := helmSecret(encodeRelease(t, release), release.Name, release.Version)
	secret.Namespace = release.Namespace
	tracer := helmManifestIdentityTracer(secret)

	result, err := tracer.TraceReleaseForObject(context.Background(), "web-release", "apps", "Job", "web-migrate", "apps")
	if err != nil || !result.FullyManaged {
		t.Fatalf("hook Job: result = %#v, err = %v; want it claimed by the release that lists it as a hook", result, err)
	}
	result, err = tracer.TraceReleaseForObject(context.Background(), "web-release", "apps", "Job", "other", "apps")
	if err != nil || result.FullyManaged {
		t.Fatalf("unlisted Job: result = %#v, err = %v; want no claim", result, err)
	}
}
