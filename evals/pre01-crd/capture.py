#!/usr/bin/env python3
"""Prepare or capture the bounded PRE-01 ServiceMonitor/CRD prerequisite case.

All capture runs require --execute. The source-only validation functions are
offline and deliberately fail closed on any source drift or ambiguous YAML.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import ssl
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

try:
    import yaml
except ImportError:
    yaml = None

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
INV04_PATH = HERE.parent / "inv04-rbac" / "capture.py"
spec = importlib.util.spec_from_file_location("pre01_inv04_capture", INV04_PATH)
inv04 = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(inv04)

SOURCE_REVISION = "9ab4c753a888dc305a3c07956c9f8f5a19eb70a0"
SCOUT_SOURCE_REVISION = "eec6d442279955af0f1fc39c637252ac8eb0f081"
SCOUT_SHA256 = "7d20aa7bb33b477dfd88afb2f53b1a6f773a5a4ffbcff7f81d8dcbfeb5a12bab"
SERVICE_MONITOR_PATH = "recipes/prometheus-community/kube-prometheus-stack/87.19.2/revisions/no-crds/r001/rendered/release-objects.yaml"
RECORD_PATH = "data/base-variant-records/records/prometheus-community-kube-prometheus-stack-87-19-2-no-crds.yaml"
CRD_PATH = "packages/prometheus-community/kube-prometheus-stack/87.19.2/prerequisites/kube-prometheus-stack-lifecycle/default-crds.yaml"
SOURCE_HASHES = {
    SERVICE_MONITOR_PATH: "86d54113ec12fcea838d30e7e2ff6722de375bd0f6cc0e09ab8a14078c941446",
    RECORD_PATH: "d915b20d548433314ceee42cb9db29ed75b1009a04ba6da81bddf0011ec75153",
    CRD_PATH: "bba9c3c345520981690399f86ad6a33ce45c9dac61f89721e1a5730f8f1ed964",
}
SERVICE_MONITOR_ID = ("monitoring.coreos.com/v1", "ServiceMonitor", "monitoring",
                      "kube-prometheus-stack-kube-state-metrics")
CRD_NAME = "servicemonitors.monitoring.coreos.com"
NAMESPACE = "monitoring"
RESOURCE_NAME = "kube-prometheus-stack-kube-state-metrics"
MAX_BODY = 4 * 1024 * 1024
MAX_OUTPUT = 2 * 1024 * 1024
API_PATHS = (
    "/apis/apiextensions.k8s.io/v1/customresourcedefinitions/" + CRD_NAME,
    "/apis/monitoring.coreos.com/v1",
    "/apis/monitoring.coreos.com/v1/namespaces/monitoring/servicemonitors/" + RESOURCE_NAME,
)


class CaptureError(inv04.CaptureError):
    pass


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _yaml_documents(data: bytes, label: str) -> list[dict]:
    if yaml is None:
        raise CaptureError("PyYAML is required for exact source parsing")
    try:
        docs = [obj for obj in yaml.safe_load_all(data.decode("utf-8")) if obj is not None]
    except (UnicodeDecodeError, yaml.YAMLError):
        raise CaptureError(label + " is not valid UTF-8 YAML") from None
    if any(not isinstance(obj, dict) for obj in docs):
        raise CaptureError(label + " contains a non-object YAML document")
    return docs


def validate_service_monitor(obj: dict) -> None:
    if (obj.get("apiVersion"), obj.get("kind")) != SERVICE_MONITOR_ID[:2]:
        raise CaptureError("selected object is not the exact ServiceMonitor GVK")
    meta = obj.get("metadata")
    if not isinstance(meta, dict) or (meta.get("namespace"), meta.get("name")) != SERVICE_MONITOR_ID[2:]:
        raise CaptureError("selected ServiceMonitor name or namespace differs from the source pin")


def select_service_monitor(docs: list[dict]) -> dict:
    matches = [d for d in docs if d.get("kind") == "ServiceMonitor" and isinstance(d.get("metadata"), dict)
               and d["metadata"].get("name") == RESOURCE_NAME]
    if len(matches) != 1:
        raise CaptureError("rendered release source must contain exactly one candidate ServiceMonitor")
    validate_service_monitor(matches[0])
    return matches[0]


def validate_crd(obj: dict) -> None:
    meta, spec = obj.get("metadata"), obj.get("spec")
    names = spec.get("names") if isinstance(spec, dict) else None
    versions = spec.get("versions") if isinstance(spec, dict) else None
    if (obj.get("apiVersion"), obj.get("kind")) != ("apiextensions.k8s.io/v1", "CustomResourceDefinition"):
        raise CaptureError("selected object is not an apiextensions.k8s.io/v1 CRD")
    if not isinstance(meta, dict) or meta.get("name") != CRD_NAME:
        raise CaptureError("selected CRD name differs from the ServiceMonitor API")
    if not isinstance(spec, dict) or spec.get("group") != "monitoring.coreos.com" or spec.get("scope") != "Namespaced":
        raise CaptureError("CRD group or scope does not match the pinned ServiceMonitor")
    if not isinstance(names, dict) or names.get("kind") != "ServiceMonitor" or names.get("plural") != "servicemonitors":
        raise CaptureError("CRD names do not match the pinned ServiceMonitor")
    if not isinstance(versions, list):
        raise CaptureError("CRD versions are missing")
    matching = [v for v in versions if isinstance(v, dict) and v.get("name") == "v1"]
    if len(matching) != 1 or matching[0].get("served") is not True or matching[0].get("storage") is not True:
        raise CaptureError("CRD does not serve and store exactly one v1 version")
    conversion = spec.get("conversion")
    if conversion not in (None, {"strategy": "None"}):
        raise CaptureError("CRD conversion is unsupported by this bounded capture")


def extract_pinned_sources(source_repo: Path, source_revision: str, expected_hashes: dict[str, str]) -> dict:
    """Read only pinned Git blobs, parse exact docs, and disclose normalization."""
    if not re.fullmatch(r"[0-9a-f]{40,64}", source_revision) or source_revision != SOURCE_REVISION:
        raise CaptureError("source revision differs from the reviewed full commit pin")
    if expected_hashes != SOURCE_HASHES:
        raise CaptureError("one or more source file hash pins differ from the reviewed pins")
    source_arg = source_repo.expanduser().absolute()
    if source_arg.is_symlink(): raise CaptureError("source checkout must be a real directory")
    repo = source_arg.resolve(strict=True)
    if not repo.is_dir(): raise CaptureError("source checkout must be a real directory")
    git = inv04._command("git")
    code, out, _err = inv04.run_bounded([git, "-C", str(repo), "rev-parse", "--show-toplevel"], 5, max_output=4096)
    if code or Path(out.decode("utf-8", "replace").strip()).resolve() != repo:
        raise CaptureError("source path is not a Git checkout root")
    code, _out, _err = inv04.run_bounded([git, "-C", str(repo), "cat-file", "-e", source_revision + "^{commit}"], 5, max_output=4096)
    if code: raise CaptureError("pinned source commit is unavailable in this checkout")
    blobs = {}
    for path, expected in SOURCE_HASHES.items():
        code, raw, _err = inv04.run_bounded([git, "-C", str(repo), "show", source_revision + ":" + path],
                                            10, max_output=5 * 1024 * 1024)
        if code: raise CaptureError("pinned source path is missing from the reviewed commit")
        if sha256(raw) != expected:
            raise CaptureError("pinned source file SHA-256 mismatch: " + path)
        blobs[path] = raw
    release_docs = _yaml_documents(blobs[SERVICE_MONITOR_PATH], "rendered release objects")
    service_monitor = select_service_monitor(release_docs)
    crd_docs = _yaml_documents(blobs[CRD_PATH], "CRD source")
    matching_crd = [d for d in crd_docs if isinstance(d.get("metadata"), dict)
                    and d["metadata"].get("name") == CRD_NAME]
    if len(matching_crd) != 1:
        raise CaptureError("CRD source must contain exactly one candidate matching CRD")
    crd = matching_crd[0]
    validate_crd(crd)
    record_docs = _yaml_documents(blobs[RECORD_PATH], "variant record")
    validate_variant_record(record_docs)
    # Use safe_dump on exactly the selected mapping. The authored source file remains
    # untouched; normalization is explicit in provenance and the normalized hash.
    sm_yaml = yaml.safe_dump(service_monitor, sort_keys=False, explicit_start=True).encode()
    crd_yaml = yaml.safe_dump(crd, sort_keys=False, explicit_start=True).encode()
    return {
        "sourceRevision": source_revision,
        "sourceFiles": {path: {"sha256": SOURCE_HASHES[path], "bytes": len(blobs[path])} for path in SOURCE_HASHES},
        "selectedServiceMonitor": {"apiVersion": SERVICE_MONITOR_ID[0], "kind": SERVICE_MONITOR_ID[1],
            "namespace": SERVICE_MONITOR_ID[2], "name": SERVICE_MONITOR_ID[3],
            "sourceObjectSha256": sha256(json.dumps(service_monitor, sort_keys=True, separators=(",", ":")).encode()),
            "normalizedYamlSha256": sha256(sm_yaml), "normalizedYaml": sm_yaml},
        "selectedCRD": {"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
            "name": CRD_NAME, "sourceObjectSha256": sha256(json.dumps(crd, sort_keys=True, separators=(",", ":")).encode()),
            "normalizedYamlSha256": sha256(crd_yaml), "normalizedYaml": crd_yaml},
        "normalization": "PyYAML safe_load_all selects the unique pinned object; safe_dump preserves parsed fields but changes formatting, anchors and comments",
        "normalizer": {"name": "PyYAML", "version": yaml.__version__},
        "prerequisiteRecordChecked": True,
    }


def validate_variant_record(docs: list[dict]) -> None:
    if len(docs) != 1:
        raise CaptureError("pinned variant record must parse as exactly one document")
    try:
        entries = docs[0]["spec"]["inputs"]["installTime"]
    except (KeyError, TypeError):
        raise CaptureError("pinned variant record lacks spec.inputs.installTime") from None
    if not isinstance(entries, list): raise CaptureError("variant record installTime input is not a list")
    exact = [entry for entry in entries if isinstance(entry, dict) and entry.get("name") == CRD_NAME]
    if len(exact) != 1:
        raise CaptureError("variant record must contain exactly one named ServiceMonitor CRD prerequisite")
    entry = exact[0]; details = entry.get("details")
    if (entry.get("type") != "requiredCRDs" or entry.get("required") is not True
            or not isinstance(details, dict) or details.get("name") != CRD_NAME
            or details.get("packagePath") != CRD_PATH):
        raise CaptureError("variant record's typed requiredCRDs declaration or package path changed")


def check_server(server: str) -> None:
    parsed = urllib.parse.urlparse(server)
    if (parsed.scheme != "https" or parsed.hostname not in ("127.0.0.1", "localhost", "::1")
            or not parsed.port or parsed.username or parsed.password or parsed.path or parsed.query or parsed.fragment):
        raise CaptureError("refusing a non-loopback or malformed Kubernetes API endpoint")


def check_api_path(path: str) -> None:
    if path not in API_PATHS:
        raise CaptureError("API path is outside the exact PRE-01 read allowlist")


def observer_kubeconfig(server: str, ca_data: str, token: str) -> bytes:
    check_server(server)
    if not ca_data or not token or any(c.isspace() for c in token):
        raise CaptureError("observer connection material is incomplete")
    config = {"apiVersion": "v1", "kind": "Config", "current-context": "pre01-observer",
        "clusters": [{"name": "pre01-owned", "cluster": {"server": server, "certificate-authority-data": ca_data}}],
        "users": [{"name": "pre01-observer", "user": {"token": token}}],
        "contexts": [{"name": "pre01-observer", "context": {"cluster": "pre01-owned", "user": "pre01-observer", "namespace": NAMESPACE}}]}
    return (json.dumps(config, sort_keys=True, separators=(",", ":")) + "\n").encode()


def credentials_absent(data: bytes, token: str, private: tuple[bytes, ...]) -> bool:
    return (not token or token.encode() not in data) and all(not x or x not in data for x in private)


def private_material(raw_config: dict, config_bytes: bytes) -> tuple[bytes, ...]:
    """Collect kubeconfig/key bytes in memory for output leak checks."""
    values = [config_bytes]
    for match in re.finditer(rb"(?m)^\s*(?:client-key-data|client-certificate-data):\s*([A-Za-z0-9+/=]+)\s*$", config_bytes):
        values.append(match[1])
        try: values.append(base64.b64decode(match[1], validate=True))
        except ValueError: raise CaptureError("private kubeconfig encoding is malformed") from None
    users = raw_config.get("users", []) if isinstance(raw_config, dict) else []
    if not isinstance(users, list): raise CaptureError("private kubeconfig users field is malformed")
    for entry in users:
        user = entry.get("user", {}) if isinstance(entry, dict) else {}
        if not isinstance(user, dict): raise CaptureError("private kubeconfig user is malformed")
        for key, value in user.items():
            if key in ("client-key-data", "client-certificate-data", "token", "password") and isinstance(value, str) and value:
                values.append(value.encode())
                if key.endswith("-data"):
                    try: values.append(base64.b64decode(value, validate=True))
                    except ValueError: raise CaptureError("private credential encoding is malformed") from None
    return tuple(values)


def prepare_output(path: Path) -> Path:
    candidate = path.expanduser().absolute()
    if candidate.is_symlink() or candidate.exists():
        raise CaptureError("output directory must be new and not a symlink")
    return inv04._fresh_output(candidate)


def verify_scout_binary(path: Path, expected_sha: str, source_revision: str, expected_revision: str) -> str:
    if source_revision != SCOUT_SOURCE_REVISION or expected_revision != SCOUT_SOURCE_REVISION:
        raise CaptureError("Scout source revision differs from the reviewed executable revision")
    if expected_sha != SCOUT_SHA256:
        raise CaptureError("Scout SHA-256 must equal the reviewed executable pin")
    candidate = path.expanduser().absolute()
    if candidate.is_symlink(): raise CaptureError("Scout executable must not be a symlink")
    binary = candidate.resolve(strict=True)
    if not binary.is_file() or not os.access(binary, os.X_OK): raise CaptureError("Scout executable is invalid")
    actual = sha256(binary.read_bytes())
    if actual != expected_sha or actual != SCOUT_SHA256:
        raise CaptureError("Scout executable does not match the reviewed SHA-256 pin")
    return actual


def validate_raw(path: str, status: int, body: bytes, phase: str) -> dict:
    """404 is absence; denied/unavailable evidence remains UNKNOWN and fails capture."""
    check_api_path(path)
    if status == 404:
        try:
            missing = json.loads(body)
        except (UnicodeDecodeError, json.JSONDecodeError):
            raise CaptureError("HTTP 404 response lacks a Kubernetes NotFound Status body") from None
        if (not isinstance(missing, dict) or missing.get("apiVersion") != "v1" or missing.get("kind") != "Status"
                or missing.get("status") != "Failure" or missing.get("reason") != "NotFound" or missing.get("code") != 404):
            raise CaptureError("HTTP 404 response is not an explicit Kubernetes NotFound")
        allowed = ((path == API_PATHS[0] and phase == "absent")
                   or (path == API_PATHS[1] and phase == "absent")
                   or (path == API_PATHS[2] and phase in ("absent", "present-before-apply")))
        if not allowed:
            raise CaptureError("required API evidence is missing in the present phase")
        return {"status": "absent", "httpStatus": 404}
    if status != 200:
        raise CaptureError("required API evidence was not readable; 403 and transport failures are not absence")
    try:
        obj = json.loads(body)
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise CaptureError("raw API response is not valid JSON") from None
    if not isinstance(obj, dict):
        raise CaptureError("raw API response is not an object")
    if path == API_PATHS[0]:
        if phase == "absent":
            raise CaptureError("the exact CRD GET returned 200 in the absent phase")
        if (obj.get("apiVersion"), obj.get("kind")) != ("apiextensions.k8s.io/v1", "CustomResourceDefinition"):
            raise CaptureError("CRD GET returned the wrong GVK")
        validate_crd(obj)
        metadata = obj.get("metadata", {})
        if (not isinstance(metadata.get("uid"), str) or not metadata.get("uid")
                or not isinstance(metadata.get("resourceVersion"), str) or not metadata.get("resourceVersion")):
            raise CaptureError("CRD response lacks UID or resourceVersion")
        status_obj = obj.get("status", {})
        conditions = status_obj.get("conditions", []) if isinstance(status_obj, dict) else None
        if not isinstance(conditions, list): raise CaptureError("CRD conditions are malformed")
        established = any(isinstance(c, dict) and c.get("type") == "Established" and c.get("status") == "True"
                          for c in conditions)
        if phase == "present" and not established:
            raise CaptureError("CRD GET does not show Established=True")
        return {"status": "present", "httpStatus": 200, "uid": metadata["uid"],
                "resourceVersion": metadata["resourceVersion"], "established": established}
    if path == API_PATHS[1]:
        if obj.get("kind") != "APIResourceList" or obj.get("groupVersion") != "monitoring.coreos.com/v1":
            raise CaptureError("discovery returned the wrong API group/version")
        resources = obj.get("resources")
        if not isinstance(resources, list):
            raise CaptureError("discovery response lacks resources")
        matches = [r for r in resources if isinstance(r, dict) and r.get("name") == "servicemonitors" and r.get("kind") == "ServiceMonitor"]
        if phase == "absent" and matches:
            raise CaptureError("ServiceMonitor discovery unexpectedly exists in absent phase")
        if phase == "present" and len(matches) != 1:
            raise CaptureError("ServiceMonitor discovery is not registered exactly once")
        return {"status": "registered" if matches else "absent", "httpStatus": 200}
    if phase in ("absent", "present-before-apply"):
        raise CaptureError("the dependent ServiceMonitor GET returned 200 before its expected absence checkpoint")
    expected_kind = "ServiceMonitor"
    if (obj.get("apiVersion"), obj.get("kind")) != ("monitoring.coreos.com/v1", expected_kind):
        raise CaptureError("ServiceMonitor GET returned the wrong GVK")
    meta = obj.get("metadata", {})
    if not isinstance(meta, dict) or (meta.get("namespace"), meta.get("name")) != (NAMESPACE, RESOURCE_NAME):
        raise CaptureError("ServiceMonitor GET returned the wrong identity")
    if (not isinstance(meta.get("uid"), str) or not meta.get("uid")
            or not isinstance(meta.get("resourceVersion"), str) or not meta.get("resourceVersion")):
        raise CaptureError("ServiceMonitor response lacks UID or resourceVersion")
    return {"status": "present", "httpStatus": 200, "uid": meta["uid"], "resourceVersion": meta["resourceVersion"]}


def prerequisite_receipt_valid(code: int, stdout: bytes, expected: str) -> dict:
    """Validate current prerequisites-met receipt field names and exact fact evidence."""
    try:
        stmt = json.loads(stdout)
        predicate = stmt["predicate"]
        if predicate.get("predicateName") != "prerequisites-met":
            raise KeyError
        evidence = predicate["evidence"]["prerequisites"]
        facts = evidence["facts"]
        summary = evidence["summary"]
    except (UnicodeDecodeError, json.JSONDecodeError, KeyError, TypeError):
        raise CaptureError("Scout output is not the current typed prerequisites receipt") from None
    if expected not in ("missing", "present") or not isinstance(facts, list) or len(facts) != 1:
        raise CaptureError("Scout prerequisite receipt did not validate the single declared CRD fact")
    fact = facts[0]
    if (not isinstance(fact, dict) or fact.get("kind") != "CRD" or fact.get("name") != CRD_NAME
            or fact.get("status") != expected):
        raise CaptureError("Scout prerequisite receipt did not validate the single declared CRD fact")
    categories = ("required", "present", "missing", "inconclusive")
    if not isinstance(summary, dict) or any(type(summary.get(key)) is not int for key in categories):
        raise CaptureError("Scout prerequisite receipt summary is malformed")
    counts = {key: summary[key] for key in categories}
    expected_counts = {"required": 1, "present": int(expected == "present"),
                       "missing": int(expected == "missing"), "inconclusive": 0}
    if counts != expected_counts or counts["required"] != counts["present"] + counts["missing"] + counts["inconclusive"]:
        raise CaptureError("Scout prerequisite receipt summary disagrees with its exact fact")
    verdict = predicate.get("verdict")
    expected_verdict = "PASS" if expected == "present" else "BLOCK"
    if verdict != expected_verdict:
        raise CaptureError("Scout prerequisite receipt verdict does not match the declared CRD fact")
    return {"verdict": verdict, "factStatus": expected, "factCount": 1}


def owned_cluster_name() -> str:
    import uuid
    name = "scout-pre01-" + inv04.datetime.now(inv04.timezone.utc).strftime("%Y%m%d%H%M%S") + "-" + uuid.uuid4().hex[:8]
    if len(name + "-control-plane") > 63 or not re.fullmatch(r"scout-pre01-[0-9a-z-]+", name):
        raise CaptureError("generated owned-cluster name is invalid")
    return name


def owns_cluster(marker: dict, name: str, pid: int) -> bool:
    return (isinstance(marker, dict) and marker.get("schema") == "pre01-owned-cluster-marker.v1"
            and marker.get("clusterName") == name and marker.get("ownerPid") == pid
            and re.fullmatch(r"scout-pre01-[0-9a-z-]+", name) is not None
            and len(name + "-control-plane") <= 63)


def observer_setup_manifest() -> bytes:
    """Minimal RBAC: exact-name CRD GET, namespace ServiceMonitor GET, discovery GET."""
    docs = [
        {"apiVersion": "v1", "kind": "Namespace", "metadata": {"name": NAMESPACE}},
        {"apiVersion": "v1", "kind": "ServiceAccount", "metadata": {"name": "pre01-observer", "namespace": NAMESPACE}},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
         "metadata": {"name": "pre01-observer", "namespace": NAMESPACE},
         "rules": [{"apiGroups": ["monitoring.coreos.com"], "resources": ["servicemonitors"], "resourceNames": [RESOURCE_NAME], "verbs": ["get"]}]},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
         "metadata": {"name": "pre01-observer", "namespace": NAMESPACE},
         "subjects": [{"kind": "ServiceAccount", "name": "pre01-observer", "namespace": NAMESPACE}],
         "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "pre01-observer"}},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole",
         "metadata": {"name": "pre01-observer-discovery"},
         "rules": [
             {"apiGroups": ["apiextensions.k8s.io"], "resources": ["customresourcedefinitions"],
              "resourceNames": [CRD_NAME], "verbs": ["get"]},
             {"nonResourceURLs": ["/api", "/api/*", "/apis", "/apis/monitoring.coreos.com", "/apis/monitoring.coreos.com/v1"], "verbs": ["get"]},
         ]},
        {"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding",
         "metadata": {"name": "pre01-observer-discovery"},
         "subjects": [{"kind": "ServiceAccount", "name": "pre01-observer", "namespace": NAMESPACE}],
         "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "pre01-observer-discovery"}},
    ]
    return ("\n---\n".join(json.dumps(doc, sort_keys=True, separators=(",", ":")) for doc in docs) + "\n").encode()


def api_get(server: str, ca: bytes, token: str, path: str, timeout: float = 15.0) -> tuple[int, bytes, float]:
    check_server(server)
    check_api_path(path)
    request = urllib.request.Request(server.rstrip("/") + path,
        headers={"Authorization": "Bearer " + token, "Accept": "application/json"})
    context = ssl.create_default_context(cadata=ca.decode("ascii"))
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), inv04._NoRedirect,
                                         urllib.request.HTTPSHandler(context=context))
    started = time.monotonic(); deadline = started + timeout
    try:
        with opener.open(request, timeout=timeout) as response:
            status, body = response.status, inv04._read_response_body(response, deadline)
    except urllib.error.HTTPError as error:
        try:
            status, body = error.code, inv04._read_response_body(error, deadline)
        finally:
            error.close()
    if len(body) > MAX_BODY:
        raise CaptureError("raw Kubernetes response exceeded the 4 MiB limit")
    return status, body, time.monotonic() - started


def capture_api(phase: str, path: str, name: str, server: str, ca: bytes, token: str,
                private: tuple[bytes, ...], output: Path, records: list[dict]) -> dict | None:
    record = {"phase": phase, "method": "GET", "path": path, "startedAt": inv04.utc_now()}
    body = b""; status = 0; elapsed = 0.0
    safe_to_write = True
    try:
        status, body, elapsed = api_get(server, ca, token, path)
        if not credentials_absent(body, token, private):
            safe_to_write = False
            record["rawWithheldCredentials"] = True
            raise CaptureError("raw API response contains private credential material")
        inv04._write(output / name, body)
        record["rawFile"] = name
        record.update({"httpStatus": status, "elapsedSeconds": elapsed, "rawBytes": len(body), "rawSha256": sha256(body)})
        record["validation"] = validate_raw(path, status, body, phase)
        return record["validation"]
    except Exception as error:
        # Always retain the exact response available, including a rejected 403/500 body.
        if safe_to_write and not (output / name).exists():
            try:
                inv04._write(output / name, body)
                record["rawFile"] = name
            except OSError: pass
        record.update({"httpStatus": status, "elapsedSeconds": elapsed, "rawBytes": len(body),
                       "rawSha256": sha256(body), "error": str(error) if isinstance(error, CaptureError) else type(error).__name__})
        raise
    finally:
        record["endedAt"] = inv04.utc_now()
        records.append(record)


def run_apply(kubectl: str, admin: Path, cluster: str, manifest: Path, cache: Path,
              output: Path, phase: str, token: str, private: tuple[bytes, ...]) -> dict:
    started = inv04.utc_now()
    cache.mkdir(mode=0o700, parents=True, exist_ok=False)
    argv = [kubectl, "--kubeconfig", str(admin), "--context", "kind-" + cluster,
            "--cache-dir", str(cache), "apply", "-f", str(manifest)]
    code, stdout, stderr = inv04.run_bounded(argv, 45, {**os.environ, "KUBECONFIG": str(admin)}, max_output=MAX_OUTPUT)
    safe_stdout = credentials_absent(stdout, token, private)
    safe_stderr = credentials_absent(stderr, token, private)
    if safe_stdout: inv04._write(output / (phase + "-apply.stdout.txt"), stdout)
    if safe_stderr: inv04._write(output / (phase + "-apply.stderr.txt"), stderr)
    expected = "missing-ServiceMonitor-GVK" if phase == "absent" else "created-ServiceMonitor"
    missing_gvk = re.search(rb'no matches for kind ["\']ServiceMonitor["\'] in version ["\']monitoring\.coreos\.com/v1["\']', stderr) is not None
    denied_or_transport = re.search(rb"forbidden|unauthorized|timed out|timeout|connection refused|unable to connect|no route to host", stderr, re.I) is not None
    created_exact = re.search(rb"(?m)^servicemonitor\.monitoring\.coreos\.com/kube-prometheus-stack-kube-state-metrics created\s*$", stdout) is not None
    accepted = ((code != 0 and missing_gvk and not denied_or_transport) if phase == "absent" else
                (code == 0 and created_exact)) and safe_stdout and safe_stderr
    return {"phase": phase, "exitCode": code, "stdoutFile": phase + "-apply.stdout.txt" if safe_stdout else None,
            "stderrFile": phase + "-apply.stderr.txt" if safe_stderr else None, "stdoutWithheldCredentials": not safe_stdout,
            "stderrWithheldCredentials": not safe_stderr, "stdoutSha256": sha256(stdout),
            "stderrSha256": sha256(stderr), "startedAt": started, "endedAt": inv04.utc_now(),
            "expected": expected, "accepted": accepted, "causalFailureEvidence": missing_gvk,
            "healthProven": False,
            "meaning": "dependent resource registration only; no controller or target health claim"}


def retain_safe_command_output(output: Path, label: str, stdout: bytes, stderr: bytes,
                               token: str, private: tuple[bytes, ...], can_check: bool = True) -> dict:
    record = {"stdoutBytes": len(stdout), "stderrBytes": len(stderr),
              "stdoutSha256": sha256(stdout), "stderrSha256": sha256(stderr)}
    for stream, data in (("stdout", stdout), ("stderr", stderr)):
        safe = can_check and credentials_absent(data, token, private)
        record[stream + "WithheldCredentials"] = not safe
        if safe:
            filename = label + "." + stream + ".txt"
            inv04._write(output / filename, data)
            record[stream + "File"] = filename
        else:
            record[stream + "File"] = None
    return record


def run_scout_receipt(scout: Path, admin: Path, observer: Path, phase: str,
                      source_revision: str, expected_status: str, output: Path,
                      token: str, private: tuple[bytes, ...]) -> dict:
    prereq = output / (phase + "-prerequisites.json")
    inv04._write(prereq, (json.dumps({"requiredCRDs": [CRD_NAME]}, separators=(",", ":")) + "\n").encode())
    stdout_path, stderr_path = phase + "-scout-receipt.json", phase + "-scout-stderr.txt"
    env = {**os.environ, "KUBECONFIG": str(observer)}
    argv = [str(scout), "receipt", "verify", "--prerequisites", str(prereq), "--scope", "namespace/" + NAMESPACE,
            "--predicate", "prerequisites-met", "--format", "json"]
    code, stdout, stderr = inv04.run_bounded(argv, 45, env, max_output=MAX_OUTPUT)
    if not credentials_absent(stdout, token, private) or not credentials_absent(stderr, token, private):
        raise CaptureError("Scout receipt output contains private credential material")
    inv04._write(output / stdout_path, stdout); inv04._write(output / stderr_path, stderr)
    validation = prerequisite_receipt_valid(code, stdout, expected_status)
    return {"phase": phase, "sourceRevision": source_revision, "binarySha256": sha256(scout.read_bytes()),
            "exitCode": code, "stdoutFile": stdout_path, "stderrFile": stderr_path,
            "stdoutSha256": sha256(stdout), "stderrSha256": sha256(stderr),
            "validation": validation, "isRawModelEvidence": False}


def capture(args, sources: dict, out: Path, shared: Path, scout: Path) -> int:
    started = inv04.utc_now(); shared_before = sha256(shared.read_bytes())
    source_repo = args.source_checkout.expanduser().resolve(strict=True)
    git = inv04._command("git")
    code, rev, _ = inv04.run_bounded([git, "-C", str(HERE.parents[1]), "rev-parse", "HEAD"], 5, max_output=4096)
    if code: raise CaptureError("could not identify capture source revision")
    capture_revision = rev.decode("ascii", "replace").strip()
    if not re.fullmatch(r"[0-9a-f]{40,64}", capture_revision): raise CaptureError("capture source revision is invalid")
    code, status, _ = inv04.run_bounded([git, "-C", str(HERE.parents[1]), "status", "--porcelain"], 5, max_output=16384)
    if code or status.strip(): raise CaptureError("capture checkout must be clean before live execution")
    inv04.require_local_docker(dict(os.environ))
    inv04.require_kind_version(inv04._call("kind", ["version"], 10).decode().strip())
    inv04.verified_node_repo_digest(inv04._call("docker", ["image", "inspect", inv04.NODE_IMAGE], 15))
    cluster = owned_cluster_name()
    admin_dir = Path(tempfile.mkdtemp(prefix="scout-pre01-private-")); os.chmod(admin_dir, 0o700)
    admin, observer = admin_dir / "admin.kubeconfig", admin_dir / "observer.kubeconfig"
    env = {**os.environ, "KUBECONFIG": str(admin)}
    create_attempted = False; cleanup_ok = False; private_hashes = {}; private_before_cleanup = {}; errors = []
    records = []; operations = []; scout_receipts = []; server = ""; ca = b""; token = ""
    sm_file, crd_file = out / "servicemonitor.normalized.yaml", out / "servicemonitor-crd.normalized.yaml"
    setup_file = out / "observer-setup.jsonl"
    inv04._write(setup_file, observer_setup_manifest())
    marker = {"schema": "pre01-owned-cluster-marker.v1", "clusterName": cluster, "ownerPid": os.getpid(),
              "sourceRevision": capture_revision, "startedAt": started}
    marker_bytes = (json.dumps(marker, sort_keys=True, indent=2) + "\n").encode()
    inv04._write(out / "owned-cluster-marker.json", marker_bytes)
    marker_sha = sha256(marker_bytes)
    signal_state = inv04._arm_capture_signals()
    try:
        existing = inv04._call("kind", ["get", "clusters"], 15).decode().splitlines()
        if cluster in existing: raise CaptureError("refusing to reuse an existing cluster")
        create_attempted = True
        create_started = inv04.utc_now()
        code, stdout, stderr = inv04.run_bounded([inv04._command("kind"), "create", "cluster", "--name", cluster,
            "--image", inv04.NODE_IMAGE, "--kubeconfig", str(admin), "--wait", "120s"], 180, env)
        try:
            admin_bytes_for_check = admin.read_bytes()
            admin_doc_for_check = yaml.safe_load(admin_bytes_for_check)
            if not isinstance(admin_doc_for_check, dict): raise CaptureError("kind admin config is malformed")
            create_private = private_material(admin_doc_for_check, admin_bytes_for_check)
            can_check_create_output = True
        except (OSError, yaml.YAMLError, CaptureError):
            create_private = ()
            can_check_create_output = False
        create_output = retain_safe_command_output(out, "kind-create", stdout, stderr, "", create_private,
                                                   can_check=can_check_create_output)
        operations.append({"operation": "create-owned-kind", "exitCode": code, "startedAt": create_started,
                           "endedAt": inv04.utc_now(), **create_output})
        if code: raise CaptureError("owned kind cluster creation failed")
        if not admin.exists(): raise CaptureError("kind did not create the private admin kubeconfig")
        os.chmod(admin, 0o600)
        kubectl = inv04._command("kubectl")
        code, _, _ = inv04.run_bounded([kubectl, "--kubeconfig", str(admin), "--context", "kind-" + cluster,
                                        "apply", "-f", str(setup_file)], 45, env, max_output=MAX_OUTPUT)
        if code: raise CaptureError("could not create the bounded read-only observer")
        raw_bytes = inv04._call("kubectl", ["--kubeconfig", str(admin), "--context", "kind-" + cluster,
            "config", "view", "--minify", "--flatten", "--raw", "-o", "json"], 15, env)
        raw = json.loads(raw_bytes)
        cluster_cfg = raw["clusters"][0]["cluster"]
        server = cluster_cfg["server"]; ca64 = cluster_cfg["certificate-authority-data"]
        check_server(server); ca = base64.b64decode(ca64, validate=True)
        token = inv04._call("kubectl", ["--kubeconfig", str(admin), "--context", "kind-" + cluster,
            "create", "token", "pre01-observer", "-n", NAMESPACE, "--duration=900s"], 15, env).decode().strip()
        if not token or any(c.isspace() for c in token): raise CaptureError("could not create bounded observer token")
        inv04._write(observer, observer_kubeconfig(server, ca64, token)); os.chmod(observer, 0o600)
        admin_bytes = admin.read_bytes(); observer_bytes = observer.read_bytes()
        private_hashes = {"admin": sha256(admin_bytes), "observer": sha256(observer_bytes)}
        private = private_material(raw, admin_bytes) + (observer_bytes, token.encode())
        del raw_bytes, raw
        for phase in ("absent", "present"):
            phase_validations = {}
            if phase == "absent":
                for path, label in zip(API_PATHS, ("crd", "discovery", "servicemonitor")):
                    phase_validations[label] = capture_api(phase, path, phase + "-" + label + ".json", server, ca, token,
                                                           private, out, records)
                scout_receipts.append(run_scout_receipt(scout, admin, observer, phase, args.scout_source_revision, "missing", out, token, private))
                operation = run_apply(kubectl, admin, cluster, sm_file, out / ("cache-" + phase), out, phase, token, private)
                operations.append(operation)
                if not operation["accepted"]: errors.append("dependent ServiceMonitor apply had unexpected absent result")
                for path, label in ((API_PATHS[0], "crd"), (API_PATHS[2], "servicemonitor")):
                    capture_api("absent", path, "absent-after-apply-" + label + ".json", server, ca, token,
                                private, out, records)
                # Install only the real, pinned chart CRD. `create` avoids a client-side
                # last-applied annotation on the large authored document.
                crd_started = inv04.utc_now()
                code, out_crd, err_crd = inv04.run_bounded([kubectl, "--kubeconfig", str(admin), "--context", "kind-" + cluster,
                    "create", "-f", str(crd_file)], 60, env, max_output=MAX_OUTPUT)
                crd_output = retain_safe_command_output(out, "crd-create", out_crd, err_crd, token, private)
                operations.append({"operation": "fixture-create-only-pinned-crd", "exitCode": code,
                                   "startedAt": crd_started, "endedAt": inv04.utc_now(),
                                   **crd_output})
                if code: raise CaptureError("could not create the pinned CRD fixture")
                wait_started = inv04.utc_now()
                code, wait_out, wait_err = inv04.run_bounded([kubectl, "--kubeconfig", str(admin), "--context", "kind-" + cluster,
                    "wait", "--for=condition=Established", "--timeout=60s", "crd/" + CRD_NAME], 70, env, max_output=MAX_OUTPUT)
                wait_output = retain_safe_command_output(out, "crd-established", wait_out, wait_err, token, private)
                operations.append({"operation": "wait-crd-established", "exitCode": code,
                                   "startedAt": wait_started, "endedAt": inv04.utc_now(),
                                   **wait_output})
                if code: raise CaptureError("pinned CRD did not become Established")
            else:
                for path, label in zip(API_PATHS[:2], ("crd", "discovery")):
                    phase_validations[label] = capture_api(phase, path, phase + "-" + label + ".json", server, ca, token,
                                                           private, out, records)
                scout_receipts.append(run_scout_receipt(scout, admin, observer, phase, args.scout_source_revision, "present", out, token, private))
                capture_api("present-before-apply", API_PATHS[2], "present-before-apply-servicemonitor.json",
                    server, ca, token, private, out, records)
                operation = run_apply(kubectl, admin, cluster, sm_file, out / ("cache-" + phase), out, phase, token, private)
                operations.append(operation)
                if not operation["accepted"]: errors.append("dependent ServiceMonitor apply had unexpected present result")
                phase_validations["servicemonitor"] = capture_api(phase, API_PATHS[2], phase + "-servicemonitor.json",
                    server, ca, token, private, out, records)
    except BaseException as error:
        errors.append(str(error) if isinstance(error, CaptureError) else type(error).__name__)
    finally:
        inv04._suppress_capture_signals(signal_state)
        for label, path in (("admin", admin), ("observer", observer)):
            try: private_before_cleanup[label] = sha256(path.read_bytes())
            except OSError: private_before_cleanup[label] = ""
            if label in private_hashes and private_before_cleanup[label] != private_hashes[label]:
                errors.append("private " + label + " kubeconfig changed before cleanup")
        try:
            marker_check = json.loads((out / "owned-cluster-marker.json").read_bytes())
            owned = create_attempted and owns_cluster(marker_check, cluster, os.getpid())
            clusters = inv04._call("kind", ["get", "clusters"], 15).decode().splitlines()
            if owned and cluster in clusters:
                inv04._call("kind", ["delete", "cluster", "--name", cluster, "--kubeconfig", str(admin)], 90, env)
            cleanup_ok = cluster not in inv04._call("kind", ["get", "clusters"], 15).decode().splitlines()
        except Exception:
            cleanup_ok = False
        shutil.rmtree(admin_dir, ignore_errors=True)
        inv04._restore_capture_signals(signal_state)
    shared_after = sha256(shared.read_bytes())
    if not cleanup_ok: errors.append("owned cluster cleanup was not verified")
    if shared_before != shared_after: errors.append("shared kubeconfig changed")
    if sha256(scout.read_bytes()) != args.expected_scout_sha256: errors.append("Scout executable changed during capture")
    provenance = {"schema": "pre01-crd-capture.v1", "captureSourceRevision": capture_revision,
        "sourceRevision": args.source_revision, "sourceFiles": pins_from_source(sources),
        "scoutSourceRevision": args.scout_source_revision, "scoutBinarySha256": sha256(scout.read_bytes()),
        "captureScriptSha256": sha256(Path(__file__).read_bytes()), "clusterName": cluster,
        "ownedClusterMarkerSha256": marker_sha, "startedAt": started, "endedAt": inv04.utc_now(),
        "apiObservations": records, "operations": operations, "derivedScoutReceipts": scout_receipts,
        "sharedKubeconfigSha256": {"before": shared_before, "after": shared_after},
        "privateKubeconfigSha256": {"before": private_hashes, "beforeCleanup": private_before_cleanup},
        "cleanupVerified": cleanup_ok, "atomicSnapshot": False, "errors": errors,
        "limitations": ["one owned kind cluster, serial absent/present phases; observations are sequential, not atomic",
            "only the pinned CRD is installed as setup; dependent registration is not controller reconciliation",
            "ServiceMonitor registration success and CRD Established do not prove operator or target health",
            "Scout receipts are derived prerequisites diagnostics and are not raw model evidence"]}
    inv04._write(out / "provenance.json", (json.dumps(provenance, sort_keys=True, indent=2) + "\n").encode())
    if errors: raise CaptureError("capture acceptance failed; inspect retained provenance.json")
    return 0


def pins_from_source(sources: dict) -> dict:
    return {"files": sources["sourceFiles"], "selectedServiceMonitor": {k: v for k, v in sources["selectedServiceMonitor"].items() if k != "normalizedYaml"},
            "selectedCRD": {k: v for k, v in sources["selectedCRD"].items() if k != "normalizedYaml"},
            "normalization": sources["normalization"], "normalizer": sources["normalizer"],
            "prerequisiteRecordChecked": sources["prerequisiteRecordChecked"]}


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--execute", action="store_true", help="required acknowledgement: creates/deletes one owned kind cluster")
    ap.add_argument("--source-checkout", type=Path, required=True)
    ap.add_argument("--source-revision", required=True)
    ap.add_argument("--expected-service-monitor-sha256", required=True)
    ap.add_argument("--expected-record-sha256", required=True)
    ap.add_argument("--expected-crd-sha256", required=True)
    ap.add_argument("--shared-kubeconfig", type=Path, required=True, help="read only for before/after integrity hashes")
    ap.add_argument("--expected-scout-sha256", required=True)
    ap.add_argument("--scout-source-revision", required=True)
    ap.add_argument("--scout-binary", type=Path, required=True)
    ap.add_argument("--expected-scout-revision", required=True)
    ap.add_argument("--output-dir", type=Path, required=True)
    args = ap.parse_args(argv)
    if not args.execute:
        ap.error("--execute is required; this helper creates and deletes one owned cluster")
    try:
        supplied = {SERVICE_MONITOR_PATH: args.expected_service_monitor_sha256,
                    RECORD_PATH: args.expected_record_sha256, CRD_PATH: args.expected_crd_sha256}
        if args.source_revision != SOURCE_REVISION:
            raise CaptureError("source revision must equal the reviewed pinned commit")
        if args.scout_source_revision != SCOUT_SOURCE_REVISION or args.expected_scout_revision != SCOUT_SOURCE_REVISION:
            raise CaptureError("Scout source revision differs from the reviewed executable revision")
        if args.expected_scout_sha256 != SCOUT_SHA256:
            raise CaptureError("Scout SHA-256 must equal the reviewed executable pin")
        for digest in (*supplied.values(), args.expected_scout_sha256):
            if not re.fullmatch(r"[0-9a-f]{64}", digest):
                raise CaptureError("a supplied SHA-256 pin is malformed")
        shared_arg = args.shared_kubeconfig.expanduser().absolute()
        scout_arg = args.scout_binary.expanduser().absolute()
        if shared_arg.is_symlink() or scout_arg.is_symlink():
            raise CaptureError("shared kubeconfig and Scout executable must not be symlinks")
        shared = shared_arg.resolve(strict=True)
        scout = scout_arg.resolve(strict=True)
        if not shared.is_file() or not scout.is_file() or not os.access(scout, os.X_OK):
            raise CaptureError("shared kubeconfig or Scout executable is invalid")
        verify_scout_binary(scout, args.expected_scout_sha256, args.scout_source_revision, args.expected_scout_revision)
        sources = extract_pinned_sources(args.source_checkout, args.source_revision, supplied)
        out = prepare_output(args.output_dir)
        inv04._write(out / "servicemonitor.normalized.yaml", sources["selectedServiceMonitor"]["normalizedYaml"])
        inv04._write(out / "servicemonitor-crd.normalized.yaml", sources["selectedCRD"]["normalizedYaml"])
        pins = {k: v for k, v in sources.items() if k not in ("selectedServiceMonitor", "selectedCRD")}
        pins["selectedServiceMonitor"] = {k: v for k, v in sources["selectedServiceMonitor"].items() if k != "normalizedYaml"}
        pins["selectedCRD"] = {k: v for k, v in sources["selectedCRD"].items() if k != "normalizedYaml"}
        inv04._write(out / "source-pins.json", (json.dumps(pins, sort_keys=True, indent=2) + "\n").encode())
        return capture(args, sources, out, shared, scout)
    except (CaptureError, OSError, ValueError) as error:
        try:
            candidate = locals().get("out")
            if isinstance(candidate, Path) and candidate.is_dir() and not (candidate / "provenance.json").exists():
                inv04._write(candidate / "capture-failure.json", (json.dumps({"schema": "pre01-crd-failure.v1",
                    "error": str(error), "recordedAt": inv04.utc_now()}, sort_keys=True, indent=2) + "\n").encode())
        except OSError:
            pass
        print(str(error), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
