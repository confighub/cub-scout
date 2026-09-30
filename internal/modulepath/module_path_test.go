package modulepath

import (
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

	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == "vendor") {
				return filepath.SkipDir
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
			underPreviousPath := importPath == previousModulePath || strings.HasPrefix(importPath, previousModulePath+"/")
			underV2Path := importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/")
			if underPreviousPath && !underV2Path {
				return fmt.Errorf("%s imports obsolete module path %q", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
