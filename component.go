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

		if interaction.Member == nil {
			session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseDeferredMessageUpdate,
			})
			return
		}

		// Handle the bug button click!
		startNewReportConversation(interaction.Member.User.ID, interaction.ChannelID)

		// Reply to Discord with that everything succeeded
		session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredMessageUpdate,
		})
	}
}
