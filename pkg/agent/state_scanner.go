// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// StateScanner scans for stuck reconciliation states
type StateScanner struct {
	client dynamic.Interface
}

// StuckThreshold is the default duration after which a resource is considered stuck
const StuckThreshold = 5 * time.Minute

// StuckFinding represents a stuck resource finding
type StuckFinding struct {
	CCVEID      string `json:"ccveId"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Condition   string `json:"condition"`
	Reason      string `json:"reason"`
	Message     string `json:"message"`
	Duration    string `json:"duration"`
	Remediation string `json:"remediation"`
	Command     string `json:"command,omitempty"`
}

// StateScanResult contains findings from state scanning
type StateScanResult struct {
	ScannedAt time.Time        `json:"scannedAt"`
	Findings  []StuckFinding   `json:"findings"`
	Summary   StateScanSummary `json:"summary"`
}

// StateScanSummary counts findings by type
type StateScanSummary struct {
	HelmReleaseStuck   int `json:"helmReleaseStuck"`
	KustomizationStuck int `json:"kustomizationStuck"`
	ApplicationStuck   int `json:"applicationStuck"`
	SilentFailures     int `json:"silentFailures"`
	Total              int `json:"total"`
}

// NewStateScanner creates a new state scanner
func NewStateScanner(config *rest.Config) (*StateScanner, error) {
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	return &StateScanner{client: client}, nil
}

// NewStateScannerWithClient creates a scanner with an existing client
func NewStateScannerWithClient(client dynamic.Interface) *StateScanner {
	return &StateScanner{client: client}
}

// Scan performs a full state scan
func (s *StateScanner) Scan(ctx context.Context) (*StateScanResult, error) {
	return s.ScanWithThreshold(ctx, StuckThreshold)
}

// ScanWithThreshold performs a state scan with a custom threshold
func (s *StateScanner) ScanWithThreshold(ctx context.Context, threshold time.Duration) (*StateScanResult, error) {
	result := &StateScanResult{
		ScannedAt: time.Now(),
		Findings:  []StuckFinding{},
	}

	// Scan Flux HelmReleases
	helmFindings := s.scanHelmReleases(ctx, threshold)
	result.Findings = append(result.Findings, helmFindings...)
	result.Summary.HelmReleaseStuck = len(helmFindings)

	// Scan Flux Kustomizations
	kustomizeFindings := s.scanKustomizations(ctx, threshold)
	result.Findings = append(result.Findings, kustomizeFindings...)
	result.Summary.KustomizationStuck = len(kustomizeFindings)

	// Scan Argo CD Applications
	argoFindings := s.scanApplications(ctx, threshold)
	result.Findings = append(result.Findings, argoFindings...)
	result.Summary.ApplicationStuck = len(argoFindings)

	// Scan for silent failures (Ready=True but misconfigured)
	silentFindings := s.scanSilentFailures(ctx)
	result.Findings = append(result.Findings, silentFindings...)
	result.Summary.SilentFailures = len(silentFindings)

	result.Summary.Total = len(result.Findings)
	return result, nil
}

// ScanNamespace scans a specific namespace
func (s *StateScanner) ScanNamespace(ctx context.Context, namespace string) (*StateScanResult, error) {
	return s.ScanNamespaceWithThreshold(ctx, namespace, StuckThreshold)
}

// ScanNamespaceWithThreshold scans a specific namespace with custom threshold
func (s *StateScanner) ScanNamespaceWithThreshold(ctx context.Context, namespace string, threshold time.Duration) (*StateScanResult, error) {
	result := &StateScanResult{
		ScannedAt: time.Now(),
		Findings:  []StuckFinding{},
	}

	// Scan Flux HelmReleases in namespace
	helmFindings := s.scanHelmReleasesNamespace(ctx, namespace, threshold)
	result.Findings = append(result.Findings, helmFindings...)
	result.Summary.HelmReleaseStuck = len(helmFindings)

	// Scan Flux Kustomizations in namespace
	kustomizeFindings := s.scanKustomizationsNamespace(ctx, namespace, threshold)
	result.Findings = append(result.Findings, kustomizeFindings...)
	result.Summary.KustomizationStuck = len(kustomizeFindings)

	// Scan Argo CD Applications in namespace
	argoFindings := s.scanApplicationsNamespace(ctx, namespace, threshold)
	result.Findings = append(result.Findings, argoFindings...)
	result.Summary.ApplicationStuck = len(argoFindings)

	// Scan for silent failures in namespace
	silentFindings := s.scanSilentFailuresNamespace(ctx, namespace)
	result.Findings = append(result.Findings, silentFindings...)
	result.Summary.SilentFailures = len(silentFindings)

	result.Summary.Total = len(result.Findings)
	return result, nil
}

// scanSilentFailuresNamespace scans for silent failures in a specific namespace
func (s *StateScanner) scanSilentFailuresNamespace(ctx context.Context, namespace string) []StuckFinding {
	var findings []StuckFinding

	helmFindings := s.scanHelmReleaseSilentFailuresNamespace(ctx, namespace)
	findings = append(findings, helmFindings...)

	kustomizeFindings := s.scanKustomizationSilentFailuresNamespace(ctx, namespace)
	findings = append(findings, kustomizeFindings...)

	return findings
}

// scanHelmReleaseSilentFailuresNamespace checks HelmReleases in a namespace for silent misconfigurations
func (s *StateScanner) scanHelmReleaseSilentFailuresNamespace(ctx context.Context, namespace string) []StuckFinding {
	var findings []StuckFinding

	gvr := schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}

	list, err := s.client.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	for _, item := range list.Items {
		name := item.GetName()
		ns := item.GetNamespace()

		if !s.isReadyOrUnknown(item) {
			continue
		}

		// Check for interval: 0
		interval, found, _ := unstructured.NestedString(item.Object, "spec", "interval")
		if found && (interval == "0" || interval == "0s" || interval == "0m") {
			findings = append(findings, StuckFinding{
				CCVEID:      "CCVE-2025-0665",
				Category:    "SILENT",
				Severity:    "warning",
				Kind:        "HelmRelease",
				Name:        name,
				Namespace:   ns,
				Condition:   "interval=0",
				Reason:      "ReconciliationDisabled",
				Message:     "interval: 0 disables continuous reconciliation",
				Remediation: "Set a non-zero interval (e.g., spec.interval: 5m)",
				Command:     fmt.Sprintf("kubectl patch helmrelease %s -n %s --type=merge -p '{\"spec\":{\"interval\":\"5m\"}}'", name, ns),
			})
		}

		// Check for valuesFrom optional missing
		valuesFrom, found, _ := unstructured.NestedSlice(item.Object, "spec", "valuesFrom")
		if found {
			for _, vf := range valuesFrom {
				vfMap, ok := vf.(map[string]interface{})
				if !ok {
					continue
				}
				optional, _ := vfMap["optional"].(bool)
				if !optional {
					continue
				}
				kind, _ := vfMap["kind"].(string)
				refName, _ := vfMap["name"].(string)
				if kind != "" && refName != "" && !s.checkResourceExists(ctx, ns, kind, refName) {
					findings = append(findings, StuckFinding{
						CCVEID:      "CCVE-2025-0662",
						Category:    "SILENT",
						Severity:    "critical",
						Kind:        "HelmRelease",
						Name:        name,
						Namespace:   ns,
						Condition:   fmt.Sprintf("valuesFrom.%s=%s missing", kind, refName),
						Reason:      "OptionalSourceMissing",
						Message:     fmt.Sprintf("valuesFrom references missing %s/%s with optional:true", kind, refName),
						Remediation: fmt.Sprintf("Create the %s '%s' or remove optional:true", kind, refName),
						Command:     fmt.Sprintf("kubectl get %s %s -n %s", strings.ToLower(kind), refName, ns),
					})
				}
			}
		}
	}

	return findings
}

// scanKustomizationSilentFailuresNamespace checks Kustomizations in a namespace for silent misconfigurations
func (s *StateScanner) scanKustomizationSilentFailuresNamespace(ctx context.Context, namespace string) []StuckFinding {
	var findings []StuckFinding

	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	list, err := s.client.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	for _, item := range list.Items {
		name := item.GetName()
		ns := item.GetNamespace()

		if !s.isReadyOrUnknown(item) {
			continue
		}

		// Check for interval: 0
		interval, found, _ := unstructured.NestedString(item.Object, "spec", "interval")
		if found && (interval == "0" || interval == "0s" || interval == "0m") {
			findings = append(findings, StuckFinding{
				CCVEID:      "CCVE-2025-0665",
				Category:    "SILENT",
				Severity:    "warning",
				Kind:        "Kustomization",
				Name:        name,
				Namespace:   ns,
				Condition:   "interval=0",
				Reason:      "ReconciliationDisabled",
				Message:     "interval: 0 disables continuous reconciliation",
				Remediation: "Set a non-zero interval (e.g., spec.interval: 5m)",
				Command:     fmt.Sprintf("kubectl patch kustomization %s -n %s --type=merge -p '{\"spec\":{\"interval\":\"5m\"}}'", name, ns),
			})
		}

		// Check for suspended source
		sourceKind, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "kind")
		sourceName, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "name")
		sourceNS, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "namespace")
		if sourceNS == "" {
			sourceNS = ns
		}
		if sourceKind == "GitRepository" && sourceName != "" && s.isSourceSuspended(ctx, sourceNS, "gitrepositories", sourceName) {
			findings = append(findings, StuckFinding{
				CCVEID:      "CCVE-2025-0666",
				Category:    "SILENT",
				Severity:    "critical",
				Kind:        "Kustomization",
				Name:        name,
				Namespace:   ns,
				Condition:   fmt.Sprintf("sourceRef.GitRepository=%s suspended", sourceName),
				Reason:      "SourceSuspended",
				Message:     "sourceRef points to suspended GitRepository; using stale revision",
				Remediation: "Resume the GitRepository or point to an active source",
				Command:     fmt.Sprintf("flux resume source git %s -n %s", sourceName, sourceNS),
			})
		}
	}

	return findings
}

// scanHelmReleases scans all HelmReleases for stuck states
func (s *StateScanner) scanHelmReleases(ctx context.Context, threshold time.Duration) []StuckFinding {
	gvr := schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		// HelmRelease CRD not installed, skip
		return nil
	}

	return s.checkHelmReleases(list.Items, threshold)
}

// scanHelmReleasesNamespace scans HelmReleases in a specific namespace
func (s *StateScanner) scanHelmReleasesNamespace(ctx context.Context, namespace string, threshold time.Duration) []StuckFinding {
	gvr := schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}

	list, err := s.client.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	return s.checkHelmReleases(list.Items, threshold)
}

// checkHelmReleases evaluates HelmReleases for stuck conditions
func (s *StateScanner) checkHelmReleases(items []unstructured.Unstructured, threshold time.Duration) []StuckFinding {
	var findings []StuckFinding

	for _, item := range items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Check if suspended (skip)
		if suspended, found, _ := unstructured.NestedBool(item.Object, "spec", "suspend"); found && suspended {
			continue
		}

		// Get conditions
		conditions, found, err := unstructured.NestedSlice(item.Object, "status", "conditions")
		if err != nil || !found {
			continue
		}

		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}

			condType, _ := condMap["type"].(string)
			status, _ := condMap["status"].(string)
			reason, _ := condMap["reason"].(string)
			message, _ := condMap["message"].(string)
			lastTransition, _ := condMap["lastTransitionTime"].(string)

			// Check for Ready=False or Stalled=True for extended period
			if (condType == "Ready" && status == "False") || (condType == "Stalled" && status == "True") {
				transitionTime, err := time.Parse(time.RFC3339, lastTransition)
				if err != nil {
					continue
				}

				duration := time.Since(transitionTime)
				if duration > threshold {
					finding := StuckFinding{
						CCVEID:      "CCVE-2025-0166", // HelmRelease stuck
						Category:    "STATE",
						Severity:    s.determineSeverity(duration),
						Kind:        "HelmRelease",
						Name:        name,
						Namespace:   namespace,
						Condition:   fmt.Sprintf("%s=%s", condType, status),
						Reason:      reason,
						Message:     truncateMessage(message, 100),
						Duration:    formatDuration(duration),
						Remediation: s.getHelmReleaseRemediation(reason),
						Command:     s.getHelmReleaseCommand(namespace, name, reason),
					}
					findings = append(findings, finding)
					break // Only report once per HelmRelease
				}
			}
		}
	}

	return findings
}

// scanKustomizations scans all Kustomizations for stuck states
func (s *StateScanner) scanKustomizations(ctx context.Context, threshold time.Duration) []StuckFinding {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	return s.checkKustomizations(list.Items, threshold)
}

// scanKustomizationsNamespace scans Kustomizations in a specific namespace
func (s *StateScanner) scanKustomizationsNamespace(ctx context.Context, namespace string, threshold time.Duration) []StuckFinding {
	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	list, err := s.client.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	return s.checkKustomizations(list.Items, threshold)
}

// checkKustomizations evaluates Kustomizations for stuck conditions
func (s *StateScanner) checkKustomizations(items []unstructured.Unstructured, threshold time.Duration) []StuckFinding {
	var findings []StuckFinding

	for _, item := range items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Check if suspended
		if suspended, found, _ := unstructured.NestedBool(item.Object, "spec", "suspend"); found && suspended {
			continue
		}

		conditions, found, err := unstructured.NestedSlice(item.Object, "status", "conditions")
		if err != nil || !found {
			continue
		}

		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}

			condType, _ := condMap["type"].(string)
			status, _ := condMap["status"].(string)
			reason, _ := condMap["reason"].(string)
			message, _ := condMap["message"].(string)
			lastTransition, _ := condMap["lastTransitionTime"].(string)

			if (condType == "Ready" && status == "False") || (condType == "Stalled" && status == "True") {
				transitionTime, err := time.Parse(time.RFC3339, lastTransition)
				if err != nil {
					continue
				}

				duration := time.Since(transitionTime)
				if duration > threshold {
					finding := StuckFinding{
						CCVEID:      "CCVE-2025-0012", // Kustomization stuck
						Category:    "STATE",
						Severity:    s.determineSeverity(duration),
						Kind:        "Kustomization",
						Name:        name,
						Namespace:   namespace,
						Condition:   fmt.Sprintf("%s=%s", condType, status),
						Reason:      reason,
						Message:     truncateMessage(message, 100),
						Duration:    formatDuration(duration),
						Remediation: s.getKustomizationRemediation(reason),
						Command:     s.getKustomizationCommand(namespace, name, reason),
					}
					findings = append(findings, finding)
					break
				}
			}
		}
	}

	return findings
}

// scanApplications scans all Argo CD Applications for stuck states
func (s *StateScanner) scanApplications(ctx context.Context, threshold time.Duration) []StuckFinding {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	return s.checkApplications(list.Items, threshold)
}

// scanApplicationsNamespace scans Applications in a specific namespace
func (s *StateScanner) scanApplicationsNamespace(ctx context.Context, namespace string, threshold time.Duration) []StuckFinding {
	gvr := schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}

	list, err := s.client.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	return s.checkApplications(list.Items, threshold)
}

// checkApplications evaluates Argo CD Applications for stuck conditions
func (s *StateScanner) checkApplications(items []unstructured.Unstructured, threshold time.Duration) []StuckFinding {
	var findings []StuckFinding

	for _, item := range items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Check sync status
		syncStatus, _, _ := unstructured.NestedString(item.Object, "status", "sync", "status")
		healthStatus, _, _ := unstructured.NestedString(item.Object, "status", "health", "status")

		// Check operation state for stuck syncs
		phase, _, _ := unstructured.NestedString(item.Object, "status", "operationState", "phase")
		startedAt, _, _ := unstructured.NestedString(item.Object, "status", "operationState", "startedAt")
		message, _, _ := unstructured.NestedString(item.Object, "status", "operationState", "message")

		// Check for stuck sync operation
		if phase == "Running" || phase == "Error" || phase == "Failed" {
			if startedAt != "" {
				startTime, err := time.Parse(time.RFC3339, startedAt)
				if err == nil {
					duration := time.Since(startTime)
					if duration > threshold {
						finding := StuckFinding{
							CCVEID:      "CCVE-2025-0169", // Application stuck syncing
							Category:    "STATE",
							Severity:    s.determineSeverity(duration),
							Kind:        "Application",
							Name:        name,
							Namespace:   namespace,
							Condition:   fmt.Sprintf("operationState.phase=%s", phase),
							Reason:      phase,
							Message:     truncateMessage(message, 100),
							Duration:    formatDuration(duration),
							Remediation: s.getApplicationRemediation(phase),
							Command:     s.getApplicationCommand(namespace, name, phase),
						}
						findings = append(findings, finding)
						continue
					}
				}
			}
		}

		// Check for unhealthy status persisting
		if healthStatus == "Degraded" || healthStatus == "Missing" || syncStatus == "OutOfSync" {
			// Use reconciledAt or operationState.finishedAt for duration
			reconciledAt, _, _ := unstructured.NestedString(item.Object, "status", "reconciledAt")
			if reconciledAt != "" {
				reconciledTime, err := time.Parse(time.RFC3339, reconciledAt)
				if err == nil {
					duration := time.Since(reconciledTime)
					if duration > threshold {
						finding := StuckFinding{
							CCVEID:      "CCVE-2025-0169",
							Category:    "STATE",
							Severity:    s.determineSeverity(duration),
							Kind:        "Application",
							Name:        name,
							Namespace:   namespace,
							Condition:   fmt.Sprintf("health=%s, sync=%s", healthStatus, syncStatus),
							Reason:      healthStatus,
							Message:     fmt.Sprintf("Application unhealthy or out of sync for %s", formatDuration(duration)),
							Duration:    formatDuration(duration),
							Remediation: s.getApplicationRemediation(healthStatus),
							Command:     s.getApplicationCommand(namespace, name, healthStatus),
						}
						findings = append(findings, finding)
					}
				}
			}
		}
	}

	return findings
}

// determineSeverity determines severity based on duration
func (s *StateScanner) determineSeverity(duration time.Duration) string {
	if duration > 1*time.Hour {
		return "critical"
	}
	if duration > 15*time.Minute {
		return "warning"
	}
	return "info"
}

// getHelmReleaseRemediation returns remediation steps for HelmRelease issues
func (s *StateScanner) getHelmReleaseRemediation(reason string) string {
	switch reason {
	case "UpgradeFailed":
		return "Check Helm release history; rollback if needed; verify chart values"
	case "InstallFailed":
		return "Check Helm template output for errors; verify prerequisites"
	case "ArtifactFailed":
		return "Verify HelmRepository or HelmChart source is accessible"
	case "DependencyNotReady":
		return "Ensure source GitRepository/HelmRepository is ready"
	case "ReconciliationFailed":
		return "Check controller logs; verify RBAC permissions"
	default:
		return "Check flux logs; force reconcile with flux reconcile"
	}
}

// getHelmReleaseCommand returns remediation command
func (s *StateScanner) getHelmReleaseCommand(namespace, name, reason string) string {
	switch reason {
	case "UpgradeFailed":
		return fmt.Sprintf("flux suspend hr %s -n %s && flux resume hr %s -n %s", name, namespace, name, namespace)
	default:
		return fmt.Sprintf("flux reconcile hr %s -n %s --with-source", name, namespace)
	}
}

// getKustomizationRemediation returns remediation steps for Kustomization issues
func (s *StateScanner) getKustomizationRemediation(reason string) string {
	switch reason {
	case "BuildFailed":
		return "Check kustomization.yaml syntax; verify paths exist in source"
	case "HealthCheckFailed":
		return "Resources deployed but not healthy; check pod logs"
	case "ArtifactFailed":
		return "GitRepository source not ready; check source status"
	case "DependencyNotReady":
		return "Dependent Kustomization not ready; check dependency chain"
	case "ReconciliationFailed":
		return "Check controller logs; verify RBAC permissions"
	case "PruneFailed":
		return "Prune failed; check for finalizers or admission webhooks"
	default:
		return "Check flux logs; force reconcile with flux reconcile"
	}
}

// getKustomizationCommand returns remediation command
func (s *StateScanner) getKustomizationCommand(namespace, name, reason string) string {
	switch reason {
	case "PruneFailed":
		return fmt.Sprintf("flux reconcile ks %s -n %s --prune=false", name, namespace)
	default:
		return fmt.Sprintf("flux reconcile ks %s -n %s --with-source", name, namespace)
	}
}

// getApplicationRemediation returns remediation steps for Application issues
func (s *StateScanner) getApplicationRemediation(reason string) string {
	switch reason {
	case "Running":
		return "Sync in progress for too long; check application-controller logs"
	case "Error", "Failed":
		return "Sync failed; check sync errors in UI or argocd app get"
	case "Degraded":
		return "Resources unhealthy; check pod status and logs"
	case "Missing":
		return "Expected resources not found; verify manifests generate resources"
	case "OutOfSync":
		return "Drift detected; review diff and sync or accept drift"
	default:
		return "Check argocd app get for details; force sync if needed"
	}
}

// getApplicationCommand returns remediation command
func (s *StateScanner) getApplicationCommand(namespace, name, reason string) string {
	switch reason {
	case "Running":
		return fmt.Sprintf("argocd app terminate-op %s", name)
	case "Error", "Failed":
		return fmt.Sprintf("argocd app sync %s --retry-limit 3", name)
	case "OutOfSync":
		return fmt.Sprintf("argocd app diff %s && argocd app sync %s", name, name)
	default:
		return fmt.Sprintf("argocd app sync %s --force", name)
	}
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh%dm", hours, minutes)
}

// truncateMessage truncates a message to the specified length
func truncateMessage(msg string, maxLen int) string {
	msg = strings.ReplaceAll(msg, "\n", " ")
	if len(msg) <= maxLen {
		return msg
	}
	return msg[:maxLen-3] + "..."
}

// scanSilentFailures scans for resources that are Ready=True but misconfigured
func (s *StateScanner) scanSilentFailures(ctx context.Context) []StuckFinding {
	var findings []StuckFinding

	// Scan HelmReleases for silent failures
	helmFindings := s.scanHelmReleaseSilentFailures(ctx)
	findings = append(findings, helmFindings...)

	// Scan Kustomizations for silent failures
	kustomizeFindings := s.scanKustomizationSilentFailures(ctx)
	findings = append(findings, kustomizeFindings...)

	return findings
}

// scanHelmReleaseSilentFailures checks HelmReleases for silent misconfigurations
func (s *StateScanner) scanHelmReleaseSilentFailures(ctx context.Context) []StuckFinding {
	var findings []StuckFinding

	gvr := schema.GroupVersionResource{
		Group:    "helm.toolkit.fluxcd.io",
		Version:  "v2",
		Resource: "helmreleases",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Only check resources that appear healthy (Ready=True or Unknown)
		if !s.isReadyOrUnknown(item) {
			continue
		}

		// Check for interval: 0 (CCVE-2025-0665)
		interval, found, _ := unstructured.NestedString(item.Object, "spec", "interval")
		if found && (interval == "0" || interval == "0s" || interval == "0m") {
			findings = append(findings, StuckFinding{
				CCVEID:      "CCVE-2025-0665",
				Category:    "SILENT",
				Severity:    "warning",
				Kind:        "HelmRelease",
				Name:        name,
				Namespace:   namespace,
				Condition:   "interval=0",
				Reason:      "ReconciliationDisabled",
				Message:     "interval: 0 disables continuous reconciliation; drift will not be corrected",
				Remediation: "Set a non-zero interval (e.g., spec.interval: 5m) to enable reconciliation",
				Command:     fmt.Sprintf("kubectl patch helmrelease %s -n %s --type=merge -p '{\"spec\":{\"interval\":\"5m\"}}'", name, namespace),
			})
		}

		// Check for valuesFrom with optional: true pointing to missing ConfigMaps (CCVE-2025-0662)
		valuesFrom, found, _ := unstructured.NestedSlice(item.Object, "spec", "valuesFrom")
		if found {
			for _, vf := range valuesFrom {
				vfMap, ok := vf.(map[string]interface{})
				if !ok {
					continue
				}

				optional, _ := vfMap["optional"].(bool)
				if !optional {
					continue
				}

				kind, _ := vfMap["kind"].(string)
				refName, _ := vfMap["name"].(string)

				if kind == "" || refName == "" {
					continue
				}

				// Check if the referenced ConfigMap/Secret exists
				exists := s.checkResourceExists(ctx, namespace, kind, refName)
				if !exists {
					findings = append(findings, StuckFinding{
						CCVEID:      "CCVE-2025-0662",
						Category:    "SILENT",
						Severity:    "critical",
						Kind:        "HelmRelease",
						Name:        name,
						Namespace:   namespace,
						Condition:   fmt.Sprintf("valuesFrom.%s=%s missing", kind, refName),
						Reason:      "OptionalSourceMissing",
						Message:     fmt.Sprintf("valuesFrom references missing %s/%s with optional:true; using chart defaults", kind, refName),
						Remediation: fmt.Sprintf("Create the %s '%s' or remove optional:true to fail explicitly", kind, refName),
						Command:     fmt.Sprintf("kubectl get %s %s -n %s", strings.ToLower(kind), refName, namespace),
					})
				}
			}
		}

		// Check for version wildcard in chart spec (CCVE-2025-0671)
		chartVersion, _, _ := unstructured.NestedString(item.Object, "spec", "chart", "spec", "version")
		if chartVersion != "" && (strings.HasPrefix(chartVersion, ">=") || strings.HasPrefix(chartVersion, ">") ||
			strings.HasPrefix(chartVersion, "^") || strings.HasPrefix(chartVersion, "~") ||
			strings.Contains(chartVersion, "*") || strings.Contains(chartVersion, ".x")) {
			findings = append(findings, StuckFinding{
				CCVEID:      "CCVE-2025-0671",
				Category:    "SILENT",
				Severity:    "warning",
				Kind:        "HelmRelease",
				Name:        name,
				Namespace:   namespace,
				Condition:   fmt.Sprintf("chart.version=%s", chartVersion),
				Reason:      "VersionWildcard",
				Message:     "Chart version uses wildcard/range; may get unexpected upgrades",
				Remediation: "Pin to a specific version (e.g., 1.2.3) for predictable deployments",
				Command:     fmt.Sprintf("kubectl get helmrelease %s -n %s -o jsonpath='{.spec.chart.spec.version}'", name, namespace),
			})
		}

		// Check for short timeout (CCVE-2025-0672)
		timeout, found, _ := unstructured.NestedString(item.Object, "spec", "timeout")
		if found && timeout != "" {
			if s.isShortTimeout(timeout) {
				findings = append(findings, StuckFinding{
					CCVEID:      "CCVE-2025-0672",
					Category:    "SILENT",
					Severity:    "warning",
					Kind:        "HelmRelease",
					Name:        name,
					Namespace:   namespace,
					Condition:   fmt.Sprintf("timeout=%s", timeout),
					Reason:      "TimeoutTooShort",
					Message:     "Timeout may be too short for reliable deployments; may cause intermittent failures",
					Remediation: "Increase timeout to at least 5m for production deployments",
					Command:     fmt.Sprintf("kubectl patch helmrelease %s -n %s --type=merge -p '{\"spec\":{\"timeout\":\"5m\"}}'", name, namespace),
				})
			}
		}

		// Check for values + valuesFrom overlap (potential precedence confusion) (CCVE-2025-0670)
		hasInlineValues, _, _ := unstructured.NestedMap(item.Object, "spec", "values")
		hasValuesFrom, _, _ := unstructured.NestedSlice(item.Object, "spec", "valuesFrom")
		if hasInlineValues != nil && len(hasInlineValues) > 0 && hasValuesFrom != nil && len(hasValuesFrom) > 0 {
			findings = append(findings, StuckFinding{
				CCVEID:      "CCVE-2025-0670",
				Category:    "SILENT",
				Severity:    "info",
				Kind:        "HelmRelease",
				Name:        name,
				Namespace:   namespace,
				Condition:   "values+valuesFrom both set",
				Reason:      "ValuesPrecedenceRisk",
				Message:     "Both inline values and valuesFrom are set; inline values take precedence (may be unexpected)",
				Remediation: "Review merge order: chart defaults <- valuesFrom <- inline values (WINS)",
				Command:     fmt.Sprintf("helm get values %s -n %s", name, namespace),
			})
		}

		// Check for postRenderers with hardcoded names (CCVE-2025-0673)
		// This detects patches that target resource names that don't match the chart or release name
		postRenderers, found, _ := unstructured.NestedSlice(item.Object, "spec", "postRenderers")
		if found && len(postRenderers) > 0 {
			for _, pr := range postRenderers {
				prMap, ok := pr.(map[string]interface{})
				if !ok {
					continue
				}
				patches, found, _ := unstructured.NestedSlice(prMap, "kustomize", "patches")
				if !found {
					continue
				}
				for _, patch := range patches {
					patchMap, ok := patch.(map[string]interface{})
					if !ok {
						continue
					}
					// Check the target field which specifies what resource the patch applies to
					target, _, _ := unstructured.NestedStringMap(patchMap, "target")
					if targetName, ok := target["name"]; ok && targetName != "" {
						chartName, _, _ := unstructured.NestedString(item.Object, "spec", "chart", "spec", "chart")
						// If target name doesn't match chart name or release name, it might be a mismatch
						if chartName != "" && targetName != chartName && targetName != name && !strings.Contains(targetName, chartName) {
							findings = append(findings, StuckFinding{
								CCVEID:      "CCVE-2025-0673",
								Category:    "SILENT",
								Severity:    "warning",
								Kind:        "HelmRelease",
								Name:        name,
								Namespace:   namespace,
								Condition:   fmt.Sprintf("postRenderer targets %s", targetName),
								Reason:      "PostRendererNameMismatch",
								Message:     fmt.Sprintf("postRenderer patch targets '%s'; may not match chart resource names", targetName),
								Remediation: "Verify target name matches actual resources created by chart",
								Command:     fmt.Sprintf("kubectl get hr %s -n %s -o yaml | grep -A20 postRenderers", name, namespace),
							})
							break
						}
					}
				}
			}
		}

		// Check for zero replicas in values (CCVE-2025-0674)
		values, found, _ := unstructured.NestedMap(item.Object, "spec", "values")
		if found && values != nil {
			// Check common replica keys
			replicaKeys := []string{"replicaCount", "replicas", "minReplicas"}
			for _, key := range replicaKeys {
				if val, exists := values[key]; exists {
					// Check for int64 0 or float64 0
					switch v := val.(type) {
					case int64:
						if v == 0 {
							findings = append(findings, StuckFinding{
								CCVEID:      "CCVE-2025-0674",
								Category:    "SILENT",
								Severity:    "warning",
								Kind:        "HelmRelease",
								Name:        name,
								Namespace:   namespace,
								Condition:   fmt.Sprintf("%s=0", key),
								Reason:      "ZeroReplicas",
								Message:     "Deployment configured with zero replicas; no pods will run",
								Remediation: "Set replicaCount >= 1 or use autoscaling",
								Command:     fmt.Sprintf("kubectl get deploy -l app.kubernetes.io/instance=%s -n %s", name, namespace),
							})
							break
						}
					case float64:
						if v == 0 {
							findings = append(findings, StuckFinding{
								CCVEID:      "CCVE-2025-0674",
								Category:    "SILENT",
								Severity:    "warning",
								Kind:        "HelmRelease",
								Name:        name,
								Namespace:   namespace,
								Condition:   fmt.Sprintf("%s=0", key),
								Reason:      "ZeroReplicas",
								Message:     "Deployment configured with zero replicas; no pods will run",
								Remediation: "Set replicaCount >= 1 or use autoscaling",
								Command:     fmt.Sprintf("kubectl get deploy -l app.kubernetes.io/instance=%s -n %s", name, namespace),
							})
							break
						}
					}
				}
			}
		}
	}

	return findings
}

// isShortTimeout checks if a timeout duration is considered too short
func (s *StateScanner) isShortTimeout(timeout string) bool {
	// Parse duration and check if less than 1 minute
	d, err := time.ParseDuration(timeout)
	if err != nil {
		return false
	}
	return d < time.Minute
}

// scanKustomizationSilentFailures checks Kustomizations for silent misconfigurations
func (s *StateScanner) scanKustomizationSilentFailures(ctx context.Context) []StuckFinding {
	var findings []StuckFinding

	gvr := schema.GroupVersionResource{
		Group:    "kustomize.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: "kustomizations",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Only check resources that appear healthy
		if !s.isReadyOrUnknown(item) {
			continue
		}

		// Check for interval: 0 (CCVE-2025-0665)
		interval, found, _ := unstructured.NestedString(item.Object, "spec", "interval")
		if found && (interval == "0" || interval == "0s" || interval == "0m") {
			findings = append(findings, StuckFinding{
				CCVEID:      "CCVE-2025-0665",
				Category:    "SILENT",
				Severity:    "warning",
				Kind:        "Kustomization",
				Name:        name,
				Namespace:   namespace,
				Condition:   "interval=0",
				Reason:      "ReconciliationDisabled",
				Message:     "interval: 0 disables continuous reconciliation; drift will not be corrected",
				Remediation: "Set a non-zero interval (e.g., spec.interval: 5m) to enable reconciliation",
				Command:     fmt.Sprintf("kubectl patch kustomization %s -n %s --type=merge -p '{\"spec\":{\"interval\":\"5m\"}}'", name, namespace),
			})
		}

		// Check for substituteFrom with optional: true pointing to missing ConfigMaps (CCVE-2025-0664)
		substituteFrom, found, _ := unstructured.NestedSlice(item.Object, "spec", "postBuild", "substituteFrom")
		if found {
			for _, sf := range substituteFrom {
				sfMap, ok := sf.(map[string]interface{})
				if !ok {
					continue
				}

				optional, _ := sfMap["optional"].(bool)
				if !optional {
					continue
				}

				kind, _ := sfMap["kind"].(string)
				refName, _ := sfMap["name"].(string)

				if kind == "" || refName == "" {
					continue
				}

				exists := s.checkResourceExists(ctx, namespace, kind, refName)
				if !exists {
					findings = append(findings, StuckFinding{
						CCVEID:      "CCVE-2025-0664",
						Category:    "SILENT",
						Severity:    "critical",
						Kind:        "Kustomization",
						Name:        name,
						Namespace:   namespace,
						Condition:   fmt.Sprintf("substituteFrom.%s=%s missing", kind, refName),
						Reason:      "OptionalSourceMissing",
						Message:     fmt.Sprintf("substituteFrom references missing %s/%s; variables remain as literals", kind, refName),
						Remediation: fmt.Sprintf("Create the %s '%s' or remove optional:true to fail explicitly", kind, refName),
						Command:     fmt.Sprintf("kubectl get %s %s -n %s", strings.ToLower(kind), refName, namespace),
					})
				}
			}
		}

		// Check for sourceRef to suspended source (CCVE-2025-0666)
		sourceKind, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "kind")
		sourceName, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "name")
		sourceNS, _, _ := unstructured.NestedString(item.Object, "spec", "sourceRef", "namespace")
		if sourceNS == "" {
			sourceNS = namespace
		}

		if sourceKind == "GitRepository" && sourceName != "" {
			if s.isSourceSuspended(ctx, sourceNS, "gitrepositories", sourceName) {
				findings = append(findings, StuckFinding{
					CCVEID:      "CCVE-2025-0666",
					Category:    "SILENT",
					Severity:    "critical",
					Kind:        "Kustomization",
					Name:        name,
					Namespace:   namespace,
					Condition:   fmt.Sprintf("sourceRef.GitRepository=%s suspended", sourceName),
					Reason:      "SourceSuspended",
					Message:     "sourceRef points to suspended GitRepository; using stale revision",
					Remediation: "Resume the GitRepository or point to an active source",
					Command:     fmt.Sprintf("flux resume source git %s -n %s", sourceName, sourceNS),
				})
			}
		}
	}

	return findings
}

// isReadyOrUnknown checks if a resource appears healthy (Ready=True or Unknown)
func (s *StateScanner) isReadyOrUnknown(item unstructured.Unstructured) bool {
	conditions, found, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	if !found {
		return true // No conditions = assume unknown
	}

	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}
		condType, _ := condMap["type"].(string)
		status, _ := condMap["status"].(string)

		if condType == "Ready" {
			return status == "True" || status == "Unknown"
		}
	}
	return true
}

// checkResourceExists checks if a ConfigMap or Secret exists
func (s *StateScanner) checkResourceExists(ctx context.Context, namespace, kind, name string) bool {
	var gvr schema.GroupVersionResource

	switch kind {
	case "ConfigMap":
		gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	case "Secret":
		gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	default:
		return true // Assume exists for unknown kinds
	}

	_, err := s.client.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	return err == nil
}

// isSourceSuspended checks if a Flux source is suspended
func (s *StateScanner) isSourceSuspended(ctx context.Context, namespace, resource, name string) bool {
	gvr := schema.GroupVersionResource{
		Group:    "source.toolkit.fluxcd.io",
		Version:  "v1",
		Resource: resource,
	}

	obj, err := s.client.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return false
	}

	suspended, _, _ := unstructured.NestedBool(obj.Object, "spec", "suspend")
	return suspended
}

// =============================================================================
// TIMING BOMB DETECTION
// =============================================================================

// TimingBombFinding represents a configuration that will fail in the future
type TimingBombFinding struct {
	CCVEID      string    `json:"ccveId"`
	Category    string    `json:"category"`
	Severity    string    `json:"severity"`
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	ExpiresAt   time.Time `json:"expiresAt"`
	ExpiresIn   string    `json:"expiresIn"`
	Reason      string    `json:"reason"`
	Message     string    `json:"message"`
	Remediation string    `json:"remediation"`
	Command     string    `json:"command,omitempty"`
}

// TimingBombResult contains all timing bomb findings
type TimingBombResult struct {
	ScannedAt time.Time           `json:"scannedAt"`
	Findings  []TimingBombFinding `json:"findings"`
	Summary   TimingBombSummary   `json:"summary"`
}

// TimingBombSummary counts timing bombs by urgency
type TimingBombSummary struct {
	Critical int `json:"critical"` // Expires within 3 days
	Warning  int `json:"warning"`  // Expires within 14 days
	Info     int `json:"info"`     // Expires within 30 days
	Total    int `json:"total"`
}

// Default timing bomb thresholds
const (
	TimingBombCritical = 3 * 24 * time.Hour  // 3 days
	TimingBombWarning  = 14 * 24 * time.Hour // 14 days
	TimingBombInfo     = 30 * 24 * time.Hour // 30 days
)

// ScanTimingBombs scans for configurations that will fail in the future
func (s *StateScanner) ScanTimingBombs(ctx context.Context) (*TimingBombResult, error) {
	result := &TimingBombResult{
		ScannedAt: time.Now(),
		Findings:  []TimingBombFinding{},
	}

	// Scan cert-manager Certificates
	certFindings := s.scanCertificateExpiry(ctx)
	result.Findings = append(result.Findings, certFindings...)

	// Scan TLS Secrets directly (for non-cert-manager certs)
	secretFindings := s.scanTLSSecretExpiry(ctx)
	result.Findings = append(result.Findings, secretFindings...)

	// Scan ResourceQuota usage > 90%
	quotaFindings := s.scanResourceQuotaUsage(ctx)
	result.Findings = append(result.Findings, quotaFindings...)

	// Scan PDB blocking evictions (minAvailable: 100% or maxUnavailable: 0)
	pdbFindings := s.scanPDBMisconfiguration(ctx)
	result.Findings = append(result.Findings, pdbFindings...)

	// Scan HPA with min = max (no scaling possible)
	hpaFindings := s.scanHPAMisconfiguration(ctx)
	result.Findings = append(result.Findings, hpaFindings...)

	// Calculate summary
	for _, f := range result.Findings {
		switch f.Severity {
		case "critical":
			result.Summary.Critical++
		case "warning":
			result.Summary.Warning++
		case "info":
			result.Summary.Info++
		}
	}
	result.Summary.Total = len(result.Findings)

	return result, nil
}

// scanCertificateExpiry checks cert-manager Certificate resources for expiry
func (s *StateScanner) scanCertificateExpiry(ctx context.Context) []TimingBombFinding {
	var findings []TimingBombFinding

	gvr := schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		// cert-manager not installed, skip
		return nil
	}

	now := time.Now()

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Get notAfter from status
		notAfterStr, found, _ := unstructured.NestedString(item.Object, "status", "notAfter")
		if !found || notAfterStr == "" {
			continue
		}

		notAfter, err := time.Parse(time.RFC3339, notAfterStr)
		if err != nil {
			continue
		}

		timeUntilExpiry := notAfter.Sub(now)
		if timeUntilExpiry <= 0 {
			// Already expired
			findings = append(findings, TimingBombFinding{
				CCVEID:      "CCVE-2025-0675",
				Category:    "TIMING",
				Severity:    "critical",
				Kind:        "Certificate",
				Name:        name,
				Namespace:   namespace,
				ExpiresAt:   notAfter,
				ExpiresIn:   "EXPIRED",
				Reason:      "CertificateExpired",
				Message:     "Certificate has expired; TLS connections will fail",
				Remediation: "Renew certificate immediately; check cert-manager logs for renewal failures",
				Command:     fmt.Sprintf("kubectl cert-manager renew %s -n %s", name, namespace),
			})
		} else if timeUntilExpiry <= TimingBombInfo {
			severity := s.timingBombSeverity(timeUntilExpiry)
			findings = append(findings, TimingBombFinding{
				CCVEID:      "CCVE-2025-0675",
				Category:    "TIMING",
				Severity:    severity,
				Kind:        "Certificate",
				Name:        name,
				Namespace:   namespace,
				ExpiresAt:   notAfter,
				ExpiresIn:   formatDurationDays(timeUntilExpiry),
				Reason:      "CertificateExpiringSoon",
				Message:     fmt.Sprintf("Certificate expires in %s", formatDurationDays(timeUntilExpiry)),
				Remediation: "Verify cert-manager is renewing; check Certificate status and issuer health",
				Command:     fmt.Sprintf("kubectl describe certificate %s -n %s", name, namespace),
			})
		}
	}

	return findings
}

// scanTLSSecretExpiry checks kubernetes.io/tls Secrets for certificate expiry
func (s *StateScanner) scanTLSSecretExpiry(ctx context.Context) []TimingBombFinding {
	var findings []TimingBombFinding

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{
		FieldSelector: "type=kubernetes.io/tls",
	})
	if err != nil {
		return nil
	}

	now := time.Now()

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Skip secrets managed by cert-manager (already checked above)
		labels := item.GetLabels()
		if labels != nil {
			if _, hasCertManager := labels["cert-manager.io/certificate-name"]; hasCertManager {
				continue
			}
		}

		// Get tls.crt data
		data, found, _ := unstructured.NestedMap(item.Object, "data")
		if !found {
			continue
		}

		tlsCrtB64, ok := data["tls.crt"].(string)
		if !ok || tlsCrtB64 == "" {
			continue
		}

		// Decode base64
		tlsCrtPEM, err := base64Decode(tlsCrtB64)
		if err != nil {
			continue
		}

		// Parse certificate to get expiry
		notAfter, err := parseCertificateExpiry(tlsCrtPEM)
		if err != nil {
			continue
		}

		timeUntilExpiry := notAfter.Sub(now)
		if timeUntilExpiry <= 0 {
			findings = append(findings, TimingBombFinding{
				CCVEID:      "CCVE-2025-0676",
				Category:    "TIMING",
				Severity:    "critical",
				Kind:        "Secret",
				Name:        name,
				Namespace:   namespace,
				ExpiresAt:   notAfter,
				ExpiresIn:   "EXPIRED",
				Reason:      "TLSSecretExpired",
				Message:     "TLS certificate in Secret has expired",
				Remediation: "Update Secret with valid certificate or configure cert-manager",
				Command:     fmt.Sprintf("kubectl get secret %s -n %s -o jsonpath='{.data.tls\\.crt}' | base64 -d | openssl x509 -noout -dates", name, namespace),
			})
		} else if timeUntilExpiry <= TimingBombInfo {
			severity := s.timingBombSeverity(timeUntilExpiry)
			findings = append(findings, TimingBombFinding{
				CCVEID:      "CCVE-2025-0676",
				Category:    "TIMING",
				Severity:    severity,
				Kind:        "Secret",
				Name:        name,
				Namespace:   namespace,
				ExpiresAt:   notAfter,
				ExpiresIn:   formatDurationDays(timeUntilExpiry),
				Reason:      "TLSSecretExpiringSoon",
				Message:     fmt.Sprintf("TLS certificate in Secret expires in %s", formatDurationDays(timeUntilExpiry)),
				Remediation: "Update Secret before expiry or migrate to cert-manager for automated renewal",
				Command:     fmt.Sprintf("kubectl get secret %s -n %s -o jsonpath='{.data.tls\\.crt}' | base64 -d | openssl x509 -noout -dates", name, namespace),
			})
		}
	}

	return findings
}

// timingBombSeverity returns severity based on time until expiry
func (s *StateScanner) timingBombSeverity(timeUntil time.Duration) string {
	if timeUntil <= TimingBombCritical {
		return "critical"
	}
	if timeUntil <= TimingBombWarning {
		return "warning"
	}
	return "info"
}

// formatDurationDays formats a duration as days/hours
func formatDurationDays(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days > 1 {
		return fmt.Sprintf("%d days", days)
	}
	if days == 1 {
		return "1 day"
	}
	hours := int(d.Hours())
	if hours > 1 {
		return fmt.Sprintf("%d hours", hours)
	}
	return "< 1 hour"
}

// base64Decode decodes a base64 string
func base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// parseCertificateExpiry extracts the NotAfter date from a PEM certificate
func parseCertificateExpiry(pemData []byte) (time.Time, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return time.Time{}, fmt.Errorf("failed to decode PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}

	return cert.NotAfter, nil
}

// scanResourceQuotaUsage checks for ResourceQuotas approaching limits
func (s *StateScanner) scanResourceQuotaUsage(ctx context.Context) []TimingBombFinding {
	var findings []TimingBombFinding

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "resourcequotas",
	}

	// List all ResourceQuotas
	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Get status.hard and status.used maps
		hardMap, _, _ := unstructured.NestedStringMap(item.Object, "status", "hard")
		usedMap, _, _ := unstructured.NestedStringMap(item.Object, "status", "used")

		if hardMap == nil || usedMap == nil {
			continue
		}

		// Check each resource in the quota
		for resource, hardStr := range hardMap {
			usedStr, ok := usedMap[resource]
			if !ok {
				continue
			}

			// Parse quantities
			hard, err := resourceQuantityParse(hardStr)
			if err != nil || hard == 0 {
				continue
			}

			used, err := resourceQuantityParse(usedStr)
			if err != nil {
				continue
			}

			pct := float64(used) / float64(hard) * 100

			// Different thresholds for different severities
			if pct >= 100 {
				findings = append(findings, TimingBombFinding{
					CCVEID:      "CCVE-2025-0677",
					Category:    "TIMING",
					Severity:    "critical",
					Kind:        "ResourceQuota",
					Name:        name,
					Namespace:   namespace,
					ExpiresAt:   time.Now(), // Already at limit
					ExpiresIn:   "AT LIMIT",
					Reason:      "QuotaExhausted",
					Message:     fmt.Sprintf("%s: %s/%s (100%%) - quota exhausted", resource, usedStr, hardStr),
					Remediation: "Increase quota limit or reduce resource usage",
					Command:     fmt.Sprintf("kubectl describe resourcequota %s -n %s", name, namespace),
				})
			} else if pct >= 95 {
				findings = append(findings, TimingBombFinding{
					CCVEID:      "CCVE-2025-0677",
					Category:    "TIMING",
					Severity:    "critical",
					Kind:        "ResourceQuota",
					Name:        name,
					Namespace:   namespace,
					ExpiresAt:   time.Now(), // About to hit limit
					ExpiresIn:   fmt.Sprintf("%.0f%% used", pct),
					Reason:      "QuotaNearLimit",
					Message:     fmt.Sprintf("%s: %s/%s (%.0f%%) - approaching limit", resource, usedStr, hardStr, pct),
					Remediation: "Increase quota limit before deployments fail",
					Command:     fmt.Sprintf("kubectl describe resourcequota %s -n %s", name, namespace),
				})
			} else if pct >= 90 {
				findings = append(findings, TimingBombFinding{
					CCVEID:      "CCVE-2025-0677",
					Category:    "TIMING",
					Severity:    "warning",
					Kind:        "ResourceQuota",
					Name:        name,
					Namespace:   namespace,
					ExpiresAt:   time.Now(),
					ExpiresIn:   fmt.Sprintf("%.0f%% used", pct),
					Reason:      "QuotaHighUsage",
					Message:     fmt.Sprintf("%s: %s/%s (%.0f%%) - high usage", resource, usedStr, hardStr, pct),
					Remediation: "Monitor quota usage; consider increasing limit",
					Command:     fmt.Sprintf("kubectl describe resourcequota %s -n %s", name, namespace),
				})
			}
		}
	}

	return findings
}

// resourceQuantityParse parses a Kubernetes quantity string to int64
func resourceQuantityParse(s string) (int64, error) {
	// Handle simple integer cases first (count resources like pods, configmaps)
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val, nil
	}

	// Handle Kubernetes quantity format (1Gi, 500m, etc.)
	// For now, use a simple suffix-based approach
	s = strings.TrimSpace(s)

	multiplier := int64(1)
	suffix := ""

	// Extract numeric part and suffix
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] >= '0' && s[i] <= '9' {
			suffix = s[i+1:]
			s = s[:i+1]
			break
		}
	}

	// Parse the numeric part
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}

	// Apply multiplier based on suffix
	switch strings.ToLower(suffix) {
	case "ki":
		multiplier = 1024
	case "mi":
		multiplier = 1024 * 1024
	case "gi":
		multiplier = 1024 * 1024 * 1024
	case "ti":
		multiplier = 1024 * 1024 * 1024 * 1024
	case "k":
		multiplier = 1000
	case "m":
		// Note: In Kubernetes, 'm' can mean milli (1/1000) for CPU or mega for memory
		// For quota purposes, we'll treat it as milli for CPU-like resources
		multiplier = 1
		val = val / 1000
	case "":
		multiplier = 1
	}

	return int64(val * float64(multiplier)), nil
}

// scanPDBMisconfiguration checks for PodDisruptionBudgets that block evictions
func (s *StateScanner) scanPDBMisconfiguration(ctx context.Context) []TimingBombFinding {
	var findings []TimingBombFinding

	gvr := schema.GroupVersionResource{
		Group:    "policy",
		Version:  "v1",
		Resource: "poddisruptionbudgets",
	}

	// List all PDBs
	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Check minAvailable - if it's 100% or matches desired replicas
		minAvail, foundMin, _ := unstructured.NestedFieldCopy(item.Object, "spec", "minAvailable")
		maxUnavail, foundMax, _ := unstructured.NestedFieldCopy(item.Object, "spec", "maxUnavailable")

		// Get current status
		desiredHealthy, _, _ := unstructured.NestedInt64(item.Object, "status", "desiredHealthy")
		currentHealthy, _, _ := unstructured.NestedInt64(item.Object, "status", "currentHealthy")
		disruptionsAllowed, _, _ := unstructured.NestedInt64(item.Object, "status", "disruptionsAllowed")

		var reason, message, remediation string
		severity := "warning"

		// Check if disruptionsAllowed = 0 (the actual blocking condition)
		if disruptionsAllowed == 0 && currentHealthy > 0 {
			// PDB is actively blocking evictions
			severity = "critical"
			reason = "PDBBlockingEvictions"
			message = fmt.Sprintf("PDB allows 0 disruptions (currentHealthy: %d, desiredHealthy: %d); node drains will fail", currentHealthy, desiredHealthy)
			remediation = "Reduce minAvailable or increase maxUnavailable to allow rolling updates"

			findings = append(findings, TimingBombFinding{
				CCVEID:      "CCVE-2025-0678",
				Category:    "TIMING",
				Severity:    severity,
				Kind:        "PodDisruptionBudget",
				Name:        name,
				Namespace:   namespace,
				ExpiresAt:   time.Now(), // Already problematic
				ExpiresIn:   "BLOCKING",
				Reason:      reason,
				Message:     message,
				Remediation: remediation,
				Command:     fmt.Sprintf("kubectl get pdb %s -n %s -o yaml", name, namespace),
			})
			continue
		}

		// Check for 100% string value
		if foundMin {
			if minStr, ok := minAvail.(string); ok {
				if minStr == "100%" {
					reason = "MinAvailable100Percent"
					message = "minAvailable: 100% blocks all evictions; node drains will fail"
					remediation = "Set minAvailable to less than 100% (e.g., 90% or n-1)"

					findings = append(findings, TimingBombFinding{
						CCVEID:      "CCVE-2025-0678",
						Category:    "TIMING",
						Severity:    severity,
						Kind:        "PodDisruptionBudget",
						Name:        name,
						Namespace:   namespace,
						ExpiresAt:   time.Now(),
						ExpiresIn:   "WILL BLOCK",
						Reason:      reason,
						Message:     message,
						Remediation: remediation,
						Command:     fmt.Sprintf("kubectl get pdb %s -n %s -o yaml", name, namespace),
					})
					continue
				}
			}
		}

		// Check for maxUnavailable: 0
		if foundMax {
			maxVal := int64(0)
			switch v := maxUnavail.(type) {
			case int64:
				maxVal = v
			case float64:
				maxVal = int64(v)
			case string:
				if v == "0" || v == "0%" {
					maxVal = 0
				} else {
					continue // Not zero
				}
			default:
				continue
			}

			if maxVal == 0 {
				reason = "MaxUnavailableZero"
				message = "maxUnavailable: 0 blocks all evictions; node drains will fail"
				remediation = "Set maxUnavailable to at least 1 to allow rolling updates"

				findings = append(findings, TimingBombFinding{
					CCVEID:      "CCVE-2025-0678",
					Category:    "TIMING",
					Severity:    severity,
					Kind:        "PodDisruptionBudget",
					Name:        name,
					Namespace:   namespace,
					ExpiresAt:   time.Now(),
					ExpiresIn:   "WILL BLOCK",
					Reason:      reason,
					Message:     message,
					Remediation: remediation,
					Command:     fmt.Sprintf("kubectl get pdb %s -n %s -o yaml", name, namespace),
				})
			}
		}
	}

	return findings
}

// scanHPAMisconfiguration checks for HPAs where min = max (no scaling possible)
func (s *StateScanner) scanHPAMisconfiguration(ctx context.Context) []TimingBombFinding {
	var findings []TimingBombFinding

	// Check both v2 and v2beta2 APIs
	versions := []string{"v2", "v2beta2"}

	for _, version := range versions {
		gvr := schema.GroupVersionResource{
			Group:    "autoscaling",
			Version:  version,
			Resource: "horizontalpodautoscalers",
		}

		list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
		if err != nil {
			continue // Try next version
		}

		for _, item := range list.Items {
			name := item.GetName()
			namespace := item.GetNamespace()

			minReplicas, foundMin, _ := unstructured.NestedInt64(item.Object, "spec", "minReplicas")
			maxReplicas, foundMax, _ := unstructured.NestedInt64(item.Object, "spec", "maxReplicas")

			if !foundMin || !foundMax {
				continue
			}

			// Default minReplicas is 1 if not set
			if minReplicas == 0 {
				minReplicas = 1
			}

			if minReplicas == maxReplicas {
				findings = append(findings, TimingBombFinding{
					CCVEID:      "CCVE-2025-0679",
					Category:    "TIMING",
					Severity:    "warning",
					Kind:        "HorizontalPodAutoscaler",
					Name:        name,
					Namespace:   namespace,
					ExpiresAt:   time.Now(),
					ExpiresIn:   "NO SCALING",
					Reason:      "HPAMinEqualsMax",
					Message:     fmt.Sprintf("minReplicas (%d) = maxReplicas (%d); autoscaling is disabled", minReplicas, maxReplicas),
					Remediation: "Set different min/max values to enable autoscaling, or remove HPA if static scaling is intended",
					Command:     fmt.Sprintf("kubectl get hpa %s -n %s -o yaml", name, namespace),
				})
			}
		}

		// If v2 worked, don't check v2beta2
		if list != nil && len(list.Items) > 0 {
			break
		}
	}

	return findings
}

// =============================================================================
// UNRESOLVED FINDINGS DETECTION
// =============================================================================

// UnresolvedFinding represents a security/policy finding from another tool that hasn't been fixed
type UnresolvedFinding struct {
	CCVEID      string    `json:"ccveId"`
	Category    string    `json:"category"`
	Source      string    `json:"source"` // trivy, kyverno, gatekeeper
	Severity    string    `json:"severity"`
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	Namespace   string    `json:"namespace"`
	FindingType string    `json:"findingType"` // vulnerability, misconfiguration, policy
	Count       int       `json:"count"`       // Number of findings in this report
	Message     string    `json:"message"`
	FirstSeen   time.Time `json:"firstSeen,omitempty"`
	Command     string    `json:"command,omitempty"`
}

// UnresolvedResult contains all unresolved findings
type UnresolvedResult struct {
	ScannedAt time.Time           `json:"scannedAt"`
	Findings  []UnresolvedFinding `json:"findings"`
	Summary   UnresolvedSummary   `json:"summary"`
}

// UnresolvedSummary counts unresolved findings by source
type UnresolvedSummary struct {
	Trivy      int `json:"trivy"`
	Kyverno    int `json:"kyverno"`
	Gatekeeper int `json:"gatekeeper"`
	Critical   int `json:"critical"`
	High       int `json:"high"`
	Total      int `json:"total"`
}

// ScanUnresolvedFindings scans for unresolved findings from security tools
func (s *StateScanner) ScanUnresolvedFindings(ctx context.Context) (*UnresolvedResult, error) {
	result := &UnresolvedResult{
		ScannedAt: time.Now(),
		Findings:  []UnresolvedFinding{},
	}

	// Scan Trivy VulnerabilityReports
	trivyVulns := s.scanTrivyVulnerabilityReports(ctx)
	result.Findings = append(result.Findings, trivyVulns...)

	// Scan Trivy ConfigAuditReports
	trivyConfigs := s.scanTrivyConfigAuditReports(ctx)
	result.Findings = append(result.Findings, trivyConfigs...)

	// Scan Kyverno PolicyReports
	kyvernoFindings := s.scanKyvernoPolicyReports(ctx)
	result.Findings = append(result.Findings, kyvernoFindings...)

	// Calculate summary
	for _, f := range result.Findings {
		switch f.Source {
		case "trivy":
			result.Summary.Trivy++
		case "kyverno":
			result.Summary.Kyverno++
		case "gatekeeper":
			result.Summary.Gatekeeper++
		}
		switch f.Severity {
		case "critical":
			result.Summary.Critical++
		case "high":
			result.Summary.High++
		}
	}
	result.Summary.Total = len(result.Findings)

	return result, nil
}

// scanTrivyVulnerabilityReports checks for Trivy Operator VulnerabilityReports
func (s *StateScanner) scanTrivyVulnerabilityReports(ctx context.Context) []UnresolvedFinding {
	var findings []UnresolvedFinding

	gvr := schema.GroupVersionResource{
		Group:    "aquasecurity.github.io",
		Version:  "v1alpha1",
		Resource: "vulnerabilityreports",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		// Trivy Operator not installed
		return nil
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Get vulnerabilities from report
		vulns, found, _ := unstructured.NestedSlice(item.Object, "report", "vulnerabilities")
		if !found || len(vulns) == 0 {
			continue
		}

		// Count by severity
		criticalCount := 0
		highCount := 0

		for _, v := range vulns {
			vuln, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			sev, _ := vuln["severity"].(string)
			switch strings.ToUpper(sev) {
			case "CRITICAL":
				criticalCount++
			case "HIGH":
				highCount++
			}
		}

		// Only report if there are critical or high vulnerabilities
		if criticalCount > 0 {
			findings = append(findings, UnresolvedFinding{
				CCVEID:      "CCVE-2025-0680",
				Category:    "UNRESOLVED",
				Source:      "trivy",
				Severity:    "critical",
				Kind:        "VulnerabilityReport",
				Name:        name,
				Namespace:   namespace,
				FindingType: "vulnerability",
				Count:       criticalCount,
				Message:     fmt.Sprintf("%d critical vulnerabilities unresolved", criticalCount),
				Command:     fmt.Sprintf("kubectl get vulnerabilityreport %s -n %s -o yaml", name, namespace),
			})
		}

		if highCount > 0 {
			findings = append(findings, UnresolvedFinding{
				CCVEID:      "CCVE-2025-0680",
				Category:    "UNRESOLVED",
				Source:      "trivy",
				Severity:    "high",
				Kind:        "VulnerabilityReport",
				Name:        name,
				Namespace:   namespace,
				FindingType: "vulnerability",
				Count:       highCount,
				Message:     fmt.Sprintf("%d high vulnerabilities unresolved", highCount),
				Command:     fmt.Sprintf("kubectl get vulnerabilityreport %s -n %s -o yaml", name, namespace),
			})
		}
	}

	return findings
}

// scanTrivyConfigAuditReports checks for Trivy Operator ConfigAuditReports
func (s *StateScanner) scanTrivyConfigAuditReports(ctx context.Context) []UnresolvedFinding {
	var findings []UnresolvedFinding

	gvr := schema.GroupVersionResource{
		Group:    "aquasecurity.github.io",
		Version:  "v1alpha1",
		Resource: "configauditreports",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Get checks from report
		checks, found, _ := unstructured.NestedSlice(item.Object, "report", "checks")
		if !found || len(checks) == 0 {
			continue
		}

		// Count failed checks by severity
		criticalCount := 0
		highCount := 0

		for _, c := range checks {
			check, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			success, _ := check["success"].(bool)
			if success {
				continue // Only count failures
			}
			sev, _ := check["severity"].(string)
			switch strings.ToUpper(sev) {
			case "CRITICAL":
				criticalCount++
			case "HIGH":
				highCount++
			}
		}

		if criticalCount > 0 {
			findings = append(findings, UnresolvedFinding{
				CCVEID:      "CCVE-2025-0681",
				Category:    "UNRESOLVED",
				Source:      "trivy",
				Severity:    "critical",
				Kind:        "ConfigAuditReport",
				Name:        name,
				Namespace:   namespace,
				FindingType: "misconfiguration",
				Count:       criticalCount,
				Message:     fmt.Sprintf("%d critical misconfigurations unresolved", criticalCount),
				Command:     fmt.Sprintf("kubectl get configauditreport %s -n %s -o yaml", name, namespace),
			})
		}

		if highCount > 0 {
			findings = append(findings, UnresolvedFinding{
				CCVEID:      "CCVE-2025-0681",
				Category:    "UNRESOLVED",
				Source:      "trivy",
				Severity:    "high",
				Kind:        "ConfigAuditReport",
				Name:        name,
				Namespace:   namespace,
				FindingType: "misconfiguration",
				Count:       highCount,
				Message:     fmt.Sprintf("%d high misconfigurations unresolved", highCount),
				Command:     fmt.Sprintf("kubectl get configauditreport %s -n %s -o yaml", name, namespace),
			})
		}
	}

	return findings
}

// scanKyvernoPolicyReports checks for Kyverno PolicyReports with failures
func (s *StateScanner) scanKyvernoPolicyReports(ctx context.Context) []UnresolvedFinding {
	var findings []UnresolvedFinding

	gvr := schema.GroupVersionResource{
		Group:    "wgpolicyk8s.io",
		Version:  "v1alpha2",
		Resource: "policyreports",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		// Try v1beta1
		gvr.Version = "v1beta1"
		list, err = s.client.Resource(gvr).List(ctx, v1.ListOptions{})
		if err != nil {
			return nil
		}
	}

	for _, item := range list.Items {
		name := item.GetName()
		namespace := item.GetNamespace()

		// Get results from report
		results, found, _ := unstructured.NestedSlice(item.Object, "results")
		if !found || len(results) == 0 {
			continue
		}

		// Count failed results by severity
		criticalCount := 0
		highCount := 0

		for _, r := range results {
			result, ok := r.(map[string]interface{})
			if !ok {
				continue
			}
			status, _ := result["result"].(string)
			if status != "fail" {
				continue
			}
			sev, _ := result["severity"].(string)
			switch strings.ToLower(sev) {
			case "critical":
				criticalCount++
			case "high":
				highCount++
			}
		}

		if criticalCount > 0 {
			findings = append(findings, UnresolvedFinding{
				CCVEID:      "CCVE-2025-0682",
				Category:    "UNRESOLVED",
				Source:      "kyverno",
				Severity:    "critical",
				Kind:        "PolicyReport",
				Name:        name,
				Namespace:   namespace,
				FindingType: "policy",
				Count:       criticalCount,
				Message:     fmt.Sprintf("%d critical policy violations unresolved", criticalCount),
				Command:     fmt.Sprintf("kubectl get policyreport %s -n %s -o yaml", name, namespace),
			})
		}

		if highCount > 0 {
			findings = append(findings, UnresolvedFinding{
				CCVEID:      "CCVE-2025-0682",
				Category:    "UNRESOLVED",
				Source:      "kyverno",
				Severity:    "high",
				Kind:        "PolicyReport",
				Name:        name,
				Namespace:   namespace,
				FindingType: "policy",
				Count:       highCount,
				Message:     fmt.Sprintf("%d high policy violations unresolved", highCount),
				Command:     fmt.Sprintf("kubectl get policyreport %s -n %s -o yaml", name, namespace),
			})
		}
	}

	return findings
}

// DanglingFinding represents a resource that references non-existent targets
type DanglingFinding struct {
	CCVEID      string `json:"ccve_id"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	TargetKind  string `json:"target_kind"`
	TargetName  string `json:"target_name"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
	Command     string `json:"command"`
}

// DanglingResult contains all dangling resource findings
type DanglingResult struct {
	Findings []DanglingFinding `json:"findings"`
	Summary  struct {
		Total           int `json:"total"`
		HPAs            int `json:"hpas"`
		VPAs            int `json:"vpas"`
		Services        int `json:"services"`
		Ingresses       int `json:"ingresses"`
		NetworkPolicies int `json:"network_policies"`
		PVCs            int `json:"pvcs"`
		Secrets         int `json:"secrets"`
	} `json:"summary"`
}

// ScanDanglingResources detects resources that reference non-existent targets
// This implements KubeLinter-style orphan detection patterns
func (s *StateScanner) ScanDanglingResources(ctx context.Context) (*DanglingResult, error) {
	result := &DanglingResult{}

	// Scan for dangling HPAs
	hpaFindings := s.scanDanglingHPAs(ctx)
	result.Findings = append(result.Findings, hpaFindings...)
	result.Summary.HPAs = len(hpaFindings)

	// Scan for dangling VPAs
	vpaFindings := s.scanDanglingVPAs(ctx)
	result.Findings = append(result.Findings, vpaFindings...)
	result.Summary.VPAs = len(vpaFindings)

	// Scan for dangling Services
	svcFindings := s.scanDanglingServices(ctx)
	result.Findings = append(result.Findings, svcFindings...)
	result.Summary.Services = len(svcFindings)

	// Scan for dangling Ingresses
	ingressFindings := s.scanDanglingIngresses(ctx)
	result.Findings = append(result.Findings, ingressFindings...)
	result.Summary.Ingresses = len(ingressFindings)

	// Scan for dangling NetworkPolicies
	npFindings := s.scanDanglingNetworkPolicies(ctx)
	result.Findings = append(result.Findings, npFindings...)
	result.Summary.NetworkPolicies = len(npFindings)

	// Scan for dangling PVCs (Pods referencing non-existent PersistentVolumeClaims)
	pvcFindings := s.scanDanglingPVCs(ctx)
	result.Findings = append(result.Findings, pvcFindings...)
	result.Summary.PVCs = len(pvcFindings)

	// Scan for dangling Secrets (Pods referencing non-existent Secrets)
	secretFindings := s.scanDanglingSecrets(ctx)
	result.Findings = append(result.Findings, secretFindings...)
	result.Summary.Secrets = len(secretFindings)

	result.Summary.Total = len(result.Findings)

	return result, nil
}

// scanDanglingHPAs detects HorizontalPodAutoscalers targeting non-existent workloads
func (s *StateScanner) scanDanglingHPAs(ctx context.Context) []DanglingFinding {
	var findings []DanglingFinding

	// List all HPAs
	hpaList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "autoscaling",
		Version:  "v2",
		Resource: "horizontalpodautoscalers",
	}).List(ctx, v1.ListOptions{})

	if err != nil {
		// Try v1 if v2 fails
		hpaList, err = s.client.Resource(schema.GroupVersionResource{
			Group:    "autoscaling",
			Version:  "v1",
			Resource: "horizontalpodautoscalers",
		}).List(ctx, v1.ListOptions{})
		if err != nil {
			return findings
		}
	}

	for _, hpa := range hpaList.Items {
		name := hpa.GetName()
		namespace := hpa.GetNamespace()

		// Get scale target reference
		scaleTargetRef, found, err := unstructured.NestedMap(hpa.Object, "spec", "scaleTargetRef")
		if err != nil || !found {
			continue
		}

		targetKind, _, _ := unstructured.NestedString(scaleTargetRef, "kind")
		targetName, _, _ := unstructured.NestedString(scaleTargetRef, "name")
		targetAPIVersion, _, _ := unstructured.NestedString(scaleTargetRef, "apiVersion")

		// Check if target exists
		if !s.checkScaleTargetExists(ctx, namespace, targetKind, targetName, targetAPIVersion) {
			findings = append(findings, DanglingFinding{
				CCVEID:      "CCVE-2025-0687",
				Category:    "ORPHAN",
				Severity:    "warning",
				Kind:        "HorizontalPodAutoscaler",
				Name:        name,
				Namespace:   namespace,
				TargetKind:  targetKind,
				TargetName:  targetName,
				Message:     fmt.Sprintf("HPA targets non-existent %s/%s", targetKind, targetName),
				Remediation: "Delete the HPA or create the missing target workload",
				Command:     fmt.Sprintf("kubectl delete hpa %s -n %s", name, namespace),
			})
		}
	}

	return findings
}

// checkScaleTargetExists verifies if an HPA scale target exists
func (s *StateScanner) checkScaleTargetExists(ctx context.Context, namespace, kind, name, apiVersion string) bool {
	var gvr schema.GroupVersionResource

	switch kind {
	case "Deployment":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	case "ReplicaSet":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	case "StatefulSet":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	case "ReplicationController":
		gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "replicationcontrollers"}
	default:
		// Unknown kind, assume exists
		return true
	}

	_, err := s.client.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	return err == nil
}

// scanDanglingServices detects Services with selectors that match no pods
func (s *StateScanner) scanDanglingServices(ctx context.Context) []DanglingFinding {
	var findings []DanglingFinding

	// List all Services
	svcList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}).List(ctx, v1.ListOptions{})

	if err != nil {
		return findings
	}

	for _, svc := range svcList.Items {
		name := svc.GetName()
		namespace := svc.GetNamespace()

		// Skip services without selectors (ExternalName, headless services for StatefulSets, etc.)
		selector, found, err := unstructured.NestedStringMap(svc.Object, "spec", "selector")
		if err != nil || !found || len(selector) == 0 {
			continue
		}

		// Skip kubernetes system service
		if name == "kubernetes" && namespace == "default" {
			continue
		}

		// Check if any pods match the selector
		if !s.checkPodsMatchSelector(ctx, namespace, selector) {
			// Build selector string for display
			selectorStr := ""
			for k, v := range selector {
				if selectorStr != "" {
					selectorStr += ","
				}
				selectorStr += fmt.Sprintf("%s=%s", k, v)
			}

			findings = append(findings, DanglingFinding{
				CCVEID:      "CCVE-2025-0688",
				Category:    "ORPHAN",
				Severity:    "warning",
				Kind:        "Service",
				Name:        name,
				Namespace:   namespace,
				TargetKind:  "Pod",
				TargetName:  selectorStr,
				Message:     fmt.Sprintf("Service selector matches no pods: %s", selectorStr),
				Remediation: "Check if pods with matching labels exist or update the service selector",
				Command:     fmt.Sprintf("kubectl get pods -n %s -l %s", namespace, selectorStr),
			})
		}
	}

	return findings
}

// checkPodsMatchSelector verifies if any pods match the given label selector
func (s *StateScanner) checkPodsMatchSelector(ctx context.Context, namespace string, selector map[string]string) bool {
	// Build label selector string
	selectorStr := ""
	for k, v := range selector {
		if selectorStr != "" {
			selectorStr += ","
		}
		selectorStr += fmt.Sprintf("%s=%s", k, v)
	}

	podList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}).Namespace(namespace).List(ctx, v1.ListOptions{
		LabelSelector: selectorStr,
	})

	if err != nil {
		return true // Assume exists on error
	}

	return len(podList.Items) > 0
}

// scanDanglingIngresses detects Ingresses with backends pointing to non-existent services
func (s *StateScanner) scanDanglingIngresses(ctx context.Context) []DanglingFinding {
	var findings []DanglingFinding

	// Try networking.k8s.io/v1 first
	ingressList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "ingresses",
	}).List(ctx, v1.ListOptions{})

	if err != nil {
		return findings
	}

	for _, ingress := range ingressList.Items {
		name := ingress.GetName()
		namespace := ingress.GetNamespace()

		// Check default backend
		defaultBackend, found, _ := unstructured.NestedMap(ingress.Object, "spec", "defaultBackend")
		if found {
			svcName := s.extractIngressServiceName(defaultBackend)
			if svcName != "" && !s.checkServiceExists(ctx, namespace, svcName) {
				findings = append(findings, DanglingFinding{
					CCVEID:      "CCVE-2025-0689",
					Category:    "ORPHAN",
					Severity:    "warning",
					Kind:        "Ingress",
					Name:        name,
					Namespace:   namespace,
					TargetKind:  "Service",
					TargetName:  svcName,
					Message:     fmt.Sprintf("Ingress default backend references non-existent service: %s", svcName),
					Remediation: "Create the missing service or update the ingress backend",
					Command:     fmt.Sprintf("kubectl get svc %s -n %s", svcName, namespace),
				})
			}
		}

		// Check rules
		rules, found, _ := unstructured.NestedSlice(ingress.Object, "spec", "rules")
		if found {
			for _, rule := range rules {
				ruleMap, ok := rule.(map[string]interface{})
				if !ok {
					continue
				}

				http, found, _ := unstructured.NestedMap(ruleMap, "http")
				if !found {
					continue
				}

				paths, found, _ := unstructured.NestedSlice(http, "paths")
				if !found {
					continue
				}

				for _, path := range paths {
					pathMap, ok := path.(map[string]interface{})
					if !ok {
						continue
					}

					backend, found, _ := unstructured.NestedMap(pathMap, "backend")
					if !found {
						continue
					}

					svcName := s.extractIngressServiceName(backend)
					if svcName != "" && !s.checkServiceExists(ctx, namespace, svcName) {
						findings = append(findings, DanglingFinding{
							CCVEID:      "CCVE-2025-0689",
							Category:    "ORPHAN",
							Severity:    "warning",
							Kind:        "Ingress",
							Name:        name,
							Namespace:   namespace,
							TargetKind:  "Service",
							TargetName:  svcName,
							Message:     fmt.Sprintf("Ingress path backend references non-existent service: %s", svcName),
							Remediation: "Create the missing service or update the ingress backend",
							Command:     fmt.Sprintf("kubectl get svc %s -n %s", svcName, namespace),
						})
					}
				}
			}
		}
	}

	return findings
}

// extractIngressServiceName extracts service name from an ingress backend
func (s *StateScanner) extractIngressServiceName(backend map[string]interface{}) string {
	// networking.k8s.io/v1 format: backend.service.name
	service, found, _ := unstructured.NestedMap(backend, "service")
	if found {
		name, _, _ := unstructured.NestedString(service, "name")
		return name
	}

	// Legacy format: backend.serviceName
	name, _, _ := unstructured.NestedString(backend, "serviceName")
	return name
}

// checkServiceExists verifies if a service exists
func (s *StateScanner) checkServiceExists(ctx context.Context, namespace, name string) bool {
	_, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "services",
	}).Namespace(namespace).Get(ctx, name, v1.GetOptions{})

	return err == nil
}

// scanDanglingNetworkPolicies detects NetworkPolicies with podSelectors that match no pods
func (s *StateScanner) scanDanglingNetworkPolicies(ctx context.Context) []DanglingFinding {
	var findings []DanglingFinding

	// List all NetworkPolicies
	npList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "networking.k8s.io",
		Version:  "v1",
		Resource: "networkpolicies",
	}).List(ctx, v1.ListOptions{})

	if err != nil {
		return findings
	}

	for _, np := range npList.Items {
		name := np.GetName()
		namespace := np.GetNamespace()

		// Get pod selector
		podSelector, found, err := unstructured.NestedMap(np.Object, "spec", "podSelector")
		if err != nil || !found {
			continue
		}

		// Empty podSelector matches all pods in namespace - skip
		matchLabels, foundLabels, _ := unstructured.NestedStringMap(podSelector, "matchLabels")
		matchExpressions, foundExprs, _ := unstructured.NestedSlice(podSelector, "matchExpressions")

		// If both are empty/missing, selector matches all pods - skip
		if (!foundLabels || len(matchLabels) == 0) && (!foundExprs || len(matchExpressions) == 0) {
			continue
		}

		// Build selector string for display (includes both matchLabels and matchExpressions)
		var selectorParts []string
		for k, v := range matchLabels {
			selectorParts = append(selectorParts, fmt.Sprintf("%s=%s", k, v))
		}
		for _, expr := range matchExpressions {
			exprMap, ok := expr.(map[string]interface{})
			if !ok {
				continue
			}
			key, _, _ := unstructured.NestedString(exprMap, "key")
			operator, _, _ := unstructured.NestedString(exprMap, "operator")
			values, _, _ := unstructured.NestedStringSlice(exprMap, "values")
			if key != "" && operator != "" {
				if len(values) > 0 {
					selectorParts = append(selectorParts, fmt.Sprintf("%s %s (%s)", key, operator, strings.Join(values, ",")))
				} else {
					selectorParts = append(selectorParts, fmt.Sprintf("%s %s", key, operator))
				}
			}
		}
		selectorStr := strings.Join(selectorParts, ", ")

		// Check if any pods match the selector (for matchLabels only - matchExpressions requires labelSelector conversion)
		// For matchExpressions, we need to build a proper label selector
		matchesPods := false
		if len(matchLabels) > 0 {
			matchesPods = s.checkPodsMatchSelector(ctx, namespace, matchLabels)
		}
		if !matchesPods && len(matchExpressions) > 0 {
			// Build label selector string for matchExpressions
			matchesPods = s.checkPodsMatchExpressions(ctx, namespace, matchExpressions)
		}

		if !matchesPods {
			findings = append(findings, DanglingFinding{
				CCVEID:      "CCVE-2025-0690",
				Category:    "ORPHAN",
				Severity:    "info",
				Kind:        "NetworkPolicy",
				Name:        name,
				Namespace:   namespace,
				TargetKind:  "Pod",
				TargetName:  selectorStr,
				Message:     fmt.Sprintf("NetworkPolicy podSelector matches no pods: %s", selectorStr),
				Remediation: "Verify pods with matching labels exist or update the NetworkPolicy",
				Command:     fmt.Sprintf("kubectl get pods -n %s --selector='%s'", namespace, s.buildLabelSelectorString(matchLabels, matchExpressions)),
			})
		}
	}

	return findings
}

// checkPodsMatchExpressions checks if any pods match the given matchExpressions
func (s *StateScanner) checkPodsMatchExpressions(ctx context.Context, namespace string, matchExpressions []interface{}) bool {
	// Build label selector from matchExpressions
	var selectorParts []string
	for _, expr := range matchExpressions {
		exprMap, ok := expr.(map[string]interface{})
		if !ok {
			continue
		}
		key, _, _ := unstructured.NestedString(exprMap, "key")
		operator, _, _ := unstructured.NestedString(exprMap, "operator")
		values, _, _ := unstructured.NestedStringSlice(exprMap, "values")

		if key == "" || operator == "" {
			continue
		}

		switch operator {
		case "In":
			selectorParts = append(selectorParts, fmt.Sprintf("%s in (%s)", key, strings.Join(values, ",")))
		case "NotIn":
			selectorParts = append(selectorParts, fmt.Sprintf("%s notin (%s)", key, strings.Join(values, ",")))
		case "Exists":
			selectorParts = append(selectorParts, key)
		case "DoesNotExist":
			selectorParts = append(selectorParts, fmt.Sprintf("!%s", key))
		}
	}

	if len(selectorParts) == 0 {
		return false
	}

	labelSelector := strings.Join(selectorParts, ",")

	// List pods with the label selector
	podList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}).Namespace(namespace).List(ctx, v1.ListOptions{
		LabelSelector: labelSelector,
	})

	if err != nil {
		return false
	}

	return len(podList.Items) > 0
}

// buildLabelSelectorString builds a kubectl-compatible label selector string
func (s *StateScanner) buildLabelSelectorString(matchLabels map[string]string, matchExpressions []interface{}) string {
	var parts []string

	// Add matchLabels
	for k, v := range matchLabels {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}

	// Add matchExpressions
	for _, expr := range matchExpressions {
		exprMap, ok := expr.(map[string]interface{})
		if !ok {
			continue
		}
		key, _, _ := unstructured.NestedString(exprMap, "key")
		operator, _, _ := unstructured.NestedString(exprMap, "operator")
		values, _, _ := unstructured.NestedStringSlice(exprMap, "values")

		if key == "" || operator == "" {
			continue
		}

		switch operator {
		case "In":
			parts = append(parts, fmt.Sprintf("%s in (%s)", key, strings.Join(values, ",")))
		case "NotIn":
			parts = append(parts, fmt.Sprintf("%s notin (%s)", key, strings.Join(values, ",")))
		case "Exists":
			parts = append(parts, key)
		case "DoesNotExist":
			parts = append(parts, fmt.Sprintf("!%s", key))
		}
	}

	return strings.Join(parts, ",")
}

// scanDanglingPVCs detects Pods that reference non-existent PersistentVolumeClaims
func (s *StateScanner) scanDanglingPVCs(ctx context.Context) []DanglingFinding {
	var findings []DanglingFinding

	// List all Pods
	podList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}).List(ctx, v1.ListOptions{})

	if err != nil {
		return findings
	}

	for _, pod := range podList.Items {
		name := pod.GetName()
		namespace := pod.GetNamespace()

		// Get volumes
		volumes, found, err := unstructured.NestedSlice(pod.Object, "spec", "volumes")
		if err != nil || !found {
			continue
		}

		for _, vol := range volumes {
			volMap, ok := vol.(map[string]interface{})
			if !ok {
				continue
			}

			// Check for persistentVolumeClaim volume source
			pvc, found, _ := unstructured.NestedMap(volMap, "persistentVolumeClaim")
			if !found {
				continue
			}

			claimName, _, _ := unstructured.NestedString(pvc, "claimName")
			if claimName == "" {
				continue
			}

			// Check if PVC exists
			if !s.checkPVCExists(ctx, namespace, claimName) {
				findings = append(findings, DanglingFinding{
					CCVEID:      "CCVE-2025-0693",
					Category:    "ORPHAN",
					Severity:    "high",
					Kind:        "Pod",
					Name:        name,
					Namespace:   namespace,
					TargetKind:  "PersistentVolumeClaim",
					TargetName:  claimName,
					Message:     fmt.Sprintf("Pod references non-existent PVC: %s", claimName),
					Remediation: "Create the missing PVC or update the Pod to remove the volume reference",
					Command:     fmt.Sprintf("kubectl get pvc %s -n %s", claimName, namespace),
				})
			}
		}
	}

	return findings
}

// checkPVCExists verifies if a PersistentVolumeClaim exists
func (s *StateScanner) checkPVCExists(ctx context.Context, namespace, name string) bool {
	_, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "persistentvolumeclaims",
	}).Namespace(namespace).Get(ctx, name, v1.GetOptions{})

	return err == nil
}

// scanDanglingSecrets detects Pods that reference non-existent Secrets
// Checks: volumes, envFrom, env secretKeyRef, and imagePullSecrets
func (s *StateScanner) scanDanglingSecrets(ctx context.Context) []DanglingFinding {
	var findings []DanglingFinding

	// List all Pods
	podList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}).List(ctx, v1.ListOptions{})

	if err != nil {
		return findings
	}

	// Build a cache of all Secrets by namespace
	secretCache := make(map[string]map[string]bool) // namespace -> secret name -> exists

	for _, pod := range podList.Items {
		name := pod.GetName()
		namespace := pod.GetNamespace()

		// Ensure we have the secrets for this namespace cached
		if _, ok := secretCache[namespace]; !ok {
			secretCache[namespace] = s.getSecretsInNamespace(ctx, namespace)
		}
		secrets := secretCache[namespace]

		// Check volume secrets
		volumes, found, _ := unstructured.NestedSlice(pod.Object, "spec", "volumes")
		if found {
			for _, vol := range volumes {
				volMap, ok := vol.(map[string]interface{})
				if !ok {
					continue
				}
				secret, found, _ := unstructured.NestedMap(volMap, "secret")
				if !found {
					continue
				}
				secretName, _, _ := unstructured.NestedString(secret, "secretName")
				if secretName != "" && !secrets[secretName] {
					// Check if secret is optional
					optional, _, _ := unstructured.NestedBool(secret, "optional")
					if !optional {
						findings = append(findings, DanglingFinding{
							CCVEID:      "CCVE-2025-0692",
							Category:    "ORPHAN",
							Severity:    "critical",
							Kind:        "Pod",
							Name:        name,
							Namespace:   namespace,
							TargetKind:  "Secret",
							TargetName:  secretName,
							Message:     fmt.Sprintf("Pod volume references non-existent Secret %q", secretName),
							Remediation: "Create the missing Secret or mark it as optional",
							Command:     fmt.Sprintf("kubectl create secret generic %s -n %s --from-literal=key=value", secretName, namespace),
						})
					}
				}
			}
		}

		// Check imagePullSecrets
		imagePullSecrets, found, _ := unstructured.NestedSlice(pod.Object, "spec", "imagePullSecrets")
		if found {
			for _, ips := range imagePullSecrets {
				ipsMap, ok := ips.(map[string]interface{})
				if !ok {
					continue
				}
				secretName, _, _ := unstructured.NestedString(ipsMap, "name")
				if secretName != "" && !secrets[secretName] {
					findings = append(findings, DanglingFinding{
						CCVEID:      "CCVE-2025-0692",
						Category:    "ORPHAN",
						Severity:    "critical",
						Kind:        "Pod",
						Name:        name,
						Namespace:   namespace,
						TargetKind:  "Secret",
						TargetName:  secretName,
						Message:     fmt.Sprintf("Pod imagePullSecret references non-existent Secret %q", secretName),
						Remediation: "Create the missing image pull Secret",
						Command:     fmt.Sprintf("kubectl create secret docker-registry %s -n %s --docker-server=REGISTRY --docker-username=USER --docker-password=PASS", secretName, namespace),
					})
				}
			}
		}

		// Check containers for envFrom and env secretKeyRef
		containers, found, _ := unstructured.NestedSlice(pod.Object, "spec", "containers")
		if found {
			findings = append(findings, s.checkContainerSecretRefs(name, namespace, containers, secrets)...)
		}

		// Check initContainers as well
		initContainers, found, _ := unstructured.NestedSlice(pod.Object, "spec", "initContainers")
		if found {
			findings = append(findings, s.checkContainerSecretRefs(name, namespace, initContainers, secrets)...)
		}
	}

	return findings
}

// checkContainerSecretRefs checks containers for envFrom and env secretKeyRef references
func (s *StateScanner) checkContainerSecretRefs(podName, namespace string, containers []interface{}, secrets map[string]bool) []DanglingFinding {
	var findings []DanglingFinding

	for _, c := range containers {
		container, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		// Check envFrom secretRef
		envFrom, found, _ := unstructured.NestedSlice(container, "envFrom")
		if found {
			for _, ef := range envFrom {
				efMap, ok := ef.(map[string]interface{})
				if !ok {
					continue
				}
				secretRef, found, _ := unstructured.NestedMap(efMap, "secretRef")
				if !found {
					continue
				}
				secretName, _, _ := unstructured.NestedString(secretRef, "name")
				if secretName != "" && !secrets[secretName] {
					// Check if optional
					optional, _, _ := unstructured.NestedBool(secretRef, "optional")
					if !optional {
						findings = append(findings, DanglingFinding{
							CCVEID:      "CCVE-2025-0692",
							Category:    "ORPHAN",
							Severity:    "critical",
							Kind:        "Pod",
							Name:        podName,
							Namespace:   namespace,
							TargetKind:  "Secret",
							TargetName:  secretName,
							Message:     fmt.Sprintf("Pod envFrom.secretRef references non-existent Secret %q", secretName),
							Remediation: "Create the missing Secret or mark it as optional",
							Command:     fmt.Sprintf("kubectl create secret generic %s -n %s --from-literal=key=value", secretName, namespace),
						})
					}
				}
			}
		}

		// Check env secretKeyRef
		envVars, found, _ := unstructured.NestedSlice(container, "env")
		if found {
			for _, ev := range envVars {
				evMap, ok := ev.(map[string]interface{})
				if !ok {
					continue
				}
				valueFrom, found, _ := unstructured.NestedMap(evMap, "valueFrom")
				if !found {
					continue
				}
				secretKeyRef, found, _ := unstructured.NestedMap(valueFrom, "secretKeyRef")
				if !found {
					continue
				}
				secretName, _, _ := unstructured.NestedString(secretKeyRef, "name")
				if secretName != "" && !secrets[secretName] {
					// Check if optional
					optional, _, _ := unstructured.NestedBool(secretKeyRef, "optional")
					if !optional {
						envName, _, _ := unstructured.NestedString(evMap, "name")
						findings = append(findings, DanglingFinding{
							CCVEID:      "CCVE-2025-0692",
							Category:    "ORPHAN",
							Severity:    "critical",
							Kind:        "Pod",
							Name:        podName,
							Namespace:   namespace,
							TargetKind:  "Secret",
							TargetName:  secretName,
							Message:     fmt.Sprintf("Pod env %q secretKeyRef references non-existent Secret %q", envName, secretName),
							Remediation: "Create the missing Secret or mark it as optional",
							Command:     fmt.Sprintf("kubectl create secret generic %s -n %s --from-literal=key=value", secretName, namespace),
						})
					}
				}
			}
		}
	}

	return findings
}

// getSecretsInNamespace returns a map of secret names that exist in the namespace
func (s *StateScanner) getSecretsInNamespace(ctx context.Context, namespace string) map[string]bool {
	secrets := make(map[string]bool)

	secretList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "secrets",
	}).Namespace(namespace).List(ctx, v1.ListOptions{})

	if err != nil {
		return secrets
	}

	for _, secret := range secretList.Items {
		secrets[secret.GetName()] = true
	}

	return secrets
}

// scanDanglingConfigMaps detects Pods that reference non-existent ConfigMaps
// Checks: volumes, envFrom configMapRef, and env configMapKeyRef
func (s *StateScanner) scanDanglingConfigMaps(ctx context.Context) []DanglingFinding {
	var findings []DanglingFinding

	// List all Pods
	podList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "pods",
	}).List(ctx, v1.ListOptions{})

	if err != nil {
		return findings
	}

	for _, pod := range podList.Items {
		name := pod.GetName()
		namespace := pod.GetNamespace()

		// Get all ConfigMaps in namespace for efficient lookup
		configMaps := s.getConfigMapsInNamespace(ctx, namespace)

		// Track ConfigMaps we've already reported for this Pod to avoid duplicates
		reportedConfigMaps := make(map[string]bool)

		// Check volumes for configMap references
		volumes, found, _ := unstructured.NestedSlice(pod.Object, "spec", "volumes")
		if found {
			for _, vol := range volumes {
				volMap, ok := vol.(map[string]interface{})
				if !ok {
					continue
				}

				// Check for configMap volume source
				cm, found, _ := unstructured.NestedMap(volMap, "configMap")
				if !found {
					continue
				}

				cmName, _, _ := unstructured.NestedString(cm, "name")
				if cmName == "" {
					continue
				}

				// Check if optional is set to true
				optional, _, _ := unstructured.NestedBool(cm, "optional")
				if optional {
					continue // Skip optional ConfigMap references
				}

				// Check if ConfigMap exists
				if !reportedConfigMaps[cmName] && !configMaps[cmName] {
					reportedConfigMaps[cmName] = true
					findings = append(findings, DanglingFinding{
						CCVEID:      "CCVE-2025-0691",
						Category:    "ORPHAN",
						Severity:    "high",
						Kind:        "Pod",
						Name:        name,
						Namespace:   namespace,
						TargetKind:  "ConfigMap",
						TargetName:  cmName,
						Message:     fmt.Sprintf("Pod volume references non-existent ConfigMap: %s", cmName),
						Remediation: "Create the missing ConfigMap or update the Pod to remove the reference",
						Command:     fmt.Sprintf("kubectl create configmap %s --from-literal=key=value -n %s", cmName, namespace),
					})
				}
			}
		}

		// Check containers for envFrom configMapRef and env configMapKeyRef
		containers, found, _ := unstructured.NestedSlice(pod.Object, "spec", "containers")
		if found {
			s.checkContainersForConfigMapRefs(ctx, containers, name, namespace, configMaps, reportedConfigMaps, &findings)
		}

		// Check initContainers for envFrom configMapRef and env configMapKeyRef
		initContainers, found, _ := unstructured.NestedSlice(pod.Object, "spec", "initContainers")
		if found {
			s.checkContainersForConfigMapRefs(ctx, initContainers, name, namespace, configMaps, reportedConfigMaps, &findings)
		}
	}

	return findings
}

// checkContainersForConfigMapRefs checks containers for ConfigMap references in envFrom and env
func (s *StateScanner) checkContainersForConfigMapRefs(ctx context.Context, containers []interface{}, podName, namespace string, configMaps, reportedConfigMaps map[string]bool, findings *[]DanglingFinding) {
	for _, container := range containers {
		containerMap, ok := container.(map[string]interface{})
		if !ok {
			continue
		}

		// Check envFrom for configMapRef
		envFrom, found, _ := unstructured.NestedSlice(containerMap, "envFrom")
		if found {
			for _, ef := range envFrom {
				efMap, ok := ef.(map[string]interface{})
				if !ok {
					continue
				}

				cmRef, found, _ := unstructured.NestedMap(efMap, "configMapRef")
				if !found {
					continue
				}

				cmName, _, _ := unstructured.NestedString(cmRef, "name")
				if cmName == "" {
					continue
				}

				// Check if optional is set to true
				optional, _, _ := unstructured.NestedBool(cmRef, "optional")
				if optional {
					continue // Skip optional ConfigMap references
				}

				if !reportedConfigMaps[cmName] && !configMaps[cmName] {
					reportedConfigMaps[cmName] = true
					*findings = append(*findings, DanglingFinding{
						CCVEID:      "CCVE-2025-0691",
						Category:    "ORPHAN",
						Severity:    "high",
						Kind:        "Pod",
						Name:        podName,
						Namespace:   namespace,
						TargetKind:  "ConfigMap",
						TargetName:  cmName,
						Message:     fmt.Sprintf("Pod envFrom references non-existent ConfigMap: %s", cmName),
						Remediation: "Create the missing ConfigMap or update the Pod to remove the reference",
						Command:     fmt.Sprintf("kubectl create configmap %s --from-literal=key=value -n %s", cmName, namespace),
					})
				}
			}
		}

		// Check env for configMapKeyRef
		envVars, found, _ := unstructured.NestedSlice(containerMap, "env")
		if found {
			for _, env := range envVars {
				envMap, ok := env.(map[string]interface{})
				if !ok {
					continue
				}

				valueFrom, found, _ := unstructured.NestedMap(envMap, "valueFrom")
				if !found {
					continue
				}

				cmKeyRef, found, _ := unstructured.NestedMap(valueFrom, "configMapKeyRef")
				if !found {
					continue
				}

				cmName, _, _ := unstructured.NestedString(cmKeyRef, "name")
				if cmName == "" {
					continue
				}

				// Check if optional is set to true
				optional, _, _ := unstructured.NestedBool(cmKeyRef, "optional")
				if optional {
					continue // Skip optional ConfigMap references
				}

				if !reportedConfigMaps[cmName] && !configMaps[cmName] {
					reportedConfigMaps[cmName] = true
					*findings = append(*findings, DanglingFinding{
						CCVEID:      "CCVE-2025-0691",
						Category:    "ORPHAN",
						Severity:    "high",
						Kind:        "Pod",
						Name:        podName,
						Namespace:   namespace,
						TargetKind:  "ConfigMap",
						TargetName:  cmName,
						Message:     fmt.Sprintf("Pod env references non-existent ConfigMap: %s", cmName),
						Remediation: "Create the missing ConfigMap or update the Pod to remove the reference",
						Command:     fmt.Sprintf("kubectl create configmap %s --from-literal=key=value -n %s", cmName, namespace),
					})
				}
			}
		}
	}
}

// getConfigMapsInNamespace returns a map of configmap names that exist in the namespace
func (s *StateScanner) getConfigMapsInNamespace(ctx context.Context, namespace string) map[string]bool {
	configMaps := make(map[string]bool)

	cmList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}).Namespace(namespace).List(ctx, v1.ListOptions{})

	if err != nil {
		return configMaps
	}

	for _, cm := range cmList.Items {
		configMaps[cm.GetName()] = true
	}

	return configMaps
}

// scanDanglingVPAs detects VerticalPodAutoscalers targeting non-existent workloads
// This covers CCVE-2025-0941 (Dangling VPA)
func (s *StateScanner) scanDanglingVPAs(ctx context.Context) []DanglingFinding {
	var findings []DanglingFinding

	// VPA uses the autoscaling.k8s.io API group
	vpaList, err := s.client.Resource(schema.GroupVersionResource{
		Group:    "autoscaling.k8s.io",
		Version:  "v1",
		Resource: "verticalpodautoscalers",
	}).List(ctx, v1.ListOptions{})

	if err != nil {
		// VPA CRD not installed or no access, skip
		return findings
	}

	for _, vpa := range vpaList.Items {
		name := vpa.GetName()
		namespace := vpa.GetNamespace()

		// Get target reference
		targetRef, found, err := unstructured.NestedMap(vpa.Object, "spec", "targetRef")
		if err != nil || !found {
			continue
		}

		targetKind, _, _ := unstructured.NestedString(targetRef, "kind")
		targetName, _, _ := unstructured.NestedString(targetRef, "name")
		targetAPIVersion, _, _ := unstructured.NestedString(targetRef, "apiVersion")

		if targetKind == "" || targetName == "" {
			continue
		}

		// Check if target exists using the existing helper
		if !s.checkScaleTargetExists(ctx, namespace, targetKind, targetName, targetAPIVersion) {
			findings = append(findings, DanglingFinding{
				CCVEID:      "CCVE-2025-0941",
				Category:    "ORPHAN",
				Severity:    "warning",
				Kind:        "VerticalPodAutoscaler",
				Name:        name,
				Namespace:   namespace,
				TargetKind:  targetKind,
				TargetName:  targetName,
				Message:     fmt.Sprintf("VPA targets non-existent %s/%s", targetKind, targetName),
				Remediation: "Delete the orphaned VPA or create the missing target workload",
				Command:     fmt.Sprintf("kubectl delete vpa %s -n %s", name, namespace),
			})
		}
	}

	return findings
}
