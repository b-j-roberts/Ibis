# Getting Started with Ibis

This guide walks you through installing Ibis, indexing your first Starknet contract, and querying the data. By the end, you'll have a running indexer with real-time event data accessible via CLI and REST API.

---

## Prerequisites

- **Go 1.25+** (only needed if building from source)
- A terminal with `curl` (for API examples)
- **Docker** (optional, only needed for PostgreSQL)

No Starknet node required -- Ibis connects to public RPC endpoints by default.

---

## Installation

### Option 1: asdf (recommended)

[asdf](https://asdf-vm.com/) manages tool versions across projects:

```bash
asdf plugin add ibis https://github.com/b-j-roberts/asdf-ibis.git
asdf install ibis latest
asdf set -u ibis latest
```

### Option 2: Binary release

Download a prebuilt binary from [GitHub Releases](https://github.com/b-j-roberts/ibis/releases):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/b-j-roberts/ibis/releases/latest/download/ibis_darwin_arm64.tar.gz | tar xz
sudo mv ibis /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/b-j-roberts/ibis/releases/latest/download/ibis_linux_amd64.tar.gz | tar xz
sudo mv ibis /usr/local/bin/
```

### Option 3: Build from source

Requires Go 1.25+:

```bash
git clone https://github.com/b-j-roberts/ibis.git
cd ibis
make build
# Binary at ./bin/ibis
```

Add it to your PATH or move it:

```bash
sudo mv ./bin/ibis /usr/local/bin/
```

### Verify the installation

```bash
ibis --help
```

You should see:

```
Ibis indexes events from Starknet smart contracts using only an RPC
connection, generates typed database tables and REST APIs from contract
ABIs, and launches with a single command from a YAML config file.

Usage:
  ibis [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  init        Scaffold an ibis.config.yaml from contract inspection
  query       Query indexed data from the terminal
  run         Start the indexer with the given config

Flags:
      --config string   path to ibis config file (default "./ibis.config.yaml")
  -h, --help            help for ibis
  -v, --version         version for ibis

Use "ibis [command] --help" for more information about a command.
```

---

## Your First Indexer

We'll index the **STRK token** on Starknet mainnet. It's an ERC-20 token with frequent Transfer events, so you'll see data immediately.

### Step 1: Generate a config

```bash
ibis init \
  --contract 0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d \
  --network mainnet \
  --database memory
```

What each flag does:
- `--contract` -- the Starknet contract address to index (this is the STRK token)
- `--network mainnet` -- connect to Starknet mainnet (sets a default public RPC)
- `--database memory` -- store data in memory (no setup required, perfect for trying things out)

> **Note**: If your terminal doesn't support interactive prompts (CI pipelines, Docker containers, scripts, or AI coding assistants), skip to the [non-interactive command](#non-interactive) below.

Ibis enters interactive mode. It will:

1. **Prompt for an RPC endpoint** -- press Enter to accept the default (`https://starknet-rpc.publicnode.com`)
2. **Ask you to name the contract** -- type `STRK` and press Enter
3. **Fetch the ABI** from the chain and list the available events (e.g., `Transfer`, `Approval`)
4. **Ask which events to index** -- select "Yes" to index all events with the wildcard (`*`)

```
Ibis Config Generator
========================================

(default: https://starknet-rpc.publicnode.com)
RPC endpoint URL: <press Enter>

Name for contract 0x04718f...938d: STRK
Fetching ABI for 0x04718f...938d from chain...
  Found 13 events:
    - Transfer (data: from, to, value)
    - Approval (data: owner, spender, value)
    - ImplementationAdded (data: implementation_data)
    ... and 10 more

Index all events with wildcard (*)? [Y/n]: Y
Customize table type for specific events? [y/N]: N

Config written to ./ibis.config.yaml
Run `ibis run --config ./ibis.config.yaml` to start indexing.
```

> **Note**: The `keys`/`data` split shown for each event depends on the contract's ABI. Cairo 1 contracts typically place all fields in `data`, while older Cairo 0 contracts may use `keys` for indexed fields.

> **Tip: Agent skills** -- If you use an AI coding assistant that supports agent skills (e.g. [Claude Code](https://claude.com/claude-code)), the `ibis-config` skill can generate configs from natural language. For example: *"index all Transfer events from the STRK token"*. Install with `npx skills add b-j-roberts/ibis --skill ibis-config`. See the [Agent Skills Guide](AGENT-SKILLS.md) for details. This is entirely optional — skipping it has no effect on the rest of the guide.

<a id="non-interactive"></a>

For scripting or CI, add `--non-interactive` to skip all prompts. Use `--name` to set the contract's query identifier (used in `ibis query <name> <event>` and REST API paths like `/v1/<name>/<event>`):

```bash
ibis init \
  --contract 0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d \
  --name STRK \
  --network mainnet \
  --database memory \
  --non-interactive
```

> **Note**: Without `--name`, non-interactive mode auto-generates a name from the contract address (e.g., `Contract_04718f`). Always pass `--name` if you plan to query by a human-friendly name.

### Step 2: Understand the config

Open `ibis.config.yaml`. Here's what was generated:

```yaml
# ibis.config.yaml - Generated by `ibis init`
#
# Environment variables can be referenced with ${VAR_NAME} syntax.
# Run `ibis run` to start indexing with this config.

network: mainnet
rpc: https://starknet-rpc.publicnode.com
database:
  backend: memory
api:
  host: 0.0.0.0
  port: 8080
indexer:
  start_block: 0
  pending_blocks: true
  batch_size: 10
contracts:
  - name: STRK
    address: "0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d"
    abi: fetch
    events:
      - name: "*"
        table:
          type: log
```

| Section | What it controls |
|---------|-----------------|
| `network` | Which Starknet network to connect to (`mainnet`, `sepolia`, or `custom`) |
| `rpc` | The RPC endpoint URL. WSS endpoints enable real-time subscriptions; HTTP falls back to polling |
| `database.backend` | Where to store indexed data. `memory` is ephemeral (lost on restart), `badger` persists to disk, `postgres` is production-grade |
| `api` | The REST API server address. Default: `0.0.0.0:8080` |
| `indexer.start_block` | Where to start indexing. `0` means the latest block (you'll only see new events). Set a specific block number to backfill history |
| `indexer.pending_blocks` | Whether to index pending (unconfirmed) blocks for lower latency |
| `indexer.batch_size` | How many blocks to fetch per batch during backfill |
| `contracts` | The contracts to index -- name, address, ABI source, and event configuration |
| `events[].name` | `"*"` is a wildcard that matches all events in the contract ABI |
| `events[].table.type` | `log` creates append-only tables. Other options: `unique` (last-write-wins by key) and `aggregation` (auto-computed sums/counts) |

### Step 3: Run the indexer

```bash
ibis run
```

You'll see startup output like this:

```
Loaded config from ./ibis.config.yaml
  Network:  mainnet
  RPC:      https://starknet-rpc.publicnode.com
  Backend:  memory
  API:      0.0.0.0:8080
  Contracts: 1
    - STRK (0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d): 1 events
time=... level=INFO msg="created table" component=engine name=strk_transfer type=log columns=10
time=... level=INFO msg="created table" component=engine name=strk_approval type=log columns=10
... (one line per event table)

API server listening on 0.0.0.0:8080
Starting indexer...
time=... level=INFO msg="contract start block" component=engine contract=STRK start_block=0
time=... level=INFO msg="WSS subscription active" component=subscriber contract=0x04718f...938d from_block=0
```

> **Note**: You'll see structured log lines for each table created and the WebSocket subscription being established — this is normal. You may also see a `level=WARN msg="RPC spec version mismatch"` warning — this is safe to ignore. If your RPC endpoint doesn't support WebSocket, you'll see `"WSS subscription failed, falling back to polling"` followed by `"starting polling fallback"` — polling works the same way.

Here's what Ibis is doing:

1. **Loading config** -- reads `ibis.config.yaml` and validates all fields
2. **Connecting to RPC** -- establishes a connection to the Starknet node
3. **Fetching ABIs** -- downloads the contract ABI from the chain (because `abi: fetch`)
4. **Building schemas** -- maps ABI event definitions to database table columns
5. **Creating tables** -- sets up the in-memory tables for each event
6. **Subscribing** -- if the RPC supports WebSocket, subscribes to real-time events; otherwise falls back to HTTP polling
7. **Serving the API** -- starts the REST server on port 8080

Leave this running in your terminal. Open a new terminal for the next steps.

---

## Querying Data

Once the indexer is running, data flows in continuously. You can query it via the REST API, SSE streaming, or the CLI.

### REST API queries

The running API server provides query capabilities over HTTP. Endpoints are auto-generated from your contract and event names:

```
GET /v1/{contract}/{event}
```

**Fetch recent Transfers with pagination:**

```bash
curl "http://localhost:8080/v1/STRK/Transfer?limit=5&order=block_number.desc"
```

```json
{
  "data": [
    {
      "block_number": 950124,
      "log_index": 2,
      "transaction_hash": "0x0ab3f...",
      "timestamp": 1713100000,
      "contract_address": "0x04718f...",
      "contract_name": "STRK",
      "event_name": "Transfer",
      "status": "ACCEPTED_ON_L2",
      "from": "0x0123...",
      "to": "0x0456...",
      "value": "750000000000000000"
    },
    ...
  ],
  "count": 5,
  "limit": 5,
  "offset": 0
}
```

> **Note**: Every event includes metadata fields (`block_number`, `log_index`, `transaction_hash`, `timestamp`, `contract_address`, `contract_name`, `event_name`, `status`) alongside the event-specific fields defined in the contract ABI.

**Filter by block range:**

```bash
curl "http://localhost:8080/v1/STRK/Transfer?block_number=gte.950000&limit=10"
```

**Get the latest event:**

```bash
curl "http://localhost:8080/v1/STRK/Transfer/latest"
```

```json
{
  "data": {
    "block_number": 950124,
    "log_index": 2,
    "transaction_hash": "0x0ab3f...",
    "timestamp": 1713100000,
    "contract_address": "0x04718f...",
    "contract_name": "STRK",
    "event_name": "Transfer",
    "status": "ACCEPTED_ON_L2",
    "from": "0x0123...",
    "to": "0x0456...",
    "value": "750000000000000000"
  }
}
```

**Count events:**

```bash
curl "http://localhost:8080/v1/STRK/Transfer/count"
```

```json
{
  "count": 47
}
```

**Check indexer status:**

```bash
curl "http://localhost:8080/v1/status"
```

```json
{
  "current_block": 950124,
  "contracts": [
    {
      "name": "STRK",
      "address": "0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d",
      "events": 1,
      "current_block": 950124
    }
  ]
}
```

This returns the current block height and the contracts being indexed (with each contract's name, address, event count, and per-contract block cursor).

The REST API supports the same filter operators as the CLI (see table below). Pass filters as query parameters: `?field=operator.value`.

### SSE streaming (real-time)

Ibis supports Server-Sent Events for real-time streaming. Open a connection and new events are pushed as they arrive on-chain:

```bash
curl -N "http://localhost:8080/v1/STRK/Transfer/stream"
```

```
id: 950124:2
data: {"block_number":950124,"log_index":2,"transaction_hash":"0x0ab3...","timestamp":1713100000,"contract_address":"0x04718f...","contract_name":"STRK","event_name":"Transfer","status":"ACCEPTED_ON_L2","from":"0x0123...","to":"0x0456...","value":"750000000000000000"}

id: 950125:0
data: {"block_number":950125,"log_index":0,"transaction_hash":"0x0cd4...","timestamp":1713100060,"contract_address":"0x04718f...","contract_name":"STRK","event_name":"Transfer","status":"ACCEPTED_ON_L2","from":"0x0789...","to":"0x0abc...","value":"100000000000000000"}
```

Press `Ctrl+C` to stop streaming. The `-N` flag disables curl's output buffering so events appear immediately.

> **Note**: The `id` field (`block_number:log_index`) is a standard SSE event ID. To resume a stream after disconnection, pass it via the `Last-Event-ID` header — Ibis will replay any events you missed: `curl -N -H "Last-Event-ID: 950124:2" "http://localhost:8080/v1/STRK/Transfer/stream"`

### CLI queries

> **Important**: The `ibis query` command reads directly from the database by opening its own connection. With the `memory` backend, this creates a **separate, empty** in-memory store — it cannot see data from the running `ibis run` process. CLI queries only work with persistent backends (`badger` or `postgres`). If you're using `memory` (as in this guide), use the REST API or SSE streaming above to query data.

The `ibis query` command is useful when you have a persistent backend configured. Basic syntax: `ibis query <contract> <event>`.

**List available tables:**

```bash
ibis query --list
```

```
Available tables:

  STRK (0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d)
    * (all ABI events)  type=log
```

**Fetch recent Transfer events as a table:**

```bash
ibis query STRK Transfer --format table
```

```
block_number  log_index  transaction_hash                                                     from                                                                 to                                                                   value
------------  ---------  ----------------                                                     ----                                                                 --                                                                   -----
950123        3          0x07a8f...                                                            0x0123...                                                            0x0456...                                                            1000000000000000000
950123        1          0x03bc1...                                                            0x0789...                                                            0x0abc...                                                            500000000000000000
950122        5          0x01ef2...                                                            0x0def...                                                            0x0fed...                                                            2500000000000000000
...
```

**Get the most recent Transfer:**

```bash
ibis query STRK Transfer --latest
```

```json
[
  {
    "block_number": 950124,
    "log_index": 2,
    "transaction_hash": "0x0ab3f...",
    "timestamp": 1713100000,
    "contract_address": "0x04718f...",
    "contract_name": "STRK",
    "event_name": "Transfer",
    "status": "ACCEPTED_ON_L2",
    "from": "0x0123...",
    "to": "0x0456...",
    "value": "750000000000000000"
  }
]
```

> **Note**: The CLI `--latest` returns a JSON array, while the REST `/latest` endpoint wraps the event in `{"data": {...}}`.

**Count total Transfer events:**

```bash
ibis query STRK Transfer --count
```

```json
{
  "count": 47
}
```

> **Tip**: Use `--format table` for human-friendly output: `Count: 47`.

**Filter by sender address:**

```bash
ibis query STRK Transfer --filter "from=eq.0x0123..." --format table
```

The `--filter` flag uses the syntax `field=operator.value`. Available operators:

| Operator | Meaning |
|----------|---------|
| `eq` | Equal to |
| `neq` | Not equal to |
| `gt` | Greater than |
| `gte` | Greater than or equal |
| `lt` | Less than |
| `lte` | Less than or equal |

You can combine multiple filters:

```bash
ibis query STRK Transfer \
  --filter "block_number=gte.950000" \
  --filter "from=eq.0x0123..." \
  --limit 10 \
  --order block_number.asc \
  --format table
```

---

## Next Steps

Now that you have a running indexer, explore these topics:

- **[Configuration Reference](CONFIGURATION.md)** -- every field in `ibis.config.yaml` explained, with defaults, types, and examples
- **[Table Types Guide](TABLE-TYPES.md)** -- when to use `log`, `unique`, and `aggregation` tables, with real-world examples
- **[Advanced Features](ADVANCED-FEATURES.md)** -- index factory-deployed contracts with automatic child discovery and shared tables
- **[Agent Skills Guide](AGENT-SKILLS.md)** -- use `ibis-config` and `ibis-query` skills to generate configs and query data using natural language

### Going to production

When you're ready to move beyond the in-memory database:

1. **Switch to PostgreSQL** -- change `database.backend` to `postgres` and add connection details (or use `make docker-compose-up` for a quick setup)
2. **Set a start block** -- change `indexer.start_block` from `0` to a specific block number to backfill historical data
3. **Use WSS for real-time** -- replace the HTTP RPC URL with a WebSocket endpoint for instant event delivery via `starknet_subscribeEvents`

---

## Troubleshooting

### RPC connection failures

```
Error: creating provider: dial tcp: lookup free-rpc.nethermind.io: no such host
```

**Cause**: The RPC endpoint is unreachable. The default public endpoint may be down or rate-limited.

**Fix**: Try a different RPC provider. Update the `rpc` field in `ibis.config.yaml` or pass `--rpc` to `ibis init`. Free alternatives:

- `https://starknet.drpc.org` (dRPC)
- `https://rpc.starknet.lava.build` (Lava)

See [Starknet RPC providers](https://www.starknet.io/fullnodes-rpc-services/) for more options.

### ABI fetch errors

```
Error: engine setup: fetching ABI for STRK: ...
```

**Cause**: Ibis couldn't fetch the contract's ABI from the chain. This can happen if the contract is not verified, the RPC is unresponsive, or the address is invalid.

**Fix**:
- Verify the contract address is correct
- Check that the RPC endpoint is accessible: `curl https://starknet-rpc.publicnode.com -X POST -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","method":"starknet_chainId","id":1}'`
- If the contract ABI isn't available on-chain, provide a local ABI file: `abi: ./path/to/abi.json`

### Port conflicts

```
Error: API server error: listen tcp 0.0.0.0:8080: bind: address already in use
```

**Cause**: Another process is using port 8080.

**Fix**: Change the port in `ibis.config.yaml`:

```yaml
api:
  port: 9090
```

Or stop the process using port 8080:

```bash
lsof -i :8080
```

### No data appearing

**Cause**: If `start_block` is `0`, Ibis begins from the latest block and only indexes new events going forward. If the contract hasn't emitted events since you started, the tables will be empty.

**Fix**: Set `indexer.start_block` to a recent block number to backfill historical events:

```yaml
indexer:
  start_block: 900000
```

Restart the indexer and Ibis will fetch events from that block forward.

### Memory database and restarts

The `memory` backend does not persist data. When you stop the indexer (`Ctrl+C`), all indexed data is lost. This is by design for quick experimentation. For persistence, switch to `badger` (embedded, writes to disk) or `postgres`.

### CLI queries return no results

If `ibis query` returns "No results found" while the REST API shows data, you're likely using the `memory` backend. The CLI opens its own database connection, which creates a separate empty in-memory store. Use the REST API (`curl http://localhost:8080/v1/...`) to query data with the memory backend, or switch to `badger` or `postgres` for CLI query support.
