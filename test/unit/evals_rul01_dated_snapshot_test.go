package unit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRUL01DatedSnapshotScaffold(t *testing.T) {
	checkRUL01DatedSnapshotScaffold(t, filepath.Join("..", "..", "evals", "rul01-dated-snapshot"))
}

func TestRUL01ManifestIsPreparedButUnrun(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "evals", "benchmark-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Status    string `json:"status"`
		Execution struct {
			Paid bool `json:"paid"`
		} `json:"execution"`
		Groups []struct {
			Cases []struct {
				ID           string `json:"id"`
				Status       string `json:"status"`
				ExistingCase string `json:"existing_case"`
			} `json:"cases"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "frozen_design_not_executable" || manifest.Execution.Paid {
		t.Fatalf("benchmark gate changed: %+v", manifest)
	}
	for _, group := range manifest.Groups {
		for _, c := range group.Cases {
			if c.ID == "RUL-01" {
				if c.Status != "raw_recording_prepared_not_run" || c.ExistingCase != "evals/rul01-dated-snapshot" {
					t.Fatalf("unexpected RUL-01 mapping: %+v", c)
				}
				return
			}
		}
	}
	t.Fatal("RUL-01 mapping missing")
}

func checkRUL01DatedSnapshotScaffold(t *testing.T, root string) {
	t.Helper()
	const rawHash = "f264204dc296d06590bc357691a1b95db08222f4ac68b3c38935d6ef200832c3"
	var baseline map[string][]byte
	for arm := 0; arm < 2; arm++ {
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
		entries, err := os.ReadDir(filepath.Join(workspace, "cluster"))
		if err != nil || len(entries) != 3 {
			t.Fatalf("arm %d staged %d files: %v", arm, len(entries), err)
		}
		current := make(map[string][]byte, len(entries))
		for _, entry := range entries {
			name := entry.Name()
			got, err := os.ReadFile(filepath.Join(workspace, "cluster", name))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(root, "fixtures", name))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("arm %d changed %s: %v", arm, name, err)
			}
			current[name] = got
		}
		sum := sha256.Sum256(current["after-pod.json"])
		if hex.EncodeToString(sum[:]) != rawHash {
			t.Fatalf("recorded Pod hash changed: %x", sum)
		}
		if arm == 0 {
			baseline = current
		} else {
			for name, got := range current {
				if !bytes.Equal(got, baseline[name]) {
					t.Fatalf("arms differ for %s", name)
				}
			}
		}
	}
	var receipt struct {
		Source struct {
			RawBytes int    `json:"rawBytes"`
			RawSHA   string `json:"rawSha256"`
		} `json:"source"`
		Request struct {
			Started   string  `json:"startedAt"`
			Ended     string  `json:"endedAt"`
			Precision int     `json:"loggedTimestampPrecisionSeconds"`
			Elapsed   float64 `json:"elapsedSeconds"`
		} `json:"request"`
	}
	if err := json.Unmarshal(baseline["capture-time-receipt.json"], &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Source.RawBytes != 5355 || receipt.Source.RawSHA != rawHash || receipt.Request.Started != "2026-10-01T06:03:25Z" || receipt.Request.Ended != "2026-10-01T06:03:25Z" || receipt.Request.Precision != 1 || receipt.Request.Elapsed <= 0 {
		t.Fatalf("receipt is not bound to reviewed one-second request evidence: %+v", receipt)
	}
}
