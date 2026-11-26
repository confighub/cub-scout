#!/usr/bin/env python3
"""Deterministic bytes-and-tokens eval for offline `graph blast-radius`.

3.3 Theme 1 (Eval-Gated Ephemeral Dependency Fan-Out):
- Zero model calls, zero credits, zero network, zero cluster access.
- Generates deterministic multi-controller recorded YAML snapshots (Flux,
  Argo CD, Flux HelmRelease, and large-snapshot truncation).
- Runs `cub-scout graph blast-radius <target> -n <ns> --recording <file> --format json`
  on each fixture and measures exact UTF-8 bytes of the raw snapshot vs the
  bounded blast-radius JSON answer.
- Uses the deterministic token approximation ceil(utf8_bytes / 4).

This measures evidence size only. Smaller evidence does not by itself prove
that an agent answers correctly for less money.
"""

import argparse
import json
import pathlib
import subprocess
import sys
import tempfile


def tokens_from_bytes(n: int) -> int:
    return (n + 3) // 4


def _noise_deployments(prefix: str, namespace: str, count: int) -> list[str]:
    docs = []
    for i in range(count):
        docs.append(
            f"""apiVersion: apps/v1
kind: Deployment
metadata:
  name: {prefix}-{i:02d}
  namespace: {namespace}
  labels:
    app: {prefix}-{i:02d}
    tier: background
spec:
  replicas: 2
  selector:
    matchLabels:
      app: {prefix}-{i:02d}
  template:
    metadata:
      labels:
        app: {prefix}-{i:02d}
    spec:
      containers:
        - name: worker
          image: ghcr.io/example/{prefix}:v1.{i}
          env:
            - name: SERVICE_ENV
              value: production"""
        )
    return docs


def build_fixtures() -> list[dict]:
    """Return deterministic multi-controller recorded snapshot fixtures."""
    # 1. Flux 4-hop chain (GitRepository -> Kustomization -> ConfigMap -> Deployment -> PDB) + noise
    flux_docs = [
        """# observedAt: 2026-11-26T00:00:00Z
# sourceContext: prod-east
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: infra
  namespace: flux-system
spec:
  url: https://github.com/example/infra""",
        """apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: apps
  namespace: flux-system
spec:
  sourceRef:
    kind: GitRepository
    name: infra
    namespace: flux-system""",
        """apiVersion: v1
kind: ConfigMap
metadata:
  name: app-cfg
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
data:
  LOG_LEVEL: info
  FEATURE_FLAGS: "v1=true,v2=false\"""",
        """apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: apps
    kustomize.toolkit.fluxcd.io/namespace: flux-system
spec:
  replicas: 3
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: ghcr.io/example/api:v1.4.2
          envFrom:
            - configMapRef:
                name: app-cfg""",
        """apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: api-pdb
  namespace: prod
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: api""",
    ] + _noise_deployments("unrelated", "prod", 20)

    # 2. Argo CD Secret fan-out across 3 Deployments + PDBs + noise
    argo_docs = [
        """# observedAt: 2026-11-26T01:00:00Z
# sourceContext: prod-west
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: payments
  namespace: argocd
spec:
  source:
    repoURL: https://github.com/example/payments""",
        """apiVersion: v1
kind: Secret
metadata:
  name: payments-db
  namespace: payments
  labels:
    argocd.argoproj.io/instance: payments
type: Opaque""",
    ]
    for svc in ("checkout", "ledger", "webhook"):
        argo_docs.append(
            f"""apiVersion: apps/v1
kind: Deployment
metadata:
  name: {svc}
  namespace: payments
  annotations:
    argocd.argoproj.io/tracking-id: "payments:apps/Deployment:payments/{svc}"
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: {svc}
    spec:
      containers:
        - name: {svc}
          image: ghcr.io/example/{svc}:v2.1.0
          env:
            - name: DB_URL
              valueFrom:
                secretKeyRef:
                  name: payments-db
                  key: url"""
        )
        argo_docs.append(
            f"""apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {svc}-pdb
  namespace: payments
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: {svc}"""
        )
    argo_docs += _noise_deployments("other", "payments", 20)

    # 3. Flux HelmRelease multi-workload shared ConfigMap + noise
    helm_docs = [
        """# observedAt: 2026-11-26T02:00:00Z
# sourceContext: staging
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: charts
  namespace: flux-system
spec:
  url: https://charts.example.com""",
        """apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: cache-stack
  namespace: flux-system
spec:
  chart:
    spec:
      chart: cache
      sourceRef:
        kind: HelmRepository
        name: charts
        namespace: flux-system""",
        """apiVersion: v1
kind: ConfigMap
metadata:
  name: cache-cfg
  namespace: cache
  labels:
    helm.toolkit.fluxcd.io/name: cache-stack
    helm.toolkit.fluxcd.io/namespace: flux-system
data:
  MAX_MEM: 2Gi""",
        """apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis
  namespace: cache
  labels:
    helm.toolkit.fluxcd.io/name: cache-stack
    helm.toolkit.fluxcd.io/namespace: flux-system
spec:
  template:
    metadata:
      labels:
        app: redis
    spec:
      volumes:
        - name: cfg
          configMap:
            name: cache-cfg
      containers:
        - name: redis
          image: redis:7.2""",
        """apiVersion: apps/v1
kind: Deployment
metadata:
  name: cache-proxy
  namespace: cache
  labels:
    helm.toolkit.fluxcd.io/name: cache-stack
    helm.toolkit.fluxcd.io/namespace: flux-system
spec:
  template:
    metadata:
      labels:
        app: cache-proxy
    spec:
      containers:
        - name: proxy
          image: ghcr.io/example/cache-proxy:v1.0
          envFrom:
            - configMapRef:
                name: cache-cfg""",
        """apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: redis-pdb
  namespace: cache
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: redis""",
    ] + _noise_deployments("noise", "cache", 20)

    # 4. Large snapshot truncation (60 Deployments under one Kustomization)
    large_docs = [
        """# observedAt: 2026-11-26T03:00:00Z
# sourceContext: prod-fleet
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: fleet
  namespace: flux-system
spec:
  path: ./fleet"""
    ]
    for i in range(60):
        large_docs.append(
            f"""apiVersion: apps/v1
kind: Deployment
metadata:
  name: svc-{i:02d}
  namespace: prod
  labels:
    kustomize.toolkit.fluxcd.io/name: fleet
    kustomize.toolkit.fluxcd.io/namespace: flux-system
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: svc-{i:02d}
    spec:
      containers:
        - name: app
          image: ghcr.io/example/svc-{i:02d}:v1.0"""
        )

    return [
        {
            "id": "flux-4hop-configmap",
            "controller": "flux",
            "target": "ConfigMap/app-cfg",
            "namespace": "prod",
            "yaml": "\n---\n".join(flux_docs) + "\n",
        },
        {
            "id": "argo-secret-fanout",
            "controller": "argocd",
            "target": "Secret/payments-db",
            "namespace": "payments",
            "yaml": "\n---\n".join(argo_docs) + "\n",
        },
        {
            "id": "flux-helmrelease-shared-cm",
            "controller": "flux-helm",
            "target": "ConfigMap/cache-cfg",
            "namespace": "cache",
            "yaml": "\n---\n".join(helm_docs) + "\n",
        },
        {
            "id": "large-snapshot-truncation",
            "controller": "flux",
            "target": "Kustomization/fleet",
            "namespace": "flux-system",
            "yaml": "\n---\n".join(large_docs) + "\n",
        },
    ]


def ensure_binary(repo_root: pathlib.Path, binary: pathlib.Path | None, tmp_dir: pathlib.Path) -> pathlib.Path:
    if binary is not None:
        return binary
    candidate = repo_root / "cub-scout"
    if candidate.is_file():
        return candidate
    built = tmp_dir / "cub-scout"
    proc = subprocess.run(
        ["go", "build", "-o", str(built), "./cmd/cub-scout"],
        cwd=str(repo_root),
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"go build failed: {proc.stderr.strip()}")
    return built


def measure_all(repo_root: pathlib.Path, binary: pathlib.Path | None = None) -> dict:
    fixtures = build_fixtures()
    rows = []

    with tempfile.TemporaryDirectory(prefix="cub-scout-blast-eval-") as tmp:
        tmp_path = pathlib.Path(tmp)
        bin_path = ensure_binary(repo_root, binary, tmp_path)

        for fx in fixtures:
            rec_file = tmp_path / f"{fx['id']}.yaml"
            raw_bytes_data = fx["yaml"].encode("utf-8")
            rec_file.write_bytes(raw_bytes_data)
            raw_bytes = len(raw_bytes_data)

            proc = subprocess.run(
                [
                    str(bin_path),
                    "graph",
                    "blast-radius",
                    fx["target"],
                    "-n",
                    fx["namespace"],
                    "--recording",
                    str(rec_file),
                    "--format",
                    "json",
                ],
                capture_output=True,
                text=True,
                check=False,
            )
            if proc.returncode != 0:
                raise RuntimeError(
                    f"graph blast-radius failed for {fx['id']}: {proc.stderr.strip()}"
                )

            out_text = proc.stdout.strip()
            answer_bytes = len(out_text.encode("utf-8"))
            parsed = json.loads(out_text)

            raw_tok = tokens_from_bytes(raw_bytes)
            ans_tok = tokens_from_bytes(answer_bytes)
            pct_of_raw = round((answer_bytes / raw_bytes) * 100.0, 2) if raw_bytes else 0.0
            reduction_pct = round((1.0 - (answer_bytes / raw_bytes)) * 100.0, 2) if raw_bytes else 0.0

            rows.append(
                {
                    "id": fx["id"],
                    "controller": fx["controller"],
                    "target": fx["target"],
                    "namespace": fx["namespace"],
                    "raw_bytes": raw_bytes,
                    "raw_tokens": raw_tok,
                    "answer_bytes": answer_bytes,
                    "answer_tokens": ans_tok,
                    "answer_pct_of_raw": pct_of_raw,
                    "reduction_pct": reduction_pct,
                    "ancestors_count": len(parsed.get("ancestors", [])),
                    "descendants_count": len(parsed.get("descendants", [])),
                    "affected_workloads_count": len(parsed.get("affectedWorkloads", [])),
                    "truncated": bool(parsed.get("truncated", False)),
                    "answer_state": parsed.get("answer", {}).get("state", ""),
                }
            )

    total_raw_bytes = sum(r["raw_bytes"] for r in rows)
    total_raw_tokens = sum(r["raw_tokens"] for r in rows)
    total_ans_bytes = sum(r["answer_bytes"] for r in rows)
    total_ans_tokens = sum(r["answer_tokens"] for r in rows)
    total_pct = round((total_ans_bytes / total_raw_bytes) * 100.0, 2) if total_raw_bytes else 0.0
    total_red = round((1.0 - (total_ans_bytes / total_raw_bytes)) * 100.0, 2) if total_raw_bytes else 0.0

    return {
        "schema": "cub-scout-blast-radius-eval.v1",
        "method": {
            "token_formula": "ceil(utf8_bytes / 4)",
            "model_calls": 0,
            "credits_spent": 0,
            "caveat": "Measures raw recorded snapshot bytes vs bounded offline graph blast-radius JSON answer bytes; does not prove agent accuracy or dollar savings.",
        },
        "cases": rows,
        "totals": {
            "cases": len(rows),
            "raw_bytes": total_raw_bytes,
            "raw_tokens": total_raw_tokens,
            "answer_bytes": total_ans_bytes,
            "answer_tokens": total_ans_tokens,
            "answer_pct_of_raw": total_pct,
            "reduction_pct": total_red,
        },
    }


def render_markdown(report: dict) -> str:
    lines = [
        "| Case | Controller | Raw snapshot (bytes / ~tokens) | Blast-radius JSON (bytes / ~tokens) | Answer % of raw | Reduction | Nodes (anc/desc/wl) | Truncated |",
        "|---|---|---:|---:|---:|---:|---:|---|",
    ]
    for r in report["cases"]:
        nodes = f"{r['ancestors_count']}/{r['descendants_count']}/{r['affected_workloads_count']}"
        lines.append(
            f"| {r['id']} | {r['controller']} | {r['raw_bytes']:,} / {r['raw_tokens']:,} "
            f"| {r['answer_bytes']:,} / {r['answer_tokens']:,} "
            f"| {r['answer_pct_of_raw']:.2f}% | {r['reduction_pct']:.2f}% | {nodes} | {str(r['truncated']).lower()} |"
        )
    tot = report["totals"]
    lines.append(
        f"| **Total ({tot['cases']} cases)** | — | **{tot['raw_bytes']:,} / {tot['raw_tokens']:,}** "
        f"| **{tot['answer_bytes']:,} / {tot['answer_tokens']:,}** "
        f"| **{tot['answer_pct_of_raw']:.2f}%** | **{tot['reduction_pct']:.2f}%** | — | — |"
    )
    return "\n".join(lines) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--format", choices=("md", "json"), default="md")
    parser.add_argument(
        "--repo-root",
        type=pathlib.Path,
        default=pathlib.Path(__file__).resolve().parents[2],
    )
    parser.add_argument("--binary", type=pathlib.Path, default=None)
    args = parser.parse_args()
    report = measure_all(args.repo_root, args.binary)
    if args.format == "json":
        print(json.dumps(report, indent=2))
    else:
        print(render_markdown(report), end="")
    return 0


if __name__ == "__main__":
    sys.exit(main())
