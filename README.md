# oktagov2snipe

Syncs active users from an Okta (or Okta-Gov) organization into [Snipe-IT](https://snipeit.app) as license seat assignments. Users are matched by email address. Role assignments are recorded in the seat's notes field.

## Installation

**Download a pre-built binary** (recommended) from the [latest release](https://github.com/jackvaughanjr/oktagov2snipe/releases/latest):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/jackvaughanjr/oktagov2snipe/releases/latest/download/oktagov2snipe-darwin-arm64 -o oktagov2snipe
chmod +x oktagov2snipe

# Linux (amd64)
curl -L https://github.com/jackvaughanjr/oktagov2snipe/releases/latest/download/oktagov2snipe-linux-amd64 -o oktagov2snipe
chmod +x oktagov2snipe

# Linux (arm64)
curl -L https://github.com/jackvaughanjr/oktagov2snipe/releases/latest/download/oktagov2snipe-linux-arm64 -o oktagov2snipe
chmod +x oktagov2snipe
```

Or build from source:

```bash
git clone https://github.com/jackvaughanjr/oktagov2snipe
cd oktagov2snipe
go build -o oktagov2snipe .
```

## Setup

```bash
cp settings.example.yaml settings.yaml
```

Edit `settings.yaml` with your credentials (see [Configuration](#configuration)).

## Requirements

- Okta API token (SSWS) with read access to users and roles
- Snipe-IT API key with license management permissions

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

slack:
  webhook_url: ""                         # optional: Slack incoming webhook URL

sync:
  dry_run: false
  force: false
```

### Environment variable overrides

| Variable        | Config key             |
|-----------------|------------------------|
| `OKTA_URL`      | `okta.url`             |
| `OKTA_TOKEN`    | `okta.api_token`       |
| `SNIPE_URL`     | `snipe_it.url`         |
| `SNIPE_TOKEN`   | `snipe_it.api_key`     |
| `SLACK_WEBHOOK` | `slack.webhook_url`    |

## Usage

### Validate connections

```bash
./oktagov2snipe test
```

Reports active user count, role holders, and current license state in Snipe-IT.

### Dry run

```bash
./oktagov2snipe sync --dry-run -v
```

Simulates a full sync without making any changes. Shows what would be checked out, updated, or checked in.

### Full sync

```bash
./oktagov2snipe sync -v
```

### Sync a single user

```bash
./oktagov2snipe sync --email user@example.com -v
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

## Slack notifications

Set `slack.webhook_url` (or `SLACK_WEBHOOK`) to an [incoming webhook URL](https://api.slack.com/messaging/webhooks) to enable notifications. If omitted, all notifications are silently skipped.

Notifications are suppressed during `--dry-run`. Three events trigger a message:

| Event | Message |
|-------|---------|
| Sync failure | Error details |
| Unmatched user | One message per Okta user with no Snipe-IT account |
| Sync success | Final counts (checked out, notes updated, checked in, skipped, warnings) |

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

## Version History

| Version | Key changes |
|---------|-------------|
| v1.0.0 | Initial build — sync Okta Gov users into Snipe-IT license seats; release workflow; Slack notifications |
| v1.0.1 | Documentation updates for release workflow and CLAUDE.md |
