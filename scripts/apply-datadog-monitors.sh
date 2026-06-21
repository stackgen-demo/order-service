#!/usr/bin/env bash
# Create or update Datadog monitors for the aiden-demo microservices stack (US3).
# All alerts notify @webhook-sabith-datadog-testbed.
#
# Usage:
#   DD_API_KEY=<us3-api-key> DD_APP_KEY=<us3-app-key> ./scripts/apply-datadog-monitors.sh
#   # or:
#   DD_API_KEY_FILE=~/.datadog/us3-api-key DD_APP_KEY_FILE=~/.datadog/us3-app-key ./scripts/apply-datadog-monitors.sh
set -euo pipefail

DD_SITE="${DD_SITE:-api.us3.datadoghq.com}"
WEBHOOK_HANDLE="${WEBHOOK_HANDLE:-@webhook-sabith-datadog-testbed}"
MONITOR_PREFIX="${MONITOR_PREFIX:-[aiden-demo]}"
ENV_TAG="${ENV_TAG:-demo}"

api_key="${DD_API_KEY:-}"
app_key="${DD_APP_KEY:-}"
if [[ -z "$api_key" && -n "${DD_API_KEY_FILE:-}" && -f "$DD_API_KEY_FILE" ]]; then
  api_key="$(<"$DD_API_KEY_FILE")"
fi
if [[ -z "$app_key" && -n "${DD_APP_KEY_FILE:-}" && -f "$DD_APP_KEY_FILE" ]]; then
  app_key="$(<"$DD_APP_KEY_FILE")"
fi

if [[ -z "$api_key" || -z "$app_key" ]]; then
  echo "error: set DD_API_KEY and DD_APP_KEY (or DD_*_FILE paths)" >&2
  echo "  API keys: https://us3.datadoghq.com/organization-settings/api-keys" >&2
  echo "  App keys: https://us3.datadoghq.com/organization-settings/application-keys" >&2
  exit 1
fi

export DD_API_KEY="$api_key" DD_APP_KEY="$app_key" DD_SITE WEBHOOK_HANDLE MONITOR_PREFIX ENV_TAG

python3 <<'PY'
import json
import os
import sys
import urllib.error
import urllib.request

API_KEY = os.environ["DD_API_KEY"]
APP_KEY = os.environ["DD_APP_KEY"]
SITE = os.environ["DD_SITE"]
WEBHOOK = os.environ["WEBHOOK_HANDLE"]
PREFIX = os.environ["MONITOR_PREFIX"]
ENV = os.environ["ENV_TAG"]

COMMON_TAGS = ["aiden-demo", f"env:{ENV}", "managed-by:apply-datadog-monitors"]
DEFAULT_OPTIONS = {
    "notify_audit": False,
    "require_full_window": False,
    "notify_no_data": False,
    "include_tags": True,
    "evaluation_delay": 60,
    "new_host_delay": 300,
    "renotify_interval": 0,
}


def api(method: str, path: str, body=None):
    url = f"https://{SITE}{path}"
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(
        url,
        data=data,
        method=method,
        headers={
            "DD-API-KEY": API_KEY,
            "DD-APPLICATION-KEY": APP_KEY,
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read().decode()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        detail = exc.read().decode()
        raise RuntimeError(f"{method} {path} failed ({exc.code}): {detail}") from exc


def monitor_message(service: str, detail: str) -> str:
    return (
        f"{service} alert in aiden-demo ({detail}).\n\n"
        f"Env: `{ENV}`\n"
        f"Notify: {WEBHOOK}\n\n"
        f"{WEBHOOK}"
    )


MONITORS = [
    {
        "name": f"{PREFIX} order-service HTTP errors",
        "type": "metric alert",
        "query": f"sum(last_5m):sum:trace.http.request.errors{{service:order-service,env:{ENV}}}.as_count() > 3",
        "message": monitor_message("order-service", "HTTP 5xx / error traces"),
        "tags": COMMON_TAGS + ["service:order-service"],
        "options": {**DEFAULT_OPTIONS, "thresholds": {"critical": 3}},
    },
    {
        "name": f"{PREFIX} order-service schema mismatch logs",
        "type": "log alert",
        "query": f'logs("service:order-service env:{ENV} DatabaseSchemaMismatch").index("*").rollup("count").last("5m") > 0',
        "message": monitor_message("order-service", "DatabaseSchemaMismatch in logs"),
        "tags": COMMON_TAGS + ["service:order-service"],
        "options": {**DEFAULT_OPTIONS, "thresholds": {"critical": 0}},
    },
    {
        "name": f"{PREFIX} payment-service gRPC errors",
        "type": "metric alert",
        "query": f"sum(last_5m):sum:trace.grpc.server.errors{{service:payment-service,env:{ENV}}}.as_count() > 0",
        "message": monitor_message("payment-service", "gRPC server errors"),
        "tags": COMMON_TAGS + ["service:payment-service"],
        "options": {**DEFAULT_OPTIONS, "thresholds": {"critical": 0}},
    },
    {
        "name": f"{PREFIX} payment-service PaymentFailure logs",
        "type": "log alert",
        "query": f'logs("service:payment-service env:{ENV} PaymentFailure").index("*").rollup("count").last("5m") > 0',
        "message": monitor_message("payment-service", "PaymentFailure in logs"),
        "tags": COMMON_TAGS + ["service:payment-service"],
        "options": {**DEFAULT_OPTIONS, "thresholds": {"critical": 0}},
    },
    {
        "name": f"{PREFIX} product-catalog-service gRPC errors",
        "type": "metric alert",
        "query": f"sum(last_5m):sum:trace.grpc.server.errors{{service:product-catalog-service,env:{ENV}}}.as_count() > 0",
        "message": monitor_message("product-catalog-service", "gRPC server errors"),
        "tags": COMMON_TAGS + ["service:product-catalog-service"],
        "options": {**DEFAULT_OPTIONS, "thresholds": {"critical": 0}},
    },
    {
        "name": f"{PREFIX} product-catalog-service rank panic logs",
        "type": "log alert",
        "query": f'logs("service:product-catalog-service env:{ENV} CatalogRankIndexPanic").index("*").rollup("count").last("5m") > 0',
        "message": monitor_message("product-catalog-service", "CatalogRankIndexPanic in logs"),
        "tags": COMMON_TAGS + ["service:product-catalog-service"],
        "options": {**DEFAULT_OPTIONS, "thresholds": {"critical": 0}},
    },
    {
        "name": f"{PREFIX} ad-service unavailable logs",
        "type": "log alert",
        "query": f'logs("service:ad-service env:{ENV} AdServiceUnavailable").index("*").rollup("count").last("5m") > 0',
        "message": monitor_message("ad-service", "AdServiceUnavailable in logs"),
        "tags": COMMON_TAGS + ["service:ad-service"],
        "options": {**DEFAULT_OPTIONS, "thresholds": {"critical": 0}},
    },
    {
        "name": f"{PREFIX} aiden-chaos-monkey error logs",
        "type": "log alert",
        "query": f'logs("service:aiden-chaos-monkey env:{ENV} status:error").index("*").rollup("count").last("5m") > 0',
        "message": monitor_message("aiden-chaos-monkey", "error logs from chaos checkout"),
        "tags": COMMON_TAGS + ["service:aiden-chaos-monkey"],
        "options": {**DEFAULT_OPTIONS, "thresholds": {"critical": 0}},
    },
]


existing = {m["name"]: m for m in api("GET", "/api/v1/monitor")}

created = 0
updated = 0
for spec in MONITORS:
    name = spec["name"]
    if name in existing:
        mid = existing[name]["id"]
        api("PUT", f"/api/v1/monitor/{mid}", spec)
        print(f"updated monitor {mid}: {name}")
        updated += 1
        continue
    result = api("POST", "/api/v1/monitor", spec)
    print(f"created monitor {result['id']}: {name}")
    created += 1

print(f"done: {created} created, {updated} updated ({len(MONITORS)} total)")
PY

echo "Datadog monitors applied (site: ${DD_SITE}, webhook: ${WEBHOOK_HANDLE})."
