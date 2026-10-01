// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestReverseTraceSecretDoesNotExportLastAppliedPayload(t *testing.T) {
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{"name": "credentials", "namespace": "team-a", "annotations": map[string]interface{}{
			"kubectl.kubernetes.io/last-applied-configuration": `{"apiVersion":"v1","kind":"Secret","data":{"password":"c2VjcmV0LW1hcmtlcg=="},"stringData":{"token":"private-secret-marker"}}`,
			"example.org/source":                               "manual-fixture",
		}},
		"data": map[string]interface{}{"password": "c2VjcmV0LW1hcmtlcg=="},
	}}
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), secret)
	result, err := NewReverseTracer(client).Trace(context.Background(), "Secret", "credentials", "team-a")
	require.NoError(t, err)
	require.NotNil(t, result.OrphanMeta)
	require.Empty(t, result.OrphanMeta.LastAppliedConfig)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private-secret-marker")
	require.NotContains(t, string(encoded), "c2VjcmV0LW1hcmtlcg==")
	require.Contains(t, string(encoded), "lastAppliedConfigOmission")
	require.Equal(t, "manual-fixture", result.OrphanMeta.Annotations["example.org/source"])
	require.Len(t, client.Actions(), 1)
	require.Equal(t, "get", client.Actions()[0].GetVerb())
	require.NotEmpty(t, secret.GetAnnotations()["kubectl.kubernetes.io/last-applied-configuration"], "source object must not be mutated")
}
