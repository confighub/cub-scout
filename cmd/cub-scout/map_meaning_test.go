// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildMeaningGroups_Deterministic(t *testing.T) {
	entries := []MapEntry{
		{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Owner: "Flux", Labels: map[string]string{"app": "payments"}},
		{Kind: "Service", Name: "payments-api", Namespace: "prod", Owner: "Flux", Labels: map[string]string{"app": "payments"}},
		{Kind: "Deployment", Name: "inventory-api", Namespace: "prod", Owner: "ArgoCD", Labels: map[string]string{"app": "inventory"}},
	}

	first, err := json.Marshal(buildMeaningGroups(entries))
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	second, err := json.Marshal(buildMeaningGroups(entries))
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("buildMeaningGroups not deterministic\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestBuildMeaningGroups_ComparativeLabels(t *testing.T) {
	entries := []MapEntry{
		{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Owner: "Flux", Labels: map[string]string{"app": "payments"}},
		{Kind: "Service", Name: "payments", Namespace: "prod", Owner: "Flux", Labels: map[string]string{"app": "payments"}},
		{Kind: "Deployment", Name: "inventory-api", Namespace: "prod", Owner: "Flux", Labels: map[string]string{"app": "inventory"}},
		{Kind: "Service", Name: "inventory", Namespace: "prod", Owner: "Flux", Labels: map[string]string{"app": "inventory"}},
	}

	groups := buildMeaningGroups(entries)
	if len(groups) < 2 {
		t.Fatalf("expected at least 2 groups, got %d", len(groups))
	}

	labels := []string{groups[0].Label, groups[1].Label}
	joined := strings.ToLower(strings.Join(labels, " "))
	if !strings.Contains(joined, "payments") {
		t.Fatalf("expected comparative label to include payments token, labels=%v", labels)
	}
	if !strings.Contains(joined, "inventory") {
		t.Fatalf("expected comparative label to include inventory token, labels=%v", labels)
	}
}

func TestRenderMeaningASCII_IncludesEvidence(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	groups := []meaningGroup{
		{
			Label:    "payments · flux",
			Evidence: []string{"owner=flux", "namespace=prod", "distinctive=payments"},
			Members: []MapEntry{
				{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Owner: "Flux"},
			},
		},
	}

	renderMeaningASCII(cmd, groups, 1)
	s := out.String()
	if !strings.Contains(s, "Meaning Groups (experimental)") {
		t.Fatalf("missing heading: %q", s)
	}
	if !strings.Contains(s, "evidence: owner=flux") {
		t.Fatalf("missing evidence line: %q", s)
	}
	if !strings.Contains(s, "Deployment/payments-api") {
		t.Fatalf("missing member row: %q", s)
	}
}
