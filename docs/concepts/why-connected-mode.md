# Why Connect cub-scout to ConfigHub

> Authoritative standalone/connected responsibility boundary.
> Reviewed 2026-10-05 against published foundations and current contracts.

Scout starts with a useful standalone investigation: what exists, who manages
it, what is failing, and how available intended configuration compares with the
cluster. ConfigHub adds durable intended-state context, explicit revision and
binding records, history and governance evidence where those records exist.

## Standalone foundations

With cluster access, Scout supports ownership/trace across supported controller
families, diagnostics/events, graphs, dependency/risk evidence and scoped reads.
Supplied rendered manifests enable standalone comparison and install receipts;
local raw YAML enables opt-in source-file enrichment. Recorded evidence can be
inspected offline. Live observations require access to the selected cluster.

Standalone does not need a ConfigHub account. Its supported single-cluster
investigation must remain complete and useful without connected credentials.
This does not mean every command is standalone: connected source-truth,
ConfigHub history, Views and fleet queries require their documented sources.

## Connected enrichment

Through supported `cub` interfaces, Scout can read explicitly scoped intent,
revisions, Links/bindings, Resource indexes, delivery reports and history.
These add answers the current cluster alone cannot establish, such as the
recorded upstream unit supplying a field or a stored intended revision.
Cross-environment comparisons need explicit identity and lineage; they cannot
be proved by matching names. Governance declarations remain declarations until
evaluated evidence is available. See the [release continuity review](../reference/configuration-investigation-continuity.md)
for shipped foundations and open acceptance requirements.

## Responsibility split

| Participant | Responsibility |
|-------------|----------------|
| Generator / installer | Resolve inputs and render resources with its own tooling. |
| ConfigHub through `cub` | Retain intended state, revision/binding records and workflow authority. |
| Delivery controller | Reconcile selected sources and report operations/conditions. |
| Scout | Observe, correlate, explain, compare and export bounded evidence. |
| User / governing consumer | Decide acceptance and authorize changes. |

Source parentage, recorded generation inputs/output, and field-level provenance
are different claims. Scout already supports source anchors, mutation
attribution, raw-YAML file/line enrichment and connected bindings. A complete
Helm/Kustomize field map requires render-time evidence; ConfigHub's involvement
alone does not establish that evidence.

## Interface and write boundaries

The supported `cub` CLI is the connected integration boundary. Standalone
Scout continues to function without `cub`. `import --git-path` is a local
structure/import-preview flow. `cub variant upload` ingests already rendered
resources; Scout does not render through the SDK or call the removed
`cub gitops` group.

Scout never mutates Kubernetes. Observation and MCP reads remain read-only;
explicit inventory import or fact publication to ConfigHub is a separate write
boundary. Connecting does not grant Scout acceptance authority or permission to
repair the cluster. Receipts record historical checks and integrity, not an
approval or an authenticated producer by themselves.

## Planning references

- [Architecture](architecture.md)
- [Roadmap](../roadmap.md)
- [Execution plan](../roadmap-3.0-execution.md)
- [CLI contract](../reference/cli-contract.md)
