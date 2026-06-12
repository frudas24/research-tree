#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <node_id> [artifact_path] [host]" >&2
  exit 1
fi

NODE_ID="$1"
ARTIFACT_PATH="${2:-}"
HOST="${3:-local}"

if [[ -n "${ARTIFACT_PATH}" ]]; then
  rt node link "${NODE_ID}" --commit auto --artifact "${ARTIFACT_PATH}" --host "${HOST}"
else
  rt node link "${NODE_ID}" --commit auto
fi

