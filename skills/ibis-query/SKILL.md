---
name: ibis-query
description: "This skill translates natural language questions about Starknet indexed data into ibis CLI queries or REST API calls. It should be used when the user asks questions about blockchain data indexed by ibis, such as 'who has the highest score?', 'show me recent transfers', 'how many swaps happened today?', or any data question that can be answered by querying ibis tables. Triggered by natural language data questions when an ibis.config.yaml is present or when the user references ibis data."
---

# Ibis Natural Language Query Skill

Translate natural language questions about Starknet indexed data into precise `ibis query` CLI commands.

## When to Use

Activate when the user asks a question about indexed blockchain data, such as:
- "Who has the highest score?"
- "Show me all transfers above 1000 STRK"
- "How many swaps happened in the last hour?"
- "What's the total trading volume?"
- "List all factory children"
- "What contracts were deployed by the factory?"

## Workflow

### Step 1: Discover Available Data

Before constructing a query, understand what data is indexed. Use one or both methods:

**Method A — Run `ibis query --list`** (preferred when ibis is installed):
```bash
ibis query --list
```
This prints all contracts, events, table types, and table names.

**Method B — Read the config file directly:**
Search for `ibis.config.yaml` in the current project or common locations:
```bash
# Common locations to check
find . -name "ibis.config.yaml" -maxdepth 3 2>/dev/null
cat ibis.config.yaml 2>/dev/null || cat configs/ibis.config.yaml 2>/dev/null
```

From the config, extract:
- Contract names and addresses
- Event names and table types (`log`, `unique`, `aggregation`)
- Factory configurations (if any)
- Unique keys and aggregation definitions

### Step 2: Map the Question to a Query

Consult the full query reference at `references/query_reference.md` for detailed syntax. Key mapping rules:

**Identify the contract and event.** The user may use informal names. Map them to the exact contract and event names from the config. Table names are `lowercase(contract_event)`.

**Identify the query type:**

| User Intent | Query Type | Flag/Endpoint |
|---|---|---|
| "Show me events/transfers/swaps" | Log query | (default) |
| "What's the latest/most recent...?" | Latest | `--latest` |
| "How many...?" | Count | `--count` |
| "Who has the highest/current...?" | Unique | `--unique` |
| "What's the total/sum/average...?" | Aggregation | `--aggregate` |
| "List deployed contracts/children" | Factory children | `--children` |
| "How many children/pairs...?" | Factory count | `--children-count` |

**Identify filters from natural language:**

| Natural Language | Filter |
|---|---|
| "above/more than/greater than X" | `--filter "field=gt.X"` |
| "at least X" / "X or more" | `--filter "field=gte.X"` |
| "below/less than X" | `--filter "field=lt.X"` |
| "at most X" / "X or less" | `--filter "field=lte.X"` |
| "exactly X" / "equal to X" | `--filter "field=eq.X"` |
| "not X" / "excluding X" | `--filter "field=neq.X"` |
| "confirmed" / "finalized" | `--filter "status=eq.ACCEPTED_L2"` |
| "pending" | `--filter "status=eq.PRE_CONFIRMED"` |
| "from contract 0x..." | `--contract-address 0x...` |

**Identify ordering:**

| Natural Language | Order Flag |
|---|---|
| "highest/most/top" | `--order field.desc` |
| "lowest/least/bottom" | `--order field.asc` |
| "most recent/latest/newest" | `--order block_number.desc` (default) |
| "oldest/earliest/first" | `--order block_number.asc` |

**Identify limits:**

| Natural Language | Limit |
|---|---|
| "top 10" / "first 5" | `--limit 10` / `--limit 5` |
| "all" (use carefully) | `--limit 500` |
| (not specified) | default `--limit 50` |

### Step 3: Construct and Execute the Query

Build the `ibis query` command from the mappings above. Always use `--format table` for readable output unless the user requests JSON or CSV.

**Command template:**
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
- "Show me more" -> increase `--limit` or use `--offset`
- "Sort by X instead" -> change `--order`
- "Only show confirmed" -> add `--filter "status=eq.ACCEPTED_L2"`
- "What about contract 0x...?" -> add `--contract-address`

## REST API Alternative

When the ibis API server is running (`ibis run`), queries can also be made via HTTP. Prefer CLI for this skill, but the API is useful when the user asks for API examples or when constructing webhook/integration queries.

Endpoint pattern: `GET http://localhost:8080/v1/{contract}/{event}?limit=50&order=block_number.desc&field=op.value`

See `references/query_reference.md` for the full endpoint listing.

## Metadata Columns

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
- **unique** — Last-write-wins by `unique_key`. Query with `--unique` to get current state per key.
- **aggregation** — Auto-computed sums/counts/averages. Query with `--aggregate` to get computed values.

## Important Notes

- Table names are `lowercase(ContractName_EventName)` — the CLI accepts the original casing as arguments
- Filter values for felt252/address fields must be hex strings starting with `0x`
- The `--filter` flag can be repeated for AND semantics
- Factory children queries only need the factory contract name (no event name)
- For shared factory tables, use `--contract-address` to filter by specific child
