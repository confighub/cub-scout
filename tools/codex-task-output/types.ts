export type SchemaVersion = `${number}.${number}.${number}`;
export type Status = "completed" | "completed_no_changes" | "blocked";
export type ChangeType = "added" | "modified" | "deleted" | "renamed";
export type FindingSeverity = "info" | "warning" | "error";

export type Tests = {
  passed: number;
  failed: number;
  skipped: number;
};

export type Change = {
  file: string;
  type: ChangeType;
  highlights: [string, ...string[]];
};

export type Finding = {
  message: string;
  severity?: FindingSeverity;
  file?: string;
};

export type Blocker = {
  type: string;
  details: string;
};

export type BaseTaskOutput = {
  schema_version: SchemaVersion;
  task_id: string;
  summary: string;
  tests: Tests;
  partial_progress?: string[];
  commands_run?: string[];
  next_steps?: string[];
};

export type CompletedTaskOutput = BaseTaskOutput & {
  status: "completed";
  changes: [Change, ...Change[]];
  findings?: never;
  blockers?: never;
  requested_input?: never;
};

export type CompletedNoChangesTaskOutput = BaseTaskOutput & {
  status: "completed_no_changes";
  findings: Finding[];
  changes?: never;
  blockers?: never;
  requested_input?: never;
};

export type BlockedTaskOutput = BaseTaskOutput & {
  status: "blocked";
  blockers: [Blocker, ...Blocker[]];
  requested_input: [string, ...string[]];
  changes?: never;
  findings?: never;
};

export type CodexTaskOutput =
  | CompletedTaskOutput
  | CompletedNoChangesTaskOutput
  | BlockedTaskOutput;
