package unit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEvalRegradeSavedTranscripts(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "test", "fixtures", "evals-regrade", "input", "result.json")
	before, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	sourceHash := sha256.Sum256(before)
	out := filepath.Join(t.TempDir(), "audit.json")
	cmd := exec.Command("python3", filepath.Join(root, "evals", "scripts", "regrade.py"), input, "--out", out)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("regrade: %v\n%s", err, output)
	}
	after, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("regrade modified its source result")
	}

	var audit struct {
		SourceSHA256   string         `json:"sourceSha256"`
		SourceParent   string         `json:"sourceParent"`
		SourceMetadata map[string]any `json:"sourceMetadata"`
		TraceRoots     map[string]any `json:"traceRoots"`
		Cases          []struct {
			Name         string `json:"name"`
			GraderHashes []struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
			} `json:"graderHashes"`
			Arms map[string][]struct {
				Outcome     string         `json:"outcome"`
				Reason      string         `json:"reason"`
				Spend       map[string]any `json:"spend"`
				TracePath   string         `json:"tracePath"`
				TraceSHA256 string         `json:"traceSha256"`
			} `json:"arms"`
		} `json:"cases"`
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if audit.SourceSHA256 != hex.EncodeToString(sourceHash[:]) {
		t.Fatalf("source hash = %q, want %x", audit.SourceSHA256, sourceHash)
	}
	if audit.SourceMetadata["partial"] != true || audit.SourceMetadata["status"] != "partial" ||
		audit.SourceMetadata["costUsd"] != 2.5 || audit.SourceMetadata["judgeCostUsd"] != 0.4 ||
		audit.SourceMetadata["config"] == nil {
		t.Errorf("top-level result metadata was not preserved: %#v", audit.SourceMetadata)
	}
	if audit.SourceParent == "" || audit.TraceRoots["default"] != audit.SourceParent {
		t.Errorf("input-parent provenance missing: parent=%q roots=%#v", audit.SourceParent, audit.TraceRoots)
	}
	if len(audit.Cases) != 3 {
		t.Fatalf("got %d cases, want 3", len(audit.Cases))
	}
	want := []string{"pass", "fail", "fail", "fail", "unknown", "unknown"}
	gotRuns := audit.Cases[0].Arms["with"]
	if len(gotRuns) != len(want) {
		t.Fatalf("got %d with-runs, want %d", len(gotRuns), len(want))
	}
	tracePath := filepath.Join(root, "test", "fixtures", "evals-regrade", "input", "traces", "good.jsonl")
	traceBytes, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	traceHash := sha256.Sum256(traceBytes)
	if gotRuns[0].TracePath != tracePath || gotRuns[0].TraceSHA256 != hex.EncodeToString(traceHash[:]) {
		t.Errorf("resolved trace evidence missing: path=%q hash=%q", gotRuns[0].TracePath, gotRuns[0].TraceSHA256)
	}
	for i, outcome := range want {
		if gotRuns[i].Outcome != outcome {
			t.Errorf("with run %d outcome = %q, want %q (%s)", i, gotRuns[i].Outcome, outcome, gotRuns[i].Reason)
		}
	}
	if got := audit.Cases[0].Arms["without"][0].Outcome; got != "pass" {
		t.Errorf("tool_used grader changed correctness outcome: got %q, want pass", got)
	}
	if got := audit.Cases[1].Arms["with"][0].Outcome; got != "unknown" {
		t.Errorf("unsupported grader outcome = %q, want unknown", got)
	}
	unsupportedPath := filepath.Join(root, "test", "fixtures", "evals-regrade", "cases", "unsupported", "graders", "unsupported.md")
	unsupportedBytes, err := os.ReadFile(unsupportedPath)
	if err != nil {
		t.Fatal(err)
	}
	unsupportedHash := sha256.Sum256(unsupportedBytes)
	if hashes := audit.Cases[1].GraderHashes; len(hashes) != 1 ||
		hashes[0].Path != "test/fixtures/evals-regrade/cases/unsupported/graders/unsupported.md" ||
		hashes[0].SHA256 != hex.EncodeToString(unsupportedHash[:]) {
		t.Errorf("unsupported grader provenance was lost: %+v", hashes)
	}
	if got := audit.Cases[2].Arms["with"][0].Outcome; got != "unknown" {
		t.Errorf("escaped case directory outcome = %q, want unknown", got)
	}
	if hashes := audit.Cases[2].GraderHashes; len(hashes) != 0 {
		t.Errorf("malformed/escaped case invented grader hashes: %+v", hashes)
	}
	if got := gotRuns[5].Outcome; got != "unknown" {
		t.Errorf("trace outside default input root outcome = %q, want unknown", got)
	}
	if len(audit.Cases[0].GraderHashes) != 2 || audit.Cases[0].GraderHashes[0].SHA256 == "" {
		t.Errorf("current grader hashes missing: %+v", audit.Cases[0].GraderHashes)
	}
	spend := gotRuns[0].Spend
	runSpend, ok := spend["run"].(map[string]any)
	if !ok || runSpend["costUsd"] != 0.12 || runSpend["judgeCostUsd"] != 0.03 || runSpend["mocks"] == nil {
		t.Errorf("agent/judge/mock spend not retained: %#v", spend)
	}
	terminal, ok := spend["terminalResult"].(map[string]any)
	if !ok || terminal["model"] != "claude-test" || terminal["usage"] == nil || terminal["total_cost_usd"] != 0.005 {
		t.Errorf("terminal model/token/cost metadata not retained: %#v", spend)
	}

	// Existing outputs are never truncated or replaced.
	if err := os.WriteFile(out, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("python3", filepath.Join(root, "evals", "scripts", "regrade.py"), input, "--out", out)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("regrade unexpectedly replaced existing output: %s", output)
	}
	kept, err := os.ReadFile(out)
	if err != nil || string(kept) != "keep" {
		t.Fatalf("existing output changed: %q, %v", kept, err)
	}

	// An operator can explicitly authorize a separate saved-trace directory.
	allowedOut := filepath.Join(t.TempDir(), "audit-with-trace-root.json")
	traceRoot := filepath.Join(root, "test", "fixtures", "evals-regrade")
	cmd = exec.Command("python3", filepath.Join(root, "evals", "scripts", "regrade.py"), input,
		"--trace-root", traceRoot, "--out", allowedOut)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("regrade with explicit trace root: %v\n%s", err, output)
	}
	allowedData, err := os.ReadFile(allowedOut)
	if err != nil {
		t.Fatal(err)
	}
	var allowed struct {
		TraceRoots map[string]any `json:"traceRoots"`
		Cases      []struct {
			Arms map[string][]struct {
				Outcome string `json:"outcome"`
			} `json:"arms"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(allowedData, &allowed); err != nil {
		t.Fatal(err)
	}
	if got := allowed.Cases[0].Arms["with"][5].Outcome; got != "pass" {
		t.Errorf("trace within explicit root outcome = %q, want pass", got)
	}
	explicitRoots, ok := allowed.TraceRoots["explicit"].([]any)
	if !ok || len(explicitRoots) != 1 || explicitRoots[0] != traceRoot {
		t.Errorf("selected explicit trace root provenance missing: %#v", allowed.TraceRoots)
	}
}
