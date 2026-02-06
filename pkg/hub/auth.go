package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
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
func IsAuthenticated() bool {
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

// IsPaidTier returns true if user has a paid subscription.
func (a *Auth) IsPaidTier() bool {
	return a.Tier == "pro" || a.Tier == "enterprise"
}
