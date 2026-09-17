// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/confighub/cub-scout/pkg/agent"
)

// stubSpaceInputs fixes CUB_SPACE, the one environment input the resolver reads.
func stubSpaceInputs(t *testing.T, cubSpaceEnv string) {
	t.Helper()
	t.Setenv("CUB_SPACE", cubSpaceEnv)
}

// fakeCubRecorder puts a fake cub first on PATH. It records every argument
// vector it receives, one per line, and answers `context get` with a context
// whose config still carries a defaultSpace, as files written by cub before
// v0.5.2 do. Every other call prints an empty JSON list.
func fakeCubRecorder(t *testing.T) (logPath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> \"" + logPath + "\"\n" +
		"case \"$1 $2\" in\n" +
		"  \"context get\") echo '{\"name\":\"old\",\"settings\":{\"defaultSpace\":\"stale-default\"}}' ;;\n" +
		"  *) echo '[]' ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func fakeCubCalls(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func TestResolveConfigHubSpace_Precedence(t *testing.T) {
	tests := []struct {
		name                 string
		flag, env            string
		wantSlug, wantSource string
	}{
		{name: "flag wins over CUB_SPACE", flag: "from-flag", env: "from-env", wantSlug: "from-flag", wantSource: spaceSourceFlag},
		{name: "CUB_SPACE when no flag", env: "from-env", wantSlug: "from-env", wantSource: spaceSourceEnv},
		{name: "every space is honoured when it is asked for", flag: "*", wantSlug: "*", wantSource: spaceSourceFlag},
		{name: "whitespace is not a space", flag: "  ", env: " "},
		// The case that must never widen: nothing given. The answer is "unset",
		// never "*".
		{name: "nothing given resolves to unset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubSpaceInputs(t, tt.env)
			got := resolveConfigHubSpace(tt.flag)
			if got.Slug != tt.wantSlug || got.Source != tt.wantSource {
				t.Fatalf("resolveConfigHubSpace = %+v, want %s from %s", got, tt.wantSlug, tt.wantSource)
			}
			if got.IsSet() != (tt.wantSlug != "") {
				t.Fatalf("IsSet() = %v for %+v", got.IsSet(), got)
			}
		})
	}
}

// cub stopped using a context default space in v0.5.2, but older config files
// still carry one. The resolver must not read it: cub-scout would scope its
// reads to a space cub itself ignores.
func TestResolveConfigHubSpace_NeverReadsTheCubContext(t *testing.T) {
	logPath := fakeCubRecorder(t)
	stubSpaceInputs(t, "")

	if got := resolveConfigHubSpace(""); got.IsSet() {
		t.Fatalf("resolveConfigHubSpace = %+v, want unset: a defaultSpace left in the cub config is not a space anyone named", got)
	}
	if _, err := requireConfigHubSpace("history", "--space", ""); err == nil {
		t.Fatal("requireConfigHubSpace found a space although none was named")
	}
	if calls := fakeCubCalls(t, logPath); len(calls) != 0 {
		t.Fatalf("resolving a space ran cub %v; it must not consult cub at all", calls)
	}
}

func TestRequireConfigHubSpace_RefusesRatherThanWiden(t *testing.T) {
	stubSpaceInputs(t, "")
	space, err := requireConfigHubSpace("history", "--space", "")
	if err == nil {
		t.Fatalf("space = %+v, want a refusal when no space is given anywhere", space)
	}
	for _, want := range []string{"history", "--space <slug>", "--space '*'", "CUB_SPACE=<slug>", "cub has no default space"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "context") {
		t.Fatalf("error = %q: it must not claim anything about the cub context, which was not read", err)
	}

	stubSpaceInputs(t, "payments-prod")
	if space, err := requireConfigHubSpace("history", "--space", ""); err != nil || space.Slug != "payments-prod" || space.Source != spaceSourceEnv {
		t.Fatalf("space = %+v err = %v, want payments-prod from CUB_SPACE", space, err)
	}
}

func TestRequireSingleConfigHubSpace(t *testing.T) {
	stubSpaceInputs(t, "")
	_, err := requireSingleConfigHubSpace("fleet outliers", "--space", "", "slugs are unique only within a space")
	if err == nil || !strings.Contains(err.Error(), "--space <slug>") || strings.Contains(err.Error(), "'*'") {
		t.Fatalf("err = %v, want a refusal that does not offer '*'", err)
	}
	if _, err := requireSingleConfigHubSpace("fleet outliers", "--space", "*", "slugs are unique only within a space"); err == nil ||
		!strings.Contains(err.Error(), "not '*' (from flag)") || !strings.Contains(err.Error(), "slugs are unique only within a space") {
		t.Fatalf("err = %v, want every-space refused with the reason and where '*' came from", err)
	}
	stubSpaceInputs(t, "*")
	if _, err := requireSingleConfigHubSpace("fleet outliers", "--space", "", "why"); err == nil || !strings.Contains(err.Error(), "(from CUB_SPACE)") {
		t.Fatalf("err = %v, want every-space from CUB_SPACE refused too", err)
	}
	stubSpaceInputs(t, "payments")
	if space, err := requireSingleConfigHubSpace("fleet outliers", "--space", "", "why"); err != nil || space.Slug != "payments" {
		t.Fatalf("space = %+v err = %v, want payments", space, err)
	}
}

func TestWithConfigHubSpace(t *testing.T) {
	got := withConfigHubSpace([]string{"unit", "list", "--json"}, " payments-prod ")
	if strings.Join(got, " ") != "unit list --json --space payments-prod" {
		t.Fatalf("args = %v", got)
	}

	// cub reads `--space ""` as every space in the organization. An empty space
	// must fail closed instead: cub refuses a space that cannot exist.
	got = withConfigHubSpace([]string{"unit", "list"}, " ")
	if strings.Join(got, " ") != "unit list --space "+unresolvedConfigHubSpace {
		t.Fatalf("args = %v, want the unresolved placeholder", got)
	}

	// The caller's backing array is never written through.
	base := make([]string, 2, 8)
	copy(base, []string{"unit", "list"})
	first := withConfigHubSpace(base, "a")
	second := withConfigHubSpace(base, "b")
	if first[3] != "a" || second[3] != "b" {
		t.Fatalf("first = %v second = %v: one call overwrote the other's space", first, second)
	}
}

func TestWithConfigHubSpaceOrTargets(t *testing.T) {
	const targetID = "11111111-2222-4333-8444-555555555555"
	tests := []struct {
		name    string
		space   string
		targets []string
		want    string
		wantErr string
	}{
		{name: "space only", space: "prod", want: "k8s get deployments --space prod"},
		{name: "space and bare target", space: "prod", targets: []string{"cluster-a"}, want: "k8s get deployments --space prod --target cluster-a"},
		{name: "qualified target needs no space", targets: []string{"prod/cluster-a"}, want: "k8s get deployments --target prod/cluster-a"},
		{name: "target UUID needs no space", targets: []string{targetID}, want: "k8s get deployments --target " + targetID},
		{name: "qualified comma list", targets: []string{"prod/a,stage/b"}, want: "k8s get deployments --target prod/a,stage/b"},
		{name: "bare target without a space is refused", targets: []string{"cluster-a"}, wantErr: `target "cluster-a" names no space`},
		{name: "an empty space part is not a space", targets: []string{"/cluster-a"}, wantErr: `target "/cluster-a" names no space`},
		{name: "cub splits on commas, so each part is checked", targets: []string{"prod/a,stage"}, wantErr: `target "stage" names no space`},
		{name: "every space does not scope a bare target", space: "*", targets: []string{"cluster-a"}, wantErr: `target "cluster-a" names no space`},
		{name: "every space with a qualified target", space: "*", targets: []string{"prod/cluster-a"}, want: "k8s get deployments --space * --target prod/cluster-a"},
		{name: "empty entry", targets: []string{"prod/a,"}, wantErr: "has an empty entry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := withConfigHubSpaceOrTargets([]string{"k8s", "get", "deployments"}, tt.space, tt.targets)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || strings.Join(got, " ") != tt.want {
				t.Fatalf("args = %v err = %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestWithConfigHubSpaceFromRef(t *testing.T) {
	for ref, want := range map[string]string{
		"payments/api":                         "unit get --json payments/api",
		"11111111-2222-4333-8444-555555555555": "unit get --json 11111111-2222-4333-8444-555555555555",
		"api":                                  "unit get --json api --space " + unresolvedConfigHubSpace,
	} {
		if got := strings.Join(withConfigHubSpaceFromRef([]string{"unit", "get", "--json"}, ref), " "); got != want {
			t.Errorf("ref %q: args = %q, want %q", ref, got, want)
		}
	}
}

// An agent cannot see the environment of the MCP server it calls, so a tool
// call names its space itself; CUB_SPACE on the server does not supply one.
func TestMCPConfigHubSpace(t *testing.T) {
	stubSpaceInputs(t, "from-server-env")
	if _, err := mcpConfigHubSpace("confighub_units", map[string]interface{}{}); err == nil || !strings.Contains(err.Error(), "`space` argument") {
		t.Fatalf("err = %v, want a refusal naming the space argument even with CUB_SPACE set", err)
	}
	if space, err := mcpConfigHubSpace("confighub_units", map[string]interface{}{"space": "prod"}); err != nil || space != "prod" {
		t.Fatalf("space = %q err = %v, want prod", space, err)
	}
}

func TestMCPConfighubUnitGet_NamesTheSpace(t *testing.T) {
	stubSpaceInputs(t, "")
	gateway := newMCPGatewayWithMode(nil, nil, true)
	build := gateway.tools["confighub_unit_get"].BuildArgs
	for _, tt := range []struct {
		name    string
		args    map[string]interface{}
		want    string
		wantErr string
	}{
		{name: "slug and space", args: map[string]interface{}{"unit": "api", "space": "prod"}, want: "unit get --json api --space prod"},
		{name: "qualified reference", args: map[string]interface{}{"unit": "prod/api"}, want: "unit get --json prod/api"},
		{name: "unit ID", args: map[string]interface{}{"unit": "11111111-2222-4333-8444-555555555555"}, want: "unit get --json 11111111-2222-4333-8444-555555555555"},
		{name: "bare slug with no space", args: map[string]interface{}{"unit": "api"}, wantErr: "needs the unit's ConfigHub space"},
		{name: "every space cannot name one unit", args: map[string]interface{}{"unit": "api", "space": "*"}, wantErr: "must name one space"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := build(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || strings.Join(got, " ") != tt.want {
				t.Fatalf("args = %v err = %v, want %q", got, err, tt.want)
			}
		})
	}
	for _, tool := range []string{"confighub_units", "confighub_changesets"} {
		required, _ := gateway.tools[tool].Descriptor.InputSchema["required"].([]string)
		if strings.Join(required, ",") != "space" {
			t.Errorf("%s required = %v, want [space]", tool, required)
		}
	}
}

func TestCubGetCommandHint(t *testing.T) {
	for _, tt := range []struct {
		space, slug, id string
		want            string
	}{
		{space: "prod", slug: "api", id: "u-1", want: "cub unit get api --json --space prod"},
		{space: "prod", id: "u-1", want: "cub unit get u-1 --json --space prod"},
		{slug: "api", id: "u-1", want: "cub unit get u-1 --json"},
		{slug: "api", want: "cub unit get api --json --space <space>"},
		{want: ""},
	} {
		if got := cubGetCommandHint("unit", tt.space, tt.slug, tt.id); got != tt.want {
			t.Errorf("cubGetCommandHint(%q, %q, %q) = %q, want %q", tt.space, tt.slug, tt.id, got, tt.want)
		}
	}
}

func TestHistoryAndAuditNameTheirSpace(t *testing.T) {
	stubSpaceInputs(t, "")
	if _, err := historyChangeSetListArgs(context.Background(), historyQuery{Resource: "deployment/api"}); err == nil {
		t.Fatal("history built cub arguments with no space; it must refuse")
	}
	args, err := historyChangeSetListArgs(context.Background(), historyQuery{Resource: "deployment/api", Namespace: "prod", Space: "payments-prod"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(args, " "); got != "changeset list --json --contains prod deployment/api --space payments-prod" {
		t.Fatalf("history args = %q", got)
	}

	oldRun := runAuditCubCommand
	t.Cleanup(func() { runAuditCubCommand = oldRun })
	var seen []string
	runAuditCubCommand = func(ctx context.Context, args []string) (string, error) {
		seen = args
		return "[]", nil
	}
	if _, err := fetchAuditEntries(context.Background(), auditListQuery{}); err == nil {
		t.Fatal("audit list read ConfigHub with no space; it must refuse")
	}
	if seen != nil {
		t.Fatalf("audit list ran cub %v before refusing", seen)
	}
	if _, err := fetchAuditEntries(context.Background(), auditListQuery{Space: "*"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(seen, " "); !strings.HasSuffix(got, "--space *") {
		t.Fatalf("audit args = %q, want an explicit --space", got)
	}
}

// history reports the space it read, so an empty result reads as "none in this
// space" and not as "never imported".
func TestHistoryReportsTheSpaceItRead(t *testing.T) {
	stubSpaceInputs(t, "payments-prod")
	t.Setenv("CUB_SCOUT_TEST_HISTORY_JSON", "")
	restore := stubHistoryForScopeTest(t)
	defer restore()

	historyFormat = "json"
	historyCmd.SetContext(context.Background())
	out := captureStdout(t, func() {
		if err := runHistory(historyCmd, []string{"deployment/api"}); err != nil {
			t.Fatal(err)
		}
	})
	var result historyResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if result.Scope == nil || result.Scope.Space != "payments-prod" || result.Scope.SpaceSource != spaceSourceEnv {
		t.Fatalf("scope = %+v, want payments-prod from CUB_SPACE", result.Scope)
	}

	ascii := renderHistoryASCII(result)
	for _, want := range []string{"ConfigHub space: payments-prod (CUB_SPACE)", "No ConfigHub change history found for this resource in space payments-prod"} {
		if !strings.Contains(ascii, want) {
			t.Fatalf("ascii output missing %q:\n%s", want, ascii)
		}
	}
	if strings.Contains(ascii, "not yet imported") {
		t.Fatalf("ascii output claims the resource was never imported, which one empty space does not show:\n%s", ascii)
	}
	if md := renderHistoryMarkdown(result); !strings.Contains(md, "- ConfigHub space: `payments-prod` (CUB_SPACE)") {
		t.Fatalf("markdown output missing the space:\n%s", md)
	}
}

func stubHistoryForScopeTest(t *testing.T) func() {
	t.Helper()
	prevRequire, prevFetch, prevNav := requireHistoryConnectedFn, fetchHistoryEntriesFn, resolveHistoryNavigationFn
	prevFormat, prevSpace, prevSince, prevNamespace := historyFormat, historySpace, historySince, historyNamespace
	requireHistoryConnectedFn = func() error { return nil }
	fetchHistoryEntriesFn = func(ctx context.Context, q historyQuery) ([]historyEntry, error) { return nil, nil }
	resolveHistoryNavigationFn = func(ctx context.Context, q historyQuery) historyNavigation { return historyNavigation{} }
	historySpace, historySince, historyNamespace = "", "7d", ""
	return func() {
		requireHistoryConnectedFn, fetchHistoryEntriesFn, resolveHistoryNavigationFn = prevRequire, prevFetch, prevNav
		historyFormat, historySpace, historySince, historyNamespace = prevFormat, prevSpace, prevSince, prevNamespace
	}
}

// The compare unit reads carry the space they were given, always.
func TestCompareUnitArgsAlwaysNameTheSpace(t *testing.T) {
	for _, args := range [][]string{
		compareUnitGetArgs("checkout", "payments-prod"),
		compareUnitDataArgs("checkout", "payments-prod"),
		compareUnitLivedataArgs("checkout", "payments-prod"),
	} {
		if got := strings.Join(args, " "); !strings.HasSuffix(got, "--space payments-prod") {
			t.Fatalf("args = %q, want an explicit --space", got)
		}
	}
}

// ensureSpace must only read, then create, and never touch the cub context.
func TestEnsureSpace_NeverChangesTheCubContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	for _, tt := range []struct {
		name   string
		exists string
		want   []string
	}{
		{name: "space exists", exists: "yes", want: []string{"space get payments-prod"}},
		{name: "space is missing", exists: "no", want: []string{"space get payments-prod", "space create payments-prod"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			log := filepath.Join(dir, "calls.log")
			script := "#!/bin/sh\necho \"$*\" >> \"" + log + "\"\n" +
				"if [ \"$1 $2\" = \"space get\" ] && [ \"$FAKE_SPACE_EXISTS\" != \"yes\" ]; then exit 1; fi\nexit 0\n"
			if err := os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+":/usr/bin:/bin")
			t.Setenv("FAKE_SPACE_EXISTS", tt.exists)

			if err := ensureSpace("payments-prod"); err != nil {
				t.Fatal(err)
			}
			calls := fakeCubCalls(t, log)
			if strings.Join(calls, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("cub calls = %v, want %v", calls, tt.want)
			}
			for _, call := range calls {
				if strings.HasPrefix(call, "context ") || strings.Contains(call, "--set-context") {
					t.Fatalf("cub call %q changes the cub context", call)
				}
			}
		})
	}
}

func TestGitOpsDeliverySpace_ReportsItsSource(t *testing.T) {
	tests := []struct {
		name                 string
		flag, env            string
		wantSlug, wantSource string
	}{
		{name: "flag", flag: "from-flag", env: "from-env", wantSlug: "from-flag", wantSource: spaceSourceFlag},
		{name: "CUB_SPACE", env: "from-env", wantSlug: "from-env", wantSource: spaceSourceEnv},
		{name: "nothing: left empty for the collector to report as an omission"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubSpaceInputs(t, tt.env)
			slug, source := gitOpsDeliverySpace(tt.flag)
			if slug != tt.wantSlug || source != tt.wantSource {
				t.Fatalf("gitOpsDeliverySpace = %q from %q, want %q from %q", slug, source, tt.wantSlug, tt.wantSource)
			}
		})
	}
}

// fleet outliers reads exactly one space, named on the cub command line, and
// refuses every-space.
func TestFleetOutliers_ReadsExactlyOneNamedSpace(t *testing.T) {
	oldLoader, oldSpace, oldFormat := loadFleetOutlierUnitsFn, fleetOutliersSpace, fleetOutliersFormat
	t.Cleanup(func() {
		loadFleetOutlierUnitsFn, fleetOutliersSpace, fleetOutliersFormat = oldLoader, oldSpace, oldFormat
	})
	fleetOutliersFormat = "json"

	var asked []string
	loadFleetOutlierUnitsFn = func(space string) ([]fleetUnitSnapshot, error) {
		asked = append(asked, space)
		// What one ConfigHub space can hold: unit slugs are unique, and each
		// unit has one target.
		return []fleetUnitSnapshot{
			{UnitSlug: "api", Cluster: "cluster-a", Revision: 3, Space: space},
			{UnitSlug: "worker", Cluster: "cluster-b", Revision: 7, Space: space},
		}, nil
	}

	stubSpaceInputs(t, "from-env")
	fleetOutliersSpace = ""
	out := captureStdout(t, func() {
		if err := runFleetOutliers(nil, nil); err != nil {
			t.Fatal(err)
		}
	})
	var report fleetOutlierReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if report.Scope.Space != "from-env" || report.Scope.SpaceSource != spaceSourceEnv {
		t.Fatalf("scope = %+v, want from-env from CUB_SPACE", report.Scope)
	}
	// Neither unit is on two clusters, so nothing was compared. Reporting each
	// cluster as missing the other's unit, or as consistent, would both be false.
	if report.Summary.ComparedUnitCount != 0 || report.Summary.OutlierClusterCount != 0 || len(report.Notes) != 1 {
		t.Fatalf("report = %+v, want nothing compared and a note saying so", report)
	}

	fleetOutliersSpace = "prod"
	captureStdout(t, func() {
		if err := runFleetOutliers(nil, nil); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Join(asked, ",") != "from-env,prod" {
		t.Fatalf("spaces passed to cub = %v, want [from-env prod]", asked)
	}

	asked = nil
	fleetOutliersSpace = "*"
	if err := runFleetOutliers(nil, nil); err == nil || !strings.Contains(err.Error(), "reads one ConfigHub space") {
		t.Fatalf("err = %v, want every-space refused", err)
	}
	stubSpaceInputs(t, "")
	fleetOutliersSpace = ""
	if err := runFleetOutliers(nil, nil); err == nil || !strings.Contains(err.Error(), "--space <slug>") {
		t.Fatalf("err = %v, want a refusal when no space is given anywhere", err)
	}
	if asked != nil {
		t.Fatalf("cub was asked for %v although the command refused", asked)
	}
}

// A failure reading the space is reported as what it is. It used to be
// reported as "Run: cub auth login" whatever cub said.
func TestFleetOutliers_ReportsWhyTheReadFailed(t *testing.T) {
	oldLoader, oldSpace := loadFleetOutlierUnitsFn, fleetOutliersSpace
	t.Cleanup(func() { loadFleetOutlierUnitsFn, fleetOutliersSpace = oldLoader, oldSpace })
	stubSpaceInputs(t, "")
	fleetOutliersSpace = "no-such-space"
	loadFleetOutlierUnitsFn = func(space string) ([]fleetUnitSnapshot, error) {
		return nil, errors.New(`cub unit list --json --quiet --space no-such-space failed: space "no-such-space" not found`)
	}
	err := runFleetOutliers(nil, nil)
	if err == nil || !strings.Contains(err.Error(), `space "no-such-space" not found`) || strings.Contains(err.Error(), "cub auth login") {
		t.Fatalf("err = %v, want cub's reason and no login advice", err)
	}
}

func TestBuildFleetOutlierReport_UnitsOnOneClusterAreNotCompared(t *testing.T) {
	report, err := buildFleetOutlierReport([]fleetUnitSnapshot{
		{UnitSlug: "api", Cluster: "cluster-a", Revision: 4},
		{UnitSlug: "api", Cluster: "cluster-b", Revision: 3},
		{UnitSlug: "only-on-a", Cluster: "cluster-a", Revision: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.ComparedUnitCount != 1 {
		t.Fatalf("compared units = %d, want 1 (api)", report.Summary.ComparedUnitCount)
	}
	for _, finding := range report.ByCluster["cluster-b"].Outliers {
		if finding.Unit == "only-on-a" {
			t.Fatalf("cluster-b reported missing %q, a unit only one cluster has", finding.Unit)
		}
	}
	if len(report.Notes) != 0 {
		t.Fatalf("notes = %v, want none when a unit was compared", report.Notes)
	}
	if ascii := renderFleetOutliersASCII(report); strings.Contains(ascii, "Not compared") {
		t.Fatalf("ascii output says nothing was compared:\n%s", ascii)
	}

	nothing, err := buildFleetOutlierReport([]fleetUnitSnapshot{
		{UnitSlug: "api", Cluster: "cluster-a", Revision: 4},
		{UnitSlug: "worker", Cluster: "cluster-b", Revision: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	ascii := renderFleetOutliersASCII(nothing)
	if !strings.Contains(ascii, "Not compared") || strings.Contains(ascii, "consistent") || strings.Contains(ascii, "Missing") {
		t.Fatalf("ascii output must say nothing was compared, without calling a cluster consistent or missing a unit:\n%s", ascii)
	}
	if md := renderFleetOutliersMarkdown(nothing); !strings.Contains(md, "Compared units: `0`") || strings.Contains(md, "consistent") {
		t.Fatalf("markdown output:\n%s", md)
	}
}

func TestMapFleetSpace(t *testing.T) {
	stubSpaceInputs(t, "")
	if got := mapFleetSpace(""); got.Slug != allConfigHubSpaces || got.Source != spaceSourceCommandDefault {
		t.Fatalf("mapFleetSpace = %+v, want every space as the command default", got)
	}
	if got := mapFleetSpace("payments"); got.Slug != "payments" || got.Source != spaceSourceFlag {
		t.Fatalf("mapFleetSpace = %+v, want the flag", got)
	}
	stubSpaceInputs(t, "from-env")
	if got := mapFleetSpace(""); got.Slug != "from-env" || got.Source != spaceSourceEnv {
		t.Fatalf("mapFleetSpace = %+v, want CUB_SPACE", got)
	}
}

func TestStatusWorkerSpace(t *testing.T) {
	if got := statusWorkerSpace(configHubSpace{}); got != allConfigHubSpaces {
		t.Fatalf("statusWorkerSpace(unset) = %q, want every space", got)
	}
	if got := statusWorkerSpace(configHubSpace{Slug: "prod", Source: spaceSourceEnv}); got != "prod" {
		t.Fatalf("statusWorkerSpace = %q, want prod", got)
	}
}

// Recorded contract: run the connected commands against a fake cub whose
// config still carries a defaultSpace, as an older cub left it. With nothing
// named, the commands that need a space refuse before calling cub; with a flag
// or CUB_SPACE, every space-scoped call carries exactly that space; and no call
// ever uses the stale default, reads the context to find a space, or changes it.
func TestConnectedCommandsNameTheirSpaceAgainstARecordingCub(t *testing.T) {
	logPath := fakeCubRecorder(t)
	t.Setenv("CUB_SCOUT_TEST_HISTORY_JSON", "")
	t.Setenv("CUB_SCOUT_TEST_AUDIT_JSON", "")
	t.Setenv("CUB_SCOUT_TEST_MAP_FLEET_JSON", "")

	prevHistoryRequire, prevAuditRequire := requireHistoryConnectedFn, requireAuditConnectedFn
	prevHistoryNav := resolveHistoryNavigationFn
	prevHistorySpace, prevAuditSpace, prevTreeSpace, prevFleetSpace, prevImpactSpace, prevMapFleetSpace :=
		historySpace, auditListSpace, treeSpace, fleetOutliersSpace, impactSpace, fleetSpace
	prevHistoryFormat, prevAuditFormat, prevFleetFormat, prevImpactFormat := historyFormat, auditListFormat, fleetOutliersFormat, impactFormat
	t.Cleanup(func() {
		requireHistoryConnectedFn, requireAuditConnectedFn = prevHistoryRequire, prevAuditRequire
		resolveHistoryNavigationFn = prevHistoryNav
		historySpace, auditListSpace, treeSpace, fleetOutliersSpace, impactSpace, fleetSpace =
			prevHistorySpace, prevAuditSpace, prevTreeSpace, prevFleetSpace, prevImpactSpace, prevMapFleetSpace
		historyFormat, auditListFormat, fleetOutliersFormat, impactFormat = prevHistoryFormat, prevAuditFormat, prevFleetFormat, prevImpactFormat
	})
	requireHistoryConnectedFn = func() error { return nil }
	requireAuditConnectedFn = func() error { return nil }
	resolveHistoryNavigationFn = func(ctx context.Context, q historyQuery) historyNavigation { return historyNavigation{} }
	historyFormat, auditListFormat, fleetOutliersFormat, impactFormat = "json", "json", "json", "json"
	historyCmd.SetContext(context.Background())
	auditListCmd.SetContext(context.Background())

	type command struct {
		name        string
		run         func() error
		setFlag     func(string)
		needsSpace  bool
		allowsEvery bool
	}
	commands := []command{
		{name: "history", setFlag: func(v string) { historySpace = v }, needsSpace: true,
			run: func() error { return runHistory(historyCmd, []string{"deployment/api"}) }},
		{name: "audit list", setFlag: func(v string) { auditListSpace = v }, needsSpace: true,
			run: func() error { return runAuditList(auditListCmd, nil) }},
		{name: "tree config", setFlag: func(v string) { treeSpace = v }, needsSpace: true,
			run: runTreeConfig},
		{name: "fleet outliers", setFlag: func(v string) { fleetOutliersSpace = v }, needsSpace: true,
			run: func() error { return runFleetOutliers(nil, nil) }},
		{name: "impact", setFlag: func(v string) { impactSpace = v }, needsSpace: true,
			run: func() error { return runImpact(impactCmd, []string{"api"}) }},
		{name: "map fleet", setFlag: func(v string) { fleetSpace = v }, allowsEvery: true,
			run: func() error { return runMapFleet(mapFleetCmd, nil) }},
	}

	spaceScoped := func(call string) bool {
		fields := strings.Fields(call)
		return len(fields) >= 2 && spaceScopedCubEntities[fields[0]]
	}
	for _, cmd := range commands {
		for _, mode := range []struct {
			name, flag, env, want string
		}{
			{name: "nothing named"},
			{name: "flag", flag: "named-by-flag", want: "--space named-by-flag"},
			{name: "CUB_SPACE", env: "named-by-env", want: "--space named-by-env"},
		} {
			t.Run(cmd.name+"/"+mode.name, func(t *testing.T) {
				if err := os.Remove(logPath); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
				stubSpaceInputs(t, mode.env)
				cmd.setFlag(mode.flag)
				var runErr error
				captureStdout(t, func() { runErr = cmd.run() })

				calls := fakeCubCalls(t, logPath)
				for _, call := range calls {
					switch {
					case strings.Contains(call, "stale-default"):
						t.Fatalf("cub %q uses the defaultSpace cub no longer honours", call)
					case strings.HasPrefix(call, "context "):
						t.Fatalf("cub %q reads or changes the cub context", call)
					case strings.Contains(call, "--set-context"):
						t.Fatalf("cub %q changes the cub context", call)
					}
				}

				switch {
				case mode.want != "":
					if runErr != nil && !strings.Contains(runErr.Error(), "not found") && !strings.Contains(runErr.Error(), "2+ clusters") {
						t.Fatalf("run: %v", runErr)
					}
					scoped := 0
					for _, call := range calls {
						if !spaceScoped(call) {
							continue
						}
						scoped++
						if !strings.Contains(call, mode.want) {
							t.Fatalf("cub %q does not carry %s", call, mode.want)
						}
					}
					if scoped == 0 {
						t.Fatalf("no space-scoped cub call was made; calls = %v", calls)
					}
				case cmd.needsSpace:
					if runErr == nil || !strings.Contains(runErr.Error(), "cub has no default space") {
						t.Fatalf("err = %v, want a refusal that names no context", runErr)
					}
					if len(calls) != 0 {
						t.Fatalf("cub was called %v before the refusal", calls)
					}
				case cmd.allowsEvery:
					if runErr != nil {
						t.Fatalf("run: %v", runErr)
					}
					for _, call := range calls {
						if spaceScoped(call) && !strings.Contains(call, "--space *") {
							t.Fatalf("cub %q: the documented every-space default must be explicit", call)
						}
					}
				}
			})
		}
	}
}

// cub prints a worker as {"BridgeWorker": {...}, "Space": {...}}. The parser
// looked only for a "Worker" key, so every worker came back nameless and
// status could never find one.
func TestParseCubWorkerListJSON_ReadsBridgeWorker(t *testing.T) {
	raw := []byte(`[{"BridgeWorker":{"Slug":"prod-east","Condition":"Ready","SpaceID":"s-1"},"Space":{"Slug":"platform"}}]`)
	got, err := parseCubWorkerListJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "prod-east" || got[0].Condition != "Ready" {
		t.Fatalf("workers = %+v, want prod-east Ready", got)
	}
}

func TestConfigHubRefNamesSpace(t *testing.T) {
	for ref, want := range map[string]bool{
		"payments/api":                         true,
		"11111111-2222-4333-8444-555555555555": true,
		"api":                                  false,
		"/api":                                 false,
		"payments/":                            false,
		"*/api":                                false,
		"a/b/c":                                false,
		" payments/api ":                       true,
	} {
		if got := configHubRefNamesSpace(strings.TrimSpace(ref)); got != want {
			t.Errorf("configHubRefNamesSpace(%q) = %v, want %v", ref, got, want)
		}
	}
	gateway := newMCPGatewayWithMode(nil, nil, true)
	build := gateway.tools["confighub_unit_get"].BuildArgs
	if _, err := build(map[string]interface{}{"unit": "*/api"}); err == nil {
		t.Fatal("confighub_unit_get accepted */api, an every-space lookup")
	}
	const unitID = "11111111-2222-4333-8444-555555555555"
	if got, err := build(map[string]interface{}{"unit": unitID, "space": "*"}); err != nil || strings.Join(got, " ") != "unit get --json "+unitID {
		t.Fatalf("args = %v err = %v: an ID needs no space, so '*' alongside it is harmless", got, err)
	}
}

func TestMCPToolEnvDropsCubSpace(t *testing.T) {
	got := mcpToolEnv([]string{"PATH=/bin", "CUB_SPACE=server-side", "CUB_CONTEXT=ctx", "CUB_SPACEX=kept"})
	if strings.Join(got, "|") != "PATH=/bin|CUB_CONTEXT=ctx|CUB_SPACEX=kept" {
		t.Fatalf("env = %v, want CUB_SPACE removed and nothing else", got)
	}
}

func TestConfigHubSpaceOfEntry(t *testing.T) {
	for _, tt := range []struct {
		name  string
		entry MapEntry
		want  string
	}{
		{name: "space name recorded by ownership", entry: MapEntry{OwnerDetails: map[string]string{"space": "payments", "spaceID": "s-1"}}, want: "payments"},
		{name: "space ID, as current releases stamp it", entry: MapEntry{OwnerDetails: map[string]string{"spaceID": "s-1"}}, want: "s-1"},
		{name: "label only", entry: MapEntry{Labels: map[string]string{"confighub.com/SpaceName": "payments"}}, want: "payments"},
		{name: "nothing recorded", entry: MapEntry{Labels: map[string]string{"app": "api"}}, want: ""},
	} {
		if got := configHubSpaceOfEntry(tt.entry); got != tt.want {
			t.Errorf("%s: configHubSpaceOfEntry = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// compare reads DRY/WET from the live object's own space: its name, else the
// space ID current ConfigHub releases stamp. Only with neither does CUB_SPACE
// decide, and the result says so.
func TestCompareDryWetSpaceComesFromTheResourceFirst(t *testing.T) {
	restoreLive, restoreConnected, restoreDryWet := loadCompareLiveSnapshotFn, compareConnectedFn, loadCompareDryWetSnapshotFn
	t.Cleanup(func() {
		loadCompareLiveSnapshotFn, compareConnectedFn, loadCompareDryWetSnapshotFn = restoreLive, restoreConnected, restoreDryWet
	})
	compareConnectedFn = func() bool { return true }

	for _, tt := range []struct {
		name, spaceName, spaceID, env string
		wantSpace, wantNote           string
	}{
		{name: "space name", spaceName: "payments", spaceID: "s-1", env: "elsewhere", wantSpace: "payments"},
		{name: "space ID only", spaceID: "s-1", env: "elsewhere", wantSpace: "s-1"},
		{name: "CUB_SPACE when the object records none", env: "from-env", wantSpace: "from-env", wantNote: "named by CUB_SPACE"},
		{name: "nothing: skipped", wantNote: "DRY/WET lookup skipped"},
		{name: "every space cannot pick a unit", env: "*", wantNote: "DRY/WET lookup skipped"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stubSpaceInputs(t, tt.env)
			loadCompareLiveSnapshotFn = func(ctx context.Context, kind, name, namespace string) (compareSideSummary, error) {
				return compareSideSummary{Source: "cluster", Kind: kind, Name: name, Namespace: namespace, UnitSlug: "api", SpaceName: tt.spaceName, SpaceID: tt.spaceID}, nil
			}
			asked := ""
			loadCompareDryWetSnapshotFn = func(ctx context.Context, unitSlug, space string, target compareResourceRef) (compareDryWetResult, error) {
				asked = space
				return compareDryWetResult{}, nil
			}
			result, err := buildCompareResourceResult(context.Background(), "deployment/api", "prod")
			if err != nil {
				t.Fatal(err)
			}
			if asked != tt.wantSpace {
				t.Fatalf("DRY/WET read space %q, want %q", asked, tt.wantSpace)
			}
			notes := strings.Join(result.Notes, "\n")
			if tt.wantNote != "" && !strings.Contains(notes, tt.wantNote) {
				t.Fatalf("notes = %q, want %q", notes, tt.wantNote)
			}
			if tt.wantNote == "" && strings.Contains(notes, "named by") {
				t.Fatalf("notes = %q: the space came from the resource", notes)
			}
		})
	}
}

// trace joins evidence to one object, so its space comes from that object or
// the flag, never from CUB_SPACE, and says which.
func TestTraceDeliverySpaceSource(t *testing.T) {
	stubSpaceInputs(t, "from-env")
	flags := traceConfigHubDeliveryFlags{Enabled: true, Namespace: "prod", Since: "24h"}

	opts, _ := traceGitOpsDeliveryOptions(flags, agent.TraceDeliveryCorrelation{Space: "payments"})
	if opts.Space != "payments" || opts.SpaceSource != spaceSourceResource {
		t.Fatalf("opts = %q from %q, want payments from resource", opts.Space, opts.SpaceSource)
	}
	flags.Space = "chosen"
	opts, _ = traceGitOpsDeliveryOptions(flags, agent.TraceDeliveryCorrelation{Space: "payments"})
	if opts.Space != "chosen" || opts.SpaceSource != spaceSourceFlag {
		t.Fatalf("opts = %q from %q, want chosen from flag", opts.Space, opts.SpaceSource)
	}
	flags.Space = ""
	opts, omissions := traceGitOpsDeliveryOptions(flags, agent.TraceDeliveryCorrelation{})
	if opts.Space != "" || len(omissions) != 1 {
		t.Fatalf("opts.Space = %q omissions = %v: CUB_SPACE must not scope a per-object join", opts.Space, omissions)
	}
}

func TestAuditListReportsTheSpaceItRead(t *testing.T) {
	stubSpaceInputs(t, "platform")
	t.Setenv("CUB_SCOUT_TEST_AUDIT_JSON", "")
	prevRequire, prevFetch, prevFormat, prevSpace := requireAuditConnectedFn, fetchAuditEntriesFn, auditListFormat, auditListSpace
	t.Cleanup(func() {
		requireAuditConnectedFn, fetchAuditEntriesFn, auditListFormat, auditListSpace = prevRequire, prevFetch, prevFormat, prevSpace
	})
	requireAuditConnectedFn = func() error { return nil }
	fetchAuditEntriesFn = func(ctx context.Context, q auditListQuery) ([]auditEntry, error) { return nil, nil }
	auditListFormat, auditListSpace = "json", ""
	auditListCmd.SetContext(context.Background())

	out := captureStdout(t, func() {
		if err := runAuditList(auditListCmd, nil); err != nil {
			t.Fatal(err)
		}
	})
	var result auditListResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if result.Scope == nil || result.Scope.Space != "platform" || result.Scope.SpaceSource != spaceSourceEnv {
		t.Fatalf("scope = %+v, want platform from CUB_SPACE", result.Scope)
	}
	if ascii := renderAuditASCII(result); !strings.Contains(ascii, "ConfigHub space: platform (CUB_SPACE)") {
		t.Fatalf("ascii output does not name the space:\n%s", ascii)
	}
}

// A link read in one space can point at a unit with the same slug in another
// space, as a variant's link to its base does. Joined by slug, the unit became
// its own dependent. impact also refuses '*' and says what the link read covers.
func TestImpactJoinsLinksWithinOneSpace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$*\" >> \"" + log + "\"\n" +
		"case \"$1 $2\" in\n" +
		"  \"unit list\") echo '[{\"Unit\":{\"Slug\":\"upstream\",\"SpaceID\":\"s-dev\"},\"Space\":{\"Slug\":\"team-dev\",\"SpaceID\":\"s-dev\"}},{\"Unit\":{\"Slug\":\"web\",\"SpaceID\":\"s-dev\"},\"Space\":{\"Slug\":\"team-dev\",\"SpaceID\":\"s-dev\"}}]' ;;\n" +
		"  \"link list\") echo '[{\"FromUnit\":{\"Slug\":\"upstream\",\"SpaceID\":\"s-dev\"},\"ToUnit\":{\"Slug\":\"upstream\",\"SpaceID\":\"s-base\"}},{\"FromUnit\":{\"Slug\":\"web\",\"SpaceID\":\"s-dev\"},\"ToUnit\":{\"Slug\":\"upstream\",\"SpaceID\":\"s-dev\"}}]' ;;\n" +
		"  *) echo '[]' ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stubSpaceInputs(t, "*")
	if _, err := fetchConfigHubUnits("impact", "--space", ""); err == nil || !strings.Contains(err.Error(), "reads one ConfigHub space") {
		t.Fatalf("err = %v, want every-space refused", err)
	}
	if calls := fakeCubCalls(t, log); len(calls) != 0 {
		t.Fatalf("cub was called %v before the refusal", calls)
	}

	stubSpaceInputs(t, "")
	cache, err := fetchConfigHubUnits("impact", "--space", "team-dev")
	if err != nil {
		t.Fatal(err)
	}
	result, err := buildImpactResult(cache, "upstream")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependents) != 1 || result.Dependents[0].Unit != "web" {
		t.Fatalf("dependents = %+v, want only web; upstream must not depend on itself", result.Dependents)
	}
	if result.Scope == nil || result.Scope.Space != "team-dev" || result.Scope.SpaceSource != spaceSourceFlag {
		t.Fatalf("scope = %+v, want team-dev from flag", result.Scope)
	}
	notes := strings.Join(result.Notes, "\n")
	for _, want := range []string{"links stored in space team-dev only", "1 link(s) in space team-dev connect to units in other spaces"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("notes = %q, want %q", notes, want)
		}
	}
	if ascii := renderImpactASCII(result); !strings.Contains(ascii, "ConfigHub space: team-dev (flag)") {
		t.Fatalf("ascii output does not name the space:\n%s", ascii)
	}
}

// status against a cub whose old config still names a default space: the JSON
// must not report that space, and the worker search must say where it looked.
func TestStatusIgnoresAStaleDefaultSpace(t *testing.T) {
	logPath := fakeCubRecorder(t)
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	t.Setenv("CUB_PLUGIN", "")
	t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing-kubeconfig"))
	if err := statusCmd.Flags().Set("json", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = statusCmd.Flags().Set("json", "false") })

	for _, tt := range []struct {
		env, wantSpace, wantWorkerScope string
	}{
		{env: "", wantSpace: "", wantWorkerScope: "--space *"},
		{env: "platform", wantSpace: "platform", wantWorkerScope: "--space platform"},
	} {
		if err := os.Remove(logPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		stubSpaceInputs(t, tt.env)
		out := captureStdout(t, func() {
			if err := runStatus(statusCmd); err != nil {
				t.Fatal(err)
			}
		})
		var status StatusInfo
		if err := json.Unmarshal([]byte(out), &status); err != nil {
			t.Fatalf("parse %q: %v", out, err)
		}
		if status.Space != tt.wantSpace || strings.Contains(out, "stale-default") {
			t.Fatalf("CUB_SPACE=%q: status = %s, want space %q and no stale default", tt.env, out, tt.wantSpace)
		}
		found := false
		for _, call := range fakeCubCalls(t, logPath) {
			if strings.HasPrefix(call, "worker list") {
				found = true
				if !strings.Contains(call, tt.wantWorkerScope) {
					t.Fatalf("CUB_SPACE=%q: cub %q, want %s", tt.env, call, tt.wantWorkerScope)
				}
			}
		}
		if !found {
			t.Fatalf("CUB_SPACE=%q: status did not look for a worker", tt.env)
		}
	}
}
