#!/usr/bin/env python3
"""Parse a Starknet ABI JSON and extract event definitions and view function candidates.

Usage: parse_events.py <abi.json>
       cat abi.json | parse_events.py -

Output: structured JSON with events, views, and summary suitable for config generation.
"""

import sys
import json
import re

# Cairo type categories for table type recommendations
NUMERIC_TYPES = {
    "core::integer::u8", "core::integer::u16", "core::integer::u32",
    "core::integer::u64", "core::integer::u128", "core::integer::u256",
    "core::integer::i8", "core::integer::i16", "core::integer::i32",
    "core::integer::i64", "core::integer::i128",
    "u8", "u16", "u32", "u64", "u128", "u256",
    "i8", "i16", "i32", "i64", "i128",
}

ADDRESS_TYPES = {
    "core::starknet::contract_address::ContractAddress",
    "ContractAddress",
}

# SQL column type mapping
TYPE_MAP = {
    "core::felt252": "string (hex)",
    "felt252": "string (hex)",
    "core::integer::u8": "int64",
    "core::integer::u16": "int64",
    "core::integer::u32": "int64",
    "core::integer::u64": "int64",
    "core::integer::u128": "string (big number)",
    "core::integer::u256": "string (big number)",
    "core::integer::i8": "int64",
    "core::integer::i16": "int64",
    "core::integer::i32": "int64",
    "core::integer::i64": "int64",
    "core::integer::i128": "string (big number)",
    "core::bool": "bool",
    "core::starknet::contract_address::ContractAddress": "string (hex)",
    "core::starknet::class_hash::ClassHash": "string (hex)",
    "core::byte_array::ByteArray": "string",
}

# Keywords suggesting fast polling intervals
FAST_POLL_KEYWORDS = {"price", "rate", "exchange", "oracle", "feed", "tick"}
SLOW_POLL_KEYWORDS = {"config", "owner", "admin", "name", "symbol", "decimals", "metadata"}


def short_type(full_type: str) -> str:
    """Extract short type name from fully qualified Cairo type."""
    # Handle tuple types: (core::integer::u128, core::integer::u256) -> (u128, u256)
    if is_tuple_type(full_type):
        inner = full_type.strip()[1:-1]  # Remove parens
        parts = [short_type(p.strip()) for p in _split_tuple_elements(inner)]
        return "(" + ", ".join(parts) + ")"
    if "::" in full_type:
        return full_type.split("::")[-1]
    return full_type


def _split_tuple_elements(s: str) -> list:
    """Split tuple inner string by commas, respecting nested parens."""
    parts = []
    depth = 0
    current = []
    for ch in s:
        if ch == '(':
            depth += 1
            current.append(ch)
        elif ch == ')':
            depth -= 1
            current.append(ch)
        elif ch == ',' and depth == 0:
            parts.append(''.join(current))
            current = []
        else:
            current.append(ch)
    if current:
        parts.append(''.join(current))
    return parts


def is_tuple_type(cairo_type: str) -> bool:
    """Check if a Cairo type is a tuple: (T1, T2, ...)."""
    stripped = cairo_type.strip()
    return stripped.startswith("(") and stripped.endswith(")")


def get_sql_type(cairo_type: str) -> str:
    """Map Cairo type to ibis column type."""
    if cairo_type in TYPE_MAP:
        return TYPE_MAP[cairo_type]
    if is_tuple_type(cairo_type):
        return "string (JSON)"
    if "Array" in cairo_type or "Span" in cairo_type:
        return "string (JSON array)"
    return "string (JSON)"


def is_numeric(cairo_type: str) -> bool:
    return cairo_type in NUMERIC_TYPES or any(cairo_type.endswith(f"::{t}") for t in
        ["u8","u16","u32","u64","u128","u256","i8","i16","i32","i64","i128"])


def is_address(cairo_type: str) -> bool:
    return cairo_type in ADDRESS_TYPES or cairo_type.endswith("ContractAddress")


def parse_abi(abi_data):
    """Parse ABI and extract events with their fields."""
    # Build type registry for struct/enum resolution
    type_registry = {}
    for item in abi_data:
        if item.get("type") in ("struct", "enum"):
            type_registry[item.get("name", "")] = item

    events = []
    for item in abi_data:
        if item.get("type") != "event":
            continue
        if item.get("kind") != "struct":
            continue

        name = item.get("name", "")
        short_name = short_type(name)
        members = item.get("members", [])

        key_fields = []
        data_fields = []
        has_numeric = False
        has_address = False
        address_fields = []
        numeric_fields = []

        for member in members:
            field_name = member.get("name", "")
            field_type = member.get("type", "")
            field_kind = member.get("kind", "data")

            field_info = {
                "name": field_name,
                "cairo_type": short_type(field_type),
                "full_type": field_type,
                "sql_type": get_sql_type(field_type),
                "is_numeric": is_numeric(field_type),
                "is_address": is_address(field_type),
                "is_tuple": is_tuple_type(field_type),
            }

            if field_kind == "key":
                key_fields.append(field_info)
            else:
                data_fields.append(field_info)

            if is_numeric(field_type):
                has_numeric = True
                numeric_fields.append(field_name)
            if is_address(field_type):
                has_address = True
                address_fields.append(field_name)

        events.append({
            "name": short_name,
            "full_name": name,
            "key_fields": key_fields,
            "data_fields": data_fields,
            "all_fields": key_fields + data_fields,
            "has_numeric": has_numeric,
            "has_address": has_address,
            "numeric_fields": numeric_fields,
            "address_fields": address_fields,
            "field_count": len(key_fields) + len(data_fields),
        })

    return events


def parse_views(abi_data):
    """Parse ABI and extract view function candidates."""
    views = []

    for item in abi_data:
        # Look for interface items that contain functions
        if item.get("type") == "interface":
            for fn in item.get("items", []):
                if fn.get("type") == "function" and fn.get("state_mutability") == "view":
                    views.append(_extract_view(fn))
        # Also check top-level functions (some ABIs flatten them)
        elif item.get("type") == "function" and item.get("state_mutability") == "view":
            views.append(_extract_view(item))

    return views


def _extract_view(fn):
    """Extract view function details from an ABI function entry."""
    name = fn.get("name", "")
    inputs = fn.get("inputs", [])
    outputs = fn.get("outputs", [])

    input_fields = []
    for inp in inputs:
        inp_type = inp.get("type", "")
        input_fields.append({
            "name": inp.get("name", ""),
            "cairo_type": short_type(inp_type),
            "full_type": inp_type,
            "sql_type": get_sql_type(inp_type),
            "is_tuple": is_tuple_type(inp_type),
        })

    output_fields = []
    for out in outputs:
        out_type = out.get("type", "")
        output_fields.append({
            "name": out.get("name", "") or "result",
            "cairo_type": short_type(out_type),
            "full_type": out_type,
            "sql_type": get_sql_type(out_type),
            "is_tuple": is_tuple_type(out_type),
        })

    # Recommend polling interval based on function name
    interval = recommend_view_interval(name)

    # Recommend table type: unique with _view_key for simple getters, log for complex
    has_inputs = len(input_fields) > 0
    recommended_table = "unique" if not has_inputs else "log"
    recommended_key = "_view_key" if not has_inputs else None

    return {
        "name": name,
        "inputs": input_fields,
        "outputs": output_fields,
        "has_inputs": has_inputs,
        "input_count": len(input_fields),
        "output_count": len(output_fields),
        "recommended_interval": interval,
        "recommended_table_type": recommended_table,
        "recommended_unique_key": recommended_key,
        "is_good_candidate": _is_good_view_candidate(name, input_fields, output_fields),
    }


def recommend_view_interval(name: str) -> str:
    """Recommend polling interval based on view function name."""
    name_lower = name.lower()

    # Fast: price-like data
    if any(kw in name_lower for kw in FAST_POLL_KEYWORDS):
        return "5s"

    # Slow: rarely changing state
    if any(kw in name_lower for kw in SLOW_POLL_KEYWORDS):
        return "5m"

    # Default: 30s for most views
    return "30s"


def _is_good_view_candidate(name: str, inputs: list, outputs: list) -> bool:
    """Determine if a view function is a good polling candidate.

    Best candidates: no inputs or simple felt inputs, with meaningful outputs.
    Poor candidates: complex struct inputs, pagination parameters, etc.
    """
    if not outputs:
        return False

    # Functions with no inputs are always good candidates
    if not inputs:
        return True

    # Functions with only simple felt/address inputs are decent candidates
    simple_input_types = {"felt252", "ContractAddress", "ClassHash",
                          "u8", "u16", "u32", "u64", "u128", "u256"}
    for inp in inputs:
        if inp["cairo_type"] not in simple_input_types:
            return False

    return True


def recommend_table_type(event):
    """Recommend table type based on event structure and naming."""
    name = event["name"].lower()

    # Naming heuristics
    unique_patterns = ["update", "changed", "set", "modify", "state", "balance",
                       "position", "config", "leaderboard", "score", "status"]
    agg_patterns = ["volume", "count", "total", "accumulated", "fee", "revenue"]
    log_patterns = ["transfer", "emit", "create", "log", "mint", "burn", "swap",
                    "deposit", "withdraw", "approve", "order", "trade", "claim"]

    # Check naming patterns
    for p in agg_patterns:
        if p in name:
            return "aggregation", f"Name contains '{p}'"

    for p in unique_patterns:
        if p in name:
            return "unique", f"Name contains '{p}'"

    # If has address key field + data fields, could be unique by address
    key_addresses = [f for f in event["key_fields"] if f["is_address"]]
    if key_addresses and event["data_fields"]:
        # Events with address key + state-like data = unique candidate
        if any(p in name for p in unique_patterns):
            return "unique", f"Address key field '{key_addresses[0]['name']}' with state data"

    # Default to log for most events
    return "log", "Default: append-only event history"


def main():
    if len(sys.argv) < 2:
        print("Usage: parse_events.py <abi.json | ->", file=sys.stderr)
        sys.exit(1)

    source = sys.argv[1]
    if source == "-":
        abi_data = json.load(sys.stdin)
    else:
        with open(source) as f:
            abi_data = json.load(f)

    events = parse_abi(abi_data)
    views = parse_views(abi_data)

    output = {"events": [], "views": [], "summary": {}}

    # Process events
    for event in events:
        rec_type, rec_reason = recommend_table_type(event)
        unique_key_candidates = [f["name"] for f in event["key_fields"] if f["is_address"]]
        if not unique_key_candidates:
            unique_key_candidates = [f["name"] for f in event["key_fields"]]

        event_output = {
            "name": event["name"],
            "full_name": event["full_name"],
            "recommended_type": rec_type,
            "recommendation_reason": rec_reason,
            "key_fields": [{
                "name": f["name"],
                "type": f["cairo_type"],
                "sql_type": f["sql_type"],
            } for f in event["key_fields"]],
            "data_fields": [{
                "name": f["name"],
                "type": f["cairo_type"],
                "sql_type": f["sql_type"],
            } for f in event["data_fields"]],
            "unique_key_candidates": unique_key_candidates,
            "aggregatable_fields": event["numeric_fields"],
            "address_fields": event["address_fields"],
        }
        output["events"].append(event_output)

    # Process views
    for view in views:
        view_output = {
            "name": view["name"],
            "inputs": view["inputs"],
            "outputs": view["outputs"],
            "has_inputs": view["has_inputs"],
            "recommended_interval": view["recommended_interval"],
            "recommended_table_type": view["recommended_table_type"],
            "recommended_unique_key": view["recommended_unique_key"],
            "is_good_candidate": view["is_good_candidate"],
        }
        output["views"].append(view_output)

    # Summary
    good_view_candidates = [v for v in views if v["is_good_candidate"]]
    output["summary"] = {
        "total_events": len(events),
        "events_with_numeric_fields": sum(1 for e in events if e["has_numeric"]),
        "events_with_address_fields": sum(1 for e in events if e["has_address"]),
        "factory_candidate_events": [
            e["name"] for e in events
            if any("address" in f["name"].lower() or f["is_address"]
                   for f in e["data_fields"])
            and any(kw in e["name"].lower()
                    for kw in ["created", "deployed", "registered", "spawned", "new"])
        ],
        "total_views": len(views),
        "good_view_candidates": len(good_view_candidates),
        "view_candidate_names": [v["name"] for v in good_view_candidates],
    }

    if not events and not views:
        print("No events or view functions found in ABI.", file=sys.stderr)
        sys.exit(1)

    print(json.dumps(output, indent=2))


if __name__ == "__main__":
    main()
