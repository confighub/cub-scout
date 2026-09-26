---
type: regex
pattern: '^UNMANAGED:(?![^\n]*\b(inventory|checkout|cart|payments-api)\b)[^\n]*\S'
flags: im
target: last_message
---
