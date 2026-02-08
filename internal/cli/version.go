package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print AYB version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ayb %s (commit: %s, built: %s)\n", buildVersion, buildCommit, buildDate)
	},
}
