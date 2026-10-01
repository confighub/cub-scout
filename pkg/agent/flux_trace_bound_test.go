// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestBoundFluxTracerUsesCapturedKubeconfigAndChildEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sentinel shell executable is Unix-only")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "captured-config")
	if err := os.WriteFile(configPath, []byte("server-A"), 0600); err != nil {
		t.Fatal(err)
	}
	originalPath := filepath.Join(dir, "source-config")
	if err := os.WriteFile(originalPath, []byte("server-A"), 0600); err != nil {
		t.Fatal(err)
	}
	argsPath := filepath.Join(dir, "argv.json")
	envPath := filepath.Join(dir, "env")
	configOut := filepath.Join(dir, "child-config")
	executable := filepath.Join(dir, "flux-sentinel")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TRACE_ARGS_OUT\"\nprintf '%s' \"$KUBECONFIG\" > \"$TRACE_ENV_OUT\"\ncat \"$KUBECONFIG\" > \"$TRACE_CONFIG_OUT\"\nprintf '%s\\n' 'Object: Kustomization/root' 'Namespace: team' 'Status: Ready'\n"
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", originalPath)
	t.Setenv("TRACE_ARGS_OUT", argsPath)
	t.Setenv("TRACE_ENV_OUT", envPath)
	t.Setenv("TRACE_CONFIG_OUT", configOut)
	tracer := NewFluxTracerWithKubeconfig(configPath, "captured-context")
	tracer.fluxPath = executable
	if !tracer.Available() {
		t.Fatal("bound tracer should be available with a valid private config")
	}
	// Retarget the ambient kubeconfig after binding. The child must continue
	// reading the separate private captured file.
	if err := os.WriteFile(originalPath, []byte("server-B"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := tracer.Trace(context.Background(), "Kustomization", "root", "team"); err != nil {
		t.Fatal(err)
	}
	argv, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	gotArgs := strings.Split(strings.TrimSpace(string(argv)), "\n")
	wantArgs := []string{"trace", "kustomization", "root", "-n", "team", "--kubeconfig", configPath, "--context", "captured-context"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("argv=%q want %q", gotArgs, wantArgs)
	}
	gotEnv, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotEnv) != configPath {
		t.Fatalf("child KUBECONFIG=%q", gotEnv)
	}
	gotConfig, err := os.ReadFile(configOut)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotConfig) != "server-A" {
		t.Fatalf("child read config=%q", gotConfig)
	}
	if got, err := os.ReadFile(originalPath); err != nil || string(got) != "server-B" {
		t.Fatalf("source config was unexpectedly modified: %q %v", got, err)
	}
	if got := os.Getenv("KUBECONFIG"); got != originalPath {
		t.Fatalf("parent KUBECONFIG changed: %q", got)
	}
	if got := withKubeconfig([]string{"PATH=/bin", "KUBECONFIG=/ambient/a", "kubeconfig=case-insensitive"}, "/private/captured"); !reflect.DeepEqual(got, []string{"PATH=/bin", "KUBECONFIG=/private/captured"}) {
		t.Fatalf("child env=%v", got)
	}
}

func TestBoundFluxTracerInvalidBindingNeverRunsAmbientFlux(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sentinel shell executable is Unix-only")
	}
	dir := t.TempDir()
	ran := filepath.Join(dir, "ran")
	executable := filepath.Join(dir, "sentinel")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf ran > \"$TRACE_RAN\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRACE_RAN", ran)
	tracer := NewFluxTracerWithKubeconfig(filepath.Join(dir, "missing"), "captured-context")
	tracer.fluxPath = executable
	if tracer.Available() {
		t.Fatal("missing bound config reported available")
	}
	if _, err := tracer.Trace(context.Background(), "Deployment", "app", "ns"); err == nil || !strings.Contains(err.Error(), "captured kubeconfig") {
		t.Fatalf("invalid binding error=%v", err)
	}
	if _, err := os.Stat(ran); !os.IsNotExist(err) {
		t.Fatalf("Flux ran without a valid binding, stat err=%v", err)
	}
}
