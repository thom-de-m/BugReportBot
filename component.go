package main

import (
	"github.com/bwmarrin/discordgo"
)

const (
	bugReportButtonID = "report_btn"
)

// This takes care of the slash command and interactions for the button that can be setup
func handleInteractions(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	switch interaction.Type {
	case discordgo.InteractionMessageComponent:
		if interaction.MessageComponentData().CustomID != bugReportButtonID {
			return
		}

		// From here we should always respond to Discord that we at least received the event and handled it accordingly
		defer session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})

		if interaction.Member == nil {
			return
		}

		// Handle the bug button click!
		go startNewReportConversation(interaction.Member.User.ID, interaction.ChannelID)
	}
}
