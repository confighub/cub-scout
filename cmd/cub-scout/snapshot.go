// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/confighub/cub-scout/pkg/agent"
)

var (
	snapshotOutput    string
	snapshotNamespace string
	snapshotKind      string
	snapshotRelations bool
)

// GSFSnapshot represents the GitOps State Format output
type GSFSnapshot struct {
	Version     string        `json:"version"`
	GeneratedAt time.Time     `json:"generatedAt"`
	Cluster     string        `json:"cluster"`
	Entries     []GSFEntry    `json:"entries"`
	Relations   []GSFRelation `json:"relations,omitempty"`
	Summary     GSFSummary    `json:"summary"`
}

// GSFEntry represents a resource entry in GSF
type GSFEntry struct {
	ID         string            `json:"id"`
	Cluster    string            `json:"cluster"`
	Namespace  string            `json:"namespace"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	APIVersion string            `json:"apiVersion"`
	Owner      *GSFOwner         `json:"owner,omitempty"`
	Drift      *GSFDrift         `json:"drift,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
}

// GSFOwner represents ownership information
type GSFOwner struct {
	Type      string            `json:"type"`
	SubType   string            `json:"subType,omitempty"`
	Name      string            `json:"name,omitempty"`
	Namespace string            `json:"namespace,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

// GSFDrift represents drift information
type GSFDrift struct {
	Type       string `json:"type"`
	Summary    string `json:"summary"`
	DetectedAt string `json:"detectedAt"`
}

// GSFRelation represents a relationship between resources
type GSFRelation struct {
	From string `json:"from"`
	To   string `json:"to"`
	Type string `json:"type"`
}

// GSFSummary provides aggregated counts
type GSFSummary struct {
	Total   int            `json:"total"`
	ByKind  map[string]int `json:"byKind"`
	ByOwner map[string]int `json:"byOwner"`
	Drifted int            `json:"drifted"`
}

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Dump cluster state as GSF JSON",
	Long: `Take a snapshot of the current cluster state and output as GitOps State Format (GSF) JSON.

This is a read-only operation that queries the Kubernetes API and outputs
ownership, relationships, and status for all detected resources.

Examples:
  # Output to stdout
  cub-scout snapshot

  # Output to file
  cub-scout snapshot -o state.json

  # Pipe to jq
  cub-scout snapshot | jq '.entries[] | select(.owner.type == "flux")'

  # Filter by namespace
  cub-scout snapshot --namespace prod

  # Filter by kind
  cub-scout snapshot --kind Deployment
`,
	RunE: runSnapshot,
}

func init() {
	rootCmd.AddCommand(snapshotCmd)

	snapshotCmd.Flags().StringVarP(&snapshotOutput, "output", "o", "", "Output file (default: stdout, use '-' for explicit stdout)")
	snapshotCmd.Flags().StringVarP(&snapshotNamespace, "namespace", "n", "", "Filter by namespace")
	snapshotCmd.Flags().StringVarP(&snapshotKind, "kind", "k", "", "Filter by kind")
	snapshotCmd.Flags().BoolVar(&snapshotRelations, "relations", false, "Include resource relations (owns, selects, mounts, references)")
}

func runSnapshot(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Build Kubernetes config
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("build kubernetes config: %w", err)
	}

	// Create dynamic client
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create dynamic client: %w", err)
	}

	// Get cluster name
	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "default"
	}

	// Collect resources
	entries := []GSFEntry{}
	byKind := map[string]int{}
	byOwner := map[string]int{}

	// Resource types to scan
	resources := []schema.GroupVersionResource{
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "replicasets"},
		{Group: "apps", Version: "v1", Resource: "statefulsets"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "", Version: "v1", Resource: "pods"},
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		// Flux resources
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		// Argo resources
		{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"},
	}

	// Store raw items for relation building
	var allItems []unstructured.Unstructured

	for _, gvr := range resources {
		var list *unstructured.UnstructuredList
		var err error

		if snapshotNamespace != "" {
			list, err = dynClient.Resource(gvr).Namespace(snapshotNamespace).List(ctx, v1.ListOptions{})
		} else {
			list, err = dynClient.Resource(gvr).List(ctx, v1.ListOptions{})
		}

		if err != nil {
			// Skip resources that don't exist (CRDs not installed)
			continue
		}

		for _, item := range list.Items {
			// Filter by kind if specified
			if snapshotKind != "" && item.GetKind() != snapshotKind {
				continue
			}

			// Detect ownership
			ownership := agent.DetectOwnership(&item)

			entry := GSFEntry{
				ID:         fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, item.GetNamespace(), gvr.Group, item.GetKind(), item.GetName()),
				Cluster:    clusterName,
				Namespace:  item.GetNamespace(),
				Kind:       item.GetKind(),
				Name:       item.GetName(),
				APIVersion: item.GetAPIVersion(),
				Labels:     item.GetLabels(),
			}

			if ownership.Type != agent.OwnerUnknown {
				entry.Owner = &GSFOwner{
					Type:      ownership.Type,
					SubType:   ownership.SubType,
					Name:      ownership.Name,
					Namespace: ownership.Namespace,
				}
			}

			entries = append(entries, entry)
			byKind[item.GetKind()]++
			if ownership.Type != "" {
				byOwner[ownership.Type]++
			} else {
				byOwner["unknown"]++
			}

			// Store for relation building
			if snapshotRelations {
				allItems = append(allItems, item)
			}
		}
	}

	// Build relations if requested
	var relations []GSFRelation
	if snapshotRelations {
		relations = append(relations, buildOwnsRelations(allItems, clusterName)...)
		relations = append(relations, buildSelectsRelations(allItems, clusterName)...)
		relations = append(relations, buildMountsRelations(allItems, clusterName)...)
	}

	// Build snapshot
	snapshot := GSFSnapshot{
		Version:     "gsf/v1",
		GeneratedAt: time.Now().UTC(),
		Cluster:     clusterName,
		Entries:     entries,
		Relations:   relations,
		Summary: GSFSummary{
			Total:   len(entries),
			ByKind:  byKind,
			ByOwner: byOwner,
		},
	}

	// Encode output
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	if snapshotOutput != "" && snapshotOutput != "-" {
		f, err := os.Create(snapshotOutput)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		enc = json.NewEncoder(f)
		enc.SetIndent("", "  ")
	}

	if err := enc.Encode(snapshot); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}

	return nil
}

// buildOwnsRelations extracts ownership relations from OwnerReferences
func buildOwnsRelations(items []unstructured.Unstructured, clusterName string) []GSFRelation {
	var relations []GSFRelation

	// Build a map of UID -> entry ID for lookups
	uidToID := make(map[string]string)
	for _, item := range items {
		uid := string(item.GetUID())
		// Build ID: cluster/namespace/group/kind/name
		gv := item.GroupVersionKind()
		entryID := fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, item.GetNamespace(), gv.Group, gv.Kind, item.GetName())
		uidToID[uid] = entryID
	}

	// Extract ownerReferences from each item
	for _, item := range items {
		ownerRefs := item.GetOwnerReferences()
		if len(ownerRefs) == 0 {
			continue
		}

		// Build child ID
		gv := item.GroupVersionKind()
		childID := fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, item.GetNamespace(), gv.Group, gv.Kind, item.GetName())

		for _, ref := range ownerRefs {
			// Try to find the owner in our UID map
			ownerID, found := uidToID[string(ref.UID)]
			if !found {
				// Owner not in our scanned resources, build ID from reference
				// Parse apiVersion to get group
				group := ""
				if idx := len(ref.APIVersion) - 1; idx > 0 {
					for i := 0; i < len(ref.APIVersion); i++ {
						if ref.APIVersion[i] == '/' {
							group = ref.APIVersion[:i]
							break
						}
					}
				}
				ownerID = fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, item.GetNamespace(), group, ref.Kind, ref.Name)
			}

			// Create relation: owner owns child
			relations = append(relations, GSFRelation{
				From: ownerID,
				To:   childID,
				Type: "owns",
			})
		}
	}

	return relations
}

// buildSelectsRelations finds Service -> Pod relations via label selectors
func buildSelectsRelations(items []unstructured.Unstructured, clusterName string) []GSFRelation {
	var relations []GSFRelation

	// Collect all Pods with their labels, indexed by namespace
	type podInfo struct {
		id     string
		labels map[string]string
	}
	podsByNamespace := make(map[string][]podInfo)

	for _, item := range items {
		if item.GetKind() != "Pod" {
			continue
		}
		gv := item.GroupVersionKind()
		podID := fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, item.GetNamespace(), gv.Group, gv.Kind, item.GetName())
		podsByNamespace[item.GetNamespace()] = append(podsByNamespace[item.GetNamespace()], podInfo{
			id:     podID,
			labels: item.GetLabels(),
		})
	}

	// For each Service, find matching Pods
	for _, item := range items {
		if item.GetKind() != "Service" {
			continue
		}

		// Get service selector
		spec, found, err := unstructured.NestedMap(item.Object, "spec")
		if err != nil || !found {
			continue
		}

		selectorRaw, found, err := unstructured.NestedMap(spec, "selector")
		if err != nil || !found || len(selectorRaw) == 0 {
			continue
		}

		// Convert selector to map[string]string
		selector := make(map[string]string)
		for k, v := range selectorRaw {
			if vs, ok := v.(string); ok {
				selector[k] = vs
			}
		}

		if len(selector) == 0 {
			continue
		}

		// Build service ID
		gv := item.GroupVersionKind()
		serviceID := fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, item.GetNamespace(), gv.Group, gv.Kind, item.GetName())

		// Find matching Pods in the same namespace
		pods := podsByNamespace[item.GetNamespace()]
		for _, pod := range pods {
			if matchesSelector(pod.labels, selector) {
				relations = append(relations, GSFRelation{
					From: serviceID,
					To:   pod.id,
					Type: "selects",
				})
			}
		}
	}

	return relations
}

// matchesSelector checks if labels match all selector requirements
func matchesSelector(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// buildMountsRelations finds Pod -> ConfigMap/Secret relations via volume mounts
func buildMountsRelations(items []unstructured.Unstructured, clusterName string) []GSFRelation {
	var relations []GSFRelation

	for _, item := range items {
		if item.GetKind() != "Pod" {
			continue
		}

		// Build pod ID
		gv := item.GroupVersionKind()
		podID := fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, item.GetNamespace(), gv.Group, gv.Kind, item.GetName())
		namespace := item.GetNamespace()

		// Get spec.volumes
		spec, found, err := unstructured.NestedMap(item.Object, "spec")
		if err != nil || !found {
			continue
		}

		volumesRaw, found, err := unstructured.NestedSlice(spec, "volumes")
		if err != nil || !found {
			continue
		}

		for _, volRaw := range volumesRaw {
			vol, ok := volRaw.(map[string]interface{})
			if !ok {
				continue
			}

			// Check for configMap volume
			if cm, found, _ := unstructured.NestedMap(vol, "configMap"); found {
				if name, ok := cm["name"].(string); ok && name != "" {
					configMapID := fmt.Sprintf("%s/%s//ConfigMap/%s", clusterName, namespace, name)
					relations = append(relations, GSFRelation{
						From: podID,
						To:   configMapID,
						Type: "mounts",
					})
				}
			}

			// Check for secret volume
			if sec, found, _ := unstructured.NestedMap(vol, "secret"); found {
				if name, ok := sec["secretName"].(string); ok && name != "" {
					secretID := fmt.Sprintf("%s/%s//Secret/%s", clusterName, namespace, name)
					relations = append(relations, GSFRelation{
						From: podID,
						To:   secretID,
						Type: "mounts",
					})
				}
			}
		}
	}

	return relations
}
