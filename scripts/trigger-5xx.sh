#!/usr/bin/env bash
# Backward-compatible alias: schema-mismatch 5xx traffic (default fault mode).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export DEMO_FAULT="${DEMO_FAULT:-schema}"
exec "${SCRIPT_DIR}/trigger-fault.sh" "$@"
