// Package gitctx provides Git repository context for v0.10+ git-aware patterns.
//
// This package implements deterministic repository scanning with the following
// guarantees (per patterns-contract.md):
//
//   - Bounded scan: Only files within the repository root are scanned
//   - No network: No git fetch, clone, or remote operations
//   - No submodule expansion: Submodules are not recursively scanned
//   - Lexicographic ordering: File paths are processed in sorted order (locale-independent)
//   - Deterministic cap: Scanning enforces a maximum file limit
//   - Reproducible: Same repository state = same output
package gitctx

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultMaxFiles is the default maximum number of files to enumerate.
// When exceeded, only the first N paths (in sorted order) are kept.
const DefaultMaxFiles = 10000

// GitContext represents the context from a local Git repository.
// This is internal plumbing; the struct is not part of the public contract.
type GitContext struct {
	// Root is the absolute path to the repository root.
	Root string

	// Valid indicates whether the git-root is usable.
	// If false, SkipReason explains why.
	Valid bool

	// SkipReason is set when Valid is false.
	// Uses deterministic strings per contract.
	SkipReason string

	// MaxFiles is the cap on enumerated files.
	MaxFiles int

	// files is lazily populated on first call to Files().
	files   []string
	scanned bool
}

// Skip reason constants (contract-locked strings).
const (
	SkipReasonNotProvided    = "no git_root provided"
	SkipReasonDoesNotExist   = "git_root path does not exist"
	SkipReasonNotDirectory   = "git_root path is not a directory"
	SkipReasonNotGitRepo     = "git_root path is not a git repository"
	SkipReasonScanError      = "git_root scan error"
)

// OpenGitRoot validates and opens a git repository at the given path.
// Returns a GitContext that is either Valid or has a SkipReason.
//
// This function does not return errors for invalid paths; instead it returns
// a GitContext with Valid=false and an appropriate SkipReason. This supports
// the contract requirement that invalid --git-root causes pattern-level SKIP,
// not global CLI failure.
func OpenGitRoot(path string) *GitContext {
	if path == "" {
		return nil // Flag not provided; patterns receive nil context
	}

	ctx := &GitContext{
		MaxFiles: DefaultMaxFiles,
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		ctx.Valid = false
		ctx.SkipReason = SkipReasonDoesNotExist
		return ctx
	}
	ctx.Root = absPath

	// Check path exists
	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		ctx.Valid = false
		ctx.SkipReason = SkipReasonDoesNotExist
		return ctx
	}
	if err != nil {
		ctx.Valid = false
		ctx.SkipReason = SkipReasonDoesNotExist
		return ctx
	}

	// Check it's a directory
	if !info.IsDir() {
		ctx.Valid = false
		ctx.SkipReason = SkipReasonNotDirectory
		return ctx
	}

	// Check it's a git repository (has .git directory or file)
	gitPath := filepath.Join(absPath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		ctx.Valid = false
		ctx.SkipReason = SkipReasonNotGitRepo
		return ctx
	}

	ctx.Valid = true
	return ctx
}

// Files returns the list of files in the repository, sorted lexicographically.
// Results are cached after first call.
//
// The scan:
//   - Excludes the .git directory
//   - Does not follow symlinks outside the root
//   - Does not expand submodules
//   - Caps at MaxFiles (keeping first N in sorted order)
func (c *GitContext) Files() []string {
	if c == nil || !c.Valid {
		return nil
	}

	if c.scanned {
		return c.files
	}

	c.files = c.scanFiles()
	c.scanned = true
	return c.files
}

// scanFiles performs the deterministic file enumeration.
func (c *GitContext) scanFiles() []string {
	var files []string

	// Walk the directory tree
	_ = filepath.Walk(c.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors, continue walking
		}

		// Get relative path
		relPath, err := filepath.Rel(c.Root, path)
		if err != nil {
			return nil
		}

		// Skip root itself
		if relPath == "." {
			return nil
		}

		// Skip .git directory entirely
		if relPath == ".git" || strings.HasPrefix(relPath, ".git"+string(filepath.Separator)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip submodules (directories containing .git file/dir)
		if info.IsDir() {
			subGit := filepath.Join(path, ".git")
			if _, err := os.Stat(subGit); err == nil {
				// This is a submodule or nested repo; skip it
				return filepath.SkipDir
			}
		}

		// Skip symlinks that point outside root
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				return nil // Skip broken symlinks
			}
			if !strings.HasPrefix(target, c.Root) {
				return nil // Skip symlinks outside root
			}
		}

		// Only include regular files
		if info.Mode().IsRegular() {
			files = append(files, relPath)
		}

		return nil
	})

	// Sort lexicographically (locale-independent)
	sort.Strings(files)

	// Apply cap
	if len(files) > c.MaxFiles {
		files = files[:c.MaxFiles]
	}

	return files
}

// HasFile checks if a file exists in the repository (relative path).
func (c *GitContext) HasFile(relPath string) bool {
	if c == nil || !c.Valid {
		return false
	}

	fullPath := filepath.Join(c.Root, relPath)

	// Security: ensure path doesn't escape root
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, c.Root) {
		return false
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

// ReadFile reads a file from the repository (relative path).
// Returns nil if file doesn't exist or is outside root.
func (c *GitContext) ReadFile(relPath string) []byte {
	if c == nil || !c.Valid {
		return nil
	}

	fullPath := filepath.Join(c.Root, relPath)

	// Security: ensure path doesn't escape root
	cleanPath := filepath.Clean(fullPath)
	if !strings.HasPrefix(cleanPath, c.Root) {
		return nil
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil
	}
	return data
}
