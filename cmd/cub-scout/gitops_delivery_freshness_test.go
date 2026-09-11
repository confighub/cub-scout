// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/client-go/dynamic"
)

type liveStatusFreshnessCase struct {
	Name       string               `json:"name"`
	ObservedAt string               `json:"observedAt"`
	Failed     bool                 `json:"failed"`
	Freshness  string               `json:"freshness"`
	Verdict    agent.ReceiptVerdict `json:"verdict"`
	Omission   string               `json:"omission"`
}

func liveStatusFreshnessCases(t *testing.T) (time.Time, time.Duration, []liveStatusFreshnessCase) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "live-delivery-observability", "freshness-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Now, StaleAfter string
		Cases           []liveStatusFreshnessCase
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	now, err := time.Parse(time.RFC3339, fixture.Now)
	if err != nil {
		t.Fatal(err)
	}
	threshold, err := time.ParseDuration(fixture.StaleAfter)
	if err != nil {
		t.Fatal(err)
	}
	return now, threshold, fixture.Cases
}

func liveStatusFreshnessInput(t *testing.T, tc liveStatusFreshnessCase) string {
	t.Helper()
	status := map[string]string{
		"source": "argobot", "app": "api", "revision": "sha256:abc",
		"syncStatus": "Synced", "healthStatus": "Healthy", "operationPhase": "Succeeded",
	}
	if tc.Failed {
		status["syncStatus"], status["healthStatus"], status["operationPhase"] = "OutOfSync", "Degraded", "Failed"
	}
	if tc.ObservedAt != "" {
		status["observedAt"] = tc.ObservedAt
	}
	annotation, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal([]interface{}{map[string]interface{}{
		"Slug": "team-a", "SpaceID": "space-a",
		"Annotations": map[string]string{configHubLiveStatusAnnotation: string(annotation)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLiveStatusFreshnessFixture(t *testing.T) {
	now, threshold, cases := liveStatusFreshnessCases(t)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			statuses, omissions := buildConfigHubLiveStatusEvidence(liveStatusFreshnessInput(t, tc), now, threshold)
			if len(statuses) != 1 {
				t.Fatalf("statuses = %+v", statuses)
			}
			got := statuses[0]
			if got.Freshness != tc.Freshness || got.DeliveryVerdict != tc.Verdict || got.ApplicationHealthVerdict != tc.Verdict {
				t.Errorf("got %s/%s/%s, want %s/%s/%s", got.Freshness, got.DeliveryVerdict, got.ApplicationHealthVerdict, tc.Freshness, tc.Verdict, tc.Verdict)
			}
			if got.ObservedAt != tc.ObservedAt || got.Revision != "sha256:abc" || got.App != "api" || got.SpaceID != "space-a" {
				t.Errorf("reported identity/timestamp/revision changed: %+v", got)
			}
			wantSync, wantHealth, wantPhase := "Synced", "Healthy", "Succeeded"
			if tc.Failed {
				wantSync, wantHealth, wantPhase = "OutOfSync", "Degraded", "Failed"
			}
			if got.SyncStatus != wantSync || got.HealthStatus != wantHealth || got.OperationPhase != wantPhase {
				t.Errorf("raw reported state lost: %+v", got)
			}
			if tc.Omission == "" {
				if len(omissions) != 0 {
					t.Errorf("unexpected omissions: %+v", omissions)
				}
			} else if len(omissions) != 1 || omissions[0].Layer != "confighub.liveStatus.freshness" || !strings.Contains(omissions[0].Reason, tc.Omission) || omissions[0].Impact == "" {
				t.Errorf("missing explainable freshness omission %q: %+v", tc.Omission, omissions)
			}
		})
	}
}

func TestLiveStatusFreshnessSurfaces(t *testing.T) {
	now, threshold, cases := liveStatusFreshnessCases(t)
	oldNow := gitopsNowFn
	gitopsNowFn = func() time.Time { return now }
	t.Cleanup(func() { gitopsNowFn = oldNow })
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			raw := liveStatusFreshnessInput(t, tc)
			statuses, omissions := buildConfigHubLiveStatusEvidence(raw, now, threshold)
			evidence := &GitOpsDeliveryEvidence{
				ObservedAt: now, Scope: GitOpsDeliveryEvidenceScope{Space: "team-a", Namespace: "prod"},
				ConfigHub: &ConfigHubDeliveryEvidence{LiveStatuses: statuses}, Omissions: omissions,
			}
			summary := buildDoctorDeliverySummary(evidence)
			wantCounts := DoctorVerdictCounts{}
			countDoctorVerdict(&wantCounts, tc.Verdict)
			if summary.LiveStatus.Delivery != wantCounts || summary.LiveStatus.ApplicationHealth != wantCounts {
				t.Errorf("doctor verdict counts: %+v", summary.LiveStatus)
			}
			if tc.Freshness != "fresh" {
				for _, issue := range buildDoctorDeliveryIssues(evidence) {
					if issue.Severity == "CRITICAL" {
						t.Errorf("old/undated report treated as current failure: %+v", issue)
					}
				}
				doctor := DoctorSummary{}
				attachDoctorDeliveryEvidence(&doctor, evidence, 3)
				if out := renderDoctorASCII(doctor, DefaultPresentationMode, false, DefaultHintContext()); !strings.Contains(out, "current delivery/application health unverified") {
					t.Errorf("doctor ASCII hides uncertainty: %s", out)
				}
			}
			row := configHubLiveStatusActivityRow(evidence, statuses[0])
			wantResult := mapActivityResultFromVerdicts(tc.Verdict, tc.Verdict)
			if row.Result != wantResult || row.DeliveryEvidence.DeliveryVerdict != string(tc.Verdict) {
				t.Errorf("activity result: %+v", row)
			}
			var rowJSON map[string]interface{}
			if err := marshalInto(row.DeliveryEvidence, &rowJSON); err != nil {
				t.Fatal(err)
			}
			if tc.ObservedAt != "" && rowJSON["observedAt"] != tc.ObservedAt {
				t.Errorf("activity lost original report timestamp: %+v", rowJSON)
			}
			if tc.ObservedAt == "" && rowJSON["observedAt"] != nil {
				t.Errorf("activity invented report timestamp: %+v", rowJSON)
			}
			joined := mapActivityLiveStatusJoinEvidence(evidence, statuses[0], "prod", nil)
			if joined.ObservedAt != tc.ObservedAt {
				t.Error("Application join lost the report timestamp")
			}
			if tc.Freshness != "fresh" && !strings.Contains(row.SuggestedNextStep, "current controller/workload") {
				t.Errorf("missing direct-current-evidence guidance: %s", row.SuggestedNextStep)
			}
			correlated := correlateTraceDeliveryEvidence(nil, evidence, agent.TraceDeliveryCorrelation{Space: "team-a", Application: "api"}, nil)
			if correlated.LiveStatus == nil || correlated.LiveStatus.DeliveryVerdict != tc.Verdict || correlated.LiveStatus.ObservedAt != tc.ObservedAt {
				t.Fatalf("trace correlation lost evidence: %+v", correlated)
			}
			if tc.Omission != "" && !containsTraceOmission(correlated.Omissions, "confighub.liveStatus.freshness") {
				t.Error("trace lost freshness omission")
			}
			if !strings.Contains(formatTraceDeliveryEvidenceLine(correlated), "delivery="+string(tc.Verdict)) {
				t.Error("human explain/trace summary lost verdict")
			}
			var markdown strings.Builder
			renderGitOpsDeliveryEvidenceMarkdown(&markdown, evidence)
			ascii := captureStdout(t, func() { outputGitOpsDeliveryEvidenceHuman(evidence) })
			for name, out := range map[string]string{"ascii": ascii, "md": markdown.String()} {
				if !strings.Contains(out, string(tc.Verdict)) || (tc.Omission != "" && !strings.Contains(out, tc.Omission)) {
					t.Errorf("%s lost verdict or freshness explanation: %s", name, out)
				}
			}
			calls := 0
			gateway := newMCPGatewayWithMode(func(context.Context, []string) (string, error) {
				t.Fatal("unexpected Kubernetes/standalone read")
				return "", nil
			}, func(_ context.Context, args []string) (string, error) {
				calls++
				if !reflect.DeepEqual(args, mcpConfigHubLiveStatusArgs("team-a")) {
					t.Fatalf("unexpected read: %v", args)
				}
				return raw, nil
			}, true)
			resp := gateway.handleRequest(context.Background(), mcpRequest{
				JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call",
				Params: json.RawMessage(`{"name":"confighub_live_status","arguments":{"space":"team-a"}}`),
			})
			var result struct {
				StructuredContent struct {
					LiveStatuses []ConfigHubLiveStatusEvidence    `json:"liveStatuses"`
					Omissions    []GitOpsDeliveryEvidenceOmission `json:"omissions"`
				} `json:"structuredContent"`
			}
			if resp == nil || resp.Error != nil {
				t.Fatalf("MCP response: %+v", resp)
			}
			if err := marshalInto(resp.Result, &result); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || len(result.StructuredContent.LiveStatuses) != 1 || result.StructuredContent.LiveStatuses[0].DeliveryVerdict != tc.Verdict {
				t.Errorf("MCP verdict/read count: %d %+v", calls, result)
			}
			if !reflect.DeepEqual(result.StructuredContent.Omissions, omissions) && !(len(result.StructuredContent.Omissions) == 0 && len(omissions) == 0) {
				t.Error("MCP omissions differ from CLI")
			}
		})
	}
}

func TestLiveStatusFreshnessCollectorAndReceipt(t *testing.T) {
	now, threshold, _ := liveStatusFreshnessCases(t)
	for _, tc := range []liveStatusFreshnessCase{
		{Name: "unknown", Verdict: agent.VerdictINCONCLUSIVE},
		{Name: "old-failure", ObservedAt: "2026-09-11T11:00:00Z", Failed: true, Verdict: agent.VerdictWATCH},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			raw := liveStatusFreshnessInput(t, tc)
			oldRequire, oldRun := requireGitOpsConfigHubFn, runGitOpsCubCommand
			t.Cleanup(func() { requireGitOpsConfigHubFn, runGitOpsCubCommand = oldRequire, oldRun })
			requireGitOpsConfigHubFn = func() error { return nil }
			calls := 0
			runGitOpsCubCommand = func(_ context.Context, args []string) (string, error) {
				calls++
				if reflect.DeepEqual(args, gitOpsConfigHubSpaceListArgs("team-a")) {
					return raw, nil
				}
				if len(args) > 1 && args[1] == "list" && (args[0] == "release" || args[0] == "unit-event") {
					return "[]", nil
				}
				t.Fatalf("unexpected command: %v", args)
				return "", nil
			}
			client := newGitOpsDeliveryFakeClient(argobotDeployment("argobot", "observer", 1, 1, "app"))
			evidence := collectGitOpsDeliveryEvidence(context.Background(), client, gitOpsDeliveryEvidenceOptions{
				Space: "team-a", Since: "24h", StaleAfter: threshold, Now: now,
			})
			if calls != 3 || len(client.Actions()) != 2 {
				t.Fatalf("unexpected read budget: cub=%d k8s=%v", calls, client.Actions())
			}
			for _, action := range client.Actions() {
				if action.GetVerb() != "list" || action.GetResource().Resource != "deployments" {
					t.Fatalf("unexpected Kubernetes access: %+v", action)
				}
			}
			if evidence.ConfigHub.LiveStatuses[0].DeliveryVerdict != tc.Verdict {
				t.Fatalf("collector verdict: %+v", evidence)
			}
			resetReceiptFlags(t)
			live := makeReceiptArgoLive()
			live.SetLabels(map[string]string{"argocd.argoproj.io/instance": "api"})
			live.SetAnnotations(map[string]string{"argocd.argoproj.io/tracking-id": "api:apps/Deployment:prod/api"})
			withFakeReceiptLoader(t, live)
			oldClient, oldNow := newReceiptDeliveryDynamicClientFn, gitopsNowFn
			t.Cleanup(func() { newReceiptDeliveryDynamicClientFn, gitopsNowFn = oldClient, oldNow })
			newReceiptDeliveryDynamicClientFn = func() (dynamic.Interface, error) { return client, nil }
			gitopsNowFn = func() time.Time { return now }
			baselineJSON := captureStdout(t, func() {
				rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod", "--format", "json"})
				if err := rootCmd.Execute(); err != nil {
					t.Fatal(err)
				}
			})
			var baseline agent.Statement
			if err := json.Unmarshal([]byte(baselineJSON), &baseline); err != nil {
				t.Fatal(err)
			}
			calls = 0
			client.ClearActions()
			out := captureStdout(t, func() {
				rootCmd.SetArgs([]string{"receipt", "verify", "deploy/api", "-n", "prod", "--with-confighub", "--confighub-space", "team-a", "--format", "json"})
				if err := rootCmd.Execute(); err != nil {
					t.Fatal(err)
				}
			})
			if calls != 3 || len(client.Actions()) != 2 {
				t.Fatalf("receipt read budget changed: cub=%d k8s=%v", calls, client.Actions())
			}
			var stmt agent.Statement
			if err := json.Unmarshal([]byte(out), &stmt); err != nil {
				t.Fatal(err)
			}
			if stmt.Predicate.Verdict != baseline.Predicate.Verdict {
				t.Error("supporting feedback changed the independent receipt predicate")
			}
			got := stmt.Predicate.Evidence.DeliveryEvidence
			if got == nil || got.LiveStatus == nil || got.LiveStatus.DeliveryVerdict != tc.Verdict || !containsTraceOmission(got.Omissions, "confighub.liveStatus.freshness") {
				t.Fatalf("receipt lost uncertainty: %+v", got)
			}
			if err := agent.VerifyStatementFingerprint(stmt); err != nil {
				t.Fatal(err)
			}
			if out := renderReceiptASCII(stmt); !strings.Contains(out, "delivery="+string(tc.Verdict)) {
				t.Fatalf("receipt ASCII lost verdict: %s", out)
			}
			got.LiveStatus.DeliveryVerdict = agent.VerdictPASS
			if err := agent.VerifyStatementFingerprint(stmt); err == nil {
				t.Fatal("tampering with the supporting verdict must invalidate fingerprint")
			}
		})
	}
}

func TestLiveStatusFreshnessEmptyStatusAndInvalidClock(t *testing.T) {
	now, threshold, _ := liveStatusFreshnessCases(t)
	fresh := liveStatusFreshnessCase{ObservedAt: now.Add(-5 * time.Minute).Format(time.RFC3339)}
	for _, freshness := range []string{"fresh", "stale", "unknown", ""} {
		status := ConfigHubLiveStatusEvidence{Freshness: freshness}
		if configHubDeliveryVerdict(status) != agent.VerdictINCONCLUSIVE || configHubApplicationHealthVerdict(status) != agent.VerdictINCONCLUSIVE {
			t.Errorf("empty status became conclusive for %q", freshness)
		}
	}
	for _, tc := range []struct {
		name      string
		now       time.Time
		threshold time.Duration
	}{
		{"missing-clock", time.Time{}, threshold}, {"invalid-threshold", now, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statuses, omissions := buildConfigHubLiveStatusEvidence(liveStatusFreshnessInput(t, fresh), tc.now, tc.threshold)
			if statuses[0].Freshness != "unknown" || statuses[0].DeliveryVerdict != agent.VerdictINCONCLUSIVE || len(omissions) != 1 {
				t.Fatalf("invalid time policy did not fail closed: %+v %+v", statuses, omissions)
			}
		})
	}
}

func TestLiveStatusFreshnessInputOrderAndScope(t *testing.T) {
	now, threshold, _ := liveStatusFreshnessCases(t)
	var fresh, stale []map[string]interface{}
	if err := json.Unmarshal([]byte(liveStatusFreshnessInput(t, liveStatusFreshnessCase{ObservedAt: now.Add(-5 * time.Minute).Format(time.RFC3339)})), &fresh); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(liveStatusFreshnessInput(t, liveStatusFreshnessCase{ObservedAt: "2026-09-11T11:00:00Z", Failed: true})), &stale); err != nil {
		t.Fatal(err)
	}
	stale[0]["Slug"], stale[0]["SpaceID"] = "team-b", "space-b"
	var baseline []ConfigHubLiveStatusEvidence
	for _, items := range [][]map[string]interface{}{{fresh[0], stale[0]}, {stale[0], fresh[0]}} {
		raw, err := json.Marshal(items)
		if err != nil {
			t.Fatal(err)
		}
		statuses, _ := buildConfigHubLiveStatusEvidence(string(raw), now, threshold)
		if baseline == nil {
			baseline = statuses
		} else if !reflect.DeepEqual(baseline, statuses) {
			t.Fatal("input order changes verdicts")
		}
		if statuses[0].SpaceID != "space-a" || statuses[0].DeliveryVerdict != agent.VerdictPASS || statuses[1].SpaceID != "space-b" || statuses[1].DeliveryVerdict != agent.VerdictWATCH {
			t.Fatalf("same-name apps crossed scopes: %+v", statuses)
		}
	}
}

func TestLiveStatusFreshnessUnchangedReportAges(t *testing.T) {
	now, threshold, _ := liveStatusFreshnessCases(t)
	raw := liveStatusFreshnessInput(t, liveStatusFreshnessCase{ObservedAt: now.Add(-5 * time.Minute).Format(time.RFC3339)})
	first, _ := buildConfigHubLiveStatusEvidence(raw, now, threshold)
	later, _ := buildConfigHubLiveStatusEvidence(raw, now.Add(time.Hour), threshold)
	if first[0].DeliveryVerdict != agent.VerdictPASS || later[0].DeliveryVerdict != agent.VerdictWATCH || first[0].ObservedAt != later[0].ObservedAt {
		t.Fatalf("reread renewed or lost the report: first=%+v later=%+v", first, later)
	}
}

func TestLiveStatusFreshnessAgeOverflow(t *testing.T) {
	now, _, _ := liveStatusFreshnessCases(t)
	raw := liveStatusFreshnessInput(t, liveStatusFreshnessCase{ObservedAt: "0002-01-01T00:00:00Z"})
	statuses, _ := buildConfigHubLiveStatusEvidence(raw, now, time.Duration(1<<63-1))
	if statuses[0].Freshness != "stale" || statuses[0].DeliveryVerdict != agent.VerdictWATCH {
		t.Fatalf("duration saturation made an ancient report fresh: %+v", statuses)
	}
}
