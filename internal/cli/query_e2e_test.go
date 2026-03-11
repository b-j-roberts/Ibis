package cli

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/store/memory"
	"github.com/b-j-roberts/ibis/internal/types"
)

// setupE2E creates a config file and returns its path along with a populated
// in-memory store containing test data for Transfer (log), Balance (unique),
// and Volume (aggregation) tables.
func setupE2E(t *testing.T) (cfgFile string, st *memory.MemoryStore) {
	t.Helper()

	dir := t.TempDir()
	cfg := `network: sepolia
rpc: wss://starknet-sepolia.example.com
database:
  backend: memory
contracts:
  - name: MyContract
    address: "0x1234567890abcdef"
    abi: fetch
    events:
      - name: Transfer
        table:
          type: log
      - name: Balance
        table:
          type: unique
          unique_key: account
      - name: Volume
        table:
          type: aggregation
          aggregate:
            - column: total_volume
              operation: sum
              field: amount
            - column: trade_count
              operation: count
              field: amount
`
	cfgFile = filepath.Join(dir, "ibis.config.yaml")
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	st = memory.New()
	ctx := t.Context()

	// Create Transfer (log) table and insert events.
	if err := st.CreateTable(ctx, &types.TableSchema{
		Name:      "mycontract_transfer",
		Contract:  "MyContract",
		Event:     "Transfer",
		TableType: types.TableTypeLog,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "log_index", Type: "uint64"},
			{Name: "transaction_hash", Type: "string"},
			{Name: "event_name", Type: "string"},
			{Name: "status", Type: "string"},
			{Name: "from", Type: "string"},
			{Name: "to", Type: "string"},
			{Name: "amount", Type: "string"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	transferOps := []store.Operation{
		{Type: store.OpInsert, Table: "mycontract_transfer", Key: "50:0", BlockNumber: 50, LogIndex: 0, Data: map[string]any{
			"block_number": uint64(50), "log_index": uint64(0), "transaction_hash": "0xaaa",
			"event_name": "Transfer", "status": "ACCEPTED_L2",
			"from": "0xalice", "to": "0xbob", "amount": "100",
		}},
		{Type: store.OpInsert, Table: "mycontract_transfer", Key: "100:0", BlockNumber: 100, LogIndex: 0, Data: map[string]any{
			"block_number": uint64(100), "log_index": uint64(0), "transaction_hash": "0xbbb",
			"event_name": "Transfer", "status": "ACCEPTED_L2",
			"from": "0xbob", "to": "0xcharlie", "amount": "200",
		}},
		{Type: store.OpInsert, Table: "mycontract_transfer", Key: "150:0", BlockNumber: 150, LogIndex: 0, Data: map[string]any{
			"block_number": uint64(150), "log_index": uint64(0), "transaction_hash": "0xccc",
			"event_name": "Transfer", "status": "ACCEPTED_L2",
			"from": "0xcharlie", "to": "0xdave", "amount": "300",
		}},
		{Type: store.OpInsert, Table: "mycontract_transfer", Key: "200:0", BlockNumber: 200, LogIndex: 0, Data: map[string]any{
			"block_number": uint64(200), "log_index": uint64(0), "transaction_hash": "0xddd",
			"event_name": "Transfer", "status": "ACCEPTED_L2",
			"from": "0xdave", "to": "0xalice", "amount": "500",
		}},
	}
	if err := st.ApplyOperations(ctx, transferOps); err != nil {
		t.Fatal(err)
	}

	// Create Balance (unique) table and insert events.
	if err := st.CreateTable(ctx, &types.TableSchema{
		Name:      "mycontract_balance",
		Contract:  "MyContract",
		Event:     "Balance",
		TableType: types.TableTypeUnique,
		UniqueKey: "account",
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "event_name", Type: "string"},
			{Name: "account", Type: "string"},
			{Name: "balance", Type: "string"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	balanceOps := []store.Operation{
		{Type: store.OpInsert, Table: "mycontract_balance", Key: "100:1", BlockNumber: 100, LogIndex: 1, Data: map[string]any{
			"block_number": uint64(100), "event_name": "Balance",
			"account": "0xalice", "balance": "1000",
		}},
		{Type: store.OpInsert, Table: "mycontract_balance", Key: "101:0", BlockNumber: 101, LogIndex: 0, Data: map[string]any{
			"block_number": uint64(101), "event_name": "Balance",
			"account": "0xbob", "balance": "500",
		}},
		// Update alice's balance at a later block.
		{Type: store.OpInsert, Table: "mycontract_balance", Key: "200:1", BlockNumber: 200, LogIndex: 1, Data: map[string]any{
			"block_number": uint64(200), "event_name": "Balance",
			"account": "0xalice", "balance": "1500",
		}},
	}
	if err := st.ApplyOperations(ctx, balanceOps); err != nil {
		t.Fatal(err)
	}

	// Create Volume (aggregation) table and insert events.
	if err := st.CreateTable(ctx, &types.TableSchema{
		Name:      "mycontract_volume",
		Contract:  "MyContract",
		Event:     "Volume",
		TableType: types.TableTypeAggregation,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "event_name", Type: "string"},
			{Name: "amount", Type: "string"},
		},
		Aggregates: []types.AggregateSpec{
			{Column: "total_volume", Operation: "sum", Field: "amount"},
			{Column: "trade_count", Operation: "count", Field: "amount"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	volumeOps := []store.Operation{
		{Type: store.OpInsert, Table: "mycontract_volume", Key: "100:2", BlockNumber: 100, LogIndex: 2, Data: map[string]any{
			"block_number": uint64(100), "event_name": "Volume", "amount": float64(100),
		}},
		{Type: store.OpInsert, Table: "mycontract_volume", Key: "200:2", BlockNumber: 200, LogIndex: 2, Data: map[string]any{
			"block_number": uint64(200), "event_name": "Volume", "amount": float64(250),
		}},
	}
	if err := st.ApplyOperations(ctx, volumeOps); err != nil {
		t.Fatal(err)
	}

	return cfgFile, st
}

// setQueryDefaults resets all query flag vars to defaults and returns a cleanup function.
func setQueryDefaults(t *testing.T) func() {
	t.Helper()
	origLimit := queryLimit
	origOffset := queryOffset
	origOrder := queryOrder
	origFilters := queryFilters
	origUnique := queryUnique
	origAggregate := queryAggregate
	origFormat := queryFormat
	origList := queryList
	origCfgPath := cfgPath
	origOverride := testCreateStoreOverride

	// Set defaults.
	queryLimit = 50
	queryOffset = 0
	queryOrder = "block_number.desc"
	queryFilters = nil
	queryUnique = false
	queryAggregate = false
	queryFormat = "json"
	queryList = false

	return func() {
		queryLimit = origLimit
		queryOffset = origOffset
		queryOrder = origOrder
		queryFilters = origFilters
		queryUnique = origUnique
		queryAggregate = origAggregate
		queryFormat = origFormat
		queryList = origList
		cfgPath = origCfgPath
		testCreateStoreOverride = origOverride
	}
}

// runQueryDirect calls runQuery bypassing cobra dispatch. It sets flag vars,
// the config path, and the store override, then calls runQuery with a
// command that captures output.
func runQueryDirect(t *testing.T, cfgFile string, st store.Store, args []string) (string, error) {
	t.Helper()

	cleanup := setQueryDefaults(t)
	defer cleanup()

	cfgPath = cfgFile
	testCreateStoreOverride = func() store.Store { return st }

	// Parse flags from args manually.
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			i++
			fmt.Sscanf(args[i], "%d", &queryLimit)
		case "--offset":
			i++
			fmt.Sscanf(args[i], "%d", &queryOffset)
		case "--order":
			i++
			queryOrder = args[i]
		case "--filter":
			i++
			queryFilters = append(queryFilters, args[i])
		case "--unique":
			queryUnique = true
		case "--aggregate":
			queryAggregate = true
		case "--format":
			i++
			queryFormat = args[i]
		case "--list":
			queryList = true
		}
	}

	// Extract positional args (non-flag arguments).
	var positional []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "--") {
			// Skip flag and its value if it takes one.
			switch args[i] {
			case "--unique", "--aggregate", "--list":
				// Bool flags, no value.
			default:
				i++ // Skip value.
			}
		} else {
			positional = append(positional, args[i])
		}
	}

	var buf bytes.Buffer
	cmd := queryCmd
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := runQuery(cmd, positional)
	return buf.String(), err
}

// --- Test Plan Items ---

func TestE2E_01_Help(t *testing.T) {
	// Test 1: ibis query --help — verify the command definition is correct.
	cmd := queryCmd

	if cmd.Short != "Query indexed data from the terminal" {
		t.Errorf("Short: got %q", cmd.Short)
	}
	if !strings.Contains(cmd.Long, "Query indexed event data") {
		t.Error("missing long description")
	}

	// Check all expected flags are registered.
	expectedFlags := []string{"limit", "offset", "order", "filter", "unique", "aggregate", "format", "list"}
	for _, name := range expectedFlags {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag: --%s", name)
		}
	}

	// Check examples are in the long description.
	if !strings.Contains(cmd.Long, "ibis query MyContract Transfer") {
		t.Error("missing usage examples in Long description")
	}

	t.Log("PASS: Help output shows usage, all flags, and examples")
}

func TestE2E_02_List(t *testing.T) {
	// Test 2: ibis query --list
	cfgFile, st := setupE2E(t)
	output, err := runQueryDirect(t, cfgFile, st, []string{"--list"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := map[string]string{
		"MyContract":          "contract name",
		"0x1234567890abcdef":  "contract address",
		"Transfer":            "Transfer event",
		"Balance":             "Balance event",
		"Volume":              "Volume event",
		"mycontract_transfer": "table name for Transfer",
		"type=log":            "table type log",
		"type=unique":         "table type unique",
		"type=aggregation":    "table type aggregation",
	}
	for substr, desc := range checks {
		if !strings.Contains(output, substr) {
			t.Errorf("should show %s (%q not in output)", desc, substr)
		}
	}
	t.Log("PASS: --list shows contracts, events, types, and table names")
}

func TestE2E_03_NoArgs(t *testing.T) {
	// Test 3: ibis query (no args, no --list) gives usage error.
	cfgFile, st := setupE2E(t)
	_, err := runQueryDirect(t, cfgFile, st, []string{})
	if err == nil {
		t.Fatal("expected error when no args provided")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "ibis query <contract> <event>") {
		t.Errorf("error should mention usage, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "--list") {
		t.Errorf("error should suggest --list, got: %s", errMsg)
	}
	t.Log("PASS: No-args gives clear usage error suggesting --list")
}

func TestE2E_04_TableFormat(t *testing.T) {
	// Test 4: ibis query MyContract Transfer --format table
	cfgFile, st := setupE2E(t)
	output, err := runQueryDirect(t, cfgFile, st, []string{"MyContract", "Transfer", "--format", "table"})
	if err != nil {
		t.Fatalf("unexpected error: %v\nOutput: %s", err, output)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should have header + separator + 4 data rows = 6 lines.
	if len(lines) != 6 {
		t.Errorf("expected 6 lines (header + sep + 4 rows), got %d:\n%s", len(lines), output)
	}

	// Header should contain column names.
	header := lines[0]
	for _, col := range []string{"block_number", "from", "to", "amount"} {
		if !strings.Contains(header, col) {
			t.Errorf("header missing %q", col)
		}
	}

	// Separator row should have dashes.
	if !strings.Contains(lines[1], "---") {
		t.Error("separator row missing dashes")
	}

	// Data rows should contain actual values.
	if !strings.Contains(output, "0xalice") {
		t.Error("table output missing '0xalice'")
	}
	if !strings.Contains(output, "0xbob") {
		t.Error("table output missing '0xbob'")
	}
	t.Log("PASS: Table format shows aligned header, separator, and data rows")
}

func TestE2E_05_CSVFormat(t *testing.T) {
	// Test 5: ibis query MyContract Transfer --format csv
	cfgFile, st := setupE2E(t)
	output, err := runQueryDirect(t, cfgFile, st, []string{"MyContract", "Transfer", "--format", "csv"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := csv.NewReader(strings.NewReader(output))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v\nOutput: %s", err, output)
	}

	// Header + 4 data rows = 5.
	if len(records) != 5 {
		t.Errorf("expected 5 CSV rows, got %d", len(records))
	}

	// Header should have column names.
	header := records[0]
	hasBlockNumber := false
	hasAmount := false
	for _, col := range header {
		if col == "block_number" {
			hasBlockNumber = true
		}
		if col == "amount" {
			hasAmount = true
		}
	}
	if !hasBlockNumber {
		t.Error("CSV header missing block_number")
	}
	if !hasAmount {
		t.Error("CSV header missing amount")
	}
	t.Log("PASS: CSV format produces valid CSV with header and data rows")
}

func TestE2E_06_FilterAndLimit(t *testing.T) {
	// Test 6: ibis query MyContract Transfer --filter "block_number=gte.100" --limit 5
	cfgFile, st := setupE2E(t)
	output, err := runQueryDirect(t, cfgFile, st, []string{
		"MyContract", "Transfer", "--filter", "block_number=gte.100", "--limit", "5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v\nOutput: %s", err, output)
	}

	var results []map[string]any
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("invalid JSON: %v\nOutput: %s", err, output)
	}

	// Blocks: 50, 100, 150, 200. Filter >=100 gives blocks 100, 150, 200 = 3 results.
	if len(results) != 3 {
		t.Errorf("expected 3 results (blocks 100,150,200), got %d", len(results))
		for i, r := range results {
			t.Logf("  result[%d]: block_number=%v", i, r["block_number"])
		}
	}

	// Verify all results have block_number >= 100.
	for i, r := range results {
		bn, ok := r["block_number"]
		if !ok {
			t.Errorf("result[%d] missing block_number", i)
			continue
		}
		if bnf, ok := bn.(float64); ok && bnf < 100 {
			t.Errorf("result[%d] block_number=%v should be >= 100", i, bn)
		}
	}
	t.Log("PASS: Filter gte.100 returns only events with block_number >= 100")
}

func TestE2E_07_UniqueQuery(t *testing.T) {
	// Test 7: ibis query MyContract Balance --unique
	cfgFile, st := setupE2E(t)
	output, err := runQueryDirect(t, cfgFile, st, []string{"MyContract", "Balance", "--unique"})
	if err != nil {
		t.Fatalf("unexpected error: %v\nOutput: %s", err, output)
	}

	var results []map[string]any
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("invalid JSON: %v\nOutput: %s", err, output)
	}

	// Should have 2 unique accounts: alice (latest=1500) and bob (500).
	if len(results) != 2 {
		t.Errorf("expected 2 unique entries, got %d", len(results))
	}

	for _, r := range results {
		switch r["account"] {
		case "0xalice":
			if r["balance"] != "1500" {
				t.Errorf("alice balance: got %v, want 1500", r["balance"])
			}
		case "0xbob":
			if r["balance"] != "500" {
				t.Errorf("bob balance: got %v, want 500", r["balance"])
			}
		}
	}
	t.Log("PASS: --unique returns latest entry per unique key")
}

func TestE2E_08_AggregateQuery(t *testing.T) {
	// Test 8: ibis query MyContract Volume --aggregate
	cfgFile, st := setupE2E(t)
	output, err := runQueryDirect(t, cfgFile, st, []string{"MyContract", "Volume", "--aggregate"})
	if err != nil {
		t.Fatalf("unexpected error: %v\nOutput: %s", err, output)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nOutput: %s", err, output)
	}

	// total_volume should be 100 + 250 = 350.
	if tv, ok := result["total_volume"]; ok {
		if tvf, ok := tv.(float64); !ok || tvf != 350 {
			t.Errorf("total_volume: got %v, want 350", tv)
		}
	} else {
		t.Error("missing total_volume in aggregation result")
	}

	// trade_count should be 2.
	if tc, ok := result["trade_count"]; ok {
		if tcf, ok := tc.(float64); !ok || tcf != 2 {
			t.Errorf("trade_count: got %v, want 2", tc)
		}
	} else {
		t.Error("missing trade_count in aggregation result")
	}
	t.Log("PASS: --aggregate returns computed sum and count values")
}

func TestE2E_09_InvalidFormat(t *testing.T) {
	// Test 9: ibis query MyContract Transfer --format xml (invalid)
	cfgFile, st := setupE2E(t)
	_, err := runQueryDirect(t, cfgFile, st, []string{"MyContract", "Transfer", "--format", "xml"})
	if err == nil {
		t.Fatal("expected error for invalid format")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "unknown format") {
		t.Errorf("error should mention unknown format, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "json, table, or csv") {
		t.Errorf("error should list valid formats, got: %s", errMsg)
	}
	t.Log("PASS: Invalid format gives error listing valid options")
}

// --- Additional edge case tests ---

func TestE2E_OrderAsc(t *testing.T) {
	cfgFile, st := setupE2E(t)
	output, err := runQueryDirect(t, cfgFile, st, []string{
		"MyContract", "Transfer", "--order", "block_number.asc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var results []map[string]any
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(results) < 2 {
		t.Fatal("expected at least 2 results")
	}

	first := results[0]["block_number"].(float64)
	last := results[len(results)-1]["block_number"].(float64)
	if first > last {
		t.Errorf("ascending order: first=%v should be <= last=%v", first, last)
	}
	t.Log("PASS: --order block_number.asc returns results in ascending order")
}

func TestE2E_LimitOffset(t *testing.T) {
	cfgFile, st := setupE2E(t)

	// Page 1: first 2 results.
	output1, err := runQueryDirect(t, cfgFile, st, []string{
		"MyContract", "Transfer", "--limit", "2", "--offset", "0",
	})
	if err != nil {
		t.Fatal(err)
	}
	var page1 []map[string]any
	json.Unmarshal([]byte(output1), &page1)

	// Page 2: next 2 results.
	output2, err := runQueryDirect(t, cfgFile, st, []string{
		"MyContract", "Transfer", "--limit", "2", "--offset", "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	var page2 []map[string]any
	json.Unmarshal([]byte(output2), &page2)

	if len(page1) != 2 {
		t.Errorf("page 1: expected 2 results, got %d", len(page1))
	}
	if len(page2) != 2 {
		t.Errorf("page 2: expected 2 results, got %d", len(page2))
	}

	// Pages should have different data.
	if len(page1) > 0 && len(page2) > 0 {
		if fmt.Sprint(page1[0]["block_number"]) == fmt.Sprint(page2[0]["block_number"]) {
			t.Error("page 1 and page 2 should have different first results")
		}
	}
	t.Log("PASS: --limit and --offset provide correct pagination")
}

func TestE2E_EmptyResults(t *testing.T) {
	cfgFile, st := setupE2E(t)
	output, err := runQueryDirect(t, cfgFile, st, []string{
		"MyContract", "Transfer", "--filter", "block_number=gt.9999",
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output, "No results found") {
		t.Errorf("expected 'No results found' message, got: %s", output)
	}
	t.Log("PASS: Empty results show friendly message")
}
