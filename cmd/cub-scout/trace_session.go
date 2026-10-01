// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// traceSession pins the Kubernetes endpoint and context label for one rich
// Trace invocation. Its clients are created lazily from the private config.
// Credential files are captured with the session; configured exec plugins
// retain their own refresh behavior. Subprocess binding is a separate stage.
type traceSession struct {
	config   *rest.Config
	context  string
	proxyURL string // Static proxy parsed with this config; empty means no provenance.

	dynamicOnce   sync.Once
	dynamic       dynamic.Interface
	dynamicErr    error
	kubeOnce      sync.Once
	kube          kubernetes.Interface
	kubeErr       error
	discoveryOnce sync.Once
	discovery     discovery.DiscoveryInterface
	discoveryErr  error
}

func newTraceSession(config *rest.Config, contextLabel string) (*traceSession, error) {
	if config == nil {
		return nil, fmt.Errorf("trace session requires a Kubernetes config")
	}
	if config.Host == "" {
		return nil, fmt.Errorf("trace session requires a Kubernetes API server address")
	}
	captured := copyTraceRESTConfig(config)
	if err := captureTraceCredentialFiles(captured); err != nil {
		return nil, err
	}
	return &traceSession{config: captured, context: contextLabel}, nil
}

// newTraceSessionForSelection preserves the legacy default loader when omitted,
// and resolves explicit selections without ambient or in-cluster fallback.
func newTraceSessionForSelection(selection clusterContextSelection) (*traceSession, error) {
	if !selection.explicit {
		return newDefaultTraceSession()
	}
	return newTraceSessionFromBinding(resolveLocalClusterBindingForSelection(selection))
}

func newDefaultTraceSession() (*traceSession, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = home + "/.kube/config"
	}
	config, contextLabel, proxyURL, err := resolveClusterConfigWithProxy("", false, &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}, rest.InClusterConfig)
	if err != nil {
		return nil, err
	}
	if contextLabel == "" {
		contextLabel = "in-cluster"
	}
	session, err := newTraceSession(config, contextLabel)
	if err != nil {
		return nil, err
	}
	session.proxyURL = proxyURL
	return session, nil
}

func copyTraceRESTConfig(config *rest.Config) *rest.Config {
	copied := rest.CopyConfig(config)
	// A client-go auth provider may write refreshed tokens to this persister.
	// Trace is read-only, so a session must not retain kubeconfig write access.
	copied.AuthConfigPersister = nil
	copied.TLSClientConfig.CAData = append([]byte(nil), config.TLSClientConfig.CAData...)
	copied.TLSClientConfig.CertData = append([]byte(nil), config.TLSClientConfig.CertData...)
	copied.TLSClientConfig.KeyData = append([]byte(nil), config.TLSClientConfig.KeyData...)
	copied.Impersonate.Groups = append([]string(nil), config.Impersonate.Groups...)
	if config.Impersonate.Extra != nil {
		copied.Impersonate.Extra = make(map[string][]string, len(config.Impersonate.Extra))
		for key, values := range config.Impersonate.Extra {
			copied.Impersonate.Extra[key] = append([]string(nil), values...)
		}
	}
	if config.ExecProvider != nil {
		copied.ExecProvider = config.ExecProvider.DeepCopy()
	}
	if config.AuthProvider != nil {
		authProvider := *config.AuthProvider
		if config.AuthProvider.Config != nil {
			authProvider.Config = make(map[string]string, len(config.AuthProvider.Config))
			for key, value := range config.AuthProvider.Config {
				authProvider.Config[key] = value
			}
		}
		copied.AuthProvider = &authProvider
	}
	return copied
}

func (s *traceSession) contextLabel() string {
	if s == nil {
		return ""
	}
	return s.context
}

func (s *traceSession) restConfig() (*rest.Config, error) {
	if s == nil || s.config == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	return copyTraceRESTConfig(s.config), nil
}

func (s *traceSession) dynamicClient() (dynamic.Interface, error) {
	if s == nil || s.config == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	s.dynamicOnce.Do(func() { s.dynamic, s.dynamicErr = dynamic.NewForConfig(s.config) })
	return s.dynamic, s.dynamicErr
}

func (s *traceSession) kubernetesClient() (kubernetes.Interface, error) {
	if s == nil || s.config == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	s.kubeOnce.Do(func() { s.kube, s.kubeErr = kubernetes.NewForConfig(s.config) })
	return s.kube, s.kubeErr
}

func (s *traceSession) discoveryClient() (discovery.DiscoveryInterface, error) {
	if s == nil || s.config == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	s.discoveryOnce.Do(func() { s.discovery, s.discoveryErr = discovery.NewDiscoveryClientForConfig(s.config) })
	return s.discovery, s.discoveryErr
}

func fetchResourceEventsWithTraceSession(ctx context.Context, session *traceSession, namespace, kind, name string) (*agent.ResourceEventSummary, error) {
	if session == nil {
		return nil, fmt.Errorf("trace session is unavailable")
	}
	client, err := session.kubernetesClient()
	if err != nil {
		return nil, err
	}
	return agent.NewEventTimelineFetcher(client).FetchRecentEvents(ctx, namespace, kind, name, 5)
}

func collectSecretEvidenceWithTraceSession(ctx context.Context, session *traceSession, kind, name, namespace string) *agent.SecretEvidenceResult {
	result, _ := collectSecretEvidenceWithTraceSessionAndError(ctx, session, kind, name, namespace)
	return result
}

// collectSecretEvidenceWithTraceSessionAndError preserves the legacy
// map-only behavior through its wrapper while allowing the rich observer to
// distinguish a failed read from a resource with no secret references.
func collectSecretEvidenceWithTraceSessionAndError(ctx context.Context, session *traceSession, kind, name, namespace string) (*agent.SecretEvidenceResult, error) {
	supportedKinds := map[string]bool{
		"Deployment": true, "StatefulSet": true, "DaemonSet": true, "Pod": true,
		"GitRepository": true, "HelmRepository": true, "Bucket": true,
		"Kustomization": true, "HelmRelease": true, "ProviderConfig": true,
	}
	if !supportedKinds[kind] {
		return nil, nil
	}
	if session == nil {
		return nil, fmt.Errorf("secret evidence unavailable for %s/%s in %s: trace session is unavailable", kind, name, namespace)
	}
	dynClient, err := session.dynamicClient()
	if err != nil {
		return nil, fmt.Errorf("secret evidence unavailable for %s/%s in %s: %w", kind, name, namespace, err)
	}
	var resource *unstructured.Unstructured
	if kind == "ProviderConfig" {
		resource, err = fetchProviderConfigResourceWithTraceSession(ctx, session, dynClient, name, namespace)
	} else {
		gvr := kindToGVR(kind)
		if gvr.Resource == "" {
			return nil, nil
		}
		resource, err = dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("secret evidence unavailable for %s/%s in %s: %w", kind, name, namespace, err)
	}
	result, err := agent.NewSecretEvidenceCollector(dynClient).CollectFromResource(ctx, resource)
	if err != nil {
		return nil, fmt.Errorf("secret evidence unavailable for %s/%s in %s: %w", kind, name, namespace, err)
	}
	return result, nil
}

func collectTraceArtifactsWithTraceSession(ctx context.Context, session *traceSession, result *agent.TraceResult) map[string]mapsvc.TraceArtifactRef {
	artifacts, _ := collectTraceArtifactsWithTraceSessionAndErrors(ctx, session, result)
	return artifacts
}

// collectTraceArtifactsWithTraceSessionAndErrors retains successful artifact
// metadata alongside source-specific read failures for observer warnings.
func collectTraceArtifactsWithTraceSessionAndErrors(ctx context.Context, session *traceSession, result *agent.TraceResult) (map[string]mapsvc.TraceArtifactRef, []error) {
	artifacts := make(map[string]mapsvc.TraceArtifactRef)
	if result == nil || len(result.Chain) == 0 {
		return artifacts, nil
	}
	sources := make([]agent.ChainLink, 0, 4)
	for _, link := range result.Chain {
		if isTraceSourceKind(link.Kind) && kindToGVR(link.Kind).Resource != "" {
			sources = append(sources, link)
		}
	}
	if len(sources) == 0 {
		return artifacts, nil
	}
	var readErrors []error
	readableSources := make([]agent.ChainLink, 0, len(sources))
	for _, source := range sources {
		if err := validateTraceArtifactIdentity(source); err != nil {
			readErrors = append(readErrors, err)
			continue
		}
		readableSources = append(readableSources, source)
	}
	if len(readableSources) == 0 {
		return artifacts, readErrors
	}
	if session == nil {
		for _, source := range readableSources {
			readErrors = append(readErrors, fmt.Errorf("artifact metadata unavailable for %s/%s/%s: trace session is unavailable", source.Kind, source.Namespace, source.Name))
		}
		return artifacts, readErrors
	}
	dynClient, err := session.dynamicClient()
	if err != nil {
		for _, source := range readableSources {
			readErrors = append(readErrors, fmt.Errorf("artifact metadata unavailable for %s/%s/%s: %w", source.Kind, source.Namespace, source.Name, err))
		}
		return artifacts, readErrors
	}
	for _, source := range readableSources {
		gvr := kindToGVR(source.Kind)
		if gvr.Resource == "" {
			continue
		}
		obj, err := dynClient.Resource(gvr).Namespace(source.Namespace).Get(ctx, source.Name, v1.GetOptions{})
		if err != nil {
			readErrors = append(readErrors, fmt.Errorf("artifact metadata unavailable for %s/%s/%s: %w", source.Kind, source.Namespace, source.Name, err))
			continue
		}
		artifact := traceArtifactUnknownForKind(source.Kind)
		if v, ok, _ := unstructured.NestedString(obj.Object, "status", "artifact", "url"); ok && strings.TrimSpace(v) != "" {
			artifact.URL = v
		}
		if v, ok, _ := unstructured.NestedString(obj.Object, "status", "artifact", "revision"); ok && strings.TrimSpace(v) != "" {
			artifact.Revision = v
		}
		if v, ok, _ := unstructured.NestedString(obj.Object, "status", "artifact", "digest"); ok && strings.TrimSpace(v) != "" {
			artifact.Digest = v
		}
		if v, ok, _ := unstructured.NestedString(obj.Object, "status", "artifact", "lastUpdateTime"); ok && strings.TrimSpace(v) != "" {
			artifact.LastUpdateTime = v
		}
		artifacts[traceArtifactKey(source.Kind, source.Namespace, source.Name)] = normalizeTraceArtifact(source.Kind, artifact)
	}
	return artifacts, readErrors
}

func validateTraceArtifactIdentity(source agent.ChainLink) error {
	reason := ""
	switch {
	case strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Name) != source.Name || len(validation.IsDNS1123Subdomain(source.Name)) != 0:
		reason = "exact Kubernetes source name is missing or invalid"
	case strings.TrimSpace(source.Namespace) == "":
		reason = "exact Kubernetes source namespace is required"
	case strings.TrimSpace(source.Namespace) != source.Namespace || len(validation.IsDNS1123Label(source.Namespace)) != 0:
		reason = "exact Kubernetes source namespace is invalid"
	}
	if reason == "" {
		return nil
	}
	return fmt.Errorf("artifact metadata unavailable for %s/%s/%s: %s", source.Kind, source.Namespace, source.Name, reason)
}

// newTraceSessionFromBinding preserves the same parsed proxy provenance used by
// the selected inventory binding; it does not reread a context or kubeconfig.
func newTraceSessionFromBinding(binding *localClusterBinding) (*traceSession, error) {
	if binding == nil {
		return nil, fmt.Errorf("trace cluster binding is unavailable")
	}
	if binding.err != nil {
		return nil, binding.err
	}
	session, err := newTraceSession(binding.config, binding.context)
	if err != nil {
		return nil, err
	}
	session.proxyURL = binding.proxyURL
	return session, nil
}
