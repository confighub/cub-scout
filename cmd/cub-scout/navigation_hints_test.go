package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderMapListFromEntries_AddsTryNextHintsInASCII(t *testing.T) {
	restore := withMapListFlagsForNavigationTest()
	defer restore()

	mapListFormat = "ascii"
	mapNamespace = "prod"
	mapExplain = false

	entries := []MapEntry{
		{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Owner: "Flux", Status: "Ready"},
		{Kind: "Service", Name: "legacy", Namespace: "prod", Owner: "Native", Status: "Ready"},
	}

	out := captureStdout(t, func() {
		if err := renderMapListFromEntries(entries); err != nil {
			t.Fatalf("renderMapListFromEntries failed: %v", err)
		}
	})

	required := []string{
		"TRY NEXT:",
		"cub-scout map orphans -n prod",
		"cub-scout explain deployment/payments-api -n prod",
		"cub-scout doctor -n prod",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in output:\n%s", s, out)
		}
	}
}

func TestRenderMapListFromEntries_NoHintsInJSON(t *testing.T) {
	restore := withMapListFlagsForNavigationTest()
	defer restore()

	mapListFormat = "json"
	mapNamespace = "prod"
	mapExplain = false

	entries := []MapEntry{
		{Kind: "Deployment", Name: "payments-api", Namespace: "prod", Owner: "Flux", Status: "Ready"},
	}

	out := captureStdout(t, func() {
		if err := renderMapListFromEntries(entries); err != nil {
			t.Fatalf("renderMapListFromEntries failed: %v", err)
		}
	})

	if strings.Contains(out, "TRY NEXT:") {
		t.Fatalf("did not expect navigation hints in JSON output:\n%s", out)
	}
	if !strings.Contains(out, "\"name\": \"payments-api\"") {
		t.Fatalf("expected JSON output, got:\n%s", out)
	}
}

func TestRenderDoctorASCII_AddsTryNextHints(t *testing.T) {
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 5},
		Ownership: DoctorOwnershipSummary{Flux: 2, Native: 3, Unmanaged: 3},
		TopIssues: []DoctorIssue{
			{Severity: "CRITICAL", Resource: "Deployment/payments-api", Namespace: "prod", Message: "missing limits"},
		},
	}

	out := renderDoctorASCII(summary)

	required := []string{
		"TRY NEXT:",
		"cub-scout map orphans -n prod",
		"cub-scout explain deployment/payments-api -n prod",
		"cub-scout quickstart -n prod --yes",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in doctor output:\n%s", s, out)
		}
	}
}

func TestRenderExplainText_AddsTryNextHints(t *testing.T) {
	summary := ExplainSummary{
		Resource:    "Deployment/payments-api",
		Namespace:   "prod",
		Owner:       "Flux",
		Source:      "https://github.com/acme/platform",
		DeployedVia: "GitRepository/platform -> Kustomization/payments -> Deployment/payments-api",
		Health:      "Healthy",
		Risks:       "0 findings",
		Drift:       "None",
	}

	out := renderExplainText(summary)

	required := []string{
		"TRY NEXT:",
		"cub-scout trace deployment/payments-api -n prod --explain",
		"cub-scout map list -n prod -q \"owner=Flux\"",
		"cub-scout doctor -n prod",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in explain text output:\n%s", s, out)
		}
	}
}

func TestRenderExplainMarkdown_AddsTryNextSection(t *testing.T) {
	summary := ExplainSummary{
		Resource:    "Deployment/payments-api",
		Namespace:   "prod",
		Owner:       "Unknown - no recognized ownership labels found",
		Source:      "unknown",
		DeployedVia: "partial trace only",
		Health:      "Unavailable",
		Risks:       "Not assessed",
		Drift:       "Unknown",
	}

	out := renderExplainMarkdown(summary)

	required := []string{
		"### Try Next",
		"`cub-scout map orphans -n prod`",
		"`cub-scout map issues -n prod`",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in explain markdown output:\n%s", s, out)
		}
	}
}

func TestWithKubeRecoveryHint_AddsActionableCommands(t *testing.T) {
	err := withKubeRecoveryHint(fmt.Errorf("build kubernetes config: no configuration has been provided"), "cub-scout map list")

	required := []string{
		"Recovery:",
		"kubectl config current-context",
		"cub-scout quickstart",
		"cub-scout map list --help",
	}
	for _, s := range required {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("expected %q in wrapped error:\n%s", s, err.Error())
		}
	}
}

func withMapListFlagsForNavigationTest() func() {
	prevNamespace := mapNamespace
	prevKind := mapKind
	prevOwner := mapOwner
	prevQuery := mapQuery
	prevJSON := mapJSON
	prevFormat := mapListFormat
	prevVerbose := mapVerbose
	prevCount := mapCount
	prevNamesOnly := mapNamesOnly
	prevExplain := mapExplain

	mapNamespace = ""
	mapKind = ""
	mapOwner = ""
	mapQuery = ""
	mapJSON = false
	mapListFormat = "ascii"
	mapVerbose = false
	mapCount = false
	mapNamesOnly = false
	mapExplain = false

	return func() {
		mapNamespace = prevNamespace
		mapKind = prevKind
		mapOwner = prevOwner
		mapQuery = prevQuery
		mapJSON = prevJSON
		mapListFormat = prevFormat
		mapVerbose = prevVerbose
		mapCount = prevCount
		mapNamesOnly = prevNamesOnly
		mapExplain = prevExplain
	}
}
