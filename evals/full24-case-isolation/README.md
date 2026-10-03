# Selected-case container isolation diagnostic

This is the first full-24 runtime isolation component. It accepts one frozen
case and optionally its declared DEL-03/04 authored control. It runs **only a
fixed file-hash/write-denial probe**, serially in the two arms. It does not start
a model, provider, ordinary-tool dispatcher, MCP server or official evaluator.

Success is defined before implementation in
[#645 comment 5969082934](https://github.com/confighub/cub-scout/issues/645#issuecomment-5969082934).
The diagnostic reuses the previously reviewed cached image and container bounds
from `evals/combined-recorded-runtime/`. No image is pulled. Docker must be
available through an explicitly named context whose endpoint is a local Unix
socket. Each invocation owns unique names/labels and cleans up only those
containers after verifying ownership. Failed attempts are retained and stop the
pair; there is no automatic retry.

Each container has exactly one evidence bind: that arm's validated
`model-stage/` at `/tools`, read-only. The preparation tree, host receipts,
private oracles, sibling cases and other arm are never mounted. The probe is
provided as fixed inline Python code, without another host mount. The image
supplies Python and its ordinary operating-system files; isolation refers to
host evidence, not an empty filesystem.

The profile has no network, a read-only root, UID/GID 65534, dropped capabilities,
no new privileges, 64 PIDs, 1 GiB memory, one CPU and a private 64 MiB `/tmp`.
Actual Docker inspection must match the profile and exact command before start
and after zero terminal exit. The probe hashes every staged file, reports its
byte count and user, requires only loopback, and tests write denial at the
evidence mount, through a symlink in `/tmp`, and at the root filesystem. Its
complete inventory must equal the source-verified stage receipt. Host source
and stage integrity are verified before and after the probe. Passing both arms
also requires equal common files and the exact declared treatment delta.

## Reproduce

Prepare a fresh source package as described in
[`full24-pair-preflight`](../full24-pair-preflight/README.md), then run:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 evals/full24-case-isolation/run_pair.py \
  --source-prep /private/tmp/cub-scout-full24-preflight \
  --case INV-01 --out /private/tmp/cub-scout-isolation-inv01 \
  --docker /usr/local/bin/docker --docker-context orbstack
```

The output parent must already exist under resolved `/tmp` or `/var/tmp`, and
the output directory must be new. Do not use a symlink or source-overlapping
path. Use your existing local context's name explicitly. To test an authored
control, use `--case DEL-03 --control
del03-missing-stale-identity-input-control.v1` (on the same command line).

The pair has a 180-second execution deadline, bounded individual Docker calls
and a separate 30-second cleanup reserve for each owned container. Output is
private and retains bounded stdout/stderr, argv, hashes, per-arm receipts and
the final `receipt.json`. The stage receipts keep their original source-only
claims; the separate runtime receipt describes this probe, not model admission.

Offline CI guards use authored Docker responses and do not run containers:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover \
  -s evals/full24-case-isolation -v
```

## Remaining gates

The ordinary-tool grants are preserved but not enforced by a model runtime.
Treatment MCP remains the inert marker; no skills were actually loaded by an
agent. This diagnostic does not establish official grader integration,
adversarial model containment, descendant/tool accounting, billing, answer
quality, cost savings or paid-run admission. It does not contact Kubernetes,
ConfigHub or an external provider. Those gates remain separate from successful
case-mount and fixed-probe evidence.

## Local proof — 2026-10-03

At source `5ff09940`, the INV-01 ordinary case and DEL-03 authored control
both passed in both arms on the first container attempt: four actual owned
containers, all removed with absence verified. See [proof summary](local-proof.json).
The ten offline tests cover all 24 ordinary cases and both authored controls
using fake Docker responses. They do not establish actual container execution
for the other cases. Three initial failed test runs are retained with their
setup/assertion explanations, alongside source preparation and successful logs.
No model, provider, MCP or official grader was executed.

## Full matrix follow-up — 2026-10-03

The same fixed probe subsequently passed every ordinary case and both authored
controls in both arms: 26 pairs, 52 unique containers, all verified absent.
The initial two pairs were reused; the remaining 24 pairs passed first attempt.
The independently audited [matrix proof](matrix-proof.json) binds all receipts,
raw output and stage hashes. This expanded proof is a local follow-up checkpoint
beyond the inspected #767 CI head. It supersedes the earlier sample's actual
coverage limit, while preserving all model/tool/MCP/grader/accounting/admission
limits. Serial timings are diagnostic timings, not a randomised benchmark.
