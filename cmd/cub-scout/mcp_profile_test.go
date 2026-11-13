// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func profileGateway(t *testing.T, spec string, connected bool, runner mcpToolRunner) *mcpGateway {
	t.Helper()
	profile, err := parseMCPToolProfile(spec)
	if err != nil {
		t.Fatalf("parseMCPToolProfile(%q): %v", spec, err)
	}
	gateway := newMCPGatewayWithMode(runner, runner, connected)
	gateway.connectedOnly = connectedOnlyToolNames(gateway.runTool, gateway.connectedRunner)
	gateway.profile = profile
	return gateway
}

func listedNames(gateway *mcpGateway) []string {
	var names []string
	for _, descriptor := range gateway.toolsForList() {
		names = append(names, descriptor.Name)
	}
	return names
}

func callText(t *testing.T, gateway *mcpGateway, name string, arguments map[string]interface{}) (string, bool) {
	t.Helper()
	params, err := json.Marshal(map[string]interface{}{"name": name, "arguments": arguments})
	if err != nil {
		t.Fatal(err)
	}
	result := gateway.callTool(context.Background(), params)
	isError, _ := result["isError"].(bool)
	content, _ := result["content"].([]map[string]string)
	if len(content) == 0 {
		t.Fatalf("no content in %v", result)
	}
	return content[0]["text"], isError
}

func TestMCPToolProfileParsing(t *testing.T) {
	for _, spec := range []string{"", "all", "ALL", " all "} {
		profile, err := parseMCPToolProfile(spec)
		if err != nil || !profile.all() || profile.label != "all" {
			t.Errorf("%q: profile=%+v err=%v, want all", spec, profile, err)
		}
	}
	core, err := parseMCPToolProfile("core")
	if err != nil || core.all() || !core.offersMore() || !core.lists("map") || core.lists("trace") {
		t.Errorf("core: %+v err=%v", core, err)
	}
	exact, err := parseMCPToolProfile(" trace, doctor ,trace")
	if err != nil || exact.label != "doctor,trace" || exact.offersMore() || exact.lists("map") {
		t.Errorf("exact list: %+v err=%v", exact, err)
	}
	// A connected tool is a known name even when no session exists.
	if _, err := parseMCPToolProfile("confighub_units,doctor,"); err != nil {
		t.Errorf("connected tool by name, trailing comma: %v", err)
	}
	if trailing, err := parseMCPToolProfile("core,"); err != nil || trailing.label != "core" {
		t.Errorf("\"core,\": %+v err=%v, want the core profile", trailing, err)
	}
	for spec, want := range map[string]string{
		"doctor,tarce":          "unknown tool(s) tarce",
		"core,trace":            "cannot be mixed",
		"doctor,cub_scout_more": "cannot be named in an exact list",
		"cub_scout_more":        "cannot be named in an exact list",
		"Trace":                 "unknown tool(s) Trace",
		" , ":                   "no tool named",
	} {
		if _, err := parseMCPToolProfile(spec); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err=%v, want it to say %q", spec, err, want)
		}
	}
}

// The default is unchanged: with no profile, the list is byte for byte what
// it was.
func TestMCPToolProfileAllListsEverythingAsBefore(t *testing.T) {
	for _, connected := range []bool{false, true} {
		plain := newMCPGatewayWithMode(nil, nil, connected)
		before, _ := json.Marshal(plain.toolList)
		after, _ := json.Marshal(profileGateway(t, "all", connected, nil).toolsForList())
		if string(before) != string(after) {
			t.Errorf("connected=%v: --tools all changed the tool list", connected)
		}
		unset, _ := json.Marshal(plain.toolsForList())
		if string(before) != string(unset) {
			t.Errorf("connected=%v: a gateway with no profile changed the tool list", connected)
		}
		// And the list itself is the one that existed before profiles did.
		want := []string{"doctor", "explain", "gitops_settings", "gitops_status", "map", "release_check", "scan", "trace"}
		if connected {
			want = []string{"compare_source_truth", "compare_three_way", "confighub_attestations", "confighub_changeorder_get", "confighub_changesets", "confighub_k8s_resources", "confighub_k8s_types", "confighub_live_status", "confighub_releases", "confighub_resources", "confighub_unit_events", "confighub_unit_get", "confighub_units", "doctor", "explain", "gitops_settings", "gitops_status", "map", "release_check", "scan", "trace"}
		}
		if got := listedNames(plain); !reflect.DeepEqual(got, want) {
			t.Errorf("connected=%v: default tools = %v, want %v", connected, got, want)
		}
	}
}

func TestMCPToolProfileCoreListsTheCoreAndMore(t *testing.T) {
	standalone := listedNames(profileGateway(t, "core", false, nil))
	if want := []string{"doctor", "explain", "gitops_status", "map", "cub_scout_more"}; !reflect.DeepEqual(standalone, want) {
		t.Errorf("standalone core = %v, want %v", standalone, want)
	}
	connected := listedNames(profileGateway(t, "core", true, nil))
	if want := []string{"compare_source_truth", "confighub_units", "doctor", "explain", "gitops_status", "map", "cub_scout_more"}; !reflect.DeepEqual(connected, want) {
		t.Errorf("connected core = %v, want %v", connected, want)
	}
	exact := listedNames(profileGateway(t, "trace,doctor", true, nil))
	if want := []string{"doctor", "trace"}; !reflect.DeepEqual(exact, want) {
		t.Errorf("exact list = %v, want %v", exact, want)
	}
}

func TestMCPToolProfileIsReportedAtInitialize(t *testing.T) {
	for spec, want := range map[string]string{"": "all", "core": "core", "trace,doctor": "doctor,trace"} {
		gateway := profileGateway(t, spec, false, nil)
		response := gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize"})
		info := response.Result.(map[string]interface{})["serverInfo"].(map[string]interface{})
		if info["toolProfile"] != want {
			t.Errorf("%q: serverInfo.toolProfile = %v, want %q", spec, info["toolProfile"], want)
		}
	}
}

func TestCubScoutMoreIndexDescribeAndRun(t *testing.T) {
	var ran [][]string
	runner := func(_ context.Context, args []string) (string, error) {
		ran = append(ran, args)
		return `{"ok":true}`, nil
	}
	gateway := profileGateway(t, "core", false, runner)

	// The index names exactly the tools the session can run and does not list.
	text, isError := callText(t, gateway, mcpMoreToolName, nil)
	if isError {
		t.Fatalf("index: %s", text)
	}
	var index struct {
		Profile  string
		Unlisted []struct{ Name, Summary string }
	}
	if err := json.Unmarshal([]byte(text), &index); err != nil {
		t.Fatalf("index is not JSON: %v\n%s", err, text)
	}
	var names []string
	for _, entry := range index.Unlisted {
		names = append(names, entry.Name)
		if entry.Summary == "" || len(entry.Summary) > 210 {
			t.Errorf("index summary for %s is %d bytes", entry.Name, len(entry.Summary))
		}
	}
	if want := []string{"gitops_settings", "release_check", "scan", "trace"}; !reflect.DeepEqual(names, want) || index.Profile != "core" {
		t.Errorf("index = %v (profile %q), want %v", names, index.Profile, want)
	}

	// Describing a tool returns the descriptor --tools all would have listed.
	text, isError = callText(t, gateway, mcpMoreToolName, map[string]interface{}{"tool": "trace"})
	if isError {
		t.Fatalf("describe: %s", text)
	}
	var described, listed interface{}
	if err := json.Unmarshal([]byte(text), &described); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(newMCPGatewayWithMode(nil, nil, false).tools["trace"].Descriptor)
	_ = json.Unmarshal(raw, &listed)
	if !reflect.DeepEqual(described, listed) {
		t.Errorf("cub_scout_more describes trace differently from the full catalog")
	}
	if len(ran) != 0 {
		t.Fatalf("describing a tool ran it: %v", ran)
	}

	// Running through cub_scout_more runs the same command a direct call does.
	arguments := map[string]interface{}{"resource": "deployment/api", "namespace": "prod"}
	text, isError = callText(t, gateway, mcpMoreToolName, map[string]interface{}{"tool": "trace", "arguments": arguments})
	if isError || text != `{"ok":true}` {
		t.Fatalf("run through cub_scout_more: isError=%v %s", isError, text)
	}
	direct := profileGateway(t, "all", false, runner)
	if _, isError := callText(t, direct, "trace", arguments); isError {
		t.Fatal("direct trace call failed")
	}
	if len(ran) != 2 || !reflect.DeepEqual(ran[0], ran[1]) {
		t.Errorf("dispatched and direct calls ran different commands: %v", ran)
	}
}

func TestCubScoutMoreRefusesWithTheSchema(t *testing.T) {
	runner := func(context.Context, []string) (string, error) {
		t.Fatal("a refused call must not run anything")
		return "", nil
	}
	gateway := profileGateway(t, "core", false, runner)
	for name, tc := range map[string]struct {
		arguments map[string]interface{}
		want      []string
	}{
		"a missing required argument":       {map[string]interface{}{"tool": "trace", "arguments": map[string]interface{}{}}, []string{"missing required argument(s) resource", "Input schema of trace", `"resource"`}},
		"an unknown argument":               {map[string]interface{}{"tool": "trace", "arguments": map[string]interface{}{"resource": "deployment/api", "namespce": "prod"}}, []string{"unknown argument(s) namespce", "Input schema of trace"}},
		"arguments that are not an object":  {map[string]interface{}{"tool": "trace", "arguments": "deployment/api"}, []string{"must be an object", "Input schema of trace"}},
		"a listed tool":                     {map[string]interface{}{"tool": "map"}, []string{"is listed in this session; call it directly"}},
		"an unknown tool":                   {map[string]interface{}{"tool": "tarce"}, []string{`unknown tool "tarce"`, "trace"}},
		"itself":                            {map[string]interface{}{"tool": mcpMoreToolName}, []string{"does not run itself"}},
		"arguments without a tool":          {map[string]interface{}{"arguments": map[string]interface{}{}}, []string{`needs "tool"`}},
		"a stray argument":                  {map[string]interface{}{"tol": "trace"}, []string{`takes only "tool" and "arguments"`}},
		"a tool that is not a string":       {map[string]interface{}{"tool": 5}, []string{`"tool" must be the exact name`}},
		"a null tool":                       {map[string]interface{}{"tool": nil}, []string{`"tool" must be the exact name`}},
		"a padded name":                     {map[string]interface{}{"tool": " trace "}, []string{`unknown tool " trace "`}},
		"null arguments":                    {map[string]interface{}{"tool": "scan", "arguments": nil}, []string{"must be an object", "Input schema of scan"}},
		"a required argument given as null": {map[string]interface{}{"tool": "trace", "arguments": map[string]interface{}{"resource": nil}}, []string{"missing required argument(s) resource"}},
	} {
		text, isError := callText(t, gateway, mcpMoreToolName, tc.arguments)
		if !isError {
			t.Errorf("%s: not refused: %s", name, text)
		}
		for _, want := range tc.want {
			if !strings.Contains(text, want) {
				t.Errorf("%s: refusal does not say %q: %s", name, want, text)
			}
		}
	}
}

func TestMCPToolProfileDirectCallsToUnlistedTools(t *testing.T) {
	runner := func(context.Context, []string) (string, error) {
		t.Fatal("an unlisted tool must not run on a direct call")
		return "", nil
	}
	arguments := map[string]interface{}{"resource": "deployment/api"}

	// core: the tool exists; the answer says how to reach it.
	text, isError := callText(t, profileGateway(t, "core", false, runner), "trace", arguments)
	if !isError || !strings.Contains(text, "exists but is not listed") || !strings.Contains(text, mcpMoreToolName) || strings.Contains(text, "unknown tool") {
		t.Errorf("core, direct call to trace: %s", text)
	}
	// An exact list is the host's choice of what may run: no way round it.
	exact := profileGateway(t, "doctor,map", false, runner)
	text, isError = callText(t, exact, "trace", arguments)
	if !isError || !strings.Contains(text, "does not offer it") || strings.Contains(text, mcpMoreToolName) {
		t.Errorf("exact list, direct call to trace: %s", text)
	}
	text, isError = callText(t, exact, mcpMoreToolName, map[string]interface{}{"tool": "trace", "arguments": arguments})
	if !isError || !strings.Contains(text, "unknown tool") {
		t.Errorf("an exact list must not serve cub_scout_more: %s", text)
	}
	// With no profile cub_scout_more does not exist.
	text, isError = callText(t, profileGateway(t, "all", false, runner), mcpMoreToolName, nil)
	if !isError || !strings.Contains(text, "unknown tool") {
		t.Errorf("all: cub_scout_more should be unknown: %s", text)
	}
	// A tool that exists nowhere is still unknown.
	text, isError = callText(t, profileGateway(t, "core", false, runner), "tarce", arguments)
	if !isError || !strings.Contains(text, `unknown tool "tarce"`) {
		t.Errorf("core, unknown tool: %s", text)
	}
}

// A profile survives the session changing under the server: logging in adds
// only the connected tools the profile lists, and the rest are reached through
// cub_scout_more.
func TestMCPToolProfileFollowsTheSession(t *testing.T) {
	gateway := profileGateway(t, "core", false, nil)
	connected := false
	gateway.sessionFn = func() bool { return connected }
	list := func() []string {
		response := gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"})
		var names []string
		for _, descriptor := range response.Result.(map[string]interface{})["tools"].([]mcpToolDescriptor) {
			names = append(names, descriptor.Name)
		}
		return names
	}
	if got := list(); len(got) != 5 {
		t.Fatalf("before login: %v", got)
	}
	connected = true
	got := list()
	if want := []string{"compare_source_truth", "confighub_units", "doctor", "explain", "gitops_status", "map", "cub_scout_more"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after login: %v, want %v", got, want)
	}
	if n := len(gateway.unlistedTools()); n != 15 {
		t.Errorf("after login cub_scout_more reaches %d tools, want 15 (21 less the 6 listed)", n)
	}
}

// A rule only the tool's own argument builder knows (compare_three_way needs
// scope or view) is refused through the dispatcher with the schema attached,
// like a rule the schema check knows.
func TestCubScoutMoreAttachesTheSchemaToTheToolsOwnRefusal(t *testing.T) {
	runner := func(context.Context, []string) (string, error) {
		t.Fatal("a refused call must not run anything")
		return "", nil
	}
	gateway := profileGateway(t, "core", true, runner)
	text, isError := callText(t, gateway, mcpMoreToolName, map[string]interface{}{"tool": "compare_three_way", "arguments": map[string]interface{}{}})
	if !isError || !strings.Contains(text, "Input schema of compare_three_way") {
		t.Errorf("refusal without the schema: %s", text)
	}
}

// A ConfigHub tool the profile does not list, in a session that cannot run it
// either: it exists, and the answer says so. It is not "unknown tool".
func TestMCPToolProfileUnlistedConnectedToolWithoutASession(t *testing.T) {
	gateway := profileGateway(t, "core", false, func(context.Context, []string) (string, error) {
		t.Fatal("nothing may run")
		return "", nil
	})
	text, isError := callText(t, gateway, "confighub_releases", map[string]interface{}{"space": "prod"})
	if !isError || !strings.Contains(text, "exists but is not listed") || strings.Contains(text, "unknown tool") {
		t.Errorf("direct call to an unlisted connected tool: %s", text)
	}
}

// An exact list is an allow-list in every way a caller can try.
func TestMCPToolProfileExactListIsAnAllowList(t *testing.T) {
	gateway := profileGateway(t, "doctor,map", true, func(context.Context, []string) (string, error) {
		t.Fatal("a tool outside the list ran")
		return "", nil
	})
	if got, want := listedNames(gateway), []string{"doctor", "map"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("listed %v, want %v", got, want)
	}
	for _, call := range []struct {
		name      string
		arguments map[string]interface{}
	}{
		{"trace", map[string]interface{}{"resource": "deployment/api"}},
		{"Trace", map[string]interface{}{"resource": "deployment/api"}},
		{" trace", map[string]interface{}{"resource": "deployment/api"}},
		{"confighub_units", map[string]interface{}{"space": "prod"}},
		{mcpMoreToolName, nil},
		{mcpMoreToolName, map[string]interface{}{"tool": "trace", "arguments": map[string]interface{}{"resource": "deployment/api"}}},
	} {
		if text, isError := callText(t, gateway, call.name, call.arguments); !isError {
			t.Errorf("%q %v was not refused: %s", call.name, call.arguments, text)
		}
	}
}

func TestResolveMCPToolsSpec(t *testing.T) {
	cases := []struct {
		name               string
		typed              bool
		flag, env          string
		recording          bool
		want, wantErrorHas string
	}{
		{name: "nothing set", want: ""},
		{name: "environment alone", env: "core", want: "core"},
		{name: "the flag wins", typed: true, flag: "all", env: "core", want: "all"},
		{name: "an empty flag still wins", typed: true, flag: "", env: "core", want: ""},
		{name: "recording ignores the environment", env: "core", recording: true, want: ""},
		{name: "recording with a typed profile is refused", typed: true, flag: "core", recording: true, wantErrorHas: "no effect with --recording"},
		{name: "recording with a typed list is refused", typed: true, flag: "doctor", recording: true, wantErrorHas: "no effect with --recording"},
		{name: "recording with --tools all is allowed", typed: true, flag: "all", recording: true, want: "all"},
	}
	for _, tc := range cases {
		got, err := resolveMCPToolsSpec(tc.typed, tc.flag, tc.env, tc.recording)
		if tc.wantErrorHas != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErrorHas) {
				t.Errorf("%s: err=%v, want it to say %q", tc.name, err, tc.wantErrorHas)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q err=%v, want %q", tc.name, got, err, tc.want)
		}
	}
}
