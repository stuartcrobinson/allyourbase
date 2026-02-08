package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the AYB server",
	Long:  `Stop a running AllYourBase server gracefully.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Implement process signaling (PID file or socket).
		fmt.Println("Stop not yet implemented. Use Ctrl+C or send SIGTERM to the running process.")
		return nil
	},
}
