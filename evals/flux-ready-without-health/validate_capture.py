#!/usr/bin/env python3
"""Validate captured workload objects by Kubernetes controller owner chain."""

import json
import pathlib
import sys

MISSING_COMMAND = "/scout-fixture-intentionally-missing"
EXPECTED_IMAGE = "registry.k8s.io/pause:3.9"


class InvalidCapture(ValueError):
    pass


def read_json(path):
    try:
        return json.loads(pathlib.Path(path).read_text())
    except (OSError, json.JSONDecodeError) as exc:
        raise InvalidCapture(f"cannot read {path}: {exc}") from exc


def controller_ref(obj, kind, uid, name):
    return any(
        ref.get("apiVersion") == "apps/v1"
        and ref.get("kind") == kind
        and ref.get("name") == name
        and ref.get("uid") == uid
        and ref.get("controller") is True
        for ref in obj.get("metadata", {}).get("ownerReferences", [])
    )


def named_container(obj, name):
    return next(
        (container for container in obj.get("spec", {}).get("template", {}).get("spec", {}).get("containers", [])
         if container.get("name") == name),
        None,
    )


def pod_container_status(pod, name):
    return next(
        (status for status in pod.get("status", {}).get("containerStatuses", [])
         if status.get("name") == name),
        None,
    )


def failing_for_fixture(pod):
    status = pod_container_status(pod, "worker")
    if status is None:
        return False
    waiting = status.get("state", {}).get("waiting") or {}
    message = waiting.get("message", "")
    if waiting.get("reason") == "CreateContainerError" and MISSING_COMMAND in message:
        return True
    if waiting.get("reason") == "CrashLoopBackOff":
        terminated = status.get("lastState", {}).get("terminated") or {}
        return type(terminated.get("exitCode")) is int and terminated["exitCode"] != 0
    terminated = status.get("state", {}).get("terminated") or {}
    return type(terminated.get("exitCode")) is int and terminated["exitCode"] != 0


def validate_capture(deployment, replicasets, pod_list):
    if deployment.get("apiVersion") != "apps/v1" or deployment.get("kind") != "Deployment" or deployment.get("metadata", {}).get("name") != "payment-worker" or deployment.get("metadata", {}).get("namespace") != "scout-hlt02":
        raise InvalidCapture("selected object is not Deployment/payment-worker")
    deployment_meta = deployment.get("metadata", {})
    deployment_uid = deployment_meta.get("uid")
    generation = deployment_meta.get("generation")
    status = deployment.get("status", {})
    if not deployment_uid or type(generation) is not int or generation < 1 or type(status.get("observedGeneration")) is not int or status.get("observedGeneration") != generation:
        raise InvalidCapture("Deployment status is not observed for its current generation")
    counts = [deployment.get("spec", {}).get("replicas", 1), status.get("availableReplicas", 0), status.get("unavailableReplicas", 0)]
    if any(type(value) is not int or value < 0 for value in counts):
        raise InvalidCapture("Deployment replica counts must be nonnegative integers")
    if deployment.get("spec", {}).get("replicas", 1) != 1 or status.get("availableReplicas", 0) != 0 or status.get("unavailableReplicas", 0) < 1:
        raise InvalidCapture("Deployment does not show one current unavailable replica")
    if deployment_meta.get("labels", {}).get("kustomize.toolkit.fluxcd.io/name") != "apps" or deployment_meta.get("labels", {}).get("kustomize.toolkit.fluxcd.io/namespace") != "flux-system":
        raise InvalidCapture("Deployment lacks the expected Flux Kustomization identity labels")
    deployment_worker = named_container(deployment, "worker")
    if deployment_worker is None or deployment_worker.get("image") != EXPECTED_IMAGE or deployment_worker.get("command") != [MISSING_COMMAND]:
        raise InvalidCapture("Deployment does not contain the intentional failing fixture template")

    replica_sets = replicasets.get("items", [])
    candidate_sets = []
    for rs in replica_sets:
        if rs.get("apiVersion") != "apps/v1" or rs.get("kind") != "ReplicaSet" or rs.get("metadata", {}).get("namespace") != "scout-hlt02" or not controller_ref(rs, "Deployment", deployment_uid, "payment-worker"):
            continue
        worker = named_container(rs, "worker")
        if worker is None or worker.get("image") != EXPECTED_IMAGE or worker.get("command") != [MISSING_COMMAND]:
            continue
        counts = [rs.get("spec", {}).get("replicas", 1), rs.get("status", {}).get("readyReplicas", 0)]
        if any(type(value) is not int or value < 0 for value in counts):
            continue
        if rs.get("spec", {}).get("replicas", 1) != 1 or rs.get("status", {}).get("readyReplicas", 0) != 0:
            continue
        candidate_sets.append(rs)
    if not candidate_sets:
        raise InvalidCapture("no captured ReplicaSet has the expected controller Deployment and fixture template")

    for rs in candidate_sets:
        rs_meta = rs.get("metadata", {})
        rs_uid = rs_meta.get("uid")
        template_hash = rs_meta.get("labels", {}).get("pod-template-hash")
        if not rs_uid or not rs_meta.get("name") or not template_hash:
            continue
        for pod in pod_list.get("items", []):
            pod_meta = pod.get("metadata", {})
            if pod.get("apiVersion") != "v1" or pod.get("kind") != "Pod" or pod_meta.get("namespace") != "scout-hlt02" or not pod_meta.get("uid"):
                continue
            worker = next((c for c in pod.get("spec", {}).get("containers", []) if c.get("name") == "worker"), None)
            if worker is None or worker.get("image") != EXPECTED_IMAGE or worker.get("command") != [MISSING_COMMAND]:
                continue
            if pod_meta.get("labels", {}).get("app.kubernetes.io/name") != "payment-worker":
                continue
            if pod_meta.get("labels", {}).get("pod-template-hash") != template_hash or not controller_ref(pod, "ReplicaSet", rs_uid, rs_meta["name"]):
                continue
            ready = next(
                (condition.get("status") for condition in pod.get("status", {}).get("conditions", [])
                 if condition.get("type") == "Ready"),
                None,
            )
            if ready != "False" or not failing_for_fixture(pod):
                continue
            return {
                "deploymentUID": deployment_uid,
                "replicaSetUID": rs_uid,
                "podUID": pod_meta.get("uid", ""),
                "podReady": False,
            }
    raise InvalidCapture("no selected Pod in the current fixture ReplicaSet has failed readiness and command-failure evidence")


def main(argv):
    if len(argv) != 2:
        print("Usage: validate_capture.py CAPTURE_DIR", file=sys.stderr)
        return 2
    root = pathlib.Path(argv[1])
    try:
        result = validate_capture(
            read_json(root / "deployment.json"),
            read_json(root / "replicasets.json"),
            read_json(root / "pods.json"),
        )
    except InvalidCapture as exc:
        print(str(exc), file=sys.stderr)
        return 1
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
