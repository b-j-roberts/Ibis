package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/b-j-roberts/ibis/internal/abi"
	"github.com/b-j-roberts/ibis/internal/config"
)

func TestShortContractName(t *testing.T) {
	tests := []struct {
		address string
		want    string
	}{
		{"0x049d36570d4e46f48e99674bd3fcc84644ddd6b96f7c741b1562b82f9e004dc7", "Contract_049d36"},
		{"0xabc", "Contract"},
	}
	for _, tt := range tests {
		got := shortContractName(tt.address)
		if got != tt.want {
			t.Errorf("shortContractName(%q) = %q, want %q", tt.address, got, tt.want)
		}
	}
}

func TestTruncateAddress(t *testing.T) {
	long := "0x049d36570d4e46f48e99674bd3fcc84644ddd6b96f7c741b1562b82f9e004dc7"
	got := truncateAddress(long)
	if !strings.HasPrefix(got, "0x049d36") || !strings.HasSuffix(got, "4dc7") {
		t.Errorf("truncateAddress(%q) = %q, expected truncated form", long, got)
	}

	short := "0xabc123"
	got = truncateAddress(short)
	if got != short {
		t.Errorf("truncateAddress(%q) = %q, expected unchanged", short, got)
	}
}

func TestDescribeEventFields(t *testing.T) {
	ev := &abi.EventDef{
		Name: "Transfer",
		KeyMembers: []abi.FieldDef{
			{Name: "from"},
			{Name: "to"},
		},
		DataMembers: []abi.FieldDef{
			{Name: "amount"},
		},
	}
	got := describeEventFields(ev)
	if !strings.Contains(got, "from") || !strings.Contains(got, "amount") {
		t.Errorf("describeEventFields = %q, expected field names", got)
	}

	empty := &abi.EventDef{Name: "Empty"}
	got = describeEventFields(empty)
	if got != "no fields" {
		t.Errorf("describeEventFields(empty) = %q, want %q", got, "no fields")
	}
}

func TestAllEventFieldNames(t *testing.T) {
	ev := &abi.EventDef{
		KeyMembers: []abi.FieldDef{
			{Name: "sender"},
		},
		DataMembers: []abi.FieldDef{
			{Name: "value"},
			{Name: "timestamp"},
		},
	}
	names := allEventFieldNames(ev)
	if len(names) != 3 {
		t.Fatalf("allEventFieldNames returned %d names, want 3", len(names))
	}
	if names[0] != "sender" || names[1] != "value" || names[2] != "timestamp" {
		t.Errorf("allEventFieldNames = %v", names)
	}
}

func TestBuildConfig(t *testing.T) {
	contracts := []config.ContractConfig{
		{
			Name:    "TestContract",
			Address: "0xabc",
			ABI:     "fetch",
			Events: []config.EventConfig{
				{Name: "*", Table: config.TableConfig{Type: "log"}},
			},
		},
	}

	cfg := buildConfig("mainnet", "wss://example.com", "memory", contracts)

	if cfg.Network != "mainnet" {
		t.Errorf("Network = %q, want %q", cfg.Network, "mainnet")
	}
	if cfg.RPC != "wss://example.com" {
		t.Errorf("RPC = %q", cfg.RPC)
	}
	if cfg.Database.Backend != "memory" {
		t.Errorf("Database.Backend = %q", cfg.Database.Backend)
	}
	if cfg.API.Port != 8080 {
		t.Errorf("API.Port = %d, want 8080", cfg.API.Port)
	}
	if len(cfg.Contracts) != 1 {
		t.Fatalf("len(Contracts) = %d, want 1", len(cfg.Contracts))
	}
}

func TestBuildConfigPostgres(t *testing.T) {
	cfg := buildConfig("mainnet", "wss://example.com", "postgres", nil)

	if cfg.Database.Backend != "postgres" {
		t.Errorf("Database.Backend = %q", cfg.Database.Backend)
	}
	if cfg.Database.Postgres.Port != 5432 {
		t.Errorf("Postgres.Port = %d, want 5432", cfg.Database.Postgres.Port)
	}
	if cfg.Database.Postgres.Host != "${IBIS_DB_HOST}" {
		t.Errorf("Postgres.Host = %q", cfg.Database.Postgres.Host)
	}
}

func TestBuildConfigBadger(t *testing.T) {
	cfg := buildConfig("sepolia", "https://rpc.example.com", "badger", nil)

	if cfg.Database.Badger.Path != "./data/ibis" {
		t.Errorf("Badger.Path = %q", cfg.Database.Badger.Path)
	}
}

func TestWriteConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ibis.config.yaml")

	cfg := buildConfig("mainnet", "wss://test.example.com", "memory", []config.ContractConfig{
		{
			Name:    "Test",
			Address: "0x123",
			ABI:     "fetch",
			Events: []config.EventConfig{
				{Name: "*", Table: config.TableConfig{Type: "log"}},
			},
		},
	})

	var buf bytes.Buffer
	err := writeConfig(&buf, cfg, path)
	if err != nil {
		t.Fatalf("writeConfig error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "ibis init") {
		t.Error("output missing header comment")
	}
	if !strings.Contains(content, "mainnet") {
		t.Error("output missing network")
	}
	if !strings.Contains(content, "wss://test.example.com") {
		t.Error("output missing RPC URL")
	}
	if !strings.Contains(content, "0x123") {
		t.Error("output missing contract address")
	}
}

func TestPrompterInput(t *testing.T) {
	in := strings.NewReader("hello\n")
	var out bytes.Buffer
	p := newPrompter(in, &out)

	val, err := p.input("Name", "default")
	if err != nil {
		t.Fatalf("input error: %v", err)
	}
	if val != "hello" {
		t.Errorf("input = %q, want %q", val, "hello")
	}
}

func TestPrompterInputDefault(t *testing.T) {
	in := strings.NewReader("\n")
	var out bytes.Buffer
	p := newPrompter(in, &out)

	val, err := p.input("Name", "default_value")
	if err != nil {
		t.Fatalf("input error: %v", err)
	}
	if val != "default_value" {
		t.Errorf("input = %q, want %q", val, "default_value")
	}
}

func TestPrompterSelectOne(t *testing.T) {
	in := strings.NewReader("2\n")
	var out bytes.Buffer
	p := newPrompter(in, &out)

	idx, err := p.selectOne("Pick", []string{"a", "b", "c"}, 0)
	if err != nil {
		t.Fatalf("selectOne error: %v", err)
	}
	if idx != 1 {
		t.Errorf("selectOne = %d, want 1", idx)
	}
}

func TestPrompterSelectOneDefault(t *testing.T) {
	in := strings.NewReader("\n")
	var out bytes.Buffer
	p := newPrompter(in, &out)

	idx, err := p.selectOne("Pick", []string{"a", "b", "c"}, 1)
	if err != nil {
		t.Fatalf("selectOne error: %v", err)
	}
	if idx != 1 {
		t.Errorf("selectOne = %d, want 1 (default)", idx)
	}
}

func TestPrompterSelectMulti(t *testing.T) {
	in := strings.NewReader("1,3\n")
	var out bytes.Buffer
	p := newPrompter(in, &out)

	indices, err := p.selectMulti("Pick", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("selectMulti error: %v", err)
	}
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 2 {
		t.Errorf("selectMulti = %v, want [0, 2]", indices)
	}
}

func TestPrompterSelectMultiAll(t *testing.T) {
	in := strings.NewReader("*\n")
	var out bytes.Buffer
	p := newPrompter(in, &out)

	indices, err := p.selectMulti("Pick", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("selectMulti error: %v", err)
	}
	if len(indices) != 3 {
		t.Errorf("selectMulti = %v, want all 3", indices)
	}
}

func TestPrompterConfirmDefault(t *testing.T) {
	tests := []struct {
		input      string
		defaultYes bool
		want       bool
	}{
		{"\n", true, true},
		{"\n", false, false},
		{"y\n", false, true},
		{"yes\n", false, true},
		{"n\n", true, false},
	}

	for _, tt := range tests {
		in := strings.NewReader(tt.input)
		var out bytes.Buffer
		p := newPrompter(in, &out)

		got, err := p.confirm("Ok?", tt.defaultYes)
		if err != nil {
			t.Fatalf("confirm error: %v", err)
		}
		if got != tt.want {
			t.Errorf("confirm(%q, default=%v) = %v, want %v", tt.input, tt.defaultYes, got, tt.want)
		}
	}
}
