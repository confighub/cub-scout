// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/spf13/cobra"
)

const recordedExplainDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
  labels:
    app.kubernetes.io/managed-by: Helm
    platform.example/managed-by: controller
  managedFields:
    - manager: kubectl-set
      operation: Update
      apiVersion: apps/v1
      fieldsType: FieldsV1
      fieldsV1:
        f:spec:
          f:replicas: {}
spec:
  replicas: 3
status:
  observedGeneration: 2
  replicas: 2
  updatedReplicas: 2
  readyReplicas: 1
`

func recordedExplainTestSnapshot(t *testing.T, source string) recordedObjectSnapshot {
	t.Helper()
	snapshot, err := loadRecordedObjectSnapshot(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestRecordedExplainUsesSamePureObjectFactsWithoutLiveEnvelope(t *testing.T) {
	identity := recordedObjectIdentity{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	snapshot := recordedExplainTestSnapshot(t, recordedExplainDeployment)
	got, err := recordedExplainSummary(snapshot, identity, ".spec.replicas")
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := snapshot.selectObject(identity)
	if err != nil {
		t.Fatal(err)
	}
	bounded := buildBoundedExplainSummary(recorded.Object, agent.BoundedReadEvidence{Resource: agent.BoundedResourceRef{
		APIVersion: identity.APIVersion, Kind: identity.Kind, Namespace: identity.Namespace, Name: identity.Name,
	}}, nil, ".spec.replicas")
	if got.Owner != bounded.Owner || got.Health != bounded.Health || got.MutationCause != bounded.MutationCause ||
		got.MutationManager != bounded.MutationManager || got.FieldAttribution == nil || bounded.FieldAttribution == nil ||
		got.FieldAttribution.Cause != bounded.FieldAttribution.Cause || got.FieldAttribution.Managers[0] != bounded.FieldAttribution.Managers[0] {
		t.Fatalf("recorded facts diverged from the shared object calculations: recorded=%#v bounded=%#v", got, bounded)
	}
	if got.RecordedInput == nil || got.ResourceRead != nil || got.CurrentChange != nil || got.ControllerRevision != nil {
		t.Fatalf("recorded provenance/time fields are incorrect: %#v", got)
	}
	if got.Owner != "Helm" || got.HealthMeasurement == nil || got.HealthMeasurement.Status != HealthMeasurementMeasured {
		t.Fatalf("expected explicit built-in owner and object readiness: %#v", got)
	}
	wire1, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wire2, err := json.Marshal(buildRecordedExplainSummary(recorded, identity, ".spec.replicas"))
	if err != nil {
		t.Fatal(err)
	}
	if string(wire1) != string(wire2) {
		t.Fatalf("recorded output is nondeterministic:\n%s\n%s", wire1, wire2)
	}
}

func TestRecordedExplainMalformedReadinessNumberRemainsUnknown(t *testing.T) {
	input := strings.Replace(recordedExplainDeployment, "readyReplicas: 1", "readyReplicas: true", 1)
	snapshot := recordedExplainTestSnapshot(t, input)
	identity := recordedObjectIdentity{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	summary, err := recordedExplainSummary(snapshot, identity, "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.HealthMeasurement == nil || summary.HealthMeasurement.Status != HealthMeasurementUnmeasured || summary.Health == "Healthy" {
		t.Fatalf("malformed readiness scalar became a health result: %#v", summary)
	}
}

func TestRecordedExplainIgnoresCustomDetectorFileChangesAfterStartup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "detectors.yaml")
	t.Setenv("CUB_SCOUT_OWNERSHIP_DETECTORS", path)
	writeConfig := func(owner string) {
		t.Helper()
		data := "detectors:\n  - name: ambient\n    labels:\n      - key: platform.example/managed-by\n        value: controller\n    owner_name: " + owner + "\n    owner_type: custom\n"
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig("First")
	source := strings.Replace(recordedExplainDeployment, "    app.kubernetes.io/managed-by: Helm\n", "", 1)
	snapshot := recordedExplainTestSnapshot(t, source)
	identity := recordedObjectIdentity{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	first, err := recordedExplainSummary(snapshot, identity, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Owner != "Unknown" {
		t.Fatalf("recorded owner must stay unknown when only a custom detector matches: %s", first.Owner)
	}
	selected, err := snapshot.selectObject(identity)
	if err != nil {
		t.Fatal(err)
	}
	if got := agent.DetectOwnership(selected.Object); got.Type != agent.OwnerCustom || got.Name != "First" {
		t.Fatalf("test detector configuration was not active: %#v", got)
	}
	writeConfig("Second")
	second, err := recordedExplainSummary(snapshot, identity, "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Owner != first.Owner || second.RecordedInput.SHA256 != first.RecordedInput.SHA256 {
		t.Fatalf("ambient config mutation changed recorded result: first=%#v second=%#v", first, second)
	}
}

func TestRecordedExplainCLIRequiresExplicitNamespaceAndKeepsProvenanceSeparate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.yaml")
	if err := os.WriteFile(path, []byte(recordedExplainDeployment), 0o600); err != nil {
		t.Fatal(err)
	}
	makeCommand := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().StringVar(&explainNamespace, "namespace", "", "")
		cmd.Flags().StringVar(&explainFormat, "format", "json", "")
		cmd.Flags().StringVar(&explainFieldPath, "field-path", "", "")
		cmd.Flags().StringVar(&explainPresentation, "presentation", "", "")
		cmd.Flags().StringVar(&explainHintMode, "hint-mode", "", "")
		cmd.Flags().BoolVar(&explainBounded, "bounded", false, "")
		cmd.Flags().StringVar(&explainAPIVersion, "api-version", "", "")
		cmd.Flags().StringVar(&explainContext, "kube-context", "", "")
		cmd.Flags().BoolVar(&explainRefresh, "refresh", false, "")
		cmd.Flags().StringVar(&explainExpectedRevision, "expected-revision", "", "")
		cmd.Flags().BoolVar(&explainWithConfigHub, "with-confighub", false, "")
		cmd.Flags().StringVar(&explainConfigHubSpace, "confighub-space", "", "")
		cmd.Flags().StringVar(&explainConfigHubSince, "confighub-since", "24h", "")
		cmd.Flags().StringVar(&explainConfigHubStaleAfter, "confighub-stale-after", "15m", "")
		cmd.Flags().StringVar(&explainRecording, "recording", "", "")
		cmd.Flags().BoolVar(&explainTUI, "tui", false, "")
		return cmd
	}
	reset := func() {
		explainNamespace, explainFormat, explainFieldPath = "", "json", ""
		explainPresentation, explainHintMode, explainRecording = "", "", ""
		explainBounded, explainRefresh, explainWithConfigHub, explainTUI = false, false, false, false
		explainAPIVersion, explainContext, explainExpectedRevision = "", "", ""
		explainConfigHubSpace, explainConfigHubSince, explainConfigHubStaleAfter = "", "24h", "15m"
	}
	defer reset()

	reset()
	cmd := makeCommand()
	if err := cmd.Flags().Set("recording", path); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("api-version", "apps/v1"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("namespace", "prod"); err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	callErr := runRecordedExplainCLI(cmd, []string{"Deployment/api"}, "json")
	_ = w.Close()
	os.Stdout = oldStdout
	output, readErr := io.ReadAll(r)
	_ = r.Close()
	if callErr != nil || readErr != nil {
		t.Fatalf("recorded CLI failed: call=%v read=%v", callErr, readErr)
	}
	var summary map[string]interface{}
	if err := json.Unmarshal(output, &summary); err != nil {
		t.Fatalf("invalid recorded CLI JSON %q: %v", output, err)
	}
	if _, ok := summary["recordedInput"]; !ok || summary["resourceRead"] != nil {
		t.Fatalf("recorded CLI mixed provenance envelopes: %#v", summary)
	}

	reset()
	cmd = makeCommand()
	if err := cmd.Flags().Set("recording", path); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("api-version", "apps/v1"); err != nil {
		t.Fatal(err)
	}
	explainNamespace = "prod" // A value without a changed raw flag is not explicit selection.
	if err := runRecordedExplainCLI(cmd, []string{"Deployment/api"}, "json"); err == nil || !strings.Contains(err.Error(), "explicit --namespace") {
		t.Fatalf("recorded CLI accepted an implicit namespace: %v", err)
	}
	if err := cmd.Flags().Set("namespace", "prod"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("bounded", "false"); err != nil {
		t.Fatal(err)
	}
	if err := runRecordedExplainCLI(cmd, []string{"Deployment/api"}, "json"); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("recorded CLI accepted explicit live bounded flag: %v", err)
	}
}

func TestRecordedMCPIsolatedImmutableAndNeverUsesLiveRunner(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"prod"}}`))
	}))
	defer server.Close()
	configPath := boundedTestConfig(t, server.URL)
	_ = configPath // Reachable kubeconfig is intentionally present for the zero-request assertion.

	path := filepath.Join(t.TempDir(), "recording.yaml")
	if err := os.WriteFile(path, []byte(recordedExplainDeployment), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := readRecordedObjectSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	gateway := newRecordedMCPGateway(snapshot)
	if len(gateway.tools) != 1 || len(gateway.toolList) != 1 || gateway.toolList[0].Name != "explain" || gateway.runTool != nil || gateway.connectedRunner != nil {
		t.Fatalf("recorded gateway exposes live dispatch: %#v", gateway)
	}
	properties := gateway.toolList[0].InputSchema["properties"].(map[string]interface{})
	if _, ok := properties["recording"]; ok {
		t.Fatalf("MCP call schema exposes the server recording path: %#v", properties)
	}
	var fallbackCalls int
	gateway.runTool = func(context.Context, []string) (string, error) { fallbackCalls++; return "", nil }
	unknown := gateway.callTool(context.Background(), json.RawMessage(`{"name":"doctor","arguments":{}}`))
	if unknown["isError"] != true || fallbackCalls != 0 {
		t.Fatalf("non-recorded tool was dispatched: %#v calls=%d", unknown, fallbackCalls)
	}
	for _, badField := range []string{"context", "bounded", "expected_revision", "with_confighub", "recording", "path"} {
		args := `{"api_version":"apps/v1","kind":"Deployment","namespace":"prod","name":"api","` + badField + `":"blocked"}`
		wrongArgs := gateway.callTool(context.Background(), json.RawMessage(`{"name":"explain","arguments":`+args+`}`))
		if wrongArgs["isError"] != true || fallbackCalls != 0 {
			t.Fatalf("recorded tool accepted %s option: %#v calls=%d", badField, wrongArgs, fallbackCalls)
		}
	}
	call := func() map[string]interface{} {
		return gateway.callTool(context.Background(), json.RawMessage(`{"name":"explain","arguments":{"api_version":"apps/v1","kind":"Deployment","namespace":"prod","name":"api"}}`))
	}
	first := call()
	changed := strings.Replace(recordedExplainDeployment, "readyReplicas: 1", "readyReplicas: 3", 1)
	if err := os.WriteFile(path, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	second := call()
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) || strings.Contains(string(firstJSON), "readyReplicas\":3") ||
		fallbackCalls != 0 || requests.Load() != 0 {
		t.Fatalf("MCP result changed or reached live runner: first=%s second=%s fallback=%d requests=%d", firstJSON, secondJSON, fallbackCalls, requests.Load())
	}
}

func TestRecordedExplainViewerShowsRecordedEvidenceWithoutInventory(t *testing.T) {
	identity := recordedObjectIdentity{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	summary, err := recordedExplainSummary(recordedExplainTestSnapshot(t, recordedExplainDeployment), identity, "")
	if err != nil {
		t.Fatal(err)
	}
	viewer := newRecordedExplainViewer(summary)
	view := viewer.content
	for _, expected := range []string{"RECORDED OBJECT", "no live cluster", "Deployment/api", "Helm", "apps/v1", "sha256=", "trusted-capture-time"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("recorded viewer lacks %q:\n%s", expected, view)
		}
	}
}

func TestRecordedExplainSurfacesShareFactsFromFullRecordedBaseline(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "evals", "recorded-explain-contract", "fixtures", "deployments.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadRecordedObjectSnapshot(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "deployments.yaml")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	fieldPath := `.spec.template.spec.containers[name="checkout"].image`
	identity := recordedObjectIdentity{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "shop", Name: "checkout"}
	want, err := recordedExplainSummary(snapshot, identity, fieldPath)
	if err != nil {
		t.Fatal(err)
	}
	if want.Owner != "Flux" || want.MutationCause != agent.CauseManualEdit || want.MutationManager != "kubectl-set" || want.FieldAttribution == nil || len(want.FieldAttribution.Managers) != 1 || want.FieldAttribution.Managers[0] != "kubectl-set" {
		t.Fatalf("full baseline facts changed: %#v", want)
	}

	resetExplain := func() {
		explainNamespace, explainFormat, explainFieldPath = "", "json", ""
		explainPresentation, explainHintMode, explainRecording = "", "", ""
		explainBounded, explainRefresh, explainWithConfigHub, explainTUI = false, false, false, false
		explainAPIVersion, explainContext, explainExpectedRevision = "", "", ""
		explainConfigHubSpace, explainConfigHubSince, explainConfigHubStaleAfter = "", "24h", "15m"
	}
	defer resetExplain()
	resetExplain()
	cmd := &cobra.Command{}
	cmd.Flags().StringVar(&explainNamespace, "namespace", "", "")
	cmd.Flags().StringVar(&explainFormat, "format", "json", "")
	cmd.Flags().StringVar(&explainFieldPath, "field-path", "", "")
	cmd.Flags().StringVar(&explainPresentation, "presentation", "", "")
	cmd.Flags().StringVar(&explainHintMode, "hint-mode", "", "")
	cmd.Flags().BoolVar(&explainBounded, "bounded", false, "")
	cmd.Flags().StringVar(&explainAPIVersion, "api-version", "", "")
	cmd.Flags().StringVar(&explainContext, "kube-context", "", "")
	cmd.Flags().BoolVar(&explainRefresh, "refresh", false, "")
	cmd.Flags().StringVar(&explainExpectedRevision, "expected-revision", "", "")
	cmd.Flags().BoolVar(&explainWithConfigHub, "with-confighub", false, "")
	cmd.Flags().StringVar(&explainConfigHubSpace, "confighub-space", "", "")
	cmd.Flags().StringVar(&explainConfigHubSince, "confighub-since", "24h", "")
	cmd.Flags().StringVar(&explainConfigHubStaleAfter, "confighub-stale-after", "15m", "")
	cmd.Flags().StringVar(&explainRecording, "recording", "", "")
	cmd.Flags().BoolVar(&explainTUI, "tui", false, "")
	for name, value := range map[string]string{"recording": path, "api-version": identity.APIVersion, "namespace": identity.Namespace, "field-path": fieldPath} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	callErr := runRecordedExplainCLI(cmd, []string{"Deployment/checkout"}, "json")
	_ = w.Close()
	os.Stdout = oldStdout
	cliOutput, readErr := io.ReadAll(r)
	_ = r.Close()
	if callErr != nil || readErr != nil {
		t.Fatalf("recorded CLI failed: call=%v read=%v", callErr, readErr)
	}
	var cliSummary ExplainSummary
	if err := json.Unmarshal(cliOutput, &cliSummary); err != nil {
		t.Fatalf("decode recorded CLI summary: %v", err)
	}

	gateway := newRecordedMCPGateway(snapshot)
	response := gateway.callTool(context.Background(), json.RawMessage(`{"name":"explain","arguments":{"api_version":"apps/v1","kind":"Deployment","namespace":"shop","name":"checkout","field_path":".spec.template.spec.containers[name=\"checkout\"].image"}}`))
	if response["isError"] != false {
		t.Fatalf("recorded MCP explain failed: %#v", response)
	}
	content := response["content"].([]map[string]string)
	var mcpSummary ExplainSummary
	if err := json.Unmarshal([]byte(content[0]["text"]), &mcpSummary); err != nil {
		t.Fatalf("decode recorded MCP summary: %v", err)
	}
	if !reflect.DeepEqual(cliSummary, mcpSummary) || cliSummary.RecordedInput == nil || cliSummary.RecordedInput.SHA256 != "305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8" {
		t.Fatalf("CLI/MCP summaries diverged or lost provenance:\nCLI=%#v\nMCP=%#v", cliSummary, mcpSummary)
	}
	for _, rendered := range []string{
		renderExplainText(cliSummary, PresentationMode(""), false, HintContext{}),
		renderExplainMarkdown(cliSummary, PresentationMode(""), false, HintContext{}),
		newRecordedExplainViewer(cliSummary).content,
	} {
		for _, required := range []string{"apps/v1", "shop", "checkout", cliSummary.RecordedInput.SHA256, "67696 bytes", "Field manager evidence", "kubectl-set", "custom-ownership-detectors", "trusted-capture-time"} {
			if !strings.Contains(rendered, required) {
				t.Errorf("recorded rendering lacks %q:\n%s", required, rendered)
			}
		}
		if strings.Contains(rendered, "Try next") {
			t.Errorf("recorded rendering suggests live follow-up commands:\n%s", rendered)
		}
	}
}

func TestRecordedExplainNeverRendersSecretPayload(t *testing.T) {
	const secret = `apiVersion: v1
kind: Secret
metadata:
  name: credentials
  namespace: prod
data:
  token: c2Vuc2l0aXZlLXZhbHVl
`
	identity := recordedObjectIdentity{APIVersion: "v1", Kind: "Secret", Namespace: "prod", Name: "credentials"}
	summary, err := recordedExplainSummary(recordedExplainTestSnapshot(t, secret), identity, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{string(mustJSON(t, summary)), renderExplainText(summary, PresentationMode(""), false, HintContext{}), renderExplainMarkdown(summary, PresentationMode(""), false, HintContext{}), newRecordedExplainViewer(summary).content} {
		if strings.Contains(rendered, "c2Vuc2l0aXZlLXZhbHVl") || strings.Contains(rendered, "sensitive-value") {
			t.Fatalf("recorded Secret payload leaked: %s", rendered)
		}
	}
}

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
