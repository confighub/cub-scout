#!/usr/bin/env python3
"""Prepare an opt-in serial owned-kind #743 before/after CLI proof."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import selectors
import signal
import subprocess
import tempfile
import time
import uuid

REPO = Path(__file__).resolve().parents[2]
OLD_SOURCE = "98fe0183a932e32be3cbc9c04aae8f1d7124740d"
# Replace with the reviewed implementation commit before considering execution.
FIXED_SOURCE = "REPLACE_AFTER_IMPLEMENTATION_COMMIT"


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


MAX_OUTPUT = 2 * 1024 * 1024
MAX_SECONDS = 600
NODE_IMAGE = "kindest/node:v1.35.0@sha256:4613778f3cfcd10e615029370f5786704559103cf27bef934597ba562b269661"
KIND_VERSION = "v0.31.0"


def run(argv: list[str], *, env: dict[str, str], timeout: float = 90,
        deadline: float | None = None, max_output: int = MAX_OUTPUT) -> dict:
    """Retain partial output on failures; terminate only this command's process group."""
    started = time.monotonic()
    end = min(started + timeout, deadline) if deadline is not None else started + timeout
    result = {"argv": argv, "exitCode": None, "stdout": "", "stderr": "",
              "elapsedSeconds": 0, "failure": None}
    outputs = {"stdout": bytearray(), "stderr": bytearray()}
    proc = None
    selector = selectors.DefaultSelector()
    try:
        if end <= started:
            raise TimeoutError("overall proof deadline expired before command")
        proc = subprocess.Popen(argv, env=env, stdin=subprocess.DEVNULL,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                start_new_session=True)
        for name in outputs:
            selector.register(getattr(proc, name), selectors.EVENT_READ, name)
        while selector.get_map() or proc.poll() is None:
            remaining = end - time.monotonic()
            if remaining <= 0:
                raise TimeoutError("command deadline expired")
            for key, _ in selector.select(min(remaining, 0.1)):
                chunk = os.read(key.fileobj.fileno(), 65536)
                if not chunk:
                    selector.unregister(key.fileobj)
                    continue
                available = max_output - sum(map(len, outputs.values()))
                outputs[key.data].extend(chunk[:available])
                if len(chunk) > available:
                    raise RuntimeError("command output exceeded bound")
        result["exitCode"] = proc.wait()
    except Exception as exc:
        result["failure"] = type(exc).__name__ + ": " + str(exc)
    finally:
        if proc is not None:
            # A successful parent can also leave children holding credentials.
            # Only this newly-created session/process group is eligible for kill.
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            proc.wait()
            if result["exitCode"] is None:
                result["exitCode"] = proc.returncode
            for name in outputs:
                getattr(proc, name).close()
        selector.close()
        result.update({name: data.decode("utf-8", "replace") for name, data in outputs.items()})
        result["elapsedSeconds"] = round(time.monotonic() - started, 3)
    return result


def succeeded(result: dict) -> bool:
    return result.get("exitCode") == 0 and not result.get("failure")


def record_command(receipt: dict, phase: str, argv: list[str], *, env: dict,
                   deadline: float, timeout: float = 90, secret: bool = False) -> dict:
    result = run(argv, env=env, timeout=timeout, deadline=deadline)
    public = dict(result)
    if secret:
        # Token/config commands never write their credential-bearing output or argv.
        public["argv"] = [argv[0], "<private credential operation>"]
        public["stdout"] = "<redacted>"
        public["stderr"] = "<redacted>"
        if public["failure"]:
            public["failure"] = "private credential operation failed"
    receipt["commands"].append({"phase": phase, **public})
    if not succeeded(result):
        raise RuntimeError("command failed in phase " + phase + "; see retained receipt")
    return result


def record_observation(receipt: dict, phase: str, argv: list[str], *, env: dict,
                       deadline: float) -> dict:
    """A semantic nonzero exit may be expected; a transport failure stops work."""
    result = run(argv, env=env, deadline=deadline)
    receipt["commands"].append({"phase": phase, **result})
    if result.get("failure"):
        raise RuntimeError("observation transport failed in phase " + phase)
    return result


def validate_observations(records: list[dict], expected_resource_count: int) -> None:
    """Require the named allowed/denied CLI outcomes, not arbitrary warning text."""
    if type(expected_resource_count) is not int or expected_resource_count < 2:
        raise RuntimeError("fixture resource count lacks direct inventory evidence")
    expected = {(phase, command) for phase in (
        "old-ambient-allowed", "old-explicit-denial", "fixed-doctor-allowed", "fixed-doctor-denied")
        for command in ("doctor", "scan")}
    observations = [r for r in records if r.get("phase") in {p for p, _ in expected}]
    actual = [(r["phase"], r["argv"][1]) for r in observations]
    if len(actual) != len(expected) or set(actual) != expected:
        raise RuntimeError("before/after CLI observations are incomplete or duplicated")
    denied_user = "system:serviceaccount:scout-context-proof:doctor-denied"
    for row in observations:
        if row.get("failure") or type(row.get("exitCode")) is not int:
            raise RuntimeError("command transport failure is not observation evidence")
        phase, command = row["phase"], row["argv"][1]
        if phase == "old-explicit-denial":
            if row["exitCode"] == 0 or "unknown flag: --kube-context" not in row["stderr"]:
                raise RuntimeError("old binary did not reject exactly the unsupported selector")
            continue
        denied = phase == "fixed-doctor-denied"
        if row["exitCode"] != 0:
            if not denied or "forbidden" not in row["stderr"].lower() or denied_user not in row["stderr"]:
                raise RuntimeError("CLI failed without the required explicit RBAC denial")
            continue
        try:
            data = json.loads(row["stdout"])
            if command == "doctor":
                if data["namespace"] != "scout-context-proof":
                    raise ValueError("wrong namespace")
                if phase.startswith("fixed") and data["kubernetesContext"] != ("doctor-denied" if denied else "doctor-allowed"):
                    raise ValueError("wrong selected context")
                total = data["resources"]["total"]
                if type(total) is not int or (denied and total != 0) or (not denied and total != expected_resource_count):
                    raise ValueError("unexpected isolated fixture resource count")
                warnings = data.get("warnings", [])
            else:
                # The fixture's zero-replica Deployment starts no image pulls.
                # Endpoint routing and nonempty scan findings are proven by the
                # separate fake-server/provider tests, not this empty state scan.
                state = data["state"]
                if not isinstance(state, dict) or not isinstance(state["summary"], dict):
                    raise ValueError("missing structured state observation")
                warnings = state.get("warnings", [])
            if not isinstance(warnings, list) or any(not isinstance(w, str) for w in warnings):
                raise ValueError("malformed coverage warnings")
            if denied and not any("forbidden" in w.lower() and denied_user in w for w in warnings):
                raise ValueError("missing exact denied service-account evidence")
            if not denied and any("forbidden" in w.lower() for w in warnings):
                raise ValueError("allowed observation unexpectedly denied")
        except (ValueError, KeyError, TypeError) as exc:
            raise RuntimeError(phase + " " + command + " failed structured acceptance: " + str(exc)) from None


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--execute", action="store_true")
    ap.add_argument("--integrity-only-shared-kubeconfig", type=Path, required=True)
    ap.add_argument("--output-dir", type=Path, required=True)
    args = ap.parse_args()
    if not args.execute:
        ap.error("refusing to run without --execute")
    if FIXED_SOURCE.startswith("REPLACE_"):
        ap.error("fixed source pin has not been reviewed and recorded")
    requested_shared = args.integrity_only_shared_kubeconfig.expanduser().absolute()
    if requested_shared.is_symlink():
        ap.error("integrity-only kubeconfig must not be a symlink")
    shared = requested_shared.resolve(strict=True)
    if not shared.is_file():
        ap.error("integrity-only kubeconfig must be a regular file")
    output = args.output_dir.expanduser().absolute()
    if output.exists() or output.is_symlink() or ".." in args.output_dir.parts:
        ap.error("output directory must be fresh and must not contain '..'")
    if not any(Path(base).resolve() == output.resolve() or Path(base).resolve() in output.resolve().parents
               for base in ("/tmp", "/var/tmp")):
        ap.error("output directory must be under /tmp or /var/tmp")
    output.mkdir(parents=True, mode=0o700)
    os.chmod(output, 0o700)

    # Early setup is also evidence: missing tools/write failures must not leave
    # an apparently successful empty output directory.
    work = None
    try:
        tools = {name: shutil.which(name) for name in ("git", "go", "kind", "kubectl", "docker")}
        if any(value is None for value in tools.values()):
            raise RuntimeError("required existing tool missing: git, go, kind, kubectl")
        cluster = "scout-docscan-" + uuid.uuid4().hex[:10]
        work = Path(tempfile.mkdtemp(prefix="scout-docscan-", dir="/tmp"))
        private_config = work / "kubeconfig"
        binaries = work / "bin"
        binaries.mkdir(mode=0o700)
        shims = work / "shims"
        shims.mkdir(mode=0o700)
        cub = shims / "cub"
        cub.write_text("#!/bin/sh\necho 'ConfigHub disabled in context proof' >&2\nexit 1\n")
        cub.chmod(0o700)
        env = {key: value for key, value in os.environ.items()
               if key in ("PATH", "HOME", "TMPDIR", "LANG", "LC_ALL", "DOCKER_CONTEXT")}
        offline_config = work / "offline.kubeconfig"
        offline_config.write_text("apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\ncurrent-context: \"\"\n")
        env.update({"KUBECONFIG": str(offline_config),
                    "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local",
                    "CUB_SCOUT_SCAN_PROVIDER": "legacy", "CUB_SCOUT_OFFLINE": "true",
                    "PATH": str(shims) + os.pathsep + os.environ.get("PATH", "")})
        env.pop("CUB_SPACE", None)
        env.pop("CUB_SCOUT_TEST_SCAN_JSON", None)
        env.pop("CUB_SCOUT_TEST_DOCTOR_INPUT_JSON", None)

        receipt = {"schema": "doctor-scan-context-owned-proof.v1",
                   "oldSource": OLD_SOURCE, "fixedSource": FIXED_SOURCE,
                   "sharedKubeconfigSha256Before": digest(shared), "commands": []}
    except Exception as exc:
        failure = {"schema": "doctor-scan-context-owned-proof.v1", "acceptance": "failed",
                   "phase": "setup", "error": type(exc).__name__ + ": " + str(exc),
                   "retainedWorkDirectory": str(work) if work else None,
                   "clusterCreationAttempted": False, "commands": []}
        (output / "receipt.json").write_text(json.dumps(failure, indent=2) + "\n")
        os.chmod(output / "receipt.json", 0o600)
        raise
    deadline = time.monotonic() + MAX_SECONDS
    created_cluster = False
    creation_attempted = False
    signal_handlers = {}
    worktrees: list[Path] = []
    cleanup_errors: list[str] = []
    def interrupt(signum, _frame):
        raise InterruptedError("capture interrupted by signal " + str(signum))

    try:
        for signum in (signal.SIGINT, signal.SIGTERM):
            signal_handlers[signum] = signal.signal(signum, interrupt)
        clean = record_command(receipt, "source-clean", [tools["git"], "-C", str(REPO), "status", "--porcelain"], env=env, deadline=deadline)
        if clean["stdout"].strip():
            raise RuntimeError("capture checkout must be committed and clean")
        record_command(receipt, "source-ancestor", [tools["git"], "-C", str(REPO), "merge-base", "--is-ancestor", FIXED_SOURCE, "HEAD"], env=env, deadline=deadline)
        receipt["captureScriptSha256"] = digest(Path(__file__))
        if os.environ.get("DOCKER_HOST"):
            raise RuntimeError("DOCKER_HOST is set; refusing a remote engine")
        kind_version = record_command(receipt, "kind-version", [tools["kind"], "version"], env=env, deadline=deadline)
        fields = kind_version["stdout"].split()
        if len(fields) < 2 or fields[:2] != ["kind", KIND_VERSION]:
            raise RuntimeError("kind version differs from pinned requirement")
        docker_context = record_command(receipt, "docker-context", [tools["docker"], "context", "show"], env=env, deadline=deadline)["stdout"].strip()
        endpoint = record_command(receipt, "docker-endpoint", [tools["docker"], "context", "inspect", docker_context, "--format", "{{json .Endpoints.docker.Host}}"], env=env, deadline=deadline)
        if not json.loads(endpoint["stdout"]).startswith("unix://"):
            raise RuntimeError("refusing non-local Docker endpoint")
        image = record_command(receipt, "cached-node-image", [tools["docker"], "image", "inspect", NODE_IMAGE], env=env, deadline=deadline)
        digests = json.loads(image["stdout"])[0].get("RepoDigests", [])
        if not any(value.rsplit("@", 1)[-1] == NODE_IMAGE.rsplit("@", 1)[-1] and value.split("@")[0].split("/")[-2:] == ["kindest", "node"] for value in digests):
            raise RuntimeError("pinned node image is not already cached")
        receipt["toolPins"] = {name: {"path": path, "sha256": digest(Path(path))} for name, path in tools.items()}
        receipt["nodeImage"] = NODE_IMAGE
        for label, ref in (("old", OLD_SOURCE), ("fixed", FIXED_SOURCE)):
            checkout = work / ("source-" + label)
            worktrees.append(checkout)
            record_command(receipt, label + "-worktree", [tools["git"], "-C", str(REPO), "worktree", "add", "--detach", str(checkout), ref], env=env, deadline=deadline)
            binary = binaries / ("cub-scout-" + label)
            record_command(receipt, label + "-build", [tools["go"], "-C", str(checkout), "build", "-o", str(binary), "./cmd/cub-scout"], env=env, deadline=deadline, timeout=300)
            commit = record_command(receipt, label + "-source", [tools["git"], "-C", str(checkout), "rev-parse", "HEAD"], env=env, deadline=deadline)["stdout"].strip()
            if commit != ref:
                raise RuntimeError("built source does not match exact source pin")
            receipt[label + "SourceCommit"] = commit

        existing = record_command(receipt, "cluster-name-check", [tools["kind"], "get", "clusters"], env=env, deadline=deadline)
        if cluster in existing["stdout"].splitlines():
            raise RuntimeError("generated cluster name already exists")
        receipt["ownedCluster"] = cluster
        (output / "owned-cluster-marker.json").write_text(json.dumps({"cluster": cluster, "ownerPid": os.getpid(), "privateDirectory": str(work)}, indent=2) + "\n")
        creation_attempted = True
        record_command(receipt, "create-owned-cluster", [tools["kind"], "create", "cluster", "--name", cluster,
                       "--image", NODE_IMAGE, "--kubeconfig", str(private_config), "--wait", "90s"],
                       env={**env, "KUBECONFIG": str(private_config)}, deadline=deadline, timeout=150)
        created_cluster = True  # Success establishes creation; an intent marker alone does not.
        os.chmod(private_config, 0o600)

        kube = {**env, "KUBECONFIG": str(private_config)}
        kubectl = tools["kubectl"]
        for argv in (
            [kubectl, "config", "rename-context", "kind-" + cluster, "doctor-allowed"],
            [kubectl, "create", "namespace", "scout-context-proof", "--context", "doctor-allowed"],
            [kubectl, "-n", "scout-context-proof", "create", "deployment", "scout-context-marker",
             "--image", "registry.k8s.io/pause:3.10", "--replicas=0", "--context", "doctor-allowed"],
            [kubectl, "-n", "scout-context-proof", "create", "configmap", "scout-context-marker",
             "--from-literal=proof=owned-context", "--context", "doctor-allowed"],
            [kubectl, "-n", "scout-context-proof", "create", "serviceaccount", "doctor-denied",
             "--context", "doctor-allowed"],
        ):
            record_command(receipt, "fixture", argv, env=kube, deadline=deadline)
        # Namespace controllers add kube-root-ca.crt. Wait for that real object
        # rather than pretending our two authored manifests are the full inventory.
        record_command(receipt, "namespace-ca-ready", [kubectl, "--context", "doctor-allowed", "-n", "scout-context-proof", "wait", "--for=create", "configmap/kube-root-ca.crt", "--timeout=30s"], env=kube, deadline=deadline, timeout=40)
        fixture_inventory = record_command(receipt, "fixture-inventory", [kubectl, "--context", "doctor-allowed", "-n", "scout-context-proof", "get", "deployments,configmaps", "-o", "json"], env=kube, deadline=deadline)
        items = json.loads(fixture_inventory["stdout"])["items"]
        expected_names = {("Deployment", "scout-context-marker"), ("ConfigMap", "scout-context-marker"), ("ConfigMap", "kube-root-ca.crt")}
        actual_names = {(item["kind"], item["metadata"]["name"]) for item in items}
        if len(items) != len(expected_names) or actual_names != expected_names or any(not item["metadata"].get("uid") or item["metadata"].get("namespace") != "scout-context-proof" for item in items):
            raise RuntimeError("owned fixture inventory differs from exact expected identities")
        receipt["fixtureResourceCount"] = len(items)
        receipt["fixtureIdentities"] = [{"kind": item["kind"], "namespace": item["metadata"]["namespace"], "name": item["metadata"]["name"], "uid": item["metadata"]["uid"]} for item in items]
        token = record_command(receipt, "denied-token", [kubectl, "-n", "scout-context-proof", "create", "token", "doctor-denied",
                     "--duration=10m", "--context", "doctor-allowed"], env=kube, deadline=deadline, secret=True)
        if token["exitCode"] != 0 or not token["stdout"].strip():
            raise RuntimeError("could not mint short-lived denied-context token")
        token_value = token["stdout"].strip()
        private_view = record_command(receipt, "private-config-read", [kubectl, "config", "view", "--raw", "-o", "json"], env=kube, deadline=deadline, secret=True)
        private_data = json.loads(private_view["stdout"])
        private_data.setdefault("users", []).append({"name": "doctor-denied-user", "user": {"token": token_value}})
        private_data.setdefault("contexts", []).append({"name": "doctor-denied", "context": {
            "cluster": "kind-" + cluster, "user": "doctor-denied-user", "namespace": "scout-context-proof"}})
        for context_entry in private_data["contexts"]:
            if context_entry["name"] == "doctor-allowed":
                context_entry["context"]["namespace"] = "scout-context-proof"
        private_data["current-context"] = "doctor-allowed"
        # Write only the owned private file; no secret-bearing process arguments.
        private_config.write_text(json.dumps(private_data) + "\n")
        os.chmod(private_config, 0o600)
        receipt["privateKubeconfigSha256BeforeReads"] = digest(private_config)
        # The old command has no selector surface; prove its exact rejection.
        for command in (("doctor", "--namespace", "scout-context-proof", "--format", "json"), ("scan", "--namespace", "scout-context-proof", "--state", "--json")):
            binary = str(binaries / "cub-scout-old")
            record_observation(receipt, "old-ambient-allowed", [binary, *command], env=kube, deadline=deadline)
            record_observation(receipt, "old-explicit-denial", [binary, *command, "--kube-context", "doctor-denied"], env=kube, deadline=deadline)
        # Compare fixed-source reads against the same owned endpoint with
        # admin credentials and with a token that has no read grants.
        for context in ("doctor-allowed", "doctor-denied"):
            for command in (("doctor", "--namespace", "scout-context-proof", "--format", "json"), ("scan", "--namespace", "scout-context-proof", "--state", "--json")):
                binary = str(binaries / "cub-scout-fixed")
                record_observation(receipt, "fixed-" + context, [binary, *command, "--kube-context", context], env=kube, deadline=deadline)
        validate_observations(receipt["commands"], receipt["fixtureResourceCount"])
        receipt["acceptance"] = "passed"
    except BaseException as exc:
        receipt["acceptance"] = "failed"
        receipt["error"] = type(exc).__name__ + ": " + str(exc)
        raise
    finally:
        for signum, handler in signal_handlers.items():
            signal.signal(signum, handler)
        receipt["clusterCreationAttempted"] = creation_attempted
        receipt["clusterCreationSucceeded"] = created_cluster
        cleanup_deadline = time.monotonic() + 120
        try:
            if private_config.is_file():
                receipt["privateKubeconfigSha256AfterReads"] = digest(private_config)
                before = receipt.get("privateKubeconfigSha256BeforeReads")
                receipt["privateKubeconfigUnchangedDuringReads"] = before is not None and before == digest(private_config)
        except OSError:
            cleanup_errors.append("private-config integrity check failed")
        if created_cluster:
            deleted = run([tools["kind"], "delete", "cluster", "--name", cluster,
                           "--kubeconfig", str(private_config)], env={**env, "KUBECONFIG": str(private_config)}, timeout=90, deadline=cleanup_deadline)
            receipt["commands"].append({"phase": "cleanup-owned-cluster", **deleted})
            if not succeeded(deleted):
                cleanup_errors.append("kind delete failed")
            absent = run([tools["docker"], "ps", "-aq", "--filter", "label=io.x-k8s.kind.cluster=" + cluster], env=env, timeout=15, deadline=cleanup_deadline)
            receipt["commands"].append({"phase": "verify-owned-nodes-absent", **absent})
            if not succeeded(absent) or absent["stdout"].strip():
                cleanup_errors.append("owned node absence unverified")
        elif creation_attempted:
            # A failed create may leave partial resources OR may have collided
            # with another creator. Retain exact-name inventory for lead review;
            # never destructively clean up based solely on intention.
            uncertain = run([tools["docker"], "ps", "-aq", "--filter", "label=io.x-k8s.kind.cluster=" + cluster], env=env, timeout=15, deadline=cleanup_deadline)
            receipt["commands"].append({"phase": "uncertain-create-node-inventory", **uncertain})
            if not succeeded(uncertain) or uncertain["stdout"].strip():
                cleanup_errors.append("creation failed; possible partial resources require ownership review before removal")
        for checkout in reversed(worktrees):
            if not checkout.exists():
                continue
            result = run([tools["git"], "-C", str(REPO), "worktree", "remove", "--force", str(checkout)], env=env, deadline=cleanup_deadline)
            receipt["commands"].append({"phase": "cleanup-source", **result})
            if not succeeded(result):
                cleanup_errors.append("temporary source worktree removal failed")
        # Receipts retain outputs; private credentials are removed even on failure.
        try:
            private_config.unlink(missing_ok=True)
        except OSError:
            cleanup_errors.append("private credentials removal failed")
        try:
            receipt["sharedKubeconfigSha256After"] = digest(shared)
            receipt["sharedKubeconfigUnchanged"] = (receipt["sharedKubeconfigSha256Before"] == receipt["sharedKubeconfigSha256After"])
        except OSError as exc:
            receipt["sharedKubeconfigUnchanged"] = False
            cleanup_errors.append("shared-config integrity check failed: " + type(exc).__name__)
        if cleanup_errors or not receipt.get("sharedKubeconfigUnchanged") or not receipt.get("privateKubeconfigUnchangedDuringReads"):
            receipt["acceptance"] = "failed"
        receipt["cleanupErrors"] = cleanup_errors
        receipt["retainedWorkDirectory"] = str(work)
        (output / "receipt.json").write_text(json.dumps(receipt, indent=2) + "\n")
        os.chmod(output / "receipt.json", 0o600)
    if cleanup_errors or not receipt.get("sharedKubeconfigUnchanged") or not receipt.get("privateKubeconfigUnchangedDuringReads"):
        raise RuntimeError("proof cleanup or config integrity failed; inspect receipt")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"capture failed: {exc}", file=__import__("sys").stderr)
        raise SystemExit(1)
