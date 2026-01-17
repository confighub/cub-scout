package hub

// Configuration for ConfigHub endpoints.
// This file is the SINGLE SOURCE OF TRUTH for all ConfigHub URLs.

const (
	// HubBaseURL is the base URL for ConfigHub API.
	HubBaseURL = "https://hub.confighub.com"

	// WebBaseURL is the base URL for ConfigHub web app.
	WebBaseURL = "https://confighub.com"

	// TelemetryEndpoint is where startup pings are sent.
	TelemetryEndpoint = HubBaseURL + "/v1/telemetry/ping"

	// AuthEndpoint is for authentication.
	AuthEndpoint = HubBaseURL + "/v1/auth"

	// ScanEndpoint is for pattern database access.
	ScanEndpoint = HubBaseURL + "/v1/scan/patterns"

	// RecordEndpoint is for recording discoveries.
	RecordEndpoint = HubBaseURL + "/v1/record"
)

// SignupURL returns the URL for user signup.
func SignupURL() string {
	return WebBaseURL + "/signup"
}

// LoginURL returns the URL for user login.
func LoginURL() string {
	return WebBaseURL + "/login"
}

// DiscordURL returns the Discord invite URL.
func DiscordURL() string {
	return "https://discord.gg/confighub"
}
