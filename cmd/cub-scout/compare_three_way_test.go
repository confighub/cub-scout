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

	report, err := buildThreeWayReport(context.Background(), threeWayScope{ScopeType: threeWayScopeNamespace, ScopeValue: "prod"}, "")
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
	failOn string
}

func setThreeWayFlagState(state threeWayFlagState) func() {
	prevScope := compareThreeWayScopeRaw
	prevFormat := compareThreeWayFormat
	prevJSON := compareThreeWayJSON
	prevFailOn := compareThreeWayFailOn

	compareThreeWayScopeRaw = state.scope
	compareThreeWayFormat = state.format
	compareThreeWayJSON = state.json
	compareThreeWayFailOn = state.failOn

	return func() {
		compareThreeWayScopeRaw = prevScope
		compareThreeWayFormat = prevFormat
		compareThreeWayJSON = prevJSON
		compareThreeWayFailOn = prevFailOn
	}
}

func TestIsValidConformanceSeverity(t *testing.T) {
	tests := []struct {
		level string
		want  bool
	}{
		{"info", true},
		{"warning", true},
		{"critical", false}, // not valid for conformance (only info and warning)
		{"", false},
		{"high", false},
	}

	for _, tt := range tests {
		got := isValidConformanceSeverity(tt.level)
		if got != tt.want {
			t.Errorf("isValidConformanceSeverity(%q) = %v, want %v", tt.level, got, tt.want)
		}
	}
}

func TestComputeConformanceExitCode(t *testing.T) {
	tests := []struct {
		name     string
		counts   map[string]int
		failOn   string
		wantCode int
	}{
		{
			name:     "no fail-on specified",
			counts:   map[string]int{"warning": 1},
			failOn:   "",
			wantCode: ConformanceExitOK,
		},
		{
			name:     "no violations with fail-on warning",
			counts:   map[string]int{"ok": 5},
			failOn:   "warning",
			wantCode: ConformanceExitOK,
		},
		{
			name:     "warning violations with fail-on warning",
			counts:   map[string]int{"warning": 1, "ok": 4},
			failOn:   "warning",
			wantCode: ConformanceExitFailure,
		},
		{
			name:     "info violations with fail-on warning",
			counts:   map[string]int{"info": 2, "ok": 3},
			failOn:   "warning",
			wantCode: ConformanceExitOK,
		},
		{
			name:     "info violations with fail-on info",
			counts:   map[string]int{"info": 2, "ok": 3},
			failOn:   "info",
			wantCode: ConformanceExitFailure,
		},
		{
			name:     "warning violations with fail-on info",
			counts:   map[string]int{"warning": 1},
			failOn:   "info",
			wantCode: ConformanceExitFailure,
		},
		{
			name:     "empty counts with fail-on warning",
			counts:   map[string]int{},
			failOn:   "warning",
			wantCode: ConformanceExitOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := threeWayReport{
				Summary: threeWaySummary{
					SeverityCounts: tt.counts,
				},
			}
			got := computeConformanceExitCode(report, tt.failOn)
			if got != tt.wantCode {
				t.Errorf("computeConformanceExitCode() = %d, want %d", got, tt.wantCode)
			}
		})
	}
}

func TestGetMaxConformanceSeverity(t *testing.T) {
	tests := []struct {
		counts map[string]int
		want   string
	}{
		{map[string]int{"warning": 1, "info": 2, "ok": 3}, "warning"},
		{map[string]int{"info": 2, "ok": 3}, "info"},
		{map[string]int{"ok": 5}, "ok"},
		{map[string]int{}, "ok"},
	}

	for _, tt := range tests {
		got := getMaxConformanceSeverity(tt.counts)
		if got != tt.want {
			t.Errorf("getMaxConformanceSeverity(%v) = %q, want %q", tt.counts, got, tt.want)
		}
	}
}

func TestConformanceSeverityMeetsThreshold(t *testing.T) {
	tests := []struct {
		maxSeverity string
		threshold   string
		want        bool
	}{
		{"warning", "warning", true},
		{"warning", "info", true},
		{"info", "warning", false},
		{"info", "info", true},
		{"ok", "info", false},
		{"ok", "warning", false},
	}

	for _, tt := range tests {
		got := conformanceSeverityMeetsThreshold(tt.maxSeverity, tt.threshold)
		if got != tt.want {
			t.Errorf("conformanceSeverityMeetsThreshold(%q, %q) = %v, want %v",
				tt.maxSeverity, tt.threshold, got, tt.want)
		}
	}
}

func TestBuildThreeWayReport_ConformanceResult(t *testing.T) {
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
				Mismatches: []compareFieldMismatch{{Field: "replicas"}},
			}, nil
		default:
			return compareResourceResult{
				Resource:  resourceArg,
				Namespace: "prod",
				Mode:      "dry-wet-live",
				Connected: true,
			}, nil
		}
	}
	defer func() { buildThreeWayResourceResultFn = prevBuilder }()

	// With --fail-on warning, one mismatch (warning) should cause pass=false
	report, err := buildThreeWayReport(context.Background(), threeWayScope{ScopeType: threeWayScopeNamespace, ScopeValue: "prod"}, "warning")
	if err != nil {
		t.Fatalf("buildThreeWayReport() error = %v", err)
	}

	// One resource has mismatches (warning severity), threshold is warning -> fail
	if report.Summary.Conformance.Pass {
		t.Error("Conformance.Pass = true, want false (warning meets warning threshold)")
	}
	if report.Summary.Conformance.Violations != 1 {
		t.Errorf("Conformance.Violations = %d, want 1", report.Summary.Conformance.Violations)
	}
	if report.Summary.Conformance.MaxSeverity != "warning" {
		t.Errorf("Conformance.MaxSeverity = %q, want %q", report.Summary.Conformance.MaxSeverity, "warning")
	}
	if report.Summary.Conformance.Threshold != "warning" {
		t.Errorf("Conformance.Threshold = %q, want %q", report.Summary.Conformance.Threshold, "warning")
	}
}

func TestBuildThreeWayReport_ConformancePass(t *testing.T) {
	prevDiscoverWorkloads := discoverThreeWayWorkloadsFn
	discoverThreeWayWorkloadsFn = func(namespace string) ([]WorkloadInfo, error) {
		return []WorkloadInfo{
			{Kind: "Deployment", Namespace: "prod", Name: "api"},
		}, nil
	}
	defer func() { discoverThreeWayWorkloadsFn = prevDiscoverWorkloads }()

	prevBuilder := buildThreeWayResourceResultFn
	buildThreeWayResourceResultFn = func(ctx context.Context, resourceArg, namespace string) (compareResourceResult, error) {
		return compareResourceResult{
			Resource:  resourceArg,
			Namespace: "prod",
			Mode:      "dry-wet-live",
			Connected: true,
			// No mismatches, no notes = ok severity
		}, nil
	}
	defer func() { buildThreeWayResourceResultFn = prevBuilder }()

	// With --fail-on warning, no issues should pass
	report, err := buildThreeWayReport(context.Background(), threeWayScope{ScopeType: threeWayScopeNamespace, ScopeValue: "prod"}, "warning")
	if err != nil {
		t.Fatalf("buildThreeWayReport() error = %v", err)
	}

	if !report.Summary.Conformance.Pass {
		t.Error("Conformance.Pass = false, want true (no violations)")
	}
	if report.Summary.Conformance.Violations != 0 {
		t.Errorf("Conformance.Violations = %d, want 0", report.Summary.Conformance.Violations)
	}
	if report.Summary.Conformance.MaxSeverity != "ok" {
		t.Errorf("Conformance.MaxSeverity = %q, want %q", report.Summary.Conformance.MaxSeverity, "ok")
	}
}

func TestBuildThreeWayReport_ConformanceNoThreshold(t *testing.T) {
	prevDiscoverWorkloads := discoverThreeWayWorkloadsFn
	discoverThreeWayWorkloadsFn = func(namespace string) ([]WorkloadInfo, error) {
		return []WorkloadInfo{
			{Kind: "Deployment", Namespace: "prod", Name: "api"},
		}, nil
	}
	defer func() { discoverThreeWayWorkloadsFn = prevDiscoverWorkloads }()

	prevBuilder := buildThreeWayResourceResultFn
	buildThreeWayResourceResultFn = func(ctx context.Context, resourceArg, namespace string) (compareResourceResult, error) {
		return compareResourceResult{
			Resource:   resourceArg,
			Namespace:  "prod",
			Mode:       "dry-wet-live",
			Connected:  true,
			Mismatches: []compareFieldMismatch{{Field: "replicas"}}, // Has issues
		}, nil
	}
	defer func() { buildThreeWayResourceResultFn = prevBuilder }()

	// Without --fail-on (pure reporting mode), pass is always true
	report, err := buildThreeWayReport(context.Background(), threeWayScope{ScopeType: threeWayScopeNamespace, ScopeValue: "prod"}, "")
	if err != nil {
		t.Fatalf("buildThreeWayReport() error = %v", err)
	}

	// Even with violations, pass=true in pure reporting mode (no threshold)
	if !report.Summary.Conformance.Pass {
		t.Error("Conformance.Pass = false, want true (pure reporting mode)")
	}
	if report.Summary.Conformance.Violations != 1 {
		t.Errorf("Conformance.Violations = %d, want 1", report.Summary.Conformance.Violations)
	}
	if report.Summary.Conformance.Threshold != "" {
		t.Errorf("Conformance.Threshold = %q, want empty", report.Summary.Conformance.Threshold)
	}
}

// TestExitCodeIndependentOfFormat_Conformance verifies the Leak Test:
// exit status must depend only on JSON facts, not ASCII rendering.
func TestExitCodeIndependentOfFormat_Conformance(t *testing.T) {
	// Create a report with violations
	report := threeWayReport{
		Summary: threeWaySummary{
			SeverityCounts: map[string]int{"warning": 1, "ok": 2},
		},
	}

	failOn := "warning"

	// The exit code computation is format-independent
	exitCode := computeConformanceExitCode(report, failOn)
	expectedExit := ConformanceExitFailure

	if exitCode != expectedExit {
		t.Errorf("exit code = %d, want %d", exitCode, expectedExit)
	}

	// Rendering functions should not affect exit code
	_ = renderThreeWayASCII(report)
	_ = renderThreeWayMarkdown(report)

	// Exit code should still be the same
	if computeConformanceExitCode(report, failOn) != expectedExit {
		t.Error("exit code changed after rendering")
	}
}
