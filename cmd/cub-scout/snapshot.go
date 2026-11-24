// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/confighub/cub-scout/v3/pkg/agent"
)

const (
	snapshotFormatGSF  = "gsf"
	snapshotFormatYAML = "yaml"
	snapshotFormatJSON = "json"

	defaultSnapshotMaxObjects = 200
)

var (
	snapshotOutput     string
	snapshotNamespace  string
	snapshotKind       string
	snapshotRelations  bool
	snapshotFormat     = snapshotFormatGSF
	snapshotMaxObjects = defaultSnapshotMaxObjects
	snapshotNow        = time.Now
)

// GSFSnapshot represents the GitOps State Format output
type GSFSnapshot struct {
	Version     string                     `json:"version"`
	GeneratedAt time.Time                  `json:"generatedAt"`
	Observation *agent.ObservationEvidence `json:"observation,omitempty"`
	Cluster     string                     `json:"cluster"`
	Entries     []GSFEntry                 `json:"entries"`
	Relations   []GSFRelation              `json:"relations,omitempty"`
	Summary     GSFSummary                 `json:"summary"`
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
	Short: "Dump cluster state as GSF JSON or sanitized offline recording YAML/JSON",
	Long: `Take a read-only snapshot of the current cluster state.

By default (--format gsf), outputs GitOps State Format (GSF) JSON with ownership
and optional relationships.

With --format yaml or --format json, captures a bounded, sanitized offline
recording of workloads (Deployment, StatefulSet, DaemonSet, Job, CronJob) and
GitOps controller objects (Flux Kustomization/HelmRelease/GitRepository/OCIRepository,
Argo CD Application, and Helm release Secrets with Secret.data and Secret.stringData
strictly stripped). Offline commands such as landed --recording, gitops status
--recording, and doctor --recording evaluate these snapshots with zero cluster access.

Examples:
  # Output GSF JSON to stdout
  cub-scout snapshot

  # Capture a sanitized offline multi-document YAML snapshot
  cub-scout snapshot --format yaml -n prod -o prod-snapshot.yaml

  # Capture a sanitized offline JSON List snapshot bounded to 100 objects
  cub-scout snapshot --format json --max-objects 100 -o prod-snapshot.json

  # Output GSF JSON to file
  cub-scout snapshot -o state.json

  # Filter by namespace
  cub-scout snapshot --namespace prod

  # Filter by kind
  cub-scout snapshot --kind Deployment
`,
	RunE: runSnapshot,
}

func init() {
	rootCmd.AddCommand(snapshotCmd)
	snapshotCmd.Flags().String("kube-context", "", "Kubernetes context to inspect (strict explicit selection)")

	snapshotCmd.Flags().StringVarP(&snapshotOutput, "output", "o", "", "Output file (default: stdout, use '-' for explicit stdout)")
	snapshotCmd.Flags().StringVarP(&snapshotNamespace, "namespace", "n", "", "Filter by namespace")
	snapshotCmd.Flags().StringVarP(&snapshotKind, "kind", "k", "", "Filter by kind")
	snapshotCmd.Flags().BoolVar(&snapshotRelations, "relations", false, "Include resource relations (owns, selects, mounts, references; GSF format only)")
	snapshotCmd.Flags().StringVar(&snapshotFormat, "format", snapshotFormatGSF, "Output format: gsf (default, GSF JSON), yaml (sanitized offline recording YAML), json (sanitized offline recording JSON List)")
	snapshotCmd.Flags().IntVar(&snapshotMaxObjects, "max-objects", defaultSnapshotMaxObjects, "Maximum number of objects to capture in yaml/json recording mode (1-512)")
}

func runSnapshot(cmd *cobra.Command, args []string) error {
	format, err := normalizeSnapshotFormat(snapshotFormat)
	if err != nil {
		return err
	}
	if format != snapshotFormatGSF {
		if snapshotMaxObjects < 1 || snapshotMaxObjects > maxRecordedObjectCount {
			return fmt.Errorf("invalid --max-objects %d (must be between 1 and %d)", snapshotMaxObjects, maxRecordedObjectCount)
		}
		if cmd.Flags().Changed("relations") && snapshotRelations {
			return fmt.Errorf("--relations applies only to --format gsf")
		}
	}

	ctx := cmd.Context()

	// Build Kubernetes config
	cfg, selectedContext, err := buildClusterConfigForCommand(cmd)
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
	if selectedContext != "" {
		clusterName = selectedContext
	} else if clusterName == "" {
		clusterName = "default"
	}
	observedAt := snapshotNow().UTC()

	if format == snapshotFormatYAML || format == snapshotFormatJSON {
		objects, truncated, err := collectRecordedSnapshotObjects(ctx, dynClient, snapshotNamespace, snapshotKind, snapshotMaxObjects)
		if err != nil {
			return err
		}
		out := io.Writer(os.Stdout)
		if snapshotOutput != "" && snapshotOutput != "-" {
			f, err := os.Create(snapshotOutput)
			if err != nil {
				return fmt.Errorf("create output file: %w", err)
			}
			defer f.Close()
			out = f
		}
		sourceContext := selectedContext
		if sourceContext == "" {
			sourceContext = clusterName
		}
		return encodeRecordedSnapshot(out, format, objects, observedAt, sourceContext, truncated)
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
		relations = append(relations, buildReferencesRelations(allItems, clusterName)...)
	}

	// Build snapshot
	snapshot := GSFSnapshot{
		Version:     "gsf/v1",
		GeneratedAt: observedAt,
		Observation: buildSnapshotObservation(clusterName, snapshotNamespace, snapshotKind, observedAt),
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

func buildSnapshotObservation(clusterName, namespace, kind string, observedAt time.Time) *agent.ObservationEvidence {
	return agent.NewObservationEvidence(
		agent.ObservationSourceKubernetesAPI,
		agent.ObservationModeSnapshot,
		observedAt,
		agent.ObservationScope{Cluster: clusterName, Namespace: namespace, Kind: kind},
	)
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

// buildReferencesRelations finds Pod -> ConfigMap/Secret relations via envFrom and env valueFrom
func buildReferencesRelations(items []unstructured.Unstructured, clusterName string) []GSFRelation {
	var relations []GSFRelation

	// Track already added relations to avoid duplicates
	seen := make(map[string]bool)

	for _, item := range items {
		if item.GetKind() != "Pod" {
			continue
		}

		// Build pod ID
		gv := item.GroupVersionKind()
		podID := fmt.Sprintf("%s/%s/%s/%s/%s", clusterName, item.GetNamespace(), gv.Group, gv.Kind, item.GetName())
		namespace := item.GetNamespace()

		// Get spec.containers and spec.initContainers
		spec, found, err := unstructured.NestedMap(item.Object, "spec")
		if err != nil || !found {
			continue
		}

		// Process both containers and initContainers
		for _, containerType := range []string{"containers", "initContainers"} {
			containersRaw, found, err := unstructured.NestedSlice(spec, containerType)
			if err != nil || !found {
				continue
			}

			for _, containerRaw := range containersRaw {
				container, ok := containerRaw.(map[string]interface{})
				if !ok {
					continue
				}

				// Process envFrom
				envFromRaw, found, _ := unstructured.NestedSlice(container, "envFrom")
				if found {
					for _, efRaw := range envFromRaw {
						ef, ok := efRaw.(map[string]interface{})
						if !ok {
							continue
						}

						// configMapRef
						if cmRef, found, _ := unstructured.NestedMap(ef, "configMapRef"); found {
							if name, ok := cmRef["name"].(string); ok && name != "" {
								targetID := fmt.Sprintf("%s/%s//ConfigMap/%s", clusterName, namespace, name)
								relKey := podID + "->" + targetID
								if !seen[relKey] {
									seen[relKey] = true
									relations = append(relations, GSFRelation{
										From: podID,
										To:   targetID,
										Type: "references",
									})
								}
							}
						}

						// secretRef
						if secRef, found, _ := unstructured.NestedMap(ef, "secretRef"); found {
							if name, ok := secRef["name"].(string); ok && name != "" {
								targetID := fmt.Sprintf("%s/%s//Secret/%s", clusterName, namespace, name)
								relKey := podID + "->" + targetID
								if !seen[relKey] {
									seen[relKey] = true
									relations = append(relations, GSFRelation{
										From: podID,
										To:   targetID,
										Type: "references",
									})
								}
							}
						}
					}
				}

				// Process env[].valueFrom
				envRaw, found, _ := unstructured.NestedSlice(container, "env")
				if found {
					for _, eRaw := range envRaw {
						e, ok := eRaw.(map[string]interface{})
						if !ok {
							continue
						}

						valueFrom, found, _ := unstructured.NestedMap(e, "valueFrom")
						if !found {
							continue
						}

						// configMapKeyRef
						if cmRef, found, _ := unstructured.NestedMap(valueFrom, "configMapKeyRef"); found {
							if name, ok := cmRef["name"].(string); ok && name != "" {
								targetID := fmt.Sprintf("%s/%s//ConfigMap/%s", clusterName, namespace, name)
								relKey := podID + "->" + targetID
								if !seen[relKey] {
									seen[relKey] = true
									relations = append(relations, GSFRelation{
										From: podID,
										To:   targetID,
										Type: "references",
									})
								}
							}
						}

						// secretKeyRef
						if secRef, found, _ := unstructured.NestedMap(valueFrom, "secretKeyRef"); found {
							if name, ok := secRef["name"].(string); ok && name != "" {
								targetID := fmt.Sprintf("%s/%s//Secret/%s", clusterName, namespace, name)
								relKey := podID + "->" + targetID
								if !seen[relKey] {
									seen[relKey] = true
									relations = append(relations, GSFRelation{
										From: podID,
										To:   targetID,
										Type: "references",
									})
								}
							}
						}
					}
				}
			}
		}
	}

	return relations
}

func normalizeSnapshotFormat(raw string) (string, error) {
	f := strings.ToLower(strings.TrimSpace(raw))
	if f == "" {
		return snapshotFormatGSF, nil
	}
	switch f {
	case snapshotFormatGSF, snapshotFormatYAML, snapshotFormatJSON:
		return f, nil
	default:
		return "", fmt.Errorf("invalid --format %q (valid: gsf, yaml, json)", raw)
	}
}

type snapshotResourceSpec struct {
	GVR  schema.GroupVersionResource
	Kind string
}

// recordedSnapshotResources lists the bounded workload and GitOps controller
// resource types captured by `cub-scout snapshot --format yaml|json`.
func recordedSnapshotResources() []snapshotResourceSpec {
	return []snapshotResourceSpec{
		// Workloads
		{GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, Kind: "Deployment"},
		{GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, Kind: "StatefulSet"},
		{GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, Kind: "DaemonSet"},
		{GVR: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, Kind: "Job"},
		{GVR: schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, Kind: "CronJob"},
		// Flux controllers & sources
		{GVR: schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}, Kind: "Kustomization"},
		{GVR: schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}, Kind: "HelmRelease"},
		{GVR: schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}, Kind: "GitRepository"},
		{GVR: schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories"}, Kind: "OCIRepository"},
		// Argo CD
		{GVR: schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}, Kind: "Application"},
		// Helm release Secrets (filtered strictly to Helm release Secrets; data/stringData always stripped)
		{GVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, Kind: "Secret"},
	}
}

func isHelmReleaseSecret(obj *unstructured.Unstructured) bool {
	if obj == nil {
		return false
	}
	if secretType, _, _ := unstructured.NestedString(obj.Object, "type"); secretType == "helm.sh/release.v1" {
		return true
	}
	if strings.HasPrefix(obj.GetName(), "sh.helm.release.v1.") {
		return true
	}
	labels := obj.GetLabels()
	return strings.EqualFold(strings.TrimSpace(labels["owner"]), "helm")
}

// sanitizeRecordedSnapshotObject returns a deep-copied, sanitized object safe
// for offline recording. For Secrets, data and stringData (and last-applied
// annotations) are strictly stripped so no secret material is ever captured.
func sanitizeRecordedSnapshotObject(item *unstructured.Unstructured, fallbackSpec snapshotResourceSpec) *unstructured.Unstructured {
	if item == nil {
		return nil
	}
	cp := item.DeepCopy()
	if cp.Object == nil {
		cp.Object = map[string]interface{}{}
	}
	if cp.GetAPIVersion() == "" && fallbackSpec.GVR.Version != "" {
		cp.SetAPIVersion(fallbackSpec.GVR.GroupVersion().String())
	}
	if cp.GetKind() == "" && fallbackSpec.Kind != "" {
		cp.SetKind(fallbackSpec.Kind)
	}

	// Ensure metadata.namespace is explicitly recorded (even "" for cluster-scoped
	// objects) so readRecordedObjectSnapshot accepts the emitted object cleanly.
	meta, _ := cp.Object["metadata"].(map[string]interface{})
	if meta == nil {
		meta = map[string]interface{}{}
		cp.Object["metadata"] = meta
	}
	if _, ok := meta["namespace"]; !ok {
		meta["namespace"] = ""
	}

	if strings.EqualFold(cp.GetKind(), "Secret") {
		sanitizedMeta := map[string]interface{}{
			"name":      cp.GetName(),
			"namespace": cp.GetNamespace(),
		}
		for _, key := range []string{"uid", "resourceVersion", "creationTimestamp", "generation", "ownerReferences", "labels"} {
			if val, exists := meta[key]; exists {
				sanitizedMeta[key] = val
			}
		}
		if rawAnn, ok := meta["annotations"].(map[string]interface{}); ok && len(rawAnn) > 0 {
			cleanAnn := make(map[string]interface{}, len(rawAnn))
			for k, v := range rawAnn {
				if k == "kubectl.kubernetes.io/last-applied-configuration" {
					continue
				}
				cleanAnn[k] = v
			}
			if len(cleanAnn) > 0 {
				sanitizedMeta["annotations"] = cleanAnn
			}
		}
		sanitized := map[string]interface{}{
			"apiVersion": cp.GetAPIVersion(),
			"kind":       "Secret",
			"metadata":   sanitizedMeta,
		}
		if secretType, found, _ := unstructured.NestedString(cp.Object, "type"); found && secretType != "" {
			sanitized["type"] = secretType
		}
		return &unstructured.Unstructured{Object: sanitized}
	}

	return cp
}

func collectRecordedSnapshotObjects(ctx context.Context, dynClient dynamic.Interface, namespace, kindFilter string, maxObjects int) ([]*unstructured.Unstructured, bool, error) {
	if maxObjects < 1 {
		maxObjects = defaultSnapshotMaxObjects
	}
	if maxObjects > maxRecordedObjectCount {
		maxObjects = maxRecordedObjectCount
	}

	captured := make([]*unstructured.Unstructured, 0)
	truncated := false
	kindFilter = strings.TrimSpace(kindFilter)

	for _, spec := range recordedSnapshotResources() {
		if kindFilter != "" && !strings.EqualFold(spec.Kind, kindFilter) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}

		var list *unstructured.UnstructuredList
		var err error
		if namespace != "" {
			list, err = dynClient.Resource(spec.GVR).Namespace(namespace).List(ctx, v1.ListOptions{})
		} else {
			list, err = dynClient.Resource(spec.GVR).List(ctx, v1.ListOptions{})
		}
		if err != nil {
			// Skip unavailable CRDs or optional APIs cleanly.
			continue
		}

		// Sort items deterministically by namespace/name before applying cap.
		items := make([]unstructured.Unstructured, len(list.Items))
		copy(items, list.Items)
		sort.Slice(items, func(i, j int) bool {
			if items[i].GetNamespace() != items[j].GetNamespace() {
				return items[i].GetNamespace() < items[j].GetNamespace()
			}
			return items[i].GetName() < items[j].GetName()
		})

		for i := range items {
			item := &items[i]
			if spec.Kind == "Secret" && !isHelmReleaseSecret(item) {
				continue
			}
			if len(captured) >= maxObjects {
				truncated = true
				break
			}
			sanitized := sanitizeRecordedSnapshotObject(item, spec)
			if sanitized != nil {
				captured = append(captured, sanitized)
			}
		}
		if truncated {
			break
		}
	}

	return captured, truncated, nil
}

func encodeRecordedSnapshot(w io.Writer, format string, objects []*unstructured.Unstructured, observedAt time.Time, sourceContext string, truncated bool) error {
	obsStr := observedAt.UTC().Format(time.RFC3339)
	annotations := map[string]string{
		snapshotObservedAtAnnotation: obsStr,
	}
	if strings.TrimSpace(sourceContext) != "" {
		annotations[snapshotSourceContextAnnotation] = strings.TrimSpace(sourceContext)
	}
	if truncated {
		annotations["cub-scout.confighub.com/snapshot-truncated"] = "true"
	}

	rawItems := make([]map[string]interface{}, 0, len(objects))
	for _, obj := range objects {
		if obj != nil && obj.Object != nil {
			rawItems = append(rawItems, obj.Object)
		}
	}

	if format == snapshotFormatJSON {
		envelope := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "List",
			"metadata": map[string]interface{}{
				"annotations": annotations,
			},
			"items": rawItems,
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(envelope)
	}

	// YAML format: emit header comments recording observedAt and sourceContext.
	var header strings.Builder
	header.WriteString("# cub-scout snapshot\n")
	header.WriteString("# observedAt: " + obsStr + "\n")
	if strings.TrimSpace(sourceContext) != "" {
		header.WriteString("# sourceContext: " + strings.TrimSpace(sourceContext) + "\n")
	}
	if truncated {
		header.WriteString("# truncated: true\n")
	}
	if _, err := io.WriteString(w, header.String()); err != nil {
		return err
	}

	// When empty or exceeding the multi-document limit (128), emit a single
	// v1/List YAML document so readRecordedObjectSnapshot can parse up to 512 objects.
	if len(rawItems) == 0 || len(rawItems) > maxRecordedObjectDocuments {
		envelope := map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "List",
			"metadata": map[string]interface{}{
				"annotations": annotations,
			},
			"items": rawItems,
		}
		yamlBytes, err := sigsyaml.Marshal(envelope)
		if err != nil {
			return fmt.Errorf("marshal snapshot list yaml: %w", err)
		}
		_, err = w.Write(yamlBytes)
		return err
	}

	// Multi-document YAML for 1..128 objects.
	for i, item := range rawItems {
		if i > 0 {
			if _, err := io.WriteString(w, "---\n"); err != nil {
				return err
			}
		}
		yamlBytes, err := sigsyaml.Marshal(item)
		if err != nil {
			return fmt.Errorf("marshal snapshot object yaml: %w", err)
		}
		if _, err := w.Write(yamlBytes); err != nil {
			return err
		}
	}
	return nil
}
