package hub

import (
	"os"
	"path/filepath"
)

// Telemetry configuration.
// Telemetry is OPT-OUT: enabled by default, user can disable.

// TelemetryEnabled returns true if telemetry is enabled.
func TelemetryEnabled() bool {
	return !telemetryDisabled()
}

// telemetryDisabled checks if user has opted out of telemetry.
func telemetryDisabled() bool {
	// Check environment variable
	if os.Getenv("CUB_SCOUT_TELEMETRY") == "false" {
		return true
	}

	// Check config file - presence of file means disabled
	// (filename is "telemetry-disabled", created by DisableTelemetry())
	configPath := telemetryConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		return true // File exists = telemetry disabled
	}

	return false
}

// DisableTelemetry opts out of telemetry.
func DisableTelemetry() error {
	configPath := telemetryConfigPath()
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(configPath, []byte("disabled"), 0644)
}

// EnableTelemetry opts back in to telemetry.
func EnableTelemetry() error {
	configPath := telemetryConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(configPath)
}

// telemetryConfigPath returns the path to the telemetry config file.
func telemetryConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cub-scout", "telemetry-disabled")
}

// SendStartupPing sends a telemetry ping on startup.
// Only called if telemetry is enabled and we're in Online or Connected mode.
//
// Current behavior (v1.0): No-op. Telemetry infrastructure is not yet deployed.
// When implemented, this will send only:
//   - Anonymous session ID (rotated daily)
//   - cub-scout version
//   - OS/arch
//
// It will NEVER send:
//   - Cluster names or data
//   - Resource information
//   - User identity (unless authenticated)
//
// Users can opt out via CUB_SCOUT_TELEMETRY=false or `cub-scout telemetry disable`.
func SendStartupPing() error {
	if !TelemetryEnabled() {
		return nil
	}

	if CurrentMode() == Offline {
		return nil
	}

	// v1.0: Telemetry endpoint not yet deployed - this is intentionally a no-op.
	// The infrastructure (TelemetryEnabled, telemetryDisabled, etc.) is in place
	// so users can pre-emptively opt out before telemetry goes live.
	return nil
}
