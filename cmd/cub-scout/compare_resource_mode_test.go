package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestBuildCompareResourceResult_Standalone(t *testing.T) {
	restoreFlags := setCombinedFlagState(combinedFlagState{
		namespace: "prod",
		format:    "json",
	})
	defer restoreFlags()

	restoreLive := loadCompareLiveSnapshotFn
	loadCompareLiveSnapshotFn = func(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
		replicas := int64(3)
		return compareSideSummary{
			Source:     "cluster",
			APIVersion: "apps/v1",
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
			Replicas:   &replicas,
			Images:     []string{"ghcr.io/acme/api:v1"},
		}, nil
	}
	defer func() { loadCompareLiveSnapshotFn = restoreLive }()

	restoreConnected := compareConnectedFn
	compareConnectedFn = func() bool { return false }
	defer func() { compareConnectedFn = restoreConnected }()

	result, err := buildCompareResourceResult(context.Background(), "deploy/api", "prod")
	if err != nil {
		t.Fatalf("buildCompareResourceResult: %v", err)
	}
	if result.Resource != "Deployment/api" {
		t.Fatalf("result.Resource = %q, want Deployment/api", result.Resource)
	}
	if result.Mode != "live-only" {
		t.Fatalf("result.Mode = %q, want live-only", result.Mode)
	}
	if result.Connected {
		t.Fatal("expected result.Connected=false")
	}
	if result.Live.Replicas == nil || *result.Live.Replicas != 3 {
		t.Fatalf("result.Live.Replicas = %#v, want 3", result.Live.Replicas)
	}
	if !strings.Contains(strings.Join(result.Notes, "\n"), "Connect to ConfigHub") {
		t.Fatalf("expected upsell note in %v", result.Notes)
	}
}

func TestBuildCompareResourceResult_Connected(t *testing.T) {
	restoreLive := loadCompareLiveSnapshotFn
	loadCompareLiveSnapshotFn = func(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
		return compareSideSummary{
			Source:     "cluster",
			APIVersion: "apps/v1",
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
		}, nil
	}
	defer func() { loadCompareLiveSnapshotFn = restoreLive }()

	restoreConnected := compareConnectedFn
	compareConnectedFn = func() bool { return true }
	defer func() { compareConnectedFn = restoreConnected }()

	result, err := buildCompareResourceResult(context.Background(), "deployment/api", "prod")
	if err != nil {
		t.Fatalf("buildCompareResourceResult: %v", err)
	}
	if !result.Connected {
		t.Fatal("expected result.Connected=true")
	}
	if !strings.Contains(strings.Join(result.Notes, "\n"), "not linked to a ConfigHub unit") {
		t.Fatalf("expected linked-unit note in %v", result.Notes)
	}
}

func TestBuildCompareResourceResult_Connected_LoadsDryWetWhenLinked(t *testing.T) {
	restoreLive := loadCompareLiveSnapshotFn
	loadCompareLiveSnapshotFn = func(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
		replicas := int64(3)
		return compareSideSummary{
			Source:     "cluster",
			APIVersion: "apps/v1",
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
			UnitSlug:   "checkout-api",
			SpaceName:  "payments-prod",
			Replicas:   &replicas,
			Images:     []string{"ghcr.io/acme/api:v3"},
		}, nil
	}
	defer func() { loadCompareLiveSnapshotFn = restoreLive }()

	restoreConnected := compareConnectedFn
	compareConnectedFn = func() bool { return true }
	defer func() { compareConnectedFn = restoreConnected }()

	restoreDryWet := loadCompareDryWetSnapshotFn
	called := false
	loadCompareDryWetSnapshotFn = func(ctx context.Context, unitSlug, space string, target compareResourceRef) (compareDryWetResult, error) {
		called = true
		if unitSlug != "checkout-api" {
			t.Fatalf("unitSlug = %q, want checkout-api", unitSlug)
		}
		if space != "payments-prod" {
			t.Fatalf("space = %q, want payments-prod", space)
		}
		if target.Kind != "Deployment" || target.Name != "api" || target.Namespace != "prod" {
			t.Fatalf("unexpected target: %+v", target)
		}
		return compareDryWetResult{
			Dry: &compareSideSummary{
				Source:     "dry",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "api",
				Namespace:  "prod",
				Replicas:   int64Ptr(1),
				Images:     []string{"ghcr.io/acme/api:v1"},
			},
			Wet: &compareSideSummary{
				Source:     "wet",
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "api",
				Namespace:  "prod",
				Replicas:   int64Ptr(2),
				Images:     []string{"ghcr.io/acme/api:v2"},
			},
		}, nil
	}
	defer func() { loadCompareDryWetSnapshotFn = restoreDryWet }()

	result, err := buildCompareResourceResult(context.Background(), "deployment/api", "prod")
	if err != nil {
		t.Fatalf("buildCompareResourceResult: %v", err)
	}
	if !called {
		t.Fatal("expected DRY/WET loader to be called")
	}
	if result.Mode != "dry-wet-live" {
		t.Fatalf("result.Mode = %q, want dry-wet-live", result.Mode)
	}
	if result.Dry == nil || result.Wet == nil {
		t.Fatalf("expected dry+wet snapshots, got dry=%v wet=%v", result.Dry != nil, result.Wet != nil)
	}
	if len(result.Mismatches) < 2 {
		t.Fatalf("expected mismatch highlights, got %#v", result.Mismatches)
	}
}

func TestRunCombined_ResourceCompareRejectsGitFlags(t *testing.T) {
	restoreFlags := setCombinedFlagState(combinedFlagState{
		gitPath:   "/tmp/repo",
		namespace: "prod",
	})
	defer restoreFlags()

	err := runCombined(&cobra.Command{}, []string{"deploy/api"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cannot be combined with --git-*") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCombined_ResourceCompareJSONOutput(t *testing.T) {
	restoreFlags := setCombinedFlagState(combinedFlagState{
		namespace: "prod",
		format:    "json",
	})
	defer restoreFlags()

	restoreLive := loadCompareLiveSnapshotFn
	loadCompareLiveSnapshotFn = func(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
		return compareSideSummary{
			Source:     "cluster",
			APIVersion: "apps/v1",
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
		}, nil
	}
	defer func() { loadCompareLiveSnapshotFn = restoreLive }()

	restoreConnected := compareConnectedFn
	compareConnectedFn = func() bool { return false }
	defer func() { compareConnectedFn = restoreConnected }()

	restoreDryWet := loadCompareDryWetSnapshotFn
	loadCompareDryWetSnapshotFn = func(ctx context.Context, unitSlug, space string, target compareResourceRef) (compareDryWetResult, error) {
		return compareDryWetResult{}, nil
	}
	defer func() { loadCompareDryWetSnapshotFn = restoreDryWet }()

	out := captureStdout(t, func() {
		if err := runCombined(&cobra.Command{}, []string{"deploy/api"}); err != nil {
			t.Fatalf("runCombined: %v", err)
		}
	})

	var payload compareResourceResult
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse compare json: %v\n%s", err, out)
	}
	if payload.Live.Name != "api" {
		t.Fatalf("payload.Live.Name = %q, want api", payload.Live.Name)
	}
	if payload.Live.Namespace != "prod" {
		t.Fatalf("payload.Live.Namespace = %q, want prod", payload.Live.Namespace)
	}
}

func TestDecodeCompareUnitDataFromGetJSON(t *testing.T) {
	raw := `{"Unit":{"Data":"YXBpVmVyc2lvbjogYXBwcy92MQpraW5kOiBEZXBsb3ltZW50Cm1ldGFkYXRhOgogIG5hbWU6IGFwaQogIG5hbWVzcGFjZTogcHJvZAo="}}`
	got, err := decodeCompareUnitDataFromGetJSON(raw)
	if err != nil {
		t.Fatalf("decodeCompareUnitDataFromGetJSON: %v", err)
	}
	if !strings.Contains(got, "kind: Deployment") {
		t.Fatalf("decoded data missing expected manifest: %q", got)
	}
}

func TestExtractCompareSummaryFromManifestYAML(t *testing.T) {
	manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 3
  template:
    spec:
      containers:
        - name: api
          image: ghcr.io/acme/api:v2
---
apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: prod
`

	got, err := extractCompareSummaryFromManifestYAML("wet", manifest, compareResourceRef{
		Kind:      "Deployment",
		Name:      "api",
		Namespace: "prod",
	})
	if err != nil {
		t.Fatalf("extractCompareSummaryFromManifestYAML: %v", err)
	}
	if got == nil {
		t.Fatal("expected summary")
	}
	if got.Replicas == nil || *got.Replicas != 3 {
		t.Fatalf("replicas = %#v, want 3", got.Replicas)
	}
	if len(got.Images) != 1 || got.Images[0] != "ghcr.io/acme/api:v2" {
		t.Fatalf("images = %#v", got.Images)
	}
}

func TestResolveCompareConfigHubLink_FromArgoApplication(t *testing.T) {
	scheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme,
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Application",
				"metadata": map[string]interface{}{
					"name":      "demo-roundtrip-cubbychat-wet",
					"namespace": "argocd",
					"annotations": map[string]interface{}{
						"confighub.com/UnitSlug": "cubbychat-wet",
					},
				},
				"spec": map[string]interface{}{
					"source": map[string]interface{}{
						"repoURL": "oci://oci.hub.confighub.com:443/unit/demo-roundtrip/cubbychat-wet",
					},
				},
			},
		},
	)

	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      "frontend",
				"namespace": "default",
				"annotations": map[string]interface{}{
					"argocd.argoproj.io/tracking-id": "demo-roundtrip-cubbychat-wet:apps/Deployment:default/frontend",
				},
			},
		},
	}

	link, err := resolveCompareConfigHubLink(context.Background(), dynClient, resource)
	if err != nil {
		t.Fatalf("resolveCompareConfigHubLink() error = %v", err)
	}
	if link.UnitSlug != "cubbychat-wet" {
		t.Fatalf("UnitSlug = %q, want cubbychat-wet", link.UnitSlug)
	}
	if link.SpaceName != "demo-roundtrip" {
		t.Fatalf("SpaceName = %q, want demo-roundtrip", link.SpaceName)
	}
}

func TestParseCompareConfigHubUnitRepoURL(t *testing.T) {
	space, unit := parseCompareConfigHubUnitRepoURL("oci://oci.hub.confighub.com:443/unit/demo-roundtrip/cubbychat-wet")
	if space != "demo-roundtrip" {
		t.Fatalf("space = %q, want demo-roundtrip", space)
	}
	if unit != "cubbychat-wet" {
		t.Fatalf("unit = %q, want cubbychat-wet", unit)
	}

	space, unit = parseCompareConfigHubUnitRepoURL("oci://oci.hub.confighub.com:443/target/demo-roundtrip/argocd-worker")
	if space != "" || unit != "" {
		t.Fatalf("unexpected parse for target repoURL: space=%q unit=%q", space, unit)
	}
}

func TestLoadCompareDryWetSnapshots(t *testing.T) {
	restoreRun := runCompareCubCommand
	runCompareCubCommand = func(ctx context.Context, args []string) (string, error) {
		if reflect.DeepEqual(args, compareUnitGetArgs("checkout", "payments-prod")) {
			return `{"Space":{"Slug":"payments-prod","SpaceID":"sp-123"},"Unit":{"Slug":"checkout","UnitID":"u-123","HeadRevisionNum":9,"LiveRevisionNum":7,"LastAppliedRevisionNum":8,"Data":"YXBpVmVyc2lvbjogYXBwcy92MQpraW5kOiBEZXBsb3ltZW50Cm1ldGFkYXRhOgogIG5hbWU6IGFwaQogIG5hbWVzcGFjZTogcHJvZApzcGVjOgogIHJlcGxpY2FzOiAxCiAgdGVtcGxhdGU6CiAgICBzcGVjOgogICAgICBjb250YWluZXJzOgogICAgICAgIC0gbmFtZTogYXBpCiAgICAgICAgICBpbWFnZTogZ2hjci5pby9hY21lL2FwaTp2MQo="}}`, nil
		}
		if reflect.DeepEqual(args, compareUnitLivedataArgs("checkout", "payments-prod")) {
			return `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 2
  template:
    spec:
      containers:
        - name: api
          image: ghcr.io/acme/api:v2
`, nil
		}
		return "", fmt.Errorf("unexpected args: %v", args)
	}
	defer func() { runCompareCubCommand = restoreRun }()

	got, err := loadCompareDryWetSnapshots(context.Background(), "checkout", "payments-prod", compareResourceRef{
		Kind:      "Deployment",
		Name:      "api",
		Namespace: "prod",
	})
	if err != nil {
		t.Fatalf("loadCompareDryWetSnapshots: %v", err)
	}
	if got.Dry == nil || got.Wet == nil {
		t.Fatalf("expected dry+wet summaries, got dry=%v wet=%v", got.Dry != nil, got.Wet != nil)
	}
	if got.Dry.Replicas == nil || *got.Dry.Replicas != 1 {
		t.Fatalf("dry replicas = %#v, want 1", got.Dry.Replicas)
	}
	if got.Wet.Replicas == nil || *got.Wet.Replicas != 2 {
		t.Fatalf("wet replicas = %#v, want 2", got.Wet.Replicas)
	}
	if got.Dry.HeadRevisionNum != 9 || got.Dry.LiveRevisionNum != 7 || got.Dry.LastAppliedRev != 8 {
		t.Fatalf("dry revision facts = head:%d live:%d applied:%d, want 9/7/8", got.Dry.HeadRevisionNum, got.Dry.LiveRevisionNum, got.Dry.LastAppliedRev)
	}
	if got.Wet.UnitID != "u-123" || got.Wet.SpaceID != "sp-123" {
		t.Fatalf("wet unit identity = unit:%q space:%q, want u-123/sp-123", got.Wet.UnitID, got.Wet.SpaceID)
	}
}

func TestLoadCompareDryWetSnapshots_WetLookupFailure(t *testing.T) {
	restoreRun := runCompareCubCommand
	runCompareCubCommand = func(ctx context.Context, args []string) (string, error) {
		if reflect.DeepEqual(args, compareUnitGetArgs("checkout", "payments-prod")) {
			return `{"Unit":{"Data":"YXBpVmVyc2lvbjogYXBwcy92MQpraW5kOiBEZXBsb3ltZW50Cm1ldGFkYXRhOgogIG5hbWU6IGFwaQogIG5hbWVzcGFjZTogcHJvZAo="}}`, nil
		}
		if reflect.DeepEqual(args, compareUnitLivedataArgs("checkout", "payments-prod")) {
			return "", fmt.Errorf("permission denied")
		}
		return "", fmt.Errorf("unexpected args: %v", args)
	}
	defer func() { runCompareCubCommand = restoreRun }()

	got, err := loadCompareDryWetSnapshots(context.Background(), "checkout", "payments-prod", compareResourceRef{
		Kind:      "Deployment",
		Name:      "api",
		Namespace: "prod",
	})
	if err != nil {
		t.Fatalf("loadCompareDryWetSnapshots: %v", err)
	}
	if got.Dry == nil {
		t.Fatal("expected DRY summary when WET lookup fails")
	}
	if got.Wet != nil {
		t.Fatal("expected WET summary to be nil on lookup failure")
	}
	if len(got.Notes) == 0 || !strings.Contains(got.Notes[0], "WET lookup failed") {
		t.Fatalf("expected wet lookup failure note, got %#v", got.Notes)
	}
}

func TestRenderCompareResourceASCII_MismatchHighlights(t *testing.T) {
	result := compareResourceResult{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Mode:      "dry-wet-live",
		Connected: true,
		Dry: &compareSideSummary{
			Replicas: int64Ptr(1),
			Images:   []string{"ghcr.io/acme/api:v1"},
		},
		Wet: &compareSideSummary{
			Replicas: int64Ptr(2),
			Images:   []string{"ghcr.io/acme/api:v2"},
		},
		Live: compareSideSummary{
			Replicas: int64Ptr(3),
			Images:   []string{"ghcr.io/acme/api:v3"},
		},
		Mismatches: []compareFieldMismatch{
			{
				Field: "replicas",
				Dry:   "1",
				Wet:   "2",
				Live:  "3",
			},
			{
				Field: "images",
				Dry:   "ghcr.io/acme/api:v1",
				Wet:   "ghcr.io/acme/api:v2",
				Live:  "ghcr.io/acme/api:v3",
			},
		},
	}

	out := renderCompareResourceASCII(result)
	if !strings.Contains(out, "Diff Highlights") {
		t.Fatalf("expected diff highlights section in output:\n%s", out)
	}
	if !strings.Contains(out, "replicas: DRY=1 | WET=2 | LIVE=3") {
		t.Fatalf("expected replicas mismatch in output:\n%s", out)
	}
}

func TestRenderCompareResourceMarkdown_MismatchHighlights(t *testing.T) {
	result := compareResourceResult{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Mode:      "dry-wet-live",
		Connected: true,
		Mismatches: []compareFieldMismatch{
			{
				Field: "images",
				Dry:   "ghcr.io/acme/api:v1",
				Wet:   "ghcr.io/acme/api:v2",
				Live:  "ghcr.io/acme/api:v3",
			},
		},
	}

	out := renderCompareResourceMarkdown(result)
	if !strings.Contains(out, "### Mismatches") {
		t.Fatalf("expected mismatches section in output:\n%s", out)
	}
	if !strings.Contains(out, "| images | `ghcr.io/acme/api:v1` | `ghcr.io/acme/api:v2` | `ghcr.io/acme/api:v3` |") {
		t.Fatalf("expected images mismatch row in output:\n%s", out)
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}
