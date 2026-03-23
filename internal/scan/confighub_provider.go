package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"
)

// ConfighubScanProvider delegates scanning to the confighub-scan binary when
// available, falling back to LegacyProvider for all operations when it is not.
// This is the future provider for ConfigHub-managed scan patterns.
type ConfighubScanProvider struct {
	fallback *LegacyProvider

	// binaryPath caches the resolved cub-scan binary path.
	// Empty string means not resolved yet; "-" means not found.
	binaryPath string

	// test seams
	exportManifestFn    func(context.Context, ClusterScanOpts) (string, func(), error)
	legacyScanClusterFn func(context.Context, ClusterScanOpts) (*CombinedResult, error)
}

const (
	legacyCatalogHomeDir      = ".confighub"
	bundleCurrentSubdir       = "pattern-bundles/current"
	defaultCatalogFile        = "risk-catalog-v1.json"
	defaultCatalogPath        = "dist/risk-catalog-v1.json"
	defaultBundleManifest     = "bundle-manifest-v1.json"
	defaultBundleManifestPath = "dist/bundle-manifest-v1.json"
	siblingPatternsRepoName   = "confighub-patterns"
	bundleAssetRiskCatalog        = "risk-catalog"
	bundleAssetRiskFunctionLinks  = "risk-function-links"
	bundleAssetKyvernoCCVE        = "kyverno-ccve-mappings"
	bundleAssetTrivyCCVE          = "trivy-ccve-mappings"
	bundleAssetCrossToolMapping   = "cross-tool-mapping"
)

type bundleManifest struct {
	Files []bundleManifestFile `json:"files"`
}

type bundleManifestFile struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// NewConfighubScanProvider creates a ConfighubScanProvider that probes for the
// confighub-scan binary. If unavailable, all methods silently delegate to the
// LegacyProvider.
func NewConfighubScanProvider(cfg ProviderConfig) *ConfighubScanProvider {
	return &ConfighubScanProvider{
		fallback: NewLegacyProvider(cfg),
	}
}

func (p *ConfighubScanProvider) Name() string { return "confighub-scan" }

// Available reports whether the confighub-scan binary is on PATH.
func (p *ConfighubScanProvider) Available() bool {
	return p.resolveBinary() != ""
}

// ScanCluster keeps runtime-oriented scans on the legacy provider and, when
// cub-scan is available, additionally exports live resources to a temporary
// manifest and runs a static cub-scan pass against that manifest.
// If export or cub-scan execution fails, the legacy result is returned.
func (p *ConfighubScanProvider) ScanCluster(ctx context.Context, opts ClusterScanOpts) (*CombinedResult, error) {
	legacyScan := p.legacyScanClusterFn
	if legacyScan == nil {
		legacyScan = p.fallback.ScanCluster
	}

	legacyResult, err := legacyScan(ctx, opts)
	if err != nil {
		return nil, err
	}

	binary := p.resolveBinary()
	if binary == "" {
		return legacyResult, nil
	}

	exportManifest := p.exportManifestFn
	if exportManifest == nil {
		exportManifest = exportClusterResourcesToManifest
	}

	manifestPath, cleanup, err := exportManifest(ctx, opts)
	if err != nil {
		return legacyResult, nil
	}
	if cleanup != nil {
		defer cleanup()
	}

	cs, err := p.invokeCubScan(ctx, binary, manifestPath)
	if err != nil {
		return legacyResult, nil
	}

	_ = persistRawFindingsV1(cs)
	legacyResult.Static = mapCubScanResult(cs, manifestPath)

	return legacyResult, nil
}

// ScanFile delegates to confighub-scan when available, otherwise falls back.
// When the binary is found, it invokes: cub-scan --format json [--catalog <path>] <file>
// and maps the output to CombinedResult.Static.
func (p *ConfighubScanProvider) ScanFile(ctx context.Context, opts FileScanOpts) (*CombinedResult, error) {
	binary := p.resolveBinary()
	if binary == "" {
		return p.fallback.ScanFile(ctx, opts)
	}

	result, err := p.invokeCubScan(ctx, binary, opts.Filename)
	if err != nil {
		// Fall back silently on binary execution errors so the user still
		// gets results from the legacy scanner rather than a hard failure.
		return p.fallback.ScanFile(ctx, opts)
	}

	combined := &CombinedResult{
		Static: mapCubScanResult(result, opts.Filename),
	}

	// Run lifecycle hazards via legacy if requested (cub-scan doesn't cover these yet).
	if opts.RunLifecycleHazards {
		legacyResult, err := p.fallback.ScanFile(ctx, FileScanOpts{
			Filename:            opts.Filename,
			RunLifecycleHazards: true,
		})
		if err == nil && legacyResult != nil {
			combined.LifecycleHazards = legacyResult.LifecycleHazards
		}
	}

	return combined, nil
}

// ListPolicies delegates to confighub-scan when available, otherwise falls back.
// ListPolicies loads the risk catalog from risk-catalog-v1.json when available,
// otherwise falls back to the legacy Kyverno-only policy listing.
func (p *ConfighubScanProvider) ListPolicies() ([]PolicyEntry, error) {
	catalogPath := resolveCatalogPath()
	if catalogPath == "" {
		return p.fallback.ListPolicies()
	}

	entries, err := loadRiskCatalog(catalogPath)
	if err != nil {
		// Fall back if catalog is corrupt/unreadable.
		return p.fallback.ListPolicies()
	}

	return entries, nil
}

// --- cub-scan binary invocation ---

// resolveBinary returns the path to the cub-scan binary, or "" if not found.
func (p *ConfighubScanProvider) resolveBinary() string {
	if p.binaryPath == "-" {
		return ""
	}
	if p.binaryPath != "" {
		return p.binaryPath
	}

	// Look for "confighub-scan" or "cub-scan" on PATH.
	for _, name := range []string{"confighub-scan", "cub-scan"} {
		path, err := exec.LookPath(name)
		if err == nil {
			p.binaryPath = path
			return path
		}
	}

	p.binaryPath = "-" // sentinel: not found
	return ""
}

// resolveCatalogPath returns the risk-catalog path, or "" if none found.
// Resolution:
//  1. CUB_SCAN_CATALOG env
//  2. active bundle cache or bundle-manifest lookup
//  3. sibling confighub-patterns checkout
//  4. legacy ~/.confighub/risk-catalog-v1.json compatibility path
func resolveCatalogPath() string {
	if env := os.Getenv("CUB_SCAN_CATALOG"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}

	if _, resolved := resolveCatalogFromManifest(); resolved != "" {
		return resolved
	}

	for _, candidate := range catalogCandidates() {
		if fileExists(candidate) {
			return candidate
		}
	}

	return ""
}

func catalogCandidates() []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(candidate string) {
		if strings.TrimSpace(candidate) == "" {
			return
		}
		clean := filepath.Clean(candidate)
		if seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, clean)
	}

	home, err := os.UserHomeDir()
	if err == nil {
		add(filepath.Join(home, legacyCatalogHomeDir, bundleCurrentSubdir, defaultCatalogFile))
		add(filepath.Join(home, legacyCatalogHomeDir, defaultCatalogFile))
	}
	add(filepath.Join("..", siblingPatternsRepoName, defaultCatalogPath))
	add(defaultCatalogFile)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(filepath.Join(exeDir, defaultCatalogFile))
		add(filepath.Join(exeDir, "..", defaultCatalogFile))
		add(filepath.Join(exeDir, "..", "share", "confighub-scan", defaultCatalogFile))
	}
	return out
}

func bundleManifestCandidates() []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(candidate string) {
		if strings.TrimSpace(candidate) == "" {
			return
		}
		clean := filepath.Clean(candidate)
		if seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, clean)
	}

	add(defaultBundleManifestPath)
	add(defaultBundleManifest)
	add(filepath.Join("..", siblingPatternsRepoName, defaultBundleManifestPath))
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, legacyCatalogHomeDir, bundleCurrentSubdir, defaultBundleManifest))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(filepath.Join(exeDir, defaultBundleManifest))
		add(filepath.Join(exeDir, "..", defaultBundleManifest))
		add(filepath.Join(exeDir, "..", "share", "confighub-scan", defaultBundleManifest))
	}
	return out
}

func resolveCatalogFromManifest() (string, string) {
	for _, manifestPath := range bundleManifestCandidates() {
		if !fileExists(manifestPath) {
			continue
		}
		if resolved := resolveManifestAsset(manifestPath, bundleAssetRiskCatalog); resolved != "" {
			return manifestPath, resolved
		}
	}
	return "", ""
}

func resolveManifestAsset(manifestPath, assetName string) string {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ""
	}
	var manifest bundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ""
	}
	for _, entry := range manifest.Files {
		if strings.TrimSpace(entry.Name) != assetName || strings.TrimSpace(entry.Path) == "" {
			continue
		}
		for _, candidate := range manifestAssetCandidates(manifestPath, entry.Path) {
			if fileExists(candidate) {
				return candidate
			}
		}
	}
	return ""
}

// resolveManifestAssetPath is like resolveManifestAsset but accepts both
// file and directory paths. Used by ResolveBundleAsset for assets like
// kyverno-ccve-mappings that point to directories.
func resolveManifestAssetPath(manifestPath, assetName string) string {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return ""
	}
	var manifest bundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return ""
	}
	for _, entry := range manifest.Files {
		if strings.TrimSpace(entry.Name) != assetName || strings.TrimSpace(entry.Path) == "" {
			continue
		}
		for _, candidate := range manifestAssetCandidates(manifestPath, entry.Path) {
			if pathExists(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func manifestAssetCandidates(manifestPath, entryPath string) []string {
	manifestDir := filepath.Dir(manifestPath)
	baseDir := manifestDir
	if filepath.Base(manifestDir) == "dist" {
		baseDir = filepath.Dir(manifestDir)
	}

	seen := map[string]bool{}
	out := []string{}
	add := func(candidate string) {
		if strings.TrimSpace(candidate) == "" {
			return
		}
		clean := filepath.Clean(candidate)
		if seen[clean] {
			return
		}
		seen[clean] = true
		out = append(out, clean)
	}

	add(filepath.Join(baseDir, entryPath))
	add(filepath.Join(manifestDir, entryPath))
	add(filepath.Join(manifestDir, filepath.Base(entryPath)))
	return out
}

// ResolveBundleAsset resolves an asset by name from the first available
// bundle-manifest-v1.json. Returns "" if no manifest or asset is found.
// Unlike resolveCatalogFromManifest, this accepts both file and directory assets.
func ResolveBundleAsset(assetName string) string {
	for _, manifestPath := range bundleManifestCandidates() {
		if !fileExists(manifestPath) {
			continue
		}
		if resolved := resolveManifestAssetPath(manifestPath, assetName); resolved != "" {
			return resolved
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// invokeCubScan executes the cub-scan binary and parses its JSON output.
func (p *ConfighubScanProvider) invokeCubScan(ctx context.Context, binary, filename string) (*cubScanResult, error) {
	args := []string{"--format", "json"}

	catalogPath := resolveCatalogPath()
	if catalogPath != "" {
		args = append(args, "--catalog", catalogPath)
	}

	args = append(args, filename)

	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// cub-scan exits non-zero when findings exist (like cub-scout scan).
		// Only treat as real error if stdout is empty (no JSON output).
		if stdout.Len() == 0 {
			return nil, fmt.Errorf("cub-scan failed: %w: %s", err, stderr.String())
		}
	}

	var result cubScanResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse cub-scan JSON output: %w", err)
	}

	return &result, nil
}

// --- cub-scan output types (mirrors confighubai/confighub-scan schema) ---

// cubScanResult is the top-level output from the cub-scan binary.
type cubScanResult struct {
	Findings  []cubScanFinding `json:"findings"`
	ScannedAt string           `json:"scanned_at,omitempty"`
	File      string           `json:"file,omitempty"`
	Summary   *cubScanSummary  `json:"summary,omitempty"`
}

// cubScanFinding represents a single finding from cub-scan.
type cubScanFinding struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Category     string             `json:"category"`
	Track        string             `json:"track"`
	Severity     string             `json:"severity"`
	Confidence   string             `json:"confidence,omitempty"`
	Tool         string             `json:"tool,omitempty"`
	Resource     cubScanResourceRef `json:"resource"`
	Message      string             `json:"message"`
	RemedyType   string             `json:"remedy_type,omitempty"`
	RemedySafety string             `json:"remedy_safety,omitempty"`
	Remediation  cubScanRemediation `json:"remediation"`
}

// cubScanResourceRef identifies a Kubernetes resource in cub-scan output.
type cubScanResourceRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// cubScanRemediation holds remediation info from cub-scan.
type cubScanRemediation struct {
	Steps    []string `json:"steps,omitempty"`
	Commands []string `json:"commands,omitempty"`
}

// cubScanSummary is the optional summary from cub-scan.
type cubScanSummary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Info     int `json:"info"`
}

// --- mapping cub-scan → agent.StaticScanResult ---

// mapCubScanResult converts cub-scan output to cub-scout's StaticScanResult.
func mapCubScanResult(cs *cubScanResult, filename string) *agent.StaticScanResult {
	result := &agent.StaticScanResult{
		File:      filename,
		ScannedAt: time.Now(),
		Findings:  make([]agent.StaticFinding, 0, len(cs.Findings)),
	}

	for _, f := range cs.Findings {
		finding := agent.StaticFinding{
			CCVEID:       f.ID,
			Name:         f.Name,
			Kind:         f.Resource.Kind,
			ResourceName: f.Resource.Name,
			Namespace:    f.Resource.Namespace,
			Severity:     f.Severity,
			Category:     f.Category,
			Message:      f.Message,
		}

		// Use first remediation command if available, otherwise first step.
		if len(f.Remediation.Commands) > 0 {
			finding.Remediation = f.Remediation.Commands[0]
		} else if len(f.Remediation.Steps) > 0 {
			finding.Remediation = f.Remediation.Steps[0]
		}

		result.Findings = append(result.Findings, finding)
	}

	result.ResourceCount = len(cs.Findings) // best estimate; cub-scan doesn't report resource count separately

	return result
}

// --- risk catalog loading (#191) ---

// riskCatalogEntry represents a single entry in risk-catalog-v1.json.
// The catalog may contain more fields than we map; we only extract what
// PolicyEntry needs.
type riskCatalogEntry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Category string `json:"category"`
}

// loadRiskCatalog parses risk-catalog-v1.json into PolicyEntry slice.
// The catalog file is a JSON array of entries.
func loadRiskCatalog(path string) ([]PolicyEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read risk catalog: %w", err)
	}

	var entries []riskCatalogEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse risk catalog: %w", err)
	}

	policies := make([]PolicyEntry, 0, len(entries))
	for _, e := range entries {
		if e.ID == "" {
			continue // skip entries without an ID
		}
		policies = append(policies, PolicyEntry{
			ID:       e.ID,
			Name:     e.Name,
			Severity: e.Severity,
			Category: e.Category,
		})
	}

	return policies, nil
}

func exportClusterResourcesToManifest(ctx context.Context, opts ClusterScanOpts) (string, func(), error) {
	dc, err := discovery.NewDiscoveryClientForConfig(opts.Config)
	if err != nil {
		return "", nil, err
	}
	dyn, err := dynamic.NewForConfig(opts.Config)
	if err != nil {
		return "", nil, err
	}

	apiResourceLists, err := dc.ServerPreferredResources()
	if err != nil && !discovery.IsGroupDiscoveryFailedError(err) {
		return "", nil, err
	}

	var resources []unstructured.Unstructured
	for _, apiList := range apiResourceLists {
		gv, err := schema.ParseGroupVersion(apiList.GroupVersion)
		if err != nil {
			continue
		}
		for _, ar := range apiList.APIResources {
			if strings.Contains(ar.Name, "/") || !supportsList(ar.Verbs) {
				continue
			}
			gvr := schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: ar.Name}

			var list *unstructured.UnstructuredList
			if ar.Namespaced {
				if opts.Namespace != "" {
					list, err = dyn.Resource(gvr).Namespace(opts.Namespace).List(ctx, v1.ListOptions{})
				} else {
					list, err = dyn.Resource(gvr).Namespace(v1.NamespaceAll).List(ctx, v1.ListOptions{})
				}
			} else {
				list, err = dyn.Resource(gvr).List(ctx, v1.ListOptions{})
			}
			if err != nil {
				continue
			}
			resources = append(resources, list.Items...)
		}
	}

	sort.Slice(resources, func(i, j int) bool {
		a := resources[i]
		b := resources[j]
		if a.GetAPIVersion() != b.GetAPIVersion() {
			return a.GetAPIVersion() < b.GetAPIVersion()
		}
		if a.GetKind() != b.GetKind() {
			return a.GetKind() < b.GetKind()
		}
		if a.GetNamespace() != b.GetNamespace() {
			return a.GetNamespace() < b.GetNamespace()
		}
		return a.GetName() < b.GetName()
	})

	tmpFile, err := os.CreateTemp("", "cub-scout-scan-*.yaml")
	if err != nil {
		return "", nil, err
	}
	defer tmpFile.Close()

	for i, obj := range resources {
		cleanObjectForExport(&obj)
		y, err := yaml.Marshal(obj.Object)
		if err != nil {
			continue
		}
		if i > 0 {
			if _, err := tmpFile.WriteString("---\n"); err != nil {
				return "", nil, err
			}
		}
		if _, err := tmpFile.Write(y); err != nil {
			return "", nil, err
		}
	}

	path := tmpFile.Name()
	cleanup := func() { _ = os.Remove(path) }
	return path, cleanup, nil
}

func supportsList(verbs v1.Verbs) bool {
	for _, v := range verbs {
		if v == "list" {
			return true
		}
	}
	return false
}

func cleanObjectForExport(obj *unstructured.Unstructured) {
	unstructured.RemoveNestedField(obj.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(obj.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(obj.Object, "metadata", "uid")
	unstructured.RemoveNestedField(obj.Object, "metadata", "generation")
	unstructured.RemoveNestedField(obj.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(obj.Object, "metadata", "selfLink")
	unstructured.RemoveNestedField(obj.Object, "status")
}

func persistRawFindingsV1(cs *cubScanResult) error {
	dir := os.Getenv("CUB_SCOUT_SCAN_RAW_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	payload := struct {
		Version   string           `json:"version"`
		ScannedAt string           `json:"scanned_at,omitempty"`
		Findings  []cubScanFinding `json:"findings"`
	}{
		Version:   "risk-scan-findings-v1",
		ScannedAt: cs.ScannedAt,
		Findings:  cs.Findings,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	name := filepath.Join(dir, fmt.Sprintf("risk-scan-findings-v1-%d.json", time.Now().UnixNano()))
	return os.WriteFile(name, b, 0o644)
}
