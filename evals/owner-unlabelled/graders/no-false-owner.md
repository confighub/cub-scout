---
type: regex
pattern: '^OWNER:[ \t]*(?!(Flux|ArgoCD|Argo|Helm|ConfigHub)\b)\S+'
flags: im
target: last_message
---
