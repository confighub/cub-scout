// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestAnnotateMapEntriesObservation(t *testing.T) {
	observedAt := time.Date(2026, 9, 10, 13, 45, 0, 0, time.UTC)
	observation := agent.NewObservationEvidence(
		agent.ObservationSourceKubernetesAPI,
		agent.ObservationModeMapList,
		observedAt,
		agent.ObservationScope{Cluster: "kind-dev", Namespace: "prod", Kind: "Deployment"},
	)
	entries := []MapEntry{
		{ID: "one", Namespace: "prod", Kind: "Deployment", Name: "api"},
		{ID: "two", Namespace: "prod", Kind: "Service", Name: "api"},
	}

	annotateMapEntriesObservation(entries, observation)

	for _, entry := range entries {
		if entry.Observation == nil {
			t.Fatalf("entry %s observation missing", entry.ID)
		}
		if entry.Observation.Source != agent.ObservationSourceKubernetesAPI {
			t.Fatalf("entry %s source = %q, want kubernetes-api", entry.ID, entry.Observation.Source)
		}
		if entry.Observation.Mode != agent.ObservationModeMapList {
			t.Fatalf("entry %s mode = %q, want map-list", entry.ID, entry.Observation.Mode)
		}
		if !entry.Observation.ObservedAt.Equal(observedAt) {
			t.Fatalf("entry %s observedAt = %s, want %s", entry.ID, entry.Observation.ObservedAt, observedAt)
		}
		if entry.Observation.Scope == nil || entry.Observation.Scope.Cluster != "kind-dev" || entry.Observation.Scope.Namespace != "prod" || entry.Observation.Scope.Kind != "Deployment" {
			t.Fatalf("entry %s scope = %+v, want map list scope", entry.ID, entry.Observation.Scope)
		}
	}
}
