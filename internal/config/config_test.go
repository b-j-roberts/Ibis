package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ibis.config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const validConfig = `
network: mainnet
rpc: wss://starknet-mainnet.example.com
database:
  backend: memory
contracts:
  - name: TestContract
    address: "0x049d36570d4e46f48e99674bd3fcc84644ddd6b96f7c741b1562b82f9e004dc7"
    events:
      - name: "*"
        table:
          type: log
`

func TestLoad_Valid(t *testing.T) {
	path := writeTestConfig(t, validConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Network != "mainnet" {
		t.Errorf("network = %q, want mainnet", cfg.Network)
	}
	if cfg.RPC != "wss://starknet-mainnet.example.com" {
		t.Errorf("rpc = %q, want wss://...", cfg.RPC)
	}
	if cfg.Database.Backend != "memory" {
		t.Errorf("backend = %q, want memory", cfg.Database.Backend)
	}
	if cfg.API.Host != "0.0.0.0" {
		t.Errorf("api.host = %q, want 0.0.0.0", cfg.API.Host)
	}
	if cfg.API.Port != 8080 {
		t.Errorf("api.port = %d, want 8080", cfg.API.Port)
	}
	if cfg.Indexer.BatchSize != 10 {
		t.Errorf("indexer.batch_size = %d, want 10", cfg.Indexer.BatchSize)
	}
	if len(cfg.Contracts) != 1 {
		t.Fatalf("contracts count = %d, want 1", len(cfg.Contracts))
	}
	if cfg.Contracts[0].ABI != "fetch" {
		t.Errorf("contracts[0].abi = %q, want fetch (default)", cfg.Contracts[0].ABI)
	}
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	t.Setenv("TEST_RPC_URL", "wss://expanded.example.com")
	t.Setenv("TEST_DB_PASS", "secret123")

	config := `
network: mainnet
rpc: ${TEST_RPC_URL}
database:
  backend: postgres
  postgres:
    host: localhost
    port: 5432
    user: ibis
    password: ${TEST_DB_PASS}
    name: ibis
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
`
	path := writeTestConfig(t, config)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RPC != "wss://expanded.example.com" {
		t.Errorf("rpc = %q, want wss://expanded.example.com", cfg.RPC)
	}
	if cfg.Database.Postgres.Password != "secret123" {
		t.Errorf("postgres.password = %q, want secret123", cfg.Database.Postgres.Password)
	}
}

func TestLoad_UnsetEnvVar(t *testing.T) {
	os.Unsetenv("UNSET_VAR_FOR_TEST")
	config := `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
`
	path := writeTestConfig(t, config)
	_, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/ibis.config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTestConfig(t, `{{invalid yaml`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidate_MissingNetwork(t *testing.T) {
	path := writeTestConfig(t, `
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidate_InvalidNetwork(t *testing.T) {
	path := writeTestConfig(t, `
network: devnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for invalid network")
	}
}

func TestValidate_MissingRPC(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for missing rpc")
	}
}

func TestValidate_InvalidRPCScheme(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: ftp://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for invalid rpc scheme")
	}
}

func TestValidate_PostgresRequiresFields(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: postgres
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for postgres missing host")
	}
}

func TestValidate_NoContracts(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts: []
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for no contracts")
	}
}

func TestValidate_InvalidContractAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"no_prefix", "abc123"},
		{"empty_hex", "0x"},
		{"invalid_chars", "0xGGGG"},
		{"too_long", "0x" + "a" + string(make([]byte, 65))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "`+tt.address+`"
    events:
      - name: E
        table:
          type: log
`)
			_, err := Load(path)
			if err == nil {
				t.Fatalf("expected validation error for address %q", tt.address)
			}
		})
	}
}

func TestValidate_UniqueTableRequiresKey(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: unique
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for unique table missing unique_key")
	}
}

func TestValidate_AggregationTableRequiresAggregates(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: aggregation
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for aggregation table missing aggregates")
	}
}

func TestValidate_InvalidAggOperation(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: aggregation
          aggregate:
            - column: total
              operation: max
              field: value
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for invalid aggregate operation")
	}
}

func TestLoad_UDCAddressDefault(t *testing.T) {
	path := writeTestConfig(t, validConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Indexer.UDCAddress != "0x04a64cd09a853868621d94cae9952b106f2c36a3f81260f85de6696c6b050221" {
		t.Errorf("udc_address = %q, want default UDC address", cfg.Indexer.UDCAddress)
	}
}

func TestLoad_UDCAddressCustom(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://starknet-mainnet.example.com
database:
  backend: memory
indexer:
  udc_address: "0x041a78e741e5af2fec34b695679bc6891742439f7afb8484ecd7766661ad02bf"
contracts:
  - name: TestContract
    address: "0x049d36570d4e46f48e99674bd3fcc84644ddd6b96f7c741b1562b82f9e004dc7"
    events:
      - name: "*"
        table:
          type: log
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Indexer.UDCAddress != "0x041a78e741e5af2fec34b695679bc6891742439f7afb8484ecd7766661ad02bf" {
		t.Errorf("udc_address = %q, want custom devnet address", cfg.Indexer.UDCAddress)
	}
}

func TestValidate_InvalidUDCAddress(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://starknet-mainnet.example.com
database:
  backend: memory
indexer:
  udc_address: "not-a-hex-address"
contracts:
  - name: TestContract
    address: "0x049d36570d4e46f48e99674bd3fcc84644ddd6b96f7c741b1562b82f9e004dc7"
    events:
      - name: "*"
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for invalid udc_address")
	}
}

func TestConfig_IsWSS(t *testing.T) {
	tests := []struct {
		rpc  string
		want bool
	}{
		{"wss://example.com", true},
		{"ws://example.com", true},
		{"https://example.com", false},
		{"http://example.com", false},
	}
	for _, tt := range tests {
		cfg := &Config{RPC: tt.rpc}
		if got := cfg.IsWSS(); got != tt.want {
			t.Errorf("IsWSS(%q) = %v, want %v", tt.rpc, got, tt.want)
		}
	}
}

func TestValidate_FullConfig(t *testing.T) {
	path := writeTestConfig(t, `
network: sepolia
rpc: https://starknet-sepolia.example.com
database:
  backend: postgres
  postgres:
    host: localhost
    port: 5432
    user: ibis
    password: secret
    name: ibis_dev
api:
  host: 127.0.0.1
  port: 9090
indexer:
  start_block: 100000
  pending_blocks: true
  batch_size: 20
contracts:
  - name: StarknetOptions
    address: "0x049d36570d4e46f48e99674bd3fcc84644ddd6b96f7c741b1562b82f9e004dc7"
    abi: ./target/dev/stops_StarknetOptions.contract_class.json
    events:
      - name: "*"
        table:
          type: log
      - name: LeaderboardUpdate
        table:
          type: unique
          unique_key: trader_address
      - name: VolumeUpdate
        table:
          type: aggregation
          aggregate:
            - column: total_volume
              operation: sum
              field: volume
            - column: trade_count
              operation: count
              field: volume
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Network != "sepolia" {
		t.Errorf("network = %q", cfg.Network)
	}
	if cfg.API.Port != 9090 {
		t.Errorf("api.port = %d, want 9090", cfg.API.Port)
	}
	if cfg.Indexer.StartBlock == nil || *cfg.Indexer.StartBlock != 100000 {
		t.Errorf("start_block = %v, want 100000", cfg.Indexer.StartBlock)
	}
	if len(cfg.Contracts[0].Events) != 3 {
		t.Errorf("events count = %d, want 3", len(cfg.Contracts[0].Events))
	}
}

// --- View Config Tests ---

func TestLoad_ViewConfig(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://starknet-mainnet.example.com
database:
  backend: memory
contracts:
  - name: PriceOracle
    address: "0x049d36570d4e46f48e99674bd3fcc84644ddd6b96f7c741b1562b82f9e004dc7"
    events:
      - name: PriceUpdated
        table:
          type: log
    views:
      - function: get_price
        calldata: ["0x4554480000000000000000000000000000000000000000000000000000000000"]
        interval: 30s
        table:
          type: unique
          unique_key: _view_key
      - function: total_supply
        interval: 5m
        table:
          type: log
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Contracts[0].Views) != 2 {
		t.Fatalf("views count = %d, want 2", len(cfg.Contracts[0].Views))
	}
	v := cfg.Contracts[0].Views[0]
	if v.Function != "get_price" {
		t.Errorf("views[0].function = %q, want get_price", v.Function)
	}
	if v.Interval != "30s" {
		t.Errorf("views[0].interval = %q, want 30s", v.Interval)
	}
	if len(v.Calldata) != 1 {
		t.Errorf("views[0].calldata count = %d, want 1", len(v.Calldata))
	}
	if v.Table.Type != "unique" {
		t.Errorf("views[0].table.type = %q, want unique", v.Table.Type)
	}
	if v.Table.UniqueKey != "_view_key" {
		t.Errorf("views[0].table.unique_key = %q, want _view_key", v.Table.UniqueKey)
	}

	v2 := cfg.Contracts[0].Views[1]
	if v2.Function != "total_supply" {
		t.Errorf("views[1].function = %q, want total_supply", v2.Function)
	}
	if v2.Table.Type != "log" {
		t.Errorf("views[1].table.type = %q, want log", v2.Table.Type)
	}
}

func TestValidate_ViewMissingFunction(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
    views:
      - interval: 30s
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for view missing function")
	}
}

func TestValidate_ViewMissingInterval(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
    views:
      - function: get_price
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for view missing interval")
	}
}

func TestValidate_ViewInvalidInterval(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
    views:
      - function: get_price
        interval: not-a-duration
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for invalid interval")
	}
}

func TestValidate_ViewIntervalTooShort(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
    views:
      - function: get_price
        interval: 500ms
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for interval < 1s")
	}
}

func TestValidate_ViewAggregationNotAllowed(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
    views:
      - function: get_price
        interval: 30s
        table:
          type: aggregation
          aggregate:
            - column: total
              operation: sum
              field: value
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for aggregation table type in view")
	}
}

func TestValidate_ViewBadCalldata(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
    views:
      - function: get_price
        calldata: ["not-hex"]
        interval: 30s
        table:
          type: log
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for bad calldata hex")
	}
}

// --- EventConfig JSON UnmarshalJSON Tests ---

func TestEventConfig_UnmarshalJSON_NestedFormat(t *testing.T) {
	input := `{"name": "Transfer", "table": {"type": "log"}}`
	var ec EventConfig
	if err := json.Unmarshal([]byte(input), &ec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if ec.Name != "Transfer" {
		t.Errorf("Name = %q, want Transfer", ec.Name)
	}
	if ec.Table.Type != "log" {
		t.Errorf("Table.Type = %q, want log", ec.Table.Type)
	}
}

func TestEventConfig_UnmarshalJSON_FlatFormat(t *testing.T) {
	input := `{"name": "Transfer", "table_type": "log"}`
	var ec EventConfig
	if err := json.Unmarshal([]byte(input), &ec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if ec.Name != "Transfer" {
		t.Errorf("Name = %q, want Transfer", ec.Name)
	}
	if ec.Table.Type != "log" {
		t.Errorf("Table.Type = %q, want log", ec.Table.Type)
	}
}

func TestEventConfig_UnmarshalJSON_FlatUniqueKey(t *testing.T) {
	input := `{"name": "Balance", "table_type": "unique", "unique_key": "account"}`
	var ec EventConfig
	if err := json.Unmarshal([]byte(input), &ec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if ec.Table.Type != "unique" {
		t.Errorf("Table.Type = %q, want unique", ec.Table.Type)
	}
	if ec.Table.UniqueKey != "account" {
		t.Errorf("Table.UniqueKey = %q, want account", ec.Table.UniqueKey)
	}
}

func TestEventConfig_UnmarshalJSON_NestedTakesPrecedence(t *testing.T) {
	// When both nested and flat are provided, nested wins.
	input := `{"name": "Transfer", "table": {"type": "unique", "unique_key": "sender"}, "table_type": "log"}`
	var ec EventConfig
	if err := json.Unmarshal([]byte(input), &ec); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if ec.Table.Type != "unique" {
		t.Errorf("Table.Type = %q, want unique (nested should take precedence)", ec.Table.Type)
	}
	if ec.Table.UniqueKey != "sender" {
		t.Errorf("Table.UniqueKey = %q, want sender", ec.Table.UniqueKey)
	}
}

func TestEventConfig_UnmarshalJSON_ContractConfigWithFlatEvents(t *testing.T) {
	input := `{
		"name": "NewToken",
		"address": "0x123",
		"events": [
			{"name": "Transfer", "table_type": "log"},
			{"name": "Balance", "table_type": "unique", "unique_key": "account"}
		]
	}`
	var cc ContractConfig
	if err := json.Unmarshal([]byte(input), &cc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(cc.Events) != 2 {
		t.Fatalf("Events count = %d, want 2", len(cc.Events))
	}
	if cc.Events[0].Table.Type != "log" {
		t.Errorf("Events[0].Table.Type = %q, want log", cc.Events[0].Table.Type)
	}
	if cc.Events[1].Table.Type != "unique" {
		t.Errorf("Events[1].Table.Type = %q, want unique", cc.Events[1].Table.Type)
	}
	if cc.Events[1].Table.UniqueKey != "account" {
		t.Errorf("Events[1].Table.UniqueKey = %q, want account", cc.Events[1].Table.UniqueKey)
	}
}

func TestValidate_ViewUniqueRequiresKey(t *testing.T) {
	path := writeTestConfig(t, `
network: mainnet
rpc: wss://example.com
database:
  backend: memory
contracts:
  - name: C
    address: "0xabc"
    events:
      - name: E
        table:
          type: log
    views:
      - function: get_price
        interval: 30s
        table:
          type: unique
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for unique view missing unique_key")
	}
}
