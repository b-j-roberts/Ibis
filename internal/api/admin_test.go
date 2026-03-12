package api_test

import (
	"bytes"
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

func setupAdminTestServer(t *testing.T, adminKey string) (*httptest.Server, *memory.MemoryStore) {
	t.Helper()

	st := memory.New()
	ctx := context.Background()

	logSchema := &types.TableSchema{
		Name:      "mytoken_transfer",
		Contract:  "MyToken",
		Event:     "Transfer",
		TableType: types.TableTypeLog,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "log_index", Type: "uint64"},
		},
	}
	st.CreateTable(ctx, logSchema)

	server := api.New(&api.ServerConfig{
		Store:   st,
		Schemas: []*types.TableSchema{logSchema},
		APIConfig: &config.APIConfig{
			Host:     "0.0.0.0",
			Port:     8080,
			AdminKey: adminKey,
		},
		Contracts: []config.ContractConfig{
			{Name: "MyToken", Address: "0x123"},
		},
		Logger: slog.Default(),
	})

	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	return ts, st
}

func TestAdminAuth_NoKey(t *testing.T) {
	// When no admin key is configured, all requests should be allowed.
	ts, _ := setupAdminTestServer(t, "")

	resp, err := http.Get(ts.URL + "/v1/admin/contracts")
	if err != nil {
		t.Fatalf("GET /v1/admin/contracts: %v", err)
	}
	defer resp.Body.Close()

	// Should return 503 since engine is nil, not 401.
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_WithKey_Missing(t *testing.T) {
	ts, _ := setupAdminTestServer(t, "secret-key")

	resp, err := http.Get(ts.URL + "/v1/admin/contracts")
	if err != nil {
		t.Fatalf("GET /v1/admin/contracts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_WithKey_Wrong(t *testing.T) {
	ts, _ := setupAdminTestServer(t, "secret-key")

	req, _ := http.NewRequest("GET", ts.URL+"/v1/admin/contracts", http.NoBody)
	req.Header.Set("X-Admin-Key", "wrong-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/admin/contracts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestAdminAuth_WithKey_Correct(t *testing.T) {
	ts, _ := setupAdminTestServer(t, "secret-key")

	req, _ := http.NewRequest("GET", ts.URL+"/v1/admin/contracts", http.NoBody)
	req.Header.Set("X-Admin-Key", "secret-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /v1/admin/contracts: %v", err)
	}
	defer resp.Body.Close()

	// 503 because engine is nil, but auth passed.
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no engine), got %d", resp.StatusCode)
	}
}

func TestAdminRegister_NoEngine(t *testing.T) {
	ts, _ := setupAdminTestServer(t, "")

	body := `{"name":"NewContract","address":"0x456"}`
	resp, err := http.Post(ts.URL+"/v1/admin/contracts", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /v1/admin/contracts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestAdminRegister_BadJSON(t *testing.T) {
	ts, _ := setupAdminTestServer(t, "")

	// No engine = 503 before parsing body.
	resp, err := http.Post(ts.URL+"/v1/admin/contracts", "application/json", bytes.NewBufferString("not json"))
	if err != nil {
		t.Fatalf("POST /v1/admin/contracts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no engine), got %d", resp.StatusCode)
	}
}

func TestAdminRegister_MissingName(t *testing.T) {
	ts, _ := setupAdminTestServer(t, "")

	// No engine = 503 before validating body.
	body := `{"address":"0x456"}`
	resp, err := http.Post(ts.URL+"/v1/admin/contracts", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST /v1/admin/contracts: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no engine), got %d", resp.StatusCode)
	}
}

func TestAdminDeregister_NoEngine(t *testing.T) {
	ts, _ := setupAdminTestServer(t, "")

	req, _ := http.NewRequest("DELETE", ts.URL+"/v1/admin/contracts/MyToken", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /v1/admin/contracts/MyToken: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", resp.StatusCode)
	}
}

func TestDynamicContractPersistence(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	cc := config.ContractConfig{
		Name:       "DynContract",
		Address:    "0x789",
		ABI:        "fetch",
		StartBlock: 100,
		Dynamic:    true,
		Events: []config.EventConfig{
			{Name: "*", Table: config.TableConfig{Type: "log"}},
		},
	}

	// Save.
	if err := st.SaveDynamicContract(ctx, &cc); err != nil {
		t.Fatalf("SaveDynamicContract: %v", err)
	}

	// Load.
	contracts, err := st.GetDynamicContracts(ctx)
	if err != nil {
		t.Fatalf("GetDynamicContracts: %v", err)
	}

	if len(contracts) != 1 {
		t.Fatalf("expected 1 dynamic contract, got %d", len(contracts))
	}
	if contracts[0].Name != "DynContract" {
		t.Errorf("expected name DynContract, got %s", contracts[0].Name)
	}
	if contracts[0].Address != "0x789" {
		t.Errorf("expected address 0x789, got %s", contracts[0].Address)
	}
	if contracts[0].StartBlock != 100 {
		t.Errorf("expected start_block 100, got %d", contracts[0].StartBlock)
	}
	if len(contracts[0].Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(contracts[0].Events))
	}

	// Delete.
	if err := st.DeleteDynamicContract(ctx, "DynContract"); err != nil {
		t.Fatalf("DeleteDynamicContract: %v", err)
	}

	contracts, err = st.GetDynamicContracts(ctx)
	if err != nil {
		t.Fatalf("GetDynamicContracts after delete: %v", err)
	}
	if len(contracts) != 0 {
		t.Errorf("expected 0 dynamic contracts, got %d", len(contracts))
	}
}

func TestDropTable(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	schema := &types.TableSchema{
		Name:      "test_events",
		Contract:  "Test",
		Event:     "Event",
		TableType: types.TableTypeLog,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "log_index", Type: "uint64"},
		},
	}
	st.CreateTable(ctx, schema)

	// Insert data.
	st.ApplyOperations(ctx, []store.Operation{
		{Type: store.OpInsert, Table: "test_events", BlockNumber: 1, LogIndex: 0,
			Data: map[string]any{"block_number": uint64(1), "log_index": uint64(0)}},
	})

	// Verify data exists.
	events, _ := st.GetEvents(ctx, "test_events", store.Query{})
	if len(events) != 1 {
		t.Fatalf("expected 1 event before drop, got %d", len(events))
	}

	// Drop table.
	if err := st.DropTable(ctx, "test_events"); err != nil {
		t.Fatalf("DropTable: %v", err)
	}

	// Verify data gone.
	events, _ = st.GetEvents(ctx, "test_events", store.Query{})
	if len(events) != 0 {
		t.Errorf("expected 0 events after drop, got %d", len(events))
	}
}

func TestDeleteCursor(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	st.SetCursor(ctx, "TestContract", 42)

	cursor, _ := st.GetCursor(ctx, "TestContract")
	if cursor != 42 {
		t.Fatalf("expected cursor 42, got %d", cursor)
	}

	if err := st.DeleteCursor(ctx, "TestContract"); err != nil {
		t.Fatalf("DeleteCursor: %v", err)
	}

	cursor, _ = st.GetCursor(ctx, "TestContract")
	if cursor != 0 {
		t.Errorf("expected cursor 0 after delete, got %d", cursor)
	}
}

func TestDynamicSchemaRegistration(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	// Start with one schema.
	initialSchema := &types.TableSchema{
		Name:      "mytoken_transfer",
		Contract:  "MyToken",
		Event:     "Transfer",
		TableType: types.TableTypeLog,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "log_index", Type: "uint64"},
		},
	}
	st.CreateTable(ctx, initialSchema)

	server := api.New(&api.ServerConfig{
		Store:   st,
		Schemas: []*types.TableSchema{initialSchema},
		APIConfig: &config.APIConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Contracts: []config.ContractConfig{
			{Name: "MyToken", Address: "0x123"},
		},
		Logger: slog.Default(),
	})

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Initial schema accessible.
	resp, _ := http.Get(ts.URL + "/v1/MyToken/Transfer")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for initial schema, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Dynamic schema not yet accessible.
	resp, _ = http.Get(ts.URL + "/v1/NewContract/Swap")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unregistered schema, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Add dynamic schemas.
	newSchema := &types.TableSchema{
		Name:      "newcontract_swap",
		Contract:  "NewContract",
		Event:     "Swap",
		TableType: types.TableTypeLog,
		Columns: []types.Column{
			{Name: "block_number", Type: "uint64"},
			{Name: "log_index", Type: "uint64"},
		},
	}
	st.CreateTable(ctx, newSchema)
	server.AddSchemas(&config.ContractConfig{Name: "NewContract", Address: "0x456"}, []*types.TableSchema{newSchema})

	// Dynamic schema now accessible.
	resp, _ = http.Get(ts.URL + "/v1/NewContract/Swap")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for dynamic schema, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Remove dynamic schemas.
	server.RemoveSchemas("NewContract")

	// Dynamic schema no longer accessible.
	resp, _ = http.Get(ts.URL + "/v1/NewContract/Swap")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after removing schema, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Original schema still accessible.
	resp, _ = http.Get(ts.URL + "/v1/MyToken/Transfer")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for original schema after dynamic removal, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminContractJSONSerialization(t *testing.T) {
	cc := config.ContractConfig{
		Name:       "TestContract",
		Address:    "0xabc",
		ABI:        "fetch",
		StartBlock: 500,
		Dynamic:    true,
		Events: []config.EventConfig{
			{
				Name: "*",
				Table: config.TableConfig{
					Type: "log",
				},
			},
			{
				Name: "Transfer",
				Table: config.TableConfig{
					Type:      "unique",
					UniqueKey: "sender",
				},
			},
		},
	}

	data, err := json.Marshal(cc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded config.ContractConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Name != cc.Name {
		t.Errorf("Name: expected %s, got %s", cc.Name, decoded.Name)
	}
	if decoded.Address != cc.Address {
		t.Errorf("Address: expected %s, got %s", cc.Address, decoded.Address)
	}
	if decoded.StartBlock != cc.StartBlock {
		t.Errorf("StartBlock: expected %d, got %d", cc.StartBlock, decoded.StartBlock)
	}
	if len(decoded.Events) != len(cc.Events) {
		t.Errorf("Events: expected %d, got %d", len(cc.Events), len(decoded.Events))
	}
	if decoded.Events[1].Table.UniqueKey != "sender" {
		t.Errorf("UniqueKey: expected 'sender', got %s", decoded.Events[1].Table.UniqueKey)
	}
}
