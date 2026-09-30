# Recorded Kubernetes object loader foundation (#604)

This is a contract for an internal parser foundation, not a public CLI/MCP replay
mode. The loader accepts caller-provided bytes/reader only; it never opens a
path, reads kubeconfig, contacts Kubernetes or ConfigHub, or falls back to live
state. A source recording must provide the complete immutable raw object input.

Success requires exactly one object matching all four requested identity
fields: `apiVersion`, `kind`, `metadata.namespace`, and `metadata.name`. The
input may be one YAML/JSON object, a YAML document stream, or a generic
`v1/List` of objects. A generic List wrapper need not have resource name or
namespace metadata; each resource document and List item must be a structurally
valid object with string identity fields (namespace may be the empty string for
a cluster-scoped identity). Duplicate keys, aliases, nested Lists, malformed
or unmodeled documents, oversized input, and ambiguous or absent matches fail
explicitly. Bytes, document count, object count, and nesting depth are bounded.
Empty YAML documents are ignored; explicit null and other non-object documents
are errors.
This identity parser does not validate the full Kubernetes schema for each
resource kind.

On success the internal result retains the selected raw object and reports the
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

The proposed public consumer uses the loader through an explicit
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
