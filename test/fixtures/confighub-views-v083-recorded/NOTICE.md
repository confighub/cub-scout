# Recorded from a real ConfigHub server: a View, its Filter, and a unit list with the View

Captured on 2026-10-09 by `TestRecordViewShapesOnARealServer`
(`test/integration/sdk_reader_parity_test.go`) in the Connected E2E lane of CI
run 37982966226, against the disposable ConfigHub that
`scripts/ci/setup-connected-server.sh` installs: server v0.8.3, `cub` v0.8.3.
The files are the artifact's bytes, unedited; `SHA256SUMS` lists them.

| File | Command, run by the test |
|---|---|
| `filter-get.json` | `cub filter get shape-filter --space <space> -o json`, a Filter on Units with `Slug LIKE 'shape-%'` |
| `view-get.json` | `cub view get shape-view --space <space> -o json`, a View made with `cub view create ... --column Unit.Slug --column Unit.DisplayName --column Unit.HeadRevisionNum --column Space.Slug --column Labels.tier` |
| `unit-list-with-view.json` | `cub unit list --space <space> -o json --view shape-view`, two Units labelled `tier=recorded` |

What they show:

- `cub view get` prints an envelope: `Filter`, `Space` and `View`. The columns
  are at `View.Columns`, and a column made with `--column` has only a `Name`.
- Given `--view`, each entry of the unit list carries `ViewColumns`, a list of
  `{Name, Value}` with one value per column of the View, evaluated by the
  server, and a copy of the `View`.

The space, the Units, the Filter, the View and every identifier belong to an
instance that was deleted when the run ended. The `Permissions` blocks name
that instance's administrator by an ID that existed only there. No credential,
name or address is in any file.

What this does not show: a column with a `ColumnSource` (an expression or a
data path), a View with no Filter or no columns, a column whose value is
empty, `--view` together with `--where`, or `--space "*"`.
