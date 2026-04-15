package hub

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Auth holds user authentication state.
type Auth struct {
	Token    string `json:"token,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	Email    string `json:"email,omitempty"`
	Tier     string `json:"tier,omitempty"` // "free", "pro", "enterprise"
	ExpireAt int64  `json:"expire_at,omitempty"`
}

// IsAuthenticated returns true if user is logged in to ConfigHub.
//
// When running as a `cub` plugin (CUB_PLUGIN=1), this also honours CUB_TOKEN
// from the environment, since the host cub process already owns the active
// credential and has passed it through.
func IsAuthenticated() bool {
	if IsPluginMode() && PluginToken() != "" {
		return true
	}
	auth, err := LoadAuth()
	if err != nil {
		return false
	}
	return auth.Token != ""
}

// LoadAuth loads authentication state from local storage.
func LoadAuth() (*Auth, error) {
	configPath := authConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Auth{}, nil
		}
		return nil, err
	}

	var auth Auth
	if err := json.Unmarshal(data, &auth); err != nil {
		// Corrupted file - return empty auth rather than error
		// User can re-login to fix
		return &Auth{}, nil
	}
	return &auth, nil
}

// SaveAuth saves authentication state to local storage.
func SaveAuth(auth *Auth) error {
	configPath := authConfigPath()
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(auth, "", "  ")
	if err != nil {
		return err
	}

	// Write with restrictive permissions (user-only read/write)
	return os.WriteFile(configPath, data, 0600)
}

// ClearAuth removes authentication state (logout).
func ClearAuth() error {
	configPath := authConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(configPath)
}

// authConfigPath returns the path to the auth config file.
func authConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cub-scout", "auth.json")
}

// CubCLIAuthenticated checks if the cub CLI has a valid auth token.
// This checks the cub CLI's own token store (~/.confighub/tokens/),
// which is separate from cub-scout's local auth.json.
// Returns false if cub is not installed or has no valid token.
// Note: this calls exec.Command so it is NOT suitable for hot paths.
// Use IsAuthenticated() for fast local-file-only checks.
//
// When running as a `cub` plugin (CUB_PLUGIN=1), this reads CUB_TOKEN from
// the environment rather than shelling out to `cub auth get-token`, since
// the host cub process has already provided the token and a recursive exec
// would be both slower and harder to reason about.
func CubCLIAuthenticated() bool {
	if IsPluginMode() {
		return PluginToken() != ""
	}
	out, err := exec.Command("cub", "auth", "get-token").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// IsPaidTier returns true if user has a paid subscription.
func (a *Auth) IsPaidTier() bool {
	return a.Tier == "pro" || a.Tier == "enterprise"
}
