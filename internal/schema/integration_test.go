package schema

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/NethermindEth/starknet.go/rpc"

	"github.com/b-j-roberts/ibis/internal/abi"
	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/types"
)

// STRK token contract on Sepolia.
const strkSepoliaAddress = "0x04718f5a0fc34cc1af16a1cdee98ffb20c31f5cd61d6ab07201858f4287c938d"

func TestIntegration_STRKContractSchemaGeneration(t *testing.T) {
	rpcURL := os.Getenv("IBIS_RPC_URL")
	if rpcURL == "" {
		rpcURL = os.Getenv("IBIS_RPC_URL_SEPOLIA")
	}
	if rpcURL == "" {
		t.Skip("set IBIS_RPC_URL or IBIS_RPC_URL_SEPOLIA to run integration tests")
	}

	ctx := context.Background()

	// Step 1: Connect to Sepolia RPC and fetch the STRK contract ABI.
	client, err := rpc.NewProvider(ctx, rpcURL)
	if err != nil {
		// starknet.go may return ErrIncompatibleVersion but still provide a usable client.
		t.Logf("warning creating RPC client (may still work): %v", err)
		if client == nil {
			t.Fatalf("creating RPC client: %v", err)
		}
	}

	resolver := config.NewABIResolver(client)
	cc := config.ContractConfig{
		Name:    "STRK",
		Address: strkSepoliaAddress,
		ABI:     "fetch",
	}

	contractABI, err := resolver.Resolve(ctx, &cc)
	if err != nil {
		t.Fatalf("resolving ABI: %v", err)
	}

	t.Logf("Resolved ABI with %d events, %d types", len(contractABI.Events), len(contractABI.Types))

	if len(contractABI.Events) == 0 {
		t.Fatal("expected at least one event in STRK ABI")
	}

	// Print discovered events.
	registry := abi.NewEventRegistry(contractABI)
	for _, ev := range registry.Events() {
		t.Logf("  Event: %s (keys: %d, data: %d)", ev.Name, len(ev.KeyMembers), len(ev.DataMembers))
		for _, m := range ev.KeyMembers {
			t.Logf("    key: %s (%s)", m.Name, m.Type.Name)
		}
		for _, m := range ev.DataMembers {
			t.Logf("    data: %s (%s)", m.Name, m.Type.Name)
		}
	}

	// Step 2: Build schemas with wildcard - all events as log tables.
	wildcardCC := config.ContractConfig{
		Name:    "STRK",
		Address: strkSepoliaAddress,
		Events: []config.EventConfig{
			{Name: "*", Table: config.TableConfig{Type: "log"}},
		},
	}

	schemas := BuildSchemas(&wildcardCC, contractABI, registry)
	t.Logf("Generated %d schemas from wildcard", len(schemas))

	if len(schemas) != len(contractABI.Events) {
		t.Fatalf("wildcard should generate schema for each ABI event: got %d, expected %d",
			len(schemas), len(contractABI.Events))
	}

	// Verify each schema has proper structure.
	for name, s := range schemas {
		t.Logf("Schema: %s (table: %s, type: %s, columns: %d)",
			name, s.Name, s.TableType, len(s.Columns))

		// Table name should be lowercase.
		if s.Name != strings.ToLower(s.Name) {
			t.Errorf("table name %s should be lowercase", s.Name)
		}

		// Should have metadata columns.
		colNames := make(map[string]bool)
		for _, col := range s.Columns {
			colNames[col.Name] = true
		}
		for _, meta := range []string{"block_number", "transaction_hash", "log_index", "timestamp", "contract_address", "event_name", "status"} {
			if !colNames[meta] {
				t.Errorf("schema %s missing metadata column %s", name, meta)
			}
		}

		// Should have at least metadata columns.
		if len(s.Columns) < 7 {
			t.Errorf("schema %s has too few columns: %d", name, len(s.Columns))
		}

		// Type should be log (wildcard default).
		if s.TableType != types.TableTypeLog {
			t.Errorf("schema %s should be log type, got %v", name, s.TableType)
		}
	}

	// Step 3: Build schemas with Transfer override as unique table.
	// Check if Transfer event exists (standard ERC20 event).
	if _, hasTransfer := schemas["Transfer"]; hasTransfer {
		overrideCC := config.ContractConfig{
			Name:    "STRK",
			Address: strkSepoliaAddress,
			Events: []config.EventConfig{
				{Name: "*", Table: config.TableConfig{Type: "log"}},
				{Name: "Transfer", Table: config.TableConfig{Type: "unique", UniqueKey: "from"}},
			},
		}

		overrideSchemas := BuildSchemas(&overrideCC, contractABI, registry)

		if overrideSchemas["Transfer"].TableType != types.TableTypeUnique {
			t.Fatal("Transfer should be overridden to unique")
		}
		if overrideSchemas["Transfer"].UniqueKey != "from" {
			t.Fatalf("Transfer unique key should be 'from', got '%s'", overrideSchemas["Transfer"].UniqueKey)
		}

		// Other events should remain log.
		for name, s := range overrideSchemas {
			if name != "Transfer" && s.TableType != types.TableTypeLog {
				t.Errorf("non-overridden event %s should be log, got %v", name, s.TableType)
			}
		}

		t.Log("Transfer override to unique: PASS")
	}

	// Step 4: Generate Postgres SQL for each schema.
	t.Log("\n--- Postgres SQL Generation ---")
	for name, s := range schemas {
		sql := GenerateCreateTableSQL(s)
		if sql == "" {
			t.Errorf("empty SQL for schema %s", name)
		}
		if !strings.Contains(sql, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s", s.Name)) {
			t.Errorf("SQL for %s missing CREATE TABLE", name)
		}
		if !strings.Contains(sql, "BIGINT") {
			t.Errorf("SQL for %s missing BIGINT columns", name)
		}

		// Print first schema's SQL as sample.
		if name == registry.Events()[0].Name {
			t.Logf("Sample SQL for %s:\n%s", name, sql)
		}
	}

	// Step 5: Generate BadgerDB key patterns for each schema.
	t.Log("\n--- BadgerDB Key Patterns ---")
	for name, s := range schemas {
		patterns := GenerateBadgerKeyPatterns(s)
		if patterns.PrimaryPrefix == "" {
			t.Errorf("empty primary prefix for schema %s", name)
		}
		if patterns.ReversePrefix == "" {
			t.Errorf("empty reverse prefix for schema %s", name)
		}
		if patterns.SchemaKey == "" {
			t.Errorf("empty schema key for schema %s", name)
		}

		t.Logf("  %s: primary=%s reverse=%s schema=%s",
			name, patterns.PrimaryPrefix, patterns.ReversePrefix, patterns.SchemaKey)
	}

	// Step 6: Verify column type mapping for real ABI types.
	t.Log("\n--- Column Type Verification ---")
	for name, s := range schemas {
		for _, col := range s.Columns {
			validTypes := map[string]bool{
				"string": true, "int64": true, "uint64": true, "bool": true, "[]byte": true,
			}
			if !validTypes[col.Type] {
				t.Errorf("schema %s column %s has invalid type %s", name, col.Name, col.Type)
			}
		}
		t.Logf("  %s: all %d columns have valid types", name, len(s.Columns))
	}

	t.Log("\nIntegration test PASSED - all schemas generated successfully from live STRK contract ABI")
}
