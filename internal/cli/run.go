package cli

import (
	"fmt"

	"github.com/b-j-roberts/ibis/internal/config"
	"github.com/spf13/cobra"
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

		// TODO: start indexing engine (task 2.6)
		fmt.Fprintln(cmd.OutOrStdout(), "\nIndexing engine not yet implemented (task 2.6)")
		return nil
	},
}
