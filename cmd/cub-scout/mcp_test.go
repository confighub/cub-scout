package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

func TestNewMCPGateway_ToolsIncludeStandaloneSet(t *testing.T) {
	gateway := newMCPGateway(nil)
	tools := gateway.toolsForList()

	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name)
	}

	want := []string{"doctor", "explain", "gitops_status", "map", "release_check", "scan", "trace"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
}

func TestMCPDoctorAndScanForwardTypedKubernetesContext(t *testing.T) {
	gateway := newMCPGateway(nil)
	for _, name := range []string{"doctor", "scan"} {
		tool := gateway.tools[name]
		args, err := tool.BuildArgs(map[string]interface{}{"context": "cluster-b"})
		if err != nil {
			t.Fatalf("%s BuildArgs: %v", name, err)
		}
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--kube-context cluster-b") {
			t.Fatalf("%s args = %v", name, args)
		}
		if _, err := tool.BuildArgs(map[string]interface{}{"context": 42}); err == nil {
			t.Fatalf("%s accepted non-string context", name)
		}
		empty, err := tool.BuildArgs(map[string]interface{}{"context": ""})
		if err != nil || !strings.Contains(strings.Join(empty, " "), "--kube-context") {
			t.Fatalf("%s did not forward explicit empty context for strict CLI rejection: %v %v", name, empty, err)
		}
	}
}

func TestNewMCPGatewayWithMode_ConnectedAddsConfigHubTools(t *testing.T) {
	gateway := newMCPGatewayWithMode(nil, nil, true)
	tools := gateway.toolsForList()

	var names []string
	for _, tool := range tools {
		names = append(names, tool.Name)
	}

	want := []string{
		"compare_source_truth",
		"compare_three_way",
		"confighub_attestations",
		"confighub_changeorder_get",
		"confighub_changesets",
		"confighub_k8s_resources",
		"confighub_k8s_types",
		"confighub_live_status",
		"confighub_releases",
		"confighub_resources",
		"confighub_unit_events",
		"confighub_unit_get",
		"confighub_units",
		"doctor",
		"explain",
		"gitops_status",
		"map",
		"release_check",
		"scan",
		"trace",
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
}

func TestMCPGatewayHandleRequest_ToolsList(t *testing.T) {
	gateway := newMCPGateway(nil)
	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	var result struct {
		Tools []struct {
			Name        string `json:"name"`
			Annotations struct {
				ReadOnlyHint bool `json:"readOnlyHint"`
			} `json:"annotations"`
		} `json:"tools"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.Tools) != 7 {
		t.Fatalf("tool count = %d, want 7", len(result.Tools))
	}
	for _, tool := range result.Tools {
		if !tool.Annotations.ReadOnlyHint {
			t.Fatalf("tool %q readOnlyHint = false, want true", tool.Name)
		}
	}
}

func TestNewMCPGateway_ToolDescriptionsExpressChainBoundaries(t *testing.T) {
	gateway := newMCPGatewayWithMode(nil, nil, true)
	tools := gateway.tools

	cases := []struct {
		name     string
		contains []string
	}{
		{name: "compare_three_way", contains: []string{"governed state agrees with live state", "Load after doctor, explain, or trace", "use doctor first"}},
		{name: "compare_source_truth", contains: []string{"EVIDENCE", "single workload", "REQUIRED input", "never inferred", "DO NOT use this tool to approve, repair, or accept", "Load after doctor, explain, or compare_three_way", "use doctor first"}},
		{name: "doctor", contains: []string{"FIRST standalone tool", "whether cub-scout is the right first read-only step", "stale kubeconfig", "optional bounded ConfigHub delivery evidence", "Use before explain, trace, or scan"}},
		{name: "map", contains: []string{"what's running in this cluster", "raw `kubectl get` output", "optional context selects one exact kubeconfig context", "absent context keeps the current default", "ownership_evidence=true", "versioned compact diagnostics envelope", "does not prove orphaning", "use doctor first"}},
		{name: "scan", contains: []string{"Use AFTER doctor", "awareness scan of live state", "DO NOT use this as a governed promotion or revision-safety gate"}},
		{name: "explain", contains: []string{"resource-level mutation evidence", "pass field_path", "that path's observed manager names", "shared ambiguous evidence remains unknown", "does not order writes by time or identify a person", "Trace provides owner/source lineage only", "raw `kubectl describe`", "DO NOT load for broad cluster inventory or health"}},
		{name: "trace", contains: []string{"owner, deployer, or GitOps/source chain", "not which field writer made a change", "do not call trace just to confirm an explain result about manual-edit attribution", "DO NOT load for broad cluster status"}},
		{name: "gitops_status", contains: []string{"GitOps/controller delivery status", "controllerCoverage[]", "absence vs RBAC/API omission", "DO NOT use to force sync"}},
		{name: "confighub_changeorder_get", contains: []string{"Connected-only", "exact-space", "reported Stage/State", "Evaluation remains unknown", "DO NOT use as a prerequisite evaluator"}},
		{name: "confighub_changesets", contains: []string{"Connected-only", "what governed write changed a known unit or space", "approval trail", "Load after trace or confighub_units"}},
		{name: "confighub_k8s_resources", contains: []string{"Connected-only", "Resource-backed Kubernetes configuration reader", "stored ConfigHub Resource data, not live cluster state", "either space or target is REQUIRED", "low-API-load"}},
		{name: "confighub_k8s_types", contains: []string{"Connected-only", "Resource-backed Kubernetes type survey", "cheapest ConfigHub-side survey", "either space or target is REQUIRED", "stored ConfigHub Resource metadata"}},
		{name: "confighub_live_status", contains: []string{"Connected-only", "live-status writeback", "Space is REQUIRED", "best-effort reported evidence"}},
		{name: "confighub_releases", contains: []string{"Connected-only", "Release history", "Space is REQUIRED", "pair with gitops status"}},
		{name: "confighub_resources", contains: []string{"Connected-only", "Resource entity query", "fleet-wide Data predicates", "Space is REQUIRED", "extracted Resource rows, not live cluster state"}},
		{name: "confighub_unit_events", contains: []string{"Connected-only", "UnitEvent reader", "Space is REQUIRED", "DO NOT use as a cursor-consuming event consumer"}},
		{name: "confighub_units", contains: []string{"Connected-only", "cluster-to-ConfigHub lookup", "first useful ConfigHub object", "Load after doctor, map, explain, or trace", "then confighub_unit_get once the unit is known"}},
		{name: "confighub_unit_get", contains: []string{"Load ONLY after", "last applied revision, or live revision", "before opening the GUI", "use confighub_units first", "compare_three_way first"}},
	}

	for _, tc := range cases {
		tool, ok := tools[tc.name]
		if !ok {
			t.Fatalf("tool %q not found", tc.name)
		}
		desc := tool.Descriptor.Description
		for _, want := range tc.contains {
			if !strings.Contains(desc, want) {
				t.Errorf("%s description missing %q\nfull description: %s", tc.name, want, desc)
			}
		}
	}

	explainTool := gateway.tools["explain"]
	mapProperties := gateway.tools["map"].Descriptor.InputSchema["properties"].(map[string]interface{})
	ownershipEvidenceSchema, ok := mapProperties["ownership_evidence"].(map[string]interface{})
	if !ok || ownershipEvidenceSchema["type"] != "boolean" {
		t.Fatalf("map ownership_evidence schema missing or not boolean: %#v", mapProperties["ownership_evidence"])
	}
	for _, prop := range []string{"field_path"} {
		if _, ok := explainTool.Descriptor.InputSchema["properties"].(map[string]interface{})[prop]; !ok {
			t.Fatalf("explain schema missing %q", prop)
		}
	}
	args, err := explainTool.BuildArgs(map[string]interface{}{"resource": "Deployment/checkout", "field_path": `.spec.template.spec.containers[name="checkout"].image`})
	if err != nil {
		t.Fatalf("BuildArgs exact field path: %v", err)
	}
	fieldArg := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--field-path" && args[i+1] == `.spec.template.spec.containers[name="checkout"].image` {
			fieldArg = true
		}
	}
	if !fieldArg {
		t.Fatalf("BuildArgs = %v, missing exact field path", args)
	}
	if _, err := explainTool.BuildArgs(map[string]interface{}{"resource": "Deployment/checkout", "field_path": `.spec.containers[*].image`}); err == nil {
		t.Fatal("BuildArgs accepted wildcard field path")
	}
}

func TestNewMCPGateway_ToolDescriptionsCoverRepresentativeIntentEdges(t *testing.T) {
	gateway := newMCPGatewayWithMode(nil, nil, true)
	tools := gateway.tools

	cases := []struct {
		tool     string
		intent   string
		contains []string
	}{
		{
			tool:     "doctor",
			intent:   "kubectl cannot reach the cluster after restart.",
			contains: []string{"stale kubeconfig", "API unreachable"},
		},
		{
			tool:     "explain",
			intent:   "Which manager is recorded for this known Deployment's exact image field?",
			contains: []string{"use this directly as a first read", "resource-level mutation evidence", "known exact field", "pass field_path", "fieldAttribution block is scoped to that path", "normal resource summary remains", "Do not load doctor, map, trace, or compare first just to rediscover manager evidence", "evidence remains unknown", "without resource-level fallback", "does not order writes by time or identify a person"},
		},
		{
			tool:     "trace",
			intent:   "Which controller or GitOps source owns this known Deployment?",
			contains: []string{"owner, deployer, or GitOps/source chain", "not which field writer", "resource is known"},
		},
		{
			tool:     "scan",
			intent:   "Is this revision safe to promote?",
			contains: []string{"awareness scan of live state", "DO NOT use this as a governed promotion or revision-safety gate"},
		},
		{
			tool:     "compare_three_way",
			intent:   "Would you sign off on this change?",
			contains: []string{"sign-off-ready", "governed state agrees with live state"},
		},
		{
			tool:     "confighub_changesets",
			intent:   "What governed write changed this known unit?",
			contains: []string{"what governed write changed a known unit or space", "approval trail", "Load after trace or confighub_units"},
		},
		{
			tool:     "confighub_k8s_resources",
			intent:   "What Deployment config does ConfigHub intend for this target before I query every cluster?",
			contains: []string{"Kubernetes resources ConfigHub says should exist", "intended configuration", "low-API-load ConfigHub-side alternative"},
		},
		{
			tool:     "confighub_k8s_types",
			intent:   "Which custom resource types are stored in this space?",
			contains: []string{"which Kubernetes types ConfigHub holds", "discover custom resources", "cheapest ConfigHub-side survey"},
		},
		{
			tool:     "confighub_units",
			intent:   "Which ConfigHub unit corresponds to deployment/api?",
			contains: []string{"cluster-to-ConfigHub lookup", "corresponds to something already identified", "first useful ConfigHub object"},
		},
		{
			tool:     "confighub_unit_get",
			intent:   "Show me the last applied revision for unit payments-api.",
			contains: []string{"last applied revision, or live revision for unit X", "before opening the GUI", "compare_three_way first"},
		},
		{
			tool:     "confighub_live_status",
			intent:   "Did the evented feedback loop report synced and healthy yet?",
			contains: []string{"sync status", "application health", "freshness"},
		},
		{
			tool:     "gitops_status",
			intent:   "Is this deployed and which controller families did you check?",
			contains: []string{"delegated delivery is healthy", "controller families cub-scout actually inspected", "reported full digest", "not proof of fetch", "read-only evidence"},
		},
		{
			tool:     "confighub_releases",
			intent:   "Which OCI release was published for this space?",
			contains: []string{"Release history", "OCI bundle", "where filter"},
		},
		{
			tool:     "confighub_resources",
			intent:   "Find every indexed resource whose container image comes from this registry.",
			contains: []string{"indexed resources across units/spaces", "fleet-wide Data predicates", "lower-load alternative to iterating units"},
		},
		{
			tool:     "confighub_unit_events",
			intent:   "Which source event supports this delivery receipt?",
			contains: []string{"what event", "source action", "time window"},
		},
	}

	for _, tc := range cases {
		tool, ok := tools[tc.tool]
		if !ok {
			t.Fatalf("tool %q not found", tc.tool)
		}
		desc := tool.Descriptor.Description
		for _, want := range tc.contains {
			if !strings.Contains(desc, want) {
				t.Errorf("%s representative intent %q missing %q\nfull description: %s", tc.tool, tc.intent, want, desc)
			}
		}
	}
}

func TestNewMCPGateway_ToolsListMarksToolsReadOnly(t *testing.T) {
	gateway := newMCPGatewayWithMode(nil, nil, true)
	tools := gateway.toolsForList()

	if len(tools) == 0 {
		t.Fatal("expected tools in list")
	}

	for _, tool := range tools {
		if tool.Annotations == nil {
			t.Fatalf("tool %q missing annotations", tool.Name)
		}
		if !tool.Annotations.ReadOnlyHint {
			t.Fatalf("tool %q readOnlyHint = false, want true", tool.Name)
		}
	}
}

func TestMCPGatewayHandleRequest_ToolsCallTrace(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"ok":true}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`7`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"trace",
			"arguments":{"resource":"deployment/api","namespace":"payments"}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"trace", "deployment/api", "-n", "payments", "--format", "json"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.IsError {
		t.Fatal("result.isError = true, want false")
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("content = %+v, want single text item", result.Content)
	}
	if result.Content[0].Text != `{"ok":true}` {
		t.Fatalf("content text = %q, want %q", result.Content[0].Text, `{"ok":true}`)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallGitOpsStatus(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"backend":"flux","controllerCoverage":[{"family":"Flux","status":"found","found":1}],"deliveryEvidence":{"configHub":{"liveStatuses":[{"revisionCorrelation":{"schema":"confighub.reportedRevisionReleaseCorrelation.v1","state":"digest_match","coverage":"complete","limitation":"correlation only"}}]}}}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`8`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"gitops_status",
			"arguments":{
				"namespace":"prod",
				"with_confighub":true,
				"confighub_space":"prod",
				"confighub_since":"2h",
				"confighub_stale_after":"5m"
			}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{
		"gitops", "status", "--format", "json",
		"-n", "prod",
		"--with-confighub",
		"--confighub-space", "prod",
		"--confighub-since", "2h",
		"--confighub-stale-after", "5m",
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}

	var result struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Data map[string]interface{} `json:"data"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.IsError {
		t.Fatal("result.isError = true, want false")
	}
	if result.StructuredContent.Data["backend"] != "flux" {
		t.Fatalf("structured data = %+v, want backend flux", result.StructuredContent.Data)
	}
	if _, ok := result.StructuredContent.Data["controllerCoverage"]; !ok {
		t.Fatalf("structured data missing controllerCoverage: %+v", result.StructuredContent.Data)
	}
	delivery, ok := result.StructuredContent.Data["deliveryEvidence"].(map[string]interface{})
	if !ok {
		t.Fatalf("MCP structured data omitted delivery evidence: %+v", result.StructuredContent.Data)
	}
	configHub, ok := delivery["configHub"].(map[string]interface{})
	if !ok {
		t.Fatalf("MCP structured data omitted ConfigHub evidence: %+v", delivery)
	}
	statuses, ok := configHub["liveStatuses"].([]interface{})
	if !ok || len(statuses) != 1 {
		t.Fatalf("MCP live statuses = %+v", configHub["liveStatuses"])
	}
	status, _ := statuses[0].(map[string]interface{})
	corr, _ := status["revisionCorrelation"].(map[string]interface{})
	if corr["state"] != "digest_match" || corr["schema"] != configHubRevisionCorrelationSchema {
		t.Fatalf("MCP correlation = %+v", corr)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallValidationError(t *testing.T) {
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		t.Fatal("runner should not be called for validation error")
		return "", nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`9`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"trace",
			"arguments":{"namespace":"payments"}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "resource") {
		t.Fatalf("unexpected error text: %+v", result.Content)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConnectedChangesets(t *testing.T) {
	var gotStandaloneArgs []string
	var gotConnectedArgs []string

	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			gotStandaloneArgs = append([]string(nil), args...)
			return `{"standalone":true}`, nil
		},
		func(ctx context.Context, args []string) (string, error) {
			gotConnectedArgs = append([]string(nil), args...)
			return `[{"Space":{"Slug":"platform"},"ChangeSet":{"Slug":"release-42","ID":"cs-123"}}]`, nil
		},
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`11`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"confighub_changesets",
			"arguments":{"space":"platform","where":"Slug LIKE 'release-%'"}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	if len(gotStandaloneArgs) != 0 {
		t.Fatalf("standalone runner should not be used, got args %v", gotStandaloneArgs)
	}
	wantConnected := []string{"changeset", "list", "-o", "json", "--space", "platform", "--where", "Slug LIKE 'release-%'"}
	if !reflect.DeepEqual(gotConnectedArgs, wantConnected) {
		t.Fatalf("connected args = %v, want %v", gotConnectedArgs, wantConnected)
	}

	var result struct {
		StructuredContent struct {
			Data      interface{}      `json:"data"`
			NextSteps []StructuredHint `json:"nextSteps"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.StructuredContent.NextSteps) == 0 {
		t.Fatal("expected nextSteps in structuredContent")
	}
	if got := result.StructuredContent.NextSteps[0].NextCommand; !strings.Contains(got, "cub changeset get") {
		t.Fatalf("next command = %q, want changeset get command", got)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallCompareThreeWay(t *testing.T) {
	var gotStandaloneArgs []string
	var gotConnectedArgs []string

	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			gotStandaloneArgs = append([]string(nil), args...)
			return `{"agreement":{"state":"agreed"}}`, nil
		},
		func(ctx context.Context, args []string) (string, error) {
			gotConnectedArgs = append([]string(nil), args...)
			return `{"connected":true}`, nil
		},
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`11.5`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"compare_three_way",
			"arguments":{"scope":"deploy/api","namespace":"payments"}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	if len(gotConnectedArgs) != 0 {
		t.Fatalf("connected runner should not be used, got args %v", gotConnectedArgs)
	}
	wantStandalone := []string{"compare", "three-way", "--format", "json", "--scope", "deploy/api", "-n", "payments"}
	if !reflect.DeepEqual(gotStandaloneArgs, wantStandalone) {
		t.Fatalf("standalone args = %v, want %v", gotStandaloneArgs, wantStandalone)
	}

	var result struct {
		StructuredContent struct {
			Data map[string]interface{} `json:"data"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if _, ok := result.StructuredContent.Data["agreement"]; !ok {
		t.Fatalf("expected structuredContent.data to include parsed compare JSON, got %+v", result.StructuredContent.Data)
	}
}

// TestMCPGatewayHandleRequest_ToolsCallCompareSourceTruth proves the
// new connected-mode tool registration. The args BuildArgs produces
// must match cub-scout's CLI surface byte-for-byte (so the MCP-driven
// invocation and a hand-typed CLI invocation produce identical
// output), and the structuredContent envelope must wrap the source-
// truth JSON under `data` so agents can read evidence directly without
// re-parsing the text content.
func TestMCPGatewayHandleRequest_ToolsCallCompareSourceTruth(t *testing.T) {
	var gotStandaloneArgs []string

	// Synthetic source-truth payload that mirrors the contract types in
	// pkg/agent/source_truth.go. Field names must match exactly, since
	// the structuredContent wrapper just hands the parsed JSON back.
	const syntheticPayload = `{
		"declared_strategy": "ConfigHub -> OCI -> Flux -> Kubernetes",
		"status": "PASS",
		"source_truth": "AGREED",
		"outlier": "unknown",
		"surfaces": {
			"confighub":  {"space":"demo","unit":"rag-server","revision":"47"},
			"controller": {"kind":"Flux","source":"oci://oci.confighub.com/demo/rag-server","revision_or_digest":"sha256:abc","health":"Ready"},
			"runtime":    {"resource":"Deployment/rag-server in demo","field":"spec.template.spec.containers[0].image","value":"oci.confighub.com/demo/rag-server@sha256:abc","health":"Current"}
		}
	}`

	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			gotStandaloneArgs = append([]string(nil), args...)
			return syntheticPayload, nil
		},
		nil,
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`12.1`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"compare_source_truth",
			"arguments":{
				"target":"Deployment/rag-server",
				"namespace":"demo",
				"strategy":"confighub-oci-flux",
				"context":"alpha-context"
			}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	wantStandalone := []string{
		"compare", "source-truth", "Deployment/rag-server",
		"-n", "demo",
		"--strategy", "confighub-oci-flux",
		"--kube-context", "alpha-context",
		"--format", "json",
	}
	if !reflect.DeepEqual(gotStandaloneArgs, wantStandalone) {
		t.Fatalf("standalone args = %v, want %v", gotStandaloneArgs, wantStandalone)
	}

	var result struct {
		StructuredContent struct {
			Data map[string]interface{} `json:"data"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	// Source-truth's contract fields must round-trip through the
	// structuredContent envelope under `data`. Spot-check the three
	// council-anchored fields plus the strategy-rendered string.
	for _, want := range []string{"declared_strategy", "status", "source_truth", "outlier", "surfaces"} {
		if _, ok := result.StructuredContent.Data[want]; !ok {
			gotKeys := make([]string, 0, len(result.StructuredContent.Data))
			for k := range result.StructuredContent.Data {
				gotKeys = append(gotKeys, k)
			}
			sort.Strings(gotKeys)
			t.Errorf("structuredContent.data missing field %q (got keys: %v)", want, gotKeys)
		}
	}
	if got := result.StructuredContent.Data["status"]; got != "PASS" {
		t.Errorf("status field = %v, want PASS", got)
	}
}

// TestMCPGatewayHandleRequest_ToolsCallCompareSourceTruthValidationError
// exercises the BuildArgs validation path — every required argument
// (target, namespace, strategy) must produce a clear error before the
// runner is ever called. Mirrors the compare_three_way validation
// test below.
func TestMCPGatewayHandleRequest_ToolsCallCompareSourceTruthValidationError(t *testing.T) {
	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			t.Fatal("runner should not be called for validation error")
			return "", nil
		},
		nil,
		true,
	)

	cases := []struct {
		name string
		args string
	}{
		{name: "missing all", args: `{}`},
		{name: "missing namespace+strategy", args: `{"target":"Deployment/x"}`},
		{name: "missing strategy", args: `{"target":"Deployment/x","namespace":"demo"}`},
		{name: "missing target", args: `{"namespace":"demo","strategy":"confighub-oci-flux"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcpRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`12.2`),
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name":"compare_source_truth","arguments":` + tc.args + `}`),
			}
			resp := gateway.handleRequest(context.Background(), req)
			if resp == nil {
				t.Fatal("response is nil")
			}
			// Validation errors come back through the result envelope
			// as isError=true, not as JSON-RPC errors — the gateway
			// uses mcpToolError for tool-level failures.
			var result struct {
				IsError bool `json:"isError"`
			}
			if err := marshalInto(resp.Result, &result); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected isError=true for %s, got %+v", tc.name, resp.Result)
			}
		})
	}
}

func TestMCPCompareSourceTruthStrategyEnumTracksAgentStrategies(t *testing.T) {
	gateway := newMCPGatewayWithMode(nil, nil, true)
	tool := gateway.tools["compare_source_truth"]
	properties, ok := tool.Descriptor.InputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("input schema properties missing: %+v", tool.Descriptor.InputSchema)
	}
	strategySchema, ok := properties["strategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("strategy schema missing: %+v", properties)
	}
	rawEnum, ok := strategySchema["enum"].([]string)
	if !ok {
		t.Fatalf("strategy enum has unexpected type: %#v", strategySchema["enum"])
	}
	want := make([]string, 0, len(agent.AllStrategies()))
	for _, strategy := range agent.AllStrategies() {
		want = append(want, string(strategy))
	}
	sort.Strings(want)
	if !reflect.DeepEqual(rawEnum, want) {
		t.Fatalf("strategy enum = %v, want %v", rawEnum, want)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConfigHubLiveStatus(t *testing.T) {
	var gotConnectedArgs []string
	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			t.Fatal("standalone runner should not be used")
			return "", nil
		},
		func(ctx context.Context, args []string) (string, error) {
			gotConnectedArgs = append([]string(nil), args...)
			return `[{"Slug":"prod","SpaceID":"sp-123","Annotations":{"confighub.com/live-status":"{\"source\":\"argobot\",\"app\":\"prod\",\"syncStatus\":\"Synced\",\"healthStatus\":\"Healthy\",\"operationPhase\":\"Succeeded\",\"observedAt\":\"2026-09-10T12:00:00Z\"}"}}]`, nil
		},
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`12.3`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"confighub_live_status","arguments":{"space":"prod"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	wantArgs := []string{"space", "list", "-o", "json", "--select", "Slug,SpaceID,Annotations,Labels", "--where", "Slug = 'prod'"}
	if !reflect.DeepEqual(gotConnectedArgs, wantArgs) {
		t.Fatalf("connected args = %v, want %v", gotConnectedArgs, wantArgs)
	}

	var result struct {
		StructuredContent struct {
			LiveStatuses []ConfigHubLiveStatusEvidence `json:"liveStatuses"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.StructuredContent.LiveStatuses) != 1 {
		t.Fatalf("liveStatuses = %+v, want one status", result.StructuredContent.LiveStatuses)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConfigHubReleases(t *testing.T) {
	var gotConnectedArgs []string
	gateway := newMCPGatewayWithMode(nil, func(ctx context.Context, args []string) (string, error) {
		gotConnectedArgs = append([]string(nil), args...)
		return `[{"Release":{"Slug":"rel-1"}}]`, nil
	}, true)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`12.4`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"confighub_releases","arguments":{"space":"prod","where":"CreatedAt > '2026-09-10T00:00:00Z'"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	wantArgs := []string{"release", "list", "--space", "prod", "-o", "json", "--where", "CreatedAt > '2026-09-10T00:00:00Z'"}
	if !reflect.DeepEqual(gotConnectedArgs, wantArgs) {
		t.Fatalf("connected args = %v, want %v", gotConnectedArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConfigHubUnitEvents(t *testing.T) {
	var gotConnectedArgs []string
	gateway := newMCPGatewayWithMode(nil, func(ctx context.Context, args []string) (string, error) {
		gotConnectedArgs = append([]string(nil), args...)
		return `[{"UnitEvent":{"Action":"ReleasePublished"}}]`, nil
	}, true)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`12.5`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"confighub_unit_events","arguments":{"space":"prod","unit":"payments-api","where":"CreatedAt > '2026-09-10T00:00:00Z'"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	wantArgs := []string{"unit-event", "list", "payments-api", "--space", "prod", "-o", "json", "--where", "CreatedAt > '2026-09-10T00:00:00Z'"}
	if !reflect.DeepEqual(gotConnectedArgs, wantArgs) {
		t.Fatalf("connected args = %v, want %v", gotConnectedArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConfigHubK8sResources(t *testing.T) {
	var gotStandaloneArgs []string
	var gotConnectedArgs []string
	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			gotStandaloneArgs = append([]string(nil), args...)
			return `{"standalone":true}`, nil
		},
		func(ctx context.Context, args []string) (string, error) {
			gotConnectedArgs = append([]string(nil), args...)
			return `[{"Space":"prod","Unit":"payments-api","Target":"prod/prod-oci","Namespace":"payments","Name":"api","Kind":"Deployment","APIVersion":"apps/v1","ResourceType":"apps/v1/Deployment"}]`, nil
		},
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`12.6`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"confighub_k8s_resources",
			"arguments":{
				"type":"deploy",
				"names":["api","web"],
				"space":"prod",
				"target":["prod/prod-oci","stage/stage-oci"],
				"namespace":"payments",
				"where":"Unit.Slug LIKE '%-backend'",
				"where_resource":"spec.replicas > 1",
				"show":"detail"
			}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	if len(gotStandaloneArgs) != 0 {
		t.Fatalf("standalone runner should not be used, got args %v", gotStandaloneArgs)
	}
	wantConnected := []string{
		"k8s", "get", "deploy", "api", "web",
		"--space", "prod",
		"--target", "prod/prod-oci",
		"--target", "stage/stage-oci",
		"-n", "payments",
		"--where", "Unit.Slug LIKE '%-backend'",
		"--where-resource", "spec.replicas > 1",
		"--show", "detail",
		"-o", "json",
	}
	if !reflect.DeepEqual(gotConnectedArgs, wantConnected) {
		t.Fatalf("connected args = %v, want %v", gotConnectedArgs, wantConnected)
	}

	var result struct {
		StructuredContent struct {
			Data []struct {
				Kind string `json:"Kind"`
			} `json:"data"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.StructuredContent.Data) != 1 || result.StructuredContent.Data[0].Kind != "Deployment" {
		t.Fatalf("structuredContent.data = %+v, want parsed k8s resources", result.StructuredContent.Data)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConfigHubK8sTypes(t *testing.T) {
	var gotConnectedArgs []string
	gateway := newMCPGatewayWithMode(nil, func(ctx context.Context, args []string) (string, error) {
		gotConnectedArgs = append([]string(nil), args...)
		return `[{"ResourceType":"apps/v1/Deployment","APIVersion":"apps/v1","Kind":"Deployment","Resources":3,"Units":2,"Spaces":1}]`, nil
	}, true)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`12.7`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"confighub_k8s_types",
			"arguments":{
				"type":"all",
				"space":"*",
				"target":["prod/prod-oci"],
				"namespace":"payments",
				"where":"Space.Labels.Environment = 'prod'",
				"where_resource":"metadata.labels.app != ''"
			}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	wantConnected := []string{
		"k8s", "types", "all",
		"--space", "*",
		"--target", "prod/prod-oci",
		"-n", "payments",
		"--where", "Space.Labels.Environment = 'prod'",
		"--where-resource", "metadata.labels.app != ''",
		"-o", "json",
	}
	if !reflect.DeepEqual(gotConnectedArgs, wantConnected) {
		t.Fatalf("connected args = %v, want %v", gotConnectedArgs, wantConnected)
	}

	var result struct {
		StructuredContent struct {
			Data []struct {
				ResourceType string `json:"ResourceType"`
			} `json:"data"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.StructuredContent.Data) != 1 || result.StructuredContent.Data[0].ResourceType != "apps/v1/Deployment" {
		t.Fatalf("structuredContent.data = %+v, want parsed k8s type summaries", result.StructuredContent.Data)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConfigHubK8sValidationError(t *testing.T) {
	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			t.Fatal("standalone runner should not be called for validation error")
			return "", nil
		},
		func(ctx context.Context, args []string) (string, error) {
			t.Fatal("connected runner should not be called for validation error")
			return "", nil
		},
		true,
	)

	cases := []struct {
		name        string
		tool        string
		args        string
		errContains string
	}{
		{name: "resources missing type", tool: "confighub_k8s_resources", args: `{"space":"prod"}`, errContains: "type"},
		{name: "resources missing bounded scope", tool: "confighub_k8s_resources", args: `{"type":"deploy"}`, errContains: "space or target"},
		{name: "resources invalid show", tool: "confighub_k8s_resources", args: `{"type":"deploy","space":"prod","show":"wide"}`, errContains: "unknown show value"},
		{name: "resources invalid names", tool: "confighub_k8s_resources", args: `{"type":"deploy","space":"prod","names":["api",5]}`, errContains: "names must contain only strings"},
		{name: "types missing bounded scope", tool: "confighub_k8s_types", args: `{}`, errContains: "space or target"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcpRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`12.8`),
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name":"` + tc.tool + `","arguments":` + tc.args + `}`),
			}

			resp := gateway.handleRequest(context.Background(), req)
			if resp == nil {
				t.Fatal("response is nil")
			}
			if resp.Error != nil {
				t.Fatalf("unexpected rpc error: %+v", resp.Error)
			}

			var result struct {
				IsError bool `json:"isError"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			}
			if err := marshalInto(resp.Result, &result); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected isError=true for %s, got %+v", tc.name, resp.Result)
			}
			if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, tc.errContains) {
				t.Fatalf("error text = %+v, want substring %q", result.Content, tc.errContains)
			}
		})
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConfigHubResources(t *testing.T) {
	var gotStandaloneArgs []string
	var gotConnectedArgs []string
	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			gotStandaloneArgs = append([]string(nil), args...)
			return `{"standalone":true}`, nil
		},
		func(ctx context.Context, args []string) (string, error) {
			gotConnectedArgs = append([]string(nil), args...)
			return `[{"Resource":{"ResourceType":"apps/v1/Deployment","ResourceName":"payments/api","SpaceSlug":"prod","UnitSlug":"payments-api"}}]`, nil
		},
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`12.9`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name":"confighub_resources",
			"arguments":{
				"space":"*",
				"where":"ResourceType = 'apps/v1/Deployment' AND Data.spec.replicas > 1",
				"contains":"payments",
				"select":"ResourceType,ResourceName,Data",
				"filter":"prod-filters/k8s",
				"view":"deployment-images",
				"raw_data":true
			}
		}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	if len(gotStandaloneArgs) != 0 {
		t.Fatalf("standalone runner should not be used, got args %v", gotStandaloneArgs)
	}
	wantConnected := []string{
		"resource", "list",
		"--space", "*",
		"-o", "json",
		"--where", "ResourceType = 'apps/v1/Deployment' AND Data.spec.replicas > 1",
		"--contains", "payments",
		"--select", "ResourceType,ResourceName,Data",
		"--filter", "prod-filters/k8s",
		"--view", "deployment-images",
		"--raw-data",
	}
	if !reflect.DeepEqual(gotConnectedArgs, wantConnected) {
		t.Fatalf("connected args = %v, want %v", gotConnectedArgs, wantConnected)
	}

	var result struct {
		StructuredContent struct {
			Data []struct {
				Resource struct {
					ResourceType string `json:"ResourceType"`
				} `json:"Resource"`
			} `json:"data"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.StructuredContent.Data) != 1 || result.StructuredContent.Data[0].Resource.ResourceType != "apps/v1/Deployment" {
		t.Fatalf("structuredContent.data = %+v, want parsed resources", result.StructuredContent.Data)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConfigHubResourcesValidationError(t *testing.T) {
	gateway := newMCPGatewayWithMode(nil, func(ctx context.Context, args []string) (string, error) {
		t.Fatal("connected runner should not be called for validation error")
		return "", nil
	}, true)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`13.1`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"confighub_resources","arguments":{}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "space") {
		t.Fatalf("unexpected error text: %+v", result.Content)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallCompareThreeWayValidationError(t *testing.T) {
	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			t.Fatal("runner should not be called for validation error")
			return "", nil
		},
		nil,
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`11.6`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"compare_three_way","arguments":{}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "scope") {
		t.Fatalf("unexpected error text: %+v", result.Content)
	}
}

func TestMCPFrameRoundTrip(t *testing.T) {
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	var stream bytes.Buffer
	if err := writeMCPFrame(&stream, payload); err != nil {
		t.Fatalf("writeMCPFrame() error = %v", err)
	}

	got, err := readMCPFrame(bufio.NewReader(&stream))
	if err != nil {
		t.Fatalf("readMCPFrame() error = %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch\ngot=%s\nwant=%s", string(got), string(payload))
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctor(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"all"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`12`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"doctor", "--format", "json"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithNamespace(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"prod"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`13`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"namespace":"prod"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"doctor", "--format", "json", "-n", "prod"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithTop(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"all"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`14`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":5}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"doctor", "--format", "json", "--top", "5"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithAllParams(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"staging"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`15`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"namespace":"staging","top":10}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{"doctor", "--format", "json", "-n", "staging", "--top", "10"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithConfigHub(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"prod"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`150`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"namespace":"prod","with_confighub":true,"confighub_space":"payments","confighub_since":"7d","confighub_stale_after":"10m"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	wantArgs := []string{
		"doctor", "--format", "json",
		"-n", "prod",
		"--with-confighub",
		"--confighub-space", "payments",
		"--confighub-since", "7d",
		"--confighub-stale-after", "10m",
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithTopZero(t *testing.T) {
	var gotArgs []string
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		gotArgs = append([]string(nil), args...)
		return `{"cluster":"minikube","namespace":"all"}`, nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`16`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":0}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	// top=0 should be passed through as --top 0 (valid CLI case)
	wantArgs := []string{"doctor", "--format", "json", "--top", "0"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("tool args = %v, want %v", gotArgs, wantArgs)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithNegativeTop(t *testing.T) {
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		t.Fatal("runner should not be called for validation error")
		return "", nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`17`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":-1}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "non-negative") {
		t.Fatalf("error should mention non-negative, got: %+v", result.Content)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithStringTop(t *testing.T) {
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		t.Fatal("runner should not be called for validation error")
		return "", nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`18`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":"5"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "integer") {
		t.Fatalf("error should mention integer, got: %+v", result.Content)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallDoctorWithFractionalTop(t *testing.T) {
	gateway := newMCPGateway(func(ctx context.Context, args []string) (string, error) {
		t.Fatal("runner should not be called for validation error")
		return "", nil
	})

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`19`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"doctor","arguments":{"top":1.5}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if !result.IsError {
		t.Fatal("result.isError = false, want true")
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "whole number") {
		t.Fatalf("error should mention whole number, got: %+v", result.Content)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConnectedUnitsIncludesTrustSurface(t *testing.T) {
	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			return `{"standalone":true}`, nil
		},
		func(ctx context.Context, args []string) (string, error) {
			return `[{"Space":{"Slug":"prod","SpaceID":"sp-123"},"Unit":{"Slug":"payments-api","UnitID":"u-123","HeadRevisionNum":9,"LiveRevisionNum":7}}]`, nil
		},
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`20`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"confighub_units","arguments":{"space":"prod"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		StructuredContent struct {
			Data                  interface{}      `json:"data"`
			ConfigHubURL          string           `json:"confighubUrl"`
			ConfigHubRevisionsURL string           `json:"confighubRevisionsUrl"`
			NextSteps             []StructuredHint `json:"nextSteps"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.StructuredContent.ConfigHubURL != "https://confighub.com/units/sp-123/u-123" {
		t.Fatalf("confighubUrl = %q, want exact unit url", result.StructuredContent.ConfigHubURL)
	}
	if result.StructuredContent.ConfigHubRevisionsURL != "https://confighub.com/units/sp-123/u-123?tab=2" {
		t.Fatalf("confighubRevisionsUrl = %q, want exact revisions url", result.StructuredContent.ConfigHubRevisionsURL)
	}
	if len(result.StructuredContent.NextSteps) == 0 {
		t.Fatal("expected nextSteps in structuredContent")
	}
	if got := result.StructuredContent.NextSteps[0].NextSurface; got != "https://confighub.com/units/sp-123/u-123?tab=2" {
		t.Fatalf("first next surface = %q, want revisions url", got)
	}
	if got := result.StructuredContent.NextSteps[1].NextCommand; got != "cub unit get payments-api -o json --space prod" {
		t.Fatalf("second next command = %q, want exact unit get command", got)
	}
}

func TestMCPGatewayHandleRequest_ToolsCallConnectedUnitGetIncludesTrustSurface(t *testing.T) {
	gateway := newMCPGatewayWithMode(
		func(ctx context.Context, args []string) (string, error) {
			return `{"standalone":true}`, nil
		},
		func(ctx context.Context, args []string) (string, error) {
			return `{"Space":{"Slug":"prod","SpaceID":"sp-123"},"Unit":{"Slug":"payments-api","UnitID":"u-123","HeadRevisionNum":9,"LiveRevisionNum":7,"LastReleasedRevisionNum":8}}`, nil
		},
		true,
	)

	req := mcpRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`21`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"confighub_unit_get","arguments":{"unit":"payments-api","space":"prod"}}`),
	}

	resp := gateway.handleRequest(context.Background(), req)
	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Error != nil {
		t.Fatalf("unexpected rpc error: %+v", resp.Error)
	}

	var result struct {
		StructuredContent struct {
			Data                  interface{}      `json:"data"`
			ConfigHubURL          string           `json:"confighubUrl"`
			ConfigHubRevisionsURL string           `json:"confighubRevisionsUrl"`
			NextSteps             []StructuredHint `json:"nextSteps"`
		} `json:"structuredContent"`
	}
	if err := marshalInto(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.StructuredContent.ConfigHubURL != "https://confighub.com/units/sp-123/u-123" {
		t.Fatalf("confighubUrl = %q, want exact unit url", result.StructuredContent.ConfigHubURL)
	}
	if result.StructuredContent.ConfigHubRevisionsURL != "https://confighub.com/units/sp-123/u-123?tab=2" {
		t.Fatalf("confighubRevisionsUrl = %q, want exact revisions url", result.StructuredContent.ConfigHubRevisionsURL)
	}
	if len(result.StructuredContent.NextSteps) == 0 {
		t.Fatal("expected nextSteps in structuredContent")
	}
	if got := result.StructuredContent.NextSteps[0].NextSurface; got != "https://confighub.com/units/sp-123/u-123?tab=2" {
		t.Fatalf("first next surface = %q, want revisions url", got)
	}
	if !strings.Contains(result.StructuredContent.NextSteps[0].Reason, "Head revision 9 is ahead of live revision 7") {
		t.Fatalf("first reason = %q, want revision-drift rationale", result.StructuredContent.NextSteps[0].Reason)
	}
	if got := result.StructuredContent.NextSteps[1].NextSurface; got != "https://confighub.com/units/sp-123/u-123" {
		t.Fatalf("second next surface = %q, want unit url", got)
	}
}

func TestArgInt(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
		key  string
		want int
	}{
		{"float64", map[string]interface{}{"top": float64(5)}, "top", 5},
		{"int", map[string]interface{}{"top": 3}, "top", 3},
		{"missing", map[string]interface{}{}, "top", 0},
		{"nil", map[string]interface{}{"top": nil}, "top", 0},
		{"string", map[string]interface{}{"top": "5"}, "top", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := argInt(tc.args, tc.key)
			if got != tc.want {
				t.Errorf("argInt() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestArgStringSlice(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		key         string
		want        []string
		wantErr     bool
		errContains string
	}{
		{
			name: "string array from json",
			args: map[string]interface{}{"names": []interface{}{" api ", "web", ""}},
			key:  "names",
			want: []string{"api", "web"},
		},
		{
			name: "string",
			args: map[string]interface{}{"target": " prod/prod-oci "},
			key:  "target",
			want: []string{"prod/prod-oci"},
		},
		{
			name: "missing",
			args: map[string]interface{}{},
			key:  "target",
			want: nil,
		},
		{
			name:        "invalid array item",
			args:        map[string]interface{}{"names": []interface{}{"api", float64(1)}},
			key:         "names",
			wantErr:     true,
			errContains: "names must contain only strings",
		},
		{
			name:        "invalid type",
			args:        map[string]interface{}{"target": float64(1)},
			key:         "target",
			wantErr:     true,
			errContains: "target must be a string array",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := argStringSlice(tc.args, tc.key)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argStringSlice() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestArgIntOpt(t *testing.T) {
	tests := []struct {
		name          string
		args          map[string]interface{}
		key           string
		allowNegative bool
		wantVal       int
		wantPresent   bool
		wantErr       bool
		errContains   string
	}{
		{"float64", map[string]interface{}{"top": float64(5)}, "top", false, 5, true, false, ""},
		{"int", map[string]interface{}{"top": 3}, "top", false, 3, true, false, ""},
		{"zero", map[string]interface{}{"top": float64(0)}, "top", false, 0, true, false, ""},
		{"missing", map[string]interface{}{}, "top", false, 0, false, false, ""},
		{"nil", map[string]interface{}{"top": nil}, "top", false, 0, false, false, ""},
		{"string", map[string]interface{}{"top": "5"}, "top", false, 0, true, true, "integer"},
		{"fractional", map[string]interface{}{"top": float64(1.5)}, "top", false, 0, true, true, "whole number"},
		{"negative_rejected", map[string]interface{}{"top": float64(-1)}, "top", false, 0, true, true, "non-negative"},
		{"negative_allowed", map[string]interface{}{"top": float64(-1)}, "top", true, -1, true, false, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotVal, gotPresent, gotErr := argIntOpt(tc.args, tc.key, tc.allowNegative)
			if gotPresent != tc.wantPresent {
				t.Errorf("argIntOpt() present = %v, want %v", gotPresent, tc.wantPresent)
			}
			if tc.wantErr {
				if gotErr == nil {
					t.Errorf("argIntOpt() error = nil, want error containing %q", tc.errContains)
				} else if !strings.Contains(gotErr.Error(), tc.errContains) {
					t.Errorf("argIntOpt() error = %q, want containing %q", gotErr.Error(), tc.errContains)
				}
			} else {
				if gotErr != nil {
					t.Errorf("argIntOpt() error = %v, want nil", gotErr)
				}
				if gotVal != tc.wantVal {
					t.Errorf("argIntOpt() = %d, want %d", gotVal, tc.wantVal)
				}
			}
		})
	}
}

func marshalInto(v interface{}, target interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// The MCP stdio transport is newline-delimited JSON. Before v2.13.0 the
// server read only Content-Length frames, so a spec client's first message
// never completed a frame and the connection timed out.
func TestServeMCPNewlineDelimited(t *testing.T) {
	gateway := newMCPGateway(func(context.Context, []string) (string, error) { return `{}`, nil })
	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}` + "\n" +
			`{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
			"\n" +
			// The last message has no trailing newline.
			`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	var out bytes.Buffer
	if err := serveMCP(context.Background(), input, &out, gateway); err != nil {
		t.Fatalf("serveMCP() error = %v", err)
	}
	if strings.Contains(out.String(), "Content-Length") {
		t.Fatalf("newline-delimited requests got Content-Length replies:\n%s", out.String())
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d reply lines, want 2:\n%s", len(lines), out.String())
	}
	for i, line := range lines {
		var resp struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("reply %d is not one JSON line: %v\n%s", i+1, err, line)
		}
		if resp.ID != i+1 || len(resp.Result) == 0 {
			t.Fatalf("reply %d = %s, want a result for id %d", i+1, line, i+1)
		}
	}
	if !strings.Contains(lines[1], `"name":"trace"`) {
		t.Fatalf("tools/list reply does not list trace: %s", lines[1])
	}
}

// Content-Length framed callers keep working, and each reply uses the framing
// of the request it answers.
func TestServeMCPReplyFramingFollowsRequest(t *testing.T) {
	gateway := newMCPGateway(func(context.Context, []string) (string, error) { return `{}`, nil })
	var input bytes.Buffer
	if err := writeMCPFrame(&input, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)); err != nil {
		t.Fatal(err)
	}
	input.WriteString(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n")
	var out bytes.Buffer
	if err := serveMCP(context.Background(), &input, &out, gateway); err != nil {
		t.Fatalf("serveMCP() error = %v", err)
	}
	reader := bufio.NewReader(&out)
	first, err := readMCPFrame(reader)
	if err != nil || !strings.Contains(string(first), `"id":1`) {
		t.Fatalf("first reply is not a Content-Length frame for id 1: %v %s", err, first)
	}
	second, framing, err := readMCPMessage(reader)
	if err != nil || framing != mcpFramingNewline || !strings.Contains(string(second), `"id":2`) {
		t.Fatalf("second reply is not a newline-delimited message for id 2: %v %v %s", err, framing, second)
	}
}

// #619: trace exits 1 for a resource no GitOps tool manages, as the CLI
// contract says, while printing a valid JSON answer. The MCP result must carry
// that answer, not only the exit status.
func TestMCPCallToolKeepsJSONOutputOnNonZeroExit(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 1").Run()
	answer := `{"command":"trace","target":{"kind":"Deployment","namespace":"default","name":"hotfix-worker"},"chain":null,"summary":{"ownerType":"Native","source":null,"deployer":null}}`
	gateway := newMCPGateway(func(_ context.Context, args []string) (string, error) {
		return "", &mcpCommandError{args: args, stdout: answer, stderr: "", err: exitErr}
	})
	params, _ := json.Marshal(map[string]interface{}{"name": "trace", "arguments": map[string]string{"resource": "deployment/hotfix-worker", "namespace": "default"}})
	result := gateway.callTool(context.Background(), params)

	if result["isError"] != true {
		t.Errorf("isError = %v, want true: the command exited non-zero", result["isError"])
	}
	content := result["content"].([]map[string]string)
	if len(content) != 2 || content[0]["text"] != answer {
		t.Fatalf("content[0] should be the command's JSON answer, got %+v", content)
	}
	if !strings.Contains(content[1]["text"], "exited with status 1") || !strings.Contains(content[1]["text"], "trace deployment/hotfix-worker") {
		t.Errorf("content[1] should say which command exited with which status, got %q", content[1]["text"])
	}
}

// A failure without a JSON answer keeps the previous error text.
func TestMCPCallToolNonJSONFailureUnchanged(t *testing.T) {
	exitErr := exec.Command("sh", "-c", "exit 1").Run()
	gateway := newMCPGateway(func(_ context.Context, args []string) (string, error) {
		return "", &mcpCommandError{args: args, stdout: "not json", stderr: "Error: trace failed: argocd context appears stale", err: exitErr}
	})
	params, _ := json.Marshal(map[string]interface{}{"name": "trace", "arguments": map[string]string{"resource": "deployment/cart", "namespace": "shop"}})
	result := gateway.callTool(context.Background(), params)

	content := result["content"].([]map[string]string)
	want := "tool command failed (trace deployment/cart -n shop --format json): Error: trace failed: argocd context appears stale"
	if result["isError"] != true || len(content) != 1 || content[0]["text"] != want {
		t.Fatalf("got isError=%v content=%+v, want the unchanged message %q", result["isError"], content, want)
	}
}

// #635: on a 300-Deployment cluster the unfiltered map answer (268 KB) was
// larger than an MCP result may be. The tool now carries the CLI's filters
// and compact modes.
func TestMCPMapToolFiltersAndModes(t *testing.T) {
	var got []string
	gateway := newMCPGateway(func(_ context.Context, args []string) (string, error) {
		got = args
		return `{}`, nil
	})
	call := func(arguments map[string]interface{}) map[string]interface{} {
		params, _ := json.Marshal(map[string]interface{}{"name": "map", "arguments": arguments})
		return gateway.callTool(context.Background(), params)
	}

	call(map[string]interface{}{"kind": "Deployment", "owner": "Native", "summary": true})
	if want := "map list --json --kind Deployment --owner Native --summary"; strings.Join(got, " ") != want {
		t.Errorf("args = %q, want %q", strings.Join(got, " "), want)
	}
	call(map[string]interface{}{"namespace": "team-05", "query": "owner!=Native", "names_only": true})
	if want := "map list --json --namespace team-05 --query owner!=Native --names-only"; strings.Join(got, " ") != want {
		t.Errorf("args = %q, want %q", strings.Join(got, " "), want)
	}
	call(map[string]interface{}{"context": "cluster-b", "kind": "Deployment"})
	if want := "map list --json --kube-context cluster-b --kind Deployment"; strings.Join(got, " ") != want {
		t.Errorf("explicit context args = %q, want %q", strings.Join(got, " "), want)
	}
	call(map[string]interface{}{})
	if want := "map list --json"; strings.Join(got, " ") != want {
		t.Errorf("args = %q, want %q: an unfiltered call is unchanged", strings.Join(got, " "), want)
	}
	call(map[string]interface{}{"ownership_evidence": true})
	if want := "map list --json --ownership-evidence"; strings.Join(got, " ") != want {
		t.Errorf("args = %q, want %q", strings.Join(got, " "), want)
	}
	got = nil
	result := call(map[string]interface{}{"summary": true, "count": true})
	if result["isError"] != true || got != nil {
		t.Errorf("two output modes should be rejected before running anything; isError=%v args=%v", result["isError"], got)
	}
	for _, invalid := range []interface{}{"", "  ", nil, 42} {
		got = nil
		result = call(map[string]interface{}{"context": invalid})
		if result["isError"] != true || got != nil {
			t.Errorf("invalid explicit context %#v must fail before running; isError=%v args=%v", invalid, result["isError"], got)
		}
	}
	for _, invalid := range []map[string]interface{}{
		{"ownership_evidence": "true"},
		{"ownership_evidence": true, "summary": true},
		{"ownership_evidence": true, "names_only": true},
		{"ownership_evidence": true, "count": true},
	} {
		got = nil
		result := call(invalid)
		if result["isError"] != true || got != nil {
			t.Errorf("invalid ownership evidence arguments should fail before invoking runner: args=%v result=%v ran=%v", invalid, result, got)
		}
	}
}
