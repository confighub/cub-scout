---
name: map-ownership-evidence-contract
description: "Summarize the offline captured compact map ownership diagnostics without turning omissions or missing markers into orphan claims."
expected_outcome: "Report the Flux detector source, bound no-marker statement for the returned API object, partial forbidden list omission and unchanged-default JSON contract."
tags: [product-contract, ownership-evidence]
max_turns: 8
timeout_seconds: 300
allowed_tools: [Read, Glob, Grep, Skill]
---

Read `evidence/map-ownership-evidence.json`. It is an offline captured contract fixture, not a live cluster response. State the detected owner/source for `shop/checkout`, the meaning of the no-known-marker result for `shop/api`, and what the collection omission says. Do not claim that either returned object is orphaned, that the API is globally unsupported, or that an omitted list has no resources. Mention that the diagnostics envelope is opt-in and the default `[]Entry` JSON shape is unchanged.

Return exactly one JSON object with keys in this order: `checkout`, `api`, `collection`, `contract`. Values:
- `checkout`: object with `owner` and canonical `source` copied from the fixture.
- `api`: object with `owner` and statement that no supported marker was observed on this returned object, not an orphan determination.
- `collection`: object with `status` and the omitted API version/resource/namespace/reason.
- `contract`: the string `opt-in compact envelope; default []Entry JSON unchanged`.
