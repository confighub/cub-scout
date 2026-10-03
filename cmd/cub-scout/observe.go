// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// ObserveScopeSummaryRequest is the request for observe.scope_summary capability.
// This is the transport-agnostic input for scope summary operations.
type ObserveScopeSummaryRequest struct {
	// Namespace scope. Empty string means all namespaces.
	Namespace string

	// TopIssues is the number of top issues to include.
	TopIssues int

	// WithConfigHub includes bounded connected delivery evidence.
	WithConfigHub bool

	// ConfigHubSpace scopes connected delivery evidence. Empty means CUB_SPACE;
	// with neither, the reads are skipped and reported as an omission.
	ConfigHubSpace string

	// ConfigHubSince is the release/event lookback window.
	ConfigHubSince string

	// ConfigHubStaleAfter marks live-status writeback as stale after this age.
	ConfigHubStaleAfter string

	// FixturePath, if non-empty, reads input from a fixture file instead of the cluster.
	// This is for testing; callers should not set this in production.
	FixturePath string
	// ClusterBinding pins every Kubernetes read in this invocation.
	ClusterBinding *localClusterBinding
}

// ObserveScopeSummaryResult contains the summary and any warnings from the operation.
type ObserveScopeSummaryResult struct {
	Summary  DoctorSummary
	Warnings []string
}

// ObserveScopeSummary returns a cluster/namespace scope summary.
// This is the transport-agnostic seam for the doctor command.
//
// The function does not know about Cobra, stdout, presentation modes, or rendering.
// It returns the canonical DoctorSummary model which callers can render as needed.
// Any warnings (e.g., scan degradation) are returned in the result rather than
// written to stderr, so callers can decide how to present them.
func ObserveScopeSummary(ctx context.Context, req ObserveScopeSummaryRequest) (ObserveScopeSummaryResult, error) {
	namespaceLabel := "all"
	if strings.TrimSpace(req.Namespace) != "" {
		namespaceLabel = req.Namespace
	}

	topN := req.TopIssues
	if topN < 0 {
		topN = 0
	}
	if req.WithConfigHub {
		if err := validateDoctorConfigHubRequest(req); err != nil {
			return ObserveScopeSummaryResult{}, err
		}
	}

	// Use fixture if explicitly provided
	if req.FixturePath != "" {
		summary, err := observeScopeSummaryFromFixture(req.FixturePath, namespaceLabel, topN)
		return ObserveScopeSummaryResult{Summary: summary}, err
	}

	if req.ClusterBinding == nil {
		binding := resolveLocalClusterBindingForSelection(clusterContextSelection{})
		if binding.err != nil {
			return ObserveScopeSummaryResult{}, binding.err
		}
		req.ClusterBinding = binding
	}
	return observeScopeSummaryFromCluster(ctx, req.Namespace, namespaceLabel, topN, req)
}

func observeScopeSummaryFromFixture(path, namespaceLabel string, topN int) (DoctorSummary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return DoctorSummary{}, fmt.Errorf("read doctor fixture: %w", err)
	}
	var in doctorFixtureInput
	if err := json.Unmarshal(b, &in); err != nil {
		return DoctorSummary{}, fmt.Errorf("parse doctor fixture: %w", err)
	}
	cluster := strings.TrimSpace(in.Cluster)
	if cluster == "" {
		cluster = getClusterName()
	}
	return buildDoctorSummary(in.Entries, in.Findings, cluster, namespaceLabel, topN), nil
}

func observeScopeSummaryFromCluster(ctx context.Context, namespace, namespaceLabel string, topN int, req ObserveScopeSummaryRequest) (ObserveScopeSummaryResult, error) {
	var result ObserveScopeSummaryResult

	entries, cluster, err := collectDoctorEntriesWithBinding(ctx, namespace, req.ClusterBinding)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("inventory coverage incomplete: %v", err))
	}

	findings, err := collectDoctorFindingsWithBinding(ctx, namespace, req.ClusterBinding)
	if err != nil {
		// Degrade gracefully if scanning is unavailable.
		// Return warning in result rather than writing to stderr.
		result.Warnings = append(result.Warnings, fmt.Sprintf("risk scan unavailable: %v", err))
	}

	result.Summary = buildDoctorSummary(entries, findings, cluster, namespaceLabel, topN)
	if req.ClusterBinding != nil {
		result.Summary.KubernetesContext = req.ClusterBinding.context
		if req.ClusterBinding.explicit {
			// The existing three-way command has no explicit Kubernetes selector.
			result.Summary.ThreeWay = nil
		}
	}
	rollouts, rolloutsErr := collectDoctorRolloutsWithBinding(ctx, namespace, topN, req.ClusterBinding)
	if rolloutsErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("rollout evidence unavailable: %v", rolloutsErr))
	}
	if rollouts != nil && (rollouts.Total > 0 || rolloutsErr != nil) {
		result.Summary.Rollouts = rollouts
	}
	if req.WithConfigHub {
		evidence, evidenceErr := collectDoctorDeliveryEvidenceFn(ctx, namespace, req, req.ClusterBinding)
		if evidenceErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("ConfigHub delivery evidence unavailable: %v", evidenceErr))
		} else {
			attachDoctorDeliveryEvidence(&result.Summary, evidence, topN)
		}
	}
	result.Summary.Warnings = append([]string(nil), result.Warnings...)
	return result, nil
}

// ObserveResourceContextRequest is the request for observe.resource_context capability.
// This is the transport-agnostic input for resource context operations.
type ObserveResourceContextRequest struct {
	// Kind is the resource kind (e.g., "Deployment", "Service").
	Kind string

	// Name is the resource name.
	Name string

	// Namespace is the resource namespace. Defaults to "default" if empty.
	Namespace string

	// FieldPath requests compact manager evidence for one exact canonical path.
	FieldPath string
}

// ObserveResourceContext returns ownership and lineage context for a resource.
// This is the transport-agnostic seam for the explain command.
//
// The function does not know about Cobra, stdout, presentation modes, or rendering.
// It returns the canonical ExplainSummary model which callers can render as needed.
func ObserveResourceContext(ctx context.Context, req ObserveResourceContextRequest) (ExplainSummary, error) {
	if req.FieldPath != "" {
		if err := agent.ValidateCanonicalFieldPath(req.FieldPath); err != nil {
			return ExplainSummary{}, err
		}
	}
	kind := normalizeKind(req.Kind)
	name := req.Name
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = "default"
	}

	traceResult, err := traceForExplain(ctx, kind, name, ns)
	if err != nil {
		summary := buildExplainSummaryFromFailure(kind, name, ns, err)
		if req.FieldPath != "" {
			if attr, fieldAttr, ok := fetchResourceAttribution(ctx, ns, kind, name, req.FieldPath); ok && attr.Cause != "" {
				summary.MutationCause = attr.Cause
				summary.MutationManager = attr.ManagerHint
				summary.FieldAttribution = fieldAttr
			} else {
				summary.FieldAttribution = unavailableFieldAttribution(req.FieldPath, "Exact field evidence unavailable because the live resource could not be read.")
			}
		}
		return summary, nil
	}

	if explainWithConfigHub {
		dynClient := enrichTraceConfigHubFromLive(ctx, traceResult, kind, name, ns)
		attachTraceConfigHubDeliveryEvidence(ctx, traceResult, dynClient, traceConfigHubDeliveryFlags{
			Enabled:    true,
			Namespace:  ns,
			Space:      explainConfigHubSpace,
			Since:      explainConfigHubSince,
			StaleAfter: explainConfigHubStaleAfter,
		})
	}

	summary := buildExplainSummary(traceResult)
	if summary.Resource == "" {
		summary.Resource = fmt.Sprintf("%s/%s", kind, name)
	}
	if summary.Namespace == "" {
		summary.Namespace = ns
	}

	// Fetch recent events for the resource
	events, eventsErr := fetchResourceEvents(ctx, ns, kind, name)
	if eventsErr == nil && events != nil && len(events.Events) > 0 {
		summary.Events = events
	}

	// Check for three-way disagreement in connected mode.
	// Note: failure details (sync status) would need to be fetched separately
	// from the deployer resource - for now we pass nil and rely on DRY/WET/LIVE comparison.
	threeWay, err := buildThreeWayDisagreement(ctx, kind, name, ns, nil)
	if err == nil && threeWay != nil && threeWay.IsDisagreement() {
		summary.ThreeWay = threeWay
	}

	// Attribute the cause of any recent field mutations from metadata.managedFields.
	// Best-effort — silently skipped on fetch failure, so explain still works
	// without cluster access. Per the parse-don't-guess rule, missing or
	// unrecognized signals yield CauseUnknown rather than misclassification.
	if attr, fieldAttr, ok := fetchResourceAttribution(ctx, ns, kind, name, req.FieldPath); ok && attr.Cause != "" {
		summary.MutationCause = attr.Cause
		summary.MutationManager = attr.ManagerHint
		summary.FieldAttribution = fieldAttr
	} else if req.FieldPath != "" {
		summary.FieldAttribution = unavailableFieldAttribution(req.FieldPath, "Exact field evidence unavailable because the live resource could not be read.")
	}

	if decision, ok := fetchRolloutDecision(ctx, ns, kind, name); ok {
		summary.CurrentChange = decision
	}

	return summary, nil
}

func fetchRolloutDecision(ctx context.Context, namespace, kind, name string) (*agent.RolloutDecision, bool) {
	if !agent.IsRolloutWorkloadKind(kind) {
		return nil, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := buildConfig()
	if err != nil {
		return nil, false
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, false
	}

	return fetchRolloutDecisionFrom(ctx, dynClient, namespace, kind, name)
}

// fetchRolloutDecisionFrom keeps the workload and related Pod reads on one
// caller-supplied client. Captured sessions can use this without reloading the
// ambient kubeconfig. Missing workload evidence remains best-effort unavailable;
// Pod read failures retain the existing workload-only decision behavior.
func fetchRolloutDecisionFrom(ctx context.Context, dynClient dynamic.Interface, namespace, kind, name string) (*agent.RolloutDecision, bool) {
	decision, ok, _ := fetchRolloutDecisionFromWithError(ctx, dynClient, namespace, kind, name)
	return decision, ok
}

func fetchRolloutDecisionFromWithError(ctx context.Context, dynClient dynamic.Interface, namespace, kind, name string) (*agent.RolloutDecision, bool, error) {
	if dynClient == nil || !agent.IsRolloutWorkloadKind(kind) {
		return nil, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return nil, false, nil
	}
	obj, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	pods, podsErr := relatedPodsForRolloutDecisionWithError(ctx, dynClient, namespace, obj)
	decision, ok := agent.BuildRolloutDecisionForWorkload(obj, pods, 0, time.Now().UTC())
	if !ok {
		if podsErr != nil {
			return nil, false, podsErr
		}
		return nil, false, fmt.Errorf("current-change workload metadata unavailable")
	}
	return &decision, true, podsErr
}

func relatedPodsForRolloutDecision(ctx context.Context, dynClient dynamic.Interface, namespace string, obj *unstructured.Unstructured) []*unstructured.Unstructured {
	pods, _ := relatedPodsForRolloutDecisionWithError(ctx, dynClient, namespace, obj)
	return pods
}

func relatedPodsForRolloutDecisionWithError(ctx context.Context, dynClient dynamic.Interface, namespace string, obj *unstructured.Unstructured) ([]*unstructured.Unstructured, error) {
	selector := workloadSelectorMatchLabels(obj)
	if len(selector) == 0 || namespace == "" {
		return nil, fmt.Errorf("related Pod scope unavailable: workload selector or namespace missing")
	}
	podsGVR := kindToGVR("Pod")
	if podsGVR.Resource == "" {
		return nil, nil
	}
	labelSelector, err := v1.LabelSelectorAsSelector(&v1.LabelSelector{MatchLabels: selector})
	if err != nil {
		return nil, err
	}
	list, err := dynClient.Resource(podsGVR).Namespace(namespace).List(ctx, v1.ListOptions{LabelSelector: labelSelector.String()})
	if err != nil {
		return nil, err
	}
	pods := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		item := list.Items[i]
		pods = append(pods, &item)
	}
	return matchPodsBySelector(pods, selector), nil
}

// fetchResourceAttribution loads the live resource and computes mutation-source
// attribution from metadata.managedFields. Returns false on any fetch or
// classification failure — attribution is purely additive evidence and must
// not block explain output.
func fetchResourceAttribution(ctx context.Context, namespace, kind, name, fieldPath string) (agent.FieldMutationAttribution, *FieldAttributionSummary, bool) {
	cfg, err := buildConfig()
	if err != nil {
		return agent.FieldMutationAttribution{}, nil, false
	}

	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return agent.FieldMutationAttribution{}, nil, false
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return agent.FieldMutationAttribution{}, nil, false
	}

	obj, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return agent.FieldMutationAttribution{}, nil, false
	}

	owner := agent.DetectOwnership(obj)
	attr := agent.AttributeFieldMutation(obj, owner)
	if fieldPath == "" {
		return attr, nil, true
	}
	fieldAttr := fieldAttributionSummary(obj, owner, fieldPath)
	return attr, fieldAttr, true
}

func fieldAttributionSummary(obj *unstructured.Unstructured, owner agent.Ownership, path string) *FieldAttributionSummary {
	attr, ok, incomplete := agent.AttributeFieldPath(obj, owner, path)
	return fieldAttributionResult(path, attr, ok, incomplete)
}

func unavailableFieldAttribution(path, reason string) *FieldAttributionSummary {
	return &FieldAttributionSummary{Path: path, Cause: agent.CauseUnknown, Reason: reason}
}

func fieldAttributionResult(path string, attr agent.FieldMutationAttribution, ok, incomplete bool) *FieldAttributionSummary {
	result := &FieldAttributionSummary{Path: path, Cause: agent.CauseUnknown}
	if incomplete {
		if ok {
			result.Managers = attr.Managers
		}
		result.Reason = "Some managedFields entries were malformed; observed managers may be incomplete."
		return result
	}
	if !ok {
		result.Reason = "No decodable managedFields entry claims this exact path."
		return result
	}
	result.Cause = attr.Cause
	result.Managers = attr.Managers
	if result.Cause == agent.CauseUnknown {
		result.Reason = "Managers claim this path, but the evidence is insufficient to select a cause."
	}
	return result
}

// fetchResourceEvents fetches recent events for a resource.
// Returns nil if cluster is unavailable or events cannot be fetched.
func fetchResourceEvents(ctx context.Context, namespace, kind, name string) (*agent.ResourceEventSummary, error) {
	cfg, err := buildConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	fetcher := agent.NewEventTimelineFetcher(clientset)
	return fetcher.FetchRecentEvents(ctx, namespace, kind, name, 5)
}
