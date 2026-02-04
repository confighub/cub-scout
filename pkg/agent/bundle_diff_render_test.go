// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
	"testing"
	"time"
)

func TestRenderBundleDiffDeterministic(t *testing.T) {
	diff := &BundleDiff{
		SchemaVersion: BundleDiffSchemaVersion,
		From: BundleRef{
			ID:        "before",
			CreatedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		},
		To: BundleRef{
			ID:        "after",
			CreatedAt: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
		},
		JoinMode: JoinObjectID,
		Summary: DiffSummary{
			Changed:   1,
			Unchanged: 2,
		},
		Objects: []ObjectDiff{
			{Identity: "Deployment:default/alpha", Status: StatusUnchanged, Sections: []SectionDiff{{Name: "drift", Status: SectionUnchanged}, {Name: "correlation", Status: SectionUnchanged}}},
			{Identity: "Deployment:default/beta", Status: StatusChanged, Sections: []SectionDiff{{Name: "drift", Status: SectionChanged, Counts: &SectionCounts{Before: 2, After: 5, Delta: 3}}, {Name: "correlation", Status: SectionUnchanged}}},
			{Identity: "Deployment:default/gamma", Status: StatusUnchanged, Sections: []SectionDiff{{Name: "drift", Status: SectionUnchanged}, {Name: "correlation", Status: SectionUnchanged}}},
		},
	}

	// Render multiple times and verify identical output
	first := RenderBundleDiffASCII(diff)
	for i := 0; i < 5; i++ {
		output := RenderBundleDiffASCII(diff)
		if output != first {
			t.Errorf("Iteration %d: output differs from first render", i)
		}
	}
}

func TestRenderBundleDiffIncludesSummary(t *testing.T) {
	diff := &BundleDiff{
		SchemaVersion: BundleDiffSchemaVersion,
		From: BundleRef{
			ID:        "before",
			CreatedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		},
		To: BundleRef{
			ID:        "after",
			CreatedAt: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
		},
		JoinMode: JoinObjectID,
		Summary: DiffSummary{
			Added:     1,
			Removed:   2,
			Changed:   3,
			Unchanged: 4,
		},
		Objects: []ObjectDiff{},
	}

	output := RenderBundleDiffASCII(diff)

	// Verify summary is present
	if !strings.Contains(output, "Summary") {
		t.Error("Output missing 'Summary' header")
	}
	if !strings.Contains(output, "added=1") {
		t.Error("Output missing 'added=1'")
	}
	if !strings.Contains(output, "removed=2") {
		t.Error("Output missing 'removed=2'")
	}
	if !strings.Contains(output, "changed=3") {
		t.Error("Output missing 'changed=3'")
	}
	if !strings.Contains(output, "unchanged=4") {
		t.Error("Output missing 'unchanged=4'")
	}
}

func TestRenderBundleDiffHeader(t *testing.T) {
	diff := &BundleDiff{
		SchemaVersion: BundleDiffSchemaVersion,
		From: BundleRef{
			ID:        "bundle-before",
			CreatedAt: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
		},
		To: BundleRef{
			ID:        "bundle-after",
			CreatedAt: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC),
		},
		JoinMode: JoinObjectID,
		Summary:  DiffSummary{},
		Objects:  []ObjectDiff{},
	}

	output := RenderBundleDiffASCII(diff)

	if !strings.Contains(output, "Bundle Diff") {
		t.Error("Output missing 'Bundle Diff' header")
	}
	if !strings.Contains(output, "From: bundle-before") {
		t.Error("Output missing 'From: bundle-before'")
	}
	if !strings.Contains(output, "To:   bundle-after") {
		t.Error("Output missing 'To:   bundle-after'")
	}
	if !strings.Contains(output, "Join: object_id") {
		t.Error("Output missing 'Join: object_id'")
	}
}

func TestRenderBundleDiffObjectIcons(t *testing.T) {
	diff := &BundleDiff{
		SchemaVersion: BundleDiffSchemaVersion,
		From:          BundleRef{ID: "a", CreatedAt: time.Now()},
		To:            BundleRef{ID: "b", CreatedAt: time.Now()},
		JoinMode:      JoinObjectID,
		Summary:       DiffSummary{Added: 1, Removed: 1, Changed: 1, Unchanged: 1},
		Objects: []ObjectDiff{
			{Identity: "added-obj", Status: StatusAdded, Sections: []SectionDiff{{Name: "drift", Status: SectionMissingFrom}}},
			{Identity: "changed-obj", Status: StatusChanged, Sections: []SectionDiff{{Name: "drift", Status: SectionChanged}}},
			{Identity: "removed-obj", Status: StatusRemoved, Sections: []SectionDiff{{Name: "drift", Status: SectionMissingTo}}},
			{Identity: "unchanged-obj", Status: StatusUnchanged, Sections: []SectionDiff{{Name: "drift", Status: SectionUnchanged}}},
		},
	}

	output := RenderBundleDiffASCII(diff)

	// Check icons are present
	if !strings.Contains(output, "+ added-obj") {
		t.Error("Output missing '+ added-obj' icon")
	}
	if !strings.Contains(output, "~ changed-obj") {
		t.Error("Output missing '~ changed-obj' icon")
	}
	if !strings.Contains(output, "- removed-obj") {
		t.Error("Output missing '- removed-obj' icon")
	}
	if !strings.Contains(output, "= unchanged-obj") {
		t.Error("Output missing '= unchanged-obj' icon")
	}
}

func TestRenderBundleDiffNoObjects(t *testing.T) {
	diff := &BundleDiff{
		SchemaVersion: BundleDiffSchemaVersion,
		From:          BundleRef{ID: "a", CreatedAt: time.Now()},
		To:            BundleRef{ID: "b", CreatedAt: time.Now()},
		JoinMode:      JoinObjectID,
		Summary:       DiffSummary{},
		Objects:       []ObjectDiff{},
	}

	output := RenderBundleDiffASCII(diff)

	if !strings.Contains(output, "No objects to compare") {
		t.Error("Output should indicate no objects to compare")
	}
}
