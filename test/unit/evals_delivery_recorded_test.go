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

func TestDeliveryRecordedProjectionsAndScaffolds(t *testing.T) {
	cases := []struct {
		name, fixture, projectionHash, sourceHash, excerptHash, document string
		lines                                                            [][]int
		mustContain                                                      []string
	}{
		{"flux-applied-digest", "flux-applied-digest.yaml", "0e912056b11379d3c7cebefab860983fac0e0ace47e1552741a96b86fbd35de2", "1872605798cbb2d554c8b0ee6a22b68c27bc652005701edf882887189e7fe24b", "f4659569fdcbd7d5c6ddc34345f46e2dcb001730a76529b9c477e27ccccad17e", "cub-flux/docs/runs/2026-09-30-bootstrapped-handover.md", [][]int{{1, 10}, {66, 86}}, []string{"Ready apps at latest@sha256:a998f128abb06eb41f65140183ace5850e61702254753778a7a9ed18377cac48", "main@sha1:2fe4c48d58f86d6fd44d2625444a2daf61d676d7", "every UID and the rollout revision identical"}},
		{"argo-published-release-lag", "argo-publication-lag.yaml", "9f701991446e38897c20906c36590fa2a1dfede669de590d05fd0ed678550dae", "44b33edec1378914e29080e774e2aef4a619505a704a3900dd040b44f297aaef", "24f859b8f44fd5c279eed0ee81c301c2df347d8240e6d74746bb8f88910111dd", "cub-argo/docs/onboard-your-argo-estate.md", [][]int{{597, 617}}, []string{"still unread ninety seconds later", "previous", "previous replica count", "hard refresh re-resolves the tag to a digest"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join("..", "..", "evals", tc.name)
			fixturePath := filepath.Join(root, "fixtures", "cluster", tc.fixture)
			fixture, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatal(err)
			}
			d := sha256.Sum256(fixture)
			if got := hex.EncodeToString(d[:]); got != tc.projectionHash {
				t.Fatalf("projection hash %s want %s", got, tc.projectionHash)
			}
			var data struct {
				Format string `yaml:"projection_format"`
				Source struct {
					Repository  string  `yaml:"repository"`
					Revision    string  `yaml:"revision"`
					Document    string  `yaml:"document"`
					SHA         string  `yaml:"sha256"`
					Ranges      [][]int `yaml:"ranges"`
					ExcerptHash string  `yaml:"excerpt_sha256"`
				} `yaml:"source"`
				Excerpt []struct {
					Line int    `yaml:"source_line"`
					Text string `yaml:"text"`
				} `yaml:"excerpt_lines"`
			}
			if err := yaml.Unmarshal(fixture, &data); err != nil {
				t.Fatal(err)
			}
			if data.Format != "recorded-markdown-excerpt/v1" || data.Source.Repository != "confighub/examples" || data.Source.Revision != "64a6c499ce824d4700a8dfbc1945333dc2e4a1e3" || data.Source.Document != tc.document || data.Source.SHA != tc.sourceHash || data.Source.ExcerptHash != tc.excerptHash || !equalRanges(data.Source.Ranges, tc.lines) {
				t.Fatalf("unexpected source provenance: %+v", data.Source)
			}
			var excerpt strings.Builder
			var expectedLines []int
			for _, r := range tc.lines {
				for n := r[0]; n <= r[1]; n++ {
					expectedLines = append(expectedLines, n)
				}
			}
			if len(data.Excerpt) != len(expectedLines) {
				t.Fatalf("projection has %d lines, want %d", len(data.Excerpt), len(expectedLines))
			}
			for i, line := range data.Excerpt {
				if line.Line != expectedLines[i] {
					t.Fatalf("source line at index %d is %d, want %d", i, line.Line, expectedLines[i])
				}
				excerpt.WriteString(line.Text)
				excerpt.WriteByte('\n')
			}
			d = sha256.Sum256([]byte(excerpt.String()))
			if got := hex.EncodeToString(d[:]); got != tc.excerptHash {
				t.Fatalf("excerpt hash %s want %s", got, tc.excerptHash)
			}
			for _, text := range tc.mustContain {
				if !strings.Contains(excerpt.String(), text) {
					t.Errorf("projection lacks %q", text)
				}
			}
			if data.Source.Document == tc.document && strings.Contains(string(fixture), "apiVersion:") {
				t.Fatal("Markdown excerpt must not masquerade as a Kubernetes resource")
			}
			caseYAML, err := os.ReadFile(filepath.Join(root, "case.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(caseYAML), "FIXTURE-OWNED-SCAFFOLD") {
				t.Fatal("case does not declare fixture-owned scaffold")
			}
			workspace := t.TempDir()
			scaffold, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("bash", scaffold)
			cmd.Dir = workspace
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("scaffold: %v\n%s", err, b)
			}
			got, err := os.ReadFile(filepath.Join(workspace, "cluster", tc.fixture))
			if err != nil || string(got) != string(fixture) {
				t.Fatalf("scaffold differs from evidence projection: %v", err)
			}
		})
	}
}

func equalRanges(a, b [][]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != 2 || a[i][0] != b[i][0] || a[i][1] != b[i][1] {
			return false
		}
	}
	return true
}

func TestDeliveryRecordedCaseGradersRejectOverclaims(t *testing.T) {
	cases := []struct {
		name, good string
		bad        map[string]string
		vocabulary []string
	}{
		{"flux-applied-digest", `{"ready_apps_digest":"sha256:a998f128abb06eb41f65140183ace5850e61702254753778a7a9ed18377cac48","source_revision":"main@sha1:2fe4c48d58f86d6fd44d2625444a2daf61d676d7","evidence_kind":"recorded_run_log","snapshot_limit":"trimmed_sequential_not_atomic"}`, map[string]string{"ready_apps_digest": "sha256:a998f128abb06eb41f65140183ace5850e61702254753778a7a9ed18377cac49", "source_revision": "main@sha1:2fe4c48d58f86d6fd44d2625444a2daf61d676d", "evidence_kind": "raw_live_snapshot", "snapshot_limit": "atomic"}, []string{"main@sha1:<40-character-hex>", "recorded_run_log", "raw_live_snapshot", "UNKNOWN", "trimmed_sequential_not_atomic", "atomic"}},
		{"argo-published-release-lag", `{"consumed_before_refresh":"NO","pre_refresh_state":"PREVIOUS_RELEASE","transition":"HARD_REFRESH_RESOLVES_LATEST","exact_digests_and_counts":"UNKNOWN"}`, map[string]string{"consumed_before_refresh": "YES", "pre_refresh_state": "CURRENT_RELEASE", "transition": "PUBLICATION_ALONE", "exact_digests_and_counts": "EXACT_VALUES_SHOWN"}, []string{"YES", "NO", "UNKNOWN", "PREVIOUS_RELEASE", "CURRENT_RELEASE", "HARD_REFRESH_RESOLVES_LATEST", "PUBLICATION_ALONE", "EXACT_VALUES_SHOWN"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompt, err := os.ReadFile(filepath.Join("..", "..", "evals", tc.name, "prompt.md"))
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range tc.vocabulary {
				if !strings.Contains(string(prompt), value) {
					t.Errorf("prompt does not disclose allowed answer value %q", value)
				}
			}
			grader, err := os.ReadFile(filepath.Join("..", "..", "evals", tc.name, "graders", "verified-answer.md"))
			if err != nil {
				t.Fatal(err)
			}
			m := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindStringSubmatch(string(grader))
			if len(m) != 2 {
				t.Fatalf("missing portable regex grader: %s", grader)
			}
			pattern, err := regexp.Compile(m[1])
			if err != nil {
				t.Fatal(err)
			}
			if !pattern.MatchString(tc.good) {
				t.Fatalf("grader rejected correct answer: %s", tc.good)
			}
			for key, value := range tc.bad {
				// Each negative is a deliberate wrong field-value substitution.
				var obj map[string]string
				if err := json.Unmarshal([]byte(tc.good), &obj); err != nil {
					t.Fatal(err)
				}
				obj[key] = value
				b, _ := json.Marshal(obj)
				if pattern.MatchString(string(b)) {
					t.Errorf("grader accepted incorrect answer: %s", b)
				}
			}
			if pattern.MatchString("Answer: "+tc.good) || pattern.MatchString(tc.good+"\nExplanation") {
				t.Error("grader accepted prose outside JSON")
			}
		})
	}
}

func TestFluxHLT02GraderRejectsHealthAndProvenanceOverclaims(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "flux-ready-without-health")
	prompt, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"ready_current_generation", "wait", "health_checks", "deployment_current_generation", "uid_chain", "source_revision", "applied_revision", "in any order", "SEQUENTIAL_NOT_ATOMIC", "CAPTURE_ONLY", "NOT_RECORDED", "UNKNOWN"} {
		if !strings.Contains(string(prompt), term) {
			t.Errorf("prompt does not define answer contract term %q", term)
		}
	}
	grader, err := os.ReadFile(filepath.Join(root, "graders", "verified-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindStringSubmatch(string(grader))
	if len(m) != 2 {
		t.Fatalf("missing deterministic regex grader: %s", grader)
	}
	good := `{"ready_current_generation":"YES","wait":"FALSE","health_checks":"ABSENT_OR_EMPTY","deployment_current_generation":"UNAVAILABLE","uid_chain":"deployment:3b56bff9-bfe2-4f1f-9913-1ebef756c0e8;replicaset:b7a593de-f7e9-417e-b5a9-1c71e1250f5d;pod:ee02c558-9a4a-489b-b1a6-e792c4b7f01b","source_revision":"sha1:7732dde28be8cf8c42c096d94efbd8ce4a9d0a19","applied_revision":"sha1:7732dde28be8cf8c42c096d94efbd8ce4a9d0a19","revision_binding":"MATCH","ready_proves_workload_healthy":"NO","observation_scope":"SEQUENTIAL_NOT_ATOMIC","current_time_claim":"CAPTURE_ONLY","application_level_check":"NOT_RECORDED"}`
	var reordered map[string]string
	if err := json.Unmarshal([]byte(good), &reordered); err != nil {
		t.Fatal(err)
	}
	reorderedBytes, err := json.Marshal(reordered)
	if err != nil {
		t.Fatal(err)
	}
	positiveCandidates := []string{good, string(reorderedBytes)}
	if results := runPythonRegexCases(t, m[1], positiveCandidates); len(results) != 2 || !results[0] || !results[1] {
		t.Fatalf("Python grader did not accept plain/reordered reference answer: %v pattern=%q good=%q reordered=%q", results, m[1], good, reorderedBytes)
	}
	negatives := []struct{ from, to string }{
		{`"ready_current_generation":"YES"`, `"ready_current_generation":"NO"`},
		{`"wait":"FALSE"`, `"wait":"TRUE"`},
		{`"health_checks":"ABSENT_OR_EMPTY"`, `"health_checks":"CONFIGURED"`},
		{`"deployment_current_generation":"UNAVAILABLE"`, `"deployment_current_generation":"AVAILABLE"`},
		{`"uid_chain":"deployment:3b56bff9-bfe2-4f1f-9913-1ebef756c0e8;replicaset:b7a593de-f7e9-417e-b5a9-1c71e1250f5d;pod:ee02c558-9a4a-489b-b1a6-e792c4b7f01b"`, `"uid_chain":"UNKNOWN"`},
		{`"source_revision":"sha1:7732dde28be8cf8c42c096d94efbd8ce4a9d0a19"`, `"source_revision":"UNKNOWN"`},
		{`"applied_revision":"sha1:7732dde28be8cf8c42c096d94efbd8ce4a9d0a19"`, `"applied_revision":"sha1:7732dde28be8cf8c42c096d94efbd8ce4a9d0a18"`},
		{`"revision_binding":"MATCH"`, `"revision_binding":"MISMATCH"`},
		{`"ready_proves_workload_healthy":"NO"`, `"ready_proves_workload_healthy":"YES"`},
		{`"observation_scope":"SEQUENTIAL_NOT_ATOMIC"`, `"observation_scope":"ATOMIC"`},
		{`"current_time_claim":"CAPTURE_ONLY"`, `"current_time_claim":"CURRENT"`},
		{`"application_level_check":"NOT_RECORDED"`, `"application_level_check":"PERFORMED"`},
	}
	var negativeCandidates []string
	for _, tc := range negatives {
		if strings.Count(good, tc.from) != 1 {
			t.Fatalf("bad test substitution %q", tc.from)
		}
		negativeCandidates = append(negativeCandidates, strings.Replace(good, tc.from, tc.to, 1))
	}
	negativeCandidates = append(negativeCandidates, "Answer: "+good, good+"\nExplanation", strings.TrimSuffix(good, "}")+`,"extra":"value"}`, strings.Replace(good, `"wait":"FALSE"`, `"wait":"FALSE","wait":"TRUE"`, 1))
	for i, matched := range runPythonRegexCases(t, m[1], negativeCandidates) {
		if matched {
			t.Errorf("Python grader accepted negative control %d: %s", i, negativeCandidates[i])
		}
	}
}

func runPythonRegexCases(t *testing.T, pattern string, samples []string) []bool {
	t.Helper()
	requireFluxHLT02Python(t)
	input, err := json.Marshal(struct {
		Pattern string   `json:"pattern"`
		Samples []string `json:"samples"`
	}{pattern, samples})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "-c", `import json,re,sys; d=json.load(sys.stdin); p=re.compile(d["pattern"]); print(json.dumps([p.search(s) is not None for s in d["samples"]]))`)
	cmd.Stdin = bytes.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run Python grader controls: %v: %s", err, output)
	}
	var results []bool
	if err := json.Unmarshal(output, &results); err != nil {
		t.Fatalf("decode Python grader results: %v: %s", err, output)
	}
	return results
}

func TestFluxHLT02CaseMetadataUsesSingleSchemaAndPromptFrontmatter(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "flux-ready-without-health")
	caseData, err := os.ReadFile(filepath.Join(root, "case.yaml"))
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
	decoder := yaml.NewDecoder(bytes.NewReader(caseData))
	decoder.KnownFields(true)
	if err := decoder.Decode(&schema); err != nil {
		t.Fatalf("decode case schema: %v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("case.yaml must contain exactly one document, second decode=%v", err)
	}
	if schema.SchemaVersion != "1.1" || schema.Name != "flux-ready-without-health" || schema.Context.Scaffold != "scaffold.sh" {
		t.Fatalf("case schema or scaffold declaration invalid: %+v", schema)
	}
	prompt, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(prompt)
	if !strings.HasPrefix(text, "---\n") {
		t.Fatal("prompt.md is missing execution frontmatter")
	}
	end := strings.Index(text[4:], "\n---\n")
	if end < 0 {
		t.Fatal("prompt.md frontmatter is unterminated")
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
	execDecoder := yaml.NewDecoder(strings.NewReader(text[4 : 4+end]))
	execDecoder.KnownFields(true)
	if err := execDecoder.Decode(&execution); err != nil {
		t.Fatalf("decode execution frontmatter: %v", err)
	}
	if execution.Name != schema.Name || execution.Description == "" || execution.ExpectedOutcome == "" || execution.MaxTurns != 4 || execution.TimeoutSeconds != 90 || len(execution.AllowedTools) != 2 || execution.AllowedTools[0] != "Read" || execution.AllowedTools[1] != "Grep" {
		t.Fatalf("execution frontmatter invalid: %+v", execution)
	}
}

func TestFluxHLT02ScaffoldPreservesIdenticalRawEvidence(t *testing.T) {
	checkFluxHLT02ScaffoldBytes(t, filepath.Join("..", "..", "evals", "flux-ready-without-health"))
}

func checkFluxHLT02ScaffoldBytes(t *testing.T, root string) {
	t.Helper()
	caseData, err := os.ReadFile(filepath.Join(root, "case.yaml"))
	if err != nil || !strings.Contains(string(caseData), "FIXTURE-OWNED-SCAFFOLD") || !strings.Contains(string(caseData), "scaffold_script: scaffold.sh") {
		t.Fatalf("case must declare its fixture-owned scaffold: %v", err)
	}
	sourceDir := filepath.Join(root, "fixtures", "2026-09-30")
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	scaffold, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	outputs := make([]string, 2)
	for i := range outputs {
		workspace := t.TempDir()
		cmd := exec.Command("bash", scaffold)
		cmd.Dir = workspace
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("scaffold run %d: %v: %s", i, err, out)
		}
		outputs[i] = filepath.Join(workspace, "cluster")
		gotEntries, err := os.ReadDir(outputs[i])
		if err != nil {
			t.Fatal(err)
		}
		if len(gotEntries) != len(entries)+2 {
			t.Fatalf("scaffold emitted %d files, want exactly %d captured inputs", len(gotEntries), len(entries)+2)
		}
		for _, entry := range gotEntries {
			if entry.IsDir() {
				t.Fatalf("scaffold emitted unexpected directory %s", entry.Name())
			}
		}
		for _, entry := range entries {
			if entry.IsDir() {
				t.Fatalf("unexpected subdirectory in recorded evidence: %s", entry.Name())
			}
			want, err := os.ReadFile(filepath.Join(sourceDir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(outputs[i], entry.Name()))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("scaffold run %d changed or omitted %s: %v", i, entry.Name(), err)
			}
		}
		for _, name := range []string{"gitrepository.yaml", "kustomization.yaml"} {
			want, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(outputs[i], name))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("scaffold run %d changed or omitted %s: %v", i, name, err)
			}
		}
	}
	for _, name := range append(func() []string {
		names := make([]string, 0, len(entries)+2)
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		return names
	}(), "gitrepository.yaml", "kustomization.yaml") {
		first, err := os.ReadFile(filepath.Join(outputs[0], name))
		if err != nil {
			t.Fatal(err)
		}
		second, err := os.ReadFile(filepath.Join(outputs[1], name))
		if err != nil || !bytes.Equal(first, second) {
			t.Fatalf("arms received different bytes for %s: %v", name, err)
		}
	}
}

func TestDeliveryCaseMappingsKeepBenchmarkUnexecutable(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "evals", "benchmark-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Status string `json:"status"`
		Groups []struct {
			Cases []struct {
				ID           string `json:"id"`
				Status       string `json:"status"`
				ExistingCase string `json:"existing_case"`
				Provenance   struct {
					Projection       string            `json:"projection_sha256"`
					Binding          string            `json:"raw_capture_binding_sha256"`
					RawFiles         map[string]string `json:"raw_files_sha256"`
					AppliedManifests map[string]string `json:"applied_manifest_sha256"`
					SourceRevision   string            `json:"source_revision"`
					AppliedRevision  string            `json:"applied_revision"`
					AtomicSnapshot   bool              `json:"atomic_snapshot"`
				} `json:"source_provenance"`
			} `json:"cases"`
		} `json:"groups"`
		Execution struct {
			Paid bool `json:"paid_runs_authorized_by_this_manifest"`
		} `json:"execution"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	found := map[string]bool{}
	var hlt02Provenance struct {
		Binding          string            `json:"raw_capture_binding_sha256"`
		RawFiles         map[string]string `json:"raw_files_sha256"`
		AppliedManifests map[string]string `json:"applied_manifest_sha256"`
		SourceRevision   string            `json:"source_revision"`
		AppliedRevision  string            `json:"applied_revision"`
		AtomicSnapshot   bool              `json:"atomic_snapshot"`
	}
	for _, g := range m.Groups {
		for _, c := range g.Cases {
			counts[c.Status]++
			if c.ID == "DEL-01" || c.ID == "DEL-02" {
				found[c.ID] = c.Status == "recorded_projection_prepared_not_run" && c.ExistingCase != "" && c.Provenance.Projection != ""
			} else if c.ID == "HLT-02" {
				found[c.ID] = c.Status == "raw_recording_prepared_not_run" && c.ExistingCase == "evals/flux-ready-without-health" && c.Provenance.Binding == "fe81b64dc4e259d76cff54cda0ca084e44bbedd367a35f6619694d89394940b3"
				hlt02Provenance.Binding = c.Provenance.Binding
				hlt02Provenance.RawFiles = c.Provenance.RawFiles
				hlt02Provenance.AppliedManifests = c.Provenance.AppliedManifests
				hlt02Provenance.SourceRevision = c.Provenance.SourceRevision
				hlt02Provenance.AppliedRevision = c.Provenance.AppliedRevision
				hlt02Provenance.AtomicSnapshot = c.Provenance.AtomicSnapshot
			}
		}
	}
	if m.Status != "frozen_design_not_executable" || m.Execution.Paid || !found["DEL-01"] || !found["DEL-02"] || !found["HLT-02"] || counts["planned"] != 12 || counts["existing_refreshed_fixture"] != 5 || counts["existing_needs_snapshot_binding"] != 2 || counts["recorded_projection_prepared_not_run"] != 4 || counts["raw_recording_prepared_not_run"] != 1 {
		t.Fatalf("case preparation changed benchmark gates or readiness: status=%q paid=%v mappings=%v counts=%v", m.Status, m.Execution.Paid, found, counts)
	}
	if hlt02Provenance.SourceRevision != "sha1:7732dde28be8cf8c42c096d94efbd8ce4a9d0a19" || hlt02Provenance.AppliedRevision != hlt02Provenance.SourceRevision || hlt02Provenance.AtomicSnapshot {
		t.Fatalf("HLT-02 provenance overstates revision or snapshot: %+v", hlt02Provenance)
	}
	caseRoot := filepath.Join("..", "..", "evals", "flux-ready-without-health")
	fixtureRoot := filepath.Join(caseRoot, "fixtures", "2026-09-30")
	bindingBytes, err := os.ReadFile(filepath.Join(fixtureRoot, "binding.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(bindingBytes)) != hlt02Provenance.Binding {
		t.Fatal("benchmark manifest raw capture binding hash differs from committed binding.json")
	}
	var binding struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(bindingBytes, &binding); err != nil {
		t.Fatal(err)
	}
	if len(binding.Files) == 0 || len(binding.Files) != len(hlt02Provenance.RawFiles) {
		t.Fatalf("manifest raw file hash inventory size=%d binding size=%d", len(hlt02Provenance.RawFiles), len(binding.Files))
	}
	for name, want := range binding.Files {
		if hlt02Provenance.RawFiles[name] != want {
			t.Fatalf("manifest raw hash for %s differs from binding", name)
		}
		contents, err := os.ReadFile(filepath.Join(fixtureRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(contents)); got != want {
			t.Fatalf("raw capture file %s hash=%s want %s", name, got, want)
		}
	}
	for name, want := range hlt02Provenance.AppliedManifests {
		contents, err := os.ReadFile(filepath.Join(caseRoot, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(contents)); got != want {
			t.Fatalf("applied manifest %s hash=%s want %s", name, got, want)
		}
	}
}
