import base64
import importlib.util
import json
from pathlib import Path
import unittest

HERE = Path(__file__).parent
probe_spec = importlib.util.spec_from_file_location("probe", HERE / "probe.py")
probe = importlib.util.module_from_spec(probe_spec)
probe_spec.loader.exec_module(probe)
accounting_spec = importlib.util.spec_from_file_location("round_accounting_v1", HERE / "round_accounting_v1.py")
accounting = importlib.util.module_from_spec(accounting_spec)
accounting_spec.loader.exec_module(accounting)


FIXTURE = "/private/offline/read-fixture.txt"
CONTENT = [{"type": "text", "text": probe.READ_SENTINEL}]


def evidence(*, copied_result=False):
    use = {"type": "tool_use", "id": "toolu_offline_read_1", "name": "Read",
           "input": {"file_path": FIXTURE}}
    messages = [{"role": "user", "content": "fixed synthetic prompt"}]
    if copied_result:
        messages.extend([
            {"role": "assistant", "content": [use]},
            {"role": "user", "content": [{"type": "tool_result", "tool_use_id": use["id"],
                                               "content": CONTENT}]},
        ])
    body = {"model": probe.PINNED_MODEL, "tools": [{"name": "Read"}],
            "stream": True, "messages": messages}
    kind, response = probe.response_for(True, "turn-limit", 1, FIXTURE)
    body_bytes = json.dumps(body).encode()
    request = {"path": "/v1/messages", "auth_kind": "x-api-key",
               "body": body_bytes.decode(), "body_base64": base64.b64encode(body_bytes).decode(),
               "response_content_type": kind,
               "response_body_base64": base64.b64encode(response).decode()}
    cli_events = [
        {"type": "assistant", "message": {"content": [use]}},
        {"type": "user", "message": {"content": [{"type": "tool_result",
            "tool_use_id": use["id"], "content": CONTENT}]}},
        {"type": "result", "subtype": "error_max_turns", "num_turns": 2, "is_error": True},
    ]
    transport = [
        {"id": 1, "kind": "preflight", "httpStatus": 204},
        {"id": 2, "kind": "accepted", "httpStatus": 200},
    ]
    cleanup = {"server_shutdown_complete": True, "handlers_joined": True,
               "server_thread_joined": True}
    return {"requests": [request], "stdout": b"\n".join(json.dumps(e).encode() for e in cli_events),
            "return_code": 1, "request_count": 1, "connection_count": 2,
            "connection_events": transport, "cleanup": cleanup, "fixture_path": FIXTURE,
            "requested_limit": 1, "original_run_status": "validation_failed",
            "original_error": accounting.LEGACY_COUNTER_ERROR}


def validate(data):
    return accounting.validate_round_accounting_v1(**data)


class RoundAccountingV1Tests(unittest.TestCase):
    def test_accepts_exact_single_round_and_reports_scalars_separately(self):
        result = validate(evidence())
        self.assertEqual(result["schema"], "direct-cli-round-accounting.v1")
        self.assertEqual(result["analysis_outcome"], "accepted_single_round_stop")
        self.assertEqual(result["requested_limit"], 1)
        self.assertEqual(result["observed_model_requests"], 1)
        self.assertEqual(result["observed_http_requests"], 1)
        self.assertEqual(result["observed_tool_use_rounds"], 1)
        self.assertEqual(result["reported_terminal_num_turns"], 2)
        self.assertEqual(result["original_run_status"], "validation_failed")
        self.assertEqual(result["original_return_code"], 1)

    def test_opaque_reported_counter_does_not_override_observed_evidence(self):
        data = evidence()
        cli = json.loads(data["stdout"].splitlines()[-1])
        cli["num_turns"] = 17
        data["stdout"] = data["stdout"].splitlines()[0] + b"\n" + data["stdout"].splitlines()[1] + b"\n" + json.dumps(cli).encode()
        result = validate(data)
        self.assertEqual(result["reported_terminal_num_turns"], 17)
        data["request_count"] = 2
        with self.assertRaises(accounting.RoundAccountingError):
            validate(data)

    def test_first_request_cannot_contain_copied_tool_history(self):
        data = evidence(copied_result=True)
        with self.assertRaises(accounting.RoundAccountingError):
            validate(data)

    def test_accepts_three_connections_with_one_exact_startup_decline(self):
        data = evidence()
        data["request_count"] = 2  # HTTP: one declined startup HEAD plus the accepted model POST.
        data["connection_count"] = 3
        data["connection_events"] = [
            {"id": 1, "kind": "preflight", "httpStatus": 204},
            {"id": 2, "kind": "startup-declined", "httpStatus": 404,
             "method": "HEAD", "pathWithoutQuery": "/api/hello"},
            {"id": 3, "kind": "accepted", "httpStatus": 200},
        ]
        result = validate(data)
        self.assertEqual(result["observed_http_requests"], 2)
        self.assertEqual(result["observed_model_requests"], 1)
        self.assertEqual(result["connection_count"], 3)

    def test_missing_duplicate_extra_wrong_path_or_error_tool_evidence_fails(self):
        base = evidence()
        cases = []
        bad = evidence(); bad["stdout"] = b""; cases.append(bad)
        bad = evidence(); events = bad["stdout"].splitlines(); bad["stdout"] = b"\n".join(events + [events[0]]); cases.append(bad)
        bad = evidence(); cli = [json.loads(line) for line in bad["stdout"].splitlines()]; cli[0]["message"]["content"][0]["name"] = "Bash"; bad["stdout"] = b"\n".join(json.dumps(e).encode() for e in cli); cases.append(bad)
        bad = evidence(); cli = [json.loads(line) for line in bad["stdout"].splitlines()]; cli[1]["message"]["content"][0]["is_error"] = True; bad["stdout"] = b"\n".join(json.dumps(e).encode() for e in cli); cases.append(bad)
        bad = evidence(); bad["fixture_path"] = "/private/other.txt"; cases.append(bad)
        for data in cases:
            with self.subTest(data=data), self.assertRaises(accounting.RoundAccountingError):
                validate(data)

    def test_counter_cap_terminal_exit_transport_and_cleanup_are_separate_gates(self):
        mutations = [
            ("requested_limit", 2), ("return_code", 0), ("request_count", 2),
            ("connection_count", 3), ("original_run_status", "timeout"),
            ("original_error", "some other validation error"),
        ]
        for key, value in mutations:
            data = evidence(); data[key] = value
            with self.subTest(key=key), self.assertRaises(accounting.RoundAccountingError):
                validate(data)
        for key, value in (("num_turns", 0), ("is_error", False), ("subtype", "success")):
            data = evidence(); cli = [json.loads(line) for line in data["stdout"].splitlines()]
            cli[-1][key] = value; data["stdout"] = b"\n".join(json.dumps(e).encode() for e in cli)
            with self.subTest(key=key), self.assertRaises(accounting.RoundAccountingError):
                validate(data)
        data = evidence(); data["cleanup"]["handlers_joined"] = False
        with self.assertRaises(accounting.RoundAccountingError):
            validate(data)
        data = evidence(); data["connection_events"].append({"id": 3, "kind": "empty-eof"})
        with self.assertRaises(accounting.RoundAccountingError):
            validate(data)

    def test_malformed_shapes_and_noninteger_counters_fail_as_typed_errors(self):
        for key, value in (("cleanup", None), ("connection_events", None),
                           ("requests", None), ("stdout", None),
                           ("request_count", True), ("request_count", 1.0),
                           ("connection_count", True), ("connection_count", 2.0),
                           ("requested_limit", True), ("return_code", True)):
            data = evidence(); data[key] = value
            with self.subTest(key=key, value=value), self.assertRaises(accounting.RoundAccountingError):
                validate(data)
        for reported in (True, 2.0):
            data = evidence(); cli = [json.loads(line) for line in data["stdout"].splitlines()]
            cli[-1]["num_turns"] = reported
            data["stdout"] = b"\n".join(json.dumps(event).encode() for event in cli)
            with self.subTest(reported=reported), self.assertRaises(accounting.RoundAccountingError):
                validate(data)


if __name__ == "__main__":
    unittest.main()
