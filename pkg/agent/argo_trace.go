// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ArgoTracer implements Tracer for Argo CD
type ArgoTracer struct {
	// argocdPath is the path to the argocd CLI (default: "argocd")
	argocdPath string
}

// NewArgoTracer creates a new Argo CD tracer
func NewArgoTracer() *ArgoTracer {
	return &ArgoTracer{
		argocdPath: "argocd",
	}
}

// NewArgoTracerWithPath creates an Argo tracer with a custom CLI path
func NewArgoTracerWithPath(path string) *ArgoTracer {
	return &ArgoTracer{
		argocdPath: path,
	}
}

// ToolName returns "argocd"
func (a *ArgoTracer) ToolName() string {
	return "argocd"
}

// Available checks if the argocd CLI is installed and logged in
func (a *ArgoTracer) Available() bool {
	cmd := exec.Command(a.argocdPath, "version", "--client")
	return cmd.Run() == nil
}

// Trace gets the full ownership chain for an Argo CD managed resource
func (a *ArgoTracer) Trace(ctx context.Context, kind, name, namespace string) (*TraceResult, error) {
	// For Argo, we need to find the Application that manages this resource
	// If kind is "Application", trace it directly
	if kind == "Application" {
		return a.traceApplication(ctx, name, namespace)
	}

	// For other resources, we need to find the owning Application
	// This requires checking the resource's labels
	return nil, fmt.Errorf("for non-Application resources, use --app flag to specify the Argo Application")
}

// TraceApplication traces an Argo CD Application
func (a *ArgoTracer) TraceApplication(ctx context.Context, appName string) (*TraceResult, error) {
	return a.traceApplication(ctx, appName, "")
}

// traceApplication gets the full status of an Argo CD Application
func (a *ArgoTracer) traceApplication(ctx context.Context, appName, namespace string) (*TraceResult, error) {
	// Run: argocd app get <name> -o json
	args := []string{"app", "get", appName, "-o", "json"}

	cmd := exec.CommandContext(ctx, a.argocdPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		stderrStr := stderr.String()
		stdoutStr := stdout.String()
		combinedOutput := stderrStr + stdoutStr

		if strings.Contains(combinedOutput, "not found") ||
			strings.Contains(combinedOutput, "does not exist") {
			return &TraceResult{
				Object: ResourceRef{
					Kind:      "Application",
					Name:      appName,
					Namespace: namespace,
				},
				FullyManaged: false,
				Tool:         "argocd",
				TracedAt:     time.Now(),
				Error:        fmt.Sprintf("Application '%s' not found", appName),
			}, nil
		}

		// Check for authentication/connection errors
		if strings.Contains(combinedOutput, "server address unspecified") ||
			strings.Contains(combinedOutput, "not logged in") ||
			strings.Contains(combinedOutput, "authentication required") {
			return nil, fmt.Errorf("argocd CLI not connected - run 'argocd login <server>' first")
		}

		return nil, fmt.Errorf("argocd app get failed: %w: %s", err, combinedOutput)
	}

	// Parse the JSON output
	return a.parseAppOutput(stdout.Bytes(), appName, namespace)
}

// argoApp represents the structure of argocd app get output
type argoApp struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Source struct {
			RepoURL        string `json:"repoURL"`
			Path           string `json:"path"`
			TargetRevision string `json:"targetRevision"`
			Chart          string `json:"chart"`
		} `json:"source"`
		Destination struct {
			Server    string `json:"server"`
			Namespace string `json:"namespace"`
		} `json:"destination"`
	} `json:"spec"`
	Status struct {
		Sync struct {
			Status   string `json:"status"`
			Revision string `json:"revision"`
		} `json:"sync"`
		Health struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"health"`
		OperationState *struct {
			Phase   string `json:"phase"`
			Message string `json:"message"`
		} `json:"operationState"`
		Resources []struct {
			Group     string `json:"group"`
			Version   string `json:"version"`
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			Health    *struct {
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"health"`
		} `json:"resources"`
		History []struct {
			Revision   string    `json:"revision"`
			DeployedAt time.Time `json:"deployedAt"`
		} `json:"history"`
	} `json:"status"`
}

// parseAppOutput parses argocd app get JSON output
func (a *ArgoTracer) parseAppOutput(data []byte, appName, namespace string) (*TraceResult, error) {
	var app argoApp
	if err := json.Unmarshal(data, &app); err != nil {
		return nil, fmt.Errorf("parse argocd output: %w", err)
	}

	result := &TraceResult{
		Object: ResourceRef{
			Kind:      "Application",
			Name:      appName,
			Namespace: namespace,
		},
		Chain:        []ChainLink{},
		FullyManaged: true,
		Tool:         "argocd",
		TracedAt:     time.Now(),
	}

	// Add source as first chain link (simulating a GitRepository)
	sourceLink := ChainLink{
		Kind:     "Source",
		Name:     extractRepoName(app.Spec.Source.RepoURL),
		URL:      app.Spec.Source.RepoURL,
		Path:     app.Spec.Source.Path,
		Revision: app.Spec.Source.TargetRevision,
		Ready:    true, // Argo doesn't track source health separately
		Status:   "Available",
	}
	if app.Spec.Source.Chart != "" {
		sourceLink.Kind = "HelmChart"
		sourceLink.Name = app.Spec.Source.Chart
	}

	// Parse OCI source if applicable
	if strings.HasPrefix(app.Spec.Source.RepoURL, "oci://") {
		ociInfo := ParseOCISource(app.Spec.Source.RepoURL)
		sourceLink.OCISource = &ociInfo

		// For ConfigHub OCI sources, update the Kind and Name
		if ociInfo.IsConfigHub {
			sourceLink.Kind = "ConfigHub OCI"
			sourceLink.Name = FormatConfigHubOCISource(ociInfo)
		} else {
			sourceLink.Kind = "OCIRepository"
		}
	}

	result.Chain = append(result.Chain, sourceLink)

	// Add Application as second link
	appReady := app.Status.Sync.Status == "Synced" && app.Status.Health.Status == "Healthy"
	appStatus := fmt.Sprintf("%s / %s", app.Status.Sync.Status, app.Status.Health.Status)

	var appMessage string
	if app.Status.Health.Message != "" {
		appMessage = app.Status.Health.Message
	}
	if app.Status.OperationState != nil && app.Status.OperationState.Message != "" {
		appMessage = app.Status.OperationState.Message
	}

	appLink := ChainLink{
		Kind:         "Application",
		Name:         app.Metadata.Name,
		Namespace:    app.Metadata.Namespace,
		Ready:        appReady,
		Status:       appStatus,
		StatusReason: app.Status.Health.Status,
		Revision:     app.Status.Sync.Revision,
		Message:      appMessage,
	}
	result.Chain = append(result.Chain, appLink)

	if !appReady {
		result.FullyManaged = false
	}

	// Add managed resources as children
	for _, res := range app.Status.Resources {
		resReady := res.Status == "Synced"
		if res.Health != nil {
			resReady = resReady && res.Health.Status == "Healthy"
		}

		resStatus := res.Status
		if res.Health != nil {
			resStatus = fmt.Sprintf("%s / %s", res.Status, res.Health.Status)
		}

		var resMessage string
		if res.Health != nil && res.Health.Message != "" {
			resMessage = res.Health.Message
		}

		resLink := ChainLink{
			Kind:      res.Kind,
			Name:      res.Name,
			Namespace: res.Namespace,
			Ready:     resReady,
			Status:    resStatus,
			Message:   resMessage,
		}
		result.Chain = append(result.Chain, resLink)

		if !resReady {
			result.FullyManaged = false
		}
	}

	return result, nil
}

// extractRepoName extracts a readable name from a git URL
func extractRepoName(url string) string {
	// Handle git@github.com:org/repo.git
	if strings.HasPrefix(url, "git@") {
		parts := strings.Split(url, ":")
		if len(parts) == 2 {
			return strings.TrimSuffix(parts[1], ".git")
		}
	}

	// Handle https://github.com/org/repo.git
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}

	return url
}

// TraceByOwnership traces a resource by its Argo ownership labels
func (a *ArgoTracer) TraceByOwnership(ctx context.Context, ownership Ownership) (*TraceResult, error) {
	if ownership.Type != OwnerArgo {
		return nil, fmt.Errorf("resource not owned by Argo CD")
	}

	// The ownership.Name is the Application name
	return a.TraceApplication(ctx, ownership.Name)
}
