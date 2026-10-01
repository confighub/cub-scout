// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var inv04FixtureHashes = map[string]string{
	"capture-scope.json":                  "c8a098daf301f02cc362968b8c78e3551fcf30e54453e46aea205a80a9180bb3",
	"readable-populated-deployments.json": "c98c09f2cb1675b58455ae124d0584ecc948394013fbb0b437c39f6b35e4a1b6",
	"readable-empty-deployments.json":     "8d70075f02a75588293ec2b45c8b765cf39d8d268366418ce76dce61386e1aaf",
	"denied-deployments.json":             "e80dc99c4fd3f8325744d2e030dea4ad1e5fe4cf19056d62d52c198f632077d5",
}

func TestINV04RBACEvidenceAndGrader(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "inv04-rbac")
	fixtures := readINV04Fixtures(t, root)
	if err := validateINV04Capture(fixtures); err != nil {
		t.Fatal(err)
	}
	checkINV04PromptAndGrader(t, root)
}

func readINV04Fixtures(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := make(map[string][]byte, len(inv04FixtureHashes))
	for name, want := range inv04FixtureHashes {
		data, err := os.ReadFile(filepath.Join(root, "fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Fatalf("%s SHA-256=%s, want %s", name, got, want)
		}
		files[name] = data
	}
	return files
}

func validateINV04Capture(files map[string][]byte) error {
	var populated struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Items      []struct {
			Metadata struct {
				Name            string `json:"name"`
				Namespace       string `json:"namespace"`
				UID             string `json:"uid"`
				ResourceVersion string `json:"resourceVersion"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(files["readable-populated-deployments.json"], &populated); err != nil {
		return err
	}
	want := [][4]string{
		{"inv04-readable-populated", "inv04-api", "c9a5e895-9724-4a43-b42c-840058a2a2e1", "474"},
		{"inv04-readable-populated", "inv04-worker", "60068abb-8e05-4c86-a5d9-1ab1f64c3662", "475"},
	}
	if populated.APIVersion != "apps/v1" || populated.Kind != "DeploymentList" || len(populated.Items) != len(want) {
		return fmt.Errorf("populated response identity kind=%s/%s items=%d", populated.APIVersion, populated.Kind, len(populated.Items))
	}
	for i, item := range populated.Items {
		got := [4]string{item.Metadata.Namespace, item.Metadata.Name, item.Metadata.UID, item.Metadata.ResourceVersion}
		if got != want[i] {
			return fmt.Errorf("populated response item %d identity=%v, want %v", i, got, want[i])
		}
	}

	var empty struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Items      []any  `json:"items"`
	}
	if err := json.Unmarshal(files["readable-empty-deployments.json"], &empty); err != nil {
		return err
	}
	if empty.APIVersion != "apps/v1" || empty.Kind != "DeploymentList" || len(empty.Items) != 0 {
		return fmt.Errorf("empty response is not a readable empty DeploymentList: %+v", empty)
	}
	var denied struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Reason     string `json:"reason"`
		Code       int    `json:"code"`
		Details    struct {
			Group string `json:"group"`
			Kind  string `json:"kind"`
		} `json:"details"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(files["denied-deployments.json"], &denied); err != nil {
		return err
	}
	if denied.APIVersion != "v1" || denied.Kind != "Status" || denied.Code != 403 || denied.Reason != "Forbidden" || denied.Details.Group != "apps" || denied.Details.Kind != "deployments" || !strings.Contains(denied.Message, `namespace "inv04-denied"`) {
		return fmt.Errorf("denied response identity does not establish apps/v1 deployments Forbidden in inv04-denied: %+v", denied)
	}

	var scope struct {
		Schema        string `json:"schema"`
		CaptureWindow struct {
			Atomic bool `json:"atomic"`
		} `json:"captureWindow"`
		Requests []struct {
			Path     string  `json:"path"`
			Status   int     `json:"httpStatus"`
			Filename string  `json:"file"`
			SHA256   string  `json:"sha256"`
			Elapsed  float64 `json:"elapsedSeconds"`
		} `json:"requests"`
		Custody struct {
			CleanupVerified           bool `json:"cleanupVerified"`
			SharedKubeconfigUnchanged bool `json:"sharedKubeconfigUnchanged"`
			ObserverUnchanged         bool `json:"privateObserverKubeconfigUnchanged"`
		} `json:"custody"`
		Limitations []string `json:"limitations"`
	}
	if err := json.Unmarshal(files["capture-scope.json"], &scope); err != nil {
		return err
	}
	if scope.Schema != "inv04.rbac.recorded-evidence.v1" || scope.CaptureWindow.Atomic || len(scope.Requests) != 3 || !scope.Custody.CleanupVerified || !scope.Custody.SharedKubeconfigUnchanged || !scope.Custody.ObserverUnchanged || len(scope.Limitations) < 5 {
		return fmt.Errorf("capture scope overstates completeness or omits custody: %+v", scope)
	}
	wantRequests := []struct {
		path, file string
		status     int
	}{
		{"/apis/apps/v1/namespaces/inv04-readable-populated/deployments", "readable-populated-deployments.json", 200},
		{"/apis/apps/v1/namespaces/inv04-readable-empty/deployments", "readable-empty-deployments.json", 200},
		{"/apis/apps/v1/namespaces/inv04-denied/deployments", "denied-deployments.json", 403},
	}
	for i, row := range scope.Requests {
		w := wantRequests[i]
		if row.Path != w.path || row.Filename != w.file || row.Status != w.status || row.SHA256 != inv04FixtureHashes[w.file] || row.Elapsed <= 0 {
			return fmt.Errorf("request %d mismatch: %+v", i, row)
		}
	}
	return nil
}

func checkINV04RBACScaffold(t *testing.T, root string) {
	t.Helper()
	fixtures := readINV04Fixtures(t, root)
	for arm := 0; arm < 2; arm++ {
		workspace := t.TempDir()
		script, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", script)
		cmd.Dir = workspace
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("run scaffold arm %d: %v\n%s", arm, err, output)
		}
		gotFiles, err := os.ReadDir(filepath.Join(workspace, "cluster"))
		if err != nil || len(gotFiles) != len(fixtures) {
			t.Fatalf("arm %d staged %d files, want %d: %v", arm, len(gotFiles), len(fixtures), err)
		}
		for name, expected := range fixtures {
			got, err := os.ReadFile(filepath.Join(workspace, "cluster", name))
			if err != nil || !bytes.Equal(got, expected) {
				t.Errorf("arm %d fixture %s differs: %v", arm, name, err)
			}
		}
	}
}

func checkINV04PromptAndGrader(t *testing.T, root string) {
	t.Helper()
	prompt, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range []string{
		"`visible_deployments`: comma-separated entries in response item order",
		"`namespace/name@uid#resourceVersion`",
		"`empty_deployment_count`: decimal count scoped to the readable-empty request",
		"`denied_result`: `FORBIDDEN` or `UNKNOWN`",
		"`denied_deployment_count` and `denied_ownership`: use `UNKNOWN` when the",
		"`readable_objects_orphan_status`: `NOT_ESTABLISHED` when the evidence does not",
		"`inventory_completeness`: `PARTIAL`, `COMPLETE`, or `UNKNOWN`",
		"`capture-scope.json+readable-populated-deployments.json+readable-empty-deployments.json+denied-deployments.json`",
		"without surrounding prose",
		"do not prove an orphan",
	} {
		if !strings.Contains(string(prompt), contract) {
			t.Errorf("prompt missing answer/caveat contract %q", contract)
		}
	}
	for _, leaked := range []string{"inv04-api", "inv04-worker", "c9a5e895", "60068abb", "#474", "#475"} {
		if strings.Contains(string(prompt), leaked) {
			t.Errorf("prompt leaks reference answer %q", leaked)
		}
	}
	grader, err := os.ReadFile(filepath.Join(root, "graders", "verified-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '([^']+)'$`).FindSubmatch(grader)
	if len(match) != 2 {
		t.Fatalf("grader missing regex pattern: %s", grader)
	}
	pattern := string(match[1])
	good := `{"populated_namespace":"inv04-readable-populated","visible_deployments":"inv04-readable-populated/inv04-api@c9a5e895-9724-4a43-b42c-840058a2a2e1#474,inv04-readable-populated/inv04-worker@60068abb-8e05-4c86-a5d9-1ab1f64c3662#475","empty_namespace":"inv04-readable-empty","empty_deployment_count":"0","denied_namespace":"inv04-denied","denied_result":"FORBIDDEN","denied_deployment_count":"UNKNOWN","denied_ownership":"UNKNOWN","readable_objects_orphan_status":"NOT_ESTABLISHED","inventory_completeness":"PARTIAL","evidence":"capture-scope.json+readable-populated-deployments.json+readable-empty-deployments.json+denied-deployments.json"}`
	var answer map[string]string
	if err := json.Unmarshal([]byte(good), &answer); err != nil {
		t.Fatal(err)
	}
	keys := []string{"evidence", "inventory_completeness", "readable_objects_orphan_status", "denied_ownership", "denied_deployment_count", "denied_result", "denied_namespace", "empty_deployment_count", "empty_namespace", "visible_deployments", "populated_namespace"}
	var reordered strings.Builder
	reordered.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			reordered.WriteByte(',')
		}
		k, _ := json.Marshal(key)
		v, _ := json.Marshal(answer[key])
		reordered.Write(k)
		reordered.WriteByte(':')
		reordered.Write(v)
	}
	reordered.WriteByte('}')
	pretty, err := json.MarshalIndent(answer, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	positive := []string{good, reordered.String(), "\n" + string(pretty) + "\n"}
	bad := []string{
		strings.Replace(good, `inv04-api@c9a5e895-9724-4a43-b42c-840058a2a2e1#474`, `inv04-api@00000000-0000-0000-0000-000000000000#474`, 1),
		strings.Replace(good, `"denied_deployment_count":"UNKNOWN"`, `"denied_deployment_count":"0"`, 1),
		strings.Replace(good, `"denied_ownership":"UNKNOWN"`, `"denied_ownership":"Native"`, 1),
		strings.Replace(good, `"readable_objects_orphan_status":"NOT_ESTABLISHED"`, `"readable_objects_orphan_status":"ORPHAN"`, 1),
		strings.Replace(good, `"inventory_completeness":"PARTIAL"`, `"inventory_completeness":"COMPLETE"`, 1),
		strings.Replace(good, `"empty_deployment_count":"0"`, `"empty_deployment_count":"2"`, 1),
		strings.Replace(good, `,"evidence":"capture-scope.json+readable-populated-deployments.json+readable-empty-deployments.json+denied-deployments.json"`, ``, 1),
		strings.Replace(good, `,"evidence":"capture-scope.json+readable-populated-deployments.json+readable-empty-deployments.json+denied-deployments.json"`, `,"extra":"claim","evidence":"capture-scope.json+readable-populated-deployments.json+readable-empty-deployments.json+denied-deployments.json"`, 1),
		strings.Replace(good, `,"evidence":"capture-scope.json+readable-populated-deployments.json+readable-empty-deployments.json+denied-deployments.json"`, `,"evidence":"capture-scope.json+readable-populated-deployments.json+readable-empty-deployments.json+denied-deployments.json","evidence":"capture-scope.json+readable-populated-deployments.json+readable-empty-deployments.json+denied-deployments.json"`, 1),
		"Answer: " + good,
		good + "\nThis namespace is globally unmanaged and orphaned.",
	}
	samples := append(positive, bad...)
	result := runOCIIdentityRegex(t, "node", `const d=JSON.parse(require("fs").readFileSync(0,"utf8")); const p=new RegExp(d.pattern); console.log(JSON.stringify(d.samples.map(s=>p.test(s))))`, pattern, samples)
	if len(result) != len(samples) {
		t.Fatalf("grader returned %d results for %d samples", len(result), len(samples))
	}
	for i := range positive {
		if !result[i] {
			t.Errorf("grader rejected positive sample %d", i)
		}
	}
	for i := len(positive); i < len(samples); i++ {
		if result[i] {
			t.Errorf("grader accepted negative sample %d", i-len(positive))
		}
	}
}
