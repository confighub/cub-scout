// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func withFakeWorkloadsLoader(t *testing.T, fn func(context.Context, []*unstructured.Unstructured, string) ([]agent.WorkloadConvergedObservedObject, error)) {
	t.Helper()
	prev := loadWorkloadsConvergedLiveFn
	loadWorkloadsConvergedLiveFn = fn
	t.Cleanup(func() { loadWorkloadsConvergedLiveFn = prev })
}

// notReadyDeployment returns a live Deployment whose status drives kstatus
// InProgress (availableReplicas below the spec).
func notReadyDeployment(desired *unstructured.Unstructured) *unstructured.Unstructured {
	live := desired.DeepCopy()
	live.SetNamespace("helm-expt-demo")
	_ = unstructured.SetNestedField(live.Object, int64(1), "metadata", "generation")
	_ = unstructured.SetNestedField(live.Object, int64(1), "status", "observedGeneration")
	_ = unstructured.SetNestedField(live.Object, int64(1), "status", "replicas")
	_ = unstructured.SetNestedField(live.Object, int64(1), "status", "updatedReplicas")
	_ = unstructured.SetNestedField(live.Object, int64(0), "status", "readyReplicas")
	_ = unstructured.SetNestedField(live.Object, int64(0), "status", "availableReplicas")
	return live
}

func configErrorPod(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]interface{}{"name": name, "namespace": "helm-expt-demo", "labels": map[string]interface{}{"app": "web"}},
		"status": map[string]interface{}{
			"containerStatuses": []interface{}{
				map[string]interface{}{
					"name":  "web",
					"state": map[string]interface{}{"waiting": map[string]interface{}{"reason": "CreateContainerConfigError"}},
				},
			},
		},
	}}
}

// The F3 reproduction at the CLI layer: object-set-matches would PASS, but
// workloads-converged must BLOCK because the pod is wedged.
func TestReceiptVerifyWorkloads_BLOCKOnCreateContainerConfigError(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: helm-expt-demo
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web
`)
	withFakeWorkloadsLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.WorkloadConvergedObservedObject, error) {
		return []agent.WorkloadConvergedObservedObject{{
			Desired: desired[0],
			Live:    notReadyDeployment(desired[0]),
			Pods:    []*unstructured.Unstructured{configErrorPod("web-abc")},
		}}, nil
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--file", path, "--scope", "namespace/helm-expt-demo", "--predicate", "workloads-converged", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("receipt verify workloads-converged returned error: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal statement: %v\n%s", err, out)
	}
	if stmt.Predicate.PredicateName != string(agent.PredicateWorkloadsConverged) {
		t.Fatalf("predicate = %s", stmt.Predicate.PredicateName)
	}
	if stmt.Predicate.Verdict != agent.VerdictBLOCK {
		t.Fatalf("verdict = %s, want BLOCK", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Evidence.Workloads == nil {
		t.Fatal("workloads evidence missing")
	}
	w := stmt.Predicate.Evidence.Workloads.Workloads[0]
	if len(w.PodReasons) == 0 || w.PodReasons[0].Reason != "CreateContainerConfigError" {
		t.Fatalf("expected CreateContainerConfigError pod reason, got %+v", w.PodReasons)
	}
}

func TestReceiptVerifyWorkloads_FailOnExit2WhenMissing(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: helm-expt-demo
spec:
  replicas: 1
`)
	withFakeWorkloadsLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.WorkloadConvergedObservedObject, error) {
		return []agent.WorkloadConvergedObservedObject{{Desired: desired[0], Live: nil}}, nil
	})

	rootCmd.SetArgs([]string{"receipt", "verify", "--file", path, "--scope", "namespace/helm-expt-demo", "--predicate", "workloads-converged", "--format", "json", "--fail-on", "any-non-pass"})
	captureStdout(t, func() {
		err := rootCmd.Execute()
		if err == nil {
			t.Fatal("missing workload with --fail-on any-non-pass must error")
		}
		var ec interface{ ExitCode() int }
		if !errors.As(err, &ec) || ec.ExitCode() != 2 {
			t.Fatalf("expected exit code 2 wrapper; got %v", err)
		}
	})
}

func TestReceiptVerifyWorkloads_RejectsBadGraceWindow(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writeObjectSetManifest(t, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: helm-expt-demo
spec:
  replicas: 1
`)
	rootCmd.SetArgs([]string{"receipt", "verify", "--file", path, "--scope", "namespace/helm-expt-demo", "--predicate", "workloads-converged", "--grace-window", "notaduration"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("invalid --grace-window must error")
	}
}
