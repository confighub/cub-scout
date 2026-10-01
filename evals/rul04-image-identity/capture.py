#!/usr/bin/env python3
"""Capture bounded raw StatefulSet image-identity evidence from one owned kind cluster.

Never run without --execute. The authored tag is intentionally mutable; runtime
imageID is preserved verbatim and does not convert that tag into intended digest proof.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import ssl
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

HERE = Path(__file__).resolve().parent
INV04_PATH = HERE.parent / "inv04-rbac" / "capture.py"
spec = importlib.util.spec_from_file_location("inv04_capture_util", INV04_PATH)
inv04 = importlib.util.module_from_spec(spec)
assert spec and spec.loader
spec.loader.exec_module(inv04)

NAMESPACE = "rul04-image-identity"
WORKLOAD = "rul04-pause"
IMAGE = "registry.k8s.io/pause:3.10"
MAX_BODY = 4 * 1024 * 1024
MANIFEST = f'''apiVersion: v1
kind: Namespace
metadata:
  name: {NAMESPACE}
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: rul04-observer
  namespace: {NAMESPACE}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: rul04-observer
  namespace: {NAMESPACE}
rules:
- apiGroups: [apps]
  resources: [statefulsets]
  verbs: [get, list]
- apiGroups: [""]
  resources: [pods]
  verbs: [get, list]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: rul04-observer
  namespace: {NAMESPACE}
subjects:
- kind: ServiceAccount
  name: rul04-observer
  namespace: {NAMESPACE}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: rul04-observer
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {WORKLOAD}
  namespace: {NAMESPACE}
  labels:
    app: {WORKLOAD}
spec:
  serviceName: {WORKLOAD}
  replicas: 1
  selector:
    matchLabels:
      app: {WORKLOAD}
  template:
    metadata:
      labels:
        app: {WORKLOAD}
    spec:
      containers:
      - name: pause
        image: {IMAGE}
'''.encode()


def validate_statefulset(body: bytes) -> dict:
    try:
        obj = json.loads(body)
        meta = obj["metadata"]
        spec = obj["spec"]
        containers = spec["template"]["spec"]["containers"]
    except (UnicodeDecodeError, json.JSONDecodeError, KeyError, TypeError):
        raise inv04.CaptureError("raw StatefulSet response is malformed") from None
    if (obj.get("apiVersion") != "apps/v1" or obj.get("kind") != "StatefulSet"
            or meta.get("name") != WORKLOAD or meta.get("namespace") != NAMESPACE
            or not meta.get("uid") or not meta.get("resourceVersion")
            or not isinstance(meta.get("generation"), int)):
        raise inv04.CaptureError("StatefulSet identity/UID/version/generation is incomplete")
    if len(containers) != 1 or containers[0].get("name") != "pause" or containers[0].get("image") != IMAGE:
        raise inv04.CaptureError("StatefulSet intended tag changed or is ambiguous")
    if "@sha256:" in containers[0]["image"]:
        raise inv04.CaptureError("fixture intent unexpectedly contains an immutable digest")
    if spec.get("replicas") != 1 or spec.get("selector", {}).get("matchLabels") != {"app": WORKLOAD}:
        raise inv04.CaptureError("StatefulSet selector/replica contract changed")
    status = obj.get("status", {})
    return {"uid": meta["uid"], "resourceVersion": meta["resourceVersion"],
            "generation": meta["generation"], "observedGeneration": status.get("observedGeneration"),
            "readyReplicas": status.get("readyReplicas", 0), "intendedImage": IMAGE,
            "intendedImageHasImmutableDigest": False}


def validate_pods(body: bytes, ss: dict) -> dict:
    try:
        obj = json.loads(body)
        items = obj["items"]
    except (UnicodeDecodeError, json.JSONDecodeError, KeyError, TypeError):
        raise inv04.CaptureError("raw PodList response is malformed") from None
    if obj.get("apiVersion") != "v1" or obj.get("kind") != "PodList" or not isinstance(items, list):
        raise inv04.CaptureError("Pod response is not a PodList")
    matches = []
    for pod in items:
        meta = pod.get("metadata", {})
        refs = meta.get("ownerReferences", [])
        if any(ref.get("kind") == "StatefulSet" and ref.get("uid") == ss["uid"] for ref in refs):
            matches.append(pod)
    if len(matches) != 1:
        raise inv04.CaptureError("expected exactly one Pod linked to captured StatefulSet UID")
    pod = matches[0]
    meta, status = pod.get("metadata", {}), pod.get("status", {})
    if (meta.get("namespace") != NAMESPACE or not meta.get("uid") or not meta.get("resourceVersion")
            or meta.get("deletionTimestamp") or status.get("phase") != "Running"):
        raise inv04.CaptureError("owned Pod identity or running state is incomplete")
    owner = next((r for r in meta.get("ownerReferences", []) if r.get("kind") == "StatefulSet"), {})
    if owner.get("name") != WORKLOAD or owner.get("uid") != ss["uid"] or owner.get("controller") is not True:
        raise inv04.CaptureError("Pod owner reference does not exactly bind StatefulSet UID")
    conditions = status.get("conditions", [])
    if not any(c.get("type") == "Ready" and c.get("status") == "True" for c in conditions):
        raise inv04.CaptureError("captured Pod is not Ready")
    statuses = status.get("containerStatuses", [])
    if len(statuses) != 1 or statuses[0].get("name") != "pause" or statuses[0].get("ready") is not True:
        raise inv04.CaptureError("running container status/ready evidence is incomplete")
    if not statuses[0].get("imageID"):
        raise inv04.CaptureError("runtime imageID is missing; preserve missing evidence as a failed capture")
    return {"podName": meta["name"], "podUID": meta["uid"], "ownerUID": owner["uid"],
            "phase": status["phase"], "ready": True, "runtimeImage": statuses[0].get("image"),
            "runtimeImageID": statuses[0]["imageID"],
            "identityConclusion": "runtime imageID observed; tag-only authored intent does not identify an immutable intended digest"}


def request(server: str, ca: bytes, token: str, path: str) -> tuple[int, bytes, float]:
    url = server.rstrip("/") + path
    req = urllib.request.Request(url, headers={"Authorization": "Bearer " + token, "Accept": "application/json"})
    ctx = ssl.create_default_context(cadata=ca.decode("ascii"))
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), inv04._NoRedirect,
                                         urllib.request.HTTPSHandler(context=ctx))
    start = time.monotonic()
    try:
        with opener.open(req, timeout=15) as response:
            status, body = response.status, inv04._read_response_body(response, start + 15)
    except urllib.error.HTTPError as e:
        status, body = e.code, inv04._read_response_body(e, start + 15)
    if len(body) > MAX_BODY:
        raise inv04.CaptureError("API body exceeds 4 MiB bound")
    return status, body, time.monotonic() - start


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--execute", action="store_true", help="required acknowledgement for owned kind lifecycle")
    ap.add_argument("--shared-kubeconfig", type=Path, required=True, help="read only for before/after hash")
    ap.add_argument("--cub-scout-binary", type=Path, required=True, help="explicit pinned executable; never built here")
    ap.add_argument("--source-revision", required=True, help="full source commit for supplied binary")
    ap.add_argument("--oci-layout-helper", type=Path, required=True, help="explicit supplied create-layout executable")
    ap.add_argument("--scout-binary", type=Path, help="optional explicit Scout executable for separate derived output")
    ap.add_argument("--output-dir", type=Path, required=True)
    args = ap.parse_args(argv)
    if not args.execute:
        ap.error("--execute is required; capture creates and deletes one owned cluster")
    try:
        if not __import__("re").fullmatch(r"[0-9a-f]{40,64}", args.source_revision):
            raise inv04.CaptureError("source revision must be a full object ID")
        shared_arg = args.shared_kubeconfig.expanduser().absolute()
        if shared_arg.is_symlink():
            raise inv04.CaptureError("shared kubeconfig must not be a symlink")
        shared = shared_arg.resolve(strict=True)
        binary = args.cub_scout_binary.resolve(strict=True)
        helper = args.oci_layout_helper.resolve(strict=True)
        scout = args.scout_binary.resolve(strict=True) if args.scout_binary else None
        if shared.is_symlink() or not shared.is_file() or not binary.is_file() or not os.access(binary, os.X_OK) or not helper.is_file() or not os.access(helper, os.X_OK):
            raise inv04.CaptureError("shared config or supplied executable is invalid")
        if scout and (not scout.is_file() or not os.access(scout, os.X_OK)):
            raise inv04.CaptureError("optional Scout executable is invalid")
        out = inv04._fresh_output(args.output_dir)
        shared_before = inv04.sha256(shared.read_bytes())
        env = dict(os.environ)
        inv04.require_local_docker(env)
        inv04.require_kind_version(inv04._call("kind", ["version"], 10).decode().strip())
        inv04.verified_node_repo_digest(inv04._call("docker", ["image", "inspect", inv04.NODE_IMAGE], 15))
        cluster = inv04.owned_cluster_name().replace("scout-inv04-rbac", "scout-rul04-identity")
        if cluster in inv04._call("kind", ["get", "clusters"], 15).decode().splitlines():
            raise inv04.CaptureError("refusing to reuse cluster")
        temp = Path(tempfile.mkdtemp(prefix="scout-rul04-private-")); os.chmod(temp, 0o700)
        admin = temp / "admin.kubeconfig"; observer = temp / "observer.kubeconfig"
        env["KUBECONFIG"] = str(admin)
        created = False; cleaned = False; errors = []
        observations = []
        inv04._write(out / "literal-manifests.yaml", MANIFEST)
        desired = out / "desired-statefulset.yaml"
        inv04._write(desired, MANIFEST.split(b"---\n")[-1])
        layout = out / "oci-layout"
        bundle_ref = ""
        layout_helper_sha = inv04.sha256(helper.read_bytes())
        signal_state = inv04._arm_capture_signals()
        try:
            created = True
            code, stdout, stderr = inv04.run_bounded([inv04._command("kind"), "create", "cluster", "--name", cluster,
                "--image", inv04.NODE_IMAGE, "--kubeconfig", str(admin), "--wait", "120s"], 180, env)
            inv04._write(out / "kind-create-stdout.txt", stdout); inv04._write(out / "kind-create-stderr.txt", stderr)
            if code: raise inv04.CaptureError("owned cluster creation failed")
            kubectl = inv04._command("kubectl")
            code, helper_out, helper_err = inv04.run_bounded([str(helper), str(desired), str(layout)], 30, env, max_output=65536)
            if code: raise inv04.CaptureError("local OCI layout helper failed")
            bundle_ref = helper_out.decode("ascii", "strict").strip()
            if not __import__("re").fullmatch(r"oci://[^/@\s]+(?:/[^@\s]+)*@sha256:[0-9a-f]{64}", bundle_ref):
                raise inv04.CaptureError("layout helper did not return a digest-pinned OCI reference")
            inv04._write(out / "oci-layout-ref.json", json.dumps({"bundleRef": bundle_ref,
                "layoutPath": str(layout), "layoutHelperSha256": layout_helper_sha,
                "authoredManifestSha256": inv04.sha256(desired.read_bytes()),
                "meaning": "authored bytes only; not evidence a controller applied this source"}, indent=2).encode()+b"\n")
            code, _, _ = inv04.run_bounded([kubectl, "--context", "kind-"+cluster, "apply", "-f", str(out/"literal-manifests.yaml")], 60, env)
            if code: raise inv04.CaptureError("fixture apply failed")
            # Wait for readiness with a fixed deadline; command output is discarded.
            code, _, _ = inv04.run_bounded([kubectl, "--context", "kind-"+cluster, "-n", NAMESPACE,
                "rollout", "status", "statefulset/"+WORKLOAD, "--timeout=90s"], 100, env)
            if code: raise inv04.CaptureError("StatefulSet did not become Ready")
            raw = json.loads(inv04._call("kubectl", ["--context", "kind-"+cluster, "config", "view", "--minify", "--flatten", "--raw", "-o", "json"], 15, env))
            cc = raw["clusters"][0]["cluster"]; server=cc["server"]; ca64=cc["certificate-authority-data"]; ca=base64.b64decode(ca64, validate=True)
            token=inv04._call("kubectl", ["--context", "kind-"+cluster, "create", "token", "rul04-observer", "-n", NAMESPACE, "--duration=900s"], 15, env).decode().strip()
            inv04._write(observer, inv04.observer_kubeconfig(server, ca64, token)); os.chmod(observer, 0o600)
            paths=[("statefulset.json", f"/apis/apps/v1/namespaces/{NAMESPACE}/statefulsets/{WORKLOAD}"),
                   ("pods.json", f"/api/v1/namespaces/{NAMESPACE}/pods")]
            ss_info=None
            for name,path in paths:
                start=inv04.utc_now(); status, body, elapsed=request(server,ca,token,path); end=inv04.utc_now()
                inv04._write(out/name,body)
                if status!=200: raise inv04.CaptureError("required API read did not return HTTP 200")
                validation=validate_statefulset(body) if name=="statefulset.json" else validate_pods(body,ss_info)
                if name=="statefulset.json": ss_info=validation
                observations.append({"method":"GET","path":path,"startedAt":start,"endedAt":end,"httpStatus":status,
                    "elapsedSeconds":elapsed,"rawFile":name,"rawSha256":inv04.sha256(body),"rawBytes":len(body),"validation":validation})
            if scout:
                argv=[str(scout),"release","check","--bundle",bundle_ref,"--oci-layout",str(layout),
                    "--controller",f"StatefulSet/{WORKLOAD}","--api-version","apps/v1","--controller-namespace",NAMESPACE,
                    "--kube-context","inv04-observer","--check-running-image","--format","json"]
                code, stdout, stderr=inv04.run_bounded(argv,45,dict(env,KUBECONFIG=str(observer)))
                inv04._write(out/"scout-derived-output.json",stdout)
                inv04._write(out/"scout-derived-stderr.txt",stderr)
                observations.append({"derivedScout":{"argv":argv,"exitCode":code,"stdoutSha256":inv04.sha256(stdout),
                    "stderrSha256":inv04.sha256(stderr),"stdoutFile":"scout-derived-output.json",
                    "stderrFile":"scout-derived-stderr.txt","isRawModelEvidence":False}})
        except BaseException as e:
            errors.append(str(e) if isinstance(e, inv04.CaptureError) else type(e).__name__)
        finally:
            inv04._suppress_capture_signals(signal_state)
            try:
                if created and cluster in inv04._call("kind",["get","clusters"],15).decode().splitlines():
                    inv04._call("kind",["delete","cluster","--name",cluster,"--kubeconfig",str(admin)],90,env)
                cleaned=cluster not in inv04._call("kind",["get","clusters"],15).decode().splitlines()
            except Exception: cleaned=False
            shutil.rmtree(temp,ignore_errors=True)
            inv04._restore_capture_signals(signal_state)
        shared_after=inv04.sha256(shared.read_bytes())
        if not cleaned: errors.append("owned cluster cleanup was not verified")
        if shared_before!=shared_after: errors.append("shared kubeconfig changed")
        provenance={"schema":"rul04-image-identity-capture.v1","sourceRevision":args.source_revision,
            "captureScriptSha256":inv04.sha256(Path(__file__).read_bytes()),"cubScoutBinarySha256":inv04.sha256(binary.read_bytes()),
            "scoutBinarySha256":inv04.sha256(scout.read_bytes()) if scout else None,
            "ociLayoutHelperSha256":layout_helper_sha,
            "kindVersion":inv04.KIND_VERSION,"nodeImage":inv04.NODE_IMAGE,"clusterName":cluster,
            "bundleRef":bundle_ref,"ociLayoutPath":str(layout),"layoutHelperSha256":layout_helper_sha,
            "startedAt":inv04.utc_now(),"atomicSnapshot":False,
            "observations":observations,"sharedKubeconfigSha256":{"before":shared_before,"after":shared_after},
            "cleanupVerified":cleaned,"errors":errors,
            "limitations":["no Argo/Flux controller or applied-source proof","OCI ref pins authored configuration bytes only",
                "tag-only authored image cannot be matched to immutable intended digest from runtime imageID alone",
                "two sequential namespace-scoped read-only API responses; not atomic or cluster-complete"]}
        inv04._write(out/"provenance.json",json.dumps(provenance,sort_keys=True,indent=2).encode()+b"\n")
        if errors: raise inv04.CaptureError("capture failed; inspect retained provenance.json")
        return 0
    except (inv04.CaptureError,OSError,KeyError,ValueError) as e:
        print(str(e),file=sys.stderr); return 1


if __name__ == "__main__":
    raise SystemExit(main())
