// Producer fixture suite for the source-truth contract — v0.1 of #395.
//
// Each fixture lives in test/fixtures/source-truth/<case-name>/ and
// contains:
//
//   surfaces.json  — the SourceTruthSurfaces input shape
//   strategy.txt   — single line, the strategy enum value
//                    (e.g. "confighub-oci-flux"); empty file means
//                    "no strategy declared", which exercises ASK
//   expected.json  — the SourceTruthEvidence the contract should emit
//
// The test walks every fixture directory, calls agent.Derive() with the
// inputs, and asserts byte-equal JSON against the expected file.
//
// Why fixtures on disk in addition to the inline table tests in
// source_truth_logic_test.go: the file shape is the artifact the
// confighub-ai-demo#264 consumer-side fixtures pair against. Pilot's
// acceptance kernel forks these JSON shapes; keeping them as files
// (not Go literals) makes that pairing reviewable across the two
// repositories.
//
// Adding a fixture: drop a new directory under test/fixtures/source-
// truth/ and re-run the test. The runner discovers fixtures
// automatically.

package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sourceTruthFixtureRoot = "../../test/fixtures/source-truth"

func TestSourceTruthFixtures(t *testing.T) {
	entries, err := os.ReadDir(sourceTruthFixtureRoot)
	if err != nil {
		t.Fatalf("read fixture root: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no fixtures found under %s", sourceTruthFixtureRoot)
	}

	caseCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		caseCount++
		caseName := entry.Name()
		caseDir := filepath.Join(sourceTruthFixtureRoot, caseName)
		t.Run(caseName, func(t *testing.T) {
			runFixtureCase(t, caseDir)
		})
	}
	if caseCount == 0 {
		t.Fatalf("no fixture directories found under %s", sourceTruthFixtureRoot)
	}
}

func runFixtureCase(t *testing.T, dir string) {
	t.Helper()

	// Strategy: empty file is valid — represents "no strategy
	// declared", which exercises the ASK / UNKNOWN path.
	strategyPath := filepath.Join(dir, "strategy.txt")
	strategyRaw, err := os.ReadFile(strategyPath)
	if err != nil {
		t.Fatalf("read %s: %v", strategyPath, err)
	}
	strategyStr := strings.TrimSpace(string(strategyRaw))
	var strategy SourceTruthStrategy
	if strategyStr != "" {
		parsed, ok := ParseStrategy(strategyStr)
		if !ok {
			t.Fatalf("strategy.txt holds %q which is not a recognised strategy", strategyStr)
		}
		strategy = parsed
	}

	// Surfaces: required input.
	surfacesPath := filepath.Join(dir, "surfaces.json")
	surfacesRaw, err := os.ReadFile(surfacesPath)
	if err != nil {
		t.Fatalf("read %s: %v", surfacesPath, err)
	}
	var surfaces SourceTruthSurfaces
	if err := json.Unmarshal(surfacesRaw, &surfaces); err != nil {
		t.Fatalf("parse %s: %v", surfacesPath, err)
	}

	// Expected: the JSON shape the contract should emit.
	expectedPath := filepath.Join(dir, "expected.json")
	expectedRaw, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read %s: %v", expectedPath, err)
	}

	got := Derive(strategy, surfaces)

	// Use the same encoder the CLI command uses so fixtures assert the
	// exact wire shape Pilot consumes.
	var buf bytes.Buffer
	if err := EncodeEvidence(&buf, got); err != nil {
		t.Fatalf("encode Derive output: %v", err)
	}
	gotJSON := buf.Bytes()

	if string(gotJSON) != string(expectedRaw) {
		t.Errorf("byte-equality regression for fixture %s\n--- expected ---\n%s\n--- got ---\n%s",
			filepath.Base(dir),
			string(expectedRaw),
			string(gotJSON),
		)
	}
}
