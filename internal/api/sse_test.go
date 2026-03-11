package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/b-j-roberts/ibis/internal/api"
	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/store/memory"
	"github.com/b-j-roberts/ibis/internal/types"
)

// setupSSEServer creates an API server with an EventBus for SSE testing.
func setupSSEServer(t *testing.T) (*httptest.Server, *memory.MemoryStore, *api.EventBus) {
	t.Helper()

	st := memory.New()
	ctx := context.Background()

	schema := &types.TableSchema{
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
	st.CreateTable(ctx, schema)

	// Pre-populate some events for replay tests.
	ops := []store.Operation{
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
	}
	st.ApplyOperations(ctx, ops)

	bus := api.NewEventBus()
	t.Cleanup(bus.Close)

	srv := api.New(api.ServerConfig{
		Store:   st,
		Schemas: []*types.TableSchema{schema},
		APIConfig: &config.APIConfig{
			Host: "localhost",
			Port: 8080,
		},
		Contracts: []config.ContractConfig{
			{Name: "MyToken", Address: "0x123", Events: []config.EventConfig{
				{Name: "Transfer"},
			}},
		},
		Logger:   slog.Default(),
		EventBus: bus,
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return ts, st, bus
}

func TestSSEStream(t *testing.T) {
	ts, _, bus := setupSSEServer(t)

	// Connect to SSE stream.
	req, _ := http.NewRequest("GET", ts.URL+"/v1/MyToken/Transfer/stream", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %q", cc)
	}

	// Publish an event.
	bus.Publish(api.StreamEvent{
		Table:       "mytoken_transfer",
		Contract:    "MyToken",
		Event:       "Transfer",
		BlockNumber: 200,
		LogIndex:    0,
		Data: map[string]any{
			"block_number": uint64(200),
			"log_index":    uint64(0),
			"from":         "0xalice",
			"to":           "0xbob",
			"amount":       int64(1000),
		},
	})

	// Read the SSE event.
	scanner := bufio.NewScanner(resp.Body)
	var id, data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			id = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if id != "200:0" {
		t.Errorf("expected id '200:0', got %q", id)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		t.Fatalf("failed to parse SSE data: %v", err)
	}
	if parsed["from"] != "0xalice" {
		t.Errorf("expected from '0xalice', got %v", parsed["from"])
	}
}

func TestSSEStreamNotFound(t *testing.T) {
	ts, _, _ := setupSSEServer(t)

	resp, err := http.Get(ts.URL + "/v1/Unknown/Event/stream")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestSSEStreamWithFilter(t *testing.T) {
	ts, _, bus := setupSSEServer(t)

	// Connect with filter: only from=0xalice.
	req, _ := http.NewRequest("GET", ts.URL+"/v1/MyToken/Transfer/stream?from=eq.0xalice", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	defer resp.Body.Close()

	// Publish an event that does NOT match filter (from=0xbob).
	bus.Publish(api.StreamEvent{
		Table:       "mytoken_transfer",
		Contract:    "MyToken",
		Event:       "Transfer",
		BlockNumber: 300,
		LogIndex:    0,
		Data:        map[string]any{"from": "0xbob", "to": "0xcharlie", "amount": int64(100)},
	})

	// Publish an event that DOES match filter (from=0xalice).
	bus.Publish(api.StreamEvent{
		Table:       "mytoken_transfer",
		Contract:    "MyToken",
		Event:       "Transfer",
		BlockNumber: 301,
		LogIndex:    0,
		Data:        map[string]any{"from": "0xalice", "to": "0xdave", "amount": int64(999)},
	})

	// The first event we receive should be the alice event (301), not bob (300).
	scanner := bufio.NewScanner(resp.Body)
	var id string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			id = strings.TrimPrefix(line, "id: ")
			break
		}
	}

	if id != "301:0" {
		t.Errorf("expected filtered event id '301:0', got %q", id)
	}
}

func TestSSELastEventIDReplay(t *testing.T) {
	ts, _, bus := setupSSEServer(t)

	// Connect with Last-Event-ID: 100:0 (should replay block 101 event).
	req, _ := http.NewRequest("GET", ts.URL+"/v1/MyToken/Transfer/stream", nil)
	req.Header.Set("Last-Event-ID", "100:0")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	defer resp.Body.Close()

	// Read the replayed event (should be block 101).
	scanner := bufio.NewScanner(resp.Body)
	var id, data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			id = strings.TrimPrefix(line, "id: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			break
		}
	}

	if id != "101:0" {
		t.Errorf("expected replayed event id '101:0', got %q", id)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		t.Fatalf("failed to parse replayed data: %v", err)
	}

	// After replay, live events should also come through.
	bus.Publish(api.StreamEvent{
		Table:       "mytoken_transfer",
		Contract:    "MyToken",
		Event:       "Transfer",
		BlockNumber: 200,
		LogIndex:    0,
		Data:        map[string]any{"from": "0xlive", "to": "0xtest", "amount": int64(42)},
	})

	var liveID string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id: ") {
			liveID = strings.TrimPrefix(line, "id: ")
			break
		}
	}

	if liveID != "200:0" {
		t.Errorf("expected live event id '200:0', got %q", liveID)
	}
}

func TestEventBusPublishSubscribe(t *testing.T) {
	bus := api.NewEventBus()
	defer bus.Close()

	id, ch := bus.Subscribe("test_table", nil)
	defer bus.Unsubscribe(id)

	evt := api.StreamEvent{
		Table:       "test_table",
		BlockNumber: 42,
		LogIndex:    0,
		Data:        map[string]any{"key": "value"},
	}
	bus.Publish(evt)

	select {
	case received := <-ch:
		if received.BlockNumber != 42 {
			t.Errorf("expected block 42, got %d", received.BlockNumber)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestEventBusTableFilter(t *testing.T) {
	bus := api.NewEventBus()
	defer bus.Close()

	id, ch := bus.Subscribe("table_a", nil)
	defer bus.Unsubscribe(id)

	// Publish to a different table.
	bus.Publish(api.StreamEvent{Table: "table_b", BlockNumber: 1, Data: map[string]any{}})

	// Publish to the subscribed table.
	bus.Publish(api.StreamEvent{Table: "table_a", BlockNumber: 2, Data: map[string]any{}})

	select {
	case received := <-ch:
		if received.BlockNumber != 2 {
			t.Errorf("expected block 2, got %d", received.BlockNumber)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for filtered event")
	}
}

func TestEventBusFieldFilter(t *testing.T) {
	bus := api.NewEventBus()
	defer bus.Close()

	filters := []store.Filter{
		{Field: "color", Operator: "eq", Value: "red"},
	}
	id, ch := bus.Subscribe("", filters)
	defer bus.Unsubscribe(id)

	// Non-matching.
	bus.Publish(api.StreamEvent{Table: "t", BlockNumber: 1, Data: map[string]any{"color": "blue"}})
	// Matching.
	bus.Publish(api.StreamEvent{Table: "t", BlockNumber: 2, Data: map[string]any{"color": "red"}})

	select {
	case received := <-ch:
		if received.BlockNumber != 2 {
			t.Errorf("expected block 2, got %d", received.BlockNumber)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for filtered event")
	}
}

func TestEventBusClose(t *testing.T) {
	bus := api.NewEventBus()

	_, ch := bus.Subscribe("test", nil)
	bus.Close()

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after bus.Close()")
	}

	// Publishing after close should not panic.
	bus.Publish(api.StreamEvent{Table: "test", Data: map[string]any{}})
}

func TestEventBusUnsubscribe(t *testing.T) {
	bus := api.NewEventBus()
	defer bus.Close()

	id, ch := bus.Subscribe("test", nil)
	bus.Unsubscribe(id)

	// Channel should be closed.
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after Unsubscribe")
	}
}

func TestStreamEventID(t *testing.T) {
	evt := api.StreamEvent{BlockNumber: 12345, LogIndex: 7}
	if got := evt.EventID(); got != "12345:7" {
		t.Errorf("expected '12345:7', got %q", got)
	}
}

func TestSSENoEventBus(t *testing.T) {
	st := memory.New()

	schema := &types.TableSchema{
		Name:      "mytoken_transfer",
		Contract:  "MyToken",
		Event:     "Transfer",
		TableType: types.TableTypeLog,
	}
	st.CreateTable(context.Background(), schema)

	// Create server WITHOUT EventBus.
	srv := api.New(api.ServerConfig{
		Store:   st,
		Schemas: []*types.TableSchema{schema},
		APIConfig: &config.APIConfig{
			Host: "localhost",
			Port: 8080,
		},
		Contracts: []config.ContractConfig{
			{Name: "MyToken", Address: "0x123"},
		},
		Logger: slog.Default(),
		// EventBus: nil -- intentionally omitted
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/MyToken/Transfer/stream")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when no event bus, got %d", resp.StatusCode)
	}
}
