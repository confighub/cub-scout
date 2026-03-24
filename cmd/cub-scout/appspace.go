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

var appCmd = &cobra.Command{
	Use:   "app",
	Short: "Manage Apps",
	Long:  `Create, list, and manage Apps in ConfigHub.`,
}

var appCreateCmd = &cobra.Command{
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
	RunE: runAppCreate,
}

var appListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Apps",
	Long:  `List all Apps in the current organization.`,
	RunE:  runAppList,
}

var (
	appSetContext bool
	appLabels     []string
	appJSON       bool
)

func init() {
	appCreateCmd.Flags().BoolVar(&appSetContext, "set-context", false, "Set as current context after creation")
	appCreateCmd.Flags().StringArrayVar(&appLabels, "label", nil, "Labels in key=value format (can be repeated)")

	appListCmd.Flags().BoolVar(&appJSON, "json", false, "Output as JSON")

	appCmd.AddCommand(appCreateCmd)
	appCmd.AddCommand(appListCmd)
	rootCmd.AddCommand(appCmd)
}

func runAppCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Build cub command
	cubArgs := []string{"space", "create", name}

	if appSetContext {
		cubArgs = append(cubArgs, "--set-context")
	}

	for _, label := range appLabels {
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

func runAppList(cmd *cobra.Command, args []string) error {
	cubArgs := []string{"space", "list"}

	if appJSON {
		cubArgs = append(cubArgs, "--json")
	}

	cubCmd := exec.Command("cub", cubArgs...)
	cubCmd.Stdout = os.Stdout
	cubCmd.Stderr = os.Stderr

	return cubCmd.Run()
}

// AppResult represents the result of creating an App
type AppResult struct {
	Name    string `json:"name"`
	Created bool   `json:"created"`
	Error   string `json:"error,omitempty"`
}

// CreateAppWithResult creates an App and returns structured result
func CreateAppWithResult(name string, setContext bool, labels []string) (*AppResult, error) {
	result := &AppResult{Name: name}

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
