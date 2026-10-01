# Direct Claude CLI offline controls probe

Termination signals enter bounded cleanup; repeat signals are suppressed during
teardown and the original handlers are restored. Requests are snapshotted only
after the server and handlers join. Unexpected rejected or excess arrivals fail acceptance,
and the final snapshot is retained on validation failure. The synthetic request
pins the Haiku model label for reproducibility; no provider model runs and all
mock usage values are authored fixture data.

This is a diagnostic harness for direct Claude CLI runs against a synthetic
loopback provider. Adversarial scenarios described below are implemented but
have not been run. It does not contact a real provider, validate billing or
subscription credits, measure answer quality, account for the whole process
tree, or admit a benchmark result. It is not full Experiment A and does not
show treatment plugin or skill value. The
reported evaluation estimate remains `$4.38666035`; provider dollars, credits,
and local development costs remain separate unknowns.

Four actual CLI attempts are retained in the
[result report](../reports/2026-10-01-direct-cli-offline-probe.json).
The first failed because its TCP counter differed from accepted HTTP requests;
its extra connection remains unexplained. Two later diagnostic attempts retained
complete transport evidence and identified `HEAD /api/hello` as a startup request.
Both failed acceptance with HTTP 400. The final reviewed helper explicitly
declines that exact bodyless startup route with 404, counts it against the HTTP
limit, and accepts at most one such event. No model inference is provided there.

The fourth wiring attempt at source `27c6c84` passed in 0.674 seconds: three connections
(private preflight 204, startup decline 404, model POST 200), two parsed HTTP
requests excluding preflight, exactly one captured model request advertising
`Read`, expected successful terminal text, and verified cleanup. Earlier attempts
remain failed. This is minimal wiring proof only; it does not demonstrate the
Task/Agent refusal or turn-limit behavior in the new scenario modes.
Runtime execution requires the explicit `--execute` switch and is bound to the
reviewed absolute regular executable
`/opt/homebrew/Caskroom/claude-code/2.1.274/claude`, SHA-256
`3509913f9d1576316c8845b88837f8fd3bbbcf26625833ac82cfb6b8985da94a`, and
version pin `2.1.274`. The helper hashes the binary before use; it does not run
the CLI to print or verify its version. The fixed values are repeated as CLI
arguments for review, and arbitrary executable/hash/version combinations are
rejected.

The helper creates a fresh mode-0700 output directory and private
`CLAUDE_CONFIG_DIR`, then retains that private directory and its synthetic
contents in the output. It builds the child environment from scratch: there is
no `HOME` override or copied caller environment/configuration, credential
helper, or OAuth token. The only API credential is a literal fake key. The
actual init trace still advertised built-in skills/agents/commands; it showed
empty MCP/plugin lists and only `Read` in the model request tool inventory.
Do not claim the built-in skill inventory is empty. `--bare` is API-key-only according to the previously reviewed local
CLI help; no OAuth or subscription-credit behavior is inferred. Explicit
environment controls disable nonessential traffic and fast mode.

Before launching the CLI, macOS `sandbox-exec` must pass a synthetic socket
check against a one-time private preflight path on the designated localhost
port and deny a different localhost port and a documentation-only external IP
with `EPERM`. The preflight connection is excluded from the provider request
count and request capture. The CLI is then confined to that one loopback port.
Missing platform/profile support or failed checks stop the run. No host
firewall setting is changed.

The fixture accepts only POST `/v1/messages` (and the SDK beta query form),
and only the exact fake `x-api-key` or fake bearer header. It selects JSON or
SSE from the request body's boolean `stream` field. It separately declines one exact bodyless `HEAD /api/hello` with 404.
It rejects malformed lengths/JSON, unexpected methods/paths/auth, oversized bodies, and requests
past the four-HTTP-request bound. TCP connections have a separate limit of
eight, including the preflight connection, with at most four active handlers.
Counters saturate at limit+1 on overflow and fail acceptance. Exact zero-byte
EOFs are recorded as empty connections; partial, malformed, denied or timed-out
requests do not qualify. Every connection must have a unique retained event.
Preflight, declined startup and accepted-request events must reconcile with
parsed HTTP counts.
Request-line lengths/hashes and bounded method/query-free path diagnostics are
retained; headers and query values are not. Per-connection reads have a two-second deadline;
the overall run has a 90-second wall bound including reserved teardown. At
most four fixture workers run concurrently. Only accepted synthetic requests
are retained, with raw body bytes base64-encoded and decoded for inspection;
auth header values are never persisted. CLI output is captured up to 2 MiB
per stream, including partial output on timeout/overflow, and owned process
groups are terminated on all exits.

The default `wiring` scenario preserves the fixed prompt asking for exactly
`offline-probe-terminal`. Every scenario allows exactly `Read`, disallows
`Task,Agent`, sets an empty strict MCP config
and empty setting sources, disables session persistence, and requests stream
JSON. It passes `--max-turns 1` and `--max-budget-usd 0.05` as diagnostic
controls only; neither is claimed as a billing or whole-tree cap. A successful
classification requires exit code zero, exactly one successful terminal
result with the exact expected text, and every actual provider request to
advertise exactly `Read`. Empty, expanded, changed, or malformed inventories
fail. The `task-denial` and `agent-denial` modes are separate synthetic
adversarial probes. Each mock response includes one named, harmless Task or
Agent `tool_use`, despite that tool being absent from the Read-only request
inventory. Acceptance requires the exact authored input in the retained mock
response, the matching assistant request, and matching CLI output plus the
following provider request with a `tool_result` whose `is_error` is true and
whose content explicitly names that tool as disallowed/unavailable. Generic
errors, missing results, conflicting events, aliases, or inventory absence
alone fail. This is narrow refusal evidence for one synthetic mock response;
it does not establish absence of all child processes.

The `turn-limit` mode creates a private mode-0600 fixture and configures the
mock to return repeated `Read` tool calls only for that path. It requests
`--max-turns 1`. Acceptance requires a real `error_max_turns` terminal event,
an exact positive `num_turns`, at least one exact fixture Read tool-use and
successful result in CLI output, and request count at or below the bound. It
does not require an extra provider round after the turn cap has stopped one.
A timeout, transport limit, generic error, or missing terminal/result is not
turn-limit evidence. Task/Agent scenarios request two turns to allow the
refusal and terminal response.

Provenance, helper and executable hashes, redacted argv, timestamps, elapsed time,
request count, safe request bytes, stdout/stderr, exit/timeout status, and
cleanup status are retained in the private output directory, including on
failure where available. The helper does not overwrite an existing output
path or follow artifact symlinks.

Run pure offline checks without creating sockets or invoking Claude:

```sh
python3 -m unittest discover -s evals/direct-cli-controls -p 'test_*.py' -v
```

The already-run wiring command is documented here as historical reproduction:

```sh
python3 evals/direct-cli-controls/probe.py --execute \
  --cli /opt/homebrew/Caskroom/claude-code/2.1.274/claude \
  --expected-version 2.1.274 \
  --expected-sha256 3509913f9d1576316c8845b88837f8fd3bbbcf26625833ac82cfb6b8985da94a \
  --output /tmp/scout-direct-cli-mock-probe-reviewed
```

The three adversarial commands below are unrun and require independent review
before invocation. Use a fresh output directory for each run and the same CLI
pin shown above:

```sh
python3 evals/direct-cli-controls/probe.py --execute --scenario task-denial --cli /opt/homebrew/Caskroom/claude-code/2.1.274/claude --expected-version 2.1.274 --expected-sha256 3509913f9d1576316c8845b88837f8fd3bbbcf26625833ac82cfb6b8985da94a --output /tmp/scout-709-task-denial
python3 evals/direct-cli-controls/probe.py --execute --scenario agent-denial --cli /opt/homebrew/Caskroom/claude-code/2.1.274/claude --expected-version 2.1.274 --expected-sha256 3509913f9d1576316c8845b88837f8fd3bbbcf26625833ac82cfb6b8985da94a --output /tmp/scout-709-agent-denial
python3 evals/direct-cli-controls/probe.py --execute --scenario turn-limit --cli /opt/homebrew/Caskroom/claude-code/2.1.274/claude --expected-version 2.1.274 --expected-sha256 3509913f9d1576316c8845b88837f8fd3bbbcf26625833ac82cfb6b8985da94a --output /tmp/scout-709-turn-limit
```

These commands are instructions for later reviewed probes, not authorization
to run now. The first invocation used a separate dated output path and is retained as a
failed probe; the example path above is not an authorization to retry. Any
later comparison must retain the frozen
24-case protocol, equal ordinary-tool treatment, and full treatment
skills/MCP. Changing runners requires an explicit recorded protocol revision
before paid admission. `--safe-mode` must not silently strip treatment. No
paid retries, package installation, registry publication, auth changes, or
benchmark-baseline updates belong here.
