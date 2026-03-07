// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	impactFormat string
	impactJSON   bool

	loadImpactUnitCacheFn = fetchConfigHubUnits
	errImpactNotConnected = errors.New("impact analysis requires ConfigHub")
)

type impactDependent struct {
	Unit         string   `json:"unit"`
	Environments []string `json:"environments,omitempty"`
	Clusters     []string `json:"clusters,omitempty"`
}

type impactSummary struct {
	DirectDependents     int    `json:"directDependents"`
	EnvironmentsAffected int    `json:"environmentsAffected"`
	ClustersAffected     int    `json:"clustersAffected"`
	Risk                 string `json:"risk"`
}

type impactResult struct {
	Unit       string            `json:"unit"`
	Connected  bool              `json:"connected"`
	Dependents []impactDependent `json:"dependents,omitempty"`
	Summary    impactSummary     `json:"summary"`
	Notes      []string          `json:"notes,omitempty"`
}

var impactCmd = &cobra.Command{
	Use:   "impact <unit>",
	Short: "Preview blast radius for a unit change (connected mode)",
	Long: `Preview impact for one ConfigHub unit using dependency links.

Examples:
  cub-scout impact unit/shared-db-config
  cub-scout impact shared-db-config --format md
  cub-scout impact shared-db-config --json`,
	Args: cobra.ExactArgs(1),
	RunE: runImpact,
}

func init() {
	rootCmd.AddCommand(impactCmd)
	impactCmd.Flags().StringVar(&impactFormat, "format", "ascii", "Output format: ascii, json, md")
	impactCmd.Flags().BoolVar(&impactJSON, "json", false, "Output as JSON (shorthand for --format json)")
}

func runImpact(cmd *cobra.Command, args []string) error {
	cache, err := loadImpactUnitCacheFn()
	if err != nil {
		return fmt.Errorf("%w. Run: cub auth login", errImpactNotConnected)
	}

	result, err := buildImpactResult(cache, args[0])
	if err != nil {
		return err
	}

	format := strings.ToLower(strings.TrimSpace(impactFormat))
	if format == "" {
		format = "ascii"
	}
	if impactJSON {
		format = "json"
	}
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	case "md":
		fmt.Print(renderImpactMarkdown(result))
		return nil
	case "ascii":
		fmt.Print(renderImpactASCII(result))
		return nil
	default:
		return fmt.Errorf("invalid --format %q (valid: ascii, json, md)", impactFormat)
	}
}

func buildImpactResult(cache *cubUnitCache, rawUnit string) (impactResult, error) {
	if cache == nil {
		return impactResult{}, fmt.Errorf("impact cache unavailable")
	}

	unitSlug := normalizeImpactUnit(rawUnit)
	unit := cache.getUnitBySlug(unitSlug)
	if unit == nil {
		return impactResult{}, fmt.Errorf("unit %q not found in connected ConfigHub context", unitSlug)
	}

	uniqueDependents := uniqueSortedStrings(unit.DependedBy)
	dependents := make([]impactDependent, 0, len(uniqueDependents))
	environments := make(map[string]struct{})
	clusters := make(map[string]struct{})
	notes := make([]string, 0, 2)

	for _, dependentSlug := range uniqueDependents {
		dependentUnit := cache.getUnitBySlug(dependentSlug)
		if dependentUnit == nil {
			notes = append(notes, fmt.Sprintf("Dependent unit %q was referenced but not found in current unit cache.", dependentSlug))
			continue
		}

		envList := uniqueSortedStrings(append([]string{dependentUnit.Space}, dependentUnit.OtherSpaces...))
		for _, env := range envList {
			if env == "" {
				continue
			}
			environments[env] = struct{}{}
		}
		clusterList := uniqueSortedStrings([]string{dependentUnit.TargetSlug})
		for _, cluster := range clusterList {
			if cluster == "" {
				continue
			}
			clusters[cluster] = struct{}{}
		}

		dependents = append(dependents, impactDependent{
			Unit:         dependentSlug,
			Environments: envList,
			Clusters:     clusterList,
		})
	}

	risk := "LOW"
	if len(dependents) > 0 {
		risk = "MEDIUM"
		for env := range environments {
			if strings.Contains(strings.ToLower(env), "prod") {
				risk = "HIGH"
				break
			}
		}
	} else {
		notes = append(notes, "No dependent units found for this unit in the current ConfigHub link graph.")
	}

	return impactResult{
		Unit:       unitSlug,
		Connected:  true,
		Dependents: dependents,
		Summary: impactSummary{
			DirectDependents:     len(dependents),
			EnvironmentsAffected: len(environments),
			ClustersAffected:     len(clusters),
			Risk:                 risk,
		},
		Notes: notes,
	}, nil
}

func normalizeImpactUnit(raw string) string {
	unit := strings.TrimSpace(raw)
	unit = strings.TrimPrefix(unit, "unit/")
	return strings.TrimSpace(unit)
}

func renderImpactASCII(result impactResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Impact Preview: unit/%s\n\n", result.Unit))
	b.WriteString("Blast Radius:\n")
	b.WriteString(fmt.Sprintf("  Direct dependents: %d unit(s)\n", result.Summary.DirectDependents))
	for _, dependent := range result.Dependents {
		envs := "-"
		if len(dependent.Environments) > 0 {
			envs = strings.Join(dependent.Environments, ", ")
		}
		clusters := "-"
		if len(dependent.Clusters) > 0 {
			clusters = strings.Join(dependent.Clusters, ", ")
		}
		b.WriteString(fmt.Sprintf("    - %s (env: %s, clusters: %s)\n", dependent.Unit, envs, clusters))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Environments affected: %d\n", result.Summary.EnvironmentsAffected))
	b.WriteString(fmt.Sprintf("  Clusters affected: %d\n", result.Summary.ClustersAffected))
	b.WriteString(fmt.Sprintf("  Risk: %s\n", result.Summary.Risk))
	if len(result.Notes) > 0 {
		b.WriteString("\nNotes:\n")
		for _, note := range result.Notes {
			b.WriteString(fmt.Sprintf("  - %s\n", note))
		}
	}
	return b.String()
}

func renderImpactMarkdown(result impactResult) string {
	var b strings.Builder
	b.WriteString("## Impact Preview\n\n")
	b.WriteString(fmt.Sprintf("- Unit: `unit/%s`\n", result.Unit))
	b.WriteString(fmt.Sprintf("- Direct dependents: `%d`\n", result.Summary.DirectDependents))
	b.WriteString(fmt.Sprintf("- Environments affected: `%d`\n", result.Summary.EnvironmentsAffected))
	b.WriteString(fmt.Sprintf("- Clusters affected: `%d`\n", result.Summary.ClustersAffected))
	b.WriteString(fmt.Sprintf("- Risk: `%s`\n\n", result.Summary.Risk))

	b.WriteString("### Dependents\n\n")
	if len(result.Dependents) == 0 {
		b.WriteString("- None\n")
	} else {
		b.WriteString("| Unit | Environments | Clusters |\n")
		b.WriteString("|---|---|---|\n")
		for _, dependent := range result.Dependents {
			envs := "-"
			if len(dependent.Environments) > 0 {
				envs = strings.Join(dependent.Environments, ", ")
			}
			clusters := "-"
			if len(dependent.Clusters) > 0 {
				clusters = strings.Join(dependent.Clusters, ", ")
			}
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | `%s` |\n", dependent.Unit, envs, clusters))
		}
	}

	if len(result.Notes) > 0 {
		b.WriteString("\n### Notes\n\n")
		for _, note := range result.Notes {
			b.WriteString("- " + note + "\n")
		}
	}
	return b.String()
}

func uniqueSortedStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
