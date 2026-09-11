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
	"time"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	doctorFormat       string
	doctorNamespace    string
	doctorTopIssues    int
	doctorPresentation string
	doctorHintMode     string

	doctorWithConfigHub       bool
	doctorConfigHubSpace      string
	doctorConfigHubSince      string
	doctorConfigHubStaleAfter string

	collectDoctorDeliveryEvidenceFn = collectDoctorDeliveryEvidence
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Show a single-command cluster health summary",
	Long: `doctor provides a compact, actionable summary of cluster state.

It combines ownership, health, risk, and drift signals into one view.

Examples:
  cub-scout doctor
  cub-scout doctor --namespace prod
  cub-scout doctor --format json
  cub-scout doctor --with-confighub --confighub-space prod --format json
`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().StringVarP(&doctorNamespace, "namespace", "n", "", "Namespace scope (default: all namespaces)")
	doctorCmd.Flags().StringVar(&doctorFormat, "format", "ascii", "Output format: ascii, json")
	doctorCmd.Flags().IntVar(&doctorTopIssues, "top", 3, "Number of top issues to include")
	doctorCmd.Flags().StringVar(&doctorPresentation, "presentation", "", PresentationModeHelp())
	doctorCmd.Flags().StringVar(&doctorHintMode, "hint-mode", "", HintModeHelp())
	doctorCmd.Flags().BoolVar(&doctorWithConfigHub, "with-confighub", false, "Include bounded ConfigHub delivery evidence for the selected scope")
	doctorCmd.Flags().StringVar(&doctorConfigHubSpace, "confighub-space", "", "ConfigHub space for connected delivery evidence (default: current cub space; use '*' explicitly for all spaces)")
	doctorCmd.Flags().StringVar(&doctorConfigHubSince, "confighub-since", "24h", "Lookback window for ConfigHub release/event evidence (examples: 24h, 7d, 2w)")
	doctorCmd.Flags().StringVar(&doctorConfigHubStaleAfter, "confighub-stale-after", "15m", "Treat ConfigHub live-status observations older than this as stale")
}

// DoctorSummary is the canonical model behind both ASCII and JSON output.
type DoctorSummary struct {
	Cluster   string                 `json:"cluster"`
	Namespace string                 `json:"namespace"`
	Resources DoctorResourceSummary  `json:"resources"`
	Ownership DoctorOwnershipSummary `json:"ownership"`
	Health    DoctorHealthSummary    `json:"health"`
	Risks     DoctorRiskSummary      `json:"risks"`
	Drift     DoctorDriftSummary     `json:"drift"`
	Rollouts  *DoctorRolloutSummary  `json:"rollouts,omitempty"`
	Delivery  *DoctorDeliverySummary `json:"delivery,omitempty"`
	ThreeWay  *DoctorThreeWaySummary `json:"threeWay,omitempty"`
	TopIssues []DoctorIssue          `json:"topIssues,omitempty"`
	NextSteps []StructuredHint       `json:"nextSteps,omitempty"` // Structured action-typed hints for AI/MCP

	// DeliveryEvidence is the raw bounded evidence envelope behind Delivery.
	// It is present only when --with-confighub is requested.
	DeliveryEvidence *GitOpsDeliveryEvidence `json:"deliveryEvidence,omitempty"`
}

// DoctorThreeWaySummary indicates three-way comparison status.
// In connected mode, this surfaces whether ConfigHub/Argo/cluster agree.
type DoctorThreeWaySummary struct {
	Available bool   `json:"available"`      // True if connected mode is available
	Hint      string `json:"hint,omitempty"` // Suggested command for full comparison
}

// DoctorResourceSummary contains resource inventory totals.
type DoctorResourceSummary struct {
	Total int `json:"total"`
}

// DoctorOwnershipSummary contains ownership counts.
type DoctorOwnershipSummary struct {
	Flux       int `json:"flux"`
	ArgoCD     int `json:"argocd"`
	Sveltos    int `json:"sveltos"`
	Modelplane int `json:"modelplane"`
	Crossplane int `json:"crossplane"`
	Kro        int `json:"kro"`
	Helm       int `json:"helm"`
	Terraform  int `json:"terraform"`
	ConfigHub  int `json:"confighub"`
	Native     int `json:"native"`
	Other      int `json:"other"`
	Unmanaged  int `json:"unmanaged"`
}

// DoctorHealthSummary contains health band counts.
type DoctorHealthSummary struct {
	Healthy int `json:"healthy"`
	Warning int `json:"warning"`
	Error   int `json:"error"`
}

// DoctorRiskSummary contains risk finding counts by severity.
type DoctorRiskSummary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

// DoctorDriftSummary contains drift signal counts.
type DoctorDriftSummary struct {
	Resources int `json:"resources"`
}

// DoctorRolloutSummary contains generation-scoped current-change evidence for
// workload resources in the doctor scope.
type DoctorRolloutSummary struct {
	Total          int                     `json:"total"`
	Pass           int                     `json:"pass"`
	Watch          int                     `json:"watch"`
	Block          int                     `json:"block"`
	Inconclusive   int                     `json:"inconclusive"`
	CurrentChanges []agent.RolloutDecision `json:"currentChanges,omitempty"`
}

// DoctorDeliverySummary is a scan-friendly rollup of the optional ConfigHub
// delivery evidence attached to doctor.
type DoctorDeliverySummary struct {
	Scope            GitOpsDeliveryEvidenceScope      `json:"scope"`
	LiveStatus       DoctorLiveStatusSummary          `json:"liveStatus"`
	EventConsumers   DoctorEventConsumerSummary       `json:"eventConsumers"`
	RecentReleases   int                              `json:"recentReleases"`
	RecentUnitEvents int                              `json:"recentUnitEvents"`
	Omissions        []GitOpsDeliveryEvidenceOmission `json:"omissions,omitempty"`
}

type DoctorLiveStatusSummary struct {
	Total             int                 `json:"total"`
	Delivery          DoctorVerdictCounts `json:"delivery"`
	ApplicationHealth DoctorVerdictCounts `json:"applicationHealth"`
	Fresh             int                 `json:"fresh"`
	Stale             int                 `json:"stale"`
	UnknownFreshness  int                 `json:"unknownFreshness"`
}

type DoctorVerdictCounts struct {
	Pass         int `json:"pass"`
	Watch        int `json:"watch"`
	Block        int `json:"block"`
	Inconclusive int `json:"inconclusive"`
}

type DoctorEventConsumerSummary struct {
	Total    int `json:"total"`
	Ready    int `json:"ready"`
	NotReady int `json:"notReady"`
}

// DoctorIssue is a concise issue entry for doctor output.
type DoctorIssue struct {
	Severity  string `json:"severity"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace"`
	Message   string `json:"message"`
}

type doctorFixtureInput struct {
	Cluster   string                   `json:"cluster"`
	Namespace string                   `json:"namespace"`
	Entries   []MapEntry               `json:"entries"`
	Findings  []scan.NormalizedFinding `json:"findings"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(doctorFormat))
	if format != "ascii" && format != "json" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json)", doctorFormat)
	}

	// Build invocation context with presentation mode resolution
	invCtx, err := NewInvocationContext(doctorPresentation, TransportCLI)
	if err != nil {
		return err
	}

	// Parse hint mode (separate from presentation mode)
	hintMode, err := ParseHintMode(doctorHintMode)
	if err != nil {
		return err
	}
	hintCtx := HintContext{Mode: hintMode}

	if doctorTopIssues < 0 {
		return fmt.Errorf("--top must be >= 0")
	}
	if doctorWithConfigHub {
		if err := validateDoctorConfigHubFlags(); err != nil {
			return err
		}
	}

	// Call the shared capability seam
	// Fixture path is passed explicitly rather than read inside the seam
	fixturePath := os.Getenv("CUB_SCOUT_TEST_DOCTOR_INPUT_JSON")
	result, err := ObserveScopeSummary(cmd.Context(), ObserveScopeSummaryRequest{
		Namespace:           doctorNamespace,
		TopIssues:           doctorTopIssues,
		FixturePath:         fixturePath,
		WithConfigHub:       doctorWithConfigHub,
		ConfigHubSpace:      doctorConfigHubSpace,
		ConfigHubSince:      doctorConfigHubSince,
		ConfigHubStaleAfter: doctorConfigHubStaleAfter,
	})
	if err != nil {
		// Only apply kube recovery hints for cluster-path errors, not fixture errors
		if fixturePath == "" {
			return withKubeRecoveryHint(err, "cub-scout doctor")
		}
		return err
	}

	// Print any warnings from the seam (CLI-specific concern)
	for _, w := range result.Warnings {
		fmt.Fprintf(os.Stderr, "Note: %s\n", w)
	}

	summary := result.Summary

	switch format {
	case "json":
		// Populate structured hints for JSON output (reuses existing hint logic)
		hints := doctorHintsWithContext(summary, hintCtx)
		sortHints(hints)
		if len(hints) > 3 {
			hints = hints[:3]
		}
		summary.NextSteps = HintsToStructured(hints)

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	default:
		// Use invocation context for presentation mode, hint context for recommendations
		fmt.Print(renderDoctorASCII(summary, invCtx.Mode(), invCtx.IsExplicit(), hintCtx))
		return nil
	}
}

func collectDoctorEntries(ctx context.Context, namespace string) ([]MapEntry, string, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, "", fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, "", fmt.Errorf("create dynamic client: %w", err)
	}

	clusterName := getClusterName()
	entries := []MapEntry{}
	byOwner := map[string]int{}

	resources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
	}
	resources = append(resources, firstClassControllerGVRs()...)

	for _, gvr := range resources {
		if namespace != "" {
			l, err := dynClient.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
			if err != nil {
				continue
			}
			for _, item := range l.Items {
				itemCopy := item
				entries = processResource(&itemCopy, gvr, clusterName, entries, byOwner)
			}
		} else {
			l, err := dynClient.Resource(gvr).List(ctx, v1.ListOptions{})
			if err != nil {
				continue
			}
			for _, item := range l.Items {
				itemCopy := item
				entries = processResource(&itemCopy, gvr, clusterName, entries, byOwner)
			}
		}
	}

	return entries, clusterName, nil
}

func collectDoctorFindings(ctx context.Context, namespace string) ([]scan.NormalizedFinding, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubernetes config: %w", err)
	}

	provider := scan.SelectProvider(scan.ProviderConfig{})
	threshold, _ := time.ParseDuration("5m")

	result, err := provider.ScanCluster(ctx, scan.ClusterScanOpts{
		Config:     cfg,
		Namespace:  namespace,
		RunKyverno: true,
		RunState:   true,
		Threshold:  threshold,
	})
	if err != nil {
		return nil, err
	}

	normalized := scan.Normalize(result)
	if normalized == nil {
		return nil, nil
	}
	return normalized.Findings, nil
}

func collectDoctorRollouts(ctx context.Context, namespace string, topN int) (*DoctorRolloutSummary, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	observedAt := time.Now().UTC()
	decisions := []agent.RolloutDecision{}
	for _, gvr := range []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "batch", Version: "v1", Resource: "jobs"},
	} {
		var items []unstructured.Unstructured
		var listErr error
		if namespace != "" {
			list, err := dynClient.Resource(gvr).Namespace(namespace).List(ctx, v1.ListOptions{})
			listErr = err
			if list != nil {
				items = list.Items
			}
		} else {
			list, err := dynClient.Resource(gvr).List(ctx, v1.ListOptions{})
			listErr = err
			if list != nil {
				items = list.Items
			}
		}
		if listErr != nil {
			continue
		}

		for i := range items {
			item := items[i]
			decision, ok := agent.BuildRolloutDecisionForWorkload(&item, nil, 0, observedAt)
			if !ok {
				continue
			}
			if decision.Verdict != agent.VerdictPASS {
				pods := relatedPodsForRolloutDecision(ctx, dynClient, item.GetNamespace(), &item)
				if len(pods) > 0 {
					if withPods, ok := agent.BuildRolloutDecisionForWorkload(&item, pods, 0, observedAt); ok {
						decision = withPods
					}
				}
			}
			decisions = append(decisions, decision)
		}
	}

	if len(decisions) == 0 {
		return nil, nil
	}
	return buildDoctorRolloutSummary(decisions, topN), nil
}

func validateDoctorConfigHubFlags() error {
	return validateDoctorConfigHubRequest(ObserveScopeSummaryRequest{
		ConfigHubSince:      doctorConfigHubSince,
		ConfigHubStaleAfter: doctorConfigHubStaleAfter,
	})
}

func validateDoctorConfigHubRequest(req ObserveScopeSummaryRequest) error {
	since := strings.TrimSpace(req.ConfigHubSince)
	if since == "" {
		since = "24h"
	}
	if _, err := parseHistorySince(since); err != nil {
		return fmt.Errorf("invalid --confighub-since: %w", err)
	}

	staleAfter := strings.TrimSpace(req.ConfigHubStaleAfter)
	if staleAfter == "" {
		staleAfter = "15m"
	}
	if _, err := parseHistorySince(staleAfter); err != nil {
		return fmt.Errorf("invalid --confighub-stale-after: %w", err)
	}
	return nil
}

func collectDoctorDeliveryEvidence(ctx context.Context, namespace string, req ObserveScopeSummaryRequest) (*GitOpsDeliveryEvidence, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubernetes config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	opts, err := doctorDeliveryEvidenceOptionsFromRequest(ctx, namespace, req)
	if err != nil {
		return nil, err
	}
	return collectGitOpsDeliveryEvidence(ctx, dynClient, opts), nil
}

func doctorDeliveryEvidenceOptionsFromRequest(ctx context.Context, namespace string, req ObserveScopeSummaryRequest) (gitOpsDeliveryEvidenceOptions, error) {
	now := gitopsNowFn().UTC()
	opts := gitOpsDeliveryEvidenceOptions{
		Namespace: strings.TrimSpace(namespace),
		Space:     strings.TrimSpace(req.ConfigHubSpace),
		Since:     strings.TrimSpace(req.ConfigHubSince),
		Now:       now,
		MaxItems:  defaultGitOpsDeliveryMaxItems,
	}
	if opts.Since == "" {
		opts.Since = "24h"
	}
	window, err := parseHistorySince(opts.Since)
	if err != nil {
		return opts, fmt.Errorf("invalid --confighub-since: %w", err)
	}
	opts.Window = window

	staleAfterRaw := strings.TrimSpace(req.ConfigHubStaleAfter)
	if staleAfterRaw == "" {
		staleAfterRaw = "15m"
	}
	staleAfter, err := parseHistorySince(staleAfterRaw)
	if err != nil {
		return opts, fmt.Errorf("invalid --confighub-stale-after: %w", err)
	}
	opts.StaleAfter = staleAfter

	if opts.Space == "" {
		opts.Space = gitopsDefaultSpaceFn(ctx)
	}
	return opts, nil
}

func buildDoctorRolloutSummary(decisions []agent.RolloutDecision, topN int) *DoctorRolloutSummary {
	summary := &DoctorRolloutSummary{Total: len(decisions)}
	current := make([]agent.RolloutDecision, 0)
	for _, decision := range decisions {
		switch decision.Verdict {
		case agent.VerdictPASS:
			summary.Pass++
		case agent.VerdictWATCH:
			summary.Watch++
			current = append(current, decision)
		case agent.VerdictBLOCK:
			summary.Block++
			current = append(current, decision)
		case agent.VerdictINCONCLUSIVE:
			summary.Inconclusive++
			current = append(current, decision)
		default:
			summary.Inconclusive++
			current = append(current, decision)
		}
	}

	sort.Slice(current, func(i, j int) bool {
		ri := rolloutVerdictRank(current[i].Verdict)
		rj := rolloutVerdictRank(current[j].Verdict)
		if ri != rj {
			return ri < rj
		}
		ii := current[i].Resource
		ij := current[j].Resource
		if ii.Namespace != ij.Namespace {
			return ii.Namespace < ij.Namespace
		}
		if ii.Kind != ij.Kind {
			return ii.Kind < ij.Kind
		}
		return ii.Name < ij.Name
	})

	if topN < 0 {
		topN = 0
	}
	if topN > len(current) {
		topN = len(current)
	}
	summary.CurrentChanges = current[:topN]
	return summary
}

func attachDoctorDeliveryEvidence(summary *DoctorSummary, evidence *GitOpsDeliveryEvidence, topN int) {
	if summary == nil || evidence == nil {
		return
	}
	summary.DeliveryEvidence = evidence
	summary.Delivery = buildDoctorDeliverySummary(evidence)

	issues := append([]DoctorIssue(nil), summary.TopIssues...)
	issues = append(issues, buildDoctorDeliveryIssues(evidence)...)
	sortDoctorIssues(issues)
	summary.TopIssues = limitDoctorIssues(issues, topN)
}

func buildDoctorDeliverySummary(evidence *GitOpsDeliveryEvidence) *DoctorDeliverySummary {
	if evidence == nil {
		return nil
	}
	summary := &DoctorDeliverySummary{
		Scope:     evidence.Scope,
		Omissions: append([]GitOpsDeliveryEvidenceOmission(nil), evidence.Omissions...),
	}
	if evidence.ConfigHub != nil {
		summary.LiveStatus.Total = len(evidence.ConfigHub.LiveStatuses)
		summary.RecentReleases = len(evidence.ConfigHub.Releases)
		summary.RecentUnitEvents = len(evidence.ConfigHub.UnitEvents)
		for _, status := range evidence.ConfigHub.LiveStatuses {
			countDoctorVerdict(&summary.LiveStatus.Delivery, status.DeliveryVerdict)
			countDoctorVerdict(&summary.LiveStatus.ApplicationHealth, status.ApplicationHealthVerdict)
			switch strings.ToLower(strings.TrimSpace(status.Freshness)) {
			case "fresh":
				summary.LiveStatus.Fresh++
			case "stale":
				summary.LiveStatus.Stale++
			default:
				summary.LiveStatus.UnknownFreshness++
			}
		}
	}
	summary.EventConsumers.Total = len(evidence.EventConsumers)
	for _, consumer := range evidence.EventConsumers {
		if consumer.Ready {
			summary.EventConsumers.Ready++
		} else {
			summary.EventConsumers.NotReady++
		}
	}
	return summary
}

func countDoctorVerdict(counts *DoctorVerdictCounts, verdict agent.ReceiptVerdict) {
	if counts == nil {
		return
	}
	switch verdict {
	case agent.VerdictPASS:
		counts.Pass++
	case agent.VerdictWATCH:
		counts.Watch++
	case agent.VerdictBLOCK:
		counts.Block++
	case agent.VerdictINCONCLUSIVE:
		counts.Inconclusive++
	default:
		counts.Inconclusive++
	}
}

func buildDoctorDeliveryIssues(evidence *GitOpsDeliveryEvidence) []DoctorIssue {
	if evidence == nil {
		return nil
	}
	issues := []DoctorIssue{}
	if evidence.ConfigHub != nil {
		for _, status := range evidence.ConfigHub.LiveStatuses {
			if status.Freshness != "fresh" {
				severity := "INFO"
				if status.Freshness == "stale" {
					severity = "WARNING"
				}
				issues = append(issues, DoctorIssue{
					Severity: severity,
					Resource: "ConfigHubLiveStatus/" + firstNonEmpty(status.App, status.Space, status.SpaceID, "unknown"),
					Message: fmt.Sprintf("current delivery/application health unverified (reported sync=%s operation=%s health=%s freshness=%s); read current controller/workload status",
						firstNonEmpty(status.SyncStatus, "-"), firstNonEmpty(status.OperationPhase, "-"),
						firstNonEmpty(status.HealthStatus, "-"), firstNonEmpty(status.Freshness, "unknown")),
				})
				continue
			}
			if severity := doctorSeverityForVerdict(status.DeliveryVerdict); severity != "" {
				issues = append(issues, DoctorIssue{
					Severity: severity,
					Resource: "ConfigHubLiveStatus/" + firstNonEmpty(status.App, status.Space, status.SpaceID, "unknown"),
					Message: fmt.Sprintf("delivery writeback reports %s (sync=%s operation=%s freshness=%s)",
						status.DeliveryVerdict,
						firstNonEmpty(status.SyncStatus, "-"),
						firstNonEmpty(status.OperationPhase, "-"),
						firstNonEmpty(status.Freshness, "-"),
					),
				})
			}

			if status.ApplicationHealthVerdict != agent.VerdictPASS {
				if severity := doctorSeverityForVerdict(status.ApplicationHealthVerdict); severity != "" {
					issues = append(issues, DoctorIssue{
						Severity: severity,
						Resource: "ConfigHubLiveStatus/" + firstNonEmpty(status.App, status.Space, status.SpaceID, "unknown"),
						Message: fmt.Sprintf("application health writeback reports %s (health=%s freshness=%s)",
							status.ApplicationHealthVerdict,
							firstNonEmpty(status.HealthStatus, "-"),
							firstNonEmpty(status.Freshness, "-"),
						),
					})
				}
			}
		}
		for _, event := range evidence.ConfigHub.UnitEvents {
			if doctorUnitEventFailed(event) {
				issues = append(issues, DoctorIssue{
					Severity: "CRITICAL",
					Resource: "ConfigHubUnitEvent/" + firstNonEmpty(event.EventID, event.Action, "unknown"),
					Message: fmt.Sprintf("unit event %s reports failure for unit=%s target=%s",
						firstNonEmpty(event.Action, event.Status, "-"),
						firstNonEmpty(event.Unit, event.UnitID, "-"),
						firstNonEmpty(event.Target, event.TargetID, "-"),
					),
				})
			}
		}
	}
	for _, consumer := range evidence.EventConsumers {
		if consumer.Ready {
			continue
		}
		issues = append(issues, DoctorIssue{
			Severity:  "WARNING",
			Resource:  consumer.Kind + "/" + consumer.Name,
			Namespace: consumer.Namespace,
			Message:   fmt.Sprintf("event consumer is not ready (%d/%d replicas ready)", consumer.ReadyReplicas, consumer.Replicas),
		})
	}
	return issues
}

func doctorSeverityForVerdict(verdict agent.ReceiptVerdict) string {
	switch verdict {
	case agent.VerdictBLOCK:
		return "CRITICAL"
	case agent.VerdictWATCH:
		return "WARNING"
	case agent.VerdictINCONCLUSIVE:
		return "INFO"
	default:
		return ""
	}
}

func doctorUnitEventFailed(event ConfigHubUnitEventEvidence) bool {
	result := strings.ToLower(strings.TrimSpace(firstNonEmpty(event.Result, event.Status)))
	switch result {
	case "failed", "failure", "error":
		return true
	default:
		return false
	}
}

func rolloutVerdictRank(verdict agent.ReceiptVerdict) int {
	switch verdict {
	case agent.VerdictBLOCK:
		return 0
	case agent.VerdictINCONCLUSIVE:
		return 1
	case agent.VerdictWATCH:
		return 2
	case agent.VerdictPASS:
		return 3
	default:
		return 4
	}
}

func buildDoctorSummary(entries []MapEntry, findings []scan.NormalizedFinding, cluster, namespace string, topN int) DoctorSummary {
	summary := DoctorSummary{
		Cluster:   cluster,
		Namespace: namespace,
		Resources: DoctorResourceSummary{Total: len(entries)},
	}

	for _, e := range entries {
		switch e.Owner {
		case "Flux":
			summary.Ownership.Flux++
		case "ArgoCD":
			summary.Ownership.ArgoCD++
		case "Sveltos":
			summary.Ownership.Sveltos++
		case "Modelplane":
			summary.Ownership.Modelplane++
		case "Crossplane":
			summary.Ownership.Crossplane++
		case "kro":
			summary.Ownership.Kro++
		case "Helm":
			summary.Ownership.Helm++
		case "Terraform":
			summary.Ownership.Terraform++
		case "ConfigHub":
			summary.Ownership.ConfigHub++
		case "Native":
			summary.Ownership.Native++
		default:
			summary.Ownership.Other++
		}

		switch e.Status {
		case "Ready":
			summary.Health.Healthy++
		case "Failed":
			summary.Health.Error++
		default:
			summary.Health.Warning++
		}

		if strings.EqualFold(e.Status, "Drifted") || (e.Kind == "Application" && e.Status != "Ready") {
			summary.Drift.Resources++
		}
	}

	summary.Ownership.Unmanaged = summary.Ownership.Native

	issues := make([]DoctorIssue, 0, len(findings))
	for _, f := range findings {
		sev := strings.ToLower(strings.TrimSpace(f.Severity))
		summary.Risks.Total++
		switch sev {
		case "critical":
			summary.Risks.Critical++
		case "warning":
			summary.Risks.Warning++
		default:
			summary.Risks.Info++
			if sev == "" {
				sev = "info"
			}
		}

		issues = append(issues, DoctorIssue{
			Severity:  strings.ToUpper(sev),
			Resource:  strings.TrimSpace(f.Resource),
			Namespace: strings.TrimSpace(f.Namespace),
			Message:   strings.TrimSpace(f.Message),
		})
	}

	sortDoctorIssues(issues)
	summary.TopIssues = limitDoctorIssues(issues, topN)

	// Check for connected mode to surface three-way comparison capability
	client := hub.NewClient()
	if err := client.RequireConnected(); err == nil {
		nsFlag := ""
		if namespace != "" && namespace != "all" {
			nsFlag = fmt.Sprintf(" --scope namespace/%s", namespace)
		} else {
			nsFlag = " --scope cluster"
		}
		summary.ThreeWay = &DoctorThreeWaySummary{
			Available: true,
			Hint:      fmt.Sprintf("cub-scout compare three-way%s", nsFlag),
		}
	}

	return summary
}

func sortDoctorIssues(issues []DoctorIssue) {
	sort.Slice(issues, func(i, j int) bool {
		ri := doctorSeverityRank(issues[i].Severity)
		rj := doctorSeverityRank(issues[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if issues[i].Namespace != issues[j].Namespace {
			return issues[i].Namespace < issues[j].Namespace
		}
		if issues[i].Resource != issues[j].Resource {
			return issues[i].Resource < issues[j].Resource
		}
		return issues[i].Message < issues[j].Message
	})
}

func limitDoctorIssues(issues []DoctorIssue, topN int) []DoctorIssue {
	if topN < 0 {
		topN = 0
	}
	if topN > len(issues) {
		topN = len(issues)
	}
	return issues[:topN]
}

func doctorSeverityRank(sev string) int {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "CRITICAL":
		return 0
	case "WARNING":
		return 1
	case "INFO":
		return 2
	default:
		return 3
	}
}

func renderDoctorASCII(summary DoctorSummary, mode PresentationMode, explicitMode bool, hintCtx HintContext) string {
	var b strings.Builder

	// Helper to render section label based on whether presentation mode was explicit
	sectionLabel := func(label string) string {
		if explicitMode {
			return SectionLabel(mode, label)
		}
		return SectionHeader(label) + ":"
	}

	// Only apply presentation framing when explicitly requested
	if explicitMode {
		// Heading - varies by presentation mode
		heading := DoctorHeading(mode)
		if mode == PresentationAI {
			fmt.Fprintf(&b, "%s\n", heading)
		} else {
			fmt.Fprintf(&b, "%s\n", Bold(heading))
		}

		// Intro - varies by presentation mode
		intro := DoctorIntro(mode, summary.Cluster, summary.Namespace)
		fmt.Fprintf(&b, "%s\n", intro)
		fmt.Fprintf(&b, "%s %d total\n\n", SectionLabel(mode, "Resources"), summary.Resources.Total)
	} else {
		// Legacy format - no heading, just cluster line
		fmt.Fprintf(&b, "%s: %s (namespace: %s)\n", Bold("Cluster"), summary.Cluster, summary.Namespace)
		fmt.Fprintf(&b, "%s: %d total\n\n", Bold("Resources"), summary.Resources.Total)
	}

	total := summary.Resources.Total
	fmt.Fprintf(&b, "%s\n", sectionLabel("Ownership"))
	for _, owner := range []struct {
		name  string
		count int
	}{
		{name: "Flux", count: summary.Ownership.Flux},
		{name: "ArgoCD", count: summary.Ownership.ArgoCD},
		{name: "Sveltos", count: summary.Ownership.Sveltos},
		{name: "Modelplane", count: summary.Ownership.Modelplane},
		{name: "Crossplane", count: summary.Ownership.Crossplane},
		{name: "kro", count: summary.Ownership.Kro},
		{name: "Helm", count: summary.Ownership.Helm},
		{name: "Terraform", count: summary.Ownership.Terraform},
		{name: "ConfigHub", count: summary.Ownership.ConfigHub},
	} {
		if owner.count == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %s: %d (%d%%)\n", OwnerColor(owner.name), owner.count, doctorPercent(owner.count, total))
	}
	unmanagedText := fmt.Sprintf("%d unmanaged", summary.Ownership.Unmanaged)
	if summary.Ownership.Unmanaged > 0 {
		unmanagedText = Yellow(unmanagedText)
	}
	fmt.Fprintf(&b, "  %s: %d (%d%%)  <- %s\n", OwnerColor("Native"), summary.Ownership.Native, doctorPercent(summary.Ownership.Native, total), unmanagedText)
	if summary.Ownership.Other > 0 {
		fmt.Fprintf(&b, "  Other: %d (%d%%)\n", summary.Ownership.Other, doctorPercent(summary.Ownership.Other, total))
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprintf(&b, "%s\n", sectionLabel("Health"))
	fmt.Fprintf(&b, "  %s: %d\n", Green("Healthy"), summary.Health.Healthy)
	fmt.Fprintf(&b, "  %s: %d\n", Yellow("Warning"), summary.Health.Warning)
	fmt.Fprintf(&b, "  %s: %d\n\n", Red("Error"), summary.Health.Error)

	if summary.Rollouts != nil && summary.Rollouts.Total > 0 {
		fmt.Fprintf(&b, "%s %d workloads (%s, %s, %s, %s)\n",
			sectionLabel("Rollouts"),
			summary.Rollouts.Total,
			Green(fmt.Sprintf("%d PASS", summary.Rollouts.Pass)),
			Yellow(fmt.Sprintf("%d WATCH", summary.Rollouts.Watch)),
			Red(fmt.Sprintf("%d BLOCK", summary.Rollouts.Block)),
			Yellow(fmt.Sprintf("%d INCONCLUSIVE", summary.Rollouts.Inconclusive)),
		)
		for i, decision := range summary.Rollouts.CurrentChanges {
			id := decision.Resource
			ns := id.Namespace
			if ns == "" {
				ns = "-"
			}
			resource := fmt.Sprintf("%s/%s", id.Kind, id.Name)
			fmt.Fprintf(&b, "  %d. %s (ns: %s) - %s\n", i+1, resource, ns, colorExplainRolloutDecision(&decision))
		}
		fmt.Fprintf(&b, "\n")
	}

	if summary.Delivery != nil {
		delivery := summary.Delivery
		fmt.Fprintf(&b, "%s live-status %d (delivery: %s; app-health: %s)\n",
			sectionLabel("Delivery"),
			delivery.LiveStatus.Total,
			doctorVerdictCountsLine(delivery.LiveStatus.Delivery),
			doctorVerdictCountsLine(delivery.LiveStatus.ApplicationHealth),
		)
		if delivery.LiveStatus.Total > 0 {
			fmt.Fprintf(&b, "  Freshness: %d fresh, %d stale, %d unknown\n",
				delivery.LiveStatus.Fresh,
				delivery.LiveStatus.Stale,
				delivery.LiveStatus.UnknownFreshness,
			)
		}
		if delivery.EventConsumers.Total > 0 {
			fmt.Fprintf(&b, "  Event consumers: %d/%d ready\n",
				delivery.EventConsumers.Ready,
				delivery.EventConsumers.Total,
			)
		}
		if delivery.RecentReleases > 0 || delivery.RecentUnitEvents > 0 {
			fmt.Fprintf(&b, "  Recent evidence: %d releases, %d unit events\n",
				delivery.RecentReleases,
				delivery.RecentUnitEvents,
			)
		}
		if len(delivery.Omissions) > 0 {
			fmt.Fprintf(&b, "  Omissions: %d\n", len(delivery.Omissions))
			for i, omission := range delivery.Omissions {
				if i >= 3 {
					break
				}
				fmt.Fprintf(&b, "    - %s: %s\n", omission.Layer, omission.Reason)
			}
		}
		fmt.Fprintf(&b, "\n")
	}

	// Color severity counts in the risks line
	criticalText := fmt.Sprintf("%d CRITICAL", summary.Risks.Critical)
	warningText := fmt.Sprintf("%d WARNING", summary.Risks.Warning)
	infoText := fmt.Sprintf("%d INFO", summary.Risks.Info)
	if summary.Risks.Critical > 0 {
		criticalText = BoldRed(criticalText)
	}
	if summary.Risks.Warning > 0 {
		warningText = Yellow(warningText)
	}
	fmt.Fprintf(&b, "%s %d findings (%s, %s, %s)\n",
		sectionLabel("Risks"), summary.Risks.Total, criticalText, warningText, infoText)

	driftText := fmt.Sprintf("%d resources drifted from declared state", summary.Drift.Resources)
	if summary.Drift.Resources > 0 {
		driftText = Yellow(driftText)
	}
	fmt.Fprintf(&b, "%s %s\n\n", sectionLabel("Drift"), driftText)

	// Three-way status (connected mode only)
	if summary.ThreeWay != nil && summary.ThreeWay.Available {
		fmt.Fprintf(&b, "%s ConfigHub connected - run %s for full comparison\n\n",
			sectionLabel("Three-Way"), Cyan(summary.ThreeWay.Hint))
	}

	fmt.Fprintf(&b, "%s\n", sectionLabel("Top Issues"))
	if len(summary.TopIssues) == 0 {
		fmt.Fprintf(&b, "  %s\n", Dim("(none)"))
	} else {
		for i, issue := range summary.TopIssues {
			ns := issue.Namespace
			if ns == "" {
				ns = "-"
			}
			msg := issue.Message
			if msg == "" {
				msg = "no details"
			}
			severityColored := SeverityColor(issue.Severity)
			fmt.Fprintf(&b, "  %d. %s (ns: %s) - %s [%s]\n", i+1, issue.Resource, ns, msg, severityColored)
		}
	}

	// Outro - only for explicit AI mode
	if explicitMode {
		outro := DoctorOutro(mode)
		if outro != "" {
			fmt.Fprintf(&b, "\n%s\n", outro)
		}
	}

	hints := doctorTryNextHintsWithContext(summary, hintCtx)
	if len(hints) > 0 {
		if explicitMode {
			b.WriteString(renderTryNextSectionWithMode(hints, mode))
		} else {
			b.WriteString(renderTryNextSection(hints))
		}
	}

	return b.String()
}

func doctorVerdictCountsLine(counts DoctorVerdictCounts) string {
	return fmt.Sprintf("%d PASS, %d WATCH, %d BLOCK, %d INCONCLUSIVE",
		counts.Pass,
		counts.Watch,
		counts.Block,
		counts.Inconclusive,
	)
}

func doctorPercent(part, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(part) * 100.0 / float64(total))
}
