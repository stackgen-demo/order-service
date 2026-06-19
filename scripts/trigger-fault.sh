#!/usr/bin/env bash
# Send POST /api/orders with X-Demo-Fault for demo and RCA scenarios.
#
# Usage:
#   DEMO_FAULT=schema ./scripts/trigger-fault.sh
#   DEMO_FAULT=dependency BASE_URL=http://localhost:3005 ./scripts/trigger-fault.sh
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:3005}"
REQUESTS="${REQUESTS:-10}"
INTERVAL_MS="${INTERVAL_MS:-1000}"
DEMO_FAULT="${DEMO_FAULT:-schema}"

health_url="${BASE_URL}/health"
health_body="$(curl -sf "$health_url" 2>/dev/null || true)"

if [[ -z "$health_body" ]]; then
  echo "Error: Cannot reach ${health_url}. Start the service first (docker compose up or make run)."
  exit 1
fi

if ! echo "$health_body" | grep -q '"service":"order-service"'; then
  echo "Error: ${health_url} is not order-service. Check BASE_URL."
  exit 1
fi

echo "Triggering ${REQUESTS} POST /api/orders with X-Demo-Fault: ${DEMO_FAULT} against ${BASE_URL}"
echo

error_count=0
success_count=0
other_count=0

for ((i = 1; i <= REQUESTS; i++)); do
  status="$(curl -s -o /tmp/trigger-body.json -w "%{http_code}" \
    -X POST "${BASE_URL}/api/orders" \
    -H "Content-Type: application/json" \
    -H "X-Demo-Fault: ${DEMO_FAULT}" \
    -d "{\"customer_email\":\"test${i}@example.com\",\"total_amount\":$((20 + i)).99,\"status\":\"pending\"}")"

  if [[ "$status" -ge 500 ]]; then
    error_count=$((error_count + 1))
    message="$(python3 -c 'import json,sys; print(json.load(open("/tmp/trigger-body.json")).get("error",""))' 2>/dev/null || true)"
    echo "[${i}/${REQUESTS}] HTTP ${status} - ${message}"
  elif [[ "$status" -ge 200 && "$status" -lt 300 ]]; then
    success_count=$((success_count + 1))
    echo "[${i}/${REQUESTS}] HTTP ${status} - success"
  else
    other_count=$((other_count + 1))
    message="$(python3 -c 'import json,sys; print(json.load(open("/tmp/trigger-body.json")).get("error",""))' 2>/dev/null || true)"
    echo "[${i}/${REQUESTS}] HTTP ${status} - ${message}"
  fi

  if [[ "$i" -lt "$REQUESTS" ]]; then
    sleep "$(python3 -c "print(${INTERVAL_MS}/1000)")"
  fi
done

echo
echo "Done. 5xx: ${error_count}, 2xx: ${success_count}, other: ${other_count}"
