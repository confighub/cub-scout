// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"sort"
	"time"

	kjson "sigs.k8s.io/json"
)

// ConfigHubAttestationsEvidence describes direct server claims, never an
// approval decision or effective/inherited coverage. Unknown revocation status
// stays unknown: a list response cannot establish absence of revocations.
type ConfigHubAttestationsEvidence struct {
	Source       string                         `json:"source"`
	SpaceID      string                         `json:"spaceId"`
	UnitID       string                         `json:"unitId"`
	UnitSlug     string                         `json:"unitSlug"`
	RevisionID   string                         `json:"revisionId"`
	RevisionNum  int64                          `json:"revisionNum"`
	DataHash     string                         `json:"dataHash,omitempty"`
	ObservedAt   time.Time                      `json:"observedAt"`
	Coverage     string                         `json:"coverage"`
	Attestations []ConfigHubAttestationEvidence `json:"attestations"`
	Omissions    []Omission                     `json:"omissions,omitempty"`
}

type ConfigHubAttestationEvidence struct {
	ID                    string            `json:"id"`
	Type                  string            `json:"type"`
	Result                string            `json:"result"`
	UserID                string            `json:"userId,omitempty"`
	CreatedAt             time.Time         `json:"createdAt"`
	ExpiresAt             *time.Time        `json:"expiresAt,omitempty"`
	Expired               bool              `json:"expired"`
	Claims                map[string]string `json:"claims,omitempty"`
	Revoked               *bool             `json:"revoked,omitempty"`
	ObservedRevocationIDs []string          `json:"observedRevocationIds,omitempty"`
}

type configHubRevisionRead struct {
	Revision struct {
		SpaceID, UnitID, UnitSlug, RevisionID, DataHash string
		RevisionNum                                     int64
		Attestations                                    map[string]string
	}
}

type configHubAttestationRead struct {
	Attestation struct {
		AttestationID, SpaceID, Type, Result, UserID, RevokedAttestationID string
		CreatedAt, ExpiresAt                                               time.Time
		Claims                                                             map[string]string
	}
}

// ParseConfigHubAttestations joins only the exact requested revision's ID map.
// The list may be filtered or permission limited, so missing IDs are omissions.
// Hash equality alone never transfers claims to another revision. The caller
// supplies the observation time; this function has no clock or network reads.
func ParseConfigHubAttestations(origin ConfigHubOriginEvidence, revisionJSON, listJSON []byte, observedAt time.Time) (*ConfigHubAttestationsEvidence, error) {
	if origin.SpaceID == "" || origin.UnitSlug == "" || origin.RevisionNum == nil || *origin.RevisionNum <= 0 || observedAt.IsZero() {
		return nil, fmt.Errorf("attestations require an exact space, unit, positive revision and observation time")
	}
	var revision configHubRevisionRead
	if err := decodeConfigHubAttestationRead(revisionJSON, &revision); err != nil {
		return nil, err
	}
	r := revision.Revision
	if r.SpaceID != origin.SpaceID || r.UnitSlug != origin.UnitSlug || r.RevisionNum != *origin.RevisionNum || r.UnitID == "" || r.RevisionID == "" || (origin.UnitID != "" && r.UnitID != origin.UnitID) {
		return nil, fmt.Errorf("ConfigHub revision response does not match the exact requested identity")
	}
	out := &ConfigHubAttestationsEvidence{Source: "confighub-revision-direct-claims", SpaceID: r.SpaceID, UnitID: r.UnitID, UnitSlug: r.UnitSlug, RevisionID: r.RevisionID, RevisionNum: r.RevisionNum, DataHash: r.DataHash, ObservedAt: observedAt.UTC(), Coverage: "direct-references-only", Attestations: []ConfigHubAttestationEvidence{}}
	omit := func(reason string) {
		out.Omissions = append(out.Omissions, Omission{Missing: "confighub-attestations", Reason: reason, Severity: "inconclusive"})
	}
	if r.Attestations == nil {
		out.Coverage = "direct-references-unavailable"
		omit("Revision GET omitted Attestations; direct and effective coverage are unknown.")
		return out, nil
	}
	if len(r.Attestations) > 100 {
		return nil, fmt.Errorf("revision exceeds the 100 direct-reference evidence limit")
	}
	var rows []configHubAttestationRead
	if err := decodeConfigHubAttestationRead(listJSON, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, fmt.Errorf("attestation list must be a JSON array")
	}
	if len(rows) > 1000 {
		return nil, fmt.Errorf("attestation list exceeds the 1000-row evidence limit")
	}
	byID := map[string]configHubAttestationRead{}
	revocations := map[string][]string{}
	for _, row := range rows {
		a := row.Attestation
		if a.AttestationID == "" || a.SpaceID != r.SpaceID {
			return nil, fmt.Errorf("attestation list contains a missing identity or different space")
		}
		if _, duplicate := byID[a.AttestationID]; duplicate {
			return nil, fmt.Errorf("attestation list contains a duplicate identity")
		}
		byID[a.AttestationID] = row
		if a.RevokedAttestationID != "" {
			revocations[a.RevokedAttestationID] = append(revocations[a.RevokedAttestationID], a.AttestationID)
		}
	}
	ids := make([]string, 0, len(r.Attestations))
	for id := range r.Attestations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		row, ok := byID[id]
		if !ok {
			omit("Referenced attestation " + id + " was not returned; filtered or permission-limited reads do not establish absence.")
			continue
		}
		a := row.Attestation
		if a.Type == "" || (a.Result != "Pass" && a.Result != "Fail") || a.CreatedAt.IsZero() || a.RevokedAttestationID != "" {
			omit("Referenced attestation " + id + " has an unsupported claim shape.")
			continue
		}
		claim := ConfigHubAttestationEvidence{ID: id, Type: a.Type, Result: a.Result, UserID: a.UserID, CreatedAt: a.CreatedAt, Claims: a.Claims}
		if !a.ExpiresAt.IsZero() {
			expiry := a.ExpiresAt
			claim.ExpiresAt = &expiry
			claim.Expired = !observedAt.Before(expiry)
		}
		if revocations[id] != nil {
			revoked := true
			claim.Revoked = &revoked
			claim.ObservedRevocationIDs = revocations[id]
			sort.Strings(claim.ObservedRevocationIDs)
		}
		out.Attestations = append(out.Attestations, claim)
	}
	omit("Effective/inherited coverage and absence of revocation are not established by revision GET and attestation list.")
	return out, nil
}

func decodeConfigHubAttestationRead(raw []byte, out interface{}) error {
	if len(raw) == 0 || len(raw) > 1024*1024 {
		return fmt.Errorf("ConfigHub attestation response is empty or exceeds 1 MiB")
	}
	errors, err := kjson.UnmarshalStrict(raw, out, kjson.DisallowDuplicateFields)
	if err != nil || len(errors) != 0 {
		return fmt.Errorf("ConfigHub attestation response is malformed or ambiguous")
	}
	return nil
}
