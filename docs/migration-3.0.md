# Migration notes for 3.0

This guide records planned compatibility changes for the 3.0 major release.
Changes marked deprecated remain available in 2.x until a later release says
otherwise.

## `fleet outliers`

`fleet outliers` is deprecated for planned removal in 3.0. It remains available
in 2.x, but its current slug-based, single-space comparison does not establish
reliable cross-cluster lineage or fleet outliers. The safe zero-comparison
response remains “not compared”; it does not mean clusters are consistent or
missing units.

There is no equivalent replacement in cub-scout 2.x. `map fleet` can provide
connected inventory, but it does not perform lineage-based outlier analysis.
Do not compare unit or target names across spaces as identity. Removal in 3.0
remains conditional: defer it if ConfigHub provides stable cross-cluster
identity and fixtures demonstrate correct comparisons.

The deprecation notice is planned for the v2.15 minor release. This source
documentation is not itself a published notice, and does not start the public
deprecation window. At least one minor release containing the notice must be
published before removal in 3.0.
