// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func withFakeObjectSetExtrasLoader(t *testing.T, fn func(context.Context, []*unstructured.Unstructured, agent.ObjectSetScope, string) ([]agent.ObjectSetObjectID, error)) {
	t.Helper()
	prev := loadObjectSetExtrasFn
	loadObjectSetExtrasFn = fn
	t.Cleanup(func() { loadObjectSetExtrasFn = prev })
}

func fakeMatchingConfigMapLoader(t *testing.T) {
	t.Helper()
	withFakeObjectSetLoader(t, func(_ context.Context, desired []*unstructured.Unstructured, _ string) ([]agent.ObjectSetObservedObject, error) {
		live := desired[0].DeepCopy()
		live.SetNamespace("helm-expt-demo")
		desired[0].SetNamespace("helm-expt-demo")
		return []agent.ObjectSetObservedObject{{Desired: desired[0], Live: live}}, nil
	})
}

const extrasManifest = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: web-config
  namespace: helm-expt-demo
`

func TestReceiptVerifyObjectSet_NoExtras_WATCHOnExtra(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writeObjectSetManifest(t, extrasManifest)
	fakeMatchingConfigMapLoader(t)
	withFakeObjectSetExtrasLoader(t, func(_ context.Context, _ []*unstructured.Unstructured, _ agent.ObjectSetScope, _ string) ([]agent.ObjectSetObjectID, error) {
		return []agent.ObjectSetObjectID{{APIVersion: "v1", Kind: "ConfigMap", Namespace: "helm-expt-demo", Name: "drift-not-in-object-set"}}, nil
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--file", path, "--scope", "namespace/helm-expt-demo", "--predicate", "object-set-matches", "--no-extras", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if stmt.Predicate.Verdict != agent.VerdictWATCH {
		t.Fatalf("verdict = %s, want WATCH", stmt.Predicate.Verdict)
	}
	if stmt.Predicate.Evidence.ObjectSet == nil || !stmt.Predicate.Evidence.ObjectSet.ExtraChecked {
		t.Fatal("extraChecked not set on evidence")
	}
	if len(stmt.Predicate.Evidence.ObjectSet.ExtraObjects) != 1 {
		t.Fatalf("extra not surfaced: %+v", stmt.Predicate.Evidence.ObjectSet.ExtraObjects)
	}
}

func TestReceiptVerifyObjectSet_NoExtras_PASSWhenExclusive(t *testing.T) {
	resetReceiptFlags(t)
	resetReceiptBatch3Flags(t)
	resetReceiptFailOnFlag(t)
	path := writeObjectSetManifest(t, extrasManifest)
	fakeMatchingConfigMapLoader(t)
	withFakeObjectSetExtrasLoader(t, func(_ context.Context, _ []*unstructured.Unstructured, _ agent.ObjectSetScope, _ string) ([]agent.ObjectSetObjectID, error) {
		return nil, nil // closed-world clean
	})

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"receipt", "verify", "--file", path, "--scope", "namespace/helm-expt-demo", "--predicate", "object-set-matches", "--no-extras", "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var stmt agent.Statement
	if err := json.Unmarshal([]byte(out), &stmt); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if stmt.Predicate.Verdict != agent.VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", stmt.Predicate.Verdict)
	}
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == agent.OmissionExtraLiveObjectCoverage {
			t.Fatal("closed-world clean: extra-live-object-coverage omission must be dropped")
		}
	}
}
