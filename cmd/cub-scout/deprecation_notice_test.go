package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Every flag and command listed for 3.0 removal in docs/migration-3.0.md (#595,
// #562) must be absent from the command tree and must fail with an error that
// names 3.0 and its replacement — never silently ignored.
func TestThreeZeroRemovalsFailAndNameReplacement(t *testing.T) {
	removedFlags := []struct {
		name        string
		cmd         func(t *testing.T) *cobra.Command
		flag        string
		args        []string
		wantReplace string
	}{
		{
			name:        "trace --json",
			cmd:         func(t *testing.T) *cobra.Command { return traceCmd },
			flag:        "json",
			args:        []string{"trace", "deployment/app", "--json"},
			wantReplace: "--format json",
		},
		{
			name:        "trace --json=true",
			cmd:         func(t *testing.T) *cobra.Command { return traceCmd },
			flag:        "json",
			args:        []string{"trace", "deployment/app", "--json=true"},
			wantReplace: "--format json",
		},
		{
			name:        "tree --json",
			cmd:         func(t *testing.T) *cobra.Command { return treeCmd },
			flag:        "json",
			args:        []string{"tree", "--json"},
			wantReplace: "--format json",
		},
		{
			name:        "graph export --json",
			cmd:         func(t *testing.T) *cobra.Command { return graphExportCmd },
			flag:        "json",
			args:        []string{"graph", "export", "--json"},
			wantReplace: "--format json",
		},
		{
			name:        "gitops status --json",
			cmd:         func(t *testing.T) *cobra.Command { return gitopsStatusCmd },
			flag:        "json",
			args:        []string{"gitops", "status", "--json"},
			wantReplace: "--format json",
		},
		{
			name: "connect --go",
			cmd: func(t *testing.T) *cobra.Command {
				c, _, err := rootCmd.Find([]string{"connect"})
				if err != nil {
					t.Fatal(err)
				}
				return c
			},
			flag:        "go",
			args:        []string{"connect", "--go"},
			wantReplace: "--map",
		},
		{
			name: "setup connect --go",
			cmd: func(t *testing.T) *cobra.Command {
				c, _, err := rootCmd.Find([]string{"setup", "connect"})
				if err != nil {
					t.Fatal(err)
				}
				return c
			},
			flag:        "go",
			args:        []string{"setup", "connect", "--go"},
			wantReplace: "--map",
		},
		{
			name:        "app create --set-context",
			cmd:         func(t *testing.T) *cobra.Command { return appCreateCmd },
			flag:        "set-context",
			args:        []string{"app", "create", "demo", "--" + "set-context"},
			wantReplace: "--space",
		},
	}

	for _, tc := range removedFlags {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.cmd(t)
			if f := cmd.Flags().Lookup(tc.flag); f != nil {
				t.Fatalf("%s still registers removed flag --%s", cmd.CommandPath(), tc.flag)
			}

			var out, errOut bytes.Buffer
			rootCmd.SetOut(&out)
			rootCmd.SetErr(&errOut)
			rootCmd.SetArgs(tc.args)
			t.Cleanup(func() {
				rootCmd.SetOut(nil)
				rootCmd.SetErr(nil)
				rootCmd.SetArgs(nil)
			})

			err := executeRootCmd(tc.args)
			if err == nil {
				t.Fatalf("%v succeeded; want error naming 3.0 and %q", tc.args, tc.wantReplace)
			}
			msg := err.Error()
			if !strings.Contains(msg, "removed in 3.0") || !strings.Contains(msg, tc.wantReplace) {
				t.Fatalf("%v error = %q; want both %q and %q", tc.args, msg, "removed in 3.0", tc.wantReplace)
			}
		})
	}

	removedCommands := []struct {
		name        string
		args        []string
		wantReplace string
	}{
		{
			name:        "hierarchy",
			args:        []string{"hierarchy"},
			wantReplace: "map --hub",
		},
		{
			name:        "fleet outliers",
			args:        []string{"fleet", "outliers"},
			wantReplace: "docs/migration-3.0.md",
		},
	}

	for _, tc := range removedCommands {
		t.Run(tc.name, func(t *testing.T) {
			if cmd, _, err := rootCmd.Find(tc.args); err == nil && cmd.CommandPath() == "cub-scout "+strings.Join(tc.args, " ") {
				t.Fatalf("removed command %q is still registered on rootCmd", tc.name)
			}

			var out, errOut bytes.Buffer
			rootCmd.SetOut(&out)
			rootCmd.SetErr(&errOut)
			rootCmd.SetArgs(tc.args)
			t.Cleanup(func() {
				rootCmd.SetOut(nil)
				rootCmd.SetErr(nil)
				rootCmd.SetArgs(nil)
			})

			err := executeRootCmd(tc.args)
			if err == nil {
				t.Fatalf("%v succeeded; want error naming 3.0 and %q", tc.args, tc.wantReplace)
			}
			msg := err.Error()
			if !strings.Contains(msg, "removed in 3.0") || !strings.Contains(msg, tc.wantReplace) {
				t.Fatalf("%v error = %q; want both %q and %q", tc.args, msg, "removed in 3.0", tc.wantReplace)
			}
		})
	}
}

// The migration guide lists every removal 3.0 performs.
func TestMigrationGuideListsEveryRemoval(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "migration-3.0.md"))
	if err != nil {
		t.Fatal(err)
	}
	guide := string(raw)
	for _, want := range []string{
		"`trace --json`", "`tree --json`", "`graph export --json`", "`gitops status --json`",
		"`connect --go`", "`app create --set-context`", "`hierarchy`", "`fleet outliers`", "verdict words",
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("docs/migration-3.0.md does not list %s", want)
		}
	}
}
