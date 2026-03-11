# Ibis

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![CI](https://github.com/b-j-roberts/ibis/actions/workflows/ci.yml/badge.svg)](https://github.com/b-j-roberts/ibis/actions/workflows/ci.yml)

A fast, easy-to-use Starknet event indexer written in Go. One config file, one command, fully typed APIs.

Ibis is a general-purpose Starknet indexer that generates typed database tables and REST APIs directly from contract ABIs, and launches with a single command from a YAML config file.

## Features

- **ABI-driven schemas** -- contract ABIs drive table creation, REST endpoints, and type safety
- **One config, one command** -- `ibis.config.yaml` + `ibis run` is all you need
- **Real-time streaming** -- SSE/WSS event streaming
- **Multiple DB backends** -- PostgreSQL, BadgerDB (embedded), or in-memory (dev/test)
- **Auto-generated REST API** -- Supabase-style query syntax with pagination, filtering, ordering
- **Reorg handling** -- revert/add operation pairs for safe pending block support
- **Backfill** -- automatic historical event catchup on startup via `starknet_getEvents`
- **Wildcard events** -- index all contract events with `name: "*"`, override specific ones as needed
- **Three table types** -- `log` (append-only), `unique` (last-write-wins), `aggregation` (auto-computed)

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
                       |  - Query CLI          |
                       +-----------------------+
```

For a deep dive into the architecture, data models, and design decisions, see [`docs/SPEC.md`](docs/SPEC.md).

## Installation

### asdf

```bash
asdf plugin add ibis https://github.com/b-j-roberts/asdf-ibis.git
asdf install ibis latest
asdf set -u ibis latest
```

### Binary release

Download the latest binary from [GitHub Releases](https://github.com/b-j-roberts/ibis/releases):

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
git clone https://github.com/b-j-roberts/ibis.git
cd ibis
make build
# Binary at ./bin/ibis
```

## Quick Start

### 1. Initialize a config

```bash
ibis init --contract 0x049d36570d4e46f48e99674bd3fcc84644ddd6b96f7c741b1562b82f9e004dc7
```

This fetches the contract ABI, lists available events, and generates an `ibis.config.yaml`.

### 2. Run the indexer

```bash
ibis run
```

Ibis connects to the Starknet RPC, subscribes to events, decodes them using the ABI, writes them to the configured database, and exposes a REST API.

### 3. Query data

```bash
# Via CLI
ibis query MyContract Transfer --limit 10 --format table

# Via REST API
curl "http://localhost:8080/v1/MyContract/Transfer?limit=10&order=block_number.desc"

# With filters
curl "http://localhost:8080/v1/MyContract/Transfer?sender=eq.0x123&block_number=gte.100000"
```

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

indexer:
  start_block: 0        # 0 = latest, or specific block number
  pending_blocks: true
  batch_size: 10

contracts:
  - name: MyContract
    address: "0x049d36570d4e46..."
    abi: fetch                  # fetch from chain, or local path
    events:
      - name: "*"               # Index all events as log tables
        table:
          type: log
      - name: LeaderboardUpdate # Override specific events
        table:
          type: unique
          unique_key: trader_address
```

See [`configs/ibis.config.yaml`](configs/ibis.config.yaml) for a fully documented example.

## API

Ibis auto-generates REST endpoints from your contract ABI:

```
GET  /v1/{contract}/{event}            # List events (paginated)
GET  /v1/{contract}/{event}/latest     # Latest event
GET  /v1/{contract}/{event}/count      # Count events
GET  /v1/{contract}/{event}/unique     # Unique table entries
GET  /v1/{contract}/{event}/aggregate  # Aggregated values
GET  /v1/{contract}/{event}/stream     # SSE real-time stream
GET  /v1/health                        # Health check
GET  /v1/status                        # Indexer status
```

Query parameters follow Supabase conventions:

- `?limit=50&offset=0` -- pagination
- `?order=block_number.desc` -- ordering
- `?field=eq.value` -- filtering (`eq`, `neq`, `gt`, `gte`, `lt`, `lte`)

## Docker

```bash
# Build and run
make docker-build
make docker-run

# Or with docker-compose (includes PostgreSQL)
make docker-compose-up
```

## Contributing

Contributions are welcome. See [`docs/ROADMAP.md`](docs/ROADMAP.md) for planned work and [`docs/SPEC.md`](docs/SPEC.md) for architecture details.

## License

[MIT](LICENSE)
