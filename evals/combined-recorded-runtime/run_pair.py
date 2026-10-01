#!/usr/bin/env python3
"""Prepare and run one strictly bounded, synthetic-provider two-arm container pair.

This command performs the actual runtime attempt when explicitly invoked. It is
not wired into CI and has not been invoked by this source-only implementation.
"""
from __future__ import annotations

import argparse, hashlib, importlib.util, json, os, shutil, signal, sys, time, uuid
from datetime import datetime, timezone
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
LINUX_HELPER = REPO / "evals/linux-runtime-versions/prepare.py"
spec = importlib.util.spec_from_file_location("combined_linux_versions", LINUX_HELPER)
if spec is None or spec.loader is None: raise RuntimeError("pinned Linux runtime helper is unavailable")
linux = importlib.util.module_from_spec(spec); spec.loader.exec_module(linux)
contract_spec = importlib.util.spec_from_file_location("combined_runtime_contract", HERE / "contract.py")
if contract_spec is None or contract_spec.loader is None: raise RuntimeError("runtime receipt contract unavailable")
contract = importlib.util.module_from_spec(contract_spec); contract_spec.loader.exec_module(contract)

IMAGE_ID = linux.IMAGE_ID
OWNER_LABEL = linux.OWNER_LABEL
TOTAL_SECONDS = 300
EXECUTION_SECONDS = 120
CLEANUP_SECONDS = 30
DOCKER_OUTPUT_CAP = 32 * 1024 * 1024
ASSETS_DEFAULT = Path("/Users/alexis/code/cub-scout/evals/results/linux-runtime-assets-20261001/assets")
DOCKER_DEFAULT = Path("/usr/local/bin/docker")
RAW_SOURCE = REPO / "evals/fixtures/scale/cluster"
RAW_PINS = {
    "configmaps.yaml": "fc7bce49a84edb5065e4c441bc9936ee906ee976567dbd80bae5fe282f4eb68f",
    "deployments.yaml": "822716e41eaf59674cec8b52913b2613c76932b578c45d54285f71747dd228b6",
    "events.yaml": "3764ff2668f35d131c98ae3cd8acbf40ae06c32888424f4e58de7bf6e0f707bd",
    "namespaces.yaml": "9c02dbb607f9ffd7e232292bfa5fb3fb1167f67b7b4ca9031c14a7cb1993b1f0",
    "pods.yaml": "eb4934b6b747acc370291984172c3b1729ee56eceb7e1d516b2b0eaf29a0d8ae",
    "replicasets.yaml": "e868ab510d43ca66ab3203b6d735ad8edc66a5fb28c497ad48e9c23707f5de68",
    "services.yaml": "c7984a35c6742a8bdf4b5d1b6d32fcfed0f04d649c069a05819fa96e9b8ae57a",
}
ASSET_NAMES = ("claude", "kubectl", "helm", "cub-scout")
PRE01_FILES = {
    "evals/recorded-api/replay.py": "5b147c7bfe409a93d9e20e6d5994e653e3c524e082722068471bd6e7aa7d8d93",
    "evals/reports/2026-10-01-pre01-crd.json": "5585ecd0298e2b6f4201a638cdc0aeab9cdeb0dfab38d3b52e090cc3cb8f5c0a",
    "evals/pre01-crd/fixtures/capture-scope.json": "6d39e83612d52f5124bad2dd63b27b30b94cce492eb70223f759248de71bfec4",
    "evals/pre01-crd/fixtures/absent-crd.json": "661902cf15f640cf0cfbcb7d5df71d36ac2d9ac6041f1b92429e66704c1614c0",
    "evals/pre01-crd/fixtures/absent-discovery.body": "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793",
    "evals/pre01-crd/fixtures/absent-servicemonitor.body": "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793",
    "evals/pre01-crd/fixtures/absent-after-apply-crd.json": "661902cf15f640cf0cfbcb7d5df71d36ac2d9ac6041f1b92429e66704c1614c0",
    "evals/pre01-crd/fixtures/absent-after-apply-servicemonitor.body": "b16e15764b8bc06c5c3f9f19bc8b99fa48e7894aa5a6ccdad65da49bbf564793",
    "evals/pre01-crd/fixtures/present-crd.json": "a5e571a80d5882cc9294cfabc27e0e6cefead3612e8e880afb885e3bf60f3e3c",
    "evals/pre01-crd/fixtures/present-discovery.json": "e791bb13f75f7767dd2c40df7485653342b0894ca58fd3af17b1b03ed087c1a8",
    "evals/pre01-crd/fixtures/present-servicemonitor.json": "78836df757ca3eb79f25bb0190f03d465fb282b455b8a27e79fbef67247a650e",
}


def sha(data: bytes) -> str: return hashlib.sha256(data).hexdigest()
def file_sha(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""): h.update(chunk)
    return h.hexdigest()
def now(): return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def private_output(path: Path) -> Path:
    p = path.expanduser().absolute()
    if p.is_symlink() or p.exists(): raise ValueError("output must be a fresh non-symlink directory")
    parent = p.parent.resolve(strict=True)
    temp_root = Path("/tmp").resolve(strict=True)
    if parent != temp_root and temp_root not in parent.parents:
        raise ValueError("output must be under /tmp")
    p.mkdir(mode=0o700); os.chmod(p, 0o700)
    return p


def write_new(path: Path, data: bytes, mode=0o600):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    with os.fdopen(fd, "wb") as stream: stream.write(data)


def _copy_pinned(src: Path, dst: Path, expected: str, mode=0o444):
    if src.is_symlink() or not src.is_file() or file_sha(src) != expected:
        raise ValueError(f"source pin mismatch: {src.name}")
    dst.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    shutil.copyfile(src, dst, follow_symlinks=False); os.chmod(dst, mode)
    return {"bytes": dst.stat().st_size, "sha256": file_sha(dst)}


def stage_common(stage: Path, assets: Path, *, include_scout: bool):
    if stage.exists():
        if stage.is_symlink() or not stage.is_dir() or any(stage.iterdir()):
            raise ValueError("staging directory must be empty and owned by this run")
    else:
        stage.mkdir(mode=0o700)
    # Docker cannot create a nested mountpoint beneath a readonly parent bind.
    if include_scout:
        (stage / "plugin").mkdir(mode=0o555)
    evidence = stage / "evidence"; evidence.mkdir(mode=0o700)
    pins = {}
    for filename, digest in RAW_PINS.items():
        pins[filename] = _copy_pinned(RAW_SOURCE / filename, evidence / filename, digest)
    for rel, digest in PRE01_FILES.items():
        dst = stage / rel
        pins[rel] = _copy_pinned(REPO / rel, dst, digest, 0o444)
    scale_helper = REPO / "evals/recorded-scale/preflight.py"
    # Existing preflight owns the full 302/300/excluded/identity parser.
    pins["evals/recorded-scale/preflight.py"] = {"bytes": scale_helper.stat().st_size, "sha256": file_sha(scale_helper)}
    scale_dest = stage / "evals/recorded-scale/preflight.py"
    scale_dest.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    shutil.copyfile(scale_helper, scale_dest); os.chmod(scale_dest, 0o444)
    direct = REPO / "evals/direct-cli-controls/probe.py"
    pins["evals/direct-cli-controls/probe.py"] = {"bytes": direct.stat().st_size, "sha256": file_sha(direct)}
    direct_dest = stage / "evals/direct-cli-controls/probe.py"
    direct_dest.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    shutil.copyfile(direct, direct_dest); os.chmod(direct_dest, 0o444)
    payload = HERE / "payload.py"
    pins["payload.py"] = {"bytes": payload.stat().st_size, "sha256": file_sha(payload)}
    shutil.copyfile(payload, stage / "payload.py"); os.chmod(stage / "payload.py", 0o444)
    for name in ASSET_NAMES:
        if name == "cub-scout" and not include_scout:
            continue
        facts = linux.validate_asset(assets / name, name)
        dst = stage / ("cub-scout" if name == "cub-scout" else name)
        shutil.copyfile(assets / name, dst); os.chmod(dst, 0o555)
        if file_sha(dst) != facts["sha256"]: raise ValueError("staged runtime asset changed during copy")
        pins["bin/" + name] = facts
    pins["_stageFiles"] = {p.relative_to(stage).as_posix(): {"bytes": p.stat().st_size, "sha256": file_sha(p)}
                           for p in sorted(stage.rglob("*")) if p.is_file()}
    # The same seven ordinary evidence bytes are available from identical paths
    # in both arms; the PRE-01 source/fixture tree is also identical.
    for directory in sorted((p for p in stage.rglob("*") if p.is_dir()), reverse=True): os.chmod(directory, 0o555)
    os.chmod(stage, 0o555)
    return pins


def stage_treatment_plugin(path: Path, base: Path):
    path.mkdir(mode=0o700)
    meta = REPO / ".claude-plugin/plugin.json"
    manifest = json.loads(meta.read_text(encoding="utf-8"))
    # Do not inherit the live default server. MCP is configured explicitly to
    # the pinned recording in this packet.
    manifest["mcpServers"] = {}
    pmeta = path / ".claude-plugin"; pmeta.mkdir(mode=0o700)
    write_new(pmeta / "plugin.json", (json.dumps(manifest, indent=2) + "\n").encode(), 0o444)
    wrapper = path / "bin/recorded-cub-scout"; wrapper.parent.mkdir(mode=0o700)
    wrapper_text = ("#!/bin/sh\nset -eu\n"
        "exec /usr/bin/env -i PATH=/tools:/usr/bin:/bin HOME=/tmp/private-home "
        "KUBECONFIG=/tmp/empty-kubeconfig /tools/cub-scout mcp serve --recording "
        "/tools/evidence/deployments.yaml\n")
    write_new(wrapper, wrapper_text.encode(), 0o555)
    skill_root = path / "skills"; skill_root.mkdir(mode=0o700)
    skill_paths = sorted(p.parent for p in (REPO / "skills").rglob("SKILL.md"))
    if len(skill_paths) != 35: raise ValueError("reviewed 35-skill source inventory changed")
    rows = []
    for src_dir in skill_paths:
        relative = src_dir.relative_to(REPO / "skills")
        dst_dir = skill_root / relative
        shutil.copytree(src_dir, dst_dir, symlinks=False)
        for item in dst_dir.rglob("*"):
            if item.is_file(): os.chmod(item, 0o444)
        for item in sorted(dst_dir.rglob("*"), reverse=True):
            if item.is_dir(): os.chmod(item, 0o555)
        os.chmod(dst_dir, 0o555)
        for item in sorted(x for x in dst_dir.rglob("*") if x.is_file()):
            rows.append({"path": item.relative_to(skill_root).as_posix(), "bytes": item.stat().st_size, "sha256": file_sha(item)})
    skill_digest = sha(json.dumps(rows, sort_keys=True, separators=(",", ":")).encode())
    if skill_digest != "9f31fd677a3490f60a6d1c97b315f8f3d7ee7db6dd5285b47051e049d3cc8b8b":
        raise ValueError("35-skill tree differs from the v1 source pin")
    for d in sorted((p for p in path.rglob("*") if p.is_dir()), reverse=True): os.chmod(d, 0o555)
    os.chmod(path, 0o555)
    return {"skillCount": len(skill_paths), "skillFiles": rows, "skillTreeSha256": skill_digest,
            "recordedMcpWrapperSha256": file_sha(wrapper),
            "pluginJsonSha256": file_sha(pmeta / "plugin.json")}


def custom_create_args(name: str, owner: str, stage: Path, *, plugin: Path | None):
    if not name.startswith("scout-combined-") or len(owner) != 32:
        raise ValueError("container identity malformed")
    args = ["container", "create", "--name", name, "--label", OWNER_LABEL + "=" + owner,
        "--pull=never", "--network=none", "--read-only", "--user", "65534:65534",
        "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=1g", "--cpus=1",
        "--tmpfs", "/tmp:rw,nosuid,nodev,size=64m,uid=65534,gid=65534",
        "--mount", "type=bind,src=" + str(stage.resolve()) + ",dst=/tools,readonly"]
    if plugin is not None:
        args += ["--mount", "type=bind,src=" + str(plugin.resolve()) + ",dst=/tools/plugin,readonly"]
    args += [IMAGE_ID, "/usr/bin/env", "-i", "PATH=/tools:/usr/bin:/bin", "HOME=/tmp/private-home",
             "LANG=C.UTF-8", "PYTHONDONTWRITEBYTECODE=1", "/usr/local/bin/python3", "/tools/payload.py",
             "treatment" if plugin else "baseline"]
    return args


def inspect_runtime(raw: bytes, name: str, owner: str, stage: Path, plugin: Path | None, *, finished=False):
    obj = linux._one(raw); ident = linux.validate_identity(obj, name, owner)
    config, host = obj.get("Config"), obj.get("HostConfig")
    if not isinstance(config, dict) or not isinstance(host, dict): raise ValueError("container config malformed")
    mounts = obj.get("Mounts")
    expected = [(stage.resolve(), "/tools")]
    if plugin is not None: expected.append((plugin.resolve(), "/tools/plugin"))
    if not isinstance(mounts, list) or any(not isinstance(m, dict) for m in mounts):
        raise ValueError("container mount list malformed")
    binds = [m for m in mounts if m.get("Type") == "bind"]
    tmpfs_mounts = [m for m in mounts if m.get("Type") == "tmpfs"]
    if (any(m.get("Type") not in ("bind", "tmpfs") for m in mounts)
        or len(tmpfs_mounts) > 1
        or any(m.get("Destination") != "/tmp" or m.get("RW") is not True for m in tmpfs_mounts)):
        raise ValueError("unexpected container mount or tmpfs")
    actual = [(Path(m["Source"]).resolve(), m.get("Destination")) for m in binds]
    if (config.get("Image") != IMAGE_ID or obj.get("Image") != IMAGE_ID or config.get("User") != "65534:65534"
        or host.get("ReadonlyRootfs") is not True or host.get("NetworkMode") != "none" or host.get("CapDrop") != ["ALL"]
        or host.get("CapAdd") not in (None, []) or host.get("PidMode") not in (None, "", "private")
        or host.get("IpcMode") not in (None, "", "private")
        or host.get("SecurityOpt") not in (["no-new-privileges"], ["no-new-privileges:true"])
        or host.get("PidsLimit") != 64 or host.get("Memory") != 1073741824 or host.get("NanoCpus") != 1000000000
        or host.get("Privileged") is not False or len(actual) != len(expected) or set(actual) != set(expected)
        or any(m.get("RW") is not False for m in binds)):
        raise ValueError("owned container differs from exact runtime/isolation profile")
    tmp = host.get("Tmpfs")
    if not isinstance(tmp, dict) or tmp.get("/tmp") not in ("rw,nosuid,nodev,size=67108864,uid=65534,gid=65534", "rw,nosuid,nodev,size=64m,uid=65534,gid=65534"):
        raise ValueError("private /tmp size or flags differ")
    if finished:
        state = obj.get("State")
        if not isinstance(state, dict) or state.get("Status") != "exited" or type(state.get("ExitCode")) is not int:
            raise ValueError("container did not report a terminal exit")
    return ident


def run_arm(docker, context, env, stage, plugin, output, deadline, run):
    arm = "treatment" if plugin else "baseline"
    owner = uuid.uuid4().hex; name = "scout-combined-" + uuid.uuid4().hex[:16]
    result = {"arm": arm, "status": "failed", "containerName": name, "ownerLabel": owner,
              "created": False, "securityVerified": False, "cleanup": {"attempted": False, "verifiedAbsent": False},
              "startedAt": now(), "operations": []}
    known_id = ""; started = time.monotonic(); arm_deadline = min(deadline, started + EXECUTION_SECONDS + CLEANUP_SECONDS)
    write_new(output / (arm + ".identity.json"), (json.dumps({"name": name, "ownerLabel": OWNER_LABEL, "owner": owner}) + "\n").encode())
    operation_number = 0
    def call(args, sec, max_output=DOCKER_OUTPUT_CAP):
        nonlocal operation_number
        remaining = min(arm_deadline, deadline) - CLEANUP_SECONDS - time.monotonic()
        if remaining <= 0: raise TimeoutError("pair or arm deadline expired")
        t0 = time.monotonic(); code, out, err = run([str(docker), "--context", context, *args], min(sec, remaining), env=env, max_output=max_output)
        operation_number += 1
        write_new(output / f"{arm}-docker-op-{operation_number:02d}-stdout.bin", out)
        write_new(output / f"{arm}-docker-op-{operation_number:02d}-stderr.bin", err)
        result["operations"].append({"argv": args, "exitCode": code, "stdoutBytes": len(out), "stdoutSha256": sha(out),
                                     "stderrBytes": len(err), "stderrSha256": sha(err), "elapsedSeconds": round(time.monotonic()-t0, 4)})
        return code, out, err
    try:
        remaining = arm_deadline-time.monotonic()
        if remaining <= 0: raise TimeoutError("arm deadline expired")
        code, out, err = call(custom_create_args(name, owner, stage, plugin=plugin), min(30, remaining))
        if code != 0: raise RuntimeError("container create failed")
        try: known_id = out.decode("ascii", "strict").strip()
        except UnicodeError: raise RuntimeError("container ID malformed") from None
        if len(known_id) != 64: raise RuntimeError("container ID malformed")
        result["created"] = True; result["containerId"] = known_id
        code, out, err = call(["container", "inspect", "--format", "{{json .}}", known_id], 10)
        if code != 0: raise RuntimeError("container inspect failed")
        inspect_runtime(out, name, owner, stage, plugin); result["securityVerified"] = True
        code, out, err = call(["container", "start", known_id], 10)
        if code != 0: raise RuntimeError("container start failed")
        code, wait_out, wait_err = call(["container", "wait", known_id], EXECUTION_SECONDS)
        if code != 0: raise RuntimeError("container wait failed")
        code, inspected, err = call(["container", "inspect", "--format", "{{json .}}", known_id], 10)
        if code != 0: raise RuntimeError("final container inspect failed")
        inspect_runtime(inspected, name, owner, stage, plugin, finished=True)
        code, logs, err = call(["container", "logs", known_id], 25)
        if code != 0 or len(logs) > DOCKER_OUTPUT_CAP: raise RuntimeError("container payload output unavailable or oversized")
        # Payload emits one compact JSON record and no other stdout. Preserve
        # even failed payload records; never convert nonzero/timeout into pass.
        payload = json.loads(logs.decode("utf-8", "strict").strip())
        result["payloadExitCode"] = json.loads(wait_out.decode("ascii", "strict").strip().splitlines()[-1])
        output_file = output / (arm + ".payload.json")
        write_new(output_file, (json.dumps(payload, sort_keys=True, indent=2) + "\n").encode())
        result["payloadFile"] = output_file.name
        result["payloadSha256"] = file_sha(output_file); result["payloadBytes"] = output_file.stat().st_size
        result["validatedFacts"] = contract.validate_arm_payload(payload, arm)
        result["status"] = "passed" if payload.get("status") == "passed" and result["payloadExitCode"] == 0 else "failed"
    except BaseException as exc:
        result["status"] = "failed"
        result["error"] = type(exc).__name__ if not isinstance(exc, (RuntimeError, ValueError, TimeoutError)) else str(exc)[:200]
        if isinstance(exc, (KeyboardInterrupt, SystemExit)):
            raise
    finally:
        cleanup = linux.cleanup(docker, context, name, owner, env,
            min(deadline, arm_deadline), run, known_id)
        result["cleanup"] = cleanup
        result["endedAt"] = now(); result["elapsedSeconds"] = round(time.monotonic()-started, 4)
        if not cleanup.get("verifiedAbsent"): result["status"] = "failed"
    return result


def run_pair(assets: Path, docker_path: Path, context: str, output: Path, *, runner=None):
    begin = time.monotonic(); started_at = now(); deadline = begin + TOTAL_SECONDS
    system = __import__("platform").system()
    if system != "Darwin": raise ValueError("host preflight supports only the reviewed macOS Docker context")
    docker, docker_target = linux.docker_launcher(docker_path)
    linux.validate_context(context)
    if assets.is_symlink() or not assets.is_dir(): raise ValueError("pinned asset archive unavailable or unsafe")
    for name in ASSET_NAMES: linux.validate_asset(assets / name, name)
    env = linux.docker_env()
    preflight = linux._inv04.run_bounded([str(docker), "context", "inspect", "--format",
        "{{json .Endpoints.docker.Host}}", context], min(10, max(0.1, deadline-time.monotonic())),
        env=env, max_output=64 * 1024)
    if type(preflight[0]) is not int or preflight[0] != 0:
        raise ValueError("Docker context inspect failed")
    docker_endpoint = linux.validate_local_endpoint(preflight[1])
    out = private_output(output)
    command_runner = linux._inv04.run_bounded if runner is None else runner
    # The existing checked helper accepts one readonly /tools stage. Keep
    # separate complete arm stages so baseline cannot inspect treatment assets.
    stages, plugins, staged = {}, {}, {}
    arms = {"baseline": {"status": "not-started"}, "treatment": {"status": "not-started"}}
    previous = {sig: signal.getsignal(sig) for sig in (signal.SIGINT, signal.SIGTERM)}
    def interrupted(signum, _frame): raise KeyboardInterrupt(signal.Signals(signum).name)
    for sig in previous: signal.signal(sig, interrupted)
    try:
        for arm in ("baseline", "treatment"):
            if time.monotonic() >= deadline: raise TimeoutError("pair wall deadline expired before staging")
            stage = out / (arm + "-tools"); stage.mkdir(mode=0o700)
            # Build each arm independently: the baseline has no Scout binary,
            # plugin, skill files, or MCP executable/recording wrapper.
            commonpins = stage_common(stage, assets, include_scout=(arm == "treatment"))
            plugin = None
            if arm == "baseline":
                pass
            else:
                plugin = out / "treatment-plugin"
                skillfacts = stage_treatment_plugin(plugin, stage)
                stage.chmod(0o555)
            stages[arm] = stage; plugins[arm] = plugin; staged[arm] = {"common": commonpins, "skills": skillfacts if plugin else None}
            plugin_before = {p.relative_to(plugin).as_posix(): file_sha(p) for p in sorted(plugin.rglob("*")) if p.is_file()} if plugin else {}
            arm_result = run_arm(docker, context, env, stage, plugin, out, deadline, command_runner)
            plugin_after = {p.relative_to(plugin).as_posix(): file_sha(p) for p in sorted(plugin.rglob("*")) if p.is_file()} if plugin else {}
            arm_result["pluginIntegrity"] = {"before": plugin_before, "after": plugin_after, "unchanged": plugin_before == plugin_after}
            if plugin_before != plugin_after:
                arm_result["status"] = "failed"
                arm_result["error"] = "treatment plugin changed during execution"
            arm_result["stageInventory"] = {"files": sorted(p.relative_to(stage).as_posix() for p in stage.rglob("*") if p.is_file()),
                "sha256": sha(json.dumps({p.relative_to(stage).as_posix(): file_sha(p) for p in sorted(stage.rglob("*")) if p.is_file()}, sort_keys=True, separators=(",", ":")).encode())}
            arm_result["stageFacts"] = staged[arm]
            arms[arm] = arm_result
            # Preserve both arms even if the first arm fails; never retry either.
    except BaseException as exc:
        arms.setdefault("runner", {"status": "failed", "error": type(exc).__name__})
    finally:
        for sig, handler in previous.items(): signal.signal(sig, handler)
    pair_facts = None
    if set(arms) == {"baseline", "treatment"} and all(x.get("status") == "passed" for x in arms.values()):
        try:
            pair_facts = contract.validate_pair_staging(arms)
        except contract.ContractError as exc:
            arms["runner"] = {"status": "failed", "error": str(exc)}
    elapsed = round(time.monotonic()-begin, 4)
    passed = pair_facts is not None and set(arms) == {"baseline", "treatment"} and elapsed <= TOTAL_SECONDS
    receipt = {"schema": "combined-recorded-runtime-run.v1", "status": "passed" if passed else "failed",
        "startedAt": started_at, "elapsedSeconds": elapsed, "totalSecondsLimit": TOTAL_SECONDS,
        "host": {"platform": system, "dockerContext": context, "dockerEndpoint": docker_endpoint, "dockerLauncher": str(docker),
            "dockerResolvedPath": str(docker_target), "dockerSha256": file_sha(docker_target)},
        "imageId": IMAGE_ID, "sourceHelperSha256": file_sha(Path(__file__)),
        "arms": arms, "pairChecks": pair_facts, "billing": "unknown-unmeasured; synthetic provider", "osProcessInventory": "not-claimed"}
    write_new(out / "run.json", (json.dumps(receipt, sort_keys=True, indent=2) + "\n").encode())
    return out, receipt


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--assets", type=Path, default=ASSETS_DEFAULT)
    parser.add_argument("--docker", type=Path, default=DOCKER_DEFAULT)
    parser.add_argument("--context", required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args(argv)
    try:
        out, receipt = run_pair(args.assets, args.docker, args.context, args.out)
        print(json.dumps({"output": str(out), "status": receipt["status"], "arms": sorted(receipt["arms"])}))
        return 0 if receipt["status"] == "passed" else 1
    except BaseException as exc:
        print(json.dumps({"status": "failed", "error": type(exc).__name__ if isinstance(exc, KeyboardInterrupt) else str(exc)[:200]}), file=sys.stderr)
        return 2


if __name__ == "__main__": raise SystemExit(main())
