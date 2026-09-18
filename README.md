# Vaultlink

[![CI](https://github.com/dream-mosaic/mattermost-plugin-vaultlink/actions/workflows/ci.yml/badge.svg)](https://github.com/dream-mosaic/mattermost-plugin-vaultlink/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**Link notes from a shared Obsidian vault directly in Mattermost conversations.**

Type `/obs` followed by part of a note's name. Vaultlink autocompletes against your
team's vault and posts a clickable `obsidian://` deep link into the channel — so
"go read the thing" becomes a link someone can actually click, and it opens in their
own local Obsidian.

```
/obs kanban  →  [Kanban](obsidian://open?vault=my-vault&file=Kanban)
```

---

## Requirements

Vaultlink is built for teams who already share an Obsidian vault via CouchDB. You need:

- **Mattermost server** 6.2.1 or later, and System Admin access to install plugins
- **A shared Obsidian vault** synced with
  [Self-Hosted LiveSync](https://github.com/vrtmrz/obsidian-livesync)
- **The CouchDB instance** LiveSync replicates to, reachable from your Mattermost server,
  with read credentials
- **Every team member** to have the vault in their local Obsidian under the *same vault
  name* — `obsidian://` links resolve against the local install, so the name must match

If you sync your vault by git, Dropbox, or filesystem share rather than LiveSync,
Vaultlink won't work as-is — it reads the vault index out of LiveSync's CouchDB
database specifically.

## How it works

Vaultlink keeps an in-memory index of note names, read from the CouchDB database
LiveSync replicates into. If your vault uses end-to-end encryption with path
obfuscation — most LiveSync setups do — file paths are encrypted at rest, so the
plugin decrypts them with your LiveSync passphrase
(PBKDF2-SHA256 → HKDF-SHA256 → AES-256-GCM).

The index is **read-only and in-memory**: Vaultlink only ever issues `GET` requests to
CouchDB, never writes, and nothing is persisted to disk. It cannot corrupt your vault or
cause replication conflicts. Note *contents* are never read — only names.

The index refreshes on activation, whenever the configuration changes, and on demand
from the System Console.

## Installation

1. Download the latest `.tar.gz` from the
   [releases page](https://github.com/dream-mosaic/mattermost-plugin-vaultlink/releases),
   or build it yourself (see [Building](#building)).
2. In Mattermost: **System Console → Plugin Management → Upload Plugin**.
3. Enable the plugin, then configure it.

What changed in each version is listed in [CHANGELOG.md](CHANGELOG.md).

## Configuration

In **System Console → Plugins → Vaultlink**:

| Field | Description |
|-------|-------------|
| **CouchDB URL** | Full URL of the CouchDB instance, e.g. `https://sync.example.com` |
| **CouchDB Username** | CouchDB user with read access to the LiveSync database |
| **CouchDB Password** | Password for that user |
| **CouchDB Database** | Database name used by Obsidian LiveSync |
| **E2EE Passphrase** | Your LiveSync end-to-end encryption passphrase |
| **Vault Name** | Vault name used in `obsidian://` URIs — must match every team member's local Obsidian vault name exactly |

## Usage

In any channel, type `/obs` and start typing a note name. Autocomplete filters the vault
as you type (case-insensitive, matches anywhere in the path). Pick a note and send — the
deep link is posted to the channel.

Clicking it opens that note in the reader's local Obsidian. Anyone without the vault will
see the link but it won't resolve for them.

## Refreshing the vault index

New notes won't appear in autocomplete until the index reloads. Click **Reload Vault** in
**System Console → Plugins → Vaultlink**.

The refresh endpoint requires a Mattermost session with `manage_system` (System Admin).
To call it directly, use an admin
[personal access token](https://developers.mattermost.com/integrate/reference/personal-access-token/):

```bash
curl -X POST https://mattermost.example.com/plugins/com.dreammosaic.vaultlink/api/v1/refresh \
  -H "Authorization: Bearer <admin-personal-access-token>"
```

## Building

Requires Go 1.25+ and Node 22.

```bash
make dist    # bundle -> dist/com.dreammosaic.vaultlink-<version>.tar.gz
make test    # run the Go test suite
```

Note that `go test ./server/...` alone will fail — `server/manifest.go` is generated from
`plugin.json` at build time. Use `make test`.

## Status

Vaultlink is in active use and maintained, but it's built primarily for our own setup.
Bug reports are welcome; we're **not accepting pull requests at the moment** — see
[CONTRIBUTING.md](CONTRIBUTING.md). Forks are fine, it's Apache-2.0.

## Related

Vaultlink is one half of a two-way bridge between Mattermost and Obsidian, built by
[Dream Mosaic](https://dreammosaic.dev). This side brings vault notes into chat; a
companion Obsidian plugin handles the other direction.

## License

Licensed under the [Apache License 2.0](LICENSE).

Built from
[mattermost-plugin-starter-template](https://github.com/mattermost/mattermost-plugin-starter-template),
which is also Apache-2.0; the build tooling in `build/` and `Makefile` derives from it.
