# Ibis Query Reference

Complete reference for the ibis REST API and `ibis query` CLI.

---

## System Endpoints

### GET /v1/health

Simple health check.

**Response:**
```json
{"status": "ok"}
```

### GET /v1/status

Indexer status — contracts, cursors, factories, and view functions.

**Response:**
```json
{
  "current_block": 850000,
  "contracts": [
    {
      "name": "MyToken",
      "address": "0x04718f5a...",
      "events": 3,
      "current_block": 850000
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

- `current_block`: minimum cursor across all contracts (excluding those with 0 events)
- `factories`: only present if factories are configured
- `views`: only present if view functions are being polled

---

## Event Endpoints

Base URL: `http://{host}:{port}/v1` (default `http://localhost:8080/v1`)

### GET /v1/{contract}/{event}

List events with pagination, filtering, and ordering.

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | int | 50 | Max results (1–500) |
| `offset` | int | 0 | Skip N results |
| `order` | string | `block_number.desc` | Sort: `field.asc` or `field.desc` |
| `{field}` | string | — | Filter: `field=op.value` (any non-reserved param) |

**Response:**
```json
{
  "data": [
    {
      "block_number": 850000,
      "transaction_hash": "0xabc...",
      "log_index": 0,
      "timestamp": 1700000000,
      "contract_address": "0x04718f5a...",
      "event_name": "Transfer",
      "status": "ACCEPTED_L2",
      "from_address": "0x...",
      "to_address": "0x...",
      "amount": "1000000"
    }
  ],
  "count": 1,
  "limit": 50,
  "offset": 0
}
```

**Example:**
```bash
curl -s "http://localhost:8080/v1/Token/Transfer?limit=10&order=block_number.desc" | jq .
```

### GET /v1/{contract}/{event}/latest

Most recent event by block_number descending.

**Response:**
```json
{
  "data": {
    "block_number": 850000,
    "transaction_hash": "0xabc...",
    "log_index": 0,
    "timestamp": 1700000000,
    "contract_address": "0x...",
    "event_name": "Transfer",
    "status": "ACCEPTED_L2",
    "from_address": "0x...",
    "to_address": "0x...",
    "amount": "1000000"
  }
}
```

**Example:**
```bash
curl -s "http://localhost:8080/v1/Token/Transfer/latest" | jq .
```

### GET /v1/{contract}/{event}/count

Count matching events. Supports field filters only.

**Response:**
```json
{"count": 42}
```

**Example:**
```bash
curl -s "http://localhost:8080/v1/Token/Transfer/count?status=eq.ACCEPTED_L2" | jq .
```

### GET /v1/{contract}/{event}/unique

Query unique table (last-write-wins by key). Same query parameters and response format as the list endpoint.

**Example:**
```bash
curl -s "http://localhost:8080/v1/Game/LeaderboardUpdate/unique?order=score.desc&limit=10" | jq .
```

### GET /v1/{contract}/{event}/aggregate

Query aggregation results. Supports field filters only.

**Response:**
```json
{
  "data": {
    "total_volume": "1500000000",
    "trade_count": 847
  }
}
```

**Example:**
```bash
curl -s "http://localhost:8080/v1/DEX/VolumeUpdate/aggregate" | jq .
```

---

## View Endpoints

View tables store results from periodic on-chain function calls. They use the **same event endpoints** above — the table name is `{contract}_{function}`.

### View Table Naming

| Config | Table Name | API Path |
|--------|------------|----------|
| contract: `MyToken`, function: `total_supply` | `mytoken_total_supply` | `/v1/MyToken/total_supply` |
| contract: `Oracle`, function: `get_price` | `oracle_get_price` | `/v1/Oracle/get_price` |

### View Table Metadata Columns

View tables have a **different set of metadata columns** than event tables:

| Column | Type | Description |
|--------|------|-------------|
| `block_number` | uint64 | Block at which the view was polled |
| `timestamp` | uint64 | Poll timestamp (unix) |
| `contract_address` | string | Contract that was called |
| `_view_key` | string | Synthetic key for deduplication |
| `contract_name` | string | (shared view tables only) Which contract |

View tables do **NOT** have: `transaction_hash`, `log_index`, `event_name`, `status`.

### _view_key Behavior

- **unique view tables**: `_view_key` deduplicates — only the latest polled value per key is kept
- **log view tables**: every poll result is appended (full history)

### View Query Examples

```bash
# Latest polled price
curl -s "http://localhost:8080/v1/Oracle/get_price?order=block_number.desc&limit=1" | jq .

# Current unique view value
curl -s "http://localhost:8080/v1/Oracle/get_price/unique" | jq .

# Historical total supply over time
curl -s "http://localhost:8080/v1/MyToken/total_supply?order=block_number.asc&limit=100" | jq .

# Filter by specific contract in shared view tables
curl -s "http://localhost:8080/v1/MyToken/total_supply?contract_address=eq.0x..." | jq .
```

---

## Factory Endpoints

### GET /v1/{factory}/children

List child contracts created by a factory.

**Query Parameters:** Field filters using factory event field names: `?field=op.value`

**Response:**
```json
{
  "data": [
    {
      "name": "DEXFactory_abc1",
      "address": "0x...",
      "deployment_block": 100000,
      "current_block": 150000,
      "status": "active",
      "events": 3,
      "token0": "0x053c91...",
      "token1": "0x049d36..."
    }
  ],
  "count": 42
}
```

**Example:**
```bash
# List all children
curl -s "http://localhost:8080/v1/DEXFactory/children" | jq .

# Filter by factory event field
curl -s "http://localhost:8080/v1/DEXFactory/children?token0=eq.0x053c91..." | jq .
```

### GET /v1/{factory}/children/count

Count factory children. Same query parameters as `/children`.

**Response:**
```json
{"count": 42}
```

**Example:**
```bash
curl -s "http://localhost:8080/v1/DEXFactory/children/count" | jq .
```

---

## Discovery Endpoints

### GET /v1/discover/{classHash}/contracts

List contracts discovered via UDC class hash watching.

**Response:**
```json
[
  {
    "address": "0x...",
    "deployment_block": 800000,
    "current_block": 850000,
    "status": "active"
  }
]
```

Status values: `active`, `syncing`, `error`, `backfilling`

**Example:**
```bash
curl -s "http://localhost:8080/v1/discover/0x{classHash}/contracts" | jq .
```

**Note:** Discovery endpoints return **contract metadata** (addresses, deployment info), not indexed event data. To query data FROM discovered contracts, use their event/view table endpoints.

---

## SSE Streaming

### GET /v1/{contract}/{event}/stream

Real-time Server-Sent Events stream for new events.

**Request Headers:**
- `Last-Event-ID: {block}:{logIndex}` — optional, for reconnection replay

**Query Parameters:** Field filters (`eq` and `neq` operators). Numeric operators are ignored on replay.

**Response Headers:**
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

**Event Format:**
```
id: 850001:0
data: {"block_number":850001,"transaction_hash":"0x...","from_address":"0x...","amount":"1000"}

```

Each event has an `id` (block_number:log_index) and `data` (JSON-encoded event).

**Examples:**
```bash
# Stream all new Transfer events
curl -N "http://localhost:8080/v1/Token/Transfer/stream"

# Stream with filter
curl -N "http://localhost:8080/v1/Token/Transfer/stream?from_address=eq.0x..."

# Resume from known position
curl -N -H "Last-Event-ID: 850000:3" "http://localhost:8080/v1/Token/Transfer/stream"
```

**Errors:**
- 404: table not found
- 400: invalid filter format
- 503: event streaming not available (EventBus not configured)

---

## Shared Tables

Factory children, discovered contracts, and admin-registered contracts can all use shared tables. Shared tables combine events from multiple contracts into one table.

**Key behavior:**
- Shared tables add a `contract_name` column for disambiguation
- Shared table naming: `{factory_name}_{event_name}` (e.g., `dex_swap`)
- Filter by specific child: `?contract_address=eq.0x...`

**Example:**
```bash
# All swaps across all factory children
curl -s "http://localhost:8080/v1/DEX/Swap?limit=20&order=block_number.desc" | jq .

# Swaps from a specific child contract
curl -s "http://localhost:8080/v1/DEX/Swap?contract_address=eq.0x123..." | jq .
```

---

## Query Parameters Reference

### Filter Operators

| Operator | Syntax | Meaning |
|----------|--------|---------|
| eq | `field=eq.value` or `field=value` | Equal (default) |
| neq | `field=neq.value` | Not equal |
| gt | `field=gt.value` | Greater than |
| gte | `field=gte.value` | Greater than or equal |
| lt | `field=lt.value` | Less than |
| lte | `field=lte.value` | Less than or equal |

Operators are checked in this order: `neq`, `gte`, `lte`, `gt`, `lt`, `eq`. First matching prefix wins.

Multiple filters combine with AND logic: `?field1=op.value1&field2=op.value2`

### Reserved Parameters

These are NOT treated as field filters: `limit`, `offset`, `order`

### Ordering

Format: `?order=field.direction`

- Direction: `asc` or `desc`
- Default: `block_number.desc`
- Examples: `?order=block_number.asc`, `?order=timestamp.desc`, `?order=amount.desc`

---

## Response Formats

### Error Response

All errors follow this format with appropriate HTTP status codes:

```json
{"error": "error message here"}
```

Common status codes:
- 400: invalid parameters (bad filter, limit, offset, order)
- 404: table/factory/contract not found
- 500: internal query error
- 503: service unavailable (engine not running, EventBus not configured)

### List Response Envelope

Used by list (`/v1/{contract}/{event}`) and unique endpoints:

```json
{
  "data": [...],
  "count": 10,
  "limit": 50,
  "offset": 0
}
```

### Single Object Response

Used by `/latest`:

```json
{
  "data": { ... }
}
```

### Count Response

```json
{"count": 42}
```

### Aggregation Response

```json
{
  "data": {
    "column_name": "aggregated_value",
    "another_column": 847
  }
}
```

### Factory Children Response

```json
{
  "data": [...],
  "count": 42
}
```

---

## Config Reference

```yaml
network: mainnet           # Network name
rpc: ${IBIS_RPC_URL}       # RPC endpoint (WSS preferred, HTTP fallback)

database:
  backend: postgres         # postgres | badger | memory
  postgres:
    host: localhost
    port: 5432
    user: ibis
    password: ${DB_PASS}
    name: ibis
  badger:
    path: ./data/ibis

api:
  host: "0.0.0.0"          # API listen host (default: 0.0.0.0)
  port: 8080               # API listen port (default: 8080)
  cors_origins:             # CORS allowed origins (default: ["*"])
    - "*"
  admin_key: ${IBIS_ADMIN_KEY}  # Optional API key for admin endpoints

indexer:
  start_block: 800000      # Global start block (optional)
  pending_blocks: true      # Track pending blocks (default: false)
  batch_size: 10            # Backfill batch size (default: 10)
  udc_address: "0x0..."    # UDC contract address (has default)
  udc_event:                # UDC event format config (optional)
    version: auto           # auto | v0 | v1

contracts:
  - name: ContractName
    address: "0x..."
    abi: fetch              # "fetch" (from RPC) or file path
    start_block: 800000     # Per-contract start block (optional, overrides global)

    events:
      - name: "*"           # Wildcard: all ABI events
        table:
          type: log         # log | unique | aggregation

      - name: EventName
        table:
          type: unique
          unique_key: field_name

      - name: AggEvent
        table:
          type: aggregation
          aggregate:
            - column: total_x
              operation: sum  # sum | count | avg
              field: x_value

    views:                  # View functions to poll (optional)
      - function: total_supply
        calldata: []        # Optional calldata for the function
        interval: "30s"     # Poll interval
        table:
          type: unique      # log | unique
          unique_key: _view_key
        headers: {}         # Optional HTTP headers

    factory:                # Factory config (optional)
      event: PairCreated
      child_address_field: pair_address
      child_abi: fetch
      shared_tables: true   # All children share same tables
      child_name_template: "{factory}_{short_address}"
      child_events:
        - name: Swap
          table:
            type: log

discover:                   # Class hash discovery (optional)
  - class_hash: "0x..."
    group: mygroup          # Optional namespace
    abi: fetch
    shared_tables: true     # All discovered instances share tables
    name_template: "{class_hash_short}_{address_short}"
    events:
      - name: Transfer
        table:
          type: log
    views:                  # Views for discovered contracts (optional)
      - function: get_balance
        interval: "60s"
        table:
          type: unique
          unique_key: _view_key
```

### Runtime-Only Fields (not in YAML)

These fields appear in API responses but are not set in config files:
- `contract.dynamic` — true for admin-registered contracts
- `contract.shared_tables` — true for contracts using shared tables
- `contract.discover_class_hash` — set on discovery-spawned contracts
- `contract.factory_name` — parent factory name (on child contracts)
- `contract.factory_meta` — extra fields from factory event

---

## Table Naming

Table names are derived as: `lowercase(ContractName + "_" + EventName)`

| Source | Table Name |
|--------|------------|
| Contract `MyToken`, Event `Transfer` | `mytoken_transfer` |
| Factory `DEX`, Shared Event `Swap` | `dex_swap` |
| Contract `Oracle`, View `get_price` | `oracle_get_price` |
| Discovery group `mygroup`, Shared Event `Transfer` | Uses ABI-based prefix |

---

## Event Table Metadata Columns

| Column | Type | Description |
|--------|------|-------------|
| `block_number` | uint64 | Block height |
| `transaction_hash` | string | Transaction hash |
| `log_index` | uint64 | Position in block |
| `timestamp` | uint64 | Block timestamp (unix) |
| `contract_address` | string | Emitting contract |
| `event_name` | string | Event type |
| `status` | string | `PRE_CONFIRMED`, `ACCEPTED_L2`, `ACCEPTED_L1` |
| `contract_name` | string | (shared tables only) Child contract name |

## View Table Metadata Columns

| Column | Type | Description |
|--------|------|-------------|
| `block_number` | uint64 | Block at poll time |
| `timestamp` | uint64 | Poll timestamp (unix) |
| `contract_address` | string | Called contract |
| `_view_key` | string | Synthetic deduplication key |
| `contract_name` | string | (shared view tables only) Contract name |

---

## Cairo Type to Column Type

| Cairo Type | Column Type |
|-----------|-------------|
| felt252 | string (hex) |
| u8–u64 | int64 |
| u128, u256 | string |
| i8–i64 | int64 |
| i128 | string |
| bool | bool |
| ContractAddress | string |
| ClassHash | string |
| ByteArray | string |
| CairoTuple | string (JSON) |
| Array/Span | string (JSON) |
| Structs/Enums | string (JSON) |

---

## CLI Reference

The CLI is a fallback when the API server is not running.

### Command Syntax

```
ibis query [contract] [event] [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--limit` | int | 50 | Maximum results (1–500) |
| `--offset` | int | 0 | Skip N results |
| `--order` | string | `block_number.desc` | Order: `field.asc` or `field.desc` |
| `--filter` | string[] | — | Repeatable. `field=op.value` |
| `--unique` | bool | false | Query unique table |
| `--aggregate` | bool | false | Query aggregation results |
| `--latest` | bool | false | Most recent event only |
| `--count` | bool | false | Count matching events |
| `--children` | bool | false | List factory children |
| `--children-count` | bool | false | Count factory children |
| `--format` | string | `json` | Output: `json`, `table`, `csv` |
| `--list` | bool | false | List all available tables |
| `--contract-address` | string | — | Filter shared table by child address |

### CLI Examples

```bash
# List available tables
ibis query --list

# Basic event query
ibis query MyContract Transfer --format table

# With pagination
ibis query MyContract Transfer --limit 10 --offset 20 --order block_number.asc

# Filtering
ibis query MyContract Transfer --filter "block_number=gte.100000" --filter "status=eq.ACCEPTED_L2"

# Latest event
ibis query MyContract Transfer --latest

# Count
ibis query MyContract Transfer --count --filter "block_number=gte.100000"

# Unique table
ibis query MyContract LeaderboardUpdate --unique --order score.desc --limit 10

# Aggregation
ibis query MyContract VolumeUpdate --aggregate

# Factory children
ibis query MyFactory --children --format table
ibis query MyFactory --children-count
ibis query MyFactory --children --filter "token0=eq.0x053c91..."

# Shared table filtered by child
ibis query MyFactory Swap --contract-address 0x123... --limit 10
```
