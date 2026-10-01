// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type readonlyAuthFixture struct {
	values    map[string]string
	persister rest.AuthProviderConfigPersister
	next      http.RoundTripper
}

func (p *readonlyAuthFixture) Login() error { return nil }
func (p *readonlyAuthFixture) WrapTransport(next http.RoundTripper) http.RoundTripper {
	p.next = next
	return p
}
func (p *readonlyAuthFixture) RoundTrip(req *http.Request) (*http.Response, error) {
	refreshed := map[string]string{"token": "refreshed-fixture-token", "fail": p.values["fail"]}
	if err := p.persister.Persist(refreshed); err != nil {
		return nil, err
	}
	if p.values["fail"] == "true" {
		return nil, fmt.Errorf("fixture authentication failed")
	}
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer refreshed-fixture-token")
	return p.next.RoundTrip(clone)
}

func TestResolveClusterConfigAuthProviderNeverPersistsSharedConfig(t *testing.T) {
	// Register once per test invocation under a unique name, without touching
	// any real provider, credential, config file, or server.
	plugin := "scout-readonly-auth-" + filepath.Base(filepath.Dir(t.TempDir()))
	require.NoError(t, rest.RegisterAuthProviderPlugin(plugin, func(_ string, values map[string]string, persister rest.AuthProviderConfigPersister) (rest.AuthProvider, error) {
		return &readonlyAuthFixture{values: values, persister: persister}, nil
	}))
	for _, explicit := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("explicit=%t/fail=%t", explicit, fail), func(t *testing.T) {
				var selectedReads, ambientReads atomic.Int32
				selected := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					selectedReads.Add(1)
					if r.Method != "GET" || r.URL.Path != "/api/v1/namespaces" || r.Header.Get("Authorization") != "Bearer refreshed-fixture-token" {
						t.Errorf("unexpected selected request: %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusForbidden)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"apiVersion":"v1","kind":"NamespaceList","items":[]}`)
				}))
				defer selected.Close()
				ambient := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ambientReads.Add(1)
					w.WriteHeader(http.StatusForbidden)
				}))
				defer ambient.Close()
				raw := clientcmdapi.NewConfig()
				raw.CurrentContext = "selected"
				if explicit {
					raw.CurrentContext = "ambient"
				}
				raw.Clusters["selected"] = &clientcmdapi.Cluster{Server: selected.URL, InsecureSkipTLSVerify: true}
				raw.Clusters["ambient"] = &clientcmdapi.Cluster{Server: ambient.URL, InsecureSkipTLSVerify: true}
				raw.AuthInfos["fixture-user"] = &clientcmdapi.AuthInfo{AuthProvider: &clientcmdapi.AuthProviderConfig{Name: plugin, Config: map[string]string{"token": "initial-fixture-token", "fail": fmt.Sprint(fail)}}}
				raw.Contexts["selected"] = &clientcmdapi.Context{Cluster: "selected", AuthInfo: "fixture-user"}
				raw.Contexts["ambient"] = &clientcmdapi.Context{Cluster: "ambient", AuthInfo: "fixture-user"}
				before, err := clientcmd.Write(*raw)
				require.NoError(t, err)
				path := filepath.Join(t.TempDir(), "config")
				require.NoError(t, os.WriteFile(path, before, 0600))
				selector := ""
				if explicit {
					selector = "selected"
				}
				cfg, selectedName, err := resolveClusterConfig(selector, explicit, resolverRules(path), nil)
				require.NoError(t, err)
				require.Equal(t, "selected", selectedName)
				independent, _, err := resolveClusterConfig(selector, explicit, resolverRules(path), nil)
				require.NoError(t, err)
				independent.AuthProvider.Config["token"] = "independent-fixture-token"
				require.Equal(t, "initial-fixture-token", cfg.AuthProvider.Config["token"], "separate resolutions must not share provider settings")
				client, err := kubernetes.NewForConfig(cfg)
				require.NoError(t, err)
				_, err = client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
				if fail {
					require.ErrorContains(t, err, "fixture authentication failed")
					require.Zero(t, selectedReads.Load())
				} else {
					require.NoError(t, err)
					require.EqualValues(t, 1, selectedReads.Load())
				}
				require.Zero(t, ambientReads.Load(), "authentication failure must not fall back")
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, string(before), string(after), "observer authentication must not persist to kubeconfig")
			})
		}
	}
}
