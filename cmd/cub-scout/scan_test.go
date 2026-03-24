package main

import (
	"testing"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/confighub/cub-scout/pkg/agent"
)

func TestComputeScanExitCode(t *testing.T) {
	tests := []struct {
		name     string
		combined *scan.CombinedResult
		failOn   string
		want     int
	}{
		{
			name:     "no fail-on, no findings",
			combined: &scan.CombinedResult{},
			failOn:   "",
			want:     0,
		},
		{
			name: "no fail-on, has findings",
			combined: &scan.CombinedResult{
				Static: &agent.StaticScanResult{
					Findings: []agent.StaticFinding{{Severity: "warning"}},
				},
			},
			failOn: "",
			want:   0,
		},
		{
			name: "fail-on warning, has warning",
			combined: &scan.CombinedResult{
				Static: &agent.StaticScanResult{
					Findings: []agent.StaticFinding{{Severity: "warning"}},
				},
			},
			failOn: "warning",
			want:   1,
		},
		{
			name: "fail-on critical, has warning only",
			combined: &scan.CombinedResult{
				Static: &agent.StaticScanResult{
					Findings: []agent.StaticFinding{{Severity: "warning"}},
				},
			},
			failOn: "critical",
			want:   0,
		},
		{
			name: "fail-on info, has info",
			combined: &scan.CombinedResult{
				Static: &agent.StaticScanResult{
					Findings: []agent.StaticFinding{{Severity: "info"}},
				},
			},
			failOn: "info",
			want:   1,
		},
		{
			name: "fail-on warning, has critical",
			combined: &scan.CombinedResult{
				Static: &agent.StaticScanResult{
					Findings: []agent.StaticFinding{{Severity: "critical"}},
				},
			},
			failOn: "warning",
			want:   1,
		},
		{
			name:     "fail-on warning, no findings",
			combined: &scan.CombinedResult{},
			failOn:   "warning",
			want:     0,
		},
		{
			name:     "unrecognized threshold, never fails",
			combined: &scan.CombinedResult{},
			failOn:   "bogus",
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeScanExitCode(tt.combined, tt.failOn)
			if got != tt.want {
				t.Errorf("computeScanExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}
