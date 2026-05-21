// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// k8sLiveSubjectDynamicFieldPaths lists the field paths cub-scout prunes
// from a live K8s object before computing its canonical digest. These
// are server-maintained fields that change without any change in
// "what's actually deployed", and including them would make the
// fingerprint unstable across e.g. reconcile heartbeats.
//
// Locked in the design synthesis (#446 round 2): see
// docs/proposals/receipts-way-forward.md § "Subject digest — dual-subject
// canonicalization".
var k8sLiveSubjectDynamicFieldPaths = [][]string{
	{"status"},
	{"metadata", "managedFields"},
	{"metadata", "resourceVersion"},
	{"metadata", "generation"},
	{"metadata", "uid"},
	{"metadata", "creationTimestamp"},
}

// BuildK8sLiveSubject constructs the live-K8s subject for a receipt: a
// k8s-live:// URI plus a SHA-256 digest over the canonical-JSON of the
// pruned object map (status + managedFields + dynamic metadata fields
// removed).
//
// Returns an error if obj is nil. The caller is expected to have
// fetched obj via a read-only client and to handle missing-resource
// cases (404) before reaching this function.
func BuildK8sLiveSubject(obj *unstructured.Unstructured) (Subject, error) {
	if obj == nil {
		return Subject{}, fmt.Errorf("k8s-live subject: nil object")
	}

	// Subject name: scheme + apiVersion/kind/namespace/name.
	gvk := obj.GetObjectKind().GroupVersionKind()
	apiVersion := gvk.GroupVersion().String()
	if apiVersion == "/" || apiVersion == "" {
		apiVersion = obj.GetAPIVersion()
	}
	kind := gvk.Kind
	if kind == "" {
		kind = obj.GetKind()
	}
	namespace := obj.GetNamespace()
	name := obj.GetName()

	var subjectName string
	if namespace == "" {
		subjectName = fmt.Sprintf("%s%s/%s/%s", SubjectSchemeK8sLive, apiVersion, kind, name)
	} else {
		subjectName = fmt.Sprintf("%s%s/%s/%s/%s", SubjectSchemeK8sLive, apiVersion, kind, namespace, name)
	}

	// Clone the unstructured object's map so we can prune in place
	// without mutating the caller's copy.
	cloned := obj.DeepCopy().Object
	for _, path := range k8sLiveSubjectDynamicFieldPaths {
		unstructured.RemoveNestedField(cloned, path...)
	}

	canon, err := CanonicalJSON(cloned)
	if err != nil {
		return Subject{}, fmt.Errorf("k8s-live subject: canonical-json: %w", err)
	}
	sum := sha256.Sum256(canon)
	digestHex := hex.EncodeToString(sum[:])

	return Subject{
		Name:   subjectName,
		Digest: map[string]string{"sha256": digestHex},
	}, nil
}

// BuildConfigHubUnitSubject constructs the connected-mode ConfigHub-unit
// subject: a confighub-unit:// URI plus a SHA-256 digest over the
// canonical-JSON of the unit's revision-canonical body.
//
// unitSlug is the user-facing unit slug (e.g. "payments-api"). revision
// is the integer revision number from ConfigHub's revision system.
// unitCanonicalJSON is the raw bytes of the unit's canonical
// representation at that revision (fetched by the CLI before reaching
// this function).
//
// Returns an error if unitSlug is empty, revision is non-positive, or
// unitCanonicalJSON is empty. Callers in standalone mode must NOT call
// this function — they record an OmissionConfigHubUnitSubject entry on
// the predicate instead.
func BuildConfigHubUnitSubject(unitSlug string, revision int, unitCanonicalJSON []byte) (Subject, error) {
	if unitSlug == "" {
		return Subject{}, fmt.Errorf("confighub-unit subject: empty unit slug")
	}
	if revision <= 0 {
		return Subject{}, fmt.Errorf("confighub-unit subject: non-positive revision %d", revision)
	}
	if len(unitCanonicalJSON) == 0 {
		return Subject{}, fmt.Errorf("confighub-unit subject: empty canonical body")
	}

	subjectName := fmt.Sprintf("%s%s@rev=%d", SubjectSchemeConfigHubUnit, unitSlug, revision)
	sum := sha256.Sum256(unitCanonicalJSON)
	digestHex := hex.EncodeToString(sum[:])

	return Subject{
		Name:   subjectName,
		Digest: map[string]string{"sha256": digestHex},
	}, nil
}
