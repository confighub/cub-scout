// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"go/ast"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// The deprecation notice cub prints on stderr for --json.
const cubJSONFlagNotice = "Flag --json has been deprecated, use -o json"

func TestCommandStdoutKeepsStderrOutOfTheData(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	out, err := commandStdout(exec.Command("sh", "-c", `echo '`+cubJSONFlagNotice+`' >&2; echo '[{"Target":{"Slug":"prod","ProviderType":"Kubernetes"}}]'`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "deprecated") {
		t.Fatalf("stdout = %q, want the notice kept out of the data", out)
	}
	if got := firstKubernetesTargetSlug(out); got != "prod" {
		t.Fatalf("target slug = %q, want prod", got)
	}

	_, err = commandStdout(exec.Command("sh", "-c", `echo 'space "x" not found' >&2; exit 1`))
	if err == nil || !strings.Contains(err.Error(), `space "x" not found`) {
		t.Fatalf("err = %v, want the stderr reason in the error", err)
	}
}

// The import wizard used to pipe `cub target list --json` through jq with
// stderr merged. With no Kubernetes target, the output was the notice alone,
// which is not empty, so the notice became the target slug.
func TestFirstKubernetesTargetSlug(t *testing.T) {
	for _, tt := range []struct {
		name, raw, want string
	}{
		{name: "first Kubernetes target", raw: `[{"Target":{"Slug":"oci","ProviderType":"OCI"}},{"Target":{"Slug":"prod","ProviderType":"Kubernetes"}}]`, want: "prod"},
		{name: "no Kubernetes target", raw: `[{"Target":{"Slug":"oci","ProviderType":"OCI"}}]`},
		{name: "empty list", raw: `[]`},
		{name: "a stderr notice is not a slug", raw: cubJSONFlagNotice},
		{name: "a notice in front of the JSON is not a list", raw: cubJSONFlagNotice + "\n" + `[{"Target":{"Slug":"prod","ProviderType":"Kubernetes"}}]`},
	} {
		if got := firstKubernetesTargetSlug([]byte(tt.raw)); got != tt.want {
			t.Errorf("%s: slug = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestCubUnitHasTarget(t *testing.T) {
	for raw, want := range map[string]bool{
		`{"Unit":{"Slug":"api"},"Target":{"TargetID":"t-1","Slug":"prod"}}`: true,
		`{"Unit":{"Slug":"api","TargetID":"t-1"}}`:                          false,
		`{"Unit":{"Slug":"api"},"Target":null}`:                             false,
		`{"Unit":{"Slug":"api"},"Target":{}}`:                               false,
		cubJSONFlagNotice:                                                   false,
	} {
		if got := cubUnitHasTarget([]byte(raw)); got != want {
			t.Errorf("cubUnitHasTarget(%s) = %v, want %v", raw, got, want)
		}
	}
}

// cub has deprecated --json in favour of -o json. The flag still works but
// prints a notice on stderr on every call, and it will go away.
func TestNoCubCallPassesTheDeprecatedJSONFlag(t *testing.T) {
	fset, files := spaceGuardParseFiles(t, "cmd", "pkg")
	var problems []string
	for _, file := range files {
		for _, inv := range cubArgLiterals(fset, file) {
			for _, arg := range inv.args {
				if arg == "--json" {
					problems = append(problems, inv.pos.String()+": `cub "+strings.Join(inv.args, " ")+"` passes --json; use -o json")
				}
			}
		}
		// Commands printed for a reader to run.
		ast.Inspect(file, func(n ast.Node) bool {
			if text, ok := spaceGuardStringLit(n); ok && strings.HasPrefix(text, "cub ") && strings.Contains(text, "--json") {
				problems = append(problems, fset.Position(n.Pos()).String()+": "+text)
			}
			return true
		})
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("%d cub calls pass the deprecated --json flag:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}

// cub removed `unit livedata` and `unit livestate` in April 2026. A removed
// subcommand does not fail: cub prints the `unit` help and exits 0, so a caller
// that reads the output gets 38 lines of help text where it expected config
// data. That reached `compare` (#564) and the import test-update flow (#571).
//
// Prose explaining the removal is allowed; an argument vector is not.
func TestNoCubCallUsesARemovedSubcommand(t *testing.T) {
	removed := map[string]string{
		"livedata":  "a unit's config data is `cub unit data`; ConfigHub has no live-data endpoint (#564, #571)",
		"livestate": "removed with livedata in April 2026 (#564)",
	}
	fset, files := spaceGuardParseFiles(t, "cmd", "pkg")
	var problems []string
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			text, ok := spaceGuardStringLit(n)
			if !ok {
				return true
			}
			for word, why := range removed {
				// An argv element, or a command printed for a reader to run.
				if text == word || (strings.HasPrefix(text, "cub ") && strings.Contains(text, "unit "+word)) {
					problems = append(problems, fmt.Sprintf("%s: %q — %s", fset.Position(n.Pos()), text, why))
				}
			}
			return true
		})
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("%d uses of a cub subcommand that no longer exists:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}

func TestTreeConfigRefusesJSON(t *testing.T) {
	prevJSON, prevFormat := treeJSON, treeFormat
	t.Cleanup(func() { treeJSON, treeFormat = prevJSON, prevFormat })
	stubSpaceInputs(t, "platform")

	for _, set := range []func(){
		func() { treeJSON, treeFormat = true, "ascii" },
		func() { treeJSON, treeFormat = false, "json" },
	} {
		set()
		err := runTreeConfig()
		if err == nil || !strings.Contains(err.Error(), "tree config has no JSON output") {
			t.Fatalf("err = %v, want JSON refused", err)
		}
	}
}
