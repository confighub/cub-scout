// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func exportContextCommand(t *testing.T, name, selection string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: name}
	cmd.SetContext(context.Background())
	cmd.Flags().String("kube-context", "", "")
	require.NoError(t, cmd.Flags().Set("kube-context", selection))
	return cmd
}
func preserveExportFlags(t *testing.T) {
	t.Helper()
	gf, go_, gn, gempty, gj, gm := graphExportFormat, graphExportOutput, graphExportNamespace, graphExportEmpty, graphExportJSON, graphExportMaxNodes
	so, sn, sk, sr := snapshotOutput, snapshotNamespace, snapshotKind, snapshotRelations
	t.Cleanup(func() {
		graphExportFormat, graphExportOutput, graphExportNamespace, graphExportEmpty, graphExportJSON, graphExportMaxNodes = gf, go_, gn, gempty, gj, gm
		snapshotOutput, snapshotNamespace, snapshotKind, snapshotRelations = so, sn, sk, sr
	})
	graphExportFormat, graphExportOutput, graphExportNamespace, graphExportEmpty, graphExportJSON, graphExportMaxNodes = "json", "", "team-a", false, false, 300
	snapshotOutput, snapshotNamespace, snapshotKind, snapshotRelations = "", "team-a", "Deployment", false
	t.Setenv("CUB_SCOUT_TEST_TIME", "")
	t.Setenv("CUB_SCOUT_TEST_CLUSTER", "")
	t.Setenv("CLUSTER_NAME", "ambient-label")
}
func TestGraphAndSnapshotExplicitContextRoutesSelectedReads(t *testing.T) {
	for _, name := range []string{"graph", "snapshot"} {
		t.Run(name, func(t *testing.T) {
			preserveExportFlags(t)
			requests := []string{}
			selected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/apis/apps/v1/namespaces/team-a/deployments":
					fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"same-name","namespace":"team-a","uid":"selected-uid"}}]}`)
				case "/apis/apps/v1/namespaces/team-a/replicasets":
					fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"ReplicaSetList","items":[]}`)
				case "/api/v1/namespaces/team-a/pods":
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"PodList","items":[]}`)
				default:
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"List","items":[]}`)
				}
			}))
			defer selected.Close()
			ambient := newCountedKubeServer(t)
			path, before := resolverKubeconfig(t, "ambient", map[string]string{"selected": selected.URL, "ambient": ambient.server.URL})
			t.Setenv("KUBECONFIG", path)
			cmd := exportContextCommand(t, name, "selected")
			var err error
			out := captureStdout(t, func() {
				if name == "graph" {
					err = runGraphExport(cmd, nil)
				} else {
					err = runSnapshot(cmd, nil)
				}
			})
			require.NoError(t, err)
			require.Zero(t, ambient.requests.Load())
			require.Contains(t, requests, "/apis/apps/v1/namespaces/team-a/deployments")
			var report map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(out), &report))
			require.Equal(t, "selected", report["cluster"])
			require.Contains(t, out, "same-name")
			require.NotContains(t, out, "ambient-label")
			if name == "graph" {
				require.Len(t, requests, 3)
			} else {
				require.Len(t, requests, 13)
			}
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
func TestGraphAndSnapshotInvalidContextRefusesBeforeFile(t *testing.T) {
	for _, name := range []string{"graph", "snapshot"} {
		for _, selection := range []string{"", "missing"} {
			t.Run(name+"/"+selection, func(t *testing.T) {
				preserveExportFlags(t)
				ambient := newCountedKubeServer(t)
				path, before := resolverKubeconfig(t, "ambient", map[string]string{"ambient": ambient.server.URL})
				t.Setenv("KUBECONFIG", path)
				output := filepath.Join(t.TempDir(), "must-not-exist")
				graphExportOutput, snapshotOutput = output, output
				cmd := exportContextCommand(t, name, selection)
				var err error
				out := captureStdout(t, func() {
					if name == "graph" {
						err = runGraphExport(cmd, nil)
					} else {
						err = runSnapshot(cmd, nil)
					}
				})
				require.Error(t, err)
				require.Empty(t, out)
				require.Zero(t, ambient.requests.Load())
				_, err = os.Stat(output)
				require.True(t, os.IsNotExist(err))
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, before, after)
			})
		}
	}
}
func TestGraphContextOfflineAndCancellationRefuse(t *testing.T) {
	preserveExportFlags(t)
	graphExportEmpty = true
	err := runGraphExport(exportContextCommand(t, "graph", "selected"), nil)
	require.ErrorContains(t, err, "requires live graph collection")
	graphExportEmpty = false
	ambient := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "ambient", map[string]string{"selected": ambient.server.URL})
	t.Setenv("KUBECONFIG", path)
	cmd := exportContextCommand(t, "graph", "selected")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	err = runGraphExport(cmd, nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, ambient.requests.Load())
	for _, command := range []*cobra.Command{graphExportCmd, snapshotCmd} {
		require.NotNil(t, command.Flags().Lookup("kube-context"))
	}
}
