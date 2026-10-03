// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestThreeWayTUIRefreshErrorClearsRenderedEvidence(t *testing.T) {
	session := &traceSession{context: "alpha"}
	pane := newThreeWayTUIModel(context.Background(), session, threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/old"}, threeWayOptions{})
	pane.start()
	report := threeWayReport{Scope: "deploy/old", Resources: []threeWayResourceEntry{{Result: compareResourceResult{Resource: "Deployment/old", Live: compareSideSummary{Images: []string{"OLD-EVIDENCE"}}}}}}
	updated, _ := pane.Update(threeWayTUIResultMsg{requestID: pane.requestID, scope: pane.scope, contextLabel: "alpha", report: report})
	pane = updated.(threeWayTUIModel)
	require.Contains(t, pane.View(), "OLD-EVIDENCE")
	updated, _ = pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	pane = updated.(threeWayTUIModel)
	pane.input = "namespace/new"
	updated, command := pane.Update(tea.KeyMsg{Type: tea.KeyEnter})
	pane = updated.(threeWayTUIModel)
	require.NotNil(t, command)
	require.NotContains(t, pane.viewport.View(), "OLD-EVIDENCE")
	updated, _ = pane.Update(threeWayTUIResultMsg{requestID: pane.requestID, scope: pane.scope, contextLabel: "alpha", err: errors.New("new scope forbidden")})
	pane = updated.(threeWayTUIModel)
	view := pane.View()
	require.Contains(t, view, "namespace/new")
	require.Contains(t, view, "new scope forbidden")
	require.NotContains(t, view, "OLD-EVIDENCE")
	require.NotContains(t, view, "Deployment/old")
	require.Empty(t, pane.output)
	if pane.cancel != nil {
		pane.cancel()
	}
}

func TestThreeWayTUIDynamicTextStripsTerminalControls(t *testing.T) {
	controls := "\x1b]52;c;encoded\x07\x1b[2J\x00\x7f\u009b"
	pane := newThreeWayTUIModel(context.Background(), &traceSession{context: "context" + controls}, threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "scope" + controls}, threeWayOptions{})
	pane.editing = true
	pane.input = "input" + controls
	pane.err = errors.New("error" + controls)
	view := pane.View()
	for _, label := range []string{"scope", "context", "input", "error"} {
		require.Contains(t, view, label)
	}
	for _, r := range view {
		require.False(t, unicode.IsControl(r) && r != '\n' && r != '\t', "terminal control U+%04X survived", r)
	}
}

func TestThreeWayViewEligibilityRefusesBeforeReads(t *testing.T) {
	for _, tc := range []struct {
		name      string
		local     bool
		dry       []*compareSideSummary
		connected bool
	}{
		{name: "offline"}, {name: "declared local dry", local: true, connected: true}, {name: "loaded local dry", dry: []*compareSideSummary{}, connected: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			answer := error(nil)
			if !tc.connected {
				answer = errors.New("recorded offline auth")
			}
			calls := answerGateWith(t, &answer)
			viewReads := 0
			installFakeRunner(t, func(context.Context, ...string) ([]byte, error) {
				viewReads++
				return nil, errors.New("must not read View")
			})
			alpha := newThreeWayContextFixture(t, "alpha", nil)
			session, err := newTraceSession(alpha.config(), "alpha")
			require.NoError(t, err)
			scope := threeWayScope{ScopeType: threeWayScopeView, ScopeValue: "806aac53-236c-446d-8ad6-91d6daf6810e"}
			options := threeWayOptions{LocalDry: tc.local, DrySummaries: tc.dry}
			_, err = collectThreeWayWithSession(context.Background(), session, scope, options)
			require.Error(t, err)
			if tc.connected {
				require.Contains(t, err.Error(), "mutually exclusive")
				require.Zero(t, *calls)
			} else {
				require.ErrorIs(t, err, errConfigHubUnavailable)
				require.Equal(t, 1, *calls)
			}
			require.Zero(t, viewReads)
			require.Empty(t, alpha.counts())
			// Editing into View executes the same eligibility check after a visible refresh.
			pane := newThreeWayTUIModel(context.Background(), session, threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}, options)
			updated, _ := pane.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
			pane = updated.(threeWayTUIModel)
			pane.input = "view/" + scope.ScopeValue
			updated, command := pane.Update(tea.KeyMsg{Type: tea.KeyEnter})
			pane = updated.(threeWayTUIModel)
			result := command().(threeWayTUIResultMsg)
			require.Error(t, result.err)
			updated, _ = pane.Update(result)
			pane = updated.(threeWayTUIModel)
			require.Contains(t, pane.View(), "Error:")
			require.Zero(t, viewReads)
			require.Empty(t, alpha.counts())
			if tc.connected {
				require.Zero(t, *calls)
			} else {
				require.Equal(t, 2, *calls, "visible refresh must recheck cached auth refusal")
			}
		})
	}
}

func TestThreeWayTUIRefreshRechecksViewAuthWithoutBlockingStandaloneResource(t *testing.T) {
	standaloneThreeWayTest(t)
	answer := error(nil)
	calls := answerGateWith(t, &answer)
	require.NoError(t, requireConfigHubFor("initial connected observation"))
	require.Equal(t, 1, *calls)
	answer = errors.New("recorded expired session")
	alpha := newThreeWayContextFixture(t, "alpha", nil)
	session, err := newTraceSession(alpha.config(), "alpha")
	require.NoError(t, err)
	reads := 0
	installFakeRunner(t, func(context.Context, ...string) ([]byte, error) {
		reads++
		return nil, errors.New("must not read View")
	})
	pane := newThreeWayTUIModel(context.Background(), session, threeWayScope{ScopeType: threeWayScopeView, ScopeValue: "806aac53-236c-446d-8ad6-91d6daf6810e"}, threeWayOptions{})
	result := pane.start()().(threeWayTUIResultMsg)
	require.ErrorIs(t, result.err, errConfigHubUnavailable)
	require.Equal(t, 2, *calls)
	require.Zero(t, reads)
	require.Empty(t, alpha.counts())
	pane.scope = threeWayScope{ScopeType: threeWayScopeResource, ScopeValue: "deploy/app"}
	pane.options.Namespace = "team"
	result = pane.start()().(threeWayTUIResultMsg)
	require.NoError(t, result.err)
	require.Equal(t, 3, *calls)
	require.NotEmpty(t, alpha.counts())
	require.Equal(t, StatePartial, result.report.Summary.Agreement.State)
	require.True(t, strings.Contains(strings.Join(result.report.Resources[0].Result.Notes, " "), compareNoteConfigHubReadsUnavailable))
}
