type: regex
pattern: '^\s*\{\s*"denied_context"\s*:\s*"rul03-denied"\s*,\s*"denied_http_status"\s*:\s*"403"\s*,\s*"denied_list_result"\s*:\s*"FORBIDDEN"\s*,\s*"denied_inventory"\s*:\s*"UNKNOWN"\s*,\s*"readable_context"\s*:\s*"rul03-readable"\s*,\s*"readable_list_count"\s*:\s*"1"\s*,\s*"deployment_namespace"\s*:\s*"rul03-proof"\s*,\s*"deployment_name"\s*:\s*"rul03-probe"\s*,\s*"deployment_uid"\s*:\s*"0b1a3bbe-dd77-42d4-96c6-f0577aee7710"\s*,\s*"default_context_before"\s*:\s*"rul03-readable"\s*,\s*"default_context_after"\s*:\s*"rul03-readable"\s*,\s*"observation_scope"\s*:\s*"SEQUENTIAL_NON_ATOMIC"\s*,\s*"evidence"\s*:\s*"capture-scope\.json\+observer-context-map\.json\+rul03-denied-deployments\.body\+rul03-readable-deployments\.body"\s*\}\s*$'
target: last_message
flags: s
