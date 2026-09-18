package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// initRouter sets up the HTTP router for the plugin.
// All /api/v1 endpoints require a valid Mattermost session.
func (p *Plugin) initRouter() *mux.Router {
	root := mux.NewRouter()

	api := root.PathPrefix("/api/v1").Subrouter()
	api.Use(p.MattermostAuthorizationRequired)
	api.HandleFunc("/refresh", p.handleRefresh).Methods(http.MethodPost)
	api.HandleFunc("/vault-files", p.handleVaultFiles).Methods(http.MethodGet)
	api.HandleFunc("/autocomplete", p.handleAutocomplete).Methods(http.MethodGet)

	return root
}

// ServeHTTP routes all plugin HTTP traffic through the mux router.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

// MattermostAuthorizationRequired rejects requests without a Mattermost-User-ID header.
func (p *Plugin) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Mattermost-User-ID") == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type vaultFileResult struct {
	Name string `json:"name"`
}

// handleVaultFiles handles GET /api/v1/vault-files?query=<string>.
// Requires a valid Mattermost session (enforced by router middleware).
// Returns a JSON array of matching file names; always an array, never null.
func (p *Plugin) handleVaultFiles(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	matches := p.vaultIndex.search(query)

	results := make([]vaultFileResult, 0, len(matches))
	for _, name := range matches {
		results = append(results, vaultFileResult{Name: name})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		p.API.LogError("Failed to encode vault-files response", "error", err)
	}
}

// handleRefresh handles POST /api/v1/refresh.
// Requires an authenticated Mattermost session with manage_system permission.
func (p *Plugin) handleRefresh(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if !p.API.HasPermissionTo(userID, model.PermissionManageSystem) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Forbidden"})
		return
	}

	cfg := p.getConfiguration()
	if cfg.CouchDBURL != "" {
		skipped, err := p.vaultIndex.reload(couchClient, cfg.CouchDBURL, cfg.CouchDBUsername, cfg.CouchDBPassword, cfg.CouchDBDatabase, cfg.CouchDBPassphrase)
		if err != nil {
			p.API.LogError("Vault rescan failed", "error", err.Error())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Vault rescan failed"})
			return
		}
		if skipped > 0 {
			p.API.LogWarn("Vault refresh skipped undecryptable paths", "count", skipped)
		}
	}

	count := p.vaultIndex.count()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": fmt.Sprintf("Vault reloaded. %d files indexed.", count),
	})
}
