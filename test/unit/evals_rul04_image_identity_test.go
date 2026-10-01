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
)

var rul04Files = map[string]string{
	"capture-scope.json":       "c64b226696b1b4962ee67f9ac16fb71b60d58b3837c90532c8a09ded2daef6a1",
	"desired-statefulset.yaml": "f1b06b67456667f073a01e602ecd5ba35dc70eb9ce76b2d7023bca025a4e2541",
	"statefulset.json":         "6e51fce81ff19b1bd548b312e510d01e098dfab7cd9c962fdf1cf273d1aa32ab",
	"pods.json":                "b6cbaae55242a2074e7b2ea9db2ed82dd641c3e1e6fbf1552cb40e2d9f4ae9ab",
}

func TestRUL04RawCaptureAndEqualScaffold(t *testing.T) {
	checkRUL04ImageIdentityScaffold(t, filepath.Join("..", "..", "evals", "rul04-image-identity"))
}

func checkRUL04ImageIdentityScaffold(t *testing.T, root string) {
	t.Helper()
	files := make(map[string][]byte, len(rul04Files))
	for name, want := range rul04Files {
		b, err := os.ReadFile(filepath.Join(root, "fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		got := hex.EncodeToString(sum[:])
		if want == strings.Repeat("0", 64) || got != want {
			t.Fatalf("%s sha256=%s want=%s", name, got, want)
		}
		files[name] = b
	}
	var scope struct {
		Capture struct {
			Atomic   bool   `json:"atomicSnapshot"`
			Start    string `json:"startedAt"`
			End      string `json:"endedAt"`
			Revision string `json:"sourceRevision"`
		} `json:"capture"`
		Observations []struct {
			Path    string `json:"path"`
			RawFile string `json:"rawFile"`
			RawSha  string `json:"rawSha256"`
			Status  int    `json:"httpStatus"`
		} `json:"observations"`
		AuthoredBundle struct {
			DesiredFile  string `json:"desiredManifestFile"`
			DesiredBytes int    `json:"desiredManifestBytes"`
			DesiredSHA   string `json:"desiredManifestSha256"`
			Meaning      string `json:"meaning"`
		} `json:"authoredBundle"`
		Limitations []string `json:"limitations"`
	}
	if err := json.Unmarshal(files["capture-scope.json"], &scope); err != nil {
		t.Fatal(err)
	}
	if scope.Capture.Atomic || scope.Capture.Revision != "fd4963872b9875bed01b9c4a2fdbab5c6db2e635" || len(scope.Observations) != 2 {
		t.Fatalf("capture scope overclaims or lacks provenance: %+v", scope)
	}
	for _, obs := range scope.Observations {
		if obs.Status != 200 || obs.RawSha == "" || rul04Files[obs.RawFile] != obs.RawSha {
			t.Fatalf("invalid API observation: %+v", obs)
		}
	}
	desired := files["desired-statefulset.yaml"]
	desiredSHA := sha256.Sum256(desired)
	if scope.AuthoredBundle.DesiredFile != "desired-statefulset.yaml" || scope.AuthoredBundle.DesiredBytes != len(desired) || scope.AuthoredBundle.DesiredSHA != hex.EncodeToString(desiredSHA[:]) || !strings.Contains(scope.AuthoredBundle.Meaning, "blobs are not included") {
		t.Fatalf("authored manifest provenance missing or overclaims staged OCI evidence: %+v", scope.AuthoredBundle)
	}
	for _, forbidden := range []string{"derivedScout", "identityConclusion", "runtimeImageID", "validation", "verdict"} {
		if bytes.Contains(files["capture-scope.json"], []byte(forbidden)) {
			t.Fatalf("scope leaks derived answer field %q", forbidden)
		}
	}
	if !bytes.Contains(files["desired-statefulset.yaml"], []byte("image: registry.k8s.io/pause:3.10")) || bytes.Contains(files["desired-statefulset.yaml"], []byte("@sha256:")) {
		t.Fatal("authored image intent is not the expected tag-only reference")
	}
	var ss struct {
		Metadata struct {
			UID, ResourceVersion string
			Generation           int
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Spec struct {
					Containers []struct{ Name, Image string } `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(files["statefulset.json"], &ss); err != nil {
		t.Fatal(err)
	}
	var pods struct {
		Items []struct {
			Metadata struct {
				Name, UID string
				Owner     []struct {
					APIVersion, Kind, Name, UID string
					Controller                  bool
				} `json:"ownerReferences"`
			} `json:"metadata"`
			Status struct {
				Phase      string
				Conditions []struct{ Type, Status string }
				Containers []struct {
					Name, Image, ImageID string
					Ready                bool
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(files["pods.json"], &pods); err != nil {
		t.Fatal(err)
	}
	if ss.Metadata.UID != "c260243a-e06c-4529-b514-c977de02f5bc" || ss.Spec.Template.Spec.Containers[0].Image != "registry.k8s.io/pause:3.10" || len(pods.Items) != 1 {
		t.Fatalf("unexpected raw StatefulSet intent/identity: %+v", ss)
	}
	pod := pods.Items[0]
	if pod.Metadata.UID != "76d359cb-8889-479c-9de4-74597bf40424" || len(pod.Metadata.Owner) != 1 || pod.Metadata.Owner[0].UID != ss.Metadata.UID || pod.Metadata.Owner[0].APIVersion != "apps/v1" || pod.Metadata.Owner[0].Controller != true || pod.Status.Phase != "Running" || len(pod.Status.Containers) != 1 || !pod.Status.Containers[0].Ready || pod.Status.Containers[0].ImageID != "sha256:afb61768ce381961ca0beff95337601f29dc70ff3ed14e5e4b3e5699057e6aa8" {
		t.Fatalf("unexpected actual Pod owner/runtime evidence: %+v", pod)
	}
	for arm := 0; arm < 2; arm++ {
		workspace := t.TempDir()
		script, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", script)
		cmd.Dir = workspace
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("scaffold arm %d: %v: %s", arm, err, out)
		}
		entries, err := os.ReadDir(filepath.Join(workspace, "cluster"))
		if err != nil || len(entries) != len(files) {
			t.Fatalf("arm %d staged unexpected evidence files: %v", arm, err)
		}
		for name, want := range files {
			got, err := os.ReadFile(filepath.Join(workspace, "cluster", name))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("arm %d staged different %s: %v", arm, name, err)
			}
		}
	}
}

func TestRUL04StrictAnswerContract(t *testing.T) {
	root := filepath.Join("..", "..", "evals", "rul04-image-identity")
	prompt, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.SplitN(string(prompt), "---\n", 3)
	if len(body) != 3 {
		t.Fatal("prompt frontmatter is malformed")
	}
	for _, answerLeak := range []string{"c260243a-e06c-4529-b514-c977de02f5bc", "76d359cb-8889-479c-9de4-74597bf40424", "afb61768ce381961ca0beff95337601f29dc70ff3ed14e5e4b3e5699057e6aa8", "The Pod is Running and Ready", "Therefore the intended-image identity verdict is UNKNOWN"} {
		if strings.Contains(body[2], answerLeak) {
			t.Errorf("prompt body leaks expected answer %q", answerLeak)
		}
	}
	normalizedBody := strings.Join(strings.Fields(body[2]), " ")
	if strings.Contains(normalizedBody, "image_identity_verdict: UNKNOWN") {
		t.Fatal("prompt contains a stale fixed-verdict encoding bullet")
	}
	for _, contract := range []string{"image_identity_verdict", "runtime_image_id", "in any order", "exactly these string-valued keys", "MATCH", "MISMATCH", "UNKNOWN", "OBSERVED", "MISSING"} {
		if !strings.Contains(normalizedBody, contract) {
			t.Errorf("prompt lacks neutral encoding contract %q", contract)
		}
	}
	grader, err := os.ReadFile(filepath.Join(root, "graders", "verified-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^pattern: "(.*)"$`).FindSubmatch(grader)
	if len(m) != 2 {
		t.Fatalf("grader regex missing: %s", grader)
	}
	var pattern string
	if err := json.Unmarshal(append(append([]byte{'"'}, m[1]...), '"'), &pattern); err != nil {
		t.Fatal(err)
	}
	answer := map[string]string{
		"image_identity_verdict": "UNKNOWN", "unknown_reason": "IMMUTABLE_INTENT_DIGEST_UNAVAILABLE",
		"intended_image": "registry.k8s.io/pause:3.10", "statefulset_uid": "c260243a-e06c-4529-b514-c977de02f5bc",
		"pod_name": "rul04-pause-0", "pod_uid": "76d359cb-8889-479c-9de4-74597bf40424",
		"pod_owner_uid": "c260243a-e06c-4529-b514-c977de02f5bc", "runtime_image": "registry.k8s.io/pause:3.10",
		"runtime_image_id":        "sha256:afb61768ce381961ca0beff95337601f29dc70ff3ed14e5e4b3e5699057e6aa8",
		"runtime_identity_status": "OBSERVED", "workload_health": "READY_RUNNING", "applied_source_binding": "UNKNOWN",
		"observation_scope": "SEQUENTIAL_NON_ATOMIC", "evidence": "capture-scope.json+desired-statefulset.yaml+pods.json+statefulset.json",
	}
	goodBytes, _ := json.Marshal(answer)
	good := string(goodBytes)
	bad := []string{
		strings.Replace(good, `"image_identity_verdict":"UNKNOWN"`, `"image_identity_verdict":"MATCH"`, 1),
		strings.Replace(good, `"unknown_reason":"IMMUTABLE_INTENT_DIGEST_UNAVAILABLE"`, `"unknown_reason":"UNSUPPORTED_WORKLOAD_ADAPTER"`, 1),
		strings.Replace(good, `"runtime_identity_status":"OBSERVED"`, `"runtime_identity_status":"MISSING"`, 1),
		strings.Replace(good, `"statefulset_uid":"c260243a-e06c-4529-b514-c977de02f5bc"`, `"statefulset_uid":"wrong"`, 1),
		strings.Replace(good, `"pod_uid":"76d359cb-8889-479c-9de4-74597bf40424"`, `"pod_uid":"wrong"`, 1),
		strings.Replace(good, `"pod_owner_uid":"c260243a-e06c-4529-b514-c977de02f5bc"`, `"pod_owner_uid":"wrong"`, 1),
		strings.Replace(good, `"runtime_image":"registry.k8s.io/pause:3.10"`, `"runtime_image":"pause:latest"`, 1),
		strings.Replace(good, `"runtime_image_id":"sha256:afb61768ce381961ca0beff95337601f29dc70ff3ed14e5e4b3e5699057e6aa8"`, `"runtime_image_id":"UNKNOWN"`, 1),
		strings.Replace(good, `"runtime_identity_status":"OBSERVED"`, `"runtime_identity_status":"MISSING"`, 1),
		strings.TrimSuffix(good, "}") + `,"extra":"value"}`,
		strings.Replace(good, `"runtime_image_id":"sha256:afb61768ce381961ca0beff95337601f29dc70ff3ed14e5e4b3e5699057e6aa8"`, `"runtime_image_id":"sha256:afb61768ce381961ca0beff95337601f29dc70ff3ed14e5e4b3e5699057e6aa8","runtime_image_id":"other"`, 1),
		strings.Replace(good, `,"runtime_image_id":"sha256:afb61768ce381961ca0beff95337601f29dc70ff3ed14e5e4b3e5699057e6aa8"`, "", 1),
		"Answer: " + good,
	}
	for i, candidate := range bad {
		if candidate == good {
			t.Fatalf("negative candidate %d did not mutate reference answer", i)
		}
	}
	samples := append([]string{good}, bad...)
	var reordered strings.Builder
	reordered.WriteByte('{')
	keys := []string{"evidence", "observation_scope", "applied_source_binding", "workload_health", "runtime_identity_status", "runtime_image_id", "runtime_image", "pod_owner_uid", "pod_uid", "pod_name", "statefulset_uid", "intended_image", "unknown_reason", "image_identity_verdict"}
	for i, key := range keys {
		if i > 0 {
			reordered.WriteByte(',')
		}
		kb, _ := json.Marshal(key)
		vb, _ := json.Marshal(answer[key])
		reordered.Write(kb)
		reordered.WriteByte(':')
		reordered.Write(vb)
	}
	reordered.WriteByte('}')
	samples = append(samples, string(reordered.String()))
	got := runOCIIdentityRegex(t, "node", `const d=JSON.parse(require("fs").readFileSync(0,"utf8"));const p=new RegExp(d.pattern,d.flags);console.log(JSON.stringify(d.samples.map(s=>p.test(s))))`, pattern, samples)
	if len(got) != len(samples) || !got[0] || !got[len(got)-1] {
		t.Fatalf("strict grader rejected valid answer: %v", got)
	}
	for i := 1; i < len(got)-1; i++ {
		if got[i] {
			t.Errorf("grader accepted negative %d: %s", i-1, samples[i])
		}
	}
}

func TestRUL04MappingPreservesFrozenScope(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "evals", "benchmark-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Status string `json:"status"`
		Groups []struct {
			Weight float64 `json:"weight"`
			Cases  []struct {
				ID           string   `json:"id"`
				Question     string   `json:"question"`
				Reference    string   `json:"reference"`
				Controls     []string `json:"controls"`
				Status       string   `json:"status"`
				ExistingCase string   `json:"existing_case"`
			} `json:"cases"`
		} `json:"groups"`
		Execution struct {
			Paid bool `json:"paid_runs_authorized_by_this_manifest"`
		} `json:"execution"`
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	count := 0
	groupWeights := map[float64]bool{}
	found := false
	for _, g := range manifest.Groups {
		groupWeights[g.Weight] = true
		for _, c := range g.Cases {
			count++
			if c.ID == "RUL-04" {
				found = true
				if c.Question != "Unsupported workload/image proof" || c.Reference != "Return unknown when the workload/image relationship is unsupported or lacks identity evidence." || len(c.Controls) != 1 || c.Controls[0] != "No tag-based or name-based image proof" {
					t.Fatalf("frozen RUL-04 definition changed: %+v", c)
				}
				if c.Status != "raw_recording_prepared_not_run" || c.ExistingCase != "evals/rul04-image-identity" {
					t.Fatalf("unexpected RUL-04 admission: %+v", c)
				}
			}
		}
	}
	if !found || count != 24 || len(groupWeights) != 1 || manifest.Status != "frozen_design_not_executable" || manifest.Execution.Paid {
		t.Fatalf("RUL-04 mapping changed frozen suite boundaries: found=%v cases=%d weights=%v status=%s paid=%v", found, count, groupWeights, manifest.Status, manifest.Execution.Paid)
	}
}
