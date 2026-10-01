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
	"time"
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
		PrivateKubeconfig                    bool              `json:"privateKubeconfig"`
		CubHiddenFromPath                    bool              `json:"cubHiddenFromPath"`
		Mutations                            string            `json:"mutations"`
		Files                                map[string]string `json:"files"`
	}
	if err := json.Unmarshal(proofBytes, &proof); err != nil {
		t.Fatal(err)
	}
	if proof.Resource != "apps/v1 Deployment team-02/auth" || proof.BeforeSource != "v2.12.4 at 11c3e38" || proof.AfterSource != "8637c81" || !proof.SameUIDAndResourceVersionBeforeAfter || !proof.PrivateKubeconfig || !proof.CubHiddenFromPath || proof.Mutations != "none; all calls read-only" || len(proof.Files) != 6 {
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
	var observationTimes [2]time.Time
	for i, name := range []string{"before-explain.json", "after-explain.json"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var explain struct {
			CurrentChange struct {
				Evidence struct {
					ObservedAt string `json:"observedAt"`
				} `json:"evidence"`
			} `json:"currentChange"`
		}
		if err := json.Unmarshal(data, &explain); err != nil {
			t.Fatal(err)
		}
		observationTimes[i], err = time.Parse(time.RFC3339, explain.CurrentChange.Evidence.ObservedAt)
		if err != nil {
			t.Fatalf("parse %s observation time: %v", name, err)
		}
	}
	if !observationTimes[0].Before(observationTimes[1]) {
		t.Fatalf("evidence does not prove ordered observations: %s then %s", observationTimes[0], observationTimes[1])
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
	caseData, err := os.ReadFile(filepath.Join(caseRoot, "case.yaml"))
	if err != nil || !strings.Contains(string(caseData), "FIXTURE-OWNED-SCAFFOLD") {
		t.Fatalf("case must visibly declare fixture-owned scaffold: %v", err)
	}
	if _, err := os.Stat(filepath.Join(caseRoot, "mocks", "cub-scout", "explain.md")); !os.IsNotExist(err) {
		t.Fatalf("fixture-only case must not install a scoped MCP mock: %v", err)
	}
	grader, err := os.ReadFile(filepath.Join(caseRoot, "graders", "health-schema.md"))
	if err != nil {
		t.Fatal(err)
	}
	graderText := string(grader)
	if !strings.Contains(graderText, "flags: s") || strings.Contains(graderText, "(?is)") || strings.Contains(graderText, "with-only") {
		t.Fatalf("grader must be a portable required schema check: %s", graderText)
	}
	match := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindStringSubmatch(graderText)
	if len(match) != 2 {
		t.Fatalf("grader has no single-quoted regex pattern: %s", graderText)
	}
	pattern, err := regexp.Compile(match[1])
	if err != nil {
		t.Fatalf("compile schema grader pattern: %v", err)
	}
	good := `{"health":"Unavailable","healthMeasurement":{"status":"unmeasured","scope":"controller-chain"},"currentChange":{"verdict":"PASS"}}`
	whitespace := "{\n  \"health\" : \"Unavailable\", \"healthMeasurement\" : { \"status\" : \"unmeasured\",\n \"scope\" : \"controller-chain\" }, \"currentChange\" : {\"verdict\":\"PASS\"}\n}"
	for _, answer := range []string{good, whitespace} {
		if !pattern.MatchString(answer) {
			t.Errorf("schema grader rejected valid output: %s", answer)
		}
	}
	for _, answer := range []string{
		strings.Replace(good, `"unmeasured"`, `"measured"`, 1),
		strings.Replace(good, `"controller-chain"`, `"object-local-readiness"`, 1),
		strings.Replace(good, `"scope":"controller-chain"`, `"scope":"controller-chain","reason":"extra"`, 1),
		"Answer: " + good,
		good + "\nExplanation",
	} {
		if pattern.MatchString(answer) {
			t.Errorf("schema grader accepted invalid output: %s", answer)
		}
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

func TestEvalRecorderPreservesFixtureOwnedScaffold(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required to exercise evals/scripts/record.py")
	}
	temp := t.TempDir()
	caseDir := filepath.Join(temp, "case")
	fixtures := filepath.Join(temp, "fixtures")
	if err := os.MkdirAll(filepath.Join(caseDir), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(fixtures, "cluster"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caseDir, "case.yaml"), []byte("context:\n  scaffold_script: scaffold.sh\n"), 0644); err != nil {
		t.Fatal(err)
	}
	custom := "#!/usr/bin/env bash\n# FIXTURE-OWNED: validate separately\necho custom\n"
	scaffoldPath := filepath.Join(caseDir, "scaffold.sh")
	if err := os.WriteFile(scaffoldPath, []byte(custom), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixtures, "cluster", "deployments.yaml"), []byte("apiVersion: v1\nkind: List\n"), 0644); err != nil {
		t.Fatal(err)
	}
	recorderPath, err := filepath.Abs(filepath.Join("..", "..", "evals", "scripts", "record.py"))
	if err != nil {
		t.Fatal(err)
	}
	pythonCode := "import sys; sys.dont_write_bytecode=True; import importlib.util; s=importlib.util.spec_from_file_location('record',sys.argv[1]); m=importlib.util.module_from_spec(s); s.loader.exec_module(m); m.write_scaffolds({'fixtures':sys.argv[2],'cases':sys.argv[3]})"
	cmd := exec.Command(python, "-c", pythonCode, recorderPath, fixtures, filepath.Join(caseDir, "case.yaml"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run recorder: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "preserving fixture-owned scaffold") {
		t.Fatalf("recorder did not disclose preservation: %s", output)
	}
	got, err := os.ReadFile(scaffoldPath)
	if err != nil || string(got) != custom {
		t.Fatalf("recorder modified fixture-owned scaffold: %v", err)
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
		// These product-contract cases own their fixed evidence and use
		// copy-based scaffolds rather than embedding the suite-wide export.
		// Validate each through dedicated assertions; keep remaining cases on the
		// generic embedded-export path below.
		if filepath.Base(caseDir) == "recorded-explain-contract" {
			checkRecordedExplainCaseScaffold(t, caseDir)
			continue
		}
		if filepath.Base(caseDir) == "consul-ingress-residue" {
			checkConsulHLT03Scaffold(t, caseDir)
			continue
		}
		if filepath.Base(caseDir) == "flux-ready-without-health" {
			checkFluxHLT02ScaffoldBytes(t, caseDir)
			continue
		}
		if filepath.Base(caseDir) == "sveltos-inferred-revision" {
			checkSveltosInferredRevisionCaseScaffold(t, caseDir)
			continue
		}
		if filepath.Base(caseDir) == "kubara-hub-spoke-placement" {
			checkKubaraHubSpokePlacementScaffold(t, caseDir)
			continue
		}
		if filepath.Base(caseDir) == "inv04-rbac" {
			checkINV04RBACScaffold(t, caseDir)
			continue
		}
		if filepath.Base(caseDir) == "pre01-crd" {
			checkPRE01CRDScaffold(t, caseDir)
			continue
		}
		if filepath.Base(caseDir) == "rul03-context" {
			checkRUL03ContextScaffold(t, caseDir)
			continue
		}
		if filepath.Base(caseDir) == "pre02-node-selector" {
			checkPRE02Scaffold(t, caseDir, map[string]string{
				"before-pod.json":    "dec5c8fae0f71860f1309aee1be0455b355b6ea3abbb921360ebd306343adefd",
				"before-nodes.json":  "9808755e4df180b936aa2b68db5ce84aef3bca84b61cc9b2db8a64b007e7374a",
				"before-events.json": "78b187ef42464644a815b8676bf8743fc7eb491f163f35b334d73bccf531f50e",
				"after-pod.json":     "f264204dc296d06590bc357691a1b95db08222f4ac68b3c38935d6ef200832c3",
				"after-nodes.json":   "263097f6d314cac3a7d673b4db7a38f36c437bd2fa1a2d402bb2c0a0dd807ee9",
				"after-events.json":  "4acce317a3a539b80d4fbb7eb7977dc313a888952caa8b64f2096243d40ee3ae",
			})
			continue
		}
		if filepath.Base(caseDir) == "rul04-image-identity" {
			checkRUL04ImageIdentityScaffold(t, caseDir)
			continue
		}
		// DEL-04 is a receipt-only case: it owns immutable public source
		// receipts, not the unrelated suite-wide Kubernetes resource export.
		if filepath.Base(caseDir) == "oci-identity-lifecycle" {
			checkOCIIdentityLifecycleScaffold(t, caseDir)
			continue
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
