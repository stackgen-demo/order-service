#!/usr/bin/env bash
# Live Incident Triage PoC demo for aiden-demo + Datadog + GitHub + SRE app.
#
# Talk track: Datadog monitor fires → SRE app ingests → single-shot investigation →
# RCA + Datadog writeback + policy-gated GitHub fix PR.
#
# Prerequisites (see solutions/examples/scenarios/datadog-aws-rca/README.md):
#   - stackgen-sre-app installed; Datadog + GitHub integrations bound
#   - tofu apply in examples/scenarios/datadog-aws-rca (GitHub merged onto SRE app)
#   - aiden-demo stack deployed; Datadog monitors → @webhook-sabith-datadog-testbed
#   - SRE app Discovery run once (Datadog monitors + GitHub repos → knowledge graph)
#   - Optional: shared:incidents namespace for prior-incident lift demo
#
# Usage:
#   ./scripts/run-incident-triage-demo.sh preflight
#   ./scripts/run-incident-triage-demo.sh resume          # scale workloads up
#   ./scripts/run-incident-triage-demo.sh fire-schema    # predictable 500 (schema mismatch)
#   ./scripts/run-incident-triage-demo.sh fire-noisy     # chaos + leaf faults
#   ./scripts/run-incident-triage-demo.sh fire-rank-commit  # bad commit → rank panic → RCA
#   ./scripts/run-incident-triage-demo.sh commit-rca     # full: resume → fire-rank-commit
#   ./scripts/run-incident-triage-demo.sh urls           # print SRE / Datadog / Guild links
#   ./scripts/run-incident-triage-demo.sh full           # resume → fire-schema → urls
#
# Env (optional):
#   NAMESPACE=aiden-demo
#   SRE_BASE_URL=https://ai.dev.stackgen.com/app/sre
#   GUILD_URL=https://ai.dev.stackgen.com/guild
#   DD_SITE=us3.datadoghq.com
#   TRACKED_SERVICE=order-service
#   TRACKED_ENV=demo
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${NAMESPACE:-aiden-demo}"
SRE_BASE_URL="${SRE_BASE_URL:-https://ai.dev.stackgen.com/app/sre}"
GUILD_URL="${GUILD_URL:-https://ai.dev.stackgen.com/guild}"
DD_SITE="${DD_SITE:-us3.datadoghq.com}"
TRACKED_SERVICE="${TRACKED_SERVICE:-order-service}"
TRACKED_ENV="${TRACKED_ENV:-demo}"

info() { printf '==> %s\n' "$*"; }
warn() { printf 'warn: %s\n' "$*" >&2; }

cmd="${1:-help}"

resume_stack() {
  info "Scaling aiden-demo workloads to 1 replica"
  kubectl -n "$NAMESPACE" scale deployment/datadog-agent deployment/aiden-demo \
    deployment/payment-service deployment/product-catalog-service deployment/ad-service \
    deployment/aiden-chaos-monkey --replicas=1
  kubectl -n "$NAMESPACE" rollout status deployment/datadog-agent --timeout=120s
  kubectl -n "$NAMESPACE" rollout status deployment/aiden-demo --timeout=180s
  ok=1
  for dep in payment-service product-catalog-service ad-service aiden-chaos-monkey; do
    kubectl -n "$NAMESPACE" rollout status "deployment/${dep}" --timeout=120s || ok=0
  done
  if [[ "$ok" != "1" ]]; then
    warn "one or more satellite rollouts did not finish in time"
  fi
}

fire_schema() {
  info "Triggering schema-mismatch 500 (predictable RCA: cmd/initdb/main.go)"
  kubectl -n "$NAMESPACE" run demo-fire-schema --rm -i --restart=Never \
    --image=curlimages/curl:8.5.0 -- \
    curl -sS -o /dev/null -w "HTTP %{http_code}\n" \
      -X POST "http://aiden-demo/api/orders" \
      -H "Content-Type: application/json" \
      -H "X-Demo-Fault: schema" \
      -d '{"customer_email":"demo@example.com","total_amount":42.50}'
}

fire_noisy() {
  info "Setting fault profile to noisy (monitors should fire within minutes)"
  "$ROOT/scripts/set-fault-level.sh" noisy
}

print_urls() {
  cat <<EOF

--- Demo URLs (open before the call) ---

SRE app alerts:
  ${SRE_BASE_URL}/alerts?attention=needs_review

SRE app discovery (run once if stale):
  ${SRE_BASE_URL}/discovery

Datadog monitors (aiden-demo):
  https://${DD_SITE}/monitors/manage?q=aiden-demo

Datadog APM service map:
  https://${DD_SITE}/apm/map?env=${TRACKED_ENV}

Datadog logs (schema mismatch):
  https://${DD_SITE}/logs?query=service%3A${TRACKED_SERVICE}%20env%3A${TRACKED_ENV}%20DatabaseSchemaMismatch

Guild workflows + activity playback:
  ${GUILD_URL}/workflows
  ${GUILD_URL}/executions

GitHub fix target (order-service schema):
  https://github.com/stackgen-demo/order-service/blob/main/cmd/initdb/main.go

--- Narration checklist ---

1. Show Datadog monitor in Alert (or SRE app alert row after webhook ingest).
2. SRE app: alert detail → investigation → RCA (single prompt; no chat back-and-forth).
3. Datadog: RCA event on monitor timeline.
4. Guild: policy approval card → GitHub fix PR (cmd/initdb/main.go).
5. Optional: Memory Explorer (shared:incidents) after bootstrap-memory run.

EOF
}

preflight() {
  info "Checking kubectl namespace and core deployments"
  kubectl -n "$NAMESPACE" get deploy -o wide
  info "Checking NetworkPolicies"
  kubectl -n "$NAMESPACE" get networkpolicy 2>/dev/null || warn "no NetworkPolicy (isolation not applied)"
  info "Checking monitors script presence"
  [[ -x "$ROOT/scripts/apply-datadog-monitors.sh" ]] || warn "apply-datadog-monitors.sh missing"
  print_urls
  warn "Manual checks before customer call:"
  warn "  - Datadog webhook sabith-datadog-testbed URL is valid (no _DELETE_THIS suffix)"
  warn "  - SRE app Discovery completed for Datadog + GitHub"
  warn "  - shared:incidents provisioned if demoing prior-incident lift"
  warn "  - enable_policies=true in datadog-aws-rca if you want Terraform HITL policies"
}

case "$cmd" in
  resume) resume_stack ;;
  fire-schema) fire_schema ;;
  fire-noisy) fire_noisy ;;
  fire-rank-commit)
    info "--- Bad Commit → Alert → Aiden RCA demo ---"
    info "Step 1: Setting demo-speed chaos (10-30s intervals, burst=3)"
    "$ROOT/scripts/set-fault-level.sh" demo

    CATALOG_REPO="${CATALOG_REPO:-$(dirname "$ROOT")/product-catalog-service}"
    if [[ ! -d "$CATALOG_REPO/.git" ]]; then
      warn "Cannot find product-catalog-service repo at $CATALOG_REPO"
      warn "Set CATALOG_REPO env var to the correct path"
      exit 1
    fi

    info "Step 2: Merging feat/premium-rank-comparison into current branch"
    info "  (re-introduces off-by-one in enrichProductWithRank)"
    (cd "$CATALOG_REPO" && git merge feat/premium-rank-comparison --no-edit)

    info "Step 3: Restarting product-catalog-service to pick up the 'bad' code"
    info "  (in production this would happen via CI → image push → rolling update)"
    kubectl -n "$NAMESPACE" rollout restart deployment/product-catalog-service
    kubectl -n "$NAMESPACE" rollout status deployment/product-catalog-service --timeout=180s

    info "Step 4: Chaos monkey will trigger GetProduct within 10-30s"
    info "  → CatalogRankIndexPanic will appear in Datadog logs"
    info "  → Monitor '[aiden-demo] product-catalog-service rank panic logs' will fire"
    info "  → Webhook → SRE app → auto-investigate → Aiden RCA"
    print_urls
    cat <<EOF

--- Commit-to-RCA narration checklist ---

1. Show the commit diff: "feat: bias rank comparison toward premium-tier catalog entries"
   → Innocent-looking +1 offset in enrichProductWithRank (PROD-4102)

2. Watch Datadog Log Explorer for CatalogRankIndexPanic (within 30s)
   https://${DD_SITE}/logs?query=service%3Aproduct-catalog-service%20env%3A${TRACKED_ENV}%20CatalogRankIndexPanic

3. Watch Datadog monitor transition to ALERT
   https://${DD_SITE}/monitors/manage?q=aiden-demo%20rank

4. SRE app: alert row appears → investigation auto-launches
   ${SRE_BASE_URL}/alerts?attention=needs_review

5. Aiden (Guild) investigates:
   - Queries Datadog logs + traces → finds CatalogRankIndexPanic
   - Queries GitHub for recent commits → finds the rank comparison commit
   - hypothesis-deploy-regression: "Commit introduced off-by-one in rank index"
   ${GUILD_URL}/executions

6. Key talking point: "Aiden traced the alert back to the exact commit and line of code."

EOF
    ;;
  commit-rca)
    info "Full commit-to-RCA flow: resume → fire-rank-commit"
    resume_stack
    # small pause so pods are fully ready before merging bad code
    sleep 5
    "$0" fire-rank-commit
    ;;
  urls) print_urls ;;
  preflight) preflight ;;
  full)
    resume_stack
    fire_schema
    print_urls
    ;;
  help|*)
    sed -n '2,20p' "$0" | sed 's/^# \?//'
    ;;
esac
