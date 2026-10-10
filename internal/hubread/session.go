package hubread

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// This file is the check `cub auth status` makes, made without cub: is there
// a credential, has it expired, does the server accept it, and is this client
// too old for the server.

// sdkCoreModule is the SDK module this package is built with. Its version is
// this client's version, as cub's own version is cub's.
const sdkCoreModule = "github.com/confighub/sdk/core"

// sdkCoreVersion is the version of the SDK linked into this binary, or ""
// when the build does not record it. It is a variable for tests.
var sdkCoreVersion = func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dependency := range info.Deps {
		if dependency.Path == sdkCoreModule {
			if dependency.Replace != nil {
				return dependency.Replace.Version
			}
			return dependency.Version
		}
	}
	return ""
}

// now is the clock, for tests.
var now = time.Now

// Session is what a session check found: the server, its version when it
// states one, and the version of the SDK this binary reads it with.
type Session struct {
	Server        string
	ServerVersion string
	SDKVersion    string
	// ClientAhead is true when this SDK's minor version is above the
	// server's. cub warns about that and carries on; so does this.
	ClientAhead bool
}

// tokenExpiry reads the "exp" claim of a JWT access token, as cub does. It
// does not verify the token; the server does that. ok is false for a token
// that is not a JWT or has no expiry.
func tokenExpiry(token string) (expiry time.Time, ok bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

// majorMinor reads the first two numbers of a version such as "v0.8.12" or
// "0.9.0-dev", the way cub compares versions.
func majorMinor(version string) (major, minor int, ok bool) {
	text := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if i := strings.IndexAny(text, "-+"); i >= 0 {
		text = text[:i]
	}
	parts := strings.Split(text, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return 0, 0, false
	}
	return major, minor, true
}

// CheckSession reports whether ConfigHub can be read with this Reader's
// credential, by the rules of `cub auth status`:
//
//   - a token that says it has expired is refused before any request;
//   - the server must accept the credential, which one GET of the caller's
//     own membership shows;
//   - a client older than the server is refused. Below 1.0 a change in
//     ConfigHub's second version number is not backward compatible, and cub
//     refuses on that ground; here the client is the SDK this binary was built
//     with. A client newer than the server is allowed and reported.
//
// A server that states no version, or a build that does not record the SDK's,
// cannot be compared and is not refused, as cub does not refuse it.
func (r *Reader) CheckSession(ctx context.Context) (Session, error) {
	const op = "session check"
	session := Session{Server: r.client.Server, SDKVersion: sdkCoreVersion()}
	if !r.tokenExpiry.IsZero() && now().After(r.tokenExpiry) {
		return session, &Error{Kind: KindUnauthorized, Op: op, Message: fmt.Sprintf(
			"the access token expired at %s; run `cub auth login`", r.tokenExpiry.UTC().Format(time.RFC3339))}
	}
	resp, err := r.client.API.GetMeWithResponse(ctx)
	if failure := classify(ctx, op, err, resp); failure != nil {
		if failure.Kind == KindUnauthorized {
			failure.Message = "the server did not accept the credential (HTTP 401); run `cub auth login`"
		}
		return session, failure
	}
	// A membership names a user or an organization. A 200 that names
	// neither, a JSON null or an empty object from something in between, is
	// not the server saying who this is.
	if member := resp.JSON200; member == nil || (member.UserID == uuid.Nil && member.OrganizationID == uuid.Nil && member.ExternalID == "") {
		return session, &Error{Kind: KindMalformed, Op: op, Message: "the server's answer was not the caller's membership"}
	}
	session.ServerVersion = r.client.ServerVersion()
	clientMajor, clientMinor, clientKnown := majorMinor(session.SDKVersion)
	serverMajor, serverMinor, serverKnown := majorMinor(session.ServerVersion)
	if !clientKnown || !serverKnown {
		return session, nil
	}
	switch {
	case clientMajor < serverMajor || (clientMajor == serverMajor && clientMinor < serverMinor):
		return session, &Error{Kind: KindVersionSkew, Op: op, Message: fmt.Sprintf(
			"this cub-scout reads ConfigHub with SDK %s, which is too old for server %s: below 1.0 a change in the second version number is not backward compatible. Upgrade cub-scout",
			session.SDKVersion, session.ServerVersion)}
	case clientMajor > serverMajor || clientMinor > serverMinor:
		session.ClientAhead = true
	}
	return session, nil
}
