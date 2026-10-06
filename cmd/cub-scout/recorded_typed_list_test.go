// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRecordedDeploymentListCapturedIdentityAndSharedProvenance(t *testing.T) {
	raw, err := os.ReadFile("../../evals/rul03-context/fixtures/rul03-readable-deployments.body")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "e0102f91e1417c8554ed377352b50450f2376e69d40632f109ea012c5c1560a2" {
		t.Fatal("source fixture drift")
	}
	snapshot, err := loadRecordedObjectSnapshot(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	identity := recordedObjectIdentity{APIVersion: "apps/v1", Kind: "Deployment", Namespace: "rul03-proof", Name: "rul03-probe"}
	selected, err := snapshot.selectObject(identity)
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	metadata := source.Items[0]["metadata"].(map[string]interface{})
	if string(selected.Object.GetUID()) != metadata["uid"] || selected.Object.GetResourceVersion() != metadata["resourceVersion"] {
		t.Fatal("source object identity changed")
	}
	selectedMetadata, _ := json.Marshal(selected.Object.Object["metadata"])
	sourceMetadata, _ := json.Marshal(metadata)
	if string(selectedMetadata) != string(sourceMetadata) {
		t.Fatal("source metadata/managedFields changed")
	}
	if selected.Provenance.SHA256 != hex.EncodeToString(digest[:]) || selected.Provenance.TypedListDerivedObjects != 1 {
		t.Fatal("missing raw/type provenance")
	}
	report, err := buildRecordedMapReport(snapshot, RecordedMapScope{APIVersion: "apps/v1", Kind: "Deployment"})
	if err != nil {
		t.Fatal(err)
	}
	if report.SelectedCount != 1 || report.Provenance.TypedListDerivedObjects != 1 {
		t.Fatal("incorrect map projection")
	}
	inputObjects := make([]string, maxRecordedObjectCount+1)
	for i := range inputObjects {
		inputObjects[i] = `{"metadata":{"name":"api","namespace":"prod"}}`
	}
	tooMany := `{"apiVersion":"apps/v1","kind":"DeploymentList","items":[` + strings.Join(inputObjects, ",") + `]}`
	if _, err := loadRecordedObjectSnapshot(strings.NewReader(tooMany)); err == nil {
		t.Fatal("typed list bypassed object bound")
	}
	for _, format := range []string{"ascii", "md"} {
		if !strings.Contains(renderRecordedMapReport(report, format), "typed list envelope: 1") {
			t.Fatal("map rendering omitted type source")
		}
	}
	if !strings.Contains(newRecordedMapViewer(report).content, "typed list envelope: 1") {
		t.Fatal("TUI parity")
	}
	summary, err := recordedExplainSummary(snapshot, identity, "")
	if err != nil {
		t.Fatal(err)
	}
	if summary.RecordedInput.TypedListDerivedObjects != 1 || !strings.Contains(formatRecordedInput(summary.RecordedInput), "typed list envelope: 1") {
		t.Fatal("explain provenance")
	}
	tool := recordedMapTool(snapshot)
	args, err := tool.BuildArgs(map[string]interface{}{"api_version": "apps/v1", "kind": "Deployment", "summary": true})
	if err != nil {
		t.Fatal(err)
	}
	output, err := tool.Runner(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	var result RecordedMapSummary
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if result.Provenance.TypedListDerivedObjects != 1 || result.SelectedCount != 1 {
		t.Fatal("MCP parity")
	}
}

func TestRecordedDeploymentListTypeRules(t *testing.T) {
	metadata := `"metadata":{"name":"api","namespace":"prod"}`
	for _, types := range []string{"", `"apiVersion":"apps/v1",`, `"kind":"Deployment",`, `"apiVersion":"apps/v1","kind":"Deployment",`} {
		input := `{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{` + types + metadata + `}]}`
		s, err := loadRecordedObjectSnapshot(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		expected := 1
		if strings.Contains(types, "apiVersion") && strings.Contains(types, "kind") {
			expected = 0
		}
		if s.Provenance.TypedListDerivedObjects != expected {
			t.Fatal("incorrect derivation count")
		}
		if _, err := s.selectObject(recordedDeploymentIdentity); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{
		`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"kind":"StatefulSet",` + metadata + `}]}`,
		`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"apiVersion":"apps/v2",` + metadata + `}]}`,
		`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"kind":null,` + metadata + `}]}`,
		`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"kind":"",` + metadata + `}]}`,
		`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"apiVersion":1,` + metadata + `}]}`,
		`{"apiVersion":"apps/v1","kind":"DeploymentList","items":{}}`,
		`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[]}`,
		`{"apiVersion":"apps/v2","kind":"DeploymentList","items":[{` + metadata + `}]}`,
		`{"apiVersion":"v1","kind":"List","items":[{` + metadata + `}]}`,
		`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[{"kind":"DeploymentList",` + metadata + `}]}`,
		`{"apiVersion":"apps/v1","kind":"UnknownList","items":[{` + metadata + `}]}`,
		`{"apiVersion":"v1","kind":"Status","status":"Failure","reason":"Forbidden","code":403}`,
	} {
		if _, err := loadRecordedObjectSnapshot(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid source: %s", input)
		}
	}
}
