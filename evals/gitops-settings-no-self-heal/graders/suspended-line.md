---
type: regex
pattern: '^[ \t>*`_-]*SUSPENDED:[ \t`*_]*(?=[^\n]*(?<![\w\/-])HelmRelease\/team\-h\/redis(?![\w-]))(?=[^\n]*(?<![\w\/-])Kustomization\/flux\-system\/legacy\-migration(?![\w-]))(?=[^\n]*(?<![\w\/-])Kustomization\/flux\-system\/monitoring(?![\w-]))(?:[^,\n]+,){2}[^,\n]+$'
flags: im
target: last_message
---
