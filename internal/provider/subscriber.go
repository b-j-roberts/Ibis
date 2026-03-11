package provider

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/NethermindEth/starknet.go/client"
	"github.com/NethermindEth/starknet.go/rpc"
)

const (
	// WSS reconnection backoff bounds.
	minBackoff = 1 * time.Second
	maxBackoff = 30 * time.Second

	// Max consecutive WSS dial failures before falling back to polling.
	maxWSSDialFailures = 3

	// Polling intervals.
	catchupPollInterval = 100 * time.Millisecond
	tipPollInterval     = 2 * time.Second

	// Blocks behind chain tip that triggers fast catchup polling.
	catchupThreshold uint64 = 50

	// Default number of blocks per polling query.
	defaultBlocksPerQuery uint64 = 100
)

// wssSession represents an active WebSocket subscription session.
// This abstraction allows injecting mock sessions in tests.
type wssSession struct {
	events <-chan *rpc.EmittedEventWithFinalityStatus
	errs   <-chan error
	reorgs <-chan *client.ReorgEvent
	close  func()
}

// wssDialer creates a WSS subscription session for the given parameters.
type wssDialer func(ctx context.Context, wsURL string, input *rpc.EventSubscriptionInput) (*wssSession, error)

// defaultWSSDialer creates a real WSS subscription using starknet.go.
func defaultWSSDialer(ctx context.Context, wsURL string, input *rpc.EventSubscriptionInput) (*wssSession, error) {
	ws, err := rpc.NewWebsocketProvider(ctx, wsURL)
	if err != nil {
		return nil, fmt.Errorf("connecting websocket: %w", err)
	}

	eventCh := make(chan *rpc.EmittedEventWithFinalityStatus, 100)
	sub, err := ws.SubscribeEvents(ctx, eventCh, input)
	if err != nil {
		ws.Close()
		return nil, fmt.Errorf("subscribing to events: %w", err)
	}

	return &wssSession{
		events: eventCh,
		errs:   sub.Err(),
		reorgs: sub.Reorg(),
		close: func() {
			sub.Unsubscribe()
			ws.Close()
		},
	}, nil
}

// SubscriberConfig configures the event subscriber behavior.
type SubscriberConfig struct {
	// BlocksPerQuery is the max block range per polling request. Default: 100.
	BlocksPerQuery uint64
}

// EventSubscriber manages per-contract event subscriptions with automatic
// WSS reconnection and HTTP polling fallback.
type EventSubscriber struct {
	provider       *StarknetProvider
	contracts      []ContractSubscription
	events         chan<- RawEvent
	logger         *slog.Logger
	blocksPerQuery uint64

	// dialWSS creates a WSS session. Override in tests.
	dialWSS wssDialer
}

// NewSubscriber creates an EventSubscriber for the given contracts.
// Events are delivered to the provided channel.
func (p *StarknetProvider) NewSubscriber(contracts []ContractSubscription, events chan<- RawEvent, cfg *SubscriberConfig) *EventSubscriber {
	blocksPerQuery := defaultBlocksPerQuery
	if cfg != nil && cfg.BlocksPerQuery > 0 {
		blocksPerQuery = cfg.BlocksPerQuery
	}

	return &EventSubscriber{
		provider:       p,
		contracts:      contracts,
		events:         events,
		logger:         p.logger.With("component", "subscriber"),
		blocksPerQuery: blocksPerQuery,
		dialWSS:        defaultWSSDialer,
	}
}

// Start begins event subscription for all contracts. Blocks until ctx is cancelled.
// Each contract gets its own goroutine with independent WSS/polling lifecycle.
func (s *EventSubscriber) Start(ctx context.Context) error {
	if len(s.contracts) == 0 {
		return fmt.Errorf("no contracts to subscribe to")
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(s.contracts))

	for _, contract := range s.contracts {
		wg.Add(1)
		go func(c ContractSubscription) {
			defer wg.Done()
			if err := s.subscribeContract(ctx, c); err != nil && ctx.Err() == nil {
				errCh <- fmt.Errorf("contract %s: %w", c.Address, err)
			}
		}(contract)
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return ctx.Err()
	}
}

// subscribeContract handles the full subscription lifecycle for one contract:
// try WSS first, fall back to polling if WSS fails.
func (s *EventSubscriber) subscribeContract(ctx context.Context, contract ContractSubscription) error {
	logger := s.logger.With("contract", contract.Address)
	lastBlock := contract.StartBlock

	err := s.subscribeWSS(ctx, contract, &lastBlock, logger)
	if err != nil && ctx.Err() == nil {
		logger.Warn("WSS subscription failed, falling back to polling", "error", err)
		return s.pollEvents(ctx, contract, &lastBlock, logger)
	}

	return err
}

// subscribeWSS manages a WSS subscription with automatic reconnection using
// exponential backoff (1s → 30s). Falls back to polling after maxWSSDialFailures
// consecutive dial failures.
func (s *EventSubscriber) subscribeWSS(ctx context.Context, contract ContractSubscription, lastBlock *uint64, logger *slog.Logger) error {
	backoff := minBackoff
	consecutiveDialFails := 0

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		subInput := &rpc.EventSubscriptionInput{
			FromAddress: contract.Address,
			Keys:        contract.Keys,
		}
		if *lastBlock > 0 {
			blockNum := *lastBlock
			subInput.SubBlockID = rpc.SubscriptionBlockID{Number: &blockNum}
		}

		// Attempt to dial WSS.
		session, err := s.dialWSS(ctx, s.provider.wsURL, subInput)
		if err != nil {
			consecutiveDialFails++
			if consecutiveDialFails >= maxWSSDialFailures {
				return fmt.Errorf("WSS dial failed %d times: %w", consecutiveDialFails, err)
			}

			logger.Warn("WSS dial failed, retrying",
				"error", err,
				"backoff", backoff,
				"attempt", consecutiveDialFails,
			)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}

			backoff = time.Duration(math.Min(float64(backoff)*2, float64(maxBackoff)))
			continue
		}

		// Connected successfully — reset dial failure tracking.
		consecutiveDialFails = 0
		backoff = minBackoff

		logger.Info("WSS subscription active", "from_block", *lastBlock)

		// Process events until session error.
		err = s.processWSSEvents(ctx, session, lastBlock, logger)
		session.close()

		if ctx.Err() != nil {
			return ctx.Err()
		}

		logger.Warn("WSS session ended, reconnecting",
			"error", err,
			"backoff", backoff,
			"resume_block", *lastBlock,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff = time.Duration(math.Min(float64(backoff)*2, float64(maxBackoff)))
	}
}

// processWSSEvents reads events from an active WSS session until an error
// occurs or the context is cancelled.
func (s *EventSubscriber) processWSSEvents(ctx context.Context, session *wssSession, lastBlock *uint64, logger *slog.Logger) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-session.errs:
			return fmt.Errorf("subscription error: %w", err)

		case reorg := <-session.reorgs:
			if reorg != nil {
				logger.Warn("chain reorganization detected",
					"start_block", reorg.StartBlockNum,
					"end_block", reorg.EndBlockNum,
				)
				// Reset to reorg start so the engine can re-process.
				if reorg.StartBlockNum < *lastBlock {
					*lastBlock = reorg.StartBlockNum
				}
			}

		case evt := <-session.events:
			if evt == nil {
				continue
			}

			rawEvent := RawEvent{
				BlockNumber:     evt.BlockNumber,
				BlockHash:       evt.BlockHash,
				TransactionHash: evt.TransactionHash,
				ContractAddress: evt.FromAddress,
				Keys:            evt.Keys,
				Data:            evt.Data,
				FinalityStatus:  string(evt.FinalityStatus),
			}

			select {
			case s.events <- rawEvent:
				if evt.BlockNumber > *lastBlock {
					*lastBlock = evt.BlockNumber
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// pollEvents implements the HTTP polling fallback with adaptive timing:
// 100ms when catching up, 2s at chain tip.
func (s *EventSubscriber) pollEvents(ctx context.Context, contract ContractSubscription, lastBlock *uint64, logger *slog.Logger) error {
	logger.Info("starting polling fallback", "from_block", *lastBlock)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		latestBlock, err := s.provider.BlockNumber(ctx)
		if err != nil {
			logger.Warn("failed to get block number", "error", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(tipPollInterval):
				continue
			}
		}

		// At chain tip — wait before polling again.
		if *lastBlock > latestBlock {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(tipPollInterval):
				continue
			}
		}

		// Process blocks in chunks of blocksPerQuery.
		endBlock := *lastBlock + s.blocksPerQuery
		if endBlock > latestBlock {
			endBlock = latestBlock
		}

		events, err := s.provider.GetEvents(ctx, GetEventsOptions{
			FromBlock: *lastBlock,
			ToBlock:   endBlock,
			Address:   contract.Address,
			Keys:      contract.Keys,
			ChunkSize: 1000,
		})
		if err != nil {
			logger.Warn("failed to get events", "error", err,
				"from", *lastBlock, "to", endBlock)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(tipPollInterval):
				continue
			}
		}

		for _, evt := range events {
			select {
			case s.events <- evt:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		*lastBlock = endBlock + 1

		// Adaptive timing: fast catchup vs slow at tip.
		interval := tipPollInterval
		if latestBlock-*lastBlock > catchupThreshold {
			interval = catchupPollInterval
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// Backfill fetches historical events for a contract in the given block range
// and sends them to the events channel. Uses configurable block-range chunking
// (default: 100 blocks per query) with continuation token pagination.
func (s *EventSubscriber) Backfill(ctx context.Context, contract ContractSubscription, fromBlock, toBlock uint64) error {
	logger := s.logger.With("contract", contract.Address, "action", "backfill")
	logger.Info("starting backfill", "from", fromBlock, "to", toBlock)

	for current := fromBlock; current <= toBlock; {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		end := current + s.blocksPerQuery - 1
		if end > toBlock {
			end = toBlock
		}

		events, err := s.provider.GetEvents(ctx, GetEventsOptions{
			FromBlock: current,
			ToBlock:   end,
			Address:   contract.Address,
			Keys:      contract.Keys,
			ChunkSize: 1000,
		})
		if err != nil {
			return fmt.Errorf("backfill events [%d, %d]: %w", current, end, err)
		}

		for _, evt := range events {
			select {
			case s.events <- evt:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		logger.Debug("backfill progress", "block", end, "events", len(events))
		current = end + 1
	}

	logger.Info("backfill complete", "from", fromBlock, "to", toBlock)
	return nil
}
