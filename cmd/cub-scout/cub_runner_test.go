// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// The runner is the run-time half of #560's space rule. The AST guard reads
// source; these are the shapes it cannot see.
func TestCubRunnerRefusesACallThatNamesNoSpace(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string // empty means the call is allowed
	}{
		// Space-scoped, and the space is named one of the ways cub accepts.
		{name: "flag", args: []string{"unit", "list", "--space", "prod", "-o", "json"}},
		{name: "flag with equals", args: []string{"unit", "list", "--space=prod"}},
		{name: "every space, deliberately", args: []string{"worker", "list", "--space", "*", "-o", "json"}},
		{name: "space in the reference", args: []string{"unit", "get", "prod/api", "-o", "json"}},
		{name: "an ID carries its own space", args: []string{"unit", "get", "806aac53-236c-446d-8ad6-91d6daf6810e"}},

		// Space-scoped with nothing naming a space.
		{name: "bare list", args: []string{"unit", "list", "-o", "json"}, want: "reads one ConfigHub space and none was named"},
		{name: "bare slug", args: []string{"unit", "get", "api", "-o", "json"}, want: "reads one ConfigHub space and none was named"},
		{name: "trailing --space", args: []string{"unit", "list", "--space"}, want: "reads one ConfigHub space and none was named"},
		{name: "link list", args: []string{"link", "list", "-o", "json"}, want: "reads one ConfigHub space"},
		{name: "a flag value is not a reference", args: []string{"unit", "create", "api", "--from-file", "dir/file.yaml"}, want: "reads one ConfigHub space"},

		// The sentinel cub-scout sends when it resolved nothing: refused here,
		// with cub-scout's own reason rather than cub's exit 1.
		{name: "unresolved sentinel", args: []string{"unit", "list", "--space", unresolvedConfigHubSpace}, want: "cub-scout resolved none"},
		// cub reads `--space ""` as every space, exactly as it reads no --space
		// at all: 348 units against a live server, versus 2 for a named space.
		{name: "empty space", args: []string{"unit", "list", "--space", ""}, want: "empty ConfigHub space"},
		{name: "empty space with equals", args: []string{"unit", "list", "--space="}, want: "empty ConfigHub space"},
		{name: "whitespace space", args: []string{"unit", "list", "--space", "   "}, want: "empty ConfigHub space"},
		// cobra keeps the last value when a flag repeats, so the check reads
		// the last one too.
		{name: "the last --space is the one cub uses", args: []string{"unit", "list", "--space", "prod", "--space", unresolvedConfigHubSpace}, want: "cub-scout resolved none"},
		{name: "the last --space names a real space", args: []string{"unit", "list", "--space", unresolvedConfigHubSpace, "--space", "prod"}},

		// A target carries its own space, which is how a read scoped by target
		// alone works: withConfigHubSpaceOrTargets emits no --space for it.
		{name: "a qualified target carries the space", args: []string{"k8s", "get", "Deployment", "--target", "prod/cluster-a", "-o", "json"}},
		{name: "a target ID carries the space", args: []string{"k8s", "get", "Deployment", "--target", "806aac53-236c-446d-8ad6-91d6daf6810e"}},
		{name: "several targets, each qualified", args: []string{"k8s", "get", "Deployment", "--target", "prod/a,prod/b"}},
		{name: "an unqualified target names no space", args: []string{"k8s", "get", "Deployment", "--target", "cluster-a"}, want: "reads one ConfigHub space"},

		// Commands that name no space.
		{name: "auth status", args: []string{"auth", "status"}},
		{name: "context get", args: []string{"context", "get", "-o", "json"}},
		{name: "version", args: []string{"version"}},
		{name: "space list", args: []string{"space", "list", "-o", "json"}},

		// Removed surface.
		{name: "unit livedata", args: []string{"unit", "livedata", "api", "--space", "prod"}, want: "no longer exists"},
		{name: "unit apply", args: []string{"unit", "apply", "api", "--space", "prod"}, want: "no longer exists"},
		// A space command names its space as the positional.
		{name: "space get", args: []string{"space", "get", "prod"}},
		{name: "space create", args: []string{"space", "create", "prod", "-o", "json"}},
		// `cub gitops` was removed too, but an unknown top-level command exits
		// 1 saying so, and a plugin can supply one, so the runner lets it
		// through rather than refusing a user who has that plugin (#573).
		{name: "gitops import", args: []string{"gitops", "import", "--space", "prod"}},
		{name: "the version flag", args: []string{"version", "--version"}, want: "cannot take --version"},
		{name: "the deprecated json flag", args: []string{"unit", "list", "--space", "prod", "--json"}, want: "cannot take --json"},
		{name: "the deprecated json flag with a value", args: []string{"unit", "list", "--space", "prod", "--json=true"}, want: "cannot take --json"},

		{name: "no subcommand", args: nil, want: "needs a subcommand"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := checkCubArgs(tt.args)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("cub %s was refused: %v", strings.Join(tt.args, " "), err)
				}
				return
			}
			if err == nil {
				t.Fatalf("cub %s was allowed, want a refusal naming %q", strings.Join(tt.args, " "), tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// A refusal names the command cub-scout was about to run, so the reader knows
// which read was skipped rather than only that something needed a space.
func TestCubRunnerRefusalNamesTheCommand(t *testing.T) {
	err := checkCubArgs([]string{"changeset", "list", "-o", "json"})
	if err == nil || !strings.Contains(err.Error(), "cub changeset list") {
		t.Fatalf("err = %v, want it to name `cub changeset list`", err)
	}
}

// A cub subcommand nobody has classified is treated as space-scoped, so a new
// one fails closed rather than quietly reading the organization.
func TestCubRunnerTreatsAnUnknownCommandAsSpaceScoped(t *testing.T) {
	if err := checkCubArgs([]string{"newthing", "list", "-o", "json"}); err == nil {
		t.Fatal("cub newthing list was allowed with no space; an unclassified command must fail closed")
	}
	if err := checkCubArgs([]string{"newthing", "list", "--space", "prod"}); err != nil {
		t.Fatalf("cub newthing list --space prod was refused: %v", err)
	}
}

func TestCubCommandBuildsTheCallAndRefusesBeforeSpawning(t *testing.T) {
	cmd, err := cubCommand(context.Background(), "unit", "list", "--space", "prod", "-o", "json")
	if err != nil {
		t.Fatalf("cubCommand: %v", err)
	}
	if got := strings.Join(cmd.Args, " "); got != "cub unit list --space prod -o json" {
		t.Fatalf("argv = %q", got)
	}

	cmd, err = cubCommand(context.Background(), "unit", "list", "-o", "json")
	if err == nil {
		t.Fatal("cubCommand returned a command for a call with no space")
	}
	if cmd != nil {
		t.Fatal("cubCommand returned both a command and an error")
	}
}

// cubStdout is the read path: stdout is the data, stderr is the reason.
func TestCubStdoutKeepsStderrOutOfTheData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"echo 'Flag --json has been deprecated, use -o json' >&2\n" +
		"echo '[]'\n"
	if err := os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, err := cubStdout(context.Background(), "unit", "list", "--space", "prod", "-o", "json")
	if err != nil {
		t.Fatalf("cubStdout: %v", err)
	}
	if strings.TrimSpace(string(out)) != "[]" {
		t.Fatalf("data = %q, want cub's stderr kept out of it", out)
	}

	text, err := cubText(context.Background(), "unit", "list", "--space", "prod", "-o", "json")
	if err != nil {
		t.Fatalf("cubText: %v", err)
	}
	if text != "[]" {
		t.Fatalf("text = %q, want it trimmed", text)
	}
}

// A failing cub call carries cub's own reason, and the command that produced it.
func TestCubTextCarriesCubsReason(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'Failed: space \"prod\" not found' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := cubText(context.Background(), "unit", "list", "--space", "prod", "-o", "json")
	if err == nil {
		t.Fatal("err = nil, want the failure")
	}
	for _, want := range []string{"cub unit list --space prod -o json", `space "prod" not found`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to contain %q", err, want)
		}
	}
}

// Every cub call goes through the runner, so the space rule and the removed
// surface are checked for argument vectors no AST guard can read.
//
// pkg/hub is the one exception, and it is named here rather than left
// unchecked: it runs `cub auth status` and `cub auth get-token`, which is the
// gate the runner's callers are behind, and it cannot import from package main.
// Both name no space, so the runner would pass them through.
func TestEveryCubCallGoesThroughTheRunner(t *testing.T) {
	// pkg/hub cannot import package main, so the few cub calls it has to make
	// are named here rather than left unchecked.
	theGateItself := map[string]bool{
		filepath.Join("pkg", "hub", "connected.go"):    true, // cub auth status
		filepath.Join("pkg", "hub", "auth.go"):         true, // cub auth get-token
		filepath.Join("pkg", "hub", "oci_registry.go"): true, // cub context get, for the server's own registry
	}

	fset, files := parseGoFilesIncludingTests(t, "cmd", "pkg")
	var problems []string
	for path, file := range files {
		base := filepath.Base(path)
		if base == "cub_runner.go" || strings.HasSuffix(base, "_test.go") {
			continue
		}
		if theGateItself[filepath.Join(filepath.Base(filepath.Dir(filepath.Dir(path))), filepath.Base(filepath.Dir(path)), base)] {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := spaceGuardCalledName(call)
			var command ast.Expr
			switch {
			case name == "Command" && len(call.Args) >= 1:
				command = call.Args[0]
			case name == "CommandContext" && len(call.Args) >= 2:
				command = call.Args[1]
			default:
				return true
			}
			if text, isLit := spaceGuardStringLit(command); isLit && text == "cub" {
				problems = append(problems, fmt.Sprintf("%s: use cubCommand, cubStdout or cubText", fset.Position(call.Pos())))
			}
			return true
		})
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("%d cub calls bypass the runner:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}
