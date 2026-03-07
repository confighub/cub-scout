// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/confighub/cub-scout/internal/scan"
	"github.com/confighub/cub-scout/internal/summarystore"
	"github.com/confighub/cub-scout/pkg/hub"
)

var summaryConnectedFn = func() bool {
	return hub.NewClient().RequireConnected() == nil
}

func buildScanSummaryRecord(result *scan.CombinedResult, cluster, namespace string, now time.Time) (summarystore.Record, error) {
	if result == nil {
		return summarystore.Record{}, fmt.Errorf("scan summary result is nil")
	}

	normalized := scan.Normalize(result)

	return summarystore.Record{
		SchemaVersion: summarystore.SchemaVersion,
		Timestamp:     now.UTC(),
		Type:          "scan",
		Cluster:       strings.TrimSpace(cluster),
		Scope: summarystore.Scope{
			Namespace: strings.TrimSpace(namespace),
		},
		Metrics: summarystore.Metrics{
			RiskTotal:    normalized.Summary.Total,
			RiskCritical: normalized.Summary.Critical,
			RiskWarning:  normalized.Summary.Warning,
			RiskInfo:     normalized.Summary.Info,
		},
		Source: "cub-scout scan",
	}, nil
}

func buildGitOpsSummaryRecord(summary GitOpsSummary, cluster, namespace string, now time.Time) summarystore.Record {
	syncOutOfSync := 0
	for _, deployer := range summary.Deployers {
		syncStatus := strings.ToLower(strings.TrimSpace(deployer.SyncStatus))
		stage := strings.ToLower(strings.TrimSpace(deployer.Stage))
		if syncStatus == "outofsync" || (!deployer.IsHealthy() && stage == "sync") {
			syncOutOfSync++
		}
	}

	syncFailed := summary.GetFailedDeployerCount() + summary.GetFailedSourceCount()
	syncTotal := len(summary.Deployers) + len(summary.Sources)

	return summarystore.Record{
		SchemaVersion: summarystore.SchemaVersion,
		Timestamp:     now.UTC(),
		Type:          "gitops-status",
		Cluster:       strings.TrimSpace(cluster),
		Scope: summarystore.Scope{
			Namespace: strings.TrimSpace(namespace),
		},
		Metrics: summarystore.Metrics{
			SyncTotal:     syncTotal,
			SyncFailed:    syncFailed,
			SyncOutOfSync: syncOutOfSync,
			DriftTotal:    syncOutOfSync,
			RiskTotal:     syncFailed,
			RiskWarning:   syncFailed,
		},
		Source: "cub-scout gitops status",
	}
}

func detectSummaryCluster() string {
	cluster := strings.TrimSpace(getCurrentContext())
	if cluster == "" || cluster == "unknown" {
		cluster = strings.TrimSpace(getClusterName())
	}
	if cluster == "" {
		cluster = "default"
	}
	return cluster
}

func persistSummaryRecord(record summarystore.Record) error {
	store, err := newSummaryStore()
	if err != nil {
		return err
	}
	return store.Write(record)
}

func persistConnectedScanSummary(result *scan.CombinedResult, namespace string) {
	if !summaryConnectedFn() {
		return
	}
	record, err := buildScanSummaryRecord(result, detectSummaryCluster(), namespace, summaryNowFn())
	if err != nil {
		warnSummaryPersistence(err)
		return
	}
	if err := persistSummaryRecord(record); err != nil {
		warnSummaryPersistence(err)
	}
}

func persistConnectedGitOpsSummary(summary GitOpsSummary, namespace string) {
	if !summaryConnectedFn() {
		return
	}
	record := buildGitOpsSummaryRecord(summary, detectSummaryCluster(), namespace, summaryNowFn())
	if err := persistSummaryRecord(record); err != nil {
		warnSummaryPersistence(err)
	}
}

func warnSummaryPersistence(err error) {
	if err == nil {
		return
	}
	if os.Getenv("CUB_SCOUT_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: summary persistence skipped: %v\n", err)
}
