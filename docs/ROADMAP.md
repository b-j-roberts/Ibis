# Ibis Development Roadmap

---

## Phase 1: Project Setup

### 1.1 Go Module and Project Scaffolding

**Description**: Initialize the Go module, set up the monorepo directory structure, and configure tooling. This establishes the foundation for all development.

**Requirements**:
- [ ] `go mod init github.com/b-j-roberts/ibis`
- [ ] Create directory structure: `cmd/ibis/`, `internal/{abi,api,cli,config,engine,provider,store,schema,types}/`, `configs/`, `scripts/`, `docs/`
- [ ] Stub `cmd/ibis/main.go` with cobra root command
- [ ] Add cobra, starknet.go, badger, pgx, yaml.v3 dependencies
- [ ] Create `.gitignore` for Go binaries, `data/` dir, `.env` files

**Implementation Notes**:
- Use `github.com/spf13/cobra` for CLI scaffolding
- Pin starknet.go to latest stable release
- BadgerDB v4 (`github.com/dgraph-io/badger/v4`)
- pgx v5 (`github.com/jackc/pgx/v5`)

### 1.2 Makefile and Docker Setup

**Description**: Create the Makefile with standard targets and Docker configuration for development and production deployments.

**Requirements**:
- [ ] Makefile with targets: `dev`, `build`, `run`, `test`, `check`, `fmt`, `vet`, `lint`, `docker-build`, `docker-run`, `docker-compose-up`, `docker-compose-down`
- [ ] `Dockerfile` (multi-stage: build + distroless runtime)
- [ ] `docker-compose.yaml` with ibis + postgres services
- [ ] `.env.example` with all configurable environment variables
- [ ] `scripts/install.sh` for binary installation

**Implementation Notes**:
- `make dev` should run the indexer with hot reload (use `air` or similar)
- `make build` should produce a single static binary at `./bin/ibis`
- Docker image should be minimal (distroless or alpine-based)
- docker-compose should mount `ibis.config.yaml` from host

### 1.3 CLI Framework and Config Loader

**Description**: Implement the cobra CLI with `init`, `run`, and `query` subcommands, and the YAML config loader with validation and environment variable expansion.

**Requirements**:
- [ ] `ibis init` command stub (will be fully implemented in MVP)
- [ ] `ibis run` command that loads config and starts the indexer
- [ ] `ibis query` command stub (will be fully implemented in MVP)
- [ ] Config struct definition matching the `ibis.config.yaml` schema from SPEC.md
- [ ] YAML loading with `${VAR_NAME}` environment variable expansion
- [ ] Config validation (required fields, valid enum values, contract address format)
- [ ] `--config` flag to specify config path (default: `./ibis.config.yaml`)
- [ ] Example `configs/ibis.config.yaml` with documented fields

**Implementation Notes**:
- Reference zindex's `internal/config/config.go` for env var expansion pattern
- Validation should fail fast with clear error messages pointing to the problematic field
- Config should support both WSS and HTTP RPC URLs (detect from scheme)

### 1.4 CI/CD Pipeline

**Description**: Set up GitHub Actions for automated testing, linting, and release builds.

**Requirements**:
- [ ] GitHub Actions workflow for `test`, `vet`, `lint` on push/PR
- [ ] `golangci-lint` configuration (`.golangci.yml`)
- [ ] Release workflow: build binaries for linux/darwin (amd64/arm64), create GitHub release
- [ ] Docker image build and push to GitHub Container Registry on tag

**Implementation Notes**:
- Use `goreleaser` for cross-platform binary builds and release automation
- Cache Go modules and build cache in CI for speed

### 1.5 Project Documents

**Description**: Create the essential project root documents — README.md, CLAUDE.md, and MIT LICENSE. The README establishes the project vision with architecture overview, feature list, and usage examples. CLAUDE.md provides Claude Code with project-specific context for effective AI-assisted development. The LICENSE file formalizes open-source terms.

**Requirements**:
- [ ] `README.md` with project overview, problem statement, feature highlights, architecture diagram (from SPEC.md), installation instructions (binary release, asdf plugin, build from source), usage examples (`ibis init`, `ibis run`, `ibis query`), config file example, API example, contributing pointer, and license badge
- [ ] `CLAUDE.md` with build/test/lint commands (from Makefile targets), project structure overview, key architectural patterns (store interface, operation pairs, ABI-driven schemas), tech stack summary, and pointers to SPEC.md and ROADMAP.md for deeper context
- [ ] `LICENSE` file with full MIT License text, copyright `2025 Brandon Roberts`
- [ ] README should reference the MIT license with a badge at the top
- [ ] CLAUDE.md should be usable immediately once task 1.1 (Go module scaffolding) and 1.2 (Makefile) land — reference the planned commands even if not yet implemented

**Implementation Notes**:
- Pull architecture diagram and feature descriptions from `docs/SPEC.md` — keep README concise and link to SPEC for deep dives
- CLAUDE.md follows the pattern from Brandon's workspace root `CLAUDE.md` — table of projects, key commands, architecture notes
- For the README, use shields.io badges for license, Go version, and build status (GitHub Actions URL can be placeholder until 1.4 lands)
- Keep README under ~200 lines — enough to sell the project vision without overwhelming; link to `docs/` for details
- Installation section should include asdf setup: `asdf plugin add ibis https://github.com/b-j-roberts/asdf-ibis.git && asdf install ibis latest`

---

## Phase 2: MVP

### 2.1 Store Interface and BadgerDB Backend

**Description**: Define the database abstraction interface and implement the BadgerDB backend. This is the foundational storage layer that all other components depend on.

**Requirements**:
- [ ] `Store` interface in `internal/store/store.go` with methods: `ApplyOperations`, `RevertOperations`, `GetEvents`, `GetUniqueEvents`, `GetAggregation`, `GetCursor`, `SetCursor`, `CreateTable`, `MigrateTable`, `Close`
- [ ] `Operation` type with `Insert`, `Update`, `Delete` variants and revert data
- [ ] `Query` type with pagination (`limit`, `offset`), ordering, and field filters
- [ ] BadgerDB implementation using key prefix patterns (reference foc-engine's `standalone/storage.go`)
- [ ] Primary index: `evt:{table}:{block}:{logIndex}`
- [ ] Unique index: `unq:{table}:{uniqueKey}`
- [ ] Reverse index for descending queries: `rev:{table}:{invertedBlock}:{logIndex}`
- [ ] Cursor persistence: `meta:cursor` key
- [ ] Unit tests for all store operations including revert

**Implementation Notes**:
- Use BadgerDB v4's `WriteBatch` for atomic multi-key writes
- Big-endian encode block numbers for correct prefix scan ordering
- Invert block numbers (`math.MaxUint64 - blockNum`) for reverse index
- Reference foc-engine's storage.go for the multi-index pattern

### 2.2 In-Memory Store Backend

**Description**: Implement the in-memory store for development and testing. Zero external dependencies, fast iteration.

**Requirements**:
- [ ] In-memory implementation of the `Store` interface using Go maps
- [ ] Thread-safe with `sync.RWMutex`
- [ ] Support all query operations (pagination, ordering, filtering)
- [ ] Support unique and aggregation table types
- [ ] Unit tests

**Implementation Notes**:
- Keep it simple: `map[string][]IndexedEvent` keyed by table name
- Sorting done in-memory on query
- No persistence -- data lost on restart (by design)

### 2.3 ABI Parser and Event Decoder

**Description**: Build the Cairo ABI parser and event decoder that translates raw Felt arrays into typed Go data. This is the core intelligence of the indexer.

**Requirements**:
- [ ] Parse Cairo ABI JSON files into Go type definitions
- [ ] Support all Cairo primitive types: `felt252`, `u8`-`u256`, `i8`-`i128`, `bool`, `ContractAddress`, `ClassHash`, `ByteArray`
- [ ] Support composite types: `Array<T>`, `Span<T>`, structs, enums
- [ ] Support snapshot types: `@Array<T>`, `@Span<T>`
- [ ] Compute event selectors via `starknet_keccak` (using `utils.GetSelectorFromNameFelt`)
- [ ] Match incoming event `keys[0]` to ABI event definitions
- [ ] Decode `keys[]` and `data[]` Felt arrays into typed `map[string]any`
- [ ] Handle nested struct members recursively
- [ ] Unit tests with real Starknet contract ABIs and known event data

**Implementation Notes**:
- Reference foc-engine's `internal/registry/parser.go` for the visitor pattern and type matching
- Cairo events encode some fields in `keys` and others in `data` -- the ABI specifies which via `kind: "key"` vs `kind: "data"` on members
- `ByteArray` decoding requires special handling (length prefix + 31-byte chunks + pending word)
- Use `starknet.go/utils.GetSelectorFromNameFelt()` for selector computation

### 2.4 ABI Resolution System

**Description**: Implement the three-tier ABI resolution: explicit path, smart local discovery, and chain fetch.

**Requirements**:
- [ ] Load ABI from explicit file path (e.g., `./target/dev/contract.contract_class.json`)
- [ ] Smart local discovery: given a contract name, search `target/dev/` for matching `*.contract_class.json`
- [ ] Scarb integration: if local ABI not found, attempt `scarb build` and retry discovery
- [ ] Chain fetch: use `provider.ClassAt(address)` to fetch ABI from deployed contract
- [ ] Cache resolved ABIs in memory for the session
- [ ] Clear error messages when ABI resolution fails at all levels

**Implementation Notes**:
- Scarb build output goes to `target/dev/{package}_{ContractName}.contract_class.json`
- Chain-fetched ABIs may be from proxy contracts -- document that users should use local ABIs for proxies
- ABI resolution happens once at startup, not per-block

### 2.5 Starknet Provider and Event Subscriber

**Description**: Build the Starknet provider wrapper with per-contract event subscription via WSS, automatic reconnection, HTTP RPC polling fallback, and backfill support. Follows foc-engine's event-driven approach (not block scanning).

**Requirements**:
- [ ] WSS event subscription using `starknet_subscribeEvents` per contract (via `starknet.go`'s `rpc.WsProvider` or raw gorilla/websocket JSON-RPC)
- [ ] Subscription params: `from_address` (contract), `block_id` (starting block number), optional `keys` filter
- [ ] HTTP provider using `starknet.go`'s `rpc.Provider` for ABI fetching and backfill
- [ ] Backfill via `starknet_getEvents` HTTP RPC with continuation tokens (chunk_size: 1000) in configurable block ranges (default: 100 blocks per query)
- [ ] Automatic reconnection with exponential backoff (1s to 30s) -- on WSS error, wait, reconnect, re-subscribe from last processed block
- [ ] Polling fallback: if WSS connection fails, fall back to `starknet_getEvents` polling loop with adaptive timing (100ms during catchup, 2s at chain tip)
- [ ] Auto-detect WSS vs HTTP from RPC URL scheme and convert if needed (http->ws, https->wss)
- [ ] Handle subscription errors gracefully (log + reconnect)
- [ ] Unit tests with mock WebSocket server

**Implementation Notes**:
- Reference foc-engine's `internal/indexer/standalone/websocket.go` for the subscription pattern: `starknet_subscribeEvents` with `block_id.block_number` and `from_address` params
- Reference foc-engine's `internal/indexer/standalone/polling.go` for the polling fallback and adaptive timing
- Reference foc-engine's `internal/indexer/standalone/rpc.go` for `starknet_getEvents` with continuation tokens
- starknet.go's WsProvider may provide `SubscribeEvents` directly -- use it if available, otherwise use raw gorilla/websocket JSON-RPC calls (like foc-engine does)
- The `sub.Err()` channel (if using starknet.go) signals subscription failures
- For multiple contracts, create one subscription per contract and multiplex with `select`

### 2.6 Indexing Engine with Reorg Handling

**Description**: Implement the core indexing orchestrator that receives events from the subscriber, decodes them via ABI, generates revert/add operation pairs, and handles chain reorganizations. The engine is event-driven (not block-scanning) -- it processes individual events as they arrive from `starknet_subscribeEvents`.

**Requirements**:
- [ ] Main event loop: receive events from WSS subscriber (or polling fallback), decode, write to store
- [ ] Event processing pipeline: match `keys[0]` selector -> find ABI definition -> decode fields -> generate operations -> write to store
- [ ] Generate `(add, revert)` operation pairs for every database write
- [ ] Store pending block operations keyed by block number for potential revert
- [ ] Reorg handling: if WSS subscription delivers reorg notifications, execute revert operations for blocks in the orphaned range. Investigate whether `starknet_subscribeEvents` delivers reorg data inline (like `starknet_subscribeNewHeads` does) -- if not, rely on linear forward progression with cursor-based resume (proven pattern from foc-engine)
- [ ] On pending block replacement: revert previous pending block ops, apply new ones
- [ ] Promote pending operations to confirmed (discard revert data) after a configurable confirmation depth
- [ ] Configurable start block: `0` for latest, specific number, or resume from cursor
- [ ] Starting block determination: `max(persisted_cursor + 1, config_start_block)` (same as foc-engine)
- [ ] Cursor persistence: update `last_processed_block` after each event batch
- [ ] Graceful shutdown: finish current event, persist cursor, close connections
- [ ] Integration tests with simulated reorg scenarios

**Implementation Notes**:
- Keep a `map[uint64][]Operation` of pending block operations in memory
- When a block is confirmed (sufficient depth), its revert operations can be discarded
- Reorg investigation: starknet.go's `SubscribeEvents` may or may not emit `ReorgData` inline -- needs testing against a real node. If it does, use it. If not, the foc-engine approach of linear progression without reorg detection is acceptable for MVP
- For unique tables, the revert operation must restore the previous unique entry value
- For aggregation tables, the revert operation must subtract the aggregated values
- Reference foc-engine's `internal/indexer/standalone/indexer.go` for the startup sequence and `determineStartingBlock` pattern

### 2.7 Schema Generator

**Description**: Generate table schemas from config + ABI definitions. These schemas drive table creation, API generation, and query execution. Supports wildcard event selection.

**Requirements**:
- [ ] Parse config `events` section to determine table type (`log`, `unique`, `aggregation`)
- [ ] Wildcard `"*"` support: when `name: "*"` is configured, expand to all events in the contract's ABI using the wildcard's table type as default
- [ ] Specific event entries override the wildcard default for that event
- [ ] If no wildcard, only explicitly listed events generate schemas
- [ ] Map ABI event member types to database column types (felt252 -> string, u64 -> int64, bool -> bool, etc.)
- [ ] Generate `TableSchema` with columns, indices, unique keys, and aggregate specs
- [ ] For Postgres: generate `CREATE TABLE` SQL with appropriate column types and indices
- [ ] For BadgerDB: generate key prefix patterns and secondary index definitions
- [ ] Add standard metadata columns: `block_number`, `transaction_hash`, `log_index`, `timestamp`, `status`
- [ ] Unit tests including wildcard expansion scenarios

**Implementation Notes**:
- Cairo `felt252` and `ContractAddress` map to string/text (hex representation)
- Cairo `u64` and below map to int64; `u128` and `u256` map to string (too large for int64)
- `ByteArray` maps to text
- Unique tables need a unique index on the configured `unique_key` column
- Aggregation tables need a separate aggregation tracking table
- Wildcard expansion: iterate ABI event definitions, create a schema for each, then apply any specific overrides from the config

### 2.8 PostgreSQL Store Backend

**Description**: Implement the PostgreSQL store backend with auto-generated table creation from schemas and efficient query execution.

**Requirements**:
- [ ] PostgreSQL implementation of the `Store` interface using `pgx/v5` connection pool
- [ ] Auto-create tables from `TableSchema` on startup (if not exists)
- [ ] Auto-migrate tables when schema changes (add new columns, never drop)
- [ ] `ApplyOperations`: batch INSERT/UPDATE/DELETE within a transaction
- [ ] `RevertOperations`: execute inverse operations within a transaction
- [ ] `GetEvents`: SELECT with pagination, ordering, and field filters
- [ ] `GetUniqueEvents`: SELECT with GROUP BY unique key, keep latest per group
- [ ] `GetAggregation`: SELECT with SUM/COUNT/AVG as configured
- [ ] Connection pool configuration from `ibis.config.yaml`
- [ ] Integration tests with testcontainers

**Implementation Notes**:
- Use `pgx/v5/pgxpool` for connection pooling
- Use `ON CONFLICT` for unique table upserts
- For aggregation, maintain a separate `{table}_agg` table that gets updated on each event
- Reference zindex's `internal/db/postgres/postgres.go` for pool setup patterns

### 2.9 Auto-Generated REST API

**Description**: Generate REST endpoints from ABI-derived table schemas with Supabase-style query syntax.

**Requirements**:
- [ ] For each configured event: generate CRUD-like REST endpoints (list, get, latest, count, filter)
- [ ] URL pattern: `/v1/{contract_name}/{event_name}`
- [ ] Pagination: `?limit=50&offset=0` (default limit: 50, max: 500)
- [ ] Ordering: `?order=block_number.desc` (default: `block_number.desc`)
- [ ] Field filtering: `?{field}=eq.{value}`, operators: `eq`, `neq`, `gt`, `gte`, `lt`, `lte`
- [ ] Unique table endpoint: `/v1/{contract_name}/{event_name}/unique`
- [ ] Aggregation endpoint: `/v1/{contract_name}/{event_name}/aggregate`
- [ ] Health check: `GET /v1/health`
- [ ] Status endpoint: `GET /v1/status` (current block, sync status, configured contracts)
- [ ] JSON response with metadata: `{ "data": [...], "count": N, "limit": 50, "offset": 0 }`
- [ ] CORS middleware (configurable allow origins)
- [ ] Request logging middleware
- [ ] Integration tests

**Implementation Notes**:
- Use Go 1.22+ `net/http` routing with method+path patterns (`GET /v1/{contract}/{event}`)
- Parse Supabase-style filter params into the `Query` type from the store interface
- Content-Type: `application/json` for all responses
- Return appropriate HTTP status codes (200, 400, 404, 500)

### 2.10 SSE Event Streaming

**Description**: Implement Server-Sent Events endpoints for real-time event delivery to client applications.

**Requirements**:
- [ ] SSE endpoint: `GET /v1/{contract_name}/{event_name}/stream`
- [ ] Stream new events as they are indexed in real-time
- [ ] Support `Last-Event-ID` header for reconnection and replay
- [ ] Event format: `id: {block}:{logIndex}\ndata: {json}\n\n`
- [ ] Filter support: same query params as REST endpoints applied to the stream
- [ ] Client connection tracking and cleanup on disconnect
- [ ] Graceful shutdown: close all SSE connections

**Implementation Notes**:
- Use `http.Flusher` interface for streaming
- Maintain an in-memory event bus (channel-based) that the engine publishes to and SSE handlers subscribe to
- Set headers: `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`
- Use `context.Done()` to detect client disconnection

### 2.11 `ibis init` Command

**Description**: Implement the `ibis init` command that scaffolds an `ibis.config.yaml` from user input and contract inspection.

**Requirements**:
- [ ] Interactive prompts: network selection, RPC URL, database backend
- [ ] Accept contract address as argument: `ibis init --contract 0x123...`
- [ ] Fetch ABI from chain and list available events
- [ ] Prompt user to select which events to index
- [ ] Prompt for table type per event (log/unique/aggregation)
- [ ] For unique tables: prompt for unique key field
- [ ] For aggregation tables: prompt for aggregate fields and operations
- [ ] Generate `ibis.config.yaml` in current directory
- [ ] Support `--output` flag for custom output path
- [ ] Non-interactive mode with flags for CI/scripting

**Implementation Notes**:
- Use `github.com/AlecAivazis/survey/v2` or similar for interactive prompts
- This is where the AI-powered config generation skill will hook in later
- Support multiple `--contract` flags to configure multiple contracts in one init

### 2.12 `ibis query` Command

**Description**: Implement the `ibis query` CLI for querying indexed data from the terminal without needing the API server.

**Requirements**:
- [ ] `ibis query {contract} {event}` -- list events
- [ ] `--limit`, `--offset`, `--order` flags for pagination
- [ ] `--filter {field}={value}` flag for field filtering
- [ ] `--unique` flag for unique table queries
- [ ] `--aggregate` flag for aggregation queries
- [ ] `--format` flag: `json` (default), `table`, `csv`
- [ ] Connects directly to the configured database (no API server needed)
- [ ] `ibis query --list` to show all available tables/events

**Implementation Notes**:
- Reads `ibis.config.yaml` for database connection settings
- Reuses the same `Store` interface and `Query` type as the API server
- Table format using `github.com/olekukonez/tablewriter` or similar

---

## Phase 3: Nice to Have

### 3.1 Natural Language Queries

**Description**: Enable users to query indexed data using natural language via a Claude Code skill. Users can ask questions like "who has the highest score?" or "show me all trades above 1000 STRK in the last hour" and get results. The skill must understand the full `ibis query` CLI, REST API, and all table types to translate natural language into precise queries.

**Requirements**:
- [ ] Claude Code skill that understands available tables, schemas, and column types
- [ ] Translates natural language to `ibis query` commands or direct REST API calls
- [ ] Returns formatted results with context
- [ ] Handles follow-up questions with conversation context
- [ ] Skill can inspect `ibis.config.yaml` to understand the indexed data model
- [ ] Supports all three table types: `log` (append-only events), `unique` (last-write-wins by key), `aggregation` (auto-computed sums/counts/averages)
- [ ] Supports factory-aware queries: listing child contracts (`--children`), counting children (`--children-count`), filtering shared tables by child address (`--contract-address`)
- [ ] Understands metadata columns always present on events: `block_number`, `transaction_hash`, `log_index`, `timestamp`, `contract_address`, `event_name`, `status` (PRE_CONFIRMED|ACCEPTED_L2|ACCEPTED_L1), and `contract_name` (on shared tables)

**Implementation Notes**:
- Build as a Claude Code skill (not embedded in the ibis binary)
- Skill reads and/or queries the config and schema to understand what data is available
- Skill can be used outside of context of ibis project (from another project using ibis executable) and can still build intelligent queries for all use cases
- **CLI query command** (`ibis query [contract] [event]`) supports these flags:
  - `--limit N` (default 50), `--offset N` (default 0) — pagination
  - `--order field.asc|desc` (default `block_number.desc`) — ordering by any column
  - `--filter "field=op.value"` (repeatable, AND semantics) — operators: `eq`, `neq`, `gt`, `gte`, `lt`, `lte`; bare `field=value` defaults to `eq`
  - `--unique` — query unique table entries (last-write-wins by unique_key)
  - `--aggregate` — query aggregation results (returns computed values, not individual events)
  - `--latest` — return only the most recent event
  - `--count` — return count of matching events
  - `--children` — list factory child contracts (usage: `ibis query <factory> --children`)
  - `--children-count` — count factory child contracts
  - `--contract-address 0x...` — filter shared factory tables by specific child contract address
  - `--format json|table|csv` (default `json`) — output format
  - `--list` — list all available tables/events from config
- **REST API endpoints** (when server is running via `ibis run`):
  - `GET /v1/{contract}/{event}` — list events with `?limit=`, `?offset=`, `?order=field.dir`, `?field=op.value` filters
  - `GET /v1/{contract}/{event}/latest` — get most recent event
  - `GET /v1/{contract}/{event}/count` — count matching events
  - `GET /v1/{contract}/{event}/unique` — unique table entries
  - `GET /v1/{contract}/{event}/aggregate` — aggregation results (`{values: {column: value}}`)
  - `GET /v1/{contract}/{event}/stream` — SSE real-time stream (supports Last-Event-ID for replay)
  - `GET /v1/{factory}/children` — list child contracts with metadata (name, address, deployment_block, current_block, events, plus factory event fields like token0/token1)
  - `GET /v1/{factory}/children/count` — count factory children
  - `GET /v1/status` — indexer status (block heights, sync progress)
- **Query syntax** follows Supabase-style: `?field=op.value` (e.g., `?block_number=gte.100000&status=eq.ACCEPTED_L2`)
- Table names are derived as `lowercase({contract}_{event})` — skill should construct these from config
- Factory children queries support metadata filtering (e.g., `--filter "token0=eq.0x053c91..."` or `?token0=eq.0x053c91...`)
- For shared tables, multiple factory children write to the same table — use `--contract-address` or `?contract_address=eq.0x...` to filter by specific child


### 3.2 AI-Powered Config Generation

**Description**: A Claude Code skill that generates `ibis.config.yaml` from a contract address or natural language description. "Index all swap events from this AMM contract" -> full config file. The skill must understand the complete config schema including factory contracts, shared tables, all table types, and ABI resolution strategies.

**Requirements**:
- [ ] Claude Code skill that accepts a contract address and network
- [ ] Fetches and analyzes the contract ABI (supports Cairo types: felt252, u8-u256, i8-i128, bool, ContractAddress, ClassHash, ByteArray, Array/Span, structs, enums)
- [ ] Suggests which events to index based on event names and data structures
- [ ] Recommends table types: `log` (append-only history), `unique` (last-write-wins, requires `unique_key` field), `aggregation` (auto-computed, requires `aggregate` config with `column`/`operation`/`field`)
- [ ] Supports aggregation operations: `sum`, `count`, `avg` — skill should recommend appropriate aggregations based on numeric fields
- [ ] Generates complete `ibis.config.yaml` with all sections: `network`, `rpc`, `database`, `api`, `indexer`, `contracts`
- [ ] Supports iterative refinement ("also add the Transfer events", "make the price table unique by pair_id", "add a factory for this AMM")
- [ ] Detects factory patterns and generates full factory configuration: `factory.event`, `child_address_field`, `child_abi`, `child_events`, `shared_tables`, `child_name_template`
- [ ] If skill used inside a contract directory, inspects contract source code for deeper understanding and enhanced recommendations
- [ ] Supports wildcard events (`name: "*"`) for indexing all ABI events with a single table type

**Implementation Notes**:
- Build as a Claude Code skill that calls the Starknet RPC to fetch ABIs
- Use event naming conventions to infer table types (e.g., "Updated"/"Changed" → unique, "Created"/"Emitted" → log, volume/count fields → aggregation)
- Skill can run `ibis init` under the hood or generate the YAML directly
- Skill can be used outside of context of ibis project (from another project using ibis executable) and can still generate configs for all use cases
- **Full config schema** the skill must be able to generate:
  ```yaml
  network: mainnet|sepolia|custom
  rpc: wss://... or https://...          # WSS preferred (enables starknet_subscribeEvents), HTTP falls back to polling

  database:
    backend: postgres|badger|memory      # postgres for production, badger for embedded, memory for testing
    postgres:
      host: localhost
      port: 5432                         # default
      user: ibis
      password: ${IBIS_DB_PASSWORD}      # env var expansion with ${VAR_NAME} syntax
      name: ibis
    badger:
      path: ./data/ibis                  # default

  api:
    host: 0.0.0.0                        # default
    port: 8080                           # default
    cors_origins: ["*"]                  # optional CORS allow list
    admin_key: ${ADMIN_KEY}              # optional, required for /v1/admin/* endpoints

  indexer:
    start_block: 0                       # 0 = latest block, or specific block number
    pending_blocks: true                 # index pre-confirmed (pending) blocks
    batch_size: 10                       # blocks per backfill query (default)

  contracts:
    - name: MyContract                   # unique identifier, used in API paths and table names
      address: "0x049d..."               # Starknet contract address
      abi: fetch|./path/to/abi.json|Name # resolution: fetch from chain, file path, or smart local discovery
      start_block: 0                     # optional per-contract override (each contract gets independent cursor)

      events:
        - name: Transfer                 # specific event name, or "*" for all ABI events
          table:
            type: log                    # append-only event log
        - name: LeaderboardUpdate
          table:
            type: unique
            unique_key: player_address   # field for last-write-wins deduplication
        - name: VolumeUpdate
          table:
            type: aggregation
            aggregate:
              - column: total_volume     # output column name
                operation: sum           # sum|count|avg
                field: amount            # source event field

      # Factory configuration (optional — for contracts that deploy child contracts)
      factory:
        event: PairCreated               # factory event that signals child deployment
        child_address_field: pair         # event field containing deployed child address
        child_abi: fetch|path|Name       # ABI for children (cached after first resolution)
        shared_tables: true              # all children write to same tables (adds contract_name column)
        child_name_template: "{factory}_{short_address}"  # supports {factory}, {short_address}, {event_field_names}
        child_events:                    # event/table config template applied to each child
          - name: "*"
            table:
              type: log
          - name: Swap
            table:
              type: aggregation
              aggregate:
                - column: total_volume
                  operation: sum
                  field: amount_in
  ```
- **ABI resolution priority**: 1) explicit file path, 2) smart local discovery (searches `target/dev/` for matching contract names), 3) fetch from chain via RPC
- **Per-contract cursors**: each contract (including factory children) tracks its own last-processed block independently, enabling targeted backfill at different rates
- **Factory children** get their deployment block as `start_block` and their own cursor for independent catch-up
- **Shared tables**: when `shared_tables: true`, children write to factory-named tables (e.g., `uniswapv2factory_swap` instead of `pair_abc123_swap`); each row includes a `contract_name` column identifying the child
- **Dynamic contract registration**: contracts can also be added at runtime via `POST /v1/admin/contracts` — the skill should note this capability but focus on config file generation
- Metadata columns are auto-added to all tables: `block_number`, `transaction_hash`, `log_index`, `timestamp`, `contract_address`, `event_name`, `status`


### 3.3 asdf Plugin

**Description**: Create an asdf plugin for easy version management and installation of the ibis binary.

**Requirements**:
- [ ] asdf plugin repository with `bin/list-all`, `bin/download`, `bin/install` scripts
- [ ] Download pre-built binaries from GitHub releases
- [ ] Support for linux and darwin (amd64 and arm64)
- [ ] `.tool-versions` integration: `ibis 0.1.0`
- [ ] Publish plugin to asdf plugin registry

**Implementation Notes**:
- Follow the asdf plugin creation guide
- Binaries come from the GitHub release workflow (goreleaser)
- Plugin should verify checksums on download
- Create the asdf plugin repo at ../ibis-asdf

### 3.4 Contract Groups & Namespacing

**Description**: Group related contracts under a logical namespace in config. Groups are reflected in API URL prefixes, enabling cleaner organization for multi-contract deployments (e.g., grouping all DEX contracts under a `dex` namespace). This is a foundational organizational primitive that later tasks (cross-contract queries, factory APIs) build on.

**Requirements**:
- [ ] Add optional `group` field to `ContractConfig` in config struct
- [ ] Validate group names (lowercase alphanumeric + hyphens, no special characters)
- [ ] API URL prefix includes group when present: `/v1/{group}/{contract}/{event}`
- [ ] Ungrouped contracts retain current URL pattern: `/v1/{contract}/{event}`
- [ ] Status endpoint (`/v1/status`) shows contracts organized by group
- [ ] Table names optionally prefixed with group: `{group}_{contract}_{event}`
- [ ] Config validation: no duplicate contract names within the same group
- [ ] SSE streaming respects group prefix: `/v1/{group}/{contract}/{event}/stream`

**Implementation Notes**:
- Config shape: `contracts: [{ name: MyDEX, group: dex, address: "0x...", ... }]`
- The group field is optional
- API route registration: if group is set, register `GET /v1/{group}/{contract}/{event}`, otherwise `GET /v1/{contract}/{event}`
- Schema generator: when group is present, `BuildTableSchema` uses `{group}_{contract}_{event}` as table name
- Reserve the group name `_all` (used in 3.5 for cross-contract queries)

### 3.5 Cross-Contract Queries

**Description**: Enable querying events across multiple contracts in a single API request. Supports both group-wide queries (all contracts in a group) and explicit multi-contract queries, returning unified results with contract attribution.

**Requirements**:
- [ ] Group-level query endpoint: `GET /v1/{group}/_all/{event}` returns events of that type from all contracts in the group
- [ ] Multi-contract query parameter: `?contracts=ContractA,ContractB` on any event endpoint
- [ ] Results include `contract_name` and `contract_address` fields for disambiguation
- [ ] Pagination, ordering, and filtering work across the unified result set
- [ ] Count endpoint works across contracts: `GET /v1/{group}/_all/{event}/count`
- [ ] SSE streaming across contracts: `GET /v1/{group}/_all/{event}/stream`
- [ ] `ibis query` CLI supports `--contracts` flag for cross-contract queries

**Implementation Notes**:
- For Postgres: use `UNION ALL` across contract tables with an added `contract_name` column, apply `ORDER BY` and `LIMIT` on the union
- For BadgerDB: merge-sort results from multiple prefix scans using a min-heap
- For in-memory: concatenate and re-sort
- The `_all` path segment is a reserved name — validate that no contract is named `_all` in config validation
- Cross-contract queries on different table types (log vs unique) should return an error — only same-type tables can be unioned
- If shared tables (3.11) are in use, cross-contract queries become simple `WHERE` clauses instead of unions

### 3.6 Proxy Contract Support

**Description**: Detect common Starknet proxy patterns and automatically resolve implementation ABIs. When a contract is a proxy, ibis fetches the implementation's ABI instead of the proxy's minimal ABI.

**Requirements**:
- [ ] Auto-detect proxies when `abi: fetch` is used: if the fetched ABI has very few events and contains upgrade-related functions, attempt implementation resolution
- [ ] Use `starknet_getClassHashAt(address)` to get the current class hash, then `starknet_getClass(class_hash)` to fetch the full implementation ABI
- [ ] Support explicit `implementation` field in contract config for manual proxy resolution: `implementation: "0x..."` (class hash or contract address)
- [ ] Config option to disable auto-proxy-detection: `proxy: false` on a contract entry
- [ ] Log a warning when proxy detection is used, noting that the implementation can change at runtime
- [ ] If proxy resolution fails, fall back to the proxy's own ABI with a warning
- [ ] Unit tests with mock proxy ABIs (minimal proxy ABI + full implementation ABI)

**Implementation Notes**:
- `starknet_getClassHashAt(address)` returns the current class hash for any contract — this is the simplest approach, no storage slot guessing needed
- Proxy detection heuristic: if the fetched ABI has fewer than 3 event definitions and contains `upgrade` or `set_implementation` functions, it's likely a proxy
- For UUPS-style proxies (OpenZeppelin Upgradeable), the class hash at the proxy address IS the implementation's class hash
- Document that users should prefer explicit ABI paths for proxies with known implementations
- Proxy resolution runs once at startup; for runtime upgrades, see task 3.13 (Contract Upgrade Tracking)

### 3.7 Dynamic Contract Registration

**Description**: Add and remove indexed contracts at runtime via the HTTP API, without restarting the indexer. This is the foundation for factory auto-registration (3.10) and class hash discovery (3.9).

**Requirements**:
- [ ] `POST /v1/admin/contracts` — register a new contract with full config (name, address, ABI source, events, group)
- [ ] `DELETE /v1/admin/contracts/{name}` — deregister a contract (stops indexing, optionally drops tables via `?drop_tables=true`)
- [ ] `GET /v1/admin/contracts` — list all registered contracts with status (active, syncing, error, backfilling)
- [ ] `PUT /v1/admin/contracts/{name}` — update contract config (e.g., add new events to index)
- [ ] New contracts start indexing from a specified `start_block` (or latest if omitted)
- [ ] Engine hot-adds a new WSS subscription for the contract without disrupting existing subscriptions
- [ ] Persist dynamic registrations in the database so they survive restarts
- [ ] Admin endpoints protected by optional API key (`api.admin_key` in config)
- [ ] API routes for new contract's events are registered dynamically
- [ ] Integration tests for full add/remove/restart lifecycle

**Implementation Notes**:
- The engine needs a thread-safe `RegisterContract(config)` method that: resolves ABI, builds schemas, creates tables, spawns subscription goroutine, registers API routes
- Use a channel or mutex-protected method that the API handler calls to signal the engine
- For WSS: spawn a new subscription goroutine for the new contract via the existing `subscribeContract` pattern
- For persistence: store dynamic contracts in a `_ibis_contracts` metadata table (Postgres) or `meta:contracts:{name}` prefix (Badger)
- On restart: load both static (config file) and dynamic (DB) contracts; static config takes precedence on conflicts
- Depends on per-contract cursors (3.8) — new contracts need their own cursor, not the global one

### 3.8 Per-Contract Cursors & Targeted Backfill

**Description**: Replace the single global cursor with per-contract cursor tracking. Each contract independently tracks its last processed block, enabling new contracts to backfill from their deployment block without re-indexing existing contracts. This is a prerequisite for dynamic registration (3.7), factory indexing (3.10), and class hash discovery (3.9).

**Requirements**:
- [ ] Store per-contract cursor: `GetCursor(ctx, contract)` and `SetCursor(ctx, contract, blockNumber)`
- [ ] Maintain a derived global cursor (`min(all contract cursors)`) for overall sync status reporting
- [ ] On startup, each contract resumes from `max(its_persisted_cursor + 1, config_start_block)`
- [ ] New dynamically-added contracts start from their specified start block regardless of other contracts' positions
- [ ] Backfill runs per-contract: each contract can independently catch up via `starknet_getEvents`
- [ ] Status endpoint shows per-contract sync progress: current block, target block, sync percentage, status

**Implementation Notes**:
- Postgres: create `_ibis_cursors` table with `(contract_name TEXT PRIMARY KEY, last_block BIGINT, updated_at TIMESTAMP)`
- BadgerDB: use `meta:cursor:{contract_name}` keys instead of single `meta:cursor`
- In-memory: `map[string]uint64` for cursors
- The global cursor becomes `min(all contract cursors)` — used for `/v1/status` overall block height
- Engine startup sequence changes: instead of one `determineStartingBlock`, each contract calls it independently
- Per-contract backfill can run concurrently (one goroutine per contract doing `starknet_getEvents` catchup), with rate limiting to avoid overwhelming the RPC node

### 3.9 Contract Discovery by Class Hash

**Description**: Watch for new contract deployments matching a specified class hash and automatically register them for indexing. Monitors the Universal Deployer Contract (UDC) for `ContractDeployed` events filtered by class hash. This enables indexing all instances of a specific contract type across the network.

**Requirements**:
- [ ] Config section for class hash watches:
  ```yaml
  discover:
    - class_hash: "0xabc123..."
      group: my_tokens          # optional: group discovered contracts
      abi: fetch                 # or explicit path
      events:
        - name: "*"
          table:
            type: log
  ```
- [ ] Subscribe to UDC events (`ContractDeployed`) and filter by `classHash` field in the event data
- [ ] Extract deployed contract address from UDC event and auto-register for indexing via dynamic registration (3.7)
- [ ] Apply the configured ABI and event template to all discovered contracts
- [ ] Backfill: on startup, scan historical UDC events for matching class hashes to discover existing contracts
- [ ] Discovered contracts tracked with deployment block for targeted backfill (3.8)
- [ ] Reorg safety: if a `ContractDeployed` event is reverted, deregister the child and revert its indexed events
- [ ] Auto-generate contract names: `{class_hash_short}_{address_short}` or configurable naming template

**Implementation Notes**:
- UDC address on mainnet/Sepolia: `0x04a64cd09a853868621d94cae9952b106f2c36a3f81260f85de6696c6b050221`
- UDC event: `ContractDeployed(address: ContractAddress, deployer: ContractAddress, unique: bool, classHash: ClassHash, calldata: Span<felt252>, salt: felt252)`
- Filter UDC subscription by `keys[0]` = `sn_keccak("ContractDeployed")` — then match `classHash` field in decoded event data
- This is a network-level discovery mechanism (vs. factory in 3.10 which is contract-specific)
- All discovered contracts share the same ABI (same class hash = same code)
- Depends on: dynamic registration (3.7) and per-contract cursors (3.8)
- Not all contracts are deployed via UDC — some use `deploy_syscall` directly from factory contracts (covered by 3.10)

### 3.10 Factory Contract Indexing

**Description**: Support factory contract patterns (JediSwap, 10kSwap, etc.) where a factory contract deploys child contracts and emits events like `PairCreated` containing the child's address. The indexer watches factory events and auto-registers child contracts for indexing using the factory's child configuration template.

**Requirements**:
- [ ] Config section for factory contracts:
  ```yaml
  contracts:
    - name: JediSwapFactory
      address: "0x..."
      abi: fetch
      factory:
        event: PairCreated              # factory event that signals a new child
        child_address_field: pair       # field in the event containing child address
        child_abi: fetch                # ABI source for children (or explicit path)
        child_events:                   # event/table config template for children
          - name: "*"
            table:
              type: log
        shared_tables: true             # children share tables (see 3.11)
  ```
- [ ] When a factory event fires, extract the child address from the specified field
- [ ] Auto-register the child contract using the factory's child config template (ABI, events, table types)
- [ ] Backfill: on startup, replay historical factory events to discover all existing children before starting live indexing
- [ ] Start indexing each child from its deployment block (the block where the factory event occurred)
- [ ] Handle constructor events: ensure child events from the same block as the factory event are captured
- [ ] Reorg safety: if a factory event is reverted, deregister the child and revert all its indexed events
- [ ] Track factory-child relationships in metadata for API and status reporting
- [ ] Support multiple factory contracts in one config
- [ ] Store additional factory event fields as child metadata (e.g., `token0`, `token1` from `PairCreated`)

**Implementation Notes**:
- Factory indexing is a two-phase startup: (1) scan factory for all historical child-creation events, (2) backfill each child from its deployment block
- The factory contract itself is also indexed — its events like `PairCreated` go into a normal log table
- Child contracts all share the same ABI (same class hash deployed by `deploy_syscall`)
- Child naming: auto-generate names like `{factory_name}_{short_address}` or use a template: `child_name_template: "{factory}_{token0}_{token1}"` referencing factory event fields
- `starknet_getEvents` only accepts one address per call — for N children, need N WSS subscriptions. For large factories (500+ children), consider subscription batching and backfill rate limiting
- The factory event handler runs in the engine's event processing pipeline, calling the dynamic registration capability (3.7) when a new child is detected
- Depends on: dynamic registration (3.7), per-contract cursors (3.8)
- Starknet factory examples: JediSwap V1 (`PairCreated` with `pair` field), JediSwap V2 (`PoolCreated` with `pool` field), 10kSwap (`PairCreated` with `pair` field)

### 3.11 Shared Tables for Same-ABI Contracts

**Description**: Instead of creating one table per contract per event (which causes schema explosion with 500+ factory children), allow contracts sharing the same ABI to write events into shared tables with a `contract_address` discriminator column. A JediSwap V1 factory with 500 pairs creates ~5-10 shared tables instead of ~2500-5000 per-contract tables.

**Requirements**:
- [ ] Config option on factory: `shared_tables: true` (shown in 3.10 config example)
- [ ] Config option on contract groups: `shared_tables: true` for manually-grouped same-ABI contracts
- [ ] When shared tables are enabled, all contracts in the factory/group write to one table per event type
- [ ] Add `contract_address` and `contract_name` columns to shared tables automatically
- [ ] Table naming for shared tables: `{factory_name}_{event_name}` (no per-child suffix)
- [ ] Unique tables: composite unique key becomes `(contract_address, original_unique_key)`
- [ ] Aggregation tables: support both per-contract and cross-contract aggregation modes
- [ ] Index on `contract_address` column for efficient per-child queries
- [ ] Schema creation deferred until first child registration (factory starts with 0 children)

**Implementation Notes**:
- Schema generator changes: when `shared_tables` is true, `BuildTableSchema` adds `contract_address TEXT NOT NULL` and `contract_name TEXT NOT NULL` columns, and adjusts unique constraints to be composite
- Postgres: `CREATE INDEX idx_{table}_contract ON {table}(contract_address)` for efficient per-child filtering
- The store's `ApplyOperations` needs no interface changes — it already operates on table names. The difference is that multiple contracts now target the same table name
- Postgres unique constraint: `UNIQUE(contract_address, {unique_key})` instead of `UNIQUE({unique_key})`
- Aggregation: per-contract aggregation uses `WHERE contract_address = ?` + `GROUP BY`; cross-contract drops the filter
- Shared tables make cross-contract queries (3.5) trivial — just query the shared table with optional `contract_address` filter, no `UNION ALL` needed
- BadgerDB shared tables: use key pattern `evt:{shared_table}:{contract_address}:{block}:{logIndex}`

### 3.12 Factory-Aware APIs & Queries

**Description**: API enhancements for querying factory-deployed contracts. Provides factory-scoped endpoints, per-child filtering, aggregate views across all children, and child metadata from factory events (e.g., token pairs).

**Requirements**:
- [ ] Factory children endpoint: `GET /v1/{factory}/children` — list all discovered child contracts with metadata (address, deployment block, sync status, factory event data)
- [ ] Per-child event query: `GET /v1/{factory}/{event}?contract_address=0x...` — filter shared table to one child
- [ ] Cross-child aggregation: `GET /v1/{factory}/{event}/aggregate` — aggregate across all children
- [ ] Per-child aggregation: `GET /v1/{factory}/{event}/aggregate?contract_address=0x...`
- [ ] Child count: `GET /v1/{factory}/children/count`
- [ ] SSE streaming with optional per-child filter: `GET /v1/{factory}/{event}/stream?contract_address=0x...`
- [ ] `ibis query` CLI: `ibis query {factory} {event} --contract-address 0x...`
- [ ] Child metadata in responses: include factory event fields (e.g., `token0`, `token1` for JediSwap pairs) as queryable/filterable attributes
- [ ] Status endpoint includes factory summary: child count, fully synced count, backfilling count

**Implementation Notes**:
- With shared tables (3.11), per-child queries are simple `WHERE contract_address = ?` filters — no unions needed
- Without shared tables, per-child queries route to the specific child's table, cross-child queries use `UNION ALL`
- The `/children` endpoint reads from `_ibis_factory_children` metadata table storing: `(factory_name, child_address, child_name, deployment_block, metadata_json, discovered_at)`
- `metadata_json` stores additional factory event fields (e.g., `{"token0": "0x...", "token1": "0x..."}`) — these become queryable: `GET /v1/jediswap/children?token0=0x...`
- SSE child filter: the EventBus subscriber checks `contract_address` field on events from shared tables
- Consider a `/v1/{factory}/pairs` alias endpoint for AMM-style factories (configurable endpoint name in factory config)

### 3.13 Contract Upgrade Tracking

**Description**: Track `replace_class_syscall` upgrades on indexed contracts. When a contract's class hash changes, re-fetch its ABI and update event decoding schemas at the upgrade boundary. Essential for long-running indexers on upgradeable contracts using OpenZeppelin's Upgradeable component.

**Requirements**:
- [ ] Config option per contract: `track_upgrades: true` (default: false)
- [ ] When enabled, also subscribe to `Upgraded(class_hash: ClassHash)` events from the contract
- [ ] On `Upgraded` event: fetch new ABI via `starknet_getClass(new_class_hash)`, rebuild event registry and schemas
- [ ] Handle schema evolution: new events get new tables/columns, changed event fields get new columns (never drop columns), removed events stop indexing forward
- [ ] Store upgrade history in `_ibis_upgrades` table: `(contract_address, block_number, old_class_hash, new_class_hash, timestamp)`
- [ ] API endpoint: `GET /v1/{contract}/upgrades` — list upgrade history for a contract
- [ ] Graceful degradation: if new ABI can't be fetched or parsed, log error and continue with previous ABI
- [ ] Events before and after upgrade are decoded with their respective ABIs (version-aware decoding)

**Implementation Notes**:
- The `Upgraded` event selector is `sn_keccak("Upgraded")` — ibis computes this at startup and adds it to the contract's subscription filter
- If using Cairo components with `#[flat]`, the `Upgraded` event may have a nested key structure — handle both flat and nested variants
- Schema migration on upgrade: use the existing `MigrateTable` method to add new columns for changed event structures
- Version-aware decoding: store `(class_hash, from_block, to_block)` ranges per contract; when decoding historical events, use the ABI that was active at that block number
- For factory children: if one child upgrades independently, only that child's decoding changes. If the factory pushes a batch upgrade, batch the ABI update across children
- This is distinct from proxy support (3.6): proxies resolve the implementation at startup, upgrade tracking handles runtime class hash changes on any contract

### 3.14 Monitoring and Observability

**Description**: Add structured logging, metrics, and health monitoring for production deployments.

**Requirements**:
- [ ] Structured logging with `slog` (Go stdlib)
- [ ] Log levels configurable via config and CLI flag
- [ ] Prometheus metrics endpoint (`/metrics`): blocks processed, events indexed, API latency, DB query duration, connection status
- [ ] Grafana dashboard template for common metrics
- [ ] Alerting rules template (block processing stalled, connection lost, high error rate)

**Implementation Notes**:
- Use Go's `log/slog` with JSON handler for production, text handler for development
- Prometheus client: `github.com/prometheus/client_golang`
- Key metrics: `ibis_blocks_processed_total`, `ibis_events_indexed_total`, `ibis_sync_lag_blocks`, `ibis_api_requests_total`, `ibis_ws_reconnections_total`

### 3.15 Kubernetes and Helm Chart

**Description**: Production-grade Kubernetes deployment configuration with Helm charts.

**Requirements**:
- [ ] Helm chart in `deploy/ibis-chart/` with configurable values
- [ ] Deployment with resource limits, health probes, and graceful shutdown
- [ ] PostgreSQL dependency (can use external or bundled)
- [ ] ConfigMap for `ibis.config.yaml`
- [ ] Secret for database credentials and RPC URLs
- [ ] Horizontal pod autoscaling for API server (separate from indexer)
- [ ] Ingress configuration
- [ ] `make helm-install`, `make helm-upgrade`, `make helm-uninstall` targets

**Implementation Notes**:
- Indexer should run as a single replica (leader election for HA is a future concern)
- API server can scale horizontally since it's read-only
- Consider splitting indexer and API into separate deployments sharing the same database
- Reference zindex's `deploy/` and Helm patterns

### 3.16 Forward Table Type

**Description**: A new `validTableType` called `forward` that forwards decoded event data to a specified URL via HTTP/HTTPS POST. Unlike `log`, `unique`, and `aggregation`, the `forward` type does not store events locally -- ibis acts as a pure event relay, parsing Starknet events via ABI and POSTing structured JSON payloads to an external endpoint. This enables users to build custom backends (their own database, message queue, analytics pipeline, etc.) and use ibis purely as an event decoder and forwarder.

**Requirements**:
- [ ] Add `"forward"` to `validTableTypes` in `internal/config/validate.go`
- [ ] Add `TableTypeForward` variant to the `TableType` enum in `internal/types/types.go`
- [ ] Add `ForwardConfig` fields to `TableConfig`: `URL` (required, string), `Headers` (optional, `map[string]string`), `Timeout` (optional, duration, default 10s), `MaxRetries` (optional, int, default 5)
- [ ] Config validation: `forward` tables REQUIRE a `url` field; reject if missing. Validate URL scheme is `http` or `https`. Apply `expandEnvVars` to URL and header values (e.g., `${WEBHOOK_TOKEN}`)
- [ ] Create `internal/forward/forwarder.go` with a `Forwarder` struct that manages HTTP delivery: singleton `http.Client` with custom `Transport` (connection pooling, 10s timeouts), bounded delivery channel, and a worker goroutine
- [ ] JSON payload format for each forwarded event:
  ```json
  {
    "event_id": "123456:0",
    "contract": "MyContract",
    "event": "Transfer",
    "block_number": 123456,
    "block_hash": "0xabc...",
    "transaction_hash": "0xdef...",
    "log_index": 0,
    "timestamp": 1710072000,
    "status": "ACCEPTED_L2",
    "data": { "from": "0x123...", "to": "0x456...", "amount": "1000" }
  }
  ```
- [ ] Include configurable HTTP headers on every request (supports `Authorization`, API keys, etc.). Always set `Content-Type: application/json`, `User-Agent: ibis-indexer/1.0`, and `X-Ibis-Event-Id: {event_id}` for idempotency
- [ ] Bounded retry with exponential backoff on failure: 5 attempts with delays of 1s, 2s, 4s, 8s, 16s (with jitter). Retry on 429, 5xx, connection errors. Do NOT retry on 4xx (except 429). Respect `Retry-After` header when present
- [ ] Non-blocking delivery: engine sends events to the forwarder via a buffered channel (capacity 10,000). If the channel is full, log a warning and drop the event. Never block the indexing pipeline
- [ ] Engine processor (`internal/engine/processor.go`): when a table's type is `forward`, skip store operations and instead route the decoded event to the `Forwarder`
- [ ] Graceful shutdown: drain the delivery channel and wait for in-flight requests (with a 30s deadline) before exiting
- [ ] Unit tests for the forwarder (delivery, retry, backoff, channel overflow) using `httptest.Server`

**Implementation Notes**:
- The forwarder hooks into the same event processing pipeline as the store -- in `processor.go`, check `schema.TableType == TableTypeForward` before calling `store.ApplyOperations`. Forward events bypass the store entirely.
- Use a single `http.Client` per `Forwarder` instance with `MaxIdleConnsPerHost: 10` and `IdleConnTimeout: 90s`. For forward tables pointing to different URLs, each gets its own `Forwarder` with its own client.
- The config shape nests under the existing `table` block:
  ```yaml
  events:
    - name: Transfer
      table:
        type: forward
        url: "https://my-api.com/events"
        timeout: 10s
        max_retries: 5
        headers:
          Authorization: "Bearer ${WEBHOOK_TOKEN}"
  ```
- Wildcard `"*"` with `type: forward` should work: all events forwarded to the same URL. Specific event entries can override with a different URL or table type.
- The existing `EventBus` (used for SSE) is a separate concern -- forward tables use their own delivery path. An event can be both forwarded (via a `forward` table entry) and stored (via a separate `log`/`unique`/`aggregation` entry for the same event) if the user configures both.
- Jitter implementation: `delay = baseDelay * 2^attempt * (0.5 + rand.Float64()*0.5)` (equal jitter).
- Consider adding a `/v1/status` field showing forward table health (events forwarded, failed, queued) for observability.

### 3.17 Ibis Query CLI --watch Mode (SSE Streaming)

**Description**: Add a `--watch` flag to `ibis query` that connects to the running ibis API server's SSE endpoint and streams new events to the terminal in real-time. This turns `ibis query` into a live tail for indexed events -- like `tail -f` for on-chain data. Instead of querying the database directly, `--watch` acts as an SSE client connecting to the `/v1/{contract}/{event}/stream` endpoint served by `ibis run`.

**Requirements**:
- [ ] Add `--watch` / `-w` boolean flag to the `queryCmd` in `internal/cli/query.go`
- [ ] Add `--api-url` string flag (default: derived from config's `api.host` and `api.port`, e.g., `http://localhost:8080`) to specify the ibis API server URL
- [ ] When `--watch` is set, construct the SSE URL: `{api-url}/v1/{contract}/{event}/stream` with query params from `--filter` and `--contract-address` flags translated to Supabase-style query params (e.g., `?block_number=gte.100&contract_address=eq.0x123`)
- [ ] Implement SSE client using Go stdlib (`net/http` + `bufio.Scanner`): parse `id:` and `data:` lines from the `text/event-stream` response, unmarshal JSON data into event objects
- [ ] Output each received event using the existing `--format` flag: `json` (one JSON object per line, newline-delimited), `table` (print header once, then one row per event), `csv` (print header once, then one row per event)
- [ ] Auto-reconnect on connection loss with exponential backoff (1s, 2s, 4s, 8s, max 30s). Send `Last-Event-ID` header on reconnect to resume from the last received event. Print a warning line (to stderr) on disconnect and reconnect
- [ ] Clean shutdown on SIGINT/SIGTERM: close the HTTP response body gracefully, print a summary line (events received count) to stderr, and exit 0
- [ ] Mutual exclusivity: `--watch` is incompatible with `--latest`, `--count`, `--aggregate`, `--unique`, `--children`, `--children-count`, and `--list`. Return a clear error if combined
- [ ] Unit tests: SSE line parser, URL construction from flags, mutual exclusivity validation. Integration test using `httptest.Server` that serves SSE events and verifies the CLI receives and formats them correctly

**Implementation Notes**:
- The SSE client is intentionally stdlib-only (no `r3labs/sse` or similar) -- the protocol is simple enough (`id: ...\ndata: ...\n\n`) and this avoids adding a dependency. Use `bufio.NewScanner` on the response body with a custom split function or line-by-line reading.
- API URL derivation: read `cfg.API.Host` and `cfg.API.Port` from the loaded config, construct `http://{host}:{port}`. The `--api-url` flag overrides this entirely. If host is `0.0.0.0`, default to `localhost` for the client URL.
- For `--format table` and `--format csv` in watch mode, print the header row on first event, then append data rows as events arrive. This differs from one-shot mode where all events are collected before rendering.
- The reconnect loop should track the last received SSE event ID and send it as `Last-Event-ID` on reconnect. The server's `replayEvents` in `sse.go` already handles this for gap-free delivery.
- Filter flags map to query params: `--filter "block_number=gte.100"` becomes `?block_number=gte.100`, and `--contract-address 0x123` becomes `?contract_address=eq.0x123`. This matches the Supabase-style filtering already used by the REST endpoints (see `parseFiltersFromURL` in `api/query.go`).
- Signal handling: use `signal.NotifyContext` with `os.Interrupt` and `syscall.SIGTERM` to get a cancellable context, then pass it to the HTTP request.

---

## Phase 4: Future

### 4.1 WebSocket Subscriptions for Clients

**Description**: Real-time event streaming via WebSocket connections for client applications, complementing the existing SSE support with bidirectional communication.

**Features**:
- WebSocket endpoint for real-time event subscriptions
- Client-side filtering and subscription management
- Multiple subscription channels per connection
- Heartbeat and automatic reconnection support
- Binary protocol option for high-throughput scenarios

**Rationale**: While SSE covers most real-time use cases, WebSocket subscriptions enable bidirectional communication, client-managed filters, and more efficient multiplexing of multiple event streams over a single connection. Essential for interactive applications like trading platforms.

### 4.2 MCP Server Integration

**Description**: Expose indexed Starknet data as MCP (Model Context Protocol) tools, enabling AI agents to query blockchain state as part of their workflows.

**Features**:
- MCP server that wraps ibis REST API as tools
- Schema-aware tool descriptions generated from ABI
- Natural language-friendly tool interfaces
- Support for complex multi-step queries
- Integration with Claude Desktop and other MCP clients

**Rationale**: As AI agents become primary consumers of structured data, exposing indexed blockchain data via MCP enables a new class of AI-powered applications that can reason about on-chain state without custom integration work.

### 4.3 State Reconstruction Tables

**Description**: Tables that reconstruct current contract state from event history, functioning as materialized views that always reflect the latest on-chain state.

**Features**:
- Define state tables that derive current values from event streams
- Automatic state computation on each relevant event
- Support for complex state transitions (not just last-write-wins)
- Snapshot and restore for fast state recovery
- Custom state reducers defined in config

**Rationale**: Many applications need current state (token balances, game positions, order books) rather than event history. Auto-derived state tables eliminate the need for custom state management code in the application layer.

### 4.4 Multi-Chain Support

**Description**: Extend Ibis beyond Starknet to support other chains that use similar event/log patterns, starting with chains that have Cairo-based execution.

**Features**:
- Chain-agnostic core with chain-specific providers
- Starknet appchain support (Madara, Dojo Katana)
- Potential EVM support for cross-chain indexing
- Unified query API across chains
- Cross-chain event correlation

**Rationale**: The ABI-driven, config-based indexer pattern is not Starknet-specific. Supporting multiple chains (especially Starknet L3s and appchains) multiplies the tool's utility with minimal architectural changes.

### 4.5 Block Processors

**Description**: Optional block-level processing pipeline that subscribes to `newHeads` and processes full blocks, enabling use cases beyond event indexing.

**Features**:
- Subscribe to `starknet_subscribeNewHeads` for block-level data
- Custom block processors that receive full block data (headers, transactions, receipts)
- Built-in processors: block metadata indexing, transaction tracking, gas analytics
- Configurable in `ibis.config.yaml` alongside event subscriptions
- Block processor hooks for custom logic (e.g., track all transactions from a specific sender)

**Rationale**: While event-driven indexing covers most use cases, some applications need block-level data (transaction counts, gas usage trends, block timing). Block processors provide this capability without changing the default event-subscription architecture.

### 4.6 Plugin System

**Description**: An extensible plugin architecture that allows users to add custom event processing, transformations, and integrations without modifying the core indexer.

**Features**:
- Go plugin interface for custom event processors
- WASM plugin support for language-agnostic extensions
- Pre-built plugins: webhook notifications, Slack alerts, custom aggregations
- Plugin marketplace or registry
- Hot-reload plugin updates without indexer restart

**Rationale**: Every indexing use case has unique requirements beyond what a config file can express. A plugin system lets power users extend Ibis without forking, while keeping the core simple for the majority of users.
