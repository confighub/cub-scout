// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubSpaceInputs fixes the two ambient inputs the resolver may consult.
func stubSpaceInputs(t *testing.T, cubSpaceEnv, contextDefault string) {
	t.Helper()
	t.Setenv("CUB_SPACE", cubSpaceEnv)
	old := cubContextDefaultSpaceFn
	t.Cleanup(func() { cubContextDefaultSpaceFn = old })
	cubContextDefaultSpaceFn = func() string { return contextDefault }
}

func TestResolveConfigHubSpace_Precedence(t *testing.T) {
	tests := []struct {
		name                     string
		explicit, explicitSource string
		env, contextDefault      string
		wantSlug, wantSource     string
	}{
		{name: "flag wins over everything", explicit: "from-flag", env: "from-env", contextDefault: "from-context", wantSlug: "from-flag", wantSource: spaceSourceFlag},
		{name: "a resource's own space is reported as such", explicit: "from-label", explicitSource: spaceSourceResource, env: "from-env", wantSlug: "from-label", wantSource: spaceSourceResource},
		{name: "CUB_SPACE wins over the context default", env: "from-env", contextDefault: "from-context", wantSlug: "from-env", wantSource: spaceSourceEnv},
		{name: "context default is the last resort", contextDefault: "from-context", wantSlug: "from-context", wantSource: spaceSourceContextDefault},
		{name: "every space is honoured when it is asked for", explicit: "*", wantSlug: "*", wantSource: spaceSourceFlag},
		{name: "whitespace is not a space", explicit: "  ", env: " ", contextDefault: "\t"},
		// The case that must never widen: nothing given, and a cub context with
		// no default space. The answer is "unset", never "*".
		{name: "nothing given resolves to unset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubSpaceInputs(t, tt.env, tt.contextDefault)
			got := resolveConfigHubSpace(tt.explicit, tt.explicitSource)
			if got.Slug != tt.wantSlug || got.Source != tt.wantSource {
				t.Fatalf("resolveConfigHubSpace = %+v, want %s from %s", got, tt.wantSlug, tt.wantSource)
			}
			if got.IsSet() != (tt.wantSlug != "") {
				t.Fatalf("IsSet() = %v for %+v", got.IsSet(), got)
			}
		})
	}
}

func TestRequireConfigHubSpace_RefusesRatherThanWiden(t *testing.T) {
	stubSpaceInputs(t, "", "")
	space, err := requireConfigHubSpace("history", "--space", "")
	if err == nil {
		t.Fatalf("space = %+v, want a refusal when no space is given anywhere", space)
	}
	for _, want := range []string{"history", "--space <slug>", "--space '*'", "CUB_SPACE=<slug>"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want it to contain %q", err, want)
		}
	}

	stubSpaceInputs(t, "", "payments-prod")
	if space, err := requireConfigHubSpace("history", "--space", ""); err != nil || space.Slug != "payments-prod" {
		t.Fatalf("space = %+v err = %v, want the context default when it is set", space, err)
	}
}

func TestWithConfigHubSpace(t *testing.T) {
	got := withConfigHubSpace([]string{"unit", "list", "--json"}, " payments-prod ")
	if strings.Join(got, " ") != "unit list --json --space payments-prod" {
		t.Fatalf("args = %v", got)
	}
}

// A target given without a space must carry its own, or it would be resolved
// against the cub context's default space.
func TestWithConfigHubSpaceOrTargets(t *testing.T) {
	const uuid = "0c0e7b0b-2011-4b19-b077-f4613e0a0d15"
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
		{name: "target UUID needs no space", targets: []string{uuid}, want: "k8s get deployments --target " + uuid},
		{name: "bare target without a space is refused", targets: []string{"cluster-a"}, wantErr: `target "cluster-a" names no space`},
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

// An MCP tool whose `space` argument is optional must not silently mean
// "whatever the cub context defaults to": an agent calling it cannot see that.
func TestMCPConfigHubSpace(t *testing.T) {
	stubSpaceInputs(t, "", "")
	if _, err := mcpConfigHubSpace("confighub_units", map[string]interface{}{}); err == nil || !strings.Contains(err.Error(), "`space` argument") {
		t.Fatalf("err = %v, want a refusal naming the space argument", err)
	}
	if space, err := mcpConfigHubSpace("confighub_units", map[string]interface{}{"space": "prod"}); err != nil || space != "prod" {
		t.Fatalf("space = %q err = %v, want prod", space, err)
	}
	stubSpaceInputs(t, "from-env", "")
	if space, err := mcpConfigHubSpace("confighub_units", map[string]interface{}{}); err != nil || space != "from-env" {
		t.Fatalf("space = %q err = %v, want CUB_SPACE", space, err)
	}
}

func TestHistoryAndAuditNameTheirSpace(t *testing.T) {
	stubSpaceInputs(t, "", "")
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

// ensureSpace used to probe for a space with `cub context set --space`, which
// switched the user's cub context as a side effect. It must only read, then
// create, and never touch the context.
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
			raw, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
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
		name                      string
		flag, env, contextDefault string
		wantSlug, wantSource      string
	}{
		{name: "flag", flag: "from-flag", env: "from-env", wantSlug: "from-flag", wantSource: spaceSourceFlag},
		{name: "CUB_SPACE", env: "from-env", contextDefault: "from-context", wantSlug: "from-env", wantSource: spaceSourceEnv},
		{name: "context default is reported as ambient", contextDefault: "from-context", wantSlug: "from-context", wantSource: spaceSourceContextDefault},
		{name: "nothing: left empty for the collector to report as an omission"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubSpaceInputs(t, tt.env, tt.contextDefault)
			slug, source := gitOpsDeliverySpace(context.Background(), tt.flag)
			if slug != tt.wantSlug || source != tt.wantSource {
				t.Fatalf("gitOpsDeliverySpace = %q from %q, want %q from %q", slug, source, tt.wantSlug, tt.wantSource)
			}
		})
	}
}

// fleet outliers matches units and clusters by slug, and a slug is unique only
// within one space. It therefore reads exactly one space, named on the cub
// command line, and refuses every-space.
func TestFleetOutliers_ReadsExactlyOneNamedSpace(t *testing.T) {
	oldLoader, oldSpace, oldFormat := loadFleetOutlierUnitsFn, fleetOutliersSpace, fleetOutliersFormat
	t.Cleanup(func() {
		loadFleetOutlierUnitsFn, fleetOutliersSpace, fleetOutliersFormat = oldLoader, oldSpace, oldFormat
	})
	fleetOutliersFormat = "md"

	var asked []string
	loadFleetOutlierUnitsFn = func(space string) ([]fleetUnitSnapshot, error) {
		asked = append(asked, space)
		return []fleetUnitSnapshot{
			{UnitSlug: "api", Cluster: "cluster-a", Revision: 3, Space: space},
			{UnitSlug: "api", Cluster: "cluster-b", Revision: 3, Space: space},
		}, nil
	}

	stubSpaceInputs(t, "", "context-default")
	fleetOutliersSpace = ""
	if err := runFleetOutliers(nil, nil); err != nil {
		t.Fatal(err)
	}
	fleetOutliersSpace = "prod"
	if err := runFleetOutliers(nil, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Join(asked, ",") != "context-default,prod" {
		t.Fatalf("spaces passed to cub = %v, want [context-default prod], each as an explicit --space", asked)
	}

	asked = nil
	fleetOutliersSpace = "*"
	if err := runFleetOutliers(nil, nil); err == nil || !strings.Contains(err.Error(), "one ConfigHub space at a time") {
		t.Fatalf("err = %v, want every-space refused", err)
	}
	stubSpaceInputs(t, "", "")
	fleetOutliersSpace = ""
	if err := runFleetOutliers(nil, nil); err == nil || !strings.Contains(err.Error(), "--space <slug>") {
		t.Fatalf("err = %v, want a refusal when no space is given anywhere", err)
	}
	if asked != nil {
		t.Fatalf("cub was asked for %v although the command refused", asked)
	}
}

// Why every-space is refused: two unrelated units that share a name, in two
// spaces, collapse into one key and come out as an outlier that does not exist.
func TestFleetOutliers_CrossSpaceInputWouldInventAnOutlier(t *testing.T) {
	report, err := buildFleetOutlierReport([]fleetUnitSnapshot{
		{UnitSlug: "api", Cluster: "cluster-a", Revision: 3, Space: "payments"},
		{UnitSlug: "api", Cluster: "cluster-b", Revision: 3, Space: "payments"},
		// A different unit that happens to be called "api", in another space.
		{UnitSlug: "api", Cluster: "cluster-b", Revision: 9, Space: "search"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.OutlierClusterCount == 0 {
		t.Skip("the slug collision no longer invents an outlier; every-space could be reconsidered")
	}
	// payments/api is at revision 3 on both clusters: there is no real outlier.
	// The report says otherwise only because search/api overwrote it.
}

// `map fleet` is documented as showing apps across spaces. With no --space the
// cub call used to carry no scope at all.
func TestFetchFleetUnits_NamesEverySpaceWhenNoneIsGiven(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake cub")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\necho \"$*\" >> \"" + log + "\"\necho '[]'\n"
	if err := os.WriteFile(filepath.Join(dir, "cub"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("CUB_SCOUT_TEST_MAP_FLEET_JSON", "")

	for _, space := range []string{"", "payments-team"} {
		if _, err := fetchFleetUnits(space, ""); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(calls) != 2 || !strings.HasSuffix(calls[0], "--space *") || !strings.HasSuffix(calls[1], "--space payments-team") {
		t.Fatalf("cub calls = %v, want an explicit --space on both", calls)
	}
}
