// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func recordedGovernance(t *testing.T, packet, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", packet, name))
	require.NoError(t, err)
	return b
}

func recordedGovernanceOrigin(t *testing.T, raw []byte) ConfigHubOriginEvidence {
	t.Helper()
	var r configHubRevisionRead
	require.NoError(t, json.Unmarshal(raw, &r))
	return ConfigHubOriginEvidence{SpaceID: r.Revision.SpaceID, UnitID: r.Revision.UnitID, UnitSlug: r.Revision.UnitSlug, RevisionNum: &r.Revision.RevisionNum}
}

func TestConfigHubAttestationsGenuineRevocationExpiryAndFailure(t *testing.T) {
	packet := "confighub-governance-v083-recorded"
	revision := recordedGovernance(t, packet, "revision-get.json")
	list := recordedGovernance(t, packet, "attestation-list-after-revoke.json")
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	out, err := ParseConfigHubAttestations(recordedGovernanceOrigin(t, revision), revision, list, now)
	require.NoError(t, err)
	require.Len(t, out.Attestations, 3)
	require.Equal(t, "direct-references-only", out.Coverage)
	require.Equal(t, "Pass", out.Attestations[0].Result)
	require.NotNil(t, out.Attestations[0].Revoked)
	require.True(t, *out.Attestations[0].Revoked)
	require.Len(t, out.Attestations[0].ObservedRevocationIDs, 1)
	require.Equal(t, "Fail", out.Attestations[1].Result)
	require.Nil(t, out.Attestations[1].Revoked, "no observed revocation is not proof of absence")
	require.True(t, out.Attestations[2].Expired)
	require.NotEmpty(t, out.Omissions)
	before := time.Date(2026, 10, 5, 5, 45, 9, 0, time.UTC)
	out, err = ParseConfigHubAttestations(recordedGovernanceOrigin(t, revision), revision, list, before)
	require.NoError(t, err)
	require.False(t, out.Attestations[2].Expired)
	// Row order never changes output or the digest a receipt computes over it.
	var rows []json.RawMessage
	require.NoError(t, json.Unmarshal(list, &rows))
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	reversed, err := json.Marshal(rows)
	require.NoError(t, err)
	got, err := ParseConfigHubAttestations(recordedGovernanceOrigin(t, revision), revision, reversed, before)
	require.NoError(t, err)
	require.Equal(t, out, got)
}

func TestConfigHubAttestationsGenuineFilteredListAndAbsentMap(t *testing.T) {
	packet := "confighub-governance-v083-revision-chain"
	revision := recordedGovernance(t, packet, "revision-initial-get.json")
	list := recordedGovernance(t, packet, "attestation-filtered-list.json")
	now := time.Date(2026, 10, 5, 17, 0, 0, 0, time.UTC)
	out, err := ParseConfigHubAttestations(recordedGovernanceOrigin(t, revision), revision, list, now)
	require.NoError(t, err)
	require.Len(t, out.Attestations, 1)
	require.Equal(t, "SecurityReview", out.Attestations[0].Type)
	require.Len(t, out.Omissions, 2)
	for _, name := range []string{"revision-after-data-get.json", "revision-after-restore-get.json"} {
		raw := recordedGovernance(t, packet, name)
		out, err := ParseConfigHubAttestations(recordedGovernanceOrigin(t, raw), raw, nil, now)
		require.NoError(t, err)
		require.Empty(t, out.Attestations)
		require.Contains(t, out.Omissions[0].Reason, "coverage are unknown")
	}
}

func TestConfigHubAttestationsRejectAmbiguousIdentity(t *testing.T) {
	packet := "confighub-governance-v083-recorded"
	revision := recordedGovernance(t, packet, "revision-get.json")
	list := recordedGovernance(t, packet, "attestation-list.json")
	origin := recordedGovernanceOrigin(t, revision)
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	for _, field := range []string{"space", "unit", "revision", "unitID"} {
		t.Run(field, func(t *testing.T) {
			bad := origin
			switch field {
			case "space":
				bad.SpaceID = "other"
			case "unit":
				bad.UnitSlug = "other"
			case "revision":
				n := int64(3)
				bad.RevisionNum = &n
			case "unitID":
				bad.UnitID = "other"
			}
			out, err := ParseConfigHubAttestations(bad, revision, list, now)
			require.Error(t, err)
			require.Nil(t, out)
		})
	}
	for _, bad := range [][]byte{[]byte(`null`), []byte(`{}`), []byte(`[{}]`), []byte(`[{"Attestation":{},"Attestation":{}}]`)} {
		out, err := ParseConfigHubAttestations(origin, revision, bad, now)
		require.Error(t, err)
		require.Nil(t, out)
	}
	var rows []json.RawMessage
	require.NoError(t, json.Unmarshal(list, &rows))
	rows = append(rows, rows[0])
	duplicate, err := json.Marshal(rows)
	require.NoError(t, err)
	_, err = ParseConfigHubAttestations(origin, revision, duplicate, now)
	require.Error(t, err)
}

func TestConfigHubAttestationsReceiptFingerprintAndVerdict(t *testing.T) {
	packet := "confighub-governance-v083-recorded"
	revision := recordedGovernance(t, packet, "revision-get.json")
	list := recordedGovernance(t, packet, "attestation-list-after-revoke.json")
	evidence, err := ParseConfigHubAttestations(recordedGovernanceOrigin(t, revision), revision, list, time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	in := makeReceiptInput()
	without, err := BuildReceipt(in)
	require.NoError(t, err)
	in.Evidence.Attestations = evidence
	with, err := BuildReceipt(in)
	require.NoError(t, err)
	require.Equal(t, without.Predicate.Verdict, with.Predicate.Verdict)
	require.NoError(t, VerifyStatementFingerprint(with))
	require.NotEqual(t, without.Predicate.Fingerprint, with.Predicate.Fingerprint)
	with.Predicate.Evidence.Attestations.Attestations[0].Result = "Fail"
	require.Error(t, VerifyStatementFingerprint(with))
}

func TestConfigHubAttestationsUnknownTypeClaimsAndExplicitEmpty(t *testing.T) {
	packet := "confighub-governance-v083-recorded"
	revision := recordedGovernance(t, packet, "revision-get.json")
	list := recordedGovernance(t, packet, "attestation-list.json")
	origin := recordedGovernanceOrigin(t, revision)
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	// Deliberate parser mutations of genuine inputs; these are not recordings.
	var rows []map[string]interface{}
	require.NoError(t, json.Unmarshal(list, &rows))
	a := rows[0]["Attestation"].(map[string]interface{})
	a["Type"] = "UnknownFutureClaim"
	a["Claims"] = map[string]string{"artifact": "sha256:example"}
	raw, err := json.Marshal(rows)
	require.NoError(t, err)
	out, err := ParseConfigHubAttestations(origin, revision, raw, now)
	require.NoError(t, err)
	require.Equal(t, "UnknownFutureClaim", out.Attestations[0].Type)
	require.Equal(t, "sha256:example", out.Attestations[0].Claims["artifact"])
	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(revision, &envelope))
	envelope["Revision"].(map[string]interface{})["Attestations"] = map[string]string{}
	raw, err = json.Marshal(envelope)
	require.NoError(t, err)
	out, err = ParseConfigHubAttestations(origin, raw, []byte("[]"), now)
	require.NoError(t, err)
	require.Equal(t, "direct-references-only", out.Coverage)
	require.Empty(t, out.Attestations)
	require.Len(t, out.Omissions, 1, "empty direct map does not establish effective coverage")
}

func TestReceiptConfigHubDataHashRequiresActualBytes(t *testing.T) {
	data := recordedGovernance(t, "confighub-governance-v083-recorded", "revision-data.txt")
	sum := sha256.Sum256(data)
	in := makeReceiptInput()
	in.Connected = true
	in.ConfigHubUnitSlug = "contract-config"
	in.ConfigHubUnitRev = 2
	in.ConfigHubUnitCanonical = []byte(`{"fixture":"canonical identity"}`)
	in.ConfigHubUnitServedData = data
	in.ConfigHubUnitDataHash = hex.EncodeToString(sum[:])
	stmt, err := BuildReceipt(in)
	require.NoError(t, err)
	require.Len(t, stmt.Subject, 2)
	require.Equal(t, in.ConfigHubUnitDataHash, stmt.Subject[1].Digest["confighub-data-sha256"])
	require.NotEmpty(t, stmt.Subject[1].Digest["sha256"])
	require.NoError(t, VerifyStatementFingerprint(stmt))
	stmt.Subject[1].Digest["confighub-data-sha256"] = "bad"
	require.Error(t, VerifyStatementFingerprint(stmt))
	in.ConfigHubUnitServedData = append(data, ' ')
	_, err = BuildReceipt(in)
	require.Error(t, err, "copying reported metadata cannot populate the digest")
}
