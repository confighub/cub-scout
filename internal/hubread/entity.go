package hubread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/confighub/sdk/core/cubapi"
	goclientnew "github.com/confighub/sdk/core/openapi/goclient-new"
	"github.com/google/uuid"
)

// This file holds the reads of one thing: a Unit's data, a View, a change
// order, a revision and its data. Each finds the one entity the way cub's
// resolver does, by a filtered list that must hold exactly one match, and
// returns what cub prints for it.

// The expansions cub asks for when it gets one View, one change order, and
// the revisions of a Unit (the SDK's viewGetInclude and changeOrderGetInclude,
// and the list cub's revision commands send).
const (
	viewGetInclude        = "SpaceID,FilterID"
	changeOrderGetInclude = "SpaceID,StartTagID,EndTagID,RestoreTagID,InvocationID,UnitFilterID"
	revisionListInclude   = "UserID,UnitID,ChangeSetID,Tags,ChangeOrders,Releases"
)

// one encodes the single entity a get found, as cub's -o json prints it.
func one(op string, entity any) ([]byte, error) {
	encoded, err := json.MarshalIndent(entity, "", "  ")
	if err != nil {
		return nil, &Error{Kind: KindMalformed, Op: op, Message: "the answer could not be encoded: " + err.Error()}
	}
	return append(encoded, '\n'), nil
}

// rejected adds the server's reason to the failure of a request it answered
// with HTTP 400. body is the response's JSON400, nil when there was no
// response or no such body.
func rejected(failure *Error, body *goclientnew.StandardErrorResponse) *Error {
	if failure.Kind == KindFailed && strings.HasPrefix(failure.Message, "HTTP 400") && body != nil {
		if reason := printable(body.Message, 300); reason != "" {
			failure.Message = "the server rejected the request (HTTP 400): " + reason
		}
	}
	return failure
}

// UnitData reads the configuration data of exactly one Unit in exactly one
// space: the bytes `cub unit data <unit> --space <space>` writes, unchanged.
//
// The Unit is found as UnitJSON finds it, and its data is then downloaded
// from the Unit's own endpoint. The Unit reports the SHA-256 of its data;
// when it does, the bytes downloaded must have that hash. A page from
// something in between, a truncated body and a Unit that changed between the
// two requests all fail that, and none is returned as the Unit's data. cub
// makes no such check.
func (r *Reader) UnitData(ctx context.Context, space, unit string) ([]byte, error) {
	const op = "unit data"
	found, _, failure := r.unit(ctx, op, space, unit)
	if failure != nil {
		return nil, failure
	}
	resp, err := r.client.API.DownloadUnitDataWithResponse(ctx, found.Unit.SpaceID, found.Unit.UnitID)
	if failure := classify(ctx, op, err, resp); failure != nil {
		if resp != nil && resp.StatusCode() == http.StatusNotFound {
			// The Unit was there a request ago, so this is not a missing
			// Unit. It is a server with no data endpoint.
			failure.Message = "the server has no data endpoint for the unit (HTTP 404)"
		}
		return nil, failure
	}
	if failure := dataMatches(op, resp.Body, found.Unit.DataHash, fmt.Sprintf("unit %q", unit)); failure != nil {
		return nil, failure
	}
	return resp.Body, nil
}

// dataMatches holds downloaded data to the SHA-256 its owner reports. An
// owner that reports no hash cannot be checked, and is not.
func dataMatches(op string, data []byte, reported, owner string) *Error {
	reported = strings.TrimSpace(reported)
	if reported == "" {
		return nil
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), reported) {
		return &Error{Kind: KindMalformed, Op: op, Message: fmt.Sprintf(
			"the data downloaded for %s (%d bytes) does not have the hash ConfigHub reports for it; it changed while it was read, or the answer is not its data", owner, len(data))}
	}
	return nil
}

// revision finds revision number of exactly one Unit in exactly one space.
// cub lists the Unit's revisions filtered to that number and takes the one
// whose number it is; so does this, and more than one is an error.
func (r *Reader) revision(ctx context.Context, op, space, unit string, number int64) (*goclientnew.ExtendedUnit, *goclientnew.ExtendedRevision, *Error) {
	if number <= 0 {
		return nil, nil, &Error{Kind: KindInvalidScope, Op: op, Message: "a revision number above zero is required"}
	}
	found, _, failure := r.unit(ctx, op, space, unit)
	if failure != nil {
		return nil, nil, failure
	}
	where, include := fmt.Sprintf("RevisionNum = %d", number), revisionListInclude
	resp, err := r.client.API.ListExtendedRevisionsWithResponse(ctx, found.Unit.SpaceID, found.Unit.UnitID,
		&goclientnew.ListExtendedRevisionsParams{Where: &where, Include: &include})
	if failure := classify(ctx, op, err, resp); failure != nil {
		if resp != nil {
			failure = rejected(failure, resp.JSON400)
		}
		return nil, nil, failure
	}
	if resp.JSON200 == nil {
		return nil, nil, &Error{Kind: KindMalformed, Op: op, Message: "the response had no revision list"}
	}
	if failure := wholeList(op, resp.HTTPResponse); failure != nil {
		return nil, nil, failure
	}
	var matches []*goclientnew.ExtendedRevision
	for i := range *resp.JSON200 {
		entry := &(*resp.JSON200)[i]
		if entry.Revision == nil || entry.Revision.RevisionNum != number {
			continue
		}
		// The endpoint is the Unit's own. A revision of anything else
		// under it is not an answer to trust in part.
		if entry.Revision.UnitID != found.Unit.UnitID || entry.Revision.SpaceID != found.Unit.SpaceID {
			return nil, nil, &Error{Kind: KindMalformed, Op: op, BeyondScope: true,
				Message: fmt.Sprintf("the server returned a revision of another unit for unit %q in space %q", unit, space)}
		}
		matches = append(matches, entry)
	}
	switch len(matches) {
	case 0:
		return nil, nil, &Error{Kind: KindNotFound, Op: op, Message: fmt.Sprintf("no revision %d of unit %q in space %q", number, unit, space)}
	case 1:
		return found, matches[0], nil
	default:
		return nil, nil, &Error{Kind: KindAmbiguous, Op: op, Message: fmt.Sprintf("more than one revision %d of unit %q in space %q", number, unit, space)}
	}
}

// RevisionJSON reads one revision of exactly one Unit in exactly one space,
// as `cub revision get <unit> <number> --space <space> -o json` prints it.
func (r *Reader) RevisionJSON(ctx context.Context, space, unit string, number int64) ([]byte, error) {
	const op = "revision get"
	_, found, failure := r.revision(ctx, op, space, unit, number)
	if failure != nil {
		return nil, failure
	}
	return one(op, found)
}

// RevisionData reads the configuration data of one revision of exactly one
// Unit in exactly one space: the bytes `cub revision data <unit> <number>
// --space <space>` writes. A revision with no data is an error, as it is for
// cub, and the bytes are held to the hash the revision reports, as UnitData
// holds a Unit's.
func (r *Reader) RevisionData(ctx context.Context, space, unit string, number int64) ([]byte, error) {
	const op = "revision data"
	owner, found, failure := r.revision(ctx, op, space, unit, number)
	if failure != nil {
		return nil, failure
	}
	if found.Revision.RevisionID == uuid.Nil {
		return nil, &Error{Kind: KindMalformed, Op: op, Message: fmt.Sprintf("revision %d of unit %q has no ID", number, unit)}
	}
	resp, err := r.client.API.DownloadRevisionDataWithResponse(ctx, owner.Unit.SpaceID, owner.Unit.UnitID, found.Revision.RevisionID)
	if failure := classify(ctx, op, err, resp); failure != nil {
		if resp != nil && resp.StatusCode() == http.StatusNotFound {
			failure.Message = "the server has no data endpoint for the revision (HTTP 404)"
		}
		return nil, failure
	}
	if len(resp.Body) == 0 {
		return nil, &Error{Kind: KindNotFound, Op: op, Message: fmt.Sprintf("no config data for revision %d of unit %q", number, unit)}
	}
	if failure := dataMatches(op, resp.Body, found.Revision.DataHash, fmt.Sprintf("revision %d of unit %q", number, unit)); failure != nil {
		return nil, failure
	}
	return resp.Body, nil
}

// canonical returns id as a UUID when it is one exactly as ConfigHub prints
// it, lower case and 36 characters.
func canonical(op, what, id string) (uuid.UUID, *Error) {
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id || parsed == uuid.Nil {
		return uuid.Nil, &Error{Kind: KindInvalidScope, Op: op, Message: "a " + what + " ID in its canonical form is required"}
	}
	return parsed, nil
}

// ViewJSON reads the one View with this ID, as `cub view get <id> --space
// <space> -o json` prints it.
//
// An ID names one View wherever it is. cub checks that the space it was given
// exists and then finds the View by ID alone, in whatever space it is; so does
// this. space is a slug, a canonical ID, or "*" for no space.
func (r *Reader) ViewJSON(ctx context.Context, space, id string) ([]byte, error) {
	const op = "view get"
	viewID, scopeErr := canonical(op, "view", strings.TrimSpace(id))
	if scopeErr != nil {
		return nil, scopeErr
	}
	switch space = strings.TrimSpace(space); space {
	case "":
		return nil, &Error{Kind: KindInvalidScope, Op: op, Message: `a space, or "*" for every space, is required`}
	case "*":
	default:
		spaceID, slug, scopeErr := r.namedSpace(ctx, space)
		if scopeErr != nil {
			return nil, scopeErr
		}
		if slug == "" {
			// Named by ID, so not looked up yet.
			if missing := r.spaceExists(ctx, spaceID); missing != nil {
				return nil, missing
			}
		}
	}
	where, include := cubapi.Where{}.Eq("ViewID", viewID.String()).String(), viewGetInclude
	resp, err := r.client.API.ListAllViewsWithResponse(ctx, &goclientnew.ListAllViewsParams{Where: &where, Include: &include})
	if failure := classify(ctx, op, err, resp); failure != nil {
		if resp != nil {
			failure = rejected(failure, resp.JSON400)
		}
		return nil, failure
	}
	if resp.JSON200 == nil {
		return nil, &Error{Kind: KindMalformed, Op: op, Message: "the response had no view list"}
	}
	if failure := wholeList(op, resp.HTTPResponse); failure != nil {
		return nil, failure
	}
	var matches []*goclientnew.ExtendedView
	for i := range *resp.JSON200 {
		if entry := &(*resp.JSON200)[i]; entry.View != nil && entry.View.ViewID == viewID {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 0:
		return nil, &Error{Kind: KindNotFound, Op: op, Message: fmt.Sprintf("no view with ID %q", viewID)}
	case 1:
		return one(op, matches[0])
	default:
		return nil, &Error{Kind: KindAmbiguous, Op: op, Message: fmt.Sprintf("more than one view with ID %q", viewID)}
	}
}

// ChangeOrderJSON reads exactly one change order in exactly one space, named
// by its slug, as `cub changeorder get <slug> --space <space> -o json` prints
// it.
func (r *Reader) ChangeOrderJSON(ctx context.Context, space, order string) ([]byte, error) {
	const op = "changeorder get"
	space, order = strings.TrimSpace(space), strings.TrimSpace(order)
	switch {
	case space == "" || space == "*":
		return nil, &Error{Kind: KindInvalidScope, Op: op, Message: "exactly one space is required"}
	case order == "" || order == "*":
		return nil, &Error{Kind: KindInvalidScope, Op: op, Message: "exactly one change order is required"}
	}
	named, scopeErr := literal(op, "Slug", order)
	if scopeErr != nil {
		return nil, scopeErr
	}
	spaceID, _, scopeErr := r.namedSpace(ctx, space)
	if scopeErr != nil {
		return nil, scopeErr
	}
	where, include := named.SpaceID(spaceID).String(), changeOrderGetInclude
	resp, err := r.client.API.ListAllChangeOrdersWithResponse(ctx, &goclientnew.ListAllChangeOrdersParams{Where: &where, Include: &include})
	if failure := classify(ctx, op, err, resp); failure != nil {
		if resp != nil {
			failure = rejected(failure, resp.JSON400)
		}
		return nil, failure
	}
	if resp.JSON200 == nil {
		return nil, &Error{Kind: KindMalformed, Op: op, Message: "the response had no change order list"}
	}
	if failure := wholeList(op, resp.HTTPResponse); failure != nil {
		return nil, failure
	}
	var matches []*goclientnew.ExtendedChangeOrder
	for i := range *resp.JSON200 {
		entry := &(*resp.JSON200)[i]
		if found := entry.ChangeOrder; found != nil && found.Slug == order && found.SpaceID == spaceID {
			matches = append(matches, entry)
		}
	}
	switch len(matches) {
	case 0:
		return nil, &Error{Kind: KindNotFound, Op: op, Message: fmt.Sprintf("no change order %q in space %q", order, space)}
	case 1:
		return one(op, matches[0])
	default:
		return nil, &Error{Kind: KindAmbiguous, Op: op, Message: fmt.Sprintf("more than one change order %q in space %q", order, space)}
	}
}
