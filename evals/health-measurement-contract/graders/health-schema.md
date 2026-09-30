---
type: regex
pattern: '^\s*\{\s*"health"\s*:\s*"Unavailable"\s*,\s*"healthMeasurement"\s*:\s*\{\s*"status"\s*:\s*"unmeasured"\s*,\s*"scope"\s*:\s*"controller-chain"\s*\}\s*,\s*"currentChange"\s*:\s*\{\s*"verdict"\s*:\s*"PASS"\s*\}\s*\}\s*$'
flags: s
target: last_message
---
