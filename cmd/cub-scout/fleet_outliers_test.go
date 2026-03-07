package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildFleetOutlierReport(t *testing.T) {
	units := []fleetUnitSnapshot{
		{UnitSlug: "api-gateway", Cluster: "us-east-1", Revision: 14},
		{UnitSlug: "api-gateway", Cluster: "us-west-2", Revision: 14},
		{UnitSlug: "api-gateway", Cluster: "eu-west-1", Revision: 12},
		{UnitSlug: "worker-service", Cluster: "us-east-1", Revision: 9},
		{UnitSlug: "worker-service", Cluster: "us-west-2", Revision: 9},
	}

	report, err := buildFleetOutlierReport(units)
	if err != nil {
		t.Fatalf("buildFleetOutlierReport: %v", err)
	}
	if report.Summary.ClusterCount != 3 {
		t.Fatalf("cluster count = %d, want 3", report.Summary.ClusterCount)
	}

	cluster := report.ByCluster["eu-west-1"]
	if cluster == nil {
		t.Fatal("expected eu-west-1 cluster report")
	}
	if len(cluster.Outliers) < 2 {
		t.Fatalf("expected eu-west-1 outliers, got %#v", cluster.Outliers)
	}
}

func TestRunFleetOutliersASCII(t *testing.T) {
	restoreFlags := setFleetOutlierFlagState(fleetOutlierFlagState{
		format: "ascii",
	})
	defer restoreFlags()

	restoreLoader := loadFleetOutlierUnitsFn
	loadFleetOutlierUnitsFn = func() ([]fleetUnitSnapshot, error) {
		return []fleetUnitSnapshot{
			{UnitSlug: "api-gateway", Cluster: "us-east-1", Revision: 14},
			{UnitSlug: "api-gateway", Cluster: "us-west-2", Revision: 12},
		}, nil
	}
	defer func() { loadFleetOutlierUnitsFn = restoreLoader }()

	out := captureStdout(t, func() {
		if err := runFleetOutliers(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runFleetOutliers: %v", err)
		}
	})
	if !strings.Contains(out, "Fleet Outliers") {
		t.Fatalf("expected fleet outlier header:\n%s", out)
	}
	if !strings.Contains(out, "us-west-2") {
		t.Fatalf("expected divergent cluster in output:\n%s", out)
	}
}

func TestRunFleetOutliersNotConnected(t *testing.T) {
	restoreFlags := setFleetOutlierFlagState(fleetOutlierFlagState{
		format: "ascii",
	})
	defer restoreFlags()

	restoreLoader := loadFleetOutlierUnitsFn
	loadFleetOutlierUnitsFn = func() ([]fleetUnitSnapshot, error) {
		return nil, errFleetOutliersNotConnected
	}
	defer func() { loadFleetOutlierUnitsFn = restoreLoader }()

	err := runFleetOutliers(&cobra.Command{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "cub auth login") {
		t.Fatalf("expected auth guidance, got: %v", err)
	}
}

type fleetOutlierFlagState struct {
	format string
	json   bool
}

func setFleetOutlierFlagState(state fleetOutlierFlagState) func() {
	prevFormat := fleetOutliersFormat
	prevJSON := fleetOutliersJSON
	fleetOutliersFormat = state.format
	fleetOutliersJSON = state.json
	return func() {
		fleetOutliersFormat = prevFormat
		fleetOutliersJSON = prevJSON
	}
}
