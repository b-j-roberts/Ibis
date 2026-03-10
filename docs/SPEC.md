# Ibis - Specification

## Project Overview

**Ibis** is a fast, easy-to-use Starknet indexer written in Go. It indexes events from Starknet using only an RPC WebSocket connection, generates typed database tables and REST APIs from contract ABIs, and launches with a single command from a YAML config file.

### Problem Statement

Setting up a production-grade Starknet indexer today requires either:
- **Apibara**: Powerful but demands custom TypeScript transform logic, no auto-generated APIs, stateless by default
- **Torii**: Zero-config but locked to the Dojo ECS framework, cannot index arbitrary contracts
- **Custom solutions**: High effort to build, maintain, and evolve

There is no general-purpose Starknet indexer that provide a modern, easy-to-use developer experience with the flexibility to index any contract.

### Target Users

- Starknet application developers who need indexed event data with typed APIs
- Me
- Teams wanting a drop-in indexer they can configure, deploy, and query without writing indexer code

### Design Principles

1. **One config, one command** -- `ibis.config.yaml` + `ibis run` is all you need
2. **ABI-driven** -- contract ABIs drive table schemas, REST endpoints, and type safety
3. **Production-ready** -- pending block support, reorg handling, multiple DB backends, Docker/K8s deployment
4. **AI era ready** -- natural language queries and AI-powered config generation

---

## Architecture

```
                        ┌──────────────────────────┐
                        │   Starknet RPC (WSS)     │
                        └────────────┬─────────────┘
                                     │
                          ┌──────────▼───────────┐
                          │  Event Subscriber    │
                          │  - Per-contract subs │
                          │  - Reconnection      │
                          │  - Polling fallback  │
                          └──────────┬───────────┘
                                     │
                          ┌──────────▼───────────┐
                          │  Event Processor     │
                          │  - ABI decoding      │
                          │  - Selector matching │
                          │  - Pending tracking  │
                          └──────────┬───────────┘
                                     │
                     ┌───────────────┼───────────────┐
                     │               │               │
              ┌──────▼──────┐  ┌─────▼──────┐  ┌─────▼──────┐
              │  BadgerDB   │  │ PostgreSQL │  │ In-Memory  │
              │  (embedded) │  │ (external) │  │  (dev/test)│
              └─────────────┘  └────────────┘  └────────────┘
                     │               │               │
                     └───────────────┼───────────────┘
                                     │
                          ┌──────────▼───────────┐
                          │   API Server         │
                          │  - REST (generated)  │
                          │  - SSE (real-time)   │
                          │  - Query CLI         │
                          └──────────────────────┘
```

### Data Flow

1. **Subscribe** -- Event Subscriber connects to Starknet RPC WSS and calls `starknet_subscribeEvents` per configured contract (with `from_address` and `block_id` params). Falls back to `starknet_getEvents` HTTP polling if WSS fails.
2. **Process** -- Event Processor matches incoming events by selector (`keys[0]`) against ABI event definitions, then decodes `keys[]` and `data[]` Felt arrays into typed data
3. **Store** -- Decoded events are written to the configured database backend using revert/add operation pairs (for pending block safety)
4. **Serve** -- API Server exposes auto-generated REST endpoints and SSE streams based on the ABI-derived table schemas
5. **Reorg** -- On reorg notification (delivered inline via event subscription), revert operations undo orphaned data. If reorg notifications are not available, the engine uses linear forward progression with cursor-based resume (like foc-engine).
6. **Backfill** -- On startup, if the cursor is behind chain head, uses `starknet_getEvents` HTTP RPC with continuation tokens to catch up before switching to WSS streaming

---

## Tech Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Language | Go 1.23+ | Performance, concurrency, single binary |
| Starknet SDK | `NethermindEth/starknet.go` | Official Go SDK, RPC + WS support, selectors, hashing |
| WebSocket | `gorilla/websocket` (via starknet.go) | Industry standard, used internally by starknet.go |
| Embedded DB | BadgerDB v4 | Fast KV store, prefix scanning, used by major blockchains |
| Relational DB | PostgreSQL (via `pgx/v5`) | Production-grade, pgx is the fastest Go Postgres driver |
| In-Memory DB | Custom (Go maps + mutexes) | Zero-dependency dev/test mode |
| Config | `gopkg.in/yaml.v3` | YAML parsing with env var expansion |
| HTTP Router | `net/http` (stdlib) | Go 1.22+ has built-in routing, no dependency needed |
| SSE | Custom (stdlib) | Simple streaming over HTTP, no library needed |
| CLI | `spf13/cobra` | Standard Go CLI framework |
| ABI Parsing | Custom | starknet.go lacks ABI decoding; build on foc-engine's parser patterns |
| Containerization | Docker | Standard deployment |
| Task Runner | Makefile | Per user preferences |

---

## Project Structure

```
ibis/
├── cmd/
│   └── ibis/
│       └── main.go                  # CLI entry point (cobra root)
├── internal/
│   ├── abi/                         # ABI parsing and event decoding
│   │   ├── parser.go                # Parse Cairo ABI JSON into Go types
│   │   ├── decoder.go               # Decode Felt arrays into typed event data
│   │   ├── selector.go              # Event selector computation and matching
│   │   └── types.go                 # ABI type definitions (struct, enum, felt, etc.)
│   ├── api/                         # HTTP API server
│   │   ├── server.go                # Server setup, middleware, SSE
│   │   ├── generator.go             # Generate REST routes from ABI schemas
│   │   ├── handlers.go              # Generated endpoint handler logic
│   │   └── query.go                 # Query parsing and execution
│   ├── cli/                         # CLI commands
│   │   ├── init.go                  # `ibis init` -- scaffold config
│   │   ├── run.go                   # `ibis run` -- start indexer
│   │   └── query.go                 # `ibis query` -- CLI queries
│   ├── config/                      # Configuration management
│   │   ├── config.go                # Config struct and loader
│   │   ├── validate.go              # Config validation
│   │   └── abi_resolve.go           # ABI resolution (chain/local/scarb)
│   ├── engine/                      # Core indexing engine
│   │   ├── engine.go                # Main indexing orchestrator
│   │   ├── processor.go             # Event processing pipeline
│   │   ├── pending.go               # Pending event/block handling
│   │   └── reorg.go                 # Reorg handling and rollback
│   ├── provider/                    # Starknet RPC/WS provider
│   │   ├── provider.go              # RPC + WS provider wrapper
│   │   ├── ws.go                    # WebSocket subscription manager
│   │   └── reconnect.go             # Reconnection with backoff
│   ├── store/                       # Database abstraction
│   │   ├── store.go                 # Store interface definitions
│   │   ├── operations.go            # Revert/add operation pair types
│   │   ├── badger/                  # BadgerDB implementation
│   │   │   └── badger.go
│   │   ├── postgres/                # PostgreSQL implementation
│   │   │   ├── postgres.go
│   │   │   └── migrations.go        # Auto-generated table migrations
│   │   └── memory/                  # In-memory implementation
│   │       └── memory.go
│   ├── schema/                      # ABI-derived table schema system
│   │   ├── schema.go                # Table schema from ABI events
│   │   ├── columns.go               # Column types (standard, unique, aggregation)
│   │   └── generator.go             # Schema generation from config
│   └── types/                       # Shared type definitions
│       └── types.go
├── configs/                         # Example configs
│   ├── ibis.config.yaml             # Example config
│   └── ibis.config.docker.yaml      # Docker example
├── docs/
│   ├── SPEC.md
│   └── ROADMAP.md
├── scripts/                         # Bash scripts
│   └── install.sh                   # Binary installer
├── Dockerfile
├── docker-compose.yaml
├── Makefile
├── go.mod
├── go.sum
└── README.md
```

---

## Core Modules

### ABI Parser (`internal/abi/`)

Parses Cairo contract ABIs (JSON) into Go type definitions. Supports all Starknet/Cairo types:
- Primitives: `felt252`, `u8`-`u256`, `i8`-`i128`, `bool`, `ContractAddress`, `ClassHash`, `ByteArray`
- Composites: `Array<T>`, `Span<T>`, structs, enums
- Snapshots: `@Array<T>`, `@Span<T>`

Computes event selectors via `starknet_keccak` and matches incoming event keys to ABI event definitions. Decodes the `keys[]` and `data[]` Felt arrays into typed Go maps based on the ABI member definitions.

Reference: foc-engine's `internal/registry/parser.go` for the visitor pattern.

### Config Manager (`internal/config/`)

Loads `ibis.config.yaml` with the following structure:

```yaml
# ibis.config.yaml
network: mainnet                        # mainnet | sepolia | custom
rpc: wss://starknet-mainnet.example.com # RPC WSS endpoint

database:
  backend: postgres                     # postgres | badger | memory
  postgres:
    host: localhost
    port: 5432
    user: ibis
    password: ${IBIS_DB_PASSWORD}
    name: ibis
  badger:
    path: ./data/ibis

api:
  host: 0.0.0.0
  port: 8080

indexer:
  start_block: 0                        # 0 = latest, or specific block number
  pending_blocks: true                  # Index pre-confirmed blocks
  batch_size: 10                        # Blocks per batch for backfill

contracts:
  - name: StarknetOptions
    address: "0x049d36570d4e46..."
    abi: ./target/dev/stops_StarknetOptions.contract_class.json  # Local path
    # OR
    # abi: fetch                        # Fetch from chain (default)
    # OR
    # abi: StarknetOptions              # Smart local discovery + scarb build
    events:
      - name: "*"                       # Wildcard: index ALL events as log tables
        table:
          type: log
      - name: LeaderboardUpdate         # Override: specific events get custom table types
        table:
          type: unique
          unique_key: trader_address    # Field to use as unique identifier
      - name: VolumeUpdate
        table:
          type: aggregation
          aggregate:
            - column: total_volume
              operation: sum
              field: volume
            - column: trade_count
              operation: count
```

**Event selection rules**:
- `"*"` -- wildcard, matches all events defined in the contract's ABI
- Specific event entries override the wildcard for that event (e.g., `LeaderboardUpdate` above overrides the `*` default of `log` with `unique`)
- If no `"*"` entry exists, only explicitly listed events are indexed
- The wildcard table type sets the default; specific entries always take precedence

ABI resolution priority:
1. Explicit file path (e.g., `./target/dev/...`)
2. Smart local discovery by contract name (searches `target/dev/`, builds with `scarb` if needed)
3. Fetch from chain via RPC `getClassAt`

Environment variable expansion via `${VAR_NAME}` syntax.

### Indexing Engine (`internal/engine/`)

The core orchestrator that coordinates event processing. Unlike block-scanning indexers, Ibis subscribes directly to contract events via `starknet_subscribeEvents` (like foc-engine):

1. **Startup**: Resolve ABIs, compute event selectors, determine starting block from cursor
2. **Backfill** (if behind): Use `starknet_getEvents` HTTP RPC with continuation tokens to catch up in chunks
3. **Stream**: Subscribe to `starknet_subscribeEvents` per contract via WSS, starting from current block
4. **Process**: For each received event, match selector, decode via ABI, generate operation pairs, write to store
5. **Fallback**: If WSS fails, fall back to `starknet_getEvents` polling loop (adaptive timing: 100ms during catchup, 2s at chain tip)
6. **Reorg**: If the WSS subscription delivers reorg notifications, execute revert operations for orphaned data. Otherwise, rely on linear forward progression.

**Pending Block Strategy**: Every database write is recorded as an `(add, revert)` operation pair. The `add` operation writes the new data. The `revert` operation undoes it (delete for inserts, restore previous value for updates). When a pending block is replaced or reverted, the revert operations are executed in reverse order.

### Store Interface (`internal/store/`)

Database-agnostic interface supporting three backends:

```go
type Store interface {
    // Event operations (with revert support)
    ApplyOperations(ctx context.Context, ops []Operation) error
    RevertOperations(ctx context.Context, ops []Operation) error

    // Query operations
    GetEvents(ctx context.Context, table string, query Query) ([]Event, error)
    GetUniqueEvents(ctx context.Context, table string, query Query) ([]Event, error)
    GetAggregation(ctx context.Context, table string, query Query) (AggResult, error)

    // Cursor tracking
    GetCursor(ctx context.Context) (uint64, error)
    SetCursor(ctx context.Context, blockNumber uint64) error

    // Schema management
    CreateTable(ctx context.Context, schema TableSchema) error
    MigrateTable(ctx context.Context, schema TableSchema) error

    Close() error
}
```

### API Server (`internal/api/`)

Auto-generates REST endpoints from the ABI-derived table schemas:

```
# For each configured event table:
GET  /v1/{contract}/{event}                  # List events (paginated)
GET  /v1/{contract}/{event}/:id              # Get single event
GET  /v1/{contract}/{event}/latest           # Get latest event
GET  /v1/{contract}/{event}/count            # Count events
POST /v1/{contract}/{event}/filter           # Filter with body params

# For unique tables:
GET  /v1/{contract}/{event}/unique           # List unique entries

# For aggregation tables:
GET  /v1/{contract}/{event}/aggregate        # Get aggregated values

# SSE endpoints:
GET  /v1/{contract}/{event}/stream           # SSE stream of new events

# System:
GET  /v1/health                              # Health check
GET  /v1/status                              # Indexer status (block height, sync %)
```

Query parameters follow Supabase conventions:
- `?limit=50&offset=0` -- pagination
- `?order=block_number.desc` -- ordering
- `?trader_address=eq.0x123` -- field filtering
- `?block_number=gte.100000` -- comparison operators (eq, neq, gt, gte, lt, lte)

### Schema System (`internal/schema/`)

Translates ABI event definitions + config into table schemas:

| Table Type | Behavior | Use Case |
|-----------|----------|----------|
| `log` | Append-only event log | Transaction history, audit trail |
| `unique` | Last-write-wins by unique key | Leaderboards, current state |
| `aggregation` | Auto-computed aggregates | Volume tracking, counters |

For PostgreSQL, schemas are translated into `CREATE TABLE` statements with appropriate indices. For BadgerDB, schemas define key prefix patterns and secondary index strategies. For in-memory, schemas define map structures.

---

## Data Models

### Core Entities

**IndexedEvent** -- A decoded, stored event:
```go
type IndexedEvent struct {
    ID              string            // Unique event ID
    ContractAddress string            // Source contract
    EventName       string            // ABI event name (e.g., "Transfer")
    EventType       string            // Full qualified type path
    BlockNumber     uint64
    BlockHash       string
    TransactionHash string
    LogIndex        uint64
    Data            map[string]any    // ABI-decoded event fields
    Timestamp       uint64
    Status          BlockStatus       // PreConfirmed | AcceptedL2 | AcceptedL1
}
```

**Operation** -- A reversible database operation:
```go
type Operation struct {
    Type    OpType           // Insert | Update | Delete
    Table   string
    Key     string
    Data    map[string]any   // For Insert/Update
    Prev    map[string]any   // Previous data (for revert on Update)
    BlockNumber uint64
}
```

**TableSchema** -- An ABI-derived table definition:
```go
type TableSchema struct {
    Name       string
    Contract   string
    Event      string
    TableType  TableType        // Log | Unique | Aggregation
    Columns    []Column
    UniqueKey  string           // For unique tables
    Aggregates []AggregateSpec  // For aggregation tables
}
```

---

## External Integrations

| Integration | Purpose | Protocol |
|------------|---------|----------|
| Starknet RPC Node | Block/event data, ABI fetching | WSS + HTTP JSON-RPC |
| PostgreSQL | Production database backend | TCP (pgx driver) |
| Scarb | Local ABI building from Cairo source | CLI subprocess |

---

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Event subscription, not block scanning | `starknet_subscribeEvents` per contract, not `newHeads` | Direct event delivery is simpler, lower overhead, and matches foc-engine's proven pattern. HTTP `starknet_getEvents` for backfill and polling fallback. |
| Custom ABI decoder | Build on foc-engine patterns | starknet.go lacks ABI decoding; need full Cairo type system support |
| Operation pairs for reorgs | Every write has a paired revert | Clean pending block handling without full state replay |
| Supabase-style REST | Filter syntax inspired by PostgREST/Supabase | Familiar to web developers, powerful query semantics |
| Cobra for CLI | `ibis init`, `ibis run`, `ibis query` | Standard Go CLI framework, subcommand pattern |
| No GraphQL for MVP | REST + SSE is simpler | GraphQL adds complexity; REST covers most use cases; add later if needed |
| Store interface pattern | Repository pattern from zindex | Clean separation, easy to add backends, testable |
| YAML config over code | Declarative config file | Aligns with "one config, one command" philosophy; no indexer code to write |
