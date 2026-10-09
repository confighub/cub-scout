---
type: regex
pattern: '^SUSPENDED:[ \t]*(?=[^\n]*HelmRelease\/team\-h\/redis(?![\w-]))(?=[^\n]*Kustomization\/flux\-system\/legacy\-migration(?![\w-]))(?=[^\n]*Kustomization\/flux\-system\/monitoring(?![\w-]))(?![^\n]*infrastructure(?![\w-]))(?![^\n]*flux\-system\/apps(?![\w-]))(?![^\n]*tenants(?![\w-]))(?![^\n]*secrets(?![\w-]))(?![^\n]*postgres(?![\w-]))(?![^\n]*grafana(?![\w-]))(?![^\n]*loki(?![\w-]))[^\n]+$'
flags: im
target: last_message
---
