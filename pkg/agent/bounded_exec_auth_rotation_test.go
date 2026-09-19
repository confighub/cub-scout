// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func boundedMTLSClient(t *testing.T, handler http.Handler) (*http.Client, *boundedExecAuth, string) {
	t.Helper()
	ca, key := newTestCA(t)
	cert1, key1 := newTestClientCertificate(t, ca, key, 1)
	cert2, key2 := newTestClientCertificate(t, ca, key, 2)
	s := httptest.NewUnstartedServer(handler)
	s.EnableHTTP2 = true
	s.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: x509.NewCertPool()}
	s.TLS.ClientCAs.AddCert(ca)
	s.StartTLS()
	t.Cleanup(s.Close)
	config := boundedExecConfig("cert",
		clientcmdapi.ExecEnvVar{Name: boundedExecHelperFile, Value: filepath.Join(t.TempDir(), "count")},
		clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_CERT1", Value: string(cert1)},
		clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_KEY1", Value: string(key1)},
		clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_CERT2", Value: string(cert2)},
		clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_KEY2", Value: string(key2)})
	config.Host, config.TLSClientConfig.Insecure = s.URL, true
	client, err := newBoundedHTTPClient(config)
	require.NoError(t, err)
	return client, client.Transport.(*boundedExecRoundTripper).auth, s.URL
}

func TestBoundedExecAuthCertificateWithExplicitHeader(t *testing.T) {
	client, _, url := boundedMTLSClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s:%s", r.Header.Get("Authorization"), r.TLS.PeerCertificates[0].SerialNumber)
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer caller")
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "Bearer caller:1", string(body))
}

func TestBoundedExecAuthRotationClosesActiveHTTP2(t *testing.T) {
	closed := make(chan struct{})
	client, auth, url := boundedMTLSClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/stream" {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(closed)
			return
		}
		fmt.Fprint(w, r.TLS.PeerCertificates[0].SerialNumber)
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/stream", nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 2, resp.ProtoMajor)
	creds, err := auth.credentials(ctx, false)
	require.NoError(t, err)
	require.NoError(t, auth.refreshIfCurrent(ctx, creds))
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("certificate rotation left an old-certificate HTTP/2 stream active")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, url+"/next", nil)
	require.NoError(t, err)
	resp2, err := client.Do(req)
	require.NoError(t, err)
	defer resp2.Body.Close()
	body, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, "2", string(body))
}

func TestBoundedExecAuthInteractiveTTYRejected(t *testing.T) {
	terminal, err := os.Open("/dev/tty")
	if err != nil {
		t.Skip("requires a controlling terminal; run with a PTY")
	}
	defer terminal.Close()
	old := os.Stdin
	os.Stdin = terminal
	defer func() { os.Stdin = old }()
	interactive, err := boundedExecInteractive(&clientcmdapi.ExecConfig{InteractiveMode: clientcmdapi.IfAvailableExecInteractiveMode})
	require.NoError(t, err)
	require.False(t, interactive, "bounded helpers must not read a background process group's terminal")
	_, err = boundedExecInteractive(&clientcmdapi.ExecConfig{InteractiveMode: clientcmdapi.AlwaysExecInteractiveMode})
	require.Error(t, err)
}
