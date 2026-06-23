#!/usr/bin/env bash
# Roll out a CI-built order-service image tag to aiden-demo (no local docker build).
set -euo pipefail

NAMESPACE="${NAMESPACE:-aiden-demo}"
BRANCH="${BRANCH:-feat-order-customer-persist}"
IMAGE="${IMAGE:-ghcr.io/stackgen-demo/order-service:${BRANCH}}"
CONTAINER="${CONTAINER:-aiden-demo}"

echo "==> set deployment/${CONTAINER} image to ${IMAGE}"
kubectl -n "$NAMESPACE" set image "deployment/${CONTAINER}" "${CONTAINER}=${IMAGE}"
kubectl -n "$NAMESPACE" rollout status "deployment/${CONTAINER}" --timeout=300s
kubectl -n "$NAMESPACE" get pods -l "app=${CONTAINER}" -o wide
echo "Deployed ${IMAGE} to ${NAMESPACE}"
