// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

type mapClusterIdentityOutput struct {
	Schema           string                        `json:"schema"`
	Cluster          agent.ClusterIdentityEvidence `json:"cluster"`
	ClusterCostScope string                        `json:"clusterCostScope"`
	Resources        []mapClusterIdentityResource  `json:"resources"`
	Collection       mapClusterIdentityCollection  `json:"collection"`
}

type mapClusterIdentityCollection struct {
	Status            string                      `json:"status"`
	Omissions         []mapsvc.CollectionOmission `json:"omissions"`
	UnavailableReason string                      `json:"unavailableReason,omitempty"`
}

func mapClusterCollection(omissions []mapsvc.CollectionOmission, unavailableReason string) mapClusterIdentityCollection {
	known := mapsvc.BuildOwnershipEvidenceOutput(nil, omissions).Collection
	if known.Omissions == nil {
		known.Omissions = []mapsvc.CollectionOmission{}
	}
	result := mapClusterIdentityCollection{Status: known.Status, Omissions: known.Omissions}
	if unavailableReason != "" {
		result.Status, result.UnavailableReason = "unavailable", unavailableReason
	}
	return result
}

func unavailableMapClusterIdentity(label, reason string) *agent.ClusterIdentityEvidence {
	if len(label) > 512 || !utf8.ValidString(label) {
		label = ""
	}
	return &agent.ClusterIdentityEvidence{Context: label, Identity: "unverified", IDSource: "v1/Namespace/kube-system", Omission: reason, Cost: agent.KubernetesReadCost{Coverage: "unavailable"}}
}

func mapClusterIdentityRequested(cmd *cobra.Command) bool {
	if cmd.Flags().Lookup("cluster-identity") == nil {
		return false
	}
	requested, _ := cmd.Flags().GetBool("cluster-identity")
	return requested
}

func validateMapClusterIdentityOptions() error {
	if mapSummary || mapCount || mapNamesOnly || mapOwnershipEvidence {
		return fmt.Errorf("--cluster-identity cannot be combined with --summary, --count, --names-only, or --ownership-evidence")
	}
	if mapListFormat != "ascii" && mapListFormat != "json" && mapListFormat != "md" {
		return fmt.Errorf("cluster identity format must be ascii, json, or md")
	}
	return nil
}

func observeMapClusterIdentity(ctx context.Context, cfg *rest.Config, label string) *agent.ClusterIdentityEvidence {
	reader, err := agent.NewClusterIdentityReader(cfg, label)
	if err != nil {
		// A constructor failure must not discard successfully read inventory or
		// disclose transport/authentication errors. No identity request occurred.
		return unavailableMapClusterIdentity(label, "identity_client_unavailable")
	}
	evidence := reader.Read(ctx)
	return &evidence
}

// Quotes escape terminal controls and preserve identities without abbreviation.
// Values cannot introduce additional lines or Markdown fence boundaries.
func mapClusterIdentityText(evidence *agent.ClusterIdentityEvidence) string {
	if evidence == nil {
		return ""
	}
	observed := "unknown"
	if evidence.ObservedAt != nil {
		observed = evidence.ObservedAt.UTC().Format(time.RFC3339Nano)
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Cluster identity: %q (Namespace instance; not a Target binding)\n", evidence.Identity)
	fmt.Fprintf(&text, "Context: %q\nAPI server: %q\nID source: %q\nID: %q\nObserved at: %q\n", evidence.Context, evidence.APIServer, evidence.IDSource, evidence.ID, observed)
	if evidence.Omission != "" {
		fmt.Fprintf(&text, "Identity omission: %q\n", evidence.Omission)
	}
	fmt.Fprintf(&text, "Identity read cost: requests=%d consumed-body-bytes=%d duration-ms=%d reused=%t transport-errors=%d body-read-errors=%d coverage=%q\n", evidence.Cost.RequestsMade, evidence.Cost.ResponseBodyBytes, evidence.Cost.DurationMillis, evidence.Cost.Reused, evidence.Cost.TransportErrors, evidence.Cost.BodyReadErrors, evidence.Cost.Coverage)
	text.WriteString("Cost scope: identity reader only; inventory/authentication/other clients excluded.\n")
	return text.String()
}

var mapIdentityKnownKinds = mapKnownResourceKinds()

// Scope comes from the pinned resource registry, never metadata.namespace.
func mapResourceIdentityScope(gvr schema.GroupVersionResource) (agent.ObservedResourceScope, bool) {
	for _, known := range defaultMapWatchResources {
		if known == gvr {
			return agent.ObservedResourceNamespaced, true
		}
	}
	for _, spec := range firstClassControllerResources() {
		if spec.GVR == gvr {
			if spec.Namespaced {
				return agent.ObservedResourceNamespaced, true
			}
			return agent.ObservedResourceCluster, true
		}
	}
	return "", false
}

func processResourceWithIdentity(item *unstructured.Unstructured, gvr schema.GroupVersionResource, clusterName string, entries []MapEntry, byOwner map[string]int, lookup mapApplicationSetLookup, cluster *agent.ClusterIdentityEvidence) []MapEntry {
	before := len(entries)
	entries = processResourceWithLookup(item, gvr, clusterName, entries, byOwner, lookup)
	if cluster == nil || len(entries) != before+1 {
		return entries
	}
	entry := &entries[before]
	entry.ResourceIdentityOmission = "object_identity_unavailable"
	if cluster.Identity != "verified" || cluster.Omission != "" {
		entry.ResourceIdentityOmission = "cluster_identity_unverified"
		return entries
	}
	scope, known := mapResourceIdentityScope(gvr)
	if !known {
		entry.ResourceIdentityOmission = "resource_scope_unknown"
		return entries
	}
	if item.GetAPIVersion() != gvr.GroupVersion().String() || item.GetKind() != mapIdentityKnownKinds[gvr] {
		entry.ResourceIdentityOmission = "resource_type_mismatch"
		return entries
	}
	identity, err := agent.NewObservedResourceIdentity(*cluster, item, scope)
	if err != nil {
		return entries
	}
	entry.ResourceIdentity, entry.ResourceIdentityOmission = &identity, ""
	return entries
}

type mapResourceIdentityEvidence struct {
	Status   string                          `json:"status"`
	Observed *agent.ObservedResourceIdentity `json:"observed,omitempty"`
	MergeKey string                          `json:"mergeKey,omitempty"`
	Omission string                          `json:"omission,omitempty"`
}

type mapClusterIdentityResource struct {
	MapEntry
	ResourceIdentity mapResourceIdentityEvidence `json:"resourceIdentity"`
}

func mapEntryIdentityEvidence(entry MapEntry, cluster *agent.ClusterIdentityEvidence) mapResourceIdentityEvidence {
	if cluster == nil || cluster.Identity != "verified" || cluster.ObservedAt == nil || cluster.ObservedAt.IsZero() || cluster.Omission != "" {
		return mapResourceIdentityEvidence{Status: "unverified", Omission: "cluster_identity_unverified"}
	}
	if entry.ResourceIdentity != nil && (entry.ResourceIdentity.ClusterID != cluster.ID || entry.ResourceIdentity.ClusterIDSource != cluster.IDSource) {
		return mapResourceIdentityEvidence{Status: "unverified", Omission: "resource_cluster_mismatch"}
	}
	if entry.ResourceIdentity != nil {
		if key, err := entry.ResourceIdentity.MergeKey(); err == nil {
			return mapResourceIdentityEvidence{Status: "verified", Observed: entry.ResourceIdentity, MergeKey: key}
		}
	}
	reason := entry.ResourceIdentityOmission
	if reason == "" {
		reason = "object_identity_not_collected"
	}
	return mapResourceIdentityEvidence{Status: "unverified", Omission: reason}
}

func mapIdentityResources(entries []MapEntry, cluster *agent.ClusterIdentityEvidence) []mapClusterIdentityResource {
	result := make([]mapClusterIdentityResource, 0, len(entries))
	for _, entry := range entries {
		result = append(result, mapClusterIdentityResource{MapEntry: entry, ResourceIdentity: mapEntryIdentityEvidence(entry, cluster)})
	}
	return result
}

func mapResourceIdentityText(entry MapEntry, cluster *agent.ClusterIdentityEvidence) string {
	evidence := mapEntryIdentityEvidence(entry, cluster)
	if evidence.Observed != nil {
		return fmt.Sprintf("Object identity: %q %q/%q %q UID=%q merge-key=%q\n", evidence.Status, entry.Namespace, entry.Name, entry.Kind, evidence.Observed.UID, evidence.MergeKey)
	}
	return fmt.Sprintf("Object identity: %q %q/%q %q omission=%q\n", evidence.Status, entry.Namespace, entry.Name, entry.Kind, evidence.Omission)
}
