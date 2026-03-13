# Ibis Query Reference

Complete reference for `ibis query` CLI and REST API.

## CLI Command

```
ibis query [contract] [event] [flags]
```

### Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--limit` | int | 50 | Maximum results (1-500) |
| `--offset` | int | 0 | Skip N results |
| `--order` | string | `block_number.desc` | Order: `field.asc` or `field.desc` |
| `--filter` | string[] | — | Repeatable. `field=op.value` or `field=value` (defaults to eq) |
| `--unique` | bool | false | Query unique table (last-write-wins) |
| `--aggregate` | bool | false | Query aggregation results |
| `--latest` | bool | false | Most recent event only |
| `--count` | bool | false | Count matching events |
| `--children` | bool | false | List factory child contracts |
| `--children-count` | bool | false | Count factory children |
| `--format` | string | `json` | Output: `json`, `table`, `csv` |
| `--list` | bool | false | List all available tables |
| `--contract-address` | string | — | Filter shared table by child address |

### Filter Operators

| Operator | Syntax | Meaning |
|----------|--------|---------|
| eq | `field=eq.value` or `field=value` | Equal (default) |
| neq | `field=neq.value` | Not equal |
| gt | `field=gt.value` | Greater than |
| gte | `field=gte.value` | Greater than or equal |
| lt | `field=lt.value` | Less than |
| lte | `field=lte.value` | Less than or equal |

Operators are checked in this order: `neq`, `gte`, `lte`, `gt`, `lt`, `eq`. The first matching prefix wins.

Multiple `--filter` flags combine with AND logic.

### CLI Examples

```bash
# List available tables
ibis query --list

# Basic event query
ibis query MyContract Transfer --format table

# With pagination and ordering
ibis query MyContract Transfer --limit 10 --offset 20 --order block_number.asc

# Filtering
ibis query MyContract Transfer --filter "block_number=gte.100000" --filter "status=eq.ACCEPTED_L2"

# Latest event
ibis query MyContract Transfer --latest

# Count
ibis query MyContract Transfer --count --filter "block_number=gte.100000"

# Unique table (current state per key)
ibis query MyContract LeaderboardUpdate --unique --order score.desc --limit 10

# Aggregation (totals/averages)
ibis query MyContract VolumeUpdate --aggregate

# Factory children
ibis query MyFactory --children --format table
ibis query MyFactory --children-count
ibis query MyFactory --children --filter "token0=eq.0x053c91..."

# Shared table filtered by child contract
ibis query MyFactory Swap --contract-address 0x123... --limit 10
```

## REST API Endpoints

Base URL: `http://{host}:{port}/v1` (default `http://localhost:8080/v1`)

### Event Queries

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/{contract}/{event}` | List events with pagination/filtering |
| GET | `/v1/{contract}/{event}/latest` | Most recent event |
| GET | `/v1/{contract}/{event}/count` | Count matching events |
| GET | `/v1/{contract}/{event}/unique` | Unique table entries |
| GET | `/v1/{contract}/{event}/aggregate` | Aggregation results |
| GET | `/v1/{contract}/{event}/stream` | SSE real-time stream |

### Factory Queries

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/{factory}/children` | List child contracts |
| GET | `/v1/{factory}/children/count` | Count children |

### System

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/health` | Health check |
| GET | `/v1/status` | Indexer status and contract cursors |

### Query Parameters (Supabase-style)

All list endpoints support:
- `limit=N` (default 50, max 500)
- `offset=N` (default 0)
- `order=field.asc` or `order=field.desc`
- `field=op.value` for filters (any non-reserved parameter)

Reserved parameters (not treated as filters): `limit`, `offset`, `order`

### Response Format

**Event list:**
```json
{
  "data": [
    {
      "block_number": 100000,
      "transaction_hash": "0x...",
      "log_index": 0,
      "timestamp": 1700000000,
      "contract_address": "0x...",
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

**Count:**
```json
{"count": 42}
```

**Aggregation:**
```json
{
  "values": {
    "total_volume": "1500000000",
    "trade_count": 847
  }
}
```

**Factory children:**
```json
[
  {
    "name": "MyFactory_abc1",
    "address": "0x...",
    "deployment_block": 100000,
    "current_block": 150000,
    "events": 3,
    "token0": "0x053c91...",
    "token1": "0x049d36..."
  }
]
```

## Config Structure

```yaml
network: mainnet
rpc: ${IBIS_RPC_URL}

database:
  backend: postgres  # postgres | badger | memory

contracts:
  - name: ContractName
    address: "0x..."
    abi: fetch  # or file path

    events:
      - name: "*"           # Wildcard: all ABI events
        table:
          type: log

      - name: EventName
        table:
          type: unique       # log | unique | aggregation
          unique_key: field   # required for unique

      - name: AggEvent
        table:
          type: aggregation
          aggregate:
            - column: total_x
              operation: sum   # sum | count | avg
              field: x_value

    factory:                   # Optional
      event: ChildCreated
      child_address_field: child_addr
      child_abi: fetch
      shared_tables: true
      child_events:
        - name: Swap
          table:
            type: log
```

## Table Naming

Table names are derived as: `lowercase(ContractName + "_" + EventName)`

Examples:
- Contract `MyToken`, Event `Transfer` -> table `mytoken_transfer`
- Factory `DEX`, Child Event `Swap` -> shared table `dex_swap`

## Metadata Columns (Always Present)

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

## Cairo Type to Column Type

| Cairo Type | Column Type |
|-----------|-------------|
| felt252 | string (hex) |
| u8-u64 | int64 |
| u128, u256 | string |
| i8-i64 | int64 |
| i128 | string |
| bool | bool |
| ContractAddress | string |
| ClassHash | string |
| ByteArray | string |
| Array/Span | string (JSON) |
| Structs/Enums | string (JSON) |
