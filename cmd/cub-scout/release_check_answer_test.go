package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/internal/mapsvc"
	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
)

// The state words decided on 2026-11-19 restate the report's own verdict:
// PASS matches, BLOCK differs, WATCH not-compared, INCONCLUSIVE
// not-observable.
func TestReleaseCheckAnswerStates(t *testing.T) {
	start := time.Date(2026, 11, 19, 14, 0, 0, 0, time.UTC)
	report := func(stages map[string]string) agent.ReleaseCheckReport {
		r := agent.NewReleaseCheckReport(agent.BoundedResourceRef{Kind: "Kustomization", Name: "apps", Namespace: "flux-system"}, "prod-east", "prod-east", 10, start)
		for i, stage := range r.Stages {
			if verdict, ok := stages[stage.Name]; ok {
				r.Stages[i].Verdict = verdict
				r.Stages[i].Reason = "fixture " + verdict
			}
		}
		r.RequestCounts = agent.BoundedReadCounts{Discovery: 2, Object: 7}
		r.Finish(start.Add(1500 * time.Millisecond))
		return r
	}
	pass := map[string]string{"bundle": "PASS", "controller": "PASS", "configuration": "PASS"}
	with := func(name, verdict string) map[string]string {
		out := map[string]string{}
		for k, v := range pass {
			out[k] = v
		}
		out[name] = verdict
		return out
	}
	cases := []struct {
		name          string
		r             agent.ReleaseCheckReport
		state, reason string
		check         func(t *testing.T, a *mapsvc.Answer)
	}{
		{"matches", report(pass), releaseStateMatches, releaseReasonMatches, func(t *testing.T, a *mapsvc.Answer) {
			require.Equal(t, "the controller stage is PASS", a.Basis[1].Claim)
			require.Equal(t, mapsvc.RestsOnControllerReport, a.Basis[1].RestsOn)
			require.Equal(t, mapsvc.RestsOnCubScoutComparison, a.Basis[2].RestsOn)
			require.Equal(t, mapsvc.UnknownNotRequested, a.Unknowns[0].Kind, "workloads were not assessed")
			var omitted []string
			for _, u := range a.Unknowns {
				if u.Kind == mapsvc.UnknownNotCompared {
					omitted = append(omitted, u.What)
				}
			}
			require.Contains(t, omitted, "application-success", "matches never claims the application works")
			require.EqualValues(t, 9, a.Cost.RequestsMade)
			require.EqualValues(t, 1500, a.Cost.DurationMillis)
			require.Equal(t, "2026-11-19T14:00:01Z", a.ObservedAt)
			require.Equal(t, "prod-east", a.Cluster.Context)
		}},
		{"configuration differs", report(with("configuration", "BLOCK")), releaseStateDiffers, releaseReasonConfigurationDiffers, nil},
		{"workload fails", report(with("workloads", "BLOCK")), releaseStateDiffers, releaseReasonWorkloadFailure, nil},
		{"other revision", report(with("controller", "WATCH")), releaseStateNotCompared, releaseReasonOtherRevision, func(t *testing.T, a *mapsvc.Answer) {
			require.Equal(t, "fixture WATCH", a.Basis[1].Quoted, "the stage reason, which can carry a reported revision, is quoted")
		}},
		{"workloads converging", report(with("workloads", "WATCH")), releaseStateNotCompared, releaseReasonWorkloadsConverging, nil},
		{"another stage watching", report(with("configuration", "WATCH")), releaseStateNotCompared, releaseReasonStageNotSettled, nil},
		{"evidence missing", report(with("controller", "INCONCLUSIVE")), releaseStateNotObservable, releaseReasonEvidenceIncomplete, func(t *testing.T, a *mapsvc.Answer) {
			require.Equal(t, "the controller stage", a.Unknowns[0].What)
		}},
		{"differs beats incomplete", report(map[string]string{"bundle": "PASS", "controller": "INCONCLUSIVE", "configuration": "BLOCK"}), releaseStateDiffers, releaseReasonConfigurationDiffers, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			answer := buildReleaseCheckAnswer(tc.r)
			requireConformingAnswer(t, answer, releaseStates, releaseReasons)
			require.Equal(t, tc.state, answer.State)
			require.Equal(t, tc.reason, answer.Reason)
			if tc.check != nil {
				tc.check(t, answer)
			}
		})
	}
}

// The JSON report carries the answer beside its own fields, which stay where
// they were, and the answer's cost is the report's own request count.
func TestReleaseCheckJSONCarriesTheAnswer(t *testing.T) {
	f := newReleaseFixture(t, "flux")
	args, err := releaseCheckMCPTool().BuildArgs(map[string]interface{}{
		"bundle": f.options.Bundle, "oci_layout": f.options.Layout,
		"controller": f.options.Controller, "api_version": f.options.APIVersion,
		"controller_namespace": "delivery", "context": "cluster-a",
	})
	require.NoError(t, err)
	cmd := newReleaseCheckCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(args[2:])
	require.NoError(t, cmd.Execute())
	var doc struct {
		agent.ReleaseCheckReport
		Answer *mapsvc.Answer `json:"answer"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	require.Equal(t, agent.VerdictPASS, doc.Verdict, "the report's own fields are unchanged")
	require.NotNil(t, doc.Answer)
	requireConformingAnswer(t, doc.Answer, releaseStates, releaseReasons)
	require.Equal(t, releaseStateMatches, doc.Answer.State)
	require.EqualValues(t, doc.RequestCounts.Discovery+doc.RequestCounts.Object, doc.Answer.Cost.RequestsMade)
	require.NotZero(t, doc.Answer.Cost.RequestsMade)
}
