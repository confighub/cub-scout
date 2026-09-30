---
type: regex
pattern: '^\s*\{\s*"resource"\s*:\s*"Deployment/api"\s*,\s*"namespace"\s*:\s*"team-01"\s*,\s*"field"\s*:\s*\{\s*"path"\s*:\s*"\.spec\.template\.spec\.containers\[name=\\"api\\"\]\.image"\s*,\s*"cause"\s*:\s*"controller-drift"\s*,\s*"managers"\s*:\s*\[\s*"helm"\s*\]\s*\}\s*,\s*"absentField"\s*:\s*\{\s*"path"\s*:\s*"\.spec\.nonexistent"\s*,\s*"cause"\s*:\s*"unknown"\s*,\s*"reason"\s*:\s*"No decodable managedFields entry claims this exact path\."\s*\}\s*,\s*"humanActor"\s*:\s*"unknown"\s*,\s*"latestWriter"\s*:\s*"unknown"\s*,\s*"evidenceBoundary"\s*:\s*"recorded snapshot; sequential, not atomic; Helm-labelled fixture is not proof of Helm reconciliation"\s*\}\s*$'
flags: s
target: last_message
