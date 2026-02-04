// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/confighub/cub-scout/pkg/agent"
)

var (
	catalogFormat  string
	catalogID      string
	catalogLabels  []string
	catalogScope   string
	catalogSeq     int
	catalogOrder   string
)

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manage bundle catalogs",
	Long: `Manage catalogs of debug bundles for multi-bundle operations.

A catalog is a file-backed manifest that indexes multiple bundles.
Catalogs enable:
- Multi-bundle comparisons (diffs)
- Timeline construction
- Explicit ordering and grouping

Catalogs are:
- Explicit: ordering is declared, not inferred from filesystem
- Deterministic: same catalog + bundles = same results
- Portable: directory-based, no database

Layout:
  catalog/
    catalog.json      # Manifest with bundle entries
    bundles/          # Copied bundle directories
      bundle-1/
      bundle-2/

Commands:
  init      Create a new empty catalog
  add       Add a bundle to a catalog
  list      List bundles in a catalog
  validate  Validate catalog integrity

Examples:
  # Create a new catalog
  cub-scout catalog init ./my-catalog

  # Add a bundle with custom ID
  cub-scout catalog add ./my-catalog ./debug-bundle --id before-fix

  # Add with labels and scope
  cub-scout catalog add ./my-catalog ./debug-bundle \
    --id after-fix \
    --label ticket=INC-123 \
    --scope prod/api/Deployment/api

  # List bundles
  cub-scout catalog list ./my-catalog

  # Validate integrity
  cub-scout catalog validate ./my-catalog
`,
}

var catalogInitCmd = &cobra.Command{
	Use:   "init <path>",
	Short: "Create a new empty catalog",
	Long: `Create a new empty catalog at the specified path.

This creates:
- catalog.json with empty bundle list
- bundles/ directory for bundle storage

Examples:
  cub-scout catalog init ./incident-123-catalog
  cub-scout catalog init /tmp/debug-catalog
`,
	Args: cobra.ExactArgs(1),
	RunE: runCatalogInit,
}

var catalogAddCmd = &cobra.Command{
	Use:   "add <catalog> <bundle>",
	Short: "Add a bundle to a catalog",
	Long: `Add a debug bundle to an existing catalog.

The bundle is copied into the catalog's bundles/ directory.
Metadata is extracted from the bundle and stored in catalog.json.

Required:
  catalog  Path to the catalog directory
  bundle   Path to the bundle to add

Options:
  --id       Unique identifier (auto-generated if not provided)
  --label    Key=value labels (can be repeated)
  --scope    Scope string: cluster/namespace/target
  --sequence Explicit ordering integer

Examples:
  # Basic add (auto-generates ID)
  cub-scout catalog add ./my-catalog ./debug-bundle

  # With custom ID
  cub-scout catalog add ./my-catalog ./debug-bundle --id before-fix

  # With labels
  cub-scout catalog add ./my-catalog ./debug-bundle \
    --id incident-start \
    --label ticket=INC-123 \
    --label source=ci

  # With scope for grouping
  cub-scout catalog add ./my-catalog ./debug-bundle \
    --scope prod/api/Deployment/api

  # With explicit sequence for ordering
  cub-scout catalog add ./my-catalog ./debug-bundle --sequence 1
`,
	Args: cobra.ExactArgs(2),
	RunE: runCatalogAdd,
}

var catalogListCmd = &cobra.Command{
	Use:   "list <catalog>",
	Short: "List bundles in a catalog",
	Long: `List all bundles in a catalog with their metadata.

Output includes:
- Bundle ID
- Creation time
- Path
- Scope (if set)
- Labels (if set)

Ordering:
  --order manifest    Order as listed in catalog.json (default)
  --order created_at  Order by creation time
  --order sequence    Order by sequence field

Examples:
  # List as ASCII table
  cub-scout catalog list ./my-catalog

  # List as JSON
  cub-scout catalog list ./my-catalog --format json

  # List ordered by creation time
  cub-scout catalog list ./my-catalog --order created_at
`,
	Args: cobra.ExactArgs(1),
	RunE: runCatalogList,
}

var catalogValidateCmd = &cobra.Command{
	Use:   "validate <catalog>",
	Short: "Validate catalog integrity",
	Long: `Validate a catalog's integrity and bundle references.

Checks:
- Schema version is supported
- All bundle IDs are unique
- All bundle paths exist
- Bundle metadata matches catalog entries
- Required fields are present

Exit codes:
  0  Valid catalog
  1  Validation errors found

Examples:
  cub-scout catalog validate ./my-catalog
`,
	Args: cobra.ExactArgs(1),
	RunE: runCatalogValidate,
}

func init() {
	rootCmd.AddCommand(catalogCmd)
	catalogCmd.AddCommand(catalogInitCmd)
	catalogCmd.AddCommand(catalogAddCmd)
	catalogCmd.AddCommand(catalogListCmd)
	catalogCmd.AddCommand(catalogValidateCmd)

	// Add flags
	catalogAddCmd.Flags().StringVar(&catalogID, "id", "", "Bundle identifier (auto-generated if not provided)")
	catalogAddCmd.Flags().StringArrayVar(&catalogLabels, "label", nil, "Labels in key=value format (can be repeated)")
	catalogAddCmd.Flags().StringVar(&catalogScope, "scope", "", "Scope: cluster/namespace/target")
	catalogAddCmd.Flags().IntVar(&catalogSeq, "sequence", 0, "Explicit ordering sequence")

	catalogListCmd.Flags().StringVar(&catalogFormat, "format", "ascii", "Output format: ascii, json")
	catalogListCmd.Flags().StringVar(&catalogOrder, "order", "manifest", "Order: manifest, created_at, sequence")
}

func runCatalogInit(cmd *cobra.Command, args []string) error {
	catalogDir := args[0]

	writer := agent.NewCatalogWriter()
	if err := writer.Init(catalogDir); err != nil {
		return fmt.Errorf("failed to initialize catalog: %w", err)
	}

	fmt.Printf("Catalog initialized at %s\n", catalogDir)
	return nil
}

func runCatalogAdd(cmd *cobra.Command, args []string) error {
	catalogDir := args[0]
	bundlePath := args[1]

	// Build entry
	entry := agent.CatalogEntry{
		ID: catalogID,
	}

	// Parse labels
	if len(catalogLabels) > 0 {
		entry.Labels = make(map[string]string)
		for _, label := range catalogLabels {
			parts := strings.SplitN(label, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid label format %q (expected key=value)", label)
			}
			entry.Labels[parts[0]] = parts[1]
		}
	}

	// Parse scope
	if catalogScope != "" {
		parts := strings.Split(catalogScope, "/")
		scope := &agent.CatalogScope{}
		if len(parts) >= 1 {
			scope.Cluster = parts[0]
		}
		if len(parts) >= 2 {
			scope.Namespace = parts[1]
		}
		if len(parts) >= 3 {
			scope.Target = strings.Join(parts[2:], "/")
		}
		entry.Scope = scope
	}

	// Set sequence if provided
	if catalogSeq != 0 {
		entry.Sequence = &catalogSeq
	}

	// Add to catalog
	writer := agent.NewCatalogWriter()
	if err := writer.Add(catalogDir, bundlePath, entry); err != nil {
		return fmt.Errorf("failed to add bundle: %w", err)
	}

	// Report the actual ID used
	if catalogID == "" {
		// Re-read to get the generated ID
		reader := agent.NewCatalogReader()
		catalog, _ := reader.Read(catalogDir)
		if len(catalog.Bundles) > 0 {
			lastEntry := catalog.Bundles[len(catalog.Bundles)-1]
			fmt.Printf("Bundle added with ID: %s\n", lastEntry.ID)
		}
	} else {
		fmt.Printf("Bundle added with ID: %s\n", catalogID)
	}

	return nil
}

func runCatalogList(cmd *cobra.Command, args []string) error {
	catalogDir := args[0]

	reader := agent.NewCatalogReader()
	catalog, err := reader.Read(catalogDir)
	if err != nil {
		return fmt.Errorf("failed to read catalog: %w", err)
	}

	// Parse order mode
	var orderMode agent.OrderMode
	switch catalogOrder {
	case "manifest":
		orderMode = agent.OrderManifest
	case "created_at":
		orderMode = agent.OrderCreatedAt
	case "sequence":
		orderMode = agent.OrderSequence
	default:
		return fmt.Errorf("invalid order mode: %s (valid: manifest, created_at, sequence)", catalogOrder)
	}

	// Order bundles
	ordered := agent.OrderBundles(catalog.Bundles, orderMode)

	switch catalogFormat {
	case "json":
		return renderCatalogListJSON(catalog, ordered)
	case "ascii":
		return renderCatalogListASCII(catalog, ordered, catalogDir)
	default:
		return fmt.Errorf("invalid format: %s (valid: ascii, json)", catalogFormat)
	}
}

// CatalogListOutput is the JSON output for catalog list.
type CatalogListOutput struct {
	SchemaVersion string                `json:"schema_version"`
	Path          string                `json:"path,omitempty"`
	BundleCount   int                   `json:"bundle_count"`
	Bundles       []agent.CatalogEntry  `json:"bundles"`
}

func renderCatalogListJSON(catalog *agent.Catalog, ordered []agent.CatalogEntry) error {
	output := CatalogListOutput{
		SchemaVersion: catalog.SchemaVersion,
		BundleCount:   len(ordered),
		Bundles:       ordered,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func renderCatalogListASCII(catalog *agent.Catalog, ordered []agent.CatalogEntry, catalogDir string) error {
	var b strings.Builder

	// Header
	b.WriteString("Bundle Catalog\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n\n")

	// Summary
	summary := agent.SummarizeCatalog(catalog)
	b.WriteString(fmt.Sprintf("Path:     %s\n", catalogDir))
	b.WriteString(fmt.Sprintf("Version:  %s\n", summary.SchemaVersion))
	b.WriteString(fmt.Sprintf("Bundles:  %d\n", summary.BundleCount))
	if summary.OldestBundle != nil && summary.NewestBundle != nil {
		b.WriteString(fmt.Sprintf("Range:    %s to %s\n",
			summary.OldestBundle.Format("2006-01-02"),
			summary.NewestBundle.Format("2006-01-02")))
	}
	b.WriteString("\n")

	// Bundle list
	if len(ordered) == 0 {
		b.WriteString("No bundles in catalog.\n")
	} else {
		b.WriteString("Bundles:\n")
		for i, entry := range ordered {
			b.WriteString(fmt.Sprintf("\n  [%d] %s\n", i+1, entry.ID))
			b.WriteString(fmt.Sprintf("      Created: %s\n", entry.CreatedAt.Format("2006-01-02 15:04:05 UTC")))
			b.WriteString(fmt.Sprintf("      Path:    %s\n", entry.Path))

			if entry.Scope != nil {
				scopeStr := formatScope(entry.Scope)
				if scopeStr != "" {
					b.WriteString(fmt.Sprintf("      Scope:   %s\n", scopeStr))
				}
			}

			if len(entry.Labels) > 0 {
				labelStrs := []string{}
				for k, v := range entry.Labels {
					labelStrs = append(labelStrs, fmt.Sprintf("%s=%s", k, v))
				}
				b.WriteString(fmt.Sprintf("      Labels:  %s\n", strings.Join(labelStrs, ", ")))
			}

			if entry.Sequence != nil {
				b.WriteString(fmt.Sprintf("      Seq:     %d\n", *entry.Sequence))
			}
		}
	}

	fmt.Print(b.String())
	return nil
}

func formatScope(scope *agent.CatalogScope) string {
	parts := []string{}
	if scope.Cluster != "" {
		parts = append(parts, scope.Cluster)
	}
	if scope.Namespace != "" {
		parts = append(parts, scope.Namespace)
	}
	if scope.Target != "" {
		parts = append(parts, scope.Target)
	}
	return strings.Join(parts, "/")
}

func runCatalogValidate(cmd *cobra.Command, args []string) error {
	catalogDir := args[0]

	reader := agent.NewCatalogReader()
	catalog, err := reader.Read(catalogDir)
	if err != nil {
		return fmt.Errorf("failed to read catalog: %w", err)
	}

	errors := reader.Validate(catalogDir, catalog)

	if len(errors) == 0 {
		fmt.Printf("Catalog valid: %d bundle(s)\n", len(catalog.Bundles))
		return nil
	}

	// Print validation errors
	fmt.Fprintf(os.Stderr, "Validation failed with %d error(s):\n", len(errors))
	for _, err := range errors {
		fmt.Fprintf(os.Stderr, "  - %s: %s\n", err.Field, err.Message)
	}

	os.Exit(1)
	return nil
}
