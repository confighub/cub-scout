// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/confighub/cub-scout/pkg/hub"
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
	Mode        string      `json:"mode"` // "offline", "online", "connected", "auth_expired"
	Email       string      `json:"email,omitempty"`
	ClusterName string      `json:"cluster_name"`
	Context     string      `json:"context"`
	Space       string      `json:"space,omitempty"`
	Worker      *WorkerInfo `json:"worker,omitempty"`
	AuthValid   *bool       `json:"auth_valid,omitempty"` // nil if offline/online, true/false if has context
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

	// Use pkg/hub for base connectivity/auth state
	hubMode := hub.CurrentMode()
	switch hubMode {
	case hub.Offline:
		status.Mode = "offline"
	case hub.Online:
		status.Mode = "online"
	case hub.Connected:
		status.Mode = "connected"
	}

	// Check local auth for email (fast, file-only)
	auth, _ := hub.LoadAuth()
	if auth != nil && auth.Email != "" {
		status.Email = auth.Email
	}

	// Try cub CLI for richer status (does not depend on local auth.json)
	if cubInstalled() {
		cubCtx, email, err := getStatusCubContext()
		if err == nil && cubCtx != nil {
			// cub CLI context overrides local auth for email
			if email != "" {
				status.Email = email
			}
			status.Space = cubCtx.Settings.DefaultSpace

			// If hub reported offline/online but cub CLI has a context,
			// upgrade to connected
			if status.Mode != "connected" {
				status.Mode = "connected"
			}

			// Validate token via cub CLI
			authValid := validateAuthToken()
			status.AuthValid = &authValid

			if !authValid {
				status.Mode = "auth_expired"
			}

			// Try to get worker status (only if auth is valid)
			if authValid && cubCtx.Settings.DefaultSpace != "" {
				worker := getWorkerForCluster(cubCtx.Settings.DefaultSpace, status.ClusterName)
				if worker != nil {
					status.Worker = worker
				}
			}
		}
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
	case "auth_expired":
		// #108: Show clear warning when auth token is expired
		fmt.Printf("ConfigHub:  \033[33m●\033[0m Connected (auth expired)")
		if s.Email != "" {
			fmt.Printf(" (%s)", s.Email)
		}
		fmt.Println()
		fmt.Println("            \033[33m⚠\033[0m Token expired or invalid")
		fmt.Println("            Run: cub auth login")
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

// cubInstalled checks if the cub CLI is available
func cubInstalled() bool {
	_, err := exec.LookPath("cub")
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

// validateAuthToken checks if the current ConfigHub auth token is valid
// Returns true if token is valid, false if expired or invalid
func validateAuthToken() bool {
	// When running as a `cub` plugin (CUB_PLUGIN=1), the host has already
	// passed a valid token through CUB_TOKEN. Honor it directly instead of
	// re-executing `cub` — that would recurse through the plugin host and
	// slow down every doctor/explain/trace invocation.
	if hub.IsPluginMode() {
		return hub.PluginToken() != ""
	}

	// Use cub auth get-token which will fail if token is expired
	// This is a lightweight check that doesn't make a network request
	// if the token is expired (it checks expiry locally)
	out, err := exec.Command("cub", "auth", "get-token").Output()
	if err != nil {
		return false
	}

	// If we got a non-empty token, auth is valid
	token := strings.TrimSpace(string(out))
	return token != ""
}
