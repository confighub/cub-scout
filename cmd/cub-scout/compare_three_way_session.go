// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// threeWayOptions is invocation-owned. Async UI work never reads command flags.
type threeWayOptions struct {
	LocalDry     bool // Explicit --dry-from declaration, including an empty loaded operand.
	Namespace    string
	FailOn       string
	DrySummaries []*compareSideSummary
	SourcePath   string
	// Flux is the captured controller adapter; nil selects the production factory.
	Flux func(*traceSession) (agent.Tracer, func() error, error)
}

type threeWayOmission struct {
	Phase     string `json:"phase"`
	Resource  string `json:"resource,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Reason    string `json:"reason"`
}

func (o threeWayOmission) String() string {
	return fmt.Sprintf("%s %s namespace=%s: %s", o.Phase, o.Resource, o.Namespace, o.Reason)
}

func threeWayFailureReason(err error) string {
	switch {
	case err == nil:
		return ""
	case apierrors.IsForbidden(err):
		return "forbidden"
	case apierrors.IsUnauthorized(err):
		return "unauthorized"
	case apierrors.IsNotFound(err):
		return "not_found"
	case err == context.Canceled || strings.Contains(err.Error(), "context canceled"):
		return "canceled"
	case err == context.DeadlineExceeded || strings.Contains(err.Error(), "deadline exceeded"):
		return "timeout"
	default:
		return "unavailable"
	}
}

func collectThreeWayForSelection(ctx context.Context, selection clusterContextSelection, scope threeWayScope, options threeWayOptions) (threeWayReport, error) {
	session, err := newTraceSessionForSelection(selection)
	if err != nil {
		return threeWayReport{}, fmt.Errorf("resolve selected Kubernetes context: %w", err)
	}
	return collectThreeWayWithSession(ctx, session, scope, options)
}

func (options threeWayOptions) hasLocalDry() bool {
	return options.LocalDry || options.DrySummaries != nil
}

func validateThreeWayViewEligibility(scope threeWayScope, options threeWayOptions) error {
	if scope.ScopeType != threeWayScopeView {
		return nil
	}
	if options.hasLocalDry() {
		return fmt.Errorf("--dry-from and --view are mutually exclusive (--view requires connected mode)")
	}
	return requireConfigHubFor("compare three-way --view")
}

// collectThreeWayWithSession is shared by CLI, MCP dispatch and both TUI entries.
// Context is a display label, never a ConfigHub Target or stable cluster ID.
func collectThreeWayWithSession(ctx context.Context, session *traceSession, scope threeWayScope, options threeWayOptions) (threeWayReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return threeWayReport{}, err
	}
	if err := validateThreeWayViewEligibility(scope, options); err != nil {
		return threeWayReport{}, err
	}
	if options.LocalDry && options.DrySummaries == nil {
		options.DrySummaries = []*compareSideSummary{}
	}
	if _, err := session.restConfig(); err != nil {
		return threeWayReport{}, err
	}
	targets, omissions, err := collectThreeWayTargetsWithSession(ctx, session, scope, options.Namespace)
	if err != nil {
		return threeWayReport{}, err
	}
	flux := options.Flux
	if flux == nil {
		flux = capturedTraceFluxFactory
	}
	build := func(ctx context.Context, resource, namespace string) (compareResourceResult, error) {
		loader := func(ctx context.Context, kind, name, ns string) (compareSideSummary, error) {
			live, err := loadCompareLiveSnapshotWithTraceSessionAndFlux(ctx, session, kind, name, ns, flux)
			if err != nil {
				omissions = append(omissions, threeWayOmission{Phase: "live-source-link", Resource: kind + "/" + name, Namespace: ns, Reason: threeWayFailureReason(err)})
			}
			if live.Kind != "" && live.GitSource == nil {
				omissions = append(omissions, threeWayOmission{Phase: "source-coverage", Resource: kind + "/" + name, Namespace: ns, Reason: "source_anchor_unavailable"})
			}
			if err != nil {
				return live, fmt.Errorf("LIVE source/link evidence %s", threeWayFailureReason(err))
			}
			return live, nil
		}
		result, err := buildCompareResourceResultWithOptions(ctx, resource, namespace, compareResourceOptions{ReportBindingErrors: true, Live: loader, DrySummaries: options.DrySummaries, SourcePath: options.SourcePath})
		if err != nil {
			if ctx.Err() != nil {
				return compareResourceResult{}, ctx.Err()
			}
			// A missing/denied object is unknown evidence, not absence or orphanhood.
			return compareResourceResult{Resource: resource, Namespace: namespace, Mode: "live-unavailable", Notes: []string{"LIVE snapshot unavailable: " + threeWayFailureReason(err)}}, nil
		}
		for _, note := range result.Notes {
			if strings.HasPrefix(note, "ConfigHub binding enrichment incomplete:") {
				omissions = append(omissions, threeWayOmission{Phase: "confighub-bindings", Resource: resource, Namespace: namespace, Reason: strings.TrimSpace(strings.TrimPrefix(note, "ConfigHub binding enrichment incomplete:"))})
			}
		}
		return result, nil
	}
	rollout := func(ctx context.Context, target threeWayTarget) (*agent.RolloutDecision, bool, error) {
		kind, name, err := parseResourceArg(target.ResourceArg)
		if err != nil {
			return nil, false, err
		}
		if !agent.IsRolloutWorkloadKind(normalizeKind(kind)) {
			return nil, false, nil
		}
		dyn, err := session.dynamicClient()
		if err != nil {
			return nil, false, err
		}
		decision, ok, err := fetchRolloutDecisionFromWithError(ctx, dyn, target.Namespace, normalizeKind(kind), name)
		if err != nil {
			omissions = append(omissions, threeWayOmission{Phase: "current-change", Resource: target.ResourceArg, Namespace: target.Namespace, Reason: threeWayFailureReason(err)})
		}
		return decision, ok, err
	}
	report, err := assembleThreeWayReport(ctx, scope, options.FailOn, targets, build, rollout)
	if err != nil {
		return threeWayReport{}, err
	}
	if err := ctx.Err(); err != nil {
		return threeWayReport{}, err
	}
	if scope.ScopeType == threeWayScopeView {
		report.ViewSelection = &threeWayViewSelection{Space: "*", Membership: "exact UnitID or space ID plus unit slug; Kubernetes namespace retained"}
	}
	for _, entry := range report.Resources {
		if entry.Result.Dry == nil {
			omissions = append(omissions, threeWayOmission{Phase: "comparison-sides", Resource: entry.Result.Resource, Namespace: entry.Result.Namespace, Reason: "dry_unavailable"})
		}
		if entry.Result.Wet == nil {
			omissions = append(omissions, threeWayOmission{Phase: "comparison-sides", Resource: entry.Result.Resource, Namespace: entry.Result.Namespace, Reason: "wet_unavailable"})
		}
	}
	report.Context, report.Omissions = session.contextLabel(), omissions
	incomplete := len(omissions) > 0
	for _, entry := range report.Resources {
		// Missing or partial sides cannot establish end-to-end agreement. Local
		// rendered DRY is deliberately separate from source-truth/controller WET.
		if entry.Result.Live.Kind == "" || entry.Result.Dry == nil || entry.Result.Wet == nil || len(entry.Result.Notes) > 0 {
			incomplete = true
		}
	}
	if incomplete {
		report.Summary.Agreement.State = StatePartial
		report.Summary.Agreement.Summary = "Comparison evidence is incomplete; agreement is not established"
		report.Summary.Agreement.Reasons = append(report.Summary.Agreement.Reasons, "Missing or denied reads and unavailable comparison sides remain unknown")
	}
	report.ConfigHubURL, report.ConfigHubRevisionsURL, report.NextSteps = buildThreeWayNavigation(report)
	return report, nil
}

// Discovery lists the supported workload kinds with the caller context. An
// all-namespaces list covers cluster/View scopes including system namespaces.
// Partial lists are retained and errors remain visible even with zero targets.
func collectThreeWayTargetsWithSession(ctx context.Context, session *traceSession, scope threeWayScope, namespace string) ([]threeWayTarget, []threeWayOmission, error) {
	if scope.ScopeType == threeWayScopeResource {
		kind, name, err := parseResourceArg(scope.ScopeValue)
		if err != nil {
			return nil, nil, err
		}
		ns := strings.TrimSpace(namespace)
		if ns == "" {
			ns = "default"
		}
		return []threeWayTarget{{ResourceArg: normalizeKind(kind) + "/" + name, Namespace: ns}}, nil, nil
	}
	var viewUnits []threeWayViewUnit
	var selectionOmissions []threeWayOmission
	switch scope.ScopeType {
	case threeWayScopeNamespace:
		namespace = strings.TrimSpace(scope.ScopeValue)
	case threeWayScopeCluster:
		namespace = ""
	case threeWayScopeView:
		if err := requireConfigHubFor("compare three-way --view"); err != nil {
			return nil, nil, err
		}
		namespace = ""
		ref, err := agent.ParseViewRef(scope.ScopeValue)
		if err != nil {
			return nil, nil, fmt.Errorf("parse view reference: %w", err)
		}
		view, err := fetchView(ctx, ref.UUID, "*")
		if err != nil {
			return nil, nil, fmt.Errorf("fetch view: %w", err)
		}
		where, err := extractWhereClause(view)
		if err != nil {
			return nil, nil, err
		}
		if where == "" {
			return nil, nil, fmt.Errorf("view has no Where filter")
		}
		units, err := listUnitsForFilter(ctx, where, "*")
		if err != nil {
			return nil, nil, err
		}
		if len(units) == 0 {
			return nil, nil, fmt.Errorf("view filter matched no units")
		}
		viewUnits = make([]threeWayViewUnit, 0, len(units))
		for _, row := range units {
			unit := threeWayViewUnitFromRow(row)
			viewUnits = append(viewUnits, unit)
			if unit.ID == "" && (unit.SpaceID == "" || unit.Slug == "") {
				selectionOmissions = append(selectionOmissions, threeWayOmission{Phase: "view-selection", Resource: unit.Slug, Reason: "unit_identity_missing"})
			}
		}
	default:
		return nil, nil, fmt.Errorf("unsupported scope type %q", scope.ScopeType)
	}
	dyn, err := session.dynamicClient()
	if err != nil {
		return nil, nil, err
	}
	var targets []threeWayTarget
	omissions := selectionOmissions
	for _, kind := range []string{"Deployment", "StatefulSet", "DaemonSet"} {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		list, err := dyn.Resource(kindToGVR(kind)).Namespace(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			omissions = append(omissions, threeWayOmission{Phase: "discovery", Resource: kind, Namespace: namespace, Reason: threeWayFailureReason(err)})
			continue
		}
		for _, obj := range list.Items {
			if namespace != "" && obj.GetNamespace() != namespace {
				omissions = append(omissions, threeWayOmission{Phase: "discovery", Resource: kind + "/" + obj.GetName(), Namespace: namespace, Reason: "scope_mismatch"})
				continue
			}
			if obj.GetName() == "" || obj.GetNamespace() == "" {
				omissions = append(omissions, threeWayOmission{Phase: "discovery", Resource: kind, Namespace: namespace, Reason: "identity_missing"})
				continue
			}
			if viewUnits != nil {
				member, reason := threeWayViewMembership(obj, viewUnits)
				if reason != "" {
					omissions = append(omissions, threeWayOmission{Phase: "view-membership", Resource: kind + "/" + obj.GetName(), Namespace: obj.GetNamespace(), Reason: reason})
				}
				if !member {
					continue
				}
			}
			targets = append(targets, threeWayTarget{ResourceArg: kind + "/" + obj.GetName(), Namespace: obj.GetNamespace()})
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Namespace+"/"+targets[i].ResourceArg < targets[j].Namespace+"/"+targets[j].ResourceArg
	})
	return targets, omissions, nil
}

// Keep generated follow-ups safe when a context label contains shell syntax.
func shellQuoteArg(value string) string {
	if value != "" && !strings.ContainsAny(value, " \t\n'\"`$;&|<>()\\") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// View membership is scoped to identities returned by the ConfigHub filter.
// A slug alone is not unique across the all-space query.
type threeWayViewUnit struct{ ID, SpaceID, Slug string }

func threeWayViewUnitFromRow(row map[string]interface{}) threeWayViewUnit {
	if unit, ok := row["Unit"].(map[string]interface{}); ok {
		row = unit
	}
	return threeWayViewUnit{ID: firstNonEmpty(readStringField(row, "UnitID"), readStringField(row, "unitID"), readStringField(row, "unitId")), SpaceID: firstNonEmpty(readStringField(row, "SpaceID"), readStringField(row, "spaceID"), readStringField(row, "spaceId")), Slug: firstNonEmpty(readStringField(row, "Slug"), readStringField(row, "slug"))}
}
func threeWayViewMembership(obj unstructured.Unstructured, units []threeWayViewUnit) (bool, string) {
	live := summarizeCompareLiveObject(&obj)
	if live.UnitID != "" {
		for _, unit := range units {
			if unit.ID == live.UnitID {
				if unit.SpaceID != "" && live.SpaceID != "" && unit.SpaceID != live.SpaceID {
					return false, "unit_space_conflict"
				}
				return true, ""
			}
		}
	}
	if live.SpaceID != "" && live.UnitSlug != "" {
		for _, unit := range units {
			if unit.SpaceID == live.SpaceID && unit.Slug == live.UnitSlug {
				if live.UnitID != "" && unit.ID != "" && live.UnitID != unit.ID {
					return false, "unit_identity_conflict"
				}
				return true, ""
			}
		}
	}
	// Exact but different IDs/spaces are exclusions; absent identity remains
	// unknown and must never be certified as absence from the View.
	for _, unit := range units {
		if unit.Slug == live.UnitSlug && (unit.ID == "" && unit.SpaceID == "" || live.UnitID == "" && live.SpaceID == "") {
			return false, "unit_identity_missing"
		}
	}
	hasRowsWithoutID := false
	for _, unit := range units {
		if unit.ID == "" {
			hasRowsWithoutID = true
		}
	}
	if (live.UnitID == "" || hasRowsWithoutID) && (live.UnitSlug == "" || live.SpaceID == "") {
		return false, "unit_link_missing"
	}
	return false, ""
}
