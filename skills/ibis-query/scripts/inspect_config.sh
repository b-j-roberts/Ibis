#!/usr/bin/env bash
# inspect_config.sh — Discover ibis config and display available tables/schemas.
# Usage: inspect_config.sh [path-to-ibis.config.yaml]
#
# If no path given, searches common locations.
# Falls back to `ibis query --list` if ibis binary is available.

set -euo pipefail

CONFIG_PATH="${1:-}"

# Search for config if not provided
if [ -z "$CONFIG_PATH" ]; then
  for candidate in \
    "./ibis.config.yaml" \
    "./configs/ibis.config.yaml" \
    "./config/ibis.config.yaml" \
    "../ibis.config.yaml" \
    "../configs/ibis.config.yaml"; do
    if [ -f "$candidate" ]; then
      CONFIG_PATH="$candidate"
      break
    fi
  done
fi

# Try ibis binary first
if command -v ibis &>/dev/null; then
  echo "=== ibis query --list ==="
  if [ -n "$CONFIG_PATH" ]; then
    ibis query --list --config "$CONFIG_PATH" 2>/dev/null || true
  else
    ibis query --list 2>/dev/null || true
  fi
  echo ""
fi

# Show raw config
if [ -n "$CONFIG_PATH" ] && [ -f "$CONFIG_PATH" ]; then
  echo "=== Config: $CONFIG_PATH ==="
  cat "$CONFIG_PATH"
else
  echo "No ibis.config.yaml found."
  echo "Searched: ./ibis.config.yaml, ./configs/ibis.config.yaml, ./config/ibis.config.yaml"
  exit 1
fi
