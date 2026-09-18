// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
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
	Mode        string `json:"mode"` // "offline", "online", "connected", "auth_expired"
	Email       string `json:"email,omitempty"`
	ClusterName string `json:"cluster_name"`
	Context     string `json:"context"`
	Space       string `json:"space,omitempty"`
	SpaceSource string `json:"space_source,omitempty"`
	// ConfigHubReads is the verdict of the gate the connected commands use, so
	// that status read as a pre-flight agrees with the command it precedes.
	ConfigHubReads       bool        `json:"confighub_reads"`
	ConfigHubReadsReason string      `json:"confighub_reads_reason,omitempty"`
	Worker               *WorkerInfo `json:"worker,omitempty"`
	AuthValid            *bool       `json:"auth_valid,omitempty"` // nil if offline/online, true/false if has context
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

	// The gate the connected commands use, asked through the same seam they
	// use so status cannot drift from them. status can otherwise print
	// "Connected" while every connected command refuses, for instance with
	// CUB_SCOUT_OFFLINE set or telemetry turned off.
	readsErr := configHubReads()
	if readsErr != nil {
		status.ConfigHubReadsReason = readsErr.Error()
	} else {
		status.ConfigHubReads = true
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
			// cub has no default space, so the only space status can report is
			// one CUB_SPACE names.
			space := resolveConfigHubSpace("")
			status.Space, status.SpaceSource = space.Slug, space.Source

			// If hub reported offline/online but cub CLI has a context,
			// upgrade to connected
			if status.Mode != "connected" {
				status.Mode = "connected"
			}

			// Whether cub has a session it accepts. The gate has usually
			// answered that already; see statusSessionValid.
			authValid := statusSessionValid(readsErr)
			status.AuthValid = &authValid

			if !authValid {
				status.Mode = "auth_expired"
			}

			// Which worker serves this cluster is a question about the whole
			// organization, so every space is searched unless CUB_SPACE names one.
			if authValid {
				if worker := getWorkerForCluster(statusWorkerSpace(space), status.ClusterName); worker != nil {
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

	// The gate's verdict, whenever it disagrees with the mode above. Both
	// directions are corrections: a mode that promises reads every connected
	// command refuses, and a mode that denies reads they would make.
	//
	// The second is reached when `cub` has a session but `cub context get`
	// failed, so the mode line describes only what hub.confighub.com answered.
	switch {
	case !s.ConfigHubReads && s.ConfigHubReadsReason != "":
		fmt.Printf("            \033[33m⚠\033[0m ConfigHub reads unavailable: %s\n", s.ConfigHubReadsReason)
	case s.ConfigHubReads && s.Mode != "connected":
		fmt.Println("            \033[32m✔\033[0m ConfigHub reads available: cub has a session")
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
		// A worker records no cluster identity, so the lookup matches a worker
		// named after the cluster; say exactly that rather than "none for this
		// cluster". A failed lookup reads the same, and is not an absence.
		if s.Space != "" {
			fmt.Printf("Worker:     (no worker named %s found in space %s, from %s)\n", s.ClusterName, s.Space, s.SpaceSource)
		} else {
			fmt.Printf("Worker:     (no worker named %s found in any space)\n", s.ClusterName)
		}
	}
}

// getStatusCubContext gets the current cub context and email
// Returns context, email, and error
func getStatusCubContext() (*statusCubContext, string, error) {
	out, err := cubStdout(context.Background(), "context", "get", "-o", "json")
	if err != nil {
		return nil, "", err
	}

	ctx, err := parseCubContextJSON(out)
	if err != nil {
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
		return ctx, email, nil
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

// statusWorkerSpace is where status looks for this cluster's worker: the space
// CUB_SPACE names, else every space.
func statusWorkerSpace(space configHubSpace) string {
	if space.IsSet() {
		return space.Slug
	}
	return allConfigHubSpaces
}

func getWorkerForCluster(space, clusterName string) *WorkerInfo {
	if space == "" {
		return nil
	}

	out, err := cubStdout(context.Background(), withConfigHubSpace([]string{"worker", "list", "-o", "json"}, space)...)
	if err != nil {
		return nil
	}

	workers, err := parseCubWorkerListJSON(out)
	if err != nil {
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

// statusSessionValid reports whether `cub` has a session it accepts.
//
// The gate has usually answered that already, and asking `cub auth status` a
// second time either repeats it or contradicts it: in plugin mode with no
// CUB_TOKEN, status printed "Connected (auth expired)", "Run: cub auth login"
// and "ConfigHub reads available" together, which cannot all be true.
//
// A gate refusal that is not about authentication — reads turned off, `cub` not
// installed — says nothing about the session, so that case still asks.
func statusSessionValid(readsErr error) bool {
	switch {
	case readsErr == nil:
		return true
	case errors.Is(readsErr, hub.ErrCubNotAuthenticated):
		return false
	default:
		return validateAuthToken()
	}
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

	// `cub auth status` is the check that notices an expired session.
	// `cub auth get-token` prints a stored token and exits 0 even after expiry,
	// which is how status came to report Connected on a dead session.
	return hub.CubSessionValid()
}
