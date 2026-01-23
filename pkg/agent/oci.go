// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
)

// OCISourceInfo contains parsed information from an OCI registry URL
type OCISourceInfo struct {
	// Raw is the full OCI URL
	Raw string

	// IsConfigHub indicates if this is a ConfigHub OCI registry
	IsConfigHub bool

	// Instance is the ConfigHub instance host (e.g., "api.confighub.com")
	Instance string

	// Space is the ConfigHub space name
	Space string

	// Target is the ConfigHub target name
	Target string

	// Registry is the OCI registry host
	Registry string

	// Repository is the OCI repository path
	Repository string
}

// ParseOCISource parses an OCI URL and extracts information
// Handles both generic OCI URLs and ConfigHub-specific OCI registry URLs
//
// ConfigHub OCI URL format: oci://oci.{instance}/target/{space}/{target}
// Example: oci://oci.api.confighub.com/target/prod/us-west
func ParseOCISource(url string) OCISourceInfo {
	info := OCISourceInfo{
		Raw: url,
	}

	// Must start with oci://
	if !strings.HasPrefix(url, "oci://") {
		return info
	}

	// Remove oci:// prefix
	remainder := strings.TrimPrefix(url, "oci://")

	// Split into registry and repository
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) < 1 {
		return info
	}

	info.Registry = parts[0]
	if len(parts) > 1 {
		info.Repository = parts[1]
	}

	// Check if this is a ConfigHub OCI registry
	// ConfigHub OCI registry format: oci.{instance-host}
	if strings.HasPrefix(info.Registry, "oci.") {
		info.IsConfigHub = true
		info.Instance = strings.TrimPrefix(info.Registry, "oci.")

		// Parse ConfigHub repository format: target/{space}/{target}
		if strings.HasPrefix(info.Repository, "target/") {
			repoPath := strings.TrimPrefix(info.Repository, "target/")
			targetParts := strings.SplitN(repoPath, "/", 2)
			if len(targetParts) >= 1 {
				info.Space = targetParts[0]
			}
			if len(targetParts) >= 2 {
				info.Target = targetParts[1]
			}
		}
	}

	return info
}

// IsConfigHubOCI checks if a URL is a ConfigHub OCI registry URL
func IsConfigHubOCI(url string) bool {
	if !strings.HasPrefix(url, "oci://") {
		return false
	}

	remainder := strings.TrimPrefix(url, "oci://")
	parts := strings.SplitN(remainder, "/", 2)
	if len(parts) < 1 {
		return false
	}

	registry := parts[0]
	return strings.HasPrefix(registry, "oci.") && len(parts) > 1 && strings.HasPrefix(parts[1], "target/")
}

// FormatConfigHubOCISource formats a ConfigHub OCI source for display
func FormatConfigHubOCISource(info OCISourceInfo) string {
	if !info.IsConfigHub {
		return info.Raw
	}

	if info.Space != "" && info.Target != "" {
		return info.Space + "/" + info.Target
	}

	return info.Raw
}
