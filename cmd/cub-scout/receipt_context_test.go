// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// #812: receipt verify had no way to select a context. Each mode must read only
// the selected one. Run as processes: receipt verify can end through os.Exit.
func TestReceiptVerifyModesReadOnlyTheSelectedContext(t *testing.T) {
	binary := sharedTestBinary(t)
	dir := t.TempDir()
	desired := filepath.Join(dir, "desired.yaml")
	require.NoError(t, os.WriteFile(desired, []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: team-a\nspec:\n  replicas: 1\n"), 0o600))
	prerequisites := filepath.Join(dir, "prerequisites.yaml")
	require.NoError(t, os.WriteFile(prerequisites, []byte("requiredNamespaces:\n  - team-a\n"), 0o600))
	for name, args := range map[string][]string{
		"single resource":     {"receipt", "verify", "deployment/api", "--namespace", "team-a", "--format", "json"},
		"source truth":        {"receipt", "verify", "deployment/api", "--namespace", "team-a", "--predicate", "source-truth-pass", "--strategy", "git-argo", "--format", "json"},
		"aggregate namespace": {"receipt", "verify", "--scope", "namespace/team-a", "--format", "json"},
		"object set":          {"receipt", "verify", "--file", desired, "--predicate", "object-set-matches", "--format", "json"},
		"workloads converged": {"receipt", "verify", "--file", desired, "--predicate", "workloads-converged", "--format", "json"},
		"prerequisites":       {"receipt", "verify", "--prerequisites", prerequisites, "--format", "json"},
	} {
		t.Run(name, func(t *testing.T) { assertProcessReadsOnlySelectedContext(t, binary, args) })
	}
}

// fakeTracerCLIs puts flux, argocd and kubectl stand-ins first on PATH. Each
// records its arguments and KUBECONFIG, then fails, so a test can see exactly
// which cluster a tracer subprocess was pointed at.
func fakeTracerCLIs(t *testing.T) (record string) {
	t.Helper()
	dir := t.TempDir()
	record = filepath.Join(dir, "calls.log")
	for _, tool := range []string{"flux", "argocd", "kubectl"} {
		// SERVER is the API server named by the kubeconfig the tool was given.
		script := "#!/bin/sh\nserver=$(grep -Eo 'https?://[^\" ,]+' \"$KUBECONFIG\" 2>/dev/null | head -1)\n" +
			"echo \"" + tool + " $* KUBECONFIG=$KUBECONFIG SERVER=$server\" >> \"" + record + "\"\n" +
			// The availability probe succeeds, so the CLI counts as installed; every
			// real command fails, which is enough to see where it was pointed.
			"[ \"$1\" = version ] && exit 0\nexit 1\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, tool), []byte(script), 0o755))
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+"/usr/bin:/bin")
	return record
}

func recordedCalls(t *testing.T, record string) []string {
	t.Helper()
	data, err := os.ReadFile(record)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var calls []string
	for _, call := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		// `version --client` is the availability probe; it contacts no cluster.
		if call != "" && !strings.Contains(call, " version --client") {
			calls = append(calls, call)
		}
	}
	return calls
}

// A receipt's Git source anchor came from flux/argocd/kubectl subprocesses on
// the ambient context, whatever the object reads used. With an explicit
// context every tracer must be bound to it, for receipt verify and for the
// receipts watch builds.
func TestReceiptGitSourceTracersAreBoundToTheSelectedContext(t *testing.T) {
	selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
	t.Setenv("KUBECONFIG", path)
	live := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "api", "namespace": "team-a"},
	}}
	fluxOwner := agent.Ownership{Type: agent.OwnerFlux, SubType: "kustomization", Name: "shop-apps", Namespace: "flux-system"}
	argoOwner := agent.Ownership{Type: agent.OwnerArgo, Name: "payments", Namespace: "argocd"}

	bound, err := boundCommandContext(exportContextCommand(t, "receipt", "selected"))
	require.NoError(t, err)

	t.Run("flux is bound to a private kubeconfig for the selected context", func(t *testing.T) {
		record := fakeTracerCLIs(t)
		receiptGitSourceAnchor(bound, live, fluxOwner)
		calls := recordedCalls(t, record)
		require.NotEmpty(t, calls, "the bound flux tracer was not invoked")
		for _, call := range calls {
			require.True(t, strings.HasPrefix(call, "flux "), "only flux may run for a Flux owner: %s", call)
			// The bound tracer gets a private single-context kubeconfig, so the
			// proof is the server that file names, not the context's name.
			require.Contains(t, call, "--kubeconfig ", "flux ran without a bound kubeconfig: %s", call)
			require.NotContains(t, call, path, "flux was given the shared kubeconfig, whose current-context is ambient: %s", call)
			require.Contains(t, call, "SERVER="+selected.server.URL, "flux was not pointed at the selected cluster: %s", call)
			require.NotContains(t, call, ambient.server.URL, "flux was pointed at the ambient cluster: %s", call)
		}
	})

	t.Run("argo uses the selected cluster's client and no subprocess", func(t *testing.T) {
		record := fakeTracerCLIs(t)
		before := selected.requests.Load()
		receiptGitSourceAnchor(bound, live, argoOwner)
		require.Empty(t, recordedCalls(t, record), "a bound Argo trace must not shell out")
		require.Greater(t, selected.requests.Load(), before, "the bound Argo tracer did not read the selected cluster")
	})

	t.Run("control: without a selection the legacy ambient tracer still runs", func(t *testing.T) {
		record := fakeTracerCLIs(t)
		receiptGitSourceAnchor(context.Background(), live, fluxOwner)
		calls := recordedCalls(t, record)
		require.NotEmpty(t, calls)
		for _, call := range calls {
			require.NotContains(t, call, "--kubeconfig ", "the legacy tracer is unbound: %s", call)
			require.Contains(t, call, "SERVER="+ambient.server.URL, "the legacy tracer follows the ambient kubeconfig: %s", call)
		}
	})

	require.Zero(t, ambient.requests.Load(), "the ambient cluster was read: %v", ambient.paths)
}

// watch --kube-context bound its own reads, but the receipts it built took
// their Git source anchor from ambient tracers. The receipt builder must be
// handed the selection; without the flag it must not be.
func TestWatchHandsItsSelectionToReceiptBuilding(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selection string
		want      string
	}{
		{"explicit context", "selected", "selected"},
		{"no flag", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer overrideWatchDeps(t)()
			priorBuild := watchBuildReceiptForEventFn
			defer func() { watchBuildReceiptForEventFn = priorBuild }()
			priorCap := watchReceiptBatchCap
			defer func() { watchReceiptBatchCap = priorCap }()
			selected, ambient := newCountedKubeServer(t), newCountedKubeServer(t)
			path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.server.URL, "ambient": ambient.server.URL})
			t.Setenv("KUBECONFIG", path)
			t.Setenv("KUBERNETES_SERVICE_HOST", "")
			watchCollectState = func(context.Context, dynamic.Interface, string) (watchState, error) {
				return watchState{entriesByID: map[string]MapEntry{"a": {Kind: "Deployment", Name: "api", Namespace: "team-a"}}, findings: map[string]watchFinding{}}, nil
			}
			calls, got := 0, "unset"
			watchBuildReceiptForEventFn = func(ctx context.Context, _ watchEvent, _ dynamic.Interface, _ bool) (*agent.Statement, error) {
				calls++
				got = ""
				if binding := treeContextBinding(ctx); binding != nil {
					got = binding.context
				}
				return nil, nil
			}
			cmd := &cobra.Command{Use: "watch"}
			cmd.SetContext(context.Background())
			cmd.Flags().String("kube-context", "", "")
			if tc.selection != "" {
				require.NoError(t, cmd.Flags().Set("kube-context", tc.selection))
			}
			output := filepath.Join(t.TempDir(), "events.jsonl")
			require.NoError(t, runWatchWithOptions(cmd, watchOptions{CommandName: "watch", OutputFile: output, Namespace: "team-a", Once: true, Interval: time.Second, MaxQueuedEvents: 10, EmitReceiptOn: "resource.discovered", EmitReceiptBatchCap: 10}))
			require.Equal(t, 1, calls, "one discovered resource should build one receipt")
			require.Equal(t, tc.want, got)
		})
	}
}
