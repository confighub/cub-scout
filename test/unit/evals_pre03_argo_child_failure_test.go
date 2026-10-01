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

const pre03CaseRoot = "../../evals/pre03-argo-child-failure"

var pre03FixtureHashes = map[string]string{
	"capture-scope.json":         "a1b64893b8df6ab27200e2b0f08e6bbb1f4413ac8694bfbdb6a24c6f485c5878",
	"argocd-core-root.json":      "5f2fe7e6f3200327567b50abd695a8bbeb85f7c6d5f4e92976798bb5a8e36fff",
	"argocd-core-child.json":     "c8e5c7476a8b0f3a7941587957b63caaa651247715ed537be3a6f3795f0e3ffe",
	"argocd-core-root-tree.txt":  "ff14e194abf60ebfed7429c30ae2c87548875a068a3bca920c10a3b60dd24411",
	"argocd-core-child-tree.txt": "5063434140df63adb2949f8287a88898e7d1c2c2f70a416a7ca3446e3d5a61f3",
	"pod-describe.txt":           "4b1f8b55eccecd18a1e0d2b1b537eef08ddd4bc3323c0b8bf64faf905948d3a7",
	"events.txt":                 "2adf884044616c0a66e01a9f1460e4f7c61579c89a8c7282ec4adc5c007b4526",
}

var pre03SourcePaths = map[string]string{
	"argocd-core-root.json":      "runs/live-helm-confighub-compare/bitnami-spark-ha/argocd-core-root.json",
	"argocd-core-child.json":     "runs/live-helm-confighub-compare/bitnami-spark-ha/argocd-core-child.json",
	"argocd-core-root-tree.txt":  "runs/live-helm-confighub-compare/bitnami-spark-ha/argocd-core-root-tree.txt",
	"argocd-core-child-tree.txt": "runs/live-helm-confighub-compare/bitnami-spark-ha/argocd-core-child-tree.txt",
	"pod-describe.txt":           "runs/live-helm-confighub-compare/bitnami-spark-ha/runtime-diagnostics/confighub-oci-argo/default/spark-worker-0/describe.txt",
	"events.txt":                 "runs/live-helm-confighub-compare/bitnami-spark-ha/runtime-diagnostics/confighub-oci-argo/default/events.txt",
}

var pre03GitBlobs = map[string]string{
	"argocd-core-root.json":      "64e5075ba2256e0a799996f5cd98c2d02a672f94",
	"argocd-core-child.json":     "d7cd66b16862f937accb25fa77787e38d45262d7",
	"argocd-core-root-tree.txt":  "069399dab0ef44b2fdac4a82054ad2125ab7fa28",
	"argocd-core-child-tree.txt": "7d2222eaac7aea65ed4c923feb5be3ffe784b807",
	"pod-describe.txt":           "21b8fc2e8114a3f4d482c064886afd7ade80a869",
	"events.txt":                 "d36cecd6170ee4f1fecb325b6ff98ea06660cfe1",
}

type pre03SourceScope struct {
	Schema        string            `json:"schema"`
	Repository    string            `json:"source_repository"`
	Revision      string            `json:"source_revision"`
	SourceFiles   map[string]string `json:"source_files"`
	FileSHA256    map[string]string `json:"source_file_sha256"`
	GitBlobs      map[string]string `json:"source_git_blob"`
	RunID         string            `json:"run_id"`
	SourceReceipt struct {
		Path       string `json:"path"`
		SHA256     string `json:"sha256"`
		Included   bool   `json:"included_as_model_evidence"`
		ObservedAt string `json:"observedAt"`
	} `json:"source_receipt"`
	Timestamps map[string]string `json:"timestamps"`
	Scope      map[string]string `json:"scope"`
	Cleanup    map[string]string `json:"cleanup"`
}

type pre03ArgoApp struct {
	Metadata struct {
		Name              string            `json:"name"`
		Namespace         string            `json:"namespace"`
		UID               string            `json:"uid"`
		ResourceVersion   string            `json:"resourceVersion"`
		CreationTimestamp string            `json:"creationTimestamp"`
		Annotations       map[string]string `json:"annotations"`
	} `json:"metadata"`
	Status struct {
		ReconciledAt string `json:"reconciledAt"`
		Sync         struct {
			Status string `json:"status"`
		} `json:"sync"`
		Health struct {
			Status             string `json:"status"`
			LastTransitionTime string `json:"lastTransitionTime"`
		} `json:"health"`
		Resources []struct {
			Group     string `json:"group"`
			Version   string `json:"version"`
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			Health    struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"health"`
		} `json:"resources"`
	} `json:"status"`
}

type pre03Answer struct {
	ParentApplication             string `json:"parent_application"`
	ParentSyncStatus              string `json:"parent_sync_status"`
	ParentHealthStatus            string `json:"parent_health_status"`
	ParentTrackedChild            string `json:"parent_tracked_child"`
	ChildApplication              string `json:"child_application"`
	ChildTrackingID               string `json:"child_tracking_id"`
	ChildSyncStatus               string `json:"child_sync_status"`
	ChildHealthStatus             string `json:"child_health_status"`
	FailingStatefulSet            string `json:"failing_statefulset"`
	FailingPod                    string `json:"failing_pod"`
	PodState                      string `json:"pod_state"`
	PodImage                      string `json:"pod_image"`
	PodFailureClass               string `json:"pod_failure_class"`
	PodFailureMessage             string `json:"pod_failure_message"`
	ParentStatusProvesChildHealth string `json:"parent_status_proves_child_health"`
	CrossplaneEvidence            string `json:"crossplane_evidence"`
	ObservationScope              string `json:"observation_scope"`
	CaptureTiming                 string `json:"capture_timing"`
	CleanupStatus                 string `json:"cleanup_status"`
	EvidenceFiles                 string `json:"evidence_files"`
}

func TestPRE03FixturePinsChainAndStrictAnswer(t *testing.T) {
	root := filepath.Clean(pre03CaseRoot)
	files := map[string][]byte{}
	for name, want := range pre03FixtureHashes {
		data, err := os.ReadFile(filepath.Join(root, "fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Fatalf("fixture %s sha256=%s want %s", name, got, want)
		}
		files[name] = data
	}
	var scope pre03SourceScope
	if err := json.Unmarshal(files["capture-scope.json"], &scope); err != nil {
		t.Fatal(err)
	}
	if scope.Schema != "pre03-argo-child-evidence.v1" || scope.Repository != "confighub/helm-expt" || scope.Revision != "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0" || scope.RunID != "helm-expt-parity-spark-mqghfthi-196o" || scope.SourceReceipt.Included || scope.SourceReceipt.ObservedAt != "2026-06-16T10:11:41Z" || scope.SourceReceipt.SHA256 != "ba2be4b6aec3e9e854a9bedbaf2471a44a617f720545bb6e41ffa79b6707c8bd" {
		t.Fatalf("source scope incomplete or unexpected: %+v", scope)
	}
	for name, want := range pre03FixtureHashes {
		if name == "capture-scope.json" {
			continue // this neutral envelope cannot recursively hash itself
		}
		if scope.FileSHA256[name] != want || scope.SourceFiles[name] != pre03SourcePaths[name] || scope.GitBlobs[name] != pre03GitBlobs[name] {
			t.Fatalf("scope source path/hash/blob mismatch for %s: path=%q sha=%q blob=%q", name, scope.SourceFiles[name], scope.FileSHA256[name], scope.GitBlobs[name])
		}
	}
	if len(scope.FileSHA256) != 6 || scope.Scope["controller_branch"] != "Argo-only parent/child Application chain" || scope.Scope["crossplane"] == "" || !strings.Contains(scope.Scope["sequence"], "sequential, not atomic") || !strings.Contains(scope.Scope["current_state"], "not established") || !strings.Contains(scope.Scope["parent_child_join"], "no UID foreign-key") || !strings.Contains(scope.Scope["pod_join"], "no complete Kubernetes object graph") {
		t.Fatalf("source scope overstates or omits coverage: %+v", scope.Scope)
	}
	if scope.Timestamps["parent_created"] != "2026-06-16T10:13:17Z" || scope.Timestamps["parent_health_transition"] != "2026-06-16T10:13:25Z" || scope.Timestamps["parent_reconciled"] != "2026-06-16T10:34:55Z" || scope.Timestamps["child_created"] != "2026-06-16T10:27:43Z" || scope.Timestamps["child_health_transition"] != "2026-06-16T10:27:45Z" || scope.Timestamps["child_reconciled"] != "2026-06-16T10:34:36Z" || !strings.Contains(scope.Timestamps["meaning"], "not file-response capture times") || !strings.Contains(scope.Timestamps["pod_event_ages"], "relative") || !strings.Contains(scope.Timestamps["response_capture_times"], "not supplied") {
		t.Fatalf("timestamp scope inaccurate: %+v", scope.Timestamps)
	}
	if scope.Cleanup["source_receipt_cleanup_result"] != "blocked" || scope.Cleanup["source_receipt_lifecycle_value"] != "cleaned-up" || !strings.Contains(scope.Cleanup["interpretation"], "not confirmed") {
		t.Fatalf("cleanup conflict was not preserved: %+v", scope.Cleanup)
	}

	var parent, child pre03ArgoApp
	if err := json.Unmarshal(files["argocd-core-root.json"], &parent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(files["argocd-core-child.json"], &child); err != nil {
		t.Fatal(err)
	}
	if parent.Metadata.Name != "helm-expt-parity-spark-mqghfthi-196o-cluster" || parent.Metadata.Namespace != "argocd" || parent.Metadata.UID != "493ef705-bc96-4717-8934-961d71043016" || parent.Status.Sync.Status != "Synced" || parent.Status.Health.Status != "Healthy" {
		t.Fatalf("parent identity/status changed: %+v %+v", parent.Metadata, parent.Status)
	}
	var parentRef string
	for _, r := range parent.Status.Resources {
		if r.Group == "argoproj.io" && r.Version == "v1alpha1" && r.Kind == "Application" && r.Namespace == "argocd" && r.Name == "spark-parity" && r.Status == "Synced" {
			parentRef = "argoproj.io/v1alpha1 Application argocd/spark-parity"
		}
	}
	if parentRef == "" {
		t.Fatal("parent status does not contain the exact child Application resource")
	}
	const trackingID = "helm-expt-parity-spark-mqghfthi-196o-cluster:argoproj.io/Application:argocd/spark-parity"
	if child.Metadata.Name != "spark-parity" || child.Metadata.Namespace != "argocd" || child.Metadata.UID != "7c373d17-c2f2-4041-b85f-d4b93e3de3ac" || child.Metadata.Annotations["argocd.argoproj.io/tracking-id"] != trackingID || child.Status.Sync.Status != "Synced" || child.Status.Health.Status != "Progressing" {
		t.Fatalf("child identity/link/status changed: %+v %+v", child.Metadata, child.Status)
	}
	var statefulSetHealth string
	for _, r := range child.Status.Resources {
		if r.Group == "apps" && r.Version == "v1" && r.Kind == "StatefulSet" && r.Namespace == "default" && r.Name == "spark-worker" && r.Status == "Synced" {
			statefulSetHealth = r.Health.Status + ":" + r.Health.Message
		}
	}
	if statefulSetHealth != "Progressing:Waiting for 3 pods to be ready..." {
		t.Fatalf("failing StatefulSet evidence changed: %s", statefulSetHealth)
	}
	tree := string(files["argocd-core-child-tree.txt"])
	if !strings.Contains(tree, "StatefulSet/spark-worker") || !strings.Contains(tree, "Pod/spark-worker-0") || !strings.Contains(tree, "Back-off pulling image") || !strings.Contains(tree, "ErrImagePull") || !strings.Contains(tree, "code = NotFound") {
		t.Fatal("child tree no longer records the StatefulSet-to-Pod failure path")
	}
	podDescribe := string(files["pod-describe.txt"])
	if !strings.Contains(podDescribe, "Image:           docker.io/bitnami/spark:4.0.0-debian-12-r20") || !strings.Contains(podDescribe, "Reason:        ImagePullBackOff") || !strings.Contains(podDescribe, "Ready:           False") {
		t.Fatal("pod describe lost the exact image/state/readiness evidence")
	}
	events := string(files["events.txt"])
	failureMessage := pre03FailureMessage(t, events)
	if !strings.Contains(podDescribe, failureMessage) || !strings.Contains(failureMessage, "rpc error: code = NotFound") || !strings.HasSuffix(failureMessage, "not found") {
		t.Fatalf("exact failure message missing or changed: %q", failureMessage)
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
	if schema.Version != "1.1" || schema.Name != "pre03-argo-child-failure" || schema.Context.Scaffold != "scaffold.sh" {
		t.Fatalf("invalid case schema: %+v", schema)
	}

	prompt, err := os.ReadFile(filepath.Join(root, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	promptText := string(prompt)
	for _, required := range []string{"minified, single-line JSON", "pod_failure_message", "NO_CROSSPLANE_CHAIN_IN_PROVIDED_FILES", "HISTORICAL_SEQUENTIAL_NON_ATOMIC_NOT_CURRENT", "UNCONFIRMED_CONFLICTING_CLEANUP_FIELDS", "UID foreign key"} {
		if !strings.Contains(promptText, required) {
			t.Errorf("prompt missing defined evidence or output contract %q", required)
		}
	}
	for _, leaked := range []string{parent.Metadata.Name, "argocd/spark-parity", trackingID, "docker.io/bitnami/spark:4.0.0-debian-12-r20", failureMessage} {
		if strings.Contains(promptText, leaked) {
			t.Errorf("prompt leaks case answer %q", leaked)
		}
	}

	answer := pre03Answer{
		ParentApplication:             "argocd/helm-expt-parity-spark-mqghfthi-196o-cluster",
		ParentSyncStatus:              parent.Status.Sync.Status,
		ParentHealthStatus:            parent.Status.Health.Status,
		ParentTrackedChild:            parentRef,
		ChildApplication:              "argocd/spark-parity",
		ChildTrackingID:               trackingID,
		ChildSyncStatus:               child.Status.Sync.Status,
		ChildHealthStatus:             child.Status.Health.Status,
		FailingStatefulSet:            "apps/v1 StatefulSet default/spark-worker",
		FailingPod:                    "default/spark-worker-0",
		PodState:                      "WAITING_IMAGE_PULL_BACK_OFF",
		PodImage:                      "docker.io/bitnami/spark:4.0.0-debian-12-r20",
		PodFailureClass:               "ERRIMAGEPULL_AND_IMAGEPULLBACKOFF",
		PodFailureMessage:             failureMessage,
		ParentStatusProvesChildHealth: "NO",
		CrossplaneEvidence:            "NO_CROSSPLANE_CHAIN_IN_PROVIDED_FILES",
		ObservationScope:              "HISTORICAL_SEQUENTIAL_NON_ATOMIC_NOT_CURRENT",
		CaptureTiming:                 "SOURCE_OBJECT_TIMES_ONLY",
		CleanupStatus:                 "UNCONFIRMED_CONFLICTING_CLEANUP_FIELDS",
		EvidenceFiles:                 "capture-scope.json+argocd-core-root.json+argocd-core-child.json+argocd-core-root-tree.txt+argocd-core-child-tree.txt+pod-describe.txt+events.txt",
	}
	grader, err := os.ReadFile(filepath.Join(root, "graders", "verified-answer.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`(?m)^pattern: '(.*)'$`).FindSubmatch(grader)
	if len(match) != 2 {
		t.Fatal("strict PRE-03 grader has no single-quoted pattern")
	}
	pattern, err := regexp.Compile(string(match[1]))
	if err != nil {
		t.Fatalf("compile PRE-03 grader: %v", err)
	}
	good, err := json.Marshal(answer)
	if err != nil || !pattern.Match(good) {
		t.Fatalf("strict grader rejected source-derived answer: err=%v answer=%s", err, good)
	}
	invalid := []struct {
		name   string
		mutate func(pre03Answer) []byte
	}{
		{"missing-field", func(v pre03Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`,"evidence_files":"capture-scope.json+argocd-core-root.json+argocd-core-child.json+argocd-core-root-tree.txt+argocd-core-child-tree.txt+pod-describe.txt+events.txt"`), nil, 1)
		}},
		{"extra-field", func(v pre03Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`{"parent_application":`), []byte(`{"extra":"x","parent_application":`), 1)
		}},
		{"duplicate-key", func(v pre03Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`"parent_health_status":"Healthy"`), []byte(`"parent_health_status":"Healthy","parent_health_status":"Degraded"`), 1)
		}},
		{"wrong-type", func(v pre03Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`"parent_sync_status":"Synced"`), []byte(`"parent_sync_status":true`), 1)
		}},
		{"wrong-parent-child-link", func(v pre03Answer) []byte {
			v.ParentTrackedChild = "argoproj.io/v1alpha1 Application argocd/other"
			b, _ := json.Marshal(v)
			return b
		}},
		{"wrong-tracking-id", func(v pre03Answer) []byte {
			v.ChildTrackingID = "some-other-parent:argoproj.io/Application:argocd/spark-parity"
			b, _ := json.Marshal(v)
			return b
		}},
		{"uid-foreign-key-claim", func(v pre03Answer) []byte {
			v.ChildTrackingID = "uid:493ef705-bc96-4717-8934-961d71043016"
			b, _ := json.Marshal(v)
			return b
		}},
		{"wrong-child-health", func(v pre03Answer) []byte { v.ChildHealthStatus = "Healthy"; b, _ := json.Marshal(v); return b }},
		{"unknown-child-health", func(v pre03Answer) []byte { v.ChildHealthStatus = "UNKNOWN"; b, _ := json.Marshal(v); return b }},
		{"wrong-parent-health", func(v pre03Answer) []byte { v.ParentHealthStatus = "Degraded"; b, _ := json.Marshal(v); return b }},
		{"wrong-statefulset-path", func(v pre03Answer) []byte {
			v.FailingStatefulSet = "apps/v1 StatefulSet default/other"
			b, _ := json.Marshal(v)
			return b
		}},
		{"wrong-pod-state", func(v pre03Answer) []byte { v.PodState = "RUNNING_READY"; b, _ := json.Marshal(v); return b }},
		{"wrong-image", func(v pre03Answer) []byte { v.PodImage += "@sha256:invented"; b, _ := json.Marshal(v); return b }},
		{"wrong-failure-class", func(v pre03Answer) []byte { v.PodFailureClass = "UNKNOWN"; b, _ := json.Marshal(v); return b }},
		{"wrong-failure-message", func(v pre03Answer) []byte {
			v.PodFailureMessage = "image unavailable"
			b, _ := json.Marshal(v)
			return b
		}},
		{"claims-parent-proves-child", func(v pre03Answer) []byte { v.ParentStatusProvesChildHealth = "YES"; b, _ := json.Marshal(v); return b }},
		{"claims-crossplane-chain", func(v pre03Answer) []byte {
			v.CrossplaneEvidence = "CROSSPLANE_CHAIN_OBSERVED"
			b, _ := json.Marshal(v)
			return b
		}},
		{"wrong-scope", func(v pre03Answer) []byte { v.ObservationScope = "ATOMIC_CURRENT"; b, _ := json.Marshal(v); return b }},
		{"wrong-capture-time", func(v pre03Answer) []byte {
			v.CaptureTiming = "PER_RESPONSE_CAPTURE_TIMES"
			b, _ := json.Marshal(v)
			return b
		}},
		{"claims-cleanup", func(v pre03Answer) []byte { v.CleanupStatus = "CLEANUP_CONFIRMED"; b, _ := json.Marshal(v); return b }},
		{"unknown-required-claim", func(v pre03Answer) []byte { v.CleanupStatus = "UNKNOWN"; b, _ := json.Marshal(v); return b }},
		{"wrong-evidence-order", func(v pre03Answer) []byte {
			v.EvidenceFiles = "events.txt+capture-scope.json"
			b, _ := json.Marshal(v)
			return b
		}},
		{"whitespace", func(v pre03Answer) []byte {
			b, _ := json.Marshal(v)
			return bytes.Replace(b, []byte(`"parent_application":`), []byte(`"parent_application": `), 1)
		}},
		{"prose-prefix", func(v pre03Answer) []byte { b, _ := json.Marshal(v); return append([]byte("Answer: "), b...) }},
		{"prose-suffix", func(v pre03Answer) []byte { b, _ := json.Marshal(v); return append(b, []byte(" explanation")...) }},
	}
	for _, tc := range invalid {
		if candidate := tc.mutate(answer); pattern.Match(candidate) {
			t.Errorf("grader accepted %s: %s", tc.name, candidate)
		}
	}
}

func pre03FailureMessage(t *testing.T, events string) string {
	t.Helper()
	for _, line := range strings.Split(events, "\n") {
		if strings.Contains(line, "pod/spark-worker-0") && strings.Contains(line, "Failed to pull image ") {
			idx := strings.Index(line, "Failed to pull image ")
			return strings.TrimSpace(line[idx:])
		}
	}
	t.Fatal("worker Pod image-pull failure message missing from retained Events")
	return ""
}

func TestPRE03PromptDefinesReferenceFieldFormats(t *testing.T) {
	prompt, err := os.ReadFile(filepath.Join(pre03CaseRoot, "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{
		"`parent_application` and `child_application` are\n  `<namespace>/<name>`",
		"`failing_pod` is `<namespace>/<name>`",
		"`parent_tracked_child` and `failing_statefulset` are\n  `<apiVersion> <Kind> <namespace>/<name>`",
		"preserving the literal API version,\n  kind, namespace, and name from the source observation",
		"do not convert between these formats",
	} {
		if !strings.Contains(string(prompt), format) {
			t.Errorf("PRE-03 prompt omits reference format rule %q", format)
		}
	}
}

func TestPRE03ManifestAndScaffold(t *testing.T) {
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
					Repository string            `json:"repository"`
					Revision   string            `json:"revision"`
					Files      map[string]string `json:"fixture_files_sha256"`
					Report     string            `json:"report"`
					Limits     string            `json:"limits"`
				} `json:"source_provenance"`
			} `json:"cases"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "frozen_design_not_executable" || manifest.Execution.Paid {
		t.Fatal("benchmark execution gate changed")
	}
	reportBytes, err := os.ReadFile(filepath.Join("..", "..", "evals", "reports", "2026-10-01-pre03-argo-child-failure.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Classification string            `json:"classification"`
		Repository     string            `json:"source_repository"`
		Revision       string            `json:"source_revision"`
		Files          map[string]string `json:"fixture_files_sha256"`
		Limits         []string          `json:"limits"`
		Paid           bool              `json:"paid_runs_authorized_by_manifest"`
	}
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatal(err)
	}
	if report.Repository != "confighub/helm-expt" || report.Revision != "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0" || report.Classification == "" || report.Paid || len(report.Files) != len(pre03FixtureHashes) {
		t.Fatalf("PRE-03 report provenance/scope invalid: %+v", report)
	}
	for name, want := range pre03FixtureHashes {
		if report.Files[name] != want {
			t.Errorf("report fixture hash %s differs", name)
		}
	}
	joinedLimits := strings.Join(report.Limits, " ")
	if !strings.Contains(joinedLimits, "No Crossplane") || !strings.Contains(joinedLimits, "not atomic") || !strings.Contains(joinedLimits, "cleanup") {
		t.Fatalf("report omits material evidence limitations: %v", report.Limits)
	}
	counts := map[string]int{}
	found := false
	for _, group := range manifest.Groups {
		for _, c := range group.Cases {
			counts[c.Status]++
			if c.ID != "PRE-03" {
				continue
			}
			found = true
			if group.ID != "prerequisites_graph" || group.Weight != 0.1666666667 || c.Question != "Argo/Crossplane child-chain failure" || c.Reference != "Trace parent to the failing child and name its failure evidence." || len(c.Controls) != 1 || c.Controls[0] != "Parent Synced/Ready does not substitute for child coverage" {
				t.Fatal("frozen PRE-03 question/reference/control/weight changed")
			}
			if c.Status != "recorded_projection_prepared_not_run" || c.ExistingCase != "evals/pre03-argo-child-failure" || !strings.Contains(c.Admission, "not run or admitted") || c.Provenance.Repository != "confighub/helm-expt" || c.Provenance.Revision != "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0" || c.Provenance.Report != "evals/reports/2026-10-01-pre03-argo-child-failure.json" || !strings.Contains(c.Provenance.Limits, "No Crossplane chain") {
				t.Fatalf("PRE-03 mapping/provenance invalid: %+v", c)
			}
			for name, want := range pre03FixtureHashes {
				if c.Provenance.Files[name] != want {
					t.Errorf("manifest hash %s differs", name)
				}
			}
		}
	}
	if !found || counts["planned"] != 2 || counts["existing_refreshed_fixture"] != 5 || counts["recorded_snapshot_binding_prepared_not_run"] != 2 || counts["recorded_projection_prepared_not_run"] != 8 || counts["raw_recording_prepared_not_run"] != 6 || counts["synthetic_source_replay_prepared_not_run"] != 1 {
		t.Fatalf("readiness counts changed unexpectedly: %v", counts)
	}
	checkPRE03Scaffold(t, filepath.Clean(pre03CaseRoot))
}

func checkPRE03Scaffold(t *testing.T, root string) {
	t.Helper()
	// Require stable file inventory without depending on map order.
	want := map[string]string{"capture-scope.json": pre03FixtureHashes["capture-scope.json"], "argocd-core-root.json": pre03FixtureHashes["argocd-core-root.json"], "argocd-core-child.json": pre03FixtureHashes["argocd-core-child.json"], "argocd-core-root-tree.txt": pre03FixtureHashes["argocd-core-root-tree.txt"], "argocd-core-child-tree.txt": pre03FixtureHashes["argocd-core-child-tree.txt"], "pod-describe.txt": pre03FixtureHashes["pod-describe.txt"], "events.txt": pre03FixtureHashes["events.txt"]}
	outputs := make([]map[string][]byte, 2)
	for i := 0; i < 2; i++ {
		workspace := t.TempDir()
		script, err := filepath.Abs(filepath.Join(root, "scaffold.sh"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", script)
		cmd.Dir = workspace
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("PRE-03 scaffold: %v: %s", err, out)
		}
		entries, err := os.ReadDir(filepath.Join(workspace, "cluster"))
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != len(want) {
			t.Fatalf("scaffold wrote %d files, want %d", len(entries), len(want))
		}
		outputs[i] = map[string][]byte{}
		for _, entry := range entries {
			if entry.IsDir() {
				t.Fatalf("unexpected directory %s", entry.Name())
			}
			data, err := os.ReadFile(filepath.Join(workspace, "cluster", entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if got, ok := want[entry.Name()]; !ok || hex.EncodeToString(sum[:]) != got {
				t.Fatalf("unexpected scaffold output %s", entry.Name())
			}
			outputs[i][entry.Name()] = data
		}
	}
	for name, first := range outputs[0] {
		if !bytes.Equal(first, outputs[1][name]) {
			t.Fatalf("PRE-03 arm scaffold differs for %s", name)
		}
	}
}
