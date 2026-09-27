// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"testing"
)

// #635: map list --summary counts entries by owner and kind after filters.
func TestBuildMapListSummary(t *testing.T) {
	entries := []MapEntry{
		{Kind: "Deployment", Namespace: "a", Name: "x", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "a", Name: "y", Owner: "Flux"},
		{Kind: "Deployment", Namespace: "b", Name: "z", Owner: ""},
		{Kind: "ConfigMap", Namespace: "a", Name: "c", Owner: "Native"},
	}
	got := buildMapListSummary(entries)
	want := MapListSummary{
		Total:   4,
		ByOwner: map[string]int{"Flux": 2, "Native": 2},
		ByKind:  map[string]int{"Deployment": 3, "ConfigMap": 1},
		ByKindOwner: map[string]map[string]int{
			"Deployment": {"Flux": 2, "Native": 1},
			"ConfigMap":  {"Native": 1},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summary = %+v\nwant %+v", got, want)
	}
	if text, wantText := renderMapListSummary(got), "Total: 4\nConfigMap (1): Native=1\nDeployment (3): Flux=2 Native=1\n"; text != wantText {
		t.Errorf("text = %q, want %q", text, wantText)
	}
}
