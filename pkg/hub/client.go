package hub

import (
	"fmt"
	"net/http"
	"time"
)

// Client is the central connection manager for ConfigHub.
// All ConfigHub API requests should go through this client.
type Client struct {
	httpClient *http.Client
	auth       *Auth
	mode       Mode
}

// NewClient creates a new ConfigHub client.
func NewClient() *Client {
	auth, _ := LoadAuth()
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		auth: auth,
		mode: CurrentMode(),
	}
}

// Mode returns the current operating mode.
func (c *Client) Mode() Mode {
	return c.mode
}

// RequireOnline checks if we're online and returns an error message if not.
func (c *Client) RequireOnline() error {
	if c.mode == Offline {
		return fmt.Errorf("this feature requires internet connectivity")
	}
	return nil
}

// RequireConnected checks if we're connected to ConfigHub.
// Returns a user-friendly message if not.
func (c *Client) RequireConnected() error {
	if c.mode == Offline {
		return fmt.Errorf("this feature requires internet connectivity")
	}
	if c.mode == Online {
		return fmt.Errorf("this feature requires ConfigHub authentication.\n\nSign up free at %s\nThen run: cub auth login", SignupURL())
	}
	return nil
}

// RequirePaid checks if user has a paid subscription.
func (c *Client) RequirePaid() error {
	if err := c.RequireConnected(); err != nil {
		return err
	}
	if !c.auth.IsPaidTier() {
		return fmt.Errorf("this feature requires a paid ConfigHub subscription.\n\nUpgrade at %s", WebBaseURL+"/pricing")
	}
	return nil
}

// Noop prints a message explaining why a feature is unavailable.
func Noop(feature string) {
	client := NewClient()
	switch client.mode {
	case Offline:
		fmt.Printf("noop: %s requires internet connectivity\n", feature)
	case Online:
		fmt.Printf("noop: %s requires ConfigHub authentication\n", feature)
		fmt.Printf("\nSign up free at %s\n", SignupURL())
		fmt.Printf("Then run: cub auth login\n")
	}
}
