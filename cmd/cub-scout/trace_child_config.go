// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const traceExecConfigExtension = "client.authentication.k8s.io/exec"
const traceChildContextName = "cub-scout-trace"

// traceChildKubeconfig owns a short-lived private kubeconfig for a child CLI.
type traceChildKubeconfig struct {
	Path    string
	Context string
	cleanup func() error
}

func (c *traceChildKubeconfig) Cleanup() error {
	if c == nil || c.cleanup == nil {
		return nil
	}
	return c.cleanup()
}

func (s *traceSession) createChildKubeconfig() (*traceChildKubeconfig, error) {
	return writeTraceChildKubeconfig(s, os.MkdirTemp, os.WriteFile, os.RemoveAll)
}

// Separate filesystem operations make failure cleanup deterministic in tests.
func writeTraceChildKubeconfig(s *traceSession, mkdirTemp func(string, string) (string, error), writeFile func(string, []byte, os.FileMode) error, removeAll func(string) error) (*traceChildKubeconfig, error) {
	if s == nil || s.config == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	cfg, err := s.restConfig()
	if err != nil {
		return nil, err
	}
	if err := rejectUnserializableTraceTransport(cfg); err != nil {
		return nil, err
	}
	proxyURL, err := traceProxyURL(cfg.Proxy, s.proxyURL)
	if err != nil {
		return nil, err
	}
	apiConfig, err := traceKubeconfigFromRESTConfig(cfg, proxyURL)
	if err != nil {
		return nil, err
	}
	data, err := clientcmd.Write(*apiConfig)
	if err != nil {
		return nil, fmt.Errorf("could not encode private Trace kubeconfig")
	}
	dir, err := mkdirTemp("", "cub-scout-trace-")
	if err != nil {
		return nil, fmt.Errorf("could not create private Trace directory")
	}
	cleanup := func() error { return removeAll(dir) }
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, cleanupTraceChildFailure("could not secure private Trace directory", cleanup)
	}
	path := filepath.Join(dir, "config")
	if err := writeFile(path, data, 0600); err != nil {
		return nil, cleanupTraceChildFailure("could not write private Trace kubeconfig", cleanup)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return nil, cleanupTraceChildFailure("could not secure private Trace kubeconfig", cleanup)
	}
	return &traceChildKubeconfig{Path: path, Context: traceChildContextName, cleanup: cleanup}, nil
}

func cleanupTraceChildFailure(reason string, cleanup func() error) error {
	if err := cleanup(); err != nil {
		return fmt.Errorf("%s; private Trace directory cleanup also failed", reason)
	}
	return fmt.Errorf("%s", reason)
}

func rejectUnserializableTraceTransport(c *rest.Config) error {
	if c.Transport != nil {
		return fmt.Errorf("Trace child binding cannot serialize a custom Kubernetes transport")
	}
	if c.Dial != nil {
		return fmt.Errorf("Trace child binding cannot serialize a custom Kubernetes dialer")
	}
	if c.WrapTransport != nil {
		return fmt.Errorf("Trace child binding cannot serialize a custom Kubernetes transport wrapper")
	}
	if c.RateLimiter != nil {
		return fmt.Errorf("Trace child binding cannot serialize a custom Kubernetes rate limiter")
	}
	return nil
}

// A rest.Config proxy callback carries no portable static URL provenance.
// Nil preserves client-go's environment default; callers with callbacks fail closed.
func traceProxyURL(proxy func(*http.Request) (*url.URL, error), capturedStaticProxyURL string) (string, error) {
	if proxy == nil {
		return capturedStaticProxyURL, nil
	}
	if capturedStaticProxyURL != "" {
		return capturedStaticProxyURL, nil
	}
	return "", fmt.Errorf("Trace child binding cannot serialize a Kubernetes proxy callback without captured static proxy provenance")
}

func traceKubeconfigFromRESTConfig(c *rest.Config, proxyURL string) (*clientcmdapi.Config, error) {
	if strings.TrimSpace(c.Host) == "" {
		return nil, fmt.Errorf("Trace session has no Kubernetes API server address")
	}
	server := c.Host
	if c.APIPath != "" {
		return nil, fmt.Errorf("Trace child binding cannot serialize a custom Kubernetes API path")
	}
	cluster := clientcmdapi.Cluster{Server: server, TLSServerName: c.TLSClientConfig.ServerName, InsecureSkipTLSVerify: c.TLSClientConfig.Insecure, CertificateAuthorityData: append([]byte(nil), c.TLSClientConfig.CAData...), ProxyURL: proxyURL, DisableCompression: c.DisableCompression}
	user := clientcmdapi.AuthInfo{Username: c.Username, Password: c.Password, Token: c.BearerToken, ClientCertificateData: append([]byte(nil), c.TLSClientConfig.CertData...), ClientKeyData: append([]byte(nil), c.TLSClientConfig.KeyData...), Impersonate: c.Impersonate.UserName, ImpersonateUID: c.Impersonate.UID, ImpersonateGroups: append([]string(nil), c.Impersonate.Groups...)}
	if c.Impersonate.Extra != nil {
		user.ImpersonateUserExtra = make(map[string][]string, len(c.Impersonate.Extra))
		for k, v := range c.Impersonate.Extra {
			user.ImpersonateUserExtra[k] = append([]string(nil), v...)
		}
	}
	if c.AuthProvider != nil {
		p := *c.AuthProvider
		p.Config = make(map[string]string, len(c.AuthProvider.Config))
		for k, v := range c.AuthProvider.Config {
			p.Config[k] = v
		}
		user.AuthProvider = &p
	}
	if c.ExecProvider != nil {
		execConfig := c.ExecProvider.DeepCopy()
		if execConfig.Config != nil {
			ext := execConfig.Config.DeepCopyObject()
			cluster.Extensions = map[string]runtime.Object{traceExecConfigExtension: ext}
			execConfig.Config = nil
		}
		user.Exec = execConfig
	}
	config := &clientcmdapi.Config{CurrentContext: traceChildContextName, Clusters: map[string]*clientcmdapi.Cluster{traceChildContextName: &cluster}, AuthInfos: map[string]*clientcmdapi.AuthInfo{traceChildContextName: &user}, Contexts: map[string]*clientcmdapi.Context{traceChildContextName: {Cluster: traceChildContextName, AuthInfo: traceChildContextName}}}
	return config, nil
}
