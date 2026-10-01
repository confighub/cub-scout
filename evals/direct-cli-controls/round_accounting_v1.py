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
    if requested_limit != 1 or type(requested_limit) is not int:
        _fail("this accounting contract is limited to requested limit 1")
    if original_run_status != "validation_failed" or not isinstance(original_error, str) or not original_error:
        _fail("source run must retain its original validation_failed status and error")
    if return_code != 1 or type(return_code) is not int:
        _fail("captured CLI return code is not the expected explicit exit 1")
    if type(request_count) is not int or request_count != 1 or len(requests) != 1:
        _fail("captured accepted model request count is not exactly one")
    if any(cleanup.get(key) is not True for key in
           ("server_shutdown_complete", "handlers_joined", "server_thread_joined")):
        _fail("fixture server or handler cleanup is incomplete")
    if connection_count != 2 or type(connection_count) is not int or len(connection_events) != 2:
        _fail("connection count is not exactly preflight plus one model request")
    try:
        lowlevel.validate_transport(connection_count, connection_events, accepted_requests=1)
    except (lowlevel.ProbeError, ValueError, TypeError, AttributeError) as exc:
        _fail(f"transport evidence is invalid: {exc}")
    if {event.get("kind") for event in connection_events} != {"preflight", "accepted"}:
        _fail("transport includes an extra or missing connection")

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
        mock_uses, provider_results = lowlevel.scenario_exchange_facts(requests)
        response_uses = lowlevel._response_tool_uses(record)
        events = [lowlevel.strict_json(line) for line in stdout.splitlines() if line.strip()]
        cli_uses, cli_results = lowlevel.cli_tool_events(events)
    except (lowlevel.ProbeError, ValueError, TypeError, KeyError) as exc:
        _fail(f"retained request/response/CLI evidence cannot be parsed: {exc}")
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
    if len(provider_results) > 1:
        _fail("provider request contains extra tool results")
    if provider_results:
        result = provider_results[0]
        if (result.get("id") != use["id"] or result.get("is_error")
                or result.get("content") != cli_result.get("content")
                or FIXTURE_MARKER not in json.dumps(result.get("content"), sort_keys=True)):
            _fail("outbound provider tool_result does not match the successful CLI result")

    return {
        "schema": SCHEMA,
        "analysis_outcome": "accepted_single_round_stop",
        "original_run_status": original_run_status,
        "original_return_code": return_code,
        "original_error": original_error,
        "requested_limit": requested_limit,
        "observed_model_requests": request_count,
        "observed_tool_use_rounds": 1,
        "reported_terminal_num_turns": reported,
        "terminal_subtype": terminal["subtype"],
        "terminal_is_error": terminal["is_error"],
        "read_tool_use_id": use["id"],
        "fixture_result_sha256": hashlib.sha256(
            json.dumps(cli_result.get("content"), sort_keys=True).encode()).hexdigest(),
        "provider_result_copied": bool(provider_results),
        "connection_count": connection_count,
        "cleanup_verified": True,
    }
