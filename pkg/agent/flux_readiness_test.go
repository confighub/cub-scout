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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// testdata/flux-readiness-826 was recorded from a real cluster (kind,
// Kubernetes 1.35, Flux) after a healthy Kustomization's spec.path was changed
// to a directory that does not exist: its Ready condition is False with
// "kustomization path not found", and `flux trace` still prints
// "Status: Last reconciled at ...".
func failingKustomizationFixture(t *testing.T) (cli *FluxTracer, objects []runtime.Object) {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata", "flux-readiness-826"))
	if err != nil {
		t.Fatal(err)
	}
	flux := filepath.Join(t.TempDir(), "flux")
	script := "#!/bin/sh\n[ \"$1\" = version ] && exit 0\ncat \"" + filepath.Join(dir, "flux-trace-deployment-failing-kustomization.txt") + "\"\n"
	if err := os.WriteFile(flux, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kustomization-failing", "gitrepository-ready", "deployment-podinfo"} {
		raw, err := os.ReadFile(filepath.Join(dir, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(raw, &obj.Object); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, obj)
	}
	return NewFluxTracerWithPath(flux), objects
}

func linkOfKind(t *testing.T, result *TraceResult, kind string) ChainLink {
	t.Helper()
	for _, link := range result.Chain {
		if link.Kind == kind {
			return link
		}
	}
	t.Fatalf("no %s link in %+v", kind, result.Chain)
	return ChainLink{}
}

// #826: read as text, a Kustomization whose build is failing looked ready,
// so trace drew it with a green tick and FullyManaged stayed true.
func TestFluxCLITraceReadinessComesFromTheReadyCondition(t *testing.T) {
	cli, objects := failingKustomizationFixture(t)

	// The defect, pinned: the text alone says ready.
	fromText, err := cli.Trace(context.Background(), "Deployment", "podinfo", "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if ks := linkOfKind(t, fromText, "Kustomization"); !ks.Ready || !fromText.FullyManaged {
		t.Fatalf("the recorded text no longer reads as ready (ready=%v fullyManaged=%v); this fixture is meant to show that it does", ks.Ready, fromText.FullyManaged)
	}

	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), objects...)
	checked, err := NewFluxTracerWithConditionReadiness(cli, client).Trace(context.Background(), "Deployment", "podinfo", "team-a")
	if err != nil {
		t.Fatal(err)
	}
	ks := linkOfKind(t, checked, "Kustomization")
	if ks.Ready {
		t.Error("the failing Kustomization is still reported ready")
	}
	if !strings.Contains(ks.StatusReason, "kustomization path not found") {
		t.Errorf("StatusReason = %q, want the Ready condition's message", ks.StatusReason)
	}
	if checked.FullyManaged {
		t.Error("FullyManaged is true with a failing Kustomization")
	}
	// What was healthy stays healthy, and the chain itself is the CLI's.
	if source := linkOfKind(t, checked, "GitRepository"); !source.Ready || source.URL != "https://github.com/stefanprodan/podinfo" {
		t.Errorf("source = %+v, want ready with the recorded URL", source)
	}
	if len(checked.Chain) != len(fromText.Chain) {
		t.Errorf("the chain changed length: %d, was %d", len(checked.Chain), len(fromText.Chain))
	}
	for _, action := range client.Actions() {
		if action.GetVerb() != "get" {
			t.Errorf("readiness was checked with a %s; it may only get", action.GetVerb())
		}
	}
}

// If a link's object cannot be read, its readiness is unknown. The text's
// "ready" must not survive as if it had been checked.
func TestFluxCLITraceReadinessIsUnknownWhenItCannotBeRead(t *testing.T) {
	cli, _ := failingKustomizationFixture(t)
	empty := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	checked, err := NewFluxTracerWithConditionReadiness(cli, empty).Trace(context.Background(), "Deployment", "podinfo", "team-a")
	if err != nil {
		t.Fatal(err)
	}
	ks := linkOfKind(t, checked, "Kustomization")
	if ks.Ready || ks.Status != "Unknown" || !strings.Contains(ks.StatusReason, "could not be read") {
		t.Errorf("link = ready %v, status %q, reason %q; want not ready, Unknown, and a reason saying it could not be read", ks.Ready, ks.Status, ks.StatusReason)
	}
	if checked.FullyManaged {
		t.Error("FullyManaged is true although readiness could not be read")
	}
}
