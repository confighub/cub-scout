// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
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

// cub has removed subcommands without a deprecation period, and a removed
// subcommand does not fail: cub prints the group's help and exits 0, so a
// caller that checks only the exit code gets 38 lines of help where it expected
// data — or reports success having done nothing. That reached `compare` (#564),
// the import test-update flow (#571) and three `unit apply` call sites, one of
// which claimed success in the TUI.
//
// Pairs, not single words: "apply" on its own is a kubectl verb.
//
// Prose that explains a removal is allowed; an argument vector is not, and
// neither is a command printed for a reader to run. Test files are scanned too:
// the first version of this guard missed a test that pinned `unit apply` as
// required argv.
func TestNoCubCallUsesARemovedSubcommand(t *testing.T) {
	removed := map[[2]string]string{
		{"unit", "livedata"}:  "a unit's config data is `cub unit data`; ConfigHub has no live-data endpoint (#564, #571)",
		{"unit", "livestate"}: "removed with livedata in April 2026 (#564)",
		{"unit", "apply"}:     "removed in July 2026; nothing applies a unit — see cub_unit_apply.go (#571)",
		{"unit", "destroy"}:   "removed in July 2026 with unit apply",
		{"unit", "refresh"}:   "removed in July 2026; the current verb is `cub k8s refresh`",
	}

	// Flags cub no longer accepts, for the same reason: it rejects them, and a
	// caller that only checks the exit code reads the refusal as an answer.
	removedFlags := map[string]string{
		"--version": "cub takes `version` as a subcommand and rejects the flag",
	}

	fset, files := parseGoFilesIncludingTests(t, "cmd", "pkg")
	var problems []string
	for path, file := range files {
		// cubArgLiterals skips a call with fewer than two arguments, and
		// `cub --version` is exactly that, so this walks the calls itself.
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := spaceGuardCalledName(call)
			argv := call.Args
			switch {
			case name == "Command" && len(call.Args) >= 1:
				if text, isLit := spaceGuardStringLit(call.Args[0]); !isLit || text != "cub" {
					return true
				}
				argv = call.Args[1:]
			case name == "CommandContext" && len(call.Args) >= 2:
				if text, isLit := spaceGuardStringLit(call.Args[1]); !isLit || text != "cub" {
					return true
				}
				argv = call.Args[2:]
			default:
				return true
			}
			for _, arg := range argv {
				text, isLit := spaceGuardStringLit(arg)
				if !isLit {
					continue
				}
				if why, isRemoved := removedFlags[text]; isRemoved {
					problems = append(problems, fmt.Sprintf("%s: `cub ... %s` — %s", fset.Position(arg.Pos()), text, why))
				}
			}
			return true
		})
		ast.Inspect(file, func(n ast.Node) bool {
			// An argument vector: adjacent string literals in a call or a slice.
			var args []ast.Expr
			switch node := n.(type) {
			case *ast.KeyValueExpr:
				// This guard's own table lists the removed pairs as map keys.
				if _, isPair := node.Key.(*ast.CompositeLit); isPair {
					return false
				}
				return true
			case *ast.CallExpr:
				args = node.Args
			case *ast.CompositeLit:
				args = node.Elts
			default:
				if text, ok := spaceGuardStringLit(n); ok {
					// A command printed for a reader to run. Trimmed, because
					// printed hints in this repo are indented.
					trimmed := strings.TrimSpace(text)
					if !strings.HasPrefix(trimmed, "cub ") {
						return true
					}
					for pair, why := range removed {
						// The command a reader would run starts with it. Prose
						// that mentions the removal in passing does not.
						if strings.HasPrefix(trimmed, "cub "+pair[0]+" "+pair[1]) {
							problems = append(problems, fmt.Sprintf("%s: %q — cub %s %s %s", fset.Position(n.Pos()), text, pair[0], pair[1], why))
						}
					}
				}
				return true
			}
			for i := 0; i+1 < len(args); i++ {
				first, ok := spaceGuardStringLit(args[i])
				if !ok {
					continue
				}
				second, ok := spaceGuardStringLit(args[i+1])
				if !ok {
					continue
				}
				if why, isRemoved := removed[[2]string{first, second}]; isRemoved {
					problems = append(problems, fmt.Sprintf("%s: `cub %s %s` — %s", fset.Position(args[i].Pos()), first, second, why))
				}
			}
			return true
		})
		_ = path
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("%d uses of a cub subcommand that no longer exists:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}

// parseGoFilesIncludingTests is spaceGuardParseFiles plus the test files, for a
// guard that has to cover what tests assert as well as what code runs.
func parseGoFilesIncludingTests(t *testing.T, dirs ...string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, dir := range dirs {
		root := filepath.Join("..", "..", dir)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if info.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			parsed, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			files[path] = parsed
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return fset, files
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
