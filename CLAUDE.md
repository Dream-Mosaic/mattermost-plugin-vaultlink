# Vaultlink — contributor guide

A Mattermost plugin providing a `/obs <note name>` slash command with dynamic
autocomplete over a shared Obsidian vault. Selecting a note posts an `obsidian://`
deep link into the channel.

See [README.md](README.md) for installation and configuration.

---

## Architecture

A **Go backend** plus a small **React webapp** component.

### Go backend

- Maintains an **in-memory vault index**: `p.vaultIndex`, a struct wrapping a
  `[]string` of note names (extension stripped, e.g. `Projects/Roadmap`), guarded
  by a `sync.RWMutex`.
- The index loads from the **CouchDB** database that Obsidian Self-Hosted LiveSync
  replicates into — on activation, on every configuration change
  (`OnConfigurationChange` calls `loadVault`), and on demand via `/refresh`.
- Three HTTP endpoints, all requiring Mattermost session auth:
  - `GET /api/v1/vault-files?query=` — search the index
  - `GET /api/v1/autocomplete?user_input=` — slash command suggestions
  - `POST /api/v1/refresh` — reload the index (additionally requires `manage_system`)

Auth is enforced by the `MattermostAuthorizationRequired` middleware in `server/api.go`,
which rejects requests without the `Mattermost-User-ID` header. Mattermost strips any
client-supplied copy of that header and sets it only for authenticated sessions, so it
cannot be spoofed from outside.

### React webapp

- One component, `ReloadVaultButton`, registered as a custom System Console setting via
  `registry.registerAdminConsoleCustomSetting('ReloadVault', ...)`.
- POSTs to `/api/v1/refresh` with `credentials: 'include'` and an `X-CSRF-Token` header.
- Built with webpack + ts-loader, externals as globals (React, ReactDOM).

---

## Vault index — how it works

### Load sequence

1. **Fetch the PBKDF2 salt** — `GET /{database}/_local/obsidian_livesync_sync_parameters`,
   read the base64 `pbkdf2salt` field.
2. **Fetch all documents** — `GET /{database}/_all_docs?include_docs=true`.
   Both calls use HTTP Basic Auth.
3. **Per row:**
   - Skip ids beginning with `_`
   - Skip rows flagged deleted (`row.Value.Deleted`, `row.Doc.Deleted`, `row.Doc._deleted`)
     — covers both native CouchDB tombstones and LiveSync soft deletes
   - If `doc.path` begins with the obfuscation prefix, decrypt it (below)
   - Keep only `.md` paths; strip the leading `/` and the extension
   - Append to the index

### E2EE path decryption — do not modify

LiveSync obfuscates paths using HKDF (the octagonal-wheels library). The sequence is
rigid; changing any size or iteration count breaks path resolution entirely and would
require re-encrypting the vault.

**Encrypted path format:** `%=` prefix + base64(binary)
**Binary layout:** `[IV 12][HKDF salt 32][AES-GCM ciphertext + 16-byte tag]`

1. `PBKDF2-SHA256(passphrase, pbkdf2Salt, 310000 iterations, 32 bytes)` → `masterKey`
2. `HKDF-SHA256(masterKey as IKM, hkdfSalt as salt, no info, 32 bytes)` → `aesKey`
3. `AES-256-GCM decrypt(iv, aesKey, ciphertext, tag 128 bits)` → plaintext JSON

The payload is `{"path": ..., "mtime": ..., "ctime": ..., "size": ..., "children": [...]}`;
use `path`.

**Frozen:** `ivLen = 12`, `hkdfSaltLen = 32`, PBKDF2 iterations `310000`, PBKDF2 hash
SHA-256, HKDF hash SHA-256, AES-GCM tag 128 bits.

### Index safety constraints

- The index is purely in-memory. No disk writes, no SQLite, no KV store.
- Access is gated by the struct's `sync.RWMutex` — reads `RLock`, reloads `Lock`.
  **Do not remove or loosen these locks**: concurrent autocomplete requests during a
  reload will race without them.
- The plugin is **strictly read-only** against CouchDB — `GET` only, no writes, no
  document mutations. This guarantees it cannot corrupt a vault or cause LiveSync
  replication conflicts.
- Only note *names* are read. Note contents are never fetched or indexed.

---

## Implementation notes

### The SiteURL trap — the autocomplete URL must be root-relative

`AutocompleteData`'s dynamic list URL must be root-relative:

```
/plugins/com.dreammosaic.vaultlink/api/v1/autocomplete
```

**Never absolute.** Mattermost tries to optimize absolute URLs by string-stripping the
configured `SiteURL` prefix, which breaks in two ways:

1. **Trailing slash mismatch** — if SiteURL has a trailing slash and the constructed URL
   doesn't (or vice versa), the strip leaves a malformed path missing its leading `/`,
   and the mux returns 404.
2. **Proxy/hostname boundary** — if the external domain doesn't match SiteURL
   character-for-character (reverse proxy, container network), Mattermost decides the URL
   is external and drops the request.

Root-relative paths skip the SiteURL comparison entirely; Mattermost routes `/plugins`
natively to the plugin's `ServeHTTP` hook.

### Autocomplete prefix stripping

Mattermost sends the **entire command line minus the leading slash** as `user_input`.
Typing `/obs alpha` sends `user_input=obs alpha`, not `alpha`. `handleAutocomplete` must
strip the trigger word before searching:

```go
if strings.HasPrefix(rawInput, obsCommandTrigger+" ") {
    searchTerm = strings.TrimPrefix(rawInput, obsCommandTrigger+" ")
} else if rawInput == obsCommandTrigger {
    searchTerm = ""
}
```

Skip this and the search runs against `"obs alpha"` and returns nothing.

---

## Repo layout

```
├── plugin.json           # manifest: ID, name, settings schema
├── Makefile              # build tooling (make dist, make test)
├── server/               # Go backend
│   ├── main.go           # plugin.ClientMain entry point
│   ├── plugin.go         # Plugin struct, OnActivate/OnDeactivate
│   ├── configuration.go  # config struct, OnConfigurationChange
│   ├── api.go            # HTTP router, handlers, auth middleware
│   ├── vault.go          # CouchDB fetch, HKDF decryption, index
│   ├── obs_command.go    # /obs command + autocomplete handler
│   └── *_test.go
├── webapp/src/index.tsx  # ReloadVaultButton + Plugin class
├── build/                # build tooling from the starter template
└── assets/               # plugin icon
```

`server/manifest.go` and `webapp/src/manifest.ts` are generated from `plugin.json` at
build time and are gitignored.

## Building and testing

```bash
make dist    # bundle
make test    # Go test suite
```

`go test ./server/...` alone fails with `undefined: manifest` — the manifest is
generated by the build. Use `make test`.
