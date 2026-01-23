// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package hierarchysvc provides utility functions for cluster hierarchy and grouping.
// It separates reusable business logic from the TUI rendering code.
package hierarchysvc

import (
	"strings"
)

// ExtractClusterName extracts a canonical cluster name from various context formats.
// Handles AWS EKS ARNs, GKE context formats, kind clusters, and returns the input as-is for unknown formats.
func ExtractClusterName(contextName string) string {
	// Handle empty/unknown
	if contextName == "" || contextName == "unknown" {
		return contextName
	}

	// AWS EKS: arn:aws:eks:region:account:cluster/name
	if strings.HasPrefix(contextName, "arn:aws:eks:") {
		if idx := strings.LastIndex(contextName, "/"); idx != -1 {
			return contextName[idx+1:]
		}
	}

	// GKE: gke_project_zone_cluster
	if strings.HasPrefix(contextName, "gke_") {
		parts := strings.Split(contextName, "_")
		if len(parts) >= 4 {
			return parts[len(parts)-1]
		}
	}

	// kind: kind-name
	if strings.HasPrefix(contextName, "kind-") {
		return strings.TrimPrefix(contextName, "kind-")
	}

	// Default: use the context name as-is
	return contextName
}

// MatchesCluster checks if a target cluster matches the current cluster.
// Supports exact match and partial/case-insensitive matching for different naming conventions.
func MatchesCluster(targetCluster, currentCluster string) bool {
	if targetCluster == "" || currentCluster == "" {
		return false
	}

	// Exact match
	if targetCluster == currentCluster {
		return true
	}

	// Partial match (for different naming conventions)
	if strings.Contains(strings.ToLower(targetCluster), strings.ToLower(currentCluster)) {
		return true
	}
	if strings.Contains(strings.ToLower(currentCluster), strings.ToLower(targetCluster)) {
		return true
	}

	return false
}
