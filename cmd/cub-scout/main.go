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

	"github.com/confighub/cub-scout/pkg/hub"
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
  quickstart       Guided first-run tour across doctor/explain/ownership/scan
  doctor           One-command cluster health summary
  explain          Plain-English ownership and lineage for one resource
  map              Interactive TUI dashboard — explore cluster visually
  map list         List resources with ownership (scriptable, supports --json)
  compare          Compare live resources or connected DRY/WET/LIVE agreement
  tree             Hierarchical views: runtime, ownership, git, composition
  trace            Trace any resource to its Git source
  scan             Find misconfigurations (46 patterns)
  gitops status    GitOps pipeline health check
  import           Preview/import workloads to ConfigHub (connected mode)

CHOOSE YOUR INTERFACE
  TUI:   cub-scout map                       Interactive exploration
  CLI:   cub-scout map list -q "owner=Flux"  Scripting and pipelines
  JSON:  cub-scout map list --json | jq      Automation and tooling

STANDALONE VS CONNECTED
  Standalone (default): Works offline, reads from kubectl context
  Connected:            Optional ConfigHub integration for import/fleet features
                        Run 'cub auth login' to enable

TIPS
  • Use 'tree ownership' to see all resources grouped by GitOps tool
  • Add --json to most commands for machine-readable output
  • Use 'import --dry-run -n <namespace>' to preview ConfigHub import
  • Press '?' in TUI for keyboard shortcuts

Environment Variables:
  CLUSTER_NAME            Name for this cluster (default: default)
  KUBECONFIG              Path to kubeconfig file (default: ~/.kube/config)
  CUB_SCOUT_OFFLINE       Set to 'true' to force offline mode
`,
	Run: func(cmd *cobra.Command, args []string) {
		firstRun, err := detectAndMarkFirstRun()
		if err != nil {
			firstRun = false
		}
		renderRootLanding(os.Stdout, firstRun)
	},
}

func main() {
	// When running as a `cub` plugin (invoked via `cub scout ...`), the host
	// cub process sets CUB_PLUGIN=1 and exec's this binary. Switch help text
	// rendering so every command path reads `cub scout ...` instead of the
	// plugin binary's filesystem name (which will be "main" under
	// $CUB_CONFIG/plugins/scout/main).
	if hub.IsPluginMode() {
		applyPluginModeHelp(rootCmd)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// applyPluginModeHelp adjusts root command metadata and cobra templates so
// help output renders the `cub scout ...` invocation form across the entire
// command tree.
//
// Cobra derives each command's name from the first whitespace-separated
// token of its Use field, and CommandPath() walks parents joining Name().
// Setting the root Use to "cub scout" would make the root name "cub" and
// drop "scout" from every subcommand path (e.g. "cub mcp serve" instead of
// "cub scout mcp serve"). Instead:
//
//  1. Set the root Use to "scout" so Name() returns "scout".
//  2. Install usage and help templates that prepend a literal "cub " to
//     {{.UseLine}} and {{.CommandPath}}.
//
// Cobra's UsageTemplate()/HelpTemplate() walks the parent chain so setting
// it on the root applies to every descendant without explicit recursion.
func applyPluginModeHelp(root *cobra.Command) {
	root.Use = "scout"
	root.SetUsageTemplate(pluginModeUsageTemplate)
	root.SetHelpTemplate(pluginModeHelpTemplate)
}

// pluginModeUsageTemplate is cobra's default usage template with a literal
// "cub " prepended in front of {{.UseLine}} and {{.CommandPath}} so the
// rendered form reads `cub scout <subcmd>` across the tree.
const pluginModeUsageTemplate = `Usage:{{if .Runnable}}
  cub {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  cub {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "cub {{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// pluginModeHelpTemplate is cobra's default help template, unchanged since
// it only emits Long / Short text plus the usage template. Kept explicit so
// any future divergence is obvious.
const pluginModeHelpTemplate = `{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`

func init() {
	// Add version command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("cub-scout version %s (built %s)\n", BuildTag, BuildDate)
		},
	})

	rootCompletionCmd.Hidden = true
	rootCompletionCmd.Deprecated = "use 'cub-scout setup completion' instead"
	rootCmd.AddCommand(rootCompletionCmd)
}

var rootCompletionCmd = newCompletionCommand("completion [bash|zsh|fish|powershell]", "Generate shell completion script")

func newCompletionCommand(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
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
		RunE:                  runCompletionCommand,
	}
}

func runCompletionCommand(cmd *cobra.Command, args []string) error {
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
