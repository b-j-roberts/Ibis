---
name: ibis-query
description: "This skill translates natural language questions about Starknet indexed data into ibis CLI queries or REST API calls. It should be used when the user asks questions about blockchain data indexed by ibis, such as 'who has the highest score?', 'show me recent transfers', 'how many swaps happened today?', or any data question that can be answered by querying ibis tables. Triggered by natural language data questions when an ibis.config.yaml is present or when the user references ibis data."
---

# Ibis Natural Language Query Skill

Translate natural language questions about Starknet indexed data into REST API `curl` commands (preferred) or `ibis query` CLI commands (fallback).

## When to Use

Activate when the user asks a question about indexed blockchain data, such as:
- "Who has the highest score?"
- "Show me all transfers above 1000 STRK"
- "How many swaps happened in the last hour?"
- "What's the total trading volume?"
- "List all factory children"
- "What contracts were deployed by the factory?"
- "What's the current token price?" (view function)
- "What contracts were discovered?" (discovery)
- "Stream new transfer events" (SSE)

## Workflow

### Step 1: Discover Available Data

Before constructing a query, understand what data is indexed. Determine the API base URL from the config:

```
Base URL: http://{api.host}:{api.port}/v1
Default:  http://localhost:8080/v1
```

**Method A — Call the status endpoint** (preferred when API server is running):
```bash
curl -s http://localhost:8080/v1/status | jq .
```
This returns all contracts (with cursor positions), factory summaries (child counts, sync status), and view function statuses (last poll time, errors). It is the single best source of truth.

**Method B — Run `ibis query --list`** (when ibis binary is installed but API may not be running):
```bash
ibis query --list
```

**Method C — Read the config file directly** (fallback):
```bash
bash ~/.claude/skills/ibis-query/scripts/inspect_config.sh
```

From discovery, extract:
- Contract names and addresses
- Event names and table types (`log`, `unique`, `aggregation`)
- View functions (contract + function name → view table)
- Factory configurations and child counts
- Discovery configs (class hashes being watched)

### Step 2: Map the Question to a Query

Consult the full reference at `references/query_reference.md` for detailed syntax.

**Identify the contract and event.** The user may use informal names. Map them to the exact contract and event names from the config. Table names are `lowercase(contract_event)`.

**Identify the query type:**

| User Intent | API Endpoint | CLI Fallback |
|---|---|---|
| "Show me events/transfers/swaps" | `GET /v1/{contract}/{event}` | `ibis query Contract Event` |
| "What's the latest/most recent...?" | `GET /v1/{contract}/{event}/latest` | `ibis query Contract Event --latest` |
| "How many...?" | `GET /v1/{contract}/{event}/count` | `ibis query Contract Event --count` |
| "Who has the highest/current...?" | `GET /v1/{contract}/{event}/unique` | `ibis query Contract Event --unique` |
| "What's the total/sum/average...?" | `GET /v1/{contract}/{event}/aggregate` | `ibis query Contract Event --aggregate` |
| "List deployed contracts/children" | `GET /v1/{factory}/children` | `ibis query Factory --children` |
| "How many children/pairs...?" | `GET /v1/{factory}/children/count` | `ibis query Factory --children-count` |
| "What's the current price/supply?" | `GET /v1/{contract}/{function}` | (view table — use API) |
| "Latest state of contract?" | `GET /v1/{contract}/{function}?order=block_number.desc&limit=1` | (view table — use API) |
| "What contracts were discovered?" | `GET /v1/discover/{classHash}/contracts` | (discovery — API only) |
| "How many instances of class X?" | `GET /v1/discover/{classHash}/contracts` then count | (discovery — API only) |
| "Stream events / watch for new..." | `curl -N .../v1/{contract}/{event}/stream` | (SSE — API only) |
| "Live tail of transfers" | `curl -N .../v1/{contract}/{event}/stream` | (SSE — API only) |

**Identify filters from natural language:**

| Natural Language | Query Parameter |
|---|---|
| "above/more than/greater than X" | `?field=gt.X` |
| "at least X" / "X or more" | `?field=gte.X` |
| "below/less than X" | `?field=lt.X` |
| "at most X" / "X or less" | `?field=lte.X` |
| "exactly X" / "equal to X" | `?field=eq.X` |
| "not X" / "excluding X" | `?field=neq.X` |
| "confirmed" / "finalized" | `?status=eq.ACCEPTED_L2` |
| "pending" | `?status=eq.PRE_CONFIRMED` |
| "from contract 0x..." | `?contract_address=eq.0x...` |

**Identify ordering:**

| Natural Language | Query Parameter |
|---|---|
| "highest/most/top" | `?order=field.desc` |
| "lowest/least/bottom" | `?order=field.asc` |
| "most recent/latest/newest" | `?order=block_number.desc` (default) |
| "oldest/earliest/first" | `?order=block_number.asc` |

**Identify limits:**

| Natural Language | Query Parameter |
|---|---|
| "top 10" / "first 5" | `?limit=10` / `?limit=5` |
| "all" (use carefully) | `?limit=500` |
| (not specified) | default `?limit=50` |

### Step 3: Construct and Execute the Query

Build a `curl` command targeting the ibis REST API. Default to `jq` for readable output.

**Command template:**
```bash
curl -s "http://localhost:8080/v1/{contract}/{event}?limit=N&offset=N&order=field.dir&field=op.value" | jq .
```

**SSE streaming template:**
```bash
curl -N "http://localhost:8080/v1/{contract}/{event}/stream"
```

With replay from a known position:
```bash
curl -N -H "Last-Event-ID: 850000:3" "http://localhost:8080/v1/{contract}/{event}/stream"
```

**CLI fallback** (when API server is not running):
```bash
ibis query <Contract> <Event> [--limit N] [--offset N] [--order field.dir] [--filter "field=op.value"] [--unique|--aggregate|--latest|--count] [--format table]
```

Execute the command via Bash and present the results.

### Step 4: Present Results with Context

After receiving query output:
- Summarize the key findings in natural language
- Highlight the answer to the user's question
- If the result set is large, point out notable entries
- If no results are found, suggest adjusting filters or checking the config

### Step 5: Handle Follow-ups

For follow-up questions, reuse the discovered schema context. Common patterns:
- "Show me more" → increase `limit` or add `offset`
- "Sort by X instead" → change `order` parameter
- "Only show confirmed" → add `&status=eq.ACCEPTED_L2`
- "What about contract 0x...?" → add `&contract_address=eq.0x...`
- "Stream those instead" → switch to `/stream` endpoint
- "What's the current value?" → switch to view table query

## View Function Tables

View tables store results from periodic on-chain function calls (e.g., `total_supply`, `get_price`). They differ from event tables:

- **Table naming**: `{contract}_{function}` (e.g., `mytoken_total_supply`)
- **Metadata columns**: `block_number`, `timestamp`, `contract_address`, `_view_key`
- **Missing columns**: View tables do NOT have `transaction_hash`, `log_index`, `event_name`, or `status`
- **`_view_key`**: Synthetic key column. For `unique` view tables, deduplicates to latest polled value. For `log` view tables, every poll result is appended.

**Query view tables the same way as event tables** — they use the same REST endpoints:
```bash
# Latest polled value
curl -s "http://localhost:8080/v1/Oracle/get_price?order=block_number.desc&limit=1" | jq .

# Current unique value (if view table is unique type)
curl -s "http://localhost:8080/v1/Oracle/get_price/unique" | jq .

# Historical values
curl -s "http://localhost:8080/v1/MyToken/total_supply?order=block_number.asc&limit=100" | jq .
```

**Filter/order view tables by**: `block_number`, `timestamp`, `contract_address`, `_view_key`, plus any decoded view output fields.

## Discovery Queries

Discovery mode watches for UDC `ContractDeployed` events matching specific class hashes and auto-registers new contracts.

**Check discovered contracts:**
```bash
curl -s "http://localhost:8080/v1/discover/0x{classHash}/contracts" | jq .
```

Response includes each discovered contract's `address`, `deployment_block`, `current_block`, and `status` (active/syncing/error/backfilling).

Discovery endpoints return **contract metadata**, not event data. If the user asks about data FROM discovered contracts, query those contracts' event/view tables instead.

## SSE Streaming

For real-time event watching, use the `/stream` endpoint:

```bash
# Stream all new Transfer events
curl -N "http://localhost:8080/v1/Token/Transfer/stream"

# Stream with filters
curl -N "http://localhost:8080/v1/Token/Transfer/stream?from_address=eq.0x..."

# Resume from a known position
curl -N -H "Last-Event-ID: 850000:3" "http://localhost:8080/v1/Token/Transfer/stream"
```

Event format:
```
id: {block_number}:{log_index}
data: {"block_number":850001,"transaction_hash":"0x...","from_address":"0x...","amount":"1000"}

```

Filters (`eq` and `neq` operators) are applied to both replayed and live events.

## Shared Tables

Factory children, discovered contracts, and admin-registered contracts can all use shared tables. Shared tables add a `contract_name` column for disambiguation.

**Filter by specific child contract:**
```bash
curl -s "http://localhost:8080/v1/DEX/Swap?contract_address=eq.0x..." | jq .
```

## Event Table Metadata Columns

Every event table always contains these columns (available for filtering/ordering):
- `block_number` — block height (uint64)
- `transaction_hash` — tx hash (string)
- `log_index` — position within block (uint64)
- `timestamp` — block timestamp as unix epoch (uint64)
- `contract_address` — source contract (string)
- `event_name` — event type name (string)
- `status` — confirmation status: `PRE_CONFIRMED`, `ACCEPTED_L2`, or `ACCEPTED_L1`
- `contract_name` — (shared tables only) which child contract emitted the event

## Table Types

- **log** — Append-only event log. Every emission is stored. Default query type.
- **unique** — Last-write-wins by `unique_key`. Query `/unique` endpoint to get current state per key.
- **aggregation** — Auto-computed sums/counts/averages. Query `/aggregate` endpoint to get computed values.

## Example Query Mappings

| Question | curl Command |
|---|---|
| "Show me recent transfers" | `curl -s ".../v1/Token/Transfer?limit=10&order=block_number.desc"` |
| "What's the total volume?" | `curl -s ".../v1/DEX/VolumeUpdate/aggregate"` |
| "Current token price?" | `curl -s ".../v1/Oracle/get_price?order=block_number.desc&limit=1"` |
| "List factory children" | `curl -s ".../v1/DEXFactory/children"` |
| "How many swaps today?" | `curl -s ".../v1/DEX/Swap/count?timestamp=gte.{unix_ts}"` |
| "Top 10 holders" | `curl -s ".../v1/Token/Transfer/unique?order=balance.desc&limit=10"` |
| "Stream new swaps" | `curl -N ".../v1/DEX/Swap/stream"` |
| "Discovered contracts for class X" | `curl -s ".../v1/discover/0x{hash}/contracts"` |
| "Is ibis healthy?" | `curl -s ".../v1/health"` |
| "Indexer progress?" | `curl -s ".../v1/status"` |

## Important Notes

- **REST API is preferred** — output `curl` commands unless the user specifically asks for CLI or the API server is not running
- Table names are `lowercase(ContractName_EventName)` — the API accepts the original casing in URL paths
- Filter values for felt252/address fields must be hex strings starting with `0x`
- Multiple filters combine with AND logic: `?field1=op.value1&field2=op.value2`
- Factory children queries only need the factory contract name (no event name)
- For shared tables, use `?contract_address=eq.0x...` to filter by specific child
- View tables use `_view_key` instead of `event_name` and lack `transaction_hash`/`log_index`/`status`
- SSE streaming supports `eq` and `neq` filter operators; numeric operators are ignored on replay
- The `/v1/status` endpoint is the best way to understand what data is available
