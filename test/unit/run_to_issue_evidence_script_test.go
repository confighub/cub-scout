package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunToIssueEvidenceScript_GeneratesSanitizedDraft(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	scriptRel := filepath.Join("scripts", "run-to-issue-evidence.sh")
	if _, err := os.Stat(filepath.Join(repoRoot, scriptRel)); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "failed-session.txt")
	outPath := filepath.Join(tmpDir, "issue-draft.md")
	transcript := strings.Join([]string{
		"$ ./cub-scout import --dry-run -n payments",
		"2026-03-06T20:00:00Z ERROR auth failed token github_pat_12345abcdef",
		"path=/tmp/session-123/output.log",
		"expected follow-up command unavailable",
	}, "\n")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	cmd := exec.Command("bash", scriptRel,
		"--title", "Capability gap: test",
		"--goal", "Capture issue-ready evidence",
		"--expected", "Generate reproducible issue draft",
		"--impact", "Blocks AI-assisted triage",
		"--transcript", transcriptPath,
		"--output", outPath,
	)
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}

	draft, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read draft: %v", err)
	}
	body := string(draft)
	if !strings.Contains(body, "## User Goal") {
		t.Fatalf("draft missing User Goal section:\n%s", body)
	}
	if !strings.Contains(body, "## Commands Attempted") {
		t.Fatalf("draft missing Commands Attempted section:\n%s", body)
	}
	if !strings.Contains(body, "./cub-scout import --dry-run -n payments") {
		t.Fatalf("draft missing extracted command:\n%s", body)
	}
	if strings.Contains(body, "github_pat_12345abcdef") {
		t.Fatalf("draft leaked token:\n%s", body)
	}
	if !strings.Contains(body, "<REDACTED_TOKEN>") {
		t.Fatalf("draft missing token redaction marker:\n%s", body)
	}
	if strings.Contains(body, "/tmp/session-123/output.log") {
		t.Fatalf("draft leaked temp path:\n%s", body)
	}
	if !strings.Contains(body, "<TEMP_PATH>") {
		t.Fatalf("draft missing temp-path redaction marker:\n%s", body)
	}
}

func TestRunToIssueEvidenceScript_OpenCallsGh(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	scriptRel := filepath.Join("scripts", "run-to-issue-evidence.sh")
	if _, err := os.Stat(filepath.Join(repoRoot, scriptRel)); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	tmpDir := t.TempDir()
	transcriptPath := filepath.Join(tmpDir, "failed-session.txt")
	outPath := filepath.Join(tmpDir, "issue-draft.md")
	argsLog := filepath.Join(tmpDir, "gh-args.log")
	bodyCopy := filepath.Join(tmpDir, "gh-body.md")

	if err := os.WriteFile(transcriptPath, []byte("$ ./cub-scout map list\nerror: demo failure\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	mockBinDir := t.TempDir()
	writeExecutableRunToIssue(t, filepath.Join(mockBinDir, "gh"), `#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> "$MOCK_GH_ARGS"
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--body-file" ]]; then
    cp "$2" "$MOCK_GH_BODY"
    break
  fi
  shift
done
echo "https://github.com/confighub/cub-scout/issues/999"
`)

	cmd := exec.Command("bash", scriptRel,
		"--title", "Capability gap: open",
		"--goal", "Capture and open",
		"--expected", "Issue created",
		"--impact", "Demo blocked",
		"--transcript", transcriptPath,
		"--output", outPath,
		"--open",
		"--repo", "confighub/cub-scout",
	)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"MOCK_GH_ARGS="+argsLog,
		"MOCK_GH_BODY="+bodyCopy,
		"PATH="+mockBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, string(out))
	}

	argsRaw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read gh args log: %v", err)
	}
	args := string(argsRaw)
	if !strings.Contains(args, "issue create") {
		t.Fatalf("gh args missing issue create:\n%s", args)
	}
	if !strings.Contains(args, "--template ai-capability-gap.yml") {
		t.Fatalf("gh args missing template:\n%s", args)
	}
	if !strings.Contains(args, "-R confighub/cub-scout") {
		t.Fatalf("gh args missing repo:\n%s", args)
	}

	bodyRaw, err := os.ReadFile(bodyCopy)
	if err != nil {
		t.Fatalf("read gh body copy: %v", err)
	}
	body := string(bodyRaw)
	if !strings.Contains(body, "## Evidence") {
		t.Fatalf("body missing evidence section:\n%s", body)
	}
	if !strings.Contains(string(out), "https://github.com/confighub/cub-scout/issues/999") {
		t.Fatalf("stdout missing mocked issue url:\n%s", string(out))
	}
}

func TestRunToIssueEvidenceScript_RequiresFlags(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	scriptRel := filepath.Join("scripts", "run-to-issue-evidence.sh")
	if _, err := os.Stat(filepath.Join(repoRoot, scriptRel)); err != nil {
		t.Fatalf("script missing: %v", err)
	}

	cmd := exec.Command("bash", scriptRel, "--title", "x")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected script to fail on missing required flags:\n%s", string(out))
	}
	if !strings.Contains(string(out), "Usage:") {
		t.Fatalf("expected usage output, got:\n%s", string(out))
	}
}

func writeExecutableRunToIssue(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod %s: %v", path, err)
	}
}
