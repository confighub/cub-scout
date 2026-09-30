// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const sveltosHLT01Case = "sveltos-hlt-01-health"
const sveltosHLT01ProjectionSHA256 = "49e34b3bb6dc9adbe28c6be75ba97a51615149298a361a230efeb5aa5c18a3ef"
const sveltosHLT01SourceSHA256 = "24ef8acd48ead05400604198bb0ea695c284c146ab68f27b10a6601f75a655a8"
const sveltosHLT01ExcerptSHA256 = "6a03c7ec3491bce8bfaf1307114be732b75200225004272608c873b170aa0621"

func TestSveltosHLT01ProjectionProvenanceAndScaffold(t *testing.T) {
	caseRoot := filepath.Join("..", "..", "evals", sveltosHLT01Case)
	fixturePath := filepath.Join(caseRoot, "fixtures", "cluster", "sveltos-health.yaml")
	fixture, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(fixture)
	if got := hex.EncodeToString(digest[:]); got != sveltosHLT01ProjectionSHA256 {
		t.Fatalf("projection SHA-256 = %s, want %s", got, sveltosHLT01ProjectionSHA256)
	}

	var projection struct {
		Format string `yaml:"projection_format"`
		Source struct {
			Repository  string `yaml:"repository"`
			Revision    string `yaml:"revision"`
			Document    string `yaml:"document"`
			SHA256      string `yaml:"sha256"`
			Section     string `yaml:"section"`
			LineRange   []int  `yaml:"source_line_range"`
			ExcerptHash string `yaml:"excerpt_sha256"`
		} `yaml:"source"`
		Scope struct {
			Cluster         string `yaml:"cluster"`
			ObservationTime string `yaml:"observation_time_in_excerpt"`
			LaterOutputTime string `yaml:"later_status_output_time"`
			AtomicSnapshot  bool   `yaml:"atomic_snapshot"`
		} `yaml:"scope"`
		ExcerptLines []struct {
			SourceLine int    `yaml:"source_line"`
			Text       string `yaml:"text"`
		} `yaml:"excerpt_lines"`
	}
	if err := yaml.Unmarshal(fixture, &projection); err != nil {
		t.Fatalf("decode source projection: %v", err)
	}
	if projection.Format != "recorded-transcript/v1" || projection.Source.Repository != "confighub/sveltos-confighub" || projection.Source.Revision != "8187910f9fe226e109e55c4d9c7c0e21297ff424" || projection.Source.Document != "health-2026-09-30.log" || projection.Source.SHA256 != sveltosHLT01SourceSHA256 || projection.Source.Section != "Part B — the HealthCheck and ClusterHealthCheck written by cub sveltos apply" || len(projection.Source.LineRange) != 2 || projection.Source.LineRange[0] != 109 || projection.Source.LineRange[1] != 131 || projection.Source.ExcerptHash != sveltosHLT01ExcerptSHA256 {
		t.Fatalf("unexpected source provenance: %+v", projection.Source)
	}
	var excerpt strings.Builder
	for i, line := range projection.ExcerptLines {
		if line.SourceLine != 109+i {
			t.Fatalf("projection source line %d at index %d, want %d", line.SourceLine, i, 109+i)
		}
		excerpt.WriteString(line.Text)
		excerpt.WriteByte('\n')
	}
	excerptBytes := []byte(excerpt.String())
	excerptDigest := sha256.Sum256(excerptBytes)
	if got := hex.EncodeToString(excerptDigest[:]); got != sveltosHLT01ExcerptSHA256 {
		t.Fatalf("selected excerpt SHA-256 = %s, want %s", got, sveltosHLT01ExcerptSHA256)
	}
	if len(projection.ExcerptLines) != 23 || !strings.HasPrefix(projection.ExcerptLines[0].Text, "== 2. A workload goes down") || projection.ExcerptLines[22].SourceLine != 131 {
		t.Fatalf("excerpt is not exactly Part B source lines 109-131: %d lines", len(projection.ExcerptLines))
	}
	excerptText := excerpt.String()
	for _, evidence := range []string{
		"07:40:43, 238s later, the ClusterHealthCheck on eu-central-test1: False",
		"kyverno-eu-central-test1-sveltos-eu-central-test1: Provisioned",
		"eu-central-test1  mer-kyverno-eu-central-test1  Synced  Degraded",
		`"healthStatus": "Degraded"`,
		`"syncStatus": "Synced"`,
	} {
		if !strings.Contains(excerptText, evidence) {
			t.Errorf("source projection missing %q", evidence)
		}
	}
	if !strings.Contains(excerptText, "sha256:3cd288e11479") || regexp.MustCompile(`sha256:[0-9a-f]{64}`).MatchString(excerptText) {
		t.Fatal("projection should retain only the displayed shortened revision, not assert a complete digest")
	}
	if projection.Scope.Cluster != "eu-central-test1" || projection.Scope.ObservationTime != "07:40:43" || projection.Scope.LaterOutputTime != "not recorded" || projection.Scope.AtomicSnapshot {
		t.Fatalf("unexpected temporal/projection scope: %+v", projection.Scope)
	}
	if strings.Contains(excerptText, "Part A") || strings.Contains(excerptText, "A probe: one HealthCheck") || strings.Contains(string(fixture), "apiVersion:") || strings.Contains(string(fixture), "release_identity:") {
		t.Fatal("fixture includes unselected evidence, a derived release-identity answer, or a Kubernetes resource marker")
	}

	caseData, err := os.ReadFile(filepath.Join(caseRoot, "case.yaml"))
	if err != nil || !strings.Contains(string(caseData), "FIXTURE-OWNED-SCAFFOLD") {
		t.Fatalf("case must declare its fixture-owned scaffold: %v", err)
	}
	prompt, err := os.ReadFile(filepath.Join(caseRoot, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, leakedAnswer := range []string{"Provisioned", "Degraded", "Synced", "health_check.*False"} {
		if strings.Contains(string(prompt), leakedAnswer) {
			t.Errorf("prompt reveals answer %q", leakedAnswer)
		}
	}

	workspace := t.TempDir()
	scaffold, err := filepath.Abs(filepath.Join(caseRoot, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scaffold)
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run source-projection scaffold: %v\n%s", err, output)
	}
	got, err := os.ReadFile(filepath.Join(workspace, "cluster", "sveltos-health.yaml"))
	if err != nil || string(got) != string(fixture) {
		t.Fatalf("scaffold output differs from checked-in projection: %v", err)
	}
}

func TestSveltosHLT01GraderRequiresRecordedHealthAndUnknownRelease(t *testing.T) {
	caseRoot := filepath.Join("..", "..", "evals", sveltosHLT01Case)
	grader, err := os.ReadFile(filepath.Join(caseRoot, "graders", "health-contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindStringSubmatch(string(grader))
	if len(match) != 2 {
		t.Fatalf("grader has no portable single-quoted regex: %s", grader)
	}
	pattern, err := regexp.Compile(match[1])
	if err != nil {
		t.Fatalf("compile grader regex: %v", err)
	}
	good := `{"profile_state":"Provisioned","health_check":"False","sync_status":"Synced","report_health":"Degraded","release_identity":"UNKNOWN"}`
	whitespace := "{\n  \"profile_state\" : \"Provisioned\", \"health_check\" : \"False\",\n  \"sync_status\": \"Synced\", \"report_health\" : \"Degraded\",\n  \"release_identity\" : \"UNKNOWN\"\n}"
	for _, answer := range []string{good, whitespace} {
		if !pattern.MatchString(answer) {
			t.Errorf("grader rejected declared correct JSON: %s", answer)
		}
	}
	for _, answer := range []string{
		strings.Replace(good, `"report_health":"Degraded"`, `"report_health":"Healthy"`, 1),
		strings.Replace(good, `"report_health":"Degraded"`, `"report_health":"UNKNOWN"`, 1),
		strings.Replace(good, `"health_check":"False"`, `"health_check":"True"`, 1),
		strings.Replace(good, `"sync_status":"Synced"`, `"sync_status":"NotSynced"`, 1),
		strings.Replace(good, `"release_identity":"UNKNOWN"`, `"release_identity":"sha256:3cd288e11479"`, 1),
		strings.Replace(good, `"release_identity":"UNKNOWN"`, `"release_identity":"EXACT"`, 1),
		strings.Replace(good, `,"release_identity":"UNKNOWN"`, `,"release_identity":"UNKNOWN","extra":"claim"`, 1),
		strings.Replace(good, `,"health_check":"False"`, `,"health_check":"False","health_check":"True"`, 1),
		strings.Replace(good, `,"sync_status":"Synced"`, ``, 1),
		"Answer: " + good,
		good + "\nExplanation",
	} {
		if pattern.MatchString(answer) {
			t.Errorf("grader accepted incorrect or overclaimed answer: %s", answer)
		}
	}
}

func TestSveltosHLT01BenchmarkMappingDoesNotEnableSuite(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "evals", "benchmark-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Status string `json:"status"`
		Groups []struct {
			Cases []struct {
				ID                 string `json:"id"`
				Status             string `json:"status"`
				ExistingCase       string `json:"existing_case"`
				BenchmarkAdmission string `json:"benchmark_admission"`
				SourceProvenance   struct {
					SourceSHA256     string `json:"source_sha256"`
					ExcerptSHA256    string `json:"excerpt_sha256"`
					ProjectionSHA256 string `json:"projection_sha256"`
					Lines            []int  `json:"source_lines"`
				} `json:"source_provenance"`
			} `json:"cases"`
		} `json:"groups"`
		Execution struct {
			PaidRunsAuthorized bool `json:"paid_runs_authorized_by_this_manifest"`
		} `json:"execution"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	var found bool
	caseCount := 0
	for _, group := range manifest.Groups {
		for _, c := range group.Cases {
			caseCount++
			if c.ID != "HLT-01" {
				continue
			}
			found = true
			if c.Status != "recorded_projection_prepared_not_run" || c.ExistingCase != "evals/sveltos-hlt-01-health" || !strings.Contains(c.BenchmarkAdmission, "not run") || c.SourceProvenance.SourceSHA256 != sveltosHLT01SourceSHA256 || c.SourceProvenance.ExcerptSHA256 != sveltosHLT01ExcerptSHA256 || c.SourceProvenance.ProjectionSHA256 != sveltosHLT01ProjectionSHA256 || len(c.SourceProvenance.Lines) != 2 || c.SourceProvenance.Lines[0] != 109 || c.SourceProvenance.Lines[1] != 131 {
				t.Fatalf("unexpected HLT-01 manifest mapping: %+v", c)
			}
		}
	}
	if !found || caseCount != 24 || manifest.Status != "frozen_design_not_executable" || manifest.Execution.PaidRunsAuthorized {
		t.Fatalf("HLT-01 preparation changed suite inventory/execution gate: found=%v cases=%d status=%q paid=%v", found, caseCount, manifest.Status, manifest.Execution.PaidRunsAuthorized)
	}
}
