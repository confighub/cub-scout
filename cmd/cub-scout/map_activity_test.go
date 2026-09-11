// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestCollectEventActivityIncludesAuditedAction(t *testing.T) {
	oldNamespace, oldOwner := mapNamespace, mapOwner
	t.Cleanup(func() {
		mapNamespace, mapOwner = oldNamespace, oldOwner
	})
	mapNamespace, mapOwner = "", ""

	eventGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{eventGVR: "EventList"},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Event",
			"metadata": map[string]interface{}{
				"name":      "api-action",
				"namespace": "prod",
				"annotations": map[string]interface{}{
					"event.toolkit.fluxcd.io/action":       "restart",
					"event.toolkit.fluxcd.io/username":     "operator@example.com",
					"event.toolkit.fluxcd.io/subject":      "Deployment/prod/api",
					"event.toolkit.fluxcd.io/change-token": "chg-123",
				},
			},
			"involvedObject": map[string]interface{}{
				"kind":      "Deployment",
				"namespace": "prod",
				"name":      "api",
			},
			"type":      "Normal",
			"reason":    "WebAction",
			"message":   "operator@example.com requested restart for Deployment/prod/api",
			"eventTime": "2026-07-09T10:00:00Z",
		}},
	)

	rows := collectEventActivity(context.Background(), client)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Source != "k8s.action" {
		t.Fatalf("Source = %q, want k8s.action", row.Source)
	}
	if row.Action != "restart" {
		t.Fatalf("Action = %q, want restart", row.Action)
	}
	if row.Owner != "Flux" {
		t.Fatalf("Owner = %q, want Flux", row.Owner)
	}
	if row.Actor != "operator@example.com" || row.Subject != "Deployment/prod/api" {
		t.Fatalf("Actor/Subject = %q/%q, want operator@example.com/Deployment/prod/api", row.Actor, row.Subject)
	}
	if row.ActionEvidence["event.toolkit.fluxcd.io/change-token"] != "chg-123" {
		t.Fatalf("ActionEvidence = %#v, want change-token evidence", row.ActionEvidence)
	}
	if !strings.Contains(row.Message, "action=restart") || !strings.Contains(row.Message, "actor=operator@example.com") {
		t.Fatalf("Message = %q, want action detail", row.Message)
	}
}

func TestGitOpsDeliveryEvidenceToActivityRows(t *testing.T) {
	observedAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	evidence := &GitOpsDeliveryEvidence{
		ObservedAt: observedAt,
		Scope: GitOpsDeliveryEvidenceScope{
			Namespace:  "prod",
			Space:      "payments-prod",
			Since:      "24h",
			StaleAfter: "15m",
			MaxItems:   10,
		},
		ConfigHub: &ConfigHubDeliveryEvidence{
			LiveStatuses: []ConfigHubLiveStatusEvidence{{
				Space:                    "payments-prod",
				SpaceID:                  "sp-123",
				App:                      "payments-api",
				SyncStatus:               "OutOfSync",
				HealthStatus:             "Healthy",
				OperationPhase:           "Running",
				ObservedAt:               "2026-09-10T11:59:00Z",
				Freshness:                "fresh",
				DeliveryVerdict:          agent.VerdictWATCH,
				ApplicationHealthVerdict: agent.VerdictPASS,
			}},
			Releases: []ConfigHubReleaseEvidence{{
				Slug:           "release-42",
				ReleaseID:      "rel-42",
				Space:          "payments-prod",
				SpaceID:        "sp-123",
				Target:         "prod",
				TargetID:       "target-123",
				Digest:         "sha256:abcdef",
				BundleBaseName: "payments-api",
				RevisionNum:    7,
				CreatedAt:      "2026-09-10T11:57:00Z",
			}},
			UnitEvents: []ConfigHubUnitEventEvidence{{
				EventID:      "evt-1",
				Action:       "ReleasePublished",
				Result:       "Failed",
				Status:       "Failed",
				Message:      "delivery failed",
				Unit:         "payments-api",
				UnitID:       "unit-123",
				Space:        "payments-prod",
				SpaceID:      "sp-123",
				Target:       "prod",
				TargetID:     "target-123",
				CreatedAt:    "2026-09-10T11:56:00Z",
				TerminatedAt: "2026-09-10T11:56:30Z",
			}},
		},
		EventConsumers: []GitOpsEventConsumerEvidence{{
			Kind:              "Deployment",
			Name:              "argobot",
			Namespace:         "confighub-ops",
			Ready:             false,
			Replicas:          1,
			ReadyReplicas:     0,
			AvailableReplicas: 0,
			EvidenceLabel:     "app=argobot",
		}},
		Omissions: []GitOpsDeliveryEvidenceOmission{{
			Layer:  "confighub.unitEvents",
			Reason: "unit-event list returned no rows",
			Impact: "recent unit-level event evidence is unavailable",
		}},
	}

	rows := gitOpsDeliveryEvidenceToActivityRows(evidence)
	if len(rows) != 5 {
		t.Fatalf("len(rows) = %d, want 5: %+v", len(rows), rows)
	}

	bySource := map[string]mapActivityRow{}
	for _, row := range rows {
		bySource[row.Source] = row
		if row.Owner != "ConfigHub" {
			t.Fatalf("row %s owner = %q, want ConfigHub", row.Source, row.Owner)
		}
		if row.DeliveryEvidence == nil {
			t.Fatalf("row %s has no deliveryEvidence", row.Source)
		}
	}

	live := bySource["confighub.liveStatus"]
	if live.Result != "pending" || live.DeliveryEvidence.DeliveryVerdict != "WATCH" || live.DeliveryEvidence.ApplicationHealthVerdict != "PASS" {
		t.Fatalf("live row = %+v, want pending WATCH/PASS", live)
	}
	if live.DeliveryEvidence.Namespace != "prod" {
		t.Fatalf("live namespace = %q, want prod", live.DeliveryEvidence.Namespace)
	}

	release := bySource["confighub.release"]
	if release.Action != "release-published" || release.DeliveryEvidence.RevisionNum != 7 || release.DeliveryEvidence.Digest != "sha256:abcdef" {
		t.Fatalf("release row = %+v, want release details", release)
	}

	unitEvent := bySource["confighub.unitEvent"]
	if unitEvent.Result != "failed" || !strings.Contains(unitEvent.Message, "delivery failed") {
		t.Fatalf("unit event row = %+v, want failed message", unitEvent)
	}

	consumer := bySource["confighub.eventConsumer"]
	if consumer.Result != "failed" || consumer.DeliveryEvidence.Ready == nil || *consumer.DeliveryEvidence.Ready {
		t.Fatalf("consumer row = %+v, want explicit ready=false", consumer)
	}
	if consumer.DeliveryEvidence.Namespace != "prod" {
		t.Fatalf("consumer deliveryEvidence namespace = %q, want collection scope prod", consumer.DeliveryEvidence.Namespace)
	}

	omission := bySource["confighub.omission"]
	if omission.Result != "inconclusive" || omission.DeliveryEvidence.Layer != "confighub.unitEvents" {
		t.Fatalf("omission row = %+v, want unitEvents omission", omission)
	}
}

func TestMapActivityMatchesNamespaceKeepsConfigHubScopeExplicit(t *testing.T) {
	row := mapActivityRow{
		Source:   "confighub.release",
		Resource: "Release/prod/release-42",
		DeliveryEvidence: &mapActivityDeliveryEvidence{
			Namespace: "apps",
			Space:     "prod",
		},
	}

	if !mapActivityMatchesNamespace(row, "apps") {
		t.Fatal("expected ConfigHub activity row to match explicit namespace scope")
	}
	if mapActivityMatchesNamespace(row, "prod") {
		t.Fatal("space name must not satisfy namespace filter for ConfigHub rows")
	}
}

func TestAttachConfigHubDeliveryEvidenceToActivityRows_JoinsArgoApplicationByExactSpaceAndApp(t *testing.T) {
	evidence := &GitOpsDeliveryEvidence{
		ObservedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
		Scope: GitOpsDeliveryEvidenceScope{
			Space:      "payments-prod",
			StaleAfter: "15m",
			MaxItems:   10,
		},
		ConfigHub: &ConfigHubDeliveryEvidence{
			LiveStatuses: []ConfigHubLiveStatusEvidence{{
				Space:                    "payments-prod",
				SpaceID:                  "sp-123",
				App:                      "payments-api",
				SyncStatus:               "Synced",
				HealthStatus:             "Healthy",
				OperationPhase:           "Succeeded",
				Revision:                 "sha256:abc",
				Freshness:                "fresh",
				DeliveryVerdict:          agent.VerdictPASS,
				ApplicationHealthVerdict: agent.VerdictPASS,
			}},
		},
	}
	rows := []mapActivityRow{{
		Time:     "2026-09-10T11:59:00Z",
		Source:   "argocd.application",
		Resource: "Application/argocd/payments-api",
		Action:   "sync-status",
		Result:   "success",
		Message:  "sync=Synced health=Healthy",
		Owner:    "ArgoCD",
	}}

	got := attachConfigHubDeliveryEvidenceToActivityRows(rows, evidence)
	if got[0].DeliveryEvidence == nil {
		t.Fatalf("DeliveryEvidence nil, want exact live-status join")
	}
	de := got[0].DeliveryEvidence
	if de.Kind != "liveStatus" || de.Namespace != "argocd" || de.Space != "payments-prod" || de.App != "payments-api" {
		t.Fatalf("DeliveryEvidence = %+v, want liveStatus join for argocd/payments-api", de)
	}
	if de.DeliveryVerdict != "PASS" || de.ApplicationHealthVerdict != "PASS" {
		t.Fatalf("verdicts = %s/%s, want PASS/PASS", de.DeliveryVerdict, de.ApplicationHealthVerdict)
	}
	if strings.Join(de.MatchedBy, ",") != "scope.space,argocdApplication.name" {
		t.Fatalf("MatchedBy = %#v, want scope+application", de.MatchedBy)
	}
	if got[0].Result != "success" {
		t.Fatalf("Result = %q, want original Argo-owned result preserved", got[0].Result)
	}
	if !strings.Contains(got[0].Message, "confighub delivery=PASS app-health=PASS freshness=fresh") {
		t.Fatalf("Message = %q, want ConfigHub evidence suffix", got[0].Message)
	}
}

func TestAttachConfigHubDeliveryEvidenceToActivityRows_WildcardSpaceDoesNotJoin(t *testing.T) {
	evidence := &GitOpsDeliveryEvidence{
		Scope: GitOpsDeliveryEvidenceScope{Space: "*"},
		ConfigHub: &ConfigHubDeliveryEvidence{
			LiveStatuses: []ConfigHubLiveStatusEvidence{{
				Space:                    "payments-prod",
				App:                      "payments-api",
				DeliveryVerdict:          agent.VerdictPASS,
				ApplicationHealthVerdict: agent.VerdictPASS,
			}},
		},
	}
	rows := []mapActivityRow{{
		Source:   "argocd.application",
		Resource: "Application/argocd/payments-api",
		Result:   "success",
		Message:  "sync=Synced health=Healthy",
	}}

	got := attachConfigHubDeliveryEvidenceToActivityRows(rows, evidence)
	if got[0].DeliveryEvidence != nil {
		t.Fatalf("DeliveryEvidence = %+v, want nil for wildcard ConfigHub space", got[0].DeliveryEvidence)
	}
}

func TestAttachConfigHubDeliveryEvidenceToActivityRows_AppMismatchDoesNotJoin(t *testing.T) {
	evidence := &GitOpsDeliveryEvidence{
		Scope: GitOpsDeliveryEvidenceScope{Space: "payments-prod"},
		ConfigHub: &ConfigHubDeliveryEvidence{
			LiveStatuses: []ConfigHubLiveStatusEvidence{{
				Space:                    "payments-prod",
				App:                      "worker",
				DeliveryVerdict:          agent.VerdictPASS,
				ApplicationHealthVerdict: agent.VerdictPASS,
			}},
		},
	}
	rows := []mapActivityRow{{
		Source:   "argocd.application",
		Resource: "Application/argocd/payments-api",
		Result:   "success",
		Message:  "sync=Synced health=Healthy",
	}}

	got := attachConfigHubDeliveryEvidenceToActivityRows(rows, evidence)
	if got[0].DeliveryEvidence != nil {
		t.Fatalf("DeliveryEvidence = %+v, want nil for app mismatch", got[0].DeliveryEvidence)
	}
}

func TestMapActivityDeliveryOptionsRejectInvalidSince(t *testing.T) {
	oldNamespace := mapNamespace
	oldSpace := mapActivityConfigHubSpace
	oldSince := mapActivityConfigHubSince
	oldStaleAfter := mapActivityConfigHubStaleAfter
	oldNow := gitopsNowFn
	t.Cleanup(func() {
		mapNamespace = oldNamespace
		mapActivityConfigHubSpace = oldSpace
		mapActivityConfigHubSince = oldSince
		mapActivityConfigHubStaleAfter = oldStaleAfter
		gitopsNowFn = oldNow
	})

	mapNamespace = "prod"
	mapActivityConfigHubSpace = "payments-prod"
	mapActivityConfigHubSince = "not-a-window"
	mapActivityConfigHubStaleAfter = "15m"
	gitopsNowFn = func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) }

	_, err := mapActivityDeliveryOptionsFromFlags(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid --confighub-since") {
		t.Fatalf("err = %v, want invalid --confighub-since", err)
	}
}
