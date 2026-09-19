// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// A ConfigHub server names its own OCI registry. Asking it is the difference
// between recognising an OCI source and guessing at one: the hosted service
// serves bundles from `oci.<instance>`, which a host prefix gives away, but a
// self-hosted or local server serves them from wherever it was configured —
// `localhost:32181` on a `cub cluster up` machine — and no prefix says so.
//
// cub itself does this for `cub cluster`: it reads the registry from
// /api/info and only falls back to deriving one from the API URL when the
// server is too old to advertise one.
//
// The endpoint is unauthenticated, but this observer still honors the same
// offline, telemetry, and connected-read gate as other ConfigHub reads.

// serverInfo is the part of /api/info this needs.
type serverInfo struct {
	OCIHost string `json:"OCIHost"`
	OCIPort string `json:"OCIPort"`
}

const (
	ociRegistryLookupTimeout = 3 * time.Second
	ociRegistryCacheTTL      = 30 * time.Second
	ociRegistryCacheMax      = 8
	ociRegistryBodyMax       = 64 << 10
)

type ociRegistryCacheEntry struct {
	registry  string
	expiresAt time.Time
}

var ociRegistryCache = struct {
	sync.Mutex
	values map[string]ociRegistryCacheEntry
}{values: make(map[string]ociRegistryCacheEntry)}

// OCIRegistry is the registry the configured ConfigHub server advertises, as
// host[:port], or "" when it is unknown: no server configured, unreachable, or
// a server that does not advertise one.
//
// Successful answers are kept per active server/context environment identity
// for a short bounded session. Failures are not cached: a transient server
// error, a just-started local server, or a repaired configuration must be
// observable on the next bounded attempt.
func OCIRegistry() string {
	// Do this before resolving a standalone cub context: ordinary offline
	// parser work must not spawn cub or touch the network at all.
	if ociRegistryReadsDisabled() {
		return ""
	}

	// These are the active host/context inputs available without spawning cub.
	// A plugin or explicit context switch therefore invalidates the entry before
	// the next lookup; standalone sessions with no such env change are bounded
	// by the cache TTL below.
	key := ociRegistryCacheKey()
	ociRegistryCache.Lock()
	if entry, ok := ociRegistryCache.values[key]; ok && time.Now().Before(entry.expiresAt) {
		ociRegistryCache.Unlock()
		return entry.registry
	}
	delete(ociRegistryCache.values, key)
	ociRegistryCache.Unlock()

	registry := CurrentOCIRegistry()
	if registry == "" {
		return ""
	}

	ociRegistryCache.Lock()
	if len(ociRegistryCache.values) >= ociRegistryCacheMax {
		var oldestKey string
		var oldest time.Time
		for candidate, entry := range ociRegistryCache.values {
			if oldestKey == "" || entry.expiresAt.Before(oldest) {
				oldestKey, oldest = candidate, entry.expiresAt
			}
		}
		delete(ociRegistryCache.values, oldestKey)
	}
	ociRegistryCache.values[key] = ociRegistryCacheEntry{
		registry:  registry,
		expiresAt: time.Now().Add(ociRegistryCacheTTL),
	}
	ociRegistryCache.Unlock()
	return registry
}

// CurrentOCIRegistry bypasses the recognition cache for exact connected joins.
// The cub default context can change on disk without any environment change.
func CurrentOCIRegistry() string {
	if err := RequireCubConnected(); err != nil {
		return ""
	}
	serverURL := ConfigHubServerURL()
	if serverURL == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), ociRegistryLookupTimeout)
	defer cancel()
	return lookupOCIRegistry(ctx, serverURL)
}

func ociRegistryReadsDisabled() bool {
	return os.Getenv("CUB_SCOUT_OFFLINE") == "true" || telemetryDisabled()
}

func ociRegistryCacheKey() string {
	return strings.Join([]string{
		os.Getenv(envCubPlugin),
		os.Getenv(envCubServer),
		os.Getenv(envCubContext),
		os.Getenv(envCubConfig),
	}, "\x00")
}

// ConfigHubServerURL is the ConfigHub server this cub-scout is configured
// against: the one the plugin host passed, else the one `cub` names.
func ConfigHubServerURL() string {
	if server := PluginServer(); server != "" {
		return server
	}
	return cubContextServerURL()
}

// lookupOCIRegistry asks one server for its registry.
func lookupOCIRegistry(ctx context.Context, serverURL string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, ociRegistryLookupTimeout)
	defer cancel()
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		return ""
	}
	base, err := url.Parse(serverURL)
	if err != nil || base.Host == "" || base.User != nil || base.Fragment != "" || base.RawPath != "" || base.Opaque != "" {
		return ""
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return ""
	}
	base.Path = "/api/info"
	base.RawQuery = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return ""
	}
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close() //nolint:errcheck // read-only probe
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, ociRegistryBodyMax+1))
	if err != nil || len(body) > ociRegistryBodyMax {
		return ""
	}
	var info serverInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return ""
	}
	return formatOCIRegistry(info.OCIHost, info.OCIPort)
}

// formatOCIRegistry joins what the server said into a registry host, leaving
// out a port only when the server named none.
func formatOCIRegistry(host, port string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if port = strings.TrimSpace(port); port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
}

// cubContextServerURL reads the server URL from `cub context get`. It is the
// one place outside connected.go and auth.go that runs cub, for the same
// reason: pkg/hub cannot import the command package's runner.
func cubContextServerURL() string {
	ctx, cancel := context.WithTimeout(context.Background(), ociRegistryLookupTimeout)
	defer cancel()
	out, err := cubCommandOutputContext(ctx, "context", "get", "-o", "json")
	if err != nil {
		return ""
	}
	var cubCtx struct {
		Coordinate struct {
			ServerURL string `json:"serverURL"`
		} `json:"coordinate"`
	}
	if err := json.Unmarshal(out, &cubCtx); err != nil {
		return ""
	}
	return strings.TrimSpace(cubCtx.Coordinate.ServerURL)
}

// resetOCIRegistryForTest clears successful answers. Failed answers are never
// retained, so a test can retry without a separate failure reset.
func resetOCIRegistryForTest() {
	ociRegistryCache.Lock()
	ociRegistryCache.values = make(map[string]ociRegistryCacheEntry)
	ociRegistryCache.Unlock()
}

func cubCommandOutputContext(ctx context.Context, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "cub", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cub %s: %w: %s", strings.Join(args, " "), err, firstLine(stderr.String()))
	}
	return stdout.Bytes(), nil
}
