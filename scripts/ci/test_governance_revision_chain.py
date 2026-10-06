#!/usr/bin/env python3
"""Check genuine recorded revision boundaries, without inferring approval."""
import hashlib
import json
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2] / "test/fixtures/confighub-governance-v083-revision-chain"


def load(label):
    return json.loads((ROOT / (label + ".json")).read_bytes())


def revision(label):
    return load("revision-" + label + "-get")["Revision"]


class GenuineRevisionChain(unittest.TestCase):
    def test_all_outputs_match_recorded_hashes_and_success_status(self):
        records = load("capture-manifest")["recordings"]
        self.assertEqual(len(records), 21)
        self.assertEqual(len({r["label"] for r in records}), len(records))
        for r in records:
            with self.subTest(label=r["label"]):
                self.assertEqual(r["exitCode"], 0)
                for file_key, hash_key in [("file", "stdoutSHA256"), ("stderrFile", "stderrSHA256")]:
                    self.assertEqual(hashlib.sha256((ROOT / r[file_key]).read_bytes()).hexdigest(), r[hash_key])

    def test_multiple_direct_claim_types_bind_to_exact_initial_revision(self):
        initial = revision("initial")
        claim_ids = set()
        for label, expected_type in [("approval-create", "Approval"), ("security-create", "SecurityReview")]:
            claim = load(label)
            self.assertEqual(claim["Attestation"]["Type"], expected_type)
            self.assertEqual(claim["Attestation"]["Result"], "Pass")
            self.assertEqual(len(claim["Subjects"]), 1)
            subject = claim["Subjects"][0]
            self.assertEqual(subject["RevisionID"], initial["RevisionID"])
            self.assertEqual(subject["RevisionNum"], initial["RevisionNum"])
            self.assertEqual(subject["UnitID"], initial["UnitID"])
            claim_ids.add(claim["Attestation"]["AttestationID"])
        self.assertEqual(set(initial["Attestations"]), claim_ids)

    def test_metadata_change_does_not_create_a_new_revision(self):
        initial, updated = revision("initial"), revision("after-metadata")
        self.assertEqual(initial["RevisionID"], updated["RevisionID"])
        self.assertEqual(initial["RevisionNum"], updated["RevisionNum"])
        self.assertEqual(initial["DataHash"], updated["DataHash"])
        self.assertEqual(load("unit-after-metadata-get")["Unit"]["Labels"]["coverage-probe"], "metadata-only")

    def test_changed_data_has_new_identity_and_missing_claim_field(self):
        original, changed = revision("initial"), revision("after-data")
        self.assertNotEqual(original["RevisionID"], changed["RevisionID"])
        self.assertGreater(changed["RevisionNum"], original["RevisionNum"])
        self.assertNotEqual(original["DataHash"], changed["DataHash"])
        self.assertNotIn("Attestations", changed)

    def test_restored_hash_does_not_supply_inherited_approval(self):
        original, changed, restored = revision("initial"), revision("after-data"), revision("after-restore")
        self.assertEqual(original["DataHash"], restored["DataHash"])
        self.assertNotEqual(original["RevisionID"], restored["RevisionID"])
        self.assertGreater(restored["RevisionNum"], changed["RevisionNum"])
        self.assertNotIn("Attestations", restored)
        self.assertNotIn("EffectiveAttestations", restored)

    def test_filtered_list_omits_an_existing_referenced_claim(self):
        rows = load("attestation-filtered-list")
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["Attestation"]["Type"], "SecurityReview")
        returned = {row["Attestation"]["AttestationID"] for row in rows}
        existing_approval = load("approval-create")["Attestation"]["AttestationID"]
        self.assertNotIn(existing_approval, returned)
        self.assertIn(existing_approval, revision("initial")["Attestations"])

    def test_served_bytes_match_reported_datahash_for_each_read(self):
        for label in ["initial", "after-metadata", "after-data", "after-restore"]:
            with self.subTest(label=label):
                data = (ROOT / ("revision-" + label + "-data.txt")).read_bytes()
                self.assertEqual(hashlib.sha256(data).hexdigest(), revision(label)["DataHash"])


if __name__ == "__main__":
    unittest.main()
