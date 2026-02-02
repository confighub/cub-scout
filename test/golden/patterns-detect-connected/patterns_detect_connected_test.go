// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package patterns_detect_connected_test provides integration tests for v0.11 connected mode.
//
// These tests verify that:
// 1. --git-url + --git-ref produces equivalent output to --git-root
// 2. HTTP failure scenarios produce correct skip_reason strings
// 3. Output is deterministic and matches goldens
package patterns_detect_connected_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/test/golden"
)

const (
	testFixtureDir = "../../../testdata/connected-repo"
	testGraphJSON  = "../../../testdata/connected-repo/graph.json"
	testTime       = "2026-01-01T00:00:00Z"
)

// TestConnectedModeEquivalence verifies that connected mode produces the same
// output as local mode when given the same repository content.
func TestConnectedModeEquivalence(t *testing.T) {
	// Build the binary
	binary := buildBinary(t)

	// Create deterministic tarball from fixture
	tarballData := createDeterministicTarball(t, testFixtureDir)

	// Start test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tarball endpoint
		if strings.Contains(r.URL.Path, "/tarball/") {
			w.Header().Set("Content-Type", "application/gzip")
			w.WriteHeader(http.StatusOK)
			w.Write(tarballData)
			return
		}
		// Repo existence check (for 404 disambiguation)
		if r.Method == "HEAD" && strings.HasPrefix(r.URL.Path, "/repos/") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Get absolute path to fixture
	absFixture, err := filepath.Abs(testFixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	absGraph, err := filepath.Abs(testGraphJSON)
	if err != nil {
		t.Fatal(err)
	}

	// Run with --git-root (local mode)
	localOutput := runPatterns(t, binary, []string{
		"patterns", "detect",
	}, map[string]string{
		"CUB_SCOUT_TEST_GRAPH_JSON":   absGraph,
		"CUB_SCOUT_TEST_TIME":         testTime,
		"CUB_SCOUT_TEST_CLUSTER":      "test-cluster",
	}, "--git-root", absFixture)

	// Run with --git-url + --git-ref (connected mode)
	connectedOutput := runPatterns(t, binary, []string{
		"patterns", "detect",
		"--git-url", "https://github.com/test/repo",
		"--git-ref", "abc123def456",
	}, map[string]string{
		"CUB_SCOUT_TEST_GRAPH_JSON":   absGraph,
		"CUB_SCOUT_TEST_TIME":         testTime,
		"CUB_SCOUT_TEST_CLUSTER":      "test-cluster",
		"CUB_SCOUT_GITHUB_API_BASE":   server.URL,
	})

	// Outputs should be equivalent
	if localOutput != connectedOutput {
		t.Errorf("Connected mode output differs from local mode:\n--- Local ---\n%s\n--- Connected ---\n%s", localOutput, connectedOutput)
	}

	// Update golden
	golden.AssertGolden(t, "patterns-detect-connected/detect-connected-equivalence.golden.txt", localOutput)

	// Also test JSON output equivalence
	localJSON := runPatterns(t, binary, []string{
		"patterns", "detect", "--json",
	}, map[string]string{
		"CUB_SCOUT_TEST_GRAPH_JSON":   absGraph,
		"CUB_SCOUT_TEST_TIME":         testTime,
		"CUB_SCOUT_TEST_CLUSTER":      "test-cluster",
	}, "--git-root", absFixture)

	connectedJSON := runPatterns(t, binary, []string{
		"patterns", "detect", "--json",
		"--git-url", "https://github.com/test/repo",
		"--git-ref", "abc123def456",
	}, map[string]string{
		"CUB_SCOUT_TEST_GRAPH_JSON":   absGraph,
		"CUB_SCOUT_TEST_TIME":         testTime,
		"CUB_SCOUT_TEST_CLUSTER":      "test-cluster",
		"CUB_SCOUT_GITHUB_API_BASE":   server.URL,
	})

	if localJSON != connectedJSON {
		t.Errorf("Connected mode JSON output differs from local mode:\n--- Local ---\n%s\n--- Connected ---\n%s", localJSON, connectedJSON)
	}

	golden.AssertGolden(t, "patterns-detect-connected/detect-connected-equivalence.golden.json", localJSON)
}

// TestConnectedModeFailures verifies that HTTP failures produce correct skip_reason strings.
func TestConnectedModeFailures(t *testing.T) {
	binary := buildBinary(t)

	absGraph, err := filepath.Abs(testGraphJSON)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		tarballStatus  int
		repoStatus     int // For 404 disambiguation
		goldenFile     string
		wantSkipReason string
	}{
		{
			name:           "repository not found",
			tarballStatus:  http.StatusNotFound,
			repoStatus:     http.StatusNotFound,
			goldenFile:     "detect-connected-repo-not-found.golden.txt",
			wantSkipReason: "git_source repository not found",
		},
		{
			name:           "ref not found",
			tarballStatus:  http.StatusNotFound,
			repoStatus:     http.StatusOK, // Repo exists, ref doesn't
			goldenFile:     "detect-connected-ref-not-found.golden.txt",
			wantSkipReason: "git_source ref not found",
		},
		{
			name:           "authentication required",
			tarballStatus:  http.StatusUnauthorized,
			repoStatus:     http.StatusUnauthorized,
			goldenFile:     "detect-connected-auth-required.golden.txt",
			wantSkipReason: "git_source authentication required",
		},
		{
			name:           "rate limited",
			tarballStatus:  http.StatusTooManyRequests,
			repoStatus:     http.StatusTooManyRequests,
			goldenFile:     "detect-connected-rate-limited.golden.txt",
			wantSkipReason: "git_source rate limited",
		},
		{
			name:           "fetch failed",
			tarballStatus:  http.StatusInternalServerError,
			repoStatus:     http.StatusInternalServerError,
			goldenFile:     "detect-connected-fetch-failed.golden.txt",
			wantSkipReason: "git_source fetch failed",
		},
		{
			name:           "tarball invalid",
			tarballStatus:  http.StatusOK, // Returns 200 but invalid content
			repoStatus:     http.StatusOK,
			goldenFile:     "detect-connected-tarball-invalid.golden.txt",
			wantSkipReason: "git_source tarball invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Tarball endpoint
				if strings.Contains(r.URL.Path, "/tarball/") {
					if tt.name == "tarball invalid" {
						// Return invalid gzip
						w.Header().Set("Content-Type", "application/gzip")
						w.WriteHeader(http.StatusOK)
						w.Write([]byte("not a valid tarball"))
						return
					}
					w.WriteHeader(tt.tarballStatus)
					return
				}
				// Repo existence check
				if r.Method == "HEAD" && strings.HasPrefix(r.URL.Path, "/repos/") && !strings.Contains(r.URL.Path, "/tarball/") {
					w.WriteHeader(tt.repoStatus)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			output := runPatterns(t, binary, []string{
				"patterns", "detect",
				"--git-url", "https://github.com/test/repo",
				"--git-ref", "abc123",
			}, map[string]string{
				"CUB_SCOUT_TEST_GRAPH_JSON":   absGraph,
				"CUB_SCOUT_TEST_TIME":         testTime,
				"CUB_SCOUT_TEST_CLUSTER":      "test-cluster",
				"CUB_SCOUT_GITHUB_API_BASE":   server.URL,
			})

			// Verify skip_reason appears in output
			if !strings.Contains(output, tt.wantSkipReason) {
				t.Errorf("Output should contain skip_reason %q:\n%s", tt.wantSkipReason, output)
			}

			golden.AssertGolden(t, "patterns-detect-connected/"+tt.goldenFile, output)
		})
	}
}

// buildBinary compiles the cub-scout binary for testing.
func buildBinary(t *testing.T) string {
	t.Helper()

	// Use existing binary if CUB_SCOUT_BINARY is set
	if binary := os.Getenv("CUB_SCOUT_BINARY"); binary != "" {
		return binary
	}

	// Find project root (directory with go.mod)
	projectRoot := findProjectRoot(t)

	// Build to temp location
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "cub-scout")

	cmd := exec.Command("go", "build", "-o", binary, "./cmd/cub-scout")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build binary: %v\n%s", err, out)
	}

	return binary
}

// findProjectRoot walks up the directory tree to find go.mod.
func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root (no go.mod)")
		}
		dir = parent
	}
}

// runPatterns executes the patterns command and returns stdout.
func runPatterns(t *testing.T, binary string, args []string, env map[string]string, extraArgs ...string) string {
	t.Helper()

	fullArgs := append(args, extraArgs...)
	cmd := exec.Command(binary, fullArgs...)

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	// Exit code 4 (pattern failures) is acceptable
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 4 {
			// Pattern failures are expected in some tests
		} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			t.Logf("Command stderr: %s", stderr.String())
		}
	}

	return stdout.String()
}

// createDeterministicTarball creates a gzipped tarball from a directory.
// Files are sorted and timestamps are fixed for determinism.
func createDeterministicTarball(t *testing.T, dir string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Fixed timestamp
	modTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Collect all files
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		// Skip .git directory in tarball (GitHub doesn't include it)
		if strings.HasPrefix(relPath, ".git") {
			return nil
		}
		files = append(files, relPath)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Sort for determinism
	sort.Strings(files)

	// GitHub tarballs have a top-level directory
	topDir := "test-repo-abc123"

	// Add top-level directory
	tw.WriteHeader(&tar.Header{
		Name:     topDir + "/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
		ModTime:  modTime,
	})

	// Add files
	for _, relPath := range files {
		fullPath := filepath.Join(dir, relPath)
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatal(err)
		}

		header := &tar.Header{
			Name:    topDir + "/" + relPath,
			Mode:    0644,
			ModTime: modTime,
		}

		if info.IsDir() {
			header.Typeflag = tar.TypeDir
			header.Mode = 0755
			header.Name += "/"
		} else {
			header.Typeflag = tar.TypeReg
			header.Size = info.Size()
		}

		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}

		if !info.IsDir() {
			data, err := os.ReadFile(fullPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write(data); err != nil {
				t.Fatal(err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// readTarball is a debug helper to inspect tarball contents.
func readTarball(data []byte) ([]string, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		names = append(names, hdr.Name)
	}
	return names, nil
}
