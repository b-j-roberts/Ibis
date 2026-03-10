package store

// OrderDirection specifies sort direction.
type OrderDirection int

const (
	OrderAsc OrderDirection = iota
	OrderDesc
)

// Filter is a field-level query filter.
type Filter struct {
	Field    string
	Operator string // eq, neq, gt, gte, lt, lte
	Value    any
}

// Query defines pagination, ordering, and filtering for store reads.
type Query struct {
	Limit    int
	Offset   int
	OrderBy  string
	OrderDir OrderDirection
	Filters  []Filter
}

// AggResult holds the result of an aggregation query.
type AggResult struct {
	Values map[string]any // column name -> aggregated value
}
