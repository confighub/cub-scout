package main

import (
	"context"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/internal/scan"
	"github.com/confighub/cub-scout/v2/internal/summarystore"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

type captureScanProvider struct {
	opts   scan.ClusterScanOpts
	result *scan.CombinedResult
	called bool
}

func (*captureScanProvider) Name() string { return "test" }
func (p *captureScanProvider) ScanCluster(_ context.Context, opts scan.ClusterScanOpts) (*scan.CombinedResult, error) {
	p.opts = opts
	p.called = true
	return p.result, nil
}
func (*captureScanProvider) ScanFile(context.Context, scan.FileScanOpts) (*scan.CombinedResult, error) {
	return &scan.CombinedResult{}, nil
}
func (*captureScanProvider) ListPolicies() ([]scan.PolicyEntry, error) { return nil, nil }
func (*captureScanProvider) Available() bool                           { return true }

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

func TestScanFindingsForTUIUsesSharedNormalizedFindingsAndSurfacesWarnings(t *testing.T) {
	result := &scan.CombinedResult{
		Static: &agent.StaticScanResult{Findings: []agent.StaticFinding{
			{CCVEID: "CCVE-2026-0042", Severity: "warning", Category: "CONFIG", ResourceName: "Deployment/api", Namespace: "team-a", Message: "marker finding"},
			{CCVEID: "CCVE-2026-0043", Severity: "info", ResourceName: "Service/web", Namespace: "team-a", Message: "uncategorized marker"},
		}},
		Kyverno: &agent.ScanResult{Error: "PolicyReport API denied"},
		State:   &agent.StateScanResult{Warnings: []string{"jobs list denied"}},
	}
	findings, categories := scanFindingsForTUI(result)
	if len(findings) != 2 || findings[0].CCVE != "CCVE-2026-0042" || findings[0].Category != "CONFIG" || categories["CONFIG"] != 1 || findings[1].Category != "UNCATEGORIZED" || categories["UNCATEGORIZED"] != 1 {
		t.Fatalf("TUI findings=%+v categories=%v", findings, categories)
	}
	if err := scanWarningsError(result); err == nil || !strings.Contains(err.Error(), "PolicyReport API denied") || !strings.Contains(err.Error(), "jobs list denied") {
		t.Fatalf("partial coverage warning = %v", err)
	}
	view := (LocalClusterModel{scanWarnings: scanWarnings(result), scanFindings: findings, scanOutput: "structured result"}).renderScan()
	if !strings.Contains(view, "PolicyReport API denied") || !strings.Contains(view, "marker finding") || !strings.Contains(view, "uncategorized marker") {
		t.Fatalf("TUI must show partial coverage and the known finding together:\n%s", view)
	}
}

func TestRunScanUsesSelectedContextAndPersistsItsLabel(t *testing.T) {
	alpha := newCountedKubeServer(t)
	beta := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	t.Setenv("KUBECONFIG", path)
	t.Setenv("CUB_SCOUT_SCAN_PROVIDER", "legacy")
	provider := &captureScanProvider{result: &scan.CombinedResult{}}
	oldProvider, oldConnected, oldPersist := selectScanProviderFn, summaryConnectedFn, persistSummaryRecordFn
	selectScanProviderFn = func(scan.ProviderConfig) scan.Provider { return provider }
	var persistedCluster string
	summaryConnectedFn = func() bool { return true }
	t.Cleanup(func() {
		selectScanProviderFn, summaryConnectedFn, persistSummaryRecordFn = oldProvider, oldConnected, oldPersist
	})
	oldContext := scanCmd.Flags().Lookup("kube-context").Value.String()
	oldChanged := scanCmd.Flags().Lookup("kube-context").Changed
	if err := scanCmd.Flags().Set("kube-context", "beta"); err != nil {
		t.Fatal(err)
	}
	scanCmd.Flags().Lookup("kube-context").Changed = true
	scanCmd.SetContext(context.Background())
	oldJSON := scanJSON
	scanJSON = true
	t.Cleanup(func() {
		scanJSON = oldJSON
		_ = scanCmd.Flags().Set("kube-context", oldContext)
		scanCmd.Flags().Lookup("kube-context").Changed = oldChanged
	})
	persistSummaryRecordFn = func(record summarystore.Record) error { persistedCluster = record.Cluster; return nil }
	if err := runScan(scanCmd, nil); err != nil {
		t.Fatalf("runScan: %v", err)
	}
	if provider.opts.Config == nil || provider.opts.Config.Host != beta.server.URL {
		t.Fatalf("provider config = %+v, want selected beta endpoint", provider.opts.Config)
	}
	if alpha.requests.Load() != 0 || beta.requests.Load() != 0 {
		t.Fatalf("fake provider should not make API requests: alpha=%d beta=%d", alpha.requests.Load(), beta.requests.Load())
	}
	if persistedCluster != "beta" {
		t.Fatalf("persisted cluster label = %q, want selected context label", persistedCluster)
	}
}

func TestRunScanRejectsInvalidAndOfflineContextSelectionsBeforeReads(t *testing.T) {
	alpha := newCountedKubeServer(t)
	beta := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	t.Setenv("KUBECONFIG", path)
	oldProviderFn := selectScanProviderFn
	provider := &captureScanProvider{result: &scan.CombinedResult{}}
	selectScanProviderFn = func(scan.ProviderConfig) scan.Provider { return provider }
	t.Cleanup(func() { selectScanProviderFn = oldProviderFn })
	flag := scanCmd.Flags().Lookup("kube-context")
	oldValue, oldChanged := flag.Value.String(), flag.Changed
	oldList, oldFile := scanList, scanFile
	t.Cleanup(func() { _ = flag.Value.Set(oldValue); flag.Changed = oldChanged; scanList, scanFile = oldList, oldFile })
	for _, tc := range []struct{ value, want string }{{"missing", "not found"}, {"  ", "non-empty"}} {
		provider.called = false
		alphaBefore, betaBefore := alpha.requests.Load(), beta.requests.Load()
		if err := flag.Value.Set(tc.value); err != nil {
			t.Fatal(err)
		}
		flag.Changed = true
		err := runScan(scanCmd, nil)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("context %q error = %v", tc.value, err)
		}
		if provider.called || alpha.requests.Load() != alphaBefore || beta.requests.Load() != betaBefore {
			t.Fatalf("invalid context %q triggered provider/API reads", tc.value)
		}
	}
	if err := flag.Value.Set("beta"); err != nil {
		t.Fatal(err)
	}
	flag.Changed = true
	for _, mode := range []string{"list", "file"} {
		scanList, scanFile = false, ""
		if mode == "list" {
			scanList = true
		} else {
			scanFile = "manifest.yaml"
		}
		if err := runScan(scanCmd, nil); err == nil || !strings.Contains(err.Error(), "only to live cluster scans") {
			t.Fatalf("explicit context in %s mode error = %v", mode, err)
		}
	}
	if alpha.requests.Load() != 0 || beta.requests.Load() != 0 {
		t.Fatalf("invalid/offline selections made API requests: alpha=%d beta=%d", alpha.requests.Load(), beta.requests.Load())
	}
}
