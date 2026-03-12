package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/b-j-roberts/ibis/internal/api"
	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/store/memory"
	"github.com/b-j-roberts/ibis/internal/types"
)

// setupTestServer creates an API server backed by an in-memory store with
// pre-populated test data.
func setupTestServer(t *testing.T) (*httptest.Server, *memory.MemoryStore) {
	t.Helper()

	st := memory.New()
	ctx := context.Background()

	// Create log table schema.
	logSchema := &types.TableSchema{
		Name:      "mytoken_transfer",
		Contract:  "MyToken",
		Event:     "Transfer",
		TableType: types.TableTypeLog,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "transaction_hash", Type: "string"},
			{Name: "log_index", Type: "uint64"},
			{Name: "timestamp", Type: "uint64"},
			{Name: "contract_address", Type: "string"},
			{Name: "event_name", Type: "string"},
			{Name: "status", Type: "string"},
			{Name: "from", Type: "string"},
			{Name: "to", Type: "string"},
			{Name: "amount", Type: "int64"},
		},
	}
	st.CreateTable(ctx, logSchema)

	// Create unique table schema.
	uniqueSchema := &types.TableSchema{
		Name:      "mytoken_balance",
		Contract:  "MyToken",
		Event:     "Balance",
		TableType: types.TableTypeUnique,
		UniqueKey: "account",
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "transaction_hash", Type: "string"},
			{Name: "log_index", Type: "uint64"},
			{Name: "timestamp", Type: "uint64"},
			{Name: "contract_address", Type: "string"},
			{Name: "event_name", Type: "string"},
			{Name: "status", Type: "string"},
			{Name: "account", Type: "string"},
			{Name: "balance", Type: "int64"},
		},
	}
	st.CreateTable(ctx, uniqueSchema)

	// Create aggregation table schema.
	aggSchema := &types.TableSchema{
		Name:      "mytoken_volume",
		Contract:  "MyToken",
		Event:     "Volume",
		TableType: types.TableTypeAggregation,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "transaction_hash", Type: "string"},
			{Name: "log_index", Type: "uint64"},
			{Name: "timestamp", Type: "uint64"},
			{Name: "contract_address", Type: "string"},
			{Name: "event_name", Type: "string"},
			{Name: "status", Type: "string"},
			{Name: "amount", Type: "int64"},
		},
		Aggregates: []types.AggregateSpec{
			{Column: "total_volume", Operation: "sum", Field: "amount"},
			{Column: "trade_count", Operation: "count", Field: "amount"},
		},
	}
	st.CreateTable(ctx, aggSchema)

	// Insert test events into log table.
	transfers := []store.Operation{
		{Type: store.OpInsert, Table: "mytoken_transfer", Key: "100:0", BlockNumber: 100, LogIndex: 0,
			Data: map[string]any{
				"block_number": uint64(100), "log_index": uint64(0), "timestamp": uint64(1000),
				"transaction_hash": "0xaaa", "contract_address": "0x123", "event_name": "Transfer",
				"status": "ACCEPTED_L2", "from": "0xalice", "to": "0xbob", "amount": int64(500),
			}},
		{Type: store.OpInsert, Table: "mytoken_transfer", Key: "101:0", BlockNumber: 101, LogIndex: 0,
			Data: map[string]any{
				"block_number": uint64(101), "log_index": uint64(0), "timestamp": uint64(1010),
				"transaction_hash": "0xbbb", "contract_address": "0x123", "event_name": "Transfer",
				"status": "ACCEPTED_L2", "from": "0xbob", "to": "0xcharlie", "amount": int64(200),
			}},
		{Type: store.OpInsert, Table: "mytoken_transfer", Key: "102:0", BlockNumber: 102, LogIndex: 0,
			Data: map[string]any{
				"block_number": uint64(102), "log_index": uint64(0), "timestamp": uint64(1020),
				"transaction_hash": "0xccc", "contract_address": "0x123", "event_name": "Transfer",
				"status": "ACCEPTED_L2", "from": "0xalice", "to": "0xdave", "amount": int64(300),
			}},
	}
	st.ApplyOperations(ctx, transfers)

	// Insert unique events.
	balances := []store.Operation{
		{Type: store.OpInsert, Table: "mytoken_balance", Key: "100:1", BlockNumber: 100, LogIndex: 1,
			Data: map[string]any{
				"block_number": uint64(100), "log_index": uint64(1), "timestamp": uint64(1000),
				"transaction_hash": "0xaaa", "contract_address": "0x123", "event_name": "Balance",
				"status": "ACCEPTED_L2", "account": "0xalice", "balance": int64(9500),
			}},
		{Type: store.OpInsert, Table: "mytoken_balance", Key: "101:1", BlockNumber: 101, LogIndex: 1,
			Data: map[string]any{
				"block_number": uint64(101), "log_index": uint64(1), "timestamp": uint64(1010),
				"transaction_hash": "0xbbb", "contract_address": "0x123", "event_name": "Balance",
				"status": "ACCEPTED_L2", "account": "0xbob", "balance": int64(300),
			}},
	}
	st.ApplyOperations(ctx, balances)

	// Insert aggregation events.
	volumes := []store.Operation{
		{Type: store.OpInsert, Table: "mytoken_volume", Key: "100:2", BlockNumber: 100, LogIndex: 2,
			Data: map[string]any{
				"block_number": uint64(100), "log_index": uint64(2), "timestamp": uint64(1000),
				"transaction_hash": "0xaaa", "contract_address": "0x123", "event_name": "Volume",
				"status": "ACCEPTED_L2", "amount": int64(500),
			}},
		{Type: store.OpInsert, Table: "mytoken_volume", Key: "101:2", BlockNumber: 101, LogIndex: 2,
			Data: map[string]any{
				"block_number": uint64(101), "log_index": uint64(2), "timestamp": uint64(1010),
				"transaction_hash": "0xbbb", "contract_address": "0x123", "event_name": "Volume",
				"status": "ACCEPTED_L2", "amount": int64(200),
			}},
	}
	st.ApplyOperations(ctx, volumes)

	// Set cursor.
	st.SetCursor(ctx, "MyToken", 102)

	// Build API server.
	srv := api.New(&api.ServerConfig{
		Store:   st,
		Schemas: []*types.TableSchema{logSchema, uniqueSchema, aggSchema},
		APIConfig: &config.APIConfig{
			Host: "localhost",
			Port: 8080,
		},
		Contracts: []config.ContractConfig{
			{Name: "MyToken", Address: "0x123", Events: []config.EventConfig{
				{Name: "Transfer"}, {Name: "Balance"}, {Name: "Volume"},
			}},
		},
		Logger: slog.Default(),
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return ts, st
}

func getJSON(t *testing.T, ts *httptest.Server, path string) map[string]any {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decoding response for %s: %v", path, err)
	}
	return result
}

func getStatus(t *testing.T, ts *httptest.Server, path string) int {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// ---- Tests ----

func TestHealthEndpoint(t *testing.T) {
	ts, _ := setupTestServer(t)
	result := getJSON(t, ts, "/v1/health")

	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %v", result["status"])
	}
}

func TestStatusEndpoint(t *testing.T) {
	ts, _ := setupTestServer(t)
	result := getJSON(t, ts, "/v1/status")

	// current_block should be 102 (our cursor).
	block := result["current_block"].(float64)
	if block != 102 {
		t.Errorf("expected current_block 102, got %v", block)
	}

	contracts := result["contracts"].([]any)
	if len(contracts) != 1 {
		t.Errorf("expected 1 contract, got %d", len(contracts))
	}
}

func TestListEvents(t *testing.T) {
	ts, _ := setupTestServer(t)
	result := getJSON(t, ts, "/v1/MyToken/Transfer")

	data := result["data"].([]any)
	if len(data) != 3 {
		t.Errorf("expected 3 transfers, got %d", len(data))
	}

	count := int(result["count"].(float64))
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	limit := int(result["limit"].(float64))
	if limit != 50 {
		t.Errorf("expected default limit 50, got %d", limit)
	}
}

func TestListEventsWithPagination(t *testing.T) {
	ts, _ := setupTestServer(t)
	result := getJSON(t, ts, "/v1/MyToken/Transfer?limit=2&offset=0")

	data := result["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 results with limit=2, got %d", len(data))
	}

	limit := int(result["limit"].(float64))
	if limit != 2 {
		t.Errorf("expected limit 2, got %d", limit)
	}

	// Get next page.
	result2 := getJSON(t, ts, "/v1/MyToken/Transfer?limit=2&offset=2")
	data2 := result2["data"].([]any)
	if len(data2) != 1 {
		t.Errorf("expected 1 result on second page, got %d", len(data2))
	}
}

func TestListEventsWithOrdering(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Default is desc.
	result := getJSON(t, ts, "/v1/MyToken/Transfer?order=block_number.desc")
	data := result["data"].([]any)
	if len(data) != 3 {
		t.Fatalf("expected 3 results, got %d", len(data))
	}

	first := data[0].(map[string]any)
	if first["block_number"].(float64) != 102 {
		t.Errorf("expected first block 102 (desc), got %v", first["block_number"])
	}

	// Ascending.
	result2 := getJSON(t, ts, "/v1/MyToken/Transfer?order=block_number.asc")
	data2 := result2["data"].([]any)
	first2 := data2[0].(map[string]any)
	if first2["block_number"].(float64) != 100 {
		t.Errorf("expected first block 100 (asc), got %v", first2["block_number"])
	}
}

func TestListEventsWithFilter(t *testing.T) {
	ts, _ := setupTestServer(t)
	result := getJSON(t, ts, "/v1/MyToken/Transfer?from=eq.0xalice")

	data := result["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 transfers from alice, got %d", len(data))
	}
}

func TestGetLatest(t *testing.T) {
	ts, _ := setupTestServer(t)
	result := getJSON(t, ts, "/v1/MyToken/Transfer/latest")

	eventData := result["data"].(map[string]any)
	if eventData["block_number"].(float64) != 102 {
		t.Errorf("expected latest block 102, got %v", eventData["block_number"])
	}
}

func TestGetCount(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Total count.
	result := getJSON(t, ts, "/v1/MyToken/Transfer/count")
	count := int(result["count"].(float64))
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	// Filtered count.
	result2 := getJSON(t, ts, "/v1/MyToken/Transfer/count?from=eq.0xalice")
	count2 := int(result2["count"].(float64))
	if count2 != 2 {
		t.Errorf("expected filtered count 2, got %d", count2)
	}
}

func TestGetUnique(t *testing.T) {
	ts, _ := setupTestServer(t)
	result := getJSON(t, ts, "/v1/MyToken/Balance/unique")

	data := result["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 unique balances, got %d", len(data))
	}
}

func TestGetAggregate(t *testing.T) {
	ts, _ := setupTestServer(t)
	result := getJSON(t, ts, "/v1/MyToken/Volume/aggregate")

	data := result["data"].(map[string]any)
	totalVolume := data["total_volume"].(float64)
	if totalVolume != 700 {
		t.Errorf("expected total_volume 700, got %v", totalVolume)
	}

	tradeCount := data["trade_count"].(float64)
	if tradeCount != 2 {
		t.Errorf("expected trade_count 2, got %v", tradeCount)
	}
}

func TestNotFound(t *testing.T) {
	ts, _ := setupTestServer(t)

	status := getStatus(t, ts, "/v1/Unknown/Event")
	if status != http.StatusNotFound {
		t.Errorf("expected 404 for unknown table, got %d", status)
	}
}

func TestInvalidFilter(t *testing.T) {
	ts, _ := setupTestServer(t)

	status := getStatus(t, ts, "/v1/MyToken/Transfer?from=badformat")
	if status != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid filter, got %d", status)
	}
}

func TestCaseInsensitiveLookup(t *testing.T) {
	ts, _ := setupTestServer(t)

	// Lowercase contract/event should still work.
	result := getJSON(t, ts, "/v1/mytoken/transfer")
	data := result["data"].([]any)
	if len(data) != 3 {
		t.Errorf("expected 3 transfers with lowercase path, got %d", len(data))
	}
}

func TestCORSHeaders(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	resp.Body.Close()

	cors := resp.Header.Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("expected CORS origin *, got %q", cors)
	}

	methods := resp.Header.Get("Access-Control-Allow-Methods")
	if methods != "GET, POST, PUT, DELETE, OPTIONS" {
		t.Errorf("expected methods 'GET, POST, PUT, DELETE, OPTIONS', got %q", methods)
	}
}

func TestContentType(t *testing.T) {
	ts, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health: %v", err)
	}
	resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
}

func TestLimitCap(t *testing.T) {
	ts, _ := setupTestServer(t)

	// limit=9999 should be capped to 500.
	result := getJSON(t, ts, "/v1/MyToken/Transfer?limit=9999")
	limit := int(result["limit"].(float64))
	if limit != 500 {
		t.Errorf("expected limit capped to 500, got %d", limit)
	}
}
