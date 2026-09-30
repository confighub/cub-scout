---
type: regex
pattern: '^\s*\{\s*"checkout"\s*:\s*\{\s*"owner"\s*:\s*"Flux"\s*,\s*"source"\s*:\s*"label:kustomize\.toolkit\.fluxcd\.io/name"\s*\}\s*,\s*"api"\s*:\s*\{\s*"owner"\s*:\s*"Native"\s*,\s*"meaning"\s*:\s*"no supported marker was observed on this returned object; this is not an orphan determination"\s*\}\s*,\s*"collection"\s*:\s*\{\s*"status"\s*:\s*"partial"\s*,\s*"omission"\s*:\s*\{\s*"apiVersion"\s*:\s*"apps/v1"\s*,\s*"resource"\s*:\s*"deployments"\s*,\s*"namespace"\s*:\s*"restricted"\s*,\s*"reason"\s*:\s*"forbidden"\s*\}\s*\}\s*,\s*"contract"\s*:\s*"opt-in compact envelope; default \[\]Entry JSON unchanged"\s*\}\s*$'
flags: s
target: last_message
