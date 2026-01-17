// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// FluxTracer implements Tracer for Flux CD
type FluxTracer struct {
	// fluxPath is the path to the flux CLI (default: "flux")
	fluxPath string
}

// NewFluxTracer creates a new Flux tracer
func NewFluxTracer() *FluxTracer {
	return &FluxTracer{
		fluxPath: "flux",
	}
}

// NewFluxTracerWithPath creates a Flux tracer with a custom CLI path
func NewFluxTracerWithPath(path string) *FluxTracer {
	return &FluxTracer{
		fluxPath: path,
	}
}

// ToolName returns "flux"
func (f *FluxTracer) ToolName() string {
	return "flux"
}

// Available checks if the flux CLI is installed
func (f *FluxTracer) Available() bool {
	cmd := exec.Command(f.fluxPath, "version", "--client")
	return cmd.Run() == nil
}

// Trace runs flux trace and parses the output
func (f *FluxTracer) Trace(ctx context.Context, kind, name, namespace string) (*TraceResult, error) {
	// Build command: flux trace <kind> <name> -n <namespace>
	args := []string{"trace", strings.ToLower(kind), name}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}

	cmd := exec.CommandContext(ctx, f.fluxPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Get output (flux sometimes writes errors to stdout with exit code 0)
	output := stdout.String()
	stderrStr := stderr.String()

	// Check for error patterns in both stdout and stderr
	errorOutput := output + stderrStr
	if strings.Contains(errorOutput, "not managed") ||
		strings.Contains(errorOutput, "no Flux object found") ||
		strings.Contains(errorOutput, "object not managed") {
		return &TraceResult{
			Object: ResourceRef{
				Kind:      kind,
				Name:      name,
				Namespace: namespace,
			},
			FullyManaged: false,
			Tool:         "flux",
			TracedAt:     time.Now(),
			Error:        "resource not managed by Flux",
		}, nil
	}

	// Check for "failed to" errors (broken chain)
	if strings.Contains(errorOutput, "failed to") {
		// Extract the error message
		errMsg := strings.TrimSpace(errorOutput)
		return &TraceResult{
			Object: ResourceRef{
				Kind:      kind,
				Name:      name,
				Namespace: namespace,
			},
			FullyManaged: false,
			Tool:         "flux",
			TracedAt:     time.Now(),
			Error:        errMsg,
		}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("flux trace failed: %w: %s", err, stderrStr)
	}

	// Parse the output
	return f.parseTraceOutput(output, kind, name, namespace)
}

// parseTraceOutput parses the flux trace text output into a TraceResult
func (f *FluxTracer) parseTraceOutput(output, kind, name, namespace string) (*TraceResult, error) {
	result := &TraceResult{
		Object: ResourceRef{
			Kind:      kind,
			Name:      name,
			Namespace: namespace,
		},
		Chain:        []ChainLink{},
		FullyManaged: true,
		Tool:         "flux",
		TracedAt:     time.Now(),
	}

	// Parse sections separated by "---"
	sections := strings.Split(output, "---")

	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}

		link, err := f.parseSection(section)
		if err != nil {
			continue // Skip unparseable sections
		}

		if link != nil {
			// Check if any link is not ready
			if !link.Ready {
				result.FullyManaged = false
			}
			result.Chain = append(result.Chain, *link)
		}
	}

	// Reverse the chain so it goes from source to target
	// flux trace outputs target first, source last
	for i, j := 0, len(result.Chain)-1; i < j; i, j = i+1, j-1 {
		result.Chain[i], result.Chain[j] = result.Chain[j], result.Chain[i]
	}

	return result, nil
}

// parseSection parses a single section from flux trace output
func (f *FluxTracer) parseSection(section string) (*ChainLink, error) {
	link := &ChainLink{}

	scanner := bufio.NewScanner(strings.NewReader(section))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Parse key: value pairs
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Object":
			// Parse "Kind/Name" format
			objParts := strings.SplitN(value, "/", 2)
			if len(objParts) == 2 {
				link.Kind = objParts[0]
				link.Name = objParts[1]
			}
		case "Namespace":
			link.Namespace = value
		case "Status":
			link.Status = value
			// Determine readiness from status
			link.Ready = f.isReadyStatus(value)
		case "Revision":
			link.Revision = value
		case "Path":
			link.Path = value
		case "URL":
			link.URL = value
		case "Message":
			link.Message = value
			if !link.Ready && link.StatusReason == "" {
				link.StatusReason = value
			}
		// Handle resource type headers
		case "GitRepository", "OCIRepository", "HelmRepository", "Bucket":
			link.Kind = key
			link.Name = value
		case "Kustomization":
			link.Kind = "Kustomization"
			link.Name = value
		case "HelmRelease":
			link.Kind = "HelmRelease"
			link.Name = value
		case "HelmChart":
			link.Kind = "HelmChart"
			link.Name = value
		}
	}

	// Skip if we didn't get a valid link
	if link.Kind == "" && link.Name == "" {
		return nil, fmt.Errorf("no valid link data")
	}

	return link, nil
}

// isReadyStatus determines if a status string indicates healthy state
func (f *FluxTracer) isReadyStatus(status string) bool {
	status = strings.ToLower(status)

	// Check negative indicators FIRST (order matters: "not ready" before "ready")
	if strings.Contains(status, "failed") ||
		strings.Contains(status, "error") ||
		strings.Contains(status, "not ready") ||
		strings.Contains(status, "stalled") ||
		strings.Contains(status, "suspended") ||
		strings.Contains(status, "reconciling") ||
		strings.Contains(status, "pending") {
		return false
	}

	// Positive indicators
	if strings.Contains(status, "applied") ||
		strings.Contains(status, "succeeded") ||
		strings.Contains(status, "ready") ||
		strings.Contains(status, "up to date") ||
		strings.Contains(status, "stored") ||
		strings.Contains(status, "artifact is") {
		return true
	}

	// Default to not ready if uncertain
	return false
}

// TraceByOwnership traces a resource by first checking its ownership labels
func (f *FluxTracer) TraceByOwnership(ctx context.Context, ownership Ownership) (*TraceResult, error) {
	if ownership.Type != OwnerFlux {
		return nil, fmt.Errorf("resource not owned by Flux")
	}

	// For Flux, the ownership already tells us the managing resource
	// We can trace the deployer directly
	switch ownership.SubType {
	case "kustomization":
		return f.Trace(ctx, "Kustomization", ownership.Name, ownership.Namespace)
	case "helmrelease":
		return f.Trace(ctx, "HelmRelease", ownership.Name, ownership.Namespace)
	default:
		return nil, fmt.Errorf("unknown Flux owner subtype: %s", ownership.SubType)
	}
}

// extractRevision extracts a short revision from a full revision string
// e.g., "main@sha1:abc123def456" -> "abc123"
func extractRevision(rev string) string {
	if rev == "" {
		return ""
	}

	// Match sha1:xxxxx pattern
	re := regexp.MustCompile(`sha1:([a-f0-9]+)`)
	matches := re.FindStringSubmatch(rev)
	if len(matches) > 1 {
		sha := matches[1]
		if len(sha) > 7 {
			return sha[:7]
		}
		return sha
	}

	// Match @xxxx pattern
	if idx := strings.LastIndex(rev, "@"); idx != -1 {
		return rev[idx+1:]
	}

	return rev
}
