// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

// Package hubread reads ConfigHub through the SDK's typed API client, and only
// reads (#758). It exists so that a connected read does not have to start a
// `cub` process and parse its output.
//
// The package exposes no SDK type and no way to reach the SDK's write calls:
// the HTTP client it builds refuses every method except GET and HEAD and
// follows no redirect, so even a call added here by mistake cannot change
// ConfigHub or carry the credential to another host.
package hubread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/confighub/sdk/core/cubapi"
	goclientnew "github.com/confighub/sdk/core/openapi/goclient-new"
	"github.com/google/uuid"
)

// Kind classifies a failed read. A caller that reports an omission uses it as
// the reason; it is never inferred from message text.
type Kind string

const (
	KindInvalidScope  Kind = "invalid_scope"
	KindNotConfigured Kind = "not_configured"
	KindUnauthorized  Kind = "unauthorized"
	KindForbidden     Kind = "forbidden"
	KindNotFound      Kind = "not_found"
	KindAmbiguous     Kind = "ambiguous"
	KindTimeout       Kind = "timeout"
	KindCanceled      Kind = "canceled"
	KindMalformed     Kind = "malformed"
	KindRefused       Kind = "refused"
	KindFailed        Kind = "request_failed"
)

// Error is a failed read. Message never contains a credential.
type Error struct {
	Kind    Kind
	Op      string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("confighub %s: %s (%s)", e.Op, e.Message, e.Kind) }

// KindOf returns the Kind of err, or KindFailed when err is not an *Error.
func KindOf(err error) Kind {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Kind
	}
	return KindFailed
}

var (
	errNotReadOnly = errors.New("hubread: only GET and HEAD are allowed")
	errRedirect    = errors.New("hubread: redirects are not followed")
)

// Stats counts what a Reader put on the wire.
type Stats struct {
	Requests      int64
	ResponseBytes int64
}

type readOnlyTransport struct {
	base     http.RoundTripper
	requests atomic.Int64
	bytes    atomic.Int64
}

func (t *readOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return nil, errNotReadOnly
	}
	t.requests.Add(1)
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &countingBody{ReadCloser: resp.Body, count: &t.bytes}
	return resp, nil
}

type countingBody struct {
	io.ReadCloser
	count *atomic.Int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.count.Add(int64(n))
	return n, err
}

// Options configures a Reader. The zero value is usable.
type Options struct {
	// Transport is the base round tripper; http.DefaultTransport when nil.
	Transport http.RoundTripper
	// Timeout bounds each request; 30 seconds when zero.
	Timeout time.Duration
	// UserAgent identifies the caller to the server.
	UserAgent string
}

// Reader is a read-only ConfigHub client for one server and one credential,
// both captured when it was built.
type Reader struct {
	client    *cubapi.Client
	transport *readOnlyTransport
}

func (o Options) httpClient() (*http.Client, *readOnlyTransport) {
	base := o.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := &readOnlyTransport{base: base}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		// A redirect could send the request, and on the same host its
		// credential, somewhere the caller did not name.
		CheckRedirect: func(*http.Request, []*http.Request) error { return errRedirect },
	}, transport
}

func (o Options) userAgent() string {
	if strings.TrimSpace(o.UserAgent) != "" {
		return o.UserAgent
	}
	return "cub-scout"
}

// Resolve builds a Reader from the credential source the SDK finds: the
// CUB_SERVER and CUB_TOKEN a `cub` plugin is given, or else the local cub
// configuration, honouring CUB_CONFIG and CUB_CONTEXT. It reads those files and
// writes nothing; it does not log in or refresh a token.
func Resolve(ctx context.Context, opts Options) (*Reader, error) {
	const op = "resolve credentials"
	notConfigured := func(err error) (*Reader, error) {
		return nil, &Error{Kind: KindNotConfigured, Op: op, Message: err.Error()}
	}
	httpClient, transport := opts.httpClient()
	clientOptions := cubapi.ClientOptions{UserAgent: opts.userAgent(), HTTPClient: httpClient}

	env, err := cubapi.LoadEnvironment(ctx)
	if err != nil {
		return notConfigured(err)
	}
	var client *cubapi.Client
	if env.HasCredentials() {
		client, err = cubapi.NewClientFromEnvironment(ctx, clientOptions)
	} else {
		// Not cubapi.ResolveClient: in SDK core v0.8.10 it hands CUB_CONFIG,
		// which names the config directory, to LoadConfig as if it were the
		// config file, and fails to read a directory. LoadConfig("") resolves
		// the directory the way cub does.
		var store *cubapi.Store
		if store, err = cubapi.LoadConfig(""); err != nil {
			return notConfigured(err)
		}
		// Use is an in-memory selection; the config file is not written.
		if err = store.Use(env.Context); err != nil {
			return notConfigured(err)
		}
		active, activeErr := store.ActiveContext()
		if activeErr != nil {
			return notConfigured(activeErr)
		}
		token, tokenErr := store.TokenData(active)
		if tokenErr != nil {
			// The cause is deliberately not included: a token file that is
			// not the expected JSON makes the parser quote what it found.
			return notConfigured(fmt.Errorf("the token for context %q could not be loaded; run `cub auth login`", active.Name))
		}
		// A context whose token file holds no token would send requests
		// with no credential and report the server's refusal as the cause.
		if strings.TrimSpace(token.AccessToken) == "" {
			return notConfigured(fmt.Errorf("context %q has no access token; run `cub auth login`", active.Name))
		}
		clientOptions.ServerURL, clientOptions.Token = active.Coordinate.ServerURL, token.AccessToken
		client, err = cubapi.NewClient(clientOptions)
	}
	if err != nil {
		return notConfigured(err)
	}
	return &Reader{client: client, transport: transport}, nil
}

// New builds a Reader for an explicit server and bearer token.
func New(serverURL, token string, opts Options) (*Reader, error) {
	if strings.TrimSpace(serverURL) == "" || strings.TrimSpace(token) == "" {
		return nil, &Error{Kind: KindNotConfigured, Op: "resolve credentials", Message: "a server URL and a token are required"}
	}
	httpClient, transport := opts.httpClient()
	client, err := cubapi.NewClient(cubapi.ClientOptions{ServerURL: serverURL, Token: token, UserAgent: opts.userAgent(), HTTPClient: httpClient})
	if err != nil {
		return nil, &Error{Kind: KindNotConfigured, Op: "resolve credentials", Message: err.Error()}
	}
	return &Reader{client: client, transport: transport}, nil
}

// Server is the server URL the Reader was built for.
func (r *Reader) Server() string { return r.client.Server }

// Stats reports the requests made and response bytes read so far.
func (r *Reader) Stats() Stats {
	return Stats{Requests: r.transport.requests.Load(), ResponseBytes: r.transport.bytes.Load()}
}

// UnitHead identifies one Unit and its head revision.
type UnitHead struct {
	Space           string
	SpaceID         string
	Unit            string
	UnitID          string
	HeadRevisionNum int64
}

// apiResponse is the part of a generated response this package reads.
type apiResponse interface {
	StatusCode() int
}

// classify turns a transport error or a non-200 status into an *Error. It
// returns nil when the response is a 200.
func classify(ctx context.Context, op string, err error, resp apiResponse) *Error {
	if err != nil {
		var urlErr *url.Error
		switch {
		case errors.Is(err, errNotReadOnly), errors.Is(err, errRedirect):
			return &Error{Kind: KindRefused, Op: op, Message: errors.Unwrap(unwrapURL(err)).Error()}
		case errors.Is(ctx.Err(), context.Canceled), errors.Is(err, context.Canceled):
			return &Error{Kind: KindCanceled, Op: op, Message: "the request was cancelled"}
		case ctx.Err() != nil, errors.Is(err, context.DeadlineExceeded), errors.As(err, &urlErr) && urlErr.Timeout():
			return &Error{Kind: KindTimeout, Op: op, Message: "the request did not complete in time"}
		case errors.As(err, &urlErr):
			return &Error{Kind: KindFailed, Op: op, Message: urlErr.Err.Error()}
		default:
			// The generated client returns a plain error when the body is
			// not the JSON the status promised.
			return &Error{Kind: KindMalformed, Op: op, Message: "the response could not be decoded: " + err.Error()}
		}
	}
	switch status := resp.StatusCode(); status {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return &Error{Kind: KindUnauthorized, Op: op, Message: "the server did not accept the credential (HTTP 401)"}
	case http.StatusForbidden:
		return &Error{Kind: KindForbidden, Op: op, Message: "the credential may not read this (HTTP 403)"}
	case http.StatusNotFound:
		return &Error{Kind: KindNotFound, Op: op, Message: "not found (HTTP 404)"}
	default:
		if status >= 300 && status < 400 {
			return &Error{Kind: KindRefused, Op: op, Message: fmt.Sprintf("the server answered with a redirect (HTTP %d), which is not followed", status)}
		}
		return &Error{Kind: KindFailed, Op: op, Message: fmt.Sprintf("HTTP %d", status)}
	}
}

// unwrapURL returns err wrapped so errors.Unwrap yields the innermost cause.
func unwrapURL(err error) error {
	for _, sentinel := range []error{errNotReadOnly, errRedirect} {
		if errors.Is(err, sentinel) {
			return fmt.Errorf("%w", sentinel)
		}
	}
	return err
}

func literal(op, field, value string) (cubapi.Where, *Error) {
	where := cubapi.Where{}.Eq(field, value)
	if err := where.Err(); err != nil {
		return where, &Error{Kind: KindInvalidScope, Op: op, Message: err.Error()}
	}
	return where, nil
}

// unitGetInclude is the expansion `cub unit get` asks for (cubapi's unexported
// unitGetInclude in SDK core v0.8.10), so that the envelope read here has the
// same related entities as the one cub prints. A test compares it with what
// cubapi.ResolveUnit sends, so an SDK bump that changes it is noticed.
const unitGetInclude = "UnitEventID,TargetID,UpstreamUnitID,SpaceID,FromLinkID,ChangeSetID,UpstreamSpaceID"

// unit reads exactly one Unit in exactly one space, with cub's expansions.
//
// space is the space's slug or its UUID; name is the Unit's slug. Both are
// required and neither may be a wildcard: an empty or "*" scope is refused
// before any request, because a list without a space spans the organization.
// Slugs are matched exactly, including case. More than one match is an error,
// never a choice.
func (r *Reader) unit(ctx context.Context, op, space, name string) (*goclientnew.ExtendedUnit, string, *Error) {
	space, name = strings.TrimSpace(space), strings.TrimSpace(name)
	switch {
	case space == "" || space == "*":
		return nil, "", &Error{Kind: KindInvalidScope, Op: op, Message: "exactly one space is required"}
	case name == "" || name == "*":
		return nil, "", &Error{Kind: KindInvalidScope, Op: op, Message: "exactly one unit is required"}
	}

	// Both names are checked before the first request, so a scope that
	// cannot be sent costs nothing and reveals nothing.
	where, scopeErr := literal(op, "Slug", name)
	if scopeErr != nil {
		return nil, "", scopeErr
	}

	spaceSlug := ""
	// Only the canonical 36-character form is an ID. uuid.Parse also accepts
	// 32 bare hex digits, braces and a urn: prefix, any of which could be a
	// slug; read as an ID, it would skip the lookup that checks the name.
	spaceID, err := uuid.Parse(space)
	if err == nil && !strings.EqualFold(spaceID.String(), space) {
		err = errors.New("not a canonical UUID")
	}
	if err != nil {
		id, resolveErr := r.spaceID(ctx, space)
		if resolveErr != nil {
			return nil, "", resolveErr
		}
		spaceID, spaceSlug = id, space
	}

	filter, include, limit := where.SpaceID(spaceID).String(), unitGetInclude, 2
	resp, err := r.client.API.ListAllUnitsWithResponse(ctx, &goclientnew.ListAllUnitsParams{Where: &filter, Include: &include, Limit: &limit})
	if failure := classify(ctx, op, err, resp); failure != nil {
		return nil, "", failure
	}
	if resp.JSON200 == nil {
		return nil, "", &Error{Kind: KindMalformed, Op: op, Message: "the response had no unit list"}
	}
	var matches []*goclientnew.ExtendedUnit
	for i := range *resp.JSON200 {
		// The server's filter is trusted to narrow, not to decide: only a
		// Unit whose slug and space are exactly the ones asked for counts.
		element := &(*resp.JSON200)[i]
		if candidate := element.Unit; candidate != nil && candidate.Slug == name && candidate.SpaceID == spaceID {
			matches = append(matches, element)
		}
	}
	switch len(matches) {
	case 0:
		return nil, "", &Error{Kind: KindNotFound, Op: op, Message: fmt.Sprintf("no unit %q in space %q", name, space)}
	case 1:
	default:
		return nil, "", &Error{Kind: KindAmbiguous, Op: op, Message: fmt.Sprintf("more than one unit %q in space %q", name, space)}
	}
	if matches[0].Unit.UnitID == uuid.Nil {
		return nil, "", &Error{Kind: KindMalformed, Op: op, Message: fmt.Sprintf("unit %q has no ID", name)}
	}
	return matches[0], spaceSlug, nil
}

// UnitHead reads the head revision of exactly one Unit in exactly one space.
// See unit for how the scope is checked.
func (r *Reader) UnitHead(ctx context.Context, space, unit string) (UnitHead, error) {
	found, spaceSlug, failure := r.unit(ctx, "unit read", space, unit)
	if failure != nil {
		return UnitHead{}, failure
	}
	return UnitHead{
		Space: spaceSlug, SpaceID: found.Unit.SpaceID.String(),
		Unit: found.Unit.Slug, UnitID: found.Unit.UnitID.String(), HeadRevisionNum: found.Unit.HeadRevisionNum,
	}, nil
}

// UnitJSON reads exactly one Unit in exactly one space and returns it as the
// JSON `cub unit get <unit> --space <space> -o json` prints: the same typed
// envelope, with the same expansions, marshalled the same way. It is for
// callers that already parse cub's output.
func (r *Reader) UnitJSON(ctx context.Context, space, unit string) ([]byte, error) {
	found, _, failure := r.unit(ctx, "unit read", space, unit)
	if failure != nil {
		return nil, failure
	}
	encoded, err := json.MarshalIndent(found, "", "  ")
	if err != nil {
		return nil, &Error{Kind: KindMalformed, Op: "unit read", Message: "the unit could not be encoded: " + err.Error()}
	}
	return append(encoded, '\n'), nil
}

func (r *Reader) spaceID(ctx context.Context, slug string) (uuid.UUID, *Error) {
	const op = "space lookup"
	where, scopeErr := literal(op, "Slug", slug)
	if scopeErr != nil {
		return uuid.Nil, scopeErr
	}
	filter, limit := where.String(), 2
	resp, err := r.client.API.ListSpacesWithResponse(ctx, &goclientnew.ListSpacesParams{Where: &filter, Limit: &limit})
	if failure := classify(ctx, op, err, resp); failure != nil {
		return uuid.Nil, failure
	}
	if resp.JSON200 == nil {
		return uuid.Nil, &Error{Kind: KindMalformed, Op: op, Message: "the response had no space list"}
	}
	var ids []uuid.UUID
	for i := range *resp.JSON200 {
		if candidate := (*resp.JSON200)[i].Space; candidate != nil && candidate.Slug == slug {
			ids = append(ids, candidate.SpaceID)
		}
	}
	switch len(ids) {
	case 0:
		return uuid.Nil, &Error{Kind: KindNotFound, Op: op, Message: fmt.Sprintf("no space %q", slug)}
	case 1:
		if ids[0] == uuid.Nil {
			return uuid.Nil, &Error{Kind: KindMalformed, Op: op, Message: fmt.Sprintf("space %q has no ID", slug)}
		}
		return ids[0], nil
	default:
		return uuid.Nil, &Error{Kind: KindAmbiguous, Op: op, Message: fmt.Sprintf("more than one space %q", slug)}
	}
}
