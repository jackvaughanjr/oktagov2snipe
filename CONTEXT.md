# okta2snipe — Integration Context

## Purpose

Syncs active users from an **Okta-Gov** (or commercial Okta) organisation into
[Snipe-IT](https://snipeit.app) as license seat assignments. Designed to follow
the same pattern as the other `*2snipe` integrations in this org.

---

## Repo

**GitHub:** https://github.com/jackvaughanjr/okta2snipe  
**Module:** `github.com/jackvaughanjr/okta2snipe`

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
  sync.go            # sync command: --dry-run, --force, --email flags
  test.go            # test command: reports active user count, role holders, license state
internal/
  okta/
    client.go        # Okta REST client (see below)
  snipeit/
    client.go        # Snipe-IT API client (see below)
  sync/
    syncer.go        # core sync logic
    result.go        # Result struct with CheckedOut/NotesUpdated/CheckedIn/Skipped/Warnings
go.mod
go.sum
settings.example.yaml
README.md
CONTEXT.md
.gitignore           # excludes: settings.yaml, okta2snipe binary, .cache/, *.log
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

sync:
  dry_run: false
  force: false
  rate_limit_ms: 500
```

### Environment variable overrides

| Variable      | Config key          |
|---------------|---------------------|
| `OKTA_URL`    | `okta.url`          |
| `OKTA_TOKEN`  | `okta.api_token`    |
| `SNIPE_URL`   | `snipe_it.url`      |
| `SNIPE_TOKEN` | `snipe_it.api_key`  |

---

## Commands & Flags

### Commands

| Command | Description |
|---------|-------------|
| `test`  | Validate API connections; report active user count, role holders, license state |
| `sync`  | Run the sync |

### Sync flags

| Flag        | Description                              |
|-------------|------------------------------------------|
| `--dry-run` | Simulate without making changes          |
| `--force`   | Re-sync even if notes appear up to date  |
| `--email`   | Sync a single user by email address      |

### Global flags

| Flag            | Description                                    |
|-----------------|------------------------------------------------|
| `--config`      | Path to config file (default: `settings.yaml`) |
| `-v, --verbose` | INFO-level logging                             |
| `-d, --debug`   | DEBUG-level logging                            |
| `--log-file`    | Append logs to a file                          |
| `--log-format`  | `text` (default) or `json`                     |

### Important: verbose/debug flag implementation

`--verbose` and `--debug` are wired via `PersistentPreRunE` on the root command,
**not** `cobra.OnInitialize`. `OnInitialize` fires before flag parsing and cannot
read flag values. `PersistentPreRunE` fires after — do not move logging init back
to `OnInitialize`.

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

### Pagination
Okta uses `Link: <url>; rel="next"` response headers. The client parses this
header and follows it automatically — no cursor tracking needed in callers.

---

## internal/snipeit/client.go

Snipe-IT REST API client. Rate-limited to 2 req/s via `golang.org/x/time/rate`.

### Auth
`Authorization: Bearer <api_key>` header on every request.

### Important envelope note
- **POST / PATCH** responses are wrapped: `{ "status": "success", "messages": {}, "payload": { ... } }`
- **GET** responses are the object directly (no envelope)
- Always check `env.Status == "success"` after POST/PATCH — a 200 response can still carry `"status": "error"` with validation messages. `CreateLicense`, `CreateManufacturer`, and `CheckoutSeat` all do this.

### Methods

| Method | Description |
|--------|-------------|
| `FindLicenseByName(ctx, name)` | Search by exact name; returns `nil, nil` if not found |
| `FindLicenseByID(ctx, id)` | Fetch by numeric ID |
| `CreateLicense(ctx, name, seats, categoryID, manufacturerID, supplierID)` | Create a new license. `categoryID` required; pass 0 to omit optional IDs |
| `FindOrCreateLicense(ctx, name, initialSeats, categoryID, manufacturerID, supplierID)` | Find or create |
| `UpdateLicenseSeats(ctx, licenseID, seats)` | Expand (or change) seat count |
| `ListLicenseSeats(ctx, licenseID)` | Returns `[]LicenseSeat` (up to 500) |
| `CheckoutSeat(ctx, licenseID, seatID, userID, notes)` | Assign seat to user via **PATCH** (not POST — 405 otherwise) |
| `CheckinSeat(ctx, licenseID, seatID)` | Return seat (DELETE endpoint) |
| `UpdateSeatNotes(ctx, licenseID, seatID, notes)` | PATCH notes on existing checkout |
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
5. **Resolve manufacturer** — if `ManufacturerID == 0`, call `snipe.FindOrCreateManufacturer("Okta", "https://www.okta.com")`. Skipped in dry-run.
6. **Find or create license** — dry-run uses `FindLicenseByName` only; synthesizes a placeholder license (id=0) if not found so the rest of the logic can run. Production uses `FindOrCreateLicense`.
7. **Expand seats if needed** — if `activeCount > license.Seats`, call `UpdateLicenseSeats(activeCount)`. Seats are **never shrunk automatically**.
8. **Load current seat assignments** — `snipe.ListLicenseSeats()`; partition into `checkedOutByEmail` map and `freeSeats` slice. Skipped for synthetic dry-run license (id=0). Fails fast in production if id=0.
9. **Checkout / update loop** — for each active user:
   - Find Snipe-IT user by email; warn + skip if not found (TODO: Slack notification)
   - If already checked out: compare notes; update if changed (or `--force`)
   - If not checked out: dry-run logs and counts; production pops a free seat and calls `CheckoutSeat`
10. **Checkin loop** (skipped when `--email` is set) — for each seat checked out to an email not in the active set: `CheckinSeat`

### Role notes format

```
Okta roles: Label1, Label2, Label3
```
Labels are sorted alphabetically. Empty string if the user has no roles.

### Result struct

```go
type Result struct {
    CheckedOut   int  // seats newly assigned
    NotesUpdated int  // seats whose notes were updated
    CheckedIn    int  // seats returned for inactive users
    Skipped      int  // users already up to date
    Warnings     int  // users with no matching Snipe-IT account, or API errors
}
```

---

## Pending TODOs

- `cmd/sync.go`: Slack notification on sync failure (with error details)
- `cmd/sync.go`: Slack notification on sync success (with result stats)
- `internal/sync/syncer.go`: Slack notification on unmatched user (with email)

---

## User Matching

Users are matched by **email address**. Snipe-IT users are provisioned via
Okta LDAP sync using the same Okta tenant, so the email in Okta is a stable
and reliable match key.

`emailKey(user)` prefers `profile.email`; falls back to `profile.login`.
All comparisons are lowercased.

---

## Building & Running

```bash
git clone https://github.com/jackvaughanjr/okta2snipe
cd okta2snipe
go mod tidy
go build -o okta2snipe .

# Validate connections
./okta2snipe test

# Dry run (no changes)
./okta2snipe sync --dry-run -v

# Full sync
./okta2snipe sync -v

# Single user
./okta2snipe sync --email user@example.com -v
```

---

## Notes for Claude Code

- `go.sum` will be generated by `go mod tidy` — do not create it manually
- `settings.yaml` is gitignored — never commit it; use `settings.example.yaml` as the template
- The binary name matches the repo: `okta2snipe`
- Snipe-IT POST/PATCH responses use the `{ status, messages, payload }` envelope — always check `env.Status == "success"` after decoding, not just the HTTP status code
- Okta's `/api/v1/users/{id}/roles` returns HTTP 403 for regular (non-admin) users in some org configurations — this is **not** an error; treat it as an empty role list
- Okta filter queries must use percent-encoded characters (`%20` for space, `%22` for quotes) — literal quotes in the URL cause a 400
- `CheckoutSeat` uses **PATCH**, not POST — POST returns 405
- `license_category_id` is required by Snipe-IT to create a license — the sync command validates this before starting
- Rate limit Snipe-IT calls to 2 req/s (`rate.Every(500ms)`) to avoid 429s
- `--verbose` / `--debug` flags must be initialized in `PersistentPreRunE`, not `cobra.OnInitialize` — `OnInitialize` runs before flag parsing
