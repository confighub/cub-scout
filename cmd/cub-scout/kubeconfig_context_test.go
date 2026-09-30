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
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const resolverTestToken = "test-only-kubeconfig-token-do-not-log"

type countedKubeServer struct {
	server   *httptest.Server
	requests atomic.Int32
	mu       sync.Mutex
	paths    []string
}

func newCountedKubeServer(t *testing.T) *countedKubeServer {
	t.Helper()
	result := &countedKubeServer{}
	result.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result.requests.Add(1)
		result.mu.Lock()
		result.paths = append(result.paths, r.URL.Path)
		result.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"apiVersion":"v1","kind":"NamespaceList","metadata":{},"items":[]}`)
	}))
	t.Cleanup(result.server.Close)
	return result
}

func (s *countedKubeServer) requestPaths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

func resolverKubeconfig(t *testing.T, current string, contexts map[string]string) (string, []byte) {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.CurrentContext = current
	for name, server := range contexts {
		clusterName, userName := name+"-cluster", name+"-user"
		cfg.Clusters[clusterName] = &clientcmdapi.Cluster{Server: server}
		cfg.AuthInfos[userName] = &clientcmdapi.AuthInfo{Token: resolverTestToken}
		cfg.Contexts[name] = &clientcmdapi.Context{Cluster: clusterName, AuthInfo: userName}
	}
	data, err := clientcmd.Write(*cfg)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(path, data, 0600))
	return path, data
}

func resolverRules(path string) *clientcmd.ClientConfigLoadingRules {
	return &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
}

func requestKubernetesNamespaces(t *testing.T, config *rest.Config) {
	t.Helper()
	client, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)
	_, err = client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
}

func TestResolveClusterConfigExplicitContextUsesOnlyNamedServer(t *testing.T) {
	alpha := newCountedKubeServer(t)
	beta := newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})

	inClusterCalls := 0
	cfg, selected, err := resolveClusterConfig("beta", true, resolverRules(path), func() (*rest.Config, error) {
		inClusterCalls++
		return &rest.Config{Host: alpha.server.URL}, nil
	})
	require.NoError(t, err)
	require.Equal(t, "beta", selected)
	require.Equal(t, 0, inClusterCalls, "strict selection must not consult in-cluster credentials")
	require.Equal(t, beta.server.URL, cfg.Host)
	requestKubernetesNamespaces(t, cfg)
	require.EqualValues(t, 0, alpha.requests.Load())
	require.EqualValues(t, 1, beta.requests.Load())
	require.Equal(t, []string{"/api/v1/namespaces"}, beta.requestPaths())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "resolving a named context must not rewrite kubeconfig")
}

func TestResolveClusterConfigMissingExplicitContextFailsWithoutCredentialFallback(t *testing.T) {
	alpha := newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "", map[string]string{"default": alpha.server.URL})
	inClusterCalls := 0
	cfg, selected, err := resolveClusterConfig("missing", true, resolverRules(path), func() (*rest.Config, error) {
		inClusterCalls++
		return &rest.Config{Host: alpha.server.URL}, nil
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing")
	require.Nil(t, cfg)
	require.Empty(t, selected)
	require.Zero(t, inClusterCalls, "missing explicit selection must fail instead of falling back")
	encoded, marshalErr := json.Marshal(map[string]string{"error": err.Error()})
	require.NoError(t, marshalErr)
	require.NotContains(t, string(encoded), resolverTestToken, "configuration errors must not expose credentials")
	require.Zero(t, alpha.requests.Load())
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, before, after)
}

func TestResolveClusterConfigDoesNotApplyKubeconfigMigrationRules(t *testing.T) {
	alpha := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL})
	destination := filepath.Join(t.TempDir(), "migrated-kubeconfig")
	rules := resolverRules(path)
	rules.MigrationRules = map[string]string{destination: path}

	cfg, selected, err := resolveClusterConfig("alpha", true, rules, nil)
	require.NoError(t, err)
	require.Equal(t, "alpha", selected)
	require.Equal(t, alpha.server.URL, cfg.Host)
	_, statErr := os.Stat(destination)
	require.ErrorIs(t, statErr, os.ErrNotExist, "context resolution must not write a migrated kubeconfig")
	require.Equal(t, map[string]string{destination: path}, rules.MigrationRules, "resolver must not mutate caller-owned rules")
}

func TestResolveClusterConfigOmittedSelectionKeepsLegacyPriority(t *testing.T) {
	alpha := newCountedKubeServer(t)
	inCluster := newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "", map[string]string{"default": alpha.server.URL})

	// The TUI may display "default" when kubeconfig has no current context.
	// A legacy launch still prefers in-cluster credentials over that display name.
	binding := resolveLocalClusterBinding("default", resolverRules(path), func() (*rest.Config, error) {
		return &rest.Config{Host: inCluster.server.URL}, nil
	})
	require.NoError(t, binding.err)
	require.Empty(t, binding.context, "in-cluster inventory has no kubeconfig context binding")
	require.Equal(t, inCluster.server.URL, binding.config.Host)
	model := LocalClusterModel{contextName: "default", clusterBinding: binding}
	loaded, ok := model.loadLocalClusterData().(localDataLoadedMsg)
	require.True(t, ok)
	require.NoError(t, loaded.err)
	require.Empty(t, loaded.boundedContext)
	require.Positive(t, inCluster.requests.Load())
	require.Zero(t, alpha.requests.Load())

	// If in-cluster credentials are unavailable, the TUI's legacy default
	// context hint selects the default context, as before.
	fallback, selected, err := resolveClusterConfig("default", false, resolverRules(path), func() (*rest.Config, error) {
		return nil, fmt.Errorf("not running in cluster")
	})
	require.NoError(t, err)
	require.Equal(t, "default", selected)
	require.Equal(t, alpha.server.URL, fallback.Host)
	requestKubernetesNamespaces(t, fallback)
	require.EqualValues(t, 1, alpha.requests.Load())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestLocalClusterTUIRefreshUsesPinnedKubeconfigContext(t *testing.T) {
	alpha := newCountedKubeServer(t)
	beta := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	rules := resolverRules(path)
	binding := resolveLocalClusterBinding("", rules, func() (*rest.Config, error) {
		return nil, fmt.Errorf("not running in cluster")
	})
	require.NoError(t, binding.err)
	require.Equal(t, "alpha", binding.context)

	// Simulate a kubeconfig current-context change during a TUI session. A
	// refresh must reuse the immutable binding captured on launch.
	updated, _ := resolverKubeconfig(t, "beta", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	updatedBytes, err := os.ReadFile(updated)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, updatedBytes, 0600))

	model := LocalClusterModel{contextName: "alpha", clusterBinding: binding}
	loaded, ok := model.loadLocalClusterData().(localDataLoadedMsg)
	require.True(t, ok)
	require.NoError(t, loaded.err)
	require.Equal(t, "alpha", loaded.boundedContext)
	require.Positive(t, alpha.requests.Load())
	require.Zero(t, beta.requests.Load(), "TUI refresh must not switch to changed current-context")
}

func TestResolveClusterConfigParallelSelectionsStayIndependent(t *testing.T) {
	alpha := newCountedKubeServer(t)
	beta := newCountedKubeServer(t)
	path, before := resolverKubeconfig(t, "alpha", map[string]string{"alpha": alpha.server.URL, "beta": beta.server.URL})
	rules := resolverRules(path)
	type result struct {
		name   string
		config *rest.Config
		err    error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"alpha", "beta"} {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg, selected, err := resolveClusterConfig(name, true, rules, nil)
			if err == nil && selected != name {
				err = fmt.Errorf("selected context %q, want %q", selected, name)
			}
			results <- result{name: name, config: cfg, err: err}
		}()
	}
	wg.Wait()
	close(results)
	resolved := map[string]*rest.Config{}
	for result := range results {
		require.NoError(t, result.err)
		resolved[result.name] = result.config
	}
	require.Equal(t, alpha.server.URL, resolved["alpha"].Host)
	require.Equal(t, beta.server.URL, resolved["beta"].Host)
	requestKubernetesNamespaces(t, resolved["alpha"])
	requestKubernetesNamespaces(t, resolved["beta"])
	require.EqualValues(t, 1, alpha.requests.Load())
	require.EqualValues(t, 1, beta.requests.Load())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
