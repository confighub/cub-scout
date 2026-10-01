"""Strict evidence contract for the combined recorded runtime pair.

This module validates receipts only. It never starts a CLI, container, network
listener, MCP server, or provider. The runner must preserve failures as data;
this validator does not infer missing events from a successful exit code.
"""
from __future__ import annotations

import hashlib
import importlib.util
import json
import math
import re
from pathlib import Path

SCHEMA = "combined-recorded-runtime.v1"
ARMS = ("baseline", "treatment")
RAW_FILES = ("configmaps.yaml", "deployments.yaml", "events.yaml", "namespaces.yaml",
             "pods.yaml", "replicasets.yaml", "services.yaml")
RAW_FILE_HASHES = {
    "configmaps.yaml": "fc7bce49a84edb5065e4c441bc9936ee906ee976567dbd80bae5fe282f4eb68f",
    "deployments.yaml": "822716e41eaf59674cec8b52913b2613c76932b578c45d54285f71747dd228b6",
    "events.yaml": "3764ff2668f35d131c98ae3cd8acbf40ae06c32888424f4e58de7bf6e0f707bd",
    "namespaces.yaml": "9c02dbb607f9ffd7e232292bfa5fb3fb1167f67b7b4ca9031c14a7cb1993b1f0",
    "pods.yaml": "eb4934b6b747acc370291984172c3b1729ee56eceb7e1d516b2b0eaf29a0d8ae",
    "replicasets.yaml": "e868ab510d43ca66ab3203b6d735ad8edc66a5fb28c497ad48e9c23707f5de68",
    "services.yaml": "c7984a35c6742a8bdf4b5d1b6d32fcfed0f04d649c069a05819fa96e9b8ae57a",
}
MAX_REQUESTS = 8
MAX_REQUEST_BODY = 1 * 1024 * 1024
MAX_RAW_OUTPUT = 8 * 1024 * 1024
MAX_EXECUTION_SECONDS = 120
MAX_CLEANUP_SECONDS = 30
MAX_PAIR_SECONDS = 300
HEX64 = re.compile(r"[0-9a-f]{64}\Z")
SOURCE_PINS = {
    "image": ("sha256:cdbd05fb6f457ca275ff51ce00d93d865ca0b6a25f5ffb08262d94f6835771e5", "cdbd05fb6f457ca275ff51ce00d93d865ca0b6a25f5ffb08262d94f6835771e5"),
    "claude": ("2.1.274-linux-arm64", "2db904daea17addff9de557ba26a725916888aa7b546e2c5dd989c20d9d49ab3"),
    "kubectl": ("v1.36.0-linux-arm64", "9f9d9c44a7b5264515ac9da5991584e2395bd50662e651132337e7b4d0c56f8f"),
    "helm": ("v4.1.4-linux-arm64", "4d6e3a69e6203094d564d5d4e94325b1f7209421dc7e832a96d2510295e03f1d"),
    "cubScout": ("0d0fd7d54e5b3b1f16d27df6fa299950388873d6-linux-arm64", "5dac8765612592b60ec076f102c1810fbbce94b52236b3577ab3f65219e17fa6"),
    "recordedScale": ("7f5d38e8881d2f6b22e132b3da57ca60f483e703", "e139701bf9d894ca9dd16ddb302fe0e4f28f3122922030cd9cdda57e8eb3fa22"),
    "pre01": ("2026-10-01-recorded-api", "5585ecd0298e2b6f4201a638cdc0aeab9cdeb0dfab38d3b52e090cc3cb8f5c0a"),
}
SKILL_COUNT = 35
SKILL_TREE_SHA256 = "9f31fd677a3490f60a6d1c97b315f8f3d7ee7db6dd5285b47051e049d3cc8b8b"


class ContractError(ValueError):
    pass


def _probe_parser():
    path = Path(__file__).resolve().parents[1] / "direct-cli-controls/probe.py"
    spec = importlib.util.spec_from_file_location("combined_direct_cli_parser", path)
    _need(spec is not None and spec.loader is not None, "pinned direct CLI event parser is unavailable")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _need(ok: bool, message: str) -> None:
    if not ok:
        raise ContractError(message)


def _strict_int(value, label: str, minimum=0, maximum=None) -> int:
    _need(type(value) is int and value >= minimum and
          (maximum is None or value <= maximum), f"{label} is outside its integer bound")
    return value


def _hash(value, label: str) -> str:
    _need(isinstance(value, str) and HEX64.fullmatch(value) is not None,
          f"{label} must be a lowercase SHA-256")
    return value


def _keys(value, expected, label):
    _need(isinstance(value, dict) and set(value) == set(expected),
          f"{label} has missing or unknown fields")


def _equal_raw(left, right):
    _need(isinstance(left, dict) and isinstance(right, dict), "arm raw evidence is malformed")
    _need(set(left) == set(RAW_FILES) and set(right) == set(RAW_FILES),
          "each arm must contain exactly the seven pinned ordinary evidence files")
    for name in RAW_FILES:
        a, b = left[name], right[name]
        _need(isinstance(a, dict) and isinstance(b, dict), "raw evidence entry is malformed")
        _keys(a, ("bytes", "sha256"), f"{name} baseline evidence")
        _keys(b, ("bytes", "sha256"), f"{name} treatment evidence")
        _strict_int(a["bytes"], f"{name} bytes")
        _strict_int(b["bytes"], f"{name} bytes")
        _hash(a["sha256"], f"{name} baseline hash")
        _hash(b["sha256"], f"{name} treatment hash")
        _need(a["sha256"] == RAW_FILE_HASHES[name], f"{name} differs from the frozen source pin")
        _need(a == b, f"raw evidence differs between arms: {name}")


def validate_pair_staging(arms: dict) -> dict:
    """Check the real host staging receipts after both separately owned arms."""
    _need(isinstance(arms, dict) and set(arms) == set(ARMS), "both staged arms are required")
    for name in ARMS:
        arm = arms[name]
        _need(isinstance(arm, dict) and arm.get("status") == "passed"
              and arm.get("validatedFacts", {}).get("status") == "passed",
              f"{name} arm did not pass payload validation and owned cleanup")
        _need(isinstance(arm.get("stageFacts"), dict) and isinstance(arm.get("stageFacts", {}).get("common"), dict),
              f"{name} staged source pins are missing")
        _need(isinstance(arm.get("stageInventory"), dict)
              and isinstance(arm["stageInventory"].get("files"), list),
              f"{name} post-run stage inventory is missing")
    baseline = arms["baseline"]["stageFacts"]["common"]
    treatment = arms["treatment"]["stageFacts"]["common"]
    _need({k: v for k, v in baseline.items() if k not in ("bin/cub-scout", "_stageFiles")} ==
          {k: v for k, v in treatment.items() if k not in ("bin/cub-scout", "_stageFiles")},
          "shared evidence/runtime source pins changed between arms")
    baseline_plan = baseline.get("_stageFiles")
    treatment_plan = treatment.get("_stageFiles")
    _need(isinstance(baseline_plan, dict) and isinstance(treatment_plan, dict)
          and {k: v for k, v in baseline_plan.items() if k != "cub-scout"} ==
          {k: v for k, v in treatment_plan.items() if k != "cub-scout"},
          "actual staged file hashes differ outside the treatment binary")
    for name in ARMS:
        arm = arms[name]
        plan = arm["stageFacts"]["common"]["_stageFiles"]
        observed = {path: facts.get("sha256") for path, facts in plan.items() if isinstance(facts, dict)}
        inventory = arm["stageInventory"]
        _need(set(inventory["files"]) == set(observed)
              and inventory.get("sha256") == hashlib.sha256(
                  json.dumps(observed, sort_keys=True, separators=(",", ":")).encode()).hexdigest(),
              f"{name} post-run staged bytes changed from their pinned inventory")
    _need("bin/cub-scout" not in baseline and "bin/cub-scout" in treatment,
          "baseline/treatment Scout binary separation is not proven")
    baseline_files = set(arms["baseline"]["stageInventory"]["files"])
    treatment_files = set(arms["treatment"]["stageInventory"]["files"])
    _need("cub-scout" not in baseline_files and "cub-scout" in treatment_files,
          "actual staged files do not preserve the Scout binary treatment delta")
    return {"sharedSourcesEqual": True, "baselineScoutAbsent": True,
            "treatmentScoutPresent": True, "order": ["baseline", "treatment"]}


def strict_json(data: bytes | str):
    """Decode receipt JSON while rejecting duplicate keys and non-finite numbers."""
    def pairs(items):
        out = {}
        for key, value in items:
            if key in out:
                raise ContractError("duplicate JSON key")
            out[key] = value
        return out
    def constant(_):
        raise ContractError("non-finite JSON number")
    try:
        if isinstance(data, bytes):
            data = data.decode("utf-8", "strict")
        return json.loads(data, object_pairs_hook=pairs, parse_constant=constant)
    except ContractError:
        raise
    except (UnicodeError, ValueError, TypeError):
        raise ContractError("malformed JSON") from None


def validate_arm_payload(value: dict, expected_arm: str) -> dict:
    """Verify raw in-container evidence before the host retains a passing arm."""
    _need(expected_arm in ARMS and isinstance(value, dict) and value.get("arm") == expected_arm,
          "arm payload identity mismatch")
    _need(value.get("schema") == "combined-runtime-arm-payload.v1" and value.get("status") == "passed",
          "arm payload is incomplete or failed")
    _need(type(value.get("returnCode")) is int and value["returnCode"] == 0 and value.get("runStatus") == "completed",
          "CLI did not complete with exact zero status")
    stdout = _decode_payload_stream(value, "stdout")
    stderr = _decode_payload_stream(value, "stderr")
    _need(len(stdout) + len(stderr) <= MAX_RAW_OUTPUT, "combined CLI raw output exceeds the 8 MiB cap")
    _need(value.get("providerFailure") is None and value.get("providerAttemptsPassed") is True,
          "provider had a rejected, excess, or unexplained attempt")
    requests = value.get("providerRequests")
    _need(isinstance(requests, list) and 1 <= len(requests) <= MAX_REQUESTS,
          "provider request list missing or outside bounds")
    safe_requests = []
    for index, record in enumerate(requests, 1):
        _need(isinstance(record, dict), "provider request record is malformed")
        ordinal = _strict_int(record.get("ordinal"), "provider request ordinal", 1, MAX_REQUESTS)
        _need(ordinal <= MAX_REQUESTS
              and record.get("kind") == "accepted" and record.get("method") == "POST"
              and record.get("path") in ("/v1/messages", "/v1/messages?beta=true")
              and record.get("status") == 200 and record.get("authKind") == "x-api-key",
              "provider request was not an accepted exact fake-auth request")
        body = _strict_b64(record.get("bodyBase64"), "provider request body")
        _need(len(body) == record.get("bodyBytes") and len(body) <= MAX_REQUEST_BODY
              and _hash(record.get("bodySha256"), "provider request body hash") == hashlib.sha256(body).hexdigest(),
              "provider request bytes/hash exceed or differ from bound")
        _need(record.get("body") == body.decode("utf-8", "strict"), "provider request text differs from captured bytes")
        parsed = strict_json(body)
        _need(isinstance(parsed, dict) and type(parsed.get("stream")) is bool,
              "provider request stream field is not a strict boolean")
        response = _strict_b64(record.get("response_body_base64"), "provider response")
        _need(len(response) == record.get("responseBytes") and
              _hash(record.get("responseSha256"), "provider response hash") == hashlib.sha256(response).hexdigest(),
              "provider response bytes/hash differ")
        if index > 1:
            _need(ordinal > safe_requests[-1]["ordinal"],
                  "provider request ordinals are duplicate or out of order")
        safe_requests.append(record)
    provider_count = value.get("providerReceivedCount")
    _strict_int(provider_count, "provider received count", 1, MAX_REQUESTS)
    connections = value.get("providerConnections")
    _need(isinstance(connections, list) and len(connections) == provider_count and
          all(isinstance(row, dict) and row.get("kind") in ("accepted", "startup-declined") for row in connections)
          and sum(row.get("kind") == "accepted" for row in connections) == len(requests)
          and sum(row.get("kind") == "startup-declined" for row in connections) <= 1,
          "provider attempts include unexplained or rejected traffic")
    ordinals = [_strict_int(row.get("ordinal"), "provider connection ordinal", 1, MAX_REQUESTS)
                for row in connections]
    _need(ordinals == list(range(1, provider_count + 1)), "provider connection order is incomplete")
    for row in connections:
        if row.get("kind") == "startup-declined":
            _need(row.get("method") == "HEAD" and row.get("path") == "/api/hello" and row.get("status") == 404,
                  "provider startup request differs from the one allowed declined probe")
    parser = _probe_parser()
    provider_uses, provider_results = parser.scenario_exchange_facts(safe_requests)
    cli_uses = value.get("cliToolUses"); cli_results = value.get("cliToolResults")
    _need(isinstance(cli_uses, list) and isinstance(cli_results, list), "CLI tool events missing")
    expected_names = ["Read", "Bash", "Task", "Agent"] + ([next((u.get("name") for u in cli_uses if isinstance(u, dict)
        and isinstance(u.get("name"), str) and u["name"].endswith("__map")), "") ] if expected_arm == "treatment" else [])
    _need([item.get("name") for item in cli_uses if isinstance(item, dict)] == expected_names
          and [item.get("name") for item in provider_uses] == expected_names,
          "provider and CLI tool-use sequence differs from the protocol")
    _need(expected_arm != "treatment" or expected_names[-1] == "mcp__cub-scout__map",
          "treatment did not call the exact recorded MCP map tool")
    _need([{key: item.get(key) for key in ("id", "name", "input")} for item in cli_uses] ==
          [{key: item.get(key) for key in ("id", "name", "input")} for item in provider_uses],
          "provider response tool-use inputs differ from CLI events")
    use_ids = [item.get("id") for item in cli_uses]
    _need(all(isinstance(item, str) and item for item in use_ids) and len(use_ids) == len(set(use_ids)),
          "tool-use IDs are missing or duplicated")
    _need([item.get("id") for item in cli_results] == use_ids, "CLI results do not match tool-use order exactly")
    _need([item.get("id") for item in provider_results] == use_ids,
          "provider tool-result history does not match tool-use order exactly")
    for cli_result, provider_result in zip(cli_results, provider_results):
        raw_content = json.dumps(cli_result.get("content"), sort_keys=True).encode()
        _need(cli_result.get("is_error") == provider_result.get("is_error")
              and hashlib.sha256(raw_content).hexdigest() == provider_result.get("content_sha256"),
              "provider and CLI tool-result content/flags disagree")
    for name in ("Task", "Agent"):
        use = next((x for x in cli_uses if x.get("name") == name), None)
        result = next((x for x in cli_results if use and x.get("id") == use.get("id")), None)
        _need(use is not None and result is not None and result.get("is_error") is True
              and parser.explicit_denial(result.get("content"), name), f"{name} lacks exact refusal evidence")
    _need(value.get("inventoryPassed") is True and value.get("fileReadPassed") is True
          and value.get("kubectlReadPassed") is True and value.get("refusalPassed") is True,
          "ordinary tool, inventory, or explicit-refusal check failed")
    _need(type(value.get("apiRequests")) is int and value.get("apiRequests") == 1 and value.get("apiCaptured") is True
          and value.get("apiThreadJoined") is True, "PRE-01 API replay was incomplete or not cleaned up")
    if expected_arm == "treatment":
        _need(value.get("skillsAdvertised") is True and value.get("skillsPassed") is True
              and value.get("skillSignal", {}).get("advertisedCount") == SKILL_COUNT
              and value.get("mcpMapPassed") is True, "treatment skills/MCP evidence incomplete")
        _need(value.get("mcpMapResultValidated") is True, "existing recorded-scale result validator did not pass")
    else:
        _need(value.get("skillsAdvertised") is False and value.get("mcpMapPassed") is True,
              "baseline unexpectedly received plugin or MCP")
    helm = value.get("helm")
    _need(isinstance(helm, dict) and helm.get("argv") == ["/tools/helm", "version", "--short"]
          and type(helm.get("exitCode")) is int and helm.get("exitCode") == 0
          and helm.get("status") == "completed" and type(helm.get("stdoutBytes")) is int
          and 1 <= helm["stdoutBytes"] <= 65536 and _hash(helm.get("stdoutSha256"), "Helm stdout hash"),
          "pinned Helm binary probe was not successful and bounded")
    _need(value.get("helmAvailable") is True, "Helm availability was not observed")
    events = []
    for line in stdout.splitlines():
        event = strict_json(line)
        _need(isinstance(event, dict), "raw CLI event is not an object")
        events.append(event)
    try:
        raw_uses, raw_results = parser.cli_tool_events(events)
    except Exception as exc:
        raise ContractError("raw CLI tool-event stream is invalid") from exc
    _need(raw_uses == cli_uses and raw_results == cli_results,
          "reported CLI tool facts differ from raw CLI output")
    terminal_events = [event for event in events if event.get("type") == "result"]
    terminal = value.get("terminalText")
    _need(len(terminal_events) == 1 and terminal == "offline-probe-terminal"
          and terminal_events[0].get("subtype") == "success"
          and terminal_events[0].get("is_error") is not True
          and terminal_events[0].get("result") == terminal
          and not any(event.get("type") in ("task_started", "agent_started")
                      or event.get("subtype") in ("task_started", "agent_started") for event in events),
          "actual successful terminal result or no-started-delegation evidence missing")
    _need(type(value.get("elapsedSeconds")) in (int, float) and 0 <= value["elapsedSeconds"] <= MAX_EXECUTION_SECONDS,
          "arm execution exceeded its bound")
    cleanup = value.get("containerLocalCleanup")
    _need(isinstance(cleanup, dict) and cleanup.get("providerThreadJoined") is True
          and cleanup.get("apiThreadJoined") is True, "in-container listener cleanup is uncertain")
    return {"arm": expected_arm, "status": "passed", "providerRequests": len(requests),
            "toolUseIds": use_ids, "terminal": terminal, "billing": "unknown-unmeasured",
            "processInventory": "not-claimed"}


def _strict_b64(value, label):
    import base64
    _need(isinstance(value, str), f"{label} base64 missing")
    try: return base64.b64decode(value, validate=True)
    except (ValueError, TypeError): raise ContractError(f"{label} base64 malformed") from None


def _decode_payload_stream(value, stream):
    data = _strict_b64(value.get(stream + "Base64"), stream)
    _need(type(value.get(stream + "Bytes")) is int and value[stream + "Bytes"] == len(data)
          and _hash(value.get(stream + "Sha256"), stream + " hash") == hashlib.sha256(data).hexdigest(),
          f"{stream} bytes/hash differ")
    return data
