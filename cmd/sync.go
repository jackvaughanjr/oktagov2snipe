package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackvaughanjr/okta2snipe/internal/okta"
	"github.com/jackvaughanjr/okta2snipe/internal/snipeit"
	"github.com/jackvaughanjr/okta2snipe/internal/sync"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync active Okta users into Snipe-IT license seats",
	RunE:  runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)

	syncCmd.Flags().Bool("dry-run", false, "simulate without making changes")
	syncCmd.Flags().Bool("force", false, "re-sync even if notes appear up to date")
	syncCmd.Flags().String("email", "", "sync a single user by email address")

	_ = viper.BindPFlag("sync.dry_run", syncCmd.Flags().Lookup("dry-run"))
	_ = viper.BindPFlag("sync.force", syncCmd.Flags().Lookup("force"))
}

func runSync(cmd *cobra.Command, args []string) error {
	oktaClient := okta.NewClient(
		viper.GetString("okta.url"),
		viper.GetString("okta.api_token"),
	)
	snipeClient := snipeit.NewClient(
		viper.GetString("snipe_it.url"),
		viper.GetString("snipe_it.api_key"),
	)

	emailFilter, _ := cmd.Flags().GetString("email")

	categoryID := viper.GetInt("snipe_it.license_category_id")
	if categoryID == 0 {
		return fmt.Errorf("snipe_it.license_category_id is required in settings.yaml")
	}

	cfg := sync.Config{
		DryRun:            viper.GetBool("sync.dry_run"),
		Force:             viper.GetBool("sync.force"),
		LicenseName:       viper.GetString("snipe_it.license_name"),
		LicenseCategoryID: categoryID,
		ManufacturerID:    viper.GetInt("snipe_it.license_manufacturer_id"),
		SupplierID:        viper.GetInt("snipe_it.license_supplier_id"),
	}

	if cfg.LicenseName == "" {
		cfg.LicenseName = "Okta"
	}

	if cfg.DryRun {
		slog.Info("dry-run mode enabled — no changes will be made")
	}

	syncer := sync.NewSyncer(oktaClient, snipeClient, cfg)
	result, err := syncer.Run(context.Background(), emailFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync failed: %v\n", err)
		// TODO(slack): send failure notification with error details
		return err
	}

	fmt.Printf("Sync complete: checked_out=%d notes_updated=%d checked_in=%d skipped=%d warnings=%d\n",
		result.CheckedOut, result.NotesUpdated, result.CheckedIn, result.Skipped, result.Warnings)
	// TODO(slack): send success notification with result stats (checked_out, notes_updated, checked_in, skipped, warnings)
	return nil
}
