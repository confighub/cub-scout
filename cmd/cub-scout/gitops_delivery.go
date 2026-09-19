// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
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
	// SpaceSource records how Space was chosen; see GitOpsDeliveryEvidenceScope.
	SpaceSource string
	// MatchBeforeTrim keeps every row in the time window so a caller that
	// correlates rows to one object can match first and trim the matches.
	// Trimming the whole space to MaxItems first would drop a busy space's
	// older rows and turn a real match into a false "no row matched".
	MatchBeforeTrim bool
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
	Namespace string `json:"namespace,omitempty"`
	Space     string `json:"space,omitempty"`
	// SpaceSource says how Space was chosen: "flag", "resource" or "CUB_SPACE".
	// CUB_SPACE is environment state, so a reader should be able to see when a
	// result depended on it.
	SpaceSource string `json:"spaceSource,omitempty"`
	Since       string `json:"since"`
	StaleAfter  string `json:"staleAfter"`
	MaxItems    int    `json:"maxItems"`
}

type ConfigHubDeliveryEvidence struct {
	LiveStatuses []ConfigHubLiveStatusEvidence `json:"liveStatuses,omitempty"`
	Releases     []ConfigHubReleaseEvidence    `json:"releases,omitempty"`
	UnitEvents   []ConfigHubUnitEventEvidence  `json:"unitEvents,omitempty"`
	// ReleasesTotal and UnitEventsTotal count the rows in the time window
	// before Releases and UnitEvents were trimmed to maxItems.
	ReleasesTotal   int `json:"releasesTotal,omitempty"`
	UnitEventsTotal int `json:"unitEventsTotal,omitempty"`

	// allUnitEvents is every unit event in the window, kept so that a failure
	// among rows trimmed from UnitEvents is still found. It is not output.
	allUnitEvents []ConfigHubUnitEventEvidence
}

// unitEventsInWindow returns every unit event read, before trimming.
func (e *ConfigHubDeliveryEvidence) unitEventsInWindow() []ConfigHubUnitEventEvidence {
	if e.allUnitEvents != nil {
		return e.allUnitEvents
	}
	return e.UnitEvents
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
	Slug      string `json:"slug,omitempty"`
	ReleaseID string `json:"releaseId,omitempty"`
	Space     string `json:"space,omitempty"`
	SpaceID   string `json:"spaceId,omitempty"`
	Target    string `json:"target,omitempty"`
	TargetID  string `json:"targetId,omitempty"`
	// Digest is the bundle's content digest. It is NOT what a puller reports:
	// against a live server, Release.Digest and the registry's
	// Docker-Content-Digest for the same release are different strings.
	Digest string `json:"digest,omitempty"`
	// ManifestDigest is the OCI manifest digest, which is exactly what the
	// registry returns as Docker-Content-Digest and what Argo and Flux record
	// as a resolved revision. Alongside source registry and space, it joins
	// source evidence to a Release, not a workload to proven execution.
	ManifestDigest string `json:"manifestDigest,omitempty"`
	BundleBaseName string `json:"bundleBaseName,omitempty"`
	RevisionNum    int    `json:"revisionNum,omitempty"`
	// ReleaseNum is the Release's sequence number within its space.
	ReleaseNum int `json:"releaseNum,omitempty"`
	// Published reports whether the Release is currently served to its Target.
	// ConfigHub clears it when a Release is withdrawn and keeps the row. Nil
	// means the server did not report it, which is not the same as false.
	Published *bool  `json:"published,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
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
	requireGitOpsConfigHubFn = requireGitOpsConfigHubConnected
	runGitOpsCubCommand      = runHistoryCubCommandImpl
	gitopsNowFn              = time.Now
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

	opts.Space, opts.SpaceSource = gitOpsDeliverySpace(opts.Space)
	return opts, nil
}

func requireGitOpsConfigHubConnected() error {
	return requireConfigHubFor("ConfigHub delivery evidence")
}

// gitOpsDeliverySpace settles the delivery-evidence space from a flag value and
// says where it came from. An empty result is left for the collector, which
// reports a confighub.scope omission and skips the reads rather than widen.
func gitOpsDeliverySpace(flagValue string) (slug, source string) {
	space := resolveConfigHubSpace(flagValue)
	return space.Slug, space.Source
}

func collectGitOpsDeliveryEvidence(ctx context.Context, client dynamic.Interface, opts gitOpsDeliveryEvidenceOptions) *GitOpsDeliveryEvidence {
	if opts.Now.IsZero() {
		opts.Now = gitopsNowFn().UTC()
	}
	if opts.SpaceSource == "" {
		opts.Space, opts.SpaceSource = gitOpsDeliverySpace(opts.Space)
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
			Namespace:   opts.Namespace,
			Space:       opts.Space,
			SpaceSource: opts.SpaceSource,
			Since:       opts.Since,
			StaleAfter:  opts.StaleAfter.String(),
			MaxItems:    opts.MaxItems,
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
	rowLimit := opts.MaxItems
	if opts.MatchBeforeTrim {
		rowLimit = 0
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
			Reason: "no ConfigHub space was given: pass --confighub-space <slug> (or '*' for every space) or set CUB_SPACE; cub has no default space",
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
		releases, omissions := buildConfigHubReleaseEvidence(rawReleases, 0)
		evidence.ConfigHub.ReleasesTotal = len(releases)
		omissions = append(omissions, trimReleaseEvidence(&releases, rowLimit)...)
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
		events, omissions := buildConfigHubUnitEventEvidence(rawEvents, 0)
		evidence.ConfigHub.UnitEventsTotal = len(events)
		evidence.ConfigHub.allUnitEvents = append([]ConfigHubUnitEventEvidence(nil), events...)
		omissions = append(omissions, trimUnitEventEvidence(&events, rowLimit)...)
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

// gitOpsConfigHubReleaseListArgs deliberately sends no --select. ConfigHub
// rejects a selection naming any field the Release entity lacks with HTTP 400,
// and that field set moves between server versions (Slug became BundleBaseName;
// v0.5 dropped BridgeWorkerID and added TargetID). A Release is a handful of
// scalars and the read is already bounded by the CreatedAt cutoff, so the full
// object is cheap and keeps this read working across versions. The MCP
// confighub_releases reader makes the same choice.
func gitOpsConfigHubReleaseListArgs(space, cutoff string) []string {
	args := []string{
		"release", "list",
		"--space", space,
		"-o", "json",
		"--where", fmt.Sprintf("CreatedAt > '%s'", configHubFilterQuote(cutoff)),
	}
	return args
}

// gitOpsConfigHubUnitEventListArgs sends no --select, for the same reason as the
// release read. cub does not forward a selection for unit events today, so the
// old one was a no-op; it also named Unit, Space and Target, which are not
// UnitEvent fields and would be rejected the day cub starts forwarding it.
func gitOpsConfigHubUnitEventListArgs(space, cutoff string) []string {
	args := []string{
		"unit-event", "list",
		"--space", space,
		"-o", "json",
		"--where", fmt.Sprintf("CreatedAt > '%s'", configHubFilterQuote(cutoff)),
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

	observedAt, validTime := parseConfigHubObservedAt(status.ObservedAt)
	freshnessProblem := ""
	switch {
	case status.ObservedAt == "":
		freshnessProblem = "missing observedAt"
	case !validTime || observedAt.IsZero():
		freshnessProblem = "invalid observedAt"
	case now.IsZero():
		freshnessProblem = "observation clock is unavailable"
	case observedAt.After(now):
		freshnessProblem = "observedAt is after the observation clock (no clock-skew allowance)"
	case staleAfter <= 0:
		freshnessProblem = "freshness threshold must be positive"
	default:
		age := now.Sub(observedAt.UTC())
		status.FreshnessSeconds = int64(age.Seconds())
		status.Freshness = "fresh"
		if observedAt.Before(now.Add(-staleAfter)) {
			status.Freshness = "stale"
			freshnessProblem = fmt.Sprintf("observedAt is older than the %s freshness threshold", staleAfter)
		}
	}
	status.DeliveryVerdict = configHubDeliveryVerdict(status)
	status.ApplicationHealthVerdict = configHubApplicationHealthVerdict(status)
	if freshnessProblem != "" {
		return status, true, GitOpsDeliveryEvidenceOmission{
			Layer:  "confighub.liveStatus.freshness",
			Reason: fmt.Sprintf("space %q app %q: %s", firstNonEmpty(space, spaceID, "unknown"), firstNonEmpty(status.App, "unknown"), freshnessProblem),
			Impact: "reported state is retained but does not establish current delivery or application health; read current controller/workload status",
		}
	}
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
	return configHubVerdictWithFreshness(verdict, status.Freshness)
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
	return configHubVerdictWithFreshness(verdict, status.Freshness)
}

// Preserve reported state separately; an undated or old report cannot assert
// either current success or current failure.
func configHubVerdictWithFreshness(verdict agent.ReceiptVerdict, freshness string) agent.ReceiptVerdict {
	switch freshness {
	case "fresh":
		return verdict
	case "stale":
		if verdict != agent.VerdictINCONCLUSIVE {
			return agent.VerdictWATCH
		}
	}
	return agent.VerdictINCONCLUSIVE
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
		space, spaceID := configHubRelatedRef(item, releaseObj, "Space")
		target, targetID := configHubRelatedRef(item, releaseObj, "Target")

		release := ConfigHubReleaseEvidence{
			Slug:           mcpFirstString(releaseObj, "Slug", "slug", "Name", "name"),
			ReleaseID:      mcpFirstString(releaseObj, "ReleaseID", "releaseId", "ID", "id"),
			Space:          space,
			SpaceID:        spaceID,
			Target:         target,
			TargetID:       targetID,
			Digest:         mcpFirstString(releaseObj, "Digest", "digest", "BundleDigest", "bundleDigest"),
			ManifestDigest: mcpFirstString(releaseObj, "ManifestDigest", "manifestDigest", "OCIManifestDigest", "ociManifestDigest"),
			BundleBaseName: mcpFirstString(releaseObj, "BundleBaseName", "bundleBaseName", "Bundle", "bundle"),
			CreatedAt:      mcpFirstString(releaseObj, "CreatedAt", "createdAt", "Timestamp", "timestamp"),
		}
		if value, ok := mcpFirstInt(releaseObj, "RevisionNum", "revisionNum", "RevisionNumber", "revisionNumber"); ok {
			release.RevisionNum = value
		}
		if value, ok := mcpFirstInt(releaseObj, "ReleaseNum", "releaseNum"); ok {
			release.ReleaseNum = value
		}
		if value, ok := configHubBoolField(releaseObj, "Published", "published"); ok {
			release.Published = &value
		}
		releases = append(releases, release)
	}

	sort.Slice(releases, func(i, j int) bool {
		return evidenceTimeAfter(releases[i].CreatedAt, releases[j].CreatedAt)
	})
	omissions := trimReleaseEvidence(&releases, maxItems)
	return releases, omissions
}

// configHubRelatedRef reads the slug and ID of an entity related to a list row.
// kind is "Space", "Target" or "Unit".
//
// ConfigHub names a related entity in one of two ways. An extended response
// carries it as a sibling object, {"Space":{"Slug":...,"SpaceID":...}}. A bare
// entity, which is what `cub release list` and `cub unit-event list` print,
// carries it as flat fields on the row itself, "SpaceSlug" and "SpaceID". The
// generic "Slug", "Name" and "ID" keys are read only from the sibling object:
// on the row they are the row's own identity, not the related entity's.
func configHubRelatedRef(item, row map[string]interface{}, kind string) (slug, id string) {
	lower := strings.ToLower(kind[:1]) + kind[1:]
	related := mcpNestedMap(item, kind, lower)
	if related == nil {
		related = mcpNestedMap(row, kind, lower)
	}
	slugKeys := []string{kind + "Slug", lower + "Slug"}
	idKeys := []string{kind + "ID", lower + "Id"}
	slug = firstNonEmpty(
		mcpFirstString(related, append([]string{"Slug", "slug", "Name", "name"}, slugKeys...)...),
		mcpFirstString(row, slugKeys...),
	)
	id = firstNonEmpty(
		mcpFirstString(related, append(append([]string{}, idKeys...), "ID", "id")...),
		mcpFirstString(row, idKeys...),
	)
	return slug, id
}

// configHubReleaseLabel is the shortest name that tells one Release from
// another: a slug if the server gave one, else bundle#number, else the ID.
// ReleaseNum is unique within a space, which is the scope the label is shown in.
func configHubReleaseLabel(release ConfigHubReleaseEvidence) string {
	return configHubReleaseDisplayName(release.Slug, release.BundleBaseName, release.ReleaseNum, release.ReleaseID)
}

func configHubReleaseDisplayName(slug, bundleBaseName string, releaseNum int, releaseID string) string {
	if slug != "" {
		return slug
	}
	if bundleBaseName != "" && releaseNum > 0 {
		return fmt.Sprintf("%s#%d", bundleBaseName, releaseNum)
	}
	return firstNonEmpty(releaseID, "unknown")
}

// configHubPublishedText renders the three publication states for text output.
func configHubPublishedText(published *bool) string {
	switch {
	case published == nil:
		return "unknown"
	case *published:
		return "true"
	default:
		return "false"
	}
}

// configHubUnitEventOutcome is the text for a unit event's outcome. ConfigHub
// sets Result to "None" for an ordinary Apply and records how it ended in
// Status, so printing Result alone shows "None" for a failed Apply.
func configHubUnitEventOutcome(result, status string) string {
	return fmt.Sprintf("result=%s status=%s", firstNonEmpty(result, "-"), firstNonEmpty(status, "-"))
}

// configHubNonZeroTime drops Go's zero time. ConfigHub marshals TerminatedAt
// without omitempty, so an event still in progress carries
// "0001-01-01T00:00:00Z". Taken at face value that becomes the row's time, in
// the year 1, and the row falls outside every --since window.
func configHubNonZeroTime(value string) string {
	if strings.HasPrefix(strings.TrimSpace(value), "0001-01-01T00:00:00") {
		return ""
	}
	return value
}

// configHubBoolField reads a boolean field, reporting whether it was present.
// Absent is distinct from false.
func configHubBoolField(item map[string]interface{}, keys ...string) (bool, bool) {
	for _, key := range keys {
		if value, ok := item[key].(bool); ok {
			return value, true
		}
	}
	return false, false
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
		unit, unitID := configHubRelatedRef(item, eventObj, "Unit")
		space, spaceID := configHubRelatedRef(item, eventObj, "Space")
		target, targetID := configHubRelatedRef(item, eventObj, "Target")

		events = append(events, ConfigHubUnitEventEvidence{
			EventID:      mcpFirstString(eventObj, "UnitEventID", "unitEventId", "EventID", "eventId", "ID", "id"),
			Action:       mcpFirstString(eventObj, "Action", "action", "Type", "type"),
			Result:       mcpFirstString(eventObj, "Result", "result"),
			Status:       mcpFirstString(eventObj, "Status", "status", "Condition", "condition"),
			Message:      mcpFirstString(eventObj, "Message", "message", "Summary", "summary"),
			Unit:         unit,
			UnitID:       unitID,
			Space:        space,
			SpaceID:      spaceID,
			Target:       target,
			TargetID:     targetID,
			CreatedAt:    mcpFirstString(eventObj, "CreatedAt", "createdAt", "Timestamp", "timestamp"),
			TerminatedAt: configHubNonZeroTime(mcpFirstString(eventObj, "TerminatedAt", "terminatedAt", "CompletedAt", "completedAt")),
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
			fmt.Printf("    - %s target=%s published=%s digest=%s at=%s\n",
				configHubReleaseLabel(release),
				firstNonEmpty(release.Target, release.TargetID, "-"),
				configHubPublishedText(release.Published),
				truncate(firstNonEmpty(release.Digest, "-"), 18),
				firstNonEmpty(release.CreatedAt, "-"),
			)
		}
	}
	if evidence.ConfigHub != nil && len(evidence.ConfigHub.UnitEvents) > 0 {
		fmt.Printf("  Recent unit events:\n")
		for _, event := range evidence.ConfigHub.UnitEvents {
			fmt.Printf("    - %s unit=%s %s at=%s\n",
				firstNonEmpty(event.Action, "-"),
				firstNonEmpty(event.Unit, event.UnitID, "-"),
				configHubUnitEventOutcome(event.Result, event.Status),
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
