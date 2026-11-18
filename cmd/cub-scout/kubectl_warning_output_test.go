package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeKubectlWithWarning puts a kubectl on PATH that prints what kubectl 1.35
// printed in CI run 44254135155, a deprecation warning on stderr, before its
// real output on stdout.
func fakeKubectlWithWarning(t *testing.T, stdout string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake kubectl")
	}
	dir := t.TempDir()
	stdoutFile := filepath.Join(dir, "stdout")
	if err := os.WriteFile(stdoutFile, []byte(stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho 'Warning: v1 Endpoints is deprecated in v1.33+; use discovery.k8s.io/v1 EndpointSlice' >&2\ncat '" + stdoutFile + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestTreeCompositionDecodesAListDespiteAKubectlWarning(t *testing.T) {
	fakeKubectlWithWarning(t, `{"apiVersion":"v1","kind":"List","items":[{"apiVersion":"v1","kind":"Endpoints","metadata":{"name":"web","namespace":"apps"}}]}`)
	list, err := kubectlGetUnstructuredList(context.Background(), "endpoints")
	if err != nil {
		t.Fatalf("kubectlGetUnstructuredList() error = %v; a warning on stderr must not reach the decoder", err)
	}
	if len(list.Items) != 1 || list.Items[0].GetName() != "web" {
		t.Fatalf("items = %#v", list.Items)
	}
}

func TestTreeCompositionResourceNamesExcludeAKubectlWarning(t *testing.T) {
	fakeKubectlWithWarning(t, "deployments.apps\nendpoints\n")
	names, err := kubectlAPIResources(context.Background())
	if err != nil {
		t.Fatalf("kubectlAPIResources() error = %v", err)
	}
	for _, name := range names {
		if strings.Contains(name, "Warning") {
			t.Fatalf("resource names include kubectl's warning: %q", names)
		}
	}
	if len(names) != 2 {
		t.Fatalf("names = %q, want the two on stdout", names)
	}
}

func TestImportManifestExcludesAKubectlWarning(t *testing.T) {
	fakeKubectlWithWarning(t, "apiVersion: v1\nkind: Endpoints\nmetadata:\n  name: web\n  namespace: apps\n")
	manifest, err := fetchManifest("Endpoints", "apps", "web")
	if err != nil {
		t.Fatalf("fetchManifest() error = %v", err)
	}
	if strings.Contains(string(manifest), "Warning") || strings.Contains(string(manifest), "deprecated") {
		t.Fatalf("the manifest to import carries kubectl's warning:\n%s", manifest)
	}
}
