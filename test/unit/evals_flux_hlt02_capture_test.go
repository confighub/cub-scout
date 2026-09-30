package unit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const fluxHLT02Dir = "../../evals/flux-ready-without-health"

func requireFluxHLT02Python(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the offline HLT-02 capture checks")
	}
}

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
	if err != nil || !strings.Contains(string(readme), "absence of `healthChecks` alone is insufficient") {
		t.Fatalf("README must preserve wait=true nuance: %v", err)
	}
}

func TestFluxHLT02CaptureRefusesExistingOwnedKindCluster(t *testing.T) {
	requireFluxHLT02Python(t)
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
	requireFluxHLT02Python(t)
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

func TestFluxHLT02CaptureCleansPartialCreateWithPrivateKubeconfigOnly(t *testing.T) {
	requireFluxHLT02Python(t)
	bin := t.TempDir()
	state := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "kind.log")
	writeExecutable(t, filepath.Join(bin, "kind"), `#!/usr/bin/env bash
set -euo pipefail
printf 'KUBECONFIG=%s ARGS=%s\n' "${KUBECONFIG-}" "$*" >> "$MOCK_KIND_LOG"
case "$1" in
  version) echo 'kind v0.31.0 go1.24.2 darwin/arm64' ;;
  get)
    if [[ -f "$MOCK_STATE/deleted" ]]; then echo 'kind-shared-other'
    elif [[ -f "$MOCK_STATE/created" ]]; then printf '%s\n%s\n' 'scout-hlt02-ready-without-health' 'kind-shared-other'
    else echo 'kind-shared-other'; fi
    ;;
  create)
    touch "$MOCK_STATE/created"
    exit 7
    ;;
  delete)
    touch "$MOCK_STATE/deleted"
    ;;
  *) exit 90 ;;
esac
`)
	writeExecutable(t, filepath.Join(bin, "docker"), "#!/usr/bin/env bash\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "flux"), `#!/usr/bin/env bash
if [[ "$1" == --version ]]; then echo 'flux version 2.8.6'; exit 0; fi
if [[ "$1" == install && "$2" == --version=v2.8.6 && "$4" == --export ]]; then
  echo 'apiVersion: v1'; exit 0
fi
exit 90
`)
	for _, name := range []string{"kubectl", "jq"} {
		writeExecutable(t, filepath.Join(bin, name), "#!/usr/bin/env bash\nexit 90\n")
	}
	cmd := exec.Command("bash", filepath.Join(fluxHLT02Dir, "capture.sh"), "--execute", filepath.Join(t.TempDir(), "capture"))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "KUBECONFIG=/tmp/sentinel-user-config", "MOCK_KIND_LOG="+logPath, "MOCK_STATE="+state)
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("stubbed partial cluster creation should fail: %s", output)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logData)
	var privateConfig string
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "ARGS=create cluster") {
			for _, part := range strings.Fields(line) {
				if strings.HasPrefix(part, "KUBECONFIG=") {
					privateConfig = strings.TrimPrefix(part, "KUBECONFIG=")
				}
			}
			if privateConfig == "" || privateConfig == "/tmp/sentinel-user-config" || !strings.Contains(line, "--kubeconfig "+privateConfig) {
				t.Fatalf("create did not use its private kubeconfig in env and arg: %s", line)
			}
		}
		if strings.Contains(line, "ARGS=delete cluster") {
			if privateConfig == "" || !strings.Contains(line, "KUBECONFIG="+privateConfig) || !strings.Contains(line, "--kubeconfig "+privateConfig) {
				t.Fatalf("cleanup did not use the same private kubeconfig in env and arg: %s", line)
			}
			if strings.Contains(line, "kind-shared-other") {
				t.Fatalf("cleanup attempted to delete unrelated shared cluster: %s", line)
			}
		}
	}
	if !strings.Contains(log, "ARGS=delete cluster") {
		t.Fatalf("partial create was not cleaned up: %s", log)
	}
}

func TestFluxHLT02CaptureValidatorRequiresUnreadyPodOwnerChain(t *testing.T) {
	requireFluxHLT02Python(t)
	makeObjects := func() (map[string]any, map[string]any, map[string]any) {
		deployment := map[string]any{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "payment-worker", "namespace": "scout-hlt02", "uid": "deployment-uid", "generation": 3, "labels": map[string]any{
				"kustomize.toolkit.fluxcd.io/name": "apps", "kustomize.toolkit.fluxcd.io/namespace": "flux-system",
			}},
			"spec": map[string]any{"replicas": 1, "template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{
				"name": "worker", "image": "registry.k8s.io/pause:3.9", "command": []any{"/scout-fixture-intentionally-missing"},
			}}}}},
			"status": map[string]any{"observedGeneration": 3, "availableReplicas": 0, "unavailableReplicas": 1},
		}
		replicaSet := map[string]any{
			"apiVersion": "apps/v1", "kind": "ReplicaSet",
			"metadata": map[string]any{"name": "payment-worker-current", "namespace": "scout-hlt02", "uid": "replicaset-uid", "labels": map[string]any{
				"app.kubernetes.io/name": "payment-worker", "pod-template-hash": "template-hash",
			}, "ownerReferences": []any{map[string]any{"apiVersion": "apps/v1", "kind": "Deployment", "name": "payment-worker", "uid": "deployment-uid", "controller": true}}},
			"spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{
				"name": "worker", "image": "registry.k8s.io/pause:3.9", "command": []any{"/scout-fixture-intentionally-missing"},
			}}}}},
		}
		pod := map[string]any{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]any{"name": "payment-worker-current-xyz", "namespace": "scout-hlt02", "uid": "pod-uid", "labels": map[string]any{
				"app.kubernetes.io/name": "payment-worker", "pod-template-hash": "template-hash",
			}, "ownerReferences": []any{map[string]any{"apiVersion": "apps/v1", "kind": "ReplicaSet", "name": "payment-worker-current", "namespace": "scout-hlt02", "uid": "replicaset-uid", "controller": true}}},
			"spec": map[string]any{"containers": []any{map[string]any{"name": "worker", "image": "registry.k8s.io/pause:3.9", "command": []any{"/scout-fixture-intentionally-missing"}}}},
			"status": map[string]any{
				"conditions": []any{map[string]any{"type": "Ready", "status": "False"}},
				"containerStatuses": []any{map[string]any{"name": "worker", "state": map[string]any{"waiting": map[string]any{
					"reason": "CreateContainerError", "message": "exec: /scout-fixture-intentionally-missing: no such file or directory",
				}}}},
			},
		}
		return deployment, map[string]any{"apiVersion": "apps/v1", "kind": "ReplicaSetList", "items": []any{replicaSet}}, map[string]any{"apiVersion": "v1", "kind": "PodList", "items": []any{pod}}
	}
	write := func(t *testing.T, dir string, name string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(t *testing.T, deployment, replicasets, pods map[string]any) error {
		t.Helper()
		dir := t.TempDir()
		write(t, dir, "deployment.json", deployment)
		write(t, dir, "replicasets.json", replicasets)
		write(t, dir, "pods.json", pods)
		cmd := exec.Command("python3", filepath.Join(fluxHLT02Dir, "validate_capture.py"), dir)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("validator rejected capture: %w: %s", err, output)
		}
		return nil
	}

	deployment, replicaSets, pods := makeObjects()
	if err := run(t, deployment, replicaSets, pods); err != nil {
		t.Fatalf("valid Deployment→ReplicaSet→Pod chain rejected: %v", err)
	}

	for _, missing := range []string{"deployment-generation", "pod-uid", "pod-namespace", "replicaset-namespace", "pod-command", "bool-generation", "bool-replicas", "bool-available", "bool-unavailable", "bool-rs-replicas", "bool-exit-code"} {
		t.Run(missing, func(t *testing.T) {
			d, r, p := makeObjects()
			pod := p["items"].([]any)[0].(map[string]any)
			rs := r["items"].([]any)[0].(map[string]any)
			switch missing {
			case "deployment-generation":
				delete(d["metadata"].(map[string]any), "generation")
				delete(d["status"].(map[string]any), "observedGeneration")
			case "pod-uid":
				delete(pod["metadata"].(map[string]any), "uid")
			case "pod-namespace":
				pod["metadata"].(map[string]any)["namespace"] = "other"
			case "replicaset-namespace":
				rs["metadata"].(map[string]any)["namespace"] = "other"
			case "pod-command":
				delete(pod, "spec")
			case "bool-generation":
				d["metadata"].(map[string]any)["generation"] = 1
				d["status"].(map[string]any)["observedGeneration"] = true
			case "bool-replicas":
				d["spec"].(map[string]any)["replicas"] = true
			case "bool-available":
				d["status"].(map[string]any)["availableReplicas"] = false
			case "bool-unavailable":
				d["status"].(map[string]any)["unavailableReplicas"] = true
			case "bool-rs-replicas":
				rs["spec"].(map[string]any)["replicas"] = true
			case "bool-exit-code":
				pod["status"].(map[string]any)["containerStatuses"].([]any)[0].(map[string]any)["state"] = map[string]any{"terminated": map[string]any{"exitCode": true}}
			}
			if err := run(t, d, r, p); err == nil {
				t.Fatalf("accepted missing or mismatched evidence: %s", missing)
			}
		})
	}

	_, wrongRSOwner, wrongRSPods := makeObjects()
	wrongPod := wrongRSPods["items"].([]any)[0].(map[string]any)
	wrongPod["metadata"].(map[string]any)["ownerReferences"].([]any)[0].(map[string]any)["uid"] = "another-replicaset"
	if err := run(t, deployment, wrongRSOwner, wrongRSPods); err == nil {
		t.Fatal("Pod with unrelated ReplicaSet UID should be rejected")
	}

	_, directOwnerSets, directOwnerPods := makeObjects()
	directPod := directOwnerPods["items"].([]any)[0].(map[string]any)
	directPod["metadata"].(map[string]any)["ownerReferences"].([]any)[0].(map[string]any)["kind"] = "Deployment"
	directPod["metadata"].(map[string]any)["ownerReferences"].([]any)[0].(map[string]any)["uid"] = "deployment-uid"
	if err := run(t, deployment, directOwnerSets, directOwnerPods); err == nil {
		t.Fatal("Pod directly naming the Deployment rather than its ReplicaSet should be rejected")
	}

	_, healthySets, healthyPods := makeObjects()
	healthyPod := healthyPods["items"].([]any)[0].(map[string]any)
	healthyPod["status"].(map[string]any)["conditions"].([]any)[0].(map[string]any)["status"] = "True"
	if err := run(t, deployment, healthySets, healthyPods); err == nil {
		t.Fatal("Ready Pod should not satisfy the HLT-02 unavailable-workload capture")
	}
}
