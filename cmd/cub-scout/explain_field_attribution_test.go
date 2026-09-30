package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/confighub/cub-scout/pkg/agent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func fieldObject(entries ...metav1.ManagedFieldsEntry) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{}}}
	obj.SetManagedFields(entries)
	return obj
}

func fieldEntry(manager, raw string) metav1.ManagedFieldsEntry {
	return metav1.ManagedFieldsEntry{Manager: manager, FieldsType: "FieldsV1", FieldsV1: &metav1.FieldsV1{Raw: []byte(raw)}}
}

func TestFieldAttributionSummaryUsesOnlyRequestedPath(t *testing.T) {
	path := `.spec.template.spec.containers[name="checkout"].image`
	obj := fieldObject(
		fieldEntry(agent.ManagerKubectlLabel, `{"f:metadata":{"f:labels":{"f:app":{}}}}`),
		fieldEntry(agent.ManagerFluxKustomize, `{"f:spec":{"f:template":{"f:spec":{"f:containers":{"k:{\"name\":\"checkout\"}":{"f:image":{}}}}}}}`),
	)
	result := fieldAttributionSummary(obj, agent.Ownership{Type: agent.OwnerFlux}, path)
	if result.Path != path || result.Cause != agent.CauseControllerDrift || len(result.Managers) != 1 || result.Managers[0] != agent.ManagerFluxKustomize {
		t.Fatalf("field result = %+v, want controller-only path evidence", result)
	}
	if result.Reason != "" {
		t.Fatalf("Reason = %q, want empty for classified path", result.Reason)
	}
}

func TestFieldAttributionSummaryMissingPathDoesNotUseResourceRollup(t *testing.T) {
	obj := fieldObject(fieldEntry(agent.ManagerKubectlEdit, `{"f:metadata":{"f:labels":{"f:app":{}}}}`))
	result := fieldAttributionSummary(obj, agent.Ownership{Type: agent.OwnerFlux}, `.spec.replicas`)
	if result.Cause != agent.CauseUnknown || len(result.Managers) != 0 || result.Reason == "" {
		t.Fatalf("missing path result = %+v, want unknown with reason and no fallback manager", result)
	}
}

func TestFieldAttributionIncompleteEvidenceIsUnknownWithObservedManagers(t *testing.T) {
	result := fieldAttributionResult(`.spec.image`, agent.FieldMutationAttribution{
		Cause: agent.CauseManualEdit, Managers: []string{agent.ManagerKubectlSet},
	}, true, true)
	if result.Cause != agent.CauseUnknown || len(result.Managers) != 1 || result.Managers[0] != agent.ManagerKubectlSet || result.Reason == "" {
		t.Fatalf("incomplete path result = %+v, want unknown with observed managers and reason", result)
	}
}

func TestFieldAttributionUnknownManagerIsVisible(t *testing.T) {
	obj := fieldObject(fieldEntry("vendor-writer", `{"f:spec":{"f:image":{}}}`))
	result := fieldAttributionSummary(obj, agent.Ownership{Type: agent.OwnerFlux}, `.spec.image`)
	if result.Cause != agent.CauseUnknown || len(result.Managers) != 1 || result.Managers[0] != "vendor-writer" || result.Reason == "" {
		t.Fatalf("unrecognized manager result = %+v, want unknown with manager and reason", result)
	}
}

func TestFieldAttributionDefaultJSONRemainsOmitted(t *testing.T) {
	encoded, err := json.Marshal(ExplainSummary{Resource: "Deployment/api"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "fieldAttribution") {
		t.Fatalf("default ExplainSummary unexpectedly emits fieldAttribution: %s", encoded)
	}
}

func TestFieldAttributionRendersInTextAndMarkdown(t *testing.T) {
	summary := ExplainSummary{
		Resource: "Deployment/checkout", Namespace: "shop", Owner: "Flux", Source: "recorded",
		Health: "Unknown", Risks: "None", Drift: "Unknown",
		FieldAttribution: &FieldAttributionSummary{
			Path:  `.spec.template.spec.containers[name="checkout"].image`,
			Cause: agent.CauseManualEdit, Managers: []string{agent.ManagerKubectlSet},
		},
	}
	for name, output := range map[string]string{
		"text": renderExplainText(summary, PresentationHuman, false, HintContext{Mode: HintModeDefault}),
		"md":   renderExplainMarkdown(summary, PresentationHuman, false, HintContext{Mode: HintModeDefault}),
	} {
		output = ansi.Strip(output)
		if !strings.Contains(output, summary.FieldAttribution.Path) || !strings.Contains(output, agent.ManagerKubectlSet) {
			t.Errorf("%s omitted selected path manager evidence: %s", name, output)
		}
	}
}
