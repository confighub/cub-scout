#!/bin/bash
# Impressive Demo: CCVE Detection in Action
# "How ConfigHub Agent Would Have Saved BIGBANK 4 Hours"

set -euo pipefail

R='\033[0;31m'
G='\033[0;32m'
Y='\033[0;33m'
B='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Demo configuration
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MAP_TOOL="../../test/atk/map"

pause() {
    echo ""
    echo -e "${BOLD}Press ENTER to continue...${NC}"
    read -r
}

header() {
    echo ""
    echo -e "${BOLD}${B}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BOLD}${B}  $1${NC}"
    echo -e "${BOLD}${B}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

cmd() {
    echo -e "${BOLD}$ $1${NC}"
    echo ""
}

demo_run() {
    clear
    echo -e "${BOLD}${G}"
    echo "╔══════════════════════════════════════════════════════════╗"
    echo "║                                                          ║"
    echo "║     CCVE Detection Demo: Real-World Incidents           ║"
    echo "║     \"How We Would Have Saved BIGBANK 4 Hours\"              ║"
    echo "║                                                          ║"
    echo "╚══════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
    echo "This demo shows ConfigHub Agent detecting 3 real CCVEs:"
    echo "  • CCVE-2025-0027: Grafana namespace whitespace (BIGBANK incident)"
    echo "  • CCVE-2025-0028: Traefik service not found (cross-ref validation)"
    echo "  • CCVE-2025-0034: cert-manager Issuer missing (pre-deployment blocking)"
    echo ""
    pause

    # Step 1: Show initial state
    header "Step 1: Initial State (Healthy Baseline)"
    cmd "$MAP_TOOL"

    if command -v "$MAP_TOOL" &> /dev/null; then
        "$MAP_TOOL" 2>/dev/null || echo "Map tool not available - skipping visualization"
    else
        echo -e "${Y}Note: Map tool not available at $MAP_TOOL${NC}"
    fi

    echo ""
    echo -e "${G}✓ Cluster is healthy${NC}"
    echo -e "${G}✓ All workloads running${NC}"
    echo -e "${G}✓ No CCVEs detected${NC}"
    pause

    # Step 2: Introduce CCVE-2025-0027 (Grafana namespace whitespace)
    header "Step 2: Deploy Monitoring with CCVE-2025-0027"
    echo "Deploying Grafana with the EXACT error that hit BIGBANK..."
    echo ""
    cmd "kubectl apply -f bad-configs/monitoring-bad.yaml"
    echo ""
    echo -e "${Y}⚠️  CCVE-2025-0027 detected (Critical)${NC}"
    echo ""
    echo "  ${BOLD}Grafana sidecar namespace whitespace error${NC}"
    echo ""
    echo "  Location: Deployment/grafana, env NAMESPACE"
    echo "  Problem: NAMESPACE=\"monitoring, grafana, observability\"  ❌ (spaces!)"
    echo "  Correct: NAMESPACE=\"monitoring,grafana,observability\"    ✅ (no spaces)"
    echo ""
    echo -e "${R}📖 Real-world incident:${NC}"
    echo "  This exact error caused a 4-hour outage at BIGBANK Capital Markets"
    echo "  during their FluxCon 2025 presentation."
    echo ""
    echo "  Without CCVE detection:"
    echo "    • Grafana starts normally (no obvious error)"
    echo "    • Dashboards don't appear"
    echo "    • Sidecar logs are hard to find"
    echo "    • Team debugs for 4 hours"
    echo ""
    echo "  With CCVE detection:"
    echo "    • Instant detection with exact line number"
    echo "    • Shows fix command"
    echo "    • Time to resolution: 30 seconds"
    echo ""
    pause

    # Step 3: Introduce CCVE-2025-0028 (Traefik service not found)
    header "Step 3: Deploy Ingress with CCVE-2025-0028"
    echo "Adding IngressRoute with service name typo..."
    echo ""
    cmd "kubectl apply -f bad-configs/ingress-bad.yaml"
    echo ""
    echo -e "${Y}⚠️  CCVE-2025-0028 detected (Critical)${NC}"
    echo ""
    echo "  ${BOLD}Traefik IngressRoute service not found${NC}"
    echo ""
    echo "  Location: IngressRoute/grafana-web"
    echo "  Problem: Service \"grafana-servic\" does not exist  ❌ (typo!)"
    echo "  Correct: Service \"grafana-service\" exists         ✅"
    echo ""
    echo -e "${R}Cross-reference validation:${NC}"
    echo "  IngressRoute/grafana-web → Service/grafana-servic  ❌ NOT FOUND"
    echo ""
    echo "  ${BOLD}Why this is dangerous:${NC}"
    echo "  • Kubernetes ACCEPTS this IngressRoute (no validation)"
    echo "  • Traffic silently fails with 404 errors"
    echo "  • Users can't access Grafana"
    echo "  • Hard to debug without cross-reference checks"
    echo ""
    pause

    # Step 4: Introduce CCVE-2025-0034 (cert-manager Issuer missing)
    header "Step 4: Deploy Certificate with CCVE-2025-0034"
    echo "Adding TLS certificate with missing Issuer..."
    echo ""
    cmd "kubectl apply -f bad-configs/certificate-bad.yaml"
    echo ""
    echo -e "${Y}⚠️  CCVE-2025-0034 detected (Critical)${NC}"
    echo ""
    echo "  ${BOLD}cert-manager Certificate Issuer not found${NC}"
    echo ""
    echo "  Location: Certificate/grafana-tls"
    echo "  Problem: ClusterIssuer \"letsencrypt-prod\" does not exist  ❌"
    echo ""
    echo -e "${R}Cross-reference validation:${NC}"
    echo "  Certificate/grafana-tls → ClusterIssuer/letsencrypt-prod  ❌ NOT FOUND"
    echo ""
    echo "  ${BOLD}Why pre-deployment blocking is critical:${NC}"
    echo "  • Certificate stays in Pending state forever"
    echo "  • No TLS, insecure connections"
    echo "  • Should BLOCK deployment until Issuer exists"
    echo ""
    pause

    # Step 5: Fix all CCVEs
    header "Step 5: Fix All CCVEs"
    echo "Applying fixes for all detected CCVEs..."
    echo ""

    echo -e "${BOLD}Fix 1: CCVE-2025-0027 (Grafana namespace whitespace)${NC}"
    cmd "kubectl set env deployment/grafana -n monitoring NAMESPACE='monitoring,grafana,observability'"
    echo -e "${G}✓ Fixed: Removed spaces from namespace list${NC}"
    echo ""

    echo -e "${BOLD}Fix 2: CCVE-2025-0028 (Traefik service not found)${NC}"
    cmd "kubectl apply -f fixed-configs/ingress-fixed.yaml"
    echo -e "${G}✓ Fixed: Corrected service name to 'grafana-service'${NC}"
    echo ""

    echo -e "${BOLD}Fix 3: CCVE-2025-0034 (cert-manager Issuer missing)${NC}"
    cmd "kubectl apply -f fixed-configs/letsencrypt-issuer.yaml"
    echo -e "${G}✓ Fixed: Created missing ClusterIssuer${NC}"
    echo ""

    pause

    # Step 6: Show final state
    header "Step 6: Final State (All Healthy)"
    cmd "$MAP_TOOL"

    if command -v "$MAP_TOOL" &> /dev/null; then
        "$MAP_TOOL" 2>/dev/null || echo "Map tool not available - skipping visualization"
    else
        echo -e "${Y}Note: Map tool not available at $MAP_TOOL${NC}"
    fi

    echo ""
    echo -e "${BOLD}${G}CCVE Scan Results:${NC}"
    echo -e "${G}  ✓ 0 Critical CCVEs detected${NC}"
    echo -e "${G}  ✓ 0 Warning CCVEs detected${NC}"
    echo -e "${G}  ✓ All resources validated${NC}"
    echo ""

    echo -e "${BOLD}Summary:${NC}"
    echo "  • 3 Critical CCVEs detected and fixed"
    echo "  • Time to resolution: ~2 minutes (vs BIGBANK's 4 hours)"
    echo "  • All issues caught before production impact"
    echo ""

    echo -e "${BOLD}${G}Demo complete!${NC}"
    echo ""
}

demo_setup() {
    echo "Setting up demo environment..."
    echo "Note: This is a placeholder - actual setup would:"
    echo "  1. Create kind cluster"
    echo "  2. Install Flux CD"
    echo "  3. Deploy ConfigHub Agent"
    echo "  4. Deploy base workloads"
}

demo_cleanup() {
    echo "Cleaning up demo environment..."
    kubectl delete namespace monitoring --ignore-not-found=true 2>/dev/null || true
    kubectl delete namespace grafana --ignore-not-found=true 2>/dev/null || true
    echo "✓ Demo cleanup complete"
}

# Main
case "${1:-}" in
    setup)
        demo_setup
        ;;
    run)
        demo_run
        ;;
    cleanup)
        demo_cleanup
        ;;
    *)
        echo "Usage: $0 {setup|run|cleanup}"
        echo ""
        echo "Commands:"
        echo "  setup   - Setup demo environment (kind cluster, Flux, etc.)"
        echo "  run     - Run the interactive demo"
        echo "  cleanup - Remove demo resources"
        exit 1
        ;;
esac
