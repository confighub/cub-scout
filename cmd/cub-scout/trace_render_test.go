// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

var traceRenderANSI = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

type failingTraceWriter struct{ err error }

func (w failingTraceWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRenderTraceHumanWritesFullEvidenceAndHonorsOptions(t *testing.T) {
	result := &agent.TraceResult{
		Object:       agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "team-a"},
		Tool:         "flux",
		FullyManaged: false,
		Error:        "controller status is partial",
		Chain: []agent.ChainLink{
			{Kind: "GitRepository", Name: "platform", Namespace: "flux-system", URL: "https://example.invalid/platform.git", Revision: "main@sha1:abc", Ready: true},
			{Kind: "Deployment", Name: "api", Namespace: "team-a", Status: "Progressing", Message: "waiting for replicas", Ready: false},
		},
		History: []agent.HistoryEntry{
			{Timestamp: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), Revision: "revision-one", Status: "failed"},
			{Timestamp: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), Revision: "revision-two", Status: "deployed"},
		},
		Secrets: &agent.SecretEvidenceResult{
			Secrets: []agent.SecretEvidence{{Name: "registry-auth", RefType: agent.SecretRefTypeImagePullSecret, Status: agent.SecretStatusMissing, StatusReason: "secret does not exist"}},
			Summary: agent.SecretEvidenceSummary{Total: 1, Missing: 1},
		},
		Events: &agent.ResourceEventSummary{
			Events:       []agent.ResourceEvent{{Severity: "warning", Age: "3m", Reason: "BackOff", Message: "container restart back-off", Count: 2}},
			TotalCount:   1,
			WarningCount: 1,
		},
		DeliveryEvidence: &agent.TraceDeliveryEvidence{
			Correlation:    agent.TraceDeliveryCorrelation{UnitSlug: "api-unit", Space: "payments"},
			LiveStatus:     &agent.TraceDeliveryLiveStatus{App: "api", SyncStatus: "Synced", HealthStatus: "Healthy", Freshness: "stale"},
			Releases:       []agent.TraceDeliveryRelease{{Slug: "release-42", Target: "prod", Digest: "sha256:abc"}},
			UnitEvents:     []agent.TraceDeliveryUnitEvent{{Action: "apply", Unit: "api-unit", Result: "success", CreatedAt: "2026-09-01T12:00:00Z"}},
			EventConsumers: []agent.TraceDeliveryEventConsumer{{Kind: "Deployment", Name: "event-consumer", Namespace: "confighub", Ready: false, ReadyReplicas: 0, Replicas: 1}},
			Omissions:      []agent.TraceDeliveryOmission{{Layer: "confighub.releases", Reason: "release history is incomplete"}},
			Notes:          []string{"delivery evidence is supporting context"},
		},
	}
	artifacts := map[string]mapsvc.TraceArtifactRef{
		"GitRepository/flux-system/platform": {URL: "oci://registry.example/platform", Revision: "sha256:abc", Digest: "sha256:abc", LastUpdateTime: "2026-09-01T12:00:00Z"},
	}
	invCtx, err := NewInvocationContext("", TransportCLI)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := renderTraceHuman(&out, result, artifacts, invCtx, traceHumanOptions{Explain: true, Artifacts: true, History: true, Limit: 1}); err != nil {
		t.Fatalf("renderTraceHuman() error = %v", err)
	}
	text := traceRenderANSI.ReplaceAllString(out.String(), "")
	for _, want := range []string{
		"TRACE: Deployment/api in team-a",
		"[warning] controller status is partial",
		"GitRepository/platform",
		"Artifact URL: oci://registry.example/platform",
		"OWNERSHIP CHAIN EXPLAINED",
		"waiting for replicas",
		"registry-auth",
		"Recent events:",
		"container restart back-off",
		"ConfigHub delivery evidence:",
		"release-42",
		"api-unit",
		"event-consumer",
		"release history is incomplete",
		"revision-one",
		"and 1 more (use --limit to show more)",
		"NEXT STEPS:",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered trace missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "revision-two") {
		t.Fatalf("history limit was ignored:\n%s", text)
	}

	oldExplain, oldArtifacts, oldHistory, oldLimit := traceExplain, traceArtifacts, traceHistory, traceLimit
	traceExplain, traceArtifacts, traceHistory, traceLimit = true, true, true, 1
	t.Cleanup(func() {
		traceExplain, traceArtifacts, traceHistory, traceLimit = oldExplain, oldArtifacts, oldHistory, oldLimit
	})
	var cliErr error
	cliText := captureStdout(t, func() { cliErr = outputTraceHuman(result, artifacts, invCtx) })
	if cliErr != nil {
		t.Fatalf("outputTraceHuman() error = %v", cliErr)
	}
	var renderer bytes.Buffer
	if err := renderTraceHuman(&renderer, result, artifacts, invCtx, traceHumanOptions{Explain: true, Artifacts: true, History: true, Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if cliText != renderer.String() {
		t.Fatalf("CLI wrapper output differs from shared renderer:\nCLI:\n%s\nrenderer:\n%s", cliText, renderer.String())
	}
}

func TestRenderTraceHumanOptionsAreExplicitAndIndependent(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "team-a"},
		Tool:   "flux",
		Chain: []agent.ChainLink{
			{Kind: "GitRepository", Name: "platform", Ready: true},
			{Kind: "Deployment", Name: "api", Ready: true},
		},
		History: []agent.HistoryEntry{{Timestamp: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), Revision: "history-marker", Status: "deployed"}},
	}
	invCtx, err := NewInvocationContext("", TransportCLI)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := renderTraceHuman(&out, result, nil, invCtx, traceHumanOptions{}); err != nil {
		t.Fatal(err)
	}
	text := traceRenderANSI.ReplaceAllString(out.String(), "")
	if strings.Contains(text, "OWNERSHIP CHAIN EXPLAINED") || strings.Contains(text, "History:") || strings.Contains(text, "Artifact URL:") {
		t.Fatalf("disabled options leaked into output:\n%s", text)
	}
	if !strings.Contains(text, "GitRepository/platform") {
		t.Fatalf("base ownership chain was omitted:\n%s", text)
	}
}

func TestRenderTraceHumanNoChainReturnsErrorWithoutExiting(t *testing.T) {
	result := &agent.TraceResult{
		Object: agent.ResourceRef{Kind: "Deployment", Name: "api", Namespace: "team-a"},
		Error:  "resource is not managed by a detected GitOps tool",
	}
	invCtx, err := NewInvocationContext("", TransportCLI)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = renderTraceHuman(&out, result, nil, invCtx, traceHumanOptions{})
	if err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("renderTraceHuman() error = %v, want no-chain failure", err)
	}
	if !strings.Contains(out.String(), "[warning] resource is not managed") {
		t.Fatalf("warning was not written before returning error:\n%s", out.String())
	}
	if _, typed := err.(traceNoChainError); !typed {
		t.Fatalf("error type = %T, want traceNoChainError", err)
	}

}

func TestRenderTraceHumanRejectsNilResult(t *testing.T) {
	invCtx, err := NewInvocationContext("", TransportCLI)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderTraceHuman(&bytes.Buffer{}, nil, nil, invCtx, traceHumanOptions{}); err == nil || !strings.Contains(err.Error(), "nil") {
		t.Fatalf("renderTraceHuman(nil) error = %v, want nil-result error", err)
	}
}

func TestRenderTraceHumanReturnsWriterError(t *testing.T) {
	invCtx, err := NewInvocationContext("", TransportCLI)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("writer failed")
	result := &agent.TraceResult{Object: agent.ResourceRef{Kind: "Deployment", Name: "api"}}
	if err := renderTraceHuman(failingTraceWriter{err: wantErr}, result, nil, invCtx, traceHumanOptions{}); !errors.Is(err, wantErr) {
		t.Fatalf("renderTraceHuman() error = %v, want writer error", err)
	}
}
