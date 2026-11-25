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

func resetGitOpsStatusTestFlags(t *testing.T) {
	t.Helper()
	origNS, origCtx, origRec, origFmt, origView, origTUI := gitopsNamespace, gitopsKubeContext, gitopsRecording, gitopsFormat, gitopsView, gitopsTUI
	origWithCH, origCHSpace, origCHSince, origCHStale := gitopsWithConfigHub, gitopsConfigHubSpace, gitopsConfigHubSince, gitopsConfigHubStaleAfter
	t.Cleanup(func() {
		gitopsNamespace, gitopsKubeContext, gitopsRecording, gitopsFormat, gitopsView, gitopsTUI = origNS, origCtx, origRec, origFmt, origView, origTUI
		gitopsWithConfigHub, gitopsConfigHubSpace, gitopsConfigHubSince, gitopsConfigHubStaleAfter = origWithCH, origCHSpace, origCHSince, origCHStale
	})
	gitopsNamespace, gitopsKubeContext, gitopsRecording, gitopsFormat, gitopsView, gitopsTUI = "", "", "", "json", gitopsStatusViewAll, false
	gitopsWithConfigHub, gitopsConfigHubSpace, gitopsConfigHubSince, gitopsConfigHubStaleAfter = false, "", "24h", "15m"
}

func resetDoctorTestFlags(t *testing.T) {
	t.Helper()
	origFmt, origView, origNS, origTop, origPres, origHint := doctorFormat, doctorView, doctorNamespace, doctorTopIssues, doctorPresentation, doctorHintMode
	origWithCH, origCHSpace, origCHSince, origCHStale := doctorWithConfigHub, doctorConfigHubSpace, doctorConfigHubSince, doctorConfigHubStaleAfter
	origCtx, origRec, origAttach := doctorKubeContext, doctorRecording, doctorBeforeAttach
	t.Cleanup(func() {
		doctorFormat, doctorView, doctorNamespace, doctorTopIssues, doctorPresentation, doctorHintMode = origFmt, origView, origNS, origTop, origPres, origHint
		doctorWithConfigHub, doctorConfigHubSpace, doctorConfigHubSince, doctorConfigHubStaleAfter = origWithCH, origCHSpace, origCHSince, origCHStale
		doctorKubeContext, doctorRecording, doctorBeforeAttach = origCtx, origRec, origAttach
	})
	doctorFormat, doctorView, doctorNamespace, doctorTopIssues, doctorPresentation, doctorHintMode = "json", doctorViewAll, "", 3, "", ""
	doctorWithConfigHub, doctorConfigHubSpace, doctorConfigHubSince, doctorConfigHubStaleAfter = false, "", "24h", "15m"
	doctorKubeContext, doctorRecording, doctorBeforeAttach = "", "", false
}

func newGitOpsStatusRecordingTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "status"}
	cmd.SetContext(context.Background())
	cmd.Flags().StringVarP(&gitopsNamespace, "namespace", "n", "", "")
	cmd.Flags().StringVar(&gitopsKubeContext, "kube-context", "", "")
	cmd.Flags().StringVar(&gitopsRecording, "recording", "", "")
	cmd.Flags().StringVar(&gitopsFormat, "format", "ascii", "")
	cmd.Flags().StringVar(&gitopsView, "view", gitopsStatusViewAll, "")
	cmd.Flags().BoolVar(&gitopsTUI, "tui", false, "")
	cmd.Flags().BoolVar(&gitopsWithConfigHub, "with-confighub", false, "")
	cmd.Flags().StringVar(&gitopsConfigHubSpace, "confighub-space", "", "")
	cmd.Flags().StringVar(&gitopsConfigHubSince, "confighub-since", "24h", "")
	cmd.Flags().StringVar(&gitopsConfigHubStaleAfter, "confighub-stale-after", "15m", "")
	return cmd
}

func newDoctorRecordingTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "doctor"}
	cmd.SetContext(context.Background())
	cmd.Flags().StringVarP(&doctorNamespace, "namespace", "n", "", "")
	cmd.Flags().StringVar(&doctorFormat, "format", "ascii", "")
	cmd.Flags().StringVar(&doctorView, "view", doctorViewAll, "")
	cmd.Flags().IntVar(&doctorTopIssues, "top", 3, "")
	cmd.Flags().StringVar(&doctorPresentation, "presentation", "", "")
	cmd.Flags().StringVar(&doctorHintMode, "hint-mode", "", "")
	cmd.Flags().BoolVar(&doctorWithConfigHub, "with-confighub", false, "")
	cmd.Flags().StringVar(&doctorConfigHubSpace, "confighub-space", "", "")
	cmd.Flags().StringVar(&doctorConfigHubSince, "confighub-since", "24h", "")
	cmd.Flags().StringVar(&doctorConfigHubStaleAfter, "confighub-stale-after", "15m", "")
	cmd.Flags().BoolVar(&doctorBeforeAttach, "before-attach", false, "")
	cmd.Flags().StringVar(&doctorKubeContext, "kube-context", "", "")
	cmd.Flags().StringVar(&doctorRecording, "recording", "", "")
	return cmd
}

func TestGitOpsStatusRecording_RejectsLiveAndConfigHubFlags(t *testing.T) {
	recPath := filepath.Join(t.TempDir(), "snap.yaml")
	if err := os.WriteFile(recPath, []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: prod\n"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Run("rejects kube-context", func(t *testing.T) {
		resetGitOpsStatusTestFlags(t)
		cmd := newGitOpsStatusRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("kube-context", "prod-ctx")
		_ = cmd.Flags().Set("format", "json")
		err := runGitOpsStatus(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "--kube-context") {
			t.Fatalf("expected --kube-context rejection, got %v", err)
		}
	})

	t.Run("rejects with-confighub", func(t *testing.T) {
		resetGitOpsStatusTestFlags(t)
		cmd := newGitOpsStatusRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("with-confighub", "true")
		_ = cmd.Flags().Set("format", "json")
		err := runGitOpsStatus(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "--with-confighub") {
			t.Fatalf("expected --with-confighub rejection, got %v", err)
		}
	})
}

func TestGitOpsStatusRecording_OfflineEvaluation(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nonexistent-kubeconfig"))

	t.Run("settled Flux snapshot with observedAt", func(t *testing.T) {
		resetGitOpsStatusTestFlags(t)
		observedAt := time.Date(2026, 11, 24, 20, 0, 0, 0, time.UTC)
		objs := []*unstructured.Unstructured{
			{
				Object: map[string]interface{}{
					"apiVersion": "source.toolkit.fluxcd.io/v1",
					"kind":       "GitRepository",
					"metadata":   map[string]interface{}{"name": "platform", "namespace": "flux-system"},
					"spec":       map[string]interface{}{"url": "https://github.com/example/platform"},
					"status": map[string]interface{}{
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
		}

		var buf bytes.Buffer
		if err := encodeRecordedSnapshot(&buf, snapshotFormatYAML, objs, observedAt, "prod-west", false); err != nil {
			t.Fatal(err)
		}
		recPath := filepath.Join(t.TempDir(), "flux-settled.yaml")
		if err := os.WriteFile(recPath, buf.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}

		cmd := newGitOpsStatusRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		out := captureStdout(t, func() {
			runErr = runGitOpsStatus(cmd, nil)
		})
		if runErr != nil {
			t.Fatalf("runGitOpsStatus offline failed: %v", runErr)
		}

		var summary GitOpsSummary
		if err := json.Unmarshal([]byte(out), &summary); err != nil {
			t.Fatalf("unmarshal gitops status JSON: %v\n%s", err, out)
		}
		if summary.Backend != "flux" || summary.Transport != "git" {
			t.Fatalf("backend=%q transport=%q, want flux/git", summary.Backend, summary.Transport)
		}
		if summary.HealthyCount != 1 || summary.FailedCount != 0 {
			t.Fatalf("healthy=%d failed=%d, want 1/0", summary.HealthyCount, summary.FailedCount)
		}
		if summary.Answer == nil {
			t.Fatal("expected answer envelope")
		}
		if !slices.Contains(gitopsStates, summary.Answer.State) {
			t.Fatalf("unapproved answer.state %q", summary.Answer.State)
		}
		if summary.Answer.State != gitopsStateSettled {
			t.Fatalf("answer.state = %q, want %q", summary.Answer.State, gitopsStateSettled)
		}
		if summary.Answer.ObservedAt != "2026-11-24T20:00:00Z" {
			t.Fatalf("answer.observedAt = %q, want 2026-11-24T20:00:00Z", summary.Answer.ObservedAt)
		}
		if summary.Answer.Cluster.Context != "prod-west" {
			t.Fatalf("answer.cluster.context = %q, want prod-west", summary.Answer.Cluster.Context)
		}
		foundOfflineUnknown := false
		for _, u := range summary.Answer.Unknowns {
			if u.Kind == mapsvc.UnknownStale && strings.Contains(u.Reason, "2026-11-24T20:00:00Z") && strings.Contains(u.Reason, "offline") {
				foundOfflineUnknown = true
				break
			}
		}
		if !foundOfflineUnknown {
			t.Fatalf("expected stale offline unknown with observedAt in answer.unknowns, got %+v", summary.Answer.Unknowns)
		}
	})

	t.Run("failing Argo CD snapshot without observedAt", func(t *testing.T) {
		resetGitOpsStatusTestFlags(t)
		raw := `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: payments
  namespace: argocd
spec:
  source:
    repoURL: https://github.com/example/payments
  destination:
    namespace: prod
status:
  sync:
    status: OutOfSync
  health:
    status: Degraded
    message: Deployment payments has 0/2 replicas ready
`
		recPath := filepath.Join(t.TempDir(), "argo-failing.yaml")
		if err := os.WriteFile(recPath, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}

		cmd := newGitOpsStatusRecordingTestCmd()
		_ = cmd.Flags().Set("recording", recPath)
		_ = cmd.Flags().Set("format", "json")

		var runErr error
		out := captureStdout(t, func() {
			runErr = runGitOpsStatus(cmd, nil)
		})
		if runErr != nil {
			t.Fatalf("runGitOpsStatus offline failed: %v", runErr)
		}

		var summary GitOpsSummary
		if err := json.Unmarshal([]byte(out), &summary); err != nil {
			t.Fatalf("unmarshal gitops status JSON: %v", err)
		}
		if summary.Backend != "argocd" || summary.FailedCount != 1 {
			t.Fatalf("backend=%q failed=%d, want argocd/1", summary.Backend, summary.FailedCount)
		}
		if summary.Answer == nil || summary.Answer.State != gitopsStateFailing {
			t.Fatalf("answer = %+v, want state=failing", summary.Answer)
		}
		foundUnknownObs := false
		for _, u := range summary.Answer.Unknowns {
			if u.Kind == mapsvc.UnknownStale && strings.Contains(u.Reason, "observedAt: unknown") {
				foundUnknownObs = true
				break
			}
		}
		if !foundUnknownObs {
			t.Fatalf("expected stale unknown with observedAt: unknown, got %+v", summary.Answer.Unknowns)
		}
	})
}

func TestDoctorRecording_RejectsLiveAndConfigHubFlags(t *testing.T) {
	recPath := filepath.Join(t.TempDir(), "snap.yaml")
	if err := os.WriteFile(recPath, []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: prod\n"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name    string
		flag    string
		val     string
		wantErr string
	}{
		{name: "kube-context", flag: "kube-context", val: "prod-ctx", wantErr: "--kube-context"},
		{name: "with-confighub", flag: "with-confighub", val: "true", wantErr: "--with-confighub"},
		{name: "before-attach", flag: "before-attach", val: "true", wantErr: "--before-attach"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetDoctorTestFlags(t)
			cmd := newDoctorRecordingTestCmd()
			_ = cmd.Flags().Set("recording", recPath)
			_ = cmd.Flags().Set(tc.flag, tc.val)
			_ = cmd.Flags().Set("format", "json")
			err := runDoctor(cmd, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDoctorRecording_EndToEndSnapshotOfflineEvaluation(t *testing.T) {
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "nonexistent-kubeconfig"))
	resetDoctorTestFlags(t)

	// Build a fake cluster with a healthy Flux Deployment, a Helm release Secret (with secret data),
	// and a failing Argo CD Deployment so we prove end-to-end snapshot -> doctor --recording parity.
	fluxDeploy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name":       "web",
				"namespace":  "prod",
				"generation": int64(2),
				"labels": map[string]interface{}{
					"kustomize.toolkit.fluxcd.io/name":      "apps",
					"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
				},
			},
			"spec": map[string]interface{}{"replicas": int64(2)},
			"status": map[string]interface{}{
				"observedGeneration": int64(2),
				"replicas":           int64(2),
				"updatedReplicas":    int64(2),
				"availableReplicas":  int64(2),
				"readyReplicas":      int64(2),
			},
		},
	}
	helmSecret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       "helm.sh/release.v1",
			"metadata": map[string]interface{}{
				"name":      "sh.helm.release.v1.web.v1",
				"namespace": "prod",
				"labels":    map[string]interface{}{"owner": "helm", "name": "web", "status": "deployed"},
			},
			"data": map[string]interface{}{"release": "MUST-NEVER-APPEAR"},
		},
	}

	dynClient := newFakeSnapshotDynamicClient(fluxDeploy, helmSecret)
	captured, truncated, err := collectRecordedSnapshotObjects(context.Background(), dynClient, "prod", "", 200)
	if err != nil {
		t.Fatalf("collectRecordedSnapshotObjects failed: %v", err)
	}
	if truncated || len(captured) != 2 {
		t.Fatalf("captured=%d truncated=%v, want 2/false", len(captured), truncated)
	}

	observedAt := time.Date(2026, 11, 24, 19, 15, 0, 0, time.UTC)
	var buf bytes.Buffer
	if err := encodeRecordedSnapshot(&buf, snapshotFormatJSON, captured, observedAt, "staging-east", false); err != nil {
		t.Fatalf("encodeRecordedSnapshot failed: %v", err)
	}
	if strings.Contains(buf.String(), "MUST-NEVER-APPEAR") {
		t.Fatal("snapshot JSON leaked secret payload")
	}

	recPath := filepath.Join(t.TempDir(), "cluster-snapshot.json")
	if err := os.WriteFile(recPath, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := newDoctorRecordingTestCmd()
	_ = cmd.Flags().Set("recording", recPath)
	_ = cmd.Flags().Set("namespace", "prod")
	_ = cmd.Flags().Set("format", "json")

	var runErr error
	out := captureStdout(t, func() {
		runErr = runDoctor(cmd, nil)
	})
	if runErr != nil {
		t.Fatalf("runDoctor offline failed: %v", runErr)
	}

	var summary DoctorSummary
	if err := json.Unmarshal([]byte(out), &summary); err != nil {
		t.Fatalf("unmarshal doctor JSON: %v\n%s", err, out)
	}
	if summary.Resources.Total != 2 {
		t.Fatalf("resources.total = %d, want 2", summary.Resources.Total)
	}
	if summary.Ownership.Flux != 1 {
		t.Fatalf("ownership.flux = %d, want 1", summary.Ownership.Flux)
	}
	if summary.Answer == nil {
		t.Fatal("expected answer envelope")
	}
	if !slices.Contains(doctorStates, summary.Answer.State) {
		t.Fatalf("unapproved doctor answer.state %q", summary.Answer.State)
	}
	if summary.Answer.State != doctorStateNothingFlagged {
		t.Fatalf("answer.state = %q, want %q", summary.Answer.State, doctorStateNothingFlagged)
	}
	if summary.Answer.ObservedAt != "2026-11-24T19:15:00Z" {
		t.Fatalf("answer.observedAt = %q, want 2026-11-24T19:15:00Z", summary.Answer.ObservedAt)
	}
	if summary.Answer.Cluster.Context != "staging-east" {
		t.Fatalf("answer.cluster.context = %q, want staging-east", summary.Answer.Cluster.Context)
	}

	foundOfflineUnknown := false
	for _, u := range summary.Answer.Unknowns {
		if u.Kind == mapsvc.UnknownStale && strings.Contains(u.Reason, "2026-11-24T19:15:00Z") && strings.Contains(u.Reason, "offline") {
			foundOfflineUnknown = true
			break
		}
	}
	if !foundOfflineUnknown {
		t.Fatalf("expected stale offline unknown with observedAt in answer.unknowns, got %+v", summary.Answer.Unknowns)
	}
}
