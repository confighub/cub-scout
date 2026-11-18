package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

func TestKnownManagerToolNamesTerraformsFieldManagersOnly(t *testing.T) {
	// Recorded in CI run 44265228493 with hashicorp/kubernetes 3.5.0.
	for manager, want := range map[string]string{
		"HashiCorp":                 "Terraform's Kubernetes provider (typed resources)",
		"Terraform":                 "Terraform's Kubernetes provider (kubernetes_manifest)",
		"terraform":                 "",
		"kubectl-client-side-apply": "",
		"":                          "",
	} {
		if got := agent.KnownManagerTool(manager); got != want {
			t.Errorf("KnownManagerTool(%q) = %q, want %q", manager, got, want)
		}
	}
}

func TestExplainCitesTerraformsFieldManagerWithoutClaimingOwnership(t *testing.T) {
	summary := ExplainSummary{
		Resource:            "Deployment/tf-manifest",
		Namespace:           "tf-lane",
		Owner:               "Native",
		Source:              "unknown",
		MutationCause:       agent.FieldMutationCause("unknown"),
		MutationManager:     "Terraform",
		MutationManagerTool: agent.KnownManagerTool("Terraform"),
	}
	for name, out := range map[string]string{
		"text":     renderExplainText(summary, PresentationHuman, true, HintContext{Mode: HintModeDefault}),
		"markdown": renderExplainMarkdown(summary, PresentationHuman, true, HintContext{Mode: HintModeDefault}),
	} {
		if !strings.Contains(out, "Terraform's Kubernetes provider (kubernetes_manifest) writes with that field manager, which is not an owner marker") {
			t.Errorf("%s: the field manager's tool is not cited:\n%s", name, out)
		}
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"owner":"Native"`) || !strings.Contains(string(raw), `"mutationManagerTool":"Terraform's Kubernetes provider (kubernetes_manifest)"`) {
		t.Errorf("JSON = %s", raw)
	}

	summary.MutationManager, summary.MutationManagerTool = "kubectl-edit", agent.KnownManagerTool("kubectl-edit")
	raw, _ = json.Marshal(summary)
	if strings.Contains(string(raw), "mutationManagerTool") {
		t.Errorf("mutationManagerTool present for a manager no tool is known by: %s", raw)
	}
}
