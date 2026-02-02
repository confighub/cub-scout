// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package gitsource

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "standard https URL",
			url:       "https://github.com/confighub/cub-scout",
			wantOwner: "confighub",
			wantRepo:  "cub-scout",
		},
		{
			name:      "https URL with .git suffix",
			url:       "https://github.com/confighub/cub-scout.git",
			wantOwner: "confighub",
			wantRepo:  "cub-scout",
		},
		{
			name:      "https URL with trailing slash",
			url:       "https://github.com/confighub/cub-scout/",
			wantOwner: "confighub",
			wantRepo:  "cub-scout",
		},
		{
			name:      "short URL without scheme",
			url:       "github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "http URL",
			url:       "http://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:    "invalid URL - not GitHub",
			url:     "https://gitlab.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "invalid URL - missing repo",
			url:     "https://github.com/owner",
			wantErr: true,
		},
		{
			name:    "invalid URL - empty",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid URL - too many parts",
			url:     "https://github.com/owner/repo/extra/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := ParseGitHubURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGitHubURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if owner != tt.wantOwner {
					t.Errorf("ParseGitHubURL() owner = %v, want %v", owner, tt.wantOwner)
				}
				if repo != tt.wantRepo {
					t.Errorf("ParseGitHubURL() repo = %v, want %v", repo, tt.wantRepo)
				}
			}
		})
	}
}

func TestIsValidSubpath(t *testing.T) {
	tests := []struct {
		name    string
		subpath string
		want    bool
	}{
		{
			name:    "empty subpath is valid",
			subpath: "",
			want:    true,
		},
		{
			name:    "simple path",
			subpath: "clusters/prod",
			want:    true,
		},
		{
			name:    "single component",
			subpath: "apps",
			want:    true,
		},
		{
			name:    "deep path",
			subpath: "a/b/c/d/e",
			want:    true,
		},
		{
			name:    "path with dots in name",
			subpath: "config.d/settings",
			want:    true,
		},
		{
			name:    "parent traversal - simple",
			subpath: "..",
			want:    false,
		},
		{
			name:    "parent traversal - prefix",
			subpath: "../etc/passwd",
			want:    false,
		},
		{
			name:    "parent traversal - middle",
			subpath: "apps/../../../etc",
			want:    false,
		},
		{
			name:    "parent traversal - end",
			subpath: "apps/prod/..",
			want:    true, // Clean path is "apps"
		},
		{
			name:    "absolute path",
			subpath: "/etc/passwd",
			want:    false,
		},
		{
			name:    "path with current dir",
			subpath: "./apps",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidSubpath(tt.subpath); got != tt.want {
				t.Errorf("IsValidSubpath(%q) = %v, want %v", tt.subpath, got, tt.want)
			}
		})
	}
}

func TestMapHTTPStatus(t *testing.T) {
	// Note: 404 is handled separately by resolve404Reason, not mapHTTPStatus
	tests := []struct {
		name   string
		status int
		want   string
	}{
		{
			name:   "200 OK",
			status: http.StatusOK,
			want:   "",
		},
		{
			name:   "401 Unauthorized",
			status: http.StatusUnauthorized,
			want:   SkipReasonAuthRequired,
		},
		{
			name:   "403 Forbidden",
			status: http.StatusForbidden,
			want:   SkipReasonAuthRequired,
		},
		{
			name:   "429 Too Many Requests",
			status: http.StatusTooManyRequests,
			want:   SkipReasonRateLimited,
		},
		{
			name:   "500 Internal Server Error",
			status: http.StatusInternalServerError,
			want:   SkipReasonFetchFailed,
		},
		{
			name:   "502 Bad Gateway",
			status: http.StatusBadGateway,
			want:   SkipReasonFetchFailed,
		},
		{
			name:   "503 Service Unavailable",
			status: http.StatusServiceUnavailable,
			want:   SkipReasonFetchFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapHTTPStatus(tt.status); got != tt.want {
				t.Errorf("MapHTTPStatus(%d) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

// createTestTarball creates a gzipped tarball with the given files.
func createTestTarball(t *testing.T, topDir string, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	// Add top-level directory
	err := tw.WriteHeader(&tar.Header{
		Name:     topDir + "/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Add files
	for path, content := range files {
		fullPath := topDir + "/" + path

		// Add parent directories
		dir := filepath.Dir(fullPath)
		if dir != topDir {
			dirs := []string{}
			for d := dir; d != topDir && d != "."; d = filepath.Dir(d) {
				dirs = append([]string{d}, dirs...)
			}
			for _, d := range dirs {
				tw.WriteHeader(&tar.Header{
					Name:     d + "/",
					Mode:     0755,
					Typeflag: tar.TypeDir,
				})
			}
		}

		// Add file
		err := tw.WriteHeader(&tar.Header{
			Name:     fullPath,
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
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

func TestMaterialize_HTTPStatusMapping(t *testing.T) {
	// Note: 404 is tested separately in TestMaterialize_404Disambiguation
	tests := []struct {
		name           string
		statusCode     int
		wantSkipReason string
	}{
		{
			name:           "401 - authentication required",
			statusCode:     http.StatusUnauthorized,
			wantSkipReason: SkipReasonAuthRequired,
		},
		{
			name:           "403 - forbidden",
			statusCode:     http.StatusForbidden,
			wantSkipReason: SkipReasonAuthRequired,
		},
		{
			name:           "429 - rate limited",
			statusCode:     http.StatusTooManyRequests,
			wantSkipReason: SkipReasonRateLimited,
		},
		{
			name:           "500 - server error",
			statusCode:     http.StatusInternalServerError,
			wantSkipReason: SkipReasonFetchFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			// Create a client that redirects to our test server
			client := &http.Client{
				Transport: &rewriteTransport{
					base:   http.DefaultTransport,
					target: server.URL,
				},
			}

			result := Materialize(Options{
				URL:        "https://github.com/test/repo",
				Ref:        "abc123",
				HTTPClient: client,
			})

			if result.SkipReason != tt.wantSkipReason {
				t.Errorf("got skip_reason %q, want %q", result.SkipReason, tt.wantSkipReason)
			}
			if result.Path != "" {
				t.Error("expected empty path for failed request")
			}
		})
	}
}

func TestMaterialize_404Disambiguation(t *testing.T) {
	tests := []struct {
		name           string
		repoExists     bool
		wantSkipReason string
	}{
		{
			name:           "repo does not exist - repository not found",
			repoExists:     false,
			wantSkipReason: SkipReasonRepoNotFound,
		},
		{
			name:           "repo exists but ref not found - ref not found",
			repoExists:     true,
			wantSkipReason: SkipReasonRefNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Tarball endpoint always returns 404
				if strings.Contains(r.URL.Path, "/tarball/") {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				// Repo existence check
				if strings.HasPrefix(r.URL.Path, "/repos/") && !strings.Contains(r.URL.Path, "/tarball/") {
					if tt.repoExists {
						w.WriteHeader(http.StatusOK)
					} else {
						w.WriteHeader(http.StatusNotFound)
					}
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			client := &http.Client{
				Transport: &rewriteTransport{
					base:   http.DefaultTransport,
					target: server.URL,
				},
			}

			result := Materialize(Options{
				URL:        "https://github.com/test/repo",
				Ref:        "nonexistent-ref",
				HTTPClient: client,
			})

			if result.SkipReason != tt.wantSkipReason {
				t.Errorf("got skip_reason %q, want %q", result.SkipReason, tt.wantSkipReason)
			}
		})
	}
}

func TestMaterialize_Success(t *testing.T) {
	tarball := createTestTarball(t, "owner-repo-abc123", map[string]string{
		"README.md":            "# Test",
		"apps/frontend.yaml":   "apiVersion: v1",
		"clusters/prod/app.yaml": "apiVersion: v1",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(tarball)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: server.URL,
		},
	}

	result := Materialize(Options{
		URL:        "https://github.com/owner/repo",
		Ref:        "abc123",
		HTTPClient: client,
	})

	if result.SkipReason != "" {
		t.Fatalf("unexpected skip_reason: %s", result.SkipReason)
	}
	if result.Path == "" {
		t.Fatal("expected non-empty path")
	}
	if result.Cleanup == nil {
		t.Fatal("expected cleanup function")
	}
	defer result.Cleanup()

	// Verify extracted files
	checkFile := func(relPath, wantContent string) {
		content, err := os.ReadFile(filepath.Join(result.Path, relPath))
		if err != nil {
			t.Errorf("failed to read %s: %v", relPath, err)
			return
		}
		if string(content) != wantContent {
			t.Errorf("%s content = %q, want %q", relPath, string(content), wantContent)
		}
	}

	checkFile("README.md", "# Test")
	checkFile("apps/frontend.yaml", "apiVersion: v1")
	checkFile("clusters/prod/app.yaml", "apiVersion: v1")

	// Verify .git directory was created (for gitctx compatibility)
	if _, err := os.Stat(filepath.Join(result.Path, ".git")); os.IsNotExist(err) {
		t.Error(".git directory was not created")
	}
}

func TestMaterialize_WithSubpath(t *testing.T) {
	tarball := createTestTarball(t, "owner-repo-abc123", map[string]string{
		"README.md":              "# Root",
		"clusters/prod/app.yaml": "prod-config",
		"clusters/dev/app.yaml":  "dev-config",
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(tarball)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: server.URL,
		},
	}

	result := Materialize(Options{
		URL:        "https://github.com/owner/repo",
		Ref:        "abc123",
		Subpath:    "clusters/prod",
		HTTPClient: client,
	})

	if result.SkipReason != "" {
		t.Fatalf("unexpected skip_reason: %s", result.SkipReason)
	}
	defer result.Cleanup()

	// Verify we're in the subpath
	content, err := os.ReadFile(filepath.Join(result.Path, "app.yaml"))
	if err != nil {
		t.Fatalf("failed to read app.yaml: %v", err)
	}
	if string(content) != "prod-config" {
		t.Errorf("got content %q, want %q", string(content), "prod-config")
	}

	// Verify README.md is NOT directly accessible (we're in subpath)
	if _, err := os.Stat(filepath.Join(result.Path, "README.md")); !os.IsNotExist(err) {
		t.Error("README.md should not be accessible from subpath root")
	}
}

func TestMaterialize_InvalidSubpath(t *testing.T) {
	tests := []struct {
		name    string
		subpath string
	}{
		{"parent traversal", "../etc"},
		{"absolute path", "/etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Materialize(Options{
				URL:     "https://github.com/owner/repo",
				Ref:     "abc123",
				Subpath: tt.subpath,
			})

			if result.SkipReason != SkipReasonTarballInvalid {
				t.Errorf("got skip_reason %q, want %q", result.SkipReason, SkipReasonTarballInvalid)
			}
		})
	}
}

func TestMaterialize_InvalidURL(t *testing.T) {
	result := Materialize(Options{
		URL: "https://gitlab.com/owner/repo",
		Ref: "abc123",
	})

	if result.SkipReason != SkipReasonRepoNotFound {
		t.Errorf("got skip_reason %q, want %q", result.SkipReason, SkipReasonRepoNotFound)
	}
}

func TestMaterialize_InvalidTarball(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not a valid tarball"))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: server.URL,
		},
	}

	result := Materialize(Options{
		URL:        "https://github.com/owner/repo",
		Ref:        "abc123",
		HTTPClient: client,
	})

	if result.SkipReason != SkipReasonTarballInvalid {
		t.Errorf("got skip_reason %q, want %q", result.SkipReason, SkipReasonTarballInvalid)
	}
}

func TestMaterialize_AuthHeader(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &rewriteTransport{
			base:   http.DefaultTransport,
			target: server.URL,
		},
	}

	// Without token
	Materialize(Options{
		URL:        "https://github.com/owner/repo",
		Ref:        "abc123",
		HTTPClient: client,
	})
	if capturedAuth != "" {
		t.Errorf("expected no auth header without token, got %q", capturedAuth)
	}

	// With token
	Materialize(Options{
		URL:        "https://github.com/owner/repo",
		Ref:        "abc123",
		Token:      "ghp_testtoken",
		HTTPClient: client,
	})
	if capturedAuth != "Bearer ghp_testtoken" {
		t.Errorf("expected auth header 'Bearer ghp_testtoken', got %q", capturedAuth)
	}
}

// rewriteTransport rewrites all requests to a target server for testing.
type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = t.target[7:] // strip "http://"
	return t.base.RoundTrip(req)
}
