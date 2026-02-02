package gitctx

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestOpenGitRoot_EmptyPath(t *testing.T) {
	ctx := OpenGitRoot("")
	if ctx != nil {
		t.Error("expected nil for empty path (flag not provided)")
	}
}

func TestOpenGitRoot_PathDoesNotExist(t *testing.T) {
	ctx := OpenGitRoot("/nonexistent/path/that/does/not/exist")
	if ctx == nil {
		t.Fatal("expected non-nil context for invalid path")
	}
	if ctx.Valid {
		t.Error("expected Valid=false for nonexistent path")
	}
	if ctx.SkipReason != SkipReasonDoesNotExist {
		t.Errorf("expected skip reason %q, got %q", SkipReasonDoesNotExist, ctx.SkipReason)
	}
}

func TestOpenGitRoot_PathIsFile(t *testing.T) {
	// Create a temp file
	f, err := os.CreateTemp("", "gitctx-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	ctx := OpenGitRoot(f.Name())
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.Valid {
		t.Error("expected Valid=false for file path")
	}
	if ctx.SkipReason != SkipReasonNotDirectory {
		t.Errorf("expected skip reason %q, got %q", SkipReasonNotDirectory, ctx.SkipReason)
	}
}

func TestOpenGitRoot_NotGitRepo(t *testing.T) {
	// Create a temp directory without .git
	dir, err := os.MkdirTemp("", "gitctx-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	ctx := OpenGitRoot(dir)
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if ctx.Valid {
		t.Error("expected Valid=false for non-git directory")
	}
	if ctx.SkipReason != SkipReasonNotGitRepo {
		t.Errorf("expected skip reason %q, got %q", SkipReasonNotGitRepo, ctx.SkipReason)
	}
}

func TestOpenGitRoot_ValidGitRepo(t *testing.T) {
	// Create a temp directory with .git
	dir, err := os.MkdirTemp("", "gitctx-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Create .git directory
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := OpenGitRoot(dir)
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if !ctx.Valid {
		t.Errorf("expected Valid=true for git directory, got skip reason: %s", ctx.SkipReason)
	}
	if ctx.Root != dir {
		t.Errorf("expected Root=%q, got %q", dir, ctx.Root)
	}
}

func TestFiles_LexicographicOrdering(t *testing.T) {
	// Create a temp git repo with files
	dir := createTestGitRepo(t, map[string]string{
		"z_file.txt":     "z",
		"a_file.txt":     "a",
		"m_file.txt":     "m",
		"dir/nested.txt": "nested",
		"dir/a_first.go": "first",
	})
	defer os.RemoveAll(dir)

	ctx := OpenGitRoot(dir)
	if ctx == nil || !ctx.Valid {
		t.Fatal("expected valid git context")
	}

	files := ctx.Files()

	// Verify sorted order
	if !sort.StringsAreSorted(files) {
		t.Errorf("files not sorted: %v", files)
	}

	// Verify expected files
	expected := []string{
		"a_file.txt",
		"dir/a_first.go",
		"dir/nested.txt",
		"m_file.txt",
		"z_file.txt",
	}

	if len(files) != len(expected) {
		t.Errorf("expected %d files, got %d: %v", len(expected), len(files), files)
	}

	for i, exp := range expected {
		if i < len(files) && files[i] != exp {
			t.Errorf("file[%d]: expected %q, got %q", i, exp, files[i])
		}
	}
}

func TestFiles_ExcludesGitDirectory(t *testing.T) {
	dir := createTestGitRepo(t, map[string]string{
		"file.txt": "content",
	})
	defer os.RemoveAll(dir)

	// Add some content to .git directory
	gitConfig := filepath.Join(dir, ".git", "config")
	os.WriteFile(gitConfig, []byte("[core]"), 0644)

	ctx := OpenGitRoot(dir)
	files := ctx.Files()

	for _, f := range files {
		if strings.HasPrefix(f, ".git") {
			t.Errorf("expected .git files to be excluded, found: %s", f)
		}
	}
}

func TestFiles_DeterministicCap(t *testing.T) {
	// Create a repo with many files
	fileMap := make(map[string]string)
	for i := 0; i < 100; i++ {
		fileMap[filepath.Join("files", string(rune('a'+i%26))+string(rune('0'+i/26))+".txt")] = "content"
	}

	dir := createTestGitRepo(t, fileMap)
	defer os.RemoveAll(dir)

	ctx := OpenGitRoot(dir)
	ctx.MaxFiles = 10 // Set a low cap for testing

	files := ctx.Files()

	if len(files) > 10 {
		t.Errorf("expected at most 10 files, got %d", len(files))
	}

	// Verify sorted and capped correctly (first N in sorted order)
	if !sort.StringsAreSorted(files) {
		t.Errorf("files not sorted: %v", files)
	}

	// Run again - should return same cached result
	files2 := ctx.Files()
	if len(files) != len(files2) {
		t.Errorf("cached result differs: %d vs %d", len(files), len(files2))
	}
}

func TestFiles_SkipsSubmodules(t *testing.T) {
	dir := createTestGitRepo(t, map[string]string{
		"main.txt": "main",
	})
	defer os.RemoveAll(dir)

	// Create a fake submodule (directory with .git)
	submodule := filepath.Join(dir, "submodule")
	if err := os.Mkdir(submodule, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(submodule, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(submodule, "subfile.txt"), []byte("sub"), 0644)

	ctx := OpenGitRoot(dir)
	files := ctx.Files()

	for _, f := range files {
		if strings.HasPrefix(f, "submodule/") {
			t.Errorf("expected submodule files to be skipped, found: %s", f)
		}
	}
}

func TestHasFile(t *testing.T) {
	dir := createTestGitRepo(t, map[string]string{
		"exists.txt":     "exists",
		"dir/nested.txt": "nested",
	})
	defer os.RemoveAll(dir)

	ctx := OpenGitRoot(dir)

	// File that exists
	if !ctx.HasFile("exists.txt") {
		t.Error("expected HasFile to return true for existing file")
	}

	// Nested file that exists
	if !ctx.HasFile("dir/nested.txt") {
		t.Error("expected HasFile to return true for nested file")
	}

	// File that doesn't exist
	if ctx.HasFile("nonexistent.txt") {
		t.Error("expected HasFile to return false for nonexistent file")
	}

	// Path traversal attempt
	if ctx.HasFile("../../../etc/passwd") {
		t.Error("expected HasFile to return false for path traversal")
	}
}

func TestReadFile(t *testing.T) {
	dir := createTestGitRepo(t, map[string]string{
		"test.txt": "hello world",
	})
	defer os.RemoveAll(dir)

	ctx := OpenGitRoot(dir)

	content := ctx.ReadFile("test.txt")
	if string(content) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(content))
	}

	// Nonexistent file
	content = ctx.ReadFile("nonexistent.txt")
	if content != nil {
		t.Errorf("expected nil for nonexistent file, got %q", string(content))
	}

	// Path traversal attempt
	content = ctx.ReadFile("../../../etc/passwd")
	if content != nil {
		t.Error("expected nil for path traversal attempt")
	}
}

func TestNilContext(t *testing.T) {
	var ctx *GitContext

	// All methods should handle nil gracefully
	files := ctx.Files()
	if files != nil {
		t.Error("expected nil files for nil context")
	}

	if ctx.HasFile("test.txt") {
		t.Error("expected HasFile to return false for nil context")
	}

	if ctx.ReadFile("test.txt") != nil {
		t.Error("expected ReadFile to return nil for nil context")
	}
}

func TestInvalidContext(t *testing.T) {
	ctx := &GitContext{Valid: false, SkipReason: SkipReasonNotGitRepo}

	files := ctx.Files()
	if files != nil {
		t.Error("expected nil files for invalid context")
	}

	if ctx.HasFile("test.txt") {
		t.Error("expected HasFile to return false for invalid context")
	}

	if ctx.ReadFile("test.txt") != nil {
		t.Error("expected ReadFile to return nil for invalid context")
	}
}

// createTestGitRepo creates a temporary git repo with the given files.
// Returns the directory path. Caller must clean up with os.RemoveAll.
func createTestGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "gitctx-test-*")
	if err != nil {
		t.Fatal(err)
	}

	// Create .git directory
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0755); err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}

	// Create files
	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			os.RemoveAll(dir)
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			os.RemoveAll(dir)
			t.Fatal(err)
		}
	}

	return dir
}
