// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseLinkListJSON(t *testing.T) {
	raw := []byte(`[
		{
			"LinkID": "link-1",
			"Slug": "image-from-app",
			"DisplayName": "Image From App",
			"UpdateType": "NeedsProvides",
			"FromUnitID": "unit-downstream",
			"ToUnitID": "unit-upstream-image",
			"ToSpaceID": "space-A",
			"AutoUpdate": true,
			"WhereResource": "kind=Deployment",
			"Bindings": [{"foo":"bar"},{"x":"y"},{"a":"b"}]
		},
		{
			"LinkID": "link-2",
			"Slug": "upgrade",
			"UpdateType": "UpgradeUnits",
			"FromUnitID": "unit-downstream",
			"ToUnitID": "unit-upstream-base",
			"AutoUpdate": false,
			"Bindings": {}
		}
	]`)

	got := parseLinkListJSON(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}

	if got[0].LinkID != "link-1" || got[0].Slug != "image-from-app" {
		t.Errorf("entry 0 mismatch: %+v", got[0])
	}
	if got[0].UpdateType != "NeedsProvides" || got[0].ToUnitID != "unit-upstream-image" {
		t.Errorf("entry 0 type/upstream mismatch: %+v", got[0])
	}
	if !got[0].AutoUpdate {
		t.Errorf("entry 0 AutoUpdate should be true; got %+v", got[0])
	}
	if got[0].BindingsCount != 3 {
		t.Errorf("entry 0 BindingsCount = %d, want 3", got[0].BindingsCount)
	}
	if got[0].WhereResource != "kind=Deployment" {
		t.Errorf("entry 0 WhereResource = %q", got[0].WhereResource)
	}

	if got[1].LinkID != "link-2" || got[1].UpdateType != "UpgradeUnits" {
		t.Errorf("entry 1 mismatch: %+v", got[1])
	}
	if got[1].AutoUpdate {
		t.Errorf("entry 1 AutoUpdate should be false; got %+v", got[1])
	}
	// Empty {} bindings object → count zero
	if got[1].BindingsCount != 0 {
		t.Errorf("entry 1 BindingsCount = %d, want 0", got[1].BindingsCount)
	}
}

func TestParseLinkListJSON_EmptyAndInvalid(t *testing.T) {
	if got := parseLinkListJSON([]byte(`[]`)); got != nil {
		t.Errorf("empty array → expected nil, got %v", got)
	}
	if got := parseLinkListJSON([]byte(`not json`)); got != nil {
		t.Errorf("invalid JSON → expected nil, got %v", got)
	}
	if got := parseLinkListJSON(nil); got != nil {
		t.Errorf("nil input → expected nil, got %v", got)
	}
}

func TestCollectIncomingBindings_RoundTrip(t *testing.T) {
	// Inject a stub runner that records args and returns canned JSON.
	var capturedArgs []string
	saved := compareLinkRunner
	defer func() { compareLinkRunner = saved }()
	compareLinkRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		capturedArgs = args
		return []byte(`[{"LinkID":"L","Slug":"s","UpdateType":"Upsert","ToUnitID":"upstream"}]`), nil
	}

	got := collectIncomingBindings(context.Background(), "unit-A", "space-X")
	if len(got) != 1 {
		t.Fatalf("expected 1 binding; got %d", len(got))
	}
	if got[0].UpdateType != "Upsert" || got[0].ToUnitID != "upstream" {
		t.Errorf("bad binding decode: %+v", got[0])
	}

	// Verify the cub command shape — link list with FromUnitID filter.
	joined := strings.Join(capturedArgs, " ")
	for _, expect := range []string{"link", "list", "--space", "space-X", "--where", "FromUnitID = 'unit-A'", "-o", "json", "--quiet"} {
		if !strings.Contains(joined, expect) {
			t.Errorf("expected %q in args; got %q", expect, joined)
		}
	}
}

func TestCollectIncomingBindings_EmptyUnitID(t *testing.T) {
	// Empty unitID short-circuits to nil without calling the runner.
	called := false
	saved := compareLinkRunner
	defer func() { compareLinkRunner = saved }()
	compareLinkRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		called = true
		return nil, nil
	}

	got := collectIncomingBindings(context.Background(), "", "space-X")
	if got != nil {
		t.Errorf("expected nil; got %v", got)
	}
	if called {
		t.Errorf("runner should not have been invoked for empty unitID")
	}
}

func TestCollectIncomingBindings_DefaultSpaceWildcard(t *testing.T) {
	// Empty space defaults to "*" so cross-space links are reachable.
	var capturedArgs []string
	saved := compareLinkRunner
	defer func() { compareLinkRunner = saved }()
	compareLinkRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		capturedArgs = args
		return []byte(`[]`), nil
	}

	_ = collectIncomingBindings(context.Background(), "unit-A", "")
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "--space *") {
		t.Errorf("expected default --space *; got %q", joined)
	}
}

func TestCollectIncomingBindings_RunnerErrorReturnsNil(t *testing.T) {
	saved := compareLinkRunner
	defer func() { compareLinkRunner = saved }()
	compareLinkRunner = func(ctx context.Context, args ...string) ([]byte, error) {
		return nil, errors.New("auth failed")
	}

	got := collectIncomingBindings(context.Background(), "unit-A", "space-X")
	if got != nil {
		t.Errorf("expected nil on runner error; got %v", got)
	}
}

func TestRenderIncomingBindingsASCII(t *testing.T) {
	var b strings.Builder
	renderIncomingBindingsASCII(&b, []IncomingBinding{
		{LinkID: "L1", Slug: "image-from-app", DisplayName: "Image From App", UpdateType: "NeedsProvides", ToUnitID: "u1", AutoUpdate: true, BindingsCount: 3},
		{LinkID: "L2", Slug: "upgrade", UpdateType: "UpgradeUnits", ToUnitID: "u2"},
	})

	out := b.String()
	if !strings.Contains(out, "Incoming Bindings (ConfigHub)") {
		t.Errorf("missing header: %q", out)
	}
	if !strings.Contains(out, "Image From App [NeedsProvides] <- unit:u1 auto-update bindings=3") {
		t.Errorf("entry 1 wrong: %q", out)
	}
	if !strings.Contains(out, "upgrade [UpgradeUnits] <- unit:u2") {
		t.Errorf("entry 2 wrong: %q", out)
	}
}

func TestRenderIncomingBindingsASCII_Empty(t *testing.T) {
	var b strings.Builder
	renderIncomingBindingsASCII(&b, nil)
	if b.Len() != 0 {
		t.Errorf("empty bindings should produce empty output; got %q", b.String())
	}
}

func TestRenderIncomingBindingsMarkdown(t *testing.T) {
	var b strings.Builder
	renderIncomingBindingsMarkdown(&b, []IncomingBinding{
		{LinkID: "L1", Slug: "image-from-app", UpdateType: "NeedsProvides", ToUnitID: "u1", AutoUpdate: true, BindingsCount: 3},
	})
	out := b.String()
	if !strings.Contains(out, "### Incoming Bindings (ConfigHub)") {
		t.Errorf("missing header: %q", out)
	}
	if !strings.Contains(out, "| `image-from-app` | `NeedsProvides` | `u1` | yes | 3 |") {
		t.Errorf("row wrong: %q", out)
	}
}
