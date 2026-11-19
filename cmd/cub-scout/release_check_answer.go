package main

// The answer block on `release check --format json`: is the expected release
// what the controller reports and the cluster carries? The state words were
// decided by the maintainer on 2026-11-19. They restate the report's own
// weakest-link verdict and add no observation: PASS is matches, BLOCK is
// differs, WATCH is not-compared (not yet), INCONCLUSIVE is not-observable.

import (
	"fmt"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

const (
	releaseStateMatches       = "matches"
	releaseStateDiffers       = "differs"
	releaseStateNotCompared   = "not-compared"
	releaseStateNotObservable = "not-observable"

	releaseReasonMatches              = "bundle-reported-and-matches"
	releaseReasonConfigurationDiffers = "configuration-differs"
	releaseReasonWorkloadFailure      = "workload-reports-failure"
	releaseReasonRunningImageDiffers  = "running-image-differs"
	releaseReasonOtherRevision        = "controller-reports-other-revision"
	releaseReasonWorkloadsConverging  = "workloads-converging"
	releaseReasonStageNotSettled      = "stage-not-settled"
	releaseReasonEvidenceIncomplete   = "evidence-incomplete"

	// releaseCostCoverage is a key, spelled out in json-contracts.md: the
	// bounded Kubernetes reads the report counts in requestCounts. Registry
	// requests are in bundle, and response bytes are not counted.
	releaseCostCoverage = "release-check-bounded-reads"
)

// releaseStates and releaseReasons are what the conformance test checks
// answers against. Adding a value means documenting it in json-contracts.md.
var (
	releaseStates  = []string{releaseStateMatches, releaseStateDiffers, releaseStateNotCompared, releaseStateNotObservable}
	releaseReasons = []string{releaseReasonMatches, releaseReasonConfigurationDiffers, releaseReasonWorkloadFailure, releaseReasonRunningImageDiffers, releaseReasonOtherRevision, releaseReasonWorkloadsConverging, releaseReasonStageNotSettled, releaseReasonEvidenceIncomplete}
)

// releaseCheckOutput carries the answer beside the report, whose fields stay
// where they were.
type releaseCheckOutput struct {
	agent.ReleaseCheckReport
	Answer *mapsvc.Answer `json:"answer,omitempty"`
}

// releaseStageRestsOn says whose word each stage is: the controller's own
// report for the revision it applied and the workloads' status, cub-scout's
// comparison for digests and authored fields.
func releaseStageRestsOn(stage string) string {
	switch stage {
	case "controller", "workloads":
		return mapsvc.RestsOnControllerReport
	}
	return mapsvc.RestsOnCubScoutComparison
}

func releaseStage(r agent.ReleaseCheckReport, name string) (agent.ReleaseCheckStage, bool) {
	for _, stage := range r.Stages {
		if stage.Name == name {
			return stage, true
		}
	}
	return agent.ReleaseCheckStage{}, false
}

// buildReleaseCheckAnswer puts a finished release report into the common
// envelope.
func buildReleaseCheckAnswer(r agent.ReleaseCheckReport) *mapsvc.Answer {
	observedAt := r.FinishedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	answer := &mapsvc.Answer{
		Cluster:    mapsvc.AnswerCluster{Context: r.Context, IDSource: "not-read"},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
		Cost: mapsvc.AnswerCost{
			RequestsMade: int64(r.RequestCounts.Discovery + r.RequestCounts.Object),
			Coverage:     releaseCostCoverage,
		},
	}
	if !r.StartedAt.IsZero() && !r.FinishedAt.IsZero() {
		answer.Cost.DurationMillis = r.FinishedAt.Sub(r.StartedAt).Milliseconds()
	}

	// Each stage is a claim by its own word; its reason is quoted, since it
	// can carry a revision string the controller reported.
	for _, stage := range r.Stages {
		switch stage.Verdict {
		case "PASS", "BLOCK", "WATCH":
			answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
				Claim: fmt.Sprintf("the %s stage is %s", stage.Name, stage.Verdict), RestsOn: releaseStageRestsOn(stage.Name), Quoted: stage.Reason,
			})
		case "INCONCLUSIVE":
			answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
				What: "the " + stage.Name + " stage", Kind: mapsvc.UnknownNotObservable, Reason: "its evidence is missing or changed during the check; the stage's reason is quoted", Quoted: stage.Reason,
			})
		case "NOT_ASSESSED":
			answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
				What: "the " + stage.Name + " stage", Kind: mapsvc.UnknownNotRequested, Reason: "not assessed for this bundle; the stage's reason is quoted", Quoted: stage.Reason,
			})
		}
	}
	// What the report never claims stays an unknown.
	for _, omission := range r.Omissions {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What: omission.Missing, Kind: mapsvc.UnknownNotCompared, Reason: omission.Reason,
		})
	}

	blocked := func(name string) bool { s, ok := releaseStage(r, name); return ok && s.Verdict == "BLOCK" }
	watching := func(name string) bool { s, ok := releaseStage(r, name); return ok && s.Verdict == "WATCH" }
	switch r.Verdict {
	case agent.VerdictPASS:
		answer.State, answer.Reason = releaseStateMatches, releaseReasonMatches
		answer.Summary = "The controller reports the expected bundle, and the live authored configuration matches it."
		if r.Convergence != nil {
			answer.Summary += " Workload controllers report convergence."
		}
	case agent.VerdictBLOCK:
		answer.State = releaseStateDiffers
		switch {
		case blocked("configuration"):
			answer.Reason = releaseReasonConfigurationDiffers
			answer.Summary = "The live authored configuration differs from the expected bundle."
		case blocked("workloads"):
			answer.Reason = releaseReasonWorkloadFailure
			answer.Summary = "A workload controller reports failure for the expected release."
		default:
			answer.Reason = releaseReasonRunningImageDiffers
			answer.Summary = "A running container is not at the intended image digest, or not running and ready."
		}
	case agent.VerdictWATCH:
		answer.State = releaseStateNotCompared
		switch {
		case watching("controller"):
			answer.Reason = releaseReasonOtherRevision
			answer.Summary = "The controller reports a different bundle revision, so the expected release is not yet applied."
		case watching("workloads"):
			answer.Reason = releaseReasonWorkloadsConverging
			answer.Summary = "The expected release is applied, and its workloads are not yet reported converged."
		default:
			answer.Reason = releaseReasonStageNotSettled
			answer.Summary = "A stage of the check is not yet settled; its reason is quoted in basis."
		}
	default:
		answer.State, answer.Reason = releaseStateNotObservable, releaseReasonEvidenceIncomplete
		answer.Summary = "Evidence the check needs is missing or changed during it, so whether the release matches is not known."
	}
	return answer
}
