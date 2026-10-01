// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestConvertParsedArgoApplicationPreservesGitSource(t *testing.T) {
	// This is the source link parseAppOutput builds from Application
	// spec.source; it is synthetic trace metadata, not a Kubernetes object
	// with labels that could support ownership-label evidence.
	result := &agent.TraceResult{
		Tool: "argocd",
		Chain: []agent.ChainLink{
			{
				Kind: "Source", Name: "team/repo", URL: "https://git.example.invalid/team/repo.git",
				Revision: "main@sha1:abc123", Ready: true,
			},
			{Kind: "Application", Name: "api", Namespace: "delivery", Ready: true},
		},
	}

	output := convertTraceToV014(result, "Application", "api", "delivery", nil)
	require.Len(t, output.Chain, 2)
	source := output.Chain[0]
	require.Equal(t, "source", source.Role)
	require.Equal(t, "source", source.DeliveryStage)
	require.Equal(t, "team/repo", source.ID.Name)
	require.Empty(t, source.Evidence)
	for _, evidence := range source.Evidence {
		require.NotEqual(t, "argocd.argoproj.io/instance", evidence.Key)
	}
	require.NotNil(t, output.Summary.Source)
	require.Equal(t, "https://git.example.invalid/team/repo.git", output.Summary.Source.URL)
	require.Equal(t, "main@sha1:abc123", output.Summary.Source.Revision)
}
