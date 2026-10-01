#!/usr/bin/env python3
"""In-container, fake-provider-only arm payload. Never runs outside the owned image."""
from __future__ import annotations

import base64, hashlib, http.server, importlib.util, json, os, re, signal, sys, threading, time
from pathlib import Path

TOOLS = Path("/tools")
FAKE_KEY = "sk-ant-api03-cub-scout-offline-fake-key"
MAX_BODY = 1024 * 1024
MAX_OUTPUT = 8 * 1024 * 1024
MAX_REQUESTS = 8
ARM_SECONDS = 120
READ_PATH = "/apis/monitoring.coreos.com/v1/namespaces/monitoring/servicemonitors/kube-prometheus-stack-kube-state-metrics"
FINAL = "offline-probe-terminal"


def sha(data): return hashlib.sha256(data).hexdigest()


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None: raise RuntimeError("pinned source cannot be loaded")
    mod = importlib.util.module_from_spec(spec); spec.loader.exec_module(mod); return mod


def json_bytes(value): return json.dumps(value, separators=(",", ":"), ensure_ascii=False).encode()


def exact_bash_body(actual, expected):
    # Claude's Bash result omits the final line terminator; no other content
    # normalization or JSON reserialization is permitted.
    return isinstance(actual, str) and actual in (expected, expected.removesuffix("\n"))


def tool_plan(arm, available):
    plan = [
        ("Read", {"file_path": "/tools/evidence/events.yaml"}),
        ("Bash", {"command": f"/tools/kubectl --kubeconfig /tmp/kubeconfig --context recorded get --raw {READ_PATH}",
                  "description": "One fixed read against the PRE-01 recorded API"}),
        ("Task", {"description": "Harmless bounded refusal control",
                  "prompt": "Return only offline-probe-terminal. Do not use tools or execute commands.",
                  "subagent_type": "general-purpose"}),
        ("Agent", {"description": "Harmless bounded refusal control",
                   "prompt": "Return only offline-probe-terminal. Do not use tools or execute commands.",
                   "subagent_type": "general-purpose"}),
    ]
    if arm == "treatment":
        tools = available if isinstance(available, list) else []
        candidates = [t.get("name") for t in tools if isinstance(t, dict) and isinstance(t.get("name"), str)
                      and t["name"].endswith("__map")]
        explains = [t.get("name") for t in tools if isinstance(t, dict) and isinstance(t.get("name"), str)
                    and t["name"].endswith("__explain")]
        # The exact exposed identifier is selected from this request only; no
        # unknown tool can be supplied or silently substituted.
        if len(candidates) != 1 or len(explains) != 1:
            raise RuntimeError("recorded MCP map/explain tool inventory is missing or ambiguous")
        plan.append((candidates[0], {"api_version": "apps/v1", "kind": "Deployment", "namespace_prefix": "team-"}))
    return plan


def provider_response(stream, arm, response_index, tools):
    plan = tool_plan(arm, tools)
    if response_index <= len(plan):
        name, inputs = plan[response_index - 1]
        call = {"type": "tool_use", "id": f"toolu_combined_{arm}_{response_index}", "name": name, "input": inputs}
        stop = "tool_use"
        blocks = [call]
    else:
        stop = "end_turn"
        blocks = [{"type": "text", "text": FINAL}]
    message = {"id": f"msg_combined_{arm}_{response_index}", "type": "message", "role": "assistant",
               "model": "claude-haiku-4-5-20251001", "content": blocks, "stop_reason": stop,
               "stop_sequence": None, "usage": {"input_tokens": 1, "output_tokens": 1}}
    if not stream:
        return "application/json", json_bytes(message)
    events = [{"type": "message_start", "message": {**message, "content": [], "stop_reason": None}}]
    if stop == "tool_use":
        events += [
            {"type": "content_block_start", "index": 0, "content_block": {"type": "tool_use", "id": call["id"], "name": call["name"], "input": {}}},
            {"type": "content_block_delta", "index": 0, "delta": {"type": "input_json_delta", "partial_json": json.dumps(call["input"], separators=(",", ":"))}},
            {"type": "content_block_stop", "index": 0},
        ]
    else:
        events += [{"type": "content_block_start", "index": 0, "content_block": {"type": "text", "text": ""}},
                   {"type": "content_block_delta", "index": 0, "delta": {"type": "text_delta", "text": FINAL}},
                   {"type": "content_block_stop", "index": 0}]
    events += [{"type": "message_delta", "delta": {"stop_reason": stop, "stop_sequence": None},
                "usage": {"output_tokens": 1}}, {"type": "message_stop"}]
    return "text/event-stream", b"".join(b"event: " + e["type"].encode() + b"\ndata: " + json_bytes(e) + b"\n\n" for e in events)


class Provider(http.server.ThreadingHTTPServer):
    daemon_threads = False
    allow_reuse_address = False

    def __init__(self, arm):
        self.arm = arm; self.lock = threading.Lock(); self.received = 0
        self.records = []; self.connections = []; self.failure = None
        super().__init__(("127.0.0.1", 0), self.handler())
        self.deadline = time.monotonic() + ARM_SECONDS

    def handler(self):
        outer = self
        class H(http.server.BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"
            def setup(self):
                super().setup(); self.request.settimeout(2.0)
                self._request_counted = False; self._timed_out = False
                self._deadline_timer = threading.Timer(max(0.01, min(2.0, outer.deadline - time.monotonic())), self._deadline_expired)
                self._deadline_timer.daemon = True; self._deadline_timer.start()
            def reserve_request(self):
                with outer.lock:
                    if self._request_counted: return self._ordinal
                    outer.received += 1; self._ordinal = outer.received; self._request_counted = True
                    return self._ordinal
            def _deadline_expired(self):
                self._timed_out = True
                if not self._request_counted:
                    ordinal = self.reserve_request()
                    with outer.lock: outer.connections.append({"ordinal": ordinal, "kind": "incomplete", "method": None,
                        "path": "<incomplete>", "status": None})
                try: self.request.shutdown(2)
                except OSError: pass
                try: self.request.close()
                except OSError: pass
            def finish(self):
                self._deadline_timer.cancel()
                try: super().finish()
                except OSError: pass
            def log_message(self, *_): pass
            def reply(self, status, kind, body):
                self.close_connection = True
                self.send_response(status); self.send_header("Content-Type", kind)
                self.send_header("Content-Length", str(len(body))); self.send_header("Connection", "close")
                self.end_headers(); self.wfile.write(body)
            def do_HEAD(self):
                ordinal = self.reserve_request()
                if self._timed_out: return
                if ordinal > MAX_REQUESTS: self.reply(429, "application/json", b"{}")
                elif self.path == "/api/hello": self.reply(404, "application/json", b"")
                else: self.reply(404, "application/json", b"{}")
                status = 429 if ordinal > MAX_REQUESTS else 404
                with outer.lock: outer.connections.append({"ordinal": ordinal, "kind": "startup-declined" if self.path == "/api/hello" and status == 404 else "rejected", "method": "HEAD", "path": self.path, "status": status})
            def do_POST(self):
                ordinal = self.reserve_request()
                if self._timed_out: return
                status, response_kind, response = 400, "application/json", b'{"error":"invalid synthetic request"}'
                safe = {"ordinal": ordinal, "kind": "rejected", "method": self.command,
                        "path": self.path, "status": None, "authKind": None}
                try:
                    if ordinal > MAX_REQUESTS: status = 429
                    elif self.command != "POST": status = 405
                    elif self.path not in ("/v1/messages", "/v1/messages?beta=true"): status = 404
                    elif (self.headers.get_all("x-api-key", []) != [FAKE_KEY]
                          or self.headers.get_all("authorization", [])
                          or self.headers.get_all("content-length", []) != [self.headers.get("content-length", "")]
                          or self.headers.get_all("transfer-encoding", [])): status = 401
                    else:
                        raw_length = self.headers.get("content-length", "")
                        if not re.fullmatch(r"[0-9]+", raw_length) or int(raw_length) > MAX_BODY: status = 413
                        else:
                            body = bytearray(); read_deadline = time.monotonic() + 2.0
                            while len(body) < int(raw_length):
                                if time.monotonic() >= read_deadline: raise TimeoutError("body read deadline")
                                chunk = self.rfile.read1(min(65536, int(raw_length) - len(body)))
                                if not chunk: raise ValueError("incomplete request body")
                                body.extend(chunk)
                            body = bytes(body)
                            if self._timed_out or time.monotonic() > read_deadline: status = 408
                            else:
                                parsed = json.loads(body.decode("utf-8", "strict"), object_pairs_hook=unique_pairs)
                                if not isinstance(parsed, dict) or type(parsed.get("stream")) is not bool: status = 400
                                elif time.monotonic() >= outer.deadline: status = 408
                                else:
                                    with outer.lock:
                                        response_index = len(outer.records) + 1
                                        response_kind, response = provider_response(parsed["stream"], outer.arm,
                                            response_index, parsed.get("tools"))
                                        status = 200; safe.update({"status": status, "kind": "accepted", "authKind": "x-api-key",
                                            "body": body.decode("utf-8"), "bodyBase64": base64.b64encode(body).decode(), "bodyBytes": len(body),
                                            "bodySha256": sha(body), "response": base64.b64encode(response).decode(),
                                            "response_body_base64": base64.b64encode(response).decode(),
                                            "response_content_type": response_kind,
                                            "responseBytes": len(response), "responseSha256": sha(response),
                                            "responseContentType": response_kind,
                                            "inventoryNames": [x.get("name") for x in parsed.get("tools", []) if isinstance(x, dict)]})
                                        outer.records.append(safe.copy())
                    if status != 200: response = json_bytes({"error": "synthetic request rejected", "status": status})
                except (ValueError, UnicodeError, RuntimeError, OSError, TimeoutError):
                    status, response_kind, response = 400, "application/json", b'{"error":"malformed synthetic request"}'
                if self._timed_out:
                    return
                safe["status"] = status
                if self.path not in ("/v1/messages", "/v1/messages?beta=true"):
                    safe["path"] = "<unexpected>"
                with outer.lock: outer.connections.append(safe)
                try: self.reply(status, response_kind, response)
                except OSError: pass
            def __getattr__(self, attr):
                if attr.startswith("do_"): return self.do_POST
                raise AttributeError(attr)
        return H


def unique_pairs(pairs):
    value = {}
    for key, item in pairs:
        if key in value: raise ValueError("duplicate JSON key")
        value[key] = item
    return value


def system_text(value):
    if isinstance(value, str): return value
    if isinstance(value, list):
        return "\n".join(item.get("text", "") for item in value if isinstance(item, dict) and item.get("type") == "text")
    return ""


def main():
    if len(sys.argv) != 2 or sys.argv[1] not in ("baseline", "treatment"):
        raise SystemExit("expected baseline or treatment arm")
    arm = sys.argv[1]; began = time.monotonic(); deadline = began + ARM_SECONDS
    direct = load("direct_probe", TOOLS / "evals/direct-cli-controls/probe.py")
    replay = load("recorded_api_replay", TOOLS / "evals/recorded-api/replay.py")
    scale = load("recorded_scale_preflight", TOOLS / "evals/recorded-scale/preflight.py")
    provider = Provider(arm); provider_thread = threading.Thread(target=provider.serve_forever, kwargs={"poll_interval": 0.05})
    routes, api_provenance = replay.load_routes("present")
    api = replay.ReplayServer(routes, host="127.0.0.1", port=0, max_requests=4, read_timeout=2.0)
    api_thread = threading.Thread(target=api.serve_bounded, kwargs={"duration": ARM_SECONDS})
    cli_rc = None; stdout = stderr = b""; run_status = "not-started"; process_error = None
    helm_rc = None; helm_stdout = helm_stderr = b""; helm_status = "not-started"; helm_ok = False
    cleanup = {"providerThreadJoined": False, "apiThreadJoined": False}
    try:
        provider_thread.start(); api_thread.start()
        cfg = {"apiVersion": "v1", "kind": "Config", "clusters": [{"name": "recorded", "cluster": {"server": f"http://127.0.0.1:{api.server_address[1]}"}}],
               "contexts": [{"name": "recorded", "context": {"cluster": "recorded", "user": "anonymous"}}],
               "current-context": "recorded", "users": [{"name": "anonymous", "user": {}}]}
        Path("/tmp/kubeconfig").write_bytes(json_bytes(cfg)); os.chmod("/tmp/kubeconfig", 0o600)
        Path("/tmp/empty-kubeconfig").write_bytes(json_bytes({"apiVersion": "v1", "kind": "Config",
            "clusters": [], "contexts": [], "users": [], "current-context": ""}))
        os.chmod("/tmp/empty-kubeconfig", 0o600)
        config_dir = Path("/tmp/claude-config"); config_dir.mkdir(mode=0o700, exist_ok=True)
        os.chmod(config_dir, 0o700)
        private_home = Path("/tmp/private-home"); private_home.mkdir(mode=0o700, exist_ok=True)
        os.chmod(private_home, 0o700)
        prompt = ("This is a synthetic offline wiring diagnostic. Execute once: read /tools/evidence/events.yaml; "
                  "run the exact kubectl command requested by the tool schedule; attempt the harmless Task and Agent "
                  "controls once; then in treatment call recorded map once. Return the fixed terminal marker only. "
                  "Do not invent findings or use any other path or command. " + FINAL)
        env = {"PATH": "/tools:/usr/bin:/bin", "HOME": "/tmp/private-home", "TMPDIR": "/tmp",
               "LANG": "C.UTF-8", "CLAUDE_CONFIG_DIR": str(config_dir), "ANTHROPIC_API_KEY": FAKE_KEY,
               "ANTHROPIC_BASE_URL": f"http://127.0.0.1:{provider.server_address[1]}",
               "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1", "CLAUDE_CODE_DISABLE_FAST_MODE": "1",
               "CI": "1", "NO_COLOR": "1", "HTTP_PROXY": "", "HTTPS_PROXY": "", "ALL_PROXY": "",
               "http_proxy": "", "https_proxy": "", "all_proxy": ""}
        try:
            helm_rc, helm_stdout, helm_stderr, helm_status = direct.run_bounded(
                ["/tools/helm", "version", "--short"], env, Path("/tmp"), deadline, 64 * 1024)
            helm_ok = type(helm_rc) is int and helm_rc == 0 and helm_status == "completed" and bool(helm_stdout.strip())
        except direct.ProcessFailure as exc:
            helm_stdout, helm_stderr, helm_status = exc.stdout, exc.stderr, exc.status
            process_error = "helm_" + type(exc).__name__
        mcp = Path("/tmp/mcp.json")
        mcp_config = {"mcpServers": {"cub-scout": {"command": "/tools/plugin/bin/recorded-cub-scout", "args": []}}}
        mcp.write_bytes(json_bytes(mcp_config if arm == "treatment" else {"mcpServers": {}})); os.chmod(mcp, 0o600)
        tool_grant = "Read,Bash,Skill" if arm == "treatment" else "Read,Bash"
        allowed = tool_grant + (",mcp__cub-scout__map,mcp__cub-scout__explain" if arm == "treatment" else "")
        argv = ["/tools/claude", "--print", "--verbose", "--output-format", "stream-json", "--model",
                "claude-haiku-4-5-20251001", "--no-session-persistence", "--tools", tool_grant,
                "--allowedTools", allowed,
                "--disallowedTools", "Task,Agent", "--strict-mcp-config", "--mcp-config", str(mcp),
                "--setting-sources", "", "--max-turns", "8"]
        if arm == "treatment": argv += ["--plugin-dir", "/tools/plugin"]
        argv += [prompt]
        try:
            cli_rc, stdout, stderr, run_status = direct.run_bounded(argv, env, Path("/tmp"), deadline, MAX_OUTPUT)
        except direct.ProcessFailure as exc:
            stdout, stderr, run_status = exc.stdout, exc.stderr, exc.status
            process_error = type(exc).__name__
        time.sleep(0.2)
    finally:
        provider.shutdown(); provider.server_close(); provider_thread.join(timeout=2)
        api.stop_event.set()
        api_thread.join(timeout=2)
        cleanup["providerThreadJoined"] = not provider_thread.is_alive()
        cleanup["apiThreadJoined"] = not api_thread.is_alive()
        cleanup["providerRequests"] = provider.received
        cleanup["providerConnections"] = provider.connections
        cleanup["apiRequests"] = api.requests_seen
        cleanup["apiConnections"] = api.connections_seen

    cli_events = []
    for line in stdout.splitlines():
        try:
            item = direct.strict_json(line)
            if isinstance(item, dict): cli_events.append(item)
        except (ValueError, TypeError):
            pass
    try:
        cli_uses, cli_results = direct.cli_tool_events(cli_events)
        provider_uses, provider_results = direct.scenario_exchange_facts(provider.records)
    except BaseException as exc:
        cli_uses, cli_results, provider_uses, provider_results = [], [], [], []
        provider.failure = type(exc).__name__
    first_body = direct.strict_json(provider.records[0]["body"]) if provider.records else {}
    plan_error = None
    try:
        expected = tool_plan(arm, first_body.get("tools", []))
    except BaseException as exc:
        expected = []
        plan_error = type(exc).__name__
    expected_names = [name for name, _ in expected]
    expected_final_requests = len(expected) + 1
    startup = [row for row in provider.connections if row.get("kind") == "startup-declined"]
    provider_attempts_ok = (provider.received <= MAX_REQUESTS and len(provider.records) == expected_final_requests
        and all(row.get("kind") in ("accepted", "startup-declined") for row in provider.connections)
        and len(startup) <= 1 and len(provider.connections) == provider.received
        and sum(row.get("kind") == "accepted" for row in provider.connections) == len(provider.records))
    matched_results = {item.get("id"): item for item in provider_results}
    cli_result_map = {item.get("id"): item for item in cli_results}
    result_equivalence = (set(matched_results) == set(cli_result_map) and all(
        matched_results[ident].get("is_error") == cli_result_map[ident].get("is_error")
        and matched_results[ident].get("content_sha256") == sha(json.dumps(cli_result_map[ident].get("content"), sort_keys=True).encode())
        for ident in matched_results))
    tool_ok = ([x.get("name") for x in cli_uses] == expected_names == [x.get("name") for x in provider_uses]
               and [x.get("input") for x in cli_uses] == [x.get("input") for x in provider_uses]
               and {x.get("id") for x in cli_uses} == {x.get("id") for x in provider_uses}
               and {x.get("id") for x in cli_uses} == {x.get("id") for x in cli_results}
               and result_equivalence and provider_attempts_ok)
    refusal_ok = True
    for name in ("Task", "Agent"):
        use = next((item for item in cli_uses if item.get("name") == name), None)
        result = next((item for item in cli_results if use and item.get("id") == use.get("id")), None)
        refusal_ok = refusal_ok and bool(use and result and result.get("is_error") is True and direct.explicit_denial(result.get("content"), name))
    map_ok = True
    map_results = []
    if arm == "treatment":
        map_use = next((item for item in cli_uses if str(item.get("name", "")).endswith("__map")), None)
        map_result = next((item for item in cli_results if map_use and item.get("id") == map_use.get("id")), None)
        map_ok = bool(map_use and map_result and map_result.get("is_error") is not True)
        if map_ok:
            content = map_result.get("content")
            text = content if isinstance(content, str) else "\n".join(x.get("text", "") for x in content if isinstance(x, dict))
            try:
                scale.validate_report(json.loads(text)); map_results = [{"sha256": sha(text.encode()), "bytes": len(text.encode())}]
            except (ValueError, TypeError): map_ok = False
    observed_tools = provider.records[0].get("inventoryNames", []) if provider.records else []
    builtins = [name for name in observed_tools if name in ("Read", "Bash")]
    mcp_inventory = [name for name in observed_tools if isinstance(name, str) and name.startswith("mcp__")]
    inventory_ok = (set(builtins) == {"Read", "Bash"} and len(builtins) == 2
        and not any(name in ("Task", "Agent") for name in observed_tools)
        and ((arm == "baseline" and not mcp_inventory) or
             (arm == "treatment" and len(mcp_inventory) == 2
              and sum(name.endswith("__map") for name in mcp_inventory) == 1
                          and sum(name.endswith("__explain") for name in mcp_inventory) == 1)))
    exact_inventory = {"Read", "Bash"} | ({"Skill", "mcp__cub-scout__map", "mcp__cub-scout__explain"} if arm == "treatment" else set())
    inventory_ok = inventory_ok and set(observed_tools) == exact_inventory and len(observed_tools) == len(exact_inventory)
    expected_skill_names = sorted(p.parent.name for p in (TOOLS / "plugin/skills").rglob("SKILL.md")) if arm == "treatment" else []
    advertised_text = system_text(first_body.get("system"))
    advertised_skills = [name for name in expected_skill_names if re.search(r"(?<![A-Za-z0-9_-])" + re.escape(name) + r"(?![A-Za-z0-9_-])", advertised_text)]
    skill_signal = {"expectedCount": len(expected_skill_names), "advertisedCount": len(advertised_skills),
                    "advertisedNames": advertised_skills, "sourceDirectoryCount": len(expected_skill_names)}
    skills_ok = (len(expected_skill_names) == 35 and len(advertised_skills) == 35) if arm == "treatment" else not (TOOLS / "plugin/skills").exists()
    def result_text(result):
        content = result.get("content")
        if isinstance(content, str): return content
        if isinstance(content, list): return "\n".join(x.get("text", "") for x in content if isinstance(x, dict))
        return ""
    read_use = next((x for x in cli_uses if x.get("name") == "Read"), None)
    read_result = next((x for x in cli_results if read_use and x.get("id") == read_use.get("id")), None)
    read_ok = bool(read_use and read_use.get("input") == {"file_path": "/tools/evidence/events.yaml"}
        and read_result and "eventTime: null" in result_text(read_result)
        and "fieldPath: spec.containers{cron}" in result_text(read_result))
    bash_use = next((x for x in cli_uses if x.get("name") == "Bash"), None)
    bash_result = next((x for x in cli_results if bash_use and x.get("id") == bash_use.get("id")), None)
    expected_api_route = next((r for r in routes if r.get("path") == READ_PATH), None)
    api_body_text = expected_api_route["body"].decode("utf-8", "replace") if expected_api_route else ""
    bash_ok = bool(bash_use and isinstance(bash_use.get("input"), dict)
        and bash_use["input"].get("command") == f"/tools/kubectl --kubeconfig /tmp/kubeconfig --context recorded get --raw {READ_PATH}"
        and bash_result and bash_result.get("is_error") is not True and exact_bash_body(result_text(bash_result), api_body_text))
    api_ok = (api.requests_seen == 1 and len(api.records) == 1 and api.records[0].get("captured") is True
              and api.records[0].get("path") == READ_PATH and not api.coverage_failure)
    terminal_events = []
    invalid_child_events = False
    for event in cli_events:
        if event.get("type") == "result": terminal_events.append(event)
        if event.get("subtype") in ("task_started", "agent_started") or event.get("type") in ("task_started", "agent_started"):
            invalid_child_events = True
    terminal = (len(terminal_events) == 1 and terminal_events[0].get("subtype") == "success"
                and terminal_events[0].get("is_error") is not True
                and terminal_events[0].get("result") == FINAL and not invalid_child_events)
    successful = (run_status == "completed" and type(cli_rc) is int and cli_rc == 0 and tool_ok and refusal_ok
                  and map_ok and api_ok and inventory_ok and read_ok and bash_ok and skills_ok and helm_ok and terminal
                  and cleanup["providerThreadJoined"] and cleanup["apiThreadJoined"]
                  and provider.failure is None)
    result = {"schema": "combined-runtime-arm-payload.v1", "arm": arm,
        "status": "passed" if successful else "failed", "returnCode": cli_rc, "runStatus": run_status,
        "processError": process_error, "planError": plan_error, "stdoutBase64": base64.b64encode(stdout).decode(),
        "stderrBase64": base64.b64encode(stderr).decode(), "stdoutBytes": len(stdout), "stderrBytes": len(stderr),
        "stdoutSha256": sha(stdout), "stderrSha256": sha(stderr), "providerRequests": provider.records,
        "providerConnections": provider.connections,
        "providerReceivedCount": provider.received, "providerFailure": provider.failure,
        "providerToolUseIds": [x.get("id") for x in provider_uses],
        "providerResultIds": [x.get("id") for x in provider_results],
        "cliToolUses": cli_uses, "cliToolResults": cli_results, "expectedToolNames": expected_names,
        "observedToolInventory": observed_tools, "inventoryPassed": inventory_ok,
        "skillSignal": skill_signal, "skillsAdvertised": bool(advertised_skills), "skillsPassed": skills_ok,
        "fileReadPassed": read_ok, "kubectlReadPassed": bash_ok,
        "providerAttemptsPassed": provider_attempts_ok, "toolResultsCorrelated": result_equivalence,
        "refusalPassed": refusal_ok, "mcpMapPassed": map_ok, "mcpMapResult": map_results,
        "mcpMapResultValidated": map_ok,
        "helm": {"argv": ["/tools/helm", "version", "--short"], "exitCode": helm_rc,
                 "status": helm_status, "stdoutBytes": len(helm_stdout), "stdoutSha256": sha(helm_stdout),
                 "stderrBytes": len(helm_stderr), "stderrSha256": sha(helm_stderr)},
        "helmAvailable": helm_ok,
        "pre01": {"provenance": api_provenance, "requests": api.records,
                  "cleanupConfirmed": cleanup["apiThreadJoined"]},
        "apiRequests": api.requests_seen,
        "apiCaptured": bool(api.records and api.records[0].get("captured") is True and not api.coverage_failure),
        "apiThreadJoined": cleanup["apiThreadJoined"], "terminalText": FINAL,
        "containerLocalCleanup": cleanup, "elapsedSeconds": round(time.monotonic() - began, 4),
        "billing": "unknown-unmeasured; synthetic provider", "osProcessInventory": "not-claimed"}
    print(json.dumps(result, separators=(",", ":"), ensure_ascii=False))
    return 0 if successful else 1


if __name__ == "__main__":
    raise SystemExit(main())
