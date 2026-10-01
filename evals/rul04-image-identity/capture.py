#!/usr/bin/env python3
"""Capture bounded raw StatefulSet image-identity evidence from one owned kind cluster.

Never run without --execute. The authored tag is intentionally mutable; runtime
imageID is preserved verbatim and does not convert that tag into intended digest proof.
"""
from __future__ import annotations

import argparse
import base64
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

sys.dont_write_bytecode = True
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
API_PREFIXES = (f"/apis/apps/v1/namespaces/{NAMESPACE}/statefulsets/{WORKLOAD}",
                f"/api/v1/namespaces/{NAMESPACE}/pods")
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
        if not isinstance(obj,dict): raise TypeError
        meta = obj["metadata"]
        spec = obj["spec"]
        containers = spec["template"]["spec"]["containers"]
    except (UnicodeDecodeError, json.JSONDecodeError, KeyError, TypeError):
        raise inv04.CaptureError("raw StatefulSet response is malformed") from None
    if not isinstance(meta,dict) or not isinstance(spec,dict) or not isinstance(containers,list) or len(containers)!=1 or not isinstance(containers[0],dict):
        raise inv04.CaptureError("StatefulSet metadata/spec/container shape is malformed")
    if (obj.get("apiVersion") != "apps/v1" or obj.get("kind") != "StatefulSet"
            or meta.get("name") != WORKLOAD or meta.get("namespace") != NAMESPACE
            or not nonempty_string(meta.get("uid")) or not nonempty_string(meta.get("resourceVersion"))
            or not positive_int(meta.get("generation"))):
        raise inv04.CaptureError("StatefulSet identity/UID/version/generation is incomplete")
    if containers[0].get("name") != "pause" or containers[0].get("image") != IMAGE:
        raise inv04.CaptureError("StatefulSet intended tag changed or is ambiguous")
    if "@sha256:" in containers[0]["image"]:
        raise inv04.CaptureError("fixture intent unexpectedly contains an immutable digest")
    if type(spec.get("replicas")) is not int or spec["replicas"] != 1 or not isinstance(spec.get("selector"), dict) or spec["selector"].get("matchLabels") != {"app": WORKLOAD}:
        raise inv04.CaptureError("StatefulSet selector/replica contract changed")
    status = obj.get("status", {})
    if (not isinstance(status, dict) or any(type(status.get(key)) is not int for key in ("observedGeneration","readyReplicas","currentReplicas","replicas"))
            or status.get("observedGeneration") != meta["generation"]
            or status.get("readyReplicas") != 1 or status.get("currentReplicas") != 1 or status.get("replicas") != 1):
        raise inv04.CaptureError("StatefulSet readiness/generation status is incomplete")
    return {"uid": meta["uid"], "resourceVersion": meta["resourceVersion"],
            "generation": meta["generation"], "observedGeneration": status.get("observedGeneration"),
            "readyReplicas": status.get("readyReplicas", 0), "intendedImage": IMAGE,
            "intendedImageHasImmutableDigest": False}


def validate_pods(body: bytes, ss: dict) -> dict:
    try:
        obj = json.loads(body)
        if not isinstance(obj,dict): raise TypeError
        items = obj["items"]
    except (UnicodeDecodeError, json.JSONDecodeError, KeyError, TypeError):
        raise inv04.CaptureError("raw PodList response is malformed") from None
    if obj.get("apiVersion") != "v1" or obj.get("kind") != "PodList" or not isinstance(items, list):
        raise inv04.CaptureError("Pod response is not a PodList")
    if not isinstance(obj.get("metadata"),dict) or not nonempty_string(obj["metadata"].get("resourceVersion")):
        raise inv04.CaptureError("PodList collection resourceVersion is missing")
    if not isinstance(ss, dict) or not nonempty_string(ss.get("uid")):
        raise inv04.CaptureError("captured StatefulSet UID is invalid")
    matches = []
    for pod in items:
        if not isinstance(pod, dict): raise inv04.CaptureError("PodList contains a non-object item")
        meta = pod.get("metadata", {})
        if not isinstance(meta, dict): raise inv04.CaptureError("Pod metadata is malformed")
        refs = meta.get("ownerReferences", [])
        if not isinstance(meta, dict) or not isinstance(refs, list) or any(not isinstance(ref, dict) for ref in refs):
            raise inv04.CaptureError("Pod metadata/ownerReferences are malformed")
        if any(ref.get("kind") == "StatefulSet" and ref.get("uid") == ss["uid"] for ref in refs):
            matches.append(pod)
    if len(matches) != 1:
        raise inv04.CaptureError("expected exactly one Pod linked to captured StatefulSet UID")
    pod = matches[0]
    meta, status = pod.get("metadata", {}), pod.get("status", {})
    if not isinstance(meta,dict) or not isinstance(status,dict): raise inv04.CaptureError("Pod metadata/status shape is malformed")
    if (meta.get("namespace") != NAMESPACE or meta.get("name") != WORKLOAD+"-0" or not nonempty_string(meta.get("uid")) or not nonempty_string(meta.get("resourceVersion"))
            or meta.get("deletionTimestamp") or status.get("phase") != "Running"):
        raise inv04.CaptureError("owned Pod identity or running state is incomplete")
    owner = next((r for r in meta.get("ownerReferences", []) if r.get("kind") == "StatefulSet"), {})
    if (owner.get("apiVersion") != "apps/v1" or owner.get("name") != WORKLOAD
            or owner.get("uid") != ss["uid"] or owner.get("controller") is not True):
        raise inv04.CaptureError("Pod owner reference does not exactly bind StatefulSet UID")
    pod_spec=pod.get("spec", {})
    containers=pod_spec.get("containers", []) if isinstance(pod_spec, dict) else None
    if not isinstance(containers, list) or len(containers)!=1 or not isinstance(containers[0],dict) or containers[0].get("name")!="pause" or containers[0].get("image")!=IMAGE:
        raise inv04.CaptureError("Pod spec does not contain the exact intended regular container")
    conditions = status.get("conditions", [])
    if not isinstance(conditions,list) or any(not isinstance(c,dict) for c in conditions): raise inv04.CaptureError("Pod conditions are malformed")
    if not any(c.get("type") == "Ready" and c.get("status") == "True" for c in conditions):
        raise inv04.CaptureError("captured Pod is not Ready")
    statuses = status.get("containerStatuses", [])
    if not isinstance(statuses,list): raise inv04.CaptureError("containerStatuses is malformed")
    if len(statuses) != 1 or not isinstance(statuses[0],dict) or statuses[0].get("name") != "pause" or statuses[0].get("ready") is not True:
        raise inv04.CaptureError("running container status/ready evidence is incomplete")
    if not isinstance(statuses[0].get("state"),dict) or not isinstance(statuses[0]["state"].get("running"),dict):
        raise inv04.CaptureError("container state does not prove it is running")
    if not valid_image_id(statuses[0].get("imageID")) or statuses[0].get("image")!=IMAGE:
        raise inv04.CaptureError("runtime imageID is missing; preserve missing evidence as a failed capture")
    return {"podName": meta["name"], "podUID": meta["uid"], "ownerUID": owner["uid"],
            "phase": status["phase"], "ready": True, "runtimeImage": statuses[0].get("image"),
            "runtimeImageID": statuses[0]["imageID"],
            "identityConclusion": "runtime imageID observed; tag-only authored intent does not identify an immutable intended digest"}


def nonempty_string(value): return isinstance(value,str) and bool(value.strip())


def positive_int(value): return isinstance(value,int) and not isinstance(value,bool) and value > 0


def valid_image_id(value):
    return nonempty_string(value) and re.fullmatch(r"(?:[a-z][a-z0-9+.-]*://)?(?:[^\s@]+@)?sha256:[0-9a-f]{64}",value) is not None


def owned_cluster_name() -> str:
    import uuid
    name="scout-rul04-"+inv04.datetime.now(inv04.timezone.utc).strftime("%Y%m%d%H%M%S")+"-"+uuid.uuid4().hex[:10]
    if len(name+"-control-plane")>63 or not re.fullmatch(r"scout-rul04-[0-9a-z-]+",name):
        raise inv04.CaptureError("generated RUL-04 node name is invalid")
    return name


def owns_cluster(marker, cluster: str, pid: int) -> bool:
    return (isinstance(marker,dict) and marker.get("schema")=="rul04-owned-cluster-marker.v1"
        and marker.get("clusterName")==cluster and marker.get("ownerPid")==pid
        and isinstance(cluster,str) and re.fullmatch(r"scout-rul04-[0-9a-z-]+",cluster) is not None
        and len(cluster+"-control-plane")<=63)


def prepare_output(path: Path) -> Path:
    candidate=path.expanduser().absolute()
    if candidate.is_symlink() or candidate.exists(): raise inv04.CaptureError("output directory must be new and not a symlink")
    return inv04._fresh_output(candidate)


def observer_config(server: str, ca_data: str, token: str) -> bytes:
    check_server(server)
    if not ca_data or not token or any(c.isspace() for c in token): raise inv04.CaptureError("observer credentials are incomplete")
    base={"apiVersion":"v1","kind":"Config","current-context":"rul04-observer",
        "clusters":[{"name":"rul04-owned","cluster":{"server":server,"certificate-authority-data":ca_data}}],
        "users":[{"name":"rul04-observer","user":{"token":token}}],
        "contexts":[{"name":"rul04-observer","context":{"cluster":"rul04-owned","user":"rul04-observer","namespace":NAMESPACE}}]}
    return (json.dumps(base,sort_keys=True,separators=(",",":"))+"\n").encode()


def check_server(server: str) -> None:
    parsed=urllib.parse.urlparse(server)
    if (parsed.scheme!="https" or parsed.hostname not in ("127.0.0.1","localhost","::1")
            or not parsed.port or parsed.username or parsed.password or parsed.path or parsed.query or parsed.fragment):
        raise inv04.CaptureError("refusing a non-loopback or malformed Kubernetes API endpoint")


def check_path(path: str) -> None:
    if path not in API_PREFIXES: raise inv04.CaptureError("API path is outside the exact capture allowlist")


def credentials_absent(data: bytes, token: str, private_configs: tuple[bytes,...]) -> bool:
    return (not token or token.encode() not in data) and all(not value or value not in data for value in private_configs)


def private_material(raw_config: dict, config_bytes: bytes) -> tuple[bytes,...]:
    """Check individual credentials as well as whole config documents, in memory."""
    values=[config_bytes]
    for match in re.finditer(rb"(?m)^\s*(?:client-key-data|client-certificate-data):\s*([A-Za-z0-9+/=]+)\s*$",config_bytes):
        values.extend((match[1],base64.b64decode(match[1],validate=True)))
    for user in raw_config.get("users", []):
        for key,value in user.get("user", {}).items():
            if key in ("client-key-data", "client-certificate-data", "token", "password") and isinstance(value,str) and value:
                values.append(value.encode())
                if key.endswith("-data"):
                    try: values.append(base64.b64decode(value,validate=True))
                    except ValueError: raise inv04.CaptureError("private credential encoding is malformed") from None
    return tuple(values)


def verify_hash(path: Path, expected: str) -> str:
    actual=inv04.sha256(path.read_bytes())
    if actual!=expected: raise inv04.CaptureError("supplied executable SHA-256 mismatch")
    return actual


def validate_scout_result(code: int, stdout: bytes) -> dict:
    try:
        result=json.loads(stdout); running=result["runningImage"]
        if code!=0 or running.get("verdict")!="unknown" or not any(w.get("reason")=="workload-ownership-unsupported" for w in running["workloads"]):
            raise inv04.CaptureError("Scout did not report expected unsupported running-image UNKNOWN")
        return {"verdict":running["verdict"],"reason":"workload-ownership-unsupported"}
    except (UnicodeDecodeError,json.JSONDecodeError,KeyError,TypeError):
        raise inv04.CaptureError("Scout output lacks structured unsupported UNKNOWN") from None


def capture_raw_observation(name, path, server, ca, token, ss_info, records, out, private_configs):
    check_path(path); start=inv04.utc_now(); record={"method":"GET","path":path,"startedAt":start,"rawFile":name}
    try:
        status,body,elapsed=request(server,ca,token,path)
        record.update({"endedAt":inv04.utc_now(),"httpStatus":status,"elapsedSeconds":elapsed,
            "rawSha256":inv04.sha256(body),"rawBytes":len(body)})
        if not credentials_absent(body,token,private_configs): raise inv04.CaptureError("raw API response contains private credentials")
        inv04._write(out/name,body)
        if status!=200: raise inv04.CaptureError("required API read did not return HTTP 200")
        validation=validate_statefulset(body) if name=="statefulset.json" else validate_pods(body,ss_info)
        record["validation"]=validation
        return validation
    except Exception as e:
        record.setdefault("endedAt",inv04.utc_now()); record["error"]=str(e) if isinstance(e,inv04.CaptureError) else type(e).__name__
        raise
    finally:
        records.append(record)


def request(server: str, ca: bytes, token: str, path: str) -> tuple[int, bytes, float]:
    check_server(server); check_path(path)
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
        try: status, body = e.code, inv04._read_response_body(e, start + 15)
        finally: e.close()
    if len(body) > MAX_BODY:
        raise inv04.CaptureError("API body exceeds 4 MiB bound")
    return status, body, time.monotonic() - start


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--execute", action="store_true", help="required acknowledgement for owned kind lifecycle")
    ap.add_argument("--shared-kubeconfig", type=Path, required=True, help="read only for before/after hash")
    ap.add_argument("--expected-layout-helper-sha256", required=True)
    ap.add_argument("--layout-helper-source-revision", required=True)
    ap.add_argument("--oci-layout-helper", type=Path, required=True, help="explicit supplied create-layout executable")
    ap.add_argument("--scout-binary", type=Path, help="optional explicit Scout executable for separate derived output")
    ap.add_argument("--expected-scout-sha256")
    ap.add_argument("--scout-source-revision")
    ap.add_argument("--output-dir", type=Path, required=True)
    args = ap.parse_args(argv)
    if not args.execute:
        ap.error("--execute is required; capture creates and deletes one owned cluster")
    try:
        if not re.fullmatch(r"[0-9a-f]{40,64}", args.layout_helper_source_revision):
            raise inv04.CaptureError("layout helper source revision must be a full object ID")
        if not re.fullmatch(r"[0-9a-f]{64}", args.expected_layout_helper_sha256):
            raise inv04.CaptureError("expected layout-helper SHA-256 is malformed")
        shared_arg = args.shared_kubeconfig.expanduser().absolute()
        if shared_arg.is_symlink():
            raise inv04.CaptureError("shared kubeconfig must not be a symlink")
        shared = shared_arg.resolve(strict=True)
        helper_arg=args.oci_layout_helper.expanduser().absolute()
        if helper_arg.is_symlink(): raise inv04.CaptureError("layout helper must not be a symlink")
        helper = helper_arg.resolve(strict=True)
        scout_arg=args.scout_binary.expanduser().absolute() if args.scout_binary else None
        if scout_arg and scout_arg.is_symlink(): raise inv04.CaptureError("Scout executable must not be a symlink")
        scout = scout_arg.resolve(strict=True) if scout_arg else None
        if not shared.is_file() or not helper.is_file() or not os.access(helper, os.X_OK):
            raise inv04.CaptureError("shared config or supplied executable is invalid")
        helper_sha=verify_hash(helper,args.expected_layout_helper_sha256)
        if scout:
            if not args.expected_scout_sha256 or not re.fullmatch(r"[0-9a-f]{64}",args.expected_scout_sha256) or not args.scout_source_revision or not re.fullmatch(r"[0-9a-f]{40,64}",args.scout_source_revision):
                raise inv04.CaptureError("optional Scout binary requires reviewed SHA-256 and source revision")
            if not scout.is_file() or not os.access(scout,os.X_OK) or inv04.sha256(scout.read_bytes())!=args.expected_scout_sha256:
                raise inv04.CaptureError("supplied Scout hash mismatch")
        elif args.expected_scout_sha256 or args.scout_source_revision:
            raise inv04.CaptureError("Scout hash/source arguments require --scout-binary")
        out = prepare_output(args.output_dir)
        shared_before = inv04.sha256(shared.read_bytes())
        started_at=inv04.utc_now()
        env = dict(os.environ)
        inv04.require_local_docker(env)
        inv04.require_kind_version(inv04._call("kind", ["version"], 10).decode().strip())
        inv04.verified_node_repo_digest(inv04._call("docker", ["image", "inspect", inv04.NODE_IMAGE], 15))
        cluster = owned_cluster_name()
        if not re.fullmatch(r"scout-rul04-[0-9a-z-]+",cluster) or len(cluster+"-control-plane")>63:
            raise inv04.CaptureError("refusing invalid generated cluster ownership name")
        if cluster in inv04._call("kind", ["get", "clusters"], 15).decode().splitlines():
            raise inv04.CaptureError("refusing to reuse cluster")
        temp = Path(tempfile.mkdtemp(prefix="scout-rul04-private-")); os.chmod(temp, 0o700)
        admin = temp / "admin.kubeconfig"; observer = temp / "observer.kubeconfig"
        env["KUBECONFIG"] = str(admin)
        create_attempted = False; cleaned = False; errors = []; private_before={}; private_before_cleanup={}
        observations = []
        inv04._write(out / "literal-manifests.yaml", MANIFEST)
        desired = out / "desired-statefulset.yaml"
        inv04._write(desired, MANIFEST.split(b"---\n")[-1])
        layout = out / "oci-layout"
        bundle_ref = ""
        layout_helper_sha = helper_sha
        git=inv04._command("git"); repo=HERE.parents[1]
        status_code,status_bytes,_=inv04.run_bounded([git,"-C",str(repo),"status","--porcelain"],5,max_output=16384)
        if status_code or status_bytes.strip(): raise inv04.CaptureError("capture source checkout must be clean")
        rev_code,rev_bytes,_=inv04.run_bounded([git,"-C",str(repo),"rev-parse","HEAD"],5,max_output=4096)
        if rev_code: raise inv04.CaptureError("could not capture source revision")
        capture_revision=rev_bytes.decode("ascii").strip()
        marker={"schema":"rul04-owned-cluster-marker.v1","clusterName":cluster,"ownerPid":os.getpid(),
            "sourceRevision":capture_revision,"createdAt":inv04.utc_now()}
        marker_bytes=(json.dumps(marker,sort_keys=True,indent=2)+"\n").encode()
        inv04._write(out/"owned-cluster-marker.json",marker_bytes)
        marker_sha=inv04.sha256(marker_bytes)
        signal_state = inv04._arm_capture_signals()
        try:
            create_attempted = True
            code, stdout, stderr = inv04.run_bounded([inv04._command("kind"), "create", "cluster", "--name", cluster,
                "--image", inv04.NODE_IMAGE, "--kubeconfig", str(admin), "--wait", "120s"], 180, env)
            admin_bytes=admin.read_bytes() if admin.exists() else b""
            if not credentials_absent(stdout+b"\n"+stderr,"",private_material({},admin_bytes)):
                raise inv04.CaptureError("kind creation diagnostics contain private config")
            inv04._write(out / "kind-create-stdout.txt", stdout); inv04._write(out / "kind-create-stderr.txt", stderr)
            if code: raise inv04.CaptureError("owned cluster creation failed")
            kubectl = inv04._command("kubectl")
            code, helper_out, helper_err = inv04.run_bounded([str(helper), str(desired), str(layout)], 30, env, max_output=65536)
            if inv04.sha256(helper.read_bytes())!=helper_sha: raise inv04.CaptureError("layout-helper executable changed during run")
            if code: raise inv04.CaptureError("local OCI layout helper failed")
            if not credentials_absent(helper_out+b"\n"+helper_err,"",private_material({},admin.read_bytes())):
                raise inv04.CaptureError("layout-helper output contains private config")
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
            check_server(server)
            token=inv04._call("kubectl", ["--context", "kind-"+cluster, "create", "token", "rul04-observer", "-n", NAMESPACE, "--duration=900s"], 15, env).decode().strip()
            inv04._write(observer, observer_config(server, ca64, token)); os.chmod(observer, 0o600)
            admin_private=admin.read_bytes()
            private_values=private_material(raw,admin_private)+(observer.read_bytes(),)
            private_before={"admin":inv04.sha256(admin_private),"observer":inv04.sha256(observer.read_bytes())}
            paths=[("statefulset.json", f"/apis/apps/v1/namespaces/{NAMESPACE}/statefulsets/{WORKLOAD}"),
                   ("pods.json", f"/api/v1/namespaces/{NAMESPACE}/pods")]
            ss_info=None
            for name,path in paths:
                validation=capture_raw_observation(name,path,server,ca,token,ss_info,observations,out,
                    private_values)
                if name=="statefulset.json": ss_info=validation
            if scout:
                argv=[str(scout),"release","check","--bundle",bundle_ref,"--oci-layout",str(layout),
                    "--controller",f"StatefulSet/{WORKLOAD}","--api-version","apps/v1","--controller-namespace",NAMESPACE,
                    "--kube-context","rul04-observer","--check-running-image","--format","json"]
                code, stdout, stderr=inv04.run_bounded(argv,45,dict(env,KUBECONFIG=str(observer)))
                if inv04.sha256(scout.read_bytes())!=args.expected_scout_sha256: raise inv04.CaptureError("Scout executable changed during capture")
                if not credentials_absent(stdout,token,private_values) or not credentials_absent(stderr,token,private_values):
                    raise inv04.CaptureError("Scout output contains private credential material")
                inv04._write(out/"scout-derived-output.json",stdout)
                inv04._write(out/"scout-derived-stderr.txt",stderr)
                derived_record={"argv":argv,"exitCode":code,"stdoutSha256":inv04.sha256(stdout),
                    "stderrSha256":inv04.sha256(stderr),"stdoutFile":"scout-derived-output.json",
                    "stderrFile":"scout-derived-stderr.txt","isRawModelEvidence":False}
                observations.append({"derivedScout":derived_record})
                derived_record["validation"]=validate_scout_result(code,stdout)
        except BaseException as e:
            errors.append(str(e) if isinstance(e, inv04.CaptureError) else type(e).__name__)
        finally:
            inv04._suppress_capture_signals(signal_state)
            for label,path in (("admin",admin),("observer",observer)):
                try: private_before_cleanup[label]=inv04.sha256(path.read_bytes())
                except OSError: private_before_cleanup[label]=""
                if label in private_before and private_before_cleanup[label]!=private_before[label]: errors.append("private "+label+" kubeconfig changed during capture")
            try:
                marker_check=json.loads((out/"owned-cluster-marker.json").read_bytes())
                owns=create_attempted and owns_cluster(marker_check,cluster,os.getpid())
                if owns and cluster in inv04._call("kind",["get","clusters"],15).decode().splitlines():
                    inv04._call("kind",["delete","cluster","--name",cluster,"--kubeconfig",str(admin)],90,env)
                cleaned=cluster not in inv04._call("kind",["get","clusters"],15).decode().splitlines()
            except Exception: cleaned=False
            shutil.rmtree(temp,ignore_errors=True)
            inv04._restore_capture_signals(signal_state)
        shared_after=inv04.sha256(shared.read_bytes())
        if not cleaned: errors.append("owned cluster cleanup was not verified")
        if shared_before!=shared_after: errors.append("shared kubeconfig changed")
        if scout and inv04.sha256(scout.read_bytes())!=args.expected_scout_sha256: errors.append("Scout binary hash changed after execution")
        if inv04.sha256(helper.read_bytes())!=helper_sha: errors.append("layout-helper hash changed after execution")
        provenance={"schema":"rul04-image-identity-capture.v1","sourceRevision":capture_revision,
            "layoutHelperSourceRevision":args.layout_helper_source_revision,
            "scoutSourceRevision":args.scout_source_revision,
            "captureScriptSha256":inv04.sha256(Path(__file__).read_bytes()),"layoutHelperSha256":layout_helper_sha,
            "scoutBinarySha256":inv04.sha256(scout.read_bytes()) if scout else None,
            "ociLayoutHelperSha256":layout_helper_sha,
            "kindVersion":inv04.KIND_VERSION,"nodeImage":inv04.NODE_IMAGE,"clusterName":cluster,
            "ownedClusterMarkerSha256":marker_sha,
            "bundleRef":bundle_ref,"ociLayoutPath":str(layout),"layoutHelperSha256":layout_helper_sha,
            "startedAt":started_at,"endedAt":inv04.utc_now(),"atomicSnapshot":False,
            "observations":observations,"sharedKubeconfigSha256":{"before":shared_before,"after":shared_after},
            "privateKubeconfigSha256":{"before":private_before,"beforeCleanup":private_before_cleanup},
            "cleanupVerified":cleaned,"errors":errors,
            "limitations":["no Argo/Flux controller or applied-source proof","OCI ref pins authored configuration bytes only",
                "tag-only authored image cannot be matched to immutable intended digest from runtime imageID alone",
                "two sequential namespace-scoped read-only API responses; not atomic or cluster-complete"]}
        inv04._write(out/"provenance.json",json.dumps(provenance,sort_keys=True,indent=2).encode()+b"\n")
        if errors: raise inv04.CaptureError("capture failed; inspect retained provenance.json")
        return 0
    except (inv04.CaptureError,OSError,KeyError,ValueError) as e:
        try:
            out_candidate=locals().get("out")
            if isinstance(out_candidate,Path) and out_candidate.is_dir() and not (out_candidate/"provenance.json").exists():
                inv04._write(out_candidate/"capture-failure.json",(json.dumps({"schema":"rul04-image-identity-failure.v1",
                    "error":str(e),"recordedAt":inv04.utc_now()},sort_keys=True,indent=2)+"\n").encode())
        except OSError: pass
        print(str(e),file=sys.stderr); return 1


if __name__ == "__main__":
    raise SystemExit(main())
