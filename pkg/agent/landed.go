// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Landing states approved on 2026-11-24 (A(1) + G1/G2; docs/proposals/did-my-change-land.md).
const (
	LandingStateLanded        = "landed"
	LandingStateConverging    = "converging"
	LandingStateNotLanded     = "not-landed"
	LandingStateNotObservable = "not-observable"
)

// Landing reason keys (closed set).
const (
	LandingReasonRevisionAndWorkloadConverged = "revision-and-workload-converged"
	LandingReasonDigestAndWorkloadConverged   = "digest-and-workload-converged"

	LandingReasonSourceFetching            = "source-fetching"
	LandingReasonControllerReconciling     = "controller-reconciling"
	LandingReasonControllerOnPriorRevision = "controller-on-prior-revision"
	LandingReasonWorkloadRollingOut        = "workload-rolling-out"

	LandingReasonSourceError                    = "source-error"
	LandingReasonControllerReconcileFailed      = "controller-reconcile-failed"
	LandingReasonControllerSettledOtherRevision = "controller-settled-other-revision"
	LandingReasonWorkloadFailed                 = "workload-failed"

	LandingReasonNoAnchorSupplied         = "no-anchor-supplied"
	LandingReasonTargetNotFound           = "target-not-found"
	LandingReasonNoOwnerMarkerRecognised  = "no-owner-marker-recognised"
	LandingReasonUnsupportedController    = "unsupported-controller"
	LandingReasonReadDenied               = "read-denied"
	LandingReasonClusterUnreachable       = "cluster-unreachable"
)

// Landing anchor kinds.
const (
	AnchorKindGitSHA       = "git-sha"
	AnchorKindOCIDigest    = "oci-digest"
	AnchorKindUnitRevision = "unit-revision"
)

// Landing stage names for brokenLink and convergence.waitingOn.
const (
	LandingStageSource   = "source"
	LandingStageDeployer = "deployer"
	LandingStageWorkload = "workload"
)

// Bounded wait defaults and caps (G2; approved 2026-11-24).
const (
	DefaultLandingPollInterval = 5 * time.Second
	MinLandingPollInterval     = 2 * time.Second
	MaxLandingWaitCLI          = 5 * time.Minute
	MaxLandingWaitMCP          = 60 * time.Second
)

// LandingAnchor is the caller-supplied revision, digest, or unit revision to
// verify against the live delivery chain.
type LandingAnchor struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// LandingObjectRef identifies a Kubernetes object observed during the check.
type LandingObjectRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
}

// LandingBrokenLink names the failing stage when state is not-landed.
type LandingBrokenLink struct {
	Stage  string            `json:"stage"`
	Ref    *LandingObjectRef `json:"ref,omitempty"`
	Quoted string            `json:"quoted,omitempty"`
}

// LandingConvergence describes what a converging check is waiting on (G1).
type LandingConvergence struct {
	WaitingOn           string `json:"waitingOn"`
	RecheckAfterSeconds int    `json:"recheckAfterSeconds"`
	TimedOut            bool   `json:"timedOut"`
}

// LandingWait summarizes bounded-wait execution (G2).
type LandingWait struct {
	RequestedSeconds int   `json:"requestedSeconds"`
	ElapsedMillis    int64 `json:"elapsedMillis"`
	Polls            int   `json:"polls"`
	Settled          bool  `json:"settled"`
}

// LandingLinkObservation captures what one link in the delivery chain reported.
// Status is one of: "ready", "progressing", "failed", "unknown".
type LandingLinkObservation struct {
	Present  bool              `json:"present"`
	Ref      *LandingObjectRef `json:"ref,omitempty"`
	Status   string            `json:"status"`
	Revision string            `json:"revision,omitempty"`
	Message  string            `json:"message,omitempty"`
	// Reporter names whose word this observation rests on (e.g. controller-report).
	Reporter string `json:"reporter,omitempty"`
}

// LandingSnapshot is the point-in-time read of a target workload and its
// Flux or Argo CD delivery chain.
type LandingSnapshot struct {
	Context     string
	Target      LandingObjectRef
	TargetFound bool
	ReadDenied  bool
	Unreachable bool
	ReadError   string

	// Owner is the detected owner type ("Flux", "ArgoCD", "Helm", "Native", ...).
	Owner      string
	Source     *LandingLinkObservation
	Deployer   *LandingLinkObservation
	Workload   *LandingLinkObservation
	PodImages  *LandingLinkObservation
	ObservedAt time.Time
}

// LandingEvaluation is the deterministic decision produced from one snapshot
// (or a bounded wait over snapshots).
type LandingEvaluation struct {
	State       string              `json:"state"`
	Reason      string              `json:"reason"`
	Summary     string              `json:"summary"`
	Anchor      LandingAnchor       `json:"anchor"`
	BrokenLink  *LandingBrokenLink  `json:"brokenLink,omitempty"`
	Convergence *LandingConvergence `json:"convergence,omitempty"`
	Wait        *LandingWait        `json:"wait,omitempty"`
	Snapshot    LandingSnapshot     `json:"-"`
}

// AnchorMatches reports whether observedRevision carries the requested anchor.
// Matching is exact on normalised strings or exact suffix/substring for
// standard Flux/Argo revision formats (e.g., "main@sha1:<sha>", "latest@sha256:<hex>",
// or unit revision annotations). Empty values never match.
func AnchorMatches(anchor LandingAnchor, observed string) bool {
	want := strings.TrimSpace(anchor.Value)
	got := strings.TrimSpace(observed)
	if want == "" || got == "" {
		return false
	}
	if got == want {
		return true
	}
	switch anchor.Kind {
	case AnchorKindGitSHA:
		// Flux formats Git revisions as "<branch>@sha1:<40-hex>" or "<40-hex>".
		if strings.HasSuffix(got, ":"+want) || strings.HasSuffix(got, "@"+want) {
			return true
		}
		// Allow unambiguous short SHA prefix (>= 7 hex chars) against a full SHA.
		if len(want) >= 7 {
			if idx := strings.LastIndex(got, ":"); idx >= 0 {
				got = got[idx+1:]
			}
			return strings.HasPrefix(got, want)
		}
		return false
	case AnchorKindOCIDigest:
		// Flux OCIRepository reports "<tag>@sha256:<hex>" or "sha256:<hex>".
		if strings.HasSuffix(got, "@"+want) {
			return true
		}
		wantNorm := strings.TrimPrefix(want, "sha256:")
		gotNorm := got
		if idx := strings.LastIndex(gotNorm, "sha256:"); idx >= 0 {
			gotNorm = gotNorm[idx+len("sha256:"):]
		}
		return wantNorm != "" && gotNorm == wantNorm
	case AnchorKindUnitRevision:
		return got == want
	default:
		return false
	}
}

func stripImageScheme(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"docker-pullable://", "docker://", "containerd://", "cri-o://"} {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimPrefix(s, prefix)
		}
	}
	return s
}

func extractSHA256Digest(s string) string {
	s = stripImageScheme(s)
	idx := strings.LastIndex(s, "sha256:")
	if idx < 0 {
		return ""
	}
	d := strings.TrimSpace(s[idx:])
	if len(d) <= len("sha256:") {
		return ""
	}
	return d
}

// PodImageMatches reports whether a running container's status.imageID or
// status.image matches an expected image reference or sha256 digest.
// Matching is deterministic and conservative: empty or malformed values never match.
func PodImageMatches(expected, imageID, image string) bool {
	want := stripImageScheme(expected)
	if want == "" {
		return false
	}
	gotID := stripImageScheme(imageID)
	gotImg := stripImageScheme(image)
	if gotID == "" && gotImg == "" {
		return false
	}

	if wantDigest := extractSHA256Digest(want); wantDigest != "" {
		// Bare digest: sha256:<hex>
		if strings.HasPrefix(want, "sha256:") {
			return extractSHA256Digest(gotID) == wantDigest || extractSHA256Digest(gotImg) == wantDigest
		}
		// Repository-qualified digest: <repo>@sha256:<hex>
		if gotID == want || gotImg == want {
			return true
		}
		wantRepo, _, hasAt := strings.Cut(want, "@")
		if !hasAt || wantRepo == "" {
			return false
		}
		for _, candidate := range []string{gotID, gotImg} {
			if extractSHA256Digest(candidate) != wantDigest {
				continue
			}
			gotRepo, _, ok := strings.Cut(candidate, "@")
			if !ok {
				continue
			}
			if gotRepo == wantRepo || strings.HasSuffix(gotRepo, "/"+wantRepo) {
				return true
			}
		}
		return false
	}

	// Tag or repository reference without digest.
	if gotImg == want || gotID == want {
		return true
	}
	firstSeg, _, hasSlash := strings.Cut(want, "/")
	hasRegistry := hasSlash && (strings.Contains(firstSeg, ".") || strings.Contains(firstSeg, ":") || firstSeg == "localhost")
	if !hasRegistry {
		if gotImg == "docker.io/library/"+want || gotImg == "docker.io/"+want {
			return true
		}
	}
	return false
}

// EvaluateLanding deterministically classifies a single LandingSnapshot
// against anchor. It enforces the zero false-landed invariant: missing,
// denied, wrong-revision, or still-rolling-out evidence never yields "landed".
func EvaluateLanding(anchor LandingAnchor, snap LandingSnapshot) LandingEvaluation {
	eval := LandingEvaluation{
		Anchor:   anchor,
		Snapshot: snap,
	}

	if strings.TrimSpace(anchor.Value) == "" || strings.TrimSpace(anchor.Kind) == "" {
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonNoAnchorSupplied
		eval.Summary = "No revision, digest, or unit-revision anchor was supplied, so landing cannot be checked."
		return eval
	}
	if snap.Unreachable {
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonClusterUnreachable
		eval.Summary = "Cluster could not be reached, so landing is not observable."
		return eval
	}
	if snap.ReadDenied {
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonReadDenied
		eval.Summary = "Reading the target or its delivery chain was denied by Kubernetes RBAC."
		return eval
	}
	if !snap.TargetFound {
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonTargetNotFound
		eval.Summary = fmt.Sprintf("%s/%s was not found in namespace %s.", snap.Target.Kind, snap.Target.Name, snap.Target.Namespace)
		return eval
	}

	ownerNorm := strings.ToLower(strings.TrimSpace(snap.Owner))
	switch ownerNorm {
	case "", "native", "unknown":
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonNoOwnerMarkerRecognised
		eval.Summary = fmt.Sprintf("No GitOps owner marker was recognised on %s/%s.", snap.Target.Kind, snap.Target.Name)
		return eval
	case "flux", "argocd", "argo", "helm":
		// Supported controllers (Flux, Argo CD, Helm).
	default:
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonUnsupportedController
		eval.Summary = fmt.Sprintf("Owner %s is not yet supported by landed; use trace or explain.", snap.Owner)
		return eval
	}

	// 1. Source link (when present, e.g., Flux GitRepository / OCIRepository).
	if snap.Source != nil && snap.Source.Present {
		switch snap.Source.Status {
		case "failed":
			eval.State = LandingStateNotLanded
			eval.Reason = LandingReasonSourceError
			eval.Summary = "The delivery source reported a failure."
			eval.BrokenLink = &LandingBrokenLink{
				Stage:  LandingStageSource,
				Ref:    snap.Source.Ref,
				Quoted: snap.Source.Message,
			}
			return eval
		case "progressing":
			eval.State = LandingStateConverging
			eval.Reason = LandingReasonSourceFetching
			eval.Summary = "The delivery source is still fetching or reconciling."
			eval.Convergence = &LandingConvergence{
				WaitingOn:           LandingStageSource,
				RecheckAfterSeconds: 10,
			}
			return eval
		}
	}

	// 2. Deployer link (Flux Kustomization / HelmRelease, or Argo CD Application).
	if snap.Deployer == nil || !snap.Deployer.Present {
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonTargetNotFound
		eval.Summary = "The delivery controller object for this workload could not be read."
		return eval
	}

	switch snap.Deployer.Status {
	case "failed":
		eval.State = LandingStateNotLanded
		eval.Reason = LandingReasonControllerReconcileFailed
		eval.Summary = "The delivery controller reported a reconciliation failure."
		eval.BrokenLink = &LandingBrokenLink{
			Stage:  LandingStageDeployer,
			Ref:    snap.Deployer.Ref,
			Quoted: snap.Deployer.Message,
		}
		return eval
	case "progressing":
		eval.State = LandingStateConverging
		eval.Reason = LandingReasonControllerReconciling
		eval.Summary = "The delivery controller is still reconciling."
		eval.Convergence = &LandingConvergence{
			WaitingOn:           LandingStageDeployer,
			RecheckAfterSeconds: 10,
		}
		return eval
	case "unknown", "":
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonTargetNotFound
		eval.Summary = "The delivery controller's status could not be determined."
		return eval
	}

	// Deployer is "ready" (settled). Check whether its applied revision matches the anchor.
	deployerMatches := AnchorMatches(anchor, snap.Deployer.Revision)
	if !deployerMatches {
		// If the source already has the requested anchor, the deployer is still
		// on a prior revision and has not reconciled the new artifact yet (G1).
		if snap.Source != nil && snap.Source.Present && AnchorMatches(anchor, snap.Source.Revision) {
			eval.State = LandingStateConverging
			eval.Reason = LandingReasonControllerOnPriorRevision
			eval.Summary = "The source has the requested revision, but the deployer is still on a prior revision."
			eval.Convergence = &LandingConvergence{
				WaitingOn:           LandingStageDeployer,
				RecheckAfterSeconds: 10,
			}
			return eval
		}
		eval.State = LandingStateNotLanded
		eval.Reason = LandingReasonControllerSettledOtherRevision
		eval.Summary = "The delivery controller is settled on a different revision than requested."
		eval.BrokenLink = &LandingBrokenLink{
			Stage:  LandingStageDeployer,
			Ref:    snap.Deployer.Ref,
			Quoted: snap.Deployer.Revision,
		}
		return eval
	}

	// 3. Workload link (kstatus on Deployment / StatefulSet / DaemonSet).
	if snap.Workload == nil || !snap.Workload.Present {
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonTargetNotFound
		eval.Summary = "Workload readiness was not observed."
		return eval
	}

	switch snap.Workload.Status {
	case "ready":
		if snap.PodImages != nil && snap.PodImages.Present {
			switch snap.PodImages.Status {
			case "ready":
				// Independent pod-image witness confirmed running pods match.
			case "failed":
				eval.State = LandingStateNotLanded
				eval.Reason = LandingReasonWorkloadFailed
				eval.Summary = fmt.Sprintf("Controller and %s/%s reported ready, but running Pod images disagreed with the expected image or digest.", snap.Target.Kind, snap.Target.Name)
				eval.BrokenLink = &LandingBrokenLink{
					Stage:  LandingStageWorkload,
					Ref:    snap.PodImages.Ref,
					Quoted: snap.PodImages.Message,
				}
				return eval
			default:
				eval.State = LandingStateNotObservable
				eval.Reason = LandingReasonTargetNotFound
				eval.Summary = fmt.Sprintf("Running Pod image verification for %s/%s could not be determined.", snap.Target.Kind, snap.Target.Name)
				return eval
			}
		}
		eval.State = LandingStateLanded
		if anchor.Kind == AnchorKindOCIDigest {
			eval.Reason = LandingReasonDigestAndWorkloadConverged
		} else {
			eval.Reason = LandingReasonRevisionAndWorkloadConverged
		}
		eval.Summary = fmt.Sprintf("%s/%s converged on the requested %s.", snap.Target.Kind, snap.Target.Name, anchor.Kind)
		return eval
	case "progressing":
		eval.State = LandingStateConverging
		eval.Reason = LandingReasonWorkloadRollingOut
		eval.Summary = fmt.Sprintf("Controller applied the requested %s; %s/%s is still rolling out.", anchor.Kind, snap.Target.Kind, snap.Target.Name)
		eval.Convergence = &LandingConvergence{
			WaitingOn:           LandingStageWorkload,
			RecheckAfterSeconds: 5,
		}
		return eval
	case "failed":
		eval.State = LandingStateNotLanded
		eval.Reason = LandingReasonWorkloadFailed
		eval.Summary = fmt.Sprintf("Controller applied the requested %s, but %s/%s reported failure.", anchor.Kind, snap.Target.Kind, snap.Target.Name)
		eval.BrokenLink = &LandingBrokenLink{
			Stage:  LandingStageWorkload,
			Ref:    snap.Workload.Ref,
			Quoted: snap.Workload.Message,
		}
		return eval
	default:
		eval.State = LandingStateNotObservable
		eval.Reason = LandingReasonTargetNotFound
		eval.Summary = "Workload readiness state is unknown."
		return eval
	}
}

// LandingWaitOptions configures G2 bounded, read-only waiting.
type LandingWaitOptions struct {
	Wait         time.Duration
	PollInterval time.Duration
	MaxWait      time.Duration
	Now          func() time.Time
	Sleep        func(context.Context, time.Duration) error
}

// WaitForLanding performs one check (when Wait <= 0) or a bounded read-only
// poll loop (when Wait > 0) until state settles or the deadline expires (G2).
func WaitForLanding(
	ctx context.Context,
	anchor LandingAnchor,
	opts LandingWaitOptions,
	fetch func(context.Context) (LandingSnapshot, error),
) (LandingEvaluation, error) {
	maxWait := opts.MaxWait
	if maxWait <= 0 {
		maxWait = MaxLandingWaitCLI
	}
	if opts.Wait < 0 {
		return LandingEvaluation{}, fmt.Errorf("--wait must be >= 0 (got %s)", opts.Wait)
	}
	if opts.Wait > maxWait {
		return LandingEvaluation{}, fmt.Errorf("--wait %s exceeds maximum allowed %s", opts.Wait, maxWait)
	}
	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = DefaultLandingPollInterval
	}
	if opts.Wait > 0 && pollInterval < MinLandingPollInterval {
		return LandingEvaluation{}, fmt.Errorf("--poll-interval %s must be at least %s", pollInterval, MinLandingPollInterval)
	}

	nowFn := opts.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	sleepFn := opts.Sleep
	if sleepFn == nil {
		sleepFn = func(c context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-c.Done():
				return c.Err()
			case <-t.C:
				return nil
			}
		}
	}

	start := nowFn()
	deadline := start.Add(opts.Wait)
	polls := 0

	for {
		polls++
		snap, err := fetch(ctx)
		if err != nil {
			return LandingEvaluation{}, err
		}
		eval := EvaluateLanding(anchor, snap)

		// Single-shot mode (--wait 0s): return immediately without a wait block.
		if opts.Wait == 0 {
			return eval, nil
		}

		// Terminal states exit early.
		if eval.State != LandingStateConverging {
			eval.Wait = &LandingWait{
				RequestedSeconds: int(opts.Wait.Seconds()),
				ElapsedMillis:    nowFn().Sub(start).Milliseconds(),
				Polls:            polls,
				Settled:          true,
			}
			return eval, nil
		}

		remaining := deadline.Sub(nowFn())
		if remaining <= 0 {
			if eval.Convergence != nil {
				eval.Convergence.TimedOut = true
			}
			eval.Wait = &LandingWait{
				RequestedSeconds: int(opts.Wait.Seconds()),
				ElapsedMillis:    nowFn().Sub(start).Milliseconds(),
				Polls:            polls,
				Settled:          false,
			}
			return eval, nil
		}

		step := pollInterval
		if step > remaining {
			step = remaining
		}
		if err := sleepFn(ctx, step); err != nil {
			return LandingEvaluation{}, err
		}
	}
}
