#!/usr/bin/env python3
"""Record the gitops-settings-no-self-heal eval case from a throwaway kind cluster.

Creates one uniquely named kind cluster with a private kubeconfig, installs the
real Argo CD and Flux CRDs (no controllers), applies scenario.yaml, and records:

  fixtures/cluster/*.yaml   the raw export both arms read (kubectl get -o yaml
                            --show-managed-fields)
  mocks/cub-scout/          what a standalone `cub-scout mcp serve` answered on
                            that cluster: tools/list, doctor, map, scan,
                            gitops_status, and gitops_settings for each call in
                            SETTINGS_CALLS
  scaffold.sh               writes the export into ./cluster/ for a run
  recording.json            provenance and file hashes

The cluster is deleted afterwards and the shared kubeconfig is never written.
Needs kind, kubectl, curl and the flux CLI. Run from the repository root:

    go build ./cmd/cub-scout
    python3 evals/gitops-settings-no-self-heal/record.py
"""
import datetime, hashlib, json, os, pathlib, shutil, subprocess, tempfile, uuid

CASE = pathlib.Path(__file__).resolve().parent
REPO = CASE.parent.parent
ARGO = "https://raw.githubusercontent.com/argoproj/argo-cd/v3.5.3/manifests/crds/"
ARGO_CRDS = {
    "application-crd.yaml": "5dde0e229249b6b707beb98674c1deae3949d5c319a6c45b9f5a80c99618e40c",
    "appproject-crd.yaml": "ab225266944322750136f1198d93786e4a79a0e43c8d149abfba44da30a3eac8",
}
NODE_IMAGE = "kindest/node:v1.35.0"
# What the baseline arm reads. ConfigMaps are exported from the argocd
# namespace only; the others from every namespace.
KINDS = ["namespaces", "appprojects.argoproj.io", "applications.argoproj.io", "gitrepositories.source.toolkit.fluxcd.io",
         "helmrepositories.source.toolkit.fluxcd.io", "kustomizations.kustomize.toolkit.fluxcd.io",
         "helmreleases.helm.toolkit.fluxcd.io"]
# The gitops_settings calls that have a recording. A recording cannot answer
# every combination of the tool's filters, so the mock answers these and says
# so for anything else. Each filter is recorded in the default summary view
# and in the per-object deployers view.
SETTINGS_FILTERS = [("all", None), ("self-heal-off", "self-heal=off"), ("suspend-on", "suspend=on")]
SETTINGS_VIEWS = ["summary", "deployers"]
FIXED_TOOLS = ["doctor", "map", "scan", "gitops_status"]

NOT_RECORDED = """---
type: fixed
error: true
---

This scenario holds GitOps deployer objects only: there are no workloads in it,
so there is nothing for `{tool}` to answer about and no recording of it. Use
`gitops_settings`, `gitops_status`, `map`, `doctor` or `scan`, or read the export.
"""

SETTINGS_MOCK_HEAD = """---
type: agent
abort_when: never; answer every call, using the not-recorded reply for anything unrecognised
---

You stand in for the cub-scout MCP `gitops_settings` tool. Every answer below was
recorded from a real `cub-scout mcp serve` against this cluster. Reply with one
of them exactly as written: no commentary, no reformatting, no summary, no code
fences.

Pick the recording from the call's arguments:

- `view`: `summary` or `deployers`. When the call has no `view`, use `summary`,
  which is the tool's default.
- `setting`: an array, or absent. Treat `on` and `true` as the same value, and
  `off` and `false` as the same value. So `["self-heal=false"]` selects the
  `self-heal=off` recordings and `["suspend=true"]` the `suspend=on` ones.

There is a recording only for: no `setting` at all; `setting` equal to exactly
`["self-heal=off"]`; and `setting` equal to exactly `["suspend=on"]`.

For any other call (a different or additional `setting`, `view` set to `groups`,
`settings` or `all`, or any `namespace`, `project` or `context` argument) reply
exactly:

This recorded scenario answers gitops_settings only for these calls: no arguments; setting ["self-heal=off"]; setting ["suspend=on"]; each optionally with view summary or deployers. The real tool accepts any filter and view. Call it with no arguments for the full inventory.
"""


def sha(path):
    return hashlib.sha256(pathlib.Path(path).read_bytes()).hexdigest()


def run(args, env=None, content=None, timeout=300):
    return subprocess.run(args, env=env, input=content, text=True, capture_output=True, check=True, timeout=timeout).stdout


def mcp_session(binary, env, calls):
    """Run one `mcp serve` process and return the reply to each call in order."""
    proc = subprocess.Popen([binary, "mcp", "serve"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, env=env, text=True)

    def send(message):
        proc.stdin.write(json.dumps(message) + "\n")
        proc.stdin.flush()

    def receive(want):
        while True:
            line = proc.stdout.readline()
            if not line:
                raise SystemExit("mcp serve exited before answering id %s" % want)
            message = json.loads(line)
            if message.get("id") == want:
                return message

    send({"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": {
        "protocolVersion": "2025-06-18", "capabilities": {}, "clientInfo": {"name": "record", "version": "0"}}})
    receive(0)
    send({"jsonrpc": "2.0", "method": "notifications/initialized"})
    replies = []
    for index, (method, params) in enumerate(calls, start=1):
        send({"jsonrpc": "2.0", "id": index, "method": method, "params": params})
        replies.append(receive(index))
    proc.stdin.close()
    proc.wait(timeout=30)
    return replies


def text_of(reply):
    assert "error" not in reply and not reply["result"].get("isError"), reply
    return "\n".join(c.get("text", "") for c in reply["result"].get("content", []) if c.get("type") == "text")


def provision(work):
    """Create an owned kind cluster with the Argo CD and Flux CRDs and no
    controllers. Returns its name, private kubeconfig path and kubectl env."""
    for tool in ["kind", "kubectl", "curl", "flux"]:
        assert shutil.which(tool), tool + " is required on PATH"
    cfg = work / "config"
    name = "scout-eval-settings-" + uuid.uuid4().hex[:8]
    env = dict(os.environ, KUBECONFIG=str(cfg))
    assert name not in run(["kind", "get", "clusters"]).splitlines()
    try:
        run(["kind", "create", "cluster", "--name", name, "--image", NODE_IMAGE, "--kubeconfig", str(cfg), "--wait", "90s"])
        cfg.chmod(0o600)
        for crd, want in ARGO_CRDS.items():
            path = work / crd
            run(["curl", "--fail", "--silent", "--show-error", "--location", "--retry", "3", "--proto", "=https",
                 "--proto-redir", "=https", ARGO + crd, "-o", str(path)])
            assert sha(path) == want, crd + " checksum mismatch"
            run(["kubectl", "apply", "--server-side", "-f", str(path)], env=env)
        exported = run(["flux", "install", "--export", "--components=source-controller,kustomize-controller,helm-controller"])
        crds = [doc for doc in exported.split("\n---\n") if "\nkind: CustomResourceDefinition\n" in "\n" + doc + "\n"]
        assert crds, "no Flux CRDs in flux install --export"
        run(["kubectl", "apply", "--server-side", "-f", "-"], env=env, content="\n---\n".join(crds) + "\n")
        established = ["crd/applications.argoproj.io", "crd/appprojects.argoproj.io", "crd/kustomizations.kustomize.toolkit.fluxcd.io",
                       "crd/helmreleases.helm.toolkit.fluxcd.io", "crd/gitrepositories.source.toolkit.fluxcd.io",
                       "crd/helmrepositories.source.toolkit.fluxcd.io"]
        run(["kubectl", "wait", "--for=condition=Established", "--timeout=120s", *established], env=env)
    except BaseException:
        delete_cluster(name, cfg)
        raise
    return name, cfg, env


def delete_cluster(name, cfg):
    subprocess.run(["kind", "delete", "cluster", "--name", name, "--kubeconfig", str(cfg)], capture_output=True)
    assert name not in run(["kind", "get", "clusters"]).splitlines(), "owned cluster not removed"


def standalone_env(env, home):
    # Standalone: hide `cub` so the MCP server does not enter connected mode
    # from the recording host's ConfigHub session.
    return dict(env, HOME=str(home), PATH=os.pathsep.join(
        d for d in env.get("PATH", "").split(os.pathsep) if not os.access(os.path.join(d, "cub"), os.X_OK)))


def export_cluster(env, case):
    """Write the raw export both arms read into case/fixtures/cluster."""
    cluster = case / "fixtures" / "cluster"
    shutil.rmtree(case / "fixtures", ignore_errors=True)
    cluster.mkdir(parents=True)
    for kind in KINDS:
        dump = run(["kubectl", "get", kind, "-A", "-o", "yaml", "--show-managed-fields"], env=env)
        (cluster / (kind.split(".")[0] + ".yaml")).write_text(dump)
    (cluster / "configmaps.yaml").write_text(
        run(["kubectl", "get", "configmaps", "-n", "argocd", "-o", "yaml", "--show-managed-fields"], env=env))
    return cluster


def write_scaffold(case):
    script = ["#!/usr/bin/env bash", "# FIXTURE-OWNED: generated by this case's record.py from fixtures/cluster. Do not edit.",
              "set -euo pipefail", "mkdir -p cluster"]
    for path in sorted((case / "fixtures" / "cluster").glob("*.yaml")):
        script += ["cat > cluster/%s <<'CUB_SCOUT_EVAL_EOF'" % path.name, path.read_text().rstrip("\n"), "CUB_SCOUT_EVAL_EOF"]
    (case / "scaffold.sh").write_text("\n".join(script) + "\n")
    (case / "scaffold.sh").chmod(0o755)


def write_manifest(case, binary, notes, extra=None):
    files = sorted(p for p in case.rglob("*") if p.is_file() and p.name != "recording.json" and "__pycache__" not in p.parts
                   and (p.parts[len(case.parts)] in ("fixtures", "mocks", "graders")
                        or p.name in ("scenario.yaml", "scaffold.sh", "expected.json")))
    manifest = {
        "recordedAt": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
        "sourceCommit": run(["git", "rev-parse", "HEAD"]).strip(),
        "sourceDirty": bool(run(["git", "status", "--porcelain", "--", "cmd", "pkg", "go.mod", "go.sum"]).strip()),
        "binarySHA256": sha(binary),
        "kubernetesNodeImage": NODE_IMAGE,
        "argoCDCRDs": {ARGO + crd: digest for crd, digest in ARGO_CRDS.items()},
        "fluxCLI": run(["flux", "--version"]).strip(),
        "managedFields": True,
        "notes": notes,
        "files": {str(p.relative_to(REPO)): sha(p) for p in files},
    }
    manifest.update(extra or {})
    (case / "recording.json").write_text(json.dumps(manifest, indent=2) + "\n")


def main():
    binary = REPO / "cub-scout"
    assert binary.exists(), "build first: go build ./cmd/cub-scout"
    work = pathlib.Path(tempfile.mkdtemp(prefix="scout-eval-settings-"))
    work.chmod(0o700)
    shared = pathlib.Path.home() / ".kube/config"
    shared_before = sha(shared) if shared.exists() else None
    name = cfg = None
    try:
        name, cfg, env = provision(work)
        run(["kubectl", "apply", "--server-side", "-f", str(CASE / "scenario.yaml")], env=env)
        cluster = export_cluster(env, CASE)

        settings_calls = []
        for label, setting in SETTINGS_FILTERS:
            for view in SETTINGS_VIEWS:
                arguments = {"view": view}
                if setting:
                    arguments["setting"] = [setting]
                settings_calls.append((label, view, arguments))
        calls = [("tools/list", {})] + [("tools/call", {"name": tool, "arguments": {}}) for tool in FIXED_TOOLS]
        calls += [("tools/call", {"name": "gitops_settings", "arguments": arguments}) for _, _, arguments in settings_calls]
        # The default call must equal the explicit summary view, or the mock's
        # "no view means summary" rule would misstate the tool.
        calls.append(("tools/call", {"name": "gitops_settings", "arguments": {}}))
        replies = mcp_session(str(binary), standalone_env(env, work), calls)

        def clean(reply):
            return text_of(reply).replace(str(cfg), "<kubeconfig>").replace(str(work), "<home>")

        mocks = CASE / "mocks" / "cub-scout"
        shutil.rmtree(CASE / "mocks", ignore_errors=True)
        (mocks / "fixtures" / "gitops_settings").mkdir(parents=True)
        (mocks / "_tools.json").write_text(json.dumps(replies[0]["result"], indent=2) + "\n")
        assert "gitops_settings" in [tool["name"] for tool in replies[0]["result"]["tools"]]
        for tool, reply in zip(FIXED_TOOLS, replies[1:1 + len(FIXED_TOOLS)]):
            (mocks / "fixtures" / (tool + ".txt")).write_text(clean(reply))
            (mocks / (tool + ".md")).write_text("---\ntype: fixed\n---\n\n{{file:fixtures/%s.txt}}\n" % tool)
        for tool in ["explain", "trace", "release_check"]:
            (mocks / (tool + ".md")).write_text(NOT_RECORDED.format(tool=tool))
        body = [SETTINGS_MOCK_HEAD]
        recorded = iter(replies[1 + len(FIXED_TOOLS):])
        by_call = {}
        for label, view, arguments in settings_calls:
            answer = clean(next(recorded))
            by_call[(label, view)] = answer
            (mocks / "fixtures" / "gitops_settings" / ("%s.%s.txt" % (label, view))).write_text(answer)
            setting = "no `setting`" if "setting" not in arguments else "`setting` %s" % json.dumps(arguments["setting"])
            body.append("## Recording: %s, `view` `%s`\n\n{{file:fixtures/gitops_settings/%s.%s.txt}}\n" % (setting, view, label, view))
        assert clean(next(recorded)) == by_call[("all", "summary")], "the default call is not the summary view"
        (mocks / "gitops_settings.md").write_text("\n".join(body))

        write_scaffold(CASE)
        write_manifest(CASE, binary, [
            "Deployer objects only. No GitOps controller is installed: nothing reconciles and no object has a status.",
            "The CRDs are the real Argo CD and Flux ones, so values a CRD defaults (Flux Kustomization spec.force) are as a real cluster returns them.",
            "MCP answers were captured after the raw export, in sequence; this is not an atomic snapshot.",
            "gitops_settings is recorded for the calls in SETTINGS_FILTERS x SETTINGS_VIEWS only; the mock says so for any other call.",
        ])
        print("recorded %d export files and %d MCP answers" % (len(list(cluster.glob("*.yaml"))), len(replies) - 1))
    finally:
        if name:
            delete_cluster(name, cfg)
        assert (sha(shared) if shared.exists() else None) == shared_before, "shared kubeconfig changed"
        shutil.rmtree(work, ignore_errors=True)


if __name__ == "__main__":
    main()
