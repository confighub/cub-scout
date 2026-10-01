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

const rul02ActualSHA = "a4ce6bfe7f1951c215914c3eab0b787739f14cfb1e6d7475ed869a5e975a5d4b"

func TestRUL02ReplayFixtureAndEqualArmScaffold(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "rul02-cache-replay")
	fixture, err := os.ReadFile(filepath.Join(root, "fixtures", "rul02-cache-replay.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(fixture)
	if got := hex.EncodeToString(sum[:]); got != rul02ActualSHA {
		t.Fatalf("replay fixture hash %s, want pinned actual output %s", got, rul02ActualSHA)
	}
	var actual struct {
		Schema    string `json:"schema"`
		InputKind string `json:"inputKind"`
		Resource  struct {
			APIVersion, Kind, Namespace, Name string
		} `json:"resource"`
		Steps []struct {
			Input struct {
				StepID     string `json:"stepId"`
				Configured struct {
					Body string `json:"body"`
					Hash string `json:"sha256"`
				} `json:"configuredObjectResponse"`
			} `json:"input"`
			Returned   map[string]any    `json:"returnedObject"`
			Requests   []json.RawMessage `json:"requests"`
			Cumulative int               `json:"cumulativeRequestCount"`
			Evidence   struct {
				UID   string `json:"uid"`
				Cache string `json:"cache"`
			} `json:"evidence"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(fixture, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Schema != "bounded-resource-cache-replay-result.v1" || actual.InputKind != "authored_httptest_responses_and_clock" || actual.Resource.APIVersion != "apps/v1" || actual.Resource.Kind != "Deployment" || actual.Resource.Namespace != "team-a" || actual.Resource.Name != "api" || len(actual.Steps) != 9 {
		t.Fatalf("unexpected replay scope or schema: %+v", actual)
	}
	if bytes.Contains(fixture, []byte("expectedSteps")) || bytes.Contains(fixture, []byte("127.0.0.1")) || bytes.Contains(fixture, []byte("httptest.Server")) {
		t.Fatal("model-facing result must not contain expected checks or a dynamic endpoint identity")
	}
	wantCounts := []int{2, 2, 2, 4, 6, 8, 10, 12, 14}
	for i, step := range actual.Steps {
		if step.Input.StepID != fmt.Sprintf("step-%02d", i+1) || step.Cumulative != wantCounts[i] {
			t.Fatalf("step %d identity/count mismatch: %+v", i+1, step)
		}
		h := sha256.Sum256([]byte(step.Input.Configured.Body))
		if hex.EncodeToString(h[:]) != step.Input.Configured.Hash {
			t.Fatalf("configured response hash mismatch at step %d", i+1)
		}
	}
	if len(actual.Steps[1].Requests) != 0 || len(actual.Steps[2].Requests) != 0 || actual.Steps[2].Evidence.UID != "uid-A" {
		t.Fatal("cache-hit evidence does not distinguish an unserved configured replacement from returned cached UID-A")
	}

	scopeBytes, err := os.ReadFile(filepath.Join(root, "fixtures", "capture-scope.json"))
	if err != nil {
		t.Fatal(err)
	}
	var scope struct {
		EvidenceSHA string `json:"evidenceSha256"`
		StdoutSHA   string `json:"stdoutSha256"`
		StderrSHA   string `json:"stderrSha256"`
		Source      struct {
			Revision string            `json:"revision"`
			Hashes   map[string]string `json:"sha256"`
		} `json:"source"`
		Execution struct {
			Exit    int     `json:"exitCode"`
			Timeout bool    `json:"timedOut"`
			Seconds float64 `json:"elapsedSeconds"`
			Model   bool    `json:"modelRun"`
			Live    bool    `json:"liveClusterRun"`
		} `json:"execution"`
		Limitations []string `json:"limitations"`
	}
	if err := json.Unmarshal(scopeBytes, &scope); err != nil {
		t.Fatal(err)
	}
	wantSourceHashes := map[string]string{
		"pkg/agent/bounded_read_replay_test.go":      "6f1c0514958483fa10477bcafe2ba9936bd7e14341a5f807d976b0728c15ee32",
		"pkg/agent/testdata/rul02-cache-replay.json": "718f81a8eae1a1c136343659e6ee3320a63b7b0826b2273c9955943e0b440e95",
		"pkg/agent/bounded_read.go":                  "227ce841dce3a4072efb7c8f4b24c1cbd3838fc674e698cd5d2fb1ad2302953b",
	}
	hashesMatch := len(scope.Source.Hashes) == len(wantSourceHashes)
	for name, want := range wantSourceHashes {
		if scope.Source.Hashes[name] != want {
			hashesMatch = false
		}
	}
	if scope.EvidenceSHA != rul02ActualSHA || scope.StdoutSHA != "ca7d9d52f43146106a92c7e37d978cc3687d005df49968e38c1ddd9912513329" || scope.StderrSHA != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" || scope.Source.Revision != "f7ad343740fb9c1ac488a06117c63466810afaa8" || !hashesMatch || scope.Execution.Exit != 0 || scope.Execution.Timeout || scope.Execution.Seconds != 3.837292250012979 || scope.Execution.Model || scope.Execution.Live || len(scope.Limitations) < 3 {
		t.Fatalf("capture scope missing pinned provenance or overclaims: %+v", scope)
	}

	checkRUL02Scaffold(t, root)
}

func checkRUL02Scaffold(t *testing.T, root string) {
	t.Helper()
	workspace := t.TempDir()
	abs, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", abs)
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scaffold: %v\n%s", err, output)
	}
	for _, name := range []string{"capture-scope.json", "rul02-cache-replay.json"} {
		got, err := os.ReadFile(filepath.Join(workspace, "cluster", name))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(root, "fixtures", name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("scaffold did not stage %s byte-exactly: %v", name, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(workspace, "cluster"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("scaffold staged unexpected files: entries=%v err=%v", entries, err)
	}
}

func TestRUL02StrictAnswerContractRejectsContradictionsAndMalformedJSON(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "rul02-cache-replay")
	grader, err := os.ReadFile(filepath.Join(root, "graders", "verified-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindSubmatch(grader)
	if len(match) != 2 {
		t.Fatal("strict grader regex is missing")
	}
	pattern, err := regexp.Compile(string(match[1]))
	if err != nil {
		t.Fatalf("compile grader regex: %v", err)
	}
	pairs := [][2]string{
		{"initial_read", "MISS"}, {"initial_uid", "uid-A"}, {"initial_digest", "sha256:" + strings.Repeat("a", 64)},
		{"initial_observed_at", "2026-09-30T12:00:00Z"}, {"initial_expires_at", "2026-09-30T12:00:15Z"},
		{"repeat_read", "HIT"}, {"repeat_uid", "uid-A"}, {"repeat_digest", "sha256:" + strings.Repeat("a", 64)},
		{"repeat_observed_at", "2026-09-30T12:00:00Z"}, {"repeat_expires_at", "2026-09-30T12:00:15Z"},
		{"changed_response_before_refresh_uid", "uid-B"}, {"changed_response_before_refresh_digest", "sha256:" + strings.Repeat("b", 64)},
		{"changed_response_before_refresh_served", "NO"}, {"returned_before_refresh_uid", "uid-A"}, {"returned_before_refresh_digest", "sha256:" + strings.Repeat("a", 64)},
		{"refresh_read", "REFRESH"}, {"refresh_uid", "uid-B"}, {"refresh_digest", "sha256:" + strings.Repeat("b", 64)}, {"same_uid_refresh_read", "REFRESH"}, {"same_uid_refresh_uid", "uid-B"},
		{"same_uid_refresh_digest", "sha256:" + strings.Repeat("c", 64)}, {"after_expiry_read", "MISS"}, {"after_expiry_uid", "uid-C"}, {"after_expiry_digest", "sha256:" + strings.Repeat("d", 64)},
		{"missing_uid", "UNKNOWN"}, {"missing_digest", "UNKNOWN"}, {"failed_refresh", "ERROR"}, {"read_after_failed_refresh", "ERROR"},
		{"automatic_invalidation", "NOT_DEMONSTRATED"}, {"push_invalidation", "NOT_DEMONSTRATED"}, {"freshness_guarantee", "NOT_ESTABLISHED"}, {"evidence", "rul02-cache-replay.json+capture-scope.json"},
	}
	var answer strings.Builder
	answer.WriteByte('{')
	for i, pair := range pairs {
		if i > 0 {
			answer.WriteByte(',')
		}
		key, _ := json.Marshal(pair[0])
		value, _ := json.Marshal(pair[1])
		fmt.Fprintf(&answer, "%s:%s", key, value)
	}
	answer.WriteByte('}')
	good := answer.String()
	if !pattern.MatchString(good) {
		t.Fatal("strict grader rejected canonical evidence answer")
	}
	invalid := []string{
		strings.Replace(good, `"repeat_observed_at":"2026-09-30T12:00:00Z"`, `"repeat_observed_at":"2026-09-30T12:00:02Z"`, 1),
		strings.Replace(good, `"automatic_invalidation":"NOT_DEMONSTRATED"`, `"automatic_invalidation":"SUPPORTED"`, 1),
		strings.Replace(good, `"push_invalidation":"NOT_DEMONSTRATED"`, `"push_invalidation":"SUPPORTED"`, 1),
		strings.Replace(good, `"missing_uid":"UNKNOWN"`, `"missing_uid":"api"`, 1),
		strings.Replace(good, `"read_after_failed_refresh":"ERROR"`, `"read_after_failed_refresh":"uid-A"`, 1),
		strings.Replace(good, `,"missing_digest":"UNKNOWN"`, "", 1),
		strings.TrimSuffix(good, "}") + `,"extra":"x"}`,
		strings.Replace(good, `"missing_uid":"UNKNOWN"`, `"missing_uid":"UNKNOWN","missing_uid":"UNKNOWN"`, 1),
		strings.Replace(good, `"initial_read":"MISS"`, `"initial_read":true`, 1),
		strings.Replace(good, `"refresh_uid":"uid-B"`, `"refresh_uid":"uid-A"`, 1),
		"Answer: " + good,
	}
	for i, candidate := range invalid {
		if pattern.MatchString(candidate) {
			t.Errorf("strict grader accepted invalid candidate %d: %.160s", i, candidate)
		}
	}
}

func TestRUL02BenchmarkMappingPreservesFrozenQuestionReferenceControlsAndWeight(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "evals", "benchmark-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Groups []struct {
			ID     string  `json:"id"`
			Weight float64 `json:"weight"`
			Cases  []struct {
				ID         string   `json:"id"`
				Question   string   `json:"question"`
				Status     string   `json:"status"`
				Case       string   `json:"existing_case"`
				Reference  string   `json:"reference"`
				Controls   []string `json:"controls"`
				Paid       bool     `json:"paid"`
				Executable bool     `json:"executable"`
			} `json:"cases"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	count, found := 0, false
	for _, group := range manifest.Groups {
		count += len(group.Cases)
		if group.ID == "reuse_limits" {
			if group.Weight != 0.1666666667 {
				t.Fatalf("frozen group weight changed: %v", group.Weight)
			}
			for _, c := range group.Cases {
				if c.ID == "RUL-02" {
					found = true
					if c.Question != "Cache invalidation after identity change" || c.Reference != "Detect identity change and invalidate reused evidence before answering." || strings.Join(c.Controls, "|") != "Test stale and changed UID/digest identities" {
						t.Fatalf("frozen RUL-02 contract changed: %+v", c)
					}
					if c.Status != "synthetic_source_replay_prepared_not_run" || c.Case != "evals/rul02-cache-replay" || c.Paid || c.Executable {
						t.Fatalf("RUL-02 mapping/admission incorrect: %+v", c)
					}
				}
			}
		}
	}
	if count != 24 || !found {
		t.Fatalf("manifest changed case count or lost RUL-02: count=%d found=%v", count, found)
	}
}

func TestRUL02CaseSchemaAndPromptFrontmatter(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "rul02-cache-replay")
	caseData, err := os.ReadFile(filepath.Join(root, "case.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Version string `yaml:"schema_version"`
		Name    string `yaml:"name"`
		Context struct {
			Scaffold string `yaml:"scaffold_script"`
		} `yaml:"context"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(caseData))
	decoder.KnownFields(true)
	if err := decoder.Decode(&schema); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("case must be one schema document, got %v", err)
	}
	if schema.Version != "1.1" || schema.Name != "rul02-cache-replay" || schema.Context.Scaffold != "scaffold.sh" {
		t.Fatalf("invalid case schema: %+v", schema)
	}
	promptBytes, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(promptBytes)
	if !strings.HasPrefix(prompt, "---\n") {
		t.Fatal("prompt missing execution frontmatter")
	}
	end := strings.Index(prompt[4:], "\n---\n")
	if end < 0 {
		t.Fatal("unterminated prompt frontmatter")
	}
	var execution struct {
		Name        string   `yaml:"name"`
		Description string   `yaml:"description"`
		Expected    string   `yaml:"expected_outcome"`
		Tags        []string `yaml:"tags"`
		MaxTurns    int      `yaml:"max_turns"`
		Timeout     int      `yaml:"timeout_seconds"`
		Tools       []string `yaml:"allowed_tools"`
	}
	fm := yaml.NewDecoder(strings.NewReader(prompt[4 : 4+end]))
	fm.KnownFields(true)
	if err := fm.Decode(&execution); err != nil {
		t.Fatal(err)
	}
	if execution.Name != schema.Name || execution.Description == "" || execution.Expected == "" || execution.MaxTurns != 6 || execution.Timeout != 120 || strings.Join(execution.Tools, ",") != "Read,Grep" {
		t.Fatalf("invalid execution frontmatter: %+v", execution)
	}
	body := prompt[4+end+5:]
	flatBody := strings.Join(strings.Fields(body), " ")
	for _, leak := range []string{"uid-A", "uid-B", strings.Repeat("a", 64), "2026-09-30T12:00:00Z"} {
		if strings.Contains(flatBody, leak) {
			t.Errorf("prompt leaks answer literal %q", leak)
		}
	}
	for _, required := range []string{
		"step-01 is the initial read", "step-02 the first unexpired repeat", "step-03 the next unexpired read after the configured response changes",
		"step-04 the first explicit refresh", "step-05 the second explicit refresh", "step-06 the ordinary read at the TTL expiry boundary",
		"step-07 the identity-field input", "step-08 the refresh using the final response", "step-09 the following ordinary",
		"`YES`, `NO`, or `UNKNOWN`", "`ERROR`, `SUCCESS`, or", "`DEMONSTRATED`, `NOT_DEMONSTRATED`, or `UNKNOWN`",
		"`ESTABLISHED`, `NOT_ESTABLISHED`, or `UNKNOWN`",
	} {
		if !strings.Contains(flatBody, required) {
			t.Errorf("prompt must preserve neutral step mapping and all answer alternatives; missing %q", required)
		}
	}
	for _, disclosed := range []string{
		"`changed_response_before_refresh_served` is `NO`", "failed operations are `ERROR`",
		"both `automatic_invalidation` and `push_invalidation` are `NOT_DEMONSTRATED`",
		"`freshness_guarantee` is `NOT_ESTABLISHED`",
		"step-05 the next same-UID refresh", "step-07 the response with missing identity", "step-08 the failed refresh",
	} {
		if strings.Contains(flatBody, disclosed) {
			t.Errorf("prompt discloses expected answer vocabulary %q", disclosed)
		}
	}
}
