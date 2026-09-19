// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestNormalizeGitOpsStatusFormat(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		legacyJSON bool
		want       string
		wantErr    bool
	}{
		{name: "default ascii", format: "", want: "ascii"},
		{name: "explicit json", format: "json", want: "json"},
		{name: "explicit markdown", format: "md", want: "md"},
		{name: "legacy json wins", format: "ascii", legacyJSON: true, want: "json"},
		{name: "invalid", format: "yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeGitOpsStatusFormat(tt.format, tt.legacyJSON)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("format = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildConfigHubLiveStatusEvidence_SeparatesDeliveryAndApplicationHealth(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	raw := `[
		{
			"Space": {
				"Slug": "payments-prod",
				"SpaceID": "sp-123",
				"Annotations": {
					"confighub.com/live-status": "{\"source\":\"argobot\",\"app\":\"payments-prod\",\"syncStatus\":\"Synced\",\"healthStatus\":\"Healthy\",\"operationPhase\":\"Succeeded\",\"revision\":\"sha256:abc\",\"observedAt\":\"2026-09-10T11:55:00Z\"}"
				}
			}
		}
	]`

	statuses, omissions := buildConfigHubLiveStatusEvidence(raw, now, 15*time.Minute)
	if len(omissions) != 0 {
		t.Fatalf("omissions = %+v, want none", omissions)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	got := statuses[0]
	if got.Space != "payments-prod" || got.App != "payments-prod" {
		t.Fatalf("status identity = %+v, want payments-prod", got)
	}
	if got.Freshness != "fresh" || got.FreshnessSeconds != 300 {
		t.Fatalf("freshness = %s/%d, want fresh/300", got.Freshness, got.FreshnessSeconds)
	}
	if got.DeliveryVerdict != agent.VerdictPASS {
		t.Fatalf("delivery verdict = %s, want PASS", got.DeliveryVerdict)
	}
	if got.ApplicationHealthVerdict != agent.VerdictPASS {
		t.Fatalf("application health verdict = %s, want PASS", got.ApplicationHealthVerdict)
	}
}

func TestBuildConfigHubLiveStatusEvidence_StalePassDowngradesToWatch(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	raw := `[
		{
			"Slug": "payments-prod",
			"SpaceID": "sp-123",
			"Annotations": {
				"confighub.com/live-status": "{\"source\":\"argobot\",\"app\":\"payments-prod\",\"syncStatus\":\"Synced\",\"healthStatus\":\"Healthy\",\"operationPhase\":\"Succeeded\",\"observedAt\":\"2026-09-10T10:00:00Z\"}"
			}
		}
	]`

	statuses, omissions := buildConfigHubLiveStatusEvidence(raw, now, 15*time.Minute)
	if len(omissions) != 1 || omissions[0].Layer != "confighub.liveStatus.freshness" {
		t.Fatalf("omissions = %+v, want stale observation explanation", omissions)
	}
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	got := statuses[0]
	if got.Freshness != "stale" {
		t.Fatalf("freshness = %q, want stale", got.Freshness)
	}
	if got.DeliveryVerdict != agent.VerdictWATCH {
		t.Fatalf("delivery verdict = %s, want WATCH for stale pass", got.DeliveryVerdict)
	}
	if got.ApplicationHealthVerdict != agent.VerdictWATCH {
		t.Fatalf("application health verdict = %s, want WATCH for stale pass", got.ApplicationHealthVerdict)
	}
}

func TestBuildConfigHubLiveStatusEvidence_MalformedOrMissingAnnotationsBecomeOmissions(t *testing.T) {
	raw := `[
		{"Slug":"no-status","Annotations":{}},
		{"Slug":"bad-status","Annotations":{"confighub.com/live-status":"not json"}}
	]`

	statuses, omissions := buildConfigHubLiveStatusEvidence(raw, time.Now().UTC(), 15*time.Minute)
	if len(statuses) != 0 {
		t.Fatalf("statuses = %+v, want none", statuses)
	}
	if len(omissions) != 2 {
		t.Fatalf("omissions = %d, want 2: %+v", len(omissions), omissions)
	}
	got := omissions[0].Layer + " " + omissions[1].Layer
	if !strings.Contains(got, "confighub.liveStatus") {
		t.Fatalf("omission layers = %q", got)
	}
}

func TestCollectGitOpsEventConsumerEvidence_DetectsArgobotDeploymentByLabel(t *testing.T) {
	client := newGitOpsDeliveryFakeClient(argobotDeployment("argobot", "argobot", 1, 1, "app"))

	consumers, omissions := collectGitOpsEventConsumerEvidence(context.Background(), client, "")
	if len(omissions) != 0 {
		t.Fatalf("omissions = %+v, want none", omissions)
	}
	if len(consumers) != 1 {
		t.Fatalf("consumers = %d, want 1", len(consumers))
	}
	got := consumers[0]
	if got.Name != "argobot" || got.Namespace != "argobot" {
		t.Fatalf("consumer identity = %+v, want argobot/argobot", got)
	}
	if !got.Ready || got.Replicas != 1 || got.ReadyReplicas != 1 {
		t.Fatalf("consumer readiness = %+v, want ready 1/1", got)
	}
	if got.EvidenceLabel != "app=argobot" {
		t.Fatalf("evidence label = %q, want app=argobot", got.EvidenceLabel)
	}
}

func TestCollectGitOpsEventConsumerEvidence_SearchesOutsideRequestedNamespace(t *testing.T) {
	client := newGitOpsDeliveryFakeClient(argobotDeployment("argobot", "confighub-ops", 1, 1, "app.kubernetes.io/name"))

	consumers, omissions := collectGitOpsEventConsumerEvidence(context.Background(), client, "payments")
	if len(omissions) != 0 {
		t.Fatalf("omissions = %+v, want none", omissions)
	}
	if len(consumers) != 1 {
		t.Fatalf("consumers = %d, want 1", len(consumers))
	}
	got := consumers[0]
	if got.Namespace != "confighub-ops" || got.EvidenceLabel != "app.kubernetes.io/name=argobot" {
		t.Fatalf("consumer = %+v, want confighub-ops with app.kubernetes.io/name label", got)
	}
}

func TestCollectGitOpsDeliveryEvidence_BoundsConfigHubReadsAndKeepsOmissionsStructured(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	client := newGitOpsDeliveryFakeClient(argobotDeployment("argobot", "argobot", 1, 1, "app"))

	oldRequire := requireGitOpsConfigHubFn
	oldRun := runGitOpsCubCommand
	t.Cleanup(func() {
		requireGitOpsConfigHubFn = oldRequire
		runGitOpsCubCommand = oldRun
	})

	requireGitOpsConfigHubFn = func() error { return nil }
	stubSpaceInputs(t, "payments-prod")

	var calls [][]string
	runGitOpsCubCommand = func(ctx context.Context, args []string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		switch {
		case reflect.DeepEqual(args, gitOpsConfigHubSpaceListArgs("payments-prod")):
			return `[{"Space":{"Slug":"payments-prod","SpaceID":"sp-123","Annotations":{"confighub.com/live-status":"{\"source\":\"argobot\",\"app\":\"payments-prod\",\"syncStatus\":\"Synced\",\"healthStatus\":\"Healthy\",\"operationPhase\":\"Succeeded\",\"revision\":\"sha256:abc\",\"observedAt\":\"2026-09-10T11:55:00Z\"}"}}}]`, nil
		case len(args) > 1 && args[0] == "release" && args[1] == "list":
			// The recorded server shape, not an invented one: see recordedV05ReleaseList.
			return recordedV05ReleaseList, nil
		case len(args) > 1 && args[0] == "unit-event" && args[1] == "list":
			return `[{"UnitEvent":{"UnitEventID":"ue-1","Action":"ReleasePublished","Result":"Succeeded","CreatedAt":"2026-09-10T11:51:00Z"},"Unit":{"Slug":"payments-api","UnitID":"u-123"},"Space":{"Slug":"payments-prod","SpaceID":"sp-123"},"Target":{"Slug":"prod","TargetID":"t-123"}}]`, nil
		default:
			t.Fatalf("unexpected cub args: %v", args)
			return "", nil
		}
	}

	evidence := collectGitOpsDeliveryEvidence(context.Background(), client, gitOpsDeliveryEvidenceOptions{
		Space:      "",
		Since:      "24h",
		Window:     24 * time.Hour,
		StaleAfter: 15 * time.Minute,
		Now:        now,
		MaxItems:   10,
	})

	if evidence.Scope.Space != "payments-prod" {
		t.Fatalf("scope space = %q, want default payments-prod", evidence.Scope.Space)
	}
	if len(calls) != 3 {
		t.Fatalf("cub calls = %d, want 3: %+v", len(calls), calls)
	}
	for _, args := range calls {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--space *") {
			t.Fatalf("unexpected unbounded all-space read: %s", joined)
		}
		if args[0] != "space" && !strings.Contains(joined, "CreatedAt > '2026-09-09T12:00:00Z'") {
			t.Fatalf("missing time-window where clause in %v", args)
		}
	}
	if len(evidence.EventConsumers) != 1 || !evidence.EventConsumers[0].Ready {
		t.Fatalf("event consumers = %+v, want one ready consumer", evidence.EventConsumers)
	}
	if len(evidence.ConfigHub.LiveStatuses) != 1 || evidence.ConfigHub.LiveStatuses[0].DeliveryVerdict != agent.VerdictPASS {
		t.Fatalf("live statuses = %+v, want one PASS status", evidence.ConfigHub.LiveStatuses)
	}
	if len(evidence.ConfigHub.Releases) != 1 || evidence.ConfigHub.Releases[0].TargetID != "55555555-5555-4555-8555-555555555555" {
		t.Fatalf("releases = %+v, want one parsed release", evidence.ConfigHub.Releases)
	}
	if len(evidence.ConfigHub.UnitEvents) != 1 || evidence.ConfigHub.UnitEvents[0].Action != "ReleasePublished" {
		t.Fatalf("unit events = %+v, want one parsed event", evidence.ConfigHub.UnitEvents)
	}
}

// ConfigHub answers a --select naming a field the entity lacks with HTTP 400,
// which turned every release read into an omission. The read stays bounded by
// space and cutoff; it must not pin a field list.
func TestGitOpsConfigHubReleaseListArgs_SendsNoSelect(t *testing.T) {
	args := gitOpsConfigHubReleaseListArgs("payments-prod", "2026-09-09T12:00:00Z")
	joined := strings.Join(args, " ")

	if strings.Contains(joined, "--select") {
		t.Fatalf("release list args pin a field selection, which ConfigHub rejects when any field is unknown: %v", args)
	}
	for _, want := range []string{"release list", "--space payments-prod", "-o json", "CreatedAt > '2026-09-09T12:00:00Z'"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("release list args = %v, want them to contain %q", args, want)
		}
	}
}

// Recorded from `cub release list -o json` against a ConfigHub v0.5.1 server,
// with identifiers replaced. There is no Release.Slug and no sibling Space or
// Target object: space and target identity are fields of the Release.
const recordedV05ReleaseList = `[{"Release":{
	"BundleBaseName":"payments-prod",
	"CreatedAt":"2026-09-03T15:02:09.791039Z",
	"DataSize":1104,
	"Digest":"sha256:0c9ae4903b120a5f9c79239c16d62a4cd04c1b540bc83541fde4c52ae0ac8d2e",
	"EntityType":"Release",
	"ManifestDigest":"sha256:0df04e007c0d5d8b2e89eec57ced4d16a2f738edff7ed43e830f652dc11897cf",
	"OrganizationID":"11111111-1111-4111-8111-111111111111",
	"Published":true,
	"ReleaseID":"22222222-2222-4222-8222-222222222222",
	"ReleaseNum":1,
	"SpaceID":"33333333-3333-4333-8333-333333333333",
	"SpaceSlug":"payments-prod",
	"TagID":"44444444-4444-4444-8444-444444444444",
	"TargetID":"55555555-5555-4555-8555-555555555555",
	"UnitCount":5,
	"UpdatedAt":"2026-09-03T15:02:09.791039Z"
}}]`

func TestBuildConfigHubReleaseEvidence_ParsesRecordedV05ReleaseShape(t *testing.T) {
	releases, omissions := buildConfigHubReleaseEvidence(recordedV05ReleaseList, 10)
	if len(omissions) != 0 {
		t.Fatalf("omissions = %+v, want none for a well-formed release list", omissions)
	}
	if len(releases) != 1 {
		t.Fatalf("releases = %+v, want one", releases)
	}

	got := releases[0]
	want := ConfigHubReleaseEvidence{
		ReleaseID: "22222222-2222-4222-8222-222222222222",
		Space:     "payments-prod",
		SpaceID:   "33333333-3333-4333-8333-333333333333",
		TargetID:  "55555555-5555-4555-8555-555555555555",
		Digest:    "sha256:0c9ae4903b120a5f9c79239c16d62a4cd04c1b540bc83541fde4c52ae0ac8d2e",
		// The recorded shape carried this all along and the reader dropped it.
		// It is the only field that joins a Release to what a controller
		// pulled: against a live server the registry's Docker-Content-Digest
		// equals ManifestDigest, and differs from Digest.
		ManifestDigest: "sha256:0df04e007c0d5d8b2e89eec57ced4d16a2f738edff7ed43e830f652dc11897cf",
		BundleBaseName: "payments-prod",
		ReleaseNum:     1,
		Published:      boolPtr(true),
		CreatedAt:      "2026-09-03T15:02:09.791039Z",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("release evidence =\n  %+v\nwant\n  %+v", got, want)
	}
}

// Published is three-valued. A withdrawn Release reports false; a server that
// does not report the field leaves it nil, which must not read as false.
func TestBuildConfigHubReleaseEvidence_PublishedIsThreeValued(t *testing.T) {
	raw := `[
		{"Release":{"ReleaseID":"served","Published":true,"CreatedAt":"2026-09-03T15:02:03Z"}},
		{"Release":{"ReleaseID":"withdrawn","Published":false,"CreatedAt":"2026-09-03T15:02:02Z"}},
		{"Release":{"ReleaseID":"unreported","CreatedAt":"2026-09-03T15:02:01Z"}}
	]`
	releases, omissions := buildConfigHubReleaseEvidence(raw, 0)
	if len(omissions) != 0 || len(releases) != 3 {
		t.Fatalf("releases = %+v omissions = %+v, want three rows", releases, omissions)
	}
	if p := releases[0].Published; p == nil || !*p {
		t.Fatalf("served release Published = %v, want true", p)
	}
	if p := releases[1].Published; p == nil || *p {
		t.Fatalf("withdrawn release Published = %v, want false", p)
	}
	if p := releases[2].Published; p != nil {
		t.Fatalf("unreported Published = %v, want nil: absent is not false", *p)
	}
}

// `cub unit-event list -o json` prints bare UnitEvent objects. This shape is
// derived from the UnitEvent schema in ConfigHub's public OpenAPI document and
// from cub's list command at v0.5.1. It is not a recording: the server it was
// tested against held no unit events. A UnitEvent names its unit and space as
// flat fields and has no target at all. Action, Result and Status use values
// from the published enums.
const derivedV05UnitEventList = `[{
	"Action":"Apply",
	"BridgeWorkerID":"66666666-6666-4666-8666-666666666666",
	"CreatedAt":"2026-09-03T15:03:00Z",
	"EntityType":"UnitEvent",
	"Message":"applied",
	"OrganizationID":"11111111-1111-4111-8111-111111111111",
	"Result":"None",
	"RevisionNum":4,
	"SpaceID":"33333333-3333-4333-8333-333333333333",
	"SpaceSlug":"payments-prod",
	"Status":"Completed",
	"TerminatedAt":"2026-09-03T15:03:05Z",
	"UnitEventID":"77777777-7777-4777-8777-777777777777",
	"UnitEventNum":9,
	"UnitID":"88888888-8888-4888-8888-888888888888",
	"UnitSlug":"payments-api"
}]`

func TestBuildConfigHubUnitEventEvidence_ReadsFlatUnitAndSpaceIdentity(t *testing.T) {
	events, omissions := buildConfigHubUnitEventEvidence(derivedV05UnitEventList, 10)
	if len(omissions) != 0 || len(events) != 1 {
		t.Fatalf("events = %+v omissions = %+v, want one event", events, omissions)
	}
	want := ConfigHubUnitEventEvidence{
		EventID:      "77777777-7777-4777-8777-777777777777",
		Action:       "Apply",
		Result:       "None",
		Status:       "Completed",
		Message:      "applied",
		Unit:         "payments-api",
		UnitID:       "88888888-8888-4888-8888-888888888888",
		Space:        "payments-prod",
		SpaceID:      "33333333-3333-4333-8333-333333333333",
		CreatedAt:    "2026-09-03T15:03:00Z",
		TerminatedAt: "2026-09-03T15:03:05Z",
	}
	if events[0] != want {
		t.Fatalf("unit event evidence =\n  %+v\nwant\n  %+v", events[0], want)
	}
}

// ConfigHub marshals TerminatedAt without omitempty, so an event that has not
// finished carries Go's zero time. Read as a real time it dates the activity
// row to the year 1, which no --since window includes. The premise comes from
// the generated UnitEvent model; the row below is constructed, not recorded.
func TestUnitEventStillInProgressKeepsItsCreatedTime(t *testing.T) {
	inProgress := strings.Replace(derivedV05UnitEventList, `"TerminatedAt":"2026-09-03T15:03:05Z"`, `"TerminatedAt":"0001-01-01T00:00:00Z"`, 1)
	inProgress = strings.Replace(inProgress, `"Status":"Completed"`, `"Status":"Progressing"`, 1)
	events, _ := buildConfigHubUnitEventEvidence(inProgress, 10)
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one", events)
	}
	if events[0].TerminatedAt != "" {
		t.Fatalf("terminatedAt = %q, want empty for an event that has not terminated", events[0].TerminatedAt)
	}

	evidence := &GitOpsDeliveryEvidence{ObservedAt: time.Date(2026, 9, 3, 16, 0, 0, 0, time.UTC)}
	row := configHubUnitEventActivityRow(evidence, events[0])
	if row.Time != "2026-09-03T15:03:00Z" {
		t.Fatalf("activity row time = %q, want the event's CreatedAt", row.Time)
	}
	if row.Result != "pending" {
		t.Fatalf("activity row result = %q, want pending for Status=Progressing", row.Result)
	}
	// A UnitEvent names no target. "target=-" would read as a missing value.
	if strings.Contains(row.Message, "target=") {
		t.Fatalf("message = %q, want no target field", row.Message)
	}
	if !strings.Contains(row.Message, "result=None status=Progressing") {
		t.Fatalf("message = %q, want both result and status", row.Message)
	}
}

func TestGitOpsConfigHubUnitEventListArgs_SendsNoSelect(t *testing.T) {
	joined := strings.Join(gitOpsConfigHubUnitEventListArgs("payments-prod", "2026-09-09T12:00:00Z"), " ")
	if strings.Contains(joined, "--select") {
		t.Fatalf("unit-event list args pin a field selection: %s", joined)
	}
	for _, want := range []string{"unit-event list", "--space payments-prod", "CreatedAt > '2026-09-09T12:00:00Z'"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("unit-event list args = %q, want them to contain %q", joined, want)
		}
	}
}

// A related entity is named by a sibling object or by flat <Kind>Slug/<Kind>ID
// fields. The row's own generic Slug, Name and ID are its own identity and
// must never be read as the related entity's.
func TestConfigHubRelatedRef(t *testing.T) {
	tests := []struct {
		name             string
		item, row        map[string]interface{}
		wantSlug, wantID string
	}{
		{
			name:     "sibling object",
			item:     map[string]interface{}{"Space": map[string]interface{}{"Slug": "payments-prod", "SpaceID": "sp-1"}},
			row:      map[string]interface{}{},
			wantSlug: "payments-prod", wantID: "sp-1",
		},
		{
			name:     "flat fields on the row",
			item:     map[string]interface{}{},
			row:      map[string]interface{}{"SpaceSlug": "payments-prod", "SpaceID": "sp-1"},
			wantSlug: "payments-prod", wantID: "sp-1",
		},
		{
			name:     "sibling object wins over flat fields",
			item:     map[string]interface{}{"Space": map[string]interface{}{"Slug": "from-sibling", "SpaceID": "sp-sibling"}},
			row:      map[string]interface{}{"SpaceSlug": "from-row", "SpaceID": "sp-row"},
			wantSlug: "from-sibling", wantID: "sp-sibling",
		},
		{
			name: "the row's own slug and ID are not the related entity's",
			item: map[string]interface{}{},
			row:  map[string]interface{}{"Slug": "row-own-slug", "Name": "row-own-name", "ID": "row-own-id"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slug, id := configHubRelatedRef(tt.item, tt.row, "Space")
			if slug != tt.wantSlug || id != tt.wantID {
				t.Fatalf("configHubRelatedRef = %q/%q, want %q/%q", slug, id, tt.wantSlug, tt.wantID)
			}
		})
	}
}

// A server older than the Release-to-Target association reports no TargetID.
// The join key stays empty so trace/explain report an omission; it is never
// filled from another field.
func TestBuildConfigHubReleaseEvidence_MissingTargetIDStaysEmpty(t *testing.T) {
	raw := `[{"Release":{"ReleaseID":"r-1","SpaceID":"sp-1","SpaceSlug":"payments-prod","Digest":"sha256:abc","CreatedAt":"2026-09-03T15:02:09Z"}}]`

	releases, omissions := buildConfigHubReleaseEvidence(raw, 10)
	if len(omissions) != 0 || len(releases) != 1 {
		t.Fatalf("releases = %+v omissions = %+v, want one release and no omissions", releases, omissions)
	}
	if releases[0].TargetID != "" || releases[0].Target != "" {
		t.Fatalf("target identity = %q/%q, want empty when the server reports none", releases[0].Target, releases[0].TargetID)
	}
	if releases[0].SpaceID != "sp-1" {
		t.Fatalf("space id = %q, want sp-1 read from the Release", releases[0].SpaceID)
	}
}

func TestCollectGitOpsDeliveryEvidence_DisconnectedIsAnOmission(t *testing.T) {
	oldRequire := requireGitOpsConfigHubFn
	oldRun := runGitOpsCubCommand
	t.Cleanup(func() {
		requireGitOpsConfigHubFn = oldRequire
		runGitOpsCubCommand = oldRun
	})

	requireGitOpsConfigHubFn = func() error { return errors.New("not connected") }
	runGitOpsCubCommand = func(ctx context.Context, args []string) (string, error) {
		t.Fatal("cub should not be called when connection check fails")
		return "", nil
	}

	evidence := collectGitOpsDeliveryEvidence(context.Background(), newGitOpsDeliveryFakeClient(), gitOpsDeliveryEvidenceOptions{
		Space:      "prod",
		Since:      "24h",
		Window:     24 * time.Hour,
		StaleAfter: 15 * time.Minute,
		Now:        time.Now().UTC(),
		MaxItems:   10,
	})
	if len(evidence.Omissions) == 0 {
		t.Fatal("expected disconnected omission")
	}
	if evidence.Omissions[len(evidence.Omissions)-1].Layer != "confighub" {
		t.Fatalf("last omission = %+v, want confighub layer", evidence.Omissions[len(evidence.Omissions)-1])
	}
}

func argobotDeployment(name, namespace string, replicas, ready int64, labelKind string) *unstructured.Unstructured {
	labels := map[string]interface{}{}
	switch labelKind {
	case "app.kubernetes.io/name":
		labels["app.kubernetes.io/name"] = "argobot"
	default:
		labels["app"] = "argobot"
	}
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels":    labels,
			},
			"spec": map[string]interface{}{
				"replicas": replicas,
			},
			"status": map[string]interface{}{
				"readyReplicas":     ready,
				"availableReplicas": ready,
			},
		},
	}
}

func newGitOpsDeliveryFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	listKinds := map[schema.GroupVersionResource]string{
		{Group: "apps", Version: "v1", Resource: "deployments"}: "DeploymentList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)
}

// The example under examples/ is the documented shape of this evidence. Decode
// it through the real types, refusing unknown fields, so it cannot drift from
// the code, and check it shows both publication states the way output names them.
func TestDeliveryEvidenceExampleMatchesTheTypes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "live-delivery-observability", "confighub-delivery-evidence.json"))
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	var envelope struct {
		DeliveryEvidence json.RawMessage `json:"deliveryEvidence"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("parse example: %v", err)
	}
	var evidence GitOpsDeliveryEvidence
	decoder := json.NewDecoder(bytes.NewReader(envelope.DeliveryEvidence))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		t.Fatalf("example deliveryEvidence does not match GitOpsDeliveryEvidence: %v", err)
	}
	if evidence.ConfigHub == nil || len(evidence.ConfigHub.Releases) != evidence.ConfigHub.ReleasesTotal {
		t.Fatalf("example releasesTotal disagrees with its rows: %+v", evidence.ConfigHub)
	}
	actions := map[string]string{}
	for _, release := range evidence.ConfigHub.Releases {
		actions[configHubReleaseLabel(release)] = configHubReleaseAction(release)
	}
	want := map[string]string{"prod#42": "release-published", "prod#41": "release-not-published"}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("example release actions = %v, want %v", actions, want)
	}
	for _, event := range evidence.ConfigHub.UnitEvents {
		if event.Target != "" || event.TargetID != "" {
			t.Fatalf("example unit event names a target, which a ConfigHub UnitEvent does not have: %+v", event)
		}
	}
}
