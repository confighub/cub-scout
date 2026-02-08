package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewClient_FallbackWhenAuthLoadFails(t *testing.T) {
	origHome := os.Getenv("HOME")
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	defer func() { os.Setenv("HOME", origHome) }()

	// Force LoadAuth() to fail by creating a directory where auth.json should be.
	authPath := filepath.Join(tmpDir, ".cub-scout", "auth.json")
	if err := os.MkdirAll(authPath, 0o755); err != nil {
		t.Fatalf("failed to create auth path directory: %v", err)
	}

	client := NewClient()
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.auth == nil {
		t.Fatal("expected non-nil auth fallback")
	}
}

func TestRequirePaid_NilAuth_NoPanic(t *testing.T) {
	client := &Client{
		auth: nil,
		mode: Connected,
	}

	err := client.RequirePaid()
	if err == nil {
		t.Fatal("expected error for missing paid subscription")
	}
	if !strings.Contains(err.Error(), "paid ConfigHub subscription") {
		t.Fatalf("unexpected error: %v", err)
	}
}
