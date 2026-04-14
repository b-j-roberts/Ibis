---
name: ibis-admin
description: "This skill manages a running ibis indexer instance via the admin REST API. It should be used when a user wants to register a new contract, add a contract to the indexer, remove/deregister a contract, check indexer status or health, list registered contracts, or update contract configuration on a live ibis instance. Triggered by requests like 'register contract', 'add contract to indexer', 'remove contract', 'indexer status', 'is ibis healthy', 'list registered contracts', 'update contract config'. This skill operates against a RUNNING ibis instance via HTTP — for generating ibis.config.yaml files, use the ibis-config skill instead."
---

# Ibis Admin

Manage a running ibis indexer instance via the admin REST API. Register and deregister contracts dynamically, check indexer health and status, list registered contracts, and update contract configurations — all without restarting the indexer.

## When to Use

Activate when the user wants to interact with a **running** ibis instance:
- "Register this contract on the indexer"
- "Add the STRK token to ibis"
- "Remove the old contract"
- "Is ibis healthy?"
- "Show indexer status"
- "List all registered contracts"
- "Update the events for MyContract"

**Do NOT use** when the user wants to generate or edit `ibis.config.yaml` — that is the ibis-config skill's job. This skill operates via HTTP against a live ibis server.

## Workflow

### Step 1: Discover Server

Determine the ibis API server URL and authentication:

1. **Check for ibis.config.yaml** in the current directory or project root:
   - Read `api.host` and `api.port` to construct the base URL
   - Read `api.admin_key` to determine if authentication is required
2. **Default**: `http://localhost:8080` if no config is found
3. **Verify reachability** (optional but recommended):
   ```bash
   curl -s http://localhost:8080/v1/health | jq .
   ```
   Expected: `{"status": "ok"}`

If the health check fails with connection refused, inform the user that ibis does not appear to be running. Suggest `ibis run` or `make run` to start it.

### Step 2: Understand Intent

Classify the user's request into one of these intents:

| Intent | Trigger Phrases |
|--------|----------------|
| `register` | "register", "add", "index", "start tracking", "watch" |
| `deregister` | "remove", "delete", "deregister", "stop tracking", "unwatch" |
| `update` | "update", "modify", "change", "add events to", "reconfigure" |
| `list` | "list", "show contracts", "what's registered", "all contracts" |
| `status` | "status", "progress", "how far behind", "syncing", "cursors" |
| `health` | "health", "alive", "is ibis running", "ping", "healthy" |

### Step 3: Execute by Intent

#### Register Intent

Translate the user's natural language request into a `POST /v1/admin/contracts` JSON payload.

**Natural language to table type mapping:**

| User Says | Table Type | Notes |
|-----------|-----------|-------|
| "track", "log", "record", "index" | `log` | Append-only history |
| "leaderboard", "current state", "latest per", "balance", "position" | `unique` | Last-write-wins by key |
| "total", "sum", "count", "volume", "aggregate" | `aggregation` | Auto-computed metrics |
| "all events", "everything", "all" | wildcard `"*"` | Uses `log` type |

**Event name inference:**

| User Says | Event Name |
|-----------|-----------|
| "transfers" | `Transfer` |
| "swaps" | `Swap` |
| "approvals" | `Approval` |
| "mints" | `Mint` |
| "burns" | `Burn` |
| "deposits" | `Deposit` |
| "withdrawals" | `Withdraw` |

For simple registrations (1-2 events, known types), construct the payload directly. For complex registrations (factories, views, aggregations, many events), ask follow-up questions:
- "Which field contains the volume amount?" (for aggregation)
- "What field should be the unique key?" (for unique tables)
- "Should factory children share tables?" (for factory contracts)

Construct the curl command:
```bash
curl -s -X POST http://localhost:8080/v1/admin/contracts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ContractName",
    "address": "0x...",
    "abi": "fetch",
    "events": [
      {
        "name": "Transfer",
        "table": { "type": "log" }
      }
    ]
  }' | jq .
```

Read `references/admin_reference.md` for the full `ContractConfig` JSON schema, including factory, views, and aggregation structures.

#### Deregister Intent

Construct `DELETE /v1/admin/contracts/{name}`.

**IMPORTANT**: If the user mentions dropping tables or cleaning up data, add `?drop_tables=true` — but **ALWAYS confirm with the user first** since this is destructive and irreversible. Explain that shared tables (used by other factory children) will not be dropped.

```bash
curl -s -X DELETE "http://localhost:8080/v1/admin/contracts/ContractName" | jq .
```

With table cleanup (only after user confirmation):
```bash
curl -s -X DELETE "http://localhost:8080/v1/admin/contracts/ContractName?drop_tables=true" | jq .
```

#### Update Intent

Construct `PUT /v1/admin/contracts/{name}` with the updated JSON body.

First, fetch the current config to show what will change:
```bash
curl -s http://localhost:8080/v1/admin/contracts | jq '.contracts[] | select(.name == "ContractName")'
```

Then construct the update with the full contract config (the PUT replaces the entire config):
```bash
curl -s -X PUT http://localhost:8080/v1/admin/contracts/ContractName \
  -H "Content-Type: application/json" \
  -d '{ ... updated config ... }' | jq .
```

#### List Intent

Construct `GET /v1/admin/contracts` and format the response:

```bash
curl -s http://localhost:8080/v1/admin/contracts | jq .
```

Present results showing: contract name, address, event count, whether it's dynamic or a factory child, and factory/discover metadata if present.

#### Status Intent

Construct `GET /v1/status` and present results with context:

```bash
curl -s http://localhost:8080/v1/status | jq .
```

**Highlight actionable insights:**
- Contracts stuck at block 0 → "Never started indexing — check RPC connectivity"
- Large gaps between per-contract cursors → "Contract X is lagging behind by N blocks"
- Factory children backfilling → "N of M factory children are still catching up"
- Views with `consecutive_errors > 0` → "View function polling is failing — check contract address and ABI"
- All contracts synced to same block → "Indexer is fully synced"

#### Health Intent

Construct `GET /v1/health`:

```bash
curl -s http://localhost:8080/v1/health | jq .
```

Present: "Ibis is healthy and running" or explain the failure.

### Step 4: Execute and Present

Run the constructed `curl` command via Bash. Parse the response and present results in human-readable format with contextual interpretation:

- **Register success**: "Contract registered successfully — it will start indexing from block {start_block}. Events: {event_list}"
- **Register with factory**: "Factory contract registered. Child contracts will be auto-discovered when {event} is emitted"
- **Deregister success**: "Contract removed. Tables {were/were not} dropped"
- **Update success**: "Contract config updated. Changes: {diff summary}"
- **List**: Format as a readable table with key info
- **Status**: Summarize sync progress, highlight lagging contracts or errors
- **Health**: Confirm healthy or explain issue

## Authentication Handling

Authentication is context-aware:

1. **Check config first**: Read `ibis.config.yaml` for `api.admin_key`
2. **If key found**: Always include `-H "X-Admin-Key: {key}"` in admin endpoint curl commands
3. **If key is an env var** (e.g., `${IBIS_ADMIN_KEY}`): Use `-H "X-Admin-Key: $IBIS_ADMIN_KEY"` (shell expansion)
4. **If no key configured**: Omit the header entirely
5. **If server returns 401**: Suggest configuring the admin key — "The server requires an admin key. Set `api.admin_key` in ibis.config.yaml or pass it via the `X-Admin-Key` header"

Note: `GET /v1/health` and `GET /v1/status` do NOT require authentication.

## Error Handling

Map common HTTP status codes to user-friendly explanations:

| Status Code | Meaning | Suggested Action |
|-------------|---------|------------------|
| 401 | Admin key is wrong or missing | Check `api.admin_key` in config or set `X-Admin-Key` header |
| 503 | Engine not running | Is `ibis run` started? Dynamic registration requires a running engine |
| 500 with "already registered" | Contract already exists | Use update instead, or deregister first then re-register |
| 404 | Contract not found | Check the contract name — run `GET /v1/admin/contracts` to list all |
| 400 | Invalid request body | Check JSON format — name and address are required, events must be valid |

All error responses follow the format: `{"error": "message"}`. Parse and present the error message clearly.

## Example Workflows

### Register a Simple ERC20
```bash
curl -s -X POST http://localhost:8080/v1/admin/contracts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "STRK",
    "address": "0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d",
    "abi": "fetch",
    "events": [
      { "name": "Transfer", "table": { "type": "log" } },
      { "name": "Approval", "table": { "type": "log" } }
    ]
  }' | jq .
```

### Register a Factory Contract with Shared Tables
```bash
curl -s -X POST http://localhost:8080/v1/admin/contracts \
  -H "Content-Type: application/json" \
  -d '{
    "name": "DEXFactory",
    "address": "0x01aa950c...",
    "abi": "fetch",
    "events": [
      { "name": "PairCreated", "table": { "type": "log" } }
    ],
    "factory": {
      "event": "PairCreated",
      "child_address_field": "pair",
      "child_abi": "fetch",
      "shared_tables": true,
      "child_name_template": "{factory}_{token0}_{token1}",
      "child_events": [
        { "name": "*", "table": { "type": "log" } },
        {
          "name": "Swap",
          "table": {
            "type": "aggregation",
            "aggregate": [
              { "column": "total_volume", "operation": "sum", "field": "amount_in" },
              { "column": "swap_count", "operation": "count", "field": "amount_in" }
            ]
          }
        }
      ]
    }
  }' | jq .
```

### Deregister with Table Cleanup
```bash
curl -s -X DELETE "http://localhost:8080/v1/admin/contracts/OldContract?drop_tables=true" | jq .
```

### Update to Add New Events
```bash
curl -s -X PUT http://localhost:8080/v1/admin/contracts/MyToken \
  -H "Content-Type: application/json" \
  -d '{
    "name": "MyToken",
    "address": "0x04718f5a...",
    "abi": "fetch",
    "events": [
      { "name": "Transfer", "table": { "type": "log" } },
      { "name": "Approval", "table": { "type": "log" } },
      { "name": "OwnershipTransferred", "table": { "type": "log" } }
    ]
  }' | jq .
```

### Check Indexer Health
```bash
curl -s http://localhost:8080/v1/health | jq .
```

### With Admin Key Authentication
```bash
curl -s -X POST http://localhost:8080/v1/admin/contracts \
  -H "Content-Type: application/json" \
  -H "X-Admin-Key: $IBIS_ADMIN_KEY" \
  -d '{ ... }' | jq .
```

## Reference

For complete admin API reference including all endpoints, request/response JSON formats, HTTP status codes, `ContractConfig` schema with all nested structures (events, views, factory, aggregation), and runtime-only fields: read `references/admin_reference.md`.
