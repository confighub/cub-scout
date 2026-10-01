"""Pure acceptance checks using emitted-schema synthetic CLI/provider fixtures."""
import base64
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest

HERE = Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
    return module


contract = load("combined_runtime_contract", HERE / "contract.py")
payload = load("combined_runtime_payload_fixture", HERE / "payload.py")
direct = load("combined_runtime_direct_parser", HERE.parent / "direct-cli-controls/probe.py")


def digest(raw): return hashlib.sha256(raw).hexdigest()


def map_fixture():
    scale = load("combined_scale_fixture", HERE.parent / "recorded-scale/preflight.py")
    resources = []
    for owner, count in scale.EXPECTED_COUNTS.items():
        for n in range(count):
            namespace, name = scale.NO_MARKER[n].split("/") if owner == "Native" else ("team-01", owner.lower() + str(n))
            resources.append({"apiVersion": "apps/v1", "kind": "Deployment", "namespace": namespace, "name": name, "owner": owner})
    return {"schema": "map-list-recorded.v1", "provenance": {"objectCount": 302, "sha256": scale.DEPLOYMENT_SHA256,
            "captureTime": "unknown", "captureCompleteness": "unknown"},
            "scope": {"apiVersion": "apps/v1", "kind": "Deployment", "namespacePrefix": "team-"},
            "selectedCount": 300, "excludedFromScopeCount": 2, "ownerCounts": scale.EXPECTED_COUNTS, "resources": resources}


def arm_payload(arm):
    treatment = arm == "treatment"
    tools = [{"name": "Read"}, {"name": "Bash"}]
    if treatment:
        tools += [{"name": "Skill"}, {"name": "mcp__cub-scout__map"}, {"name": "mcp__cub-scout__explain"}]
    plan = payload.tool_plan(arm, tools)
    listing = "<system-reminder>\nThe following skills are available for use with the Skill tool:\n\n" + "\n".join("- cub-scout:" + p.parent.name for p in sorted((HERE.parents[1] / "skills").rglob("SKILL.md"))) + "\n</system-reminder>"
    history = [{"role": "user", "content": [{"type": "text", "text": listing}]}] if treatment else []
    requests = []
    uses, results = [], []
    for index in range(1, len(plan) + 2):
        content_type, response = payload.provider_response(False, arm, index, tools)
        body_obj = {"stream": False, "tools": tools,
                    "system": " ".join("skill-%02d" % n for n in range(35)) if treatment else "",
                    "messages": copy.deepcopy(history)}
        body = json.dumps(body_obj, separators=(",", ":")).encode()
        record = {"ordinal": index, "kind": "accepted", "method": "POST", "path": "/v1/messages",
                  "status": 200, "authKind": "x-api-key", "body": body.decode(),
                  "bodyBase64": base64.b64encode(body).decode(), "bodyBytes": len(body),
                  "bodySha256": digest(body), "response_body_base64": base64.b64encode(response).decode(),
                  "responseBytes": len(response), "responseSha256": digest(response),
                  "response_content_type": content_type}
        requests.append(record)
        parsed_response = direct.strict_json(response)
        if parsed_response["stop_reason"] == "tool_use":
            call = parsed_response["content"][0]
            uses.append({"id": call["id"], "name": call["name"], "input": call["input"]})
            name = call["name"]
            if name in ("Task", "Agent"):
                result_content = [{"type": "text", "text": f"No such tool available: {name}"}]
                is_error = True
            elif name == "Read":
                result_content = [{"type": "text", "text": "eventTime: null\nfieldPath: spec.containers{cron}"}]
                is_error = False
            elif name.endswith("__map"):
                result_content = [{"type": "text", "text": json.dumps(map_fixture())}]
                is_error = False
            else:
                result_content = [{"type": "text", "text": "synthetic recorded result"}]
                is_error = False
            result = {"type": "tool_result", "tool_use_id": call["id"],
                      "content": result_content, "is_error": is_error}
            results.append({"id": call["id"], "is_error": is_error, "content": result_content})
            history.extend([{"role": "assistant", "content": [call]},
                            {"role": "user", "content": [result]}])
    terminal_event = {"type": "result", "subtype": "success", "is_error": False,
                      "result": "offline-probe-terminal"}
    events = []
    for use, result in zip(uses, results):
        events.append({"type": "assistant", "message": {"content": [{"type": "tool_use", **use}]}})
        events.append({"type": "user", "message": {"content": [{"type": "tool_result", "tool_use_id": result["id"], "content": result["content"], "is_error": result["is_error"]}]}})
    events.append(terminal_event)
    stdout = ("\n".join(json.dumps(event, separators=(",", ":")) for event in events) + "\n").encode()
    return {"schema": "combined-runtime-arm-payload.v1", "arm": arm, "status": "passed",
        "returnCode": 0, "runStatus": "completed", "processError": None,
        "stdoutBase64": base64.b64encode(stdout).decode(), "stderrBase64": "",
        "stdoutBytes": len(stdout), "stderrBytes": 0, "stdoutSha256": digest(stdout),
        "stderrSha256": digest(b""), "providerRequests": requests,
        "providerConnections": [{"ordinal": i, "kind": "accepted"} for i in range(1, len(requests)+1)],
        "providerReceivedCount": len(requests), "providerFailure": None,
        "providerAttemptsPassed": True, "cliToolUses": uses, "cliToolResults": results,
        "inventoryPassed": True, "fileReadPassed": True, "kubectlReadPassed": True,
        "refusalPassed": True, "apiRequests": 1, "apiCaptured": True, "apiThreadJoined": True,
        "skillsAdvertised": treatment, "skillsPassed": True,
        "skillSignal": {"advertisedCount": 35 if treatment else 0},
        "mcpMapPassed": True, "mcpMapResultValidated": True, "helmAvailable": True,
        "mcpMapResult": [{"source": "inline", "path": None, "bytes": len(json.dumps(map_fixture()).encode()),
                          "sha256": digest(json.dumps(map_fixture()).encode()), "bodyBase64": base64.b64encode(json.dumps(map_fixture()).encode()).decode()}] if treatment else [],
        "helm": {"argv": ["/tools/helm", "version", "--short"], "exitCode": 0,
                 "status": "completed", "stdoutBytes": 4, "stdoutSha256": digest(b"v4.1")},
        "terminalText": "offline-probe-terminal", "elapsedSeconds": 3,
        "containerLocalCleanup": {"providerThreadJoined": True, "apiThreadJoined": True}}


class ContractTests(unittest.TestCase):
    def test_accepts_actual_arm_payload_schema_for_both_arms(self):
        for arm in ("baseline", "treatment"):
            with self.subTest(arm=arm):
                facts = contract.validate_arm_payload(arm_payload(arm), arm)
                self.assertEqual(facts["status"], "passed")

    def test_payload_validator_rejects_incomplete_or_tampered_evidence(self):
        mutations = (
            lambda x: x.update(status="failed"),
            lambda x: x.update(returnCode=True),
            lambda x: x.update(terminalText="offline-probe-terminal", stdoutBase64=base64.b64encode(b'{"type":"assistant","text":"offline-probe-terminal"}\n').decode(), stdoutBytes=58, stdoutSha256=digest(b'{"type":"assistant","text":"offline-probe-terminal"}\n')),
            lambda x: x.update(providerReceivedCount=99),
            lambda x: x.update(providerConnections=[]),
            lambda x: x.update(skillsAdvertised=False),
        )
        for mutate in mutations:
            value = arm_payload("treatment"); mutate(value)
            with self.assertRaises(contract.ContractError): contract.validate_arm_payload(value, "treatment")

    def test_raw_cli_missing_tools_or_malformed_events_cannot_pass(self):
        for raw in (b'{"type":"result","subtype":"success","is_error":false,"result":"offline-probe-terminal"}\n',
                    base64.b64decode(arm_payload("baseline")["stdoutBase64"]) + b'{truncated\n'):
            value = arm_payload("baseline")
            value.update(stdoutBase64=base64.b64encode(raw).decode(), stdoutBytes=len(raw), stdoutSha256=digest(raw))
            with self.assertRaises(contract.ContractError): contract.validate_arm_payload(value, "baseline")

    def test_observed_token_count_decline_is_bounded_and_not_unknown_traffic(self):
        value = arm_payload("treatment")
        body = b'{"model":"synthetic","messages":[]}'
        row = {"ordinal": len(value["providerConnections"]) + 1, "kind": "token-count-declined",
               "method": "POST", "path": "/v1/messages/count_tokens?beta=true", "status": 404,
               "authKind": "x-api-key", "bodyBase64": base64.b64encode(body).decode(),
               "bodyBytes": len(body), "bodySha256": digest(body)}
        value["providerConnections"].append(row); value["providerReceivedCount"] += 1
        contract.validate_arm_payload(value, "treatment")
        for field, changed in (("path", "/unknown"), ("method", "GET"), ("authKind", None), ("status", 200), ("bodySha256", "0" * 64)):
            broken = copy.deepcopy(value); broken["providerConnections"][-1][field] = changed
            with self.assertRaises(contract.ContractError): contract.validate_arm_payload(broken, "treatment")
        value["providerConnections"].append({**row, "ordinal": row["ordinal"] + 1}); value["providerReceivedCount"] += 1
        with self.assertRaises(contract.ContractError): contract.validate_arm_payload(value, "treatment")

    def test_pair_validator_requires_matching_stages_and_explicit_binary_delta(self):
        arms = {}
        for name in ("baseline", "treatment"):
            file_plan = {"payload.py": {"bytes": 1, "sha256": "a" * 64},
                         "evidence/events.yaml": {"bytes": 1, "sha256": "b" * 64}}
            common = {"evidence/events.yaml": {"bytes": 1, "sha256": "b" * 64},
                      "evals/pre01.json": {"sha256": "d" * 64}, "_stageFiles": file_plan}
            files = sorted(file_plan)
            if name == "treatment":
                common["bin/cub-scout"] = {"sha256": "c" * 64}
                file_plan["cub-scout"] = {"bytes": 1, "sha256": "c" * 64}; files.append("cub-scout")
            observed = {k: v["sha256"] for k, v in file_plan.items()}
            stage_sha = digest(json.dumps(observed, sort_keys=True, separators=(",", ":")).encode())
            arms[name] = {"status": "passed", "validatedFacts": {"status": "passed"},
                "stageFacts": {"common": common}, "stageInventory": {"files": files, "sha256": stage_sha}}
        self.assertTrue(contract.validate_pair_staging(arms)["sharedSourcesEqual"])
        broken = copy.deepcopy(arms); broken["treatment"]["stageFacts"]["common"]["evidence/events.yaml"]["sha256"] = "d" * 64
        with self.assertRaises(contract.ContractError): contract.validate_pair_staging(broken)
        broken = copy.deepcopy(arms); broken["baseline"]["stageInventory"]["files"].append("cub-scout")
        with self.assertRaises(contract.ContractError): contract.validate_pair_staging(broken)

    def test_strict_json_rejects_duplicate_and_nonfinite_values(self):
        for raw in (b'{"schema":"a","schema":"b"}', b'{"n":NaN}', b"\xff"):
            with self.assertRaises(contract.ContractError): contract.strict_json(raw)


if __name__ == "__main__": unittest.main()
