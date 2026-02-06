package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAuth_NoFile(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	auth, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth() error = %v", err)
	}
	if auth == nil {
		t.Fatal("LoadAuth() returned nil")
	}
	if auth.Token != "" {
		t.Errorf("expected empty token, got %q", auth.Token)
	}
}

func TestSaveAuth_RoundTrip(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	original := &Auth{
		Token:    "test-token-123",
		UserID:   "user-456",
		Email:    "test@example.com",
		Tier:     "pro",
		ExpireAt: 1234567890,
	}

	// Save
	if err := SaveAuth(original); err != nil {
		t.Fatalf("SaveAuth() error = %v", err)
	}

	// Verify file exists with correct permissions
	configPath := filepath.Join(tmpDir, ".cub-scout", "auth.json")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600, got %o", info.Mode().Perm())
	}

	// Load back
	loaded, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth() error = %v", err)
	}

	// Verify fields
	if loaded.Token != original.Token {
		t.Errorf("Token = %q, want %q", loaded.Token, original.Token)
	}
	if loaded.UserID != original.UserID {
		t.Errorf("UserID = %q, want %q", loaded.UserID, original.UserID)
	}
	if loaded.Email != original.Email {
		t.Errorf("Email = %q, want %q", loaded.Email, original.Email)
	}
	if loaded.Tier != original.Tier {
		t.Errorf("Tier = %q, want %q", loaded.Tier, original.Tier)
	}
	if loaded.ExpireAt != original.ExpireAt {
		t.Errorf("ExpireAt = %d, want %d", loaded.ExpireAt, original.ExpireAt)
	}
}

func TestLoadAuth_CorruptedFile(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	// Create corrupted config file
	configDir := filepath.Join(tmpDir, ".cub-scout")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "auth.json")
	if err := os.WriteFile(configPath, []byte("not valid json{{{"), 0600); err != nil {
		t.Fatal(err)
	}

	// Should return empty auth, not error
	auth, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth() should not error on corrupted file, got %v", err)
	}
	if auth.Token != "" {
		t.Errorf("expected empty auth on corrupted file, got token %q", auth.Token)
	}
}

func TestClearAuth(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	// Save auth first
	original := &Auth{Token: "to-be-cleared"}
	if err := SaveAuth(original); err != nil {
		t.Fatalf("SaveAuth() error = %v", err)
	}

	// Clear
	if err := ClearAuth(); err != nil {
		t.Fatalf("ClearAuth() error = %v", err)
	}

	// Load should return empty
	loaded, err := LoadAuth()
	if err != nil {
		t.Fatalf("LoadAuth() error = %v", err)
	}
	if loaded.Token != "" {
		t.Errorf("expected empty token after clear, got %q", loaded.Token)
	}
}

func TestIsAuthenticated(t *testing.T) {
	// Use a temp directory
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	// Initially not authenticated
	if IsAuthenticated() {
		t.Error("expected not authenticated initially")
	}

	// Save auth with token
	if err := SaveAuth(&Auth{Token: "valid-token"}); err != nil {
		t.Fatal(err)
	}

	// Now authenticated
	if !IsAuthenticated() {
		t.Error("expected authenticated after saving token")
	}
}

func TestIsPaidTier(t *testing.T) {
	tests := []struct {
		tier string
		want bool
	}{
		{"", false},
		{"free", false},
		{"pro", true},
		{"enterprise", true},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			auth := &Auth{Tier: tt.tier}
			if got := auth.IsPaidTier(); got != tt.want {
				t.Errorf("IsPaidTier() = %v, want %v", got, tt.want)
			}
		})
	}
}
