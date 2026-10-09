# Configuration investigation delivery: v2.14 and beyond

Status: v2.14 execution contracts and worked design examples; unimplemented
capabilities below are not release claims. Published baseline: v2.13.3.
This makes the [continuity review](configuration-investigation-continuity.md)
a delivery checklist under [#645](https://github.com/confighub/cub-scout/issues/645),
not a parallel product architecture. The [ordered release sequence](../releases/v2.13-to-v2.14-sequence.md)
and adopted P4 scope remain authoritative.

## The investigation Scout must make useful

For a selected workload, explain: what is running; what intended it; which
controller is reconciling it; how available generation inputs relate to output;
why actual configuration differs; whether the reported delivery refers to this
release; and what evidence is missing. Preserve selection and export those facts.
Inventory and faster navigation are foundations. The useful result is a correct,
scoped configuration explanation with a safe next read.

Use one factual model across CLI, plugin, MCP, watch, bot and TUI. A controller's
own graph or Ready condition is one evidence source, not the whole answer.
Technology-specific adapters populate shared identities, revisions, times,
coverage and omissions; unsupported semantics stay explicit. ConfigHub is an
optional source of intended configuration and recorded governance, not a
prerequisite for standalone diagnosis.

## Ordered delivery and retained commitments

| Order | Outcome and foundation | v2.14 work and proof | Allocation and completion status |
|---|---|---|---|
| 1 | Reuse released contracts and integrate prepared P4 source | Identity/map, watch omissions, recorded pagination/budgets and conservative composition joins integrated onto v2.13.1; full offline Go, CLI docs, genuine acceptance and source CI | #599/#604/#601/#768–#772; integration candidate, not yet published |
| 2 | Explain generation without guessing | Three evidence levels below; producer record and field-map contract/examples, exact digest/target/revision binding and unknown negatives | #778, coordinated with #594/#596/#519; P4 contract checkpoint, reviewed runtime slices follow; full producer adapters may extend P5 |
| 3 | Explain Helm inputs safely | Supported-version precedence contract; source references, optional/denied inputs and redacted sensitive sources | #779; P4 contract checkpoint; bounded supported implementation must have tests and CLI/TUI parity before shipping |
| 4 | Verify the relevant delivery rather than a green parent | Controller capability matrix, exact-release joins, report age and independent workload convergence with wrong-target/revision and partial-population controls | #594/#596/#561/#641/#601/#602/#584; adopted P4 runtime and live acceptance scope remains open |
| 5 | Preserve a coherent standalone investigation | Symptom → source/controller → workload/events → comparison → export; captured scope, identity and original evidence time; no ConfigHub credentials needed | #519/#599/#596; #781 defines local history/retention, #605/#600 retain later storage work |
| 6 | Enforce permission and sensitive-read boundaries | Operation-specific permissions, real restricted reader, denied get/list/watch and separate logs/events/sensitive reads | #780/#599/#596; P4 profile contract and owned-cluster proof; transport authentication remains #604 |
| 7 | Demonstrate benefit with complete costs | Operator correctness/time protocol and frozen paired-agent experiment; cold/warm/idle/change/denial/reconnect requests, bytes, memory and age | #519/#603/#626/#709/#539/#604; no score, reduced bytes or release number establishes savings; reuse/history extend P5 |
| 8 | Maintain accurate doctrine and distribution | Generators render; ConfigHub retains intent; controllers reconcile; Scout observes; user/consumer decides. Public install and supported compatibility proof | #595/#520, every P4 packet and P6 claims; no renderer or new ConfigHub API dependency |

Each packet cites the released component, deterministic tests, existing or new
example and precise missing answer before implementation. A P4 checkpoint closes
only with its supported result or explicit issue/dependency allocation. An open
issue is not a shipped feature. Generic discovery and on-demand metrics retain
separate task-driven design assessment in the roadmap; they cannot replace
configuration depth or expand read permissions implicitly.

## Generation provenance: contract checkpoint (#778)

Keep three levels separate:

1. **Parentage:** an ApplicationSet creates an Application; a HelmRelease points
   at a chart; an XR or kro instance references its definition. Reuse parsers,
   typed owner references, controller trace and composition lineage.
2. **Generation identity:** a producer recorded immutable inputs, tool/version,
   rendered output and exact target/revision/object binding. Reuse source anchors,
   rendered-set digests, receipt chains and connected Links where present.
3. **Field origin:** a producer supplied a supported field map, or an existing
   binding/raw-YAML source position identifies a field. A managedFields writer
   remains last-writer evidence. Neither it nor parentage establishes generation.

A producer record must declare its contract version, producer/tool/version,
input identities and digest algorithms, output identity, target/object/revision
binding, observation time, completeness/coverage and omissions. Supplied evidence
must not trigger automatic rendering, repository fetches or sensitive reads.
Optional field maps name output object identity and path, input identity/path and
supported transformation semantics. Unsupported templates retain the existing
`templated-source-not-resolved` meaning.

Worked design example: a pipeline supplies chart digest C, sanitized values
identity V, tool/version T and rendered-set digest R for target K and revision Q.
Scout recomputes the supplied output digest before joining R to the comparison.
That proves the stated bytes match R, not that the producer actually ran T or is
trusted. Producer authenticity and complete input history remain unknown unless
independently evidenced. A current object with target K but revision Q2 cannot
inherit Q's generation record. A missing values identity yields partial generation
history while retaining useful ownership, events and manifest comparison.

The same contract can describe a supplied Kustomize output or a composition
producer record. Adapters preserve differences; they do not invent equal
semantics. An ApplicationSet definition alone is parentage, not a materialized
record of all generated Applications. A chained generator needs each recorded
edge; an absent edge remains missing, never inferred from matching names.

Required controls: complete/partial records; wrong target/revision/digest;
conflicting producers; absent timestamps; unsupported maps; secret sentinel
redaction; two distinct producer families and reversed input order. Extend
existing receipt and mutation-attribution examples. Runtime completion requires
CLI ASCII/JSON/Markdown, headless TUI and MCP fact parity plus recorded integration;
real producer acceptance must retain genuine captures separately from fixtures.

## Helm effective-input explanation: contract checkpoint (#779)

Show ordered declared sources only under a pinned supported controller/version
contract. Each source reports type, namespace/name/key, optionality, evidence
identity, readability and interpretation limits. Reuse existing reference counts,
missing-input findings, overlap notes and secret dependency metadata. Do not
silently resolve Secret values or run Helm.

Worked design example: a supported adapter declares inputs A then B then inline
values I. A and I are available non-sensitive evidence; B is a denied Secret
reference. Show that declared precedence and B's omission. Even if A and I both
mention `replicaCount`, do not certify an effective value for the full input set
without supported merge semantics and complete relevant evidence. Optional absent
input and denied input are distinct states. A producer-supplied effective output
is a separate generation-identity fact, not inferred from the declarations.

Required controls: conflicting values, missing required/optional inputs, denied
ConfigMap/Secret reads, cross-namespace name collisions, targetPath/unsupported
merge rules, stale supplied output, exact requests and no sensitive sentinel in
any format. Extend Helm/secret examples; prove live restricted-reader behavior
in an owned environment before advertising runtime support.

## Safe continuity and standalone proof (#780/#781)

Bind local navigation to captured scope plus exact object reference/UID; do not
merge histories because contexts or objects have matching names. Define bounded
retention, eviction, opt-in persistence and redaction before adding durable history.
Revisit never renews timestamps. A delayed reply from an old scope cannot replace
current selection. Distinguish navigation history, retained observations, replay
bundles and integrity receipts from intended-state governance.

Permission profiles distinguish inventory, observed identity, controller status,
events, logs and sensitive resource data. Default inventory needs no Secret-value
read. Prove profiles with actual restricted credentials; a read-only mode is not
an RBAC proof. Denied or unreachable reads preserve partial/unknown evidence,
not empty, unmanaged or healthy results.

The guided standalone example uses existing supplied-manifest comparison,
source enrichment, diagnostics and export without ConfigHub credentials. The
connected variant adds recorded intent, bindings, history and governance through
supported reads. Removing that enrichment must not break the standalone loop.
Test cancellation, deletion/recreation, denial/recovery, two colliding scopes,
late replies, retention/eviction and sensitive output in CLI/TUI exports.

## Acceptance and honest claims

Use the existing operator assessment and six-surface conformance work. Include
multiple supported delivery technologies and explicitly blocked cells; the same
permission and evidence bounds apply to each arm. Preserve the frozen 24 agent
cases, grants, budgets and graders. New design examples are outside that benchmark.

v2.14 release notes must separate implemented source, deterministic controls,
recorded process proof, genuine controller/RBAC acceptance and measured outcomes.
Any general improvement claim requires its corresponding passed study. Allocation
of a design to P5 is recorded here and in its issue, never hidden as a completed
v2.14 runtime capability.
