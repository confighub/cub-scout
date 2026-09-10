// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	configHubLiveStatusAnnotation = "confighub.com/live-status"
	defaultGitOpsDeliveryMaxItems = 10
)

type gitOpsDeliveryEvidenceOptions struct {
	Namespace  string
	Space      string
	Since      string
	Window     time.Duration
	StaleAfter time.Duration
	Now        time.Time
	MaxItems   int
}

// GitOpsDeliveryEvidence is an opt-in, bounded connected evidence envelope.
// It is intentionally not a delivery verdict for the whole deployment; it only
// reports what cub-scout can observe read-only from the cluster and ConfigHub.
type GitOpsDeliveryEvidence struct {
	ObservedAt     time.Time                        `json:"observedAt"`
	Scope          GitOpsDeliveryEvidenceScope      `json:"scope"`
	ConfigHub      *ConfigHubDeliveryEvidence       `json:"configHub,omitempty"`
	EventConsumers []GitOpsEventConsumerEvidence    `json:"eventConsumers,omitempty"`
	Omissions      []GitOpsDeliveryEvidenceOmission `json:"omissions,omitempty"`
	Notes          []string                         `json:"notes,omitempty"`
}

type GitOpsDeliveryEvidenceScope struct {
	Namespace  string `json:"namespace,omitempty"`
	Space      string `json:"space,omitempty"`
	Since      string `json:"since"`
	StaleAfter string `json:"staleAfter"`
	MaxItems   int    `json:"maxItems"`
}

type ConfigHubDeliveryEvidence struct {
	LiveStatuses []ConfigHubLiveStatusEvidence `json:"liveStatuses,omitempty"`
	Releases     []ConfigHubReleaseEvidence    `json:"releases,omitempty"`
	UnitEvents   []ConfigHubUnitEventEvidence  `json:"unitEvents,omitempty"`
}

type ConfigHubLiveStatusEvidence struct {
	Space                    string               `json:"space,omitempty"`
	SpaceID                  string               `json:"spaceId,omitempty"`
	Source                   string               `json:"source,omitempty"`
	App                      string               `json:"app,omitempty"`
	SyncStatus               string               `json:"syncStatus,omitempty"`
	HealthStatus             string               `json:"healthStatus,omitempty"`
	OperationPhase           string               `json:"operationPhase,omitempty"`
	Revision                 string               `json:"revision,omitempty"`
	Message                  string               `json:"message,omitempty"`
	ObservedAt               string               `json:"observedAt,omitempty"`
	Freshness                string               `json:"freshness"`
	FreshnessSeconds         int64                `json:"freshnessSeconds,omitempty"`
	DeliveryVerdict          agent.ReceiptVerdict `json:"deliveryVerdict"`
	ApplicationHealthVerdict agent.ReceiptVerdict `json:"applicationHealthVerdict"`
}

type ConfigHubReleaseEvidence struct {
	Slug           string `json:"slug,omitempty"`
	ReleaseID      string `json:"releaseId,omitempty"`
	Space          string `json:"space,omitempty"`
	SpaceID        string `json:"spaceId,omitempty"`
	Target         string `json:"target,omitempty"`
	TargetID       string `json:"targetId,omitempty"`
	Digest         string `json:"digest,omitempty"`
	BundleBaseName string `json:"bundleBaseName,omitempty"`
	RevisionNum    int    `json:"revisionNum,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
}

type ConfigHubUnitEventEvidence struct {
	EventID      string `json:"eventId,omitempty"`
	Action       string `json:"action,omitempty"`
	Result       string `json:"result,omitempty"`
	Status       string `json:"status,omitempty"`
	Message      string `json:"message,omitempty"`
	Unit         string `json:"unit,omitempty"`
	UnitID       string `json:"unitId,omitempty"`
	Space        string `json:"space,omitempty"`
	SpaceID      string `json:"spaceId,omitempty"`
	Target       string `json:"target,omitempty"`
	TargetID     string `json:"targetId,omitempty"`
	CreatedAt    string `json:"createdAt,omitempty"`
	TerminatedAt string `json:"terminatedAt,omitempty"`
}

type GitOpsEventConsumerEvidence struct {
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	Namespace         string `json:"namespace,omitempty"`
	Ready             bool   `json:"ready"`
	Replicas          int64  `json:"replicas,omitempty"`
	ReadyReplicas     int64  `json:"readyReplicas,omitempty"`
	AvailableReplicas int64  `json:"availableReplicas,omitempty"`
	EvidenceLabel     string `json:"evidenceLabel,omitempty"`
}

type GitOpsDeliveryEvidenceOmission struct {
	Layer   string `json:"layer"`
	Reason  string `json:"reason"`
	Impact  string `json:"impact,omitempty"`
	Command string `json:"command,omitempty"`
}

var (
	errGitOpsConfigHubDisconnected = errors.New("ConfigHub evidence requires ConfigHub connection. Run: cub auth login")
	requireGitOpsConfigHubFn       = requireGitOpsConfigHubConnected
	runGitOpsCubCommand            = runHistoryCubCommandImpl
	gitopsNowFn                    = time.Now
	gitopsDefaultSpaceFn           = detectGitOpsConfigHubSpace
)

func normalizeGitOpsStatusFormat(raw string, legacyJSON bool) (string, error) {
	format := strings.ToLower(strings.TrimSpace(raw))
	if format == "" {
		format = "ascii"
	}
	if legacyJSON {
		format = "json"
	}
	switch format {
	case "ascii", "json", "md":
		return format, nil
	default:
		return "", fmt.Errorf("invalid --format %q (valid: ascii, json, md)", raw)
	}
}

func gitOpsDeliveryEvidenceOptionsFromFlags(ctx context.Context) (gitOpsDeliveryEvidenceOptions, error) {
	now := gitopsNowFn().UTC()
	opts := gitOpsDeliveryEvidenceOptions{
		Namespace: strings.TrimSpace(gitopsNamespace),
		Space:     strings.TrimSpace(gitopsConfigHubSpace),
		Since:     strings.TrimSpace(gitopsConfigHubSince),
		Now:       now,
		MaxItems:  defaultGitOpsDeliveryMaxItems,
	}
	if opts.Since == "" {
		opts.Since = "24h"
	}
	if !gitopsWithConfigHub {
		return opts, nil
	}

	window, err := parseHistorySince(opts.Since)
	if err != nil {
		return opts, fmt.Errorf("invalid --confighub-since: %w", err)
	}
	opts.Window = window

	staleAfterRaw := strings.TrimSpace(gitopsConfigHubStaleAfter)
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

func requireGitOpsConfigHubConnected() error {
	if err := hub.NewClient().RequireConnected(); err != nil {
		return errGitOpsConfigHubDisconnected
	}
	if _, err := exec.LookPath("cub"); err != nil {
		return fmt.Errorf("ConfigHub evidence requires cub CLI for connected queries: %w", err)
	}
	return nil
}

func detectGitOpsConfigHubSpace(ctx context.Context) string {
	if space := hub.PluginSpace(); space != "" {
		return strings.TrimSpace(space)
	}
	cubCtx, _, err := getStatusCubContext()
	if err != nil || cubCtx == nil {
		return ""
	}
	return strings.TrimSpace(cubCtx.Settings.DefaultSpace)
}

func collectGitOpsDeliveryEvidence(ctx context.Context, client dynamic.Interface, opts gitOpsDeliveryEvidenceOptions) *GitOpsDeliveryEvidence {
	if opts.Now.IsZero() {
		opts.Now = gitopsNowFn().UTC()
	}
	if strings.TrimSpace(opts.Space) == "" {
		opts.Space = gitopsDefaultSpaceFn(ctx)
	}
	if strings.TrimSpace(opts.Since) == "" {
		opts.Since = "24h"
	}
	if opts.Window <= 0 {
		if parsed, err := parseHistorySince(opts.Since); err == nil {
			opts.Window = parsed
		} else {
			opts.Window = 24 * time.Hour
		}
	}
	if opts.StaleAfter <= 0 {
		opts.StaleAfter = 15 * time.Minute
	}
	evidence := &GitOpsDeliveryEvidence{
		ObservedAt: opts.Now,
		Scope: GitOpsDeliveryEvidenceScope{
			Namespace:  opts.Namespace,
			Space:      opts.Space,
			Since:      opts.Since,
			StaleAfter: opts.StaleAfter.String(),
			MaxItems:   opts.MaxItems,
		},
		ConfigHub: &ConfigHubDeliveryEvidence{},
		Notes: []string{
			"ConfigHub evidence is opt-in, read-only, bounded by space and time window, and does not consume event cursors.",
		},
	}

	if opts.MaxItems <= 0 {
		opts.MaxItems = defaultGitOpsDeliveryMaxItems
		evidence.Scope.MaxItems = opts.MaxItems
	}

	consumers, consumerOmissions := collectGitOpsEventConsumerEvidence(ctx, client, opts.Namespace)
	evidence.EventConsumers = consumers
	evidence.Omissions = append(evidence.Omissions, consumerOmissions...)

	if err := requireGitOpsConfigHubFn(); err != nil {
		evidence.Omissions = append(evidence.Omissions, GitOpsDeliveryEvidenceOmission{
			Layer:  "confighub",
			Reason: err.Error(),
			Impact: "release, unit-event, and live-status evidence are omitted",
		})
		return evidence
	}
	if strings.TrimSpace(opts.Space) == "" {
		evidence.Omissions = append(evidence.Omissions, GitOpsDeliveryEvidenceOmission{
			Layer:  "confighub.scope",
			Reason: "no ConfigHub space was supplied and no current cub default space could be resolved",
			Impact: "release and unit-event queries are skipped to avoid unbounded ConfigHub reads",
		})
		return evidence
	}

	rawSpaces, err := runGitOpsCubCommand(ctx, gitOpsConfigHubSpaceListArgs(opts.Space))
	if err != nil {
		evidence.Omissions = append(evidence.Omissions, GitOpsDeliveryEvidenceOmission{
			Layer:   "confighub.liveStatus",
			Reason:  err.Error(),
			Impact:  "live-status writeback evidence is unavailable",
			Command: "cub " + strings.Join(gitOpsConfigHubSpaceListArgs(opts.Space), " "),
		})
	} else {
		statuses, omissions := buildConfigHubLiveStatusEvidence(rawSpaces, opts.Now, opts.StaleAfter)
		evidence.ConfigHub.LiveStatuses = statuses
		evidence.Omissions = append(evidence.Omissions, omissions...)
	}

	cutoff := opts.Now.Add(-opts.Window).UTC().Format(time.RFC3339)
	releaseArgs := gitOpsConfigHubReleaseListArgs(opts.Space, cutoff)
	rawReleases, err := runGitOpsCubCommand(ctx, releaseArgs)
	if err != nil {
		evidence.Omissions = append(evidence.Omissions, GitOpsDeliveryEvidenceOmission{
			Layer:   "confighub.releases",
			Reason:  err.Error(),
			Impact:  "recent release publishing evidence is unavailable",
			Command: "cub " + strings.Join(releaseArgs, " "),
		})
	} else {
		releases, omissions := buildConfigHubReleaseEvidence(rawReleases, opts.MaxItems)
		evidence.ConfigHub.Releases = releases
		evidence.Omissions = append(evidence.Omissions, omissions...)
	}

	eventArgs := gitOpsConfigHubUnitEventListArgs(opts.Space, cutoff)
	rawEvents, err := runGitOpsCubCommand(ctx, eventArgs)
	if err != nil {
		evidence.Omissions = append(evidence.Omissions, GitOpsDeliveryEvidenceOmission{
			Layer:   "confighub.unitEvents",
			Reason:  err.Error(),
			Impact:  "recent unit event evidence is unavailable",
			Command: "cub " + strings.Join(eventArgs, " "),
		})
	} else {
		events, omissions := buildConfigHubUnitEventEvidence(rawEvents, opts.MaxItems)
		evidence.ConfigHub.UnitEvents = events
		evidence.Omissions = append(evidence.Omissions, omissions...)
	}

	return evidence
}

func gitOpsConfigHubSpaceListArgs(space string) []string {
	args := []string{"space", "list", "-o", "json", "--select", "Slug,SpaceID,Annotations,Labels"}
	if strings.TrimSpace(space) != "*" {
		args = append(args, "--where", fmt.Sprintf("Slug = '%s'", configHubFilterQuote(space)))
	}
	return args
}

func gitOpsConfigHubReleaseListArgs(space, cutoff string) []string {
	args := []string{
		"release", "list",
		"--space", space,
		"-o", "json",
		"--where", fmt.Sprintf("CreatedAt > '%s'", configHubFilterQuote(cutoff)),
		"--select", "Slug,ReleaseID,CreatedAt,Digest,BundleBaseName,RevisionNum,Space,Target",
	}
	return args
}

func gitOpsConfigHubUnitEventListArgs(space, cutoff string) []string {
	args := []string{
		"unit-event", "list",
		"--space", space,
		"-o", "json",
		"--where", fmt.Sprintf("CreatedAt > '%s'", configHubFilterQuote(cutoff)),
		"--select", "UnitEventID,Action,Result,Status,Message,CreatedAt,TerminatedAt,Unit,Space,Target",
	}
	return args
}

func configHubFilterQuote(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "'", "''")
}

func buildConfigHubLiveStatusEvidence(raw string, now time.Time, staleAfter time.Duration) ([]ConfigHubLiveStatusEvidence, []GitOpsDeliveryEvidenceOmission) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, []GitOpsDeliveryEvidenceOmission{{
			Layer:  "confighub.liveStatus",
			Reason: "space list returned no rows",
			Impact: "delivery/application status writeback could not be observed",
		}}
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, []GitOpsDeliveryEvidenceOmission{{
			Layer:  "confighub.liveStatus",
			Reason: "space list JSON could not be parsed: " + err.Error(),
			Impact: "delivery/application status writeback could not be observed",
		}}
	}

	items := cubExtractItems(payload)
	statuses := make([]ConfigHubLiveStatusEvidence, 0, len(items))
	omissions := []GitOpsDeliveryEvidenceOmission{}
	for _, item := range items {
		status, ok, omission := configHubLiveStatusFromSpaceItem(item, now, staleAfter)
		if omission.Layer != "" {
			omissions = append(omissions, omission)
		}
		if ok {
			statuses = append(statuses, status)
		}
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Space < statuses[j].Space
	})
	if len(statuses) == 0 && len(omissions) == 0 {
		omissions = append(omissions, GitOpsDeliveryEvidenceOmission{
			Layer:  "confighub.liveStatus",
			Reason: fmt.Sprintf("no %s annotation found", configHubLiveStatusAnnotation),
			Impact: "delivery/application status writeback has not been observed for the selected space",
		})
	}
	return statuses, omissions
}

func configHubLiveStatusFromSpaceItem(item map[string]interface{}, now time.Time, staleAfter time.Duration) (ConfigHubLiveStatusEvidence, bool, GitOpsDeliveryEvidenceOmission) {
	spaceObj := mcpNestedMap(item, "Space", "space")
	if spaceObj == nil {
		spaceObj = item
	}
	annotations := mapStringFromJSONValue(mcpFirstRaw(spaceObj, "Annotations", "annotations"))
	if len(annotations) == 0 {
		annotations = mapStringFromJSONValue(mcpFirstRaw(item, "Annotations", "annotations"))
	}

	annotation := strings.TrimSpace(annotations[configHubLiveStatusAnnotation])
	space := mcpFirstString(spaceObj, "Slug", "slug", "Name", "name")
	spaceID := mcpFirstString(spaceObj, "SpaceID", "spaceId", "ID", "id")
	if annotation == "" {
		return ConfigHubLiveStatusEvidence{}, false, GitOpsDeliveryEvidenceOmission{
			Layer:  "confighub.liveStatus",
			Reason: fmt.Sprintf("space %q has no %s annotation", firstNonEmpty(space, spaceID, "unknown"), configHubLiveStatusAnnotation),
			Impact: "no event-consumer status writeback is available for this space",
		}
	}

	var statusMap map[string]interface{}
	if err := json.Unmarshal([]byte(annotation), &statusMap); err != nil {
		return ConfigHubLiveStatusEvidence{}, false, GitOpsDeliveryEvidenceOmission{
			Layer:  "confighub.liveStatus",
			Reason: fmt.Sprintf("space %q has malformed %s annotation: %v", firstNonEmpty(space, spaceID, "unknown"), configHubLiveStatusAnnotation, err),
			Impact: "delivery/application status writeback could not be interpreted",
		}
	}

	status := ConfigHubLiveStatusEvidence{
		Space:          space,
		SpaceID:        spaceID,
		Source:         mcpFirstString(statusMap, "source", "Source"),
		App:            mcpFirstString(statusMap, "app", "App", "application", "Application"),
		SyncStatus:     mcpFirstString(statusMap, "syncStatus", "SyncStatus", "sync_status"),
		HealthStatus:   mcpFirstString(statusMap, "healthStatus", "HealthStatus", "health_status"),
		OperationPhase: mcpFirstString(statusMap, "operationPhase", "OperationPhase", "operation_phase"),
		Revision:       mcpFirstString(statusMap, "revision", "Revision"),
		Message:        mcpFirstString(statusMap, "message", "Message"),
		ObservedAt:     mcpFirstString(statusMap, "observedAt", "ObservedAt", "observed_at"),
		Freshness:      "unknown",
	}

	if observedAt, ok := parseConfigHubObservedAt(status.ObservedAt); ok {
		age := now.Sub(observedAt.UTC())
		if age < 0 {
			age = 0
		}
		status.FreshnessSeconds = int64(age.Seconds())
		status.Freshness = "fresh"
		if staleAfter > 0 && age > staleAfter {
			status.Freshness = "stale"
		}
	}
	status.DeliveryVerdict = configHubDeliveryVerdict(status)
	status.ApplicationHealthVerdict = configHubApplicationHealthVerdict(status)
	return status, true, GitOpsDeliveryEvidenceOmission{}
}

func parseConfigHubObservedAt(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02 15:04:05", raw); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func configHubDeliveryVerdict(status ConfigHubLiveStatusEvidence) agent.ReceiptVerdict {
	syncStatus := strings.ToLower(strings.TrimSpace(status.SyncStatus))
	operationPhase := strings.ToLower(strings.TrimSpace(status.OperationPhase))

	var verdict agent.ReceiptVerdict
	switch {
	case syncStatus == "" && operationPhase == "":
		verdict = agent.VerdictINCONCLUSIVE
	case operationPhase == "failed" || operationPhase == "error":
		verdict = agent.VerdictBLOCK
	case syncStatus == "synced" && (operationPhase == "" || operationPhase == "succeeded"):
		verdict = agent.VerdictPASS
	case syncStatus == "outofsync":
		verdict = agent.VerdictWATCH
	case syncStatus == "unknown" || syncStatus == "progressing" || operationPhase == "running" || operationPhase == "terminating":
		verdict = agent.VerdictWATCH
	default:
		verdict = agent.VerdictWATCH
	}
	if verdict == agent.VerdictPASS && status.Freshness == "stale" {
		return agent.VerdictWATCH
	}
	return verdict
}

func configHubApplicationHealthVerdict(status ConfigHubLiveStatusEvidence) agent.ReceiptVerdict {
	health := strings.ToLower(strings.TrimSpace(status.HealthStatus))
	var verdict agent.ReceiptVerdict
	switch health {
	case "":
		verdict = agent.VerdictINCONCLUSIVE
	case "healthy":
		verdict = agent.VerdictPASS
	case "degraded", "missing":
		verdict = agent.VerdictBLOCK
	case "progressing", "suspended", "unknown":
		verdict = agent.VerdictWATCH
	default:
		verdict = agent.VerdictWATCH
	}
	if verdict == agent.VerdictPASS && status.Freshness == "stale" {
		return agent.VerdictWATCH
	}
	return verdict
}

func buildConfigHubReleaseEvidence(raw string, maxItems int) ([]ConfigHubReleaseEvidence, []GitOpsDeliveryEvidenceOmission) {
	payload, items, omission := parseGitOpsEvidenceItems(raw, "confighub.releases", "release list")
	if omission.Layer != "" {
		return nil, []GitOpsDeliveryEvidenceOmission{omission}
	}
	_ = payload

	releases := make([]ConfigHubReleaseEvidence, 0, len(items))
	for _, item := range items {
		releaseObj := mcpNestedMap(item, "Release", "release")
		if releaseObj == nil {
			releaseObj = item
		}
		spaceObj := mcpNestedMap(item, "Space", "space")
		if spaceObj == nil {
			spaceObj = mcpNestedMap(releaseObj, "Space", "space")
		}
		targetObj := mcpNestedMap(item, "Target", "target")
		if targetObj == nil {
			targetObj = mcpNestedMap(releaseObj, "Target", "target")
		}

		release := ConfigHubReleaseEvidence{
			Slug:           mcpFirstString(releaseObj, "Slug", "slug", "Name", "name"),
			ReleaseID:      mcpFirstString(releaseObj, "ReleaseID", "releaseId", "ID", "id"),
			Space:          mcpFirstString(spaceObj, "Slug", "slug", "Name", "name", "SpaceSlug", "spaceSlug"),
			SpaceID:        mcpFirstString(spaceObj, "SpaceID", "spaceId", "ID", "id"),
			Target:         mcpFirstString(targetObj, "Slug", "slug", "Name", "name", "TargetSlug", "targetSlug"),
			TargetID:       mcpFirstString(targetObj, "TargetID", "targetId", "ID", "id"),
			Digest:         mcpFirstString(releaseObj, "Digest", "digest", "OCIManifestDigest", "ociManifestDigest", "BundleDigest", "bundleDigest"),
			BundleBaseName: mcpFirstString(releaseObj, "BundleBaseName", "bundleBaseName", "Bundle", "bundle"),
			CreatedAt:      mcpFirstString(releaseObj, "CreatedAt", "createdAt", "Timestamp", "timestamp"),
		}
		if release.Space == "" {
			release.Space = mcpFirstString(releaseObj, "SpaceSlug", "spaceSlug")
		}
		if release.Target == "" {
			release.Target = mcpFirstString(releaseObj, "TargetSlug", "targetSlug")
		}
		if value, ok := mcpFirstInt(releaseObj, "RevisionNum", "revisionNum", "RevisionNumber", "revisionNumber"); ok {
			release.RevisionNum = value
		}
		releases = append(releases, release)
	}

	sort.Slice(releases, func(i, j int) bool {
		return evidenceTimeAfter(releases[i].CreatedAt, releases[j].CreatedAt)
	})
	omissions := trimReleaseEvidence(&releases, maxItems)
	return releases, omissions
}

func buildConfigHubUnitEventEvidence(raw string, maxItems int) ([]ConfigHubUnitEventEvidence, []GitOpsDeliveryEvidenceOmission) {
	_, items, omission := parseGitOpsEvidenceItems(raw, "confighub.unitEvents", "unit-event list")
	if omission.Layer != "" {
		return nil, []GitOpsDeliveryEvidenceOmission{omission}
	}

	events := make([]ConfigHubUnitEventEvidence, 0, len(items))
	for _, item := range items {
		eventObj := mcpNestedMap(item, "UnitEvent", "unitEvent", "event")
		if eventObj == nil {
			eventObj = item
		}
		unitObj := mcpNestedMap(item, "Unit", "unit")
		if unitObj == nil {
			unitObj = mcpNestedMap(eventObj, "Unit", "unit")
		}
		spaceObj := mcpNestedMap(item, "Space", "space")
		if spaceObj == nil {
			spaceObj = mcpNestedMap(eventObj, "Space", "space")
		}
		targetObj := mcpNestedMap(item, "Target", "target")
		if targetObj == nil {
			targetObj = mcpNestedMap(eventObj, "Target", "target")
		}

		events = append(events, ConfigHubUnitEventEvidence{
			EventID:      mcpFirstString(eventObj, "UnitEventID", "unitEventId", "EventID", "eventId", "ID", "id"),
			Action:       mcpFirstString(eventObj, "Action", "action", "Type", "type"),
			Result:       mcpFirstString(eventObj, "Result", "result"),
			Status:       mcpFirstString(eventObj, "Status", "status", "Condition", "condition"),
			Message:      mcpFirstString(eventObj, "Message", "message", "Summary", "summary"),
			Unit:         mcpFirstString(unitObj, "Slug", "slug", "Name", "name", "UnitSlug", "unitSlug"),
			UnitID:       mcpFirstString(unitObj, "UnitID", "unitId", "ID", "id"),
			Space:        mcpFirstString(spaceObj, "Slug", "slug", "Name", "name", "SpaceSlug", "spaceSlug"),
			SpaceID:      mcpFirstString(spaceObj, "SpaceID", "spaceId", "ID", "id"),
			Target:       mcpFirstString(targetObj, "Slug", "slug", "Name", "name", "TargetSlug", "targetSlug"),
			TargetID:     mcpFirstString(targetObj, "TargetID", "targetId", "ID", "id"),
			CreatedAt:    mcpFirstString(eventObj, "CreatedAt", "createdAt", "Timestamp", "timestamp"),
			TerminatedAt: mcpFirstString(eventObj, "TerminatedAt", "terminatedAt", "CompletedAt", "completedAt"),
		})
	}

	sort.Slice(events, func(i, j int) bool {
		return evidenceTimeAfter(events[i].CreatedAt, events[j].CreatedAt)
	})
	omissions := trimUnitEventEvidence(&events, maxItems)
	return events, omissions
}

func parseGitOpsEvidenceItems(raw, layer, noun string) (interface{}, []map[string]interface{}, GitOpsDeliveryEvidenceOmission) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil, GitOpsDeliveryEvidenceOmission{
			Layer:  layer,
			Reason: noun + " returned no rows",
		}
	}

	var payload interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, nil, GitOpsDeliveryEvidenceOmission{
			Layer:  layer,
			Reason: noun + " JSON could not be parsed: " + err.Error(),
		}
	}
	return payload, cubExtractItems(payload), GitOpsDeliveryEvidenceOmission{}
}

func trimReleaseEvidence(releases *[]ConfigHubReleaseEvidence, maxItems int) []GitOpsDeliveryEvidenceOmission {
	if maxItems <= 0 || len(*releases) <= maxItems {
		return nil
	}
	total := len(*releases)
	*releases = (*releases)[:maxItems]
	return []GitOpsDeliveryEvidenceOmission{{
		Layer:  "confighub.releases",
		Reason: fmt.Sprintf("trimmed %d release rows to maxItems=%d", total, maxItems),
		Impact: "older rows are omitted from output but the query remains time-window bounded",
	}}
}

func trimUnitEventEvidence(events *[]ConfigHubUnitEventEvidence, maxItems int) []GitOpsDeliveryEvidenceOmission {
	if maxItems <= 0 || len(*events) <= maxItems {
		return nil
	}
	total := len(*events)
	*events = (*events)[:maxItems]
	return []GitOpsDeliveryEvidenceOmission{{
		Layer:  "confighub.unitEvents",
		Reason: fmt.Sprintf("trimmed %d unit-event rows to maxItems=%d", total, maxItems),
		Impact: "older rows are omitted from output but the query remains time-window bounded",
	}}
}

func evidenceTimeAfter(left, right string) bool {
	lt, lok := parseConfigHubObservedAt(left)
	rt, rok := parseConfigHubObservedAt(right)
	switch {
	case lok && rok:
		return lt.After(rt)
	case lok:
		return true
	case rok:
		return false
	default:
		return left > right
	}
}

func collectGitOpsEventConsumerEvidence(ctx context.Context, client dynamic.Interface, namespace string) ([]GitOpsEventConsumerEvidence, []GitOpsDeliveryEvidenceOmission) {
	if client == nil {
		return nil, []GitOpsDeliveryEvidenceOmission{{
			Layer:  "eventConsumer",
			Reason: "no Kubernetes client available",
			Impact: "in-cluster event consumer health is not observed",
		}}
	}

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	resource := client.Resource(gvr)

	items, omissions, err := listGitOpsEventConsumerDeployments(ctx, resource, strings.TrimSpace(namespace))
	if err != nil {
		return nil, []GitOpsDeliveryEvidenceOmission{{
			Layer:  "eventConsumer",
			Reason: "Deployment label-selected list failed: " + err.Error(),
			Impact: "in-cluster event consumer health is not observed",
		}}
	}

	out := []GitOpsEventConsumerEvidence{}
	for i := range items {
		item := &items[i]
		labels := item.GetLabels()
		evidenceLabel := ""
		if strings.EqualFold(strings.TrimSpace(labels["app"]), "argobot") {
			evidenceLabel = "app=argobot"
		}
		if evidenceLabel == "" && strings.EqualFold(strings.TrimSpace(labels["app.kubernetes.io/name"]), "argobot") {
			evidenceLabel = "app.kubernetes.io/name=argobot"
		}
		if evidenceLabel == "" {
			continue
		}

		replicas, _, _ := unstructured.NestedInt64(item.Object, "spec", "replicas")
		readyReplicas, _, _ := unstructured.NestedInt64(item.Object, "status", "readyReplicas")
		availableReplicas, _, _ := unstructured.NestedInt64(item.Object, "status", "availableReplicas")
		ready := replicas > 0 && readyReplicas >= replicas
		out = append(out, GitOpsEventConsumerEvidence{
			Kind:              "Deployment",
			Name:              item.GetName(),
			Namespace:         item.GetNamespace(),
			Ready:             ready,
			Replicas:          replicas,
			ReadyReplicas:     readyReplicas,
			AvailableReplicas: availableReplicas,
			EvidenceLabel:     evidenceLabel,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	if len(out) == 0 {
		return nil, []GitOpsDeliveryEvidenceOmission{{
			Layer:  "eventConsumer",
			Reason: "no Deployment with label app=argobot or app.kubernetes.io/name=argobot was found",
			Impact: "evented sync/status feedback may still exist, but cub-scout did not observe the known open-source consumer shape",
		}}
	}
	return out, omissions
}

func listGitOpsEventConsumerDeployments(ctx context.Context, resource dynamic.NamespaceableResourceInterface, namespace string) ([]unstructured.Unstructured, []GitOpsDeliveryEvidenceOmission, error) {
	selectors := []string{"app=argobot", "app.kubernetes.io/name=argobot"}
	items := []unstructured.Unstructured{}
	seen := map[string]bool{}
	var omissions []GitOpsDeliveryEvidenceOmission
	var failures []string
	usedNamespaceFallback := false

	for _, selector := range selectors {
		list, err := resource.List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			if namespace == "" {
				failures = append(failures, selector+": "+err.Error())
				continue
			}
			list, err = resource.Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
			if err != nil {
				failures = append(failures, selector+": "+err.Error())
				continue
			}
			usedNamespaceFallback = true
		}

		for _, item := range list.Items {
			key := item.GetNamespace() + "/" + item.GetName()
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, item)
		}
	}

	if len(items) == 0 && len(failures) > 0 {
		return nil, nil, errors.New(strings.Join(failures, "; "))
	}
	if usedNamespaceFallback {
		omissions = append(omissions, GitOpsDeliveryEvidenceOmission{
			Layer:  "eventConsumer.scope",
			Reason: fmt.Sprintf("all-namespace Deployment list was unavailable; fell back to namespace %q", namespace),
			Impact: "an event consumer in another namespace could be missed",
		})
	}
	return items, omissions, nil
}

func mapStringFromJSONValue(raw interface{}) map[string]string {
	switch typed := raw.(type) {
	case map[string]string:
		return typed
	case map[string]interface{}:
		out := make(map[string]string, len(typed))
		for key, value := range typed {
			if s, ok := value.(string); ok {
				out[key] = strings.TrimSpace(s)
			}
		}
		return out
	default:
		return nil
	}
}

func mcpFirstRaw(item map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if raw, ok := item[key]; ok && raw != nil {
			return raw
		}
	}
	return nil
}

func outputGitOpsDeliveryEvidenceHuman(evidence *GitOpsDeliveryEvidence) {
	if evidence == nil {
		return
	}

	fmt.Printf("%sCONFIGHUB DELIVERY EVIDENCE%s\n", colorBold, colorReset)
	fmt.Printf("────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("  Scope: space=%s since=%s stale-after=%s max-items=%d\n",
		firstNonEmpty(evidence.Scope.Space, "(unknown)"),
		firstNonEmpty(evidence.Scope.Since, "-"),
		firstNonEmpty(evidence.Scope.StaleAfter, "-"),
		evidence.Scope.MaxItems,
	)

	if len(evidence.EventConsumers) > 0 {
		fmt.Printf("  Event consumers:\n")
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

	if evidence.ConfigHub != nil && len(evidence.ConfigHub.LiveStatuses) > 0 {
		fmt.Printf("  Live status writeback:\n")
		for _, status := range evidence.ConfigHub.LiveStatuses {
			fmt.Printf("    - %s app=%s sync=%s health=%s op=%s delivery=%s app-health=%s freshness=%s\n",
				firstNonEmpty(status.Space, "-"),
				firstNonEmpty(status.App, "-"),
				firstNonEmpty(status.SyncStatus, "-"),
				firstNonEmpty(status.HealthStatus, "-"),
				firstNonEmpty(status.OperationPhase, "-"),
				status.DeliveryVerdict,
				status.ApplicationHealthVerdict,
				status.Freshness,
			)
		}
	}
	if evidence.ConfigHub != nil && len(evidence.ConfigHub.Releases) > 0 {
		fmt.Printf("  Recent releases:\n")
		for _, release := range evidence.ConfigHub.Releases {
			fmt.Printf("    - %s target=%s digest=%s at=%s\n",
				firstNonEmpty(release.Slug, release.ReleaseID, "-"),
				firstNonEmpty(release.Target, "-"),
				truncate(firstNonEmpty(release.Digest, "-"), 18),
				firstNonEmpty(release.CreatedAt, "-"),
			)
		}
	}
	if evidence.ConfigHub != nil && len(evidence.ConfigHub.UnitEvents) > 0 {
		fmt.Printf("  Recent unit events:\n")
		for _, event := range evidence.ConfigHub.UnitEvents {
			fmt.Printf("    - %s unit=%s result=%s at=%s\n",
				firstNonEmpty(event.Action, event.Status, "-"),
				firstNonEmpty(event.Unit, "-"),
				firstNonEmpty(event.Result, event.Status, "-"),
				firstNonEmpty(event.CreatedAt, "-"),
			)
		}
	}
	if len(evidence.Omissions) > 0 {
		fmt.Printf("  Omissions:\n")
		for _, omission := range evidence.Omissions {
			fmt.Printf("    - %s: %s\n", omission.Layer, omission.Reason)
		}
	}
	fmt.Printf("\n")
}

func outputGitOpsStatusMarkdown(summary GitOpsSummary) error {
	var b strings.Builder
	b.WriteString("# GitOps Status\n\n")
	b.WriteString(fmt.Sprintf("- Backend: `%s`\n", summary.Backend))
	b.WriteString(fmt.Sprintf("- Transport: `%s`\n", summary.Transport))
	b.WriteString(fmt.Sprintf("- Healthy deployers: `%d`\n", summary.HealthyCount))
	b.WriteString(fmt.Sprintf("- Failed deployers: `%d`\n", summary.FailedCount))
	if summary.ConfigHubTarget != nil {
		b.WriteString(fmt.Sprintf("- ConfigHub target: space `%s`, target `%s`\n", summary.ConfigHubTarget.Space, summary.ConfigHubTarget.Target))
	}

	if len(summary.ControllerCoverage) > 0 {
		b.WriteString("\n## Controller Coverage\n\n")
		b.WriteString("| Family | Status | Found | Found kinds | Checked kinds | Omissions |\n")
		b.WriteString("|---|---|---:|---|---|---|\n")
		for _, entry := range summary.ControllerCoverage {
			b.WriteString(fmt.Sprintf("| %s | %s | %d | %s | %s | %s |\n",
				entry.Family,
				entry.Status,
				entry.Found,
				firstNonEmpty(strings.Join(entry.FoundKinds, ", "), "-"),
				firstNonEmpty(strings.Join(entry.ResourceKinds, ", "), "-"),
				controllerCoverageOmissionsMarkdown(entry.Omissions),
			))
		}
	}

	if len(summary.Sources) > 0 {
		b.WriteString("\n## Sources\n\n")
		b.WriteString("| Kind | Namespace | Name | Ready | Reason | URL/Digest |\n")
		b.WriteString("|---|---|---|---|---|---|\n")
		for _, src := range summary.Sources {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %t | %s | %s |\n",
				src.Kind,
				src.Namespace,
				src.Name,
				src.Ready,
				firstNonEmpty(src.Reason, "-"),
				firstNonEmpty(src.URL, truncate(src.ArtifactDigest, 18), "-"),
			))
		}
	}

	if len(summary.Deployers) > 0 {
		b.WriteString("\n## Deployers\n\n")
		b.WriteString("| Kind | Namespace | Name | Ready | Stage | Sync | Health | Message |\n")
		b.WriteString("|---|---|---|---|---|---|---|---|\n")
		for _, dep := range summary.Deployers {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %t | %s | %s | %s | %s |\n",
				dep.Kind,
				dep.Namespace,
				dep.Name,
				dep.Ready,
				dep.Stage,
				firstNonEmpty(dep.SyncStatus, "-"),
				firstNonEmpty(dep.HealthStatus, "-"),
				firstNonEmpty(dep.Message, "-"),
			))
		}
	}

	if summary.DeliveryEvidence != nil {
		renderGitOpsDeliveryEvidenceMarkdown(&b, summary.DeliveryEvidence)
	}

	fmt.Print(b.String())
	return nil
}

func controllerCoverageOmissionsMarkdown(omissions []ControllerCoverageOmission) string {
	if len(omissions) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(omissions))
	for _, omission := range omissions {
		parts = append(parts, fmt.Sprintf("%s: %s", omission.Resource, omission.Reason))
	}
	return strings.Join(parts, "<br>")
}

func renderGitOpsDeliveryEvidenceMarkdown(b *strings.Builder, evidence *GitOpsDeliveryEvidence) {
	b.WriteString("\n## ConfigHub Delivery Evidence\n\n")
	b.WriteString(fmt.Sprintf("- Space: `%s`\n", firstNonEmpty(evidence.Scope.Space, "-")))
	b.WriteString(fmt.Sprintf("- Since: `%s`\n", firstNonEmpty(evidence.Scope.Since, "-")))
	b.WriteString(fmt.Sprintf("- Stale after: `%s`\n", firstNonEmpty(evidence.Scope.StaleAfter, "-")))

	if len(evidence.EventConsumers) > 0 {
		b.WriteString("\n### Event Consumers\n\n")
		b.WriteString("| Namespace | Name | Ready | Replicas |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, consumer := range evidence.EventConsumers {
			b.WriteString(fmt.Sprintf("| %s | %s | %t | %d/%d |\n",
				firstNonEmpty(consumer.Namespace, "-"),
				consumer.Name,
				consumer.Ready,
				consumer.ReadyReplicas,
				consumer.Replicas,
			))
		}
	}

	if evidence.ConfigHub != nil && len(evidence.ConfigHub.LiveStatuses) > 0 {
		b.WriteString("\n### Live Status Writeback\n\n")
		b.WriteString("| Space | App | Sync | Health | Operation | Delivery | App Health | Freshness |\n")
		b.WriteString("|---|---|---|---|---|---|---|---|\n")
		for _, status := range evidence.ConfigHub.LiveStatuses {
			b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s |\n",
				firstNonEmpty(status.Space, "-"),
				firstNonEmpty(status.App, "-"),
				firstNonEmpty(status.SyncStatus, "-"),
				firstNonEmpty(status.HealthStatus, "-"),
				firstNonEmpty(status.OperationPhase, "-"),
				status.DeliveryVerdict,
				status.ApplicationHealthVerdict,
				status.Freshness,
			))
		}
	}

	if len(evidence.Omissions) > 0 {
		b.WriteString("\n### Omissions\n\n")
		for _, omission := range evidence.Omissions {
			b.WriteString(fmt.Sprintf("- `%s`: %s\n", omission.Layer, omission.Reason))
		}
	}
}
