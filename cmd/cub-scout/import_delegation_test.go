// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"testing"
)

func TestSelectGitOpsTargets(t *testing.T) {
	t.Parallel()

	targets := []cubTargetRef{
		{Slug: "dev-kubernetes-yaml-kind-test", ProviderType: "Kubernetes", Toolchain: "Kubernetes/YAML"},
		{Slug: "argo-render", ProviderType: "ArgocdRenderer", Toolchain: "ArgocdRenderer"},
		{Slug: "flux-render", ProviderType: "FluxRenderer", Toolchain: "FluxRenderer"},
	}

	k8s, argo, flux := selectGitOpsTargets(targets)
	if k8s != "dev-kubernetes-yaml-kind-test" {
		t.Fatalf("k8s target mismatch: got %q", k8s)
	}
	if argo != "argo-render" {
		t.Fatalf("argo renderer target mismatch: got %q", argo)
	}
	if flux != "flux-render" {
		t.Fatalf("flux renderer target mismatch: got %q", flux)
	}
}

func TestFilterScoutWorkloadsAfterDelegation(t *testing.T) {
	t.Parallel()

	workloads := []WorkloadInfo{
		{Owner: "ArgoCD", Namespace: "a", Name: "api"},
		{Owner: "Flux", Namespace: "b", Name: "worker"},
		{Owner: "Helm", Namespace: "c", Name: "redis"},
		{Owner: "Native", Namespace: "d", Name: "cfg"},
	}

	filtered := filterScoutWorkloadsAfterDelegation(workloads, gitOpsDelegationResult{
		ArgoDelegated: true,
		FluxDelegated: false,
	})

	got := []string{}
	for _, w := range filtered {
		got = append(got, w.Owner+":"+w.Name)
	}
	want := []string{"Flux:worker", "Helm:redis", "Native:cfg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered workloads: got %v want %v", got, want)
	}
}

func TestGitOpsNamespacesForOwner(t *testing.T) {
	t.Parallel()

	workloads := []WorkloadInfo{
		{
			Owner:     "ArgoCD",
			Namespace: "x",
			Name:      "api",
			GitOpsRef: &GitOpsReference{Namespace: "argocd"},
		},
		{
			Owner:     "ArgoCD",
			Namespace: "y",
			Name:      "worker",
			GitOpsRef: &GitOpsReference{Namespace: "argocd"},
		},
		{
			Owner:     "ArgoCD",
			Namespace: "z",
			Name:      "billing",
			GitOpsRef: &GitOpsReference{Namespace: "argocd-alt"},
		},
	}

	got := gitOpsNamespacesForOwner(workloads, "ArgoCD", "argocd")
	want := []string{"argocd", "argocd-alt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("namespace selection mismatch: got %v want %v", got, want)
	}
}
