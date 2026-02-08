package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show AYB server status",
	Long:  `Show the running state of the AllYourBase server.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Check PID file or health endpoint.
		fmt.Println("Status not yet implemented.")
		return nil
	},
}
