// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
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
	if len(omissions) != 0 {
		t.Fatalf("omissions = %+v, want none", omissions)
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
	oldDefaultSpace := gitopsDefaultSpaceFn
	t.Cleanup(func() {
		requireGitOpsConfigHubFn = oldRequire
		runGitOpsCubCommand = oldRun
		gitopsDefaultSpaceFn = oldDefaultSpace
	})

	requireGitOpsConfigHubFn = func() error { return nil }
	gitopsDefaultSpaceFn = func(ctx context.Context) string { return "payments-prod" }

	var calls [][]string
	runGitOpsCubCommand = func(ctx context.Context, args []string) (string, error) {
		calls = append(calls, append([]string(nil), args...))
		switch {
		case reflect.DeepEqual(args, gitOpsConfigHubSpaceListArgs("payments-prod")):
			return `[{"Space":{"Slug":"payments-prod","SpaceID":"sp-123","Annotations":{"confighub.com/live-status":"{\"source\":\"argobot\",\"app\":\"payments-prod\",\"syncStatus\":\"Synced\",\"healthStatus\":\"Healthy\",\"operationPhase\":\"Succeeded\",\"revision\":\"sha256:abc\",\"observedAt\":\"2026-09-10T11:55:00Z\"}"}}}]`, nil
		case len(args) > 1 && args[0] == "release" && args[1] == "list":
			return `[{"Release":{"Slug":"rel-42","ReleaseID":"r-42","Digest":"sha256:abcdef","CreatedAt":"2026-09-10T11:50:00Z"},"Space":{"Slug":"payments-prod","SpaceID":"sp-123"},"Target":{"Slug":"prod","TargetID":"t-123"}}]`, nil
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
	if len(evidence.ConfigHub.Releases) != 1 || evidence.ConfigHub.Releases[0].Digest != "sha256:abcdef" {
		t.Fatalf("releases = %+v, want one parsed release", evidence.ConfigHub.Releases)
	}
	if len(evidence.ConfigHub.UnitEvents) != 1 || evidence.ConfigHub.UnitEvents[0].Action != "ReleasePublished" {
		t.Fatalf("unit events = %+v, want one parsed event", evidence.ConfigHub.UnitEvents)
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
