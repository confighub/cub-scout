// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// cub entities that live in a space. A `cub <entity> <verb>` run without
// --space does not fail: it reads or writes whatever space the cub context
// happens to default to, or every space, which is a scope nobody chose.
var spaceScopedCubEntities = map[string]bool{
	"unit": true, "link": true, "target": true, "worker": true, "release": true,
	"unit-event": true, "resource": true, "view": true, "filter": true,
	"changeset": true, "revision": true, "trigger": true, "tag": true, "k8s": true,
	"gitops": true, "function": true, "invocation": true, "mutation": true,
}

// Verbs cub has for those entities. The MCP gateway also builds argument
// vectors for cub-scout's own commands ("gitops status", "release check"),
// which are not cub calls and have no space to name.
var cubVerbs = map[string]bool{
	"list": true, "get": true, "create": true, "update": true, "delete": true, "apply": true,
	"destroy": true, "data": true, "livedata": true, "tree": true, "import": true,
	"discover": true, "types": true, "source": true, "collect": true, "refresh": true,
	"publish": true, "upload": true, "approve": true, "tag": true, "edit": true, "set-target": true,
}

// Arguments that name their space another way, so --space is not required.
func namesItsOwnSpace(args []string) bool {
	for _, arg := range args {
		if arg == "--space" || strings.HasPrefix(arg, "--space=") {
			return true
		}
	}
	return false
}

type cubInvocation struct {
	pos  token.Position
	args []string
}

// cubArgLiterals finds every literal cub argument vector in a file: the
// arguments of exec.Command("cub", ...), of the repo's cub runner helpers, and
// any []string{...} literal that starts with a cub entity and verb, which is
// how arguments are built before being appended to.
func cubArgLiterals(fset *token.FileSet, file *ast.File) []cubInvocation {
	var out []cubInvocation
	literals := func(exprs []ast.Expr) []string {
		var args []string
		for _, expr := range exprs {
			if text, ok := spaceGuardStringLit(expr); ok {
				args = append(args, text)
			} else {
				args = append(args, "<expr>")
			}
		}
		return args
	}
	// A literal anywhere inside the first argument of withConfigHubSpace or
	// withConfigHubSpaceOrTargets gets its scope from that call.
	wrapped := map[ast.Node]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && strings.HasPrefix(spaceGuardCalledName(call), "withConfigHubSpace") && len(call.Args) > 0 {
			ast.Inspect(call.Args[0], func(inner ast.Node) bool {
				if inner != nil {
					wrapped[inner] = true
				}
				return true
			})
		}
		return true
	})
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			name := spaceGuardCalledName(node)
			var argv []ast.Expr
			switch {
			case name == "Command" && len(node.Args) >= 1:
				if text, ok := spaceGuardStringLit(node.Args[0]); ok && text == "cub" {
					argv = node.Args[1:]
				}
			case name == "CommandContext" && len(node.Args) >= 2:
				if text, ok := spaceGuardStringLit(node.Args[1]); ok && text == "cub" {
					argv = node.Args[2:]
				}
			case name == "runCubCommand":
				argv = node.Args
			}
			if len(argv) >= 2 && !wrapped[argv[0]] {
				out = append(out, cubInvocation{fset.Position(node.Pos()), literals(argv)})
			}
		case *ast.CompositeLit:
			arr, ok := node.Type.(*ast.ArrayType)
			if !ok || len(node.Elts) < 2 {
				return true
			}
			if elem, ok := arr.Elt.(*ast.Ident); !ok || elem.Name != "string" {
				return true
			}
			args := literals(node.Elts)
			if spaceScopedCubEntities[args[0]] && cubVerbs[args[1]] && !wrapped[node] {
				out = append(out, cubInvocation{fset.Position(node.Pos()), args})
			}
		}
		return true
	})
	return out
}

// Every cub call on a space-scoped entity must put its space on the command
// line, either as a literal --space or through withConfigHubSpace. Nothing may
// change the cub context's default space, which is shared, ambient state.
func TestEveryCubCallNamesItsSpace(t *testing.T) {
	fset, files := spaceGuardParseFiles(t, "cmd", "pkg")
	var problems []string
	for _, file := range files {
		for _, inv := range cubArgLiterals(fset, file) {
			joined := strings.Join(inv.args, " ")
			switch {
			case len(inv.args) >= 2 && inv.args[0] == "context" && inv.args[1] == "set":
				problems = append(problems, inv.pos.String()+": `cub "+joined+"` changes the cub context's default space for every shell")
			case strings.Contains(joined, "--set-context"):
				problems = append(problems, inv.pos.String()+": `cub "+joined+"` changes the cub context's default space for every shell")
			case spaceScopedCubEntities[inv.args[0]] && !namesItsOwnSpace(inv.args):
				problems = append(problems, inv.pos.String()+": `cub "+joined+"` names no space; use --space or withConfigHubSpace")
			}
		}
	}
	// --set-context is usually appended to arguments built elsewhere, so look
	// for the literal itself. The cub-scout flag of that name is declared as
	// "set-context", without the dashes, and is deprecated and ignored.
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if text, ok := spaceGuardStringLit(n); ok && text == "--set-context" {
				problems = append(problems, fset.Position(n.Pos()).String()+": passes --set-context to cub, which changes the cub context's default space for every shell")
			}
			return true
		})
	}
	sort.Strings(problems)
	if len(problems) > 0 {
		t.Fatalf("%d cub calls depend on the cub context's default space:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}

// The helpers below are private to this guard so it does not depend on any
// other test file in the package.

func spaceGuardStringLit(node ast.Node) (string, bool) {
	lit, ok := node.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	return text, err == nil
}

func spaceGuardCalledName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

func spaceGuardParseFiles(t *testing.T, dirs ...string) (*token.FileSet, map[string]*ast.File) {
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
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
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
