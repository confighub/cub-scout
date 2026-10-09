---
type: regex
pattern: '^[ \t>*`_-]*NO_SELF_HEAL:[ \t`*_]*(?=[^\n]*(?<![\w\/-])argocd\/etl\-nightly(?![\w-]))(?=[^\n]*(?<![\w\/-])argocd\/feature\-store(?![\w-]))(?=[^\n]*(?<![\w\/-])argocd\/fx\-rates(?![\w-]))(?=[^\n]*(?<![\w\/-])argocd\/ingress\-nginx(?![\w-]))(?=[^\n]*(?<![\w\/-])argocd\/playground(?![\w-]))(?=[^\n]*(?<![\w\/-])argocd\/settlement\-worker(?![\w-]))(?:[^,\n]+,){5}[^,\n]+$'
flags: im
target: last_message
---
