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
The main scenario's mocks are suite-wide (evals/mocks/). The scale scenario
runs live, against the recording cluster (`--mocks off`; see
evals/scripts/live-path.sh), because a recording cannot answer every filter
combination the MCP map tool takes. Its cases get guard mocks instead, which
return an error if they are run without --mocks off.

Usage: evals/scripts/record.py <kube-context> [--scenario main|scale] [--binary path]
       evals/scripts/record.py --scaffolds-only [--scenario main|scale]

Only the named context is read: the script writes a minified kubeconfig for it
to a temporary file, so the shared current-context is never changed. `cub` is
hidden from PATH, so the recording is of a standalone server.
"""
import argparse, glob, json, os, shutil, subprocess, tempfile

EVALS = os.path.normpath(os.path.join(os.path.dirname(__file__), ".."))
KINDS = ["namespaces", "deployments", "replicasets", "pods", "services", "configmaps", "events"]
# The main scenario's workloads. trace and explain answers are recorded per
# workload; an agent mock maps whatever spelling the agent passes as
# "resource" to the right recording.
WORKLOADS = [("shop", "checkout"), ("shop", "cart"), ("payments", "payments-api"),
             ("inventory", "inventory"), ("default", "hotfix-worker"), ("temp-testing", "debug-nginx"),
             ("shop", "orders"), ("billing", "billing"), ("shop", "ledger")]


SCENARIOS = {
    "main": {
        "fixtures": os.path.join(EVALS, "fixtures"),
        "cases": os.path.join(EVALS, "*", "case.yaml"),
        "live": False,
    },
    "scale": {
        "fixtures": os.path.join(EVALS, "fixtures", "scale"),
        "cases": os.path.join(EVALS, "scale", "*", "case.yaml"),
        "live": True,
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


def write_agent_mocks(mocks):
    """trace and explain answer per workload. The harness cannot key a fixed
    mock on a value containing "/", so an agent mock picks the recording."""
    for tool in ["trace", "explain"]:
        parts = [AGENT_MOCK.replace("{tool}", tool)]
        for ns, name in WORKLOADS:
            parts.append("\n## Recording: Deployment `%s` in namespace `%s`\n\n{{file:fixtures/%s/%s.txt}}\n" % (name, ns, tool, name))
        open(os.path.join(mocks, tool + ".md"), "w").write("".join(parts))


LIVE_GUARD = """---
type: fixed
error: true
---

This case runs against a live cluster, not recordings. Run it with --mocks off
and PATH from evals/scripts/live-path.sh; see evals/README.md.
"""


def write_live_guards(sc):
    """Live cases carry mocks that fail loudly, so a run without --mocks off
    cannot silently answer from the main scenario's suite-wide recordings."""
    # Every live case, including live-only ones that read no export.
    for case in sorted(os.path.dirname(p) for p in glob.glob(sc["cases"])):
        dest = os.path.join(case, "mocks", "cub-scout")
        shutil.rmtree(dest, ignore_errors=True)
        os.makedirs(dest)
        for tool in ["doctor", "explain", "gitops_status", "map", "release_check", "scan", "trace"]:
            open(os.path.join(dest, tool + ".md"), "w").write(LIVE_GUARD)


def write_scaffolds(sc):
    """Write each case's scaffold.sh from the recorded export.

    Each run starts in an empty workspace. scaffold.sh writes the export there
    as ./cluster/ with the files embedded, so it does not depend on where the
    harness keeps the case directory. A scaffold whose header declares
    FIXTURE-OWNED is preserved and announced; its case-specific fixtures and
    proof outputs are validated by test/unit rather than replaced by the
    scenario-wide export.
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
        if os.path.isfile(dest):
            with open(dest) as existing:
                if any(line.startswith("# FIXTURE-OWNED:") for _, line in zip(range(3), existing)):
                    print("preserving fixture-owned scaffold: %s (validated from case fixtures)" % dest)
                    continue
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

        if sc["live"]:
            write_live_guards(sc)
            print("recorded %d kubectl dumps; %s cases run live" % (len(KINDS), len(scenario_cases(sc))))
            return

        calls = [("tools/list", {})] + [("tools/call", {"name": t, "arguments": {}})
                                       for t in ["map", "doctor", "scan", "gitops_status"]]
        for tool in ["trace", "explain"]:
            for ns, name in WORKLOADS:
                calls.append(("tools/call", {"name": tool, "arguments": {"resource": "deployment/" + name, "namespace": ns}}))
        results = mcp_session(binary, env, calls)

        def clean(msg):
            # Keep timestamps and pod hashes, but not the recording host's paths.
            return result_text(msg).replace(f.name, "<kubeconfig>")

        mocks = os.path.join(EVALS, "mocks", "cub-scout")
        fixtures = os.path.join(mocks, "fixtures")
        shutil.rmtree(fixtures, ignore_errors=True)
        os.makedirs(fixtures)
        json.dump(results[0]["result"], open(os.path.join(mocks, "_tools.json"), "w"), indent=2)
        for tool, msg in zip(["map", "doctor", "scan", "gitops_status"], results[1:5]):
            open(os.path.join(fixtures, tool + ".txt"), "w").write(clean(msg))
            open(os.path.join(mocks, tool + ".md"), "w").write("---\ntype: fixed\n---\n\n{{file:fixtures/%s.txt}}\n" % tool)
        rest = iter(results[5:])
        for tool in ["trace", "explain"]:
            os.makedirs(os.path.join(fixtures, tool), exist_ok=True)
            for ns, name in WORKLOADS:
                open(os.path.join(fixtures, tool, name + ".txt"), "w").write(clean(next(rest)))
        write_agent_mocks(mocks)
        print("recorded %d kubectl dumps and %d MCP answers" % (len(KINDS), len(results) - 1))
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
