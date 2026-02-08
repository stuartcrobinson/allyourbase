package cli

import (
	"github.com/spf13/cobra"
)

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

// SetVersion is called from main to inject build-time version info.
func SetVersion(version, commit, date string) {
	buildVersion = version
	buildCommit = commit
	buildDate = date
}

var rootCmd = &cobra.Command{
	Use:   "ayb",
	Short: "AllYourBase — Backend-as-a-Service for PostgreSQL",
	Long: `AllYourBase (AYB) connects to PostgreSQL, introspects the schema,
and auto-generates a REST API with an admin dashboard. Single binary. One config file.

Get started (embedded Postgres, zero config):
  ayb start

Or with an external database:
  ayb start --database-url postgresql://user:pass@localhost:5432/mydb`,
	SilenceUsage: true,
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(adminCmd)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
