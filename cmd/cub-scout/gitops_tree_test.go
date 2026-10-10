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

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// The tests in this file are about the command: its argument, flags, filters
// and the three renderings. Their cluster is written by hand in the shape a
// real Argo CD records. What a real controller reports is in the recorded
// tests (gitops_tree_recorded_test.go).

func treeEntry(group, kind, namespace, name, sync string) map[string]interface{} {
	entry := map[string]interface{}{"version": "v1", "kind": kind, "name": name, "status": sync}
	if group != "" {
		entry["group"] = group
	}
	if namespace != "" {
		entry["namespace"] = namespace
	}
	return entry
}

func treeAppEntry(name string) map[string]interface{} {
	return treeEntry("argoproj.io", "Application", "argocd", name, "Synced")
}

// treeApplication is an Application whose status reports resources. A nil
// resources leaves it with no status at all.
func treeApplication(name, sync, health string, spec map[string]interface{}, resources []interface{}) *unstructured.Unstructured {
	app := settingsObject("argoproj.io/v1alpha1", "Application", "argocd", name, spec)
	if resources != nil {
		app.Object["status"] = map[string]interface{}{
			"sync": map[string]interface{}{"status": sync}, "health": map[string]interface{}{"status": health}, "resources": resources,
		}
	}
	return app
}

// treeFixtureClient is a small app-of-apps: a platform root with a team under
// it, a child that is gone, a plain ConfigMap, and a Flux Kustomization
// beside it.
func treeFixtureClient(extra ...runtime.Object) *dynamicfake.FakeDynamicClient {
	objects := []runtime.Object{
		treeApplication("platform", "Synced", "Healthy", map[string]interface{}{"project": "default",
			"syncPolicy":        map[string]interface{}{"automated": map[string]interface{}{"prune": true, "selfHeal": true}},
			"ignoreDifferences": []interface{}{map[string]interface{}{"group": "argoproj.io", "kind": "Application", "jsonPointers": []interface{}{"/spec/syncPolicy"}}}},
			[]interface{}{treeAppEntry("team-a"), treeAppEntry("gone"), treeEntry("", "ConfigMap", "platform", "settings", "Synced")}),
		treeApplication("team-a", "OutOfSync", "Degraded", map[string]interface{}{"project": "default",
			"syncPolicy": map[string]interface{}{"automated": map[string]interface{}{}}},
			[]interface{}{
				treeEntry("apps", "Deployment", "web", "web", "OutOfSync"),
				treeEntry("", "Service", "web", "web", "Synced"),
				treeEntry("", "ConfigMap", "web", "web-settings", "Synced"),
			}),
		settingsObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", map[string]interface{}{"prune": true}),
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), settingsListKinds, append(objects, extra...)...)
}

func treeReport(t *testing.T, client *dynamicfake.FakeDynamicClient, params deliveryTreeParams) deliveryTreeReport {
	t.Helper()
	if params.View == "" {
		params.View = deliveryTreeViewAll
	}
	report, err := buildDeliveryTreeReport(context.Background(), client, params)
	require.NoError(t, err)
	report.Context = "prod-east"
	return report
}

func TestGitOpsTreeASCII(t *testing.T) {
	report := treeReport(t, treeFixtureClient(), deliveryTreeParams{})
	out := renderDeliveryTreeASCII(report, 25)

	require.Equal(t, strings.TrimLeft(`
DELIVERY TREE  (context prod-east)
2 roots, 4 deployers, 4 resources reported
Each deployer's resources are what it reports about itself. They are not checked against the cluster.
  ! 1 reported but not found in the cluster
  ! 1 with no status: not reconciled
  ! 1 of a kind whose children are not read yet

Application argocd/platform  ·  Synced / Healthy  ·  auto-sync on, self-heal on, prune on
├─ Application argocd/gone
│     ! not found in the cluster
│     ! parent ignores differences at /spec/syncPolicy
├─ Application argocd/team-a  ·  OutOfSync / Degraded  ·  auto-sync on, self-heal off, prune off
│     ! parent ignores differences at /spec/syncPolicy
│  ├─ ConfigMap web/web-settings  ·  Synced
│  ├─ Deployment web/web  ·  OutOfSync
│  └─ Service web/web  ·  Synced
└─ ConfigMap platform/settings  ·  Synced

Kustomization flux-system/apps  ·  suspend off, prune on, force off, wait off
   ! not reconciled: the object has no status
   ! what a Kustomization delivers is not read yet (#856)

Note: No resource health is reported: Argo CD keeps it in its own tree, not on the Application. A resource with no health shown is not known to be healthy.
Note: What a Flux Kustomization or HelmRelease delivers is not read yet (#856); those deployers are shown without their children.
Reads:
  Argo CD Application: read (2)
  Flux Kustomization: read (1)
  Flux HelmRelease: read (0)
`, "\n"), out)
}

func TestGitOpsTreeCountsTheResourcesItDoesNotList(t *testing.T) {
	report := treeReport(t, treeFixtureClient(), deliveryTreeParams{Root: &agent.DeliveryRef{Kind: "Application", Name: "team-a"}})
	require.Equal(t, "Application/team-a", report.Root)

	out := renderDeliveryTreeASCII(report, 1)
	require.Contains(t, out, "DELIVERY TREE  Application/team-a  (context prod-east)")
	require.Contains(t, out, "├─ ConfigMap web/web-settings  ·  Synced\n")
	// The rest is counted by kind and the flag that lists it is named. It
	// never just stops.
	require.Contains(t, out, "└─ … 2 resources more (1 Deployment, 1 Service); --max-resources 0 lists all\n")
	require.NotContains(t, out, "Deployment web/web")

	require.Contains(t, renderDeliveryTreeASCII(report, 0), "Deployment web/web")
	require.NotContains(t, renderDeliveryTreeASCII(report, 3), "more (")

	markdown := renderDeliveryTreeMarkdown(report, 1)
	require.Contains(t, markdown, "  - … 2 resources more (1 Deployment, 1 Service); --max-resources 0 lists all\n")
}

func TestGitOpsTreeMarkdown(t *testing.T) {
	out := renderDeliveryTreeMarkdown(treeReport(t, treeFixtureClient(), deliveryTreeParams{}), 25)
	for _, want := range []string{
		"# Delivery tree\n",
		"Context: `prod-east`",
		"2 roots, 4 deployers, 4 resources reported.",
		"> Each deployer's resources are what it reports about itself. They are not checked against the cluster.",
		"- **1 reported but not found in the cluster**",
		"- **Application argocd/platform** · Synced · Healthy · auto-sync on, self-heal on, prune on\n",
		"  - **Application argocd/gone**\n    - _not found in the cluster_\n    - _parent ignores differences at /spec/syncPolicy_\n",
		"  - **Application argocd/team-a** · OutOfSync · Degraded · auto-sync on, self-heal off, prune off\n",
		"    - Deployment web/web  ·  OutOfSync\n",
		"  - ConfigMap platform/settings  ·  Synced\n",
		"- **Kustomization flux-system/apps** · suspend off, prune on, force off, wait off\n",
		"## Reads\n\n| Controller | Kind | Status | Count |\n|---|---|---|---|\n| Argo CD | Application | read | 2 |\n",
	} {
		require.Contains(t, out, want)
	}
}

func TestGitOpsTreeJSONIsTheWholeTreeAndSaysWhatItIs(t *testing.T) {
	var out bytes.Buffer
	params := deliveryTreeParams{View: deliveryTreeViewAll}
	require.NoError(t, writeDeliveryTree(&out, treeReport(t, treeFixtureClient(), params), "json", params))
	var report deliveryTreeReport
	require.NoError(t, json.Unmarshal(out.Bytes(), &report))

	require.True(t, report.Complete)
	require.Equal(t, deliveryTreeEvidence, report.Evidence)
	require.Equal(t, agent.DeliveryTreeSummary{Roots: 2, Deployers: 4, Resources: 4, MaxDepth: 1, NotFound: 1, NotReconciled: 1, NotSupported: 1}, report.Summary)
	require.Len(t, report.Reads, 3)
	require.Len(t, report.Notes, 2)
	require.Nil(t, report.Filters)
	require.Nil(t, report.Shown)

	platform := report.Roots[0]
	require.Equal(t, "platform", platform.Name)
	gone := platform.Children.Deployers[0]
	require.Equal(t, agent.DeliveryObjectNotFound, gone.Object)
	require.Equal(t, agent.DeliveryChildrenNoObject, gone.Children.Status)
	// The parent's rule is on the child it names, verbatim.
	require.Equal(t, []interface{}{map[string]interface{}{"group": "argoproj.io", "kind": "Application", "jsonPointers": []interface{}{"/spec/syncPolicy"}}},
		gone.ReportedByParent.IgnoredByParent)
	require.Empty(t, platform.Children.Resources[0].IgnoredByParent)
	// A resource has no health because none was reported; the field is
	// absent, not empty and not "Healthy".
	require.NotContains(t, out.String(), `"health": ""`)
	require.Equal(t, 2, strings.Count(out.String(), `"health"`), "only the two Applications that report their own")
}

func TestGitOpsTreeSummaryViewCountsLeavesAndNamesTheExceptions(t *testing.T) {
	var out bytes.Buffer
	params := deliveryTreeParams{View: deliveryTreeViewSummary}
	require.NoError(t, writeDeliveryTree(&out, treeReport(t, treeFixtureClient(), params), "json", params))
	require.Equal(t, 1, strings.Count(out.String(), "\n"), "one line")

	var document struct {
		View     string                       `json:"view"`
		Complete bool                         `json:"complete"`
		Evidence string                       `json:"evidence"`
		Summary  agent.DeliveryTreeSummary    `json:"summary"`
		Reads    []agent.DeliverySettingsRead `json:"reads"`
		Notes    []string                     `json:"notes"`
		Roots    json.RawMessage              `json:"roots"`
		Tree     []deliveryTreeSummaryNode    `json:"tree"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &document))
	require.Equal(t, deliveryTreeViewSummary, document.View)
	require.Nil(t, document.Roots, "the full tree is left out")
	// What says whether the answer is whole is kept.
	require.True(t, document.Complete)
	require.Equal(t, deliveryTreeEvidence, document.Evidence)
	require.Equal(t, 4, document.Summary.Deployers)
	require.Len(t, document.Reads, 3)
	require.Len(t, document.Notes, 2)

	require.Equal(t, deliveryTreeSummaryNode{
		Deployer: "Application argocd/platform", Sync: "Synced", Health: "Healthy",
		Policies:  "auto-sync on, self-heal on, prune on",
		Resources: map[string]int{"ConfigMap": 1},
		Deployers: []deliveryTreeSummaryNode{
			{Deployer: "Application argocd/gone", Object: agent.DeliveryObjectNotFound,
				Marks: []string{"not found in the cluster", "parent ignores differences at /spec/syncPolicy"}},
			{Deployer: "Application argocd/team-a", Sync: "OutOfSync", Health: "Degraded",
				Policies: "auto-sync on, self-heal off, prune off", Marks: []string{"parent ignores differences at /spec/syncPolicy"},
				Resources: map[string]int{"ConfigMap": 1, "Deployment": 1, "Service": 1},
				NotSynced: []string{"Deployment web/web OutOfSync"}},
		},
	}, document.Tree[0])
	require.Equal(t, "Kustomization flux-system/apps", document.Tree[1].Deployer)
	require.Contains(t, document.Tree[1].Marks, "what a Kustomization delivers is not read yet (#856)")
}

func TestGitOpsTreeFiltersKeepTheDeployersAbove(t *testing.T) {
	names := func(report deliveryTreeReport) []string {
		var out []string
		var walk func(nodes []agent.DeliveryTreeNode, depth int)
		walk = func(nodes []agent.DeliveryTreeNode, depth int) {
			for _, node := range nodes {
				out = append(out, strings.Repeat(">", depth)+node.Name)
				for _, resource := range node.Children.Resources {
					out = append(out, strings.Repeat(">", depth+1)+resource.Kind+"/"+resource.Name)
				}
				walk(node.Children.Deployers, depth+1)
			}
		}
		walk(report.Roots, 0)
		return out
	}
	for name, tc := range map[string]struct {
		params deliveryTreeParams
		want   []string
		shown  deliveryTreeShown
	}{
		// A Deployment two levels down, with both deployers above it.
		"one kind": {deliveryTreeParams{Kinds: []string{"deployment"}}, []string{"platform", ">team-a", ">>Deployment/web"}, deliveryTreeShown{2, 1}},
		"two kinds": {deliveryTreeParams{Kinds: []string{"Service", "ConfigMap"}},
			[]string{"platform", ">ConfigMap/settings", ">team-a", ">>ConfigMap/web-settings", ">>Service/web"}, deliveryTreeShown{2, 3}},
		// A deployer is matched on its own sync, so team-a stays for itself
		// as well as for its Deployment.
		"out of sync":    {deliveryTreeParams{Syncs: []string{"OutOfSync"}}, []string{"platform", ">team-a", ">>Deployment/web"}, deliveryTreeShown{2, 1}},
		"kind and sync":  {deliveryTreeParams{Kinds: []string{"Service"}, Syncs: []string{"OutOfSync"}}, nil, deliveryTreeShown{}},
		"a deployer":     {deliveryTreeParams{Kinds: []string{"Kustomization"}}, []string{"apps"}, deliveryTreeShown{1, 0}},
		"its own health": {deliveryTreeParams{Healths: []string{"Degraded"}}, []string{"platform", ">team-a"}, deliveryTreeShown{2, 0}},
		"no such health": {deliveryTreeParams{Healths: []string{"Missing"}}, nil, deliveryTreeShown{}},
		// A child that is not in the cluster has no state of its own. It is
		// matched on what its parent reports of it.
		"a missing child, by its parent's report": {deliveryTreeParams{Syncs: []string{"Synced"}, Kinds: []string{"Application"}},
			[]string{"platform", ">gone"}, deliveryTreeShown{2, 0}},
	} {
		t.Run(name, func(t *testing.T) {
			report := treeReport(t, treeFixtureClient(), tc.params)
			require.Equal(t, tc.want, names(report))
			require.Equal(t, &tc.shown, report.Shown)
			// The summary still counts the whole tree.
			require.Equal(t, 4, report.Summary.Deployers)
			require.Equal(t, 4, report.Summary.Resources)
			require.NotNil(t, report.Filters)
			out := renderDeliveryTreeASCII(report, 25)
			require.Contains(t, out, "2 roots, 4 deployers, 4 resources reported; showing ")
			if tc.want == nil {
				require.Contains(t, out, "Nothing matches the filters.")
				require.NotNil(t, report.Roots, "an empty result is an empty list in JSON, not null")
			}
		})
	}
}

func TestGitOpsTreeRootArgument(t *testing.T) {
	for raw, want := range map[string]agent.DeliveryRef{
		"app/platform":                {Kind: "Application", Name: "platform"},
		"Application/platform":        {Kind: "Application", Name: "platform"},
		"applications/argocd/root":    {Kind: "Application", Namespace: "argocd", Name: "root"},
		"ks/apps":                     {Kind: "Kustomization", Name: "apps"},
		"kustomization/flux-system/a": {Kind: "Kustomization", Namespace: "flux-system", Name: "a"},
		"hr/chart":                    {Kind: "HelmRelease", Name: "chart"},
	} {
		ref, err := parseDeliveryRef(raw, "")
		require.NoError(t, err, raw)
		require.Equal(t, want, *ref, raw)
	}
	ref, err := parseDeliveryRef("app/platform", "argocd")
	require.NoError(t, err)
	require.Equal(t, agent.DeliveryRef{Kind: "Application", Namespace: "argocd", Name: "platform"}, *ref)
	ref, err = parseDeliveryRef("app/argocd/platform", "argocd")
	require.NoError(t, err)
	require.Equal(t, "argocd", ref.Namespace)

	for raw, want := range map[string]string{
		"platform":             "invalid deployer",
		"app/":                 "invalid deployer",
		"app//platform":        "invalid deployer",
		"app/a/b/c":            "invalid deployer",
		"deploy/web":           `unknown deployer kind "deploy"`,
		"app/team-b/platform ": `names namespace "team-b" and --namespace names "argocd"`,
	} {
		_, err := parseDeliveryRef(raw, "argocd")
		require.ErrorContains(t, err, want, raw)
	}

	// A root that is not there is an error, and says so plainly; when a list
	// could not be read it says the deployer may exist.
	_, err = buildDeliveryTreeReport(context.Background(), treeFixtureClient(), deliveryTreeParams{Root: &agent.DeliveryRef{Kind: "Application", Name: "nope"}})
	require.EqualError(t, err, "no deployer Application/nope was found")
	denied := treeFixtureClient()
	denied.PrependReactor("list", "applications", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "applications"}, "", nil)
	})
	_, err = buildDeliveryTreeReport(context.Background(), denied, deliveryTreeParams{Root: &agent.DeliveryRef{Kind: "Application", Name: "platform"}})
	require.EqualError(t, err, "no deployer Application/platform was found; some deployers could not be listed, so it may exist")
}

func TestGitOpsTreeSaysWhenItIsIncomplete(t *testing.T) {
	denied := treeFixtureClient()
	denied.PrependReactor("list", "kustomizations", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "kustomizations"}, "", nil)
	})
	report := treeReport(t, denied, deliveryTreeParams{})
	require.False(t, report.Complete)
	out := renderDeliveryTreeASCII(report, 25)
	require.Contains(t, out, "INCOMPLETE: something could not be read; see Reads.")
	require.Contains(t, out, "  Flux Kustomization: not read (forbidden)")
	require.Contains(t, renderDeliveryTreeMarkdown(report, 25), "**Incomplete:** something could not be read; see Reads.")
	require.Contains(t, renderDeliveryTreeMarkdown(report, 25), "| Flux | Kustomization | not read (forbidden) |  |")

	// A child that may not be read makes the tree incomplete too, though
	// every list succeeded: something under that child is unknown.
	scoped := treeFixtureClient(
		treeApplication("outer", "Synced", "Healthy", nil, []interface{}{treeEntry("argoproj.io", "Application", "team-x", "inner", "Synced")}))
	scoped.PrependReactor("get", "applications", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "applications"}, "inner", nil)
	})
	report = treeReport(t, scoped, deliveryTreeParams{Namespace: "argocd", Root: &agent.DeliveryRef{Kind: "Application", Name: "outer"}})
	require.Equal(t, 1, report.Summary.NotRead)
	require.False(t, report.Complete)
	out = renderDeliveryTreeASCII(report, 25)
	require.Contains(t, out, "INCOMPLETE")
	require.Contains(t, out, "! could not be read: forbidden")
	require.Contains(t, out, "  ! 1 could not be read")

	// With no deployers at all, it says so rather than printing nothing.
	empty := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), settingsListKinds)
	require.Contains(t, renderDeliveryTreeASCII(treeReport(t, empty, deliveryTreeParams{}), 25), "No deployers were found.")
}

func TestGitOpsTreeFlagValidation(t *testing.T) {
	run := func(args ...string) error {
		cmd := &cobra.Command{Use: "tree", Args: cobra.MaximumNArgs(1)}
		addGitOpsTreeFlags(cmd.Flags())
		require.NoError(t, cmd.ParseFlags(args))
		_, err := deliveryTreeParamsFromFlags(cmd, cmd.Flags().Args())
		return err
	}
	require.NoError(t, run())
	require.NoError(t, run("app/platform", "-n", "argocd", "--depth", "2", "--kind", "Deployment,Service", "--sync", "OutOfSync", "--view", "Summary"))
	require.ErrorContains(t, run("--depth", "-1"), "invalid --depth -1")
	require.ErrorContains(t, run("--max-resources", "-2"), "invalid --max-resources -2")
	require.ErrorContains(t, run("--view", "groups"), `invalid --view "groups" (valid: all, summary)`)
	require.ErrorContains(t, run("pod/web"), `unknown deployer kind "pod"`)

	cmd := &cobra.Command{Use: "tree"}
	addGitOpsTreeFlags(cmd.Flags())
	require.NoError(t, cmd.ParseFlags([]string{"--kind", " Deployment ", "--kind", "", "--health", "Degraded"}))
	params, err := deliveryTreeParamsFromFlags(cmd, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"Deployment"}, params.Kinds)
	require.Equal(t, []string{"Degraded"}, params.Healths)
	require.Equal(t, 25, params.MaxResources)
	require.True(t, params.filtered())
}

func TestGitOpsTreeTUIShowsTheMarkdownSnapshot(t *testing.T) {
	report := treeReport(t, treeFixtureClient(), deliveryTreeParams{})
	model := newGitOpsMarkdownTUIModel(renderDeliveryTreeMarkdown(report, 25) + "\x1b[31m")
	require.Contains(t, model.content, "# Delivery tree")
	require.Contains(t, model.content, "Application argocd/team-a")
	require.NotContains(t, model.content, "\x1b", "control characters from cluster data never reach the terminal")
}

// Names, sync states and condition messages are text written by whoever can
// create an Application. Printed raw, a newline forges a line of the tree and
// an escape sequence rewrites the terminal.
func TestGitOpsTreeTextOutputNeutralisesClusterSuppliedControlCharacters(t *testing.T) {
	hostile := treeApplication("evil\n└─ Application argocd/forged", "Synced\x1b[2J", "Healthy", nil,
		[]interface{}{treeEntry("", "ConfigMap", "ns\rX", "cm\n! forged mark", "Synced\x07")})
	hostile.Object["status"].(map[string]interface{})["conditions"] = []interface{}{
		map[string]interface{}{"type": "ComparisonError", "message": "line one\nReads:\n  Argo CD Application: read (999)"},
	}
	// A parent's ignore rule is printed on the child it names: its text is
	// the parent author's too.
	parent := treeApplication("parent", "Synced", "Healthy", map[string]interface{}{"ignoreDifferences": []interface{}{
		map[string]interface{}{"group": "argoproj.io", "kind": "Application", "jsonPointers": []interface{}{"/spec\x1b[2J\n└─ Application argocd/forged"}}}},
		[]interface{}{treeAppEntry("child")})
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), settingsListKinds, hostile, parent)
	report := treeReport(t, client, deliveryTreeParams{})

	for name, out := range map[string]string{"ascii": renderDeliveryTreeASCII(report, 25), "markdown": renderDeliveryTreeMarkdown(report, 25)} {
		t.Run(name, func(t *testing.T) {
			require.NotContains(t, out, "\x1b")
			require.NotContains(t, out, "\x07")
			require.NotContains(t, out, "\r")
			for _, line := range strings.Split(out, "\n") {
				require.False(t, strings.HasPrefix(strings.TrimSpace(line), "└─ Application argocd/forged"), "a forged tree line: %q", line)
				require.False(t, strings.HasPrefix(strings.TrimSpace(line), "! forged mark"), "a forged mark: %q", line)
			}
			// The condition's text is one line of the tree. It starts no
			// line of its own, so the only Reads section is the real one.
			readsHeadings, forgedReads := 0, 0
			for _, line := range strings.Split(out, "\n") {
				switch trimmed := strings.TrimSpace(line); {
				case trimmed == "Reads:" || trimmed == "## Reads":
					readsHeadings++
				case strings.HasPrefix(trimmed, "Argo CD Application: read (999)"):
					forgedReads++
				}
			}
			require.Equal(t, 1, readsHeadings)
			require.Zero(t, forgedReads)
			require.Contains(t, out, "ComparisonError: line one Reads: Argo CD Application: read (999)")
		})
	}
	// JSON is left verbatim; it is data, not a terminal.
	var out bytes.Buffer
	params := deliveryTreeParams{View: deliveryTreeViewAll}
	require.NoError(t, writeDeliveryTree(&out, report, "json", params))
	require.Contains(t, out.String(), `evil\n└─ Application argocd/forged`)
}
