#!/usr/bin/env bash
set -euo pipefail

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

set +e
crossplane render tests/render/xr.yaml tests/render/composition.yaml tests/render/functions.yaml \
  --extra-resources tests/render/extra-resources.yaml >"$tmp" 2>&1
status=$?
set -e

if [[ "$status" -eq 0 ]]; then
  echo "crossplane render succeeded, expected failure" >&2
  cat "$tmp" >&2
  exit 1
fi

if ! grep -q "Referenced Service Bus namespace does not allow this environment." "$tmp"; then
  echo "crossplane render failed without expected validation message" >&2
  cat "$tmp" >&2
  exit 1
fi
