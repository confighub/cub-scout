#!/bin/bash
# Demo: Repo Skeleton → ConfigHub Hierarchy Mapping
#
# Shows how different GitOps patterns map to ConfigHub's Hub/Space/Unit model

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Source UI library
source "$ROOT_DIR/test/atk/lib/ui.sh"
ui_init "$ROOT_DIR"

# Colors
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
DIM='\033[2m'
BOLD='\033[1m'
NC='\033[0m'

clear

# Header
ui_header "⚡ REPO SKELETON → CONFIGHUB HIERARCHY"

echo ""
ui_section "CONCEPT" "Your repo structure maps to ConfigHub's Hub → App Space → Unit model"
echo ""

# Show the mapping concept
cat << 'EOF'
┌─────────────────────────────────────────────────────────────────────────┐
│                        YOUR GITOPS REPO                                  │
│                                                                          │
│   apps/                     ──────────────────▶  App Spaces              │
│   ├── frontend/                                                          │
│   │   └── overlays/         ──────────────────▶  Units + Variants        │
│   │       ├── dev/                               (frontend: dev, prod)   │
│   │       └── prod/                                                      │
│   └── backend/                                                           │
│       └── overlays/         ──────────────────▶  Units + Variants        │
│           ├── dev/                               (backend: dev, prod)    │
│           └── prod/                                                      │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
EOF

echo ""
sleep 1

# Pattern comparison
ui_section "PATTERN COMPARISON" "How each skeleton maps to ConfigHub"
echo ""

echo -e "${BOLD}Pattern A1: Flux Monorepo${NC}"
echo ""
cat << 'EOF'
flux-monorepo/                    ConfigHub Hierarchy
─────────────                     ───────────────────
apps/apptique/                    Hub: apptique-platform
├── base/           ─────────▶   │
│   └── deployment.yaml          │ App Space: apptique
└── overlays/                    │ │
    ├── dev/        ─────────▶   │ └── Unit: frontend
    │   └── kustomization.yaml   │     ├── variant: dev   → apptique-dev
    └── prod/       ─────────▶   │     └── variant: prod  → apptique-prod
        └── kustomization.yaml
EOF
echo ""
sleep 1

echo -e "${BOLD}Pattern B1: Argo ApplicationSet${NC}"
echo ""
cat << 'EOF'
argo-applicationset/              ConfigHub Hierarchy
────────────────────              ───────────────────
bootstrap/                        Hub: apptique-platform
└── applicationset.yaml  ────▶   │
                                 │ Generator Unit: apptique-appset
apps/apptique/                   │ │
├── dev/             ─────────▶  │ ├── Instance: apptique-dev
│   └── deployment.yaml          │ │   └── target: dev-cluster
└── prod/            ─────────▶  │ └── Instance: apptique-prod
    └── deployment.yaml          │     └── target: prod-cluster
EOF
echo ""
sleep 1

echo -e "${BOLD}Pattern B4: Argo App-of-Apps${NC}"
echo ""
cat << 'EOF'
argo-app-of-apps/                 ConfigHub Hierarchy
─────────────────                 ───────────────────
root/                             Hub: apptique-platform
└── root-app.yaml    ─────────▶  │ [NOT imported - manages App CRs only]
                                 │
apps/                            │ App Space: apptique
├── apptique-dev.yaml ────────▶  │ │
└── apptique-prod.yaml           │ └── Unit: frontend
                                 │     ├── variant: dev  (via child app)
manifests/apptique/              │     └── variant: prod (via child app)
├── dev/
│   └── deployment.yaml
└── prod/
    └── deployment.yaml
EOF
echo ""
sleep 1

# Live hierarchy demo
ui_section "LIVE HIERARCHY" "What ConfigHub sees from your cluster"
echo ""

# Check if we can show live data
if command -v cub &> /dev/null && cub auth status &> /dev/null; then
    echo -e "${GREEN}✓${NC} ConfigHub connected - showing live hierarchy"
    echo ""

    # Show actual hierarchy
    "$ROOT_DIR/test/atk/map" confighub 2>/dev/null || {
        echo -e "${YELLOW}⚠${NC} No units imported yet. Try:"
        echo "   ./cub-scout import -n apptique-dev"
        echo "   ./cub-scout import -n apptique-prod"
    }
else
    echo -e "${YELLOW}⚠${NC} ConfigHub not connected - showing simulated hierarchy"
    echo ""

    # Simulated hierarchy
    cat << 'EOF'
Hub: apptique-platform
├── App Space: apptique
│   ├── Unit: frontend
│   │   ├── dev     ✓ synced @ rev 127   → namespace: apptique-dev
│   │   └── prod    ✓ synced @ rev 127   → namespace: apptique-prod
│   └── Unit: cart-service
│       ├── dev     ✓ synced @ rev 125
│       └── prod    ⚠ drift detected
└── App Space: apptique-infra
    ├── Unit: redis
    │   └── prod    ✓ synced @ rev 89
    └── Unit: monitoring
        └── prod    ✓ synced @ rev 91
EOF
fi

echo ""

# Queries
ui_section "FLEET QUERIES" "Questions ConfigHub answers instantly"
echo ""

cat << EOF
${DIM}# Find all production units${NC}
cub unit list --where "environment=prod"

${DIM}# Find units with drift${NC}
cub unit list --where "drift=true"

${DIM}# Find unhealthy units${NC}
cub unit list --where "healthy=false"

${DIM}# Find units by owner (Flux, ArgoCD, Helm)${NC}
./cub-scout map list -q "owner=Flux"

${DIM}# Find units in specific namespace pattern${NC}
./cub-scout map list -q "namespace=apptique-*"
EOF

echo ""

# Next steps
ui_section "NEXT STEPS" "Try it yourself"
echo ""

cat << EOF
1. Deploy a pattern:
   kubectl apply -k examples/apptique-examples/flux-monorepo/apps/apptique/overlays/dev/

2. See ownership:
   ./test/atk/map workloads

3. Import to ConfigHub:
   ./cub-scout import -n apptique-dev

4. View hierarchy:
   ./test/atk/map confighub
EOF

echo ""
ui_msg ok "Demo complete"
