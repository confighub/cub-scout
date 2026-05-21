// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestSplitCanonicalPath(t *testing.T) {
	tests := map[string][]string{
		".spec.replicas":      {"spec", "replicas"},
		"spec.replicas":       {"spec", "replicas"},
		".apiVersion":         {"apiVersion"},
		"":                    nil,
		".":                   nil,
		"  .  ":               nil,
		".metadata.namespace": {"metadata", "namespace"},
		".a..b":               {"a", "b"},
	}
	for in, want := range tests {
		t.Run(in, func(t *testing.T) {
			got := splitCanonicalPath(in)
			if len(got) != len(want) {
				t.Fatalf("splitCanonicalPath(%q) = %v, want %v", in, got, want)
			}
			for i, w := range want {
				if got[i] != w {
					t.Errorf("split[%d] = %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

func TestBackResolveGitSource_RawYAML(t *testing.T) {
	root := t.TempDir()
	// Manifest under apps/prod/api/deployment.yaml.
	yaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
  labels:
    app: api
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: api
        image: ghcr.io/example/api:v1
`
	writeFile(t, root, "apps/prod/api/deployment.yaml", yaml)

	t.Run("locate spec.replicas line", func(t *testing.T) {
		got, ok := BackResolveGitSource(root, "Deployment", "api", "prod", ".spec.replicas")
		if !ok {
			t.Fatal("expected hit")
		}
		if got.File != filepath.Join("apps", "prod", "api", "deployment.yaml") {
			t.Errorf("File = %q", got.File)
		}
		// The value `3` lives on the same line as `replicas:` — line 9 in the YAML above.
		if got.Line != 9 {
			t.Errorf("Line = %d, want 9", got.Line)
		}
	})

	t.Run("locate metadata.namespace", func(t *testing.T) {
		got, ok := BackResolveGitSource(root, "Deployment", "api", "prod", ".metadata.namespace")
		if !ok {
			t.Fatal("expected hit")
		}
		if got.Line != 5 {
			t.Errorf("Line = %d, want 5", got.Line)
		}
	})

	t.Run("missing field returns false", func(t *testing.T) {
		_, ok := BackResolveGitSource(root, "Deployment", "api", "prod", ".spec.notARealField")
		if ok {
			t.Error("expected no hit for missing field")
		}
	})

	t.Run("wrong resource returns false", func(t *testing.T) {
		_, ok := BackResolveGitSource(root, "Deployment", "wrong-name", "prod", ".spec.replicas")
		if ok {
			t.Error("expected no hit for wrong name")
		}
	})

	t.Run("wrong namespace returns false", func(t *testing.T) {
		_, ok := BackResolveGitSource(root, "Deployment", "api", "wrong-ns", ".spec.replicas")
		if ok {
			t.Error("expected no hit for wrong namespace")
		}
	})

	t.Run("blank namespace matches any", func(t *testing.T) {
		_, ok := BackResolveGitSource(root, "Deployment", "api", "", ".spec.replicas")
		if !ok {
			t.Error("blank namespace should match")
		}
	})
}

func TestBackResolveGitSource_MultiDocFile(t *testing.T) {
	root := t.TempDir()
	multiDoc := `apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: prod
spec:
  ports:
  - port: 80
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 5
`
	writeFile(t, root, "manifests.yaml", multiDoc)

	got, ok := BackResolveGitSource(root, "Deployment", "api", "prod", ".spec.replicas")
	if !ok {
		t.Fatal("expected hit on second doc")
	}
	if got.Line != 16 {
		t.Errorf("Line = %d, want 16 (replicas in second doc)", got.Line)
	}
}

func TestBackResolveGitSource_MultiFileSearch(t *testing.T) {
	root := t.TempDir()
	// Decoy: another resource in a different file.
	writeFile(t, root, "decoy.yaml", `apiVersion: v1
kind: ConfigMap
metadata:
  name: api
  namespace: prod
data:
  k: v
`)
	// Actual target.
	writeFile(t, root, "nested/deployment.yaml", `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
spec:
  replicas: 7
`)

	got, ok := BackResolveGitSource(root, "Deployment", "api", "prod", ".spec.replicas")
	if !ok {
		t.Fatal("expected hit in nested file")
	}
	if got.File != filepath.Join("nested", "deployment.yaml") {
		t.Errorf("File = %q", got.File)
	}
}

func TestBackResolveGitSource_InvalidInputs(t *testing.T) {
	t.Run("empty root", func(t *testing.T) {
		if _, ok := BackResolveGitSource("", "Deployment", "api", "prod", ".spec.replicas"); ok {
			t.Error("empty root should return false")
		}
	})

	t.Run("non-directory root", func(t *testing.T) {
		root := t.TempDir()
		file := writeFile(t, root, "not-a-dir.yaml", "foo: bar")
		if _, ok := BackResolveGitSource(file, "Deployment", "api", "prod", ".spec.replicas"); ok {
			t.Error("file path as root should return false")
		}
	})

	t.Run("empty field path", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "d.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n  namespace: y\n")
		if _, ok := BackResolveGitSource(root, "ConfigMap", "x", "y", ""); ok {
			t.Error("empty field path should return false")
		}
	})

	t.Run("no matching docs", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "d.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n  namespace: y\n")
		if _, ok := BackResolveGitSource(root, "Deployment", "x", "y", ".spec.replicas"); ok {
			t.Error("no matching docs should return false")
		}
	})

	t.Run("ignores non-yaml extensions", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "readme.txt", "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n  namespace: y\n")
		if _, ok := BackResolveGitSource(root, "ConfigMap", "x", "y", ".kind"); ok {
			t.Error("non-yaml files should be skipped")
		}
	})
}
