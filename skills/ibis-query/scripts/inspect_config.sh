#!/usr/bin/env bash
# inspect_config.sh — Discover ibis config and display available tables/schemas.
# Usage: inspect_config.sh [--json] [path-to-ibis.config.yaml]
#
# If no path given, searches common locations.
# Falls back to `ibis query --list` if ibis binary is available.
# Use --json for machine-readable JSON output.

set -euo pipefail

JSON_MODE=false
CONFIG_PATH=""

# Parse arguments
for arg in "$@"; do
  case "$arg" in
    --json) JSON_MODE=true ;;
    *) CONFIG_PATH="$arg" ;;
  esac
done

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

# Show parsed config
if [ -n "$CONFIG_PATH" ] && [ -f "$CONFIG_PATH" ]; then
  echo "=== Config: $CONFIG_PATH ==="
  echo ""

  # Strip comments for reliable parsing
  CLEAN=$(sed 's/#.*//' "$CONFIG_PATH")

  if $JSON_MODE; then
    # JSON output using python for reliable YAML-comment-stripped parsing
    echo "$CLEAN" | python3 -c '
import sys, json, re

# Simple line-by-line state machine parser
lines = sys.stdin.readlines()
result = {"contracts": [], "views": [], "discovery": [], "factories": []}

contract = None
in_events = False
in_views = False
in_factory = False
in_discover = False
event = None
view = None
discover = None

for line in lines:
    stripped = line.rstrip()
    if not stripped:
        continue

    indent = len(line) - len(line.lstrip())

    # Top level
    if indent == 0 and stripped.startswith("contracts:"):
        in_discover = False
        continue
    if indent == 0 and stripped.startswith("discover:"):
        in_discover = True
        continue
    if indent == 0 and not stripped.startswith(" "):
        in_discover = False
        continue

    # Discovery entries
    if in_discover and indent == 2 and "class_hash:" in stripped:
        val = stripped.split("class_hash:")[-1].strip().strip("\"")
        discover = {"class_hash": val}
        result["discovery"].append(discover)
        continue
    if in_discover and discover and "group:" in stripped:
        discover["group"] = stripped.split("group:")[-1].strip().strip("\"")
        continue
    if in_discover and discover and "shared_tables:" in stripped:
        discover["shared_tables"] = "true" in stripped
        continue

    # Contract level (indent 2 = "  - name:", indent 4 = "    address:")
    if "- name:" in stripped and indent <= 4 and not in_discover:
        name = stripped.split("- name:")[-1].strip().strip("\"")
        contract = {"name": name, "events": [], "address": ""}
        result["contracts"].append(contract)
        in_events = False
        in_views = False
        in_factory = False
        continue
    if contract and "address:" in stripped and indent <= 6 and not in_discover:
        contract["address"] = stripped.split("address:")[-1].strip().strip("\"")
        continue

    # Events
    if contract and stripped.strip() == "events:" and not in_discover:
        in_events = True
        in_views = False
        in_factory = False
        continue
    if contract and stripped.strip() == "views:" and not in_discover:
        in_views = True
        in_events = False
        in_factory = False
        continue
    if contract and stripped.strip() == "factory:" and not in_discover:
        in_factory = True
        in_events = False
        in_views = False
        continue

    if in_events and "- name:" in stripped:
        ename = stripped.split("- name:")[-1].strip().strip("\"")
        event = {"name": ename}
        contract["events"].append(event)
        continue
    if in_events and event and "type:" in stripped:
        event["type"] = stripped.split("type:")[-1].strip().strip("\"")
        continue

    # Views
    if in_views and "- function:" in stripped:
        fname = stripped.split("- function:")[-1].strip().strip("\"")
        view = {"contract": contract["name"], "function": fname}
        result["views"].append(view)
        continue
    if in_views and view and "interval:" in stripped:
        view["interval"] = stripped.split("interval:")[-1].strip().strip("\"")
        continue

    # Factory
    if in_factory and "event:" in stripped:
        factory = {"contract": contract["name"]}
        factory["event"] = stripped.split("event:")[-1].strip().strip("\"")
        result["factories"].append(factory)
        continue
    if in_factory and "shared_tables:" in stripped:
        if result["factories"]:
            result["factories"][-1]["shared_tables"] = "true" in stripped
        continue

print(json.dumps(result, indent=2))
' 2>/dev/null || echo '{"error": "python3 required for --json mode"}'
  else
    # Human-readable output mode

    # === Contracts ===
    echo "=== Contracts ==="
    echo "$CLEAN" | awk '
    BEGIN { in_contracts=0; in_events=0; pending_event="" }
    /^contracts:/ { in_contracts=1; next }
    /^[a-z]/ { in_contracts=0 }
    !in_contracts { next }

    # Contract entry
    /^  - name:/ {
      gsub(/^  - name: */, ""); gsub(/"/, "")
      printf "\n  %s\n", $0
      in_events=0
      next
    }
    /^    address:/ {
      gsub(/^    address: */, ""); gsub(/"/, "")
      printf "    address: %s\n", $0
      next
    }
    /^    events:/ { in_events=1; next }
    /^    [a-z]/ && !/^      / { in_events=0 }

    # Event entry — capture name, print with type on the type: line
    in_events && /^      - name:/ {
      gsub(/^      - name: */, ""); gsub(/"/, "")
      pending_event=$0
      next
    }
    in_events && pending_event != "" && /type:/ {
      gsub(/.*type: */, ""); gsub(/"/, "")
      printf "    event: %s (%s)\n", pending_event, $0
      pending_event=""
      next
    }
    '
    echo ""

    # === View Functions ===
    echo "=== View Functions ==="
    echo "$CLEAN" | awk '
    BEGIN { found=0; in_views=0; current_contract=""; pending_func="" }
    /^  - name:/ { gsub(/^  - name: */, ""); gsub(/"/, ""); current_contract=$0 }
    /^    views:/ { in_views=1; next }
    /^    [a-z]/ && !/^      / { in_views=0 }
    in_views && /- function:/ {
      gsub(/.*- function: */, ""); gsub(/"/, "")
      pending_func=$0
      found=1
      next
    }
    in_views && pending_func != "" && /interval:/ {
      gsub(/.*interval: */, ""); gsub(/"/, "")
      printf "  %s.%s (every %s)\n", current_contract, pending_func, $0
      pending_func=""
      next
    }
    END { if (!found) print "  (none)" }
    '
    echo ""

    # === Discovery ===
    echo "=== Discovery ==="
    echo "$CLEAN" | awk '
    BEGIN { found=0; in_discover=0 }
    /^discover:/ { in_discover=1; next }
    /^[a-z]/ && !/^  / { in_discover=0 }
    in_discover && /class_hash:/ {
      gsub(/.*class_hash: */, ""); gsub(/"/, "")
      if (found) printf "\n"
      printf "  class_hash: %s", $0
      found=1
    }
    in_discover && /group:/ {
      gsub(/.*group: */, ""); gsub(/"/, "")
      printf " (group: %s)", $0
    }
    in_discover && /shared_tables: *true/ {
      printf " [shared]"
    }
    END { if (!found) print "  (none)"; else printf "\n" }
    '
    echo ""

    # === Factories ===
    echo "=== Factories ==="
    echo "$CLEAN" | awk '
    BEGIN { found=0; current_contract=""; in_factory=0; fac_event="" }
    /^  - name:/ { gsub(/^  - name: */, ""); gsub(/"/, ""); current_contract=$0; in_factory=0 }
    /^    factory:/ { in_factory=1; fac_event=""; fac_shared=0; next }
    /^    [a-z]/ && !/^      / && in_factory {
      if (fac_event != "") {
        printf "  %s (event: %s", current_contract, fac_event
        if (fac_shared) printf ", shared"
        printf ")\n"
        found=1
      }
      in_factory=0
    }
    in_factory && /event:/ && !/child_events/ {
      gsub(/.*event: */, ""); gsub(/"/, "")
      fac_event=$0
    }
    in_factory && /shared_tables: *true/ {
      fac_shared=1
    }
    END {
      if (in_factory && fac_event != "") {
        printf "  %s (event: %s", current_contract, fac_event
        if (fac_shared) printf ", shared"
        printf ")\n"
        found=1
      }
      if (!found) print "  (none)"
    }
    '
    echo ""
  fi
else
  echo "No ibis.config.yaml found."
  echo "Searched: ./ibis.config.yaml, ./configs/ibis.config.yaml, ./config/ibis.config.yaml"
  exit 1
fi
