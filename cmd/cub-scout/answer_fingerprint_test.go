// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

func TestComputeAnswerFingerprint_IgnoresObservedAtAndCost(t *testing.T) {
	require.Equal(t, "", computeAnswerFingerprint(nil))

	base := &mapsvc.Answer{
		State:      agent.LandingStateLanded,
		Reason:     agent.LandingReasonDigestAndWorkloadConverged,
		Summary:    "Deployment/api converged on the requested oci-digest.",
		ObservedAt: "2026-11-24T16:00:00Z",
		Cluster:    mapsvc.AnswerCluster{Context: "prod-east", IDSource: "not-read"},
		Basis: []mapsvc.AnswerBasis{
			{Claim: "deployer reported ready", RestsOn: mapsvc.RestsOnControllerReport, Quoted: "revision=latest@sha256:0828"},
		},
		Unknowns: []mapsvc.AnswerUnknown{},
		Next:     []mapsvc.AnswerNext{},
		Cost:     mapsvc.AnswerCost{RequestsMade: 3, ResponseBodyBytes: 4096, DurationMillis: 42, Coverage: "landed-session-client"},
	}

	later := *base
	later.ObservedAt = "2026-11-24T16:05:30Z"
	later.Cost = mapsvc.AnswerCost{RequestsMade: 4, ResponseBodyBytes: 8192, DurationMillis: 199, Coverage: "landed-session-client"}

	fp1 := computeAnswerFingerprint(base)
	fp2 := computeAnswerFingerprint(&later)
	require.Len(t, fp1, answerFingerprintHexLen)
	require.Equal(t, fp1, fp2, "volatile observedAt and cost must not change fingerprint")

	changed := *base
	changed.State = agent.LandingStateConverging
	fp3 := computeAnswerFingerprint(&changed)
	require.NotEqual(t, fp1, fp3, "state change must change fingerprint")
}

func TestLandedIfUnchanged_MatchAndMismatch(t *testing.T) {
	deployRef := agent.LandingObjectRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}
	ksRef := &agent.LandingObjectRef{APIVersion: "kustomize.toolkit.fluxcd.io/v1", Kind: "Kustomization", Namespace: "flux-system", Name: "apps"}

	eval := agent.EvaluateLanding(
		agent.LandingAnchor{Kind: agent.AnchorKindOCIDigest, Value: "sha256:0828"},
		agent.LandingSnapshot{
			Context:     "prod-east",
			Target:      deployRef,
			TargetFound: true,
			Owner:       "Flux",
			ObservedAt:  observedAtForTests,
			Deployer:    &agent.LandingLinkObservation{Present: true, Ref: ksRef, Status: "ready", Revision: "latest@sha256:0828"},
			Workload:    &agent.LandingLinkObservation{Present: true, Ref: &deployRef, Status: "ready"},
		},
	)
	out := buildLandedOutput(eval, nil)
	require.Len(t, out.Fingerprint, answerFingerprintHexLen)

	// Full payload (no --if-unchanged).
	var fullBuf bytes.Buffer
	require.NoError(t, renderLandedOutputWithDelta(&fullBuf, out, "json", ""))
	var fullDecoded landedOutput
	require.NoError(t, json.Unmarshal(fullBuf.Bytes(), &fullDecoded))
	require.Equal(t, out.Fingerprint, fullDecoded.Fingerprint)
	require.NotNil(t, fullDecoded.Answer)

	// Matching --if-unchanged (even with uppercase / whitespace) returns compact unchanged envelope.
	outLater := out
	ansLater := *out.Answer
	ansLater.ObservedAt = observedAtForTests.Add(30 * time.Second).UTC().Format(time.RFC3339)
	outLater.Answer = &ansLater

	var unchangedBuf bytes.Buffer
	require.NoError(t, renderLandedOutputWithDelta(&unchangedBuf, outLater, "json", " "+out.Fingerprint+" "))
	require.LessOrEqual(t, unchangedBuf.Len(), 120, "unchanged envelope should stay <= 120 bytes: %s", unchangedBuf.String())
	require.Less(t, float64(unchangedBuf.Len())/float64(fullBuf.Len()), 0.15, "unchanged envelope must save >85%% bytes vs full payload")

	var env unchangedEnvelope
	require.NoError(t, json.Unmarshal(unchangedBuf.Bytes(), &env))
	require.True(t, env.Unchanged)
	require.Equal(t, out.Fingerprint, env.Fingerprint)
	require.Equal(t, ansLater.ObservedAt, env.ObservedAt)

	// Non-matching --if-unchanged returns the full JSON payload.
	var mismatchBuf bytes.Buffer
	require.NoError(t, renderLandedOutputWithDelta(&mismatchBuf, outLater, "json", "deadbeef00000000"))
	var mismatchDecoded landedOutput
	require.NoError(t, json.Unmarshal(mismatchBuf.Bytes(), &mismatchDecoded))
	require.Equal(t, out.Fingerprint, mismatchDecoded.Fingerprint)
	require.NotNil(t, mismatchDecoded.Answer)
}

func TestGitOpsStatusIfUnchanged_MatchAndMismatch(t *testing.T) {
	summary := GitOpsSummary{
		Context:   "prod-east",
		Backend:   "flux",
		Transport: "oci",
		Deployers: []DeployerStatus{
			{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: false, Reason: "BuildFailed", Stage: "build"},
		},
		Sources: []SourceStatus{
			{Kind: "OCIRepository", Name: "apps", Namespace: "flux-system", Ready: true, URL: "oci://registry/apps"},
		},
		FailedCount: 1,
	}
	summary.Answer = buildGitOpsStatusAnswer(summary, observedAtForTests, nil)

	// Full payload emits fingerprint.
	var fullBuf bytes.Buffer
	require.NoError(t, writeGitOpsStatusJSON(&fullBuf, summary, ""))
	var fullDecoded GitOpsSummary
	require.NoError(t, json.Unmarshal(fullBuf.Bytes(), &fullDecoded))
	require.Len(t, fullDecoded.Fingerprint, answerFingerprintHexLen)
	require.NotNil(t, fullDecoded.Answer)

	// Matching --if-unchanged returns compact unchanged envelope.
	summaryLater := summary
	summaryLater.Answer = buildGitOpsStatusAnswer(summary, observedAtForTests.Add(45*time.Second), nil)

	var unchangedBuf bytes.Buffer
	require.NoError(t, writeGitOpsStatusJSON(&unchangedBuf, summaryLater, fullDecoded.Fingerprint))
	require.LessOrEqual(t, unchangedBuf.Len(), 120, "unchanged envelope should stay <= 120 bytes: %s", unchangedBuf.String())
	require.Less(t, float64(unchangedBuf.Len())/float64(fullBuf.Len()), 0.15, "unchanged envelope must save >85%% bytes vs full payload")

	var env unchangedEnvelope
	require.NoError(t, json.Unmarshal(unchangedBuf.Bytes(), &env))
	require.True(t, env.Unchanged)
	require.Equal(t, fullDecoded.Fingerprint, env.Fingerprint)
	require.Equal(t, summaryLater.Answer.ObservedAt, env.ObservedAt)

	// Non-matching --if-unchanged returns full payload.
	var mismatchBuf bytes.Buffer
	require.NoError(t, writeGitOpsStatusJSON(&mismatchBuf, summaryLater, "0000000000000000"))
	var mismatchDecoded GitOpsSummary
	require.NoError(t, json.Unmarshal(mismatchBuf.Bytes(), &mismatchDecoded))
	require.Equal(t, fullDecoded.Fingerprint, mismatchDecoded.Fingerprint)
	require.NotNil(t, mismatchDecoded.Answer)
}
