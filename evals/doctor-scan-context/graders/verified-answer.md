type: regex
pattern: '^\s*\{\s*"allowed_doctor_context"\s*:\s*"doctor\-allowed"\s*,\s*"allowed_scan_context"\s*:\s*"doctor\-allowed"\s*,\s*"allowed_observed_resources"\s*:\s*"3"\s*,\s*"denied_doctor_context"\s*:\s*"doctor\-denied"\s*,\s*"denied_scan_context"\s*:\s*"doctor\-denied"\s*,\s*"denied_inventory"\s*:\s*"UNKNOWN"\s*,\s*"denied_scan_coverage"\s*:\s*"INCOMPLETE"\s*,\s*"denied_healthy"\s*:\s*"NOT_PROVEN"\s*,\s*"stable_cluster_identity"\s*:\s*"NOT_PROVEN"\s*,\s*"dollar_savings"\s*:\s*"NOT_PROVEN"\s*\}\s*$'
target: last_message
flags: s
