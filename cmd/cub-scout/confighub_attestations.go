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
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

var configHubAttestationsRead = boundedAttestationCubRead

// The two reads have independent byte limits and one shared deadline. Neither
// read invokes workflow evaluation, creates claims, or revokes anything.
func collectConfigHubAttestations(ctx context.Context, origin agent.ConfigHubOriginEvidence, at time.Time) (*agent.ConfigHubAttestationsEvidence, error) {
	if !validAttestationOrigin(origin) {
		return nil, fmt.Errorf("exact ConfigHub origin with a positive revision is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	revision, err := configHubAttestationsRead(ctx, "revision", "get", origin.UnitSlug, strconv.FormatInt(*origin.RevisionNum, 10), "--space", origin.SpaceID, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("revision read unavailable (unsupported client/server, permission or transport failure)")
	}
	// Validate the exact identity before spending a second request. An absent
	// reference map needs no list and must not be turned into an empty result.
	probe, err := agent.ParseConfigHubAttestations(origin, revision, []byte("[]"), at)
	if err != nil {
		return nil, err
	}
	if probe.Coverage == "direct-references-unavailable" {
		return probe, nil
	}
	list, err := configHubAttestationsRead(ctx, "attestation", "list", "--space", origin.SpaceID, "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("attestation list unavailable (unsupported client/server, permission or transport failure)")
	}
	return agent.ParseConfigHubAttestations(origin, revision, list, at)
}

func validAttestationOrigin(origin agent.ConfigHubOriginEvidence) bool {
	valid := regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`).MatchString
	return valid(origin.SpaceID) && valid(origin.UnitSlug) && origin.RevisionNum != nil && *origin.RevisionNum > 0
}

func attachConfigHubAttestations(ctx context.Context, out *agent.TraceDeliveryEvidence, origin *agent.ConfigHubOriginEvidence, flagSpace string) {
	if out == nil {
		return
	}
	omit := func(reason string) {
		out.Omissions = append(out.Omissions, agent.TraceDeliveryOmission{Layer: "confighub.attestations", Reason: reason, Impact: "direct revision claims omitted; runtime verdict unchanged"})
	}
	if origin == nil || !validAttestationOrigin(*origin) {
		omit("Live object has no unambiguous combined ConfigHub origin with a positive revision.")
		return
	}
	if flagSpace != "" && flagSpace != origin.SpaceID && flagSpace != origin.SpaceSlug {
		omit("Selected ConfigHub space conflicts with the observed origin.")
		return
	}
	evidence, err := collectConfigHubAttestations(ctx, *origin, out.ObservedAt)
	if err != nil {
		omit(err.Error())
		return
	}
	out.Attestations = evidence
}

func formatConfigHubAttestations(e *agent.ConfigHubAttestationsEvidence) string {
	parts := []string{fmt.Sprintf("unit=%s rev=%d direct-claims=%d", e.UnitSlug, e.RevisionNum, len(e.Attestations))}
	for _, a := range e.Attestations {
		state := fmt.Sprintf("%s:%s id=%s user=%s expired=%t", safeAttestationText(a.Type), a.Result, safeAttestationText(a.ID), safeAttestationText(a.UserID), a.Expired)
		if a.Revoked != nil && *a.Revoked {
			state += " revoked=observed"
		} else {
			state += " revoked=unknown"
		}
		parts = append(parts, state)
	}
	for _, o := range e.Omissions {
		parts = append(parts, "omission="+o.Reason)
	}
	return strings.Join(parts, "; ")
}

func safeAttestationText(s string) string {
	for _, r := range s {
		if unicode.IsControl(r) {
			return strconv.QuoteToASCII(s)
		}
	}
	return s
}

type attestationLimitedBuffer struct{ bytes.Buffer }

func (b *attestationLimitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1024*1024 {
		return 0, fmt.Errorf("ConfigHub read exceeds 1 MiB")
	}
	return b.Buffer.Write(p)
}

func boundedAttestationCubRead(ctx context.Context, args ...string) ([]byte, error) {
	if len(args) < 2 || !((args[0] == "revision" && (args[1] == "get" || args[1] == "data")) || (args[0] == "attestation" && args[1] == "list")) {
		return nil, fmt.Errorf("attestation evidence permits only revision get/data and attestation list")
	}
	cmd, err := cubCommand(ctx, args...)
	if err != nil {
		return nil, err
	}
	var stdout, stderr attestationLimitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		return nil, fmt.Errorf("bounded ConfigHub read failed")
	}
	return stdout.Bytes(), nil
}

func collectReceiptConfigHubRevisionData(ctx context.Context, e *agent.ConfigHubAttestationsEvidence) ([]byte, []byte, error) {
	if e == nil || e.RevisionNum <= 0 || e.SpaceID == "" || e.UnitSlug == "" || e.RevisionID == "" || e.UnitID == "" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(e.DataHash) {
		return nil, nil, fmt.Errorf("exact revision identity and a SHA-256 DataHash are required")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	data, err := configHubAttestationsRead(ctx, "revision", "data", e.UnitSlug, strconv.FormatInt(e.RevisionNum, 10), "--space", e.SpaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("served revision data unavailable")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != e.DataHash {
		return nil, nil, fmt.Errorf("served revision bytes do not match reported DataHash")
	}
	// Reuse the existing bounded, duplicate-rejecting manifest parser. The
	// canonical body preserves document order and pins the immutable identity.
	snapshot, err := loadRecordedObjectSnapshot(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("served revision data has no supported unambiguous Kubernetes manifest representation")
	}
	objects := make([]map[string]interface{}, 0, len(snapshot.Objects))
	for _, obj := range snapshot.Objects {
		objects = append(objects, obj.Object)
	}
	canonical, err := agent.CanonicalJSON(map[string]interface{}{"schema": "confighub-unit-canonical.v1", "spaceId": e.SpaceID, "unitId": e.UnitID, "revisionId": e.RevisionID, "objects": objects})
	if err != nil {
		return nil, nil, fmt.Errorf("revision data cannot be canonicalized")
	}
	return canonical, data, nil
}

func configHubAttestationsMCPTool() mcpTool {
	return mcpTool{
		Descriptor: mcpToolDescriptor{Name: "confighub_attestations", Description: "Connected-only direct ConfigHub revision claims. Requires exact space ID, unit slug and revision from observed origin. Shows observed expiry and revocations; cannot evaluate approval or inherited coverage. Does not establish runtime health.", Annotations: &mcpToolAnnotations{ReadOnlyHint: true}, InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{
				"space_id": map[string]interface{}{"type": "string"},
				"unit":     map[string]interface{}{"type": "string"},
				"unit_id":  map[string]interface{}{"type": "string"},
				"revision": map[string]interface{}{"type": "integer", "minimum": 1},
			}, "required": []string{"space_id", "unit", "revision"}, "additionalProperties": false,
		}},
		BuildArgs: func(arguments map[string]interface{}) ([]string, error) {
			revision, present, err := argIntOpt(arguments, "revision", false)
			if err != nil || !present || revision <= 0 {
				return nil, fmt.Errorf("revision must be a positive integer")
			}
			n := int64(revision)
			origin := agent.ConfigHubOriginEvidence{SpaceID: argString(arguments, "space_id"), UnitSlug: argString(arguments, "unit"), UnitID: argString(arguments, "unit_id"), RevisionNum: &n}
			if !validAttestationOrigin(origin) {
				return nil, fmt.Errorf("exact space_id, unit and revision are required")
			}
			return []string{origin.SpaceID, origin.UnitSlug, strconv.FormatInt(n, 10), origin.UnitID}, nil
		},
		Runner: func(ctx context.Context, args []string) (string, error) {
			if len(args) != 4 {
				return "", fmt.Errorf("invalid attestation read arguments")
			}
			n, err := strconv.ParseInt(args[2], 10, 64)
			if err != nil {
				return "", err
			}
			out, err := collectConfigHubAttestations(ctx, agent.ConfigHubOriginEvidence{SpaceID: args[0], UnitSlug: args[1], RevisionNum: &n, UnitID: args[3]}, gitopsNowFn().UTC())
			if err != nil {
				return "", err
			}
			raw, err := json.Marshal(out)
			return string(raw), err
		},
	}
}
