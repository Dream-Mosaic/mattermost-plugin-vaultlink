# Security Policy

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Use GitHub's private vulnerability reporting instead: go to the
[Security tab](https://github.com/dream-mosaic/mattermost-plugin-vaultlink/security)
and click **Report a vulnerability**. This opens a private channel visible only to
the maintainers.

Helpful things to include:

- What the issue is and roughly how severe you think it is
- Steps to reproduce, or a proof of concept
- The Vaultlink version and Mattermost server version
- Anything you already know about a fix

## What to expect

Vaultlink is maintained by a small team, so this is a best-effort policy rather than a
commercial SLA. We aim to acknowledge a report within a week, and we'll keep you posted
on what we find and when a fix is likely. If you'd like credit in the release notes,
say so and we'll include you.

## Scope

Vaultlink handles CouchDB credentials and an end-to-end-encryption passphrase, and
implements path decryption (PBKDF2-SHA256 → HKDF-SHA256 → AES-256-GCM). Issues that are
squarely in scope include:

- Authentication or authorization bypass on any `/api/v1` endpoint
- Exposure of the configured CouchDB password or E2EE passphrase
- Flaws in the key derivation or decryption path
- Anything that would let the plugin write to, or corrupt, a vault — it is designed to
  be strictly read-only against CouchDB

**Out of scope** (report these upstream):

- Mattermost server itself — see
  [Mattermost's security policy](https://mattermost.com/security-vulnerability-report/)
- CouchDB, or [Obsidian Self-Hosted LiveSync](https://github.com/vrtmrz/obsidian-livesync)
- Vulnerabilities requiring System Admin access, since a System Admin can already install
  arbitrary plugins
- Misconfiguration of your own CouchDB instance, such as exposing it without auth
