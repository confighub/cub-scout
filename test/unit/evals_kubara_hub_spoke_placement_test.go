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

const kubaraPlacementSourceRevision = "cef6e7337884c6196c3b7c35cec587b8faf07447"

var kubaraPlacementFixtureHashes = map[string]string{
	"desired-matrix.json": "e4e111557d4df457ac327c20baf09e5f2f8080cf698eb1767b12ba96962c0ee8",
	"config.yaml":         "7260fe438066413c1bc44b5f674e4441e3ae9873146e860d4188ef518abc1211",
}

func TestKubaraHubSpokePlacementEvidenceAndGrader(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "kubara-hub-spoke-placement")
	checkKubaraHubSpokePlacementScaffold(t, root)
	checkKubaraPlacementMetadata(t, root)
	checkKubaraPlacementGrader(t, root)
}

func checkKubaraHubSpokePlacementScaffold(t *testing.T, root string) {
	t.Helper()
	fixtureDir := filepath.Join(root, "fixtures")
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("fixture directory has %d entries, want two sources and provenance", len(entries))
	}
	fixtures := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("unexpected nested fixture directory %s", entry.Name())
		}
		b, err := os.ReadFile(filepath.Join(fixtureDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if want, pinned := kubaraPlacementFixtureHashes[entry.Name()]; pinned {
			sum := sha256.Sum256(b)
			if got := hex.EncodeToString(sum[:]); got != want {
				t.Fatalf("%s SHA-256=%s, want %s", entry.Name(), got, want)
			}
		} else if entry.Name() != "source-provenance.json" {
			t.Fatalf("unexpected fixture %q", entry.Name())
		}
		fixtures[entry.Name()] = b
	}
	if err := validateKubaraPlacementEvidence(fixtures); err != nil {
		t.Fatal(err)
	}

	var armCopies [2]map[string][]byte
	for arm := range armCopies {
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
		staged, err := os.ReadDir(filepath.Join(workspace, "cluster"))
		if err != nil {
			t.Fatal(err)
		}
		if len(staged) != len(fixtures) {
			t.Fatalf("arm %d got %d files, want %d", arm, len(staged), len(fixtures))
		}
		armCopies[arm] = make(map[string][]byte, len(fixtures))
		for name, want := range fixtures {
			got, err := os.ReadFile(filepath.Join(workspace, "cluster", name))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("arm %d changed fixture %s", arm, name)
			}
			armCopies[arm][name] = got
		}
	}
	for name := range fixtures {
		if !bytes.Equal(armCopies[0][name], armCopies[1][name]) {
			t.Errorf("the two arms received different bytes for %s", name)
		}
	}
}

func validateKubaraPlacementEvidence(fixtures map[string][]byte) error {
	var provenance struct {
		Schema           string `json:"schema"`
		SourceRepository string `json:"sourceRepository"`
		SourceRevision   string `json:"sourceRevision"`
		EvidenceKind     string `json:"evidenceKind"`
		Files            []struct {
			Fixture    string `json:"fixture"`
			SourcePath string `json:"sourcePath"`
			SHA256     string `json:"sha256"`
		} `json:"files"`
		Scope struct {
			LiveReceiptConsumed    bool     `json:"liveReceiptConsumed"`
			ParsedObservationCells int      `json:"parsedObservationCells"`
			LiveReads              int      `json:"liveReads"`
			Limitations            []string `json:"limitations"`
		} `json:"scope"`
	}
	if err := json.Unmarshal(fixtures["source-provenance.json"], &provenance); err != nil {
		return err
	}
	if provenance.Schema != "kubara.desired-placement-source-provenance.v1" ||
		provenance.SourceRepository != "https://github.com/confighub/kubara-confighub" ||
		provenance.SourceRevision != kubaraPlacementSourceRevision ||
		!strings.Contains(provenance.EvidenceKind, "not a raw cluster capture") ||
		len(provenance.Files) != len(kubaraPlacementFixtureHashes) {
		return fmt.Errorf("unexpected provenance: %+v", provenance)
	}
	sourcePaths := map[string]string{
		"desired-matrix.json": "data/kubara-platform-matrix/desired-matrix.json",
		"config.yaml":         "examples/kubara/current-platform/source/config.yaml",
	}
	for _, file := range provenance.Files {
		if _, ok := fixtures[file.Fixture]; !ok || file.SHA256 != kubaraPlacementFixtureHashes[file.Fixture] || file.SourcePath != sourcePaths[file.Fixture] {
			return fmt.Errorf("invalid source mapping: %+v", file)
		}
	}
	if provenance.Scope.LiveReceiptConsumed || provenance.Scope.ParsedObservationCells != 0 ||
		provenance.Scope.LiveReads != 0 || len(provenance.Scope.Limitations) < 2 {
		return fmt.Errorf("provenance overstates live evidence: %+v", provenance.Scope)
	}

	var matrix struct {
		Spec struct {
			Evidence struct {
				Mode           string `json:"mode"`
				KubaraVersion  string `json:"kubaraVersion"`
				MiniIDPReceipt struct {
					Status         string `json:"status"`
					AcceptedAsLive bool   `json:"acceptedAsLive"`
					ParsedCells    int    `json:"parsedCells"`
				} `json:"miniIdpReceipt"`
				ParsedObservationCells int   `json:"parsedObservationCells"`
				LiveReads              []any `json:"liveReads"`
			} `json:"evidence"`
			Rows []struct {
				Component       string `json:"component"`
				Cluster         string `json:"cluster"`
				ClusterType     string `json:"clusterType"`
				SelectedVersion string `json:"selectedVersion"`
				Presence        string `json:"presence"`
				ProofStatus     string `json:"proofStatus"`
				ObservedVersion string `json:"observedVersion"`
				SyncState       string `json:"syncState"`
				HealthState     string `json:"healthState"`
				Readiness       struct {
					Result string `json:"result"`
				} `json:"readiness"`
			} `json:"rows"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(fixtures["desired-matrix.json"], &matrix); err != nil {
		return err
	}
	evidence := matrix.Spec.Evidence
	if evidence.Mode != "current-config-plus-effective-render" || evidence.KubaraVersion != "v0.13.0" ||
		evidence.MiniIDPReceipt.Status != "not-consumed-for-desired-artifact" ||
		evidence.MiniIDPReceipt.AcceptedAsLive || evidence.MiniIDPReceipt.ParsedCells != 0 ||
		evidence.ParsedObservationCells != 0 || len(evidence.LiveReads) != 0 {
		return fmt.Errorf("desired matrix includes live observations: %+v", evidence)
	}

	wantClusters := map[string]string{
		"hx-app-dev": "hub", "hx-app-staging": "spoke", "hx-app-prod-a": "spoke", "hx-app-prod-b": "spoke",
	}
	seen := map[string]bool{}
	certManagerRows := 0
	argoSpokes := 0
	for _, row := range matrix.Spec.Rows {
		if row.Component == "cert-manager" {
			certManagerRows++
			if row.SelectedVersion != "jetstack/cert-manager@v1.21.0" ||
				row.ClusterType != wantClusters[row.Cluster] || row.Presence != "rendered-intent" ||
				row.ObservedVersion != "Unknown" || row.SyncState != "Unknown" ||
				row.HealthState != "Unknown" || row.Readiness.Result != "unknown" {
				return fmt.Errorf("unexpected cert-manager row: %+v", row)
			}
			seen[row.Cluster] = true
		}
		if row.Component == "argo-cd" && row.ClusterType == "spoke" {
			argoSpokes++
			if row.Presence != "hub-managed" || row.ProofStatus != "centralized" ||
				row.ObservedVersion != "Unknown" || row.SyncState != "Unknown" || row.HealthState != "Unknown" {
				return fmt.Errorf("unexpected spoke Argo row: %+v", row)
			}
		}
	}
	if len(seen) != 4 || certManagerRows != 4 || argoSpokes != 3 {
		return fmt.Errorf("cert-manager rows=%d (%v); spoke Argo rows=%d", certManagerRows, seen, argoSpokes)
	}

	var config struct {
		Clusters []struct {
			Name     string `yaml:"name"`
			Type     string `yaml:"type"`
			Services map[string]struct {
				Status string `yaml:"status"`
			} `yaml:"services"`
		} `yaml:"clusters"`
	}
	if err := yaml.Unmarshal(fixtures["config.yaml"], &config); err != nil {
		return err
	}
	if len(config.Clusters) != 4 {
		return fmt.Errorf("source config has %d clusters, want 4", len(config.Clusters))
	}
	for _, cluster := range config.Clusters {
		if cluster.Type != wantClusters[cluster.Name] || cluster.Services["cert-manager"].Status != "enabled" {
			return fmt.Errorf("unexpected source config row: %s type=%s services=%+v", cluster.Name, cluster.Type, cluster.Services)
		}
	}
	return nil
}

func checkKubaraPlacementMetadata(t *testing.T, root string) {
	t.Helper()
	caseBytes, err := os.ReadFile(filepath.Join(root, "case.yaml"))
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
	decoder := yaml.NewDecoder(bytes.NewReader(caseBytes))
	decoder.KnownFields(true)
	if err := decoder.Decode(&schema); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("case.yaml must contain one YAML document, second decode=%v", err)
	}
	if schema.SchemaVersion != "1.1" || schema.Name != "kubara-hub-spoke-placement" || schema.Context.Scaffold != "scaffold.sh" {
		t.Fatalf("unexpected case schema: %+v", schema)
	}
	promptBytes, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	prompt := string(promptBytes)
	if !strings.HasPrefix(prompt, "---\n") {
		t.Fatal("prompt has no execution frontmatter")
	}
	end := strings.Index(prompt[4:], "\n---\n")
	if end < 0 {
		t.Fatal("prompt frontmatter is not closed")
	}
	var frontmatter struct {
		Name            string   `yaml:"name"`
		Description     string   `yaml:"description"`
		ExpectedOutcome string   `yaml:"expected_outcome"`
		Tags            []string `yaml:"tags"`
		MaxTurns        int      `yaml:"max_turns"`
		TimeoutSeconds  int      `yaml:"timeout_seconds"`
		AllowedTools    []string `yaml:"allowed_tools"`
	}
	fm := yaml.NewDecoder(strings.NewReader(prompt[4 : 4+end]))
	fm.KnownFields(true)
	if err := fm.Decode(&frontmatter); err != nil {
		t.Fatal(err)
	}
	if frontmatter.Name != schema.Name || frontmatter.Description == "" || frontmatter.ExpectedOutcome == "" ||
		frontmatter.MaxTurns != 8 || frontmatter.TimeoutSeconds != 120 ||
		len(frontmatter.AllowedTools) != 2 || frontmatter.AllowedTools[0] != "Read" || frontmatter.AllowedTools[1] != "Grep" {
		t.Fatalf("invalid execution frontmatter: %+v", frontmatter)
	}
	body := prompt[4+end+5:]
	for _, leaked := range []string{"hx-app-dev", "hx-app-staging", "v1.21.0", "hub-managed", "VERSION_SYNC_HEALTH_READINESS_UNKNOWN_for_all_four_cells"} {
		if strings.Contains(body, leaked) {
			t.Errorf("prompt body leaks reference answer %q", leaked)
		}
	}
}

func checkKubaraPlacementGrader(t *testing.T, root string) {
	t.Helper()
	grader, err := os.ReadFile(filepath.Join(root, "graders", "verified-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindSubmatch(grader)
	if len(match) != 2 {
		t.Fatalf("grader lacks single-quoted regex pattern: %s", grader)
	}
	pattern := string(match[1])
	good := `{"component":"cert-manager","selected_version":"jetstack/cert-manager@v1.21.0","hub_cluster":"hx-app-dev","spoke_clusters":"hx-app-staging,hx-app-prod-a,hx-app-prod-b","cert_manager_placement":"selected_on_hub_and_all_three_spokes","cert_manager_live_observation_fields":"VERSION_SYNC_HEALTH_READINESS_UNKNOWN_for_all_four_cells","spoke_argo_placement":"hub-managed","spoke_argo_live_observation":"UNKNOWN","unknown_interpretation":"not_disabled_unmanaged_or_unhealthy","evidence":"desired-matrix.json+config.yaml"}`
	var answer map[string]string
	if err := json.Unmarshal([]byte(good), &answer); err != nil {
		t.Fatal(err)
	}
	keys := []string{"evidence", "unknown_interpretation", "spoke_argo_live_observation", "spoke_argo_placement", "cert_manager_live_observation_fields", "cert_manager_placement", "spoke_clusters", "hub_cluster", "selected_version", "component"}
	var reordered strings.Builder
	reordered.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			reordered.WriteByte(',')
		}
		keyJSON, _ := json.Marshal(key)
		valueJSON, _ := json.Marshal(answer[key])
		reordered.Write(keyJSON)
		reordered.WriteByte(':')
		reordered.Write(valueJSON)
	}
	reordered.WriteByte('}')
	pretty, err := json.MarshalIndent(answer, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	positive := []string{good, reordered.String(), "\n  " + string(pretty) + " \n"}
	bad := []string{
		strings.Replace(good, `"hub_cluster":"hx-app-dev"`, `"hub_cluster":"hx-app-staging"`, 1),
		strings.Replace(good, `"spoke_clusters":"hx-app-staging,hx-app-prod-a,hx-app-prod-b"`, `"spoke_clusters":"hx-app-dev"`, 1),
		strings.Replace(good, `"cert_manager_live_observation_fields":"VERSION_SYNC_HEALTH_READINESS_UNKNOWN_for_all_four_cells"`, `"cert_manager_live_observation_fields":"Healthy"`, 1),
		strings.Replace(good, `"cert_manager_placement":"selected_on_hub_and_all_three_spokes"`, `"cert_manager_placement":"disabled"`, 1),
		strings.Replace(good, `"unknown_interpretation":"not_disabled_unmanaged_or_unhealthy"`, `"unknown_interpretation":"unmanaged"`, 1),
		strings.Replace(good, `"spoke_argo_placement":"hub-managed"`, `"spoke_argo_placement":"installed"`, 1),
		strings.Replace(good, `,"evidence":"desired-matrix.json+config.yaml"`, "", 1),
		strings.Replace(good, `,"evidence":"desired-matrix.json+config.yaml"`, `,"extra":"claim","evidence":"desired-matrix.json+config.yaml"`, 1),
		strings.Replace(good, `,"evidence":"desired-matrix.json+config.yaml"`, `,"evidence":"desired-matrix.json+config.yaml","evidence":"desired-matrix.json+config.yaml"`, 1),
		"Answer: " + good,
		good + "\nArgo is definitely installed and healthy on every spoke.",
	}
	samples := append(positive, bad...)
	result := runOCIIdentityRegex(t, "node", `const d=JSON.parse(require("fs").readFileSync(0,"utf8")); const p=new RegExp(d.pattern); console.log(JSON.stringify(d.samples.map(s=>p.test(s))))`, pattern, samples)
	if len(result) != len(samples) {
		t.Fatalf("JavaScript grader returned %d results for %d samples", len(result), len(samples))
	}
	for i := range positive {
		if !result[i] {
			t.Errorf("JavaScript grader rejected positive answer %d: %s", i, samples[i])
		}
	}
	for i := len(positive); i < len(samples); i++ {
		if result[i] {
			t.Errorf("JavaScript grader accepted overclaim/invalid answer %d: %s", i-len(positive), samples[i])
		}
	}
}
