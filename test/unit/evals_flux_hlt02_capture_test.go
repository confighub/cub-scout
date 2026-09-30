package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const fluxHLT02Dir = "../../evals/flux-ready-without-health"

func TestFluxHLT02CaptureManifestsDefineWaitFalseAndPinnedSource(t *testing.T) {
	var source struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name      string `yaml:"name"`
			Namespace string `yaml:"namespace"`
		} `yaml:"metadata"`
		Spec struct {
			URL string `yaml:"url"`
			Ref struct {
				Commit string `yaml:"commit"`
			} `yaml:"ref"`
		} `yaml:"spec"`
	}
	data, err := os.ReadFile(filepath.Join(fluxHLT02Dir, "gitrepository.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	if source.APIVersion != "source.toolkit.fluxcd.io/v1" || source.Kind != "GitRepository" || source.Metadata.Name != "scout-hlt02-source" || source.Spec.URL != "https://github.com/confighub/cub-scout" || source.Spec.Ref.Commit != "7732dde28be8cf8c42c096d94efbd8ce4a9d0a19" {
		t.Fatalf("unexpected pinned source manifest: %+v", source)
	}

	var ks struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec struct {
			Path             string `yaml:"path"`
			Wait             *bool  `yaml:"wait"`
			HealthChecks     []any  `yaml:"healthChecks"`
			HealthCheckExprs []any  `yaml:"healthCheckExprs"`
			TargetNamespace  string `yaml:"targetNamespace"`
			Patch            []struct {
				Patch string `yaml:"patch"`
			} `yaml:"patches"`
		} `yaml:"spec"`
	}
	data, err = os.ReadFile(filepath.Join(fluxHLT02Dir, "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &ks); err != nil {
		t.Fatal(err)
	}
	if ks.APIVersion != "kustomize.toolkit.fluxcd.io/v1" || ks.Kind != "Kustomization" || ks.Metadata.Name != "apps" || ks.Spec.Path != "./examples/combined-git-live/git-repo/apps/payment-worker/base" || ks.Spec.Wait == nil || *ks.Spec.Wait || len(ks.Spec.HealthChecks) != 0 || len(ks.Spec.HealthCheckExprs) != 0 || ks.Spec.TargetNamespace != "scout-hlt02" || len(ks.Spec.Patch) != 1 {
		t.Fatalf("manifest does not encode the HLT-02 conditions: %+v", ks)
	}
	if !strings.Contains(ks.Spec.Patch[0].Patch, "registry.k8s.io/pause:3.9") || !strings.Contains(ks.Spec.Patch[0].Patch, "/scout-fixture-intentionally-missing") {
		t.Fatal("fixture patch must use the pinned cached pause image and deterministic missing command")
	}
	readme, err := os.ReadFile(filepath.Join(fluxHLT02Dir, "README.md"))
	if err != nil || !strings.Contains(string(readme), "absence of `healthChecks` alone is insufficient") || !strings.Contains(string(readme), "not run yet") {
		t.Fatalf("README must preserve wait=true nuance and unrun status: %v", err)
	}
}

func TestFluxHLT02CaptureRefusesExistingOwnedKindCluster(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "calls.log")
	writeExecutable(t, filepath.Join(bin, "kind"), `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  version) echo 'kind v0.31.0 go1.24.2 darwin/arm64' ;;
  get) printf '%s\n' "$MOCK_CLUSTERS" ;;
  *) echo "$*" >> "$MOCK_CALL_LOG"; exit 90 ;;
esac
`)
	writeExecutable(t, filepath.Join(bin, "docker"), `#!/usr/bin/env bash
echo "$*" >> "$MOCK_CALL_LOG"
exit 0
`)
	writeExecutable(t, filepath.Join(bin, "flux"), `#!/usr/bin/env bash
if [[ "$1" == --version ]]; then echo 'flux version 2.8.6'; exit 0; fi
exit 90
`)
	for _, name := range []string{"kubectl", "jq"} {
		writeExecutable(t, filepath.Join(bin, name), "#!/usr/bin/env bash\nexit 90\n")
	}
	cmd := exec.Command("bash", filepath.Join(fluxHLT02Dir, "capture.sh"), "--execute", filepath.Join(t.TempDir(), "capture"))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "MOCK_CLUSTERS=scout-hlt02-ready-without-health", "MOCK_CALL_LOG="+logPath)
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "Refusing to reuse existing kind cluster") {
		t.Fatalf("capture should refuse its already-existing exact cluster name: err=%v output=%s", err, out)
	}
	log, _ := os.ReadFile(logPath)
	if strings.Contains(string(log), "create cluster") || strings.Contains(string(log), "delete cluster") {
		t.Fatalf("refusal must not create or delete a cluster: %s", log)
	}
}

func TestFluxHLT02CaptureRefusesUncachedPinnedNodeImage(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "calls.log")
	writeExecutable(t, filepath.Join(bin, "kind"), `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  version) echo 'kind v0.31.0 go1.24.2 darwin/arm64' ;;
  get) exit 0 ;;
  *) echo "$*" >> "$MOCK_CALL_LOG"; exit 90 ;;
esac
`)
	writeExecutable(t, filepath.Join(bin, "docker"), `#!/usr/bin/env bash
echo "$*" >> "$MOCK_CALL_LOG"
exit 1
`)
	writeExecutable(t, filepath.Join(bin, "flux"), `#!/usr/bin/env bash
if [[ "$1" == --version ]]; then echo 'flux version 2.8.6'; exit 0; fi
exit 90
`)
	for _, name := range []string{"kubectl", "jq"} {
		writeExecutable(t, filepath.Join(bin, name), "#!/usr/bin/env bash\nexit 90\n")
	}
	cmd := exec.Command("bash", filepath.Join(fluxHLT02Dir, "capture.sh"), "--execute", filepath.Join(t.TempDir(), "capture"))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "MOCK_CALL_LOG="+logPath)
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "not already cached locally") {
		t.Fatalf("capture should refuse an unavailable cached image: err=%v output=%s", err, out)
	}
	log, _ := os.ReadFile(logPath)
	if strings.Contains(string(log), "create cluster") || strings.Contains(string(log), "delete cluster") {
		t.Fatalf("cache guard must run before any kind lifecycle operation: %s", log)
	}
}
