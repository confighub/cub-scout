# Recorded from a real ConfigHub server: one Unit's data, its revisions, a View, a change order

Captured on 2026-10-10 by `TestSDKReaderMatchesCubOnEntitiesAndData`
(`test/integration/sdk_reader_entity_parity_test.go`) in the Connected E2E
lane of CI run 38078870310, against the disposable ConfigHub that
`scripts/ci/setup-connected-server.sh` installs: server v0.8.12, `cub` v0.8.12
(`cub-version.txt`). The files are the artifact's bytes, unedited;
`SHA256SUMS` lists them.

| File | Command, run by the test | Notes |
|---|---|---|
| `unit-data.txt` | `cub unit data entity-unit --space <space>` | The file uploaded last, 99 bytes. Its SHA-256 is the Unit's `DataHash` |
| `revision-3-get.json` | `cub revision get entity-unit 3 --space <space> -o json` | The head revision, with the Unit and the user who made it |
| `revision-3-data.txt` | `cub revision data entity-unit 3 --space <space>` | The same 99 bytes; its SHA-256 is the revision's `DataHash` |
| `revision-2-get.json`, `revision-2-data.txt` | the same for revision 2 | The first upload |
| `revision-1-get.json` | `cub revision get entity-unit 1 --space <space> -o json` | The revision the Unit was created with. It has no data: `cub revision data` fails for it, and so does the reader |
| `view-get.json` | `cub view get <id> --space '*' -o json` | A View over a filter, created by the test |
| `changeorder-get.json` | `cub changeorder get entity-order --space <space> -o json` | A change order created with a description and nothing else |
| `entities.json` | | Written by the test: what it could create and whether the two routes were equal |

In the same run the SDK reader's output for each was identical to cub's, byte
for byte: with the space named by slug and by ID, and for the View with no
space, with its space by slug and with its space by ID.

Everything here belongs to an instance that was deleted when the run ended.
The test created it through `cub`, as the instance's built-in administrator,
whose name and address (`admin@local.confighub.dev`) are in the `User` block
`cub` prints with each revision. No credential is in any file.

What this does not show: a revision made by a function, a merge or a change
order; a change order with a workflow, a filter or promotions; a View with
expression or data-path columns; data that is not Kubernetes YAML or is large;
any other server version; a hosted server; what a credential with fewer rights
would be shown.
