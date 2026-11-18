"""Offline checks for the before-attach-wired-cluster case (#888).

The expected answer is derived again here from the raw export, independently
of cub-scout: the help text's rule plus the Secret list. The graders must take
the expected lines and refuse the wrong ones, and the doctor recordings must
say what attach did.
"""
import json
import pathlib
import re
import subprocess
import tempfile
import unittest

CASE = pathlib.Path(__file__).resolve().parent
EXPORT = CASE / "fixtures" / "cluster"


def grader_pattern(name):
    text = (CASE / "graders" / name).read_text()
    pattern = re.search(r"^pattern: '(.*)'$", text, re.M).group(1).replace("''", "'")
    return re.compile(pattern, re.I | re.M)


def secrets():
    rows = (EXPORT / "secrets-by-name.txt").read_text().splitlines()[1:]
    return {"/".join(row.split()[:2]) for row in rows if row.strip()}


def deployments():
    rows = (EXPORT / "workloads.txt").read_text().splitlines()[1:]
    return {"/".join(row.split()[1:3]) for row in rows if row.split()[:1] == ["Deployment"]}


class ExpectedAnswer(unittest.TestCase):
    def test_the_help_text_states_the_rule_and_names_the_secret(self):
        help_text = (EXPORT / "cub-cluster-attach-help.txt").read_text()
        self.assertIn("attach refuses a cluster whose\nArgo CD already holds ConfigHub registry credentials", help_text)
        self.assertIn("argocd/confighub-oci-creds", help_text)

    def test_the_export_holds_the_secret_and_an_installed_argo_cd(self):
        self.assertIn("argocd/confighub-oci-creds", secrets())
        # Installed Argo CD alone would mean "use the existing one": the trap.
        self.assertIn("argocd/argocd-server", deployments())

    def test_no_secret_data_is_in_the_export(self):
        for path in EXPORT.iterdir():
            text = path.read_text()
            self.assertNotRegex(text, r'"data"\s*:\s*\{\s*"', path.name)
            self.assertNotIn("eyJ", text, path.name)


class Graders(unittest.TestCase):
    def test_expected_lines_pass(self):
        answer = "Some reasoning.\n\nATTACH: refuses\nBLOCKER: argocd/confighub-oci-creds\n"
        self.assertTrue(grader_pattern("attach-line.md").search(answer))
        self.assertTrue(grader_pattern("blocker-line.md").search(answer))
        decorated = "**ATTACH:** `refuses`\n**BLOCKER:** `argocd/confighub-oci-creds`"
        self.assertTrue(grader_pattern("attach-line.md").search(decorated))
        self.assertTrue(grader_pattern("blocker-line.md").search(decorated))

    def test_wrong_lines_fail(self):
        for line in ["ATTACH: proceeds", "ATTACH: unknown", "ATTACH: refuses or proceeds"]:
            self.assertFalse(grader_pattern("attach-line.md").search(line), line)
        for line in ["BLOCKER: none", "BLOCKER: argocd/argocd-server", "BLOCKER: argocd/confighub-oci-creds, argobot/argobot-secrets"]:
            self.assertFalse(grader_pattern("blocker-line.md").search(line), line)


class Scaffold(unittest.TestCase):
    def test_scaffold_reproduces_the_export(self):
        with tempfile.TemporaryDirectory() as work:
            subprocess.run(["bash", str(CASE / "scaffold.sh")], cwd=work, check=True)
            made = pathlib.Path(work) / "cluster"
            self.assertEqual(sorted(p.name for p in made.iterdir()), sorted(p.name for p in EXPORT.iterdir()))
            for path in EXPORT.iterdir():
                self.assertEqual((made / path.name).read_text(), path.read_text(), path.name)


class Recordings(unittest.TestCase):
    def test_doctor_before_attach_recording_says_refuse_for_argo(self):
        report = json.loads((CASE / "mocks" / "cub-scout" / "fixtures" / "doctor-before-attach.json").read_text())["attach"]
        argo = next(c for c in report["controllers"] if c["controller"] == "argo")
        self.assertEqual(argo["attachWould"], "refuse")
        self.assertIn("argocd/confighub-oci-creds", argo["why"])

    def test_plain_doctor_recording_has_no_attach_section(self):
        doctor = json.loads((CASE / "mocks" / "cub-scout" / "fixtures" / "doctor.json").read_text())
        self.assertNotIn("attach", doctor)

    def test_tool_catalog_offers_before_attach(self):
        tools = json.loads((CASE / "mocks" / "cub-scout" / "_tools.json").read_text())["tools"]
        doctor = next(t for t in tools if t["name"] == "doctor")
        self.assertIn("before_attach", doctor["inputSchema"]["properties"])


if __name__ == "__main__":
    unittest.main()
