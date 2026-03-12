package engine

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/NethermindEth/juno/core/felt"

	"github.com/b-j-roberts/ibis/internal/abi"
	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/provider"
	"github.com/b-j-roberts/ibis/internal/schema"
	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/store/memory"
	"github.com/b-j-roberts/ibis/internal/types"
)

// --- Helpers ---

// testEventDef creates a simple event definition with one key and one data member.
func testEventDef(name string) *abi.EventDef {
	return &abi.EventDef{
		Name:     name,
		FullName: "test::" + name,
		Selector: abi.ComputeSelector(name),
		KeyMembers: []abi.FieldDef{
			{Name: "sender", Type: &abi.TypeDef{Kind: abi.CairoContractAddress, Name: "ContractAddress"}},
		},
		DataMembers: []abi.FieldDef{
			{Name: "amount", Type: &abi.TypeDef{Kind: abi.CairoU64, Name: "u64"}},
		},
	}
}

// testContractState creates a contractState for testing with the given events.
func testContractState(address *felt.Felt, contractName string, events []*abi.EventDef, tableType types.TableType) *contractState {
	parsedABI := &abi.ABI{
		Types:  make(map[string]*abi.TypeDef),
		Events: events,
	}
	registry := abi.NewEventRegistry(parsedABI)

	schemas := make(map[string]*types.TableSchema)
	for _, ev := range events {
		schemas[ev.Name] = &types.TableSchema{
			Name:      contractName + "_" + ev.Name,
			Contract:  contractName,
			Event:     ev.Name,
			TableType: tableType,
			Columns: []types.Column{
				{Name: "block_number", Type: "uint64"},
				{Name: "transaction_hash", Type: "string"},
				{Name: "log_index", Type: "uint64"},
				{Name: "timestamp", Type: "uint64"},
				{Name: "contract_address", Type: "string"},
				{Name: "event_name", Type: "string"},
				{Name: "status", Type: "string"},
				{Name: "sender", Type: "string"},
				{Name: "amount", Type: "int64"},
			},
		}
	}

	return &contractState{
		config: config.ContractConfig{
			Name:    contractName,
			Address: address.String(),
		},
		address:  address,
		abi:      parsedABI,
		registry: registry,
		schemas:  schemas,
	}
}

// makeRawEvent creates a RawEvent for testing.
func makeRawEvent(selector, contractAddr *felt.Felt, blockNumber uint64, senderFelt, amountFelt *felt.Felt) provider.RawEvent {
	txHash := new(felt.Felt).SetUint64(blockNumber*1000 + 1)
	blockHash := new(felt.Felt).SetUint64(blockNumber * 100)
	return provider.RawEvent{
		BlockNumber:     blockNumber,
		BlockHash:       blockHash,
		TransactionHash: txHash,
		ContractAddress: contractAddr,
		Keys:            []*felt.Felt{selector, senderFelt},
		Data:            []*felt.Felt{amountFelt},
		FinalityStatus:  "ACCEPTED_ON_L2",
	}
}

// --- PendingTracker Tests ---

func TestPendingTracker_TrackAndRetrieve(t *testing.T) {
	pt := NewPendingTracker()

	op1 := store.Operation{Type: store.OpInsert, Table: "test", Key: "1:0", BlockNumber: 1}
	op2 := store.Operation{Type: store.OpInsert, Table: "test", Key: "1:1", BlockNumber: 1}
	op3 := store.Operation{Type: store.OpInsert, Table: "test", Key: "2:0", BlockNumber: 2}

	pt.Track(1, op1)
	pt.Track(1, op2)
	pt.Track(2, op3)

	if !pt.HasBlock(1) {
		t.Fatal("expected block 1 to be tracked")
	}
	if !pt.HasBlock(2) {
		t.Fatal("expected block 2 to be tracked")
	}
	if pt.HasBlock(3) {
		t.Fatal("expected block 3 to not be tracked")
	}

	ops := pt.GetBlock(1)
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops for block 1, got %d", len(ops))
	}

	if pt.Len() != 2 {
		t.Fatalf("expected 2 pending blocks, got %d", pt.Len())
	}
}

func TestPendingTracker_ConfirmUpTo(t *testing.T) {
	pt := NewPendingTracker()

	for i := uint64(1); i <= 5; i++ {
		pt.Track(i, store.Operation{Type: store.OpInsert, Table: "test", Key: "k", BlockNumber: i})
	}

	pt.ConfirmUpTo(3)

	if pt.HasBlock(1) || pt.HasBlock(2) || pt.HasBlock(3) {
		t.Fatal("blocks 1-3 should be confirmed and removed")
	}
	if !pt.HasBlock(4) || !pt.HasBlock(5) {
		t.Fatal("blocks 4-5 should still be pending")
	}
}

func TestPendingTracker_BlockRange(t *testing.T) {
	pt := NewPendingTracker()
	pt.Track(5, store.Operation{})
	pt.Track(2, store.Operation{})
	pt.Track(8, store.Operation{})

	blocks := pt.BlockRange()
	if len(blocks) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(blocks))
	}
	if blocks[0] != 2 || blocks[1] != 5 || blocks[2] != 8 {
		t.Fatalf("expected sorted [2,5,8], got %v", blocks)
	}
}

func TestPendingTracker_RemoveBlock(t *testing.T) {
	pt := NewPendingTracker()
	pt.Track(1, store.Operation{})
	pt.Track(2, store.Operation{})

	pt.RemoveBlock(1)

	if pt.HasBlock(1) {
		t.Fatal("block 1 should be removed")
	}
	if !pt.HasBlock(2) {
		t.Fatal("block 2 should still exist")
	}
}

// --- ProcessEvent Tests ---

func TestProcessEvent_MatchAndDecode(t *testing.T) {
	st := memory.New()
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	eventDef := testEventDef("Transfer")
	cs := testContractState(contractAddr, "mytoken", []*abi.EventDef{eventDef}, types.TableTypeLog)

	// Create table in store.
	ctx := context.Background()
	if err := st.CreateTable(ctx, cs.schemas["Transfer"]); err != nil {
		t.Fatal(err)
	}

	e := &Engine{
		store:        st,
		logger:       noopLogger(),
		pending:      NewPendingTracker(),
		logIndices:   make(map[uint64]uint64),
		contracts:    []*contractState{cs},
		confirmDepth: DefaultConfirmationDepth,
	}

	sender := new(felt.Felt).SetUint64(0xDEAD)
	amount := new(felt.Felt).SetUint64(1000)
	raw := makeRawEvent(eventDef.Selector, contractAddr, 100, sender, amount)

	if err := e.processEvent(ctx, &raw); err != nil {
		t.Fatalf("processEvent failed: %v", err)
	}

	// Verify event was stored.
	events, err := st.GetEvents(ctx, "mytoken_Transfer", store.Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].BlockNumber != 100 {
		t.Fatalf("expected block 100, got %d", events[0].BlockNumber)
	}
	if events[0].Data["sender"] != sender.String() {
		t.Fatalf("expected sender %s, got %v", sender.String(), events[0].Data["sender"])
	}

	// Verify per-contract cursor was updated.
	cursor, _ := st.GetCursor(ctx, "mytoken")
	if cursor != 100 {
		t.Fatalf("expected cursor 100, got %d", cursor)
	}

	// Verify pending tracker has the operation.
	if !e.pending.HasBlock(100) {
		t.Fatal("expected block 100 in pending tracker")
	}
}

func TestProcessEvent_SkipsUnknownContract(t *testing.T) {
	st := memory.New()
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	unknownAddr := new(felt.Felt).SetUint64(0xDEF)
	eventDef := testEventDef("Transfer")
	cs := testContractState(contractAddr, "mytoken", []*abi.EventDef{eventDef}, types.TableTypeLog)

	e := &Engine{
		store:        st,
		logger:       noopLogger(),
		pending:      NewPendingTracker(),
		logIndices:   make(map[uint64]uint64),
		contracts:    []*contractState{cs},
		confirmDepth: DefaultConfirmationDepth,
	}

	raw := makeRawEvent(eventDef.Selector, unknownAddr, 100, new(felt.Felt), new(felt.Felt))
	if err := e.processEvent(context.Background(), &raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No events should be stored.
	events, _ := st.GetEvents(context.Background(), "mytoken_Transfer", store.Query{Limit: 10})
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestProcessEvent_SkipsUnknownSelector(t *testing.T) {
	st := memory.New()
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	eventDef := testEventDef("Transfer")
	cs := testContractState(contractAddr, "mytoken", []*abi.EventDef{eventDef}, types.TableTypeLog)

	e := &Engine{
		store:        st,
		logger:       noopLogger(),
		pending:      NewPendingTracker(),
		logIndices:   make(map[uint64]uint64),
		contracts:    []*contractState{cs},
		confirmDepth: DefaultConfirmationDepth,
	}

	unknownSelector := new(felt.Felt).SetUint64(0x999)
	raw := makeRawEvent(unknownSelector, contractAddr, 100, new(felt.Felt), new(felt.Felt))
	if err := e.processEvent(context.Background(), &raw); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessEvent_MultipleEventsInBlock(t *testing.T) {
	st := memory.New()
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	eventDef := testEventDef("Transfer")
	cs := testContractState(contractAddr, "mytoken", []*abi.EventDef{eventDef}, types.TableTypeLog)

	ctx := context.Background()
	if err := st.CreateTable(ctx, cs.schemas["Transfer"]); err != nil {
		t.Fatal(err)
	}

	e := &Engine{
		store:        st,
		logger:       noopLogger(),
		pending:      NewPendingTracker(),
		logIndices:   make(map[uint64]uint64),
		contracts:    []*contractState{cs},
		confirmDepth: DefaultConfirmationDepth,
	}

	// Process 3 events in the same block.
	for i := uint64(0); i < 3; i++ {
		sender := new(felt.Felt).SetUint64(i + 1)
		amount := new(felt.Felt).SetUint64((i + 1) * 100)
		raw := makeRawEvent(eventDef.Selector, contractAddr, 50, sender, amount)
		if err := e.processEvent(ctx, &raw); err != nil {
			t.Fatalf("processEvent %d failed: %v", i, err)
		}
	}

	events, _ := st.GetEvents(ctx, "mytoken_Transfer", store.Query{Limit: 10})
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify log indices are unique and sequential.
	indices := make(map[uint64]bool)
	for _, ev := range events {
		indices[ev.LogIndex] = true
	}
	if len(indices) != 3 {
		t.Fatal("expected 3 unique log indices")
	}
}

// --- Reorg Tests ---

func TestHandleReorg_RevertsBlockOps(t *testing.T) {
	st := memory.New()
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	eventDef := testEventDef("Transfer")
	cs := testContractState(contractAddr, "mytoken", []*abi.EventDef{eventDef}, types.TableTypeLog)

	ctx := context.Background()
	if err := st.CreateTable(ctx, cs.schemas["Transfer"]); err != nil {
		t.Fatal(err)
	}

	e := &Engine{
		store:        st,
		logger:       noopLogger(),
		pending:      NewPendingTracker(),
		logIndices:   make(map[uint64]uint64),
		contracts:    []*contractState{cs},
		confirmDepth: 100, // Large depth so nothing gets confirmed.
	}

	// Index events in blocks 10, 11, 12.
	for block := uint64(10); block <= 12; block++ {
		sender := new(felt.Felt).SetUint64(block)
		amount := new(felt.Felt).SetUint64(block * 100)
		raw := makeRawEvent(eventDef.Selector, contractAddr, block, sender, amount)
		if err := e.processEvent(ctx, &raw); err != nil {
			t.Fatalf("processEvent block %d: %v", block, err)
		}
	}

	// Verify all 3 events stored.
	events, _ := st.GetEvents(ctx, "mytoken_Transfer", store.Query{Limit: 10})
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Simulate reorg: blocks 11-12 are orphaned.
	reorg := provider.ReorgNotification{StartBlock: 11, EndBlock: 12}
	if err := e.handleReorg(ctx, reorg); err != nil {
		t.Fatalf("handleReorg failed: %v", err)
	}

	// Only block 10 event should remain.
	events, _ = st.GetEvents(ctx, "mytoken_Transfer", store.Query{Limit: 10})
	if len(events) != 1 {
		t.Fatalf("expected 1 event after reorg, got %d", len(events))
	}
	if events[0].BlockNumber != 10 {
		t.Fatalf("expected remaining event from block 10, got %d", events[0].BlockNumber)
	}

	// Per-contract cursor should be reset to 10 (startBlock - 1).
	cursor, _ := st.GetCursor(ctx, "mytoken")
	if cursor != 10 {
		t.Fatalf("expected cursor 10 after reorg, got %d", cursor)
	}

	// Pending tracker should no longer have blocks 11, 12.
	if e.pending.HasBlock(11) || e.pending.HasBlock(12) {
		t.Fatal("orphaned blocks should be removed from pending tracker")
	}
	if !e.pending.HasBlock(10) {
		t.Fatal("block 10 should still be in pending tracker")
	}
}

func TestHandleReorg_FullReorg(t *testing.T) {
	st := memory.New()
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	eventDef := testEventDef("Transfer")
	cs := testContractState(contractAddr, "mytoken", []*abi.EventDef{eventDef}, types.TableTypeLog)

	ctx := context.Background()
	if err := st.CreateTable(ctx, cs.schemas["Transfer"]); err != nil {
		t.Fatal(err)
	}

	e := &Engine{
		store:        st,
		logger:       noopLogger(),
		pending:      NewPendingTracker(),
		logIndices:   make(map[uint64]uint64),
		contracts:    []*contractState{cs},
		confirmDepth: 100,
	}

	// Index events in blocks 5, 6, 7.
	for block := uint64(5); block <= 7; block++ {
		sender := new(felt.Felt).SetUint64(block)
		amount := new(felt.Felt).SetUint64(block * 100)
		raw := makeRawEvent(eventDef.Selector, contractAddr, block, sender, amount)
		if err := e.processEvent(ctx, &raw); err != nil {
			t.Fatal(err)
		}
	}

	// Reorg all blocks.
	reorg := provider.ReorgNotification{StartBlock: 5, EndBlock: 7}
	if err := e.handleReorg(ctx, reorg); err != nil {
		t.Fatal(err)
	}

	events, _ := st.GetEvents(ctx, "mytoken_Transfer", store.Query{Limit: 10})
	if len(events) != 0 {
		t.Fatalf("expected 0 events after full reorg, got %d", len(events))
	}

	cursor, _ := st.GetCursor(ctx, "mytoken")
	if cursor != 4 {
		t.Fatalf("expected cursor 4 after full reorg, got %d", cursor)
	}
}

// --- ConfirmBlocks Tests ---

func TestConfirmBlocks_PromotesPastDepth(t *testing.T) {
	e := &Engine{
		pending:      NewPendingTracker(),
		logIndices:   make(map[uint64]uint64),
		confirmDepth: 5,
	}

	for i := uint64(1); i <= 10; i++ {
		e.pending.Track(i, store.Operation{})
		e.logIndices[i] = 1
	}

	e.confirmBlocks(10)

	// Blocks 1-5 should be confirmed (10 - 5 = 5).
	for i := uint64(1); i <= 5; i++ {
		if e.pending.HasBlock(i) {
			t.Fatalf("block %d should be confirmed", i)
		}
		if _, ok := e.logIndices[i]; ok {
			t.Fatalf("block %d logIndex should be cleaned up", i)
		}
	}

	// Blocks 6-10 should remain pending.
	for i := uint64(6); i <= 10; i++ {
		if !e.pending.HasBlock(i) {
			t.Fatalf("block %d should still be pending", i)
		}
	}
}

// --- Schema Building Tests ---

func TestBuildSchemas_ExplicitEvents(t *testing.T) {
	transferDef := testEventDef("Transfer")
	approvalDef := testEventDef("Approval")

	parsedABI := &abi.ABI{
		Types:  make(map[string]*abi.TypeDef),
		Events: []*abi.EventDef{transferDef, approvalDef},
	}
	registry := abi.NewEventRegistry(parsedABI)

	cc := config.ContractConfig{
		Name:    "mytoken",
		Address: "0x123",
		Events: []config.EventConfig{
			{Name: "Transfer", Table: config.TableConfig{Type: "log"}},
		},
	}

	schemas := schema.BuildSchemas(cc, parsedABI, registry)

	if len(schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(schemas))
	}
	if _, ok := schemas["Transfer"]; !ok {
		t.Fatal("expected Transfer schema")
	}
	if schemas["Transfer"].TableType != types.TableTypeLog {
		t.Fatal("expected log table type")
	}
}

func TestBuildSchemas_Wildcard(t *testing.T) {
	transferDef := testEventDef("Transfer")
	approvalDef := testEventDef("Approval")

	parsedABI := &abi.ABI{
		Types:  make(map[string]*abi.TypeDef),
		Events: []*abi.EventDef{transferDef, approvalDef},
	}
	registry := abi.NewEventRegistry(parsedABI)

	cc := config.ContractConfig{
		Name:    "mytoken",
		Address: "0x123",
		Events: []config.EventConfig{
			{Name: "*", Table: config.TableConfig{Type: "log"}},
		},
	}

	schemas := schema.BuildSchemas(cc, parsedABI, registry)

	if len(schemas) != 2 {
		t.Fatalf("expected 2 schemas (wildcard), got %d", len(schemas))
	}
	if _, ok := schemas["Transfer"]; !ok {
		t.Fatal("expected Transfer schema")
	}
	if _, ok := schemas["Approval"]; !ok {
		t.Fatal("expected Approval schema")
	}
}

func TestBuildSchemas_WildcardWithOverride(t *testing.T) {
	transferDef := testEventDef("Transfer")
	approvalDef := testEventDef("Approval")

	parsedABI := &abi.ABI{
		Types:  make(map[string]*abi.TypeDef),
		Events: []*abi.EventDef{transferDef, approvalDef},
	}
	registry := abi.NewEventRegistry(parsedABI)

	cc := config.ContractConfig{
		Name:    "mytoken",
		Address: "0x123",
		Events: []config.EventConfig{
			{Name: "*", Table: config.TableConfig{Type: "log"}},
			{Name: "Transfer", Table: config.TableConfig{Type: "unique", UniqueKey: "sender"}},
		},
	}

	schemas := schema.BuildSchemas(cc, parsedABI, registry)

	if len(schemas) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(schemas))
	}

	// Transfer should be overridden to unique.
	if schemas["Transfer"].TableType != types.TableTypeUnique {
		t.Fatal("expected Transfer to be unique type (override)")
	}
	if schemas["Transfer"].UniqueKey != "sender" {
		t.Fatal("expected Transfer unique key to be 'sender'")
	}

	// Approval should use wildcard default (log).
	if schemas["Approval"].TableType != types.TableTypeLog {
		t.Fatal("expected Approval to be log type (wildcard default)")
	}
}

func TestBuildSchemas_MetadataColumns(t *testing.T) {
	transferDef := testEventDef("Transfer")

	parsedABI := &abi.ABI{
		Types:  make(map[string]*abi.TypeDef),
		Events: []*abi.EventDef{transferDef},
	}
	registry := abi.NewEventRegistry(parsedABI)

	cc := config.ContractConfig{
		Name:    "mytoken",
		Address: "0x123",
		Events: []config.EventConfig{
			{Name: "Transfer", Table: config.TableConfig{Type: "log"}},
		},
	}

	schemas := schema.BuildSchemas(cc, parsedABI, registry)
	schema := schemas["Transfer"]

	// Check standard metadata columns are present.
	colNames := make(map[string]bool)
	for _, col := range schema.Columns {
		colNames[col.Name] = true
	}

	expected := []string{"block_number", "transaction_hash", "log_index", "timestamp", "contract_address", "event_name", "status", "sender", "amount"}
	for _, name := range expected {
		if !colNames[name] {
			t.Fatalf("missing expected column: %s", name)
		}
	}
}

// --- DetermineStartBlock Tests ---

func TestDetermineStartBlocks_FromCursor(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	st.SetCursor(ctx, "mytoken", 100)

	contractAddr := new(felt.Felt).SetUint64(0xABC)
	eventDef := testEventDef("Transfer")
	cs := testContractState(contractAddr, "mytoken", []*abi.EventDef{eventDef}, types.TableTypeLog)

	e := &Engine{
		cfg:       &config.Config{Indexer: config.IndexerConfig{StartBlock: 50}},
		store:     st,
		contracts: []*contractState{cs},
	}

	blocks, err := e.determineStartBlocks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// max(cursor+1, configStart) = max(101, 50) = 101
	if blocks["mytoken"] != 101 {
		t.Fatalf("expected start block 101, got %d", blocks["mytoken"])
	}
}

func TestDetermineStartBlocks_FromConfig(t *testing.T) {
	st := memory.New()
	ctx := context.Background()

	contractAddr := new(felt.Felt).SetUint64(0xABC)
	eventDef := testEventDef("Transfer")
	cs := testContractState(contractAddr, "mytoken", []*abi.EventDef{eventDef}, types.TableTypeLog)

	e := &Engine{
		cfg:       &config.Config{Indexer: config.IndexerConfig{StartBlock: 500}},
		store:     st,
		contracts: []*contractState{cs},
	}

	blocks, err := e.determineStartBlocks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if blocks["mytoken"] != 500 {
		t.Fatalf("expected start block 500, got %d", blocks["mytoken"])
	}
}

func TestDetermineStartBlocks_PerContract(t *testing.T) {
	st := memory.New()
	ctx := context.Background()
	st.SetCursor(ctx, "contract_a", 100)
	// contract_b has no cursor yet

	addrA := new(felt.Felt).SetUint64(0xAAA)
	addrB := new(felt.Felt).SetUint64(0xBBB)
	eventDef := testEventDef("Transfer")
	csA := testContractState(addrA, "contract_a", []*abi.EventDef{eventDef}, types.TableTypeLog)
	csB := testContractState(addrB, "contract_b", []*abi.EventDef{eventDef}, types.TableTypeLog)

	e := &Engine{
		cfg:       &config.Config{Indexer: config.IndexerConfig{StartBlock: 50}},
		store:     st,
		contracts: []*contractState{csA, csB},
	}

	blocks, err := e.determineStartBlocks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Contract A has cursor 100, so start = max(101, 50) = 101.
	if blocks["contract_a"] != 101 {
		t.Fatalf("expected contract_a start 101, got %d", blocks["contract_a"])
	}
	// Contract B has no cursor, so start = max(0, 50) = 50.
	if blocks["contract_b"] != 50 {
		t.Fatalf("expected contract_b start 50, got %d", blocks["contract_b"])
	}
}

// --- Integration: EventLoop with Reorg ---

func TestEventLoop_ProcessesEventsAndReorgs(t *testing.T) {
	st := memory.New()
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	eventDef := testEventDef("Transfer")
	cs := testContractState(contractAddr, "mytoken", []*abi.EventDef{eventDef}, types.TableTypeLog)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := st.CreateTable(ctx, cs.schemas["Transfer"]); err != nil {
		t.Fatal(err)
	}

	events := make(chan provider.RawEvent, 10)
	reorgs := make(chan provider.ReorgNotification, 10)

	e := &Engine{
		store:        st,
		logger:       noopLogger(),
		pending:      NewPendingTracker(),
		logIndices:   make(map[uint64]uint64),
		contracts:    []*contractState{cs},
		confirmDepth: 100,
		events:       events,
		reorgs:       reorgs,
	}

	// Run event loop in background.
	done := make(chan error, 1)
	go func() {
		done <- e.eventLoop(ctx)
	}()

	// Send events for blocks 10, 11, 12.
	for block := uint64(10); block <= 12; block++ {
		sender := new(felt.Felt).SetUint64(block)
		amount := new(felt.Felt).SetUint64(block * 100)
		events <- makeRawEvent(eventDef.Selector, contractAddr, block, sender, amount)
	}

	// Wait for processing.
	time.Sleep(50 * time.Millisecond)

	// Verify 3 events stored.
	evts, _ := st.GetEvents(ctx, "mytoken_Transfer", store.Query{Limit: 10})
	if len(evts) != 3 {
		t.Fatalf("expected 3 events, got %d", len(evts))
	}

	// Send reorg for blocks 11-12.
	reorgs <- provider.ReorgNotification{StartBlock: 11, EndBlock: 12}

	// Wait for reorg processing.
	time.Sleep(50 * time.Millisecond)

	// Only block 10 should remain.
	evts, _ = st.GetEvents(ctx, "mytoken_Transfer", store.Query{Limit: 10})
	if len(evts) != 1 {
		t.Fatalf("expected 1 event after reorg, got %d", len(evts))
	}

	// Send replacement events for blocks 11, 12.
	for block := uint64(11); block <= 12; block++ {
		sender := new(felt.Felt).SetUint64(block + 100)
		amount := new(felt.Felt).SetUint64(block * 200)
		events <- makeRawEvent(eventDef.Selector, contractAddr, block, sender, amount)
	}

	time.Sleep(50 * time.Millisecond)

	// Now 3 events again (1 original + 2 replacement).
	evts, _ = st.GetEvents(ctx, "mytoken_Transfer", store.Query{Limit: 10})
	if len(evts) != 3 {
		t.Fatalf("expected 3 events after replacement, got %d", len(evts))
	}

	cancel()
	<-done
}

// --- BuildSubscriptions Tests ---

func TestBuildSubscriptions_WildcardLeavesKeysNil(t *testing.T) {
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	transferDef := testEventDef("Transfer")
	approvalDef := testEventDef("Approval")

	parsedABI := &abi.ABI{
		Types:  make(map[string]*abi.TypeDef),
		Events: []*abi.EventDef{transferDef, approvalDef},
	}
	registry := abi.NewEventRegistry(parsedABI)

	cs := &contractState{
		config: config.ContractConfig{
			Name:    "mytoken",
			Address: contractAddr.String(),
			Events: []config.EventConfig{
				{Name: "*", Table: config.TableConfig{Type: "log"}},
			},
		},
		address:  contractAddr,
		abi:      parsedABI,
		registry: registry,
		schemas:  map[string]*types.TableSchema{},
	}

	e := &Engine{contracts: []*contractState{cs}}
	subs := e.buildSubscriptions(map[string]uint64{"mytoken": 100})

	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if subs[0].Keys != nil {
		t.Fatalf("expected nil Keys for wildcard config, got %v", subs[0].Keys)
	}
	if subs[0].StartBlock != 100 {
		t.Fatalf("expected start block 100, got %d", subs[0].StartBlock)
	}
}

func TestBuildSubscriptions_ExplicitEventsPopulatesKeys(t *testing.T) {
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	transferDef := testEventDef("Transfer")
	approvalDef := testEventDef("Approval")

	parsedABI := &abi.ABI{
		Types:  make(map[string]*abi.TypeDef),
		Events: []*abi.EventDef{transferDef, approvalDef},
	}
	registry := abi.NewEventRegistry(parsedABI)

	cs := &contractState{
		config: config.ContractConfig{
			Name:    "mytoken",
			Address: contractAddr.String(),
			Events: []config.EventConfig{
				{Name: "Transfer", Table: config.TableConfig{Type: "log"}},
				{Name: "Approval", Table: config.TableConfig{Type: "log"}},
			},
		},
		address:  contractAddr,
		abi:      parsedABI,
		registry: registry,
		schemas:  map[string]*types.TableSchema{},
	}

	e := &Engine{contracts: []*contractState{cs}}
	subs := e.buildSubscriptions(map[string]uint64{"mytoken": 50})

	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if subs[0].Keys == nil {
		t.Fatal("expected non-nil Keys for explicit event config")
	}
	if len(subs[0].Keys) != 1 {
		t.Fatalf("expected Keys with 1 position, got %d", len(subs[0].Keys))
	}
	if len(subs[0].Keys[0]) != 2 {
		t.Fatalf("expected 2 selectors in Keys[0], got %d", len(subs[0].Keys[0]))
	}

	// Verify the selectors match the event definitions.
	selectorSet := make(map[string]bool)
	for _, sel := range subs[0].Keys[0] {
		selectorSet[sel.String()] = true
	}
	if !selectorSet[transferDef.Selector.String()] {
		t.Fatal("expected Transfer selector in Keys")
	}
	if !selectorSet[approvalDef.Selector.String()] {
		t.Fatal("expected Approval selector in Keys")
	}
}

func TestBuildSubscriptions_SingleExplicitEvent(t *testing.T) {
	contractAddr := new(felt.Felt).SetUint64(0xABC)
	transferDef := testEventDef("Transfer")
	approvalDef := testEventDef("Approval")

	parsedABI := &abi.ABI{
		Types:  make(map[string]*abi.TypeDef),
		Events: []*abi.EventDef{transferDef, approvalDef},
	}
	registry := abi.NewEventRegistry(parsedABI)

	// Only configure Transfer, not Approval.
	cs := &contractState{
		config: config.ContractConfig{
			Name:    "mytoken",
			Address: contractAddr.String(),
			Events: []config.EventConfig{
				{Name: "Transfer", Table: config.TableConfig{Type: "log"}},
			},
		},
		address:  contractAddr,
		abi:      parsedABI,
		registry: registry,
		schemas:  map[string]*types.TableSchema{},
	}

	e := &Engine{contracts: []*contractState{cs}}
	subs := e.buildSubscriptions(map[string]uint64{"mytoken": 0})

	if len(subs[0].Keys[0]) != 1 {
		t.Fatalf("expected 1 selector in Keys[0], got %d", len(subs[0].Keys[0]))
	}
	if !subs[0].Keys[0][0].Equal(transferDef.Selector) {
		t.Fatal("expected Transfer selector")
	}
}

// --- Column Type Mapping Tests ---

func TestCairoTypeToColumnType(t *testing.T) {
	tests := []struct {
		kind     abi.CairoType
		expected string
	}{
		{abi.CairoFelt252, "string"},
		{abi.CairoContractAddress, "string"},
		{abi.CairoU64, "int64"},
		{abi.CairoU128, "string"},
		{abi.CairoU256, "string"},
		{abi.CairoBool, "bool"},
		{abi.CairoByteArray, "string"},
		{abi.CairoArray, "string"},
		{abi.CairoStruct, "string"},
	}

	for _, tt := range tests {
		td := &abi.TypeDef{Kind: tt.kind}
		result := schema.CairoTypeToColumnType(td)
		if result != tt.expected {
			t.Errorf("CairoTypeToColumnType(%d) = %s, want %s", tt.kind, result, tt.expected)
		}
	}
}

// --- Helpers ---

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(nopWriter{}, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
