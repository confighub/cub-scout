// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// fakeFluxForAnchor answers `flux trace` with output recorded from Flux on a
// real cluster (testdata/flux-trace-814) and logs every invocation. The
// cluster had Deployment team-a/podinfo applied by Kustomization
// flux-system/podinfo from GitRepository podinfo, and that Kustomization
// labelled as managed by a parent Kustomization "fleet" with its own
// GitRepository, as `flux bootstrap` lays things out.
func fakeFluxForAnchor(t *testing.T) (path, calls string) {
	t.Helper()
	data, err := filepath.Abs("testdata/flux-trace-814")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	calls = filepath.Join(dir, "calls.log")
	path = filepath.Join(dir, "flux")
	script := `#!/bin/sh
echo "$*" >> "` + calls + `"
case "$*" in
  "version --client"*) exit 0 ;;
  "trace deployment podinfo -n team-a"*) cat "` + data + `/deployment-podinfo.txt"; exit 0 ;;
  "trace kustomization podinfo -n flux-system"*) cat "` + data + `/kustomization-podinfo-with-parent.txt"; exit 0 ;;
esac
echo "failed to trace: object not managed by Flux" >&2
exit 1
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, calls
}

// #814: the Git source anchor for a Flux-delivered workload was taken from a
// trace of its owning Kustomization. Under a parent Kustomization that names
// the fleet repository, so a receipt for the Deployment passed "applied
// matches spec" against a repository the Deployment did not come from.
func TestFluxGitSourceAnchorIsTheWorkloadsOwnSource(t *testing.T) {
	flux, calls := fakeFluxForAnchor(t)
	deployment := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{
			"name": "podinfo", "namespace": "team-a",
			"labels": map[string]interface{}{
				"kustomize.toolkit.fluxcd.io/name":      "podinfo",
				"kustomize.toolkit.fluxcd.io/namespace": "flux-system",
			},
		},
	}}
	owner := DetectOwnership(deployment)
	if owner.Type != OwnerFlux || owner.Name != "podinfo" {
		t.Fatalf("fixture ownership = %+v, want Flux/podinfo", owner)
	}

	anchor := CollectGitSourceAnchorForOwnerWith(context.Background(), deployment, owner, GitSourceTracers{Flux: NewFluxTracerWithPath(flux)})
	if anchor == nil {
		t.Fatal("no Git source anchor for a Flux-delivered Deployment")
	}
	if anchor.RepoURL != "https://github.com/stefanprodan/podinfo" {
		t.Errorf("RepoURL = %q, want the repository the Deployment came from", anchor.RepoURL)
	}
	if strings.Contains(anchor.RepoURL, "flux2-kustomize-helm-example") {
		t.Errorf("the anchor names the parent Kustomization's fleet repository: %q", anchor.RepoURL)
	}
	if !strings.Contains(anchor.Revision, "6b7aab8a10d6ee8b895b0a5048f4ab0966ed29ff") {
		t.Errorf("Revision = %q, want the podinfo revision that was applied", anchor.Revision)
	}

	logged, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range strings.Split(strings.TrimSpace(string(logged)), "\n") {
		if strings.HasPrefix(call, "trace kustomization") {
			t.Errorf("flux was asked what manages the owner, not where the workload came from: %s", call)
		}
	}
}

// A workload Flux does not manage gets no anchor, and nothing is borrowed from
// an owner trace to fill the gap.
func TestFluxGitSourceAnchorIsAbsentWhenTheWorkloadTraceFails(t *testing.T) {
	flux, _ := fakeFluxForAnchor(t)
	other := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "unknown", "namespace": "team-a"},
	}}
	// Ownership still says Flux/podinfo, as a stale or copied label would.
	owner := Ownership{Type: OwnerFlux, SubType: "kustomization", Name: "podinfo", Namespace: "flux-system"}
	if anchor := CollectGitSourceAnchorForOwnerWith(context.Background(), other, owner, GitSourceTracers{Flux: NewFluxTracerWithPath(flux)}); anchor != nil {
		t.Errorf("anchor = %+v, want none: the workload itself could not be traced", anchor)
	}
}
