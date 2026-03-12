package store

import (
	"context"

	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/types"
)

// Store is the database-agnostic interface for the indexer storage layer.
// Implementations exist for BadgerDB (embedded), PostgreSQL (external), and
// in-memory (dev/test).
type Store interface {
	// ApplyOperations writes a batch of operations atomically.
	ApplyOperations(ctx context.Context, ops []Operation) error

	// RevertOperations undoes a batch of operations atomically.
	// Each operation is inverted before execution (insert→delete, etc).
	RevertOperations(ctx context.Context, ops []Operation) error

	// GetEvents returns events from a log table with pagination, ordering,
	// and field filtering.
	GetEvents(ctx context.Context, table string, query Query) ([]types.IndexedEvent, error)

	// GetUniqueEvents returns the latest entry per unique key from a unique table.
	GetUniqueEvents(ctx context.Context, table string, query Query) ([]types.IndexedEvent, error)

	// GetAggregation returns computed aggregate values for an aggregation table.
	GetAggregation(ctx context.Context, table string, query Query) (AggResult, error)

	// GetCursor returns the last processed block number for the given contract,
	// or 0 if no cursor exists.
	GetCursor(ctx context.Context, contract string) (uint64, error)

	// SetCursor persists the last processed block number for the given contract.
	SetCursor(ctx context.Context, contract string, blockNumber uint64) error

	// GetAllCursors returns a map of contract name to last processed block number
	// for all contracts that have a persisted cursor.
	GetAllCursors(ctx context.Context) (map[string]uint64, error)

	// CreateTable initializes a table from the given schema.
	CreateTable(ctx context.Context, schema *types.TableSchema) error

	// MigrateTable updates a table schema (adds new columns, never drops).
	MigrateTable(ctx context.Context, schema *types.TableSchema) error

	// CountEvents returns the total number of events matching the filters.
	CountEvents(ctx context.Context, table string, filters []Filter) (int64, error)

	// DropTable removes a table and all its data. Used when deregistering contracts.
	DropTable(ctx context.Context, tableName string) error

	// SaveDynamicContract persists a dynamically registered contract config.
	SaveDynamicContract(ctx context.Context, cc *config.ContractConfig) error

	// GetDynamicContracts returns all dynamically registered contract configs.
	GetDynamicContracts(ctx context.Context) ([]config.ContractConfig, error)

	// DeleteDynamicContract removes a persisted dynamic contract config.
	DeleteDynamicContract(ctx context.Context, name string) error

	// DeleteCursor removes the cursor for the given contract.
	DeleteCursor(ctx context.Context, contract string) error

	// Close releases all resources held by the store.
	Close() error
}
