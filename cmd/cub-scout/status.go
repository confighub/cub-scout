// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show connection status and cluster info",
	Long: `Show cub-scout connection status, cluster info, and worker status.

Displays:
  - ConfigHub connection status (Offline/Online/Connected)
  - Current cluster name (from CLUSTER_NAME env or default)
  - Current kubectl context
  - Worker status (if connected to ConfigHub)

Examples:
  cub-scout status
  cub-scout status --json
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus(cmd)
	},
}

func init() {
	statusCmd.Flags().Bool("json", false, "Output as JSON")
}

// StatusInfo holds status information for display
type StatusInfo struct {
	Mode        string      `json:"mode"` // "offline", "online", "connected"
	Email       string      `json:"email,omitempty"`
	ClusterName string      `json:"cluster_name"`
	Context     string      `json:"context"`
	Space       string      `json:"space,omitempty"`
	Worker      *WorkerInfo `json:"worker,omitempty"`
}

// WorkerInfo holds worker status
type WorkerInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "connected", "disconnected", "unknown"
	Cluster string `json:"cluster,omitempty"`
}

// statusCubContext represents the output of cub context get --json for status command
type statusCubContext struct {
	Name       string `json:"name"`
	Coordinate struct {
		ServerURL      string `json:"serverURL"`
		OrganizationID string `json:"organizationID"`
	} `json:"coordinate"`
	Settings struct {
		DefaultSpace string `json:"defaultSpace"`
	} `json:"settings"`
}

func runStatus(cmd *cobra.Command) error {
	jsonOutput, _ := cmd.Flags().GetBool("json")

	status := StatusInfo{
		Mode:        "offline",
		ClusterName: getClusterName(),
		Context:     getCurrentContext(),
	}

	// Check ConfigHub connection by running cub context get
	cubCtx, email, err := getStatusCubContext()
	if err == nil && cubCtx != nil {
		status.Mode = "connected"
		status.Email = email
		status.Space = cubCtx.Settings.DefaultSpace

		// Try to get worker status for the current cluster
		worker := getWorkerForCluster(cubCtx.Settings.DefaultSpace, status.ClusterName)
		if worker != nil {
			status.Worker = worker
		}
	} else if isOnline() {
		status.Mode = "online"
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	// Human-readable output
	printStatus(status)
	return nil
}

func printStatus(s StatusInfo) {
	// Mode indicator
	switch s.Mode {
	case "connected":
		fmt.Printf("ConfigHub:  \033[32m●\033[0m Connected")
		if s.Email != "" {
			fmt.Printf(" (%s)", s.Email)
		}
		fmt.Println()
	case "online":
		fmt.Println("ConfigHub:  \033[33m○\033[0m Online (not authenticated)")
		fmt.Println("            Run: cub auth login")
	case "offline":
		fmt.Println("ConfigHub:  \033[31m○\033[0m Offline")
	}

	// Cluster info
	fmt.Printf("Cluster:    %s\n", s.ClusterName)
	fmt.Printf("Context:    %s\n", s.Context)

	// Worker info
	if s.Worker != nil {
		switch s.Worker.Status {
		case "connected":
			fmt.Printf("Worker:     \033[32m●\033[0m %s (connected)\n", s.Worker.Name)
		case "disconnected":
			fmt.Printf("Worker:     \033[31m○\033[0m %s (disconnected)\n", s.Worker.Name)
			fmt.Println("            Run: cub worker run " + s.Worker.Name)
		default:
			fmt.Printf("Worker:     \033[33m○\033[0m %s (%s)\n", s.Worker.Name, s.Worker.Status)
		}
	} else if s.Mode == "connected" {
		fmt.Println("Worker:     (none for this cluster)")
	}
}

// getStatusCubContext gets the current cub context and email
// Returns context, email, and error
func getStatusCubContext() (*statusCubContext, string, error) {
	out, err := exec.Command("cub", "context", "get", "--json").Output()
	if err != nil {
		return nil, "", err
	}

	var ctx statusCubContext
	if err := json.Unmarshal(out, &ctx); err != nil {
		return nil, "", err
	}

	// Try to get email from cub auth status or similar
	// For now, use the context name if it looks like an email
	email := ""
	if strings.Contains(ctx.Name, "@") {
		email = ctx.Name
	}

	// If context has a name, we're connected
	if ctx.Name != "" {
		return &ctx, email, nil
	}

	return nil, "", fmt.Errorf("no context found")
}

func getClusterName() string {
	name := os.Getenv("CLUSTER_NAME")
	if name == "" {
		return "default"
	}
	return name
}

// isOnline checks basic internet connectivity
func isOnline() bool {
	// Simple check - if we can run cub without errors, we're probably online
	_, err := exec.Command("cub", "--version").Output()
	return err == nil
}

// WorkerListItem represents a worker from cub worker list
type WorkerListItem struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	Condition string `json:"condition"`
}

func getWorkerForCluster(space, clusterName string) *WorkerInfo {
	if space == "" {
		return nil
	}

	out, err := exec.Command("cub", "worker", "list", "--space", space, "--json").Output()
	if err != nil {
		return nil
	}

	var workers []WorkerListItem
	if err := json.Unmarshal(out, &workers); err != nil {
		return nil
	}

	// Find worker for this cluster
	for _, w := range workers {
		if w.Cluster == clusterName || w.Name == clusterName {
			status := "unknown"
			switch strings.ToLower(w.Condition) {
			case "ready", "connected":
				status = "connected"
			case "disconnected", "notready":
				status = "disconnected"
			}
			return &WorkerInfo{
				Name:    w.Name,
				Status:  status,
				Cluster: w.Cluster,
			}
		}
	}

	return nil
}
