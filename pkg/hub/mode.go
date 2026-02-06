// Package hub provides the connection library for ConfigHub integration.
// This is the SINGLE SOURCE OF TRUTH for all ConfigHub connectivity.
package hub

import (
	"net/http"
	"os"
	"time"
)

const (
	// ConfigHubHealthEndpoint is the endpoint used to check ConfigHub connectivity.
	ConfigHubHealthEndpoint = "https://api.confighub.com/health"

	// ConnectivityTimeout is the maximum time to wait for connectivity check.
	ConnectivityTimeout = 2 * time.Second
)

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

// hasConnectivity checks if we can reach ConfigHub.
// Returns false if:
// - CUB_SCOUT_OFFLINE=true environment variable is set
// - Telemetry is disabled (implies user wants offline mode)
// - HTTP request to ConfigHub health endpoint fails or times out
func hasConnectivity() bool {
	// Check explicit offline mode
	if os.Getenv("CUB_SCOUT_OFFLINE") == "true" {
		return false
	}

	// Check if user has opted out of network activity
	if telemetryDisabled() {
		return false
	}

	// Perform actual connectivity check
	client := &http.Client{
		Timeout: ConnectivityTimeout,
	}

	resp, err := client.Head(ConfigHubHealthEndpoint)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Any 2xx or 3xx response indicates connectivity
	return resp.StatusCode < 400
}
