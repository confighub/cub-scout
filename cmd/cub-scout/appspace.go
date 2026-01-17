// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var appSpaceCmd = &cobra.Command{
	Use:   "app-space",
	Short: "Manage App Spaces",
	Long:  `Create, list, and manage App Spaces in ConfigHub.`,
}

var appSpaceCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create an App Space",
	Long: `Create a new App Space in ConfigHub.

An App Space is a team workspace containing all environments (dev, staging, prod)
for applications managed by a single deployer (Flux or Argo CD).

Examples:
  # Create an App Space
  cub-scout app-space create payments-team

  # Create and set as current context
  cub-scout app-space create payments-team --set-context

  # Create with labels
  cub-scout app-space create payments-team --label team=payments --label owner=platform
`,
	Args: cobra.ExactArgs(1),
	RunE: runAppSpaceCreate,
}

var appSpaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List App Spaces",
	Long:  `List all App Spaces in the current organization.`,
	RunE:  runAppSpaceList,
}

var (
	appSpaceSetContext bool
	appSpaceLabels     []string
	appSpaceJSON       bool
)

func init() {
	appSpaceCreateCmd.Flags().BoolVar(&appSpaceSetContext, "set-context", false, "Set as current context after creation")
	appSpaceCreateCmd.Flags().StringArrayVar(&appSpaceLabels, "label", nil, "Labels in key=value format (can be repeated)")

	appSpaceListCmd.Flags().BoolVar(&appSpaceJSON, "json", false, "Output as JSON")

	appSpaceCmd.AddCommand(appSpaceCreateCmd)
	appSpaceCmd.AddCommand(appSpaceListCmd)
	rootCmd.AddCommand(appSpaceCmd)
}

func runAppSpaceCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Build cub command
	cubArgs := []string{"space", "create", name}

	if appSpaceSetContext {
		cubArgs = append(cubArgs, "--set-context")
	}

	for _, label := range appSpaceLabels {
		cubArgs = append(cubArgs, "--label", label)
	}

	cubCmd := exec.Command("cub", cubArgs...)
	cubCmd.Stdout = os.Stdout
	cubCmd.Stderr = os.Stderr

	if err := cubCmd.Run(); err != nil {
		return fmt.Errorf("create app space: %w", err)
	}

	return nil
}

func runAppSpaceList(cmd *cobra.Command, args []string) error {
	cubArgs := []string{"space", "list"}

	if appSpaceJSON {
		cubArgs = append(cubArgs, "--json")
	}

	cubCmd := exec.Command("cub", cubArgs...)
	cubCmd.Stdout = os.Stdout
	cubCmd.Stderr = os.Stderr

	return cubCmd.Run()
}

// AppSpaceResult represents the result of creating an App Space
type AppSpaceResult struct {
	Name    string `json:"name"`
	Created bool   `json:"created"`
	Error   string `json:"error,omitempty"`
}

// CreateAppSpaceWithResult creates an App Space and returns structured result
func CreateAppSpaceWithResult(name string, setContext bool, labels []string) (*AppSpaceResult, error) {
	result := &AppSpaceResult{Name: name}

	cubArgs := []string{"space", "create", name, "--json"}

	if setContext {
		cubArgs = append(cubArgs, "--set-context")
	}

	for _, label := range labels {
		cubArgs = append(cubArgs, "--label", label)
	}

	cubCmd := exec.Command("cub", cubArgs...)
	output, err := cubCmd.CombinedOutput()

	if err != nil {
		// Check if space already exists
		if strings.Contains(string(output), "already exists") {
			result.Created = false
			return result, nil
		}
		result.Error = strings.TrimSpace(string(output))
		return result, fmt.Errorf("create app space: %w", err)
	}

	result.Created = true
	return result, nil
}

