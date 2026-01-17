# App Config Example: Real-Time Messaging Style

This example shows how ConfigHub can manage application configuration in the style of a platform's infrastructure — without Kubernetes, GitOps, or container orchestration.

## The Scenario

A platform company (like a real-time messaging platform) has:
- **60+ environments** across dev, nonprod, and production
- **Multiple services** (realtime, health-server, frontdoor)
- **Per-region config** with overrides
- **Enterprise customers** who need visibility AND self-serve config

## The Real Architecture (from Ably)

*Based on email exchange with Matt Hammond, January 2026*

Their current system:
```
CLI tool → DynamoDB (config + app versions) → S3 (fallback, replicated at write)
                         ↓
           Nodes poll every 1 minute (mechanism baked into AMIs)
                         ↓
           Update config AND/OR deploy new app versions
```

**Key characteristics:**
- **DynamoDB stores config + versions** — both app settings and which versions to deploy
- **1-minute polling** — nodes check DynamoDB every minute, not push-based
- **Config triggers deployments** — a config change can initiate new version deployment
- **AMI-baked** — the config pull mechanism is in the AMI itself
- **S3 fallback** — config replicated to S3 at write time for resilience
- **Terraform is vanilla** — TF does infrastructure only, doesn't manage config

**What this architecture lacks** (gaps ConfigHub fills):
- No audit trail (who changed what, when, why)
- No cross-cutting queries ("show all production configs")
- No customer visibility (customers can't see their slice)
- No customer self-serve (customers can't edit their own rate limits)
- No inheritance (each environment is a full copy, no templates)
- No approval workflows (just y/n confirmation in CLI)

## What ConfigHub Adds

| Before (DynamoDB + CLI) | After (ConfigHub) |
|-------------------------|-------------------|
| No audit trail | Who changed what, when, why |
| No customer visibility | Customers see their config slice |
| No self-serve | Customers edit their own values |
| No approval workflow | ChangeSets for production |
| No cross-cutting queries | "Show all production configs" |

## Structure

```
hub.yaml                    # Catalog: templates + constraints
spaces/
  realtime-team.yaml        # Internal team space
  customer-acme.yaml        # Customer self-serve space
units/
  templates/
    realtime-service.yaml   # Base template (in Hub)
  instances/
    production-blows.yaml   # Production instance (cloned from template)
    nonprod-matth.yaml      # Dev instance
  customer/
    acme-realtime.yaml      # Customer's config (inherits + overrides)
```

## Key Concepts Demonstrated

### 1. Hub as Catalog

The Hub holds **base templates** that teams clone:

```yaml
# hub.yaml
kind: Hub
metadata:
  name: rtmsg-platform
spec:
  templates:
    - realtime-service      # Teams clone this
    - health-server
  constraints:
    - name: production-requires-approval
      match: { labels: { environment: production } }
      require: changeset-approval
```

### 2. Units with Labels

Every config entity is a Unit with queryable labels:

```yaml
kind: Unit
metadata:
  name: production-blows
  labels:
    service: realtime
    environment: production
    region: blows
    tier: critical
```

### 3. Customer Self-Serve

Customers get a Space where they can edit specific fields:

```yaml
kind: AppSpace
metadata:
  name: customer-acme
spec:
  # Customer can edit these Units
  units:
    - acme-realtime

  # But only these fields
  editable_fields:
    - config.rate_limit
    - config.feature_flags.*
    - config.custom_domain

  # Everything else inherits from upstream
  upstream: realtime-team/production-blows
```

### 4. Cross-Cutting Queries

```bash
# All production configs
cub query "environment=production"

# All configs for customer ACME
cub query "customer=acme"

# Critical tier that changed this week
cub query "tier=critical AND modified>7d"

# Realtime service across all environments
cub query "service=realtime"
```

## Try It

```bash
# Run the TUI demo
./demo.sh

# See the hub catalog
cat hub.yaml

# See a production config
cat units/instances/production-blows.yaml

# See customer self-serve config
cat units/customer/acme-realtime.yaml

# See what customer ACME can edit
cat spaces/customer-acme.yaml
```

## TUI Demo

Run `./demo.sh` to see this mockup with terminal colors:

**Color Legend:**
- 🟢 **Green** = Production / Healthy / Editable values
- 🟡 **Yellow** = Non-prod / Staging
- 🔵 **Cyan** = Internal team
- 🟣 **Magenta** = Customer / Self-serve
- ⚫ **Dim** = Dev / Read-only / Inherited

```
╭────────────────────────────────────────────────────────────────────────╮
│  ⚡ APP CONFIG: RTMSG EXAMPLE                                          │
╰────────────────────────────────────────────────────────────────────────╯

This demo shows how ConfigHub manages app config (not K8s).
Modeled after a platform's DynamoDB-backed configuration system.

HUB
┌────────────────────────────────────────────────────────────────────────┐
│  rtmsg-platform                                                        │
└────────────────────────────────────────────────────────────────────────┘

  Templates                     Constraints
  ─────────────────────────     ─────────────────────────────────────
  • realtime-service            • production-requires-approval
  • health-server               • critical-tier-restricted
  • frontdoor                   • customer-config-audit

APP SPACES
┌────────────────────────────────────────────────────────────────────────┐
│  2 App Spaces                                                              │
└────────────────────────────────────────────────────────────────────────┘

  🔵 realtime-team (internal)       🟣 customer-acme (self-serve)
  ───────────────────────────────     ───────────────────────────────
  Owner: realtime@rtmsg.io            Owner: platform-admin@acme.com
  Units: 9                           Units: 1

  🟢 ✓ production-blows               🟣 ✓ acme-realtime-config
  🟢 ✓ production-cn                        └── inherits: production-blows
  🟢 ✓ production-drill
  🟡 ○ nonprod-realtime-matth
  🟡 ○ dev-alice

UNITS
┌────────────────────────────────────────────────────────────────────────┐
│  by environment                                                        │
└────────────────────────────────────────────────────────────────────────┘

  ENVIRONMENT          UNIT                      SERVICE         TIER         REVISION
  ─────────────────────────────────────────────────────────────────────────────────
🟢 production           production-blows          realtime        critical     20251223.3
🟢 production           production-cn             realtime        critical     20251223.1
🟢 production           production-drill          realtime        critical     20251222.5
🟣 production           acme-realtime-config      realtime        enterprise   20251220.2
🟡 nonprod              nonprod-realtime-matth    realtime        dev          20251223.1
🟡 nonprod              nonprod-staging           realtime        staging      20251222.8
⚫ dev                  dev-alice                 realtime        dev          20251223.2
⚫ dev                  dev-bob                   realtime        dev          20251223.1

CUSTOMER VIEW
┌────────────────────────────────────────────────────────────────────────┐
│  acme-realtime-config                                                  │
└────────────────────────────────────────────────────────────────────────┘

Customer ACME sees only their config. They can edit highlighted fields.

  🟣 ╔══════════════════════════════════════════════════════════════╗
  🟣 ║ acme-realtime-config                                         ║
  🟣 ║ Upstream: production-blows │ Revision: 20251220.2            ║
  🟣 ╠══════════════════════════════════════════════════════════════╣
  🟣 ║ EDITABLE BY CUSTOMER                                         ║
  🟣 ║                                                              ║
  🟣 ║ rate_limit:                                                  ║
  🟢 ║   messages_per_second: 100000      ← 2x default              ║  🟢 editable
  🟢 ║   connections_per_channel: 200000                            ║  🟢 editable
  🟣 ║                                                              ║
  🟣 ║ feature_flags:                                               ║
  🟢 ║   enable_reactor: true             ← enabled                 ║  🟢 editable
  🟢 ║   enable_firehose: true            ← enabled                 ║  🟢 editable
  🟣 ║                                                              ║
  🟢 ║ custom_domain: realtime.acme.com                             ║  🟢 editable
  🟢 ║ message_retention_days: 14                                   ║  🟢 editable
  🟣 ║                                                              ║
  🟣 ║ webhooks:                                                    ║
  🟢 ║   on_message: https://hooks.acme.com/rtmsg/message           ║  🟢 editable
  🟢 ║   on_error: https://hooks.acme.com/rtmsg/error               ║  🟢 editable
  🟣 ╠══════════════════════════════════════════════════════════════╣
  ⚫ ║ INHERITED FROM PLATFORM (read-only)                          ║  ⚫ dim/locked
  ⚫ ║                                                              ║
  ⚫ ║ image_tags:                                                  ║
  ⚫ ║   core: prod-20251220.1-a1b2c3d                              ║
  ⚫ ║   frontdoor: prod-20251218.2-e4f5g6h                         ║
  ⚫ ║                                                              ║
  ⚫ ║ service_endpoints:                                           ║
  ⚫ ║   api: https://api.rtmsg.io                                  ║
  ⚫ ║   realtime: wss://realtime-blows.rtmsg.io                    ║
  ⚫ ║                                                              ║
  ⚫ ║ internal_settings:                                           ║
  ⚫ ║   cluster_size: 12                                           ║
  🟣 ╚══════════════════════════════════════════════════════════════╝

AUDIT
┌────────────────────────────────────────────────────────────────────────┐
│  recent changes                                                        │
└────────────────────────────────────────────────────────────────────────┘

  DATE         USER                      UNIT                      CHANGE
  ─────────────────────────────────────────────────────────────────────────────────
     Dec 23       alice@rtmsg.io            production-blows          Increased cluster size
🟣   Dec 20       devops@acme.com           acme-realtime-config      Increased rate limits
     Dec 18       bob@rtmsg.io              production-cn             Updated frontdoor image
🟣   Dec 15       admin@acme.com            acme-realtime-config      Added error webhook
🟡   Dec 15       matt@rtmsg.io             nonprod-matth             Testing new build

QUERIES
┌────────────────────────────────────────────────────────────────────────┐
│  cross-cutting visibility                                              │
└────────────────────────────────────────────────────────────────────────┘

Examples of queries that DynamoDB can't do:

🔵 cub query "environment=production"
⚫ → production-blows, production-cn, production-drill, acme-realtime-config

🔵 cub query "customer=acme"
⚫ → acme-realtime-config

🔵 cub query "config.rate_limit.messages_per_second>50000"
⚫ → acme-realtime-config (100000)

🔵 cub query "modified>7d AND tier=critical"
⚫ → production-blows (cluster size change)


  ┌────────────────────────────────────────────────────────────────────┐
  │ WHAT THIS DEMO SHOWS                                               │
  │                                                                    │
  │ 1. Hub as catalog      - Templates + constraints in one place     │
  │ 2. App Spaces as boundaries - Internal team vs customer self-serve    │
  │ 3. Units with labels   - Queryable across all environments        │
  │ 4. Customer self-serve - ACME edits their slice, platform rest    │
  │ 5. Audit trail         - Who changed what, when, why              │
  │ 6. Cross-cutting queries - Visibility DynamoDB can't provide      │
  └────────────────────────────────────────────────────────────────────┘
```

### What the Demo Shows

1. **HUB** — `rtmsg-platform` catalog with templates + constraints
2. **APP SPACES** — Internal team (cyan) vs customer self-serve (magenta)
3. **UNITS** — Color-coded by environment (green=prod, yellow=nonprod, dim=dev)
4. **CUSTOMER VIEW** — Editable fields (green) vs inherited (dim)
5. **AUDIT** — Who changed what, with customer changes highlighted
6. **QUERIES** — Cross-cutting examples DynamoDB can't do

## Future Additions

- **DynamoDB as Source**: Read existing config from DynamoDB, govern via ConfigHub
- **Spegel for distribution**: P2P config distribution to nodes via OCI
- **Triggers**: Auto-propagate template changes to all instances

## Mapping to Ably's Actual System

| Ably Concept | ConfigHub Equivalent |
|--------------|---------------------|
| `ably-env config show <env>` | `cub unit get <unit>` |
| `ably-env config set-service` | `cub unit edit` + ChangeSets |
| Environment (production-blows) | Unit with labels |
| Config version (20251223.2) | Revision (automatic) |
| DynamoDB table | ConfigHub is the store (or DynamoDB as Source for migration) |
| S3 fallback | OCI export + Spegel (future) |
| 1-minute polling from AMIs | Nodes pull OCI artifacts (future) |
| Config triggers deployment | ConfigHub Actions/Triggers (future) |

## How ConfigHub Replaces/Complements This

**Option 1: ConfigHub as the store (recommended for new deployments)**
```
cub CLI → ConfigHub → OCI Registry → Nodes pull config
```
DynamoDB not needed. ConfigHub stores config natively.

**Option 2: Migration path (for existing systems like Ably)**
```
Existing DynamoDB → Import → ConfigHub (source of truth)
                              ↓
ConfigHub → Sync → DynamoDB (for legacy readers during transition)
                              ↓
ConfigHub → OCI → New consumers
```

**Option 3: Governance overlay (minimal change)**
```
ably-env → DynamoDB (still the store)
              ↓
ConfigHub observes via DynamoDB Streams → Adds audit, queries, visibility
```
