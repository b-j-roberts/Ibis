package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/b-j-roberts/ibis/internal/store"
)

const (
	defaultLimit = 50
	maxLimit     = 500
)

// parseQuery parses Supabase-style query parameters into a store.Query.
// Supports: ?limit=50&offset=0&order=block_number.desc&field=op.value
func parseQuery(r *http.Request) (store.Query, error) {
	q := store.Query{
		Limit:    defaultLimit,
		OrderBy:  "block_number",
		OrderDir: store.OrderDesc,
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 {
			return q, fmt.Errorf("invalid limit: %s", v)
		}
		if limit > maxLimit {
			limit = maxLimit
		}
		q.Limit = limit
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		offset, err := strconv.Atoi(v)
		if err != nil || offset < 0 {
			return q, fmt.Errorf("invalid offset: %s", v)
		}
		q.Offset = offset
	}

	// Parse order: "field.desc" or "field.asc" (default: desc).
	if v := r.URL.Query().Get("order"); v != "" {
		parts := strings.SplitN(v, ".", 2)
		q.OrderBy = parts[0]
		if len(parts) == 2 && parts[1] == "asc" {
			q.OrderDir = store.OrderAsc
		}
	}

	// Parse field filters: ?field=op.value
	filters, err := parseFiltersFromURL(r)
	if err != nil {
		return q, err
	}
	q.Filters = filters

	return q, nil
}

// parseFiltersFromURL extracts filters from URL query params, skipping reserved params.
func parseFiltersFromURL(r *http.Request) ([]store.Filter, error) {
	reserved := map[string]bool{
		"limit": true, "offset": true, "order": true,
	}
	var filters []store.Filter
	for key, values := range r.URL.Query() {
		if reserved[key] || len(values) == 0 {
			continue
		}
		filter, err := parseFilterParam(key, values[0])
		if err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}
	return filters, nil
}

// parseFilterParam parses "op.value" format into a Filter.
// e.g., field "block_number" with value "gte.100000" -> {Field: "block_number", Operator: "gte", Value: "100000"}.
// When no valid operator prefix is found, defaults to "eq" with the raw value.
// This enables simple filters like ?contract_address=0x123 without the eq. prefix.
func parseFilterParam(field, value string) (store.Filter, error) { //nolint:unparam // error kept for future validation
	validOps := map[string]bool{
		"eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true,
	}

	parts := strings.SplitN(value, ".", 2)
	if len(parts) == 2 && validOps[parts[0]] {
		return store.Filter{
			Field:    field,
			Operator: parts[0],
			Value:    parts[1],
		}, nil
	}

	// No valid operator prefix: default to equality filter.
	return store.Filter{
		Field:    field,
		Operator: "eq",
		Value:    value,
	}, nil
}
