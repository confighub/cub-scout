# Delivery identity answer-grader controls

## Success criteria

The offline control test succeeds only when all four selected full24 delivery
graders are the validated, positive-weight (`1`) `regex` graders from the
prepared oracle; their configured target is `last_message`; and the canonical
answer vectors match both the frozen regex and facts in the prepared source
inputs. For these listed controls, it compares Python and the locally installed
Node.js regex engine using the selected pattern and flags. This sample parity
does not establish equivalence for every possible answer or the official
evaluator. Each answer field receives a wrong-value negative; the suite also
rejects missing, duplicate, and extra fields, wrong types, partial JSON, and
prose. Whitespace variants are accepted only where the selected patterns
declare whitespace flexibility.

The DEL-03 and DEL-04 authored-control acceptance vectors are checked against
their validated source-only control packages and are labeled synthetic,
non-weighted, and not run. Their answers must not pass the original selected
grader.

Run only this packet from the repository root:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s evals/full24-pair-preflight -p 'test_delivery_graders.py'
```

Node.js is required for the dialect parity check. If it is unavailable, the
test fails with an installation-free diagnostic; it does not download or
install Node.

## Source binding checked by the tests

| Case | Expected answer facts checked against prepared inputs |
| --- | --- |
| DEL-01 | The exact ready-apps digest and source revision are extracted from their recorded log lines. Evidence kind and non-snapshot scope are checked against the pinned excerpt metadata. |
| DEL-02 | The excerpt states that the published release remained unread after 90 seconds, the prior release remained in use, and a hard refresh re-resolved `latest`; it supplies no exact digest or replica count. |
| DEL-03 | The target and shortened revision come from the status excerpt; inference basis comes from its documented behavior; receipt cluster, approved-revision match, and deployed HelmRelease come from the separate delivery receipt. No field binds that receipt to a runtime Sveltos digest or atomic cross-artifact observation. |
| DEL-04 | Package, rendering, release, consumer, image-reference, hook-policy, and observation-time fields are read from their respective pinned intent and receipts. Missing runtime image identity, current state, independent bundle verification, hook execution, and policy execution remain `UNKNOWN` where the fixtures do not prove them. |

## Limits

These controls execute only local Python/Node regex matching against synthetic
answer strings. They do not execute the official plugin grader or model, and
do not prove answer quality, evidence discovery, tool use, runtime access,
ordinary-tool-grant enforcement, token accounting, billing, or paid benchmark
admission. The authored controls are source-derived negative input vectors,
not observations or weighted benchmark cases. Other full24 grader families
remain outside this packet.
