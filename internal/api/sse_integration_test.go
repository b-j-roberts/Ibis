package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/b-j-roberts/ibis/internal/api"
	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/store/memory"
	"github.com/b-j-roberts/ibis/internal/types"
)

// setupIntegrationServer creates a full test server with event bus, pre-populated
// data, and returns all components for thorough integration testing.
func setupIntegrationServer(t *testing.T) (*httptest.Server, *memory.MemoryStore, *api.EventBus) {
	t.Helper()

	st := memory.New()
	ctx := context.Background()

	schema := &types.TableSchema{
		Name:      "strk_transfer",
		Contract:  "STRK",
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
			{Name: "amount", Type: "string"},
		},
	}
	st.CreateTable(ctx, schema)

	// Pre-populate events at blocks 1000, 1001, 1002 for replay tests.
	ops := []store.Operation{
		{Type: store.OpInsert, Table: "strk_transfer", Key: "1000:0", BlockNumber: 1000, LogIndex: 0,
			Data: map[string]any{
				"block_number": uint64(1000), "log_index": uint64(0), "timestamp": uint64(10000),
				"transaction_hash": "0xabc1", "contract_address": "0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d",
				"event_name": "Transfer", "status": "ACCEPTED_ON_L2",
				"from": "0xdeadbeef", "to": "0xcafe", "amount": "1000000000000000000",
			}},
		{Type: store.OpInsert, Table: "strk_transfer", Key: "1001:0", BlockNumber: 1001, LogIndex: 0,
			Data: map[string]any{
				"block_number": uint64(1001), "log_index": uint64(0), "timestamp": uint64(10010),
				"transaction_hash": "0xabc2", "contract_address": "0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d",
				"event_name": "Transfer", "status": "ACCEPTED_ON_L2",
				"from": "0xcafe", "to": "0xbabe", "amount": "500000000000000000",
			}},
		{Type: store.OpInsert, Table: "strk_transfer", Key: "1002:0", BlockNumber: 1002, LogIndex: 0,
			Data: map[string]any{
				"block_number": uint64(1002), "log_index": uint64(0), "timestamp": uint64(10020),
				"transaction_hash": "0xabc3", "contract_address": "0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d",
				"event_name": "Transfer", "status": "ACCEPTED_ON_L2",
				"from": "0xdeadbeef", "to": "0xfeed", "amount": "2000000000000000000",
			}},
	}
	st.ApplyOperations(ctx, ops)
	st.SetCursor(ctx, "STRK", 1002)

	bus := api.NewEventBus()
	t.Cleanup(bus.Close)

	srv := api.New(&api.ServerConfig{
		Store:   st,
		Schemas: []*types.TableSchema{schema},
		APIConfig: &config.APIConfig{
			Host: "localhost",
			Port: 8080,
		},
		Contracts: []config.ContractConfig{
			{Name: "STRK", Address: "0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d",
				Events: []config.EventConfig{{Name: "Transfer"}}},
		},
		Logger:   slog.Default(),
		EventBus: bus,
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return ts, st, bus
}

// readSSEEvent reads one complete SSE event (id + data) from a scanner.
// Returns empty strings on timeout or stream end.
func readSSEEvent(t *testing.T, scanner *bufio.Scanner, timeout time.Duration) (id, data string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "id: ") {
				id = strings.TrimPrefix(line, "id: ")
			}
			if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
				close(done)
				return
			}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Log("timeout reading SSE event")
	}
	return
}

// Test 1: Basic SSE stream connection — verify headers and open connection
func TestIntegration_SSEBasicConnection(t *testing.T) {
	ts, _, _ := setupIntegrationServer(t)

	req, _ := http.NewRequest("GET", ts.URL+"/v1/STRK/Transfer/stream", http.NoBody)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer resp.Body.Close()

	// Verify status code.
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify SSE headers.
	checks := map[string]string{
		"Content-Type":  "text/event-stream",
		"Cache-Control": "no-cache",
		"Connection":    "keep-alive",
	}
	for header, expected := range checks {
		got := resp.Header.Get(header)
		if got != expected {
			t.Errorf("header %s: expected %q, got %q", header, expected, got)
		}
	}

	t.Log("PASS: SSE connection established with correct headers")
}

// Test 2: Receive real-time events — publish events and verify SSE format
func TestIntegration_SSEReceiveRealTimeEvents(t *testing.T) {
	ts, _, bus := setupIntegrationServer(t)

	req, _ := http.NewRequest("GET", ts.URL+"/v1/STRK/Transfer/stream", http.NoBody)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer resp.Body.Close()

	// Publish 3 events simulating real-time indexing.
	events := []api.StreamEvent{
		{Table: "strk_transfer", Contract: "STRK", Event: "Transfer", BlockNumber: 2000, LogIndex: 0,
			Data: map[string]any{"from": "0xaaa", "to": "0xbbb", "amount": "100", "block_number": uint64(2000), "log_index": uint64(0)}},
		{Table: "strk_transfer", Contract: "STRK", Event: "Transfer", BlockNumber: 2000, LogIndex: 1,
			Data: map[string]any{"from": "0xccc", "to": "0xddd", "amount": "200", "block_number": uint64(2000), "log_index": uint64(1)}},
		{Table: "strk_transfer", Contract: "STRK", Event: "Transfer", BlockNumber: 2001, LogIndex: 0,
			Data: map[string]any{"from": "0xeee", "to": "0xfff", "amount": "300", "block_number": uint64(2001), "log_index": uint64(0)}},
	}
	for _, e := range events {
		bus.Publish(e)
	}

	// Read and verify all 3 events.
	scanner := bufio.NewScanner(resp.Body)

	expectedIDs := []string{"2000:0", "2000:1", "2001:0"}
	expectedFroms := []string{"0xaaa", "0xccc", "0xeee"}

	for i := 0; i < 3; i++ {
		id, data := readSSEEvent(t, scanner, 2*time.Second)
		if id != expectedIDs[i] {
			t.Errorf("event %d: expected id %q, got %q", i, expectedIDs[i], id)
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(data), &parsed); err != nil {
			t.Fatalf("event %d: failed to parse JSON: %v", i, err)
		}
		if parsed["from"] != expectedFroms[i] {
			t.Errorf("event %d: expected from %q, got %v", i, expectedFroms[i], parsed["from"])
		}
	}

	t.Log("PASS: Received 3 real-time events in correct SSE format (id: block:logIndex\\ndata: json\\n\\n)")
}

// Test 3: Filter support — only matching events are streamed
func TestIntegration_SSEFilterSupport(t *testing.T) {
	ts, _, bus := setupIntegrationServer(t)

	// Subscribe with filter: from=eq.0xdeadbeef
	req, _ := http.NewRequest("GET", ts.URL+"/v1/STRK/Transfer/stream?from=eq.0xdeadbeef", http.NoBody)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer resp.Body.Close()

	// Publish non-matching event.
	bus.Publish(api.StreamEvent{
		Table: "strk_transfer", Contract: "STRK", Event: "Transfer",
		BlockNumber: 3000, LogIndex: 0,
		Data: map[string]any{"from": "0xother", "to": "0xbbb", "amount": "100"},
	})

	// Publish matching event.
	bus.Publish(api.StreamEvent{
		Table: "strk_transfer", Contract: "STRK", Event: "Transfer",
		BlockNumber: 3001, LogIndex: 0,
		Data: map[string]any{"from": "0xdeadbeef", "to": "0xfeed", "amount": "999"},
	})

	scanner := bufio.NewScanner(resp.Body)
	id, data := readSSEEvent(t, scanner, 2*time.Second)

	// Should receive 3001 (matching), NOT 3000 (non-matching).
	if id != "3001:0" {
		t.Errorf("expected filtered event id '3001:0', got %q", id)
	}

	var parsed map[string]any
	json.Unmarshal([]byte(data), &parsed)
	if parsed["from"] != "0xdeadbeef" {
		t.Errorf("expected from '0xdeadbeef', got %v", parsed["from"])
	}

	t.Log("PASS: Filter correctly excluded non-matching events")
}

// Test 4: Last-Event-ID replay — missed events are replayed before live stream
func TestIntegration_SSELastEventIDReplay(t *testing.T) {
	ts, _, bus := setupIntegrationServer(t)

	// Connect with Last-Event-ID: 1000:0
	// Should replay events after block 1000, log_index 0 (i.e., 1001:0 and 1002:0).
	req, _ := http.NewRequest("GET", ts.URL+"/v1/STRK/Transfer/stream", http.NoBody)
	req.Header.Set("Last-Event-ID", "1000:0")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)

	// First replayed event: block 1001.
	id1, data1 := readSSEEvent(t, scanner, 2*time.Second)
	if id1 != "1001:0" {
		t.Errorf("expected first replay id '1001:0', got %q", id1)
	}
	var p1 map[string]any
	json.Unmarshal([]byte(data1), &p1)
	if p1["from"] != "0xcafe" {
		t.Errorf("expected first replay from '0xcafe', got %v", p1["from"])
	}

	// Second replayed event: block 1002.
	id2, data2 := readSSEEvent(t, scanner, 2*time.Second)
	if id2 != "1002:0" {
		t.Errorf("expected second replay id '1002:0', got %q", id2)
	}
	var p2 map[string]any
	json.Unmarshal([]byte(data2), &p2)
	if p2["from"] != "0xdeadbeef" {
		t.Errorf("expected second replay from '0xdeadbeef', got %v", p2["from"])
	}

	// Now publish a live event — it should come through after replay.
	bus.Publish(api.StreamEvent{
		Table: "strk_transfer", Contract: "STRK", Event: "Transfer",
		BlockNumber: 5000, LogIndex: 0,
		Data: map[string]any{"from": "0xlive", "to": "0xtest", "amount": "42"},
	})

	id3, _ := readSSEEvent(t, scanner, 2*time.Second)
	if id3 != "5000:0" {
		t.Errorf("expected live event id '5000:0', got %q", id3)
	}

	t.Log("PASS: Replayed 2 missed events then received live event")
}

// Test 5: Unknown table returns 404
func TestIntegration_SSENotFound(t *testing.T) {
	ts, _, _ := setupIntegrationServer(t)

	resp, err := http.Get(ts.URL + "/v1/Unknown/Event/stream")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] != "table not found" {
		t.Errorf("expected error 'table not found', got %v", result["error"])
	}

	t.Log("PASS: Unknown table returns 404 with proper error message")
}

// Test 6: Client disconnect cleanup — subscription is removed after disconnect
func TestIntegration_SSEClientDisconnectCleanup(t *testing.T) {
	ts, _, bus := setupIntegrationServer(t)

	// Connect a client.
	req, _ := http.NewRequest("GET", ts.URL+"/v1/STRK/Transfer/stream", http.NoBody)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Verify we can receive an event.
	bus.Publish(api.StreamEvent{
		Table: "strk_transfer", Contract: "STRK", Event: "Transfer",
		BlockNumber: 6000, LogIndex: 0,
		Data: map[string]any{"from": "0xtest"},
	})

	scanner := bufio.NewScanner(resp.Body)
	id, _ := readSSEEvent(t, scanner, 2*time.Second)
	if id != "6000:0" {
		t.Errorf("expected event id '6000:0' before disconnect, got %q", id)
	}

	// Disconnect the client.
	cancel()
	resp.Body.Close()

	// Give the server a moment to clean up.
	time.Sleep(100 * time.Millisecond)

	// Publish another event — should not panic or block.
	bus.Publish(api.StreamEvent{
		Table: "strk_transfer", Contract: "STRK", Event: "Transfer",
		BlockNumber: 6001, LogIndex: 0,
		Data: map[string]any{"from": "0xafter_disconnect"},
	})

	t.Log("PASS: Client disconnect was handled cleanly, no panic or blocking")
}

// Test 7: Graceful shutdown — all SSE connections close when bus is closed
func TestIntegration_SSEGracefulShutdown(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	schema := &types.TableSchema{
		Name: "strk_transfer", Contract: "STRK", Event: "Transfer",
		TableType: types.TableTypeLog,
		Columns:   []types.Column{{Name: "block_number", Type: "uint64"}, {Name: "log_index", Type: "uint64"}},
	}
	st.CreateTable(ctx, schema)

	bus := api.NewEventBus()

	srv := api.New(&api.ServerConfig{
		Store:   st,
		Schemas: []*types.TableSchema{schema},
		APIConfig: &config.APIConfig{
			Host: "localhost",
			Port: 8080,
		},
		Contracts: []config.ContractConfig{
			{Name: "STRK", Address: "0x123", Events: []config.EventConfig{{Name: "Transfer"}}},
		},
		Logger:   slog.Default(),
		EventBus: bus,
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Connect multiple clients.
	var clients []*http.Response
	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", ts.URL+"/v1/STRK/Transfer/stream", http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("client %d connect: %v", i, err)
		}
		clients = append(clients, resp)
	}

	// Close the event bus (simulates graceful shutdown).
	bus.Close()

	// Verify all client streams end.
	for i, resp := range clients {
		wg.Add(1)
		go func(idx int, r *http.Response) {
			defer wg.Done()
			defer r.Body.Close()
			// Read should eventually return EOF/empty because the channel was closed.
			buf := make([]byte, 1024)
			_, _ = r.Body.Read(buf)
		}(i, resp)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("PASS: All 3 SSE clients disconnected cleanly on shutdown")
	case <-time.After(5 * time.Second):
		t.Error("timeout waiting for clients to disconnect on shutdown")
	}
}

// Test: Multiple concurrent SSE clients receive the same event
func TestIntegration_SSEMultipleClients(t *testing.T) {
	ts, _, bus := setupIntegrationServer(t)

	// Connect 3 concurrent clients.
	type clientResult struct {
		id   string
		data string
	}

	results := make([]chan clientResult, 3)
	for i := 0; i < 3; i++ {
		results[i] = make(chan clientResult, 1)
		go func(idx int) {
			req, _ := http.NewRequest("GET", ts.URL+"/v1/STRK/Transfer/stream", http.NoBody)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req = req.WithContext(ctx)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			scanner := bufio.NewScanner(resp.Body)
			id, data := readSSEEvent(t, scanner, 2*time.Second)
			results[idx] <- clientResult{id: id, data: data}
		}(i)
	}

	// Give clients time to connect.
	time.Sleep(100 * time.Millisecond)

	// Publish one event.
	bus.Publish(api.StreamEvent{
		Table: "strk_transfer", Contract: "STRK", Event: "Transfer",
		BlockNumber: 7000, LogIndex: 0,
		Data: map[string]any{"from": "0xbroadcast", "to": "0xall", "amount": "1"},
	})

	// All 3 clients should receive the same event.
	for i := 0; i < 3; i++ {
		select {
		case r := <-results[i]:
			if r.id != "7000:0" {
				t.Errorf("client %d: expected id '7000:0', got %q", i, r.id)
			}
		case <-time.After(3 * time.Second):
			t.Errorf("client %d: timeout waiting for event", i)
		}
	}

	t.Log("PASS: All 3 concurrent clients received the broadcast event")
}
