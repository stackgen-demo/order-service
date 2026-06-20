#!/usr/bin/env bash
# Build, publish, and roll out order-service to aiden-demo (EKS).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
IMAGE="${IMAGE:-ghcr.io/stackgen-demo/order-service:latest}"
NAMESPACE="${NAMESPACE:-aiden-demo}"

cd "$ROOT"

echo "==> go test"
go test ./...

echo "==> docker build ${IMAGE}"
docker build -t "$IMAGE" .

echo "==> docker push ${IMAGE}"
docker push "$IMAGE"

echo "==> kubectl apply stack"
kubectl apply -f k8s/stack.yaml
kubectl -n "$NAMESPACE" delete deployment datadog-agent --ignore-not-found

echo "==> rollout aiden-demo"
kubectl -n "$NAMESPACE" rollout restart deployment/aiden-demo
kubectl -n "$NAMESPACE" rollout status deployment/aiden-demo --timeout=180s

if kubectl -n "$NAMESPACE" get daemonset datadog-agent >/dev/null 2>&1; then
  kubectl -n "$NAMESPACE" rollout status daemonset/datadog-agent --timeout=180s
fi

echo "Deployed ${IMAGE} to namespace ${NAMESPACE}"
