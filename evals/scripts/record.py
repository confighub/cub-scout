#!/usr/bin/env python3
"""Record the eval fixtures from a live cluster running evals/fixtures/scenario.yaml.

Writes, under evals/:
  fixtures/cluster/*.yaml         kubectl dumps; both eval arms read these
  mocks/cub-scout/_tools.json     the real MCP tools/list response
  mocks/cub-scout/fixtures/...    one recorded answer per MCP tool call

Usage: evals/scripts/record.py <kube-context> [path-to-cub-scout]

Only the named context is read: the script writes a minified kubeconfig for it
to a temporary file, so the shared current-context is never changed. `cub` is
hidden from PATH, so the recording is of a standalone server.
"""
import glob, json, os, re, shutil, subprocess, sys, tempfile

EVALS = os.path.normpath(os.path.join(os.path.dirname(__file__), ".."))
KINDS = ["namespaces", "deployments", "replicasets", "pods", "services", "configmaps", "events"]
# The scenario's workloads. Each is recorded under every spelling an agent is
# likely to pass as the MCP "resource" argument.
WORKLOADS = [("shop", "checkout"), ("shop", "cart"), ("payments", "payments-api"),
             ("inventory", "inventory"), ("default", "hotfix-worker"), ("temp-testing", "debug-nginx")]
KIND_SPELLINGS = ["deployment", "deploy", "Deployment", "deployments"]


def rpc_session(binary, env, calls):
    """Run one `mcp serve` process and return the result of each call in order."""
    proc = subprocess.Popen([binary, "mcp", "serve"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, env=env, text=True)
    def send(msg):
        proc.stdin.write(json.dumps(msg) + "\n")
        proc.stdin.flush()
    def recv(want_id):
        while True:
            line = proc.stdout.readline()
            if not line:
                raise SystemExit("mcp serve exited before answering id %s" % want_id)
            msg = json.loads(line)
            if msg.get("id") == want_id:
                return msg
    send({"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": {
        "protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "record", "version": "0"}}})
    recv(0)
    send({"jsonrpc": "2.0", "method": "notifications/initialized"})
    out = []
    for i, (method, params) in enumerate(calls, start=1):
        send({"jsonrpc": "2.0", "id": i, "method": method, "params": params})
        out.append(recv(i))
    proc.stdin.close()
    proc.wait(timeout=30)
    return out


def result_text(msg):
    if "error" in msg:
        return json.dumps(msg["error"], indent=2)
    parts = [c.get("text", "") for c in msg["result"].get("content", []) if c.get("type") == "text"]
    return "\n".join(parts)


def main():
    if len(sys.argv) < 2:
        raise SystemExit(__doc__)
    context = sys.argv[1]
    binary = os.path.abspath(sys.argv[2] if len(sys.argv) > 2 else os.path.join(EVALS, "..", "cub-scout"))
    kubeconfig = subprocess.run(["kubectl", "config", "view", "--minify", "--flatten", "--context", context],
                                check=True, capture_output=True, text=True).stdout
    with tempfile.NamedTemporaryFile("w", suffix=".kubeconfig", delete=False) as f:
        f.write(kubeconfig)
    env = dict(os.environ, KUBECONFIG=f.name)
    # The scenario is standalone: hide `cub` so the MCP server does not enter
    # connected mode from the recording host's ConfigHub session.
    env["PATH"] = os.pathsep.join(d for d in env.get("PATH", "").split(os.pathsep)
                                  if not os.access(os.path.join(d, "cub"), os.X_OK))
    try:
        cluster = os.path.join(EVALS, "fixtures", "cluster")
        os.makedirs(cluster, exist_ok=True)
        for kind in KINDS:
            dump = subprocess.run(["kubectl", "get", kind, "-A", "-o", "yaml"], env=env,
                                  check=True, capture_output=True, text=True).stdout
            open(os.path.join(cluster, kind + ".yaml"), "w").write(dump)

        # add_dirs must be inside each case directory, so every case that reads
        # the dump gets its own copy (identical blobs, stored once by git).
        for case_yaml in glob.glob(os.path.join(EVALS, "*", "case.yaml")):
            if re.search(r"add_dirs:\s*\[[^]]*\bcluster\b", open(case_yaml).read()):
                dest = os.path.join(os.path.dirname(case_yaml), "cluster")
                shutil.rmtree(dest, ignore_errors=True)
                shutil.copytree(cluster, dest)

        mocks = os.path.join(EVALS, "mocks", "cub-scout")
        fixtures = os.path.join(mocks, "fixtures")
        calls = [("tools/list", {})]
        names = []
        for tool in ["map", "doctor", "scan", "gitops_status"]:
            calls.append(("tools/call", {"name": tool, "arguments": {}}))
            names.append(os.path.join(fixtures, tool + ".txt"))
        for tool in ["trace", "explain"]:
            for ns, name in WORKLOADS:
                calls.append(("tools/call", {"name": tool, "arguments": {"resource": "deployment/" + name, "namespace": ns}}))
                names.append([os.path.join(fixtures, tool, k, name + ".txt") for k in KIND_SPELLINGS])
        results = rpc_session(binary, env, calls)

        os.makedirs(fixtures, exist_ok=True)
        json.dump(results[0]["result"], open(os.path.join(mocks, "_tools.json"), "w"), indent=2)
        for paths, msg in zip(names, results[1:]):
            text = result_text(msg)
            # Recorded timestamps and pod hashes differ on every run; keep them,
            # but do not let the recording host's paths leak into fixtures.
            text = text.replace(f.name, "<kubeconfig>")
            for path in paths if isinstance(paths, list) else [paths]:
                os.makedirs(os.path.dirname(path), exist_ok=True)
                open(path, "w").write(text)
        print("recorded %d kubectl dumps and %d MCP answers" % (len(KINDS), len(results) - 1))
    finally:
        os.unlink(f.name)


if __name__ == "__main__":
    main()
