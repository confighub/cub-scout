// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// The delivery tree answers "what is under this deployer, to any depth" (#855).
//
// Its evidence is each deployer's own report of what it applied, read from the
// deployer object in the cluster: an Argo CD Application's status.resources. A
// reported entry that is itself a deployer this package reads is a child
// deployer, and is walked the same way. Nothing here asks the controller's own
// API, and nothing checks a reported resource against the cluster: a leaf is
// what the deployer says it manages, not what was found.

const (
	// DeliveryChildrenReported: the deployer lists what it applied. The list
	// may be empty.
	DeliveryChildrenReported = "reported"
	// DeliveryChildrenNoneReported: the deployer object has no such list. That
	// is not "no children": an Application that has never been compared, or
	// that no controller reconciles, says nothing either way.
	DeliveryChildrenNoneReported = "none_reported"
	// DeliveryChildrenNotSupported: this kind of deployer is not read for its
	// children yet.
	DeliveryChildrenNotSupported = "not_supported"
	// DeliveryChildrenDepthLimit: the walk stopped here because of the depth
	// asked for.
	DeliveryChildrenDepthLimit = "depth_limit"
	// DeliveryChildrenCycle: this deployer is already on the path from the
	// root, so it is not walked again.
	DeliveryChildrenCycle = "cycle"
	// DeliveryChildrenNoObject: the deployer object itself was not read, so
	// there is no report to read.
	DeliveryChildrenNoObject = "no_object"

	DeliveryObjectFound    = "found"
	DeliveryObjectNotFound = "not_found"
	DeliveryObjectNotRead  = "not_read"
)

// DeliveryReported is one entry of a deployer's own list of what it applied.
type DeliveryReported struct {
	Group     string `json:"group,omitempty"`
	Version   string `json:"version,omitempty"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	// Sync and Health are present only when the deployer reports them for
	// this entry. Argo CD 3 reports sync here and keeps resource health in
	// its own tree, so Health is usually absent: absent is "not reported",
	// not "healthy".
	Sync            string `json:"sync,omitempty"`
	Health          string `json:"health,omitempty"`
	RequiresPruning bool   `json:"requiresPruning,omitempty"`
	Hook            bool   `json:"hook,omitempty"`
	// IgnoredByParent are the reporting deployer's ignore rules that name
	// this entry, verbatim.
	IgnoredByParent []interface{} `json:"ignoredByParent,omitempty"`
}

// DeliveryPolicyValue is one policy setting of a deployer and the value in
// effect: the declared value, or the controller's documented default.
type DeliveryPolicyValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// DeliveryCondition is a condition a deployer reports about itself.
type DeliveryCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status,omitempty"`
	Message string `json:"message,omitempty"`
}

// DeliveryDeployerState is what a deployer object says about itself.
type DeliveryDeployerState struct {
	// Reconciled is false when the object carries no status at all: nothing
	// this cluster shows is reconciling it.
	Reconciled bool                `json:"reconciled"`
	Sync       string              `json:"sync,omitempty"`
	Health     string              `json:"health,omitempty"`
	Revision   string              `json:"revision,omitempty"`
	Conditions []DeliveryCondition `json:"conditions,omitempty"`
}

// DeliveryChildren is what one deployer reports as its own.
type DeliveryChildren struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	// Deployers are the reported entries that are themselves deployers.
	Deployers []DeliveryTreeNode `json:"deployers,omitempty"`
	// Resources are every other reported entry.
	Resources []DeliveryReported `json:"resources,omitempty"`
}

// DeliveryTreeNode is one deployer in the tree.
type DeliveryTreeNode struct {
	Controller string `json:"controller"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	// Object says whether the deployer object itself was read. A child a
	// parent reports is kept in the tree even when it was not.
	Object       string `json:"object"`
	ObjectReason string `json:"objectReason,omitempty"`
	// ReportedByParent is the parent's entry for this deployer; nil for a root.
	ReportedByParent *DeliveryReported `json:"reportedByParent,omitempty"`
	// ReportedBy is how many deployers report this one, when more than one.
	ReportedBy  int                    `json:"reportedBy,omitempty"`
	GeneratedBy string                 `json:"generatedBy,omitempty"`
	URL         string                 `json:"url,omitempty"`
	Policies    []DeliveryPolicyValue  `json:"policies,omitempty"`
	Options     []string               `json:"options,omitempty"`
	State       *DeliveryDeployerState `json:"state,omitempty"`
	Children    DeliveryChildren       `json:"children"`
}

// DeliveryTreeSummary counts what the tree holds. A deployer reported by two
// parents is counted once for each place it appears.
type DeliveryTreeSummary struct {
	Roots         int `json:"roots"`
	Deployers     int `json:"deployers"`
	Resources     int `json:"resources"`
	MaxDepth      int `json:"maxDepth"`
	NotFound      int `json:"notFound"`
	NotRead       int `json:"notRead"`
	NotReconciled int `json:"notReconciled"`
	NoneReported  int `json:"noneReported"`
	NotSupported  int `json:"notSupported"`
	Shared        int `json:"shared"`
	Cycles        int `json:"cycles"`
	DepthLimited  int `json:"depthLimited"`
}

// DeliveryTree is everything one walk read.
type DeliveryTree struct {
	Roots   []DeliveryTreeNode     `json:"roots"`
	Reads   []DeliverySettingsRead `json:"reads"`
	Summary DeliveryTreeSummary    `json:"summary"`
}

// DeliveryRef names one deployer.
type DeliveryRef struct {
	Kind      string
	Namespace string
	Name      string
}

func (r DeliveryRef) String() string {
	if r.Namespace == "" {
		return r.Kind + "/" + r.Name
	}
	return r.Kind + " " + r.Namespace + "/" + r.Name
}

// DeliveryTreeOptions scopes a walk.
type DeliveryTreeOptions struct {
	// Namespace limits the deployers listed. Empty lists every namespace. A
	// child a deployer reports in another namespace is read on its own.
	Namespace string
	// Root, when set, is the one deployer to walk from. Otherwise every
	// deployer that no other deployer reports is a root.
	Root *DeliveryRef
	// Depth limits how far below a root the walk goes; 0 is no limit. With
	// Depth 1 a root's own children are listed and its child deployers are
	// not walked.
	Depth int
}

// DeliveryRootError says why the deployer asked for could not be the root.
type DeliveryRootError struct {
	Ref        DeliveryRef
	Candidates []DeliveryRef
}

func (e *DeliveryRootError) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("no deployer %s was found", e.Ref)
	}
	names := make([]string, len(e.Candidates))
	for i, candidate := range e.Candidates {
		names[i] = candidate.String()
	}
	return fmt.Sprintf("%s names more than one deployer (%s); give its namespace", e.Ref, strings.Join(names, ", "))
}

type deliveryKey struct{ group, kind, namespace, name string }

type deliveryIndexed struct {
	listed   listedDeployer
	settings DeliveryDeployerSettings
	reported []DeliveryReported
	status   string
	reason   string
	parents  map[deliveryKey]bool
}

func deliverySourceFor(group, kind string) (deliverySettingsSource, bool) {
	for _, source := range deliverySettingsSources {
		if source.group == group && source.kind == kind {
			return source, true
		}
	}
	return deliverySettingsSource{}, false
}

// reportedByDeployer reads what a deployer object says it applied.
func reportedByDeployer(source deliverySettingsSource, object *unstructured.Unstructured) ([]DeliveryReported, string, string) {
	if source.controller != DeliveryControllerArgoCD || source.kind != "Application" {
		return nil, DeliveryChildrenNotSupported, "what a " + source.kind + " delivers is not read yet (#856)"
	}
	raw, found, err := unstructured.NestedFieldNoCopy(object.Object, "status", "resources")
	if err != nil || !found || raw == nil {
		// No list at all. An Application gets one when Argo CD compares it;
		// one that was never compared, or that nothing reconciles, has none.
		return nil, DeliveryChildrenNoneReported, "the Application has no status.resources"
	}
	entries, ok := raw.([]interface{})
	if !ok {
		return nil, DeliveryChildrenNoneReported, "status.resources is not a list"
	}
	reported := make([]DeliveryReported, 0, len(entries))
	for _, entry := range entries {
		fields, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		text := func(name string) string {
			value, _ := fields[name].(string)
			return strings.TrimSpace(value)
		}
		one := DeliveryReported{
			Group: text("group"), Version: text("version"), Kind: text("kind"),
			Namespace: text("namespace"), Name: text("name"), Sync: text("status"),
		}
		if health, ok := fields["health"].(map[string]interface{}); ok {
			one.Health, _ = health["status"].(string)
		}
		one.RequiresPruning, _ = fields["requiresPruning"].(bool)
		one.Hook, _ = fields["hook"].(bool)
		if one.Kind == "" || one.Name == "" {
			continue
		}
		reported = append(reported, one)
	}
	return reported, DeliveryChildrenReported, ""
}

// deployerState reads what a deployer object says about itself.
func deployerState(source deliverySettingsSource, object *unstructured.Unstructured) *DeliveryDeployerState {
	status, _, _ := unstructured.NestedMap(object.Object, "status")
	state := &DeliveryDeployerState{Reconciled: len(status) > 0}
	if !state.Reconciled {
		return state
	}
	conditions, _, _ := unstructured.NestedSlice(object.Object, "status", "conditions")
	for _, raw := range conditions {
		fields, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		condition := DeliveryCondition{}
		condition.Type, _ = fields["type"].(string)
		condition.Status, _ = fields["status"].(string)
		condition.Message, _ = fields["message"].(string)
		if condition.Type != "" {
			state.Conditions = append(state.Conditions, condition)
		}
	}
	if source.controller == DeliveryControllerArgoCD {
		state.Sync, _, _ = unstructured.NestedString(object.Object, "status", "sync", "status")
		state.Health, _, _ = unstructured.NestedString(object.Object, "status", "health", "status")
		state.Revision, _, _ = unstructured.NestedString(object.Object, "status", "sync", "revision")
		return state
	}
	// Flux says whether it is ready in a condition, and which revision it
	// last applied.
	for _, condition := range state.Conditions {
		if condition.Type == "Ready" {
			state.Health = "Ready=" + condition.Status
		}
	}
	state.Revision, _, _ = unstructured.NestedString(object.Object, "status", "lastAppliedRevision")
	return state
}

// ignoreRulesNaming returns the deployer's ignore rules that name entry: an
// Argo CD ignoreDifferences rule matches on group and kind, and on name and
// namespace when it gives them.
func ignoreRulesNaming(rules []interface{}, entry DeliveryReported) []interface{} {
	var naming []interface{}
	for _, raw := range rules {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		text := func(name string) string {
			value, _ := rule[name].(string)
			return value
		}
		matches := func(pattern, value string) bool { return pattern == "*" || pattern == value }
		// A rule with no group names the core group.
		if !matches(text("group"), entry.Group) || !matches(text("kind"), entry.Kind) {
			continue
		}
		if name := text("name"); name != "" && name != entry.Name {
			continue
		}
		if namespace := text("namespace"); namespace != "" && namespace != entry.Namespace {
			continue
		}
		naming = append(naming, raw)
	}
	return naming
}

func deliveryPolicies(settings DeliveryDeployerSettings) ([]DeliveryPolicyValue, []string) {
	var policies []DeliveryPolicyValue
	var options []string
	for _, setting := range settings.Settings {
		if setting.Category == DeliverySettingPolicy {
			value := setting.Effective
			if value == "" {
				value = setting.Value
			}
			policies = append(policies, DeliveryPolicyValue{Name: setting.Name, Value: value})
			continue
		}
		option := setting.Name
		if setting.Value != "" && setting.Value != DeliveryValueSet {
			option += "=" + setting.Value
		}
		options = append(options, option)
	}
	return policies, options
}

// CollectDeliveryTree lists the deployers and walks from each root through
// what every deployer reports as its own.
func CollectDeliveryTree(ctx context.Context, client dynamic.Interface, opts DeliveryTreeOptions) (DeliveryTree, error) {
	listed, reads := listDeliveryObjects(ctx, client, opts.Namespace)
	tree := DeliveryTree{Roots: []DeliveryTreeNode{}, Reads: reads}
	readStatus := map[string]DeliverySettingsRead{}
	for _, read := range reads {
		readStatus[read.Kind] = read
	}

	index := map[deliveryKey]*deliveryIndexed{}
	var order []deliveryKey
	for _, one := range listed {
		key := deliveryKey{one.source.group, one.source.kind, one.object.GetNamespace(), one.object.GetName()}
		indexed := &deliveryIndexed{listed: one, settings: one.source.parse(one.object), parents: map[deliveryKey]bool{}}
		indexed.reported, indexed.status, indexed.reason = reportedByDeployer(one.source, one.object)
		index[key] = indexed
		order = append(order, key)
	}
	keyLess := func(a, b deliveryKey) bool {
		for _, pair := range [][2]string{{a.group, b.group}, {a.kind, b.kind}, {a.namespace, b.namespace}} {
			if pair[0] != pair[1] {
				return pair[0] < pair[1]
			}
		}
		return a.name < b.name
	}
	sort.Slice(order, func(i, j int) bool { return keyLess(order[i], order[j]) })
	// Who reports whom, among the deployers that were read.
	for _, key := range order {
		for _, entry := range index[key].reported {
			child := deliveryKey{entry.Group, entry.Kind, entry.Namespace, entry.Name}
			if _, isDeployer := deliverySourceFor(entry.Group, entry.Kind); !isDeployer || child == key {
				continue
			}
			if target := index[child]; target != nil {
				target.parents[key] = true
			}
		}
	}

	// A reported child that the list did not return is read on its own when
	// the list was limited to a namespace it is not in.
	lookup := func(source deliverySettingsSource, key deliveryKey) (*deliveryIndexed, string, string) {
		if indexed := index[key]; indexed != nil {
			return indexed, DeliveryObjectFound, ""
		}
		read := readStatus[source.kind]
		switch {
		case read.Status == DeliveryReadNotRead:
			return nil, DeliveryObjectNotRead, read.Reason
		case read.Status != DeliveryReadRead:
			return nil, DeliveryObjectNotFound, "this kind is not installed"
		case opts.Namespace == "" || opts.Namespace == key.namespace:
			return nil, DeliveryObjectNotFound, ""
		case key.namespace == "":
			return nil, DeliveryObjectNotFound, "the parent reports no namespace for it"
		}
		_, version, _ := strings.Cut(read.Resource, "/")
		gvr := schema.GroupVersionResource{Group: source.group, Version: version, Resource: source.resource}
		object, err := client.Resource(gvr).Namespace(key.namespace).Get(ctx, key.name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			return nil, DeliveryObjectNotFound, ""
		case err != nil:
			return nil, DeliveryObjectNotRead, deliveryReadReason(err)
		}
		one := listedDeployer{source: source, object: object}
		indexed := &deliveryIndexed{listed: one, settings: source.parse(object), parents: map[deliveryKey]bool{}}
		indexed.reported, indexed.status, indexed.reason = reportedByDeployer(source, object)
		index[key] = indexed
		return indexed, DeliveryObjectFound, ""
	}

	var build func(source deliverySettingsSource, key deliveryKey, depth int, path map[deliveryKey]bool) DeliveryTreeNode
	build = func(source deliverySettingsSource, key deliveryKey, depth int, path map[deliveryKey]bool) DeliveryTreeNode {
		node := DeliveryTreeNode{Controller: source.controller, Kind: key.kind, Namespace: key.namespace, Name: key.name}
		tree.Summary.Deployers++
		if depth > tree.Summary.MaxDepth {
			tree.Summary.MaxDepth = depth
		}
		indexed, object, reason := lookup(source, key)
		node.Object, node.ObjectReason = object, reason
		if indexed == nil {
			node.Children = DeliveryChildren{Status: DeliveryChildrenNoObject}
			if object == DeliveryObjectNotRead {
				tree.Summary.NotRead++
			} else {
				tree.Summary.NotFound++
			}
			return node
		}
		node.GeneratedBy, node.URL = indexed.settings.GeneratedBy, indexed.settings.URL
		node.Policies, node.Options = deliveryPolicies(indexed.settings)
		node.State = deployerState(source, indexed.listed.object)
		if !node.State.Reconciled {
			tree.Summary.NotReconciled++
		}
		if len(indexed.parents) > 1 {
			node.ReportedBy = len(indexed.parents)
			tree.Summary.Shared++
		}

		node.Children = DeliveryChildren{Status: indexed.status, Reason: indexed.reason}
		switch indexed.status {
		case DeliveryChildrenNoneReported:
			tree.Summary.NoneReported++
			return node
		case DeliveryChildrenNotSupported:
			tree.Summary.NotSupported++
			return node
		}
		if path[key] {
			node.Children = DeliveryChildren{Status: DeliveryChildrenCycle}
			tree.Summary.Cycles++
			return node
		}
		if opts.Depth > 0 && depth >= opts.Depth {
			node.Children = DeliveryChildren{Status: DeliveryChildrenDepthLimit}
			tree.Summary.DepthLimited++
			return node
		}
		path[key] = true
		defer delete(path, key)
		for _, entry := range indexed.reported {
			entry.IgnoredByParent = ignoreRulesNaming(indexed.settings.IgnoreRules, entry)
			childSource, isDeployer := deliverySourceFor(entry.Group, entry.Kind)
			if !isDeployer {
				node.Children.Resources = append(node.Children.Resources, entry)
				tree.Summary.Resources++
				continue
			}
			childKey := deliveryKey{entry.Group, entry.Kind, entry.Namespace, entry.Name}
			reportedEntry := entry
			child := build(childSource, childKey, depth+1, path)
			child.ReportedByParent = &reportedEntry
			node.Children.Deployers = append(node.Children.Deployers, child)
		}
		sort.SliceStable(node.Children.Deployers, func(i, j int) bool {
			a, b := node.Children.Deployers[i], node.Children.Deployers[j]
			for _, pair := range [][2]string{{a.Kind, b.Kind}, {a.Namespace, b.Namespace}} {
				if pair[0] != pair[1] {
					return pair[0] < pair[1]
				}
			}
			return a.Name < b.Name
		})
		sort.SliceStable(node.Children.Resources, func(i, j int) bool {
			a, b := node.Children.Resources[i], node.Children.Resources[j]
			for _, pair := range [][2]string{{a.Kind, b.Kind}, {a.Group, b.Group}, {a.Namespace, b.Namespace}} {
				if pair[0] != pair[1] {
					return pair[0] < pair[1]
				}
			}
			return a.Name < b.Name
		})
		return node
	}

	var roots []deliveryKey
	if opts.Root != nil {
		var candidates []deliveryKey
		for _, key := range order {
			if key.kind == opts.Root.Kind && key.name == opts.Root.Name && (opts.Root.Namespace == "" || opts.Root.Namespace == key.namespace) {
				candidates = append(candidates, key)
			}
		}
		if len(candidates) != 1 {
			failure := &DeliveryRootError{Ref: *opts.Root}
			for _, candidate := range candidates {
				failure.Candidates = append(failure.Candidates, DeliveryRef{Kind: candidate.kind, Namespace: candidate.namespace, Name: candidate.name})
			}
			return tree, failure
		}
		roots = candidates
	} else {
		for _, key := range order {
			if len(index[key].parents) == 0 {
				roots = append(roots, key)
			}
		}
		// Deployers that only report each other have no root among them.
		// Each such ring is entered once, at its first member, so that
		// nothing read is left out of the tree.
		reached := map[deliveryKey]bool{}
		var mark func(key deliveryKey)
		mark = func(key deliveryKey) {
			if reached[key] || index[key] == nil {
				return
			}
			reached[key] = true
			for _, entry := range index[key].reported {
				if _, isDeployer := deliverySourceFor(entry.Group, entry.Kind); isDeployer {
					mark(deliveryKey{entry.Group, entry.Kind, entry.Namespace, entry.Name})
				}
			}
		}
		for _, key := range roots {
			mark(key)
		}
		for _, key := range order {
			if !reached[key] {
				roots = append(roots, key)
				mark(key)
			}
		}
		sort.Slice(roots, func(i, j int) bool { return keyLess(roots[i], roots[j]) })
	}
	for _, key := range roots {
		source, _ := deliverySourceFor(key.group, key.kind)
		tree.Roots = append(tree.Roots, build(source, key, 0, map[deliveryKey]bool{}))
	}
	tree.Summary.Roots = len(tree.Roots)
	return tree, nil
}
