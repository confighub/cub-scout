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
	// Use a cluster with few unmanaged resources (below native-heavy threshold)
	// so quickstart appears in top 3 hints
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 10},
		Ownership: DoctorOwnershipSummary{Flux: 6, ArgoCD: 2, Native: 2, Unmanaged: 2},
		TopIssues: []DoctorIssue{
			{Severity: "CRITICAL", Resource: "Deployment/payments-api", Namespace: "prod", Message: "missing limits"},
		},
	}

	out := renderDoctorASCII(summary, DefaultPresentationMode, false, DefaultHintContext())

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

	out := renderExplainText(summary, DefaultPresentationMode, false, DefaultHintContext())

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

	out := renderExplainMarkdown(summary, DefaultPresentationMode, false, DefaultHintContext())

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

	out := renderExplainText(summary, DefaultPresentationMode, false, DefaultHintContext())

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

	out := renderExplainText(summary, DefaultPresentationMode, false, DefaultHintContext())

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

// Tests for HintContext and mode-aware ranking (#349)

func TestHintContext_DefaultMode(t *testing.T) {
	ctx := DefaultHintContext()
	if ctx.Mode != HintModeDefault {
		t.Fatalf("expected default mode, got %q", ctx.Mode)
	}
	if !ctx.isBeginnerMode() {
		t.Fatal("default mode should be considered beginner mode")
	}
	if ctx.isOperatorMode() {
		t.Fatal("default mode should not be considered operator mode")
	}
}

func TestHintContext_ModeClassification(t *testing.T) {
	tests := []struct {
		mode       HintMode
		isBeginner bool
		isOperator bool
	}{
		{HintModeDefault, true, false},
		{HintModeBeginner, true, false},
		{HintModeOperator, false, true},
		{HintModeDemo, false, true},
	}

	for _, tc := range tests {
		ctx := HintContext{Mode: tc.mode}
		if ctx.isBeginnerMode() != tc.isBeginner {
			t.Errorf("mode %q: isBeginnerMode()=%v, want %v", tc.mode, ctx.isBeginnerMode(), tc.isBeginner)
		}
		if ctx.isOperatorMode() != tc.isOperator {
			t.Errorf("mode %q: isOperatorMode()=%v, want %v", tc.mode, ctx.isOperatorMode(), tc.isOperator)
		}
	}
}

func TestDoctorHints_QuickstartSuppressedInOperatorMode(t *testing.T) {
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 10},
		Ownership: DoctorOwnershipSummary{Flux: 5, ArgoCD: 3, Native: 2, Unmanaged: 2},
	}

	// In default mode, quickstart should have low but not suppressed priority
	defaultCtx := HintContext{Mode: HintModeDefault}
	defaultHints := doctorHintsWithContext(summary, defaultCtx)

	var quickstartPriorityDefault int
	for _, h := range defaultHints {
		if strings.Contains(h.Command, "quickstart") {
			quickstartPriorityDefault = h.Priority
			break
		}
	}

	// In operator mode, quickstart should be suppressed
	operatorCtx := HintContext{Mode: HintModeOperator}
	operatorHints := doctorHintsWithContext(summary, operatorCtx)

	var quickstartPriorityOperator int
	for _, h := range operatorHints {
		if strings.Contains(h.Command, "quickstart") {
			quickstartPriorityOperator = h.Priority
			break
		}
	}

	if quickstartPriorityOperator >= quickstartPriorityDefault {
		t.Fatalf("quickstart priority should be lower in operator mode: default=%d, operator=%d",
			quickstartPriorityDefault, quickstartPriorityOperator)
	}
	if quickstartPriorityOperator != hintPrioritySuppressed {
		t.Fatalf("quickstart priority in operator mode should be suppressed (%d), got %d",
			hintPrioritySuppressed, quickstartPriorityOperator)
	}
}

func TestDoctorHints_QuickstartBoostedInBeginnerMode(t *testing.T) {
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 5},
		Ownership: DoctorOwnershipSummary{Flux: 3, Native: 2, Unmanaged: 2},
	}

	// In beginner mode, quickstart should be boosted
	beginnerCtx := HintContext{Mode: HintModeBeginner}
	beginnerHints := doctorHintsWithContext(summary, beginnerCtx)

	var quickstartPriorityBeginner int
	for _, h := range beginnerHints {
		if strings.Contains(h.Command, "quickstart") {
			quickstartPriorityBeginner = h.Priority
			break
		}
	}

	// In default mode, quickstart should have lower priority
	defaultCtx := HintContext{Mode: HintModeDefault}
	defaultHints := doctorHintsWithContext(summary, defaultCtx)

	var quickstartPriorityDefault int
	for _, h := range defaultHints {
		if strings.Contains(h.Command, "quickstart") {
			quickstartPriorityDefault = h.Priority
			break
		}
	}

	if quickstartPriorityBeginner <= quickstartPriorityDefault {
		t.Fatalf("quickstart priority should be higher in beginner mode: beginner=%d, default=%d",
			quickstartPriorityBeginner, quickstartPriorityDefault)
	}
	if quickstartPriorityBeginner != hintPriorityNormal {
		t.Fatalf("quickstart priority in beginner mode should be normal (%d), got %d",
			hintPriorityNormal, quickstartPriorityBeginner)
	}
}

func TestDoctorHints_ImportHintForNativeHeavyCluster(t *testing.T) {
	// A cluster with many unmanaged resources should get an import hint
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 10},
		Ownership: DoctorOwnershipSummary{Flux: 2, Native: 8, Unmanaged: 8},
	}

	hints := doctorHintsWithContext(summary, DefaultHintContext())

	var hasImportHint bool
	for _, h := range hints {
		if strings.Contains(h.Command, "import --dry-run") {
			hasImportHint = true
			break
		}
	}

	if !hasImportHint {
		t.Fatal("expected import --dry-run hint for native-heavy cluster")
	}
}

func TestDoctorHints_NoImportHintForWellManagedCluster(t *testing.T) {
	// A cluster with few unmanaged resources should not get an import hint
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 20},
		Ownership: DoctorOwnershipSummary{Flux: 15, ArgoCD: 3, Native: 2, Unmanaged: 2},
	}

	hints := doctorHintsWithContext(summary, DefaultHintContext())

	for _, h := range hints {
		if strings.Contains(h.Command, "import --dry-run") {
			t.Fatalf("did not expect import hint for well-managed cluster, got: %q", h.Command)
		}
	}
}

func TestDoctorHints_ImportHintBoostedInOperatorMode(t *testing.T) {
	// When native-heavy and in operator mode, import hint should be boosted.
	// Use a ratio between 0.3 and 0.5 so only operator mode triggers boost.
	// 5 unmanaged out of 12 = 41.6%, which is > 0.3 (native-heavy) but <= 0.5 (no auto-boost)
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 12},
		Ownership: DoctorOwnershipSummary{Flux: 7, Native: 5, Unmanaged: 5},
	}

	defaultHints := doctorHintsWithContext(summary, HintContext{Mode: HintModeDefault})
	operatorHints := doctorHintsWithContext(summary, HintContext{Mode: HintModeOperator})

	var importPriorityDefault, importPriorityOperator int
	for _, h := range defaultHints {
		if strings.Contains(h.Command, "import --dry-run") {
			importPriorityDefault = h.Priority
			break
		}
	}
	for _, h := range operatorHints {
		if strings.Contains(h.Command, "import --dry-run") {
			importPriorityOperator = h.Priority
			break
		}
	}

	if importPriorityOperator <= importPriorityDefault {
		t.Fatalf("import hint priority should be higher in operator mode: default=%d, operator=%d",
			importPriorityDefault, importPriorityOperator)
	}
}

func TestExplainHints_DoctorSuppressedInOperatorMode(t *testing.T) {
	summary := ExplainSummary{
		Resource:  "Deployment/api",
		Namespace: "prod",
		Owner:     "Flux",
		Health:    "Healthy",
	}

	defaultHints := explainHintsWithContext(summary, HintContext{Mode: HintModeDefault})
	operatorHints := explainHintsWithContext(summary, HintContext{Mode: HintModeOperator})

	var doctorPriorityDefault, doctorPriorityOperator int
	for _, h := range defaultHints {
		if strings.Contains(h.Command, "doctor") {
			doctorPriorityDefault = h.Priority
			break
		}
	}
	for _, h := range operatorHints {
		if strings.Contains(h.Command, "doctor") {
			doctorPriorityOperator = h.Priority
			break
		}
	}

	if doctorPriorityOperator >= doctorPriorityDefault {
		t.Fatalf("doctor hint priority should be lower in operator mode: default=%d, operator=%d",
			doctorPriorityDefault, doctorPriorityOperator)
	}
	if doctorPriorityOperator != hintPrioritySuppressed {
		t.Fatalf("doctor priority in operator mode should be suppressed (%d), got %d",
			hintPrioritySuppressed, doctorPriorityOperator)
	}
}

func TestExplainHints_ImportHintInOperatorModeForUnknownOwner(t *testing.T) {
	summary := ExplainSummary{
		Resource:  "Deployment/legacy-app",
		Namespace: "prod",
		Owner:     "Unknown - no recognized ownership labels found",
		Health:    "Healthy",
	}

	// In operator mode, should suggest import for unknown-owner resources
	operatorHints := explainHintsWithContext(summary, HintContext{Mode: HintModeOperator})

	var hasImportHint bool
	for _, h := range operatorHints {
		if strings.Contains(h.Command, "import --dry-run") {
			hasImportHint = true
			break
		}
	}

	if !hasImportHint {
		t.Fatal("expected import --dry-run hint for unknown-owner resource in operator mode")
	}

	// In default mode, should NOT suggest import
	defaultHints := explainHintsWithContext(summary, HintContext{Mode: HintModeDefault})

	for _, h := range defaultHints {
		if strings.Contains(h.Command, "import --dry-run") {
			t.Fatalf("did not expect import hint for unknown-owner resource in default mode, got: %q", h.Command)
		}
	}
}

func TestParseHintMode(t *testing.T) {
	tests := []struct {
		input    string
		expected HintMode
		wantErr  bool
	}{
		{"", HintModeDefault, false},
		{"default", HintModeDefault, false},
		{"DEFAULT", HintModeDefault, false},
		{"beginner", HintModeBeginner, false},
		{"Beginner", HintModeBeginner, false},
		{"operator", HintModeOperator, false},
		{"OPERATOR", HintModeOperator, false},
		{"demo", HintModeDefault, true},  // demo not exposed via flag (no distinct behavior yet)
		{"invalid", HintModeDefault, true},
		{"foo", HintModeDefault, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			mode, err := ParseHintMode(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for input %q: %v", tc.input, err)
				return
			}
			if mode != tc.expected {
				t.Errorf("ParseHintMode(%q) = %q, want %q", tc.input, mode, tc.expected)
			}
		})
	}
}

func TestHintContext_DeterministicAcrossModes(t *testing.T) {
	summary := DoctorSummary{
		Cluster:   "kind-dev",
		Namespace: "prod",
		Resources: DoctorResourceSummary{Total: 10},
		Ownership: DoctorOwnershipSummary{Flux: 5, Native: 5, Unmanaged: 5},
		TopIssues: []DoctorIssue{
			{Severity: "WARNING", Resource: "Deployment/api", Namespace: "prod", Message: "no limits"},
		},
	}

	// Run multiple times with same context - should be identical
	for _, mode := range []HintMode{HintModeDefault, HintModeBeginner, HintModeOperator, HintModeDemo} {
		ctx := HintContext{Mode: mode}
		hints1 := doctorTryNextHintsWithContext(summary, ctx)
		hints2 := doctorTryNextHintsWithContext(summary, ctx)

		if len(hints1) != len(hints2) {
			t.Fatalf("mode %q: hint count changed: %d vs %d", mode, len(hints1), len(hints2))
		}
		for i := range hints1 {
			if hints1[i] != hints2[i] {
				t.Fatalf("mode %q: hint %d changed:\n  first:  %q\n  second: %q", mode, i, hints1[i], hints2[i])
			}
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
