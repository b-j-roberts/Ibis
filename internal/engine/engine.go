package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/NethermindEth/juno/core/felt"

	"github.com/b-j-roberts/ibis/internal/abi"
	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/provider"
	"github.com/b-j-roberts/ibis/internal/schema"
	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/types"
)

// DefaultConfirmationDepth is the number of blocks after which pending
// operations are promoted to confirmed and their revert data is discarded.
const DefaultConfirmationDepth uint64 = 10

// contractState holds per-contract ABI, event registry, and table schemas.
type contractState struct {
	config   config.ContractConfig
	address  *felt.Felt
	abi      *abi.ABI
	registry *abi.EventRegistry
	schemas  map[string]*types.TableSchema // event name -> schema
}

// Engine is the core indexing orchestrator. It receives events from the
// subscriber, decodes them via ABI, generates revert/add operation pairs,
// writes to the store, and handles chain reorganizations.
type Engine struct {
	cfg      *config.Config
	store    store.Store
	provider *provider.StarknetProvider
	logger   *slog.Logger

	// Per-contract state built during setup.
	contracts []*contractState

	// Pending block operation tracker for reorg support.
	pending *PendingTracker

	// Event channel from subscriber.
	events chan provider.RawEvent

	// Reorg notification channel from subscriber.
	reorgs chan provider.ReorgNotification

	// Log index counter per block.
	logIndices map[uint64]uint64

	// Confirmation depth: blocks past this depth are considered confirmed.
	confirmDepth uint64

	// onEvent is an optional callback invoked after an event is successfully
	// indexed. Used by the API server's EventBus for SSE streaming.
	onEvent func(contract, event, table string, blockNumber, logIndex uint64, data map[string]any)

	// setupDone tracks whether Setup has been called.
	setupDone bool
}

// New creates an Engine with the given dependencies.
func New(cfg *config.Config, st store.Store, prov *provider.StarknetProvider, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		cfg:          cfg,
		store:        st,
		provider:     prov,
		logger:       logger.With("component", "engine"),
		pending:      NewPendingTracker(),
		events:       make(chan provider.RawEvent, 256),
		reorgs:       make(chan provider.ReorgNotification, 16),
		logIndices:   make(map[uint64]uint64),
		confirmDepth: DefaultConfirmationDepth,
	}
}

// SetConfirmationDepth overrides the default confirmation depth.
func (e *Engine) SetConfirmationDepth(depth uint64) {
	e.confirmDepth = depth
}

// SetOnEvent sets a callback that is invoked after each event is successfully
// indexed. The callback receives the contract name, event name, table name,
// block number, log index, and decoded event data.
func (e *Engine) SetOnEvent(fn func(contract, event, table string, blockNumber, logIndex uint64, data map[string]any)) {
	e.onEvent = fn
}

// Setup resolves ABIs, builds event registries and table schemas, and creates
// tables in the store. Call this before Run to access Schemas() for the API server.
func (e *Engine) Setup(ctx context.Context) error {
	if e.setupDone {
		return nil
	}
	if err := e.setup(ctx); err != nil {
		return fmt.Errorf("engine setup: %w", err)
	}
	e.setupDone = true
	return nil
}

// Schemas returns all table schemas built during Setup.
func (e *Engine) Schemas() []*types.TableSchema {
	var schemas []*types.TableSchema
	for _, cs := range e.contracts {
		for _, s := range cs.schemas {
			schemas = append(schemas, s)
		}
	}
	return schemas
}

// Run starts the indexing engine. It resolves ABIs, creates table schemas,
// determines the starting block, starts the event subscriber, and processes
// events until the context is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	// Step 1: Resolve ABIs and build per-contract state.
	if !e.setupDone {
		if err := e.setup(ctx); err != nil {
			return fmt.Errorf("engine setup: %w", err)
		}
	}

	// Step 2: Determine starting block.
	startBlock, err := e.determineStartBlock(ctx)
	if err != nil {
		return fmt.Errorf("determine start block: %w", err)
	}
	e.logger.Info("starting indexer", "start_block", startBlock)

	// Step 3: Build subscriptions and start the subscriber.
	subs := e.buildSubscriptions(startBlock)
	subscriber := e.provider.NewSubscriber(subs, e.events, &provider.SubscriberConfig{
		BlocksPerQuery: uint64(e.cfg.Indexer.BatchSize) * 10,
	})
	subscriber.SetReorgChan(e.reorgs)

	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()

	var subErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := subscriber.Start(subCtx); err != nil && ctx.Err() == nil {
			subErr = err
			subCancel()
		}
	}()

	// Step 4: Main event loop.
	err = e.eventLoop(ctx)

	// Step 5: Graceful shutdown.
	subCancel()
	wg.Wait()

	if err != nil && ctx.Err() != nil {
		e.logger.Info("engine stopped")
		return nil
	}
	if subErr != nil {
		return fmt.Errorf("subscriber: %w", subErr)
	}
	return err
}

// setup resolves ABIs, builds event registries and table schemas, and creates
// tables in the store.
func (e *Engine) setup(ctx context.Context) error {
	resolver := config.NewABIResolver(e.provider)
	abis, err := resolver.ResolveAll(ctx, e.cfg.Contracts)
	if err != nil {
		return fmt.Errorf("resolve ABIs: %w", err)
	}

	for _, cc := range e.cfg.Contracts {
		contractABI := abis[cc.Address]
		if contractABI == nil {
			return fmt.Errorf("no ABI resolved for contract %s (%s)", cc.Name, cc.Address)
		}

		registry := abi.NewEventRegistry(contractABI)
		schemas := schema.BuildSchemas(cc, contractABI, registry)

		// Parse contract address.
		address, err := new(felt.Felt).SetString(cc.Address)
		if err != nil {
			return fmt.Errorf("parsing address for %s: %w", cc.Name, err)
		}

		cs := &contractState{
			config:   cc,
			address:  address,
			abi:      contractABI,
			registry: registry,
			schemas:  schemas,
		}
		e.contracts = append(e.contracts, cs)

		// Create tables in store.
		for _, schema := range schemas {
			if err := e.store.CreateTable(ctx, schema); err != nil {
				return fmt.Errorf("create table %s: %w", schema.Name, err)
			}
			e.logger.Info("created table",
				"name", schema.Name,
				"type", schema.TableType,
				"columns", len(schema.Columns),
			)
		}
	}

	return nil
}

// determineStartBlock computes the block number to begin indexing from.
// Logic: max(persisted_cursor + 1, config_start_block).
// If both are 0, starts from the latest chain block.
func (e *Engine) determineStartBlock(ctx context.Context) (uint64, error) {
	cursor, err := e.store.GetCursor(ctx)
	if err != nil {
		return 0, fmt.Errorf("get cursor: %w", err)
	}

	configStart := e.cfg.Indexer.StartBlock

	if configStart == 0 && cursor == 0 {
		latest, err := e.provider.BlockNumber(ctx)
		if err != nil {
			return 0, fmt.Errorf("get latest block: %w", err)
		}
		return latest, nil
	}

	// max(cursor + 1, configStart)
	startBlock := configStart
	if cursor > 0 && cursor+1 > startBlock {
		startBlock = cursor + 1
	}

	return startBlock, nil
}

// buildSubscriptions creates ContractSubscription entries for the subscriber.
// If a contract only configures specific events (no wildcard "*"), the
// subscription includes a Keys filter so the node only sends matching events.
func (e *Engine) buildSubscriptions(startBlock uint64) []provider.ContractSubscription {
	subs := make([]provider.ContractSubscription, 0, len(e.contracts))
	for _, cs := range e.contracts {
		sub := provider.ContractSubscription{
			Address:    cs.address,
			StartBlock: startBlock,
		}

		// Only set key filters when there is no wildcard event configured.
		if !hasWildcardEvent(cs.config) {
			var selectors []*felt.Felt
			for _, ec := range cs.config.Events {
				if ev := cs.registry.MatchName(ec.Name); ev != nil {
					selectors = append(selectors, ev.Selector)
				}
			}
			if len(selectors) > 0 {
				sub.Keys = [][]*felt.Felt{selectors}
			}
		}

		subs = append(subs, sub)
	}
	return subs
}

// hasWildcardEvent returns true if the contract config includes a "*" event entry.
func hasWildcardEvent(cc config.ContractConfig) bool {
	for _, ec := range cc.Events {
		if ec.Name == "*" {
			return true
		}
	}
	return false
}

// eventLoop processes events and reorg notifications until the context is cancelled.
func (e *Engine) eventLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		case reorg := <-e.reorgs:
			if err := e.handleReorg(ctx, reorg); err != nil {
				e.logger.Error("reorg handling failed", "error", err)
				return fmt.Errorf("handle reorg: %w", err)
			}

		case event, ok := <-e.events:
			if !ok {
				return nil
			}
			if err := e.processEvent(ctx, event); err != nil {
				e.logger.Error("event processing failed",
					"block", event.BlockNumber,
					"error", err,
				)
				continue
			}
		}
	}
}

// EventChan returns the engine's event channel for direct injection (testing).
func (e *Engine) EventChan() chan<- provider.RawEvent {
	return e.events
}

// ReorgChan returns the engine's reorg channel for direct injection (testing).
func (e *Engine) ReorgChan() chan<- provider.ReorgNotification {
	return e.reorgs
}

// Store returns the engine's store (for testing/inspection).
func (e *Engine) Store() store.Store {
	return e.store
}
