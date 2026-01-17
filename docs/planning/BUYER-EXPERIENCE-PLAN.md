# Buyer Experience Plan

**Goal:** A potential buyer finds cub-scout and can try it in under 2 minutes.

---

## Current State

| Step | Experience |
|------|------------|
| Find repo | Private - can't access |
| Clone | Fails |
| Install | Requires Go toolchain |
| See what it does | No screenshots |
| Understand value | Technical jargon |
| Try it | Impossible |

**Result:** Buyer leaves.

---

## Target State

| Step | Experience |
|------|------------|
| Find repo | Public, appears in search |
| See what it does | Screenshot + GIF in README |
| Understand value | "Find orphans in 10 seconds" |
| Install | `brew install` or `docker run` |
| Try it | Works on their cluster immediately |
| Want more | Clear path to ConfigHub |

**Result:** Buyer tries it, sees value, explores ConfigHub.

---

## Action Plan

### Phase 1: Make It Visible (Day 1)

| Task | Owner | Effort |
|------|-------|--------|
| Make repo public | Alexis | 5 min |
| Add LICENSE file (MIT) | Alexis | 5 min |
| Add .github/FUNDING.yml | Alexis | 5 min |

```bash
gh repo edit confighub/cub-scout --visibility public
```

---

### Phase 2: Make It Tryable (Day 1-2)

#### 2a. GitHub Releases

| Task | Effort |
|------|--------|
| Add GoReleaser config | 30 min |
| Create release workflow | 30 min |
| Tag v0.1.0 | 5 min |

**.goreleaser.yaml:**
```yaml
builds:
  - main: ./cmd/cub-scout
    binary: cub-scout
    goos: [linux, darwin]
    goarch: [amd64, arm64]

archives:
  - format: tar.gz

brews:
  - repository:
      owner: confighub
      name: homebrew-tap
    homepage: https://confighub.com
    description: Explore and map GitOps in your clusters
```

**Result:**
```bash
brew install confighub/tap/cub-scout
cub-scout map
```

#### 2b. Container Image

| Task | Effort |
|------|--------|
| Add Dockerfile (already exists) | 0 min |
| Add ghcr.io publish workflow | 30 min |

**Result:**
```bash
docker run --rm -v ~/.kube:/root/.kube ghcr.io/confighub/cub-scout map list
```

---

### Phase 3: Make It Sellable (Day 2-3)

#### 3a. README Screenshot

Capture TUI screenshot:
```bash
./cub-scout map  # screenshot the TUI
```

Add to README.md above "Start Here":
```markdown
![cub-scout TUI](docs/images/tui-screenshot.png)

**See who owns every resource in your cluster.**
```

#### 3b. Demo GIF

Record 30-second demo:
```bash
# Use asciinema or vhs
vhs record demo.tape
```

**demo.tape:**
```
Output demo.gif
Set Width 1200
Set Height 600

Type "./cub-scout map"
Enter
Sleep 2s
Type "2"  # Switch to deployers tab
Sleep 2s
Type "q"
```

#### 3c. Value Proposition

**Current README:**
> Explore and map GitOps in your clusters

**New README:**
> **Find who owns every Kubernetes resource in 10 seconds.**
>
> - Detect Flux, ArgoCD, Helm, or orphaned resources
> - Trace any resource back to its Git source
> - Find misconfigurations before they cause outages
>
> No signup required. Works on any cluster.

---

### Phase 4: Clear Pricing (Day 3)

Add to README:

```markdown
## Pricing

| Feature | Free | Pro |
|---------|------|-----|
| Single cluster | ✓ | ✓ |
| Ownership detection | ✓ | ✓ |
| Orphan detection | ✓ | ✓ |
| CCVE scanning | ✓ | ✓ |
| Multi-cluster fleet | — | ✓ |
| Import to ConfigHub | — | ✓ |
| Team collaboration | — | ✓ |

**Free forever for single cluster use.**

[Start free →](https://confighub.com/signup)
```

---

## Timeline

| Day | Deliverable |
|-----|-------------|
| Day 1 | Repo public, GitHub Releases, Homebrew tap |
| Day 2 | Docker image, Screenshot in README |
| Day 3 | Demo GIF, Value prop rewrite, Pricing table |

---

## Success Metrics

| Metric | Target |
|--------|--------|
| Time to first `map` | < 2 minutes |
| README has screenshot | Yes |
| `brew install` works | Yes |
| `docker run` works | Yes |
| Pricing visible | Yes |
| No Go required | Yes |

---

## Files to Create/Modify

| File | Action |
|------|--------|
| `.goreleaser.yaml` | Create |
| `.github/workflows/release.yaml` | Create |
| `.github/workflows/docker.yaml` | Create |
| `docs/images/tui-screenshot.png` | Create |
| `docs/images/demo.gif` | Create |
| `README.md` | Rewrite |
| `Dockerfile` | Verify works |

---

## Quick Wins (Do Today)

1. `gh repo edit confighub/cub-scout --visibility public`
2. Screenshot the TUI, add to README
3. Rewrite first 3 lines of README with value prop

Everything else can follow.
