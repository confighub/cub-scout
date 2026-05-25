# Receipts and Proofs

> Status: Current (Deep Dive)
> Last reviewed: 2026-05-25
> Concepts index: [README.md](README.md)

## TL;DR

> A **receipt** is a stamped, hand-offable record of one verification — an in-toto Statement v1 envelope around a verdict and its evidence, fingerprinted via SHA-256 over RFC 8785 canonical JSON. The **proof** is that fingerprint: any third party can recompute it over the receipt's canonical bytes and confirm nothing has been edited since the receipt was stamped. A receipt without proof is a claim; a receipt with proof is evidence.

The rest of this doc unpacks each term, places them inside the broader vocabulary they sit alongside (log, journal, record, ledger, provenance), and notes where the boundaries blur honestly.

---

## Why this matters

"Receipt" and "proof" are loaded words. cub-scout uses both, and it's worth being precise about which sense we mean — both for users picking the right tool and for integrators choosing the right vocabulary in their own docs.

---

## Receipt

A **receipt** in cub-scout is a stamped, hand-offable record of a single verification — *who ran what check, on what subject, at what time, with what verdict and what supporting evidence.*

Structurally it is an [in-toto Statement v1](https://github.com/in-toto/attestation/blob/main/spec/v1/statement.md) envelope wrapping a predicate (verdict + evidence + optional pointers to upstream receipts). It is fingerprinted via SHA-256 over [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785) canonical JSON, so any third party can recompute the fingerprint and detect tampering. Receipts are optionally chainable: the predicate carries `inputAttestations[]` referencing upstream receipts by digest.

What lifts a receipt above a plain record:

- **It is transactional, not passive.** A record describes an entity. A receipt describes a *check that ran* — there is a verb in it (`verify`, `aggregate`, `emit`).
- **It is stamped.** Cryptographic integrity is intrinsic, not bolted on by the storage layer.
- **It is hand-offable.** The in-toto envelope is a standard format meant to be passed to a third party who does not trust the issuer's database.
- **It is contestable.** Anyone with the same inputs can re-run the verify and challenge the stamped verdict.

Single-line definition:

> **A receipt is a record-with-proof of one verification, in a hand-offable envelope.**

The wire format is documented in [docs/reference/json-contracts.md § Receipt Contract](../reference/json-contracts.md). The operational lifecycle is documented in [docs/howto/receipts-end-to-end.md](../howto/receipts-end-to-end.md).

---

## Proof

A **proof** in cub-scout is *the verifiable property of a receipt, not a separate artifact.*

The proof is: you can recompute the fingerprint over the canonical JSON of the predicate, and if it matches the stamped value, the receipt has not been tampered with. For chained receipts the proof extends — you walk the chain, independently re-validate each upstream's fingerprint, and confirm the DAG is intact.

cub-scout uses "proof" in the sense of **tamper-evidence of the attestation** — not in the strong sense of formal verification or zero-knowledge cryptography. The fingerprint shows that the receipt has not been edited since it was stamped; it does not, by itself, prove the underlying claim is *true* — it proves the issuer's claim has not been altered.

Useful framing:

> **A receipt without verifiable proof is a claim. A receipt with verifiable proof is evidence.**

Validating the proof for a saved receipt:

```bash
cub-scout receipt validate path/to/receipt.json
# exit 0 → fingerprint matches; receipt is intact
# exit 1 → fingerprint mismatch; receipt has been tampered with
# exit 2 → I/O error
```

---

## The broader vocabulary

"Receipt" and "proof" sit alongside five other terms that get reached for in similar territory. cub-scout's surface intentionally covers each of them at a different resolution, and being clear about which term applies to which surface helps avoid muddled vocabulary in handover docs and integrator code.

| Term | Focus | cub-scout analog | Surface |
|---|---|---|---|
| **Log** | Time and sequence — an append-only chronological stream of events | Watch event stream | `cub-scout watch` |
| **Journal** | The first, unaltered record of a transaction with full raw detail | Per-field diff trace | `cub-scout trace deploy/x -n y` |
| **Record** | A complete, structured unit of data about a single entity or event | A single in-toto Statement (one receipt file) | `cub-scout receipt verify --save` |
| **Ledger** | A master system-of-record that aggregates and categorizes — current state, not raw history | Aggregate receipt over a scope | `cub-scout receipt verify --scope namespace/<ns>` |
| **Provenance** | Verifiable origin and chain of custody across the lifecycle | Chained-receipt DAG | `cub-scout receipt verify --input-attestation <upstream>` |

The receipt sits at the **record** layer of this vocabulary, with attestation semantics added — a record-with-proof, hand-offable to a third party.

- The **chain** (provenance) is built *from* receipts; each link inherits the receipt's proof property.
- The **aggregate** (ledger) is built *from* receipts; the synthesized verdict inherits the proof property of the receipts it summarizes.
- The **log** and **journal** sit *below* the receipt: lower-resolution surfaces that are not stamped by default. A watch event can be promoted to a receipt via `watch --emit-receipt-on`.

---

## Where the terms blur honestly

Some of these distinctions matter more in marketing copy than in code. The honest edges:

- **Receipt vs. attestation.** in-toto and SLSA call the artifact an "attestation"; the inner JSON we call a `predicate` is literally the in-toto `predicate` field. We chose "receipt" because it reads to a non-supply-chain audience without explanation. The trade is some industry-specificity for clarity — the two words refer to the same artifact in our case.

- **Chain vs. provenance.** A cub-scout chain is provenance-*shaped* but discrete — a DAG of stamped checkpoints, not a continuous lineage of every field across every transformation. If you need that breadth, reach for OpenLineage or OpenTelemetry traces. cub-scout's chain is "evidence at gates," not "everything that ever happened."

- **Aggregate vs. ledger.** A cub-scout aggregate is a single receipt synthesized from N input receipts via a policy (current: `max-severity`). It is ledger-shaped — rolled-up state over a category — but it is a snapshot, not a running balance. Running balances are built by composing chains of aggregates.

- **Proof, weak sense vs. strong sense.** Already noted above: we mean tamper-evidence, not formal proof. The strong sense — "this verdict is mathematically correct" — would require an independent verifier to re-run the underlying observation against the live cluster. The receipt format carries enough state to support that re-verification, but the receipt itself does not perform it.

- **Record vs. receipt.** Every receipt is a record; not every record is a receipt. A row in a `scan` output is a record. A row that has been stamped via `receipt verify` with a fingerprint over canonical JSON is a receipt.

---

## A single worked example

You verify a deploy. cub-scout produces, at increasing resolution:

```
1. watch event              → log entry        (time + event type + subject)
2. trace --format json      → journal entry    (per-field intent vs observed, raw)
3. receipt verify           → record + proof   (in-toto Statement, fingerprint stamped)
4. --input-attestation prev → provenance link  (this receipt references prior by digest)
5. --scope namespace/prod   → ledger row       (synthesized aggregate over the scope)
```

The proof property is what lets you hand 3-5 to a third party — an auditor, a paranoid reviewer, an AI agent running a year later — and have them independently confirm: this verdict was produced by this binary, on this subject, at this time, and nothing has been edited since. They do not have to trust your storage; they recompute the fingerprint, and the answer is the same or it is not.

---

## Short answer for a slide

- **Receipt** = a stamped, hand-offable record of one verification, in a standard envelope (in-toto v1).
- **Proof** = the verifiable property of a receipt: recompute the fingerprint, compare to the stamped value.
- **Chain** = receipts linked by digest → provenance.
- **Aggregate** = receipts synthesized by policy → ledger.
- **Log / journal** = lower-resolution surfaces (watch events / trace output); receipts are what get stamped at gates.

---

## See also

- [docs/howto/receipts-end-to-end.md](../howto/receipts-end-to-end.md) — operational lifecycle (pre-deploy gate → audit chain → namespace aggregate → real-time emission → reading back)
- [docs/reference/json-contracts.md § Receipt Contract](../reference/json-contracts.md) — wire format, fingerprint algorithm, predicate schemas
- [docs/reference/watch-events.md](../reference/watch-events.md) — event taxonomy and `--emit-receipt-on` semantics
- [examples/receipts/](../../examples/receipts/) — four worked example directories with paste-ready CI snippets
- [skills/scout-verify/SKILL.md](../../skills/scout-verify/SKILL.md) — the Verify verb group for AI-agent loaders
