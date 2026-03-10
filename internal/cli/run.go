package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/b-j-roberts/ibis/internal/config"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the indexer with the given config",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Loaded config from %s\n", cfgPath)
		fmt.Fprintf(cmd.OutOrStdout(), "  Network:  %s\n", cfg.Network)
		fmt.Fprintf(cmd.OutOrStdout(), "  RPC:      %s\n", cfg.RPC)
		fmt.Fprintf(cmd.OutOrStdout(), "  Backend:  %s\n", cfg.Database.Backend)
		fmt.Fprintf(cmd.OutOrStdout(), "  API:      %s:%d\n", cfg.API.Host, cfg.API.Port)
		fmt.Fprintf(cmd.OutOrStdout(), "  Contracts: %d\n", len(cfg.Contracts))
		for _, c := range cfg.Contracts {
			fmt.Fprintf(cmd.OutOrStdout(), "    - %s (%s): %d events\n", c.Name, c.Address, len(c.Events))
		}

		// Resolve ABIs for all contracts at startup.
		// TODO: pass a real ABIFetcher once provider is implemented (task 2.5)
		resolver := config.NewABIResolver(nil)
		abis, err := resolver.ResolveAll(cmd.Context(), cfg.Contracts)
		if err != nil {
			return fmt.Errorf("resolving ABIs: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "\nResolved %d contract ABIs:\n", len(abis))
		for _, c := range cfg.Contracts {
			parsed := abis[c.Address]
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: %d events\n", c.Name, len(parsed.Events))
			for _, ev := range parsed.Events {
				fmt.Fprintf(cmd.OutOrStdout(), "    - %s (selector: 0x%s)\n", ev.Name, ev.Selector.Text(16))
			}
		}

		// TODO: start indexing engine (task 2.6)
		fmt.Fprintln(cmd.OutOrStdout(), "\nIndexing engine not yet implemented (task 2.6)")
		return nil
	},
}
