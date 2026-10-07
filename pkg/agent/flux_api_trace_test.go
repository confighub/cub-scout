// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// The objects and the `flux trace` output in testdata/flux-api-trace were
// recorded from one real cluster in one state (kind, Kubernetes 1.35, Flux
// source, kustomize and helm controllers): podinfo delivered by a
// Kustomization from a Git repository, and by a HelmRelease from a chart
// repository.
func recordedFluxObjects(t *testing.T) []runtime.Object {
	t.Helper()
	return recordedObjects(t,
		"deployment-podinfo", "kustomization-podinfo", "gitrepository-podinfo",
		"deployment-podinfo-helm", "helmrelease-podinfo-helm", "helmchart-podinfo-helm", "helmrepository-podinfo")
}

// recordedOCIObjects is a second recording, from another cluster of the same
// kind: podinfo delivered by a Kustomization from an OCIRepository. It is kept
// apart because its Deployment has the same name and namespace as the first.
func recordedOCIObjects(t *testing.T) []runtime.Object {
	t.Helper()
	return recordedObjects(t, "oci/deployment-podinfo-oci", "oci/kustomization-podinfo-oci", "oci/ocirepository-podinfo-oci")
}

func recordedObjects(t *testing.T, names ...string) []runtime.Object {
	t.Helper()
	var objects []runtime.Object
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join("testdata", "flux-api-trace", filepath.FromSlash(name)+".json"))
		if err != nil {
			t.Fatal(err)
		}
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(raw, &obj.Object); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		objects = append(objects, obj)
	}
	return objects
}

func fluxAPITracerOn(t *testing.T, objects []runtime.Object) (*FluxAPITracer, *dynamicfake.FakeDynamicClient) {
	t.Helper()
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), objects...)
	return NewFluxTracerWithKubernetesClient(client), client
}

// cliTraceOfRecording runs the flux CLI tracer over the recorded output.
func cliTraceOfRecording(t *testing.T, recording, kind, name, namespace string) *TraceResult {
	t.Helper()
	data, err := filepath.Abs(filepath.Join("testdata", "flux-api-trace", recording))
	if err != nil {
		t.Fatal(err)
	}
	flux := filepath.Join(t.TempDir(), "flux")
	if err := os.WriteFile(flux, []byte("#!/bin/sh\n[ \"$1\" = version ] && exit 0\ncat \""+data+"\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := NewFluxTracerWithPath(flux).Trace(context.Background(), kind, name, namespace)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// #824: without the flux CLI no surface could say where a Flux-delivered
// workload came from. The API tracer must find the same chain the CLI does,
// from the same cluster state.
func TestFluxAPITraceMatchesTheCLITraceOfTheSameCluster(t *testing.T) {
	for _, tc := range []struct {
		name, recording, workload, namespace string
		objects                              func(*testing.T) []runtime.Object
		wantKinds                            []string
		wantURL                              string
	}{
		{"kustomization", "flux-trace-deployment-podinfo.txt", "podinfo", "team-a", recordedFluxObjects,
			[]string{"GitRepository", "Kustomization", "Deployment"}, "https://github.com/stefanprodan/podinfo"},
		{"helm release", "flux-trace-deployment-podinfo-helm.txt", "podinfo-helm", "team-b", recordedFluxObjects,
			[]string{"HelmRepository", "HelmChart", "HelmRelease", "Deployment"}, "https://stefanprodan.github.io/podinfo"},
		{"oci repository", "oci/flux-trace-deployment-podinfo-oci.txt", "podinfo", "team-a", recordedOCIObjects,
			[]string{"OCIRepository", "Kustomization", "Deployment"}, "oci://ghcr.io/stefanprodan/manifests/podinfo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracer, client := fluxAPITracerOn(t, tc.objects(t))
			got, err := tracer.Trace(context.Background(), "Deployment", tc.workload, tc.namespace)
			if err != nil {
				t.Fatal(err)
			}
			want := cliTraceOfRecording(t, tc.recording, "Deployment", tc.workload, tc.namespace)
			if len(got.Chain) != len(want.Chain) {
				t.Fatalf("chain has %d links, the CLI trace has %d\n got: %+v\nwant: %+v", len(got.Chain), len(want.Chain), got.Chain, want.Chain)
			}
			for i, wantLink := range want.Chain {
				gotLink := got.Chain[i]
				if gotLink.Kind != tc.wantKinds[i] {
					t.Errorf("link %d kind = %q, want %q", i, gotLink.Kind, tc.wantKinds[i])
				}
				for field, pair := range map[string][2]string{
					"kind": {gotLink.Kind, wantLink.Kind}, "name": {gotLink.Name, wantLink.Name},
					"namespace": {gotLink.Namespace, wantLink.Namespace}, "url": {gotLink.URL, wantLink.URL},
					"revision": {gotLink.Revision, wantLink.Revision}, "path": {gotLink.Path, wantLink.Path},
				} {
					if pair[0] != pair[1] {
						t.Errorf("link %d (%s) %s = %q, the CLI trace has %q", i, wantLink.Kind, field, pair[0], pair[1])
					}
				}
				if gotLink.Ready != wantLink.Ready {
					t.Errorf("link %d (%s) ready = %v, the CLI trace has %v", i, wantLink.Kind, gotLink.Ready, wantLink.Ready)
				}
			}
			if got.Chain[0].URL != tc.wantURL {
				t.Errorf("source URL = %q, want %q", got.Chain[0].URL, tc.wantURL)
			}
			if !got.FullyManaged || got.Tool != "flux" {
				t.Errorf("FullyManaged=%v Tool=%q, want true and flux", got.FullyManaged, got.Tool)
			}
			if anchor := GitSourceAnchorFromTrace(got); anchor == nil || anchor.RepoURL != tc.wantURL {
				t.Errorf("anchor = %+v, want RepoURL %q", anchor, tc.wantURL)
			}
			// Exact reads only: this tracer must never list.
			for _, action := range client.Actions() {
				if action.GetVerb() != "get" {
					t.Errorf("the API tracer issued a %s on %s; it may only get", action.GetVerb(), action.GetResource().Resource)
				}
			}
		})
	}
}

func TestFluxAPITraceRefusesToGuess(t *testing.T) {
	podinfoKustomization := schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}

	t.Run("an object with no Flux labels is not managed by Flux", func(t *testing.T) {
		unmanaged := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": "manual", "namespace": "team-a"},
		}}
		tracer, _ := fluxAPITracerOn(t, []runtime.Object{unmanaged})
		result, err := tracer.Trace(context.Background(), "Deployment", "manual", "team-a")
		if result != nil || err == nil || err.Error() != "resource not managed by Flux" {
			t.Fatalf("result=%+v err=%v, want the not-managed error", result, err)
		}
	})

	t.Run("a source the reader may not get gives an error naming it, and no partial chain", func(t *testing.T) {
		tracer, client := fluxAPITracerOn(t, recordedFluxObjects(t))
		client.PrependReactor("get", "gitrepositories", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(podinfoKustomization.GroupResource(), "podinfo", nil)
		})
		result, err := tracer.Trace(context.Background(), "Deployment", "podinfo", "team-a")
		if result != nil {
			t.Fatalf("a chain was returned without its source: %+v", result.Chain)
		}
		if err == nil || !strings.Contains(err.Error(), "GitRepository flux-system/podinfo") || !apierrors.IsForbidden(err) {
			t.Fatalf("err = %v, want a forbidden error naming GitRepository flux-system/podinfo", err)
		}
	})

	t.Run("labels naming an owner that does not exist give an error, not a source", func(t *testing.T) {
		orphan := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{"name": "copied", "namespace": "team-a", "labels": map[string]interface{}{
				fluxKustomizeNameLabel: "gone", fluxKustomizeNamespaceLabel: "flux-system"}},
		}}
		tracer, _ := fluxAPITracerOn(t, append(recordedFluxObjects(t), orphan))
		result, err := tracer.Trace(context.Background(), "Deployment", "copied", "team-a")
		if result != nil || err == nil || !strings.Contains(err.Error(), "Kustomization flux-system/gone") {
			t.Fatalf("result=%+v err=%v, want an error naming the missing Kustomization", result, err)
		}
	})

	t.Run("readiness is the Ready condition, and a suspended owner is not ready", func(t *testing.T) {
		objects := recordedFluxObjects(t)
		for _, obj := range objects {
			u := obj.(*unstructured.Unstructured)
			switch u.GetKind() {
			case "GitRepository":
				_ = unstructured.SetNestedSlice(u.Object, []interface{}{map[string]interface{}{
					"type": "Ready", "status": "False", "reason": "GitOperationFailed", "message": "failed to checkout: authentication required"}}, "status", "conditions")
			case "Kustomization":
				_ = unstructured.SetNestedField(u.Object, true, "spec", "suspend")
			}
		}
		tracer, _ := fluxAPITracerOn(t, objects)
		result, err := tracer.Trace(context.Background(), "Deployment", "podinfo", "team-a")
		if err != nil {
			t.Fatal(err)
		}
		source, owner := result.Chain[0], result.Chain[1]
		if source.Ready || source.StatusReason != "failed to checkout: authentication required" {
			t.Errorf("source ready=%v reason=%q, want not ready with the condition's message", source.Ready, source.StatusReason)
		}
		if owner.Ready || owner.Status != "Suspended" {
			t.Errorf("owner ready=%v status=%q, want suspended and not ready", owner.Ready, owner.Status)
		}
		if result.FullyManaged {
			t.Error("FullyManaged is true with a failing source and a suspended owner")
		}
	})
}
