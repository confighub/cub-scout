// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestReceiptVerifyWithConfigHub_AttachesDeliveryEvidence(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	observedAt := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	prev := collectReceiptDeliveryEvidenceFn
	collectReceiptDeliveryEvidenceFn = func(_ context.Context, live *unstructured.Unstructured, owner agent.Ownership, flags traceConfigHubDeliveryFlags) (*agent.TraceDeliveryEvidence, error) {
		if live.GetName() != "api" {
			t.Fatalf("live name = %q, want api", live.GetName())
		}
		if owner.Type != agent.OwnerArgo || owner.Name != "payments-api" {
			t.Fatalf("owner = %+v, want Argo payments-api", owner)
		}
		if flags.Space != "payments-prod" || flags.Since != "2h" || flags.StaleAfter != "5m" {
			t.Fatalf("delivery flags = %+v, want payments-prod/2h/5m", flags)
		}
		return &agent.TraceDeliveryEvidence{
			Source:     "confighub",
			ObservedAt: observedAt,
			Scope: agent.TraceDeliveryEvidenceScope{
				Namespace:  "prod",
				Space:      "payments-prod",
				Since:      "2h",
				StaleAfter: "5m",
				MaxItems:   10,
			},
			Correlation: agent.TraceDeliveryCorrelation{
				Application: "payments-api",
				Space:       "payments-prod",
				MatchedBy:   []string{"scope.space", "chain.application"},
			},
			LiveStatus: &agent.TraceDeliveryLiveStatus{
				Space:                    "payments-prod",
				Source:                   "argobot",
				App:                      "payments-api",
				SyncStatus:               "Synced",
				HealthStatus:             "Healthy",
				OperationPhase:           "Succeeded",
				Freshness:                "fresh",
				DeliveryVerdict:          agent.VerdictPASS,
				ApplicationHealthVerdict: agent.VerdictPASS,
				MatchedBy:                []string{"scope.space", "liveStatus.app==chain.application"},
			},
			EventConsumers: []agent.TraceDeliveryEventConsumer{{
				Kind:          "Deployment",
				Name:          "argobot",
				Namespace:     "confighub-ops",
				Ready:         true,
				Replicas:      1,
				ReadyReplicas: 1,
				EvidenceLabel: "app=argobot",
			}},
		}, nil
	}
	t.Cleanup(func() { collectReceiptDeliveryEvidenceFn = prev })

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"receipt", "verify", "deploy/api",
			"-n", "prod",
			"--with-confighub",
			"--confighub-space", "payments-prod",
			"--confighub-since", "2h",
			"--confighub-stale-after", "5m",
			"--format", "json",
		})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify --with-confighub returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal receipt: %v\nraw:\n%s", err, out)
	}
	if stmt.Predicate.Evidence.DeliveryEvidence == nil {
		t.Fatalf("delivery evidence missing from receipt: %+v", stmt.Predicate.Evidence)
	}
	if got := stmt.Predicate.Evidence.DeliveryEvidence.LiveStatus.SyncStatus; got != "Synced" {
		t.Fatalf("delivery liveStatus.syncStatus = %q, want Synced", got)
	}
	if err := agent.VerifyStatementFingerprint(stmt); err != nil {
		t.Fatalf("receipt fingerprint must cover delivery evidence: %v", err)
	}
}

func TestReceiptVerifyWithConfigHub_ASCIIIncludesDeliverySummary(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	prev := collectReceiptDeliveryEvidenceFn
	collectReceiptDeliveryEvidenceFn = func(_ context.Context, _ *unstructured.Unstructured, _ agent.Ownership, _ traceConfigHubDeliveryFlags) (*agent.TraceDeliveryEvidence, error) {
		return &agent.TraceDeliveryEvidence{
			Source:     "confighub",
			ObservedAt: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
			LiveStatus: &agent.TraceDeliveryLiveStatus{
				App:                      "payments-api",
				SyncStatus:               "Synced",
				HealthStatus:             "Healthy",
				OperationPhase:           "Succeeded",
				Freshness:                "fresh",
				DeliveryVerdict:          agent.VerdictPASS,
				ApplicationHealthVerdict: agent.VerdictPASS,
			},
		}, nil
	}
	t.Cleanup(func() { collectReceiptDeliveryEvidenceFn = prev })

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod", "--with-confighub"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify --with-confighub returned error: %v", err)
		}
	})

	for _, want := range []string{
		"Evidence (delivery)",
		"summary:     ConfigHub delivery=PASS sync=Synced appHealth=PASS health=Healthy freshness=fresh",
		"liveStatus:  app=payments-api sync=Synced health=Healthy op=Succeeded freshness=fresh",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("ASCII output missing %q:\n%s", want, out)
		}
	}
}

func TestReceiptVerifyWithConfigHubRejectsInvalidWindow(t *testing.T) {
	resetReceiptFlags(t)
	withFakeReceiptLoader(t, makeReceiptArgoLive())

	prev := collectReceiptDeliveryEvidenceFn
	called := false
	collectReceiptDeliveryEvidenceFn = func(context.Context, *unstructured.Unstructured, agent.Ownership, traceConfigHubDeliveryFlags) (*agent.TraceDeliveryEvidence, error) {
		called = true
		return nil, nil
	}
	t.Cleanup(func() { collectReceiptDeliveryEvidenceFn = prev })

	rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod", "--with-confighub", "--confighub-since", "nope"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid --confighub-since") {
		t.Fatalf("err = %v, want invalid --confighub-since", err)
	}
	if called {
		t.Fatal("delivery evidence collector must not run after invalid --confighub-since")
	}
}

func TestReceiptVerifyWithConfigHubRejectsObjectSetMode(t *testing.T) {
	resetReceiptFlags(t)
	manifest := writeTempReceiptDeliveryFile(t, "deploy.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
      - name: api
        image: example/api:v1
`)

	rootCmd.SetArgs([]string{"receipt", "verify", "--file", manifest, "--scope", "namespace/prod", "--with-confighub"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "single-resource receipt verify") {
		t.Fatalf("err = %v, want single-resource limitation", err)
	}
}

func TestReceiptTraceResultFromLiveUsesExactLabels(t *testing.T) {
	obj := makeReceiptArgoLive()
	obj.SetLabels(map[string]string{
		"argocd.argoproj.io/instance": "payments-app",
		"confighub.com/UnitSlug":      "payments-api",
		"confighub.com/SpaceName":     "payments-prod",
	})
	obj.SetAnnotations(map[string]string{
		"confighub.com/UnitID":   "u-123",
		"confighub.com/SpaceID":  "sp-123",
		"confighub.com/TargetID": "t-123",
	})

	result := receiptTraceResultFromLive(obj, agent.Ownership{Type: agent.OwnerArgo, Name: "payments-app"})
	correlation := buildTraceDeliveryCorrelation(result)
	if correlation.UnitSlug != "payments-api" || correlation.Space != "payments-prod" || correlation.TargetID != "t-123" {
		t.Fatalf("correlation = %+v, want ConfigHub unit/space/target IDs", correlation)
	}
	if correlation.Application != "payments-app" {
		t.Fatalf("application = %q, want payments-app", correlation.Application)
	}
}

func writeTempReceiptDeliveryFile(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
