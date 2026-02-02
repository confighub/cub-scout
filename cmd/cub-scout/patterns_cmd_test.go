// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import "testing"

func TestValidateGitFlags(t *testing.T) {
	tests := []struct {
		name       string
		gitRoot    string
		gitURL     string
		gitRef     string
		gitSubpath string
		wantErr    bool
		errMsg     string
	}{
		{
			name:    "no flags - valid",
			wantErr: false,
		},
		{
			name:    "git-root only - valid",
			gitRoot: "/path/to/repo",
			wantErr: false,
		},
		{
			name:    "git-url and git-ref - valid",
			gitURL:  "https://github.com/owner/repo",
			gitRef:  "abc123",
			wantErr: false,
		},
		{
			name:       "git-url, git-ref, git-subpath - valid",
			gitURL:    "https://github.com/owner/repo",
			gitRef:    "abc123",
			gitSubpath: "clusters/prod",
			wantErr:   false,
		},
		{
			name:       "git-root with git-subpath - valid",
			gitRoot:   "/path/to/repo",
			gitSubpath: "clusters/prod",
			wantErr:   false,
		},
		{
			name:    "git-root and git-url - mutual exclusivity error",
			gitRoot: "/path/to/repo",
			gitURL:  "https://github.com/owner/repo",
			gitRef:  "abc123",
			wantErr: true,
			errMsg:  "--git-root and --git-url are mutually exclusive",
		},
		{
			name:    "git-url without git-ref - error",
			gitURL:  "https://github.com/owner/repo",
			wantErr: true,
			errMsg:  "--git-url requires --git-ref",
		},
		{
			name:    "git-ref without git-url - error",
			gitRef:  "abc123",
			wantErr: true,
			errMsg:  "--git-ref requires --git-url",
		},
		{
			name:       "git-subpath without context - error",
			gitSubpath: "clusters/prod",
			wantErr:   true,
			errMsg:    "--git-subpath requires --git-root or --git-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitFlags(tt.gitRoot, tt.gitURL, tt.gitRef, tt.gitSubpath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateGitFlags() expected error, got nil")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("validateGitFlags() error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateGitFlags() unexpected error: %v", err)
				}
			}
		})
	}
}
