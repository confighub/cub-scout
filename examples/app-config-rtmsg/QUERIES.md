# Example Queries

These queries demonstrate the power of the Map — cross-cutting visibility that DynamoDB/Consul can't provide.

## Environment Queries

```bash
# All production configs
cub query "environment=production"
# Returns: production-blows, production-cn, production-drill, acme-realtime-config

# All dev environments
cub query "environment=dev OR environment=nonprod"
# Returns: dev-alice, dev-bob, nonprod-realtime-matth, nonprod-staging

# Critical tier only
cub query "tier=critical"
# Returns: production-blows, production-cn, production-drill
```

## Service Queries

```bash
# All realtime service configs (all environments)
cub query "service=realtime"
# Returns: all realtime units across prod/nonprod/dev

# Realtime in production only
cub query "service=realtime AND environment=production"
# Returns: production-blows, production-cn, etc.
```

## Customer Queries

```bash
# All configs for customer ACME
cub query "customer=acme"
# Returns: acme-realtime-config

# All customer-facing configs
cub query "customer-facing=true"
# Returns: all production units + customer units

# All enterprise tier customers
cub query "tier=enterprise"
# Returns: acme-realtime-config, (other enterprise customers)
```

## Change Queries

```bash
# What changed today?
cub query "modified>today"

# What changed this week in production?
cub query "environment=production AND modified>7d"

# Who changed critical tier configs?
cub query "tier=critical AND modified>30d" --show-audit

# Changes by a specific person
cub query "audit.modified_by=devops@acme.com"
```

## Drift Queries

```bash
# Configs that drifted from upstream
cub query "drift=true"

# Customer configs that differ from platform defaults
cub query "customer=* AND upstream_diff=true"

# Show what ACME changed vs platform default
cub diff acme-realtime-config --upstream
```

## Field-Specific Queries

```bash
# All configs with reactor enabled
cub query "config.feature_flags.enable_reactor=true"

# High rate limit configs
cub query "config.rate_limit.messages_per_second>50000"

# Configs with custom domains
cub query "config.custom_domain!=null"

# Configs with webhooks configured
cub query "config.webhooks.on_error!=null"
```

## Platform Operations

```bash
# Which configs use old image version?
cub query "config.image_tags.core=prod-20251215*"

# Configs that need image update
cub query "config.image_tags.core!=prod-20251220*"

# Cluster size audit
cub query "config.internal_settings.cluster_size<3 AND environment=production"
# Should return nothing (all prod should have cluster_size >= 3)
```

## Customer Self-Serve Visibility

```bash
# What can ACME see?
cub query --space customer-acme
# Returns only: acme-realtime-config

# What did ACME change recently?
cub query --space customer-acme "modified>7d" --show-audit

# Diff between ACME's config and platform default
cub diff acme-realtime-config production-blows
```

## The Power: Impossible with DynamoDB/Consul

These queries are trivial with ConfigHub but require custom code with DynamoDB:

1. **"Show all configs for customer X"** — DynamoDB requires knowing which tables/keys
2. **"What changed this week across all environments"** — DynamoDB has no cross-table queries
3. **"Which configs use old image version"** — DynamoDB can't query by nested field value
4. **"Diff customer config vs platform default"** — DynamoDB has no inheritance model

**ConfigHub's Map makes config queryable like a database, not a key-value store.**
