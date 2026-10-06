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
	"k8s.io/client-go/rest"
)

type mapClusterIdentityOutput struct {
	Schema           string                        `json:"schema"`
	Cluster          agent.ClusterIdentityEvidence `json:"cluster"`
	ClusterCostScope string                        `json:"clusterCostScope"`
	Resources        []MapEntry                    `json:"resources"`
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
