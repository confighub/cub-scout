// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"testing"
	"time"
)

func TestEvaluateLanding_RecordedFixtures(t *testing.T) {
	at := time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)
	deployRef := LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	ociRef := &LandingObjectRef{APIVersion: "source.toolkit.fluxcd.io/v1", Kind: "OCIRepository", Namespace: "flux-system", Name: "apps"}
	ksRef := &LandingObjectRef{APIVersion: "kustomize.toolkit.fluxcd.io/v1", Kind: "Kustomization", Namespace: "flux-system", Name: "apps"}
	argoRef := &LandingObjectRef{APIVersion: "argoproj.io/v1alpha1", Kind: "Application", Namespace: "argocd", Name: "api"}

	const targetDigest = "sha256:0828082808280828"
	const priorDigest = "sha256:0000000000000001"
	const targetSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const olderSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	t.Run("01-flux-oci-landed", func(t *testing.T) {
		eval := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindOCIDigest, Value: targetDigest},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: "Flux", ObservedAt: at,
				Source:   &LandingLinkObservation{Present: true, Ref: ociRef, Status: "ready", Revision: "latest@" + targetDigest},
				Deployer: &LandingLinkObservation{Present: true, Ref: ksRef, Status: "ready", Revision: "latest@" + targetDigest},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		)
		if eval.State != LandingStateLanded || eval.Reason != LandingReasonDigestAndWorkloadConverged {
			t.Fatalf("got (%s, %s), want (%s, %s)", eval.State, eval.Reason, LandingStateLanded, LandingReasonDigestAndWorkloadConverged)
		}
	})

	t.Run("02-flux-oci-converging-prior-rev", func(t *testing.T) {
		eval := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindOCIDigest, Value: targetDigest},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: "Flux", ObservedAt: at,
				Source:   &LandingLinkObservation{Present: true, Ref: ociRef, Status: "ready", Revision: "latest@" + targetDigest},
				Deployer: &LandingLinkObservation{Present: true, Ref: ksRef, Status: "ready", Revision: "latest@" + priorDigest},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		)
		if eval.State != LandingStateConverging || eval.Reason != LandingReasonControllerOnPriorRevision {
			t.Fatalf("got (%s, %s), want (%s, %s)", eval.State, eval.Reason, LandingStateConverging, LandingReasonControllerOnPriorRevision)
		}
		if eval.Convergence == nil || eval.Convergence.WaitingOn != LandingStageDeployer || eval.Convergence.RecheckAfterSeconds <= 0 {
			t.Fatalf("unexpected convergence: %+v", eval.Convergence)
		}
	})

	t.Run("03-flux-kustomization-build-failed", func(t *testing.T) {
		eval := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindOCIDigest, Value: targetDigest},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: "Flux", ObservedAt: at,
				Source:   &LandingLinkObservation{Present: true, Ref: ociRef, Status: "ready", Revision: "latest@" + targetDigest},
				Deployer: &LandingLinkObservation{Present: true, Ref: ksRef, Status: "failed", Message: "kustomize build failed"},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		)
		if eval.State != LandingStateNotLanded || eval.Reason != LandingReasonControllerReconcileFailed {
			t.Fatalf("got (%s, %s), want (%s, %s)", eval.State, eval.Reason, LandingStateNotLanded, LandingReasonControllerReconcileFailed)
		}
		if eval.BrokenLink == nil || eval.BrokenLink.Stage != LandingStageDeployer {
			t.Fatalf("unexpected brokenLink: %+v", eval.BrokenLink)
		}
	})

	t.Run("04-argo-settled-wrong-revision", func(t *testing.T) {
		eval := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: targetSHA},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: "ArgoCD", ObservedAt: at,
				Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: olderSHA},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		)
		if eval.State != LandingStateNotLanded || eval.Reason != LandingReasonControllerSettledOtherRevision {
			t.Fatalf("got (%s, %s), want (%s, %s)", eval.State, eval.Reason, LandingStateNotLanded, LandingReasonControllerSettledOtherRevision)
		}
	})

	t.Run("05-workload-rolling-out", func(t *testing.T) {
		eval := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: targetSHA},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: "ArgoCD", ObservedAt: at,
				Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: targetSHA},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "progressing", Message: "1/3 replicas ready"},
			},
		)
		if eval.State != LandingStateConverging || eval.Reason != LandingReasonWorkloadRollingOut {
			t.Fatalf("got (%s, %s), want (%s, %s)", eval.State, eval.Reason, LandingStateConverging, LandingReasonWorkloadRollingOut)
		}
		if eval.Convergence == nil || eval.Convergence.WaitingOn != LandingStageWorkload {
			t.Fatalf("unexpected convergence: %+v", eval.Convergence)
		}
	})

	t.Run("06-rbac-denied-and-no-owner", func(t *testing.T) {
		denied := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: targetSHA},
			LandingSnapshot{Context: "prod-east", Target: deployRef, ReadDenied: true, ObservedAt: at},
		)
		if denied.State != LandingStateNotObservable || denied.Reason != LandingReasonReadDenied {
			t.Fatalf("denied got (%s, %s), want (%s, %s)", denied.State, denied.Reason, LandingStateNotObservable, LandingReasonReadDenied)
		}

		noOwner := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: targetSHA},
			LandingSnapshot{Context: "prod-east", Target: deployRef, TargetFound: true, Owner: "Native", ObservedAt: at},
		)
		if noOwner.State != LandingStateNotObservable || noOwner.Reason != LandingReasonNoOwnerMarkerRecognised {
			t.Fatalf("noOwner got (%s, %s), want (%s, %s)", noOwner.State, noOwner.Reason, LandingStateNotObservable, LandingReasonNoOwnerMarkerRecognised)
		}
	})
}

func TestWaitForLanding_BoundedWaitEarlyExitAndTimeout(t *testing.T) {
	deployRef := LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	argoRef := &LandingObjectRef{APIVersion: "argoproj.io/v1alpha1", Kind: "Application", Namespace: "argocd", Name: "api"}
	anchor := LandingAnchor{Kind: AnchorKindGitSHA, Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}

	t.Run("07-bounded-wait-settles-early", func(t *testing.T) {
		clock := time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)
		snaps := []LandingSnapshot{
			{
				Target: deployRef, TargetFound: true, Owner: "ArgoCD",
				Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "progressing"},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "progressing"},
			},
			{
				Target: deployRef, TargetFound: true, Owner: "ArgoCD",
				Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: anchor.Value},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "progressing"},
			},
			{
				Target: deployRef, TargetFound: true, Owner: "ArgoCD",
				Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: anchor.Value},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		}
		idx := 0
		eval, err := WaitForLanding(context.Background(), anchor, LandingWaitOptions{
			Wait:         30 * time.Second,
			PollInterval: 5 * time.Second,
			MaxWait:      MaxLandingWaitMCP,
			Now:          func() time.Time { return clock },
			Sleep: func(_ context.Context, d time.Duration) error {
				clock = clock.Add(d)
				return nil
			},
		}, func(context.Context) (LandingSnapshot, error) {
			s := snaps[idx]
			idx++
			return s, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if eval.State != LandingStateLanded {
			t.Fatalf("state = %s, want landed", eval.State)
		}
		if eval.Wait == nil || eval.Wait.Polls != 3 || !eval.Wait.Settled || eval.Wait.ElapsedMillis != 10000 {
			t.Fatalf("unexpected wait summary: %+v", eval.Wait)
		}
	})

	t.Run("08-bounded-wait-timeout", func(t *testing.T) {
		clock := time.Date(2026, 11, 24, 16, 0, 0, 0, time.UTC)
		eval, err := WaitForLanding(context.Background(), anchor, LandingWaitOptions{
			Wait:         12 * time.Second,
			PollInterval: 5 * time.Second,
			MaxWait:      MaxLandingWaitMCP,
			Now:          func() time.Time { return clock },
			Sleep: func(_ context.Context, d time.Duration) error {
				clock = clock.Add(d)
				return nil
			},
		}, func(context.Context) (LandingSnapshot, error) {
			return LandingSnapshot{
				Target: deployRef, TargetFound: true, Owner: "ArgoCD",
				Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "progressing"},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "progressing"},
			}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if eval.State != LandingStateConverging || eval.Convergence == nil || !eval.Convergence.TimedOut {
			t.Fatalf("want converging+timedOut, got state=%s convergence=%+v", eval.State, eval.Convergence)
		}
		if eval.Wait == nil || eval.Wait.Settled || eval.Wait.ElapsedMillis != 12000 {
			t.Fatalf("unexpected wait summary: %+v", eval.Wait)
		}
	})

	t.Run("rejects wait above cap", func(t *testing.T) {
		_, err := WaitForLanding(context.Background(), anchor, LandingWaitOptions{
			Wait:    2 * time.Minute,
			MaxWait: MaxLandingWaitMCP,
		}, func(context.Context) (LandingSnapshot, error) {
			return LandingSnapshot{}, nil
		})
		if err == nil {
			t.Fatal("expected error for --wait exceeding MaxLandingWaitMCP")
		}
	})
}

// Zero false-landed invariant: none of these negative controls may ever yield "landed".
func TestEvaluateLanding_ZeroFalseLandedInvariant(t *testing.T) {
	deployRef := LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	argoRef := &LandingObjectRef{APIVersion: "argoproj.io/v1alpha1", Kind: "Application", Namespace: "argocd", Name: "api"}
	anchor := LandingAnchor{Kind: AnchorKindGitSHA, Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}

	negativeControls := []struct {
		name   string
		anchor LandingAnchor
		snap   LandingSnapshot
	}{
		{"empty anchor", LandingAnchor{}, LandingSnapshot{Target: deployRef, TargetFound: true, Owner: "ArgoCD"}},
		{"cluster unreachable", anchor, LandingSnapshot{Target: deployRef, Unreachable: true}},
		{"rbac denied", anchor, LandingSnapshot{Target: deployRef, ReadDenied: true}},
		{"target not found", anchor, LandingSnapshot{Target: deployRef, TargetFound: false}},
		{"unrecognised owner", anchor, LandingSnapshot{Target: deployRef, TargetFound: true, Owner: "Native"}},
		{"unsupported controller", anchor, LandingSnapshot{Target: deployRef, TargetFound: true, Owner: "Sveltos"}},
		{"missing deployer", anchor, LandingSnapshot{Target: deployRef, TargetFound: true, Owner: "ArgoCD"}},
		{"wrong revision on healthy workload", anchor, LandingSnapshot{
			Target: deployRef, TargetFound: true, Owner: "ArgoCD",
			Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: "cccccccccccccccccccccccccccccccccccccccc"},
			Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
		}},
		{"target revision still rolling out", anchor, LandingSnapshot{
			Target: deployRef, TargetFound: true, Owner: "ArgoCD",
			Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: anchor.Value},
			Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "progressing"},
		}},
		{"workload failed on target revision", anchor, LandingSnapshot{
			Target: deployRef, TargetFound: true, Owner: "ArgoCD",
			Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: anchor.Value},
			Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "failed"},
		}},
	}

	for _, tc := range negativeControls {
		t.Run(tc.name, func(t *testing.T) {
			eval := EvaluateLanding(tc.anchor, tc.snap)
			if eval.State == LandingStateLanded {
				t.Fatalf("negative control %q produced false landed (%+v)", tc.name, eval)
			}
		})
	}
}
