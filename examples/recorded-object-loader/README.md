# Recorded Kubernetes object loader foundation (#604)

## Typed DeploymentList recordings

Exact `apps/v1 DeploymentList` responses can omit API version/Kind on their
items: the declared API list type supplies `apps/v1 Deployment`. Explicit item
types must agree. The shared loader reports an optional input-wide
`typedListDerivedObjects` count when it supplies either missing field, retaining
the original source SHA-256. Generic `v1/List` does not supply omitted types.
No filename, managedFields entry or current cluster is used to infer identity.

The [authored opt-in contract](../../evals/recorded-typed-list/) uses the unchanged
RUL-03 readable response and has independent exact-byte/scaffold controls:

```sh
./cub-scout map list --recording evals/recorded-typed-list/fixtures/deployments.json --api-version apps/v1 --kind Deployment --format json
./cub-scout explain Deployment/rul03-probe -n rul03-proof --recording evals/recorded-typed-list/fixtures/deployments.json --api-version apps/v1 --format json
./cub-scout map list --recording evals/recorded-typed-list/fixtures/deployments.json --api-version apps/v1 --kind Deployment --tui
```

CLI/MCP/TUI use the same model and source note. Empty input and denied Status
responses still refuse; no per-context joining or current inventory is added.

This describes the internal parser used by recorded explain and
[recorded inventory](../recorded-inventory/). The loader accepts caller-provided
bytes/reader only; it never opens a
path, reads kubeconfig, contacts Kubernetes or ConfigHub, or falls back to live
state. A source recording must provide the complete immutable raw object input.

Success requires exactly one object matching all four requested identity
fields: `apiVersion`, `kind`, `metadata.namespace`, and `metadata.name`. The
input may be one YAML/JSON object, a YAML document stream, a generic
`v1/List` of objects, or the exact typed DeploymentList described above. A generic List wrapper need not have resource name or
namespace metadata; each resource document and List item must be a structurally
valid object with string identity fields (namespace may be the empty string for
a cluster-scoped identity). Duplicate keys, aliases, nested Lists, malformed
or unmodeled documents, oversized input, and ambiguous or absent matches fail
explicitly. Bytes, document count, object count, and nesting depth are bounded.
Empty YAML documents are ignored; explicit null and other non-object documents
are errors.
This identity parser does not validate the full Kubernetes schema for each
resource kind.

On success the internal result retains the selected object (supplying only
omitted type fields from a supported typed envelope) and reports the
SHA-256 and byte/document/object counts of the exact input bytes. It does not
infer capture time from file metadata, object timestamps, or the current clock.
A later replay consumer must omit time-dependent conclusions when capture time
is absent; this foundation itself does not build an explain summary.

For example, a request for `apps/v1 Deployment prod api` over a recorded stream
succeeds only if exactly one object has that exact identity. A second matching
object is ambiguous even if its content is byte-identical; a Deployment in
`staging`, a prefix name such as `api-canary`, or an object with missing
identity metadata cannot satisfy the request. Errors describe the failure
class only and never include object payloads (including Secret data).

## Recorded explain surface contract

The public explain consumer uses the loader through an explicit
`explain <kind/name> --recording FILE --api-version VERSION --namespace NS`
selection; callers must supply `--namespace ""` for an explicitly empty
namespace, and exact resource names are not normalized. Built-in ownership
rules retain normal precedence; ambient custom detector files are not read and
that limitation is explicit in the result. `--tui` renders this one selected
object interactively, without creating a cluster inventory. The JSON summary has a separate `recordedInput`
provenance block and no live `resourceRead` block. The MCP server may instead
be started with a fixed `--recording FILE`; its explain tool takes only exact
identity fields, while other live or connected tools are not exposed.

Recorded summaries reuse only pure object facts. Without explicit trusted
capture-time metadata they omit `currentChange` and controller-revision
comparisons, identify time-dependent conclusions as unavailable, and do not
consult kubeconfig, the network, ConfigHub, filesystem paths supplied to MCP
calls, or a wall clock. Integer values retain Kubernetes-compatible `int64`
types and precision through YAML/JSON decoding. The prepared
[recorded explain case](../../evals/recorded-explain-contract/) uses the complete
raw baseline but only grades file-evidence interpretation; offline Go tests
exercise the product surfaces on those same bytes. Neither is a benchmark run
or evidence of comparative benefit.

Recorded MCP also exposes ownership `map` with exact API version/Kind scope.
It shares the input hash and bounded parser while rejecting duplicate full
identities across the entire recording before filtering. See the
[recorded inventory example](../recorded-inventory/).
