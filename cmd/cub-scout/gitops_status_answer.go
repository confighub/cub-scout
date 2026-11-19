package main

// The answer block on `gitops status --format json`: is delivery settled
// right now? The state words were decided by the maintainer on 2026-11-19.
// The answer adds no observation: it is drawn from the summary beside it,
// and claims less than the summary where the summary does not support more.

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

const (
	gitopsStateSettled       = "settled"
	gitopsStateConverging    = "converging"
	gitopsStateFailing       = "failing"
	gitopsStateNoDeployers   = "no-deployers"
	gitopsStateNotObservable = "not-observable"

	gitopsReasonDeployersReady      = "deployers-report-ready"
	gitopsReasonReconciling         = "deployers-reconciling"
	gitopsReasonDeployerFailure     = "deployer-reports-failure"
	gitopsReasonSourceFailure       = "source-reports-failure"
	gitopsReasonNoDeployers         = "no-deployers-found"
	gitopsReasonDeployersNotRead    = "deployers-not-read"
	gitopsReasonControllersNotRead  = "controllers-not-readable"
	gitopsCostCoverage              = "gitops-status-session-clients"
	gitopsAnswerBasisLimit          = 10
	gitopsAnswerUnknownExampleLimit = 5
)

// gitopsStates and gitopsReasons are what the conformance test checks
// answers against. Adding a value means documenting it in json-contracts.md.
var (
	gitopsStates  = []string{gitopsStateSettled, gitopsStateConverging, gitopsStateFailing, gitopsStateNoDeployers, gitopsStateNotObservable}
	gitopsReasons = []string{gitopsReasonDeployersReady, gitopsReasonReconciling, gitopsReasonDeployerFailure, gitopsReasonSourceFailure, gitopsReasonNoDeployers, gitopsReasonDeployersNotRead, gitopsReasonControllersNotRead}
)

// gitopsFluxInProgressReasons are the Flux Ready reasons that say the
// controller is still working, not that it failed.
var gitopsFluxInProgressReasons = map[string]bool{"Progressing": true, "DependencyNotReady": true}

// gitopsDeployerConverging reports whether a deployer that is not ready is
// still being worked on, by its controller's own word: Flux's Ready reason,
// or Argo CD's health status.
func gitopsDeployerConverging(d DeployerStatus) bool {
	if d.Ready || d.notRead {
		return false
	}
	switch d.Kind {
	case "Kustomization", "HelmRelease":
		return gitopsFluxInProgressReasons[d.Reason]
	case "Application":
		return strings.EqualFold(d.HealthStatus, "Progressing")
	}
	return false
}

func gitopsObjectLabel(kind, name string) (string, bool) {
	if traceSafeKind.MatchString(kind) && traceSafeName.MatchString(name) {
		return kind + "/" + name, true
	}
	return "a " + kind, false
}

func gitopsQuoted(named bool, kind, name, reason, message string) string {
	var parts []string
	if !named {
		parts = append(parts, "object: "+strings.TrimSpace(kind+"/"+name))
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		parts = append(parts, "reason: "+reason)
	}
	if message = strings.TrimSpace(message); message != "" {
		parts = append(parts, "message: "+message)
	}
	return strings.Join(parts, "; ")
}

func gitopsRef(kind, name, namespace string, named bool) *mapsvc.AnswerRef {
	if !named || (namespace != "" && !traceSafeName.MatchString(namespace)) {
		return nil
	}
	return &mapsvc.AnswerRef{Kind: kind, Namespace: namespace, Name: name}
}

// buildGitOpsStatusAnswer puts a gitops status summary into the common
// envelope. cost is nil when the reads were not metered (fixture input).
func buildGitOpsStatusAnswer(summary GitOpsSummary, observedAt time.Time, cost *agent.KubernetesReadCost) *mapsvc.Answer {
	answer := &mapsvc.Answer{
		Cluster:    mapsvc.AnswerCluster{Context: summary.Context, IDSource: "not-read"},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
	}
	if cost != nil {
		answer.Cost = mapsvc.AnswerCostFrom(*cost)
	} else {
		answer.Cost.Coverage = "not-measured"
	}

	var ready, suspended, failing, converging, notRead []DeployerStatus
	for _, d := range summary.Deployers {
		switch {
		case d.notRead:
			notRead = append(notRead, d)
		case d.Suspended:
			suspended = append(suspended, d)
		case d.IsHealthy():
			ready = append(ready, d)
		case gitopsDeployerConverging(d):
			converging = append(converging, d)
		default:
			failing = append(failing, d)
		}
	}
	var failingSources, unreadSources []SourceStatus
	for _, s := range summary.Sources {
		switch {
		case s.notRead:
			unreadSources = append(unreadSources, s)
		case !s.Ready:
			failingSources = append(failingSources, s)
		}
	}

	// Basis: the deployers and sources that decide the state, each in its
	// controller's word, up to a limit; a ready deployer is counted, not
	// listed.
	var items []mapsvc.AnswerBasis
	deployerBasis := func(d DeployerStatus, claim string, quote bool) mapsvc.AnswerBasis {
		label, named := gitopsObjectLabel(d.Kind, d.Name)
		basis := mapsvc.AnswerBasis{Claim: label + " " + claim, RestsOn: mapsvc.RestsOnControllerReport, Ref: gitopsRef(d.Kind, d.Name, d.Namespace, named)}
		reason := ""
		if quote {
			reason = d.Reason
			if d.Kind == "Application" && d.HealthStatus != "" {
				reason = strings.TrimSpace(d.SyncStatus + " / " + d.HealthStatus)
			}
			basis.Quoted = gitopsQuoted(named, d.Kind, d.Name, reason, d.Message)
		} else {
			basis.Quoted = gitopsQuoted(named, d.Kind, d.Name, "", "")
		}
		return basis
	}
	for _, d := range failing {
		items = append(items, deployerBasis(d, "is reported not ready by its controller", true))
	}
	for _, src := range failingSources {
		label, named := gitopsObjectLabel(src.Kind, src.Name)
		items = append(items, mapsvc.AnswerBasis{
			Claim: label + " is reported not ready by its controller", RestsOn: mapsvc.RestsOnControllerReport,
			Ref: gitopsRef(src.Kind, src.Name, src.Namespace, named), Quoted: gitopsQuoted(named, src.Kind, src.Name, src.Reason, src.Message),
		})
	}
	for _, d := range converging {
		items = append(items, deployerBasis(d, "is reported still reconciling by its controller", true))
	}
	for _, d := range suspended {
		items = append(items, deployerBasis(d, "is suspended: its controller is not reconciling it", false))
	}
	if len(items) > gitopsAnswerBasisLimit {
		more := len(items) - gitopsAnswerBasisLimit
		items = append(items[:gitopsAnswerBasisLimit], mapsvc.AnswerBasis{
			Claim: fmt.Sprintf("%d more not-ready or suspended deployer(s) and source(s) are listed in deployers and sources", more), RestsOn: mapsvc.RestsOnControllerReport,
		})
	}
	answer.Basis = append(answer.Basis, items...)
	if len(ready) > 0 {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim: fmt.Sprintf("%d deployer(s) are reported ready by their controllers", len(ready)), RestsOn: mapsvc.RestsOnControllerReport,
		})
	}

	// Unknowns: what could not be read, named.
	if len(notRead)+len(unreadSources) > 0 {
		var names []string
		for _, d := range notRead {
			if label, named := gitopsObjectLabel(d.Kind, d.Name); named {
				names = append(names, label)
			}
		}
		for _, s := range unreadSources {
			if label, named := gitopsObjectLabel(s.Kind, s.Name); named {
				names = append(names, label)
			}
		}
		sort.Strings(names)
		more := ""
		if len(names) > gitopsAnswerUnknownExampleLimit {
			more = fmt.Sprintf(" and %d more", len(names)-gitopsAnswerUnknownExampleLimit)
			names = names[:gitopsAnswerUnknownExampleLimit]
		}
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What:   fmt.Sprintf("the readiness of %d deployer(s) and source(s): %s%s", len(notRead)+len(unreadSources), strings.Join(names, ", "), more),
			Kind:   mapsvc.UnknownNotObservable,
			Reason: "their objects were found but could not be read",
		})
	}
	unreadableFamilies := false
	for _, family := range summary.ControllerCoverage {
		if family.Status != controllerCoverageUnreadable && family.Status != controllerCoveragePartial {
			continue
		}
		unreadableFamilies = true
		kind, reason := mapsvc.UnknownNotObservable, "its kinds could not be listed"
		for _, omission := range family.Omissions {
			if strings.Contains(strings.ToLower(omission.Reason), "forbidden") {
				kind, reason = mapsvc.UnknownDenied, "listing its kinds was forbidden"
			}
		}
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{What: family.Family + " deployers", Kind: kind, Reason: reason})
	}
	if len(summary.Deployers) > 0 {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What: "the workloads these deployers deliver", Kind: mapsvc.UnknownNotCompared,
			Reason: "gitops status reads what the delivery controllers report, not the workloads",
		})
	}

	explainFirst := func(d DeployerStatus, why string) {
		if label, named := gitopsObjectLabel(d.Kind, d.Name); named && (d.Namespace == "" || traceSafeName.MatchString(d.Namespace)) {
			scope := ""
			if d.Namespace != "" {
				scope = " -n " + d.Namespace
			}
			answer.Next = append(answer.Next, mapsvc.AnswerNext{Command: preferInvocationForm("cub-scout explain " + label + scope + " --format json"), Why: why})
		}
	}

	switch {
	case len(failing) > 0:
		answer.State, answer.Reason = gitopsStateFailing, gitopsReasonDeployerFailure
		label, _ := gitopsObjectLabel(failing[0].Kind, failing[0].Name)
		answer.Summary = fmt.Sprintf("%d of %d deployer(s) report a failure, first %s.", len(failing), len(summary.Deployers), label)
		explainFirst(failing[0], "what failed, its conditions and recent events")
	case len(failingSources) > 0:
		answer.State, answer.Reason = gitopsStateFailing, gitopsReasonSourceFailure
		label, _ := gitopsObjectLabel(failingSources[0].Kind, failingSources[0].Name)
		answer.Summary = fmt.Sprintf("%d source(s) report a failure, first %s.", len(failingSources), label)
	case len(converging) > 0:
		answer.State, answer.Reason = gitopsStateConverging, gitopsReasonReconciling
		answer.Summary = fmt.Sprintf("%d of %d deployer(s) are still reconciling; none reports a failure.", len(converging), len(summary.Deployers))
		answer.Next = append(answer.Next, mapsvc.AnswerNext{Command: preferInvocationForm("cub-scout gitops status --format json"), Why: "read again once the controllers have reconciled"})
	case len(notRead)+len(unreadSources) > 0:
		answer.State, answer.Reason = gitopsStateNotObservable, gitopsReasonDeployersNotRead
		answer.Summary = fmt.Sprintf("%d deployer(s) and source(s) could not be read; the rest report no failure.", len(notRead)+len(unreadSources))
	case len(summary.Deployers) == 0 && unreadableFamilies:
		answer.State, answer.Reason = gitopsStateNotObservable, gitopsReasonControllersNotRead
		answer.Summary = "No deployers were found, and some controller kinds could not be listed, so whether there are any is not known."
	case len(summary.Deployers) == 0:
		answer.State, answer.Reason = gitopsStateNoDeployers, gitopsReasonNoDeployers
		answer.Summary = "No GitOps deployers were found."
	default:
		answer.State, answer.Reason = gitopsStateSettled, gitopsReasonDeployersReady
		switch {
		case len(ready) == 0:
			answer.Summary = fmt.Sprintf("All %d deployer(s) are suspended; none reports a failure.", len(suspended))
		case len(suspended) > 0:
			answer.Summary = fmt.Sprintf("%d deployer(s) report ready; %d more are suspended.", len(ready), len(suspended))
		default:
			answer.Summary = fmt.Sprintf("All %d deployer(s) report ready.", len(ready))
		}
	}
	if unreadableFamilies && answer.State == gitopsStateSettled {
		answer.Summary += " Some controller kinds could not be listed; see unknowns."
	}
	return answer
}

// gitopsFailingCount is how many deployers the answer counts as failing:
// read, not suspended, not ready and not reconciling.
func gitopsFailingCount(summary GitOpsSummary) int {
	n := 0
	for _, d := range summary.Deployers {
		if !d.notRead && !d.Suspended && !d.IsHealthy() && !gitopsDeployerConverging(d) {
			n++
		}
	}
	return n
}
