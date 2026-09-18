package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

// testPassphrase and testPbkdf2Salt are fixed values used across encryption tests.
const testPassphrase = "test-passphrase"

var testPbkdf2Salt = []byte("test-pbkdf2-salt-16b")

// encryptPath builds a "/\:%=<base64>" encrypted path string from a plaintext path,
// mirroring the Obsidian LiveSync E2EE format.
func encryptPath(t *testing.T, passphrase string, pbkdf2Salt []byte, plaintextPath string) string {
	t.Helper()

	masterKey := pbkdf2.Key([]byte(passphrase), pbkdf2Salt, 310000, 32, sha256.New)

	iv := make([]byte, 12)
	_, err := rand.Read(iv)
	require.NoError(t, err)

	hkdfSalt := make([]byte, 32)
	_, err = rand.Read(hkdfSalt)
	require.NoError(t, err)

	hkdfReader := hkdf.New(sha256.New, masterKey, hkdfSalt, nil)
	aesKey := make([]byte, 32)
	_, err = io.ReadFull(hkdfReader, aesKey)
	require.NoError(t, err)

	block, err := aes.NewCipher(aesKey)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)

	payload, err := json.Marshal(map[string]string{"path": plaintextPath})
	require.NoError(t, err)

	ciphertext := gcm.Seal(nil, iv, payload, nil)

	raw := make([]byte, 0, len(iv)+len(hkdfSalt)+len(ciphertext))
	raw = append(raw, iv...)
	raw = append(raw, hkdfSalt...)
	raw = append(raw, ciphertext...)
	return "/\\:%=" + base64.StdEncoding.EncodeToString(raw)
}

// mockCouchServer starts an httptest.Server that serves CouchDB-shaped responses.
// rows is a list of [id, path] pairs. If path starts with "/\:" it is used as-is
// (already an encrypted path string).
func mockCouchServer(t *testing.T, pbkdf2Salt []byte, rows [][]string) *httptest.Server {
	t.Helper()

	saltB64 := base64.StdEncoding.EncodeToString(pbkdf2Salt)

	type docRow struct {
		ID  string `json:"id"`
		Doc struct {
			Path string `json:"path"`
		} `json:"doc"`
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/db/_local/obsidian_livesync_sync_parameters", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"pbkdf2salt": saltB64})
	})
	mux.HandleFunc("/db/_all_docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		docRows := make([]docRow, 0, len(rows))
		for _, r := range rows {
			var row docRow
			row.ID = r[0]
			row.Doc.Path = r[1]
			docRows = append(docRows, row)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rows": docRows})
	})

	return httptest.NewServer(mux)
}

// --- loadVaultCouchDB ---

func TestLoadVaultCouchDB_plainPaths(t *testing.T) {
	srv := mockCouchServer(t, testPbkdf2Salt, [][]string{
		{"Alpha.md", "/Alpha.md"},
		{"Beta Note.md", "/Beta Note.md"},
		{"Folder/Sub.md", "/Folder/Sub.md"},
	})
	defer srv.Close()

	names, _, err := loadVaultCouchDB(http.DefaultClient, srv.URL, "", "", "db", testPassphrase)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Alpha", "Beta Note", "Folder/Sub"}, names)
}

func TestLoadVaultCouchDB_encryptedPaths(t *testing.T) {
	encA := encryptPath(t, testPassphrase, testPbkdf2Salt, "/Alpha.md")
	encB := encryptPath(t, testPassphrase, testPbkdf2Salt, "/Folder/Beta.md")

	srv := mockCouchServer(t, testPbkdf2Salt, [][]string{
		{"enc-a", encA},
		{"enc-b", encB},
	})
	defer srv.Close()

	names, _, err := loadVaultCouchDB(http.DefaultClient, srv.URL, "", "", "db", testPassphrase)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Alpha", "Folder/Beta"}, names)
}

func TestLoadVaultCouchDB_mixed(t *testing.T) {
	encNote := encryptPath(t, testPassphrase, testPbkdf2Salt, "/Encrypted Note.md")

	srv := mockCouchServer(t, testPbkdf2Salt, [][]string{
		{"Plain.md", "/Plain.md"},
		{"enc-note", encNote},
	})
	defer srv.Close()

	names, _, err := loadVaultCouchDB(http.DefaultClient, srv.URL, "", "", "db", testPassphrase)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Plain", "Encrypted Note"}, names)
}

func TestLoadVaultCouchDB_skipsSystemDocs(t *testing.T) {
	srv := mockCouchServer(t, testPbkdf2Salt, [][]string{
		{"_design/idx", "/internal.md"},
		{"_local/settings", "/local.md"},
		{"Real Note.md", "/Real Note.md"},
	})
	defer srv.Close()

	names, _, err := loadVaultCouchDB(http.DefaultClient, srv.URL, "", "", "db", testPassphrase)

	require.NoError(t, err)
	assert.Equal(t, []string{"Real Note"}, names)
}

func TestLoadVaultCouchDB_skipsNonMd(t *testing.T) {
	srv := mockCouchServer(t, testPbkdf2Salt, [][]string{
		{"note.md", "/note.md"},
		{"image.png", "/image.png"},
		{"readme.txt", "/readme.txt"},
		{"no-extension", "/no-extension"},
	})
	defer srv.Close()

	names, _, err := loadVaultCouchDB(http.DefaultClient, srv.URL, "", "", "db", testPassphrase)

	require.NoError(t, err)
	assert.Equal(t, []string{"note"}, names)
}

func TestLoadVaultCouchDB_empty(t *testing.T) {
	srv := mockCouchServer(t, testPbkdf2Salt, [][]string{})
	defer srv.Close()

	names, _, err := loadVaultCouchDB(http.DefaultClient, srv.URL, "", "", "db", testPassphrase)

	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestLoadVaultCouchDB_skipsDeletedDocs(t *testing.T) {
	saltB64 := base64.StdEncoding.EncodeToString(testPbkdf2Salt)

	type docRow struct {
		ID    string `json:"id"`
		Value struct {
			Deleted bool `json:"deleted"`
		} `json:"value"`
		Doc struct {
			Path       string `json:"path"`
			Deleted    bool   `json:"deleted"`
			IsCouchDel bool   `json:"_deleted"`
		} `json:"doc"`
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/db/_local/obsidian_livesync_sync_parameters", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"pbkdf2salt": saltB64})
	})
	mux.HandleFunc("/db/_all_docs", func(w http.ResponseWriter, _ *http.Request) {
		rows := []docRow{
			// value.deleted tombstone
			{ID: "Deleted1.md", Doc: struct {
				Path       string `json:"path"`
				Deleted    bool   `json:"deleted"`
				IsCouchDel bool   `json:"_deleted"`
			}{Path: "/Deleted1.md"}, Value: struct {
				Deleted bool `json:"deleted"`
			}{Deleted: true}},
			// doc.deleted LiveSync soft-delete
			{ID: "Deleted2.md", Doc: struct {
				Path       string `json:"path"`
				Deleted    bool   `json:"deleted"`
				IsCouchDel bool   `json:"_deleted"`
			}{Path: "/Deleted2.md", Deleted: true}},
			// doc._deleted CouchDB flag
			{ID: "Deleted3.md", Doc: struct {
				Path       string `json:"path"`
				Deleted    bool   `json:"deleted"`
				IsCouchDel bool   `json:"_deleted"`
			}{Path: "/Deleted3.md", IsCouchDel: true}},
			// live document — should appear
			{ID: "Live.md", Doc: struct {
				Path       string `json:"path"`
				Deleted    bool   `json:"deleted"`
				IsCouchDel bool   `json:"_deleted"`
			}{Path: "/Live.md"}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"rows": rows})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	names, _, err := loadVaultCouchDB(http.DefaultClient, srv.URL, "", "", "db", testPassphrase)

	require.NoError(t, err)
	assert.Equal(t, []string{"Live"}, names)
}

func TestLoadVaultCouchDB_countsSkippedEncryptedPaths(t *testing.T) {
	srv := mockCouchServer(t, testPbkdf2Salt, [][]string{
		{"note.md", "/note.md"},
		// "/\:" prefix with garbage — looks encrypted but won't decrypt
		{"bad-enc", "/\\:%=!!!notvalidbase64!!!"},
	})
	defer srv.Close()

	names, skipped, err := loadVaultCouchDB(http.DefaultClient, srv.URL, "", "", "db", testPassphrase)

	require.NoError(t, err)
	assert.Equal(t, []string{"note"}, names)
	assert.Equal(t, 1, skipped)
}

// --- vaultIndex.search (behaviour unchanged from filesystem implementation) ---

func TestVaultIndex_search_caseFolding(t *testing.T) {
	v := &vaultIndex{files: []string{"Alpha Note", "Beta Guide", "alpha summary"}}

	assert.ElementsMatch(t, []string{"Alpha Note", "alpha summary"}, v.search("alpha"))
	assert.ElementsMatch(t, []string{"Alpha Note", "alpha summary"}, v.search("ALPHA"))
	assert.ElementsMatch(t, []string{"Alpha Note", "alpha summary"}, v.search("Alpha"))
}

func TestVaultIndex_search_noMatch(t *testing.T) {
	v := &vaultIndex{files: []string{"Alpha Note", "Beta Guide"}}

	result := v.search("zzz")

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestVaultIndex_search_emptyQuery(t *testing.T) {
	v := &vaultIndex{files: []string{"Alpha Note", "Beta Guide", "gamma"}}

	assert.ElementsMatch(t, []string{"Alpha Note", "Beta Guide", "gamma"}, v.search(""))
}

func TestVaultIndex_search_emptyVault(t *testing.T) {
	v := &vaultIndex{}

	result := v.search("anything")

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

// --- decryptPath edge cases ---

func TestDecryptPath_badBase64(t *testing.T) {
	masterKey := pbkdf2.Key([]byte(testPassphrase), testPbkdf2Salt, 310000, 32, sha256.New)

	_, err := decryptPath(masterKey, "%=!!!not-valid-base64!!!")

	assert.Error(t, err)
}

func TestDecryptPath_truncated(t *testing.T) {
	masterKey := pbkdf2.Key([]byte(testPassphrase), testPbkdf2Salt, 310000, 32, sha256.New)

	// 59 bytes is one short of the minimum (IV 12 + HKDF salt 32 + GCM tag 16 = 60).
	short := make([]byte, 59)
	encoded := "%=" + base64.StdEncoding.EncodeToString(short)

	_, err := decryptPath(masterKey, encoded)

	assert.Error(t, err)
}

func TestDecryptPath_wrongPassphrase(t *testing.T) {
	// Encrypt with the correct passphrase.
	encrypted := encryptPath(t, testPassphrase, testPbkdf2Salt, "/Alpha.md")
	// Strip the "/\:" prefix to get the "%=..." portion.
	encoded := encrypted[3:]

	// Decrypt with a different master key.
	wrongKey := pbkdf2.Key([]byte("wrong-passphrase"), testPbkdf2Salt, 310000, 32, sha256.New)
	_, err := decryptPath(wrongKey, encoded)

	assert.Error(t, err)
}

func TestDecryptPath_missingPercentEquals(t *testing.T) {
	masterKey := pbkdf2.Key([]byte(testPassphrase), testPbkdf2Salt, 310000, 32, sha256.New)

	_, err := decryptPath(masterKey, "notencrypted")

	assert.Error(t, err)
}
