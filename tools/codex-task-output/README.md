# codex-task-output

Strict JSON Schema + TypeScript contract for Codex task handoff payloads.

## Files

- `codex-task-output.schema.json`: Draft 2020-12 schema
- `types.ts`: discriminated union task output types
- `validate.ts`: Ajv2020 validator helper
- `examples/`: schema-valid sample payloads

## Usage

```ts
import { assertCodexTaskOutput } from "./validate";

const parsed = JSON.parse(rawJson);
const output = assertCodexTaskOutput(parsed);

// output is typed as CodexTaskOutput after validation
console.log(output.status);
```
