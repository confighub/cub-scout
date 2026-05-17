// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	fleetOutliersFormat string
	fleetOutliersJSON   bool

	loadFleetOutlierUnitsFn      = loadFleetOutlierUnits
	errFleetOutliersNotConnected = errors.New("fleet views require ConfigHub")
)

type fleetUnitSnapshot struct {
	UnitSlug string `json:"unitSlug"`
	Cluster  string `json:"cluster"`
	Revision int    `json:"revision"`
	Space    string `json:"space,omitempty"`
}

type fleetOutlierFinding struct {
	Kind        string `json:"kind"` // revision, missing
	Unit        string `json:"unit"`
	Revision    int    `json:"revision,omitempty"`
	FleetNorm   int    `json:"fleetNorm,omitempty"`
	PresentIn   int    `json:"presentIn,omitempty"`
	Total       int    `json:"total,omitempty"`
	Description string `json:"description"`
}

type fleetOutlierClusterReport struct {
	Cluster  string                `json:"cluster"`
	Outliers []fleetOutlierFinding `json:"outliers,omitempty"`
}

type fleetOutlierSummary struct {
	ClusterCount        int `json:"clusterCount"`
	OutlierClusterCount int `json:"outlierClusterCount"`
}

type fleetOutlierReport struct {
	Summary   fleetOutlierSummary                   `json:"summary"`
	Clusters  []string                              `json:"clusters"`
	ByCluster map[string]*fleetOutlierClusterReport `json:"byCluster"`
}

var fleetTopCmd = &cobra.Command{
	Use:   "fleet",
	Short: "Fleet-level connected insights",
}

var fleetOutliersCmd = &cobra.Command{
	Use:   "outliers",
	Short: "Identify clusters that diverge from fleet norms",
	Long: `Detect outlier clusters by comparing unit revision and unit presence across clusters.

Examples:
  cub-scout fleet outliers
  cub-scout fleet outliers --format md
  cub-scout fleet outliers --json`,
	RunE: runFleetOutliers,
}

func init() {
	fleetTopCmd.AddCommand(fleetOutliersCmd)
	rootCmd.AddCommand(fleetTopCmd)

	fleetOutliersCmd.Flags().StringVar(&fleetOutliersFormat, "format", "ascii", "Output format: ascii, json, md")
	fleetOutliersCmd.Flags().BoolVar(&fleetOutliersJSON, "json", false, "Output as JSON (shorthand for --format json)")
}

func runFleetOutliers(cmd *cobra.Command, args []string) error {
	units, err := loadFleetOutlierUnitsFn()
	if err != nil {
		return fmt.Errorf("%w. Run: cub auth login", errFleetOutliersNotConnected)
	}

	report, err := buildFleetOutlierReport(units)
	if err != nil {
		return err
	}

	format := strings.ToLower(strings.TrimSpace(fleetOutliersFormat))
	if format == "" {
		format = "ascii"
	}
	if fleetOutliersJSON {
		format = "json"
	}

	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "md":
		fmt.Print(renderFleetOutliersMarkdown(report))
		return nil
	case "ascii":
		fmt.Print(renderFleetOutliersASCII(report))
		return nil
	default:
		return fmt.Errorf("invalid --format %q (valid: ascii, json, md)", fleetOutliersFormat)
	}
}

func loadFleetOutlierUnits() ([]fleetUnitSnapshot, error) {
	cmd := exec.Command("cub", "unit", "list", "--json", "--quiet")
	out, err := cmd.Output()
	if err != nil {
		return nil, errFleetOutliersNotConnected
	}

	unitList, err := parseCubUnitListJSON(out)
	if err != nil {
		return nil, fmt.Errorf("parse unit list: %w", err)
	}

	units := make([]fleetUnitSnapshot, 0, len(unitList))
	for _, item := range unitList {
		cluster := strings.TrimSpace(item.TargetSlug)
		if cluster == "" {
			continue
		}
		units = append(units, fleetUnitSnapshot{
			UnitSlug: strings.TrimSpace(item.UnitSlug),
			Cluster:  cluster,
			Revision: item.HeadRevisionNum,
			Space:    strings.TrimSpace(item.SpaceSlug),
		})
	}
	return units, nil
}

func buildFleetOutlierReport(units []fleetUnitSnapshot) (fleetOutlierReport, error) {
	clusterSet := make(map[string]struct{})
	unitClusters := make(map[string]map[string]int)
	for _, unit := range units {
		if strings.TrimSpace(unit.UnitSlug) == "" || strings.TrimSpace(unit.Cluster) == "" {
			continue
		}
		clusterSet[unit.Cluster] = struct{}{}
		if unitClusters[unit.UnitSlug] == nil {
			unitClusters[unit.UnitSlug] = map[string]int{}
		}
		unitClusters[unit.UnitSlug][unit.Cluster] = unit.Revision
	}

	clusters := make([]string, 0, len(clusterSet))
	for cluster := range clusterSet {
		clusters = append(clusters, cluster)
	}
	sort.Strings(clusters)
	if len(clusters) < 2 {
		return fleetOutlierReport{}, fmt.Errorf("fleet comparison requires 2+ clusters")
	}

	report := fleetOutlierReport{
		Summary: fleetOutlierSummary{
			ClusterCount: len(clusters),
		},
		Clusters:  clusters,
		ByCluster: map[string]*fleetOutlierClusterReport{},
	}
	for _, cluster := range clusters {
		report.ByCluster[cluster] = &fleetOutlierClusterReport{
			Cluster:  cluster,
			Outliers: make([]fleetOutlierFinding, 0),
		}
	}

	unitNames := make([]string, 0, len(unitClusters))
	for unit := range unitClusters {
		unitNames = append(unitNames, unit)
	}
	sort.Strings(unitNames)

	for _, unit := range unitNames {
		perCluster := unitClusters[unit]
		norm := modeRevision(perCluster)
		presentIn := len(perCluster)

		for _, cluster := range clusters {
			revision, exists := perCluster[cluster]
			if !exists {
				report.ByCluster[cluster].Outliers = append(report.ByCluster[cluster].Outliers, fleetOutlierFinding{
					Kind:        "missing",
					Unit:        unit,
					PresentIn:   presentIn,
					Total:       len(clusters),
					Description: fmt.Sprintf("Missing: unit/%s (present in %d/%d clusters)", unit, presentIn, len(clusters)),
				})
				continue
			}
			if revision == norm {
				continue
			}
			delta := norm - revision
			report.ByCluster[cluster].Outliers = append(report.ByCluster[cluster].Outliers, fleetOutlierFinding{
				Kind:      "revision",
				Unit:      unit,
				Revision:  revision,
				FleetNorm: norm,
				Description: fmt.Sprintf("unit/%s: revision %d (fleet norm: %d) — %d revision(s) behind",
					unit, revision, norm, delta),
			})
		}
	}

	for _, cluster := range clusters {
		sort.Slice(report.ByCluster[cluster].Outliers, func(i, j int) bool {
			left := report.ByCluster[cluster].Outliers[i]
			right := report.ByCluster[cluster].Outliers[j]
			if left.Kind != right.Kind {
				return left.Kind < right.Kind
			}
			return left.Unit < right.Unit
		})
		if len(report.ByCluster[cluster].Outliers) > 0 {
			report.Summary.OutlierClusterCount++
		}
	}

	return report, nil
}

func modeRevision(perCluster map[string]int) int {
	counts := make(map[int]int)
	norm := 0
	normCount := -1
	for _, rev := range perCluster {
		counts[rev]++
	}
	for rev, count := range counts {
		if count > normCount || (count == normCount && rev > norm) {
			norm = rev
			normCount = count
		}
	}
	return norm
}

func renderFleetOutliersASCII(report fleetOutlierReport) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Fleet Outliers (%d clusters)\n\n", report.Summary.ClusterCount))
	b.WriteString("Outliers detected:\n")
	for _, cluster := range report.Clusters {
		clusterReport := report.ByCluster[cluster]
		if clusterReport == nil || len(clusterReport.Outliers) == 0 {
			b.WriteString(fmt.Sprintf("  %s %s: consistent\n", SymOK, cluster))
			continue
		}
		b.WriteString(fmt.Sprintf("  %s %s:\n", SymWarning, cluster))
		for _, outlier := range clusterReport.Outliers {
			b.WriteString(fmt.Sprintf("    - %s\n", outlier.Description))
		}
	}
	return b.String()
}

func renderFleetOutliersMarkdown(report fleetOutlierReport) string {
	var b strings.Builder
	b.WriteString("## Fleet Outliers\n\n")
	b.WriteString(fmt.Sprintf("- Clusters: `%d`\n", report.Summary.ClusterCount))
	b.WriteString(fmt.Sprintf("- Outlier clusters: `%d`\n\n", report.Summary.OutlierClusterCount))
	b.WriteString("| Cluster | Status | Findings |\n")
	b.WriteString("|---|---|---|\n")
	for _, cluster := range report.Clusters {
		clusterReport := report.ByCluster[cluster]
		if clusterReport == nil || len(clusterReport.Outliers) == 0 {
			b.WriteString(fmt.Sprintf("| `%s` | consistent | - |\n", cluster))
			continue
		}
		findings := make([]string, 0, len(clusterReport.Outliers))
		for _, outlier := range clusterReport.Outliers {
			findings = append(findings, outlier.Description)
		}
		b.WriteString(fmt.Sprintf("| `%s` | outlier | %s |\n", cluster, strings.Join(findings, "<br>")))
	}
	return b.String()
}
