package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestMapListAnswerStates(t *testing.T) {
	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	entries := []MapEntry{{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux"}, {Kind: "ConfigMap", Name: "x", Namespace: "prod", Owner: "Native"}}
	forbidden := []mapsvc.CollectionOmission{{APIVersion: "argoproj.io/v1alpha1", Resource: "applications", Reason: "applications.argoproj.io is forbidden"}}
	cases := []struct {
		name          string
		entries       []MapEntry
		omissions     []mapsvc.CollectionOmission
		unavailable   string
		state, reason string
		check         func(t *testing.T, a *mapsvc.Answer)
	}{
		{"all listed", entries, nil, "", mapStateListed, mapReasonInventoryListed, func(t *testing.T, a *mapsvc.Answer) {
			require.Equal(t, "2 resource(s) listed; owner by their own markers: Flux 1, Native 1", a.Basis[0].Claim)
			require.Contains(t, a.Unknowns[0].What, "no recognised owner marker", "no marker is not shown to be unmanaged")
		}},
		{"a kind forbidden", entries, forbidden, "", mapStatePartial, mapReasonKindsNotListed, func(t *testing.T, a *mapsvc.Answer) {
			last := a.Unknowns[len(a.Unknowns)-1]
			require.Equal(t, mapsvc.UnknownDenied, last.Kind)
			require.Equal(t, "applications (argoproj.io/v1alpha1)", last.What)
		}},
		{"nothing listed, a kind failed", nil, forbidden, "", mapStateNotObservable, mapReasonInventoryNotRead, nil},
		{"inventory unavailable", nil, nil, "connection refused", mapStateNotObservable, mapReasonInventoryNotRead, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			answer := buildMapListAnswer(tc.entries, tc.omissions, tc.unavailable, nil, at)
			requireConformingAnswer(t, answer, mapStates, mapReasons)
			require.Equal(t, tc.state, answer.State)
			require.Equal(t, tc.reason, answer.Reason)
			if tc.check != nil {
				tc.check(t, answer)
			}
		})
	}
}

// The cluster's own identity, when it was read, is the answer's.
func TestMapListAnswerCarriesTheReadClusterIdentity(t *testing.T) {
	observed := time.Date(2026, 11, 19, 12, 30, 0, 0, time.UTC)
	identity := &agent.ClusterIdentityEvidence{Context: "prod-east", ID: "kube-system-uid", IDSource: "kube-system-namespace-uid", ObservedAt: &observed}
	answer := buildMapListAnswer(nil, nil, "", identity, time.Now())
	require.Equal(t, mapsvc.AnswerCluster{Context: "prod-east", ID: "kube-system-uid", IDSource: "kube-system-namespace-uid"}, answer.Cluster)
	require.Equal(t, "2026-11-19T12:30:00Z", answer.ObservedAt)
}

// The object forms carry the answer; the default bare array does not, since
// adding one would change its type for every consumer.
func TestMapListJSONFormsAndTheAnswer(t *testing.T) {
	entries := []MapEntry{{Kind: "Deployment", Name: "api", Namespace: "prod", Owner: "Flux"}}
	oldFormat, oldSummary := mapListFormat, mapSummary
	t.Cleanup(func() { mapListFormat, mapSummary = oldFormat, oldSummary })
	mapListFormat = "json"

	mapSummary = false
	out := captureStdout(t, func() { require.NoError(t, renderMapListFromEntriesWithIdentity(entries, nil, false, nil, "")) })
	var array []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &array), "the default output stays a JSON array")

	for name, run := range map[string]func() error{
		"summary": func() error {
			mapSummary = true
			return renderMapListFromEntriesWithIdentity(entries, nil, false, nil, "")
		},
	} {
		out := captureStdout(t, func() { require.NoError(t, run()) })
		var doc struct {
			Answer *mapsvc.Answer `json:"answer"`
		}
		require.NoError(t, json.Unmarshal([]byte(out), &doc), name)
		require.NotNil(t, doc.Answer, name)
		requireConformingAnswer(t, doc.Answer, mapStates, mapReasons)
		require.Equal(t, mapStateListed, doc.Answer.State, name)
	}
}
