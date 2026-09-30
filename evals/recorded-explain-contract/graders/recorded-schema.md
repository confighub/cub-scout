---
type: regex
pattern: '^\s*\{\s*"owner"\s*:\s*"Flux"\s*,\s*"health"\s*:\s*"Current"\s*,\s*"healthMeasurement"\s*:\s*\{\s*"status"\s*:\s*"measured"\s*,\s*"scope"\s*:\s*"object-local-readiness"\s*\}\s*,\s*"mutationCause"\s*:\s*"manual-edit"\s*,\s*"mutationManager"\s*:\s*"kubectl-set"\s*,\s*"fieldPath"\s*:\s*"\.spec\.template\.spec\.containers\[name=\\"checkout\\"\]\.image"\s*,\s*"recordedInputSha256"\s*:\s*"305614fa67327ba3ff6bea85c3c23f5ba9b57db181155b1c62af25d6f882eca8"\s*,\s*"resourceRead"\s*:\s*null\s*\}\s*$'
flags: s
target: last_message
