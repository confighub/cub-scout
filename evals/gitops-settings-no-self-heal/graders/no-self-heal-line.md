---
type: regex
pattern: '^NO_SELF_HEAL:[ \t]*(?=[^\n]*argocd\/etl\-nightly(?![\w-]))(?=[^\n]*argocd\/feature\-store(?![\w-]))(?=[^\n]*argocd\/fx\-rates(?![\w-]))(?=[^\n]*argocd\/ingress\-nginx(?![\w-]))(?=[^\n]*argocd\/settlement\-worker(?![\w-]))(?=[^\n]*team\-sandbox\/playground(?![\w-]))(?![^\n]*ledger\-api(?![\w-]))(?![^\n]*invoices(?![\w-]))(?![^\n]*checkout\-web(?![\w-]))(?![^\n]*legacy\-billing(?![\w-]))(?![^\n]*cert\-manager(?![\w-]))(?![^\n]*external\-dns(?![\w-]))(?![^\n]*monitoring\-stack(?![\w-]))(?![^\n]*cluster\-bootstrap(?![\w-]))(?![^\n]*dns\-migration(?![\w-]))(?![^\n]*warehouse(?![\w-]))(?![^\n]*stream\-processor(?![\w-]))(?![^\n]*notebooks(?![\w-]))(?![^\n]*backfill\-2025(?![\w-]))(?![^\n]*guestbook(?![\w-]))[^\n]+$'
flags: im
target: last_message
---
