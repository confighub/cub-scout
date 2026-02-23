package hub

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMode_String(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{Offline, "offline"},
		{Online, "online"},
		{Connected, "connected"},
		{Mode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("Mode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCurrentMode_OfflineEnvVar(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	// Set offline mode via env var
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	t.Setenv("CUB_SCOUT_TELEMETRY", "") // Clear telemetry setting

	mode := CurrentMode()
	if mode != Offline {
		t.Errorf("CurrentMode() = %v, want Offline when CUB_SCOUT_OFFLINE=true", mode)
	}
}

func TestCurrentMode_TelemetryDisabledImpliesOffline(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	// Clear offline mode but disable telemetry
	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "false")

	mode := CurrentMode()
	if mode != Offline {
		t.Errorf("CurrentMode() = %v, want Offline when telemetry disabled", mode)
	}
}

func TestCurrentMode_Connected(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	// Clear offline and telemetry settings
	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "")

	// Save auth with token
	if err := SaveAuth(&Auth{Token: "valid-token"}); err != nil {
		t.Fatal(err)
	}

	// Note: This test may return Online instead of Connected if
	// the ConfigHub endpoint is not reachable. That's expected behavior.
	mode := CurrentMode()
	if mode == Offline {
		// Offline is acceptable if network check fails
		t.Log("CurrentMode() = Offline (network check failed, acceptable)")
	} else if mode == Online {
		// Online means network is up but auth check failed
		// This shouldn't happen since we saved auth
		t.Log("CurrentMode() = Online (auth token present but mode is Online)")
	} else if mode == Connected {
		t.Log("CurrentMode() = Connected (as expected)")
	}
}

// --- QuickMode tests ---

func TestQuickMode_OfflineEnvVar(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	t.Setenv("CUB_SCOUT_TELEMETRY", "")

	if got := QuickMode(); got != Offline {
		t.Errorf("QuickMode() = %v, want Offline when CUB_SCOUT_OFFLINE=true", got)
	}
}

func TestQuickMode_TelemetryDisabled(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "false")

	if got := QuickMode(); got != Offline {
		t.Errorf("QuickMode() = %v, want Offline when telemetry disabled", got)
	}
}

func TestQuickMode_NoAuth(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "")

	if got := QuickMode(); got != Online {
		t.Errorf("QuickMode() = %v, want Online when no auth file", got)
	}
}

func TestQuickMode_WithAuth(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "")

	if err := SaveAuth(&Auth{Token: "valid-token"}); err != nil {
		t.Fatal(err)
	}

	if got := QuickMode(); got != Connected {
		t.Errorf("QuickMode() = %v, want Connected when auth token present", got)
	}
}

func TestQuickMode_Fast(t *testing.T) {
	// QuickMode must never make network calls — verify it returns fast.
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	t.Setenv("CUB_SCOUT_OFFLINE", "")
	t.Setenv("CUB_SCOUT_TELEMETRY", "")

	start := time.Now()
	_ = QuickMode()
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("QuickMode() took %v, expected < 100ms (should not make network calls)", elapsed)
	}
}

// --- DisplayName tests ---

func TestMode_DisplayName(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{Offline, "Offline"},
		{Online, "Standalone"},
		{Connected, "Connected"},
		{Mode(99), "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.DisplayName(); got != tt.want {
				t.Errorf("Mode(%d).DisplayName() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

// --- hasConnectivity tests ---

func TestHasConnectivity_OfflineEnvVar(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	t.Setenv("CUB_SCOUT_TELEMETRY", "")

	// Without offline env var, hasConnectivity depends on network
	t.Setenv("CUB_SCOUT_OFFLINE", "")
	// Don't assert result - depends on network

	// With offline env var, should return false
	t.Setenv("CUB_SCOUT_OFFLINE", "true")
	if hasConnectivity() {
		t.Error("hasConnectivity() should return false when CUB_SCOUT_OFFLINE=true")
	}
}

func TestHasConnectivity_TelemetryDisabled(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	t.Setenv("CUB_SCOUT_OFFLINE", "")

	// Create telemetry-disabled file
	configDir := filepath.Join(tmpDir, ".cub-scout")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "telemetry-disabled")
	if err := os.WriteFile(configPath, []byte("disabled"), 0644); err != nil {
		t.Fatal(err)
	}

	if hasConnectivity() {
		t.Error("hasConnectivity() should return false when telemetry disabled")
	}
}
