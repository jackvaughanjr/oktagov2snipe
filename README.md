# okta2snipe

Syncs active users from an Okta (or Okta-Gov) organization into [Snipe-IT](https://snipeit.app) as license seat assignments. Users are matched by email address. Role assignments are recorded in the seat's notes field.

## Requirements

- Go 1.22+
- Okta API token (SSWS)
- Snipe-IT API key

## Setup

```bash
git clone https://github.com/jackvaughanjr/okta2snipe
cd okta2snipe
go mod tidy
go build -o okta2snipe .
cp settings.example.yaml settings.yaml
```

Edit `settings.yaml` with your credentials (see [Configuration](#configuration)).

## Configuration

`settings.yaml` is never committed. Use `settings.example.yaml` as the template.

```yaml
okta:
  url: "https://your-org.okta-gov.com"   # or *.okta.com for commercial
  api_token: ""                           # Okta API token (SSWS auth)

snipe_it:
  url: "https://your-snipe-it-instance.example.com"
  api_key: ""
  license_name: "Okta"                   # created automatically if missing
  license_category_id: 0                 # required: Snipe-IT category ID for the license
  license_manufacturer_id: 0             # optional: 0 = auto find/create "Okta" manufacturer
  license_supplier_id: 0                 # optional: 0 = omit from license

sync:
  dry_run: false
  force: false
```

### Environment variable overrides

| Variable       | Config key         |
|----------------|--------------------|
| `OKTA_URL`     | `okta.url`         |
| `OKTA_TOKEN`   | `okta.api_token`   |
| `SNIPE_URL`    | `snipe_it.url`     |
| `SNIPE_TOKEN`  | `snipe_it.api_key` |

## Usage

### Validate connections

```bash
./okta2snipe test
```

Reports active user count, role holders, and current license state in Snipe-IT.

### Dry run

```bash
./okta2snipe sync --dry-run -v
```

Simulates a full sync without making any changes. Shows what would be checked out, updated, or checked in.

### Full sync

```bash
./okta2snipe sync -v
```

### Sync a single user

```bash
./okta2snipe sync --email user@example.com -v
```

## Commands & flags

### `test`

Validates API connectivity and reports current state. No changes are made.

### `sync`

| Flag        | Description                                      |
|-------------|--------------------------------------------------|
| `--dry-run` | Simulate without making changes                  |
| `--force`   | Re-sync even if seat notes appear up to date     |
| `--email`   | Sync a single user by email address              |

### Global flags

| Flag              | Description                                    |
|-------------------|------------------------------------------------|
| `--config`        | Path to config file (default: `settings.yaml`) |
| `-v, --verbose`   | INFO-level logging                             |
| `-d, --debug`     | DEBUG-level logging                            |
| `--log-file`      | Append logs to a file                          |
| `--log-format`    | `text` (default) or `json`                     |

## Sync behavior

- **License**: Found by name in Snipe-IT, or created automatically on first run.
- **Seats**: Expanded automatically if active user count exceeds current seat count. Seats are never shrunk.
- **Checkout**: Active Okta users without a seat are assigned the next available seat. Okta role assignments are recorded in the seat's notes field.
- **Notes update**: If a user's roles change, their seat notes are updated. Use `--force` to re-write notes even if unchanged.
- **Checkin**: Users no longer active in Okta have their seats returned. Skipped when `--email` is used.
- **Unmatched users**: Okta users with no matching Snipe-IT account are warned and skipped — they do not abort the sync.

## User matching

Users are matched by email address (`profile.email`, falling back to `profile.login`). Snipe-IT users are expected to be provisioned via the same Okta tenant (e.g. via LDAP sync), making email a stable match key.

## Role notes format

Okta role labels are written to the Snipe-IT seat's notes field, sorted alphabetically:

```
Okta roles: Internal App Administrator, Super Administrator
```

Empty string if the user has no roles.
