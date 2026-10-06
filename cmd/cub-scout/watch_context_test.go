// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func watchContextCommand(t *testing.T, name, selection string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: name}
	cmd.SetContext(context.Background())
	cmd.Flags().String("kube-context", "", "")
	require.NoError(t, cmd.Flags().Set("kube-context", selection))
	return cmd
}

func TestWatchAndBotExplicitContextPinsConfigBeforeCollection(t *testing.T) {
	for _, name := range []string{"watch", "bot"} {
		t.Run(name, func(t *testing.T) {
			defer overrideWatchDeps(t)()
			priorCap := watchReceiptBatchCap
			defer func() { watchReceiptBatchCap = priorCap }()
			beta := newCountedKubeServer(t)
			alpha := &countedKubeServer{}
			headers := make(chan string, 2)
			alpha.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				alpha.requests.Add(1)
				alpha.mu.Lock()
				alpha.paths = append(alpha.paths, r.URL.Path)
				alpha.mu.Unlock()
				headers <- r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"apiVersion":"apps/v1","kind":"DeploymentList","items":[]}`)
			}))
			t.Cleanup(alpha.server.Close)
			path, before := resolverKubeconfig(t, "beta", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
			config, err := clientcmd.Load(before)
			require.NoError(t, err)
			config.AuthInfos["alpha-user"].Token = "selected-token"
			config.Clusters["alpha-cluster"].CertificateAuthorityData = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: alpha.server.Certificate().Raw})
			before, err = clientcmd.Write(*config)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, before, 0600))
			t.Setenv("KUBECONFIG", path)
			watchBuildConfig = func() (*rest.Config, error) { t.Fatal("explicit selection entered ambient builder"); return nil, nil }
			watchCollectState = func(ctx context.Context, client dynamic.Interface, namespace string) (watchState, error) {
				// Mutating this private file after client capture cannot retarget this cycle.
				config, err := clientcmd.Load(before)
				require.NoError(t, err)
				config.Clusters["alpha-cluster"].Server = beta.server.URL
				config.AuthInfos["alpha-user"].Token = "changed-token"
				updated, err := clientcmd.Write(*config)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, updated, 0600))
				_, err = client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace(namespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)
				return watchState{entriesByID: map[string]MapEntry{"a": {Kind: "Deployment", Name: "api", Namespace: namespace}}, findings: map[string]watchFinding{}}, nil
			}
			output := filepath.Join(t.TempDir(), "events.jsonl")
			require.NoError(t, runWatchWithOptions(watchContextCommand(t, name, "alpha"), watchOptions{CommandName: name, OutputFile: output, Namespace: "team-a", Once: true, Interval: time.Second, MaxQueuedEvents: 10}))
			require.EqualValues(t, 1, alpha.requests.Load())
			require.Zero(t, beta.requests.Load())
			require.Equal(t, "Bearer selected-token", <-headers)
			require.Equal(t, []string{"/apis/apps/v1/namespaces/team-a/deployments"}, alpha.requestPaths())
			raw, err := os.ReadFile(output)
			require.NoError(t, err)
			require.Contains(t, string(raw), "resource.discovered")
			// Restore and rerun resolution without the deliberate test mutation to
			// independently prove command execution never writes kubeconfig.
			require.NoError(t, os.WriteFile(path, before, 0600))
			watchCollectState = func(context.Context, dynamic.Interface, string) (watchState, error) { return watchState{}, nil }
			require.NoError(t, runWatchWithOptions(watchContextCommand(t, name, "alpha"), watchOptions{OutputFile: output, Once: true, Interval: time.Second, MaxQueuedEvents: 10}))
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestWatchAndBotInvalidContextRefusesBeforeSinkAndReads(t *testing.T) {
	for _, name := range []string{"watch", "bot"} {
		for _, selection := range []string{"", "missing"} {
			t.Run(name+"/"+selection, func(t *testing.T) {
				defer overrideWatchDeps(t)()
				priorCap := watchReceiptBatchCap
				defer func() { watchReceiptBatchCap = priorCap }()
				server := newCountedKubeServer(t)
				path, before := resolverKubeconfig(t, "ambient", map[string]string{"ambient": server.server.URL})
				t.Setenv("KUBECONFIG", path)
				watchBuildConfig = func() (*rest.Config, error) { t.Fatal("invalid explicit selection fell back"); return nil, nil }
				watchCollectState = func(context.Context, dynamic.Interface, string) (watchState, error) {
					t.Fatal("invalid selection read inventory")
					return watchState{}, nil
				}
				output := filepath.Join(t.TempDir(), "must-not-exist")
				err := runWatchWithOptions(watchContextCommand(t, name, selection), watchOptions{OutputFile: output, Once: true, Interval: time.Second, MaxQueuedEvents: 10})
				require.Error(t, err)
				require.Zero(t, server.requests.Load())
				_, err = os.Stat(output)
				require.True(t, os.IsNotExist(err))
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, before, after)
			})
		}
	}
}

func TestWatchAndBotExposeContextFlag(t *testing.T) {
	for _, cmd := range []*cobra.Command{watchCmd, botCmd} {
		require.NotNil(t, cmd.Flags().Lookup("kube-context"))
	}
}

func TestWatchAndBotFailedSelectedAPINeverFallsBack(t *testing.T) {
	for _, name := range []string{"watch", "bot"} {
		for _, failure := range []string{"denied", "unreachable"} {
			t.Run(name+"/"+failure, func(t *testing.T) {
				defer overrideWatchDeps(t)()
				priorCap := watchReceiptBatchCap
				defer func() { watchReceiptBatchCap = priorCap }()
				beta := newCountedKubeServer(t)
				alpha := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","message":"denied","code":403}`)
				}))
				t.Cleanup(alpha.Close)
				if failure == "unreachable" {
					alpha.Close()
				}
				path, before := resolverKubeconfig(t, "beta", map[string]string{"alpha": alpha.URL, "beta": beta.server.URL})
				t.Setenv("KUBECONFIG", path)
				watchBuildConfig = func() (*rest.Config, error) { t.Fatal("selected API failure entered ambient builder"); return nil, nil }
				watchCollectState = func(ctx context.Context, client dynamic.Interface, namespace string) (watchState, error) {
					_, err := client.Resource(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}).Namespace(namespace).List(ctx, metav1.ListOptions{})
					return watchState{}, err
				}
				output := filepath.Join(t.TempDir(), "events.jsonl")
				err := runWatchWithOptions(watchContextCommand(t, name, "alpha"), watchOptions{OutputFile: output, Namespace: "team-a", Once: true, Interval: time.Second, MaxQueuedEvents: 10})
				require.Error(t, err)
				if failure == "denied" {
					require.True(t, apierrors.IsForbidden(err))
				}
				require.Zero(t, beta.requests.Load())
				data, readErr := os.ReadFile(output)
				require.NoError(t, readErr)
				require.Empty(t, data, "failure must not become a healthy empty event stream")
				after, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.Equal(t, before, after)
			})
		}
	}
}
