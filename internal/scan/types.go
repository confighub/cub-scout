// Package scan defines the scan provider boundary for cub-scout.
//
// Providers abstract scan execution so cub-scout can delegate to different
// scan engines (legacy built-in scanners, confighub-scan, etc.) behind a
// common interface. The CLI layer in cmd/cub-scout/scan.go owns rendering;
// providers own execution.
package scan

import (
	"context"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/client-go/rest"
)

// Provider abstracts scan execution behind a testable boundary.
// Implementations: LegacyProvider (built-in), ConfighubScanProvider (future).
type Provider interface {
	// Name returns the provider identifier (e.g. "legacy", "confighub-scan").
	Name() string

	// ScanCluster runs enabled scan types against a live cluster.
	ScanCluster(ctx context.Context, opts ClusterScanOpts) (*CombinedResult, error)

	// ScanFile runs static analysis on a YAML file (no cluster required).
	ScanFile(ctx context.Context, opts FileScanOpts) (*CombinedResult, error)

	// ListPolicies returns the Kyverno policy catalog.
	ListPolicies() ([]PolicyEntry, error)

	// Available reports whether this provider can execute.
	Available() bool
}

// ClusterScanOpts encapsulates all options for a cluster scan.
type ClusterScanOpts struct {
	Config              *rest.Config
	Namespace           string
	RunKyverno          bool
	RunState            bool
	RunTimingBombs      bool
	RunUnresolved       bool
	RunDangling         bool
	RunLifecycleHazards bool
	Threshold           time.Duration
}

// FileScanOpts encapsulates options for a static file scan.
type FileScanOpts struct {
	Filename            string
	RunLifecycleHazards bool
}

// CombinedResult holds results from all scanner types.
// Field names and JSON tags match the legacy CombinedScanResult exactly
// to preserve serialization compatibility with golden tests and the
// CUB_SCOUT_TEST_SCAN_JSON test hook.
type CombinedResult struct {
	Kyverno          *agent.ScanResult            `json:"kyverno,omitempty"`
	State            *agent.StateScanResult       `json:"state,omitempty"`
	TimingBombs      *agent.TimingBombResult      `json:"timingBombs,omitempty"`
	Unresolved       *agent.UnresolvedResult      `json:"unresolved,omitempty"`
	Dangling         *agent.DanglingResult        `json:"dangling,omitempty"`
	LifecycleHazards *agent.LifecycleHazardResult `json:"lifecycleHazards,omitempty"`
	Static           *agent.StaticScanResult      `json:"static,omitempty"`
}

// PolicyEntry represents a policy in the catalog listing.
type PolicyEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Category string `json:"category"`
}
