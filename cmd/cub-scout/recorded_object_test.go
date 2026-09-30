// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var recordedDeploymentIdentity = recordedObjectIdentity{
	APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api",
}

const recordedDeployment = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
  labels:
    app: api
  managedFields:
    - manager: controller.example
      operation: Apply
spec:
  replicas: 2
status:
  readyReplicas: 1
`

func TestLoadRecordedObjectExactIdentityAndStableProvenance(t *testing.T) {
	input := "# exact source bytes\n" + recordedDeployment
	first, err := loadRecordedObject(strings.NewReader(input), recordedDeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadRecordedObject(strings.NewReader(input), recordedDeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256([]byte(input))
	if first.Provenance.SHA256 != hex.EncodeToString(wantDigest[:]) || first.Provenance != second.Provenance {
		t.Fatalf("provenance is not deterministic over exact bytes: %#v vs %#v", first.Provenance, second.Provenance)
	}
	if first.Provenance.Bytes != len(input) || first.Provenance.Documents != 1 || first.Provenance.ObjectCount != 1 {
		t.Fatalf("unexpected provenance counts: %#v", first.Provenance)
	}
	if first.Object.GetAPIVersion() != "apps/v1" || first.Object.GetKind() != "Deployment" ||
		first.Object.GetNamespace() != "prod" || first.Object.GetName() != "api" {
		t.Fatalf("wrong object selected: %#v", first.Object.Object)
	}
	managedFields, found, err := unstructured.NestedSlice(first.Object.Object, "metadata", "managedFields")
	manager := ""
	if found && len(managedFields) == 1 {
		if entry, ok := managedFields[0].(map[string]interface{}); ok {
			manager, _ = entry["manager"].(string)
		}
	}
	if err != nil || !found || manager != "controller.example" {
		t.Fatalf("raw managedFields were not retained: %#v, found=%v err=%v", managedFields, found, err)
	}
	first.Object.Object["metadata"].(map[string]interface{})["name"] = "mutated"
	if second.Object.GetName() != "api" {
		t.Fatal("results unexpectedly share mutable object storage")
	}
}

func TestLoadRecordedObjectSupportsJSONAndGenericList(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "json",
			input: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"prod"},"spec":{"replicas":2}}`,
		},
		{
			name:  "list",
			input: "",
		},
	}
	// Keep the List fixture readable and assert its item identity is parsed as a
	// normal Kubernetes object, without inferring any live or capture metadata.
	tests[1].input = "apiVersion: v1\nkind: List\nitems:\n  - " + strings.ReplaceAll(recordedDeployment, "\n", "\n    ")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadRecordedObject(strings.NewReader(tt.input), recordedDeploymentIdentity)
			if err != nil {
				t.Fatal(err)
			}
			if got.Object.GetName() != "api" || got.Object.GetNamespace() != "prod" {
				t.Fatalf("unexpected selected object: %#v", got.Object.Object)
			}
		})
	}
}

func TestLoadRecordedObjectAllowsExplicitEmptyClusterNamespace(t *testing.T) {
	input := `{"apiVersion":"v1","kind":"Node","metadata":{"name":"worker","namespace":""}}`
	want := recordedObjectIdentity{APIVersion: "v1", Kind: "Node", Namespace: "", Name: "worker"}
	got, err := loadRecordedObject(strings.NewReader(input), want)
	if err != nil {
		t.Fatal(err)
	}
	if got.Object.GetNamespace() != "" || got.Object.GetName() != "worker" {
		t.Fatalf("unexpected cluster-scoped identity: %#v", got.Object.Object)
	}
}

func TestLoadRecordedObjectPreservesKubernetesIntegerTypesAndPrecision(t *testing.T) {
	const large = int64(9007199254740993) // one above the exact IEEE-754 integer range
	input := fmt.Sprintf(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","namespace":"prod","generation":%d},"spec":{"replicas":%d},"status":{"replicas":%d,"readyReplicas":%d}}`, large, large, large, large)
	var legacy map[string]interface{}
	if err := json.Unmarshal([]byte(input), &legacy); err != nil {
		t.Fatal(err)
	}
	if got, found, err := unstructured.NestedInt64(legacy, "status", "readyReplicas"); err == nil || found || got != 0 {
		t.Fatalf("legacy encoding/json map unexpectedly supplied a Kubernetes int64: got=%d found=%v err=%v", got, found, err)
	}
	got, err := loadRecordedObject(strings.NewReader(input), recordedDeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range [][]string{{"metadata", "generation"}, {"spec", "replicas"}, {"status", "replicas"}, {"status", "readyReplicas"}} {
		value, found, err := unstructured.NestedInt64(got.Object.Object, path...)
		if err != nil || !found || value != large {
			t.Fatalf("%v integer = %d found=%v err=%v, want exact int64 %d", path, value, found, err, large)
		}
	}
	yamlInput := fmt.Sprintf("apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: prod\n  generation: %d\nspec:\n  replicas: %d\nstatus:\n  replicas: %d\n  readyReplicas: %d\n", large, large, large, large)
	yamlGot, err := loadRecordedObject(strings.NewReader(yamlInput), recordedDeploymentIdentity)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range [][]string{{"metadata", "generation"}, {"spec", "replicas"}, {"status", "replicas"}, {"status", "readyReplicas"}} {
		value, found, err := unstructured.NestedInt64(yamlGot.Object.Object, path...)
		if err != nil || !found || value != large {
			t.Fatalf("YAML %v integer = %d found=%v err=%v, want exact int64 %d", path, value, found, err, large)
		}
	}
}

func TestLoadRecordedObjectRejectsAbsentAmbiguousAndMalformedEvidence(t *testing.T) {
	otherNamespace := strings.Replace(recordedDeployment, "namespace: prod", "namespace: staging", 1)
	prefixName := strings.Replace(recordedDeployment, "name: api", "name: api-canary", 1)
	double := recordedDeployment + "---\n" + recordedDeployment
	badAfterMatch := recordedDeployment + "---\nnot: [valid"
	malformedUnrelated := recordedDeployment + "---\n" + strings.Replace(recordedDeployment, "  name: api", "  name: ' other'", 1)
	duplicateYAML := strings.Replace(recordedDeployment, "  namespace: prod", "  namespace: prod\n  namespace: prod", 1)
	duplicateJSON := `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"api","name":"api","namespace":"prod"}}`
	numericIdentity := strings.Replace(recordedDeployment, "  name: api", "  name: 7", 1)
	missingNamespace := strings.Replace(recordedDeployment, "  namespace: prod\n", "", 1)
	numericAPIVersion := strings.Replace(recordedDeployment, "apiVersion: apps/v1", "apiVersion: 4", 1)
	numericKind := strings.Replace(recordedDeployment, "kind: Deployment", "kind: 5", 1)
	numericNamespace := strings.Replace(recordedDeployment, "namespace: prod", "namespace: 7", 1)
	aliasIdentity := "apiVersion: apps/v1\nkind: Deployment\nmetadata: &meta\n  name: api\n  namespace: prod\nother: *meta\n"
	nestedList := "apiVersion: v1\nkind: List\nitems:\n  - apiVersion: v1\n    kind: List\n    items: []\n"
	invalidListItem := "apiVersion: v1\nkind: List\nitems:\n  - apiVersion: apps/v1\n    kind: Deployment\n    metadata: {}\n"
	tests := []struct {
		name  string
		input string
	}{
		{"wrong namespace", otherNamespace},
		{"prefix name", prefixName},
		{"ambiguous duplicate documents", double},
		{"malformed later document", badAfterMatch},
		{"malformed unrelated document", malformedUnrelated},
		{"duplicate yaml key", duplicateYAML},
		{"duplicate json key", duplicateJSON},
		{"numeric name scalar", numericIdentity},
		{"missing namespace identity", missingNamespace},
		{"numeric apiVersion scalar", numericAPIVersion},
		{"numeric kind scalar", numericKind},
		{"numeric namespace scalar", numericNamespace},
		{"alias", aliasIdentity},
		{"nested List", nestedList},
		{"invalid List item", invalidListItem},
		{"non-object document", "- not-an-object\n"},
		{"null document", "null\n"},
		{"unsupported list apiVersion", "apiVersion: apps/v1\nkind: List\nitems: []\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadRecordedObject(strings.NewReader(tt.input), recordedDeploymentIdentity)
			if err == nil {
				t.Fatalf("expected explicit failure, got %#v", got)
			}
		})
	}
}

func TestLoadRecordedObjectRejectsWhitespaceIdentity(t *testing.T) {
	inputs := []struct {
		name string
		want recordedObjectIdentity
	}{
		{"apiVersion", recordedObjectIdentity{APIVersion: " apps/v1", Kind: "Deployment", Namespace: "prod", Name: "api"}},
		{"kind", recordedObjectIdentity{APIVersion: "apps/v1", Kind: "Deployment ", Namespace: "prod", Name: "api"}},
		{"name", recordedObjectIdentity{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "prod", Name: " "}},
		{"namespace", recordedObjectIdentity{APIVersion: "apps/v1", Kind: "Deployment", Namespace: " prod", Name: "api"}},
	}
	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := loadRecordedObject(strings.NewReader(recordedDeployment), tt.want); err == nil {
				t.Fatal("expected malformed request identity to fail")
			}
		})
	}

	for _, tt := range []struct {
		name  string
		input string
	}{
		{"whitespace apiVersion", strings.Replace(recordedDeployment, "apps/v1", "' apps/v1'", 1)},
		{"whitespace kind", strings.Replace(recordedDeployment, "Deployment", "'Deployment '", 1)},
		{"whitespace name", strings.Replace(recordedDeployment, "name: api", "name: ' api'", 1)},
		{"whitespace namespace", strings.Replace(recordedDeployment, "namespace: prod", "namespace: 'prod '", 1)},
	} {
		t.Run("source "+tt.name, func(t *testing.T) {
			if _, err := loadRecordedObject(strings.NewReader(tt.input), recordedDeploymentIdentity); err == nil {
				t.Fatal("expected malformed source identity to fail")
			}
		})
	}
}

func TestLoadRecordedObjectEnforcesByteDocumentObjectAndDepthBounds(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "byte limit",
			input: strings.Repeat(" ", maxRecordedObjectBytes+1),
		},
		{
			name:  "document limit",
			input: strings.Repeat("---\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: other\n  namespace: prod\n", maxRecordedObjectDocuments+1),
		},
		{
			name:  "object limit",
			input: fmt.Sprintf("apiVersion: v1\nkind: List\nitems:\n%s", strings.Repeat("  - apiVersion: apps/v1\n    kind: Deployment\n    metadata:\n      name: other\n      namespace: prod\n", maxRecordedObjectCount+1)),
		},
		{
			name:  "nesting limit",
			input: "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: other\n  namespace: prod\nspec:\n" + strings.Repeat("  nested:\n", maxRecordedObjectDepth+1) + "    leaf: value\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := loadRecordedObject(strings.NewReader(tt.input), recordedDeploymentIdentity); err == nil {
				t.Fatal("expected bounded parser failure")
			}
		})
	}
}

func TestLoadRecordedObjectErrorsDoNotExposeSecretPayload(t *testing.T) {
	secret := "very-sensitive-secret-payload"
	input := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: private\n  namespace: prod\ndata:\n  password: " + secret + "\n"
	_, err := loadRecordedObject(strings.NewReader(input), recordedDeploymentIdentity)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("expected payload-safe error, got %v", err)
	}
}
