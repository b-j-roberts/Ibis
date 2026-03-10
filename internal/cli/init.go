package cli

import (
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold an ibis.config.yaml from contract inspection",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: implement in task 2.11
		cmd.Println("ibis init: not yet implemented")
		return nil
	},
}
