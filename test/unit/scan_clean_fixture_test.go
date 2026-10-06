// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT
package unit

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func TestCleanScanFixtureExplicitBaseline(t *testing.T) {
	raw, err := os.ReadFile("../golden/scan-file/testdata/inputs/clean-deployment.yaml")
	require.NoError(t, err)
	data, err := yaml.YAMLToJSON(raw)
	require.NoError(t, err)
	var object map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &object))
	base := []string{"spec", "template", "spec"}
	for _, field := range []struct {
		path     []string
		expected bool
	}{
		{[]string{"automountServiceAccountToken"}, false},
		{[]string{"securityContext", "runAsNonRoot"}, true},
	} {
		value, found, err := unstructured.NestedBool(object, append(base, field.path...)...)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, field.expected, value)
	}
	seccomp, found, err := unstructured.NestedString(object, "spec", "template", "spec", "securityContext", "seccompProfile", "type")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "RuntimeDefault", seccomp)
	anti, found, err := unstructured.NestedSlice(object, "spec", "template", "spec", "affinity", "podAntiAffinity", "preferredDuringSchedulingIgnoredDuringExecution")
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, anti)
	containers, found, err := unstructured.NestedSlice(object, "spec", "template", "spec", "containers")
	require.NoError(t, err)
	require.True(t, found)
	require.NotEmpty(t, containers)
	for _, entry := range containers {
		container, ok := entry.(map[string]interface{})
		require.True(t, ok)
		for field, expected := range map[string]bool{"runAsNonRoot": true, "allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true} {
			value, found, err := unstructured.NestedBool(container, "securityContext", field)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, expected, value)
		}
		drop, found, err := unstructured.NestedStringSlice(container, "securityContext", "capabilities", "drop")
		require.NoError(t, err)
		require.True(t, found)
		require.Contains(t, drop, "ALL")
	}
}
