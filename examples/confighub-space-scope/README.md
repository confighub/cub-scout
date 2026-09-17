# ConfigHub Space Scope

Connected cub-scout commands read ConfigHub through `cub`. This example shows
which ConfigHub space each read uses, and how to check it on your own machine.

From v0.5.2, `cub` has no default space. A `cub ... list` with no `--space`
reads the whole organization, and so does `--space ""`. cub-scout therefore
takes a space only from:

1. `--space` (or `--confighub-space`), or the space the traced resource itself
   records, then
2. `CUB_SPACE`, then
3. nowhere. The command refuses, or, for evidence that only adds to a report,
   skips the ConfigHub reads and records a `confighub.scope` omission.

It never reads the `defaultSpace` an older `cub` left in its config, and it never
widens an unnamed space to every space. `*` means every space only when you pass
it.

## See the `cub` calls a command makes

[`record-cub-calls.sh`](record-cub-calls.sh) runs one cub-scout command with a
`cub` wrapper first on `PATH`. The wrapper records each argument vector and
passes the call to the real `cub` unchanged. It is read-only, and records no
output and no credentials.

```bash
go build ./cmd/cub-scout
./examples/confighub-space-scope/record-cub-calls.sh ./cub-scout history deploy/api -n prod
CUB_SPACE=platform ./examples/confighub-space-scope/record-cub-calls.sh ./cub-scout history deploy/api -n prod
./examples/confighub-space-scope/record-cub-calls.sh ./cub-scout impact web --space platform
```

Recorded against `cub` v0.5.2 and a ConfigHub v0.5.1 server. Space names are
replaced.

Nothing named: the command refuses, and no space-scoped read runs.

```
Error: history needs a ConfigHub space and none was given: pass --space <slug> (or --space '*' for every space), or set CUB_SPACE=<slug>. cub has no default space, so none is assumed

--- cub calls (CUB_SPACE=<unset>)
cub auth get-token
```

`CUB_SPACE` names the space: every read carries it, and the output says where it
came from.

```
ConfigHub space: platform (CUB_SPACE)
No ConfigHub change history found for this resource in space platform

--- cub calls (CUB_SPACE=platform)
cub auth get-token
cub changeset list --json --contains prod deploy/api --space platform
```

`impact` reads one space and says what that read covers:

```
Impact Preview: unit/web
ConfigHub space: platform (flag)
...
Notes:
  - Dependents are counted from dependency links stored in space platform only. Units in other spaces that depend on this unit are not counted.
  - 5 link(s) in space platform connect to units in other spaces and are not counted.

--- cub calls (CUB_SPACE=<unset>)
cub unit list --json --quiet --space platform
cub link list --json --quiet --space platform
...
```

## What each command does without a space

| Command | No `--space`, no `CUB_SPACE` |
|---|---|
| `history`, `audit list`, `tree config` | Refuse. `*` is accepted when passed. |
| `impact`, `fleet outliers`, connected `map` deep-dive | Refuse, and refuse `*`: they match units by slug, which is unique only within a space. |
| `map fleet` | Reads every space, as documented, and prints `ConfigHub space: * (command-default)`. |
| `status` | Looks for a worker named after the cluster in every space, and says so. |
| `gitops status`, `doctor`, `map activity --with-confighub` | Skip the ConfigHub reads and record a `confighub.scope` omission. |
| `trace`, `explain`, `receipt verify --with-confighub` | Use the resource's own space, or `--confighub-space`; `CUB_SPACE` does not scope a per-object join. |
| MCP `confighub_units`, `confighub_changesets` | Return an error naming the `space` argument. |
| MCP `confighub_unit_get` | Takes `space`, a `<space>/<slug>` unit, or a unit ID. |

## Validated by

- `TestConnectedCommandsNameTheirSpaceAgainstARecordingCub`
  (`cmd/cub-scout/confighub_space_test.go`) runs `history`, `audit list`,
  `tree config`, `fleet outliers`, `impact` and `map fleet` against a fake `cub`
  whose config still carries a stale `defaultSpace`. Each runs with nothing
  named, with `--space`, and with `CUB_SPACE`, and every call is recorded.
- `TestStatusIgnoresAStaleDefaultSpace` and `TestImpactJoinsLinksWithinOneSpace`
  in the same file.

See also [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md)
for the `scope` fields, and #552 for the design.
