// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/monadic/confighub-agent/pkg/queries"
)

var queriesJSON bool

var mapQueriesCmd = &cobra.Command{
	Use:   "queries",
	Short: "List and manage saved queries",
	Long: `List and manage saved queries for filtering resources.

Saved queries are named, reusable query expressions. They come in two types:
- Built-in: Shipped with the agent to help you get started
- User: Your custom queries saved in ~/.confighub/queries.yaml

Use saved queries with the -q flag:
  cub-agent map list -q unmanaged
  cub-agent map list -q "unmanaged AND namespace=prod*"

Examples:
  # List all saved queries
  cub-agent map queries

  # Run a saved query by name
  cub-agent map list -q unmanaged

  # Save a new query
  cub-agent map queries save my-apps "labels[team]=payments"

  # Delete a user query
  cub-agent map queries delete my-apps

  # Show ConfigHub connection status
  cub-agent map queries connect
`,
	RunE: runQueriesList,
}

var mapQueriesSaveCmd = &cobra.Command{
	Use:   "save NAME QUERY [DESCRIPTION]",
	Short: "Save a new user query",
	Long: `Save a new user query to ~/.confighub/queries.yaml.

Examples:
  cub-agent map queries save my-team "labels[team]=payments"
  cub-agent map queries save my-team "labels[team]=payments" "Payment team resources"
`,
	Args: cobra.RangeArgs(2, 3),
	RunE: runQueriesSave,
}

var mapQueriesDeleteCmd = &cobra.Command{
	Use:   "delete NAME",
	Short: "Delete a user query",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueriesDelete,
}

var mapQueriesConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Check ConfigHub connection status and next steps",
	Long: `Check your ConfigHub connection status and see what to do next.

ConfigHub gives you:
  • Saved queries shared with your team
  • Alerts when query results change
  • History and trends over time
  • Fleet-wide queries across all clusters

This command shows your current setup status and guides you to the next step.
`,
	RunE: runQueriesConnect,
}

func init() {
	mapCmd.AddCommand(mapQueriesCmd)
	mapQueriesCmd.AddCommand(mapQueriesSaveCmd)
	mapQueriesCmd.AddCommand(mapQueriesDeleteCmd)
	mapQueriesCmd.AddCommand(mapQueriesConnectCmd)

	mapQueriesCmd.Flags().BoolVar(&queriesJSON, "json", false, "Output in JSON format")
}

func runQueriesList(cmd *cobra.Command, args []string) error {
	store, err := queries.NewQueryStore()
	if err != nil {
		return fmt.Errorf("load queries: %w", err)
	}

	allQueries := store.List()

	if queriesJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(allQueries)
	}

	// Print built-in queries
	builtins := store.ListBuiltin()
	if len(builtins) > 0 {
		fmt.Println("BUILT-IN QUERIES")
		fmt.Println("────────────────")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, q := range builtins {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", q.Name, q.Description, q.Query)
		}
		w.Flush()
		fmt.Println()
	}

	// Print user queries
	userQueries := store.ListUser()
	if len(userQueries) > 0 {
		fmt.Println("YOUR QUERIES")
		fmt.Println("────────────")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, q := range userQueries {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", q.Name, q.Description, q.Query)
		}
		w.Flush()
		fmt.Println()
	}

	// Usage hint
	fmt.Println("USAGE")
	fmt.Println("─────")
	fmt.Println("  cub-agent map list -q <name>          Run a saved query")
	fmt.Println("  cub-agent map list -q \"<name> AND namespace=prod*\"")
	fmt.Println("  cub-agent map queries save <name> <query>")
	fmt.Println()

	// Light hook to ConfigHub
	fmt.Println("────────────────────────────────────────────────────────────────")
	fmt.Println("🔗 Want team-shared queries, alerts, and history?")
	fmt.Println("   See: cub-agent map queries connect")
	fmt.Println("────────────────────────────────────────────────────────────────")

	return nil
}

func runQueriesSave(cmd *cobra.Command, args []string) error {
	name := args[0]
	queryExpr := args[1]
	description := ""
	if len(args) > 2 {
		description = args[2]
	}

	// Check if trying to overwrite a builtin
	store, _ := queries.NewQueryStore()
	if existing, found := store.Get(name); found && existing.Category == "builtin" {
		return fmt.Errorf("cannot overwrite built-in query %q. Choose a different name", name)
	}

	query := queries.SavedQuery{
		Name:        name,
		Description: description,
		Query:       queryExpr,
	}

	if err := queries.SaveUserQuery(query); err != nil {
		return fmt.Errorf("save query: %w", err)
	}

	fmt.Printf("✓ Saved query %q\n", name)
	fmt.Printf("  Query: %s\n", queryExpr)
	fmt.Printf("  File:  %s\n", queries.UserQueriesFile())
	fmt.Println()
	fmt.Printf("Run it: cub-agent map list -q %s\n", name)

	return nil
}

func runQueriesDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Check if trying to delete a builtin
	store, _ := queries.NewQueryStore()
	if existing, found := store.Get(name); found && existing.Category == "builtin" {
		return fmt.Errorf("cannot delete built-in query %q", name)
	}

	if err := queries.DeleteUserQuery(name); err != nil {
		return err
	}

	fmt.Printf("✓ Deleted query %q\n", name)
	return nil
}

func runQueriesConnect(cmd *cobra.Command, args []string) error {
	fmt.Println("🔗 CONNECT TO CONFIGHUB")
	fmt.Println("═══════════════════════")
	fmt.Println()
	fmt.Println("ConfigHub gives you:")
	fmt.Println("  • Saved queries shared with your team")
	fmt.Println("  • Alerts when query results change")
	fmt.Println("  • History and trends over time")
	fmt.Println("  • Fleet-wide queries across all clusters")
	fmt.Println()
	fmt.Println("GET STARTED")
	fmt.Println("───────────")
	fmt.Println("  1. Sign up or log in:  https://confighub.com")
	fmt.Println("  2. Import workloads:   cub-agent import --namespace <ns>")
	fmt.Println()
	fmt.Println("  Full guide: docs/IMPORTING-WORKLOADS.md")
	fmt.Println()

	return nil
}
