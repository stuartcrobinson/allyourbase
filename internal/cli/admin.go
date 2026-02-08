package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/postgres"
	"github.com/spf13/cobra"
)

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Admin user management",
}

var adminCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new user account",
	Long: `Create a new user account in the database. Requires a running database
(or database URL in config/env/flag).

Example:
  ayb admin create --email admin@example.com --password mysecretpassword`,
	RunE: runAdminCreate,
}

func init() {
	adminCmd.AddCommand(adminCreateCmd)

	adminCreateCmd.Flags().String("config", "", "Path to ayb.toml config file")
	adminCreateCmd.Flags().String("database-url", "", "PostgreSQL connection URL (overrides config)")
	adminCreateCmd.Flags().String("email", "", "User email address")
	adminCreateCmd.Flags().String("password", "", "User password (min 8 characters)")
	adminCreateCmd.MarkFlagRequired("email")
	adminCreateCmd.MarkFlagRequired("password")
}

func runAdminCreate(cmd *cobra.Command, args []string) error {
	email, _ := cmd.Flags().GetString("email")
	password, _ := cmd.Flags().GetString("password")
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := loadMigrateConfig(cmd)
	if err != nil {
		return err
	}

	dbURL := cfg.Database.URL
	if v, _ := cmd.Flags().GetString("database-url"); v != "" {
		dbURL = v
	}
	if dbURL == "" {
		return fmt.Errorf("no database URL configured (set database.url in ayb.toml, AYB_DATABASE_URL env, or --database-url flag)")
	}

	ctx := context.Background()
	pool, err := postgres.New(ctx, postgres.Config{
		URL:      dbURL,
		MaxConns: 5,
		MinConns: 1,
	}, logger)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	defer pool.Close()

	// Ensure system migrations are applied (creates _ayb_users table).
	migRunner := migrations.NewRunner(pool.DB(), logger)
	if err := migRunner.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrapping migrations: %w", err)
	}
	if _, err := migRunner.Run(ctx); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	user, err := auth.CreateUser(ctx, pool.DB(), email, password)
	if err != nil {
		return fmt.Errorf("creating user: %w", err)
	}

	fmt.Printf("Created user: %s (%s)\n", user.Email, user.ID)
	return nil
}
