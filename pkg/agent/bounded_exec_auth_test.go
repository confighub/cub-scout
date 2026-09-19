// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	boundedExecHelperEnv      = "CUB_SCOUT_BOUNDED_EXEC_HELPER"
	boundedExecHelperMode     = "CUB_SCOUT_BOUNDED_EXEC_MODE"
	boundedExecHelperFile     = "CUB_SCOUT_BOUNDED_EXEC_FILE"
	boundedExecHelperChild    = "CUB_SCOUT_BOUNDED_EXEC_CHILD_FILE"
	boundedExecHelperToken    = "CUB_SCOUT_BOUNDED_EXEC_TOKEN"
	boundedExecHelperSentinel = "bounded-exec-secret-sentinel"
)

// TestBoundedExecAuthHelperProcess is a subprocess fixture. Keeping it in the
// test binary makes the lifecycle tests independent of shell quoting and PATH.
func TestBoundedExecAuthHelperProcess(t *testing.T) {
	if os.Getenv(boundedExecHelperEnv) != "1" {
		return
	}
	mode := os.Getenv(boundedExecHelperMode)
	switch mode {
	case "token":
		count := incrementHelperFile(os.Getenv(boundedExecHelperFile))
		token := os.Getenv(boundedExecHelperToken)
		if token == "" {
			token = "bounded-token-" + strconv.Itoa(count)
		}
		if os.Getenv("CUB_SCOUT_BOUNDED_EXEC_NO_EXPIRATION") == "1" {
			fmt.Printf(`{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{"token":%q}}`, token)
		} else {
			fmt.Printf(`{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{"token":%q,"expirationTimestamp":"2099-01-01T00:00:00Z"}}`, token)
		}
	case "malformed":
		switch os.Getenv("CUB_SCOUT_BOUNDED_EXEC_MALFORMED") {
		case "empty":
			fmt.Print(`{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{}}`)
		case "half-cert":
			fmt.Print(`{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{"clientCertificateData":"only-one"}}`)
		case "bad-cert":
			fmt.Print(`{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{"clientCertificateData":"bad","clientKeyData":"bad"}}`)
		case "wrong-version":
			fmt.Print(`{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"must-not-use"}}`)
		default:
			fmt.Print("not-json")
		}
	case "stderr":
		fmt.Fprint(os.Stderr, boundedExecHelperSentinel)
		fmt.Print(`{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{}}`)
	case "oversized":
		fmt.Print(strings.Repeat("x", boundedExecOutputLimit+1))
	case "hang":
		writeHelperFile(os.Getenv(boundedExecHelperFile), strconv.Itoa(os.Getpid()))
		if childFile := os.Getenv(boundedExecHelperChild); childFile != "" {
			child := exec.Command(os.Args[0], "-test.run=TestBoundedExecAuthHelperProcess")
			child.Env = append(os.Environ(), boundedExecHelperEnv+"=1", boundedExecHelperMode+"=child", boundedExecHelperFile+"="+childFile)
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			if err := child.Start(); err != nil {
				os.Exit(91)
			}
		}
		for {
			time.Sleep(time.Hour)
		}
	case "exit-child":
		childFile := os.Getenv(boundedExecHelperChild)
		child := exec.Command(os.Args[0], "-test.run=TestBoundedExecAuthHelperProcess")
		child.Env = append(os.Environ(), boundedExecHelperEnv+"=1", boundedExecHelperMode+"=child", boundedExecHelperFile+"="+childFile)
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(92)
		}
		fmt.Print(`{"apiVersion":"client.authentication.k8s.io/v1beta1","kind":"ExecCredential","status":{"token":"bounded-token"}}`)
	case "child":
		writeHelperFile(os.Getenv(boundedExecHelperFile), strconv.Itoa(os.Getpid()))
		for {
			time.Sleep(time.Hour)
		}
	case "cert":
		count := incrementHelperFile(os.Getenv(boundedExecHelperFile))
		certPEM, keyPEM := []byte(os.Getenv("CUB_SCOUT_BOUNDED_CERT1")), []byte(os.Getenv("CUB_SCOUT_BOUNDED_KEY1"))
		if count > 1 {
			certPEM, keyPEM = []byte(os.Getenv("CUB_SCOUT_BOUNDED_CERT2")), []byte(os.Getenv("CUB_SCOUT_BOUNDED_KEY2"))
		}
		output, _ := json.Marshal(map[string]any{
			"apiVersion": "client.authentication.k8s.io/v1beta1",
			"kind":       "ExecCredential",
			"status": map[string]string{
				"clientCertificateData": string(certPEM),
				"clientKeyData":         string(keyPEM),
			},
		})
		os.Stdout.Write(output)
	}
	if mode != "hang" && mode != "child" {
		os.Exit(0)
	}
}

func writeHelperFile(path, value string) {
	if path != "" {
		_ = os.WriteFile(path, []byte(value), 0o600)
	}
}

func incrementHelperFile(path string) int {
	if path == "" {
		return 1
	}
	data, _ := os.ReadFile(path)
	count, _ := strconv.Atoi(string(data))
	count++
	writeHelperFile(path, strconv.Itoa(count))
	return count
}

func boundedExecFixtureServer(t *testing.T, checkAuth func(string)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkAuth(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1":
			fmt.Fprint(w, boundedDiscoveryFixture)
		case "/apis/apps/v1/namespaces/team-a/deployments/api":
			fmt.Fprint(w, boundedObjectFixture("team-a"))
		default:
			http.NotFound(w, r)
		}
	}))
}

func boundedExecConfig(mode string, env ...clientcmdapi.ExecEnvVar) *rest.Config {
	env = append([]clientcmdapi.ExecEnvVar{
		{Name: boundedExecHelperEnv, Value: "1"},
		{Name: boundedExecHelperMode, Value: mode},
	}, env...)
	return &rest.Config{
		Host: "http://127.0.0.1",
		ExecProvider: &clientcmdapi.ExecConfig{
			Command:         os.Args[0],
			Args:            []string{"-test.run=TestBoundedExecAuthHelperProcess"},
			Env:             env,
			APIVersion:      "client.authentication.k8s.io/v1beta1",
			InteractiveMode: clientcmdapi.NeverExecInteractiveMode,
		},
	}
}

func boundedExecReader(t *testing.T, serverURL, mode string, env ...clientcmdapi.ExecEnvVar) *BoundedResourceReader {
	t.Helper()
	config := boundedExecConfig(mode, env...)
	config.Host = serverURL
	reader, err := NewBoundedResourceReader(config, "exec-test")
	require.NoError(t, err)
	return reader
}

func boundedExecRef() BoundedResourceRef {
	return BoundedResourceRef{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "team-a", Name: "api"}
}

func TestBoundedExecAuthValidToken(t *testing.T) {
	var auth atomic.Value
	server := boundedExecFixtureServer(t, func(value string) { auth.Store(value) })
	defer server.Close()
	reader := boundedExecReader(t, server.URL, "token", clientcmdapi.ExecEnvVar{Name: boundedExecHelperToken, Value: "exec-token"})
	_, _, err := reader.Read(context.Background(), boundedExecRef(), false)
	require.NoError(t, err)
	require.Equal(t, "Bearer exec-token", auth.Load())
}

func TestBoundedExecAuthCachesNonExpiringCredentials(t *testing.T) {
	countFile := filepath.Join(t.TempDir(), "helper-count")
	server := boundedExecFixtureServer(t, func(string) {})
	defer server.Close()
	reader := boundedExecReader(t, server.URL, "token",
		clientcmdapi.ExecEnvVar{Name: boundedExecHelperFile, Value: countFile},
		clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_EXEC_NO_EXPIRATION", Value: "1"})
	_, _, err := reader.Read(context.Background(), boundedExecRef(), false)
	require.NoError(t, err)
	_, _, err = reader.Read(context.Background(), boundedExecRef(), false)
	require.NoError(t, err)
	data, err := os.ReadFile(countFile)
	require.NoError(t, err)
	require.Equal(t, "1", strings.TrimSpace(string(data)))
}

func TestBoundedExecAuthMalformedCredentialsFailClosed(t *testing.T) {
	var requests atomic.Int32
	server := boundedExecFixtureServer(t, func(string) { requests.Add(1) })
	defer server.Close()
	for _, malformed := range []string{"json", "wrong-version", "empty", "half-cert", "bad-cert"} {
		t.Run(malformed, func(t *testing.T) {
			reader := boundedExecReader(t, server.URL, "malformed", clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_EXEC_MALFORMED", Value: malformed})
			_, _, err := reader.Read(context.Background(), boundedExecRef(), false)
			require.Error(t, err)
		})
	}
	require.Zero(t, requests.Load())
}

func TestBoundedExecAuthNoCredentialLeakage(t *testing.T) {
	server := boundedExecFixtureServer(t, func(string) { t.Error("server must not be called") })
	defer server.Close()
	reader := boundedExecReader(t, server.URL, "stderr")
	_, evidence, err := reader.Read(context.Background(), boundedExecRef(), false)
	require.Error(t, err)
	require.NotContains(t, err.Error(), boundedExecHelperSentinel)
	require.NotContains(t, fmt.Sprintf("%+v", evidence), boundedExecHelperSentinel)
}

func TestBoundedExecAuthOversizedOutput(t *testing.T) {
	var requests atomic.Int32
	server := boundedExecFixtureServer(t, func(string) { requests.Add(1) })
	defer server.Close()
	reader := boundedExecReader(t, server.URL, "oversized")
	_, _, err := reader.Read(context.Background(), boundedExecRef(), false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds bounded limit")
	require.Zero(t, requests.Load())
}

func TestBoundedExecAuthStaticCredentialsTakePrecedence(t *testing.T) {
	var requests atomic.Int32
	server := boundedExecFixtureServer(t, func(auth string) {
		requests.Add(1)
		if auth != "Bearer static-token" {
			t.Errorf("authorization = %q", auth)
		}
	})
	defer server.Close()
	config := boundedExecConfig("hang")
	config.Host = server.URL
	config.BearerToken = "static-token"
	config.ExecProvider.Command = "bounded-helper-must-not-run"
	reader, err := NewBoundedResourceReader(config, "static-test")
	require.NoError(t, err)
	_, _, err = reader.Read(context.Background(), boundedExecRef(), false)
	require.NoError(t, err)
	require.EqualValues(t, 2, requests.Load())
}

func TestBoundedExecAuthRefreshesAfterUnauthorized(t *testing.T) {
	countFile := filepath.Join(t.TempDir(), "helper-count")
	var requests atomic.Int32
	var seen sync.Mutex
	var authValues []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.Header.Get("Authorization")
		seen.Lock()
		authValues = append(authValues, value)
		seen.Unlock()
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","code":401}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/apis/apps/v1" {
			fmt.Fprint(w, boundedDiscoveryFixture)
		} else {
			fmt.Fprint(w, boundedObjectFixture("team-a"))
		}
	}))
	defer server.Close()
	reader := boundedExecReader(t, server.URL, "token", clientcmdapi.ExecEnvVar{Name: boundedExecHelperFile, Value: countFile})
	_, _, firstErr := reader.Read(context.Background(), boundedExecRef(), false)
	require.Error(t, firstErr)
	_, _, secondErr := reader.Read(context.Background(), boundedExecRef(), false)
	require.NoError(t, secondErr)
	seen.Lock()
	defer seen.Unlock()
	require.GreaterOrEqual(t, len(authValues), 3)
	require.Equal(t, "Bearer bounded-token-1", authValues[0])
	for _, value := range authValues[1:] {
		require.Equal(t, "Bearer bounded-token-2", value)
	}
}

func TestBoundedExecAuthHungHelperCancellationReapsProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fallback guarantees direct-helper cleanup, not descendant process-group cleanup")
	}
	parentFile := filepath.Join(t.TempDir(), "parent-pid")
	childFile := filepath.Join(t.TempDir(), "child-pid")
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("server must not be called") }))
	defer server.Close()
	reader := boundedExecReader(t, server.URL, "hang",
		clientcmdapi.ExecEnvVar{Name: boundedExecHelperFile, Value: parentFile},
		clientcmdapi.ExecEnvVar{Name: boundedExecHelperChild, Value: childFile})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, _, err := reader.Read(ctx, boundedExecRef(), false); result <- err }()
	require.Eventually(t, func() bool { return helperFileExists(parentFile) && helperFileExists(childFile) }, 2*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("bounded read did not return after helper cancellation")
	}
	parentPID := readHelperPID(t, parentFile)
	childPID := readHelperPID(t, childFile)
	require.Eventually(t, func() bool { return !processExists(parentPID) && !processExists(childPID) }, 2*time.Second, 10*time.Millisecond)
}

func TestBoundedExecAuthHelperTimeoutReapsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows process-tree cleanup is an explicit weaker guarantee")
	}
	pidFile := filepath.Join(t.TempDir(), "pid")
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("server must not be called") }))
	defer server.Close()
	reader := boundedExecReader(t, server.URL, "hang", clientcmdapi.ExecEnvVar{Name: boundedExecHelperFile, Value: pidFile})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, err := reader.Read(ctx, boundedExecRef(), false)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Eventually(t, func() bool { return helperFileExists(pidFile) && !processExists(readHelperPID(t, pidFile)) }, 2*time.Second, 10*time.Millisecond)
}

func TestBoundedExecAuthParentExitCleansInheritedStdoutChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fallback does not guarantee descendant process-tree cleanup")
	}
	parentFile := filepath.Join(t.TempDir(), "parent-pid")
	childFile := filepath.Join(t.TempDir(), "child-pid")
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("server must not be called") }))
	defer server.Close()
	reader := boundedExecReader(t, server.URL, "exit-child",
		clientcmdapi.ExecEnvVar{Name: boundedExecHelperFile, Value: parentFile},
		clientcmdapi.ExecEnvVar{Name: boundedExecHelperChild, Value: childFile})
	_, _, err := reader.Read(context.Background(), boundedExecRef(), false)
	require.Error(t, err)
	require.Eventually(t, func() bool { return helperFileExists(childFile) && !processExists(readHelperPID(t, childFile)) }, 3*time.Second, 20*time.Millisecond)
}

func helperFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readHelperPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return pid
}

func processExists(pid int) bool {
	return boundedExecProcessExists(pid)
}

func TestBoundedExecAuthCertificateRotation(t *testing.T) {
	caCert, caKey := newTestCA(t)
	cert1, key1 := newTestClientCertificate(t, caCert, caKey, 1)
	cert2, key2 := newTestClientCertificate(t, caCert, caKey, 2)
	var serials sync.Mutex
	var seen []*big.Int
	var requestCount atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) > 0 {
			serials.Lock()
			seen = append(seen, new(big.Int).Set(r.TLS.PeerCertificates[0].SerialNumber))
			serials.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		if requestCount.Add(1) == 3 {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","code":401}`)
			return
		}
		if r.URL.Path == "/apis/apps/v1" {
			fmt.Fprint(w, boundedDiscoveryFixture)
		} else {
			fmt.Fprint(w, boundedObjectFixture("team-a"))
		}
	}))
	server.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: x509.NewCertPool()}
	server.TLS.ClientCAs.AddCert(caCert)
	server.StartTLS()
	defer server.Close()
	countFile := filepath.Join(t.TempDir(), "cert-count")
	config := boundedExecConfig("cert",
		clientcmdapi.ExecEnvVar{Name: boundedExecHelperFile, Value: countFile},
		clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_CERT1", Value: string(cert1)},
		clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_KEY1", Value: string(key1)},
		clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_CERT2", Value: string(cert2)},
		clientcmdapi.ExecEnvVar{Name: "CUB_SCOUT_BOUNDED_KEY2", Value: string(key2)})
	config.Host = server.URL
	config.TLSClientConfig.Insecure = true
	reader, err := NewBoundedResourceReader(config, "mtls-test")
	require.NoError(t, err)
	_, _, err = reader.Read(context.Background(), boundedExecRef(), false)
	require.NoError(t, err)
	// The next discovery request receives 401. The bounded exec transport
	// refreshes the helper credentials, and the following read uses cert2.
	_, _, err = reader.Read(context.Background(), boundedExecRef(), true)
	require.Error(t, err)
	_, _, err = reader.Read(context.Background(), boundedExecRef(), false)
	require.NoError(t, err)
	countData, err := os.ReadFile(countFile)
	require.NoError(t, err)
	require.Equal(t, "2", strings.TrimSpace(string(countData)))
	serials.Lock()
	defer serials.Unlock()
	require.NotEmpty(t, seen)
	require.Equal(t, big.NewInt(1), seen[0])
	require.Equal(t, big.NewInt(2), seen[len(seen)-1])
}

func newTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(100), Subject: pkix.Name{CommonName: "bounded-test-ca"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return cert, key
}

func newTestClientCertificate(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, serial int64) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "bounded-client"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return certPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}
