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

	"github.com/confighub/cub-scout/pkg/agent"
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

	// FixturePath, if non-empty, reads input from a fixture file instead of the cluster.
	// This is for testing; callers should not set this in production.
	FixturePath string
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

	// Use fixture if explicitly provided
	if req.FixturePath != "" {
		summary, err := observeScopeSummaryFromFixture(req.FixturePath, namespaceLabel, topN)
		return ObserveScopeSummaryResult{Summary: summary}, err
	}

	return observeScopeSummaryFromCluster(ctx, req.Namespace, namespaceLabel, topN)
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

func observeScopeSummaryFromCluster(ctx context.Context, namespace, namespaceLabel string, topN int) (ObserveScopeSummaryResult, error) {
	var result ObserveScopeSummaryResult

	entries, cluster, err := collectDoctorEntries(ctx, namespace)
	if err != nil {
		// Return raw error - let caller decide how to phrase recovery hints
		return result, err
	}

	findings, err := collectDoctorFindings(ctx, namespace)
	if err != nil {
		// Degrade gracefully if scanning is unavailable.
		// Return warning in result rather than writing to stderr.
		result.Warnings = append(result.Warnings, fmt.Sprintf("risk scan unavailable: %v", err))
		findings = nil
	}

	result.Summary = buildDoctorSummary(entries, findings, cluster, namespaceLabel, topN)
	rollouts, rolloutsErr := collectDoctorRollouts(ctx, namespace, topN)
	if rolloutsErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("rollout evidence unavailable: %v", rolloutsErr))
	} else if rollouts != nil && rollouts.Total > 0 {
		result.Summary.Rollouts = rollouts
	}
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
}

// ObserveResourceContext returns ownership and lineage context for a resource.
// This is the transport-agnostic seam for the explain command.
//
// The function does not know about Cobra, stdout, presentation modes, or rendering.
// It returns the canonical ExplainSummary model which callers can render as needed.
func ObserveResourceContext(ctx context.Context, req ObserveResourceContextRequest) (ExplainSummary, error) {
	kind := normalizeKind(req.Kind)
	name := req.Name
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = "default"
	}

	traceResult, err := traceForExplain(ctx, kind, name, ns)
	if err != nil {
		return buildExplainSummaryFromFailure(kind, name, ns, err), nil
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
	if attr, ok := fetchResourceAttribution(ctx, ns, kind, name); ok && attr.Cause != "" {
		summary.MutationCause = attr.Cause
		summary.MutationManager = attr.ManagerHint
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
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return nil, false
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, false
	}

	obj, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return nil, false
	}

	pods := relatedPodsForRolloutDecision(ctx, dynClient, namespace, obj)
	decision, ok := agent.BuildRolloutDecisionForWorkload(obj, pods, 0, time.Now().UTC())
	if !ok {
		return nil, false
	}
	return &decision, true
}

func relatedPodsForRolloutDecision(ctx context.Context, dynClient dynamic.Interface, namespace string, obj *unstructured.Unstructured) []*unstructured.Unstructured {
	selector := workloadSelectorMatchLabels(obj)
	if len(selector) == 0 || namespace == "" {
		return nil
	}
	podsGVR := kindToGVR("Pod")
	if podsGVR.Resource == "" {
		return nil
	}
	labelSelector, err := v1.LabelSelectorAsSelector(&v1.LabelSelector{MatchLabels: selector})
	if err != nil {
		return nil
	}
	list, err := dynClient.Resource(podsGVR).Namespace(namespace).List(ctx, v1.ListOptions{LabelSelector: labelSelector.String()})
	if err != nil {
		return nil
	}
	pods := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		item := list.Items[i]
		pods = append(pods, &item)
	}
	return matchPodsBySelector(pods, selector)
}

// fetchResourceAttribution loads the live resource and computes mutation-source
// attribution from metadata.managedFields. Returns false on any fetch or
// classification failure — attribution is purely additive evidence and must
// not block explain output.
func fetchResourceAttribution(ctx context.Context, namespace, kind, name string) (agent.FieldMutationAttribution, bool) {
	cfg, err := buildConfig()
	if err != nil {
		return agent.FieldMutationAttribution{}, false
	}

	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return agent.FieldMutationAttribution{}, false
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return agent.FieldMutationAttribution{}, false
	}

	obj, err := dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	if err != nil {
		return agent.FieldMutationAttribution{}, false
	}

	owner := agent.DetectOwnership(obj)
	return agent.AttributeFieldMutation(obj, owner), true
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
