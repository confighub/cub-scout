package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTelemetryDisabled_EnvVar(t *testing.T) {
	// Use a temp directory (no config file)
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	// Without env var, telemetry should be enabled
	t.Setenv("CUB_SCOUT_TELEMETRY", "")
	if telemetryDisabled() {
		t.Error("expected telemetry enabled when env var not set")
	}

	// With env var set to false, telemetry should be disabled
	t.Setenv("CUB_SCOUT_TELEMETRY", "false")
	if !telemetryDisabled() {
		t.Error("expected telemetry disabled when CUB_SCOUT_TELEMETRY=false")
	}

	// With env var set to anything else, telemetry should be enabled
	t.Setenv("CUB_SCOUT_TELEMETRY", "true")
	if telemetryDisabled() {
		t.Error("expected telemetry enabled when CUB_SCOUT_TELEMETRY=true")
	}
}

func TestTelemetryDisabled_ConfigFile(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("CUB_SCOUT_TELEMETRY", "") // Clear env var
	defer func() { os.Setenv("HOME", origHome) }()

	// Without config file, telemetry should be enabled
	if telemetryDisabled() {
		t.Error("expected telemetry enabled when no config file")
	}

	// Create telemetry-disabled file
	configDir := filepath.Join(tmpDir, ".cub-scout")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "telemetry-disabled")
	if err := os.WriteFile(configPath, []byte("disabled"), 0644); err != nil {
		t.Fatal(err)
	}

	// With config file, telemetry should be disabled
	if !telemetryDisabled() {
		t.Error("expected telemetry disabled when config file exists")
	}
}

func TestDisableTelemetry(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("CUB_SCOUT_TELEMETRY", "")
	defer func() { os.Setenv("HOME", origHome) }()

	// Initially enabled
	if !TelemetryEnabled() {
		t.Error("expected telemetry enabled initially")
	}

	// Disable
	if err := DisableTelemetry(); err != nil {
		t.Fatalf("DisableTelemetry() error = %v", err)
	}

	// Now disabled
	if TelemetryEnabled() {
		t.Error("expected telemetry disabled after DisableTelemetry()")
	}

	// Verify file exists
	configPath := filepath.Join(tmpDir, ".cub-scout", "telemetry-disabled")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("expected telemetry-disabled file to exist")
	}
}

func TestEnableTelemetry(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("CUB_SCOUT_TELEMETRY", "")
	defer func() { os.Setenv("HOME", origHome) }()

	// Disable first
	if err := DisableTelemetry(); err != nil {
		t.Fatal(err)
	}
	if TelemetryEnabled() {
		t.Fatal("telemetry should be disabled")
	}

	// Enable
	if err := EnableTelemetry(); err != nil {
		t.Fatalf("EnableTelemetry() error = %v", err)
	}

	// Now enabled
	if !TelemetryEnabled() {
		t.Error("expected telemetry enabled after EnableTelemetry()")
	}

	// Verify file removed
	configPath := filepath.Join(tmpDir, ".cub-scout", "telemetry-disabled")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("expected telemetry-disabled file to be removed")
	}
}

func TestEnableTelemetry_NoFile(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	// Enable when already enabled (no file) should not error
	if err := EnableTelemetry(); err != nil {
		t.Fatalf("EnableTelemetry() should not error when already enabled, got %v", err)
	}
}

func TestSendStartupPing_NoOp(t *testing.T) {
	// SendStartupPing is intentionally a no-op in v1.0
	// Just verify it doesn't error
	if err := SendStartupPing(); err != nil {
		t.Errorf("SendStartupPing() error = %v", err)
	}
}
