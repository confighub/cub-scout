package main

import (
	"context"
	"github.com/confighub/cub-scout/v2/internal/scan"
	"strings"
	"testing"
)

type scanListInvocationProvider struct{}

func (scanListInvocationProvider) Name() string { return "test" }
func (scanListInvocationProvider) ScanCluster(context.Context, scan.ClusterScanOpts) (*scan.CombinedResult, error) {
	return nil, nil
}
func (scanListInvocationProvider) ScanFile(context.Context, scan.FileScanOpts) (*scan.CombinedResult, error) {
	return nil, nil
}
func (scanListInvocationProvider) ListPolicies() ([]scan.PolicyEntry, error) {
	return []scan.PolicyEntry{{ID: "KPOL-0001", Name: "fixture", Severity: "warning", Category: "test"}}, nil
}
func (scanListInvocationProvider) Available() bool { return true }

func TestScanListTrailer_FollowsInvocationForm(t *testing.T) {
	oldJSON := scanJSON
	scanJSON = false
	t.Cleanup(func() { scanJSON = oldJSON })

	for _, tc := range []struct {
		name, plugin, want, reject string
	}{
		{name: "plugin", plugin: "1", want: "Run 'cub scout scan' to check for violations", reject: "Run 'cub-scout scan'"},
		{name: "standalone", plugin: "", want: "Run 'cub-scout scan' to check for violations", reject: "Run 'cub scout scan'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CUB_PLUGIN", tc.plugin)
			out := captureStdout(t, func() {
				if err := listKPOLPolicies(scanListInvocationProvider{}); err != nil {
					t.Errorf("listKPOLPolicies: %v", err)
				}
			})
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected %q in scan --list output:\n%s", tc.want, out)
			}
			if strings.Contains(out, tc.reject) {
				t.Errorf("scan --list output contains %q in %s mode:\n%s", tc.reject, tc.name, out)
			}
		})
	}
}
