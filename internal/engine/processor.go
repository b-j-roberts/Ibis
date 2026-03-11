package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/NethermindEth/juno/core/felt"

	"github.com/b-j-roberts/ibis/internal/abi"
	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/provider"
	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/types"
)

// processEvent decodes a raw event and writes it to the store.
// Pipeline: match selector -> find ABI definition -> decode fields ->
// generate operation -> apply to store -> track in pending tracker.
func (e *Engine) processEvent(ctx context.Context, raw provider.RawEvent) error {
	// Find which contract this event belongs to.
	cs := e.findContract(raw.ContractAddress)
	if cs == nil {
		return nil // Unknown contract, skip.
	}

	// Match selector from keys[0].
	if len(raw.Keys) == 0 {
		return nil
	}
	eventDef := cs.registry.MatchSelector(raw.Keys[0])
	if eventDef == nil {
		return nil // Unknown event selector, skip.
	}

	// Check if this event is configured for indexing.
	schema, ok := cs.schemas[eventDef.Name]
	if !ok {
		return nil // Not configured, skip.
	}

	// Decode event fields from keys and data.
	decoded, err := abi.DecodeEvent(eventDef, raw.Keys[1:], raw.Data)
	if err != nil {
		return fmt.Errorf("decoding event %s: %w", eventDef.Name, err)
	}

	// Add standard metadata columns.
	logIndex := e.nextLogIndex(raw.BlockNumber)

	decoded["block_number"] = raw.BlockNumber
	decoded["log_index"] = logIndex
	decoded["event_name"] = eventDef.Name
	decoded["contract_address"] = raw.ContractAddress.String()
	if raw.TransactionHash != nil {
		decoded["transaction_hash"] = raw.TransactionHash.String()
	}
	if raw.FinalityStatus != "" {
		decoded["status"] = raw.FinalityStatus
	} else {
		decoded["status"] = "ACCEPTED_ON_L2"
	}

	// Generate insert operation.
	op := store.Operation{
		Type:        store.OpInsert,
		Table:       schema.Name,
		Key:         fmt.Sprintf("%d:%d", raw.BlockNumber, logIndex),
		Data:        decoded,
		BlockNumber: raw.BlockNumber,
		LogIndex:    logIndex,
	}

	// Apply to store.
	if err := e.store.ApplyOperations(ctx, []store.Operation{op}); err != nil {
		return fmt.Errorf("applying operation: %w", err)
	}

	// Track for potential revert.
	e.pending.Track(raw.BlockNumber, op)

	// Update cursor.
	if err := e.store.SetCursor(ctx, raw.BlockNumber); err != nil {
		return fmt.Errorf("setting cursor: %w", err)
	}

	// Promote confirmed blocks.
	e.confirmBlocks(raw.BlockNumber)

	e.logger.Debug("indexed event",
		"event", eventDef.Name,
		"contract", cs.config.Name,
		"block", raw.BlockNumber,
		"log_index", logIndex,
	)

	return nil
}

// findContract returns the contractState matching the given address, or nil.
func (e *Engine) findContract(address *felt.Felt) *contractState {
	if address == nil {
		return nil
	}
	for _, cs := range e.contracts {
		if cs.address != nil && cs.address.Equal(address) {
			return cs
		}
	}
	return nil
}

// nextLogIndex returns and increments the log index counter for a block.
func (e *Engine) nextLogIndex(blockNumber uint64) uint64 {
	idx := e.logIndices[blockNumber]
	e.logIndices[blockNumber] = idx + 1
	return idx
}

// buildSchemas creates TableSchema definitions for a contract's configured events.
// Handles wildcard ("*") expansion: all ABI events get the wildcard's table type,
// with specific event entries overriding the default.
func buildSchemas(cc config.ContractConfig, contractABI *abi.ABI, registry *abi.EventRegistry) map[string]*types.TableSchema {
	schemas := make(map[string]*types.TableSchema)

	// Build a lookup of explicitly configured events.
	explicit := make(map[string]config.EventConfig)
	var wildcard *config.EventConfig
	for _, ec := range cc.Events {
		if ec.Name == "*" {
			ecCopy := ec
			wildcard = &ecCopy
		} else {
			explicit[ec.Name] = ec
		}
	}

	// Determine which events to index.
	var eventsToIndex []*abi.EventDef

	if wildcard != nil {
		// Wildcard: all ABI events.
		eventsToIndex = registry.Events()
	} else {
		// Only explicitly listed events.
		for name := range explicit {
			if ev := registry.MatchName(name); ev != nil {
				eventsToIndex = append(eventsToIndex, ev)
			}
		}
	}

	for _, ev := range eventsToIndex {
		// Use explicit config if available, otherwise wildcard default.
		var ec config.EventConfig
		if explicitEC, ok := explicit[ev.Name]; ok {
			ec = explicitEC
		} else if wildcard != nil {
			ec = *wildcard
			ec.Name = ev.Name
		} else {
			continue
		}

		schema := buildTableSchema(cc.Name, ev, ec)
		schemas[ev.Name] = schema
	}

	return schemas
}

// buildTableSchema creates a single TableSchema from an event definition and config.
func buildTableSchema(contractName string, ev *abi.EventDef, ec config.EventConfig) *types.TableSchema {
	tableName := strings.ToLower(contractName + "_" + ev.Name)

	// Map ABI event members to columns.
	var columns []types.Column

	// Standard metadata columns.
	columns = append(columns,
		types.Column{Name: "block_number", Type: "uint64"},
		types.Column{Name: "transaction_hash", Type: "string"},
		types.Column{Name: "log_index", Type: "uint64"},
		types.Column{Name: "contract_address", Type: "string"},
		types.Column{Name: "event_name", Type: "string"},
		types.Column{Name: "status", Type: "string"},
	)

	// Event-specific columns from ABI.
	for _, member := range ev.KeyMembers {
		columns = append(columns, types.Column{
			Name: member.Name,
			Type: cairoTypeToColumnType(member.Type),
		})
	}
	for _, member := range ev.DataMembers {
		columns = append(columns, types.Column{
			Name: member.Name,
			Type: cairoTypeToColumnType(member.Type),
		})
	}

	// Determine table type.
	tableType := types.TableTypeLog
	switch ec.Table.Type {
	case "unique":
		tableType = types.TableTypeUnique
	case "aggregation":
		tableType = types.TableTypeAggregation
	}

	// Build aggregate specs.
	var aggregates []types.AggregateSpec
	for _, agg := range ec.Table.Aggregates {
		aggregates = append(aggregates, types.AggregateSpec{
			Column:    agg.Column,
			Operation: agg.Operation,
			Field:     agg.Field,
		})
	}

	return &types.TableSchema{
		Name:       tableName,
		Contract:   contractName,
		Event:      ev.Name,
		TableType:  tableType,
		Columns:    columns,
		UniqueKey:  ec.Table.UniqueKey,
		Aggregates: aggregates,
	}
}

// cairoTypeToColumnType maps a Cairo type definition to a store column type.
func cairoTypeToColumnType(td *abi.TypeDef) string {
	switch td.Kind {
	case abi.CairoFelt252, abi.CairoContractAddress, abi.CairoClassHash:
		return "string"
	case abi.CairoU8, abi.CairoU16, abi.CairoU32, abi.CairoU64:
		return "int64"
	case abi.CairoU128, abi.CairoU256:
		return "string" // Too large for int64.
	case abi.CairoI8, abi.CairoI16, abi.CairoI32, abi.CairoI64:
		return "int64"
	case abi.CairoI128:
		return "string"
	case abi.CairoBool:
		return "bool"
	case abi.CairoByteArray:
		return "string"
	case abi.CairoArray, abi.CairoSpan:
		return "string" // JSON-encoded.
	case abi.CairoStruct:
		return "string" // JSON-encoded.
	case abi.CairoEnum:
		return "string" // JSON-encoded.
	default:
		return "string"
	}
}
