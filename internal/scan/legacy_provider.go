package scan

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

// LegacyProvider wraps the existing pkg/agent scanners behind the Provider
// interface. This is the default provider — it uses the built-in scanner
// implementations that have been shipping since v0.5.
type LegacyProvider struct {
	config ProviderConfig
}

// NewLegacyProvider creates a LegacyProvider with resolved configuration.
func NewLegacyProvider(cfg ProviderConfig) *LegacyProvider {
	return &LegacyProvider{config: ResolveConfig(cfg)}
}

func (p *LegacyProvider) Name() string    { return "legacy" }
func (p *LegacyProvider) Available() bool { return true }

// ScanCluster runs all enabled scan types against a live cluster.
// The orchestration logic is extracted from cmd/cub-scout/scan.go runScan().
func (p *LegacyProvider) ScanCluster(ctx context.Context, opts ClusterScanOpts) (*CombinedResult, error) {
	var result CombinedResult

	// Kyverno scan
	if opts.RunKyverno {
		kyvernoResult, kyvernoOnlyMode, err := p.scanKyverno(ctx, opts)
		if err != nil {
			return nil, err
		}
		result.Kyverno = kyvernoResult
		// If Kyverno was explicitly requested but not available, return early
		// with the error result (matches legacy behavior).
		if kyvernoOnlyMode {
			return &result, nil
		}
	}

	// State scan
	if opts.RunState {
		stateResult, err := p.scanState(ctx, opts)
		if err != nil {
			return nil, err
		}
		result.State = stateResult
	}

	// Timing bombs scan
	if opts.RunTimingBombs {
		tbResult, err := p.scanTimingBombs(ctx, opts)
		if err != nil {
			return nil, err
		}
		result.TimingBombs = tbResult
	}

	// Unresolved findings scan
	if opts.RunUnresolved {
		urResult, err := p.scanUnresolved(ctx, opts)
		if err != nil {
			return nil, err
		}
		result.Unresolved = urResult
	}

	// Dangling resources scan
	if opts.RunDangling {
		dangResult, err := p.scanDangling(ctx, opts)
		if err != nil {
			return nil, err
		}
		result.Dangling = dangResult
	}

	// Lifecycle hazards scan
	if opts.RunLifecycleHazards {
		lhResult, err := p.scanLifecycleHazards(ctx, opts)
		if err != nil {
			return nil, err
		}
		result.LifecycleHazards = lhResult
	}

	return &result, nil
}

// ScanFile performs static analysis on a YAML file.
// Extracted from cmd/cub-scout/scan.go runFileScan().
func (p *LegacyProvider) ScanFile(ctx context.Context, opts FileScanOpts) (*CombinedResult, error) {
	scanner, err := agent.NewStaticScanner(p.config.CCVEDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create static scanner: %w", err)
	}

	staticResult, err := scanner.ScanFile(ctx, opts.Filename)
	if err != nil {
		return nil, fmt.Errorf("static scan failed: %w", err)
	}

	var result CombinedResult
	result.Static = staticResult

	// Optional lifecycle hazards scan
	if opts.RunLifecycleHazards {
		objects, err := scanner.LoadObjectsFromFile(opts.Filename)
		if err != nil {
			return nil, fmt.Errorf("failed to load objects for lifecycle scan: %w", err)
		}
		lifecycleScanner := agent.NewLifecycleHazardScannerFromObjects(objects)
		result.LifecycleHazards = lifecycleScanner.ScanObjects(objects)
	}

	return &result, nil
}

// ListPolicies returns the Kyverno policy catalog.
// Extracted from cmd/cub-scout/scan.go listKPOLPolicies().
func (p *LegacyProvider) ListPolicies() ([]PolicyEntry, error) {
	if p.config.PolicyDBDir == "" {
		return nil, fmt.Errorf("policy database not found")
	}

	scanner := agent.NewKyvernoScannerWithClient(nil, p.config.PolicyDBDir)
	policies, err := scanner.GetPolicyCatalog()
	if err != nil {
		return nil, fmt.Errorf("failed to load policy catalog: %w", err)
	}

	// Sort by ID (deterministic output)
	sort.Slice(policies, func(i, j int) bool {
		return policies[i].ID < policies[j].ID
	})

	entries := make([]PolicyEntry, len(policies))
	for i, p := range policies {
		entries[i] = PolicyEntry{
			ID:       p.ID,
			Name:     p.Name,
			Severity: p.Severity,
			Category: p.Category,
		}
	}
	return entries, nil
}

// --- internal scanner methods ---

func (p *LegacyProvider) scanKyverno(ctx context.Context, opts ClusterScanOpts) (*agent.ScanResult, bool, error) {
	scanner, err := agent.NewKyvernoScanner(opts.Config, p.config.PolicyDBDir)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create kyverno scanner: %w", err)
	}

	if !scanner.Available(ctx) {
		// Return nil result when Kyverno is not installed.
		// Second return value signals "kyverno-only mode" to the caller
		// so it can decide whether to return an error message.
		return nil, false, nil
	}

	var result *agent.ScanResult
	if opts.Namespace != "" {
		result, err = scanner.ScanNamespace(ctx, opts.Namespace)
	} else {
		result, err = scanner.Scan(ctx)
	}
	if err != nil {
		return nil, false, fmt.Errorf("kyverno scan failed: %w", err)
	}
	return result, false, nil
}

func (p *LegacyProvider) scanState(ctx context.Context, opts ClusterScanOpts) (*agent.StateScanResult, error) {
	stateScanner, err := agent.NewStateScanner(opts.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create state scanner: %w", err)
	}

	threshold := opts.Threshold
	if threshold == 0 {
		threshold = 5 * time.Minute
	}

	if opts.Namespace != "" {
		return stateScanner.ScanNamespaceWithThreshold(ctx, opts.Namespace, threshold)
	}
	return stateScanner.ScanWithThreshold(ctx, threshold)
}

func (p *LegacyProvider) scanTimingBombs(ctx context.Context, opts ClusterScanOpts) (*agent.TimingBombResult, error) {
	stateScanner, err := agent.NewStateScanner(opts.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create state scanner for timing bombs: %w", err)
	}
	return stateScanner.ScanTimingBombs(ctx)
}

func (p *LegacyProvider) scanUnresolved(ctx context.Context, opts ClusterScanOpts) (*agent.UnresolvedResult, error) {
	stateScanner, err := agent.NewStateScanner(opts.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create state scanner for unresolved: %w", err)
	}
	return stateScanner.ScanUnresolvedFindings(ctx)
}

func (p *LegacyProvider) scanDangling(ctx context.Context, opts ClusterScanOpts) (*agent.DanglingResult, error) {
	stateScanner, err := agent.NewStateScanner(opts.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create state scanner for dangling: %w", err)
	}
	return stateScanner.ScanDanglingResources(ctx)
}

func (p *LegacyProvider) scanLifecycleHazards(ctx context.Context, opts ClusterScanOpts) (*agent.LifecycleHazardResult, error) {
	lifecycleScanner, err := agent.NewLifecycleHazardScanner(opts.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create lifecycle hazard scanner: %w", err)
	}
	if opts.Namespace != "" {
		return lifecycleScanner.ScanNamespace(ctx, opts.Namespace)
	}
	return lifecycleScanner.Scan(ctx)
}
