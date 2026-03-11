package schema

import (
	"fmt"

	"github.com/b-j-roberts/ibis/internal/types"
)

// BadgerKeyPatterns defines the key prefix patterns for a table in BadgerDB.
type BadgerKeyPatterns struct {
	// PrimaryPrefix is the key pattern for ascending event lookups.
	// Format: evt:{table}:{block}:{logIndex}
	PrimaryPrefix string

	// ReversePrefix is the key pattern for descending event lookups.
	// Format: rev:{table}:{invertedBlock}:{logIndex}
	ReversePrefix string

	// UniquePrefix is the key pattern for unique entry lookups (unique tables only).
	// Format: unq:{table}:{uniqueKey}
	UniquePrefix string

	// AggregationKey is the key for aggregation data (aggregation tables only).
	// Format: agg:{table}
	AggregationKey string

	// SchemaKey is the key for storing the table schema definition.
	// Format: schema:{table}
	SchemaKey string
}

// GenerateBadgerKeyPatterns generates BadgerDB key prefix patterns from a TableSchema.
func GenerateBadgerKeyPatterns(schema *types.TableSchema) BadgerKeyPatterns {
	patterns := BadgerKeyPatterns{
		PrimaryPrefix: fmt.Sprintf("evt:%s:", schema.Name),
		ReversePrefix: fmt.Sprintf("rev:%s:", schema.Name),
		SchemaKey:     fmt.Sprintf("schema:%s", schema.Name),
	}

	if schema.TableType == types.TableTypeUnique && schema.UniqueKey != "" {
		patterns.UniquePrefix = fmt.Sprintf("unq:%s:", schema.Name)
	}

	if schema.TableType == types.TableTypeAggregation {
		patterns.AggregationKey = fmt.Sprintf("agg:%s", schema.Name)
	}

	return patterns
}
