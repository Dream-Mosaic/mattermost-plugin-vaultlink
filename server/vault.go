package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

// couchClient is used for all CouchDB requests. A 30 s timeout prevents hung
// connections from blocking the refresh handler indefinitely.
var couchClient = &http.Client{Timeout: 30 * time.Second}

type vaultIndex struct {
	mu    sync.RWMutex
	files []string
}

// search returns file names containing query (case-insensitive).
// Always returns a non-nil slice so it encodes as JSON [].
func (v *vaultIndex) search(query string) []string {
	v.mu.RLock()
	files := v.files
	v.mu.RUnlock()

	q := strings.ToLower(query)
	result := make([]string, 0, len(files))
	for _, f := range files {
		if q == "" || strings.Contains(strings.ToLower(f), q) {
			result = append(result, f)
		}
	}
	return result
}

// reload fetches the current file list from CouchDB and replaces the index.
// On error the existing index is left unchanged.
// Returns the number of paths that could not be decrypted and were skipped.
func (v *vaultIndex) reload(client *http.Client, couchURL, username, password, database, passphrase string) (int, error) {
	files, skipped, err := loadVaultCouchDB(client, couchURL, username, password, database, passphrase)
	if err != nil {
		return 0, err
	}
	v.mu.Lock()
	v.files = files
	v.mu.Unlock()
	return skipped, nil
}

// count returns the number of files currently in the index.
func (v *vaultIndex) count() int {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return len(v.files)
}

// loadVault reloads the vault from CouchDB using the current configuration.
// Logs on error; does not crash.
func (p *Plugin) loadVault() {
	cfg := p.getConfiguration()
	if cfg.CouchDBURL == "" {
		return
	}
	skipped, err := p.vaultIndex.reload(couchClient, cfg.CouchDBURL, cfg.CouchDBUsername, cfg.CouchDBPassword, cfg.CouchDBDatabase, cfg.CouchDBPassphrase)
	if err != nil {
		p.API.LogError("Failed to load vault", "error", err.Error())
		return
	}
	if skipped > 0 {
		p.API.LogWarn("Vault load skipped undecryptable paths", "count", skipped)
	}
}

// couchSyncParams holds the fields we need from the LiveSync sync parameters doc.
type couchSyncParams struct {
	Pbkdf2Salt string `json:"pbkdf2salt"`
}

// couchAllDocsResponse is the top-level shape of /_all_docs?include_docs=true.
type couchAllDocsResponse struct {
	Rows []struct {
		ID    string `json:"id"`
		Value struct {
			Deleted bool `json:"deleted"`
		} `json:"value"`
		Doc struct {
			Path       string `json:"path"`
			Deleted    bool   `json:"deleted"`
			IsCouchDel bool   `json:"_deleted"`
		} `json:"doc"`
	} `json:"rows"`
}

// loadVaultCouchDB fetches all documents from the LiveSync CouchDB database,
// decrypts obfuscated paths, and returns the list of .md file names without extension.
// It is read-only: no documents are written or modified.
// The second return value is the count of paths that could not be decrypted and were skipped.
func loadVaultCouchDB(client *http.Client, couchURL, username, password, database, passphrase string) ([]string, int, error) {
	pbkdf2Salt, err := fetchSyncParams(client, couchURL, username, password, database)
	if err != nil {
		return nil, 0, fmt.Errorf("sync params: %w", err)
	}

	allDocs, err := fetchAllDocs(client, couchURL, username, password, database)
	if err != nil {
		return nil, 0, fmt.Errorf("all docs: %w", err)
	}

	masterKey := pbkdf2.Key([]byte(passphrase), pbkdf2Salt, 310000, 32, sha256.New)

	var names []string
	var skipped int
	for _, row := range allDocs.Rows {
		if strings.HasPrefix(row.ID, "_") {
			continue
		}

		// Skip files that have been natively deleted or flagged as soft-deleted by LiveSync
		if row.Value.Deleted || row.Doc.Deleted || row.Doc.IsCouchDel {
			continue
		}

		path := row.Doc.Path

		// "/\:" prefix marks an E2EE-obfuscated path.
		if strings.HasPrefix(path, "/\\:") {
			decrypted, decErr := decryptPath(masterKey, path[3:])
			if decErr != nil {
				skipped++
				continue
			}
			path = decrypted
		}

		lower := strings.ToLower(path)
		if !strings.HasSuffix(lower, ".md") {
			continue
		}

		path = strings.TrimPrefix(path, "/")
		path = path[:len(path)-3] // strip ".md"
		names = append(names, path)
	}
	return names, skipped, nil
}

// fetchSyncParams retrieves the PBKDF2 salt from the LiveSync sync parameters document.
func fetchSyncParams(client *http.Client, couchURL, username, password, database string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/_local/obsidian_livesync_sync_parameters", couchURL, database)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var params couchSyncParams
	if jsonErr := json.NewDecoder(resp.Body).Decode(&params); jsonErr != nil {
		return nil, jsonErr
	}

	salt, err := base64.StdEncoding.DecodeString(params.Pbkdf2Salt)
	if err != nil {
		return nil, fmt.Errorf("pbkdf2salt base64: %w", err)
	}
	return salt, nil
}

// fetchAllDocs retrieves all documents from the database.
func fetchAllDocs(client *http.Client, couchURL, username, password, database string) (*couchAllDocsResponse, error) {
	url := fmt.Sprintf("%s/%s/_all_docs?include_docs=true", couchURL, database)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, password)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result couchAllDocsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// decryptPath decrypts an Obsidian LiveSync E2EE-obfuscated path.
// encoded is the portion after the "/\:" prefix, in the form "%=<base64>".
// The decoded binary layout is: [IV 12 bytes][HKDF salt 32 bytes][AES-GCM ciphertext+tag].
// The plaintext is a JSON object with a "path" field.
func decryptPath(masterKey []byte, encoded string) (string, error) {
	if !strings.HasPrefix(encoded, "%=") {
		return "", fmt.Errorf("unexpected format: missing %%=")
	}

	raw, err := base64.StdEncoding.DecodeString(encoded[2:])
	if err != nil {
		return "", fmt.Errorf("base64: %w", err)
	}

	const ivLen = 12
	const hkdfSaltLen = 32
	const minLen = ivLen + hkdfSaltLen + 16 // +16 for GCM tag minimum
	if len(raw) < minLen {
		return "", fmt.Errorf("encrypted data too short: %d bytes", len(raw))
	}

	iv := raw[:ivLen]
	hkdfSalt := raw[ivLen : ivLen+hkdfSaltLen]
	ciphertext := raw[ivLen+hkdfSaltLen:]

	hkdfReader := hkdf.New(sha256.New, masterKey, hkdfSalt, nil)
	aesKey := make([]byte, 32)
	if _, readErr := io.ReadFull(hkdfReader, aesKey); readErr != nil {
		return "", fmt.Errorf("hkdf: %w", readErr)
	}

	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm: %w", err)
	}

	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	var result struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(plaintext, &result); err != nil {
		return "", fmt.Errorf("json: %w", err)
	}
	return result.Path, nil
}
