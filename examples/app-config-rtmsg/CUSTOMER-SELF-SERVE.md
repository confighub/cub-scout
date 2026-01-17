# Customer Self-Serve Model

This is the key differentiator for enterprise customers like a real-time messaging platform's clients.

## The Problem Today

a platform's enterprise customers need to configure their slice of the platform:
- Rate limits for their tier
- Feature flags for their use case
- Webhook URLs for their integrations
- Custom domains for their brand

**Today:** Customer files a support ticket → RT-Message-App engineer makes change → hours/days later

**With ConfigHub:** Customer logs in → edits their config → change is live (with audit)

## How It Works

### 1. Customer Gets a Space

```yaml
kind: AppSpace
metadata:
  name: customer-acme
spec:
  owner: platform-admin@acme.com
  type: customer
```

### 2. Space Defines What They Can Edit

```yaml
editable_fields:
  # Customer controls
  - path: config.rate_limit
  - path: config.feature_flags.*
  - path: config.custom_domain
  - path: config.webhooks.*
  - path: config.message_retention_days

readonly_fields:
  # Platform controls
  - config.image_tags
  - config.service_endpoints
  - config.internal_settings
```

### 3. Customer Edits via CLI or UI

```bash
# Customer runs this (in their Space context)
cub unit edit acme-realtime-config \
  --set config.rate_limit.messages_per_second=100000 \
  --reason "Holiday traffic increase"

# Or via UI: customer sees editable fields, makes changes, submits
```

### 4. Change is Audited

```yaml
audit:
  modified_by: devops@acme.com
  modified_at: "2025-12-20T16:45:00Z"
  reason: "Holiday traffic increase"
  changes:
    - field: rate_limit.messages_per_second
      old: 50000
      new: 100000
```

### 5. Both Parties See History

**ACME sees:**
- Their change history
- What they edited vs platform defaults
- Effective config (merged view)

**RT-Message-App sees:**
- All customer changes across all customers
- Anomaly detection ("customer X increased rate limit 10x")
- Billing reconciliation ("customer X uses enterprise features")

## The Value Chain

```
┌─────────────────────────────────────────────────────────────────┐
│                    BEFORE (Support Ticket)                       │
│                                                                  │
│  Customer → Ticket → RT-Message-App Engineer → DynamoDB → Done            │
│  Time: hours to days                                            │
│  Audit: ticket number (if you can find it)                      │
│  Visibility: none for customer                                   │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                    AFTER (Self-Serve)                            │
│                                                                  │
│  Customer → ConfigHub → Audit → Done                            │
│  Time: seconds                                                   │
│  Audit: full who/what/when/why                                  │
│  Visibility: customer sees their config + history                │
└─────────────────────────────────────────────────────────────────┘
```

## Constraints & Safety

Customers can't break things because:

1. **Field-level permissions** — can only edit allowed fields
2. **Value constraints** — rate_limit.max can't exceed 1M
3. **Schema validation** — custom_domain must be valid hostname
4. **Inheritance** — platform fields always come from upstream
5. **Audit** — all changes logged, reversible

## Enterprise Features

For larger customers:

### Approval Workflows
```yaml
# Big changes need approval
policies:
  - match: { change_size: large }
    require:
      - approval-from: [platform-admin@acme.com]
```

### Change Windows
```yaml
# Only allow changes during business hours
policies:
  - match: { environment: production }
    allow:
      - time_window: "Mon-Fri 09:00-17:00 UTC"
```

### Role-Based Access
```yaml
members:
  - email: admin@acme.com
    role: admin           # Can edit + approve
  - email: devops@acme.com
    role: editor          # Can edit, needs approval
  - email: support@acme.com
    role: viewer          # Read-only
```

## Why This Matters

1. **Faster for customers** — no support tickets for routine changes
2. **Cheaper for platform** — fewer support tickets, less engineering time
3. **Better audit** — every change tracked, both parties see history
4. **Safer** — constraints prevent misconfigurations
5. **Scalable** — works for 1 customer or 1000 customers

**This is the product RT-Message-App should be selling to their enterprise customers.**
