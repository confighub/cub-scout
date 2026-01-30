#!/bin/bash
# Cross-Owner Reference Demo
# Shows: Crossplane detection, cross-owner references, elapsed time
#
# This is a visual demo that simulates output without requiring a cluster.
# For real cluster testing, apply cross-owner-demo.yaml first.

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
DIM='\033[2m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Symbols
CHECK="${GREEN}✓${NC}"
WARN="${YELLOW}⚠${NC}"
CROSS="${RED}✗${NC}"
ARROW="${CYAN}→${NC}"
BULLET="${DIM}•${NC}"

clear
echo ""
echo -e "${BOLD}╔═══════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║       cub-scout: Cross-Owner Reference Detection Demo             ║${NC}"
echo -e "${BOLD}╚═══════════════════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${DIM}New in v0.3.3: Crossplane detection, cross-owner warnings, elapsed time${NC}"
echo ""

sleep 1

# =============================================================================
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${BLUE}  FEATURE 1: Crossplane Owner Detection${NC}"
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${DIM}Crossplane manages cloud infrastructure as Kubernetes resources.${NC}"
echo -e "${DIM}cub-scout now detects Crossplane ownership via labels and annotations.${NC}"
echo ""
sleep 1

echo -e "${CYAN}$ ./cub-scout map workloads -n crossplane-system${NC}"
echo ""
sleep 0.5

cat << 'EOF'
WORKLOADS (crossplane-system)
──────────────────────────────────────────────────────────────────

  Deployment              Owner        Claim                    Ready
  ──────────────────────────────────────────────────────────────────
  rds-proxy               Crossplane   ecommerce-db             ✓ 1/1
  elasticache-proxy       Crossplane   ecommerce-cache          ✓ 1/1

EOF
echo ""
sleep 2

echo -e "${CYAN}$ ./cub-scout trace deploy/rds-proxy -n crossplane-system${NC}"
echo ""
sleep 0.5

echo -e "  ${BOLD}Deployment${NC} rds-proxy (crossplane-system)"
echo -e "  ${DIM}Owner:${NC}   ${GREEN}Crossplane${NC} (claim)"
echo -e "  ${DIM}Claim:${NC}   ecommerce-db"
echo -e "  ${DIM}Status:${NC}  ${GREEN}Ready${NC}"
echo -e "  ${DIM}Elapsed:${NC} 2h 15m"
echo ""
echo -e "  ${BULLET} Crossplane Composite: xpostgresqlinstance-abc123"
echo -e "  ${BULLET} Composition Resource: rds-instance"
echo ""
sleep 2

# =============================================================================
echo ""
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${BLUE}  FEATURE 2: Cross-Owner Reference Detection${NC}"
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${DIM}When a workload references secrets/configmaps owned by a different${NC}"
echo -e "${DIM}controller, cub-scout warns about potential coordination issues.${NC}"
echo ""
sleep 1

echo -e "${CYAN}$ ./cub-scout trace deploy/api-server -n ecommerce${NC}"
echo ""
sleep 0.5

echo -e "  ${BOLD}Deployment${NC} api-server (ecommerce)"
echo -e "  ${DIM}Owner:${NC}   ${GREEN}Flux${NC} (Kustomization)"
echo -e "  ${DIM}Source:${NC}  ecommerce-apps"
echo -e "  ${DIM}Status:${NC}  ${GREEN}Ready${NC}"
echo -e "  ${DIM}Elapsed:${NC} 5m 30s"
echo ""
echo -e "  ${BOLD}Ownership Chain${NC}"
echo -e "  ├─ Kustomization ecommerce-apps (flux-system)"
echo -e "  │  ${DIM}Status:${NC} Reconciliation succeeded"
echo -e "  │  ${DIM}Elapsed:${NC} 3m 45s"
echo -e "  └─ GitRepository platform-config (flux-system)"
echo -e "     ${DIM}Status:${NC} Fetched revision main@sha1:abc123"
echo -e "     ${DIM}Elapsed:${NC} 4m 12s"
echo ""
echo -e "  ${BOLD}${YELLOW}Cross-Owner References${NC}"
echo -e "  ${WARN} Secret/db-credentials ${ARROW} Owner: ${CYAN}Terraform${NC}"
echo -e "     ${DIM}Referenced via: envFrom.secretRef${NC}"
echo -e "     ${DIM}Risk: Secret updates won't trigger Flux reconciliation${NC}"
echo ""
echo -e "  ${WARN} Secret/redis-credentials ${ARROW} Owner: ${CYAN}Terraform${NC}"
echo -e "     ${DIM}Referenced via: env.valueFrom.secretKeyRef${NC}"
echo ""
sleep 3

# =============================================================================
echo ""
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${BLUE}  FEATURE 3: Elapsed Time Display${NC}"
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${DIM}Trace output now shows elapsed time since last reconciliation.${NC}"
echo -e "${DIM}Highlights resources stuck in non-ready state for >5 minutes.${NC}"
echo ""
sleep 1

echo -e "${CYAN}$ ./cub-scout trace deploy/stuck-deployment -n prod${NC}"
echo ""
sleep 0.5

echo -e "  ${BOLD}Deployment${NC} stuck-deployment (prod)"
echo -e "  ${DIM}Owner:${NC}   ${GREEN}Flux${NC} (Kustomization)"
echo -e "  ${DIM}Status:${NC}  ${YELLOW}Progressing${NC}"
echo -e "  ${DIM}Elapsed:${NC} ${YELLOW}12m 45s ⚠${NC}"
echo ""
echo -e "  ${BOLD}Ownership Chain${NC}"
echo -e "  ├─ Kustomization app-stack (flux-system)"
echo -e "  │  ${DIM}Status:${NC} ${YELLOW}Reconciling${NC}"
echo -e "  │  ${DIM}Elapsed:${NC} ${YELLOW}8m 30s ⚠${NC}"
echo -e "  │  ${DIM}Message:${NC} ${RED}dependency 'flux-system/infra' is not ready${NC}"
echo -e "  └─ GitRepository main-repo (flux-system)"
echo -e "     ${DIM}Status:${NC} ${GREEN}Fetched revision main@sha1:def456${NC}"
echo -e "     ${DIM}Elapsed:${NC} 1m 15s"
echo ""
sleep 2

# =============================================================================
echo ""
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${BLUE}  Real-World Scenario: Multi-Team Platform${NC}"
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "${DIM}Platform team uses Crossplane + Terraform for infrastructure.${NC}"
echo -e "${DIM}App teams use Flux/ArgoCD for workloads.${NC}"
echo -e "${DIM}cub-scout shows the full picture.${NC}"
echo ""
sleep 1

echo -e "${CYAN}$ ./cub-scout map workloads -n ecommerce${NC}"
echo ""
sleep 0.5

cat << 'EOF'
WORKLOADS (ecommerce)
──────────────────────────────────────────────────────────────────

  Deployment              Owner        Source                   Ready   Cross-Refs
  ──────────────────────────────────────────────────────────────────────────────
  api-server              Flux         ecommerce-apps           ✓ 3/3   ⚠ 2 secrets
  payment-service         Flux         ecommerce-apps           ✓ 2/2   ⚠ 1 secret
  frontend                Flux         ecommerce-apps           ✓ 2/2
  analytics-collector     ArgoCD       analytics-app            ✓ 1/1   ⚠ 1 secret
  debug-pod               Native       kubectl                  ✓ 1/1   ⚠ 1 secret

CROSS-OWNER SUMMARY
──────────────────────────────────────────────────────────────────
  ⚠ 4 workloads reference 3 Terraform-managed secrets

  Secret/db-credentials        → Used by: api-server, analytics, debug-pod
  Secret/redis-credentials     → Used by: api-server
  Secret/payment-api-keys      → Used by: payment-service

EOF
echo ""
sleep 2

# =============================================================================
echo ""
echo -e "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}${GREEN}  Why This Matters${NC}"
echo -e "${BOLD}${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "  ${CHECK} ${BOLD}Crossplane Detection${NC}"
echo -e "     See cloud infrastructure alongside app workloads"
echo -e "     Understand the full deployment topology"
echo ""
echo -e "  ${CHECK} ${BOLD}Cross-Owner Warnings${NC}"
echo -e "     Identify coordination risks between teams"
echo -e "     Know when secret rotation won't auto-redeploy apps"
echo ""
echo -e "  ${CHECK} ${BOLD}Elapsed Time${NC}"
echo -e "     Quickly spot stalled reconciliations"
echo -e "     Debug \"why isn't my change deploying?\" faster"
echo ""
sleep 1

echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BOLD}  Try it yourself:${NC}"
echo ""
echo -e "  ${DIM}# Apply the demo resources${NC}"
echo -e "  kubectl apply -f examples/demos/cross-owner-demo.yaml"
echo ""
echo -e "  ${DIM}# Trace a workload with cross-owner references${NC}"
echo -e "  ./cub-scout trace deploy/api-server -n ecommerce"
echo ""
echo -e "  ${DIM}# See all workloads with their owners${NC}"
echo -e "  ./cub-scout map workloads"
echo ""
echo -e "  ${DIM}# Clean up${NC}"
echo -e "  kubectl delete -f examples/demos/cross-owner-demo.yaml"
echo ""
echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
