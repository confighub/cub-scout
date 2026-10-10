// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gobwas/glob"
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
	// DeliveryChildrenShownAbove: this deployer is reported by more than one
	// deployer and what is under it is already in the tree, at its first
	// appearance. It is not walked again.
	DeliveryChildrenShownAbove = "shown_above"

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
	// this entry, verbatim. An Argo CD rule says which differences do not
	// make the entry OutOfSync; it does not by itself stop a sync from
	// overwriting them.
	IgnoredByParent []interface{} `json:"ignoredByParent,omitempty"`
	// IgnoreRespectedOnSync is true when the reporting deployer also sets
	// RespectIgnoreDifferences=true, so that a sync leaves those fields alone.
	IgnoreRespectedOnSync bool `json:"ignoreRespectedOnSync,omitempty"`
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
	// Malformed counts reported entries that could not be read as an entry:
	// not an object, or with no kind or name, or a field of the wrong type.
	// They are in neither list; the count says the lists are short.
	Malformed int `json:"malformed,omitempty"`
	// HiddenByFilter counts the entries directly under this deployer that a
	// filter left out. It is set only on a filtered tree.
	HiddenByFilter int `json:"hiddenByFilter,omitempty"`
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
	// ReportedBy is how many of the deployers that were read report this
	// one, when more than one. A parent outside the namespace listed is not
	// counted, because it was not read.
	ReportedBy int `json:"reportedBy,omitempty"`
	// GeneratedBy names the ApplicationSet that owns an Application.
	GeneratedBy string                 `json:"generatedBy,omitempty"`
	Policies    []DeliveryPolicyValue  `json:"policies,omitempty"`
	Options     []string               `json:"options,omitempty"`
	State       *DeliveryDeployerState `json:"state,omitempty"`
	Children    DeliveryChildren       `json:"children"`
}

// DeliveryTreeSummary counts what the tree holds. A deployer reported by two
// parents is counted once for each place it appears; what is under it is
// walked, and counted, once.
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
	Malformed     int `json:"malformed"`
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

func deliveryKeyLess(a, b deliveryKey) bool {
	for _, pair := range [][2]string{{a.group, b.group}, {a.kind, b.kind}, {a.namespace, b.namespace}} {
		if pair[0] != pair[1] {
			return pair[0] < pair[1]
		}
	}
	return a.name < b.name
}

type deliveryIndexed struct {
	listed    listedDeployer
	settings  DeliveryDeployerSettings
	reported  []DeliveryReported
	malformed int
	status    string
	reason    string
}

func deliverySourceFor(group, kind string) (deliverySettingsSource, bool) {
	for _, source := range deliverySettingsSources {
		if source.group == group && source.kind == kind {
			return source, true
		}
	}
	return deliverySettingsSource{}, false
}

// reportedByDeployer reads what a deployer object says it applied. malformed
// counts the entries of the list that are not entries: they are left out, and
// the count is how the caller knows.
func reportedByDeployer(source deliverySettingsSource, object *unstructured.Unstructured) (reported []DeliveryReported, malformed int, status, reason string) {
	if source.controller != DeliveryControllerArgoCD || source.kind != "Application" {
		return nil, 0, DeliveryChildrenNotSupported, "what a " + source.kind + " delivers is not read yet (#856)"
	}
	raw, found, err := unstructured.NestedFieldNoCopy(object.Object, "status", "resources")
	if err != nil || !found || raw == nil {
		// No list at all. An Application gets one when Argo CD compares it;
		// one that could not be compared, or that nothing reconciles, has
		// none.
		return nil, 0, DeliveryChildrenNoneReported, "the Application has no status.resources"
	}
	entries, ok := raw.([]interface{})
	if !ok {
		return nil, 0, DeliveryChildrenNoneReported, "status.resources is not a list"
	}
	reported = make([]DeliveryReported, 0, len(entries))
	for _, entry := range entries {
		fields, ok := entry.(map[string]interface{})
		if !ok {
			malformed++
			continue
		}
		// A field that is present and not a string is not coerced: a group
		// read as "" would turn an Application into a plain resource.
		wellFormed := true
		text := func(name string) string {
			value, present := fields[name]
			if !present || value == nil {
				return ""
			}
			typed, isString := value.(string)
			if !isString {
				wellFormed = false
			}
			return strings.TrimSpace(typed)
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
		if !wellFormed || one.Kind == "" || one.Name == "" {
			malformed++
			continue
		}
		reported = append(reported, one)
	}
	return reported, malformed, DeliveryChildrenReported, ""
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

// ignoreRulesNaming returns the deployer's ignore rules that name entry, as
// Argo CD matches an ignoreDifferences rule to a resource (v3.5.3,
// util/argo/normalizers): group and kind are glob patterns, and name and
// namespace are exact, each matching anything when the rule leaves it out. A
// rule with no group names the core group. A pattern that does not compile
// matches nothing.
//
// Only the Application's own spec.ignoreDifferences is read. Rules set for
// the whole Argo CD instance in argocd-cm are not.
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
		matches := func(pattern, value string) bool {
			compiled, err := glob.Compile(pattern)
			return err == nil && compiled.Match(value)
		}
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

// respectsIgnoreOnSync reports whether a deployer's options say that a sync
// leaves ignored differences alone.
func respectsIgnoreOnSync(settings DeliveryDeployerSettings) bool {
	for _, setting := range settings.Settings {
		if setting.Name == "RespectIgnoreDifferences" && setting.Value == "true" {
			return true
		}
	}
	return false
}

// deliveryGetReason says why one object could not be read.
func deliveryGetReason(err error) string {
	if reason := deliveryReadReason(err); reason != "list_failed" {
		return reason
	}
	return "get_failed"
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
//
// A root is a deployer that no deployer read reports. Deployers that only
// report each other have no such member; each such ring that nothing outside
// it reports is entered once, at its first member. With a Namespace, "read"
// means listed in that namespace: a deployer whose only parents are elsewhere
// is a root of what was listed.
//
// A deployer that more than one deployer reports appears under each, and what
// is under it is walked once, at its first appearance. That keeps the tree
// the size of what was read, however the deployers share each other.
func CollectDeliveryTree(ctx context.Context, client dynamic.Interface, opts DeliveryTreeOptions) (DeliveryTree, error) {
	listed, reads := listDeliveryObjects(ctx, client, opts.Namespace)
	tree := DeliveryTree{Roots: []DeliveryTreeNode{}, Reads: reads}
	readStatus := map[string]DeliverySettingsRead{}
	for _, read := range reads {
		readStatus[read.Kind] = read
	}

	index := map[deliveryKey]*deliveryIndexed{}
	// parentsOf is who reports whom, among the deployers that were read. A
	// child is recorded whether or not it was read itself.
	parentsOf := map[deliveryKey]map[deliveryKey]bool{}
	childKeys := func(indexed *deliveryIndexed) []deliveryKey {
		var keys []deliveryKey
		for _, entry := range indexed.reported {
			if _, isDeployer := deliverySourceFor(entry.Group, entry.Kind); isDeployer {
				keys = append(keys, deliveryKey{entry.Group, entry.Kind, entry.Namespace, entry.Name})
			}
		}
		return keys
	}
	add := func(one listedDeployer) deliveryKey {
		key := deliveryKey{one.source.group, one.source.kind, one.object.GetNamespace(), one.object.GetName()}
		indexed := &deliveryIndexed{listed: one, settings: one.source.parse(one.object)}
		indexed.reported, indexed.malformed, indexed.status, indexed.reason = reportedByDeployer(one.source, one.object)
		// Entries are walked in a fixed order, so that which appearance of a
		// shared deployer comes first does not depend on the order its
		// parent happened to list it in.
		sort.SliceStable(indexed.reported, func(i, j int) bool {
			a, b := indexed.reported[i], indexed.reported[j]
			return deliveryKeyLess(deliveryKey{a.Group, a.Kind, a.Namespace, a.Name}, deliveryKey{b.Group, b.Kind, b.Namespace, b.Name})
		})
		index[key] = indexed
		for _, child := range childKeys(indexed) {
			// Reporting itself does not make a deployer its own parent.
			if child == key {
				continue
			}
			if parentsOf[child] == nil {
				parentsOf[child] = map[deliveryKey]bool{}
			}
			parentsOf[child][key] = true
		}
		return key
	}
	var order []deliveryKey
	for _, one := range listed {
		order = append(order, add(one))
	}
	sort.Slice(order, func(i, j int) bool { return deliveryKeyLess(order[i], order[j]) })

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
			return nil, DeliveryObjectNotRead, deliveryGetReason(err)
		}
		add(listedDeployer{source: source, object: object})
		return index[key], DeliveryObjectFound, ""
	}

	walked := map[deliveryKey]bool{}
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
		node.GeneratedBy = indexed.settings.GeneratedBy
		node.Policies, node.Options = deliveryPolicies(indexed.settings)
		node.State = deployerState(source, indexed.listed.object)
		if !node.State.Reconciled {
			tree.Summary.NotReconciled++
		}
		if parents := len(parentsOf[key]); parents > 1 {
			node.ReportedBy = parents
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
		switch {
		case path[key]:
			node.Children = DeliveryChildren{Status: DeliveryChildrenCycle}
			tree.Summary.Cycles++
			return node
		case walked[key]:
			node.Children = DeliveryChildren{Status: DeliveryChildrenShownAbove}
			return node
		case opts.Depth > 0 && depth >= opts.Depth:
			node.Children = DeliveryChildren{Status: DeliveryChildrenDepthLimit}
			tree.Summary.DepthLimited++
			return node
		}
		walked[key] = true
		path[key] = true
		defer delete(path, key)
		node.Children.Malformed = indexed.malformed
		tree.Summary.Malformed += indexed.malformed
		respected := respectsIgnoreOnSync(indexed.settings)
		for _, entry := range indexed.reported {
			if entry.IgnoredByParent = ignoreRulesNaming(indexed.settings.IgnoreRules, entry); len(entry.IgnoredByParent) > 0 {
				entry.IgnoreRespectedOnSync = respected
			}
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
		roots = deliveryRoots(order, func(key deliveryKey) []deliveryKey {
			var listedChildren []deliveryKey
			for _, child := range childKeys(index[key]) {
				if index[child] != nil && child != key {
					listedChildren = append(listedChildren, child)
				}
			}
			return listedChildren
		})
	}
	for _, key := range roots {
		source, _ := deliverySourceFor(key.group, key.kind)
		tree.Roots = append(tree.Roots, build(source, key, 0, map[deliveryKey]bool{}))
	}
	tree.Summary.Roots = len(tree.Roots)
	return tree, nil
}

// deliveryRoots returns where the walk starts when no root is named: one
// member of each group of deployers that nothing outside the group reports.
//
// Such a group is usually one deployer with no parent. It can also be a ring,
// deployers that report each other, and then it is entered at its first
// member in order. A deployer that anything outside its own ring reports is
// never a root, because the walk reaches it from there. order must be sorted;
// children gives the deployers a deployer reports, among those in order.
func deliveryRoots(order []deliveryKey, children func(deliveryKey) []deliveryKey) []deliveryKey {
	// Tarjan's strongly connected components.
	position := map[deliveryKey]int{}
	for i, key := range order {
		position[key] = i
	}
	const unvisited = -1
	visit, low, component := make([]int, len(order)), make([]int, len(order)), make([]int, len(order))
	onStack := make([]bool, len(order))
	for i := range order {
		visit[i], component[i] = unvisited, unvisited
	}
	var stack []int
	next, components := 0, 0
	var connect func(v int)
	connect = func(v int) {
		visit[v], low[v] = next, next
		next++
		stack, onStack[v] = append(stack, v), true
		for _, child := range children(order[v]) {
			w := position[child]
			switch {
			case visit[w] == unvisited:
				connect(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			case onStack[w] && visit[w] < low[v]:
				low[v] = visit[w]
			}
		}
		if low[v] != visit[v] {
			return
		}
		for {
			w := stack[len(stack)-1]
			stack, onStack[w] = stack[:len(stack)-1], false
			component[w] = components
			if w == v {
				break
			}
		}
		components++
	}
	for v := range order {
		if visit[v] == unvisited {
			connect(v)
		}
	}
	// A component that an edge from another component enters is not a source.
	entered := make([]bool, components)
	for v, key := range order {
		for _, child := range children(key) {
			if w := position[child]; component[w] != component[v] {
				entered[component[w]] = true
			}
		}
	}
	var roots []deliveryKey
	taken := make([]bool, components)
	for v, key := range order {
		if c := component[v]; !entered[c] && !taken[c] {
			taken[c] = true
			roots = append(roots, key)
		}
	}
	return roots
}
