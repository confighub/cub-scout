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
  - map
  - trace
  - scan
  - explain

Additional tools in connected mode (when authenticated to ConfigHub):
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
		"map": {
			Descriptor: mcpToolDescriptor{
				Name:        "map",
				Description: "List resources with ownership classification (map list --json).",
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
				Description: "Run configuration and runtime scan findings (scan --json).",
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
				Description: "Trace ownership chain for one resource (trace --format json).",
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
				Description: "Plain-English ownership and lineage summary (explain --format json).",
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
		tools["confighub_changesets"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_changesets",
				Description: "Connected-mode ChangeSet history from ConfigHub (cub changeset list --json).",
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
				Description: "Connected-mode fleet/unit context from ConfigHub (cub unit list --json).",
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
				Description: "Connected-mode unit details from ConfigHub (cub unit get --json).",
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
