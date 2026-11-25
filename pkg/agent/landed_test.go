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

	t.Run("07-kro-and-crossplane-landed-and-missing-revision", func(t *testing.T) {
		kroRef := &LandingObjectRef{APIVersion: "kro.run/v1alpha1", Kind: "LaneApp", Namespace: "kro-lane", Name: "lane"}
		kroLanded := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: "2"},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: OwnerKro, ObservedAt: at,
				Deployer: &LandingLinkObservation{Present: true, Ref: kroRef, Status: "ready", Revision: "2"},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		)
		if kroLanded.State != LandingStateLanded || kroLanded.Reason != LandingReasonRevisionAndWorkloadConverged {
			t.Fatalf("kro landed got (%s, %s)", kroLanded.State, kroLanded.Reason)
		}

		kroNoRev := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: "2"},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: OwnerKro, ObservedAt: at,
				Deployer: &LandingLinkObservation{Present: true, Ref: kroRef, Status: "ready", Revision: ""},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		)
		if kroNoRev.State != LandingStateNotObservable || kroNoRev.Reason != LandingReasonUnsupportedController {
			t.Fatalf("kro no-rev got (%s, %s), want not-observable/unsupported-controller", kroNoRev.State, kroNoRev.Reason)
		}

		xpRef := &LandingObjectRef{APIVersion: "lane.example.org/v1", Kind: "XLaneApp", Namespace: "xp-lane", Name: "lane"}
		xpLanded := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: "1"},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: OwnerCrossplane, ObservedAt: at,
				Deployer: &LandingLinkObservation{Present: true, Ref: xpRef, Status: "ready", Revision: "1"},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		)
		if xpLanded.State != LandingStateLanded || xpLanded.Reason != LandingReasonRevisionAndWorkloadConverged {
			t.Fatalf("crossplane landed got (%s, %s)", xpLanded.State, xpLanded.Reason)
		}

		xpNoRev := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: "1"},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: OwnerCrossplane, ObservedAt: at,
				Deployer: &LandingLinkObservation{Present: true, Ref: xpRef, Status: "ready", Revision: ""},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		)
		if xpNoRev.State != LandingStateNotObservable || xpNoRev.Reason != LandingReasonUnsupportedController {
			t.Fatalf("crossplane no-rev got (%s, %s), want not-observable/unsupported-controller", xpNoRev.State, xpNoRev.Reason)
		}
	})

	t.Run("08-sveltos-always-not-observable", func(t *testing.T) {
		svRef := &LandingObjectRef{APIVersion: "config.projectsveltos.io/v1beta1", Kind: "ClusterProfile", Name: "prod-addons"}
		eval := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: targetSHA},
			LandingSnapshot{
				Context: "prod-east", Target: deployRef, TargetFound: true, Owner: OwnerSveltos, ObservedAt: at,
				Deployer: &LandingLinkObservation{Present: true, Ref: svRef, Status: "unknown"},
				Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			},
		)
		if eval.State != LandingStateNotObservable || eval.Reason != LandingReasonUnsupportedController {
			t.Fatalf("sveltos got (%s, %s), want not-observable/unsupported-controller", eval.State, eval.Reason)
		}
	})

	t.Run("09-job-and-cronjob-targets", func(t *testing.T) {
		jobRef := LandingObjectRef{APIVersion: "batch/v1", Kind: "Job", Namespace: "prod", Name: "migrate"}
		jobOK := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: targetSHA},
			LandingSnapshot{
				Context: "prod-east", Target: jobRef, TargetFound: true, Owner: "ArgoCD", ObservedAt: at,
				Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: targetSHA},
				Workload: &LandingLinkObservation{Present: true, Ref: &jobRef, Status: "ready"},
			},
		)
		if jobOK.State != LandingStateLanded || jobOK.Reason != LandingReasonRevisionAndWorkloadConverged {
			t.Fatalf("job ok got (%s, %s)", jobOK.State, jobOK.Reason)
		}

		cjRef := LandingObjectRef{APIVersion: "batch/v1", Kind: "CronJob", Namespace: "prod", Name: "nightly"}
		cjProg := EvaluateLanding(
			LandingAnchor{Kind: AnchorKindGitSHA, Value: targetSHA},
			LandingSnapshot{
				Context: "prod-east", Target: cjRef, TargetFound: true, Owner: "ArgoCD", ObservedAt: at,
				Deployer: &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: targetSHA},
				Workload: &LandingLinkObservation{Present: true, Ref: &cjRef, Status: "progressing", Message: "CronJob has 1 active Job(s)"},
			},
		)
		if cjProg.State != LandingStateConverging || cjProg.Reason != LandingReasonWorkloadRollingOut {
			t.Fatalf("cronjob progressing got (%s, %s)", cjProg.State, cjProg.Reason)
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
		{"pod image mismatch despite ready controller and workload", anchor, LandingSnapshot{
			Target: deployRef, TargetFound: true, Owner: "ArgoCD",
			Deployer:  &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: anchor.Value},
			Workload:  &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			PodImages: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "failed", Message: "pod api-1 imageID did not match"},
		}},
		{"pod image verification unknown despite ready controller", anchor, LandingSnapshot{
			Target: deployRef, TargetFound: true, Owner: "ArgoCD",
			Deployer:  &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: anchor.Value},
			Workload:  &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
			PodImages: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "unknown"},
		}},
		{"kro ready without observedGeneration", anchor, LandingSnapshot{
			Target: deployRef, TargetFound: true, Owner: OwnerKro,
			Deployer: &LandingLinkObservation{Present: true, Status: "ready", Revision: ""},
			Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
		}},
		{"crossplane ready without observedGeneration", anchor, LandingSnapshot{
			Target: deployRef, TargetFound: true, Owner: OwnerCrossplane,
			Deployer: &LandingLinkObservation{Present: true, Status: "ready", Revision: ""},
			Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
		}},
		{"sveltos with ready workload", anchor, LandingSnapshot{
			Target: deployRef, TargetFound: true, Owner: OwnerSveltos,
			Deployer: &LandingLinkObservation{Present: true, Status: "ready", Revision: anchor.Value},
			Workload: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
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

func TestPodImageMatches(t *testing.T) {
	const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const otherDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

	cases := []struct {
		name     string
		expected string
		imageID  string
		image    string
		want     bool
	}{
		{"bare digest matches docker-pullable imageID", digest, "docker-pullable://ghcr.io/org/app@" + digest, "ghcr.io/org/app:v1", true},
		{"bare digest matches containerd imageID", digest, "ghcr.io/org/app@" + digest, "ghcr.io/org/app:v1", true},
		{"bare digest matches bare imageID", digest, digest, "ghcr.io/org/app:v1", true},
		{"bare digest mismatch", otherDigest, "docker-pullable://ghcr.io/org/app@" + digest, "ghcr.io/org/app:v1", false},
		{"repo+digest exact match", "ghcr.io/org/app@" + digest, "docker-pullable://ghcr.io/org/app@" + digest, "ghcr.io/org/app:v1", true},
		{"repo+digest different repo rejected", "evil.io/other@" + digest, "docker-pullable://ghcr.io/org/app@" + digest, "ghcr.io/org/app:v1", false},
		{"short library repo+digest matches docker.io/library", "nginx@" + digest, "docker-pullable://docker.io/library/nginx@" + digest, "nginx:1.25", true},
		{"tag ref matches exact image", "ghcr.io/org/app:v1.2.3", "docker-pullable://ghcr.io/org/app@" + digest, "ghcr.io/org/app:v1.2.3", true},
		{"short tag ref matches docker.io/library", "nginx:1.25", "", "docker.io/library/nginx:1.25", true},
		{"tag ref mismatch", "nginx:1.26", "", "docker.io/library/nginx:1.25", false},
		{"empty expected never matches", "", "docker-pullable://ghcr.io/org/app@" + digest, "ghcr.io/org/app:v1", false},
		{"empty sha256 prefix never matches", "sha256:", "docker-pullable://ghcr.io/org/app@" + digest, "ghcr.io/org/app:v1", false},
		{"empty observed never matches", digest, "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PodImageMatches(tc.expected, tc.imageID, tc.image); got != tc.want {
				t.Fatalf("PodImageMatches(%q, %q, %q) = %v, want %v", tc.expected, tc.imageID, tc.image, got, tc.want)
			}
		})
	}
}

func TestEvaluateLanding_PodImagesWitness(t *testing.T) {
	deployRef := LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	podRef := &LandingObjectRef{APIVersion: "v1", Kind: "Pod", Namespace: "prod", Name: "api-abc12"}
	argoRef := &LandingObjectRef{APIVersion: "argoproj.io/v1alpha1", Kind: "Application", Namespace: "argocd", Name: "api"}
	anchor := LandingAnchor{Kind: AnchorKindGitSHA, Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}

	okEval := EvaluateLanding(anchor, LandingSnapshot{
		Target: deployRef, TargetFound: true, Owner: "ArgoCD",
		Deployer:  &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: anchor.Value},
		Workload:  &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
		PodImages: &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready", Message: "2/2 running pods matched"},
	})
	if okEval.State != LandingStateLanded || okEval.Reason != LandingReasonRevisionAndWorkloadConverged {
		t.Fatalf("positive pod witness got (%s, %s), want (landed, revision-and-workload-converged)", okEval.State, okEval.Reason)
	}

	mismatchEval := EvaluateLanding(anchor, LandingSnapshot{
		Target: deployRef, TargetFound: true, Owner: "ArgoCD",
		Deployer:  &LandingLinkObservation{Present: true, Ref: argoRef, Status: "ready", Revision: anchor.Value},
		Workload:  &LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
		PodImages: &LandingLinkObservation{Present: true, Ref: podRef, Status: "failed", Message: "running pod api-abc12 disagreed"},
	})
	if mismatchEval.State != LandingStateNotLanded || mismatchEval.Reason != LandingReasonWorkloadFailed {
		t.Fatalf("negative pod witness got (%s, %s), want (not-landed, workload-failed)", mismatchEval.State, mismatchEval.Reason)
	}
	if mismatchEval.BrokenLink == nil || mismatchEval.BrokenLink.Stage != LandingStageWorkload || mismatchEval.BrokenLink.Ref == nil || mismatchEval.BrokenLink.Ref.Kind != "Pod" {
		t.Fatalf("unexpected brokenLink: %+v", mismatchEval.BrokenLink)
	}
}
