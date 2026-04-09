# oktagov2snipe — Integration Context

Cross-cutting conventions, API patterns, and Snipe-IT quirks live in `CLAUDE.md`.
This file contains only what is specific to the Okta → Snipe-IT integration.

---

## Purpose

Syncs active users from an **Okta-Gov** (or commercial Okta) organisation into
[Snipe-IT](https://snipeit.app) as license seat assignments. Designed to follow
the same pattern as the other `*2snipe` integrations in this org.

---

## Repo

**GitHub:** https://github.com/jackvaughanjr/oktagov2snipe  
**Module:** `github.com/jackvaughanjr/oktagov2snipe`

---

## Stack

- **Language:** Go 1.22+
- **CLI framework:** `cobra` + `viper`
- **Config:** `settings.yaml` (gitignored), env var overrides
- **Logging:** `log/slog` — structured, levelled, text or JSON output
- **Rate limiting:** `golang.org/x/time/rate` — 2 req/s to Snipe-IT

---

## File Structure

```
main.go
cmd/
  root.go            # CLI setup, cobra/viper init, logging, env var bindings
  sync.go            # sync command: --dry-run, --force, --email flags; Slack notifications
  test.go            # test command: reports active user count, role holders, license state
internal/
  okta/
    client.go        # Okta REST client (see below)
  slack/
    client.go        # Slack incoming-webhook client
  snipeit/
    client.go        # Snipe-IT API client (see below)
  sync/
    syncer.go        # core sync logic
    result.go        # Result struct
go.mod
go.sum
settings.example.yaml
README.md
CONTEXT.md
.gitignore           # excludes: settings.yaml, oktagov2snipe binary, .cache/, *.log
```

---

## Configuration

**`settings.yaml`** (never committed — see `settings.example.yaml` for template):

```yaml
okta:
  url: "https://your-org.okta-gov.com"   # or *.okta.com for commercial
  api_token: ""                           # Okta API token (SSWS auth)

snipe_it:
  url: "https://your-snipe-it-instance.example.com"
  api_key: ""
  license_name: "Okta"                   # created automatically if missing
  license_category_id: 0                 # required: Snipe-IT category ID
  license_manufacturer_id: 0             # optional: 0 = auto find/create "Okta" manufacturer
  license_supplier_id: 0                 # optional: 0 = omit from license

slack:
  webhook_url: ""                         # optional: see CLAUDE.md for Slack behavior

sync:
  dry_run: false
  force: false
  rate_limit_ms: 500
```

### Environment variable overrides

| Variable        | Config key          |
|-----------------|---------------------|
| `OKTA_URL`      | `okta.url`          |
| `OKTA_TOKEN`    | `okta.api_token`    |
| `SNIPE_URL`     | `snipe_it.url`      |
| `SNIPE_TOKEN`   | `snipe_it.api_key`  |
| `SLACK_WEBHOOK` | `slack.webhook_url` |

---

## internal/okta/client.go

Lightweight Okta REST API client. No external Okta SDK — plain `net/http`.

### Auth

`Authorization: SSWS <api_token>` header on every request.  
Works identically against `*.okta-gov.com` and `*.okta.com`.

### Methods

| Method | Endpoint | Notes |
|--------|----------|-------|
| `ListActiveUsers(ctx)` | `GET /api/v1/users?filter=status%20eq%20%22ACTIVE%22&limit=200` | Follows `Link: rel="next"` pagination. Quotes must be percent-encoded — literal `"` causes a 400. |
| `ListAllUsers(ctx)` | `GET /api/v1/users?limit=200` | All statuses; used for checkin pass |
| `GetUserByEmail(ctx, email)` | `GET /api/v1/users/{email}` | Single user lookup |
| `GetUserRoles(ctx, userID)` | `GET /api/v1/users/{id}/roles` | Returns `[]Role`; Okta 403 → treated as no roles (not an error) |

### Okta API quirks

- **Filter URL encoding**: Filter query strings must use percent-encoding (`%20` for
  space, `%22` for quotes). Literal quotes in the URL cause a 400 error.
- **403 on roles**: `GET /api/v1/users/{id}/roles` returns HTTP 403 for regular
  (non-admin) users in some org configurations. This is **not** an error — treat it
  as an empty role list and continue.
- **Pagination**: Okta uses `Link: <url>; rel="next"` response headers. The client
  parses this header and follows it automatically — no cursor tracking needed in callers.

### Types

```go
type User struct {
    ID      string
    Status  string
    Profile UserProfile // Login, Email, FirstName, LastName
}

type Role struct {
    ID    string
    Type  string  // e.g. "SUPER_ADMIN", "ORG_ADMIN", "APP_ADMIN"
    Label string  // human-readable
}
```

---

## internal/snipeit/client.go

Snipe-IT REST API client. Rate-limited to 2 req/s via `golang.org/x/time/rate`.
See `CLAUDE.md` for envelope behavior, rate limiting, checkout/checkin rules, and
FindOrCreate patterns — those apply to all integrations.

### Methods

| Method | Description |
|--------|-------------|
| `FindLicenseByName(ctx, name)` | Search by exact name; returns `nil, nil` if not found |
| `FindLicenseByID(ctx, id)` | Fetch by numeric ID |
| `CreateLicense(ctx, name, seats, categoryID, manufacturerID, supplierID)` | Create a new license; `categoryID` required, pass 0 to omit optional IDs |
| `FindOrCreateLicense(ctx, name, initialSeats, categoryID, manufacturerID, supplierID)` | Find or create |
| `UpdateLicenseSeats(ctx, licenseID, seats)` | Expand (or change) seat count |
| `ListLicenseSeats(ctx, licenseID)` | Returns `[]LicenseSeat` (up to 500) |
| `CheckoutSeat(ctx, licenseID, seatID, userID, notes)` | Assign seat to user via PATCH |
| `CheckinSeat(ctx, licenseID, seatID)` | Return seat (DELETE endpoint) |
| `UpdateSeatNotes(ctx, licenseID, seatID, notes)` | PATCH notes on an existing checkout |
| `FindUserByEmail(ctx, email)` | Search Snipe-IT users; returns `nil, nil` if not found |
| `FindManufacturerByName(ctx, name)` | Search by exact name; returns `nil, nil` if not found |
| `CreateManufacturer(ctx, name, url)` | Create a new manufacturer record |
| `FindOrCreateManufacturer(ctx, name, url)` | Find or create |

### Types

```go
type License struct {
    ID             int
    Name           string
    Seats          int
    FreeSeatsCount int
}

type LicenseSeat struct {
    ID         int
    LicenseID  int
    AssignedTo *AssignedTo  // nil if free
    Notes      string
}

type SnipeUser struct {
    ID       int
    Name     string
    Username string
    Email    string
}

type Manufacturer struct {
    ID   int
    Name string
    URL  string
}
```

---

## internal/sync/syncer.go — Sync Logic

### Config

```go
type Config struct {
    DryRun            bool
    Force             bool
    LicenseName       string
    LicenseCategoryID int  // required
    ManufacturerID    int  // 0 = auto find/create "Okta" manufacturer
    SupplierID        int  // 0 = omit
}
```

### Run(ctx, emailFilter) flow — 10 steps

1. **Fetch active Okta users** — `okta.ListActiveUsers()`, paginated
2. **Build active email set** — used for the checkin pass in step 10
3. **Apply `--email` filter** — if set, narrow to one user
4. **Fetch roles per user** — `okta.GetUserRoles()`; 403 → no roles (not fatal)
5. **Resolve manufacturer** — if `ManufacturerID == 0`, call
   `snipe.FindOrCreateManufacturer("Okta", "https://www.okta.com")`. Skipped in dry-run.
6. **Find or create license** — dry-run uses `FindLicenseByName` only; synthesizes a
   placeholder `&License{Name: ..., Seats: activeCount}` (id=0) if not found.
   Production uses `FindOrCreateLicense`.
7. **Expand seats if needed** — if `activeCount > license.Seats`, call
   `UpdateLicenseSeats(activeCount)`. Seats are **never shrunk automatically**.
8. **Load current seat assignments** — `snipe.ListLicenseSeats()`; partition into
   `checkedOutByEmail` map and `freeSeats` slice. Skipped for synthetic dry-run
   license (id=0). Fails fast in production if id=0.
9. **Checkout / update loop** — for each active user:
   - Find Snipe-IT user by email; warn + skip + append to `result.UnmatchedEmails` if not found
   - If already checked out: compare notes; update if changed (or `--force`)
   - If not checked out: dry-run logs and counts; production pops a free seat and
     calls `CheckoutSeat`
10. **Checkin loop** (skipped when `--email` is set) — for each seat checked out to
    an email not in the active set: `CheckinSeat`

### Role notes format

Okta role labels are written to the seat's `notes` field, sorted alphabetically:

```
Okta roles: Label1, Label2, Label3
```

Empty string if the user has no roles.

### User matching

`emailKey(user)` prefers `profile.email`; falls back to `profile.login`.
All comparisons are lowercased.

---

## Slack notification messages

Messages sent by `cmd/sync.go` for this integration (see `CLAUDE.md` for the
general Slack pattern — when to send, how errors are handled, dry-run suppression):

| Event | Message |
|-------|---------|
| Sync failure | `oktagov2snipe sync failed: <error>` |
| Unmatched user | `oktagov2snipe: no Snipe-IT account found for Okta user — <email>` (one per user) |
| Sync success | `oktagov2snipe sync complete — checked out: N, notes updated: N, checked in: N, skipped: N, warnings: N` |

---

## Building & Running

```bash
git clone https://github.com/jackvaughanjr/oktagov2snipe
cd oktagov2snipe
go mod tidy
go build -o oktagov2snipe .

# Validate connections
./oktagov2snipe test

# Dry run (no changes)
./oktagov2snipe sync --dry-run -v

# Full sync
./oktagov2snipe sync -v

# Single user
./oktagov2snipe sync --email user@example.com -v
```
