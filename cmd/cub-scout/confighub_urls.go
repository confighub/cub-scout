// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"github.com/confighub/cub-scout/pkg/hub"
)

func configHubUnitDetailURL(spaceID, unitID string) string {
	spaceID = strings.TrimSpace(spaceID)
	unitID = strings.TrimSpace(unitID)
	if spaceID == "" || unitID == "" {
		return ""
	}
	return fmt.Sprintf("%s/units/%s/%s", strings.TrimRight(hub.WebBaseURL, "/"), spaceID, unitID)
}

func configHubUnitRevisionsURL(spaceID, unitID string) string {
	spaceID = strings.TrimSpace(spaceID)
	unitID = strings.TrimSpace(unitID)
	if spaceID == "" || unitID == "" {
		return ""
	}
	return fmt.Sprintf("%s/units/%s/%s?tab=2", strings.TrimRight(hub.WebBaseURL, "/"), spaceID, unitID)
}
