// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktesting "k8s.io/client-go/testing"
)

// The tests in this file are about how the walk behaves: cycles, shared and
// missing children, depth and scope. Their inputs are written by hand, with a
// status.resources of the shape a real Argo CD records (see the recorded
// tests). What a real controller reports is tested from the recording.

// reported is one status.resources entry as Argo CD writes it.
func reported(group, kind, namespace, name, sync string) map[string]interface{} {
	entry := map[string]interface{}{"version": "v1", "kind": kind, "name": name, "status": sync}
	if group != "" {
		entry["group"] = group
	}
	if namespace != "" {
		entry["namespace"] = namespace
	}
	return entry
}

func reportedApp(namespace, name string) map[string]interface{} {
	entry := reported("argoproj.io", "Application", namespace, name, "Synced")
	entry["version"] = "v1alpha1"
	return entry
}

// treeApp is an Application with a spec and, unless resources is nil, a
// status that reports them.
func treeApp(namespace, name string, spec map[string]interface{}, resources []interface{}) *unstructured.Unstructured {
	app := settingsApp(namespace, name, spec)
	if resources != nil {
		app.Object["status"] = map[string]interface{}{
			"sync":      map[string]interface{}{"status": "Synced", "revision": "abc123"},
			"health":    map[string]interface{}{"status": "Healthy"},
			"resources": resources,
		}
	}
	return app
}

func treeNames(nodes []DeliveryTreeNode) []string {
	names := make([]string, len(nodes))
	for i, node := range nodes {
		names[i] = node.Namespace + "/" + node.Name
	}
	return names
}

func TestDeliveryTreeWalksEveryLevelAndKeepsWhatItCouldNotSee(t *testing.T) {
	automated := map[string]interface{}{"syncPolicy": map[string]interface{}{"automated": map[string]interface{}{"selfHeal": true}}}
	client := deliveryClient(
		treeApp("argocd", "root", automated, []interface{}{
			reportedApp("argocd", "team"),
			reportedApp("argocd", "gone"),
			reportedApp("argocd", "idle"),
			reported("", "ConfigMap", "platform", "settings", "Synced"),
			reported("", "Namespace", "", "platform", "OutOfSync"),
		}),
		treeApp("argocd", "team", nil, []interface{}{
			reportedApp("argocd", "leaf"),
			reported("apps", "Deployment", "web", "web", "Synced"),
		}),
		treeApp("argocd", "leaf", nil, []interface{}{}),
		// Exists, and no controller has given it a status.
		treeApp("argocd", "idle", nil, nil),
		treeApp("other", "standalone", nil, []interface{}{reported("", "Service", "other", "svc", "Synced")}),
	)

	tree, err := CollectDeliveryTree(context.Background(), client, DeliveryTreeOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"argocd/root", "other/standalone"}, treeNames(tree.Roots), "a root is a deployer no other deployer reports")

	root := tree.Roots[0]
	require.Equal(t, DeliveryObjectFound, root.Object)
	require.Equal(t, &DeliveryDeployerState{Reconciled: true, Sync: "Synced", Health: "Healthy", Revision: "abc123"}, root.State)
	require.Equal(t, []DeliveryPolicyValue{{"auto-sync", "on"}, {"self-heal", "on"}, {"prune", "off"}}, root.Policies)
	require.Nil(t, root.ReportedByParent)
	require.Equal(t, DeliveryChildrenReported, root.Children.Status)
	require.Equal(t, []string{"argocd/gone", "argocd/idle", "argocd/team"}, treeNames(root.Children.Deployers))
	// Resources are sorted by kind, and a cluster-scoped one has no namespace.
	require.Equal(t, []DeliveryReported{
		{Version: "v1", Kind: "ConfigMap", Namespace: "platform", Name: "settings", Sync: "Synced"},
		{Version: "v1", Kind: "Namespace", Name: "platform", Sync: "OutOfSync"},
	}, root.Children.Resources)

	gone, idle, team := root.Children.Deployers[0], root.Children.Deployers[1], root.Children.Deployers[2]
	// Reported by its parent and not in the cluster: kept, and said so.
	require.Equal(t, DeliveryObjectNotFound, gone.Object)
	require.Equal(t, DeliveryChildrenNoObject, gone.Children.Status)
	require.Nil(t, gone.State)
	require.Equal(t, "Synced", gone.ReportedByParent.Sync, "what the parent says about it is kept")
	// In the cluster with no status: not reconciled, and it reports nothing.
	// That is not the same as having nothing under it.
	require.Equal(t, DeliveryObjectFound, idle.Object)
	require.Equal(t, &DeliveryDeployerState{Reconciled: false}, idle.State)
	require.Equal(t, DeliveryChildrenNoneReported, idle.Children.Status)
	require.NotEmpty(t, idle.Children.Reason)

	require.Equal(t, []string{"argocd/leaf"}, treeNames(team.Children.Deployers))
	require.Equal(t, "Deployment", team.Children.Resources[0].Kind)
	leaf := team.Children.Deployers[0]
	// An Application that reports an empty list has nothing under it.
	require.Equal(t, DeliveryChildrenReported, leaf.Children.Status)
	require.Empty(t, leaf.Children.Deployers)
	require.Empty(t, leaf.Children.Resources)

	require.Equal(t, DeliveryTreeSummary{
		Roots: 2, Deployers: 6, Resources: 4, MaxDepth: 2, NotFound: 1, NotReconciled: 1, NoneReported: 1,
	}, tree.Summary)
}

func TestDeliveryTreeFromOneRoot(t *testing.T) {
	client := deliveryClient(
		treeApp("argocd", "root", nil, []interface{}{reportedApp("argocd", "team")}),
		treeApp("argocd", "team", nil, []interface{}{reportedApp("argocd", "leaf")}),
		treeApp("argocd", "leaf", nil, []interface{}{}),
		treeApp("argocd", "other", nil, []interface{}{}),
		treeApp("elsewhere", "team", nil, []interface{}{}),
	)
	ctx := context.Background()

	// A deployer in the middle of a tree can be the root asked for.
	tree, err := CollectDeliveryTree(ctx, client, DeliveryTreeOptions{Root: &DeliveryRef{Kind: "Application", Namespace: "argocd", Name: "team"}})
	require.NoError(t, err)
	require.Equal(t, []string{"argocd/team"}, treeNames(tree.Roots))
	require.Equal(t, []string{"argocd/leaf"}, treeNames(tree.Roots[0].Children.Deployers))
	require.Equal(t, 2, tree.Summary.Deployers)

	// A name in two namespaces is not chosen between.
	_, err = CollectDeliveryTree(ctx, client, DeliveryTreeOptions{Root: &DeliveryRef{Kind: "Application", Name: "team"}})
	var rootErr *DeliveryRootError
	require.ErrorAs(t, err, &rootErr)
	require.Len(t, rootErr.Candidates, 2)
	require.ErrorContains(t, err, "names more than one deployer (Application argocd/team, Application elsewhere/team)")

	_, err = CollectDeliveryTree(ctx, client, DeliveryTreeOptions{Root: &DeliveryRef{Kind: "Application", Name: "missing"}})
	require.ErrorAs(t, err, &rootErr)
	require.Empty(t, rootErr.Candidates)
	require.EqualError(t, err, "no deployer Application/missing was found")

	// Kind is part of the name: a Kustomization called "root" is not it.
	_, err = CollectDeliveryTree(ctx, client, DeliveryTreeOptions{Root: &DeliveryRef{Kind: "Kustomization", Name: "root"}})
	require.Error(t, err)
}

func TestDeliveryTreeStopsAtTheDepthAskedFor(t *testing.T) {
	client := deliveryClient(
		treeApp("argocd", "root", nil, []interface{}{reportedApp("argocd", "team"), reported("", "ConfigMap", "p", "c", "Synced")}),
		treeApp("argocd", "team", nil, []interface{}{reportedApp("argocd", "leaf"), reported("", "ConfigMap", "p", "d", "Synced")}),
		treeApp("argocd", "leaf", nil, []interface{}{reported("", "ConfigMap", "p", "e", "Synced")}),
	)
	tree, err := CollectDeliveryTree(context.Background(), client, DeliveryTreeOptions{Depth: 1})
	require.NoError(t, err)
	root := tree.Roots[0]
	// The root's own children are listed.
	require.Len(t, root.Children.Resources, 1)
	team := root.Children.Deployers[0]
	// The child is shown with its own state and settings, and is not walked:
	// the cut says so and lists nothing, so nothing reads as "empty".
	require.NotNil(t, team.State)
	require.Equal(t, DeliveryChildren{Status: DeliveryChildrenDepthLimit}, team.Children)
	require.Equal(t, DeliveryTreeSummary{Roots: 1, Deployers: 2, Resources: 1, MaxDepth: 1, DepthLimited: 1}, tree.Summary)

	tree, err = CollectDeliveryTree(context.Background(), client, DeliveryTreeOptions{Depth: 2})
	require.NoError(t, err)
	leaf := tree.Roots[0].Children.Deployers[0].Children.Deployers[0]
	require.Equal(t, DeliveryChildrenDepthLimit, leaf.Children.Status)
	require.Equal(t, 2, tree.Summary.Resources)
}

func TestDeliveryTreeCyclesAndSharedChildren(t *testing.T) {
	client := deliveryClient(
		treeApp("argocd", "a", nil, []interface{}{reportedApp("argocd", "b"), reportedApp("argocd", "shared")}),
		treeApp("argocd", "b", nil, []interface{}{reportedApp("argocd", "a"), reportedApp("argocd", "shared")}),
		treeApp("argocd", "shared", nil, []interface{}{reported("", "ConfigMap", "p", "c", "Synced")}),
		// Reports itself.
		treeApp("argocd", "self", nil, []interface{}{reportedApp("argocd", "self")}),
	)
	tree, err := CollectDeliveryTree(context.Background(), client, DeliveryTreeOptions{})
	require.NoError(t, err)
	// a and b report each other, so neither is a root by the rule. Nothing
	// read is left out: the ring is entered once, at its first member.
	require.Equal(t, []string{"argocd/a", "argocd/self"}, treeNames(tree.Roots))

	a := tree.Roots[0]
	b := a.Children.Deployers[0]
	require.Equal(t, "b", b.Name)
	again := b.Children.Deployers[0]
	require.Equal(t, "a", again.Name)
	require.Equal(t, DeliveryChildren{Status: DeliveryChildrenCycle}, again.Children, "a is already above: it is not walked again")
	require.NotNil(t, again.State, "it is still shown as what it is")

	// The shared child is under both, and says two deployers report it.
	for _, parent := range []DeliveryTreeNode{a, b} {
		shared := parent.Children.Deployers[len(parent.Children.Deployers)-1]
		require.Equal(t, "shared", shared.Name)
		require.Equal(t, 2, shared.ReportedBy)
		require.Len(t, shared.Children.Resources, 1)
	}

	self := tree.Roots[1]
	require.Equal(t, 0, self.ReportedBy, "reporting itself does not make it shared")
	require.Equal(t, DeliveryChildrenCycle, self.Children.Deployers[0].Children.Status)
	require.Equal(t, 2, tree.Summary.Cycles)
	require.Equal(t, 2, tree.Summary.Shared)
}

func TestDeliveryTreeReadsAChildOutsideTheNamespaceListed(t *testing.T) {
	client := deliveryClient(
		treeApp("argocd", "root", nil, []interface{}{reportedApp("team-a", "inside"), reportedApp("team-a", "absent"), reportedApp("team-b", "denied")}),
		treeApp("team-a", "inside", nil, []interface{}{reported("", "ConfigMap", "team-a", "c", "Synced")}),
		treeApp("team-b", "denied", nil, []interface{}{}),
	)
	client.PrependReactor("get", "applications", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() == "team-b" {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "argoproj.io", Resource: "applications"}, "denied", nil)
		}
		return false, nil, nil
	})

	tree, err := CollectDeliveryTree(context.Background(), client, DeliveryTreeOptions{Namespace: "argocd"})
	require.NoError(t, err)
	require.Equal(t, []string{"argocd/root"}, treeNames(tree.Roots))
	children := tree.Roots[0].Children.Deployers
	require.Equal(t, []string{"team-a/absent", "team-a/inside", "team-b/denied"}, treeNames(children))
	require.Equal(t, DeliveryObjectNotFound, children[0].Object)
	// Outside the namespace listed, so it was read on its own, and walked.
	require.Equal(t, DeliveryObjectFound, children[1].Object)
	require.Len(t, children[1].Children.Resources, 1)
	// A child that may not be read is not a child that is not there.
	require.Equal(t, DeliveryObjectNotRead, children[2].Object)
	require.Equal(t, "forbidden", children[2].ObjectReason)
	require.Equal(t, 1, tree.Summary.NotRead)
	require.Equal(t, 1, tree.Summary.NotFound)
}

func TestDeliveryTreeWhenAKindCannotBeListed(t *testing.T) {
	client := deliveryClient(deliveryObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", map[string]interface{}{"prune": true}))
	failList(client, "applications", "", apierrors.NewForbidden(schema.GroupResource{Resource: "applications"}, "", nil))

	tree, err := CollectDeliveryTree(context.Background(), client, DeliveryTreeOptions{})
	require.NoError(t, err)
	reads := map[string]DeliverySettingsRead{}
	for _, read := range tree.Reads {
		reads[read.Kind] = read
	}
	require.Equal(t, DeliveryReadNotRead, reads["Application"].Status)
	require.Equal(t, "forbidden", reads["Application"].Reason)

	// A Flux deployer is in the tree with its own settings, and says its
	// children are not read yet. That is not "nothing under it".
	require.Equal(t, []string{"flux-system/apps"}, treeNames(tree.Roots))
	apps := tree.Roots[0]
	require.Equal(t, DeliveryControllerFlux, apps.Controller)
	require.Equal(t, DeliveryChildrenNotSupported, apps.Children.Status)
	require.Contains(t, apps.Children.Reason, "#856")
	require.Contains(t, apps.Policies, DeliveryPolicyValue{Name: "prune", Value: "on"})
	require.Equal(t, 1, tree.Summary.NotSupported)
}

// An Application that applies a Flux object has it as a child deployer: the
// rule is "a reported entry of a kind this package reads", not "an Application".
func TestDeliveryTreeFollowsADeployerOfAnotherController(t *testing.T) {
	kustomization := deliveryObject("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", "apps", map[string]interface{}{"prune": true})
	kustomization.Object["status"] = map[string]interface{}{
		"lastAppliedRevision": "main@sha1:abc",
		"conditions":          []interface{}{map[string]interface{}{"type": "Ready", "status": "True", "message": "Applied revision"}},
	}
	entry := reported("kustomize.toolkit.fluxcd.io", "Kustomization", "flux-system", "apps", "Synced")
	client := deliveryClient(treeApp("argocd", "bootstrap", nil, []interface{}{entry}), kustomization)

	tree, err := CollectDeliveryTree(context.Background(), client, DeliveryTreeOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"argocd/bootstrap"}, treeNames(tree.Roots))
	child := tree.Roots[0].Children.Deployers[0]
	require.Equal(t, DeliveryControllerFlux, child.Controller)
	require.Equal(t, "Kustomization", child.Kind)
	require.Equal(t, &DeliveryDeployerState{
		Reconciled: true, Health: "Ready=True", Revision: "main@sha1:abc",
		Conditions: []DeliveryCondition{{Type: "Ready", Status: "True", Message: "Applied revision"}},
	}, child.State)
	require.Equal(t, DeliveryChildrenNotSupported, child.Children.Status)
}

func TestIgnoreRulesNamingAnEntry(t *testing.T) {
	rule := func(group, kind string, extra map[string]interface{}) map[string]interface{} {
		out := map[string]interface{}{"kind": kind, "jsonPointers": []interface{}{"/spec/syncPolicy"}}
		if group != "" {
			out["group"] = group
		}
		for key, value := range extra {
			out[key] = value
		}
		return out
	}
	app := DeliveryReported{Group: "argoproj.io", Kind: "Application", Namespace: "argocd", Name: "team"}
	core := DeliveryReported{Kind: "ConfigMap", Namespace: "p", Name: "c"}

	for name, tc := range map[string]struct {
		rule  map[string]interface{}
		entry DeliveryReported
		want  bool
	}{
		"group and kind":             {rule("argoproj.io", "Application", nil), app, true},
		"another kind":               {rule("argoproj.io", "AppProject", nil), app, false},
		"another group":              {rule("apps", "Application", nil), app, false},
		"no group is the core group": {rule("", "ConfigMap", nil), core, true},
		"no group is not any group":  {rule("", "Application", nil), app, false},
		"wildcards":                  {rule("*", "*", nil), app, true},
		"the name given":             {rule("argoproj.io", "Application", map[string]interface{}{"name": "team"}), app, true},
		"another name":               {rule("argoproj.io", "Application", map[string]interface{}{"name": "other"}), app, false},
		"the namespace given":        {rule("argoproj.io", "Application", map[string]interface{}{"namespace": "argocd"}), app, true},
		"another namespace":          {rule("argoproj.io", "Application", map[string]interface{}{"namespace": "x"}), app, false},
	} {
		t.Run(name, func(t *testing.T) {
			naming := ignoreRulesNaming([]interface{}{tc.rule, "not a rule"}, tc.entry)
			if !tc.want {
				require.Empty(t, naming)
				return
			}
			require.Equal(t, []interface{}{tc.rule}, naming, "the rule is kept verbatim")
		})
	}

	// On the walk, a parent's rule is put on the child it names and nowhere else.
	client := deliveryClient(
		treeApp("argocd", "root", map[string]interface{}{"ignoreDifferences": []interface{}{rule("argoproj.io", "Application", nil)}},
			[]interface{}{reportedApp("argocd", "team"), reported("", "ConfigMap", "p", "c", "Synced")}),
		treeApp("argocd", "team", nil, []interface{}{}),
	)
	tree, err := CollectDeliveryTree(context.Background(), client, DeliveryTreeOptions{})
	require.NoError(t, err)
	root := tree.Roots[0]
	require.Len(t, root.Children.Deployers[0].ReportedByParent.IgnoredByParent, 1)
	require.Empty(t, root.Children.Resources[0].IgnoredByParent)
}

func TestReportedByDeployerReadsOnlyWhatIsThere(t *testing.T) {
	source, _ := deliverySourceFor("argoproj.io", "Application")
	app := treeApp("argocd", "a", nil, []interface{}{
		map[string]interface{}{"group": "apps", "version": "v1", "kind": "Deployment", "namespace": "n", "name": "d", "status": "OutOfSync",
			"health": map[string]interface{}{"status": "Degraded"}, "requiresPruning": true, "hook": true},
		map[string]interface{}{"kind": "Service", "name": ""},
		map[string]interface{}{"name": "no-kind"},
		"not an entry",
	})
	entries, status, _ := reportedByDeployer(source, app)
	require.Equal(t, DeliveryChildrenReported, status)
	// Health is kept when the deployer reports it; an entry that names
	// nothing is not an entry.
	require.Equal(t, []DeliveryReported{{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "n", Name: "d",
		Sync: "OutOfSync", Health: "Degraded", RequiresPruning: true, Hook: true}}, entries)

	for name, status := range map[string]interface{}{
		"no status":              nil,
		"a status with no list":  map[string]interface{}{"sync": map[string]interface{}{"status": "Unknown"}},
		"a null list":            map[string]interface{}{"resources": nil},
		"a list that is not one": map[string]interface{}{"resources": "none"},
	} {
		t.Run(name, func(t *testing.T) {
			app := settingsApp("argocd", "a", nil)
			if status != nil {
				app.Object["status"] = status
			}
			entries, got, reason := reportedByDeployer(source, app)
			require.Equal(t, DeliveryChildrenNoneReported, got)
			require.NotEmpty(t, reason)
			require.Nil(t, entries)
		})
	}
}

// recordedDeliveryTree is the example app-of-apps as a real Argo CD v3.5.3
// left it (see the fixture's NOTICE).
func recordedDeliveryTree(t *testing.T) []*unstructured.Unstructured {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "delivery-tree-argocd-v353-recorded", "applications.json"))
	require.NoError(t, err)
	var list struct {
		Items []map[string]interface{} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(data, &list))
	require.Len(t, list.Items, 8)
	objects := make([]*unstructured.Unstructured, len(list.Items))
	for i := range list.Items {
		objects[i] = &unstructured.Unstructured{Object: list.Items[i]}
	}
	return objects
}

// #855: the tree of a real app-of-apps, from what a real Argo CD reports.
func TestDeliveryTreeOfTheRecordedAppOfApps(t *testing.T) {
	recorded := recordedDeliveryTree(t)
	objects := make([]runtime.Object, len(recorded))
	byName := map[string]*unstructured.Unstructured{}
	for i, object := range recorded {
		objects[i] = object
		byName[object.GetNamespace()+"/"+object.GetName()] = object
	}
	tree, err := CollectDeliveryTree(context.Background(), deliveryClient(objects...), DeliveryTreeOptions{})
	require.NoError(t, err)

	// Two roots: the example's, and the lane's own guestbook. Every other
	// Application is reported by one above it.
	require.Equal(t, []string{"argocd/delivery-tree", "argocd/guestbook"}, treeNames(tree.Roots))
	require.Equal(t, DeliveryTreeSummary{
		Roots: 2, Deployers: 9, Resources: 10, MaxDepth: 2, NotFound: 1, NotReconciled: 1, NoneReported: 2,
	}, tree.Summary)

	root := tree.Roots[0]
	require.Equal(t, "Synced", root.State.Sync)
	require.Equal(t, "Healthy", root.State.Health)
	require.Equal(t, "b7bda177a33c2010ba635cd3d1a7747d370fd1e5", root.State.Revision)
	require.Equal(t, []DeliveryPolicyValue{{"auto-sync", "on"}, {"self-heal", "on"}, {"prune", "on"}}, root.Policies)
	require.Equal(t, []string{"CreateNamespace=true", "ignoreDifferences"}, root.Options)
	require.Equal(t, []string{
		"argocd/delivery-tree-broken", "argocd/delivery-tree-team-a", "argocd/delivery-tree-team-b",
		"delivery-tree-elsewhere-apps/delivery-tree-elsewhere",
	}, treeNames(root.Children.Deployers))
	require.Equal(t, []DeliveryReported{
		{Version: "v1", Kind: "ConfigMap", Namespace: "delivery-tree-platform", Name: "platform-settings", Sync: "Synced"},
		{Version: "v1", Kind: "Namespace", Name: "delivery-tree-elsewhere-apps", Sync: "Synced"},
		{Version: "v1", Kind: "Namespace", Name: "delivery-tree-platform", Sync: "Synced"},
	}, root.Children.Resources)
	broken, teamA, teamB, elsewhere := root.Children.Deployers[0], root.Children.Deployers[1], root.Children.Deployers[2], root.Children.Deployers[3]

	// The root's ignore rule names Applications, so it is on each of its
	// child Applications and on none of its other resources.
	rule := []interface{}{map[string]interface{}{"group": "argoproj.io", "kind": "Application", "jsonPointers": []interface{}{"/spec/syncPolicy"}}}
	for _, child := range root.Children.Deployers {
		require.Equal(t, rule, child.ReportedByParent.IgnoredByParent, child.Name)
		require.Equal(t, "Synced", child.ReportedByParent.Sync, "the root applied the child's manifest, whatever the child then did")
	}

	// Its path does not exist: it could not be compared, so it says nothing
	// about what it applied. That is not "nothing under it".
	require.Equal(t, "Unknown", broken.State.Sync)
	require.Len(t, broken.State.Conditions, 1)
	require.Equal(t, "ComparisonError", broken.State.Conditions[0].Type)
	require.Contains(t, broken.State.Conditions[0].Message, "app path does not exist")
	require.Equal(t, DeliveryChildrenNoneReported, broken.Children.Status)

	require.Equal(t, []DeliveryPolicyValue{{"auto-sync", "on"}, {"self-heal", "off"}, {"prune", "on"}}, teamA.Policies)
	require.Equal(t, []string{"argocd/delivery-tree-jobs", "argocd/delivery-tree-web"}, treeNames(teamA.Children.Deployers))
	require.Empty(t, teamA.Children.Resources)
	jobs, web := teamA.Children.Deployers[0], teamA.Children.Deployers[1]
	require.Nil(t, jobs.ReportedByParent.IgnoredByParent, "team-a has no ignore rules; the root's do not reach its grandchildren")
	require.Equal(t, []DeliveryPolicyValue{{"auto-sync", "on"}, {"self-heal", "off"}, {"prune", "off"}}, jobs.Policies)
	require.Equal(t, []DeliveryReported{
		{Version: "v1", Kind: "ConfigMap", Namespace: "delivery-tree-jobs", Name: "nightly-schedule", Sync: "Synced"},
		{Version: "v1", Kind: "ConfigMap", Namespace: "delivery-tree-jobs", Name: "weekly-schedule", Sync: "Synced"},
	}, jobs.Children.Resources)
	require.Equal(t, []DeliveryReported{
		{Version: "v1", Kind: "ConfigMap", Namespace: "delivery-tree-web", Name: "web-settings", Sync: "Synced"},
		{Group: "apps", Version: "v1", Kind: "Deployment", Namespace: "delivery-tree-web", Name: "web", Sync: "Synced"},
		{Version: "v1", Kind: "Service", Namespace: "delivery-tree-web", Name: "web", Sync: "Synced"},
	}, web.Children.Resources)

	// team-b has no automated sync and was never synced. It still reports
	// the Application it would create, which is not in the cluster: an entry
	// is what the deployer wants, not only what exists.
	require.Equal(t, "OutOfSync", teamB.State.Sync)
	require.Equal(t, "Missing", teamB.State.Health)
	require.Equal(t, []DeliveryPolicyValue{{"auto-sync", "off"}, {"self-heal", "n/a"}, {"prune", "n/a"}}, teamB.Policies)
	require.Equal(t, []string{"argocd/delivery-tree-api"}, treeNames(teamB.Children.Deployers))
	api := teamB.Children.Deployers[0]
	require.Equal(t, DeliveryObjectNotFound, api.Object)
	require.Equal(t, "OutOfSync", api.ReportedByParent.Sync)
	require.Nil(t, api.State)
	require.Equal(t, DeliveryChildrenNoObject, api.Children.Status)

	// In a namespace this Argo CD does not read: the object is there with
	// no status. Not reconciled, and it reports nothing.
	require.Equal(t, DeliveryObjectFound, elsewhere.Object)
	require.Equal(t, &DeliveryDeployerState{Reconciled: false}, elsewhere.State)
	require.Equal(t, DeliveryChildrenNoneReported, elsewhere.Children.Status)

	// No resource carries a health: Argo CD 3 does not put one on the
	// Application. Absent is "not reported".
	var walk func(nodes []DeliveryTreeNode)
	walk = func(nodes []DeliveryTreeNode) {
		for _, node := range nodes {
			for _, resource := range node.Children.Resources {
				require.Empty(t, resource.Health, resource.Name)
			}
			// Every deployer in the tree has the settings gitops settings
			// reports for it: the two commands read one model.
			if object := byName[node.Namespace+"/"+node.Name]; object != nil {
				policies, options := deliveryPolicies(ArgoApplicationDeliverySettings(object))
				require.Equal(t, policies, node.Policies, node.Name)
				require.Equal(t, options, node.Options, node.Name)
			}
			walk(node.Children.Deployers)
		}
	}
	walk(tree.Roots)

	// From the middle, and limited in depth.
	tree, err = CollectDeliveryTree(context.Background(), deliveryClient(objects...), DeliveryTreeOptions{
		Root: &DeliveryRef{Kind: "Application", Name: "delivery-tree"}, Depth: 1})
	require.NoError(t, err)
	require.Equal(t, DeliveryTreeSummary{Roots: 1, Deployers: 5, Resources: 3, MaxDepth: 1, NotReconciled: 1, NoneReported: 2, DepthLimited: 2}, tree.Summary)
}
