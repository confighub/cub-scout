// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCompleteOwnersIncludesCrossplaneAndTerraform(t *testing.T) {
	cmd := &cobra.Command{}
	owners, _ := completeOwners(cmd, nil, "")

	foundTerraform := false
	foundCrossplane := false
	for _, o := range owners {
		if o == "Terraform" {
			foundTerraform = true
		}
		if o == "Crossplane" {
			foundCrossplane = true
		}
	}
	if !foundTerraform {
		t.Fatalf("expected Terraform in owner completions, got: %v", owners)
	}
	if !foundCrossplane {
		t.Fatalf("expected Crossplane in owner completions, got: %v", owners)
	}
}
