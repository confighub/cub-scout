// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMapHelmHookToArgoPhase(t *testing.T) {
	tests := []struct {
		name     string
		helmHook string
		want     string
	}{
		{
			name:     "pre-install maps to PreSync",
			helmHook: "pre-install",
			want:     "PreSync",
		},
		{
			name:     "post-install maps to PostSync",
			helmHook: "post-install",
			want:     "PostSync",
		},
		{
			name:     "pre-upgrade maps to PreSync",
			helmHook: "pre-upgrade",
			want:     "PreSync",
		},
		{
			name:     "post-upgrade maps to PostSync",
			helmHook: "post-upgrade",
			want:     "PostSync",
		},
		{
			name:     "combined hooks",
			helmHook: "post-install,post-upgrade",
			want:     "PostSync",
		},
		{
			name:     "mixed pre and post",
			helmHook: "pre-install,post-install",
			want:     "PostSync,PreSync",
		},
		{
			name:     "test hook maps to PostSync",
			helmHook: "test",
			want:     "PostSync",
		},
		{
			name:     "pre-delete maps to PreSync",
			helmHook: "pre-delete",
			want:     "PreSync",
		},
		{
			name:     "post-delete maps to PostSync",
			helmHook: "post-delete",
			want:     "PostSync",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapHelmHookToArgoPhase(tt.helmHook)
			if got != tt.want {
				t.Errorf("mapHelmHookToArgoPhase(%q) = %q, want %q", tt.helmHook, got, tt.want)
			}
		})
	}
}

func TestMapHooksCommand_FileMode(t *testing.T) {
	// Create a temporary YAML file with hooks
	content := `apiVersion: batch/v1
kind: Job
metadata:
  name: db-migrate
  namespace: prod
  annotations:
    helm.sh/hook: "post-install,post-upgrade"
    helm.sh/hook-weight: "-5"
spec:
  template:
    spec:
      containers:
      - name: migrate
        image: migrate:latest
---
apiVersion: batch/v1
kind: Job
metadata:
  name: db-backup
  namespace: prod
  annotations:
    helm.sh/hook: "post-install"
    helm.sh/hook-delete-policy: "before-hook-creation"
spec:
  template:
    spec:
      containers:
      - name: backup
        image: backup:latest
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 1
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "hooks.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Set the hooksFile variable and reset after test
	oldHooksFile := hooksFile
	oldMapListFormat := mapListFormat
	oldMapNamespace := mapNamespace
	defer func() {
		hooksFile = oldHooksFile
		mapListFormat = oldMapListFormat
		mapNamespace = oldMapNamespace
	}()

	t.Run("extracts hooks from file", func(t *testing.T) {
		hooksFile = tmpFile
		mapListFormat = "json"
		mapNamespace = ""

		// Create a mock command
		cmd := mapHooksCmd
		err := runMapHooks(cmd, nil)
		if err != nil {
			t.Fatalf("runMapHooks failed: %v", err)
		}
	})

	t.Run("filters by namespace", func(t *testing.T) {
		hooksFile = tmpFile
		mapListFormat = "json"
		mapNamespace = "staging" // No hooks in staging

		cmd := mapHooksCmd
		err := runMapHooks(cmd, nil)
		if err != nil {
			t.Fatalf("runMapHooks failed: %v", err)
		}
	})
}

func TestMapHooksCommand_NoHooks(t *testing.T) {
	// Create a temporary YAML file without hooks
	content := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 1
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "no-hooks.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	oldHooksFile := hooksFile
	oldMapListFormat := mapListFormat
	defer func() {
		hooksFile = oldHooksFile
		mapListFormat = oldMapListFormat
	}()

	hooksFile = tmpFile
	mapListFormat = "ascii"

	cmd := mapHooksCmd
	err := runMapHooks(cmd, nil)
	if err != nil {
		t.Fatalf("runMapHooks failed: %v", err)
	}
}

func TestMapHooksCommand_ArgoHooks(t *testing.T) {
	// Create a file with ArgoCD hook annotations
	content := `apiVersion: batch/v1
kind: Job
metadata:
  name: presync-check
  namespace: prod
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/sync-wave: "-1"
spec:
  template:
    spec:
      containers:
      - name: check
        image: check:latest
---
apiVersion: batch/v1
kind: Job
metadata:
  name: postsync-notify
  namespace: prod
  annotations:
    argocd.argoproj.io/hook: PostSync
    argocd.argoproj.io/sync-wave: "1"
spec:
  template:
    spec:
      containers:
      - name: notify
        image: notify:latest
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "argo-hooks.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	oldHooksFile := hooksFile
	oldMapListFormat := mapListFormat
	defer func() {
		hooksFile = oldHooksFile
		mapListFormat = oldMapListFormat
	}()

	hooksFile = tmpFile
	mapListFormat = "json"

	cmd := mapHooksCmd
	err := runMapHooks(cmd, nil)
	if err != nil {
		t.Fatalf("runMapHooks failed: %v", err)
	}
}
