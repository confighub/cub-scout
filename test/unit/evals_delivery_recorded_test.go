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
	}{
		{"flux-applied-digest", `{"ready_apps_digest":"sha256:a998f128abb06eb41f65140183ace5850e61702254753778a7a9ed18377cac48","source_revision":"2fe4c48d58f86d6fd44d2625444a2daf61d676d7","evidence_kind":"recorded_run_log","snapshot_limit":"trimmed_sequential_not_atomic"}`, map[string]string{"ready_apps_digest": "sha256:a998f128abb06eb41f65140183ace5850e61702254753778a7a9ed18377cac49", "source_revision": "2fe4c48d58f86d6fd44d2625444a2daf61d676d", "evidence_kind": "raw_live_snapshot", "snapshot_limit": "atomic"}},
		{"argo-published-release-lag", `{"consumed_before_refresh":"NO","pre_refresh_state":"PREVIOUS_RELEASE","transition":"HARD_REFRESH_RESOLVES_LATEST","exact_digests_and_counts":"UNKNOWN"}`, map[string]string{"consumed_before_refresh": "YES", "pre_refresh_state": "CURRENT_RELEASE", "transition": "PUBLICATION_ALONE", "exact_digests_and_counts": "sha256:123"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
					Projection string `json:"projection_sha256"`
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
	for _, g := range m.Groups {
		for _, c := range g.Cases {
			counts[c.Status]++
			if c.ID == "DEL-01" || c.ID == "DEL-02" {
				found[c.ID] = c.Status == "recorded_projection_prepared_not_run" && c.ExistingCase != "" && c.Provenance.Projection != ""
			}
		}
	}
	if m.Status != "frozen_design_not_executable" || m.Execution.Paid || !found["DEL-01"] || !found["DEL-02"] || counts["planned"] != 14 || counts["existing_refreshed_fixture"] != 5 || counts["existing_needs_snapshot_binding"] != 2 || counts["recorded_projection_prepared_not_run"] != 3 {
		t.Fatalf("case preparation changed benchmark gates or readiness: status=%q paid=%v mappings=%v counts=%v", m.Status, m.Execution.Paid, found, counts)
	}
}
