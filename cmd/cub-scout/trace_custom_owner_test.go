// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestBuildCustomOwnerUnsupportedTraceResult(t *testing.T) {
	result := buildCustomOwnerUnsupportedTraceResult(
		"Deployment",
		"platform-api",
		"default",
		&agent.Ownership{
			Type: agent.OwnerCustom,
			Name: "Internal Platform",
		},
	)

	if result.Object.Kind != "Deployment" || result.Object.Name != "platform-api" || result.Object.Namespace != "default" {
		t.Fatalf("unexpected object ref: %#v", result.Object)
	}
	if !strings.Contains(result.Error, "custom owner detected: Internal Platform") {
		t.Fatalf("expected custom owner in error, got: %q", result.Error)
	}
}

func TestOwnershipLabel_CustomUsesName(t *testing.T) {
	got := ownershipLabel(&agent.Ownership{Type: agent.OwnerCustom, Name: "Pulumi"})
	if got != "Pulumi" {
		t.Fatalf("ownershipLabel(custom) = %q, want %q", got, "Pulumi")
	}
}
