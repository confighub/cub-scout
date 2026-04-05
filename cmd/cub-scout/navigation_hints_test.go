package main

import (
	"encoding/json"
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

	out := renderDoctorASCII(summary, DefaultPresentationMode, false)

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

	out := renderExplainText(summary, DefaultPresentationMode, false)

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

	out := renderExplainMarkdown(summary, DefaultPresentationMode, false)

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

// Tests for the new Hint struct and rationale/priority system

func TestHintRationale_IsDeterministic(t *testing.T) {
	// Run the same hint generation multiple times and verify output is identical
	entries := []MapEntry{
		{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux", Status: "Ready"},
		{Kind: "Service", Name: "legacy", Namespace: "prod", Owner: "Native", Status: "Ready"},
	}
	byOwner := map[string]int{"Flux": 1, "Native": 1}

	// Generate hints twice
	hints1 := mapListTryNextHints(entries, byOwner, "prod")
	hints2 := mapListTryNextHints(entries, byOwner, "prod")

	if len(hints1) != len(hints2) {
		t.Fatalf("hint count changed: %d vs %d", len(hints1), len(hints2))
	}
	for i := range hints1 {
		if hints1[i] != hints2[i] {
			t.Fatalf("hint %d changed:\n  first:  %q\n  second: %q", i, hints1[i], hints2[i])
		}
	}
}

func TestHintRationale_ContainsWhyText(t *testing.T) {
	entries := []MapEntry{
		{Kind: "Service", Name: "legacy", Namespace: "prod", Owner: "Native", Status: "Ready"},
	}
	byOwner := map[string]int{"Native": 1}

	hints := mapListTryNextHints(entries, byOwner, "prod")

	// The orphan hint should explain WHY it matters
	found := false
	for _, h := range hints {
		if strings.Contains(h, "unmanaged") && strings.Contains(h, "GitOps ownership") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected rationale about GitOps ownership in hints:\n%v", hints)
	}
}

func TestHintPriority_CriticalIssueComesFirst(t *testing.T) {
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 10},
		Ownership: DoctorOwnershipSummary{Flux: 5, Native: 5, Unmanaged: 5},
		TopIssues: []DoctorIssue{
			{Severity: "CRITICAL", Resource: "Deployment/broken", Namespace: "prod", Message: "crash loop"},
		},
	}

	hints := doctorTryNextHints(summary)

	// First hint should be about the CRITICAL issue
	if len(hints) == 0 {
		t.Fatal("expected at least one hint")
	}
	if !strings.Contains(hints[0], "CRITICAL") {
		t.Fatalf("expected CRITICAL issue hint first, got: %q", hints[0])
	}
}

func TestHintPriority_HighUnmanagedCountBoostsPriority(t *testing.T) {
	// When there are many unmanaged resources, that hint should be prioritized
	entriesLow := []MapEntry{
		{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux", Status: "Ready"},
	}
	entriesHigh := make([]MapEntry, 15)
	for i := 0; i < 15; i++ {
		entriesHigh[i] = MapEntry{
			Kind:      "ConfigMap",
			Name:      fmt.Sprintf("cm-%d", i),
			Namespace: "prod",
			Owner:     "Native",
			Status:    "Ready",
		}
	}

	byOwnerLow := map[string]int{"Flux": 1, "Native": 1}
	byOwnerHigh := map[string]int{"Native": 15}

	hintsLow := mapListHints(entriesLow, byOwnerLow, "prod")
	hintsHigh := mapListHints(entriesHigh, byOwnerHigh, "prod")

	// Find orphan hint priority in both cases
	var priorityLow, priorityHigh int
	for _, h := range hintsLow {
		if strings.Contains(h.Command, "orphans") {
			priorityLow = h.Priority
			break
		}
	}
	for _, h := range hintsHigh {
		if strings.Contains(h.Command, "orphans") {
			priorityHigh = h.Priority
			break
		}
	}

	if priorityHigh <= priorityLow {
		t.Fatalf("expected high unmanaged count to boost priority: low=%d, high=%d", priorityLow, priorityHigh)
	}
}

func TestHintPriority_UnknownHealthBoostsTracePriority(t *testing.T) {
	summaryHealthy := ExplainSummary{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Owner:     "Flux",
		Health:    "Healthy",
	}
	summaryUnknown := ExplainSummary{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Owner:     "Flux",
		Health:    "Unavailable",
	}

	hintsHealthy := explainHints(summaryHealthy)
	hintsUnknown := explainHints(summaryUnknown)

	// Find trace hint priority in both cases
	var priorityHealthy, priorityUnknown int
	for _, h := range hintsHealthy {
		if strings.Contains(h.Command, "trace") {
			priorityHealthy = h.Priority
			break
		}
	}
	for _, h := range hintsUnknown {
		if strings.Contains(h.Command, "trace") {
			priorityUnknown = h.Priority
			break
		}
	}

	if priorityUnknown <= priorityHealthy {
		t.Fatalf("expected unknown health to boost trace priority: healthy=%d, unknown=%d", priorityHealthy, priorityUnknown)
	}
}

// Tests for ConfigHub URL hints (#350)

func TestExplainConfigHubHint_ReturnsHintWhenURLPresent(t *testing.T) {
	summary := ExplainSummary{
		Resource:     "Deployment/api",
		Namespace:    "prod",
		Owner:        "Flux",
		ConfigHubURL: "https://confighub.com/spaces/sp-123/units/payments-api",
	}

	hint := explainConfigHubHint(summary)

	if hint == nil {
		t.Fatal("expected ConfigHub hint when URL is present")
	}
	if hint.ConfigHubURL != summary.ConfigHubURL {
		t.Fatalf("expected URL %q, got %q", summary.ConfigHubURL, hint.ConfigHubURL)
	}
	if hint.Rationale == "" {
		t.Fatal("expected rationale to explain why GUI helps")
	}
}

func TestExplainConfigHubHint_ReturnsNilWhenNoURL(t *testing.T) {
	summary := ExplainSummary{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Owner:     "Flux",
		// No ConfigHubURL
	}

	hint := explainConfigHubHint(summary)

	if hint != nil {
		t.Fatalf("expected no ConfigHub hint when URL is absent, got: %+v", hint)
	}
}

func TestRenderConfigHubSection_RendersWhenHintPresent(t *testing.T) {
	hint := &Hint{
		ConfigHubURL: "https://confighub.com/spaces/sp-123/units/my-unit",
		Rationale:    "Review this unit for audit trail",
	}

	out := renderConfigHubSection(hint)

	required := []string{
		"OPEN IN CONFIGHUB:",
		"Review this unit",
		"https://confighub.com/spaces/sp-123/units/my-unit",
	}
	for _, s := range required {
		if !strings.Contains(out, s) {
			t.Fatalf("expected %q in output:\n%s", s, out)
		}
	}
}

func TestRenderConfigHubSection_EmptyWhenNoHint(t *testing.T) {
	out := renderConfigHubSection(nil)
	if out != "" {
		t.Fatalf("expected empty output for nil hint, got: %q", out)
	}

	out = renderConfigHubSection(&Hint{ConfigHubURL: ""})
	if out != "" {
		t.Fatalf("expected empty output for empty URL, got: %q", out)
	}
}

func TestRenderExplainText_IncludesConfigHubSection(t *testing.T) {
	summary := ExplainSummary{
		Resource:     "Deployment/payments-api",
		Namespace:    "prod",
		Owner:        "Flux",
		Source:       "https://github.com/acme/config",
		DeployedVia:  "GitRepository -> Kustomization -> Deployment",
		Health:       "Healthy",
		Risks:        "0 findings",
		Drift:        "None",
		ConfigHubURL: "https://confighub.com/spaces/sp-abc/units/payments",
	}

	out := renderExplainText(summary, DefaultPresentationMode, false)

	// Should have TRY NEXT section
	if !strings.Contains(out, "TRY NEXT:") {
		t.Fatalf("expected TRY NEXT section in output:\n%s", out)
	}
	// Should have OPEN IN CONFIGHUB section
	if !strings.Contains(out, "OPEN IN CONFIGHUB:") {
		t.Fatalf("expected OPEN IN CONFIGHUB section in output:\n%s", out)
	}
	if !strings.Contains(out, "https://confighub.com/spaces/sp-abc/units/payments") {
		t.Fatalf("expected ConfigHub URL in output:\n%s", out)
	}
}

func TestRenderExplainText_NoConfigHubSectionWithoutURL(t *testing.T) {
	summary := ExplainSummary{
		Resource:    "Deployment/payments-api",
		Namespace:   "prod",
		Owner:       "Flux",
		Source:      "https://github.com/acme/config",
		DeployedVia: "GitRepository -> Kustomization -> Deployment",
		Health:      "Healthy",
		Risks:       "0 findings",
		Drift:       "None",
		// No ConfigHubURL
	}

	out := renderExplainText(summary, DefaultPresentationMode, false)

	// Should have TRY NEXT section
	if !strings.Contains(out, "TRY NEXT:") {
		t.Fatalf("expected TRY NEXT section in output:\n%s", out)
	}
	// Should NOT have OPEN IN CONFIGHUB section
	if strings.Contains(out, "OPEN IN CONFIGHUB:") {
		t.Fatalf("did not expect OPEN IN CONFIGHUB section when no URL:\n%s", out)
	}
}

func TestExplainSummary_JSONOmitsEmptyConfigHubURL(t *testing.T) {
	summary := ExplainSummary{
		Resource:    "Deployment/api",
		Namespace:   "prod",
		Owner:       "Flux",
		Source:      "https://github.com/example",
		DeployedVia: "GitRepository -> Deployment",
		Health:      "Healthy",
		Risks:       "0 findings",
		Drift:       "None",
		// No ConfigHubURL - should be omitted from JSON
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, "confighubUrl") {
		t.Fatalf("expected confighubUrl to be omitted when empty, got: %s", jsonStr)
	}
}

func TestExplainSummary_JSONIncludesConfigHubURL(t *testing.T) {
	summary := ExplainSummary{
		Resource:     "Deployment/api",
		Namespace:    "prod",
		Owner:        "Flux",
		Source:       "https://github.com/example",
		DeployedVia:  "GitRepository -> Deployment",
		Health:       "Healthy",
		Risks:        "0 findings",
		Drift:        "None",
		ConfigHubURL: "https://confighub.com/spaces/sp-123/units/my-unit",
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"confighubUrl":"https://confighub.com/spaces/sp-123/units/my-unit"`) {
		t.Fatalf("expected confighubUrl in JSON output, got: %s", jsonStr)
	}
}

func TestHintRationale_SingularVsPlural(t *testing.T) {
	// Test that rationale uses correct grammar for 1 vs multiple resources
	byOwner1 := map[string]int{"Native": 1}
	byOwner5 := map[string]int{"Native": 5}

	hints1 := mapListTryNextHints(nil, byOwner1, "prod")
	hints5 := mapListTryNextHints(nil, byOwner5, "prod")

	var rationale1, rationale5 string
	for _, h := range hints1 {
		if strings.Contains(h, "orphans") {
			rationale1 = h
			break
		}
	}
	for _, h := range hints5 {
		if strings.Contains(h, "orphans") {
			rationale5 = h
			break
		}
	}

	if !strings.Contains(rationale1, "1 unmanaged resource") {
		t.Fatalf("expected singular 'resource' for count=1, got: %q", rationale1)
	}
	if !strings.Contains(rationale5, "5 unmanaged resources") {
		t.Fatalf("expected plural 'resources' for count=5, got: %q", rationale5)
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
