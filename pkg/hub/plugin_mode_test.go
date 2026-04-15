// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hub

import (
	"testing"
)

func TestIsPluginMode_EnvNotSet(t *testing.T) {
	t.Setenv(envCubPlugin, "")
	if IsPluginMode() {
		t.Error("IsPluginMode() = true, want false when CUB_PLUGIN is unset")
	}
}

func TestIsPluginMode_EnvSet(t *testing.T) {
	t.Setenv(envCubPlugin, "1")
	if !IsPluginMode() {
		t.Error("IsPluginMode() = false, want true when CUB_PLUGIN=1")
	}
}

func TestIsPluginMode_EnvWhitespace(t *testing.T) {
	// Whitespace-only CUB_PLUGIN is ignored, matching TrimSpace semantics.
	t.Setenv(envCubPlugin, "   ")
	if IsPluginMode() {
		t.Error("IsPluginMode() = true, want false for whitespace-only CUB_PLUGIN")
	}
}

func TestPluginToken_EnvSet(t *testing.T) {
	t.Setenv(envCubToken, "hostprovided-token")
	if got := PluginToken(); got != "hostprovided-token" {
		t.Errorf("PluginToken() = %q, want %q", got, "hostprovided-token")
	}
}

func TestPluginToken_EnvUnset(t *testing.T) {
	t.Setenv(envCubToken, "")
	if got := PluginToken(); got != "" {
		t.Errorf("PluginToken() = %q, want empty string", got)
	}
}

func TestPluginAccessors_TrimWhitespace(t *testing.T) {
	t.Setenv(envCubServer, "  https://hub.example.com  ")
	t.Setenv(envCubContext, "  prod  ")
	t.Setenv(envCubSpace, "  platform  ")
	if got := PluginServer(); got != "https://hub.example.com" {
		t.Errorf("PluginServer() = %q, want trimmed value", got)
	}
	if got := PluginContext(); got != "prod" {
		t.Errorf("PluginContext() = %q, want trimmed value", got)
	}
	if got := PluginSpace(); got != "platform" {
		t.Errorf("PluginSpace() = %q, want trimmed value", got)
	}
}

func TestIsAuthenticated_PluginTokenOverride(t *testing.T) {
	// When running as a cub plugin, a non-empty CUB_TOKEN is sufficient for
	// IsAuthenticated to report true, even when no local auth.json exists.
	t.Setenv(envCubPlugin, "1")
	t.Setenv(envCubToken, "host-token")

	// Use a throwaway HOME so LoadAuth cannot accidentally find a real token.
	t.Setenv("HOME", t.TempDir())

	if !IsAuthenticated() {
		t.Error("IsAuthenticated() = false, want true when CUB_PLUGIN=1 and CUB_TOKEN is set")
	}
}

func TestIsAuthenticated_PluginNoToken(t *testing.T) {
	// CUB_PLUGIN set but CUB_TOKEN empty should NOT short-circuit; it should
	// fall through to the local auth.json check, which is empty here.
	t.Setenv(envCubPlugin, "1")
	t.Setenv(envCubToken, "")
	t.Setenv("HOME", t.TempDir())

	if IsAuthenticated() {
		t.Error("IsAuthenticated() = true, want false when CUB_PLUGIN=1 but CUB_TOKEN is empty and no local auth exists")
	}
}

func TestCubCLIAuthenticated_PluginMode(t *testing.T) {
	// In plugin mode, CubCLIAuthenticated must read CUB_TOKEN from the env
	// rather than shelling out to `cub auth get-token`, because re-execing
	// cub from inside a cub plugin would recurse.
	t.Setenv(envCubPlugin, "1")
	t.Setenv(envCubToken, "host-token")
	// Guarantee that even if the real cub were on PATH, plugin mode bypasses it.
	t.Setenv("PATH", t.TempDir())

	if !CubCLIAuthenticated() {
		t.Error("CubCLIAuthenticated() = false, want true in plugin mode with CUB_TOKEN set")
	}
}

func TestCubCLIAuthenticated_PluginModeNoToken(t *testing.T) {
	t.Setenv(envCubPlugin, "1")
	t.Setenv(envCubToken, "")
	t.Setenv("PATH", t.TempDir())

	if CubCLIAuthenticated() {
		t.Error("CubCLIAuthenticated() = true, want false in plugin mode with CUB_TOKEN empty")
	}
}
