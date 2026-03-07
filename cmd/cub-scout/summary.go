// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/confighub/cub-scout/internal/summarystore"
	"github.com/spf13/cobra"
)

var (
	summaryListSince     string
	summaryListFormat    string
	summaryListJSON      bool
	summaryListType      string
	summaryListCluster   string
	summaryListNamespace string

	summaryNowFn    = time.Now
	newSummaryStore = newSummaryStoreDefault
)

type summaryListResult struct {
	Since   string                `json:"since"`
	Count   int                   `json:"count"`
	Entries []summarystore.Record `json:"entries"`
}

var summaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Query connected summary storage",
}

var summaryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List persisted connected summaries by time window",
	Long: `List connected scan/status summary artifacts from local persistent storage.

Examples:
  cub-scout summary list
  cub-scout summary list --since 7d --type scan --format md
  cub-scout summary list --cluster kind-dev --namespace prod --json`,
	RunE: runSummaryList,
}

func init() {
	summaryCmd.AddCommand(summaryListCmd)
	rootCmd.AddCommand(summaryCmd)

	summaryListCmd.Flags().StringVar(&summaryListSince, "since", "24h", "Lookback window (examples: 24h, 7d, 2w)")
	summaryListCmd.Flags().StringVar(&summaryListFormat, "format", "ascii", "Output format: ascii, json, md")
	summaryListCmd.Flags().BoolVar(&summaryListJSON, "json", false, "Output as JSON (shorthand for --format json)")
	summaryListCmd.Flags().StringVar(&summaryListType, "type", "", "Filter by summary type (scan, gitops-status)")
	summaryListCmd.Flags().StringVar(&summaryListCluster, "cluster", "", "Filter by cluster")
	summaryListCmd.Flags().StringVarP(&summaryListNamespace, "namespace", "n", "", "Filter by namespace")
}

func defaultSummaryStoreDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("CUB_SCOUT_SUMMARY_DIR")); override != "" {
		return override, nil
	}
	return summarystore.DefaultRootDir()
}

func summaryRetentionDaysFromEnv() int {
	value := strings.TrimSpace(os.Getenv("CUB_SCOUT_SUMMARY_RETENTION_DAYS"))
	if value == "" {
		return 30
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 30
	}
	return n
}

func newSummaryStoreDefault() (*summarystore.Store, error) {
	dir, err := defaultSummaryStoreDir()
	if err != nil {
		return nil, err
	}
	return summarystore.New(summarystore.Options{
		RootDir:       dir,
		RetentionDays: summaryRetentionDaysFromEnv(),
		Now:           summaryNowFn,
	})
}

func runSummaryList(cmd *cobra.Command, args []string) error {
	format := strings.ToLower(strings.TrimSpace(summaryListFormat))
	if format == "" {
		format = "ascii"
	}
	if summaryListJSON {
		format = "json"
	}
	if format != "ascii" && format != "json" && format != "md" {
		return fmt.Errorf("invalid --format %q (valid: ascii, json, md)", summaryListFormat)
	}

	window, err := parseHistorySince(summaryListSince)
	if err != nil {
		return err
	}

	store, err := newSummaryStore()
	if err != nil {
		return fmt.Errorf("open summary store: %w", err)
	}

	entries, err := store.List(summarystore.Query{
		Since:     summaryNowFn().UTC().Add(-window),
		Type:      strings.TrimSpace(summaryListType),
		Cluster:   strings.TrimSpace(summaryListCluster),
		Namespace: strings.TrimSpace(summaryListNamespace),
	})
	if err != nil {
		return fmt.Errorf("query summary store: %w", err)
	}

	payload := summaryListResult{
		Since:   strings.TrimSpace(summaryListSince),
		Count:   len(entries),
		Entries: entries,
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	case "md":
		fmt.Print(renderSummaryListMarkdown(payload))
		return nil
	default:
		fmt.Print(renderSummaryListASCII(payload))
		return nil
	}
}

func renderSummaryListASCII(result summaryListResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Connected Summaries (last %s)\n\n", result.Since))
	if len(result.Entries) == 0 {
		b.WriteString("No connected summaries recorded for this window.\n")
		return b.String()
	}

	b.WriteString("Time (UTC)           Type            Cluster            Namespace    Risk(c/w/i)   Sync(f/o)   Drift\n")
	b.WriteString("-------------------  --------------  -----------------  -----------  ------------  ----------  -----\n")
	for _, entry := range result.Entries {
		namespace := entry.Scope.Namespace
		if namespace == "" {
			namespace = "-"
		}
		b.WriteString(fmt.Sprintf("%-19s  %-14s  %-17s  %-11s  %4d/%-4d/%-4d  %4d/%-4d  %-5d\n",
			entry.Timestamp.UTC().Format("2006-01-02 15:04"),
			entry.Type,
			entry.Cluster,
			namespace,
			entry.Metrics.RiskCritical,
			entry.Metrics.RiskWarning,
			entry.Metrics.RiskInfo,
			entry.Metrics.SyncFailed,
			entry.Metrics.SyncOutOfSync,
			entry.Metrics.DriftTotal,
		))
	}
	return b.String()
}

func renderSummaryListMarkdown(result summaryListResult) string {
	var b strings.Builder
	b.WriteString("## Connected Summary Storage\n\n")
	b.WriteString(fmt.Sprintf("- Window: `%s`\n", result.Since))
	b.WriteString(fmt.Sprintf("- Entries: `%d`\n\n", result.Count))
	if len(result.Entries) == 0 {
		b.WriteString("No connected summaries recorded for this window.\n")
		return b.String()
	}

	b.WriteString("| Time (UTC) | Type | Cluster | Namespace | Risk C/W/I | Sync Failed/OutOfSync | Drift |\n")
	b.WriteString("|---|---|---|---|---:|---:|---:|\n")
	for _, entry := range result.Entries {
		namespace := entry.Scope.Namespace
		if namespace == "" {
			namespace = "-"
		}
		b.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` | `%s` | `%d/%d/%d` | `%d/%d` | `%d` |\n",
			entry.Timestamp.UTC().Format(time.RFC3339),
			entry.Type,
			entry.Cluster,
			namespace,
			entry.Metrics.RiskCritical,
			entry.Metrics.RiskWarning,
			entry.Metrics.RiskInfo,
			entry.Metrics.SyncFailed,
			entry.Metrics.SyncOutOfSync,
			entry.Metrics.DriftTotal,
		))
	}
	return b.String()
}
