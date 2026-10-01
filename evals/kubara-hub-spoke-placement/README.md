# PRE-04 — Kubara intended hub/spoke placement with unknown observations

Prepared, unrun, recorded-only case for the frozen PRE-04 question in
`evals/benchmark-v1.json`. This packet does not edit the benchmark mapping,
admit the case, establish product quality, or claim live state or savings.
Both arms receive the same two pinned source files and provenance file through
`scaffold.sh`.

## Evidence and limits

The source is `confighub/kubara-confighub` at commit
[`cef6e7337884c6196c3b7c35cec587b8faf07447`](https://github.com/confighub/kubara-confighub/tree/cef6e7337884c6196c3b7c35cec587b8faf07447):

| Fixture | Source path | SHA-256 |
|---|---|---|
| `fixtures/desired-matrix.json` | `data/kubara-platform-matrix/desired-matrix.json` | `e4e111557d4df457ac327c20baf09e5f2f8080cf698eb1767b12ba96962c0ee8` |
| `fixtures/config.yaml` | `examples/kubara/current-platform/source/config.yaml` | `7260fe438066413c1bc44b5f674e4441e3ae9873146e860d4188ef518abc1211` |

The projection identifies a v0.13.0 config-plus-effective-render view. It
records four `cert-manager` intended cells: the dev hub and staging, prod-a,
and prod-b spokes. The selected package is `jetstack/cert-manager@v1.21.0`.
Its desired matrix explicitly reports observed version, sync, health, and
readiness as unknown in all four cells. It records spoke Argo as
`hub-managed`/centralized; that is placement metadata, not evidence that Argo
is installed on any spoke. The source config corroborates cluster roles and
the cert-manager selection.

The desired matrix says the mini-IDP live receipt was not consumed, zero
observation cells were parsed, and there were no live reads. A separate later
`matrix.json` consumes a live receipt and is not part of this case. These files
are a source config and a generated desired-state projection, not raw cluster
objects, a cluster snapshot, or evidence of current installation or health.
Unknown does not mean disabled, unmanaged, unhealthy, or absent. Provenance and
scope are also recorded in `fixtures/source-provenance.json`.

The grader checks the exact JSON answer contract in JavaScript-compatible
regex and rejects wrong cluster roles, omitted or extra/duplicate fields,
claims of live health/installation, and interpretations that turn unknown into
disabled/unmanaged/unhealthy. Four separate required observation fields report
version, sync, health, and readiness across all four cert-manager cells. It does not
grade arbitrary explanatory quality.

## Reproduce the prepared input

From a clone of the pinned source repository, verify source bytes with:

```sh
git show cef6e7337884c6196c3b7c35cec587b8faf07447:data/kubara-platform-matrix/desired-matrix.json | shasum -a 256
git show cef6e7337884c6196c3b7c35cec587b8faf07447:examples/kubara/current-platform/source/config.yaml | shasum -a 256
```

The unit test `TestKubaraHubSpokePlacementEvidenceAndGrader` verifies fixture
hashes and metadata, executes the scaffold twice in separate empty workspaces,
checks byte-identical arm inputs, and runs positive/negative grader samples
through both Go's test contract and the JavaScript regex engine used by the
harness. No cluster, model, or paid eval is needed to prepare or validate this
case.
