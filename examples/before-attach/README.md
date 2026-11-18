# Before `cub cluster attach`

What `cub cluster attach` would meet in a cluster that already runs Argo CD and
Flux, read before it runs
([#888](https://github.com/confighub/cub-scout/issues/888)).

```bash
cub scout doctor --before-attach
./cub-scout doctor --before-attach --format json | jq '.attach.controllers[] | {controller, attachWould, why}'
```

cub-scout only reads. It never attaches, and never says a cluster is safe to
attach. Secrets are read by metadata only.

## The cluster

[`test/e2e/fixtures/attach-lane/brownfield.yaml`](../../test/e2e/fixtures/attach-lane/brownfield.yaml):
Argo CD v3.6.1 with a `guestbook` Application, Flux v2.10.0 with a `podinfo`
Kustomization, and a `legacy-web` Deployment applied by hand, on kind.

## Recorded, not written by hand

Every output here comes from the attach lane
([`scripts/ci/attach-lane.sh`](../../scripts/ci/attach-lane.sh), dispatched
with `-f level=attach`, run 44350497933). For each of Argo CD and Flux it
records the cluster, runs `doctor --before-attach`, attaches the cluster for
real with cub v0.11.4 (`--no-install --prune`) to a disposable ConfigHub,
runs it again, and attaches a second time. The lane fails unless cub-scout
said `use-existing` before the attach that went ahead and `refuse` before the
one attach refused.

| File | When |
|------|------|
| [`expected-output/argo-before.txt`](expected-output/argo-before.txt) | Before attaching through Argo CD |
| [`expected-output/argo-after.txt`](expected-output/argo-after.txt) | After it, before a second attach, which attach refused |
| [`expected-output/flux-before.txt`](expected-output/flux-before.txt) | Before attaching through Flux |
| [`expected-output/flux-after.txt`](expected-output/flux-after.txt) | After it, before a second attach, which attach refused |

The raw recordings, including attach's own output and its refusal, are under
[`test/e2e/fixtures/attach-lane/recorded/`](../../test/e2e/fixtures/attach-lane/recorded/).

## Reading it

Before the Argo CD attach, `--controller argo` reads `use-existing`: Argo CD
is installed, and attach leaves it as found. It also says attach patches
`argocd-cm`, which it then did.

After it, `--controller argo` reads `refuse`, naming
`argocd/confighub-oci-creds`. The second attach was refused with:

```
Failed: the cluster is already wired to a ConfigHub: Argo CD holds registry credentials for oci://172.18.0.1:32181 (Secret argocd/confighub-oci-creds). ...
```

and it created nothing in ConfigHub. `--controller flux` still reads
`use-existing`: attach checks only the chosen controller's credentials, so
the report says per controller what it would do, and lists the cluster as
already attached through Argo CD.

In both runs the `guestbook` Application, the `podinfo` Kustomization and
`legacy-web` were unchanged after an attach with `--prune`.

## What it cannot tell

Spaces of the same names in ConfigHub, whether the cluster reaches ConfigHub's
registry, whether your credentials have cluster-admin, and overlap with what
you deploy later. Everything about Sveltos is read from cub's source and not
run; those findings say `cub-source`.
