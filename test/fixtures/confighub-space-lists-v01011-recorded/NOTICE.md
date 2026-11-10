# Recorded from a real ConfigHub server: the lists of a space, on v0.10.11

Captured on 2026-11-10 by `TestSDKReaderMatchesCubOnARealServer`
(`test/integration/sdk_reader_parity_test.go`) in the Connected E2E lane of CI
run 42865011040, against the disposable ConfigHub that
`scripts/ci/setup-connected-server.sh` installs: server v0.10.11, `cub` v0.10.11
(`cub-version.txt`). `SHA256SUMS` lists the files. Every byte is the artifact's;
nothing was edited.

| File | Command, run by the test |
|---|---|
| `target-list.json` | `cub target create`, then `cub target list --space <space> -o json` |
| `changeset-list.json` | `cub changeset list --space <space> -o json` |
| `link-list.json` | `cub link list --space <space> -o json` |
| `attestation-list.json` | `cub attestation list --space <space> -o json` |
| `unit-list.json`, `space-list.json` | `cub unit list --space <space> -o json`, `cub space list -o json` |
| `space-lists.json` | Written by the test: which entities it created, and how many each list held |

In the same run the SDK reader's output for each list was identical to cub's,
byte for byte. There is no worker list: ConfigHub API 0.10 removed bridge
workers.

The space, the entities and every identifier belong to an instance that was
deleted when the run ended. The test created them through `cub`, as its
administrator. No user name, email, address or credential is in any file; the
IDs of that instance's users are, where an entity records who made it.

What this does not show: any other server version, a hosted server, a list
longer than one page, or what a credential with fewer rights would be shown.
