// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func settingsObject(apiVersion, kind, namespace, name string, spec map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]interface{}{"namespace": namespace, "name": name},
	}}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	return obj
}

var settingsListKinds = map[schema.GroupVersionResource]string{
	{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}:                  "ApplicationList",
	{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}:      "KustomizationList",
	{Group: "kustomize.toolkit.fluxcd.io", Version: "v1beta2", Resource: "kustomizations"}: "KustomizationList",
	{Group: "kustomize.toolkit.fluxcd.io", Version: "v1beta1", Resource: "kustomizations"}: "KustomizationList",
	{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}:             "HelmReleaseList",
	{Group: "helm.toolkit.fluxcd.io", Version: "v2beta2", Resource: "helmreleases"}:        "HelmReleaseList",
	{Group: "helm.toolkit.fluxcd.io", Version: "v2beta1", Resource: "helmreleases"}:        "HelmReleaseList",
	{Version: "v1", Resource: "configmaps"}:                                                "ConfigMapList",
}

// settingsFixtureClient is a small mixed cluster: three Applications in two
// projects, one outside the Argo CD namespace, two Kustomizations and one
// HelmRelease.
func settingsFixtureClient() *dynamicfake.FakeDynamicClient {
	automated := func(fields map[string]interface{}, options ...interface{}) map[string]interface{} {
		return map[string]interface{}{"automated": fields, "syncOptions": options}
	}
	configMap := settingsObject("v1", "ConfigMap", "argocd", "argocd-cm", nil)
	configMap.Object["data"] = map[string]interface{}{"url": "https://argocd.example.test"}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), settingsListKinds,
		settingsObject("argoproj.io/v1alpha1", "Application", "argocd", "ledger-api", map[string]interface{}{"project": "payments",
			"syncPolicy": automated(map[string]interface{}{"prune": true, "selfHeal": true}, "ServerSideApply=true")}),
		settingsObject("argoproj.io/v1alpha1", "Application", "argocd", "fx-rates", map[string]interface{}{"project": "payments",
			"syncPolicy":        automated(map[string]interface{}{}, "ServerSideApply=true", "Validate=false"),
			"ignoreDifferences": []interface{}{map[string]interface{}{"kind": "Deployment", "jsonPointers": []interface{}{"/spec/replicas"}}}}),
		settingsObject("argoproj.io/v1alpha1", "Application", "argocd", "legacy", map[string]interface{}{"project": "platform"}),
		settingsObject("argoproj.io/v1alpha1", "Application", "team-b", "outside", map[string]interface{}{"project": "platform",
			"syncPolicy": automated(map[string]interface{}{"selfHeal": false})}),
		settingsObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", map[string]interface{}{"prune": true}),
		settingsObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "held", map[string]interface{}{"prune": false, "suspend": true}),
		settingsObject("helm.toolkit.fluxcd.io/v2", "HelmRelease", "team-h", "chart", map[string]interface{}{
			"driftDetection": map[string]interface{}{"mode": "enabled"}, "upgrade": map[string]interface{}{"force": true}}),
		configMap,
	)
}

func settingsReport(t *testing.T, client *dynamicfake.FakeDynamicClient, params deliverySettingsParams) deliverySettingsReport {
	t.Helper()
	report, err := buildDeliverySettingsReport(context.Background(), client, params)
	require.NoError(t, err)
	report.Context = "prod-east"
	return report
}

func shownDeployers(report deliverySettingsReport) []string {
	out := []string{}
	for _, deployer := range report.Deployers {
		out = append(out, deployer.Kind+"/"+deployer.Namespace+"/"+deployer.Name)
	}
	return out
}

// #839: the view an auditor asked for. Grouped by project, each setting lists
// the deployers that have each value; an absent field is marked unset.
func TestGitOpsSettingsASCIIGroupsByProjectThenSetting(t *testing.T) {
	out := renderDeliverySettingsASCII(settingsReport(t, settingsFixtureClient(), deliverySettingsParams{}), deliverySettingsGroupByProject)
	require.Equal(t, strings.Join([]string{
		"DELIVERY SETTINGS",
		strings.Repeat("═", 68),
		"Context: prod-east   Namespace: all",
		"Read: 4 Applications, 2 Kustomizations, 1 HelmRelease",
		"",
		"Argo CD Application, project payments (2)",
		"  auto-sync",
		"    on    2  argocd/fx-rates, argocd/ledger-api",
		"  self-heal",
		"    on    1  argocd/ledger-api",
		"    off   1 (1 unset, controller default)  argocd/fx-rates (unset)",
		"  prune",
		"    on    1  argocd/ledger-api",
		"    off   1 (1 unset, controller default)  argocd/fx-rates (unset)",
		"  options",
		"    ServerSideApply=true  2  argocd/fx-rates, argocd/ledger-api",
		"    Validate=false  1  argocd/fx-rates",
		"    ignoreDifferences  1  argocd/fx-rates (1 rule, comparison only)",
		"",
		"Argo CD Application, project platform (2)",
		"  auto-sync",
		"    on    1  team-b/outside",
		"    off   1 (1 unset, controller default)  argocd/legacy (unset)",
		"  self-heal",
		"    off   1  team-b/outside",
		"    n/a   1  argocd/legacy (auto-sync is off)",
		"  prune",
		"    off   1 (1 unset, controller default)  team-b/outside (unset)",
		"    n/a   1  argocd/legacy (auto-sync is off)",
		"",
		"Flux HelmRelease, namespace team-h (1)",
		"  suspend",
		"    off   1 (1 unset, controller default)  team-h/chart (unset)",
		"  driftDetection.mode",
		"    enabled 1  team-h/chart",
		"  options",
		"    upgrade.force=true  1  team-h/chart",
		"",
		"Flux Kustomization, namespace flux-system (2)",
		"  suspend",
		"    on    1  flux-system/held",
		"    off   1 (1 unset, controller default)  flux-system/apps (unset)",
		"  prune",
		"    on    1  flux-system/apps",
		"    off   1  flux-system/held",
		"  force",
		"    off   2 (2 unset, controller default)  flux-system/apps (unset), flux-system/held (unset)",
		"  wait",
		"    off   2 (2 unset, controller default)  flux-system/apps (unset), flux-system/held (unset)",
		"",
		"Argo CD UI links, by Application namespace",
		"  argocd: https://argocd.example.test/applications/argocd/<name>",
		"  team-b: no argocd-cm in this namespace; no links",
		"",
		"Reads",
		"  applications.argoproj.io/v1alpha1: read, 4",
		"  kustomizations.kustomize.toolkit.fluxcd.io/v1: read, 2",
		"  helmreleases.helm.toolkit.fluxcd.io/v2: read, 1",
		"",
	}, "\n"), out)
}

func TestGitOpsSettingsOtherGroupings(t *testing.T) {
	report := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{})

	bySetting := renderDeliverySettingsASCII(report, deliverySettingsGroupBySetting)
	require.Contains(t, bySetting, "\nArgo CD Application (4)\n  auto-sync\n    on    3  argocd/fx-rates, argocd/ledger-api, team-b/outside\n")
	require.NotContains(t, bySetting, "project payments")

	byDeployer := renderDeliverySettingsASCII(report, deliverySettingsGroupByDeployer)
	require.Contains(t, byDeployer, strings.Join([]string{
		"Argo CD Application argocd/fx-rates, project payments",
		"  https://argocd.example.test/applications/argocd/fx-rates",
		"  auto-sync on; self-heal unset (controller default off); prune unset (controller default off)",
		"  options: ServerSideApply=true, Validate=false, ignoreDifferences (1 rule, comparison only)",
	}, "\n"))
	require.Contains(t, byDeployer, "Argo CD Application team-b/outside, project platform\n  auto-sync on;",
		"an Application with no known Argo CD URL has no link line")
	require.Contains(t, byDeployer, "Flux Kustomization flux-system/held, namespace flux-system\n  suspend on; prune off;")
}

func TestGitOpsSettingsMarkdownLinksDeployersWithAKnownURL(t *testing.T) {
	report := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{})
	markdown := renderDeliverySettingsMarkdown(report, deliverySettingsGroupByProject)
	require.Contains(t, markdown, "## Argo CD Application, project payments (2)\n\n| Setting | Count | Deployers |\n|---|---|---|\n")
	require.Contains(t, markdown, "| self-heal off | 1 (1 unset, controller default) | [argocd/fx-rates](https://argocd.example.test/applications/argocd/fx-rates) (unset) |")
	require.Contains(t, markdown, "| Validate=false | 1 | [argocd/fx-rates](https://argocd.example.test/applications/argocd/fx-rates) |")
	require.Contains(t, markdown, "| self-heal off | 1 | team-b/outside |", "no link is written where no URL is known")
	require.Contains(t, markdown, "- applications.argoproj.io/v1alpha1: read, 4\n")

	table := renderDeliverySettingsMarkdown(report, deliverySettingsGroupByDeployer)
	require.Contains(t, table, "| Argo CD Application [argocd/ledger-api](https://argocd.example.test/applications/argocd/ledger-api) | project payments | auto-sync on; self-heal on; prune on | ServerSideApply=true |")
}

func TestGitOpsSettingsJSONCarriesBothViewsAndEveryRead(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, writeDeliverySettings(&out, settingsReport(t, settingsFixtureClient(), deliverySettingsParams{}), "json", deliverySettingsGroupByDeployer))
	var decoded struct {
		Context   string `json:"context"`
		Complete  bool   `json:"complete"`
		Counts    []deliveryKindCount
		Deployers []struct {
			Kind, Name, URL, GroupKind, Group string
			Settings                          []struct{ Name, Category, Value, Default, Effective, Path string }
			IgnoreRules                       []interface{}
		}
		Groups   []struct{ Controller, Kind, GroupKind, Group string }
		Settings []struct{ Kind, GroupKind string }
		Reads    []struct {
			Resource, Status string
			Count            int
		}
		LinkSources []struct{ Namespace, Status, URL string }
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &decoded))
	require.Equal(t, "prod-east", decoded.Context)
	require.True(t, decoded.Complete)
	require.Equal(t, []deliveryKindCount{
		{Controller: "ArgoCD", Kind: "Application", Status: "read", Read: 4, Shown: 4},
		{Controller: "Flux", Kind: "Kustomization", Status: "read", Read: 2, Shown: 2},
		{Controller: "Flux", Kind: "HelmRelease", Status: "read", Read: 1, Shown: 1},
	}, decoded.Counts)
	require.Len(t, decoded.Deployers, 7)
	require.Len(t, decoded.Groups, 4, "the per-project view is present whatever --group-by rendered")
	require.Len(t, decoded.Settings, 3)
	require.Len(t, decoded.Reads, 3)
	require.Len(t, decoded.LinkSources, 2)
	for _, deployer := range decoded.Deployers {
		if deployer.Name != "fx-rates" {
			continue
		}
		require.Len(t, deployer.IgnoreRules, 1)
		require.Equal(t, "self-heal", deployer.Settings[1].Name)
		require.Equal(t, [3]string{"unset", "off", "off"}, [3]string{deployer.Settings[1].Value, deployer.Settings[1].Default, deployer.Settings[1].Effective})
	}
	// Absent optional fields are omitted, not written as empty strings.
	require.NotContains(t, out.String(), `"default": ""`)
	require.NotContains(t, out.String(), `"url": ""`)
}

func TestGitOpsSettingsFilters(t *testing.T) {
	selfHealOff := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{Settings: []string{"self-heal=off"}})
	require.Equal(t, []string{"Application/argocd/fx-rates", "Application/team-b/outside"}, shownDeployers(selfHealOff),
		"declared off and off-by-default both match; an Application that syncs manually does not")
	require.Equal(t, deliveryKindCount{Controller: "ArgoCD", Kind: "Application", Status: "read", Read: 4, Shown: 2}, selfHealOff.Counts[0])
	ascii := renderDeliverySettingsASCII(selfHealOff, deliverySettingsGroupByProject)
	require.Contains(t, ascii, "Read: 2 of 4 Applications, 0 of 2 Kustomizations, 0 of 1 HelmRelease\nFilter: setting self-heal=off\n")

	both := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{Settings: []string{"ServerSideApply=true", "Validate"}})
	require.Equal(t, []string{"Application/argocd/fx-rates"}, shownDeployers(both), "every --setting must match")

	// prune=off spans controllers: the Argo CD policy and the Flux field.
	pruneOff := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{Settings: []string{"prune=false"}})
	require.Equal(t, []string{"Application/argocd/fx-rates", "Application/team-b/outside", "Kustomization/flux-system/held"}, shownDeployers(pruneOff))

	byProject := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{Projects: []string{"payments"}})
	require.Equal(t, []string{"Application/argocd/fx-rates", "Application/argocd/ledger-api"}, shownDeployers(byProject))
	require.Equal(t, []string{"--project selects Argo CD Applications only; 3 Flux object(s) have no project and are left out."}, byProject.Notes)
	require.Contains(t, renderDeliverySettingsASCII(byProject, deliverySettingsGroupByProject), "\nNote: --project selects Argo CD Applications only;")

	// A namespace that happens to share a project's name is not that project.
	sameName := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{Projects: []string{"flux-system"}})
	require.Empty(t, sameName.Deployers, "--project must not select Flux objects by their namespace")

	scoped := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{Namespace: "team-h"})
	require.Equal(t, []string{"HelmRelease/team-h/chart"}, shownDeployers(scoped))
	require.Contains(t, renderDeliverySettingsASCII(scoped, deliverySettingsGroupByProject), "Context: prod-east   Namespace: team-h\nRead: 0 Applications, 0 Kustomizations, 1 HelmRelease\n")

	none := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{Settings: []string{"NoSuchOption"}})
	require.Empty(t, none.Deployers)
	require.Contains(t, renderDeliverySettingsASCII(none, deliverySettingsGroupByProject), "\nNo deployers to show.\n")
	var out bytes.Buffer
	require.NoError(t, writeDeliverySettings(&out, none, "json", deliverySettingsGroupByProject))
	require.Contains(t, out.String(), `"deployers": []`, "an empty result is an empty list, not null")
}

// A kind the caller may not list must not look like a cluster without it.
func TestGitOpsSettingsReportsAKindItCouldNotRead(t *testing.T) {
	client := settingsFixtureClient()
	client.PrependReactor("list", "applications", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "", nil)
	})
	report := settingsReport(t, client, deliverySettingsParams{})
	require.False(t, report.Complete)
	require.Equal(t, []string{"HelmRelease/team-h/chart", "Kustomization/flux-system/apps", "Kustomization/flux-system/held"}, shownDeployers(report))

	for format, out := range map[string]string{
		"ascii": renderDeliverySettingsASCII(report, deliverySettingsGroupByProject),
		"md":    renderDeliverySettingsMarkdown(report, deliverySettingsGroupByProject),
	} {
		require.Contains(t, out, "applications.argoproj.io/v1alpha1: NOT READ (forbidden); Applications are not known to be absent", format)
		require.Contains(t, strings.ToLower(out), "incomplete", format)
		// The header must not count a kind it could not read: "0 Applications"
		// says there are none.
		require.Contains(t, out, "Read: Applications NOT READ, 2 Kustomizations, 1 HelmRelease\n", format)
		require.NotContains(t, out, "0 Application", format)
		require.NotContains(t, out, "Argo CD Application", format)
	}
	var out bytes.Buffer
	require.NoError(t, writeDeliverySettings(&out, report, "json", deliverySettingsGroupByProject))
	require.Contains(t, out.String(), `"complete": false`)
	require.Contains(t, out.String(), `"status": "not_read"`)
	require.Contains(t, out.String(), `"reason": "forbidden"`)
	require.Equal(t, deliveryKindCount{Controller: "ArgoCD", Kind: "Application", Status: "not_read"}, report.Counts[0])
	filtered := settingsReport(t, client, deliverySettingsParams{Settings: []string{"prune=on"}})
	require.Contains(t, renderDeliverySettingsASCII(filtered, deliverySettingsGroupByProject), "Read: Applications NOT READ, 1 of 2 Kustomizations, 0 of 1 HelmRelease\n")

	// Nothing installed at all is complete: every list was answered.
	empty := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), settingsListKinds)
	empty.PrependReactor("list", "*", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), "")
	})
	absent := settingsReport(t, empty, deliverySettingsParams{})
	require.True(t, absent.Complete)
	text := renderDeliverySettingsASCII(absent, deliverySettingsGroupByProject)
	require.Contains(t, text, "applications.argoproj.io/v1alpha1: not installed")
	require.Contains(t, text, "No deployers to show.")
	require.Contains(t, text, "Read: Applications not installed, Kustomizations not installed, HelmReleases not installed\n")
	require.NotContains(t, text, "INCOMPLETE")
}

func TestGitOpsSettingsFlagValidation(t *testing.T) {
	run := func(args ...string) error {
		cmd := &cobra.Command{Use: "settings"}
		addGitOpsSettingsFlags(cmd.Flags())
		require.NoError(t, cmd.ParseFlags(args))
		if _, err := deliverySettingsParamsFromFlags(cmd); err != nil {
			return err
		}
		format, _ := cmd.Flags().GetString("format")
		if _, err := normalizeGitOpsStatusFormat(format, false); err != nil {
			return err
		}
		tui, _ := cmd.Flags().GetBool("tui")
		return validateGitOpsTUIFormat(tui, cmd.Flags().Changed("format"), cmd.Flags().Changed("json"))
	}
	require.NoError(t, run("--group-by", "Setting", "--setting", "Validate=false", "--setting", "ignoreDifferences"))
	require.ErrorContains(t, run("--group-by", "team"), `invalid --group-by "team" (valid: project, setting, deployer)`)
	require.ErrorContains(t, run("--setting", "=false"), `invalid --setting "=false" (want name or name=value)`)
	require.ErrorContains(t, run("--project", " "), "--project must not be empty")
	require.ErrorContains(t, run("--format", "yaml"), `invalid --format "yaml"`)
	require.ErrorContains(t, run("--tui", "--format", "json"), "--tui cannot be combined with output format options")

	require.NotNil(t, gitopsSettingsCmd.Flags().Lookup("kube-context"), "gitops settings takes an explicit context")
	require.Contains(t, contextBoundReadCommands(), gitopsSettingsCmd)
}

func TestGitOpsSettingsTUIShowsTheMarkdownSnapshot(t *testing.T) {
	report := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{})
	model := newGitOpsMarkdownTUIModel(renderDeliverySettingsMarkdown(report, deliverySettingsGroupByProject) + "\x1b[31m")
	require.Contains(t, model.content, "# Delivery settings")
	require.Contains(t, model.content, "| self-heal on | 1 |")
	require.NotContains(t, model.content, "\x1b", "control characters from cluster data never reach the terminal")
}

// Project names, sync options and ApplicationSet names are free text written
// by whoever can create an Application. Printed raw, a newline forges output
// lines and an escape sequence rewrites the terminal.
func TestGitOpsSettingsTextOutputNeutralisesClusterSuppliedControlCharacters(t *testing.T) {
	hostile := settingsObject("argoproj.io/v1alpha1", "Application", "argocd", "app", map[string]interface{}{
		"project": "pay\nReads\n  applications.argoproj.io/v1alpha1: read, 0",
		"syncPolicy": map[string]interface{}{"syncOptions": []interface{}{
			"Validate=false\n\nReads\n  everything: read, 0", "\x1b[2J\x1b[31mServerSideApply=true|x"}},
	})
	configMap := settingsObject("v1", "ConfigMap", "argocd", "argocd-cm", nil)
	configMap.Object["data"] = map[string]interface{}{"url": "https://argocd.example.test/a)b|c"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), settingsListKinds, hostile, configMap)
	report := settingsReport(t, client, deliverySettingsParams{})

	for _, groupBy := range []string{deliverySettingsGroupByProject, deliverySettingsGroupBySetting, deliverySettingsGroupByDeployer} {
		for format, out := range map[string]string{
			"ascii": renderDeliverySettingsASCII(report, groupBy),
			"md":    renderDeliverySettingsMarkdown(report, groupBy),
		} {
			require.NotContains(t, out, "\x1b", "%s %s", format, groupBy)
			require.Equal(t, 1, strings.Count(out, "\nReads\n")+strings.Count(out, "\n## Reads\n"), "%s %s: a forged Reads section\n%s", format, groupBy, out)
			for _, forged := range []string{"applications.argoproj.io/v1alpha1: read, 0", "everything: read, 0"} {
				for _, line := range strings.Split(out, "\n") {
					require.False(t, strings.HasPrefix(strings.TrimLeft(line, " -"), forged), "%s %s: forged line %q", format, groupBy, line)
				}
			}
			require.Contains(t, out, "applications.argoproj.io/v1alpha1: read, 1", "%s %s", format, groupBy)
		}
	}
	markdown := renderDeliverySettingsMarkdown(report, deliverySettingsGroupByProject)
	require.Contains(t, markdown, "[argocd/app](https://argocd.example.test/a%29b%7Cc/applications/argocd/app)", "a URL cannot close its own link or add a column")
	require.Contains(t, markdown, "ServerSideApply=true\\|x", "a pipe cannot add a table column")

	// JSON keeps what the cluster said, encoded.
	var out bytes.Buffer
	require.NoError(t, writeDeliverySettings(&out, report, "json", deliverySettingsGroupByProject))
	require.Contains(t, out.String(), `"group": "pay\nReads\n  applications.argoproj.io/v1alpha1: read, 0"`)
	require.Contains(t, out.String(), `\u001b[2J`)
}

// --view keeps the scope, counts and reads, which say whether the view is
// complete, and drops the two views that were not asked for.
func TestGitOpsSettingsJSONViewKeepsCoverageAndDropsOtherViews(t *testing.T) {
	client := settingsFixtureClient()
	client.PrependReactor("list", "helmreleases", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "helmreleases"}, "", nil)
	})
	report := settingsReport(t, client, deliverySettingsParams{})
	var full bytes.Buffer
	require.NoError(t, writeDeliverySettings(&full, report, "json", deliverySettingsGroupByProject))

	for view, dropped := range map[string][]string{
		deliverySettingsViewGroups:    {"settings", "deployers"},
		deliverySettingsViewSettings:  {"groups", "deployers"},
		deliverySettingsViewDeployers: {"groups", "settings"},
	} {
		var out bytes.Buffer
		require.NoError(t, writeDeliverySettingsView(&out, report, view))
		var document map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(out.Bytes(), &document))
		require.JSONEq(t, `"`+view+`"`, string(document["view"]))
		require.Contains(t, document, view)
		for _, key := range dropped {
			require.NotContains(t, document, key, "view %s", view)
		}
		for _, kept := range []string{"context", "complete", "counts", "reads", "linkSources"} {
			require.Contains(t, document, kept, "view %s must keep %s", view, kept)
		}
		require.JSONEq(t, "false", string(document["complete"]), "an incomplete read stays visible in every view")
		require.Less(t, out.Len(), full.Len(), "view %s is smaller than the full report", view)

		var fullDocument map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(full.Bytes(), &fullDocument))
		require.JSONEq(t, string(fullDocument[view]), string(document[view]), "view %s is the same data as in the full report", view)
	}
}

func TestGitOpsSettingsViewFlag(t *testing.T) {
	parse := func(args ...string) (deliverySettingsParams, error) {
		cmd := &cobra.Command{Use: "settings"}
		addGitOpsSettingsFlags(cmd.Flags())
		require.NoError(t, cmd.ParseFlags(args))
		return deliverySettingsParamsFromFlags(cmd)
	}
	params, err := parse()
	require.NoError(t, err)
	require.Equal(t, deliverySettingsViewAll, params.View, "the CLI default is the full report, as in v2.13.3")
	params, err = parse("--view", "Groups", "--format", "json")
	require.NoError(t, err)
	require.Equal(t, deliverySettingsViewGroups, params.View)
	_, err = parse("--view", "summary", "--json")
	require.NoError(t, err)
	// A view is a JSON shape. Asking for one with text output is refused, not
	// silently ignored.
	for _, args := range [][]string{{"--view", "summary"}, {"--view", "deployers", "--format", "md"}} {
		_, err = parse(args...)
		require.ErrorContains(t, err, "--view applies to JSON output")
	}
	_, err = parse("--view", "brief", "--json")
	require.ErrorContains(t, err, `invalid --view "brief" (valid: all, summary, groups, settings, deployers)`)
}

// The MCP tool is the CLI's JSON, with the compact summary by default so one
// call returns one small inversion and not three verbose ones.
func TestMCPGitOpsSettingsBuildsTheCLICall(t *testing.T) {
	gateway := newMCPGateway(nil)
	tool, ok := gateway.tools["gitops_settings"]
	require.True(t, ok)
	require.Nil(t, tool.Runner, "a Kubernetes-only tool stays on Scout's runner")
	properties := tool.Descriptor.InputSchema["properties"].(map[string]interface{})
	for _, name := range []string{"namespace", "context", "project", "setting", "view"} {
		require.Contains(t, properties, name)
	}
	require.Equal(t, false, tool.Descriptor.InputSchema["additionalProperties"])

	for _, tc := range []struct {
		name      string
		arguments map[string]interface{}
		want      []string
	}{
		{"no arguments", map[string]interface{}{}, []string{"gitops", "settings", "--format", "json", "--view", "summary"}},
		{"filters", map[string]interface{}{"setting": []interface{}{"self-heal=off", "Validate"}, "project": []interface{}{"payments"}, "namespace": "argocd"},
			[]string{"gitops", "settings", "--format", "json", "-n", "argocd", "--project", "payments", "--setting", "self-heal=off", "--setting", "Validate", "--view", "summary"}},
		{"context and view", map[string]interface{}{"context": "prod-east", "view": "deployers"},
			[]string{"gitops", "settings", "--format", "json", "--kube-context", "prod-east", "--view", "deployers"}},
		{"an empty context is passed on so the command refuses it", map[string]interface{}{"context": ""},
			[]string{"gitops", "settings", "--format", "json", "--kube-context", "", "--view", "summary"}},
	} {
		got, err := tool.BuildArgs(tc.arguments)
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.want, got, tc.name)
	}
	for name, arguments := range map[string]map[string]interface{}{
		"context": {"context": 17},
		"setting": {"setting": []interface{}{"self-heal=off", 3}},
		"project": {"project": map[string]interface{}{"a": "b"}},
		"view":    {"view": true},
	} {
		_, err := tool.BuildArgs(arguments)
		require.Error(t, err, "a wrong-typed %s must be refused, not dropped", name)
	}
}

func TestMCPGitOpsSettingsReturnsTheReportAsStructuredData(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, writeDeliverySettingsView(&out, settingsReport(t, settingsFixtureClient(), deliverySettingsParams{}), deliverySettingsViewSummary))
	var captured []string
	gateway := newMCPGateway(func(_ context.Context, args []string) (string, error) {
		captured = args
		return out.String(), nil
	})
	params, _ := json.Marshal(map[string]interface{}{"name": "gitops_settings", "arguments": map[string]interface{}{"setting": []string{"prune=off"}}})
	response := gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call", Params: params})
	require.Nil(t, response.Error)
	encoded, err := json.Marshal(response.Result)
	require.NoError(t, err)
	var result struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Data map[string]interface{} `json:"data"`
		} `json:"structuredContent"`
	}
	require.NoError(t, json.Unmarshal(encoded, &result))
	require.False(t, result.IsError)
	require.Equal(t, []string{"gitops", "settings", "--format", "json", "--setting", "prune=off", "--view", "summary"}, captured)
	require.Equal(t, "summary", result.StructuredContent.Data["view"])
	require.Equal(t, true, result.StructuredContent.Data["complete"])
	require.Contains(t, result.StructuredContent.Data, "reads")

	// An invalid argument is a tool error, and nothing is run.
	captured = nil
	params, _ = json.Marshal(map[string]interface{}{"name": "gitops_settings", "arguments": map[string]interface{}{"view": 3}})
	response = gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`2`), Method: "tools/call", Params: params})
	encoded, _ = json.Marshal(response.Result)
	require.NoError(t, json.Unmarshal(encoded, &result))
	require.True(t, result.IsError)
	require.Nil(t, captured)
}

// The summary says what the groups view says, with each deployer written once
// per setting. It must lose nothing the groups view uses to avoid a false
// claim: unset members stay marked, n/a stays n/a, and coverage stays.
func TestGitOpsSettingsSummaryViewIsTheGroupsViewCompacted(t *testing.T) {
	client := settingsFixtureClient()
	client.PrependReactor("list", "helmreleases", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "helmreleases"}, "", nil)
	})
	report := settingsReport(t, client, deliverySettingsParams{})
	var out, groups bytes.Buffer
	require.NoError(t, writeDeliverySettingsView(&out, report, deliverySettingsViewSummary))
	require.NoError(t, writeDeliverySettingsView(&groups, report, deliverySettingsViewGroups))
	require.Equal(t, 1, strings.Count(out.String(), "\n"), "the summary is one line")
	require.Less(t, out.Len()*3, groups.Len(), "the summary is well under a third of the groups view")

	var document struct {
		View     string
		Complete bool
		Reads    []struct{ Kind, Status, Reason string }
		Summary  []deliverySummaryGroup
		Groups   json.RawMessage
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &document))
	require.Equal(t, "summary", document.View)
	require.False(t, document.Complete)
	require.Equal(t, "not_read", document.Reads[2].Status)
	require.Nil(t, document.Groups, "the verbose groups view is not also sent")

	require.Equal(t, []deliverySummaryGroup{
		{Controller: "ArgoCD", Kind: "Application", GroupKind: "project", Group: "payments", Deployers: 2,
			Settings: map[string]map[string][]string{
				"auto-sync":         {"on": {"argocd/fx-rates", "argocd/ledger-api"}},
				"self-heal":         {"on": {"argocd/ledger-api"}, "off": {"argocd/fx-rates"}},
				"prune":             {"on": {"argocd/ledger-api"}, "off": {"argocd/fx-rates"}},
				"ServerSideApply":   {"true": {"argocd/fx-rates", "argocd/ledger-api"}},
				"Validate":          {"false": {"argocd/fx-rates"}},
				"ignoreDifferences": {"set": {"argocd/fx-rates"}},
			},
			Unset:   map[string][]string{"self-heal": {"argocd/fx-rates"}, "prune": {"argocd/fx-rates"}},
			Details: map[string]map[string]string{"ignoreDifferences": {"argocd/fx-rates": "1 rule, comparison only"}}},
		{Controller: "ArgoCD", Kind: "Application", GroupKind: "project", Group: "platform", Deployers: 2,
			Settings: map[string]map[string][]string{
				"auto-sync": {"on": {"team-b/outside"}, "off": {"argocd/legacy"}},
				"self-heal": {"off": {"team-b/outside"}, "n/a": {"argocd/legacy"}},
				"prune":     {"off": {"team-b/outside"}, "n/a": {"argocd/legacy"}},
			},
			Unset: map[string][]string{"auto-sync": {"argocd/legacy"}, "prune": {"team-b/outside"}}},
		{Controller: "Flux", Kind: "Kustomization", GroupKind: "namespace", Group: "flux-system", Deployers: 2,
			Settings: map[string]map[string][]string{
				"suspend": {"on": {"flux-system/held"}, "off": {"flux-system/apps"}},
				"prune":   {"on": {"flux-system/apps"}, "off": {"flux-system/held"}},
				"force":   {"off": {"flux-system/apps", "flux-system/held"}},
				"wait":    {"off": {"flux-system/apps", "flux-system/held"}},
			},
			Unset: map[string][]string{"suspend": {"flux-system/apps"}, "force": {"flux-system/apps", "flux-system/held"}, "wait": {"flux-system/apps", "flux-system/held"}}},
	}, document.Summary)
}
