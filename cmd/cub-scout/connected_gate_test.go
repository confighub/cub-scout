// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
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
	"github.com/spf13/cobra"
)

func stubConnectedGate(t *testing.T, err error) {
	t.Helper()
	old := requireCubConnectedFn
	t.Cleanup(func() { requireCubConnectedFn = old })
	requireCubConnectedFn = func() error { return err }
}

func TestRequireConfigHubFor_NamesTheCommandAndKeepsTheCause(t *testing.T) {
	stubConnectedGate(t, hub.ErrCubNotInstalled)
	err := requireConfigHubFor("compare source-truth")
	if !errors.Is(err, hub.ErrCubNotInstalled) {
		t.Fatalf("error = %v, want it to wrap %v", err, hub.ErrCubNotInstalled)
	}
	for _, want := range []string{"compare source-truth", "cub auth login"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err, want)
		}
	}

	stubConnectedGate(t, nil)
	if err := requireConfigHubFor("views resolve"); err != nil {
		t.Fatalf("error = %v, want nil when cub reports an authenticated session", err)
	}
}

// Each connected-only command must refuse with the gate's own error, and must
// do so before it reads anything from ConfigHub.
func TestConnectedOnlyCommandsRefuseBeforeReadingConfigHub(t *testing.T) {
	const viewID = "806aac53-236c-446d-8ad6-91d6daf6810e"
	refusal := errors.New("gate refused")

	oldRunner := viewCubRunner
	t.Cleanup(func() { viewCubRunner = oldRunner })
	viewCubRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		t.Fatalf("ConfigHub was read (cub %s) although the gate refused", strings.Join(args, " "))
		return nil, nil
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "compare source-truth", run: func() error {
			oldFormat, oldStrategy := sourceTruthFormat, sourceTruthStrategy
			defer func() { sourceTruthFormat, sourceTruthStrategy = oldFormat, oldStrategy }()
			sourceTruthFormat, sourceTruthStrategy = "json", "git-argo"
			return runSourceTruth(&cobra.Command{}, []string{"deployment/api"})
		}},
		{name: "views resolve", run: func() error {
			return runViewsResolve(&cobra.Command{}, []string{viewID})
		}},
		{name: "views project", run: func() error {
			oldFormat := viewsProjectFormat
			defer func() { viewsProjectFormat = oldFormat }()
			viewsProjectFormat = "json"
			return runViewsProject(&cobra.Command{}, []string{viewID})
		}},
		{name: "compare three-way --view", run: func() error {
			oldScope, oldView := compareThreeWayScopeRaw, compareThreeWayView
			defer func() { compareThreeWayScopeRaw, compareThreeWayView = oldScope, oldView }()
			compareThreeWayScopeRaw, compareThreeWayView = "", viewID
			return runCompareThreeWay(&cobra.Command{}, nil)
		}},
		{name: "import argocd", run: checkCubAuth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubConnectedGate(t, refusal)
			err := tt.run()
			if !errors.Is(err, refusal) {
				t.Fatalf("error = %v, want the gate's refusal", err)
			}
			if !strings.Contains(err.Error(), tt.name) {
				t.Fatalf("error = %q, want it to name %q", err, tt.name)
			}
		})
	}
}

// The MCP gateway offers its connected tools exactly when the CLI would run
// them, so `compare source-truth` cannot work on the CLI while the MCP tool of
// the same name is never listed.
func TestMCPConnectedModeFollowsTheSameGate(t *testing.T) {
	stubConnectedGate(t, nil)
	if !detectMCPConnectedMode() {
		t.Fatal("detectMCPConnectedMode() = false although the gate passes")
	}
	stubConnectedGate(t, hub.ErrCubNotAuthenticated)
	if detectMCPConnectedMode() {
		t.Fatal("detectMCPConnectedMode() = true although the gate refuses")
	}
}

// parseRepoGoFiles parses every non-test Go file under the given repo-relative
// directories, skipping testdata.
func parseRepoGoFiles(t *testing.T, dirs ...string) (*token.FileSet, map[string]*ast.File) {
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

// hub.QuickMode and hub.IsAuthenticated never consult the cub CLI, so neither
// may decide whether a command runs. Rather than chase every way of writing
// such a test (a comparison, a switch, a stored value), allow the names only
// where they are known to be display-only.
func TestDisplayOnlyHubChecksStayOutOfCommandGates(t *testing.T) {
	allowed := map[string]string{
		"localcluster.go": "QuickMode", // TUI header, refined asynchronously
	}
	fset, files := parseRepoGoFiles(t, "cmd")
	for path, file := range files {
		base := filepath.Base(path)
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "hub" || (sel.Sel.Name != "QuickMode" && sel.Sel.Name != "IsAuthenticated") {
				return true
			}
			if allowed[base] != sel.Sel.Name {
				t.Errorf("%s uses hub.%s, which never consults the cub CLI; gate with requireConfigHubFor", fset.Position(sel.Pos()), sel.Sel.Name)
			}
			return true
		})
	}
}

var (
	envVarName       = regexp.MustCompile(`\b(?:CONFIGHUB|CUB_SCOUT|CUB)_[A-Z0-9]+(?:_[A-Z0-9]+)*\b`)
	envVarAssignment = regexp.MustCompile(`^(?:CONFIGHUB|CUB_SCOUT|CUB)_[A-Z0-9_]+=\S*$`)
)

// envVarsReadOutsideThisRepo are read by the cub CLI, not by cub-scout, so text
// may name them. Each was checked against cub v0.5.1: CUB_CONTEXT selects the
// context, CUB_CONFIG names the config directory.
var envVarsReadOutsideThisRepo = map[string]bool{
	"CUB_CONFIG":  true,
	"CUB_CONTEXT": true,
}

// A message must never tell someone to set an environment variable that
// nothing reads. Every CONFIGHUB_*, CUB_SCOUT_* or CUB_* name appearing in a
// string in cmd/, pkg/ or internal/ must reach os.Getenv or os.LookupEnv:
// directly, through a constant, or through a function that passes its
// parameter on to one of them. Declaring a constant is not enough.
func TestEveryEnvVarNamedInTextIsRead(t *testing.T) {
	fset, files := parseRepoGoFiles(t, "cmd", "pkg", "internal")
	if unread := unreadEnvVarMentions(fset, files); len(unread) > 0 {
		t.Fatalf("text names environment variables that nothing reads:\n  %s", strings.Join(unread, "\n  "))
	}
}

// The guard must catch the defect it was written for, in each shape it could
// return in, and must not be silenced by a constant nobody reads.
func TestEnvVarGuardCatchesPhantoms(t *testing.T) {
	scan := func(t *testing.T, src string) []string {
		t.Helper()
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "sample.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		return unreadEnvVarMentions(fset, map[string]*ast.File{"sample.go": file})
	}

	phantoms := map[string]string{
		"error string":            "package p\nimport \"fmt\"\nfunc f() error { return fmt.Errorf(\"run cub auth login or set CONFIGHUB_API_KEY\") }",
		"message held in a const": "package p\nconst authHint = \"run cub auth login or set CONFIGHUB_API_KEY\"",
		"unread constant":         "package p\nimport \"fmt\"\nconst envAPIKey = \"CONFIGHUB_API_KEY\"\nfunc f() error { return fmt.Errorf(\"set CONFIGHUB_API_KEY\") }",
		"shell example":           "package p\nvar example = []string{\"CONFIGHUB_API_KEY=<key> cub-scout compare source-truth\"}",
		"only ever set":           "package p\nimport \"os\"\nfunc f() { os.Setenv(\"CONFIGHUB_API_KEY\", \"x\") }\nvar help = \"set CONFIGHUB_API_KEY\"",
	}
	for name, src := range phantoms {
		t.Run("phantom/"+name, func(t *testing.T) {
			unread := scan(t, src)
			if len(unread) != 1 || !strings.HasPrefix(unread[0], "CONFIGHUB_API_KEY") {
				t.Fatalf("unread = %v, want the phantom CONFIGHUB_API_KEY reported", unread)
			}
		})
	}

	legitimate := map[string]string{
		"read directly":           "package p\nimport \"os\"\nvar v = os.Getenv(\"CUB_SCOUT_DEBUG\")\nvar help = \"set CUB_SCOUT_DEBUG=1 to debug\"",
		"read through a constant": "package p\nimport \"os\"\nconst envDebug = \"CUB_SCOUT_DEBUG\"\nvar v = os.Getenv(envDebug)\nvar help = \"set CUB_SCOUT_DEBUG=1 to debug\"",
		"read through a helper":   "package p\nimport \"os\"\nfunc envOr(name, d string) string { if v, ok := os.LookupEnv(name); ok { return v }; return d }\nvar v = envOr(\"CUB_SCOUT_DEBUG\", \"\")\nvar help = \"set CUB_SCOUT_DEBUG=1 to debug\"",
		"exported to a child":     "package p\nvar env = []string{\"CUB_SCOUT_TUI=1\"}",
	}
	for name, src := range legitimate {
		t.Run("legitimate/"+name, func(t *testing.T) {
			if unread := scan(t, src); len(unread) != 0 {
				t.Fatalf("unread = %v, want none", unread)
			}
		})
	}
}

func stringLiteral(node ast.Node) (string, bool) {
	lit, ok := node.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	return text, err == nil
}

func isEnvReader(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Getenv" && sel.Sel.Name != "LookupEnv") {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os"
}

func calledName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

func unreadEnvVarMentions(fset *token.FileSet, files map[string]*ast.File) []string {
	// Pass 1: constants holding exactly one variable name, and helpers that
	// hand one of their parameters to os.Getenv or os.LookupEnv.
	constants := map[string]string{}
	helpers := map[string]int{}
	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, value := range vs.Values {
						if text, ok := stringLiteral(value); ok && i < len(vs.Names) && envVarName.FindString(text) == text {
							constants[vs.Names[i].Name] = text
						}
					}
				}
			case *ast.FuncDecl:
				if d.Body == nil || d.Type.Params == nil {
					continue
				}
				index := map[string]int{}
				n := 0
				for _, field := range d.Type.Params.List {
					for _, name := range field.Names {
						index[name.Name] = n
						n++
					}
				}
				ast.Inspect(d.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok || !isEnvReader(call) || len(call.Args) == 0 {
						return true
					}
					if id, ok := call.Args[0].(*ast.Ident); ok {
						if i, isParam := index[id.Name]; isParam {
							helpers[d.Name.Name] = i
						}
					}
					return true
				})
			}
		}
	}

	// Pass 2: every name that actually reaches a reader.
	read := map[string]bool{}
	nameOf := func(expr ast.Expr) (string, bool) {
		if text, ok := stringLiteral(expr); ok && envVarName.FindString(text) == text {
			return text, true
		}
		if id, ok := expr.(*ast.Ident); ok {
			text, ok := constants[id.Name]
			return text, ok
		}
		return "", false
	}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			arg := -1
			if isEnvReader(call) {
				arg = 0
			} else if i, ok := helpers[calledName(call)]; ok {
				arg = i
			}
			if arg >= 0 && arg < len(call.Args) {
				if name, ok := nameOf(call.Args[arg]); ok {
					read[name] = true
				}
			}
			return true
		})
	}

	// Pass 3: every name that text mentions.
	var unread []string
	seen := map[string]bool{}
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			text, ok := stringLiteral(node)
			if !ok {
				return true
			}
			// A bare name is a constant's value or a reader's argument, not
			// prose. "CUB_SCOUT_TUI=1" with no spaces builds a child process's
			// environment. Neither tells anyone to set anything.
			if envVarName.FindString(text) == text || envVarAssignment.MatchString(text) {
				return true
			}
			for _, name := range envVarName.FindAllString(text, -1) {
				if read[name] || envVarsReadOutsideThisRepo[name] || seen[name] {
					continue
				}
				seen[name] = true
				unread = append(unread, name+" (first named at "+fset.Position(node.Pos()).String()+")")
			}
			return true
		})
	}
	sort.Strings(unread)
	return unread
}
