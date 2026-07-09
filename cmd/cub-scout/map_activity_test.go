// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestCollectEventActivityIncludesAuditedAction(t *testing.T) {
	oldNamespace, oldOwner := mapNamespace, mapOwner
	t.Cleanup(func() {
		mapNamespace, mapOwner = oldNamespace, oldOwner
	})
	mapNamespace, mapOwner = "", ""

	eventGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{eventGVR: "EventList"},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Event",
			"metadata": map[string]interface{}{
				"name":      "api-action",
				"namespace": "prod",
				"annotations": map[string]interface{}{
					"event.toolkit.fluxcd.io/action":       "restart",
					"event.toolkit.fluxcd.io/username":     "operator@example.com",
					"event.toolkit.fluxcd.io/subject":      "Deployment/prod/api",
					"event.toolkit.fluxcd.io/change-token": "chg-123",
				},
			},
			"involvedObject": map[string]interface{}{
				"kind":      "Deployment",
				"namespace": "prod",
				"name":      "api",
			},
			"type":      "Normal",
			"reason":    "WebAction",
			"message":   "operator@example.com requested restart for Deployment/prod/api",
			"eventTime": "2026-07-09T10:00:00Z",
		}},
	)

	rows := collectEventActivity(context.Background(), client)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Source != "k8s.action" {
		t.Fatalf("Source = %q, want k8s.action", row.Source)
	}
	if row.Action != "restart" {
		t.Fatalf("Action = %q, want restart", row.Action)
	}
	if row.Owner != "Flux" {
		t.Fatalf("Owner = %q, want Flux", row.Owner)
	}
	if row.Actor != "operator@example.com" || row.Subject != "Deployment/prod/api" {
		t.Fatalf("Actor/Subject = %q/%q, want operator@example.com/Deployment/prod/api", row.Actor, row.Subject)
	}
	if row.ActionEvidence["event.toolkit.fluxcd.io/change-token"] != "chg-123" {
		t.Fatalf("ActionEvidence = %#v, want change-token evidence", row.ActionEvidence)
	}
	if !strings.Contains(row.Message, "action=restart") || !strings.Contains(row.Message, "actor=operator@example.com") {
		t.Fatalf("Message = %q, want action detail", row.Message)
	}
}
