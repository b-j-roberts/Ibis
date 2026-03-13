# Ibis Development Roadmap

---

## Phase 3: Nice to Have

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
