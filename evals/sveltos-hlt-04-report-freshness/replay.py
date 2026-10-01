#!/usr/bin/env python3
"""Prepare or run the pinned, local-only HLT-04 producer replay."""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
from pathlib import Path, PurePosixPath
import platform
import resource
import selectors
import shutil
import signal
import stat
import subprocess
import tarfile
import time
from typing import Any


SOURCE_ROOT = Path("/Users/alexis/code/sveltos-confighub-work")
SOURCE_COMMIT = "8187910f9fe226e109e55c4d9c7c0e21297ff424"
SOURCE_HASHES = {
    "go.mod": "e480afbdc90c7571cbdee94a91008d04732b7cf3e07c4844fd9512112f2c3eec",
    "go.sum": "3b453000ed3ac0098f9d002ff575e32f247b6ad21e3ee64f6b7d5ce8b3848efd",
    "chartrender/README.md": "1ee21d182721b4ed21edec7416cced26e3dd5eca33e813b7af97bbcffc574ce4",
    "chartrender/chartrender.go": "8a245abe52d93a4a2947caa07630d75825b3e15c26e95b9b426d192caee62452",
    "chartrender/chartrender_test.go": "f8a2527d2c600610b19276f61c8403114437736ae9a041506859bb85566732db",
    "chartrender/compare.go": "3def6a768376da6399cfc58959eaccc036903dc73f281bb44f6f4b0bd8c77ad1",
    "chartrender/compare_test.go": "74cde493fdb1aa9e332907e4d62157dd1a8017c97ab793f5bd3c860713ff769a",
    "internal/onboard/charts.go": "8d9cb70a6163d94cc2f435d47405a43a6694a406b4cf89b35fbf9cdbc0136f6e",
    "internal/onboard/check.go": "51d4d1f0dc1845e488cbedac24d8169204f321bda99ba942eec7f4828ede84b3",
    "internal/onboard/check_test.go": "2f02bd128db33e1b56c6e0eb92884a228d56f1a4267a8b6f82c82eade5f70e03",
    "internal/onboard/component.go": "7bdf88c7fe1010811f60ca7c3fddbffa04073f757e48481db0fa83c30f83e2c5",
    "internal/onboard/delivery.go": "a259557743f943cfc16b8f7ad83d810e0d1b2ee29863ef6e6d3ca33ae010b882",
    "internal/onboard/departures.go": "4de1ba8682a5dc7e47dac90fde69be5782528d6d68abaa1cfd14312331dd4b8c",
    "internal/onboard/facts.go": "ede17dd813d766be87f3b7982056181dd0866da37fa034d2b61bb55c850b0bb3",
    "internal/onboard/facts_test.go": "3df665eae94b01c3bff75872e9cdeaba53bbd5bb304c08301a92b79585a4c2b8",
    "internal/onboard/gates.go": "0f0f10f94ab985e32edfad9a8581d7b221e283b87f849677b283f7d4fe87b252",
    "internal/onboard/gates_test.go": "62703026a19bfa9f6428447cc263ce1dca24e01269711f1154765fc01ec29026",
    "internal/onboard/health.go": "c8443c6b8cb2a7c6d58ee77d2dcfadb9d084dc122a4add7c1c6238be7df84add",
    "internal/onboard/health_test.go": "2fc10c0faa5f80f34f2084c34a712616557bd0b1931203580949f5495e64e5ff",
    "internal/onboard/impact.go": "951bf5e2bc9a5cedb861513cc357e863098196d20712c1ef87f5d08227afa22f",
    "internal/onboard/impact_test.go": "0a28c36a20dc568e00df87a62ed2ff4dedfeefa5f5d594e5c9bd9c799e979cff",
    "internal/onboard/live.go": "c99a928e747b905aacd0caae5439cd5742fbcea9ddbb55487b335be5d9523f52",
    "internal/onboard/management_test.go": "2a2505256173ffe4694f8f06dff753de2e5cf0b6edd1f6643f5f63fe68c74a90",
    "internal/onboard/onboard_test.go": "137a6066d9b5ba9978acb9df7b2458d29da399fbbc64e61749a52dae322c56d9",
    "internal/onboard/plan.go": "9d695998c673e2baf1759f73b2e28a68d9829281e4c09b1db89475191c8a293b",
    "internal/onboard/policyset.go": "14d334c60d3b8dbff93971b8f4b4375ea07846a9469aacbd4d0629e89e33dcfc",
    "internal/onboard/render.go": "bfc7052fd56a9002a8952f8be5e4fac0914a2de3c1f3c57b6fd805f7d4a655b4",
    "internal/onboard/status.go": "1a20679c5302c7fa7b54a0fab2da8017fd85ffc7ef796f8b031ef98bfbc4c32a",
    "internal/onboard/status_test.go": "e2e7178602233ff161513697b3e4aab0e376789898589956d9e6e915b87b52d5",
    "internal/onboard/testdata/renders/ingress-nginx-4-15-1-aac043a2.json.gz": "98a816b8a895595c55e8632be629243018b99bc8c7c28bd6f54991e3bf361d5b",
    "internal/onboard/testdata/renders/kyverno-3-8-1-f3ce047f.json.gz": "df1d3c1fb9e343f55382e3edbf0ecd645f2807f862375f04bb1e5614ed73c5a6",
    "internal/onboard/watch.go": "3df439907a4741699028afbd3d9a0254a54ecb45acfae4a82376818593b85d81",
    "internal/onboard/watch_test.go": "aad57dfa4d7d07c99e814ace2921411a17606aa0b0f6a2c475cf70b081be489d",
    "internal/onboard/write.go": "bb196987583530348e7f9bc006527f4be7acdfdf7dbbc728193a3ad23273966b",
    "internal/onboard/yaml.go": "57c8ba0bbaa2b038eea05bf0fff87f4539f2a66f83dc73a7226e2990444a344c",
}
MAX_ARCHIVE_BYTES = 8 * 1024 * 1024
MAX_OUTPUT_BYTES = 4 * 1024 * 1024
DEADLINE_SECONDS = 90
TEST_NAME = "TestHLT04OfflineReplay"
REPLAY_TEST = Path(__file__).parent / "harness" / "hlt04_replay_test.go"

EXPECTED_CASES = {
    "missing-held-report-time": {"write": True, "reason": "first synthetic report"},
    "missing-observed-at-field": {"write": True, "reason": "existing report has no observedAt"},
    "young-held-report": {"write": False, "reason": "unchanged within 10 minute refresh"},
    "old-held-report-renewal": {"write": True, "semantic_status_changed": False},
    "malformed-held-report-time": {"write": True, "reason": "malformed timestamp is not accepted"},
    "future-held-report-time": {"write": False, "reason": "negative age currently passes the refresh comparison"},
    "clock-skew-control": {"revision": "time-inferred only", "digest_proven": False},
    "pre-apply-transition-control": {"lastTransitionTime_is_lastCheckTime": False},
}


class ReplayError(RuntimeError):
    pass


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def source_manifest() -> dict[str, Any]:
    return {
        "repository": "confighub/sveltos-confighub",
        "revision": SOURCE_COMMIT,
        "source_hashes": dict(SOURCE_HASHES),
    }


def replay_command(go_path: str, sandbox_path: str, source_dir: Path) -> list[str]:
    if not Path(go_path).is_absolute() or not Path(sandbox_path).is_absolute():
        raise ReplayError("tool paths must be absolute resolved paths")
    profile = '(version 1) (allow default) (deny network*)'
    return [sandbox_path, "-p", profile, go_path, "test", "-buildvcs=false",
            "./internal/onboard", "-run", f"^{TEST_NAME}$", "-count=1", "-v"]


def compile_command(go_path: str, sandbox_path: str, source_dir: Path) -> list[str]:
    if not Path(go_path).is_absolute() or not Path(sandbox_path).is_absolute():
        raise ReplayError("tool paths must be absolute resolved paths")
    profile = '(version 1) (allow default) (deny network*)'
    return [sandbox_path, "-p", profile, go_path, "test", "-c", "-buildvcs=false",
            "-o", str(source_dir / "hlt04.test"), "./internal/onboard"]


def minimal_go_environment(path: str, private_tmp: str, go_cache: str,
                           module_cache: str) -> dict[str, str]:
    return {
        "PATH": path,
        "TMPDIR": private_tmp,
        "GOCACHE": go_cache,
        "GOMODCACHE": module_cache,
        "GOENV": "off",
        "GOTOOLCHAIN": "local",
        "GOPROXY": "off",
        "GOSUMDB": "off",
        "GONOSUMDB": "*",
        "GONOPROXY": "*",
        "GOFLAGS": "-mod=readonly",
        "LANG": "C",
        "LC_ALL": "C",
    }


def safe_archive_members(archive: tarfile.TarFile) -> list[tarfile.TarInfo]:
    members = archive.getmembers()
    for member in members:
        name = PurePosixPath(member.name)
        if name.is_absolute() or ".." in name.parts:
            raise ReplayError("source archive contains an unsafe path")
        if not (member.isfile() or member.isdir()):
            raise ReplayError("source archive contains a link or special file")
    return members


def parse_result_summaries(stdout: bytes) -> list[dict[str, Any]]:
    marker = b"HLT04_RESULT "
    lines = [line[line.index(marker) + len(marker):].decode("utf-8")
             for line in stdout.splitlines() if marker in line]
    if len(lines) != 2:
        raise ReplayError("expected exactly two structured HLT04_RESULT lines")
    def unique_pairs(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ReplayError(f"duplicate JSON key in replay summary: {key}")
            result[key] = value
        return result
    def reject_constant(value):
        raise ReplayError(f"non-finite JSON number: {value}")
    def finite_float(value):
        number = float(value)
        if not math.isfinite(number):
            reject_constant(value)
        return number
    def decode(value):
        try:
            return json.loads(value, object_pairs_hook=unique_pairs,
                              parse_constant=reject_constant, parse_float=finite_float)
        except (TypeError, ValueError) as exc:
            raise ReplayError(f"invalid replay JSON: {exc}") from exc
    try:
        parsed = [decode(line) for line in lines]
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ReplayError("structured replay result is malformed") from exc
    if any(not isinstance(item, dict) or item.get("schema") != "sveltos-hlt04-offline-replay.v1"
           or item.get("source_kind") != "synthetic-source-contract-replay" for item in parsed):
        raise ReplayError("structured replay result has an unexpected schema")
    all_cases = []
    for item in parsed:
        cases = item.get("cases")
        if not isinstance(cases, list):
            raise ReplayError("case results are missing or not a list")
        all_cases.extend(cases)
    expected = {"missing-held-report-time", "missing-observed-at-field", "young-held-report",
                "old-held-report-renewal", "malformed-held-report-time", "future-held-report-time",
                "clock-skew-control", "pre-apply-transition-control"}
    names = [case.get("name") for case in all_cases if isinstance(case, dict)]
    if (len(names) != len(all_cases) or any(not isinstance(name, str) for name in names)
            or len(names) != len(set(names)) or set(names) != expected):
        raise ReplayError("case result set is missing, duplicated, or unexpected")
    expected_write = {
        "missing-held-report-time": True, "missing-observed-at-field": True,
        "young-held-report": False, "old-held-report-renewal": True,
        "malformed-held-report-time": True, "future-held-report-time": False,
        "clock-skew-control": True, "pre-apply-transition-control": True,
    }
    for case in all_cases:
        if not isinstance(case.get("wrote"), bool) or case["wrote"] != expected_write[case["name"]]:
            raise ReplayError("case result has an unexpected producer write result")
        evidence = case.get("evidence")
        if not isinstance(evidence, dict) or not isinstance(evidence.get("raw_inputs"), dict):
            raise ReplayError("case result omitted raw authored inputs")
        if not isinstance(evidence.get("held_before"), str) or not isinstance(evidence.get("held_after"), str):
            raise ReplayError("case result omitted held annotation before or after")
        if not isinstance(evidence.get("computed_report"), dict) or "write_patch" not in evidence:
            raise ReplayError("case result omitted computed report or exact write patch")
        if case["wrote"] and not isinstance(evidence["write_patch"], str):
            raise ReplayError("writing case did not preserve exact patch bytes")
        if not case["wrote"] and evidence["write_patch"] is not None:
            raise ReplayError("non-writing case unexpectedly preserved a write patch")
        if any(key in case for key in ("held_before", "held_after", "computed_report", "write_patch", "raw_inputs")):
            raise ReplayError("case duplicates its authoritative evidence fields")
        raw = evidence["raw_inputs"]
        required = {"clusterprofiles", "clustersummaries", "clusterhealthchecks", "published_releases"}
        if set(raw) != required or not isinstance(raw["published_releases"], list):
            raise ReplayError("raw authored input inventory is incomplete")
        if any(not isinstance(raw[key], dict) or not isinstance(raw[key].get("items"), list)
               for key in required - {"published_releases"}):
            raise ReplayError("raw authored list inputs are malformed")
        computed = evidence["computed_report"]
        if any(not isinstance(computed.get(key), str) or not computed[key]
               for key in ("source", "observedAt", "healthStatus", "syncStatus")):
            raise ReplayError("computed report is incomplete")
        if evidence.get("synthetic_now") != computed["observedAt"]:
            raise ReplayError("computed report time differs from the injected clock")
        if case["wrote"]:
            patch = decode(evidence["write_patch"])
            if (not isinstance(patch, dict) or not isinstance(patch.get("Annotations"), dict)
                    or patch["Annotations"].get("confighub.com/live-status") != evidence["held_after"]
                    or decode(evidence["held_after"]) != computed):
                raise ReplayError("written patch, held annotation and computed report disagree")
        elif evidence["held_after"] != evidence["held_before"] or not isinstance(decode(evidence["held_after"]), dict):
            raise ReplayError("skipped write changed or omitted the held annotation")
        if case["name"] in {"young-held-report", "future-held-report-time"} and case.get("why") != "unchanged":
            raise ReplayError("skip case lacks the producer's unchanged result")
        if case["name"] == "old-held-report-renewal" and (
                case.get("source_input_hashes_unchanged") is not True or
                case.get("synthetic_check_evidence_consumed_by_reporter") is not False):
            raise ReplayError("renewal case changed source/check evidence claims")
        if case["name"] == "missing-observed-at-field":
            try:
                held = decode(evidence["held_before"])
            except json.JSONDecodeError as exc:
                raise ReplayError("missing-observedAt control has malformed held JSON") from exc
            if not isinstance(held, dict) or "observedAt" in held:
                raise ReplayError("missing-observedAt control does not represent an existing report")
            if held != {key: value for key, value in computed.items() if key != "observedAt"}:
                raise ReplayError("missing-observedAt control changes semantic status as well as time")
    if parsed[1].get("clock_skew_control", {}).get("release_digest_proven") is not False:
        raise ReplayError("clock-skew case overclaims digest proof")
    if parsed[1].get("pre_apply_transition_control", {}).get("last_transition_time_is_check_execution_time") is not False:
        raise ReplayError("transition control overclaims check execution time")
    return parsed


def cleanup_confirmed(record: dict[str, Any]) -> bool:
    return record.get("cleanup_confirmed") is True and record.get("group_gone_confirmed") is True and record.get("parent_reaped") is True


def _write_private(path: Path, data: bytes) -> None:
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW
    fd = os.open(path, flags, 0o600)
    try:
        with os.fdopen(fd, "wb", closefd=False) as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
    finally:
        os.close(fd)


def create_output(path: Path) -> Path:
    if path.exists() or path.is_symlink():
        raise ReplayError("output already exists; refusing overwrite or symlink")
    parent = path.parent.resolve(strict=True)
    out = parent / path.name
    os.mkdir(out, 0o700)
    if stat.S_IMODE(out.stat().st_mode) != 0o700:
        raise ReplayError("cannot create private output directory")
    return out


def export_source(source_root: Path, destination: Path) -> dict[str, Any]:
    git_info = resolved_tool("git")
    git = git_info["path"]
    root = source_root.resolve(strict=True)
    verify = subprocess.run([git, "-C", str(root), "rev-parse", "--verify", f"{SOURCE_COMMIT}^{{commit}}"],
                            stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, timeout=10, check=False)
    if verify.returncode != 0 or verify.stdout.decode().strip() != SOURCE_COMMIT:
        raise ReplayError("pinned source commit is unavailable locally")
    archive_path = destination.parent / "source.tar"
    def cap_archive_file():
        resource.setrlimit(resource.RLIMIT_FSIZE, (MAX_ARCHIVE_BYTES, MAX_ARCHIVE_BYTES))
    with archive_path.open("xb") as archive_stream:
        exported = subprocess.run([git, "-C", str(root), "archive", "--format=tar", SOURCE_COMMIT,
                                   "go.mod", "go.sum", "chartrender", "internal/onboard"],
                                  stdin=subprocess.DEVNULL, stdout=archive_stream,
                                  stderr=subprocess.PIPE, timeout=20, check=False,
                                  preexec_fn=cap_archive_file)
    archive_size = archive_path.stat().st_size
    if exported.returncode != 0:
        raise ReplayError("could not export the exact pinned source archive")
    if archive_size > MAX_ARCHIVE_BYTES:
        archive_path.unlink(missing_ok=True)
        raise ReplayError("pinned source archive exceeds its size bound")
    archive_bytes = archive_path.read_bytes()
    archive_path.unlink()
    destination.mkdir(mode=0o700)
    with tarfile.open(fileobj=__import__("io").BytesIO(archive_bytes), mode="r:") as archive:
        members = safe_archive_members(archive)
        archive.extractall(destination, members=members, filter="data")
    observed = {}
    actual_files = {path.relative_to(destination).as_posix() for path in destination.rglob("*") if path.is_file()}
    if actual_files != set(SOURCE_HASHES):
        raise ReplayError("exported source file inventory differs from the pinned manifest")
    for relative, expected in SOURCE_HASHES.items():
        path = destination / relative
        if path.is_symlink() or not path.is_file():
            raise ReplayError(f"pinned source file missing or linked: {relative}")
        observed[relative] = sha256_file(path)
        if observed[relative] != expected:
            raise ReplayError(f"pinned source hash mismatch: {relative}")
    return {"source_commit": SOURCE_COMMIT, "git_tool": git_info,
            "archive_sha256": sha256_bytes(archive_bytes),
            "source_hashes": observed, "archive_bytes": len(archive_bytes)}


def resolved_tool(binary: str) -> dict[str, str]:
    found = shutil.which(binary)
    if not found:
        raise ReplayError(f"required local tool is unavailable: {binary}")
    path = Path(found).resolve(strict=True)
    info = path.stat()
    if not stat.S_ISREG(info.st_mode):
        raise ReplayError(f"tool is not a regular executable: {binary}")
    return {"path": str(path), "sha256": sha256_file(path)}


def _kill_owned_group(process: subprocess.Popen[bytes], pgid: int) -> dict[str, Any]:
    requested = []
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(pgid, sig)
            requested.append(sig.name)
        except ProcessLookupError:
            break
        except PermissionError:
            break
        deadline = time.monotonic() + (0.25 if sig == signal.SIGTERM else 0.75)
        while time.monotonic() < deadline:
            try:
                os.killpg(pgid, 0)
            except (ProcessLookupError, PermissionError):
                break
            time.sleep(0.02)
    try:
        process.wait(timeout=0.5)
        parent_reaped = True
    except subprocess.TimeoutExpired:
        parent_reaped = False
    try:
        os.killpg(pgid, 0)
        group_gone = False
    except ProcessLookupError:
        group_gone = True
    except PermissionError:
        group_gone = False
    return {"owned_pgid": pgid, "signals_requested": requested,
            "group_gone_confirmed": group_gone, "parent_reaped": parent_reaped,
            "cleanup_confirmed": group_gone and parent_reaped}

def run_owned(argv: list[str], env: dict[str, str], cwd: Path,
              timeout: float = DEADLINE_SECONDS) -> tuple[int | None, bytes, bytes, str, dict[str, Any]]:
    proc = subprocess.Popen(argv, cwd=cwd, env=env, stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            start_new_session=True, close_fds=True)
    pgid = proc.pid
    assert proc.stdout and proc.stderr
    selector = selectors.DefaultSelector()
    selector.register(proc.stdout, selectors.EVENT_READ, "stdout")
    selector.register(proc.stderr, selectors.EVENT_READ, "stderr")
    chunks = {"stdout": bytearray(), "stderr": bytearray()}
    deadline = time.monotonic() + timeout
    status = "running"
    return_code: int | None = None
    cleanup: dict[str, Any] = {"owned_pgid": pgid, "cleanup_confirmed": False}
    try:
        while selector.get_map() or proc.poll() is None:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                status = "timeout"
                break
            for key, _ in selector.select(min(remaining, 0.2)):
                data = os.read(key.fileobj.fileno(), 65536)
                if not data:
                    selector.unregister(key.fileobj)
                    key.fileobj.close()
                    continue
                total = sum(len(value) for value in chunks.values())
                room = MAX_OUTPUT_BYTES - total
                if len(data) > room:
                    chunks[key.data].extend(data[:max(0, room)])
                    status = "output_limit"
                    break
                chunks[key.data].extend(data)
            if status != "running":
                break
        if status == "running":
            return_code = proc.wait(timeout=max(0.01, deadline - time.monotonic()))
            status = "completed"
        else:
            return_code = None
    except BaseException as exc:
        status = "interrupted"
        cleanup["interruption"] = f"{type(exc).__name__}: {str(exc)[:200]}"
        return_code = None
    finally:
        cleanup.update(_kill_owned_group(proc, pgid))
        if not cleanup.get("parent_reaped"):
            return_code = None
        selector.close()
        for stream in (proc.stdout, proc.stderr):
            if stream and not stream.closed:
                stream.close()
    return return_code, bytes(chunks["stdout"]), bytes(chunks["stderr"]), status, {
        **cleanup}


def execute(output: Path, source_root: Path = SOURCE_ROOT) -> dict[str, Any]:
    started = time.time()
    deadline = time.monotonic() + DEADLINE_SECONDS
    root = create_output(output)
    record: dict[str, Any] = {"schema": "sveltos-hlt04-replay-run.v1", "status": "preparing",
                              "started_at_unix": started, "source": source_manifest(),
                              "network_policy": "deny-all sandbox; GOPROXY=off; GOSUMDB=off"}
    original_handlers = {}
    def interrupt(signum, _frame):
        raise ReplayError(f"interrupted by signal {signum}; owned process cleanup requested")
    for sig in (signal.SIGINT, signal.SIGTERM):
        original_handlers[sig] = signal.signal(sig, interrupt)
    try:
        if platform.system() != "Darwin":
            raise ReplayError("network-denying sandbox is only implemented for macOS; refusing run")
        source_info = export_source(source_root, root / "pinned-source")
        harness_bytes = REPLAY_TEST.read_bytes()
        harness_hash = sha256_bytes(harness_bytes)
        harness_target = root / "pinned-source/internal/onboard/hlt04_replay_test.go"
        _write_private(harness_target, harness_bytes)

        go = resolved_tool("go")
        sandbox = resolved_tool("sandbox-exec")
        version = subprocess.run([go["path"], "version"], stdin=subprocess.DEVNULL,
                                 stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                 timeout=5, env={"PATH": os.path.dirname(go["path"]), "GOENV": "off",
                                                 "GOTOOLCHAIN": "local", "GOPROXY": "off",
                                                 "GOSUMDB": "off"}, check=False)
        if version.returncode != 0:
            raise ReplayError("local Go version could not be read")
        module_cache = Path(os.environ.get("GOMODCACHE", str(Path.home() / "go/pkg/mod"))).resolve(strict=True)
        if not module_cache.is_dir():
            raise ReplayError("existing Go module cache is not a directory")
        (root / "go-cache").mkdir(mode=0o700)
        (root / "tmp").mkdir(mode=0o700)
        env = minimal_go_environment(os.path.dirname(go["path"]), str(root / "tmp"),
                                     str(root / "go-cache"), str(module_cache))
        argv = replay_command(go["path"], sandbox["path"], root / "pinned-source")
        record.update({"status": "running", "pinned_source": source_info,
                       "helper_sha256": sha256_file(Path(__file__)),
                       "harness_sha256": harness_hash, "go": {**go, "version": version.stdout.decode().strip()},
                       "sandbox_exec": sandbox, "argv": argv, "environment_keys": sorted(env),
                       "module_cache_path": str(module_cache), "input_kind": "authored synthetic JSON only"})
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise ReplayError(f"overall {DEADLINE_SECONDS}-second deadline expired before replay")
        return_code, stdout, stderr, run_status, cleanup = run_owned(argv, env, root / "pinned-source", remaining)
        _write_private(root / "stdout.bin", stdout)
        _write_private(root / "stderr.bin", stderr)
        record.update({"status": run_status if run_status != "completed" else
                       ("passed" if return_code == 0 else "failed"),
                       "return_code": return_code, "run_status": run_status,
                       "stdout_sha256": sha256_bytes(stdout), "stderr_sha256": sha256_bytes(stderr),
                       "stdout_bytes": len(stdout), "stderr_bytes": len(stderr), "cleanup": cleanup})
        if run_status != "completed" or type(return_code) is not int or return_code != 0:
            raise ReplayError("offline producer replay failed, timed out, or exceeded output bounds")
        if not cleanup_confirmed(cleanup):
            raise ReplayError("owned process group cleanup or parent reaping was not confirmed")
        record["result_summaries"] = parse_result_summaries(stdout)
        record["parse_status"] = "complete"
        record["producer_test_executed"] = True
    except Exception as exc:
        record.update({"status": "failed", "error_type": type(exc).__name__, "error": str(exc)[:500]})
    finally:
        for sig, handler in original_handlers.items():
            signal.signal(sig, handler)
        record["ended_at_unix"] = time.time()
        record["elapsed_seconds"] = max(0.0, record["ended_at_unix"] - started)
        try:
            _write_private(root / "provenance.json", (json.dumps(record, sort_keys=True, indent=2) + "\n").encode())
        except FileExistsError:
            # An earlier failed attempt's private provenance is never overwritten.
            pass
    if record.get("status") != "passed" or record.get("parse_status") != "complete":
        raise ReplayError(f"offline producer replay did not pass; private evidence: {root}")
    return record


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true", help="explicitly run the isolated offline producer test")
    parser.add_argument("--source-root", type=Path, default=SOURCE_ROOT)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    if not args.execute:
        parser.error("replay requires --execute after independent review")
    try:
        record = execute(args.output, args.source_root)
    except ReplayError as exc:
        parser.exit(2, f"replay failed closed: {exc}\n")
    print(json.dumps({"schema": record["schema"], "status": record["status"],
                      "parse_status": record["parse_status"],
                      "output": str(args.output.resolve())}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
