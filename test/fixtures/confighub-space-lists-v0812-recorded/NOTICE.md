# Recorded from a real ConfigHub server: the other lists of a space

Captured on 2026-10-10 by `TestSDKReaderMatchesCubOnARealServer`
(`test/integration/sdk_reader_parity_test.go`) in the Connected E2E lane of CI
run 38071527126, against the disposable ConfigHub that
`scripts/ci/setup-connected-server.sh` installs: server v0.8.12, `cub` v0.8.12
(`cub-version.txt`). `SHA256SUMS` lists the files.

| File | Command, run by the test | Notes |
|---|---|---|
| `worker-list.json` | `cub worker list --space <space> -o json` | One worker, created by the test. **Edited:** one line removed, see below |
| `target-list.json` | `cub target create`, then `cub target list --space <space> -o json` | One Target, with no worker |
| `changeset-list.json` | `cub changeset list --space <space> -o json` | One open ChangeSet |
| `link-list.json` | `cub link list --space <space> -o json` | One Link between the test's two Units |
| `attestation-list.json` | `cub attestation list --space <space> -o json` | One attestation of type `SecurityReview` |
| `unit-list.json`, `space-list.json` | `cub unit list --space <space> -o json`, `cub space list -o json` | The same space, for the tests that look it up |
| `space-lists.json` | | Written by the test: which entities it could create, and how many each list held |

In the same run the SDK reader's output for each list was identical to cub's,
byte for byte, with the space named by slug and by ID.

## The one edit

`cub worker list -o json` prints each worker's `Secret`, the token the worker
authenticates with. The line holding it has been removed from
`worker-list.json`; every other byte of that file, and every byte of the
others, is the artifact's. The worker belonged to an instance that was deleted
when the run ended. The SDK reader has dropped that field since this recording
was made, so `worker-list.json` is also what it returns.

The space, the entities and every identifier belong to that deleted instance.
The test created them through `cub`, as its administrator. No user identity or
address is in any file.

What this does not show: any other server version, a hosted server, a Target
attached to a worker, a closed ChangeSet, a Link across spaces, a list longer
than one page, or what a credential with fewer rights would be shown.
