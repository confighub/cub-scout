package unit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func checkPRE01CRDScaffold(t *testing.T, root string) {
	t.Helper()
	want, err := os.ReadDir(filepath.Join(root, "fixtures"))
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 15 {
		t.Fatalf("PRE-01 fixture count=%d, want 15", len(want))
	}
	var first map[string][]byte
	for arm := 0; arm < 2; arm++ {
		workspace := t.TempDir()
		script, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", script)
		cmd.Dir = workspace
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("run PRE-01 scaffold arm %d: %v\n%s", arm, err, output)
		}
		staged, err := os.ReadDir(filepath.Join(workspace, "cluster"))
		if err != nil || len(staged) != len(want) {
			t.Fatalf("arm %d staged %d files, want %d: %v", arm, len(staged), len(want), err)
		}
		current := make(map[string][]byte, len(staged))
		for _, entry := range staged {
			name := entry.Name()
			got, err := os.ReadFile(filepath.Join(workspace, "cluster", name))
			if err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile(filepath.Join(root, "fixtures", name))
			if err != nil || !bytes.Equal(got, source) {
				t.Fatalf("arm %d changed fixture %s: %v", arm, name, err)
			}
			current[name] = got
		}
		if arm == 0 {
			first = current
		} else {
			for name, body := range first {
				if !bytes.Equal(current[name], body) {
					t.Errorf("arms received different bytes for %s", name)
				}
			}
		}
	}
}

func TestPRE01RawRecordingAndStrictAnswerContract(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "pre01-crd")
	files, err := os.ReadDir(filepath.Join(root, "fixtures"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 15 {
		t.Fatalf("fixture count=%d, want 15", len(files))
	}
	scopeBytes, err := os.ReadFile(filepath.Join(root, "fixtures", "capture-scope.json"))
	if err != nil {
		t.Fatal(err)
	}
	var scope struct {
		Schema  string `json:"schema"`
		Capture struct {
			CaptureSourceCommit string `json:"captureSourceCommit"`
			CaptureScriptSHA256 string `json:"captureScriptSha256"`
			SourceRevision      string `json:"sourceRevision"`
			ProductRevision     string `json:"productSourceRevision"`
			ProductBinarySHA256 string `json:"productBinarySha256"`
			WrapperProofResult  string `json:"wrapperProofResult"`
			WrapperStartedAt    string `json:"wrapperStartedAt"`
			WrapperEndedAt      string `json:"wrapperEndedAt"`
			AtomicSnapshot      bool   `json:"atomicSnapshot"`
		} `json:"capture"`
		Requests []struct {
			Phase      string  `json:"phase"`
			Method     string  `json:"method"`
			Path       string  `json:"path"`
			Status     int     `json:"httpStatus"`
			RawFile    string  `json:"rawFile"`
			RawBytes   int     `json:"rawBytes"`
			RawSHA256  string  `json:"rawSha256"`
			StartedAt  string  `json:"startedAt"`
			EndedAt    string  `json:"endedAt"`
			ElapsedSec float64 `json:"elapsedSeconds"`
		} `json:"requests"`
		Operations []struct {
			Phase      string `json:"phase"`
			ExitCode   int    `json:"exitCode"`
			StdoutFile string `json:"stdoutFile"`
			StdoutHash string `json:"stdoutSha256"`
			StderrFile string `json:"stderrFile"`
			StderrHash string `json:"stderrSha256"`
		} `json:"dependentOperations"`
		Files map[string]struct {
			Bytes int    `json:"bytes"`
			Hash  string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(scopeBytes, &scope); err != nil {
		t.Fatal(err)
	}
	if scope.Schema != "pre01-crd-capture-scope.v1" || scope.Capture.CaptureSourceCommit != "682160f4cf536a8a16e48ebf237976609d3d1c5f" || scope.Capture.CaptureScriptSHA256 != "4c2390164be8a1651e836fe4d8b5a5245ca21fd6fca266f99bfd645d1bcc3800" || scope.Capture.SourceRevision != "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0" || scope.Capture.ProductRevision != "eec6d442279955af0f1fc39c637252ac8eb0f081" || scope.Capture.ProductBinarySHA256 != "7d20aa7bb33b477dfd88afb2f53b1a6f773a5a4ffbcff7f81d8dcbfeb5a12bab" || scope.Capture.WrapperProofResult != "exit 0" || scope.Capture.WrapperStartedAt != "2026-10-01T03:56:51Z" || scope.Capture.WrapperEndedAt != "2026-10-01T03:57:30Z" || scope.Capture.AtomicSnapshot {
		t.Fatalf("PRE-01 provenance is incomplete or overstates capture: %+v", scope.Capture)
	}
	scopeHash := sha256.Sum256(scopeBytes)
	if hex.EncodeToString(scopeHash[:]) != "6d39e83612d52f5124bad2dd63b27b30b94cce492eb70223f759248de71bfec4" {
		t.Fatal("capture-scope.json changed from the reviewed fixture")
	}
	if len(scope.Requests) != 9 || len(scope.Operations) != 2 || len(scope.Files) != 14 {
		t.Fatalf("scope inventory request=%d operation=%d files=%d", len(scope.Requests), len(scope.Operations), len(scope.Files))
	}
	wantHashes := map[string]string{
		"absent-after-apply-crd.json":              "661902cf15f640cf0cfbcb7d5df71d36ac2d9ac6041f1b92429e66704c1614c0",
		"absent-after-apply-servicemonitor.body":   "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793",
		"absent-apply.stderr.txt":                  "eec841cdc7ba8979d51398d7dc9a753ea8d2a8efb13d32312596a37863cb2c1b",
		"absent-apply.stdout.txt":                  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"absent-crd.json":                          "661902cf15f640cf0cfbcb7d5df71d36ac2d9ac6041f1b92429e66704c1614c0",
		"absent-discovery.body":                    "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793",
		"absent-servicemonitor.body":               "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793",
		"present-apply.stderr.txt":                 "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"present-apply.stdout.txt":                 "daabc3a7462204b0ba9a99bd0893b37872cbeb0a5954ac8d6a2756ba70acf505",
		"present-before-apply-servicemonitor.json": "d6ad3cd3caf021eda33553b022d71402a909db0cb5f2faad407075eff973efd6",
		"present-crd.json":                         "a5e571a80d5882cc9294cfabc27e0e6cefead3612e8e880afb885e3bf60f3e3c",
		"present-discovery.json":                   "e791bb13f75f7767dd2c40df7485653342b0894ca58fd3af17b1b03ed087c1a8",
		"present-servicemonitor.json":              "78836df757ca3eb79f25bb0190f03d465fb282b455b8a27e79fbef67247a650e",
		"servicemonitor.normalized.yaml":           "8379723ecf51a8776c6dceeedbd7101cbb9838c94feef4cd21bd5eb75f603b75",
	}
	if len(wantHashes) != len(scope.Files) {
		t.Fatal("hard-coded source hash inventory has the wrong size")
	}
	for name, recorded := range scope.Files {
		body, err := os.ReadFile(filepath.Join(root, "fixtures", name))
		if err != nil {
			t.Fatalf("recorded fixture %s: %v", name, err)
		}
		hash := sha256.Sum256(body)
		if len(body) != recorded.Bytes || hex.EncodeToString(hash[:]) != recorded.Hash || recorded.Hash != wantHashes[name] {
			t.Errorf("fixture %s does not match factual byte/hash inventory", name)
		}
	}
	if !bytes.Contains(mustRead(t, filepath.Join(root, "fixtures", "absent-discovery.body")), []byte("404 page not found\n")) || !bytes.Contains(mustRead(t, filepath.Join(root, "fixtures", "absent-servicemonitor.body")), []byte("404 page not found\n")) {
		t.Fatal("raw route 404 bodies were normalized or replaced")
	}
	if scope.Requests[0].Phase != "absent" || scope.Requests[0].Status != 404 || scope.Requests[4].RawFile != "absent-after-apply-servicemonitor.body" || scope.Requests[5].Phase != "present" || scope.Requests[7].Phase != "present-before-apply" || scope.Requests[8].Status != 200 {
		t.Fatal("request ordering/status facts do not preserve the two-phase capture")
	}
	if scope.Operations[0].Phase != "absent" || scope.Operations[0].ExitCode != 1 || scope.Operations[1].Phase != "present" || scope.Operations[1].ExitCode != 0 {
		t.Fatal("dependent operation exit codes do not preserve observed absent/present phases")
	}
	for _, forbidden := range []string{"derivedScoutReceipts", "observer-token", "admin-private", "observer.kubeconfig", "owned-cluster-marker", "present-scout-receipt", "absent-scout-receipt"} {
		if strings.Contains(string(scopeBytes), forbidden) {
			t.Errorf("scope metadata includes excluded material %q", forbidden)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "fixtures", "provenance.json")); !os.IsNotExist(err) {
		t.Fatal("full provenance with derived receipts or config hashes was staged")
	}

	prompt := string(mustRead(t, filepath.Join(root, "prompt.md")))
	for _, leaked := range []string{"774c4c4e-97b9-40b4-801d-39efe8d2b81b", "kube-prometheus-stack-kube-state-metrics", "ad5a1b97-abcc-4146-9443-4ce1b9500495"} {
		if strings.Contains(prompt, leaked) {
			t.Errorf("prompt leaks answer literal %q", leaked)
		}
	}
	grader := string(mustRead(t, filepath.Join(root, "graders", "verified-answer.md")))
	pattern := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindStringSubmatch(grader)
	if len(pattern) != 2 {
		t.Fatal("strict answer grader regex is missing")
	}
	re := regexp.MustCompile(pattern[1])
	good := `{"crd_name":"servicemonitors.monitoring.coreos.com","dependent_operation":"kubectl apply|monitoring.coreos.com/v1|ServiceMonitor|monitoring/kube-prometheus-stack-kube-state-metrics","crd_before":"ABSENT","pre_setup_apply":"MISSING_GVK_FAILURE","crd_after_failed_apply":"ABSENT","crd_registration":"ESTABLISHED","api_discovery":"REGISTERED","object_before_successful_apply":"TYPED_NOT_FOUND","post_setup_apply":"CREATED","object_identity":"monitoring.coreos.com/v1|ServiceMonitor|monitoring/kube-prometheus-stack-kube-state-metrics|774c4c4e-97b9-40b4-801d-39efe8d2b81b|475","health_claim":"NOT_PROVEN","evidence":"capture-scope.json+absent-crd.json+absent-discovery.body+absent-servicemonitor.body+absent-after-apply-crd.json+absent-after-apply-servicemonitor.body+present-crd.json+present-discovery.json+present-before-apply-servicemonitor.json+present-servicemonitor.json+servicemonitor.normalized.yaml+absent-apply.stdout.txt+absent-apply.stderr.txt+present-apply.stdout.txt+present-apply.stderr.txt"}`
	if !re.MatchString(good) {
		t.Fatal("strict grader rejects exact recorded answer")
	}
	for _, invalid := range []string{
		"explanation: " + good,
		strings.TrimSuffix(good, "}") + `,"extra":"x"}`,
		strings.Replace(good, `"crd_name":"servicemonitors.monitoring.coreos.com"`, `"crd_name":"other"`, 1),
		strings.Replace(good, `"dependent_operation":"kubectl apply|monitoring.coreos.com/v1|ServiceMonitor|monitoring/kube-prometheus-stack-kube-state-metrics"`, `"dependent_operation":"unknown"`, 1),
		strings.Replace(good, `"crd_before":"ABSENT"`, `"crd_before":"PRESENT"`, 1),
		strings.Replace(good, `"health_claim":"NOT_PROVEN"`, `"health_claim":"PROVEN"`, 1),
		strings.Replace(good, `"pre_setup_apply":"MISSING_GVK_FAILURE"`, `"pre_setup_apply":"SUCCEEDED"`, 1),
		strings.Replace(good, `"object_identity":"`, `"object_identity":"x","object_identity":"`, 1),
	} {
		if re.MatchString(invalid) {
			t.Errorf("strict grader accepted invalid answer: %.100s", invalid)
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPRE01BenchmarkMappingRemainsUnrun(t *testing.T) {
	b := mustRead(t, filepath.Join("..", "..", "evals", "benchmark-v1.json"))
	var manifest struct {
		Status string `json:"status"`
		Paid   bool   `json:"paid_runs_authorized_by_this_manifest"`
		Groups []struct {
			Weight float64 `json:"weight"`
			Cases  []struct {
				ID           string `json:"id"`
				Status       string `json:"status"`
				ExistingCase string `json:"existing_case"`
				Provenance   struct {
					RawFiles map[string]string `json:"raw_files_sha256"`
				} `json:"source_provenance"`
				Admission string `json:"benchmark_admission"`
			} `json:"cases"`
		} `json:"groups"`
		Execution struct {
			Paid bool `json:"paid_runs_authorized_by_this_manifest"`
		} `json:"execution"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "frozen_design_not_executable" || manifest.Execution.Paid {
		t.Fatal("PRE-01 preparation must not change benchmark execution gates")
	}
	found := false
	for _, group := range manifest.Groups {
		if group.Weight != 0.1666666667 {
			t.Errorf("group weight changed: %g", group.Weight)
		}
		for _, c := range group.Cases {
			if c.ID == "PRE-01" {
				found = true
				if c.Status != "raw_recording_prepared_not_run" || c.ExistingCase != "evals/pre01-crd" || len(c.Provenance.RawFiles) != 15 || !strings.Contains(c.Admission, "not run") {
					t.Fatalf("unexpected PRE-01 readiness: %+v", c)
				}
				for name, digest := range c.Provenance.RawFiles {
					if name == "capture-scope.json" {
						if digest != "6d39e83612d52f5124bad2dd63b27b30b94cce492eb70223f759248de71bfec4" {
							t.Fatal("benchmark hash for capture-scope.json does not match reviewed scope")
						}
						continue
					}
					body := mustRead(t, filepath.Join("..", "..", "evals", "pre01-crd", "fixtures", name))
					hash := sha256.Sum256(body)
					if hex.EncodeToString(hash[:]) != digest {
						t.Errorf("benchmark raw hash mismatch for %s", name)
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("PRE-01 benchmark case is missing")
	}
}
