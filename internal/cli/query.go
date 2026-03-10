package cli

import (
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query indexed data from the terminal",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement in task 2.12
		cmd.Println("ibis query: not yet implemented")
		return nil
	},
}
