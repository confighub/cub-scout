package unit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestArgoImportDemo_AIFirstBundleExists(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "AI_START_HERE.md"),
		filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "prompts.md"),
		filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "contracts.md"),
		filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "setup.sh"),
		filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "verify.sh"),
		filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "cleanup.sh"),
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
		if strings.HasSuffix(path, ".sh") && runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
			t.Fatalf("expected %s to be executable", path)
		}
	}
}

func TestArgoImportSetupScript_ExplainJSONContract(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "setup.sh")

	cmd := exec.Command("bash", scriptPath, "--explain-json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("setup.sh --explain-json failed: %v\n%s", err, string(out))
	}

	var result struct {
		Example             string   `json:"example"`
		Entrypoint          string   `json:"entrypoint"`
		Mutates             bool     `json:"mutates"`
		ClusterName         string   `json:"clusterName"`
		ConfigHubSpace      string   `json:"configHubSpace"`
		KeepsClusterRunning bool     `json:"keepsClusterRunning"`
		ExecutionCommand    string   `json:"executionCommand"`
		EvidenceSurfaces    []string `json:"evidenceSurfaces"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("setup.sh --explain-json output is not valid JSON: %v\n%s", err, string(out))
	}

	if result.Example != "argo-import-confighub-demo" {
		t.Fatalf("example = %q, want argo-import-confighub-demo", result.Example)
	}
	if result.Entrypoint != "./setup.sh" {
		t.Fatalf("entrypoint = %q, want ./setup.sh", result.Entrypoint)
	}
	if result.Mutates {
		t.Fatal("mutates = true, want false for explain-json")
	}
	if result.ClusterName != "argo-import-demo" {
		t.Fatalf("clusterName = %q, want argo-import-demo", result.ClusterName)
	}
	if result.ConfigHubSpace != "argo-import-demo" {
		t.Fatalf("configHubSpace = %q, want argo-import-demo", result.ConfigHubSpace)
	}
	if !result.KeepsClusterRunning {
		t.Fatal("keepsClusterRunning = false, want true")
	}
	if !strings.Contains(result.ExecutionCommand, "./demo.sh --keep") {
		t.Fatalf("executionCommand = %q, want delegated demo.sh --keep path", result.ExecutionCommand)
	}
	if len(result.EvidenceSurfaces) != 3 {
		t.Fatalf("evidenceSurfaces length = %d, want 3", len(result.EvidenceSurfaces))
	}
}

func TestArgoImportVerifyScript_SucceedsWithMockedEvidenceSurfaces(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "verify.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("verify script missing: %v", err)
	}

	tmpDir := t.TempDir()
	mockBinDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(mockBinDir, 0o755); err != nil {
		t.Fatalf("mkdir mock bin: %v", err)
	}

	workerDir := filepath.Join(tmpDir, "cub-scout-demo-workers")
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("mkdir worker dir: %v", err)
	}
	pidFile := filepath.Join(workerDir, "argo-import-demo-discovery-worker.pid")
	if err := os.WriteFile(pidFile, []byte("12345\n"), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	workerJSON := filepath.Join(tmpDir, "workers.json")
	targetJSON := filepath.Join(tmpDir, "targets.json")
	unitJSON := filepath.Join(tmpDir, "units.json")
	importJSON := filepath.Join(tmpDir, "import.json")

	mustWriteArgoDemo(t, workerJSON, `[{"Condition":"Ready","Name":"demo-worker","Cluster":"kind-argo-import-demo"}]`)
	mustWriteArgoDemo(t, targetJSON, `[
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-kubernetes","ProviderType":"Kubernetes","ToolchainType":"Kubernetes"}},
	  {"BridgeWorker":{"Slug":"demo-worker"},"Target":{"Slug":"demo-argocdrenderer","ProviderType":"Renderer","ToolchainType":"argocdrenderer"}}
	]`)
	mustWriteArgoDemo(t, unitJSON, `[
	  {"Unit":{"Slug":"demo-app-dry"}},
	  {"Unit":{"Slug":"demo-app-wet"}}
	]`)
	mustWriteArgoDemo(t, importJSON, `{"workloads":[{"name":"api","connected":true},{"name":"worker","connected":true}]}`)

	writeExecutableArgoDemo(t, filepath.Join(mockBinDir, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *"get nodes"*)
    exit 0
    ;;
  *"get namespace argocd"*)
    exit 0
    ;;
  *"get application helm-guestbook -n argocd"*)
    exit 0
    ;;
  *"get application kustomize-guestbook -n argocd"*)
    exit 0
    ;;
  *"get application myapp-dev -n argocd"*)
    exit 0
    ;;
  *"get application myapp-staging -n argocd"*)
    exit 0
    ;;
  *"get application myapp-prod -n argocd"*)
    exit 0
    ;;
  *"get applications -n argocd"*)
    echo "NAME                 SYNCED"
    echo "helm-guestbook       True"
    echo "kustomize-guestbook  True"
    exit 0
    ;;
esac
echo "unexpected kubectl args: $*" >&2
exit 2
`)

	writeExecutableArgoDemo(t, filepath.Join(mockBinDir, "cub"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "auth" && "$2" == "get-token" ]]; then
  echo "token"
  exit 0
fi
if [[ "$1" == "worker" && "$2" == "list" && "$*" == *"--json"* ]]; then
  cat "$MOCK_WORKER_JSON"
  exit 0
fi
if [[ "$1" == "target" && "$2" == "list" ]]; then
  if [[ "$*" == *"--json"* ]]; then
    cat "$MOCK_TARGET_JSON"
  else
    echo "demo-kubernetes Kubernetes"
    echo "demo-argocdrenderer argocdrenderer"
  fi
  exit 0
fi
if [[ "$1" == "unit" && "$2" == "list" ]]; then
  if [[ "$*" == *"--json"* ]]; then
    cat "$MOCK_UNIT_JSON"
  else
    echo "helm-guestbook"
    echo "helm-guestbook-rendered"
  fi
  exit 0
fi
echo "unexpected cub args: $*" >&2
exit 2
`)

	writeExecutableArgoDemo(t, filepath.Join(mockBinDir, "cub-scout"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "import" && "$2" == "--dry-run" && "$3" == "--json" ]]; then
  cat "$MOCK_IMPORT_JSON"
  exit 0
fi
if [[ "$1" == "gitops" && "$2" == "status" ]]; then
  echo "GITOPS STATUS"
  exit 0
fi
if [[ "$1" == "map" && "$2" == "list" ]]; then
  echo "NAMESPACE KIND NAME OWNER"
  exit 0
fi
if [[ "$1" == "scan" && "$2" == "--state" && "$3" == "--json" ]]; then
  cat "$MOCK_SCAN_JSON"
  exit 0
fi
echo "unexpected cub-scout args: $*" >&2
exit 2
`)

	scanJSON := filepath.Join(tmpDir, "scan.json")
	mustWriteArgoDemo(t, scanJSON, `{
	  "state": {
	    "findings": [],
	    "runtimeFindings": [
	      {
	        "ccveId": "CCVE-2025-0690",
	        "namespace": "myapp-dev",
	        "kind": "Pod",
	        "name": "api-12345"
	      }
	    ],
	    "summary": {
	      "applicationStuck": 0,
	      "runtimeFailures": 1,
	      "total": 0
	    }
	  }
	}`)

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"TMPDIR="+tmpDir,
		"MOCK_WORKER_JSON="+workerJSON,
		"MOCK_TARGET_JSON="+targetJSON,
		"MOCK_UNIT_JSON="+unitJSON,
		"MOCK_IMPORT_JSON="+importJSON,
		"MOCK_SCAN_JSON="+scanJSON,
		"CUB_SCOUT_BIN="+filepath.Join(mockBinDir, "cub-scout"),
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify.sh failed: %v\n%s", err, string(out))
	}

	output := string(out)
	for _, needle := range []string{
		"Checking cluster connectivity",
		"Checking ConfigHub evidence",
		"PASS connected demo readiness",
		"Checking cub-scout gitops status",
		"Checking cub-scout map list",
		"Checking cub-scout scan evidence",
		"scan_runtime_findings=1",
		"scan_sample=CCVE-2025-0690 myapp-dev/Pod/api-12345",
		"Verification completed.",
		"three evidence surfaces",
	} {
		if !strings.Contains(output, needle) {
			t.Fatalf("verify output missing %q:\n%s", needle, output)
		}
	}
}

func TestArgoImportVerifyScript_FailsWhenScanEvidenceIsEmpty(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "examples", "argo-import-confighub-demo", "verify.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("verify script missing: %v", err)
	}

	tmpDir := t.TempDir()
	mockBinDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(mockBinDir, 0o755); err != nil {
		t.Fatalf("mkdir mock bin: %v", err)
	}

	writeExecutableArgoDemo(t, filepath.Join(mockBinDir, "kubectl"), `#!/usr/bin/env bash
set -euo pipefail
case "$*" in
  *"get nodes"*)
    exit 0
    ;;
  *"get namespace argocd"*)
    exit 0
    ;;
  *"get application helm-guestbook -n argocd"*)
    exit 0
    ;;
  *"get application kustomize-guestbook -n argocd"*)
    exit 0
    ;;
  *"get application myapp-dev -n argocd"*)
    exit 0
    ;;
  *"get application myapp-staging -n argocd"*)
    exit 0
    ;;
  *"get application myapp-prod -n argocd"*)
    exit 0
    ;;
  *"get applications -n argocd"*)
    echo "NAME                 SYNCED"
    echo "helm-guestbook       True"
    exit 0
    ;;
esac
echo "unexpected kubectl args: $*" >&2
exit 2
`)

	writeExecutableArgoDemo(t, filepath.Join(mockBinDir, "cub-scout"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" == "gitops" && "$2" == "status" ]]; then
  echo "GITOPS STATUS"
  exit 0
fi
if [[ "$1" == "map" && "$2" == "list" ]]; then
  echo "NAMESPACE KIND NAME OWNER"
  exit 0
fi
if [[ "$1" == "scan" && "$2" == "--state" && "$3" == "--json" ]]; then
  cat "$MOCK_SCAN_JSON"
  exit 0
fi
echo "unexpected cub-scout args: $*" >&2
exit 2
`)

	scanJSON := filepath.Join(tmpDir, "scan-empty.json")
	mustWriteArgoDemo(t, scanJSON, `{
	  "state": {
	    "findings": [],
	    "runtimeFindings": [],
	    "summary": {
	      "applicationStuck": 0,
	      "runtimeFailures": 0,
	      "total": 0
	    }
	  }
	}`)

	cmd := exec.Command("bash", scriptPath)
	cmd.Env = append(os.Environ(),
		"TMPDIR="+tmpDir,
		"MOCK_SCAN_JSON="+scanJSON,
		"CUB_SCOUT_BIN="+filepath.Join(mockBinDir, "cub-scout"),
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("verify.sh unexpectedly succeeded:\n%s", string(out))
	}

	output := string(out)
	if !strings.Contains(output, "cub-scout scan produced no findings or runtimeFindings") {
		t.Fatalf("verify output missing empty scan failure:\n%s", output)
	}
}

func writeExecutableArgoDemo(t *testing.T, path, content string) {
	t.Helper()
	mustWriteArgoDemo(t, path, content)
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}

func mustWriteArgoDemo(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
