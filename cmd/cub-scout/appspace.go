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
	Short: "Manage Apps",
	Long:  `Create, list, and manage Apps in ConfigHub.`,
}

var appSpaceCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create an App",
	Long: `Create a new App in ConfigHub.

An App is a team workspace containing all environments (dev, staging, prod)
for applications managed by a single deployer (Flux or Argo CD).

Examples:
  # Create an App
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
	Short: "List Apps",
	Long:  `List all Apps in the current organization.`,
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
		return fmt.Errorf("create app: %w", err)
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

// AppSpaceResult represents the result of creating an App
type AppSpaceResult struct {
	Name    string `json:"name"`
	Created bool   `json:"created"`
	Error   string `json:"error,omitempty"`
}

// CreateAppSpaceWithResult creates an App and returns structured result
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
		return result, fmt.Errorf("create app: %w", err)
	}

	result.Created = true
	return result, nil
}
