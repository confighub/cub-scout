// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// LifecycleHazardScanner detects GitOps lifecycle hazards in Kubernetes resources.
// It focuses on Helm hook semantics under ArgoCD, where hooks are mapped differently
// than native Helm behavior.
type LifecycleHazardScanner struct {
	client dynamic.Interface
}

// NewLifecycleHazardScanner creates a scanner for lifecycle hazards.
func NewLifecycleHazardScanner(cfg *rest.Config) (*LifecycleHazardScanner, error) {
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	return &LifecycleHazardScanner{client: client}, nil
}

// NewLifecycleHazardScannerFromObjects creates a scanner for offline analysis.
func NewLifecycleHazardScannerFromObjects(objects []*unstructured.Unstructured) *LifecycleHazardScanner {
	return &LifecycleHazardScanner{}
}

// LifecycleHazardResult contains findings from lifecycle hazard scanning.
type LifecycleHazardResult struct {
	ScannedAt time.Time                `json:"scannedAt"`
	Findings  []LifecycleHazardFinding `json:"findings"`
	Summary   LifecycleHazardSummary   `json:"summary"`
}

// LifecycleHazardSummary counts findings by rule.
type LifecycleHazardSummary struct {
	HelmHookAmbiguity     int `json:"helmHookAmbiguity"`
	PreSyncDependencyRace int `json:"preSyncDependencyRace"`
	PostSyncIdempotency   int `json:"postSyncIdempotency"`
	Total                 int `json:"total"`
}

// LifecycleHazardFinding represents a detected lifecycle hazard.
type LifecycleHazardFinding struct {
	// Rule identifies the hazard type
	Rule string `json:"rule"`

	// Resource identifies the affected resource
	Resource string `json:"resource"`

	// Namespace of the resource
	Namespace string `json:"namespace,omitempty"`

	// Severity: warning or critical
	Severity string `json:"severity"`

	// Hooks lists the hook annotations found
	Hooks []string `json:"hooks,omitempty"`

	// MappedPhase is the ArgoCD phase this maps to
	MappedPhase string `json:"mappedPhase,omitempty"`

	// Risk describes the potential issue
	Risk string `json:"risk"`

	// Remediation suggests how to fix
	Remediation string `json:"remediation"`

	// Evidence provides supporting details
	Evidence map[string]string `json:"evidence,omitempty"`
}

// Lifecycle hazard rule constants
const (
	RuleHelmHookAmbiguity     = "helm-hook-ambiguity"
	RulePreSyncDependencyRace = "presync-dependency-race"
	RulePostSyncIdempotency   = "postsync-idempotency-risk"
)

// Helm hook annotation keys
const (
	HelmHookAnnotation             = "helm.sh/hook"
	HelmHookWeightAnnotation       = "helm.sh/hook-weight"
	HelmHookDeletePolicyAnnotation = "helm.sh/hook-delete-policy"
	ArgoSyncWaveAnnotation         = "argocd.argoproj.io/sync-wave"
	ArgoHookAnnotation             = "argocd.argoproj.io/hook"
)

// Helm hook values
var helmHookPhases = map[string]string{
	"pre-install":   "PreSync",
	"post-install":  "PostSync",
	"pre-upgrade":   "PreSync",
	"post-upgrade":  "PostSync",
	"pre-rollback":  "PreSync",
	"post-rollback": "PostSync",
	"pre-delete":    "PreSync",
	"post-delete":   "PostSync",
	"test":          "Skip", // Tests are not run by Argo
	"test-success":  "Skip",
	"test-failure":  "Skip",
}

// ScanObjects analyzes a set of objects for lifecycle hazards.
// This is the primary entry point for offline/bundle analysis.
func (s *LifecycleHazardScanner) ScanObjects(objects []*unstructured.Unstructured) *LifecycleHazardResult {
	result := &LifecycleHazardResult{
		ScannedAt: time.Now().UTC(),
		Findings:  []LifecycleHazardFinding{},
	}

	for _, obj := range objects {
		findings := s.analyzeObject(obj)
		result.Findings = append(result.Findings, findings...)
	}

	// Sort findings for deterministic output
	sort.Slice(result.Findings, func(i, j int) bool {
		if result.Findings[i].Rule != result.Findings[j].Rule {
			return result.Findings[i].Rule < result.Findings[j].Rule
		}
		if result.Findings[i].Namespace != result.Findings[j].Namespace {
			return result.Findings[i].Namespace < result.Findings[j].Namespace
		}
		return result.Findings[i].Resource < result.Findings[j].Resource
	})

	// Build summary
	for _, f := range result.Findings {
		switch f.Rule {
		case RuleHelmHookAmbiguity:
			result.Summary.HelmHookAmbiguity++
		case RulePreSyncDependencyRace:
			result.Summary.PreSyncDependencyRace++
		case RulePostSyncIdempotency:
			result.Summary.PostSyncIdempotency++
		}
		result.Summary.Total++
	}

	return result
}

// Scan analyzes cluster resources for lifecycle hazards.
func (s *LifecycleHazardScanner) Scan(ctx context.Context) (*LifecycleHazardResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("no cluster client configured")
	}

	// Collect objects from cluster
	objects, err := s.collectClusterObjects(ctx)
	if err != nil {
		return nil, err
	}

	return s.ScanObjects(objects), nil
}

// ScanNamespace analyzes resources in a specific namespace.
func (s *LifecycleHazardScanner) ScanNamespace(ctx context.Context, namespace string) (*LifecycleHazardResult, error) {
	if s.client == nil {
		return nil, fmt.Errorf("no cluster client configured")
	}

	objects, err := s.collectNamespaceObjects(ctx, namespace)
	if err != nil {
		return nil, err
	}

	return s.ScanObjects(objects), nil
}

func (s *LifecycleHazardScanner) analyzeObject(obj *unstructured.Unstructured) []LifecycleHazardFinding {
	var findings []LifecycleHazardFinding

	annotations := obj.GetAnnotations()
	if annotations == nil {
		return findings
	}

	// Check for Helm hook annotations
	helmHook, hasHelmHook := annotations[HelmHookAnnotation]
	if !hasHelmHook {
		return findings
	}

	resourceID := fmt.Sprintf("%s/%s", obj.GetKind(), obj.GetName())

	// Rule 1: Helm hook ambiguity (comma-separated hooks)
	if finding := s.checkHelmHookAmbiguity(obj, helmHook, resourceID); finding != nil {
		findings = append(findings, *finding)
	}

	// Rule 2: PostSync idempotency risk
	if finding := s.checkPostSyncIdempotency(obj, helmHook, resourceID, annotations); finding != nil {
		findings = append(findings, *finding)
	}

	return findings
}

// checkHelmHookAmbiguity detects comma-separated helm.sh/hook values
// which are mapped to a single ArgoCD phase, potentially changing behavior.
func (s *LifecycleHazardScanner) checkHelmHookAmbiguity(obj *unstructured.Unstructured, helmHook, resourceID string) *LifecycleHazardFinding {
	hooks := parseHelmHooks(helmHook)
	if len(hooks) <= 1 {
		return nil
	}

	// Check if hooks map to different phases
	phases := make(map[string]bool)
	for _, h := range hooks {
		if phase, ok := helmHookPhases[h]; ok {
			phases[phase] = true
		}
	}

	// If all hooks map to the same phase, less risk
	// But comma-separated hooks still indicate Helm-style usage
	mappedPhase := getMappedPhase(hooks)

	return &LifecycleHazardFinding{
		Rule:        RuleHelmHookAmbiguity,
		Resource:    resourceID,
		Namespace:   obj.GetNamespace(),
		Severity:    "warning",
		Hooks:       hooks,
		MappedPhase: mappedPhase,
		Risk:        "ArgoCD maps comma-separated Helm hooks to a single phase; behavior may differ from native Helm",
		Remediation: "Split into separate resources with explicit argocd.argoproj.io/hook annotations, or convert to ArgoCD Sync hook",
		Evidence: map[string]string{
			"annotation": fmt.Sprintf("%s=%s", HelmHookAnnotation, helmHook),
		},
	}
}

// checkPostSyncIdempotency detects Jobs with hook-delete-policy that may rerun on drift.
func (s *LifecycleHazardScanner) checkPostSyncIdempotency(obj *unstructured.Unstructured, helmHook, resourceID string, annotations map[string]string) *LifecycleHazardFinding {
	// Only check Jobs
	if obj.GetKind() != "Job" {
		return nil
	}

	// Check if this is a post-* hook
	hooks := parseHelmHooks(helmHook)
	isPostHook := false
	for _, h := range hooks {
		if strings.HasPrefix(h, "post-") {
			isPostHook = true
			break
		}
	}
	if !isPostHook {
		return nil
	}

	// Check delete policy
	deletePolicy := annotations[HelmHookDeletePolicyAnnotation]
	if deletePolicy == "" {
		return nil
	}

	// before-hook-creation means the Job will be deleted and recreated on each sync
	if strings.Contains(deletePolicy, "before-hook-creation") {
		return &LifecycleHazardFinding{
			Rule:        RulePostSyncIdempotency,
			Resource:    resourceID,
			Namespace:   obj.GetNamespace(),
			Severity:    "warning",
			Hooks:       hooks,
			MappedPhase: "PostSync",
			Risk:        "Job with before-hook-creation will rerun on every sync; non-idempotent operations may fail",
			Remediation: "Ensure Job operations are idempotent, or use hook-succeeded/hook-failed delete policies",
			Evidence: map[string]string{
				"deletePolicy": deletePolicy,
			},
		}
	}

	return nil
}

func parseHelmHooks(hookAnnotation string) []string {
	parts := strings.Split(hookAnnotation, ",")
	var hooks []string
	for _, p := range parts {
		h := strings.TrimSpace(p)
		if h != "" {
			hooks = append(hooks, h)
		}
	}
	sort.Strings(hooks)
	return hooks
}

func getMappedPhase(hooks []string) string {
	// Determine the dominant phase
	preCount := 0
	postCount := 0
	for _, h := range hooks {
		phase := helmHookPhases[h]
		if phase == "PreSync" {
			preCount++
		} else if phase == "PostSync" {
			postCount++
		}
	}

	if preCount > 0 && postCount > 0 {
		return "Mixed (PreSync+PostSync)"
	}
	if preCount > 0 {
		return "PreSync"
	}
	if postCount > 0 {
		return "PostSync"
	}
	return "Unknown"
}

func (s *LifecycleHazardScanner) collectClusterObjects(ctx context.Context) ([]*unstructured.Unstructured, error) {
	resources := []schema.GroupVersionResource{
		{Group: "batch", Version: "v1", Resource: "jobs"},
		{Group: "batch", Version: "v1", Resource: "cronjobs"},
		{Group: "", Version: "v1", Resource: "pods"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "", Version: "v1", Resource: "serviceaccounts"},
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
	}

	var objects []*unstructured.Unstructured
	for _, gvr := range resources {
		list, err := s.client.Resource(gvr).List(ctx, metav1.ListOptions{})
		if err != nil {
			// Best-effort scanning across clusters with variable CRDs/RBAC.
			continue
		}
		for i := range list.Items {
			item := list.Items[i].DeepCopy()
			annotations := item.GetAnnotations()
			if annotations == nil || annotations[HelmHookAnnotation] == "" {
				continue
			}
			objects = append(objects, item)
		}
	}

	return objects, nil
}

func (s *LifecycleHazardScanner) collectNamespaceObjects(ctx context.Context, namespace string) ([]*unstructured.Unstructured, error) {
	resources := []schema.GroupVersionResource{
		{Group: "batch", Version: "v1", Resource: "jobs"},
		{Group: "batch", Version: "v1", Resource: "cronjobs"},
		{Group: "", Version: "v1", Resource: "pods"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "", Version: "v1", Resource: "serviceaccounts"},
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
	}

	var objects []*unstructured.Unstructured
	for _, gvr := range resources {
		list, err := s.client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			// Best-effort scanning across clusters with variable CRDs/RBAC.
			continue
		}
		for i := range list.Items {
			item := list.Items[i].DeepCopy()
			annotations := item.GetAnnotations()
			if annotations == nil || annotations[HelmHookAnnotation] == "" {
				continue
			}
			objects = append(objects, item)
		}
	}

	return objects, nil
}

// RenderLifecycleHazardsASCII produces human-readable output.
func RenderLifecycleHazardsASCII(result *LifecycleHazardResult) string {
	var b strings.Builder

	b.WriteString("GitOps Lifecycle Hazards\n")
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n\n")

	if result.Summary.Total == 0 {
		b.WriteString("No lifecycle hazards detected.\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("Scanned: %s\n", result.ScannedAt.Format("2006-01-02 15:04:05 UTC")))
	b.WriteString(fmt.Sprintf("Total:   %d finding(s)\n\n", result.Summary.Total))

	// Group by rule
	byRule := make(map[string][]LifecycleHazardFinding)
	for _, f := range result.Findings {
		byRule[f.Rule] = append(byRule[f.Rule], f)
	}

	ruleOrder := []string{RuleHelmHookAmbiguity, RulePreSyncDependencyRace, RulePostSyncIdempotency}
	ruleNames := map[string]string{
		RuleHelmHookAmbiguity:     "Helm Hook Ambiguity",
		RulePreSyncDependencyRace: "PreSync Dependency Race",
		RulePostSyncIdempotency:   "PostSync Idempotency Risk",
	}

	for _, rule := range ruleOrder {
		findings := byRule[rule]
		if len(findings) == 0 {
			continue
		}

		b.WriteString(fmt.Sprintf("%s (%d)\n", ruleNames[rule], len(findings)))
		b.WriteString(strings.Repeat("─", 40))
		b.WriteString("\n")

		for _, f := range findings {
			icon := "⚠"
			if f.Severity == "critical" {
				icon = "✗"
			}

			b.WriteString(fmt.Sprintf("\n%s %s", icon, f.Resource))
			if f.Namespace != "" {
				b.WriteString(fmt.Sprintf(" (ns: %s)", f.Namespace))
			}
			b.WriteString("\n")

			if len(f.Hooks) > 0 {
				b.WriteString(fmt.Sprintf("  Hooks: %s\n", strings.Join(f.Hooks, ", ")))
			}
			if f.MappedPhase != "" {
				b.WriteString(fmt.Sprintf("  Phase: %s\n", f.MappedPhase))
			}
			b.WriteString(fmt.Sprintf("  Risk: %s\n", f.Risk))
			b.WriteString(fmt.Sprintf("  Fix:  %s\n", f.Remediation))
		}
		b.WriteString("\n")
	}

	return b.String()
}
