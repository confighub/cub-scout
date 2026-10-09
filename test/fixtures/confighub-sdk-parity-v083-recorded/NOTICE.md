# Recorded from a real ConfigHub server: unit get, unit list, space list

Captured on 2026-10-09 by `TestSDKReaderMatchesCubOnARealServer`
(`test/integration/sdk_reader_parity_test.go`) in the Connected E2E lane of CI
run 37967084137, against the disposable ConfigHub that
`scripts/ci/setup-connected-server.sh` installs: server v0.8.3, `cub` v0.8.3
(`cub-version.txt`). The files are the artifact's bytes, unedited;
`SHA256SUMS` lists them.

| File | Command, run by the test | Notes |
|---|---|---|
| `unit-get.json` | `cub unit get parity-unit --space <space> -o json` | The SDK reader's output for the same Unit in the same run was identical, byte for byte |
| `unit-list.json` | `cub unit list --space <space> -o json` | Two Units, created by the test |
| `space-list.json` | `cub space list -o json` | The test's space and the instance's `default` space |
| `timings.json` | | One read by each route, 15 runs, on the CI runner with the server in a kind cluster on the same machine |

The space, the two Units and every identifier belong to an instance that was
deleted when the run ended. The test created them through `cub`, as an
administrator of that instance. No credential, user identity or address is in
any file.

What this does not show: any other server version, a hosted server, a Unit
with a target, an upstream or a live revision, a list longer than one page, or
what a credential with fewer rights would be shown.
