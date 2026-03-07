package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCompareThreeWayCommandRegistered(t *testing.T) {
	var compare *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "combined" {
			compare = cmd
			break
		}
	}
	if compare == nil {
		t.Fatal("combined command not found")
	}

	found := false
	for _, sub := range compare.Commands() {
		if sub.Name() == "three-way" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("compare three-way subcommand is not registered")
	}
}

func TestParseThreeWayScope(t *testing.T) {
	tests := []struct {
		in      string
		wantTyp threeWayScopeType
		wantVal string
		wantErr bool
	}{
		{in: "deploy/api", wantTyp: threeWayScopeResource, wantVal: "deploy/api"},
		{in: "resource:statefulset/db", wantTyp: threeWayScopeResource, wantVal: "statefulset/db"},
		{in: "namespace/prod", wantTyp: threeWayScopeNamespace, wantVal: "prod"},
		{in: "cluster", wantTyp: threeWayScopeCluster, wantVal: "cluster"},
		{in: "unit/payment-api", wantErr: true},
		{in: "", wantErr: true},
	}

	for _, tt := range tests {
		got, err := parseThreeWayScope(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("parseThreeWayScope(%q) expected error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseThreeWayScope(%q) error = %v", tt.in, err)
		}
		if got.ScopeType != tt.wantTyp || got.ScopeValue != tt.wantVal {
			t.Fatalf("parseThreeWayScope(%q) = %+v, want type=%v value=%q", tt.in, got, tt.wantTyp, tt.wantVal)
		}
	}
}

func TestBuildThreeWayReport_NamespaceScope(t *testing.T) {
	prevDiscoverWorkloads := discoverThreeWayWorkloadsFn
	discoverThreeWayWorkloadsFn = func(namespace string) ([]WorkloadInfo, error) {
		return []WorkloadInfo{
			{Kind: "Deployment", Namespace: "prod", Name: "api"},
			{Kind: "Deployment", Namespace: "prod", Name: "worker"},
		}, nil
	}
	defer func() { discoverThreeWayWorkloadsFn = prevDiscoverWorkloads }()

	prevBuilder := buildThreeWayResourceResultFn
	buildThreeWayResourceResultFn = func(ctx context.Context, resourceArg, namespace string) (compareResourceResult, error) {
		switch resourceArg {
		case "Deployment/api":
			return compareResourceResult{
				Resource:   "Deployment/api",
				Namespace:  "prod",
				Mode:       "dry-wet-live",
				Connected:  true,
				Mismatches: []compareFieldMismatch{{Field: "replicas", Dry: "3", Wet: "2", Live: "1"}},
			}, nil
		case "Deployment/worker":
			return compareResourceResult{
				Resource:  "Deployment/worker",
				Namespace: "prod",
				Mode:      "live-only",
				Connected: false,
				Notes:     []string{"Connect to ConfigHub to unlock DRY/WET/LIVE expected-state comparison."},
			}, nil
		default:
			return compareResourceResult{}, nil
		}
	}
	defer func() { buildThreeWayResourceResultFn = prevBuilder }()

	report, err := buildThreeWayReport(context.Background(), threeWayScope{ScopeType: threeWayScopeNamespace, ScopeValue: "prod"})
	if err != nil {
		t.Fatalf("buildThreeWayReport() error = %v", err)
	}
	if report.Summary.TotalResources != 2 {
		t.Fatalf("total resources = %d, want 2", report.Summary.TotalResources)
	}
	if report.Summary.MismatchedResources != 1 {
		t.Fatalf("mismatched resources = %d, want 1", report.Summary.MismatchedResources)
	}
	if report.Summary.CauseBuckets["disconnected"] != 1 {
		t.Fatalf("disconnected cause count = %d, want 1", report.Summary.CauseBuckets["disconnected"])
	}
}

func TestRunCompareThreeWay_JSON(t *testing.T) {
	restoreFlags := setThreeWayFlagState(threeWayFlagState{scope: "namespace/prod", format: "json"})
	defer restoreFlags()

	prevDiscoverWorkloads := discoverThreeWayWorkloadsFn
	discoverThreeWayWorkloadsFn = func(namespace string) ([]WorkloadInfo, error) {
		return []WorkloadInfo{{Kind: "Deployment", Namespace: "prod", Name: "api"}}, nil
	}
	defer func() { discoverThreeWayWorkloadsFn = prevDiscoverWorkloads }()

	prevBuilder := buildThreeWayResourceResultFn
	buildThreeWayResourceResultFn = func(ctx context.Context, resourceArg, namespace string) (compareResourceResult, error) {
		return compareResourceResult{
			Resource:  "Deployment/api",
			Namespace: "prod",
			Mode:      "dry-wet-live",
			Connected: true,
		}, nil
	}
	defer func() { buildThreeWayResourceResultFn = prevBuilder }()

	out := captureStdout(t, func() {
		if err := runCompareThreeWay(&cobra.Command{}, nil); err != nil {
			t.Fatalf("runCompareThreeWay() error = %v", err)
		}
	})

	var payload threeWayReport
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid json output: %v\n%s", err, out)
	}
	if payload.Scope != "namespace/prod" {
		t.Fatalf("scope = %q, want namespace/prod", payload.Scope)
	}
	if len(payload.Resources) != 1 {
		t.Fatalf("resource count = %d, want 1", len(payload.Resources))
	}
}

func TestRunCompareThreeWay_UnsupportedScope(t *testing.T) {
	restoreFlags := setThreeWayFlagState(threeWayFlagState{scope: "unit/payment-api", format: "ascii"})
	defer restoreFlags()

	err := runCompareThreeWay(&cobra.Command{}, nil)
	if err == nil {
		t.Fatal("expected unsupported scope error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "scope") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type threeWayFlagState struct {
	scope  string
	format string
	json   bool
}

func setThreeWayFlagState(state threeWayFlagState) func() {
	prevScope := compareThreeWayScopeRaw
	prevFormat := compareThreeWayFormat
	prevJSON := compareThreeWayJSON

	compareThreeWayScopeRaw = state.scope
	compareThreeWayFormat = state.format
	compareThreeWayJSON = state.json

	return func() {
		compareThreeWayScopeRaw = prevScope
		compareThreeWayFormat = prevFormat
		compareThreeWayJSON = prevJSON
	}
}
