package main

// The answer block on `doctor --format json`: is anything wrong in this
// scope? The state words were decided by the maintainer on 2026-11-19.
// "nothing-flagged" claims only what doctor checked; it is never "healthy".

import (
	"fmt"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
)

const (
	doctorStateIssuesFlagged  = "issues-flagged"
	doctorStateNothingFlagged = "nothing-flagged"
	doctorStateIncomplete     = "incomplete"
	doctorStateNotObservable  = "not-observable"

	doctorReasonIssuesFound        = "issues-found"
	doctorReasonNothingFound       = "nothing-found"
	doctorReasonCoverageIncomplete = "coverage-incomplete"
	doctorReasonInventoryNotRead   = "inventory-not-read"

	// doctorCostCoverage is a key, spelled out in json-contracts.md: the
	// Kubernetes requests of doctor's own in-process clients. Not counted:
	// subprocesses and ConfigHub reads.
	doctorCostCoverage = "doctor-session-clients"
)

// doctorStates and doctorReasons are what the conformance test checks
// answers against. Adding a value means documenting it in json-contracts.md.
var (
	doctorStates  = []string{doctorStateIssuesFlagged, doctorStateNothingFlagged, doctorStateIncomplete, doctorStateNotObservable}
	doctorReasons = []string{doctorReasonIssuesFound, doctorReasonNothingFound, doctorReasonCoverageIncomplete, doctorReasonInventoryNotRead}
)

// buildDoctorAnswer puts a doctor summary into the common envelope. It adds
// no observation.
func buildDoctorAnswer(summary DoctorSummary, observedAt time.Time) *mapsvc.Answer {
	answer := &mapsvc.Answer{
		Cluster:    mapsvc.AnswerCluster{Context: summary.KubernetesContext, IDSource: "not-read"},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
		Cost:       mapsvc.AnswerCost{Coverage: "not-measured"},
	}
	if summary.readCost != nil {
		answer.Cost = mapsvc.AnswerCostFrom(*summary.readCost)
	}

	// What doctor flags, by whose word.
	flagged := 0
	if n := summary.Risks.Critical + summary.Risks.Warning; n > 0 {
		flagged += n
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim:   fmt.Sprintf("the risk scan found %d critical and %d warning finding(s)", summary.Risks.Critical, summary.Risks.Warning),
			RestsOn: mapsvc.RestsOnCubScoutComparison,
		})
	}
	if summary.Risks.Info > 0 {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim:   fmt.Sprintf("the risk scan found %d informational finding(s), which flag nothing", summary.Risks.Info),
			RestsOn: mapsvc.RestsOnCubScoutComparison,
		})
	}
	if n := summary.Health.Error; n > 0 {
		flagged += n
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim: fmt.Sprintf("%d resource(s) report Failed", n), RestsOn: mapsvc.RestsOnControllerReport,
		})
	}
	if summary.Rollouts != nil && summary.Rollouts.Block > 0 {
		flagged += summary.Rollouts.Block
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim: fmt.Sprintf("%d current rollout(s) are blocked", summary.Rollouts.Block), RestsOn: mapsvc.RestsOnCubScoutComparison,
		})
	}
	if d := summary.Delivery; d != nil {
		if n := d.LiveStatus.Delivery.Block + d.LiveStatus.ApplicationHealth.Block; n > 0 {
			flagged += n
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
				Claim: fmt.Sprintf("%d recorded live status(es) block delivery or application health", n), RestsOn: mapsvc.RestsOnDeployerReport,
			})
		}
		if n := d.EventConsumers.NotReady; n > 0 {
			flagged += n
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
				Claim: fmt.Sprintf("%d event consumer(s) are not ready", n), RestsOn: mapsvc.RestsOnControllerReport,
			})
		}
	}

	// Every read that failed is an unknown, its text quoted.
	inventoryFailed := false
	for _, warning := range summary.Warnings {
		unknown := mapsvc.AnswerUnknown{What: doctorWarningSubject(warning), Kind: mapsvc.UnknownNotObservable, Reason: "the read failed or was incomplete; its message is quoted", Quoted: warning}
		if strings.Contains(strings.ToLower(warning), "forbidden") {
			unknown.Kind = mapsvc.UnknownDenied
		}
		if strings.HasPrefix(warning, "inventory coverage incomplete") {
			inventoryFailed = true
		}
		answer.Unknowns = append(answer.Unknowns, unknown)
	}

	scope := "this cluster"
	if summary.Namespace != "" && summary.Namespace != "all" {
		scope = "namespace " + summary.Namespace
		if !traceSafeName.MatchString(summary.Namespace) {
			scope = "this namespace"
		}
	}
	switch {
	case summary.Resources.Total == 0 && inventoryFailed:
		answer.State, answer.Reason = doctorStateNotObservable, doctorReasonInventoryNotRead
		answer.Summary = fmt.Sprintf("No resource in %s could be inventoried, so nothing about it is known.", scope)
	case flagged > 0:
		answer.State, answer.Reason = doctorStateIssuesFlagged, doctorReasonIssuesFound
		answer.Summary = fmt.Sprintf("Doctor flagged %d issue(s) in %s.", flagged, scope)
		if len(summary.Warnings) > 0 {
			answer.Summary += " Some reads failed, so there may be more."
		}
		if len(summary.TopIssues) > 0 {
			top := summary.TopIssues[0]
			kind, name, ok := strings.Cut(top.Resource, "/")
			if ok && traceSafeKind.MatchString(kind) && traceSafeName.MatchString(name) && (top.Namespace == "" || traceSafeName.MatchString(top.Namespace)) {
				ns := ""
				if top.Namespace != "" {
					ns = " -n " + top.Namespace
				}
				answer.Next = append(answer.Next, mapsvc.AnswerNext{Command: preferInvocationForm("cub-scout explain " + kind + "/" + name + ns + " --format json"), Why: "who owns the top issue's resource, and who last changed it"})
			}
		}
	case len(summary.Warnings) > 0:
		answer.State, answer.Reason = doctorStateIncomplete, doctorReasonCoverageIncomplete
		answer.Summary = fmt.Sprintf("Doctor flagged nothing in %s, but some reads failed, so that is not established.", scope)
	default:
		answer.State, answer.Reason = doctorStateNothingFlagged, doctorReasonNothingFound
		answer.Summary = fmt.Sprintf("Doctor flagged nothing in %d resource(s) in %s.", summary.Resources.Total, scope)
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What: "anything doctor does not check", Kind: mapsvc.UnknownNotCompared,
			Reason: "nothing-flagged covers the risk scan, resource status and rollouts, not every way a cluster can be wrong",
		})
	}
	return answer
}

// doctorWarningSubject names what a warning is about, in cub-scout's words.
func doctorWarningSubject(warning string) string {
	for prefix, subject := range map[string]string{
		"inventory coverage incomplete":    "part of the resource inventory",
		"risk scan unavailable":            "the risk scan",
		"rollout evidence unavailable":     "rollout evidence",
		"ConfigHub delivery evidence":      "ConfigHub delivery evidence",
		"before-attach report unavailable": "the before-attach report",
		"scan coverage incomplete":         "part of the risk scan",
	} {
		if strings.HasPrefix(warning, prefix) {
			return subject
		}
	}
	return "a doctor read"
}
