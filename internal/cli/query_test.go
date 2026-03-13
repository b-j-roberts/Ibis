package cli

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/store/memory"
	"github.com/b-j-roberts/ibis/internal/types"
)

// writeTestConfig creates a minimal ibis config file for testing.
func writeTestConfig(t *testing.T, dir string) string {
	t.Helper()
	cfg := `network: sepolia
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: TestContract
    address: "0x1234"
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
            - column: total
              operation: sum
              field: amount
`
	path := filepath.Join(dir, "ibis.config.yaml")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFilterFlag(t *testing.T) {
	tests := []struct {
		input    string
		field    string
		operator string
		value    string
		wantErr  bool
	}{
		{"block_number=gte.100", "block_number", "gte", "100", false},
		{"status=ACCEPTED_L2", "status", "eq", "ACCEPTED_L2", false},
		{"amount=lt.500", "amount", "lt", "500", false},
		{"name=eq.alice", "name", "eq", "alice", false},
		{"count=neq.0", "count", "neq", "0", false},
		{"price=gt.1000", "price", "gt", "1000", false},
		{"score=lte.99", "score", "lte", "99", false},
		// No operator prefix defaults to eq.
		{"trader=0x123", "trader", "eq", "0x123", false},
		// Invalid formats.
		{"noequals", "", "", "", true},
		{"=value", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			f, err := parseFilterFlag(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Field != tt.field {
				t.Errorf("field: got %q, want %q", f.Field, tt.field)
			}
			if f.Operator != tt.operator {
				t.Errorf("operator: got %q, want %q", f.Operator, tt.operator)
			}
			if f.Value.(string) != tt.value {
				t.Errorf("value: got %q, want %q", f.Value, tt.value)
			}
		})
	}
}

func TestBuildQuery(t *testing.T) {
	// Save and restore package vars.
	origLimit, origOffset, origOrder, origFilters := queryLimit, queryOffset, queryOrder, queryFilters
	defer func() {
		queryLimit, queryOffset, queryOrder, queryFilters = origLimit, origOffset, origOrder, origFilters
	}()

	queryLimit = 10
	queryOffset = 5
	queryOrder = "log_index.asc"
	queryFilters = []string{"block_number=gte.100", "status=ACCEPTED_L2"}

	q, err := buildQuery()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if q.Limit != 10 {
		t.Errorf("Limit: got %d, want 10", q.Limit)
	}
	if q.Offset != 5 {
		t.Errorf("Offset: got %d, want 5", q.Offset)
	}
	if q.OrderBy != "log_index" {
		t.Errorf("OrderBy: got %q, want %q", q.OrderBy, "log_index")
	}
	if q.OrderDir != store.OrderAsc {
		t.Errorf("OrderDir: got %v, want OrderAsc", q.OrderDir)
	}
	if len(q.Filters) != 2 {
		t.Fatalf("Filters: got %d, want 2", len(q.Filters))
	}
	if q.Filters[0].Field != "block_number" || q.Filters[0].Operator != "gte" {
		t.Errorf("Filter[0]: got %+v", q.Filters[0])
	}
	if q.Filters[1].Field != "status" || q.Filters[1].Operator != "eq" {
		t.Errorf("Filter[1]: got %+v", q.Filters[1])
	}
}

func TestBuildQuery_InvalidOrder(t *testing.T) {
	origOrder := queryOrder
	defer func() { queryOrder = origOrder }()

	queryOrder = "block_number.sideways"
	queryLimit = 50
	queryOffset = 0
	queryFilters = nil

	_, err := buildQuery()
	if err == nil {
		t.Error("expected error for invalid order direction")
	}
}

func TestCollectColumns(t *testing.T) {
	events := []types.IndexedEvent{
		{Data: map[string]any{"block_number": uint64(1), "event_name": "A", "custom_field": "x"}},
		{Data: map[string]any{"block_number": uint64(2), "status": "ok", "another_field": "y"}},
	}

	cols := collectColumns(events)

	// Metadata columns should come first (in order), then extra sorted.
	if len(cols) == 0 {
		t.Fatal("expected columns, got none")
	}

	// block_number should be first (it's the first metadata column present).
	if cols[0] != "block_number" {
		t.Errorf("first column: got %q, want %q", cols[0], "block_number")
	}

	// Check that extra columns are sorted after metadata.
	metaCount := 0
	for _, c := range cols {
		switch c {
		case "block_number", "event_name", "status":
			metaCount++
		}
	}
	if metaCount != 3 {
		t.Errorf("expected 3 metadata columns, got %d", metaCount)
	}

	// Extra columns should include both custom_field and another_field, sorted.
	extraStart := -1
	for i, c := range cols {
		if c == "another_field" || c == "custom_field" {
			if extraStart == -1 {
				extraStart = i
			}
		}
	}
	if extraStart == -1 {
		t.Error("extra columns not found")
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{float64(42), "42"},
		{float64(3.14), "3.14"},
		{true, "true"},
		{false, "false"},
		{uint64(100), "100"},
		{map[string]any{"a": 1}, `{"a":1}`},
	}

	for _, tt := range tests {
		got := formatValue(tt.input)
		if got != tt.want {
			t.Errorf("formatValue(%v): got %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestOutputJSON(t *testing.T) {
	events := []types.IndexedEvent{
		{Data: map[string]any{"block_number": float64(100), "event_name": "Transfer"}},
	}

	var buf bytes.Buffer
	if err := outputJSON(&buf, events); err != nil {
		t.Fatal(err)
	}

	var result []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0]["event_name"] != "Transfer" {
		t.Errorf("event_name: got %v, want Transfer", result[0]["event_name"])
	}
}

func TestOutputTable(t *testing.T) {
	events := []types.IndexedEvent{
		{Data: map[string]any{"block_number": float64(100), "event_name": "Transfer", "amount": "500"}},
		{Data: map[string]any{"block_number": float64(101), "event_name": "Transfer", "amount": "200"}},
	}

	var buf bytes.Buffer
	if err := outputTable(&buf, events); err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Should have header, separator, and 2 data rows.
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines, got %d:\n%s", len(lines), output)
	}

	// Header should contain column names.
	if !strings.Contains(lines[0], "block_number") {
		t.Errorf("header missing block_number: %s", lines[0])
	}
	if !strings.Contains(lines[0], "amount") {
		t.Errorf("header missing amount: %s", lines[0])
	}

	// Separator should contain dashes.
	if !strings.Contains(lines[1], "---") {
		t.Errorf("separator missing dashes: %s", lines[1])
	}
}

func TestOutputCSV(t *testing.T) {
	events := []types.IndexedEvent{
		{Data: map[string]any{"block_number": float64(100), "event_name": "Transfer"}},
	}

	var buf bytes.Buffer
	if err := outputCSV(&buf, events); err != nil {
		t.Fatal(err)
	}

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("invalid CSV: %v", err)
	}

	// Header + 1 data row.
	if len(records) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(records))
	}
}

func TestOutputAggregation(t *testing.T) {
	result := store.AggResult{
		Values: map[string]any{
			"total_volume": float64(5000),
			"trade_count":  float64(42),
		},
	}

	// Test JSON format.
	queryFormat = "json"
	cmd := queryCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := outputAggregation(cmd, result); err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["trade_count"] != float64(42) {
		t.Errorf("trade_count: got %v, want 42", parsed["trade_count"])
	}

	// Test table format.
	queryFormat = "table"
	buf.Reset()
	cmd.SetOut(&buf)

	if err := outputAggregation(cmd, result); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "total_volume") {
		t.Errorf("table output missing total_volume: %s", output)
	}
}

func TestListTables(t *testing.T) {
	dir := t.TempDir()
	path := writeTestConfig(t, dir)

	// Load config to pass to listTables.
	origCfgPath := cfgPath
	cfgPath = path
	defer func() { cfgPath = origCfgPath }()

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	var buf bytes.Buffer
	cmd := queryCmd
	cmd.SetOut(&buf)

	listTables(cmd, cfg)

	output := buf.String()
	if !strings.Contains(output, "TestContract") {
		t.Error("output should contain contract name")
	}
	if !strings.Contains(output, "Transfer") {
		t.Error("output should contain Transfer event")
	}
	if !strings.Contains(output, "testcontract_transfer") {
		t.Error("output should contain table name")
	}
	if !strings.Contains(output, "type=log") {
		t.Error("output should contain table type")
	}
}

func TestQueryEndToEnd(t *testing.T) {
	// Create an in-memory store with test data.
	st := memory.New()
	ctx := context.Background()

	// Create table schema.
	schema := &types.TableSchema{
		Name:      "testcontract_transfer",
		Contract:  "TestContract",
		Event:     "Transfer",
		TableType: types.TableTypeLog,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "event_name", Type: "string"},
			{Name: "from", Type: "string"},
			{Name: "to", Type: "string"},
			{Name: "amount", Type: "string"},
		},
	}
	if err := st.CreateTable(ctx, schema); err != nil {
		t.Fatal(err)
	}

	// Insert test events.
	ops := []store.Operation{
		{
			Type:        store.OpInsert,
			Table:       "testcontract_transfer",
			Key:         "100:0",
			BlockNumber: 100,
			LogIndex:    0,
			Data: map[string]any{
				"block_number": uint64(100),
				"event_name":   "Transfer",
				"from":         "0xabc",
				"to":           "0xdef",
				"amount":       "1000",
			},
		},
		{
			Type:        store.OpInsert,
			Table:       "testcontract_transfer",
			Key:         "101:0",
			BlockNumber: 101,
			LogIndex:    0,
			Data: map[string]any{
				"block_number": uint64(101),
				"event_name":   "Transfer",
				"from":         "0xdef",
				"to":           "0x123",
				"amount":       "500",
			},
		},
	}
	if err := st.ApplyOperations(ctx, ops); err != nil {
		t.Fatal(err)
	}

	// Query with default settings.
	q := store.Query{
		Limit:    50,
		OrderBy:  "block_number",
		OrderDir: store.OrderDesc,
	}
	events, err := st.GetEvents(ctx, "testcontract_transfer", q)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Verify JSON output.
	var buf bytes.Buffer
	if err := outputJSON(&buf, events); err != nil {
		t.Fatal(err)
	}
	var result []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 JSON entries, got %d", len(result))
	}

	// Verify table output.
	buf.Reset()
	if err := outputTable(&buf, events); err != nil {
		t.Fatal(err)
	}
	tableOut := buf.String()
	if !strings.Contains(tableOut, "0xabc") {
		t.Error("table output should contain event data")
	}

	// Verify CSV output.
	buf.Reset()
	if err := outputCSV(&buf, events); err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	// Header + 2 data rows.
	if len(records) != 3 {
		t.Errorf("expected 3 CSV rows, got %d", len(records))
	}
}

// TestBuildQuery_ContractAddressFlag verifies the --contract-address flag
// adds a contract_address equality filter to the query.
func TestBuildQuery_ContractAddressFlag(t *testing.T) {
	origLimit, origOffset, origOrder, origFilters := queryLimit, queryOffset, queryOrder, queryFilters
	origAddr := queryContractAddress
	defer func() {
		queryLimit, queryOffset, queryOrder, queryFilters = origLimit, origOffset, origOrder, origFilters
		queryContractAddress = origAddr
	}()

	queryLimit = 50
	queryOffset = 0
	queryOrder = "block_number.desc"
	queryFilters = nil
	queryContractAddress = "0xC001"

	q, err := buildQuery()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(q.Filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(q.Filters))
	}
	f := q.Filters[0]
	if f.Field != "contract_address" {
		t.Errorf("expected filter field=contract_address, got %q", f.Field)
	}
	if f.Operator != "eq" {
		t.Errorf("expected filter operator=eq, got %q", f.Operator)
	}
	if f.Value != "0xC001" {
		t.Errorf("expected filter value=0xC001, got %q", f.Value)
	}
}

func TestOutputCount(t *testing.T) {
	cmd := queryCmd

	// JSON format.
	queryFormat = "json"
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := outputCount(cmd, 42); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["count"] != float64(42) {
		t.Errorf("count: got %v, want 42", parsed["count"])
	}

	// Table format (default text).
	queryFormat = "table"
	buf.Reset()
	cmd.SetOut(&buf)
	if err := outputCount(cmd, 7); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Count: 7") {
		t.Errorf("expected 'Count: 7', got %q", buf.String())
	}
}

func TestLatestQuery(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	schema := &types.TableSchema{
		Name:      "testcontract_transfer",
		Contract:  "TestContract",
		Event:     "Transfer",
		TableType: types.TableTypeLog,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "event_name", Type: "string"},
			{Name: "amount", Type: "string"},
		},
	}
	if err := st.CreateTable(ctx, schema); err != nil {
		t.Fatal(err)
	}

	ops := []store.Operation{
		{
			Type: store.OpInsert, Table: "testcontract_transfer",
			Key: "100:0", BlockNumber: 100, LogIndex: 0,
			Data: map[string]any{"block_number": uint64(100), "event_name": "Transfer", "amount": "1000"},
		},
		{
			Type: store.OpInsert, Table: "testcontract_transfer",
			Key: "200:0", BlockNumber: 200, LogIndex: 0,
			Data: map[string]any{"block_number": uint64(200), "event_name": "Transfer", "amount": "500"},
		},
	}
	if err := st.ApplyOperations(ctx, ops); err != nil {
		t.Fatal(err)
	}

	// Use --latest: should return only 1 event (the most recent).
	q := store.Query{Limit: 1, OrderBy: "block_number", OrderDir: store.OrderDesc}
	events, err := st.GetEvents(ctx, "testcontract_transfer", q)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data["block_number"] != uint64(200) {
		t.Errorf("expected block_number 200, got %v", events[0].Data["block_number"])
	}
}

func TestCountQuery(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	schema := &types.TableSchema{
		Name:      "testcontract_transfer",
		Contract:  "TestContract",
		Event:     "Transfer",
		TableType: types.TableTypeLog,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "event_name", Type: "string"},
		},
	}
	if err := st.CreateTable(ctx, schema); err != nil {
		t.Fatal(err)
	}

	ops := []store.Operation{
		{
			Type: store.OpInsert, Table: "testcontract_transfer",
			Key: "100:0", BlockNumber: 100, LogIndex: 0,
			Data: map[string]any{"block_number": uint64(100), "event_name": "Transfer"},
		},
		{
			Type: store.OpInsert, Table: "testcontract_transfer",
			Key: "101:0", BlockNumber: 101, LogIndex: 0,
			Data: map[string]any{"block_number": uint64(101), "event_name": "Transfer"},
		},
		{
			Type: store.OpInsert, Table: "testcontract_transfer",
			Key: "102:0", BlockNumber: 102, LogIndex: 0,
			Data: map[string]any{"block_number": uint64(102), "event_name": "Transfer"},
		},
	}
	if err := st.ApplyOperations(ctx, ops); err != nil {
		t.Fatal(err)
	}

	count, err := st.CountEvents(ctx, "testcontract_transfer", nil)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("expected count=3, got %d", count)
	}
}

func TestFactoryChildrenQuery(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	// Save two dynamic contracts as factory children.
	child1 := &config.ContractConfig{
		Name:        "MyFactory_pair_0x111",
		Address:     "0x111",
		StartBlock:  100,
		FactoryName: "MyFactory",
		FactoryMeta: map[string]any{"token0": "0xAAA", "token1": "0xBBB"},
		Events:      []config.EventConfig{{Name: "Swap"}},
	}
	child2 := &config.ContractConfig{
		Name:        "MyFactory_pair_0x222",
		Address:     "0x222",
		StartBlock:  200,
		FactoryName: "MyFactory",
		FactoryMeta: map[string]any{"token0": "0xCCC", "token1": "0xDDD"},
		Events:      []config.EventConfig{{Name: "Swap"}},
	}
	// A child of a different factory (should not appear).
	other := &config.ContractConfig{
		Name:        "OtherFactory_pair_0x333",
		Address:     "0x333",
		StartBlock:  300,
		FactoryName: "OtherFactory",
		Events:      []config.EventConfig{{Name: "Trade"}},
	}

	for _, cc := range []*config.ContractConfig{child1, child2, other} {
		if err := st.SaveDynamicContract(ctx, cc); err != nil {
			t.Fatal(err)
		}
	}

	// Set cursors.
	_ = st.SetCursor(ctx, child1.Name, 150)
	_ = st.SetCursor(ctx, child2.Name, 210)

	// Get dynamic contracts and filter by factory.
	allContracts, err := st.GetDynamicContracts(ctx)
	if err != nil {
		t.Fatal(err)
	}

	var children []map[string]any
	for i := range allContracts {
		cc := &allContracts[i]
		if cc.FactoryName != "MyFactory" {
			continue
		}
		cursor, _ := st.GetCursor(ctx, cc.Name)
		entry := map[string]any{
			"name":             cc.Name,
			"address":          cc.Address,
			"deployment_block": cc.StartBlock,
			"current_block":    cursor,
			"events":           len(cc.Events),
		}
		for k, v := range cc.FactoryMeta {
			entry[k] = v
		}
		children = append(children, entry)
	}

	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}

	// Verify metadata promoted to top-level.
	found := false
	for _, child := range children {
		if child["address"] == "0x111" {
			if child["token0"] != "0xAAA" {
				t.Errorf("expected token0=0xAAA, got %v", child["token0"])
			}
			found = true
		}
	}
	if !found {
		t.Error("child 0x111 not found")
	}
}

func TestMatchChildFilters(t *testing.T) {
	entry := map[string]any{
		"name":    "pair_0x111",
		"address": "0x111",
		"token0":  "0xAAA",
		"token1":  "0xBBB",
	}

	// Match on token0.
	if !matchChildFilters(entry, []store.Filter{{Field: "token0", Operator: "eq", Value: "0xAAA"}}) {
		t.Error("expected match on token0=0xAAA")
	}

	// No match.
	if matchChildFilters(entry, []store.Filter{{Field: "token0", Operator: "eq", Value: "0xCCC"}}) {
		t.Error("expected no match on token0=0xCCC")
	}

	// neq match.
	if !matchChildFilters(entry, []store.Filter{{Field: "token0", Operator: "neq", Value: "0xCCC"}}) {
		t.Error("expected match on token0 neq 0xCCC")
	}

	// Missing field.
	if matchChildFilters(entry, []store.Filter{{Field: "missing", Operator: "eq", Value: "x"}}) {
		t.Error("expected no match on missing field")
	}
}

func TestCollectMapColumns(t *testing.T) {
	entries := []map[string]any{
		{"name": "a", "address": "0x1", "token0": "0xA"},
		{"name": "b", "address": "0x2", "deployment_block": uint64(100), "token1": "0xB"},
	}

	cols := collectMapColumns(entries)

	// Known columns should come first.
	if cols[0] != "name" {
		t.Errorf("first column: got %q, want 'name'", cols[0])
	}
	if cols[1] != "address" {
		t.Errorf("second column: got %q, want 'address'", cols[1])
	}

	// Extra columns should be sorted after known ones.
	found := false
	for _, c := range cols {
		if c == "token0" {
			found = true
		}
	}
	if !found {
		t.Error("expected token0 in columns")
	}
}

// TestBuildQuery_ContractAddressCombinedWithFilters verifies --contract-address
// works alongside regular --filter flags.
func TestBuildQuery_ContractAddressCombinedWithFilters(t *testing.T) {
	origLimit, origOffset, origOrder, origFilters := queryLimit, queryOffset, queryOrder, queryFilters
	origAddr := queryContractAddress
	defer func() {
		queryLimit, queryOffset, queryOrder, queryFilters = origLimit, origOffset, origOrder, origFilters
		queryContractAddress = origAddr
	}()

	queryLimit = 50
	queryOffset = 0
	queryOrder = "block_number.desc"
	queryFilters = []string{"block_number=gte.100"}
	queryContractAddress = "0xC001"

	q, err := buildQuery()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(q.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(q.Filters))
	}

	// First filter: from --filter flag.
	if q.Filters[0].Field != "block_number" || q.Filters[0].Operator != "gte" {
		t.Errorf("expected block_number gte filter, got %+v", q.Filters[0])
	}
	// Second filter: from --contract-address flag.
	if q.Filters[1].Field != "contract_address" || q.Filters[1].Value != "0xC001" {
		t.Errorf("expected contract_address filter, got %+v", q.Filters[1])
	}
}
