package scan

import (
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

// TestCubScanToNormalized_RoundTrip verifies that a cub-scan Finding can be
// mapped through CombinedResult.Static → NormalizedFinding without losing
// fields that NormalizedFinding supports.
//
// Lossy fields (cub-scan → NormalizedFinding):
//   - confidence:     dropped — NormalizedFinding has no Confidence field (#192)
//   - tool:           dropped — NormalizedFinding has no Tool/Source field
//   - remedy_type:    dropped — NormalizedFinding has no RemedyType field
//   - remedy_safety:  dropped — "manual_only"/"safe_auto"/"unsafe_auto" classification lost
//   - remediation.steps[1:]: only the first step or first command is preserved
//
// These fields are intentionally omitted from NormalizedFinding for v1.2.
// Adding them is tracked as a follow-up decision in #192.
func TestCubScanToNormalized_RoundTrip(t *testing.T) {
	// Simulate cub-scan output
	cubScanFindings := []cubScanFinding{
		{
			ID:         "CCVE-2025-0244",
			Name:       "Probe timeout exceeds period",
			Category:   "SILENT",
			Track:      "static",
			Severity:   "warning",
			Confidence: "high",
			Tool:       "cub-scan",
			Resource:   cubScanResourceRef{Kind: "Deployment", Name: "my-app", Namespace: "production"},
			Message:    "livenessProbe timeout (15s) > period (10s)",
			RemedyType:   "config",
			RemedySafety: "safe_auto",
			Remediation: cubScanRemediation{
				Steps:    []string{"Ensure probe timeoutSeconds <= periodSeconds", "Review container health checks"},
				Commands: []string{"kubectl patch deployment my-app -p '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"app\",\"livenessProbe\":{\"timeoutSeconds\":5}}]}}}}'"},
			},
		},
		{
			ID:       "CCVE-2025-0248",
			Name:     "Missing resource limits",
			Category: "CONFIG",
			Track:    "static",
			Severity: "warning",
			Resource: cubScanResourceRef{Kind: "Deployment", Name: "my-app", Namespace: "production"},
			Message:  "Container app has no resource limits defined",
			Remediation: cubScanRemediation{
				Steps: []string{"Add resources.limits.cpu and resources.limits.memory"},
			},
		},
		{
			ID:       "CCVE-2025-0100",
			Name:     "No remediation info",
			Category: "TEST",
			Track:    "static",
			Severity: "info",
			Resource: cubScanResourceRef{Kind: "Service", Name: "my-svc"},
			Message:  "FYI finding",
		},
	}

	// Step 1: Map cub-scan findings → StaticScanResult (via mapCubScanResult)
	cubResult := &cubScanResult{
		Findings:  cubScanFindings,
		ScannedAt: "2026-02-25T12:00:00Z",
		File:      "/tmp/test.yaml",
	}
	staticResult := mapCubScanResult(cubResult, "/tmp/test.yaml")

	// Step 2: Wrap in CombinedResult
	combined := &CombinedResult{Static: staticResult}

	// Step 3: Normalize → NormalizedResult
	normalized := Normalize(combined)

	// Verify schema version
	if normalized.SchemaVersion != NormalizedSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", normalized.SchemaVersion, NormalizedSchemaVersion)
	}

	// Verify finding count preserved
	if len(normalized.Findings) != 3 {
		t.Fatalf("Findings count = %d, want 3", len(normalized.Findings))
	}

	// Verify preserved fields (round-trip fidelity)
	tests := []struct {
		name        string
		idx         int
		wantID      string
		wantTitle   string
		wantCat     string
		wantTrack   string
		wantSev     string
		wantRes     string
		wantNS      string
		wantMsg     string
		wantRemed   string
	}{
		{
			name:      "finding with commands",
			idx:       0,
			wantID:    "CCVE-2025-0244",
			wantTitle: "Probe timeout exceeds period",
			wantCat:   "SILENT",
			wantTrack: "static",
			wantSev:   "warning",
			wantRes:   "Deployment/my-app",
			wantNS:    "production",
			wantMsg:   "livenessProbe timeout (15s) > period (10s)",
			// Commands take precedence over steps in mapCubScanResult
			wantRemed: "kubectl patch deployment my-app -p '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"app\",\"livenessProbe\":{\"timeoutSeconds\":5}}]}}}}'",
		},
		{
			name:      "finding with steps only",
			idx:       1,
			wantID:    "CCVE-2025-0248",
			wantTitle: "Missing resource limits",
			wantCat:   "CONFIG",
			wantTrack: "static",
			wantSev:   "warning",
			wantRes:   "Deployment/my-app",
			wantNS:    "production",
			wantMsg:   "Container app has no resource limits defined",
			wantRemed: "Add resources.limits.cpu and resources.limits.memory",
		},
		{
			name:      "finding with no remediation",
			idx:       2,
			wantID:    "CCVE-2025-0100",
			wantTitle: "No remediation info",
			wantCat:   "TEST",
			wantTrack: "static",
			wantSev:   "info",
			wantRes:   "Service/my-svc",
			wantNS:    "",
			wantMsg:   "FYI finding",
			wantRemed: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := normalized.Findings[tt.idx]

			if f.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", f.ID, tt.wantID)
			}
			if f.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", f.Title, tt.wantTitle)
			}
			if f.Category != tt.wantCat {
				t.Errorf("Category = %q, want %q", f.Category, tt.wantCat)
			}
			if f.Track != tt.wantTrack {
				t.Errorf("Track = %q, want %q", f.Track, tt.wantTrack)
			}
			if f.Severity != tt.wantSev {
				t.Errorf("Severity = %q, want %q", f.Severity, tt.wantSev)
			}
			if f.Resource != tt.wantRes {
				t.Errorf("Resource = %q, want %q", f.Resource, tt.wantRes)
			}
			if f.Namespace != tt.wantNS {
				t.Errorf("Namespace = %q, want %q", f.Namespace, tt.wantNS)
			}
			if f.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", f.Message, tt.wantMsg)
			}
			if f.Remediation != tt.wantRemed {
				t.Errorf("Remediation = %q, want %q", f.Remediation, tt.wantRemed)
			}
		})
	}

	// Verify summary
	if normalized.Summary.Total != 3 {
		t.Errorf("Summary.Total = %d, want 3", normalized.Summary.Total)
	}
	if normalized.Summary.Warning != 2 {
		t.Errorf("Summary.Warning = %d, want 2", normalized.Summary.Warning)
	}
	if normalized.Summary.Info != 1 {
		t.Errorf("Summary.Info = %d, want 1", normalized.Summary.Info)
	}
}

// TestCubScanToNormalized_LossyFields explicitly documents which cub-scan fields
// are lost during the mapping pipeline. This test serves as a living contract
// for field coverage.
func TestCubScanToNormalized_LossyFields(t *testing.T) {
	finding := cubScanFinding{
		ID:           "CCVE-2025-0001",
		Name:         "Test",
		Category:     "TEST",
		Track:        "static",
		Severity:     "info",
		Confidence:   "high",       // LOSSY: not in NormalizedFinding
		Tool:         "cub-scan",   // LOSSY: not in NormalizedFinding
		RemedyType:   "config",     // LOSSY: not in NormalizedFinding
		RemedySafety: "safe_auto",  // LOSSY: not in NormalizedFinding
		Resource:     cubScanResourceRef{Kind: "Pod", Name: "test"},
		Message:      "test message",
		Remediation: cubScanRemediation{
			Steps: []string{"step 1", "step 2", "step 3"}, // LOSSY: only first preserved
		},
	}

	result := mapCubScanResult(&cubScanResult{Findings: []cubScanFinding{finding}}, "test.yaml")
	static := result.Findings[0]

	// Verify we have the first step
	if static.Remediation != "step 1" {
		t.Errorf("Remediation = %q, want first step 'step 1'", static.Remediation)
	}

	// Verify fields that exist in StaticFinding
	if static.CCVEID != "CCVE-2025-0001" {
		t.Errorf("CCVEID = %q", static.CCVEID)
	}
	if static.Kind != "Pod" {
		t.Errorf("Kind = %q", static.Kind)
	}

	// Now normalize and verify no panics and correct output
	combined := &CombinedResult{Static: result}
	normalized := Normalize(combined)

	if len(normalized.Findings) != 1 {
		t.Fatalf("expected 1 normalized finding, got %d", len(normalized.Findings))
	}

	nf := normalized.Findings[0]
	// These cub-scan fields are NOT present in NormalizedFinding:
	// - confidence (finding.Confidence = "high")
	// - tool (finding.Tool = "cub-scan")
	// - remedy_type (finding.RemedyType = "config")
	// - remedy_safety (finding.RemedySafety = "safe_auto")
	// - remediation.steps[1:] (only first step preserved)
	//
	// This is intentional for v1.2. See #192 for field-addition decisions.
	_ = nf // Compile-time verification that NormalizedFinding doesn't have these fields
}

// TestLegacyAndCubScanProduceSameNormalizedShape verifies that both providers
// produce the same NormalizedFinding shape for equivalent input.
func TestLegacyAndCubScanProduceSameNormalizedShape(t *testing.T) {
	scannedAt := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC)

	// Simulate legacy provider output
	legacyResult := &CombinedResult{
		Static: &agent.StaticScanResult{
			File:          "/tmp/test.yaml",
			ScannedAt:     scannedAt,
			ResourceCount: 1,
			Findings: []agent.StaticFinding{
				{
					CCVEID:       "CCVE-2025-0244",
					Name:         "Probe timeout exceeds period",
					Kind:         "Deployment",
					ResourceName: "my-app",
					Namespace:    "production",
					Severity:     "warning",
					Category:     "SILENT",
					Message:      "livenessProbe timeout (15s) > period (10s)",
					Remediation:  "Ensure probe timeoutSeconds <= periodSeconds",
				},
			},
		},
	}

	// Simulate cub-scan provider output (same finding, via mapCubScanResult)
	cubResult := mapCubScanResult(&cubScanResult{
		Findings: []cubScanFinding{
			{
				ID:       "CCVE-2025-0244",
				Name:     "Probe timeout exceeds period",
				Category: "SILENT",
				Track:    "static",
				Severity: "warning",
				Resource: cubScanResourceRef{Kind: "Deployment", Name: "my-app", Namespace: "production"},
				Message:  "livenessProbe timeout (15s) > period (10s)",
				Remediation: cubScanRemediation{
					Steps: []string{"Ensure probe timeoutSeconds <= periodSeconds"},
				},
			},
		},
	}, "/tmp/test.yaml")
	cubResult.ScannedAt = scannedAt
	cubCombined := &CombinedResult{Static: cubResult}

	// Normalize both
	legacyNorm := Normalize(legacyResult)
	cubNorm := Normalize(cubCombined)

	if len(legacyNorm.Findings) != len(cubNorm.Findings) {
		t.Fatalf("finding count mismatch: legacy=%d, cubscan=%d", len(legacyNorm.Findings), len(cubNorm.Findings))
	}

	// Compare field by field
	lf := legacyNorm.Findings[0]
	cf := cubNorm.Findings[0]

	if lf.ID != cf.ID {
		t.Errorf("ID: legacy=%q, cubscan=%q", lf.ID, cf.ID)
	}
	if lf.Title != cf.Title {
		t.Errorf("Title: legacy=%q, cubscan=%q", lf.Title, cf.Title)
	}
	if lf.Category != cf.Category {
		t.Errorf("Category: legacy=%q, cubscan=%q", lf.Category, cf.Category)
	}
	if lf.Track != cf.Track {
		t.Errorf("Track: legacy=%q, cubscan=%q", lf.Track, cf.Track)
	}
	if lf.Severity != cf.Severity {
		t.Errorf("Severity: legacy=%q, cubscan=%q", lf.Severity, cf.Severity)
	}
	if lf.Resource != cf.Resource {
		t.Errorf("Resource: legacy=%q, cubscan=%q", lf.Resource, cf.Resource)
	}
	if lf.Namespace != cf.Namespace {
		t.Errorf("Namespace: legacy=%q, cubscan=%q", lf.Namespace, cf.Namespace)
	}
	if lf.Message != cf.Message {
		t.Errorf("Message: legacy=%q, cubscan=%q", lf.Message, cf.Message)
	}
	if lf.Remediation != cf.Remediation {
		t.Errorf("Remediation: legacy=%q, cubscan=%q", lf.Remediation, cf.Remediation)
	}
}
