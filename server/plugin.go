package main

import (
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// router is the HTTP router for handling API requests.
	router *mux.Router

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	// vaultIndex holds the in-memory list of Obsidian vault file names.
	// Zero value is safe; access is guarded by vaultIndex.mu.
	vaultIndex vaultIndex
}

// OnActivate is invoked when the plugin is activated. If an error is returned, the plugin will be deactivated.
func (p *Plugin) OnActivate() error {
	p.router = p.initRouter()

	p.registerObsCommand()

	p.loadVault()

	return nil
}

// OnDeactivate is invoked when the plugin is deactivated.
func (p *Plugin) OnDeactivate() error {
	if err := p.API.UnregisterCommand("", obsCommandTrigger); err != nil {
		p.API.LogError("Failed to unregister /obs command", "error", err.Error())
	}
	return nil
}

// ExecuteCommand dispatches slash commands registered by this plugin.
func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	trigger := ""
	if fields := strings.Fields(args.Command); len(fields) > 0 {
		trigger = strings.TrimPrefix(fields[0], "/")
	}

	switch trigger {
	case obsCommandTrigger:
		return p.executeObsCommand(c, args)
	default:
		return nil, model.NewAppError("ExecuteCommand", "plugin.command.execute_command.app_error", nil,
			"unknown command: "+trigger, http.StatusBadRequest)
	}
}
