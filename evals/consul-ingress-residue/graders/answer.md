---
type: regex
pattern: '^(?=[\s\S]*"receipt_outcome"\s*:\s*"WATCH")(?=[\s\S]*"workload_outcome"\s*:\s*"PASS")(?=[\s\S]*"child_sync"\s*:\s*"SYNCED")(?=[\s\S]*"child_health"\s*:\s*"PROGRESSING")(?=[\s\S]*"residual_identity"\s*:\s*"Ingress/consul/consul-consul-ui")(?=[\s\S]*"residual_sync"\s*:\s*"SYNCED")(?=[\s\S]*"residual_health"\s*:\s*"PROGRESSING")(?=[\s\S]*"residual_cause"\s*:\s*"UNKNOWN")(?=[\s\S]*"observation_scope"\s*:\s*"HISTORICAL_SEQUENTIAL_NONATOMIC_NOT_CURRENT_NOT_FULL_K8S_SNAPSHOT")[\s\r\n]*\{\s*(?:"(?:receipt_outcome|workload_outcome|child_sync|child_health|residual_identity|residual_sync|residual_health|residual_cause|observation_scope)"\s*:\s*"[^"\\]*"\s*,\s*){8}"(?:receipt_outcome|workload_outcome|child_sync|child_health|residual_identity|residual_sync|residual_health|residual_cause|observation_scope)"\s*:\s*"[^"\\]*"\s*\}[\s\r\n]*$'
flags: s
target: last_message
---
