// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"
)

func TestNewObservationEvidence_NormalizesScopeAndTime(t *testing.T) {
	observedAt := time.Date(2026, 9, 10, 14, 30, 0, 0, time.FixedZone("BST", 3600))

	evidence := NewObservationEvidence(
		" kubernetes-api ",
		" watch-poll ",
		observedAt,
		ObservationScope{Cluster: " kind-dev ", Namespace: " prod ", Kind: " Deployment "},
	)
	if evidence == nil {
		t.Fatal("expected observation evidence")
	}
	if evidence.Source != ObservationSourceKubernetesAPI {
		t.Fatalf("source = %q, want kubernetes-api", evidence.Source)
	}
	if evidence.Mode != ObservationModeWatchPoll {
		t.Fatalf("mode = %q, want watch-poll", evidence.Mode)
	}
	wantTime := time.Date(2026, 9, 10, 13, 30, 0, 0, time.UTC)
	if !evidence.ObservedAt.Equal(wantTime) {
		t.Fatalf("observedAt = %s, want %s", evidence.ObservedAt.Format(time.RFC3339), wantTime.Format(time.RFC3339))
	}
	if evidence.Freshness != ObservationFreshnessPointInTime {
		t.Fatalf("freshness = %q, want point-in-time", evidence.Freshness)
	}
	if evidence.Scope == nil {
		t.Fatal("scope missing")
	}
	if evidence.Scope.Cluster != "kind-dev" || evidence.Scope.Namespace != "prod" || evidence.Scope.Kind != "Deployment" {
		t.Fatalf("scope = %+v, want trimmed cluster/namespace/kind", evidence.Scope)
	}
}

func TestNewObservationEvidence_OmitsEmptyScope(t *testing.T) {
	evidence := NewObservationEvidence(
		ObservationSourceKubernetesAPI,
		ObservationModeSnapshot,
		time.Date(2026, 9, 10, 13, 30, 0, 0, time.UTC),
		ObservationScope{},
	)
	if evidence == nil {
		t.Fatal("expected observation evidence")
	}
	if evidence.Scope != nil {
		t.Fatalf("scope = %+v, want nil", evidence.Scope)
	}
}

func TestNewObservationEvidence_RequiresSourceModeAndTime(t *testing.T) {
	now := time.Date(2026, 9, 10, 13, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		source   string
		mode     string
		observed time.Time
	}{
		{name: "missing source", source: "", mode: ObservationModeMapList, observed: now},
		{name: "missing mode", source: ObservationSourceKubernetesAPI, mode: "", observed: now},
		{name: "missing time", source: ObservationSourceKubernetesAPI, mode: ObservationModeMapList},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if evidence := NewObservationEvidence(tc.source, tc.mode, tc.observed, ObservationScope{}); evidence != nil {
				t.Fatalf("evidence = %+v, want nil", evidence)
			}
		})
	}
}
