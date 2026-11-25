// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func resetTraceRecordingTestFlags(t *testing.T) {
	t.Helper()
	origCtx, origRec, origNS, origFmt, origApp := traceKubeContext, traceRecording, traceNamespace, traceFormat, traceApp
	origRev, origDiff, origDesired, origAPIVer, origExp := traceReverse, traceDiff, traceDesiredFile, traceAPIVersion, traceExplain
	origHist, origLim, origArt, origPres := traceHistory, traceLimit, traceArtifacts, tracePresentation
	origWithCH, origCHSpace, origCHSince, origCHStale := traceWithConfigHub, traceConfigHubSpace, traceConfigHubSince, traceConfigHubStaleAfter
	t.Cleanup(func() {
		traceKubeContext, traceRecording, traceNamespace, traceFormat, traceApp = origCtx, origRec, origNS, origFmt, origApp
		traceReverse, traceDiff, traceDesiredFile, traceAPIVersion, traceExplain = origRev, origDiff, origDesired, origAPIVer, origExp
		traceHistory, traceLimit, traceArtifacts, tracePresentation = origHist, origLim, origArt, origPres
		traceWithConfigHub, traceConfigHubSpace, traceConfigHubSince, traceConfigHubStaleAfter = origWithCH, origCHSpace, origCHSince, origCHStale
	})
	traceKubeContext, traceRecording, traceNamespace, traceFormat, traceApp = "", "", "", "json", ""
	traceReverse, traceDiff, traceDesiredFile, traceAPIVersion, traceExplain = false, false, "", "", false
	traceHistory, traceLimit, traceArtifacts, tracePresentation = false, 10, false, ""
	traceWithConfigHub, traceConfigHubSpace, traceConfigHubSince, traceConfigHubStaleAfter = false, "", "24h", "15m"
}

func newTraceRecordingTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "trace"}
	cmd.SetContext(context.Background())
	cmd.Flags().StringVar(&traceKubeContext, "kube-context", "", "")
	cmd.Flags().StringVar(&traceRecording, "recording", "", "")
	cmd.Flags().StringVarP(&traceNamespace, "namespace", "n", "", "")
	cmd.Flags().StringVar(&traceFormat, "format", "ascii", "")
	cmd.Flags().StringVar(&traceApp, "app", "", "")
	cmd.Flags().BoolVarP(&traceReverse, "reverse", "r", false, "")
	cmd.Flags().BoolVarP(&traceDiff, "diff", "d", false, "")
	cmd.Flags().StringVar(&traceDesiredFile, "desired-file", "", "")
	cmd.Flags().StringVar(&traceAPIVersion, "api-version", "", "")
	cmd.Flags().BoolVar(&traceExplain, "explain", false, "")
	cmd.Flags().BoolVar(&traceHistory, "history", false, "")
	cmd.Flags().IntVar(&traceLimit, "limit", 10, "")
	cmd.Flags().BoolVar(&traceArtifacts, "artifacts", false, "")
	cmd.Flags().StringVar(&tracePresentation, "presentation", "", "")
	cmd.Flags().BoolVar(&traceWithConfigHub, "with-confighub", false, "")
	cmd.Flags().StringVar(&traceConfigHubSpace, "confighub-space", "", "")
	cmd.Flags().StringVar(&traceConfigHubSince, "confighub-since", "24h", "")
	cmd.Flags().StringVar(&traceConfigHubStaleAfter, "confighub-stale-after", "15m", "")
	return cmd
}

func writeTraceRecordingSnapshot(t *testing.T, format string, objs []*unstructured.Unstructured, observedAt time.Time, sourceCtx string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := encodeRecordedSnapshot(&buf, format, objs, observedAt, sourceCtx, false); err != nil {
		t.Fatalf("encodeRecordedSnapshot failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "trace-snapshot."+format)
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertValidOfflineTraceAnswer(t *testing.T, out mapsvc.TraceOutput, wantState, wantObservedAt, wantContext string) {
	t.Helper()
	if out.Answer == nil {
		t.Fatal("expected answer envelope in trace output")
	}
	if !slices.Contains(traceStates, out.Answer.State) {
		t.Fatalf("unapproved trace answer.state %q (allowed: %v)", out.Answer.State, traceStates)
	}
	if out.Answer.State != wantState {
		t.Fatalf("answer.state = %q, want %q (summary=%q)", out.Answer.State, wantState, out.Answer.Summary)
	}
	if wantObservedAt != "" && out.Answer.ObservedAt != wantObservedAt {
		t.Fatalf("answer.observedAt = %q, want %q", out.Answer.ObservedAt, wantObservedAt)
	}
	if wantContext != "" && out.Answer.Cluster.Context != wantContext {
		t.Fatalf("answer.cluster.context = %q, want %q", out.Answer.Cluster.Context, wantContext)
	}
	foundStale := false
	for _, u := range out.Answer.Unknowns {
		if u.Kind == mapsvc.UnknownStale && strings.Contains(u.Reason, "offline") {
			if wantObservedAt != "" && strings.Contains(u.Reason, wantObservedAt) {
				foundStale = true
				break
			}
			if wantObservedAt == "" && strings.Contains(u.Reason, "observedAt: unknown") {
				foundStale = true
				break
			}
		}
	}
	if !foundStale {
		t.Fatalf("expected kind=%q offline unknown with observedAt in answer.unknowns, got %+v", mapsvc.UnknownStale, out.Answer.Unknowns)
	}
}

func TestTraceRecording_RejectsLiveFlags(t *testing.T) {
	recPath := filepath.Join(t.TempDir(), "snap.yaml")
	if err := os.WriteFile(recPath, []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: prod\n"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{name: "kube-context", flags: map[string]string{"kube-context": "prod-ctx"}, wantErr: "--kube-context"},
		{name: "diff", flags: map[string]string{"diff": "true"}, wantErr: "--diff"},
		{name: "reverse", flags: map[string]string{"reverse": "true"}, wantErr: "--reverse"},
		{name: "history", flags: map[string]string{"history": "true"}, wantErr: "--history"},
		{name: "with-confighub", flags: map[string]string{"with-confighub": "true"}, wantErr: "--with-confighub"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetTraceRecordingTestFlags(t)
			cmd := newTraceRecordingTestCmd()
			_ = cmd.Flags().Set("recording", recPath)
			_ = cmd.Flags().Set("namespace", "prod")
			_ = cmd.Flags().Set("format", "json")
			for k, v := range tc.flags {
				_ = cmd.Flags().Set(k, v)
			}
			err := runTrace(cmd, []string{"deploy/api"})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestTraceRecording_FluxChains(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nonexistent-kubeconfig"))
	observedAt := time.Date(2026, 11, 24, 20, 0, 0, 0, time.UTC)

	t.Run("Flux Kustomization -> GitRepository ready", func(t *testing.T) {
		resetTraceRecordingTestFlags(t)
		objs := []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "source.toolkit.fluxcd.io/v1",
					"kind":       "GitRepository",
					"metadata":   map[string]interface{}{"name": "platform", "namespace": "flux-system"},
					"spec":       map[string]interface{}{"url": "https://github.com/example/platform"},
					"status": map[string]interface{}{
						"artifact": map[string]interface{}{
							"revision": "main@sha1:abc1234",
							"digest":   "sha256:11112222",
						},
						"conditions": []interface{}{
							map[string]interface{}{"type": "Ready", "status": "True", "reason": "Succeeded"},
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
					"kind":       "Kustomization",
					"metadata":   map[string]interface{}{"name": "apps", "namespace": "flux-system"},
					"spec": map[string]interface{}{
						"path":      "./clusters/prod",
						"sourceRef": map[string]interface{}{"kind": "GitRepository", "name": "platform"},
					},
					"status": map[string]interface{}{
						"lastAppliedRevision": "main@sha1:abc1234",
						"conditions": []interface{}{
							map[string]interface{}{"type": "Ready", "status": "True", "reason": "ReconciliationSucceeded"},
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "web",
						"namespace": "prod",
						"labels": map[string]interface{}{
							"kustomize.toolkit.fluxcd.io/name":      "apps",
							"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
						},
					},
				},
			},
		}
		recPath := writeTraceRecordingSnapshot(t, snapshotFormatYAML, objs, observedAt, "prod-west")

		cmd := newTraceRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("namespace", "prod")
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		rawOut := captureStdout(t, func() {
			runErr = runTrace(cmd, []string{"deploy/web"})
		})
		if runErr != nil {
			t.Fatalf("runTrace offline failed: %v", runErr)
		}

		var out mapsvc.TraceOutput
		if err := json.Unmarshal([]byte(rawOut), &out); err != nil {
			t.Fatalf("unmarshal trace JSON: %v\n%s", err, rawOut)
		}
		if len(out.Chain) != 3 {
			t.Fatalf("len(chain) = %d, want 3 (GitRepository -> Kustomization -> Deployment)", len(out.Chain))
		}
		if out.Chain[0].ID.Kind != "GitRepository" || out.Chain[1].ID.Kind != "Kustomization" || out.Chain[2].ID.Kind != "Deployment" {
			t.Fatalf("unexpected chain order: %+v", out.Chain)
		}
		assertValidOfflineTraceAnswer(t, out, traceStateTraced, "2026-11-24T20:00:00Z", "prod-west")
	})

	t.Run("Flux HelmRelease -> OCIRepository not ready", func(t *testing.T) {
		resetTraceRecordingTestFlags(t)
		objs := []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "source.toolkit.fluxcd.io/v1",
					"kind":       "OCIRepository",
					"metadata":   map[string]interface{}{"name": "charts", "namespace": "flux-system"},
					"spec":       map[string]interface{}{"url": "oci://ghcr.io/example/charts"},
					"status": map[string]interface{}{
						"artifact": map[string]interface{}{"revision": "1.2.3@sha256:deadbeef"},
						"conditions": []interface{}{
							map[string]interface{}{"type": "Ready", "status": "True", "reason": "Succeeded"},
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "helm.toolkit.fluxcd.io/v2",
					"kind":       "HelmRelease",
					"metadata":   map[string]interface{}{"name": "payments", "namespace": "prod"},
					"spec": map[string]interface{}{
						"chartRef": map[string]interface{}{"kind": "OCIRepository", "name": "charts", "namespace": "flux-system"},
					},
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{"type": "Ready", "status": "False", "reason": "UpgradeFailed", "message": "timed out waiting for condition"},
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "payments",
						"namespace": "prod",
						"labels": map[string]interface{}{
							"helm.toolkit.fluxcd.io/name":      "payments",
							"helm.toolkit.fluxcd.io/namespace": "prod",
						},
					},
				},
			},
		}
		recPath := writeTraceRecordingSnapshot(t, snapshotFormatJSON, objs, observedAt, "prod-east")

		cmd := newTraceRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("namespace", "prod")
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		rawOut := captureStdout(t, func() {
			runErr = runTrace(cmd, []string{"deployment/payments"})
		})
		if runErr != nil {
			t.Fatalf("runTrace offline failed: %v", runErr)
		}

		var out mapsvc.TraceOutput
		if err := json.Unmarshal([]byte(rawOut), &out); err != nil {
			t.Fatalf("unmarshal trace JSON: %v\n%s", err, rawOut)
		}
		if len(out.Chain) != 3 || out.Chain[0].ID.Kind != "OCIRepository" || out.Chain[1].ID.Kind != "HelmRelease" {
			t.Fatalf("unexpected chain: %+v", out.Chain)
		}
		assertValidOfflineTraceAnswer(t, out, traceStateChainNotReady, "2026-11-24T20:00:00Z", "prod-east")
	})
}

func TestTraceRecording_ArgoCD(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nonexistent-kubeconfig"))

	t.Run("Argo CD Synced/Healthy workload", func(t *testing.T) {
		resetTraceRecordingTestFlags(t)
		observedAt := time.Date(2026, 11, 24, 21, 0, 0, 0, time.UTC)
		objs := []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "argoproj.io/v1alpha1",
					"kind":       "Application",
					"metadata":   map[string]interface{}{"name": "checkout", "namespace": "argocd"},
					"spec": map[string]interface{}{
						"source": map[string]interface{}{
							"repoURL":        "https://github.com/example/checkout",
							"path":           "k8s/prod",
							"targetRevision": "main",
						},
					},
					"status": map[string]interface{}{
						"sync":   map[string]interface{}{"status": "Synced", "revision": "abc999"},
						"health": map[string]interface{}{"status": "Healthy"},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "checkout-api",
						"namespace": "prod",
						"labels": map[string]interface{}{
							"argocd.argoproj.io/instance": "checkout",
						},
					},
				},
			},
		}
		recPath := writeTraceRecordingSnapshot(t, snapshotFormatYAML, objs, observedAt, "argo-prod")

		cmd := newTraceRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("namespace", "prod")
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		rawOut := captureStdout(t, func() {
			runErr = runTrace(cmd, []string{"deploy/checkout-api"})
		})
		if runErr != nil {
			t.Fatalf("runTrace offline failed: %v", runErr)
		}

		var out mapsvc.TraceOutput
		if err := json.Unmarshal([]byte(rawOut), &out); err != nil {
			t.Fatalf("unmarshal trace JSON: %v\n%s", err, rawOut)
		}
		if len(out.Chain) != 3 || out.Chain[1].ID.Kind != "Application" || out.Chain[1].ID.Name != "checkout" {
			t.Fatalf("unexpected Argo chain: %+v", out.Chain)
		}
		assertValidOfflineTraceAnswer(t, out, traceStateTraced, "2026-11-24T21:00:00Z", "argo-prod")
	})

	t.Run("Argo CD Degraded without observedAt", func(t *testing.T) {
		resetTraceRecordingTestFlags(t)
		raw := `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: billing
  namespace: argocd
spec:
  source:
    repoURL: https://github.com/example/billing
    path: deploy
status:
  sync:
    status: OutOfSync
  health:
    status: Degraded
    message: Deployment billing has 0/2 ready
`
		recPath := filepath.Join(t.TempDir(), "argo-degraded.yaml")
		if err := os.WriteFile(recPath, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}

		cmd := newTraceRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("app", "billing")
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		rawOut := captureStdout(t, func() {
			runErr = runTrace(cmd, nil)
		})
		if runErr != nil {
			t.Fatalf("runTrace --app offline failed: %v", runErr)
		}

		var out mapsvc.TraceOutput
		if err := json.Unmarshal([]byte(rawOut), &out); err != nil {
			t.Fatalf("unmarshal trace JSON: %v\n%s", err, rawOut)
		}
		assertValidOfflineTraceAnswer(t, out, traceStateChainNotReady, "", "")
	})
}

func TestTraceRecording_HelmReleaseSecret(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nonexistent-kubeconfig"))
	observedAt := time.Date(2026, 11, 24, 22, 0, 0, 0, time.UTC)

	t.Run("Helm deployed selects highest revision Secret", func(t *testing.T) {
		resetTraceRecordingTestFlags(t)
		objs := []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "redis",
						"namespace": "cache",
						"labels": map[string]interface{}{
							"app.kubernetes.io/managed-by": "Helm",
							"helm.sh/chart":                "redis-19.0.0",
						},
						"annotations": map[string]interface{}{
							"meta.helm.sh/release-name":      "redis",
							"meta.helm.sh/release-namespace": "cache",
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"type":       "helm.sh/release.v1",
					"metadata": map[string]interface{}{
						"name":      "sh.helm.release.v1.redis.v1",
						"namespace": "cache",
						"labels":    map[string]interface{}{"owner": "helm", "name": "redis", "status": "superseded", "version": "1"},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"type":       "helm.sh/release.v1",
					"metadata": map[string]interface{}{
						"name":      "sh.helm.release.v1.redis.v2",
						"namespace": "cache",
						"labels":    map[string]interface{}{"owner": "helm", "name": "redis", "status": "deployed", "version": "2"},
					},
				},
			},
		}
		recPath := writeTraceRecordingSnapshot(t, snapshotFormatYAML, objs, observedAt, "helm-prod")

		cmd := newTraceRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("namespace", "cache")
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		rawOut := captureStdout(t, func() {
			runErr = runTrace(cmd, []string{"deploy/redis"})
		})
		if runErr != nil {
			t.Fatalf("runTrace offline failed: %v", runErr)
		}

		var out mapsvc.TraceOutput
		if err := json.Unmarshal([]byte(rawOut), &out); err != nil {
			t.Fatalf("unmarshal trace JSON: %v\n%s", err, rawOut)
		}
		if len(out.Chain) != 3 || out.Chain[0].ID.Kind != "HelmChart" || out.Chain[1].ID.Kind != "Release" || out.Chain[2].ID.Kind != "Deployment" {
			t.Fatalf("unexpected Helm chain: %+v", out.Chain)
		}
		assertValidOfflineTraceAnswer(t, out, traceStateTraced, "2026-11-24T22:00:00Z", "helm-prod")
	})

	t.Run("Helm failed release reports chain-not-ready", func(t *testing.T) {
		resetTraceRecordingTestFlags(t)
		objs := []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "redis",
						"namespace": "cache",
						"labels":    map[string]interface{}{"app.kubernetes.io/managed-by": "Helm"},
						"annotations": map[string]interface{}{
							"meta.helm.sh/release-name":      "redis",
							"meta.helm.sh/release-namespace": "cache",
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Secret",
					"type":       "helm.sh/release.v1",
					"metadata": map[string]interface{}{
						"name":      "sh.helm.release.v1.redis.v3",
						"namespace": "cache",
						"labels":    map[string]interface{}{"owner": "helm", "name": "redis", "status": "failed", "version": "3"},
					},
				},
			},
		}
		recPath := writeTraceRecordingSnapshot(t, snapshotFormatJSON, objs, observedAt, "helm-prod")

		cmd := newTraceRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("namespace", "cache")
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		rawOut := captureStdout(t, func() {
			runErr = runTrace(cmd, []string{"deploy/redis"})
		})
		if runErr != nil {
			t.Fatalf("runTrace offline failed: %v", runErr)
		}

		var out mapsvc.TraceOutput
		if err := json.Unmarshal([]byte(rawOut), &out); err != nil {
			t.Fatalf("unmarshal trace JSON: %v\n%s", err, rawOut)
		}
		assertValidOfflineTraceAnswer(t, out, traceStateChainNotReady, "2026-11-24T22:00:00Z", "helm-prod")
	})
}

func TestTraceRecording_KroCrossplaneAndUnmanaged(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nonexistent-kubeconfig"))
	observedAt := time.Date(2026, 11, 24, 23, 0, 0, 0, time.UTC)

	t.Run("kro instance chain", func(t *testing.T) {
		resetTraceRecordingTestFlags(t)
		objs := []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "kro.run/v1alpha1",
					"kind":       "WebApp",
					"metadata":   map[string]interface{}{"name": "frontend", "namespace": "apps"},
					"status": map[string]interface{}{
						"state": "ACTIVE",
						"conditions": []interface{}{
							map[string]interface{}{"type": "Ready", "status": "True"},
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "frontend",
						"namespace": "apps",
						"labels": map[string]interface{}{
							"kro.run/owned":              "true",
							"kro.run/instance-group":     "kro.run",
							"kro.run/instance-version":   "v1alpha1",
							"kro.run/instance-kind":      "WebApp",
							"kro.run/instance-name":      "frontend",
							"kro.run/instance-namespace": "apps",
						},
					},
				},
			},
		}
		recPath := writeTraceRecordingSnapshot(t, snapshotFormatYAML, objs, observedAt, "kro-cluster")

		cmd := newTraceRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("namespace", "apps")
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		rawOut := captureStdout(t, func() {
			runErr = runTrace(cmd, []string{"deploy/frontend"})
		})
		if runErr != nil {
			t.Fatalf("runTrace kro offline failed: %v", runErr)
		}

		var out mapsvc.TraceOutput
		if err := json.Unmarshal([]byte(rawOut), &out); err != nil {
			t.Fatalf("unmarshal trace JSON: %v\n%s", err, rawOut)
		}
		if len(out.Chain) != 2 || out.Chain[0].ID.Kind != "WebApp" || out.Chain[1].ID.Kind != "Deployment" {
			t.Fatalf("unexpected kro chain: %+v", out.Chain)
		}
		assertValidOfflineTraceAnswer(t, out, traceStateReadinessNotRead, "2026-11-24T23:00:00Z", "kro-cluster")
	})

	t.Run("Crossplane composite chain", func(t *testing.T) {
		resetTraceRecordingTestFlags(t)
		objs := []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "database.example.crossplane.io/v1alpha1",
					"kind":       "XPostgres",
					"metadata": map[string]interface{}{
						"name":      "orders-db-xr",
						"namespace": "",
						"labels":    map[string]interface{}{"crossplane.io/composite": "orders-db-xr"},
					},
					"status": map[string]interface{}{
						"conditions": []interface{}{
							map[string]interface{}{"type": "Synced", "status": "True"},
							map[string]interface{}{"type": "Ready", "status": "True"},
						},
					},
				},
			},
			{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "StatefulSet",
					"metadata": map[string]interface{}{
						"name":      "orders-db",
						"namespace": "data",
						"labels": map[string]interface{}{
							"crossplane.io/composite": "orders-db-xr",
						},
						"ownerReferences": []interface{}{
							map[string]interface{}{
								"apiVersion": "database.example.crossplane.io/v1alpha1",
								"kind":       "XPostgres",
								"name":       "orders-db-xr",
								"uid":        "xr-uid-1",
							},
						},
					},
				},
			},
		}
		recPath := writeTraceRecordingSnapshot(t, snapshotFormatJSON, objs, observedAt, "xp-cluster")

		cmd := newTraceRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("namespace", "data")
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		rawOut := captureStdout(t, func() {
			runErr = runTrace(cmd, []string{"statefulset/orders-db"})
		})
		if runErr != nil {
			t.Fatalf("runTrace crossplane offline failed: %v", runErr)
		}

		var out mapsvc.TraceOutput
		if err := json.Unmarshal([]byte(rawOut), &out); err != nil {
			t.Fatalf("unmarshal trace JSON: %v\n%s", err, rawOut)
		}
		if len(out.Chain) != 2 || out.Chain[0].ID.Kind != "XPostgres" || out.Chain[1].ID.Kind != "StatefulSet" {
			t.Fatalf("unexpected Crossplane chain: %+v", out.Chain)
		}
		assertValidOfflineTraceAnswer(t, out, traceStateTraced, "2026-11-24T23:00:00Z", "xp-cluster")
	})

	t.Run("unmanaged resource reports no-owner-marker", func(t *testing.T) {
		resetTraceRecordingTestFlags(t)
		objs := []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata":   map[string]interface{}{"name": "manual", "namespace": "default"},
				},
			},
		}
		recPath := writeTraceRecordingSnapshot(t, snapshotFormatYAML, objs, observedAt, "dev-cluster")

		cmd := newTraceRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("namespace", "default")
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		rawOut := captureStdout(t, func() {
			runErr = runTrace(cmd, []string{"deploy/manual"})
		})
		if runErr != nil {
			t.Fatalf("runTrace unmanaged offline failed: %v", runErr)
		}

		var out mapsvc.TraceOutput
		if err := json.Unmarshal([]byte(rawOut), &out); err != nil {
			t.Fatalf("unmarshal trace JSON: %v\n%s", err, rawOut)
		}
		assertValidOfflineTraceAnswer(t, out, traceStateNoOwnerMarker, "2026-11-24T23:00:00Z", "dev-cluster")
	})
}
