#!/usr/bin/env bash
# Restart deployments after editing aiden-demo-fault-profile (kubectl edit configmap ...).
set -euo pipefail

NAMESPACE="${NAMESPACE:-aiden-demo}"

for dep in aiden-chaos-monkey payment-service product-catalog-service ad-service; do
  kubectl -n "$NAMESPACE" rollout restart "deployment/${dep}"
done

for dep in aiden-chaos-monkey payment-service product-catalog-service ad-service; do
  kubectl -n "$NAMESPACE" rollout status "deployment/${dep}" --timeout=180s
done

kubectl -n "$NAMESPACE" get configmap aiden-demo-fault-profile -o jsonpath='{.data.AIDEN_DEMO_FAULT_LEVEL}{"\n"}' \
  | xargs -I{} echo "reloaded fault profile (level={})"
