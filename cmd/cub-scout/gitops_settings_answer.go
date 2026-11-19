package main

// The answer block on `gitops settings --format json`: what delivery
// settings do the deployers here declare? The state words were decided by the
// maintainer on 2026-11-19. Settings are declared configuration: nothing in
// the answer says a controller acted on one.

import (
	"fmt"
	"strings"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
)

const (
	settingsStateListed        = "listed"
	settingsStateNoDeployers   = "no-deployers"
	settingsStatePartial       = "partial"
	settingsStateNotObservable = "not-observable"

	settingsReasonSettingsRead     = "settings-read"
	settingsReasonKindsNotRead     = "kinds-not-read"
	settingsReasonNoDeployersFound = "no-deployers-found"
	settingsReasonNoneMatch        = "no-deployers-match-filters"

	// settingsCostCoverage is a key, spelled out in json-contracts.md: the
	// Kubernetes requests of the settings read's own client.
	settingsCostCoverage = "gitops-settings-session-clients"
)

// settingsStates and settingsReasons are what the conformance test checks
// answers against. Adding a value means documenting it in json-contracts.md.
var (
	settingsStates  = []string{settingsStateListed, settingsStateNoDeployers, settingsStatePartial, settingsStateNotObservable}
	settingsReasons = []string{settingsReasonSettingsRead, settingsReasonKindsNotRead, settingsReasonNoDeployersFound, settingsReasonNoneMatch}
)

// buildDeliverySettingsAnswer puts a settings report into the common
// envelope. It adds no observation.
func buildDeliverySettingsAnswer(report deliverySettingsReport, cost *agent.KubernetesReadCost, observedAt time.Time) *mapsvc.Answer {
	answer := &mapsvc.Answer{
		Cluster:    mapsvc.AnswerCluster{Context: report.Context, IDSource: "not-read"},
		ObservedAt: observedAt.UTC().Format(time.RFC3339),
		Basis:      []mapsvc.AnswerBasis{},
		Unknowns:   []mapsvc.AnswerUnknown{},
		Next:       []mapsvc.AnswerNext{},
		Cost:       mapsvc.AnswerCost{Coverage: "not-measured"},
	}
	if cost != nil {
		answer.Cost = mapsvc.AnswerCostFrom(*cost)
	}

	var notRead, notInstalled []string
	for _, count := range report.Counts {
		label := count.Controller + " " + count.Kind
		if !traceSafeKind.MatchString(count.Controller) || !traceSafeKind.MatchString(count.Kind) {
			label = "a deployer kind"
		}
		switch count.Status {
		case agent.DeliveryReadRead:
			if count.Shown > 0 {
				answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
					Claim:   fmt.Sprintf("%d %s object(s) were read and their spec settings listed", count.Shown, label),
					RestsOn: mapsvc.RestsOnObjectMetadata,
				})
			}
		case agent.DeliveryReadNotInstalled:
			notInstalled = append(notInstalled, label)
		case agent.DeliveryReadNotRead:
			notRead = append(notRead, label)
		}
	}
	if len(notInstalled) > 0 {
		answer.Basis = append(answer.Basis, mapsvc.AnswerBasis{
			Claim:   "the API server serves no " + strings.Join(notInstalled, ", no ") + " kind",
			RestsOn: mapsvc.RestsOnObjectMetadata,
		})
	}
	for _, read := range report.Reads {
		if read.Status != agent.DeliveryReadNotRead {
			continue
		}
		label := read.Controller + " " + read.Kind
		if !traceSafeKind.MatchString(read.Controller) || !traceSafeKind.MatchString(read.Kind) {
			label = "a deployer kind"
		}
		unknown := mapsvc.AnswerUnknown{What: "every " + label + " and its settings", Kind: mapsvc.UnknownNotObservable, Reason: "the list failed; its message is quoted", Quoted: read.Message}
		if read.Reason == "forbidden" || read.Reason == "unauthorized" {
			unknown.Kind = mapsvc.UnknownDenied
		}
		answer.Unknowns = append(answer.Unknowns, unknown)
	}

	scope := "this cluster"
	if report.Namespace != "" {
		scope = "namespace " + report.Namespace
		if !traceSafeName.MatchString(report.Namespace) {
			scope = "this namespace"
		}
	}
	shown := len(report.Deployers)
	switch {
	case shown == 0 && len(notRead) > 0:
		answer.State, answer.Reason = settingsStateNotObservable, settingsReasonKindsNotRead
		answer.Summary = fmt.Sprintf("No deployer settings were read in %s, and %d deployer kind(s) could not be listed, so whether there are any is unknown.", scope, len(notRead))
	case shown == 0:
		answer.State = settingsStateNoDeployers
		if report.Filters != nil {
			answer.Reason = settingsReasonNoneMatch
			answer.Summary = fmt.Sprintf("No deployer in %s matches the filters.", scope)
		} else {
			answer.Reason = settingsReasonNoDeployersFound
			answer.Summary = fmt.Sprintf("No Argo CD Application or Flux Kustomization or HelmRelease was found in %s.", scope)
		}
	case len(notRead) > 0:
		answer.State, answer.Reason = settingsStatePartial, settingsReasonKindsNotRead
		answer.Summary = fmt.Sprintf("The settings of %d deployer(s) in %s are listed, but %d deployer kind(s) could not be listed, so some may be missing.", shown, scope, len(notRead))
	default:
		answer.State, answer.Reason = settingsStateListed, settingsReasonSettingsRead
		answer.Summary = fmt.Sprintf("The settings of %d deployer(s) in %s are listed, as each one's spec declares them.", shown, scope)
	}
	if shown > 0 {
		answer.Unknowns = append(answer.Unknowns, mapsvc.AnswerUnknown{
			What: "whether each controller acted on these settings", Kind: mapsvc.UnknownNotCompared,
			Reason: "settings are declared configuration, read from each spec",
		})
		ns := ""
		if report.Namespace != "" && traceSafeName.MatchString(report.Namespace) {
			ns = " -n " + report.Namespace
		}
		answer.Next = append(answer.Next, mapsvc.AnswerNext{Command: preferInvocationForm("cub-scout gitops status" + ns + " --format json"), Why: "whether these deployers' controllers report them ready"})
	}
	return answer
}
