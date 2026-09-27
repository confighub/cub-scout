#!/usr/bin/env python3
"""Record the eval fixtures from a live cluster running one of the scenarios.

Scenarios:
  main   evals/fixtures/scenario.yaml         cases in evals/<case>/
  scale  evals/fixtures/scale/scenario.yaml   cases in evals/scale/<case>/

Writes, for the scenario:
  <fixtures>/cluster/*.yaml       kubectl dumps; both eval arms read these
  <case>/scaffold.sh              writes those dumps into a run's workspace
  <mocks>/cub-scout/_tools.json   the real MCP tools/list response
  <mocks>/cub-scout/fixtures/...  recorded answers per MCP tool call
The main scenario's mocks are suite-wide (evals/mocks/); the scale scenario's
are written into each scale case's own mocks/ directory, which overrides the
suite's file by file.

Usage: evals/scripts/record.py <kube-context> [--scenario main|scale] [--binary path]
       evals/scripts/record.py --scaffolds-only [--scenario main|scale]

Only the named context is read: the script writes a minified kubeconfig for it
to a temporary file, so the shared current-context is never changed. `cub` is
hidden from PATH, so the recording is of a standalone server.
"""
import argparse, glob, importlib.util, json, os, shutil, subprocess, tempfile
from concurrent.futures import ThreadPoolExecutor

EVALS = os.path.normpath(os.path.join(os.path.dirname(__file__), ".."))
KINDS = ["namespaces", "deployments", "replicasets", "pods", "services", "configmaps", "events"]
# The main scenario's workloads. trace and explain answers are recorded per
# workload; an agent mock maps whatever spelling the agent passes as
# "resource" to the right recording.
WORKLOADS = [("shop", "checkout"), ("shop", "cart"), ("payments", "payments-api"),
             ("inventory", "inventory"), ("default", "hotfix-worker"), ("temp-testing", "debug-nginx"),
             ("shop", "orders"), ("billing", "billing"), ("shop", "ledger")]


def scale_workloads():
    path = os.path.join(EVALS, "fixtures", "scale", "generate.py")
    spec = importlib.util.spec_from_file_location("scale_generate", path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    workloads, _ = mod.plan()
    return [(ns, app) for ns, app, _ in workloads]


SCENARIOS = {
    "main": {
        "fixtures": os.path.join(EVALS, "fixtures"),
        "cases": os.path.join(EVALS, "*", "case.yaml"),
        "per_namespace": False,
        "sessions": 1,
    },
    "scale": {
        "fixtures": os.path.join(EVALS, "fixtures", "scale"),
        "cases": os.path.join(EVALS, "scale", "*", "case.yaml"),
        "per_namespace": True,
        "sessions": 6,
    },
}


def scenario_cases(sc):
    return sorted(os.path.dirname(p) for p in glob.glob(sc["cases"])
                  if "scaffold_script: scaffold.sh" in open(p).read())


def mcp_session(binary, env, calls):
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


def mcp_calls(binary, env, calls, sessions):
    """Spread calls over parallel sessions; results come back in call order."""
    if sessions <= 1 or len(calls) < 2 * sessions:
        return mcp_session(binary, env, calls)
    chunks = [calls[i::sessions] for i in range(sessions)]
    with ThreadPoolExecutor(sessions) as pool:
        parts = list(pool.map(lambda c: mcp_session(binary, env, c), chunks))
    out = [None] * len(calls)
    for i, part in enumerate(parts):
        for j, msg in enumerate(part):
            out[i + j * sessions] = msg
    return out


def result_text(msg):
    if "error" in msg:
        return json.dumps(msg["error"], indent=2)
    parts = [c.get("text", "") for c in msg["result"].get("content", []) if c.get("type") == "text"]
    return "\n".join(parts)


AGENT_MOCK = """---
type: agent
abort_when: never; answer every call, using the not-found reply for anything unrecognised
---

You stand in for the cub-scout MCP `{tool}` tool. Every answer below was recorded
from a real `cub-scout mcp serve` against this cluster. Reply with one of them
exactly as written: no commentary, no reformatting, no summary, no code fences.

Pick the recording by the call's `resource` argument, which must be
`KIND/NAME`. KIND matches a Deployment when, ignoring case, it is `deploy`,
`deployment` or `deployments`. The `namespace` argument, if given, must match
the recording's namespace.

If `resource` has no `/` (a bare name), reply exactly:

tool command failed ({tool} RESOURCE -n NAMESPACE --format json): Error: invalid resource format: use kind/name (e.g., deployment/nginx)

If it has a `/` but matches none of the recordings below, reply exactly:

tool command failed ({tool} RESOURCE -n NAMESPACE --format json): Error: deployments.apps "NAME" not found

In both, take RESOURCE, NAMESPACE and NAME from the call; that is what the real
tool returns.
"""


def write_agent_mocks(mocks, workloads, per_namespace):
    """trace and explain answer per workload. The harness cannot key a fixed
    mock on a value containing "/", so an agent mock picks the recording. With
    many workloads the recordings are grouped per namespace, and the mock
    includes only the namespace the call names, keeping its prompt small."""
    for tool in ["trace", "explain"]:
        parts = [AGENT_MOCK.replace("{tool}", tool)]
        if per_namespace:
            parts.append("\nThe recordings below are the Deployments in namespace `{{input.namespace}}`, the "
                         "namespace this call names; a Deployment not listed is not in that namespace.\n\n"
                         "{{file:fixtures/%s/{input.namespace}.txt}}\n" % tool)
        else:
            for ns, name in workloads:
                parts.append("\n## Recording: Deployment `%s` in namespace `%s`\n\n{{file:fixtures/%s/%s.txt}}\n" % (name, ns, tool, name))
        open(os.path.join(mocks, tool + ".md"), "w").write("".join(parts))


def write_scaffolds(sc):
    """Write each case's scaffold.sh from the recorded export.

    Each run starts in an empty workspace. scaffold.sh writes the export there
    as ./cluster/ with the files embedded, so it does not depend on where the
    harness keeps the case directory.
    """
    cluster = os.path.join(sc["fixtures"], "cluster")
    script = ["#!/usr/bin/env bash", "# Generated by evals/scripts/record.py. Do not edit.", "set -euo pipefail", "mkdir -p cluster"]
    for path in sorted(glob.glob(os.path.join(cluster, "*.yaml"))):
        name = os.path.basename(path)
        script.append("cat > cluster/%s <<'CUB_SCOUT_EVAL_EOF'" % name)
        script.append(open(path).read().rstrip("\n"))
        script.append("CUB_SCOUT_EVAL_EOF")
    body = "\n".join(script) + "\n"
    for case in scenario_cases(sc):
        dest = os.path.join(case, "scaffold.sh")
        open(dest, "w").write(body)
        os.chmod(dest, 0o755)


def record(context, sc, binary):
    kubeconfig = subprocess.run(["kubectl", "config", "view", "--minify", "--flatten", "--context", context],
                                check=True, capture_output=True, text=True).stdout
    with tempfile.NamedTemporaryFile("w", suffix=".kubeconfig", delete=False) as f:
        f.write(kubeconfig)
    env = dict(os.environ, KUBECONFIG=f.name)
    # Standalone: hide `cub` so the MCP server does not enter connected mode
    # from the recording host's ConfigHub session.
    env["PATH"] = os.pathsep.join(d for d in env.get("PATH", "").split(os.pathsep)
                                  if not os.access(os.path.join(d, "cub"), os.X_OK))
    try:
        cluster = os.path.join(sc["fixtures"], "cluster")
        os.makedirs(cluster, exist_ok=True)
        for kind in KINDS:
            dump = subprocess.run(["kubectl", "get", kind, "-A", "-o", "yaml"], env=env,
                                  check=True, capture_output=True, text=True).stdout
            open(os.path.join(cluster, kind + ".yaml"), "w").write(dump)
        write_scaffolds(sc)

        workloads = scale_workloads() if sc["per_namespace"] else WORKLOADS
        calls = [("tools/list", {})] + [("tools/call", {"name": t, "arguments": {}})
                                       for t in ["map", "doctor", "scan", "gitops_status"]]
        for tool in ["trace", "explain"]:
            for ns, name in workloads:
                calls.append(("tools/call", {"name": tool, "arguments": {"resource": "deployment/" + name, "namespace": ns}}))
        results = mcp_calls(binary, env, calls, sc["sessions"])

        def clean(msg):
            # Keep timestamps and pod hashes, but not the recording host's paths.
            return result_text(msg).replace(f.name, "<kubeconfig>")

        staging = tempfile.mkdtemp()
        mocks = os.path.join(staging, "cub-scout")
        fixtures = os.path.join(mocks, "fixtures")
        os.makedirs(fixtures)
        json.dump(results[0]["result"], open(os.path.join(mocks, "_tools.json"), "w"), indent=2)
        for tool, msg in zip(["map", "doctor", "scan", "gitops_status"], results[1:5]):
            open(os.path.join(fixtures, tool + ".txt"), "w").write(clean(msg))
            open(os.path.join(mocks, tool + ".md"), "w").write("---\ntype: fixed\n---\n\n{{file:fixtures/%s.txt}}\n" % tool)
        rest = iter(results[5:])
        for tool in ["trace", "explain"]:
            os.makedirs(os.path.join(fixtures, tool), exist_ok=True)
            grouped = {}
            for ns, name in workloads:
                text = clean(next(rest))
                if sc["per_namespace"]:
                    grouped.setdefault(ns, []).append("## Deployment `%s`\n\n%s\n" % (name, text))
                else:
                    open(os.path.join(fixtures, tool, name + ".txt"), "w").write(text)
            for ns, sections in grouped.items():
                open(os.path.join(fixtures, tool, ns + ".txt"), "w").write("\n".join(sections))
        write_agent_mocks(mocks, workloads, sc["per_namespace"])

        targets = ([os.path.join(case, "mocks") for case in scenario_cases(sc)] if sc["per_namespace"]
                   else [os.path.join(EVALS, "mocks")])
        for target in targets:
            dest = os.path.join(target, "cub-scout")
            shutil.rmtree(dest, ignore_errors=True)
            shutil.copytree(mocks, dest)
        shutil.rmtree(staging)
        print("recorded %d kubectl dumps and %d MCP answers into %d mock set(s)" % (len(KINDS), len(results) - 1, len(targets)))
    finally:
        os.unlink(f.name)


def main():
    ap = argparse.ArgumentParser(usage=__doc__)
    ap.add_argument("context", nargs="?")
    ap.add_argument("--scenario", choices=sorted(SCENARIOS), default="main")
    ap.add_argument("--binary", default=os.path.join(EVALS, "..", "cub-scout"))
    ap.add_argument("--scaffolds-only", action="store_true")
    args = ap.parse_args()
    sc = SCENARIOS[args.scenario]
    if args.scaffolds_only:
        write_scaffolds(sc)
        print("wrote scaffold.sh for each %s case from %s" % (args.scenario, os.path.join(sc["fixtures"], "cluster")))
        return
    if not args.context:
        raise SystemExit(__doc__)
    record(args.context, sc, os.path.abspath(args.binary))


if __name__ == "__main__":
    main()
