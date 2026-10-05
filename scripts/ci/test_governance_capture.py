#!/usr/bin/env python3
"""Validate the captured packet; this is not all governance acceptance."""
import datetime
import hashlib
import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2] / "test/fixtures/confighub-governance-v083-recorded"


def load(name):
    return json.loads((ROOT / (name + ".json")).read_text())


class GenuineGovernanceCapture(unittest.TestCase):
    def test_raw_command_outputs_match_retained_hashes(self):
        for record in load("capture-manifest")["recordings"]:
            with self.subTest(label=record["label"]):
                self.assertEqual(hashlib.sha256((ROOT / record["file"]).read_bytes()).hexdigest(), record["stdoutSHA256"])
                if record["exitCode"]:
                    self.assertEqual(hashlib.sha256((ROOT / record["stderrFile"]).read_bytes()).hexdigest(), record["stderrSHA256"])
                datetime.datetime.fromisoformat(record["retrievedAt"])

    def test_exact_revision_served_bytes_and_identity(self):
        revision = load("revision-get")["Revision"]
        unit = load("unit-get")["Unit"]
        self.assertEqual(revision["UnitID"], unit["UnitID"])
        self.assertEqual(revision["RevisionID"], unit["HeadRevisionID"])
        self.assertEqual(revision["RevisionNum"], unit["HeadRevisionNum"])
        self.assertEqual(hashlib.sha256((ROOT / "revision-data.txt").read_bytes()).hexdigest(), revision["DataHash"])
        self.assertEqual(len((ROOT / "revision-data.txt").read_bytes()), revision["DataSize"])

    def test_server_subjects_bind_direct_revision_references(self):
        revision = load("revision-get")["Revision"]
        for label, outcome in [("attestation-pass-create", "Pass"), ("attestation-fail-create", "Fail"), ("attestation-expiring-create", "Pass")]:
            with self.subTest(label=label):
                response = load(label)
                self.assertEqual(response["Attestation"]["Result"], outcome)
                self.assertIn(response["Attestation"]["AttestationID"], revision["Attestations"])
                self.assertEqual(len(response["Subjects"]), 1)
                subject = response["Subjects"][0]
                for key in ("UnitID", "RevisionID", "RevisionNum"):
                    self.assertEqual(subject[key], revision[key])

    def test_revocation_is_separate_from_immutable_claim(self):
        before = load("attestation-pass-get-before-revoke")["Attestation"]
        after = load("attestation-pass-get-after-revoke")["Attestation"]
        self.assertEqual(before, after)
        rows = load("attestation-list-after-revoke")
        revocations = [row["Attestation"] for row in rows if row["Attestation"].get("RevokedAttestationID") == before["AttestationID"]]
        self.assertEqual(len(revocations), 1)
        self.assertNotEqual(revocations[0]["AttestationID"], before["AttestationID"])

    def test_expiry_is_before_actual_retrieval(self):
        attestation = load("attestation-expired-get")["Attestation"]
        record = next(r for r in load("capture-manifest")["recordings"] if r["label"] == "attestation-expired-get")
        self.assertLess(datetime.datetime.fromisoformat(attestation["ExpiresAt"]), datetime.datetime.fromisoformat(record["retrievedAt"]))

    def test_missing_id_is_a_real_refusal_and_not_a_subject(self):
        record = next(r for r in load("capture-manifest")["recordings"] if r["label"] == "attestation-missing-id-get")
        self.assertNotEqual(record["exitCode"], 0)
        self.assertEqual((ROOT / record["file"]).read_bytes(), b"")
        self.assertNotIn(record["argv"][3], load("revision-get")["Revision"]["Attestations"])

    def test_ungoverned_changeorder_is_not_gate_evaluation_evidence(self):
        change = load("changeorder-get")["ChangeOrder"]
        self.assertTrue(change["ChangeOrderID"])
        self.assertNotIn("ChangeWorkflowID", change)
        self.assertNotIn("Gates", change)


if __name__ == "__main__": unittest.main()
