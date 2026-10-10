// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/confighub/cub-scout/v2/pkg/agent"
)

// recordedTreeClient serves the Applications a real Argo CD v3.5.3 left for
// examples/delivery-tree/argocd (see the fixture's NOTICE).
func recordedTreeClient(t *testing.T) *dynamicfake.FakeDynamicClient {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "delivery-tree-argocd-v353-recorded", "applications.json"))
	require.NoError(t, err)
	var list struct {
		Items []map[string]interface{} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(data, &list))
	objects := make([]runtime.Object, len(list.Items))
	for i := range list.Items {
		objects[i] = &unstructured.Unstructured{Object: list.Items[i]}
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), settingsListKinds, objects...)
}

// #855: the outputs shown in examples/delivery-tree are what the command
// prints for the recording. Run with UPDATE_DELIVERY_TREE_EXAMPLE=1 to write
// them again after a deliberate change, and read the diff.
func TestGitOpsTreeExampleOutputsAreWhatTheCommandPrints(t *testing.T) {
	root := &agent.DeliveryRef{Kind: "Application", Name: "delivery-tree"}
	for name, tc := range map[string]struct {
		params deliveryTreeParams
		format string
	}{
		"tree.txt":             {deliveryTreeParams{Root: root, MaxResources: 25}, "ascii"},
		"tree.md":              {deliveryTreeParams{Root: root, MaxResources: 25}, "md"},
		"tree.json":            {deliveryTreeParams{Root: root}, "json"},
		"tree-summary.json":    {deliveryTreeParams{Root: root, View: deliveryTreeViewSummary}, "json"},
		"tree-depth-1.txt":     {deliveryTreeParams{Root: root, Depth: 1, MaxResources: 25}, "ascii"},
		"tree-deployments.txt": {deliveryTreeParams{Root: root, Kinds: []string{"Deployment"}, MaxResources: 25}, "ascii"},
		"tree-out-of-sync.txt": {deliveryTreeParams{Root: root, Syncs: []string{"OutOfSync"}, MaxResources: 25}, "ascii"},
		"tree-every-root.txt":  {deliveryTreeParams{MaxResources: 25}, "ascii"},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.params.View == "" {
				tc.params.View = deliveryTreeViewAll
			}
			report := treeReport(t, recordedTreeClient(t), tc.params)
			report.Context = "kind-gitops-cluster"
			var out bytes.Buffer
			require.NoError(t, writeDeliveryTree(&out, report, tc.format, tc.params))

			path := filepath.Join("..", "..", "examples", "delivery-tree", "expected", name)
			if os.Getenv("UPDATE_DELIVERY_TREE_EXAMPLE") == "1" {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, out.Bytes(), 0o644))
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, string(want), out.String())
		})
	}
}
