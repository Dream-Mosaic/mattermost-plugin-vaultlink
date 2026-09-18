package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPluginAPI stubs the Mattermost plugin API for handler tests.
type mockPluginAPI struct {
	plugin.API
	hasPermission bool
}

func (m *mockPluginAPI) HasPermissionTo(_ string, _ *model.Permission) bool {
	return m.hasPermission
}

func (m *mockPluginAPI) LogError(_ string, _ ...any) {}
func (m *mockPluginAPI) LogWarn(_ string, _ ...any)  {}

// newTestPlugin returns a Plugin wired for HTTP handler tests. The vaultIndex and
// configuration fields are set by each test as needed.
func newTestPlugin() *Plugin {
	p := &Plugin{}
	p.router = p.initRouter()
	return p
}

// newTestPluginWithAPI returns a Plugin with a mock API set for permission checks.
func newTestPluginWithAPI(isAdmin bool) *Plugin {
	p := &Plugin{}
	p.API = &mockPluginAPI{hasPermission: isAdmin}
	p.router = p.initRouter()
	return p
}

// --- /api/v1/vault-files ---

func TestVaultFiles_authenticated(t *testing.T) {
	p := newTestPlugin()
	p.vaultIndex.files = []string{"Alpha Note", "Beta Guide"}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/vault-files", nil)
	r.Header.Set("Mattermost-User-ID", "user1")
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var results []vaultFileResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))
	assert.Len(t, results, 2)
}

func TestVaultFiles_unauthenticated(t *testing.T) {
	p := newTestPlugin()

	r := httptest.NewRequest(http.MethodGet, "/api/v1/vault-files", nil)
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestVaultFiles_emptyVault(t *testing.T) {
	p := newTestPlugin()
	// vaultIndex is zero value — no files loaded

	r := httptest.NewRequest(http.MethodGet, "/api/v1/vault-files", nil)
	r.Header.Set("Mattermost-User-ID", "user1")
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var results []vaultFileResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))
	assert.NotNil(t, results)
	assert.Empty(t, results)
}

func TestVaultFiles_queryFilter(t *testing.T) {
	p := newTestPlugin()
	p.vaultIndex.files = []string{"Alpha Note", "Beta Guide", "alpha summary"}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/vault-files?query=alpha", nil)
	r.Header.Set("Mattermost-User-ID", "user1")
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var results []vaultFileResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &results))
	require.Len(t, results, 2)
	names := []string{results[0].Name, results[1].Name}
	assert.ElementsMatch(t, []string{"Alpha Note", "alpha summary"}, names)
}

// --- /api/v1/refresh ---

func TestRefresh_unauthenticated(t *testing.T) {
	p := newTestPlugin()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", nil)
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRefresh_nonAdmin(t *testing.T) {
	p := newTestPluginWithAPI(false)

	r := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", nil)
	r.Header.Set("Mattermost-User-ID", "user1")
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRefresh_updatesCache(t *testing.T) {
	// Start a minimal CouchDB mock serving a single plain-path document.
	saltB64 := base64.StdEncoding.EncodeToString([]byte("test-salt"))
	mux := http.NewServeMux()
	mux.HandleFunc("/vaultdb/_local/obsidian_livesync_sync_parameters", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"pbkdf2salt": saltB64})
	})
	mux.HandleFunc("/vaultdb/_all_docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rows": []map[string]any{
				{"id": "My Note.md", "doc": map[string]string{"path": "/My Note.md"}},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestPluginWithAPI(true)
	p.configuration = &configuration{
		CouchDBURL:      srv.URL,
		CouchDBDatabase: "vaultdb",
	}

	// Vault is empty before the refresh.
	assert.Empty(t, p.vaultIndex.search(""))

	r := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", nil)
	r.Header.Set("Mattermost-User-ID", "admin1")
	w := httptest.NewRecorder()
	p.ServeHTTP(nil, w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"My Note"}, p.vaultIndex.search(""))
}

func TestRefresh_couchDBError(t *testing.T) {
	saltB64 := base64.StdEncoding.EncodeToString([]byte("test-salt"))
	mux := http.NewServeMux()
	mux.HandleFunc("/vaultdb/_local/obsidian_livesync_sync_parameters", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"pbkdf2salt": saltB64})
	})
	mux.HandleFunc("/vaultdb/_all_docs", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p := newTestPluginWithAPI(true)
	p.configuration = &configuration{
		CouchDBURL:      srv.URL,
		CouchDBDatabase: "vaultdb",
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", nil)
	r.Header.Set("Mattermost-User-ID", "admin1")
	w := httptest.NewRecorder()
	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// --- /api/v1/autocomplete ---

func TestAutocomplete_authenticated(t *testing.T) {
	p := newTestPlugin()
	p.vaultIndex.files = []string{"Alpha Note", "Beta Guide"}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete", nil)
	r.Header.Set("Mattermost-User-ID", "user1")
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var items []model.AutocompleteListItem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	assert.Len(t, items, 2)
}

func TestAutocomplete_unauthenticated(t *testing.T) {
	p := newTestPlugin()

	r := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete", nil)
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAutocomplete_queryFilter(t *testing.T) {
	p := newTestPlugin()
	p.vaultIndex.files = []string{"Alpha Note", "Beta Guide", "alpha summary"}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete?user_input=alpha", nil)
	r.Header.Set("Mattermost-User-ID", "user1")
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var items []model.AutocompleteListItem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	require.Len(t, items, 2)
	names := []string{items[0].Item, items[1].Item}
	assert.ElementsMatch(t, []string{"Alpha Note", "alpha summary"}, names)
}

func TestAutocomplete_leadingSpaceTrimmed(t *testing.T) {
	p := newTestPlugin()
	p.vaultIndex.files = []string{"Alpha Note", "Beta Guide"}

	// The server sends user_input with a leading space before the typed text.
	r := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete?user_input=+Alpha", nil)
	r.Header.Set("Mattermost-User-ID", "user1")
	w := httptest.NewRecorder()

	p.ServeHTTP(nil, w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var items []model.AutocompleteListItem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "Alpha Note", items[0].Item)
}
