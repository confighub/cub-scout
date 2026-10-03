"""Source-derived answer vectors. Imports only pure validators; never runs capture."""
from __future__ import annotations

import importlib.util
import hashlib
import json
from pathlib import Path
import re


def load_module(path: Path):
    spec = importlib.util.spec_from_file_location("remaining_" + path.parent.name.replace("-", "_"), path)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    return module


def require(condition, message):
    if not condition:
        raise AssertionError(message)


class EvidenceAnswers:
    def __init__(self, prepared: Path, repo: Path, yaml_load):
        self.prepared, self.repo, self.yaml_load = prepared, repo, yaml_load

    def raw(self, case, name):
        return (self.prepared / "arms/without/cases" / case / "cluster" / name).read_bytes()

    def data(self, case, name):
        return json.loads(self.raw(case, name))

    def yaml(self, case, name):
        return self.yaml_load(self.raw(case, name).decode())

    def validator(self, package, filename="capture.py"):
        return load_module(self.repo / "evals" / package / filename)

    def answer(self, case):
        return getattr(self, case.lower().replace("-", "_"))()

    def inv_04(self):
        c = "INV-04"
        scope = self.data(c, "capture-scope.json")
        validator = self.validator("inv04-rbac")
        results, objects = [], []
        for request in scope["requests"]:
            raw = self.raw(c, request["file"])
            namespace = request["path"].split("/")[5]
            result = validator.validate_api_response(namespace, request["httpStatus"], raw)
            results.append((namespace, result))
            if result["result"] == "readable_populated":
                objects = json.loads(raw)["items"]
        require([r[1]["result"] for r in results] == ["readable_populated", "readable_empty", "denied"], "INV-04 request scope changed")
        require(any("not an orphan conclusion" in limit for limit in scope["limitations"]), "INV-04 ownership limit missing")
        visible = ",".join(f"{m['namespace']}/{m['name']}@{m['uid']}#{m['resourceVersion']}" for m in sorted((o["metadata"] for o in objects), key=lambda m: m["name"]))
        return dict(populated_namespace=results[0][0], visible_deployments=visible,
                    empty_namespace=results[1][0], empty_deployment_count=str(results[1][1]["items"]),
                    denied_namespace=results[2][0], denied_result="FORBIDDEN", denied_deployment_count="UNKNOWN",
                    denied_ownership="UNKNOWN", readable_objects_orphan_status="NOT_ESTABLISHED", inventory_completeness="PARTIAL",
                    evidence="capture-scope.json+" + "+".join(r["file"] for r in scope["requests"]))

    def hlt_01(self):
        projection = self.yaml("HLT-01", "sveltos-health.yaml")
        lines = {line["source_line"]: line["text"] for line in projection["excerpt_lines"]}
        profile = lines[116].rsplit(": ", 1)[1]
        condition = re.search(r"ClusterHealthCheck on [^:]+: (\w+)", lines[114]).group(1)
        report = json.loads("\n".join(lines[n] for n in range(127, 132)))
        require(projection["scope"]["atomic_snapshot"] is False, "HLT-01 snapshot limit missing")
        require("sha256:" not in json.dumps(report), "HLT-01 report unexpectedly supplies release identity")
        revisions = re.findall(r"sha256:([0-9a-f]+)", "\n".join(lines.values()))
        require(revisions and all(len(digest) < 64 for digest in revisions), "HLT-01 transcript release identities no longer shortened")
        return dict(profile_state=profile, health_check=condition, sync_status=report["syncStatus"],
                    report_health=report["healthStatus"], release_identity="UNKNOWN")

    def hlt_02(self):
        c = "HLT-02"
        ks = self.data(c, "kustomization-end.json")
        dep = self.data(c, "deployment.json")
        chain = self.validator("flux-ready-without-health", "validate_capture.py").validate_capture(
            dep, self.data(c, "replicasets.json"), self.data(c, "pods.json"))
        ready = next(x for x in ks["status"]["conditions"] if x["type"] == "Ready")
        require(ready["status"] == "True" and ready["observedGeneration"] == ks["metadata"]["generation"], "HLT-02 Ready is not current")
        require(ks["spec"]["wait"] is False and not ks["spec"].get("healthChecks") and not ks["spec"].get("healthCheckExprs"), "HLT-02 workload checks changed")
        require(self.data(c, "provenance.json")["atomicSnapshot"] is False, "HLT-02 capture became atomic")
        require(dep["status"]["observedGeneration"] == dep["metadata"]["generation"] and dep["status"].get("availableReplicas", 0) == 0, "HLT-02 workload failure no longer current")
        source = self.data(c, "gitrepository.json")["status"]["artifact"]["revision"]
        applied = ks["status"]["lastAppliedRevision"]
        return dict(ready_current_generation="YES", wait="FALSE", health_checks="ABSENT_OR_EMPTY", deployment_current_generation="UNAVAILABLE",
                    uid_chain=f"deployment:{chain['deploymentUID']};replicaset:{chain['replicaSetUID']};pod:{chain['podUID']}",
                    source_revision=source, applied_revision=applied, revision_binding="MATCH" if source == applied else "MISMATCH",
                    ready_proves_workload_healthy="NO", observation_scope="SEQUENTIAL_NOT_ATOMIC", current_time_claim="CAPTURE_ONLY",
                    application_level_check="NOT_RECORDED")

    def hlt_03(self):
        c = "HLT-03"
        receipt = self.yaml(c, "receipt.yaml")["spec"]
        status = self.data(c, "argocd-child.json")["status"]
        residual = [r for r in status["resources"] if r["kind"] == "Ingress"]
        require(len(residual) == 1, "HLT-03 residual scope changed")
        r = residual[0]
        require(all(leg["runtime"]["result"] == "pass" for leg in receipt["legs"].values()), "HLT-03 workload convergence changed")
        limits = self.data(c, "source-provenance.json")["limits"]
        require(all(limits[k] is False for k in ("atomic_join_established", "current_live_state_established", "full_kubernetes_object_snapshot")), "HLT-03 historical scope missing")
        require(not r.get("health", {}).get("message"), "HLT-03 unexpectedly supplies residual cause")
        return dict(receipt_outcome=receipt["result"].upper(), workload_outcome="PASS", child_sync=status["sync"]["status"].upper(),
                    child_health=status["health"]["status"].upper(), residual_identity=f"{r['kind']}/{r['namespace']}/{r['name']}",
                    residual_sync=r["status"].upper(), residual_health=r["health"]["status"].upper(), residual_cause="UNKNOWN",
                    observation_scope="HISTORICAL_SEQUENTIAL_NONATOMIC_NOT_CURRENT_NOT_FULL_K8S_SNAPSHOT")

    def hlt_04(self):
        c = "HLT-04"
        metadata = self.data(c, "source-metadata.json")
        receipt = self.data(c, "synthetic-check-execution-receipt.json")
        require(metadata["evidence_class"] == "authored_synthetic_json" and receipt["synthetic"] is True, "HLT-04 synthetic boundary missing")
        require(metadata["other_authored_fixture_artifacts"] == [metadata["receipt_file"]] and metadata["receipt_file"] not in metadata["producer_input_artifacts"], "HLT-04 receipt became producer input")
        limits = (self.repo / "evals/sveltos-hlt-04-report-freshness/README.md").read_text()
        require("revision is time-inferred" in limits and "not called a last-check time" in limits and "does not imply renewed health checks" in limits,
                "HLT-04 revision/transition/renewal interpretation limits changed")
        records = []
        for record in self.data(c, "scenario-evidence.json")["records"]:
            before = json.loads(record["held_before"]) if record["held_before"] else {}
            after = json.loads(record["held_after"])
            computed = record["computed_report"]
            for name, data in record["raw_inputs"].items():
                raw = json.dumps(data, sort_keys=True, separators=(",", ":")).encode()
                expected = metadata["producer_input_sha256_by_record"][record["id"]][name]
                require(hashlib.sha256(raw).hexdigest() == expected, "HLT-04 input hash mismatch")
            require(computed["observedAt"] == record["synthetic_now"], "HLT-04 computed clock binding changed")
            if record["write_patch"] is None:
                require(record["held_before"] == record["held_after"], "HLT-04 withheld write changed held report")
            else:
                patch = json.loads(record["write_patch"])
                require(json.loads(patch["Annotations"]["confighub.com/live-status"]) == after == computed, "HLT-04 patch/report mismatch")
            records.append(dict(id=record["id"], held_before_observed_at=before.get("observedAt", "MISSING"),
                                held_after_observed_at=after["observedAt"], computed_observed_at=computed["observedAt"],
                                write_applied="YES" if record["write_patch"] else "NO", health_status=computed["healthStatus"],
                                sync_status=computed["syncStatus"], revision=computed["revision"]))
        require(len(records) == metadata["record_count"] == 8, "HLT-04 record count changed")
        return dict(records=records, check_receipt="AUTHORED_SYNTHETIC_NOT_CONTROLLER_EVIDENCE", check_receipt_consumed="NO",
                    last_transition_time_role="CONDITION_TRANSITION_NOT_CHECK_EXECUTION", applied_release_digest_proven="NO",
                    evidence_scope="SYNTHETIC_SOURCE_CONTRACT_ONLY", renewal_proves_new_check="NO", future_timestamp_proves_freshness="NO")

    def pre_01(self):
        c = "PRE-01"
        scope = self.data(c, "capture-scope.json")
        validator = self.validator("pre01-crd")
        validated = {r["rawFile"]: validator.validate_raw(r["path"], r["httpStatus"], self.raw(c, r["rawFile"]), r["phase"]) for r in scope["requests"]}
        require(validated["absent-crd.json"]["status"] == validated["absent-after-apply-crd.json"]["status"] == "absent", "PRE-01 typed CRD absence changed")
        require(validated["present-crd.json"]["established"] and validated["present-discovery.json"]["status"] == "registered", "PRE-01 registration changed")
        require(validated["present-before-apply-servicemonitor.json"]["status"] == "absent", "PRE-01 object not-found changed")
        ops = scope["dependentOperations"]
        require([op["exitCode"] for op in ops] == [1, 0], "PRE-01 operation outcomes changed")
        require('no matches for kind "ServiceMonitor"' in self.raw(c, ops[0]["stderrFile"]).decode(), "PRE-01 missing-GVK failure missing")
        require("created" in self.raw(c, ops[1]["stdoutFile"]).decode(), "PRE-01 successful creation missing")
        require(any("does not measure" in x for x in scope["scope"]["limitations"]), "PRE-01 health limit missing")
        obj = self.data(c, "present-servicemonitor.json")
        manifest = self.yaml(c, "servicemonitor.normalized.yaml")
        validator.validate_service_monitor(manifest)
        m = obj["metadata"]
        identity = f"{obj['apiVersion']}|{obj['kind']}|{m['namespace']}/{m['name']}"
        files = [r["rawFile"] for r in scope["requests"]] + ["servicemonitor.normalized.yaml"] + [op[key] for op in ops for key in ("stdoutFile", "stderrFile")]
        return dict(crd_name=self.data(c, "present-crd.json")["metadata"]["name"], dependent_operation="kubectl apply|" + identity,
                    crd_before="ABSENT", pre_setup_apply="MISSING_GVK_FAILURE", crd_after_failed_apply="ABSENT", crd_registration="ESTABLISHED",
                    api_discovery="REGISTERED", object_before_successful_apply="TYPED_NOT_FOUND", post_setup_apply="CREATED",
                    object_identity=identity + f"|{m['uid']}|{m['resourceVersion']}", health_claim="NOT_PROVEN", evidence="capture-scope.json+" + "+".join(files))

    def pre_03(self):
        c = "PRE-03"
        parent, child = (self.data(c, name) for name in ("argocd-core-root.json", "argocd-core-child.json"))
        scope = self.data(c, "capture-scope.json")
        pod = self.raw(c, "pod-describe.txt").decode()
        events = self.raw(c, "events.txt").decode()
        child_id = f"{child['metadata']['namespace']}/{child['metadata']['name']}"
        tracked = next(r for r in parent["status"]["resources"] if r["kind"] == "Application" and r["name"] == child["metadata"]["name"])
        statefulset = next(r for r in child["status"]["resources"] if r["kind"] == "StatefulSet" and r["name"] == "spark-worker")
        require("State:           Waiting" in pod and "Reason:        ImagePullBackOff" in pod and "ErrImagePull" in events, "PRE-03 image pull failure missing")
        image = re.search(r"(?m)^\s+Image:\s+(\S+)", pod).group(1)
        message = re.search(r'Failed to pull image ".*?: not found', pod).group(0)
        require("no Crossplane" in scope["scope"]["crossplane"] and scope["scope"]["current_state"] == "not established", "PRE-03 controller/current scope changed")
        require(scope["timestamps"]["response_capture_times"] == "not supplied per file", "PRE-03 capture timing scope changed")
        require(scope["cleanup"]["source_receipt_cleanup_result"] != "pass" and scope["cleanup"]["source_receipt_lifecycle_value"] == "cleaned-up", "PRE-03 cleanup conflict missing")
        tree = self.raw(c, "argocd-core-child-tree.txt").decode()
        require("spark-worker-0" in tree and "spark-worker" in tree, "PRE-03 tree nesting missing")
        return dict(parent_application=f"{parent['metadata']['namespace']}/{parent['metadata']['name']}", parent_sync_status=parent["status"]["sync"]["status"],
                    parent_health_status=parent["status"]["health"]["status"], parent_tracked_child=f"{tracked['group']}/v1alpha1 {tracked['kind']} {tracked['namespace']}/{tracked['name']}",
                    child_application=child_id, child_tracking_id=child["metadata"]["annotations"]["argocd.argoproj.io/tracking-id"],
                    child_sync_status=child["status"]["sync"]["status"], child_health_status=child["status"]["health"]["status"],
                    failing_statefulset=f"{statefulset['group']}/v1 StatefulSet {statefulset['namespace']}/{statefulset['name']}",
                    failing_pod=re.search(r"(?m)^Namespace:\s+(\S+)", pod).group(1) + "/" + re.search(r"(?m)^Name:\s+(\S+)", pod).group(1),
                    pod_state="WAITING_IMAGE_PULL_BACK_OFF", pod_image=image, pod_failure_class="ERRIMAGEPULL_AND_IMAGEPULLBACKOFF", pod_failure_message=message,
                    parent_status_proves_child_health="NO", crossplane_evidence="NO_CROSSPLANE_CHAIN_IN_PROVIDED_FILES", observation_scope="HISTORICAL_SEQUENTIAL_NON_ATOMIC_NOT_CURRENT",
                    capture_timing="SOURCE_OBJECT_TIMES_ONLY", cleanup_status="UNCONFIRMED_CONFLICTING_CLEANUP_FIELDS",
                    evidence_files="capture-scope.json+argocd-core-root.json+argocd-core-child.json+argocd-core-root-tree.txt+argocd-core-child-tree.txt+pod-describe.txt+events.txt")

    def pre_04(self):
        c = "PRE-04"
        spec = self.data(c, "desired-matrix.json")["spec"]
        config = self.yaml(c, "config.yaml")
        selected = [r for r in spec["rows"] if r["component"] == "cert-manager"]
        hubs = [r for r in selected if r["clusterType"] == "hub"]
        spokes = [r for r in selected if r["clusterType"] == "spoke"]
        require(len(hubs) == 1 and len(spokes) == 3, "PRE-04 placement scope changed")
        require(spec["evidence"]["liveReads"] == [] and spec["evidence"]["parsedObservationCells"] == 0, "PRE-04 unexpectedly includes live reads")
        for row in selected:
            require(next(x for x in config["clusters"] if x["name"] == row["cluster"])["services"]["cert-manager"]["status"] == "enabled", "PRE-04 configured selection mismatch")
            require(row["proofStatus"] == "rendered-only" and row["observedVersion"] == row["argoSyncState"] == row["healthState"] == "Unknown" and row["readiness"]["result"] == "unknown", "PRE-04 unknown evidence changed")
        require(any("not a live sync or workload assertion" in x for x in spec["claimBoundary"]), "PRE-04 desired/live interpretation limit missing")
        argo = [r for r in spec["rows"] if r["component"] == "argo-cd" and r["clusterType"] == "spoke"]
        require(all(r["presence"] == "hub-managed" and r["argoSyncState"] == "Unknown" for r in argo), "PRE-04 spoke Argo scope changed")
        return dict(component=hubs[0]["component"], selected_version=hubs[0]["selectedVersion"], hub_cluster=hubs[0]["cluster"], hub_intent="selected",
                    spoke_clusters=",".join(r["cluster"] for r in spokes), spoke_intent="selected", cert_manager_observed_version="UNKNOWN",
                    cert_manager_argo_sync="UNKNOWN", cert_manager_health="UNKNOWN", cert_manager_readiness="UNKNOWN", spoke_argo_placement=argo[0]["presence"],
                    spoke_argo_live_observation="UNKNOWN", unknown_interpretation="not_disabled_unmanaged_or_unhealthy", evidence="desired-matrix.json+config.yaml")

    def rul_02(self):
        c = "RUL-02"
        steps = self.data(c, "rul02-cache-replay.json")["steps"]
        require(len(steps) == 9, "RUL-02 replay sequence changed")
        scope = self.data(c, "capture-scope.json")
        require(scope["inputKind"] == "authored_httptest_responses_and_clock" and any("does not establish push or automatic" in x for x in scope["limitations"]), "RUL-02 cache limit missing")
        a = {}
        def digest(obj):
            image = obj["spec"]["template"]["spec"]["containers"][0]["image"]
            return image.split("@", 1)[1] if "@" in image else "UNKNOWN"
        for prefix, idx in (("initial", 0), ("repeat", 1), ("refresh", 3), ("same_uid_refresh", 4), ("after_expiry", 5)):
            step = steps[idx]
            a[prefix + "_read"] = step["evidence"]["cache"].upper()
            a[prefix + "_uid"] = step["returnedObject"]["metadata"]["uid"]
            a[prefix + "_digest"] = digest(step["returnedObject"])
            if idx in (0, 1):
                for key in ("observedAt", "expiresAt"):
                    a[prefix + ("_observed_at" if key == "observedAt" else "_expires_at")] = step["evidence"][key]
        changed = json.loads(steps[2]["input"]["configuredObjectResponse"]["body"])
        a.update(changed_response_before_refresh_uid=changed["metadata"]["uid"], changed_response_before_refresh_digest=digest(changed),
                 changed_response_before_refresh_served="NO" if steps[2]["evidence"]["reads"]["object"] == 0 else "YES",
                 returned_before_refresh_uid=steps[2]["returnedObject"]["metadata"]["uid"], returned_before_refresh_digest=digest(steps[2]["returnedObject"]))
        require(not steps[6]["returnedObject"]["metadata"].get("uid"), "RUL-02 missing UID control changed")
        require(all(not s["evidence"]["available"] and s["returnedObject"] is None for s in steps[7:]), "RUL-02 failed refresh served stale evidence")
        a.update(missing_uid="UNKNOWN", missing_digest=digest(steps[6]["returnedObject"]), failed_refresh="ERROR", read_after_failed_refresh="ERROR",
                 automatic_invalidation="NOT_DEMONSTRATED", push_invalidation="NOT_DEMONSTRATED", freshness_guarantee="NOT_ESTABLISHED", evidence="rul02-cache-replay.json+capture-scope.json")
        # This frozen answer contract declares field order.
        order = ("initial_read initial_uid initial_digest initial_observed_at initial_expires_at repeat_read repeat_uid repeat_digest repeat_observed_at repeat_expires_at "
                 "changed_response_before_refresh_uid changed_response_before_refresh_digest changed_response_before_refresh_served returned_before_refresh_uid returned_before_refresh_digest "
                 "refresh_read refresh_uid refresh_digest same_uid_refresh_read same_uid_refresh_uid same_uid_refresh_digest after_expiry_read after_expiry_uid after_expiry_digest "
                 "missing_uid missing_digest failed_refresh read_after_failed_refresh automatic_invalidation push_invalidation freshness_guarantee evidence").split()
        return {key: a[key] for key in order}

    def rul_03(self):
        c = "RUL-03"
        scope = self.data(c, "capture-scope.json")
        validator = self.validator("rul03-context")
        observations = scope["observations"]
        validated = [validator.validate_observation(r["context"], r["httpStatus"], self.raw(c, r["rawFile"])) for r in observations]
        require(scope["atomicSnapshot"] is False and all(r["explicitContext"] for r in observations), "RUL-03 explicit/sequential scope changed")
        mapping = self.data(c, "observer-context-map.json")
        require(mapping["currentContext"] == scope["observerCurrentContext"]["before"] == scope["observerCurrentContext"]["after"], "RUL-03 ambient context changed")
        d = validated[1]["deployment"]
        return dict(denied_context=observations[0]["context"], denied_http_status=str(validated[0]["httpStatus"]), denied_list_result="FORBIDDEN", denied_inventory="UNKNOWN",
                    readable_context=observations[1]["context"], readable_list_count=str(validated[1]["itemCount"]), deployment_namespace=d["namespace"], deployment_name=d["name"],
                    deployment_uid=d["uid"], default_context_before=scope["observerCurrentContext"]["before"], default_context_after=scope["observerCurrentContext"]["after"],
                    observation_scope="SEQUENTIAL_NON_ATOMIC", evidence="capture-scope.json+observer-context-map.json+" + "+".join(r["rawFile"] for r in observations))

    def rul_04(self):
        c = "RUL-04"
        validator = self.validator("rul04-image-identity")
        ss = validator.validate_statefulset(self.raw(c, "statefulset.json"))
        pod = validator.validate_pods(self.raw(c, "pods.json"), ss)
        desired = self.yaml(c, "desired-statefulset.yaml")
        image = desired["spec"]["template"]["spec"]["containers"][0]["image"]
        require(image == ss["intendedImage"] and "@sha256:" not in image, "RUL-04 mutable authored intent changed")
        scope = self.data(c, "capture-scope.json")
        require(scope["capture"]["atomicSnapshot"] is False and any("No Argo/Flux" in x for x in scope["limitations"]), "RUL-04 applied-source limit missing")
        return dict(image_identity_verdict="UNKNOWN", unknown_reason="IMMUTABLE_INTENT_DIGEST_UNAVAILABLE", intended_image=image, statefulset_uid=ss["uid"],
                    pod_name=pod["podName"], pod_uid=pod["podUID"], pod_owner_uid=pod["ownerUID"], runtime_image=pod["runtimeImage"], runtime_image_id=pod["runtimeImageID"],
                    runtime_identity_status="OBSERVED", workload_health="READY_RUNNING", applied_source_binding="UNKNOWN", observation_scope="SEQUENTIAL_NON_ATOMIC",
                    evidence="capture-scope.json+desired-statefulset.yaml+pods.json+statefulset.json")
