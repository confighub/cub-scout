# Recorded from a real ConfigHub server: releases and resources of a space

Captured on 2026-10-10 by `TestSDKReaderMatchesCubOnReleasesResourcesAndEvents`
(`test/integration/sdk_reader_release_parity_test.go`) in the Connected E2E
lane of CI run 38073699271, against the disposable ConfigHub that
`scripts/ci/setup-connected-server.sh` installs: server v0.8.12, `cub` v0.8.12
(`cub-version.txt`). The files are the artifact's bytes, unedited;
`SHA256SUMS` lists them.

| File | Command, run by the test | Notes |
|---|---|---|
| `release-list.json` | `cub release list --space <space> -o json` | Two releases, newest first. The test made a Target, set it as the space's release Target, assigned both Units to it, published, changed one Unit and published again |
| `release-list-first.json` | the same with `--where "ReleaseNum = 1"` | The first release alone |
| `resource-list.json` | `cub resource list --space <space> -o json` | The ConfigMap of each of the two Units |
| `unit-event-list.json` | `cub unit-event list --space <space> -o json` | `[]`: nothing on this server applies a Unit, so it has no unit events |
| `release-event-lists.json` | | Written by the test: what each step printed, how many entries each list held, and whether the two routes were equal |

In the same run the SDK reader's output for each list was identical to cub's,
byte for byte, with the space named by slug and by ID.

The space, the Units, the releases and every identifier belong to an instance
that was deleted when the run ended. The test created them through `cub`, as
its administrator. No user name, email, address or credential is in any file.

What this does not show: a unit event of any kind, a release that a deployer
reported a live status for, a release in more than one space, a resource of a
Unit with a Target attached to a worker, any other server version, a hosted
server, a list longer than one page, or what a credential with fewer rights
would be shown.
