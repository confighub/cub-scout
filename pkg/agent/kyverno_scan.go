// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// KyvernoScanner scans for Kyverno policy violations
type KyvernoScanner struct {
	client      dynamic.Interface
	policyDBDir string
}

// KyvernoPolicy represents a KPOL policy from our database
type KyvernoPolicy struct {
	ID          string `yaml:"id" json:"id"`
	Type        string `yaml:"type" json:"type"`
	Category    string `yaml:"category" json:"category"`
	Name        string `yaml:"name" json:"name"`
	Severity    string `yaml:"severity" json:"severity"`
	Confidence  string `yaml:"confidence" json:"confidence"`
	DerivedFrom struct {
		Source            string `yaml:"source" json:"source"`
		PolicyName        string `yaml:"policy_name" json:"policy_name"`
		URL               string `yaml:"url" json:"url"`
		Category          string `yaml:"category" json:"category"`
		MinKyvernoVersion string `yaml:"min_kyverno_version" json:"min_kyverno_version"`
	} `yaml:"derived_from" json:"derived_from"`
	Detection struct {
		Resources []string `yaml:"resources" json:"resources"`
		Condition string   `yaml:"condition" json:"condition"`
	} `yaml:"detection" json:"detection"`
	RootCause   string `yaml:"root_cause" json:"root_cause"`
	Remediation struct {
		Steps []string `yaml:"steps" json:"steps"`
	} `yaml:"remediation" json:"remediation"`
	Tags []string `yaml:"tags" json:"tags"`
}

// ScanResult represents the result of a Kyverno scan
type ScanResult struct {
	ClusterName string        `json:"clusterName"`
	ScannedAt   time.Time     `json:"scannedAt"`
	Summary     ScanSummary   `json:"summary"`
	Findings    []ScanFinding `json:"findings"`
	Error       string        `json:"error,omitempty"`
}

// ScanSummary contains counts by severity
type ScanSummary struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
	Pass     int `json:"pass"`
}

// ScanFinding represents a single policy violation
type ScanFinding struct {
	ID         string        `json:"id"`
	PolicyID   string        `json:"policyId,omitempty"` // Our KPOL-* ID if matched
	PolicyName string        `json:"policyName"`
	Category   string        `json:"category"`
	Severity   string        `json:"severity"`
	Resource   string        `json:"resource"`
	Namespace  string        `json:"namespace"`
	Message    string        `json:"message"`
	Result     string        `json:"result"` // fail, warn, pass, skip
	Rule       string        `json:"rule,omitempty"`
	ConfigHub  *ConfigHubRef `json:"confighub,omitempty"`
}

// ConfigHubRef contains ConfigHub-specific references for the finding
type ConfigHubRef struct {
	UnitSlug       string `json:"unitSlug,omitempty"`
	SpaceID        string `json:"spaceId,omitempty"`
	TargetID       string `json:"targetId,omitempty"`
	RemediationURL string `json:"remediationUrl,omitempty"`
}

// NewKyvernoScanner creates a new Kyverno scanner
func NewKyvernoScanner(config *rest.Config, policyDBDir string) (*KyvernoScanner, error) {
	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	return &KyvernoScanner{
		client:      client,
		policyDBDir: policyDBDir,
	}, nil
}

// NewKyvernoScannerWithClient creates a scanner with an existing client
func NewKyvernoScannerWithClient(client dynamic.Interface, policyDBDir string) *KyvernoScanner {
	return &KyvernoScanner{
		client:      client,
		policyDBDir: policyDBDir,
	}
}

// Available checks if Kyverno is installed in the cluster
func (s *KyvernoScanner) Available(ctx context.Context) bool {
	// Check for PolicyReport CRD
	gvr := schema.GroupVersionResource{
		Group:    "wgpolicyk8s.io",
		Version:  "v1alpha2",
		Resource: "policyreports",
	}

	_, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{Limit: 1})
	return err == nil
}

// Scan performs a full Kyverno policy scan
func (s *KyvernoScanner) Scan(ctx context.Context) (*ScanResult, error) {
	result := &ScanResult{
		ScannedAt: time.Now(),
		Findings:  []ScanFinding{},
	}

	// Check if Kyverno is available
	if !s.Available(ctx) {
		result.Error = "Kyverno not installed or PolicyReport CRD not found"
		return result, nil
	}

	// Load our policy database for matching
	policyDB, err := s.loadPolicyDB()
	if err != nil {
		// Continue without policy matching
		policyDB = make(map[string]*KyvernoPolicy)
	}

	// Get all PolicyReports (namespaced)
	policyReports, err := s.getPolicyReports(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get PolicyReports: %w", err)
	}

	// Get ClusterPolicyReports (cluster-scoped)
	clusterReports, err := s.getClusterPolicyReports(ctx)
	if err != nil {
		// Continue without cluster reports
		clusterReports = []unstructured.Unstructured{}
	}

	// Process all reports
	allReports := append(policyReports, clusterReports...)
	for _, report := range allReports {
		findings := s.processPolicyReport(report, policyDB)
		result.Findings = append(result.Findings, findings...)
	}

	// Calculate summary
	for _, f := range result.Findings {
		switch strings.ToLower(f.Severity) {
		case "critical", "high":
			result.Summary.Critical++
		case "warning", "medium":
			result.Summary.Warning++
		case "info", "low":
			result.Summary.Info++
		}
		if f.Result == "pass" {
			result.Summary.Pass++
		}
	}

	return result, nil
}

// ScanNamespace scans a specific namespace
func (s *KyvernoScanner) ScanNamespace(ctx context.Context, namespace string) (*ScanResult, error) {
	result := &ScanResult{
		ScannedAt: time.Now(),
		Findings:  []ScanFinding{},
	}

	if !s.Available(ctx) {
		result.Error = "Kyverno not installed or PolicyReport CRD not found"
		return result, nil
	}

	// Load policy database
	policyDB, _ := s.loadPolicyDB()
	if policyDB == nil {
		policyDB = make(map[string]*KyvernoPolicy)
	}

	// Get PolicyReports for specific namespace
	gvr := schema.GroupVersionResource{
		Group:    "wgpolicyk8s.io",
		Version:  "v1alpha2",
		Resource: "policyreports",
	}

	list, err := s.client.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list PolicyReports in namespace %s: %w", namespace, err)
	}

	for _, report := range list.Items {
		findings := s.processPolicyReport(report, policyDB)
		result.Findings = append(result.Findings, findings...)
	}

	// Calculate summary
	for _, f := range result.Findings {
		switch strings.ToLower(f.Severity) {
		case "critical", "high":
			result.Summary.Critical++
		case "warning", "medium":
			result.Summary.Warning++
		case "info", "low":
			result.Summary.Info++
		}
	}

	return result, nil
}

// getPolicyReports fetches all PolicyReports
func (s *KyvernoScanner) getPolicyReports(ctx context.Context) ([]unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{
		Group:    "wgpolicyk8s.io",
		Version:  "v1alpha2",
		Resource: "policyreports",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return list.Items, nil
}

// getClusterPolicyReports fetches all ClusterPolicyReports
func (s *KyvernoScanner) getClusterPolicyReports(ctx context.Context) ([]unstructured.Unstructured, error) {
	gvr := schema.GroupVersionResource{
		Group:    "wgpolicyk8s.io",
		Version:  "v1alpha2",
		Resource: "clusterpolicyreports",
	}

	list, err := s.client.Resource(gvr).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return list.Items, nil
}

// processPolicyReport extracts findings from a PolicyReport
func (s *KyvernoScanner) processPolicyReport(report unstructured.Unstructured, policyDB map[string]*KyvernoPolicy) []ScanFinding {
	var findings []ScanFinding

	results, found, err := unstructured.NestedSlice(report.Object, "results")
	if err != nil || !found {
		return findings
	}

	namespace := report.GetNamespace()

	for _, r := range results {
		resultMap, ok := r.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract fields
		policy, _ := resultMap["policy"].(string)
		rule, _ := resultMap["rule"].(string)
		result, _ := resultMap["result"].(string)
		message, _ := resultMap["message"].(string)
		severity, _ := resultMap["severity"].(string)
		category, _ := resultMap["category"].(string)

		// Extract resource reference
		var resourceStr string
		if resources, ok := resultMap["resources"].([]interface{}); ok && len(resources) > 0 {
			if res, ok := resources[0].(map[string]interface{}); ok {
				kind, _ := res["kind"].(string)
				name, _ := res["name"].(string)
				ns, _ := res["namespace"].(string)
				if ns == "" {
					ns = namespace
				}
				resourceStr = fmt.Sprintf("%s/%s", kind, name)
				if ns != "" {
					namespace = ns
				}
			}
		}

		// Only include failures and warnings (skip pass/skip)
		if result != "fail" && result != "warn" {
			continue
		}

		finding := ScanFinding{
			ID:         fmt.Sprintf("%s/%s", policy, rule),
			PolicyName: policy,
			Category:   category,
			Severity:   normalizeSeverity(severity, result),
			Resource:   resourceStr,
			Namespace:  namespace,
			Message:    message,
			Result:     result,
			Rule:       rule,
		}

		// Try to match to our KPOL database
		if kpol := s.matchPolicy(policy, policyDB); kpol != nil {
			finding.PolicyID = kpol.ID
			finding.Category = kpol.Category
			if kpol.Severity != "" {
				finding.Severity = kpol.Severity
			}
		}

		findings = append(findings, finding)
	}

	return findings
}

// matchPolicy tries to find a matching KPOL for a Kyverno policy name
func (s *KyvernoScanner) matchPolicy(policyName string, policyDB map[string]*KyvernoPolicy) *KyvernoPolicy {
	// Normalize policy name
	normalized := strings.ToLower(strings.ReplaceAll(policyName, "-", "_"))

	for _, kpol := range policyDB {
		derivedName := strings.ToLower(strings.ReplaceAll(kpol.DerivedFrom.PolicyName, "-", "_"))
		if derivedName == normalized || strings.Contains(normalized, derivedName) {
			return kpol
		}
	}
	return nil
}

// loadPolicyDB loads KPOL policies from the database directory
func (s *KyvernoScanner) loadPolicyDB() (map[string]*KyvernoPolicy, error) {
	db := make(map[string]*KyvernoPolicy)

	if s.policyDBDir == "" {
		return db, nil
	}

	pattern := filepath.Join(s.policyDBDir, "KPOL-*.yaml")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var policy KyvernoPolicy
		if err := yaml.Unmarshal(data, &policy); err != nil {
			continue
		}

		db[policy.ID] = &policy
	}

	return db, nil
}

// normalizeSeverity normalizes severity from Kyverno to our format
func normalizeSeverity(severity, result string) string {
	if severity != "" {
		switch strings.ToLower(severity) {
		case "critical", "high":
			return "critical"
		case "medium":
			return "warning"
		case "low", "info":
			return "info"
		}
		return severity
	}

	// Infer from result
	if result == "fail" {
		return "warning"
	}
	return "info"
}

// GetPolicyCatalog returns all KPOL policies from the database
func (s *KyvernoScanner) GetPolicyCatalog() ([]*KyvernoPolicy, error) {
	db, err := s.loadPolicyDB()
	if err != nil {
		return nil, err
	}

	policies := make([]*KyvernoPolicy, 0, len(db))
	for _, p := range db {
		policies = append(policies, p)
	}
	return policies, nil
}

// ToJSON returns the scan result as JSON
func (r *ScanResult) ToJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
