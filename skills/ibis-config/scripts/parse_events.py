#!/usr/bin/env python3
"""Parse a Starknet ABI JSON and extract event definitions with field details.

Usage: parse_events.py <abi.json>
       cat abi.json | parse_events.py -

Output: structured event summary suitable for config generation decisions.
"""

import sys
import json

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


def short_type(full_type: str) -> str:
    """Extract short type name from fully qualified Cairo type."""
    if "::" in full_type:
        return full_type.split("::")[-1]
    return full_type


def get_sql_type(cairo_type: str) -> str:
    """Map Cairo type to ibis column type."""
    if cairo_type in TYPE_MAP:
        return TYPE_MAP[cairo_type]
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

    if not events:
        print("No events found in ABI.", file=sys.stderr)
        sys.exit(1)

    output = {"events": [], "summary": {}}

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
    }

    print(json.dumps(output, indent=2))


if __name__ == "__main__":
    main()
