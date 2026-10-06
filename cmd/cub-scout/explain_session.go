// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

var newExplainSessionForSelection = newTraceSessionForSelection

type explainObservationOptions struct {
	FieldPath string
	Delivery  traceConfigHubDeliveryFlags
	Flux      func(*traceSession) (agent.Tracer, func() error, error)
}

func bindExplainHintSelection(hints []Hint, selected string) []Hint {
	if selected == "" {
		return hints
	}
	quoted := "'" + strings.ReplaceAll(selected, "'", "'\"'\"'") + "'"
	for i := range hints {
		command := strings.TrimSpace(hints[i].Command)
		if command == "" {
			continue
		}
		words := strings.Fields(command)
		if len(words) > 0 && (words[0] == "cub-scout" || words[0] == "./cub-scout") {
			words = words[1:]
		} else {
			words = nil
		}
		supported := len(words) > 0 && (words[0] == "explain" || words[0] == "trace" || words[0] == "scan" || words[0] == "doctor" || (len(words) > 1 && ((words[0] == "map" && words[1] == "list") || (words[0] == "gitops" && words[1] == "status"))))
		if supported && !strings.Contains(command, "--kube-context") {
			hints[i].Command = command + " --kube-context " + quoted
		} else {
			hints[i].Command = ""
			hints[i].Rationale += " (no context-bound command is available for this selection)"
		}
	}
	return hints
}

func explainTryNextHintsForSelection(summary ExplainSummary, ctx HintContext) []string {
	if summary.KubernetesContext == "" {
		return explainTryNextHintsWithContext(summary, ctx)
	}
	hints := bindExplainHintSelection(explainHintsWithContext(summary, ctx), summary.KubernetesContext)
	sortHints(hints)
	if len(hints) > 3 {
		hints = hints[:3]
	}
	return hintsToStrings(hints)
}

// observeExplainWithSession shares one immutable Kubernetes binding across all
// enriched reads. ConfigHub reads retain their independently selected space.
func observeExplainWithSession(ctx context.Context, session *traceSession, kind, name, namespace string, opts explainObservationOptions) (ExplainSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ExplainSummary{}, err
	}
	if _, err := session.restConfig(); err != nil {
		return ExplainSummary{}, err
	}
	if opts.FieldPath != "" {
		if err := agent.ValidateCanonicalFieldPath(opts.FieldPath); err != nil {
			return ExplainSummary{}, err
		}
	}
	kind = normalizeKind(kind)
	if namespace = strings.TrimSpace(namespace); namespace == "" {
		namespace = "default"
	}
	summary := buildExplainSummaryFromFailure(kind, name, namespace, nil)
	summary.Owner = "Unknown"
	summary.KubernetesContext = session.contextLabel()
	omit := func(layer string, err error) {
		if err == nil {
			return
		}
		summary.Omissions = append(summary.Omissions, agent.Omission{Missing: layer, Reason: err.Error(), Severity: "inconclusive"})
		summary.Notes = append(summary.Notes, layer+" unavailable: "+err.Error())
	}
	if opts.Flux == nil {
		opts.Flux = capturedTraceFluxFactory
	}
	dyn, err := session.dynamicClient()
	if err != nil {
		return ExplainSummary{}, err
	}
	obj, err := fetchTraceResource(ctx, dyn, kind, name, namespace)
	if kind == "ProviderConfig" {
		obj, err = fetchProviderConfigResourceWithTraceSession(ctx, session, dyn, name, namespace)
	}
	if err != nil {
		omit("ownership", err)
		if opts.FieldPath != "" {
			summary.FieldAttribution = unavailableFieldAttribution(opts.FieldPath, "Selected live object could not be read.")
		}
		return summary, ctx.Err()
	}
	owner := agent.DetectOwnership(obj)
	if owner.Type == agent.OwnerUnknown && isCrossplaneProviderConfig(obj) {
		owner = agent.Ownership{Type: agent.OwnerCrossplane, SubType: "providerconfig", Name: obj.GetName(), Namespace: obj.GetNamespace(), Source: "apiGroup:" + obj.GroupVersionKind().Group, Confidence: "high"}
	}
	result := buildOwnershipOnlyTraceResult(kind, name, namespace, &owner, nil)
	result.DetectedOwner = owner.Type
	// Native ownership is metadata evidence; trying an unrelated CLI tracer
	// cannot establish a controller chain and is unnecessary.
	var traceErr error
	if !isNativeOwnership(&owner) {
		observation, observeErr := observeTrace(ctx, session, kind, name, namespace, traceObservationOptions{Flux: opts.Flux})
		traceErr = observeErr
		if observation != nil && observation.Result != nil {
			result = observation.Result
		}
	}
	result.Object = agent.ResourceRef{Kind: kind, Name: name, Namespace: namespace}
	if opts.Delivery.Enabled {
		enrichTraceConfigHubFromObject(result, obj)
		flags := opts.Delivery
		flags.Namespace = namespace
		attachTraceConfigHubDeliveryEvidenceWithTraceSession(ctx, result, dyn, session, flags)
	}
	summary = buildExplainSummary(result)
	summary.KubernetesContext = session.contextLabel()
	omit("controller-chain", traceErr)
	if result.Error != "" {
		omit("trace-enrichment", fmt.Errorf("%s", result.Error))
	}
	attr := agent.AttributeFieldMutation(obj, owner)
	summary.MutationCause, summary.MutationManager = attr.Cause, attr.ManagerHint
	if opts.FieldPath != "" {
		summary.FieldAttribution = fieldAttributionSummary(obj, owner, opts.FieldPath)
	}
	events, eventErr := fetchResourceEventsWithTraceSession(ctx, session, namespace, kind, name)
	if events != nil && len(events.Events) > 0 {
		summary.Events = events
	}
	omit("events", eventErr)
	decision, _, decisionErr := fetchRolloutDecisionFromWithError(ctx, dyn, namespace, kind, name)
	summary.CurrentChange = decision
	omit("rollout", decisionErr)
	if requireConfigHubFor("three-way comparison") == nil {
		comparison, compareErr := buildCompareResourceResultWithOptions(ctx, kind+"/"+name, namespace, compareResourceOptions{
			ReportBindingErrors: true,
			Live: func(ctx context.Context, kind, name, ns string) (compareSideSummary, error) {
				return loadCompareLiveSnapshotWithTraceSessionAndFlux(ctx, session, kind, name, ns, opts.Flux)
			},
		})
		omit("three-way", compareErr)
		for _, note := range comparison.Notes {
			summary.Notes = append(summary.Notes, "three-way: "+note)
			summary.Omissions = append(summary.Omissions, agent.Omission{Missing: "three-way", Reason: note, Severity: "inconclusive"})
		}
		if compareErr == nil && comparison.Dry != nil && comparison.Wet != nil && len(comparison.Notes) == 0 {
			syncStatus, healthStatus := "unknown", ""
			d := classifyThreeWayPattern(comparison, syncStatus, healthStatus)
			if d.IsDisagreement() {
				d.Pattern = PatternUnknown
				d.Meaning = "Observed configuration values differ; controller sync progress was not established."
				summary.ThreeWay = d
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return ExplainSummary{}, err
	}
	return summary, nil
}
