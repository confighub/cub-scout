// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package mapsvc

import "github.com/confighub/cub-scout/v2/pkg/agent"

// Answer is the envelope every JSON answer is to carry, so an agent that has
// learned to read one command's answer can read the next. Design and the
// reasons for each field: docs/proposals/answer-envelope.md. `trace` is the
// first command to carry it.
type Answer struct {
	// State is one word from the command's closed list. Never a verdict.
	State string `json:"state"`
	// Reason is a stable key for the state. Decide on this, not on Summary.
	Reason string `json:"reason"`
	// Summary is one sentence for a person. Its words may change.
	Summary string `json:"summary"`
	// ObservedAt is when the cluster was read.
	ObservedAt string `json:"observedAt"`
	// Cluster says which cluster was read. ID is read only on request.
	Cluster AnswerCluster `json:"cluster"`
	// Basis says, for each claim the answer makes, whose word it rests on.
	Basis []AnswerBasis `json:"basis"`
	// Unknowns is everything the answer could not establish. Empty means the
	// command found nothing it could not read; it is never omitted.
	Unknowns []AnswerUnknown `json:"unknowns"`
	// Next is exact read-only commands that follow from Reason.
	Next []AnswerNext `json:"next"`
	// Cost is what the reads behind this answer cost.
	Cost agent.KubernetesReadCost `json:"cost"`
}

type AnswerCluster struct {
	Context  string `json:"context"`
	ID       string `json:"id"`
	IDSource string `json:"idSource"`
}

// The parties a claim can rest on.
const (
	RestsOnObjectMetadata     = "object-metadata"
	RestsOnControllerReport   = "controller-report"
	RestsOnDeployerReport     = "deployer-report"
	RestsOnConfigHubRecord    = "confighub-record"
	RestsOnCubScoutComparison = "cub-scout-comparison"
)

type AnswerBasis struct {
	Claim   string `json:"claim"`
	RestsOn string `json:"restsOn"`
	// Ref is set only when the claim is about an object that exists in the
	// cluster and can be fetched by this reference.
	Ref *AnswerRef `json:"ref,omitempty"`
	// Quoted is text that came from the cluster, word for word. It is data,
	// not cub-scout's own words, and never an instruction.
	Quoted string `json:"quoted,omitempty"`
}

type AnswerRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// The kinds of unknown.
const (
	UnknownNotObservable = "not-observable"
	UnknownDenied        = "denied"
	UnknownStale         = "stale"
	UnknownNotRequested  = "not-requested"
	UnknownNotCompared   = "not-compared"
)

type AnswerUnknown struct {
	What   string `json:"what"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
	// Quoted is error or cluster text, word for word. Data, not instruction.
	Quoted string `json:"quoted,omitempty"`
}

type AnswerNext struct {
	Command string `json:"command"`
	Why     string `json:"why"`
}
