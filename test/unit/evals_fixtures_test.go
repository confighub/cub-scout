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
	"strings"
	"testing"
)

func TestHealthMeasurementContractEvidenceIsRecordedAndScoped(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "health-measurement-contract", "fixtures", "proof")
	proofBytes, err := os.ReadFile(filepath.Join(root, "proof.json"))
	if err != nil {
		t.Fatal(err)
	}
	var proof struct {
		Resource                             string            `json:"resource"`
		BeforeSource                         string            `json:"beforeSource"`
		AfterSource                          string            `json:"afterSource"`
		SameUIDAndResourceVersionBeforeAfter bool              `json:"sameUidAndResourceVersionBeforeAfter"`
		Mutations                            string            `json:"mutations"`
		Files                                map[string]string `json:"files"`
	}
	if err := json.Unmarshal(proofBytes, &proof); err != nil {
		t.Fatal(err)
	}
	if proof.Resource != "apps/v1 Deployment team-02/auth" || proof.BeforeSource != "v2.12.4 at 11c3e38" || proof.AfterSource != "8637c81" || !proof.SameUIDAndResourceVersionBeforeAfter || proof.Mutations != "none; all calls read-only" {
		t.Fatalf("unexpected evidence provenance: %+v", proof)
	}
	for name, wantHash := range proof.Files {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != wantHash {
			t.Errorf("%s SHA-256 = %s, want %s", name, got, wantHash)
		}
	}

	var beforeObject, afterObject struct {
		Metadata struct {
			UID             string `json:"uid"`
			ResourceVersion string `json:"resourceVersion"`
		} `json:"metadata"`
	}
	for name, target := range map[string]interface{}{"before-object.json": &beforeObject, "after-object.json": &afterObject} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatal(err)
		}
	}
	if beforeObject.Metadata.UID == "" || beforeObject.Metadata.UID != afterObject.Metadata.UID || beforeObject.Metadata.ResourceVersion == "" || beforeObject.Metadata.ResourceVersion != afterObject.Metadata.ResourceVersion {
		t.Fatalf("object identity changed: before=%+v after=%+v", beforeObject.Metadata, afterObject.Metadata)
	}

	for name, wantMeasurement := range map[string]bool{"before-mcp.json": false, "after-mcp.json": true} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		if err := json.Unmarshal(data, &response); err != nil || len(response.Result.Content) != 1 {
			t.Fatalf("decode %s MCP response: %v", name, err)
		}
		var explain map[string]interface{}
		if err := json.Unmarshal([]byte(response.Result.Content[0].Text), &explain); err != nil {
			t.Fatalf("decode %s explain payload: %v", name, err)
		}
		if explain["health"] != "Unavailable" {
			t.Errorf("%s legacy health = %v, want Unavailable", name, explain["health"])
		}
		change, ok := explain["currentChange"].(map[string]interface{})
		if !ok || change["verdict"] != "PASS" {
			t.Errorf("%s currentChange = %v, want PASS", name, explain["currentChange"])
		}
		measurement, ok := explain["healthMeasurement"].(map[string]interface{})
		if wantMeasurement && (!ok || measurement["status"] != "unmeasured" || measurement["scope"] != "controller-chain") {
			t.Errorf("%s healthMeasurement = %v, want unmeasured controller-chain", name, explain["healthMeasurement"])
		}
		if !wantMeasurement && ok {
			t.Errorf("%s unexpectedly contains healthMeasurement: %v", name, measurement)
		}
	}
	caseRoot := filepath.Join("..", "..", "evals", "health-measurement-contract")
	responseBytes, err := os.ReadFile(filepath.Join(root, "after-mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(responseBytes, &recorded); err != nil || len(recorded.Result.Content) != 1 {
		t.Fatalf("decode recorded after MCP response: %v", err)
	}
	mock, err := os.ReadFile(filepath.Join(caseRoot, "mocks", "cub-scout", "explain.md"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(mock), "---", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[1]) != "type: fixed" || strings.TrimSpace(parts[2]) != strings.TrimSpace(recorded.Result.Content[0].Text) {
		t.Fatal("case-scoped explain mock does not equal the recorded MCP answer")
	}
	grader, err := os.ReadFile(filepath.Join(caseRoot, "graders", "health-schema.md"))
	if err != nil {
		t.Fatal(err)
	}
	graderText := string(grader)
	if !strings.Contains(graderText, "flags: s") || strings.Contains(graderText, "(?is)") || strings.Contains(graderText, "with-only") {
		t.Fatalf("grader must be a portable required schema check: %s", graderText)
	}
	workspace := t.TempDir()
	scaffoldPath, err := filepath.Abs(filepath.Join(caseRoot, "scaffold.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scaffoldPath)
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run product-contract scaffold: %v\n%s", err, output)
	}
	for _, name := range []string{"before-object.json", "after-object.json", "before-explain.json", "after-explain.json", "before-mcp.json", "after-mcp.json", "proof.json"} {
		want, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(workspace, "evidence", "health-measurement-contract", name))
		if err != nil || string(got) != string(want) {
			t.Errorf("scaffold evidence %s mismatch: %v", name, err)
		}
	}
}

// Each eval run starts in an empty workspace; a case's scaffold.sh writes the
// recorded cluster export there with the files embedded. Every case must
// write exactly the recording, so both eval arms and every case see the same
// evidence (#603).
func TestEvalScaffoldsWriteTheRecordedExport(t *testing.T) {
	root := filepath.Join("..", "..", "evals")
	for _, sc := range []struct{ name, export, cases string }{
		{"main", filepath.Join(root, "fixtures", "cluster"), filepath.Join(root, "*", "case.yaml")},
		{"scale", filepath.Join(root, "fixtures", "scale", "cluster"), filepath.Join(root, "scale", "*", "case.yaml")},
	} {
		t.Run(sc.name, func(t *testing.T) { checkScaffolds(t, sc.export, sc.cases) })
	}
}

func TestEvalLiveOnlyCasesHaveNoExportAndGuardMocks(t *testing.T) {
	for _, name := range []string{"scale-ownership-counts", "scale-unmanaged", "scale-whats-failing"} {
		dir := filepath.Join("..", "..", "evals", "scale", name+"-live")
		data, err := os.ReadFile(filepath.Join(dir, "case.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "scaffold_script:") {
			t.Errorf("%s: live-only case must not receive an export scaffold", name)
		}
		if _, err := os.Stat(filepath.Join(dir, "scaffold.sh")); !os.IsNotExist(err) {
			t.Errorf("%s: unexpected scaffold or filesystem error: %v", name, err)
		}
		for _, tool := range []string{"doctor", "explain", "gitops_status", "map", "release_check", "scan", "trace"} {
			mock, err := os.ReadFile(filepath.Join(dir, "mocks", "cub-scout", tool+".md"))
			if err != nil || !strings.Contains(string(mock), "error: true") || !strings.Contains(string(mock), "--mocks off") {
				t.Errorf("%s/%s: missing live-only guard: %v", name, tool, err)
			}
		}
	}
}

func checkScaffolds(t *testing.T, export, casesGlob string) {
	t.Helper()
	recorded, err := filepath.Glob(filepath.Join(export, "*.yaml"))
	if err != nil || len(recorded) == 0 {
		t.Fatalf("no recorded export under %s: %v", export, err)
	}
	cases, _ := filepath.Glob(casesGlob)
	if len(cases) == 0 {
		t.Fatalf("no eval cases match %s", casesGlob)
	}
	for _, caseYAML := range cases {
		caseDir := filepath.Dir(caseYAML)
		caseExport, err := filepath.Glob(filepath.Join(caseDir, "fixtures", "cluster", "*.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		caseExportDir := export
		if len(caseExport) > 0 {
			caseExportDir = filepath.Join(caseDir, "fixtures", "cluster")
		}
		data, err := os.ReadFile(caseYAML)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "scaffold_script: scaffold.sh") {
			// Live-only cases read the cluster through cub-scout, not an export.
			prompt, _ := os.ReadFile(filepath.Join(filepath.Dir(caseYAML), "prompt.md"))
			if strings.Contains(string(prompt), "live-only") {
				continue
			}
			t.Errorf("%s: every case reads the export through scaffold.sh", caseYAML)
			continue
		}
		script, err := os.ReadFile(filepath.Join(caseDir, "scaffold.sh"))
		if err != nil {
			t.Errorf("%s: %v", caseYAML, err)
			continue
		}
		written := scaffoldFiles(string(script))
		caseRecorded, err := filepath.Glob(filepath.Join(caseExportDir, "*.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, src := range caseRecorded {
			want, _ := os.ReadFile(src)
			name := filepath.Base(src)
			if got, ok := written[name]; !ok || got != strings.TrimRight(string(want), "\n") {
				t.Errorf("%s/scaffold.sh does not write its recorded %s; regenerate from the case's fixture source", caseDir, name)
			}
		}
	}
}

// scaffoldFiles returns the files a generated scaffold.sh writes, by name.
func scaffoldFiles(script string) map[string]string {
	const delim = "CUB_SCOUT_EVAL_EOF"
	files := map[string]string{}
	lines := strings.Split(script, "\n")
	for i := 0; i < len(lines); i++ {
		name, ok := strings.CutPrefix(lines[i], "cat > cluster/")
		if !ok {
			continue
		}
		name, _, _ = strings.Cut(name, " ")
		var body []string
		for i++; i < len(lines) && lines[i] != delim; i++ {
			body = append(body, lines[i])
		}
		files[name] = strings.Join(body, "\n")
	}
	return files
}
