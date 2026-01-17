# RT-Message-App App Config: TODO

## What Works Today

- [x] Units from YAML (`cub unit create myunit config.yaml`)
- [x] Upstream/downstream inheritance (`--upstream-unit`)
- [x] Labels for querying (`--label environment=production`)
- [x] App Spaces for boundaries (`cub space create`)
- [x] Unit variants (effective config = merged upstream + overrides)
- [x] Revisions (automatic versioning)
- [x] Audit trail (who/what/when)
- [x] Functions for transforms
- [x] `push-upgrade` to propagate upstream changes

## To Figure Out (ConfigHub Core)

- [ ] **Field-level edit permissions** — How does ConfigHub control which fields a customer can edit? (Alexis: "can be controlled from within ConfigHub")

- [ ] **Customer Space type** — Is there a way to mark a Space as "customer-facing" with different permissions than internal App Spaces?

- [ ] **Editable fields constraint** — Can we define at Space or Unit level: "this user/Space can only modify paths X, Y, Z"?

## To Build (New Features)

- [ ] **DynamoDB as Source** — Read/write config from DynamoDB, like Git is a Source
  - Enables: migrate existing RT-Message-App config into ConfigHub
  - Enables: ConfigHub governs, DynamoDB is backing store

- [ ] **OCI export on change** — Push config as OCI artifact when Unit changes
  - Enables: Spegel distribution for K8s workloads
  - Enables: Standard pull-based config distribution

- [ ] **Config pull SDK** — Go/Python/Node libraries for workloads to fetch config
  - Simple: `confighub.Get("unit-slug")` → returns config
  - With caching, polling, etc.

## To Validate (Demo/Test)

- [ ] **Deploy this example** — Use `cub` to create the Hub, App Spaces, Units
- [ ] **Test inheritance** — Verify unit variants work as expected
- [ ] **Test push-upgrade** — Change template, see it propagate
- [ ] **Customer edit flow** — Simulate ACME editing their config

## Questions for ConfigHub Team

1. How do field-level permissions work today?
2. Is there a "customer Space" concept or do we need to build it?
3. What's the best pattern for "customer edits override, but platform fields always win"?
4. Does `push-upgrade` handle merge conflicts? How?

## Future (After Core Works)

- [ ] Spegel integration for P2P distribution
- [ ] AI context from config Units (decision traces)
- [ ] Customer dashboard in ConfigHub GUI
- [ ] Self-serve portal for enterprise customers
