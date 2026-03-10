package config

import (
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
	if cfg.Indexer.StartBlock != 100000 {
		t.Errorf("start_block = %d, want 100000", cfg.Indexer.StartBlock)
	}
	if len(cfg.Contracts[0].Events) != 3 {
		t.Errorf("events count = %d, want 3", len(cfg.Contracts[0].Events))
	}
}
