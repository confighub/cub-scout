// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const sveltosInferredRevisionCase = "sveltos-inferred-revision"
const sveltosInferredRevisionOnboardExcerptSHA256 = "c32e6ffa43b150517ef5afded1ee2ab50401e25071199f0b0a0b7c76ed096b4b"
const sveltosInferredRevisionKnownExcerptSHA256 = "142a9dfd6119e4d3fff6cdf82b99ebae26ace4f5286992706a19b4e470ec7751"
const sveltosInferredRevisionReceiptSHA256 = "9aa3ac57090001aa5b37e5a91c095aaca95ea2a91edcbb2bdb77fe526aa857b2"

func TestSveltosInferredRevisionEvidenceAndGrader(t *testing.T) {
	caseRoot := filepath.Join("..", "..", "evals", sveltosInferredRevisionCase)
	onboard, err := os.ReadFile(filepath.Join(caseRoot, "fixtures", "source-doc", "onboard-excerpt.md"))
	if err != nil {
		t.Fatal(err)
	}
	known, err := os.ReadFile(filepath.Join(caseRoot, "fixtures", "source-doc", "known-behaviours-excerpt.md"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := os.ReadFile(filepath.Join(caseRoot, "fixtures", "receipt", "sveltos-oci-delivery-proof.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		data []byte
		want string
	}{
		"onboard excerpt":          {onboard, sveltosInferredRevisionOnboardExcerptSHA256},
		"known-behaviours excerpt": {known, sveltosInferredRevisionKnownExcerptSHA256},
		"delivery receipt":         {receipt, sveltosInferredRevisionReceiptSHA256},
	} {
		digest := sha256.Sum256(tc.data)
		if got := hex.EncodeToString(digest[:]); got != tc.want {
			t.Errorf("%s SHA-256 = %s, want %s", name, got, tc.want)
		}
	}
	onboardText, knownText, receiptText := string(onboard), string(known), string(receipt)
	onboardNormalized := strings.Join(strings.Fields(onboardText), " ")
	for _, evidence := range []string{
		"eu-central-uat1", "sha256:e2b3ed3756b1", "Which release a cluster runs is worked out from times",
		"Sveltos does not report which release it fetched", "not proof that the cluster runs that exact release",
	} {
		if !strings.Contains(onboardNormalized, evidence) {
			t.Errorf("pinned status excerpt missing %q", evidence)
		}
	}
	if !strings.Contains(knownText, "The release a cluster runs is worked out, not reported") || !strings.Contains(knownText, "not as proof of the exact artifact") {
		t.Fatal("known-behaviours excerpt no longer preserves the inference limit")
	}
	if regexp.MustCompile(`sha256:[0-9a-f]{64}`).Match(onboard) {
		t.Fatal("status document excerpt unexpectedly contains a full digest")
	}
	for _, fact := range []string{
		`recordedAt: "2026-08-14T16:41:11.683Z"`,
		`cluster: "hx-sveltos-fleet-pilot"`,
		`manifestDigest: "sha256:d2083bf32b4b70ddffc7375a4990dcc78299bc80369708641315d12be5b5183b"`,
		`profileMatchesApprovedRevision: true`,
		`chart: "kyverno-3.8.1"`,
		`status: "deployed"`,
	} {
		if !strings.Contains(receiptText, fact) {
			t.Errorf("separate delivery receipt missing %q", fact)
		}
	}
	if strings.Contains(receiptText, "-----BEGIN") || strings.Contains(receiptText, "kind: Secret") {
		t.Fatal("receipt fixture must not contain credentials or Secret payload")
	}

	caseData, err := os.ReadFile(filepath.Join(caseRoot, "case.yaml"))
	if err != nil || !strings.Contains(string(caseData), "FIXTURE-OWNED-SCAFFOLD") {
		t.Fatalf("case must declare its fixture-owned scaffold: %v", err)
	}
	var schema struct {
		SchemaVersion string `yaml:"schema_version"`
		Name          string `yaml:"name"`
		Context       struct {
			Scaffold string `yaml:"scaffold_script"`
		} `yaml:"context"`
	}
	caseDecoder := yaml.NewDecoder(bytes.NewReader(caseData))
	caseDecoder.KnownFields(true)
	if err := caseDecoder.Decode(&schema); err != nil {
		t.Fatalf("decode case schema: %v", err)
	}
	var extra any
	if err := caseDecoder.Decode(&extra); err != io.EOF {
		t.Fatalf("case.yaml must contain exactly one document, second decode=%v", err)
	}
	if schema.SchemaVersion != "1.1" || schema.Name != sveltosInferredRevisionCase || schema.Context.Scaffold != "scaffold.sh" {
		t.Fatalf("case schema or scaffold declaration invalid: %+v", schema)
	}
	prompt, err := os.ReadFile(filepath.Join(caseRoot, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	promptText := string(prompt)
	if !strings.HasPrefix(promptText, "---\n") {
		t.Fatal("prompt.md is missing execution frontmatter")
	}
	frontmatterEnd := strings.Index(promptText[4:], "\n---\n")
	if frontmatterEnd < 0 {
		t.Fatal("prompt.md frontmatter is unterminated")
	}
	var execution struct {
		Name           string   `yaml:"name"`
		Description    string   `yaml:"description"`
		Tags           []string `yaml:"tags"`
		MaxTurns       int      `yaml:"max_turns"`
		TimeoutSeconds int      `yaml:"timeout_seconds"`
		AllowedTools   []string `yaml:"allowed_tools"`
	}
	execDecoder := yaml.NewDecoder(strings.NewReader(promptText[4 : 4+frontmatterEnd]))
	execDecoder.KnownFields(true)
	if err := execDecoder.Decode(&execution); err != nil {
		t.Fatalf("decode execution frontmatter: %v", err)
	}
	if execution.Name != schema.Name || execution.Description == "" || execution.MaxTurns != 5 || execution.TimeoutSeconds != 90 || len(execution.AllowedTools) != 2 || execution.AllowedTools[0] != "Read" || execution.AllowedTools[1] != "Grep" {
		t.Fatalf("execution frontmatter invalid: %+v", execution)
	}
	if !containsString(execution.Tags, "DEL-03") || !containsString(execution.Tags, "sveltos") {
		t.Fatalf("execution frontmatter missing case tags: %+v", execution.Tags)
	}
	promptBody := promptText[4+frontmatterEnd+5:]
	for _, leaked := range []string{"eu-central-uat1", "sha256:e2b3ed3756b1", "hx-sveltos-fleet-pilot"} {
		if strings.Contains(promptBody, leaked) {
			t.Errorf("prompt leaks expected answer value %q", leaked)
		}
	}
	if strings.Contains(promptBody, "MCP") || strings.Contains(string(caseData), "MCP") {
		t.Fatal("case must not force or declare an MCP-specific route")
	}

	grader, err := os.ReadFile(filepath.Join(caseRoot, "graders", "verified-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindStringSubmatch(string(grader))
	if len(match) != 2 {
		t.Fatalf("grader has no portable single-quoted regex: %s", grader)
	}
	good := `{"cross_artifact_join":"UNESTABLISHED","receipt_workload_evidence":"helmrelease_chart_deployed","status_revision":"sha256:e2b3ed3756b1","receipt_profile_matches_approved_revision":true,"runtime_sveltos_digest":"UNKNOWN","receipt_cluster":"hx-sveltos-fleet-pilot","status_cluster":"eu-central-uat1","status_revision_basis":"inferred_from_timestamps"}`
	prettyUnordered := "{\n  \"status_revision_basis\" : \"inferred_from_timestamps\",\n  \"status_cluster\": \"eu-central-uat1\",\n  \"receipt_cluster\": \"hx-sveltos-fleet-pilot\",\n  \"status_revision\" : \"sha256:e2b3ed3756b1\",\n  \"receipt_profile_matches_approved_revision\" : true,\n  \"runtime_sveltos_digest\" : \"UNKNOWN\",\n  \"cross_artifact_join\" : \"UNESTABLISHED\",\n  \"receipt_workload_evidence\" : \"helmrelease_chart_deployed\"\n}"
	badAnswers := []string{
		strings.Replace(good, `"sha256:e2b3ed3756b1"`, `"sha256:d2083bf32b4b70ddffc7375a4990dcc78299bc80369708641315d12be5b5183b"`, 1),
		strings.Replace(good, `"inferred_from_timestamps"`, `"reported_exactly"`, 1),
		strings.Replace(good, `"receipt_profile_matches_approved_revision":true`, `"receipt_profile_matches_approved_revision":false`, 1),
		strings.Replace(good, `"receipt_workload_evidence":"helmrelease_chart_deployed"`, `"receipt_workload_evidence":"sveltos_runtime_digest"`, 1),
		strings.Replace(good, `"runtime_sveltos_digest":"UNKNOWN"`, `"runtime_sveltos_digest":"sha256:d2083bf32b4b70ddffc7375a4990dcc78299bc80369708641315d12be5b5183b"`, 1),
		strings.Replace(good, `"cross_artifact_join":"UNESTABLISHED"`, `"cross_artifact_join":"ESTABLISHED"`, 1),
		strings.Replace(good, `"status_cluster":"eu-central-uat1"`, `"status_cluster":"hx-sveltos-fleet-pilot"`, 1),
		strings.Replace(good, `"receipt_cluster":"hx-sveltos-fleet-pilot"`, `"receipt_cluster":"eu-central-uat1"`, 1),
		strings.TrimSuffix(good, "}") + `,"person":"operator"}`,
		strings.TrimSuffix(good, "}") + `,"current":true}`,
		strings.TrimSuffix(good, "}") + `,"atomic_observation":true}`,
		strings.Replace(good, `"status_revision":"sha256:e2b3ed3756b1"`, `"status_revision":"sha256:e2b3ed3756b1","status_revision":"sha256:d2083bf32b4b70ddffc7375a4990dcc78299bc80369708641315d12be5b5183b"`, 1),
		"Answer: " + good,
		good + "\nExplanation",
	}
	assertEvalRegexInPythonAndJS(t, match[1], []string{good, prettyUnordered}, badAnswers)
	checkSveltosInferredRevisionCaseScaffold(t, caseRoot)
}

func checkSveltosInferredRevisionCaseScaffold(t *testing.T, caseRoot string) {
	t.Helper()
	workspace := t.TempDir()
	scaffoldPath, err := filepath.Abs(filepath.Join(caseRoot, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scaffoldPath)
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run Sveltos revision scaffold: %v\n%s", err, output)
	}
	for _, file := range []struct{ fixture, output string }{
		{"fixtures/source-doc/onboard-excerpt.md", "cluster/onboard-excerpt.md"},
		{"fixtures/source-doc/known-behaviours-excerpt.md", "cluster/known-behaviours-excerpt.md"},
		{"fixtures/receipt/sveltos-oci-delivery-proof.yaml", "cluster/sveltos-oci-delivery-proof.yaml"},
	} {
		want, err := os.ReadFile(filepath.Join(caseRoot, file.fixture))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(workspace, file.output))
		if err != nil || string(got) != string(want) {
			t.Errorf("scaffold output %s differs from pinned fixture %s: %v", file.output, file.fixture, err)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertEvalRegexInPythonAndJS(t *testing.T, pattern string, good, bad []string) {
	t.Helper()
	cases, err := json.Marshal(struct {
		Good []string `json:"good"`
		Bad  []string `json:"bad"`
	}{good, bad})
	if err != nil {
		t.Fatal(err)
	}
	if python, err := exec.LookPath("python3"); err == nil {
		script := `import json,re,sys; p=re.compile(sys.argv[1], re.S); d=json.loads(sys.argv[2]); assert all(p.search(x) for x in d["good"]); assert not any(p.search(x) for x in d["bad"])`
		cmd := exec.Command(python, "-c", script, pattern, string(cases))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Python regex grader contract: %v\n%s", err, out)
		}
	} else {
		t.Skip("python3 is required for the Python grader-engine contract")
	}
	if node, err := exec.LookPath("node"); err == nil {
		script := `const p=new RegExp(process.argv[1],"s");const d=JSON.parse(process.argv[2]);if(!d.good.every(x=>p.test(x))||d.bad.some(x=>p.test(x)))process.exit(1);`
		cmd := exec.Command(node, "-e", script, pattern, string(cases))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("JavaScript regex grader contract: %v\n%s", err, out)
		}
	} else {
		t.Log("node unavailable: paired JavaScript grader-engine contract not run")
	}
}
