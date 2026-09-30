// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// resolveClusterConfig selects credentials without changing the kubeconfig or
// any process-wide current-context state. A non-empty explicit context is
// strict and never falls back to in-cluster credentials. Legacy callers can
// omit explicit selection to retain in-cluster-first behavior, then use the
// supplied legacy context hint (or kubeconfig current-context).
func resolveClusterConfig(
	contextName string,
	explicit bool,
	rules *clientcmd.ClientConfigLoadingRules,
	inClusterConfig func() (*rest.Config, error),
) (*rest.Config, string, error) {
	if rules == nil {
		rules = clientcmd.NewDefaultClientConfigLoadingRules()
	}
	// client-go's default loading rules may migrate old config files as a side
	// effect. Work from a private copy with migration disabled so resolution is
	// read-only and concurrent selections cannot share mutations.
	readRules := *rules
	readRules.MigrationRules = nil
	rules = &readRules

	if explicit && strings.TrimSpace(contextName) == "" {
		return nil, "", fmt.Errorf("explicit Kubernetes context name is empty")
	}

	if !explicit && inClusterConfig != nil {
		config, err := inClusterConfig()
		if err == nil && config != nil {
			return rest.CopyConfig(config), "", nil
		}
	}

	raw, err := rules.Load()
	if err != nil {
		return nil, "", fmt.Errorf("load kubeconfig: %w", err)
	}
	selected := contextName
	if !explicit && selected == "" {
		selected = raw.CurrentContext
	}
	if explicit {
		if _, ok := raw.Contexts[selected]; !ok {
			return nil, "", fmt.Errorf("Kubernetes context %q was not found in kubeconfig", selected)
		}
	}

	clientConfig := clientcmd.NewNonInteractiveClientConfig(*raw, selected, &clientcmd.ConfigOverrides{}, rules)
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, "", fmt.Errorf("build Kubernetes config for context %q: %w", selected, err)
	}
	return rest.CopyConfig(config), selected, nil
}

// localClusterBinding is the TUI's private, session-pinned client config. It is
// never rendered or serialized; refreshes reuse the same selection and
// credentials instead of consulting a possibly changed current context.
type localClusterBinding struct {
	config  *rest.Config
	context string
	err     error
}

func resolveLegacyLocalClusterBinding(contextName string) *localClusterBinding {
	return resolveLocalClusterBinding(contextName, clientcmd.NewDefaultClientConfigLoadingRules(), rest.InClusterConfig)
}

func resolveLocalClusterBinding(
	contextName string,
	rules *clientcmd.ClientConfigLoadingRules,
	inClusterConfig func() (*rest.Config, error),
) *localClusterBinding {
	config, boundContext, err := resolveClusterConfig(
		contextName,
		false,
		rules,
		inClusterConfig,
	)
	return &localClusterBinding{config: config, context: boundContext, err: err}
}
