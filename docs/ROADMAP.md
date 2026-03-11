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
- [ ] `README.md` with project overview, problem statement, feature highlights, architecture diagram (from SPEC.md), installation instructions (placeholders), usage examples (`ibis init`, `ibis run`, `ibis query`), config file example, API example, contributing pointer, and license badge
- [ ] `CLAUDE.md` with build/test/lint commands (from Makefile targets), project structure overview, key architectural patterns (store interface, operation pairs, ABI-driven schemas), tech stack summary, and pointers to SPEC.md and ROADMAP.md for deeper context
- [ ] `LICENSE` file with full MIT License text, copyright `2025 Brandon Roberts`
- [ ] README should reference the MIT license with a badge at the top
- [ ] CLAUDE.md should be usable immediately once task 1.1 (Go module scaffolding) and 1.2 (Makefile) land — reference the planned commands even if not yet implemented

**Implementation Notes**:
- Pull architecture diagram and feature descriptions from `docs/SPEC.md` — keep README concise and link to SPEC for deep dives
- CLAUDE.md follows the pattern from Brandon's workspace root `CLAUDE.md` — table of projects, key commands, architecture notes
- For the README, use shields.io badges for license, Go version, and build status (GitHub Actions URL can be placeholder until 1.4 lands)
- Keep README under ~200 lines — enough to sell the project vision without overwhelming; link to `docs/` for details

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

**Description**: Enable users to query indexed data using natural language via a Claude Code skill. Users can ask questions like "who has the highest score?" or "show me all trades above 1000 STRK in the last hour" and get results.

**Requirements**:
- [ ] Claude Code skill that understands available tables, schemas, and column types
- [ ] Translates natural language to `ibis query` commands or direct API calls
- [ ] Returns formatted results with context
- [ ] Handles follow-up questions with conversation context
- [ ] Skill can inspect `ibis.config.yaml` to understand the indexed data model

**Implementation Notes**:
- Build as a Claude Code skill (not embedded in the ibis binary)
- Skill reads and/or queries the config and schema to understand what data is available
- Generates `ibis query` commands with appropriate filters
- Can also hit the REST API if the server is running

### 3.2 AI-Powered Config Generation

**Description**: A Claude Code skill that generates `ibis.config.yaml` from a contract address or natural language description. "Index all swap events from this AMM contract" -> full config file.

**Requirements**:
- [ ] Claude Code skill that accepts a contract address and network
- [ ] Fetches and analyzes the contract ABI
- [ ] Suggests which events to index based on event names and data structures
- [ ] Recommends table types (log vs unique vs aggregation) based on event semantics
- [ ] Generates complete `ibis.config.yaml`
- [ ] Supports iterative refinement ("also add the Transfer events", "make the price table unique by pair_id")

**Implementation Notes**:
- Build as a Claude Code skill that calls the Starknet RPC to fetch ABIs
- Use event naming conventions to infer table types (e.g., "Updated" -> unique, "Created" -> log)
- Skill can run `ibis init` under the hood or generate the YAML directly

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

### 3.4 Multiple Contract Support Enhancements

**Description**: Improve the multi-contract experience with contract grouping, cross-contract queries, and contract discovery.

**Requirements**:
- [ ] Contract groups in config: group related contracts under a namespace
- [ ] Cross-contract queries: query events across multiple contracts in a single request
- [ ] Contract discovery: watch for new contract deployments matching a class hash
- [ ] Proxy contract support: follow proxy patterns to resolve implementation ABIs
- [ ] Dynamic contract registration: add new contracts without restarting the indexer

**Implementation Notes**:
- Contract groups map to API URL prefixes (`/v1/{group}/{contract}/{event}`)
- Cross-contract queries need a unified table view or UNION query support
- Proxy detection: check for `__implementation` storage slot or known proxy patterns

### 3.5 Monitoring and Observability

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

### 3.6 Kubernetes and Helm Chart

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
