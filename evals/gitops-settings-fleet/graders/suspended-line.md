---
type: regex
pattern: '^[ \t>*`_-]*SUSPENDED:[ \t`*_]*(?=[^\n]*(?<![\w\/-])HelmRelease\/team\-a\/postgres(?![\w-]))(?=[^\n]*(?<![\w\/-])HelmRelease\/team\-e\/postgres(?![\w-]))(?=[^\n]*(?<![\w\/-])HelmRelease\/team\-e\/vault(?![\w-]))(?=[^\n]*(?<![\w\/-])Kustomization\/team\-b\/infrastructure(?![\w-]))(?=[^\n]*(?<![\w\/-])Kustomization\/team\-f\/tenants(?![\w-]))(?:[^,\n]+,){4}[^,\n]+$'
flags: im
target: last_message
---
