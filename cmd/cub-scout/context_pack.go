// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/spf13/cobra"
)

var (
	contextPackNamespace string
	contextPackFormat    string
	contextPackTopRisks  int
	contextPackTraceN    int
	contextPackMaxBytes  int
	contextPackNowFn     = time.Now
)

var contextPackCmd = &cobra.Command{
	Use:   "context-pack",
	Short: "Export deterministic AI context-pack JSON (v2)",
	Long: `Export a compact, model-agnostic Kubernetes evidence bundle for AI handoffs.

The pack includes ownership summary, top risks, trace seeds, and command evidence references.
`,
	RunE: runContextPack,
}

func init() {
	rootCmd.AddCommand(contextPackCmd)
	contextPackCmd.Flags().StringVarP(&contextPackNamespace, "namespace", "n", "", "Namespace scope (default: all namespaces)")
	contextPackCmd.Flags().StringVar(&contextPackFormat, "format", "json", "Output format (json)")
	contextPackCmd.Flags().IntVar(&contextPackTopRisks, "top-risks", 5, "Number of risk issues to include")
	contextPackCmd.Flags().IntVar(&contextPackTraceN, "trace-seeds", 5, "Number of trace seed resources to include")
	contextPackCmd.Flags().IntVar(&contextPackMaxBytes, "max-bytes", 16384, "Maximum JSON output size in bytes")
}

type contextPackV2 struct {
	Version          string                   `json:"version"`
	GeneratedAt      string                   `json:"generatedAt"`
	Cluster          string                   `json:"cluster"`
	Namespace        string                   `json:"namespace"`
	Provenance       contextPackProvenance    `json:"provenance"`
	OwnershipSummary DoctorOwnershipSummary   `json:"ownershipSummary"`
	TopRisks         []DoctorIssue            `json:"topRisks"`
	TraceSeeds       []contextPackTraceSeed   `json:"traceSeeds"`
	CommandEvidence  []contextPackCommandHint `json:"commandEvidence"`
	Truncated        bool                     `json:"truncated"`
	SizeBytes        int                      `json:"sizeBytes"`
}

type contextPackProvenance struct {
	Source        string `json:"source"`
	Deterministic bool   `json:"deterministic"`
	Confidence    string `json:"confidence"`
}

type contextPackTraceSeed struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Owner        string `json:"owner"`
	Status       string `json:"status"`
	Confidence   string `json:"confidence"`
	TraceCommand string `json:"traceCommand"`
}

type contextPackCommandHint struct {
	Command string `json:"command"`
	Purpose string `json:"purpose"`
}

type contextPackInput struct {
	Cluster   string                   `json:"cluster"`
	Namespace string                   `json:"namespace"`
	Entries   []MapEntry               `json:"entries"`
	Findings  []scan.NormalizedFinding `json:"findings"`
}

func runContextPack(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(contextPackFormat))
	if format != "json" {
		return fmt.Errorf("invalid --format %q (valid: json)", contextPackFormat)
	}
	if contextPackTopRisks < 0 {
		return fmt.Errorf("--top-risks must be >= 0")
	}
	if contextPackTraceN < 0 {
		return fmt.Errorf("--trace-seeds must be >= 0")
	}
	if contextPackMaxBytes <= 0 {
		return fmt.Errorf("--max-bytes must be > 0")
	}

	input, source, err := loadContextPackInput(cmd.Context(), strings.TrimSpace(contextPackNamespace))
	if err != nil {
		return err
	}

	namespace := "all"
	if strings.TrimSpace(contextPackNamespace) != "" {
		namespace = strings.TrimSpace(contextPackNamespace)
	} else if strings.TrimSpace(input.Namespace) != "" {
		namespace = strings.TrimSpace(input.Namespace)
	}

	cluster := strings.TrimSpace(input.Cluster)
	if cluster == "" {
		cluster = getClusterName()
	}

	now := contextPackNowFn().UTC()
	if raw := strings.TrimSpace(os.Getenv("CUB_SCOUT_TEST_TIME")); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			now = parsed.UTC()
		}
	}

	summary := buildDoctorSummary(input.Entries, input.Findings, cluster, namespace, contextPackTopRisks)
	pack := contextPackV2{
		Version:          "v2",
		GeneratedAt:      now.Format(time.RFC3339),
		Cluster:          cluster,
		Namespace:        namespace,
		OwnershipSummary: summary.Ownership,
		TopRisks:         summary.TopIssues,
		TraceSeeds:       buildContextPackTraceSeeds(input.Entries, contextPackTraceN),
		CommandEvidence:  buildContextPackCommandHints(namespace),
		Provenance: contextPackProvenance{
			Source:        source,
			Deterministic: source == "fixture",
			Confidence:    contextPackConfidence(source),
		},
	}

	pack, payload, err := applyContextPackSizeLimit(pack, contextPackMaxBytes)
	if err != nil {
		return err
	}

	if _, err := os.Stdout.Write(payload); err != nil {
		return err
	}
	if len(payload) == 0 || payload[len(payload)-1] != '\n' {
		fmt.Println()
	}
	_ = pack
	return nil
}

func loadContextPackInput(ctx context.Context, namespace string) (contextPackInput, string, error) {
	if fixturePath := strings.TrimSpace(os.Getenv("CUB_SCOUT_TEST_CONTEXT_PACK_INPUT_JSON")); fixturePath != "" {
		var in contextPackInput
		b, err := os.ReadFile(fixturePath)
		if err != nil {
			return contextPackInput{}, "", fmt.Errorf("read context-pack fixture: %w", err)
		}
		if err := json.Unmarshal(b, &in); err != nil {
			return contextPackInput{}, "", fmt.Errorf("parse context-pack fixture: %w", err)
		}
		return in, "fixture", nil
	}

	entries, cluster, err := collectDoctorEntries(ctx, namespace)
	if err != nil {
		return contextPackInput{}, "", withKubeRecoveryHint(err, "cub-scout context-pack")
	}

	findings, err := collectDoctorFindings(ctx, namespace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Note: context-pack risk scan unavailable: %v\n", err)
		findings = nil
	}

	return contextPackInput{
		Cluster:   cluster,
		Namespace: namespace,
		Entries:   entries,
		Findings:  findings,
	}, "cluster", nil
}

func buildContextPackTraceSeeds(entries []MapEntry, limit int) []contextPackTraceSeed {
	if limit <= 0 {
		return nil
	}
	items := append([]MapEntry(nil), entries...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Name < items[j].Name
	})

	out := make([]contextPackTraceSeed, 0, limit)
	for _, e := range items {
		if strings.TrimSpace(e.Kind) == "" || strings.TrimSpace(e.Name) == "" {
			continue
		}
		ns := strings.TrimSpace(e.Namespace)
		nsFlag := ""
		if ns != "" {
			nsFlag = " -n " + ns
		}
		kind := strings.ToLower(strings.TrimSpace(e.Kind))
		out = append(out, contextPackTraceSeed{
			Kind:         e.Kind,
			Name:         e.Name,
			Namespace:    e.Namespace,
			Owner:        e.Owner,
			Status:       e.Status,
			Confidence:   contextPackSeedConfidence(e.Owner),
			TraceCommand: fmt.Sprintf("cub-scout trace %s/%s%s", kind, e.Name, nsFlag),
		})
		if len(out) == limit {
			break
		}
	}
	return out
}

func buildContextPackCommandHints(namespace string) []contextPackCommandHint {
	nsFlag := ""
	if strings.TrimSpace(namespace) != "" && namespace != "all" {
		nsFlag = " -n " + strings.TrimSpace(namespace)
	}
	return []contextPackCommandHint{
		{
			Command: fmt.Sprintf("cub-scout map list --json%s", nsFlag),
			Purpose: "ownership and inventory baseline",
		},
		{
			Command: fmt.Sprintf("cub-scout scan --json%s", nsFlag),
			Purpose: "risk findings and severity summary",
		},
		{
			Command: fmt.Sprintf("cub-scout doctor --format json%s", nsFlag),
			Purpose: "compact health + ownership + risk summary",
		},
	}
}

func contextPackSeedConfidence(owner string) string {
	switch strings.TrimSpace(owner) {
	case "Flux", "ArgoCD", "Helm":
		return "high"
	case "Native":
		return "low"
	default:
		return "medium"
	}
}

func contextPackConfidence(source string) string {
	switch source {
	case "fixture":
		return "high"
	case "cluster":
		return "medium"
	default:
		return "low"
	}
}

func applyContextPackSizeLimit(pack contextPackV2, maxBytes int) (contextPackV2, []byte, error) {
	if maxBytes <= 0 {
		return contextPackV2{}, nil, fmt.Errorf("maxBytes must be > 0")
	}

	marshal := func(p contextPackV2) ([]byte, error) {
		return json.MarshalIndent(p, "", "  ")
	}

	payload, err := marshal(pack)
	if err != nil {
		return contextPackV2{}, nil, err
	}

	for len(payload) > maxBytes && (len(pack.TopRisks) > 0 || len(pack.TraceSeeds) > 0) {
		if len(pack.TopRisks) >= len(pack.TraceSeeds) && len(pack.TopRisks) > 0 {
			pack.TopRisks = pack.TopRisks[:len(pack.TopRisks)-1]
		} else if len(pack.TraceSeeds) > 0 {
			pack.TraceSeeds = pack.TraceSeeds[:len(pack.TraceSeeds)-1]
		}
		pack.Truncated = true
		payload, err = marshal(pack)
		if err != nil {
			return contextPackV2{}, nil, err
		}
	}

	for i := 0; i < 3; i++ {
		pack.SizeBytes = len(payload)
		payload, err = marshal(pack)
		if err != nil {
			return contextPackV2{}, nil, err
		}
	}

	if len(payload) > maxBytes {
		return contextPackV2{}, nil, fmt.Errorf("context-pack cannot fit within --max-bytes=%d", maxBytes)
	}
	return pack, payload, nil
}
