# Composition root display collisions (2.14 candidate)

`inventory.yaml` is an authored offline control with same-kind/name parents
from different API groups. Each known parent has its own typed child reference.
A stale supplied owner UID stays partial, and an ambiguous composite label
keeps its unknown group/version/namespace rather than guessing a parent.

`buildCompositionIndex` now separates supplied references by platform, complete
`ResourceRef` and present/partial state. This is a presentation identity; it
contains neither object UID nor verified cluster identity. Served versions stay
separate rather than asserting that two observations are the same physical
object. Unknown name-only buckets are provisional display groups, not proof of
a common observed parent or orphanhood.

The JSON map shape is unchanged. An unambiguous root keeps its existing
`platform::display-reference` key. Every member of a collision instead receives
that key plus `::ref=` and an unpadded base64url-encoded JSON reference tuple.
Treat map keys as opaque and read the `xr.ref` and `xr.present` fields. Consumers
must not reconstruct keys from names. Colliding root and sibling display labels
include their supplied API version; unknown metadata remains unknown.

Four authored controls in `tree_composition_collision_test.go` cover separate
API groups, stale/unknown partial roots, input reversal, exact JSON/human output,
legacy unambiguous keys, full-reference child ordering, served versions and the
existing loaded trace pane's owner-reference API evidence. No new API reads are
introduced. These controls do not establish automatic TUI composition routing,
controller acceptance, connected/fleet identity or six-surface completion.
