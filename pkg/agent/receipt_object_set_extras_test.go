// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package agent

import (
	"testing"
	"time"
)

func closedWorldEvidence(t *testing.T, extras []ObjectSetObjectID, extraChecked bool) ObjectSetEvidence {
	t.Helper()
	desired := objectSetDeployment(3, "example/api:v1")
	live := objectSetDeployment(3, "example/api:v1")
	ev, err := BuildObjectSetEvidence(
		ObjectSetSource{Type: "file", Ref: "rendered.yaml"},
		ObjectSetScope{Kind: "namespace", Namespace: "prod"},
		[]ObjectSetObservedObject{{Desired: desired, Live: live}},
	)
	if err != nil {
		t.Fatalf("BuildObjectSetEvidence: %v", err)
	}
	ev.ExtraChecked = extraChecked
	ev.ExtraObjects = extras
	return ev
}

func TestObjectSet_ClosedWorld_WATCHOnExtras(t *testing.T) {
	ev := closedWorldEvidence(t, []ObjectSetObjectID{
		{APIVersion: "v1", Kind: "ConfigMap", Namespace: "prod", Name: "drift-not-in-object-set"},
	}, true)
	stmt, err := BuildObjectSetReceipt(BuildObjectSetReceiptInput{
		Evidence:   ev,
		Verifier:   Verifier{Tool: "cub-scout", Version: "test"},
		VerifiedAt: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildObjectSetReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictWATCH {
		t.Fatalf("verdict = %s, want WATCH", stmt.Predicate.Verdict)
	}
	hasExtras, hasCoverage := false, false
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == OmissionExtraLiveObjects {
			hasExtras = true
		}
		if o.Missing == OmissionExtraLiveObjectCoverage {
			hasCoverage = true
		}
	}
	if !hasExtras {
		t.Fatal("expected extra-live-objects omission")
	}
	if hasCoverage {
		t.Fatal("closed-world check ran; extra-live-object-coverage omission must be dropped")
	}
	if len(stmt.Predicate.Evidence.ObjectSet.ExtraObjects) != 1 {
		t.Fatalf("extraObjects not surfaced in evidence: %+v", stmt.Predicate.Evidence.ObjectSet.ExtraObjects)
	}
	if err := VerifyStatementFingerprint(stmt); err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
}

func TestObjectSet_ClosedWorld_PASSWhenExclusive(t *testing.T) {
	ev := closedWorldEvidence(t, nil, true) // checked, no extras
	stmt, err := BuildObjectSetReceipt(BuildObjectSetReceiptInput{
		Evidence: ev,
		Verifier: Verifier{Tool: "cub-scout", Version: "test"},
	})
	if err != nil {
		t.Fatalf("BuildObjectSetReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", stmt.Predicate.Verdict)
	}
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == OmissionExtraLiveObjectCoverage {
			t.Fatal("closed-world clean: coverage omission must be dropped")
		}
		if o.Missing == OmissionExtraLiveObjects {
			t.Fatal("no extras: extra-objects omission must be absent")
		}
	}
}

func TestObjectSet_NoClosedWorld_KeepsCoverageOmission(t *testing.T) {
	ev := closedWorldEvidence(t, nil, false) // not checked
	stmt, err := BuildObjectSetReceipt(BuildObjectSetReceiptInput{
		Evidence: ev,
		Verifier: Verifier{Tool: "cub-scout", Version: "test"},
	})
	if err != nil {
		t.Fatalf("BuildObjectSetReceipt: %v", err)
	}
	if stmt.Predicate.Verdict != VerdictPASS {
		t.Fatalf("verdict = %s, want PASS", stmt.Predicate.Verdict)
	}
	found := false
	for _, o := range stmt.Predicate.Omissions {
		if o.Missing == OmissionExtraLiveObjectCoverage {
			found = true
		}
	}
	if !found {
		t.Fatal("without closed-world, the extra-live-object-coverage omission must remain")
	}
}
