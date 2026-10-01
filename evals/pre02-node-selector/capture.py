#!/usr/bin/env python3
"""Capture a bounded before/after node-selector prerequisite on one owned kind cluster.

This is a source-preparation helper. It does not run unless --execute is supplied.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import importlib.util
import json
import math
import os
from pathlib import Path
import re
import shutil
import signal
import ssl
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
REPO = HERE.parents[1]
INV04_PATH = HERE.parent / "inv04-rbac" / "capture.py"
_spec = importlib.util.spec_from_file_location("pre02_inv04_capture", INV04_PATH)
inv04 = importlib.util.module_from_spec(_spec)
assert _spec and _spec.loader
_spec.loader.exec_module(inv04)

RUL04_PATH = HERE.parent / "rul04-image-identity" / "capture.py"
_rul_spec = importlib.util.spec_from_file_location("pre02_rul04_capture", RUL04_PATH)
rul04 = importlib.util.module_from_spec(_rul_spec)
assert _rul_spec and _rul_spec.loader
_rul_spec.loader.exec_module(rul04)

NAMESPACE = "pre02-node-selector"
POD_NAME = "scout-pre02-selector"
CONTAINER_NAME = "pause"
IMAGE = "registry.k8s.io/pause:3.10"
LABEL_KEY = "scout-pre02-zone"
LABEL_VALUE = "fixture-zone"
MAX_BODY = 2 * 1024 * 1024
MAX_COMMAND_OUTPUT = 2 * 1024 * 1024
EXECUTION_SECONDS = 120
PREREQUISITE_WAIT_SECONDS = 30
CLEANUP_SECONDS = 30
TOTAL_SECONDS = 150
API_POD = f"/api/v1/namespaces/{NAMESPACE}/pods/{POD_NAME}"
API_NODES = "/api/v1/nodes"
API_EVENTS_PREFIX = f"/api/v1/namespaces/{NAMESPACE}/events?fieldSelector=involvedObject.uid="


class CaptureError(RuntimeError):
    pass


class CaptureInterrupted(CaptureError):
    pass


MANIFEST = (json.dumps({"apiVersion": "v1", "kind": "Namespace",
                        "metadata": {"name": NAMESPACE}}, sort_keys=True, separators=(",", ":")) + "\n---\n" +
           json.dumps({"apiVersion": "v1", "kind": "ServiceAccount",
                       "metadata": {"name": "pre02-observer", "namespace": NAMESPACE}},
                      sort_keys=True, separators=(",", ":")) + "\n---\n" +
           json.dumps({"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
                       "metadata": {"name": "pre02-observer", "namespace": NAMESPACE},
                       "rules": [{"apiGroups": [""], "resources": ["pods"],
                                  "verbs": ["get"]},
                                 {"apiGroups": [""], "resources": ["events"],
                                  "verbs": ["list"]}]}, sort_keys=True, separators=(",", ":")) + "\n---\n" +
           json.dumps({"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
                       "metadata": {"name": "pre02-observer", "namespace": NAMESPACE},
                       "subjects": [{"kind": "ServiceAccount", "name": "pre02-observer",
                                     "namespace": NAMESPACE}],
                       "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role",
                                   "name": "pre02-observer"}}, sort_keys=True, separators=(",", ":")) + "\n---\n" +
           json.dumps({"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole",
                       "metadata": {"name": "pre02-observer"},
                       "rules": [{"apiGroups": [""], "resources": ["nodes"],
                                  "verbs": ["list"]}]}, sort_keys=True, separators=(",", ":")) + "\n---\n" +
           json.dumps({"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding",
                       "metadata": {"name": "pre02-observer"},
                       "subjects": [{"kind": "ServiceAccount", "name": "pre02-observer",
                                     "namespace": NAMESPACE}],
                       "roleRef": {"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole",
                                   "name": "pre02-observer"}}, sort_keys=True, separators=(",", ":")) + "\n---\n" +
           json.dumps({"apiVersion": "v1", "kind": "Pod",
                       "metadata": {"name": POD_NAME, "namespace": NAMESPACE,
                                    "labels": {"app": POD_NAME}},
                       "spec": {"restartPolicy": "Never", "nodeSelector": {LABEL_KEY: LABEL_VALUE},
                                "containers": [{"name": CONTAINER_NAME, "image": IMAGE,
                                                "imagePullPolicy": "Never"}]}},
                      sort_keys=True, separators=(",", ":")) + "\n").encode()


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def strict_json(data: bytes, label: str):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise CaptureError(label + " contains a duplicate JSON key")
            result[key] = value
        return result
    def constant(_value):
        raise CaptureError(label + " contains a non-finite JSON value")
    def finite(value):
        number = float(value)
        if not math.isfinite(number):
            raise CaptureError(label + " contains a non-finite JSON value")
        return number
    try:
        return json.loads(data.decode("utf-8", "strict"), object_pairs_hook=unique,
                          parse_constant=constant, parse_float=finite)
    except (json.JSONDecodeError, UnicodeDecodeError):
        raise CaptureError(label + " is not valid UTF-8 JSON") from None


def nonempty(value) -> bool:
    return isinstance(value, str) and bool(value.strip())


def _identity(obj: dict, kind: str, name: str | None = None) -> tuple[dict, dict]:
    if not isinstance(obj, dict) or obj.get("apiVersion") != "v1" or obj.get("kind") != kind:
        raise CaptureError("raw API response has unexpected Kubernetes kind/version")
    metadata = obj.get("metadata")
    if not isinstance(metadata, dict) or not nonempty(metadata.get("uid")) or not nonempty(metadata.get("resourceVersion")):
        raise CaptureError("raw API object lacks UID/resourceVersion")
    if name is not None and metadata.get("name") != name:
        raise CaptureError("raw API object name differs from the authored target")
    return obj, metadata


def validate_pod(body: bytes, phase: str) -> dict:
    obj = strict_json(body, "Pod response")
    if not isinstance(obj, dict):
        raise CaptureError("Pod response must be a JSON object")
    pod, metadata = _identity(obj, "Pod", POD_NAME)
    if metadata.get("namespace") != NAMESPACE:
        raise CaptureError("Pod response namespace differs from authored namespace")
    spec = pod.get("spec")
    if not isinstance(spec, dict):
        raise CaptureError("Pod spec is missing or malformed")
    containers = spec.get("containers")
    if (spec.get("restartPolicy") != "Never" or spec.get("nodeSelector") != {LABEL_KEY: LABEL_VALUE}
            or not isinstance(containers, list) or len(containers) != 1 or not isinstance(containers[0], dict)
            or containers[0].get("name") != CONTAINER_NAME or containers[0].get("image") != IMAGE
            or containers[0].get("imagePullPolicy") != "Never"):
        raise CaptureError("Pod spec differs from the exact selector fixture")
    status = pod.get("status")
    if not isinstance(status, dict):
        raise CaptureError("Pod status is missing or malformed")
    conditions = status.get("conditions")
    if not isinstance(conditions, list) or any(not isinstance(item, dict) for item in conditions):
        raise CaptureError("Pod conditions are missing or malformed")
    scheduled = [item for item in conditions if item.get("type") == "PodScheduled"]
    if len(scheduled) != 1:
        raise CaptureError("Pod must have exactly one PodScheduled condition")
    condition = scheduled[0]
    if phase == "before":
        if (condition.get("status") != "False" or condition.get("reason") != "Unschedulable"
                or not nonempty(condition.get("message"))):
            raise CaptureError("before Pod response does not prove Unschedulable")
        if spec.get("nodeName"):
            raise CaptureError("before Pod is already bound to a node")
        message = condition["message"].lower()
        if "selector" not in message and "affinity" not in message:
            raise CaptureError("before Pod condition does not identify node selector/affinity")
    elif phase == "after":
        if condition.get("status") != "True" or not nonempty(spec.get("nodeName")):
            raise CaptureError("after Pod response does not prove scheduling to a node")
    else:
        raise CaptureError("unexpected Pod phase")
    return {"uid": metadata["uid"], "resourceVersion": metadata["resourceVersion"],
            "spec": spec, "nodeName": spec.get("nodeName"), "podScheduled": condition,
            "phase": phase}


def validate_nodes(body: bytes, phase: str, expected_node_name: str | None = None) -> dict:
    obj = strict_json(body, "NodeList response")
    if (not isinstance(obj, dict) or obj.get("apiVersion") != "v1" or obj.get("kind") != "NodeList"
            or not isinstance(obj.get("items"), list)):
        raise CaptureError("raw node response is not a NodeList")
    meta = obj.get("metadata")
    if not isinstance(meta, dict) or not nonempty(meta.get("resourceVersion")):
        raise CaptureError("NodeList lacks collection resourceVersion")
    nodes = []
    names = set()
    for node in obj["items"]:
        if not isinstance(node, dict):
            raise CaptureError("NodeList contains a non-object item")
        if node.get("apiVersion", "v1") != "v1" or node.get("kind", "Node") != "Node":
            raise CaptureError("NodeList item has contradictory Kubernetes type metadata")
        node_meta = node.get("metadata")
        if (not isinstance(node_meta, dict) or not nonempty(node_meta.get("uid"))
                or not nonempty(node_meta.get("resourceVersion"))):
            raise CaptureError("NodeList item lacks UID/resourceVersion")
        name = node_meta.get("name")
        labels = node_meta.get("labels")
        if not nonempty(name) or name in names or not isinstance(labels, dict):
            raise CaptureError("NodeList contains invalid/duplicate node identity or labels")
        names.add(name)
        nodes.append({"name": name, "uid": node_meta["uid"], "resourceVersion": node_meta["resourceVersion"],
                      "labels": labels})
    if not nodes:
        raise CaptureError("NodeList is empty; selector prerequisite cannot be established")
    matches = [node for node in nodes if node["labels"].get(LABEL_KEY) == LABEL_VALUE]
    if phase == "before":
        if matches:
            raise CaptureError("before NodeList already has a node satisfying the authored selector")
    elif phase == "after":
        if len(matches) != 1 or matches[0]["name"] != expected_node_name:
            raise CaptureError("after NodeList lacks the unique label on the scheduled node")
    else:
        raise CaptureError("unexpected NodeList phase")
    return {"resourceVersion": meta["resourceVersion"], "nodes": nodes,
            "matchingNodeNames": [node["name"] for node in matches]}


def validate_events(body: bytes, pod_uid: str, phase: str) -> dict:
    obj = strict_json(body, "EventList response")
    if (not isinstance(obj, dict) or obj.get("apiVersion") != "v1" or obj.get("kind") != "EventList"
            or not isinstance(obj.get("items"), list)):
        raise CaptureError("raw event response is not an EventList")
    meta = obj.get("metadata")
    if not isinstance(meta, dict) or not nonempty(meta.get("resourceVersion")):
        raise CaptureError("EventList lacks collection resourceVersion")
    correlated = []
    for event in obj["items"]:
        if not isinstance(event, dict):
            raise CaptureError("EventList contains a malformed item")
        event_meta = event.get("metadata")
        involved = event.get("involvedObject")
        if not isinstance(event_meta, dict) or not isinstance(involved, dict):
            raise CaptureError("Event identity or involved object is malformed")
        if involved.get("uid") != pod_uid:
            continue
        if (involved.get("apiVersion") != "v1" or involved.get("kind") != "Pod"
                or involved.get("name") != POD_NAME or involved.get("namespace") != NAMESPACE):
            raise CaptureError("event UID matches but Pod identity fields contradict the capture")
        if not nonempty(event_meta.get("name")) or event_meta.get("namespace") != NAMESPACE:
            raise CaptureError("UID-correlated Event lacks name/namespace")
        correlated.append(event)
    if phase == "before":
        failed = [event for event in correlated if event.get("type") == "Warning"
                  and event.get("reason") == "FailedScheduling"]
        if not failed:
            raise CaptureError("before EventList lacks a UID-correlated FailedScheduling event")
        messages = [event.get("message") for event in failed]
        if any(not isinstance(message, str) for message in messages):
            raise CaptureError("FailedScheduling event message is malformed")
        messages = [message.lower() for message in messages]
        if not any(("node(s) didn't match pod's node affinity/selector" in message
                    or "didn't match pod's node affinity/selector" in message)
                   for message in messages if isinstance(message, str)):
            raise CaptureError("FailedScheduling event does not identify a node selector/affinity mismatch")
        if any(any(marker in message for marker in ("insufficient cpu", "insufficient memory", "untolerated taint"))
               for message in messages if isinstance(message, str)):
            raise CaptureError("scheduler event contains a different resource/taint failure")
        return {"resourceVersion": meta["resourceVersion"], "uidCorrelatedEventCount": len(correlated),
                "failedSchedulingEventCount": len(failed), "phase": phase,
                "matchedPodUID": pod_uid}
    if phase == "after":
        # Old FailedScheduling events may remain; the current PodScheduled=True
        # and nodeName evidence controls the after-state conclusion.
        return {"resourceVersion": meta["resourceVersion"], "uidCorrelatedEventCount": len(correlated),
                "staleFailedSchedulingEventCount": sum(1 for event in correlated
                    if event.get("type") == "Warning" and event.get("reason") == "FailedScheduling"),
                "phase": phase, "matchedPodUID": pod_uid,
                "historicalEventsNotTreatedAsCurrentFailure": True}
    raise CaptureError("unexpected event phase")


def validate_transition(before_pod: dict, before_nodes: dict,
                        after_pod: dict, after_nodes: dict) -> dict:
    if before_pod.get("uid") != after_pod.get("uid"):
        raise CaptureError("Pod UID changed across the selector-only transition")
    before_spec, after_spec = dict(before_pod.get("spec", {})), dict(after_pod.get("spec", {}))
    if before_spec.get("nodeName"):
        raise CaptureError("before Pod spec unexpectedly has a node binding")
    after_binding = after_spec.pop("nodeName", None)
    before_spec.pop("nodeName", None)
    if before_spec != after_spec or after_binding != after_pod.get("nodeName"):
        raise CaptureError("Pod spec changed beyond the legitimate scheduler node binding")
    first = {node["name"]: node for node in before_nodes["nodes"]}
    last = {node["name"]: node for node in after_nodes["nodes"]}
    if set(first) != set(last):
        raise CaptureError("Node membership changed during the selector-only transition")
    changed = []
    for name, old in first.items():
        new = last[name]
        if old["uid"] != new["uid"]:
            raise CaptureError("Node UID changed during the selector-only transition")
        old_labels, new_labels = dict(old["labels"]), dict(new["labels"])
        old_selector = old_labels.pop(LABEL_KEY, None)
        new_selector = new_labels.pop(LABEL_KEY, None)
        if old_labels != new_labels:
            raise CaptureError("a node label besides the fixture selector changed")
        if old_selector != new_selector:
            changed.append((name, old_selector, new_selector))
    if changed != [(after_pod["nodeName"], None, LABEL_VALUE)]:
        raise CaptureError("the owned transition did not add only the selector label to the scheduled node")
    if after_nodes["matchingNodeNames"] != [after_pod["nodeName"]]:
        raise CaptureError("scheduled node does not uniquely satisfy the authored selector")
    return {"podUIDUnchanged": True, "podSpecUnchangedExceptSchedulerNodeName": True,
            "nodeMembershipUnchanged": True, "onlySelectorLabelChanged": True,
            "scheduledNode": after_pod["nodeName"],
            "conclusion": "selector prerequisite satisfied; scheduling is not readiness or application health"}


def validate_api_path(path: str, pod_uid: str | None = None) -> None:
    valid = {API_POD, API_NODES}
    if pod_uid:
        valid.add(API_EVENTS_PREFIX + urllib.parse.quote(pod_uid, safe=""))
    if path not in valid:
        raise CaptureError("API path is outside the PRE-02 read allowlist")


def check_server(server: str) -> None:
    parsed = urllib.parse.urlparse(server)
    if (parsed.scheme != "https" or parsed.hostname not in ("127.0.0.1", "localhost", "::1")
            or not parsed.port or parsed.username or parsed.password or parsed.path or parsed.query or parsed.fragment):
        raise CaptureError("refusing a non-loopback or malformed Kubernetes endpoint")


def observer_config(server: str, ca_data: str, token: str) -> bytes:
    check_server(server)
    if not ca_data or not token or any(char.isspace() for char in token):
        raise CaptureError("observer connection material is incomplete")
    value = {"apiVersion": "v1", "kind": "Config", "current-context": "pre02-observer",
             "clusters": [{"name": "pre02-owned", "cluster": {"server": server,
                           "certificate-authority-data": ca_data}}],
             "users": [{"name": "pre02-observer", "user": {"token": token}}],
             "contexts": [{"name": "pre02-observer", "context": {"cluster": "pre02-owned",
                            "user": "pre02-observer", "namespace": NAMESPACE}}]}
    return (json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n").encode()


def credentials_absent(data: bytes, token: str, private: tuple[bytes, ...]) -> bool:
    return (not token or token.encode() not in data) and all(not item or item not in data for item in private)


def cluster_name() -> str:
    name = "scout-pre02-" + time.strftime("%Y%m%d%H%M%S", time.gmtime()) + "-" + uuid.uuid4().hex[:10]
    if not re.fullmatch(r"scout-pre02-[0-9a-z-]+", name) or len(name + "-control-plane") > 63:
        raise CaptureError("generated cluster name is invalid")
    return name


def owns_cluster(marker: dict, name: str, pid: int) -> bool:
    return (isinstance(marker, dict) and marker.get("schema") == "pre02-owned-cluster-marker.v1"
            and marker.get("clusterName") == name and marker.get("ownerPid") == pid
            and re.fullmatch(r"scout-pre02-[0-9a-z-]+", name) is not None
            and len(name + "-control-plane") <= 63)


def fetch_api(server: str, ca_pem: bytes, token: str, path: str,
              *, timeout: float = 10.0, opener_factory=None) -> tuple[int, bytes, float]:
    event_uid = urllib.parse.unquote(path[len(API_EVENTS_PREFIX):]) if path.startswith(API_EVENTS_PREFIX) else None
    validate_api_path(path, event_uid)
    check_server(server)
    url = server.rstrip("/") + path
    request = urllib.request.Request(url, headers={"Authorization": "Bearer " + token,
        "Accept": "application/json"})
    if opener_factory:
        opener = opener_factory()
    else:
        ctx = ssl.create_default_context(cadata=ca_pem.decode("ascii"))
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), rul04.inv04._NoRedirect,
                                             urllib.request.HTTPSHandler(context=ctx))
    began = time.monotonic()
    deadline = began + timeout
    try:
        with opener.open(request, timeout=timeout) as response:
            status = response.status
            body = rul04.inv04._read_response_body(response, deadline)
    except urllib.error.HTTPError as error:
        try:
            status = error.code
            body = rul04.inv04._read_response_body(error, deadline)
        finally:
            error.close()
    except Exception as error:
        raise CaptureError("bounded Kubernetes API request failed: " + type(error).__name__) from None
    if len(body) > MAX_BODY:
        raise CaptureError("Kubernetes API response exceeded the body bound")
    return status, body, time.monotonic() - began


def capture_api(out: Path, phase: str, name: str, path: str, server: str, ca: bytes,
                token: str, pod_uid: str | None, private: tuple[bytes, ...], records: list[dict],
                *, api_reader=None, deadline: float,
                expected_node_name: str | None = None) -> tuple[bytes, dict]:
    validate_api_path(path, pod_uid)
    if time.monotonic() >= deadline:
        raise CaptureError("capture deadline expired before API read")
    started_at, began = inv04.utc_now(), time.monotonic()
    record = {"phase": phase, "method": "GET", "path": path, "startedAt": started_at,
              "rawFile": name, "validation": None}
    try:
        read = fetch_api if api_reader is None else api_reader
        status, body, elapsed = read(server, ca, token, path,
                                     timeout=min(10.0, deadline - time.monotonic()))
        if len(body) > MAX_BODY:
            raise CaptureError("Kubernetes API response exceeded the body bound")
        record.update({"httpStatus": status, "elapsedSeconds": elapsed, "rawBytes": len(body),
                       "rawSha256": sha256(body), "endedAt": inv04.utc_now()})
        if not credentials_absent(body, token, private):
            raise CaptureError("observer API response contains private credential material")
        inv04._write(out / name, body)
        if status != 200:
            raise CaptureError("required observer API GET did not return HTTP 200")
        if name.endswith("pod.json"):
            value = validate_pod(body, phase)
        elif name.endswith("nodes.json"):
            value = validate_nodes(body, phase, expected_node_name)
        else:
            value = validate_events(body, pod_uid or "", phase)
        record["validation"] = value
        return body, value
    except Exception as error:
        record.setdefault("endedAt", inv04.utc_now())
        record["error"] = str(error)[:250] if isinstance(error, (CaptureError, inv04.CaptureError)) else type(error).__name__
        raise
    finally:
        record.setdefault("elapsedSeconds", time.monotonic() - began)
        records.append(record)


def wait_for_unschedulable_pod(server: str, ca: bytes, token: str,
                               private: tuple[bytes, ...], *, api_reader=None,
                               deadline: float, observations: list[dict] | None = None) -> dict:
    """Wait read-only for scheduler evidence without replacing retained phase artifacts."""
    wait_deadline = min(deadline, time.monotonic() + PREREQUISITE_WAIT_SECONDS)
    read = fetch_api if api_reader is None else api_reader
    observations = [] if observations is None else observations
    pod_uid = None
    while time.monotonic() < wait_deadline:
        tick = time.monotonic()
        entry = {"startedAt": inv04.utc_now()}
        try:
            status, pod_body, pod_elapsed = read(server, ca, token, API_POD,
                timeout=min(10.0, wait_deadline - time.monotonic()))
            if len(pod_body) > MAX_BODY or not credentials_absent(pod_body, token, private):
                raise CaptureError("prerequisite poll Pod response violates body/credential bounds")
            entry.update({"podHttpStatus": status, "podSha256": sha256(pod_body),
                          "podElapsedSeconds": pod_elapsed})
            if status != 200:
                raise CaptureError("prerequisite poll Pod GET did not return HTTP 200")
            current_pod = validate_pod(pod_body, "before")
            if pod_uid is not None and current_pod["uid"] != pod_uid:
                raise CaptureError("Pod UID changed while awaiting scheduler evidence")
            pod_uid = current_pod["uid"]
            path = API_EVENTS_PREFIX + urllib.parse.quote(pod_uid, safe="")
            status, event_body, event_elapsed = read(server, ca, token, path,
                timeout=min(10.0, wait_deadline - time.monotonic()))
            if len(event_body) > MAX_BODY or not credentials_absent(event_body, token, private):
                raise CaptureError("prerequisite poll EventList violates body/credential bounds")
            entry.update({"eventsHttpStatus": status, "eventsSha256": sha256(event_body),
                          "eventsElapsedSeconds": event_elapsed})
            if status != 200:
                raise CaptureError("prerequisite poll EventList GET did not return HTTP 200")
            validate_events(event_body, pod_uid, "before")
            entry["validation"] = "Pod Unschedulable and UID-correlated FailedScheduling selector mismatch observed"
            entry["endedAt"] = inv04.utc_now()
            entry["elapsedSeconds"] = time.monotonic() - tick
            observations.append(entry)
            return current_pod
        except CaptureError as error:
            entry["error"] = str(error)[:250]
        except Exception as error:
            entry["error"] = type(error).__name__
        entry["endedAt"] = inv04.utc_now()
        entry["elapsedSeconds"] = time.monotonic() - tick
        observations.append(entry)
        remaining = wait_deadline - time.monotonic()
        if remaining > 0:
            time.sleep(min(1.0, remaining))
    raise CaptureError("timed out waiting for PodScheduled=False/Unschedulable and UID-correlated FailedScheduling event")


def capture(shared_kubeconfig: Path, output: Path, *, runner=None, api_reader=None) -> int:
    out = inv04._fresh_output(output)
    shared = shared_kubeconfig.expanduser().absolute()
    if shared.is_symlink() or not shared.is_file():
        raise CaptureError("shared kubeconfig must be an existing regular non-symlink file")
    shared = shared.resolve(strict=True)
    shared_before = sha256_file(shared)
    env = dict(os.environ)
    if env.get("DOCKER_HOST"):
        raise CaptureError("DOCKER_HOST is set; refusing a potentially remote engine")
    run = inv04.run_bounded if runner is None else runner
    started_at, began = inv04.utc_now(), time.monotonic()
    total_deadline = began + TOTAL_SECONDS
    execution_deadline = began + EXECUTION_SECONDS
    records: list[dict] = []
    commands: list[dict] = []
    errors: list[str] = []
    cluster = cluster_name()
    owner_marker = {"schema": "pre02-owned-cluster-marker.v1", "clusterName": cluster,
                    "ownerPid": os.getpid(), "sourceRevision": None, "createdAt": inv04.utc_now()}
    owner_bytes = (json.dumps(owner_marker, sort_keys=True, indent=2) + "\n").encode()
    inv04._write(out / "owned-cluster-marker.json", owner_bytes)
    owner_hash = sha256(owner_bytes)
    manifest_path = out / "fixture.yaml"
    inv04._write(manifest_path, MANIFEST)
    temp = Path(tempfile.mkdtemp(prefix="scout-pre02-private-"))
    os.chmod(temp, 0o700)
    admin = temp / "admin.kubeconfig"
    observer = temp / "observer.kubeconfig"
    env["KUBECONFIG"] = str(admin)
    create_attempted = False
    cleanup_verified = False
    private_before: dict[str, str] = {}
    private_before_cleanup: dict[str, str] = {}
    private_after_cleanup: dict[str, str] = {}
    helper_paths = (Path(__file__), INV04_PATH, RUL04_PATH)
    helper_hashes_before = {str(path): sha256_file(path) for path in helper_paths}
    prerequisite_wait: list[dict] = []
    source_revision = ""
    pins: dict = {}
    signal_state = None
    before_pod = before_nodes = after_pod = after_nodes = None
    before_events = after_events = None
    transition = None
    token = ""
    private_material: tuple[bytes, ...] = ()
    error_message: str | None = None
    receipt = {"schema": "pre02-node-selector-capture.v1", "status": "running"}

    def call(label: str, argv: list[str], timeout: float = 15, *, env_override=None,
             persist: bool = False) -> tuple[int, bytes, bytes]:
        remaining = execution_deadline - time.monotonic()
        if remaining <= 0:
            raise CaptureError("120-second execution deadline expired")
        command_started = inv04.utc_now()
        tick = time.monotonic()
        try:
            code, stdout, stderr = run(argv, min(timeout, remaining),
                                       env=env if env_override is None else env_override,
                                       max_output=MAX_COMMAND_OUTPUT)
        except Exception as err:
            commands.append({"operation": label, "startedAt": command_started, "endedAt": inv04.utc_now(),
                "exitCode": None, "error": type(err).__name__, "outputFilesRetained": False})
            raise CaptureError(label + " failed: " + type(err).__name__) from None
        commands.append({"operation": label, "startedAt": command_started, "endedAt": inv04.utc_now(),
            "elapsedSeconds": time.monotonic() - tick, "exitCode": code,
            "stdoutBytes": len(stdout), "stderrBytes": len(stderr),
            "stdoutSha256": sha256(stdout), "stderrSha256": sha256(stderr),
            "outputFilesRetained": persist})
        if persist:
            inv04._write(out / (label + ".stdout.bin"), stdout)
            inv04._write(out / (label + ".stderr.bin"), stderr)
        return code, stdout, stderr

    try:
        signal_state = inv04._arm_capture_signals()
        git = inv04._command("git")
        code, revision, _ = run([git, "-C", str(REPO), "rev-parse", "HEAD"], 5,
                                env=env, max_output=4096)
        if code:
            raise CaptureError("could not read source revision")
        source_revision = revision.decode("ascii", "strict").strip()
        if not re.fullmatch(r"[0-9a-f]{40,64}", source_revision):
            raise CaptureError("source revision is malformed")
        code, dirty, _ = run([git, "-C", str(REPO), "status", "--porcelain"], 5,
                             env=env, max_output=16384)
        if code or dirty.strip():
            raise CaptureError("capture source checkout must be clean")
        owner_marker["sourceRevision"] = source_revision
        owner_bytes = (json.dumps(owner_marker, sort_keys=True, indent=2) + "\n").encode()
        (out / "owned-cluster-marker.json").write_bytes(owner_bytes)
        owner_hash = sha256(owner_bytes)

        kind = inv04._command("kind")
        kubectl = inv04._command("kubectl")
        docker = inv04._command("docker")
        for label, path in (("kind", Path(kind)), ("kubectl", Path(kubectl)), ("docker", Path(docker))):
            pins[label] = {"path": str(path.resolve(strict=True)), "sha256": sha256_file(path.resolve(strict=True))}
        if inv04._command("docker") != docker:
            raise CaptureError("Docker executable changed during preflight")
        if env.get("DOCKER_HOST"):
            raise CaptureError("DOCKER_HOST is set; refusing a potentially remote engine")
        code, context_bytes, _ = call("docker-context-show", [docker, "context", "show"], 10)
        if code != 0:
            raise CaptureError("could not identify selected Docker context")
        context = context_bytes.decode("utf-8", "strict").strip()
        if not context or not re.fullmatch(r"[A-Za-z0-9_.-]+", context):
            raise CaptureError("selected Docker context name is malformed")
        code, endpoint_bytes, _ = call("docker-context-inspect", [docker, "context", "inspect", context,
            "--format", "{{json .Endpoints.docker.Host}}"], 10)
        if code != 0:
            raise CaptureError("could not inspect selected Docker context")
        endpoint = strict_json(endpoint_bytes, "Docker context endpoint")
        if not isinstance(endpoint, str) or not endpoint.startswith("unix://"):
            raise CaptureError("selected Docker context is not a local Unix socket")
        env["DOCKER_CONTEXT"] = context
        pins["dockerContext"] = {"name": context, "endpoint": endpoint,
                                  "selectionPinnedInEnvironment": True}
        code, version_bytes, _ = call("kind-version", [kind, "version"], 10)
        if code != 0:
            raise CaptureError("could not read local kind version")
        version = version_bytes.decode("utf-8", "strict").strip()
        inv04.require_kind_version(version)
        code, image, _ = call("inspect-pinned-node-image", [docker, "image", "inspect", inv04.NODE_IMAGE], 15)
        if code != 0:
            raise CaptureError("pinned kind node image is not cached")
        image_digest = inv04.verified_node_repo_digest(image)
        pins["kindNodeImage"] = {"requested": inv04.NODE_IMAGE, "verifiedRepoDigest": image_digest,
                                 "inspectSha256": sha256(image)}
        code, clusters, _ = call("list-clusters-before", [kind, "get", "clusters"], 10)
        if code or cluster in clusters.decode("utf-8", "replace").splitlines():
            raise CaptureError("could not establish fresh non-reused cluster name")
        create_attempted = True
        code, stdout, stderr = call("create-owned-cluster", [kind, "create", "cluster", "--name", cluster,
            "--image", inv04.NODE_IMAGE, "--kubeconfig", str(admin), "--wait", "90s"], 90)
        if code != 0:
            raise CaptureError("owned kind cluster creation failed")
        admin_bytes = admin.read_bytes()
        if not credentials_absent(stdout + b"\n" + stderr, "", rul04.private_material({}, admin_bytes)):
            raise CaptureError("kind diagnostics contain private kubeconfig material")
        code, _, _ = call("apply-authored-fixture", [kubectl, "--context", "kind-" + cluster,
            "--kubeconfig", str(admin), "apply", "-f", str(manifest_path)], 30)
        if code != 0:
            raise CaptureError("authored namespace/observer/Pod apply failed")
        code, raw_config_bytes, _ = call("read-private-admin-config", [kubectl, "--context", "kind-" + cluster,
            "--kubeconfig", str(admin), "config", "view", "--minify", "--flatten", "--raw", "-o", "json"], 15)
        if code != 0:
            raise CaptureError("private kind kubeconfig inspection failed")
        raw_config = strict_json(raw_config_bytes, "private kubeconfig")
        cluster_config = raw_config["clusters"][0]["cluster"]
        server = cluster_config["server"]
        ca64 = cluster_config["certificate-authority-data"]
        ca_pem = base64.b64decode(ca64, validate=True)
        check_server(server)
        code, token_bytes, _ = call("create-private-observer-token", [kubectl, "--context", "kind-" + cluster,
            "--kubeconfig", str(admin), "create", "token", "pre02-observer", "-n", NAMESPACE,
            "--duration=900s"], 15)
        if code != 0:
            raise CaptureError("could not create short-lived observer token")
        token = token_bytes.decode("utf-8", "strict").strip()
        if not token or any(char.isspace() for char in token):
            raise CaptureError("observer ServiceAccount token is malformed")
        inv04._write(observer, observer_config(server, ca64, token), mode=0o600)
        private_material = rul04.private_material(raw_config, admin_bytes) + (observer.read_bytes(),)
        private_before = {"admin": sha256(admin_bytes), "observer": sha256(observer.read_bytes())}

        waited_pod = wait_for_unschedulable_pod(
            server, ca_pem, token, private_material, api_reader=api_reader,
            deadline=execution_deadline, observations=prerequisite_wait)

        before_pod_body, before_pod = capture_api(out, "before", "before-pod.json", API_POD,
            server, ca_pem, token, None, private_material, records, api_reader=api_reader,
            deadline=execution_deadline)
        if before_pod["uid"] != waited_pod["uid"]:
            raise CaptureError("Pod UID changed between scheduler prerequisite wait and retained before snapshot")
        before_nodes_body, before_nodes = capture_api(out, "before", "before-nodes.json", API_NODES,
            server, ca_pem, token, before_pod["uid"], private_material, records,
            api_reader=api_reader, deadline=execution_deadline)
        events_path = API_EVENTS_PREFIX + urllib.parse.quote(before_pod["uid"], safe="")
        before_events_body, before_events = capture_api(out, "before", "before-events.json", events_path,
            server, ca_pem, token, before_pod["uid"], private_material, records,
            api_reader=api_reader, deadline=execution_deadline)
        if before_pod["nodeName"] or before_nodes["matchingNodeNames"]:
            raise CaptureError("before phase unexpectedly satisfies the node selector")
        if len(before_nodes["nodes"]) != 1:
            raise CaptureError("fixture expects one fresh kind node for this topology proof")
        node_name = before_nodes["nodes"][0]["name"]
        code, _, _ = call("label-owned-node", [kubectl, "--context", "kind-" + cluster,
            "--kubeconfig", str(admin), "label", "node", node_name,
            LABEL_KEY + "=" + LABEL_VALUE, "--overwrite=false"], 15)
        if code != 0:
            raise CaptureError("could not add the one authored prerequisite label")
        code, _, _ = call("wait-for-scheduling", [kubectl, "--context", "kind-" + cluster,
            "--kubeconfig", str(admin), "--namespace", NAMESPACE, "wait", "--for=jsonpath={.status.conditions[?(@.type==\"PodScheduled\")].status}=True",
            "pod/" + POD_NAME, "--timeout=45s"], 50)
        if code != 0:
            raise CaptureError("Pod did not become scheduled after the selector label")
        after_pod_body, after_pod = capture_api(out, "after", "after-pod.json", API_POD,
            server, ca_pem, token, before_pod["uid"], private_material, records,
            api_reader=api_reader, deadline=execution_deadline)
        after_nodes_body, after_nodes = capture_api(out, "after", "after-nodes.json", API_NODES,
            server, ca_pem, token, before_pod["uid"], private_material, records,
            api_reader=api_reader, deadline=execution_deadline, expected_node_name=node_name)
        after_events_path = API_EVENTS_PREFIX + urllib.parse.quote(before_pod["uid"], safe="")
        after_events_body, after_events = capture_api(out, "after", "after-events.json", after_events_path,
            server, ca_pem, token, before_pod["uid"], private_material, records,
            api_reader=api_reader, deadline=execution_deadline)
        transition = validate_transition(before_pod, before_nodes, after_pod, after_nodes)
        if after_pod["nodeName"] != node_name:
            raise CaptureError("Pod scheduled to a node other than the explicitly labeled node")
        if not credentials_absent(b"".join((before_pod_body, before_nodes_body, before_events_body,
                                             after_pod_body, after_nodes_body, after_events_body)), token,
                                 private_material):
            raise CaptureError("captured API evidence contains private credential material")
        if sha256_file(Path(kind).resolve(strict=True)) != pins["kind"]["sha256"]:
            raise CaptureError("kind executable changed during capture")
        if sha256_file(Path(kubectl).resolve(strict=True)) != pins["kubectl"]["sha256"]:
            raise CaptureError("kubectl executable changed during capture")
        if sha256_file(Path(docker).resolve(strict=True)) != pins["docker"]["sha256"]:
            raise CaptureError("Docker executable changed during capture")
    except BaseException as err:
        error_message = str(err)[:300] if isinstance(err, (CaptureError, inv04.CaptureError)) else type(err).__name__
        errors.append(error_message)
    finally:
        cleanup_started = time.monotonic()
        cleanup_deadline = min(total_deadline, cleanup_started + CLEANUP_SECONDS)
        if signal_state is not None:
            inv04._suppress_capture_signals(signal_state)
        if create_attempted:
            try:
                for label, path in (("admin", admin), ("observer", observer)):
                    try:
                        private_before_cleanup[label] = sha256_file(path)
                    except OSError:
                        private_before_cleanup[label] = ""
                    if label in private_before and private_before_cleanup[label] != private_before[label]:
                        errors.append(label + " private kubeconfig changed before owned-cluster cleanup")
                marker_doc = strict_json((out / "owned-cluster-marker.json").read_bytes(), "ownership marker")
                if not owns_cluster(marker_doc, cluster, os.getpid()):
                    raise CaptureError("owned-cluster marker failed ownership validation")
                code, cluster_out, _ = run([kind, "get", "clusters"], min(10, cleanup_deadline-time.monotonic()),
                                           env=env, max_output=4096)
                if code != 0:
                    raise CaptureError("could not verify owned cluster before deletion")
                names = cluster_out.decode("utf-8", "strict").splitlines()
                if cluster in names:
                    remaining = cleanup_deadline - time.monotonic()
                    if remaining <= 0:
                        raise CaptureError("cleanup deadline expired")
                    code, _, _ = run([kind, "delete", "cluster", "--name", cluster,
                                      "--kubeconfig", str(admin)], min(20, remaining),
                                     env=env, max_output=MAX_COMMAND_OUTPUT)
                    if code != 0:
                        raise CaptureError("owned cluster deletion failed")
                remaining = cleanup_deadline - time.monotonic()
                if remaining <= 0:
                    raise CaptureError("cleanup deadline expired before absence check")
                code, cluster_out, _ = run([kind, "get", "clusters"], min(10, remaining),
                                           env=env, max_output=4096)
                cleanup_verified = code == 0 and cluster not in cluster_out.decode("utf-8", "replace").splitlines()
                if not cleanup_verified:
                    raise CaptureError("owned cluster absence was not verified")
            except BaseException as err:
                cleanup_verified = False
                errors.append("cleanup: " + (str(err)[:250] if isinstance(err, CaptureError) else type(err).__name__))
        else:
            cleanup_verified = True
        for label, path in (("admin", admin), ("observer", observer)):
            try:
                private_after_cleanup[label] = sha256_file(path)
            except OSError:
                private_after_cleanup[label] = ""
        shutil.rmtree(temp, ignore_errors=True)
        if signal_state is not None:
            inv04._restore_capture_signals(signal_state)

    try:
        shared_after = sha256_file(shared)
    except OSError:
        shared_after = ""
        errors.append("shared kubeconfig could not be hashed after capture")
    if shared_before != shared_after:
        errors.append("shared kubeconfig changed during capture")
    if not cleanup_verified:
        errors.append("owned cluster cleanup is uncertain")
    helper_hashes_after = {}
    for path in helper_paths:
        try:
            helper_hashes_after[str(path)] = sha256_file(path)
        except OSError:
            helper_hashes_after[str(path)] = ""
    if helper_hashes_before != helper_hashes_after:
        errors.append("capture or dependency source changed during run")
    status = "passed" if not errors and transition is not None and len(records) == 6 else "failed"
    receipt.update({"status": status, "sourceRevision": source_revision or None,
        "sourceHashes": {"before": helper_hashes_before, "after": helper_hashes_after},
        "inv04Runner": {"path": str(INV04_PATH), "sha256": helper_hashes_after.get(str(INV04_PATH), "")},
        "rul04ObserverHelpers": {"path": str(RUL04_PATH),
        "sha256": helper_hashes_after.get(str(RUL04_PATH), "")}, "toolPins": pins, "kindNodeImage": inv04.NODE_IMAGE,
        "kindVersionRequired": inv04.KIND_VERSION, "clusterName": cluster,
        "ownedClusterMarkerSha256": owner_hash, "fixtureSha256": sha256(MANIFEST),
        "fixturePath": "fixture.yaml", "labelMutation": {"nodeLabelKey": LABEL_KEY,
        "nodeLabelValue": LABEL_VALUE, "scope": "one node in the owned cluster only"},
        "readOnlyObserver": {"serviceAccount": "pre02-observer", "allowedResources": {
        "namespaced": {"pods": ["get"], "events": ["list"]}, "cluster": {"nodes": ["list"]}},
        "secretReads": False}, "phases": {"before": ["before-pod.json", "before-nodes.json", "before-events.json"],
        "after": ["after-pod.json", "after-nodes.json", "after-events.json"]},
        "observations": records, "transition": transition,
        "sharedKubeconfigSha256": {"before": shared_before, "after": shared_after},
        "privateKubeconfigSha256": {"before": private_before, "beforeCleanup": private_before_cleanup,
                                     "afterCleanup": private_after_cleanup},
        "cleanupVerified": cleanup_verified, "commands": commands,
        "prerequisiteWait": prerequisite_wait,
        "timing": {"startedAt": started_at, "endedAt": inv04.utc_now(),
                   "elapsedSeconds": time.monotonic() - began, "executionLimitSeconds": EXECUTION_SECONDS,
                   "cleanupLimitSeconds": CLEANUP_SECONDS, "totalLimitSeconds": TOTAL_SECONDS},
        "outputLimits": {"apiBodyBytes": MAX_BODY, "commandBytesEach": MAX_COMMAND_OUTPUT},
        "claims": {"prerequisite": ("verified: node selector label absent before and satisfied after on the same owned cluster"
                                     if status == "passed" else "not verified: capture did not complete successfully"),
                   "cloudAPI": "not observed", "secret": "not observed", "capacity": "not inferred",
                   "readinessOrHealth": "not assessed"}, "error": errors})
    try:
        inv04._write(out / "provenance.json", (json.dumps(receipt, sort_keys=True, indent=2) + "\n").encode())
    except OSError:
        return 1
    return 0 if status == "passed" else 1


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--execute", action="store_true", help="required acknowledgement for owned kind lifecycle")
    parser.add_argument("--shared-kubeconfig", type=Path, required=True,
                        help="hash only; never used for fixture reads/writes")
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args(argv)
    if not args.execute:
        parser.error("--execute is required; capture creates and deletes one owned kind cluster")
    try:
        return capture(args.shared_kubeconfig, args.output_dir)
    except (CaptureError, inv04.CaptureError, OSError, ValueError) as err:
        print("PRE-02 capture failed: " + (str(err) if isinstance(err, (CaptureError, inv04.CaptureError)) else type(err).__name__),
              file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
