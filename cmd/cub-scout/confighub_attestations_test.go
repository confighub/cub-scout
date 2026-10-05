// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
)

func attestationTestInputs(t *testing.T) (agent.ConfigHubOriginEvidence, []byte, []byte) {
	t.Helper()
	root := filepath.Join("..", "..", "test", "fixtures", "confighub-governance-v083-recorded")
	revision, err := os.ReadFile(filepath.Join(root, "revision-get.json"))
	require.NoError(t, err)
	list, err := os.ReadFile(filepath.Join(root, "attestation-list-after-revoke.json"))
	require.NoError(t, err)
	n := int64(2)
	return agent.ConfigHubOriginEvidence{SpaceID: "68843338-f9bd-485c-9a66-5d5aee820247", SpaceSlug: "scout-v213-contract", UnitID: "6221926e-8cc4-4847-a25c-a4104042f640", UnitSlug: "contract-config", RevisionNum: &n}, revision, list
}

func TestCollectConfigHubAttestationsBoundedReadsAndMCP(t *testing.T) {
	origin, revision, list := attestationTestInputs(t)
	old := configHubAttestationsRead
	t.Cleanup(func() { configHubAttestationsRead = old })
	var calls [][]string
	configHubAttestationsRead = func(ctx context.Context, args ...string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 15*time.Second)
		calls = append(calls, args)
		if args[0] == "revision" {
			return revision, nil
		}
		return list, nil
	}
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	evidence, err := collectConfigHubAttestations(context.Background(), origin, now)
	require.NoError(t, err)
	require.Len(t, evidence.Attestations, 3)
	require.Equal(t, [][]string{{"revision", "get", "contract-config", "2", "--space", origin.SpaceID, "-o", "json"}, {"attestation", "list", "--space", origin.SpaceID, "-o", "json"}}, calls)
	tool := configHubAttestationsMCPTool()
	args, err := tool.BuildArgs(map[string]interface{}{"space_id": origin.SpaceID, "unit": origin.UnitSlug, "unit_id": origin.UnitID, "revision": float64(2)})
	require.NoError(t, err)
	raw, err := tool.Runner(context.Background(), args)
	require.NoError(t, err)
	var got agent.ConfigHubAttestationsEvidence
	require.NoError(t, json.Unmarshal([]byte(raw), &got))
	require.Equal(t, evidence.Attestations, got.Attestations)
	require.True(t, tool.Descriptor.Annotations.ReadOnlyHint)
	_, standalone := newMCPGatewayWithMode(nil, nil, false).tools["confighub_attestations"]
	require.False(t, standalone)
	_, connected := newMCPGatewayWithMode(nil, nil, true).tools["confighub_attestations"]
	require.True(t, connected)
	for _, bad := range []map[string]interface{}{{}, {"space_id": "*", "unit": "x", "revision": 2}, {"space_id": origin.SpaceID, "unit": "--other", "revision": 2}, {"space_id": origin.SpaceID, "unit": "x", "revision": 2.5}} {
		_, err := tool.BuildArgs(bad)
		require.Error(t, err)
	}
}

func TestAttachConfigHubAttestationsDegradesWithoutBroadReads(t *testing.T) {
	origin, revision, _ := attestationTestInputs(t)
	old := configHubAttestationsRead
	t.Cleanup(func() { configHubAttestationsRead = old })
	calls := 0
	configHubAttestationsRead = func(ctx context.Context, args ...string) ([]byte, error) {
		calls++
		return revision, fmt.Errorf("private server error")
	}
	for _, input := range []*agent.ConfigHubOriginEvidence{nil, {}} {
		out := &agent.TraceDeliveryEvidence{ObservedAt: time.Now()}
		attachConfigHubAttestations(context.Background(), out, input, "")
		require.Nil(t, out.Attestations)
		require.Len(t, out.Omissions, 1)
	}
	out := &agent.TraceDeliveryEvidence{ObservedAt: time.Now()}
	attachConfigHubAttestations(context.Background(), out, &origin, "different-space")
	require.Zero(t, calls)
	require.Contains(t, out.Omissions[0].Reason, "conflicts")
	out = &agent.TraceDeliveryEvidence{ObservedAt: time.Now()}
	attachConfigHubAttestations(context.Background(), out, &origin, origin.SpaceSlug)
	require.Equal(t, 1, calls)
	require.Nil(t, out.Attestations)
	require.NotContains(t, out.Omissions[0].Reason, "private server error")
	// Byte limits apply before output accumulation; failure never returns a
	// truncated JSON response for the parser to mistake for complete coverage.
	var b attestationLimitedBuffer
	_, err := b.Write(make([]byte, 1024*1024))
	require.NoError(t, err)
	_, err = b.Write([]byte("x"))
	require.Error(t, err)
	require.Equal(t, 1024*1024, b.Len())
}

func TestConfigHubAttestationsCLIAndTUIRenderingAgree(t *testing.T) {
	origin, revision, list := attestationTestInputs(t)
	evidence, err := agent.ParseConfigHubAttestations(origin, revision, list, time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	delivery := &agent.TraceDeliveryEvidence{Attestations: evidence}
	summary := ExplainSummary{DeliveryEvidence: delivery}
	for _, render := range []string{renderExplainText(summary, PresentationHuman, true, HintContext{}), renderExplainMarkdown(summary, PresentationHuman, true, HintContext{}), formatTraceDeliveryEvidenceLine(delivery), newEnrichedExplainViewer(summary).content} {
		for _, fact := range []string{"Approval:Pass", "Approval:Fail", "expired=true", "revoked=observed", "revoked=unknown", "Effective/inherited"} {
			require.Contains(t, render, fact)
		}
	}
	var noChain bytes.Buffer
	err = renderTraceHuman(&noChain, &agent.TraceResult{Error: "no controller chain", DeliveryEvidence: delivery}, nil, InvocationContext{}, traceHumanOptions{})
	require.Error(t, err)
	require.Contains(t, noChain.String(), "revoked=observed", "missing controller chain must not discard supporting facts")
	// Production readers can only invoke these two read verbs.
	data, err := os.ReadFile("confighub_attestations.go")
	require.NoError(t, err)
	for _, forbidden := range []string{`"attestation", "create"`, `"attestation", "revoke"`, "CreateAttestation(", ".Attest("} {
		require.False(t, strings.Contains(string(data), forbidden))
	}
}

func TestReceiptConfigHubSubjectUsesVerifiedServedBytes(t *testing.T) {
	origin, revision, list := attestationTestInputs(t)
	evidence, err := agent.ParseConfigHubAttestations(origin, revision, list, time.Now())
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "confighub-governance-v083-recorded", "revision-data.txt"))
	require.NoError(t, err)
	old := configHubAttestationsRead
	t.Cleanup(func() { configHubAttestationsRead = old })
	var argsSeen []string
	configHubAttestationsRead = func(ctx context.Context, args ...string) ([]byte, error) { argsSeen = args; return data, nil }
	canonical, served, err := collectReceiptConfigHubRevisionData(context.Background(), evidence)
	require.NoError(t, err)
	require.Equal(t, []string{"revision", "data", origin.UnitSlug, "2", "--space", origin.SpaceID}, argsSeen)
	require.Equal(t, data, served)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(canonical, &body))
	require.Equal(t, evidence.RevisionID, body["revisionId"])
	require.Equal(t, "confighub-unit-canonical.v1", body["schema"])
	canonicalHash := sha256.Sum256(canonical)
	require.NotEqual(t, evidence.DataHash, hex.EncodeToString(canonicalHash[:]), "canonical identity and served bytes have distinct digest inputs")
	data = append(data, ' ')
	canonical, served, err = collectReceiptConfigHubRevisionData(context.Background(), evidence)
	require.Error(t, err)
	require.Nil(t, canonical)
	require.Nil(t, served)
}

func TestConfigHubAttestationExamplesRetainIntegrityAndVerdict(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "receipts", "attestations")
	manifestRaw, err := os.ReadFile(filepath.Join(root, "capture-manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Files map[string]string `json:"files"`
	}
	require.NoError(t, json.Unmarshal(manifestRaw, &manifest))
	require.Len(t, manifest.Files, 3)
	for file, want := range manifest.Files {
		raw, err := os.ReadFile(filepath.Join(root, file))
		require.NoError(t, err)
		sum := sha256.Sum256(raw)
		require.Equal(t, want, hex.EncodeToString(sum[:]))
	}
	readReceipt := func(name string) agent.Statement {
		raw, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		var stmt agent.Statement
		require.NoError(t, json.Unmarshal(raw, &stmt))
		require.NoError(t, agent.VerifyStatementFingerprint(stmt))
		return stmt
	}
	with, without := readReceipt("receipt.json"), readReceipt("receipt-without-claims.json")
	require.Equal(t, without.Predicate.Verdict, with.Predicate.Verdict)
	require.Len(t, with.Subject, 2)
	require.Len(t, without.Subject, 1)
	require.NotEmpty(t, with.Subject[1].Digest["sha256"])
	require.Equal(t, with.Predicate.Evidence.Attestations.DataHash, with.Subject[1].Digest["confighub-data-sha256"])
	at, err := time.Parse(time.RFC3339, with.Predicate.VerifiedAt)
	require.NoError(t, err)
	require.Equal(t, at, with.Predicate.Evidence.Attestations.ObservedAt)
	require.Len(t, with.Predicate.Evidence.Attestations.Attestations, 3)
	for _, claim := range with.Predicate.Evidence.Attestations.Attestations {
		require.Equal(t, claim.ExpiresAt != nil && !at.Before(*claim.ExpiresAt), claim.Expired)
	}
	data, err := os.ReadFile(filepath.Join(root, "explain.json"))
	require.NoError(t, err)
	var explain ExplainSummary
	require.NoError(t, json.Unmarshal(data, &explain))
	require.Equal(t, with.Predicate.Evidence.Attestations.Attestations, explain.DeliveryEvidence.Attestations.Attestations)
}
