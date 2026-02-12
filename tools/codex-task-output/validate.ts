import Ajv2020 from "ajv/dist/2020";
import type { CodexTaskOutput } from "./types";
import schema from "./codex-task-output.schema.json";

const ajv = new Ajv2020({ allErrors: true, strict: true });
const validateCodexTaskOutput = ajv.compile<CodexTaskOutput>(schema);

export function assertCodexTaskOutput(input: unknown): CodexTaskOutput {
  if (validateCodexTaskOutput(input)) {
    return input;
  }

  throw new Error(
    ajv.errorsText(validateCodexTaskOutput.errors, { separator: "\n" })
  );
}
