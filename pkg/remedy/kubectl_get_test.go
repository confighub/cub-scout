// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package remedy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordingKubectl is a kubectl stand-in that writes one line per invocation,
// with each argument on its own bracketed field, so a test can see the exact
// argument vector it was given.
func recordingKubectl(t *testing.T) (path, record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "calls.log")
	path = filepath.Join(dir, "kubectl")
	script := "#!/bin/sh\nfor a in \"$@\"; do printf '[%s]' \"$a\" >> \"" + record + "\"; done\nprintf '\\n' >> \"" + record + "\"\necho 'kind: ConfigMap'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, record
}

// The suggesters read a resource's current state with kubectl. The kind, name
// and namespace come from flags and files, and were interpolated into a
// command line run by `sh -c`, so a value like "x; some-command" ran
// some-command. They are passed as arguments now, after validation.
func TestKubectlGetNeverRunsAShell(t *testing.T) {
	kubectl, record := recordingKubectl(t)
	marker := filepath.Join(t.TempDir(), "marker")
	payload := "x; echo reached > " + marker + " #"

	for name, ref := range map[string]ResourceRef{
		"namespace":            {Kind: "ConfigMap", Name: "settings", Namespace: payload},
		"name":                 {Kind: "ConfigMap", Name: payload},
		"kind":                 {Kind: "configmap; echo reached > " + marker + " #", Name: "settings"},
		"flag as name":         {Kind: "ConfigMap", Name: "--kubeconfig=/etc/passwd"},
		"flag as namespace":    {Kind: "ConfigMap", Name: "settings", Namespace: "--all-namespaces"},
		"command substitution": {Kind: "ConfigMap", Name: "$(touch " + marker + ")"},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := KubectlGet(context.Background(), kubectl, "yaml", ref)
			if err == nil {
				t.Fatalf("an invalid %s was accepted; output %q", name, out)
			}
			if !strings.Contains(err.Error(), "invalid") {
				t.Errorf("err = %v, want it to say the value is invalid", err)
			}
			if _, statErr := os.Stat(record); statErr == nil {
				t.Error("kubectl was run with an invalid value")
			}
			if _, statErr := os.Stat(marker); statErr == nil {
				t.Fatal("the value was executed by a shell")
			}
		})
	}

	t.Run("valid values are passed as arguments after --", func(t *testing.T) {
		out, err := KubectlGet(context.Background(), kubectl, "yaml", ResourceRef{Kind: "ConfigMap", Name: "settings", Namespace: "team-a"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "kind: ConfigMap") {
			t.Errorf("output = %q", out)
		}
		calls, err := os.ReadFile(record)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := strings.TrimSpace(string(calls)), "[get][-o][yaml][-n][team-a][--][configmap][settings]"; got != want {
			t.Errorf("kubectl arguments = %s, want %s", got, want)
		}
	})
}

// Both suggesters must reach kubectl only through KubectlGet.
func TestSuggestersDoNotRunAShell(t *testing.T) {
	kubectl, record := recordingKubectl(t)
	marker := filepath.Join(t.TempDir(), "marker")
	ref := ResourceRef{Kind: "ConfigMap", Name: "x; echo reached > " + marker + " #"}

	if _, err := (&ConfigFixSuggester{kubectl: kubectl}).getCurrentState(context.Background(), ref); err == nil {
		t.Error("ConfigFixSuggester accepted an invalid name")
	}
	if _, err := (&DeleteResourceSuggester{kubectl: kubectl}).getResourceYAML(context.Background(), ref); err == nil {
		t.Error("DeleteResourceSuggester accepted an invalid name")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a suggester ran the value through a shell")
	}
	if _, err := os.Stat(record); err == nil {
		t.Error("a suggester ran kubectl with an invalid value")
	}
}
