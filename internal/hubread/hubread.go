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
	"os"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/confighub/sdk/core/constants"
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
	KindIncomplete    Kind = "incomplete"
	KindFailed        Kind = "request_failed"
)

// Error is a failed read. Message never contains a credential.
type Error struct {
	Kind    Kind
	Op      string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("confighub %s: %s (%s)", e.Op, e.Message, e.Kind) }

// Is lets a caller that asks errors.Is for the context package's errors find
// a cancelled or timed-out read, though the cause itself is not kept.
func (e *Error) Is(target error) bool {
	switch target {
	case context.Canceled:
		return e.Kind == KindCanceled
	case context.DeadlineExceeded:
		return e.Kind == KindTimeout
	}
	return false
}

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

// Resolve builds a Reader from the credential cub itself would use: the
// CUB_SERVER and CUB_TOKEN that cub gives a plugin it starts, or else the local
// cub configuration, honouring CUB_CONFIG and CUB_CONTEXT. It reads those files
// and writes nothing; it does not log in or refresh a token.
//
// The environment pair is used only under CUB_PLUGIN=1, which cub sets with
// it. The cub CLI does not read CUB_SERVER or CUB_TOKEN, so a pair exported in
// a shell would otherwise send this reader to one server while `cub`, and
// every read cub-scout still makes through it, goes to another.
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
	if env.HasCredentials() && os.Getenv("CUB_PLUGIN") == "1" {
		client, err = cubapi.NewClientFromEnvironment(ctx, clientOptions)
	} else {
		// Not cubapi.ResolveClient: in SDK core v0.8.10 and v0.8.12 it hands CUB_CONFIG,
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
		// Every read here is a list, and a list with no match is an empty
		// list. A 404 says the endpoint is missing, which is a wrong server
		// URL or a server this client does not fit, not a missing Unit.
		return &Error{Kind: KindFailed, Op: op, Message: "the server has no such endpoint (HTTP 404); check the server URL"}
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

	spaceID, spaceSlug, scopeErr := r.namedSpace(ctx, space)
	if scopeErr != nil {
		return nil, "", scopeErr
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

// namedSpace returns the ID of the one space a read names, and its slug when
// it was named by slug.
//
// Only the canonical 36-character form is an ID. uuid.Parse also accepts 32
// bare hex digits, braces and a urn: prefix, any of which could be a slug;
// read as an ID, it would skip the lookup that checks the name.
func (r *Reader) namedSpace(ctx context.Context, space string) (uuid.UUID, string, *Error) {
	id, err := uuid.Parse(space)
	if err == nil && strings.EqualFold(id.String(), space) {
		return id, "", nil
	}
	id, resolveErr := r.spaceID(ctx, space)
	if resolveErr != nil {
		return uuid.Nil, "", resolveErr
	}
	return id, space, nil
}

// unitListInclude and spaceListInclude are the expansions `cub unit list` and
// `cub space list` ask for (cub's unitListInclude, and the Component a space
// list with its summary includes). Tests compare what is sent here with what
// the SDK's own list helpers send for the same options.
const (
	unitListInclude  = "UnitEventID,TargetID,UpstreamUnitID,SpaceID,FromLinkID,ChangeSetID"
	spaceListInclude = "ComponentID"
)

// listJSON encodes a list as cub prints one: the typed envelopes, indented,
// and "[]" rather than "null" when there are none.
func listJSON[T any](op string, list *[]T) ([]byte, error) {
	// A JSON null is a list with nothing in it.
	elements := []T{}
	if list != nil && *list != nil {
		elements = *list
	}
	encoded, err := json.MarshalIndent(elements, "", "  ")
	if err != nil {
		return nil, &Error{Kind: KindMalformed, Op: op, Message: "the list could not be encoded: " + err.Error()}
	}
	return append(encoded, '\n'), nil
}

// Filter narrows a list the way cub's flags of the same names do. The zero
// value lists everything in the space.
type Filter struct {
	// Where is a filter expression, passed to the server as the caller
	// wrote it and AND-ed with the space, as cub composes it.
	Where string
	// Contains is a free-text search.
	Contains string
}

// UnitFilter is Filter, under the name the unit list first had.
type UnitFilter = Filter

// The expansions each of cub's lists asks for, from the cub source at SDK core
// v0.8.12. Tests compare what is sent here with what the SDK's own list
// helpers send for the same options, and the Connected lane compares the
// answers with cub's on a real server.
const (
	workerListInclude    = "SpaceID"
	targetListInclude    = "SpaceID,TriggerFilterID,TriggerIDs"
	changeSetListInclude = "SpaceID,StartTagID,EndTagID"
	linkListInclude      = "SpaceID,FromUnitID,ToUnitID,ToSpaceID"
)

// listQuery is what one list sends: the filter with the space AND-ed on, the
// expansions, and the search.
type listQuery struct {
	where    *string
	include  *string
	contains *string
}

// listAnswer is what one list call came back with.
type listAnswer[T any] struct {
	response apiResponse
	list     *[]T
	http     *http.Response
	rejected *goclientnew.StandardErrorResponse
}

// spaceList reads the entities of one kind in exactly one space and returns
// the list as the JSON `cub <kind> list --space <space> -o json` prints.
//
// space is the space's slug or its canonical UUID; an empty or "*" space is
// refused before any request, because a list without a space spans the
// organization. As cub does without --limit, it asks for the whole list in
// one request.
//
// A list is whole or it is an error. An entry with no entity, an entry from
// any other space, a 200 that is not JSON and an answer the server marks as
// cut short all fail the read; none becomes a shorter or an empty list. What
// each entry carries of related entities is what the server expands, as in
// cub's output, and is not checked.
func spaceList[T any](ctx context.Context, r *Reader, kind, space string, filter Filter, include string,
	call func(listQuery) (listAnswer[T], error), spaceOf func(*T) (uuid.UUID, bool)) ([]byte, error) {
	op := kind + " list"
	space = strings.TrimSpace(space)
	if space == "" || space == "*" {
		return nil, &Error{Kind: KindInvalidScope, Op: op, Message: "exactly one space is required"}
	}
	spaceID, spaceSlug, scopeErr := r.namedSpace(ctx, space)
	if scopeErr != nil {
		return nil, scopeErr
	}
	where := cubapi.NewWhere(filter.Where).SpaceID(spaceID).String()
	query := listQuery{where: &where}
	if include != "" {
		query.include = &include
	}
	if contains := filter.Contains; contains != "" {
		query.contains = &contains
	}
	answer, err := call(query)
	if failure := classify(ctx, op, err, answer.response); failure != nil {
		// The server says what is wrong with a filter it rejects, and the
		// caller wrote the filter, so that reason is passed on.
		if failure.Kind == KindFailed && answer.rejected != nil {
			if reason := printable(answer.rejected.Message, 300); reason != "" {
				failure.Message = "the server rejected the request (HTTP 400): " + reason
			}
		}
		return nil, failure
	}
	// A 200 that is not JSON leaves no list at all. That is not an empty
	// list: saying "none" for a proxy's HTML page would be a false claim.
	if answer.list == nil {
		return nil, &Error{Kind: KindMalformed, Op: op, Message: "the response had no " + kind + " list"}
	}
	if failure := wholeList(op, answer.http); failure != nil {
		return nil, failure
	}
	// A space named by ID was not looked up, and a list filtered by an ID
	// that names no space is empty too. cub says the space was not found;
	// "none" would be a claim about a space that does not exist.
	if len(*answer.list) == 0 && spaceSlug == "" {
		if failure := r.spaceExists(ctx, spaceID); failure != nil {
			return nil, failure
		}
	}
	for i := range *answer.list {
		switch entrySpace, ok := spaceOf(&(*answer.list)[i]); {
		case !ok:
			return nil, &Error{Kind: KindMalformed, Op: op, Message: "the list has an entry with no " + kind}
		case entrySpace != spaceID:
			message := fmt.Sprintf("the server returned a %s from another space for space %q", kind, space)
			if filter.Where != "" {
				// The expression is sent as written. One that the server
				// reads as an alternative to the space, not a narrowing
				// of it, would do this.
				message += "; the filter may reach beyond the space, and its result is not shown"
			}
			return nil, &Error{Kind: KindMalformed, Op: op, Message: message}
		}
	}
	return listJSON(op, answer.list)
}

// rejection is the server's own message for an HTTP 400, when it sent one.
func rejection(status int, rejected *goclientnew.StandardErrorResponse) *goclientnew.StandardErrorResponse {
	if status == http.StatusBadRequest {
		return rejected
	}
	return nil
}

// UnitListJSON reads the Units of exactly one space and returns the list as the
// JSON `cub unit list --space <space> -o json` prints, with --where and
// --contains when filter has them. See spaceList for the scope and the
// failure rules.
func (r *Reader) UnitListJSON(ctx context.Context, space string, filter Filter) ([]byte, error) {
	return spaceList(ctx, r, "unit", space, filter, unitListInclude,
		func(q listQuery) (listAnswer[goclientnew.ExtendedUnit], error) {
			resp, err := r.client.API.ListAllUnitsWithResponse(ctx, &goclientnew.ListAllUnitsParams{Where: q.where, Include: q.include, Contains: q.contains})
			if resp == nil {
				return listAnswer[goclientnew.ExtendedUnit]{}, err
			}
			return listAnswer[goclientnew.ExtendedUnit]{resp, resp.JSON200, resp.HTTPResponse, rejection(resp.StatusCode(), resp.JSON400)}, err
		},
		func(entry *goclientnew.ExtendedUnit) (uuid.UUID, bool) {
			if entry.Unit == nil {
				return uuid.Nil, false
			}
			return entry.Unit.SpaceID, true
		})
}

// WorkerListJSON reads the workers of exactly one space, as
// `cub worker list --space <space> -o json` prints them, without one field:
// each worker's Secret.
//
// The server returns the token a worker authenticates with to anyone who may
// list the workers, and cub prints it. An observer has no use for it, and
// what this returns is parsed, logged and recorded, so it is dropped here
// before anything else sees it. This is the one place the reader's JSON is
// not cub's.
func (r *Reader) WorkerListJSON(ctx context.Context, space string, filter Filter) ([]byte, error) {
	return spaceList(ctx, r, "worker", space, filter, workerListInclude,
		func(q listQuery) (listAnswer[goclientnew.ExtendedBridgeWorker], error) {
			resp, err := r.client.API.ListAllBridgeWorkersWithResponse(ctx, &goclientnew.ListAllBridgeWorkersParams{Where: q.where, Include: q.include, Contains: q.contains})
			if resp == nil {
				return listAnswer[goclientnew.ExtendedBridgeWorker]{}, err
			}
			if resp.JSON200 != nil {
				for i := range *resp.JSON200 {
					if worker := (*resp.JSON200)[i].BridgeWorker; worker != nil {
						worker.Secret = ""
					}
				}
			}
			return listAnswer[goclientnew.ExtendedBridgeWorker]{resp, resp.JSON200, resp.HTTPResponse, rejection(resp.StatusCode(), resp.JSON400)}, err
		},
		func(entry *goclientnew.ExtendedBridgeWorker) (uuid.UUID, bool) {
			if entry.BridgeWorker == nil {
				return uuid.Nil, false
			}
			return entry.BridgeWorker.SpaceID, true
		})
}

// TargetListJSON reads the targets of exactly one space, as
// `cub target list --space <space> -o json` prints them.
func (r *Reader) TargetListJSON(ctx context.Context, space string, filter Filter) ([]byte, error) {
	return spaceList(ctx, r, "target", space, filter, targetListInclude,
		func(q listQuery) (listAnswer[goclientnew.ExtendedTarget], error) {
			resp, err := r.client.API.ListAllTargetsWithResponse(ctx, &goclientnew.ListAllTargetsParams{Where: q.where, Include: q.include, Contains: q.contains})
			if resp == nil {
				return listAnswer[goclientnew.ExtendedTarget]{}, err
			}
			return listAnswer[goclientnew.ExtendedTarget]{resp, resp.JSON200, resp.HTTPResponse, rejection(resp.StatusCode(), resp.JSON400)}, err
		},
		func(entry *goclientnew.ExtendedTarget) (uuid.UUID, bool) {
			if entry.Target == nil {
				return uuid.Nil, false
			}
			return entry.Target.SpaceID, true
		})
}

// ChangeSetListJSON reads the change sets of exactly one space, as
// `cub changeset list --space <space> -o json` prints them.
func (r *Reader) ChangeSetListJSON(ctx context.Context, space string, filter Filter) ([]byte, error) {
	return spaceList(ctx, r, "changeset", space, filter, changeSetListInclude,
		func(q listQuery) (listAnswer[goclientnew.ExtendedChangeSet], error) {
			resp, err := r.client.API.ListAllChangeSetsWithResponse(ctx, &goclientnew.ListAllChangeSetsParams{Where: q.where, Include: q.include, Contains: q.contains})
			if resp == nil {
				return listAnswer[goclientnew.ExtendedChangeSet]{}, err
			}
			return listAnswer[goclientnew.ExtendedChangeSet]{resp, resp.JSON200, resp.HTTPResponse, rejection(resp.StatusCode(), resp.JSON400)}, err
		},
		func(entry *goclientnew.ExtendedChangeSet) (uuid.UUID, bool) {
			if entry.ChangeSet == nil {
				return uuid.Nil, false
			}
			return entry.ChangeSet.SpaceID, true
		})
}

// LinkListJSON reads the links of exactly one space, as
// `cub link list --space <space> -o json` prints them. cub lists links
// through the search endpoint, and so does this.
func (r *Reader) LinkListJSON(ctx context.Context, space string, filter Filter) ([]byte, error) {
	return spaceList(ctx, r, "link", space, filter, linkListInclude,
		func(q listQuery) (listAnswer[goclientnew.ExtendedLink], error) {
			resp, err := r.client.API.SearchListLinksWithResponse(ctx, &goclientnew.SearchListLinksParams{Where: q.where, Include: q.include, Contains: q.contains})
			if resp == nil {
				return listAnswer[goclientnew.ExtendedLink]{}, err
			}
			return listAnswer[goclientnew.ExtendedLink]{resp, resp.JSON200, resp.HTTPResponse, rejection(resp.StatusCode(), resp.JSON400)}, err
		},
		func(entry *goclientnew.ExtendedLink) (uuid.UUID, bool) {
			if entry.Link == nil {
				return uuid.Nil, false
			}
			return entry.Link.SpaceID, true
		})
}

// AttestationListJSON reads the attestations of exactly one space, as
// `cub attestation list --space <space> -o json` prints them.
func (r *Reader) AttestationListJSON(ctx context.Context, space string, filter Filter) ([]byte, error) {
	return spaceList(ctx, r, "attestation", space, filter, "",
		func(q listQuery) (listAnswer[goclientnew.ExtendedAttestation], error) {
			resp, err := r.client.API.ListAllAttestationsWithResponse(ctx, &goclientnew.ListAllAttestationsParams{Where: q.where, Include: q.include, Contains: q.contains})
			if resp == nil {
				return listAnswer[goclientnew.ExtendedAttestation]{}, err
			}
			return listAnswer[goclientnew.ExtendedAttestation]{resp, resp.JSON200, resp.HTTPResponse, rejection(resp.StatusCode(), resp.JSON400)}, err
		},
		func(entry *goclientnew.ExtendedAttestation) (uuid.UUID, bool) {
			if entry.Attestation == nil {
				return uuid.Nil, false
			}
			return entry.Attestation.SpaceID, true
		})
}

// SpaceListJSON reads the organization's spaces and returns the list as the
// JSON `cub space list -o json` prints, with each space's summary counts. The
// spaces themselves are the organization-wide question, so it takes no scope.
func (r *Reader) SpaceListJSON(ctx context.Context) ([]byte, error) {
	const op = "space list"
	include, summary := spaceListInclude, true
	resp, err := r.client.API.ListSpacesWithResponse(ctx, &goclientnew.ListSpacesParams{Include: &include, Summary: &summary})
	if failure := classify(ctx, op, err, resp); failure != nil {
		return nil, failure
	}
	if resp.JSON200 == nil {
		return nil, &Error{Kind: KindMalformed, Op: op, Message: "the response had no space list"}
	}
	if failure := wholeList(op, resp.HTTPResponse); failure != nil {
		return nil, failure
	}
	for i := range *resp.JSON200 {
		if (*resp.JSON200)[i].Space == nil {
			return nil, &Error{Kind: KindMalformed, Op: op, Message: "the list has an entry with no space"}
		}
	}
	return listJSON(op, resp.JSON200)
}

// printable returns text cut to at most limit characters, with everything
// that could act on a terminal rather than be read replaced by a space:
// control characters (C0 and C1, so an escape sequence loses its introducer),
// format characters such as the bidirectional overrides, and the line and
// paragraph separators. It is for a server's message shown to a user.
func printable(text string, limit int) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return ' '
		}
		return r
	}, strings.TrimSpace(text))
	if characters := []rune(cleaned); len(characters) > limit {
		return string(characters[:limit]) + "…"
	}
	return cleaned
}

// wholeList fails a list the server says it cut short. Asked for no limit, the
// server returns every entity; a continue token on the response means it did
// not, and following it is not implemented here. cub ignores the token on such
// a request and prints the part it got.
func wholeList(op string, response *http.Response) *Error {
	if response != nil && response.Header.Get(constants.ContinueHeader) != "" {
		return &Error{Kind: KindIncomplete, Op: op, Message: "the server returned part of the list and a token for the rest; this reader does not read in pages"}
	}
	return nil
}

// spaceExists checks that id names a space, the way cub resolves a space
// given by its ID.
func (r *Reader) spaceExists(ctx context.Context, id uuid.UUID) *Error {
	const op = "space lookup"
	filter, limit := cubapi.Where{}.SpaceID(id).String(), 2
	resp, err := r.client.API.ListSpacesWithResponse(ctx, &goclientnew.ListSpacesParams{Where: &filter, Limit: &limit})
	if failure := classify(ctx, op, err, resp); failure != nil {
		return failure
	}
	if resp.JSON200 == nil {
		return &Error{Kind: KindMalformed, Op: op, Message: "the response had no space list"}
	}
	for i := range *resp.JSON200 {
		if found := (*resp.JSON200)[i].Space; found != nil && found.SpaceID == id {
			return nil
		}
	}
	return &Error{Kind: KindNotFound, Op: op, Message: fmt.Sprintf("no space with ID %q", id)}
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
