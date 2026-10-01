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
	config  *rest.Config
	context string

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

func newDefaultTraceSession() (*traceSession, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = home + "/.kube/config"
	}
	config, contextLabel, err := resolveClusterConfig("", false, &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}, rest.InClusterConfig)
	if err != nil {
		return nil, err
	}
	if contextLabel == "" {
		contextLabel = "in-cluster"
	}
	return newTraceSession(config, contextLabel)
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
	supportedKinds := map[string]bool{
		"Deployment": true, "StatefulSet": true, "DaemonSet": true, "Pod": true,
		"GitRepository": true, "HelmRepository": true, "Bucket": true,
		"Kustomization": true, "HelmRelease": true, "ProviderConfig": true,
	}
	if !supportedKinds[kind] || session == nil {
		return nil
	}
	dynClient, err := session.dynamicClient()
	if err != nil {
		return nil
	}
	var resource *unstructured.Unstructured
	if kind == "ProviderConfig" {
		resource, err = fetchProviderConfigResourceWithTraceSession(ctx, session, dynClient, name, namespace)
	} else {
		gvr := kindToGVR(kind)
		if gvr.Resource == "" {
			return nil
		}
		resource, err = dynClient.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
	}
	if err != nil {
		return nil
	}
	result, err := agent.NewSecretEvidenceCollector(dynClient).CollectFromResource(ctx, resource)
	if err != nil {
		return nil
	}
	return result
}

func collectTraceArtifactsWithTraceSession(ctx context.Context, session *traceSession, result *agent.TraceResult) map[string]mapsvc.TraceArtifactRef {
	artifacts := make(map[string]mapsvc.TraceArtifactRef)
	if result == nil || len(result.Chain) == 0 || session == nil {
		return artifacts
	}
	sources := make([]agent.ChainLink, 0, 4)
	for _, link := range result.Chain {
		if isTraceSourceKind(link.Kind) {
			sources = append(sources, link)
		}
	}
	if len(sources) == 0 {
		return artifacts
	}
	dynClient, err := session.dynamicClient()
	if err != nil {
		return artifacts
	}
	for _, source := range sources {
		gvr := kindToGVR(source.Kind)
		if gvr.Resource == "" {
			continue
		}
		obj, err := dynClient.Resource(gvr).Namespace(source.Namespace).Get(ctx, source.Name, v1.GetOptions{})
		if err != nil {
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
	return artifacts
}
