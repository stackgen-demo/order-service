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
kubectl -n "$NAMESPACE" delete daemonset datadog-agent --ignore-not-found

echo "==> rollout agent + aiden-demo"
kubectl -n "$NAMESPACE" rollout restart deployment/datadog-agent deployment/aiden-demo
kubectl -n "$NAMESPACE" rollout status deployment/datadog-agent --timeout=120s
kubectl -n "$NAMESPACE" rollout status deployment/aiden-demo --timeout=180s

echo "Deployed ${IMAGE} to namespace ${NAMESPACE}"
