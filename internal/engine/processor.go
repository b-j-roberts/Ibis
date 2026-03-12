package engine

import (
	"context"
	"fmt"

	"github.com/NethermindEth/juno/core/felt"

	"github.com/b-j-roberts/ibis/internal/abi"
	"github.com/b-j-roberts/ibis/internal/provider"
	"github.com/b-j-roberts/ibis/internal/store"
)

// processEvent decodes a raw event and writes it to the store.
// Pipeline: match selector -> find ABI definition -> decode fields ->
// generate operation -> apply to store -> track in pending tracker.
func (e *Engine) processEvent(ctx context.Context, raw *provider.RawEvent) error {
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
	decoded["timestamp"] = uint64(0) // Populated from block data when available.
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

	// Update per-contract cursor.
	if err := e.store.SetCursor(ctx, cs.config.Name, raw.BlockNumber); err != nil {
		return fmt.Errorf("setting cursor for %s: %w", cs.config.Name, err)
	}

	// Promote confirmed blocks.
	e.confirmBlocks(raw.BlockNumber)

	// Notify SSE subscribers.
	if e.onEvent != nil {
		e.onEvent(cs.config.Name, eventDef.Name, schema.Name, raw.BlockNumber, logIndex, decoded)
	}

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
