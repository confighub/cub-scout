# HLT-03 — Consul workload convergence with Ingress residue

This is a prepared, unrun source-evidence case for benchmark-v1 HLT-03. It
does not make the 24-case benchmark executable and supports no live-state,
product-quality, or savings claim.

## Evidence and limits

The receipt and child capture under `fixtures/` are byte-for-byte copies from the public
[`confighub/helm-expt` repository](https://github.com/confighub/helm-expt),
pinned at revision
[`9ab4c753a888dc305a3c07956c9f8f5a19eb70a0`](https://github.com/confighub/helm-expt/tree/9ab4c753a888dc305a3c07956c9f8f5a19eb70a0):

- `receipt.yaml` from
  `runs/live-helm-confighub-compare/hashicorp-consul-secure-mesh-existing-secrets/receipt.yaml`
- `argocd-child.json` from
  `runs/live-helm-confighub-compare/hashicorp-consul-secure-mesh-existing-secrets/argocd-core-child.json`

The receipt's observed time is `2026-06-14T14:36:32Z`; its Argo wait is later
recorded as `Synced`/`Progressing` and `watch`, while receipt workload checks
for the separate Helm/direct/Argo legs report pass. The child Application
capture identifies the child `hashicorp-consul-secure-mesh-existing-secrets-parity`
in namespace `argocd`; its recorded resources include
`Ingress/consul/consul-consul-ui` as `Synced`/`Progressing`. The child JSON is
not a full Kubernetes object snapshot, and the two files are not an atomic
capture of one instant. Neither establishes current state or the cause of the
Ingress health state. This case asks about the named child, not a root
Application's health.

The source capture contains no Kubernetes `Secret` object or secret data
payload. The receipt has references to staged/separated Secret filenames only;
those references are retained as part of the unmodified public receipt.

`fixtures/source-provenance.json` records the pinned paths, hashes, and scope
limits. The source file SHA-256 values are:

| File | SHA-256 |
|---|---|
| `receipt.yaml` | `521b08ae448f4d6503123ddef0263944887ced4204debab457677829c43fba15` |
| `argocd-child.json` | `adf89697a41493e7399ff08b0e777a97e14d691b377d2fdddff2fa4faa27a078` |

## Reproduce and verify the pinned files

From a clone of `confighub/helm-expt` that contains the pinned commit, run:

```sh
git show 9ab4c753a888dc305a3c07956c9f8f5a19eb70a0:runs/live-helm-confighub-compare/hashicorp-consul-secure-mesh-existing-secrets/receipt.yaml | shasum -a 256
git show 9ab4c753a888dc305a3c07956c9f8f5a19eb70a0:runs/live-helm-confighub-compare/hashicorp-consul-secure-mesh-existing-secrets/argocd-core-child.json | shasum -a 256
```

`scaffold.sh` copies the receipt, child capture, and bounded provenance file
byte-for-byte into `cluster/` for each arm. No cluster, MCP server, or paid
model is used to prepare this case.
The grader accepts exactly one JSON object with the declared fields in any
order; it rejects extra/missing keys, wrong status or identity, unsupported
cause/current/atomic/full-snapshot claims, and prose outside the object. It
measures only this narrow answer contract, not free-form reasoning quality.
