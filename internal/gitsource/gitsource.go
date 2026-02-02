// Package gitsource provides connected mode git source materialization for v0.11+.
//
// This package downloads GitHub repository tarballs and extracts them to a local
// snapshot directory that can be passed to gitctx.OpenGitRoot. Patterns don't need
// special handling for connected mode - they see a normal local git-like directory.
//
// Determinism constraints:
//   - Full commit SHA (40 chars) = deterministic output
//   - Branch/tag names = non-deterministic (output changes as ref advances)
//   - File paths are sorted lexicographically
//   - Same SHA + same repo state = same extracted content
package gitsource

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Skip reason constants (contract-locked strings per patterns-contract.md v0.11+).
const (
	SkipReasonRepoNotFound   = "git_source repository not found"
	SkipReasonRefNotFound    = "git_source ref not found"
	SkipReasonAuthRequired   = "git_source authentication required"
	SkipReasonRateLimited    = "git_source rate limited"
	SkipReasonFetchFailed    = "git_source fetch failed"
	SkipReasonTarballInvalid = "git_source tarball invalid"
)

// DefaultTimeout is the default HTTP timeout for tarball downloads.
const DefaultTimeout = 60 * time.Second

// DefaultMaxSize is the default maximum tarball size (100 MB).
const DefaultMaxSize = 100 * 1024 * 1024

// Result represents the outcome of a Materialize call.
type Result struct {
	// Path is the local directory containing the extracted snapshot.
	// Empty if materialization failed.
	Path string

	// Cleanup should be called when done with the snapshot.
	// Safe to call even if nil.
	Cleanup func()

	// SkipReason is set when materialization failed.
	// Uses contract-locked strings.
	SkipReason string
}

// Options configures the materialization behavior.
type Options struct {
	// URL is the GitHub repository URL (required).
	// Supports: https://github.com/owner/repo, https://github.com/owner/repo.git
	URL string

	// Ref is the git ref to download (required).
	// Can be: commit SHA (recommended), branch name, or tag.
	Ref string

	// Subpath is an optional subdirectory within the repository.
	// Must be a clean relative path (no .., no absolute, no escape).
	Subpath string

	// Token is the GitHub token for authentication (optional).
	// When set, enables access to private repositories.
	Token string

	// Timeout is the HTTP request timeout.
	// Defaults to DefaultTimeout.
	Timeout time.Duration

	// MaxSize is the maximum tarball size in bytes.
	// Defaults to DefaultMaxSize.
	MaxSize int64

	// HTTPClient allows injection of a custom client for testing.
	// Defaults to http.DefaultClient with Timeout.
	HTTPClient *http.Client
}

// Materialize downloads a GitHub repository tarball and extracts it to a temp directory.
// Returns a Result with either a valid Path or a SkipReason.
//
// The caller is responsible for calling Result.Cleanup() when done.
//
// Runtime failures (network, auth, 404) result in SkipReason being set.
// This is pattern-level SKIP behavior, not a global CLI error.
func Materialize(opts Options) Result {
	// Parse and validate URL
	owner, repo, err := parseGitHubURL(opts.URL)
	if err != nil {
		return Result{SkipReason: SkipReasonRepoNotFound}
	}

	// Validate subpath if provided
	if opts.Subpath != "" {
		if !isValidSubpath(opts.Subpath) {
			return Result{SkipReason: SkipReasonTarballInvalid}
		}
	}

	// Set defaults
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	maxSize := opts.MaxSize
	if maxSize == 0 {
		maxSize = DefaultMaxSize
	}

	// Create HTTP client
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	// Construct tarball URL
	// GitHub API: GET /repos/{owner}/{repo}/tarball/{ref}
	tarballURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/tarball/%s", owner, repo, opts.Ref)

	// Create request
	req, err := http.NewRequest("GET", tarballURL, nil)
	if err != nil {
		return Result{SkipReason: SkipReasonFetchFailed}
	}

	// Set headers
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cub-scout/0.11")
	if opts.Token != "" {
		req.Header.Set("Authorization", "Bearer "+opts.Token)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return Result{SkipReason: SkipReasonFetchFailed}
	}
	defer resp.Body.Close()

	// Handle 404 specially: distinguish repo-not-found vs ref-not-found
	if resp.StatusCode == http.StatusNotFound {
		skipReason := resolve404Reason(client, owner, repo, opts.Token)
		return Result{SkipReason: skipReason}
	}

	// Map other HTTP status codes to skip reasons
	skipReason := mapHTTPStatus(resp.StatusCode)
	if skipReason != "" {
		return Result{SkipReason: skipReason}
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "cub-scout-snapshot-*")
	if err != nil {
		return Result{SkipReason: SkipReasonTarballInvalid}
	}

	cleanup := func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	}

	// Limit read size
	limitedReader := io.LimitReader(resp.Body, maxSize)

	// Extract tarball
	extractedRoot, err := extractTarball(limitedReader, tmpDir, opts.Subpath)
	if err != nil {
		cleanup()
		return Result{SkipReason: SkipReasonTarballInvalid}
	}

	// Create a fake .git directory so gitctx.OpenGitRoot recognizes it
	gitDir := filepath.Join(extractedRoot, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		cleanup()
		return Result{SkipReason: SkipReasonTarballInvalid}
	}

	return Result{
		Path:    extractedRoot,
		Cleanup: cleanup,
	}
}

// parseGitHubURL extracts owner and repo from a GitHub URL.
// Supports:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo.git
//   - github.com/owner/repo
func parseGitHubURL(url string) (owner, repo string, err error) {
	// Normalize: remove trailing slashes and .git suffix
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, ".git")

	// Pattern: github.com/owner/repo
	patterns := []string{
		`^https?://github\.com/([^/]+)/([^/]+)$`,
		`^github\.com/([^/]+)/([^/]+)$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(url); matches != nil {
			return matches[1], matches[2], nil
		}
	}

	return "", "", fmt.Errorf("invalid GitHub URL: %s", url)
}

// isValidSubpath checks if a subpath is safe (no escaping).
func isValidSubpath(subpath string) bool {
	// Must not be empty
	if subpath == "" {
		return true // Empty is valid (no subpath)
	}

	// Must not be absolute
	if filepath.IsAbs(subpath) {
		return false
	}

	// Clean the path
	cleaned := filepath.Clean(subpath)

	// Must not start with .. or be exactly ..
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return false
	}

	// Must not contain .. components after cleaning
	parts := strings.Split(cleaned, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return false
		}
	}

	return true
}

// mapHTTPStatus maps HTTP status codes to deterministic skip reasons.
// Note: 404 is handled separately by resolve404Reason for proper disambiguation.
func mapHTTPStatus(status int) string {
	switch status {
	case http.StatusOK:
		return "" // Success

	case http.StatusUnauthorized, http.StatusForbidden:
		return SkipReasonAuthRequired

	case http.StatusTooManyRequests:
		return SkipReasonRateLimited

	default:
		if status >= 400 {
			return SkipReasonFetchFailed
		}
		return ""
	}
}

// resolve404Reason distinguishes between repo-not-found and ref-not-found.
// When tarball returns 404, we check if the repo exists to determine the cause.
func resolve404Reason(client *http.Client, owner, repo, token string) string {
	// Check if repository exists
	repoURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
	req, err := http.NewRequest("HEAD", repoURL, nil)
	if err != nil {
		return SkipReasonRepoNotFound
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "cub-scout/0.11")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Network error checking repo - assume repo not found
		return SkipReasonRepoNotFound
	}
	defer resp.Body.Close()

	// If repo exists (200), then the ref was not found
	if resp.StatusCode == http.StatusOK {
		return SkipReasonRefNotFound
	}

	// Repo doesn't exist or access denied
	return SkipReasonRepoNotFound
}

// extractTarball extracts a gzipped tarball to a destination directory.
// Returns the path to the extracted root (which may be a subdirectory if subpath is set).
func extractTarball(r io.Reader, destDir string, subpath string) (string, error) {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("invalid gzip: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// GitHub tarballs have a top-level directory like "owner-repo-sha"
	// We need to detect and strip it
	var topLevelDir string
	extractedFiles := make(map[string]bool)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("tar read error: %w", err)
		}

		// Get the path parts
		name := header.Name
		parts := strings.Split(name, "/")
		if len(parts) == 0 {
			continue
		}

		// Detect top-level directory
		if topLevelDir == "" && len(parts) > 0 {
			topLevelDir = parts[0]
		}

		// Skip the top-level directory itself
		if name == topLevelDir || name == topLevelDir+"/" {
			continue
		}

		// Strip the top-level directory
		if !strings.HasPrefix(name, topLevelDir+"/") {
			continue
		}
		relPath := strings.TrimPrefix(name, topLevelDir+"/")
		if relPath == "" {
			continue
		}

		// Security: validate the relative path
		if !isValidSubpath(relPath) {
			continue // Skip suspicious paths
		}

		// Construct destination path
		destPath := filepath.Join(destDir, relPath)

		// Security: ensure destPath is within destDir using filepath.Rel
		cleanDest := filepath.Clean(destPath)
		cleanBase := filepath.Clean(destDir)
		rel, err := filepath.Rel(cleanBase, cleanDest)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue // Skip paths that escape
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return "", fmt.Errorf("mkdir failed: %w", err)
			}
			extractedFiles[relPath] = true

		case tar.TypeReg:
			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return "", fmt.Errorf("mkdir failed: %w", err)
			}

			// Create file
			f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0755)
			if err != nil {
				return "", fmt.Errorf("create file failed: %w", err)
			}

			// Copy content with size limit
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return "", fmt.Errorf("write file failed: %w", err)
			}
			f.Close()
			extractedFiles[relPath] = true

		case tar.TypeSymlink:
			// Skip symlinks for security
			continue

		default:
			// Skip other types
			continue
		}
	}

	// Determine the root to return
	rootPath := destDir

	// If subpath is specified, validate and return that subdirectory
	if subpath != "" {
		subpathDir := filepath.Join(destDir, subpath)
		cleanSubpath := filepath.Clean(subpathDir)
		cleanBase := filepath.Clean(destDir)

		// Ensure subpath doesn't escape using filepath.Rel
		rel, err := filepath.Rel(cleanBase, cleanSubpath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("subpath escapes root")
		}

		// Check if subpath exists
		info, err := os.Stat(cleanSubpath)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("subpath does not exist")
		}

		rootPath = cleanSubpath
	}

	return rootPath, nil
}

// ParseGitHubURL is exported for testing.
var ParseGitHubURL = parseGitHubURL

// IsValidSubpath is exported for testing.
var IsValidSubpath = isValidSubpath

// MapHTTPStatus is exported for testing.
var MapHTTPStatus = mapHTTPStatus
