// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up shell completions and configuration",
	Long: `Set up cub-scout for your environment.

This command installs shell completions so you can use tab completion
for all cub-scout commands, flags, and arguments.

Supported shells: bash, zsh, fish

Examples:
  cub-scout setup              # Auto-detect shell and install
  cub-scout setup --shell zsh  # Install for specific shell
  cub-scout setup --dry-run    # Show what would be installed`,
	RunE: runSetup,
}

var (
	setupShell  string
	setupDryRun bool
)

func init() {
	setupCmd.Flags().StringVar(&setupShell, "shell", "", "Shell to configure (bash, zsh, fish). Auto-detects if not specified.")
	setupCmd.Flags().BoolVar(&setupDryRun, "dry-run", false, "Show what would be done without making changes")
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	// Detect shell if not specified
	shell := setupShell
	if shell == "" {
		shell = detectShell()
	}

	fmt.Printf("Setting up cub-scout for %s...\n\n", shell)

	switch shell {
	case "bash":
		return setupBash()
	case "zsh":
		return setupZsh()
	case "fish":
		return setupFish()
	default:
		return fmt.Errorf("unsupported shell: %s (supported: bash, zsh, fish)", shell)
	}
}

func detectShell() string {
	// Check SHELL environment variable
	shellPath := os.Getenv("SHELL")
	if shellPath != "" {
		base := filepath.Base(shellPath)
		switch base {
		case "bash", "zsh", "fish":
			return base
		}
	}

	// Default based on OS
	if runtime.GOOS == "darwin" {
		return "zsh" // macOS default since Catalina
	}
	return "bash"
}

func setupBash() error {
	rcFile := filepath.Join(os.Getenv("HOME"), ".bashrc")

	// Check if already configured
	if isAlreadyConfigured(rcFile, "cub-scout completion bash") {
		fmt.Println("✓ Shell completions already configured in ~/.bashrc")
		return nil
	}

	completionLine := `
# cub-scout completion (added by cub-scout setup)
source <(cub-scout completion bash)
`

	if setupDryRun {
		fmt.Println("Would add to ~/.bashrc:")
		fmt.Println(completionLine)
		return nil
	}

	if err := appendToFile(rcFile, completionLine); err != nil {
		return fmt.Errorf("failed to update ~/.bashrc: %w", err)
	}

	fmt.Println("✓ Added completion to ~/.bashrc")
	fmt.Println("\nRestart your shell or run:")
	fmt.Println("  source ~/.bashrc")

	return nil
}

func setupZsh() error {
	// Create completions directory
	compDir := filepath.Join(os.Getenv("HOME"), ".zsh", "completions")
	compFile := filepath.Join(compDir, "_cub-scout")
	rcFile := filepath.Join(os.Getenv("HOME"), ".zshrc")

	if setupDryRun {
		fmt.Printf("Would create: %s\n", compDir)
		fmt.Printf("Would write completion to: %s\n", compFile)
		fmt.Println("Would add to ~/.zshrc (if not present):")
		fmt.Println("  fpath=(~/.zsh/completions $fpath)")
		fmt.Println("  autoload -Uz compinit && compinit")
		return nil
	}

	// Create directory
	if err := os.MkdirAll(compDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", compDir, err)
	}

	// Generate completion script
	completionScript, err := generateZshCompletion()
	if err != nil {
		return fmt.Errorf("failed to generate completion: %w", err)
	}

	if err := os.WriteFile(compFile, []byte(completionScript), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", compFile, err)
	}
	fmt.Printf("✓ Wrote completion script to %s\n", compFile)

	// Add fpath to .zshrc if needed
	if !isAlreadyConfigured(rcFile, ".zsh/completions") {
		fpathConfig := `
# cub-scout completion (added by cub-scout setup)
fpath=(~/.zsh/completions $fpath)
autoload -Uz compinit && compinit
`
		if err := appendToFile(rcFile, fpathConfig); err != nil {
			return fmt.Errorf("failed to update ~/.zshrc: %w", err)
		}
		fmt.Println("✓ Added completion path to ~/.zshrc")
	} else {
		fmt.Println("✓ Completion path already in ~/.zshrc")
	}

	fmt.Println("\nRestart your shell or run:")
	fmt.Println("  source ~/.zshrc")

	return nil
}

func setupFish() error {
	compDir := filepath.Join(os.Getenv("HOME"), ".config", "fish", "completions")
	compFile := filepath.Join(compDir, "cub-scout.fish")

	if setupDryRun {
		fmt.Printf("Would create: %s\n", compDir)
		fmt.Printf("Would write completion to: %s\n", compFile)
		return nil
	}

	// Create directory
	if err := os.MkdirAll(compDir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", compDir, err)
	}

	// Generate completion script
	completionScript, err := generateFishCompletion()
	if err != nil {
		return fmt.Errorf("failed to generate completion: %w", err)
	}

	if err := os.WriteFile(compFile, []byte(completionScript), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", compFile, err)
	}

	fmt.Printf("✓ Wrote completion script to %s\n", compFile)
	fmt.Println("\nFish will auto-load completions on next shell start.")

	return nil
}

func generateZshCompletion() (string, error) {
	// Use cobra's built-in completion generation
	cmd := exec.Command(os.Args[0], "completion", "zsh")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func generateFishCompletion() (string, error) {
	cmd := exec.Command(os.Args[0], "completion", "fish")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func isAlreadyConfigured(filename, searchStr string) bool {
	content, err := os.ReadFile(filename)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), searchStr)
}

func appendToFile(filename, content string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}
