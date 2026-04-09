package sync

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackvaughanjr/oktagov2snipe/internal/okta"
	"github.com/jackvaughanjr/oktagov2snipe/internal/snipeit"
)

// Config controls sync behaviour.
type Config struct {
	DryRun            bool
	Force             bool
	LicenseName       string
	LicenseCategoryID int
	// ManufacturerID is optional. If 0, an "Okta" manufacturer is found or created automatically.
	ManufacturerID int
	// SupplierID is optional. If 0, no supplier is set on the license.
	SupplierID int
}

// Syncer orchestrates the Okta → Snipe-IT user sync.
type Syncer struct {
	okta   *okta.Client
	snipe  *snipeit.Client
	config Config
}

func NewSyncer(okta *okta.Client, snipe *snipeit.Client, cfg Config) *Syncer {
	return &Syncer{okta: okta, snipe: snipe, config: cfg}
}

// Run executes the full sync. emailFilter restricts the checkout pass to one
// user (and skips the checkin pass entirely).
func (s *Syncer) Run(ctx context.Context, emailFilter string) (Result, error) {
	var result Result

	// 1. Fetch active Okta users.
	slog.Info("fetching active Okta users")
	activeUsers, err := s.okta.ListActiveUsers(ctx)
	if err != nil {
		return result, err
	}
	slog.Info("fetched active users", "count", len(activeUsers))

	// 2. Build active email set (for checkin pass).
	activeEmails := make(map[string]struct{}, len(activeUsers))
	for _, u := range activeUsers {
		activeEmails[emailKey(u)] = struct{}{}
	}

	// 3. Apply --email filter.
	if emailFilter != "" {
		needle := strings.ToLower(emailFilter)
		filtered := activeUsers[:0]
		for _, u := range activeUsers {
			if emailKey(u) == needle {
				filtered = append(filtered, u)
				break
			}
		}
		activeUsers = filtered
		slog.Info("filtered to single user", "email", emailFilter, "found", len(activeUsers) > 0)
	}

	// 4. Fetch roles per user.
	type userWithRoles struct {
		user  okta.User
		roles []okta.Role
	}
	usersWithRoles := make([]userWithRoles, 0, len(activeUsers))
	for _, u := range activeUsers {
		roles, err := s.okta.GetUserRoles(ctx, u.ID)
		if err != nil {
			slog.Warn("could not fetch roles for user", "user", emailKey(u), "error", err)
		}
		usersWithRoles = append(usersWithRoles, userWithRoles{u, roles})
	}

	// 5. Resolve manufacturer ID.
	// If not pinned in config, find or create an "Okta" manufacturer in Snipe-IT.
	// Skipped in dry-run to avoid side effects.
	manufacturerID := s.config.ManufacturerID
	if !s.config.DryRun && manufacturerID == 0 {
		slog.Info("resolving Okta manufacturer in Snipe-IT")
		mfr, err := s.snipe.FindOrCreateManufacturer(ctx, "Okta", "https://www.okta.com")
		if err != nil {
			return result, err
		}
		manufacturerID = mfr.ID
		slog.Info("manufacturer resolved", "id", manufacturerID, "name", mfr.Name)
	}

	// 6. Find or create the license.
	// In dry-run mode only find — never create — and synthesize a placeholder
	// if the license doesn't exist yet so the rest of the logic can run.
	slog.Info("finding or creating license", "name", s.config.LicenseName)
	var lic *snipeit.License
	if s.config.DryRun {
		lic, err = s.snipe.FindLicenseByName(ctx, s.config.LicenseName)
		if err != nil {
			return result, err
		}
		if lic == nil {
			slog.Info("[dry-run] license not found; would be created", "name", s.config.LicenseName, "seats", len(activeEmails))
			lic = &snipeit.License{Name: s.config.LicenseName, Seats: len(activeEmails)}
		}
	} else {
		lic, err = s.snipe.FindOrCreateLicense(ctx, s.config.LicenseName, len(activeEmails), s.config.LicenseCategoryID, manufacturerID, s.config.SupplierID)
		if err != nil {
			return result, err
		}
	}
	slog.Info("license resolved", "id", lic.ID, "seats", lic.Seats, "free", lic.FreeSeatsCount)

	// 7. Expand seats if needed (never shrink).
	if len(activeEmails) > lic.Seats {
		slog.Info("expanding license seats", "current", lic.Seats, "needed", len(activeEmails))
		if !s.config.DryRun {
			lic, err = s.snipe.UpdateLicenseSeats(ctx, lic.ID, len(activeEmails))
			if err != nil {
				return result, err
			}
		}
	}

	// 8. Load current seat assignments.
	// Dry-run with a synthetic license (id == 0) skips the API call.
	// In production, id == 0 means something went wrong — fail fast.
	checkedOutByEmail := make(map[string]*snipeit.LicenseSeat)
	var freeSeats []*snipeit.LicenseSeat
	if lic.ID != 0 {
		slog.Info("loading current seat assignments")
		seats, err := s.snipe.ListLicenseSeats(ctx, lic.ID)
		if err != nil {
			return result, err
		}
		for i := range seats {
			seat := &seats[i]
			if seat.AssignedTo != nil && seat.AssignedTo.Email != "" {
				checkedOutByEmail[strings.ToLower(seat.AssignedTo.Email)] = seat
			} else {
				freeSeats = append(freeSeats, seat)
			}
		}
	} else if !s.config.DryRun {
		return result, fmt.Errorf("license resolved with id=0 in production mode — check Snipe-IT API permissions and required fields")
	} else {
		slog.Info("[dry-run] skipping seat load for new license")
	}
	slog.Info("seat state loaded",
		"checked_out", len(checkedOutByEmail),
		"free", len(freeSeats))

	// 9. Checkout / update loop.
	for _, uwr := range usersWithRoles {
		email := emailKey(uwr.user)
		notes := buildNotes(uwr.roles)

		snipeUser, err := s.snipe.FindUserByEmail(ctx, email)
		if err != nil {
			slog.Warn("error looking up Snipe-IT user", "email", email, "error", err)
			result.Warnings++
			continue
		}
		if snipeUser == nil {
			slog.Warn("no Snipe-IT user found for Okta user", "email", email)
			result.UnmatchedEmails = append(result.UnmatchedEmails, email)
			result.Warnings++
			continue
		}

		if existing, ok := checkedOutByEmail[email]; ok {
			// Already checked out — update notes if changed (or --force).
			if existing.Notes == notes && !s.config.Force {
				slog.Debug("seat up to date", "email", email)
				result.Skipped++
				continue
			}
			slog.Info("updating seat notes", "email", email, "dry_run", s.config.DryRun)
			if !s.config.DryRun {
				if err := s.snipe.UpdateSeatNotes(ctx, lic.ID, existing.ID, notes); err != nil {
					slog.Warn("failed to update seat notes", "email", email, "error", err)
					result.Warnings++
					continue
				}
			}
			result.NotesUpdated++
			continue
		}

		// Not checked out — pop a free seat (or simulate in dry-run).
		if s.config.DryRun {
			slog.Info("[dry-run] would check out seat", "email", email, "notes", notes)
			result.CheckedOut++
			continue
		}
		if len(freeSeats) == 0 {
			slog.Warn("no free seats available", "email", email)
			result.Warnings++
			continue
		}
		seat := freeSeats[0]
		freeSeats = freeSeats[1:]

		slog.Info("checking out seat", "email", email, "seat_id", seat.ID)
		if err := s.snipe.CheckoutSeat(ctx, lic.ID, seat.ID, snipeUser.ID, notes); err != nil {
			slog.Warn("failed to checkout seat", "email", email, "error", err)
			freeSeats = append(freeSeats, seat) // return it
			result.Warnings++
			continue
		}
		result.CheckedOut++
	}

	// 10. Checkin loop — skip when --email filter is set.
	if emailFilter == "" {
		for email, seat := range checkedOutByEmail {
			if _, active := activeEmails[email]; active {
				continue
			}
			slog.Info("checking in seat for inactive user", "email", email, "seat_id", seat.ID, "dry_run", s.config.DryRun)
			if !s.config.DryRun {
				if err := s.snipe.CheckinSeat(ctx, lic.ID, seat.ID); err != nil {
					slog.Warn("failed to checkin seat", "email", email, "error", err)
					result.Warnings++
					continue
				}
			}
			result.CheckedIn++
		}
	}

	return result, nil
}

// emailKey returns the canonical (lowercased) email for an Okta user.
// Prefers profile.email; falls back to profile.login.
func emailKey(u okta.User) string {
	if u.Profile.Email != "" {
		return strings.ToLower(u.Profile.Email)
	}
	return strings.ToLower(u.Profile.Login)
}

// buildNotes returns the formatted role notes string for a seat.
func buildNotes(roles []okta.Role) string {
	if len(roles) == 0 {
		return ""
	}
	labels := make([]string, 0, len(roles))
	for _, r := range roles {
		labels = append(labels, r.Label)
	}
	sort.Strings(labels)
	return "Okta roles: " + strings.Join(labels, ", ")
}
