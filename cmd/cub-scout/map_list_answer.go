package main

// The answer block on map list's JSON object forms (--summary and
// --cluster-identity): what runs here, and who owns it? --ownership-evidence
// is a compact diagnostic projection, kept smaller than the full list, and
// carries none. State words decided by the maintainer on 2026-11-19. The default
// output is a bare JSON array and carries no answer: adding one would change
// its type for every consumer, a change kept for 3.0.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

const (
	mapStateListed        = "listed"
	mapStatePartial       = "partial"
	mapStateNotObservable = "not-observable"

	mapReasonInventoryListed  = "inventory-listed"
	mapReasonKindsNotListed   = "kinds-not-listed"
	mapReasonInventoryNotRead = "inventory-not-read"
)

// mapStates and mapReasons are what the conformance test checks answers
// against. Adding a value means documenting it in json-contracts.md.
var (
	mapStates  = []string{mapStateListed, mapStatePartial, mapStateNotObservable}
	mapReasons = []string{mapReasonInventoryListed, mapReasonKindsNotListed, mapReasonInventoryNotRead}
)

// buildMapListAnswer puts a map list into the common envelope. identity is
// the cluster identity when it was read (--cluster-identity), else nil.
func buildMapListAnswer(entries []MapEntry, omissions []mapsvc.CollectionOmission, unavailableReason string, identity *agent.ClusterIdentityEvidence, observedAt time.Time) *mapsvc.Answer {
	answer := &mapsvc.Answer{
		Cluster:    mapsvc.AnswerCluster{IDSource: "not-read"},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
		Cost:       mapsvc.AnswerCost{Coverage: "not-measured"},
	}
	if identity != nil {
		answer.Cluster = mapsvc.AnswerCluster{Context: identity.Context, ID: identity.ID, IDSource: identity.IDSource}
		if answer.Cluster.IDSource == "" {
			answer.Cluster.IDSource = "not-read"
		}
		if identity.ObservedAt != nil && !identity.ObservedAt.IsZero() {
			answer.ObservedAt = identity.ObservedAt.UTC().Format(time.RFC3339)
		}
	}

	owners := map[string]int{}
	for _, e := range entries {
		owner := e.Owner
		if owner == "" {
			owner = "Native"
		}
		owners[owner]++
	}
	var names []string
	for owner := range owners {
		names = append(names, owner)
	}
	sort.Strings(names)
	var counts []string
	for _, owner := range names {
		counts = append(counts, fmt.Sprintf("%s %d", owner, owners[owner]))
	}
	if len(entries) > 0 {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim:   fmt.Sprintf("%d resource(s) listed; owner by their own markers: %s", len(entries), strings.Join(counts, ", ")),
			RestsOn: mapsvc.RestsOnObjectMetadata,
		})
	}
	if owners["Native"] > 0 {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   fmt.Sprintf("who manages the %d resource(s) with no recognised owner marker", owners["Native"]),
			Kind:   mapsvc.UnknownNotObservable,
			Reason: "no recognised marker does not show nothing manages them",
		})
	}
	for _, omission := range omissions {
		what := strings.TrimSpace(omission.Resource)
		if omission.APIVersion != "" {
			what += " (" + omission.APIVersion + ")"
		}
		unknown := mapsvc.AnswerUnknown{What: what, Kind: mapsvc.UnknownNotObservable, Reason: "this kind could not be listed; the reason is quoted", Quoted: omission.Reason}
		if strings.Contains(strings.ToLower(omission.Reason), "forbidden") {
			unknown.Kind = mapsvc.UnknownDenied
		}
		answer.Unknowns = append(answer.Unknowns, unknown)
	}

	switch {
	case unavailableReason != "" || (len(entries) == 0 && len(omissions) > 0):
		answer.State, answer.Reason = mapStateNotObservable, mapReasonInventoryNotRead
		answer.Summary = "The inventory could not be read, so what runs here is not known."
		if unavailableReason != "" {
			answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{What: "the inventory", Kind: mapsvc.UnknownNotObservable, Reason: "it was unavailable; the reason is quoted", Quoted: unavailableReason})
		}
	case len(omissions) > 0:
		answer.State, answer.Reason = mapStatePartial, mapReasonKindsNotListed
		answer.Summary = fmt.Sprintf("%d resource(s) listed; %d kind(s) could not be listed, so the inventory is incomplete.", len(entries), len(omissions))
	default:
		answer.State, answer.Reason = mapStateListed, mapReasonInventoryListed
		answer.Summary = fmt.Sprintf("%d resource(s) listed.", len(entries))
	}
	return answer
}
