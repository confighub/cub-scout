// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Command cub-agent provides CLI commands for the ConfigHub Agent.
// It observes Kubernetes clusters and detects resource ownership.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	// BuildTag is set during build
	BuildTag = "dev"
	// BuildDate is set during build
	BuildDate = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "cub-agent",
	Short: "ConfigHub Agent CLI",
	Long: `ConfigHub Agent - Kubernetes resource visibility and ownership detection

The cub-agent observes Kubernetes clusters and detects resource ownership.
It provides commands for:

  - Mapping resources and their ownership (Flux, Argo CD, Helm, ConfigHub, Native)
  - Scanning for CCVEs (configuration anti-patterns)
  - Tracing ownership chains
  - Importing resources into ConfigHub

Interacts with ConfigHub via the cub CLI (like kubectl, flux, argocd).

Environment Variables:
  CLUSTER_NAME            Name for this cluster (default: default)
  KUBECONFIG              Path to kubeconfig file (default: ~/.kube/config)
`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Add version command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("cub-agent version %s (built %s)\n", BuildTag, BuildDate)
		},
	})

	// Add completion command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion script for cub-agent.

Bash:
  $ source <(cub-agent completion bash)
  # Or add to ~/.bashrc:
  $ cub-agent completion bash >> ~/.bashrc

Zsh:
  $ source <(cub-agent completion zsh)
  # Or install to fpath:
  $ cub-agent completion zsh > "${fpath[1]}/_cub-agent"

Fish:
  $ cub-agent completion fish | source
  # Or install:
  $ cub-agent completion fish > ~/.config/fish/completions/cub-agent.fish

PowerShell:
  PS> cub-agent completion powershell | Out-String | Invoke-Expression
`,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.ExactArgs(1),
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	})
}

// buildConfig builds a Kubernetes client config
func buildConfig() (*rest.Config, error) {
	// Try in-cluster config first
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}

	// Fall back to kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = home + "/.kube/config"
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// getCurrentContext returns the current kubectl context name
func getCurrentContext() string {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return "unknown"
	}

	if rawConfig.CurrentContext == "" {
		return "default"
	}

	return rawConfig.CurrentContext
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
