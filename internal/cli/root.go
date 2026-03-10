package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ibis",
	Short: "A fast, easy-to-use Starknet event indexer",
	Long: `Ibis indexes events from Starknet smart contracts using only an RPC
connection, generates typed database tables and REST APIs from contract
ABIs, and launches with a single command from a YAML config file.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(queryCmd)
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold an ibis.config.yaml from contract inspection",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement in task 2.11
		cmd.Println("ibis init: not yet implemented")
		return nil
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the indexer with the given config",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement in task 1.3
		cmd.Println("ibis run: not yet implemented")
		return nil
	},
}

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query indexed data from the terminal",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement in task 2.12
		cmd.Println("ibis query: not yet implemented")
		return nil
	},
}
