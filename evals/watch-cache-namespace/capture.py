#!/usr/bin/env python3
"""Prepare an opt-in bounded before/after #735 proof on one fresh local kind cluster.

This helper is source-only until invoked with --execute. It never selects or
falls back to the user's current Kubernetes context; the Go probe receives only
a temporary private observer kubeconfig generated for the owned cluster.
"""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import sys
import tempfile
import time
import uuid
from datetime import datetime, timezone

# The shared runner helper is imported from source, not installed as a package.
# Keep this opt-in harness from leaving bytecode in the repository when run.
sys.dont_write_bytecode = True

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
INV04_PATH = REPO / "evals/inv04-rbac/capture.py"
_INV04_SPEC = importlib.util.spec_from_file_location("watch_namespace_inv04", INV04_PATH)
assert _INV04_SPEC and _INV04_SPEC.loader
inv04 = importlib.util.module_from_spec(_INV04_SPEC)
_INV04_SPEC.loader.exec_module(inv04)

OLD_SOURCE = "d7f081e88e1841a9557e086a8c0b479c8180008d"
NEW_SOURCE = "7611908afeb361b15a03d2687ccdaff6c504a811"
GO_PROBE = HERE / "observation_watch_live_test.go"
SCHEMA = "watch-cache-namespace-owned-proof.v1"
EXPECTED_OLD_MISMATCHES = {
    "team-b-fallback-and-identity",
    "all-namespace-fallback-and-denial",
    "denied-namespace-fallback-preserves-denial",
    "cluster-scope-fallback-and-error",
}
EXPECTED_NEW_CHECKS = {
    "sameNameDifferentUID", "teamAExactCacheHit", "teamBFallbackAndIdentity",
    "allNamespaceFallbackPreservesDenial", "deniedNamespaceFallbackPreservesDenial",
    "clusterScopeFallbackPreservesError",
}
MAX_OUTPUT = 2 * 1024 * 1024
MAX_SECONDS = 600
CLEANUP_SECONDS = 75


class CaptureError(RuntimeError):
    pass


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    return sha256(path.read_bytes())


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def _write(path: Path, data: bytes, mode: int = 0o600) -> None:
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    with os.fdopen(fd, "wb") as stream:
        stream.write(data)
        stream.flush()
        os.fsync(stream.fileno())


def _fresh_output(path: Path) -> Path:
    requested = path.expanduser().absolute()
    if requested.exists() or requested.is_symlink() or ".." in path.parts:
        raise CaptureError("output directory must be a fresh non-symlink path")
    resolved = requested.resolve()
    if resolved == REPO or REPO in resolved.parents or Path.home() == resolved or Path.home() in resolved.parents:
        raise CaptureError("output directory must be outside the source checkout and home directory")
    if not any(resolved == Path(root).resolve() or Path(root).resolve() in resolved.parents
               for root in (tempfile.gettempdir(), "/tmp", "/var/tmp")):
        raise CaptureError("output directory must be under a temporary directory")
    requested.mkdir(parents=True, mode=0o700)
    os.chmod(requested, 0o700)
    return requested


def _run(args: list[str], timeout: float, env: dict[str, str] | None = None,
         max_output: int = MAX_OUTPUT, *, deadline: float | None = None) -> tuple[int, bytes, bytes]:
    if deadline is not None:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise CaptureError("overall proof deadline expired")
        timeout = min(timeout, remaining)
    try:
        return inv04.run_bounded(args, timeout, env=env, max_output=max_output)
    except inv04.CaptureError as error:
        raise CaptureError(str(error)) from None


def _required_command(name: str) -> str:
    path = shutil.which(name)
    if not path:
        raise CaptureError("required local executable not found: " + name)
    resolved = Path(path).resolve(strict=True)
    if not resolved.is_file() or not os.access(resolved, os.X_OK):
        raise CaptureError("required executable is not a regular executable: " + name)
    return str(resolved)


def _owned_cluster_name() -> str:
    name = "scout-watch-ns-" + datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S") + "-" + uuid.uuid4().hex[:8]
    if len(name + "-control-plane") > 63 or not re.fullmatch(r"scout-watch-ns-[a-z0-9-]+", name):
        raise CaptureError("generated owned kind name is invalid")
    return name


def _fixture_manifests(namespaces: tuple[str, str, str], cm_name: str) -> bytes:
    a, b, denied = namespaces
    docs = [{"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": ns}}
            for ns in namespaces]
    docs.append({"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": "watch-probe", "namespace": a}})
    for ns in (a, b):
        docs.extend([
            {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
             "metadata": {"name": "watch-probe-reader", "namespace": ns},
             "rules": [{"apiGroups": [""], "resources": ["configmaps"], "verbs": ["get", "list", "watch"]}]},
            {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
             "metadata": {"name": "watch-probe-reader", "namespace": ns},
             "subjects": [{"kind": "ServiceAccount", "name": "watch-probe", "namespace": a}],
             "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "watch-probe-reader"}},
            {"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": cm_name, "namespace": ns},
             "data": {"fixture": "watch-cache-namespace"}},
        ])
    return ("\n---\n".join(json.dumps(doc, sort_keys=True, separators=(",", ":")) for doc in docs) + "\n").encode()


def _observer_config(server: str, ca_data: str, token: str, context_name: str, namespace: str) -> bytes:
    if not token or not ca_data or not server.startswith("https://"):
        raise CaptureError("private observer credentials are incomplete")
    import urllib.parse
    parsed = urllib.parse.urlparse(server)
    if parsed.hostname not in ("127.0.0.1", "localhost", "::1") or parsed.username or parsed.password:
        raise CaptureError("refusing a non-loopback or credential-bearing Kubernetes endpoint")
    data = {
        "apiVersion": "v1", "kind": "Config", "current-context": context_name,
        "clusters": [{"name": "owned", "cluster": {"server": server, "certificate-authority-data": ca_data}}],
        "users": [{"name": "observer", "user": {"token": token}}],
        "contexts": [{"name": context_name, "context": {"cluster": "owned", "user": "observer", "namespace": namespace}}],
    }
    return (json.dumps(data, sort_keys=True, separators=(",", ":")) + "\n").encode()


def _old_adapter(probe: bytes) -> bytes:
    text = probe.decode("utf-8")
    current = "[]schema.GroupVersionResource{configMapGVR},\n\t\t\tmap[schema.GroupVersionResource]resourceScope{configMapGVR: resourceScopeNamespaced}, namespaceA)"
    if text.count(current) != 1:
        raise CaptureError("Go probe does not contain the reviewed new constructor signature exactly once")
    text = text.replace(current, "[]schema.GroupVersionResource{configMapGVR}, namespaceA)")
    cluster_scope = "\t\t\twatchClient.scopes[clusterNodeGVR] = resourceScopeCluster\n"
    if text.count(cluster_scope) != 1:
        raise CaptureError("Go probe does not contain exactly one scope adapter line")
    text = text.replace(cluster_scope, "")
    if "resourceScope" in text:
        raise CaptureError("old-source probe adaptation left a new-only scope reference")
    return text.encode()


def validate_probe_result(variant: str, exit_code: int, result: dict,
                          expected_fixture_uids: dict[str, str] | None = None) -> dict:
    if (not isinstance(result, dict) or result.get("schema") != "watch-cache-namespace-live-probe.v1"
            or result.get("variant") != variant):
        raise CaptureError(variant + " live probe result is absent or has an unknown schema")
    mismatches = result.get("mismatches")
    checks = result.get("checks")
    if not isinstance(mismatches, list) or any(not isinstance(item, str) for item in mismatches):
        raise CaptureError(variant + " live probe mismatch list is malformed")
    if not isinstance(checks, dict) or any(type(value) is not bool for value in checks.values()):
        raise CaptureError(variant + " live probe checks are malformed")
    for label in ("directRequests", "watchRequests"):
        if label not in result or not isinstance(result[label], list):
            raise CaptureError(variant + " probe omitted HTTP request evidence")
        for request in result[label]:
            if (not isinstance(request, dict) or request.get("method") != "GET"
                    or not isinstance(request.get("path"), str) or not request["path"].startswith("/api/")
                    or type(request.get("status")) is not int or type(request.get("watch")) is not bool):
                raise CaptureError(variant + " HTTP request evidence is malformed")
    direct = result.get("direct")
    if not isinstance(direct, dict):
        raise CaptureError(variant + " probe omitted direct API observations")

    def identities(value: object, namespace: str) -> list[tuple[str, str, str]]:
        if not isinstance(value, list):
            raise CaptureError(variant + " direct identity rows are malformed")
        normalized = []
        for row in value:
            if (not isinstance(row, dict) or row.get("namespace") != namespace
                    or not isinstance(row.get("name"), str) or not isinstance(row.get("uid"), str)
                    or not row["name"] or not row["uid"]):
                raise CaptureError(variant + " direct identity row lacks exact namespace/name/UID")
            normalized.append((row["namespace"], row["name"], row["uid"]))
        return sorted(normalized)

    namespace_a, namespace_b = result.get("namespaceA"), result.get("namespaceB")
    denied_namespace, config_map_name = result.get("deniedNamespace"), result.get("configMapName")
    if any(not isinstance(value, str) or not value for value in
           (namespace_a, namespace_b, denied_namespace, config_map_name)):
        raise CaptureError(variant + " probe scope identity is malformed")
    if len({namespace_a, namespace_b, denied_namespace}) != 3:
        raise CaptureError(variant + " probe namespaces are not distinct")
    direct_a = identities(direct.get("teamA"), namespace_a)
    direct_b = identities(direct.get("teamB"), namespace_b)
    if (len(direct_a) != 1 or direct_a[0][1] != config_map_name
            or len(direct_b) != 1 or direct_b[0][1] != config_map_name
            or direct_a[0][2] == direct_b[0][2]):
        raise CaptureError(variant + " direct reads do not prove same-name distinct-UID objects")
    if expected_fixture_uids is not None and (
            direct_a[0][2] != expected_fixture_uids.get(namespace_a)
            or direct_b[0][2] != expected_fixture_uids.get(namespace_b)):
        raise CaptureError(variant + " direct API identity differs from the admin-verified fixture UIDs")
    if (direct.get("teamAError") != "success" or direct.get("teamBError") != "success"
            or direct.get("allError") != "Forbidden" or direct.get("deniedError") != "Forbidden"
            or direct.get("clusterScopedNamespacedError") not in ("NotFound", "BadRequest")):
        raise CaptureError(variant + " direct read outcomes differ from the required controls")
    paths = {
        "teamA": "/api/v1/namespaces/" + namespace_a + "/configmaps",
        "teamB": "/api/v1/namespaces/" + namespace_b + "/configmaps",
        "denied": "/api/v1/namespaces/" + denied_namespace + "/configmaps",
        "all": "/api/v1/configmaps",
        "nodes": "/api/v1/namespaces/" + namespace_a + "/nodes",
    }

    def statuses(records: list[dict], path: str) -> list[int]:
        return [request["status"] for request in records if request["path"] == path and not request["watch"]]

    direct_records = result["directRequests"]
    if not all(statuses(direct_records, path) for path in paths.values()):
        raise CaptureError(variant + " direct reads did not retain all five exact API paths")
    if (200 not in statuses(direct_records, paths["teamA"])
            or 200 not in statuses(direct_records, paths["teamB"])
            or 403 not in statuses(direct_records, paths["all"])
            or 403 not in statuses(direct_records, paths["denied"])):
        raise CaptureError(variant + " direct API status controls differ from the fixture contract")
    if not any(status in (400, 404) for status in statuses(direct_records, paths["nodes"])):
        raise CaptureError(variant + " direct cluster-scope invalid-namespace control was not a 400/404")
    if variant == "before":
        if exit_code == 0 or set(mismatches) != EXPECTED_OLD_MISMATCHES:
            raise CaptureError("pinned old source did not show exactly the expected namespace-cache regression")
        if checks.get("sameNameDifferentUID") is not True:
            raise CaptureError("old-source fixture did not prove colliding names have distinct UIDs")
        watched = result.get("watch")
        if (not isinstance(watched, dict) or identities(watched.get("teamA"), namespace_a) != direct_a
                or watched.get("teamB") != [] or watched.get("teamBError") != "success"
                or watched.get("allError") != "success" or watched.get("deniedError") != "success"
                or watched.get("clusterScopedNamespacedError") != "success"):
            raise CaptureError("old source did not retain the exact successful false-empty results")
        for path in (paths["teamB"], paths["all"], paths["denied"], paths["nodes"]):
            if statuses(result["watchRequests"], path):
                raise CaptureError("old source unexpectedly fell through for a scope-mismatched cache read")
    elif variant == "after":
        if exit_code != 0 or mismatches or not EXPECTED_NEW_CHECKS.issubset(checks) or any(checks[key] is not True for key in EXPECTED_NEW_CHECKS):
            raise CaptureError("pinned fixed source did not pass every namespace-cache acceptance check")
        watched = result.get("watch")
        if not isinstance(watched, dict):
            raise CaptureError("fixed-source probe omitted watch-backed observations")
        if (identities(watched.get("teamA"), namespace_a) != direct_a
                or identities(watched.get("teamB"), namespace_b) != direct_b
                or watched.get("teamAError") != "success" or watched.get("teamBError") != "success"
                or watched.get("allError") != "Forbidden" or watched.get("deniedError") != "Forbidden"
                or watched.get("clusterScopedNamespacedError") not in ("NotFound", "BadRequest")):
            raise CaptureError("fixed-source outcomes/identities differ from direct API evidence")
        for path, expected in ((paths["teamB"], 200), (paths["all"], 403), (paths["denied"], 403)):
            if expected not in statuses(result["watchRequests"], path):
                raise CaptureError("fixed source did not retain the expected fallback status for " + path)
        if not any(status in (400, 404) for status in statuses(result["watchRequests"], paths["nodes"])):
            raise CaptureError("fixed source did not retain the cluster-scope API fallback status")
    else:
        raise CaptureError("unknown probe variant")
    return {"variant": variant, "exitCode": exit_code, "mismatches": mismatches,
            "checks": checks, "directRequests": result["directRequests"], "watchRequests": result["watchRequests"]}


def _write_probe_in_worktree(worktree: Path, probe: bytes, variant: str) -> str:
    target = worktree / "cmd/cub-scout/observation_watch_live_test.go"
    if target.exists():
        raise CaptureError("source revision unexpectedly already contains the temporary live probe")
    payload = probe if variant == "after" else _old_adapter(probe)
    _write(target, payload)
    return sha256(payload)


def _git_text(git: str, args: list[str], timeout: float, *, deadline: float | None = None) -> str:
    code, stdout, _stderr = _run([git, *args], timeout, max_output=MAX_OUTPUT, deadline=deadline)
    if code:
        raise CaptureError("git operation failed: " + args[0])
    return stdout.decode("utf-8", "strict").strip()


def _git_bytes(git: str, args: list[str], timeout: float, *, deadline: float | None = None) -> bytes:
    code, stdout, _stderr = _run([git, *args], timeout, max_output=MAX_OUTPUT, deadline=deadline)
    if code:
        raise CaptureError("git operation failed: " + args[0])
    return stdout


def _worktree(git: str, revision: str, directory: Path, deadline: float) -> None:
    code, _stdout, _stderr = _run([git, "-C", str(REPO), "worktree", "add", "--detach", str(directory), revision],
                                  30, max_output=MAX_OUTPUT, deadline=deadline)
    if code:
        raise CaptureError("could not create isolated source worktree for " + revision[:12])


def _private_kubeconfig(admin: Path, observer: Path, cluster: str, namespace: str,
                        kubectl: str, env: dict[str, str], deadline: float) -> tuple[str, str]:
    command = [kubectl, "--kubeconfig", str(admin), "--context", "kind-" + cluster,
               "config", "view", "--minify", "--flatten", "--raw", "-o", "json"]
    code, stdout, _stderr = _run(command, 20, env=env, max_output=MAX_OUTPUT, deadline=deadline)
    if code:
        raise CaptureError("could not read the freshly-created private kind config")
    try:
        config = json.loads(stdout)
        cluster_data = config["clusters"][0]["cluster"]
        server, ca_data = cluster_data["server"], cluster_data["certificate-authority-data"]
    except (json.JSONDecodeError, KeyError, IndexError, TypeError):
        raise CaptureError("private kind config lacks server or CA data") from None
    token_args = [kubectl, "--kubeconfig", str(admin), "--context", "kind-" + cluster,
                  "-n", namespace, "create", "token", "watch-probe", "--duration=600s"]
    code, token_bytes, _stderr = _run(token_args, 20, env=env, max_output=4096, deadline=deadline)
    if code:
        raise CaptureError("could not create bounded read-only probe token")
    token = token_bytes.decode("ascii", "strict").strip()
    if not token or any(char.isspace() for char in token):
        raise CaptureError("observer token response is malformed")
    context_name = "kind-" + cluster
    _write(observer, _observer_config(server, ca_data, token, context_name, namespace))
    os.chmod(observer, 0o600)
    return sha256(observer.read_bytes()), context_name


def _probe_environment(base_env: dict[str, str], kubeconfig: Path, context_name: str,
                       variant: str, namespaces: tuple[str, str, str], cm_name: str,
                       result_path: Path, go_cache: Path) -> dict[str, str]:
    private_env = {key: value for key, value in base_env.items()
                   if key in ("PATH", "HOME", "TMPDIR", "LANG", "LC_ALL")}
    private_env.update({
        "KUBECONFIG": str(kubeconfig),
        "SCOUT_WNS_PRIVATE_KUBECONFIG": str(kubeconfig),
        "SCOUT_WNS_EXPECTED_CONTEXT": context_name,
        "SCOUT_WNS_OWNED_CLUSTER": context_name.removeprefix("kind-"),
        "SCOUT_WNS_LIVE_PROBE": "1",
        "SCOUT_WNS_VARIANT": variant,
        "SCOUT_WNS_NAMESPACE_A": namespaces[0],
        "SCOUT_WNS_NAMESPACE_B": namespaces[1],
        "SCOUT_WNS_DENIED_NAMESPACE": namespaces[2],
        "SCOUT_WNS_CONFIGMAP_NAME": cm_name,
        "SCOUT_WNS_RESULT_PATH": str(result_path),
        "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local",
        "GOCACHE": str(go_cache),
    })
    return private_env


def _compile_probe(worktree: Path, variant: str, deadline: float,
                   base_env: dict[str, str], go_cache: Path) -> tuple[int, bytes, bytes, float]:
    compile_env = {key: value for key, value in base_env.items()
                   if key in ("PATH", "HOME", "TMPDIR", "LANG", "LC_ALL")}
    compile_env.update({"GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local",
                        "GOCACHE": str(go_cache)})
    started = time.monotonic()
    code, stdout, stderr = _run(["go", "test", "./cmd/cub-scout", "-run", "^$", "-count=1"],
                                180, env=compile_env, max_output=MAX_OUTPUT, deadline=deadline)
    return code, stdout, stderr, round(time.monotonic() - started, 3)


def _run_probe(worktree: Path, variant: str, kubeconfig: Path, context_name: str,
               namespaces: tuple[str, str, str], cm_name: str, result_path: Path,
               deadline: float, base_env: dict[str, str], go_cache: Path) -> tuple[int, bytes, bytes, dict]:
    private_env = _probe_environment(base_env, kubeconfig, context_name, variant,
                                     namespaces, cm_name, result_path, go_cache)
    command = ["go", "test", "./cmd/cub-scout", "-run", "^TestWatchNamespaceLiveProbe$", "-count=1", "-v"]
    code, stdout, stderr = _run(command, 220, env=private_env, max_output=MAX_OUTPUT, deadline=deadline)
    try:
        result = json.loads(result_path.read_text())
    except (OSError, json.JSONDecodeError):
        result = {}
    return code, stdout, stderr, result


def capture(output: Path, shared_config_hash_source: Path) -> dict:
    out = _fresh_output(output)
    started = utc_now()
    start_mono = time.monotonic()
    deadline = start_mono + MAX_SECONDS
    errors: list[str] = []
    cluster = _owned_cluster_name()
    namespaces = (cluster + "-a", cluster + "-b", cluster + "-denied")
    cm_name = "same-name"
    private = Path(tempfile.mkdtemp(prefix="scout-watch-ns-private-"))
    os.chmod(private, 0o700)
    admin = private / "admin.kubeconfig"
    observer = private / "observer.kubeconfig"
    old_tree, new_tree = private / "source-before", private / "source-after"
    create_attempted = False
    cleanup_verified = False
    private_hashes: dict[str, str] = {}
    shared_before = ""
    shared_after = ""
    source_records: dict[str, dict] = {}
    phase_records: list[dict] = []
    operations: list[dict] = []
    tool_pins: dict[str, dict] = {}
    marker = {"schema": "watch-cache-namespace-owned-marker.v1", "clusterName": cluster,
              "ownerPid": os.getpid(), "sourceBefore": OLD_SOURCE, "sourceAfter": NEW_SOURCE,
              "createdAt": utc_now(), "privateDirectoryName": private.name}
    marker_bytes = (json.dumps(marker, sort_keys=True, indent=2) + "\n").encode()
    _write(out / "owned-cluster-marker.json", marker_bytes)
    signal_state = None
    try:
        requested_shared = shared_config_hash_source.expanduser().absolute()
        if requested_shared.is_symlink():
            raise CaptureError("integrity-only kubeconfig must be an existing regular non-symlink file")
        shared_config_hash_source = requested_shared.resolve(strict=True)
        if not shared_config_hash_source.is_file():
            raise CaptureError("integrity-only kubeconfig must be an existing regular non-symlink file")
        shared_before = sha256_file(shared_config_hash_source)
        signal_state = inv04._arm_capture_signals()
        probe = GO_PROBE.read_bytes()
        probe_sha = sha256(probe)
        git, kind, kubectl, docker, go = (_required_command(n) for n in ("git", "kind", "kubectl", "docker", "go"))
        tool_paths = {"git": git, "kind": kind, "kubectl": kubectl, "docker": docker, "go": go}
        tool_pins = {name: {"path": path, "sha256": sha256_file(Path(path))} for name, path in tool_paths.items()}
        repo_head = _git_text(git, ["-C", str(REPO), "rev-parse", "HEAD"], 10, deadline=deadline)
        if _git_text(git, ["-C", str(REPO), "status", "--porcelain"], 10, deadline=deadline):
            raise CaptureError("source checkout must be clean before owned-cluster proof")
        ancestor_code, _ancestor_out, _ancestor_err = _run(
            [git, "-C", str(REPO), "merge-base", "--is-ancestor", NEW_SOURCE, repo_head],
            10, deadline=deadline)
        if ancestor_code:
            raise CaptureError("reviewed fixed source is not an ancestor of this clean helper checkout")
        current_watch_sha = sha256(_git_bytes(git, ["-C", str(REPO), "show", "HEAD:cmd/cub-scout/observation_watch.go"], 15, deadline=deadline))
        pinned_watch_sha = sha256(_git_bytes(git, ["-C", str(REPO), "show", NEW_SOURCE + ":cmd/cub-scout/observation_watch.go"], 15, deadline=deadline))
        if current_watch_sha != pinned_watch_sha:
            raise CaptureError("current checkout watch implementation differs from the reviewed fixed source revision")
        for label, revision in (("before", OLD_SOURCE), ("after", NEW_SOURCE)):
            checked = _git_text(git, ["-C", str(REPO), "cat-file", "-t", revision], 10, deadline=deadline)
            if checked != "commit":
                raise CaptureError("pinned source revision is not available locally")
            source_records[label] = {
                "revision": revision,
                "watchSourceSha256": sha256(_git_bytes(git, ["-C", str(REPO), "show", revision + ":cmd/cub-scout/observation_watch.go"], 15, deadline=deadline)),
            }
        source_records["after"]["goProbeSha256"] = probe_sha
        source_records["captureCheckoutRevision"] = repo_head
        source_records["captureScriptSha256"] = sha256_file(Path(__file__))
        source_records["inv04HelperSha256"] = sha256_file(INV04_PATH)
        source_records["goProbePath"] = str(GO_PROBE.relative_to(REPO))
        source_records["adapterRules"] = "before variant changes only newWatchBackedClient signature and removes new scope-map assignment from the temporary probe; old cache eligibility source is unchanged"

        local_env = dict(os.environ)
        if local_env.get("DOCKER_HOST"):
            raise CaptureError("DOCKER_HOST is set; refusing a remote container engine")
        inv04.require_local_docker(local_env)
        kind_version = inv04._call("kind", ["version"], 10).decode("utf-8", "replace").strip()
        inv04.require_kind_version(kind_version)
        image_info = inv04._call("docker", ["image", "inspect", inv04.NODE_IMAGE], 15)
        node_repo_digest = inv04.verified_node_repo_digest(image_info)
        tool_pins["kind"]["version"] = kind_version
        tool_pins["kubectl"]["versionJson"] = json.loads(inv04._call(
            "kubectl", ["version", "--client", "-o", "json"], 15).decode("utf-8", "strict"))
        tool_pins["go"]["version"] = inv04._call("go", ["version"], 10).decode("utf-8", "replace").strip()

        _worktree(git, OLD_SOURCE, old_tree, deadline)
        _worktree(git, NEW_SOURCE, new_tree, deadline)
        for label, tree in (("before", old_tree), ("after", new_tree)):
            test_hash = _write_probe_in_worktree(tree, probe, label)
            source_records[label]["adaptedProbeSha256"] = test_hash

        # Compile both pinned source/probe pairs before creating any cluster.
        # No KUBECONFIG is present in this process environment and -run '^$'
        # executes no Go tests.
        compile_base_env = {key: value for key, value in os.environ.items()
                            if key in ("PATH", "HOME", "TMPDIR", "LANG", "LC_ALL")}
        for label, tree in (("before", old_tree), ("after", new_tree)):
            compile_started = utc_now()
            code, stdout, stderr, elapsed = _compile_probe(tree, label, deadline,
                                                           compile_base_env, private / "go-cache")
            _write(out / (label + "-compile-stdout.txt"), stdout)
            _write(out / (label + "-compile-stderr.txt"), stderr)
            operation = {"operation": "offline-go-probe-compile", "variant": label,
                         "sourceRevision": OLD_SOURCE if label == "before" else NEW_SOURCE,
                         "startedAt": compile_started, "endedAt": utc_now(),
                         "elapsedSeconds": elapsed, "exitCode": code,
                         "argv": ["go", "test", "./cmd/cub-scout", "-run", "^$", "-count=1"],
                         "networkDownloadsDisabled": True, "kubeconfigProvided": False}
            operations.append(operation)
            if code:
                raise CaptureError(label + " Go probe compile-only preflight failed before cluster creation")

        preflight_clusters = inv04._call("kind", ["get", "clusters"], 15).decode("utf-8", "replace").splitlines()
        if cluster in preflight_clusters:
            raise CaptureError("refusing to reuse the generated kind cluster name")
        admin_env = dict(local_env, KUBECONFIG=str(admin))
        create_attempted = True
        create_started, create_clock = utc_now(), time.monotonic()
        create_code, _create_out, _create_err = _run(
            [kind, "create", "cluster", "--name", cluster, "--image", inv04.NODE_IMAGE,
             "--kubeconfig", str(admin), "--wait", "120s"], 165, env=admin_env,
            max_output=MAX_OUTPUT, deadline=deadline)
        if create_code:
            raise CaptureError("owned kind cluster creation failed")
        operations.append({"operation": "kind-create-owned-cluster", "startedAt": create_started,
                           "endedAt": utc_now(), "elapsedSeconds": round(time.monotonic() - create_clock, 3),
                           "exitCode": create_code, "clusterName": cluster, "image": inv04.NODE_IMAGE})
        os.chmod(admin, 0o600)
        private_hashes["adminInitial"] = sha256_file(admin)
        version_bytes = inv04._call("kubectl", ["--kubeconfig", str(admin), "--context", "kind-" + cluster,
                                                  "version", "-o", "json"], 15, admin_env)
        api_versions = json.loads(version_bytes)
        tool_pins["kubernetesServerVersion"] = api_versions.get("serverVersion", {})
        manifests = _fixture_manifests(namespaces, cm_name)
        _write(out / "literal-fixtures.yaml", manifests)
        apply_started, apply_clock = utc_now(), time.monotonic()
        apply_code, _apply_out, _apply_err = _run(
            [kubectl, "--kubeconfig", str(admin), "--context", "kind-" + cluster, "apply", "-f", str(out / "literal-fixtures.yaml")],
            60, env=admin_env, deadline=deadline)
        if apply_code:
            raise CaptureError("could not create owned namespace proof fixtures")
        operations.append({"operation": "kubectl-apply-owned-fixtures", "startedAt": apply_started,
                           "endedAt": utc_now(), "elapsedSeconds": round(time.monotonic() - apply_clock, 3),
                           "exitCode": apply_code, "manifestSha256": sha256(manifests)})
        for ns in namespaces[:2]:
            raw = inv04._call("kubectl", ["--kubeconfig", str(admin), "--context", "kind-" + cluster,
                                            "-n", ns, "get", "configmap", cm_name, "-o", "json"], 15, admin_env)
            fixture = json.loads(raw)
            if fixture.get("kind") != "ConfigMap" or fixture.get("metadata", {}).get("namespace") != ns or not fixture.get("metadata", {}).get("uid"):
                raise CaptureError("owned ConfigMap fixture identity could not be verified")
            source_records.setdefault("fixtureUIDs", {})[ns] = fixture["metadata"]["uid"]
        observer_hash, context_name = _private_kubeconfig(admin, observer, cluster, namespaces[0], kubectl, admin_env, deadline)
        private_hashes["observerInitial"] = observer_hash
        observer_env = {key: value for key, value in os.environ.items() if key in ("PATH", "HOME", "TMPDIR", "LANG", "LC_ALL")}
        observer_env.update({"KUBECONFIG": str(observer), "GOPROXY": "off", "GOSUMDB": "off", "GOTOOLCHAIN": "local"})
        for label, tree in (("before", old_tree), ("after", new_tree)):
            result_path = out / (label + "-probe-result.json")
            probe_started, probe_clock = utc_now(), time.monotonic()
            code, stdout, stderr, result = _run_probe(tree, label, observer, context_name, namespaces,
                                                       cm_name, result_path, deadline, observer_env,
                                                       private / "go-cache")
            ended_at, probe_elapsed = utc_now(), round(time.monotonic() - probe_clock, 3)
            _write(out / (label + "-probe-stdout.txt"), stdout)
            _write(out / (label + "-probe-stderr.txt"), stderr)
            phase = validate_probe_result(label, code, result, source_records.get("fixtureUIDs", {}))
            phase_records.append(phase)
            phase["startedAt"], phase["endedAt"], phase["elapsedSeconds"] = probe_started, ended_at, probe_elapsed
            phase["argv"] = ["go", "test", "./cmd/cub-scout", "-run", "^TestWatchNamespaceLiveProbe$", "-count=1", "-v"]
            phase["workingSourceRevision"] = OLD_SOURCE if label == "before" else NEW_SOURCE
            phase["resultFile"] = result_path.name
            phase["resultSha256"] = sha256_file(result_path)
            phase["adaptedProbeSha256"] = source_records[label]["adaptedProbeSha256"]
            if time.monotonic() >= deadline:
                raise CaptureError("overall proof deadline expired")
    except BaseException as error:
        errors.append(str(error) if isinstance(error, (CaptureError, inv04.CaptureError)) else type(error).__name__)
    finally:
        # Once interrupted or timed out, restore a bounded cleanup window and
        # act only on the freshly generated, marker-owned cluster name.
        if signal_state is not None:
            try:
                inv04._suppress_capture_signals(signal_state)
            except Exception:
                pass
        try:
            cleanup_started, cleanup_clock = utc_now(), time.monotonic()
            if create_attempted:
                listed = inv04._call("kind", ["get", "clusters"], 15).decode("utf-8", "replace").splitlines()
                marker_matches = marker.get("clusterName") == cluster and marker.get("ownerPid") == os.getpid()
                if marker_matches and cluster in listed:
                    _run([_required_command("kind"), "delete", "cluster", "--name", cluster,
                          "--kubeconfig", str(admin)], CLEANUP_SECONDS,
                         env=dict(os.environ, KUBECONFIG=str(admin)), max_output=MAX_OUTPUT)
                after = inv04._call("kind", ["get", "clusters"], 15).decode("utf-8", "replace").splitlines()
                cleanup_verified = cluster not in after
            else:
                cleanup_verified = True
            operations.append({"operation": "owned-kind-cleanup", "startedAt": cleanup_started,
                               "endedAt": utc_now(), "elapsedSeconds": round(time.monotonic() - cleanup_clock, 3),
                               "clusterAbsent": cleanup_verified})
        except Exception:
            cleanup_verified = False
            errors.append("owned kind cleanup could not be confirmed")
        for label, path in (("adminBeforeCleanup", admin), ("observerBeforeCleanup", observer)):
            if path.is_file():
                private_hashes[label] = sha256_file(path)
        if private_hashes.get("adminInitial") and private_hashes.get("adminBeforeCleanup") != private_hashes["adminInitial"]:
            errors.append("private admin kubeconfig changed during capture")
        if private_hashes.get("observerInitial") and private_hashes.get("observerBeforeCleanup") != private_hashes["observerInitial"]:
            errors.append("private observer kubeconfig changed during capture")
        try:
            shared_after = sha256_file(shared_config_hash_source)
        except Exception:
            errors.append("integrity-only shared kubeconfig could not be rehashed")
        if shared_before and shared_after != shared_before:
            errors.append("integrity-only shared kubeconfig changed")
        for label, tree in (("before", old_tree), ("after", new_tree)):
            if tree.exists():
                code, _stdout, _stderr = _run([_required_command("git"), "-C", str(REPO), "worktree", "remove", "--force", str(tree)],
                                              CLEANUP_SECONDS, max_output=MAX_OUTPUT)
                if code:
                    errors.append(label + " temporary source worktree cleanup failed")
        shutil.rmtree(private, ignore_errors=True)
        if signal_state is not None:
            try:
                inv04._restore_capture_signals(signal_state)
            except Exception:
                pass

    provenance = {
        "schema": SCHEMA, "startedAt": started, "endedAt": utc_now(),
        "elapsedSeconds": round(time.monotonic() - start_mono, 3),
        "expectedSourceCommits": {"before": OLD_SOURCE, "after": NEW_SOURCE},
        "source": source_records, "toolPins": tool_pins,
        "kind": {"version": tool_pins.get("kind", {}).get("version"), "nodeImage": inv04.NODE_IMAGE,
                 "nodeImageRepoDigest": node_repo_digest if "node_repo_digest" in locals() else None,
                 "clusterName": cluster, "markerSha256": sha256(marker_bytes), "createdAttempted": create_attempted},
        "fixture": {"namespaces": list(namespaces), "configMapName": cm_name,
                    "manifestSha256": sha256(manifests) if "manifests" in locals() else None,
                    "sameNameFixtureUIDs": source_records.get("fixtureUIDs", {})},
        "observerScope": "namespace Role in A and B; no RoleBinding in denied namespace; no cluster Role",
        "phases": phase_records,
        "operations": operations,
        "privateKubeconfigSha256": private_hashes,
        "integrityOnlySharedKubeconfigSha256": {"before": shared_before, "after": shared_after,
                                                 "unchanged": bool(shared_before and shared_before == shared_after)},
        "cleanupVerified": cleanup_verified, "errors": errors,
        "limits": ["one fresh local kind cluster; sequential old/new source probes against unchanged fixtures",
                   "only two ConfigMap namespace LISTs, one all-namespace LIST, one denied-namespace LIST, and one invalid namespaced cluster-GVR LIST are compared",
                   "no cluster-wide inventory completeness, informer freshness, reconnect, storage, or production latency claim",
                   "old probe adapts only the constructor signature and synthetic cluster-scope test lister setup; old cache eligibility is unmodified",
                   "credentials and kubeconfig contents are not retained; the explicit shared-config input is read only for before/after hashing"],
    }
    _write(out / "provenance.json", (json.dumps(provenance, indent=2, sort_keys=True) + "\n").encode())
    if errors or not cleanup_verified or len(phase_records) != 2:
        raise CaptureError("owned proof incomplete; inspect retained private provenance.json")
    return provenance


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true", help="required acknowledgement to create/delete one owned local kind cluster")
    parser.add_argument("--integrity-only-shared-kubeconfig", type=Path,
                        help="explicit kubeconfig path to hash before/after only; it is never parsed or passed to a command")
    parser.add_argument("--output-dir", type=Path, required=True, help="fresh private evidence directory under /tmp")
    args = parser.parse_args(argv)
    if not args.execute or not args.integrity_only_shared_kubeconfig:
        parser.error("--execute and --integrity-only-shared-kubeconfig are required; no default kubeconfig is used")
    try:
        result = capture(args.output_dir, args.integrity_only_shared_kubeconfig)
    except (CaptureError, inv04.CaptureError, OSError, ValueError) as error:
        print(str(error), file=sys.stderr)
        return 1
    print(json.dumps({"schema": result["schema"], "cleanupVerified": result["cleanupVerified"],
                      "phaseCount": len(result["phases"]), "modelCalls": 0}, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
