# Custom Ownership Detectors (YAML)

Configure custom ownership detection without Go code.

## 1) Install detector config

```bash
mkdir -p ~/.cub-scout
cp examples/custom-ownership-detectors/detectors.yaml ~/.cub-scout/detectors.yaml
```

Optional override path:

```bash
export CUB_SCOUT_OWNERSHIP_DETECTORS="$(pwd)/examples/custom-ownership-detectors/detectors.yaml"
```

## 2) Label or annotate a resource

```bash
kubectl annotate deploy/my-app -n default pulumi.com/stack=platform/prod --overwrite
# or
kubectl label deploy/my-app -n default platform.company.com/managed-by=platform-controller --overwrite
```

## 3) Verify output

```bash
# Map shows configured owner name
./cub-scout map list -n default -q "name=my-app"

# Explain shows configured owner name in summary
./cub-scout explain deploy/my-app -n default

# Trace reports custom owner with unsupported-chain warning
./cub-scout trace deploy/my-app -n default
```

## Matching semantics

- Built-in detectors run first.
- Custom detectors run next, in file order.
- First matching custom detector wins.
- Invalid config prints one warning and falls back to built-ins.
