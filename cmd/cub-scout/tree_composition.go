// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// CrossplaneCompositionTree groups composed resources by composition root.
// For Crossplane: XR -> managed (+ optional claim).
// For kro: instance -> managed (+ optional definition).
// It is intentionally presentation-only: it uses merged lineage resolvers.

type CrossplaneCompositionTree struct {
	Platform string                        `json:"platform,omitempty"`
	XR       agent.CrossplaneLineageNode   `json:"xr"`
	Claim    *agent.CrossplaneLineageNode  `json:"claim,omitempty"`
	Managed  []agent.CrossplaneLineageNode `json:"managed"`
}

func runTreeComposition(ctx context.Context) (resultErr error) {
	debug := os.Getenv("CUB_SCOUT_DEBUG") != ""
	var startTotal time.Time
	if debug {
		startTotal = time.Now()
	}

	cfg, err := treeClusterConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	if binding := treeContextBinding(ctx); binding != nil {
		child, err := (&traceSession{config: binding.config, context: binding.context, proxyURL: binding.proxyURL}).createChildKubeconfig()
		if err != nil {
			return fmt.Errorf("bind composition child reads: %w", err)
		}
		defer func() {
			if err := child.Cleanup(); err != nil && resultErr == nil {
				resultErr = fmt.Errorf("private composition binding cleanup failed")
			}
		}()
		ctx = context.WithValue(ctx, treeChildConfigKey{}, child)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	var startList time.Time
	if debug {
		startList = time.Now()
	}
	objs, warnings := listAllObjectsForComposition(ctx, dynClient)
	if debug {
		fmt.Fprintf(os.Stderr, "[debug] list: %d objects in %v\n", len(objs), time.Since(startList))
	}
	if len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Printf("%sNote:%s %s\n", colorYellow, colorReset, w)
		}
	}
	if len(objs) == 0 {
		fmt.Printf("%sNo resources found.%s\n", colorDim, colorReset)
		return nil
	}

	var startIndex time.Time
	if debug {
		startIndex = time.Now()
	}
	byXR := buildCompositionIndex(objs)
	if debug {
		fmt.Fprintf(os.Stderr, "[debug] index: %d XRs from %d objects in %v\n", len(byXR), len(objs), time.Since(startIndex))
		fmt.Fprintf(os.Stderr, "[debug] total: %v\n", time.Since(startTotal))
	}

	if treeJSON {
		return json.NewEncoder(os.Stdout).Encode(byXR)
	}

	printCompositionTreeHuman(byXR)
	return nil
}

// listAllObjectsForComposition gathers a broad set of objects to allow deterministic
// lineage resolution without discovery dependencies.
//
// Implementation detail: we shell out to kubectl api-resources to avoid client-side
// discovery / pluralization complexity. This is a best-effort scan; failures are reported
// as warnings and do not abort the tree.
func listAllObjectsForComposition(ctx context.Context, dynClient dynamic.Interface) ([]*unstructured.Unstructured, []string) {
	var warnings []string
	resourceNames, err := kubectlAPIResources(ctx)
	if err != nil {
		return nil, []string{fmt.Sprintf("unable to enumerate api-resources: %v", err)}
	}

	var objs []*unstructured.Unstructured

	// First attempt: use dynamic client for a curated set of known composition GVRs.
	// This keeps output useful even if kubectl is restricted.
	known := []schema.GroupVersionResource{
		{Group: "pkg.crossplane.io", Version: "v1", Resource: "providers"},
		{Group: "pkg.crossplane.io", Version: "v1", Resource: "providerrevisions"},
		{Group: "pkg.crossplane.io", Version: "v1", Resource: "configurations"},
		{Group: "pkg.crossplane.io", Version: "v1", Resource: "configurationrevisions"},
		{Group: "apiextensions.crossplane.io", Version: "v1", Resource: "compositions"},
		{Group: "apiextensions.crossplane.io", Version: "v1", Resource: "compositeresourcedefinitions"},
		{Group: "kro.run", Version: "v1alpha1", Resource: "resourcegraphdefinitions"},
	}
	for _, gvr := range known {
		list, err := dynClient.Resource(gvr).List(ctx, metav1.ListOptions{})
		if err == nil {
			for i := range list.Items {
				item := list.Items[i]
				objs = append(objs, &item)
			}
		}
	}

	// Broad scan via kubectl for composed resources / XRs / claims.
	for _, res := range resourceNames {
		ul, err := kubectlGetUnstructuredList(ctx, res)
		if err != nil {
			// Many resource types will fail due to RBAC or unsupported list operations; ignore.
			warnings = append(warnings, fmt.Sprintf("skipping %s: %v", res, err))
			continue
		}
		for i := range ul.Items {
			item := ul.Items[i]
			ns := item.GetNamespace()
			if treeNamespace != "" && ns != treeNamespace {
				continue
			}
			if !treeAll && ns != "" && isSystemNamespace(ns) {
				continue
			}
			objs = append(objs, &item)
		}
	}

	return objs, warnings
}

// This identifies a supplied presentation reference, not an object UID or cluster.
// Present/partial references remain separate even when their display names match.
type compositionRootIdentity struct {
	Platform string
	Ref      agent.ResourceRef
	Present  bool
}

func compositionReferenceKey(node agent.CrossplaneLineageNode) string {
	raw, _ := json.Marshal(compositionRootIdentity{Ref: node.Ref, Present: node.Present})
	return string(raw)
}

func compositionQualifiedRef(ref agent.ResourceRef, qualify bool) string {
	label := ref.String()
	if !qualify {
		return label
	}
	apiVersion := ref.Version
	if ref.Group != "" {
		apiVersion = ref.Group + "/" + ref.Version
	}
	if apiVersion == "" {
		apiVersion = "unknown"
	}
	return label + " [apiVersion=" + apiVersion + "]"
}

// buildCompositionIndex groups resources by composition root using platform lineage resolvers.
// Reuses the supplied inventory index without adding collection reads.
func buildCompositionIndex(objs []*unstructured.Unstructured) map[string]*CrossplaneCompositionTree {
	byRoot := make(map[compositionRootIdentity]*CrossplaneCompositionTree)

	// Build index once, reuse for all objects
	idx := agent.NewUnstructuredIndex(objs)

	for _, obj := range objs {
		if lineage, ok := agent.ResolveCrossplaneLineageWithIndex(obj, idx); ok && lineage != nil {
			// Skip completely unnamed roots. Named unresolved references remain
			// provisional partial buckets rather than asserted observed parents.
			if lineage.Composite.Ref.Name == "" {
				continue
			}

			xrKey := compositionRootIdentity{Platform: "crossplane", Ref: lineage.Composite.Ref, Present: lineage.Composite.Present}

			node := byRoot[xrKey]
			if node == nil {
				node = &CrossplaneCompositionTree{
					Platform: "crossplane",
					XR:       lineage.Composite,
				}
				if lineage.Claim != nil {
					node.Claim = lineage.Claim
				}
				byRoot[xrKey] = node
			}

			// Claims enrich only this supplied root reference.
			if lineage.Claim != nil {
				if node.Claim == nil || (lineage.Claim.Present && !node.Claim.Present) {
					node.Claim = lineage.Claim
				}
			}

			// A shared name does not identify the same object: compare the full supplied reference.
			if lineage.Managed.Ref.Name != "" && lineage.Managed.Ref != lineage.Composite.Ref {
				node.Managed = append(node.Managed, lineage.Managed)
			}
			continue
		}

		lineage, ok := agent.ResolveKroLineageWithIndex(obj, idx)
		if !ok || lineage == nil || lineage.Instance.Ref.Name == "" {
			continue
		}

		xrKey := compositionRootIdentity{Platform: "kro", Ref: lineage.Instance.Ref, Present: lineage.Instance.Present}
		node := byRoot[xrKey]
		if node == nil {
			node = &CrossplaneCompositionTree{
				Platform: "kro",
				XR:       toCrossplaneLineageNode(lineage.Instance),
			}
			if lineage.Definition != nil {
				def := toCrossplaneLineageNode(*lineage.Definition)
				node.Claim = &def
			}
			byRoot[xrKey] = node
		}

		if lineage.Definition != nil {
			def := toCrossplaneLineageNode(*lineage.Definition)
			if node.Claim == nil || (def.Present && !node.Claim.Present) {
				node.Claim = &def
			}
		}

		managed := toCrossplaneLineageNode(lineage.Managed)
		if managed.Ref.Name != "" && managed.Ref != lineage.Instance.Ref {
			node.Managed = append(node.Managed, managed)
		}
	}

	// Sort managed for stability.
	for _, node := range byRoot {
		sort.Slice(node.Managed, func(i, j int) bool {
			left, right := node.Managed[i], node.Managed[j]
			if left.Ref.String() != right.Ref.String() {
				return left.Ref.String() < right.Ref.String()
			}
			return compositionReferenceKey(left) < compositionReferenceKey(right)
		})
	}

	// Project compatibility keys only after all distinct references are collected.
	counts := make(map[string]int)
	for identity := range byRoot {
		counts[identity.Platform+"::"+identity.Ref.String()]++
	}
	byXR := make(map[string]*CrossplaneCompositionTree, len(byRoot))
	for identity, node := range byRoot {
		key := identity.Platform + "::" + identity.Ref.String()
		if counts[key] > 1 {
			raw, _ := json.Marshal(identity) // Fixed string/bool fields cannot fail JSON encoding.
			key += "::ref=" + base64.RawURLEncoding.EncodeToString(raw)
		}
		byXR[key] = node
	}
	return byXR
}

func toCrossplaneLineageNode(node agent.KroLineageNode) agent.CrossplaneLineageNode {
	return agent.CrossplaneLineageNode{
		Ref:     node.Ref,
		Present: node.Present,
	}
}

func printCompositionTreeHuman(byXR map[string]*CrossplaneCompositionTree) {
	fmt.Printf("%sPlatform Composition Tree%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 60))

	xrKeys := make([]string, 0, len(byXR))
	for k := range byXR {
		xrKeys = append(xrKeys, k)
	}
	sort.Strings(xrKeys)
	rootLabels := make(map[string]int)
	for _, node := range byXR {
		if node != nil {
			rootLabels[node.Platform+"::"+node.XR.Ref.String()]++
		}
	}

	for _, xrKey := range xrKeys {
		node := byXR[xrKey]
		if node == nil {
			continue
		}

		platform := node.Platform
		if platform == "" {
			platform = "crossplane"
		}

		// XR/instance line
		xrLabel := compositionQualifiedRef(node.XR.Ref, rootLabels[node.Platform+"::"+node.XR.Ref.String()] > 1)
		if xrLabel == "" {
			xrLabel = xrKey
		}
		if !node.XR.Present {
			xrLabel += fmt.Sprintf(" %s(partial lineage)%s", colorDim, colorReset)
		}
		fmt.Printf("%s[%s]%s %s%s%s\n", colorCyan, platform, colorReset, colorCyan, xrLabel, colorReset)

		// Optional parent (crossplane claim / kro definition)
		if node.Claim != nil {
			claimLabel := node.Claim.Ref.String()
			if !node.Claim.Present {
				claimLabel += fmt.Sprintf(" %s(partial lineage)%s", colorDim, colorReset)
			}
			label := "claim"
			if platform == "kro" {
				label = "definition"
			}
			fmt.Printf("  ├── %s: %s\n", label, claimLabel)
		}

		// Managed resources with colliding display labels retain their API references.
		childLabels := make(map[string]map[string]bool)
		for _, child := range node.Managed {
			label := child.Ref.String()
			if childLabels[label] == nil {
				childLabels[label] = make(map[string]bool)
			}
			childLabels[label][compositionReferenceKey(child)] = true
		}
		for i, m := range node.Managed {
			connector := "├──"
			if i == len(node.Managed)-1 {
				connector = "└──"
			}
			fmt.Printf("  %s %s\n", connector, compositionQualifiedRef(m.Ref, len(childLabels[m.Ref.String()]) > 1))
		}
		fmt.Println()
	}
}

func kubectlAPIResources(ctx context.Context) ([]string, error) {
	cmd := treeKubectlCommand(ctx, "api-resources", "--verbs=list", "-o", "name")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl api-resources failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(string(out), "\n")
	var resources []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		resources = append(resources, l)
	}
	return resources, nil
}

func kubectlGetUnstructuredList(ctx context.Context, resource string) (*unstructured.UnstructuredList, error) {
	args := []string{"get", resource, "-o", "json"}
	// namespace filtering is done after fetch to avoid needing discovery for namespace-scoped types.
	args = append(args, "-A")

	cmd := treeKubectlCommand(ctx, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("kubectl get %s failed: %v (%s)", resource, err, strings.TrimSpace(string(out)))
	}

	var ul unstructured.UnstructuredList
	if err := json.Unmarshal(out, &ul); err != nil {
		return nil, fmt.Errorf("decode %s list: %w", resource, err)
	}
	return &ul, nil
}
