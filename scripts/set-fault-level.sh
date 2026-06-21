#!/usr/bin/env bash
# Apply a fault noise preset and restart workloads that read aiden-demo-fault-profile.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${NAMESPACE:-aiden-demo}"
LEVEL="${1:-}"

usage() {
  cat <<EOF
Usage: $(basename "$0") <quiet|normal|noisy|demo>

Presets (ConfigMap aiden-demo-fault-profile):
  quiet   — chaos monkey off, no random leaf failures
  normal  — default demo noise (45–480s chaos, moderate fractions)
  noisy   — short chaos intervals, high leaf failure rates
  demo    — fastest intervals (10–30s, burst=3) for live audience demos

After apply, restarts chaos-monkey and leaf service deployments so env picks up changes.
EOF
}

if [[ -z "$LEVEL" ]]; then
  usage
  exit 1
fi

PROFILE="$ROOT/k8s/fault-profiles/${LEVEL}.yaml"
if [[ ! -f "$PROFILE" ]]; then
  echo "unknown level: $LEVEL (expected quiet, normal, noisy, or demo)" >&2
  exit 1
fi

echo "==> apply fault profile: $LEVEL"
kubectl apply -f "$PROFILE"

echo "==> restart fault-aware deployments in ${NAMESPACE}"
for dep in aiden-chaos-monkey payment-service product-catalog-service ad-service; do
  kubectl -n "$NAMESPACE" rollout restart "deployment/${dep}"
done

for dep in aiden-chaos-monkey payment-service product-catalog-service ad-service; do
  kubectl -n "$NAMESPACE" rollout status "deployment/${dep}" --timeout=180s
done

echo "fault level ${LEVEL} active (AIDEN_DEMO_FAULT_LEVEL in configmap aiden-demo-fault-profile)"
