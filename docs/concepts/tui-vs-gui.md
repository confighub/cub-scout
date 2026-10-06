# Scout interfaces and ConfigHub responsibilities

> Status: Current (Deep Dive)
> Last reviewed: 2026-10-05
> Concepts index: [README.md](README.md)

The responsibility boundary follows evidence and authority, not whether the
user opens a terminal or a browser. Scout's CLI, TUI and MCP are interfaces to
the same observer model. A TUI is not restricted to LIVE-only facts: supplied
manifest comparisons and connected intent are supported by their specific
collectors and prerequisites.

| Workflow | Evidence source | Responsibility |
|----------|-----------------|----------------|
| Standalone investigation | One selected Kubernetes context; optional local manifests/recordings | Scout observes ownership, health, relationships and bounded configuration agreement. |
| Connected investigation | Explicit ConfigHub space/View/target plus scoped live observations | Scout adds recorded intent, bindings, history and delivery evidence; omissions stay visible. |
| Local repository preview | Local GitOps manifests and supported generator definitions | Scout parses structure and previews import; it does not execute generators. |
| Rendering / intended-state changes | Generator tooling and supported ConfigHub workflows through `cub` | The intended-state toolchain produces/retains rendered configuration and owns changes. |
| Acceptance / repair | User or governing consumer using checks and policy | The consumer decides and authorizes action; Scout supplies evidence. |

Standalone inspects one context at a time. Connected fleet queries require
explicit identity and available indexed evidence. Neither a UI label nor a
same-named resource establishes a cross-cluster binding.

Use the interface suited to the task while preserving facts, scope, freshness
and omissions. New features require CLI/TUI parity, with supported MCP exposure
using the same semantics. Scope safety and read-only Kubernetes permissions
apply across interfaces. Explicit ConfigHub inventory import/publication is a
separate write boundary.

For shipped capabilities and future extensions, use the
[release continuity review](../reference/configuration-investigation-continuity.md),
[CLI contract](../reference/cli-contract.md) and
[connected boundary](why-connected-mode.md). These describe verified Scout
contracts; they do not promise unverified behavior in another interface.
