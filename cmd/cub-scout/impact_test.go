package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildImpactResult(t *testing.T) {
	cache := &cubUnitCache{
		units: map[string]*cubUnitInfo{
			"shared-db-config": {
				Slug:       "shared-db-config",
				DependedBy: []string{"api-gateway", "worker-service"},
			},
			"api-gateway": {
				Slug:       "api-gateway",
				Space:      "payments-prod",
				TargetSlug: "prod-east",
				OtherSpaces: []string{
					"payments-staging",
				},
			},
			"worker-service": {
				Slug:       "worker-service",
				Space:      "payments-prod",
				TargetSlug: "prod-west",
			},
		},
	}

	got, err := buildImpactResult(cache, "shared-db-config")
	if err != nil {
		t.Fatalf("buildImpactResult: %v", err)
	}
	if got.Unit != "shared-db-config" {
		t.Fatalf("unit = %q, want shared-db-config", got.Unit)
	}
	if got.Summary.DirectDependents != 2 {
		t.Fatalf("direct dependents = %d, want 2", got.Summary.DirectDependents)
	}
	if got.Summary.EnvironmentsAffected != 2 {
		t.Fatalf("environments affected = %d, want 2", got.Summary.EnvironmentsAffected)
	}
	if got.Summary.ClustersAffected != 2 {
		t.Fatalf("clusters affected = %d, want 2", got.Summary.ClustersAffected)
	}
	if got.Summary.Risk != "HIGH" {
		t.Fatalf("risk = %q, want HIGH", got.Summary.Risk)
	}
}

func TestRunImpactASCII(t *testing.T) {
	restoreFlags := setImpactFlagState(impactFlagState{
		format: "ascii",
	})
	defer restoreFlags()

	restoreLoader := loadImpactUnitCacheFn
	loadImpactUnitCacheFn = func() (*cubUnitCache, error) {
		return &cubUnitCache{
			units: map[string]*cubUnitInfo{
				"shared-db-config": {
					Slug:       "shared-db-config",
					DependedBy: []string{"api-gateway"},
				},
				"api-gateway": {
					Slug:       "api-gateway",
					Space:      "payments-prod",
					TargetSlug: "prod-east",
				},
			},
		}, nil
	}
	defer func() { loadImpactUnitCacheFn = restoreLoader }()

	out := captureStdout(t, func() {
		if err := runImpact(&cobra.Command{}, []string{"unit/shared-db-config"}); err != nil {
			t.Fatalf("runImpact: %v", err)
		}
	})

	for _, needle := range []string{
		"Impact Preview: unit/shared-db-config",
		"Direct dependents: 1 unit(s)",
		"Risk: HIGH",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("output missing %q:\n%s", needle, out)
		}
	}
}

func TestRunImpactNotConnected(t *testing.T) {
	restoreFlags := setImpactFlagState(impactFlagState{
		format: "ascii",
	})
	defer restoreFlags()

	restoreLoader := loadImpactUnitCacheFn
	loadImpactUnitCacheFn = func() (*cubUnitCache, error) {
		return nil, errImpactNotConnected
	}
	defer func() { loadImpactUnitCacheFn = restoreLoader }()

	err := runImpact(&cobra.Command{}, []string{"shared-db-config"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cub auth login") {
		t.Fatalf("expected auth guidance, got: %v", err)
	}
}

type impactFlagState struct {
	format string
	json   bool
}

func setImpactFlagState(state impactFlagState) func() {
	prevFormat := impactFormat
	prevJSON := impactJSON
	impactFormat = state.format
	impactJSON = state.json
	return func() {
		impactFormat = prevFormat
		impactJSON = prevJSON
	}
}
