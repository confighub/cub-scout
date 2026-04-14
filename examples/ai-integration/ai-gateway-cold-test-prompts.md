# AI Gateway Cold-Test Prompts

Use this prompt set to compare shell-first operation versus MCP-enabled operation for `cub scout`.

Run the exact same prompts in two fresh sessions:

1. shell-first
2. MCP-enabled (`cub scout mcp serve`; local repo command: `./cub-scout mcp serve`)

Score the results using:

- [AI Gateway Value Test](../../docs/reference/ai-gateway-value-test.md)

## Rules

1. Use the same model for both runs.
2. Use the same cluster and connected-mode state for both runs.
3. Do not rescue the run unless it is clearly drifting or blocked.
4. Record the first tool chosen before any human correction.

## Standalone Core Set

These prompts should work in standalone mode with cluster access.

| Prompt | Expected first tool | Proof surface |
|---|---|---|
| "What's wrong with cubbychat in prod?" | `doctor` | `doctor` JSON or a justified chain into `explain` |
| "kubectl cannot reach the cluster after restart. Is the cluster broken or is my access broken?" | `doctor` | `doctor` JSON or a justified chain into access diagnosis |
| "What's running in the `prod` namespace?" | `map` | `map list --json` |
| "Who owns `deployment/frontend` in `prod`?" | `explain` | `explain --format json` |
| "Where did `deployment/frontend` in `prod` come from?" | `trace` | `trace --format json` |
| "Should I use cub scout or kubectl here?" | `doctor` | `doctor` plus a tool-choice explanation grounded in cub scout facts |

## Connected Set

These prompts require connected mode.

| Prompt | Expected first tool | Proof surface |
|---|---|---|
| "Compare governed state to live state for `deployment/frontend` in `prod`." | `compare_three_way` | `compare three-way --format json` |
| "Do ConfigHub, the deployer, and the cluster agree for `deployment/frontend` in `prod`?" | `compare_three_way` | `summary.agreement` |
| "Would you sign off on this change for `deployment/frontend` in `prod`?" | `compare_three_way` | `compare three-way --format json` plus convergence/sign-off evidence |
| "Which ConfigHub unit corresponds to `deployment/frontend` in `prod`?" | `confighub_units` after scope is known | `cub unit list --json` |
| "What is the first useful ConfigHub object I should open for `deployment/frontend` in `prod`?" | `confighub_units` after scope is known | `cub unit list --json` |
| "Show me the exact governed unit details once you find the unit for `deployment/frontend`." | `confighub_unit_get` after unit slug is known | `cub unit get --json` |
| "What changed recently for this governed unit?" | `confighub_changesets` | `cub changeset list --json` |

## Stretch Set

Use these when you want to test chaining quality, not just first-tool attraction.

| Prompt | Expected chain |
|---|---|
| "Figure out what is wrong with `frontend` and tell me where I should look next." | `doctor` -> `explain` |
| "I know `frontend` is Argo-managed. Show me where it came from and whether it has converged." | `trace` -> `compare_three_way` |
| "Tell me whether this is sign-off-ready and show me the first useful governed object to inspect." | `compare_three_way` -> `confighub_units` |
| "Find the governed unit for `frontend` and show me the latest governed receipt." | `explain` or `trace` -> `confighub_units` -> `confighub_changesets` |

## Score Sheet Template

Copy this table for each run.

| Prompt | First tool | Correct first tool? | Tool hops to proof | Rescue needed? | Proof surface reached | Stopped correctly? |
|---|---|---|---|---|---|---|
| Prompt 1 |  |  |  |  |  |  |
| Prompt 2 |  |  |  |  |  |  |
| Prompt 3 |  |  |  |  |  |  |
| Prompt 4 |  |  |  |  |  |  |
| Prompt 5 |  |  |  |  |  |  |

## What To Watch For

Good MCP behavior:

1. fewer invented flags and shell forms
2. faster use of `doctor` for cold-start diagnosis
3. direct use of `compare_three_way` for governed-vs-live questions
4. less confusion between cluster inventory and governed inventory

Bad behavior:

1. opening connected tools before cluster-side scope is known
2. answering convergence questions from live-only evidence
3. using `map` or `scan` as the first answer to "what's wrong?"
4. continuing to wander after exact proof has already been reached
