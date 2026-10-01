// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestTraceSessionCapturesCredentialFilesBeforeFirstRead(t *testing.T) {
	var authorization string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"NamespaceList","items":[]}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	caPath, tokenPath := filepath.Join(dir, "ca"), filepath.Join(dir, "token")
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	require.NoError(t, os.WriteFile(caPath, ca, 0600))
	require.NoError(t, os.WriteFile(tokenPath, []byte("captured-token\n"), 0600))
	config := &rest.Config{Host: server.URL, TLSClientConfig: rest.TLSClientConfig{CAFile: caPath}, BearerTokenFile: tokenPath, BearerToken: "fallback-token"}
	session, err := newTraceSession(config, "captured")
	require.NoError(t, err)
	// Mutate credentials before even constructing the first client.
	require.NoError(t, os.WriteFile(caPath, []byte("not a certificate"), 0600))
	require.NoError(t, os.WriteFile(tokenPath, []byte("rotated-token"), 0600))
	client, err := session.kubernetesClient()
	require.NoError(t, err)
	_, err = client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	require.Equal(t, "Bearer captured-token", authorization)
	got, err := session.restConfig()
	require.NoError(t, err)
	require.Empty(t, got.CAFile)
	require.Empty(t, got.BearerTokenFile)
	require.Equal(t, ca, got.CAData)
	// The caller's config is untouched.
	require.Equal(t, caPath, config.CAFile)
	require.Equal(t, tokenPath, config.BearerTokenFile)
	require.Equal(t, "fallback-token", config.BearerToken)
	data, err := os.ReadFile(tokenPath)
	require.NoError(t, err)
	require.Equal(t, "rotated-token", string(data))
}

func TestTraceSessionCredentialFileCaptureFailsClosed(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "sensitive-credential-path")
	for _, tc := range []struct {
		name   string
		config rest.Config
	}{
		{"ca", rest.Config{TLSClientConfig: rest.TLSClientConfig{CAFile: missing}}},
		{"certificate", rest.Config{TLSClientConfig: rest.TLSClientConfig{CertFile: missing}}},
		{"key", rest.Config{TLSClientConfig: rest.TLSClientConfig{KeyFile: missing}}},
		{"token", rest.Config{BearerTokenFile: missing, BearerToken: "do-not-fallback"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.config.Host = "https://127.0.0.1"
			session, err := newTraceSession(&tc.config, "test")
			require.Error(t, err)
			require.Nil(t, session)
			require.NotContains(t, err.Error(), missing)
			require.NotContains(t, err.Error(), "do-not-fallback")
		})
	}
	empty := filepath.Join(t.TempDir(), "empty")
	require.NoError(t, os.WriteFile(empty, []byte(" \n"), 0600))
	_, err := newTraceSession(&rest.Config{Host: "https://127.0.0.1", BearerTokenFile: empty, BearerToken: "fallback"}, "test")
	require.ErrorContains(t, err, "configured file is empty")
}

func TestTraceSessionCredentialDataOverridesFiles(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	session, err := newTraceSession(&rest.Config{Host: "https://127.0.0.1", TLSClientConfig: rest.TLSClientConfig{
		CAFile: missing, CertFile: missing, KeyFile: missing,
		CAData: []byte("ca"), CertData: []byte("cert"), KeyData: []byte("key"),
	}}, "test")
	require.NoError(t, err)
	got, err := session.restConfig()
	require.NoError(t, err)
	require.Equal(t, []byte("ca"), got.CAData)
	require.Equal(t, []byte("cert"), got.CertData)
	require.Equal(t, []byte("key"), got.KeyData)
	require.Empty(t, got.CAFile)
	require.Empty(t, got.CertFile)
	require.Empty(t, got.KeyFile)
}
