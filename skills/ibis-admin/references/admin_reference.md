# Ibis Admin API Reference

Complete reference for the ibis admin REST API endpoints.

---

## Authentication

Admin endpoints (`/v1/admin/*`) are protected by the `X-Admin-Key` header when `api.admin_key` is configured.

| Header | Value |
|--------|-------|
| `X-Admin-Key` | The admin key string from `api.admin_key` in ibis.config.yaml |

If `api.admin_key` is empty or not set, no authentication is required.

`GET /v1/health` and `GET /v1/status` do NOT require authentication.

---

## Error Response Format

All errors follow this format:

```json
{"error": "error message here"}
```

### Common Status Codes

| Code | Meaning |
|------|---------|
| 400 | Invalid request (bad JSON, missing required fields) |
| 401 | Missing or incorrect `X-Admin-Key` |
| 404 | Contract not found |
| 500 | Internal error (registration/deregistration/update failed) |
| 503 | Engine not running (dynamic registration requires `ibis run`) |

---

## Endpoints

### POST /v1/admin/contracts

Register a new contract for indexing.

**Headers:**
- `Content-Type: application/json`
- `X-Admin-Key: {key}` (if configured)

**Request Body:** Full `ContractConfig` JSON (see schema below).

**Response (201 Created):**
```json
{
  "status": "registered",
  "name": "ContractName",
  "address": "0x04718f5a..."
}
```

**Error Cases:**
- 400: Invalid JSON, missing `name` or `address`, invalid event config
- 401: Missing or wrong admin key
- 500: Registration failed (e.g., contract already registered)
- 503: No engine running

---

### DELETE /v1/admin/contracts/{name}

Deregister a contract and optionally drop its tables.

**URL Parameters:**
- `{name}` — contract name (case-sensitive)

**Query Parameters:**
- `drop_tables=true` — (optional) Drop database tables. Default: `false`. **Destructive and irreversible.** Shared tables (used by other factory children) are NOT dropped.

**Headers:**
- `X-Admin-Key: {key}` (if configured)

**Response (200 OK):**
```json
{
  "status": "deregistered",
  "name": "ContractName",
  "drop_tables": true
}
```

**Error Cases:**
- 400: Missing contract name in URL
- 401: Missing or wrong admin key
- 500: Deregistration failed
- 503: No engine running

---

### GET /v1/admin/contracts

List all registered contracts with their full configuration.

**Headers:**
- `X-Admin-Key: {key}` (if configured)

**Response (200 OK):**
```json
{
  "contracts": [
    {
      "name": "STRK",
      "address": "0x04718f5a...",
      "abi": "fetch",
      "events": [
        {
          "name": "Transfer",
          "table": { "type": "log" }
        }
      ],
      "views": [],
      "start_block": 850000,
      "dynamic": true,
      "factory": null,
      "factory_name": "",
      "factory_meta": null,
      "shared_tables": false,
      "discover_class_hash": ""
    }
  ],
  "count": 1
}
```

**Runtime-only fields** (not set via config, populated by engine):
- `dynamic` — `true` for contracts registered via admin API
- `factory_name` — parent factory name (on child contracts)
- `factory_meta` — metadata fields from the factory event
- `shared_tables` — `true` when contract uses shared tables
- `discover_class_hash` — class hash (on discovery-spawned contracts)

**Error Cases:**
- 401: Missing or wrong admin key
- 503: No engine running

---

### PUT /v1/admin/contracts/{name}

Update an existing contract's configuration. Replaces the entire config.

**URL Parameters:**
- `{name}` — contract name (case-sensitive, must match existing contract)

**Headers:**
- `Content-Type: application/json`
- `X-Admin-Key: {key}` (if configured)

**Request Body:** Full `ContractConfig` JSON (same schema as POST).

**Response (200 OK):**
```json
{
  "status": "updated",
  "name": "ContractName"
}
```

**Error Cases:**
- 400: Invalid JSON, missing contract name in URL
- 401: Missing or wrong admin key
- 404: Contract with given name not found
- 500: Update failed
- 503: No engine running

---

### GET /v1/health

Simple health check. No authentication required.

**Response (200 OK):**
```json
{"status": "ok"}
```

---

### GET /v1/status

Detailed indexer status. No authentication required.

**Response (200 OK):**
```json
{
  "current_block": 850000,
  "contracts": [
    {
      "name": "STRK",
      "address": "0x04718f5a...",
      "events": 3,
      "current_block": 850000
    },
    {
      "name": "DEXFactory",
      "address": "0x01aa950c...",
      "events": 1,
      "current_block": 849500
    }
  ],
  "factories": {
    "DEXFactory": {
      "child_count": 42,
      "synced": 40,
      "backfilling": 2
    }
  },
  "views": [
    {
      "function_name": "get_price",
      "contract": "Oracle",
      "interval": "30s",
      "last_poll_block": 849999,
      "last_poll_time": "2025-01-15T10:30:00Z",
      "consecutive_errors": 0
    }
  ]
}
```

**Fields:**
- `current_block` — global cursor, minimum across all contracts (excluding zero-event contracts)
- `contracts[].events` — number of configured events
- `contracts[].current_block` — per-contract cursor position
- `factories` — only present if factory contracts are configured
- `views` — only present if view functions are being polled

---

## ContractConfig JSON Schema

The full JSON structure accepted by `POST /v1/admin/contracts` and `PUT /v1/admin/contracts/{name}`.

```json
{
  "name": "string (required)",
  "address": "string (required, 0x-prefixed, 1-64 hex chars)",
  "abi": "string (optional, default: 'fetch')",
  "start_block": 850000,
  "events": [
    {
      "name": "string (required, event name from ABI or '*' for all)",
      "table": {
        "type": "log | unique | aggregation",
        "unique_key": "string (required if type=unique)",
        "aggregate": [
          {
            "column": "string (output column name)",
            "operation": "sum | count | avg",
            "field": "string (source event field)"
          }
        ]
      }
    }
  ],
  "views": [
    {
      "function": "string (required, view function name from ABI)",
      "calldata": ["0x-prefixed hex strings"],
      "interval": "duration string (e.g., '5s', '30s', '5m')",
      "table": {
        "type": "log | unique",
        "unique_key": "string (use '_view_key' for single-row mode)"
      },
      "headers": { "key": "value" }
    }
  ],
  "factory": {
    "event": "string (required, factory event name)",
    "child_address_field": "string (required, event field with child address)",
    "child_abi": "string (default: 'fetch')",
    "child_events": [
      {
        "name": "string",
        "table": { "type": "log | unique | aggregation", "..." : "..." }
      }
    ],
    "shared_tables": true,
    "child_name_template": "string (e.g., '{factory}_{token0}_{token1}')"
  }
}
```

### Required Fields

| Field | Constraint |
|-------|-----------|
| `name` | Non-empty string, unique across all contracts |
| `address` | `0x` + 1-64 hex characters |
| `events` | At least one event entry |
| `events[].name` | Non-empty string (or `"*"` for wildcard) |
| `events[].table.type` | One of: `log`, `unique`, `aggregation` |

### Conditional Fields

| Field | Required When |
|-------|--------------|
| `table.unique_key` | `table.type` is `unique` |
| `table.aggregate` | `table.type` is `aggregation` (at least one spec) |
| `aggregate[].column` | Always (within aggregate array) |
| `aggregate[].operation` | Always — one of: `sum`, `count`, `avg` |
| `aggregate[].field` | Always — must reference a numeric event field |
| `factory.event` | Factory config is present |
| `factory.child_address_field` | Factory config is present |
| `factory.child_events` | Factory config is present (at least one) |

### Optional Fields with Defaults

| Field | Default |
|-------|---------|
| `abi` | `"fetch"` (fetches ABI from chain via RPC) |
| `start_block` | `null` (uses global `indexer.start_block`, or 0) |
| `views` | `[]` (no view function polling) |
| `factory` | `null` (not a factory contract) |
| `factory.child_abi` | `"fetch"` |
| `factory.shared_tables` | `false` (but `true` is recommended for most factories) |
| `factory.child_name_template` | `"{factory}_{short_address}"` |

### ABI Resolution

The `abi` field supports three modes:

1. **`"fetch"`** (default) — Fetches ABI from chain via `starknet_getClassAt` RPC call
2. **File path** (contains `./`, `/`, `../`, or ends with `.json`) — Reads ABI from local file
3. **Named ABI** (any other string) — Searches `target/dev/*_{Name}.contract_class.json`

### Table Types

| Type | Behavior | Extra Config |
|------|----------|-------------|
| `log` | Append-only, every event creates a row | None |
| `unique` | Last-write-wins by key, stores latest per key | `unique_key` required |
| `aggregation` | Auto-computes running sums/counts/averages | `aggregate[]` required |

### View Table Constraints

- Table type must be `log` or `unique` only (no `aggregation`)
- `interval` is a Go duration string (e.g., `5s`, `30s`, `5m`), minimum `1s`
- Use `unique_key: "_view_key"` for single-row latest-value mode
- View tables have different metadata columns than event tables (no `transaction_hash`, `log_index`, `event_name`, `status`)

---

## Event Table Metadata Columns

Auto-added to all event tables (not specified in config):

| Column | Type | Description |
|--------|------|-------------|
| `block_number` | uint64 | Block height |
| `transaction_hash` | string | Transaction hash |
| `log_index` | uint64 | Event position within block |
| `timestamp` | uint64 | Block timestamp (unix epoch) |
| `contract_address` | string | Emitting contract address |
| `event_name` | string | Event type name |
| `status` | string | `PRE_CONFIRMED`, `ACCEPTED_L2`, or `ACCEPTED_L1` |
| `contract_name` | string | (shared tables only) Source contract name |

---

## Cairo Type to Column Type

| Cairo Type | Column Type | Notes |
|-----------|-------------|-------|
| felt252 | string | Hex-encoded |
| u8, u16, u32, u64 | int64 | Fits int64 |
| u128, u256 | string | Big number as decimal string |
| i8, i16, i32, i64 | int64 | Two's complement |
| i128 | string | Big number as string |
| bool | bool | Native boolean |
| ContractAddress | string | Hex-encoded |
| ClassHash | string | Hex-encoded |
| ByteArray | string | UTF-8 text |
| Array\<T\>, Span\<T\> | string | JSON-encoded array |
| struct | string | JSON-encoded object |
| enum | string | JSON-encoded variant |
| (T1, T2, ...) | string | CairoTuple — JSON-encoded array |

### Aggregation-Compatible Types

Only numeric types support `sum` and `avg` operations:
- u8, u16, u32, u64, u128, u256
- i8, i16, i32, i64, i128

The `count` operation works on any field type.
