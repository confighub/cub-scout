// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

// The answer block on `watch` and `bot` JSONL/webhook events (#596):
// deterministic, compact, read-only envelope built from the event's existing
// observation without extra cluster reads.
//
// Reuses doctor's approved state words exclusively (zero new state words):
// issues-flagged, nothing-flagged, incomplete, not-observable.

import (
	"fmt"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
)

const (
	watchReasonFindingFlagged         = "finding-flagged"
	watchReasonDriftDetected          = "drift-detected"
	watchReasonOwnershipChanged       = "ownership-changed"
	watchReasonResourceDiscovered     = "resource-discovered"
	watchReasonResourceDeleted        = "resource-deleted"
	watchReasonInfoObserved           = "info-observed"
	watchReasonClusterObserved        = "cluster-observed"
	watchReasonCollectionPartial      = "collection-partial"
	watchReasonInventoryNotRead       = "inventory-not-read"
	watchReasonClusterIdentityNotRead = "cluster-identity-not-read"
)

// watchReasons is the closed list of documented reason keys for watch/bot
// event answers. State words reuse doctorStates with zero additions.
var watchReasons = []string{
	watchReasonFindingFlagged,
	watchReasonDriftDetected,
	watchReasonOwnershipChanged,
	watchReasonResourceDiscovered,
	watchReasonResourceDeleted,
	watchReasonInfoObserved,
	watchReasonClusterObserved,
	watchReasonCollectionPartial,
	watchReasonInventoryNotRead,
	watchReasonClusterIdentityNotRead,
}

// buildWatchEventAnswer builds a compact, conforming Answer envelope for a
// single watch/bot event from data already collected in this cycle.
func buildWatchEventAnswer(event watchEvent, state watchState) *mapsvc.Answer {
	ts := event.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	answer := &mapsvc.Answer{
		Cluster:    mapsvc.AnswerCluster{Context: state.context, IDSource: "not-read"},
		ObservedAt: ts.UTC().Format(time.RFC3339),
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
		Cost:       mapsvc.AnswerCost{Coverage: "not-measured"},
	}
	if event.Cluster != nil {
		idSource := event.Cluster.IDSource
		if idSource == "" {
			idSource = "not-read"
		}
		answer.Cluster = mapsvc.AnswerCluster{
			Context:  firstNonEmpty(event.Cluster.Context, state.context),
			ID:       event.Cluster.ID,
			IDSource: idSource,
		}
		if event.Type == "cluster.observed" && event.Cluster.Cost.Coverage != "" {
			answer.Cost = mapsvc.AnswerCostFrom(event.Cluster.Cost)
		}
	}

	resLabel := watchSafeResourceLabel(event.Resource)
	scope := watchSafeScopeLabel(firstNonEmpty(event.Resource.Namespace, state.namespace))
	ref := watchSafeRef(event.Resource)

	switch event.Type {
	case "scan.finding":
		msg := watchEventDetailString(event.Details, "message")
		if strings.EqualFold(event.Severity, "info") {
			answer.State = doctorStateNothingFlagged
			answer.Reason = watchReasonInfoObserved
			answer.Summary = fmt.Sprintf("Watch observed an informational finding on %s in %s.", resLabel, scope)
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
				Claim:   "state scan reported an informational finding",
				RestsOn: mapsvc.RestsOnCubScoutComparison,
				Ref:     ref,
				Quoted:  msg,
			})
		} else {
			answer.State = doctorStateIssuesFlagged
			answer.Reason = watchReasonFindingFlagged
			answer.Summary = fmt.Sprintf("Watch flagged a scan finding on %s in %s.", resLabel, scope)
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
				Claim:   "state scan reported a finding",
				RestsOn: mapsvc.RestsOnCubScoutComparison,
				Ref:     ref,
				Quoted:  msg,
			})
			if next, ok := watchExplainNext(event.Resource, "inspect owner and recent field managers"); ok {
				answer.Next = append(answer.Next, next)
			}
		}

	case "drift.detected":
		msg := watchEventDetailString(event.Details, "message")
		if strings.EqualFold(event.Severity, "info") {
			answer.State = doctorStateNothingFlagged
			answer.Reason = watchReasonInfoObserved
			answer.Summary = fmt.Sprintf("Watch observed informational drift on %s in %s.", resLabel, scope)
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
				Claim:   "state scan reported informational drift",
				RestsOn: mapsvc.RestsOnCubScoutComparison,
				Ref:     ref,
				Quoted:  msg,
			})
		} else {
			answer.State = doctorStateIssuesFlagged
			answer.Reason = watchReasonDriftDetected
			answer.Summary = fmt.Sprintf("Watch flagged drift on %s in %s.", resLabel, scope)
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
				Claim:   "state scan detected drift",
				RestsOn: mapsvc.RestsOnCubScoutComparison,
				Ref:     ref,
				Quoted:  msg,
			})
			if next, ok := watchTraceNext(event.Resource, "inspect delivery chain for this resource"); ok {
				answer.Next = append(answer.Next, next)
			}
		}

	case "ownership.changed":
		before := watchEventDetailString(event.Details, "before")
		after := watchEventDetailString(event.Details, "after")
		quoted := ""
		if before != "" || after != "" {
			quoted = fmt.Sprintf("before=%s after=%s", before, after)
		}
		answer.State = doctorStateIssuesFlagged
		answer.Reason = watchReasonOwnershipChanged
		answer.Summary = fmt.Sprintf("Watch flagged an ownership change on %s in %s.", resLabel, scope)
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim:   "ownership marker changed between watch cycles",
			RestsOn: mapsvc.RestsOnObjectMetadata,
			Ref:     ref,
			Quoted:  quoted,
		})
		if next, ok := watchExplainNext(event.Resource, "verify current owner and recent field managers"); ok {
			answer.Next = append(answer.Next, next)
		}

	case "resource.discovered":
		answer.State = doctorStateNothingFlagged
		answer.Reason = watchReasonResourceDiscovered
		answer.Summary = fmt.Sprintf("Watch observed %s in %s.", resLabel, scope)
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim:   "resource observed in inventory",
			RestsOn: mapsvc.RestsOnObjectMetadata,
			Ref:     ref,
		})
		if next, ok := watchExplainNext(event.Resource, "inspect ownership and field managers"); ok {
			answer.Next = append(answer.Next, next)
		}

	case "resource.deleted":
		answer.State = doctorStateNothingFlagged
		answer.Reason = watchReasonResourceDeleted
		answer.Summary = fmt.Sprintf("Watch observed %s removed from %s.", resLabel, scope)
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim:   "resource absent from inventory after prior cycle",
			RestsOn: mapsvc.RestsOnCubScoutComparison,
		})
		if next, ok := watchMapListNext(event.Resource.Namespace, "list current inventory in scope"); ok {
			answer.Next = append(answer.Next, next)
		}

	case "collection.partial":
		var omissions []mapsvc.CollectionOmission
		if event.Collection != nil && len(event.Collection.Omissions) > 0 {
			omissions = event.Collection.Omissions
		} else {
			omissions = state.omissions
		}
		const maxListedOmissions = 2
		for i, omission := range omissions {
			if i >= maxListedOmissions {
				answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
					What:   fmt.Sprintf("%d additional resource kind(s)", len(omissions)-maxListedOmissions),
					Kind:   mapsvc.UnknownNotObservable,
					Reason: "additional inventory lists could not be read",
				})
				break
			}
			what := strings.TrimSpace(omission.Resource)
			if what == "" {
				what = "resource list"
			}
			kind := mapsvc.UnknownNotObservable
			if strings.Contains(strings.ToLower(omission.Reason), "forbidden") {
				kind = mapsvc.UnknownDenied
			}
			answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
				What:   what,
				Kind:   kind,
				Reason: "kind could not be listed; reason is quoted",
				Quoted: omission.Reason,
			})
		}
		if len(state.entriesByID) == 0 {
			answer.State = doctorStateNotObservable
			answer.Reason = watchReasonInventoryNotRead
			answer.Summary = fmt.Sprintf("No inventory could be read in %s.", scope)
		} else {
			answer.State = doctorStateIncomplete
			answer.Reason = watchReasonCollectionPartial
			answer.Summary = fmt.Sprintf("Inventory is incomplete in %s; some kinds could not be listed.", scope)
		}
		if next, ok := watchMapListNext(firstNonEmpty(event.Resource.Namespace, state.namespace), "inspect inventory summary"); ok {
			answer.Next = append(answer.Next, next)
		}

	case "cluster.observed":
		if event.Cluster != nil && event.Cluster.Identity == "verified" {
			answer.State = doctorStateNothingFlagged
			answer.Reason = watchReasonClusterObserved
			answer.Summary = "Watch verified cluster identity."
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
				Claim:   "cluster identity verified",
				RestsOn: mapsvc.RestsOnObjectMetadata,
			})
		} else {
			omission := ""
			if event.Cluster != nil {
				omission = event.Cluster.Omission
			}
			kind := mapsvc.UnknownNotObservable
			if strings.Contains(strings.ToLower(omission), "forbidden") {
				kind = mapsvc.UnknownDenied
			}
			answer.State = doctorStateNotObservable
			answer.Reason = watchReasonClusterIdentityNotRead
			answer.Summary = "Cluster identity could not be verified."
			answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
				What:   "cluster identity",
				Kind:   kind,
				Reason: "identity check failed; reason is quoted",
				Quoted: omission,
			})
		}

	default:
		answer.State = doctorStateNotObservable
		answer.Reason = watchReasonInventoryNotRead
		answer.Summary = "Watch emitted an unrecognized event."
	}

	return answer
}

func watchEventDetailString(details map[string]interface{}, key string) string {
	if details == nil {
		return ""
	}
	v, ok := details[key]
	if !ok || v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func watchSafeResourceLabel(res watchEventResource) string {
	if traceSafeKind.MatchString(res.Kind) && traceSafeName.MatchString(res.Name) {
		return res.Kind + "/" + res.Name
	}
	return "this resource"
}

func watchSafeScopeLabel(namespace string) string {
	if namespace == "" {
		return "this cluster"
	}
	if traceSafeName.MatchString(namespace) {
		return "namespace " + namespace
	}
	return "this namespace"
}

func watchSafeRef(res watchEventResource) *mapsvc.AnswerRef {
	if !traceSafeKind.MatchString(res.Kind) || !traceSafeName.MatchString(res.Name) {
		return nil
	}
	if res.Namespace != "" && !traceSafeName.MatchString(res.Namespace) {
		return nil
	}
	return &mapsvc.AnswerRef{
		Kind:      res.Kind,
		Namespace: res.Namespace,
		Name:      res.Name,
	}
}

func watchExplainNext(res watchEventResource, why string) (mapsvc.AnswerNext, bool) {
	if !traceSafeKind.MatchString(res.Kind) || !traceSafeName.MatchString(res.Name) {
		return mapsvc.AnswerNext{}, false
	}
	if res.Namespace != "" && !traceSafeName.MatchString(res.Namespace) {
		return mapsvc.AnswerNext{}, false
	}
	ns := ""
	if res.Namespace != "" {
		ns = " -n " + res.Namespace
	}
	return mapsvc.AnswerNext{
		Command: preferInvocationForm("cub-scout explain " + res.Kind + "/" + res.Name + ns + " --format json"),
		Why:     why,
	}, true
}

func watchTraceNext(res watchEventResource, why string) (mapsvc.AnswerNext, bool) {
	if !traceSafeKind.MatchString(res.Kind) || !traceSafeName.MatchString(res.Name) {
		return mapsvc.AnswerNext{}, false
	}
	if res.Namespace != "" && !traceSafeName.MatchString(res.Namespace) {
		return mapsvc.AnswerNext{}, false
	}
	ns := ""
	if res.Namespace != "" {
		ns = " -n " + res.Namespace
	}
	return mapsvc.AnswerNext{
		Command: preferInvocationForm("cub-scout trace " + res.Kind + "/" + res.Name + ns + " --format json"),
		Why:     why,
	}, true
}

func watchMapListNext(namespace, why string) (mapsvc.AnswerNext, bool) {
	ns := ""
	if namespace != "" {
		if !traceSafeName.MatchString(namespace) {
			return mapsvc.AnswerNext{}, false
		}
		ns = " -n " + namespace
	}
	return mapsvc.AnswerNext{
		Command: preferInvocationForm("cub-scout map list" + ns + " --summary --format json"),
		Why:     why,
	}, true
}
