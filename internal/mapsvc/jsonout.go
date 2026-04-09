// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package mapsvc provides JSON output types for v0.14 schema.
//
// JSON is the canonical truth; ASCII/Markdown are projections.
// See docs/v0.14-json-schema.md for the full specification.
package mapsvc

import "strings"

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
	Command string               `json:"command"`
	Subtype string               `json:"subtype"`
	Context OwnershipTreeContext `json:"context"`
	Groups  []OwnershipTreeGroup `json:"groups"`
	Summary OwnershipTreeSummary `json:"summary"`
}

// OwnershipTreeContext captures query parameters.
type OwnershipTreeContext struct {
	Cluster   string               `json:"cluster"`
	Namespace *string              `json:"namespace"` // null = all namespaces
	Filters   OwnershipTreeFilters `json:"filters"`
}

// OwnershipTreeFilters captures active filters.
type OwnershipTreeFilters struct {
	Owner    *string `json:"owner"`    // null = no owner filter
	MaxItems int     `json:"maxItems"` // 0 = no limit (for ASCII rendering)
}

// OwnershipTreeGroup represents resources under one owner.
type OwnershipTreeGroup struct {
	Owner       string              `json:"owner"`
	DisplayMeta DisplayMeta         `json:"displayMeta"`
	Items       []OwnershipTreeItem `json:"items"`
}

// OwnershipTreeItem represents a single resource in the tree.
type OwnershipTreeItem struct {
	ID       ResourceID   `json:"id"`
	OwnerRef *ResourceID  `json:"ownerRef"` // null for Native
	Lineage  []ResourceID `json:"lineage,omitempty"`
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

			if e.OwnerDetails != nil {
				// OwnerDetails["name"] is like "kustomization/web-apps" or "release/redis"
				if e.OwnerDetails["name"] != "" {
					item.OwnerRef = parseOwnerRef(e.OwnerDetails, e.Namespace)
				}
				if lineage := parseLineageRefs(e.OwnerDetails, e.Namespace); len(lineage) > 0 {
					item.Lineage = lineage
				}
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

func parseLineageRefs(details map[string]string, defaultNamespace string) []ResourceID {
	if details == nil {
		return nil
	}
	lineage := strings.TrimSpace(details["lineage"])
	if lineage == "" {
		return nil
	}

	segments := strings.Split(lineage, "<-")
	refs := make([]ResourceID, 0, len(segments))
	for _, segment := range segments {
		name := strings.TrimSpace(segment)
		if name == "" {
			continue
		}

		ref := parseOwnerRef(map[string]string{
			"name":      name,
			"namespace": details["namespace"],
		}, defaultNamespace)
		if ref != nil {
			refs = append(refs, *ref)
		}
	}

	if len(refs) == 0 {
		return nil
	}
	return refs
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
	case "applicationset", "ApplicationSet":
		return "ApplicationSet"
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
	Command string        `json:"command"`
	Target  ResourceID    `json:"target"`
	Chain   []ChainNode   `json:"chain"`
	Summary TraceSummary  `json:"summary"`
	Secrets *TraceSecrets `json:"secrets,omitempty"` // Secret evidence (v0.14.1+)
	Events  *TraceEvents  `json:"events,omitempty"`  // Recent K8s events (v1.10+)
}

// ChainNode represents a single node in the trace chain.
type ChainNode struct {
	ID             ResourceID `json:"id"`
	Role           string     `json:"role"`         // source, deployer, workload, intermediate
	Relationship   string     `json:"relationship"` // provides-manifests-to, applies, manages, managed-by
	Evidence       []Evidence `json:"evidence"`
	DeliveryStage  string     `json:"deliveryStage,omitempty"`  // source, render, artifact, deployer, workload (v1.1+)
	RenderedFrom   string     `json:"renderedFrom,omitempty"`   // provenance: what rendered this resource's manifests (v1.1+)
	OriginalSource string     `json:"originalSource,omitempty"` // provenance: original source-of-truth (v1.1+)
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
	Kind      string            `json:"kind"`
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	URL       string            `json:"url,omitempty"` // Git URL for GitRepository, registry for OCI
	Artifact  *TraceArtifactRef `json:"artifact,omitempty"`
}

// TraceArtifactRef captures source artifact provenance when requested (`trace --artifacts`).
type TraceArtifactRef struct {
	URL            string `json:"url"`
	Revision       string `json:"revision"`
	Digest         string `json:"digest"`
	LastUpdateTime string `json:"lastUpdateTime"`
	SourceKind     string `json:"sourceKind"`
}

// TraceSecrets represents secret evidence in the v0.14 trace schema.
// Only safe metadata is included; secret data is never exposed.
type TraceSecrets struct {
	Secrets []TraceSecretEvidence `json:"secrets"`
	Summary TraceSecretSummary    `json:"summary"`
}

// TraceSecretEvidence represents a single secret reference and its resolution status.
type TraceSecretEvidence struct {
	Name         string              `json:"name"`
	Namespace    string              `json:"namespace"`
	RefType      string              `json:"refType"`               // e.g., "envFrom.secretRef", "volume.secret"
	RefPath      string              `json:"refPath,omitempty"`     // e.g., "containers[app].envFrom[0].secretRef"
	Status       string              `json:"status"`                // "present", "missing", "unreadable", "unresolved"
	StatusReason string              `json:"statusReason,omitempty"`
	SecretType   string              `json:"secretType,omitempty"`   // e.g., "Opaque", "kubernetes.io/tls" (when present)
	CreatedAt    string              `json:"createdAt,omitempty"`    // RFC3339 timestamp (when present)
	Owner        *TraceSecretOwner   `json:"owner,omitempty"`        // Detected ownership (when present)
	Optional     bool                `json:"optional,omitempty"`
}

// TraceSecretOwner represents detected ownership of a secret.
type TraceSecretOwner struct {
	Type      string `json:"type"`                // e.g., "flux", "argocd", "helm", "confighub"
	SubType   string `json:"subType,omitempty"`   // e.g., "kustomization", "helmrelease"
	Name      string `json:"name,omitempty"`      // Owner resource name
	Namespace string `json:"namespace,omitempty"` // Owner resource namespace
}

// TraceSecretSummary provides counts by status.
type TraceSecretSummary struct {
	Total      int `json:"total"`
	Present    int `json:"present"`
	Missing    int `json:"missing"`
	Unreadable int `json:"unreadable"`
	Unresolved int `json:"unresolved"`
}

// TraceEvents represents recent Kubernetes events in the v1.10 trace schema.
// Events are bounded (top 5) and prioritized by severity (warnings/errors first).
type TraceEvents struct {
	Events       []TraceEvent `json:"events"`
	TotalCount   int          `json:"totalCount"`   // Total events found
	WarningCount int          `json:"warningCount"` // Number of warning events
	ErrorCount   int          `json:"errorCount"`   // Number of error-severity events
}

// TraceEvent represents a single Kubernetes event.
type TraceEvent struct {
	Type      string `json:"type"`              // Normal or Warning
	Reason    string `json:"reason"`            // e.g., Pulled, BackOff, FailedScheduling
	Message   string `json:"message"`           // Human-readable message
	Count     int32  `json:"count,omitempty"`   // Number of occurrences
	Age       string `json:"age"`               // Human-readable age (e.g., "5m", "2h")
	Severity  string `json:"severity"`          // info, warning, error
	Source    string `json:"source,omitempty"`  // Component that generated the event
	FirstSeen string `json:"firstSeen,omitempty"` // RFC3339 timestamp
	LastSeen  string `json:"lastSeen,omitempty"`  // RFC3339 timestamp
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
	case "GitRepository", "OCIRepository", "ConfigHub OCI", "HelmRepository", "Bucket":
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

// Delivery stage constants for trace chain nodes (v1.1+).
// These classify where a resource sits in the delivery pipeline:
//
//	source → render → artifact → deployer → workload
const (
	StageSource   = "source"
	StageRender   = "render"
	StageArtifact = "artifact"
	StageDeployer = "deployer"
	StageWorkload = "workload"
)

// InferDeliveryStage determines the delivery pipeline stage from a resource Kind.
// Returns "" for kinds that don't map to a known stage.
func InferDeliveryStage(kind string) string {
	switch kind {
	case "GitRepository", "HelmRepository", "Bucket":
		return StageSource
	case "OCIRepository", "ConfigHub OCI":
		return StageArtifact
	case "Kustomization", "HelmRelease", "Application":
		return StageDeployer
	case "Deployment", "StatefulSet", "DaemonSet", "Service", "ConfigMap", "Secret", "Ingress",
		"ReplicaSet", "Pod", "Job", "CronJob":
		return StageWorkload
	default:
		return ""
	}
}

// =============================================================================
// Ownership Schema (v0.14)
// =============================================================================

// MapListOutput is the v0.14 JSON schema for `map list --format json`.
// It's a flat, joinable inventory of resources with ownership attribution.
type MapListOutput struct {
	Command   string            `json:"command"`
	Context   MapListContext    `json:"context"`
	Resources []MapListResource `json:"resources"`
	Summary   MapListSummary    `json:"summary"`
}

// MapListContext captures query parameters for the map list.
type MapListContext struct {
	Cluster   string  `json:"cluster"`
	Namespace *string `json:"namespace"` // null = all namespaces
}

// MapListResource represents a single resource with ownership attribution.
type MapListResource struct {
	ID     ResourceID `json:"id"`
	Owner  Owner      `json:"owner"`
	Status string     `json:"status"` // Ready, NotReady, Unknown, Progressing
}

// Owner represents the ownership attribution for a resource.
type Owner struct {
	Type string      `json:"type"` // Flux, ArgoCD, Helm, ConfigHub, Native
	Ref  *ResourceID `json:"ref"`  // null for Native
}

// MapListSummary provides aggregate counts in deterministic order.
type MapListSummary struct {
	Total    int           `json:"total"`
	ByOwner  []OwnerCount  `json:"byOwner"`  // Canonical owner order
	ByStatus []StatusCount `json:"byStatus"` // Sorted by status name
}

// OwnerCount represents a count for a specific owner type.
type OwnerCount struct {
	OwnerType string `json:"ownerType"`
	Count     int    `json:"count"`
}

// StatusCount represents a count for a specific status.
type StatusCount struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// BuildMapListJSON builds v0.14 JSON output from entries.
// Resources are sorted by (namespace, kind, name) for determinism.
// Summary arrays use canonical ordering (owners) or alphabetical (status).
func BuildMapListJSON(entries []Entry, cluster string, namespace *string) MapListOutput {
	// Sort entries for deterministic output
	sortEntriesDeterministic(entries)

	// Build resources and count by owner/status
	resources := make([]MapListResource, len(entries))
	ownerCounts := make(map[string]int)
	statusCounts := make(map[string]int)

	for i, e := range entries {
		// Normalize owner to canonical form
		ownerType := normalizeOwner(e.Owner)

		// Build owner reference
		var ownerRef *ResourceID
		if e.OwnerDetails != nil && e.OwnerDetails["name"] != "" {
			ownerRef = parseOwnerRef(e.OwnerDetails, e.Namespace)
		}

		// Normalize status (empty → "Unknown")
		status := e.Status
		if status == "" {
			status = "Unknown"
		}

		resources[i] = MapListResource{
			ID: ResourceID{
				Kind:      e.Kind,
				Namespace: e.Namespace,
				Name:      e.Name,
			},
			Owner: Owner{
				Type: ownerType,
				Ref:  ownerRef,
			},
			Status: status,
		}

		ownerCounts[ownerType]++
		statusCounts[status]++
	}

	// Build byOwner array in canonical order (include all owners, even with 0 count)
	byOwner := make([]OwnerCount, len(CanonicalOwnerOrder))
	for i, owner := range CanonicalOwnerOrder {
		byOwner[i] = OwnerCount{
			OwnerType: owner,
			Count:     ownerCounts[owner],
		}
	}

	// Build byStatus array in alphabetical order (only include non-zero)
	var byStatus []StatusCount
	statusOrder := []string{"NotReady", "Pending", "Progressing", "Ready", "Unknown"}
	for _, status := range statusOrder {
		if count := statusCounts[status]; count > 0 {
			byStatus = append(byStatus, StatusCount{
				Status: status,
				Count:  count,
			})
		}
	}

	return MapListOutput{
		Command: "map-list",
		Context: MapListContext{
			Cluster:   cluster,
			Namespace: namespace,
		},
		Resources: resources,
		Summary: MapListSummary{
			Total:    len(entries),
			ByOwner:  byOwner,
			ByStatus: byStatus,
		},
	}
}
