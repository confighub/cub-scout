type: regex
pattern: '^\s*\{\s*"context"\s*:\s*"alpha"\s*,\s*"source_url"\s*:\s*"https://alpha\.example\.invalid/team/repo\.git"\s*,\s*"source_revision"\s*:\s*"alpha-branch"\s*,\s*"revision_role"\s*:\s*"DECLARED_TARGET"\s*,\s*"events_coverage"\s*:\s*"INCOMPLETE"\s*,\s*"denied_context"\s*:\s*"denied"\s*,\s*"denied_inventory"\s*:\s*"UNKNOWN"\s*,\s*"stable_cluster_identity"\s*:\s*"NOT_PROVEN"\s*,\s*"dollar_savings"\s*:\s*"NOT_PROVEN"\s*\}\s*$'
target: last_message
flags: s
