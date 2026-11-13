// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// An agent takes a tool's words at face value. These phrases each turned
// "nothing was found" into a claim about the cluster, or sent the reader to
// the wrong fix, on a real cluster on 2026-11-13
// (docs/reference/agentic-gitops-observability.md, "Lessons from real
// clusters"). They may not come back in a string the binary can print.
var phrasesAnAgentWouldBelieve = map[string]string{
	"not confighub-managed":              "absence of one label is not evidence that ConfigHub does not manage the object; say what could not be read",
	"is not managed by gitops":           "no recognised owner marker is not evidence that nothing manages the resource",
	"these lack gitops ownership":        "no recognised owner marker is not evidence that the resources lack an owner",
	"not managed by known owners":        "say that no owner marker was recognised",
	"verify `cub` auth and connectivity": "name the cause that was observed, or point at where it is reported; do not guess a login problem",
}

func TestNoPrintedStringClaimsWhatWasOnlyNotFound(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, dir := range []string{"cmd", "pkg", "internal"} {
		_ = filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				t.Errorf("parse %s: %v", path, perr)
				return nil
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				text, uerr := strconv.Unquote(lit.Value)
				if uerr != nil {
					return true
				}
				lower := strings.ToLower(text)
				for phrase, why := range phrasesAnAgentWouldBelieve {
					if strings.Contains(lower, phrase) {
						t.Errorf("%s: %q contains %q: %s", fset.Position(lit.Pos()), text, phrase, why)
					}
				}
				return true
			})
			return nil
		})
	}
}
