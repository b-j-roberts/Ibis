package engine

import (
	"sort"
	"sync"

	"github.com/b-j-roberts/ibis/internal/store"
)

// PendingTracker stores operations per block number so they can be reverted
// on reorg or pending block replacement. Once a block reaches sufficient
// confirmation depth, its operations are discarded (no longer need revert data).
type PendingTracker struct {
	mu     sync.Mutex
	blocks map[uint64][]store.Operation
}

// NewPendingTracker creates an empty pending tracker.
func NewPendingTracker() *PendingTracker {
	return &PendingTracker{
		blocks: make(map[uint64][]store.Operation),
	}
}

// Track records an operation for the given block. Operations are appended
// in the order they were applied.
func (pt *PendingTracker) Track(blockNumber uint64, op store.Operation) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.blocks[blockNumber] = append(pt.blocks[blockNumber], op)
}

// GetBlock returns the operations recorded for the given block, or nil if
// no operations exist.
func (pt *PendingTracker) GetBlock(blockNumber uint64) []store.Operation {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	ops := pt.blocks[blockNumber]
	if ops == nil {
		return nil
	}
	// Return a copy to prevent external mutation.
	cp := make([]store.Operation, len(ops))
	copy(cp, ops)
	return cp
}

// RemoveBlock discards all tracked operations for the given block.
func (pt *PendingTracker) RemoveBlock(blockNumber uint64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	delete(pt.blocks, blockNumber)
}

// ConfirmUpTo removes all tracked operations for blocks <= confirmBlock.
// These blocks are considered confirmed and their revert data is no longer needed.
func (pt *PendingTracker) ConfirmUpTo(confirmBlock uint64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	for block := range pt.blocks {
		if block <= confirmBlock {
			delete(pt.blocks, block)
		}
	}
}

// HasBlock returns true if the tracker has any operations for the given block.
func (pt *PendingTracker) HasBlock(blockNumber uint64) bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	_, ok := pt.blocks[blockNumber]
	return ok
}

// BlockRange returns the sorted list of pending block numbers.
func (pt *PendingTracker) BlockRange() []uint64 {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	blocks := make([]uint64, 0, len(pt.blocks))
	for b := range pt.blocks {
		blocks = append(blocks, b)
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i] < blocks[j] })
	return blocks
}

// Len returns the number of blocks being tracked.
func (pt *PendingTracker) Len() int {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return len(pt.blocks)
}
