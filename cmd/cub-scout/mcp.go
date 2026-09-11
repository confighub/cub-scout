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

	"github.com/confighub/cub-scout/pkg/agent"
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
  - gitops_status

Additional tools in connected mode (when authenticated to ConfigHub):
  - compare_three_way
  - compare_source_truth
  - confighub_changesets
  - confighub_k8s_resources
  - confighub_k8s_types
  - confighub_live_status
  - confighub_releases
  - confighub_resources
  - confighub_unit_events
  - confighub_units
  - confighub_unit_get

Standalone tools are sourced from existing cub-scout CLI JSON output.
Connected tools are sourced from read-only cub CLI queries.

Doctor is the first troubleshooting and tool-choice entrypoint.
Connected tools add governed lookup, intended configuration, receipts, and
convergence facts once scope is known.`,
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
	Annotations *mcpToolAnnotations    `json:"annotations,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type mcpToolAnnotations struct {
	ReadOnlyHint bool `json:"readOnlyHint,omitempty"`
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

	readOnly := &mcpToolAnnotations{ReadOnlyHint: true}

	tools := map[string]mcpTool{
		"doctor": {
			Descriptor: mcpToolDescriptor{
				Name:        "doctor",
				Description: "FIRST standalone tool to load for 'what's wrong?', 'what's broken?', or a compact cluster or namespace health summary. Also use when the user asks which cub-scout troubleshooting tool to start with, whether cub-scout is the right first read-only step instead of raw kubectl or the Argo UI, or when local access to the cluster may itself be the problem (wrong context, stale kubeconfig, API unreachable). Returns ownership, health, risks, drift, rollout evidence, optional bounded ConfigHub delivery evidence, and next steps (doctor --format json). Use before explain, trace, or scan when the user has not narrowed to one resource yet.",
				Annotations: readOnly,
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
						"with_confighub": map[string]interface{}{
							"type":        "boolean",
							"description": "Include bounded ConfigHub release, unit-event, live-status, and event-consumer evidence. Requires cub auth for connected rows.",
						},
						"confighub_space": map[string]interface{}{
							"type":        "string",
							"description": "ConfigHub space for connected evidence. Defaults to the current cub space; pass '*' only for an explicit all-spaces read.",
						},
						"confighub_since": map[string]interface{}{
							"type":        "string",
							"description": "Lookback window for connected release/event evidence, for example 24h or 7d.",
						},
						"confighub_stale_after": map[string]interface{}{
							"type":        "string",
							"description": "Treat live-status writeback older than this as stale, for example 15m.",
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
				if argBool(arguments, "with_confighub") {
					args = append(args, "--with-confighub")
				}
				if space := argString(arguments, "confighub_space"); space != "" {
					args = append(args, "--confighub-space", space)
				}
				if since := argString(arguments, "confighub_since"); since != "" {
					args = append(args, "--confighub-since", since)
				}
				if staleAfter := argString(arguments, "confighub_stale_after"); staleAfter != "" {
					args = append(args, "--confighub-stale-after", staleAfter)
				}
				return args, nil
			},
		},
		"map": {
			Descriptor: mcpToolDescriptor{
				Name:        "map",
				Description: "Standalone resource inventory with ownership classification (map list --json). Use for 'what's running in this cluster or namespace?' and broad inventory questions, especially when the user wants more meaning than raw `kubectl get` output. DO NOT load for bare 'what's broken?' or one-resource root cause; use doctor first for health, then explain for a specific resource.",
				Annotations: readOnly,
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
					args = append(args, "--namespace", ns)
				}
				return args, nil
			},
		},
		"scan": {
			Descriptor: mcpToolDescriptor{
				Name:        "scan",
				Description: "Standalone live-cluster configuration and runtime findings (scan --json). Use AFTER doctor when the user wants detailed risk or misconfiguration findings in the cluster or namespace right now, or an awareness scan of live state, not a broad health summary. DO NOT load first for bare 'what's wrong?' if doctor has not run yet. DO NOT use this as a governed promotion or revision-safety gate.",
				Annotations: readOnly,
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
				Annotations: readOnly,
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
				Annotations: readOnly,
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
		"gitops_status": {
			Descriptor: mcpToolDescriptor{
				Name:        "gitops_status",
				Description: "Standalone GitOps/controller delivery status (gitops status --format json). Use when the user asks whether delegated delivery is healthy, whether an app or source revision is synced, what controller families cub-scout actually inspected, or whether missing status is absence vs RBAC/API omission. Returns backend, transport, sources, deployers, source/build/apply/sync stages, and controllerCoverage[] for Flux, Argo CD, ConfigHub, Sveltos, and Modelplane. Optional ConfigHub evidence is bounded by space and time. DO NOT use to force sync, retry delivery, or declare application success by itself; it is read-only evidence.",
				Annotations: readOnly,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Optional Kubernetes namespace scope.",
						},
						"with_confighub": map[string]interface{}{
							"type":        "boolean",
							"description": "Include bounded ConfigHub release, unit-event, live-status, and event-consumer evidence. Requires cub auth.",
						},
						"confighub_space": map[string]interface{}{
							"type":        "string",
							"description": "ConfigHub space for connected evidence. Defaults to the current cub space; pass '*' only for an explicit all-spaces read.",
						},
						"confighub_since": map[string]interface{}{
							"type":        "string",
							"description": "Lookback window for connected release/event evidence, for example 24h or 7d.",
						},
						"confighub_stale_after": map[string]interface{}{
							"type":        "string",
							"description": "Treat live-status writeback older than this as stale, for example 15m.",
						},
					},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				args := []string{"gitops", "status", "--format", "json"}
				if ns := argString(arguments, "namespace"); ns != "" {
					args = append(args, "-n", ns)
				}
				if argBool(arguments, "with_confighub") {
					args = append(args, "--with-confighub")
				}
				if space := argString(arguments, "confighub_space"); space != "" {
					args = append(args, "--confighub-space", space)
				}
				if since := argString(arguments, "confighub_since"); since != "" {
					args = append(args, "--confighub-since", since)
				}
				if staleAfter := argString(arguments, "confighub_stale_after"); staleAfter != "" {
					args = append(args, "--confighub-stale-after", staleAfter)
				}
				return args, nil
			},
		},
	}
	if connected {
		tools["compare_three_way"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "compare_three_way",
				Description: "Connected-only DRY/WET/LIVE comparison for governed intent vs rendered deployer state vs live cluster state (compare three-way --format json). Use when the user asks whether governed state agrees with live state, whether ConfigHub, the deployer, and the cluster converge, whether a change is sign-off-ready, or to compare governed state to live state for one resource, namespace, or the full cluster. Load after doctor, explain, or trace has identified the scope you care about. DO NOT load for first-pass troubleshooting when the scope is still unknown; use doctor first.",
				Annotations: readOnly,
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
		tools["compare_source_truth"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name: "compare_source_truth",
				// The description is deliberate about the architectural triad:
				// this tool emits *evidence*, not an acceptance verdict. Pilot
				// (or any agent) layers acceptance on top. Wording must never
				// imply that the tool can approve, repair, or mutate.
				Description: "Connected-only read-only source-truth EVIDENCE document for a single workload (compare source-truth --format json). Joins ConfigHub intent, the GitOps controller's observed source/revision/digest, and the live runtime into one structured verdict relative to a declared delivery strategy. Use when the user asks 'is this workload's source of truth consistent end-to-end?' or 'where is the disagreement between ConfigHub, the controller, and the cluster?'. Strategy is REQUIRED input — never inferred — and the contract refuses to emit PASS when any source/digest/runtime field is missing. Load after doctor, explain, or compare_three_way has identified the workload you care about. DO NOT use this tool to approve, repair, or accept anything; it produces evidence only, and acceptance is a Pilot/operator decision. DO NOT load for first-pass cluster troubleshooting; use doctor first.",
				Annotations: readOnly,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"target": map[string]interface{}{
							"type":        "string",
							"description": "Workload reference in `kind/name` form (e.g. `Deployment/rag-server`). Supported kinds in v0.1: Deployment, StatefulSet, DaemonSet.",
						},
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Required Kubernetes namespace of the workload.",
						},
						"strategy": map[string]interface{}{
							"type":        "string",
							"description": sourceTruthStrategySchemaDescription(),
							"enum":        sourceTruthStrategySchemaValues(),
						},
					},
					"required":             []string{"target", "namespace", "strategy"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				target := argString(arguments, "target")
				if target == "" {
					return nil, fmt.Errorf("missing required argument: target")
				}
				namespace := argString(arguments, "namespace")
				if namespace == "" {
					return nil, fmt.Errorf("missing required argument: namespace")
				}
				strategy := argString(arguments, "strategy")
				if strategy == "" {
					return nil, fmt.Errorf("missing required argument: strategy")
				}
				return []string{
					"compare", "source-truth", target,
					"-n", namespace,
					"--strategy", strategy,
					"--format", "json",
				}, nil
			},
		}
		tools["confighub_changesets"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_changesets",
				Description: "Connected-only governed ChangeSet history and receipts from ConfigHub (cub changeset list --json). Use when the user asks what governed write changed a known unit or space, who changed it, what receipt proves a governed write, or what approval trail sits behind the change. Load after trace or confighub_units has identified the governed object or scope you care about. DO NOT load for current cluster health or ownership, or when the governed object is still unknown; use doctor, explain, trace, or confighub_units first.",
				Annotations: readOnly,
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
		tools["confighub_k8s_resources"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_k8s_resources",
				Description: "Connected-only ConfigHub Resource-backed Kubernetes configuration reader (cub k8s get -o json). Use when the user asks what Kubernetes resources ConfigHub says should exist, wants intended configuration across a space or target, or needs a low-API-load ConfigHub-side alternative before hitting live clusters. This reads stored ConfigHub Resource data, not live cluster state. Type is REQUIRED, and either space or target is REQUIRED to keep the read bounded; pass space='*' only for an explicit all-spaces query. DO NOT use for rollout health, live drift, or source-truth proof by itself; pair with gitops_status, trace, compare_three_way, or compare_source_truth for controller/runtime evidence.",
				Annotations: readOnly,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"type": map[string]interface{}{
							"type":        "string",
							"description": "Required Kubernetes resource type selector accepted by cub k8s get, such as deploy, svc, all, Kind, or apps/v1/Deployment. Comma-separated types are allowed.",
						},
						"names": map[string]interface{}{
							"type":        "array",
							"description": "Optional Kubernetes resource names to match within the selected type(s).",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"space": map[string]interface{}{
							"type":        "string",
							"description": "ConfigHub space slug or ID; use '*' only for an explicit all-spaces read. Required unless target is provided.",
						},
						"target": map[string]interface{}{
							"type":        "array",
							"description": "Optional ConfigHub target scopes in space-slug/target-slug form. Required unless space is provided.",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Optional Kubernetes namespace filter.",
						},
						"where": map[string]interface{}{
							"type":        "string",
							"description": "Optional ConfigHub entity filter passed to --where, such as Unit.Slug or Space.Labels predicates.",
						},
						"where_resource": map[string]interface{}{
							"type":        "string",
							"description": "Optional resource-configuration filter passed to --where-resource, such as spec.replicas > 1.",
						},
						"show": map[string]interface{}{
							"type":        "string",
							"description": "Optional cub k8s get view. One of: list, detail, data. Defaults to list.",
							"enum":        []string{"list", "detail", "data"},
						},
					},
					"required":             []string{"type"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				resourceType := argString(arguments, "type")
				if resourceType == "" {
					return nil, fmt.Errorf("missing required argument: type")
				}
				names, err := argStringSlice(arguments, "names")
				if err != nil {
					return nil, err
				}
				targets, err := argStringSlice(arguments, "target")
				if err != nil {
					return nil, err
				}
				space := argString(arguments, "space")
				if space == "" && len(targets) == 0 {
					return nil, fmt.Errorf("missing required bounded scope: space or target")
				}

				args := []string{"k8s", "get", resourceType}
				args = append(args, names...)
				if space != "" {
					args = append(args, "--space", space)
				}
				for _, target := range targets {
					args = append(args, "--target", target)
				}
				if ns := argString(arguments, "namespace"); ns != "" {
					args = append(args, "-n", ns)
				}
				if where := argString(arguments, "where"); where != "" {
					args = append(args, "--where", where)
				}
				if whereResource := argString(arguments, "where_resource"); whereResource != "" {
					args = append(args, "--where-resource", whereResource)
				}
				if show := argString(arguments, "show"); show != "" {
					switch show {
					case "list", "detail", "data":
						args = append(args, "--show", show)
					default:
						return nil, fmt.Errorf("unknown show value %q; valid: list, detail, data", show)
					}
				}
				args = append(args, "-o", "json")
				return args, nil
			},
			Runner: connectedRunner,
		}
		tools["confighub_k8s_types"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_k8s_types",
				Description: "Connected-only ConfigHub Resource-backed Kubernetes type survey (cub k8s types -o json). Use when the user asks which Kubernetes types ConfigHub holds, wants to discover custom resources before a larger query, or needs the cheapest ConfigHub-side survey before fetching resource bodies. This reads stored ConfigHub Resource metadata, not live cluster state, and either space or target is REQUIRED to keep the read bounded; pass space='*' only for an explicit all-spaces query. DO NOT use for rollout health, live drift, or app success; follow with confighub_k8s_resources for intended config and gitops_status, trace, or compare_three_way for live/controller evidence.",
				Annotations: readOnly,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"type": map[string]interface{}{
							"type":        "string",
							"description": "Optional Kubernetes resource type selector accepted by cub k8s types, such as all, deploy, Kind, or apps/v1/Deployment. Comma-separated types are allowed.",
						},
						"space": map[string]interface{}{
							"type":        "string",
							"description": "ConfigHub space slug or ID; use '*' only for an explicit all-spaces read. Required unless target is provided.",
						},
						"target": map[string]interface{}{
							"type":        "array",
							"description": "Optional ConfigHub target scopes in space-slug/target-slug form. Required unless space is provided.",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"namespace": map[string]interface{}{
							"type":        "string",
							"description": "Optional Kubernetes namespace filter.",
						},
						"where": map[string]interface{}{
							"type":        "string",
							"description": "Optional ConfigHub entity filter passed to --where, such as Unit.Slug or Space.Labels predicates.",
						},
						"where_resource": map[string]interface{}{
							"type":        "string",
							"description": "Optional resource-configuration filter passed to --where-resource, such as spec.replicas > 1.",
						},
					},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				targets, err := argStringSlice(arguments, "target")
				if err != nil {
					return nil, err
				}
				space := argString(arguments, "space")
				if space == "" && len(targets) == 0 {
					return nil, fmt.Errorf("missing required bounded scope: space or target")
				}

				args := []string{"k8s", "types"}
				if resourceType := argString(arguments, "type"); resourceType != "" {
					args = append(args, resourceType)
				}
				if space != "" {
					args = append(args, "--space", space)
				}
				for _, target := range targets {
					args = append(args, "--target", target)
				}
				if ns := argString(arguments, "namespace"); ns != "" {
					args = append(args, "-n", ns)
				}
				if where := argString(arguments, "where"); where != "" {
					args = append(args, "--where", where)
				}
				if whereResource := argString(arguments, "where_resource"); whereResource != "" {
					args = append(args, "--where-resource", whereResource)
				}
				args = append(args, "-o", "json")
				return args, nil
			},
			Runner: connectedRunner,
		}
		tools["confighub_live_status"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_live_status",
				Description: "Connected-only ConfigHub live-status writeback reader (cub space list -o json). Use when the user asks whether the evented delivery/status feedback loop has reported sync status, application health, operation phase, observed revision, or freshness for a known space. Space is REQUIRED to keep the read bounded; pass '*' only when the user explicitly asks for all spaces. DO NOT use as proof that delivery succeeded by itself; it is best-effort reported evidence owned by the underlying controller.",
				Annotations: readOnly,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space": map[string]interface{}{
							"type":        "string",
							"description": "Required ConfigHub space slug, or '*' for an explicit all-spaces read.",
						},
					},
					"required":             []string{"space"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				space := argString(arguments, "space")
				if space == "" {
					return nil, fmt.Errorf("missing required argument: space")
				}
				return mcpConfigHubLiveStatusArgs(space), nil
			},
			Runner: connectedRunner,
		}
		tools["confighub_releases"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_releases",
				Description: "Connected-only bounded ConfigHub Release history (cub release list -o json). Use when the user asks which release or OCI bundle was published for a known space, target, or time window. Space is REQUIRED to avoid broad ConfigHub reads; add a where filter when narrowing by time, target, or release. DO NOT use for live cluster health; pair with gitops status, trace, or compare_source_truth for controller/runtime evidence.",
				Annotations: readOnly,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space": map[string]interface{}{
							"type":        "string",
							"description": "Required ConfigHub space slug or ID; '*' is allowed only for an explicit all-spaces read.",
						},
						"where": map[string]interface{}{
							"type":        "string",
							"description": "Optional filter expression passed to --where, usually time- or target-bounded.",
						},
					},
					"required":             []string{"space"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				space := argString(arguments, "space")
				if space == "" {
					return nil, fmt.Errorf("missing required argument: space")
				}
				args := []string{"release", "list", "--space", space, "-o", "json"}
				if where := argString(arguments, "where"); where != "" {
					args = append(args, "--where", where)
				}
				return args, nil
			},
			Runner: connectedRunner,
		}
		tools["confighub_resources"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_resources",
				Description: "Connected-only ConfigHub Resource entity query (cub resource list -o json). Use when the user asks for indexed resources across units/spaces, non-Kubernetes resources extracted from unit data, fleet-wide Data predicates, TargetID/resource-type/resource-name filters, or a lower-load alternative to iterating units. Space is REQUIRED to keep the read bounded; pass '*' only for an explicit all-spaces query. This reads ConfigHub's extracted Resource rows, not live cluster state. DO NOT use for rollout health, live drift, or app success by itself; pair with confighub_k8s_resources for Kubernetes-shaped intended config and gitops_status, trace, compare_three_way, or compare_source_truth for controller/runtime evidence.",
				Annotations: readOnly,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space": map[string]interface{}{
							"type":        "string",
							"description": "Required ConfigHub space slug or ID; use '*' only for an explicit all-spaces read.",
						},
						"where": map[string]interface{}{
							"type":        "string",
							"description": "Optional Resource/entity/Data filter passed to --where, such as ResourceType, ResourceName, TargetID, Unit.Labels.*, Space.Labels.*, or Data.* predicates.",
						},
						"contains": map[string]interface{}{
							"type":        "string",
							"description": "Optional full-text contains query passed to --contains.",
						},
						"select": map[string]interface{}{
							"type":        "string",
							"description": "Optional comma-separated fields passed to --select, for example ResourceType,ResourceName,Data.",
						},
						"filter": map[string]interface{}{
							"type":        "string",
							"description": "Optional ConfigHub filter slug or ID passed to --filter.",
						},
						"view": map[string]interface{}{
							"type":        "string",
							"description": "Optional ConfigHub view slug or ID passed to --view.",
						},
						"raw_data": map[string]interface{}{
							"type":        "boolean",
							"description": "Include each resource's original configuration as RawData in JSON output. Can be large; prefer select/where when possible.",
						},
					},
					"required":             []string{"space"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				space := argString(arguments, "space")
				if space == "" {
					return nil, fmt.Errorf("missing required argument: space")
				}

				args := []string{"resource", "list", "--space", space, "-o", "json"}
				if where := argString(arguments, "where"); where != "" {
					args = append(args, "--where", where)
				}
				if contains := argString(arguments, "contains"); contains != "" {
					args = append(args, "--contains", contains)
				}
				if selectFields := argString(arguments, "select"); selectFields != "" {
					args = append(args, "--select", selectFields)
				}
				if filter := argString(arguments, "filter"); filter != "" {
					args = append(args, "--filter", filter)
				}
				if view := argString(arguments, "view"); view != "" {
					args = append(args, "--view", view)
				}
				if argBool(arguments, "raw_data") {
					args = append(args, "--raw-data")
				}
				return args, nil
			},
			Runner: connectedRunner,
		}
		tools["confighub_unit_events"] = mcpTool{
			Descriptor: mcpToolDescriptor{
				Name:        "confighub_unit_events",
				Description: "Connected-only bounded ConfigHub UnitEvent reader (cub unit-event list -o json). Use when the user asks what event or source action happened for a known unit/space, whether an event was successful, or what event evidence supports a delivery receipt. Space is REQUIRED and unit is optional; add a where filter for a time window. DO NOT use as a cursor-consuming event consumer or a live watcher.",
				Annotations: readOnly,
				InputSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"space": map[string]interface{}{
							"type":        "string",
							"description": "Required ConfigHub space slug or ID; '*' is allowed only for an explicit all-spaces read.",
						},
						"unit": map[string]interface{}{
							"type":        "string",
							"description": "Optional unit slug or ID to narrow the event read.",
						},
						"where": map[string]interface{}{
							"type":        "string",
							"description": "Optional filter expression passed to --where, usually time-bounded.",
						},
					},
					"required":             []string{"space"},
					"additionalProperties": false,
				},
			},
			BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
				space := argString(arguments, "space")
				if space == "" {
					return nil, fmt.Errorf("missing required argument: space")
				}
				args := []string{"unit-event", "list"}
				if unit := argString(arguments, "unit"); unit != "" {
					args = append(args, unit)
				}
				args = append(args, "--space", space, "-o", "json")
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
				Description: "Connected-only ConfigHub unit and fleet inventory / cluster-to-ConfigHub lookup (cub unit list --json). Use when the user asks which ConfigHub unit corresponds to something already identified, wants governed unit inventory before drilling into one unit, or needs the first useful ConfigHub object to inspect after cluster-side discovery. Load after doctor, map, explain, or trace has established the cluster-side object you care about. DO NOT load for raw cluster inventory or exact unit facts; use map or doctor first, then confighub_unit_get once the unit is known.",
				Annotations: readOnly,
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
				Description: "Connected-only exact ConfigHub unit details and applied/live revision facts (cub unit get --json). Load ONLY after the user already has a unit slug or ID, or after confighub_units identified it. Use this for 'show me the intended state, last applied revision, or live revision for unit X,' or when you need exact unit facts before opening the GUI or other trust surfaces. If you do not have a unit yet, use confighub_units first. DO NOT load for bare cluster troubleshooting or governed-vs-live comparison; use explain, trace, or compare_three_way first.",
				Annotations: readOnly,
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

	result := map[string]interface{}{
		"isError": false,
		"content": []map[string]string{
			{
				"type": "text",
				"text": output,
			},
		},
	}
	if structured := buildMCPStructuredContent(params.Name, output); structured != nil {
		result["structuredContent"] = structured
	}
	return result
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

func argStringSlice(arguments map[string]interface{}, key string) ([]string, error) {
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return nil, nil
	}
	switch value := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s must contain only strings", key)
			}
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, nil
	case []string:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, nil
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil, nil
		}
		return []string{trimmed}, nil
	default:
		return nil, fmt.Errorf("%s must be a string array", key)
	}
}

func argBool(arguments map[string]interface{}, key string) bool {
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return false
	}
	value, ok := raw.(bool)
	return ok && value
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

func sourceTruthStrategySchemaValues() []string {
	values := make([]string, 0, len(agent.AllStrategies()))
	for _, strategy := range agent.AllStrategies() {
		values = append(values, string(strategy))
	}
	sort.Strings(values)
	return values
}

func sourceTruthStrategySchemaDescription() string {
	return "Declared delivery path. Required, never inferred. One of: " + strings.Join(sourceTruthStrategySchemaValues(), ", ") + "."
}

func mcpConfigHubLiveStatusArgs(space string) []string {
	args := []string{"space", "list", "-o", "json", "--select", "Slug,SpaceID,Annotations,Labels"}
	if strings.TrimSpace(space) != "*" {
		args = append(args, "--where", fmt.Sprintf("Slug = '%s'", configHubFilterQuote(space)))
	}
	return args
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
