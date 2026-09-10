// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/confighub/cub-scout/pkg/agent"
)

var (
	gitopsNamespace string
	gitopsJSON      bool
	gitopsFormat    string

	gitopsWithConfigHub       bool
	gitopsConfigHubSpace      string
	gitopsConfigHubSince      string
	gitopsConfigHubStaleAfter string
)

var gitopsCmd = &cobra.Command{
	Use:   "gitops",
	Short: "GitOps status and diagnostics",
	Long: `GitOps status and diagnostics for Flux, Argo CD, Sveltos, and Modelplane.

Shows the health of your GitOps pipeline including:
- Detected backend/controller family (Flux, Argo CD, Sveltos, Modelplane)
- Transport mode (OCI, Git, Helm)
- Deployer status (Kustomization, HelmRelease, Application, Sveltos and Modelplane controller resources)
- Source status (OCIRepository, GitRepository)
- Failure stage and reason when things go wrong

Examples:
  # Show GitOps status for all namespaces
  cub-scout gitops status

  # Show GitOps status for a specific namespace
  cub-scout gitops status -n flux-system

  # Output as JSON
  cub-scout gitops status --format json

  # Include bounded ConfigHub release/event/live-status evidence
  cub-scout gitops status --with-confighub --confighub-space prod --confighub-since 24h
`,
}

var gitopsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show GitOps pipeline status",
	Long: `Show the status of GitOps/controller deployers and sources in the cluster.

This command helps diagnose delegated apply issues by showing:
- Backend: Which GitOps/controller family is managing resources (Flux, Argo CD, Sveltos, Modelplane)
- Transport: How manifests are delivered (OCI, Git, Helm)
- Controller coverage: Which controller families were checked, observed, or unreadable
- ConfigHub target: If resources are managed by ConfigHub
- Failing stage: Where in the pipeline things are breaking (source, build, apply, sync)
- Reason/Message: The specific error causing the failure

Examples:
  # Quick health check
  cub-scout gitops status

  # Check specific namespace
  cub-scout gitops status -n production

  # Machine-readable output
  cub-scout gitops status --format json

  # Include bounded ConfigHub release/event/live-status evidence
  cub-scout gitops status --with-confighub --confighub-space prod --confighub-since 24h
`,
	RunE: runGitOpsStatus,
}

func init() {
	rootCmd.AddCommand(gitopsCmd)
	gitopsCmd.AddCommand(gitopsStatusCmd)

	gitopsStatusCmd.Flags().StringVarP(&gitopsNamespace, "namespace", "n", "", "Namespace to scan (default: all namespaces)")
	gitopsStatusCmd.Flags().StringVar(&gitopsFormat, "format", "ascii", "Output format: ascii, json, md")
	gitopsStatusCmd.Flags().BoolVar(&gitopsJSON, "json", false, "Output as JSON (shorthand for --format json)")
	gitopsStatusCmd.Flags().BoolVar(&gitopsWithConfigHub, "with-confighub", false, "Include bounded ConfigHub release, unit-event, and live-status evidence")
	gitopsStatusCmd.Flags().StringVar(&gitopsConfigHubSpace, "confighub-space", "", "ConfigHub space for connected evidence (default: current cub space; use '*' explicitly for all spaces)")
	gitopsStatusCmd.Flags().StringVar(&gitopsConfigHubSince, "confighub-since", "24h", "Lookback window for ConfigHub release/event evidence (examples: 24h, 7d, 2w)")
	gitopsStatusCmd.Flags().StringVar(&gitopsConfigHubStaleAfter, "confighub-stale-after", "15m", "Treat ConfigHub live-status observations older than this as stale")
}

// GitOpsSummary holds the summary of GitOps status for output
type GitOpsSummary struct {
	// Backend is the detected GitOps backend: flux, argocd, worker, none
	Backend string `json:"backend"`

	// Transport is the source transport: oci, git, helm, unknown
	Transport string `json:"transport"`

	// ConfigHubTarget is present if ConfigHub worker is detected
	ConfigHubTarget *agent.ConfigHubTarget `json:"confighubTarget,omitempty"`

	// Deployers are the detected deployer resources
	Deployers []DeployerStatus `json:"deployers,omitempty"`

	// Sources are the detected source resources
	Sources []SourceStatus `json:"sources,omitempty"`

	// ControllerCoverage records which controller families were checked.
	ControllerCoverage []ControllerCoverageStatus `json:"controllerCoverage,omitempty"`

	// HealthyCount is the number of healthy deployers
	HealthyCount int `json:"healthyCount"`

	// FailedCount is the number of failed deployers
	FailedCount int `json:"failedCount"`

	// DeliveryEvidence is optional connected evidence from ConfigHub and event consumers.
	DeliveryEvidence *GitOpsDeliveryEvidence `json:"deliveryEvidence,omitempty"`
}

// ControllerCoverageStatus reports whether a GitOps/controller family was
// found, absent, partially readable, or unreadable with the current RBAC.
type ControllerCoverageStatus struct {
	Family        string                       `json:"family"`
	Status        string                       `json:"status"`
	ResourceKinds []string                     `json:"resourceKinds,omitempty"`
	FoundKinds    []string                     `json:"foundKinds,omitempty"`
	Found         int                          `json:"found"`
	Omissions     []ControllerCoverageOmission `json:"omissions,omitempty"`
}

// ControllerCoverageOmission is a safe, structured explanation for why a
// controller-family resource could not be inspected.
type ControllerCoverageOmission struct {
	Resource string `json:"resource"`
	Reason   string `json:"reason"`
	Message  string `json:"message,omitempty"`
}

const (
	controllerCoverageFound      = "found"
	controllerCoverageNotFound   = "not_found"
	controllerCoveragePartial    = "partial"
	controllerCoverageUnreadable = "unreadable"
)

// DeployerStatus holds the status of a single deployer
type DeployerStatus struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Owner     string `json:"owner,omitempty"`
	Ready     bool   `json:"ready"`
	Suspended bool   `json:"suspended,omitempty"`

	// Stage indicates where failure occurred: source, build, apply, sync, healthy
	Stage string `json:"stage"`

	// Reason is the condition reason
	Reason string `json:"reason,omitempty"`

	// Message is the human-readable error message
	Message string `json:"message,omitempty"`

	// SourceRef is the key of the source this deployer references
	SourceRef string `json:"sourceRef,omitempty"`

	// Argo-specific fields
	SyncStatus    string         `json:"syncStatus,omitempty"`
	HealthStatus  string         `json:"healthStatus,omitempty"`
	PodReady      int            `json:"podReady,omitempty"`
	PodTotal      int            `json:"podTotal,omitempty"`
	RuntimeIssues []RuntimeIssue `json:"runtimeIssues,omitempty"`

	// Revision information
	LastAppliedRevision   string `json:"lastAppliedRevision,omitempty"`
	LastAttemptedRevision string `json:"lastAttemptedRevision,omitempty"`
}

// RuntimeIssue summarizes pod runtime failures for an Argo Application.
type RuntimeIssue struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// IsHealthy returns true if the deployer is healthy
func (d DeployerStatus) IsHealthy() bool {
	return d.Ready && d.Stage == string(agent.StageHealthy) && !d.Suspended
}

// SourceStatus holds the status of a single source
type SourceStatus struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     bool   `json:"ready"`

	// Reason is the condition reason
	Reason string `json:"reason,omitempty"`

	// Message is the human-readable error message
	Message string `json:"message,omitempty"`

	// URL is the source URL (OCI, Git, Helm)
	URL string `json:"url,omitempty"`

	// ArtifactDigest is the current artifact digest
	ArtifactDigest string `json:"artifactDigest,omitempty"`
}

// IsHealthy returns true if the source is healthy
func (s SourceStatus) IsHealthy() bool {
	return s.Ready
}

// HasFailures returns true if there are any failures in the summary
func (g GitOpsSummary) HasFailures() bool {
	for _, d := range g.Deployers {
		if !d.IsHealthy() {
			return true
		}
	}
	for _, s := range g.Sources {
		if !s.IsHealthy() {
			return true
		}
	}
	return false
}

// GetFailedDeployerCount returns the number of failed deployers
func (g GitOpsSummary) GetFailedDeployerCount() int {
	count := 0
	for _, d := range g.Deployers {
		if !d.IsHealthy() {
			count++
		}
	}
	return count
}

// GetFailedSourceCount returns the number of failed sources
func (g GitOpsSummary) GetFailedSourceCount() int {
	count := 0
	for _, s := range g.Sources {
		if !s.IsHealthy() {
			count++
		}
	}
	return count
}

func runGitOpsStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	format, err := normalizeGitOpsStatusFormat(gitopsFormat, gitopsJSON)
	if err != nil {
		return err
	}
	evidenceOptions, err := gitOpsDeliveryEvidenceOptionsFromFlags(ctx)
	if err != nil {
		return err
	}

	// TEST HOOK: Load status data from JSON file to bypass cluster access in tests.
	if statusJSONFile := os.Getenv("CUB_SCOUT_TEST_GITOPS_JSON"); statusJSONFile != "" {
		return loadAndRenderGitOpsStatusFromJSON(statusJSONFile, format)
	}

	// Build k8s config
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	// Detect backend
	detector := agent.NewApplyBackendDetector(dynClient)
	backendInfo, err := detector.DetectBackend(ctx, gitopsNamespace)
	if err != nil {
		return fmt.Errorf("failed to detect GitOps backend: %w", err)
	}

	// Build summary
	summary := buildGitOpsSummary(ctx, dynClient, backendInfo)
	if gitopsWithConfigHub {
		summary.DeliveryEvidence = collectGitOpsDeliveryEvidence(ctx, dynClient, evidenceOptions)
	}

	// Connected-mode durability: persist sync/drift summary snapshot for query/trend workflows.
	persistConnectedGitOpsSummary(summary, gitopsNamespace)

	// Output
	switch format {
	case "json":
		return outputGitOpsStatusJSON(summary)
	case "md":
		return outputGitOpsStatusMarkdown(summary)
	default:
		return outputGitOpsStatusHuman(summary)
	}
}

// buildGitOpsSummary builds a GitOpsSummary from the detected backend info
func buildGitOpsSummary(ctx context.Context, client dynamic.Interface, info *agent.ApplyBackendInfo) GitOpsSummary {
	if info == nil {
		info = &agent.ApplyBackendInfo{
			Backend:   agent.BackendNone,
			Transport: agent.TransportUnknown,
		}
	}

	coverage := gitOpsCoverageFromBackendInfo(info)
	summary := GitOpsSummary{
		Backend:         string(info.Backend),
		Transport:       string(info.Transport),
		ConfigHubTarget: info.ConfigHubTarget,
		Deployers:       []DeployerStatus{},
		Sources:         []SourceStatus{},
	}

	// Process sources first to build source status map
	sourceStatus := make(map[string]SourceStatus)
	for _, src := range info.Sources {
		status := SourceStatus{
			Kind:      src.Kind,
			Name:      src.Name,
			Namespace: src.Namespace,
			Ready:     true, // Default, will be updated if we can fetch the resource
		}

		// Try to fetch and extract failure details
		resource := fetchSourceResource(ctx, client, src)
		if resource != nil {
			details := agent.ExtractFluxSourceFailure(resource)
			status.Ready = details.Ready
			status.Reason = details.Reason
			status.Message = details.Message
			status.URL = details.SourceURL
			status.ArtifactDigest = details.ArtifactDigest
		}

		sourceStatus[src.Key()] = status
		summary.Sources = append(summary.Sources, status)
	}

	// Process deployers
	for _, dep := range info.Deployers {
		status := DeployerStatus{
			Kind:      dep.Kind,
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Ready:     true, // Default, will be updated
			Stage:     string(agent.StageHealthy),
		}

		if dep.SourceRef != nil {
			status.SourceRef = dep.SourceRef.Key()
		}

		// Fetch and extract failure details based on deployer type
		resource := fetchDeployerResource(ctx, client, dep)
		if resource != nil {
			switch dep.Kind {
			case "Kustomization", "HelmRelease":
				details := agent.ExtractFluxDeployerFailure(resource)
				status.Ready = details.Ready
				status.Suspended = details.Suspended
				status.Stage = string(details.Stage)
				status.Reason = details.Reason
				status.Message = details.Message
				status.LastAppliedRevision = details.LastAppliedRevision
				status.LastAttemptedRevision = details.LastAttemptedRevision
			case "Application":
				details := agent.ExtractArgoFailure(resource)
				status.Ready = details.Ready
				status.Stage = string(details.Stage)
				status.Reason = details.Reason
				status.Message = details.Message
				status.SyncStatus = details.SyncStatus
				status.HealthStatus = details.HealthStatus
				status.LastAttemptedRevision = details.LastAttemptedRevision
				enrichArgoApplicationRuntimeStatus(ctx, client, &status, resource)
			}
		}

		if status.IsHealthy() {
			summary.HealthyCount++
		} else {
			summary.FailedCount++
		}

		summary.Deployers = append(summary.Deployers, status)
	}

	controllerDeployers, controllerCoverage := collectFirstClassControllerDeployers(ctx, client, gitopsNamespace)
	coverage = mergeControllerCoverage(coverage, controllerCoverage)
	summary.ControllerCoverage = coverage
	if len(controllerDeployers) > 0 {
		summary.Deployers = append(summary.Deployers, controllerDeployers...)
		summary.HealthyCount = 0
		summary.FailedCount = 0
		for _, dep := range summary.Deployers {
			if dep.IsHealthy() {
				summary.HealthyCount++
			} else {
				summary.FailedCount++
			}
		}
		if summary.Backend == "none" {
			summary.Backend = "controllers"
		}
	}

	return summary
}

func collectFirstClassControllerDeployers(ctx context.Context, client dynamic.Interface, namespace string) ([]DeployerStatus, []ControllerCoverageStatus) {
	out := []DeployerStatus{}
	coverage := newGitOpsControllerCoverageTracker()
	for _, spec := range firstClassControllerResources() {
		coverage.addResourceKind(spec.Owner, spec.Kind)

		list, err := listControllerResource(ctx, client, spec, namespace)
		if err != nil {
			if !isGitOpsCRDNotFound(err) {
				coverage.addOmission(spec.Owner, ControllerCoverageOmission{
					Resource: controllerCoverageResourceID(spec),
					Reason:   gitOpsCoverageOmissionReason(err),
					Message:  err.Error(),
				})
			}
			continue
		}
		for i := range list.Items {
			item := list.Items[i]
			coverage.addFound(spec.Owner, spec.Kind)
			if !isControllerDeployerKind(spec.Kind) {
				continue
			}
			if item.GetKind() == "" {
				item.SetKind(spec.Kind)
			}
			if item.GetAPIVersion() == "" {
				item.SetAPIVersion(spec.GVR.GroupVersion().String())
			}
			out = append(out, controllerDeployerStatus(&item, spec))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out, coverage.statuses()
}

type gitOpsControllerCoverageTracker struct {
	families map[string]*gitOpsControllerCoverageFamily
	order    []string
}

type gitOpsControllerCoverageFamily struct {
	family        string
	resourceKinds map[string]struct{}
	foundKinds    map[string]struct{}
	found         int
	omissions     []ControllerCoverageOmission
}

func newGitOpsControllerCoverageTracker() *gitOpsControllerCoverageTracker {
	t := &gitOpsControllerCoverageTracker{
		families: map[string]*gitOpsControllerCoverageFamily{},
	}
	for _, family := range []string{"Flux", "ArgoCD", "ConfigHub", "Sveltos", "Modelplane"} {
		t.ensureFamily(family)
	}
	return t
}

func gitOpsCoverageFromBackendInfo(info *agent.ApplyBackendInfo) []ControllerCoverageStatus {
	tracker := newGitOpsControllerCoverageTracker()
	for _, kind := range []string{"Kustomization", "HelmRelease"} {
		tracker.addResourceKind("Flux", kind)
	}
	tracker.addResourceKind("ArgoCD", "Application")
	tracker.addResourceKind("ConfigHub", "ConfigHubTarget")

	if info == nil {
		return tracker.statuses()
	}
	for _, dep := range info.Deployers {
		switch dep.Kind {
		case "Kustomization", "HelmRelease":
			tracker.addFound("Flux", dep.Kind)
		case "Application":
			tracker.addFound("ArgoCD", dep.Kind)
		}
	}
	if info.ConfigHubTarget != nil {
		tracker.addFound("ConfigHub", "ConfigHubTarget")
	}
	return tracker.statuses()
}

func (t *gitOpsControllerCoverageTracker) ensureFamily(family string) *gitOpsControllerCoverageFamily {
	if strings.TrimSpace(family) == "" {
		family = "Unknown"
	}
	if existing := t.families[family]; existing != nil {
		return existing
	}
	entry := &gitOpsControllerCoverageFamily{
		family:        family,
		resourceKinds: map[string]struct{}{},
		foundKinds:    map[string]struct{}{},
	}
	t.families[family] = entry
	t.order = append(t.order, family)
	return entry
}

func (t *gitOpsControllerCoverageTracker) addResourceKind(family, kind string) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return
	}
	t.ensureFamily(family).resourceKinds[kind] = struct{}{}
}

func (t *gitOpsControllerCoverageTracker) addFound(family, kind string) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return
	}
	entry := t.ensureFamily(family)
	entry.resourceKinds[kind] = struct{}{}
	entry.foundKinds[kind] = struct{}{}
	entry.found++
}

func (t *gitOpsControllerCoverageTracker) addOmission(family string, omission ControllerCoverageOmission) {
	omission.Resource = strings.TrimSpace(omission.Resource)
	omission.Reason = strings.TrimSpace(omission.Reason)
	omission.Message = strings.TrimSpace(omission.Message)
	if omission.Resource == "" || omission.Reason == "" {
		return
	}
	entry := t.ensureFamily(family)
	entry.omissions = append(entry.omissions, omission)
}

func (t *gitOpsControllerCoverageTracker) statuses() []ControllerCoverageStatus {
	out := make([]ControllerCoverageStatus, 0, len(t.order))
	for _, family := range t.order {
		entry := t.families[family]
		if entry == nil {
			continue
		}
		status := controllerCoverageNotFound
		switch {
		case entry.found > 0 && len(entry.omissions) > 0:
			status = controllerCoveragePartial
		case entry.found > 0:
			status = controllerCoverageFound
		case len(entry.omissions) > 0:
			status = controllerCoverageUnreadable
		}
		omissions := append([]ControllerCoverageOmission(nil), entry.omissions...)
		sort.Slice(omissions, func(i, j int) bool {
			if omissions[i].Resource != omissions[j].Resource {
				return omissions[i].Resource < omissions[j].Resource
			}
			if omissions[i].Reason != omissions[j].Reason {
				return omissions[i].Reason < omissions[j].Reason
			}
			return omissions[i].Message < omissions[j].Message
		})
		out = append(out, ControllerCoverageStatus{
			Family:        entry.family,
			Status:        status,
			ResourceKinds: sortedStringSet(entry.resourceKinds),
			FoundKinds:    sortedStringSet(entry.foundKinds),
			Found:         entry.found,
			Omissions:     omissions,
		})
	}
	return out
}

func mergeControllerCoverage(groups ...[]ControllerCoverageStatus) []ControllerCoverageStatus {
	tracker := newGitOpsControllerCoverageTracker()
	for _, group := range groups {
		for _, status := range group {
			for _, kind := range status.ResourceKinds {
				tracker.addResourceKind(status.Family, kind)
			}
			for i := 0; i < status.Found; i++ {
				if len(status.FoundKinds) == 0 {
					tracker.addFound(status.Family, "Unknown")
					continue
				}
				tracker.addFound(status.Family, status.FoundKinds[min(i, len(status.FoundKinds)-1)])
			}
			for _, omission := range status.Omissions {
				tracker.addOmission(status.Family, omission)
			}
		}
	}
	return tracker.statuses()
}

func sortedStringSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func isGitOpsCRDNotFound(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no matches for kind") ||
		strings.Contains(msg, "the server could not find the requested resource")
}

func gitOpsCoverageOmissionReason(err error) string {
	switch {
	case apierrors.IsForbidden(err):
		return "forbidden"
	case apierrors.IsUnauthorized(err):
		return "unauthorized"
	case apierrors.IsTimeout(err):
		return "timeout"
	default:
		return "list_failed"
	}
}

func controllerCoverageResourceID(spec controllerResourceSpec) string {
	if spec.GVR.Group == "" {
		return fmt.Sprintf("%s/%s", spec.GVR.Version, spec.GVR.Resource)
	}
	return fmt.Sprintf("%s.%s/%s", spec.GVR.Resource, spec.GVR.Group, spec.GVR.Version)
}

func controllerDeployerStatus(item *unstructured.Unstructured, spec controllerResourceSpec) DeployerStatus {
	ready, status, reason, message := observedObjectStatus(item)
	stage := string(agent.StageHealthy)
	if !ready {
		stage = controllerFailureStage(status, reason)
	}
	return DeployerStatus{
		Kind:      spec.Kind,
		Name:      item.GetName(),
		Namespace: item.GetNamespace(),
		Owner:     spec.Owner,
		Ready:     ready,
		Stage:     stage,
		Reason:    reason,
		Message:   message,
	}
}

func controllerFailureStage(status, reason string) string {
	text := strings.ToLower(strings.TrimSpace(status + " " + reason))
	switch {
	case strings.Contains(text, "source"), strings.Contains(text, "reference"):
		return string(agent.StageSource)
	case strings.Contains(text, "build"), strings.Contains(text, "render"), strings.Contains(text, "template"):
		return string(agent.StageBuild)
	case strings.Contains(text, "apply"), strings.Contains(text, "failedcluster"):
		return string(agent.StageApply)
	default:
		return string(agent.StageSync)
	}
}

// fetchSourceResource fetches a source resource from the cluster
func fetchSourceResource(ctx context.Context, client dynamic.Interface, ref agent.SourceRef) *unstructured.Unstructured {
	gvr := kindToGVR(ref.Kind)
	if gvr.Resource == "" {
		return nil
	}

	resource, err := client.Resource(gvr).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	return resource
}

// fetchDeployerResource fetches a deployer resource from the cluster
func fetchDeployerResource(ctx context.Context, client dynamic.Interface, ref agent.DeployerRef) *unstructured.Unstructured {
	gvr := kindToGVR(ref.Kind)
	if gvr.Resource == "" {
		return nil
	}

	resource, err := client.Resource(gvr).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return nil
	}
	return resource
}

func enrichArgoApplicationRuntimeStatus(ctx context.Context, client dynamic.Interface, status *DeployerStatus, app *unstructured.Unstructured) {
	if client == nil || status == nil || app == nil {
		return
	}

	appName := strings.TrimSpace(status.Name)
	if appName == "" {
		appName = strings.TrimSpace(app.GetName())
	}
	if appName == "" {
		return
	}

	destinationNamespace, _, _ := unstructured.NestedString(app.Object, "spec", "destination", "namespace")
	destinationNamespace = strings.TrimSpace(destinationNamespace)
	if destinationNamespace == "" {
		return
	}

	podsGVR := kindToGVR("Pod")
	if podsGVR.Resource == "" {
		return
	}

	podList, err := client.Resource(podsGVR).Namespace(destinationNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	issueCounts := map[string]int{}
	for i := range podList.Items {
		pod := &podList.Items[i]
		if strings.TrimSpace(pod.GetLabels()["argocd.argoproj.io/instance"]) != appName {
			continue
		}

		status.PodTotal++
		if isPodReadyForGitOpsStatus(pod) {
			status.PodReady++
			continue
		}

		if reason := extractPodRuntimeIssueReason(pod); reason != "" {
			issueCounts[reason]++
		}
	}

	if len(issueCounts) == 0 {
		return
	}

	issues := make([]RuntimeIssue, 0, len(issueCounts))
	for reason, count := range issueCounts {
		issues = append(issues, RuntimeIssue{
			Reason: reason,
			Count:  count,
		})
	}

	sort.Slice(issues, func(i, j int) bool {
		if runtimeIssueOrder(issues[i].Reason) == runtimeIssueOrder(issues[j].Reason) {
			return issues[i].Reason < issues[j].Reason
		}
		return runtimeIssueOrder(issues[i].Reason) < runtimeIssueOrder(issues[j].Reason)
	})

	status.RuntimeIssues = issues
}

func isPodReadyForGitOpsStatus(pod *unstructured.Unstructured) bool {
	phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
	if !strings.EqualFold(strings.TrimSpace(phase), "Running") {
		return false
	}

	containerStatuses, found, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
	if !found || len(containerStatuses) == 0 {
		return false
	}

	for _, raw := range containerStatuses {
		status, ok := raw.(map[string]interface{})
		if !ok {
			return false
		}
		ready, ok := status["ready"].(bool)
		if !ok || !ready {
			return false
		}
	}
	return true
}

func extractPodRuntimeIssueReason(pod *unstructured.Unstructured) string {
	checkStatuses := func(statuses []interface{}) string {
		for _, raw := range statuses {
			status, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			state, _ := status["state"].(map[string]interface{})
			waiting, _ := state["waiting"].(map[string]interface{})
			if waiting == nil {
				continue
			}

			reason := strings.TrimSpace(fmt.Sprintf("%v", waiting["reason"]))
			switch reason {
			case "ImagePullBackOff", "ErrImagePull", "CrashLoopBackOff":
				return reason
			case "CreateContainerError", "CreateContainerConfigError":
				return "CreateContainerError"
			}
		}
		return ""
	}

	initStatuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "initContainerStatuses")
	if reason := checkStatuses(initStatuses); reason != "" {
		return reason
	}

	containerStatuses, _, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
	if reason := checkStatuses(containerStatuses); reason != "" {
		return reason
	}

	phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
	statusReason, _, _ := unstructured.NestedString(pod.Object, "status", "reason")

	if strings.EqualFold(strings.TrimSpace(phase), "Failed") && strings.EqualFold(strings.TrimSpace(statusReason), "Evicted") {
		return "Evicted"
	}

	if strings.EqualFold(strings.TrimSpace(phase), "Pending") {
		return "Pending"
	}

	return ""
}

func runtimeIssueOrder(reason string) int {
	order := map[string]int{
		"ImagePullBackOff":     0,
		"ErrImagePull":         1,
		"CrashLoopBackOff":     2,
		"CreateContainerError": 3,
		"Pending":              4,
		"Evicted":              5,
	}
	if idx, ok := order[reason]; ok {
		return idx
	}
	return 99
}

// outputGitOpsStatusJSON outputs the summary as JSON
func outputGitOpsStatusJSON(summary GitOpsSummary) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

// outputGitOpsStatusHuman outputs the summary in human-readable format
func outputGitOpsStatusHuman(summary GitOpsSummary) error {
	fmt.Printf("\n")
	fmt.Printf("%s%sGITOPS STATUS%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("════════════════════════════════════════════════════════════════════\n\n")

	// Backend and Transport
	backendColor := colorWhite
	switch summary.Backend {
	case "flux":
		backendColor = colorCyan
	case "argocd":
		backendColor = colorPurple
	case "worker", "controllers":
		backendColor = colorBlue
	case "none":
		backendColor = colorDim
	}

	transportColor := colorWhite
	switch summary.Transport {
	case "oci":
		transportColor = colorBlue
	case "git":
		transportColor = colorGreen
	case "helm":
		transportColor = colorYellow
	}

	fmt.Printf("  %sBackend:%s   %s%s%s\n", colorDim, colorReset, backendColor, strings.ToUpper(summary.Backend), colorReset)
	fmt.Printf("  %sTransport:%s %s%s%s\n", colorDim, colorReset, transportColor, strings.ToUpper(summary.Transport), colorReset)

	// ConfigHub target if present
	if summary.ConfigHubTarget != nil {
		fmt.Printf("  %sConfigHub:%s Space=%s%s%s Target=%s%s%s\n",
			colorDim, colorReset,
			colorBlue, summary.ConfigHubTarget.Space, colorReset,
			colorBlue, summary.ConfigHubTarget.Target, colorReset)
	}

	fmt.Printf("\n")
	outputControllerCoverageHuman(summary.ControllerCoverage)

	// Summary counts
	if summary.Backend == "none" && len(summary.Deployers) == 0 {
		fmt.Printf("  %sNo GitOps/controller backend detected in cluster.%s\n", colorYellow, colorReset)
		fmt.Printf("  %sInstall Flux, Argo CD, Sveltos, or Modelplane to enable controller status.%s\n\n", colorDim, colorReset)
		outputGitOpsDeliveryEvidenceHuman(summary.DeliveryEvidence)
		return nil
	}

	totalDeployers := len(summary.Deployers)
	if totalDeployers == 0 {
		fmt.Printf("  %sNo deployers found.%s\n\n", colorDim, colorReset)
		outputGitOpsDeliveryEvidenceHuman(summary.DeliveryEvidence)
		return nil
	}

	// Health summary
	if summary.HasFailures() {
		fmt.Printf("  %s%s⚠ %d/%d deployers failing%s\n\n",
			colorBold, colorYellow, summary.FailedCount, totalDeployers, colorReset)
	} else {
		fmt.Printf("  %s%s✓ All %d deployers healthy%s\n\n",
			colorBold, colorGreen, totalDeployers, colorReset)
	}

	// Sources section
	if len(summary.Sources) > 0 {
		fmt.Printf("%sSOURCES%s\n", colorBold, colorReset)
		fmt.Printf("────────────────────────────────────────────────────────────────────\n")
		for _, src := range summary.Sources {
			outputSourceStatus(src)
		}
		fmt.Printf("\n")
	}

	// Deployers section
	if len(summary.Deployers) > 0 {
		fmt.Printf("%sDEPLOYERS%s\n", colorBold, colorReset)
		fmt.Printf("────────────────────────────────────────────────────────────────────\n")
		for _, dep := range summary.Deployers {
			outputDeployerStatus(dep)
		}
		fmt.Printf("\n")
	}

	outputGitOpsDeliveryEvidenceHuman(summary.DeliveryEvidence)

	// Next steps for failures
	if summary.HasFailures() {
		fmt.Printf("%sNEXT STEPS:%s\n", colorBold, colorReset)

		// Find first failing deployer for trace suggestion
		for _, dep := range summary.Deployers {
			if !dep.IsHealthy() {
				fmt.Printf("→ Trace failing resource:  %s\n", gitopsTraceCommand(dep))
				break
			}
		}

		// Check for source failures
		for _, src := range summary.Sources {
			if !src.IsHealthy() {
				if strings.Contains(strings.ToLower(src.Reason), "auth") {
					fmt.Printf("→ Check credentials:       kubectl get secret -n %s\n", src.Namespace)
				}
				break
			}
		}

		fmt.Printf("→ Scan for issues:         ./cub-scout scan --state\n")
		fmt.Printf("\n")
	}

	return nil
}

func outputControllerCoverageHuman(coverage []ControllerCoverageStatus) {
	if len(coverage) == 0 {
		return
	}
	fmt.Printf("%sCONTROLLER COVERAGE%s\n", colorBold, colorReset)
	fmt.Printf("────────────────────────────────────────────────────────────────────\n")
	for _, entry := range coverage {
		icon, iconColor := controllerCoverageIconAndColor(entry.Status)
		status := strings.ReplaceAll(entry.Status, "_", " ")
		details := []string{}
		if entry.Found > 0 {
			details = append(details, fmt.Sprintf("%d observed", entry.Found))
		}
		if len(entry.FoundKinds) > 0 {
			details = append(details, "found "+formatControllerCoverageKinds(entry.FoundKinds, 5))
		} else if len(entry.ResourceKinds) > 0 {
			details = append(details, "checked "+formatControllerCoverageKinds(entry.ResourceKinds, 5))
		}
		if len(entry.Omissions) > 0 {
			details = append(details, fmt.Sprintf("%d omission(s)", len(entry.Omissions)))
		}
		if len(details) == 0 {
			details = append(details, "no resource kinds registered")
		}
		fmt.Printf("  %s%s%s %s: %s%s%s (%s)\n",
			iconColor, icon, colorReset,
			entry.Family,
			controllerCoverageStatusColor(entry.Status), status, colorReset,
			strings.Join(details, "; "),
		)
		for i, omission := range entry.Omissions {
			if i >= 3 {
				fmt.Printf("      %s... %d more omission(s)%s\n", colorDim, len(entry.Omissions)-i, colorReset)
				break
			}
			fmt.Printf("      %sOmitted:%s %s (%s)\n", colorDim, colorReset, omission.Resource, omission.Reason)
		}
	}
	fmt.Printf("\n")
}

func controllerCoverageIconAndColor(status string) (string, string) {
	switch status {
	case controllerCoverageFound:
		return SymOK, colorGreen
	case controllerCoveragePartial:
		return SymWarning, colorYellow
	case controllerCoverageUnreadable:
		return SymError, colorRed
	default:
		return SymInactive, colorDim
	}
}

func controllerCoverageStatusColor(status string) string {
	switch status {
	case controllerCoverageFound:
		return colorGreen
	case controllerCoveragePartial:
		return colorYellow
	case controllerCoverageUnreadable:
		return colorRed
	default:
		return colorDim
	}
}

func formatControllerCoverageKinds(kinds []string, max int) string {
	if len(kinds) == 0 {
		return "none"
	}
	if max <= 0 || len(kinds) <= max {
		return strings.Join(kinds, ", ")
	}
	return fmt.Sprintf("%s, +%d more", strings.Join(kinds[:max], ", "), len(kinds)-max)
}

func gitopsTraceCommand(dep DeployerStatus) string {
	cmd := fmt.Sprintf("./cub-scout trace %s/%s", strings.ToLower(dep.Kind), dep.Name)
	if dep.Namespace != "" {
		cmd += " -n " + dep.Namespace
	}
	return cmd
}

// outputSourceStatus outputs a single source status
func outputSourceStatus(src SourceStatus) {
	// Status icon
	icon := SymOK
	iconColor := colorGreen
	if !src.Ready {
		icon = SymError
		iconColor = colorRed
	}

	// Kind color
	kindColor := colorPurple
	if src.Kind == "GitRepository" {
		kindColor = colorGreen
	} else if src.Kind == "HelmRepository" {
		kindColor = colorYellow
	}

	fmt.Printf("  %s%s%s %s%s%s/%s%s%s\n",
		iconColor, icon, colorReset,
		kindColor, src.Kind, colorReset,
		colorBold, src.Name, colorReset)

	fmt.Printf("      %sNamespace:%s %s\n", colorDim, colorReset, src.Namespace)

	if src.URL != "" {
		fmt.Printf("      %sURL:%s %s%s%s\n", colorDim, colorReset, colorBlue, src.URL, colorReset)
	}

	if !src.Ready {
		if src.Reason != "" {
			fmt.Printf("      %sReason:%s %s%s%s\n", colorDim, colorReset, colorRed, src.Reason, colorReset)
		}
		if src.Message != "" {
			msg := src.Message
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			fmt.Printf("      %sMessage:%s %s\n", colorDim, colorReset, msg)
		}
	} else if src.ArtifactDigest != "" {
		digest := src.ArtifactDigest
		if len(digest) > 20 {
			digest = digest[:20] + "..."
		}
		fmt.Printf("      %sDigest:%s %s\n", colorDim, colorReset, digest)
	}

	fmt.Printf("\n")
}

// outputDeployerStatus outputs a single deployer status
func outputDeployerStatus(dep DeployerStatus) {
	// Status icon
	icon := SymOK
	iconColor := colorGreen
	if !dep.Ready {
		icon = SymError
		iconColor = colorRed
	}
	if dep.Suspended {
		icon = SymPaused
		iconColor = colorYellow
	}

	// Kind color
	kindColor := colorCyan
	if dep.Kind == "Application" {
		kindColor = colorPurple
	} else if dep.Kind == "HelmRelease" {
		kindColor = colorYellow
	} else if spec, ok := controllerResourceByKind(dep.Kind); ok {
		switch spec.Owner {
		case "Sveltos":
			kindColor = colorBlue
		case "Modelplane":
			kindColor = colorGreen
		}
	}

	fmt.Printf("  %s%s%s %s%s%s/%s%s%s\n",
		iconColor, icon, colorReset,
		kindColor, dep.Kind, colorReset,
		colorBold, dep.Name, colorReset)

	fmt.Printf("      %sNamespace:%s %s\n", colorDim, colorReset, dep.Namespace)
	if dep.Owner != "" {
		fmt.Printf("      %sOwner:%s %s\n", colorDim, colorReset, dep.Owner)
	}

	// Show stage for non-healthy deployers
	if !dep.IsHealthy() {
		stageColor := colorYellow
		switch dep.Stage {
		case string(agent.StageSource):
			stageColor = colorPurple
		case string(agent.StageBuild):
			stageColor = colorYellow
		case string(agent.StageApply):
			stageColor = colorRed
		case string(agent.StageSync):
			stageColor = colorRed
		}
		fmt.Printf("      %sStage:%s %s%s%s\n", colorDim, colorReset, stageColor, dep.Stage, colorReset)
	}

	// Argo-specific status
	if dep.Kind == "Application" {
		if dep.SyncStatus != "" {
			syncColor := colorGreen
			if dep.SyncStatus != "Synced" {
				syncColor = colorYellow
			}
			fmt.Printf("      %sSync:%s %s%s%s\n", colorDim, colorReset, syncColor, dep.SyncStatus, colorReset)
		}
		if dep.HealthStatus != "" {
			healthColor := colorGreen
			if dep.HealthStatus != "Healthy" {
				healthColor = colorRed
			}
			healthLabel := dep.HealthStatus + " (ArgoCD)"
			fmt.Printf("      %sHealth:%s %s%s%s\n", colorDim, colorReset, healthColor, healthLabel, colorReset)
		}

		if dep.PodTotal > 0 {
			fmt.Printf("      Pods: %d/%d running\n", dep.PodReady, dep.PodTotal)
		}

		if strings.EqualFold(dep.HealthStatus, "Healthy") && dep.PodTotal > 0 && dep.PodReady < dep.PodTotal {
			fmt.Printf("      %s⚠ Warning:%s %d/%d pods running - ArgoCD health may be misleading\n",
				colorYellow, colorReset, dep.PodReady, dep.PodTotal)
		}

		if len(dep.RuntimeIssues) > 0 {
			fmt.Printf("      %sRuntime issues:%s\n", colorDim, colorReset)
			for _, issue := range dep.RuntimeIssues {
				fmt.Printf("        %s: %d pod(s)\n", issue.Reason, issue.Count)
			}
		}
	}

	// Error details
	if !dep.Ready && !dep.Suspended {
		if dep.Reason != "" {
			fmt.Printf("      %sReason:%s %s%s%s\n", colorDim, colorReset, colorRed, dep.Reason, colorReset)
		}
		if dep.Message != "" {
			msg := dep.Message
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			fmt.Printf("      %sMessage:%s %s\n", colorDim, colorReset, msg)
		}
	}

	// Revision mismatch warning
	if dep.LastAppliedRevision != "" && dep.LastAttemptedRevision != "" &&
		dep.LastAppliedRevision != dep.LastAttemptedRevision {
		fmt.Printf("      %s⚠ Revision mismatch:%s applied=%s, attempted=%s\n",
			colorYellow, colorReset,
			truncate(dep.LastAppliedRevision, 12),
			truncate(dep.LastAttemptedRevision, 12))
	}

	// Suspended indicator
	if dep.Suspended {
		fmt.Printf("      %s(suspended)%s\n", colorDim, colorReset)
	}

	fmt.Printf("\n")
}

// loadAndRenderGitOpsStatusFromJSON loads status from JSON file for testing
func loadAndRenderGitOpsStatusFromJSON(path, format string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read gitops JSON: %w", err)
	}

	var summary GitOpsSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return fmt.Errorf("failed to parse gitops JSON: %w", err)
	}

	switch format {
	case "json":
		return outputGitOpsStatusJSON(summary)
	case "md":
		return outputGitOpsStatusMarkdown(summary)
	default:
		return outputGitOpsStatusHuman(summary)
	}
}
