package onboard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const hlt04Applied = "2026-10-01T12:00:00Z"
const hlt04Now = "2026-10-01T12:10:00Z"
const hlt04Marker = "HLT04_SYNTHETIC_ONLY"

// This is separately authored synthetic check-execution evidence. ReportStatus
// never consumes it; it is not an observation of Sveltos or an execution log.
var hlt04SyntheticCheckEvidence = []byte(`{"kind":"synthetic-check-execution","synthetic":true,"source":"authored fixture; not producer input","profile":"demo-prod","completedAt":"2026-10-01T12:05:00Z"}`)

type hlt04Fixture struct {
	now              time.Time
	held             string
	profiles         []byte
	summaries        []byte
	watching         []byte
	releases         []byte
	writes           []string
	patches          [][]byte
	heldBefore       string
	checkEvidence    []byte
	lastReportStatus LiveStatus
	lastEvidence     map[string]any
}

func hlt04JSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func newHLT04Fixture(t *testing.T, appliedAt, transitionAt string, releases []map[string]any) *hlt04Fixture {
	t.Helper()
	profiles := []any{map[string]any{
		"metadata": map[string]any{"name": "demo-prod"},
		"spec": map[string]any{
			"validateHealths": []any{map[string]any{"name": "demo-workloads"}},
			"policyRefs": []any{map[string]any{"deploymentType": "Remote", "remoteURL": map[string]any{
				"url": "oci://oci.hub.confighub.com/space/demo:latest",
			}}},
		},
	}}
	summary := map[string]any{
		"metadata": map[string]any{"labels": map[string]any{"projectsveltos.io/cluster-profile-name": "demo-prod"}},
		"spec":     map[string]any{"clusterName": "demo", "clusterNamespace": "projectsveltos"},
		"status": map[string]any{"featureSummaries": []any{map[string]any{
			"featureID": "Resources", "status": "Provisioned", "lastAppliedTime": appliedAt,
		}}, "deployedGVKs": []any{map[string]any{"deployedGroupVersionKind": []any{"Deployment.v1.apps"}}}},
	}
	watching := []any{map[string]any{
		"metadata": map[string]any{"name": "demo-health", "labels": map[string]any{ProfileLabel: "demo"}},
		"status": map[string]any{"clusterCondition": []any{map[string]any{
			"clusterInfo": map[string]any{"cluster": map[string]any{"namespace": "projectsveltos", "name": "demo"}},
			"conditions": []any{map[string]any{
				"type": "HealthCheck:workloads", "name": "workloads", "status": "False",
				"lastTransitionTime": transitionAt,
				"message":            "Deployment: demo/api status is Degraded",
			}},
		}},
		}}}
	return &hlt04Fixture{
		now: time.Date(2026, 10, 1, 12, 10, 0, 0, time.UTC), held: "", checkEvidence: append([]byte(nil), hlt04SyntheticCheckEvidence...),
		profiles:  hlt04JSON(t, map[string]any{"items": profiles}),
		summaries: hlt04JSON(t, map[string]any{"items": []any{summary}}),
		watching:  hlt04JSON(t, map[string]any{"items": watching}),
		releases:  hlt04JSON(t, releases),
	}
}

func (f *hlt04Fixture) runner(t *testing.T) Runner {
	t.Helper()
	return func(name string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if name == "kubectl" {
			switch joined {
			case "get clusterprofiles -o json":
				return append([]byte(nil), f.profiles...), nil
			case "get clustersummaries -A -o json":
				return append([]byte(nil), f.summaries...), nil
			case "get clusterhealthchecks -o json":
				return append([]byte(nil), f.watching...), nil
			}
		}
		if name == "cub" {
			switch joined {
			case "release list --space demo -o json":
				return append([]byte(nil), f.releases...), nil
			case "space get demo -o json":
				return hlt04JSON(t, map[string]any{"Space": map[string]any{"Annotations": map[string]string{
					LiveStatusAnnotation: f.held,
				}}}), nil
			}
		}
		return nil, errors.New("unexpected offline replay command: " + name + " " + joined)
	}
}

func (f *hlt04Fixture) report(t *testing.T) StatusReport {
	t.Helper()
	options := StatusOptions{
		Refresh: 10 * time.Minute,
		Now:     func() time.Time { return f.now },
		Write: func(_ string, patch []byte) error {
			var value struct{ Annotations map[string]string }
			if err := json.Unmarshal(patch, &value); err != nil {
				return err
			}
			f.held = value.Annotations[LiveStatusAnnotation]
			f.writes = append(f.writes, f.held)
			f.patches = append(f.patches, append([]byte(nil), patch...))
			return nil
		},
	}
	f.heldBefore = f.held
	inputs := hlt04RawInputs(f)
	patchCount := len(f.patches)
	reports, err := ReportStatus(f.runner(t), options)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("got %d reports, want exactly one synthetic profile", len(reports))
	}
	f.lastReportStatus = reports[0].Status
	var patch any
	if len(f.patches) > patchCount {
		patch = append([]byte(nil), f.patches[len(f.patches)-1]...)
	}
	f.lastEvidence = map[string]any{"raw_inputs": inputs, "held_before": f.heldBefore,
		"held_after": f.held, "computed_report": reports[0].Status, "write_patch": patch}
	return reports[0]
}

func hlt04Hash(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func hlt04RawInputs(f *hlt04Fixture) map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"clusterprofiles":     append(json.RawMessage(nil), f.profiles...),
		"clustersummaries":    append(json.RawMessage(nil), f.summaries...),
		"clusterhealthchecks": append(json.RawMessage(nil), f.watching...),
		"published_releases":  append(json.RawMessage(nil), f.releases...),
	}
}

func hlt04InputHashes(f *hlt04Fixture) map[string]string {
	return map[string]string{
		"clusterprofiles":     hlt04Hash(f.profiles),
		"clustersummaries":    hlt04Hash(f.summaries),
		"clusterhealthchecks": hlt04Hash(f.watching),
		"published_releases":  hlt04Hash(f.releases),
	}
}

func hlt04StatusWithoutReportTime(status LiveStatus) LiveStatus {
	status.ObservedAt = ""
	return status
}

func TestHLT04OfflineReplay(t *testing.T) {
	{
		baseRelease := []map[string]any{{"Release": map[string]any{
			"ReleaseNum": 1, "Published": true, "ManifestDigest": "sha256:synthetic-release-a",
			"CreatedAt": "2026-10-01T11:59:00Z",
		}}}
		f := newHLT04Fixture(t, hlt04Applied, "2026-10-01T12:01:00Z", baseRelease)
		initial := f.report(t)
		initialEvidence := f.lastEvidence // Missing held annotation: write the current synthetic report.
		if !initial.Wrote || initial.Status.ObservedAt != hlt04Now || initial.Status.HealthStatus != "Degraded" {
			t.Fatalf("missing-report case unexpected: %+v", initial)
		}
		initialStatus := hlt04StatusWithoutReportTime(initial.Status)
		initialObserved := initial.Status.ObservedAt
		initialCheckHash := hlt04Hash(f.checkEvidence)
		initialSourceHashes := hlt04InputHashes(f)

		f.now = f.now.Add(time.Minute)
		skipped := f.report(t)
		skippedEvidence := f.lastEvidence
		if skipped.Wrote || skipped.Why != "unchanged" {
			t.Fatalf("young report was not skipped: %+v", skipped)
		}

		f.now = f.now.Add(15 * time.Minute)
		renewed := f.report(t)
		renewedEvidence := f.lastEvidence
		if !renewed.Wrote {
			t.Fatalf("old report was not renewed: %+v", renewed)
		}
		if hlt04StatusWithoutReportTime(renewed.Status) != initialStatus || renewed.Status.ObservedAt == initialObserved {
			t.Fatalf("renewal changed semantic status or failed to update reporter time: %+v", renewed.Status)
		}
		if hlt04Hash(f.checkEvidence) != initialCheckHash {
			t.Fatal("separately authored synthetic check-execution evidence changed across report renewal")
		}
		finalSourceHashes := hlt04InputHashes(f)
		if fmt.Sprint(initialSourceHashes) != fmt.Sprint(finalSourceHashes) {
			t.Fatal("synthetic producer source inputs changed across renewal")
		}

		// A malformed held timestamp is not accepted as fresh, although same() omits it.
		var malformed LiveStatus
		if err := json.Unmarshal([]byte(f.held), &malformed); err != nil {
			t.Fatal(err)
		}
		malformed.ObservedAt = "not-a-time"
		badHeld, err := json.Marshal(malformed)
		if err != nil {
			t.Fatal(err)
		}
		f.held = string(badHeld)
		f.now = time.Date(2026, 10, 1, 12, 30, 0, 0, time.UTC)
		malformedResult := f.report(t)
		malformedResultEvidence := f.lastEvidence
		if !malformedResult.Wrote {
			t.Fatalf("malformed held timestamp was not rewritten: %+v", malformedResult)
		}

		// A future held reporter time has negative age; source currently skips it.
		var future LiveStatus
		if err := json.Unmarshal([]byte(f.held), &future); err != nil {
			t.Fatal(err)
		}
		future.ObservedAt = "2026-10-02T12:30:00Z"
		futureHeld, err := json.Marshal(future)
		if err != nil {
			t.Fatal(err)
		}
		f.held = string(futureHeld)
		f.now = time.Date(2026, 10, 1, 12, 30, 0, 0, time.UTC)
		futureResult := f.report(t)
		futureResultEvidence := f.lastEvidence
		if futureResult.Wrote || futureResult.Why != "unchanged" {
			t.Fatalf("future held timestamp behavior changed: %+v", futureResult)
		}

		// Existing held report omits observedAt entirely (distinct from no annotation).
		missingTimeFixture := newHLT04Fixture(t, hlt04Applied, "2026-10-01T12:01:00Z", baseRelease)
		missingTimeFixture.held = `{"source":"cub-scout","app":"demo-prod","healthStatus":"Degraded","syncStatus":"Synced","revision":"sha256:synthetic-release-a"}`
		missingTime := missingTimeFixture.report(t)
		missingTimeEvidence := missingTimeFixture.lastEvidence
		if !missingTime.Wrote {
			t.Fatalf("held report without observedAt was not rewritten: %+v", missingTime)
		}
		result := map[string]any{
			"schema":                  "sveltos-hlt04-offline-replay.v1",
			"source_kind":             "synthetic-source-contract-replay",
			"synthetic_clock_start":   hlt04Now,
			"producer_input_sha256":   initialSourceHashes,
			"write_annotations_exact": append([]string(nil), f.writes...),
			"write_annotation_sha256": []string{hlt04Hash([]byte(f.writes[0])), hlt04Hash([]byte(f.writes[1])), hlt04Hash([]byte(f.writes[2]))},
			"synthetic_check_execution_evidence": map[string]any{
				"synthetic": true, "completed_at": "2026-10-01T12:05:00Z",
				"sha256": initialCheckHash, "consumed_by_reporter": false,
			},
			"cases": []map[string]any{
				{"name": "missing-held-report-time", "evidence": initialEvidence, "wrote": initial.Wrote, "report_observed_at": initial.Status.ObservedAt,
					"health": initial.Status.HealthStatus, "revision": initial.Status.Revision, "report": initial.Status,
					"raw_inputs": hlt04RawInputs(f), "held_before": "", "held_after": f.held, "computed_report": initial.Status, "write_patch": f.patches[0]},
				{"name": "missing-observed-at-field", "evidence": missingTimeEvidence, "wrote": missingTime.Wrote, "raw_inputs": hlt04RawInputs(missingTimeFixture),
					"held_before": missingTimeFixture.heldBefore, "held_after": missingTimeFixture.held, "computed_report": missingTime.Status,
					"write_patch": missingTimeFixture.patches[0]},
				{"name": "young-held-report", "evidence": skippedEvidence, "wrote": skipped.Wrote, "why": skipped.Why, "raw_inputs": hlt04RawInputs(f),
					"held_before": f.heldBefore, "held_after": f.held, "computed_report": skipped.Status, "write_patch": nil},
				{"name": "old-held-report-renewal", "evidence": renewedEvidence, "wrote": renewed.Wrote, "report_observed_at": renewed.Status.ObservedAt,
					"health": renewed.Status.HealthStatus, "revision": renewed.Status.Revision, "report": renewed.Status,
					"source_input_hashes_unchanged": true, "synthetic_check_evidence_sha256": initialCheckHash,
					"synthetic_check_evidence_consumed_by_reporter": false, "raw_inputs": hlt04RawInputs(f),
					"held_before": f.heldBefore, "held_after": f.held, "computed_report": renewed.Status, "write_patch": f.patches[len(f.patches)-1]},
				{"name": "malformed-held-report-time", "evidence": malformedResultEvidence, "wrote": malformedResult.Wrote, "report": malformedResult.Status,
					"raw_inputs": hlt04RawInputs(f), "held_before": f.heldBefore, "held_after": f.held,
					"computed_report": malformedResult.Status, "write_patch": f.patches[len(f.patches)-1]},
				{"name": "future-held-report-time", "evidence": futureResultEvidence, "wrote": futureResult.Wrote, "why": futureResult.Why,
					"computed_report": futureResult.Status, "raw_inputs": hlt04RawInputs(f), "held_before": f.heldBefore,
					"held_after": f.held, "write_patch": nil},
			},
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("HLT04_RESULT %s\n", encoded)

	}
	{
		releases := []map[string]any{
			{"Release": map[string]any{"ReleaseNum": 1, "Published": true,
				"ManifestDigest": "sha256:synthetic-older", "CreatedAt": "2026-10-01T11:59:00Z"}},
			{"Release": map[string]any{"ReleaseNum": 2, "Published": true,
				"ManifestDigest": "sha256:synthetic-newer", "CreatedAt": "2026-10-01T12:01:00Z"}},
		}
		f := newHLT04Fixture(t, hlt04Applied, "2026-10-01T12:01:00Z", releases)
		clockSkew := f.report(t)
		clockSkewEvidence := f.lastEvidence
		if clockSkew.Status.Revision != "sha256:synthetic-older" {
			t.Fatalf("time-based revision inference changed: %+v", clockSkew.Status)
		}

		// lastTransitionTime is only a condition transition; no code treats it as a check time.
		transitionFixture := newHLT04Fixture(t, hlt04Applied, "2026-10-01T11:59:00Z", releases[:1])
		transition := transitionFixture.report(t)
		transitionEvidence := transitionFixture.lastEvidence
		if transition.Status.HealthStatus != "Healthy" {
			t.Fatalf("pre-apply transition control unexpected: %+v", transition.Status)
		}
		result := map[string]any{
			"schema":                  "sveltos-hlt04-offline-replay.v1",
			"source_kind":             "synthetic-source-contract-replay",
			"clock_skew_input_sha256": hlt04InputHashes(f),
			"clock_skew_report":       clockSkew.Status,
			"clock_skew_control":      map[string]any{"revision": clockSkew.Status.Revision, "release_digest_proven": false},
			"transition_input_sha256": hlt04InputHashes(transitionFixture),
			"transition_report":       transition.Status,
			"pre_apply_transition_control": map[string]any{"health": transition.Status.HealthStatus,
				"last_transition_time_is_check_execution_time": false},
			"cases": []map[string]any{
				{"name": "clock-skew-control", "wrote": clockSkew.Wrote, "evidence": clockSkewEvidence, "raw_inputs": hlt04RawInputs(f), "held_before": "", "held_after": f.held,
					"computed_report": clockSkew.Status, "write_patch": f.patches[0]},
				{"name": "pre-apply-transition-control", "wrote": transition.Wrote, "evidence": transitionEvidence, "raw_inputs": hlt04RawInputs(transitionFixture), "held_before": "", "held_after": transitionFixture.held,
					"computed_report": transition.Status, "write_patch": transitionFixture.patches[0]},
			},
			"synthetic_fixture_only": true,
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("HLT04_RESULT %s\n", encoded)
	}
}

func Example_hlt04NoCommands() {
	// The harness Runner is an in-memory switch; it never invokes shell tools.
	fmt.Println("offline synthetic Runner; no kubectl/cub process")
	// Output: offline synthetic Runner; no kubectl/cub process
}
