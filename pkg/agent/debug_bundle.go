// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package agent provides Debug Bundle v1 for packaging debug context.
//
// A Debug Bundle is a structured snapshot containing debug facts.
// It introduces NO new semantics — only packaging of existing outputs.
//
// Contract:
// - Bundles are pure packaging (no interpretation)
// - All contents are existing facts from other commands
// - Bundles are deterministic and replayable
// - Bundles can be inspected offline
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BundleFormatVersion is the current bundle format version.
const BundleFormatVersion = "v1"

// BundleMetadata contains metadata about the debug bundle.
// This is pure metadata — no interpretation of contents.
type BundleMetadata struct {
	// FormatVersion is the bundle format version (e.g., "v1")
	FormatVersion string `json:"formatVersion"`

	// CubScoutVersion is the cub-scout version that created this bundle
	CubScoutVersion string `json:"cubScoutVersion"`

	// CreatedAt is when the bundle was created
	CreatedAt time.Time `json:"createdAt"`

	// Label is an optional user-provided label (e.g., "prod-incident-123")
	Label string `json:"label,omitempty"`

	// Target identifies what was debugged
	Target BundleTarget `json:"target"`

	// Contents lists what files are included in the bundle
	Contents BundleContents `json:"contents"`

	// GitContext is optional git repository context (v0.18+)
	// This is pure metadata for workflow context — replay semantics are unchanged.
	GitContext *BundleGitContext `json:"gitContext,omitempty"`
}

// BundleGitContext contains git repository context for workflow integration.
// This is pure metadata — replay behavior does not depend on these fields.
// Added in v0.18 for CI/PR workflow context.
type BundleGitContext struct {
	// RepoRoot is the absolute path to the repository root
	RepoRoot string `json:"repoRoot,omitempty"`

	// CommitSHA is the current commit SHA (if available)
	CommitSHA string `json:"commitSHA,omitempty"`

	// Branch is the current branch name (if available)
	Branch string `json:"branch,omitempty"`

	// RemoteURL is the origin remote URL (if available)
	RemoteURL string `json:"remoteURL,omitempty"`
}

// BundleTarget identifies the debug target.
type BundleTarget struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Cluster   string `json:"cluster,omitempty"`
}

// BundleContents describes what's in the bundle.
type BundleContents struct {
	HasSession     bool `json:"hasSession"`
	HasDrift       bool `json:"hasDrift"`
	HasEvents      bool `json:"hasEvents"`
	HasLogs        bool `json:"hasLogs"`
	HasAttribution bool `json:"hasAttribution"`
}

// DebugBundle represents a complete debug bundle.
// All fields are references to existing data — no new facts.
type DebugBundle struct {
	// Metadata about the bundle
	Metadata BundleMetadata

	// Session is the debug session data (optional)
	Session *DebugSessionData `json:"session,omitempty"`

	// DriftFindings are drift findings (optional)
	DriftFindings []DriftFinding `json:"driftFindings,omitempty"`

	// Events are timeline events (optional)
	Events []TimelineEvent `json:"events,omitempty"`

	// Logs are container logs (optional)
	Logs []ContainerLogResult `json:"logs,omitempty"`

	// Attribution is the composition lineage graph (optional, v0.16+)
	Attribution *AttributionGraph `json:"attribution,omitempty"`
}

// DebugSessionData contains debug session information.
// This mirrors the debug command's session structure.
type DebugSessionData struct {
	// Target resource
	Target BundleTarget `json:"target"`

	// StartedAt is when debugging started
	StartedAt time.Time `json:"startedAt"`

	// CompletedAt is when debugging finished
	CompletedAt time.Time `json:"completedAt,omitempty"`

	// WorkloadHealth contains workload status
	WorkloadHealth *WorkloadHealthSnapshot `json:"workloadHealth,omitempty"`

	// OwnershipChain contains ownership information
	OwnershipChain *OwnershipSnapshot `json:"ownershipChain,omitempty"`

	// DeployerStatus contains pipeline status
	DeployerStatus *DeployerSnapshot `json:"deployerStatus,omitempty"`

	// SourceStatus contains source status
	SourceStatus *SourceSnapshot `json:"sourceStatus,omitempty"`

	// RootCause contains root cause analysis
	RootCause *RootCauseSnapshot `json:"rootCause,omitempty"`
}

// WorkloadHealthSnapshot is a snapshot of workload health.
// Uses PodIssue from workload_health.go.
type WorkloadHealthSnapshot struct {
	Kind                string     `json:"kind"`
	Name                string     `json:"name"`
	Namespace           string     `json:"namespace"`
	Replicas            int32      `json:"replicas"`
	ReadyReplicas       int32      `json:"readyReplicas"`
	UnavailableReplicas int32      `json:"unavailableReplicas"`
	PodIssues           []PodIssue `json:"podIssues,omitempty"`
}

// OwnershipSnapshot is a snapshot of ownership chain.
// Uses ChainLink from trace.go.
type OwnershipSnapshot struct {
	Owner       string      `json:"owner"`
	K8sChain    []ChainLink `json:"k8sChain,omitempty"`
	GitOpsChain []ChainLink `json:"gitopsChain,omitempty"`
}

// DeployerSnapshot is a snapshot of deployer status.
type DeployerSnapshot struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     bool   `json:"ready"`
	Suspended bool   `json:"suspended"`
	Stage     string `json:"stage,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
}

// SourceSnapshot is a snapshot of source status.
type SourceSnapshot struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     bool   `json:"ready"`
	URL       string `json:"url,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
}

// RootCauseSnapshot is a snapshot of root cause analysis.
type RootCauseSnapshot struct {
	Category       string   `json:"category"`
	Stage          string   `json:"stage"`
	Confidence     string   `json:"confidence"`
	Summary        string   `json:"summary"`
	ProbableCauses []string `json:"probableCauses,omitempty"`
	SuggestedFixes []string `json:"suggestedFixes,omitempty"`
}

// BundleWriter writes debug bundles to disk.
type BundleWriter struct {
	cubScoutVersion string
}

// NewBundleWriter creates a new bundle writer.
func NewBundleWriter(version string) *BundleWriter {
	return &BundleWriter{
		cubScoutVersion: version,
	}
}

// Write writes a debug bundle to the specified directory.
// The directory will be created if it doesn't exist.
func (w *BundleWriter) Write(bundle *DebugBundle, outputDir string) error {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create bundle directory: %w", err)
	}

	// Update metadata
	bundle.Metadata.FormatVersion = BundleFormatVersion
	bundle.Metadata.CubScoutVersion = w.cubScoutVersion
	bundle.Metadata.CreatedAt = time.Now().UTC()
	bundle.Metadata.Contents = BundleContents{
		HasSession:     bundle.Session != nil,
		HasDrift:       len(bundle.DriftFindings) > 0,
		HasEvents:      len(bundle.Events) > 0,
		HasLogs:        len(bundle.Logs) > 0,
		HasAttribution: bundle.Attribution != nil && !bundle.Attribution.IsEmpty(),
	}

	// Capture git context if available (v0.18+ workflow integration)
	if bundle.Metadata.GitContext == nil {
		bundle.Metadata.GitContext = CaptureGitContext()
	}

	// Write metadata.json
	if err := w.writeJSON(filepath.Join(outputDir, "metadata.json"), bundle.Metadata); err != nil {
		return fmt.Errorf("failed to write metadata.json: %w", err)
	}

	// Write session.json (if present)
	if bundle.Session != nil {
		if err := w.writeJSON(filepath.Join(outputDir, "session.json"), bundle.Session); err != nil {
			return fmt.Errorf("failed to write session.json: %w", err)
		}
	}

	// Write drift.json (if present)
	if len(bundle.DriftFindings) > 0 {
		if err := w.writeJSON(filepath.Join(outputDir, "drift.json"), bundle.DriftFindings); err != nil {
			return fmt.Errorf("failed to write drift.json: %w", err)
		}
	}

	// Write events.json (if present)
	if len(bundle.Events) > 0 {
		if err := w.writeJSON(filepath.Join(outputDir, "events.json"), bundle.Events); err != nil {
			return fmt.Errorf("failed to write events.json: %w", err)
		}
	}

	// Write logs.json (if present)
	if len(bundle.Logs) > 0 {
		if err := w.writeJSON(filepath.Join(outputDir, "logs.json"), bundle.Logs); err != nil {
			return fmt.Errorf("failed to write logs.json: %w", err)
		}
	}

	// Write attribution.json (if present and non-empty)
	if bundle.Attribution != nil && !bundle.Attribution.IsEmpty() {
		if err := w.writeJSON(filepath.Join(outputDir, "attribution.json"), bundle.Attribution); err != nil {
			return fmt.Errorf("failed to write attribution.json: %w", err)
		}
	}

	// Write README.md
	if err := w.writeReadme(outputDir, bundle); err != nil {
		return fmt.Errorf("failed to write README.md: %w", err)
	}

	return nil
}

// writeJSON writes a value as pretty-printed JSON.
func (w *BundleWriter) writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// writeReadme generates the README.md for the bundle.
func (w *BundleWriter) writeReadme(outputDir string, bundle *DebugBundle) error {
	readme := fmt.Sprintf(`# Debug Bundle

Generated by cub-scout %s at %s

## Target

- Kind: %s
- Name: %s
- Namespace: %s

## Contents

| File | Present | Description |
|------|---------|-------------|
| metadata.json | Yes | Bundle metadata |
| session.json | %s | Debug session data |
| drift.json | %s | Drift findings |
| events.json | %s | Timeline events |
| logs.json | %s | Container logs |
| attribution.json | %s | Composition lineage |

## How to Inspect

%s

## Semantic Guarantees

- This bundle contains **facts only** — no interpretations
- All data is a snapshot from the time of creation
- Bundle format version: %s
`,
		bundle.Metadata.CubScoutVersion,
		bundle.Metadata.CreatedAt.Format(time.RFC3339),
		bundle.Metadata.Target.Kind,
		bundle.Metadata.Target.Name,
		bundle.Metadata.Target.Namespace,
		yesNo(bundle.Metadata.Contents.HasSession),
		yesNo(bundle.Metadata.Contents.HasDrift),
		yesNo(bundle.Metadata.Contents.HasEvents),
		yesNo(bundle.Metadata.Contents.HasLogs),
		yesNo(bundle.Metadata.Contents.HasAttribution),
		w.inspectInstructions(outputDir),
		bundle.Metadata.FormatVersion,
	)

	return os.WriteFile(filepath.Join(outputDir, "README.md"), []byte(readme), 0644)
}

func (w *BundleWriter) inspectInstructions(outputDir string) string {
	return fmt.Sprintf("```bash\n# View bundle contents\ncub-scout debug bundle inspect %s\n\n# View specific files\ncat %s/metadata.json\ncat %s/session.json\n```", outputDir, outputDir, outputDir)
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// BundleReader reads debug bundles from disk.
type BundleReader struct{}

// NewBundleReader creates a new bundle reader.
func NewBundleReader() *BundleReader {
	return &BundleReader{}
}

// Read reads a debug bundle from the specified directory.
func (r *BundleReader) Read(bundleDir string) (*DebugBundle, error) {
	bundle := &DebugBundle{}

	// Read metadata.json (required)
	metadataPath := filepath.Join(bundleDir, "metadata.json")
	if err := r.readJSON(metadataPath, &bundle.Metadata); err != nil {
		return nil, fmt.Errorf("failed to read metadata.json: %w", err)
	}

	// Read session.json (optional)
	sessionPath := filepath.Join(bundleDir, "session.json")
	if _, err := os.Stat(sessionPath); err == nil {
		bundle.Session = &DebugSessionData{}
		if err := r.readJSON(sessionPath, bundle.Session); err != nil {
			return nil, fmt.Errorf("failed to read session.json: %w", err)
		}
	}

	// Read drift.json (optional)
	driftPath := filepath.Join(bundleDir, "drift.json")
	if _, err := os.Stat(driftPath); err == nil {
		if err := r.readJSON(driftPath, &bundle.DriftFindings); err != nil {
			return nil, fmt.Errorf("failed to read drift.json: %w", err)
		}
	}

	// Read events.json (optional)
	eventsPath := filepath.Join(bundleDir, "events.json")
	if _, err := os.Stat(eventsPath); err == nil {
		if err := r.readJSON(eventsPath, &bundle.Events); err != nil {
			return nil, fmt.Errorf("failed to read events.json: %w", err)
		}
	}

	// Read logs.json (optional)
	logsPath := filepath.Join(bundleDir, "logs.json")
	if _, err := os.Stat(logsPath); err == nil {
		if err := r.readJSON(logsPath, &bundle.Logs); err != nil {
			return nil, fmt.Errorf("failed to read logs.json: %w", err)
		}
	}

	// Read attribution.json (optional)
	attributionPath := filepath.Join(bundleDir, "attribution.json")
	if _, err := os.Stat(attributionPath); err == nil {
		bundle.Attribution = &AttributionGraph{}
		if err := r.readJSON(attributionPath, bundle.Attribution); err != nil {
			return nil, fmt.Errorf("failed to read attribution.json: %w", err)
		}
		// Update metadata if it wasn't set
		bundle.Metadata.Contents.HasAttribution = true
	}

	return bundle, nil
}

// readJSON reads and unmarshals a JSON file.
func (r *BundleReader) readJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// BundleSummary returns a summary of the bundle for display.
type BundleSummary struct {
	FormatVersion      string
	CubScoutVersion    string
	CreatedAt          time.Time
	Target             string
	Label              string
	SessionPresent     bool
	DriftCount         int
	EventCount         int
	LogCount           int
	AttributionPresent bool
	AttributionNodes   int
	AttributionEdges   int
}

// Summarize returns a summary of a bundle.
func Summarize(bundle *DebugBundle) BundleSummary {
	target := fmt.Sprintf("%s/%s", bundle.Metadata.Target.Kind, bundle.Metadata.Target.Name)
	if bundle.Metadata.Target.Namespace != "" {
		target += " in " + bundle.Metadata.Target.Namespace
	}

	summary := BundleSummary{
		FormatVersion:      bundle.Metadata.FormatVersion,
		CubScoutVersion:    bundle.Metadata.CubScoutVersion,
		CreatedAt:          bundle.Metadata.CreatedAt,
		Target:             target,
		Label:              bundle.Metadata.Label,
		SessionPresent:     bundle.Session != nil,
		DriftCount:         len(bundle.DriftFindings),
		EventCount:         len(bundle.Events),
		LogCount:           len(bundle.Logs),
		AttributionPresent: bundle.Attribution != nil && !bundle.Attribution.IsEmpty(),
	}

	if bundle.Attribution != nil {
		summary.AttributionNodes = len(bundle.Attribution.Nodes)
		summary.AttributionEdges = len(bundle.Attribution.Edges)
	}

	return summary
}

// CaptureGitContext captures git repository context from the current directory.
// Returns nil if not in a git repository or git is not available.
// This is pure metadata for workflow integration — replay semantics are unchanged.
func CaptureGitContext() *BundleGitContext {
	// Check if we're in a git repository
	repoRoot := gitCommand("rev-parse", "--show-toplevel")
	if repoRoot == "" {
		return nil
	}

	ctx := &BundleGitContext{
		RepoRoot: repoRoot,
	}

	// Get current commit SHA
	if sha := gitCommand("rev-parse", "HEAD"); sha != "" {
		ctx.CommitSHA = sha
	}

	// Get current branch
	if branch := gitCommand("rev-parse", "--abbrev-ref", "HEAD"); branch != "" && branch != "HEAD" {
		ctx.Branch = branch
	}

	// Get origin remote URL (if available)
	if remote := gitCommand("remote", "get-url", "origin"); remote != "" {
		ctx.RemoteURL = remote
	}

	return ctx
}

// gitCommand runs a git command and returns the trimmed output.
// Returns empty string on any error (git not installed, not a repo, etc.).
func gitCommand(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
