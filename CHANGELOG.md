# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-09-18

Initial public release.

### Added

- `/obs <note name>` slash command with dynamic autocomplete over a shared Obsidian
  vault. Selecting a note posts an `obsidian://` deep link to the channel.
- In-memory vault index built from the CouchDB database that
  [Obsidian Self-Hosted LiveSync](https://github.com/vrtmrz/obsidian-livesync)
  replicates into. Strictly read-only — `GET` requests only, nothing written to disk.
- End-to-end-encrypted path decryption for vaults using LiveSync path obfuscation
  (PBKDF2-SHA256 → HKDF-SHA256 → AES-256-GCM).
- **Reload Vault** button in System Console → Plugins → Vaultlink for refreshing the
  index on demand. The index also reloads on activation and whenever configuration
  changes.
- Three HTTP endpoints, all requiring a Mattermost session:
  `GET /api/v1/vault-files`, `GET /api/v1/autocomplete`, and `POST /api/v1/refresh`
  (which additionally requires `manage_system`).
- Configuration via System Console: CouchDB URL, username, password, database, E2EE
  passphrase, and vault name.

[1.0.0]: https://github.com/dream-mosaic/mattermost-plugin-vaultlink/releases/tag/v1.0.0
