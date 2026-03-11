package api

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/b-j-roberts/ibis/internal/store"
)

// StreamEvent is an indexed event published to SSE subscribers.
type StreamEvent struct {
	Table       string         // Table name (e.g., "mytoken_transfer")
	Contract    string         // Contract name
	Event       string         // Event name
	BlockNumber uint64
	LogIndex    uint64
	Data        map[string]any
}

// EventID returns the SSE event ID in "block:logIndex" format.
func (e StreamEvent) EventID() string {
	return fmt.Sprintf("%d:%d", e.BlockNumber, e.LogIndex)
}

// EventBus is a channel-based in-memory pub/sub for streaming indexed events
// to SSE clients. The engine publishes events after writing to the store, and
// SSE handlers subscribe to receive matching events.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[uint64]*subscriber
	nextID      atomic.Uint64
	closed      bool
}

type subscriber struct {
	ch      chan StreamEvent
	table   string         // filter: only events for this table
	filters []store.Filter // additional field filters
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[uint64]*subscriber),
	}
}

// Publish sends an event to all matching subscribers. Non-blocking: if a
// subscriber's channel is full, the event is dropped for that subscriber.
func (b *EventBus) Publish(event StreamEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	for _, sub := range b.subscribers {
		if sub.table != "" && sub.table != event.Table {
			continue
		}
		if !matchFilters(event.Data, sub.filters) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			// Drop event for slow subscribers.
		}
	}
}

// Subscribe registers a new subscriber filtered by table name and optional
// field filters. Returns a subscription ID and a channel of events.
func (b *EventBus) Subscribe(table string, filters []store.Filter) (uint64, <-chan StreamEvent) {
	id := b.nextID.Add(1)
	ch := make(chan StreamEvent, 64)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.subscribers[id] = &subscriber{
		ch:      ch,
		table:   table,
		filters: filters,
	}
	return id, ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *EventBus) Unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if sub, ok := b.subscribers[id]; ok {
		close(sub.ch)
		delete(b.subscribers, id)
	}
}

// Close shuts down the event bus and closes all subscriber channels.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	for id, sub := range b.subscribers {
		close(sub.ch)
		delete(b.subscribers, id)
	}
}

// matchFilters checks whether event data matches all filters.
func matchFilters(data map[string]any, filters []store.Filter) bool {
	for _, f := range filters {
		val, ok := data[f.Field]
		if !ok {
			return false
		}
		if !matchFilter(val, f) {
			return false
		}
	}
	return true
}

// matchFilter checks a single filter against a value.
func matchFilter(val any, f store.Filter) bool {
	valStr := fmt.Sprintf("%v", val)
	filterVal := fmt.Sprintf("%v", f.Value)

	switch f.Operator {
	case "eq":
		return valStr == filterVal
	case "neq":
		return valStr != filterVal
	default:
		// For gt/gte/lt/lte, only applicable to numeric fields.
		// For SSE streaming, eq/neq are the most common filter operations.
		// The full comparison is done by the store on replay queries.
		return true
	}
}
