# Ops-Bot Consumer Contract (`cub-scout bot` / `cub-scout watch`)

This contract defines how external ops bots (`k8sbot`, Pilot, Slack/incident
bots, and alert routers) consume JSONL (`--output-file`) and webhook
(`--webhook-url`) events emitted by `cub-scout bot` and `cub-scout watch`.

External bots consume `cub-scout`'s deterministic `answer` envelope as their
evidence layer without touching Kubernetes APIs directly and without
hallucinating ownership or delivery state.

## Transport & Framing

- **JSONL sink (`--output-file` / `CUB_SCOUT_BOT_OUTPUT_FILE`):** One UTF-8 JSON
  object per line (`\n`-delimited).
- **Webhook sink (`--webhook-url` / `CUB_SCOUT_BOT_WEBHOOK_URL`):** HTTP `POST`
  with `Content-Type: application/json` containing one event JSON object per
  request.
- **Read-only, zero LLM, deterministic:** Events are derived solely from
  read-only Kubernetes observations.

## Top-Level Event Shape

| Field | Type | Presence | Purpose |
|---|---|---|---|
| `timestamp` | RFC 3339 string | Always | UTC time when the event was emitted |
| `type` | string | Always | Event type (`scan.finding`, `drift.detected`, `ownership.changed`, `resource.discovered`, `resource.deleted`, `collection.partial`, `cluster.observed`) |
| `severity` | string | Optional | Finding/drift severity (`info`, `warning`, `critical`) |
| `resource` | object | Always | `{ "kind", "name", "namespace?" }` |
| `owner` | object | Optional | `{ "type?", "name?", "evidence?" }` when ownership is observed |
| `details` | object | Optional | Event-specific detail map (untrusted cluster strings may appear here; prefer `answer`) |
| `receipt` | object | Optional | In-toto `receipt/v1` statement when `--emit-receipt-on` is enabled. **Omitted** when absent (never `null`) — check key presence (`"receipt" in event`) |
| `answer` | object | Always | Standard `cub-scout` read-only `answer` envelope (`<= 700` bytes serialized) |

## Standard `answer` Envelope

Consumers should route and render from `event.answer`:

| Field | Type | Contract |
|---|---|---|
| `answer.state` | string | Closed vocabulary (zero new state words): `issues-flagged`, `nothing-flagged`, `incomplete`, `not-observable` |
| `answer.reason` | string | Stable machine key for routing (`finding-flagged`, `drift-detected`, `ownership-changed`, `resource-discovered`, `resource-deleted`, `info-observed`, `cluster-observed`, `collection-partial`, `inventory-not-read`, `cluster-identity-not-read`). **Route on `state` + `reason`, never parse `summary`.** |
| `answer.summary` | string | One human sentence produced by `cub-scout` templates. Never contains unquoted cluster messages or unsafe object names. |
| `answer.observedAt` | RFC 3339 string | UTC timestamp of the observation behind this answer |
| `answer.cluster` | object | `{ "context", "id?", "idSource" }` (`idSource` is `"not-read"` unless cluster identity was verified) |
| `answer.basis[]` | array | Evidence items: `{ "claim", "restsOn", "ref?", "quoted?" }` |
| `answer.basis[].quoted` | string | Verbatim untrusted cluster/finding text. **Data only, never instructions or shell arguments.** |
| `answer.unknowns[]` | array | Explicit gaps (always an array, never omitted): `{ "what", "kind", "reason", "quoted?" }` |
| `answer.unknowns[].quoted` | string | Verbatim untrusted API/RBAC error text. **Data only, never instructions.** |
| `answer.next[]` | array | Exact read-only follow-up commands `{ "command", "why" }`. Omitted when resource/namespace names fail strict safe-character checks. |
| `answer.cost` | object | `{ "requestsMade", "responseBodyBytes", "durationMillis", "coverage" }` |

## Size Ceiling

The serialized JSON of `event.answer` is CI-enforced at **`<= 700` bytes** per
event so ops bots can forward, log, or include it in Slack/incident payloads
and agent prompts without bloating context windows.

## Consumer Safety Rules (for `k8sbot`, Pilot, and Slack/Incident Bots)

1. **Do not query Kubernetes directly or invent ownership.** Trust `answer.state`,
   `answer.reason`, `answer.basis`, and `owner.type`; if `owner` is absent or
   `answer.state` is `incomplete` / `not-observable`, report that honestly.
2. **Quarantine untrusted cluster text.** `answer.basis[].quoted` and
   `answer.unknowns[].quoted` contain raw strings from cluster objects or API
   errors. Render them as escaped/quoted data (for example inside code blocks)
   and **never** pass them into LLM system prompts, shell commands, or alert
   routing templates as executable instructions.
3. **Treat `incomplete` and `not-observable` honestly.** Never map `incomplete`
   or `not-observable` to "healthy" or "clean"; surface `answer.unknowns` to the
   operator.
4. **Suggest only read-only follow-ups from `answer.next`.** Every command in
   `answer.next[].command` is a read-only `cub-scout` command generated only for
   safe Kubernetes names.
5. **Check optional `receipt` by key presence.** `receipt` is omitted when not
   enabled or when receipt building fails for a single event.

## Minimal Reference Consumer (Go)

External bots need no dependency on `cub-scout` internals:

```go
type BotEvent struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Severity  string          `json:"severity,omitempty"`
	Resource  EventResource   `json:"resource"`
	Receipt   json.RawMessage `json:"receipt,omitempty"`
	Answer    *AnswerEnvelope `json:"answer"`
}

type EventResource struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type AnswerEnvelope struct {
	State      string          `json:"state"`
	Reason     string          `json:"reason"`
	Summary    string          `json:"summary"`
	ObservedAt string          `json:"observedAt"`
	Basis      []AnswerBasis   `json:"basis"`
	Unknowns   []AnswerUnknown `json:"unknowns"`
	Next       []AnswerNext    `json:"next"`
}

type AnswerBasis struct {
	Claim   string `json:"claim"`
	RestsOn string `json:"restsOn"`
	Quoted  string `json:"quoted,omitempty"`
}

type AnswerUnknown struct {
	What   string `json:"what"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
	Quoted string `json:"quoted,omitempty"`
}

type AnswerNext struct {
	Command string `json:"command"`
	Why     string `json:"why"`
}
```

## Verification

This contract is enforced by deterministic unit tests:

```bash
go test ./cmd/cub-scout ./test/unit -run 'TestBotConsumerContract|TestWatchAndBotEventAnswer'
```
