package api

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/b-j-roberts/ibis/internal/engine"
	"github.com/b-j-roberts/ibis/internal/store"
)

// factoryChildrenResponse is the JSON envelope for paginated factory children.
type factoryChildrenResponse struct {
	Data   []map[string]any `json:"data"`
	Count  int              `json:"count"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

// handleFactoryChildren handles GET /v1/{factory}/children.
// Lists all discovered child contracts for a factory with their metadata
// (address, deployment block, sync status, factory event data).
// Metadata fields from the factory event (e.g., token0, token1) are promoted
// to top-level fields and can be filtered via query params.
// Supports pagination (?limit=&offset=) and sorting (?order=field.asc|desc).
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

	// Parse query params: limit, offset, order, and filters.
	q, err := parseQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Override default order: factory children default to deployment_block.desc.
	if r.URL.Query().Get("order") == "" {
		q.OrderBy = "deployment_block"
		q.OrderDir = store.OrderDesc
	}

	// Filter children by metadata.
	data := make([]map[string]any, 0, len(children))
	for _, child := range children {
		entry := factoryChildToMap(&child)
		if len(q.Filters) > 0 && !matchFilters(entry, q.Filters) {
			continue
		}
		data = append(data, entry)
	}

	// Sort in-memory.
	sortFactoryChildren(data, q.OrderBy, q.OrderDir)

	total := len(data)

	// Apply offset/limit.
	if q.Offset >= len(data) {
		data = []map[string]any{}
	} else {
		data = data[q.Offset:]
		if q.Limit < len(data) {
			data = data[:q.Limit]
		}
	}

	writeJSON(w, http.StatusOK, factoryChildrenResponse{
		Data:   data,
		Count:  len(data),
		Total:  total,
		Limit:  q.Limit,
		Offset: q.Offset,
	})
}

// sortFactoryChildren sorts a slice of factory child maps in-place.
// Known numeric fields (deployment_block, current_block, events) use typed
// comparison; all others compare as strings via fmt.Sprint.
func sortFactoryChildren(data []map[string]any, field string, dir store.OrderDirection) {
	numericFields := map[string]bool{
		"deployment_block": true,
		"current_block":    true,
		"events":           true,
	}

	slices.SortStableFunc(data, func(a, b map[string]any) int {
		va, vb := a[field], b[field]

		var cmp int
		if numericFields[field] {
			cmp = compareNumeric(va, vb)
		} else {
			sa := fmt.Sprint(va)
			sb := fmt.Sprint(vb)
			cmp = strings.Compare(sa, sb)
		}

		if dir == store.OrderDesc {
			return -cmp
		}
		return cmp
	})
}

// compareNumeric compares two values as uint64 (with fallback to float64 for JSON numbers).
func compareNumeric(a, b any) int {
	fa := toUint64(a)
	fb := toUint64(b)
	if fa < fb {
		return -1
	}
	if fa > fb {
		return 1
	}
	return 0
}

// toUint64 converts a value to uint64, handling common Go numeric types.
func toUint64(v any) uint64 {
	switch n := v.(type) {
	case uint64:
		return n
	case int:
		return uint64(n)
	case int64:
		return uint64(n)
	case float64:
		return uint64(n)
	default:
		return 0
	}
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
