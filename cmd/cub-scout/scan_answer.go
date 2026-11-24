package main

// The answer block on `scan --json` and `scan --normalized`: what do the
// scanners flag in this scope right now? The state words were decided by the
// maintainer on 2026-11-19 and mirror doctor's. "no-findings" claims only
// what the scanners check; it is never "clean".

import (
	"fmt"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/internal/scan"
	"github.com/confighub/cub-scout/v3/pkg/agent"
)

const (
	scanStateFindings      = "findings"
	scanStateNoFindings    = "no-findings"
	scanStateIncomplete    = "incomplete"
	scanStateNotObservable = "not-observable"

	scanReasonFindingsFound      = "findings-found"
	scanReasonNothingFound       = "nothing-found"
	scanReasonCoverageIncomplete = "coverage-incomplete"
	scanReasonScannersNotRead    = "scanners-not-read"

	// scanCostCoverage is a key, spelled out in json-contracts.md: the
	// Kubernetes requests of the scan's in-process clients. Not counted: an
	// external scanner binary the provider runs as a subprocess.
	scanCostCoverage = "scan-session-clients"
)

// scanStates and scanReasons are what the conformance test checks answers
// against. Adding a value means documenting it in json-contracts.md.
var (
	scanStates  = []string{scanStateFindings, scanStateNoFindings, scanStateIncomplete, scanStateNotObservable}
	scanReasons = []string{scanReasonFindingsFound, scanReasonNothingFound, scanReasonCoverageIncomplete, scanReasonScannersNotRead}
)

// scanNormalizedOutput and scanCombinedOutput carry the answer beside the
// scan's own JSON, whose fields stay where they were.
type scanNormalizedOutput struct {
	*scan.NormalizedResult
	Answer *mapsvc.Answer `json:"answer,omitempty"`
}

type scanCombinedOutput struct {
	*scan.CombinedResult
	Answer *mapsvc.Answer `json:"answer,omitempty"`
}

// scanReadAnything reports whether any scanner returned evidence rather
// than only an error.
func scanReadAnything(result *scan.CombinedResult) bool {
	if result == nil {
		return false
	}
	if result.Kyverno != nil && strings.TrimSpace(result.Kyverno.Error) == "" {
		return true
	}
	return result.State != nil || result.TimingBombs != nil || result.Unresolved != nil || result.Dangling != nil || result.LifecycleHazards != nil
}

// buildScanAnswer puts a live cluster scan into the common envelope. It adds
// no observation.
func buildScanAnswer(result *scan.CombinedResult, namespace string, cost *agent.KubernetesReadCost, observedAt time.Time) *mapsvc.Answer {
	normalized := scan.Normalize(result)
	kubeContext := ""
	if result != nil {
		kubeContext = result.KubernetesContext
	}
	answer := &mapsvc.Answer{
		Cluster:    mapsvc.AnswerCluster{Context: kubeContext, IDSource: "not-read"},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
		Cost:       mapsvc.AnswerCost{Coverage: "not-measured"},
	}
	if cost != nil {
		answer.Cost = mapsvc.AnswerCostFrom(*cost)
	}

	// What the scanners flag, by whose word: Kyverno's PolicyReports are
	// Kyverno's report; every other track is cub-scout's own check.
	flaggedBy := map[bool]int{}
	for _, f := range normalized.Findings {
		if f.Severity == "critical" || f.Severity == "warning" {
			flaggedBy[f.Track == "kyverno"]++
		}
	}
	if n := flaggedBy[true]; n > 0 {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: fmt.Sprintf("Kyverno's policy reports carry %d critical or warning finding(s)", n), RestsOn: mapsvc.RestsOnControllerReport})
	}
	if n := flaggedBy[false]; n > 0 {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: fmt.Sprintf("cub-scout's own checks found %d critical or warning finding(s)", n), RestsOn: mapsvc.RestsOnCubScoutComparison})
	}
	if normalized.Summary.Info > 0 {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{Claim: fmt.Sprintf("%d informational finding(s), which flag nothing", normalized.Summary.Info), RestsOn: mapsvc.RestsOnCubScoutComparison})
	}

	// Every scanner that failed or read part of its scope is an unknown, its
	// text quoted.
	for _, warning := range normalized.Warnings {
		unknown := mapsvc.AnswerUnknown{What: scanWarningSubject(warning), Kind: mapsvc.UnknownNotObservable, Reason: "the scanner failed or read part of its scope; its message is quoted", Quoted: warning}
		if strings.Contains(strings.ToLower(warning), "forbidden") {
			unknown.Kind = mapsvc.UnknownDenied
		}
		answer.Unknowns = append(answer.Unknowns, unknown)
	}

	scope := "this cluster"
	if namespace != "" {
		scope = "namespace " + namespace
		if !traceSafeName.MatchString(namespace) {
			scope = "this namespace"
		}
	}
	flagged := normalized.Summary.Critical + normalized.Summary.Warning
	switch {
	case flagged == 0 && !scanReadAnything(result):
		answer.State, answer.Reason = scanStateNotObservable, scanReasonScannersNotRead
		answer.Summary = fmt.Sprintf("No scanner could read %s, so nothing about it is known.", scope)
		if len(answer.Unknowns) == 0 {
			answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{What: "every scanner's evidence", Kind: mapsvc.UnknownNotObservable, Reason: "no scanner returned a result"})
		}
	case flagged > 0:
		answer.State, answer.Reason = scanStateFindings, scanReasonFindingsFound
		answer.Summary = fmt.Sprintf("Scan flagged %d finding(s) in %s: %d critical, %d warning.", flagged, scope, normalized.Summary.Critical, normalized.Summary.Warning)
		if len(normalized.Warnings) > 0 {
			answer.Summary += " Some scanners failed, so there may be more."
		}
		if next, ok := scanTopFindingNext(normalized.Findings); ok {
			answer.Next = append(answer.Next, next)
		}
	case len(normalized.Warnings) > 0:
		answer.State, answer.Reason = scanStateIncomplete, scanReasonCoverageIncomplete
		answer.Summary = fmt.Sprintf("Scan flagged nothing in %s, but some scanners failed, so that is not established.", scope)
	default:
		answer.State, answer.Reason = scanStateNoFindings, scanReasonNothingFound
		answer.Summary = fmt.Sprintf("Scan flagged nothing in %s.", scope)
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What: "anything the scanners do not check", Kind: mapsvc.UnknownNotCompared,
			Reason: "no-findings covers the scanners that ran, not every way a cluster can be wrong",
		})
	}
	return answer
}

// scanTopFindingNext points at the first critical finding's resource, else
// the first warning's, when its name is safe to put in a command.
func scanTopFindingNext(findings []scan.NormalizedFinding) (mapsvc.AnswerNext, bool) {
	for _, severity := range []string{"critical", "warning"} {
		for _, f := range findings {
			if f.Severity != severity {
				continue
			}
			kind, name, ok := strings.Cut(f.Resource, "/")
			if !ok || !traceSafeKind.MatchString(kind) || !traceSafeName.MatchString(name) || (f.Namespace != "" && !traceSafeName.MatchString(f.Namespace)) {
				continue
			}
			ns := ""
			if f.Namespace != "" {
				ns = " -n " + f.Namespace
			}
			return mapsvc.AnswerNext{Command: preferInvocationForm("cub-scout explain " + kind + "/" + name + ns + " --format json"), Why: "who owns the top finding's resource, and who last changed it"}, true
		}
	}
	return mapsvc.AnswerNext{}, false
}

// scanWarningSubject names which scanner a coverage warning is about.
func scanWarningSubject(warning string) string {
	switch {
	case strings.HasPrefix(warning, "Kyverno scan"):
		return "Kyverno's policy reports"
	case strings.HasPrefix(warning, "state scan"):
		return "part of the state scan"
	}
	return "a scanner"
}
