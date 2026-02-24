// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Command cub-scout provides CLI commands for exploring and mapping GitOps in Kubernetes clusters.
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
	Use:           "cub-scout",
	Short:         "Explore and map GitOps in your clusters",
	SilenceErrors: true, // We print errors in main() — don't let cobra duplicate them
	SilenceUsage:  true, // Don't print usage on runtime errors
	Long: `cub-scout - explore and map GitOps in your clusters

Offline-first, deterministic, read-only cluster explorer for Kubernetes and GitOps.

POPULAR COMMANDS
  map              Interactive TUI dashboard — explore cluster visually
  map list         List resources with ownership (scriptable, supports --json)
  tree             Hierarchical views: runtime, ownership, git, composition
  trace            Trace any resource to its Git source
  scan             Find misconfigurations (46 patterns)
  gitops status    GitOps pipeline health check

CHOOSE YOUR INTERFACE
  TUI:   cub-scout map                       Interactive exploration
  CLI:   cub-scout map list -q "owner=Flux"  Scripting and pipelines
  JSON:  cub-scout map list --json | jq      Automation and tooling

STANDALONE VS CONNECTED
  Standalone (default): Works offline, reads from kubectl context
  Connected:            Optional ConfigHub integration for fleet features
                        Run 'cub auth login' to enable

TIPS
  • Use 'tree ownership' to see all resources grouped by GitOps tool
  • Add --json to most commands for machine-readable output
  • Press '?' in TUI for keyboard shortcuts

Environment Variables:
  CLUSTER_NAME            Name for this cluster (default: default)
  KUBECONFIG              Path to kubeconfig file (default: ~/.kube/config)
  CUB_SCOUT_OFFLINE       Set to 'true' to force offline mode
`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("cub-scout - explore and map GitOps in your clusters")
		fmt.Println()
		fmt.Println("Quick start:")
		fmt.Println("  cub-scout map              Interactive TUI (press ? for help)")
		fmt.Println("  cub-scout tree ownership   See resources by GitOps owner")
		fmt.Println("  cub-scout trace deploy/x   Trace a resource to Git")
		fmt.Println("  cub-scout map list --json  JSON output for automation")
		fmt.Println()
		fmt.Println("Run 'cub-scout --help' for all commands")
	},
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
			fmt.Printf("cub-scout version %s (built %s)\n", BuildTag, BuildDate)
		},
	})

	// Add completion command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion script for cub-scout.

Bash:
  $ source <(cub-scout completion bash)
  # Or add to ~/.bashrc:
  $ cub-scout completion bash >> ~/.bashrc

Zsh:
  $ source <(cub-scout completion zsh)
  # Or install to fpath:
  $ cub-scout completion zsh > "${fpath[1]}/_cub-scout"

Fish:
  $ cub-scout completion fish | source
  # Or install:
  $ cub-scout completion fish > ~/.config/fish/completions/cub-scout.fish

PowerShell:
  PS> cub-scout completion powershell | Out-String | Invoke-Expression
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
