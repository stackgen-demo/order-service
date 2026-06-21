#!/usr/bin/env bash
# Deploy full aiden-demo stack: Datadog agent, four microservices, chaos monkey.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${NAMESPACE:-aiden-demo}"

ORDER_REPO="${ORDER_REPO:-$ROOT}"
PAYMENT_REPO="${PAYMENT_REPO:-$(dirname "$ROOT")/payment-service}"
CATALOG_REPO="${CATALOG_REPO:-$(dirname "$ROOT")/product-catalog-service}"
AD_REPO="${AD_REPO:-$(dirname "$ROOT")/ad-service}"

echo "==> apply order-service stack (namespace, agent, order-service, log paths)"
kubectl apply -f "$ORDER_REPO/k8s/stack.yaml"
kubectl -n "$NAMESPACE" delete daemonset datadog-agent --ignore-not-found

echo "==> apply network policies (namespace isolation)"
kubectl apply -f "$ORDER_REPO/k8s/network-policy.yaml"

echo "==> apply shared fault profile (normal noise level)"
kubectl apply -f "$ORDER_REPO/k8s/fault-profiles/normal.yaml"

echo "==> apply satellite services"
kubectl apply -f "$PAYMENT_REPO/k8s/payment-service.yaml"
kubectl apply -f "$CATALOG_REPO/k8s/product-catalog-service.yaml"
kubectl apply -f "$AD_REPO/k8s/ad-service.yaml"

echo "==> apply chaos monkey (replaces suspended CronJob for continuous faults)"
kubectl apply -f "$ORDER_REPO/k8s/chaos-monkey.yaml"
kubectl apply -f "$ORDER_REPO/k8s/trigger-fault-cronjob.yaml"

echo "==> wait for rollouts"
for dep in datadog-agent aiden-demo payment-service product-catalog-service ad-service aiden-chaos-monkey; do
  kubectl -n "$NAMESPACE" rollout status "deployment/${dep}" --timeout=180s
done

echo "aiden-demo stack ready in namespace ${NAMESPACE}"
