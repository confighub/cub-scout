// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package mapsvc

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestOwnershipEvidenceEnvelopeIsCompactAndDoesNotChangeEntryJSON(t *testing.T) {
	entry := Entry{
		ID: "cluster/ns/deploy/app", ClusterName: "cluster", APIVersion: "apps/v1", Kind: "Deployment",
		Namespace: "ns", Name: "app", Owner: "Native", Labels: map[string]string{"secret": "large"},
		Status: "Ready", CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Observation:        &agent.ObservationEvidence{Source: agent.ObservationSourceKubernetesAPI},
		OwnershipDetection: NewOwnershipDetectionEvidence(agent.Ownership{Type: agent.OwnerUnknown}),
	}
	legacy, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacy), "ownershipDetection") {
		t.Fatalf("legacy Entry JSON changed: %s", legacy)
	}

	envelope := BuildOwnershipEvidenceOutput([]Entry{entry}, []CollectionOmission{{APIVersion: "apps/v1", Resource: "deployments", Namespace: "team", Reason: "forbidden"}})
	if envelope.Schema != "map-list-ownership-evidence.v1" || envelope.Collection.Status != "partial" {
		t.Fatalf("unexpected envelope header: %#v", envelope)
	}
	if len(envelope.Resources) != 1 || envelope.Resources[0].OwnershipDetection.Status != OwnershipDetectionStatusNoKnownMarker {
		t.Fatalf("unexpected resource diagnostic: %#v", envelope.Resources)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{`"labels"`, `"createdAt"`, `"observation"`, `"secret"`, `"Ready"`} {
		if strings.Contains(string(encoded), excluded) {
			t.Errorf("compact projection contains %s: %s", excluded, encoded)
		}
	}
	if !strings.Contains(string(encoded), `"reason":"forbidden"`) {
		t.Fatalf("sanitized omission missing: %s", encoded)
	}
}

func TestOwnershipDetectionDistinguishesKnownKubernetesSourceFromNoMarker(t *testing.T) {
	known := NewOwnershipDetectionEvidence(agent.Ownership{Type: agent.OwnerKubernetes, Source: "ownerRef:Deployment"})
	unknown := NewOwnershipDetectionEvidence(agent.Ownership{Type: agent.OwnerUnknown})
	if known.Status != OwnershipDetectionStatusDetected || known.Source != "ownerRef:Deployment" {
		t.Fatalf("known Kubernetes evidence was lost: %#v", known)
	}
	if unknown.Status != OwnershipDetectionStatusNoKnownMarker || unknown.Source != "" {
		t.Fatalf("unknown detector result was not distinct: %#v", unknown)
	}
}
