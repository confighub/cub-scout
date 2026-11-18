package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeDemoKubectl puts a kubectl on the demo's path that records each call's
// arguments, one call per line, and answers api-resources as if Flux and
// Argo CD were installed. Nothing reaches a cluster.
func fakeDemoKubectl(t *testing.T) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls.log")
	script := filepath.Join(dir, "kubectl")
	require.NoError(t, os.WriteFile(script, []byte(`#!/bin/sh
echo "$*" >> "`+logPath+`"
case "$*" in *api-resources*) echo "gitrepositories.source.toolkit.fluxcd.io applications.argoproj.io";; esac
`), 0o755))
	old := demoKubectlBinary
	demoKubectlBinary = script
	t.Cleanup(func() { demoKubectlBinary = old })
	return logPath
}

func recordDemoCubScout(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	old := execCubScoutFn
	execCubScoutFn = func(args ...string) error {
		calls = append(calls, append([]string(nil), args...))
		return nil
	}
	t.Cleanup(func() { execCubScoutFn = old })
	return &calls
}

func demoKubectlCalls(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func setDemoContextFlag(t *testing.T, value string, set bool) {
	t.Helper()
	flag := quickstartDemoCmd.Flags().Lookup("kube-context")
	require.NotNil(t, flag)
	t.Cleanup(func() {
		_ = flag.Value.Set("")
		flag.Changed = false
		demoKubeContext = ""
	})
	if set {
		require.NoError(t, quickstartDemoCmd.Flags().Set("kube-context", value))
	}
	quickstartDemoCmd.SetContext(context.Background())
}

// #599: the demos write, with kubectl apply. With --kube-context every
// kubectl call names that context and every cub-scout command the demo runs
// is pinned to it; an empty or unknown name is refused before any call.
func TestDemoAppliesAndReadsOnlyTheSelectedContext(t *testing.T) {
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": "https://127.0.0.1:1", "ambient": "https://127.0.0.1:2"})
	t.Setenv("KUBECONFIG", path)

	t.Run("selected", func(t *testing.T) {
		logPath := fakeDemoKubectl(t)
		cubCalls := recordDemoCubScout(t)
		setDemoContextFlag(t, "selected", true)
		captureStdout(t, func() { require.NoError(t, runDemo(quickstartDemoCmd, []string{"quick"})) })

		kubectl := demoKubectlCalls(t, logPath)
		require.NotEmpty(t, kubectl)
		var applied bool
		for _, call := range kubectl {
			require.True(t, strings.HasPrefix(call, "--context selected "), "kubectl ran without the selected context: %q", call)
			applied = applied || strings.Contains(call, " apply ")
		}
		require.True(t, applied, "the quick demo applies its fixtures: %v", kubectl)
		require.NotEmpty(t, *cubCalls)
		for _, call := range *cubCalls {
			require.Equal(t, []string{"--kube-context", "selected"}, call[len(call)-2:], "cub-scout ran without the selected context: %v", call)
		}
	})

	for _, refused := range []string{"missing", ""} {
		t.Run("refuses "+refused, func(t *testing.T) {
			logPath := fakeDemoKubectl(t)
			cubCalls := recordDemoCubScout(t)
			setDemoContextFlag(t, refused, true)
			var err error
			captureStdout(t, func() { err = runDemo(quickstartDemoCmd, []string{"quick"}) })
			require.Error(t, err)
			require.Empty(t, demoKubectlCalls(t, logPath), "kubectl ran before the context was refused")
			require.Empty(t, *cubCalls)
		})
	}

	t.Run("without the flag, as before", func(t *testing.T) {
		logPath := fakeDemoKubectl(t)
		cubCalls := recordDemoCubScout(t)
		setDemoContextFlag(t, "", false)
		captureStdout(t, func() { require.NoError(t, runDemo(quickstartDemoCmd, []string{"quick"})) })
		for _, call := range demoKubectlCalls(t, logPath) {
			require.False(t, strings.Contains(call, "--context"), "kubectl gained a context nobody asked for: %q", call)
		}
		for _, call := range *cubCalls {
			require.NotContains(t, call, "--kube-context")
		}
	})
}

// The scenario command binds the context too: its cleanup deletes, and the
// deletes name the selected context.
func TestDemoScenarioCleanupDeletesOnlyInTheSelectedContext(t *testing.T) {
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": "https://127.0.0.1:1", "ambient": "https://127.0.0.1:2"})
	t.Setenv("KUBECONFIG", path)
	logPath := fakeDemoKubectl(t)
	recordDemoCubScout(t)

	cmd := quickstartDemoScenarioCmd
	flag := cmd.Flags().Lookup("kube-context")
	require.NotNil(t, flag)
	oldCleanup := demoCleanup
	demoCleanup = true
	t.Cleanup(func() {
		demoCleanup = oldCleanup
		_ = flag.Value.Set("")
		flag.Changed = false
		demoKubeContext = ""
	})
	require.NoError(t, cmd.Flags().Set("kube-context", "selected"))
	cmd.SetContext(context.Background())

	captureStdout(t, func() { require.NoError(t, cmd.RunE(cmd, []string{"break-glass"})) })
	calls := demoKubectlCalls(t, logPath)
	require.Len(t, calls, 2, "the cleanup deletes the fixtures and the namespace: %v", calls)
	for _, call := range calls {
		require.True(t, strings.HasPrefix(call, "--context selected delete "), "a delete ran without the selected context: %q", call)
	}
}
