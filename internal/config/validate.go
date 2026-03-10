package config

import (
	"fmt"
	"strings"
)

var validNetworks = map[string]bool{
	"mainnet": true,
	"sepolia": true,
	"custom":  true,
}

var validBackends = map[string]bool{
	"postgres": true,
	"badger":   true,
	"memory":   true,
}

var validTableTypes = map[string]bool{
	"log":         true,
	"unique":      true,
	"aggregation": true,
}

var validAggOps = map[string]bool{
	"sum":   true,
	"count": true,
	"avg":   true,
}

// Validate checks the Config for required fields, valid enum values,
// and contract address format. Returns the first error found.
func Validate(cfg *Config) error {
	if cfg.Network == "" {
		return fieldError("network", "required")
	}
	if !validNetworks[cfg.Network] {
		return fieldError("network", "must be one of: mainnet, sepolia, custom")
	}

	if cfg.RPC == "" {
		return fieldError("rpc", "required")
	}
	scheme := cfg.RPCScheme()
	if scheme != "wss" && scheme != "ws" && scheme != "https" && scheme != "http" {
		return fieldError("rpc", "must use wss://, ws://, https://, or http:// scheme")
	}

	if !validBackends[cfg.Database.Backend] {
		return fieldError("database.backend", "must be one of: postgres, badger, memory")
	}

	if cfg.Database.Backend == "postgres" {
		if cfg.Database.Postgres.Host == "" {
			return fieldError("database.postgres.host", "required when backend is postgres")
		}
		if cfg.Database.Postgres.User == "" {
			return fieldError("database.postgres.user", "required when backend is postgres")
		}
		if cfg.Database.Postgres.Name == "" {
			return fieldError("database.postgres.name", "required when backend is postgres")
		}
	}

	if len(cfg.Contracts) == 0 {
		return fieldError("contracts", "at least one contract is required")
	}

	for i, c := range cfg.Contracts {
		prefix := fmt.Sprintf("contracts[%d]", i)
		if c.Name == "" {
			return fieldError(prefix+".name", "required")
		}
		if c.Address == "" {
			return fieldError(prefix+".address", "required")
		}
		if err := validateContractAddress(c.Address); err != nil {
			return fieldError(prefix+".address", err.Error())
		}
		if len(c.Events) == 0 {
			return fieldError(prefix+".events", "at least one event is required")
		}
		for j, e := range c.Events {
			ePrefix := fmt.Sprintf("%s.events[%d]", prefix, j)
			if e.Name == "" {
				return fieldError(ePrefix+".name", "required")
			}
			if !validTableTypes[e.Table.Type] {
				return fieldError(ePrefix+".table.type", "must be one of: log, unique, aggregation")
			}
			if e.Table.Type == "unique" && e.Table.UniqueKey == "" {
				return fieldError(ePrefix+".table.unique_key", "required when table type is unique")
			}
			if e.Table.Type == "aggregation" {
				if len(e.Table.Aggregates) == 0 {
					return fieldError(ePrefix+".table.aggregate", "required when table type is aggregation")
				}
				for k, a := range e.Table.Aggregates {
					aPrefix := fmt.Sprintf("%s.table.aggregate[%d]", ePrefix, k)
					if a.Column == "" {
						return fieldError(aPrefix+".column", "required")
					}
					if !validAggOps[a.Operation] {
						return fieldError(aPrefix+".operation", "must be one of: sum, count, avg")
					}
					if a.Field == "" {
						return fieldError(aPrefix+".field", "required")
					}
				}
			}
		}
	}

	return nil
}

// validateContractAddress checks that the address looks like a Starknet address.
func validateContractAddress(addr string) error {
	if !strings.HasPrefix(addr, "0x") {
		return fmt.Errorf("must start with 0x")
	}
	hex := addr[2:]
	if len(hex) == 0 || len(hex) > 64 {
		return fmt.Errorf("hex part must be 1-64 characters")
	}
	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return fmt.Errorf("invalid hex character: %c", c)
		}
	}
	return nil
}

func fieldError(field, msg string) error {
	return fmt.Errorf("%s: %s", field, msg)
}
