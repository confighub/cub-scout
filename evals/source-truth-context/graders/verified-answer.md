type: regex
pattern: '^\s*\{\s*"selected_context"\s*:\s*"alpha"\s*,\s*"context_identity"\s*:\s*"LABEL_ONLY"\s*,\s*"confighub_status"\s*:\s*"PRESENT"\s*,\s*"controller_status"\s*:\s*"UNAVAILABLE"\s*,\s*"verdict"\s*:\s*"NOT_PASS"\s*,\s*"config_hub_selected_by_kube_context"\s*:\s*"NO"\s*\}\s*$'
target: last_message
flags: s
