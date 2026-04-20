# Ibis

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![CI](https://github.com/b-j-roberts/ibis/actions/workflows/ci.yml/badge.svg)](https://github.com/b-j-roberts/ibis/actions/workflows/ci.yml)

A fast, easy-to-use Starknet event indexer. One config file, one command, fully typed database tables and REST APIs -- all generated from contract ABIs.

## Features

- **ABI-driven** -- contract ABIs drive table creation, column types, REST endpoints, and query execution
- **One config, one command** -- define contracts in `ibis.config.yaml`, run `ibis run`
- **Real-time streaming** -- SSE with reconnection replay via `Last-Event-ID`
- **Three database backends** -- PostgreSQL (production), BadgerDB (embedded), in-memory (dev/test)
- **Auto-generated REST API** -- Supabase-style filtering, pagination, and ordering
- **Reorg-safe** -- every write produces an `(add, revert)` operation pair for pending block support
- **Backfill** -- automatic historical event catchup on startup via `starknet_getEvents`
- **Wildcard events** -- index all contract events with `name: "*"`, override specific ones as needed
- **Three table types** -- `log` (append-only), `unique` (last-write-wins by key), `aggregation` (auto-computed sums/counts)
- **Factory contracts** -- automatic child contract discovery with shared tables
- **Class-hash discovery** -- watch the UDC for new deployments matching a class hash
- **View function polling** -- periodically call contract view functions and store results
- **Dynamic management** -- register, deregister, and update contracts at runtime via admin API
- **CLI queries** -- query indexed data from the terminal in JSON, table, or CSV format

## Installation

### asdf

```bash
asdf plugin add ibis https://github.com/b-j-roberts/asdf-ibis.git
asdf install ibis latest
asdf set -u ibis latest
```

### Binary release

Download from [GitHub Releases](https://github.com/b-j-roberts/ibis/releases):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/b-j-roberts/ibis/releases/latest/download/ibis_darwin_arm64.tar.gz | tar xz
sudo mv ibis /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/b-j-roberts/ibis/releases/latest/download/ibis_linux_amd64.tar.gz | tar xz
sudo mv ibis /usr/local/bin/
```

### Build from source

```bash
git clone https://github.com/b-j-roberts/ibis.git && cd ibis
make build
# Binary at ./bin/ibis
```

## Quick Start

```bash
# 1. Generate a config from a contract address
ibis init --contract 0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d

# 2. Start indexing
ibis run

# 3. Query data
ibis query STRK Transfer --limit 5 --format table
```

The `init` command fetches the contract ABI, lists available events, and generates an `ibis.config.yaml`. The `run` command connects to Starknet, subscribes to events, decodes them, writes to the database, and serves a REST API.

For a full walkthrough, see the [Getting Started Guide](docs/GETTING-STARTED.md).

## Configuration

```yaml
network: mainnet
rpc: wss://starknet-mainnet.example.com

database:
  backend: postgres
  postgres:
    host: localhost
    port: 5432
    user: ibis
    password: ${IBIS_DB_PASSWORD}
    name: ibis

api:
  host: 0.0.0.0
  port: 8080
  cors_origins: ["*"]
  admin_key: ${IBIS_ADMIN_KEY}

indexer:
  start_block: 0
  pending_blocks: true
  batch_size: 10

contracts:
  - name: MyToken
    address: "0x04718f5a0fc..."
    abi: fetch
    events:
      - name: "*"
        table:
          type: log
      - name: Transfer
        table:
          type: unique
          unique_key: sender

  - name: MyFactory
    address: "0x..."
    abi: fetch
    factory:
      event: PairCreated
      child_address_field: pair
      child_abi: fetch
      shared_tables: true
      child_events:
        - name: Swap
          table:
            type: log
```

Environment variables are expanded with `${VAR_NAME}` syntax. See the [Configuration Reference](docs/CONFIGURATION.md) for all options, or [`configs/ibis.config.yaml`](configs/ibis.config.yaml) for a fully annotated example.

## CLI

```
ibis init     Scaffold a config by inspecting contracts on-chain
ibis run      Start the indexer
ibis query    Query indexed data from the terminal
```

**`ibis init`** -- generates `ibis.config.yaml` from contract addresses:

```bash
ibis init --contract 0x... --network mainnet --database postgres
```

**`ibis query`** -- query data from the terminal:

```bash
ibis query MyToken Transfer --limit 10 --format table
ibis query MyToken Transfer --filter "sender=eq.0x123" --order block_number.desc
ibis query --list                          # List all available tables
```

See the [CLI Reference](docs/CLI-REFERENCE.md) for all flags and options.

## REST API

Ibis auto-generates REST endpoints from your contract ABI:

```
GET  /v1/{contract}/{event}              List events (paginated)
GET  /v1/{contract}/{event}/latest       Latest event
GET  /v1/{contract}/{event}/count        Event count
GET  /v1/{contract}/{event}/unique       Unique table entries
GET  /v1/{contract}/{event}/aggregate    Aggregated values
GET  /v1/{contract}/{event}/stream       SSE real-time stream
```

**Query parameters** follow Supabase conventions:

```bash
# Pagination and ordering
curl "localhost:8080/v1/MyToken/Transfer?limit=10&offset=0&order=block_number.desc"

# Filtering (eq, neq, gt, gte, lt, lte)
curl "localhost:8080/v1/MyToken/Transfer?sender=eq.0x123&block_number=gte.100000"

# Real-time streaming
curl "localhost:8080/v1/MyToken/Transfer/stream"
```

**Factory endpoints:**

```
GET  /v1/{factory}/children              List child contracts
GET  /v1/{factory}/children/count        Count child contracts
```

**Admin endpoints** (protected by `admin_key` when configured):

```
POST   /v1/admin/contracts               Register a contract
GET    /v1/admin/contracts               List all contracts
PUT    /v1/admin/contracts/{name}        Update a contract
DELETE /v1/admin/contracts/{name}        Deregister a contract
```

**System endpoints:**

```
GET  /v1/health                          Health check
GET  /v1/status                          Indexer status
```

See the [API Reference](docs/API-REFERENCE.md) for full documentation.

## Architecture

```
                    +---------------------------+
                    |   Starknet RPC (WSS)      |
                    +-------------+-------------+
                                  |
                       +----------v------------+
                       |  Event Subscriber     |
                       |  - Per-contract subs  |
                       |  - Reconnection       |
                       |  - Polling fallback   |
                       +----------+------------+
                                  |
                       +----------v------------+
                       |  Event Processor      |
                       |  - ABI decoding       |
                       |  - Selector matching  |
                       |  - Pending tracking   |
                       |  - Factory detection  |
                       +----------+------------+
                                  |
                  +---------------+---------------+
                  |               |               |
           +------v------+  +-----v------+ +------v-----+
           |  BadgerDB   |  | PostgreSQL | |  In-Memory |
           |  (embedded) |  | (external) | | (dev/test) |
           +-------------+  +------------+ +------------+
                  |               |               |
                  +---------------+---------------+
                                  |
                       +----------v------------+
                       |   API Server          |
                       |  - REST (generated)   |
                       |  - SSE (real-time)    |
                       |  - Admin (dynamic)    |
                       |  - Query CLI          |
                       +-----------------------+
```

For a deep dive into the architecture, data models, and design decisions, see the [Specification](docs/SPEC.md).

## Docker

```bash
# Build and run standalone
make docker-build
make docker-run

# Or with docker-compose (includes PostgreSQL)
cp .env.example .env   # Edit with your values
make docker-compose-up
```

See the [Deployment Guide](docs/DEPLOYMENT.md) for production deployment patterns.

## Agent Skills

Ibis ships with [Claude Code skills](docs/AGENT-SKILLS.md) for AI-powered config generation and natural language querying:

```bash
npx skills add b-j-roberts/ibis
```

## Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](docs/GETTING-STARTED.md) | Installation and first indexer walkthrough |
| [Configuration](docs/CONFIGURATION.md) | Full YAML config reference |
| [API Reference](docs/API-REFERENCE.md) | REST API endpoints and query parameters |
| [CLI Reference](docs/CLI-REFERENCE.md) | All CLI commands and flags |
| [Table Types](docs/TABLE-TYPES.md) | Guide to log, unique, and aggregation tables |
| [SSE Streaming](docs/SSE-STREAMING.md) | Real-time event streaming |
| [Advanced Features](docs/ADVANCED-FEATURES.md) | Factory contracts, discovery, view polling |
| [Deployment](docs/DEPLOYMENT.md) | Production deployment guide |
| [Specification](docs/SPEC.md) | Architecture and design decisions |
| [Roadmap](docs/ROADMAP.md) | Planned features and development phases |

## Contributing

Contributions are welcome. See the [Roadmap](docs/ROADMAP.md) for planned work and the [Specification](docs/SPEC.md) for architecture context.

```bash
make check   # Run fmt, vet, lint, and tests
```

## License

[MIT](LICENSE)
