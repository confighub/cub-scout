// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func obj(kind string, m map[string]interface{}) *unstructured.Unstructured {
	m["apiVersion"] = "v1"
	m["kind"] = kind
	if _, ok := m["metadata"]; !ok {
		m["metadata"] = map[string]interface{}{"name": "x", "namespace": "ns"}
	}
	return &unstructured.Unstructured{Object: m}
}

func normalized(t *testing.T, u *unstructured.Unstructured) map[string]interface{} {
	t.Helper()
	out, err := NormalizeObjectForProfile(NormalizationProfileK8sZeroDefaultsV1, u)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	return out.Object
}

func TestNormalize_ProbeInitialDelayZeroDropped(t *testing.T) {
	u := obj("Pod", map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []interface{}{map[string]interface{}{
				"name":          "c",
				"livenessProbe": map[string]interface{}{"initialDelaySeconds": int64(0), "periodSeconds": int64(10)},
			}},
		},
	})
	spec := normalized(t, u)["spec"].(map[string]interface{})
	probe := spec["containers"].([]interface{})[0].(map[string]interface{})["livenessProbe"].(map[string]interface{})
	if _, present := probe["initialDelaySeconds"]; present {
		t.Fatal("initialDelaySeconds: 0 must be dropped")
	}
	if probe["periodSeconds"] != int64(10) {
		t.Fatal("non-zero sibling must survive")
	}
}

func TestNormalize_RBACEmptyRulesDropped_GrafanaCase(t *testing.T) {
	role := obj("ClusterRole", map[string]interface{}{"rules": []interface{}{}})
	if _, present := normalized(t, role)["rules"]; present {
		t.Fatal("ClusterRole rules: [] must normalize to absent (grafana watch case)")
	}
	// A non-RBAC kind keeps its empty list (NetworkPolicy ingress: [] is meaningful).
	np := obj("NetworkPolicy", map[string]interface{}{"spec": map[string]interface{}{"ingress": []interface{}{}}})
	spec := normalized(t, np)["spec"].(map[string]interface{})
	if _, present := spec["ingress"]; !present {
		t.Fatal("NetworkPolicy spec.ingress: [] must be preserved")
	}
}

func TestNormalize_OtherRules(t *testing.T) {
	u := obj("Service", map[string]interface{}{
		"metadata": map[string]interface{}{"name": "s", "annotations": map[string]interface{}{}},
		"spec": map[string]interface{}{
			"publishNotReadyAddresses": false,
			"caBundle":                 "",
			"ports":                    []interface{}{map[string]interface{}{"port": int64(80)}},
		},
	})
	out := normalized(t, u)
	meta := out["metadata"].(map[string]interface{})
	if _, present := meta["annotations"]; present {
		t.Fatal("empty metadata.annotations must be dropped")
	}
	spec := out["spec"].(map[string]interface{})
	if _, present := spec["publishNotReadyAddresses"]; present {
		t.Fatal("spec.publishNotReadyAddresses: false must be dropped")
	}
	if _, present := spec["caBundle"]; present {
		t.Fatal("spec.caBundle: \"\" must be dropped")
	}
}

func TestNormalize_UnknownProfileErrors(t *testing.T) {
	if _, err := NormalizeObjectForProfile("nope/v9", obj("Pod", map[string]interface{}{})); err == nil {
		t.Fatal("unknown profile must error")
	}
}

// The cross-tool contract: the standalone digest equals the receipt's
// desiredDigest for the same inputs and profile.
func TestRenderedObjectSetDigest_MatchesEvidenceDesiredDigest(t *testing.T) {
	desired := []*unstructured.Unstructured{
		obj("ClusterRole", map[string]interface{}{"metadata": map[string]interface{}{"name": "r"}, "rules": []interface{}{}}),
		objectSetDeployment(2, "example/api:v1"),
	}
	profile := NormalizationProfileK8sZeroDefaultsV1
	standalone, count, err := ComputeRenderedObjectSetDigest(profile, desired)
	if err != nil {
		t.Fatalf("standalone digest: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d", count)
	}

	observed := make([]ObjectSetObservedObject, 0, len(desired))
	for _, d := range desired {
		observed = append(observed, ObjectSetObservedObject{Desired: d, Live: d.DeepCopy()})
	}
	normalizedObs, err := NormalizeObservedObjects(profile, observed)
	if err != nil {
		t.Fatalf("normalize observed: %v", err)
	}
	evidence, err := BuildObjectSetEvidence(
		ObjectSetSource{Type: "file", Ref: "rendered.yaml"},
		ObjectSetScope{Kind: "namespace", Namespace: "ns"},
		normalizedObs,
	)
	if err != nil {
		t.Fatalf("BuildObjectSetEvidence: %v", err)
	}
	if evidence.DesiredDigest != standalone {
		t.Fatalf("contract broken: standalone %s != evidence desiredDigest %s", standalone, evidence.DesiredDigest)
	}
}

// The acceptance shape for the grafana watch row: desired authors rules: [],
// the live API server stores the object without rules. Raw comparison
// mismatches; the profile matches.
func TestNormalize_GrafanaRulesAcceptance(t *testing.T) {
	desired := obj("ClusterRole", map[string]interface{}{"metadata": map[string]interface{}{"name": "g"}, "rules": []interface{}{}})
	live := obj("ClusterRole", map[string]interface{}{"metadata": map[string]interface{}{"name": "g"}})

	raw, err := BuildObjectSetEvidence(ObjectSetSource{Type: "file", Ref: "r.yaml"}, ObjectSetScope{Kind: "cluster"},
		[]ObjectSetObservedObject{{Desired: desired, Live: live}})
	if err != nil {
		t.Fatalf("raw evidence: %v", err)
	}
	if raw.Summary.Mismatched != 1 {
		t.Fatalf("raw comparison should mismatch (got %+v)", raw.Summary)
	}

	normalizedObs, err := NormalizeObservedObjects(NormalizationProfileK8sZeroDefaultsV1,
		[]ObjectSetObservedObject{{Desired: desired.DeepCopy(), Live: live.DeepCopy()}})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	prof, err := BuildObjectSetEvidence(ObjectSetSource{Type: "file", Ref: "r.yaml"}, ObjectSetScope{Kind: "cluster"}, normalizedObs)
	if err != nil {
		t.Fatalf("profile evidence: %v", err)
	}
	if prof.Summary.Matched != 1 {
		t.Fatalf("profile comparison should match (got %+v)", prof.Summary)
	}
}
