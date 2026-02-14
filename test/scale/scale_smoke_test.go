// Package scale provides smoke tests for cub-scout at production-like scale.
//
// These tests generate 500 mixed-ownership Kubernetes resources as YAML fixtures
// and verify that scan --file completes without panic and within a reasonable time.
//
// No cluster required. Fully offline.
//
// Run: go test ./test/scale/ -v -timeout 60s
package scale

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/test/golden"
)

// generateScaleFixture creates a YAML file with n resources using mixed ownership.
// Ownership distribution:
//   - 35% Flux (Kustomization labels)
//   - 25% ArgoCD (instance label)
//   - 15% Helm (managed-by label)
//   - 5%  Terraform (run-id annotation)
//   - 5%  Crossplane (claim-name label)
//   - 5%  ConfigHub (UnitSlug label)
//   - 10% Native (no ownership labels)
func generateScaleFixture(t *testing.T, n int) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "scale-fixture.yaml")

	var sb strings.Builder
	namespaces := []string{"prod", "staging", "dev", "monitoring", "kube-system",
		"flux-system", "argocd", "cert-manager", "ingress-nginx", "crossplane-system",
		"team-alpha", "team-beta", "team-gamma", "team-delta", "platform",
		"data", "api", "frontend", "backend", "batch"}

	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteString("---\n")
		}

		ns := namespaces[i%len(namespaces)]
		name := fmt.Sprintf("workload-%04d", i)

		// Vary the kind
		kind := "Deployment"
		apiVersion := "apps/v1"
		switch i % 10 {
		case 0:
			kind = "StatefulSet"
		case 1:
			kind = "DaemonSet"
		case 2:
			kind = "Service"
			apiVersion = "v1"
		case 3:
			kind = "ConfigMap"
			apiVersion = "v1"
		case 4:
			kind = "Secret"
			apiVersion = "v1"
		case 5:
			kind = "CronJob"
			apiVersion = "batch/v1"
		}

		// Determine ownership based on distribution
		var labels, annotations string
		switch {
		case i%20 < 7: // 35% Flux
			labels = fmt.Sprintf(`    app.kubernetes.io/name: %s
    kustomize.toolkit.fluxcd.io/name: ks-%s
    kustomize.toolkit.fluxcd.io/namespace: flux-system`, name, ns)
			annotations = ""
		case i%20 < 12: // 25% ArgoCD
			labels = fmt.Sprintf(`    app.kubernetes.io/name: %s
    argocd.argoproj.io/instance: app-%s`, name, ns)
			annotations = ""
		case i%20 < 15: // 15% Helm
			labels = fmt.Sprintf(`    app.kubernetes.io/name: %s
    app.kubernetes.io/managed-by: Helm
    helm.sh/chart: %s-1.0.0`, name, name)
			annotations = fmt.Sprintf(`    meta.helm.sh/release-name: %s
    meta.helm.sh/release-namespace: %s`, name, ns)
		case i%20 < 16: // 5% Terraform
			labels = fmt.Sprintf(`    app.kubernetes.io/name: %s`, name)
			annotations = fmt.Sprintf(`    app.terraform.io/run-id: run-%04d`, i)
		case i%20 < 17: // 5% Crossplane
			labels = fmt.Sprintf(`    app.kubernetes.io/name: %s
    crossplane.io/claim-name: claim-%s
    crossplane.io/claim-namespace: %s`, name, name, ns)
			annotations = ""
		case i%20 < 18: // 5% ConfigHub
			labels = fmt.Sprintf(`    app.kubernetes.io/name: %s
    confighub.com/UnitSlug: unit-%s`, name, ns)
			annotations = ""
		default: // 10% Native
			labels = fmt.Sprintf(`    app.kubernetes.io/name: %s`, name)
			annotations = ""
		}

		sb.WriteString(fmt.Sprintf(`apiVersion: %s
kind: %s
metadata:
  name: %s
  namespace: %s
  labels:
%s
`, apiVersion, kind, name, ns, labels))

		if annotations != "" {
			sb.WriteString(fmt.Sprintf(`  annotations:
%s
`, annotations))
		}

		// Add a spec with containers for workload kinds
		if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" {
			sb.WriteString(fmt.Sprintf(`spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  template:
    metadata:
      labels:
        app.kubernetes.io/name: %s
    spec:
      containers:
        - name: app
          image: nginx:1.25
          ports:
            - containerPort: 8080
`, name, name))
		}
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("failed to write scale fixture: %v", err)
	}

	return path
}

// TestScaleScanFile_500Resources verifies that scan --file completes on 500
// mixed-ownership resources without panic and within 30 seconds.
func TestScaleScanFile_500Resources(t *testing.T) {
	fixturePath := generateScaleFixture(t, 500)

	start := time.Now()
	result := golden.RunCubScout(t, "scan", "--file", fixturePath)
	elapsed := time.Since(start)

	// Must not crash (exit code 0 = clean, 1 = findings — both acceptable)
	if result.ExitCode != 0 && result.ExitCode != 1 {
		t.Errorf("unexpected exit code %d (want 0 or 1)\nstderr: %s", result.ExitCode, result.Stderr)
	}

	// Must produce output
	if len(result.Stdout) == 0 {
		t.Error("scan produced no output for 500 resources")
	}

	// Must complete within 30 seconds
	if elapsed > 30*time.Second {
		t.Errorf("scan took %v (want < 30s) for 500 resources", elapsed)
	}

	t.Logf("scan --file completed in %v for 500 resources (exit=%d, output=%d bytes)",
		elapsed, result.ExitCode, len(result.Stdout))
}

// TestScaleScanFile_500Resources_JSON verifies JSON output at scale.
func TestScaleScanFile_500Resources_JSON(t *testing.T) {
	fixturePath := generateScaleFixture(t, 500)

	start := time.Now()
	result := golden.RunCubScout(t, "scan", "--file", fixturePath, "--json")
	elapsed := time.Since(start)

	if result.ExitCode != 0 && result.ExitCode != 1 {
		t.Errorf("unexpected exit code %d (want 0 or 1)\nstderr: %s", result.ExitCode, result.Stderr)
	}

	// JSON output must contain valid structure markers
	if !strings.Contains(result.Stdout, "{") && !strings.Contains(result.Stdout, "[") {
		t.Error("JSON output does not contain expected structure")
	}

	if elapsed > 30*time.Second {
		t.Errorf("scan --json took %v (want < 30s) for 500 resources", elapsed)
	}

	t.Logf("scan --file --json completed in %v for 500 resources (exit=%d, output=%d bytes)",
		elapsed, result.ExitCode, len(result.Stdout))
}

// TestScaleScanFile_200Resources verifies scan at a more moderate scale.
func TestScaleScanFile_200Resources(t *testing.T) {
	fixturePath := generateScaleFixture(t, 200)

	start := time.Now()
	result := golden.RunCubScout(t, "scan", "--file", fixturePath)
	elapsed := time.Since(start)

	if result.ExitCode != 0 && result.ExitCode != 1 {
		t.Errorf("unexpected exit code %d (want 0 or 1)\nstderr: %s", result.ExitCode, result.Stderr)
	}

	if elapsed > 15*time.Second {
		t.Errorf("scan took %v (want < 15s) for 200 resources", elapsed)
	}

	t.Logf("scan --file completed in %v for 200 resources (exit=%d, output=%d bytes)",
		elapsed, result.ExitCode, len(result.Stdout))
}
