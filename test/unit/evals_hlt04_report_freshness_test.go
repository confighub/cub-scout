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

const hlt04CaseRoot = "../../evals/sveltos-hlt-04-report-freshness"

var hlt04FixtureHashes = map[string]string{
	"scenario-evidence.json":                 "e1ae4cb9cbdaa26488d1ed1b3e920e9cc092703e560cffef83f3a845fe9c9b7c",
	"source-metadata.json":                   "a224a85f89ca16cdbe6c0e8734ade6e4503e81573d16fa71fc60b4abd71ea1fb",
	"synthetic-check-execution-receipt.json": "1352073e9000d280a92b440d1c9789f6fd987af099e04f4b7638804a48ad0e06",
}

type hlt04SourceMetadata struct {
	Schema                      string                       `json:"schema"`
	SourceRepository            string                       `json:"source_repository"`
	SourceRevision              string                       `json:"source_revision"`
	SourceFileSHA256            map[string]string            `json:"source_file_sha256"`
	ProducerInputSHA256ByRecord map[string]map[string]string `json:"producer_input_sha256_by_record"`
	Replay                      map[string]any               `json:"replay"`
	CapturedOutputSHA256        map[string]string            `json:"captured_output_sha256"`
	RecordCount                 int                          `json:"record_count"`
	ReceiptFile                 string                       `json:"receipt_file"`
	ProducerInputArtifacts      []string                     `json:"producer_input_artifacts"`
	OtherFixtureArtifacts       []string                     `json:"other_authored_fixture_artifacts"`
	SyntheticReceiptSHA256      string                       `json:"synthetic_check_receipt_sha256"`
}

type hlt04ScenarioEvidence struct {
	Schema  string `json:"schema"`
	Records []struct {
		ID             string                     `json:"id"`
		SyntheticNow   string                     `json:"synthetic_now"`
		RawInputs      map[string]json.RawMessage `json:"raw_inputs"`
		HeldBefore     string                     `json:"held_before"`
		HeldAfter      string                     `json:"held_after"`
		ComputedReport json.RawMessage            `json:"computed_report"`
		WritePatch     *string                    `json:"write_patch"`
	} `json:"records"`
}

type hlt04AnswerRow struct {
	ID                   string `json:"id"`
	HeldBeforeObservedAt string `json:"held_before_observed_at"`
	HeldAfterObservedAt  string `json:"held_after_observed_at"`
	ComputedObservedAt   string `json:"computed_observed_at"`
	WriteApplied         string `json:"write_applied"`
	HealthStatus         string `json:"health_status"`
	SyncStatus           string `json:"sync_status"`
	Revision             string `json:"revision"`
}

type hlt04Answer struct {
	Records                        []hlt04AnswerRow `json:"records"`
	CheckReceipt                   string           `json:"check_receipt"`
	CheckReceiptConsumed           string           `json:"check_receipt_consumed"`
	LastTransitionTimeRole         string           `json:"last_transition_time_role"`
	AppliedReleaseDigestProven     string           `json:"applied_release_digest_proven"`
	EvidenceScope                  string           `json:"evidence_scope"`
	RenewalProvesNewCheck          string           `json:"renewal_proves_new_check"`
	FutureTimestampProvesFreshness string           `json:"future_timestamp_proves_freshness"`
}

func TestHLT04FixtureHashesSchemaAndStrictAnswer(t *testing.T) {
	root := filepath.Clean(hlt04CaseRoot)
	files := map[string][]byte{}
	for name, want := range hlt04FixtureHashes {
		data, err := os.ReadFile(filepath.Join(root, "fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(data)
		if got := hex.EncodeToString(h[:]); got != want {
			t.Fatalf("fixture %s sha256=%s want %s", name, got, want)
		}
		files[name] = data
	}
	var metadata hlt04SourceMetadata
	if err := json.Unmarshal(files["source-metadata.json"], &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Schema != "hlt04-synthetic-source-metadata.v1" || metadata.SourceRepository != "confighub/sveltos-confighub" || metadata.SourceRevision != "8187910f9fe226e109e55c4d9c7c0e21297ff424" || len(metadata.SourceFileSHA256) != 35 || metadata.RecordCount != 8 || metadata.ReceiptFile != "synthetic-check-execution-receipt.json" ||
		strings.Join(metadata.ProducerInputArtifacts, ",") != "clusterprofiles,clustersummaries,clusterhealthchecks,published_releases" ||
		strings.Join(metadata.OtherFixtureArtifacts, ",") != "synthetic-check-execution-receipt.json" || metadata.SyntheticReceiptSHA256 != "1352073e9000d280a92b440d1c9789f6fd987af099e04f4b7638804a48ad0e06" {
		t.Fatalf("source metadata is incomplete or unexpected: %+v", metadata)
	}
	if metadata.Replay["status"] != "passed" || metadata.Replay["producer_test_executed"] != true || metadata.Replay["owned_process_group_cleanup_confirmed"] != true || metadata.Replay["parent_reaped"] != true {
		t.Fatalf("source replay provenance lacks completed cleanup facts: %+v", metadata.Replay)
	}
	if metadata.Replay["helper_revision"] != "6a13aeea8809d486513363293b4775a5588f9399" || metadata.Replay["helper_sha256"] != "c52554e9b7da0e3f9ff91b1049da24380b815c56ee7ebb5cd547327a6abd3b62" {
		t.Fatal("fixture does not pin the helper used by the retained replay")
	}
	var evidence hlt04ScenarioEvidence
	if err := json.Unmarshal(files["scenario-evidence.json"], &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.Schema != "hlt04-scenario-evidence.v1" || len(evidence.Records) != 8 || len(metadata.ProducerInputSHA256ByRecord) != len(evidence.Records) {
		t.Fatalf("scenario inventory mismatch: schema=%q records=%d", evidence.Schema, len(evidence.Records))
	}
	var receipt struct {
		Kind        string `json:"kind"`
		Synthetic   bool   `json:"synthetic"`
		Source      string `json:"source"`
		CompletedAt string `json:"completedAt"`
	}
	if err := json.Unmarshal(files["synthetic-check-execution-receipt.json"], &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Kind != "synthetic-check-execution" || !receipt.Synthetic || receipt.Source != "authored fixture; not producer input" || receipt.CompletedAt != "2026-10-01T12:05:00Z" {
		t.Fatalf("separate authored check receipt changed: %+v", receipt)
	}
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
		t.Fatalf("case.yaml has extra document: %v", err)
	}
	if schema.Version != "1.1" || schema.Name != "sveltos-hlt-04-report-freshness" || schema.Context.Scaffold != "scaffold.sh" {
		t.Fatalf("case schema invalid: %+v", schema)
	}
	prompt, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	promptText := string(prompt)
	for _, required := range []string{"minified, single-line JSON", "UNKNOWN", "renewal_proves_new_check", "future_timestamp_proves_freshness", "CONTROLLER_EXECUTION_RECEIPT", "CONDITION_TRANSITION_NOT_CHECK_EXECUTION", "MIXED_EVIDENCE"} {
		if !strings.Contains(promptText, required) {
			t.Errorf("prompt is missing answer-format/vocabulary definition %q", required)
		}
	}
	if !strings.HasPrefix(promptText, "---\n") {
		t.Fatal("prompt frontmatter missing")
	}
	frontEnd := strings.Index(promptText[4:], "\n---\n")
	if frontEnd < 0 {
		t.Fatal("prompt frontmatter malformed")
	}
	var execution struct {
		Name     string   `yaml:"name"`
		MaxTurns int      `yaml:"max_turns"`
		Timeout  int      `yaml:"timeout_seconds"`
		Tools    []string `yaml:"allowed_tools"`
	}
	execDecoder := yaml.NewDecoder(strings.NewReader(promptText[4 : 4+frontEnd]))
	execDecoder.KnownFields(false)
	if err := execDecoder.Decode(&execution); err != nil {
		t.Fatal(err)
	}
	if execution.Name != schema.Name || execution.MaxTurns != 6 || execution.Timeout != 120 || strings.Join(execution.Tools, ",") != "Read,Grep" {
		t.Fatalf("prompt execution schema invalid: %+v", execution)
	}
	answer := hlt04Answer{Records: make([]hlt04AnswerRow, 0, len(evidence.Records)),
		CheckReceipt: "AUTHORED_SYNTHETIC_NOT_CONTROLLER_EVIDENCE", CheckReceiptConsumed: "NO",
		LastTransitionTimeRole:     "CONDITION_TRANSITION_NOT_CHECK_EXECUTION",
		AppliedReleaseDigestProven: "NO", EvidenceScope: "SYNTHETIC_SOURCE_CONTRACT_ONLY",
		RenewalProvesNewCheck: "NO", FutureTimestampProvesFreshness: "NO"}
	for i, record := range evidence.Records {
		wantID := "R" + string(rune('0'+(i+1)/10)) + string(rune('0'+(i+1)%10))
		if record.ID != wantID || len(record.RawInputs) != 4 || record.SyntheticNow == "" || len(record.ComputedReport) == 0 {
			t.Fatalf("record %d lacks exact source evidence: %+v", i, record)
		}
		if _, ok := metadata.ProducerInputSHA256ByRecord[record.ID]; !ok {
			t.Fatalf("missing source-input hashes for %s", record.ID)
		}
		for name, raw := range record.RawInputs {
			var decoded any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("%s/%s invalid JSON: %v", record.ID, name, err)
			}
			canonical, err := json.Marshal(decoded)
			if err != nil {
				t.Fatal(err)
			}
			h := sha256.Sum256(canonical)
			if got, want := hex.EncodeToString(h[:]), metadata.ProducerInputSHA256ByRecord[record.ID][name]; got != want {
				t.Fatalf("%s/%s input sha256=%s want %s", record.ID, name, got, want)
			}
		}
		var heldBefore, heldAfter struct {
			ObservedAt string `json:"observedAt"`
		}
		_ = json.Unmarshal([]byte(record.HeldBefore), &heldBefore)
		_ = json.Unmarshal([]byte(record.HeldAfter), &heldAfter)
		var computed struct {
			ObservedAt string `json:"observedAt"`
			Health     string `json:"healthStatus"`
			Sync       string `json:"syncStatus"`
			Revision   string `json:"revision"`
		}
		if err := json.Unmarshal(record.ComputedReport, &computed); err != nil {
			t.Fatal(err)
		}
		write := "NO"
		if record.WritePatch != nil {
			write = "YES"
		}
		before := heldBefore.ObservedAt
		if record.HeldBefore == "" || (before == "" && !strings.Contains(record.HeldBefore, `"observedAt"`)) {
			before = "MISSING"
		}
		after := heldAfter.ObservedAt
		if after == "" && !strings.Contains(record.HeldAfter, `"observedAt"`) {
			after = "MISSING"
		}
		answer.Records = append(answer.Records, hlt04AnswerRow{record.ID, before, after, computed.ObservedAt, write, computed.Health, computed.Sync, computed.Revision})
	}
	grader, err := os.ReadFile(filepath.Join(root, "graders", "verified-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '(.*)'$`).FindSubmatch(grader)
	if len(match) != 2 {
		t.Fatal("strict HLT-04 grader has no pattern")
	}
	pattern, err := regexp.Compile(string(match[1]))
	if err != nil {
		t.Fatalf("strict HLT-04 grader regex is invalid: %v", err)
	}
	good, err := json.Marshal(answer)
	if err != nil || !pattern.Match(good) {
		t.Fatalf("grader rejected the source-derived answer: err=%v answer=%s", err, good)
	}
	invalid := []struct {
		name   string
		mutate func(hlt04Answer) []byte
	}{
		{"missing-record", func(v hlt04Answer) []byte { v.Records = v.Records[:7]; b, _ := json.Marshal(v); return b }},
		{"extra-record", func(v hlt04Answer) []byte {
			v.Records = append(v.Records, v.Records[7])
			b, _ := json.Marshal(v)
			return b
		}},
		{"wrong-type", func(v hlt04Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`"write_applied":"YES"`), []byte(`"write_applied":true`), 1)
		}},
		{"conflicting-claim", func(v hlt04Answer) []byte { v.AppliedReleaseDigestProven = "YES"; b, _ := json.Marshal(v); return b }},
		{"non-finite", func(v hlt04Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`"computed_observed_at":"2026-10-01T12:10:00Z"`), []byte(`"computed_observed_at":NaN`), 1)
		}},
		{"renewal-claim-yes", func(v hlt04Answer) []byte { v.RenewalProvesNewCheck = "YES"; b, _ := json.Marshal(v); return b }},
		{"renewal-claim-unknown", func(v hlt04Answer) []byte { v.RenewalProvesNewCheck = "UNKNOWN"; b, _ := json.Marshal(v); return b }},
		{"future-timestamp-claim-yes", func(v hlt04Answer) []byte {
			v.FutureTimestampProvesFreshness = "YES"
			b, _ := json.Marshal(v)
			return b
		}},
		{"future-timestamp-claim-unknown", func(v hlt04Answer) []byte {
			v.FutureTimestampProvesFreshness = "UNKNOWN"
			b, _ := json.Marshal(v)
			return b
		}},
		{"noncanonical-whitespace", func(v hlt04Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`"records":`), []byte(`"records": `), 1)
		}},
		{"trailing-newline", func(v hlt04Answer) []byte { b, _ := json.Marshal(v); return append(b, '\n') }},
		{"duplicate-key", func(v hlt04Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`"check_receipt":"AUTHORED_SYNTHETIC_NOT_CONTROLLER_EVIDENCE"`), []byte(`"check_receipt":"AUTHORED_SYNTHETIC_NOT_CONTROLLER_EVIDENCE","check_receipt":"CONTROLLER_CONFIRMED"`), 1)
		}},
		{"extra-property", func(v hlt04Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`"records":`), []byte(`"unexpected":"value","records":`), 1)
		}},
	}
	for _, tc := range invalid {
		if candidate := tc.mutate(answer); pattern.Match(candidate) {
			t.Errorf("strict grader accepted %s: %s", tc.name, candidate)
		}
	}
	for _, answerLeak := range []string{"sha256:synthetic-release-a", "sha256:synthetic-older", "2026-10-01T12:26:00Z"} {
		if bytes.Contains(prompt, []byte(answerLeak)) {
			t.Errorf("prompt leaks answer token %q", answerLeak)
		}
	}
}

func TestHLT04ManifestCountsAndFrozenQuestion(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "evals", "benchmark-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Status    string `json:"status"`
		Execution struct {
			Paid bool `json:"paid_runs_authorized_by_this_manifest"`
		} `json:"execution"`
		Groups []struct {
			ID     string  `json:"id"`
			Weight float64 `json:"weight"`
			Cases  []struct {
				ID           string   `json:"id"`
				Question     string   `json:"question"`
				Reference    string   `json:"reference"`
				Controls     []string `json:"controls"`
				Status       string   `json:"status"`
				ExistingCase string   `json:"existing_case"`
				Admission    string   `json:"benchmark_admission"`
				Provenance   struct {
					Files  map[string]string `json:"fixture_files_sha256"`
					Report string            `json:"replay_report"`
				} `json:"source_provenance"`
			} `json:"cases"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "frozen_design_not_executable" || manifest.Execution.Paid {
		t.Fatalf("manifest enables execution: %+v", manifest)
	}
	counts := map[string]int{}
	found := false
	for _, group := range manifest.Groups {
		for _, c := range group.Cases {
			counts[c.Status]++
			if c.ID != "HLT-04" {
				continue
			}
			found = true
			if group.ID != "health" || group.Weight != 1.0/6.0 && group.Weight != 0.1666666667 || c.Question != "Missing, old, or renewed report timestamps" || c.Reference != "Report release identity and health separately; a refreshed report timestamp does not refresh its underlying health checks or prove a release digest." || len(c.Controls) != 1 || c.Controls[0] != "Test clock skew, old checks in a refreshed report, and stale timestamp renewal" {
				t.Fatal("frozen HLT-04 question/reference/control/weight changed")
			}
			if c.Status != "synthetic_source_replay_prepared_not_run" || c.ExistingCase != "evals/sveltos-hlt-04-report-freshness" || !strings.Contains(c.Admission, "no model run") || c.Provenance.Report != "evals/reports/2026-10-01-sveltos-hlt04-report-freshness.json" {
				t.Fatalf("HLT-04 readiness mapping invalid: %+v", c)
			}
			for name, want := range hlt04FixtureHashes {
				if c.Provenance.Files[name] != want {
					t.Fatalf("manifest HLT-04 fixture hash for %s differs", name)
				}
			}
		}
	}
<<<<<<< HEAD
	if !found || counts["planned"] != 2 || counts["existing_refreshed_fixture"] != 5 || counts["recorded_snapshot_binding_prepared_not_run"] != 2 || counts["recorded_projection_prepared_not_run"] != 8 || counts["raw_recording_prepared_not_run"] != 6 || counts["synthetic_source_replay_prepared_not_run"] != 1 {
=======
	if !found || counts["planned"] != 2 || counts["existing_refreshed_fixture"] != 5 || counts["recorded_snapshot_binding_prepared_not_run"] != 2 || counts["recorded_projection_prepared_not_run"] != 8 || counts["raw_recording_prepared_not_run"] != 6 || counts["synthetic_source_replay_prepared_not_run"] != 1 {
>>>>>>> origin/main
		t.Fatalf("benchmark readiness counts changed: found=%v counts=%v", found, counts)
	}
}

func TestHLT04ScaffoldIsEqualAndExact(t *testing.T) {
	checkHLT04Scaffold(t, filepath.Clean(hlt04CaseRoot))
}

func checkHLT04Scaffold(t *testing.T, root string) {
	t.Helper()
	want := map[string]string{
		"scenario-evidence.json":                 hlt04FixtureHashes["scenario-evidence.json"],
		"source-metadata.json":                   hlt04FixtureHashes["source-metadata.json"],
		"synthetic-check-execution-receipt.json": hlt04FixtureHashes["synthetic-check-execution-receipt.json"],
	}
	scaffold, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	outputs := make([]map[string][]byte, 2)
	for i := range outputs {
		workspace := t.TempDir()
		cmd := exec.Command("bash", scaffold)
		cmd.Dir = workspace
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("HLT-04 scaffold: %v: %s", err, out)
		}
		entries, err := os.ReadDir(filepath.Join(workspace, "cluster"))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != len(want) {
			t.Fatalf("scaffold emitted %d files, want %d", len(entries), len(want))
		}
		outputs[i] = map[string][]byte{}
		for _, entry := range entries {
			if entry.IsDir() {
				t.Fatalf("unexpected scaffold directory %s", entry.Name())
			}
			data, err := os.ReadFile(filepath.Join(workspace, "cluster", entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			h := sha256.Sum256(data)
			if hex.EncodeToString(h[:]) != want[entry.Name()] {
				t.Fatalf("scaffold output %s hash mismatch", entry.Name())
			}
			outputs[i][entry.Name()] = data
		}
	}
	for name, first := range outputs[0] {
		if !bytes.Equal(first, outputs[1][name]) {
			t.Fatalf("benchmark arms differ on %s", name)
		}
	}
}
