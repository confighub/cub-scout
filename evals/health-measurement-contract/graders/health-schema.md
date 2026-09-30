---
type: regex
pattern: '^\{"health":"Unavailable","healthMeasurement":\{"status":"unmeasured","scope":"controller-chain"(?:,"reason":"[^"]+")?\},"currentChange":\{"verdict":"PASS"\}\}$'
flags: s
target: last_message
---
