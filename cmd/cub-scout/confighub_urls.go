// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"

	"github.com/confighub/cub-scout/pkg/hub"
)

func configHubUnitURL(spaceID, unitSlug string) string {
	spaceID = strings.TrimSpace(spaceID)
	unitSlug = strings.TrimSpace(unitSlug)
	if spaceID == "" || unitSlug == "" {
		return ""
	}
	return strings.TrimRight(hub.WebBaseURL, "/") + "/spaces/" + spaceID + "/units/" + unitSlug
}
