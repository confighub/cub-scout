#!/usr/bin/env python3
"""Prepare or run the pinned, local-only HLT-04 producer replay."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import platform
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
    "internal/onboard/status.go": "1a20679c5302c7fa7b54a0fab2da8017fd85ffc7ef796f8b031ef98bfbc4c32a",
    "internal/onboard/status_test.go": "e2e7178602233ff161513697b3e4aab0e376789898589956d9e6e915b87b52d5",
}
MAX_ARCHIVE_BYTES = 8 * 1024 * 1024
MAX_OUTPUT_BYTES = 4 * 1024 * 1024
DEADLINE_SECONDS = 120
TEST_NAME = "TestHLT04OfflineReplay"
REPLAY_TEST = Path(__file__).parent / "harness" / "hlt04_replay_test.go"

EXPECTED_CASES = {
    "missing-held-report-time": {"write": True, "reason": "first synthetic report"},
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
    try:
        parsed = [json.loads(line) for line in lines]
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ReplayError("structured replay result is malformed") from exc
    if any(not isinstance(item, dict) or item.get("schema") != "sveltos-hlt04-offline-replay.v1"
           for item in parsed):
        raise ReplayError("structured replay result has an unexpected schema")
    return parsed


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
    exported = subprocess.run([git, "-C", str(root), "archive", "--format=tar", SOURCE_COMMIT,
                               "go.mod", "go.sum", "internal/onboard"],
                              stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE, timeout=20, check=False)
    if exported.returncode != 0:
        raise ReplayError("could not export the exact pinned source archive")
    if len(exported.stdout) > MAX_ARCHIVE_BYTES:
        raise ReplayError("pinned source archive exceeds its size bound")
    destination.mkdir(mode=0o700)
    with tarfile.open(fileobj=__import__("io").BytesIO(exported.stdout), mode="r:") as archive:
        members = safe_archive_members(archive)
        archive.extractall(destination, members=members, filter="data")
    observed = {}
    for relative, expected in SOURCE_HASHES.items():
        path = destination / relative
        if path.is_symlink() or not path.is_file():
            raise ReplayError(f"pinned source file missing or linked: {relative}")
        observed[relative] = sha256_file(path)
        if observed[relative] != expected:
            raise ReplayError(f"pinned source hash mismatch: {relative}")
    return {"source_commit": SOURCE_COMMIT, "git_tool": git_info,
            "archive_sha256": sha256_bytes(exported.stdout),
            "source_hashes": observed, "archive_bytes": len(exported.stdout)}


def resolved_tool(binary: str) -> dict[str, str]:
    found = shutil.which(binary)
    if not found:
        raise ReplayError(f"required local tool is unavailable: {binary}")
    path = Path(found).resolve(strict=True)
    info = path.stat()
    if not stat.S_ISREG(info.st_mode):
        raise ReplayError(f"tool is not a regular executable: {binary}")
    return {"path": str(path), "sha256": sha256_file(path)}


def _kill_owned_group(process: subprocess.Popen[bytes], pgid: int) -> None:
    try:
        os.killpg(pgid, signal.SIGTERM)
    except (ProcessLookupError, PermissionError):
        return
    deadline = time.monotonic() + 0.35
    while time.monotonic() < deadline and process.poll() is None:
        time.sleep(0.02)
    try:
        os.killpg(pgid, signal.SIGKILL)
    except (ProcessLookupError, PermissionError):
        pass


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
    cleanup = {"owned_pgid": pgid, "process_group_cleanup": "pending"}
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
    finally:
        _kill_owned_group(proc, pgid)
        cleanup["process_group_cleanup"] = "TERM_then_KILL_requested"
        try:
            proc.wait(timeout=1)
        except subprocess.TimeoutExpired:
            return_code = None
            cleanup["parent_reaped"] = False
        else:
            cleanup["parent_reaped"] = True
        selector.close()
    return return_code, bytes(chunks["stdout"]), bytes(chunks["stderr"]), status, {
        **cleanup}


def execute(output: Path, source_root: Path = SOURCE_ROOT) -> dict[str, Any]:
    started = time.time()
    root = create_output(output)
    record: dict[str, Any] = {"schema": "sveltos-hlt04-replay-run.v1", "status": "preparing",
                              "started_at_unix": started, "source": source_manifest(),
                              "network_policy": "deny-all sandbox; GOPROXY=off; GOSUMDB=off"}
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
        return_code, stdout, stderr, run_status, cleanup = run_owned(argv, env, root / "pinned-source")
        _write_private(root / "stdout.bin", stdout)
        _write_private(root / "stderr.bin", stderr)
        record.update({"status": run_status if run_status != "completed" else
                       ("passed" if return_code == 0 else "failed"),
                       "return_code": return_code, "run_status": run_status,
                       "stdout_sha256": sha256_bytes(stdout), "stderr_sha256": sha256_bytes(stderr),
                       "stdout_bytes": len(stdout), "stderr_bytes": len(stderr), "cleanup": cleanup})
        record["result_summaries"] = parse_result_summaries(stdout)
        record["parse_status"] = "complete"
    except Exception as exc:
        record.update({"status": "failed", "error_type": type(exc).__name__, "error": str(exc)[:500]})
    finally:
        record["ended_at_unix"] = time.time()
        _write_private(root / "provenance.json", (json.dumps(record, sort_keys=True, indent=2) + "\n").encode())
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
