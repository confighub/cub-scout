"""Loopback-only, read-only API proxy for the combined live proof.

The proxy keeps upstream credentials out of cub-scout's observation kubeconfig.
It is intentionally narrow: callers choose one of two fixed endpoints and the
proxy forwards only allowlisted GETs to the owned kind API.
"""
from __future__ import annotations

import base64
import copy
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import http.client
import json
import os
import ssl
import threading
from urllib.parse import parse_qs, urlsplit


class ReadOnlyAPIProxy:
    def __init__(self, *, label: str, upstream: str, tls: ssl.SSLContext,
                 authorization: str = "", namespace: str, deployment: str,
                 application: str, missing_deployment: str, event_log=None):
        self.label = label
        self.upstream = urlsplit(upstream)
        if self.upstream.scheme != "https" or not self.upstream.hostname:
            raise ValueError("owned API upstream must be HTTPS")
        self.tls = tls
        self.authorization = authorization
        self.namespace = namespace
        self.deployment = deployment
        self.application = application
        self.missing_deployment = missing_deployment
        self.event_log = event_log
        self._lock = threading.Lock()
        self.requests: list[dict] = []
        self._event_bytes = 0
        self.overflow = False
        owner = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def setup(self):
                super().setup()
                self.connection.settimeout(5)

            def log_message(self, *_args):
                return

            def _finish(self, status: int, body: bytes = b""):
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.send_header("Connection", "close")
                self.end_headers()
                if body:
                    self.wfile.write(body)

            def do_GET(self):
                parsed = urlsplit(self.path)
                if not owner.allowed_path(parsed.path, parse_qs(parsed.query, keep_blank_values=True)):
                    owner.record("GET", parsed.path, 403, "rejected")
                    self._finish(403, b'{"message":"proof proxy refused unowned read"}')
                    return
                conn = None
                status = 0
                try:
                    conn = http.client.HTTPSConnection(owner.upstream.hostname,
                        owner.upstream.port or 443, context=owner.tls, timeout=5)
                    path = parsed.path + (("?" + parsed.query) if parsed.query else "")
                    headers = {"Accept": "application/json", "Connection": "close"}
                    if owner.authorization:
                        headers["Authorization"] = owner.authorization
                    conn.request("GET", path, headers=headers)
                    response = conn.getresponse()
                    status = response.status
                    body = response.read(2 * 1024 * 1024 + 1)
                    if len(body) > 2 * 1024 * 1024:
                        status, body = 502, b'{"message":"proof proxy upstream response exceeded bound"}'
                    owner.record("GET", parsed.path, status, "forwarded")
                    self._finish(status, body)
                except Exception:
                    owner.record("GET", parsed.path, status, "upstream-error")
                    self._finish(502, b'{"message":"proof proxy upstream read failed"}')
                finally:
                    if conn is not None:
                        conn.close()

            def _deny_method(self):
                path = urlsplit(self.path).path
                owner.record(self.command, path, 403, "rejected")
                self._finish(403, b'{"message":"proof proxy permits GET only"}')

            do_POST = _deny_method
            do_PUT = _deny_method
            do_PATCH = _deny_method
            do_DELETE = _deny_method
            do_OPTIONS = _deny_method
            do_HEAD = _deny_method

            def __getattr__(self, name):
                if name.startswith("do_"):
                    return self._deny_method
                raise AttributeError(name)

        class BoundedHTTPServer(ThreadingHTTPServer):
            def __init__(self, address, request_handler):
                self.slots = threading.BoundedSemaphore(16)
                super().__init__(address, request_handler)

            def process_request(self, request, client_address):
                if not self.slots.acquire(blocking=False):
                    request.close()
                    return
                try:
                    super().process_request(request, client_address)
                except Exception:
                    self.slots.release()
                    raise

            def process_request_thread(self, request, client_address):
                try:
                    super().process_request_thread(request, client_address)
                finally:
                    self.slots.release()

        self.server = BoundedHTTPServer(("127.0.0.1", 0), Handler)
        self.server.daemon_threads = False
        self.server.block_on_close = True
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    @property
    def endpoint(self) -> str:
        host, port = self.server.server_address
        return f"http://{host}:{port}"

    def record(self, method: str, path: str, status: int, disposition: str) -> None:
        row = {"type": "request", "endpoint": self.label, "method": method,
               "path": path, "status": status, "disposition": disposition}
        with self._lock:
            if len(self.requests) >= 512:
                self.overflow = True
                return
            self.requests.append({key: value for key, value in row.items() if key != "type"})
            if self.event_log is not None:
                data = json.dumps(row, separators=(",", ":")).encode() + b"\n"
                if self._event_bytes + len(data) > 1024 * 1024:
                    self.overflow = True
                    return
                fd = os.open(self.event_log, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
                try:
                    os.write(fd, data)
                finally:
                    os.close(fd)
                self._event_bytes += len(data)

    def snapshot(self, *, clear: bool = False) -> list[dict]:
        with self._lock:
            rows = [dict(row) for row in self.requests]
            if self.overflow:
                rows.append({"overflow": True})
            if clear:
                self.requests.clear()
            return rows

    def allowed_path(self, path: str, query: dict[str, list[str]]) -> bool:
        ns = self.namespace
        if path in ("/version", "/api", "/api/v1", "/apis", "/apis/apps",
                    "/apis/apps/v1", "/apis/argoproj.io", "/apis/argoproj.io/v1alpha1"):
            return not query
        if path in (f"/apis/apps/v1/namespaces/{ns}/deployments/{self.deployment}",
                    f"/apis/apps/v1/namespaces/{ns}/deployments/{self.missing_deployment}"):
            return not query
        if path == f"/apis/argoproj.io/v1alpha1/namespaces/{ns}/applications/{self.application}":
            return not query
        if path == "/apis/argoproj.io/v1alpha1/applications":
            return (set(query) == {"fieldSelector"} and
                    query.get("fieldSelector") == ["metadata.name=" + self.application])
        if path == f"/api/v1/namespaces/{ns}/events":
            selectors = query.get("fieldSelector", [])
            return (set(query) == {"fieldSelector"} and len(selectors) == 1 and
                    selectors[0] == "involvedObject.name=" + self.deployment)
        return False

    def close(self) -> None:
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        if self.thread.is_alive():
            raise RuntimeError("owned API proxy did not stop")


def upstream_credentials(cluster: dict, user: dict, *, private_dir, label: str):
    """Build verified upstream TLS auth from the private owned-kind config."""
    ca_data = cluster.get("certificate-authority-data")
    if not isinstance(ca_data, str) or not ca_data:
        raise RuntimeError("owned API CA data is missing")
    try:
        ca_pem = base64.b64decode(ca_data, validate=True).decode("ascii")
    except Exception as exc:
        raise RuntimeError("owned API CA data is malformed") from exc
    tls = ssl.create_default_context(cadata=ca_pem)
    raw = user.get("user", {})
    if not isinstance(raw, dict):
        raise RuntimeError("owned context credential record is malformed")
    token = raw.get("token")
    cert_data, key_data = raw.get("client-certificate-data"), raw.get("client-key-data")
    if isinstance(token, str) and token:
        if cert_data or key_data:
            raise RuntimeError("owned context contains multiple credential mechanisms")
        return tls, "Bearer " + token, []
    if not isinstance(cert_data, str) or not isinstance(key_data, str):
        raise RuntimeError("owned context lacks supported inline credentials")
    paths = []
    try:
        for suffix, data in (("cert", cert_data), ("key", key_data)):
            path = private_dir / ("proxy-" + label + "-" + suffix + ".pem")
            path.write_bytes(base64.b64decode(data, validate=True))
            path.chmod(0o600)
            paths.append(path)
        tls.load_cert_chain(str(paths[0]), str(paths[1]))
    except Exception as exc:
        for path in paths:
            path.unlink(missing_ok=True)
        raise RuntimeError("owned context certificate credentials are malformed") from exc
    return tls, "", paths


def observation_kubeconfig(config: dict, *, allowed_endpoint: str, denied_endpoint: str,
                           allowed_cluster_name: str, denied_cluster_name: str,
                           allowed_context: str, denied_context: str, namespace: str) -> dict:
    """Return a credential-free kubeconfig whose contexts are proxy-bound."""
    result = copy.deepcopy(config)
    contexts = result.get("contexts", [])
    context_names = {row.get("name") for row in contexts if isinstance(row, dict)}
    if not {allowed_context, denied_context}.issubset(context_names):
        raise RuntimeError("owned proof contexts are incomplete")
    result["clusters"] = [
        {"name": allowed_cluster_name, "cluster": {"server": allowed_endpoint}},
        {"name": denied_cluster_name, "cluster": {"server": denied_endpoint}},
    ]
    result["users"] = [{"name": "proof-proxy-no-credentials", "user": {}}]
    bindings = {allowed_context: allowed_cluster_name,
                denied_context: denied_cluster_name}
    result["contexts"] = [
        {"name": context_name, "context": {"cluster": cluster_name,
         "user": "proof-proxy-no-credentials", "namespace": namespace}}
        for context_name, cluster_name in bindings.items()
    ]
    result["current-context"] = allowed_context
    for key in ("preferences", "extensions"):
        result.pop(key, None)
    return result


def validate_api_records(rows: list[dict], *, endpoint: str, target_path: str,
                         target_status: int) -> None:
    if not rows:
        raise RuntimeError("action has no recorded Kubernetes API traffic")
    if any(row.get("endpoint") != endpoint for row in rows):
        raise RuntimeError("action used a different bound API endpoint")
    if any(row.get("method") != "GET" for row in rows):
        raise RuntimeError("action attempted a non-GET Kubernetes request")
    if any(row.get("disposition") != "forwarded" for row in rows):
        raise RuntimeError("action attempted a route refused by the read-only proof proxy")
    targets = [row for row in rows if row.get("path") == target_path]
    if not targets or any(row.get("status") != target_status for row in targets):
        raise RuntimeError("action did not prove the exact target read and response status")


def group_action_events(path, expected_phases: list[str]) -> dict[str, list[dict]]:
    """Read bounded request/phase JSONL and reject unassigned or reordered data."""
    raw = path.read_bytes()
    if len(raw) > 1024 * 1024:
        raise RuntimeError("API event log exceeded its 1 MiB bound")
    grouped: dict[str, list[dict]] = {}
    current = None
    phase_order = []
    for line in raw.splitlines():
        if len(line) > 8192:
            raise RuntimeError("API event line exceeded its bound")
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            raise RuntimeError("API event log contains malformed JSON") from None
        if not isinstance(row, dict):
            raise RuntimeError("API event log row is not an object")
        if row.get("type") == "phase":
            current = row.get("phase")
            if not isinstance(current, str) or current in grouped:
                raise RuntimeError("API event log has an unknown or duplicate phase marker")
            grouped[current] = []
            phase_order.append(current)
        elif row.get("type") == "request":
            if current is None:
                raise RuntimeError("API request has no action phase marker")
            if not isinstance(row.get("endpoint"), str) or not isinstance(row.get("method"), str) or not isinstance(row.get("path"), str) or type(row.get("status")) is not int:
                raise RuntimeError("API request record is incomplete")
            grouped[current].append({key: value for key, value in row.items() if key != "type"})
        else:
            raise RuntimeError("API event log contains an unknown record type")
    if phase_order != expected_phases:
        raise RuntimeError("API event phase sequence is incomplete, duplicated, or out of order")
    if any(not rows for rows in grouped.values()):
        raise RuntimeError("an action phase has no Kubernetes request evidence")
    return grouped


def group_cub_events(path, expected_phases: list[str]) -> dict[str, list[dict]]:
    raw = path.read_bytes()
    if len(raw) > 1024 * 1024:
        raise RuntimeError("ConfigHub argv event log exceeded its 1 MiB bound")
    grouped: dict[str, list[dict]] = {}
    current = None
    phase_order = []
    for line in raw.splitlines():
        if len(line) > 8192:
            raise RuntimeError("ConfigHub argv event line exceeded its bound")
        try:
            row = json.loads(line)
        except json.JSONDecodeError:
            raise RuntimeError("ConfigHub argv event log contains malformed JSON") from None
        if not isinstance(row, dict):
            raise RuntimeError("ConfigHub argv event row is not an object")
        if row.get("type") == "phase":
            current = row.get("phase")
            if not isinstance(current, str) or current in grouped:
                raise RuntimeError("ConfigHub argv log has an unknown or duplicate phase")
            grouped[current] = []
            phase_order.append(current)
        elif row.get("type") == "call":
            if current is None or not isinstance(row.get("argv"), list) or type(row.get("exitCode")) is not int:
                raise RuntimeError("ConfigHub command has no valid action phase")
            grouped[current].append(row)
        else:
            raise RuntimeError("ConfigHub argv log contains an unknown record type")
    if phase_order != expected_phases:
        raise RuntimeError("ConfigHub argv phase sequence is incomplete, duplicated, or out of order")
    return grouped


def validate_source_truth(body: dict, *, context: str, require_unit_failure: bool = True) -> None:
    if not isinstance(body, dict) or body.get("context") != context:
        raise RuntimeError("source-truth result lost the selected Kubernetes context label")
    if body.get("status") != "BLOCK" or body.get("source_truth") != "BLOCKED":
        raise RuntimeError("ConfigHub fixture failure was reported as a clean source-truth result")
    errors = body.get("collection_errors")
    if not isinstance(errors, list) or not any(isinstance(error, str) and "ConfigHub" in error for error in errors):
        raise RuntimeError("source-truth result omitted ConfigHub read failure evidence")
    if require_unit_failure and not any("recorded unit lookup unavailable in owned proof" in error for error in errors):
        raise RuntimeError("source-truth result did not preserve the exact recorded unit-read failure")


def mcp_result_json(response: dict) -> dict:
    """Extract the actual tools/call result shape without accepting MCP errors."""
    if not isinstance(response, dict) or response.get("error") is not None:
        raise RuntimeError("MCP tools/call returned a JSON-RPC error")
    result = response.get("result")
    if not isinstance(result, dict) or result.get("isError") is True:
        raise RuntimeError("MCP tools/call returned an error result")
    structured = result.get("structuredContent")
    if isinstance(structured, dict):
        payload = structured.get("data")
        if isinstance(payload, dict):
            return payload
        raise RuntimeError("MCP structuredContent omitted its data object")
    content = result.get("content")
    if not isinstance(content, list):
        raise RuntimeError("MCP tools/call omitted content")
    for block in content:
        if isinstance(block, dict) and block.get("type") == "text" and isinstance(block.get("text"), str):
            try:
                value = json.loads(block["text"])
            except json.JSONDecodeError:
                continue
            if isinstance(value, dict):
                return value
    raise RuntimeError("MCP tools/call content did not contain a JSON object")


def validate_diff(body: dict, *, context: str, status: str, namespace: str,
                  name: str, api_version: str = "apps/v1") -> None:
    if not isinstance(body, dict) or body.get("context") != context:
        raise RuntimeError("trace diff result lost its selected context label")
    if body.get("status") != status:
        raise RuntimeError("trace diff result status does not match the exact API observation")
    resource = body.get("resource")
    if not isinstance(resource, dict) or (resource.get("kind"), resource.get("namespace"),
                                          resource.get("name"), resource.get("apiVersion")) != (
            "Deployment", namespace, name, api_version):
        raise RuntimeError("trace diff result lost the exact rendered GVK and object identity")
    read = body.get("read")
    if not isinstance(read, dict) or not isinstance(read.get("reads"), dict) or read["reads"].get("object") != 1:
        raise RuntimeError("trace diff result omitted the exact live-object read evidence")
