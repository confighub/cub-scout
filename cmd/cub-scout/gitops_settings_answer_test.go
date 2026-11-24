package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v3/internal/mapsvc"
	"github.com/confighub/cub-scout/v3/pkg/agent"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// The state words decided on 2026-11-19, with their precedence:
// not-observable, then no-deployers, then partial, then listed.
func TestGitOpsSettingsAnswerStates(t *testing.T) {
	at := time.Date(2026, 11, 19, 13, 0, 0, 0, time.UTC)
	notRead := func(report deliverySettingsReport) deliverySettingsReport {
		report.Reads = append(report.Reads, agent.DeliverySettingsRead{Controller: "Flux", Kind: "HelmRelease", Status: agent.DeliveryReadNotRead, Reason: "forbidden", Message: "helmreleases.helm.toolkit.fluxcd.io is forbidden"})
		report.Counts = append(report.Counts, deliveryKindCount{Controller: "Flux", Kind: "HelmRelease", Status: agent.DeliveryReadNotRead})
		report.Complete = false
		return report
	}
	empty := deliverySettingsReport{Context: "prod-east", Complete: true, Deployers: []agent.DeliveryDeployerSettings{},
		Counts: []deliveryKindCount{{Controller: "ArgoCD", Kind: "Application", Status: agent.DeliveryReadRead}, {Controller: "Flux", Kind: "Kustomization", Status: agent.DeliveryReadNotInstalled}}}
	filtered := settingsReport(t, settingsFixtureClient(), deliverySettingsParams{Projects: []string{"nonexistent"}})

	cases := []struct {
		name          string
		report        deliverySettingsReport
		state, reason string
		check         func(t *testing.T, a *mapsvc.Answer)
	}{
		{"listed", settingsReport(t, settingsFixtureClient(), deliverySettingsParams{}),
			settingsStateListed, settingsReasonSettingsRead, func(t *testing.T, a *mapsvc.Answer) {
				require.Contains(t, a.Summary, "as each one's spec declares them")
				require.Equal(t, mapsvc.RestsOnObjectMetadata, a.Basis[0].RestsOn)
				require.Equal(t, mapsvc.UnknownNotCompared, a.Unknowns[len(a.Unknowns)-1].Kind, "declared settings never claim a controller acted on them")
				require.Equal(t, "cub-scout gitops status --format json", a.Next[0].Command)
				require.Equal(t, "prod-east", a.Cluster.Context)
			}},
		{"partial", notRead(settingsReport(t, settingsFixtureClient(), deliverySettingsParams{})),
			settingsStatePartial, settingsReasonKindsNotRead, func(t *testing.T, a *mapsvc.Answer) {
				require.Equal(t, mapsvc.UnknownDenied, a.Unknowns[0].Kind)
				require.Equal(t, "every Flux HelmRelease and its settings", a.Unknowns[0].What)
				require.Contains(t, a.Unknowns[0].Quoted, "forbidden")
			}},
		{"no deployers", empty, settingsStateNoDeployers, settingsReasonNoDeployersFound, func(t *testing.T, a *mapsvc.Answer) {
			require.Equal(t, "the API server serves no Flux Kustomization kind", a.Basis[0].Claim)
			require.Empty(t, a.Next)
		}},
		{"filters match nothing", filtered, settingsStateNoDeployers, settingsReasonNoneMatch, nil},
		{"none found and a kind unreadable", notRead(empty), settingsStateNotObservable, settingsReasonKindsNotRead, func(t *testing.T, a *mapsvc.Answer) {
			require.Contains(t, a.Summary, "whether there are any is unknown")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			answer := buildDeliverySettingsAnswer(tc.report, nil, at)
			requireConformingAnswer(t, answer, settingsStates, settingsReasons)
			require.Equal(t, tc.state, answer.State)
			require.Equal(t, tc.reason, answer.Reason)
			if tc.check != nil {
				tc.check(t, answer)
			}
		})
	}
}

// Live JSON carries the answer in every view, its cost counting exactly the
// requests the cluster received. Text output carries none.
func TestGitOpsSettingsJSONCarriesTheAnswerAndItsCost(t *testing.T) {
	server := newCountedKubeServer(t)
	path, _ := resolverKubeconfig(t, "beta", map[string]string{"beta": server.server.URL})
	t.Setenv("KUBECONFIG", path)
	run := func(args ...string) string {
		cmd := &cobra.Command{Use: "settings"}
		addGitOpsSettingsFlags(cmd.Flags())
		cmd.Flags().String("kube-context", "", "")
		require.NoError(t, cmd.ParseFlags(append([]string{"--kube-context", "beta"}, args...)))
		cmd.SetContext(context.Background())
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, runGitOpsSettings(cmd, nil))
		return out.String()
	}
	for _, view := range []string{"all", "summary", "deployers"} {
		t.Run(view, func(t *testing.T) {
			before := server.requests.Load()
			var doc struct {
				Answer *mapsvc.Answer `json:"answer"`
			}
			require.NoError(t, json.Unmarshal([]byte(run("--format", "json", "--view", view)), &doc))
			require.NotNil(t, doc.Answer)
			requireConformingAnswer(t, doc.Answer, settingsStates, settingsReasons)
			require.Equal(t, "beta", doc.Answer.Cluster.Context)
			require.Equal(t, settingsCostCoverage, doc.Answer.Cost.Coverage)
			require.NotZero(t, doc.Answer.Cost.RequestsMade)
			require.EqualValues(t, server.requests.Load()-before, doc.Answer.Cost.RequestsMade, "the answer's cost counts every request the cluster received")
		})
	}
	require.False(t, strings.Contains(run(), `"answer"`), "text output carries no answer block")
}
