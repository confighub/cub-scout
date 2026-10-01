---
type: regex
pattern: '^\s*\{\s*"status"\s*:\s*"changed"\s*,\s*"comparison"\s*:\s*"authored-fields-only"\s*,\s*"coverage"\s*:\s*"one-selected-object"\s*,\s*"resource"\s*:\s*\{\s*"apiVersion"\s*:\s*"apps/v1"\s*,\s*"kind"\s*:\s*"Deployment"\s*,\s*"namespace"\s*:\s*"shop"\s*,\s*"name"\s*:\s*"checkout"\s*,\s*"uid"\s*:\s*"synthetic-uid-alpha"\s*,\s*"resourceVersion"\s*:\s*"17"\s*\}\s*,\s*"live_read"\s*:\s*\{\s*"observed_at"\s*:\s*"2026-10-01T12:13:14Z"\s*,\s*"scope_discovery_reads"\s*:\s*0\s*,\s*"bounded_reader_discovery_reads"\s*:\s*1\s*,\s*"object_reads"\s*:\s*1\s*\}\s*,\s*"difference"\s*:\s*\{\s*"field"\s*:\s*"spec\.replicas"\s*,\s*"desired"\s*:\s*"2"\s*,\s*"live"\s*:\s*"1"\s*\}\s*,\s*"limits"\s*:\s*"local rendered input is not controller desired state; one object only; no reconciliation prediction"\s*\}\s*$'
flags: s
target: last_message
