# Ibis Config Schema Reference

## Complete Config Structure

```yaml
# REQUIRED: Network identifier
network: mainnet|sepolia|custom

# REQUIRED: Starknet RPC endpoint
# WSS preferred (enables starknet_subscribeEvents real-time streaming)
# HTTP falls back to polling via starknet_getEvents
rpc: wss://... | https://...

# REQUIRED: Database configuration
database:
  backend: postgres|badger|memory    # Default: memory
  postgres:                          # Required if backend=postgres
    host: localhost                  # Default: "" (required)
    port: 5432                       # Default: 5432
    user: ibis                       # Required
    password: ${IBIS_DB_PASSWORD}    # Required (env var expansion supported)
    name: ibis                       # Required
  badger:                            # For embedded KV store
    path: ./data/ibis                # Default: ./data/ibis

# OPTIONAL: REST API configuration
api:
  host: 0.0.0.0                      # Default: 0.0.0.0
  port: 8080                         # Default: 8080
  cors_origins: ["*"]                # Optional CORS allow list
  admin_key: ${ADMIN_KEY}            # Optional: required for /v1/admin/* endpoints

# OPTIONAL: Indexer settings
indexer:
  start_block: 0                     # Default: 0 (latest block). Use specific number for historical backfill
  pending_blocks: true               # Default: true (index pre-confirmed blocks)
  batch_size: 10                     # Default: 10 (blocks per backfill query)

# REQUIRED: At least one contract
contracts:
  - name: MyContract                 # Required: unique identifier (used in API paths, table names)
    address: "0x049d..."             # Required: Starknet contract address (0x + 1-64 hex chars)
    abi: fetch                       # Default: "fetch". Options: fetch | ./path/to.json | ContractName
    start_block: 0                   # Optional per-contract override

    # REQUIRED: at least one event
    events:
      - name: Transfer               # Event name from ABI, or "*" for all events
        table:
          type: log                  # log | unique | aggregation

      - name: BalanceUpdate
        table:
          type: unique
          unique_key: account        # Required for unique: field for last-write-wins dedup

      - name: VolumeUpdate
        table:
          type: aggregation
          aggregate:                 # Required for aggregation: at least one spec
            - column: total_volume   # Output column name
              operation: sum         # sum | count | avg
              field: amount          # Source event field

    # OPTIONAL: Factory configuration (for contracts that deploy child contracts)
    factory:
      event: PairCreated             # Required: factory event that signals child deployment
      child_address_field: pair      # Required: event field containing deployed child address
      child_abi: fetch               # Default: "fetch". ABI for children (cached after first)
      shared_tables: true            # Default: false. All children write to same tables
      child_name_template: "{factory}_{short_address}"  # Supports {factory}, {short_address}, {event_field_names}
      child_events:                  # Required: event/table config template for each child
        - name: "*"
          table:
            type: log
```

## Environment Variable Expansion

Pattern: `${VAR_NAME}` anywhere in string values.
Unset variables expand to empty string.
Common pattern for secrets:

```yaml
database:
  postgres:
    host: ${IBIS_DB_HOST}
    password: ${IBIS_DB_PASSWORD}
api:
  admin_key: ${ADMIN_KEY}
```

## ABI Resolution Priority

1. **Explicit file path**: Value contains `./`, `/`, `../`, or ends with `.json`
   - Example: `abi: "./target/dev/myproject_MyContract.contract_class.json"`
2. **Smart local discovery**: Value is not "fetch" and not a file path
   - Searches `target/dev/*_{Name}.contract_class.json` (Scarb build artifacts)
   - Example: `abi: "ERC20"` finds `target/dev/myproject_ERC20.contract_class.json`
3. **Chain fetch**: Value is "fetch" or other strategies fail
   - Calls `starknet_getClassAt` via RPC
   - Handles both Sierra (modern) and deprecated (Cairo 0) contract formats
   - Note: proxy contracts may return proxy ABI; use file path for implementation ABI

## Default RPC Endpoints

| Network  | HTTP                                               | WSS                |
|----------|----------------------------------------------------|--------------------|
| mainnet  | https://free-rpc.nethermind.io/mainnet-juno         | (user-provided)    |
| sepolia  | https://free-rpc.nethermind.io/sepolia-juno         | (user-provided)    |

## Table Types

### Log (append-only)
Every event creates a new row. Full history preserved.
- Use for: transfers, swaps, mints, burns, deposits, withdrawals, trades, approvals
- No additional config required beyond `type: log`

### Unique (last-write-wins)
Only the latest entry per unique key is stored. Previous entries overwritten.
- Use for: leaderboards, balances, positions, status tracking, configuration state
- Required: `unique_key` field name (must exist in event fields)
- With shared tables: unique key becomes composite `(contract_address, unique_key)`

### Aggregation (auto-computed)
Automatically computes running aggregates from events.
- Use for: volume tracking, counters, statistics, averages
- Required: `aggregate` array with at least one spec
- Each spec: `column` (output name), `operation` (sum|count|avg), `field` (source event field)
- Field must be numeric (u8-u256, i8-i128)

## Metadata Columns (auto-added to ALL tables)

| Column             | Type   | Description                                       |
|--------------------|--------|---------------------------------------------------|
| block_number       | uint64 | Block containing the event                        |
| transaction_hash   | string | Transaction that emitted the event                |
| log_index          | uint64 | Event index within the transaction                |
| timestamp          | uint64 | Block timestamp                                   |
| contract_address   | string | Address of the emitting contract                  |
| event_name         | string | Name of the event                                 |
| status             | string | PRE_CONFIRMED, ACCEPTED_L2, or ACCEPTED_L1       |

With shared tables, an additional `contract_name` column identifies the child contract.

## Factory Pattern Details

### When to Use Factory Config
- Contract deploys other contracts (e.g., AMM factories deploying pair contracts)
- Want to automatically discover and index child contracts
- All children share the same ABI/event structure

### Shared Tables vs Per-Contract Tables
- `shared_tables: true` (recommended for 10+ children): One table per event type. All children write to same tables with `contract_name` discriminator. Prevents table explosion.
- `shared_tables: false`: Separate tables per child. Table names: `{child_name}_{event_name}`. Only suitable for small numbers of children.

### Child Name Template Placeholders
- `{factory}` - Factory contract name
- `{short_address}` - First 8 hex chars of child address
- `{field_name}` - Any field from the factory event (e.g., `{token0}`, `{token1}`)

### Factory Event Detection Heuristics
Events likely to be factory events:
- Name contains: "Created", "Deployed", "Registered", "Spawned", "New", "Launched"
- Has a data field of type ContractAddress (the deployed child address)
- Has additional data fields that describe the child (e.g., token addresses, pool parameters)

## Wildcard Events

```yaml
events:
  - name: "*"
    table:
      type: log
```

Indexes ALL events found in the contract ABI with the specified table type.
Specific event entries override the wildcard for that event:

```yaml
events:
  - name: "*"              # Default: all events as log
    table:
      type: log
  - name: BalanceUpdate    # Override: this specific event as unique
    table:
      type: unique
      unique_key: account
```

## Validation Rules

- `network`: required, must be mainnet|sepolia|custom
- `rpc`: required, must start with wss://|ws://|https://|http://
- `database.backend`: must be postgres|badger|memory
- `contracts`: at least one required
- Each contract: `name` (non-empty), `address` (0x + 1-64 hex), `events` (at least one)
- Event `table.type`: must be log|unique|aggregation
- Unique table: `unique_key` required
- Aggregation table: `aggregate` array required, each with `column`, `operation` (sum|count|avg), `field`
- Factory: `event`, `child_address_field`, `child_events` (at least one) required

## Cairo Type to SQL Column Mapping

| Cairo Type         | SQL Column Type      | Notes                    |
|--------------------|----------------------|--------------------------|
| felt252            | string               | Hex-encoded              |
| u8, u16, u32, u64  | int64                | Fits uint64              |
| u128, u256          | string               | Big number as string     |
| i8, i16, i32, i64   | int64                | Two's complement         |
| i128               | string               | Big number as string     |
| bool               | bool                 | Native boolean           |
| ContractAddress    | string               | Hex-encoded              |
| ClassHash          | string               | Hex-encoded              |
| ByteArray          | string               | UTF-8 text               |
| Array<T>, Span<T>  | string               | JSON-encoded array       |
| struct             | string               | JSON-encoded object      |
| enum               | string               | JSON-encoded variant     |

### Aggregation-Compatible Types (numeric)
Only these types support sum/avg operations:
- u8, u16, u32, u64, u128, u256
- i8, i16, i32, i64, i128
- count operation works on any field type
