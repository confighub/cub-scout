// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func resetCompareObjectSetFlags(t *testing.T) {
	t.Helper()
	prev := struct {
		dryFrom, ns, scope, format, out, ttl, failOn, norm string
		diff, save                                         bool
	}{
		compareObjectSetDryFrom, compareObjectSetNamespace, compareObjectSetScopeRaw,
		compareObjectSetFormat, compareObjectSetOut, compareObjectSetTTL,
		compareObjectSetFailOn, compareObjectSetNormProf,
		compareObjectSetDiff, compareObjectSetSave,
	}
	compareObjectSetDryFrom = ""
	compareObjectSetNamespace = ""
	compareObjectSetScopeRaw = ""
	compareObjectSetFormat = "json"
	compareObjectSetOut = ""
	compareObjectSetTTL = ""
	compareObjectSetFailOn = ""
	compareObjectSetNormProf = ""
	compareObjectSetDiff = false
	compareObjectSetSave = false
	t.Cleanup(func() {
		compareObjectSetDryFrom = prev.dryFrom
		compareObjectSetNamespace = prev.ns
		compareObjectSetScopeRaw = prev.scope
		compareObjectSetFormat = prev.format
		compareObjectSetOut = prev.out
		compareObjectSetTTL = prev.ttl
		compareObjectSetFailOn = prev.failOn
		compareObjectSetNormProf = prev.norm
		compareObjectSetDiff = prev.diff
		compareObjectSetSave = prev.save
	})
}

// runCompareObjectSetJSON executes `compare object-set` with the given extra
// args (--dry-from / --scope are supplied by the caller via args) and decodes
// the emitted Statement.
func runCompareObjectSetJSON(t *testing.T, args []string) agent.Statement {
	t.Helper()
	out := captureStdout(t, func() {
		rootCmd.SetArgs(append([]string{"compare", "object-set", "--format", "json"}, args...))
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("compare object-set returned error: %v", err)
		}
	})
	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal statement: %v\n%s", err, out)
	}
	if stmt.Predicate.PredicateName != string(agent.PredicateObjectSetDiff) {
		t.Fatalf("predicateName = %q, want %q", stmt.Predicate.PredicateName, agent.PredicateObjectSetDiff)
	}
	if strings.TrimSpace(stmt.Predicate.Fingerprint) == "" {
		t.Fatal("fingerprint is empty")
	}
	if stmt.Predicate.Evidence.ObjectSetDiff == nil {
		t.Fatal("objectSetDiff evidence missing")
	}
	return stmt
}

// (a) desired == live: no diff -> PASS, no changed/added/removed.
func TestCompareObjectSet_NoDiff_Pass(t *testing.T) {
	resetCompareObjectSetFlags(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: api
        image: example/api:v1
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, defaultNS string) ([]agent.ObjectSetObservedObject, error) {
		if defaultNS != "redis" {
			t.Fatalf("default namespace = %q, want redis", defaultNS)
		}
		live := desired[0].DeepCopy()
		live.SetNamespace(defaultNS)
		desired[0].SetNamespace(defaultNS)
		// Server-added noise must be ignored by authored-field projection.
		_ = unstructured.SetNestedField(live.Object, "123", "metadata", "resourceVersion")
		_ = unstructured.SetNestedField(live.Object, "server-added", "metadata", "annotations", "example.com/defaulted")
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})

	stmt := runCompareObjectSetJSON(t, []string{"--dry-from", path, "--scope", "namespace/redis"})
	if stmt.Predicate.Verdict != agent.VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", stmt.Predicate.Verdict)
	}
	ev := stmt.Predicate.Evidence.ObjectSetDiff
	if len(ev.ChangedObjects) != 0 || len(ev.AddedObjects) != 0 || len(ev.RemovedObjects) != 0 {
		t.Fatalf("expected no deltas; got changed=%d added=%d removed=%d", len(ev.ChangedObjects), len(ev.AddedObjects), len(ev.RemovedObjects))
	}
	if ev.Summary.Total != 1 || ev.Summary.Changed != 0 {
		t.Fatalf("summary = %+v, want total=1 changed=0", ev.Summary)
	}
}

// (b) one object with changed image + replicas -> BLOCK, 1 changedObject with
// the right field deltas (reproduces helm-expt#992 image.digest dry-run).
func TestCompareObjectSet_ChangedObject_Block(t *testing.T) {
	resetCompareObjectSetFlags(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: redis
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: redis
        image: redis@sha256:newdigest
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.ObjectSetObservedObject, error) {
		live := desired[0].DeepCopy()
		// Live differs from the desired (changed) render: old digest + old replicas.
		_ = unstructured.SetNestedField(live.Object, int64(2), "spec", "replicas")
		containers := []interface{}{map[string]interface{}{"name": "redis", "image": "redis@sha256:olddigest"}}
		_ = unstructured.SetNestedSlice(live.Object, containers, "spec", "template", "spec", "containers")
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})

	stmt := runCompareObjectSetJSON(t, []string{"--dry-from", path})
	if stmt.Predicate.Verdict != agent.VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
	ev := stmt.Predicate.Evidence.ObjectSetDiff
	if len(ev.ChangedObjects) != 1 {
		t.Fatalf("changedObjects = %d, want 1", len(ev.ChangedObjects))
	}
	if ev.Summary.Changed != 1 || ev.Summary.Total != 1 {
		t.Fatalf("summary = %+v, want changed=1 total=1", ev.Summary)
	}
	co := ev.ChangedObjects[0]
	if co.ID.Kind != "Deployment" || co.ID.Name != "redis" {
		t.Fatalf("changed object id = %+v, want Deployment/redis", co.ID)
	}
	gotFields := map[string]agent.ObjectSetFieldDiff{}
	for _, d := range co.Differences {
		gotFields[d.Field] = d
	}
	repl, ok := gotFields["spec.replicas"]
	if !ok {
		t.Fatalf("expected a spec.replicas delta; got fields %v", fieldNames(co.Differences))
	}
	if repl.Desired != "3" || repl.Live != "2" {
		t.Fatalf("replicas delta = desired %q live %q, want desired 3 live 2", repl.Desired, repl.Live)
	}
	img, ok := gotFields["spec.template.spec.containers[0].image"]
	if !ok {
		t.Fatalf("expected an image delta; got fields %v", fieldNames(co.Differences))
	}
	if img.Desired != "redis@sha256:newdigest" || img.Live != "redis@sha256:olddigest" {
		t.Fatalf("image delta = desired %q live %q", img.Desired, img.Live)
	}
}

// (c) --diff with a live-only object -> addedObjects populated, WATCH.
func TestCompareObjectSet_DiffAddedObject_Watch(t *testing.T) {
	resetCompareObjectSetFlags(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: redis
spec:
  replicas: 3
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.ObjectSetObservedObject, error) {
		live := desired[0].DeepCopy()
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})
	withFakeObjectSetExtrasLoader(t, func(_ context.Context, _ []*unstructured.Unstructured, _ agent.ObjectSetScope, _ string) ([]agent.ObjectSetObjectID, error) {
		return []agent.ObjectSetObjectID{{
			APIVersion: "v1",
			Kind:       "ConfigMap",
			Namespace:  "redis",
			Name:       "rogue-config",
		}}, nil
	})

	stmt := runCompareObjectSetJSON(t, []string{"--dry-from", path, "--diff"})
	if stmt.Predicate.Verdict != agent.VerdictWATCH {
		t.Fatalf("verdict = %s, want WATCH", stmt.Predicate.Verdict)
	}
	ev := stmt.Predicate.Evidence.ObjectSetDiff
	if len(ev.ChangedObjects) != 0 {
		t.Fatalf("changedObjects = %d, want 0", len(ev.ChangedObjects))
	}
	if len(ev.AddedObjects) != 1 || ev.AddedObjects[0].Name != "rogue-config" {
		t.Fatalf("addedObjects = %+v, want [rogue-config]", ev.AddedObjects)
	}
	if ev.Summary.Added != 1 {
		t.Fatalf("summary.added = %d, want 1", ev.Summary.Added)
	}
}

// A desired object absent live is a removed (membership) delta -> WATCH even
// without --diff, since removed is computed from the observed pairs.
func TestCompareObjectSet_RemovedObject_Watch(t *testing.T) {
	resetCompareObjectSetFlags(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: redis
spec:
  replicas: 3
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.ObjectSetObservedObject, error) {
		// Desired object not found live (Live nil, no inconclusive flag).
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Error: "live object not found"}}, nil
	})

	stmt := runCompareObjectSetJSON(t, []string{"--dry-from", path})
	if stmt.Predicate.Verdict != agent.VerdictWATCH {
		t.Fatalf("verdict = %s, want WATCH", stmt.Predicate.Verdict)
	}
	ev := stmt.Predicate.Evidence.ObjectSetDiff
	if len(ev.RemovedObjects) != 1 || ev.RemovedObjects[0].Name != "api" {
		t.Fatalf("removedObjects = %+v, want [api]", ev.RemovedObjects)
	}
	if len(ev.ChangedObjects) != 0 {
		t.Fatalf("changedObjects = %d, want 0 (missing is a membership delta)", len(ev.ChangedObjects))
	}
}

// --fail-on BLOCK on a changed object exits non-zero (code 2) while still
// emitting the receipt.
func TestCompareObjectSet_FailOnBlock(t *testing.T) {
	resetCompareObjectSetFlags(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: redis
spec:
  replicas: 3
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.ObjectSetObservedObject, error) {
		live := desired[0].DeepCopy()
		_ = unstructured.SetNestedField(live.Object, int64(5), "spec", "replicas")
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})

	rootCmd.SetArgs([]string{"compare", "object-set", "--dry-from", path, "--format", "json", "--fail-on", "BLOCK"})
	captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("BLOCK object-set-diff with --fail-on BLOCK must error")
		}
		var ec interface{ ExitCode() int }
		if !errors.As(err, &ec) || ec.ExitCode() != 2 {
			t.Fatalf("expected exit code 2 wrapper; got %v", err)
		}
		if !strings.Contains(err.Error(), "fail-on") {
			t.Fatalf("expected fail-on error, got %v", err)
		}
	})
}

// An inconclusive live lookup must not produce a false PASS — it errors out.
func TestCompareObjectSet_InconclusiveErrors(t *testing.T) {
	resetCompareObjectSetFlags(t)
	path := writeObjectSetManifest(t, `
apiVersion: example.com/v1
kind: Widget
metadata:
  name: w
  namespace: redis
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.ObjectSetObservedObject, error) {
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Error: "no matches for kind", Inconclusive: true}}, nil
	})

	rootCmd.SetArgs([]string{"compare", "object-set", "--dry-from", path, "--format", "json"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("inconclusive live lookup must error, not emit a PASS receipt")
	}
	if !strings.Contains(err.Error(), "inconclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A live object whose replicas were last written by kubectl-edit carries that
// field-manager attribution through to the corresponding field delta
// (cause=manual-edit + managerHint). This is the "which tool changed this
// field" provenance reused from the three-way compare path.
func TestCompareObjectSet_FieldAttribution(t *testing.T) {
	resetCompareObjectSetFlags(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: redis
spec:
  replicas: 3
`)
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.ObjectSetObservedObject, error) {
		live := desired[0].DeepCopy()
		_ = unstructured.SetNestedField(live.Object, int64(5), "spec", "replicas")
		live.SetManagedFields([]metav1.ManagedFieldsEntry{{
			Manager:    agent.ManagerKubectlEdit,
			Operation:  metav1.ManagedFieldsOperationUpdate,
			FieldsType: "FieldsV1",
			FieldsV1:   &metav1.FieldsV1{Raw: []byte(`{"f:spec":{"f:replicas":{}}}`)},
		}})
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})

	stmt := runCompareObjectSetJSON(t, []string{"--dry-from", path})
	if stmt.Predicate.Verdict != agent.VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
	ev := stmt.Predicate.Evidence.ObjectSetDiff
	if len(ev.ChangedObjects) != 1 {
		t.Fatalf("changedObjects = %d, want 1", len(ev.ChangedObjects))
	}
	var repl *agent.ObjectSetFieldDiff
	for i := range ev.ChangedObjects[0].Differences {
		if ev.ChangedObjects[0].Differences[i].Field == "spec.replicas" {
			repl = &ev.ChangedObjects[0].Differences[i]
		}
	}
	if repl == nil {
		t.Fatalf("no spec.replicas delta; fields = %v", fieldNames(ev.ChangedObjects[0].Differences))
	}
	if repl.Cause != string(agent.CauseManualEdit) {
		t.Fatalf("replicas cause = %q, want %q", repl.Cause, agent.CauseManualEdit)
	}
	if repl.ManagerHint != agent.ManagerKubectlEdit {
		t.Fatalf("replicas managerHint = %q, want %q", repl.ManagerHint, agent.ManagerKubectlEdit)
	}
}

func fieldNames(diffs []agent.ObjectSetFieldDiff) []string {
	out := make([]string, 0, len(diffs))
	for _, d := range diffs {
		out = append(out, d.Field)
	}
	return out
}
