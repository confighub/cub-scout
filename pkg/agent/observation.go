// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"time"
)

const (
	ObservationSourceKubernetesAPI = "kubernetes-api"
	ObservationSourceSummaryStore  = "summary-store"

	ObservationModeMapList      = "map-list"
	ObservationModeSnapshot     = "snapshot"
	ObservationModeSummary      = "summary-list"
	ObservationModeWatchPoll    = "watch-poll"
	ObservationModeWatchInformer = "watch-informer"

	ObservationFreshnessPointInTime = "point-in-time"
)

// ObservationEvidence describes the read source and freshness boundary for
// an observed fact. It is evidence about how cub-scout collected data, not a
// claim that the fact remains true after observedAt.
type ObservationEvidence struct {
	Source     string            `json:"source"`
	Mode       string            `json:"mode"`
	ObservedAt time.Time         `json:"observedAt"`
	Freshness  string            `json:"freshness"`
	Scope      *ObservationScope `json:"scope,omitempty"`
}

type ObservationScope struct {
	Cluster   string `json:"cluster,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind,omitempty"`
}

func NewObservationEvidence(source, mode string, observedAt time.Time, scope ObservationScope) *ObservationEvidence {
	source = strings.TrimSpace(source)
	mode = strings.TrimSpace(mode)
	if source == "" || mode == "" || observedAt.IsZero() {
		return nil
	}
	return &ObservationEvidence{
		Source:     source,
		Mode:       mode,
		ObservedAt: observedAt.UTC(),
		Freshness:  ObservationFreshnessPointInTime,
		Scope:      normalizeObservationScope(scope),
	}
}

func normalizeObservationScope(scope ObservationScope) *ObservationScope {
	scope.Cluster = strings.TrimSpace(scope.Cluster)
	scope.Namespace = strings.TrimSpace(scope.Namespace)
	scope.Kind = strings.TrimSpace(scope.Kind)
	if scope.Cluster == "" && scope.Namespace == "" && scope.Kind == "" {
		return nil
	}
	return &scope
}
