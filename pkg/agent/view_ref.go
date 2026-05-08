// Package agent — ConfigHub View reference parsing.
//
// v0.1 of #391: lock in the URL-as-positional convention and the View
// reference shape downstream commands consume.
//
// Background: ConfigHub Views are saved filter+projection specs (filter
// clause + columns + grouping + ordering), addressable by stable UUID.
// The user explicitly preferred URL-as-positional input over UUID flags
// across cub-scout's ConfigHub integrations — see the saved memory at
// `principle_url_as_input` and the comment chain on #391. This file
// implements the parser side of that convention.
//
// Two accepted input forms (both produce the same ViewRef):
//
//   1. Bare UUID:
//        806aac53-236c-446d-8ad6-91d6daf6810e
//
//   2. View Explorer URL:
//        https://hub.confighub.com/x/view-explorer?view=<uuid>
//        https://hub.confighub.com/x/view-explorer?view=<uuid>&group=<v>
//
// The parser accepts any ConfigHub-shaped URL with a ?view= query
// parameter; future View Explorer URL parameters (`group`, paging,
// etc.) round-trip in ViewRef.Extras so consumers can use them
// without the parser growing flags.

package agent

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ViewRef is a parsed reference to a ConfigHub View.
type ViewRef struct {
	// UUID is the canonical View identifier (stable, server-side).
	UUID string

	// SourceForm captures whether the caller passed a UUID or a URL —
	// useful for emitting back the same shape they typed (e.g. in
	// hint output) and for telemetry.
	SourceForm ViewRefSource

	// OriginalURL is set when SourceForm == ViewRefURL. Empty for
	// UUID-only inputs. Stored verbatim so consumers can deep-link
	// back to the View Explorer if they want to.
	OriginalURL string

	// Extras carries any additional View Explorer query parameters the
	// parser did not interpret (e.g. `group`, future filters). The
	// parser does not act on these — downstream commands opt in to
	// the params they understand. Always non-nil; empty when the
	// input had no extra params.
	Extras url.Values
}

// ViewRefSource discriminates the input shape a ViewRef was parsed from.
type ViewRefSource string

const (
	ViewRefUUID ViewRefSource = "uuid"
	ViewRefURL  ViewRefSource = "url"
)

// uuidPattern matches the canonical 8-4-4-4-12 UUID shape with
// case-insensitive hex digits. ConfigHub UUIDs are lowercase but the
// parser is tolerant.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ParseViewRef parses a CLI-friendly View identifier. Accepts bare UUIDs
// and ConfigHub View Explorer URLs. Empty input returns ("", false) so
// callers can distinguish "not set" from "invalid".
//
// The parser does NOT validate that the UUID corresponds to a real View
// — that requires a connected-mode `cub view get` call. It only validates
// shape.
//
// Hostname check: ConfigHub URLs are accepted regardless of host
// (hub.confighub.com, on-prem deployments, dev clusters). The parser
// does not pin the host because cub-scout users may run against any
// ConfigHub instance.
//
// Errors are descriptive — ParseViewRef is intended to back CLI flag
// parsing where users see the message directly.
func ParseViewRef(input string) (*ViewRef, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil, fmt.Errorf("view reference is empty; expected a UUID or a ConfigHub View Explorer URL")
	}

	// Bare UUID.
	if uuidPattern.MatchString(s) {
		return &ViewRef{
			UUID:       strings.ToLower(s),
			SourceForm: ViewRefUUID,
			Extras:     url.Values{},
		}, nil
	}

	// URL form. Accept anything URL-shaped that has a ?view= param.
	// strings.Contains is a cheap pre-check before the parser cost.
	if !strings.Contains(s, "://") {
		return nil, fmt.Errorf("view reference %q is neither a UUID nor a URL (got non-URL input)", input)
	}

	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("view reference %q is not a parseable URL: %w", input, err)
	}

	// We do not pin the host — see comment above. We do require that
	// the URL path mentions view-explorer so we don't accept arbitrary
	// URLs that happen to have a ?view= param.
	if !strings.Contains(u.Path, "view-explorer") {
		return nil, fmt.Errorf("URL %q is not a View Explorer URL (path does not contain view-explorer)", input)
	}

	q := u.Query()
	uuid := strings.TrimSpace(q.Get("view"))
	if uuid == "" {
		return nil, fmt.Errorf("URL %q is missing the ?view=<uuid> query parameter", input)
	}
	if !uuidPattern.MatchString(uuid) {
		return nil, fmt.Errorf("URL %q has ?view=%q which is not a valid UUID", input, uuid)
	}

	// Strip the `view` param from extras so consumers can iterate
	// without re-encountering it.
	extras := url.Values{}
	for k, vs := range q {
		if k == "view" {
			continue
		}
		extras[k] = vs
	}

	return &ViewRef{
		UUID:        strings.ToLower(uuid),
		SourceForm:  ViewRefURL,
		OriginalURL: s,
		Extras:      extras,
	}, nil
}
