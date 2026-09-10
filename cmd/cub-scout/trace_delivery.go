// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

type traceConfigHubDeliveryFlags struct {
	Enabled    bool
	Namespace  string
	Space      string
	Since      string
	StaleAfter string
}

func enrichTraceConfigHubFromLive(ctx context.Context, result *agent.TraceResult, kind, name, namespace string) dynamic.Interface {
	if result == nil {
		return nil
	}
	cfg, err := buildConfig()
	if err != nil {
		return nil
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil
	}
	obj, err := fetchTraceResource(ctx, dynClient, kind, name, namespace)
	if err != nil || obj == nil {
		return dynClient
	}
	enrichTraceConfigHubFromObject(result, obj)
	return dynClient
}

func enrichTraceConfigHubFromObject(result *agent.TraceResult, obj *unstructured.Unstructured) {
	if result == nil || obj == nil {
		return
	}
	result.EnrichWithConfigHub(obj.GetLabels(), obj.GetAnnotations())
}

func attachTraceConfigHubDeliveryEvidence(ctx context.Context, result *agent.TraceResult, dynClient dynamic.Interface, flags traceConfigHubDeliveryFlags) {
	if result == nil || !flags.Enabled {
		return
	}

	correlation := buildTraceDeliveryCorrelation(result)
	opts, preflightOmissions := traceGitOpsDeliveryOptions(flags, correlation)
	if opts.Now.IsZero() {
		opts.Now = gitopsNowFn().UTC()
	}

	var raw *GitOpsDeliveryEvidence
	if strings.TrimSpace(opts.Space) == "" {
		raw = &GitOpsDeliveryEvidence{
			ObservedAt: opts.Now,
			Scope: GitOpsDeliveryEvidenceScope{
				Namespace:  opts.Namespace,
				Space:      opts.Space,
				Since:      opts.Since,
				StaleAfter: opts.StaleAfter.String(),
				MaxItems:   opts.MaxItems,
			},
			Notes: []string{
				"ConfigHub delivery evidence was requested, but release/unit-event/live-status reads require an explicit object or flag space.",
			},
		}
		consumers, omissions := collectGitOpsEventConsumerEvidence(ctx, dynClient, opts.Namespace)
		raw.EventConsumers = consumers
		raw.Omissions = append(raw.Omissions, omissions...)
	} else {
		raw = collectGitOpsDeliveryEvidence(ctx, dynClient, opts)
	}
	result.DeliveryEvidence = correlateTraceDeliveryEvidence(result, raw, correlation, preflightOmissions)
}

func traceGitOpsDeliveryOptions(flags traceConfigHubDeliveryFlags, correlation agent.TraceDeliveryCorrelation) (gitOpsDeliveryEvidenceOptions, []agent.TraceDeliveryOmission) {
	now := gitopsNowFn().UTC()
	opts := gitOpsDeliveryEvidenceOptions{
		Namespace: strings.TrimSpace(flags.Namespace),
		Space:     strings.TrimSpace(flags.Space),
		Since:     strings.TrimSpace(flags.Since),
		Now:       now,
		MaxItems:  defaultGitOpsDeliveryMaxItems,
	}
	if opts.Since == "" {
		opts.Since = "24h"
	}
	window, err := parseHistorySince(opts.Since)
	if err != nil {
		opts.Window = 24 * time.Hour
	} else {
		opts.Window = window
	}

	staleAfterRaw := strings.TrimSpace(flags.StaleAfter)
	if staleAfterRaw == "" {
		staleAfterRaw = "15m"
	}
	staleAfter, err := parseHistorySince(staleAfterRaw)
	if err != nil {
		opts.StaleAfter = 15 * time.Minute
	} else {
		opts.StaleAfter = staleAfter
	}

	if opts.Space == "" {
		opts.Space = strings.TrimSpace(correlation.Space)
	}

	var omissions []agent.TraceDeliveryOmission
	if strings.TrimSpace(opts.Space) == "" {
		omissions = append(omissions, agent.TraceDeliveryOmission{
			Layer:  "confighub.scope",
			Reason: "resource has no explicit ConfigHub space label, ConfigHub OCI space, or --confighub-space flag",
			Impact: "release, unit-event, and live-status reads are skipped to avoid broad connected queries",
		})
	}
	return opts, omissions
}

func correlateTraceDeliveryEvidence(
	result *agent.TraceResult,
	raw *GitOpsDeliveryEvidence,
	correlation agent.TraceDeliveryCorrelation,
	preflightOmissions []agent.TraceDeliveryOmission,
) *agent.TraceDeliveryEvidence {
	if raw == nil {
		return nil
	}
	out := &agent.TraceDeliveryEvidence{
		Source:      "confighub",
		ObservedAt:  raw.ObservedAt,
		Correlation: correlation,
		Scope: agent.TraceDeliveryEvidenceScope{
			Namespace:  raw.Scope.Namespace,
			Space:      raw.Scope.Space,
			Since:      raw.Scope.Since,
			StaleAfter: raw.Scope.StaleAfter,
			MaxItems:   raw.Scope.MaxItems,
		},
		Notes: append([]string(nil), raw.Notes...),
	}
	out.Omissions = append(out.Omissions, preflightOmissions...)
	out.Omissions = append(out.Omissions, convertTraceDeliveryOmissions(raw.Omissions)...)
	out.EventConsumers = convertTraceEventConsumers(raw.EventConsumers)

	if raw.ConfigHub != nil {
		if status, ok, omission := matchTraceLiveStatus(correlation, raw.ConfigHub.LiveStatuses); ok {
			out.LiveStatus = status
		} else if omission.Layer != "" {
			out.Omissions = append(out.Omissions, omission)
		}

		releases, omission := matchTraceReleases(correlation, raw.ConfigHub.Releases)
		out.Releases = releases
		if omission.Layer != "" {
			out.Omissions = append(out.Omissions, omission)
		}

		events, omission := matchTraceUnitEvents(correlation, raw.ConfigHub.UnitEvents)
		out.UnitEvents = events
		if omission.Layer != "" {
			out.Omissions = append(out.Omissions, omission)
		}
	}

	if result != nil && result.ConfigHub == nil && len(correlation.MatchedBy) == 0 {
		out.Omissions = append(out.Omissions, agent.TraceDeliveryOmission{
			Layer:  "confighub.identity",
			Reason: "live resource has no ConfigHub unit labels/annotations and trace chain has no ConfigHub OCI source",
			Impact: "delivery evidence cannot be treated as object-level proof",
		})
	}

	return out
}

func buildTraceDeliveryCorrelation(result *agent.TraceResult) agent.TraceDeliveryCorrelation {
	var correlation agent.TraceDeliveryCorrelation
	if result == nil {
		return correlation
	}
	matched := map[string]bool{}
	addMatch := func(value string) {
		if value == "" || matched[value] {
			return
		}
		correlation.MatchedBy = append(correlation.MatchedBy, value)
		matched[value] = true
	}

	if result.ConfigHub != nil {
		correlation.UnitSlug = strings.TrimSpace(result.ConfigHub.UnitSlug)
		correlation.UnitID = strings.TrimSpace(result.ConfigHub.UnitID)
		correlation.Space = strings.TrimSpace(result.ConfigHub.SpaceName)
		correlation.SpaceID = strings.TrimSpace(result.ConfigHub.SpaceID)
		correlation.TargetID = strings.TrimSpace(result.ConfigHub.TargetID)
		if correlation.UnitSlug != "" {
			addMatch("confighub.unitSlug")
		}
		if correlation.UnitID != "" {
			addMatch("confighub.unitId")
		}
		if correlation.Space != "" {
			addMatch("confighub.spaceName")
		}
		if correlation.SpaceID != "" {
			addMatch("confighub.spaceId")
		}
		if correlation.TargetID != "" {
			addMatch("confighub.targetId")
		}
	}

	for _, link := range result.Chain {
		if correlation.Application == "" && strings.EqualFold(strings.TrimSpace(link.Kind), "Application") {
			correlation.Application = strings.TrimSpace(link.Name)
			if correlation.Application != "" {
				addMatch("chain.application")
			}
		}
		if link.OCISource != nil && link.OCISource.IsConfigHub {
			if correlation.Space == "" {
				correlation.Space = strings.TrimSpace(link.OCISource.Space)
				if correlation.Space != "" {
					addMatch("chain.configHubOCI.space")
				}
			}
			if correlation.Target == "" {
				correlation.Target = strings.TrimSpace(link.OCISource.Target)
				if correlation.Target != "" {
					addMatch("chain.configHubOCI.target")
				}
			}
		}
		if link.RenderedFrom != "" {
			space, target := parseRenderedFromConfigHub(link.RenderedFrom)
			if correlation.Space == "" && space != "" {
				correlation.Space = space
				addMatch("chain.renderedFrom.space")
			}
			if correlation.Target == "" && target != "" {
				correlation.Target = target
				addMatch("chain.renderedFrom.target")
			}
		}
	}

	return correlation
}

func parseRenderedFromConfigHub(raw string) (space, target string) {
	raw = strings.TrimSpace(raw)
	const prefix = "confighub:space/"
	if !strings.HasPrefix(raw, prefix) {
		return "", ""
	}
	rest := strings.TrimPrefix(raw, prefix)
	parts := strings.Split(rest, "/target/")
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func matchTraceLiveStatus(correlation agent.TraceDeliveryCorrelation, statuses []ConfigHubLiveStatusEvidence) (*agent.TraceDeliveryLiveStatus, bool, agent.TraceDeliveryOmission) {
	for _, status := range statuses {
		spaceMatch, spaceBy := traceSpaceMatches(correlation, status.Space, status.SpaceID)
		if !spaceMatch {
			continue
		}
		app := strings.TrimSpace(status.App)
		matchedBy := []string{spaceBy}
		switch {
		case correlation.Application != "" && strings.EqualFold(app, correlation.Application):
			matchedBy = append(matchedBy, "liveStatus.app==chain.application")
		case correlation.UnitSlug != "" && strings.EqualFold(app, correlation.UnitSlug):
			matchedBy = append(matchedBy, "liveStatus.app==confighub.unitSlug")
		default:
			continue
		}
		return &agent.TraceDeliveryLiveStatus{
			Space:                    status.Space,
			SpaceID:                  status.SpaceID,
			Source:                   status.Source,
			App:                      status.App,
			SyncStatus:               status.SyncStatus,
			HealthStatus:             status.HealthStatus,
			OperationPhase:           status.OperationPhase,
			Revision:                 status.Revision,
			Message:                  status.Message,
			ObservedAt:               status.ObservedAt,
			Freshness:                status.Freshness,
			FreshnessSeconds:         status.FreshnessSeconds,
			DeliveryVerdict:          status.DeliveryVerdict,
			ApplicationHealthVerdict: status.ApplicationHealthVerdict,
			MatchedBy:                matchedBy,
		}, true, agent.TraceDeliveryOmission{}
	}
	return nil, false, agent.TraceDeliveryOmission{
		Layer:  "confighub.liveStatus",
		Reason: "no live-status writeback matched the traced resource by exact space plus application or unit slug",
		Impact: "delivery/application health writeback is omitted for this object",
	}
}

func matchTraceReleases(correlation agent.TraceDeliveryCorrelation, releases []ConfigHubReleaseEvidence) ([]agent.TraceDeliveryRelease, agent.TraceDeliveryOmission) {
	out := []agent.TraceDeliveryRelease{}
	for _, release := range releases {
		spaceMatch, spaceBy := traceSpaceMatches(correlation, release.Space, release.SpaceID)
		if !spaceMatch {
			continue
		}
		targetMatch, targetBy := traceTargetMatches(correlation, release.Target, release.TargetID)
		if !targetMatch {
			continue
		}
		out = append(out, agent.TraceDeliveryRelease{
			Slug:           release.Slug,
			ReleaseID:      release.ReleaseID,
			Space:          release.Space,
			SpaceID:        release.SpaceID,
			Target:         release.Target,
			TargetID:       release.TargetID,
			Digest:         release.Digest,
			BundleBaseName: release.BundleBaseName,
			RevisionNum:    release.RevisionNum,
			CreatedAt:      release.CreatedAt,
			MatchedBy:      []string{spaceBy, targetBy},
		})
	}
	if len(out) > 0 {
		return out, agent.TraceDeliveryOmission{}
	}
	return nil, agent.TraceDeliveryOmission{
		Layer:  "confighub.releases",
		Reason: "no release row matched the traced resource by exact space plus target",
		Impact: "recent release publishing evidence is omitted for this object",
	}
}

func matchTraceUnitEvents(correlation agent.TraceDeliveryCorrelation, events []ConfigHubUnitEventEvidence) ([]agent.TraceDeliveryUnitEvent, agent.TraceDeliveryOmission) {
	out := []agent.TraceDeliveryUnitEvent{}
	for _, event := range events {
		var matchedBy []string
		switch {
		case correlation.UnitID != "" && strings.EqualFold(event.UnitID, correlation.UnitID):
			matchedBy = append(matchedBy, "unitEvent.unitId==confighub.unitId")
		case correlation.UnitSlug != "" && strings.EqualFold(event.Unit, correlation.UnitSlug):
			spaceMatch, spaceBy := traceSpaceMatches(correlation, event.Space, event.SpaceID)
			if !spaceMatch {
				continue
			}
			matchedBy = append(matchedBy, "unitEvent.unit==confighub.unitSlug", spaceBy)
		default:
			continue
		}
		out = append(out, agent.TraceDeliveryUnitEvent{
			EventID:      event.EventID,
			Action:       event.Action,
			Result:       event.Result,
			Status:       event.Status,
			Message:      event.Message,
			Unit:         event.Unit,
			UnitID:       event.UnitID,
			Space:        event.Space,
			SpaceID:      event.SpaceID,
			Target:       event.Target,
			TargetID:     event.TargetID,
			CreatedAt:    event.CreatedAt,
			TerminatedAt: event.TerminatedAt,
			MatchedBy:    matchedBy,
		})
	}
	if len(out) > 0 {
		return out, agent.TraceDeliveryOmission{}
	}
	return nil, agent.TraceDeliveryOmission{
		Layer:  "confighub.unitEvents",
		Reason: "no unit-event row matched the traced resource by exact unit ID or unit slug plus space",
		Impact: "recent unit-level event evidence is omitted for this object",
	}
}

func traceSpaceMatches(correlation agent.TraceDeliveryCorrelation, space, spaceID string) (bool, string) {
	space = strings.TrimSpace(space)
	spaceID = strings.TrimSpace(spaceID)
	if correlation.SpaceID != "" && spaceID != "" && strings.EqualFold(correlation.SpaceID, spaceID) {
		return true, "spaceId"
	}
	if correlation.Space != "" && space != "" && strings.EqualFold(correlation.Space, space) {
		return true, "space"
	}
	return false, ""
}

func traceTargetMatches(correlation agent.TraceDeliveryCorrelation, target, targetID string) (bool, string) {
	target = strings.TrimSpace(target)
	targetID = strings.TrimSpace(targetID)
	if correlation.TargetID != "" && targetID != "" && strings.EqualFold(correlation.TargetID, targetID) {
		return true, "targetId"
	}
	if correlation.Target != "" && target != "" && strings.EqualFold(correlation.Target, target) {
		return true, "target"
	}
	return false, ""
}

func convertTraceEventConsumers(consumers []GitOpsEventConsumerEvidence) []agent.TraceDeliveryEventConsumer {
	out := make([]agent.TraceDeliveryEventConsumer, 0, len(consumers))
	for _, consumer := range consumers {
		out = append(out, agent.TraceDeliveryEventConsumer{
			Kind:              consumer.Kind,
			Name:              consumer.Name,
			Namespace:         consumer.Namespace,
			Ready:             consumer.Ready,
			Replicas:          consumer.Replicas,
			ReadyReplicas:     consumer.ReadyReplicas,
			AvailableReplicas: consumer.AvailableReplicas,
			EvidenceLabel:     consumer.EvidenceLabel,
		})
	}
	return out
}

func convertTraceDeliveryOmissions(omissions []GitOpsDeliveryEvidenceOmission) []agent.TraceDeliveryOmission {
	out := make([]agent.TraceDeliveryOmission, 0, len(omissions))
	for _, omission := range omissions {
		out = append(out, agent.TraceDeliveryOmission{
			Layer:   omission.Layer,
			Reason:  omission.Reason,
			Impact:  omission.Impact,
			Command: omission.Command,
		})
	}
	return out
}

func formatTraceDeliveryEvidenceLine(evidence *agent.TraceDeliveryEvidence) string {
	if evidence == nil {
		return ""
	}
	parts := []string{"ConfigHub"}
	if evidence.LiveStatus != nil {
		parts = append(parts,
			fmt.Sprintf("delivery=%s", evidence.LiveStatus.DeliveryVerdict),
			fmt.Sprintf("sync=%s", firstNonEmpty(evidence.LiveStatus.SyncStatus, "-")),
			fmt.Sprintf("appHealth=%s", evidence.LiveStatus.ApplicationHealthVerdict),
			fmt.Sprintf("health=%s", firstNonEmpty(evidence.LiveStatus.HealthStatus, "-")),
			fmt.Sprintf("freshness=%s", firstNonEmpty(evidence.LiveStatus.Freshness, "-")),
		)
	}
	if len(evidence.Releases) > 0 {
		parts = append(parts, fmt.Sprintf("releases=%d", len(evidence.Releases)))
	}
	if len(evidence.UnitEvents) > 0 {
		parts = append(parts, fmt.Sprintf("unitEvents=%d", len(evidence.UnitEvents)))
	}
	if len(parts) == 1 {
		parts = append(parts, "no exact object-level match")
	}
	return strings.Join(parts, " ")
}

func renderTraceDeliveryEvidenceHuman(evidence *agent.TraceDeliveryEvidence) {
	if evidence == nil {
		return
	}
	fmt.Printf("\n")
	fmt.Printf("%s%sConfigHub delivery evidence:%s\n", colorBold, colorWhite, colorReset)
	corr := evidence.Correlation
	identity := []string{}
	if corr.UnitSlug != "" {
		identity = append(identity, "unit="+corr.UnitSlug)
	}
	if corr.Space != "" {
		identity = append(identity, "space="+corr.Space)
	}
	if corr.Target != "" {
		identity = append(identity, "target="+corr.Target)
	}
	if corr.Application != "" {
		identity = append(identity, "app="+corr.Application)
	}
	if len(identity) > 0 {
		fmt.Printf("  %sCorrelation:%s %s\n", colorDim, colorReset, strings.Join(identity, " "))
	}
	if evidence.LiveStatus != nil {
		fmt.Printf("  %sLive status:%s app=%s sync=%s health=%s op=%s delivery=%s app-health=%s freshness=%s\n",
			colorDim,
			colorReset,
			firstNonEmpty(evidence.LiveStatus.App, "-"),
			firstNonEmpty(evidence.LiveStatus.SyncStatus, "-"),
			firstNonEmpty(evidence.LiveStatus.HealthStatus, "-"),
			firstNonEmpty(evidence.LiveStatus.OperationPhase, "-"),
			evidence.LiveStatus.DeliveryVerdict,
			evidence.LiveStatus.ApplicationHealthVerdict,
			firstNonEmpty(evidence.LiveStatus.Freshness, "-"),
		)
	}
	if len(evidence.Releases) > 0 {
		fmt.Printf("  %sRecent releases:%s\n", colorDim, colorReset)
		for _, release := range evidence.Releases {
			fmt.Printf("    - %s target=%s digest=%s at=%s\n",
				firstNonEmpty(release.Slug, release.ReleaseID, "-"),
				firstNonEmpty(release.Target, release.TargetID, "-"),
				truncate(firstNonEmpty(release.Digest, "-"), 18),
				firstNonEmpty(release.CreatedAt, "-"),
			)
		}
	}
	if len(evidence.UnitEvents) > 0 {
		fmt.Printf("  %sRecent unit events:%s\n", colorDim, colorReset)
		for _, event := range evidence.UnitEvents {
			fmt.Printf("    - %s unit=%s result=%s at=%s\n",
				firstNonEmpty(event.Action, event.Status, "-"),
				firstNonEmpty(event.Unit, event.UnitID, "-"),
				firstNonEmpty(event.Result, event.Status, "-"),
				firstNonEmpty(event.CreatedAt, "-"),
			)
		}
	}
	if len(evidence.EventConsumers) > 0 {
		fmt.Printf("  %sEvent consumers:%s\n", colorDim, colorReset)
		for _, consumer := range evidence.EventConsumers {
			state := "not ready"
			if consumer.Ready {
				state = "ready"
			}
			fmt.Printf("    - %s/%s %s (%d/%d ready)\n",
				firstNonEmpty(consumer.Namespace, "-"),
				consumer.Name,
				state,
				consumer.ReadyReplicas,
				consumer.Replicas,
			)
		}
	}
	if len(evidence.Omissions) > 0 {
		fmt.Printf("  %sOmissions:%s\n", colorDim, colorReset)
		for _, omission := range evidence.Omissions {
			fmt.Printf("    - %s: %s\n", omission.Layer, omission.Reason)
		}
	}
}

func renderTraceDeliveryEvidenceMarkdown(evidence *agent.TraceDeliveryEvidence) {
	if evidence == nil {
		return
	}
	fmt.Printf("\nConfigHub delivery evidence:\n")
	if line := formatTraceDeliveryEvidenceLine(evidence); line != "" {
		fmt.Printf("  %s\n", line)
	}
	if len(evidence.Omissions) > 0 {
		fmt.Printf("  Omissions:\n")
		for _, omission := range evidence.Omissions {
			fmt.Printf("    - %s: %s\n", omission.Layer, omission.Reason)
		}
	}
}
