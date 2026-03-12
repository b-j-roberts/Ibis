package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level ibis configuration.
type Config struct {
	Network   string           `yaml:"network"`
	RPC       string           `yaml:"rpc"`
	Database  DatabaseConfig   `yaml:"database"`
	API       APIConfig        `yaml:"api"`
	Indexer   IndexerConfig    `yaml:"indexer"`
	Contracts []ContractConfig `yaml:"contracts"`
}

type DatabaseConfig struct {
	Backend  string         `yaml:"backend"`
	Postgres PostgresConfig `yaml:"postgres"`
	Badger   BadgerConfig   `yaml:"badger"`
}

type PostgresConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
}

type BadgerConfig struct {
	Path string `yaml:"path"`
}

type APIConfig struct {
	Host        string   `yaml:"host"`
	Port        int      `yaml:"port"`
	CORSOrigins []string `yaml:"cors_origins"`
	AdminKey    string   `yaml:"admin_key"`
}

type IndexerConfig struct {
	StartBlock    uint64 `yaml:"start_block"`
	PendingBlocks bool   `yaml:"pending_blocks"`
	BatchSize     int    `yaml:"batch_size"`
}

type ContractConfig struct {
	Name       string        `yaml:"name" json:"name"`
	Address    string        `yaml:"address" json:"address"`
	ABI        string        `yaml:"abi" json:"abi"`
	Events     []EventConfig `yaml:"events" json:"events"`
	StartBlock uint64        `yaml:"start_block,omitempty" json:"start_block,omitempty"`
	Dynamic    bool          `yaml:"-" json:"dynamic,omitempty"`
}

type EventConfig struct {
	Name  string      `yaml:"name" json:"name"`
	Table TableConfig `yaml:"table" json:"table"`
}

type TableConfig struct {
	Type       string            `yaml:"type" json:"type"`
	UniqueKey  string            `yaml:"unique_key" json:"unique_key,omitempty"`
	Aggregates []AggregateConfig `yaml:"aggregate" json:"aggregate,omitempty"`
}

type AggregateConfig struct {
	Column    string `yaml:"column" json:"column"`
	Operation string `yaml:"operation" json:"operation"`
	Field     string `yaml:"field" json:"field"`
}

// envVarPattern matches ${VAR_NAME} for environment variable expansion.
var envVarPattern = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// expandEnvVars replaces all ${VAR_NAME} occurrences with their environment
// variable values. Unset variables expand to empty string.
func expandEnvVars(data []byte) []byte {
	return envVarPattern.ReplaceAllFunc(data, func(match []byte) []byte {
		varName := envVarPattern.FindSubmatch(match)[1]
		return []byte(os.Getenv(string(varName)))
	})
}

// Load reads the YAML config file at path, expands environment variables,
// parses it into a Config, and validates it.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	expanded := expandEnvVars(data)

	var cfg Config
	if err := yaml.Unmarshal(expanded, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	applyDefaults(&cfg)

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Database.Backend == "" {
		cfg.Database.Backend = "memory"
	}
	if cfg.Database.Postgres.Port == 0 {
		cfg.Database.Postgres.Port = 5432
	}
	if cfg.Database.Badger.Path == "" {
		cfg.Database.Badger.Path = "./data/ibis"
	}
	if cfg.API.Host == "" {
		cfg.API.Host = "0.0.0.0"
	}
	if cfg.API.Port == 0 {
		cfg.API.Port = 8080
	}
	if cfg.Indexer.BatchSize == 0 {
		cfg.Indexer.BatchSize = 10
	}

	for i := range cfg.Contracts {
		if cfg.Contracts[i].ABI == "" {
			cfg.Contracts[i].ABI = "fetch"
		}
	}
}

// RPCScheme returns the scheme of the RPC URL (wss, ws, https, http).
func (c *Config) RPCScheme() string {
	if idx := strings.Index(c.RPC, "://"); idx != -1 {
		return c.RPC[:idx]
	}
	return ""
}

// IsWSS returns true if the RPC URL uses a WebSocket scheme.
func (c *Config) IsWSS() bool {
	scheme := c.RPCScheme()
	return scheme == "wss" || scheme == "ws"
}
