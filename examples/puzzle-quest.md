# The Missing Ownership Maze

## A Puzzle Quest for New `cub-scout` Operators

You are a first-time operator dropped into a mystery cluster.
Your mission: prove you can see reality, explain ownership, and — if you
dare — claim the territory as your own.

Complete Stages 0–4 to earn the title: **Scout of the Read-Only Realm**.
Complete all stages to earn the title: **Warden of DRY, WET, and LIVE**.

---

## Mission Rules

1. Use `./cub-scout` as your main tool (always the local binary).
2. Prefer read-only commands first. Never modify cluster state.
3. Capture evidence (command output) for each stage.
4. Report what failed and why — silence is not an answer.
5. Stages 0–3 require **no** ConfigHub authentication.
6. Stages 4–5 introduce ConfigHub. Auth is optional — the quest guides you.

### Execution Mode

This quest works two ways:

- **Human mode:** Read each stage, run the commands, record your answers.
- **AI mode:** An AI agent can execute every command sequentially.
  Every step is scriptable. No interactive-only steps.

---

## Stage 0: The Arena

Before you can explore, you need a world.

This stage builds cub-scout from source and creates a demo cluster with
mixed ownership — real GitOps controllers, Helm releases, and unmanaged
resources all coexisting.

### Step 1 — Build cub-scout

```bash
cd <repo-root>
go build ./cmd/cub-scout
./cub-scout version
```

### Step 2 — Check prerequisites

```bash
for tool in kind kubectl docker jq; do
  command -v "$tool" >/dev/null 2>&1 && echo "OK  $tool" || echo "MISSING  $tool — install before continuing"
done
docker info >/dev/null 2>&1 && echo "OK  Docker is running" || echo "FAIL  Docker is not running — start it first"
```

All four tools must show `OK`. If any are missing, install them before
continuing:

| Tool | Install |
|------|---------|
| docker | <https://docs.docker.com/get-docker/> |
| kind | `brew install kind` or <https://kind.sigs.k8s.io/> |
| kubectl | `brew install kubectl` or <https://kubernetes.io/docs/tasks/tools/> |
| jq | `brew install jq` or <https://jqlang.github.io/jq/download/> |

### Step 3 — Bootstrap the demo cluster

Pick **one** path. Both create a cluster with 3+ ownership types:

**Option A — ArgoCD** (ArgoCD + Helm + Native, ~5 min):

```bash
cd examples/argo-import-confighub-demo
./demo.sh --keep
cd <repo-root>
```

**Option B — Flux** (Flux + Helm + Native, ~5 min):

```bash
cd examples/flux-import-confighub-demo
./demo.sh --keep
cd <repo-root>
```

The `--keep` flag preserves the cluster for exploration.

### Step 4 — Verify the arena

```bash
./cub-scout version
kubectl config current-context
kubectl get namespaces
```

### Success condition

- `./cub-scout version` prints a version string.
- `kubectl config current-context` shows `kind-argo-import-demo` or `kind-flux-import-demo`.
- `kubectl get namespaces` shows application namespaces (ArgoCD path: `myapp-dev`, `myapp-prod`, `guestbook`; Flux path: `payments`, `store`, `podinfo`).

### If something goes wrong

| Problem | Fix |
|---------|-----|
| Docker not running | Start Docker Desktop or `systemctl start docker` |
| kind cluster already exists | `kind delete cluster --name argo-import-demo` (or `flux-import-demo`), then re-run |
| `flux` CLI missing (Flux path) | `brew install fluxcd/tap/flux` |
| Demo script fails mid-way | Check the error message. Usually Docker or network. Re-run the script. |

### Passphrase

**"The arena is set."**

---

## Stage 1: The Map

The cluster is alive and no one left a README. Your first job: find out
who owns what. Multiple tools have left their marks — labels, annotations,
the breadcrumbs of ownership.

Find at least three different owners.

### Commands

```bash
# See everything at a glance
./cub-scout map list

# Count owners (the money command)
./cub-scout map list --json | jq -r '.[].owner' | sort | uniq -c | sort -rn

# How many resources total?
./cub-scout map list --json | jq 'length'

# Workload-focused view (optional)
./cub-scout map workloads
```

### Expected output

| Path | You should see |
|------|----------------|
| ArgoCD | ArgoCD (~10), Helm (~3), Native (50+) |
| Flux | Flux (~13), Helm (~1), Native (40+) |

Exact numbers vary. The important thing: **at least three distinct owners**.

### Success condition

- List owner counts from the `jq` command.
- Identify which owner has the most resources.
- Name at least one resource for each owner type found.

### If something goes wrong

| Problem | Fix |
|---------|-----|
| `map list` returns nothing | Verify `kubectl config current-context` matches the demo cluster |
| Only Native resources | Demo fixtures may not have applied — re-run the demo script |

### Passphrase

**"The cluster has many kings."**

---

## Stage 2: The Trail

Ownership labels tell you WHO. But what is the full picture?

Use `tree ownership` to see every resource grouped by its owner, with
the parent GitOps resource that manages it. Then optionally use `trace`
to follow a single workload deeper.

### Commands

```bash
# See the full ownership hierarchy (always works, no controller CLI needed)
./cub-scout tree ownership

# Workload-focused grouping
./cub-scout map workloads
```

**Optional deep trace** (requires controller CLI context — see note below):

**ArgoCD path:**

```bash
./cub-scout trace deploy/api -n myapp-prod
./cub-scout trace deploy/api -n myapp-prod --explain
```

**Flux path:**

```bash
./cub-scout trace deploy/payment-api -n payments
./cub-scout trace deploy/payment-api -n payments --explain
```

> **Note on `trace`:** The `trace` command for ArgoCD workloads requires
> `argocd` CLI context (login to the ArgoCD server). The demo cluster
> installs ArgoCD but does not configure the CLI context. If `trace`
> fails with "argocd context appears stale", that is expected — use
> `tree ownership` instead. It shows the same ownership chain without
> requiring controller CLI access.
>
> To set up ArgoCD CLI context (optional):
> ```bash
> ARGO_PWD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d)
> kubectl port-forward svc/argocd-server -n argocd 8080:443 &
> argocd login localhost:8080 --username admin --password "$ARGO_PWD" --insecure
> ```

### What to expect from `tree ownership`

```
ArgoCD (8)
  ├── guestbook/helm-guestbook -> application/helm-guestbook
  ├── guestbook/kustomize-guestbook-ui -> application/kustomize-guestbook
  ├── myapp-dev/api -> application/myapp-dev
  ├── myapp-dev/worker -> application/myapp-dev
  ...

Helm (3)
  ├── myapp-dev/redis
  ├── myapp-prod/redis
  └── myapp-staging/redis
```

The demo cluster contains two kinds of workloads:

1. **Real sync** — guestbook (ArgoCD) or podinfo (Flux). These have real
   GitOps parents: `application/helm-guestbook`, `kustomization/podinfo`.
2. **Brownfield fixtures** — Arnie (ArgoCD) or D2 (Flux). These have
   correct ownership labels but fake source repos. The tree shows
   ownership correctly. **This is expected.** cub-scout reports what the
   metadata says — it does not guess.

### Success condition

- Show the output of `tree ownership`.
- Identify how many ArgoCD, Helm, and Native resources exist.
- Name one workload and its parent GitOps resource (e.g., "api in myapp-prod
  is managed by application/myapp-prod").
- If you tried `trace` and it failed, explain why (e.g., "ArgoCD CLI context
  not configured — this is a trace-specific requirement, not a cub-scout bug").

### If something goes wrong

| Problem | Fix |
|---------|-----|
| `tree ownership` shows nothing | Verify `kubectl config current-context` matches the demo cluster |
| `trace` fails with "argocd context appears stale" | Expected. Use `tree ownership` instead, or set up ArgoCD CLI context (see note above) |
| "no GitOps owner detected" | The workload is Native — that IS a valid finding. Report it |

### Passphrase

**"Ownership is evidence, not guesswork."**

---

## Stage 3: The Weather Report

Now you know who owns what and where it came from. But is the pipeline
healthy? A deployer can exist and still be broken.

Read the weather.

### Commands

```bash
# Human-friendly view
./cub-scout gitops status

# Machine-readable (for scripting and evidence)
./cub-scout gitops status --json | jq '{backend, transport, healthyCount, failedCount}'

# Risk scan (bonus depth)
./cub-scout scan
```

### What to expect

| Path | Backend | Healthy | Failing | Why |
|------|---------|---------|---------|-----|
| ArgoCD | argocd | 2 (guestbook apps) | 3 (myapp-dev/staging/prod) | Source repos are fictional |
| Flux | flux | ~2 (podinfo) | ~4 (D2 brownfield Kustomizations) | Source repos are fictional |

### Success condition

- Report the detected backend (ArgoCD or Flux) and transport (Git, OCI, Helm).
- Report healthy vs failing deployer count.
- Name one specific failing deployer and explain why it is failing.
- From `./cub-scout scan`: report any stuck states or policy violations found (or "none detected").

### If something goes wrong

| Problem | Fix |
|---------|-----|
| "no deployers found" | Check `kubectl get applications -A` (ArgoCD) or `kubectl get kustomizations -A` (Flux) |
| All deployers show healthy | Fictional fixtures may have been skipped. Re-check `kubectl get deploy -A` |

### Passphrase

**"I know where the pipeline breaks."**

---

## Intermission: The View From Here

Stop. Look at what you have done.

You can now:

- **See** every resource on the cluster and who owns it
- **Trace** any workload back to its Git source (or report where the trail ends)
- **Read** the health of every GitOps deployer and explain what is broken

You are a **read-only expert**. You have mapped the territory.

But mapping is not managing.

Everything you discovered exists only in your terminal output. It is not
tracked. It is not versioned. It is not connected to anything larger.
If your terminal closes, it is gone.

What if you could take everything you just discovered — every owner, every
trace, every broken pipeline — and turn it into managed state? What if the
cluster could be not just observed but **owned**?

That is what Stages 4 and 5 do. They introduce **ConfigHub** — the platform
that turns discovery into managed infrastructure.

Here is the progression:

| Stage | What you can do | What changes |
|-------|-----------------|--------------|
| 0–3 | **I can SEE everything** | Nothing. Pure observation. |
| 4 | **I can PREVIEW what managing it looks like** | Nothing. Still read-only. |
| 5 | **I OWN it now** | Workloads become managed ConfigHub units. |

Stage 4 is still safe. It is a dry-run — a mirror that shows you what
ConfigHub *would* do, without doing it.

Stage 5 is where you claim the territory.

**You mapped the territory. Now claim it.**

---

## Stage 4: The Mirror of Suggestion

This stage is still read-only. No ConfigHub auth required. No changes
made. Nothing leaves your machine.

The import command reads your cluster and proposes a structure: Apps,
Deployments, unit groupings. Review the proposal like a building
inspector reviews blueprints.

### Commands

```bash
# See the proposal in human-readable form
./cub-scout import --dry-run

# Capture the full proposal as JSON
./cub-scout import --dry-run --json | tee /tmp/cub-import-proposal.json

# Inspect the proposal
jq '.proposal.appSpace' /tmp/cub-import-proposal.json
jq '.proposal.units | length' /tmp/cub-import-proposal.json
jq '.proposal.units[] | {slug, app}' /tmp/cub-import-proposal.json

# Workload-to-owner mapping
jq '.workloads[] | {name, namespace, owner, connected}' /tmp/cub-import-proposal.json
```

### What to look for

- The **App Space name** — auto-derived from namespace patterns.
- The **number of proposed units** — how many logical groupings did
  cub-scout find?
- At least one unit grouping that makes sense (e.g., "api is grouped
  across dev/staging/prod — that tracks").
- At least one label mapping to question or note.
- Confirm `(dry-run mode - no changes made)` appears in the ASCII output.
- All workloads should show `"connected": false` — nothing is imported yet.

### Success condition

- State the proposed App Space name.
- State the number of proposed units.
- Identify one unit grouping that looks correct and explain why.
- Confirm the dry-run safety message appeared.

### If something goes wrong

| Problem | Fix |
|---------|-----|
| "No workloads found" | Verify cluster context with `kubectl config current-context` |
| JSON parse error | Check with `jq '.' /tmp/cub-import-proposal.json \| head -20` first |

### Passphrase

**"I reviewed before I wrote."**

---

## Stage 5: The Level-Up

This is the moment.

Everything you discovered in Stages 0–4 — every owner, every trail, every
broken pipeline, every unit proposal — is about to become real.

After this stage, your workloads are not just visible. They are **managed**.

### Pre-flight: Auth check

```bash
if command -v cub >/dev/null 2>&1 && cub auth get-token >/dev/null 2>&1; then
  echo "Authenticated to ConfigHub. Ready to import."
else
  echo ""
  echo "  Not authenticated to ConfigHub (or cub CLI not installed)."
  echo ""
  echo "  To continue:"
  echo "    1. Install the cub CLI:  https://confighub.com/docs/cli"
  echo "    2. Authenticate:         cub auth login"
  echo "    3. Re-run this stage."
  echo ""
  echo "  No account? Sign up free:  https://app.confighub.com"
  echo ""
  echo "  Stages 0-4 are complete. You have already proven you can SEE"
  echo "  the cluster. Stage 5 lets you MANAGE it."
  echo "  Come back when ready. The cluster will wait."
  echo ""
fi
```

**If the auth check fails, stop here.** You have earned the title
**Scout of the Read-Only Realm**. Come back for Stage 5 when you have
a ConfigHub account. The demo cluster persists (you used `--keep`).

### Import (only if authenticated)

```bash
# Import with auto-confirm (scriptable, no interactive prompt)
./cub-scout import -y
```

This is equivalent to running `./cub-scout import` and typing `y` at the
`Import N deployments into App 'X'? [y/N]` prompt. The `-y` flag makes it
scriptable for both human and AI execution.

### What to capture

After the import completes:

```bash
# Check connection status
./cub-scout status

# Verify units were created in ConfigHub
cub unit list --space "$(./cub-scout status --json | jq -r '.space')"

# Check the mode
./cub-scout status --json | jq '{mode, space, auth_valid}'
```

### Success condition

- Import completes without error.
- Output shows the App Space name and number of units created.
- `./cub-scout status` shows mode `"connected"` (not `"offline"`).
- `cub unit list` shows the units that were created.

> **Note:** The `connected` field in `import --dry-run --json` workload
> output will still show `false`. This is correct — `import` creates units
> in ConfigHub but does not modify cluster labels (cub-scout is read-only).
> The `connected` flag flips to `true` only after a ConfigHub worker
> applies the `confighub.com/UnitSlug` label to the cluster resources.
> For this demo, verify via `cub unit list` instead.

### If something goes wrong

| Problem | Fix |
|---------|-----|
| `cub` command not found | Install the cub CLI first |
| "token expired" | Run `cub auth login` and retry |
| "App already exists" | A previous import partially completed. Note it and continue — this is fine |
| No `cub` and no account | **This is OK.** Stop here. Stages 0–4 are complete. |

### Passphrase

**"I claimed the territory."**

---

## Stage 6: The Verification Seal

Trust but verify.

You imported the cluster into ConfigHub. But did it work? Run the same
discovery commands from Stages 1–3 and prove the cluster is now **known**.

The best test of an import is that the same tools that discovered the
cluster *before* now show it as *connected*.

### Commands

```bash
# Re-run the owner count (should be UNCHANGED — import doesn't modify labels)
./cub-scout map list --json | jq -r '.[].owner' | sort | uniq -c | sort -rn

# Show connection status and space
./cub-scout status
./cub-scout status --json | jq '{mode, space, auth_valid}'

# Verify ConfigHub has the units (the proof that import worked)
SPACE=$(./cub-scout status --json | jq -r '.space')
cub unit list --space "$SPACE"
echo "Units in space '$SPACE': $(cub unit list --space "$SPACE" 2>/dev/null | tail -n +2 | wc -l | tr -d ' ')"

# Re-run tree ownership — still works exactly as before
./cub-scout tree ownership
```

### Success condition

- Owner breakdown is **unchanged** (import does not modify ownership labels).
- `./cub-scout status` shows mode `"connected"` and your ConfigHub space.
- `cub unit list` shows the 5 units created in Stage 5.
- `tree ownership` output is identical to Stage 2 — zero cluster-side changes.

### What this proves

Stage 6 validates that import is **additive, not destructive**:

- **Cluster side:** Zero changes. Same labels, same owners, same resources.
  cub-scout is read-only. Always.
- **ConfigHub side:** 5 new units exist. The cluster's workloads are now
  tracked as managed infrastructure.

The same discovery tools that worked before the import work identically after.
The only difference is that ConfigHub now knows about your cluster.

**This protects against:** false ownership changes, silent label mutation,
import side-effects that break existing tooling.

### If something goes wrong

| Problem | Fix |
|---------|-----|
| No connected resources | The import may have created the App Space but not linked workloads. Check `./cub-scout status --json` |
| Status shows "offline" | The `cub` CLI is not configured. Run `cub auth login` |

### Passphrase

**"Tested truths beat lucky runs."**

---

## Bonus: The Oracle Report

Produce a summary report. Every line must have command evidence — not
assumptions, not guesses, not memory. If you cannot fill a line, say why.

```
=== ORACLE REPORT ===

Cluster Context:      [kubectl config current-context]
Cluster Source:        [argo-import-confighub-demo or flux-import-confighub-demo]
cub-scout Version:    [./cub-scout version]

--- DISCOVERY (Stages 1-3) ---

Owners Found:         [./cub-scout map list --json | jq -r '.[].owner' | sort | uniq -c]
Total Resources:      [./cub-scout map list --json | jq 'length']

Traced Workload:      [which workload was traced in Stage 2]
  Owner:              [detected owner]
  Source Resolved:    [yes/no, and what was found]
  Trail Notes:        [complete trail / fictional source / broken at stage X]

GitOps Backend:       [./cub-scout gitops status --json | jq -r '.backend']
GitOps Transport:     [./cub-scout gitops status --json | jq -r '.transport']
Healthy Deployers:    [./cub-scout gitops status --json | jq '.healthyCount']
Failed Deployers:     [./cub-scout gitops status --json | jq '.failedCount']

--- IMPORT (Stages 4-5) ---

Dry-Run App Space:    [from Stage 4]
Dry-Run Units:        [from Stage 4]

Import Result:        [completed / skipped (no auth) / failed (reason)]
ConfigHub Space:      [./cub-scout status --json | jq -r '.space']
ConfigHub Units:      [cub unit list --space <space> | tail -n +2 | wc -l]
ConfigHub Mode:       [./cub-scout status --json | jq -r '.mode']

--- ASSESSMENT ---

Open Risks:           [from ./cub-scout scan, or "None detected"]
Limitations Found:    [any incomplete trails, fictional sources, etc.]
Quest Completed:      [Stage reached: 0-4 or 0-6 + Bonus]
Title Earned:         [Scout of the Read-Only Realm / Warden of DRY, WET, and LIVE]
```

You win if your report contains concrete command evidence for every line
you filled in.

---

## Cleanup

When you are done exploring, tear down the demo cluster:

```bash
# ArgoCD path
kind delete cluster --name argo-import-demo

# Flux path
kind delete cluster --name flux-import-demo
```

---

## Designer Notes

- Stage 0 bootstraps the arena — no pre-existing cluster assumed.
- Stages 0–3 work with zero ConfigHub interaction.
- Stage 4 is a read-only dry-run — shows what ConfigHub WOULD do.
- Stage 5 has a graceful auth gate — the quest does not abort.
- Stage 6 reuses discovery commands as verification (additive, not destructive).
- Every command uses `./cub-scout` (the local binary).
- Every jq path is verified against the cub-scout source code.
- Both ArgoCD and Flux demo paths are fully supported.
- Passphrases are achievements, not gates.
- Partial completion (Stages 0–4) earns its own title.
