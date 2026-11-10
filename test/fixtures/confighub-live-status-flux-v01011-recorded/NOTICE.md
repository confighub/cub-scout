# Recorded from a real attached cluster: a live status and the Flux objects beside it

Captured on 2026-11-10 by `scripts/ci/live-status-proof.sh` in the Live Status
Proof lane of CI run 42869308295: a disposable ConfigHub v0.10.11 with `cub`
v0.10.11 (`cub-version.txt`), and a kind cluster created and attached with
`cub cluster up scout-live --controller flux`, which installs Flux and argobot.
`SHA256SUMS` lists the files. Every byte is the artifact's; nothing was edited.

| File | How it was read |
|---|---|
| `release-list.json` | `cub release list --space '*' -o json`, once a Release carried a `LiveStatus` |
| `flux-objects.json` | `kubectl get ocirepositories,kustomizations -A -o json` in the attached cluster, seconds later |
| `api-info.json` | the server's `/api/info` |

What it shows, and why it is kept: argobot recorded `Sync: Synced` on two
Releases and named the Kustomization it read as `DataSource`. Each
Kustomization reports the Release's `ManifestDigest` as applied. The cluster
pulls from `oci://172.18.0.1:32181/space/<slug>`, the Docker bridge address,
while the server advertises `localhost:32181`, so the registry address alone
does not identify the source as ConfigHub's.

The server, the cluster and every identifier belonged to instances deleted when
the run ended. No user name, email, address or credential is in any file; the
IDs of that instance's users are, where an entity records who made it. The
OCIRepositories name a pull secret by name only.

What this does not show: Argo CD or Sveltos, a hosted server, a deployment
space created with `cub variant create --target`, a Release that is out of
sync, or a status that has gone stale.
