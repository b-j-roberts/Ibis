package cli

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/b-j-roberts/ibis/internal/engine"
	"github.com/b-j-roberts/ibis/internal/provider"
	"github.com/b-j-roberts/ibis/internal/store"
	"github.com/b-j-roberts/ibis/internal/store/badger"
	"github.com/b-j-roberts/ibis/internal/store/memory"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the indexer with the given config",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}

		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))

		fmt.Fprintf(cmd.OutOrStdout(), "Loaded config from %s\n", cfgPath)
		fmt.Fprintf(cmd.OutOrStdout(), "  Network:  %s\n", cfg.Network)
		fmt.Fprintf(cmd.OutOrStdout(), "  RPC:      %s\n", cfg.RPC)
		fmt.Fprintf(cmd.OutOrStdout(), "  Backend:  %s\n", cfg.Database.Backend)
		fmt.Fprintf(cmd.OutOrStdout(), "  API:      %s:%d\n", cfg.API.Host, cfg.API.Port)
		fmt.Fprintf(cmd.OutOrStdout(), "  Contracts: %d\n", len(cfg.Contracts))
		for _, c := range cfg.Contracts {
			fmt.Fprintf(cmd.OutOrStdout(), "    - %s (%s): %d events\n", c.Name, c.Address, len(c.Events))
		}

		// Create Starknet provider.
		ctx := cmd.Context()
		prov, err := provider.New(ctx, cfg.RPC, logger)
		if err != nil {
			return fmt.Errorf("creating provider: %w", err)
		}
		defer prov.Close()

		// Create store backend.
		st, err := createStore(cfg, logger)
		if err != nil {
			return fmt.Errorf("creating store: %w", err)
		}
		defer st.Close()

		// Create and run engine with signal handling.
		ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()

		eng := engine.New(cfg, st, prov, logger)

		fmt.Fprintln(cmd.OutOrStdout(), "\nStarting indexer...")
		if err := eng.Run(ctx); err != nil {
			return fmt.Errorf("engine: %w", err)
		}

		return nil
	},
}

// createStore initializes the appropriate store backend from config.
func createStore(cfg *config.Config, logger *slog.Logger) (store.Store, error) {
	switch cfg.Database.Backend {
	case "memory":
		logger.Info("using in-memory store")
		return memory.New(), nil
	case "badger":
		path := cfg.Database.Badger.Path
		if path == "" {
			path = "./data/ibis"
		}
		logger.Info("using BadgerDB store", "path", path)
		return badger.New(path)
	case "postgres":
		return nil, fmt.Errorf("PostgreSQL store not yet implemented (task 2.8)")
	default:
		return nil, fmt.Errorf("unknown database backend: %s", cfg.Database.Backend)
	}
}
