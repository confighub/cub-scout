---
name: operator-incident-evidence
description: 'Use when the user is GATHERING EVIDENCE FOR A POSTMORTEM / INCIDENT REPORT after the incident is past peak. Natural phrasing: "build the evidence package for last night''s incident", "I need to write the postmortem — what does cub-scout have?", "gather all the data we have about the prod outage", "what changed on payments-api between 14:00 and 16:00?", "produce an audit trail for the rollback decision", "save the evidence before the cluster state moves on". Composes `trace` + `compare three-way` + `bundle` + `history` + `audit` + `receipt verify --save` for a STRUCTURED, IMMUTABLE evidence package. Read-only by design — postmortems should NOT mutate state. Do NOT load for: pager-active triage (use triage-unhealthy-workload), single-resource drift question (use investigate-drift), or any mutating action (rollback / fix is a separate decision; this skill captures evidence, not actions).'
phase: cross-cutting
allowed-tools: Bash(./cub-scout trace *) Bash(cub-scout trace *) Bash(cub scout trace *) Bash(./cub-scout explain *) Bash(cub-scout explain *) Bash(cub scout explain *) Bash(./cub-scout compare three-way *) Bash(cub-scout compare three-way *) Bash(cub scout compare three-way *) Bash(./cub-scout compare drift *) Bash(cub-scout compare drift *) Bash(cub scout compare drift *) Bash(./cub-scout doctor *) Bash(cub-scout doctor *) Bash(cub scout doctor *) Bash(./cub-scout bundle *) Bash(cub-scout bundle *) Bash(cub scout bundle *) Bash(./cub-scout history *) Bash(cub-scout history *) Bash(cub scout history *) Bash(./cub-scout audit *) Bash(cub-scout audit *) Bash(cub scout audit *) Bash(./cub-scout receipt verify *) Bash(cub-scout receipt verify *) Bash(cub scout receipt verify *) Bash(./cub-scout receipt show *) Bash(cub-scout receipt show *) Bash(cub scout receipt show *) Bash(./cub-scout receipt list *) Bash(cub-scout receipt list *) Bash(cub scout receipt list *) Bash(./cub-scout receipt validate *) Bash(cub-scout receipt validate *) Bash(cub scout receipt validate *) Bash(./cub-scout snapshot *) Bash(cub-scout snapshot *) Bash(cub scout snapshot *) Bash(kubectl get *) Bash(kubectl describe *) Bash(kubectl get events *) Bash(kubectl logs *) Bash(cub * get) Bash(cub * list) Bash(cub history *)
---

# operator-incident-evidence

The postmortem-evidence loop. After the incident is past peak (or fully resolved), the operator needs to **assemble a structured evidence package** for the postmortem doc, the release-engineering meeting, or the audit ticket. This skill composes `trace` + `compare three-way` + `bundle` + `history` + `audit` + `receipt verify --save` into a deterministic, fingerprinted record.

Read-only by design. Postmortems should **never** mutate cluster state — capture, don't fix.

## When to use

Explicit phrasings:

- "Build the evidence package for last night's incident"
- "I need to write the postmortem — what does cub-scout have on payments-api?"
- "Gather all the data we have about the prod outage at 14:00"
- "What changed on payments-api between 14:00 and 16:00?"
- "Produce an audit trail for the rollback decision"
- "Save the evidence before the cluster state moves on"
- "Snapshot everything relevant to the outage"
- "Fingerprinted evidence for the SEV-2 ticket"

Implicit intents:

- The incident is **past peak** (the pager is no longer firing); time pressure has eased
- The user wants **structured, persistent** evidence — JSON snapshots, fingerprinted receipts, bundle artifacts
- The user is writing for an **audience** (the postmortem doc, the release meeting, the compliance audit), not just for themselves
- The user values **immutability** — the evidence shouldn't shift if someone fixes the cluster

## Do not load for

- Pager-active triage — [`triage-unhealthy-workload`](../triage-unhealthy-workload/SKILL.md)
- Single-resource drift question — [`investigate-drift`](../investigate-drift/SKILL.md)
- Migration planning — [`migrate-from-kubectl`](../migrate-from-kubectl/SKILL.md)
- Any mutating action. The rollback / fix decision is **separate** from the evidence capture — and cub-scout doesn't act on either.

## The loop

1. **Scope.** What resource(s) were involved? What time window? Single workload, namespace, or fleet?
2. **Capture the resource-level evidence.** `trace` + `explain` + `compare three-way` for each affected resource, with `--format json` so the output is machine-parseable.
3. **Capture the cluster-level snapshot.** `bundle` produces a debug-bundle archive that captures the cluster state at this moment for offline replay. `snapshot` produces a GSF JSON snapshot.
4. **Capture the connected-mode timeline.** `history` for the ConfigHub ChangeSet timeline; `audit list` for ConfigHub audit entries; `impact` for blast-radius.
5. **Generate fingerprinted receipts.** `receipt verify --save` for each affected resource, with the relevant predicate (`applied-matches-spec`, `source-truth-pass`, `no-manual-edits-since`). Receipts are **immutable**; they freeze the verdict at this moment.
6. **Validate later.** `receipt validate <path>` confirms the artifact wasn't tampered with between capture and review.

## Step-by-step

### Step 1 — scope

For the worked example: `payments-api` deployment broke between 14:00 and 16:00 UTC on 2026-05-22, recovered via Argo rollback at 16:15. Operator wants to capture evidence at 17:00 for the postmortem.

```bash
$ INCIDENT_START="2026-05-22T14:00:00Z"
$ INCIDENT_END="2026-05-22T16:15:00Z"
$ RESOURCES=("deploy/api" "deploy/worker")  # affected workloads
$ NAMESPACE="prod"
$ OUT_DIR="./incident-2026-05-22-payments"
$ mkdir -p $OUT_DIR
```

### Step 2 — resource-level evidence

```bash
# Per-resource explain + trace + three-way as JSON
$ for r in "${RESOURCES[@]}"; do
    safe=$(echo "$r" | tr / -)
    cub-scout explain "$r" -n $NAMESPACE --format json > "$OUT_DIR/${safe}-explain.json"
    cub-scout trace "$r" -n $NAMESPACE --format json > "$OUT_DIR/${safe}-trace.json"
    cub-scout compare three-way "$r" -n $NAMESPACE --format json > "$OUT_DIR/${safe}-compare.json"
  done
```

Each file is a structured contract — `explain` has the ownership + cause + nextSteps; `trace` has the ownership chain + git revision + secrets; `compare three-way` has the per-field DRY/WET/LIVE plus per-field attribution.

### Step 3 — cluster-level snapshot

```bash
# Bundle archive (offline-replayable)
$ cub-scout bundle summarize -n $NAMESPACE -o "$OUT_DIR/cluster.bundle.tar"

# Or GSF JSON snapshot (for diff'ing later)
$ cub-scout snapshot -n $NAMESPACE --format gsf > "$OUT_DIR/cluster-snapshot.gsf.json"
```

`bundle` archives are the canonical "this is what cub-scout saw" artifact. Replay with `cub-scout bundle inspect/replay/diff` against a different bundle (e.g., the pre-incident bundle from this morning's automated capture) to see what actually changed.

### Step 4 — connected timeline

If `cub auth status` reports connected:

```bash
# ChangeSet timeline for each unit
$ for r in "${RESOURCES[@]}"; do
    safe=$(echo "$r" | tr / -)
    cub-scout history "$r" -n $NAMESPACE --format json > "$OUT_DIR/${safe}-history.json"
  done

# Audit trail (read-only access to ConfigHub audit log)
$ cub-scout audit list --since $INCIDENT_START --until $INCIDENT_END --format json > "$OUT_DIR/audit-log.json"

# Blast-radius for the primary affected unit
$ cub-scout impact payments-api --format json > "$OUT_DIR/impact.json"
```

`history` is particularly valuable: ChangeSets show who applied what and when. If a ConfigHub apply at 13:55 (just before the incident) is the trigger, `history` makes it obvious.

### Step 5 — fingerprinted receipts

```bash
# applied-matches-spec — what was the controller saying right after recovery?
$ for r in "${RESOURCES[@]}"; do
    cub-scout receipt verify "$r" -n $NAMESPACE --predicate applied-matches-spec --save --save-dir "$OUT_DIR/receipts"
  done

# no-manual-edits-since the rollback completed — proves no human touched the resources during recovery
$ for r in "${RESOURCES[@]}"; do
    cub-scout receipt verify "$r" -n $NAMESPACE --predicate no-manual-edits-since --since "$INCIDENT_END" --save --save-dir "$OUT_DIR/receipts"
  done

# source-truth-pass if you want the strategy-typed verdict on whichever delivery chain was active
$ for r in "${RESOURCES[@]}"; do
    cub-scout receipt verify "$r" -n $NAMESPACE --strategy git-argo --save --save-dir "$OUT_DIR/receipts"
  done
```

Each receipt is a fingerprinted, immutable JSON artifact. SHA-256 over RFC 8785 canonical JSON of the full in-toto Statement v1 envelope. If someone questions the evidence later ("are you sure that's what the cluster looked like?"), `receipt validate <path>` confirms it.

See [`scout-verify`](../scout-verify/SKILL.md) for the receipt surface in detail.

### Step 6 — archive

```bash
$ ls "$OUT_DIR"
audit-log.json
cluster.bundle.tar
cluster-snapshot.gsf.json
deploy-api-compare.json
deploy-api-explain.json
deploy-api-history.json
deploy-api-trace.json
deploy-worker-compare.json
deploy-worker-explain.json
deploy-worker-history.json
deploy-worker-trace.json
impact.json
receipts/2026-05-22T17-00-00Z__applied-matches-spec__Deployment-api__abc123.receipt.json
receipts/2026-05-22T17-00-00Z__no-manual-edits-since__Deployment-api__def456.receipt.json
...

$ tar czf "incident-2026-05-22-payments.tar.gz" "$OUT_DIR"
$ # ship to S3 / a git LFS path / the SEV ticket
```

The archive is now the **postmortem source-of-truth**. Anyone reviewing the incident can extract it and run `cub-scout receipt validate` / `bundle inspect` against the artifacts to confirm authenticity.

## Boundaries with the triage skill

- [`triage-unhealthy-workload`](../triage-unhealthy-workload/SKILL.md) is the **30-second loop during the incident**. Pager firing, time-pressed, evidence is incidental.
- `operator-incident-evidence` is the **postmortem loop after the incident**. Time has eased; the goal is **complete, structured, immutable** capture. Different audience, different output shape.

Use both: triage first (during), evidence-capture second (after).

## Tool boundary

- **Allowed:** all the read verbs (trace / explain / compare / doctor / map); bundle (read-side); history / audit / impact (connected, read-side); receipt verify / show / validate / list / save; snapshot; kubectl get / describe / events / logs.
- **Not allowed:** any mutating action. The rollback / fix already happened (or is happening separately); this skill captures evidence about it, but does not perform it.

## Validation later

```bash
$ cub-scout receipt list --dir ./incident-2026-05-22-payments/receipts --format json | jq '.[] | {predicate: .predicateName, verdict: .verdict, scope: .scope.name}'

$ for r in ./incident-2026-05-22-payments/receipts/*.receipt.json; do
    cub-scout receipt validate "$r" || echo "TAMPERED: $r"
  done
```

`receipt validate` exits 0 if the fingerprint matches; 1 if tampered; 2 on I/O error. The exit-code shape makes it CI-friendly — wire into the postmortem-package-publish workflow.

## References

- [`scout-verify`](../scout-verify/SKILL.md) — receipt verbs
- [`scout-govern`](../scout-govern/SKILL.md) — bundle / history / audit / impact
- [`triage-unhealthy-workload`](../triage-unhealthy-workload/SKILL.md) — the during-incident counterpart
- [`investigate-drift`](../investigate-drift/SKILL.md) — for the per-resource attribution dimension
- Bundle format: [`docs/howto/bundle-diff.md`](../../docs/howto/bundle-diff.md), [`docs/howto/bundle-timeline.md`](../../docs/howto/bundle-timeline.md)
- Receipt contract: [`docs/reference/json-contracts.md`](../../docs/reference/json-contracts.md) § Receipt Contract
- Secret evidence on trace: `#328`, `#360`

## Constraints

- **Capture is not action.** This skill ends at "evidence archive." The rollback / fix / process-improvement is a separate decision in a separate context.
- For **maximum immutability**, archive the entire `incident-<date>` directory to off-cluster storage (S3, git LFS) the moment capture completes. The local store is `$XDG_DATA_HOME/cub-scout/receipts` by default but a local disk can be lost.
- Receipts in the archive use `--save-dir` to land in the incident folder rather than the operator's default local store. Keeps the incident package self-contained.
- If `audit list` or `history` are unavailable (standalone mode, ConfigHub auth lapsed), record that explicitly in the postmortem — missing connected-mode evidence is a known gap, not silent absence. The `omissions[]` field on the receipt covers the receipt side; the postmortem should call out the bundle/audit gaps independently.
