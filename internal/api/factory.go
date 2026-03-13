package api

import (
	"net/http"

	"github.com/b-j-roberts/ibis/internal/engine"
)

// handleFactoryChildren handles GET /v1/{factory}/children.
// Lists all discovered child contracts for a factory with their metadata
// (address, deployment block, sync status, factory event data).
// Metadata fields from the factory event (e.g., token0, token1) are promoted
// to top-level fields and can be filtered via query params.
func (s *Server) handleFactoryChildren(w http.ResponseWriter, r *http.Request) {
	factory := r.PathValue("factory")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	if !s.engine.IsFactory(factory) {
		writeError(w, http.StatusNotFound, "factory not found: "+factory)
		return
	}

	children := s.engine.FactoryChildren(factory)

	// Parse optional metadata filters (e.g., ?token0=eq.0x...).
	filters, err := parseFiltersFromURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	data := make([]map[string]any, 0, len(children))
	for _, child := range children {
		entry := factoryChildToMap(&child)
		if len(filters) > 0 && !matchFilters(entry, filters) {
			continue
		}
		data = append(data, entry)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  data,
		"count": len(data),
	})
}

// handleFactoryChildCount handles GET /v1/{factory}/children/count.
func (s *Server) handleFactoryChildCount(w http.ResponseWriter, r *http.Request) {
	factory := r.PathValue("factory")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	if !s.engine.IsFactory(factory) {
		writeError(w, http.StatusNotFound, "factory not found: "+factory)
		return
	}

	children := s.engine.FactoryChildren(factory)

	// Apply optional metadata filters.
	filters, err := parseFiltersFromURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	count := len(children)
	if len(filters) > 0 {
		count = 0
		for _, child := range children {
			entry := factoryChildToMap(&child)
			if matchFilters(entry, filters) {
				count++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"count": count})
}

// factoryChildToMap converts a ContractInfo into a flat map with factory
// metadata fields promoted to top-level for queryability.
func factoryChildToMap(info *engine.ContractInfo) map[string]any {
	m := map[string]any{
		"name":             info.Name,
		"address":          info.Address,
		"deployment_block": info.StartBlock,
		"current_block":    info.CurrentBlock,
		"status":           info.Status,
		"events":           info.Events,
	}
	for k, v := range info.FactoryMeta {
		m[k] = v
	}
	return m
}
