#!/usr/bin/env bash
# FIXTURE-OWNED: deterministic pinned public-source projection; not a cluster export.
set -euo pipefail
mkdir -p cluster
cat > cluster/argo-publication-lag.yaml <<'CUB_SCOUT_EVAL_EOF'
# Recorded public-source projection, not raw cluster/API output.
projection_format: recorded-markdown-excerpt/v1
source:
  repository: confighub/examples
  revision: 64a6c499ce824d4700a8dfbc1945333dc2e4a1e3
  document: cub-argo/docs/onboard-your-argo-estate.md
  sha256: 44b33edec1378914e29080e774e2aef4a619505a704a3900dd040b44f297aaef
  ranges: [[597, 617]]
  excerpt_sha256: 24f859b8f44fd5c279eed0ee81c301c2df347d8240e6d74746bb8f88910111dd
scope:
  evidence_kind: public recorded run-log or guide excerpt
  snapshot: false
excerpt_lines:
  - source_line: 597
    text: "## A published release does not arrive on its own"
  - source_line: 598
    text: ""
  - source_line: 599
    text: "This one is measured, and it surprised us. After `handover.sh`, an Application"
  - source_line: 600
    text: "reads `oci://<gateway>` at `targetRevision: latest` — and Argo **caches the"
  - source_line: 601
    text: "digest it resolved for that tag**. On Argo CD v3.5.3 a newly published release"
  - source_line: 602
    text: "was still unread ninety seconds later: the Application sat at the previous"
  - source_line: 603
    text: "digest, and the cluster ran the previous replica count."
  - source_line: 604
    text: ""
  - source_line: 605
    text: "A hard refresh re-resolves the tag to a digest, and the release lands at once:"
  - source_line: 606
    text: ""
  - source_line: 607
    text: "```bash"
  - source_line: 608
    text: "kubectl -n argocd annotate application <name> argocd.argoproj.io/refresh=hard --overwrite"
  - source_line: 609
    text: "```"
  - source_line: 610
    text: ""
  - source_line: 611
    text: "```mermaid"
  - source_line: 612
    text: "flowchart LR"
  - source_line: 613
    text: "  p[\"cub release publish\"] --> g[\"ConfigHub gateway<br/>new digest under :latest\"]"
  - source_line: 614
    text: "  g -.->|\"Argo does not notice:<br/>the old digest is cached\"| a[\"Application\"]"
  - source_line: 615
    text: "  g ==>|\"hard refresh<br/>re-resolves tag to digest\"| a"
  - source_line: 616
    text: "  a --> c[\"the cluster\"]"
  - source_line: 617
    text: "```"
CUB_SCOUT_EVAL_EOF
