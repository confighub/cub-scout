// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestTraceChildKubeconfigCapturesOnePrivateContext(t *testing.T) {
	root := t.TempDir()
	caPath := filepath.Join(root, "ca.pem")
	certPath := filepath.Join(root, "cert.pem")
	keyPath := filepath.Join(root, "key.pem")
	tokenPath := filepath.Join(root, "token")
	for path, content := range map[string]string{caPath: "ca-A", certPath: "cert-A", keyPath: "key-A", tokenPath: "token-A"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	original := filepath.Join(root, "source-config")
	originalBytes := []byte("source kubeconfig must remain unchanged")
	if err := os.WriteFile(original, originalBytes, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := &rest.Config{Host: "https://cluster-A.invalid", Username: "user-A", Password: "pass-A", BearerTokenFile: tokenPath, TLSClientConfig: rest.TLSClientConfig{CAFile: caPath, CertFile: certPath, KeyFile: keyPath, ServerName: "api-A", Insecure: true}, Impersonate: rest.ImpersonationConfig{UserName: "actor-A", UID: "uid-A", Groups: []string{"group-A"}, Extra: map[string][]string{"scope": {"one", "two"}}}, ExecProvider: &api.ExecConfig{Command: "credential-helper", Args: []string{"--audience", "aud-A"}, Env: []api.ExecEnvVar{{Name: "EXEC_MARKER", Value: "captured"}}, APIVersion: "client.authentication.k8s.io/v1", ProvideClusterInfo: true, InteractiveMode: api.NeverExecInteractiveMode, Config: &unstructured.Unstructured{Object: map[string]interface{}{"audience": "nested-aud"}}}, AuthProvider: &api.AuthProviderConfig{Name: "oidc", Config: map[string]string{"id-token": "auth-A"}}}
	session, err := newTraceSession(cfg, "source-context-A")
	if err != nil {
		t.Fatal(err)
	}
	// Mutating the caller's config and deleting all referenced files after
	// capture must not retarget or erase the private child configuration.
	cfg.Host = "https://cluster-B.invalid"
	cfg.Username = "user-B"
	cfg.TLSClientConfig.CAData = []byte("ca-B")
	cfg.Impersonate.Groups[0] = "group-B"
	cfg.Impersonate.Extra["scope"][0] = "changed"
	cfg.ExecProvider.Args[1] = "aud-B"
	cfg.AuthProvider.Config["id-token"] = "auth-B"
	for _, path := range []string{caPath, certPath, keyPath, tokenPath} {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	child, err := session.createChildKubeconfig()
	if err != nil {
		t.Fatal(err)
	}
	defer child.Cleanup()
	if child.Context != traceChildContextName {
		t.Fatalf("context=%q", child.Context)
	}
	info, err := os.Stat(child.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("kubeconfig mode=%o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(child.Path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0700 {
		t.Fatalf("directory mode=%o", dirInfo.Mode().Perm())
	}
	data, err := os.ReadFile(child.Path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := clientcmd.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Clusters) != 1 || len(loaded.AuthInfos) != 1 || len(loaded.Contexts) != 1 {
		t.Fatalf("expected one private tuple, got clusters=%d users=%d contexts=%d", len(loaded.Clusters), len(loaded.AuthInfos), len(loaded.Contexts))
	}
	cluster := loaded.Clusters[traceChildContextName]
	user := loaded.AuthInfos[traceChildContextName]
	if cluster.Server != "https://cluster-A.invalid" || string(cluster.CertificateAuthorityData) != "ca-A" || cluster.TLSServerName != "api-A" || !cluster.InsecureSkipTLSVerify {
		t.Fatalf("captured cluster differs: %#v", cluster)
	}
	if user.Username != "user-A" || user.Password != "pass-A" || user.Token != "token-A" || string(user.ClientCertificateData) != "cert-A" || string(user.ClientKeyData) != "key-A" {
		t.Fatalf("captured credentials differ: %#v", user)
	}
	if user.Impersonate != "actor-A" || user.ImpersonateUID != "uid-A" || !reflect.DeepEqual(user.ImpersonateGroups, []string{"group-A"}) || !reflect.DeepEqual(user.ImpersonateUserExtra, map[string][]string{"scope": {"one", "two"}}) {
		t.Fatalf("impersonation not preserved: %#v", user)
	}
	if user.AuthProvider == nil || user.AuthProvider.Name != "oidc" || user.AuthProvider.Config["id-token"] != "auth-A" {
		t.Fatalf("auth provider not preserved: %#v", user.AuthProvider)
	}
	if user.Exec == nil || user.Exec.Command != "credential-helper" || !reflect.DeepEqual(user.Exec.Args, []string{"--audience", "aud-A"}) || len(user.Exec.Env) != 1 || user.Exec.Env[0].Value != "captured" || !user.Exec.ProvideClusterInfo {
		t.Fatalf("exec config not preserved: %#v", user.Exec)
	}
	execOnly := loaded.DeepCopy()
	execOnly.AuthInfos[traceChildContextName].AuthProvider = nil
	execOnly.AuthInfos[traceChildContextName].Token = ""
	execOnly.AuthInfos[traceChildContextName].Username = ""
	execOnly.AuthInfos[traceChildContextName].Password = ""
	clientConfig := clientcmd.NewNonInteractiveClientConfig(*execOnly, traceChildContextName, &clientcmd.ConfigOverrides{}, nil)
	restCfg, err := clientConfig.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if restCfg.ExecProvider == nil || restCfg.ExecProvider.Config == nil || string(restCfg.ExecProvider.Config.(*runtime.Unknown).Raw) != `{"audience":"nested-aud"}` {
		t.Fatalf("exec cluster extension lost on client-go load: %#v", restCfg.ExecProvider)
	}
	if got, err := os.ReadFile(original); err != nil || !reflect.DeepEqual(got, originalBytes) {
		t.Fatalf("source config changed: %q err=%v", got, err)
	}
	if strings.Contains(string(data), "source-context-A") || strings.Contains(string(data), "source-config") {
		t.Fatal("private config leaked source path or display context")
	}
}

func TestTraceChildKubeconfigSurvivesSourceRetargetAndCredentialRemoval(t *testing.T) {
	root := t.TempDir()
	credential := filepath.Join(root, "token")
	if err := os.WriteFile(credential, []byte("token-A"), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	original := &clientcmdapi.Config{
		CurrentContext: "context-A",
		Clusters:       map[string]*clientcmdapi.Cluster{"cluster-A": {Server: "https://cluster-A.invalid"}},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{"user-A": {TokenFile: credential}},
		Contexts:       map[string]*clientcmdapi.Context{"context-A": {Cluster: "cluster-A", AuthInfo: "user-A"}},
	}
	if err := clientcmd.WriteToFile(*original, source); err != nil {
		t.Fatal(err)
	}
	loaded, err := clientcmd.LoadFromFile(source)
	if err != nil {
		t.Fatal(err)
	}
	restConfig, err := clientcmd.NewNonInteractiveClientConfig(*loaded, "context-A", &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	session, err := newTraceSession(restConfig, "context-A")
	if err != nil {
		t.Fatal(err)
	}
	retarget := &clientcmdapi.Config{CurrentContext: "context-B", Clusters: map[string]*clientcmdapi.Cluster{"cluster-B": {Server: "https://cluster-B.invalid"}}, AuthInfos: map[string]*clientcmdapi.AuthInfo{"user-B": {Token: "token-B"}}, Contexts: map[string]*clientcmdapi.Context{"context-B": {Cluster: "cluster-B", AuthInfo: "user-B"}}}
	if err := clientcmd.WriteToFile(*retarget, source); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(credential); err != nil {
		t.Fatal(err)
	}
	child, err := session.createChildKubeconfig()
	if err != nil {
		t.Fatal(err)
	}
	defer child.Cleanup()
	loadedChild, err := clientcmd.LoadFromFile(child.Path)
	if err != nil {
		t.Fatal(err)
	}
	if loadedChild.Clusters[child.Context].Server != "https://cluster-A.invalid" || loadedChild.AuthInfos[child.Context].Token != "token-A" {
		t.Fatalf("child followed changed source: cluster=%#v user=%#v", loadedChild.Clusters[child.Context], loadedChild.AuthInfos[child.Context])
	}
}

func TestTraceChildKubeconfigRejectsInvalidAndCleansWriteFailure(t *testing.T) {
	if _, err := (*traceSession)(nil).createChildKubeconfig(); err == nil {
		t.Fatal("nil session unexpectedly succeeded")
	}
	if _, err := (&traceSession{config: &rest.Config{}}).createChildKubeconfig(); err == nil {
		t.Fatal("invalid host unexpectedly succeeded")
	}
	if _, err := (&traceSession{config: &rest.Config{Host: "https://a", Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) { return nil, nil })}}).createChildKubeconfig(); err == nil || !strings.Contains(err.Error(), "custom Kubernetes transport") {
		t.Fatalf("custom transport error=%v", err)
	}
	if _, err := (&traceSession{config: &rest.Config{Host: "https://a", Proxy: func(req *http.Request) (*url.URL, error) {
		if req.URL.Host == "trace.invalid" {
			return &url.URL{Scheme: "http", Host: "a"}, nil
		}
		return &url.URL{Scheme: "http", Host: "b"}, nil
	}}}).createChildKubeconfig(); err == nil || !strings.Contains(err.Error(), "proxy callback") {
		t.Fatalf("dynamic proxy error=%v", err)
	}
	proxyURL, _ := url.Parse("http://proxy.invalid:8080")
	staticSession := &traceSession{config: &rest.Config{Host: "https://a", Proxy: http.ProxyURL(proxyURL)}, proxyURL: proxyURL.String()}
	staticChild, err := staticSession.createChildKubeconfig()
	if err != nil {
		t.Fatal(err)
	}
	loadedStatic, err := clientcmd.LoadFromFile(staticChild.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := loadedStatic.Clusters[staticChild.Context].ProxyURL; got != proxyURL.String() {
		t.Fatalf("static proxy=%q", got)
	}
	_ = staticChild.Cleanup()
	root := t.TempDir()
	_, err = writeTraceChildKubeconfig(&traceSession{config: &rest.Config{Host: "https://captured.invalid"}}, func(_, pattern string) (string, error) { return os.MkdirTemp(root, pattern) }, func(string, []byte, os.FileMode) error { return os.ErrPermission }, os.RemoveAll)
	if err == nil {
		t.Fatal("injected write failure unexpectedly succeeded")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temporary directory leaked after write error: %v", entries)
	}
	var leakedDir string
	_, err = writeTraceChildKubeconfig(&traceSession{config: &rest.Config{Host: "https://captured.invalid"}}, func(_, pattern string) (string, error) {
		var e error
		leakedDir, e = os.MkdirTemp(root, pattern)
		return leakedDir, e
	}, func(string, []byte, os.FileMode) error { return os.ErrPermission }, func(string) error { return os.ErrPermission })
	if err == nil || !strings.Contains(err.Error(), "cleanup also failed") {
		t.Fatalf("cleanup failure was hidden: %v", err)
	}
	_ = os.RemoveAll(leakedDir)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTraceChildKubeconfigPreservesEnvironmentProxyDefault(t *testing.T) {
	cfg, err := traceKubeconfigFromRESTConfig(&rest.Config{Host: "https://captured.invalid"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Clusters[traceChildContextName].ProxyURL != "" {
		t.Fatalf("empty proxy should retain client-go environment behavior")
	}
}
