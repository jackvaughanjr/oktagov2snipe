package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackvaughanjr/okta2snipe/internal/okta"
	"github.com/jackvaughanjr/okta2snipe/internal/snipeit"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Validate API connections and report current state",
	RunE:  runTest,
}

func init() {
	rootCmd.AddCommand(testCmd)
}

func runTest(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

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

	// --- Okta ---
	fmt.Println("=== Okta ===")
	slog.Info("fetching active Okta users", "url", viper.GetString("okta.url"))
	users, err := oktaClient.ListActiveUsers(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Okta error: %v\n", err)
		return err
	}
	fmt.Printf("Active users: %d\n", len(users))

	// Count role holders (best-effort — 403 is non-fatal per spec)
	roleHolders := 0
	for _, u := range users {
		slog.Debug("fetching roles", "user", u.Profile.Email)
		roles, err := oktaClient.GetUserRoles(ctx, u.ID)
		if err != nil {
			slog.Warn("could not fetch roles", "user", u.Profile.Email, "error", err)
			continue
		}
		if len(roles) > 0 {
			slog.Debug("user has roles", "user", u.Profile.Email, "count", len(roles))
			roleHolders++
		}
	}
	fmt.Printf("Users with roles: %d\n", roleHolders)

	// --- Snipe-IT ---
	fmt.Println("\n=== Snipe-IT ===")
	licenseName := viper.GetString("snipe_it.license_name")
	if licenseName == "" {
		licenseName = "Okta"
	}

	slog.Info("looking up license in Snipe-IT", "url", viper.GetString("snipe_it.url"), "license", licenseName)
	lic, err := snipeClient.FindLicenseByName(ctx, licenseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Snipe-IT error: %v\n", err)
		return err
	}
	if lic == nil {
		fmt.Printf("License %q: not found\n", licenseName)
	} else {
		slog.Debug("license detail", "id", lic.ID, "seats", lic.Seats, "free", lic.FreeSeatsCount)
		fmt.Printf("License %q: id=%d seats=%d free=%d\n",
			lic.Name, lic.ID, lic.Seats, lic.FreeSeatsCount)
	}

	return nil
}
