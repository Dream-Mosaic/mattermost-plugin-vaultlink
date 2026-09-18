package main

import (
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObsidianEncode(t *testing.T) {
	assert.Equal(t, "Dream%20Mosaic", obsidianEncode("Dream Mosaic"))
	assert.Equal(t, "My%20Alpha%20Note", obsidianEncode("My Alpha Note"))
	assert.Equal(t, "NoSpaces", obsidianEncode("NoSpaces"))
}

func TestExecuteObsCommand_emptyArgs(t *testing.T) {
	p := &Plugin{}
	p.configuration = &configuration{VaultName: "TestVault"}

	resp, appErr := p.executeObsCommand(nil, &model.CommandArgs{Command: "/obs"})

	require.Nil(t, appErr)
	assert.Equal(t, model.CommandResponseTypeEphemeral, resp.ResponseType)
	assert.Contains(t, resp.Text, "/obs Alpha Note")
}

func TestExecuteObsCommand_correctLinkFormat(t *testing.T) {
	p := &Plugin{}
	p.configuration = &configuration{VaultName: "MyVault"}

	resp, appErr := p.executeObsCommand(nil, &model.CommandArgs{Command: "/obs ProjectNotes"})

	require.Nil(t, appErr)
	assert.Equal(t, model.CommandResponseTypeInChannel, resp.ResponseType)
	assert.Equal(t, "[ProjectNotes](obsidian://open?vault=MyVault&file=ProjectNotes)", resp.Text)
}

func TestExecuteObsCommand_spacesInFileName(t *testing.T) {
	p := &Plugin{}
	p.configuration = &configuration{VaultName: "MyVault"}

	resp, appErr := p.executeObsCommand(nil, &model.CommandArgs{Command: "/obs My Alpha Note"})

	require.Nil(t, appErr)
	assert.Equal(t, model.CommandResponseTypeInChannel, resp.ResponseType)
	assert.Equal(t, "[My Alpha Note](obsidian://open?vault=MyVault&file=My%20Alpha%20Note)", resp.Text)
}

func TestExecuteObsCommand_vaultNameFromConfig(t *testing.T) {
	p := &Plugin{}
	p.configuration = &configuration{VaultName: "Dream Mosaic"}

	resp, appErr := p.executeObsCommand(nil, &model.CommandArgs{Command: "/obs SomeFile"})

	require.Nil(t, appErr)
	assert.Equal(t, model.CommandResponseTypeInChannel, resp.ResponseType)
	assert.Equal(t, "[SomeFile](obsidian://open?vault=Dream%20Mosaic&file=SomeFile)", resp.Text)
}

func TestExecuteObsCommand_vaultNameAndFileNameBothEncoded(t *testing.T) {
	p := &Plugin{}
	p.configuration = &configuration{VaultName: "Dream Mosaic"}

	resp, appErr := p.executeObsCommand(nil, &model.CommandArgs{Command: "/obs Chapter One Notes"})

	require.Nil(t, appErr)
	assert.Equal(t, "[Chapter One Notes](obsidian://open?vault=Dream%20Mosaic&file=Chapter%20One%20Notes)", resp.Text)
}

func TestExecuteObsCommand_emptyVaultName(t *testing.T) {
	p := &Plugin{}
	p.configuration = &configuration{VaultName: ""}

	resp, appErr := p.executeObsCommand(nil, &model.CommandArgs{Command: "/obs Alpha Note"})

	require.Nil(t, appErr)
	assert.Equal(t, model.CommandResponseTypeInChannel, resp.ResponseType)
	assert.Equal(t, "[Alpha Note](obsidian://open?vault=&file=Alpha%20Note)", resp.Text)
}
