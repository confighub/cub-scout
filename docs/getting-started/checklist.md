# New User Checklist

A friendly guide to getting comfortable with cub-scout. These are suggestions, not rules.

---

## First 5 Minutes

- [ ] **Install cub-scout**
  ```bash
  brew install confighub/tap/cub-scout
  ```

- [ ] **Run your first map**
  ```bash
  cub-scout map
  ```

- [ ] **Press `?` for help** — See all keyboard shortcuts

- [ ] **Press `w` for workloads** — See what's running and who owns it

- [ ] **Press `q` to quit** — You can always come back

---

## First 30 Minutes

- [ ] **Trace a deployment** — Pick any deployment, see where it came from
  ```bash
  cub-scout trace deploy/<name> -n <namespace>
  ```

- [ ] **Find orphans** — Press `o` in the TUI or run:
  ```bash
  cub-scout map orphans
  ```

- [ ] **Check for configuration issues** — Scan for common problems
  ```bash
  cub-scout scan
  ```

- [ ] **Try deep-dive** — Press `4` in the TUI to see everything

---

## First Day

- [ ] **Explore CLI variants** — Same data, different formats
  ```bash
  cub-scout map status              # One-liner health check
  cub-scout map list                # Plain text list
  cub-scout map list --format json  # JSON for scripts
  ```

- [ ] **Try queries** — Filter resources in the TUI
  ```
  /owner:Flux              # Only Flux-managed
  /namespace:production    # Only production namespace
  /owner:Native            # Unmanaged resources
  ```

- [ ] **Read the FAQ** — [docs/FAQ.md](../FAQ.md) has answers to common questions

- [ ] **Check out demos** — See cub-scout in action
  ```bash
  cub-scout demo list
  cub-scout demo quick
  ```

- [ ] **Explore the CLI reference** — [CLI-GUIDE.md](../../CLI-GUIDE.md) has everything

---

## Ready for Connected Mode? *(1.x)*

> These steps require a ConfigHub account. Everything above works standalone.

- [ ] **Connect to ConfigHub** — Free account, unlocks import and fleet features
  ```bash
  cub auth login
  cub-scout status    # Should show ● Connected
  ```

- [ ] **Preview an import** — See what cub-scout would tell ConfigHub
  ```bash
  cub-scout import --dry-run -n <namespace>
  ```

- [ ] **See the full walkthrough** — [First Import guide](first-import.md) (10 minutes)

- [ ] **Import more namespaces** — Follow the [Migration Playbook](../howto/migration-playbook.md)

- [ ] **Learn why connected matters** — [WHY_CONNECTED_MODE.md](../WHY_CONNECTED_MODE.md)

---

## Tips for Success

1. **Start read-only** — cub-scout never modifies your cluster. Explore freely.

2. **Trust the ownership detection** — If cub-scout says "Native", it means no GitOps labels were found. That's useful information.

3. **Use the TUI first** — The interactive map is the fastest way to learn. CLI commands come naturally after.

4. **Press `?` when lost** — Help is always one keystroke away.

5. **Orphans aren't always bad** — System resources, debugging tools, and infrastructure components are often intentionally unmanaged.

---

## Getting Help

- **In the TUI:** Press `?` for help
- **On the CLI:** `cub-scout --help` or `cub-scout <command> --help`
- **Documentation:** [CLI-GUIDE.md](../../CLI-GUIDE.md)
- **Questions:** [Open an issue](https://github.com/confighub/cub-scout/issues)
- **Community:** [Discord](https://discord.gg/confighub)

---

## You're Ready!

Once you've checked off the "First 30 Minutes" items, you know enough to use cub-scout productively. Everything else is exploration and mastery.

**Next:** Try [first-map.md](first-map.md) for a detailed walkthrough.
