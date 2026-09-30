package modulepath

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/confighub/cub-scout/v2"
const previousModulePath = "github.com/confighub/cub-scout"

func TestModulePathAndSelfImportsUseV2(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(goMod), "module "+modulePath+"\n") {
		t.Fatalf("go.mod must declare module %q", modulePath)
	}

	if err := scanSelfImports(root, modulePath, previousModulePath); err != nil {
		t.Fatal(err)
	}
}

func TestScanSelfImportsFixtureContract(t *testing.T) {
	t.Run("rejects old unsuffixed active import", func(t *testing.T) {
		root := t.TempDir()
		writeGoFile(t, root, "pkg/active.go", `package active
import _ "github.com/confighub/cub-scout/pkg/agent"
`)
		err := scanSelfImports(root, modulePath, previousModulePath)
		if err == nil || !strings.Contains(err.Error(), previousModulePath+"/pkg/agent") {
			t.Fatalf("scanSelfImports error = %v, want obsolete import error", err)
		}
	})

	t.Run("accepts v2 active import", func(t *testing.T) {
		root := t.TempDir()
		writeGoFile(t, root, "pkg/active.go", `package active
import _ "github.com/confighub/cub-scout/v2/pkg/agent"
`)
		if err := scanSelfImports(root, modulePath, previousModulePath); err != nil {
			t.Fatalf("scanSelfImports rejected /v2 import: %v", err)
		}
	})

	t.Run("ignores Go auxiliary trees and nested checkouts/modules", func(t *testing.T) {
		root := t.TempDir()
		writeGoFile(t, root, "pkg/active.go", "package active\n")
		for _, dir := range []string{".hidden", "_generated", "vendor", "testdata"} {
			writeGoFile(t, root, filepath.Join(dir, "legacy.go"), `package ignored
import _ "github.com/confighub/cub-scout/pkg/legacy"
`)
		}
		writeGoFile(t, root, "nested-module/go.mod", "module example.com/nested\n")
		writeGoFile(t, root, "nested-module/pkg/legacy.go", `package nested
import _ "github.com/confighub/cub-scout/pkg/legacy"
`)
		writeGoFile(t, root, "nested-checkout/.git", "gitdir: ../.git/worktrees/child\n")
		writeGoFile(t, root, "nested-checkout/pkg/legacy.go", `package nested
import _ "github.com/confighub/cub-scout/pkg/legacy"
`)
		if err := scanSelfImports(root, modulePath, previousModulePath); err != nil {
			t.Fatalf("scanSelfImports included an ignored or independent subtree: %v", err)
		}
	})

	t.Run("parses inactive-build-tag files in active module", func(t *testing.T) {
		root := t.TempDir()
		writeGoFile(t, root, "inactivepkg/platform_android.go", `//go:build never

package inactivepkg
import (
  _ "github.com/confighub/cub-scout/v2/pkg/agent"
`)
		err := scanSelfImports(root, modulePath, previousModulePath)
		if err == nil || !strings.Contains(err.Error(), "platform_android.go") {
			t.Fatalf("scanSelfImports error = %v, want parse error for inactive source", err)
		}
	})
}

func writeGoFile(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanSelfImports(root, currentModulePath, oldModulePath string) error {
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root {
				skip, skipErr := skipGoTreeDirectory(path, entry.Name())
				if skipErr != nil {
					return skipErr
				}
				if skip {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range file.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			underPreviousPath := importPath == oldModulePath || strings.HasPrefix(importPath, oldModulePath+"/")
			underV2Path := importPath == currentModulePath || strings.HasPrefix(importPath, currentModulePath+"/")
			if underPreviousPath && !underV2Path {
				return fmt.Errorf("%s imports obsolete module path %q", path, importPath)
			}
		}
		return nil
	})
	return err
}

func skipGoTreeDirectory(path, name string) (bool, error) {
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "vendor" || name == "testdata" {
		return true, nil
	}
	for _, marker := range []string{"go.mod", ".git"} {
		_, err := os.Lstat(filepath.Join(path, marker))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false, fmt.Errorf("inspect %s marker in %s: %w", marker, path, err)
		}
	}
	return false, nil
}
