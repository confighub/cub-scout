// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

type watchClusterIdentityKey struct{}

func watchClusterIdentityFromContext(ctx context.Context) *agent.ClusterIdentityEvidence {
	identity, _ := ctx.Value(watchClusterIdentityKey{}).(*agent.ClusterIdentityEvidence)
	return identity
}

// Only two verified observations in the same cluster can establish recreation.
// Identity denial is not deletion, and another cluster is not an old instance.
func watchVerifiedRecreation(before, after MapEntry) bool {
	a, b := before.ResourceIdentity, after.ResourceIdentity
	if a == nil || b == nil || a.ClusterID != b.ClusterID || a.ClusterIDSource != b.ClusterIDSource || a.Group != b.Group || a.Kind != b.Kind || a.Namespace != b.Namespace || a.Name != b.Name {
		return false
	}
	_, aerr := a.MergeKey()
	_, berr := b.MergeKey()
	return aerr == nil && berr == nil && a.UID != b.UID
}

func watchIdentityEntry(entries map[string]MapEntry, resource watchEventResource) (MapEntry, string) {
	var found MapEntry
	count := 0
	for _, entry := range entries {
		if entry.Kind == resource.Kind && entry.Name == resource.Name && entry.Namespace == resource.Namespace {
			found = entry
			count++
		}
	}
	if count == 0 {
		return MapEntry{}, "object_identity_not_collected"
	}
	if count != 1 {
		return MapEntry{}, "object_identity_ambiguous"
	}
	return found, ""
}

func attachWatchIdentity(events []watchEvent, prev, curr watchState, ts time.Time) []watchEvent {
	if curr.clusterIdentity == nil {
		return events // No default JSON additions or extra events.
	}
	for i := range events {
		event := &events[i]
		event.Cluster, event.ClusterCostScope = curr.clusterIdentity, "identity-reader"
		if event.Type == "collection.partial" {
			continue
		}
		state := curr
		if event.Type == "resource.deleted" {
			state = prev // Last observed UID/time; never bless it with this cycle's ID.
			event.Cluster = prev.clusterIdentity
		}
		entry, reason := watchIdentityEntry(state.entriesByID, event.Resource)
		evidence := mapEntryIdentityEvidence(entry, state.clusterIdentity)
		if reason != "" {
			evidence = mapResourceIdentityEvidence{Status: "unverified", Omission: reason}
		}
		event.ResourceIdentity = &evidence
	}
	// Always expose the cycle, including empty inventories and denied identity.
	return append(events, watchEvent{Type: "cluster.observed", Timestamp: ts,
		Cluster: curr.clusterIdentity, ClusterCostScope: "identity-reader",
		Resource: watchEventResource{Kind: "Collection", Name: "cluster-identity", Namespace: curr.namespace}})
}
