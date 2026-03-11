package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/b-j-roberts/ibis/internal/store"
)

// handleStream serves an SSE endpoint that streams new indexed events in
// real-time. Supports Last-Event-ID for reconnection replay and the same
// query param filters as the REST endpoints.
//
// Endpoint: GET /v1/{contract}/{event}/stream
// Headers:  Content-Type: text/event-stream, Cache-Control: no-cache, Connection: keep-alive
// Format:   id: {block}:{logIndex}\ndata: {json}\n\n
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	contract := r.PathValue("contract")
	event := r.PathValue("event")

	schema := s.lookupSchema(contract, event)
	if schema == nil {
		writeError(w, http.StatusNotFound, "table not found")
		return
	}

	if s.bus == nil {
		writeError(w, http.StatusServiceUnavailable, "event streaming not available")
		return
	}

	// Verify streaming support.
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Parse filters from query params (same as REST endpoints).
	filters, err := parseFiltersFromURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Handle Last-Event-ID replay.
	if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
		s.replayEvents(w, flusher, schema.Name, lastID, filters)
	}

	// Subscribe to the event bus for this table.
	subID, ch := s.bus.Subscribe(schema.Name, filters)
	defer s.bus.Unsubscribe(subID)

	s.logger.Info("SSE client connected",
		"contract", contract,
		"event", event,
		"subscription_id", subID,
	)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("SSE client disconnected",
				"contract", contract,
				"event", event,
				"subscription_id", subID,
			)
			return

		case evt, ok := <-ch:
			if !ok {
				// Channel closed (bus shutdown).
				return
			}
			writeSSEEvent(w, flusher, evt)
		}
	}
}

// replayEvents sends events that were indexed after the Last-Event-ID.
// The ID format is "block:logIndex". Events with block_number > block OR
// (block_number == block AND log_index > logIndex) are replayed.
func (s *Server) replayEvents(w http.ResponseWriter, flusher http.Flusher, table, lastID string, filters []store.Filter) {
	block, logIndex, err := parseEventID(lastID)
	if err != nil {
		s.logger.Warn("invalid Last-Event-ID", "id", lastID, "error", err)
		return
	}

	// Query events after the last seen event.
	replayFilters := append([]store.Filter{}, filters...)
	replayFilters = append(replayFilters, store.Filter{
		Field:    "block_number",
		Operator: "gte",
		Value:    fmt.Sprintf("%d", block),
	})

	q := store.Query{
		Limit:    maxLimit,
		OrderBy:  "block_number",
		OrderDir: store.OrderAsc,
		Filters:  replayFilters,
	}

	events, err := s.store.GetEvents(context.Background(), table, q)
	if err != nil {
		s.logger.Error("SSE replay query failed", "table", table, "error", err)
		return
	}

	for _, evt := range events {
		evtBlock := evt.BlockNumber
		evtLogIndex := evt.LogIndex

		// Skip events at or before the last event ID.
		if evtBlock < block || (evtBlock == block && evtLogIndex <= logIndex) {
			continue
		}

		writeSSEEvent(w, flusher, StreamEvent{
			Table:       table,
			BlockNumber: evtBlock,
			LogIndex:    evtLogIndex,
			Data:        evt.Data,
		})
	}
}

// writeSSEEvent writes a single SSE event to the response.
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, evt StreamEvent) {
	data, err := json.Marshal(evt.Data)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "id: %s\ndata: %s\n\n", evt.EventID(), data)
	flusher.Flush()
}

// parseEventID parses "block:logIndex" into its components.
func parseEventID(id string) (uint64, uint64, error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected block:logIndex format")
	}

	block, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid block number: %w", err)
	}

	logIndex, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid log index: %w", err)
	}

	return block, logIndex, nil
}
