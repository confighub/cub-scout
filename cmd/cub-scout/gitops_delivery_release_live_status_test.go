package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/confighub/cub-scout/v2/pkg/agent"
	"github.com/stretchr/testify/require"
)

// recordedReleasesWithLiveStatus is the release list recorded from ConfigHub
// v0.8.12, with a LiveStatus added in the shape the SDK's ReleaseLiveStatus
// prints, as argobot writes it. Entry 0 is ReleaseNum 2, entry 1 ReleaseNum 1;
// both are in one Space, for one Target.
func recordedReleasesWithLiveStatus(t *testing.T, statuses ...map[string]interface{}) (string, string, string) {
	t.Helper()
	raw, err := os.ReadFile("../../test/fixtures/confighub-release-lists-v0812-recorded/release-list.json")
	require.NoError(t, err)
	var entries []map[string]map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &entries))
	require.Len(t, entries, 2)
	for i, status := range statuses {
		if status != nil {
			entries[i]["Release"]["LiveStatus"] = status
		}
	}
	out, err := json.Marshal(entries)
	require.NoError(t, err)
	release := entries[0]["Release"]
	return string(out), release["SpaceID"].(string), release["SpaceSlug"].(string)
}

func liveStatus(sync, health, operation, observedAt string) map[string]interface{} {
	return map[string]interface{}{
		"Reporter": "argobot", "DataSource": "argo-cd",
		"Sync": sync, "Health": health, "Operation": operation,
		"ReporterSync": sync, "ReporterHealth": health, "ReporterOperation": operation,
		"Message": "reported by the deploying tool", "ObservedAt": observedAt,
	}
}

func TestLiveStatusIsReadFromTheNewestReleaseThatHasOne(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	raw, spaceID, space := recordedReleasesWithLiveStatus(t,
		liveStatus("OutOfSync", "Progressing", "Running", "2026-10-26T11:58:00.123456Z"),
		liveStatus("Synced", "Healthy", "Succeeded", "2026-10-26T11:59:00Z"),
	)
	statuses, omissions := configHubLiveStatusFromReleases(raw, now, 15*time.Minute)
	require.Empty(t, omissions)
	require.Len(t, statuses, 1, "one Space and Target: only the newest Release's status describes what runs")
	status := statuses[0]
	require.Equal(t, "release", status.RecordedOn)
	require.Equal(t, spaceID, status.SpaceID)
	require.Equal(t, space, status.Space)
	require.Equal(t, space+"#2", status.Release, "the newer Release wins even though the older one was observed later")
	require.Equal(t, "bac2e18e-526d-4efe-9d31-7f6c4a299a56", status.ReleaseID)
	require.Equal(t, "argobot", status.Source)
	require.Equal(t, "argo-cd", status.DataSource)
	require.Equal(t, "OutOfSync", status.SyncStatus)
	require.Equal(t, "Running", status.OperationPhase)
	require.Equal(t, "fresh", status.Freshness)
	require.Equal(t, agent.VerdictWATCH, status.DeliveryVerdict)
	require.Equal(t, agent.VerdictWATCH, status.ApplicationHealthVerdict)
	require.Nil(t, status.RevisionCorrelation, "a status on a Release names its Release; nothing is inferred from a reported revision")
}

func TestReleaseLiveStatusWithNoObservedTimeIsNotFresh(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	// The SDK prints an unset ObservedAt as the zero time.
	raw, _, _ := recordedReleasesWithLiveStatus(t, liveStatus("Synced", "Healthy", "Succeeded", "0001-01-01T00:00:00Z"))
	statuses, omissions := configHubLiveStatusFromReleases(raw, now, 15*time.Minute)
	require.Len(t, statuses, 1)
	require.Equal(t, "unknown", statuses[0].Freshness)
	require.Equal(t, agent.VerdictINCONCLUSIVE, statuses[0].DeliveryVerdict)
	require.Len(t, omissions, 1)
	require.Contains(t, omissions[0].Reason, "missing observedAt")
}

func TestReleasesWithoutLiveStatusGiveNoStatus(t *testing.T) {
	raw, _, _ := recordedReleasesWithLiveStatus(t)
	statuses, omissions := configHubLiveStatusFromReleases(raw, time.Now(), 15*time.Minute)
	require.Empty(t, statuses)
	require.Empty(t, omissions)
}

// gitops status --with-confighub: the status on a Release is reported, and the
// space is not also reported as missing its (retired) annotation. A space with
// neither says where both were looked for.
func TestCollectorReadsLiveStatusFromReleasesAndTheAnnotation(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	raw, spaceID, space := recordedReleasesWithLiveStatus(t, liveStatus("Synced", "Healthy", "Succeeded", "2026-10-26T11:59:00Z"))
	oldRequire, oldRun := requireGitOpsConfigHubFn, runGitOpsCubCommand
	t.Cleanup(func() { requireGitOpsConfigHubFn, runGitOpsCubCommand = oldRequire, oldRun })
	requireGitOpsConfigHubFn = func() error { return nil }
	runGitOpsCubCommand = func(_ context.Context, args []string) (string, error) {
		switch {
		case args[0] == "space":
			return `[{"Space":{"Slug":"` + space + `","SpaceID":"` + spaceID + `","Annotations":{}}},{"Space":{"Slug":"quiet","SpaceID":"sp-quiet","Annotations":{}}}]`, nil
		case args[0] == "release":
			return raw, nil
		default:
			return "[]", nil
		}
	}
	evidence := collectGitOpsDeliveryEvidence(context.Background(), newGitOpsDeliveryFakeClient(), gitOpsDeliveryEvidenceOptions{
		Space: "*", Now: now, Window: 24 * time.Hour, StaleAfter: 15 * time.Minute, MaxItems: 10,
	})
	require.Len(t, evidence.ConfigHub.LiveStatuses, 1)
	require.Equal(t, "release", evidence.ConfigHub.LiveStatuses[0].RecordedOn)
	require.Equal(t, agent.VerdictPASS, evidence.ConfigHub.LiveStatuses[0].DeliveryVerdict)
	var liveOmissions []string
	for _, omission := range evidence.Omissions {
		if omission.Layer == "confighub.liveStatus" {
			liveOmissions = append(liveOmissions, omission.Reason)
		}
	}
	require.Len(t, liveOmissions, 1, "only the space with no status anywhere: %v", liveOmissions)
	require.Contains(t, liveOmissions[0], `space "quiet" has no live status: no Release created since 2026-10-25T12:00:00Z carries LiveStatus`)

	var markdown strings.Builder
	renderGitOpsDeliveryEvidenceMarkdown(&markdown, evidence)
	require.Contains(t, markdown.String(), "| release `")
	require.Contains(t, markdown.String(), space+"#2")
	ascii := captureStdout(t, func() { outputGitOpsDeliveryEvidenceHuman(evidence) })
	require.Contains(t, ascii, "recorded on release "+space+"#2 (reporter argobot)")
}

// The MCP tool reads both, as the CLI does, and says so when the Releases
// could not be read rather than reporting no status.
func TestMCPLiveStatusReadsReleasesAsTheCLIDoes(t *testing.T) {
	raw, spaceID, space := recordedReleasesWithLiveStatus(t, liveStatus("Synced", "Degraded", "Succeeded", time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)))
	spaces := `[{"Space":{"Slug":"` + space + `","SpaceID":"` + spaceID + `","Annotations":{}}}]`
	call := func(releaseErr error) map[string]interface{} {
		t.Helper()
		gateway := newMCPGatewayWithMode(nil, func(_ context.Context, args []string) (string, error) {
			if args[0] == "release" {
				if releaseErr != nil {
					return "", releaseErr
				}
				return raw, nil
			}
			return spaces, nil
		}, true)
		resp := gateway.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call",
			Params: json.RawMessage(`{"name":"confighub_live_status","arguments":{"space":"` + space + `","since":"7d"}}`)})
		require.NotNil(t, resp)
		require.Nil(t, resp.Error)
		var result struct {
			StructuredContent map[string]interface{} `json:"structuredContent"`
		}
		require.NoError(t, marshalInto(resp.Result, &result))
		return result.StructuredContent
	}

	content := call(nil)
	statuses, _ := content["liveStatuses"].([]interface{})
	require.Len(t, statuses, 1)
	status := statuses[0].(map[string]interface{})
	require.Equal(t, "release", status["recordedOn"])
	require.Equal(t, "Degraded", status["healthStatus"])
	require.Equal(t, string(agent.VerdictBLOCK), status["applicationHealthVerdict"])
	require.Nil(t, content["omissions"])

	content = call(context.DeadlineExceeded)
	require.Nil(t, content["liveStatuses"])
	encoded, _ := json.Marshal(content["omissions"])
	require.Contains(t, string(encoded), "the release list could not be read")
}

func TestMCPLiveStatusRefusesABadWindow(t *testing.T) {
	tool := newMCPGatewayWithMode(nil, func(context.Context, []string) (string, error) { return "[]", nil }, true).tools["confighub_live_status"]
	_, err := tool.BuildArgs(map[string]interface{}{"space": "prod", "since": "soon"})
	require.ErrorContains(t, err, `invalid since "soon"`)
	args, err := tool.BuildArgs(map[string]interface{}{"space": "prod"})
	require.NoError(t, err)
	require.Equal(t, []string{"prod", "24h"}, args)
}
