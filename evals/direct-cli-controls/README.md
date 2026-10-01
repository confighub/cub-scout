# Direct Claude CLI offline controls probe

This directory contains a **diagnostic wiring probe** for checking direct CLI
controls against a synthetic loopback provider. It does not contact a real
provider, validate provider billing, prove model quality, or admit a benchmark
result. It is not full Experiment A and does not establish treatment plugin or
skill value; the fixture is deliberately minimal. The reported evaluation
estimate remains `$4.38666035`; provider dollars, subscription credits, and
local development cost are unknown and separate.

The probe has no implicit execution mode. It requires `--execute`, absolute
resolved executable path, a reviewed SHA-256 pin, and version `2.1.274`. Before
launching the CLI, it creates a fresh mode-0700 output/config directory,
starts one bounded loopback fixture server, and checks macOS `sandbox-exec`
allows its designated localhost port while denying a different localhost port
and a documentation-only external IP with `EPERM`. Missing platform support or
failed isolation checks stop the run. The CLI receives a fresh environment
containing a literal fake API key and mock base URL; no caller environment,
credentials, keychain, OAuth token, hooks, plugins, skills, or inherited config
are copied. `--bare` is API-key-only according to the reviewed CLI help; this
does not establish behavior for OAuth or subscription credits.

The fixed synthetic prompt requests one terminal response. The command pins
`Read` as the only allowed built-in tool, explicitly disallows `Task,Agent`,
uses an empty strict MCP config and empty setting sources, disables session
persistence, and requests stream JSON. It also passes `--max-turns 1` and
`--max-budget-usd 0.05` as diagnostic controls. Neither is treated as proof of
whole-process cost enforcement. The harness captures exact bounded mock
request bodies and observed tool inventory, CLI stdout/stderr, exit status,
timeouts, executable/runtime provenance, and hashes in a private output
directory. It rejects malformed or missing terminal results, duplicate
terminal records, request inventory drift, and an observed delegation tool.
It does not claim delegation denial from the inventory alone; no synthetic
task-call denial is exercised in this first probe.

Pure offline fixture tests:

```sh
python3 -m unittest discover -s evals/direct-cli-controls -p 'test_*.py' -v
```

No real CLI/model/provider request has been made by adding this helper. A later
actual invocation must first receive independent review of this implementation
and its exact argv, then use the reviewed resolved binary and pin, for example:

```sh
python3 evals/direct-cli-controls/probe.py --execute \
  --cli /opt/homebrew/Caskroom/claude-code/2.1.274/claude \
  --expected-version 2.1.274 \
  --expected-sha256 3509913f9d1576316c8845b88837f8fd3bbbcf26625833ac82cfb6b8985da94a \
  --output /tmp/scout-direct-cli-mock-probe-reviewed
```

That command is documented for review and has **not** been run. The eventual
real comparison must retain the frozen 24-case protocol, equal ordinary-tool
treatment, and full treatment skills/MCP. Changing runners requires an
explicit recorded protocol revision before any paid admission. `--safe-mode`
must not silently strip treatment. No paid retries, package installation,
registry publication, auth changes, or benchmark-baseline updates belong here.
