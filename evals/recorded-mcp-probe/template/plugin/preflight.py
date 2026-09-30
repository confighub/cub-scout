#!/usr/bin/env python3
"""Bounded, no-model MCP protocol check for the prepared recorded server."""
import hashlib
import json
import os
from pathlib import Path
import subprocess

root = Path(__file__).resolve().parent
wrapper = root / "server-wrapper.sh"
case_dir = root / "evals/recorded-explain-mcp"
fixture = case_dir / "fixtures/deployments.yaml"
recording = case_dir / "fixtures/recording.json"
home = root.parent / "isolated-home"
kubeconfig = root.parent / "kubeconfig-empty"
expected_fixture_sha = "305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8"

sha = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
assert sha(fixture) == expected_fixture_sha, "recorded fixture hash mismatch"
assert json.loads(recording.read_text())["sha256"] == expected_fixture_sha
assert home.is_dir() and not any(home.iterdir()), "isolated HOME must be empty before server start"
assert kubeconfig.is_file() and kubeconfig.stat().st_size == 0, "owned kubeconfig must be empty"

requests = [
    {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
        "protocolVersion": "2024-11-05", "capabilities": {},
        "clientInfo": {"name": "recorded-mcp-probe-preflight", "version": "1"}}},
    {"jsonrpc": "2.0", "method": "notifications/initialized"},
    {"jsonrpc": "2.0", "id": 2, "method": "tools/list"},
    {"jsonrpc": "2.0", "id": 3, "method": "tools/call", "params": {"name": "explain", "arguments": {
        "api_version": "apps/v1", "kind": "Deployment", "namespace": "shop", "name": "checkout",
        "field_path": '.spec.template.spec.containers[name="checkout"].image'}}},
    {"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": {"name": "not-a-tool", "arguments": {}}},
]
proc = subprocess.run(
    [str(wrapper)], input="\n".join(map(json.dumps, requests)) + "\n",
    text=True, capture_output=True, timeout=30, env={"PATH": "/usr/bin:/bin"},
)
assert proc.returncode == 0, (proc.returncode, proc.stderr)
responses = {obj["id"]: obj for line in proc.stdout.splitlines() if line.strip()
             for obj in [json.loads(line)] if "id" in obj}
assert set(responses) == {1, 2, 3, 4}, responses
assert responses[1]["result"]["protocolVersion"] in {"2024-11-05", "2025-03-26"}, responses[1]
tools = responses[2]["result"]["tools"]
assert [tool["name"] for tool in tools] == ["explain"], tools
props = tools[0]["inputSchema"]["properties"]
assert set(props) == {"api_version", "kind", "namespace", "name", "field_path"}, props
assert responses[3]["result"].get("isError") is not True, responses[3]
answer = json.loads(responses[3]["result"]["content"][0]["text"])
assert answer["recordedInput"]["sha256"] == expected_fixture_sha
assert answer["recordedInput"]["identity"] == {
    "apiVersion": "apps/v1", "kind": "Deployment", "namespace": "shop", "name": "checkout"}
assert answer.get("resourceRead") is None
assert answer["owner"] == "Flux"
assert answer["fieldAttribution"]["path"] == '.spec.template.spec.containers[name="checkout"].image'
assert answer["fieldAttribution"]["managers"] == ["kubectl-set"]
assert "nextSteps" not in answer
assert responses[4].get("result", {}).get("isError") or "error" in responses[4], responses[4]
assert sha(fixture) == expected_fixture_sha, "fixture changed during preflight"
assert not any(home.iterdir()), "isolated HOME changed during server run"
print(json.dumps({"protocol": "stdio MCP", "advertisedTools": ["explain"],
                  "exactCall": "passed", "unsupportedTool": "rejected",
                  "fixtureSHA256": expected_fixture_sha, "isolatedHomeUnchanged": True}, sort_keys=True))
