package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const obsCommandTrigger = "obs"

// obsidianEncode percent-encodes s for use in obsidian:// URIs.
// url.QueryEscape encodes spaces as '+'; obsidian:// requires '%20'.
func obsidianEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// registerObsCommand registers the /obs slash command with dynamic autocomplete.
// A registration failure is logged but does not prevent the plugin from loading.
func (p *Plugin) registerObsCommand() {
	fetchURL := fmt.Sprintf("/plugins/%s/api/v1/autocomplete", manifest.Id)

	autocompleteData := model.NewAutocompleteData(
		obsCommandTrigger,
		"[file name]",
		"Insert a link to an Obsidian vault file",
	)
	autocompleteData.AddDynamicListArgument("Vault file name", fetchURL, true)

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          obsCommandTrigger,
		AutoComplete:     true,
		AutoCompleteHint: "[file name]",
		AutoCompleteDesc: "Insert a link to an Obsidian vault file",
		AutocompleteData: autocompleteData,
	}); err != nil {
		p.API.LogError("Failed to register /obs command", "error", err.Error())
	}
}

// executeObsCommand handles /obs <file name> and posts an obsidian:// deep link.
func (p *Plugin) executeObsCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	fileName := strings.TrimSpace(strings.TrimPrefix(args.Command, "/"+obsCommandTrigger))
	if fileName == "" {
		return &model.CommandResponse{
			ResponseType: model.CommandResponseTypeEphemeral,
			Text:         "Specify a file name — e.g. `/obs Alpha Note`",
		}, nil
	}

	cfg := p.getConfiguration()
	vaultName := cfg.VaultName

	link := fmt.Sprintf("[%s](obsidian://open?vault=%s&file=%s)",
		fileName,
		obsidianEncode(vaultName),
		obsidianEncode(fileName),
	)

	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeInChannel,
		Text:         link,
	}, nil
}

// handleAutocomplete handles GET /api/v1/autocomplete?user_input=<string>.
// Called by the Mattermost server for /obs dynamic autocomplete suggestions.
// Requires a valid Mattermost session (enforced by router middleware).
// Returns []model.AutocompleteListItem — the shape the server decodes from dynamic list endpoints.
func (p *Plugin) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	rawInput := r.URL.Query().Get("user_input")

	// Mattermost sends the entire line minus the leading slash (e.g., "obs " or "obs alpha")
	// Strip the trigger word and space to isolate what the user is actually searching for.
	searchTerm := rawInput
	if strings.HasPrefix(rawInput, obsCommandTrigger+" ") {
		searchTerm = strings.TrimPrefix(rawInput, obsCommandTrigger+" ")
	} else if rawInput == obsCommandTrigger {
		searchTerm = ""
	}
	searchTerm = strings.TrimSpace(searchTerm)

	matches := p.vaultIndex.search(searchTerm)

	items := make([]model.AutocompleteListItem, 0, len(matches))
	for _, name := range matches {
		items = append(items, model.AutocompleteListItem{Item: name})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(items); err != nil {
		if p.API != nil {
			p.API.LogError("Failed to encode autocomplete response", "error", err)
		}
	}
}
