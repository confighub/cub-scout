package scan

import (
	"testing"
	"time"

	"github.com/confighub/cub-scout/pkg/agent"
)

func TestNormalize_Nil(t *testing.T) {
	got := Normalize(nil)
	if got.SchemaVersion != NormalizedSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", got.SchemaVersion, NormalizedSchemaVersion)
	}
	if len(got.Findings) != 0 {
		t.Errorf("Findings = %d items, want 0", len(got.Findings))
	}
}

func TestNormalize_EmptyResult(t *testing.T) {
	got := Normalize(&CombinedResult{})
	if got.Summary.Total != 0 {
		t.Errorf("Total = %d, want 0", got.Summary.Total)
	}
}

func TestNormalize_StateFindings(t *testing.T) {
	now := time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC)
	combined := &CombinedResult{
		State: &agent.StateScanResult{
			ScannedAt: now,
			Findings: []agent.StuckFinding{
				{
					CCVEID:      "CCVE-2025-0001",
					Category:    "STATE",
					Severity:    "critical",
					Kind:        "HelmRelease",
					Name:        "payment-api",
					Namespace:   "prod",
					Message:     "Stuck for 45m",
					Remediation: "Suspend and resume",
					Command:     "flux suspend hr payment-api -n prod",
				},
			},
		},
	}

	got := Normalize(combined)

	if got.Summary.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Summary.Total)
	}
	if got.Summary.Critical != 1 {
		t.Errorf("Critical = %d, want 1", got.Summary.Critical)
	}

	f := got.Findings[0]
	if f.ID != "CCVE-2025-0001" {
		t.Errorf("ID = %q, want CCVE-2025-0001", f.ID)
	}
	if f.Track != "state" {
		t.Errorf("Track = %q, want state", f.Track)
	}
	if f.Resource != "HelmRelease/payment-api" {
		t.Errorf("Resource = %q, want HelmRelease/payment-api", f.Resource)
	}
	if f.Namespace != "prod" {
		t.Errorf("Namespace = %q, want prod", f.Namespace)
	}
	if f.Command != "flux suspend hr payment-api -n prod" {
		t.Errorf("Command = %q, want flux suspend hr payment-api -n prod", f.Command)
	}
	if !got.ScannedAt.Equal(now) {
		t.Errorf("ScannedAt = %v, want %v", got.ScannedAt, now)
	}
}

func TestNormalize_RuntimeFindings(t *testing.T) {
	combined := &CombinedResult{
		State: &agent.StateScanResult{
			RuntimeFindings: []agent.RuntimeFailureFinding{
				{
					CCVEID:      "CCVE-2025-0690",
					Category:    "RUNTIME",
					Severity:    "critical",
					Kind:        "Pod",
					Name:        "api-1",
					Namespace:   "prod",
					FailureType: "ImagePullBackOff",
					Message:     "pull access denied",
					Remediation: "Check imagePullSecrets",
					Command:     "kubectl describe pod api-1 -n prod",
				},
			},
		},
	}

	got := Normalize(combined)
	if got.Summary.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Summary.Total)
	}

	f := got.Findings[0]
	if f.Track != "runtime" {
		t.Errorf("Track = %q, want runtime", f.Track)
	}
	if f.Resource != "Pod/api-1" {
		t.Errorf("Resource = %q, want Pod/api-1", f.Resource)
	}
	if f.Namespace != "prod" {
		t.Errorf("Namespace = %q, want prod", f.Namespace)
	}
	if f.Command != "kubectl describe pod api-1 -n prod" {
		t.Errorf("Command = %q, want kubectl describe pod api-1 -n prod", f.Command)
	}
}

func TestNormalize_KyvernoFindings(t *testing.T) {
	combined := &CombinedResult{
		Kyverno: &agent.ScanResult{
			ScannedAt: time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC),
			Findings: []agent.ScanFinding{
				{
					ID:         "finding-1",
					PolicyID:   "KPOL-001",
					PolicyName: "require-labels",
					Category:   "best-practices",
					Severity:   "warning",
					Resource:   "Deployment/nginx",
					Namespace:  "default",
					Message:    "Missing required labels",
				},
			},
		},
	}

	got := Normalize(combined)

	if got.Summary.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Summary.Total)
	}

	f := got.Findings[0]
	if f.ID != "KPOL-001" {
		t.Errorf("ID = %q, want KPOL-001 (PolicyID takes precedence)", f.ID)
	}
	if f.Track != "kyverno" {
		t.Errorf("Track = %q, want kyverno", f.Track)
	}
	if f.Title != "require-labels" {
		t.Errorf("Title = %q, want require-labels", f.Title)
	}
}

func TestNormalize_StaticFindings(t *testing.T) {
	combined := &CombinedResult{
		Static: &agent.StaticScanResult{
			ScannedAt:     time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC),
			File:          "deploy.yaml",
			ResourceCount: 1,
			Findings: []agent.StaticFinding{
				{
					CCVEID:       "CCVE-2025-0248",
					Name:         "Missing resource limits",
					Kind:         "Deployment",
					ResourceName: "nginx",
					Severity:     "warning",
					Category:     "CONFIG",
					Message:      "No resource limits defined",
					Remediation:  "Add resource limits",
				},
			},
		},
	}

	got := Normalize(combined)

	if got.Summary.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Summary.Total)
	}

	f := got.Findings[0]
	if f.ID != "CCVE-2025-0248" {
		t.Errorf("ID = %q, want CCVE-2025-0248", f.ID)
	}
	if f.Track != "static" {
		t.Errorf("Track = %q, want static", f.Track)
	}
	if f.Resource != "Deployment/nginx" {
		t.Errorf("Resource = %q, want Deployment/nginx", f.Resource)
	}
}

func TestNormalize_MixedTracks(t *testing.T) {
	combined := &CombinedResult{
		State: &agent.StateScanResult{
			ScannedAt: time.Date(2026, 2, 25, 12, 0, 0, 0, time.UTC),
			Findings: []agent.StuckFinding{
				{CCVEID: "CCVE-1", Severity: "critical", Kind: "HR", Name: "a", Namespace: "ns"},
			},
		},
		Kyverno: &agent.ScanResult{
			Findings: []agent.ScanFinding{
				{ID: "f-1", PolicyID: "KPOL-1", Severity: "warning", PolicyName: "pol", Resource: "Deploy/b", Namespace: "ns"},
			},
		},
		Static: &agent.StaticScanResult{
			Findings: []agent.StaticFinding{
				{CCVEID: "CCVE-2", Severity: "info", Name: "check", Kind: "Deploy", ResourceName: "c"},
			},
		},
	}

	got := Normalize(combined)

	if got.Summary.Total != 3 {
		t.Errorf("Total = %d, want 3", got.Summary.Total)
	}
	if got.Summary.Critical != 1 {
		t.Errorf("Critical = %d, want 1", got.Summary.Critical)
	}
	if got.Summary.Warning != 1 {
		t.Errorf("Warning = %d, want 1", got.Summary.Warning)
	}
	if got.Summary.Info != 1 {
		t.Errorf("Info = %d, want 1", got.Summary.Info)
	}

	// Verify ordering: kyverno, then state, then static (source order)
	tracks := make([]string, len(got.Findings))
	for i, f := range got.Findings {
		tracks[i] = f.Track
	}
	if tracks[0] != "kyverno" || tracks[1] != "state" || tracks[2] != "static" {
		t.Errorf("Tracks = %v, want [kyverno state static]", tracks)
	}
}

func TestNormalize_DanglingFindings(t *testing.T) {
	combined := &CombinedResult{
		Dangling: &agent.DanglingResult{
			Findings: []agent.DanglingFinding{
				{
					CCVEID:    "CCVE-2025-0100",
					Category:  "DANGLING",
					Severity:  "warning",
					Kind:      "Service",
					Name:      "orphaned-svc",
					Namespace: "default",
					Message:   "No matching pods",
				},
			},
		},
	}

	got := Normalize(combined)
	if got.Summary.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Summary.Total)
	}
	f := got.Findings[0]
	if f.Track != "dangling" {
		t.Errorf("Track = %q, want dangling", f.Track)
	}
}

func TestNormalize_TimingBombFindings(t *testing.T) {
	combined := &CombinedResult{
		TimingBombs: &agent.TimingBombResult{
			Findings: []agent.TimingBombFinding{
				{
					CCVEID:    "CCVE-2025-0200",
					Category:  "TIMING",
					Severity:  "critical",
					Kind:      "Certificate",
					Name:      "tls-cert",
					Namespace: "istio-system",
					ExpiresIn: "2d",
					Message:   "Expires in 2 days",
				},
			},
		},
	}

	got := Normalize(combined)
	if got.Summary.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Summary.Total)
	}
	f := got.Findings[0]
	if f.Track != "timing-bomb" {
		t.Errorf("Track = %q, want timing-bomb", f.Track)
	}
}

func TestNormalize_LifecycleFindings(t *testing.T) {
	combined := &CombinedResult{
		LifecycleHazards: &agent.LifecycleHazardResult{
			Findings: []agent.LifecycleHazardFinding{
				{
					Rule:        "helm-hook-ambiguity",
					Resource:    "Job/db-migrate",
					Namespace:   "prod",
					Severity:    "warning",
					Risk:        "Helm hook may conflict with ArgoCD sync",
					Remediation: "Convert to ArgoCD sync hook",
				},
			},
		},
	}

	got := Normalize(combined)
	if got.Summary.Total != 1 {
		t.Fatalf("Total = %d, want 1", got.Summary.Total)
	}
	f := got.Findings[0]
	if f.Track != "lifecycle" {
		t.Errorf("Track = %q, want lifecycle", f.Track)
	}
	if f.ID != "helm-hook-ambiguity" {
		t.Errorf("ID = %q, want helm-hook-ambiguity", f.ID)
	}
}
