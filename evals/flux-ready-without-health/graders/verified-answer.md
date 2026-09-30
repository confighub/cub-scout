---
type: regex
pattern: '^\s*\{\s*"ready_current_generation"\s*:\s*"YES"\s*,\s*"wait"\s*:\s*"FALSE"\s*,\s*"health_checks"\s*:\s*"ABSENT_OR_EMPTY"\s*,\s*"deployment_current_generation"\s*:\s*"UNAVAILABLE"\s*,\s*"uid_chain"\s*:\s*"deployment:3b56bff9-bfe2-4f1f-9913-1ebef756c0e8;replicaset:b7a593de-f7e9-417e-b5a9-1c71e1250f5d;pod:ee02c558-9a4a-489b-b1a6-e792c4b7f01b"\s*,\s*"source_revision"\s*:\s*"sha1:7732dde28be8cf8c42c096d94efbd8ce4a9d0a19"\s*,\s*"applied_revision"\s*:\s*"sha1:7732dde28be8cf8c42c096d94efbd8ce4a9d0a19"\s*,\s*"revision_binding"\s*:\s*"MATCH"\s*,\s*"ready_proves_workload_healthy"\s*:\s*"NO"\s*,\s*"observation_scope"\s*:\s*"SEQUENTIAL_NOT_ATOMIC"\s*,\s*"current_time_claim"\s*:\s*"CAPTURE_ONLY"\s*,\s*"application_level_check"\s*:\s*"NOT_RECORDED"\s*\}\s*$'
target: last_message
---
