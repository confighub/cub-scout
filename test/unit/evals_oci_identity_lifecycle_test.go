// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const ociIdentitySourceRevision = "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0"

var ociIdentityReceiptHashes = map[string]string{
	"oci-evidence-chain.yaml":            "e9816912866968552b93e31ea6ac7b919bdea0536c05d8ba72ff32bf91f5f43e",
	"installer-publication-receipt.yaml": "ccf8623f8d5328461e909448865e918ac01454bbf99225714672bf23eec6c0d3",
	"render-intent.yaml":                 "4e88c279923b1ff1d025e4f1c2f4f8eae572e70363d4bf211498bdf35de1c21a",
	"render-receipt.yaml":                "a67ac02c888bcf56b4adac6a6cf481c001bc59e26d8618cfba1af8988bb0fd09",
	"catalog-delivery-proof.yaml":        "0734c0be8c931e96fee18917b66aaae619954a3c467a384e8cbc60e002da4d38",
}

var ociIdentitySourcePaths = map[string]string{
	"oci-evidence-chain.yaml":            "data/oci-evidence-chains/records/cub-installer-nginx-three-consumers.yaml",
	"installer-publication-receipt.yaml": "runs/installer-oci/bitnami-nginx/24.0.2/installer-package-publication-receipt.yaml",
	"render-intent.yaml":                 "data/helm-render-intents/intents/bitnami-nginx-24-0-2-http-clusterip.yaml",
	"render-receipt.yaml":                "recipes/bitnami/nginx/24.0.2/revisions/http-clusterip/r001/receipts/render-receipt.yaml",
	"catalog-delivery-proof.yaml":        "runs/catalog-oci-delivery-proof/bitnami-nginx-24-0-2-http-clusterip.yaml",
}

func TestOCIIdentityLifecyclePinnedReceiptsAndEqualScaffolds(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "oci-identity-lifecycle")
	checkOCIIdentityLifecycleScaffold(t, root)
}

// checkOCIIdentityLifecycleScaffold validates this receipts-only fixture-owned
// scaffold without requiring unrelated suite-wide Kubernetes exports.
func checkOCIIdentityLifecycleScaffold(t *testing.T, root string) {
	t.Helper()
	fixtureDir := filepath.Join(root, "fixtures", "evidence")
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(ociIdentityReceiptHashes) {
		t.Fatalf("fixture has %d files, want exactly %d pinned receipts", len(entries), len(ociIdentityReceiptHashes))
	}
	fixtures := make(map[string][]byte, len(entries))
	secretPayload := regexp.MustCompile(`(?im)^\s*(?:password|client[_-]?secret|access[_-]?token|refresh[_-]?token|private[_-]?key|authorization)\s*:`)
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected directory in evidence fixture: %s", entry.Name())
		}
		want, ok := ociIdentityReceiptHashes[entry.Name()]
		if !ok {
			t.Fatalf("unexpected unpinned evidence file %q", entry.Name())
		}
		b, err := os.ReadFile(filepath.Join(fixtureDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("%s sha256=%s, want pinned source %s", entry.Name(), got, want)
		}
		if secretPayload.Match(b) {
			t.Errorf("%s appears to contain credential payload", entry.Name())
		}
		fixtures[entry.Name()] = b
	}

	var armCopies [2]map[string][]byte
	for arm := range armCopies {
		workspace := t.TempDir()
		script, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", script)
		cmd.Dir = workspace
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("scaffold arm %d: %v\n%s", arm, err, output)
		}
		armCopies[arm] = make(map[string][]byte, len(fixtures))
		for name, original := range fixtures {
			got, err := os.ReadFile(filepath.Join(workspace, "evidence", "oci-identity-lifecycle", name))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, original) {
				t.Errorf("arm %d changed pinned receipt %s", arm, name)
			}
			armCopies[arm][name] = got
		}
	}
	for name := range fixtures {
		if !bytes.Equal(armCopies[0][name], armCopies[1][name]) {
			t.Errorf("with/without arms received different receipt bytes for %s", name)
		}
	}
}

func TestOCIIdentityLifecycleCaseMetadataAndPromptContract(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "oci-identity-lifecycle")
	caseBytes, err := os.ReadFile(filepath.Join(root, "case.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		SchemaVersion string `yaml:"schema_version"`
		Name          string `yaml:"name"`
		Context       struct {
			Scaffold string `yaml:"scaffold_script"`
		} `yaml:"context"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(caseBytes))
	decoder.KnownFields(true)
	if err := decoder.Decode(&schema); err != nil {
		t.Fatalf("decode case schema: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("case.yaml must contain one schema document, second decode=%v", err)
	}
	if schema.SchemaVersion != "1.1" || schema.Name != "oci-identity-lifecycle" || schema.Context.Scaffold != "scaffold.sh" {
		t.Fatalf("unexpected case schema: %+v", schema)
	}

	promptBytes, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(promptBytes)
	if !strings.HasPrefix(prompt, "---\n") {
		t.Fatal("prompt is missing execution frontmatter")
	}
	end := strings.Index(prompt[4:], "\n---\n")
	if end < 0 {
		t.Fatal("prompt frontmatter is unterminated")
	}
	var execution struct {
		Name            string   `yaml:"name"`
		Description     string   `yaml:"description"`
		ExpectedOutcome string   `yaml:"expected_outcome"`
		Tags            []string `yaml:"tags"`
		MaxTurns        int      `yaml:"max_turns"`
		TimeoutSeconds  int      `yaml:"timeout_seconds"`
		AllowedTools    []string `yaml:"allowed_tools"`
	}
	frontmatter := yaml.NewDecoder(strings.NewReader(prompt[4 : 4+end]))
	frontmatter.KnownFields(true)
	if err := frontmatter.Decode(&execution); err != nil {
		t.Fatalf("decode prompt execution frontmatter: %v", err)
	}
	if execution.Name != schema.Name || execution.Description == "" || execution.ExpectedOutcome == "" || execution.MaxTurns != 8 || execution.TimeoutSeconds != 120 || len(execution.AllowedTools) != 2 || execution.AllowedTools[0] != "Read" || execution.AllowedTools[1] != "Grep" {
		t.Fatalf("invalid execution metadata: %+v", execution)
	}
	for _, answerOnly := range []string{
		"europe-west1-docker.pkg.dev/nth-fort-499605-q5/helm-expt/bitnami-nginx:24.0.2@sha256:7cf08c0348a32d577ffa0e16069ec6c2510ce773b372008d25b938f9546c5f67",
		"7cf08c0348a32d577ffa0e16069ec6c2510ce773b372008d25b938f9546c5f67",
		"9905c5ba5f0b0187f5d22b7f4113856635476bb081d584b36251f5513b010414",
		"9547e0067fbdbb318d9e4309e3dcc74211bf01cb7ac243b8b23586ccc337d7e7",
		"7fb28597f2eea8113612fa9e67ac84760cba1e72c42a4c59e4c25d284e0a94ba",
		"1dbed784-4fb6-4092-a734-075eb8bdfd4c",
		"26d97438d5b52dcacf140d2ef4c57a97bacd0c63e22d5cfa19076a0723b73049",
		"882622f0736d2bd740da6189689cd716ee427b8f1ce44814291e9dc0274fbb89",
		"805bcc863fc3f602589fc75cae91eeedebad234d5ce5a476c96b03a747821e7f",
		"2026-07-26T17:10:27.568Z",
	} {
		if strings.Contains(prompt, answerOnly) {
			t.Errorf("prompt leaks reference answer value %q", answerOnly)
		}
	}
}

func TestOCIIdentityLifecycleGraderAcceptsUnorderedAndRejectsDigestSwaps(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "oci-identity-lifecycle")
	grader, err := os.ReadFile(filepath.Join(root, "graders", "verified-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindSubmatch(grader)
	if len(match) != 2 {
		t.Fatalf("missing single-line portable grader: %s", grader)
	}
	pattern := string(match[1])
	values := ociIdentityExpectedAnswer()
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	// Keep a stable forward order independent of map iteration.
	keys = []string{
		"package_oci_reference", "package_manifest_digest", "package_layer_digest",
		"rendered_manifest_sha256", "rendered_object_set_sha256", "confighub_release_id",
		"output_oci_digest", "bundle_digest", "consumer_digests_match",
		"recorded_consumer_results", "recorded_image_reference",
		"current_cluster_state", "independent_bundle_verification", "recorded_hook_policy",
		"no_hooks_render_flag", "lifecycle_observed", "hook_execution", "policy_execution",
		"observation_time",
	}
	good := marshalOCIAnswer(t, values, keys, false)
	reversed := append([]string(nil), keys...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	reordered := marshalOCIAnswer(t, values, reversed, false)
	pretty := marshalOCIAnswer(t, values, keys, true)

	negativeMaps := []map[string]string{
		{"package_manifest_digest": values["output_oci_digest"]},
		{"package_layer_digest": "sha256:7cf08c0348a32d577ffa0e16069ec6c2510ce773b372008d25b938f9546c5f67"},
		{"rendered_manifest_sha256": values["rendered_object_set_sha256"]},
		{"output_oci_digest": values["package_manifest_digest"]},
		{"bundle_digest": values["output_oci_digest"]},
		{"consumer_digests_match": "NO"},
		{"recorded_consumer_results": "Argo CD=1/1;Flux=1/1"},
		{"recorded_image_reference": "registry-1.docker.io/bitnami/nginx:latest"},
		{"current_cluster_state": "READY"},
		{"independent_bundle_verification": "YES"},
		{"recorded_hook_policy": "hooks"},
		{"no_hooks_render_flag": "--include-crds"},
		{"lifecycle_observed": "yes"},
		{"hook_execution": "YES"},
		{"policy_execution": "YES"},
		{"observation_time": "2026-09-30T22:45:00Z"},
	}
	candidates := []string{good, reordered, pretty}
	for _, changes := range negativeMaps {
		copyValues := make(map[string]string, len(values))
		for key, value := range values {
			copyValues[key] = value
		}
		for key, value := range changes {
			copyValues[key] = value
		}
		candidates = append(candidates, marshalOCIAnswer(t, copyValues, keys, false))
	}
	duplicate := strings.Replace(good, `"hook_execution":"UNKNOWN"`, `"hook_execution":"UNKNOWN","hook_execution":"UNKNOWN"`, 1)
	extra := strings.TrimSuffix(good, "}") + `,"unsupported":"value"}`
	missing := strings.Replace(good, `,"policy_execution":"NOT_RUN"`, "", 1)
	wrongType := strings.Replace(good, `"hook_execution":"UNKNOWN"`, `"hook_execution":true`, 1)
	candidates = append(candidates, duplicate, extra, missing, wrongType, "Answer: "+good, good+"\nExplanation")
	want := make([]bool, len(candidates))
	for i := range want {
		want[i] = i < 3
	}

	t.Run("python", func(t *testing.T) {
		got := runOCIIdentityRegex(t, "python3", `import json,re,sys; d=json.load(sys.stdin); p=re.compile(d["pattern"]); print(json.dumps([p.search(s) is not None for s in d["samples"]]))`, pattern, candidates)
		assertOCIRegexResults(t, "Python", got, want, candidates)
	})
	t.Run("javascript", func(t *testing.T) {
		if _, err := exec.LookPath("node"); err != nil {
			t.Skip("node unavailable; JavaScript regex cross-check skipped")
		}
		got := runOCIIdentityRegex(t, "node", `const d=JSON.parse(require("fs").readFileSync(0,"utf8")); const p=new RegExp(d.pattern); console.log(JSON.stringify(d.samples.map(s=>p.test(s))))`, pattern, candidates)
		assertOCIRegexResults(t, "JavaScript", got, want, candidates)
	})
}

func ociIdentityExpectedAnswer() map[string]string {
	return map[string]string{
		"package_oci_reference":           "oci://europe-west1-docker.pkg.dev/nth-fort-499605-q5/helm-expt/bitnami-nginx:24.0.2@sha256:7cf08c0348a32d577ffa0e16069ec6c2510ce773b372008d25b938f9546c5f67",
		"package_manifest_digest":         "sha256:7cf08c0348a32d577ffa0e16069ec6c2510ce773b372008d25b938f9546c5f67",
		"package_layer_digest":            "sha256:9905c5ba5f0b0187f5d22b7f4113856635476bb081d584b36251f5513b010414",
		"rendered_manifest_sha256":        "9547e0067fbdbb318d9e4309e3dcc74211bf01cb7ac243b8b23586ccc337d7e7",
		"rendered_object_set_sha256":      "7fb28597f2eea8113612fa9e67ac84760cba1e72c42a4c59e4c25d284e0a94ba",
		"confighub_release_id":            "1dbed784-4fb6-4092-a734-075eb8bdfd4c",
		"output_oci_digest":               "sha256:26d97438d5b52dcacf140d2ef4c57a97bacd0c63e22d5cfa19076a0723b73049",
		"bundle_digest":                   "sha256:882622f0736d2bd740da6189689cd716ee427b8f1ce44814291e9dc0274fbb89",
		"consumer_digests_match":          "YES",
		"recorded_consumer_results":       "Argo CD=1/1;Flux=1/1;Direct apply=1/1",
		"recorded_image_reference":        "registry-1.docker.io/bitnami/nginx@sha256:805bcc863fc3f602589fc75cae91eeedebad234d5ce5a476c96b03a747821e7f",
		"current_cluster_state":           "UNKNOWN",
		"independent_bundle_verification": "UNKNOWN",
		"recorded_hook_policy":            "no-hooks",
		"no_hooks_render_flag":            "--no-hooks",
		"lifecycle_observed":              "n/a",
		"hook_execution":                  "UNKNOWN",
		"policy_execution":                "NOT_RUN",
		"observation_time":                "2026-07-26T17:10:27.568Z",
	}
}

func marshalOCIAnswer(t *testing.T, values map[string]string, keys []string, pretty bool) string {
	t.Helper()
	var b bytes.Buffer
	indent := ""
	if pretty {
		indent = "  "
	}
	b.WriteString("{")
	for i, key := range keys {
		if _, ok := values[key]; !ok {
			t.Fatalf("answer is missing key %q", key)
		}
		if i > 0 {
			b.WriteByte(',')
		}
		if pretty {
			b.WriteString("\n" + indent)
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			t.Fatal(err)
		}
		valueJSON, err := json.Marshal(values[key])
		if err != nil {
			t.Fatal(err)
		}
		b.Write(keyJSON)
		b.WriteString(":")
		if pretty {
			b.WriteByte(' ')
		}
		b.Write(valueJSON)
	}
	if pretty {
		b.WriteByte('\n')
	}
	b.WriteByte('}')
	return b.String()
}

func runOCIIdentityRegex(t *testing.T, binary, program, pattern string, samples []string) []bool {
	t.Helper()
	payload, err := json.Marshal(struct {
		Pattern string   `json:"pattern"`
		Samples []string `json:"samples"`
	}{pattern, samples})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("%s unavailable", binary)
	}
	cmd := exec.Command(binary, "-c", program)
	if binary == "node" {
		cmd = exec.Command(binary, "-e", program)
	}
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s regex engine: %v\n%s", binary, err, output)
	}
	var result []bool
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode %s regex results: %v\n%s", binary, err, output)
	}
	return result
}

func assertOCIRegexResults(t *testing.T, engine string, got, want []bool, candidates []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s returned %d results for %d candidates", engine, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s grader result %d=%v, want %v for: %s", engine, i, got[i], want[i], candidates[i])
		}
	}
}

func TestOCIIdentityLifecycleCaseDocumentsRecordedLimits(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "oci-identity-lifecycle")
	b, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(b)), " ")
	if !strings.Contains(text, ociIdentitySourceRevision) {
		t.Errorf("README does not pin the public source revision %s", ociIdentitySourceRevision)
	}
	for fixture, sourcePath := range ociIdentitySourcePaths {
		row := fmt.Sprintf("`%s` | `%s` | `%s`", fixture, sourcePath, ociIdentityReceiptHashes[fixture])
		if !strings.Contains(text, row) {
			t.Errorf("README source provenance is missing fixture mapping %s", row)
		}
	}
	for _, phrase := range []string{
		"prepared-not-run", "not mapped into `benchmark-v1`", "not a raw Kubernetes snapshot",
		"or raw OCI/bundle bytes", "raw runtime image identity is outside",
		"not current-state evidence", "delivery, not policy execution",
		"independent digest recomputation is outside the fixture",
	} {
		if !strings.Contains(text, phrase) {
			t.Errorf("README omits evidence limit %q", phrase)
		}
	}
}
