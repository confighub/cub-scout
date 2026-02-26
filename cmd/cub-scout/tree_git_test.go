package main

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestBuildTreeGitJSON_IncludesApplicationSetRelationships(t *testing.T) {
	appSet := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "ApplicationSet",
			"metadata": map[string]interface{}{
				"name":      "workloads-generator",
				"namespace": "argocd",
			},
			"spec": map[string]interface{}{
				"generators": []interface{}{
					map[string]interface{}{
						"list": map[string]interface{}{
							"elements": []interface{}{map[string]interface{}{"env": "dev"}},
						},
					},
					map[string]interface{}{
						"git": map[string]interface{}{
							"repoURL": "https://git.example.local/platform.git",
						},
					},
				},
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"source": map[string]interface{}{
							"repoURL": "https://git.example.local/platform.git",
							"path":    "argo/applicationset/{{env}}",
						},
					},
				},
			},
		},
	}

	workloadsDev := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "workloads-dev",
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"argocd.argoproj.io/application-set-name": "workloads-generator",
				},
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"repoURL": "https://git.example.local/platform.git",
					"path":    "argo/applicationset/dev",
				},
			},
		},
	}

	workloadsProd := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "workloads-prod",
				"namespace": "argocd",
				"annotations": map[string]interface{}{
					"cub-scout.io/generated-by-applicationset": "workloads-generator",
				},
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"repoURL": "https://git.example.local/platform.git",
					"path":    "argo/applicationset/prod",
				},
			},
		},
	}

	paymentsDev := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "payments-dev",
				"namespace": "argocd",
				"annotations": map[string]interface{}{
					"cub-scout.io/parent-application": "platform-root",
				},
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"repoURL": "https://git.example.local/platform.git",
					"path":    "argo/app-of-apps/apps/payments-dev",
				},
			},
		},
	}

	rootApp := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "platform-root",
				"namespace": "argocd",
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"repoURL": "https://git.example.local/platform.git",
					"path":    "argo/app-of-apps/root",
				},
			},
		},
	}

	result := buildTreeGitJSON(
		nil,
		&unstructured.UnstructuredList{Items: []unstructured.Unstructured{workloadsDev, workloadsProd, paymentsDev, rootApp}},
		&unstructured.UnstructuredList{Items: []unstructured.Unstructured{appSet}},
	)

	if len(result.ApplicationSets) != 1 {
		t.Fatalf("expected 1 applicationset, got %d", len(result.ApplicationSets))
	}

	appSetItem := result.ApplicationSets[0]
	if appSetItem.Name != "workloads-generator" {
		t.Fatalf("expected applicationset workloads-generator, got %q", appSetItem.Name)
	}
	if !reflect.DeepEqual(appSetItem.GeneratorTypes, []string{"git", "list"}) {
		t.Fatalf("expected generator types [git list], got %#v", appSetItem.GeneratorTypes)
	}
	if !reflect.DeepEqual(appSetItem.GeneratedApplications, []string{"workloads-dev", "workloads-prod"}) {
		t.Fatalf("expected generated applications [workloads-dev workloads-prod], got %#v", appSetItem.GeneratedApplications)
	}

	appIndex := map[string]treeGitAppJSON{}
	for _, item := range result.ArgoApplications {
		appIndex[item.Name] = item
	}

	if appIndex["workloads-dev"].GeneratedByApplicationSet != "workloads-generator" {
		t.Fatalf("expected workloads-dev applicationset link, got %q", appIndex["workloads-dev"].GeneratedByApplicationSet)
	}
	if appIndex["workloads-prod"].GeneratedByApplicationSet != "workloads-generator" {
		t.Fatalf("expected workloads-prod applicationset link, got %q", appIndex["workloads-prod"].GeneratedByApplicationSet)
	}
	if appIndex["payments-dev"].ParentApplication != "platform-root" {
		t.Fatalf("expected payments-dev parent application, got %q", appIndex["payments-dev"].ParentApplication)
	}
}

func TestDetectApplicationSetGeneratorTypes_UnknownFallback(t *testing.T) {
	spec := map[string]interface{}{
		"generators": []interface{}{
			map[string]interface{}{
				"custom": map[string]interface{}{"enabled": true},
			},
		},
	}

	got := detectApplicationSetGeneratorTypes(spec)
	want := []string{"unknown"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestResolveGeneratedByApplicationSet_PrefersOwnerRef(t *testing.T) {
	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "payments-dev",
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"argocd.argoproj.io/application-set-name": "from-label",
				},
				"annotations": map[string]interface{}{
					"cub-scout.io/generated-by-applicationset": "from-annotation",
				},
			},
		},
	}

	app.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "ApplicationSet",
			Name:       "from-ownerref",
		},
	})

	if got := resolveGeneratedByApplicationSet(app); got != "from-ownerref" {
		t.Fatalf("expected owner ref precedence, got %q", got)
	}
}

func TestBuildTreeGitJSON_LineageConfidenceAndChildren(t *testing.T) {
	rootApp := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "platform-root",
				"namespace": "argocd",
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"repoURL": "https://git.example.local/platform.git",
					"path":    "argo/root",
				},
			},
		},
	}

	childViaOwnerRef := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "child-explicit",
				"namespace": "argocd",
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"repoURL": "https://git.example.local/platform.git",
					"path":    "argo/child1",
				},
			},
		},
	}
	childViaOwnerRef.SetOwnerReferences([]metav1.OwnerReference{
		{APIVersion: "argoproj.io/v1alpha1", Kind: "Application", Name: "platform-root"},
	})

	childViaLabel := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "child-inferred",
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"app.kubernetes.io/part-of": "platform-root",
				},
			},
			"spec": map[string]interface{}{
				"source": map[string]interface{}{
					"repoURL": "https://git.example.local/platform.git",
					"path":    "argo/child2",
				},
			},
		},
	}

	result := buildTreeGitJSON(
		nil,
		&unstructured.UnstructuredList{Items: []unstructured.Unstructured{rootApp, childViaOwnerRef, childViaLabel}},
		nil,
	)

	if len(result.ArgoApplications) != 3 {
		t.Fatalf("expected 3 applications, got %d", len(result.ArgoApplications))
	}

	appIndex := map[string]treeGitAppJSON{}
	for _, app := range result.ArgoApplications {
		appIndex[app.Name] = app
	}

	// child-explicit should have explicit confidence
	if appIndex["child-explicit"].LineageConfidence != "explicit" {
		t.Errorf("child-explicit confidence = %q, want explicit", appIndex["child-explicit"].LineageConfidence)
	}
	if appIndex["child-explicit"].ParentApplication != "platform-root" {
		t.Errorf("child-explicit parent = %q, want platform-root", appIndex["child-explicit"].ParentApplication)
	}

	// child-inferred should have inferred confidence
	if appIndex["child-inferred"].LineageConfidence != "inferred" {
		t.Errorf("child-inferred confidence = %q, want inferred", appIndex["child-inferred"].LineageConfidence)
	}

	// platform-root should list children
	rootResult := appIndex["platform-root"]
	if len(rootResult.ChildApplications) != 2 {
		t.Fatalf("platform-root children = %d, want 2", len(rootResult.ChildApplications))
	}
	if rootResult.ChildApplications[0] != "child-explicit" || rootResult.ChildApplications[1] != "child-inferred" {
		t.Errorf("platform-root children = %v, want [child-explicit child-inferred]", rootResult.ChildApplications)
	}
}

func TestResolveParentApplicationWithConfidence_OwnerRefTakesPrecedence(t *testing.T) {
	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      "child-app",
				"namespace": "argocd",
				"labels": map[string]interface{}{
					"app.kubernetes.io/part-of": "label-parent",
				},
				"annotations": map[string]interface{}{
					"cub-scout.io/parent-application": "annotation-parent",
				},
			},
		},
	}
	app.SetOwnerReferences([]metav1.OwnerReference{
		{APIVersion: "argoproj.io/v1alpha1", Kind: "Application", Name: "ownerref-parent"},
	})

	name, confidence := resolveParentApplicationWithConfidence(app)
	if name != "ownerref-parent" {
		t.Errorf("name = %q, want ownerref-parent", name)
	}
	if confidence != "explicit" {
		t.Errorf("confidence = %q, want explicit", confidence)
	}
}
