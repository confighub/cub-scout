// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/hub"
)

func TestRequireConfigHubFor_NamesTheCommandAndKeepsTheCause(t *testing.T) {
	old := requireCubConnectedFn
	t.Cleanup(func() { requireCubConnectedFn = old })

	requireCubConnectedFn = func() error { return hub.ErrCubNotLoggedIn }
	err := requireConfigHubFor("compare source-truth")
	if !errors.Is(err, hub.ErrCubNotLoggedIn) {
		t.Fatalf("error = %v, want it to wrap %v", err, hub.ErrCubNotLoggedIn)
	}
	for _, want := range []string{"compare source-truth", "cub auth login"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err, want)
		}
	}

	requireCubConnectedFn = func() error { return nil }
	if err := requireConfigHubFor("views resolve"); err != nil {
		t.Fatalf("error = %v, want nil when ConfigHub is reachable through cub", err)
	}
}

// parseNonTestGoFiles parses every non-test Go file under the given repo-relative
// directories.
func parseNonTestGoFiles(t *testing.T, dirs ...string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, dir := range dirs {
		root := filepath.Join("..", "..", dir)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			files[path] = file
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	return fset, files
}

func isHubCall(expr ast.Expr, fn string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != fn {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "hub"
}

// hub.QuickMode() is a display helper that never consults the cub CLI.
// Comparing it to decide whether a command may run refuses a logged-in user in
// standalone form. Commands gate with requireConfigHubFor instead.
func TestNoCommandGatesOnQuickMode(t *testing.T) {
	fset, files := parseNonTestGoFiles(t, "cmd")
	for path, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			cmp, ok := n.(*ast.BinaryExpr)
			if !ok || (cmp.Op != token.EQL && cmp.Op != token.NEQ) {
				return true
			}
			if isHubCall(cmp.X, "QuickMode") || isHubCall(cmp.Y, "QuickMode") {
				t.Errorf("%s compares hub.QuickMode() to gate behaviour; use requireConfigHubFor", fset.Position(cmp.Pos()))
			}
			return true
		})
		_ = path
	}
}

func TestConnectedOnlyCommandsUseTheSharedGate(t *testing.T) {
	_, files := parseNonTestGoFiles(t, "cmd")
	got := map[string]bool{}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "requireConfigHubFor" {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok {
				if feature, err := strconv.Unquote(lit.Value); err == nil {
					got[feature] = true
				}
			}
			return true
		})
	}
	for _, want := range []string{"compare source-truth", "compare three-way --view", "views resolve", "views project"} {
		if !got[want] {
			t.Errorf("no requireConfigHubFor(%q) call found; gated commands = %v", want, gatedFeatureNames(got))
		}
	}
}

func gatedFeatureNames(m map[string]bool) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var envVarToken = regexp.MustCompile(`\b(?:CONFIGHUB|CUB_SCOUT|CUB)_[A-Z0-9]+(?:_[A-Z0-9]+)*\b`)

var envVarAssignment = regexp.MustCompile(`^((?:CONFIGHUB|CUB_SCOUT|CUB)_[A-Z0-9_]+)=`)

// Environment variables the `cub` CLI reads itself. Text here may name them
// because a child `cub` process honours them even though cub-scout does not.
var envVarsReadByCub = map[string]bool{
	"CUB_CONFIG":  true,
	"CUB_CONTEXT": true,
	"CUB_SERVER":  true,
}

// A message must never tell the user to set an environment variable that
// nothing reads. Every CONFIGHUB_*, CUB_SCOUT_* or CUB_* name that appears in a
// string in cmd/ or pkg/ must be one the code reads: passed to os.Getenv or
// os.LookupEnv, held in a declared constant, or formed from a declared prefix.
func TestEveryEnvVarNamedInTextIsRead(t *testing.T) {
	fset, files := parseNonTestGoFiles(t, "cmd", "pkg")

	read := map[string]bool{}
	var prefixes []string
	mentioned := map[string]token.Position{}

	for _, file := range files {
		declared := map[ast.Node]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ValueSpec:
				// const envCubToken = "CUB_TOKEN"; const prefix = "CUB_SCOUT_BOT_"
				for _, value := range node.Values {
					lit, ok := value.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					text, err := strconv.Unquote(lit.Value)
					if err != nil {
						continue
					}
					declared[lit] = true
					if strings.HasSuffix(text, "_") && envVarToken.MatchString(text+"X") {
						prefixes = append(prefixes, text)
					} else if envVarToken.FindString(text) == text {
						read[text] = true
					}
				}
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "Getenv" && sel.Sel.Name != "LookupEnv" && sel.Sel.Name != "Setenv") {
					return true
				}
				for _, arg := range node.Args {
					if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if text, err := strconv.Unquote(lit.Value); err == nil && envVarToken.FindString(text) == text {
							read[text] = true
							declared[lit] = true
						}
					}
				}
			}
			return true
		})
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || declared[lit] {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			// "CUB_CLUSTER="+name builds a child process's environment: cub-scout
			// is exporting the variable, not telling anyone to set it.
			if exported := envVarAssignment.FindStringSubmatch(text); exported != nil {
				read[exported[1]] = true
				return true
			}
			for _, name := range envVarToken.FindAllString(text, -1) {
				if _, seen := mentioned[name]; !seen {
					mentioned[name] = fset.Position(lit.Pos())
				}
			}
			return true
		})
	}

	var unread []string
	for name, pos := range mentioned {
		if read[name] || envVarsReadByCub[name] {
			continue
		}
		fromPrefix := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(name, prefix) {
				fromPrefix = true
				break
			}
		}
		if !fromPrefix {
			unread = append(unread, name+" (first named at "+pos.String()+")")
		}
	}
	sort.Strings(unread)
	if len(unread) > 0 {
		t.Fatalf("text names environment variables that nothing reads:\n  %s", strings.Join(unread, "\n  "))
	}
}
