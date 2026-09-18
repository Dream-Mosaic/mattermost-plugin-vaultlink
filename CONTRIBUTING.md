# Contributing to Vaultlink

## Current status: not accepting code contributions

Vaultlink is built and maintained by a small team for our own use, and we're **not taking
pull requests at the moment**. This isn't a judgement on anyone's code — we just don't
have the bandwidth to review and support external changes properly, and it's fairer to
say so up front than to leave a PR sitting open.

**What is welcome:**

- **Bug reports** — open an issue using the bug report template. The environment
  questions in it aren't bureaucracy; most Vaultlink problems are configuration rather
  than code, and the server log lines usually identify the cause immediately.
- **Security reports** — don't open a public issue. Use
  [private vulnerability reporting](https://github.com/dream-mosaic/mattermost-plugin-vaultlink/security/advisories/new).
  See [SECURITY.md](SECURITY.md).
- **Questions about whether it fits your setup** — an issue is fine.

If you need Vaultlink to do something it doesn't, forking is entirely reasonable — it's
Apache-2.0. The rest of this document is written with that in mind: it covers how to
build it and, more importantly, the invariants that will silently break working installs
if you change them.

This may open up later.

## Development

Requires Go 1.25+ and Node 22.

```bash
make test           # run the Go test suite
make check-style    # golangci-lint
make dist           # build the plugin bundle
```

**Use `make test`, not `go test ./...`.** Running `go test` directly fails with
`undefined: manifest` — `server/manifest.go` is generated from `plugin.json` at build
time and is gitignored. The Makefile generates it first.

For the webapp:

```bash
cd webapp && npm ci
npm run check-types
npm run lint
```

## Things that will break users if you change them

These aren't style preferences. Each one silently breaks working installations.

### The crypto parameters are frozen

Vaultlink decrypts obfuscated vault paths using parameters that must match exactly what
Obsidian Self-Hosted LiveSync used to encrypt them:

- PBKDF2-SHA256, **310,000 iterations**, 32-byte key
- HKDF-SHA256, 32-byte salt, 32-byte key
- AES-256-GCM, 12-byte IV, 128-bit tag

Raising the iteration count to a more modern number is the obvious "improvement" here and
it is **not safe**. These values are dictated by LiveSync, not chosen by us. Changing any
of them means every existing user's paths stop decrypting — and they fail quietly: the
vault index just comes back empty or short, with a skipped-paths count in the logs and no
loud error. There is no migration path short of re-encrypting the vault.

### The autocomplete URL must stay root-relative

`AutocompleteData`'s dynamic list URL must be `/plugins/<plugin-id>/api/v1/autocomplete`,
never absolute. Mattermost string-strips the configured `SiteURL` prefix from absolute
URLs, which breaks on trailing-slash mismatches and behind reverse proxies where the
external hostname doesn't match `SiteURL` character-for-character. Root-relative paths
skip that comparison entirely.

### The plugin is read-only against CouchDB

Only `GET` requests, no writes, no document mutations, and nothing persisted to disk. This
is what guarantees Vaultlink can't corrupt a vault or cause LiveSync replication
conflicts. Please don't add a write path.

### The index locks

`p.vaultIndex` is guarded by a `sync.RWMutex` — reads take `RLock`, reloads take `Lock`.
Autocomplete requests arrive concurrently with reloads, so removing or loosening these
races.

## If contributions open later

The bar will be: focused changes, one concern each, tests for behaviour changes
(the existing suite covers the auth paths, the decryption edge cases, and the index
filtering — follow those patterns), and `make test` plus `make check-style` passing.
Anything touching the invariants above needs to say how you verified existing vaults
still resolve.

## License

Vaultlink is licensed under the [Apache License 2.0](LICENSE). Forks and derivative works
are welcome under those terms.
