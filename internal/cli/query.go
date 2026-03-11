package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/types"
)

var (
	queryLimit     int
	queryOffset    int
	queryOrder     string
	queryFilters   []string
	queryUnique    bool
	queryAggregate bool
	queryFormat    string
	queryList      bool

	// testCreateStoreOverride is a test hook. When non-nil, runQuery uses the
	// returned store instead of opening one from config.
	testCreateStoreOverride func() store.Store
)

var queryCmd = &cobra.Command{
	Use:   "query [contract] [event]",
	Short: "Query indexed data from the terminal",
	Long: `Query indexed event data directly from the configured database
without needing the API server running.

Examples:
  ibis query MyContract Transfer
  ibis query MyContract Transfer --limit 10 --order block_number.asc
  ibis query MyContract Transfer --filter "block_number=gte.100"
  ibis query MyContract LeaderboardUpdate --unique
  ibis query MyContract VolumeUpdate --aggregate
  ibis query --list`,
	Args: cobra.MaximumNArgs(2),
	RunE: runQuery,
}

func init() {
	queryCmd.Flags().IntVar(&queryLimit, "limit", 50, "maximum number of results")
	queryCmd.Flags().IntVar(&queryOffset, "offset", 0, "number of results to skip")
	queryCmd.Flags().StringVar(&queryOrder, "order", "block_number.desc", "ordering (field.asc or field.desc)")
	queryCmd.Flags().StringArrayVar(&queryFilters, "filter", nil, "field filter (field=value or field=op.value)")
	queryCmd.Flags().BoolVar(&queryUnique, "unique", false, "query unique table entries")
	queryCmd.Flags().BoolVar(&queryAggregate, "aggregate", false, "query aggregation results")
	queryCmd.Flags().StringVar(&queryFormat, "format", "json", "output format: json, table, csv")
	queryCmd.Flags().BoolVar(&queryList, "list", false, "list all available tables/events")
}

func runQuery(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if queryList {
		listTables(cmd, cfg)
		return nil
	}

	if len(args) < 2 {
		return fmt.Errorf("usage: ibis query <contract> <event>\n  use --list to see available tables")
	}

	contractName := args[0]
	eventName := args[1]
	tableName := strings.ToLower(contractName + "_" + eventName)

	// Connect to store.
	var st store.Store
	if testCreateStoreOverride != nil {
		st = testCreateStoreOverride()
	} else {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))
		var storeErr error
		st, storeErr = createStore(cfg, logger)
		if storeErr != nil {
			return fmt.Errorf("opening database: %w", storeErr)
		}
	}
	defer st.Close()

	ctx := context.Background()

	// Aggregation query.
	if queryAggregate {
		result, err := st.GetAggregation(ctx, tableName, store.Query{})
		if err != nil {
			return fmt.Errorf("aggregation query failed: %w", err)
		}
		return outputAggregation(cmd, result)
	}

	// Build query from flags.
	q, err := buildQuery()
	if err != nil {
		return err
	}

	// Execute query.
	var events []types.IndexedEvent
	if queryUnique {
		events, err = st.GetUniqueEvents(ctx, tableName, q)
	} else {
		events, err = st.GetEvents(ctx, tableName, q)
	}
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	return outputEvents(cmd, events)
}

func listTables(cmd *cobra.Command, cfg *config.Config) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Available tables:")
	fmt.Fprintln(out)
	for _, c := range cfg.Contracts {
		fmt.Fprintf(out, "  %s (%s)\n", c.Name, c.Address)
		for _, ev := range c.Events {
			if ev.Name == "*" {
				fmt.Fprintf(out, "    * (all ABI events)  type=%s\n", ev.Table.Type)
			} else {
				tableName := strings.ToLower(c.Name + "_" + ev.Name)
				fmt.Fprintf(out, "    %-30s  type=%-12s  table=%s\n", ev.Name, ev.Table.Type, tableName)
			}
		}
		fmt.Fprintln(out)
	}
}

func buildQuery() (store.Query, error) {
	q := store.Query{
		Limit:    queryLimit,
		Offset:   queryOffset,
		OrderBy:  "block_number",
		OrderDir: store.OrderDesc,
	}

	// Parse order flag (e.g., "block_number.desc").
	if queryOrder != "" {
		parts := strings.SplitN(queryOrder, ".", 2)
		q.OrderBy = parts[0]
		if len(parts) == 2 {
			switch parts[1] {
			case "asc":
				q.OrderDir = store.OrderAsc
			case "desc":
				q.OrderDir = store.OrderDesc
			default:
				return q, fmt.Errorf("invalid order direction: %s (use asc or desc)", parts[1])
			}
		}
	}

	// Parse filters (e.g., "block_number=gte.100" or "status=ACCEPTED_L2").
	for _, f := range queryFilters {
		filter, err := parseFilterFlag(f)
		if err != nil {
			return q, err
		}
		q.Filters = append(q.Filters, filter)
	}

	return q, nil
}

// parseFilterFlag parses a --filter flag value like "field=op.value" or "field=value".
// When no operator prefix is given, defaults to "eq".
func parseFilterFlag(s string) (store.Filter, error) {
	eqIdx := strings.Index(s, "=")
	if eqIdx < 1 {
		return store.Filter{}, fmt.Errorf("invalid filter format: %q (expected field=value or field=op.value)", s)
	}

	field := s[:eqIdx]
	rest := s[eqIdx+1:]

	// Check for operator prefix (e.g., "gte.100").
	operator := "eq"
	value := rest
	for _, op := range []string{"neq", "gte", "lte", "gt", "lt", "eq"} {
		if strings.HasPrefix(rest, op+".") {
			operator = op
			value = rest[len(op)+1:]
			break
		}
	}

	return store.Filter{
		Field:    field,
		Operator: operator,
		Value:    value,
	}, nil
}

func outputEvents(cmd *cobra.Command, events []types.IndexedEvent) error {
	out := cmd.OutOrStdout()

	if len(events) == 0 {
		fmt.Fprintln(out, "No results found.")
		return nil
	}

	switch queryFormat {
	case "json":
		return outputJSON(out, events)
	case "table":
		return outputTable(out, events)
	case "csv":
		return outputCSV(out, events)
	default:
		return fmt.Errorf("unknown format: %s (use json, table, or csv)", queryFormat)
	}
}

func outputJSON(out io.Writer, events []types.IndexedEvent) error {
	data := make([]map[string]any, len(events))
	for i, evt := range events {
		data[i] = evt.Data
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func outputTable(out io.Writer, events []types.IndexedEvent) error {
	cols := collectColumns(events)
	if len(cols) == 0 {
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)

	// Header row.
	fmt.Fprintln(tw, strings.Join(cols, "\t"))

	// Separator row.
	seps := make([]string, len(cols))
	for i, col := range cols {
		seps[i] = strings.Repeat("-", len(col))
	}
	fmt.Fprintln(tw, strings.Join(seps, "\t"))

	// Data rows.
	for _, evt := range events {
		vals := make([]string, len(cols))
		for i, col := range cols {
			vals[i] = formatValue(evt.Data[col])
		}
		fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}

	return tw.Flush()
}

func outputCSV(out io.Writer, events []types.IndexedEvent) error {
	cols := collectColumns(events)
	if len(cols) == 0 {
		return nil
	}

	w := csv.NewWriter(out)

	// Header row.
	if err := w.Write(cols); err != nil {
		return err
	}

	// Data rows.
	for _, evt := range events {
		row := make([]string, len(cols))
		for i, col := range cols {
			row[i] = formatValue(evt.Data[col])
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func outputAggregation(cmd *cobra.Command, result store.AggResult) error {
	out := cmd.OutOrStdout()

	switch queryFormat {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result.Values)
	case "table", "csv":
		// For table/csv, show key-value pairs.
		keys := make([]string, 0, len(result.Values))
		for k := range result.Values {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "COLUMN\tVALUE")
		fmt.Fprintln(tw, "------\t-----")
		for _, k := range keys {
			fmt.Fprintf(tw, "%s\t%s\n", k, formatValue(result.Values[k]))
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown format: %s (use json, table, or csv)", queryFormat)
	}
}

// collectColumns gathers all unique column names from events, with metadata
// columns first in a fixed order, then remaining columns sorted alphabetically.
func collectColumns(events []types.IndexedEvent) []string {
	// Metadata columns in preferred display order.
	metaOrder := []string{
		"block_number", "log_index", "transaction_hash",
		"contract_address", "event_name", "status", "timestamp",
	}
	metaSet := make(map[string]bool, len(metaOrder))
	for _, m := range metaOrder {
		metaSet[m] = true
	}

	// Collect all unique keys from event data.
	seen := make(map[string]bool)
	var extra []string
	for _, evt := range events {
		for k := range evt.Data {
			if !seen[k] {
				seen[k] = true
				if !metaSet[k] {
					extra = append(extra, k)
				}
			}
		}
	}
	sort.Strings(extra)

	// Build final column list: metadata first (if present), then extra.
	var cols []string
	for _, m := range metaOrder {
		if seen[m] {
			cols = append(cols, m)
		}
	}
	cols = append(cols, extra...)
	return cols
}

// formatValue converts an arbitrary value to a display string.
func formatValue(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case map[string]any, []any:
		b, _ := json.Marshal(val)
		return string(b)
	default:
		return fmt.Sprintf("%v", val)
	}
}
