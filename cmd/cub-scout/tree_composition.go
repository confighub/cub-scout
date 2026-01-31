// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/confighub/cub-scout/pkg/agent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// CrossplaneCompositionTree groups composed resources by XR (Composite).
// It is intentionally presentation-only: it uses the merged lineage resolver.

type CrossplaneCompositionTree struct {
	XR      agent.CrossplaneLineageNode   `json:"xr"`
	Claim   *agent.CrossplaneLineageNode  `json:"claim,omitempty"`
	Managed []agent.CrossplaneLineageNode `json:"managed"`
}

func runTreeComposition(ctx context.Context) error {
	cfg, err := buildConfig()
	if err != nil {
		return fmt.Errorf("failed to build config: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	objs, warnings := listAllObjectsForComposition(ctx, dynClient)
	if len(warnings) > 0 {
		for _, w := range warnings {
			fmt.Printf("%sNote:%s %s\n", colorYellow, colorReset, w)
		}
	}
	if len(objs) == 0 {
		fmt.Printf("%sNo resources found.%s\n", colorDim, colorReset)
		return nil
	}

	byXR := buildCompositionIndex(objs)
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

	// First attempt: use dynamic client for a curated set of known Crossplane GVRs.
	// This keeps output useful even if kubectl is restricted.
	known := []schema.GroupVersionResource{
		{Group: "pkg.crossplane.io", Version: "v1", Resource: "providers"},
		{Group: "pkg.crossplane.io", Version: "v1", Resource: "providerrevisions"},
		{Group: "pkg.crossplane.io", Version: "v1", Resource: "configurations"},
		{Group: "pkg.crossplane.io", Version: "v1", Resource: "configurationrevisions"},
		{Group: "apiextensions.crossplane.io", Version: "v1", Resource: "compositions"},
		{Group: "apiextensions.crossplane.io", Version: "v1", Resource: "compositeresourcedefinitions"},
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

// buildCompositionIndex groups resources by XR using the lineage resolver.
func buildCompositionIndex(objs []*unstructured.Unstructured) map[string]*CrossplaneCompositionTree {
	byXR := make(map[string]*CrossplaneCompositionTree)

	for _, obj := range objs {
		lineage, ok := agent.ResolveCrossplaneLineage(obj, objs)
		if !ok || lineage == nil {
			continue
		}

		// Skip resources where the XR could not be identified (partial lineage with unknown XR).
		// This avoids creating spurious groups for XRs that have claim labels but no parent XR.
		if lineage.Composite.Ref.Name == "" {
			continue
		}

		xrKey := lineage.Composite.Ref.String()
		if xrKey == "" {
			xrKey = lineage.Composite.Ref.Name
		}
		if xrKey == "" {
			continue
		}

		node := byXR[xrKey]
		if node == nil {
			node = &CrossplaneCompositionTree{XR: lineage.Composite}
			if lineage.Claim != nil {
				node.Claim = lineage.Claim
			}
			byXR[xrKey] = node
		}

		// Prefer a present XR/Claim if we see one later.
		if lineage.Composite.Present && !node.XR.Present {
			node.XR = lineage.Composite
		}
		if lineage.Claim != nil {
			if node.Claim == nil || (lineage.Claim.Present && !node.Claim.Present) {
				node.Claim = lineage.Claim
			}
		}

		// Don't add the XR itself as managed.
		if lineage.Managed.Ref.Name != "" && lineage.Managed.Ref.Name != lineage.Composite.Ref.Name {
			node.Managed = append(node.Managed, lineage.Managed)
		}
	}

	// Sort managed for stability.
	for _, node := range byXR {
		sort.Slice(node.Managed, func(i, j int) bool {
			return node.Managed[i].Ref.String() < node.Managed[j].Ref.String()
		})
	}

	return byXR
}

func printCompositionTreeHuman(byXR map[string]*CrossplaneCompositionTree) {
	fmt.Printf("%sCrossplane Composition Tree%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 60))

	xrKeys := make([]string, 0, len(byXR))
	for k := range byXR {
		xrKeys = append(xrKeys, k)
	}
	sort.Strings(xrKeys)

	for _, xrKey := range xrKeys {
		node := byXR[xrKey]
		if node == nil {
			continue
		}

		// XR line
		xrLabel := node.XR.Ref.String()
		if xrLabel == "" {
			xrLabel = xrKey
		}
		if !node.XR.Present {
			xrLabel += fmt.Sprintf(" %s(partial lineage)%s", colorDim, colorReset)
		}
		fmt.Printf("%s%s%s\n", colorCyan, xrLabel, colorReset)

		// Optional claim
		if node.Claim != nil {
			claimLabel := node.Claim.Ref.String()
			if !node.Claim.Present {
				claimLabel += fmt.Sprintf(" %s(partial lineage)%s", colorDim, colorReset)
			}
			fmt.Printf("  ├── claim: %s\n", claimLabel)
		}

		// Managed resources
		for i, m := range node.Managed {
			connector := "├──"
			if i == len(node.Managed)-1 {
				connector = "└──"
			}
			fmt.Printf("  %s %s\n", connector, m.Ref.String())
		}
		fmt.Println()
	}
}

func kubectlAPIResources(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "api-resources", "--verbs=list", "-o", "name")
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

	cmd := exec.CommandContext(ctx, "kubectl", args...)
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
