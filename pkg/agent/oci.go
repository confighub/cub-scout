// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"strings"
)

var knownConfigHubOCIRegistries = map[string]struct{}{
	// Hosted ConfigHub.
	"oci.api.confighub.com": {},
	// The exact local endpoint used by the legacy target layout.
	"oci.localhost:8080": {},
}

// OCILayout is the shape of a ConfigHub OCI repository path.
//
// ConfigHub has emitted two. The older one names a target:
//
//	oci://oci.<instance>/target/<space>/<target>
//
// The current one names a space, and the reference is the release:
//
//	oci://<registry>/space/<space-slug>:latest
//
// A local server advertises its registry at /api/info as host and port —
// `localhost:32181` on a `cub cluster up` machine — so the `oci.` host prefix
// the first layout relied on is not there to key off.
type OCILayout int

const (
	// OCILayoutNone is a repository path that matches neither layout.
	OCILayoutNone OCILayout = iota
	// OCILayoutTarget is target/<space>/<target>.
	OCILayoutTarget
	// OCILayoutSpace is space/<slug>.
	OCILayoutSpace
)

func (l OCILayout) String() string {
	switch l {
	case OCILayoutTarget:
		return "target"
	case OCILayoutSpace:
		return "space"
	default:
		return "none"
	}
}

// OCISourceInfo contains parsed information from an OCI registry URL
type OCISourceInfo struct {
	// Raw is the full OCI URL
	Raw string

	// IsConfigHub reports that this URL is a ConfigHub registry's, on evidence
	// rather than on the path shape alone: either the host is an exact known
	// ConfigHub endpoint, or it is the registry the configured server advertises.
	//
	// A third-party registry can serve a repository called `space/anything`,
	// so the path shape on its own is not enough to claim ConfigHub. When it
	// is all there is, Layout and SpaceCandidate are still filled in and a
	// caller with corroboration can act on them.
	IsConfigHub bool

	// Instance is the ConfigHub instance host (e.g., "api.confighub.com")
	Instance string

	// Space is the ConfigHub space this source is claimed to be in. Empty
	// unless IsConfigHub.
	Space string

	// Target is the ConfigHub target name. Only the target layout has one.
	Target string

	// Registry is the OCI registry host
	Registry string

	// Repository is the OCI repository path, without any reference
	Repository string

	// Reference is the tag or digest the URL pins, without its separator:
	// "latest" for :latest, "sha256:..." for @sha256:...
	Reference string

	// Layout is which ConfigHub repository shape the path matches, whoever
	// serves it.
	Layout OCILayout

	// SpaceCandidate is the space slug the path names, whoever serves it. It
	// is what a caller that can corroborate — the registry the server
	// advertises, a space that exists — should confirm before treating this
	// source as ConfigHub's.
	SpaceCandidate string
}

// ConfigHubOCIRegistryFn reports the registry the configured ConfigHub server
// advertises, as host[:port], or "" when it is unknown.
//
// The command layer points this at hub.OCIRegistry, which asks the server at
// /api/info and keeps the answer. It is a variable because pkg/agent has no
// business knowing how a server is configured, and because a parser that
// reached out on its own would be a surprise; tests use
// ParseOCISourceForRegistry directly and leave this alone.
var ConfigHubOCIRegistryFn = func() string { return "" }

// ParseOCISource parses an OCI URL, claiming a ConfigHub source when the host
// is an exact known endpoint or matches the registry the configured server
// advertises.
func ParseOCISource(url string) OCISourceInfo {
	// Only ask about the registry for a URL that could be ConfigHub's: a
	// standalone run with no ConfigHub configured should not pay for a lookup
	// on every ghcr.io reference.
	if !strings.HasPrefix(url, "oci://") {
		return OCISourceInfo{Raw: url}
	}
	info := ParseOCISourceForRegistry(url, "")
	if info.IsConfigHub || info.Layout == OCILayoutNone {
		return info
	}
	if ConfigHubOCIRegistryFn == nil {
		return info
	}
	return ParseOCISourceForRegistry(url, ConfigHubOCIRegistryFn())
}

// ParseOCISourceForRegistry parses an OCI URL, treating confighubRegistry (a
// host, optionally with a port, as /api/info advertises it) as ConfigHub's.
func ParseOCISourceForRegistry(url, confighubRegistry string) OCISourceInfo {
	info := OCISourceInfo{Raw: url}

	if !strings.HasPrefix(url, "oci://") {
		return info
	}
	remainder := strings.TrimPrefix(url, "oci://")

	parts := strings.SplitN(remainder, "/", 2)
	info.Registry = parts[0]
	if len(parts) > 1 {
		info.Repository, info.Reference = splitOCIReference(parts[1])
	}

	info.Layout, info.SpaceCandidate, _ = parseOCIRepositoryLayout(info.Repository)
	if info.Layout == OCILayoutNone {
		return info
	}

	// The claim. Only exact known endpoints from the emitted source formats, or
	// the registry the configured server advertised, are evidence. A path shape
	// and an arbitrary `oci.` prefix are not enough.
	switch {
	case isKnownConfigHubOCIRegistry(info.Registry):
		info.IsConfigHub = true
		info.Instance = strings.TrimPrefix(strings.ToLower(info.Registry), "oci.")
	case confighubRegistry != "" && strings.EqualFold(info.Registry, strings.TrimSpace(confighubRegistry)):
		info.IsConfigHub = true
		info.Instance = info.Registry
	}
	if !info.IsConfigHub {
		return info
	}

	_, space, target := parseOCIRepositoryLayout(info.Repository)
	info.Space, info.Target = space, target
	return info
}

func isKnownConfigHubOCIRegistry(registry string) bool {
	_, ok := knownConfigHubOCIRegistries[strings.ToLower(strings.TrimSpace(registry))]
	return ok
}

// parseOCIRepositoryLayout reports which ConfigHub layout a repository path
// matches, and the space and target it names.
func parseOCIRepositoryLayout(repository string) (OCILayout, string, string) {
	switch {
	case strings.HasPrefix(repository, "target/"):
		rest := strings.TrimPrefix(repository, "target/")
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || !validOCIScopePart(parts[0]) || !validOCIScopePart(parts[1]) {
			return OCILayoutNone, "", ""
		}
		return OCILayoutTarget, parts[0], parts[1]
	case strings.HasPrefix(repository, "space/"):
		// The server serves exactly /v2/space/{space}/manifests/{ref}, so
		// anything further down the path is not this layout.
		space := strings.TrimPrefix(repository, "space/")
		if !validOCIScopePart(space) {
			return OCILayoutNone, "", ""
		}
		return OCILayoutSpace, space, ""
	default:
		return OCILayoutNone, "", ""
	}
}

// Scope names become exact connected read arguments. Never turn malformed URL
// syntax, escaped separators or a wildcard into an all-spaces query.
func validOCIScopePart(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, c := range value {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// splitOCIReference separates a repository path from the tag or digest pinned
// to it. A digest uses @, a tag uses the last colon — and a registry host with
// a port has already been taken off the front by the caller.
func splitOCIReference(path string) (repository, reference string) {
	if at := strings.LastIndex(path, "@"); at >= 0 {
		return path[:at], path[at+1:]
	}
	if colon := strings.LastIndex(path, ":"); colon >= 0 && !strings.Contains(path[colon+1:], "/") {
		return path[:colon], path[colon+1:]
	}
	return path, ""
}

// IsConfigHubOCI checks if a URL is a ConfigHub OCI registry URL
func IsConfigHubOCI(url string) bool {
	return ParseOCISource(url).IsConfigHub
}

// IsConfigHubOCIForRegistry is IsConfigHubOCI for a caller that knows the
// ConfigHub server's registry.
func IsConfigHubOCIForRegistry(url, confighubRegistry string) bool {
	return ParseOCISourceForRegistry(url, confighubRegistry).IsConfigHub
}

// FormatConfigHubOCISource formats a ConfigHub OCI source for display
func FormatConfigHubOCISource(info OCISourceInfo) string {
	if !info.IsConfigHub {
		return info.Raw
	}

	switch {
	// Only the target layout has a target, so this needs no layout check and
	// keeps working for a value built by hand.
	case info.Space != "" && info.Target != "":
		return info.Space + "/" + info.Target
	case info.Layout == OCILayoutSpace && info.Space != "" && info.Reference != "":
		return info.Space + "@" + info.Reference
	case info.Layout == OCILayoutSpace && info.Space != "":
		return info.Space
	}

	// A target URL that names no target is malformed rather than a space, so
	// it is shown as it came.
	return info.Raw
}
