// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package mapsvc provides JSON output types for v0.14 schema.
//
// JSON is the canonical truth; ASCII/Markdown are projections.
// See docs/v0.14-json-schema.md for the full specification.
package mapsvc

// ResourceID is the canonical identity for cross-schema joins.
type ResourceID struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// DisplayMeta indicates what ASCII rendering would show.
// JSON always contains ALL items; this is metadata for parity.
type DisplayMeta struct {
	MaxItems   int `json:"maxItems"`
	TotalItems int `json:"totalItems"`
}

// OwnershipTreeOutput is the v0.14 JSON schema for `tree ownership`.
type OwnershipTreeOutput struct {
	Command string                 `json:"command"`
	Subtype string                 `json:"subtype"`
	Context OwnershipTreeContext   `json:"context"`
	Groups  []OwnershipTreeGroup   `json:"groups"`
	Summary OwnershipTreeSummary   `json:"summary"`
}

// OwnershipTreeContext captures query parameters.
type OwnershipTreeContext struct {
	Cluster   string                    `json:"cluster"`
	Namespace *string                   `json:"namespace"` // null = all namespaces
	Filters   OwnershipTreeFilters      `json:"filters"`
}

// OwnershipTreeFilters captures active filters.
type OwnershipTreeFilters struct {
	Owner    *string `json:"owner"`    // null = no owner filter
	MaxItems int     `json:"maxItems"` // 0 = no limit (for ASCII rendering)
}

// OwnershipTreeGroup represents resources under one owner.
type OwnershipTreeGroup struct {
	Owner       string                   `json:"owner"`
	DisplayMeta DisplayMeta              `json:"displayMeta"`
	Items       []OwnershipTreeItem      `json:"items"`
}

// OwnershipTreeItem represents a single resource in the tree.
type OwnershipTreeItem struct {
	ID       ResourceID  `json:"id"`
	OwnerRef *ResourceID `json:"ownerRef"` // null for Native
}

// OwnershipTreeSummary provides aggregate counts.
type OwnershipTreeSummary struct {
	TotalResources int            `json:"totalResources"`
	ByOwner        map[string]int `json:"byOwner"`
}

// CanonicalOwnerOrder defines the deterministic owner ordering.
// This order is used everywhere: JSON groups, summary.byOwner iteration.
// MUST match the ASCII canonical model in render.go (BuildOwnershipTreeLines).
var CanonicalOwnerOrder = []string{"Flux", "ArgoCD", "Helm", "ConfigHub", "Native"}

// BuildOwnershipTreeJSON builds v0.14 JSON output from entries.
// It uses the same canonical model as ASCII (BuildOwnershipTreeLines).
func BuildOwnershipTreeJSON(entries []Entry, opts OwnershipTreeOpts, cluster string, namespace *string) OwnershipTreeOutput {
	// Apply owner filter if specified
	filtered := entries
	if opts.FilterOwner != "" {
		filtered = make([]Entry, 0)
		for _, e := range entries {
			if normalizeOwner(e.Owner) == normalizeOwner(opts.FilterOwner) {
				filtered = append(filtered, e)
			}
		}
	}

	// Group by owner (same logic as BuildOwnershipTreeLines)
	byOwner := make(map[string][]Entry)
	for _, e := range filtered {
		byOwner[e.Owner] = append(byOwner[e.Owner], e)
	}

	// Build groups in canonical owner order
	var groups []OwnershipTreeGroup
	byOwnerCount := make(map[string]int)

	for _, owner := range CanonicalOwnerOrder {
		ownerEntries := byOwner[owner]
		if len(ownerEntries) == 0 {
			continue
		}

		// Sort by (namespace, kind, name) for determinism
		sortEntriesDeterministic(ownerEntries)

		// Build items (ALL items - JSON is lossless)
		items := make([]OwnershipTreeItem, len(ownerEntries))
		for i, e := range ownerEntries {
			item := OwnershipTreeItem{
				ID: ResourceID{
					Kind:      e.Kind,
					Namespace: e.Namespace,
					Name:      e.Name,
				},
			}

			// Parse ownerRef from OwnerDetails if available
			if e.OwnerDetails != nil && e.OwnerDetails["name"] != "" {
				// OwnerDetails["name"] is like "kustomization/web-apps" or "release/redis"
				item.OwnerRef = parseOwnerRef(e.OwnerDetails, e.Namespace)
			}

			items[i] = item
		}

		// DisplayMeta indicates what ASCII would show
		displayMax := len(ownerEntries)
		if opts.MaxDepth > 0 && opts.MaxDepth < displayMax {
			displayMax = opts.MaxDepth
		}

		groups = append(groups, OwnershipTreeGroup{
			Owner: owner,
			DisplayMeta: DisplayMeta{
				MaxItems:   displayMax,
				TotalItems: len(ownerEntries),
			},
			Items: items,
		})

		byOwnerCount[owner] = len(ownerEntries)
	}

	// Build context
	var ownerFilter *string
	if opts.FilterOwner != "" {
		ownerFilter = &opts.FilterOwner
	}

	context := OwnershipTreeContext{
		Cluster:   cluster,
		Namespace: namespace,
		Filters: OwnershipTreeFilters{
			Owner:    ownerFilter,
			MaxItems: opts.MaxDepth,
		},
	}

	// Build summary
	total := 0
	for _, count := range byOwnerCount {
		total += count
	}

	return OwnershipTreeOutput{
		Command: "tree",
		Subtype: "ownership",
		Context: context,
		Groups:  groups,
		Summary: OwnershipTreeSummary{
			TotalResources: total,
			ByOwner:        byOwnerCount,
		},
	}
}

// normalizeOwner normalizes owner names for comparison.
// MUST match the canonical owner categories in CanonicalOwnerOrder.
func normalizeOwner(owner string) string {
	// Case-insensitive comparison
	switch {
	case owner == "Flux" || owner == "flux":
		return "Flux"
	case owner == "ArgoCD" || owner == "argocd" || owner == "Argocd":
		return "ArgoCD"
	case owner == "Helm" || owner == "helm":
		return "Helm"
	case owner == "ConfigHub" || owner == "confighub":
		return "ConfigHub"
	default:
		// All other owners (including Crossplane) fall to Native until
		// they become first-class categories in the ASCII canonical model.
		return "Native"
	}
}

// sortEntriesDeterministic sorts entries by (namespace, kind, name).
func sortEntriesDeterministic(entries []Entry) {
	// Using sort.SliceStable to maintain consistent ordering
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if compareEntries(entries[i], entries[j]) > 0 {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// compareEntries returns -1, 0, or 1 for deterministic sorting.
func compareEntries(a, b Entry) int {
	if a.Namespace < b.Namespace {
		return -1
	}
	if a.Namespace > b.Namespace {
		return 1
	}
	if a.Kind < b.Kind {
		return -1
	}
	if a.Kind > b.Kind {
		return 1
	}
	if a.Name < b.Name {
		return -1
	}
	if a.Name > b.Name {
		return 1
	}
	return 0
}

// parseOwnerRef parses OwnerDetails into a ResourceID.
func parseOwnerRef(details map[string]string, defaultNamespace string) *ResourceID {
	if details == nil {
		return nil
	}

	name := details["name"]
	if name == "" {
		return nil
	}

	// Parse "kind/name" format (e.g., "kustomization/web-apps")
	parts := splitOwnerRef(name)
	if len(parts) != 2 {
		return nil
	}

	// Determine namespace from details or use default
	ns := details["namespace"]
	if ns == "" {
		ns = defaultNamespace
	}

	return &ResourceID{
		Kind:      normalizeKind(parts[0]),
		Namespace: ns,
		Name:      parts[1],
	}
}

// splitOwnerRef splits "kind/name" into parts.
func splitOwnerRef(ref string) []string {
	idx := -1
	for i, c := range ref {
		if c == '/' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return []string{ref}
	}
	return []string{ref[:idx], ref[idx+1:]}
}

// normalizeKind normalizes kind names to proper case.
func normalizeKind(kind string) string {
	switch kind {
	case "kustomization", "Kustomization":
		return "Kustomization"
	case "helmrelease", "HelmRelease":
		return "HelmRelease"
	case "application", "Application":
		return "Application"
	case "release", "Release":
		return "HelmRelease" // Helm release
	default:
		return kind
	}
}

// =============================================================================
// Trace Schema (v0.14)
// =============================================================================

// TraceOutput is the v0.14 JSON schema for `trace <resource>`.
type TraceOutput struct {
	Command string       `json:"command"`
	Target  ResourceID   `json:"target"`
	Chain   []ChainNode  `json:"chain"`
	Summary TraceSummary `json:"summary"`
}

// ChainNode represents a single node in the trace chain.
type ChainNode struct {
	ID           ResourceID `json:"id"`
	Role         string     `json:"role"`         // source, deployer, workload, intermediate
	Relationship string     `json:"relationship"` // provides-manifests-to, applies, manages, managed-by
	Evidence     []Evidence `json:"evidence"`
}

// Evidence represents structured proof of a relationship.
type Evidence struct {
	Type  string `json:"type"`  // label, annotation, ownerRef, field, inventory
	Key   string `json:"key"`   // the label/annotation/field key
	Value string `json:"value"` // the actual value
	Path  string `json:"path"`  // JSON path in the resource (e.g., metadata.labels)
}

// TraceSummary provides a quick overview of the trace.
type TraceSummary struct {
	OwnerType string          `json:"ownerType"` // Flux, ArgoCD, Helm, Native
	Source    *TraceSourceRef `json:"source"`    // null if not found
	Deployer  *ResourceID     `json:"deployer"`  // null if not found
}

// TraceSourceRef includes source resource info plus URL.
type TraceSourceRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	URL       string `json:"url,omitempty"` // Git URL for GitRepository, registry for OCI
}

// ChainRole constants for trace chain nodes.
const (
	RoleSource       = "source"
	RoleDeployer     = "deployer"
	RoleWorkload     = "workload"
	RoleIntermediate = "intermediate"
)

// Relationship constants for trace chain edges.
// Canonical vocabulary aligned with ASCII trace verbs.
const (
	RelSources   = "sources"    // Source provides manifests to deployer
	RelApplies   = "applies"    // Deployer applies manifests to cluster
	RelGenerates = "generates"  // Generator creates downstream resources
	RelSyncs     = "syncs"      // Controller syncs resources
	RelManages   = "manages"    // Controller manages child resource
	RelManagedBy = "managed-by" // Workload back-reference to deployer
	RelSelects   = "selects"    // Service selects pods
)

// EvidenceType constants for evidence entries.
const (
	EvidenceLabel      = "label"
	EvidenceAnnotation = "annotation"
	EvidenceOwnerRef   = "ownerRef"
	EvidenceField      = "field"
	EvidenceInventory  = "inventory"
)

// InferRole determines the role of a chain node based on its Kind.
func InferRole(kind string) string {
	switch kind {
	case "GitRepository", "OCIRepository", "HelmRepository", "Bucket":
		return RoleSource
	case "Kustomization", "HelmRelease", "Application":
		return RoleDeployer
	case "ReplicaSet", "Pod":
		return RoleIntermediate
	default:
		return RoleWorkload
	}
}

// InferRelationship determines the relationship for a node based on its role.
// The relationship describes what this node does (or its characteristic).
func InferRelationship(fromKind, toKind string) string {
	fromRole := InferRole(fromKind)
	toRole := InferRole(toKind)

	switch {
	case fromRole == RoleSource && toRole == RoleDeployer:
		return RelSources
	case fromRole == RoleDeployer && toRole == RoleWorkload:
		return RelApplies
	case fromRole == RoleDeployer && toRole == RoleIntermediate:
		return RelManages
	case fromRole == RoleIntermediate && toRole == RoleIntermediate:
		return RelManages
	case fromRole == RoleWorkload:
		return RelManagedBy
	default:
		return RelManages
	}
}

// RelationshipForRole returns the canonical relationship for a given role.
// This is used when we know the role but not the adjacent nodes.
func RelationshipForRole(role string) string {
	switch role {
	case RoleSource:
		return RelSources
	case RoleDeployer:
		return RelApplies
	case RoleWorkload:
		return RelManagedBy
	case RoleIntermediate:
		return RelManages
	default:
		return RelManages
	}
}
