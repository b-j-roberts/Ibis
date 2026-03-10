package store

// OpType represents the type of database operation.
type OpType int

const (
	OpInsert OpType = iota
	OpUpdate
	OpDelete
)

func (o OpType) String() string {
	switch o {
	case OpInsert:
		return "INSERT"
	case OpUpdate:
		return "UPDATE"
	case OpDelete:
		return "DELETE"
	default:
		return "UNKNOWN"
	}
}

// Operation is a reversible database operation. Every write is recorded as an
// (add, revert) pair. The Prev field stores previous data needed to undo the
// operation during reorgs or pending block replacements.
type Operation struct {
	Type        OpType
	Table       string
	Key         string         // Primary key for the record
	Data        map[string]any // For Insert/Update: the new data
	Prev        map[string]any // For Update revert: the previous data; for Delete revert: the deleted data
	BlockNumber uint64
	LogIndex    uint64
}

// InverseOp returns the operation that reverses this one.
func (op Operation) InverseOp() Operation {
	switch op.Type {
	case OpInsert:
		return Operation{
			Type:        OpDelete,
			Table:       op.Table,
			Key:         op.Key,
			Data:        op.Data,
			BlockNumber: op.BlockNumber,
			LogIndex:    op.LogIndex,
		}
	case OpUpdate:
		return Operation{
			Type:        OpUpdate,
			Table:       op.Table,
			Key:         op.Key,
			Data:        op.Prev,
			Prev:        op.Data,
			BlockNumber: op.BlockNumber,
			LogIndex:    op.LogIndex,
		}
	case OpDelete:
		return Operation{
			Type:        OpInsert,
			Table:       op.Table,
			Key:         op.Key,
			Data:        op.Prev,
			BlockNumber: op.BlockNumber,
			LogIndex:    op.LogIndex,
		}
	default:
		return op
	}
}
