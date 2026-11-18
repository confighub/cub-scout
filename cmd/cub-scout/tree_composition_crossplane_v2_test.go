package main

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// The shape recorded from Crossplane 2.4.3 in CI run 44254135155: a
// namespaced composite labelled with its own name, and the Deployment it
// composed, labelled with the composite's name and owned by it.
func crossplaneV2Objects(composedName string) []*unstructured.Unstructured {
	composite := &unstructured.Unstructured{}
	composite.SetAPIVersion("lane.example.org/v1")
	composite.SetKind("XLaneApp")
	composite.SetName("lane")
	composite.SetNamespace("xp-lane")
	composite.SetUID("xr-uid")
	composite.SetLabels(map[string]string{"crossplane.io/composite": "lane"})
	_ = unstructured.SetNestedField(composite.Object, "xlaneapps.lane.example.org", "spec", "crossplane", "compositionRef", "name")

	composed := &unstructured.Unstructured{}
	composed.SetAPIVersion("apps/v1")
	composed.SetKind("Deployment")
	composed.SetName(composedName)
	composed.SetNamespace("xp-lane")
	composed.SetLabels(map[string]string{"crossplane.io/composite": "lane"})
	composed.SetAnnotations(map[string]string{"crossplane.io/composition-resource-name": "deployment"})
	controller := true
	composed.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "lane.example.org/v1", Kind: "XLaneApp", Name: "lane", UID: "xr-uid", Controller: &controller}})
	return []*unstructured.Unstructured{composite, composed}
}

func TestCompositionTreeShowsACrossplaneV2CompositeAsItsOwnRoot(t *testing.T) {
	for _, composedName := range []string{"lane-74b4534e477d", "lane"} {
		t.Run(composedName, func(t *testing.T) {
			byXR := buildCompositionIndex(crossplaneV2Objects(composedName))
			if len(byXR) != 1 {
				keys := make([]string, 0, len(byXR))
				for key := range byXR {
					keys = append(keys, key)
				}
				t.Fatalf("roots = %q; want only XLaneApp/lane (no made-up CompositeResource parent)", keys)
			}
			for key, node := range byXR {
				if strings.Contains(key, "CompositeResource") || node.XR.Ref.Kind != "XLaneApp" || !node.XR.Present {
					t.Fatalf("root %s = %#v; want the present XLaneApp/lane", key, node.XR)
				}
				if len(node.Managed) != 1 || node.Managed[0].Ref.Kind != "Deployment" || node.Managed[0].Ref.Name != composedName {
					t.Fatalf("managed = %#v; want the composed Deployment %s", node.Managed, composedName)
				}
			}
		})
	}
}
