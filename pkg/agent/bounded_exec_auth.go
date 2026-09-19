// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	osexec "os/exec"
	"reflect"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	clientauthentication "k8s.io/client-go/pkg/apis/clientauthentication"
	clientauthenticationinstall "k8s.io/client-go/pkg/apis/clientauthentication/install"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ktransport "k8s.io/client-go/transport"
	"k8s.io/client-go/util/connrotation"
)

const (
	boundedExecInfoEnv     = "KUBERNETES_EXEC_INFO"
	boundedExecWaitDelay   = time.Second
	boundedExecOutputLimit = 1 << 20
)

var boundedExecCodecs = func() serializer.CodecFactory {
	scheme := runtime.NewScheme()
	clientauthenticationinstall.Install(scheme)
	return serializer.NewCodecFactory(scheme)
}()

// boundedExecCredentials is deliberately kept private: credential values must
// never become part of bounded-read evidence or diagnostic output.
type boundedExecCredentials struct {
	token string           `datapolicy:"token"`
	cert  *tls.Certificate `datapolicy:"secret-key"`
	exp   time.Time
}

type boundedExecAuth struct {
	config  *clientcmdapi.ExecConfig
	group   schema.GroupVersion
	cluster *clientauthentication.Cluster

	// gate makes waiting for a helper cancellable and ensures only one helper
	// runs at a time. The request context is retained through the whole helper
	// lifecycle, including process reaping.
	gate chan struct{}

	mu               sync.RWMutex
	cached           *boundedExecCredentials
	closeConnections func()
}

type boundedExecRoundTripper struct {
	auth *boundedExecAuth
	base http.RoundTripper
}

type boundedExecOutput struct {
	buffer    bytes.Buffer
	oversized bool
	cancel    func()
}

func (w *boundedExecOutput) Write(p []byte) (int, error) {
	if int64(w.buffer.Len())+int64(len(p)) > boundedExecOutputLimit {
		w.oversized = true
		if w.cancel != nil {
			w.cancel()
		}
		return 0, errors.New("exec credential output exceeds bounded limit")
	}
	return w.buffer.Write(p)
}

func newBoundedExecAuth(config *clientcmdapi.ExecConfig, cluster *clientauthentication.Cluster) (*boundedExecAuth, error) {
	if config == nil {
		return nil, errors.New("bounded exec auth requires an exec configuration")
	}
	group, err := schema.ParseGroupVersion(config.APIVersion)
	if err != nil || (config.APIVersion != "client.authentication.k8s.io/v1" && config.APIVersion != "client.authentication.k8s.io/v1beta1") {
		return nil, fmt.Errorf("exec plugin: invalid apiVersion %q", config.APIVersion)
	}
	return &boundedExecAuth{
		config:  config.DeepCopy(),
		group:   group,
		cluster: cluster,
		gate:    make(chan struct{}, 1),
	}, nil
}

func (r *boundedExecRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	// Certificates must be loaded before TLS even with a caller-supplied header.
	creds, err := r.auth.credentials(req.Context(), false)
	if err != nil {
		return nil, fmt.Errorf("bounded exec auth: %w", err)
	}
	request := req.Clone(req.Context())
	if creds.token != "" && request.Header.Get("Authorization") == "" {
		request.Header.Set("Authorization", "Bearer "+creds.token)
	}
	response, err := r.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusUnauthorized {
		return response, nil
	}

	if err := r.auth.refreshIfCurrent(req.Context(), creds); err != nil && req.Context().Err() != nil {
		response.Body.Close()
		return nil, req.Context().Err()
	}
	return response, nil
}

func (a *boundedExecAuth) credentials(ctx context.Context, force bool) (*boundedExecCredentials, error) {
	if err := acquireBoundedExec(ctx, a.gate); err != nil {
		return nil, err
	}
	defer releaseBoundedExec(a.gate)

	a.mu.RLock()
	cached := a.cached
	a.mu.RUnlock()
	if !force && cached != nil && !boundedExecCredentialsExpired(cached) {
		return cached, nil
	}
	return a.refreshLocked(ctx)
}

func (a *boundedExecAuth) refreshIfCurrent(ctx context.Context, current *boundedExecCredentials) error {
	if err := acquireBoundedExec(ctx, a.gate); err != nil {
		return err
	}
	defer releaseBoundedExec(a.gate)

	a.mu.RLock()
	cached := a.cached
	a.mu.RUnlock()
	if cached != current {
		return nil
	}
	_, err := a.refreshLocked(ctx)
	return err
}

// refreshLocked runs while the gate is held. The process itself is canceled by
// exec.Cmd's context watcher, and the platform hook kills its process group so
// a helper child cannot keep stdout/stderr pipes alive after cancellation.
func (a *boundedExecAuth) refreshLocked(ctx context.Context) (*boundedExecCredentials, error) {
	interactive, err := boundedExecInteractive(a.config)
	if err != nil {
		return nil, err
	}

	credential := &clientauthentication.ExecCredential{
		Spec: clientauthentication.ExecCredentialSpec{Interactive: interactive},
	}
	if a.config.ProvideClusterInfo {
		credential.Spec.Cluster = a.cluster
	}
	data, err := runtime.Encode(boundedExecCodecs.LegacyCodec(a.group), credential)
	if err != nil {
		return nil, fmt.Errorf("encode ExecCredentials: %v", err)
	}

	env := append([]string(nil), os.Environ()...)
	for _, item := range a.config.Env {
		env = append(env, item.Name+"="+item.Value)
	}
	env = append(env, boundedExecInfoEnv+"="+string(data))

	stdout := &boundedExecOutput{}
	cmd := osexec.CommandContext(ctx, a.config.Command, a.config.Args...)
	cmd.Env = env
	// Helper diagnostics can contain tokens or certificate material. They are
	// intentionally discarded rather than attached to a bounded-read error.
	cmd.Stderr = io.Discard
	cmd.Stdout = stdout
	prepareBoundedExecCommand(cmd)
	cmd.Cancel = func() error { return cancelBoundedExecCommand(cmd) }
	cmd.WaitDelay = boundedExecWaitDelay
	stdout.cancel = func() { _ = cancelBoundedExecCommand(cmd) }

	if err := cmd.Run(); err != nil {
		if stdout.oversized {
			return nil, errors.New("exec credential output exceeds bounded limit")
		}
		if errors.Is(err, osexec.ErrWaitDelay) {
			// The parent may have exited while a descendant still owns one of
			// the output pipes. CommandContext's Cancel is not called in that
			// case, so close the Unix process group explicitly before returning.
			_ = cancelBoundedExecCommand(cmd)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, boundedExecCommandError(err)
	}
	if stdout.oversized {
		return nil, errors.New("exec credential output exceeds bounded limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	_, gvk, err := boundedExecCodecs.UniversalDecoder(a.group).Decode(stdout.buffer.Bytes(), nil, credential)
	if err != nil {
		return nil, errors.New("decoding exec credential stdout")
	}
	if gvk.Group != a.group.Group || gvk.Version != a.group.Version {
		return nil, fmt.Errorf("exec plugin returned API version %s, expected %s", schema.GroupVersion{Group: gvk.Group, Version: gvk.Version}, a.group)
	}
	status := credential.Status
	if status == nil {
		return nil, errors.New("exec plugin did not return credentials")
	}
	if status.Token == "" && status.ClientCertificateData == "" && status.ClientKeyData == "" {
		return nil, errors.New("exec plugin did not return a token or certificate pair")
	}
	if (status.ClientCertificateData == "") != (status.ClientKeyData == "") {
		return nil, errors.New("exec plugin returned only one half of a certificate pair")
	}

	credentials := &boundedExecCredentials{token: status.Token}
	if status.ExpirationTimestamp != nil {
		credentials.exp = status.ExpirationTimestamp.Time
	}
	if status.ClientCertificateData != "" {
		cert, err := tls.X509KeyPair([]byte(status.ClientCertificateData), []byte(status.ClientKeyData))
		if err != nil {
			return nil, errors.New("exec plugin returned an invalid certificate pair")
		}
		cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, errors.New("exec plugin returned an invalid client certificate")
		}
		credentials.cert = &cert
	}

	a.mu.Lock()
	old := a.cached
	a.cached = credentials
	a.mu.Unlock()
	if old != nil && !reflect.DeepEqual(old.cert, credentials.cert) && a.closeConnections != nil {
		a.closeConnections()
	}
	return credentials, nil
}

func boundedExecCredentialsExpired(credentials *boundedExecCredentials) bool {
	return !credentials.exp.IsZero() && time.Now().After(credentials.exp)
}

func (a *boundedExecAuth) cachedCertificate() (*tls.Certificate, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cached == nil {
		return nil, nil
	}
	return a.cached.cert, nil
}

func boundedExecInteractive(config *clientcmdapi.ExecConfig) (bool, error) {
	switch config.InteractiveMode {
	case clientcmdapi.NeverExecInteractiveMode, clientcmdapi.IfAvailableExecInteractiveMode:
		return false, nil
	case clientcmdapi.AlwaysExecInteractiveMode:
		return false, errors.New("bounded reads require non-interactive exec credentials; authenticate separately before observing")
	default:
		return false, fmt.Errorf("exec plugin cannot use interactive mode: unknown interactiveMode: %q", config.InteractiveMode)
	}
}

func boundedExecCommandError(err error) error {
	var notFound *osexec.Error
	if errors.As(err, &notFound) {
		return errors.New("exec credential helper was not found")
	}
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("exec credential helper failed with exit code %d", exitErr.ProcessState.ExitCode())
	}
	return errors.New("exec credential helper failed")
}

func acquireBoundedExec(ctx context.Context, gate chan struct{}) error {
	select {
	case gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseBoundedExec(gate chan struct{}) { <-gate }

// newBoundedHTTPClient keeps client-go's transport construction, including
// static credentials and configured wrappers, while removing its unbounded
// exec authenticator from this reader's transport.
func newBoundedHTTPClient(config *rest.Config) (*http.Client, error) {
	clientConfig := dynamic.ConfigFor(rest.CopyConfig(config))
	var auth *boundedExecAuth
	if clientConfig.ExecProvider != nil {
		execConfig := clientConfig.ExecProvider.DeepCopy()
		if clientConfig.AuthProvider != nil {
			return nil, errors.New("execProvider and authProvider cannot be used in combination")
		}
		cluster, err := rest.ConfigToExecCluster(clientConfig)
		if err != nil {
			return nil, err
		}
		// TransportConfig constructs client-go's native exec authenticator while
		// ExecProvider is set. Clear it before constructing the base transport so
		// an unbounded helper cannot be installed underneath this wrapper.
		clientConfig.ExecProvider = nil
		transportConfig, err := clientConfig.TransportConfig()
		if err != nil {
			return nil, err
		}
		if !transportConfig.HasTokenAuth() && !transportConfig.HasBasicAuth() && !transportConfig.HasCertAuth() {
			auth, err = newBoundedExecAuth(execConfig, cluster)
			if err != nil {
				return nil, err
			}
			transportConfig.TLS.GetCertHolder = &ktransport.GetCertHolder{GetCert: auth.cachedCertificate}
			dial := (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
			if transportConfig.DialHolder != nil {
				if transportConfig.DialHolder.Dial == nil {
					return nil, errors.New("bounded exec auth: configured dialer is nil")
				}
				dial = transportConfig.DialHolder.Dial
			}
			tracker := connrotation.NewDialer(dial)
			transportConfig.DialHolder = &ktransport.DialHolder{Dial: tracker.DialContext}
			auth.closeConnections = tracker.CloseAll
			base, err := ktransport.New(transportConfig)
			if err != nil {
				return nil, err
			}
			roundTripper := &boundedExecRoundTripper{auth: auth, base: base}
			return &http.Client{Transport: roundTripper, Timeout: clientConfig.Timeout}, nil
		}
		// Match client-go: explicit static credentials take precedence over exec.
	}
	httpClient, err := rest.HTTPClientFor(clientConfig)
	if err != nil {
		return nil, err
	}
	return httpClient, nil
}
