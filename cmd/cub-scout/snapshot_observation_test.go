// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestBuildSnapshotObservation(t *testing.T) {
	observedAt := time.Date(2026, 9, 10, 14, 0, 0, 0, time.UTC)

	observation := buildSnapshotObservation("kind-dev", "prod", "Deployment", observedAt)
	if observation == nil {
		t.Fatal("observation missing")
	}
	if observation.Source != agent.ObservationSourceKubernetesAPI {
		t.Fatalf("source = %q, want kubernetes-api", observation.Source)
	}
	if observation.Mode != agent.ObservationModeSnapshot {
		t.Fatalf("mode = %q, want snapshot", observation.Mode)
	}
	if !observation.ObservedAt.Equal(observedAt) {
		t.Fatalf("observedAt = %s, want %s", observation.ObservedAt, observedAt)
	}
	if observation.Freshness != agent.ObservationFreshnessPointInTime {
		t.Fatalf("freshness = %q, want point-in-time", observation.Freshness)
	}
	if observation.Scope == nil {
		t.Fatal("scope missing")
	}
	if observation.Scope.Cluster != "kind-dev" || observation.Scope.Namespace != "prod" || observation.Scope.Kind != "Deployment" {
		t.Fatalf("scope = %+v, want cluster/namespace/kind", observation.Scope)
	}
}
