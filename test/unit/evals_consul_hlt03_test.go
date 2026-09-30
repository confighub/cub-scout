// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const consulHLT03Dir = "consul-ingress-residue"

func TestConsulHLT03PinnedReceiptAndChildCapture(t *testing.T) {
	root := filepath.Join("..", "..", "evals", consulHLT03Dir)
	receiptBytes, err := os.ReadFile(filepath.Join(root, "fixtures", "receipt.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	childBytes, err := os.ReadFile(filepath.Join(root, "fixtures", "argocd-child.json"))
	if err != nil {
		t.Fatal(err)
	}
	provenanceBytes, err := os.ReadFile(filepath.Join(root, "fixtures", "source-provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	for name, gotWant := range map[string][2]string{
		"receipt":    {sha256Hex(receiptBytes), "521b08ae448f4d6503123ddef0263944887ced4204debab457677829c43fba15"},
		"child JSON": {sha256Hex(childBytes), "adf89697a41493e7399ff08b0e777a97e14d691b377d2fdddff2fa4faa27a078"},
	} {
		if gotWant[0] != gotWant[1] {
			t.Errorf("%s SHA-256 = %s, want pinned public source %s", name, gotWant[0], gotWant[1])
		}
	}

	var receipt struct {
		Kind string `yaml:"kind"`
		Spec struct {
			Result     string `yaml:"result"`
			ObservedAt string `yaml:"observedAt"`
			Legs       struct {
				RegularHelm struct {
					Result  string `yaml:"result"`
					Runtime struct {
						Result   string   `yaml:"result"`
						NotReady []string `yaml:"notReady"`
					} `yaml:"runtime"`
				} `yaml:"regularHelm"`
				KubectlApply struct {
					Result  string `yaml:"result"`
					Runtime struct {
						Result   string   `yaml:"result"`
						NotReady []string `yaml:"notReady"`
					} `yaml:"runtime"`
				} `yaml:"configHubKubectlApply"`
				OCIArgo struct {
					Result  string `yaml:"result"`
					App     string `yaml:"app"`
					Sync    string `yaml:"sync"`
					Health  string `yaml:"health"`
					Runtime struct {
						Result   string   `yaml:"result"`
						NotReady []string `yaml:"notReady"`
					} `yaml:"runtime"`
				} `yaml:"configHubOciArgo"`
			} `yaml:"legs"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatalf("decode recorded receipt: %v", err)
	}
	if receipt.Kind != "LiveHelmConfigHubParityReceipt" || receipt.Spec.Result != "watch" || receipt.Spec.ObservedAt != "2026-06-14T14:36:32Z" {
		t.Fatalf("unexpected receipt identity/result/time: kind=%q result=%q observedAt=%q", receipt.Kind, receipt.Spec.Result, receipt.Spec.ObservedAt)
	}
	if receipt.Spec.Legs.RegularHelm.Result != "pass" || receipt.Spec.Legs.RegularHelm.Runtime.Result != "pass" || len(receipt.Spec.Legs.RegularHelm.Runtime.NotReady) != 0 ||
		receipt.Spec.Legs.KubectlApply.Result != "pass" || receipt.Spec.Legs.KubectlApply.Runtime.Result != "pass" || len(receipt.Spec.Legs.KubectlApply.Runtime.NotReady) != 0 ||
		receipt.Spec.Legs.OCIArgo.Result != "watch" || receipt.Spec.Legs.OCIArgo.Runtime.Result != "pass" || len(receipt.Spec.Legs.OCIArgo.Runtime.NotReady) != 0 || receipt.Spec.Legs.OCIArgo.Sync != "Synced" || receipt.Spec.Legs.OCIArgo.Health != "Progressing" || receipt.Spec.Legs.OCIArgo.App != "hashicorp-consul-secure-mesh-existing-secrets-parity" {
		t.Fatal("receipt no longer supports pass workload checks alongside the watched Argo leg")
	}

	var child struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Status struct {
			Sync struct {
				Status string `json:"status"`
			} `json:"sync"`
			Health struct {
				Status string `json:"status"`
			} `json:"health"`
			Resources []struct {
				Kind      string `json:"kind"`
				Namespace string `json:"namespace"`
				Name      string `json:"name"`
				Status    string `json:"status"`
				Health    struct {
					Status  string  `json:"status"`
					Message *string `json:"message"`
				} `json:"health"`
			} `json:"resources"`
		} `json:"status"`
	}
	if err := json.Unmarshal(childBytes, &child); err != nil {
		t.Fatalf("decode child Application capture: %v", err)
	}
	if child.Metadata.Name != "hashicorp-consul-secure-mesh-existing-secrets-parity" || child.Metadata.Namespace != "argocd" || child.Status.Sync.Status != "Synced" || child.Status.Health.Status != "Progressing" {
		t.Fatalf("unexpected child Application identity/state: %s/%s sync=%s health=%s", child.Metadata.Namespace, child.Metadata.Name, child.Status.Sync.Status, child.Status.Health.Status)
	}
	var foundIngress bool
	for _, r := range child.Status.Resources {
		if r.Kind == "Ingress" && r.Namespace == "consul" && r.Name == "consul-consul-ui" {
			foundIngress = r.Status == "Synced" && r.Health.Status == "Progressing"
			if r.Health.Message != nil && *r.Health.Message != "" {
				t.Fatalf("the exact Ingress carries an unreviewed cause message: %q", *r.Health.Message)
			}
		}
	}
	if !foundIngress {
		t.Fatal("child capture lacks the exact Synced/Progressing Ingress evidence")
	}
	for _, r := range child.Status.Resources {
		if strings.EqualFold(r.Kind, "Secret") {
			t.Fatal("child capture contains a Secret resource; do not publish payload-bearing captures")
		}
	}
	if strings.Contains(string(childBytes), "-----BEGIN ") || strings.Contains(string(childBytes), "Bearer ") {
		t.Fatal("child capture contains a credential-like payload")
	}
	var provenance struct {
		Format     string `json:"format"`
		Repository string `json:"repository"`
		Revision   string `json:"revision"`
		Files      []struct {
			Path string `json:"path"`
			Name string `json:"local_name"`
			SHA  string `json:"sha256"`
		} `json:"files"`
		Limits struct {
			AtomicJoin bool `json:"atomic_join_established"`
			Current    bool `json:"current_live_state_established"`
			FullK8s    bool `json:"full_kubernetes_object_snapshot"`
		} `json:"limits"`
	}
	if err := json.Unmarshal(provenanceBytes, &provenance); err != nil {
		t.Fatalf("decode bounded source provenance: %v", err)
	}
	if provenance.Format != "pinned-public-recording/v1" || provenance.Repository != "confighub/helm-expt" || provenance.Revision != "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0" || len(provenance.Files) != 2 || provenance.Files[0].SHA != sha256Hex(receiptBytes) || provenance.Files[1].SHA != sha256Hex(childBytes) || provenance.Limits.AtomicJoin || provenance.Limits.Current || provenance.Limits.FullK8s || strings.Contains(string(provenanceBytes), "residual_cause") {
		t.Fatalf("unexpected provenance or unsupported scope claim: %+v", provenance)
	}

	checkConsulHLT03Scaffold(t, root)
	promptBytes, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(promptBytes)
	if strings.Contains(prompt, "Ingress/consul/consul-consul-ui") || strings.Contains(prompt, `"receipt_outcome":"WATCH"`) || !strings.Contains(prompt, "hashicorp-consul-secure-mesh-existing-secrets-parity") {
		t.Fatal("prompt leaks the expected residual/outcome or omits the exact child Application identity")
	}
}

func checkConsulHLT03Scaffold(t *testing.T, root string) {
	t.Helper()
	caseYAML, err := os.ReadFile(filepath.Join(root, "case.yaml"))
	if err != nil || !strings.Contains(string(caseYAML), "FIXTURE-OWNED-SCAFFOLD") {
		t.Fatalf("case must declare its fixture-owned scaffold: %v", err)
	}
	scaffold, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, arm := range []string{"with", "without"} {
		workspace := t.TempDir()
		cmd := exec.Command("bash", scaffold)
		cmd.Dir = workspace
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("run %s receipt scaffold: %v\n%s", arm, err, output)
		}
		entries, err := os.ReadDir(filepath.Join(workspace, "cluster"))
		if err != nil || len(entries) != 3 {
			t.Fatalf("%s scaffold inventory: %d files, %v", arm, len(entries), err)
		}
		for _, name := range []string{"receipt.yaml", "argocd-child.json", "source-provenance.json"} {
			got, err := os.ReadFile(filepath.Join(workspace, "cluster", name))
			want, readErr := os.ReadFile(filepath.Join(root, "fixtures", name))
			if err != nil || readErr != nil || string(got) != string(want) {
				t.Fatalf("%s scaffold output %s differs from source: %v %v", arm, name, err, readErr)
			}
		}
	}
}

func TestConsulHLT03AnswerGraderPythonAndJavaScript(t *testing.T) {
	root := filepath.Join("..", "..", "evals", consulHLT03Dir)
	grader, err := os.ReadFile(filepath.Join(root, "graders", "answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	var graderFrontmatter struct {
		Type    string `yaml:"type"`
		Pattern string `yaml:"pattern"`
		Flags   string `yaml:"flags"`
		Target  string `yaml:"target"`
	}
	if err := yaml.Unmarshal([]byte(strings.TrimSuffix(strings.TrimPrefix(string(grader), "---\n"), "---\n")), &graderFrontmatter); err != nil {
		t.Fatalf("decode grader frontmatter: %v", err)
	}
	if graderFrontmatter.Type != "regex" || graderFrontmatter.Flags != "s" || graderFrontmatter.Target != "last_message" || graderFrontmatter.Pattern == "" {
		t.Fatalf("unexpected grader contract: %+v", graderFrontmatter)
	}

	answer := map[string]string{
		"receipt_outcome": "WATCH", "workload_outcome": "PASS", "child_sync": "SYNCED", "child_health": "PROGRESSING",
		"residual_identity": "Ingress/consul/consul-consul-ui", "residual_sync": "SYNCED", "residual_health": "PROGRESSING",
		"residual_cause": "UNKNOWN", "observation_scope": "HISTORICAL_SEQUENTIAL_NONATOMIC_NOT_CURRENT_NOT_FULL_K8S_SNAPSHOT",
	}
	good := marshalHLT03Answer(t, answer)
	reordered := `{"observation_scope":"HISTORICAL_SEQUENTIAL_NONATOMIC_NOT_CURRENT_NOT_FULL_K8S_SNAPSHOT", "residual_cause":"UNKNOWN", "residual_health":"PROGRESSING", "residual_sync":"SYNCED", "residual_identity":"Ingress/consul/consul-consul-ui", "child_health":"PROGRESSING", "child_sync":"SYNCED", "workload_outcome":"PASS", "receipt_outcome":"WATCH"}`
	whitespace := "{\n" + strings.Join([]string{
		`  "receipt_outcome": "WATCH"`, `  "workload_outcome": "PASS"`, `  "child_sync": "SYNCED"`, `  "child_health": "PROGRESSING"`,
		`  "residual_identity": "Ingress/consul/consul-consul-ui"`, `  "residual_sync": "SYNCED"`, `  "residual_health": "PROGRESSING"`,
		`  "residual_cause": "UNKNOWN"`, `  "observation_scope": "HISTORICAL_SEQUENTIAL_NONATOMIC_NOT_CURRENT_NOT_FULL_K8S_SNAPSHOT"`,
	}, ",\n") + "\n}\n"
	bad := []string{
		mutateHLT03(t, answer, "receipt_outcome", "PASS"),
		mutateHLT03(t, answer, "workload_outcome", "UNKNOWN"),
		mutateHLT03(t, answer, "child_sync", "OUT_OF_SYNC"),
		mutateHLT03(t, answer, "child_health", "HEALTHY"),
		mutateHLT03(t, answer, "residual_identity", "Service/consul/consul-consul-ui"),
		mutateHLT03(t, answer, "residual_sync", "OUT_OF_SYNC"),
		mutateHLT03(t, answer, "residual_health", "HEALTHY"),
		mutateHLT03(t, answer, "residual_cause", "RECORDED"),
		mutateHLT03(t, answer, "observation_scope", "CURRENT"),
		mutateHLT03(t, answer, "observation_scope", "ATOMIC"),
		strings.Replace(good, `"residual_identity":"Ingress/consul/consul-consul-ui"`, `"residual_identity":"Ingress/consul/other-ui"`, 1),
		strings.Replace(good, `"child_health":"PROGRESSING"`, `"child_health":"HEALTHY"`, 1),
		strings.TrimSuffix(good, "}") + `,"extra":"claim"}`,
		strings.Replace(good, `,"residual_health":"PROGRESSING"`, `,"residual_health":"PROGRESSING","residual_health":"HEALTHY"`, 1),
		strings.Replace(good, `,"residual_cause":"UNKNOWN"`, ``, 1),
		"Answer: " + good,
		good + "\nExplanation",
	}
	frontmatter := strings.SplitN(string(grader), "---", 3)
	if len(frontmatter) != 3 {
		t.Fatal("grader needs YAML frontmatter")
	}
	for engine, command := range map[string][]string{
		"python":     {"python3", "-c", pythonRegexHarness},
		"javascript": {"node", "-e", javaScriptRegexHarness},
	} {
		if _, err := exec.LookPath(command[0]); err != nil {
			if engine == "python" {
				t.Skip("python3 is required for grader contract checks")
			}
			t.Skip("node is required to cross-check JavaScript grader semantics")
		}
		input, err := json.Marshal(struct {
			Pattern string   `json:"pattern"`
			Flags   string   `json:"flags"`
			Good    []string `json:"good"`
			Bad     []string `json:"bad"`
		}{graderFrontmatter.Pattern, graderFrontmatter.Flags, []string{good, reordered, whitespace}, bad})
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Stdin = strings.NewReader(string(input))
		cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s regex behavior failed: %v\n%s", engine, err, output)
		}
	}
}

func TestConsulHLT03BenchmarkMappingStaysPreparedAndUnrun(t *testing.T) {
	manifestBytes, err := os.ReadFile(filepath.Join("..", "..", "evals", "benchmark-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Status string `json:"status"`
		Groups []struct {
			Cases []struct {
				ID                 string `json:"id"`
				Status             string `json:"status"`
				ExistingCase       string `json:"existing_case"`
				BenchmarkAdmission string `json:"benchmark_admission"`
				Provenance         struct {
					Repository string `json:"repository"`
					Revision   string `json:"revision"`
					Files      []struct {
						Path string `json:"path"`
						SHA  string `json:"sha256"`
					} `json:"files"`
					Limits string `json:"limits"`
				} `json:"source_provenance"`
			} `json:"cases"`
		} `json:"groups"`
		Execution struct {
			Paid bool `json:"paid_runs_authorized_by_this_manifest"`
		} `json:"execution"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	var hlt03 *struct {
		ID                 string `json:"id"`
		Status             string `json:"status"`
		ExistingCase       string `json:"existing_case"`
		BenchmarkAdmission string `json:"benchmark_admission"`
		Provenance         struct {
			Repository string `json:"repository"`
			Revision   string `json:"revision"`
			Files      []struct {
				Path string `json:"path"`
				SHA  string `json:"sha256"`
			} `json:"files"`
			Limits string `json:"limits"`
		} `json:"source_provenance"`
	}
	caseCount := 0
	for gi := range manifest.Groups {
		for ci := range manifest.Groups[gi].Cases {
			caseCount++
			if manifest.Groups[gi].Cases[ci].ID == "HLT-03" {
				hlt03 = &manifest.Groups[gi].Cases[ci]
			}
		}
	}
	if caseCount != 24 || manifest.Status != "frozen_design_not_executable" || manifest.Execution.Paid || hlt03 == nil {
		t.Fatalf("HLT-03 mapping changed benchmark inventory/execution gate: cases=%d status=%q paid=%v found=%v", caseCount, manifest.Status, manifest.Execution.Paid, hlt03 != nil)
	}
	if hlt03.Status != "recorded_projection_prepared_not_run" || hlt03.ExistingCase != "evals/consul-ingress-residue" || !strings.Contains(hlt03.BenchmarkAdmission, "not run") || hlt03.Provenance.Repository != "confighub/helm-expt" || hlt03.Provenance.Revision != "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0" || len(hlt03.Provenance.Files) != 2 || hlt03.Provenance.Files[0].SHA != "521b08ae448f4d6503123ddef0263944887ced4204debab457677829c43fba15" || hlt03.Provenance.Files[1].SHA != "adf89697a41493e7399ff08b0e777a97e14d691b377d2fdddff2fa4faa27a078" || !strings.Contains(hlt03.Provenance.Limits, "not an atomic join") || !strings.Contains(hlt03.Provenance.Limits, "full Kubernetes snapshot") {
		t.Fatalf("unexpected HLT-03 mapping/provenance: %+v", *hlt03)
	}
}

const pythonRegexHarness = `import json,re,sys
x=json.load(sys.stdin); p=re.compile(x['pattern'], re.S if 's' in x['flags'] else 0)
assert all(p.fullmatch(s) for s in x['good']), 'rejected valid answer or key order'
assert all(not p.fullmatch(s) for s in x['bad']), 'accepted incorrect/overclaimed answer'
`

const javaScriptRegexHarness = `const fs=require('fs'); const x=JSON.parse(fs.readFileSync(0,'utf8')); const p=new RegExp(x.pattern,x.flags);
if (!x.good.every(s=>p.test(s))) throw new Error('rejected valid answer or key order');
if (x.bad.some(s=>p.test(s))) throw new Error('accepted incorrect/overclaimed answer');
`

func marshalHLT03Answer(t *testing.T, answer map[string]string) string {
	t.Helper()
	b, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mutateHLT03(t *testing.T, original map[string]string, key, value string) string {
	t.Helper()
	changed := make(map[string]string, len(original))
	for k, v := range original {
		changed[k] = v
	}
	changed[key] = value
	return marshalHLT03Answer(t, changed)
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
