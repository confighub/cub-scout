// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// ArgoTracer implements Tracer for Argo CD
type ArgoTracer struct {
	// argocdPath is the path to the argocd CLI (default: "argocd")
	argocdPath string
	// kubectlPath is the path to kubectl (default: "kubectl")
	kubectlPath string
}

// NewArgoTracer creates a new Argo CD tracer
func NewArgoTracer() *ArgoTracer {
	return &ArgoTracer{
		argocdPath:  "argocd",
		kubectlPath: "kubectl",
	}
}

// NewArgoTracerWithPath creates an Argo tracer with a custom CLI path
func NewArgoTracerWithPath(path string) *ArgoTracer {
	return &ArgoTracer{
		argocdPath:  path,
		kubectlPath: "kubectl",
	}
}

// NewArgoTracerWithPaths creates an Argo tracer with custom CLI paths.
func NewArgoTracerWithPaths(argocdPath, kubectlPath string) *ArgoTracer {
	if strings.TrimSpace(argocdPath) == "" {
		argocdPath = "argocd"
	}
	if strings.TrimSpace(kubectlPath) == "" {
		kubectlPath = "kubectl"
	}
	return &ArgoTracer{
		argocdPath:  argocdPath,
		kubectlPath: kubectlPath,
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

		// Detect stale/invalid context and print actionable remediation path.
		if help, ok := FormatArgoContextError(combinedOutput); ok {
			// Graceful degradation: fallback to kubectl-based Application CR trace.
			fallbackResult, fallbackErr := a.traceApplicationViaKubectl(ctx, appName, namespace)
			if fallbackErr == nil {
				fallbackResult.Error = "ArgoCD CLI unavailable - showing kubectl-based Application trace only. Run 'argocd login <server>' for full CLI-backed trace context."
				return fallbackResult, nil
			}
			return nil, fmt.Errorf("%s\n\nkubectl fallback failed: %v", help, fallbackErr)
		}

		return nil, fmt.Errorf("argocd app get failed: %w: %s", err, combinedOutput)
	}

	// Parse the JSON output
	return a.parseAppOutput(stdout.Bytes(), appName, namespace)
}

// traceApplicationViaKubectl degrades to Application CR inspection when the
// argocd CLI context is stale/unavailable.
func (a *ArgoTracer) traceApplicationViaKubectl(ctx context.Context, appName, namespace string) (*TraceResult, error) {
	args := []string{"get", "applications.argoproj.io", "--all-namespaces", "-o", "json"}
	cmd := exec.CommandContext(ctx, a.kubectlPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("kubectl get applications failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var appList struct {
		Items []argoApp `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &appList); err != nil {
		return nil, fmt.Errorf("parse kubectl applications output: %w", err)
	}

	matches := make([]argoApp, 0, len(appList.Items))
	for _, app := range appList.Items {
		if strings.TrimSpace(app.Metadata.Name) != appName {
			continue
		}
		if namespace != "" && strings.TrimSpace(app.Metadata.Namespace) != namespace {
			continue
		}
		matches = append(matches, app)
	}
	if len(matches) == 0 {
		if namespace != "" {
			return nil, fmt.Errorf("application %q not found via kubectl in namespace %q", appName, namespace)
		}
		return nil, fmt.Errorf("application %q not found via kubectl", appName)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Metadata.Namespace == matches[j].Metadata.Namespace {
			return matches[i].Metadata.Name < matches[j].Metadata.Name
		}
		return matches[i].Metadata.Namespace < matches[j].Metadata.Namespace
	})

	selected := matches[0]
	if namespace == "" {
		for _, candidate := range matches {
			if strings.TrimSpace(candidate.Metadata.Namespace) == "argocd" {
				selected = candidate
				break
			}
		}
	}

	selectedData, err := json.Marshal(selected)
	if err != nil {
		return nil, fmt.Errorf("marshal kubectl-selected application: %w", err)
	}

	return a.parseAppOutput(selectedData, selected.Metadata.Name, selected.Metadata.Namespace)
}

// argoOwnerRef mirrors metav1.OwnerReference for JSON parsing.
type argoOwnerRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Controller *bool  `json:"controller,omitempty"`
}

// argoSource is one entry in spec.source (singular) or spec.sources (plural).
// Argo CD ≥ v2.6 supports multi-source Applications via spec.sources[]; older
// apps use spec.source. Both shapes are parsed into argoSource so the rest of
// the tracer is shape-agnostic.
type argoSource struct {
	RepoURL        string `json:"repoURL"`
	Path           string `json:"path"`
	TargetRevision string `json:"targetRevision"`
	Chart          string `json:"chart"`
}

// argoApp represents the structure of argocd app get output
type argoApp struct {
	Metadata struct {
		Name            string            `json:"name"`
		Namespace       string            `json:"namespace"`
		Labels          map[string]string `json:"labels,omitempty"`
		Annotations     map[string]string `json:"annotations,omitempty"`
		OwnerReferences []argoOwnerRef    `json:"ownerReferences,omitempty"`
	} `json:"metadata"`
	Spec struct {
		Source  argoSource   `json:"source"`
		Sources []argoSource `json:"sources,omitempty"`
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
		ReconciledAt string `json:"reconciledAt,omitempty"`
		Health       struct {
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

	// Argo CD ≥ v2.6 supports spec.sources[] (plural). For single-source apps
	// expressed as a one-element array, copy Sources[0] into Source so the rest
	// of the parser is shape-agnostic. For multi-source apps (len > 1) we still
	// pick the first source for the chain link, but flag the result so #409's
	// equality logic emits an explicit "controller.multi_source" proof gap —
	// cub-scout cannot honestly assert equality across sources it didn't parse.
	if app.Spec.Source.RepoURL == "" && len(app.Spec.Sources) > 0 {
		app.Spec.Source = app.Spec.Sources[0]
	}
	multiSource := len(app.Spec.Sources) > 1

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
		MultiSource:  multiSource,
	}

	sourceSync := strings.TrimSpace(app.Status.Sync.Status)
	if sourceSync == "" {
		sourceSync = "Unknown"
	}
	sourceReady := strings.EqualFold(app.Status.Sync.Status, "Synced")
	sourceStatus := sourceSync

	var sourceLastTransition *time.Time
	sourceSignal := []string{}
	if !sourceReady && strings.TrimSpace(app.Status.Sync.Status) != "" {
		sourceSignal = append(sourceSignal, fmt.Sprintf("application sync status is %s", app.Status.Sync.Status))
	}
	if reconciled, ok := parseArgoStatusTimestamp(app.Status.ReconciledAt); ok {
		sourceLastTransition = &reconciled
		sourceStatus = fmt.Sprintf("%s (reconciledAt: %s)", sourceStatus, app.Status.ReconciledAt)
		sourceSignal = append(sourceSignal, fmt.Sprintf("reconciledAt=%s", app.Status.ReconciledAt))
	} else if len(app.Status.History) > 0 && !app.Status.History[len(app.Status.History)-1].DeployedAt.IsZero() {
		deployedAt := app.Status.History[len(app.Status.History)-1].DeployedAt.UTC()
		sourceLastTransition = &deployedAt
		sourceSignal = append(sourceSignal, fmt.Sprintf("history.deployedAt=%s", deployedAt.Format(time.RFC3339)))
	}

	// Add source as first chain link (simulating a GitRepository)
	sourceLink := ChainLink{
		Kind:               "Source",
		Name:               extractRepoName(app.Spec.Source.RepoURL),
		URL:                app.Spec.Source.RepoURL,
		Path:               app.Spec.Source.Path,
		Revision:           app.Spec.Source.TargetRevision,
		Ready:              sourceReady,
		Status:             sourceStatus,
		StatusReason:       sourceSync,
		Message:            strings.Join(sourceSignal, "; "),
		LastTransitionTime: sourceLastTransition,
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

	// Extract deployment history
	if len(app.Status.History) > 0 {
		result.History = make([]HistoryEntry, 0, len(app.Status.History))
		for _, h := range app.Status.History {
			entry := HistoryEntry{
				Timestamp: h.DeployedAt,
				Revision:  h.Revision,
				Status:    "deployed",
			}
			result.History = append(result.History, entry)
		}
	}

	// Detect App-of-Apps parent lineage (#194)
	parent, parentConf := resolveParentFromArgoApp(&app)
	if parent != "" {
		result.ParentApplication = parent
		result.LineageConfidence = parentConf
	}

	// Detect ApplicationSet generator lineage (#195)
	appSet, appSetConf := resolveApplicationSetFromArgoApp(&app)
	if appSet != "" {
		result.GeneratedByApplicationSet = appSet
		// Use the more explicit confidence if both are set.
		if result.LineageConfidence == "" || appSetConf == "explicit" {
			result.LineageConfidence = appSetConf
		}
	}

	return result, nil
}

// resolveParentFromArgoApp detects an App-of-Apps parent Application from
// the parsed argocd app get output. Returns (parentName, confidence).
// Confidence: "explicit" for ownerReference, "inferred" for label/annotation.
func resolveParentFromArgoApp(app *argoApp) (string, string) {
	self := strings.TrimSpace(app.Metadata.Name)

	// 1. ownerReferences with Kind=Application (explicit)
	for _, ref := range app.Metadata.OwnerReferences {
		if strings.EqualFold(ref.Kind, "Application") {
			name := strings.TrimSpace(ref.Name)
			if name != "" && name != self {
				return name, "explicit"
			}
		}
	}

	// 2. cub-scout.io/parent-application annotation (inferred)
	if name := strings.TrimSpace(app.Metadata.Annotations["cub-scout.io/parent-application"]); name != "" && name != self {
		return name, "inferred"
	}

	// 3. argocd.argoproj.io/managed-by annotation (inferred)
	if name := strings.TrimSpace(app.Metadata.Annotations["argocd.argoproj.io/managed-by"]); name != "" && name != self {
		return name, "inferred"
	}

	// 4. app.kubernetes.io/part-of label (inferred)
	if name := strings.TrimSpace(app.Metadata.Labels["app.kubernetes.io/part-of"]); name != "" && name != self {
		return name, "inferred"
	}

	return "", ""
}

// resolveApplicationSetFromArgoApp detects an ApplicationSet generator from
// the parsed argocd app get output. Returns (appSetName, confidence).
func resolveApplicationSetFromArgoApp(app *argoApp) (string, string) {
	// 1. ownerReferences with Kind=ApplicationSet (explicit)
	for _, ref := range app.Metadata.OwnerReferences {
		if strings.EqualFold(ref.Kind, "ApplicationSet") {
			name := strings.TrimSpace(ref.Name)
			if name != "" {
				return name, "explicit"
			}
		}
	}

	// 2. argocd.argoproj.io/application-set-name label (inferred)
	if name := strings.TrimSpace(app.Metadata.Labels["argocd.argoproj.io/application-set-name"]); name != "" {
		return name, "inferred"
	}

	// 3. argocd.argoproj.io/application-set-name annotation (inferred)
	if name := strings.TrimSpace(app.Metadata.Annotations["argocd.argoproj.io/application-set-name"]); name != "" {
		return name, "inferred"
	}

	// 4. cub-scout.io/generated-by-applicationset annotation (inferred)
	if name := strings.TrimSpace(app.Metadata.Annotations["cub-scout.io/generated-by-applicationset"]); name != "" {
		return name, "inferred"
	}

	return "", ""
}

func parseArgoStatusTimestamp(raw string) (time.Time, bool) {
	ts := strings.TrimSpace(raw)
	if ts == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t, true
	}
	return time.Time{}, false
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

// FormatArgoContextError detects stale/invalid argocd context errors and
// returns a remediation-focused message. The bool return is true when a
// context issue was detected.
func FormatArgoContextError(output string) (string, bool) {
	out := strings.ToLower(output)

	containsAny := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(out, p) {
				return true
			}
		}
		return false
	}

	reason := ""
	switch {
	case containsAny(
		"server address unspecified",
		"not logged in",
		"authentication required",
		"unauthorized",
		"permission denied",
		"token is expired",
	):
		reason = "authentication/context is missing or expired"
	case containsAny(
		"context deadline exceeded",
		"connection refused",
		"i/o timeout",
		"dial tcp",
		"no such host",
		"x509:",
		"certificate signed by unknown authority",
		"tls:",
		"rpc error: code = unavailable",
		"transport is closing",
	):
		reason = "current Argo endpoint is unreachable or stale"
	default:
		return "", false
	}

	msg := fmt.Sprintf(
		"argocd context appears stale or invalid (%s).\n\n"+
			"Trace context troubleshooting:\n"+
			"  1) argocd context\n"+
			"  2) argocd app list\n"+
			"  3) argocd logout <server>\n"+
			"  4) argocd login <server>\n\n"+
			"Then retry:\n"+
			"  cub-scout trace --app <app-name>",
		reason,
	)
	return msg, true
}
