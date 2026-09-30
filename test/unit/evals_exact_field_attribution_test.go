// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestExactFieldAttributionProductCaseEvidenceAndGrader(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "exact-field-attribution-contract")
	caseData, err := os.ReadFile(filepath.Join(root, "case.yaml"))
	if err != nil || !strings.Contains(string(caseData), "FIXTURE-OWNED-SCAFFOLD") {
		t.Fatalf("case must declare its fixture-owned scaffold: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "mocks", "cub-scout", "explain.md")); !os.IsNotExist(err) {
		t.Fatalf("file-only case must not install an MCP mock: %v", err)
	}

	proofDir := filepath.Join(root, "fixtures", "proof")
	proofBytes, err := os.ReadFile(filepath.Join(proofDir, "proof.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		AfterSource string            `json:"afterSource"`
		Context     string            `json:"context"`
		Resource    string            `json:"resource"`
		Path        string            `json:"path"`
		Files       map[string]string `json:"files"`
	}
	if err := json.Unmarshal(proofBytes, &proof); err != nil {
		t.Fatal(err)
	}
	if proof.AfterSource != "9d3efad1078aa59824a7cc1868fef81d2591d8d2" || proof.Context != "kind-scout-evals-scale" || proof.Resource != "apps/v1 Deployment team-01/api" || proof.Path != `.spec.template.spec.containers[name="api"].image` || len(proof.Files) != 10 {
		t.Fatalf("unexpected proof provenance: %+v", proof)
	}
	for name, want := range proof.Files {
		data, err := os.ReadFile(filepath.Join(proofDir, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != want {
			t.Errorf("%s SHA-256 = %s, want %s", name, got, want)
		}
	}

	for _, tc := range []struct{ name, path, cause string }{
		{name: "after-explain.json", path: proof.Path, cause: "controller-drift"},
		{name: "after-absent-path.json", path: ".spec.nonexistent", cause: "unknown"},
	} {
		data, err := os.ReadFile(filepath.Join(proofDir, tc.name))
		if err != nil {
			t.Fatal(err)
		}
		var cliResult struct {
			Stdout struct {
				FieldAttribution struct {
					Path     string   `json:"path"`
					Cause    string   `json:"cause"`
					Managers []string `json:"managers"`
					Reason   string   `json:"reason"`
				} `json:"fieldAttribution"`
			} `json:"stdout"`
		}
		if err := json.Unmarshal(data, &cliResult); err != nil {
			t.Fatalf("decode %s: %v", tc.name, err)
		}
		if cliResult.Stdout.FieldAttribution.Cause != tc.cause || cliResult.Stdout.FieldAttribution.Path != tc.path {
			t.Errorf("%s field result = %+v, want path=%s cause=%s", tc.name, cliResult.Stdout.FieldAttribution, tc.path, tc.cause)
		}
		if tc.cause == "unknown" && (cliResult.Stdout.FieldAttribution.Reason == "" || len(cliResult.Stdout.FieldAttribution.Managers) != 0) {
			t.Fatalf("absent-path evidence = %+v, want explicit unknown with no invented managers", cliResult.Stdout.FieldAttribution)
		}
	}

	clusterPath := filepath.Join(root, "fixtures", "cluster", "deployments.yaml")
	clusterBytes, err := os.ReadFile(clusterPath)
	if err != nil {
		t.Fatal(err)
	}
	clusterJSON, err := utilyaml.ToJSON(clusterBytes)
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(clusterJSON, &recorded); err != nil || len(recorded.Items) != 1 {
		t.Fatalf("decode exact-field fixture: %v (items=%d)", err, len(recorded.Items))
	}
	var obj unstructured.Unstructured
	if err := json.Unmarshal(recorded.Items[0], &obj.Object); err != nil {
		t.Fatal(err)
	}
	owner := agent.DetectOwnership(&obj)
	if owner.Type != agent.OwnerHelm {
		t.Fatalf("fixture owner = %q, want Helm label", owner.Type)
	}
	field, ok, incomplete := agent.AttributeFieldPath(&obj, owner, proof.Path)
	if !ok || incomplete || field.Cause != agent.CauseControllerDrift || len(field.Managers) != 1 || field.Managers[0] != "helm" {
		t.Fatalf("exact path = (%+v, %v, %v), want helm only", field, ok, incomplete)
	}
	if rollup := agent.AttributeFieldMutation(&obj, owner); rollup.Cause != field.Cause || rollup.ManagerHint != "helm" {
		t.Fatalf("fixture resource rollup unexpectedly differs: %+v", rollup)
	}
	missing, ok, _ := agent.AttributeFieldPath(&obj, owner, ".spec.nonexistent")
	if ok || missing.Cause != agent.CauseUnknown || len(missing.Managers) != 0 {
		t.Fatalf("missing exact path = (%+v, %v), want unknown without rollup fallback", missing, ok)
	}

	grader, err := os.ReadFile(filepath.Join(root, "graders", "field-attribution.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindStringSubmatch(string(grader))
	if len(match) != 2 {
		t.Fatalf("grader must have a single-quoted regex pattern: %s", grader)
	}
	pattern, err := regexp.Compile("(?s)" + match[1])
	if err != nil {
		t.Fatalf("compile attribution grader: %v", err)
	}
	good := `{"resource":"Deployment/api","namespace":"team-01","field":{"path":".spec.template.spec.containers[name=\"api\"].image","cause":"controller-drift","managers":["helm"]},"absentField":{"path":".spec.nonexistent","cause":"unknown","reason":"No decodable managedFields entry claims this exact path."},"humanActor":"unknown","latestWriter":"unknown","evidenceBoundary":"recorded snapshot; sequential, not atomic; Helm-labelled fixture is not proof of Helm reconciliation"}`
	if !pattern.MatchString(good) {
		t.Fatalf("grader rejected correct evidence contract: %s", good)
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, []byte(good), "", "  "); err != nil {
		t.Fatal(err)
	}
	if !pattern.MatchString(pretty.String()) {
		t.Fatalf("grader rejected correct multiline JSON evidence contract: %s", pretty.String())
	}
	if !pattern.MatchString(" \n" + good + "\n\t") {
		t.Fatal("grader rejected correct JSON with outer whitespace")
	}
	for _, bad := range []string{
		strings.Replace(good, `["helm"]`, `["kubectl-set"]`, 1),
		strings.Replace(good, `"cause":"unknown"`, `"cause":"controller-drift"`, 1),
		strings.Replace(good, `"humanActor":"unknown"`, `"humanActor":"operator"`, 1),
		strings.Replace(good, `"latestWriter":"unknown"`, `"latestWriter":"helm"`, 1),
		strings.Replace(good, `"evidenceBoundary":"recorded snapshot; sequential, not atomic; Helm-labelled fixture is not proof of Helm reconciliation"`, `"evidenceBoundary":"Helm changed the field last"`, 1),
	} {
		if pattern.MatchString(bad) {
			t.Errorf("grader accepted unsupported attribution claim: %s", bad)
		}
	}

	workspace := t.TempDir()
	scaffold, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scaffold)
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run exact-field scaffold: %v\n%s", err, output)
	}
	for name := range proof.Files {
		want, err := os.ReadFile(filepath.Join(proofDir, name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(workspace, "evidence", "exact-field-attribution", name))
		if err != nil || string(got) != string(want) {
			t.Errorf("scaffold proof %s mismatch: %v", name, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "cluster", "deployments.yaml")); err != nil || string(got) != string(clusterBytes) {
		t.Errorf("scaffold cluster export mismatch: %v", err)
	}
}
