#!/usr/bin/env bash
# Apply a real Datadog API key to the aiden-demo namespace (US3 site).
#
# Usage:
#   DD_API_KEY=<your-us3-key> ./scripts/apply-datadog-secret.sh
#   # or:
#   DD_API_KEY_FILE=~/.datadog/us3-api-key ./scripts/apply-datadog-secret.sh
set -euo pipefail

NAMESPACE="${NAMESPACE:-aiden-demo}"
SECRET_NAME="${SECRET_NAME:-datadog-secret}"

api_key="${DD_API_KEY:-}"
if [[ -z "$api_key" && -n "${DD_API_KEY_FILE:-}" && -f "$DD_API_KEY_FILE" ]]; then
  api_key="$(<"$DD_API_KEY_FILE")"
fi

if [[ -z "$api_key" ]]; then
  echo "error: set DD_API_KEY or DD_API_KEY_FILE (32-char key from https://us3.datadoghq.com/organization-settings/api-keys)" >&2
  exit 1
fi

kubectl -n "$NAMESPACE" create secret generic "$SECRET_NAME" \
  --from-literal=api-key="$api_key" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n "$NAMESPACE" delete daemonset datadog-agent --ignore-not-found
kubectl -n "$NAMESPACE" rollout restart deployment/datadog-agent
kubectl -n "$NAMESPACE" rollout status deployment/datadog-agent --timeout=120s
kubectl -n "$NAMESPACE" rollout restart deployment/aiden-demo
kubectl -n "$NAMESPACE" rollout status deployment/aiden-demo --timeout=120s

echo "datadog-secret updated in namespace $NAMESPACE; agent and app restarted."
