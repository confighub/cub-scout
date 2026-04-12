// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/confighub/cub-scout/pkg/hub"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "MCP gateway (read-only observation tools)",
	Long: `MCP gateway for exposing cub-scout observation tools to AI agents.

Standalone mode:
  Serves read-only observation tools using local cluster access.

Connected mode:
  Future versions can route richer context from ConfigHub while preserving
  read-only behavior in cub-scout itself.`,
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve MCP over stdio",
	Long: `Serve a read-only Model Context Protocol (MCP) gateway over stdio.

Supported tools in standalone mode:
  - doctor
  - map
  - trace
  - scan
  - explain

Additional tools in connected mode (when authenticated to ConfigHub):
  - compare_three_way
  - confighub_changesets
  - confighub_units
  - confighub_unit_get

Standalone tools are sourced from existing cub-scout CLI JSON output.
Connected tools are sourced from read-only cub CLI queries.`,
	RunE: runMCPServe,
}

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.AddCommand(mcpServeCmd)
}

type mcpToolRunner func(ctx context.Context, args []string) (string, error)

type mcpToolDescriptor struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type mcpTool struct {
	Descriptor mcpToolDescriptor
	BuildArgs  func(arguments map[string]interface{}) ([]string, error)
	Runner     mcpToolRunner
}

type mcpGateway struct {
	tools    map[string]mcpTool
	toolList []mcpToolDescriptor
	runTool  mcpToolRunner
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func runMCPServe(cmd *cobra.Command, args []string) error {
	gateway := newMCPGatewayWithMode(runMCPToolCommand, runMCPConnectedToolCommand, detectMCPConnectedMode())
	return serveMCP(cmd.Context(), os.Stdin, os.Stdout, gateway)
}

func serveMCP(ctx context.Context, in io.Reader, out io.Writer, gateway *mcpGateway) error {
	reader := bufio.NewReader(in)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		frame, err := readMCPFrame(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read mcp frame: %w", err)
		}

		var req mcpRequest
		if err := json.Unmarshal(frame, &req); err != nil {
			continue
		}

		resp := gateway.handleRequest(ctx, req)
		if resp == nil {
			continue
		}

		payload, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal mcp response: %w", err)
		}
		if err := writeMCPFrame(out, payload); err != nil {
			return fmt.Errorf("write mcp frame: %w", err)
		}
	}
}

func newMCPGateway(runner mcpToolRunner) *mcpGateway {
	return newMCPGatewayWithMode(runner, nil, false)
}

func newMCPGatewayWithMode(runner mcpToolRunner, connectedRunner mcpToolRunner, connected bool) *mcpGateway {
	if runner == nil {
		runner = runMCPToolCommand
	}
	if connectedRunner == nil {
		connectedRunner = runMCPConnectedToolCommand
	}

	tools := map[string]mcpTool{
		"doctor": {
			Descriptor: mcpToolDescriptor{
				Name:        "doctor",
				Description: "FIRST standalone tool to load for 'what's wrong?', 'what's broken?', or a compact cluster or namespace health summary. Also use when the user asks which cub-scout troubleshooting tool to start with, or whether cub-scout is the right first read-only step instead of raw kubectl or the Argo UI. Returns ownership, health, risks, drift, and next steps (doctor --format json). Use before explain, trace, or scan when the user has not narrowed to one resource yet.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Optional namespace scope (default: all namespaces).",
						},
						"top": map[string]interface{}{
							"type":        "integer",
							"description": "Number of top issues to include (default: 3).",
						},
					},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				args := []string{"doctor", "--format", "json"}
				if ns := argString(arguments, "namespace"); ns != "" {
					args = append(args, "-n", ns)
				}
				if top, present, err := argIntOpt(arguments, "top", false); err != nil {
					return nil, err
				} else if present {
					args = append(args, "--top", fmt.Sprintf("%d", top))
				}
				return args, nil
			},
		},
		"map": {
			Descriptor: mcpToolDescriptor{
				Name:        "map",
				Description: "Standalone resource inventory with ownership classification (map list --json). Use for 'what's running in this cluster or namespace?' and broad inventory questions, especially when the user wants more meaning than raw `kubectl get` output. DO NOT load for bare 'what's broken?' or one-resource root cause; use doctor first for health, then explain for a specific resource.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Optional namespace filter.",
						},
					},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				args := []string{"map", "list", "--json"}
				if ns := argString(arguments, "namespace"); ns != "" {
					args = append(args, "-n", ns)
				}
				return args, nil
			},
		},
		"scan": {
			Descriptor: mcpToolDescriptor{
				Name:        "scan",
				Description: "Standalone live-cluster configuration and runtime findings (scan --json). Use AFTER doctor when the user wants detailed risk or misconfiguration findings in the cluster or namespace right now, or an awareness scan of live state, not a broad health summary. DO NOT load first for bare 'what's wrong?' if doctor has not run yet. DO NOT use this as a governed promotion or revision-safety gate.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Optional namespace filter.",
						},
					},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				args := []string{"scan", "--json"}
				if ns := argString(arguments, "namespace"); ns != "" {
					args = append(args, "-n", ns)
				}
				return args, nil
			},
		},
		"trace": {
			Descriptor: mcpToolDescriptor{
				Name:        "trace",
				Description: "Exact ownership and source chain for one resource (trace --format json). Use AFTER doctor or explain once the user has narrowed to one resource and wants to know where it came from, which source or deployer owns it end-to-end, or what GitOps chain produced it. DO NOT load for broad cluster status or first-pass troubleshooting; use doctor first, then explain if the resource is still unclear.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"resource": map[string]interface{}{
							"type":        "string",
							"description": "Resource selector as kind/name (for example deployment/api).",
						},
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Optional namespace override.",
						},
					},
					"required":             []string{"resource"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				resource := argString(arguments, "resource")
				if resource == "" {
					return nil, fmt.Errorf("missing required argument: resource")
				}
				args := []string{"trace", resource}
				if ns := argString(arguments, "namespace"); ns != "" {
					args = append(args, "-n", ns)
				}
				args = append(args, "--format", "json")
				return args, nil
			},
		},
		"explain": {
			Descriptor: mcpToolDescriptor{
				Name:        "explain",
				Description: "Plain-English explanation for one resource: who owns it, health or drift, recent events, and what to do next (explain --format json). Use AFTER doctor or map once the user has narrowed to a specific resource, especially when the user wants more computed meaning than raw `kubectl describe`. DO NOT load for broad cluster inventory or health; use doctor first.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"resource": map[string]interface{}{
							"type":        "string",
							"description": "Resource selector as kind/name (for example deployment/api).",
						},
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Optional namespace override.",
						},
					},
					"required":             []string{"resource"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				resource := argString(arguments, "resource")
				if resource == "" {
					return nil, fmt.Errorf("missing required argument: resource")
				}
				args := []string{"explain", resource}
				if ns := argString(arguments, "namespace"); ns != "" {
					args = append(args, "-n", ns)
				}
				args = append(args, "--format", "json")
				return args, nil
			},
		},
	}
	if connected {
		tools["compare_three_way"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "compare_three_way",
				Description: "Connected-only DRY/WET/LIVE comparison for governed intent vs rendered deployer state vs live cluster state (compare three-way --format json). Use when the user asks whether governed state agrees with live state, whether ConfigHub, the deployer, and the cluster converge, or to compare governed state to live state for one resource, namespace, or the full cluster. Load after doctor, explain, or trace has identified the scope you care about. DO NOT load for first-pass troubleshooting when the scope is still unknown; use doctor first.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"scope": map[string]interface{}{
							"type":        "string",
							"description": "Required scope: <kind/name>, resource:<kind/name>, namespace/<ns>, or cluster.",
						},
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Optional namespace override for resource scope.",
						},
					},
					"required":             []string{"scope"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				scope := argString(arguments, "scope")
				if scope == "" {
					return nil, fmt.Errorf("missing required argument: scope")
				}
				args := []string{"compare", "three-way", "--format", "json", "--scope", scope}
				if ns := argString(arguments, "namespace"); ns != "" {
					args = append(args, "-n", ns)
				}
				return args, nil
			},
		}
		tools["confighub_changesets"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_changesets",
				Description: "Connected-only governed ChangeSet history and receipts from ConfigHub (cub changeset list --json). Use when the user asks what governed write changed a known unit or space, who changed it, or what receipt proves a governed write. Load after trace or confighub_units has identified the governed object or scope you care about. DO NOT load for current cluster health or ownership, or when the governed object is still unknown; use doctor, explain, trace, or confighub_units first.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space": map[string]interface{}{
							"type":        "string",
							"description": "Optional ConfigHub space slug or ID.",
						},
						"where": map[string]interface{}{
							"type":        "string",
							"description": "Optional filter expression passed to --where.",
						},
					},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				args := []string{"changeset", "list", "--json"}
				if space := argString(arguments, "space"); space != "" {
					args = append(args, "--space", space)
				}
				if where := argString(arguments, "where"); where != "" {
					args = append(args, "--where", where)
				}
				return args, nil
			},
			Runner: connectedRunner,
		}
		tools["confighub_units"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_units",
				Description: "Connected-only ConfigHub unit and fleet inventory / cluster-to-ConfigHub lookup (cub unit list --json). Use when the user asks which ConfigHub unit corresponds to something already identified, or wants governed unit inventory before drilling into one unit. Load after doctor, map, explain, or trace has established the cluster-side object you care about. DO NOT load for raw cluster inventory or exact unit facts; use map or doctor first, then confighub_unit_get once the unit is known.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space": map[string]interface{}{
							"type":        "string",
							"description": "Optional ConfigHub space slug or ID.",
						},
						"where": map[string]interface{}{
							"type":        "string",
							"description": "Optional filter expression passed to --where.",
						},
						"contains": map[string]interface{}{
							"type":        "string",
							"description": "Optional full-text contains query passed to --contains.",
						},
					},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				args := []string{"unit", "list", "--json"}
				if space := argString(arguments, "space"); space != "" {
					args = append(args, "--space", space)
				}
				if where := argString(arguments, "where"); where != "" {
					args = append(args, "--where", where)
				}
				if contains := argString(arguments, "contains"); contains != "" {
					args = append(args, "--contains", contains)
				}
				return args, nil
			},
			Runner: connectedRunner,
		}
		tools["confighub_unit_get"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_unit_get",
				Description: "Connected-only exact ConfigHub unit details and applied/live revision facts (cub unit get --json). Load ONLY after the user already has a unit slug or ID, or after confighub_units identified it. Use this for 'show me the intended state, last applied revision, or live revision for unit X.' If you do not have a unit yet, use confighub_units first. DO NOT load for bare cluster troubleshooting or governed-vs-live comparison; use explain, trace, or compare_three_way first.",
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"unit": map[string]interface{}{
							"type":        "string",
							"description": "Required unit slug or ID.",
						},
						"space": map[string]interface{}{
							"type":        "string",
							"description": "Optional ConfigHub space slug or ID.",
						},
					},
					"required":             []string{"unit"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				unit := argString(arguments, "unit")
				if unit == "" {
					return nil, fmt.Errorf("missing required argument: unit")
				}
				args := []string{"unit", "get", "--json", unit}
				if space := argString(arguments, "space"); space != "" {
					args = append(args, "--space", space)
				}
				return args, nil
			},
			Runner: connectedRunner,
		}
	}

	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)

	list := make([]mcpToolDescriptor, 0, len(names))
	for _, name := range names {
		list = append(list, tools[name].Descriptor)
	}

	return &mcpGateway{
		tools:    tools,
		toolList: list,
		runTool:  runner,
	}
}

func (g *mcpGateway) toolsForList() []mcpToolDescriptor {
	out := make([]mcpToolDescriptor, len(g.toolList))
	copy(out, g.toolList)
	return out
}

func (g *mcpGateway) handleRequest(ctx context.Context, req mcpRequest) *mcpResponse {
	if req.Method == "" {
		return g.errorResponse(req.ID, -32600, "invalid request: missing method")
	}

	switch req.Method {
	case "notifications/initialized":
		return nil
	case "initialize":
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2025-03-26",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "cub-scout-mcp",
					"version": BuildTag,
				},
			},
		}
	case "ping":
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]interface{}{},
		}
	case "tools/list":
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": g.toolsForList(),
			},
		}
	case "tools/call":
		result := g.callTool(ctx, req.Params)
		return &mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}
	default:
		return g.errorResponse(req.ID, -32601, "method not found")
	}
}

func (g *mcpGateway) callTool(ctx context.Context, paramsRaw json.RawMessage) map[string]interface{} {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return mcpToolError(fmt.Sprintf("invalid tools/call params: %v", err))
	}

	tool, ok := g.tools[params.Name]
	if !ok {
		return mcpToolError(fmt.Sprintf("unknown tool %q", params.Name))
	}
	if params.Arguments == nil {
		params.Arguments = map[string]interface{}{}
	}

	args, err := tool.BuildArgs(params.Arguments)
	if err != nil {
		return mcpToolError(err.Error())
	}

	runner := g.runTool
	if tool.Runner != nil {
		runner = tool.Runner
	}

	output, err := runner(ctx, args)
	if err != nil {
		return mcpToolError(err.Error())
	}

	return map[string]interface{}{
		"isError": false,
		"content": []map[string]string{
			{
				"type": "text",
				"text": output,
			},
		},
	}
}

func mcpToolError(msg string) map[string]interface{} {
	return map[string]interface{}{
		"isError": true,
		"content": []map[string]string{
			{
				"type": "text",
				"text": msg,
			},
		},
	}
}

func (g *mcpGateway) errorResponse(id json.RawMessage, code int, message string) *mcpResponse {
	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &mcpError{
			Code:    code,
			Message: message,
		},
	}
}

func argString(arguments map[string]interface{}, key string) string {
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func argInt(arguments map[string]interface{}, key string) int {
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return 0
	}
	// JSON numbers are decoded as float64
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

// argIntOpt returns an integer argument with presence and validation.
// Returns (value, true, nil) if key is present with a valid integer.
// Returns (0, false, nil) if key is absent or nil.
// Returns (0, true, error) if key is present but invalid (wrong type or negative when not allowed).
func argIntOpt(arguments map[string]interface{}, key string, allowNegative bool) (int, bool, error) {
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	var value int
	switch v := raw.(type) {
	case float64:
		if v != float64(int(v)) {
			return 0, true, fmt.Errorf("%s must be a whole number", key)
		}
		value = int(v)
	case int:
		value = v
	case int64:
		value = int(v)
	default:
		return 0, true, fmt.Errorf("%s must be an integer", key)
	}
	if !allowNegative && value < 0 {
		return 0, true, fmt.Errorf("%s must be non-negative", key)
	}
	return value, true, nil
}

func runMCPToolCommand(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("tool command failed (%s): %s", strings.Join(args, " "), msg)
	}

	return strings.TrimSpace(stdout.String()), nil
}

func runMCPConnectedToolCommand(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, "cub", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("connected tool command failed (cub %s): %s", strings.Join(args, " "), msg)
	}

	return strings.TrimSpace(stdout.String()), nil
}

func detectMCPConnectedMode() bool {
	if hub.CurrentMode() != hub.Connected {
		return false
	}
	_, err := exec.LookPath("cub")
	return err == nil
}

func readMCPFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" {
				return nil, io.EOF
			}
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])
		if key == "content-length" {
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid content-length %q", value)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing content-length header")
	}
	if contentLength == 0 {
		return []byte{}, nil
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func writeMCPFrame(w io.Writer, payload []byte) error {
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
