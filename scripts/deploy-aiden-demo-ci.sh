#!/usr/bin/env bash
# Roll out a CI-built order-service image tag to aiden-demo (no local docker build).
# Sets DD_VERSION and deployment annotations so investigations can correlate running image → git SHA.
set -euo pipefail

NAMESPACE="${NAMESPACE:-aiden-demo}"
BRANCH="${BRANCH:-main}"
IMAGE="${IMAGE:-ghcr.io/stackgen-demo/order-service:latest}"
CONTAINER="${CONTAINER:-aiden-demo}"
REPO="${REPO:-stackgen-demo/order-service}"

echo "==> set deployment/${CONTAINER} image to ${IMAGE}"
kubectl -n "$NAMESPACE" set image "deployment/${CONTAINER}" "${CONTAINER}=${IMAGE}"
kubectl -n "$NAMESPACE" rollout status "deployment/${CONTAINER}" --timeout=300s

GIT_SHA="${GIT_SHA:-}"
if [[ -z "$GIT_SHA" ]] && command -v gh >/dev/null 2>&1; then
  GIT_SHA="$(gh api "/repos/${REPO}/commits?sha=${BRANCH}&per_page=1" --jq '.[0].sha' 2>/dev/null | cut -c1-12 || true)"
fi
if [[ -z "$GIT_SHA" ]]; then
  GIT_SHA="${BRANCH}"
fi

echo "==> tag deployment with git revision ${GIT_SHA} (DD_VERSION + annotations)"
kubectl -n "$NAMESPACE" set env "deployment/${CONTAINER}" "DD_VERSION=${GIT_SHA}"
kubectl -n "$NAMESPACE" annotate "deployment/${CONTAINER}" \
  "stackgen.demo/git-revision=${GIT_SHA}" \
  "stackgen.demo/image=${IMAGE}" \
  --overwrite
kubectl -n "$NAMESPACE" rollout status "deployment/${CONTAINER}" --timeout=300s

kubectl -n "$NAMESPACE" get pods -l "app=${CONTAINER}" -o wide
echo "Deployed ${IMAGE} to ${NAMESPACE} (DD_VERSION=${GIT_SHA})"
