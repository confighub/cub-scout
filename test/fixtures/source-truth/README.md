# Source-truth producer fixtures

Producer-side fixture suite for the source-truth evidence contract
(#393). Each subdirectory is a single black-box test of `agent.Derive`
against a hand-authored set of surfaces and a declared strategy.

Pairs with the consumer-side fixtures at
`confighubai/confighub-ai-demo#264` — every fixture here will get a
matching acceptance verdict + Charlie-readable receipt on that side.

## Layout

```
test/fixtures/source-truth/
├── README.md                               (this file)
├── <NN>-<verdict>-<scenario>/
│   ├── strategy.txt    declared strategy enum value (empty == ASK)
│   ├── surfaces.json   the SourceTruthSurfaces input
│   └── expected.json   exact byte-equal SourceTruthEvidence output
```

## What this v0.1 covers (6 fixtures)

| # | Strategy | Verdict | Council case |
|---|---|---|---|
| 01 | `confighub-oci-flux` | `PASS` / `AGREED` | happy path |
| 02 | `confighub-oci-argo` | `BLOCK` / `MISMATCH` outlier=controller | strategy violation (Argo reads Git under OCI strategy) — the council's primary trap |
| 03 | `git-argo` | `PASS` / `AGREED` | strategy-relativity proof — same observation as 02 but correct under vanilla GitOps |
| 04 | `confighub-oci-flux` | `WATCH` / `INCOMPLETE` | strict missing-proof rule (no controller revision/digest) |
| 05 | `confighub-oci-flux` | `BLOCK` / `BLOCKED` | fetch failure (ConfigHub surface absent) |
| 06 | (empty) | `ASK` / `UNKNOWN` | empty strategy short-circuits to ASK |

## Adding a fixture

1. Create a new directory `<NN>-<verdict>-<short-description>/`. The
   `<NN>-` prefix sorts deterministic.
2. Drop `strategy.txt` (one line, the strategy enum value, or empty
   for ASK), `surfaces.json` (the `SourceTruthSurfaces` input), and an
   empty `expected.json` placeholder.
3. Run `go test ./pkg/agent/ -run TestSourceTruthFixtures` — the test
   will fail and print the captured `--- got ---` JSON.
4. Copy the captured JSON into `expected.json`. Re-run; should pass.

## Why files on disk (not Go literals)

The byte-exact JSON shape is the artifact pilot's acceptance kernel
forks. Keeping fixtures as files makes that pairing reviewable
across the cub-scout and confighub-ai-demo repositories — diff one,
diff the other.

## Out of scope for v0.1

- Process-contract fixtures from #395 issue body (Flux suspended,
  ApplicationSet child still Git-sourced, empty wet unit, etc.).
  Coverage matrix in #395 lists the full set.
- The ArgoCDOCI Helm-source shape trap (cross-repo dep
  `confighubai/confighub#4356`). Will land alongside the symptom
  classifier in cub-scout's `compare source-truth` once #4356 is
  fixed upstream.
- Live-collector e2e fixtures. v0.1 tests the pure decision logic via
  `agent.Derive`. Live-collector black-box fixtures depend on
  fake-clientset + mocked `cub view get` + mocked tracer — substantial
  follow-up.
