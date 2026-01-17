// Package hub provides the connection library for ConfigHub integration.
// This is the SINGLE SOURCE OF TRUTH for all ConfigHub connectivity.
package hub

// Mode represents the current operating mode of cub-scout.
type Mode int

const (
	// Offline means no internet connectivity.
	// All discovery/mapping features work, but no telemetry or ConfigHub features.
	Offline Mode = iota

	// Online means internet is available but user is not authenticated.
	// Telemetry ping on startup (opt-out). ConfigHub features show "noop" message.
	Online

	// Connected means user is authenticated with ConfigHub.
	// All features available based on subscription tier.
	Connected
)

// String returns the mode name.
func (m Mode) String() string {
	switch m {
	case Offline:
		return "offline"
	case Online:
		return "online"
	case Connected:
		return "connected"
	default:
		return "unknown"
	}
}

// CurrentMode returns the current operating mode.
// It checks connectivity and authentication state.
func CurrentMode() Mode {
	// Check if user has disabled network or we're air-gapped
	if !hasConnectivity() {
		return Offline
	}

	// Check if user is authenticated
	if IsAuthenticated() {
		return Connected
	}

	return Online
}

// hasConnectivity checks if we can reach the internet.
func hasConnectivity() bool {
	// TODO: Implement connectivity check
	// For now, assume online unless explicitly disabled
	return !telemetryDisabled()
}
