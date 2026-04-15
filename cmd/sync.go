package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackvaughanjr/okta2snipe/internal/okta"
	"github.com/jackvaughanjr/okta2snipe/internal/slack"
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
	syncCmd.Flags().Bool("no-slack", false, "suppress Slack notifications for this run")

	_ = viper.BindPFlag("sync.dry_run", syncCmd.Flags().Lookup("dry-run"))
	_ = viper.BindPFlag("sync.force", syncCmd.Flags().Lookup("force"))
}

func runSync(cmd *cobra.Command, args []string) error {
	oktaClient := okta.NewClient(
		viper.GetString("okta.url"),
		viper.GetString("okta.api_token"),
	)
	rateLimitMs := viper.GetInt("sync.rate_limit_ms")
	if rateLimitMs <= 0 {
		rateLimitMs = 500
	}
	snipeClient := snipeit.NewClient(
		viper.GetString("snipe_it.url"),
		viper.GetString("snipe_it.api_key"),
		rateLimitMs,
	)

	emailFilter, _ := cmd.Flags().GetString("email")
	noSlack, _ := cmd.Flags().GetBool("no-slack")

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

	slackClient := slack.NewClient(viper.GetString("slack.webhook_url"))
	ctx := context.Background()

	syncer := sync.NewSyncer(oktaClient, snipeClient, cfg)
	result, err := syncer.Run(ctx, emailFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sync failed: %v\n", err)
		if !cfg.DryRun && !noSlack {
			msg := fmt.Sprintf("okta2snipe sync failed: %v", err)
			if notifyErr := slackClient.Send(ctx, msg); notifyErr != nil {
				slog.Warn("slack notification failed", "error", notifyErr)
			}
		}
		return err
	}

	if !cfg.DryRun && !noSlack {
		for _, email := range result.UnmatchedEmails {
			msg := fmt.Sprintf("okta2snipe: no Snipe-IT account found for Okta user — %s", email)
			if notifyErr := slackClient.Send(ctx, msg); notifyErr != nil {
				slog.Warn("slack notification failed", "email", email, "error", notifyErr)
			}
		}

		msg := fmt.Sprintf(
			"okta2snipe sync complete — checked out: %d, notes updated: %d, checked in: %d, skipped: %d, warnings: %d",
			result.CheckedOut, result.NotesUpdated, result.CheckedIn, result.Skipped, result.Warnings,
		)
		if notifyErr := slackClient.Send(ctx, msg); notifyErr != nil {
			slog.Warn("slack notification failed", "error", notifyErr)
		}
	}

	fmt.Printf("Sync complete: checked_out=%d notes_updated=%d checked_in=%d skipped=%d warnings=%d\n",
		result.CheckedOut, result.NotesUpdated, result.CheckedIn, result.Skipped, result.Warnings)
	return nil
}
