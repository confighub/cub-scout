## Compare Resource

- Resource: `Deployment/checkout`
- Namespace: `prod`
- Mode: `dry-wet-live`
- Connection: `connected`

### DRY (unit intent)

| Field | Value |
|---|---|
| apiVersion | `apps/v1` |
| kind | `Deployment` |
| name | `checkout` |
| namespace | `prod` |
| replicas | 1 |
| images | `ghcr.io/acme/checkout:v1` |

### WET (rendered target)

| Field | Value |
|---|---|
| apiVersion | `apps/v1` |
| kind | `Deployment` |
| name | `checkout` |
| namespace | `prod` |
| replicas | 2 |
| images | `ghcr.io/acme/checkout:v2` |

### LIVE (cluster)

| Field | Value |
|---|---|
| apiVersion | `apps/v1` |
| kind | `Deployment` |
| name | `checkout` |
| namespace | `prod` |
| replicas | 3 |
| images | `ghcr.io/acme/checkout:v3` |


### Mismatches

| Field | DRY | WET | LIVE |
|---|---|---|---|
| replicas | `1` | `2` | `3` |
| images | `ghcr.io/acme/checkout:v1` | `ghcr.io/acme/checkout:v2` | `ghcr.io/acme/checkout:v3` |


### Notes

- Connected DRY/WET snapshots loaded for linked unit checkout.
