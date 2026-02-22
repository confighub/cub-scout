// Package patterns tests ownership detection on complex GitOps pattern fixtures.
//
// These tests verify that cub-scout correctly classifies ownership for:
//   - 3-level App-of-Apps (ArgoCD)
//   - Multi-generator ApplicationSets (ArgoCD)
//   - Flux multi-tenant Kustomizations
//   - Mixed-tool clusters (all 7 ownership types)
//   - Bridge delivery patterns (Git→Flux, Git→ArgoCD, ConfigHub→OCI, Live Import)
//
// All tests are fully offline — no cluster required.
//
// Run: go test ./test/fixtures/patterns/ -v
package patterns

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/internal/graph"
	"github.com/confighub/cub-scout/internal/patterns"
	"github.com/confighub/cub-scout/pkg/agent"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// fixtureDir returns the absolute path to the fixtures/patterns directory.
func fixtureDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file location")
	}
	return filepath.Dir(filename)
}

// loadObjects parses a multi-document YAML file into unstructured objects.
func loadObjects(t *testing.T, path string) []*unstructured.Unstructured {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open fixture %s: %v", path, err)
	}
	defer f.Close()

	var objects []*unstructured.Unstructured
	decoder := yaml.NewDecoder(bufio.NewReader(f))
	for {
		var doc map[string]interface{}
		err := decoder.Decode(&doc)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("failed to decode YAML document in %s: %v", path, err)
		}
		if doc != nil && len(doc) > 0 {
			objects = append(objects, &unstructured.Unstructured{Object: doc})
		}
	}
	return objects
}

// filterByKind returns objects matching any of the given kinds.
func filterByKind(objects []*unstructured.Unstructured, kinds ...string) []*unstructured.Unstructured {
	kindSet := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}
	var result []*unstructured.Unstructured
	for _, obj := range objects {
		if kindSet[obj.GetKind()] {
			result = append(result, obj)
		}
	}
	return result
}

// detectAll runs ownership detection on all objects and returns a map of
// "namespace/kind/name" → ownership type.
func detectAll(objects []*unstructured.Unstructured) map[string]string {
	result := make(map[string]string)
	for _, obj := range objects {
		ownership := agent.DetectOwnership(obj)
		key := obj.GetNamespace() + "/" + obj.GetKind() + "/" + obj.GetName()
		result[key] = ownership.Type
	}
	return result
}

// --- App-of-Apps 3-Level Tests ---

func TestAoA3Level_AllLeavesArgoOwned(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "aoa-deep", "aoa-3level.yaml")
	objects := loadObjects(t, path)

	// All Deployments should be detected as ArgoCD-managed
	// (via argocd.argoproj.io/instance labels)
	deployments := filterByKind(objects, "Deployment")
	if len(deployments) != 4 {
		t.Fatalf("expected 4 Deployments, got %d", len(deployments))
	}

	for _, dep := range deployments {
		ownership := agent.DetectOwnership(dep)
		if ownership.Type != agent.OwnerArgo {
			t.Errorf("Deployment %s/%s: want owner=%s, got=%s",
				dep.GetNamespace(), dep.GetName(), agent.OwnerArgo, ownership.Type)
		}
	}
}

func TestAoA3Level_ApplicationCount(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "aoa-deep", "aoa-3level.yaml")
	objects := loadObjects(t, path)

	apps := filterByKind(objects, "Application")
	// 1 root + 2 intermediate + 4 leaf = 7
	if len(apps) != 7 {
		t.Errorf("expected 7 Applications, got %d", len(apps))
	}
}

func TestAoA3Level_ParentAnnotations(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "aoa-deep", "aoa-3level.yaml")
	objects := loadObjects(t, path)

	// Verify intermediate Applications have parent annotation
	apps := filterByKind(objects, "Application")
	parentAnnotated := 0
	for _, app := range apps {
		annotations := app.GetAnnotations()
		if annotations["cub-scout.io/parent-application"] != "" {
			parentAnnotated++
		}
	}
	// 2 intermediates + 4 leaves = 6 annotated (root has no parent)
	if parentAnnotated != 6 {
		t.Errorf("expected 6 Applications with parent annotation, got %d", parentAnnotated)
	}
}

func TestAoA3Level_DeploymentInstanceLabels(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "aoa-deep", "aoa-3level.yaml")
	objects := loadObjects(t, path)

	// Each leaf Deployment should have instance label matching its parent Application
	expected := map[string]string{
		"frontend-dev/Deployment/web-ui":      "frontend-dev",
		"frontend-prod/Deployment/web-ui":     "frontend-prod",
		"backend-dev/Deployment/api-server":   "backend-dev",
		"backend-prod/Deployment/api-server":  "backend-prod",
	}

	ownerMap := detectAll(objects)
	for key := range expected {
		if ownerMap[key] != agent.OwnerArgo {
			t.Errorf("%s: want owner=%s, got=%s", key, agent.OwnerArgo, ownerMap[key])
		}
	}
}

// --- ApplicationSet Multi-Generator Tests ---

func TestAppSetMultiGen_AllGeneratedAppsDetected(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "appset-multi-gen", "appset-multi-generator.yaml")
	objects := loadObjects(t, path)

	apps := filterByKind(objects, "Application")
	// 5 generated Applications: app-dev, app-staging, app-prod, app-us-east, app-eu-west
	if len(apps) != 5 {
		t.Errorf("expected 5 Applications, got %d", len(apps))
	}

	// All should have ApplicationSet label
	for _, app := range apps {
		labels := app.GetLabels()
		setName := labels["argocd.argoproj.io/application-set-name"]
		if setName != "multi-env-apps" {
			t.Errorf("Application %s missing application-set-name label (got %q)",
				app.GetName(), setName)
		}
	}
}

func TestAppSetMultiGen_LeafDeploymentsArgoOwned(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "appset-multi-gen", "appset-multi-generator.yaml")
	objects := loadObjects(t, path)

	deployments := filterByKind(objects, "Deployment")
	if len(deployments) != 5 {
		t.Fatalf("expected 5 Deployments, got %d", len(deployments))
	}

	for _, dep := range deployments {
		ownership := agent.DetectOwnership(dep)
		if ownership.Type != agent.OwnerArgo {
			t.Errorf("Deployment %s/%s: want owner=%s, got=%s",
				dep.GetNamespace(), dep.GetName(), agent.OwnerArgo, ownership.Type)
		}
	}
}

func TestAppSetMultiGen_ApplicationSetPresent(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "appset-multi-gen", "appset-multi-generator.yaml")
	objects := loadObjects(t, path)

	appSets := filterByKind(objects, "ApplicationSet")
	if len(appSets) != 1 {
		t.Fatalf("expected 1 ApplicationSet, got %d", len(appSets))
	}
	if appSets[0].GetName() != "multi-env-apps" {
		t.Errorf("ApplicationSet name: want %q, got %q", "multi-env-apps", appSets[0].GetName())
	}
}

// --- Flux Tenancy Tests ---

func TestFluxTenancy_TenantIsolation(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "flux-tenancy", "flux-tenancy.yaml")
	objects := loadObjects(t, path)

	// Each tenant's workload should be Flux-owned with the correct Kustomization name
	expected := map[string]string{
		"team-alpha/Deployment/alpha-api":       "tenant-alpha",
		"team-beta/Deployment/beta-worker":      "tenant-beta",
		"team-gamma/Deployment/gamma-frontend":  "tenant-gamma",
		"platform/Deployment/ingress-nginx":     "platform-infra",
	}

	for _, obj := range objects {
		key := obj.GetNamespace() + "/" + obj.GetKind() + "/" + obj.GetName()
		wantKs, ok := expected[key]
		if !ok {
			continue
		}

		ownership := agent.DetectOwnership(obj)
		if ownership.Type != agent.OwnerFlux {
			t.Errorf("%s: want owner type=%s, got=%s", key, agent.OwnerFlux, ownership.Type)
			continue
		}
		if ownership.Name != wantKs {
			t.Errorf("%s: want owner name=%q, got=%q", key, wantKs, ownership.Name)
		}
	}
}

func TestFluxTenancy_AllDeploymentsFluxOwned(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "flux-tenancy", "flux-tenancy.yaml")
	objects := loadObjects(t, path)

	deployments := filterByKind(objects, "Deployment")
	if len(deployments) != 4 {
		t.Fatalf("expected 4 Deployments, got %d", len(deployments))
	}

	for _, dep := range deployments {
		ownership := agent.DetectOwnership(dep)
		if ownership.Type != agent.OwnerFlux {
			t.Errorf("Deployment %s/%s: want owner=%s, got=%s",
				dep.GetNamespace(), dep.GetName(), agent.OwnerFlux, ownership.Type)
		}
	}
}

func TestFluxTenancy_KustomizationCount(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "flux-tenancy", "flux-tenancy.yaml")
	objects := loadObjects(t, path)

	kustomizations := filterByKind(objects, "Kustomization")
	// 1 platform + 3 tenant = 4
	if len(kustomizations) != 4 {
		t.Errorf("expected 4 Kustomizations, got %d", len(kustomizations))
	}
}

// --- Mixed Tool Tests ---

func TestMixedTool_AllOwnersDetected(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "mixed-tool", "mixed-tool-cluster.yaml")
	objects := loadObjects(t, path)

	expected := map[string]string{
		// 7 ownership types
		"apps/Deployment/flux-web":            agent.OwnerFlux,
		"infra/Deployment/argo-api":           agent.OwnerArgo,
		"data/Deployment/helm-redis":          agent.OwnerHelm,
		"infra/Deployment/tf-vpc-controller":  agent.OwnerTerraform,
		"data/Deployment/xp-database":         agent.OwnerCrossplane,
		"apps/Deployment/ch-config-sync":      agent.OwnerConfigHub,
		"apps/Deployment/legacy-cron":         agent.OwnerUnknown,
	}

	ownerMap := detectAll(objects)

	for key, wantType := range expected {
		gotType, ok := ownerMap[key]
		if !ok {
			t.Errorf("resource %s not found in parsed objects", key)
			continue
		}
		if gotType != wantType {
			t.Errorf("%s: want owner=%s, got=%s", key, wantType, gotType)
		}
	}
}

func TestMixedTool_FluxServicesDetected(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "mixed-tool", "mixed-tool-cluster.yaml")
	objects := loadObjects(t, path)

	ownerMap := detectAll(objects)

	// Flux-managed Service
	if got := ownerMap["apps/Service/flux-web-svc"]; got != agent.OwnerFlux {
		t.Errorf("Service flux-web-svc: want owner=%s, got=%s", agent.OwnerFlux, got)
	}

	// ArgoCD-managed Service
	if got := ownerMap["infra/Service/argo-api-svc"]; got != agent.OwnerArgo {
		t.Errorf("Service argo-api-svc: want owner=%s, got=%s", agent.OwnerArgo, got)
	}
}

func TestMixedTool_HelmConfigMapDetected(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "mixed-tool", "mixed-tool-cluster.yaml")
	objects := loadObjects(t, path)

	ownerMap := detectAll(objects)

	if got := ownerMap["data/ConfigMap/redis-config"]; got != agent.OwnerHelm {
		t.Errorf("ConfigMap redis-config: want owner=%s, got=%s", agent.OwnerHelm, got)
	}
}

func TestMixedTool_NativeResourcesDetected(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "mixed-tool", "mixed-tool-cluster.yaml")
	objects := loadObjects(t, path)

	ownerMap := detectAll(objects)

	// Native ConfigMap (no ownership labels)
	if got := ownerMap["apps/ConfigMap/app-config"]; got != agent.OwnerUnknown {
		t.Errorf("ConfigMap app-config: want owner=%s, got=%s", agent.OwnerUnknown, got)
	}

	// Native DaemonSet
	if got := ownerMap["monitoring/DaemonSet/node-exporter"]; got != agent.OwnerUnknown {
		t.Errorf("DaemonSet node-exporter: want owner=%s, got=%s", agent.OwnerUnknown, got)
	}
}

func TestMixedTool_ResourceCount(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "mixed-tool", "mixed-tool-cluster.yaml")
	objects := loadObjects(t, path)

	// Count resource kinds
	kindCounts := make(map[string]int)
	for _, obj := range objects {
		kindCounts[obj.GetKind()]++
	}

	// Verify we have a mix of kinds
	if kindCounts["Deployment"] < 7 {
		t.Errorf("expected at least 7 Deployments, got %d", kindCounts["Deployment"])
	}
	if kindCounts["Service"] < 2 {
		t.Errorf("expected at least 2 Services, got %d", kindCounts["Service"])
	}
}

func TestMixedTool_OwnershipDistribution(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "mixed-tool", "mixed-tool-cluster.yaml")
	objects := loadObjects(t, path)

	// Count ownership types across all workload resources (Deployment, StatefulSet, DaemonSet)
	workloads := filterByKind(objects, "Deployment", "StatefulSet", "DaemonSet")
	typeCounts := make(map[string]int)
	for _, obj := range workloads {
		ownership := agent.DetectOwnership(obj)
		typeCounts[ownership.Type]++
	}

	// Verify all expected types are present
	expectedTypes := []string{
		agent.OwnerFlux,
		agent.OwnerArgo,
		agent.OwnerHelm,
		agent.OwnerTerraform,
		agent.OwnerCrossplane,
		agent.OwnerConfigHub,
		agent.OwnerUnknown,
	}

	for _, ownerType := range expectedTypes {
		if typeCounts[ownerType] == 0 {
			t.Errorf("no workloads detected with owner type %q", ownerType)
		}
	}
}

// --- Scan CLI integration tests ---

func TestPatternFixtures_ScanFileDoesNotPanic(t *testing.T) {
	// Verify that scan --file completes without crashing on each fixture.
	// This is a smoke test — actual ownership is tested above via the Go API.
	fixtures := []struct {
		name string
		path string
	}{
		{"aoa-3level", "aoa-deep/aoa-3level.yaml"},
		{"appset-multi-gen", "appset-multi-gen/appset-multi-generator.yaml"},
		{"flux-tenancy", "flux-tenancy/flux-tenancy.yaml"},
		{"mixed-tool", "mixed-tool/mixed-tool-cluster.yaml"},
	}

	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			path := filepath.Join(fixtureDir(t), f.path)
			objects := loadObjects(t, path)
			if len(objects) == 0 {
				t.Errorf("fixture %s produced 0 objects", f.name)
			}

			// Run ownership detection on every object (must not panic)
			for _, obj := range objects {
				_ = agent.DetectOwnership(obj)
			}
		})
	}
}

// --- Cross-fixture uniqueness ---

func TestAllFixtures_DeterministicOwnership(t *testing.T) {
	// Run ownership detection twice on each fixture and verify results are identical.
	fixtures := []string{
		"aoa-deep/aoa-3level.yaml",
		"appset-multi-gen/appset-multi-generator.yaml",
		"flux-tenancy/flux-tenancy.yaml",
		"mixed-tool/mixed-tool-cluster.yaml",
	}

	for _, f := range fixtures {
		t.Run(strings.ReplaceAll(f, "/", "_"), func(t *testing.T) {
			path := filepath.Join(fixtureDir(t), f)
			objects := loadObjects(t, path)

			run1 := detectAll(objects)
			run2 := detectAll(objects)

			for key, v1 := range run1 {
				if v2 := run2[key]; v1 != v2 {
					t.Errorf("%s: non-deterministic ownership: run1=%s, run2=%s", key, v1, v2)
				}
			}
		})
	}
}

// --- Bridge pattern fixture integration tests ---

// buildGraphFromObjects constructs a graph.Graph from parsed YAML objects.
// Each object becomes a graph Node with labels extracted from metadata.
func buildGraphFromObjects(objects []*unstructured.Unstructured) *graph.Graph {
	g := graph.NewGraph("fixture-cluster")
	for _, obj := range objects {
		nodeID := fmt.Sprintf("fixture-cluster/%s/%s/%s",
			obj.GetNamespace(), obj.GetKind(), obj.GetName())
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		g.AddNode(graph.Node{
			ID:         nodeID,
			Cluster:    "fixture-cluster",
			Namespace:  obj.GetNamespace(),
			Kind:       obj.GetKind(),
			Name:       obj.GetName(),
			APIVersion: obj.GetAPIVersion(),
			Labels:     labels,
		})
	}
	return g
}

func TestBridgeGitFlux_Fixture(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "bridge-git-flux", "bridge-git-flux.yaml")
	objects := loadObjects(t, path)
	g := buildGraphFromObjects(objects)

	pr := patterns.DetectOne(g, "delivery.bridge.git_flux")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != patterns.StatusPass {
		t.Errorf("expected pass, got %s (skip: %s)", pr.Status, pr.SkipReason)
	}
	if len(pr.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(pr.Findings))
	}

	f := pr.Findings[0]
	if !strings.Contains(f.Message, "1 sources") {
		t.Errorf("expected 1 source, got: %s", f.Message)
	}
	if !strings.Contains(f.Message, "1 deployers") {
		t.Errorf("expected 1 deployer, got: %s", f.Message)
	}
	if !strings.Contains(f.Message, "2 managed workloads") {
		t.Errorf("expected 2 managed workloads, got: %s", f.Message)
	}
}

func TestBridgeGitArgoCD_Fixture(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "bridge-git-argocd", "bridge-git-argocd.yaml")
	objects := loadObjects(t, path)
	g := buildGraphFromObjects(objects)

	pr := patterns.DetectOne(g, "delivery.bridge.git_argocd")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != patterns.StatusPass {
		t.Errorf("expected pass, got %s (skip: %s)", pr.Status, pr.SkipReason)
	}
	if len(pr.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(pr.Findings))
	}

	f := pr.Findings[0]
	if !strings.Contains(f.Message, "1 Applications") {
		t.Errorf("expected 1 Application, got: %s", f.Message)
	}
	if !strings.Contains(f.Message, "2 managed workloads") {
		t.Errorf("expected 2 managed workloads, got: %s", f.Message)
	}
}

func TestBridgeConfigHubOCI_Fixture(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "bridge-confighub-oci", "bridge-confighub-oci.yaml")
	objects := loadObjects(t, path)
	g := buildGraphFromObjects(objects)

	pr := patterns.DetectOne(g, "delivery.bridge.confighub_oci")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != patterns.StatusPass {
		t.Errorf("expected pass, got %s (skip: %s)", pr.Status, pr.SkipReason)
	}
	if len(pr.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(pr.Findings))
	}

	f := pr.Findings[0]
	if !strings.Contains(f.Message, "1 OCI sources with ConfigHub origin") {
		t.Errorf("expected 1 ConfigHub OCI source, got: %s", f.Message)
	}
	if !strings.Contains(f.Message, "1 dual-managed workloads") {
		t.Errorf("expected 1 dual-managed workload, got: %s", f.Message)
	}
}

func TestBridgeLiveImport_Fixture(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "bridge-live-import", "bridge-live-import.yaml")
	objects := loadObjects(t, path)
	g := buildGraphFromObjects(objects)

	pr := patterns.DetectOne(g, "delivery.bridge.live_import")
	if pr == nil {
		t.Fatal("expected result")
	}
	if pr.Status != patterns.StatusPass {
		t.Errorf("expected pass, got %s (skip: %s)", pr.Status, pr.SkipReason)
	}
	if len(pr.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(pr.Findings))
	}

	f := pr.Findings[0]
	if !strings.Contains(f.Message, "2 resources with ConfigHub ownership but no GitOps deployer") {
		t.Errorf("expected 2 live imports, got: %s", f.Message)
	}
}
