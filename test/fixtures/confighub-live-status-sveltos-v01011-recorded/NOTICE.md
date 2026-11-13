# Recorded from a real attached cluster: a live status and the Sveltos objects beside it

Captured on 2026-11-13 by `scripts/ci/live-status-proof.sh` in the Live Status
Proof lane of CI run 43414463943: a disposable ConfigHub v0.10.11 with `cub`
v0.10.11 (`cub-version.txt`), and a kind cluster created and attached with
`cub cluster up scout-live --controller sveltos`, which installs Sveltos
v1.16.0 and argobot in its Sveltos mode. `SHA256SUMS` lists the files. Every
byte is the artifact's; nothing was edited.

| File | How it was read |
|---|---|
| `release-list.json` | `cub release list --space '*' -o json`, once a Release carried a `LiveStatus` whose `Sync` was `Synced` |
| `sveltos-objects.json` | `kubectl get clusterprofiles,profiles,clustersummaries,clusterconfigurations,clusterreports` of `config.projectsveltos.io`, `-A -o json`, in the attached cluster, seconds later |
| `api-info.json` | the server's `/api/info` |

What it shows, and why it is kept:

- argobot recorded `Sync: Synced` and `Health: Healthy` on the Release of
  `argobot-scout-live`, with `ReporterSync: Provisioned`, and named its
  `DataSource` by the ClusterProfile's name alone.
- It recorded nothing on the Release of the apps space
  (`scout-live-sveltos-apps`), which the root ClusterProfile pulls.
- A ClusterProfile names what it pulls in `spec.policyRefs[].remoteURL.url`
  (`oci://172.18.0.1:32181/space/<slug>:latest`, the Docker bridge address,
  while the server advertises `localhost:32181`).
- Nothing Sveltos writes carries the digest it pulled. A ClusterSummary's
  `status.featureSummaries[]` has a `status`, a `lastAppliedTime` and a `hash`
  that is Sveltos's own, in base64, and is not the Release's `ManifestDigest`.
  That is why cub-scout names a Sveltos ClusterProfile and compares nothing.

The server, the cluster and every identifier belonged to instances deleted when
the run ended. No user name, email, address or credential is in any file; the
IDs of that instance's users are, where an entity records who made it. The
`secretRef` entries are names of Secrets, not their contents.

What this does not show: a hosted server, a deployment space created with
`cub variant create --target`, a Release that is out of sync, a status that
has gone stale, or the workloads (the lane began keeping them after this run).
