// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// An agent pays for every listed tool at the start of every session. A tool
// profile chooses which tools `tools/list` returns; it never changes what a
// tool does. See docs/proposals/mcp-default-tool-set.md.
//
//	all    every tool the session can run (the default; unchanged behaviour)
//	core   a small set, plus cub_scout_more, which reaches the rest
//	a,b,c  exactly these tools, and nothing else is callable. This is an
//	       allow-list, so cub_scout_more, which reaches every tool, cannot be
//	       named in it.

const (
	mcpToolProfileAll  = "all"
	mcpToolProfileCore = "core"
	mcpMoreToolName    = "cub_scout_more"
	mcpToolProfileEnv  = "CUB_SCOUT_MCP_TOOLS"
)

// mcpCoreTools is the core set. The standalone four were chosen from how often
// each tool appears in the recorded eval runs, and confirmed by the maintainer
// on 2026-11-13 (map stays). The last two are listed only when the session can
// run them.
var mcpCoreTools = []string{
	"doctor", "explain", "map", "gitops_status",
	"compare_source_truth", "confighub_units",
}

type mcpToolProfile struct {
	// label is what the host asked for, normalised: "all", "core" or the
	// sorted comma-separated list.
	label string
	// listed is nil for "all". Otherwise it is every name the profile lists,
	// whether or not the session can run it right now.
	listed map[string]bool
}

func (p mcpToolProfile) all() bool { return p.listed == nil }

func (p mcpToolProfile) lists(name string) bool { return p.all() || p.listed[name] }

// offersMore reports whether the profile gives the agent cub_scout_more.
func (p mcpToolProfile) offersMore() bool { return !p.all() && p.listed[mcpMoreToolName] }

// knownMCPToolNames is every tool name the gateway can ever serve, standalone
// or connected, so a profile can be checked at start-up whatever the session
// is at that moment.
func knownMCPToolNames() map[string]bool {
	known := map[string]bool{}
	for name := range newMCPGatewayWithMode(nil, nil, true).tools {
		known[name] = true
	}
	return known
}

// parseMCPToolProfile turns --tools (or CUB_SCOUT_MCP_TOOLS) into a profile.
// An unknown name is an error: a misspelt tool silently left out would look,
// to the agent, like a tool that does not exist.
func parseMCPToolProfile(spec string) (mcpToolProfile, error) {
	spec = strings.TrimSpace(spec)
	switch strings.ToLower(spec) {
	case "", mcpToolProfileAll:
		return mcpToolProfile{label: mcpToolProfileAll}, nil
	case mcpToolProfileCore:
		listed := map[string]bool{mcpMoreToolName: true}
		for _, name := range mcpCoreTools {
			listed[name] = true
		}
		return mcpToolProfile{label: mcpToolProfileCore, listed: listed}, nil
	}
	var parts []string
	for _, part := range strings.Split(spec, ",") {
		if name := strings.TrimSpace(part); name != "" {
			parts = append(parts, name)
		}
	}
	if len(parts) == 1 && (strings.EqualFold(parts[0], mcpToolProfileAll) || strings.EqualFold(parts[0], mcpToolProfileCore)) {
		return parseMCPToolProfile(parts[0])
	}
	known := knownMCPToolNames()
	listed := map[string]bool{}
	var unknown []string
	for _, name := range parts {
		if name == mcpToolProfileAll || name == mcpToolProfileCore {
			return mcpToolProfile{}, fmt.Errorf("--tools: %q is a profile and cannot be mixed with tool names; use it alone", name)
		}
		if name == mcpMoreToolName {
			return mcpToolProfile{}, fmt.Errorf("--tools: %s reaches every tool, so it cannot be named in an exact list, which allows only the tools it names; use %q to get it", mcpMoreToolName, mcpToolProfileCore)
		}
		if !known[name] {
			unknown = append(unknown, name)
			continue
		}
		listed[name] = true
	}
	if len(unknown) > 0 {
		names := make([]string, 0, len(known))
		for name := range known {
			names = append(names, name)
		}
		sort.Strings(names)
		sort.Strings(unknown)
		return mcpToolProfile{}, fmt.Errorf("--tools: unknown tool(s) %s; use %q, %q or a comma-separated list of: %s", strings.Join(unknown, ", "), mcpToolProfileAll, mcpToolProfileCore, strings.Join(names, ", "))
	}
	if len(listed) == 0 {
		return mcpToolProfile{}, fmt.Errorf("--tools: no tool named; use %q, %q or a comma-separated list of tool names", mcpToolProfileAll, mcpToolProfileCore)
	}
	names := make([]string, 0, len(listed))
	for name := range listed {
		names = append(names, name)
	}
	sort.Strings(names)
	return mcpToolProfile{label: strings.Join(names, ","), listed: listed}, nil
}

func mcpMoreToolDescriptor() mcpToolDescriptor {
	return mcpToolDescriptor{
		Name:        mcpMoreToolName,
		Description: "Reaches the cub-scout read-only tools this session does not list (for example trace, scan, release_check, gitops_settings and the ConfigHub readers). No arguments: an index of them, one line each. With tool: that tool's full description and input schema. With tool and arguments: runs it and returns its own answer. Use when the listed tools do not answer the question. Runs only cub-scout's own tools, by exact name.",
		Annotations: &mcpToolAnnotations{ReadOnlyHint: true},
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tool": map[string]interface{}{
					"type":        "string",
					"description": "Exact name of an unlisted tool, from the index.",
				},
				"arguments": map[string]interface{}{
					"type":        "object",
					"description": "Arguments for that tool, as its input schema describes. Present (even empty) means run it; absent means describe it.",
				},
			},
			"additionalProperties": false,
		},
	}
}

// listedTools is what tools/list returns under the gateway's profile: the
// tools the session can run that the profile lists, in the usual order, then
// cub_scout_more when the profile offers it.
func (g *mcpGateway) listedTools() []mcpToolDescriptor {
	if g.profile.all() {
		out := make([]mcpToolDescriptor, len(g.toolList))
		copy(out, g.toolList)
		return out
	}
	out := make([]mcpToolDescriptor, 0, len(g.profile.listed))
	for _, descriptor := range g.toolList {
		if g.profile.listed[descriptor.Name] {
			out = append(out, descriptor)
		}
	}
	if g.profile.offersMore() {
		out = append(out, mcpMoreToolDescriptor())
	}
	return out
}

// unlistedTools are the tools the session can run that the profile does not
// list: what cub_scout_more reaches.
func (g *mcpGateway) unlistedTools() []mcpToolDescriptor {
	var out []mcpToolDescriptor
	for _, descriptor := range g.toolList {
		if !g.profile.lists(descriptor.Name) {
			out = append(out, descriptor)
		}
	}
	return out
}

// notListedError answers a direct call to a tool that exists but that this
// profile does not list. It is not an unknown tool, and the answer says how
// to reach it, or that the host chose not to offer it.
func (g *mcpGateway) notListedError(name string) map[string]interface{} {
	if g.profile.offersMore() {
		return mcpToolError(fmt.Sprintf("tool %q exists but is not listed in this session's tool profile (%s). Call %s with {\"tool\": %q} for its description and input schema, or add \"arguments\" to run it.", name, g.profile.label, mcpMoreToolName, name))
	}
	return mcpToolError(fmt.Sprintf("tool %q exists but this server was started with a tool profile that does not offer it (--tools %s).", name, g.profile.label))
}

// mcpToolSummary is the first sentence of a description, for the index.
func mcpToolSummary(description string) string {
	summary := description
	if i := strings.Index(summary, ". "); i >= 0 {
		summary = summary[:i+1]
	}
	const limit = 200
	if len(summary) > limit {
		summary = strings.TrimSpace(summary[:limit]) + "..."
	}
	return summary
}

// callMore serves cub_scout_more: the index, one tool's descriptor, or a run
// of one unlisted tool.
func (g *mcpGateway) callMore(ctx context.Context, arguments map[string]interface{}) map[string]interface{} {
	for key := range arguments {
		if key != "tool" && key != "arguments" {
			return mcpToolError(fmt.Sprintf("%s takes only \"tool\" and \"arguments\"; got %q", mcpMoreToolName, key))
		}
	}
	unlisted := g.unlistedTools()
	// The name is taken exactly as given: no trimming, as in a direct call.
	name := ""
	if rawTool, given := arguments["tool"]; given {
		text, isString := rawTool.(string)
		if !isString || text == "" {
			return mcpToolError(fmt.Sprintf("%s: \"tool\" must be the exact name of a tool, as a string", mcpMoreToolName))
		}
		name = text
	}
	rawArguments, run := arguments["arguments"]

	if name == "" {
		if run {
			return mcpToolError(fmt.Sprintf("%s: \"arguments\" needs \"tool\", the exact name of the tool to run", mcpMoreToolName))
		}
		type entry struct {
			Name    string `json:"name"`
			Summary string `json:"summary"`
		}
		index := struct {
			Profile  string  `json:"profile"`
			Unlisted []entry `json:"unlisted"`
			Use      string  `json:"use"`
		}{Profile: g.profile.label, Unlisted: []entry{}, Use: fmt.Sprintf("Call %s with {\"tool\": \"<name>\"} for a tool's full description and input schema, and add \"arguments\" to run it.", mcpMoreToolName)}
		for _, descriptor := range unlisted {
			index.Unlisted = append(index.Unlisted, entry{Name: descriptor.Name, Summary: mcpToolSummary(descriptor.Description)})
		}
		return mcpJSONResult(index)
	}

	tool, ok := g.tools[name]
	switch {
	case name == mcpMoreToolName:
		return mcpToolError(fmt.Sprintf("%s does not run itself", mcpMoreToolName))
	case ok && g.profile.lists(name):
		return mcpToolError(fmt.Sprintf("tool %q is listed in this session; call it directly", name))
	case !ok:
		// The same answer a direct call gives: a ConfigHub tool this session
		// cannot run says why, and anything else is unknown.
		if g.connectedOnly[name] {
			if err := requireConfigHubFor(name); err != nil {
				return mcpToolError(err.Error())
			}
		}
		names := make([]string, 0, len(unlisted))
		for _, descriptor := range unlisted {
			names = append(names, descriptor.Name)
		}
		return mcpToolError(fmt.Sprintf("unknown tool %q; the unlisted tools are: %s", name, strings.Join(names, ", ")))
	}

	if !run {
		return mcpJSONResult(tool.Descriptor)
	}
	// null is not "run with no arguments": leave "arguments" out to describe
	// the tool, or pass {} to run it with none.
	toolArguments, isObject := rawArguments.(map[string]interface{})
	if !isObject {
		return g.withSchema(name, tool, fmt.Sprintf("%s: \"arguments\" must be an object", mcpMoreToolName))
	}
	if problem := mcpArgumentsAgainstSchema(tool.Descriptor.InputSchema, toolArguments); problem != "" {
		return g.withSchema(name, tool, problem)
	}
	// The tool's own argument builder knows rules the schema check above
	// does not (one-of, value shapes). Its refusal carries the schema too.
	if _, err := tool.BuildArgs(toolArguments); err != nil {
		return g.withSchema(name, tool, err.Error())
	}
	return g.runNamedTool(ctx, name, tool, toolArguments)
}

// withSchema refuses a dispatched call and gives the shape that would work.
// The host could not check these arguments against a schema, because the
// tool was not listed; the refusal carries the schema for that reason.
func (g *mcpGateway) withSchema(name string, tool mcpTool, problem string) map[string]interface{} {
	schema, _ := json.Marshal(tool.Descriptor.InputSchema)
	return mcpToolError(fmt.Sprintf("%s: %s. Input schema of %s: %s", name, problem, name, schema))
}

// mcpArgumentsAgainstSchema makes the checks an MCP host would have made had
// the tool been listed: no unknown argument where the schema forbids them,
// and every required one present. Types are left to the tool's own argument
// builder, which already refuses what it cannot use.
func mcpArgumentsAgainstSchema(schema map[string]interface{}, arguments map[string]interface{}) string {
	properties, _ := schema["properties"].(map[string]interface{})
	// Closed means the schema says additionalProperties: false. Every
	// cub-scout tool does; an open schema lets any argument pass.
	additional, declared := schema["additionalProperties"].(bool)
	if closed := declared && !additional; closed {
		var unknown []string
		for key := range arguments {
			if _, ok := properties[key]; !ok {
				unknown = append(unknown, key)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return "unknown argument(s) " + strings.Join(unknown, ", ")
		}
	}
	var required []string
	switch list := schema["required"].(type) {
	case []string:
		required = list
	case []interface{}:
		for _, item := range list {
			if text, ok := item.(string); ok {
				required = append(required, text)
			}
		}
	}
	var missing []string
	for _, key := range required {
		if value, ok := arguments[key]; !ok || value == nil {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return "missing required argument(s) " + strings.Join(missing, ", ")
	}
	return ""
}

func mcpJSONResult(value interface{}) map[string]interface{} {
	text, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return mcpToolError(err.Error())
	}
	return map[string]interface{}{
		"isError": false,
		"content": []map[string]string{{"type": "text", "text": string(text)}},
	}
}

// resolveMCPToolsSpec decides which --tools value applies. The flag, when it
// was typed, wins over the environment. With --recording the server offers a
// fixed pair of tools: a typed --tools is a contradiction and is refused, but
// a value that only came from the environment is ignored, since the person
// did not ask for it on this command.
func resolveMCPToolsSpec(flagTyped bool, flagValue, envValue string, recording bool) (string, error) {
	if flagTyped {
		if recording {
			if profile, err := parseMCPToolProfile(flagValue); err != nil || !profile.all() {
				return "", fmt.Errorf("--tools has no effect with --recording, which serves a fixed pair of tools; drop one of them")
			}
		}
		return flagValue, nil
	}
	if recording {
		return "", nil
	}
	return envValue, nil
}
