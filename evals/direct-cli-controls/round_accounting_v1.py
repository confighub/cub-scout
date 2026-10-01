"""Independent, narrow validator for one retained synthetic Read round.

This deliberately does not reuse the legacy aggregate ``num_turns`` decision.
It reconciles captured request/response/tool/transport evidence and preserves
the source run's failed status in its analysis result.
"""

from __future__ import annotations

import base64
import hashlib
import json
from typing import Any

import probe as lowlevel


SCHEMA = "direct-cli-round-accounting.v1"
MODEL = lowlevel.PINNED_MODEL
FIXTURE_MARKER = lowlevel.READ_SENTINEL
LEGACY_COUNTER_ERROR = "turn-limit terminal num_turns is missing or inconsistent with its cap"


class RoundAccountingError(ValueError):
    pass


def _fail(message: str):
    raise RoundAccountingError(message)


def validate_round_accounting_v1(*, requests: list[dict[str, Any]], stdout: bytes,
                                 return_code: int, request_count: int,
                                 connection_count: int, connection_events: list[dict[str, Any]],
                                 cleanup: dict[str, Any], fixture_path: str,
                                 requested_limit: int, original_run_status: str,
                                 original_error: str) -> dict[str, Any]:
    """Validate only the one-Read synthetic stop and retain source failure facts.

    ``num_turns`` is recorded as an opaque terminal-reported field. It is not
    compared to the requested cap and never interpreted as request/billing data.
    """
    if type(requested_limit) is not int or requested_limit != 1:
        _fail("this accounting contract is limited to requested limit 1")
    if (original_run_status != "validation_failed" or original_error != LEGACY_COUNTER_ERROR):
        _fail("source run does not carry the exact legacy num_turns validation failure")
    if not isinstance(requests, list) or not requests or any(not isinstance(item, dict) for item in requests):
        _fail("retained provider requests are malformed")
    if not isinstance(stdout, bytes):
        _fail("captured CLI output must be bytes")
    if return_code != 1 or type(return_code) is not int:
        _fail("captured CLI return code is not the expected explicit exit 1")
    if type(request_count) is not int or request_count not in (1, 2) or len(requests) != 1:
        _fail("HTTP request count or captured model request count is outside the one-round bound")
    if not isinstance(cleanup, dict):
        _fail("cleanup evidence is malformed")
    if any(cleanup.get(key) is not True for key in
           ("server_shutdown_complete", "handlers_joined", "server_thread_joined")):
        _fail("fixture server or handler cleanup is incomplete")
    if (type(connection_count) is not int or connection_count != request_count + 1
            or not isinstance(connection_events, list) or len(connection_events) != connection_count
            or any(not isinstance(event, dict) for event in connection_events)):
        _fail("TCP connection count does not equal HTTP request count plus preflight")
    try:
        lowlevel.validate_transport(connection_count, connection_events, accepted_requests=len(requests))
    except (lowlevel.ProbeError, ValueError, TypeError, AttributeError) as exc:
        _fail(f"transport evidence is invalid: {exc}")
    startup_count = sum(event.get("kind") == "startup-declined" for event in connection_events)
    if startup_count not in (0, 1) or {event.get("kind") for event in connection_events} != (
            {"preflight", "accepted"} if startup_count == 0 else
            {"preflight", "startup-declined", "accepted"}):
        _fail("transport includes an extra or missing connection")
    if request_count != len(requests) + startup_count:
        _fail("HTTP request count differs from accepted model and declined startup requests")
    if not isinstance(fixture_path, str) or not fixture_path:
        _fail("private fixture path is malformed")

    record = requests[0]
    try:
        body_bytes = record["body"].encode("utf-8")
        if base64.b64decode(record["body_base64"], validate=True) != body_bytes:
            _fail("retained provider request body representations conflict")
        if record.get("path") not in ("/v1/messages", "/v1/messages?beta=true"):
            _fail("retained accepted request has an unexpected endpoint")
        if record.get("auth_kind") not in ("x-api-key", "bearer"):
            _fail("retained accepted request lacks safe synthetic-auth classification")
        body = lowlevel._request_body(record)
        messages = body.get("messages")
        if not isinstance(messages, list) or not messages:
            _fail("first model request has malformed message history")
        for message in messages:
            if not isinstance(message, dict):
                _fail("first model request has malformed message history")
            blocks = message.get("content")
            if isinstance(blocks, list):
                if any(not isinstance(block, dict) or not isinstance(block.get("type"), str)
                       for block in blocks):
                    _fail("first model request has malformed content blocks")
                if any(block.get("type") in ("tool_use", "tool_result") for block in blocks):
                    _fail("first and sole provider request cannot contain future tool history")
            elif not isinstance(blocks, str):
                _fail("first model request has malformed message content")
        mock_uses, provider_results = lowlevel.scenario_exchange_facts(requests)
        response_uses = lowlevel._response_tool_uses(record)
        events = [lowlevel.strict_json(line) for line in stdout.splitlines() if line.strip()]
        cli_uses, cli_results = lowlevel.cli_tool_events(events)
    except (lowlevel.ProbeError, ValueError, TypeError, KeyError, AttributeError) as exc:
        _fail(f"retained request/response/CLI evidence cannot be parsed: {exc}")
    # This contract covers only the observed single-round event sequence. Do
    # not silently ignore unknown events, post-terminal work, or malformed
    # result flags through the more permissive shared event extractor.
    sequence = list(events)
    if sequence and isinstance(sequence[0], dict) and sequence[0].get("type") == "system" and sequence[0].get("subtype") == "init":
        sequence = sequence[1:]
    if (len(sequence) != 3 or any(not isinstance(event, dict) for event in sequence)
            or [event.get("type") for event in sequence] != ["assistant", "user", "result"]):
        _fail("CLI evidence is not an ordered single tool-use/result/terminal sequence")
    for event, expected_type in zip(sequence[:2], ("tool_use", "tool_result")):
        message = event.get("message")
        blocks = message.get("content") if isinstance(message, dict) else None
        if (not isinstance(blocks, list) or len(blocks) != 1
                or not isinstance(blocks[0], dict) or blocks[0].get("type") != expected_type):
            _fail("CLI evidence contains extra or malformed tool blocks")
    result_block = sequence[1]["message"]["content"][0]
    if "is_error" in result_block and result_block["is_error"] is not False:
        _fail("Read result error flag is not an explicit success")
    if body.get("model") != MODEL:
        _fail("captured request model label differs from the pinned mock label")
    tools = body.get("tools")
    if (not isinstance(tools, list) or any(not isinstance(tool, dict) for tool in tools)
            or [tool.get("name") for tool in tools] != ["Read"] or body.get("stream") is not True):
        _fail("captured request does not advertise exactly Read")
    if len(response_uses) != 1 or len(mock_uses) != 1:
        _fail("mock response does not contain exactly one tool-use round")
    use = response_uses[0]
    if (use.get("type") != "tool_use" or use.get("name") != "Read"
            or not isinstance(use.get("id"), str)
            or use.get("input") != {"file_path": fixture_path}):
        _fail("mock response Read ID/input differs from the exact fixture path")

    terminals = [event for event in events if isinstance(event, dict) and event.get("type") == "result"]
    if len(terminals) != 1:
        _fail("CLI output lacks exactly one terminal result")
    terminal = terminals[0]
    if terminal.get("subtype") != "error_max_turns" or terminal.get("is_error") is not True:
        _fail("CLI terminal is not an explicit error_max_turns result")
    reported = terminal.get("num_turns")
    if type(reported) is not int or reported < 1:
        _fail("terminal-reported num_turns is not an exact positive integer")

    if len(cli_uses) != 1 or cli_uses[0] != {k: use.get(k) for k in ("id", "name", "input")}:
        _fail("CLI output does not contain the exact single mock Read tool-use")
    if len(cli_results) != 1 or cli_results[0].get("id") != use["id"] or cli_results[0].get("is_error"):
        _fail("CLI output does not contain exactly one successful result for the Read ID")
    cli_result = cli_results[0]
    if FIXTURE_MARKER not in json.dumps(cli_result.get("content"), sort_keys=True):
        _fail("CLI Read result lacks the exact private fixture marker")

    if any(use_item.get("id") != use["id"] or use_item != use for use_item in mock_uses):
        _fail("response tool-use facts are inconsistent")
    if provider_results:
        _fail("first and sole provider request unexpectedly contains tool-result history")

    return {
        "schema": SCHEMA,
        "analysis_outcome": "accepted_single_round_stop",
        "original_run_status": original_run_status,
        "original_return_code": return_code,
        "original_error": original_error,
        "requested_limit": requested_limit,
        "observed_http_requests": request_count,
        "observed_model_requests": len(requests),
        "observed_tool_use_rounds": 1,
        "reported_terminal_num_turns": reported,
        "terminal_subtype": terminal["subtype"],
        "terminal_is_error": terminal["is_error"],
        "read_tool_use_id": use["id"],
        "fixture_result_sha256": hashlib.sha256(
            json.dumps(cli_result.get("content"), sort_keys=True).encode()).hexdigest(),
        "connection_count": connection_count,
        "cleanup_verified": True,
    }
