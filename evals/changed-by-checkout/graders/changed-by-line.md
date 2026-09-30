---
type: regex
pattern: '^(?:CHANGED_BY: MANUAL_TOOL \| MANAGER: kubectl-set \| FIELD_PATH: spec\.template\.spec\.containers\[name=checkout\]\.image \| HUMAN_ACTOR: UNKNOWN \| SCOPE: recorded evidence only; no live confirmation; no Git desired state provided|\x60CHANGED_BY: MANUAL_TOOL \| MANAGER: kubectl-set \| FIELD_PATH: spec\.template\.spec\.containers\[name=checkout\]\.image \| HUMAN_ACTOR: UNKNOWN \| SCOPE: recorded evidence only; no live confirmation; no Git desired state provided\x60)$'
flags: i
target: last_message
---
