package types

// BlockStatus represents the confirmation status of a block.
type BlockStatus int

const (
	BlockStatusPreConfirmed BlockStatus = iota
	BlockStatusAcceptedL2
	BlockStatusAcceptedL1
)

func (s BlockStatus) String() string {
	switch s {
	case BlockStatusPreConfirmed:
		return "PRE_CONFIRMED"
	case BlockStatusAcceptedL2:
		return "ACCEPTED_L2"
	case BlockStatusAcceptedL1:
		return "ACCEPTED_L1"
	default:
		return "UNKNOWN"
	}
}

// TableType defines the behavior of a table.
type TableType int

const (
	TableTypeLog         TableType = iota // Append-only event log
	TableTypeUnique                       // Last-write-wins by unique key
	TableTypeAggregation                  // Auto-computed aggregates
)

func (t TableType) String() string {
	switch t {
	case TableTypeLog:
		return "log"
	case TableTypeUnique:
		return "unique"
	case TableTypeAggregation:
		return "aggregation"
	default:
		return "unknown"
	}
}

// Column defines a table column derived from an ABI event member.
type Column struct {
	Name     string
	Type     string // Go type representation: "string", "int64", "bool", "[]byte"
	Nullable bool
}

// AggregateSpec defines an aggregation operation on a column.
type AggregateSpec struct {
	Column    string // Output column name
	Operation string // sum, count, avg
	Field     string // Source event field
}

// TableSchema defines an ABI-derived table.
type TableSchema struct {
	Name       string
	Contract   string
	Event      string
	TableType  TableType
	Columns    []Column
	UniqueKey  string          // For unique tables
	Aggregates []AggregateSpec // For aggregation tables
}

// IndexedEvent is a decoded, stored event.
type IndexedEvent struct {
	ID              string
	ContractAddress string
	EventName       string
	BlockNumber     uint64
	BlockHash       string
	TransactionHash string
	LogIndex        uint64
	Data            map[string]any
	Timestamp       uint64
	Status          BlockStatus
}
