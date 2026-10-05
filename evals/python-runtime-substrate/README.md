# Python substrate execution checkpoint

The [report](report.json) complements the earlier static image inventory. It
does not admit the full benchmark runtime or change frozen cases, prompts,
evidence, grants, budgets or graders. No model or provider run occurred.

The cached, separately verified OCI index/platform/layers run Linux arm64 Python
3.11.15. Executed interpreter and libpython bytes match the static inventory's
SHA-256 values. Eleven standard-library imports, in-memory SQLite and one Python
child process pass. The final invocation uses `python3 -I -S`; its explicit
read-only library path loads PyYAML 6.0.3 and its compiled extension, and parses
the authored YAML control. That is the existing frozen grader requirement,
not a requirements change. The wheel's byte count and SHA-256 match the retained
[official PyPI release metadata source](https://pypi.org/pypi/PyYAML/6.0.3/json).

Four attempts are retained. The first refused a noncanonical mount path before
Python execution; the guard was preserved and the staging path corrected. The
second passed standard-library execution but found the required PyYAML missing.
The third verified the staged dependency with normal Python startup. The fourth
also verified isolated/no-site startup. Every owned container was verified absent
after cleanup; wheel files were unchanged after execution.

The existing reviewed Docker helper supplies limits and ownership checks: local
explicit context, cached image/no pulls, no network, read-only root, UID/GID 65534,
dropped capabilities, no-new-privileges, 64 PIDs, 1 GiB, one CPU, a read-only tools
mount and private 64 MiB tmpfs. Raw execution and pre/post inspection outputs are
retained unedited under `evidence/`; file hashes are in the report. The one-off
harness source and full originals remain at the report's local paths. This is a
diagnostic checkpoint, not the portable final launch implementation.

Selected-case/overlay mounts, Claude and current Scout assets, official grading,
hooks, tool grants, full descendant enforcement and reconciled cost accounting
still require runtime admission. One observed Python child is not that proof.
