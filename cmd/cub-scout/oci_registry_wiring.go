// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"github.com/confighub/cub-scout/pkg/agent"
	"github.com/confighub/cub-scout/pkg/hub"
)

// A ConfigHub OCI source is recognised by its registry, and only the server
// knows where its registry is: the hosted service serves bundles from
// `oci.<instance>`, but a self-hosted or local one serves them from wherever it
// was configured, with no prefix to key off. hub.OCIRegistry asks it, at the
// unauthenticated /api/info, and keeps the answer.
//
// The wiring lives here rather than in pkg/agent so that a parser stays a
// parser: nothing in pkg/agent reaches out to a server on its own.
func init() {
	agent.ConfigHubOCIRegistryFn = hub.OCIRegistry
}
