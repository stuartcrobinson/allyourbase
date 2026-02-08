package cli

import (
	"fmt"

	"github.com/allyourbase/ayb/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Print resolved configuration",
	Long: `Load and print the resolved AYB configuration as TOML.
Shows the result of merging defaults, ayb.toml, environment variables, and flags.`,
	RunE: runConfig,
}

func init() {
	configCmd.Flags().String("config", "", "Path to ayb.toml config file")
}

func runConfig(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")

	cfg, err := config.Load(configPath, nil)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	out, err := cfg.ToTOML()
	if err != nil {
		return fmt.Errorf("serializing config: %w", err)
	}

	fmt.Print(out)
	return nil
}
